package agentclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestReadOnlyContextChecksRuntimeWithoutAuthorizingExecution(t *testing.T) {
	manifest, err := os.ReadFile(filepath.Join("..", "..", "..", "releases", "phase1", "gitea", "28.1.0", "pilot2", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := verify.ParseJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := verify.CanonicalDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	mapping := map[string]string{"sha256:afd05701641f238dc198f98d7733b1d3b09cb6ae1a3f1cd9b3bc2058e706570b": "sha256:07a1cbdc452fe6ba4984b8733874c4ff729a29a5c318c073911c0fc5a39e2bae"}
	compose, err := RenderPilotCompose(manifest, mapping)
	if err != nil {
		t.Fatal(err)
	}
	artifact := map[string]any{"artifactId": "art_executor01", "manifestDigest": digest, "projectName": "wazi-gitea-phase1", "renderer": map[string]any{"name": "wazi-compose-renderer", "version": "0.4.0"}, "composeDigest": fmt.Sprintf("sha256:%x", sha256.Sum256(compose))}
	artifactDigest, err := verify.CanonicalDigest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	p := executorPrepared(t, "health.check", "{\"target\":\"http://127.0.0.1:13000/api/healthz\"}")
	p.valid = false
	p.release.manifestRaw = manifest
	p.binding.Release.ManifestDigest = digest
	p.envelopeRaw = canonical(t, map[string]any{"envelopeId": "env_executor01", "composeArtifactRef": map[string]any{"recordId": "art_executor01", "digest": artifactDigest}})
	b := &DockerPilotBoundary{config: DockerPilotConfig{TenantScope: p.TenantScope(), HostID: p.HostID(), Project: "wazi-gitea-phase1", AllowedManifests: map[string]bool{digest: true}, ImageConfigDigests: mapping}, artifact: canonical(t, artifact)}
	if err := b.ValidateContext(p); err != nil {
		t.Fatal(err)
	}
	if err := b.ValidatePrepared(p); err == nil {
		t.Fatal("read-only context became executable")
	}
	b.config.AllowedManifests[digest] = false
	if err := b.ValidateContext(p); err == nil {
		t.Fatal("runtime-unapproved manifest accepted")
	}
}

func TestPilotRendererClosesExecutableSurfaceAndPinsArchiveImageID(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "releases", "phase1", "gitea", "28.1.0", "pilot2", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	mapping := map[string]string{"sha256:afd05701641f238dc198f98d7733b1d3b09cb6ae1a3f1cd9b3bc2058e706570b": "sha256:07a1cbdc452fe6ba4984b8733874c4ff729a29a5c318c073911c0fc5a39e2bae"}
	compose, err := RenderPilotCompose(raw, mapping)
	if err != nil {
		t.Fatal(err)
	}
	again, err := RenderPilotCompose(raw, mapping)
	if err != nil || !bytes.Equal(compose, again) {
		t.Fatal("renderer is not deterministic")
	}
	var value map[string]any
	if err := json.Unmarshal(compose, &value); err != nil {
		t.Fatal(err)
	}
	service := value["services"].(map[string]any)["gitea"].(map[string]any)
	if service["image"] != mapping["sha256:afd05701641f238dc198f98d7733b1d3b09cb6ae1a3f1cd9b3bc2058e706570b"] || service["pull_policy"] != "never" || service["mem_limit"] != "1073741824b" || service["privileged"] != nil || service["command"] != nil {
		t.Fatal("renderer escaped closed execution policy")
	}
	for _, name := range []string{"unknown image", "foreign mount", "excess cpu", "public port", "secret environment"} {
		t.Run(name, func(t *testing.T) {
			m := jsonObject(t, string(raw))
			r := m["release"].(map[string]any)
			switch name {
			case "unknown image":
				r["images"].([]any)[0].(map[string]any)["digest"] = "sha256:unknown"
			case "foreign mount":
				r["volumes"].([]any)[0].(map[string]any)["mountPath"] = "/etc"
			case "excess cpu":
				r["resourceLimits"].([]any)[0].(map[string]any)["cpuCores"] = json.Number("8")
			case "public port":
				r["ports"].([]any)[0].(map[string]any)["exposure"] = "public"
			case "secret environment":
				r["environment"].([]any)[0].(map[string]any)["secretRef"] = "secret://pilot/value"
			}
			if _, err := RenderPilotCompose(canonical(t, m), mapping); err == nil {
				t.Fatal("unsafe pilot render accepted")
			}
		})
	}
}
