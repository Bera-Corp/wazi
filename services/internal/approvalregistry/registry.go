// Package approvalregistry stores operator-issued approval records and their
// single-use consumption state in a private, durable directory.
package approvalregistry

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
	"strings"
	"sync"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const (
	MaxApprovalBytes   = 64 << 10
	MaxApprovalRecords = 1000
	MaxRegistryBytes   = 16 << 20
	lockFileName       = ".wazi-approval-registry.lock"
)

var (
	ErrInvalidApproval = errors.New("approval record rejected")
	ErrConflict        = errors.New("approval ID already exists with different content")
	ErrCapacity        = errors.New("approval registry capacity reached")
	ErrBindingMismatch = errors.New("approval does not match the consumption request")
	ErrExpired         = errors.New("approval is expired or not yet valid")
	ErrNotApproved     = errors.New("approval decision is not approved")
	ErrAlreadyConsumed = errors.New("approval was consumed by a different job")

	approvalIDPattern = regexp.MustCompile(`^appr_[A-Za-z0-9_-]+$`)
	jobIDPattern      = regexp.MustCompile(`^job_[A-Za-z0-9_-]+$`)
)

type Entry struct {
	ApprovalID string
	CaseID     string
	PlanID     string
	PlanDigest string
	ConsumedBy string
	Digest     string
}

type ConsumptionBinding struct {
	JobID          string
	CaseID         string
	PlanID         string
	PlanDigest     string
	EnvelopeID     string
	EnvelopeDigest string
	TenantScope    string
	HostID         string
	ApplicationID  string
	Action         string
}

type diskRecord struct {
	Record     json.RawMessage `json:"record"`
	ConsumedBy string          `json:"consumedBy,omitempty"`
	ConsumedAt string          `json:"consumedAt,omitempty"`
}

type storedRecord struct {
	entry      Entry
	raw        []byte
	consumed   string
	consumedAt string
	fileSize   int64
}

type Registry struct {
	root    string
	lock    *os.File
	now     func() time.Time
	mu      sync.RWMutex
	closed  bool
	records map[string]storedRecord
	total   int64
}

func New(root string, now func() time.Time) (*Registry, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("approval registry directory is required")
	}
	if now == nil {
		now = time.Now
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve approval registry directory: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0700); err != nil {
		return nil, fmt.Errorf("create approval registry directory: %w", err)
	}
	info, err := os.Lstat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect approval registry directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("approval registry path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("approval registry directory must not be accessible by group or other users")
	}
	lockPath := filepath.Join(absoluteRoot, lockFileName)
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("approval registry is locked; inspect and remove a stale lock only after verifying no writer is running")
		}
		return nil, fmt.Errorf("create approval registry lock: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write approval registry lock: %w", err)
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync approval registry lock: %w", err)
	}
	if err := syncDirectory(absoluteRoot); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync approval registry directory: %w", err)
	}
	registry := &Registry{root: absoluteRoot, lock: lock, now: now, records: map[string]storedRecord{}}
	if err := registry.load(); err != nil {
		_ = registry.Close()
		return nil, err
	}
	return registry, nil
}

func (r *Registry) Put(raw []byte) (Entry, error) {
	if len(raw) == 0 || len(raw) > MaxApprovalBytes {
		return Entry{}, fmt.Errorf("%w: record size must be between 1 and %d bytes", ErrInvalidApproval, MaxApprovalBytes)
	}
	record, err := decodeCanonical(raw)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: %v", ErrInvalidApproval, err)
	}
	if record.Decision != "approved" && record.Decision != "denied" {
		return Entry{}, fmt.Errorf("%w: decision is invalid", ErrInvalidApproval)
	}
	if err := validateApprovalWindow(record, r.now().UTC()); err != nil {
		return Entry{}, fmt.Errorf("%w: %v", ErrInvalidApproval, err)
	}
	digest, err := contracts.ApprovalRecordDigest(raw)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: digest record: %v", ErrInvalidApproval, err)
	}
	key := record.ApprovalID
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Entry{}, fmt.Errorf("approval registry is closed")
	}
	if existing, ok := r.records[key]; ok {
		if existing.entry.Digest != digest {
			return Entry{}, ErrConflict
		}
		return existing.entry, nil
	}
	if len(r.records) >= MaxApprovalRecords {
		return Entry{}, ErrCapacity
	}
	stored := storedRecord{entry: Entry{
		ApprovalID: record.ApprovalID, CaseID: record.CaseID, PlanID: record.PlanID,
		PlanDigest: record.PlanDigest, Digest: digest,
	}, raw: append([]byte(nil), raw...)}
	fileSize, err := serializedSize(stored)
	if err != nil {
		return Entry{}, err
	}
	if r.total+fileSize > MaxRegistryBytes {
		return Entry{}, ErrCapacity
	}
	stored.fileSize = fileSize
	persistedSize, err := r.persist(key, stored)
	if err != nil {
		r.closed = true
		return Entry{}, fmt.Errorf("persist approval record: %w", err)
	}
	stored.fileSize = persistedSize
	r.records[key] = stored
	r.total += stored.fileSize
	return stored.entry, nil
}

