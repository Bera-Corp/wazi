package agentclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func packageRenderFixture(t *testing.T, profile string) (*DockerPackageBoundary, map[string]any) {
	t.Helper()
	shape, err := shapeFor(profile, false)
	if err != nil {
		t.Fatal(err)
	}
	project := "wazi-" + profile + "-01"
	m := parseReleaseObject(t, readReleaseFixture(t, "app-manifest.json"))
	m["appId"] = shape.App
	release := m["release"].(map[string]any)
	digest := "sha256:" + strings.Repeat("a", 64)
	release["images"] = []any{map[string]any{"service": shape.Service, "image": shape.Image, "digest": digest}}
	release["resourceLimits"] = []any{map[string]any{"service": shape.Service, "cpuCores": json.Number(shape.CPU), "ramMb": json.Number(fmt.Sprint(shape.RAM))}}
	release["volumes"] = []any{map[string]any{"service": shape.Service, "name": shape.Volume, "mountPath": shape.Mount, "backup": true, "storageLimitMb": json.Number(fmt.Sprint(shape.Storage))}}
	release["ports"] = []any{map[string]any{"service": shape.Service, "hostPort": json.Number(fmt.Sprint(shape.Port)), "containerPort": json.Number(fmt.Sprint(shape.ContainerPort)), "exposure": "loopback"}}
	release["healthCheck"] = map[string]any{"type": map[string]string{"postgresql": "postgresql-authenticated", "mysql": "mysql-authenticated", "nginx": "nginx-config-and-http"}[profile], "target": shape.Health}
	entries := []any{}
	secrets := map[string]string{}
	if profile != "nginx" {
		prefix := "POSTGRES"
		if profile == "mysql" {
			prefix = "MYSQL"
		}
		for _, item := range []struct{ name, value string }{{prefix + "_USER", "wazi"}, {prefix + "_DB", "wazi"}} {
			name := item.name
			if profile == "mysql" && name == "MYSQL_DB" {
				name = "MYSQL_DATABASE"
			}
			entries = append(entries, map[string]any{"service": shape.Service, "name": name, "default": item.value})
		}
		entries = append(entries, map[string]any{"service": shape.Service, "name": prefix + "_PASSWORD_FILE", "secretRef": "secret://" + profile + "/password"})
		secrets["password"] = "/etc/wazi-phase2/secrets/" + project + "/password"
		secrets["client"] = "/etc/wazi-phase2/secrets/" + project + "/client"
		if profile == "mysql" {
			entries = append(entries, map[string]any{"service": shape.Service, "name": "MYSQL_ROOT_PASSWORD_FILE", "secretRef": "secret://" + profile + "/root-password"})
			secrets["root-password"] = "/etc/wazi-phase2/secrets/" + project + "/root-password"
		}
	}
	release["environment"] = entries
	config := PackageInstanceConfig{Profile: profile, Runtime: DockerPilotConfig{Project: project, ImageConfigDigests: map[string]string{digest: "sha256:" + strings.Repeat("b", 64)}}, SecretFiles: secrets}
	return &DockerPackageBoundary{config: config, shape: shape}, m
}

func TestReviewedPackageRendererIsClosedAndDeterministic(t *testing.T) {
	for _, profile := range []string{"postgresql", "mysql", "nginx"} {
		t.Run(profile, func(t *testing.T) {
			b, manifest := packageRenderFixture(t, profile)
			raw := canonical(t, manifest)
			first, err := b.render(raw)
			if err != nil {
				t.Fatal(err)
			}
			second, err := b.render(raw)
			if err != nil || string(first) != string(second) {
				t.Fatal("package renderer is not deterministic")
			}
			var output map[string]any
			if err := json.Unmarshal(first, &output); err != nil {
				t.Fatal(err)
			}
			service := output["services"].(map[string]any)[b.shape.Service].(map[string]any)
			if service["privileged"] != nil || service["network_mode"] != nil || service["pull_policy"] != "never" {
				t.Fatal("package escaped closed runtime")
			}
			for _, attack := range []string{"port", "cpu", "volume", "image", "environment"} {
				changed := parseReleaseObject(t, raw)
				release := changed["release"].(map[string]any)
				switch attack {
				case "port":
					release["ports"].([]any)[0].(map[string]any)["exposure"] = "public"
				case "cpu":
					release["resourceLimits"].([]any)[0].(map[string]any)["cpuCores"] = json.Number("8")
				case "volume":
					release["volumes"].([]any)[0].(map[string]any)["mountPath"] = "/etc"
				case "image":
					release["images"].([]any)[0].(map[string]any)["digest"] = "sha256:" + strings.Repeat("f", 64)
				case "environment":
					release["environment"] = []any{map[string]any{"service": b.shape.Service, "name": "UNREVIEWED", "default": "value"}}
				}
				if _, err := b.render(canonical(t, changed)); err == nil {
					t.Fatalf("unsafe %s accepted", attack)
				}
			}
		})
	}
}

