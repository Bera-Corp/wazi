package agentclient

import (
	"testing"
	"time"
)

func TestVerifyApprovalRecordsBindsExactCaseAndLivePolicy(t *testing.T) {
	raw := canonicalApprovalFixtureForAgent(t)
	expected := validApprovalExpectation()
	if err := VerifyApprovalRecords([][]byte{raw}, expected); err != nil {
		t.Fatalf("valid approval rejected: %v", err)
	}

	wrongCase := mutateCanonicalObject(t, raw, func(record map[string]any) {
		record["caseId"] = "case_other"
	})
	if err := VerifyApprovalRecords([][]byte{wrongCase}, expected); err == nil {
		t.Fatal("approval for a different case was accepted")
	}
	wrongPlan := mutateCanonicalObject(t, raw, func(record map[string]any) {
		record["planDigest"] = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	})
	if err := VerifyApprovalRecords([][]byte{wrongPlan}, expected); err == nil {
		t.Fatal("approval for a different plan digest was accepted")
	}
	denied := mutateCanonicalObject(t, raw, func(record map[string]any) {
		record["decision"] = "denied"
	})
	if err := VerifyApprovalRecords([][]byte{denied}, expected); err == nil {
		t.Fatal("denied approval was accepted")
	}
	expected.Now = time.Date(2026, time.October, 22, 0, 0, 0, 0, time.UTC)
	if err := VerifyApprovalRecords([][]byte{raw}, expected); err == nil {
		t.Fatal("expired approval was accepted")
	}
}

func TestVerifyApprovalRecordsRejectsFutureAndOverlongValidity(t *testing.T) {
	raw := canonicalApprovalFixtureForAgent(t)
	future := mutateCanonicalObject(t, raw, func(record map[string]any) {
		record["approvedAt"] = "2026-10-21T09:06:00Z"
	})
	if err := VerifyApprovalRecords([][]byte{future}, validApprovalExpectation()); err == nil {
		t.Fatal("future approval was accepted")
	}
	expected := validApprovalExpectation()
	expected.MaxValidity = 10 * time.Minute
	if err := VerifyApprovalRecords([][]byte{raw}, expected); err == nil {
		t.Fatal("approval longer than the configured maximum was accepted")
	}
}

func TestVerifyApprovalRecordsRequiresDistinctMaintainersForQuorum(t *testing.T) {
	base := canonicalApprovalFixtureForAgent(t)
	first := mutateCanonicalObject(t, base, func(record map[string]any) {
		record["approvalId"] = "appr_maintainer1"
		record["approver"].(map[string]any)["approverId"] = "maintainer_one"
		record["approver"].(map[string]any)["role"] = "maintainer"
	})
	second := mutateCanonicalObject(t, base, func(record map[string]any) {
		record["approvalId"] = "appr_maintainer2"
		record["approver"].(map[string]any)["approverId"] = "maintainer_two"
		record["approver"].(map[string]any)["role"] = "maintainer"
	})
	expected := validApprovalExpectation()
	expected.Requirements["public-exposure"] = "two-maintainers"
	if err := VerifyApprovalRecords([][]byte{first, second}, expected); err != nil {
		t.Fatalf("two distinct maintainers rejected: %v", err)
	}
	if err := VerifyApprovalRecords([][]byte{first}, expected); err == nil {
		t.Fatal("one maintainer satisfied a two-maintainer quorum")
	}
	duplicate := mutateCanonicalObject(t, second, func(record map[string]any) {
		record["approver"].(map[string]any)["approverId"] = "maintainer_one"
	})
	if err := VerifyApprovalRecords([][]byte{first, duplicate}, expected); err == nil {
		t.Fatal("duplicate approver satisfied a two-maintainer quorum")
	}
}

func TestVerifyApprovalRecordsRequiresActionPolicy(t *testing.T) {
	expected := validApprovalExpectation()
	delete(expected.Requirements, "public-exposure")
	if err := VerifyApprovalRecords([][]byte{canonicalApprovalFixtureForAgent(t)}, expected); err == nil {
		t.Fatal("approval with a missing policy rule was accepted")
	}
	expected = validApprovalExpectation()
	expected.Requirements["public-exposure"] = "maintainer"
	if err := VerifyApprovalRecords([][]byte{canonicalApprovalFixtureForAgent(t)}, expected); err == nil {
		t.Fatal("user approval satisfied a maintainer-only action")
	}
}

func canonicalApprovalFixtureForAgent(t *testing.T) []byte {
	t.Helper()
	return canonicalReleaseObject(t, parseReleaseObject(t, readReleaseFixture(t, "approval-record.json")))
}

func validApprovalExpectation() ApprovalExpectation {
	return ApprovalExpectation{
		CaseID:          "case_demo01",
		PlanID:          "plan_demo01",
		PlanDigest:      "sha256:ba161af0b8d8fb71f2542956a6fd2acf7abd368ac3cb74498a85f8b711d96cb4",
		EnvelopeID:      "env_demo01",
		EnvelopeDigest:  "sha256:d652131efbe34237cc664ec81741d5de7dc30f54eb3906ae8338f0e6f31f76a4",
		TenantScope:     "tenant_demo",
		HostID:          "host_demo01",
		ApplicationID:   "org.example.uptime-demo",
		RequiredActions: []string{"public-exposure"},
		Requirements:    map[string]string{"public-exposure": "user"},
		Now:             time.Date(2026, time.October, 21, 9, 5, 0, 0, time.UTC),
		MaxValidity:     30 * time.Minute,
	}
}
