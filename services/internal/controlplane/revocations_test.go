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
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/controlplane"
	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestRevocationIngestReadAndRollbackUseOperatorBoundary(t *testing.T) {
	key, policy := newControlplaneRevocationSigner(t)
	sequenceTwo := controlplaneRevocationRecord(t, key, 2, "second")
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
	server := controlplane.New(controlplane.Config{OperatorToken: opToken, ManifestRegistry: registry})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	post := func(body []byte, token string) (*http.Response, error) {
		request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/revocations", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("X-Wazi-Operator-Token", token)
		}
		return httpServer.Client().Do(request)
	}
	response, err := post(sequenceTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revocation write without operator token = %d, want 401", response.StatusCode)
	}
	response, err = post(sequenceTwo, opToken)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid revocation ingest = %d, want 200", response.StatusCode)
	}

	getURL := httpServer.URL + "/v1/revocations/current"
	response, err = httpServer.Client().Get(getURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revocation read without operator token = %d, want 401", response.StatusCode)
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
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(body, sequenceTwo) {
		t.Fatalf("revocation read = %d (%s), want stored canonical list", response.StatusCode, body)
	}
	if response.Header.Get("ETag") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("revocation response is missing digest ETag or no-store cache control")
	}

	response, err = post(controlplaneRevocationRecord(t, key, 1, "older"), opToken)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("revocation rollback response = %d, want 409", response.StatusCode)
	}
}

func newControlplaneRevocationSigner(t *testing.T) (*ecdsa.PrivateKey, verify.Policy) {
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
	return key, verify.Policy{
		Algorithms: []string{verify.CosignBlobAlgorithm},
		TrustedSigners: map[string]verify.TrustedKey{
			"revocation-api-test": {
				KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
				PublicKeyPEM: publicPEM,
			},
		},
	}
}

func controlplaneRevocationRecord(t *testing.T, key *ecdsa.PrivateKey, sequence int64, reason string) []byte {
	t.Helper()
	record := map[string]any{
		"schemaVersion": "1.0.0",
		"listId":        "revlist_phase1",
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
		"signedBy":      "revocation-api-test",
		"payloadDigest": payloadDigest,
	}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
