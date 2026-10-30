package agentclient

import (
	"crypto/ecdsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestVerifyExecutionBindingAcceptsExactSignedPlanGraph(t *testing.T) {
	fixture := newExecutionBindingFixture(t)
	result, err := VerifyExecutionBinding(
		fixture.manifest, fixture.procedure, fixture.rollback, fixture.envelope, fixture.plan,
		fixture.policy, fixture.expected,
	)
	if err != nil {
		t.Fatalf("valid execution binding rejected: %v", err)
	}
	if result.Release.ManifestDigest == "" || result.Release.ProcedureDigest == "" ||
		result.EnvelopeDigest == "" || result.PlanDigest == "" || result.RollbackProcedureDigest == "" {
		t.Fatalf("execution binding returned incomplete digests: %+v", result)
	}
}

func TestVerifyExecutionBindingRejectsMismatchedContextAndPlan(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*executionBindingFixture)
	}{
		{
			name: "wrong authenticated tenant",
			mutate: func(f *executionBindingFixture) {
				f.expected.TenantScope = "tenant_other"
			},
		},
		{
			name: "procedure reference digest mismatch",
			mutate: func(f *executionBindingFixture) {
				f.envelope = mutateCanonicalObject(t, f.envelope, func(record map[string]any) {
					ref := record["procedureRef"].(map[string]any)
					ref["digest"] = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				})
			},
		},
		{
			name: "policy reference digest mismatch",
			mutate: func(f *executionBindingFixture) {
				f.envelope = mutateCanonicalObject(t, f.envelope, func(record map[string]any) {
					ref := record["policyBundleRef"].(map[string]any)
					ref["digest"] = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				})
			},
		},
		{
			name: "envelope capability denied by authenticated policy",
			mutate: func(f *executionBindingFixture) {
				f.expected.Policy.DeniedCapabilities = []string{"compose.up"}
			},
		},
		{
			name: "plan operation mismatch",
			mutate: func(f *executionBindingFixture) {
				f.plan = mutateCanonicalObject(t, f.plan, func(record map[string]any) {
					record["operation"] = "upgrade"
				})
			},
		},
		{
			name: "capability order mismatch",
			mutate: func(f *executionBindingFixture) {
				f.plan = mutateCanonicalObject(t, f.plan, func(record map[string]any) {
					steps := record["steps"].([]any)
					steps[1], steps[2] = steps[2], steps[1]
				})
			},
		},
		{
			name: "fixed parameter mismatch",
			mutate: func(f *executionBindingFixture) {
				f.plan = mutateCanonicalObject(t, f.plan, func(record map[string]any) {
					steps := record["steps"].([]any)
					last := steps[len(steps)-1].(map[string]any)
					last["parameters"].(map[string]any)["expectedStatus"] = json.Number("201")
				})
			},
		},
		{
			name: "wrong compensation capability",
			mutate: func(f *executionBindingFixture) {
				f.plan = mutateCanonicalObject(t, f.plan, func(record map[string]any) {
					step := findPlanStep(t, record["steps"].([]any), "step_proxy")
					step["rollbackStepId"] = "step_down"
				})
			},
		},
		{
			name: "compensation resource mismatch",
			mutate: func(f *executionBindingFixture) {
				f.plan = mutateCanonicalObject(t, f.plan, func(record map[string]any) {
					step := findPlanStep(t, record["steps"].([]any), "step_proxy")
					step["parameters"].(map[string]any)["hostname"] = "other.example.com"
				})
			},
		},
		{
			name: "capability removed from envelope allowlist",
			mutate: func(f *executionBindingFixture) {
				f.envelope = mutateCanonicalObject(t, f.envelope, func(record map[string]any) {
					caps := record["allowedCapabilities"].([]any)
					record["allowedCapabilities"] = caps[:len(caps)-1]
				})
			},
		},
		{
			name: "plan approval requirement mismatch",
			mutate: func(f *executionBindingFixture) {
				f.plan = mutateCanonicalObject(t, f.plan, func(record map[string]any) {
					record["requiredApprovals"] = []any{}
				})
			},
		},
		{
			name: "missing separately signed rollback procedure",
			mutate: func(f *executionBindingFixture) {
				f.rollback = nil
			},
		},
		{
			name: "wrong rollback procedure",
			mutate: func(f *executionBindingFixture) {
				f.rollback = f.procedure
			},
		},
		{
			name: "expired envelope",
			mutate: func(f *executionBindingFixture) {
				f.expected.Now = time.Date(2026, time.October, 22, 0, 0, 0, 0, time.UTC)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newExecutionBindingFixture(t)
			test.mutate(&fixture)
			if _, err := VerifyExecutionBinding(
				fixture.manifest, fixture.procedure, fixture.rollback, fixture.envelope, fixture.plan,
				fixture.policy, fixture.expected,
			); err == nil {
				t.Fatal("mismatched execution binding was accepted")
			}
		})
	}
}

