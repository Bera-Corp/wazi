// Package pairing implements Phase 1 node pairing: one-time
// enrollment codes, a node registry with revocation, and the CA
// operations that bind a client certificate to exactly one
// authorised (tenant, node) pair.
//
// Order is load-bearing (pairing -> verified registry -> agent
// pre-mutation checks -> execution): nothing in this package
// authorises deployment. It only establishes who a node is.
// Deployment stays disabled until the remaining safety gates pass.
package pairing

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Sentinel errors. Handlers map these to stable HTTP statuses
// without leaking which check failed to unauthenticated callers.
var (
	ErrCodeNotFound    = errors.New("pairing code not found")
	ErrCodeExpired     = errors.New("pairing code expired")
	ErrCodeReplay      = errors.New("pairing code already used")
	ErrCodeInvalidated = errors.New("pairing code invalidated by revocation")
	ErrBindingMismatch = errors.New("binding mismatch")
	ErrNodeExists      = errors.New("node already enrolled")
	ErrUnknownNode     = errors.New("unknown node")
	ErrRevoked         = errors.New("node revoked")
	ErrCertExpired     = errors.New("certificate expired")
	ErrCertBinding     = errors.New("certificate carries no wazi binding")
)

// Node is one enrolled (tenant, host) identity.
type Node struct {
	TenantScope string
	HostID      string
	Serial      string
	Fingerprint string // sha256 hex of the certificate DER
	EnrolledAt  time.Time
	ExpiresAt   time.Time // certificate NotAfter at enrollment
	Revoked     bool
	RevokedAt   time.Time
	// Pending rotation (two-phase renewal): the replacement
	// certificate that has been issued but not yet activated by
	// presenting it. Empty when no renewal is in flight. The
	// active fields above keep authorising until activation.
	PendingSerial      string    `json:"pendingSerial,omitempty"`
	PendingFingerprint string    `json:"pendingFingerprint,omitempty"`
	PendingExpiresAt   time.Time `json:"pendingExpiresAt,omitempty"`
}

type pairingCode struct {
	tenant      string
	hostID      string
	createdAt   time.Time
	expiresAt   time.Time
	generation  uint64 // revocation epoch at mint time
	used        bool
	invalidated bool // set by Revoke for every code minted before it
}

// Registry is the control plane's pairing state. It is in-memory
// for the Phase 1 skeleton; persistence is required before any
// multi-instance deployment, and restarts must fail closed (nodes
// re-enroll) rather than resurrect revoked identities.
type Registry struct {
	mu            sync.Mutex
	codes         map[string]*pairingCode
	nodes         map[string]*Node // key: tenant + "\x00" + hostID
	byFingerprint map[string]string
	generations   map[string]uint64 // per-(tenant,node) revocation epoch
	storePath     string            // "" = in-memory only (see store.go)
	storeLock     *os.File
	lockPath      string
	now           func() time.Time
}

// NewRegistry creates an empty registry. now may be nil (time.Now).
func NewRegistry(now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{
		codes:         map[string]*pairingCode{},
		nodes:         map[string]*Node{},
		byFingerprint: map[string]string{},
		generations:   map[string]uint64{},
		now:           now,
	}
}

func nodeKey(tenant, hostID string) string { return tenant + "\x00" + hostID }

// registrySnapshot is a deep copy of all mutable registry state.
// Mutations that persist (saveLocked) run transactionally: take a
// snapshot, mutate, save; if the save fails, restore the snapshot
// so live authorisation state never diverges from what was
// reported. A failed save must not consume a code, revoke a node,
// or retire a certificate that callers were told still works.
type registrySnapshot struct {
	codes         map[string]*pairingCode
	nodes         map[string]*Node
	byFingerprint map[string]string
	generations   map[string]uint64
}

func (r *Registry) snapshotLocked() registrySnapshot {
	snap := registrySnapshot{
		codes:         make(map[string]*pairingCode, len(r.codes)),
		nodes:         make(map[string]*Node, len(r.nodes)),
		byFingerprint: make(map[string]string, len(r.byFingerprint)),
		generations:   make(map[string]uint64, len(r.generations)),
	}
	for hash, entry := range r.codes {
		copyEntry := *entry
		snap.codes[hash] = &copyEntry
	}
	for key, node := range r.nodes {
		copyNode := *node
		snap.nodes[key] = &copyNode
	}
	for fp, key := range r.byFingerprint {
		snap.byFingerprint[fp] = key
	}
	for key, gen := range r.generations {
		snap.generations[key] = gen
	}
	return snap
}

