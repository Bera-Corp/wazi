package controlplane_test

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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/controlplane"
	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestProcedureIngestAndReadRequireOperatorInDevelopmentMode(t *testing.T) {
	record, policy := signedProcedureFixture(t)
	registry, err := manifestregistry.New(filepath.Join(t.TempDir(), "registry"), policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registry.Close(); err != nil {
			t.Errorf("close manifest registry: %v", err)
		}
	})
	opToken := "test-" + "operator-token"
	server := controlplane.New(controlplane.Config{
		OperatorToken:    opToken,
		ManifestRegistry: registry,
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	post := func(token string) (*http.Response, error) {
		request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/procedures", bytes.NewReader(record))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("X-Wazi-Operator-Token", token)
		}
		return httpServer.Client().Do(request)
	}
	response, err := post("")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("procedure write without operator token = %d, want 401", response.StatusCode)
	}
	response, err = post(opToken)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid procedure ingest = %d, want 200", response.StatusCode)
	}

	getURL := httpServer.URL + "/v1/procedures/proc_install_uptime_demo/1.0.0"
	response, err = httpServer.Client().Get(getURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("procedure read without operator token = %d, want 401", response.StatusCode)
	}
	getRequest, err := http.NewRequest(http.MethodGet, getURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	getRequest.Header.Set("X-Wazi-Operator-Token", opToken)
	response, err = httpServer.Client().Do(getRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(body, record) {
		t.Fatalf("procedure read = %d (%s), want stored canonical record", response.StatusCode, body)
	}
	if response.Header.Get("ETag") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("procedure response is missing its digest ETag or no-store cache control")
	}
}

func signedProcedureFixture(t *testing.T) ([]byte, verify.Policy) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "context", "examples", "procedure-install.json"))
	if err != nil {
		t.Fatal(err)
	}
	recordValue, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := recordValue.(map[string]any)
	delete(record, "signature")

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
	signed, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := verify.VerifyRecord(signed, policy); err != nil {
		t.Fatalf("procedure test fixture signature failed verification: %v", err)
	}
	return signed, policy
}
