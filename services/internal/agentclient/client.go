// Package agentclient is the host agent's control-plane client:
// one-time pairing enrollment (node key generated locally, only a
// CSR leaves the node) and mTLS host-profile submission. The
// certificate it receives is bound by the control plane to exactly
// one (tenant, node); this client never asserts a different one.
package agentclient

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/probe"
)

func pemDecode(data []byte) (*pem.Block, []byte) { return pem.Decode(data) }

var probeCurrentHostID = probe.CurrentHostID

// secureBaseURL parses a control-plane base URL and enforces the
// transport contract for credential-bearing requests: HTTPS only,
// with a host, and no embedded credentials or fragment. A pairing
// code is a bearer credential; accepting http:// would deliver it
// in plaintext even when TLS trust was configured for later calls.
func secureBaseURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("control plane URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("control plane URL must use https (got %q): pairing codes and profiles never travel over plaintext", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("control plane URL must include a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("control plane URL must not embed credentials")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("control plane URL must not include a fragment")
	}
	return strings.TrimRight(baseURL, "/"), nil
}

// newSecureClient builds an HTTP client that never follows
// redirects. A redirect from an HTTPS control plane to a plaintext
// (or different) origin must not re-deliver the pairing code or the
// client-certificate-authenticated profile body, so every redirect
// is a hard error rather than something to restrict by origin.
func newSecureClient(tlsConfig *tls.Config) *http.Client {
	transport := &http.Transport{}
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("control-plane redirects are forbidden")
		},
	}
}

func requirePairedTLS(tlsConfig *tls.Config, purpose string) error {
	if tlsConfig == nil || tlsConfig.InsecureSkipVerify {
		return fmt.Errorf("%s requires verified TLS", purpose)
	}
	if len(tlsConfig.Certificates) == 0 && tlsConfig.GetClientCertificate == nil {
		return fmt.Errorf("%s requires a paired client certificate", purpose)
	}
	if len(tlsConfig.Certificates) > 0 {
		certificate := tlsConfig.Certificates[0]
		if len(certificate.Certificate) == 0 || certificate.PrivateKey == nil {
			return fmt.Errorf("%s has an incomplete paired client certificate", purpose)
		}
	}
	return nil
}

// PendingFile names the renewal-pending marker. Empty selects
// <cert dir>/renewal.pending.
func (id Identity) pendingFile() string {
	if id.PendingFile != "" {
		return id.PendingFile
	}
	return filepath.Join(filepath.Dir(id.CertFile), "renewal.pending")
}

// HasPendingRenewal reports whether a persisted identity is
// waiting for renewal activation.
func (id Identity) HasPendingRenewal() bool {
	_, err := os.Stat(id.pendingFile())
	return err == nil
}

// Identity is the enrolled node credential material on disk.
// CAFile is the *server* trust anchor (operator-supplied, used to
// verify the control plane's server certificate). It is never
// overwritten by enrollment. IssuerCAFile is where the *client*
// issuing CA from an enrollment/renewal response is persisted
// after validation; it is evidence about who issued the node
// certificate, and is deliberately not used as server trust, so a
// response can never substitute the anchor the server was
// verified against. Keeping the two separate also allows the
// server CA and the pairing CA to be different authorities.
type Identity struct {
	CertFile     string
	KeyFile      string
	CAFile       string // server trust anchor; never overwritten by responses
	IssuerCAFile string // issued-cert authority, persisted after validation
	PendingFile  string // renewal-pending marker; empty = beside the cert
}

func (id Identity) issuerFile() string {
	if id.IssuerCAFile != "" {
		return id.IssuerCAFile
	}
	return filepath.Join(filepath.Dir(id.CertFile), "issuer.crt")
}

// CertificateBinding reads a node certificate file and returns its
// server-issued (tenant, node) binding.
func CertificateBinding(certFile string) (tenant, hostID string, err error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return "", "", err
	}
	block, _ := pemDecode(certPEM)
	if block == nil {
		return "", "", fmt.Errorf("node certificate: no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("node certificate: %w", err)
	}
	tenant, hostID, ok := pairing.CertBinding(cert)
	if !ok {
		return "", "", fmt.Errorf("node certificate carries no wazi tenant/node binding")
	}
	return tenant, hostID, nil
}

