package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestDecodeContributionBudgetExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "context", "examples", "contribution-budget.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := DecodeContributionBudget(raw)
	if err != nil {
		t.Fatalf("valid contribution budget example rejected: %v", err)
	}
	if record.ApprovalState != "draft" {
		t.Fatalf("example approval state = %q, want draft", record.ApprovalState)
	}
	if record.OwnerBudget.RAMMb != json.Number("2048") {
		t.Fatalf("owner budget RAM = %q, want exact decimal 2048", record.OwnerBudget.RAMMb)
	}
}

func TestDecodePolicyBundleAndRejectsConflictingRules(t *testing.T) {
	raw := readResourceExample(t, "policy-bundle.json")
	bundle, err := DecodePolicyBundle(raw)
	if err != nil {
		t.Fatalf("valid policy bundle rejected: %v", err)
	}
	if bundle["policyId"] != "pol_default" {
		t.Fatalf("policy identity = %v, want pol_default", bundle["policyId"])
	}

	overlap := mutateResourceExample(t, raw, func(record map[string]any) {
		record["deniedCapabilities"] = []any{"compose.up"}
	})
	if err := ValidatePolicyBundle(overlap); err == nil || !strings.Contains(err.Error(), "both allowed and denied") {
		t.Fatalf("overlapping allow/deny capability was not rejected: %v", err)
	}

	duplicate := mutateResourceExample(t, raw, func(record map[string]any) {
		rules := record["approvalRules"].([]any)
		rules = append(rules, rules[0])
		record["approvalRules"] = rules
	})
	if err := ValidatePolicyBundle(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate approval rule") {
		t.Fatalf("duplicate approval action was not rejected: %v", err)
	}
}

func TestContributionBudgetSemanticRejections(t *testing.T) {
	raw := readResourceExample(t, "contribution-budget.json")
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "allocation and reserve exceed budget by a precise decimal",
			mutate: func(record map[string]any) {
				nestedResource(record, "systemReserve")["cpuCores"] = json.Number("0.5000000000000000001")
			},
			want: "allocations + systemReserve exceed ownerBudget for cpuCores",
		},
		{
			name: "owner budget exceeds physical total",
			mutate: func(record map[string]any) {
				nestedResource(record, "ownerBudget")["ramMb"] = json.Number("7164")
			},
			want: "ownerBudget exceeds physical for ramMb",
		},
		{
			name: "duplicate allocation role",
			mutate: func(record map[string]any) {
				allocations := record["allocations"].([]any)
				allocations[1].(map[string]any)["role"] = allocations[0].(map[string]any)["role"]
			},
			want: "duplicate allocation role",
		},
		{
			name: "offset-equivalent validity endpoints",
			mutate: func(record map[string]any) {
				record["validFrom"] = "2026-10-21T10:00:00+01:00"
				record["validUntil"] = "2026-10-21T09:00:00Z"
			},
			want: "validFrom must be before validUntil",
		},
		{
			name: "inverted sub-nanosecond validity window",
			mutate: func(record map[string]any) {
				record["validFrom"] = "2026-10-21T09:00:00.1234567891Z"
				record["validUntil"] = "2026-10-21T09:00:00.1234567890Z"
			},
			want: "validFrom must be before validUntil",
		},
		{
			name: "negative allocation rejected by schema",
			mutate: func(record map[string]any) {
				allocations := record["allocations"].([]any)
				nested := allocations[0].(map[string]any)["resources"].(map[string]any)
				nested["cpuCores"] = json.Number("-0.01")
			},
			want: "does not satisfy its schema",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := mutateResourceExample(t, raw, tc.mutate)
			err := ValidateContributionBudget(changed)
			if err == nil {
				t.Fatal("invalid contribution budget unexpectedly passed")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should contain %q", err, tc.want)
			}
		})
	}
}

func TestContributionBudgetAcceptsExactOffsetAndSubNanosecondWindow(t *testing.T) {
	raw := readResourceExample(t, "contribution-budget.json")
	changed := mutateResourceExample(t, raw, func(record map[string]any) {
		record["validFrom"] = "2026-10-21T10:00:00.0000000001+01:00"
		record["validUntil"] = "2026-10-21T09:00:00.0000000002Z"
	})
	if err := ValidateContributionBudget(changed); err != nil {
		t.Fatalf("valid exact fractional window rejected: %v", err)
	}
}

