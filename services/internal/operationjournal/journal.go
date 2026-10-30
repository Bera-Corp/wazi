// Package operationjournal records durable execution intent and recovery
// evidence. It deliberately has no capability adapter and cannot mutate a host.
package operationjournal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion         = 1
	MaxOperations         = 1000
	MaxOperationBytes     = 4 << 20
	MaxJournalBytes       = 64 << 20
	MaxEventsPerOperation = 4096
	lockFileName          = ".wazi-operation-journal.lock"
)

var (
	ErrInvalidOperation     = errors.New("operation identity rejected")
	ErrConflict             = errors.New("operation identity conflicts with an existing journal entry")
	ErrCaseAlreadyUsed      = errors.New("deployment case already has a journal entry")
	ErrCapacity             = errors.New("operation journal capacity reached")
	ErrNotFound             = errors.New("operation journal entry not found")
	ErrAlreadyStarted       = errors.New("operation has already started")
	ErrReconciliationNeeded = errors.New("operation requires resource reconciliation")
	ErrTerminal             = errors.New("operation journal entry is terminal")

	operationIDPattern = regexp.MustCompile("^op_[A-Za-z0-9_-]+$")
	jobIDPattern       = regexp.MustCompile("^job_[A-Za-z0-9_-]+$")
	caseIDPattern      = regexp.MustCompile("^case_[A-Za-z0-9_-]+$")
	planIDPattern      = regexp.MustCompile("^plan_[A-Za-z0-9_-]+$")
	envelopeIDPattern  = regexp.MustCompile("^env_[A-Za-z0-9_-]+$")
	hostIDPattern      = regexp.MustCompile("^host_[A-Za-z0-9_-]+$")
	applicationPattern = regexp.MustCompile("^[a-z0-9]+(\\.[a-z0-9-]+)+$")
	tenantPattern      = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
	digestPattern      = regexp.MustCompile("^sha256:[0-9a-f]{64}$")
	stepIDPattern      = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
	resourcePattern    = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,255}$")
	codePattern        = regexp.MustCompile("^[a-z0-9][a-z0-9._-]{0,63}$")
)

type State string

const (
	StateAccepted             State = "accepted"
	StateRunning              State = "running"
	StateReconciliationNeeded State = "reconciliation-required"
	StateCompleted            State = "completed"
	StateFailed               State = "failed"
	StateCancelled            State = "cancelled"
	StateBlocked              State = "blocked"
)

type EventKind string

const (
	EventCreated        EventKind = "created"
	EventStarted        EventKind = "started"
	EventMutationIntent EventKind = "mutation-intent"
	EventMutationResult EventKind = "mutation-result"
	EventReconciled     EventKind = "reconciled"
	EventCompleted      EventKind = "completed"
	EventFailed         EventKind = "failed"
	EventCancelled      EventKind = "cancelled"
	EventBlocked        EventKind = "blocked"
)

// Identity binds one journal entry to an approved case and exact signed inputs.
// It stores digests and identifiers only; capability parameter values and
// secrets are not accepted by this package.
type Identity struct {
	OperationID    string `json:"operationId"`
	JobID          string `json:"jobId"`
	CaseID         string `json:"caseId"`
	PlanID         string `json:"planId"`
	PlanDigest     string `json:"planDigest"`
	EnvelopeID     string `json:"envelopeId"`
	EnvelopeDigest string `json:"envelopeDigest"`
	TenantScope    string `json:"tenantScope"`
	HostID         string `json:"hostId"`
	ApplicationID  string `json:"applicationId"`
	ManifestDigest string `json:"manifestDigest"`
	Operation      string `json:"operation"`
}

// Event is an append-only audit fact. Mutation intent records a digest of
// parameters instead of their contents and must be durably appended before the
// caller invokes the matching adapter.
type Event struct {
	Sequence         uint64    `json:"sequence"`
	Kind             EventKind `json:"kind"`
	At               string    `json:"at"`
	StepID           string    `json:"stepId,omitempty"`
	Capability       string    `json:"capability,omitempty"`
	ResourceRef      string    `json:"resourceRef,omitempty"`
	ParametersDigest string    `json:"parametersDigest,omitempty"`
	Result           string    `json:"result,omitempty"`
	SnapshotDigest   string    `json:"snapshotDigest,omitempty"`
	Code             string    `json:"code,omitempty"`
}

