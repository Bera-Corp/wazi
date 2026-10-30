package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/operationjournal"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type executorBoundary struct {
	journal       *operationjournal.Journal
	jobID         string
	verifyCalls   int
	executeCalls  int
	mutationsSeen int
	executeErr    error
	readState     string
}

func (b *executorBoundary) VerifyRootless(context.Context) error {
	b.verifyCalls++
	return nil
}

func (b *executorBoundary) Execute(_ context.Context, _ PreparedExecution, step Step) (Outcome, error) {
	b.executeCalls++
	if isMutating(step.Capability) {
		b.mutationsSeen++
		snapshot, ok := b.journal.GetByJob(b.jobID)
		if !ok || len(snapshot.Operation.Events) < 3 || snapshot.Operation.Events[len(snapshot.Operation.Events)-1].Kind != operationjournal.EventMutationIntent {
			return Outcome{}, fmt.Errorf("boundary called before durable mutation intent")
		}
	}
	if b.executeErr != nil {
		return Outcome{}, b.executeErr
	}
	state := b.readState
	if state == "" {
		state = "succeeded"
	}
	return Outcome{State: state}, nil
}

func TestExecutorJournalsIntentBeforeTypedMutation(t *testing.T) {
	prepared := executorPrepared(t, "compose.up", `{"service":"gitea","projectName":"wazi-gitea","composeArtifactDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	journal := executorJournal(t)
	boundary := &executorBoundary{journal: journal, jobID: "job_executor01"}
	executor, err := New(Config{
		Journal: journal, Boundary: boundary,
		Preflight: func(context.Context) (PreparedExecution, error) { return prepared, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Execute(context.Background(), Request{JobID: "job_executor01", CaseID: "case_executor01"})
	if err != nil {
		t.Fatalf("execute prepared plan: %v", err)
	}
	if snapshot.Operation.State != operationjournal.StateCompleted {
		t.Fatalf("operation state = %s, want completed", snapshot.Operation.State)
	}
	if boundary.verifyCalls != 1 || boundary.executeCalls != 1 || boundary.mutationsSeen != 1 {
		t.Fatalf("boundary calls = verify:%d execute:%d mutations:%d", boundary.verifyCalls, boundary.executeCalls, boundary.mutationsSeen)
	}
	events := snapshot.Operation.Events
	if len(events) != 5 || events[2].Kind != operationjournal.EventMutationIntent || events[3].Kind != operationjournal.EventMutationResult || events[4].Kind != operationjournal.EventCompleted {
		t.Fatalf("unexpected journal order: %+v", events)
	}
}

func TestExecutorRepreflightsAndRejectsChangedBindingBeforeMutation(t *testing.T) {
	prepared := executorPrepared(t, "compose.up", `{"service":"gitea","projectName":"wazi-gitea","composeArtifactDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	changed := prepared
	changed.binding.PlanDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	journal := executorJournal(t)
	boundary := &executorBoundary{journal: journal, jobID: "job_executor02"}
	preflightCalls := 0
	executor, err := New(Config{
		Journal: journal, Boundary: boundary,
		Preflight: func(context.Context) (PreparedExecution, error) {
			preflightCalls++
			if preflightCalls == 1 {
				return prepared, nil
			}
			return changed, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Execute(context.Background(), Request{JobID: "job_executor02", CaseID: "case_executor02"})
	if err == nil || snapshot.Operation.State != operationjournal.StateFailed {
		t.Fatalf("changed authorization result = state %q, err %v; want failed before mutation", snapshot.Operation.State, err)
	}
	if preflightCalls != 2 || boundary.verifyCalls != 0 || boundary.executeCalls != 0 {
		t.Fatalf("changed preflight reached boundary: preflight=%d verify=%d execute=%d", preflightCalls, boundary.verifyCalls, boundary.executeCalls)
	}
}

func TestExecutorBlocksAmbiguousMutationForReconciliation(t *testing.T) {
	prepared := executorPrepared(t, "compose.up", `{"service":"gitea","projectName":"wazi-gitea","composeArtifactDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	journal := executorJournal(t)
	boundary := &executorBoundary{journal: journal, jobID: "job_executor03", executeErr: errors.New("connection dropped after request")}
	executor, err := New(Config{
		Journal: journal, Boundary: boundary,
		Preflight: func(context.Context) (PreparedExecution, error) { return prepared, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Execute(context.Background(), Request{JobID: "job_executor03", CaseID: "case_executor03"})
	if err == nil || snapshot.Operation.State != operationjournal.StateReconciliationNeeded || snapshot.PendingIntent == nil {
		t.Fatalf("ambiguous result = state %q pending %+v err %v", snapshot.Operation.State, snapshot.PendingIntent, err)
	}
	if boundary.executeCalls != 1 {
		t.Fatalf("executor continued after ambiguous mutation: %d calls", boundary.executeCalls)
	}
}

func TestExecutorFailsClosedOnUnverifiedPreflight(t *testing.T) {
	prepared := executorPrepared(t, "compose.up", `{"service":"gitea","projectName":"wazi-gitea","composeArtifactDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	prepared.valid = false
	journal := executorJournal(t)
	boundary := &executorBoundary{journal: journal, jobID: "job_executor04"}
	executor, err := New(Config{
		Journal: journal, Boundary: boundary,
		Preflight: func(context.Context) (PreparedExecution, error) { return prepared, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), Request{JobID: "job_executor04", CaseID: "case_executor04"}); err == nil {
		t.Fatal("unverified preflight was accepted")
	}
	if _, ok := journal.GetByJob("job_executor04"); ok || boundary.executeCalls != 0 {
		t.Fatal("unverified preflight created an operation or called the runtime")
	}
}

func TestExecutorRejectsUnverifiedReadOnlyOutcome(t *testing.T) {
	prepared := executorPrepared(t, "host.probe", `{"checks":["docker"]}`)
	journal := executorJournal(t)
	boundary := &executorBoundary{journal: journal, jobID: "job_executor05", readState: "ambiguous"}
	executor, err := New(Config{
		Journal: journal, Boundary: boundary,
		Preflight: func(context.Context) (PreparedExecution, error) { return prepared, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Execute(context.Background(), Request{JobID: "job_executor05", CaseID: "case_executor05"})
	if err == nil || snapshot.Operation.State != operationjournal.StateFailed || boundary.verifyCalls != 0 {
		t.Fatalf("read-only outcome = state %q verify %d err %v", snapshot.Operation.State, boundary.verifyCalls, err)
	}
}

func TestExecutorRejectsImageAndComposeDigestMismatchesBeforeRuntime(t *testing.T) {
	tests := []struct {
		name       string
		capability string
		parameters string
	}{
		{
			name:       "image digest differs from signed manifest",
			capability: "compose.pull",
			parameters: "{\"service\":\"gitea\",\"imageDigest\":\"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\"}",
		},
		{
			name:       "compose artifact digest differs from signed release",
			capability: "compose.up",
			parameters: "{\"service\":\"gitea\",\"projectName\":\"wazi-gitea\",\"composeArtifactDigest\":\"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd\"}",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepared := executorPrepared(t, test.capability, test.parameters)
			journal := executorJournal(t)
			boundary := &executorBoundary{journal: journal, jobID: "job_executor_digest"}
			executor, err := New(Config{
				Journal: journal, Boundary: boundary,
				Preflight: func(context.Context) (PreparedExecution, error) { return prepared, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := executor.Execute(context.Background(), Request{
				JobID: "job_executor_digest", CaseID: "case_executor_digest",
			}); err == nil {
				t.Fatal("mismatched digest was accepted")
			}
			if _, exists := journal.GetByJob("job_executor_digest"); exists {
				t.Fatal("mismatched digest created a journal operation")
			}
			if boundary.verifyCalls != 0 || boundary.executeCalls != 0 {
				t.Fatalf("mismatched digest reached runtime: verify=%d execute=%d", boundary.verifyCalls, boundary.executeCalls)
			}
		})
	}
}

func executorJournal(t *testing.T) *operationjournal.Journal {
	t.Helper()
	journal, err := operationjournal.New(filepath.Join(t.TempDir(), "journal"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	return journal
}

func executorPrepared(t *testing.T, capability, parameters string) PreparedExecution {
	t.Helper()
	params := jsonObject(t, parameters)
	plan := map[string]any{
		"schemaVersion": "1.0.0", "planId": "plan_executor01", "envelopeId": "env_executor01",
		"composeArtifactRef": map[string]any{"recordId": "art_executor01", "digest": "sha256:" + strings.Repeat("a", 64), "version": "1.0.0"},
		"operation":          "install", "origin": "deterministic-resolver", "unresolvedQuestions": []any{},
		"steps": []any{map[string]any{
			"stepId": "step_executor", "capability": capability, "parameters": params,
			"citations": []any{"manifest_executor01"}, "requiresApproval": false,
			"verification": map[string]any{"type": "exit-state"},
		}},
		"rollbackSteps": []any{}, "requiredApprovals": []any{},
		"createdAt": "2026-10-30T17:00:00Z", "expiresAt": "2099-10-30T17:00:00Z",
	}
	planRaw := canonical(t, plan)
	envelopeRaw := canonical(t, map[string]any{"envelopeId": "env_executor01"})
	manifestRaw := canonical(t, map[string]any{
		"appId": "org.example.gitea", "reviewState": "signed",
		"release": map[string]any{
			"images":      []any{map[string]any{"service": "gitea", "image": "ghcr.io/example/gitea", "digest": "sha256:" + strings.Repeat("b", 64)}},
			"volumes":     []any{map[string]any{"name": "gitea-data", "backup": true}},
			"ports":       []any{map[string]any{"hostPort": json.Number("3000"), "exposure": "private"}},
			"healthCheck": map[string]any{"target": "http://localhost:3000/api/healthz"},
			"environment": []any{},
		},
	})
	planValue := jsonObject(t, string(planRaw))
	envelopeValue := jsonObject(t, string(envelopeRaw))
	manifestValue := jsonObject(t, string(manifestRaw))
	planDigest, err := verify.CanonicalDigest(planValue)
	if err != nil {
		t.Fatal(err)
	}
	envelopeDigest, err := verify.CanonicalDigest(envelopeValue)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := verify.CanonicalDigest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	procedureDigest := "sha256:" + strings.Repeat("d", 64)
	return PreparedExecution{
		valid: true,
		binding: VerifiedExecutionBinding{
			Release:        VerifiedRelease{ManifestDigest: manifestDigest, ProcedureDigest: procedureDigest},
			EnvelopeDigest: envelopeDigest, PlanDigest: planDigest,
			RollbackProcedureDigest:          "sha256:" + strings.Repeat("e", 64),
			HostProfileDigest:                "sha256:" + strings.Repeat("f", 64),
			ContributionBudgetDigest:         "sha256:" + strings.Repeat("1", 64),
			ManagedVolumeQualificationDigest: "sha256:" + strings.Repeat("2", 64),
		},
		release:     VerifiedReleaseArtifacts{Release: VerifiedRelease{ManifestDigest: manifestDigest, ProcedureDigest: procedureDigest}, manifestRaw: manifestRaw},
		envelopeRaw: envelopeRaw, planRaw: planRaw, tenantScope: "tenant_executor", hostID: "host_executor01",
		policy: VerifiedCapabilityPolicy{snapshot: AuthenticatedCapabilityPolicy{
			BundleID: "pol_executor01", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("3", 64),
			TenantScope: "tenant_executor", AllowedCapabilities: []string{capability},
		}},
	}
}

func canonical(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func jsonObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	value, err := verify.ParseJSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T", value)
	}
	return object
}
