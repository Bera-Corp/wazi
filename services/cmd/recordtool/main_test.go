package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

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
