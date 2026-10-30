package agentclient

import (
	"encoding/json"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestVerifyExecutionBindingWithResourcesAcceptsExactBoundRecords(t *testing.T) {
	fixture, records := newResourceBindingFixture(t)
	result, err := VerifyExecutionBindingWithResources(
		fixture.manifest, fixture.procedure, fixture.rollback, fixture.envelope, fixture.plan,
		fixture.policy, fixture.expected, records,
	)
	if err != nil {
		t.Fatalf("valid resource-bound execution rejected: %v", err)
	}
	if result.HostProfileDigest == "" || result.ContributionBudgetDigest == "" || result.ManagedVolumeQualificationDigest == "" {
		t.Fatalf("resource binding returned incomplete digests: %+v", result)
	}
}

func TestVerifyExecutionBindingWithResourcesRejectsUnsafeResourceRecords(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *executionBindingFixture, *ExecutionResourceRecords)
	}{
		{
			name: "service CPU exceeds application allocation",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					limits := record["release"].(map[string]any)["resourceLimits"].([]any)
					limits[0].(map[string]any)["cpuCores"] = json.Number("2")
				})
			},
		},
		{
			name: "service RAM exceeds application allocation",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					limits := record["release"].(map[string]any)["resourceLimits"].([]any)
					limits[0].(map[string]any)["ramMb"] = json.Number("2048")
				})
			},
		},
		{
			name: "limits below signed application minimum",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					limits := record["release"].(map[string]any)["resourceLimits"].([]any)
					limits[0].(map[string]any)["cpuCores"] = json.Number("0.5")
					limits[0].(map[string]any)["ramMb"] = json.Number("256")
				})
			},
		},
		{
			name: "duplicate service limit",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					limits := record["release"].(map[string]any)["resourceLimits"].([]any)
					limits = append(limits, map[string]any{
						"service": "app", "cpuCores": json.Number("0.1"), "ramMb": json.Number("64"),
					})
					record["release"].(map[string]any)["resourceLimits"] = limits
				})
			},
		},
		{
			name: "unknown service limit",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					limits := record["release"].(map[string]any)["resourceLimits"].([]any)
					limits[0].(map[string]any)["service"] = "ghost"
				})
			},
		},
		{
			name: "managed volume lacks a hard storage limit",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					volumes := record["release"].(map[string]any)["volumes"].([]any)
					delete(volumes[0].(map[string]any), "storageLimitMb")
				})
			},
		},
		{
			name: "managed volume storage exceeds application allocation",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				mutateSignedManifest(t, fixture, func(record map[string]any) {
					volumes := record["release"].(map[string]any)["volumes"].([]any)
					volumes[0].(map[string]any)["storageLimitMb"] = json.Number("18001")
				})
			},
		},
		{
			name: "missing application budget role",
			mutate: func(t *testing.T, fixture *executionBindingFixture, records *ExecutionResourceRecords) {
				records.ContributionBudgetRaw = mutateResourceRecord(t, records.ContributionBudgetRaw, func(record map[string]any) {
					allocations := record["allocations"].([]any)
					allocations[0].(map[string]any)["role"] = "upgrade-temporary"
				})
				digest, err := canonicalResourceDigest(records.ContributionBudgetRaw, "budget")
				if err != nil {
					t.Fatal(err)
				}
				fixture.envelope = mutateCanonicalObject(t, fixture.envelope, func(envelope map[string]any) {
					envelope["contributionBudgetRef"].(map[string]any)["digest"] = digest
				})
			},
		},
		{
			name: "unapproved budget state",
			mutate: func(t *testing.T, _ *executionBindingFixture, records *ExecutionResourceRecords) {
				records.ContributionBudgetRaw = mutateResourceRecord(t, records.ContributionBudgetRaw, func(record map[string]any) {
					record["approvalState"] = "draft"
				})
			},
		},
		{
			name: "budget host mismatch",
			mutate: func(t *testing.T, _ *executionBindingFixture, records *ExecutionResourceRecords) {
				records.ContributionBudgetRaw = mutateResourceRecord(t, records.ContributionBudgetRaw, func(record map[string]any) {
					record["hostId"] = "host_other"
				})
			},
		},
		{
			name: "preflight-only storage claim",
			mutate: func(t *testing.T, _ *executionBindingFixture, records *ExecutionResourceRecords) {
				records.ManagedVolumeQualificationRaw = mutateResourceRecord(t, records.ManagedVolumeQualificationRaw, func(record map[string]any) {
					storage := record["storage"].(map[string]any)
					storage["quotaEnforceable"] = false
					storage["enforcementMethod"] = "preflight-only"
				})
			},
		},
		{
			name: "expired volume qualification",
			mutate: func(t *testing.T, _ *executionBindingFixture, records *ExecutionResourceRecords) {
				records.ManagedVolumeQualificationRaw = mutateResourceRecord(t, records.ManagedVolumeQualificationRaw, func(record map[string]any) {
					record["expiresAt"] = "2026-10-21T09:04:00Z"
				})
			},
		},
		{
			name: "insufficient managed storage",
			mutate: func(t *testing.T, _ *executionBindingFixture, records *ExecutionResourceRecords) {
				records.ManagedVolumeQualificationRaw = mutateResourceRecord(t, records.ManagedVolumeQualificationRaw, func(record map[string]any) {
					record["storage"].(map[string]any)["storageFreeMb"] = json.Number("19999")
				})
			},
		},
		{
			name: "resource digest mismatch",
			mutate: func(t *testing.T, fixture *executionBindingFixture, _ *ExecutionResourceRecords) {
				fixture.envelope = mutateCanonicalObject(t, fixture.envelope, func(envelope map[string]any) {
					envelope["contributionBudgetRef"].(map[string]any)["digest"] = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture, records := newResourceBindingFixture(t)
			tc.mutate(t, &fixture, &records)
			if _, err := VerifyExecutionBindingWithResources(
				fixture.manifest, fixture.procedure, fixture.rollback, fixture.envelope, fixture.plan,
				fixture.policy, fixture.expected, records,
			); err == nil {
				t.Fatal("unsafe resource-bound execution was accepted")
			}
		})
	}
}