// ResolveHostID resolves the single node identity for a CLI run.
// Precedence and validation: an existing certificate's binding is
// authoritative (it is what the server will accept); an explicit
// host ID must agree with it, and the tenant flag must agree with
// its tenant. Without a certificate, the explicit ID wins, else
// the hostname-derived ID. Enrollment, inventory and submission
// all use this one result, so a custom -host-id can no longer
// enroll one identity while inventory reports another.
func ResolveHostID(explicitHostID, tenant string, id Identity) (string, error) {
	certTenant, certHost, err := CertificateBinding(id.CertFile)
	switch {
	case err == nil:
		if tenant != "" && certTenant != tenant {
			return "", fmt.Errorf("node certificate is bound to tenant %q, not %q", certTenant, tenant)
		}
		if explicitHostID != "" && explicitHostID != certHost {
			return "", fmt.Errorf("-host-id %q disagrees with the enrolled certificate identity %q", explicitHostID, certHost)
		}
		return certHost, nil
	case os.IsNotExist(err):
		// No enrolled identity yet.
	default:
		return "", err
	}
	if explicitHostID != "" {
		return explicitHostID, nil
	}
	return probeCurrentHostID()
}

// EnrollResult mirrors the control plane enrollment response.
type EnrollResult struct {
	CertificatePEM string `json:"certificatePem"`
	CAPEM          string `json:"caPem"`
	TenantScope    string `json:"tenantScope"`
	HostID         string `json:"hostId"`
	ExpiresAt      string `json:"expiresAt"`
}

// Enroll performs pairing: generate a node key, send only its CSR
// with the one-time code, and persist the issued certificate. The
// private key is written 0600 and never transmitted.
//
// Trust discipline: trust verifies the *server*. bootstrapCAPEM is
// the operator-supplied server trust anchor behind it (nil when
// system roots are used). The response's caPem is the *client
// issuing* CA - a different role. When bootstrapCAPEM is supplied,
// a response CA is only accepted as issuer evidence, never as a
// replacement anchor; server trust stays exactly what the operator
// configured. Before anything is persisted, the issued certificate
// is validated: it must chain to the response CA, carry exactly
// the requested (tenant, node) binding, match the locally
// generated key, and be currently valid. Persistence is one
// staged transaction (see persistIdentity).
func Enroll(ctx context.Context, baseURL string, trust *tls.Config, bootstrapCAPEM []byte, pairingCode, tenant, hostID string, id Identity) (EnrollResult, error) {
	var result EnrollResult
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return result, err
	}
	keyPEM, csrPEM, err := pairing.GenerateNodeKeyAndCSR(hostID)
	if err != nil {
		return result, err
	}
	body, err := json.Marshal(map[string]string{
		"pairingCode": pairingCode,
		"tenantScope": tenant,
		"hostId":      hostID,
		"csrPem":      string(csrPEM),
	})
	if err != nil {
		return result, err
	}
	client := newSecureClient(trust)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/pairing/enroll", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("enroll request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return result, err
	}
	if resp.StatusCode != http.StatusCreated {
		return result, fmt.Errorf("enroll rejected (%s): %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return result, fmt.Errorf("enroll response: %w", err)
	}
	if result.TenantScope != tenant || result.HostID != hostID {
		return result, fmt.Errorf("enroll response binding %s/%s does not match requested %s/%s", result.TenantScope, result.HostID, tenant, hostID)
	}
	if err := validateIssued(keyPEM, []byte(result.CertificatePEM), []byte(result.CAPEM), tenant, hostID); err != nil {
		return result, fmt.Errorf("issued certificate failed validation, nothing persisted: %w", err)
	}
	// Server trust remains the operator-configured anchor behind
	// trust; the response CA is issuer evidence only and never
	// replaces it (the two CAs may legitimately differ).
	if len(bootstrapCAPEM) > 0 {
		block, _ := pem.Decode(bootstrapCAPEM)
		if block == nil {
			return result, fmt.Errorf("bootstrap server trust anchor is not valid PEM")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return result, fmt.Errorf("bootstrap server trust anchor: %w", err)
		}
	}
	if err := persistIdentity(id, keyPEM, []byte(result.CertificatePEM), []byte(result.CAPEM)); err != nil {
		return result, err
	}
	return result, nil
}

