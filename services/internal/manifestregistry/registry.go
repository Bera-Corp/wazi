// Package manifestregistry stores app manifests and lifecycle procedures
// accepted by the Phase 1 canonical signature and schema verifiers.
package manifestregistry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const (
	MaxManifestBytes   = 1 << 20
	MaxRegistryRecords = 1000
	MaxRegistryBytes   = 64 << 20
	lockFileName       = ".wazi-manifest-registry.lock"
	procedureDirName   = "procedures"
	revocationFilename = "revocation-current.json"
)

var (
	ErrInvalidRecord          = errors.New("signed record rejected")
	ErrVersionConflict        = errors.New("record version already exists with different content")
	ErrRegistryLimit          = errors.New("verified registry capacity reached")
	ErrRevocationRollback     = errors.New("revocation sequence rollback rejected")
	ErrRevocationListConflict = errors.New("revocation list identity cannot change")

	appIDPattern            = regexp.MustCompile("^[a-z0-9]+(\\.[a-z0-9-]+)+$")
	versionPattern          = regexp.MustCompile("^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)([-+][0-9A-Za-z.-]+)?$")
	procedureIDPattern      = regexp.MustCompile("^proc_[A-Za-z0-9_-]+$")
	revocationListIDPattern = regexp.MustCompile("^revlist_[A-Za-z0-9_-]+$")
)

// Entry identifies one immutable app release and its canonical record digest.
type Entry struct {
	AppID   string
	Version string
	Digest  string
}

// ProcedureEntry identifies one immutable signed lifecycle procedure.
type ProcedureEntry struct {
	ProcedureID   string
	Version       string
	ApplicationID string
	Operation     string
	Digest        string
}

type RevocationEntry struct {
	ListID   string
	Sequence int64
	Digest   string
}

type storedRecord struct {
	entry Entry
	raw   []byte
}

type storedProcedure struct {
	entry ProcedureEntry
	raw   []byte
}

type storedRevocation struct {
	entry RevocationEntry
	raw   []byte
}

// Registry persists verified app manifests in one private directory. A lock
// file prevents multiple writers; a stale lock after a crash requires
// operator inspection and removal.
type Registry struct {
	root       string
	policy     verify.Policy
	lock       *os.File
	mu         sync.RWMutex
	closed     bool
	records    map[string]storedRecord
	procedures map[string]storedProcedure
	revocation *storedRevocation
	totalSize  int64
}

// New opens or creates a private, durable registry directory and verifies
// every stored record before serving it.
func New(root string, policy verify.Policy) (*Registry, error) {
	if err := verify.ValidatePolicy(policy); err != nil {
		return nil, fmt.Errorf("invalid signing policy: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("manifest registry directory is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve manifest registry directory: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0700); err != nil {
		return nil, fmt.Errorf("create manifest registry directory: %w", err)
	}
	info, err := os.Lstat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect manifest registry directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("manifest registry path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("manifest registry directory must not be accessible by group or other users")
	}

	lockPath := filepath.Join(absoluteRoot, lockFileName)
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("manifest registry is locked; inspect and remove a stale lock only after verifying no writer is running")
		}
		return nil, fmt.Errorf("create manifest registry lock: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write manifest registry lock: %w", err)
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync manifest registry lock: %w", err)
	}
	if err := syncDirectory(absoluteRoot); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync manifest registry directory: %w", err)
	}

	registry := &Registry{
		root:       absoluteRoot,
		policy:     policy,
		lock:       lock,
		records:    map[string]storedRecord{},
		procedures: map[string]storedProcedure{},
	}
	if err := registry.load(); err != nil {
		_ = registry.Close()
		return nil, err
	}
	return registry, nil
}

