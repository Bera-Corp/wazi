package agentclient

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// PackageInstanceConfig is reviewed operator policy. Requests cannot introduce
// a profile, project, endpoint, image mapping, budget or secret file.
type PackageInstanceConfig struct {
	Profile, BudgetDigest                         string
	ReleaseVersion, BudgetFile, QualificationFile string
	ApplicationStorageBudgetMb                    int64
	Runtime                                       DockerPilotConfig
	SecretFiles                                   map[string]string
}

type packageShape struct {
	App, Service, Image, Volume, Mount, Health string
	RAM, Storage, Port, ContainerPort          int
	CPU                                        string
	Caps                                       []any
}

func shapeFor(profile string, second bool) (packageShape, error) {
	offset := 0
	// Reuse preserves the signed port contract. A second instance is admitted
	// only after the first has stopped and released that same private port.
	_ = second
	switch profile {
	case "postgresql":
		return packageShape{"org.postgresql.postgresql", "postgresql", "ghcr.io/bera-corp/wazi-phase2-postgresql", "data", "/var/lib/postgresql/data", "postgresql://local/wazi", 384, 2048, 15432 + offset, 5432, "0.5", []any{"CHOWN", "FOWNER", "DAC_OVERRIDE", "SETUID", "SETGID"}}, nil
	case "mysql":
		return packageShape{"com.mysql.mysql", "mysql", "ghcr.io/bera-corp/wazi-phase2-mysql", "data", "/var/lib/mysql", "mysql://local/wazi", 768, 2048, 13306 + offset, 3306, "0.5", []any{"CHOWN", "FOWNER", "DAC_OVERRIDE", "SETUID", "SETGID"}}, nil
	case "nginx":
		return packageShape{"org.nginx.nginx", "nginx", "ghcr.io/bera-corp/wazi-phase2-nginx", "config", "/etc/nginx", "http://127.0.0.1:" + strconv.Itoa(18080+offset) + "/", 64, 128, 18080 + offset, 80, "0.25", []any{"SETUID", "SETGID", "NET_BIND_SERVICE"}}, nil
	}
	return packageShape{}, fmt.Errorf("package profile is not reviewed")
}

type DockerPackageBoundary struct {
	core   *DockerPilotBoundary
	config PackageInstanceConfig
	shape  packageShape
}

// RenderReviewedPackageCompose is operator tooling for the versioned renderer.
// It validates the same protected instance policy used by the executor.
func RenderReviewedPackageCompose(manifest []byte, config PackageInstanceConfig) ([]byte, error) {
	b, err := NewDockerPackageBoundary(config, []byte("{}"))
	if err != nil {
		return nil, err
	}
	return b.render(manifest)
}

func (b *DockerPackageBoundary) rendererInputsDigest() (string, error) {
	raw, err := canonicalPayload(map[string]any{"profile": b.config.Profile, "project": b.config.Runtime.Project, "images": b.config.Runtime.ImageConfigDigests, "secretFiles": b.config.SecretFiles})
	if err != nil {
		return "", err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return "", err
	}
	return verify.CanonicalDigest(value)
}

