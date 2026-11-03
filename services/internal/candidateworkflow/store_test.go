package candidateworkflow

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestCandidateWorkflowPersistsDigestBoundTransitions(t *testing.T) {
	key, policy := candidateTestSigner(t)
	root := filepath.Join(t.TempDir(), "candidates")
	store, err := Open(root, policy, "candidate-test-signer")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	request := candidateTestImport(t)
	candidate, err := store.Import("tenant_alpha", request, now)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.State != "candidate" || candidate.CandidateDigest == "" || candidate.PacketDigest == "" {
		t.Fatalf("bad import result: %+v", candidate)
	}
	if _, ok := store.Get("tenant_other", candidate.CandidateID); ok {
		t.Fatal("cross-tenant read succeeded")
	}
	if len(store.List("tenant_other")) != 0 {
		t.Fatal("cross-tenant list leaked records")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(root, policy, "candidate-test-signer")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store.Close()
	receipt := candidateTestQualification(t, key, "tenant_alpha", candidate.CandidateDigest, false)
	if _, err := store.RecordQualification("tenant_alpha", candidate.CandidateID, "sha256:"+strings.Repeat("0", 64), receipt, now); err == nil {
		t.Fatal("stale candidate digest accepted")
	}
	if _, err := store.RecordQualification("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, candidateTestQualification(t, key, "tenant_alpha", candidate.CandidateDigest, true), now); err == nil {
		t.Fatal("live-volume qualification accepted")
	}
	inconsistent := candidateTestQualificationOutcome(t, key, "tenant_alpha", candidate.CandidateDigest, false,
		"qual_inconsistent_result", "failed", "passed")
	if _, err := store.RecordQualification("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, inconsistent, now); err == nil {
		t.Fatal("failed result with no failing test accepted")
	}
	failedReceipt := candidateTestQualificationOutcome(t, key, "tenant_alpha", candidate.CandidateDigest, false,
		"qual_failed_attempt", "failed", "failed")
	failed, err := store.RecordQualification("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, failedReceipt, now)
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != "failed" || failed.Qualification == nil || failed.Qualification.Result != "failed" {
		t.Fatalf("failed qualification was not retained: %+v", failed)
	}
	summaries := store.List("tenant_alpha")
	if len(summaries) != 1 || summaries[0].State != "failed" || summaries[0].Qualification == nil ||
		summaries[0].Qualification.Result != "failed" || summaries[0].Qualification.Receipt != nil {
		t.Fatal("tenant candidate summary did not expose the failed result safely")
	}
	if _, err := store.Review("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, "maintainer", now); err == nil {
		t.Fatal("failed qualification advanced to maintainer review")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(root, policy, "candidate-test-signer")
	if err != nil {
		t.Fatalf("failed-attempt restart: %v", err)
	}
	defer store.Close()
	failedReloaded, ok := store.Get("tenant_alpha", candidate.CandidateID)
	if !ok || failedReloaded.State != "failed" || failedReloaded.Qualification == nil ||
		failedReloaded.Qualification.Result != "failed" || len(failedReloaded.QualificationHistory) != 0 {
		t.Fatal("failed qualification was not visible after restart")
	}
	tested, err := store.RecordQualification("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	if tested.State != "tested" || len(tested.QualificationHistory) != 1 ||
		tested.QualificationHistory[0].Result != "failed" || tested.Qualification == nil || tested.Qualification.Result != "passed" {
		t.Fatalf("state=%s", tested.State)
	}
	reviewed, err := store.Review("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, "maintainer", now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := verify.ParseJSON(reviewed.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest := parsed.(map[string]any)
	manifest["reviewState"] = "signed"
	signed := signCandidateRecord(t, key, manifest)
	digest, err := ValidatePromotion(reviewed, signed, policy)
	if err != nil {
		t.Fatalf("candidate did not bind to signed release: %v", err)
	}
	parsed, _ = verify.ParseJSON(signed)
	changed := parsed.(map[string]any)
	changed["name"] = "Changed after review"
	if _, err := ValidatePromotion(reviewed, signCandidateRecord(t, key, changed), policy); err == nil {
		t.Fatal("changed candidate content was promoted")
	}
	promoted, err := store.MarkPromoted("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, digest, now)
	if err != nil || promoted.State != "signed" {
		t.Fatalf("promotion = %s, %v", promoted.State, err)
	}
	if repeatedDigest, err := ValidatePromotion(promoted, signed, policy); err != nil || repeatedDigest != digest {
		t.Fatalf("same signed promotion could not be retried: digest=%s err=%v", repeatedDigest, err)
	}
	repeated, err := store.MarkPromoted("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, digest, now.Add(time.Second))
	if err != nil || repeated.Revision != promoted.Revision || repeated.SignedManifestDigest != digest {
		t.Fatalf("idempotent promotion retry changed state: revision=%d err=%v", repeated.Revision, err)
	}
	if _, err := store.MarkPromoted("tenant_alpha", candidate.CandidateID, candidate.CandidateDigest, "sha256:"+strings.Repeat("f", 64), now); err == nil {
		t.Fatal("different signed manifest was accepted for an already promoted candidate")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(root, policy, "candidate-test-signer")
	if err != nil {
		t.Fatalf("promoted restart validation: %v", err)
	}
	defer store.Close()
	got, ok := store.Get("tenant_alpha", candidate.CandidateID)
	if !ok || got.State != "signed" || got.CandidateDigest != candidate.CandidateDigest {
		t.Fatal("promoted state not durable")
	}
}

func TestCandidateImportRejectsUncitedAndSignedManifest(t *testing.T) {
	_, policy := candidateTestSigner(t)
	store, err := Open(filepath.Join(t.TempDir(), "candidates"), policy, "candidate-test-signer")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := candidateTestImport(t)
	request.Citations = []string{"not-cited"}
	if _, err := store.Import("tenant_alpha", request, time.Now()); err == nil {
		t.Fatal("uncited candidate accepted")
	}
	request = candidateTestImport(t)
	parsed, _ := verify.ParseJSON(request.Manifest)
	manifest := parsed.(map[string]any)
	manifest["reviewState"] = "signed"
	request.Manifest = signCandidateRecord(t, candidateTestPrivate(t), manifest)
	if _, err := store.Import("tenant_alpha", request, time.Now()); err == nil {
		t.Fatal("signed manifest accepted as candidate")
	}
}

func candidateTestImport(t *testing.T) ImportRequest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "releases", "phase2", "postgresql", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	manifest := parsed.(map[string]any)
	delete(manifest, "signature")
	manifest["reviewState"] = "candidate"
	canonical, err := verify.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	provenance := manifest["provenance"].(map[string]any)
	sourceID := provenance["sourceId"].(string)
	return ImportRequest{
		Manifest:  canonical,
		Sources:   []Source{{SourceID: sourceID, URL: sourceID, ContentHash: provenance["contentHash"].(string)}},
		Citations: []string{sourceID},
		Origin:    Origin{Assistant: "Codex chat", RequestDigest: "sha256:" + strings.Repeat("a", 64)},
	}
}

func candidateTestQualification(t *testing.T, key *ecdsa.PrivateKey, tenant, digest string, live bool) []byte {
	return candidateTestQualificationOutcome(t, key, tenant, digest, live, "qual_test_run", "passed", "passed")
}

func candidateTestQualificationOutcome(t *testing.T, key *ecdsa.PrivateKey, tenant, digest string, live bool, id, result, testResult string) []byte {
	t.Helper()
	value := map[string]any{
		"schemaVersion": "1.0.0", "qualificationId": id, "tenantScope": tenant,
		"candidateDigest": digest, "result": result, "runnerId": "wazi-synthetic-runner",
		"runnerVersion": "1.0.0", "suiteDigest": "sha256:" + strings.Repeat("b", 64),
		"startedAt": "2026-11-03T09:00:00Z", "completedAt": "2026-11-03T09:01:00Z",
		"isolation": map[string]any{"syntheticProject": true, "network": "deny-egress", "liveVolumeAccess": live,
			"privileged": false, "cpuMilli": json.Number("500"), "ramMb": json.Number("512"), "storageMb": json.Number("1024")},
		"tests": []any{map[string]any{"testId": "schema-and-render", "result": testResult}},
	}
	return signCandidateRecord(t, key, value)
}

func candidateTestPrivate(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func candidateTestSigner(t *testing.T) (*ecdsa.PrivateKey, verify.Policy) {
	t.Helper()
	key := candidateTestPrivate(t)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := sha256.Sum256(der)
	return key, verify.Policy{Algorithms: []string{verify.CosignBlobAlgorithm}, TrustedSigners: map[string]verify.TrustedKey{
		"candidate-test-signer": {KeyID: "sha256:" + hex.EncodeToString(keyDigest[:]), PublicKeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})},
	}}
}

func signCandidateRecord(t *testing.T, key *ecdsa.PrivateKey, record map[string]any) []byte {
	t.Helper()
	delete(record, "signature")
	unsigned, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := verify.PayloadDigest(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	message := sha256.Sum256([]byte(payloadDigest))
	signature, err := ecdsa.SignASN1(rand.Reader, key, message[:])
	if err != nil {
		t.Fatal(err)
	}
	record["signature"] = map[string]any{"algorithm": verify.CosignBlobAlgorithm, "payloadDigest": payloadDigest,
		"signedBy": "candidate-test-signer", "value": base64.StdEncoding.EncodeToString(signature)}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