// validateIssued checks an issued node certificate before trust:
// chain to the issuing CA, exact binding, key match, validity.
func validateIssued(keyPEM, certPEM, issuerCAPEM []byte, tenant, hostID string) error {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("certificate: no PEM block")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("certificate: %w", err)
	}
	issuerBlock, _ := pem.Decode(issuerCAPEM)
	if issuerBlock == nil {
		return fmt.Errorf("issuing CA: no PEM block")
	}
	issuer, err := x509.ParseCertificate(issuerBlock.Bytes)
	if err != nil {
		return fmt.Errorf("issuing CA: %w", err)
	}
	if !issuer.IsCA {
		return fmt.Errorf("issuing CA certificate is not a CA")
	}
	if err := cert.CheckSignatureFrom(issuer); err != nil {
		return fmt.Errorf("certificate does not chain to the issuing CA: %w", err)
	}
	certTenant, certHost, ok := pairing.CertBinding(cert)
	if !ok || certTenant != tenant || certHost != hostID {
		return fmt.Errorf("certificate binding %q/%q does not match requested %q/%q", certTenant, certHost, tenant, hostID)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("node key: no PEM block")
	}
	nodeKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("node key: %w", err)
	}
	certKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate public key is not ECDSA")
	}
	if !certKey.Equal(&nodeKey.PublicKey) {
		return fmt.Errorf("certificate public key does not match the locally generated node key")
	}
	now := time.Now().UTC()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return fmt.Errorf("certificate is not currently valid (%s to %s)", cert.NotBefore, cert.NotAfter)
	}
	return nil
}

// persistIdentity writes key, certificate and issuer CA as one
// recoverable transaction. Order matters for recovery:
//
//  1. Stage every new file (final permissions, synced). No
//     destination has been touched yet.
//  2. Capture backups of every existing destination *in memory*
//     (PEM files are small). Any failure here - including the
//     first backup operation - still leaves every destination
//     exactly as it was. Moving files aside one at a time was
//     the previous defect: a backup failure midway left earlier
//     destinations already moved.
//  3. Rename staged files into place. Any failure rolls the
//     already-committed destinations back from the in-memory
//     backups (or removes them if they did not exist before).
//
// Directory sync is attempted after the renames where the
// platform supports it. This guarantees an all-or-previous
// outcome for ordinary errors; instantaneous power loss during
// step 3 can still leave a mixed set on filesystems that do not
// order the renames, which is a documented qualification limit.
func persistIdentity(id Identity, keyPEM, certPEM, issuerCAPEM []byte) error {
	return persistFiles([]identityFile{
		{id.KeyFile, keyPEM, 0o600},
		{id.CertFile, certPEM, 0o644},
		{id.issuerFile(), issuerCAPEM, 0o644},
	})
}

type identityFile struct {
	path string
	data []byte
	perm os.FileMode
}