func NewDockerPackageBoundary(config PackageInstanceConfig, artifact []byte) (*DockerPackageBoundary, error) {
	project := config.Runtime.Project
	if project != "wazi-"+config.Profile+"-01" && project != "wazi-"+config.Profile+"-02" {
		return nil, fmt.Errorf("package instance is outside the reviewed project set")
	}
	shape, err := shapeFor(config.Profile, strings.HasSuffix(project, "-02"))
	if err != nil {
		return nil, err
	}
	r := config.Runtime
	if config.ApplicationStorageBudgetMb <= 0 || config.ApplicationStorageBudgetMb > 25600 {
		return nil, fmt.Errorf("package requires the approved aggregate application-storage reservation")
	}
	if r.TenantScope == "" || r.HostID == "" || r.RuntimeUID <= 0 ||
		r.Endpoint != "unix:///run/user/"+strconv.Itoa(r.RuntimeUID)+"/docker.sock" ||
		r.Home != "/srv/wazi-runtime/home" || r.DataRoot != "/srv/wazi-runtime/home/.local/share/docker" ||
		r.QuotaHelper != "/usr/local/libexec/wazi-phase2-quota" ||
		len(r.AllowedManifests) < 1 || len(r.AllowedManifests) > 2 || len(r.ImageConfigDigests) < 1 || len(r.ImageConfigDigests) > 2 ||
		!regexp.MustCompile("^sha256:[0-9a-f]{64}$").MatchString(config.BudgetDigest) {
		return nil, fmt.Errorf("package requires pinned rootless identity, releases and budget")
	}
	if filepath.Clean(r.StateDirectory) != "/srv/wazi-runtime/phase2/"+project {
		return nil, fmt.Errorf("package state directory mismatch")
	}
	for _, path := range config.SecretFiles {
		if filepath.Clean(path) != path || !strings.HasPrefix(path, "/etc/wazi-phase2/secrets/"+project+"/") || strings.ContainsAny(filepath.Base(path), "/\\") {
			return nil, fmt.Errorf("secret file is outside its operator-owned instance")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0027 != 0 {
			return nil, fmt.Errorf("secret file must be protected and regular")
		}
	}
	expectedSecrets := 0
	if config.Profile == "postgresql" {
		expectedSecrets = 2
	}
	if config.Profile == "mysql" {
		expectedSecrets = 3
	}
	if len(config.SecretFiles) != expectedSecrets {
		return nil, fmt.Errorf("package secret surface mismatch")
	}
	if _, err := parseCanonicalObject(artifact, "package compose artifact"); err != nil {
		return nil, err
	}
	// Defensive copies prevent mutable caller maps from changing admission.
	r.AllowedManifests = cloneBoolMap(r.AllowedManifests)
	r.ImageConfigDigests = cloneStringMap(r.ImageConfigDigests)
	config.Runtime = r
	config.SecretFiles = cloneStringMap(config.SecretFiles)
	core := &DockerPilotBoundary{config: r, artifact: append([]byte(nil), artifact...)}
	core.run = core.command
	return &DockerPackageBoundary{core: core, config: config, shape: shape}, nil
}

func cloneBoolMap(value map[string]bool) map[string]bool {
	copy := map[string]bool{}
	for k, v := range value {
		copy[k] = v
	}
	return copy
}
func cloneStringMap(value map[string]string) map[string]string {
	copy := map[string]string{}
	for k, v := range value {
		copy[k] = v
	}
	return copy
}
func (b *DockerPackageBoundary) VerifyRootless(ctx context.Context) error {
	if err := b.core.VerifyRootless(ctx); err != nil {
		return err
	}
	if err := b.verifyCapacity(ctx); err != nil {
		return err
	}
	return b.verifyPort(ctx)
}

func (b *DockerPackageBoundary) verifyPort(ctx context.Context) error {
	raw, err := b.core.run(ctx, nil, nil, "ps", "--filter", "status=running", "--filter", "label=com.docker.compose.project="+b.config.Runtime.Project, "--filter", "label=com.docker.compose.service="+b.shape.Service, "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	ids := strings.Fields(string(raw))
	if len(ids) > 1 {
		return fmt.Errorf("package instance has duplicate running services")
	}
	if len(ids) == 1 {
		return b.inspectLimits(ctx)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(b.shape.Port))
	if err != nil {
		return fmt.Errorf("package private port is occupied")
	}
	return listener.Close()
}

// RenderPackageCompose emits only the reviewed standalone surface. The initial
// package renderer has no commands, build contexts, public ports or host mounts.
// Secret mounts originate exclusively from protected operator configuration.
func (b *DockerPackageBoundary) render(manifestRaw []byte) ([]byte, error) {
	m, err := parseCanonicalObject(manifestRaw, "package manifest")
	if err != nil {
		return nil, err
	}
	if m["appId"] != b.shape.App {
		return nil, fmt.Errorf("manifest does not match reviewed package profile")
	}
	release, ok := objectValue(m["release"])
	if !ok {
		return nil, fmt.Errorf("package release missing")
	}
	images, err := stringObjects(release["images"], "images")
	if err != nil || len(images) != 1 || images[0]["service"] != b.shape.Service || images[0]["image"] != b.shape.Image {
		return nil, fmt.Errorf("package image surface mismatch")
	}
	digest, _ := images[0]["digest"].(string)
	image := b.config.Runtime.ImageConfigDigests[digest]
	if !regexp.MustCompile("^sha256:[0-9a-f]{64}$").MatchString(image) {
		return nil, fmt.Errorf("package image has no verified archive binding")
	}
	limits, err := stringObjects(release["resourceLimits"], "limits")
	if err != nil || len(limits) != 1 || limits[0]["service"] != b.shape.Service || limits[0]["cpuCores"] != json.Number(b.shape.CPU) || limits[0]["ramMb"] != json.Number(strconv.Itoa(b.shape.RAM)) {
		return nil, fmt.Errorf("package resource ceiling mismatch")
	}
	volumes, err := stringObjects(release["volumes"], "volumes")
	if err != nil || len(volumes) != 1 || volumes[0]["name"] != b.shape.Volume || volumes[0]["service"] != b.shape.Service || volumes[0]["mountPath"] != b.shape.Mount || volumes[0]["backup"] != true || volumes[0]["storageLimitMb"] != json.Number(strconv.Itoa(b.shape.Storage)) {
		return nil, fmt.Errorf("package managed volume mismatch")
	}
	ports, err := stringObjects(release["ports"], "ports")
	if err != nil || len(ports) != 1 || ports[0]["service"] != b.shape.Service || ports[0]["containerPort"] != json.Number(strconv.Itoa(b.shape.ContainerPort)) || ports[0]["hostPort"] != json.Number(strconv.Itoa(b.shape.Port)) || ports[0]["exposure"] != "loopback" {
		return nil, fmt.Errorf("package loopback port mismatch")
	}
	health, ok := objectValue(release["healthCheck"])
	healthType := map[string]string{"postgresql": "postgresql-authenticated", "mysql": "mysql-authenticated", "nginx": "nginx-config-and-http"}[b.config.Profile]
	if !ok || health["target"] != b.shape.Health || health["type"] != healthType {
		return nil, fmt.Errorf("package health target mismatch")
	}
	entries, err := optionalObjects(release["environment"], "environment")
	if err != nil {
		return nil, err
	}
	environment := map[string]any{}
	secrets := map[string]any{}
	serviceSecrets := []any{}
	expected := map[string]string{}
	if b.config.Profile == "postgresql" {
		expected = map[string]string{"POSTGRES_DB": "wazi", "POSTGRES_USER": "wazi", "POSTGRES_PASSWORD_FILE": "/run/secrets/password"}
	}
	if b.config.Profile == "mysql" {
		expected = map[string]string{"MYSQL_DATABASE": "wazi", "MYSQL_USER": "wazi", "MYSQL_PASSWORD_FILE": "/run/secrets/password", "MYSQL_ROOT_PASSWORD_FILE": "/run/secrets/root-password"}
	}
	if len(entries) != len(expected) {
		return nil, fmt.Errorf("package environment surface mismatch")
	}
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		if entry["service"] != b.shape.Service || expected[name] == "" || environment[name] != nil {
			return nil, fmt.Errorf("package environment field rejected")
		}
		if strings.HasSuffix(name, "_PASSWORD_FILE") {
			secretName := "password"
			if name == "MYSQL_ROOT_PASSWORD_FILE" {
				secretName = "root-password"
			}
			want := "secret://" + b.config.Profile + "/" + secretName
			if entry["secretRef"] != want || entry["default"] != nil {
				return nil, fmt.Errorf("package secret reference mismatch")
			}
		} else if entry["default"] != expected[name] || entry["secretRef"] != nil {
			return nil, fmt.Errorf("package environment default mismatch")
		}
		environment[name] = expected[name]
	}
	for _, name := range []string{"password", "root-password", "client"} {
		path := b.config.SecretFiles[name]
		if path == "" {
			if name == "root-password" && b.config.Profile == "postgresql" || b.config.Profile == "nginx" {
				continue
			}
			return nil, fmt.Errorf("package secret file missing")
		}
		target := name
		if name == "client" {
			target = "client.conf"
		}
		secrets[name] = map[string]any{"file": path}
		serviceSecrets = append(serviceSecrets, map[string]any{"source": name, "target": target})
	}
	service := map[string]any{"image": image, "pull_policy": "never", "cpus": json.Number(b.shape.CPU), "mem_limit": strconv.Itoa(b.shape.RAM*1024*1024) + "b", "pids_limit": json.Number("256"),
		"security_opt": []any{"no-new-privileges:true"}, "cap_drop": []any{"ALL"}, "cap_add": b.shape.Caps,
		"ports":       []any{"127.0.0.1:" + strconv.Itoa(b.shape.Port) + ":" + strconv.Itoa(b.shape.ContainerPort)},
		"volumes":     []any{map[string]any{"type": "volume", "source": b.shape.Volume, "target": b.shape.Mount}},
		"environment": environment, "labels": map[string]any{"dev.wazi.profile": b.config.Profile, "dev.wazi.tenant": b.config.Runtime.TenantScope},
		"logging": map[string]any{"driver": "json-file", "options": map[string]any{"max-size": "10m", "max-file": "2"}}}
	if len(serviceSecrets) > 0 {
		service["secrets"] = serviceSecrets
	}
	if b.config.Profile == "nginx" {
		service["entrypoint"] = []any{"nginx"}
		service["command"] = []any{"-g", "daemon off;"}
	}
	output := map[string]any{"services": map[string]any{b.shape.Service: service}, "volumes": map[string]any{b.shape.Volume: map[string]any{"external": true, "name": b.config.Runtime.Project + "_" + b.shape.Volume}}}
	if len(secrets) > 0 {
		output["secrets"] = secrets
	}
	return verify.CanonicalJSON(output)
}

func (b *DockerPackageBoundary) bound(p PreparedExecution) ([]byte, error) {
	if p.TenantScope() != b.config.Runtime.TenantScope || p.HostID() != b.config.Runtime.HostID || !b.config.Runtime.AllowedManifests[p.Binding().Release.ManifestDigest] {
		return nil, fmt.Errorf("package identity or release not admitted")
	}
	artifact, err := parseCanonicalObject(b.core.artifact, "package artifact")
	if err != nil {
		return nil, err
	}
	envelope, err := parseCanonicalObject(p.Envelope(), "package envelope")
	if err != nil {
		return nil, err
	}
	ref, ok := objectValue(envelope["composeArtifactRef"])
	digest, err := verify.CanonicalDigest(artifact)
	renderer, rendererOK := objectValue(artifact["renderer"])
	inputsDigest, inputErr := b.rendererInputsDigest()
	if err != nil || !ok || ref["recordId"] != artifact["artifactId"] || ref["digest"] != digest || artifact["manifestDigest"] != p.Binding().Release.ManifestDigest || artifact["projectName"] != b.config.Runtime.Project ||
		!rendererOK || renderer["name"] != "wazi-compose-renderer" || renderer["version"] != "0.5.0" || inputErr != nil || renderer["inputsDigest"] != inputsDigest {
		return nil, fmt.Errorf("package artifact binding mismatch")
	}
	raw, err := b.render(p.Manifest())
	if err != nil {
		return nil, err
	}
	if artifact["composeDigest"] != fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) {
		return nil, fmt.Errorf("package compose digest mismatch")
	}
	return raw, nil
}