// Put verifies and durably stores a canonical signed app-manifest record.
// Repeating the same record is idempotent; changing content for an existing
// (appId, version) is rejected.
func (r *Registry) Put(raw []byte) (Entry, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return Entry{}, fmt.Errorf("%w: record size must be between 1 and %d bytes", ErrInvalidRecord, MaxManifestBytes)
	}
	entry, key, err := identifyRecord(raw, r.policy)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Entry{}, fmt.Errorf("manifest registry is closed")
	}
	if existing, ok := r.records[key]; ok {
		if existing.entry.Digest != entry.Digest {
			return Entry{}, ErrVersionConflict
		}
		return existing.entry, nil
	}
	if r.recordCount() >= MaxRegistryRecords || r.totalSize+int64(len(raw)) > MaxRegistryBytes {
		return Entry{}, ErrRegistryLimit
	}
	filename := filepath.Join(r.root, recordFilename(key))
	if _, err := os.Lstat(filename); err == nil {
		return Entry{}, fmt.Errorf("unindexed manifest file already exists; registry requires operator inspection")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Entry{}, fmt.Errorf("inspect manifest registry file: %w", err)
	}
	if err := writeAtomic(r.root, filename, raw); err != nil {
		return Entry{}, fmt.Errorf("persist manifest: %w", err)
	}
	copied := append([]byte(nil), raw...)
	r.records[key] = storedRecord{entry: entry, raw: copied}
	r.totalSize += int64(len(copied))
	return entry, nil
}

// PutProcedure verifies and durably stores one canonical signed procedure.
// A procedure cannot replace a different record with the same ID/version.
func (r *Registry) PutProcedure(raw []byte) (ProcedureEntry, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return ProcedureEntry{}, fmt.Errorf("%w: procedure size must be between 1 and %d bytes", ErrInvalidRecord, MaxManifestBytes)
	}
	entry, key, err := identifyProcedure(raw, r.policy)
	if err != nil {
		return ProcedureEntry{}, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ProcedureEntry{}, fmt.Errorf("manifest registry is closed")
	}
	if existing, ok := r.procedures[key]; ok {
		if existing.entry.Digest != entry.Digest {
			return ProcedureEntry{}, ErrVersionConflict
		}
		return existing.entry, nil
	}
	if r.recordCount() >= MaxRegistryRecords || r.totalSize+int64(len(raw)) > MaxRegistryBytes {
		return ProcedureEntry{}, ErrRegistryLimit
	}
	directory := filepath.Join(r.root, procedureDirName)
	if err := ensureProcedureDirectory(directory); err != nil {
		return ProcedureEntry{}, err
	}
	filename := filepath.Join(directory, procedureRecordFilename(key))
	if _, err := os.Lstat(filename); err == nil {
		return ProcedureEntry{}, fmt.Errorf("unindexed procedure file already exists; registry requires operator inspection")
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProcedureEntry{}, fmt.Errorf("inspect procedure registry file: %w", err)
	}
	if err := writeAtomic(directory, filename, raw); err != nil {
		return ProcedureEntry{}, fmt.Errorf("persist procedure: %w", err)
	}
	r.procedures[key] = storedProcedure{entry: entry, raw: append([]byte(nil), raw...)}
	r.totalSize += int64(len(raw))
	return entry, nil
}