func TestPackageCheckpointBindsScopeAndRejectsCorruptionBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX checkpoint permissions and directory fsync are qualified natively on Linux")
	}
	b, _ := packageRenderFixture(t, "postgresql")
	state := t.TempDir()
	b.config.Runtime.StateDirectory = state
	b.config.Runtime.TenantScope = "tenant_executor"
	b.core = &DockerPilotBoundary{config: b.config.Runtime}
	calls := []string{}
	b.core.run = func(_ context.Context, input io.Reader, output io.Writer, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "ps" {
			return []byte("container-id\n"), nil
		}
		if output != nil {
			_, err := io.WriteString(output, "database-dump-bytes")
			return nil, err
		}
		if input != nil {
			_, err := io.ReadAll(input)
			return nil, err
		}
		return nil, nil
	}
	p := executorPrepared(t, "health.check", "{\"target\":\"http://localhost:3000/api/healthz\"}")
	if err := b.backup(context.Background(), p, "bak_package_test", "backups"); err != nil {
		t.Fatal(err)
	}
	file, _, err := b.verifiedCheckpoint(p, "bak_package_test")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	before := len(calls)
	if err := os.WriteFile(filepath.Join(state, "backups", "bak_package_test.dump"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.restore(context.Background(), p, "bak_package_test", nil); err == nil {
		t.Fatal("corrupt checkpoint accepted")
	}
	if len(calls) != before {
		t.Fatal("mutation attempted before checkpoint validation")
	}
	for _, call := range calls {
		if strings.Contains(call, "PGPASSWORD=") || strings.Contains(call, "--password=") || strings.Contains(call, "/bin/sh") {
			t.Fatal("database adapter exposed a secret or shell")
		}
	}
}

func TestNginxCheckpointRejectsEscapingPathsAndNonRegularEntries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../escape", "/absolute", "a\\b"} {
		if err := writeNginxConfig(root, map[string]string{"nginx.conf": "events {}", "" + name: "data"}); err == nil {
			t.Fatalf("unsafe config path %s accepted", name)
		}
	}
	if err := writeNginxConfig(root, map[string]string{"nginx.conf": "events {}", "conf.d/default.conf": "server {}"}); err != nil {
		t.Fatal(err)
	}
}

func TestPackageCapacityIncludesNeighboursAndHelperReserve(t *testing.T) {
	shape, _ := shapeFor("mysql", false)
	existing := []runningPackageResource{{Project: "wazi-gitea-phase1", Memory: 1024 * 1024 * 1024, NanoCPUs: 2e9}, {Project: "wazi-postgresql-01", Memory: 384 * 1024 * 1024, NanoCPUs: 5e8}, {Project: "wazi-nginx-01", Memory: 64 * 1024 * 1024, NanoCPUs: 25e7}}
	if err := packageCapacityCheck(3072*1024*1024, 4, existing, "wazi-mysql-01", shape); err != nil {
		t.Fatal(err)
	}
	for _, attack := range []string{"memory", "cpu", "unbounded", "extra-instance"} {
		memory, cpu := int64(3072*1024*1024), 4.0
		neighbours := append([]runningPackageResource(nil), existing...)
		switch attack {
		case "memory":
			memory = 2048 * 1024 * 1024
		case "cpu":
			cpu = 3.5
		case "unbounded":
			neighbours[0].Memory = 0
		case "extra-instance":
			neighbours = append(neighbours, runningPackageResource{Project: "other", Memory: 768 * 1024 * 1024, NanoCPUs: 5e8})
		}
		if err := packageCapacityCheck(memory, cpu, neighbours, "wazi-mysql-01", shape); err == nil {
			t.Fatalf("%s admitted", attack)
		}
	}
}

func TestPackageRendererBindsInstanceInputs(t *testing.T) {
	b, _ := packageRenderFixture(t, "postgresql")
	first, err := b.rendererInputsDigest()
	if err != nil {
		t.Fatal(err)
	}
	b.config.Runtime.Project = "wazi-postgresql-02"
	second, err := b.rendererInputsDigest()
	if err != nil || first == second {
		t.Fatal("instance inputs were not bound")
	}
	b.config.SecretFiles["client"] = "/etc/wazi-phase2/secrets/wazi-postgresql-02/client"
	third, err := b.rendererInputsDigest()
	if err != nil || second == third {
		t.Fatal("secret reference path was not bound")
	}
}
