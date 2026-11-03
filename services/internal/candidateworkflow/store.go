// Package candidateworkflow stores tenant-scoped unsigned candidates and
// digest-bound qualification, review, and promotion evidence.
package candidateworkflow

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const (
	maxCandidates            = 1000
	maxStoreBytes            = 64 << 20
	maxCandidateBytes        = 1 << 20
	maxQualificationBytes    = 64 << 10
	maxQualificationAttempts = 16
	// Import, up to sixteen qualifications, review and promotion.
	maxCandidateSnapshots = maxQualificationAttempts + 3
)

var (
	candidateIDPattern     = regexp.MustCompile("^cand_[0-9a-f]{32}$")
	digestPattern          = regexp.MustCompile("^sha256:[0-9a-f]{64}$")
	sourceIDPattern        = regexp.MustCompile("^[^\\s]+$")
	qualificationIDPattern = regexp.MustCompile("^qual_[A-Za-z0-9_-]{1,96}$")
	lockName               = ".wazi-candidate-workflow.lock"
	secretAssignment       = regexp.MustCompile("(?i)[\\\"']?(password|passwd|secret|token|api[_-]?key)[\\\"']?\\s*[:=]\\s*[\\\"']?([^\\s\\\"',}]{8,})")
	credentialPatterns     = []*regexp.Regexp{
		regexp.MustCompile("gh[pousr]_[A-Za-z0-9]{10,}"),
		regexp.MustCompile("AKIA[0-9A-Z]{16}"),
		regexp.MustCompile("-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY"),
		regexp.MustCompile("(?i)Bearer\\s+[A-Za-z0-9._-]{16,}"),
	}
)

type Source struct {
	SourceID    string `json:"sourceId"`
	URL         string `json:"url"`
	ContentHash string `json:"contentHash"`
}

type Origin struct {
	Assistant     string `json:"assistant"`
	RequestDigest string `json:"requestDigest"`
}

type ImportRequest struct {
	Manifest  json.RawMessage `json:"manifest"`
	Sources   []Source        `json:"sources"`
	Citations []string        `json:"citations"`
	Origin    Origin          `json:"origin"`
}

type Review struct {
	ReviewerID string `json:"reviewerId"`
	ReviewedAt string `json:"reviewedAt"`
}

type Qualification struct {
	QualificationID string          `json:"qualificationId"`
	Digest          string          `json:"digest"`
	Result          string          `json:"result"`
	Receipt         json.RawMessage `json:"receipt"`
}

type Candidate struct {
	CandidateID          string          `json:"candidateId"`
	TenantScope          string          `json:"tenantScope"`
	CandidateDigest      string          `json:"candidateDigest"`
	PacketDigest         string          `json:"packetDigest"`
	Revision             uint64          `json:"revision"`
	State                string          `json:"state"`
	CreatedAt            string          `json:"createdAt"`
	UpdatedAt            string          `json:"updatedAt"`
	Manifest             json.RawMessage `json:"manifest"`
	Sources              []Source        `json:"sources"`
	Citations            []string        `json:"citations"`
	Origin               Origin          `json:"origin"`
	Qualification        *Qualification  `json:"qualification,omitempty"`
	QualificationHistory []Qualification `json:"qualificationHistory,omitempty"`
	Review               *Review         `json:"review,omitempty"`
	SignedManifestDigest string          `json:"signedManifestDigest,omitempty"`
}

type Store struct {
	root                 string
	policy               verify.Policy
	qualificationSigners map[string]bool
	lock                 *os.File
	mu                   sync.RWMutex
	records              map[string]Candidate
	totalBytes           int64
	closed               bool
}

