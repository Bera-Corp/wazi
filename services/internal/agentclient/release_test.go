package agentclient

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestVerifyReleaseRecordsBindsSignedManifestAndProcedure(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	manifest := readReleaseFixture(t, "app-manifest.json")
	procedure := readReleaseFixture(t, "procedure-install.json")
	signedManifest := signReleaseFixture(t, manifest, key)
	signedProcedure := signReleaseFixture(t, procedure, key)

	expected := ReleaseExpectation{
		ApplicationID:    "org.example.uptime-demo",
		ReleaseVersion:   "1.0.0",
		ProcedureID:      "proc_install_uptime_demo",
		ProcedureVersion: "1.0.0",
		Operation:        "install",
	}
	verified, err := VerifyReleaseRecords(signedManifest, signedProcedure, policy, expected)
	if err != nil {
		t.Fatalf("valid signed release rejected: %v", err)
	}
	if verified.ManifestDigest == "" || verified.ProcedureDigest == "" ||
		verified.ManifestDigest == verified.ProcedureDigest {
		t.Fatalf("unexpected release digests: %+v", verified)
	}

	expected.Operation = "upgrade"
	if _, err := VerifyReleaseRecords(signedManifest, signedProcedure, policy, expected); err == nil {
		t.Fatal("procedure for another operation was accepted")
	}
}

func TestVerifyReleaseRecordsRejectsSchemaInvalidAndUntrustedRecords(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	manifest := readReleaseFixture(t, "app-manifest.json")
	procedure := readReleaseFixture(t, "procedure-install.json")
	signedProcedure := signReleaseFixture(t, procedure, key)
	value, err := verify.ParseJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	record := value.(map[string]any)
	record["unexpectedAuthority"] = "self-supplied"
	signedInvalidManifest := signReleaseValue(t, record, key)
	expected := ReleaseExpectation{
		ApplicationID:    "org.example.uptime-demo",
		ReleaseVersion:   "1.0.0",
		ProcedureID:      "proc_install_uptime_demo",
		ProcedureVersion: "1.0.0",
		Operation:        "install",
	}
	if _, err := VerifyReleaseRecords(signedInvalidManifest, signedProcedure, policy, expected); err == nil {
		t.Fatal("validly signed schema-invalid manifest was accepted")
	}
	if _, err := VerifyReleaseRecords(signedProcedure, signedProcedure, verify.Policy{}, expected); err == nil {
		t.Fatal("untrusted signer was accepted")
	}
}

func TestFetchVerifiedReleaseChecksCanonicalETagsAndSignatures(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	manifest := signReleaseFixture(t, readReleaseFixture(t, "app-manifest.json"), key)
	procedure := signReleaseFixture(t, readReleaseFixture(t, "procedure-install.json"), key)
	records := map[string][]byte{
		"/v1/manifests/org.example.uptime-demo/1.0.0":   manifest,
		"/v1/procedures/proc_install_uptime_demo/1.0.0": procedure,
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
	verified, err := FetchVerifiedReleaseArtifacts(context.Background(), server.URL, tlsConfig, policy, expected)
	if err != nil {
		t.Fatalf("valid fetched release rejected: %v", err)
	}
	if verified.Release.ManifestDigest == "" || verified.Release.ProcedureDigest == "" {
		t.Fatalf("fetched release has empty digests: %+v", verified.Release)
	}
	if !bytes.Equal(verified.Manifest(), manifest) || !bytes.Equal(verified.Procedure(), procedure) {
		t.Fatal("fetched artifact bytes differ from the records whose signatures were verified")
	}
	copy := verified.Manifest()
	copy[0] ^= 0xff
	if !bytes.Equal(verified.Manifest(), manifest) {
		t.Fatal("mutating an accessor result changed the retained verified manifest")
	}
}

func pairedTestTLSConfig(t *testing.T, server *httptest.Server) *tls.Config {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	ca, err := pairing.GenerateCA("agentclient-test")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, _, err := ca.IssueClientCert("tenant_a", "host_alpha", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	clientCertificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      roots,
		Certificates: []tls.Certificate{clientCertificate},
	}
}

func TestFetchCanonicalRecordRejectsMismatchedETag(t *testing.T) {
	raw := readReleaseFixture(t, "app-manifest.json")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", fmt.Sprintf("\"sha256:%064d\"", 0))
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	if _, err := fetchCanonicalRecord(context.Background(), server.URL, tlsConfig); err == nil {
		t.Fatal("record with a mismatched ETag was accepted")
	}
}

func readReleaseFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	path := filepath.Join(root, "context", "examples", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newReleaseTestSigner(t *testing.T) (verify.Policy, *ecdsa.PrivateKey) {
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
	return verify.Policy{
		Algorithms: []string{verify.CosignBlobAlgorithm},
		TrustedSigners: map[string]verify.TrustedKey{
			"agent-release-test-signer": {
				KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
				PublicKeyPEM: publicPEM,
			},
		},
	}, key
}

func signReleaseFixture(t *testing.T, raw []byte, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return signReleaseValue(t, value.(map[string]any), key)
}

func signReleaseValue(t *testing.T, record map[string]any, key *ecdsa.PrivateKey) []byte {
	t.Helper()
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
		"signedBy":      "agent-release-test-signer",
		"payloadDigest": payloadDigest,
	}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
