package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestStartupErrorReleasesConfiguredRegistryWriters(t *testing.T) {
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	flag.CommandLine = flag.NewFlagSet("startup-error", flag.ContinueOnError)
	dir := t.TempDir()
	store := filepath.Join(dir, "registry.json")
	policyFile := writeSigningPolicyFixture(t, dir)
	manifestDir := filepath.Join(dir, "manifests")
	t.Setenv("WAZI_OPERATOR_TOKEN", "test-"+"operator-token")
	os.Args = []string{"controlplane", "-addr", "127.0.0.1:0", "-registry-file", store,
		"-signing-policy-file", policyFile, "-manifest-registry-dir", manifestDir,
		"-tls-cert", filepath.Join(dir, "missing.crt"), "-tls-key", filepath.Join(dir, "missing.key")}
	if err := run(context.Background()); err == nil {
		t.Fatal("missing TLS files should fail startup")
	}
	reopened, err := pairing.NewRegistryWithStore(nil, store)
	if err != nil {
		t.Fatalf("startup failure leaked the registry writer lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	policy, err := verify.LoadPolicyFile(policyFile)
	if err != nil {
		t.Fatal(err)
	}
	reopenedManifests, err := manifestregistry.New(manifestDir, policy)
	if err != nil {
		t.Fatalf("startup failure leaked the manifest registry writer lock: %v", err)
	}
	if err := reopenedManifests.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeSigningPolicyFixture(t *testing.T, directory string) string {
	t.Helper()
	publicPath := filepath.Join("..", "..", "internal", "verify", "testdata", "cosign-v3.1.4-public.pem")
	publicPEM, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(publicPEM)
	if block == nil || len(rest) != 0 {
		t.Fatal("invalid test public key PEM")
	}
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	spki, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := sha256.Sum256(spki)
	policy := map[string]any{
		"schemaVersion": "1.0.0",
		"algorithms":    []string{"sigstore-cosign"},
		"trustedSigners": map[string]any{
			"cosign-v3.1.4-fixture": map[string]string{
				"keyId":        "sha256:" + hex.EncodeToString(keyDigest[:]),
				"publicKeyPem": string(publicPEM),
			},
		},
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "signing-policy.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
