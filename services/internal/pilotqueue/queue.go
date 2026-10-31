// Package pilotqueue persists operator-authorized requests before acknowledging
// them. A single worker executes them; operationjournal prevents replay after a
// crash. This is a private Unix-socket development transport, not a public API.
package pilotqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const MaxJobBytes = 1 << 20

var jobPattern = regexp.MustCompile("^job_[A-Za-z0-9_-]{1,96}$")
var casePattern = regexp.MustCompile("^case_[A-Za-z0-9_-]{1,96}$")

type Execute func(context.Context, agentclient.PilotJob) (any, error)
type Queue struct {
	root    string
	execute Execute
	mu      sync.Mutex
	wake    chan struct{}
	observe func(string) (any, bool)
}

// SetObserver attaches a read-only journal view. It never changes queue state.
func (q *Queue) SetObserver(observer func(string) (any, bool)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.observe = observer
}

func New(root string, execute Execute) (*Queue, error) {
	if execute == nil {
		return nil, fmt.Errorf("queue executor is required")
	}
	for _, name := range []string{"requests", "results"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return nil, fmt.Errorf("queue directory must be private and cannot be a symlink")
		}
	}
	return &Queue{root: root, execute: execute, wake: make(chan struct{}, 1)}, nil
}

func (q *Queue) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", q.post)
	mux.HandleFunc("GET /jobs/{jobID}", q.get)
	return mux
}

func Decode(raw []byte) (agentclient.PilotJob, error) {
	var job agentclient.PilotJob
	if len(raw) == 0 || len(raw) > MaxJobBytes {
		return job, fmt.Errorf("invalid request size")
	}
	if _, err := verify.ParseJSON(raw); err != nil {
		return job, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&job); err != nil {
		return job, err
	}
	if !jobPattern.MatchString(job.JobID) || !casePattern.MatchString(job.CaseID) {
		return job, fmt.Errorf("invalid job or case identity")
	}
	if len(job.Envelope) == 0 || len(job.Plan) == 0 || len(job.Artifact) == 0 || len(job.Budget) == 0 || len(job.Qualification) == 0 || len(job.Approvals) == 0 {
		return job, fmt.Errorf("pilot requires all bound records and an explicit approval")
	}
	return job, nil
}

func (q *Queue) post(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxJobBytes))
	if err != nil {
		http.Error(w, "invalid job body", 400)
		return
	}
	job, err := Decode(raw)
	if err != nil {
		http.Error(w, "invalid typed job", 400)
		return
	}
	// Canonicalize the decoded typed object: alternative property casing or
	// whitespace cannot create distinct requests with the same meaning.
	encoded, err := json.Marshal(job)
	if err != nil {
		http.Error(w, "invalid job", 400)
		return
	}
	value, err := verify.ParseJSON(encoded)
	if err != nil {
		http.Error(w, "invalid job", 400)
		return
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		http.Error(w, "invalid job", 400)
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	path := filepath.Join(q.root, "requests", job.JobID+".json")
	previous, readErr := os.ReadFile(path)
	if readErr == nil {
		if !bytes.Equal(previous, canonical) {
			http.Error(w, "job identity already binds different records", 409)
			return
		}
		q.write(w, 200, map[string]string{"jobId": job.JobID, "state": "already-queued"})
		return
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		http.Error(w, "request store unavailable", 503)
		return
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) >= 1000 {
		http.Error(w, "request store capacity reached", 503)
		return
	}
	if err := writeAtomic(path, canonical); err != nil {
		http.Error(w, "cannot persist job", 503)
		return
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	q.write(w, 202, map[string]string{"jobId": job.JobID, "state": "queued"})
}

func (q *Queue) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("jobID")
	if !jobPattern.MatchString(id) {
		http.Error(w, "invalid job ID", 400)
		return
	}
	result, err := os.ReadFile(filepath.Join(q.root, "results", id+".json"))
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(result)
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		http.Error(w, "result store unavailable", 503)
		return
	}
	if _, err := os.Stat(filepath.Join(q.root, "requests", id+".json")); err != nil {
		http.Error(w, "job not found", 404)
		return
	}
	q.mu.Lock()
	observer := q.observe
	q.mu.Unlock()
	if observer != nil {
		if observation, found := observer(id); found {
			q.write(w, 200, map[string]any{"jobId": id, "state": "pending-or-running", "operation": observation})
			return
		}
	}
	q.write(w, 200, map[string]string{"jobId": id, "state": "pending-or-running"})
}

// Run must be called exactly once while the caller holds its durable journal
// writer lock. A completed result is never replayed; ambiguous operation intent
// is rejected by the journal instead of being automatically retried.
func (q *Queue) Run(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := q.drain(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-q.wake:
		}
	}
}

func (q *Queue) drain(ctx context.Context) error {
	q.mu.Lock()
	entries, err := os.ReadDir(filepath.Join(q.root, "requests"))
	q.mu.Unlock()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected queue entry")
		}
		path := filepath.Join(q.root, "requests", entry.Name())
		resultPath := filepath.Join(q.root, "results", entry.Name())
		if _, err := os.Stat(resultPath); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		job, err := Decode(raw)
		if err != nil || entry.Name() != job.JobID+".json" {
			return fmt.Errorf("corrupt persisted job")
		}
		result, executeErr := q.execute(ctx, job)
		state := "completed"
		detail := ""
		if executeErr != nil {
			state = "rejected-or-reconciliation-required"
			detail = executeErr.Error()
		}
		encoded, err := json.Marshal(map[string]any{"jobId": job.JobID, "caseId": job.CaseID, "state": state, "detail": detail, "operation": result, "recordedAt": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return err
		}
		if err := writeAtomic(resultPath, encoded); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAtomic(path string, raw []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
