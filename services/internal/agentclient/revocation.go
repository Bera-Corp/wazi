package agentclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const (
	revocationStateFilename  = "revocation-state.json"
	revocationLockFilename   = ".wazi-agent-revocation.lock"
	maxRevocationStateBytes  = 4096
	maxRevocationRecordBytes = 1 << 20
)

var (
	ErrReleaseRevoked = errors.New("requested application release is revoked")

	revocationAppIDPattern  = regexp.MustCompile("^[a-z0-9]+(\\.[a-z0-9-]+)+$")
	revocationDigestPattern = regexp.MustCompile("^sha256:[a-f0-9]{64}$")
	revocationListIDPattern = regexp.MustCompile("^revlist_[A-Za-z0-9_-]+$")
)

type RevocationExpectation struct {
	ApplicationID  string
	ReleaseVersion string
	ManifestDigest string
}

// RevocationFreshness is an operator-selected acceptance bound. MaxAge must
// be positive; the verifier deliberately supplies no default freshness window.
type RevocationFreshness struct {
	Now       func() time.Time
	MaxAge    time.Duration
	ClockSkew time.Duration
}

type RevocationCursor struct {
	ListID   string
	Sequence int64
	Digest   string
}

type RevocationDecision struct {
	Cursor  RevocationCursor
	Revoked bool
	Reason  string
}