func (b *DockerPackageBoundary) ValidateContext(p PreparedExecution) error {
	if _, err := b.bound(p); err != nil {
		return err
	}
	plan, _, err := decodePrepared(p)
	if err != nil {
		return err
	}
	if plan.Operation != "install" {
		if err := b.checkReleaseAnchor(p, false); err != nil {
			return err
		}
	}
	for _, step := range plan.Steps {
		switch step.Capability {
		case "manifest.verify", "compose.pull", "compose.up", "compose.down", "compose.ps", "health.check", "volume.backup", "volume.restore":
		default:
			return fmt.Errorf("package capability has no reviewed adapter")
		}
	}
	return nil
}
func (b *DockerPackageBoundary) ValidatePrepared(p PreparedExecution) error {
	if !p.Verified() {
		return fmt.Errorf("package execution requires approvals")
	}
	return b.ValidateContext(p)
}

func (b *DockerPackageBoundary) container(ctx context.Context) (string, error) {
	raw, err := b.core.run(ctx, nil, nil, "ps", "--all", "--filter", "label=com.docker.compose.project="+b.config.Runtime.Project, "--filter", "label=com.docker.compose.service="+b.shape.Service, "--format", "{{.ID}}")
	values := strings.Fields(string(raw))
	if err != nil || len(values) != 1 {
		return "", fmt.Errorf("package must have exactly one owned service container")
	}
	return values[0], nil
}

