package agentclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTransitionPolicyRequiresAdmittedReleasesAndQualification(t *testing.T) {
	a := "sha256:" + strings.Repeat("a", 64)
	b := "sha256:" + strings.Repeat("b", 64)
	config := PackageInstanceConfig{Profile: "uptime-kuma", Runtime: DockerPilotConfig{AllowedManifests: map[string]bool{a: true, b: true}}, Transitions: []PackageTransition{{FromManifestDigest: a, ToManifestDigest: b, ProcedureDigest: a, QualificationDigest: b, Operation: "upgrade", Mode: "uptime-kuma-startup"}}}
	if err := validatePackageTransitions(config); err != nil {
		t.Fatal(err)
	}
	delete(config.Runtime.AllowedManifests, a)
	if err := validatePackageTransitions(config); err == nil {
		t.Fatal("unadmitted source release accepted")
	}
	config.Runtime.AllowedManifests[a] = true
	config.Transitions[0].QualificationDigest = ""
	if err := validatePackageTransitions(config); err == nil {
		t.Fatal("missing qualification accepted")
	}
}

func TestPendingTransitionSurvivesReopenAndRejectsAnotherPlan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory durability qualified on Linux")
	}
	b := &DockerPackageBoundary{config: PackageInstanceConfig{Profile: "uptime-kuma", Runtime: DockerPilotConfig{Project: "wazi-uptime-kuma-01", StateDirectory: t.TempDir()}}}
	from := map[string]string{"profile": "uptime-kuma", "project": b.config.Runtime.Project, "manifestDigest": "sha256:" + strings.Repeat("a", 64), "imageConfig": "sha256:" + strings.Repeat("b", 64)}
	to := map[string]string{"profile": "uptime-kuma", "project": b.config.Runtime.Project, "manifestDigest": "sha256:" + strings.Repeat("c", 64), "imageConfig": "sha256:" + strings.Repeat("d", 64)}
	if err := b.writePackageState("instance-release.json", from); err != nil {
		t.Fatal(err)
	}
	intent := packageTransitionIntent{From: from, To: to, PlanDigest: "sha256:" + strings.Repeat("e", 64), CheckpointID: "bak_old"}
	if err := b.writePackageState("instance-transition.json", intent); err != nil {
		t.Fatal(err)
	}
	reopened := &DockerPackageBoundary{config: b.config}
	effective, pending, err := reopened.readPackageAnchor()
	if err != nil || pending == nil || effective["manifestDigest"] != to["manifestDigest"] {
		t.Fatalf("intent lost: %v", err)
	}
	if err := reopened.finishPackageTransition(PreparedExecution{}); err == nil {
		t.Fatal("unrelated plan committed migration")
	}
	raw, err := os.ReadFile(filepath.Join(b.config.Runtime.StateDirectory, "instance-release.json"))
	if err != nil {
		t.Fatal(err)
	}
	var unchanged map[string]string
	if json.Unmarshal(raw, &unchanged) != nil || unchanged["manifestDigest"] != from["manifestDigest"] {
		t.Fatal("rejected finalization changed the committed release")
	}
}