// PutRevocation verifies and durably replaces the one current signed
// revocation list. Its sequence must advance monotonically for a stable list
// identity; list rotation requires operator inspection and state migration.
func (r *Registry) PutRevocation(raw []byte) (RevocationEntry, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return RevocationEntry{}, fmt.Errorf("%w: revocation record size must be between 1 and %d bytes", ErrInvalidRecord, MaxManifestBytes)
	}
	entry, err := identifyRevocation(raw, r.policy)
	if err != nil {
		return RevocationEntry{}, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return RevocationEntry{}, fmt.Errorf("manifest registry is closed")
	}
	if current := r.revocation; current != nil {
		if entry.ListID != current.entry.ListID {
			return RevocationEntry{}, ErrRevocationListConflict
		}
		switch {
		case entry.Sequence < current.entry.Sequence:
			return RevocationEntry{}, ErrRevocationRollback
		case entry.Sequence == current.entry.Sequence && entry.Digest != current.entry.Digest:
			return RevocationEntry{}, ErrVersionConflict
		case entry.Sequence == current.entry.Sequence:
			return current.entry, nil
		}
	} else if r.recordCount() >= MaxRegistryRecords {
		return RevocationEntry{}, ErrRegistryLimit
	}
	oldSize := int64(0)
	if r.revocation != nil {
		oldSize = int64(len(r.revocation.raw))
	}
	if r.totalSize+int64(len(raw))-oldSize > MaxRegistryBytes {
		return RevocationEntry{}, ErrRegistryLimit
	}
	filename := filepath.Join(r.root, revocationFilename)
	if r.revocation == nil {
		if _, err := os.Lstat(filename); err == nil {
			return RevocationEntry{}, fmt.Errorf("unindexed revocation file already exists; registry requires operator inspection")
		} else if !errors.Is(err, os.ErrNotExist) {
			return RevocationEntry{}, fmt.Errorf("inspect revocation registry file: %w", err)
		}
	} else if info, err := os.Lstat(filename); err != nil {
		return RevocationEntry{}, fmt.Errorf("inspect current revocation file: %w", err)
	} else if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return RevocationEntry{}, fmt.Errorf("current revocation file must be a regular file")
	}
	if err := writeAtomic(r.root, filename, raw); err != nil {
		return RevocationEntry{}, fmt.Errorf("persist revocation record: %w", err)
	}
	r.revocation = &storedRevocation{entry: entry, raw: append([]byte(nil), raw...)}
	r.totalSize += int64(len(raw)) - oldSize
	return entry, nil
}

// GetRevocation returns a copy of the current authenticated revocation list.
func (r *Registry) GetRevocation() ([]byte, RevocationEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed || r.revocation == nil {
		return nil, RevocationEntry{}, false
	}
	return append([]byte(nil), r.revocation.raw...), r.revocation.entry, true
}

// GetProcedure returns a copy of one verified procedure by ID and version.
func (r *Registry) GetProcedure(procedureID, version string) ([]byte, ProcedureEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, ProcedureEntry{}, false
	}
	record, ok := r.procedures[procedureID+"\x00"+version]
	if !ok {
		return nil, ProcedureEntry{}, false
	}
	return append([]byte(nil), record.raw...), record.entry, true
}

// ListProcedures returns procedures in stable ID/version order.
func (r *Registry) ListProcedures() []ProcedureEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]ProcedureEntry, 0, len(r.procedures))
	if r.closed {
		return entries
	}
	for _, record := range r.procedures {
		entries = append(entries, record.entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ProcedureID == entries[j].ProcedureID {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].ProcedureID < entries[j].ProcedureID
	})
	return entries
}

// Get returns a copy of the verified record for one app release.
func (r *Registry) Get(appID, version string) ([]byte, Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, Entry{}, false
	}
	record, ok := r.records[appID+"\x00"+version]
	if !ok {
		return nil, Entry{}, false
	}
	return append([]byte(nil), record.raw...), record.entry, true
}

// List returns a stable, digest-bearing summary of all stored releases.
func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]Entry, 0, len(r.records))
	if r.closed {
		return entries
	}
	for _, record := range r.records {
		entries = append(entries, record.entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].AppID == entries[j].AppID {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].AppID < entries[j].AppID
	})
	return entries
}

// Close releases the single-writer lock. A crashed process leaves the lock
// behind deliberately; startup refuses to guess whether its owner is alive.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.lock == nil {
		return nil
	}
	lockPath := filepath.Join(r.root, lockFileName)
	closeErr := r.lock.Close()
	removeErr := os.Remove(lockPath)
	syncErr := syncDirectory(r.root)
	r.lock = nil
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil {
		return removeErr
	}
	return syncErr
}