type Operation struct {
	SchemaVersion int      `json:"schemaVersion"`
	Identity      Identity `json:"identity"`
	State         State    `json:"state"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
	Events        []Event  `json:"events"`
}

type Snapshot struct {
	Operation        Operation
	RecoveryRequired bool
	PendingIntent    *Event
}

type Journal struct {
	root         string
	lock         *os.File
	now          func() time.Time
	mu           sync.RWMutex
	closed       bool
	operations   map[string]Operation
	byJob        map[string]string
	byCase       map[string]string
	recoveryOnly map[string]bool
	total        int64
}

// New opens a private, single-writer journal directory. A stale lock is never
// removed automatically; an operator must verify that the recorded writer is
// no longer running before removing it.
func New(root string, now func() time.Time) (*Journal, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("operation journal directory is required")
	}
	if now == nil {
		now = time.Now
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve operation journal directory: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0700); err != nil {
		return nil, fmt.Errorf("create operation journal directory: %w", err)
	}
	info, err := os.Lstat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect operation journal directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("operation journal path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("operation journal directory must not be accessible by group or other users")
	}
	lockPath := filepath.Join(absoluteRoot, lockFileName)
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("operation journal is locked; inspect and remove a stale lock only after verifying no writer is running")
		}
		return nil, fmt.Errorf("create operation journal lock: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write operation journal lock: %w", err)
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync operation journal lock: %w", err)
	}
	if err := syncDirectory(absoluteRoot); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync operation journal directory: %w", err)
	}
	journal := &Journal{
		root: absoluteRoot, lock: lock, now: now,
		operations: map[string]Operation{}, byJob: map[string]string{},
		byCase: map[string]string{}, recoveryOnly: map[string]bool{},
	}
	if err := journal.load(); err != nil {
		_ = journal.Close()
		return nil, err
	}
	if err := journal.blockInterruptedOnStartup(); err != nil {
		_ = journal.Close()
		return nil, fmt.Errorf("block interrupted operations on startup: %w", err)
	}
	return journal, nil
}

// Begin durably records an accepted operation. Re-delivery of the same exact
// identity is idempotent and reports existing=true; a new job cannot reuse a
// case, even after the earlier job reached a terminal state.
func (j *Journal) Begin(identity Identity) (operation Operation, existing bool, err error) {
	if err := validateIdentity(identity); err != nil {
		return Operation{}, false, fmt.Errorf("%w: %v", ErrInvalidOperation, err)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Operation{}, false, fmt.Errorf("operation journal is closed")
	}
	if prior, ok := j.operations[identity.OperationID]; ok {
		if prior.Identity != identity {
			return Operation{}, false, ErrConflict
		}
		return cloneOperation(prior), true, nil
	}
	if priorID, ok := j.byJob[identity.JobID]; ok {
		if j.operations[priorID].Identity != identity {
			return Operation{}, false, ErrConflict
		}
		return cloneOperation(j.operations[priorID]), true, nil
	}
	if _, ok := j.byCase[identity.CaseID]; ok {
		return Operation{}, false, ErrCaseAlreadyUsed
	}
	if len(j.operations) >= MaxOperations {
		return Operation{}, false, ErrCapacity
	}
	now := j.now().UTC().Format(time.RFC3339Nano)
	created := Operation{
		SchemaVersion: SchemaVersion,
		Identity:      identity,
		State:         StateAccepted,
		CreatedAt:     now,
		UpdatedAt:     now,
		Events:        []Event{{Sequence: 1, Kind: EventCreated, At: now}},
	}
	if err := validateOperation(created); err != nil {
		return Operation{}, false, fmt.Errorf("construct journal entry: %w", err)
	}
	raw, err := json.Marshal(created)
	if err != nil {
		return Operation{}, false, fmt.Errorf("encode journal entry: %w", err)
	}
	if len(raw) > MaxOperationBytes || j.total+int64(len(raw)) > MaxJournalBytes {
		return Operation{}, false, ErrCapacity
	}
	if err := j.persist(identity.OperationID, raw); err != nil {
		j.closed = true
		return Operation{}, false, fmt.Errorf("persist journal entry: %w", err)
	}
	j.operations[identity.OperationID] = cloneOperation(created)
	j.byJob[identity.JobID] = identity.OperationID
	j.byCase[identity.CaseID] = identity.OperationID
	j.total += int64(len(raw))
	return cloneOperation(created), false, nil
}

// Start records that execution is beginning. A duplicate delivery cannot
// start an already-started job a second time.
func (j *Journal) Start(operationID string) (Operation, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Operation{}, fmt.Errorf("operation journal is closed")
	}
	current, ok := j.operations[operationID]
	if !ok {
		return Operation{}, ErrNotFound
	}
	state, pending, err := deriveState(current.Events)
	if err != nil {
		return Operation{}, err
	}
	if pending != nil {
		return Operation{}, ErrReconciliationNeeded
	}
	switch state {
	case StateAccepted:
	case StateRunning:
		return Operation{}, ErrAlreadyStarted
	case StateReconciliationNeeded:
		return Operation{}, ErrReconciliationNeeded
	default:
		return Operation{}, ErrTerminal
	}
	return j.appendLocked(operationID, Event{Kind: EventStarted})
}

// RecordIntent must succeed before the caller performs the capability
// mutation. Only typed Phase 0 mutating capabilities are journalled.
func (j *Journal) RecordIntent(operationID string, stepID, capability, resourceRef, parametersDigest string) (Operation, error) {
	if !stepIDPattern.MatchString(stepID) || !mutatingCapabilities[capability] ||
		!resourcePattern.MatchString(resourceRef) || strings.Contains(resourceRef, "..") ||
		strings.Contains(resourceRef, "//") || !digestPattern.MatchString(parametersDigest) {
		return Operation{}, fmt.Errorf("%w: mutation intent fields are invalid", ErrInvalidOperation)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Operation{}, fmt.Errorf("operation journal is closed")
	}
	current, ok := j.operations[operationID]
	if !ok {
		return Operation{}, ErrNotFound
	}
	state, pending, err := deriveState(current.Events)
	if err != nil {
		return Operation{}, err
	}
	if pending != nil || state == StateReconciliationNeeded {
		return Operation{}, ErrReconciliationNeeded
	}
	if state != StateRunning {
		if isTerminal(state) {
			return Operation{}, ErrTerminal
		}
		return Operation{}, fmt.Errorf("mutation intent requires a running operation")
	}
	for _, event := range current.Events {
		if event.Kind == EventMutationIntent && event.StepID == stepID {
			return Operation{}, ErrConflict
		}
	}
	return j.appendLocked(operationID, Event{
		Kind: EventMutationIntent, StepID: stepID, Capability: capability,
		ResourceRef: resourceRef, ParametersDigest: parametersDigest,
	})
}

// RecordOutcome records a capability result. An ambiguous result keeps the
// intent unresolved and blocks all further execution until reconciliation.
func (j *Journal) RecordOutcome(operationID, stepID, result, code string) (Operation, error) {
	if !stepIDPattern.MatchString(stepID) {
		return Operation{}, fmt.Errorf("%w: step ID is invalid", ErrInvalidOperation)
	}
	if result != "succeeded" && result != "failed" && result != "ambiguous" {
		return Operation{}, fmt.Errorf("%w: mutation result is invalid", ErrInvalidOperation)
	}
	if (result == "succeeded" && code != "") ||
		(result != "succeeded" && !codePattern.MatchString(code)) {
		return Operation{}, fmt.Errorf("%w: mutation result code is invalid", ErrInvalidOperation)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Operation{}, fmt.Errorf("operation journal is closed")
	}
	current, ok := j.operations[operationID]
	if !ok {
		return Operation{}, ErrNotFound
	}
	state, pending, err := deriveState(current.Events)
	if err != nil {
		return Operation{}, err
	}
	if state == StateReconciliationNeeded || j.recoveryOnly[operationID] {
		return Operation{}, ErrReconciliationNeeded
	}
	if state != StateRunning {
		if isTerminal(state) {
			return Operation{}, ErrTerminal
		}
		return Operation{}, fmt.Errorf("mutation outcome requires a running operation")
	}
	if pending == nil || pending.StepID != stepID {
		return Operation{}, fmt.Errorf("%w: result does not match the outstanding intent", ErrInvalidOperation)
	}
	updated, err := j.appendLocked(operationID, Event{Kind: EventMutationResult, StepID: stepID, Result: result, Code: code})
	if err == nil && result == "ambiguous" {
		j.recoveryOnly[operationID] = true
	}
	return updated, err
}

// Reconcile records an observation made by a separate recovery procedure.
// Reconciliation never resumes the job: it moves the entry to blocked so an
// operator must create a newly approved case before any further mutation.
func (j *Journal) Reconcile(operationID, stepID, observedState, snapshotDigest string) (Operation, error) {
	if !stepIDPattern.MatchString(stepID) ||
		(observedState != "present" && observedState != "absent" && observedState != "partial" && observedState != "unknown") ||
		!digestPattern.MatchString(snapshotDigest) {
		return Operation{}, fmt.Errorf("%w: reconciliation observation is invalid", ErrInvalidOperation)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Operation{}, fmt.Errorf("operation journal is closed")
	}
	current, ok := j.operations[operationID]
	if !ok {
		return Operation{}, ErrNotFound
	}
	state, pending, err := deriveState(current.Events)
	if err != nil {
		return Operation{}, err
	}
	if pending == nil || (state != StateRunning && state != StateReconciliationNeeded) {
		return Operation{}, fmt.Errorf("operation has no unresolved mutation to reconcile")
	}
	if pending.StepID != stepID {
		return Operation{}, fmt.Errorf("%w: reconciliation step does not match outstanding intent", ErrInvalidOperation)
	}
	updated, err := j.appendLocked(operationID, Event{
		Kind: EventReconciled, StepID: stepID, Result: observedState, SnapshotDigest: snapshotDigest,
	})
	if err == nil {
		delete(j.recoveryOnly, operationID)
	}
	return updated, err
}

// Complete records successful completion after the executor has produced its
// required evidence. The journal does not itself evaluate lifecycle evidence.
func (j *Journal) Complete(operationID string) (Operation, error) {
	return j.finish(operationID, Event{Kind: EventCompleted})
}

// Fail records a terminal failure code without persisting raw error text.
func (j *Journal) Fail(operationID, code string) (Operation, error) {
	if !codePattern.MatchString(code) {
		return Operation{}, fmt.Errorf("%w: failure code is invalid", ErrInvalidOperation)
	}
	return j.finish(operationID, Event{Kind: EventFailed, Code: code})
}

// Cancel is permitted only when no mutation intent is unresolved.
func (j *Journal) Cancel(operationID, code string) (Operation, error) {
	if !codePattern.MatchString(code) {
		return Operation{}, fmt.Errorf("%w: cancellation code is invalid", ErrInvalidOperation)
	}
	return j.finish(operationID, Event{Kind: EventCancelled, Code: code})
}

// Block stops an accepted or running operation after a policy, revocation, or
// host revalidation failure. An unresolved mutation must be reconciled first.
func (j *Journal) Block(operationID, reasonCode string) (Operation, error) {
	if !codePattern.MatchString(reasonCode) {
		return Operation{}, fmt.Errorf("%w: block reason is invalid", ErrInvalidOperation)
	}
	return j.finish(operationID, Event{Kind: EventBlocked, Code: reasonCode})
}

func (j *Journal) finish(operationID string, event Event) (Operation, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Operation{}, fmt.Errorf("operation journal is closed")
	}
	current, ok := j.operations[operationID]
	if !ok {
		return Operation{}, ErrNotFound
	}
	state, pending, err := deriveState(current.Events)
	if err != nil {
		return Operation{}, err
	}
	if pending != nil || state == StateReconciliationNeeded {
		return Operation{}, ErrReconciliationNeeded
	}
	switch event.Kind {
	case EventCompleted, EventFailed:
		if state != StateRunning {
			if isTerminal(state) {
				return Operation{}, ErrTerminal
			}
			return Operation{}, fmt.Errorf("operation completion requires a running operation")
		}
	case EventCancelled, EventBlocked:
		if state != StateAccepted && state != StateRunning {
			if isTerminal(state) {
				return Operation{}, ErrTerminal
			}
			return Operation{}, fmt.Errorf("operation cannot be stopped from state %s", state)
		}
	default:
		return Operation{}, fmt.Errorf("unsupported terminal event %s", event.Kind)
	}
	return j.appendLocked(operationID, event)
}

func (j *Journal) appendLocked(operationID string, event Event) (Operation, error) {
	current := j.operations[operationID]
	updated := cloneOperation(current)
	now := j.now().UTC()
	if now.Before(mustParseTime(current.UpdatedAt)) {
		return Operation{}, fmt.Errorf("operation journal clock moved backwards")
	}
	event.Sequence = uint64(len(updated.Events) + 1)
	event.At = now.Format(time.RFC3339Nano)
	if len(updated.Events) >= MaxEventsPerOperation {
		return Operation{}, ErrCapacity
	}
	updated.Events = append(updated.Events, event)
	updated.UpdatedAt = event.At
	state, _, err := deriveState(updated.Events)
	if err != nil {
		return Operation{}, err
	}
	updated.State = state
	if err := validateOperation(updated); err != nil {
		return Operation{}, fmt.Errorf("journal transition rejected: %w", err)
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return Operation{}, fmt.Errorf("encode journal update: %w", err)
	}
	currentSize := len(mustMarshal(current))
	if len(raw) > MaxOperationBytes || j.total-int64(currentSize)+int64(len(raw)) > MaxJournalBytes {
		return Operation{}, ErrCapacity
	}
	if err := j.persist(operationID, raw); err != nil {
		j.closed = true
		return Operation{}, fmt.Errorf("persist journal update: %w", err)
	}
	j.total = j.total - int64(currentSize) + int64(len(raw))
	j.operations[operationID] = cloneOperation(updated)
	return cloneOperation(updated), nil
}

func (j *Journal) Get(operationID string) (Snapshot, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return Snapshot{}, false
	}
	operation, ok := j.operations[operationID]
	if !ok {
		return Snapshot{}, false
	}
	return snapshot(operation), true
}

// GetByJob resolves the idempotency key used by delivery. Its return does not
// authorize starting or resuming the operation.
func (j *Journal) GetByJob(jobID string) (Snapshot, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return Snapshot{}, false
	}
	operationID, ok := j.byJob[jobID]
	if !ok {
		return Snapshot{}, false
	}
	return snapshot(j.operations[operationID]), true
}

// ListRecoveryRequired returns every interrupted or ambiguous operation in a
// stable order. Callers must inspect owned resources and then record
// Reconcile; this method never retries a capability.
func (j *Journal) ListRecoveryRequired() []Snapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return nil
	}
	ids := make([]string, 0, len(j.operations))
	for id, operation := range j.operations {
		state, pending, err := deriveState(operation.Events)
		if err == nil && (state == StateReconciliationNeeded || pending != nil) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	result := make([]Snapshot, 0, len(ids))
	for _, id := range ids {
		result = append(result, snapshot(j.operations[id]))
	}
	return result
}

func snapshot(operation Operation) Snapshot {
	copy := cloneOperation(operation)
	state, pending, _ := deriveState(copy.Events)
	result := Snapshot{Operation: copy, RecoveryRequired: state == StateReconciliationNeeded || pending != nil}
	if pending != nil {
		intentCopy := *pending
		result.PendingIntent = &intentCopy
	}
	return result
}

func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.lock == nil {
		j.closed = true
		return nil
	}
	j.closed = true
	if err := j.lock.Close(); err != nil {
		return err
	}
	j.lock = nil
	if err := os.Remove(filepath.Join(j.root, lockFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(j.root)
}

func (j *Journal) load() error {
	entries, err := os.ReadDir(j.root)
	if err != nil {
		return fmt.Errorf("read operation journal: %w", err)
	}
	for _, item := range entries {
		if item.Name() == lockFileName {
			continue
		}
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			return fmt.Errorf("unexpected file in operation journal: %s", item.Name())
		}
		path := filepath.Join(j.root, item.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("operation journal entry is not a regular file: %s", item.Name())
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("operation journal entry is accessible by group or other users: %s", item.Name())
		}
		if info.Size() <= 0 || info.Size() > MaxOperationBytes {
			return fmt.Errorf("operation journal entry has invalid size: %s", item.Name())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read operation journal entry %s: %w", item.Name(), err)
		}
		var operation Operation
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&operation); err != nil {
			return fmt.Errorf("decode operation journal entry %s: %w", item.Name(), err)
		}
		if err := ensureEOF(decoder); err != nil {
			return fmt.Errorf("decode operation journal entry %s: %w", item.Name(), err)
		}
		if err := validateOperation(operation); err != nil {
			return fmt.Errorf("stored operation journal entry %s is invalid: %w", item.Name(), err)
		}
		if item.Name() != operation.Identity.OperationID+".json" {
			return fmt.Errorf("operation journal filename does not match operation identity: %s", item.Name())
		}
		if _, exists := j.operations[operation.Identity.OperationID]; exists {
			return fmt.Errorf("duplicate operation identity: %s", operation.Identity.OperationID)
		}
		if _, exists := j.byJob[operation.Identity.JobID]; exists {
			return fmt.Errorf("duplicate job identity: %s", operation.Identity.JobID)
		}
		if _, exists := j.byCase[operation.Identity.CaseID]; exists {
			return fmt.Errorf("duplicate case identity: %s", operation.Identity.CaseID)
		}
		j.operations[operation.Identity.OperationID] = cloneOperation(operation)
		j.byJob[operation.Identity.JobID] = operation.Identity.OperationID
		j.byCase[operation.Identity.CaseID] = operation.Identity.OperationID
		state, pending, _ := deriveState(operation.Events)
		if state == StateReconciliationNeeded || pending != nil {
			j.recoveryOnly[operation.Identity.OperationID] = true
		}
		j.total += int64(len(raw))
	}
	if len(j.operations) > MaxOperations || j.total > MaxJournalBytes {
		return ErrCapacity
	}
	return nil
}

// blockInterruptedOnStartup prevents a restarted process from continuing an
// operation that had started but had no outstanding typed mutation intent.
// Entries with an unresolved intent remain available for explicit resource
// reconciliation instead.
func (j *Journal) blockInterruptedOnStartup() error {
	ids := make([]string, 0, len(j.operations))
	for id, operation := range j.operations {
		state, pending, err := deriveState(operation.Events)
		if err != nil {
			return err
		}
		if state == StateRunning && pending == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := j.appendLocked(id, Event{Kind: EventBlocked, Code: "process.restarted"}); err != nil {
			return err
		}
	}
	return nil
}

func (j *Journal) persist(operationID string, raw []byte) error {
	temp, err := os.CreateTemp(j.root, ".operation-pending-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := io.Copy(temp, bytes.NewReader(raw)); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	destination := filepath.Join(j.root, operationID+".json")
	if err := os.Rename(tempName, destination); err != nil {
		return err
	}
	return syncDirectory(j.root)
}

func validateIdentity(identity Identity) error {
	if !operationIDPattern.MatchString(identity.OperationID) ||
		!jobIDPattern.MatchString(identity.JobID) ||
		!caseIDPattern.MatchString(identity.CaseID) ||
		!planIDPattern.MatchString(identity.PlanID) ||
		!envelopeIDPattern.MatchString(identity.EnvelopeID) ||
		!hostIDPattern.MatchString(identity.HostID) ||
		!applicationPattern.MatchString(identity.ApplicationID) ||
		!tenantPattern.MatchString(identity.TenantScope) ||
		!digestPattern.MatchString(identity.PlanDigest) ||
		!digestPattern.MatchString(identity.EnvelopeDigest) ||
		!digestPattern.MatchString(identity.ManifestDigest) {
		return fmt.Errorf("one or more IDs, digests, or scopes are malformed")
	}
	switch identity.Operation {
	case "install", "upgrade", "backup", "restore", "rollback", "uninstall", "health-check":
		return nil
	default:
		return fmt.Errorf("operation type is unsupported")
	}
}

var mutatingCapabilities = map[string]bool{
	"compose.pull":    true,
	"compose.up":      true,
	"compose.down":    true,
	"volume.backup":   true,
	"volume.restore":  true,
	"proxy.configure": true,
	"proxy.remove":    true,
	"secret.inject":   true,
}

func validateOperation(operation Operation) error {
	if operation.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported journal schema version %d", operation.SchemaVersion)
	}
	if err := validateIdentity(operation.Identity); err != nil {
		return err
	}
	if len(operation.Events) == 0 || len(operation.Events) > MaxEventsPerOperation {
		return fmt.Errorf("journal event count is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, operation.CreatedAt)
	if err != nil {
		return fmt.Errorf("createdAt is invalid: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, operation.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updatedAt is invalid: %w", err)
	}
	state, _, err := deriveState(operation.Events)
	if err != nil {
		return err
	}
	firstAt, err := time.Parse(time.RFC3339Nano, operation.Events[0].At)
	if err != nil || !createdAt.Equal(firstAt) {
		return fmt.Errorf("createdAt does not match the first journal event")
	}
	lastAt, err := time.Parse(time.RFC3339Nano, operation.Events[len(operation.Events)-1].At)
	if err != nil || !updatedAt.Equal(lastAt) {
		return fmt.Errorf("updatedAt does not match the last journal event")
	}
	if operation.State != state {
		return fmt.Errorf("stored state %q does not match event-derived state %q", operation.State, state)
	}
	return nil
}

func deriveState(events []Event) (State, *Event, error) {
	state := State("")
	var pending *Event
	usedSteps := map[string]bool{}
	var previousAt time.Time
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			return "", nil, fmt.Errorf("event sequence is not contiguous")
		}
		at, err := time.Parse(time.RFC3339Nano, event.At)
		if err != nil {
			return "", nil, fmt.Errorf("event timestamp is invalid: %w", err)
		}
		if index > 0 && at.Before(previousAt) {
			return "", nil, fmt.Errorf("event timestamps moved backwards")
		}
		previousAt = at
		if index == 0 {
			if event.Kind != EventCreated || !eventFieldsEmpty(event) {
				return "", nil, fmt.Errorf("first event must be an empty created event")
			}
			state = StateAccepted
			continue
		}
		switch event.Kind {
		case EventStarted:
			if state != StateAccepted || !eventFieldsEmpty(event) {
				return "", nil, fmt.Errorf("started event is invalid from state %s", state)
			}
			state = StateRunning
		case EventMutationIntent:
			if state != StateRunning || pending != nil || !validIntentEvent(event) || usedSteps[event.StepID] {
				return "", nil, fmt.Errorf("mutation intent is invalid")
			}
			pending = &event
			usedSteps[event.StepID] = true
		case EventMutationResult:
			if state != StateRunning || pending == nil || pending.StepID != event.StepID ||
				!validResultEvent(event) {
				return "", nil, fmt.Errorf("mutation result does not match an outstanding intent")
			}
			if event.Result == "ambiguous" {
				state = StateReconciliationNeeded
			} else {
				pending = nil
			}
		case EventReconciled:
			if (state != StateRunning && state != StateReconciliationNeeded) ||
				pending == nil || pending.StepID != event.StepID || !validReconciledEvent(event) {
				return "", nil, fmt.Errorf("reconciliation event does not match an unresolved intent")
			}
			pending = nil
			state = StateBlocked
		case EventCompleted:
			if state != StateRunning || pending != nil || !eventFieldsEmpty(event) {
				return "", nil, fmt.Errorf("completed event is invalid from state %s", state)
			}
			state = StateCompleted
		case EventFailed:
			if state != StateRunning || pending != nil || !validCodeEvent(event) {
				return "", nil, fmt.Errorf("failed event is invalid from state %s", state)
			}
			state = StateFailed
		case EventCancelled:
			if (state != StateAccepted && state != StateRunning) || pending != nil || !validCodeEvent(event) {
				return "", nil, fmt.Errorf("cancelled event is invalid from state %s", state)
			}
			state = StateCancelled
		case EventBlocked:
			if (state != StateAccepted && state != StateRunning) || pending != nil || !validCodeEvent(event) {
				return "", nil, fmt.Errorf("blocked event is invalid from state %s", state)
			}
			state = StateBlocked
		default:
			return "", nil, fmt.Errorf("unknown journal event kind %q", event.Kind)
		}
		if isTerminal(state) && index != len(events)-1 {
			return "", nil, fmt.Errorf("journal contains events after terminal state")
		}
	}
	if state == "" {
		return "", nil, fmt.Errorf("journal has no state")
	}
	return state, pending, nil
}

func validIntentEvent(event Event) bool {
	return stepIDPattern.MatchString(event.StepID) && mutatingCapabilities[event.Capability] &&
		resourcePattern.MatchString(event.ResourceRef) && !strings.Contains(event.ResourceRef, "..") &&
		!strings.Contains(event.ResourceRef, "//") && digestPattern.MatchString(event.ParametersDigest) &&
		event.Result == "" && event.SnapshotDigest == "" && event.Code == ""
}

func validResultEvent(event Event) bool {
	if !stepIDPattern.MatchString(event.StepID) || event.Capability != "" || event.ResourceRef != "" ||
		event.ParametersDigest != "" || event.SnapshotDigest != "" {
		return false
	}
	switch event.Result {
	case "succeeded":
		return event.Code == ""
	case "failed", "ambiguous":
		return codePattern.MatchString(event.Code)
	default:
		return false
	}
}

func validReconciledEvent(event Event) bool {
	if !stepIDPattern.MatchString(event.StepID) || event.Capability != "" || event.ResourceRef != "" ||
		event.ParametersDigest != "" || !digestPattern.MatchString(event.SnapshotDigest) || event.Code != "" {
		return false
	}
	return event.Result == "present" || event.Result == "absent" ||
		event.Result == "partial" || event.Result == "unknown"
}

func validCodeEvent(event Event) bool {
	return event.Code != "" && codePattern.MatchString(event.Code) &&
		event.StepID == "" && event.Capability == "" && event.ResourceRef == "" &&
		event.ParametersDigest == "" && event.Result == "" && event.SnapshotDigest == ""
}

func eventFieldsEmpty(event Event) bool {
	return event.StepID == "" && event.Capability == "" && event.ResourceRef == "" &&
		event.ParametersDigest == "" && event.Result == "" && event.SnapshotDigest == "" && event.Code == ""
}

func lastUnresolvedIntent(events []Event) *Event {
	var pending *Event
	for _, event := range events {
		switch event.Kind {
		case EventMutationIntent:
			copy := event
			pending = &copy
		case EventMutationResult:
			if event.Result != "ambiguous" {
				pending = nil
			}
		case EventReconciled:
			pending = nil
		}
	}
	return pending
}

func isTerminal(state State) bool {
	return state == StateCompleted || state == StateFailed || state == StateCancelled || state == StateBlocked
}

func cloneOperation(operation Operation) Operation {
	copy := operation
	copy.Events = append([]Event(nil), operation.Events...)
	return copy
}

func mustMarshal(operation Operation) []byte {
	raw, _ := json.Marshal(operation)
	return raw
}

func mustParseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected second JSON value")
		}
		return err
	}
	return nil
}

func syncDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