func (b *DockerPackageBoundary) client(ctx context.Context, program string, input io.Reader, output io.Writer, extra ...string) ([]byte, error) {
	container, err := b.container(ctx)
	if err != nil {
		return nil, err
	}
	args := []string{"exec", "-i"}
	if b.config.Profile == "postgresql" {
		args = append(args, "--env", "PGSERVICEFILE=/run/secrets/client.conf")
	}
	args = append(args, container, program)
	args = append(args, extra...)
	raw, runErr := b.core.run(ctx, input, output, args...)
	if runErr != nil {
		return nil, fmt.Errorf("reviewed package client command failed")
	}
	return raw, nil
}

func (b *DockerPackageBoundary) readiness(ctx context.Context) error {
	switch b.config.Profile {
	case "postgresql":
		raw, err := b.client(ctx, "psql", nil, nil, "--dbname=service=wazi", "--no-password", "--tuples-only", "--no-align", "--command", "SELECT 1")
		if err != nil || strings.TrimSpace(string(raw)) != "1" {
			return fmt.Errorf("authenticated PostgreSQL readiness failed")
		}
	case "mysql":
		raw, err := b.client(ctx, "mysql", nil, nil, "--defaults-extra-file=/run/secrets/client.conf", "--batch", "--skip-column-names", "--execute", "SELECT 1")
		if err != nil || strings.TrimSpace(string(raw)) != "1" {
			return fmt.Errorf("authenticated MySQL readiness failed")
		}
	case "nginx":
		if _, err := b.client(ctx, "nginx", nil, nil, "-t"); err != nil {
			return fmt.Errorf("nginx configuration validation failed")
		}
		return waitForHealth(ctx, b.shape.Health, healthTimeout, healthPollInterval)
	}
	return nil
}

