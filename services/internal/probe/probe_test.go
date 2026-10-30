package probe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSanitizeID(t *testing.T) {
	cases := map[string]string{
		"bera-003":     "bera-003",
		"My Host_01":   "my-host_01",
		"host.example": "host-example",
	}
	for input, want := range cases {
		if got := sanitizeID(input); got != want {
			t.Errorf("sanitizeID(%q) = %q, want %q", input, got, want)
		}
	}
}

// The daemon, not the client, is compatibility evidence (P1). The
// stub records the exact command the probe issues.
func TestDaemonVersionQueriesServerNotClient(t *testing.T) {
	var gotName string
	var gotArgs []string
	restore := stubCommand(func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte("29.1.3\n"), nil
	})
	defer restore()

	version, err := dockerDaemonVersion(context.Background())
	if err != nil {
		t.Fatalf("dockerDaemonVersion: %v", err)
	}
	if version != "29.1.3" {
		t.Errorf("daemon version = %q, want 29.1.3", version)
	}
	joined := gotName + " " + strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "version") || !strings.Contains(joined, "{{.Server.Version}}") {
		t.Errorf("daemon probe command = %q, want docker version --format {{{{.Server.Version}}}}", joined)
	}
	if strings.Contains(joined, "--version") && !strings.Contains(joined, "Server") {
		t.Errorf("daemon probe used the client-only --version form: %q", joined)
	}
}

// A prerelease daemon must never be silently accepted as the stable
// release it precedes (P1: 27.3.1-rc1 must not become 27.3.1).
func TestDaemonVersionRejectsPrereleasePreservingIdentity(t *testing.T) {
	restore := stubCommand(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("27.3.1-rc1\n"), nil
	})
	defer restore()

	reported, err := parseVersionToken("27.3.1-rc1")
	if err != nil || reported != "27.3.1-rc1" {
		t.Fatalf("parseVersionToken preserved = %q, %v; want exact 27.3.1-rc1", reported, err)
	}
	if _, err := dockerDaemonVersion(context.Background()); err == nil {
		t.Fatal("prerelease daemon version was accepted")
	} else if !strings.Contains(err.Error(), "27.3.1-rc1") {
		t.Errorf("rejection must preserve the prerelease identity, got: %v", err)
	}
}

func TestVersionNormalization(t *testing.T) {
	cases := []struct {
		reported string
		want     string
		wantErr  bool
	}{
		{"29.1.3", "29.1.3", false},
		{"2.40.3", "2.40.3", false},
		// Ubuntu packaging suffix is build metadata, not a prerelease.
		{"2.40.3+ds1-0ubuntu1", "2.40.3", false},
		{"27.3.1-rc1", "", true},
		{"27.3.1-beta.2", "", true},
		{"not-a-version", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeStableVersion(tc.reported)
		if tc.wantErr && err == nil {
			t.Errorf("normalizeStableVersion(%q) = %q, want error", tc.reported, got)
		}
		if !tc.wantErr && (err != nil || got != tc.want) {
			t.Errorf("normalizeStableVersion(%q) = %q, %v; want %q", tc.reported, got, err, tc.want)
		}
	}
	if got, err := parseVersionToken("Docker Compose version 2.40.3+ds1-0ubuntu1"); err != nil || got != "2.40.3+ds1-0ubuntu1" {
		t.Errorf("compose token = %q, %v; want preserved 2.40.3+ds1-0ubuntu1", got, err)
	}
	if got, err := dockerComposeVersionWith("Docker Compose version v2.40.3"); err != nil || got != "2.40.3" {
		t.Errorf("compose stable = %q, %v; want 2.40.3", got, err)
	}
}

// Connected UDP sockets retain their bound port and must be
// reported occupied; only LISTEN TCP sockets block deployment (P2).
func TestParseOccupiedPortsIncludesConnectedUDP(t *testing.T) {
	udp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"  10: 0100007F:1F90 0200007F:0035 01 00000000:00000000 00:00000000 00000000  1000  0 111 2 0 0\n" + // 127.0.0.1:8080 connected (st 01)
		"  11: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000  0 112 2 0 0\n" // 0.0.0.0:53 unconnected (st 07)
	if got := parseOccupiedPorts(udp, false); !slices.Equal(got, []int{53, 8080}) {
		t.Errorf("UDP ports = %v, want [53 8080] (connected socket included)", got)
	}
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0  0 211 1 0 100 0 0\n" + // 443 LISTEN
		"   1: 0100007F:1F90 0200007F:01BB 01 00000000:00000000 00:00000000 00000000  1000  0 212 1 0 100 0 0\n" // 8080 ESTABLISHED
	if got := parseOccupiedPorts(tcp, true); !slices.Equal(got, []int{443}) {
		t.Errorf("TCP ports = %v, want [443] (only LISTEN)", got)
	}
}

// Writability is proven by a write, not by existence (P2). A path
// that merely exists (a regular file, a missing dir) must fail.
func TestProbeWritableDirRequiresActualWrite(t *testing.T) {
	dir := t.TempDir()
	if !probeWritableDir(dir) {
		t.Error("writable temp dir reported not writable")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("probe left files behind: %v, %v", entries, err)
	}
	if probeWritableDir(filepath.Join(dir, "does-not-exist")) {
		t.Error("missing directory reported writable")
	}
	file := filepath.Join(dir, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if probeWritableDir(file) {
		t.Error("regular file reported as writable directory")
	}
	if probeWritableDir("") {
		t.Error("empty path reported writable")
	}
}

// Remote Docker contexts must never mix another machine's daemon
// facts with this host's local inventory.
func TestVerifyLocalDockerContext(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		wantErr  bool
	}{
		{"unix:///var/run/docker.sock", false},
		{"npipe:////./pipe/docker_engine", false},
		{"tcp://remote.example:2376", true},
		{"ssh://user@remote.example", true},
		{"https://remote.example:2376", true},
	} {
		restore := stubCommand(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(tc.endpoint + "\n"), nil
		})
		err := verifyLocalDockerContext(context.Background())
		restore()
		if tc.wantErr && err == nil {
			t.Errorf("endpoint %q accepted, want refusal", tc.endpoint)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("endpoint %q refused: %v", tc.endpoint, err)
		}
	}
}