func (r *Registry) load() error {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return fmt.Errorf("read manifest registry directory: %w", err)
	}
	for _, file := range entries {
		if file.Name() == lockFileName {
			continue
		}
		if file.Name() == procedureDirName {
			path := filepath.Join(r.root, file.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("inspect procedure registry directory: %w", err)
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("procedure registry path must be a real directory")
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
				return fmt.Errorf("procedure registry directory must not be accessible by group or other users")
			}
			continue
		}
		if file.Name() == revocationFilename {
			path := filepath.Join(r.root, file.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("inspect current revocation record: %w", err)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("current revocation record must be a regular file")
			}
			if info.Size() <= 0 || info.Size() > MaxManifestBytes {
				return fmt.Errorf("current revocation record has an invalid size")
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
				return fmt.Errorf("current revocation record must not be accessible by group or other users")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read current revocation record: %w", err)
			}
			entry, err := identifyRevocation(raw, r.policy)
			if err != nil {
				return fmt.Errorf("stored revocation record failed verification: %w", err)
			}
			if r.recordCount() >= MaxRegistryRecords {
				return fmt.Errorf("stored manifest registry exceeds configured record capacity")
			}
			r.revocation = &storedRevocation{entry: entry, raw: append([]byte(nil), raw...)}
			r.totalSize += int64(len(raw))
			if r.totalSize > MaxRegistryBytes {
				return fmt.Errorf("stored manifest registry exceeds configured capacity")
			}
			continue
		}
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			return fmt.Errorf("unexpected file %q in manifest registry; operator inspection required", file.Name())
		}
		path := filepath.Join(r.root, file.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect manifest %q: %w", file.Name(), err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("manifest %q must be a regular file", file.Name())
		}
		if info.Size() <= 0 || info.Size() > MaxManifestBytes {
			return fmt.Errorf("manifest %q has an invalid size", file.Name())
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("manifest %q must not be accessible by group or other users", file.Name())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read manifest %q: %w", file.Name(), err)
		}
		entry, key, err := identifyRecord(raw, r.policy)
		if err != nil {
			return fmt.Errorf("stored manifest %q failed verification: %w", file.Name(), err)
		}
		if file.Name() != recordFilename(key) {
			return fmt.Errorf("stored manifest %q does not match its app/version key", file.Name())
		}
		if _, duplicate := r.records[key]; duplicate {
			return fmt.Errorf("duplicate stored release %s %s", entry.AppID, entry.Version)
		}
		if r.recordCount() >= MaxRegistryRecords || r.totalSize+int64(len(raw)) > MaxRegistryBytes {
			return fmt.Errorf("stored manifest registry exceeds configured capacity")
		}
		r.records[key] = storedRecord{entry: entry, raw: append([]byte(nil), raw...)}
		r.totalSize += int64(len(raw))
	}
	return r.loadProcedures()
}

func (r *Registry) loadProcedures() error {
	directory := filepath.Join(r.root, procedureDirName)
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect procedure registry directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("procedure registry path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("procedure registry directory must not be accessible by group or other users")
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read procedure registry directory: %w", err)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			return fmt.Errorf("unexpected file %q in procedure registry; operator inspection required", file.Name())
		}
		path := filepath.Join(directory, file.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect procedure %q: %w", file.Name(), err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("procedure %q must be a regular file", file.Name())
		}
		if info.Size() <= 0 || info.Size() > MaxManifestBytes {
			return fmt.Errorf("procedure %q has an invalid size", file.Name())
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("procedure %q must not be accessible by group or other users", file.Name())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read procedure %q: %w", file.Name(), err)
		}
		entry, key, err := identifyProcedure(raw, r.policy)
		if err != nil {
			return fmt.Errorf("stored procedure %q failed verification: %w", file.Name(), err)
		}
		if file.Name() != procedureRecordFilename(key) {
			return fmt.Errorf("stored procedure %q does not match its procedure/version key", file.Name())
		}
		if _, duplicate := r.procedures[key]; duplicate {
			return fmt.Errorf("duplicate stored procedure %s %s", entry.ProcedureID, entry.Version)
		}
		if r.recordCount() >= MaxRegistryRecords || r.totalSize+int64(len(raw)) > MaxRegistryBytes {
			return fmt.Errorf("stored manifest and procedure registry exceeds configured capacity")
		}
		r.procedures[key] = storedProcedure{entry: entry, raw: append([]byte(nil), raw...)}
		r.totalSize += int64(len(raw))
	}
	return nil
}