func persistFiles(files []identityFile) error {
	type staged struct {
		dest, tmp  string
		backup     []byte
		backupPerm os.FileMode
		hadBackup  bool
		committed  bool
	}
	stages := make([]*staged, 0, len(files))
	cleanup := func() {
		for _, s := range stages {
			if s.tmp != "" {
				_ = os.Remove(s.tmp)
			}
		}
	}
	// Phase 1: stage everything; destinations untouched.
	for _, f := range files {
		if f.path == "" {
			cleanup()
			return fmt.Errorf("identity file path is required")
		}
		dir := filepath.Dir(f.path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				cleanup()
				return err
			}
		}
		tmp, err := os.CreateTemp(dir, ".wazi-identity-*")
		if err != nil {
			cleanup()
			return err
		}
		s := &staged{dest: f.path, tmp: tmp.Name()}
		stages = append(stages, s)
		if err := tmp.Chmod(f.perm); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if _, err := tmp.Write(f.data); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if err := tmp.Close(); err != nil {
			cleanup()
			return err
		}
	}
	// Phase 2: capture every backup before moving anything. A
	// failure on any file (locked, unreadable, a directory)
	// aborts with all destinations untouched.
	for i, f := range files {
		info, err := os.Stat(f.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			cleanup()
			return fmt.Errorf("backup %s: %w", f.path, err)
		}
		if info.IsDir() {
			cleanup()
			return fmt.Errorf("backup %s: is a directory", f.path)
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			cleanup()
			return fmt.Errorf("backup %s: %w", f.path, err)
		}
		stages[i].backup, stages[i].backupPerm, stages[i].hadBackup = data, info.Mode().Perm(), true
	}
	rollback := func() {
		for _, s := range stages {
			if !s.committed {
				continue
			}
			if s.hadBackup {
				_ = os.WriteFile(s.dest, s.backup, s.backupPerm)
			} else {
				_ = os.Remove(s.dest)
			}
		}
		cleanup()
	}
	// Phase 3: commit. Any rename failure restores committed files.
	dirs := map[string]bool{}
	for _, s := range stages {
		if err := os.Rename(s.tmp, s.dest); err != nil {
			rollback()
			return fmt.Errorf("persist identity: %w", err)
		}
		s.committed = true
		s.tmp = ""
		dirs[filepath.Dir(s.dest)] = true
	}
	for dir := range dirs {
		syncDirBestEffort(dir)
	}
	return nil
}

func syncDirBestEffort(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}

// LoadTLS builds the mTLS client config from identity files.
// RootCAs come from the operator's server trust file (CAFile),
// never from an enrollment response.
// serverName is verified against the server certificate; pass the
// control-plane host name (or an explicit override in tests).
func LoadTLS(id Identity, serverName string) (*tls.Config, error) {
	// Consistent with LoadBootstrap: an absent server-trust file
	// means system roots, not a hard failure. A present file must
	// parse and becomes the exclusive anchor.
	var pool *x509.CertPool
	caPEM, err := os.ReadFile(id.CAFile)
	switch {
	case err == nil:
		pool = x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("CA file contains no certificates")
		}
	case os.IsNotExist(err):
		pool = nil // system roots
	default:
		return nil, fmt.Errorf("read CA: %w", err)
	}
	pair, err := tls.LoadX509KeyPair(id.CertFile, id.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load node key pair: %w", err)
	}
	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{pair},
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// LoadBootstrap builds the server-auth-only config used during
// enrollment and returns the operator-supplied anchor bytes behind
// it (nil when the file does not exist and system roots apply).
// The anchor is the server trust the operator configured; Enroll
// receives it explicitly so response handling can never confuse
// it with the client issuing CA.
func LoadBootstrap(caFile, serverName string) (*tls.Config, []byte, error) {
	config := &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12}
	if caFile == "" {
		return config, nil, nil
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil, nil
		}
		return nil, nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, nil, fmt.Errorf("CA file contains no certificates")
	}
	config.RootCAs = pool
	return config, caPEM, nil
}

// EnrollmentTLS is LoadBootstrap without the anchor bytes.
func EnrollmentTLS(caFile, serverName string) (*tls.Config, error) {
	config, _, err := LoadBootstrap(caFile, serverName)
	return config, err
}

