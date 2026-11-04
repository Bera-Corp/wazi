package pilotqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
)

func TestApprovalFreeTransportAcceptsOnlyClosedHealthJobs(t *testing.T) {
	load := func(name string) map[string]any {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "context", "examples", name))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	envelope, plan := load("execution-context-envelope.json"), load("deployment-plan.json")
	mutation := plan["steps"].([]any)[1]
	steps := plan["steps"].([]any)
	plan["operation"], envelope["operation"] = "health-check", "health-check"
	plan["steps"] = []any{steps[0], steps[len(steps)-1]}
	plan["rollbackSteps"], plan["requiredApprovals"] = []any{}, []any{}
	envelope["schemaVersion"] = "2.0.0"
	envelope["contributionBudgetRef"] = map[string]any{"recordId": "budget_queue", "version": "1.0.0", "digest": "sha256:" + strings.Repeat("a", 64)}
	envelope["managedVolumeQualificationRef"] = map[string]any{"recordId": "volqual_queue", "version": "1.0.0", "digest": "sha256:" + strings.Repeat("b", 64)}
	envelope["requiredApprovals"] = []any{}
	envelope["allowedCapabilities"] = []any{"manifest.verify", "health.check"}
	envelope["rollbackStrategy"] = map[string]any{"supported": false, "checkpointRequired": false}
	job := agentclient.PilotJob{JobID: "job_health", CaseID: "case_health", Release: agentclient.ReleaseExpectation{Operation: "health-check"}, Artifact: json.RawMessage("{}"), Budget: json.RawMessage("{}"), Qualification: json.RawMessage("{}")}
	decode := func() error {
		job.Envelope, _ = json.Marshal(envelope)
		job.Plan, _ = json.Marshal(plan)
		raw, _ := json.Marshal(job)
		_, err := Decode(raw)
		return err
	}
	if err := decode(); err != nil {
		t.Fatal("closed read-only health job rejected:", err)
	}
	plan["steps"] = append(plan["steps"].([]any), mutation)
	if err := decode(); err == nil {
		t.Fatal("approval-free mutation admitted")
	}
	plan["steps"] = []any{steps[0], steps[len(steps)-1]}
	envelope["requiredApprovals"] = []any{"backup"}
	if err := decode(); err == nil {
		t.Fatal("required approval omitted")
	}
	envelope["requiredApprovals"] = []any{}
	job.Release.Operation = "install"
	if err := decode(); err == nil {
		t.Fatal("approval-free install admitted")
	}
}

func TestQueuePersistsAcknowledgementRejectsConflictsAndNeverReplaysResults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "queue")
	calls := 0
	execute := func(context.Context, agentclient.PilotJob) (any, error) {
		calls++
		return map[string]string{"state": "completed"}, nil
	}
	q, err := New(root, execute)
	if err != nil {
		t.Fatal(err)
	}
	job := agentclient.PilotJob{JobID: "job_queue01", CaseID: "case_queue01", Envelope: json.RawMessage("{}"), Plan: json.RawMessage("{}"), Artifact: json.RawMessage("{}"), Budget: json.RawMessage("{}"), Qualification: json.RawMessage("{}"), Approvals: []agentclient.ApprovalReference{{ApprovalID: "appr_queue01", Action: "install"}}}
	post := func(job agentclient.PilotJob) int {
		raw, err := json.Marshal(job)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		q.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/jobs", bytes.NewReader(raw)))
		return w.Code
	}
	if got := post(job); got != 202 {
		t.Fatalf("queue status=%d", got)
	}
	if calls != 0 {
		t.Fatal("request executed before durable acknowledgement")
	}
	q.SetObserver(func(id string) (any, bool) {
		if id != job.JobID {
			t.Fatalf("observer received %s", id)
		}
		return map[string]any{"state": "running", "events": []string{"mutation-intent"}}, true
	})
	observed := httptest.NewRecorder()
	q.Handler().ServeHTTP(observed, httptest.NewRequest("GET", "/jobs/"+job.JobID, nil))
	if observed.Code != 200 || !bytes.Contains(observed.Body.Bytes(), []byte("mutation-intent")) || calls != 0 {
		t.Fatal("read-only journal observation missing or caused execution")
	}
	if got := post(job); got != 200 {
		t.Fatalf("idempotent status=%d", got)
	}
	changed := job
	changed.CaseID = "case_other"
	if got := post(changed); got != 409 {
		t.Fatalf("conflict status=%d", got)
	}
	if err := q.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("executions=%d", calls)
	}
	q, err = New(root, execute)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("completed job replayed after reopen")
	}
	w := httptest.NewRecorder()
	q.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/jobs/job_queue01", nil))
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("completed")) {
		t.Fatalf("receipt=%d %s", w.Code, w.Body.String())
	}
	invalid := job
	invalid.JobID = "../escape"
	if got := post(invalid); got != 400 {
		t.Fatalf("unsafe job ID status=%d", got)
	}
}
