package contracts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
}

func TestEmbeddedSchemasMatchCanonicalSource(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{"common.schema.json", "app-manifest.schema.json", "procedure.schema.json", "revocation-record.schema.json", "execution-context-envelope.schema.json", "execution-context-envelope-v2.schema.json", "deployment-plan.schema.json", "approval-record.schema.json", "contribution-budget.schema.json", "managed-volume-qualification.schema.json", "policy-bundle.schema.json"} {
		source, err := os.ReadFile(filepath.Join(root, "schemas", name))
		if err != nil {
			t.Fatalf("read canonical %s: %v", name, err)
		}
		embedded, err := embeddedSchemas.ReadFile("schema/" + name)
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		if !bytes.Equal(embedded, source) {
			t.Errorf("embedded schema %s is stale; copy the canonical root schema", name)
		}
	}
}

func TestValidateApprovalRecordAgainstCanonicalSchema(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "context", "examples", "approval-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateApprovalRecord(raw); err != nil {
		t.Fatalf("valid approval record rejected by schema: %v", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	delete(record, "caseId")
	changed, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateApprovalRecord(changed); err == nil {
		t.Fatal("approval record without a durable case identifier unexpectedly passed")
	}
}

func TestValidateExecutionEnvelopeAndPlanAgainstCanonicalSchemas(t *testing.T) {
	root := repositoryRoot(t)
	envelope, err := os.ReadFile(filepath.Join(root, "context", "examples", "execution-context-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutionContextEnvelope(envelope); err != nil {
		t.Fatalf("valid execution context envelope rejected by schema: %v", err)
	}
	plan, err := os.ReadFile(filepath.Join(root, "context", "examples", "deployment-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeploymentPlan(plan); err != nil {
		t.Fatalf("valid deployment plan rejected by schema: %v", err)
	}
	value, err := verify.ParseJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	record["unexpectedAuthority"] = "self-supplied"
	changed, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeploymentPlan(changed); err == nil {
		t.Fatal("deployment plan with an unknown top-level property unexpectedly passed")
	}
}

func TestValidateExecutionEnvelopeV2RequiresResourceReferences(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "context", "examples", "execution-context-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	envelope := value.(map[string]any)
	envelope["schemaVersion"] = "2.0.0"
	ref := func(recordID string) map[string]any {
		return map[string]any{
			"recordId": recordID,
			"digest":   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"version":  "1.0.0",
		}
	}
	envelope["contributionBudgetRef"] = ref("budget_demo")
	envelope["managedVolumeQualificationRef"] = ref("volqual_demo")
	changed, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutionContextEnvelopeV2(changed); err != nil {
		t.Fatalf("valid resource-bound v2 envelope rejected: %v", err)
	}
	if err := ValidateExecutionContextEnvelope(changed); err == nil {
		t.Fatal("v2 resource-bound envelope unexpectedly passed the unchanged v1 contract")
	}
	delete(envelope, "managedVolumeQualificationRef")
	missingRef, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutionContextEnvelopeV2(missingRef); err == nil {
		t.Fatal("v2 envelope without managed-volume qualification reference unexpectedly passed")
	}
}

func TestValidateRevocationRecordAgainstCanonicalSchema(t *testing.T) {
	record := map[string]any{
		"schemaVersion": "1.0.0",
		"listId":        "revlist_phase1",
		"sequence":      1,
		"generatedAt":   "2026-10-30T10:00:00Z",
		"expiresAt":     "2026-10-30T10:10:00Z",
		"entries":       []any{},
		"signature": map[string]any{
			"algorithm":     "sigstore-cosign",
			"value":         "Y2Fub25pY2FsLXNpZ25hdHVyZQ==",
			"signedBy":      "phase1-test-signer",
			"payloadDigest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRevocationRecord(raw); err != nil {
		t.Fatalf("valid revocation record rejected by schema: %v", err)
	}
	delete(record, "sequence")
	raw, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRevocationRecord(raw); err == nil {
		t.Fatal("revocation record without a monotonic sequence unexpectedly passed")
	}
}

func TestValidateProcedureAgainstCanonicalSchema(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "context", "examples", "procedure-install.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProcedure(raw); err != nil {
		t.Fatalf("valid procedure example rejected by schema: %v", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	record["unexpectedCapability"] = "shell.exec"
	changed, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProcedure(changed); err == nil {
		t.Fatal("procedure with an unknown capability property unexpectedly passed")
	}
}

func TestValidateAppManifestAgainstCanonicalSchema(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "services", "internal", "verify", "testdata", "cosign-v3.1.4-app-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAppManifest(raw); err != nil {
		t.Fatalf("valid Cosign-signed manifest rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "unknown property",
			mutate: func(record map[string]any) {
				record["unexpectedAuthority"] = "self-supplied"
			},
		},
		{
			name: "missing required property",
			mutate: func(record map[string]any) {
				delete(record, "license")
			},
		},
		{
			name: "unsupported schema version",
			mutate: func(record map[string]any) {
				record["schemaVersion"] = "2.0.0"
			},
		},
		{
			name: "privileged container",
			mutate: func(record map[string]any) {
				requirements := record["requirements"].(map[string]any)
				requirements["privileged"] = true
			},
		},
		{
			name: "invalid image digest",
			mutate: func(record map[string]any) {
				release := record["release"].(map[string]any)
				images := release["images"].([]any)
				images[0].(map[string]any)["digest"] = "mutable:latest"
			},
		},
		{
			name: "non-positive service resource ceiling",
			mutate: func(record map[string]any) {
				release := record["release"].(map[string]any)
				release["resourceLimits"] = []any{map[string]any{
					"service": "app", "cpuCores": json.Number("0"), "ramMb": json.Number("512"),
				}}
			},
		},
		{
			name: "non-positive managed-volume ceiling",
			mutate: func(record map[string]any) {
				release := record["release"].(map[string]any)
				volumes := release["volumes"].([]any)
				volumes[0].(map[string]any)["storageLimitMb"] = json.Number("0")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := verify.ParseJSON(raw)
			if err != nil {
				t.Fatal(err)
			}
			record := value.(map[string]any)
			test.mutate(record)
			changed, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateAppManifest(changed); err == nil {
				t.Fatal("invalid manifest unexpectedly passed schema validation")
			}
		})
	}
}

func TestValidateAppManifestRejectsDuplicateKeys(t *testing.T) {
	raw := []byte("{\"appId\":\"org.example.test\",\"appId\":\"org.example.other\"}")
	if err := ValidateAppManifest(raw); err == nil {
		t.Fatal("duplicate JSON object key unexpectedly passed")
	}
}