func Open(root string, policy verify.Policy, qualificationSignerIDs ...string) (*Store, error) {
	if err := verify.ValidatePolicy(policy); err != nil {
		return nil, fmt.Errorf("candidate signing policy invalid: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("candidate store directory required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("candidate store must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("candidate store must be private to the service identity")
	}
	lock, err := os.OpenFile(filepath.Join(absolute, lockName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("candidate store is locked; inspect a stale lock only after verifying no writer is running")
		}
		return nil, err
	}
	if _, err := fmt.Fprintf(lock, "%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(filepath.Join(absolute, lockName))
		return nil, err
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		_ = os.Remove(filepath.Join(absolute, lockName))
		return nil, err
	}
	qualificationSigners := map[string]bool{}
	for _, signerID := range qualificationSignerIDs {
		if _, trusted := policy.TrustedSigners[signerID]; !trusted || qualificationSigners[signerID] {
			_ = lock.Close()
			_ = os.Remove(filepath.Join(absolute, lockName))
			return nil, fmt.Errorf("qualification signer list must contain unique trusted signer IDs")
		}
		qualificationSigners[signerID] = true
	}
	store := &Store{root: absolute, policy: policy, qualificationSigners: qualificationSigners, lock: lock, records: map[string]Candidate{}}
	if err := store.load(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.lock == nil {
		return nil
	}
	_ = s.lock.Close()
	err := os.Remove(filepath.Join(s.root, lockName))
	s.lock = nil
	return err
}

func (s *Store) load() error {
	files, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	type item struct {
		name, id string
		revision uint64
	}
	filePattern := regexp.MustCompile("^(cand_[0-9a-f]{32})\\.([0-9]{6})\\.json$")
	var snapshots []item
	for _, file := range files {
		match := filePattern.FindStringSubmatch(file.Name())
		if match == nil {
			if file.Name() == lockName {
				continue
			}
			return fmt.Errorf("unexpected entry in candidate store")
		}
		revision, err := strconv.ParseUint(match[2], 10, 64)
		if err != nil || revision == 0 {
			return fmt.Errorf("candidate snapshot path invalid")
		}
		snapshots = append(snapshots, item{file.Name(), match[1], revision})
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].id != snapshots[j].id {
			return snapshots[i].id < snapshots[j].id
		}
		return snapshots[i].revision < snapshots[j].revision
	})
	if len(snapshots) > maxCandidates*maxCandidateSnapshots {
		return fmt.Errorf("candidate workflow snapshot limit exceeded")
	}
	for _, file := range snapshots {
		path := filepath.Join(s.root, file.name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			(runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) || info.Size() > 2*maxCandidateBytes {
			return fmt.Errorf("candidate snapshot is not a protected regular file")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var record Candidate
		if err := strictJSON(raw, &record); err != nil {
			return fmt.Errorf("candidate snapshot invalid: %w", err)
		}
		if record.CandidateID != file.id || record.Revision != file.revision {
			return fmt.Errorf("candidate snapshot identity mismatch")
		}
		previous, exists := s.records[file.id]
		if (!exists && record.Revision != 1) || (exists && record.Revision != previous.Revision+1) {
			return fmt.Errorf("candidate snapshot sequence mismatch")
		}
		if err := s.validateStored(record); err != nil {
			return fmt.Errorf("candidate snapshot rejected: %w", err)
		}
		if exists && !validTransition(previous, record) {
			return fmt.Errorf("candidate snapshot transition invalid")
		}
		s.records[file.id] = record
		s.totalBytes += int64(len(raw))
		if len(s.records) > maxCandidates || s.totalBytes > maxStoreBytes {
			return fmt.Errorf("candidate store capacity exceeded")
		}
	}
	return nil
}

func (s *Store) Import(tenant string, request ImportRequest, now time.Time) (Candidate, error) {
	var empty Candidate
	if !validTenant(tenant) {
		return empty, fmt.Errorf("candidate tenant scope unavailable")
	}
	manifest, digest, err := candidateManifest(request.Manifest)
	if err != nil {
		return empty, err
	}
	if len(request.Sources) == 0 || len(request.Sources) > 64 || len(request.Citations) == 0 || len(request.Citations) > 64 {
		return empty, fmt.Errorf("candidate source evidence required")
	}
	sourceIDs := map[string]bool{}
	for _, source := range request.Sources {
		if err := validateSource(source); err != nil || sourceIDs[source.SourceID] {
			return empty, fmt.Errorf("candidate source metadata invalid")
		}
		if scanCandidateSecrets(source.SourceID + " " + source.URL) {
			return empty, fmt.Errorf("candidate source metadata contains a secret pattern")
		}
		sourceIDs[source.SourceID] = true
	}
	citations := map[string]bool{}
	for _, citation := range request.Citations {
		if !sourceIDs[citation] || citations[citation] {
			return empty, fmt.Errorf("candidate citation is missing or duplicated")
		}
		citations[citation] = true
	}
	if !provenanceSourceMatches(manifest, request.Sources, citations) {
		return empty, fmt.Errorf("candidate provenance is not bound to a cited source digest")
	}
	if request.Origin.Assistant != "Codex chat" && request.Origin.Assistant != "manual" {
		return empty, fmt.Errorf("candidate origin is invalid")
	}
	if request.Origin.Assistant == "Codex chat" && !digestPattern.MatchString(request.Origin.RequestDigest) {
		return empty, fmt.Errorf("assisted candidate request digest required")
	}
	if request.Origin.Assistant == "manual" && request.Origin.RequestDigest != "" && !digestPattern.MatchString(request.Origin.RequestDigest) {
		return empty, fmt.Errorf("candidate request digest invalid")
	}
	manifestValue, _ := verify.ParseJSON(manifest)
	packetDigest, err := verify.CanonicalDigest(map[string]any{
		"manifest": manifestValue, "sources": sourcesAsAny(request.Sources),
		"citations": stringsAsAny(request.Citations),
		"origin":    map[string]any{"assistant": request.Origin.Assistant, "requestDigest": request.Origin.RequestDigest},
	})
	if err != nil {
		return empty, err
	}
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return empty, err
	}
	now = now.UTC()
	record := Candidate{
		CandidateID: "cand_" + hex.EncodeToString(randomID[:]), TenantScope: tenant,
		CandidateDigest: digest, PacketDigest: packetDigest, Revision: 1, State: "candidate",
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
		Manifest: manifest, Sources: append([]Source(nil), request.Sources...),
		Citations: append([]string(nil), request.Citations...), Origin: request.Origin,
	}
	if err := s.commit(record, false); err != nil {
		return empty, err
	}
	return cloneCandidate(record), nil
}

func (s *Store) Get(tenant, id string) (Candidate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	if s.closed || !ok || record.TenantScope != tenant {
		return Candidate{}, false
	}
	return cloneCandidate(record), true
}

func (s *Store) List(tenant string) []Candidate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Candidate, 0)
	if s.closed {
		return out
	}
	for _, record := range s.records {
		if record.TenantScope == tenant {
			out = append(out, summary(record))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *Store) RecordQualification(tenant, id, expectedDigest string, receiptRaw []byte, now time.Time) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if s.closed || !ok || record.TenantScope != tenant {
		return Candidate{}, fmt.Errorf("candidate not found")
	}
	if (record.State != "candidate" && record.State != "failed") || expectedDigest != record.CandidateDigest {
		return Candidate{}, fmt.Errorf("candidate digest or state changed")
	}
	existingAttempts := len(record.QualificationHistory)
	if record.Qualification != nil {
		existingAttempts++
	}
	if existingAttempts >= maxQualificationAttempts {
		return Candidate{}, fmt.Errorf("candidate qualification attempt limit exceeded")
	}
	receipt, qualificationID, digest, result, err := validateQualification(receiptRaw, tenant, expectedDigest, s.policy, s.qualificationSigners)
	if err != nil {
		return Candidate{}, err
	}
	if record.Qualification != nil {
		record.QualificationHistory = append(record.QualificationHistory, *record.Qualification)
	}
	record.Qualification = &Qualification{QualificationID: qualificationID, Digest: digest, Result: result, Receipt: receipt}
	record.State = "tested"
	if result == "failed" {
		record.State = "failed"
	}
	record.Revision++
	record.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	if err := s.commitLocked(record, true); err != nil {
		return Candidate{}, err
	}
	return cloneCandidate(record), nil
}

func (s *Store) Review(tenant, id, expectedDigest, reviewer string, now time.Time) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if s.closed || !ok || record.TenantScope != tenant {
		return Candidate{}, fmt.Errorf("candidate not found")
	}
	if record.State != "tested" || expectedDigest != record.CandidateDigest || reviewer == "" {
		return Candidate{}, fmt.Errorf("candidate digest, qualification, or state changed")
	}
	record.Review = &Review{ReviewerID: reviewer, ReviewedAt: now.UTC().Format(time.RFC3339Nano)}
	record.State = "reviewed"
	record.Revision++
	record.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	if err := s.commitLocked(record, true); err != nil {
		return Candidate{}, err
	}
	return cloneCandidate(record), nil
}