func (r *Registry) Get(approvalID string) ([]byte, Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.records[approvalID]
	if !ok || r.closed {
		return nil, Entry{}, false
	}
	return append([]byte(nil), record.raw...), record.entry, true
}

func (r *Registry) Consume(approvalID string, binding ConsumptionBinding) ([]byte, Entry, error) {
	if !approvalIDPattern.MatchString(approvalID) || !jobIDPattern.MatchString(binding.JobID) {
		return nil, Entry{}, fmt.Errorf("approval or job identity is invalid")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, Entry{}, fmt.Errorf("approval registry is closed")
	}
	stored, ok := r.records[approvalID]
	if !ok {
		return nil, Entry{}, os.ErrNotExist
	}
	record, err := decodeCanonical(stored.raw)
	if err != nil {
		r.closed = true
		return nil, Entry{}, fmt.Errorf("stored approval failed revalidation: %w", err)
	}
	if !matches(record, binding) {
		return nil, Entry{}, ErrBindingMismatch
	}
	if record.Decision != "approved" {
		return nil, Entry{}, ErrNotApproved
	}
	if err := validateApprovalWindow(record, r.now().UTC()); err != nil {
		return nil, Entry{}, fmt.Errorf("%w: %v", ErrExpired, err)
	}
	if stored.consumed != "" {
		if stored.consumed != binding.JobID {
			return nil, Entry{}, ErrAlreadyConsumed
		}
		return append([]byte(nil), stored.raw...), stored.entry, nil
	}
	updated := stored
	updated.consumed = binding.JobID
	updated.consumedAt = r.now().UTC().Format(time.RFC3339Nano)
	updated.entry.ConsumedBy = binding.JobID
	fileSize, err := serializedSize(updated)
	if err != nil {
		r.closed = true
		return nil, Entry{}, fmt.Errorf("encode approval consumption: %w", err)
	}
	if r.total-stored.fileSize+fileSize > MaxRegistryBytes {
		return nil, Entry{}, ErrCapacity
	}
	persistedSize, err := r.persist(approvalID, updated)
	if err != nil {
		r.closed = true
		return nil, Entry{}, fmt.Errorf("persist approval consumption: %w", err)
	}
	updated.fileSize = persistedSize
	r.total = r.total - stored.fileSize + persistedSize
	r.records[approvalID] = updated
	return append([]byte(nil), stored.raw...), updated.entry, nil
}

