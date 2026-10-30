package agentclient_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/controlplane"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
)

// Backup capture happens before any destination moves: if
// capturing a later file fails (here the certificate destination
// is a directory), the earlier key file must still be at its
// configured path with its original contents. The previous
// move-aside implementation lost exactly that file.
func TestEnrollBackupFailureLeavesKeyUntouched(t *testing.T) {
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
	server := controlplane.New(controlplane.Config{Registry: registry, CA: ca})
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = server.TLSConfig(serverCert)
	ts.StartTLS()
	defer ts.Close()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := agentclient.Identity{
		CertFile:     filepath.Join(dir, "node.crt"),
		KeyFile:      filepath.Join(dir, "node.key"),
		CAFile:       caFile,
		IssuerCAFile: filepath.Join(dir, "issuer.crt"),
	}
	originalKey := []byte("previous-node-key-contents")
	if err := os.WriteFile(identity.KeyFile, originalKey, 0o600); err != nil {
		t.Fatal(err)
	}
	// Certificate destination is a non-empty directory: capturing
	// its backup must fail before anything is moved.
	if err := os.Mkdir(identity.CertFile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(identity.CertFile, "occupant"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, err := registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := agentclient.EnrollmentTLS(caFile, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentclient.Enroll(context.Background(), ts.URL, trust, ca.CertPEM(), code, "tenant_a", "host_alpha", identity); err == nil {
		t.Fatal("enroll with a directory as certificate destination succeeded")
	}
	got, err := os.ReadFile(identity.KeyFile)
	if err != nil {
		t.Fatalf("node key missing after backup failure: %v", err)
	}
	if string(got) != string(originalKey) {
		t.Error("node key contents changed after backup failure")
	}
}

// The stranded-renewal regression: server renewal begins, local
// persistence then fails. The old identity files must be
// unchanged *and still authorised* (the old certificate was never
// retired), so fixing the local problem and retrying succeeds.
func TestRenewalPersistenceFailureDoesNotStrandNode(t *testing.T) {
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
	server := controlplane.New(controlplane.Config{Registry: registry, CA: ca})
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = server.TLSConfig(serverCert)
	ts.StartTLS()
	defer ts.Close()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := agentclient.Identity{
		CertFile:     filepath.Join(dir, "node.crt"),
		KeyFile:      filepath.Join(dir, "node.key"),
		CAFile:       caFile,
		IssuerCAFile: filepath.Join(dir, "issuer.crt"),
	}
	code, err := registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := agentclient.EnrollmentTLS(caFile, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := agentclient.Enroll(ctx, ts.URL, trust, ca.CertPEM(), code, "tenant_a", "host_alpha", identity); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	oldKey, _ := os.ReadFile(identity.KeyFile)
	oldCert, _ := os.ReadFile(identity.CertFile)
	tlsConfig, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}

	// Force local persistence to fail after the server has begun
	// renewal: the issuer destination is a non-empty directory, so
	// the commit rename must fail and roll back.
	issuerDir := identity.IssuerCAFile
	if err := os.Remove(issuerDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(issuerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuerDir, "occupant"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := agentclient.Renew(ctx, ts.URL, tlsConfig, "tenant_a", "host_alpha", identity); err == nil {
		t.Fatal("renew with unpersistable issuer destination succeeded")
	}
	// Old files unchanged...
	if nowKey, _ := os.ReadFile(identity.KeyFile); string(nowKey) != string(oldKey) {
		t.Error("failed renewal changed the node key file")
	}
	if nowCert, _ := os.ReadFile(identity.CertFile); string(nowCert) != string(oldCert) {
		t.Error("failed renewal changed the node certificate file")
	}
	// ...and still authorised (not stranded with a 401).
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, validProfile("tenant_a", "host_alpha")); err != nil {
		t.Errorf("old identity after failed renewal persistence: %v (node stranded)", err)
	}
	// Repair and retry: renewal now completes and activates.
	if err := os.RemoveAll(issuerDir); err != nil {
		t.Fatal(err)
	}
	if _, err := agentclient.Renew(ctx, ts.URL, tlsConfig, "tenant_a", "host_alpha", identity); err != nil {
		t.Fatalf("retry renew after repair: %v", err)
	}
	if identity.HasPendingRenewal() {
		t.Error("pending marker remains after successful renewal")
	}
	renewedTLS, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, renewedTLS, validProfile("tenant_a", "host_alpha")); err != nil {
		t.Errorf("submit after repaired renewal: %v", err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, validProfile("tenant_a", "host_alpha")); err == nil {
		t.Error("old certificate still submits after completed renewal")
	}
	_ = strings.Contains // strings retained for future assertions
}

// A renewal whose activation response was lost converges by
// retry: the persisted pending identity plus its marker lets
// ActivatePending finish the handoff without re-renewing.
func TestActivatePendingCompletesLostActivation(t *testing.T) {
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
	server := controlplane.New(controlplane.Config{Registry: registry, CA: ca})
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = server.TLSConfig(serverCert)
	ts.StartTLS()
	defer ts.Close()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := agentclient.Identity{
		CertFile:     filepath.Join(dir, "node.crt"),
		KeyFile:      filepath.Join(dir, "node.key"),
		CAFile:       caFile,
		IssuerCAFile: filepath.Join(dir, "issuer.crt"),
	}
	code, err := registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := agentclient.EnrollmentTLS(caFile, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := agentclient.Enroll(ctx, ts.URL, trust, ca.CertPEM(), code, "tenant_a", "host_alpha", identity); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	tlsConfig, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}

	// Stage a lost activation by hand: begin renewal over the old
	// identity, persist the new pair locally, write the marker,
	// then stop before activation.
	newKey, csr, err := pairing.GenerateNodeKeyAndCSR("host_alpha")
	if err != nil {
		t.Fatal(err)
	}
	renewClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
	renewBody, _ := json.Marshal(map[string]string{"csrPem": string(csr)})
	renewResp, err := renewClient.Post(ts.URL+"/v1/pairing/renew", "application/json", bytes.NewReader(renewBody))
	if err != nil {
		t.Fatal(err)
	}
	renewData, _ := io.ReadAll(renewResp.Body)
	renewResp.Body.Close()
	if renewResp.StatusCode != http.StatusCreated {
		t.Fatalf("begin renewal = %d (%s)", renewResp.StatusCode, renewData)
	}
	var issued struct {
		CertificatePEM string `json:"certificatePem"`
	}
	if err := json.Unmarshal(renewData, &issued); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity.KeyFile, newKey, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity.CertFile, []byte(issued.CertificatePEM), 0o644); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(dir, "renewal.pending")
	if err := os.WriteFile(markerPath, []byte("tenant=tenant_a host=host_alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	renewedTLS, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, renewedTLS, validProfile("tenant_a", "host_alpha")); err == nil {
		t.Fatal("pending identity submitted before activation")
	}
	activated, err := agentclient.ActivatePending(ctx, ts.URL, tlsConfig, identity)
	if err != nil || !activated {
		t.Fatalf("ActivatePending = %v, %v; want true, nil", activated, err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("pending marker remains after activation")
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, renewedTLS, validProfile("tenant_a", "host_alpha")); err != nil {
		t.Errorf("submit after activation retry: %v", err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, validProfile("tenant_a", "host_alpha")); err == nil {
		t.Error("old certificate still submits after activation")
	}
	// And with nothing pending, ActivatePending is a clean no-op.
	activated, err = agentclient.ActivatePending(ctx, ts.URL, tlsConfig, identity)
	if err != nil || activated {
		t.Errorf("ActivatePending steady state = %v, %v; want false, nil", activated, err)
	}
}