// Renew rotates the node certificate before expiry, in the
// server's two phases so no failure can strand the node:
//
//  1. Initiate: a fresh node key/CSR is sent under the current
//     certificate; the server records the issued replacement as
//     pending and keeps the old certificate fully authorised.
//  2. The response is validated (same checks as enrollment,
//     including an unchanged issuing CA) and persisted locally.
//     If this fails, the old files are untouched and the old
//     identity still works; renewal is simply retried.
//  3. A pending marker is written, then Activate presents the new
//     certificate to retire the old one. If activation or its
//     response is lost, the marker stays: ActivatePending (run
//     automatically by the agent before submission) retries it
//     with the persisted new identity.
func Renew(ctx context.Context, baseURL string, tlsConfig *tls.Config, tenant, hostID string, id Identity) (EnrollResult, error) {
	var result EnrollResult
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return result, err
	}
	keyPEM, csrPEM, err := pairing.GenerateNodeKeyAndCSR(hostID)
	if err != nil {
		return result, err
	}
	body, err := json.Marshal(map[string]string{"csrPem": string(csrPEM)})
	if err != nil {
		return result, err
	}
	client := newSecureClient(tlsConfig)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/pairing/renew", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("renew request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return result, err
	}
	if resp.StatusCode != http.StatusCreated {
		return result, fmt.Errorf("renew rejected (%s): %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return result, fmt.Errorf("renew response: %w", err)
	}
	if result.TenantScope != tenant || result.HostID != hostID {
		return result, fmt.Errorf("renew response binding %s/%s does not match %s/%s", result.TenantScope, result.HostID, tenant, hostID)
	}
	// Renewal must come from the stored issuing CA; a different
	// issuer requires explicit re-enrollment, not silent rotation.
	if stored, err := os.ReadFile(id.issuerFile()); err == nil {
		storedBlock, _ := pem.Decode(stored)
		respBlock, _ := pem.Decode([]byte(result.CAPEM))
		if storedBlock == nil || respBlock == nil {
			return result, fmt.Errorf("stored or response issuing CA is not valid PEM, nothing persisted")
		}
		storedCert, err1 := x509.ParseCertificate(storedBlock.Bytes)
		respCert, err2 := x509.ParseCertificate(respBlock.Bytes)
		if err1 != nil || err2 != nil || string(storedCert.Raw) != string(respCert.Raw) {
			return result, fmt.Errorf("renewal was issued by a different CA than enrollment; re-enroll explicitly, nothing persisted")
		}
	}
	if err := validateIssued(keyPEM, []byte(result.CertificatePEM), []byte(result.CAPEM), tenant, hostID); err != nil {
		return result, fmt.Errorf("renewed certificate failed validation, nothing persisted: %w", err)
	}
	if err := persistIdentity(id, keyPEM, []byte(result.CertificatePEM), []byte(result.CAPEM)); err != nil {
		return result, err
	}
	// Mark pending before activation so a lost activation (or its
	// response) is retryable across restarts, then activate with
	// the freshly persisted identity.
	marker := []byte("tenant=" + tenant + " host=" + hostID + "\n")
	if err := os.WriteFile(id.pendingFile(), marker, 0o600); err != nil {
		return result, fmt.Errorf("persisted renewed identity but could not write the pending marker (activation can still be completed with ActivatePending): %w", err)
	}
	if err := activate(ctx, base, tlsConfig, id); err != nil {
		return result, fmt.Errorf("renewed identity persisted but activation is pending (retry with ActivatePending): %w", err)
	}
	_ = os.Remove(id.pendingFile())
	return result, nil
}

// activate presents the persisted (pending) certificate to retire
// the old one. tlsConfig supplies server trust/name; its client
// certificate is replaced with the persisted pair.
func activate(ctx context.Context, base string, tlsConfig *tls.Config, id Identity) error {
	pair, err := tls.LoadX509KeyPair(id.CertFile, id.KeyFile)
	if err != nil {
		return fmt.Errorf("load renewed identity: %w", err)
	}
	newConfig := tlsConfig.Clone()
	newConfig.Certificates = []tls.Certificate{pair}
	client := newSecureClient(newConfig)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/pairing/renew/activate", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("activate request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("activate rejected (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// ActivatePending completes a previously persisted renewal when
// its marker is present (activation was lost or deferred). It
// reports whether a pending renewal existed. The server treats
// activation with the already-active certificate as success, so
// retrying after a lost success response is safe.
func ActivatePending(ctx context.Context, baseURL string, tlsConfig *tls.Config, id Identity) (bool, error) {
	if !id.HasPendingRenewal() {
		return false, nil
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return true, err
	}
	if err := activate(ctx, base, tlsConfig, id); err != nil {
		return true, err
	}
	_ = os.Remove(id.pendingFile())
	return true, nil
}

// SubmitProfile posts one profile over mTLS. The control plane
// rejects any profile whose tenant/host differs from the
// certificate binding (cross-tenant/node submission).
func SubmitProfile(ctx context.Context, baseURL string, tlsConfig *tls.Config, profile contracts.HostProfile) error {
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	client := newSecureClient(tlsConfig)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/host-profiles", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("submit profile: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("submit profile rejected (%s): %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}