func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed && r.lock == nil {
		return nil
	}
	r.closed = true
	if r.lock == nil {
		return nil
	}
	if err := r.lock.Close(); err != nil {
		return err
	}
	r.lock = nil
	if err := os.Remove(filepath.Join(r.root, lockFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(r.root)
}

func (r *Registry) load() error {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return fmt.Errorf("read approval registry: %w", err)
	}
	for _, item := range entries {
		if item.Name() == lockFileName {
			continue
		}
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			return fmt.Errorf("unexpected file in approval registry: %s", item.Name())
		}
		path := filepath.Join(r.root, item.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("approval registry entry is not a regular file: %s", item.Name())
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("approval registry entry is accessible by group or other users: %s", item.Name())
		}
		if info.Size() <= 0 || info.Size() > MaxApprovalBytes+4096 {
			return fmt.Errorf("approval registry entry has invalid size: %s", item.Name())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read approval registry entry %s: %w", item.Name(), err)
		}
		var disk diskRecord
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&disk); err != nil {
			return fmt.Errorf("decode approval registry entry %s: %w", item.Name(), err)
		}
		if err := ensureEOF(decoder); err != nil {
			return fmt.Errorf("decode approval registry entry %s: %w", item.Name(), err)
		}
		record, err := decodeCanonical(disk.Record)
		if err != nil {
			return fmt.Errorf("stored approval %s is invalid: %w", item.Name(), err)
		}
		if record.Decision != "approved" && record.Decision != "denied" {
			return fmt.Errorf("stored approval %s has an invalid decision", item.Name())
		}
		digest, err := contracts.ApprovalRecordDigest(disk.Record)
		if err != nil {
			return fmt.Errorf("digest stored approval %s: %w", item.Name(), err)
		}
		if item.Name() != record.ApprovalID+".json" || !approvalIDPattern.MatchString(record.ApprovalID) {
			return fmt.Errorf("approval registry filename does not match record identity: %s", item.Name())
		}
		if (disk.ConsumedBy == "") != (disk.ConsumedAt == "") {
			return fmt.Errorf("stored approval %s has incomplete consumption metadata", item.Name())
		}
		if disk.ConsumedBy != "" {
			if !jobIDPattern.MatchString(disk.ConsumedBy) {
				return fmt.Errorf("stored approval %s has invalid consumedBy", item.Name())
			}
			consumedAt, err := time.Parse(time.RFC3339Nano, disk.ConsumedAt)
			if err != nil {
				return fmt.Errorf("stored approval %s has invalid consumedAt", item.Name())
			}
			approvedAt, err := time.Parse(time.RFC3339Nano, record.ApprovedAt)
			if err != nil {
				return fmt.Errorf("stored approval %s has invalid approvedAt", item.Name())
			}
			expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
			if err != nil || consumedAt.Before(approvedAt) || !consumedAt.Before(expiresAt) || consumedAt.After(r.now().UTC()) {
				return fmt.Errorf("stored approval %s has invalid consumption interval", item.Name())
			}
		}
		stored := storedRecord{entry: Entry{
			ApprovalID: record.ApprovalID, CaseID: record.CaseID, PlanID: record.PlanID,
			PlanDigest: record.PlanDigest, ConsumedBy: disk.ConsumedBy, Digest: digest,
		}, raw: append([]byte(nil), disk.Record...), consumed: disk.ConsumedBy,
			consumedAt: disk.ConsumedAt, fileSize: int64(len(raw))}
		r.records[record.ApprovalID] = stored
		r.total += stored.fileSize
	}
	if len(r.records) > MaxApprovalRecords || r.total > MaxRegistryBytes {
		return ErrCapacity
	}
	return nil
}

func (r *Registry) persist(approvalID string, stored storedRecord) (int64, error) {
	raw, err := marshalDiskRecord(stored)
	if err != nil {
		return 0, err
	}
	temp, err := os.CreateTemp(r.root, ".approval-pending-*")
	if err != nil {
		return 0, err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return 0, err
	}
	if _, err := io.Copy(temp, bytes.NewReader(raw)); err != nil {
		_ = temp.Close()
		return 0, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return 0, err
	}
	if err := temp.Close(); err != nil {
		return 0, err
	}
	destination := filepath.Join(r.root, approvalID+".json")
	if err := os.Rename(tempName, destination); err != nil {
		return 0, err
	}
	if err := syncDirectory(r.root); err != nil {
		return 0, err
	}
	return int64(len(raw)), nil
}

func serializedSize(stored storedRecord) (int64, error) {
	raw, err := marshalDiskRecord(stored)
	return int64(len(raw)), err
}

func marshalDiskRecord(stored storedRecord) ([]byte, error) {
	disk := diskRecord{Record: append(json.RawMessage(nil), stored.raw...), ConsumedBy: stored.consumed, ConsumedAt: stored.consumedAt}
	raw, err := json.Marshal(disk)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func decodeCanonical(raw []byte) (contracts.ApprovalRecord, error) {
	var empty contracts.ApprovalRecord
	if err := contracts.ValidateApprovalRecord(raw); err != nil {
		return empty, err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return empty, err
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		return empty, err
	}
	if !bytes.Equal(raw, canonical) {
		return empty, fmt.Errorf("approval record is not canonical JSON")
	}
	return contracts.DecodeApprovalRecord(raw)
}

func validateApprovalWindow(record contracts.ApprovalRecord, now time.Time) error {
	approvedAt, err := time.Parse(time.RFC3339Nano, record.ApprovedAt)
	if err != nil {
		return fmt.Errorf("approvedAt is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("expiresAt is invalid: %w", err)
	}
	if !expiresAt.After(approvedAt) || approvedAt.After(now) || !now.Before(expiresAt) {
		return fmt.Errorf("approval is future-dated, expired, or has an invalid interval")
	}
	return nil
}

func matches(record contracts.ApprovalRecord, binding ConsumptionBinding) bool {
	return record.CaseID == binding.CaseID && record.PlanID == binding.PlanID &&
		record.PlanDigest == binding.PlanDigest && record.EnvelopeID == binding.EnvelopeID &&
		record.EnvelopeDigest == binding.EnvelopeDigest && record.TenantScope == binding.TenantScope &&
		record.HostID == binding.HostID && record.ApplicationID == binding.ApplicationID &&
		record.Action == binding.Action
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
