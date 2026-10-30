package agentclient_test

import (
	"context"
	"crypto/tls"
	"net"
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

// One identity per CLI run: explicit IDs are honoured for inventory,
// an existing certificate's binding is authoritative, and any
// disagreement is an early error rather than a rejected submission.
func TestResolveHostIDSingleIdentity(t *testing.T) {
	ca, err := pairing.GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _, _, err := ca.IssueClientCert("tenant_a", "host_enrolled", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "node.crt")
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	id := agentclient.Identity{CertFile: certFile, KeyFile: filepath.Join(dir, "node.key"), CAFile: filepath.Join(dir, "ca.crt")}

	if got, err := agentclient.ResolveHostID("", "tenant_a", id); err != nil || got != "host_enrolled" {
		t.Errorf("cert-derived identity = %q, %v; want host_enrolled", got, err)
	}
	if got, err := agentclient.ResolveHostID("host_enrolled", "tenant_a", id); err != nil || got != "host_enrolled" {
		t.Errorf("agreeing explicit identity = %q, %v", got, err)
	}
	if _, err := agentclient.ResolveHostID("host_other", "tenant_a", id); err == nil {
		t.Error("explicit ID disagreeing with the certificate was accepted")
	}
	if _, err := agentclient.ResolveHostID("", "tenant_b", id); err == nil {
		t.Error("tenant disagreeing with the certificate was accepted")
	}
	noCert := agentclient.Identity{CertFile: filepath.Join(dir, "absent.crt"), KeyFile: id.KeyFile, CAFile: id.CAFile}
	if got, err := agentclient.ResolveHostID("host_custom42", "tenant_a", noCert); err != nil || got != "host_custom42" {
		t.Errorf("custom ID without cert = %q, %v; want host_custom42", got, err)
	}
}

// System-root trust is consistent between bootstrap and later
// loads: an absent CA file means system roots in both, not a
// bootstrap that succeeds and a LoadTLS that then refuses.
func TestLoadTLSAbsentCAFileUsesSystemRoots(t *testing.T) {
	ca, err := pairing.GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, _, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	id := agentclient.Identity{
		CertFile: filepath.Join(dir, "node.crt"),
		KeyFile:  filepath.Join(dir, "node.key"),
		CAFile:   filepath.Join(dir, "absent-ca.crt"),
	}
	if err := os.WriteFile(id.CertFile, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(id.KeyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := agentclient.LoadTLS(id, "localhost")
	if err != nil {
		t.Fatalf("LoadTLS with absent CA file: %v", err)
	}
	if cfg.RootCAs != nil {
		t.Error("absent CA file should select system roots (nil RootCAs)")
	}
}

// Server trust and the client issuing CA are separate roles: with
// two different CAs, enrollment must leave the operator's server
// anchor byte-identical, persist the issuer separately, and still
// submit successfully afterwards. A custom node ID is used
// throughout the whole journey.
func TestEnrollmentSeparatesServerAndIssuingTrust(t *testing.T) {
	serverCA, err := pairing.GenerateCA("server-ca")
	if err != nil {
		t.Fatal(err)
	}
	pairingCA, err := pairing.GenerateCA("pairing-ca")
	if err != nil {
		t.Fatal(err)
	}
	serverCertPEM, serverKeyPEM, err := serverCA.IssueServerCert([]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	registry := pairing.NewRegistry(nil)
	server := controlplane.New(controlplane.Config{Registry: registry, CA: pairingCA})
	ts := httptest.NewUnstartedServer(server.Handler())
	ts.TLS = pairing.ServerTLSConfig(pairingCA, serverCert)
	ts.StartTLS()
	defer ts.Close()

	dir := t.TempDir()
	serverTrustFile := filepath.Join(dir, "server-ca.crt")
	if err := os.WriteFile(serverTrustFile, serverCA.CertPEM(), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := agentclient.Identity{
		CertFile:     filepath.Join(dir, "node.crt"),
		KeyFile:      filepath.Join(dir, "node.key"),
		CAFile:       serverTrustFile,
		IssuerCAFile: filepath.Join(dir, "issuer.crt"),
	}
	code, err := registry.CreatePairingCode("tenant_a", "host_custom42", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	trust, bootstrapPEM, err := agentclient.LoadBootstrap(serverTrustFile, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := agentclient.Enroll(ctx, ts.URL, trust, bootstrapPEM, code, "tenant_a", "host_custom42", identity)
	if err != nil {
		t.Fatalf("enroll with separate CAs: %v", err)
	}
	if result.HostID != "host_custom42" {
		t.Errorf("enrolled host = %q, want host_custom42", result.HostID)
	}
	anchorAfter, err := os.ReadFile(serverTrustFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(anchorAfter) != string(serverCA.CertPEM()) {
		t.Error("enrollment overwrote the operator server trust anchor")
	}
	issuerAfter, err := os.ReadFile(identity.IssuerCAFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(issuerAfter) != string(pairingCA.CertPEM()) {
		t.Error("issuing CA was not persisted separately from server trust")
	}
	// The resolved identity now comes from the certificate itself.
	if got, err := agentclient.ResolveHostID("", "tenant_a", identity); err != nil || got != "host_custom42" {
		t.Errorf("post-enroll resolved identity = %q, %v", got, err)
	}
	tlsConfig, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, validProfile("tenant_a", "host_custom42")); err != nil {
		t.Fatalf("submit with separate CAs: %v", err)
	}

	// Renewal rotates key and certificate; the old pair stops
	// working and server trust is still untouched.
	oldCert, err := os.ReadFile(identity.CertFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentclient.Renew(ctx, ts.URL, tlsConfig, "tenant_a", "host_custom42", identity); err != nil {
		t.Fatalf("renew: %v", err)
	}
	newCert, err := os.ReadFile(identity.CertFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(oldCert) == string(newCert) {
		t.Error("renewal did not replace the certificate")
	}
	renewedTLS, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, renewedTLS, validProfile("tenant_a", "host_custom42")); err != nil {
		t.Fatalf("submit after renewal: %v", err)
	}
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, validProfile("tenant_a", "host_custom42")); err == nil {
		t.Error("pre-renewal certificate still submits after rotation")
	}
	anchorFinal, _ := os.ReadFile(serverTrustFile)
	if string(anchorFinal) != string(serverCA.CertPEM()) {
		t.Error("renewal overwrote the server trust anchor")
	}
	if !strings.Contains(string(issuerAfter), "pairing-ca") && string(issuerAfter) != string(pairingCA.CertPEM()) {
		t.Error("issuer evidence changed unexpectedly")
	}
	// Windows can transiently refuse the final directory unlink
	// after synced identity writes; remove explicitly with retries
	// so the TempDir cleanup does not flake on an empty directory.
	for attempt := 0; attempt < 50; attempt++ {
		if err := os.RemoveAll(dir); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
