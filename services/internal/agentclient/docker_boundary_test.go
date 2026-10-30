package agentclient

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
