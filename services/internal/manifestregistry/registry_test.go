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
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestRegistryPersistsOnlyVerifiedCanonicalRecords(t *testing.T) {
	raw := readManifestFixture(t)
	policy := manifestFixturePolicy(t)
	directory := filepath.Join(t.TempDir(), "registry")

	registry, err := New(directory, policy)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := registry.Put(raw)
	if err != nil {
		t.Fatalf("valid signed manifest rejected: %v", err)
	}
	if entry.AppID != "org.example.uptime-demo" || entry.Version != "1.0.0" {
		t.Fatalf("unexpected registry entry: %+v", entry)
	}
	if repeated, err := registry.Put(raw); err != nil || repeated != entry {
		t.Fatalf("idempotent put = %+v, %v; want %+v", repeated, err, entry)
	}
	stored, gotEntry, ok := registry.Get(entry.AppID, entry.Version)
	if !ok || gotEntry != entry || !bytes.Equal(stored, raw) {
		t.Fatal("registry did not return the verified record")
	}
	if entries := registry.List(); len(entries) != 1 || entries[0] != entry {
		t.Fatalf("registry list = %+v, want one entry %+v", entries, entry)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(directory, policy)
	if err != nil {
		t.Fatalf("verified manifest did not survive restart: %v", err)
	}
	defer reopened.Close()
	if _, gotEntry, ok := reopened.Get(entry.AppID, entry.Version); !ok || gotEntry != entry {
		t.Fatal("reopened registry lost its verified release")
	}
}

func TestRegistryRejectsAlteredRecordBeforeWriting(t *testing.T) {
	raw := readManifestFixture(t)
	altered := bytes.Replace(raw, []byte("Uptime Demo"), []byte("Broken Demo"), 1)
	if bytes.Equal(raw, altered) {
		t.Fatal("test failed to alter the manifest")
	}
	directory := filepath.Join(t.TempDir(), "registry")
	registry, err := New(directory, manifestFixturePolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if _, err := registry.Put(altered); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("altered manifest error = %v, want ErrInvalidRecord", err)
	}
	if entries := registry.List(); len(entries) != 0 {
		t.Fatalf("altered manifest was indexed: %+v", entries)
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != lockFileName {
		t.Fatalf("rejected manifest left registry files: %+v", files)
	}
}

func TestRegistryRejectsSignedSchemaInvalidRecordBeforeWriting(t *testing.T) {
	value, err := verify.ParseJSON(readManifestFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	record["unexpectedAuthority"] = "self-supplied"
	raw, policy := signRecordWithEphemeralKey(t, record)
	if err := verify.VerifyRecord(raw, policy); err != nil {
		t.Fatalf("test record must have a valid signature before schema rejection: %v", err)
	}

	directory := filepath.Join(t.TempDir(), "registry")
	registry, err := New(directory, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if _, err := registry.Put(raw); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("signed but schema-invalid manifest error = %v, want ErrInvalidRecord", err)
	}
	if entries := registry.List(); len(entries) != 0 {
		t.Fatalf("signed schema-invalid manifest was indexed: %+v", entries)
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != lockFileName {
		t.Fatalf("rejected schema-invalid manifest left registry files: %+v", files)
	}
}

func TestProcedureRegistryPersistsVerifiedSchemaRecords(t *testing.T) {
	value, err := verify.ParseJSON(readProcedureFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, policy := signRecordWithEphemeralKey(t, value.(map[string]any))
	if err := verify.VerifyRecord(raw, policy); err != nil {
		t.Fatalf("signed procedure fixture failed cryptographic verification: %v", err)
	}

	directory := filepath.Join(t.TempDir(), "registry")
	registry, err := New(directory, policy)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte("\"operation\":\"install\""), []byte("\"operation\":\"upgrade\""), 1)
	if bytes.Equal(raw, tampered) {
		t.Fatal("test failed to alter the signed procedure")
	}
	if _, err := registry.PutProcedure(tampered); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("tampered procedure error = %v, want ErrInvalidRecord", err)
	}
	if entries := registry.ListProcedures(); len(entries) != 0 {
		t.Fatalf("tampered procedure was indexed: %+v", entries)
	}
	entry, err := registry.PutProcedure(raw)
	if err != nil {
		t.Fatalf("valid signed procedure rejected: %v", err)
	}
	if entry.ProcedureID != "proc_install_uptime_demo" ||
		entry.Version != "1.0.0" ||
		entry.ApplicationID != "org.example.uptime-demo" ||
		entry.Operation != "install" {
		t.Fatalf("unexpected procedure registry entry: %+v", entry)
	}
	if repeated, err := registry.PutProcedure(raw); err != nil || repeated != entry {
		t.Fatalf("idempotent procedure put = %+v, %v; want %+v", repeated, err, entry)
	}
	stored, gotEntry, ok := registry.GetProcedure(entry.ProcedureID, entry.Version)
	if !ok || gotEntry != entry || !bytes.Equal(stored, raw) {
		t.Fatal("procedure registry did not return the verified record")
	}
	if entries := registry.ListProcedures(); len(entries) != 1 || entries[0] != entry {
		t.Fatalf("procedure registry list = %+v, want one entry %+v", entries, entry)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(directory, policy)
	if err != nil {
		t.Fatalf("verified procedure did not survive restart: %v", err)
	}
	if _, gotEntry, ok := reopened.GetProcedure(entry.ProcedureID, entry.Version); !ok || gotEntry != entry {
		t.Fatal("reopened registry lost its verified procedure")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	procedureDirectory := filepath.Join(directory, procedureDirName)
	files, err := os.ReadDir(procedureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0].Name(), ".json") {
		t.Fatalf("unexpected persisted procedure files: %+v", files)
	}
	corrupt := append(append([]byte(nil), raw...), ' ')
	if err := os.WriteFile(filepath.Join(procedureDirectory, files[0].Name()), corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(directory, policy); err == nil {
		t.Fatal("registry loaded a corrupted stored procedure")
	}
}

func TestRegistryLockAndCorruptRestartFailClosed(t *testing.T) {
	raw := readManifestFixture(t)
	policy := manifestFixturePolicy(t)
	directory := filepath.Join(t.TempDir(), "registry")
	registry, err := New(directory, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(directory, policy); err == nil {
		t.Fatal("second registry writer was admitted")
	}
	entry, err := registry.Put(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one stored manifest, got %d files", len(files))
	}
	path := filepath.Join(directory, files[0].Name())
	corrupt := bytes.Replace(raw, []byte("Uptime Demo"), []byte("Broken Demo"), 1)
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(directory, policy); err == nil {
		t.Fatalf("registry loaded corrupted release %+v", entry)
	}
}

func readManifestFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../verify/testdata/cosign-v3.1.4-app-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func readProcedureFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "context", "examples", "procedure-install.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func signRecordWithEphemeralKey(t *testing.T, record map[string]any) ([]byte, verify.Policy) {
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
			"ephemeral-test-signer": {
				KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
				PublicKeyPEM: publicPEM,
			},
		},
	}
	delete(record, "signature")
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
		"signedBy":      "ephemeral-test-signer",
		"payloadDigest": payloadDigest,
	}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw, policy
}

func manifestFixturePolicy(t *testing.T) verify.Policy {
	t.Helper()
	publicPEM, err := os.ReadFile("../verify/testdata/cosign-v3.1.4-public.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(publicPEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		t.Fatal("invalid test public key PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("test public key is not ECDSA")
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := sha256.Sum256(der)
	return verify.Policy{
		Algorithms: []string{verify.CosignBlobAlgorithm},
		TrustedSigners: map[string]verify.TrustedKey{
			"cosign-v3.1.4-fixture": {
				KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
				PublicKeyPEM: publicPEM,
			},
		},
	}
}
