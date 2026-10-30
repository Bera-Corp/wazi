package operationjournal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOperationJournalPersistsIntentAndRequiresReconciliationAfterRestart(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	root := t.TempDir()
	journal := openJournal(t, root, &now)
	created, existing, err := journal.Begin(testIdentity())
	if err != nil || existing {
		t.Fatalf("begin operation: existing=%v err=%v", existing, err)
	}
	if created.State != StateAccepted {
		t.Fatalf("initial state = %s, want accepted", created.State)
	}
	if _, err := journal.Start(created.Identity.OperationID); err != nil {
		t.Fatalf("start operation: %v", err)
	}
	intent, err := journal.RecordIntent(created.Identity.OperationID, "start-app", "compose.up",
		"compose-project:gitea", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("record mutation intent before execution: %v", err)
	}
	if got := intent.Events[len(intent.Events)-1].Kind; got != EventMutationIntent {
		t.Fatalf("last event = %s, want mutation-intent", got)
	}
	closeJournal(t, journal)

	// Opening a new process context sees the persisted intent as interrupted.
	// The journal does not retry the capability or accept a late result.
	journal = openJournal(t, root, &now)
	defer closeJournal(t, journal)
	snapshot, ok := journal.Get(created.Identity.OperationID)
	if !ok || !snapshot.RecoveryRequired || snapshot.PendingIntent == nil {
		t.Fatalf("interrupted operation snapshot = %+v, found=%v", snapshot, ok)
	}
	if snapshot.PendingIntent.StepID != "start-app" || snapshot.PendingIntent.ResourceRef != "compose-project:gitea" {
		t.Fatalf("pending intent lost its owned-resource binding: %+v", snapshot.PendingIntent)
	}
	if _, err := journal.Start(created.Identity.OperationID); !errors.Is(err, ErrReconciliationNeeded) {
		t.Fatalf("duplicate start after restart = %v, want reconciliation requirement", err)
	}
	if _, err := journal.RecordOutcome(created.Identity.OperationID, "start-app", "succeeded", ""); !errors.Is(err, ErrReconciliationNeeded) {
		t.Fatalf("late mutation result after restart = %v, want reconciliation requirement", err)
	}
	recoveries := journal.ListRecoveryRequired()
	if len(recoveries) != 1 || recoveries[0].Operation.Identity.OperationID != created.Identity.OperationID {
		t.Fatalf("recovery list = %+v", recoveries)
	}
	blocked, err := journal.Reconcile(created.Identity.OperationID, "start-app", "present",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatalf("record external resource reconciliation: %v", err)
	}
	if blocked.State != StateBlocked {
		t.Fatalf("reconciled state = %s, want blocked", blocked.State)
	}
	if _, err := journal.Start(created.Identity.OperationID); !errors.Is(err, ErrTerminal) {
		t.Fatalf("reconciled operation restarted = %v, want terminal block", err)
	}
}

