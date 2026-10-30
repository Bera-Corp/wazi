// Package probe builds a Phase 0 host-profile record from live host
// inventory. It reports facts only: no mutation, no secret material.
package probe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
)

// commandTimeout bounds every external probe command so inventory
// cannot hang on an unresponsive daemon.
const commandTimeout = 5 * time.Second

// versionRegexp matches a full semantic version, preserving any
// prerelease suffix (-rc1, -beta.2) and build metadata (+ds1).
// Preserving the prerelease is load-bearing: stripping it would turn
// 27.3.1-rc1 into the stable 27.3.1 and defeat compatibility checks.
var versionRegexp = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?`)

// stableVersion is the only form the Phase 0 contract accepts
// (schemas/host-profile.schema.json, scripts/validate.py version_tuple).
var stableVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// commandOutput runs an external command with a timeout. It is a
// variable so tests can substitute canned daemon responses.
var commandOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// CurrentHostID returns the hostId this machine's profile will
// carry, so pairing enrollment binds the same identity the agent
// will later submit profiles for.
func CurrentHostID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("hostname: %w", err)
	}
	return "host_" + sanitizeID(hostname), nil
}

func sanitizeID(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	return builder.String()
}

// parseVersionToken extracts the first full version token from raw
// command output, preserving any prerelease/build suffix exactly as
// reported. It never silently truncates a prerelease into a stable
// version.
func parseVersionToken(raw string) (string, error) {
	version := versionRegexp.FindString(raw)
	if version == "" {
		return "", fmt.Errorf("could not parse version from %q", strings.TrimSpace(raw))
	}
	return version, nil
}

// normalizeStableVersion converts a reported version token into the
// stable form stored in a host profile. Build metadata (+ds1, the
// distro packaging suffix on e.g. Ubuntu Compose builds) does not
// change the upstream release and is dropped. A prerelease suffix
// means the runtime is not a stable release: it is rejected with the
// raw identity preserved in the error, never silently accepted as
// the corresponding stable version.
func normalizeStableVersion(reported string) (string, error) {
	// Build metadata is stripped first: its suffix may itself
	// contain hyphens (2.40.3+ds1-0ubuntu1) that are packaging
	// detail, not a prerelease of the upstream version.
	core, _, _ := strings.Cut(reported, "+")
	if strings.Contains(core, "-") {
		return "", fmt.Errorf("unsupported prerelease runtime version %q: only stable releases are supported", reported)
	}
	if !stableVersion.MatchString(core) {
		return "", fmt.Errorf("unsupported runtime version format %q", reported)
	}
	return core, nil
}

// dockerDaemonVersion queries the Docker daemon (server), not the
// client. `docker --version` reports the client binary, which can
// differ from the daemon that executes workloads; only the server
// version is compatibility evidence.
func dockerDaemonVersion(ctx context.Context) (string, error) {
	out, err := commandOutput(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return "", err
	}
	reported, err := parseVersionToken(string(out))
	if err != nil {
		return "", fmt.Errorf("docker daemon version: %w", err)
	}
	stable, err := normalizeStableVersion(reported)
	if err != nil {
		return "", fmt.Errorf("docker daemon version: %w", err)
	}
	return stable, nil
}

// dockerComposeVersion reports the Compose plugin version. Compose
// has no daemon counterpart, so the client plugin is the correct
// source; prerelease identities are still preserved and rejected.
func dockerComposeVersion(ctx context.Context) (string, error) {
	out, err := commandOutput(ctx, "docker", "compose", "version")
	if err != nil {
		return "", err
	}
	reported, err := parseVersionToken(string(out))
	if err != nil {
		return "", fmt.Errorf("docker compose version: %w", err)
	}
	stable, err := normalizeStableVersion(reported)
	if err != nil {
		return "", fmt.Errorf("docker compose version: %w", err)
	}
	return stable, nil
}

// verifyLocalDockerContext refuses remote Docker contexts. Host
// inventory mixes local facts (CPU, RAM, ports, filesystem) with
// daemon facts (versions, Docker root); a remote context would
// silently attribute another machine's daemon to this host.
func verifyLocalDockerContext(ctx context.Context) error {
	out, err := commandOutput(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if err != nil {
		return fmt.Errorf("docker context probe: %w", err)
	}
	endpoint := strings.TrimSpace(string(out))
	if strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "npipe://") {
		return nil
	}
	return fmt.Errorf("docker context endpoint %q is not local (unix:// or npipe://): refusing to mix remote daemon facts with local host observations", endpoint)
}

// dockerRootDir returns the daemon's data root directory.
func dockerRootDir(ctx context.Context) (string, error) {
	out, err := commandOutput(ctx, "docker", "info", "--format", "{{.DockerRootDir}}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// parseFilesystemType selects, from /proc/mounts content, the
// filesystem type of the mount containing path (longest
// mount-point prefix). Mount points are space-escaped (\040);
// that one sequence is unescaped.
func parseFilesystemType(mounts, path string) string {
	if path == "" {
		path = "/"
	}
	bestLen := -1
	bestType := "unknown"
	for _, line := range strings.Split(mounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPoint := strings.ReplaceAll(fields[1], `\040`, " ")
		if mountPoint == path {
			return fields[2]
		}
		if mountPoint == "/" || strings.HasPrefix(path, mountPoint+"/") {
			if len(mountPoint) > bestLen {
				bestLen, bestType = len(mountPoint), fields[2]
			}
		}
	}
	return bestType
}

// parseOccupiedPorts parses /proc/net/{tcp,tcp6,udp,udp6} content.
// TCP: only LISTEN (0A) sockets are deployment blockers, because a
// connected TCP socket's local port is normally ephemeral; a caller
// that needs ephemeral collision checks can widen this later.
// UDP: every socket with a bound local port retains that port,
// including connected sockets (state 01, ESTABLISHED). Filtering UDP
// to the unconnected state (07) would miss connected sockets that
// still hold their port, so all UDP states are included.
func parseOccupiedPorts(data string, isTCP bool) []int {
	seen := map[int]bool{}
	lines := strings.Split(data, "\n")
	if len(lines) == 0 {
		return nil
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if isTCP && fields[3] != "0A" {
			continue
		}
		// UDP: include all states (07 unconnected/listening and
		// 01 connected both retain the bound local port).
		_, portHex, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err == nil && port > 0 && port <= 65535 {
			seen[int(port)] = true
		}
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

// probeWritableDir proves writability by the operation the executor
// actually needs: creating, writing and removing a file in dir. It
// fails closed (false) on any error, including a missing directory.
// Merely stating that the directory exists is not evidence.
func probeWritableDir(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".wazi-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	if _, err := f.Write([]byte{0}); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return false
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return false
	}
	return os.Remove(name) == nil
}

// Build collects the current host inventory into a host-profile record
// valid for 30 minutes from observedAt, matching the Phase 0 example
// freshness policy. The host ID is derived from the hostname.
func Build(tenantScope string) (contracts.HostProfile, error) {
	return buildWithContext(context.Background(), tenantScope, "")
}

// BuildForHost is Build with an explicit node identity. The CLI
// resolves one identity (flag, existing certificate, or hostname)
// and passes it here so enrollment and inventory can never disagree
// about which node this is. An empty hostID derives from hostname.
func BuildForHost(tenantScope, hostID string) (contracts.HostProfile, error) {
	return buildWithContext(context.Background(), tenantScope, hostID)
}

func buildWithContext(ctx context.Context, tenantScope, hostID string) (contracts.HostProfile, error) {
	var profile contracts.HostProfile
	if hostID == "" {
		derived, err := CurrentHostID()
		if err != nil {
			return profile, err
		}
		hostID = derived
	}
	now := time.Now().UTC()
	profile.SchemaVersion = "1.0.0"
	profile.HostID = hostID
	profile.TenantScope = tenantScope
	profile.ObservedAt = now.Format(time.RFC3339)
	profile.ExpiresAt = now.Add(30 * time.Minute).Format(time.RFC3339)

	profile.OS.Family = runtime.GOOS
	profile.OS.Distribution, profile.OS.Version = osRelease()
	profile.Architecture = map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
	if profile.Architecture == "" {
		profile.Architecture = runtime.GOARCH
	}
	profile.Resources.CPUCores = float64(runtime.NumCPU())
	profile.Resources.RAMMb = totalRAMMb()

	// Local facts (CPU/RAM/ports/filesystem) must never be mixed
	// with a remote daemon's facts: refuse non-local Docker
	// contexts before trusting any daemon answer.
	contextCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	err := verifyLocalDockerContext(contextCtx)
	cancel()
	if err != nil {
		return profile, err
	}

	engineCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	engine, err := dockerDaemonVersion(engineCtx)
	cancel()
	if err != nil {
		return profile, fmt.Errorf("docker engine probe: %w", err)
	}
	composeCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	compose, err := dockerComposeVersion(composeCtx)
	cancel()
	if err != nil {
		return profile, fmt.Errorf("docker compose probe: %w", err)
	}
	profile.Docker.EngineVersion = engine
	profile.Docker.ComposeVersion = compose

	rootCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	dockerRoot, err := dockerRootDir(rootCtx)
	cancel()
	if err != nil {
		return profile, fmt.Errorf("docker root probe: %w", err)
	}
	// Storage is measured on the filesystem the deployment will
	// actually consume (the Docker root), not always on /.
	storagePath := dockerRoot
	if storagePath == "" {
		storagePath = "/"
	}
	profile.Resources.StorageFreeMb = storageFreeMb(storagePath)

	ports, err := occupiedPorts()
	if err != nil {
		// An unreadable port table is uncertainty, not an empty
		// host: fail instead of reporting ports as free.
		return profile, fmt.Errorf("occupied ports probe: %w", err)
	}
	profile.OccupiedPorts = ports
	profile.Filesystem.Type = filesystemTypeFor(storagePath)
	profile.Filesystem.DockerRootWritable = dockerRoot != "" && probeWritableDir(dockerRoot)
	profile.Network.Mode = "byo-vps"
	profile.Network.PublicExposureCapable = false
	profile.Provenance.SourceType = "host-agent-probe"
	profile.Provenance.ExtractionMethod = "host-probe"
	profile.Redaction.Applied = true
	profile.Redaction.SecretValuesPresent = false
	return profile, nil
}