func (r *Registry) restoreLocked(snap registrySnapshot) {
	r.codes = snap.codes
	r.nodes = snap.nodes
	r.byFingerprint = snap.byFingerprint
	r.generations = snap.generations
}

// CreatePairingCode mints a one-time code bound to (tenant, hostID).
// The code is the enrollment challenge: whoever presents it with a
// CSR for the same binding receives that node's certificate, once.
func (r *Registry) CreatePairingCode(tenant, hostID string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(hostID) == "" {
		return "", fmt.Errorf("tenant and hostId are required")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("pairing code ttl must be positive")
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := hex.EncodeToString(buf)
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := r.snapshotLocked()
	now := r.now()
	key := nodeKey(tenant, hostID)
	r.codes[hashCode(code)] = &pairingCode{tenant: tenant, hostID: hostID, createdAt: now, expiresAt: now.Add(ttl), generation: r.generations[key]}
	if err := r.saveLocked(); err != nil {
		r.restoreLocked(snap)
		return "", err
	}
	return code, nil
}

// Enroll consumes code and registers the issued certificate. The
// consume-and-register is atomic so a replayed code can never mint
// a second certificate, even concurrently.
func (r *Registry) Enroll(code, tenant, hostID, serial, fingerprint string, certExpiresAt time.Time) (Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := r.snapshotLocked()
	entry, ok := r.codes[hashCode(code)]
	if !ok {
		return Node{}, ErrCodeNotFound
	}
	if entry.used {
		return Node{}, ErrCodeReplay
	}
	if entry.invalidated {
		return Node{}, ErrCodeInvalidated
	}
	now := r.now()
	if !now.Before(entry.expiresAt) {
		return Node{}, ErrCodeExpired
	}
	if entry.tenant != tenant || entry.hostID != hostID {
		return Node{}, ErrBindingMismatch
	}
	key := nodeKey(tenant, hostID)
	// Defence in depth: any code minted in an earlier revocation
	// epoch is dead, even if its invalidated flag was somehow not
	// set. Re-enrollment after revocation requires a code minted
	// after it (fresh operator approval, current epoch).
	if entry.generation != r.generations[key] {
		return Node{}, ErrCodeInvalidated
	}
	if existing, ok := r.nodes[key]; ok && !existing.Revoked && existing.ExpiresAt.After(now) {
		return Node{}, ErrNodeExists
	} else if ok {
		// Replacing a revoked or expired identity with a fresh,
		// operator-approved enrollment: drop the old certificate's
		// fingerprint so it can never authenticate against the new
		// registration. Live (unexpired, unrevoked) registrations
		// cannot be replaced this way; they renew instead. This is
		// also the documented re-pair path for expired nodes, which
		// cannot renew with an expired certificate.
		delete(r.byFingerprint, existing.Fingerprint)
		if existing.PendingFingerprint != "" {
			delete(r.byFingerprint, existing.PendingFingerprint)
		}
	}
	entry.used = true
	node := &Node{
		TenantScope: tenant, HostID: hostID, Serial: serial,
		Fingerprint: fingerprint, EnrolledAt: now, ExpiresAt: certExpiresAt,
	}
	r.nodes[key] = node
	r.byFingerprint[fingerprint] = key
	if err := r.saveLocked(); err != nil {
		r.restoreLocked(snap)
		return Node{}, err
	}
	return *node, nil
}

// Revoke marks a node revoked and invalidates every outstanding
// pairing code for that (tenant, node): revocation is a statement
// about the identity, not only about one certificate, so a spare
// code minted earlier must not quietly re-enroll the same node.
// Re-enrollment afterwards requires a freshly minted code (fresh
// operator approval, in the new revocation epoch). Revoking a
// never-enrolled (tenant, node) still advances its epoch and kills
// its outstanding codes; it reports ErrUnknownNode so operators
// notice the typo, but the invalidation is not rolled back.
func (r *Registry) Revoke(tenant, hostID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := r.snapshotLocked()
	key := nodeKey(tenant, hostID)
	r.generations[key]++
	for _, entry := range r.codes {
		if entry.tenant == tenant && entry.hostID == hostID && !entry.used {
			entry.invalidated = true
		}
	}
	node, ok := r.nodes[key]
	if !ok {
		// Epoch advance and code invalidation above still persist
		// on success: a typo'd revocation must not leave spare
		// codes alive. On save failure everything is restored.
		if err := r.saveLocked(); err != nil {
			r.restoreLocked(snap)
			return err
		}
		return ErrUnknownNode
	}
	node.Revoked = true
	node.RevokedAt = r.now()
	node.PendingSerial, node.PendingFingerprint, node.PendingExpiresAt = "", "", time.Time{}
	// The fingerprint mapping is kept so Authenticate can report
	// ErrRevoked precisely; it is dropped only when a fresh
	// enrollment replaces the identity (see Enroll).
	if err := r.saveLocked(); err != nil {
		r.restoreLocked(snap)
		return err
	}
	return nil
}

// BeginRotation starts two-phase renewal: authenticated with the
// current certificate, it records the issued replacement as
// pending without retiring the active certificate. If the client
// then fails to persist or loses the response, its old identity
// keeps working and renewal can simply be retried - the node is
// never stranded by a half-completed rotation. A newer
// BeginRotation replaces any earlier pending certificate.
func (r *Registry) BeginRotation(cert *x509.Certificate, newSerial, newFingerprint string, newExpiresAt time.Time) (Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := r.snapshotLocked()
	node, err := r.authenticateLocked(cert)
	if err != nil {
		return Node{}, err
	}
	stored := r.nodes[nodeKey(node.TenantScope, node.HostID)]
	stored.PendingSerial = newSerial
	stored.PendingFingerprint = newFingerprint
	stored.PendingExpiresAt = newExpiresAt
	if err := r.saveLocked(); err != nil {
		r.restoreLocked(snap)
		return Node{}, err
	}
	return *stored, nil
}

// ActivateRotation completes renewal when the node presents the
// pending certificate itself (proof it persisted the new key and
// certificate). Atomically: the pending identity becomes active,
// the old fingerprint is retired, and pending state clears. The
// operation is idempotency-friendly in one direction: retrying
// activation with the now-active certificate authenticates
// normally, which the endpoint treats as already-activated.
func (r *Registry) ActivateRotation(cert *x509.Certificate) (Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cert == nil {
		return Node{}, ErrCertBinding
	}
	tenant, hostID, ok := CertBinding(cert)
	if !ok {
		return Node{}, ErrCertBinding
	}
	now := r.now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return Node{}, ErrCertExpired
	}
	key := nodeKey(tenant, hostID)
	stored, ok := r.nodes[key]
	if !ok {
		return Node{}, ErrUnknownNode
	}
	if stored.Revoked {
		return Node{}, ErrRevoked
	}
	// Already activated (a retry after a lost response): the
	// presented cert is the active one, so activation has
	// observably succeeded.
	if stored.Fingerprint == Fingerprint(cert) && stored.Serial == cert.SerialNumber.String() {
		return *stored, nil
	}
	if stored.PendingFingerprint == "" || stored.PendingFingerprint != Fingerprint(cert) || stored.PendingSerial != cert.SerialNumber.String() {
		return Node{}, ErrUnknownNode
	}
	snap := r.snapshotLocked()
	delete(r.byFingerprint, stored.Fingerprint)
	stored.Serial = stored.PendingSerial
	stored.Fingerprint = stored.PendingFingerprint
	stored.ExpiresAt = stored.PendingExpiresAt
	stored.PendingSerial, stored.PendingFingerprint, stored.PendingExpiresAt = "", "", time.Time{}
	r.byFingerprint[stored.Fingerprint] = key
	if err := r.saveLocked(); err != nil {
		r.restoreLocked(snap)
		return Node{}, err
	}
	return *stored, nil
}

