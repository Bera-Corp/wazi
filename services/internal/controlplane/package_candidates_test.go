package controlplane

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/candidateworkflow"
	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestPackageCandidateHTTPRequiresMaintainerSessionCSRFAndFixedTenant(t *testing.T) {
	policy := candidateWorkflowTestPolicy(t)
	store, err := candidateworkflow.Open(filepath.Join(t.TempDir(), "candidates"), policy, "runner-qualification")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	policyBundle, err := os.ReadFile(filepath.Join("..", "..", "..", "context", "examples", "policy-bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		PilotAgentSocket: "/run/wazi-phase1/agent.sock", OperatorToken: "local-" + "fixture-" + "operator",
		ApproverID: "pilot-maintainer", ApproverRole: "maintainer",
		CapabilityPolicyBundle: policyBundle, CandidateStore: store, SigningPolicy: policy,
		Now: func() time.Time { return time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC) },
	}
	server := New(config)
	handler := server.Handler()
	call := func(method, path string, body []byte, cookie *http.Cookie, origin, csrf string, operator bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "https://localhost"+path, bytes.NewReader(body))
		request.TLS = &tls.ConnectionState{}
		request.RemoteAddr = "127.0.0.1:4321"
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if csrf != "" {
			request.Header.Set("X-Wazi-CSRF", csrf)
		}
		if operator {
			request.Header.Set("X-Wazi-Operator-Token", config.OperatorToken)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	if got := call(http.MethodGet, "/v1/package-candidates", nil, nil, "", "", false).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d", got)
	}
	bootstrap := call(http.MethodPost, "/v1/context-sessions", nil, nil, "", "", true)
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d: %s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapBody map[string]string
	if err := json.Unmarshal(bootstrap.Body.Bytes(), &bootstrapBody); err != nil {
		t.Fatal(err)
	}
	exchangeBody, _ := json.Marshal(map[string]string{"code": bootstrapBody["bootstrapCode"]})
	exchanged := call(http.MethodPost, "/v1/context-sessions/exchange", exchangeBody, nil, "https://localhost", "", false)
	if exchanged.Code != http.StatusOK {
		t.Fatalf("exchange status = %d: %s", exchanged.Code, exchanged.Body.String())
	}
	var sessionBody map[string]string
	if err := json.Unmarshal(exchanged.Body.Bytes(), &sessionBody); err != nil {
		t.Fatal(err)
	}
	cookies := exchanged.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("session cookie missing")
	}
	if got := call(http.MethodPost, "/v1/package-candidates", []byte("{}"), cookies[0], "https://localhost", "wrong", false).Code; got != http.StatusForbidden {
		t.Fatalf("bad CSRF import = %d", got)
	}

	manifest := candidateWorkflowTestManifest(t)
	value, err := verify.ParseJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	provenance := value.(map[string]any)["provenance"].(map[string]any)
	sourceID := provenance["sourceId"].(string)
	packet, err := json.Marshal(candidateworkflow.ImportRequest{
		Manifest:  manifest,
		Sources:   []candidateworkflow.Source{{SourceID: sourceID, URL: sourceID, ContentHash: provenance["contentHash"].(string)}},
		Citations: []string{sourceID},
		Origin:    candidateworkflow.Origin{Assistant: "Codex chat", RequestDigest: "sha256:" + strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	imported := call(http.MethodPost, "/v1/package-candidates", packet, cookies[0], "https://localhost", sessionBody["csrf"], false)
	if imported.Code != http.StatusCreated {
		t.Fatalf("candidate import = %d: %s", imported.Code, imported.Body.String())
	}
	var record map[string]any
	if err := json.Unmarshal(imported.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["tenantScope"] != "tenant_demo" || record["state"] != "candidate" {
		t.Fatalf("candidate was not bound to policy tenant and initial state: %v", record)
	}
	id := record["candidateId"].(string)
	listed := call(http.MethodGet, "/v1/package-candidates", nil, cookies[0], "", "", false)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(id)) {
		t.Fatalf("tenant candidate list = %d: %s", listed.Code, listed.Body.String())
	}
	if got := call(http.MethodGet, "/v1/package-candidates/"+id, nil, cookies[0], "", "", false).Code; got != http.StatusOK {
		t.Fatalf("candidate detail = %d", got)
	}

	server.cfg.ApproverRole = "user"
	request := httptest.NewRequest(http.MethodGet, "https://localhost/v1/package-candidates", nil)
	request.TLS = &tls.ConnectionState{}
	request.RemoteAddr = "127.0.0.1:4321"
	request.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-maintainer candidate access = %d", response.Code)
	}
}

func TestPackageCandidatePromotionRetriesAfterRegistryOrResponseFailure(t *testing.T) {
	keys, policy := candidateWorkflowTestKeys(t)
	root := t.TempDir()
	store, err := candidateworkflow.Open(filepath.Join(root, "candidates"), policy, "runner-qualification")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry, err := manifestregistry.New(filepath.Join(root, "manifests"), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	policyBundle, err := os.ReadFile(filepath.Join("..", "..", "..", "context", "examples", "policy-bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		PilotAgentSocket: "/run/wazi-phase1/agent.sock", OperatorToken: "local-" + "fixture-" + "operator",
		ApproverID: "pilot-maintainer", ApproverRole: "maintainer",
		CapabilityPolicyBundle: candidateReleasePolicyFixture(t, policyBundle), CandidateStore: store, ManifestRegistry: registry, SigningPolicy: policy,
		Now: func() time.Time { return time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC) },
	}
	handler := New(config).Handler()
	call := func(method, path string, body []byte, cookie *http.Cookie, origin, csrf string, operator bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "https://localhost"+path, bytes.NewReader(body))
		request.TLS = &tls.ConnectionState{}
		request.RemoteAddr = "127.0.0.1:4321"
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if csrf != "" {
			request.Header.Set("X-Wazi-CSRF", csrf)
		}
		if operator {
			request.Header.Set("X-Wazi-Operator-Token", config.OperatorToken)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	bootstrap := call(http.MethodPost, "/v1/context-sessions", nil, nil, "", "", true)
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("bootstrap = %d: %s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapBody map[string]string
	if err := json.Unmarshal(bootstrap.Body.Bytes(), &bootstrapBody); err != nil {
		t.Fatal(err)
	}
	exchangeBody, _ := json.Marshal(map[string]string{"code": bootstrapBody["bootstrapCode"]})
	exchanged := call(http.MethodPost, "/v1/context-sessions/exchange", exchangeBody, nil, "https://localhost", "", false)
	if exchanged.Code != http.StatusOK || len(exchanged.Result().Cookies()) != 1 {
		t.Fatalf("exchange = %d: %s", exchanged.Code, exchanged.Body.String())
	}
	cookie := exchanged.Result().Cookies()[0]
	var session map[string]string
	if err := json.Unmarshal(exchanged.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	candidateManifest := candidateWorkflowTestManifest(t)
	value, err := verify.ParseJSON(candidateManifest)
	if err != nil {
		t.Fatal(err)
	}
	provenance := value.(map[string]any)["provenance"].(map[string]any)
	sourceID := provenance["sourceId"].(string)
	packet, err := json.Marshal(candidateworkflow.ImportRequest{
		Manifest:  candidateManifest,
		Sources:   []candidateworkflow.Source{{SourceID: sourceID, URL: sourceID, ContentHash: provenance["contentHash"].(string)}},
		Citations: []string{sourceID},
		Origin:    candidateworkflow.Origin{Assistant: "Codex chat", RequestDigest: "sha256:" + strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	imported := call(http.MethodPost, "/v1/package-candidates", packet, cookie, "https://localhost", session["csrf"], false)
	if imported.Code != http.StatusCreated {
		t.Fatalf("import = %d: %s", imported.Code, imported.Body.String())
	}
	var candidate candidateworkflow.Candidate
	if err := json.Unmarshal(imported.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{
		"schemaVersion": "1.0.0", "qualificationId": "qual_http_flow", "tenantScope": "tenant_demo",
		"candidateDigest": candidate.CandidateDigest, "result": "passed", "runnerId": "wazi-synthetic-runner",
		"runnerVersion": "1.0.0", "suiteDigest": "sha256:" + strings.Repeat("b", 64),
		"startedAt": "2026-11-03T09:00:00Z", "completedAt": "2026-11-03T09:01:00Z",
		"isolation": map[string]any{"syntheticProject": true, "network": "deny-egress", "liveVolumeAccess": false,
			"privileged": false, "cpuMilli": json.Number("500"), "ramMb": json.Number("512"), "storageMb": json.Number("1024")},
		"tests": []any{map[string]any{"testId": "schema-and-render", "result": "passed"}},
	}
	signedReceipt := candidateWorkflowSignRecord(t, keys["runner-qualification"], "runner-qualification", receipt)
	qualificationBody, _ := json.Marshal(map[string]any{"candidateDigest": candidate.CandidateDigest, "receipt": json.RawMessage(signedReceipt)})
	qualified := call(http.MethodPost, "/v1/package-candidates/"+candidate.CandidateID+"/qualification", qualificationBody, cookie, "https://localhost", session["csrf"], false)
	if qualified.Code != http.StatusOK {
		t.Fatalf("qualification = %d: %s", qualified.Code, qualified.Body.String())
	}
	reviewBody, _ := json.Marshal(map[string]string{"candidateDigest": candidate.CandidateDigest})
	reviewed := call(http.MethodPost, "/v1/package-candidates/"+candidate.CandidateID+"/review", reviewBody, cookie, "https://localhost", session["csrf"], false)
	if reviewed.Code != http.StatusOK {
		t.Fatalf("review = %d: %s", reviewed.Code, reviewed.Body.String())
	}
	var reviewedCandidate candidateworkflow.Candidate
	if err := json.Unmarshal(reviewed.Body.Bytes(), &reviewedCandidate); err != nil {
		t.Fatal(err)
	}
	signedValue, err := verify.ParseJSON(reviewedCandidate.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	signedManifestValue := signedValue.(map[string]any)
	signedManifestValue["reviewState"] = "signed"
	qualificationSigned := candidateWorkflowSignRecord(t, keys["runner-qualification"], "runner-qualification", signedManifestValue)
	deniedBody, _ := json.Marshal(map[string]any{"candidateDigest": candidate.CandidateDigest, "signedManifest": json.RawMessage(qualificationSigned)})
	denied := call(http.MethodPost, "/v1/package-candidates/"+candidate.CandidateID+"/promotion", deniedBody, cookie, "https://localhost", session["csrf"], false)
	if denied.Code != http.StatusUnprocessableEntity || len(registry.List()) != 0 {
		t.Fatalf("qualification key authorized release or mutated registry: status=%d entries=%d", denied.Code, len(registry.List()))
	}
	stillReviewed, _ := store.Get("tenant_demo", candidate.CandidateID)
	if stillReviewed.State != "reviewed" {
		t.Fatal("denied promotion advanced candidate state")
	}
	signedManifest := candidateWorkflowSignRecord(t, keys["release-signer"], "release-signer", signedManifestValue)

	// Simulate a process stop after the manifest registry is durable but before
	// the candidate snapshot records promotion. Repeating the request must
	// finish the state transition without duplicating or changing the release.
	persistedEntry, err := registry.Put(signedManifest)
	if err != nil {
		t.Fatal(err)
	}
	promotionBody, _ := json.Marshal(map[string]any{"candidateDigest": candidate.CandidateDigest, "signedManifest": json.RawMessage(signedManifest)})
	first := call(http.MethodPost, "/v1/package-candidates/"+candidate.CandidateID+"/promotion", promotionBody, cookie, "https://localhost", session["csrf"], false)
	if first.Code != http.StatusOK {
		t.Fatalf("recovered promotion = %d: %s", first.Code, first.Body.String())
	}
	var promoted candidateworkflow.Candidate
	if err := json.Unmarshal(first.Body.Bytes(), &promoted); err != nil {
		t.Fatal(err)
	}
	if promoted.State != "signed" || promoted.SignedManifestDigest != persistedEntry.Digest {
		t.Fatalf("promotion did not bind the registry entry: %+v", promoted)
	}
	second := call(http.MethodPost, "/v1/package-candidates/"+candidate.CandidateID+"/promotion", promotionBody, cookie, "https://localhost", session["csrf"], false)
	if second.Code != http.StatusOK {
		t.Fatalf("response-loss retry = %d: %s", second.Code, second.Body.String())
	}
	var repeated candidateworkflow.Candidate
	if err := json.Unmarshal(second.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.Revision != promoted.Revision || repeated.SignedManifestDigest != promoted.SignedManifestDigest {
		t.Fatal("promotion retry changed its durable record")
	}
}

func candidateReleasePolicyFixture(t *testing.T, raw []byte) []byte {
	t.Helper()
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	value.(map[string]any)["signing"].(map[string]any)["trustedSigners"] = []any{"release-signer"}
	result, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func candidateWorkflowTestPolicy(t *testing.T) verify.Policy {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(der)
	publicKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return verify.Policy{Algorithms: []string{verify.CosignBlobAlgorithm}, TrustedSigners: map[string]verify.TrustedKey{
		"runner-qualification": {KeyID: "sha256:" + hex.EncodeToString(digest[:]), PublicKeyPEM: publicKey},
	}}
}

func candidateWorkflowTestKeys(t *testing.T) (map[string]*ecdsa.PrivateKey, verify.Policy) {
	t.Helper()
	keys := map[string]*ecdsa.PrivateKey{}
	trusted := map[string]verify.TrustedKey{}
	for _, signerID := range []string{"runner-qualification", "release-signer"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(der)
		keys[signerID] = key
		trusted[signerID] = verify.TrustedKey{
			KeyID:        "sha256:" + hex.EncodeToString(digest[:]),
			PublicKeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
		}
	}
	return keys, verify.Policy{Algorithms: []string{verify.CosignBlobAlgorithm}, TrustedSigners: trusted}
}

func candidateWorkflowSignRecord(t *testing.T, key *ecdsa.PrivateKey, signerID string, record map[string]any) []byte {
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
		"algorithm": verify.CosignBlobAlgorithm, "payloadDigest": payloadDigest,
		"signedBy": signerID, "value": base64.StdEncoding.EncodeToString(signature),
	}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func candidateWorkflowTestManifest(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "releases", "phase2", "postgresql", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	manifest := value.(map[string]any)
	delete(manifest, "signature")
	manifest["reviewState"] = "candidate"
	canonical, err := verify.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
