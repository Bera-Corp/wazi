package pairing

import (
	"crypto/x509"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func enroll(t *testing.T, ca *CA, reg *Registry, tenant, hostID string) Node {
	t.Helper()
	code, err := reg.CreatePairingCode(tenant, hostID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert(tenant, hostID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// IssueClientCert mints a fresh key; enrollment registers the
	// cert it returns. (The CSR path is covered end-to-end in the
	// controlplane tests.)
	node, err := reg.Enroll(code, tenant, hostID, cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter)
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func TestPairingCodeSingleUseAndBinding(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(nil)
	code, err := reg.CreatePairingCode("tenant_a", "host_one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Wrong binding must not consume or succeed.
	if _, err := reg.Enroll(code, "tenant_b", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); !errors.Is(err, ErrBindingMismatch) {
		t.Errorf("cross-binding enroll err = %v, want ErrBindingMismatch", err)
	}
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
		t.Fatalf("first enroll: %v", err)
	}
	// Replay of the same code must never mint a second identity.
	_, _, cert2, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert2.SerialNumber.String(), Fingerprint(cert2), cert2.NotAfter); !errors.Is(err, ErrCodeReplay) {
		t.Errorf("replay err = %v, want ErrCodeReplay", err)
	}
	// The replayed cert was never registered: Authenticate rejects it.
	if _, err := reg.Authenticate(cert2); !errors.Is(err, ErrUnknownNode) {
		t.Errorf("replayed cert authenticate err = %v, want ErrUnknownNode", err)
	}
	if _, err := reg.Authenticate(cert); err != nil {
		t.Errorf("enrolled cert authenticate: %v", err)
	}
}

func TestPairingCodeExpiryAndUnknown(t *testing.T) {
	now := time.Now()
	reg := NewRegistry(func() time.Time { return now })
	code, err := reg.CreatePairingCode("tenant_a", "host_one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll("no-such-code", "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); !errors.Is(err, ErrCodeNotFound) {
		t.Errorf("unknown code err = %v, want ErrCodeNotFound", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); !errors.Is(err, ErrCodeExpired) {
		t.Errorf("expired code err = %v, want ErrCodeExpired", err)
	}
}

func TestRevokedNodeRejected(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(nil)
	code, err := reg.CreatePairingCode("tenant_a", "host_rev", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_rev", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll(code, "tenant_a", "host_rev", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Authenticate(cert); err != nil {
		t.Fatalf("pre-revoke authenticate: %v", err)
	}
	if err := reg.Revoke("tenant_a", "host_rev"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Authenticate(cert); !errors.Is(err, ErrRevoked) {
		t.Errorf("revoked authenticate err = %v, want ErrRevoked", err)
	}
	if err := reg.Revoke("tenant_a", "host_missing"); !errors.Is(err, ErrUnknownNode) {
		t.Errorf("revoke unknown err = %v, want ErrUnknownNode", err)
	}
}

// Revocation must kill every outstanding code for the node, not
// just the enrolled certificate: a spare code minted earlier must
// not re-enroll the revoked node. A code minted after revocation
// (fresh operator approval) is the only way back.
func TestRevocationInvalidatesOutstandingCodes(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(nil)
	codeUsed, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	codeSpare, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	enrollWith := func(code string) (*x509.Certificate, error) {
		_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reg.Enroll(code, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
			return cert, err
		}
		return cert, nil
	}
	firstCert, err := enrollWith(codeUsed)
	if err != nil {
		t.Fatalf("initial enroll: %v", err)
	}
	if err := reg.Revoke("tenant_a", "host_one"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Authenticate(firstCert); !errors.Is(err, ErrRevoked) {
		t.Errorf("revoked cert authenticate = %v, want ErrRevoked", err)
	}
	if _, err := enrollWith(codeSpare); !errors.Is(err, ErrCodeInvalidated) {
		t.Errorf("spare pre-revocation code enroll = %v, want ErrCodeInvalidated", err)
	}
	codeFresh, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	freshCert, err := enrollWith(codeFresh)
	if err != nil {
		t.Errorf("fresh post-revocation code enroll: %v", err)
	}
	if _, err := reg.Authenticate(freshCert); err != nil {
		t.Errorf("re-enrolled cert authenticate: %v", err)
	}
	if _, err := reg.Authenticate(firstCert); err == nil {
		t.Error("original revoked certificate regained access after re-enrollment")
	}
}

func TestAuthorizeCommitNeverRunsForRevokedNode(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(nil)
	code, err := reg.CreatePairingCode("tenant_a", "host_one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
		t.Fatal(err)
	}
	ran := false
	if err := reg.AuthorizeCommit(cert, func(node Node) error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("commit for live node: ran=%v err=%v", ran, err)
	}
	if err := reg.Revoke("tenant_a", "host_one"); err != nil {
		t.Fatal(err)
	}
	ran = false
	if err := reg.AuthorizeCommit(cert, func(node Node) error { ran = true; return nil }); !errors.Is(err, ErrRevoked) || ran {
		t.Errorf("commit for revoked node: ran=%v err=%v, want no run + ErrRevoked", ran, err)
	}
}

// Converse ordering to the in-flight body test: a commit that
// already holds the authorisation lock must delay revocation
// until it finishes; after it releases, revocation lands and
// every later commit is rejected.
func TestActiveCommitDelaysRevocation(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(nil)
	code, err := reg.CreatePairingCode("tenant_a", "host_one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
		t.Fatal(err)
	}
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- reg.AuthorizeCommit(cert, func(node Node) error {
			close(commitStarted)
			<-releaseCommit
			return nil
		})
	}()
	<-commitStarted
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- reg.Revoke("tenant_a", "host_one") }()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation completed while a commit held the lock (err=%v)", err)
	case <-time.After(250 * time.Millisecond):
		// Still blocked, as required.
	}
	close(releaseCommit)
	if err := <-commitDone; err != nil {
		t.Fatalf("held commit: %v", err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("revocation after commit release: %v", err)
	}
	ran := false
	if err := reg.AuthorizeCommit(cert, func(node Node) error { ran = true; return nil }); !errors.Is(err, ErrRevoked) || ran {
		t.Errorf("post-revocation commit ran=%v err=%v, want no run + ErrRevoked", ran, err)
	}
}

// Durable storage: nodes, revocation, epochs and consumed codes
// survive a restart; nothing revoked or consumed is resurrected.
func TestRegistryPersistenceAcrossRestart(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/registry.json"
	reg, err := NewRegistryWithStore(nil, path)
	if err != nil {
		t.Fatal(err)
	}
	codeUsed, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	codeSpare, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll(codeUsed, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
		t.Fatal(err)
	}
	if err := reg.Revoke("tenant_a", "host_one"); err != nil {
		t.Fatal(err)
	}
	// A second writer over the same store is refused while the
	// first registry holds it.
	if _, err := NewRegistryWithStore(nil, path); err == nil {
		t.Error("second registry writer over the same store was admitted")
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewRegistryWithStore(nil, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reloaded.Close() }()
	if _, err := reloaded.Authenticate(cert); !errors.Is(err, ErrRevoked) {
		t.Errorf("after restart revoked cert = %v, want ErrRevoked", err)
	}
	_, _, cert2, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.Enroll(codeUsed, "tenant_a", "host_one", cert2.SerialNumber.String(), Fingerprint(cert2), cert2.NotAfter); !errors.Is(err, ErrCodeReplay) {
		t.Errorf("after restart consumed code = %v, want ErrCodeReplay", err)
	}
	if _, err := reloaded.Enroll(codeSpare, "tenant_a", "host_one", cert2.SerialNumber.String(), Fingerprint(cert2), cert2.NotAfter); !errors.Is(err, ErrCodeInvalidated) {
		t.Errorf("after restart spare code = %v, want ErrCodeInvalidated", err)
	}
	// The store must not contain a usable raw pairing code.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), codeSpare) || strings.Contains(string(raw), codeUsed) {
		t.Error("registry store contains a raw pairing code")
	}
	// Corrupt stores refuse to start rather than resurrecting state.
	if err := reloaded.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistryWithStore(nil, path); err == nil {
		t.Error("corrupt registry store loaded without error")
	}
}

// Failed saves are transactional: forcing the snapshot write to
// fail must leave enrollment codes unconsumed, certificates
// authorised, and revocations unapplied, so repairing storage and
// retrying behaves exactly as if the attempt never happened.
func TestFailedRegistrySaveRollsBackLiveState(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := dir + "/registry.json"
	reg, err := NewRegistryWithStore(nil, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()
	breakStore := func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	repairStore := func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	issue := func() *x509.Certificate {
		_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return cert
	}

	// Enrollment with a broken store: error, code not consumed.
	code, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cert1 := issue()
	breakStore()
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert1.SerialNumber.String(), Fingerprint(cert1), cert1.NotAfter); err == nil {
		t.Fatal("enroll with broken store succeeded")
	}
	repairStore()
	if _, err := reg.Enroll(code, "tenant_a", "host_one", cert1.SerialNumber.String(), Fingerprint(cert1), cert1.NotAfter); err != nil {
		t.Errorf("retry enroll after repair: %v (code was consumed by the failed save)", err)
	}

	// Rotation-begin with a broken store: old cert still authorised.
	cert2 := issue()
	breakStore()
	if _, err := reg.BeginRotation(cert1, cert2.SerialNumber.String(), Fingerprint(cert2), cert2.NotAfter); err == nil {
		t.Fatal("begin rotation with broken store succeeded")
	}
	if _, err := reg.Authenticate(cert1); err != nil {
		t.Errorf("old cert after failed rotation save: %v (was retired by a failed save)", err)
	}
	repairStore()
	if _, err := reg.BeginRotation(cert1, cert2.SerialNumber.String(), Fingerprint(cert2), cert2.NotAfter); err != nil {
		t.Errorf("retry begin rotation after repair: %v", err)
	}
	if _, err := reg.ActivateRotation(cert2); err != nil {
		t.Errorf("activate after repair: %v", err)
	}

	// Revocation with a broken store: node stays authorised.
	breakStore()
	if err := reg.Revoke("tenant_a", "host_one"); err == nil {
		t.Fatal("revoke with broken store succeeded")
	}
	if _, err := reg.Authenticate(cert2); err != nil {
		t.Errorf("cert after failed revoke save: %v (revocation applied despite failed save)", err)
	}
	repairStore()
	if err := reg.Revoke("tenant_a", "host_one"); err != nil {
		t.Errorf("retry revoke after repair: %v", err)
	}
	if _, err := reg.Authenticate(cert2); !errors.Is(err, ErrRevoked) {
		t.Errorf("cert after repaired revoke = %v, want ErrRevoked", err)
	}
}

// Expired registrations can be replaced by a fresh operator code
// (the re-pair path); live registrations cannot.
func TestExpiredRegistrationCanRePairWithFreshCode(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	reg := NewRegistry(func() time.Time { return now })
	mintAndEnroll := func() *x509.Certificate {
		code, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reg.Enroll(code, "tenant_a", "host_one", cert.SerialNumber.String(), Fingerprint(cert), cert.NotAfter); err != nil {
			t.Fatal(err)
		}
		return cert
	}
	first := mintAndEnroll()
	// Still live: a second enrollment is refused.
	code, err := reg.CreatePairingCode("tenant_a", "host_one", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, _, other, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Enroll(code, "tenant_a", "host_one", other.SerialNumber.String(), Fingerprint(other), other.NotAfter); !errors.Is(err, ErrNodeExists) {
		t.Errorf("live re-enroll = %v, want ErrNodeExists", err)
	}
	// Advance beyond the certificate expiry recorded at
	// enrollment: a fresh code now replaces the registration.
	now = now.Add(2 * time.Hour)
	second := mintAndEnroll()
	_ = second
	if _, err := reg.Authenticate(first); err == nil {
		t.Error("expired replaced certificate still authenticates")
	}
}

func TestCertBindingExtraction(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	_, _, cert, err := ca.IssueClientCert("tenant_a", "host_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tenant, hostID, ok := CertBinding(cert)
	if !ok || tenant != "tenant_a" || hostID != "host_one" {
		t.Errorf("binding = %q/%q ok=%v, want tenant_a/host_one", tenant, hostID, ok)
	}
	// A CA cert carries no node binding.
	if _, _, ok := CertBinding(ca.Cert); ok {
		t.Error("CA certificate unexpectedly carries a node binding")
	}
}

func TestCSRFlowBindsServerSideIdentity(t *testing.T) {
	ca, err := GenerateCA("test-ca")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, csrPEM, err := GenerateNodeKeyAndCSR("host_claimed")
	if err != nil {
		t.Fatal(err)
	}
	if len(keyPEM) == 0 {
		t.Fatal("node key not generated")
	}
	// The issued binding comes from the enrollment arguments, and a
	// tampered CSR is rejected.
	certPEM, cert, err := ca.IssueClientCertFromCSR(csrPEM, "tenant_a", "host_real", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(certPEM) == 0 {
		t.Fatal("empty cert PEM")
	}
	tenant, hostID, _ := CertBinding(cert)
	if tenant != "tenant_a" || hostID != "host_real" {
		t.Errorf("CSR-issued binding = %s/%s, want tenant_a/host_real (server-side, not CSR-asserted)", tenant, hostID)
	}
	if _, _, err := ca.IssueClientCertFromCSR([]byte("not a csr"), "tenant_a", "host_real", time.Hour); err == nil {
		t.Error("garbage CSR accepted")
	}
}