func (s *Store) MarkPromoted(tenant, id, expectedDigest, signedDigest string, now time.Time) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if s.closed || !ok || record.TenantScope != tenant {
		return Candidate{}, fmt.Errorf("candidate not found")
	}
	if expectedDigest != record.CandidateDigest || !digestPattern.MatchString(signedDigest) {
		return Candidate{}, fmt.Errorf("candidate digest or review state changed")
	}
	if record.State == "signed" && record.SignedManifestDigest == signedDigest {
		return cloneCandidate(record), nil
	}
	if record.State != "reviewed" {
		return Candidate{}, fmt.Errorf("candidate digest or review state changed")
	}
	record.State = "signed"
	record.SignedManifestDigest = signedDigest
	record.Revision++
	record.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	if err := s.commitLocked(record, true); err != nil {
		return Candidate{}, err
	}
	return cloneCandidate(record), nil
}

func ValidatePromotion(candidate Candidate, raw []byte, policy verify.Policy) (string, error) {
	if candidate.State != "reviewed" && candidate.State != "signed" {
		return "", fmt.Errorf("candidate must be tested and reviewed before promotion")
	}
	if err := verify.VerifyRecord(raw, policy); err != nil {
		return "", fmt.Errorf("signed release rejected: %w", err)
	}
	if err := contracts.ValidateAppManifest(raw); err != nil {
		return "", fmt.Errorf("signed release schema rejected: %w", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return "", err
	}
	signedDigest, err := verify.CanonicalDigest(value)
	if err != nil {
		return "", err
	}
	manifest, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("signed release must be an object")
	}
	delete(manifest, "signature")
	manifest["reviewState"] = "candidate"
	digest, err := verify.CanonicalDigest(manifest)
	if err != nil {
		return "", err
	}
	if digest != candidate.CandidateDigest || manifest["appId"] != manifestAppID(candidate.Manifest) {
		return "", fmt.Errorf("signed release does not match the reviewed candidate")
	}
	if candidate.State == "signed" && candidate.SignedManifestDigest != signedDigest {
		return "", fmt.Errorf("signed release differs from the recorded promotion")
	}
	return signedDigest, nil
}