func newResourceBindingFixture(t *testing.T) (executionBindingFixture, ExecutionResourceRecords) {
	t.Helper()
	fixture := newExecutionBindingFixture(t)
	fixture.expected.HostID = "host_demo01"
	budget := contracts.ContributionBudget{
		SchemaVersion: "1.0.0", BudgetID: "budget_test", TenantScope: fixture.expected.TenantScope,
		HostID: fixture.expected.HostID, ApprovalState: "approved", ApprovedBy: "owner_test",
		ApprovedAt: "2026-10-21T09:00:00Z", ValidFrom: "2026-10-21T09:00:00Z", ValidUntil: "2026-10-21T09:10:00Z",
		Physical:      resourceSet("4", "8192", "51200"),
		OwnerBudget:   resourceSet("2", "2048", "20000"),
		SystemReserve: resourceSet("0.25", "512", "1000"),
		Allocations: []contracts.ContributionAllocation{
			{Role: "application", Resources: resourceSet("1.5", "1024", "18000")},
			{Role: "backup-helper", Resources: resourceSet("0.125", "128", "1000")},
		},
	}
	budgetRaw, err := json.Marshal(budget)
	if err != nil {
		t.Fatal(err)
	}
	qualification := contracts.ManagedVolumeQualification{
		SchemaVersion: "1.0.0", QualificationID: "volqual_test", TenantScope: fixture.expected.TenantScope,
		HostID: fixture.expected.HostID, ObservedAt: "2026-10-21T09:00:00Z", ExpiresAt: "2026-10-21T09:10:00Z",
		Daemon:  contracts.ManagedVolumeDaemon{ContextName: "wazi", EndpointKind: "unix", ServerVersion: "27.3.1", RootDir: "/run/user/1000/docker"},
		Storage: contracts.ManagedVolumeStorage{ManagedVolumesCapable: true, EnforcementMethod: "filesystem-quota", QuotaEnforceable: true, StorageFreeMb: json.Number("25000"), FilesystemType: "xfs"},
		Result:  "qualified",
	}
	qualificationRaw, err := json.Marshal(qualification)
	if err != nil {
		t.Fatal(err)
	}
	records := ExecutionResourceRecords{
		HostProfileRaw:                readReleaseFixture(t, "host-profile.json"),
		ContributionBudgetRaw:         budgetRaw,
		ManagedVolumeQualificationRaw: qualificationRaw,
	}
	hostProfile, err := contracts.DecodeHostProfile(records.HostProfileRaw)
	if err != nil {
		t.Fatal(err)
	}
	hostDigest, err := canonicalResourceDigest(records.HostProfileRaw, "host profile")
	if err != nil {
		t.Fatal(err)
	}
	budgetDigest, err := canonicalResourceDigest(records.ContributionBudgetRaw, "budget")
	if err != nil {
		t.Fatal(err)
	}
	qualificationDigest, err := canonicalResourceDigest(records.ManagedVolumeQualificationRaw, "qualification")
	if err != nil {
		t.Fatal(err)
	}
	fixture.envelope = mutateCanonicalObject(t, fixture.envelope, func(envelope map[string]any) {
		envelope["schemaVersion"] = "2.0.0"
		envelope["hostProfileRef"] = recordReference(hostProfile.HostID, hostDigest, hostProfile.SchemaVersion)
		envelope["contributionBudgetRef"] = recordReference(budget.BudgetID, budgetDigest, budget.SchemaVersion)
		envelope["managedVolumeQualificationRef"] = recordReference(qualification.QualificationID, qualificationDigest, qualification.SchemaVersion)
	})
	return fixture, records
}

func resourceSet(cpu, ram, storage string) contracts.ContributionResourceSet {
	return contracts.ContributionResourceSet{CPUCores: json.Number(cpu), RAMMb: json.Number(ram), StorageMb: json.Number(storage)}
}

func recordReference(recordID, digest, version string) map[string]any {
	return map[string]any{"recordId": recordID, "digest": digest, "version": version}
}

func mutateResourceRecord(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	mutate(value.(map[string]any))
	changed, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}