// Build uses the resolved identity, the daemon (server) versions,
// the Docker root for storage/writability, and the local context.
func TestBuildUsesResolvedHostAndDockerRoot(t *testing.T) {
	root := t.TempDir()
	restore := stubCommand(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "Endpoints.docker.Host"):
			return []byte("unix:///var/run/docker.sock\n"), nil
		case strings.Contains(joined, "Server.Version"):
			return []byte("29.1.3\n"), nil
		case strings.Contains(joined, "compose"):
			return []byte("Docker Compose version 2.40.3\n"), nil
		case strings.Contains(joined, "DockerRootDir"):
			return []byte(root + "\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %q", joined)
	})
	defer restore()

	profile, err := BuildForHost("tenant_a", "host_custom42")
	if err != nil {
		t.Fatalf("BuildForHost: %v", err)
	}
	if profile.HostID != "host_custom42" {
		t.Errorf("profile hostId = %q, want the resolved host_custom42", profile.HostID)
	}
	if profile.Docker.EngineVersion != "29.1.3" || profile.Docker.ComposeVersion != "2.40.3" {
		t.Errorf("versions = %q/%q", profile.Docker.EngineVersion, profile.Docker.ComposeVersion)
	}
	if !profile.Filesystem.DockerRootWritable {
		t.Error("temp Docker root should be proven writable")
	}
}

func TestBuildRejectsRemoteDockerContext(t *testing.T) {
	restore := stubCommand(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "Endpoints.docker.Host") {
			return []byte("tcp://remote.example:2376\n"), nil
		}
		return []byte("29.1.3\n"), nil
	})
	defer restore()
	if _, err := BuildForHost("tenant_a", "host_custom42"); err == nil || !strings.Contains(err.Error(), "not local") {
		t.Errorf("Build with remote context err = %v, want not-local refusal", err)
	}
}

func TestParseFilesystemTypeSelectsContainingMount(t *testing.T) {
	mounts := "/dev/sda1 / ext4 rw 0 0\n" +
		"/dev/sdb1 /var/lib/docker xfs rw 0 0\n" +
		"overlay /var/lib/docker/overlay2 overlay rw 0 0\n"
	if got := parseFilesystemType(mounts, "/var/lib/docker"); got != "xfs" {
		t.Errorf("docker root fs = %q, want xfs (not the / filesystem)", got)
	}
	if got := parseFilesystemType(mounts, "/var/lib/docker/overlay2/abc"); got != "overlay" {
		t.Errorf("nested docker path fs = %q, want overlay (longest prefix)", got)
	}
	if got := parseFilesystemType(mounts, "/var/lib/docker/volumes/app"); got != "xfs" {
		t.Errorf("docker volume path fs = %q, want xfs (containing mount)", got)
	}
	if got := parseFilesystemType(mounts, "/home/user"); got != "ext4" {
		t.Errorf("root-contained path fs = %q, want ext4", got)
	}
	if got := parseFilesystemType("", "/var/lib/docker"); got != "unknown" {
		t.Errorf("empty mounts fs = %q, want unknown", got)
	}
}

func stubCommand(fn func(context.Context, string, ...string) ([]byte, error)) func() {
	prev := commandOutput
	commandOutput = fn
	return func() { commandOutput = prev }
}

func dockerComposeVersionWith(output string) (string, error) {
	restore := stubCommand(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(output), nil
	})
	defer restore()
	return dockerComposeVersion(context.Background())
}