// Node returns a copy of the registered node, if any.
func (r *Registry) Node(tenant, hostID string) (Node, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[nodeKey(tenant, hostID)]
	if !ok {
		return Node{}, false
	}
	return *node, true
}

// Authenticate resolves a peer certificate to its registered node
// and enforces binding, revocation and expiry. A valid certificate
// from the CA is not enough: it must be the exact certificate
// issued at enrollment for that (tenant, node).
func (r *Registry) Authenticate(cert *x509.Certificate) (Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.authenticateLocked(cert)
}

// AuthorizeCommit authenticates cert and runs commit while still
// holding the registry lock that Revoke also takes. This makes the
// final authorisation and the caller's write one linearisable
// step with respect to revocation: either the commit lands before
// revocation takes effect, or it observes the revoked state and
// must not write. A second standalone Authenticate before the
// write would still leave a check-then-act race; callers must do
// the actual store inside commit. commit must be fast, must not
// call back into the Registry, and its error is returned verbatim.
func (r *Registry) AuthorizeCommit(cert *x509.Certificate, commit func(Node) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, err := r.authenticateLocked(cert)
	if err != nil {
		return err
	}
	return commit(node)
}

// authenticateLocked is Authenticate with r.mu already held.
func (r *Registry) authenticateLocked(cert *x509.Certificate) (Node, error) {
	if cert == nil {
		return Node{}, ErrCertBinding
	}
	tenant, hostID, ok := CertBinding(cert)
	if !ok {
		return Node{}, ErrCertBinding
	}
	now := r.now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return Node{}, ErrCertExpired
	}
	fingerprint := Fingerprint(cert)
	key, ok := r.byFingerprint[fingerprint]
	if !ok {
		return Node{}, ErrUnknownNode
	}
	node := r.nodes[key]
	if node == nil || node.TenantScope != tenant || node.HostID != hostID {
		return Node{}, ErrBindingMismatch
	}
	if node.Serial != cert.SerialNumber.String() {
		return Node{}, ErrBindingMismatch
	}
	if node.Revoked {
		return Node{}, ErrRevoked
	}
	return *node, nil
}