type executionBindingFixture struct {
	manifest   []byte
	procedure  []byte
	rollback   []byte
	envelope   []byte
	plan       []byte
	policy     verify.Policy
	signingKey *ecdsa.PrivateKey
	expected   ExecutionBindingExpectation
}

func newExecutionBindingFixture(t *testing.T) executionBindingFixture {
	t.Helper()
	policy, key := newReleaseTestSigner(t)
	manifestRecord := parseReleaseObject(t, readReleaseFixture(t, "app-manifest.json"))
	release := manifestRecord["release"].(map[string]any)
	release["resourceLimits"] = []any{
		map[string]any{"service": "app", "cpuCores": json.Number("1.5"), "ramMb": json.Number("1024")},
	}
	volumes := release["volumes"].([]any)
	volumes[0].(map[string]any)["storageLimitMb"] = json.Number("16384")
	manifest := signReleaseValue(t, manifestRecord, key)
	procedure := signReleaseFixture(t, readReleaseFixture(t, "procedure-install.json"), key)
	rollback := signReleaseFixture(t, readReleaseFixture(t, "procedure-rollback.json"), key)
	manifestRecord = parseReleaseObject(t, manifest)
	procedureRecord := parseReleaseObject(t, procedure)
	rollbackRecord := parseReleaseObject(t, rollback)
	policyBundleRecord := parseReleaseObject(t, readReleaseFixture(t, "policy-bundle.json"))
	manifestDigest, err := verify.CanonicalDigest(manifestRecord)
	if err != nil {
		t.Fatal(err)
	}
	procedureDigest, err := verify.CanonicalDigest(procedureRecord)
	if err != nil {
		t.Fatal(err)
	}
	rollbackDigest, err := verify.CanonicalDigest(rollbackRecord)
	if err != nil {
		t.Fatal(err)
	}
	policyBundleDigest, err := verify.CanonicalDigest(policyBundleRecord)
	if err != nil {
		t.Fatal(err)
	}
	envelope := mutateCanonicalObject(t, readReleaseFixture(t, "execution-context-envelope.json"), func(record map[string]any) {
		record["manifestDigest"] = manifestDigest
		record["manifestReviewState"] = manifestRecord["reviewState"]
		record["policyBundleRef"] = map[string]any{
			"recordId": policyBundleRecord["policyId"],
			"digest":   policyBundleDigest,
			"version":  policyBundleRecord["version"],
		}
		record["procedureRef"] = map[string]any{
			"recordId": procedureRecord["procedureId"],
			"digest":   procedureDigest,
			"version":  procedureRecord["version"],
		}
		strategy := record["rollbackStrategy"].(map[string]any)
		strategy["procedureRef"] = map[string]any{
			"recordId": rollbackRecord["procedureId"],
			"digest":   rollbackDigest,
			"version":  rollbackRecord["version"],
		}
	})
	plan := canonicalReleaseObject(t, parseReleaseObject(t, readReleaseFixture(t, "deployment-plan.json")))
	now, err := time.Parse(time.RFC3339, "2026-10-21T09:05:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return executionBindingFixture{
		manifest:   manifest,
		procedure:  procedure,
		rollback:   rollback,
		envelope:   envelope,
		plan:       plan,
		policy:     policy,
		signingKey: key,
		expected: ExecutionBindingExpectation{
			Release: ReleaseExpectation{
				ApplicationID:    "org.example.uptime-demo",
				ReleaseVersion:   "1.0.0",
				ProcedureID:      "proc_install_uptime_demo",
				ProcedureVersion: "1.0.0",
				Operation:        "install",
			},
			TenantScope: "tenant_demo",
			Policy: AuthenticatedCapabilityPolicy{
				BundleID:            "pol_default",
				Version:             "1.0.0",
				Digest:              policyBundleDigest,
				TenantScope:         "tenant_demo",
				AllowedCapabilities: []string{"host.probe", "manifest.verify", "compose.pull", "compose.up", "compose.down", "compose.ps", "compose.logs-redacted", "volume.backup", "volume.restore", "health.check", "secret.inject", "proxy.configure", "proxy.remove"},
			},
			EnvelopeID: "env_demo01",
			PlanID:     "plan_demo01",
			Now:        now,
		},
	}
}

func parseReleaseObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatal("expected JSON object")
	}
	return object
}

func mutateCanonicalObject(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	object := parseReleaseObject(t, raw)
	mutate(object)
	return canonicalReleaseObject(t, object)
}

func canonicalReleaseObject(t *testing.T, value map[string]any) []byte {
	t.Helper()
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func mutateSignedManifest(t *testing.T, fixture *executionBindingFixture, mutate func(map[string]any)) {
	t.Helper()
	manifest := parseReleaseObject(t, fixture.manifest)
	mutate(manifest)
	fixture.manifest = signReleaseValue(t, manifest, fixture.signingKey)
	manifest = parseReleaseObject(t, fixture.manifest)
	digest, err := verify.CanonicalDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	fixture.envelope = mutateCanonicalObject(t, fixture.envelope, func(envelope map[string]any) {
		envelope["manifestDigest"] = digest
		envelope["manifestReviewState"] = manifest["reviewState"]
	})
}

func findPlanStep(t *testing.T, steps []any, stepID string) map[string]any {
	t.Helper()
	for _, rawStep := range steps {
		step := rawStep.(map[string]any)
		if step["stepId"] == stepID {
			return step
		}
	}
	t.Fatalf("plan step %q not found", stepID)
	return nil
}

// A capability may repeat in a signed sequence (backing up two
// volumes) while its parameter contract appears once. Exact cover is
// a set comparison; foreign contracts still fail.
func TestBindProcedureStepsAllowsRepeatedCapability(t *testing.T) {
	procedure := map[string]any{
		"capabilitySequence": []any{"manifest.verify", "volume.backup", "volume.backup"},
		"parameterContracts": []any{
			map[string]any{"capability": "manifest.verify", "requiredParameters": []any{"manifestDigest", "requireReviewState"}},
			map[string]any{"capability": "volume.backup", "requiredParameters": []any{"volume", "backupId"}},
		},
	}
	steps := []map[string]any{
		{"capability": "manifest.verify", "parameters": map[string]any{"manifestDigest": "sha256:x", "requireReviewState": "signed"}},
		{"capability": "volume.backup", "parameters": map[string]any{"volume": "gitea-data", "backupId": "bak_1"}},
		{"capability": "volume.backup", "parameters": map[string]any{"volume": "gitea-config", "backupId": "bak_1"}},
	}
	if err := bindProcedureSteps(steps, procedure, "forward"); err != nil {
		t.Fatalf("repeated capability rejected: %v", err)
	}
	procedure["parameterContracts"] = append(procedure["parameterContracts"].([]any),
		map[string]any{"capability": "compose.up", "requiredParameters": []any{"service"}})
	if err := bindProcedureSteps(steps, procedure, "forward"); err == nil {
		t.Fatal("foreign parameter contract accepted")
	}
}