func identifyRecord(raw []byte, policy verify.Policy) (Entry, string, error) {
	if err := verify.VerifyRecord(raw, policy); err != nil {
		return Entry{}, "", err
	}
	if err := contracts.ValidateAppManifest(raw); err != nil {
		return Entry{}, "", err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return Entry{}, "", err
	}
	record, ok := value.(map[string]any)
	if !ok {
		return Entry{}, "", fmt.Errorf("record must be a JSON object")
	}
	if record["schemaVersion"] != "1.0.0" {
		return Entry{}, "", fmt.Errorf("schemaVersion must be 1.0.0")
	}
	appID, appOK := record["appId"].(string)
	version, versionOK := record["version"].(string)
	name, nameOK := record["name"].(string)
	license, licenseOK := record["license"].(string)
	if !appOK || !appIDPattern.MatchString(appID) {
		return Entry{}, "", fmt.Errorf("appId is missing or invalid")
	}
	if !versionOK || !versionPattern.MatchString(version) {
		return Entry{}, "", fmt.Errorf("version is missing or invalid")
	}
	if !nameOK || strings.TrimSpace(name) == "" {
		return Entry{}, "", fmt.Errorf("name is required")
	}
	if !licenseOK || len(strings.TrimSpace(license)) < 2 {
		return Entry{}, "", fmt.Errorf("license is required")
	}
	for _, field := range []string{"provenance", "release", "requirements"} {
		if _, ok := record[field].(map[string]any); !ok {
			return Entry{}, "", fmt.Errorf("%s must be an object", field)
		}
	}
	digest, err := verify.CanonicalDigest(record)
	if err != nil {
		return Entry{}, "", err
	}
	entry := Entry{AppID: appID, Version: version, Digest: digest}
	return entry, appID + "\x00" + version, nil
}

func identifyProcedure(raw []byte, policy verify.Policy) (ProcedureEntry, string, error) {
	if err := verify.VerifyRecord(raw, policy); err != nil {
		return ProcedureEntry{}, "", err
	}
	if err := contracts.ValidateProcedure(raw); err != nil {
		return ProcedureEntry{}, "", err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return ProcedureEntry{}, "", err
	}
	record, ok := value.(map[string]any)
	if !ok {
		return ProcedureEntry{}, "", fmt.Errorf("procedure must be a JSON object")
	}
	procedureID, idOK := record["procedureId"].(string)
	version, versionOK := record["version"].(string)
	applicationID, appOK := record["applicationId"].(string)
	operation, operationOK := record["operation"].(string)
	if !idOK || !procedureIDPattern.MatchString(procedureID) {
		return ProcedureEntry{}, "", fmt.Errorf("procedureId is missing or invalid")
	}
	if !versionOK || strings.TrimSpace(version) == "" {
		return ProcedureEntry{}, "", fmt.Errorf("procedure version is required")
	}
	if !appOK || !appIDPattern.MatchString(applicationID) {
		return ProcedureEntry{}, "", fmt.Errorf("procedure applicationId is missing or invalid")
	}
	if !operationOK || !validProcedureOperation(operation) {
		return ProcedureEntry{}, "", fmt.Errorf("procedure operation is missing or invalid")
	}
	digest, err := verify.CanonicalDigest(record)
	if err != nil {
		return ProcedureEntry{}, "", err
	}
	entry := ProcedureEntry{
		ProcedureID:   procedureID,
		Version:       version,
		ApplicationID: applicationID,
		Operation:     operation,
		Digest:        digest,
	}
	return entry, procedureID + "\x00" + version, nil
}