// VerifyRevocationRecord checks the signature, schema, time window,
// anti-rollback cursor, and whether the requested release appears in the
// signed list. A caller must persist the returned cursor before any mutation.
func VerifyRevocationRecord(raw []byte, policy verify.Policy, expected RevocationExpectation, current RevocationCursor, freshness RevocationFreshness) (RevocationDecision, error) {
	var decision RevocationDecision
	if len(raw) == 0 || len(raw) > maxRevocationRecordBytes {
		return decision, fmt.Errorf("revocation record size must be between 1 and %d bytes", maxRevocationRecordBytes)
	}
	if err := validateRevocationExpectation(expected); err != nil {
		return decision, err
	}
	if freshness.MaxAge <= 0 || freshness.ClockSkew < 0 {
		return decision, fmt.Errorf("revocation freshness requires a positive max age and non-negative clock skew")
	}
	if err := verify.ValidatePolicy(policy); err != nil {
		return decision, fmt.Errorf("signing policy is invalid: %w", err)
	}
	if err := verify.VerifySignedRecord(raw, policy); err != nil {
		return decision, fmt.Errorf("revocation signature rejected: %w", err)
	}
	record, err := contracts.DecodeRevocationRecord(raw)
	if err != nil {
		return decision, fmt.Errorf("revocation schema rejected: %w", err)
	}
	if !revocationListIDPattern.MatchString(record.ListID) || record.Sequence < 1 {
		return decision, fmt.Errorf("revocation list identity or sequence is invalid")
	}
	now := time.Now().UTC()
	if freshness.Now != nil {
		now = freshness.Now().UTC()
	}
	generatedAt, err := time.Parse(time.RFC3339, record.GeneratedAt)
	if err != nil {
		return decision, fmt.Errorf("revocation generatedAt is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, record.ExpiresAt)
	if err != nil {
		return decision, fmt.Errorf("revocation expiresAt is invalid: %w", err)
	}
	if !expiresAt.After(generatedAt) {
		return decision, fmt.Errorf("revocation expiresAt must be after generatedAt")
	}
	if generatedAt.After(now.Add(freshness.ClockSkew)) {
		return decision, fmt.Errorf("revocation list is future-dated")
	}
	if !now.Before(expiresAt) {
		return decision, fmt.Errorf("revocation list is expired")
	}
	if now.Sub(generatedAt) > freshness.MaxAge {
		return decision, fmt.Errorf("revocation list exceeds configured max age")
	}
	digest, err := contracts.RevocationRecordDigest(raw)
	if err != nil {
		return decision, fmt.Errorf("digest verified revocation list: %w", err)
	}
	if err := validateRevocationCursor(current); err != nil {
		return decision, err
	}
	if current.Sequence > 0 {
		if record.ListID != current.ListID {
			return decision, fmt.Errorf("revocation list identity changed; operator migration is required")
		}
		switch {
		case record.Sequence < current.Sequence:
			return decision, fmt.Errorf("revocation sequence rollback detected")
		case record.Sequence == current.Sequence && digest != current.Digest:
			return decision, fmt.Errorf("revocation sequence was reused with different content")
		}
	}

	decision.Cursor = RevocationCursor{ListID: record.ListID, Sequence: record.Sequence, Digest: digest}
	for _, entry := range record.Entries {
		if entry.ApplicationID != expected.ApplicationID {
			continue
		}
		if entry.ReleaseVersion != "" && entry.ReleaseVersion != expected.ReleaseVersion {
			continue
		}
		if entry.ManifestDigest != "" && entry.ManifestDigest != expected.ManifestDigest {
			continue
		}
		revokedAt, err := time.Parse(time.RFC3339, entry.RevokedAt)
		if err != nil {
			return RevocationDecision{}, fmt.Errorf("revocation entry revokedAt is invalid: %w", err)
		}
		if revokedAt.After(now) {
			continue
		}
		decision.Revoked = true
		decision.Reason = entry.Reason
		break
	}
	return decision, nil
}

func validateRevocationExpectation(expected RevocationExpectation) error {
	if !revocationAppIDPattern.MatchString(expected.ApplicationID) || strings.TrimSpace(expected.ReleaseVersion) == "" {
		return fmt.Errorf("revocation expectation must identify an application and release")
	}
	if !revocationDigestPattern.MatchString(expected.ManifestDigest) {
		return fmt.Errorf("revocation expectation must include a lowercase sha256 manifest digest")
	}
	return nil
}

func validateRevocationCursor(cursor RevocationCursor) error {
	if cursor.Sequence < 0 {
		return fmt.Errorf("stored revocation sequence is negative")
	}
	if cursor.Sequence == 0 {
		if cursor.ListID != "" || cursor.Digest != "" {
			return fmt.Errorf("empty revocation cursor contains list identity or digest")
		}
		return nil
	}
	if !revocationListIDPattern.MatchString(cursor.ListID) || !revocationDigestPattern.MatchString(cursor.Digest) {
		return fmt.Errorf("stored revocation cursor is invalid")
	}
	return nil
}

// FetchVerifiedReleaseWithRevocation completes the read-only trust preflight
// for one release and persists the authenticated revocation cursor before
// returning. It does not perform approval, host, runtime, or mutation checks.
func FetchVerifiedReleaseWithRevocation(ctx context.Context, baseURL string, tlsConfig *tls.Config, policy verify.Policy, expected ReleaseExpectation, freshness RevocationFreshness, state *RevocationStateStore) (VerifiedRelease, error) {
	artifacts, err := FetchVerifiedReleaseArtifactsWithRevocation(ctx, baseURL, tlsConfig, policy, expected, freshness, state)
	if err != nil {
		return VerifiedRelease{}, err
	}
	return artifacts.Release, nil
}

// FetchVerifiedReleaseArtifactsWithRevocation returns the same verified
// manifest/procedure bytes only after the current signed revocation list has
// passed freshness and anti-rollback checks and the cursor is durably stored.
func FetchVerifiedReleaseArtifactsWithRevocation(ctx context.Context, baseURL string, tlsConfig *tls.Config, policy verify.Policy, expected ReleaseExpectation, freshness RevocationFreshness, state *RevocationStateStore) (VerifiedReleaseArtifacts, error) {
	var empty VerifiedReleaseArtifacts
	if state == nil {
		return empty, fmt.Errorf("persistent revocation state store is required")
	}
	if err := requirePairedTLS(tlsConfig, "revocation-aware release fetch"); err != nil {
		return empty, err
	}
	release, err := FetchVerifiedReleaseArtifacts(ctx, baseURL, tlsConfig, policy, expected)
	if err != nil {
		return empty, err
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return empty, err
	}
	raw, err := fetchCanonicalRecord(ctx, base+"/v1/revocations/current", tlsConfig)
	if err != nil {
		return empty, fmt.Errorf("fetch revocation list: %w", err)
	}
	decision, err := state.Accept(raw, policy, RevocationExpectation{
		ApplicationID: expected.ApplicationID, ReleaseVersion: expected.ReleaseVersion,
		ManifestDigest: release.Release.ManifestDigest,
	}, freshness)
	if err != nil {
		return empty, err
	}
	if decision.Revoked {
		return empty, fmt.Errorf("%w: %s", ErrReleaseRevoked, decision.Reason)
	}
	return release, nil
}

// RevocationStateStore serializes local acceptance and durably remembers the
// highest accepted sequence. A stale lock after a crash requires operator
// inspection; the store never removes a lock it did not create.
type RevocationStateStore struct {
	root   string
	lock   *os.File
	mu     sync.Mutex
	closed bool
	cursor RevocationCursor
}

func OpenRevocationStateStore(directory string) (*RevocationStateStore, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("revocation state directory is required")
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve revocation state directory: %w", err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create revocation state directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect revocation state directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("revocation state path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("revocation state directory must not be accessible by group or other users")
	}
	lockPath := filepath.Join(root, revocationLockFilename)
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("revocation state is locked; inspect and remove a stale lock only after verifying no writer is running")
		}
		return nil, fmt.Errorf("create revocation state lock: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write revocation state lock: %w", err)
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync revocation state lock: %w", err)
	}
	if err := syncRevocationDirectory(root); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync revocation state directory: %w", err)
	}
	store := &RevocationStateStore{root: root, lock: lock}
	cursor, err := loadRevocationCursor(filepath.Join(root, revocationStateFilename))
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	store.cursor = cursor
	return store, nil
}

