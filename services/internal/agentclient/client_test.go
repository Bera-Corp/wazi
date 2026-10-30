package agentclient_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/controlplane"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
)

// Pairing codes are bearer credentials: they must never be sent to
// a plaintext URL, and a redirect must never re-deliver them (or an
// authenticated profile) to another origin.
func TestEnrollAndSubmitNeverUsePlaintextOrRedirects(t *testing.T) {
	ctx := context.Background()
	var plainHits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plainHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()

	dir := t.TempDir()
	identity := agentclient.Identity{
		CertFile: filepath.Join(dir, "node.crt"),
		KeyFile:  filepath.Join(dir, "node.key"),
		CAFile:   filepath.Join(dir, "ca.crt"),
	}
	if _, err := agentclient.Enroll(ctx, plain.URL, nil, nil, "secret-pairing-code", "tenant_a", "host_alpha", identity); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("plaintext enroll err = %v, want https refusal before any request", err)
	}
	if err := agentclient.SubmitProfile(ctx, plain.URL, &tls.Config{}, validProfile("tenant_a", "host_alpha")); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("plaintext submit err = %v, want https refusal before any request", err)
	}
	if got := plainHits.Load(); got != 0 {
		t.Fatalf("plaintext endpoint received %d credential-bearing requests, want 0", got)
	}

	// HTTPS endpoint that redirects everything to the plaintext
	// capture server: the client must refuse to follow, so the
	// capture server still sees nothing.
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
	redirector := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+r.URL.Path, http.StatusFound)
	}))
	redirector.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS12}
	redirector.StartTLS()
	defer redirector.Close()

	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o644); err != nil {
		t.Fatal(err)
	}
	trust, err := agentclient.EnrollmentTLS(caFile, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentclient.Enroll(ctx, redirector.URL, trust, ca.CertPEM(), "secret-pairing-code", "tenant_a", "host_alpha", identity); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("redirecting enroll err = %v, want redirect refusal", err)
	}
	if err := agentclient.SubmitProfile(ctx, redirector.URL, trust, validProfile("tenant_a", "host_alpha")); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("redirecting submit err = %v, want redirect refusal", err)
	}
	if got := plainHits.Load(); got != 0 {
		t.Fatalf("redirect delivered %d requests to the plaintext origin, want 0", got)
	}
}

// End-to-end pairing journey: operator mints a one-time code, the
// agent generates its key locally, enrolls with only a CSR, then
// submits its own profile over mTLS. A second submission under a
// different tenant (cross-tenant forgery with the same key) fails.
func TestEnrollAndSubmitEndToEnd(t *testing.T) {
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

	code, err := registry.CreatePairingCode("tenant_a", "host_alpha", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := agentclient.Identity{
		CertFile: filepath.Join(dir, "node.crt"),
		KeyFile:  filepath.Join(dir, "node.key"),
		CAFile:   caFile,
	}
	trust, err := agentclient.EnrollmentTLS(caFile, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := agentclient.Enroll(ctx, ts.URL, trust, ca.CertPEM(), code, "tenant_a", "host_alpha", identity)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if result.TenantScope != "tenant_a" || result.HostID != "host_alpha" {
		t.Errorf("enroll binding = %s/%s", result.TenantScope, result.HostID)
	}
	keyInfo, err := os.Stat(identity.KeyFile)
	if err != nil || keyInfo.Size() == 0 {
		t.Fatalf("node key not persisted: %v", err)
	}

	tlsConfig, err := agentclient.LoadTLS(identity, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	profile := validProfile("tenant_a", "host_alpha")
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, profile); err != nil {
		t.Fatalf("submit own profile: %v", err)
	}
	forged := validProfile("tenant_b", "host_alpha")
	if err := agentclient.SubmitProfile(ctx, ts.URL, tlsConfig, forged); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("cross-tenant submission should fail with 403, got: %v", err)
	}
	// Pairing codes are single-use even through the real client path.
	if _, err := agentclient.Enroll(ctx, ts.URL, trust, ca.CertPEM(), code, "tenant_a", "host_alpha", identity); err == nil {
		t.Error("replayed pairing code accepted through agent client")
	}
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
	p.OccupiedPorts = []int{22}
	p.Docker.EngineVersion = "29.1.3"
	p.Docker.ComposeVersion = "2.40.3"
	p.Filesystem.Type = "ext4"
	p.Filesystem.DockerRootWritable = true
	p.Network.Mode = "byo-vps"
	p.Provenance.SourceType = "host-agent-probe"
	p.Provenance.ExtractionMethod = "host-probe"
	p.Redaction.Applied = true
	return p
}