func identifyRevocation(raw []byte, policy verify.Policy) (RevocationEntry, error) {
	if err := verify.VerifySignedRecord(raw, policy); err != nil {
		return RevocationEntry{}, err
	}
	record, err := contracts.DecodeRevocationRecord(raw)
	if err != nil {
		return RevocationEntry{}, err
	}
	if record.SchemaVersion != "1.0.0" {
		return RevocationEntry{}, fmt.Errorf("schemaVersion must be 1.0.0")
	}
	if !revocationListIDPattern.MatchString(record.ListID) {
		return RevocationEntry{}, fmt.Errorf("listId is missing or invalid")
	}
	if record.Sequence < 1 {
		return RevocationEntry{}, fmt.Errorf("sequence must be positive")
	}
	generatedAt, err := time.Parse(time.RFC3339, record.GeneratedAt)
	if err != nil {
		return RevocationEntry{}, fmt.Errorf("generatedAt is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, record.ExpiresAt)
	if err != nil {
		return RevocationEntry{}, fmt.Errorf("expiresAt is invalid: %w", err)
	}
	if !expiresAt.After(generatedAt) {
		return RevocationEntry{}, fmt.Errorf("expiresAt must be after generatedAt")
	}
	seen := make(map[string]bool, len(record.Entries))
	for _, item := range record.Entries {
		if !appIDPattern.MatchString(item.ApplicationID) {
			return RevocationEntry{}, fmt.Errorf("revocation entry applicationId is invalid")
		}
		if strings.TrimSpace(item.Reason) == "" {
			return RevocationEntry{}, fmt.Errorf("revocation entry reason is required")
		}
		if _, err := time.Parse(time.RFC3339, item.RevokedAt); err != nil {
			return RevocationEntry{}, fmt.Errorf("revocation entry revokedAt is invalid: %w", err)
		}
		key := item.ApplicationID + "\x00" + item.ReleaseVersion + "\x00" + item.ManifestDigest
		if seen[key] {
			return RevocationEntry{}, fmt.Errorf("revocation list contains a duplicate application/release/digest entry")
		}
		seen[key] = true
	}
	digest, err := contracts.RevocationRecordDigest(raw)
	if err != nil {
		return RevocationEntry{}, err
	}
	return RevocationEntry{ListID: record.ListID, Sequence: record.Sequence, Digest: digest}, nil
}

func (r *Registry) recordCount() int {
	count := len(r.records) + len(r.procedures)
	if r.revocation != nil {
		count++
	}
	return count
}

func validProcedureOperation(operation string) bool {
	switch operation {
	case "install", "upgrade", "backup", "restore", "rollback", "uninstall", "health-check":
		return true
	default:
		return false
	}
}

func procedureRecordFilename(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:]) + ".json"
}

func ensureProcedureDirectory(directory string) error {
	created := false
	if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create procedure registry directory: %w", err)
	} else if err == nil {
		created = true
	}
	if created {
		if err := syncDirectory(filepath.Dir(directory)); err != nil {
			return fmt.Errorf("sync procedure registry parent directory: %w", err)
		}
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect procedure registry directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("procedure registry path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("procedure registry directory must not be accessible by group or other users")
	}
	return nil
}

func recordFilename(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:]) + ".json"
}

func writeAtomic(directory, destination string, raw []byte) (returnErr error) {
	temp, err := os.CreateTemp(directory, ".manifest-pending-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
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
	if err := os.Rename(tempName, destination); err != nil {
		return err
	}
	if err := syncDirectory(directory); err != nil {
		_ = os.Remove(destination)
		_ = syncDirectory(directory)
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
