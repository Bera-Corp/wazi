package main

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
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestCommunityIndexSignatureBindsFileInventory(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := sha256.Sum256(der)
	policy := map[string]any{"schemaVersion": "1.0.0", "algorithms": []any{verify.CosignBlobAlgorithm}, "trustedSigners": map[string]any{"test-maintainer": map[string]any{"keyId": "sha256:" + hex.EncodeToString(fingerprint[:]), "publicKeyPem": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))}}}
	record := map[string]any{"schemaVersion": "1.0.0", "recipeId": "test", "files": map[string]any{"manifest.json": "sha256:" + hex.EncodeToString(fingerprint[:])}}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := verify.PayloadDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	message := sha256.Sum256([]byte(payload))
	signature, err := ecdsa.SignASN1(rand.Reader, key, message[:])
	if err != nil {
		t.Fatal(err)
	}
	record["signature"] = map[string]any{"algorithm": verify.CosignBlobAlgorithm, "signedBy": "test-maintainer", "payloadDigest": payload, "value": base64.StdEncoding.EncodeToString(signature)}
	dir := t.TempDir()
	policyPath, recordPath := filepath.Join(dir, "policy.json"), filepath.Join(dir, "recipe.json")
	policyRaw, _ := json.Marshal(policy)
	if err := os.WriteFile(policyPath, policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	write := func() {
		t.Helper()
		data, err := verify.CanonicalJSON(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(recordPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := run([]string{"verify-community-package", recordPath, policyPath}); err != nil {
		t.Fatal("valid package index rejected:", err)
	}
	record["files"].(map[string]any)["manifest.json"] = "sha256:" + string(make([]byte, 64))
	write()
	if err := run([]string{"verify-community-package", recordPath, policyPath}); err == nil {
		t.Fatal("tampered package inventory accepted")
	}
}

func TestVerifyRevocationDoesNotRequireManifestReviewState(t *testing.T) {
	policy := filepath.Join("..", "..", "..", "releases", "phase1", "gitea", "28.2.0", "signing-policy.json")
	if err := run([]string{"verify", filepath.Join("testdata", "revocation.json"), policy}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "revocation.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	value.(map[string]any)["expiresAt"] = "2030-01-01T00:00:00Z"
	tampered, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tampered.json")
	if err := os.WriteFile(path, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"verify", path, policy}); err == nil {
		t.Fatal("tampered revocation signature accepted")
	}
}
