package manifestregistry

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestRevocationRegistryPersistsSignedMonotonicSequence(t *testing.T) {
	key, policy := newRevocationTestSigner(t)
	directory := filepath.Join(t.TempDir(), "registry")
	registry, err := New(directory, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	sequenceTwo := signedRevocationTestRecord(t, key, "revlist_phase1", 2, "second")
	entry, err := registry.PutRevocation(sequenceTwo)
	if err != nil {
		t.Fatalf("valid revocation list rejected: %v", err)
	}
	if entry.ListID != "revlist_phase1" || entry.Sequence != 2 || entry.Digest == "" {
		t.Fatalf("unexpected revocation registry entry: %+v", entry)
	}
	if repeated, err := registry.PutRevocation(sequenceTwo); err != nil || repeated != entry {
		t.Fatalf("idempotent revocation put = %+v, %v; want %+v", repeated, err, entry)
	}
	if _, err := registry.PutRevocation(signedRevocationTestRecord(t, key, "revlist_phase1", 1, "older")); !errors.Is(err, ErrRevocationRollback) {
		t.Fatalf("lower sequence error = %v, want ErrRevocationRollback", err)
	}
	if _, err := registry.PutRevocation(signedRevocationTestRecord(t, key, "revlist_phase1", 2, "different")); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("same-sequence changed record error = %v, want ErrVersionConflict", err)
	}
	if _, err := registry.PutRevocation(signedRevocationTestRecord(t, key, "revlist_rotated", 3, "rotated")); !errors.Is(err, ErrRevocationListConflict) {
		t.Fatalf("list identity change error = %v, want ErrRevocationListConflict", err)
	}
	sequenceThree := signedRevocationTestRecord(t, key, "revlist_phase1", 3, "third")
	entry, err = registry.PutRevocation(sequenceThree)
	if err != nil || entry.Sequence != 3 {
		t.Fatalf("higher sequence put = %+v, %v", entry, err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(directory, policy)
	if err != nil {
		t.Fatalf("signed revocation list did not survive restart: %v", err)
	}
	defer reopened.Close()
	stored, gotEntry, ok := reopened.GetRevocation()
	if !ok || gotEntry != entry || !bytes.Equal(stored, sequenceThree) {
		t.Fatal("reopened registry did not return the latest signed revocation list")
	}
}

func TestRevocationRegistryRejectsInvalidRecordBeforeWriting(t *testing.T) {
	key, policy := newRevocationTestSigner(t)
	raw := signedRevocationTestRecord(t, key, "revlist_phase1", 1, "initial")
	tampered := bytes.Replace(raw, []byte("\"sequence\":1"), []byte("\"sequence\":9"), 1)
	if bytes.Equal(raw, tampered) {
		t.Fatal("test failed to alter the signed revocation list")
	}
	directory := filepath.Join(t.TempDir(), "registry")
	registry, err := New(directory, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if _, err := registry.PutRevocation(tampered); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("tampered revocation error = %v, want ErrInvalidRecord", err)
	}
	if _, _, ok := registry.GetRevocation(); ok {
		t.Fatal("tampered revocation list was indexed")
	}
}

func newRevocationTestSigner(t *testing.T) (*ecdsa.PrivateKey, verify.Policy) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	keyDigest := sha256.Sum256(publicDER)
	policy := verify.Policy{
		Algorithms: []string{verify.CosignBlobAlgorithm},
		TrustedSigners: map[string]verify.TrustedKey{
			"revocation-test-signer": {
				KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
				PublicKeyPEM: publicPEM,
			},
		},
	}
	return key, policy
}

func signedRevocationTestRecord(t *testing.T, key *ecdsa.PrivateKey, listID string, sequence int64, reason string) []byte {
	t.Helper()
	record := map[string]any{
		"schemaVersion": "1.0.0",
		"listId":        listID,
		"sequence":      json.Number(strconv.FormatInt(sequence, 10)),
		"generatedAt":   "2026-10-30T10:00:00Z",
		"expiresAt":     "2026-10-30T11:00:00Z",
		"entries": []any{
			map[string]any{
				"applicationId":  "org.example.uptime-demo",
				"releaseVersion": "1.0.0",
				"revokedAt":      "2026-10-30T10:00:00Z",
				"reason":         reason,
			},
		},
	}
	payloadDigest, err := verify.CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	messageDigest := sha256.Sum256([]byte(payloadDigest))
	signature, err := ecdsa.SignASN1(rand.Reader, key, messageDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	record["signature"] = map[string]any{
		"algorithm":     verify.CosignBlobAlgorithm,
		"value":         base64.StdEncoding.EncodeToString(signature),
		"signedBy":      "revocation-test-signer",
		"payloadDigest": payloadDigest,
	}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
