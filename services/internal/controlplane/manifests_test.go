package controlplane_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
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

func TestManifestIngestRequiresOperatorAndServesVerifiedRecord(t *testing.T) {
	record, policy := manifestHTTPFixture(t)
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
		AllowPlaintextProfiles: true,
		OperatorToken:          opToken,
		ManifestRegistry:       registry,
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	post := func(body []byte, token string) (*http.Response, error) {
		request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/manifests", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("X-Wazi-Operator-Token", token)
		}
		return httpServer.Client().Do(request)
	}
	response, err := post(record, "")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("manifest write without operator token = %d, want 401", response.StatusCode)
	}

	altered := bytes.Replace(record, []byte("Uptime Demo"), []byte("Broken Demo"), 1)
	response, err = post(altered, opToken)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("altered manifest ingest = %d, want 400", response.StatusCode)
	}

	response, err = post(record, opToken)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("valid manifest ingest = %d (%s), want 200", response.StatusCode, body)
	}
	response.Body.Close()

	getURL := httpServer.URL + "/v1/manifests/org.example.uptime-demo/1.0.0"
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
		t.Fatalf("manifest read = %d (%s), want stored canonical record", response.StatusCode, body)
	}
	if response.Header.Get("ETag") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("manifest response is missing its digest ETag or no-store cache control")
	}
}

func TestManifestReadRequiresNodeAuthenticationOutsideDevMode(t *testing.T) {
	_, policy := manifestHTTPFixture(t)
	registry, err := manifestregistry.New(filepath.Join(t.TempDir(), "registry"), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	server := controlplane.New(controlplane.Config{ManifestRegistry: registry})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	response, err := httpServer.Client().Get(httpServer.URL + "/v1/manifests/org.example.uptime-demo/1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated manifest read = %d, want 401", response.StatusCode)
	}
}

func manifestHTTPFixture(t *testing.T) ([]byte, verify.Policy) {
	t.Helper()
	base := filepath.Join("..", "verify", "testdata")
	record, err := os.ReadFile(filepath.Join(base, "cosign-v3.1.4-app-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	publicPEM, err := os.ReadFile(filepath.Join(base, "cosign-v3.1.4-public.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(publicPEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		t.Fatal("test public key PEM is invalid")
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
	policy := verify.Policy{
		Algorithms: []string{verify.CosignBlobAlgorithm},
		TrustedSigners: map[string]verify.TrustedKey{
			"cosign-v3.1.4-fixture": {
				KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
				PublicKeyPEM: publicPEM,
			},
		},
	}
	return record, policy
}