func (b *DockerPackageBoundary) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	for {
		if err := b.readiness(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("package readiness deadline exceeded")
		case <-time.After(time.Second):
		}
	}
}

func (b *DockerPackageBoundary) Execute(ctx context.Context, p PreparedExecution, step Step) (Outcome, error) {
	success := Outcome{State: "succeeded"}
	if err := b.ValidatePrepared(p); err != nil {
		return Outcome{}, err
	}
	compose, err := b.bound(p)
	if err != nil {
		return Outcome{}, err
	}
	if err := b.VerifyRootless(ctx); err != nil {
		return Outcome{}, err
	}
	switch step.Capability {
	case "manifest.verify":
		return success, nil
	case "compose.pull":
		id := b.config.Runtime.ImageConfigDigests[step.Parameters.ComposePull.ImageDigest]
		if id == "" {
			return Outcome{}, fmt.Errorf("package image not admitted")
		}
		raw, err := b.core.run(ctx, nil, nil, "image", "inspect", id, "--format", "{{.Id}}")
		if err != nil || strings.TrimSpace(string(raw)) != id {
			return Outcome{}, fmt.Errorf("package attested image is not loaded")
		}
		return success, nil
	case "compose.up":
		if step.Parameters.ComposeUp.ProjectName != b.config.Runtime.Project {
			return Outcome{}, fmt.Errorf("package project mismatch")
		}
		plan, _, err := decodePrepared(p)
		if err != nil {
			return Outcome{}, err
		}
		if plan.Operation == "install" {
			raw, err := b.core.run(ctx, nil, nil, "volume", "ls", "--format", "{{.Name}}")
			if err != nil {
				return Outcome{}, err
			}
			for _, name := range strings.Fields(string(raw)) {
				if name == b.config.Runtime.Project+"_"+b.shape.Volume {
					return Outcome{}, fmt.Errorf("fresh package install requires absent instance volumes")
				}
			}
		}
		if err := b.core.ensureVolumes(ctx, p); err != nil {
			return Outcome{}, err
		}
		if err := b.checkReleaseAnchor(p, true); err != nil {
			return Outcome{}, err
		}
		if b.config.Profile == "nginx" && plan.Operation == "install" {
			if err := b.initializeNginx(); err != nil {
				return Outcome{}, err
			}
		}
		_, err = b.core.compose(ctx, compose, "up", "--detach", "--no-build", "--pull", "never", b.shape.Service)
		if err == nil {
			err = b.inspectLimits(ctx)
		}
		return success, err
	case "compose.down":
		if step.Parameters.ComposeDown.ProjectName != b.config.Runtime.Project || step.Parameters.ComposeDown.RemoveVolumes {
			return Outcome{}, fmt.Errorf("foreign or destructive package request")
		}
		_, err = b.core.compose(ctx, compose, "down", "--timeout", "30")
		return success, err
	case "compose.ps":
		_, err = b.core.compose(ctx, compose, "ps", "--all", "--format", "json")
		return success, err
	case "health.check":
		if step.Parameters.HealthCheck.Target != b.shape.Health {
			return Outcome{}, fmt.Errorf("unapproved package readiness target")
		}
		return success, b.waitReady(ctx)
	case "volume.backup":
		if step.Parameters.VolumeBackup.Volume != b.shape.Volume {
			return Outcome{}, fmt.Errorf("package backup volume mismatch")
		}
		return success, b.backup(ctx, p, step.Parameters.VolumeBackup.BackupID, "backups")
	case "volume.restore":
		if step.Parameters.VolumeRestore.Volume != b.shape.Volume {
			return Outcome{}, fmt.Errorf("package restore volume mismatch")
		}
		return success, b.restore(ctx, p, step.Parameters.VolumeRestore.BackupID, compose)
	}
	return Outcome{}, fmt.Errorf("package adapter unavailable")
}

