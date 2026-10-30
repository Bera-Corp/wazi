package approvalregistry

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestApprovalConsumptionIsDurableSingleUseAndIdempotentPerJob(t *testing.T) {
	now := time.Date(2026, time.October, 21, 9, 5, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "store")
	registry := openApprovalRegistry(t, root, now)
	raw := canonicalApprovalFixture(t)
	if _, err := registry.Put(raw); err != nil {
		t.Fatalf("put approval: %v", err)
	}
	first, entry, err := registry.Consume("appr_demo01", validConsumption())
	if err != nil {
		t.Fatalf("consume approval: %v", err)
	}
	if entry.ConsumedBy != "job_demo01" || string(first) != string(raw) {
		t.Fatalf("unexpected consumed approval: %+v", entry)
	}
	if _, _, err := registry.Consume("appr_demo01", validConsumption()); err != nil {
		t.Fatalf("same-job retry was not idempotent: %v", err)
	}
	otherJob := validConsumption()
	otherJob.JobID = "job_demo02"
	if _, _, err := registry.Consume("appr_demo01", otherJob); !errors.Is(err, ErrAlreadyConsumed) {
		t.Fatalf("different-job replay got %v, want ErrAlreadyConsumed", err)
	}
	closeApprovalRegistry(t, registry)

	registry = openApprovalRegistry(t, root, now)
	defer closeApprovalRegistry(t, registry)
	if _, _, err := registry.Consume("appr_demo01", otherJob); !errors.Is(err, ErrAlreadyConsumed) {
		t.Fatalf("restart forgot consumption state: %v", err)
	}
	if _, _, err := registry.Consume("appr_demo01", validConsumption()); err != nil {
		t.Fatalf("same-job retry after restart failed: %v", err)
	}
}

func TestApprovalConsumptionRejectsMismatchedCaseOrDigestsBeforeMutation(t *testing.T) {
	now := time.Date(2026, time.October, 21, 9, 5, 0, 0, time.UTC)
	registry := openApprovalRegistry(t, filepath.Join(t.TempDir(), "store"), now)
	defer closeApprovalRegistry(t, registry)
	if _, err := registry.Put(canonicalApprovalFixture(t)); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*ConsumptionBinding){
		func(binding *ConsumptionBinding) { binding.CaseID = "case_other" },
		func(binding *ConsumptionBinding) { binding.TenantScope = "tenant_other" },
		func(binding *ConsumptionBinding) {
			binding.PlanDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		func(binding *ConsumptionBinding) {
			binding.EnvelopeDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		func(binding *ConsumptionBinding) { binding.HostID = "host_other" },
	}
	for index, mutate := range mutations {
		binding := validConsumption()
		mutate(&binding)
		if _, _, err := registry.Consume("appr_demo01", binding); !errors.Is(err, ErrBindingMismatch) {
			t.Fatalf("binding mutation %d got %v, want ErrBindingMismatch", index, err)
		}
	}
	_, entry, ok := registry.Get("appr_demo01")
	if !ok || entry.ConsumedBy != "" {
		t.Fatalf("mismatched consumption changed durable state: %+v, found=%v", entry, ok)
	}
}

func TestApprovalRegistryRejectsExpiredAndCorruptRecords(t *testing.T) {
	now := time.Date(2026, time.October, 22, 0, 0, 0, 0, time.UTC)
	registry := openApprovalRegistry(t, filepath.Join(t.TempDir(), "store"), now)
	defer closeApprovalRegistry(t, registry)
	if _, err := registry.Put(canonicalApprovalFixture(t)); err == nil {
		t.Fatal("expired approval was stored")
	}

	root := filepath.Join(t.TempDir(), "store")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(root, "appr_corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, func() time.Time { return now }); err == nil {
		t.Fatal("corrupt approval state was accepted at startup")
	}
}

func TestApprovalRegistryAllowsOnlyOneWriter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	first := openApprovalRegistry(t, root, time.Now())
	if _, err := New(root, time.Now); err == nil {
		t.Fatal("second registry writer unexpectedly opened")
	}
	closeApprovalRegistry(t, first)
}

func openApprovalRegistry(t *testing.T, root string, now time.Time) *Registry {
	t.Helper()
	registry, err := New(root, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func closeApprovalRegistry(t *testing.T, registry *Registry) {
	t.Helper()
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
}

func canonicalApprovalFixture(t *testing.T) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "context", "examples", "approval-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func validConsumption() ConsumptionBinding {
	return ConsumptionBinding{
		JobID:          "job_demo01",
		CaseID:         "case_demo01",
		PlanID:         "plan_demo01",
		PlanDigest:     "sha256:ba161af0b8d8fb71f2542956a6fd2acf7abd368ac3cb74498a85f8b711d96cb4",
		EnvelopeID:     "env_demo01",
		EnvelopeDigest: "sha256:d652131efbe34237cc664ec81741d5de7dc30f54eb3906ae8338f0e6f31f76a4",
		TenantScope:    "tenant_demo",
		HostID:         "host_demo01",
		ApplicationID:  "org.example.uptime-demo",
		Action:         "public-exposure",
	}
}
