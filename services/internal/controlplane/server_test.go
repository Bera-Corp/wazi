package controlplane_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/approvalregistry"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/controlplane"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// --- helpers ---

type fixture struct {
	ca       *pairing.CA
	registry *pairing.Registry
	server   *controlplane.Server
	ts       *httptest.Server
	baseURL  string
}

func newMTLSFixture(t *testing.T) *fixture {
	return newMTLSFixtureWithPolicy(t, nil)
}

func newMTLSFixtureWithPolicy(t *testing.T, policyBundle []byte) *fixture {
	t.Helper()
	ca, err := pairing.GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.IssueServerCert([]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	registry := pairing.NewRegistry(nil)
	server := controlplane.New(controlplane.Config{
		Registry: registry, CA: ca, CapabilityPolicyBundle: policyBundle,
	})
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = server.TLSConfig(serverCert)
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return &fixture{ca: ca, registry: registry, server: server, ts: ts, baseURL: ts.URL}
}

func (f *fixture) trustClient(t *testing.T, serverName string) *http.Client {
	t.Helper()
	cfg, err := pairing.ClientTLSConfig(f.ca, nil, nil, serverName)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

func (f *fixture) certClient(t *testing.T, certPEM, keyPEM []byte, serverName string) *http.Client {
	t.Helper()
	cfg, err := pairing.ClientTLSConfig(f.ca, certPEM, keyPEM, serverName)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

// enrollViaHTTP runs the real pairing exchange and returns the
// node's certificate + its locally generated private key.
func (f *fixture) enrollViaHTTP(t *testing.T, tenant, hostID string) (certPEM, keyPEM []byte) {
	t.Helper()
	code, err := f.registry.CreatePairingCode(tenant, hostID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, csrPEM, err := pairing.GenerateNodeKeyAndCSR(hostID)
	if err != nil {
		t.Fatal(err)
	}
	status, body := f.enrollWithCode(t, code, tenant, hostID, csrPEM)
	if status != http.StatusCreated {
		t.Fatalf("enroll status = %d, body = %s", status, body)
	}
	var resp struct {
		CertificatePEM string `json:"certificatePem"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	return []byte(resp.CertificatePEM), keyPEM
}

// enrollAttempt runs one enrollment exchange with a caller-supplied
// code and returns status, body and (on success) the issued pair.
func (f *fixture) enrollAttempt(t *testing.T, code, tenant, hostID string) (int, string, []byte, []byte) {
	t.Helper()
	keyPEM, csrPEM, err := pairing.GenerateNodeKeyAndCSR(hostID)
	if err != nil {
		t.Fatal(err)
	}
	status, body := f.enrollWithCode(t, code, tenant, hostID, csrPEM)
	if status != http.StatusCreated {
		return status, body, nil, nil
	}
	var resp struct {
		CertificatePEM string `json:"certificatePem"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	return status, body, []byte(resp.CertificatePEM), keyPEM
}

func parseCertPEM(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no cert PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func (f *fixture) enrollWithCode(t *testing.T, code, tenant, hostID string, csrPEM []byte) (int, string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{
		"pairingCode": code, "tenantScope": tenant, "hostId": hostID, "csrPem": string(csrPEM),
	})
	resp, err := f.trustClient(t, "localhost").Post(f.baseURL+"/v1/pairing/enroll", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("enroll request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func validProfile(tenant, hostID string) contracts.HostProfile {
	now := time.Now().UTC()
	var p contracts.HostProfile
	p.SchemaVersion = "1.0.0"
	p.HostID = hostID
	p.TenantScope = tenant
	p.ObservedAt = now.Add(-time.Minute).Format(time.RFC3339)
	p.ExpiresAt = now.Add(29 * time.Minute).Format(time.RFC3339)
	p.OS.Family = "linux"
	p.OS.Version = "24.04"
	p.Architecture = "x86_64"
	p.Resources.CPUCores = 4
	p.Resources.RAMMb = 8192
	p.Resources.StorageFreeMb = 51200
	p.Docker.EngineVersion = "29.1.3"
	p.Docker.ComposeVersion = "2.40.3"
	p.OccupiedPorts = []int{22}
	p.Filesystem.Type = "ext4"
	p.Filesystem.DockerRootWritable = true
	p.Network.Mode = "byo-vps"
	p.Provenance.SourceType = "host-agent-probe"
	p.Provenance.ExtractionMethod = "host-probe"
	p.Redaction.Applied = true
	return p
}

func postProfile(client *http.Client, baseURL string, profile contracts.HostProfile) (int, string, error) {
	body, _ := json.Marshal(profile)
	resp, err := client.Post(baseURL+"/v1/host-profiles", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data), nil
}

func getProfiles(client *http.Client, baseURL string) (int, map[string]contracts.HostProfile, error) {
	resp, err := client.Get(baseURL + "/v1/host-profiles")
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	var out map[string]contracts.HostProfile
	err = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, err
}

// --- mTLS acceptance matrix ---

func TestValidCertBoundToTenantAndNodeAccepted(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	client := f.certClient(t, certPEM, keyPEM, "localhost")

	status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha"))
	if err != nil || status != http.StatusAccepted {
		t.Fatalf("valid bound submission = %d, %v, %s; want 202", status, err, body)
	}
	status, profiles, err := getProfiles(client, f.baseURL)
	if err != nil || status != http.StatusOK || len(profiles) != 1 {
		t.Fatalf("list = %d, %v, %v; want 200 with exactly own profile", status, profiles, err)
	}
	if _, ok := profiles["host_alpha"]; !ok {
		t.Errorf("own profile missing from list: %v", profiles)
	}
}

func TestMissingClientCertRejected(t *testing.T) {
	f := newMTLSFixture(t)
	client := f.trustClient(t, "localhost")
	// The enrollment endpoint must stay reachable without a cert
	// (bootstrap), so profile routes reject a missing cert at the
	// handler with 401 instead of failing the handshake.
	status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha"))
	if err != nil {
		t.Fatalf("missing-cert request should reach the handler, got: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Errorf("missing-cert submit = %d (%s), want 401", status, body)
	}
	if status, _ := getStatus(t, client, f.baseURL+"/v1/host-profiles"); status != http.StatusUnauthorized {
		t.Errorf("missing-cert list = %d, want 401", status)
	}
}

func getStatus(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestUntrustedCertRejected(t *testing.T) {
	f := newMTLSFixture(t)
	otherCA, err := pairing.GenerateCA("attacker-ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, _, err := otherCA.IssueClientCert("tenant_a", "host_alpha", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pairing.ClientTLSConfig(f.ca, certPEM, keyPEM, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
	// Either the handshake fails on the untrusted chain, or the
	// client withholds the non-matching cert and the handler
	// returns 401. Both are rejections; acceptance (202) is not.
	status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha"))
	if err == nil && status == http.StatusAccepted {
		t.Fatalf("untrusted-CA client certificate was accepted: %d %s", status, body)
	}
	if err == nil && status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Errorf("untrusted cert = %d (%s), want handshake failure or 401/403", status, body)
	}
}

func TestWrongServerIdentityRejected(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	client := f.certClient(t, certPEM, keyPEM, "wrong.example")
	if _, _, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha")); err == nil {
		t.Fatal("connection with the wrong expected server identity was not rejected")
	} else if !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Errorf("wrong-identity error should be a certificate error, got: %v", err)
	}
}

func TestExpiredClientCertRejected(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM, err := f.ca.IssueExpiredClientCert("tenant_a", "host_alpha")
	if err != nil {
		t.Fatal(err)
	}
	client := f.certClient(t, certPEM, keyPEM, "localhost")
	if _, _, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha")); err == nil {
		t.Fatal("expired client certificate was not rejected")
	}
}

func TestValidButUnenrolledCertRejected(t *testing.T) {
	f := newMTLSFixture(t)
	// Signed by the trusted CA and correctly bound, but never
	// enrolled: a valid certificate alone is not authorisation.
	certPEM, keyPEM, _, err := f.ca.IssueClientCert("tenant_a", "host_ghost", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	client := f.certClient(t, certPEM, keyPEM, "localhost")
	status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_ghost"))
	if err != nil {
		t.Fatalf("unenrolled cert request failed at handshake (should reach handler): %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Errorf("unenrolled cert status = %d (%s), want 401", status, body)
	}
}

func TestEnrollmentReplayRejected(t *testing.T) {
	f := newMTLSFixture(t)
	code, err := f.registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, csrPEM, err := pairing.GenerateNodeKeyAndCSR("host_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if status, body := f.enrollWithCode(t, code, "tenant_a", "host_alpha", csrPEM); status != http.StatusCreated {
		t.Fatalf("first enroll = %d (%s), want 201", status, body)
	}
	_, csr2, err := pairing.GenerateNodeKeyAndCSR("host_alpha")
	if err != nil {
		t.Fatal(err)
	}
	status, body := f.enrollWithCode(t, code, "tenant_a", "host_alpha", csr2)
	if status != http.StatusUnauthorized || !strings.Contains(body, "already used") {
		t.Errorf("replayed enroll = %d (%s), want 401 already used", status, body)
	}
}

func TestEnrollmentWrongBindingRejected(t *testing.T) {
	f := newMTLSFixture(t)
	code, err := f.registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, csrPEM, err := pairing.GenerateNodeKeyAndCSR("host_alpha")
	if err != nil {
		t.Fatal(err)
	}
	status, body := f.enrollWithCode(t, code, "tenant_b", "host_alpha", csrPEM)
	if status != http.StatusForbidden {
		t.Errorf("cross-tenant enroll = %d (%s), want 403", status, body)
	}
}

func TestRevokedNodeRejected(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	client := f.certClient(t, certPEM, keyPEM, "localhost")
	if status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusAccepted {
		t.Fatalf("pre-revoke submit = %d, %v, %s", status, err, body)
	}
	if err := f.registry.Revoke("tenant_a", "host_alpha"); err != nil {
		t.Fatal(err)
	}
	status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha"))
	if err != nil {
		t.Fatalf("revoked request should reach the handler, got handshake error: %v", err)
	}
	if status != http.StatusForbidden || !strings.Contains(body, "revoked") {
		t.Errorf("revoked submit = %d (%s), want 403 revoked", status, body)
	}
}

// Reproduced gap: two codes minted, enroll with one, revoke, then
// the spare code must not regain access. Only a code minted after
// revocation (fresh operator approval) may re-enroll the node.
func TestRevokedNodeSpareCodeCannotRegainAccess(t *testing.T) {
	f := newMTLSFixture(t)
	codeUsed, err := f.registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	codeSpare, err := f.registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	status, body, certPEM, keyPEM := f.enrollAttempt(t, codeUsed, "tenant_a", "host_alpha")
	if status != http.StatusCreated {
		t.Fatalf("initial enroll = %d (%s)", status, body)
	}
	client := f.certClient(t, certPEM, keyPEM, "localhost")
	if status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusAccepted {
		t.Fatalf("initial submit = %d, %v, %s", status, err, body)
	}
	if err := f.registry.Revoke("tenant_a", "host_alpha"); err != nil {
		t.Fatal(err)
	}
	status, body, _, _ = f.enrollAttempt(t, codeSpare, "tenant_a", "host_alpha")
	if status != http.StatusUnauthorized || !strings.Contains(body, "invalidated") {
		t.Errorf("spare-code enroll after revoke = %d (%s), want 401 invalidated", status, body)
	}
	if status, body, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusForbidden {
		t.Errorf("revoked submit = %d, %v (%s), want 403", status, err, body)
	}
	codeFresh, err := f.registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	status, body, cert2, key2 := f.enrollAttempt(t, codeFresh, "tenant_a", "host_alpha")
	if status != http.StatusCreated {
		t.Fatalf("fresh-code re-enroll = %d (%s), want 201", status, body)
	}
	client2 := f.certClient(t, cert2, key2, "localhost")
	if status, body, err := postProfile(client2, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusAccepted {
		t.Errorf("re-enrolled submit = %d, %v (%s), want 202", status, err, body)
	}
	if status, _, err := postProfile(client, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status == http.StatusAccepted {
		t.Errorf("old revoked cert after re-enrollment = %d, %v; must stay rejected", status, err)
	}
}

// blockingBody pauses the first Read until released, simulating a
// profile upload that is in flight while revocation lands.
type blockingBody struct {
	data    []byte
	started chan struct{}
	release chan struct{}
	once    sync.Once
	off     int
}

func (b *blockingBody) Read(p []byte) (int, error) {
	b.once.Do(func() {
		close(b.started)
		<-b.release
	})
	if b.off >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.off:])
	b.off += n
	return n, nil
}
func (b *blockingBody) Close() error { return nil }

// Reproduced gap: authenticate, pause mid-body, revoke, resume.
// The write must observe the revocation at commit and store
// nothing. A handler-level second Authenticate would still race;
// only the atomic authorize+commit closes this.
func TestInFlightProfileCannotCommitAfterRevocation(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, _ := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	cert := parseCertPEM(t, certPEM)

	profileJSON, err := json.Marshal(validProfile("tenant_a", "host_alpha"))
	if err != nil {
		t.Fatal(err)
	}
	body := &blockingBody{data: profileJSON, started: make(chan struct{}), release: make(chan struct{})}
	req := httptest.NewRequest(http.MethodPost, "https://localhost/v1/host-profiles", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:4321"
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.server.Handler().ServeHTTP(recorder, req)
	}()

	select {
	case <-body.started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started reading the body (fail-fast auth may have rejected)")
	}
	// The request authenticated and is now parked mid-body.
	// Revocation must be able to complete here...
	if err := f.registry.Revoke("tenant_a", "host_alpha"); err != nil {
		t.Fatal(err)
	}
	close(body.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not finish after body release")
	}
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "revoked") {
		t.Errorf("in-flight commit after revoke = %d (%s), want 403 revoked", recorder.Code, recorder.Body.String())
	}
	// ...and nothing may have been stored. Re-enroll freshly and
	// confirm the tenant listing is empty.
	codeFresh, err := f.registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert2, key2 := f.enrollAttempt(t, codeFresh, "tenant_a", "host_alpha")
	client2 := f.certClient(t, cert2, key2, "localhost")
	_, profiles, err := getProfiles(client2, f.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("blocked in-flight write leaked into storage: %v", profiles)
	}
}

// Presence/nullability must be enforced over real mTLS intake, not
// only by unit-level validation (value structs hid omitted fields).
func TestIntakePresenceAndNullabilityOverMTLS(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	client := f.certClient(t, certPEM, keyPEM, "localhost")

	valid, err := json.Marshal(validProfile("tenant_a", "host_alpha"))
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(t *testing.T, fn func(map[string]any)) []byte {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(valid, &doc); err != nil {
			t.Fatal(err)
		}
		fn(doc)
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	nested := func(doc map[string]any, parent string) map[string]any {
		child, _ := doc[parent].(map[string]any)
		return child
	}
	postRawProfile := func(raw []byte) (int, string) {
		resp, err := client.Post(f.baseURL+"/v1/host-profiles", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("post raw profile: %v", err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}

	cases := []struct {
		name string
		fn   func(map[string]any)
	}{
		{"omit ramMb", func(d map[string]any) { delete(nested(d, "resources"), "ramMb") }},
		{"omit storageFreeMb", func(d map[string]any) { delete(nested(d, "resources"), "storageFreeMb") }},
		{"null ramMb", func(d map[string]any) { nested(d, "resources")["ramMb"] = nil }},
		{"omit secretValuesPresent", func(d map[string]any) { delete(nested(d, "redaction"), "secretValuesPresent") }},
		{"null secretValuesPresent", func(d map[string]any) { nested(d, "redaction")["secretValuesPresent"] = nil }},
		{"null network", func(d map[string]any) { d["network"] = nil }},
		{"null resources", func(d map[string]any) { d["resources"] = nil }},
		{"omit expiresAt", func(d map[string]any) { delete(d, "expiresAt") }},
	}
	for _, tc := range cases {
		if status, body := postRawProfile(mutate(t, tc.fn)); status != http.StatusBadRequest {
			t.Errorf("%s over mTLS = %d (%s), want 400", tc.name, status, body)
		}
	}
	if status, body := postRawProfile(valid); status != http.StatusAccepted {
		t.Errorf("valid profile over mTLS = %d (%s), want 202", status, body)
	}
}

// Intake -> storage -> GET must preserve schema validity: a valid
// profile that omits occupiedPorts and uses network:{} must not
// come back with "occupiedPorts":null or "mode":"". Case aliases
// are rejected over the same real mTLS path.
func TestIntakeStorageListingSchemaRoundTrip(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	client := f.certClient(t, certPEM, keyPEM, "localhost")
	postRawProfile := func(raw []byte) (int, string) {
		resp, err := client.Post(f.baseURL+"/v1/host-profiles", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("post raw: %v", err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}

	valid, err := json.Marshal(validProfile("tenant_a", "host_alpha"))
	if err != nil {
		t.Fatal(err)
	}
	// Alias probe from the review: extra "Redaction" object.
	alias := strings.Replace(string(valid), `"redaction":`, `"Redaction":{"secretValuesPresent":null},"redaction":`, 1)
	if status, body := postRawProfile([]byte(alias)); status != http.StatusBadRequest {
		t.Errorf("Redaction alias over mTLS = %d (%s), want 400", status, body)
	}
	// Explicitly supplied empty mode is schema-invalid.
	var doc map[string]any
	if err := json.Unmarshal(valid, &doc); err != nil {
		t.Fatal(err)
	}
	doc["network"] = map[string]any{"mode": ""}
	emptyMode, _ := json.Marshal(doc)
	if status, body := postRawProfile(emptyMode); status != http.StatusBadRequest {
		t.Errorf("explicit empty mode over mTLS = %d (%s), want 400", status, body)
	}

	// Valid minimal-optionals profile: no occupiedPorts, network:{}.
	delete(doc, "occupiedPorts")
	doc["network"] = map[string]any{}
	minimal, _ := json.Marshal(doc)
	if status, body := postRawProfile(minimal); status != http.StatusAccepted {
		t.Fatalf("minimal-optionals profile = %d (%s), want 202", status, body)
	}
	resp, err := client.Get(f.baseURL + "/v1/host-profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	listing, _ := io.ReadAll(resp.Body)
	// CI gate: when asked, hand the exact listing bytes to the
	// independent Python canonical-schema validator
	// (scripts/validate_go_profile.py), so round-trip validity is
	// checked by the schema itself, not only Go-side stand-ins.
	if out := os.Getenv("WAZI_ROUNDTRIP_OUT"); out != "" {
		if err := os.WriteFile(out, listing, 0o644); err != nil {
			t.Fatalf("write round-trip artifact: %v", err)
		}
	}
	if strings.Contains(string(listing), `"occupiedPorts":null`) {
		t.Errorf("listing emitted occupiedPorts null: %s", listing)
	}
	if strings.Contains(string(listing), `"mode":""`) {
		t.Errorf("listing emitted empty mode: %s", listing)
	}
	// The listed record must re-decode and re-validate (the Go-side
	// stand-in for canonical schema validation of the round-trip).
	var listed map[string]contracts.HostProfile
	if err := json.Unmarshal(listing, &listed); err != nil {
		t.Fatal(err)
	}
	stored, ok := listed["host_alpha"]
	if !ok {
		t.Fatalf("stored profile missing from listing: %s", listing)
	}
	reserialised, _ := json.Marshal(stored)
	again, err := contracts.DecodeHostProfile(reserialised)
	if err != nil {
		t.Fatalf("listed profile failed re-decode: %v (%s)", err, reserialised)
	}
	if err := contracts.ValidateHostProfile(again, time.Now().UTC()); err != nil {
		t.Fatalf("listed profile failed re-validation: %v", err)
	}
}

func TestCrossTenantAndCrossNodeSubmissionRejected(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	client := f.certClient(t, certPEM, keyPEM, "localhost")

	for _, profile := range []contracts.HostProfile{
		validProfile("tenant_b", "host_alpha"), // cross-tenant
		validProfile("tenant_a", "host_other"), // cross-node
		validProfile("tenant_b", "host_other"), // both
	} {
		status, body, err := postProfile(client, f.baseURL, profile)
		if err != nil || status != http.StatusForbidden {
			t.Errorf("submit tenant=%s host=%s = %d, %v (%s); want 403", profile.TenantScope, profile.HostID, status, err, body)
		}
	}
	// Nothing cross-tenant was stored under the victim tenant.
	if _, profiles, _ := getProfiles(client, f.baseURL); len(profiles) != 0 {
		t.Errorf("cross-tenant submissions leaked into storage: %v", profiles)
	}
}

func TestTenantScopedListing(t *testing.T) {
	f := newMTLSFixture(t)
	certA, keyA := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	certB, keyB := f.enrollViaHTTP(t, "tenant_b", "host_beta")
	clientA := f.certClient(t, certA, keyA, "localhost")
	clientB := f.certClient(t, certB, keyB, "localhost")
	if status, _, err := postProfile(clientA, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusAccepted {
		t.Fatalf("tenant_a submit = %d, %v", status, err)
	}
	if status, _, err := postProfile(clientB, f.baseURL, validProfile("tenant_b", "host_beta")); err != nil || status != http.StatusAccepted {
		t.Fatalf("tenant_b submit = %d, %v", status, err)
	}
	_, profilesA, err := getProfiles(clientA, f.baseURL)
	if err != nil || len(profilesA) != 1 {
		t.Fatalf("tenant_a list = %v, %v; want exactly 1", profilesA, err)
	}
	if _, leaked := profilesA["host_beta"]; leaked {
		t.Error("tenant_a can see tenant_b's profile")
	}
}

func TestCapabilityPolicyFetchRequiresPairedTenantAndEnvelopeDigest(t *testing.T) {
	raw := canonicalContextExample(t, "policy-bundle.json")
	bundle, err := contracts.DecodePolicyBundle(raw)
	if err != nil {
		t.Fatalf("decode policy example: %v", err)
	}
	digest, err := verify.CanonicalDigest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	f := newMTLSFixtureWithPolicy(t, raw)
	nodeCert, nodeKey := f.enrollViaHTTP(t, "tenant_demo", "host_policy")
	tlsConfig, err := pairing.ClientTLSConfig(f.ca, nodeCert, nodeKey, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	expected := agentclient.CapabilityPolicyReference{
		PolicyID: "pol_default", Version: "1.0.0", Digest: digest, TenantScope: "tenant_demo",
	}
	policy, err := agentclient.FetchCapabilityPolicyBundle(context.Background(), f.baseURL, tlsConfig, expected)
	if err != nil {
		t.Fatalf("paired tenant policy fetch rejected: %v", err)
	}
	if got := policy.BindingSnapshot(); got.BundleID != expected.PolicyID || got.Digest != expected.Digest {
		t.Fatalf("fetched policy identity mismatch: %+v", got)
	}
	if got := policy.ApprovalRequirements()["rollback"]; got != "user" {
		t.Fatalf("rollback approval requirement = %q, want user", got)
	}

	wrongDigest := expected
	wrongDigest.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := agentclient.FetchCapabilityPolicyBundle(context.Background(), f.baseURL, tlsConfig, wrongDigest); err == nil {
		t.Fatal("capability policy with a digest different from the envelope reference was accepted")
	}

	otherCert, otherKey := f.enrollViaHTTP(t, "tenant_other", "host_other")
	otherTLS, err := pairing.ClientTLSConfig(f.ca, otherCert, otherKey, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	otherExpected := expected
	otherExpected.TenantScope = "tenant_other"
	if _, err := agentclient.FetchCapabilityPolicyBundle(context.Background(), f.baseURL, otherTLS, otherExpected); err == nil {
		t.Fatal("cross-tenant capability policy read was accepted")
	}

	response, err := f.trustClient(t, "localhost").Get(f.baseURL + "/v1/policy-bundles/pol_default/1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unpaired policy read status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestApprovalIssueRequiresOperatorAndConsumptionIsPairedSingleUse(t *testing.T) {
	f, approvalStore := newApprovalMTLSFixture(t)
	plan := canonicalContextExample(t, "deployment-plan.json")
	envelope := canonicalContextExample(t, "execution-context-envelope.json")
	planValue := parseContextObject(t, plan)
	envelopeValue := parseContextObject(t, envelope)
	planDigest, err := verify.CanonicalDigest(planValue)
	if err != nil {
		t.Fatal(err)
	}
	envelopeDigest, err := verify.CanonicalDigest(envelopeValue)
	if err != nil {
		t.Fatal(err)
	}
	issueBody, err := json.Marshal(map[string]any{
		"caseId": "case_demo01", "plan": json.RawMessage(plan),
		"envelope": json.RawMessage(envelope), "hostId": "host_demo01",
		"action": "public-exposure", "decision": "approved", "validForSeconds": 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_demo", "host_demo01")
	client := f.certClient(t, certPEM, keyPEM, "localhost")
	operatorURL := f.baseURL + "/v1/approvals"
	if status, body := postRaw(t, client, operatorURL, "application/json", string(issueBody)); status != http.StatusUnauthorized {
		t.Fatalf("approval without operator token = %d (%s), want 401", status, body)
	}
	wrongActionBody := bytes.Replace(issueBody, []byte(`"action":"public-exposure"`), []byte(`"action":"install"`), 1)
	wrongActionRequest, err := http.NewRequest(http.MethodPost, operatorURL, bytes.NewReader(wrongActionBody))
	if err != nil {
		t.Fatal(err)
	}
	wrongActionRequest.Header.Set("Content-Type", "application/json")
	wrongActionRequest.Header.Set("X-Wazi-Operator-Token", testApprovalOperatorAuth())
	wrongActionResponse, err := client.Do(wrongActionRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = wrongActionResponse.Body.Close()
	if wrongActionResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("approval for an undeclared action = %d, want 400", wrongActionResponse.StatusCode)
	}
	operatorRequest, err := http.NewRequest(http.MethodPost, operatorURL, bytes.NewReader(issueBody))
	if err != nil {
		t.Fatal(err)
	}
	operatorRequest.Header.Set("Content-Type", "application/json")
	operatorRequest.Header.Set("X-Wazi-Operator-Token", testApprovalOperatorAuth())
	operatorResponse, err := client.Do(operatorRequest)
	if err != nil {
		t.Fatal(err)
	}
	approvalRaw, err := io.ReadAll(operatorResponse.Body)
	_ = operatorResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if operatorResponse.StatusCode != http.StatusCreated {
		t.Fatalf("approval issue = %d (%s), want 201", operatorResponse.StatusCode, approvalRaw)
	}
	var approval contracts.ApprovalRecord
	if err := json.Unmarshal(approvalRaw, &approval); err != nil {
		t.Fatal(err)
	}
	if approval.CaseID != "case_demo01" || approval.Approver.ID != "local_operator" || approval.Approver.Role != "user" {
		t.Fatalf("approval identity or case binding is wrong: %+v", approval)
	}
	if operatorResponse.Header.Get("ETag") == "" {
		t.Fatal("approval response has no canonical digest ETag")
	}

	consumeURL := f.baseURL + "/v1/approvals/" + approval.ApprovalID + "/consume"
	consume := func(jobID, caseID string) (int, string) {
		payload, marshalErr := json.Marshal(map[string]string{
			"jobId": jobID, "caseId": caseID, "planId": approval.PlanID,
			"planDigest": planDigest, "envelopeId": approval.EnvelopeID,
			"envelopeDigest": envelopeDigest, "applicationId": approval.ApplicationID,
			"action": approval.Action,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return postRaw(t, client, consumeURL, "application/json", string(payload))
	}
	otherCert, otherKey := f.enrollViaHTTP(t, "tenant_demo", "host_other")
	otherClient := f.certClient(t, otherCert, otherKey, "localhost")
	otherPayload, err := json.Marshal(map[string]string{
		"jobId": "job_wrong_host", "caseId": approval.CaseID, "planId": approval.PlanID,
		"planDigest": planDigest, "envelopeId": approval.EnvelopeID,
		"envelopeDigest": envelopeDigest, "applicationId": approval.ApplicationID,
		"action": approval.Action,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, body := postRaw(t, otherClient, consumeURL, "application/json", string(otherPayload)); status != http.StatusForbidden {
		t.Fatalf("cross-host approval consume = %d (%s), want 403", status, body)
	}
	if status, body := consume("job_demo02", "case_other"); status != http.StatusForbidden {
		t.Fatalf("cross-case approval consume = %d (%s), want 403", status, body)
	}
	if status, body := consume("job_demo01", "case_demo01"); status != http.StatusOK {
		t.Fatalf("paired approval consume = %d (%s), want 200", status, body)
	}
	if status, body := consume("job_demo01", "case_demo01"); status != http.StatusOK {
		t.Fatalf("same-job approval retry = %d (%s), want idempotent 200", status, body)
	}
	if status, body := consume("job_demo02", "case_demo01"); status != http.StatusConflict {
		t.Fatalf("approval replay under a different job = %d (%s), want 409", status, body)
	}
	if _, entry, ok := approvalStore.Get(approval.ApprovalID); !ok || entry.ConsumedBy != "job_demo01" {
		t.Fatalf("approval consumption was not stored: %+v, found=%v", entry, ok)
	}
}

func TestDistinctApproverCredentialsReachAgentQuorum(t *testing.T) {
	ca, err := pairing.GenerateCA("approval-quorum-test-ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.IssueServerCert([]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 21, 9, 5, 0, 0, time.UTC)
	approvalStore, err := approvalregistry.New(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := approvalStore.Close(); err != nil {
			t.Errorf("close approval registry: %v", err)
		}
	})
	registry := pairing.NewRegistry(nil)
	credentialA := "approval-" + strings.Repeat("a", 32)
	credentialB := "approval-" + strings.Repeat("b", 32)
	server := controlplane.New(controlplane.Config{
		Registry: registry, CA: ca, ApprovalRegistry: approvalStore,
		ApproverCredentials: []controlplane.ApproverCredential{
			approverCredentialForTest("maintainer_one", "maintainer", credentialA),
			approverCredentialForTest("maintainer_two", "maintainer", credentialB),
		},
		Now: func() time.Time { return now },
	})
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = server.TLSConfig(serverCert)
	ts.StartTLS()
	t.Cleanup(ts.Close)
	f := &fixture{ca: ca, registry: registry, server: server, ts: ts, baseURL: ts.URL}

	plan := canonicalContextExample(t, "deployment-plan.json")
	envelope := canonicalContextExample(t, "execution-context-envelope.json")
	planValue := parseContextObject(t, plan)
	envelopeValue := parseContextObject(t, envelope)
	planDigest, err := verify.CanonicalDigest(planValue)
	if err != nil {
		t.Fatal(err)
	}
	envelopeDigest, err := verify.CanonicalDigest(envelopeValue)
	if err != nil {
		t.Fatal(err)
	}
	issueBody, err := json.Marshal(map[string]any{
		"caseId": "case_demo01", "plan": json.RawMessage(plan),
		"envelope": json.RawMessage(envelope), "hostId": "host_demo01",
		"action": "public-exposure", "decision": "approved", "validForSeconds": 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM = f.enrollViaHTTP(t, "tenant_demo", "host_demo01")
	issueClient := f.certClient(t, certPEM, keyPEM, "localhost")
	issue := func(credential string) contracts.ApprovalRecord {
		request, requestErr := http.NewRequest(http.MethodPost, f.baseURL+"/v1/approvals", bytes.NewReader(issueBody))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Wazi-Approver-Credential", credential)
		response, requestErr := issueClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("issue approval = %d (%s), want 201", response.StatusCode, body)
		}
		var record contracts.ApprovalRecord
		if err := json.Unmarshal(body, &record); err != nil {
			t.Fatal(err)
		}
		return record
	}

	operatorOnly, err := http.NewRequest(http.MethodPost, f.baseURL+"/v1/approvals", bytes.NewReader(issueBody))
	if err != nil {
		t.Fatal(err)
	}
	operatorOnly.Header.Set("Content-Type", "application/json")
	operatorOnly.Header.Set("X-Wazi-Operator-Token", testApprovalOperatorAuth())
	operatorResponse, err := issueClient.Do(operatorOnly)
	if err != nil {
		t.Fatal(err)
	}
	_ = operatorResponse.Body.Close()
	if operatorResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy operator credential bypassed configured approver credentials: %d", operatorResponse.StatusCode)
	}

	first := issue(credentialA)
	second := issue(credentialB)
	if first.Approver.ID == second.Approver.ID || first.Approver.Role != "maintainer" || second.Approver.Role != "maintainer" {
		t.Fatalf("approval records did not preserve distinct maintainer identities: %+v %+v", first.Approver, second.Approver)
	}
	clientTLS, err := pairing.ClientTLSConfig(ca, certPEM, keyPEM, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	expected := agentclient.ApprovalExpectation{
		CaseID: "case_demo01", PlanID: planValue["planId"].(string), PlanDigest: planDigest,
		EnvelopeID: envelopeValue["envelopeId"].(string), EnvelopeDigest: envelopeDigest,
		TenantScope: "tenant_demo", HostID: "host_demo01",
		ApplicationID:   envelopeValue["applicationId"].(string),
		RequiredActions: []string{"public-exposure"},
		Requirements:    map[string]string{"public-exposure": "two-maintainers"},
		Now:             now, MaxValidity: controlplane.MaxApprovalLifetime,
	}
	references := []agentclient.ApprovalReference{
		{ApprovalID: first.ApprovalID, Action: "public-exposure"},
		{ApprovalID: second.ApprovalID, Action: "public-exposure"},
	}
	records, err := agentclient.ConsumeAndVerifyApprovalRecords(
		context.Background(), f.baseURL, clientTLS, "job_quorum01", references, expected,
	)
	if err != nil {
		t.Fatalf("consume and verify two-maintainer quorum: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("consumed %d approval records, want 2", len(records))
	}
	retried, err := agentclient.ConsumeAndVerifyApprovalRecords(
		context.Background(), f.baseURL, clientTLS, "job_quorum01", references, expected,
	)
	if err != nil || len(retried) != 2 {
		t.Fatalf("same-job approval retry = %d records, %v", len(retried), err)
	}
}

func approverCredentialForTest(id, role, credential string) controlplane.ApproverCredential {
	digest := sha256.Sum256([]byte(credential))
	return controlplane.ApproverCredential{
		ID: id, Role: role, CredentialSHA256: "sha256:" + hex.EncodeToString(digest[:]),
	}
}

func newApprovalMTLSFixture(t *testing.T) (*fixture, *approvalregistry.Registry) {
	t.Helper()
	ca, err := pairing.GenerateCA("approval-test-ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.IssueServerCert([]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 21, 9, 5, 0, 0, time.UTC)
	approvalStore, err := approvalregistry.New(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := approvalStore.Close(); err != nil {
			t.Errorf("close approval registry: %v", err)
		}
	})
	registry := pairing.NewRegistry(nil)
	config := controlplane.Config{
		Registry: registry, CA: ca, ApprovalRegistry: approvalStore,
		ApproverID: "local_operator", ApproverRole: "user", Now: func() time.Time { return now },
	}
	auth := testApprovalOperatorAuth()
	config.OperatorToken = auth
	server := controlplane.New(config)
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = server.TLSConfig(serverCert)
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return &fixture{ca: ca, registry: registry, server: server, ts: ts, baseURL: ts.URL}, approvalStore
}

func testApprovalOperatorAuth() string {
	return "approval-" + "operator-test"
}

func canonicalContextExample(t *testing.T, name string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "context", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func parseContextObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	value, err := verify.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatal("context example must be a JSON object")
	}
	return object
}

// --- Intake validation / limits (plain loopback dev mode) ---

func newPlainFixture(t *testing.T) (*controlplane.Server, *httptest.Server) {
	t.Helper()
	server := controlplane.New(controlplane.Config{AllowPlaintextProfiles: true})
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return server, ts
}

func postRaw(t *testing.T, client *http.Client, url, contentType, body string) (int, string) {
	t.Helper()
	resp, err := client.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func TestIntakeValidationAndLimits(t *testing.T) {
	_, ts := newPlainFixture(t)
	client := ts.Client()
	url := ts.URL + "/v1/host-profiles"

	valid, _ := json.Marshal(validProfile("tenant_a", "host_alpha"))
	if status, body := postRaw(t, client, url, "application/json", string(valid)); status != http.StatusAccepted {
		t.Fatalf("valid plain submit = %d (%s), want 202", status, body)
	}

	cases := []struct {
		name string
		body string
	}{
		{"unknown field", strings.Replace(string(valid), `"schemaVersion"`, `"bogus":1,"schemaVersion"`, 1)},
		{"trailing json", string(valid) + ` {}`},
		{"missing tenant", strings.Replace(string(valid), `"tenantScope":"tenant_a",`, ``, 1)},
		{"prerelease engine", strings.Replace(string(valid), `"engineVersion":"29.1.3"`, `"engineVersion":"27.3.1-rc1"`, 1)},
		{"not json", `{`},
	}
	for _, tc := range cases {
		if status, body := postRaw(t, client, url, "application/json", tc.body); status != http.StatusBadRequest {
			t.Errorf("%s: status = %d (%s), want 400", tc.name, status, body)
		}
	}

	expired := validProfile("tenant_a", "host_alpha")
	expired.ObservedAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	expired.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	expiredBody, _ := json.Marshal(expired)
	if status, body := postRaw(t, client, url, "application/json", string(expiredBody)); status != http.StatusBadRequest || !strings.Contains(body, "expired") {
		t.Errorf("expired profile = %d (%s), want 400 expired", status, body)
	}

	oversized := string(valid) + strings.Repeat(" ", controlplane.MaxProfileBodyBytes)
	if status, _ := postRaw(t, client, url, "application/json", oversized); status != http.StatusBadRequest {
		t.Errorf("oversized body status = %d, want 400 (body limit)", status)
	}
	if status, _ := postRaw(t, client, url, "text/plain", string(valid)); status != http.StatusBadRequest {
		t.Errorf("wrong content-type status = %d, want 400", status)
	}
}

func TestOperatorCodeMintAndRevokeEndpoints(t *testing.T) {
	_, ts := newPlainFixture(t)
	client := ts.Client()
	status, body := postRaw(t, client, ts.URL+"/v1/pairing/codes", "application/json", `{"tenantScope":"tenant_a","hostId":"host_alpha"}`)
	if status != http.StatusCreated || !strings.Contains(body, "pairingCode") {
		t.Fatalf("mint code = %d (%s), want 201 with pairingCode", status, body)
	}
	// Plain fixture has no CA, so enrollment itself is unavailable;
	// revocation of a never-enrolled node is a clean 404.
	status, _ = postRaw(t, client, ts.URL+"/v1/pairing/revoke", "application/json", `{"tenantScope":"tenant_a","hostId":"host_alpha"}`)
	if status != http.StatusNotFound {
		t.Errorf("revoke unknown = %d, want 404", status)
	}
}

func TestOperatorTokenRequiredWhenConfigured(t *testing.T) {
	opToken := "test-" + "operator-token"
	server := controlplane.New(controlplane.Config{AllowPlaintextProfiles: true, OperatorToken: opToken})
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	post := func(header string) (int, string) {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/pairing/codes", strings.NewReader(`{"tenantScope":"tenant_a","hostId":"host_alpha"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if header != "" {
			req.Header.Set("X-Wazi-Operator-Token", header)
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if status, body := post(""); status != http.StatusUnauthorized {
		t.Errorf("mint without token = %d (%s), want 401", status, body)
	}
	if status, body := post("wrong-token"); status != http.StatusUnauthorized {
		t.Errorf("mint with wrong token = %d (%s), want 401", status, body)
	}
	if status, body := post(opToken); status != http.StatusCreated {
		t.Errorf("mint with token = %d (%s), want 201", status, body)
	}
}

// Renewal is two-phase: initiating records the replacement as
// pending (old cert keeps working, pending cert cannot submit
// yet), and presenting the pending cert activates it (old cert is
// retired). A revoked node cannot renew.
func TestCertificateRenewalRotatesIdentity(t *testing.T) {
	f := newMTLSFixture(t)
	certPEM, keyPEM := f.enrollViaHTTP(t, "tenant_a", "host_alpha")
	oldClient := f.certClient(t, certPEM, keyPEM, "localhost")

	newKeyPEM, csrPEM, err := pairing.GenerateNodeKeyAndCSR("host_alpha")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"csrPem": string(csrPEM)})
	resp, err := oldClient.Post(f.baseURL+"/v1/pairing/renew", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("renew = %d (%s), want 201", resp.StatusCode, body)
	}
	var renewed struct {
		CertificatePEM string `json:"certificatePem"`
		TenantScope    string `json:"tenantScope"`
		HostID         string `json:"hostId"`
	}
	if err := json.Unmarshal(body, &renewed); err != nil {
		t.Fatal(err)
	}
	if renewed.TenantScope != "tenant_a" || renewed.HostID != "host_alpha" {
		t.Errorf("renewed binding = %s/%s, want tenant_a/host_alpha", renewed.TenantScope, renewed.HostID)
	}
	renewedClient := f.certClient(t, []byte(renewed.CertificatePEM), newKeyPEM, "localhost")
	// Before activation: old cert still works (a client that fails
	// to persist the renewal is not stranded), pending cert does
	// not yet authorise general traffic.
	if status, body, err := postProfile(oldClient, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusAccepted {
		t.Errorf("old cert before activation = %d, %v (%s), want 202", status, err, body)
	}
	if status, _, err := postProfile(renewedClient, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusUnauthorized {
		t.Errorf("pending cert submit before activation = %d, %v; want 401", status, err)
	}
	// Activation presents the pending certificate itself.
	activateResp, err := renewedClient.Post(f.baseURL+"/v1/pairing/renew/activate", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	activateBody, _ := io.ReadAll(activateResp.Body)
	activateResp.Body.Close()
	if activateResp.StatusCode != http.StatusOK {
		t.Fatalf("activate = %d (%s), want 200", activateResp.StatusCode, activateBody)
	}
	// Retrying activation after a lost response also succeeds.
	retryResp, err := renewedClient.Post(f.baseURL+"/v1/pairing/renew/activate", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	retryResp.Body.Close()
	if retryResp.StatusCode != http.StatusOK {
		t.Errorf("activate retry = %d, want 200 (idempotent)", retryResp.StatusCode)
	}
	if status, body, err := postProfile(renewedClient, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status != http.StatusAccepted {
		t.Errorf("renewed cert submit = %d, %v (%s), want 202", status, err, body)
	}
	if status, _, err := postProfile(oldClient, f.baseURL, validProfile("tenant_a", "host_alpha")); err != nil || status == http.StatusAccepted {
		t.Errorf("old cert after activation = %d, %v; must be rejected", status, err)
	}
	// A revoked node cannot renew: re-enroll, revoke, attempt renew.
	cert2, key2 := f.enrollViaHTTP(t, "tenant_b", "host_beta")
	client2 := f.certClient(t, cert2, key2, "localhost")
	if err := f.registry.Revoke("tenant_b", "host_beta"); err != nil {
		t.Fatal(err)
	}
	_, csr2, err := pairing.GenerateNodeKeyAndCSR("host_beta")
	if err != nil {
		t.Fatal(err)
	}
	payload2, _ := json.Marshal(map[string]string{"csrPem": string(csr2)})
	resp2, err := client2.Post(f.baseURL+"/v1/pairing/renew", "application/json", bytes.NewReader(payload2))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("renew while revoked = %d, want 403", resp2.StatusCode)
	}
}

type slowReader struct {
	delay time.Duration
	data  []byte
	slept bool
	off   int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if !r.slept {
		r.slept = true
		time.Sleep(r.delay)
	}
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

// An unfinished body must not hold a handler open: the server's
// per-request read deadline aborts it promptly.
func TestBodyReadDeadlineBoundsSlowUpload(t *testing.T) {
	server := controlplane.New(controlplane.Config{AllowPlaintextProfiles: true, BodyReadTimeout: 150 * time.Millisecond})
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	valid, _ := json.Marshal(validProfile("tenant_a", "host_alpha"))
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/host-profiles", &slowReader{delay: 2 * time.Second, data: valid})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(valid))
	start := time.Now()
	resp, err := ts.Client().Do(req)
	elapsed := time.Since(start)
	if err != nil {
		// A transport-level abort is also acceptable proof the
		// deadline fired; a slow full upload is not.
		if elapsed > 1500*time.Millisecond {
			t.Fatalf("slow upload took %s before failing: %v", elapsed, err)
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("slow upload status = %d, want 400 after deadline", resp.StatusCode)
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("slow upload took %s, deadline did not bound it", elapsed)
	}
}

func TestDeploymentDisabled(t *testing.T) {
	_, ts := newPlainFixture(t)
	resp, err := ts.Client().Post(ts.URL+"/v1/deployments", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotImplemented || !strings.Contains(string(body), "deployment_disabled") {
		t.Errorf("deployment route = %d (%s), want 501 deployment_disabled", resp.StatusCode, body)
	}
}

func TestValidateListenAddr(t *testing.T) {
	cases := []struct {
		addr  string
		mtls  bool
		valid bool
	}{
		{"127.0.0.1:8080", false, true},
		{"localhost:8080", false, true},
		{"[::1]:8080", false, true},
		{":8080", false, false},        // every interface, no auth (P1)
		{"0.0.0.0:8080", false, false}, // every interface, no auth (P1)
		{"192.168.1.10:8080", false, false},
		{":8443", true, true}, // mTLS may bind beyond loopback
		{"0.0.0.0:8443", true, true},
	}
	for _, tc := range cases {
		err := controlplane.ValidateListenAddr(tc.addr, tc.mtls)
		if tc.valid && err != nil {
			t.Errorf("ValidateListenAddr(%q, mtls=%v) = %v, want nil", tc.addr, tc.mtls, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("ValidateListenAddr(%q, mtls=%v) = nil, want refusal", tc.addr, tc.mtls)
		}
	}
	if !controlplane.IsLoopbackAddr("127.0.0.1:8080") || controlplane.IsLoopbackAddr(":8080") {
		t.Error("IsLoopbackAddr misclassified loopback/wildcard")
	}
}