// Fingerprint returns the sha256 hex fingerprint of a certificate.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// BindingURI encodes the authorised (tenant, node) pair carried in
// the certificate's URI SANs.
func BindingURI(tenant, hostID string) string {
	return "urn:wazi:tenant:" + tenant + ":node:" + hostID
}

// CertBinding extracts the (tenant, node) binding from a certificate.
// It returns ok=false when no well-formed Wazi binding is present.
func CertBinding(cert *x509.Certificate) (tenant, hostID string, ok bool) {
	for _, uri := range cert.URIs {
		if uri == nil {
			continue
		}
		rest, found := strings.CutPrefix(uri.String(), "urn:wazi:tenant:")
		if !found {
			continue
		}
		t, h, found := strings.Cut(rest, ":node:")
		if !found || t == "" || h == "" {
			continue
		}
		return t, h, true
	}
	return "", "", false
}

// --- CA ---

// CA is the control plane's issuing authority for node and server
// certificates. Private CA material is never committed; it is
// loaded from files or generated ephemerally for dev/tests.
type CA struct {
	Cert    *x509.Certificate
	Key     crypto.Signer
	certPEM []byte
}

// GenerateCA creates a new ECDSA CA valid for 10 years.
func GenerateCA(commonName string) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{Cert: cert, Key: key, certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}

// LoadCA loads a PEM CA certificate and ECDSA private key.
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("ca certificate: no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca certificate: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("ca key: no PEM block")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		anyKey, pkcs8Err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if pkcs8Err != nil {
			return nil, fmt.Errorf("ca key: %w", err)
		}
		ecKey, ok := anyKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("ca key: not an ECDSA key")
		}
		key = ecKey
	}
	return &CA{Cert: cert, Key: key, certPEM: certPEM}, nil
}

// CertPEM returns the CA certificate PEM (public; safe to distribute).
func (ca *CA) CertPEM() []byte { return ca.certPEM }

// KeyPEM returns the CA private key PEM. Callers must store it as a
// secret (0600) and never commit it.
func (ca *CA) KeyPEM() ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(ca.Key.(*ecdsa.PrivateKey))
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

// Pool returns a cert pool trusting only this CA.
func (ca *CA) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	return pool
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