func (s *RevocationStateStore) Accept(raw []byte, policy verify.Policy, expected RevocationExpectation, freshness RevocationFreshness) (RevocationDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RevocationDecision{}, fmt.Errorf("revocation state store is closed")
	}
	decision, err := VerifyRevocationRecord(raw, policy, expected, s.cursor, freshness)
	if err != nil {
		return RevocationDecision{}, err
	}
	if decision.Cursor != s.cursor {
		if err := saveRevocationCursor(s.root, decision.Cursor); err != nil {
			return RevocationDecision{}, fmt.Errorf("persist accepted revocation sequence: %w", err)
		}
		s.cursor = decision.Cursor
	}
	return decision, nil
}

func (s *RevocationStateStore) Cursor() RevocationCursor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursor
}

func (s *RevocationStateStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.lock == nil {
		return nil
	}
	lockPath := filepath.Join(s.root, revocationLockFilename)
	closeErr := s.lock.Close()
	removeErr := os.Remove(lockPath)
	syncErr := syncRevocationDirectory(s.root)
	s.lock = nil
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil {
		return removeErr
	}
	return syncErr
}

func loadRevocationCursor(path string) (RevocationCursor, error) {
	var empty RevocationCursor
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, fmt.Errorf("inspect revocation state: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return empty, fmt.Errorf("revocation state must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxRevocationStateBytes {
		return empty, fmt.Errorf("revocation state size is invalid")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return empty, fmt.Errorf("revocation state must not be accessible by group or other users")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return empty, fmt.Errorf("read revocation state: %w", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return empty, fmt.Errorf("parse revocation state: %w", err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return empty, fmt.Errorf("revocation state is not canonical JSON")
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 4 {
		return empty, fmt.Errorf("revocation state has an invalid shape")
	}
	version, versionOK := fields["schemaVersion"].(string)
	listID, listOK := fields["listId"].(string)
	sequenceNumber, sequenceOK := fields["sequence"].(json.Number)
	digest, digestOK := fields["digest"].(string)
	if !versionOK || version != "1.0.0" || !listOK || !sequenceOK || !digestOK {
		return empty, fmt.Errorf("revocation state fields are invalid")
	}
	sequence, err := strconv.ParseInt(string(sequenceNumber), 10, 64)
	if err != nil {
		return empty, fmt.Errorf("revocation state sequence is invalid: %w", err)
	}
	cursor := RevocationCursor{ListID: listID, Sequence: sequence, Digest: digest}
	if err := validateRevocationCursor(cursor); err != nil || sequence < 1 {
		return empty, fmt.Errorf("revocation state cursor is invalid")
	}
	return cursor, nil
}

func saveRevocationCursor(root string, cursor RevocationCursor) error {
	if err := validateRevocationCursor(cursor); err != nil || cursor.Sequence < 1 {
		return fmt.Errorf("refusing to persist invalid revocation cursor")
	}
	raw, err := verify.CanonicalJSON(map[string]any{
		"schemaVersion": "1.0.0",
		"listId":        cursor.ListID,
		"sequence":      json.Number(strconv.FormatInt(cursor.Sequence, 10)),
		"digest":        cursor.Digest,
	})
	if err != nil {
		return err
	}
	destination := filepath.Join(root, revocationStateFilename)
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("existing revocation state must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect existing revocation state: %w", err)
	}
	temp, err := os.CreateTemp(root, ".wazi-revocation-state-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
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
	if err := os.Rename(tempPath, destination); err != nil {
		return err
	}
	return syncRevocationDirectory(root)
}

func syncRevocationDirectory(directory string) error {
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
