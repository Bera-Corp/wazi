package agentclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// DockerPilotConfig is operator-owned configuration, never accepted from a
// deployment request. ImageConfigDigests binds verified OCI archive provenance
// to the actual image ID loaded by Docker. No mutable image tags are executed.
type DockerPilotConfig struct {
	TenantScope, HostID, Project, Endpoint, DataRoot, Home, StateDirectory string
	RuntimeUID                                                             int
	AllowedManifests                                                       map[string]bool
	ImageConfigDigests                                                     map[string]string
	QuotaHelper                                                            string
}

type DockerPilotBoundary struct {
	config   DockerPilotConfig
	artifact []byte
	run      func(context.Context, io.Reader, io.Writer, ...string) ([]byte, error)
}

const (
	// A freshly started Gitea needs several seconds before its web server
	// accepts requests; the health gate polls the exact approved target
	// until it answers 200 or the bounded deadline expires.
	healthTimeout      = 90 * time.Second
	healthPollInterval = 2 * time.Second
)

// waitForHealth polls target until it returns 200. Only the exact target
// is requested, redirects are never followed, and any non-200 outcome
// keeps polling until the deadline; it never reports success early.
func waitForHealth(ctx context.Context, target string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("health redirects are forbidden") }}
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err == nil {
			var response *http.Response
			response, err = client.Do(request)
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
				err = fmt.Errorf("health check did not pass: status %d", response.StatusCode)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("health check did not pass before deadline: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func NewDockerPilotBoundary(config DockerPilotConfig, artifact []byte) (*DockerPilotBoundary, error) {
	if config.TenantScope == "" || config.HostID == "" || config.Project != "wazi-gitea-phase1" || config.RuntimeUID <= 0 ||
		config.Endpoint != "unix:///run/user/"+strconv.Itoa(config.RuntimeUID)+"/docker.sock" ||
		config.DataRoot != "/srv/wazi-runtime/home/.local/share/docker" || config.Home != "/srv/wazi-runtime/home" ||
		config.QuotaHelper != "/usr/local/libexec/wazi-phase1-quota" || len(config.AllowedManifests) != 2 || len(config.ImageConfigDigests) != 2 {
		return nil, fmt.Errorf("pilot runtime requires the exact operator-pinned rootless boundary and A/B releases")
	}
	if !filepath.IsAbs(config.StateDirectory) || !strings.HasPrefix(filepath.Clean(config.StateDirectory), "/srv/wazi-runtime/") {
		return nil, fmt.Errorf("pilot state must remain on the quota-managed runtime filesystem")
	}
	if _, err := parseCanonicalObject(artifact, "compose artifact"); err != nil {
		return nil, err
	}
	// Copy mutable configuration so a caller cannot change trust after construction.
	allowed := make(map[string]bool, len(config.AllowedManifests))
	for k, v := range config.AllowedManifests {
		allowed[k] = v
	}
	config.AllowedManifests = allowed
	images := make(map[string]string, len(config.ImageConfigDigests))
	for k, v := range config.ImageConfigDigests {
		images[k] = v
	}
	config.ImageConfigDigests = images
	b := &DockerPilotBoundary{config: config, artifact: append([]byte(nil), artifact...)}
	b.run = b.command
	return b, nil
}

// command supplies a clean environment, pins the Unix socket on every Docker
// call, bounds output and duration, and never invokes a shell.
func (b *DockerPilotBoundary) command(ctx context.Context, input io.Reader, output io.Writer, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/docker", append([]string{"--host", b.config.Endpoint}, args...)...)
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + b.config.Home, "XDG_RUNTIME_DIR=/run/user/" + strconv.Itoa(b.config.RuntimeUID)}
	command.Stdin = input
	var data boundedOutput
	if output != nil {
		command.Stdout = output
	} else {
		command.Stdout = &data
	}
	var diagnostics boundedOutput
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(diagnostics.String())
		if len(detail) > 512 {
			detail = detail[:512]
		}
		if detail != "" {
			return nil, fmt.Errorf("rootless Docker operation failed: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("rootless Docker operation failed: %w", err)
	}
	return data.Bytes(), nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("Docker response exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}

// rootfulDenialEstablished reports whether a failed rootful-socket dial
// proves the runtime identity cannot use that socket: either the dial
// itself was denied, or (under sandboxes that surface other errors) the
// socket exists and its mode bits deny this identity write access.
func rootfulDenialEstablished(err error, uid int, groups func() ([]int, error)) bool {
	return rootfulDenialEstablishedFor("/var/run/docker.sock", err, uid, groups)
}

func rootfulDenialEstablishedFor(path string, err error, uid int, groups func() ([]int, error)) bool {
	if errors.Is(err, fs.ErrPermission) {
		return true
	}
	return socketNotWritableBy(path, uid, groups)
}

func (b *DockerPilotBoundary) VerifyRootless(ctx context.Context) error {
	if runtime.GOOS != "linux" || os.Getuid() != b.config.RuntimeUID {
		return fmt.Errorf("agent is not the approved non-root runtime UID")
	}
	rootful, rootfulErr := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", "/var/run/docker.sock")
	if rootfulErr == nil {
		rootful.Close()
		return fmt.Errorf("runtime identity can access the rootful Docker socket")
	}
	if !rootfulDenialEstablished(rootfulErr, os.Getuid(), os.Getgroups) {
		return fmt.Errorf("rootful socket permission denial could not be established")
	}
	raw, err := b.run(ctx, nil, nil, "info", "--format", "{{json .}}")
	if err != nil {
		return err
	}
	var info struct {
		DockerRootDir, OSType, CgroupVersion string
		SecurityOptions                      []string
	}
	if json.Unmarshal(raw, &info) != nil || info.DockerRootDir != b.config.DataRoot || info.OSType != "linux" || info.CgroupVersion != "2" {
		return fmt.Errorf("Docker daemon does not match the approved runtime boundary")
	}
	for _, option := range info.SecurityOptions {
		if option == "name=rootless" {
			return nil
		}
	}
	return fmt.Errorf("Docker daemon is not rootless")
}

// RenderPilotCompose is the closed 0.4.0 renderer for the one-app pilot. Its
// output is JSON (valid Compose YAML) with no executable commands, bind mounts,
// build contexts, host namespaces, secret interpolation, or mutable image tags.
func RenderPilotCompose(manifestRaw []byte, imageConfigs map[string]string) ([]byte, error) {
	m, err := parseCanonicalObject(manifestRaw, "manifest")
	if err != nil {
		return nil, err
	}
	if m["appId"] != "org.gitea.gitea" {
		return nil, fmt.Errorf("pilot only supports Gitea")
	}
	r, ok := objectValue(m["release"])
	if !ok {
		return nil, fmt.Errorf("missing release")
	}
	images, err := stringObjects(r["images"], "images")
	if err != nil || len(images) != 1 || images[0]["service"] != "gitea" || images[0]["image"] != "ghcr.io/bera-corp/wazi-gitea-phase1" {
		return nil, fmt.Errorf("pilot image surface is not supported")
	}
	digest, _ := images[0]["digest"].(string)
	configDigest := imageConfigs[digest]
	if !strings.HasPrefix(configDigest, "sha256:") || len(configDigest) != 71 {
		return nil, fmt.Errorf("image has no verified archive-to-config binding")
	}
	limits, err := stringObjects(r["resourceLimits"], "resource limits")
	if err != nil || len(limits) != 1 || limits[0]["service"] != "gitea" || limits[0]["cpuCores"] != json.Number("2") || limits[0]["ramMb"] != json.Number("1024") {
		return nil, fmt.Errorf("pilot service limits must match the approved allocation")
	}
	volumes, err := stringObjects(r["volumes"], "volumes")
	if err != nil || len(volumes) != 2 {
		return nil, fmt.Errorf("pilot requires exactly its data and config volumes")
	}
	mounts := []any{}
	volumeDefinitions := map[string]any{}
	for _, v := range volumes {
		name, _ := v["name"].(string)
		path, _ := v["mountPath"].(string)
		if v["service"] != "gitea" || v["backup"] != true || !((name == "gitea-data" && path == "/var/lib/gitea" && v["storageLimitMb"] == json.Number("18432")) || (name == "gitea-config" && path == "/etc/gitea" && v["storageLimitMb"] == json.Number("2048"))) {
			return nil, fmt.Errorf("pilot volume declaration is outside the approved allocation")
		}
		if _, duplicate := volumeDefinitions[name]; duplicate {
			return nil, fmt.Errorf("duplicate volume")
		}
		mounts = append(mounts, map[string]any{"type": "volume", "source": name, "target": path})
		volumeDefinitions[name] = map[string]any{"external": true, "name": "wazi-gitea-phase1_" + name}
	}
	ports, err := stringObjects(r["ports"], "ports")
	if err != nil || len(ports) != 1 || ports[0]["service"] != "gitea" || ports[0]["containerPort"] != json.Number("3000") || ports[0]["hostPort"] != json.Number("13000") || ports[0]["exposure"] != "loopback" {
		return nil, fmt.Errorf("pilot only permits loopback port 13000")
	}
	environment := map[string]any{}
	entries, err := optionalObjects(r["environment"], "environment")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name, n := e["name"].(string)
		value, v := e["default"].(string)
		if !n || !v || e["service"] != "gitea" || !strings.HasPrefix(name, "GITEA__") || strings.ContainsAny(value, "\r\n") || e["secretRef"] != nil {
			return nil, fmt.Errorf("pilot environment entry is not supported")
		}
		if _, dup := environment[name]; dup {
			return nil, fmt.Errorf("duplicate environment entry")
		}
		environment[name] = strings.ReplaceAll(value, "$", "$$")
	}
	service := map[string]any{"image": configDigest, "pull_policy": "never", "cpus": json.Number("2"), "mem_limit": "1073741824b", "pids_limit": json.Number("256"), "security_opt": []any{"no-new-privileges:true"}, "cap_drop": []any{"ALL"}, "ports": []any{"127.0.0.1:13000:3000"}, "volumes": mounts, "environment": environment, "labels": map[string]any{"dev.wazi.pilot": "phase1-gitea"}, "logging": map[string]any{"driver": "json-file", "options": map[string]any{"max-size": "10m", "max-file": "2"}}}
	return verify.CanonicalJSON(map[string]any{"services": map[string]any{"gitea": service}, "volumes": volumeDefinitions})
}

func (b *DockerPilotBoundary) boundCompose(p PreparedExecution) ([]byte, error) {
	if !p.Verified() || p.TenantScope() != b.config.TenantScope || p.HostID() != b.config.HostID || !b.config.AllowedManifests[p.Binding().Release.ManifestDigest] {
		return nil, fmt.Errorf("execution is outside the approved pilot identity or release")
	}
	a, err := parseCanonicalObject(b.artifact, "compose artifact")
	if err != nil {
		return nil, err
	}
	artifactDigest, err := verify.CanonicalDigest(a)
	if err != nil {
		return nil, err
	}
	envelope, err := parseCanonicalObject(p.Envelope(), "envelope")
	if err != nil {
		return nil, err
	}
	ref, ok := objectValue(envelope["composeArtifactRef"])
	if !ok || ref["digest"] != artifactDigest || ref["recordId"] != a["artifactId"] || a["manifestDigest"] != p.Binding().Release.ManifestDigest || a["projectName"] != b.config.Project {
		return nil, fmt.Errorf("compose artifact is not bound to the approved envelope and release")
	}
	renderer, ok := objectValue(a["renderer"])
	if !ok || renderer["name"] != "wazi-compose-renderer" || renderer["version"] != "0.4.0" {
		return nil, fmt.Errorf("unsupported pilot renderer")
	}
	compose, err := RenderPilotCompose(p.Manifest(), b.config.ImageConfigDigests)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(compose)
	if a["composeDigest"] != fmt.Sprintf("sha256:%x", hash) {
		return nil, fmt.Errorf("rendered artifact digest does not match approved bytes")
	}
	return compose, nil
}

func (b *DockerPilotBoundary) ValidatePrepared(p PreparedExecution) error {
	if _, err := b.boundCompose(p); err != nil {
		return err
	}
	plan, _, err := decodePrepared(p)
	if err != nil {
		return err
	}
	for _, sequence := range [][]planStep{plan.Steps} {
		for _, step := range sequence {
			switch step.Capability {
			case "manifest.verify", "compose.pull", "compose.up", "compose.down", "compose.ps", "health.check", "volume.backup", "volume.restore":
			default:
				return fmt.Errorf("capability %q has no qualified pilot adapter", step.Capability)
			}
		}
	}
	return nil
}

func (b *DockerPilotBoundary) Execute(ctx context.Context, p PreparedExecution, step Step) (Outcome, error) {
	compose, err := b.boundCompose(p)
	if err != nil {
		return Outcome{}, err
	}
	if err := b.VerifyRootless(ctx); err != nil {
		return Outcome{}, err
	}
	success := Outcome{State: "succeeded"}
	switch step.Capability {
	case "manifest.verify":
		return success, nil
	case "compose.pull":
		// Archives are operator-imported only after provenance verification. The
		// typed pull step proves the exact attested image ID is available offline.
		id := b.config.ImageConfigDigests[step.Parameters.ComposePull.ImageDigest]
		if id == "" {
			return Outcome{}, fmt.Errorf("unapproved image")
		}
		raw, err := b.run(ctx, nil, nil, "image", "inspect", id, "--format", "{{.Id}}")
		if err != nil || strings.TrimSpace(string(raw)) != id {
			return Outcome{}, fmt.Errorf("attested image is not loaded")
		}
		return success, nil
	case "compose.up":
		if step.Parameters.ComposeUp.ProjectName != b.config.Project {
			return Outcome{}, fmt.Errorf("project mismatch")
		}
		if err := b.ensureVolumes(ctx, p); err != nil {
			return Outcome{}, err
		}
		_, err = b.compose(ctx, compose, "up", "--detach", "--no-build", "--pull", "never", "gitea")
		if err == nil {
			err = b.inspectLimits(ctx)
		}
		return success, err
	case "compose.down":
		if step.Parameters.ComposeDown.ProjectName != b.config.Project || step.Parameters.ComposeDown.RemoveVolumes {
			return Outcome{}, fmt.Errorf("destructive or foreign project request")
		}
		_, err = b.compose(ctx, compose, "down", "--timeout", "30")
		return success, err
	case "volume.backup":
		if step.Parameters.VolumeBackup == nil {
			return Outcome{}, fmt.Errorf("backup parameters are required")
		}
		imageID, err := b.pilotImageID(p)
		if err != nil {
			return Outcome{}, err
		}
		if err := b.backupVolume(ctx, compose, *step.Parameters.VolumeBackup, imageID); err != nil {
			return Outcome{}, err
		}
		return success, nil
	case "volume.restore":
		if step.Parameters.VolumeRestore == nil {
			return Outcome{}, fmt.Errorf("restore parameters are required")
		}
		// Stopping the exact pilot project first is part of restore:
		// volume bytes are never swapped under a running container.
		if _, err := b.compose(ctx, compose, "down", "--timeout", "30"); err != nil {
			return Outcome{}, err
		}
		imageID, err := b.pilotImageID(p)
		if err != nil {
			return Outcome{}, err
		}
		restore := volumeRestoreRequest{
			VolumeDir:      b.volumeDir(step.Parameters.VolumeRestore.Volume),
			BackupsRoot:    filepath.Join(b.config.StateDirectory, "backups"),
			PreRestoreRoot: filepath.Join(b.config.StateDirectory, "pre-restore"),
			Volume:         step.Parameters.VolumeRestore.Volume,
			BackupID:       step.Parameters.VolumeRestore.BackupID,
		}
		if err := restoreVolumeStream(ctx, b.streamer(imageID), restore); err != nil {
			return Outcome{}, err
		}
		return success, nil
	case "compose.ps":
		_, err = b.compose(ctx, compose, "ps", "--all", "--format", "json")
		return success, err
	case "health.check":
		if step.Parameters.HealthCheck.Target != "http://127.0.0.1:13000/api/healthz" {
			return Outcome{}, fmt.Errorf("unapproved health target")
		}
		if err := waitForHealth(ctx, step.Parameters.HealthCheck.Target, healthTimeout, healthPollInterval); err != nil {
			return Outcome{}, err
		}
		return success, nil
	default:
		return Outcome{}, fmt.Errorf("capability %q has no qualified pilot adapter", step.Capability)
	}
}

func (b *DockerPilotBoundary) compose(ctx context.Context, raw []byte, args ...string) ([]byte, error) {
	// Compose reads exact approved bytes from stdin, with all interpolation
	// disabled. No workspace .env file or user-selected path can participate.
	base := []string{"compose", "--project-name", b.config.Project, "--project-directory", b.config.StateDirectory, "--env-file", "/dev/null", "--file", "-"}
	return b.run(ctx, bytes.NewReader(raw), nil, append(base, args...)...)
}

func (b *DockerPilotBoundary) ensureVolumes(ctx context.Context, p PreparedExecution) error {
	m, err := parseCanonicalObject(p.Manifest(), "manifest")
	if err != nil {
		return err
	}
	r, _ := objectValue(m["release"])
	volumes, err := stringObjects(r["volumes"], "volumes")
	if err != nil {
		return err
	}
	plan, err := parseCanonicalObject(p.Plan(), "plan")
	if err != nil {
		return err
	}
	if plan["operation"] == "install" {
		list, listErr := b.run(ctx, nil, nil, "volume", "ls", "--format", "{{.Name}}")
		if listErr != nil {
			return listErr
		}
		for _, existing := range strings.Fields(string(list)) {
			if existing == b.config.Project+"_gitea-data" || existing == b.config.Project+"_gitea-config" {
				return fmt.Errorf("fresh install requires absent application volumes; existing data needs a checkpointed lifecycle")
			}
		}
	}
	for _, v := range volumes {
		name := b.config.Project + "_" + v["name"].(string)
		raw, inspectErr := b.run(ctx, nil, nil, "volume", "inspect", name, "--format", "{{json .Labels}}")
		if inspectErr != nil {
			// Only absence, proven by a successful list, permits creation.
			list, listErr := b.run(ctx, nil, nil, "volume", "ls", "--format", "{{.Name}}")
			if listErr != nil {
				return listErr
			}
			for _, existing := range strings.Fields(string(list)) {
				if existing == name {
					return fmt.Errorf("existing volume cannot be inspected")
				}
			}
			_, err = b.run(ctx, nil, nil, "volume", "create", "--label", "dev.wazi.project="+b.config.Project, "--label", "dev.wazi.tenant="+b.config.TenantScope, name)
			if err != nil {
				return err
			}
		} else {
			var labels map[string]string
			if json.Unmarshal(raw, &labels) != nil || labels["dev.wazi.project"] != b.config.Project || labels["dev.wazi.tenant"] != b.config.TenantScope {
				return fmt.Errorf("volume ownership mismatch")
			}
		}
		limit, _ := v["storageLimitMb"].(json.Number)
		command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", b.config.QuotaHelper, name, string(limit))
		command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
		if err := command.Run(); err != nil {
			return fmt.Errorf("hard volume quota was not applied and verified: %w", err)
		}
	}
	return nil
}

func (b *DockerPilotBoundary) inspectLimits(ctx context.Context) error {
	ids, err := b.run(ctx, nil, nil, "ps", "--all", "--filter", "label=com.docker.compose.project="+b.config.Project, "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	values := strings.Fields(string(ids))
	if len(values) != 1 {
		return fmt.Errorf("pilot must have exactly one application container")
	}
	raw, err := b.run(ctx, nil, nil, "inspect", values[0], "--format", "{{json .HostConfig}}")
	if err != nil {
		return err
	}
	var c struct {
		Memory, NanoCpus int64
		Privileged       bool
		NetworkMode      string
	}
	if json.Unmarshal(raw, &c) != nil || c.Memory != 1073741824 || c.NanoCpus != 2000000000 || c.Privileged || c.NetworkMode == "host" {
		return fmt.Errorf("runtime resource limits do not match the approved service")
	}
	return nil
}

func (b *DockerPilotBoundary) volumeDir(volume string) string {
	return filepath.Join(b.config.DataRoot, "volumes", b.config.Project+"_"+volume, "_data")
}

// pilotImageID resolves the verified manifest's image digest to the
// operator-pinned loaded image ID. Helper containers only ever run this
// exact image; no tag or request-supplied reference is accepted.
func (b *DockerPilotBoundary) pilotImageID(p PreparedExecution) (string, error) {
	manifest, err := parseCanonicalObject(p.Manifest(), "manifest")
	if err != nil {
		return "", err
	}
	release, _ := objectValue(manifest["release"])
	images, err := stringObjects(release["images"], "manifest release images")
	if err != nil {
		return "", err
	}
	for _, image := range images {
		if image["service"] == "gitea" {
			digest, _ := image["digest"].(string)
			if id := b.config.ImageConfigDigests[digest]; id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("verified manifest image is not pinned by the operator")
}

// streamer moves volume bytes through a one-off helper container running
// the pinned image. In rootless Docker the volume files belong to the
// user-namespace-mapped application UID, which the agent process cannot
// read or write directly; the helper runs as that identity instead. The
// helper has no network, the approved resource limits, and a fixed tar
// command constructed here — never from request data.
func (b *DockerPilotBoundary) streamer(imageID string) pilotVolumeStreamer {
	return dockerVolumeStreamer{boundary: b, imageID: imageID}
}

type dockerVolumeStreamer struct {
	boundary *DockerPilotBoundary
	imageID  string
}

func (s dockerVolumeStreamer) StreamOut(ctx context.Context, volume string, w io.Writer) error {
	if err := validatePilotVolumeName(volume); err != nil {
		return err
	}
	name := s.boundary.config.Project + "_" + volume
	_, err := s.boundary.run(ctx, nil, w, "run", "--rm", "--network", "none",
		"--memory", "1073741824", "--cpus", "2", "--pull", "never",
		"--entrypoint", "/bin/tar", "-v", name+":/wazi-volume:ro",
		s.imageID, "-cf", "-", "-C", "/wazi-volume", ".")
	return err
}

func (s dockerVolumeStreamer) StreamIn(ctx context.Context, volume string, r io.Reader) error {
	if err := validatePilotVolumeName(volume); err != nil {
		return err
	}
	name := s.boundary.config.Project + "_" + volume
	// Extraction runs as namespace root so archived application ownership
	// is restored exactly; the fixed helper command clears the volume
	// first so files absent from the checkpoint do not survive a
	// restore. A second fixed helper call returns the volume root
	// itself to the application identity.
	if _, err := s.boundary.run(ctx, r, nil, "run", "--rm", "-i", "--network", "none",
		"--memory", "1073741824", "--cpus", "2", "--pull", "never", "--user", "0",
		"--entrypoint", "/bin/sh", "-v", name+":/wazi-volume",
		s.imageID, "-c", "find /wazi-volume -mindepth 1 -delete && tar -xf - -C /wazi-volume"); err != nil {
		return err
	}
	_, err := s.boundary.run(ctx, nil, nil, "run", "--rm", "--network", "none",
		"--memory", "1073741824", "--cpus", "2", "--pull", "never", "--user", "0",
		"--entrypoint", "/bin/chown", "-v", name+":/wazi-volume",
		s.imageID, "1000:1000", "/wazi-volume")
	return err
}

func (b *DockerPilotBoundary) backupVolume(ctx context.Context, compose []byte, params VolumeParameters, imageID string) error {
	if err := validatePilotVolume(params.Volume, params.BackupID); err != nil {
		return err
	}
	ids, err := b.run(ctx, nil, nil, "ps", "--filter", "label=com.docker.compose.project="+b.config.Project, "--filter", "status=running", "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	paused := len(strings.Fields(string(ids))) > 0
	if paused {
		if _, err := b.compose(ctx, compose, "pause", "gitea"); err != nil {
			return err
		}
		defer func() { _, _ = b.compose(ctx, compose, "unpause", "gitea") }()
	}
	return backupVolumeStream(ctx, b.streamer(imageID), filepath.Join(b.config.StateDirectory, "backups"), params.Volume, params.BackupID)
}