// IssueServerCert issues a server certificate for the control plane.
// The returned key is freshly generated here; file-based deployments
// may instead bring their own cert/key issued by this CA.
func (ca *CA) IssueServerCert(dnsNames []string, ips []net.IP, ttl time.Duration) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	certPEM, _, err = ca.issue(&x509.Certificate{
		DNSNames:    dnsNames,
		IPAddresses: ips,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &key.PublicKey, ttl, time.Now().UTC())
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

// IssueClientCert generates a fresh node key and a client certificate
// bound to (tenant, hostID). Production enrollment uses
// IssueClientCertFromCSR so the node key never leaves the node; this
// helper exists for tests and bootstrap tooling.
func (ca *CA) IssueClientCert(tenant, hostID string, ttl time.Duration) (certPEM, keyPEM []byte, cert *x509.Certificate, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	certPEM, cert, err = ca.issueClient(&key.PublicKey, tenant, hostID, ttl, time.Now().UTC())
	if err != nil {
		return nil, nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	return certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), cert, nil
}

// IssueExpiredClientCert issues an already-expired bound certificate.
// It exists so expiry rejection can be tested honestly.
func (ca *CA) IssueExpiredClientCert(tenant, hostID string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	certPEM, _, err = ca.issueClient(&key.PublicKey, tenant, hostID, time.Hour, past)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func (ca *CA) issueClient(pub crypto.PublicKey, tenant, hostID string, ttl time.Duration, notBefore time.Time) ([]byte, *x509.Certificate, error) {
	binding, err := url.Parse(BindingURI(tenant, hostID))
	if err != nil {
		return nil, nil, err
	}
	return ca.issue(&x509.Certificate{
		Subject:     pkix.Name{CommonName: hostID},
		URIs:        []*url.URL{binding},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}, pub, ttl, notBefore)
}

func (ca *CA) issue(extra *x509.Certificate, pub crypto.PublicKey, ttl time.Duration, notBefore time.Time) ([]byte, *x509.Certificate, error) {
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := *extra
	tmpl.SerialNumber = serial
	tmpl.NotBefore = notBefore.Add(-time.Minute)
	tmpl.NotAfter = notBefore.Add(ttl)
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	tmpl.BasicConstraintsValid = true
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, ca.Cert, pub, ca.Key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert, nil
}

// IssueClientCertFromCSR verifies the CSR signature and issues a
// client certificate whose binding comes from the enrollment record,
// never from CSR-asserted fields. This is the production path: the
// node's private key is generated on the node and never transmitted.
func (ca *CA) IssueClientCertFromCSR(csrPEM []byte, tenant, hostID string, ttl time.Duration) (certPEM []byte, cert *x509.Certificate, err error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("csr: no PEM block")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("csr: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, nil, fmt.Errorf("csr signature: %w", err)
	}
	return ca.issueClient(csr.PublicKey, tenant, hostID, ttl, time.Now().UTC())
}

// GenerateNodeKeyAndCSR creates a node key pair and a CSR. The agent
// keeps the returned key PEM private (0600) and sends only the CSR.
func GenerateNodeKeyAndCSR(hostID string) (keyPEM, csrPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: hostID},
	}, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

// ServerTLSConfig builds the control-plane listener config: TLS 1.2+
// with client certificates verified against ca whenever presented.
// ClientAuth is VerifyClientCertIfGiven (not Require) for one
// reason: the enrollment endpoint must be reachable by a node that
// has no certificate yet, and TLS cannot vary client-auth by HTTP
// path. Authorisation is therefore enforced per route: enrollment
// requires the one-time pairing code, and every profile route calls
// Registry.Authenticate, which rejects missing, unenrolled and
// revoked certificates. Untrusted or expired presented certificates
// still fail the handshake itself.
func ServerTLSConfig(ca *CA, serverCert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    ca.Pool(),
		MinVersion:   tls.VersionTLS12,
	}
}

// ClientTLSConfig builds the agent's dial config. serverName is
// verified against the server certificate; a wrong identity fails
// the handshake. Empty client cert/key means no client certificate
// (used only by negative tests).
func ClientTLSConfig(ca *CA, certPEM, keyPEM []byte, serverName string) (*tls.Config, error) {
	config := &tls.Config{
		RootCAs:    ca.Pool(),
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}
	if len(certPEM) > 0 {
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{pair}
	}
	return config, nil
}
