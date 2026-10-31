package pilotqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
)

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
