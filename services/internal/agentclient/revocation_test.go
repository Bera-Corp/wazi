package agentclient

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestVerifyRevocationRecordEnforcesFreshnessScopeAndSequence(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	manifest := signReleaseFixture(t, readReleaseFixture(t, "app-manifest.json"), key)
	manifestValue, err := verify.ParseJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := verify.CanonicalDigest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	expected := RevocationExpectation{
		ApplicationID:  "org.example.uptime-demo",
		ReleaseVersion: "1.0.0",
		ManifestDigest: manifestDigest,
	}
	now := time.Date(2026, 10, 30, 10, 30, 0, 0, time.UTC)
	freshness := RevocationFreshness{
		Now: func() time.Time { return now }, MaxAge: time.Hour, ClockSkew: time.Minute,
	}
	entry := map[string]any{
		"applicationId":  expected.ApplicationID,
		"releaseVersion": expected.ReleaseVersion,
		"manifestDigest": expected.ManifestDigest,
		"revokedAt":      "2026-10-30T10:00:00Z",
		"reason":         "fixture revocation",
	}
	sequenceTwo := signedAgentRevocation(t, key, 2, []any{entry}, "2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	decision, err := VerifyRevocationRecord(sequenceTwo, policy, expected, RevocationCursor{}, freshness)
	if err != nil {
		t.Fatalf("fresh signed revocation rejected: %v", err)
	}
	if !decision.Revoked || decision.Cursor.Sequence != 2 || decision.Reason != "fixture revocation" {
		t.Fatalf("revocation decision = %+v, want matching release denied", decision)
	}
	repeated, err := VerifyRevocationRecord(sequenceTwo, policy, expected, decision.Cursor, freshness)
	if err != nil || repeated.Cursor != decision.Cursor {
		t.Fatalf("idempotent revocation check = %+v, %v", repeated, err)
	}

	sequenceOne := signedAgentRevocation(t, key, 1, []any{}, "2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	if _, err := VerifyRevocationRecord(sequenceOne, policy, expected, decision.Cursor, freshness); err == nil {
		t.Fatal("lower revocation sequence was accepted")
	}
	reusedSequence := signedAgentRevocation(t, key, 2, []any{}, "2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	if _, err := VerifyRevocationRecord(reusedSequence, policy, expected, decision.Cursor, freshness); err == nil {
		t.Fatal("same revocation sequence with different content was accepted")
	}
	if _, err := VerifyRevocationRecord(sequenceTwo, policy, expected, RevocationCursor{}, RevocationFreshness{}); err == nil {
		t.Fatal("revocation acceptance without an explicit max age was accepted")
	}

	now = now.Add(2 * time.Hour)
	if _, err := VerifyRevocationRecord(sequenceTwo, policy, expected, RevocationCursor{}, freshness); err == nil {
		t.Fatal("expired revocation list was accepted")
	}
}

func TestRevocationStateStorePersistsSequenceAndFailsClosedOnCorruption(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	directory := filepath.Join(t.TempDir(), "agent-state")
	now := time.Date(2026, 10, 30, 10, 30, 0, 0, time.UTC)
	freshness := RevocationFreshness{Now: func() time.Time { return now }, MaxAge: time.Hour}
	expected := RevocationExpectation{
		ApplicationID:  "org.example.uptime-demo",
		ReleaseVersion: "1.0.0",
		ManifestDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	sequenceTwo := signedAgentRevocation(t, key, 2, []any{}, "2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	sequenceOne := signedAgentRevocation(t, key, 1, []any{}, "2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	store, err := OpenRevocationStateStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRevocationStateStore(directory); err == nil {
		t.Fatal("second revocation state writer was admitted")
	}
	decision, err := store.Accept(sequenceTwo, policy, expected, freshness)
	if err != nil || decision.Cursor.Sequence != 2 {
		t.Fatalf("accept sequence two = %+v, %v", decision, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenRevocationStateStore(directory)
	if err != nil {
		t.Fatalf("open persisted revocation state: %v", err)
	}
	if reopened.Cursor().Sequence != 2 {
		t.Fatalf("persisted cursor = %+v, want sequence 2", reopened.Cursor())
	}
	if _, err := reopened.Accept(sequenceOne, policy, expected, freshness); err == nil {
		t.Fatal("restart accepted a lower signed revocation sequence")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(directory, revocationStateFilename)
	if err := os.WriteFile(statePath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRevocationStateStore(directory); err == nil {
		t.Fatal("corrupt persisted anti-rollback cursor was accepted")
	}
}

func TestFetchVerifiedReleaseWithRevocationPersistsAndDeniesBeforeReturn(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	manifest := signReleaseFixture(t, readReleaseFixture(t, "app-manifest.json"), key)
	procedure := signReleaseFixture(t, readReleaseFixture(t, "procedure-install.json"), key)
	manifestValue, err := verify.ParseJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := verify.CanonicalDigest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	revocation := signedAgentRevocation(t, key, 1, []any{
		map[string]any{
			"applicationId":  "org.example.uptime-demo",
			"releaseVersion": "1.0.0",
			"manifestDigest": manifestDigest,
			"revokedAt":      "2026-10-30T10:00:00Z",
			"reason":         "test must stop before mutation",
		},
	}, "2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	records := map[string][]byte{
		"/v1/manifests/org.example.uptime-demo/1.0.0":   manifest,
		"/v1/procedures/proc_install_uptime_demo/1.0.0": procedure,
		"/v1/revocations/current":                       revocation,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record, ok := records[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		value, err := verify.ParseJSON(record)
		if err != nil {
			http.Error(w, "invalid test record", http.StatusInternalServerError)
			return
		}
		digest, err := verify.CanonicalDigest(value)
		if err != nil {
			http.Error(w, "invalid test digest", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("ETag", fmt.Sprintf("\"%s\"", digest))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(record)
	}))
	defer server.Close()
	tlsConfig := pairedTestTLSConfig(t, server)
	expected := ReleaseExpectation{
		ApplicationID:    "org.example.uptime-demo",
		ReleaseVersion:   "1.0.0",
		ProcedureID:      "proc_install_uptime_demo",
		ProcedureVersion: "1.0.0",
		Operation:        "install",
	}
	store, err := OpenRevocationStateStore(filepath.Join(t.TempDir(), "agent-state"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 30, 10, 30, 0, 0, time.UTC)
	_, err = FetchVerifiedReleaseWithRevocation(
		context.Background(), server.URL, tlsConfig, policy, expected,
		RevocationFreshness{Now: func() time.Time { return now }, MaxAge: time.Hour}, store,
	)
	if err == nil || !errors.Is(err, ErrReleaseRevoked) {
		t.Fatalf("revoked release result error = %v, want ErrReleaseRevoked", err)
	}
	if store.Cursor().Sequence != 1 {
		t.Fatalf("revocation cursor = %+v, want persisted sequence 1 before denial", store.Cursor())
	}

	records["/v1/revocations/current"] = signedAgentRevocation(t, key, 2, nil,
		"2026-10-30T10:00:00Z", "2026-10-30T11:00:00Z")
	artifacts, err := FetchVerifiedReleaseArtifactsWithRevocation(
		context.Background(), server.URL, tlsConfig, policy, expected,
		RevocationFreshness{Now: func() time.Time { return now }, MaxAge: time.Hour}, store,
	)
	if err != nil {
		t.Fatalf("unrevoked release artifacts rejected: %v", err)
	}
	if !bytes.Equal(artifacts.Manifest(), manifest) || !bytes.Equal(artifacts.Procedure(), procedure) {
		t.Fatal("revocation-aware fetch returned bytes other than the verified release records")
	}
	if store.Cursor().Sequence != 2 {
		t.Fatalf("revocation cursor = %+v, want persisted sequence 2 before return", store.Cursor())
	}
}

func signedAgentRevocation(t *testing.T, key *ecdsa.PrivateKey, sequence int64, entries []any, generatedAt, expiresAt string) []byte {
	t.Helper()
	return signReleaseValue(t, map[string]any{
		"schemaVersion": "1.0.0",
		"listId":        "revlist_phase1",
		"sequence":      json.Number(strconv.FormatInt(sequence, 10)),
		"generatedAt":   generatedAt,
		"expiresAt":     expiresAt,
		"entries":       entries,
	}, key)
}