func TestDecodeManagedVolumeQualificationExample(t *testing.T) {
	raw := readResourceExample(t, "managed-volume-qualification.json")
	record, err := DecodeManagedVolumeQualification(raw)
	if err != nil {
		t.Fatalf("valid managed-volume example rejected: %v", err)
	}
	if record.Result != "not-qualified" {
		t.Fatalf("example result = %q, want not-qualified", record.Result)
	}
	if record.Storage.QuotaEnforceable || record.Storage.EnforcementMethod != "preflight-only" {
		t.Fatal("example must preserve its preflight-only, non-quota claim")
	}
}

func TestIsWindowActiveUsesExactHalfOpenRFC3339Window(t *testing.T) {
	now := time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)
	active, err := IsWindowActive(
		"2026-10-21T10:00:00+01:00",
		"2026-10-21T09:00:01Z",
		now,
	)
	if err != nil || !active {
		t.Fatalf("offset-equivalent inclusive start should be active: active=%t err=%v", active, err)
	}
	active, err = IsWindowActive(
		"2026-10-21T08:59:59Z",
		"2026-10-21T09:00:00.0000000001Z",
		now,
	)
	if err != nil || !active {
		t.Fatalf("sub-nanosecond future end should remain active at the exact second: active=%t err=%v", active, err)
	}
	active, err = IsWindowActive(
		"2026-10-21T09:00:00.0000000001Z",
		"2026-10-21T09:00:01Z",
		now,
	)
	if err != nil || active {
		t.Fatalf("sub-nanosecond future start should not be active: active=%t err=%v", active, err)
	}
	if _, err := IsWindowActive("2026-10-21T09:01:00Z", "2026-10-21T09:00:00Z", now); err == nil {
		t.Fatal("inverted validity window was accepted")
	}
}

func TestManagedVolumeQualificationRejectsUnsafeClaims(t *testing.T) {
	raw := readResourceExample(t, "managed-volume-qualification.json")
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "remote daemon endpoint",
			mutate: func(record map[string]any) {
				record["daemon"].(map[string]any)["endpointKind"] = "tcp"
			},
			want: "does not satisfy its schema",
		},
		{
			name: "unenforceable quota cannot claim quota enforcement",
			mutate: func(record map[string]any) {
				record["storage"].(map[string]any)["enforcementMethod"] = "filesystem-quota"
			},
			want: "does not satisfy its schema",
		},
		{
			name: "qualified result requires enforceable storage quota",
			mutate: func(record map[string]any) {
				record["result"] = "qualified"
			},
			want: "does not satisfy its schema",
		},
		{
			name: "qualified result requires managed-volume capability",
			mutate: func(record map[string]any) {
				record["result"] = "qualified"
				record["storage"].(map[string]any)["managedVolumesCapable"] = false
			},
			want: "does not satisfy its schema",
		},
		{
			name: "qualified record needs positive free space",
			mutate: func(record map[string]any) {
				record["result"] = "qualified"
				record["storage"].(map[string]any)["quotaEnforceable"] = true
				record["storage"].(map[string]any)["enforcementMethod"] = "docker-volume-quota"
				record["storage"].(map[string]any)["storageFreeMb"] = json.Number("0")
			},
			want: "qualified with no free storage",
		},
		{
			name: "offset-equivalent observation and expiry",
			mutate: func(record map[string]any) {
				record["observedAt"] = "2026-10-21T10:00:00+01:00"
				record["expiresAt"] = "2026-10-21T09:00:00Z"
			},
			want: "expiresAt must be after observedAt",
		},
		{
			name: "inverted sub-nanosecond observation window",
			mutate: func(record map[string]any) {
				record["observedAt"] = "2026-10-21T09:00:00.1234567891Z"
				record["expiresAt"] = "2026-10-21T09:00:00.1234567890Z"
			},
			want: "expiresAt must be after observedAt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := mutateResourceExample(t, raw, tc.mutate)
			err := ValidateManagedVolumeQualification(changed)
			if err == nil {
				t.Fatal("invalid managed-volume qualification unexpectedly passed")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should contain %q", err, tc.want)
			}
		})
	}
}

func TestManagedVolumeQualificationAcceptsExactOffsetAndSubNanosecondWindow(t *testing.T) {
	raw := readResourceExample(t, "managed-volume-qualification.json")
	changed := mutateResourceExample(t, raw, func(record map[string]any) {
		record["observedAt"] = "2026-10-21T10:00:00.0000000001+01:00"
		record["expiresAt"] = "2026-10-21T09:00:00.0000000002Z"
	})
	if err := ValidateManagedVolumeQualification(changed); err != nil {
		t.Fatalf("valid exact fractional window rejected: %v", err)
	}
}

func readResourceExample(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "context", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mutateResourceExample(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	mutate(record)
	changed, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func nestedResource(record map[string]any, name string) map[string]any {
	return record[name].(map[string]any)
}