func (s *Store) commit(record Candidate, transition bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("candidate store closed")
	}
	if !transition && len(s.records) >= maxCandidates {
		return fmt.Errorf("candidate store capacity exceeded")
	}
	return s.commitLocked(record, false)
}

func (s *Store) commitLocked(record Candidate, transition bool) error {
	if err := s.validateStored(record); err != nil {
		return err
	}
	if transition {
		previous, exists := s.records[record.CandidateID]
		if !exists || !validTransition(previous, record) {
			return fmt.Errorf("candidate state transition rejected")
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(raw) > 2*maxCandidateBytes || s.totalBytes+int64(len(raw)) > maxStoreBytes {
		return fmt.Errorf("candidate store capacity exceeded")
	}
	name := fmt.Sprintf("%s.%06d.json", record.CandidateID, record.Revision)
	target := filepath.Join(s.root, name)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("candidate snapshot already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".candidate-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := syncDirectory(s.root); err != nil {
		return err
	}
	s.records[record.CandidateID] = cloneCandidate(record)
	s.totalBytes += int64(len(raw))
	return nil
}

func (s *Store) validateStored(record Candidate) error {
	if !candidateIDPattern.MatchString(record.CandidateID) || !validTenant(record.TenantScope) ||
		record.Revision == 0 || !digestPattern.MatchString(record.CandidateDigest) || !digestPattern.MatchString(record.PacketDigest) {
		return fmt.Errorf("candidate record identity invalid")
	}
	manifest, digest, err := candidateManifest(record.Manifest)
	if err != nil || digest != record.CandidateDigest || !bytes.Equal(manifest, record.Manifest) {
		return fmt.Errorf("candidate manifest binding invalid")
	}
	if len(record.Sources) == 0 || len(record.Sources) > 64 || len(record.Citations) == 0 || len(record.Citations) > 64 {
		return fmt.Errorf("candidate source evidence invalid")
	}
	ids := map[string]bool{}
	for _, source := range record.Sources {
		if err := validateSource(source); err != nil || ids[source.SourceID] {
			return fmt.Errorf("candidate source metadata invalid")
		}
		ids[source.SourceID] = true
	}
	cited := map[string]bool{}
	for _, sourceID := range record.Citations {
		if !ids[sourceID] || cited[sourceID] {
			return fmt.Errorf("candidate citations invalid")
		}
		cited[sourceID] = true
	}
	if !provenanceSourceMatches(manifest, record.Sources, cited) {
		return fmt.Errorf("candidate provenance source binding invalid")
	}
	if record.Origin.Assistant != "Codex chat" && record.Origin.Assistant != "manual" {
		return fmt.Errorf("candidate origin invalid")
	}
	if record.Origin.Assistant == "Codex chat" && !digestPattern.MatchString(record.Origin.RequestDigest) {
		return fmt.Errorf("candidate request digest invalid")
	}
	if record.Origin.Assistant == "manual" && record.Origin.RequestDigest != "" && !digestPattern.MatchString(record.Origin.RequestDigest) {
		return fmt.Errorf("candidate request digest invalid")
	}
	manifestValue, _ := verify.ParseJSON(manifest)
	packetDigest, err := verify.CanonicalDigest(map[string]any{
		"manifest": manifestValue, "sources": sourcesAsAny(record.Sources), "citations": stringsAsAny(record.Citations),
		"origin": map[string]any{"assistant": record.Origin.Assistant, "requestDigest": record.Origin.RequestDigest},
	})
	if err != nil || packetDigest != record.PacketDigest {
		return fmt.Errorf("candidate packet digest invalid")
	}
	switch record.State {
	case "candidate":
		if record.Qualification != nil || len(record.QualificationHistory) != 0 || record.Review != nil || record.SignedManifestDigest != "" {
			return fmt.Errorf("candidate evidence state inconsistent")
		}
	case "failed":
		if err := s.validateQualificationRecord(record); err != nil || record.Qualification.Result != "failed" || record.Review != nil || record.SignedManifestDigest != "" {
			return fmt.Errorf("failed candidate evidence invalid")
		}
	case "tested":
		if err := s.validateQualificationRecord(record); err != nil || record.Qualification.Result != "passed" || record.Review != nil || record.SignedManifestDigest != "" {
			return fmt.Errorf("tested candidate evidence invalid")
		}
	case "reviewed":
		if err := s.validateQualificationRecord(record); err != nil || record.Qualification.Result != "passed" || record.Review == nil || record.Review.ReviewerID == "" || record.SignedManifestDigest != "" {
			return fmt.Errorf("reviewed candidate evidence invalid")
		}
	case "signed":
		if err := s.validateQualificationRecord(record); err != nil || record.Qualification.Result != "passed" || record.Review == nil || record.Review.ReviewerID == "" || !digestPattern.MatchString(record.SignedManifestDigest) {
			return fmt.Errorf("promoted candidate evidence invalid")
		}
	default:
		return fmt.Errorf("candidate state invalid")
	}
	return nil
}

func (s *Store) validateQualificationRecord(record Candidate) error {
	if record.Qualification == nil {
		return fmt.Errorf("qualification missing")
	}
	if len(record.QualificationHistory) >= maxQualificationAttempts {
		return fmt.Errorf("qualification history limit exceeded")
	}
	attempts := make([]Qualification, 0, len(record.QualificationHistory)+1)
	attempts = append(attempts, record.QualificationHistory...)
	attempts = append(attempts, *record.Qualification)
	seen := map[string]bool{}
	for _, attempt := range attempts {
		receipt, id, digest, result, err := validateQualification(attempt.Receipt, record.TenantScope, record.CandidateDigest, s.policy, s.qualificationSigners)
		if err != nil || id != attempt.QualificationID || seen[id] || digest != attempt.Digest ||
			result != attempt.Result || !bytes.Equal(receipt, attempt.Receipt) {
			return fmt.Errorf("qualification receipt binding invalid")
		}
		seen[id] = true
	}
	return nil
}

func validTransition(previous, next Candidate) bool {
	if previous.CandidateID != next.CandidateID || previous.TenantScope != next.TenantScope ||
		previous.CandidateDigest != next.CandidateDigest || previous.PacketDigest != next.PacketDigest ||
		next.Revision != previous.Revision+1 || !bytes.Equal(previous.Manifest, next.Manifest) {
		return false
	}
	switch previous.State {
	case "candidate":
		return (next.State == "tested" || next.State == "failed") &&
			next.Qualification != nil && len(next.QualificationHistory) == 0 && next.Review == nil
	case "failed":
		if (next.State != "failed" && next.State != "tested") || previous.Qualification == nil ||
			next.Qualification == nil || next.Review != nil {
			return false
		}
		expectedHistory := append(append([]Qualification(nil), previous.QualificationHistory...), *previous.Qualification)
		return sameQualifications(expectedHistory, next.QualificationHistory) &&
			next.Qualification.QualificationID != previous.Qualification.QualificationID
	case "tested":
		return next.State == "reviewed" && sameQualifications(previous.QualificationHistory, next.QualificationHistory) &&
			sameQualification(previous.Qualification, next.Qualification) && next.Review != nil
	case "reviewed":
		return next.State == "signed" && sameQualifications(previous.QualificationHistory, next.QualificationHistory) &&
			sameQualification(previous.Qualification, next.Qualification) && sameReview(previous.Review, next.Review)
	default:
		return false
	}
}

func candidateManifest(raw []byte) ([]byte, string, error) {
	if len(raw) == 0 || len(raw) > maxCandidateBytes {
		return nil, "", fmt.Errorf("candidate manifest size invalid")
	}
	if err := contracts.ValidateAppManifest(raw); err != nil {
		return nil, "", fmt.Errorf("candidate manifest rejected: %w", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, "", err
	}
	object, ok := value.(map[string]any)
	if !ok || object["reviewState"] != "candidate" {
		return nil, "", fmt.Errorf("only unsigned candidate manifests may be imported")
	}
	if _, exists := object["signature"]; exists {
		return nil, "", fmt.Errorf("candidate manifest must not contain a signature")
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		return nil, "", err
	}
	if scanCandidateSecrets(string(canonical)) {
		return nil, "", fmt.Errorf("candidate manifest contains a secret pattern")
	}
	digest, err := verify.CanonicalDigest(value)
	return canonical, digest, err
}

func validateSource(source Source) error {
	if len(source.SourceID) > 2048 || !sourceIDPattern.MatchString(source.SourceID) || !digestPattern.MatchString(source.ContentHash) {
		return fmt.Errorf("source identity invalid")
	}
	parsed, err := url.ParseRequestURI(source.URL)
	if err != nil || !strings.HasPrefix(source.URL, "https://") || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || len(source.URL) > 2048 {
		return fmt.Errorf("source URL must be a pinned public HTTPS location")
	}
	return nil
}

func scanCandidateSecrets(text string) bool {
	for _, pattern := range credentialPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	for _, line := range strings.Split(text, "\n") {
		for _, match := range secretAssignment.FindAllStringSubmatch(line, -1) {
			value := match[2]
			if strings.HasPrefix(value, "secret://") || (strings.EqualFold(match[1], "secret") && strings.HasPrefix(value, "//")) {
				continue
			}
			if len(value) >= 12 || shannonEntropy(value) >= 3.5 {
				return true
			}
		}
	}
	return false
}

func shannonEntropy(value string) float64 {
	if len(value) == 0 {
		return 0
	}
	counts := map[byte]int{}
	for index := 0; index < len(value); index++ {
		counts[value[index]]++
	}
	entropy := 0.0
	for _, count := range counts {
		probability := float64(count) / float64(len(value))
		entropy -= probability * math.Log2(probability)
	}
	return entropy
}

func provenanceSourceMatches(manifest []byte, sources []Source, citations map[string]bool) bool {
	value, err := verify.ParseJSON(manifest)
	if err != nil {
		return false
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	provenance, ok := object["provenance"].(map[string]any)
	if !ok {
		return false
	}
	id, idOK := provenance["sourceId"].(string)
	digest, digestOK := provenance["contentHash"].(string)
	if !idOK || !digestOK || !citations[id] {
		return false
	}
	for _, source := range sources {
		if source.SourceID == id && source.ContentHash == digest {
			return true
		}
	}
	return false
}

func validateQualification(raw []byte, tenant, candidateDigest string, policy verify.Policy, qualificationSigners map[string]bool) (json.RawMessage, string, string, string, error) {
	if len(raw) == 0 || len(raw) > maxQualificationBytes {
		return nil, "", "", "", fmt.Errorf("qualification receipt size invalid")
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("qualification receipt rejected")
	}
	object, ok := value.(map[string]any)
	if !ok || !hasExactKeys(object, "schemaVersion", "qualificationId", "tenantScope", "candidateDigest", "result", "runnerId", "runnerVersion", "suiteDigest", "startedAt", "completedAt", "isolation", "tests", "signature") {
		return nil, "", "", "", fmt.Errorf("qualification receipt shape invalid")
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil || contracts.ValidateCandidateQualification(canonical) != nil {
		return nil, "", "", "", fmt.Errorf("qualification receipt schema rejected")
	}
	if verify.VerifySignedRecord(canonical, policy) != nil {
		return nil, "", "", "", fmt.Errorf("qualification receipt signature rejected")
	}
	signature, ok := object["signature"].(map[string]any)
	signerID, signerOK := signature["signedBy"].(string)
	if !ok || !signerOK || !qualificationSigners[signerID] {
		return nil, "", "", "", fmt.Errorf("qualification receipt signer is not configured for runner evidence")
	}
	id, idOK := object["qualificationId"].(string)
	recordTenant, tenantOK := object["tenantScope"].(string)
	recordDigest, digestOK := object["candidateDigest"].(string)
	result, resultOK := object["result"].(string)
	if !idOK || !qualificationIDPattern.MatchString(id) || !tenantOK || recordTenant != tenant ||
		!digestOK || recordDigest != candidateDigest || !resultOK || (result != "passed" && result != "failed") || object["schemaVersion"] != "1.0.0" {
		return nil, "", "", "", fmt.Errorf("qualification receipt binding invalid")
	}
	for _, key := range []string{"runnerId", "runnerVersion"} {
		if text, ok := object[key].(string); !ok || strings.TrimSpace(text) == "" || len(text) > 128 {
			return nil, "", "", "", fmt.Errorf("qualification runner identity invalid")
		}
	}
	if suite, ok := object["suiteDigest"].(string); !ok || !digestPattern.MatchString(suite) {
		return nil, "", "", "", fmt.Errorf("qualification suite digest invalid")
	}
	started, startOK := object["startedAt"].(string)
	completed, endOK := object["completedAt"].(string)
	start, startErr := time.Parse(time.RFC3339, started)
	end, endErr := time.Parse(time.RFC3339, completed)
	if !startOK || !endOK || startErr != nil || endErr != nil || !end.After(start) {
		return nil, "", "", "", fmt.Errorf("qualification time window invalid")
	}
	isolation, ok := object["isolation"].(map[string]any)
	if !ok || !hasExactKeys(isolation, "syntheticProject", "network", "liveVolumeAccess", "privileged", "cpuMilli", "ramMb", "storageMb") ||
		isolation["syntheticProject"] != true || isolation["network"] != "deny-egress" || isolation["liveVolumeAccess"] != false || isolation["privileged"] != false {
		return nil, "", "", "", fmt.Errorf("qualification isolation requirements not met")
	}
	cpu, cpuOK := integerField(isolation["cpuMilli"])
	ram, ramOK := integerField(isolation["ramMb"])
	storage, storageOK := integerField(isolation["storageMb"])
	if !cpuOK || !ramOK || !storageOK || cpu < 1 || cpu > 2000 || ram < 64 || ram > 4096 || storage < 64 || storage > 10240 {
		return nil, "", "", "", fmt.Errorf("qualification resource ceiling invalid")
	}
	tests, ok := object["tests"].([]any)
	if !ok || len(tests) == 0 || len(tests) > 128 {
		return nil, "", "", "", fmt.Errorf("qualification test results missing")
	}
	testIDs := map[string]bool{}
	hasFailedTest := false
	for _, rawTest := range tests {
		test, ok := rawTest.(map[string]any)
		if !ok || !hasExactKeys(test, "testId", "result") {
			return nil, "", "", "", fmt.Errorf("qualification test record invalid")
		}
		testID, idOK := test["testId"].(string)
		testResult, resultOK := test["result"].(string)
		if !idOK || strings.TrimSpace(testID) == "" || len(testID) > 128 || testIDs[testID] || !resultOK ||
			(testResult != "passed" && testResult != "failed") {
			return nil, "", "", "", fmt.Errorf("qualification test record invalid or duplicated")
		}
		if testResult == "failed" {
			hasFailedTest = true
		}
		testIDs[testID] = true
	}
	if (result == "passed" && hasFailedTest) || (result == "failed" && !hasFailedTest) {
		return nil, "", "", "", fmt.Errorf("qualification result does not match test results")
	}
	digest, err := verify.CanonicalDigest(value)
	if err != nil {
		return nil, "", "", "", err
	}
	return canonical, id, digest, result, nil
}

func hasExactKeys(object map[string]any, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

func integerField(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(string(number), 10, 64)
	return parsed, err == nil
}

func validTenant(value string) bool {
	return len(value) > 0 && len(value) <= 128 && regexp.MustCompile("^[A-Za-z0-9_.:-]+$").MatchString(value)
}

func strictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func sourcesAsAny(sources []Source) []any {
	out := make([]any, 0, len(sources))
	for _, source := range sources {
		out = append(out, map[string]any{"sourceId": source.SourceID, "url": source.URL, "contentHash": source.ContentHash})
	}
	return out
}

func stringsAsAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func manifestAppID(raw []byte) string {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return ""
	}
	object, _ := value.(map[string]any)
	appID, _ := object["appId"].(string)
	return appID
}

func summary(record Candidate) Candidate {
	out := cloneCandidate(record)
	out.Manifest = nil
	out.Sources = nil
	out.Citations = nil
	if out.Qualification != nil {
		out.Qualification.Receipt = nil
	}
	for index := range out.QualificationHistory {
		out.QualificationHistory[index].Receipt = nil
	}
	return out
}

func cloneCandidate(record Candidate) Candidate {
	record.Manifest = append(json.RawMessage(nil), record.Manifest...)
	record.Sources = append([]Source(nil), record.Sources...)
	record.Citations = append([]string(nil), record.Citations...)
	if record.Qualification != nil {
		copy := *record.Qualification
		copy.Receipt = append(json.RawMessage(nil), copy.Receipt...)
		record.Qualification = &copy
	}
	record.QualificationHistory = append([]Qualification(nil), record.QualificationHistory...)
	for index := range record.QualificationHistory {
		record.QualificationHistory[index].Receipt = append(json.RawMessage(nil), record.QualificationHistory[index].Receipt...)
	}
	if record.Review != nil {
		copy := *record.Review
		record.Review = &copy
	}
	return record
}

func sameQualification(left, right *Qualification) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.QualificationID == right.QualificationID && left.Digest == right.Digest &&
		left.Result == right.Result && bytes.Equal(left.Receipt, right.Receipt)
}

func sameQualifications(left, right []Qualification) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].QualificationID != right[index].QualificationID || left[index].Digest != right[index].Digest ||
			left[index].Result != right[index].Result || !bytes.Equal(left[index].Receipt, right[index].Receipt) {
			return false
		}
	}
	return true
}

func sameReview(left, right *Review) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