func (b *DockerPackageBoundary) inspectLimits(ctx context.Context) error {
	container, err := b.container(ctx)
	if err != nil {
		return err
	}
	raw, err := b.core.run(ctx, nil, nil, "inspect", container, "--format", "{{json .HostConfig}}")
	if err != nil {
		return err
	}
	var value struct {
		Memory, NanoCpus int64
		Privileged       bool
		NetworkMode      string
		CapAdd, CapDrop  []string
		PortBindings     map[string][]struct{ HostIP, HostPort string }
	}
	if json.Unmarshal(raw, &value) != nil || value.Memory != int64(b.shape.RAM)*1024*1024 || value.Privileged || value.NetworkMode == "host" || len(value.CapDrop) != 1 || value.CapDrop[0] != "ALL" || len(value.CapAdd) != len(b.shape.Caps) {
		return fmt.Errorf("package container escaped resource or capability profile")
	}
	cpu, _ := strconv.ParseFloat(b.shape.CPU, 64)
	if value.NanoCpus != int64(cpu*1e9) {
		return fmt.Errorf("package CPU limit mismatch")
	}
	ports := value.PortBindings[strconv.Itoa(b.shape.ContainerPort)+"/tcp"]
	if len(value.PortBindings) != 1 || len(ports) != 1 || ports[0].HostIP != "127.0.0.1" || ports[0].HostPort != strconv.Itoa(b.shape.Port) {
		return fmt.Errorf("package port escaped its private binding")
	}
	expected := map[string]bool{}
	for _, name := range b.shape.Caps {
		expected[name.(string)] = true
	}
	for _, name := range value.CapAdd {
		if !expected[strings.TrimPrefix(name, "CAP_")] {
			return fmt.Errorf("unexpected package capability")
		}
	}
	return nil
}

func (b *DockerPackageBoundary) initializeNginx() error {
	root := b.core.volumeDir(b.shape.Volume)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("nginx configuration volume unavailable")
	}
	// Fixed configuration, no envsubst, uploaded scripts or arbitrary directives.
	main := "worker_processes 1;\npid /tmp/nginx.pid;\nerror_log /dev/stderr warn;\nevents { worker_connections 256; }\nhttp { access_log /dev/stdout; include /etc/nginx/conf.d/*.conf; }\n"
	site := "server { listen 80; server_name _; location / { default_type text/plain; return 200 'Wazi Nginx ready\\n'; } }\n"
	if err := os.Mkdir(filepath.Join(root, "conf.d"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "nginx.conf"), []byte(main), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "conf.d", "default.conf"), []byte(site), 0644)
}