func TestAmbiguousOutcomeRequiresReconciliationAndBlocksFurtherMutations(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	journal := openJournal(t, t.TempDir(), &now)
	defer closeJournal(t, journal)
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Start(operation.Identity.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordIntent(operation.Identity.OperationID, "restore-data", "volume.restore",
		"volume:gitea-data", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := journal.RecordOutcome(operation.Identity.OperationID, "restore-data", "ambiguous", "daemon.connection_lost")
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.State != StateReconciliationNeeded {
		t.Fatalf("ambiguous result state = %s, want reconciliation-required", ambiguous.State)
	}
	if _, err := journal.RecordIntent(operation.Identity.OperationID, "start-app", "compose.up",
		"compose-project:gitea", "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"); !errors.Is(err, ErrReconciliationNeeded) {
		t.Fatalf("follow-on mutation after ambiguous restore = %v", err)
	}
	blocked, err := journal.Reconcile(operation.Identity.OperationID, "restore-data", "unknown",
		"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	if err != nil || blocked.State != StateBlocked {
		t.Fatalf("reconcile ambiguous restore: state=%s err=%v", blocked.State, err)
	}
}

func TestRestartBeforeFirstMutationBlocksRatherThanResuming(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	root := t.TempDir()
	journal := openJournal(t, root, &now)
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Start(operation.Identity.OperationID); err != nil {
		t.Fatal(err)
	}
	closeJournal(t, journal)

	journal = openJournal(t, root, &now)
	defer closeJournal(t, journal)
	restarted, ok := journal.Get(operation.Identity.OperationID)
	if !ok || restarted.Operation.State != StateBlocked {
		t.Fatalf("restarted operation = %+v, found=%v, want blocked", restarted, ok)
	}
	last := restarted.Operation.Events[len(restarted.Operation.Events)-1]
	if last.Kind != EventBlocked || last.Code != "process.restarted" {
		t.Fatalf("restart event = %+v", last)
	}
	if _, err := journal.RecordIntent(operation.Identity.OperationID, "start-app", "compose.up",
		"compose-project:gitea", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); !errors.Is(err, ErrTerminal) {
		t.Fatalf("restarted operation accepted a new mutation = %v, want terminal block", err)
	}
}

func TestBeginIsIdempotentAndRejectsCaseReuse(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	journal := openJournal(t, t.TempDir(), &now)
	defer closeJournal(t, journal)
	first, existing, err := journal.Begin(testIdentity())
	if err != nil || existing {
		t.Fatalf("first begin: existing=%v err=%v", existing, err)
	}
	retry, existing, err := journal.Begin(testIdentity())
	if err != nil || !existing || retry.Identity != first.Identity {
		t.Fatalf("idempotent begin: existing=%v err=%v", existing, err)
	}
	changed := testIdentity()
	changed.ManifestDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if _, _, err := journal.Begin(changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("operation ID with changed digest = %v, want conflict", err)
	}
	otherJob := testIdentity()
	otherJob.OperationID = "op_demo02"
	otherJob.JobID = "job_demo02"
	if _, _, err := journal.Begin(otherJob); !errors.Is(err, ErrCaseAlreadyUsed) {
		t.Fatalf("new job reusing case = %v, want case conflict", err)
	}
}

func TestFailedMutationCanBeCompensatedBeforeOperationCompletes(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	journal := openJournal(t, t.TempDir(), &now)
	defer closeJournal(t, journal)
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	id := operation.Identity.OperationID
	if _, err := journal.Start(id); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordIntent(id, "start-app", "compose.up", "compose-project:gitea",
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordOutcome(id, "start-app", "failed", "runtime.exit_nonzero"); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordIntent(id, "cleanup-app", "compose.down", "compose-project:gitea",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err != nil {
		t.Fatalf("record compensation intent: %v", err)
	}
	if _, err := journal.RecordOutcome(id, "cleanup-app", "succeeded", ""); err != nil {
		t.Fatalf("record compensation result: %v", err)
	}
	completed, err := journal.Complete(id)
	if err != nil || completed.State != StateCompleted {
		t.Fatalf("complete after successful compensation: state=%s err=%v", completed.State, err)
	}
}

func TestCancellationCannotEraseAnUnresolvedMutation(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	journal := openJournal(t, t.TempDir(), &now)
	defer closeJournal(t, journal)
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	id := operation.Identity.OperationID
	if _, err := journal.Start(id); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordIntent(id, "pull-image", "compose.pull", "image:gitea",
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Cancel(id, "operator.cancelled"); !errors.Is(err, ErrReconciliationNeeded) {
		t.Fatalf("cancel with unresolved intent = %v, want reconciliation requirement", err)
	}

	other := testIdentity()
	other.OperationID = "op_demo02"
	other.JobID = "job_demo02"
	other.CaseID = "case_demo02"
	accepted, _, err := journal.Begin(other)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := journal.Cancel(accepted.Identity.OperationID, "operator.cancelled")
	if err != nil || cancelled.State != StateCancelled {
		t.Fatalf("cancel before first mutation: state=%s err=%v", cancelled.State, err)
	}
}

func TestJournalRejectsReadOnlyCapabilitiesAndCorruptRestart(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	root := t.TempDir()
	journal := openJournal(t, root, &now)
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Start(operation.Identity.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordIntent(operation.Identity.OperationID, "probe", "host.probe", "host:bera-003",
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("read-only capability intent = %v, want rejected", err)
	}
	closeJournal(t, journal)

	path := filepath.Join(root, operation.Identity.OperationID+".json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, func() time.Time { return now }); err == nil {
		t.Fatal("corrupt journal state was accepted at startup")
	}
}

func TestJournalAllowsOnlyOneWriterAndReturnsDefensiveSnapshots(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	root := t.TempDir()
	journal := openJournal(t, root, &now)
	if _, err := New(root, func() time.Time { return now }); err == nil {
		t.Fatal("second writer unexpectedly opened")
	}
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := journal.Get(operation.Identity.OperationID)
	if !ok {
		t.Fatal("journal entry was not found")
	}
	snapshot.Operation.Events[0].Kind = EventBlocked
	again, ok := journal.Get(operation.Identity.OperationID)
	if !ok || again.Operation.Events[0].Kind != EventCreated {
		t.Fatal("caller mutation changed the journal's in-memory event")
	}
	closeJournal(t, journal)
}

func TestJournalRejectsClockRollback(t *testing.T) {
	now := time.Date(2026, time.October, 30, 8, 0, 0, 0, time.UTC)
	journal := openJournal(t, t.TempDir(), &now)
	defer closeJournal(t, journal)
	operation, _, err := journal.Begin(testIdentity())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(-time.Second)
	if _, err := journal.Start(operation.Identity.OperationID); err == nil {
		t.Fatal("journal accepted an event after clock rollback")
	}
}

func openJournal(t *testing.T, root string, now *time.Time) *Journal {
	t.Helper()
	journal, err := New(root, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func closeJournal(t *testing.T, journal *Journal) {
	t.Helper()
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func testIdentity() Identity {
	return Identity{
		OperationID: "op_demo01", JobID: "job_demo01", CaseID: "case_demo01",
		PlanID:         "plan_demo01",
		PlanDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EnvelopeID:     "env_demo01",
		EnvelopeDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		TenantScope:    "tenant_demo", HostID: "host_demo01",
		ApplicationID:  "org.example.gitea",
		ManifestDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Operation:      "install",
	}
}
