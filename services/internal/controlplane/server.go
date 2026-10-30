// Package controlplane is the Wazi control plane HTTP surface.
//
// Phase 1 trust order: pairing (this package) -> verified manifest and
// procedure registry -> approval and bounded execution. The registry rejects
// unsigned or untrusted records; deployment remains deliberately
// disabled until the remaining safety gates pass.
package controlplane

import (
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/approvalregistry"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
)

// Limits bound intake so a single caller cannot exhaust memory or
// storage (review P2).
const (
	MaxProfileBodyBytes = 64 * 1024
	MaxStoredProfiles   = 1000
	ClientCertTTL       = 24 * time.Hour
	PairingCodeTTL      = 10 * time.Minute
)

// Config controls one Server. RequireMTLS is set automatically when
// a CA is configured; plain mode is a loopback-only dev mode and the
// binary refuses to bind it beyond loopback.
type Config struct {
	Registry               *pairing.Registry
	ManifestRegistry       *manifestregistry.Registry
	ApprovalRegistry       *approvalregistry.Registry
	CapabilityPolicyBundle []byte
	CA                     *pairing.CA // nil = plain loopback dev mode (no enrollment issuance)
	Now                    func() time.Time
	// AllowPlaintextProfiles permits profile intake without a client
	// certificate. Only safe on a loopback dev listener; the binary
	// enforces that combination. Tests set it explicitly.
	AllowPlaintextProfiles bool
	// OperatorToken, when set, is required (constant-time compared)
	// in the X-Wazi-Operator-Token header for code minting and
	// revocation, in addition to the loopback restriction. The
	// binary sources it from the environment, never from a flag
	// value that would leak into process listings or history.
	OperatorToken string
	// ApproverID and ApproverRole describe the single configured local
	// operator principal authenticated by OperatorToken for the pilot.
	// This does not provide distinct-user quorum or a distributed identity.
	ApproverID   string
	ApproverRole string
	// ApproverCredentials enables separately provisioned principal credentials
	// for approval issuance. Distinct IDs and credentials are checked, while
	// distinct human custody remains an operator responsibility.
	ApproverCredentials []ApproverCredential
	// BodyReadTimeout bounds how long a request body may take to
	// arrive, so an unfinished enrollment/profile body cannot hold
	// a handler open indefinitely. Zero selects the default.
	BodyReadTimeout time.Duration
}

// DefaultBodyReadTimeout is the per-request body deadline.
const DefaultBodyReadTimeout = 15 * time.Second

func (s *Server) bodyTimeout() time.Duration {
	if s.cfg.BodyReadTimeout > 0 {
		return s.cfg.BodyReadTimeout
	}
	return DefaultBodyReadTimeout
}

// beginBodyRead applies the body deadline to this connection. It is
// best-effort: writers that do not support deadlines simply proceed
// under the size limit alone.
func (s *Server) beginBodyRead(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(s.bodyTimeout()))
}

// Server holds pairing and profile state.
type Server struct {
	cfg      Config
	mu       sync.Mutex
	profiles map[string]contracts.HostProfile // key: tenant + "\x00" + hostID
}

// New creates a Server. A nil Registry gets a fresh one.
func New(cfg Config) *Server {
	if cfg.Registry == nil {
		cfg.Registry = pairing.NewRegistry(cfg.Now)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Server{cfg: cfg, profiles: map[string]contracts.HostProfile{}}
}

// Registry exposes the pairing registry (operator actions: mint
// codes, revoke nodes).
func (s *Server) Registry() *pairing.Registry { return s.cfg.Registry }

// RequireMTLS reports whether authenticated intake is enforced.
func (s *Server) RequireMTLS() bool { return s.cfg.CA != nil }

// TLSConfig returns the mTLS listener config, or nil in plain mode.
func (s *Server) TLSConfig(serverCert tls.Certificate) *tls.Config {
	if s.cfg.CA == nil {
		return nil
	}
	return pairing.ServerTLSConfig(s.cfg.CA, serverCert)
}

// Handler returns the HTTP routing surface.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	registerBrandAssets(mux)
	mux.HandleFunc("GET /{$}", handleStatusPage)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/pairing/codes", s.handleCreateCode)
	mux.HandleFunc("POST /v1/pairing/revoke", s.handleRevoke)
	mux.HandleFunc("POST /v1/pairing/enroll", s.handleEnroll)
	mux.HandleFunc("POST /v1/pairing/renew", s.handleRenew)
	mux.HandleFunc("POST /v1/pairing/renew/activate", s.handleRenewActivate)
	mux.HandleFunc("POST /v1/host-profiles", s.handlePostProfile)
	mux.HandleFunc("GET /v1/host-profiles", s.handleListProfiles)
	mux.HandleFunc("POST /v1/manifests", s.handlePostManifest)
	mux.HandleFunc("GET /v1/manifests/{appID}/{version}", s.handleGetManifest)
	mux.HandleFunc("POST /v1/procedures", s.handlePostProcedure)
	mux.HandleFunc("GET /v1/procedures/{procedureID}/{version}", s.handleGetProcedure)
	mux.HandleFunc("GET /v1/policy-bundles/{policyID}/{version}", s.handleGetCapabilityPolicyBundle)
	mux.HandleFunc("POST /v1/revocations", s.handlePostRevocation)
	mux.HandleFunc("GET /v1/revocations/current", s.handleGetRevocation)
	mux.HandleFunc("POST /v1/approvals", s.handleIssueApproval)
	mux.HandleFunc("POST /v1/approvals/{approvalID}/consume", s.handleConsumeApproval)
	// Deployment stays disabled until approval, runtime-boundary,
	// revocation, journal, and lifecycle gates pass.
	mux.HandleFunc("POST /v1/deployments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error":  "deployment_disabled",
			"detail": "deployment is disabled until operator approval, policy and revocation checks, bounded runtime, journal recovery, and lifecycle acceptance pass",
		})
	})
	return mux
}

type createCodeRequest struct {
	TenantScope string `json:"tenantScope"`
	HostID      string `json:"hostId"`
}

// requireOperator enforces the operator boundary for minting and
// revocation: same-host (loopback) callers only, plus the
// configured operator token when one is set. Loopback alone is a
// skeleton boundary, not sufficient behind any reverse proxy that
// rewrites the source address; deployments must set the token (or
// a future admin identity) before exposing the listener.
func (s *Server) requireOperator(w http.ResponseWriter, r *http.Request) bool {
	if !isLoopbackRemote(r.RemoteAddr) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "operator actions may only be performed by a local operator"})
		return false
	}
	if s.cfg.OperatorToken != "" {
		supplied := r.Header.Get("X-Wazi-Operator-Token")
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(s.cfg.OperatorToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "operator token required"})
			return false
		}
	}
	return true
}

// handleCreateCode is the local-operator surface for minting
// one-time pairing codes, additionally gated by the operator token
// when configured.
func (s *Server) handleCreateCode(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	var req createCodeRequest
	if err := s.decodeStrict(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "detail": err.Error()})
		return
	}
	code, err := s.cfg.Registry.CreatePairingCode(req.TenantScope, req.HostID, PairingCodeTTL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pairing_request", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"pairingCode": code,
		"tenantScope": req.TenantScope,
		"hostId":      req.HostID,
		"expiresIn":   PairingCodeTTL.String(),
	})
}

// handleRevoke is the operator revocation surface (same boundary
// as code minting).
func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	var req createCodeRequest
	if err := s.decodeStrict(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "detail": err.Error()})
		return
	}
	if err := s.cfg.Registry.Revoke(req.TenantScope, req.HostID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_node", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"revoked": req.HostID, "tenantScope": req.TenantScope})
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type enrollRequest struct {
	PairingCode string `json:"pairingCode"`
	TenantScope string `json:"tenantScope"`
	HostID      string `json:"hostId"`
	CSR         string `json:"csrPem"`
}

type enrollResponse struct {
	CertificatePEM string `json:"certificatePem"`
	CAPEM          string `json:"caPem"`
	TenantScope    string `json:"tenantScope"`
	HostID         string `json:"hostId"`
	ExpiresAt      string `json:"expiresAt"`
}

// handleEnroll exchanges a one-time pairing code + CSR for a node
// certificate bound to the code's (tenant, node). It intentionally
// does not require a client certificate (the node has none yet);
// its authorisation is the single-use, expiring code, and transport
// server identity is verified by the agent against the CA.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "enrollment_unavailable"})
		return
	}
	var req enrollRequest
	if err := s.decodeStrict(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "detail": err.Error()})
		return
	}
	if req.PairingCode == "" || req.TenantScope == "" || req.HostID == "" || req.CSR == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pairingCode, tenantScope, hostId and csrPem are required"})
		return
	}
	certPEM, cert, err := s.cfg.CA.IssueClientCertFromCSR([]byte(req.CSR), req.TenantScope, req.HostID, ClientCertTTL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_csr", "detail": err.Error()})
		return
	}
	// Issue first, then atomically consume+register. If the code is
	// replayed/expired/mismatched, the minted cert is discarded and
	// never registered, so Authenticate will reject it as unknown.
	if _, err := s.cfg.Registry.Enroll(req.PairingCode, req.TenantScope, req.HostID,
		cert.SerialNumber.String(), pairing.Fingerprint(cert), cert.NotAfter); err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, pairing.ErrBindingMismatch) || errors.Is(err, pairing.ErrNodeExists) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": "enrollment_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, enrollResponse{
		CertificatePEM: string(certPEM),
		CAPEM:          string(s.cfg.CA.CertPEM()),
		TenantScope:    req.TenantScope,
		HostID:         req.HostID,
		ExpiresAt:      cert.NotAfter.UTC().Format(time.RFC3339),
	})
}

type renewRequest struct {
	CSR string `json:"csrPem"`
}

// handleRenew begins two-phase renewal: authenticated with the
// current (still valid, unrevoked) certificate, it issues a
// replacement with the same server-side binding and records it as
// pending. The old certificate stays fully authorised until the
// node activates the replacement by presenting it
// (handleRenewActivate), so a client that fails to persist or
// loses this response is never stranded. Renewal after expiry or
// revocation is impossible by design: expired nodes re-pair with
// a fresh operator code (Enroll replaces expired registrations),
// revoked nodes need revocation lifted by re-pairing after a
// fresh code in the new epoch.
func (s *Server) handleRenew(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "enrollment_unavailable"})
		return
	}
	node, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var req renewRequest
	if err := s.decodeStrict(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "detail": err.Error()})
		return
	}
	if req.CSR == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "csrPem is required"})
		return
	}
	certPEM, cert, err := s.cfg.CA.IssueClientCertFromCSR([]byte(req.CSR), node.TenantScope, node.HostID, ClientCertTTL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_csr", "detail": err.Error()})
		return
	}
	if _, err := s.cfg.Registry.BeginRotation(peerCert(r), cert.SerialNumber.String(), pairing.Fingerprint(cert), cert.NotAfter); err != nil {
		writeJSON(w, authErrorStatus(err), map[string]string{"error": "renewal_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, enrollResponse{
		CertificatePEM: string(certPEM),
		CAPEM:          string(s.cfg.CA.CertPEM()),
		TenantScope:    node.TenantScope,
		HostID:         node.HostID,
		ExpiresAt:      cert.NotAfter.UTC().Format(time.RFC3339),
	})
}

// handleRenewActivate completes renewal. Authentication *is* the
// activation: the caller presents the pending certificate, and
// the registry atomically makes it active and retires the old
// one. Presenting the already-active certificate also succeeds,
// so a client retrying after a lost response can converge.
func (s *Server) handleRenewActivate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "enrollment_unavailable"})
		return
	}
	cert := peerCert(r)
	if cert == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "client certificate required"})
		return
	}
	node, err := s.cfg.Registry.ActivateRotation(cert)
	if err != nil {
		writeJSON(w, authErrorStatus(err), map[string]string{"error": "activation_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"activated": node.HostID, "tenantScope": node.TenantScope})
}

var (
	errStorageFull   = errors.New("profile storage limit reached")
	errCommitBinding = errors.New("profile tenantScope/hostId must match the certificate binding")
)

// authErrorStatus maps registry authentication failures to HTTP
// statuses, shared by the fail-fast check and the atomic commit.
func authErrorStatus(err error) int {
	if errors.Is(err, pairing.ErrUnknownNode) || errors.Is(err, pairing.ErrCertBinding) {
		return http.StatusUnauthorized
	}
	return http.StatusForbidden
}

// authenticate enforces the tenant/node binding for profile routes.
// In mTLS mode the peer certificate must resolve to a registered,
// unrevoked node. In plain dev mode (loopback only) intake is
// allowed only when explicitly enabled. This is a fail-fast check;
// the write path re-authorises atomically at commit time (see
// handlePostProfile), because a check here alone races revocation.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (pairing.Node, bool) {
	if s.cfg.CA == nil {
		if s.cfg.AllowPlaintextProfiles {
			return pairing.Node{}, true
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "client certificate required"})
		return pairing.Node{}, false
	}
	cert := peerCert(r)
	if cert == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "client certificate required"})
		return pairing.Node{}, false
	}
	node, err := s.cfg.Registry.Authenticate(cert)
	if err != nil {
		writeJSON(w, authErrorStatus(err), map[string]string{"error": "unauthorized_node", "detail": err.Error()})
		return pairing.Node{}, false
	}
	return node, true
}

func peerCert(r *http.Request) *x509.Certificate {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}
	return r.TLS.PeerCertificates[0]
}

// storeProfile writes one validated profile if capacity allows.
// Callers hold no server lock; it takes s.mu itself.
func (s *Server) storeProfile(profile contracts.HostProfile) error {
	key := profile.TenantScope + "\x00" + profile.HostID
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.profiles[key]; !exists && len(s.profiles) >= MaxStoredProfiles {
		return errStorageFull
	}
	s.profiles[key] = profile
	return nil
}

func (s *Server) snapshotProfiles(tenantScope string, scoped bool) map[string]contracts.HostProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]contracts.HostProfile{}
	for _, profile := range s.profiles {
		if scoped && profile.TenantScope != tenantScope {
			continue
		}
		out[profile.HostID] = profile
	}
	return out
}

// readLimitedBody reads the whole request body under the intake
// size limit. The profile path needs the raw bytes (presence-aware
// decoding), so it cannot stream-decode into a struct.
func (s *Server) readLimitedBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	s.beginBodyRead(w)
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return nil, fmt.Errorf("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxProfileBodyBytes)
	return io.ReadAll(r.Body)
}

func (s *Server) handlePostProfile(w http.ResponseWriter, r *http.Request) {
	// Fail-fast authorisation only. It rejects most bad callers
	// before body parsing, but it is NOT the authorisation that
	// permits the write: revocation could land while the body is
	// in flight, so the store below re-authorises atomically.
	node, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	body, err := s.readLimitedBody(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "detail": err.Error()})
		return
	}
	profile, err := contracts.DecodeHostProfile(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "detail": err.Error()})
		return
	}
	if err := contracts.ValidateHostProfile(profile, s.cfg.Now()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_host_profile", "detail": err.Error()})
		return
	}
	// Authenticated tenant/node binding: a valid certificate may
	// only submit its own node's profile for its own tenant.
	if s.cfg.CA != nil && (profile.TenantScope != node.TenantScope || profile.HostID != node.HostID) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error":  "cross_tenant_or_node_submission",
			"detail": "profile tenantScope/hostId must match the certificate binding",
		})
		return
	}

	if s.cfg.CA == nil {
		// Plain loopback dev mode: no identity to re-check.
		if err := s.storeProfile(profile); err != nil {
			writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"stored": profile.HostID, "tenantScope": profile.TenantScope})
		return
	}

	// Atomic final authorisation + commit. AuthorizeCommit holds
	// the registry lock (the same lock Revoke takes) across the
	// authentication and the store, so there is no check-then-act
	// window: a revocation that returns before this call makes the
	// commit observe ErrRevoked and write nothing; a revocation
	// that arrives during it waits until the store has landed and
	// then takes effect. In-flight writes are therefore never
	// permitted to land after revocation has completed.
	err = s.cfg.Registry.AuthorizeCommit(peerCert(r), func(finalNode pairing.Node) error {
		if profile.TenantScope != finalNode.TenantScope || profile.HostID != finalNode.HostID {
			return errCommitBinding
		}
		return s.storeProfile(profile)
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]string{"stored": profile.HostID, "tenantScope": profile.TenantScope})
	case errors.Is(err, errStorageFull):
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": err.Error()})
	case errors.Is(err, errCommitBinding):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross_tenant_or_node_submission", "detail": err.Error()})
	case errors.Is(err, pairing.ErrRevoked), errors.Is(err, pairing.ErrUnknownNode),
		errors.Is(err, pairing.ErrCertBinding), errors.Is(err, pairing.ErrCertExpired),
		errors.Is(err, pairing.ErrBindingMismatch):
		writeJSON(w, authErrorStatus(err), map[string]string{"error": "unauthorized_node", "detail": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit_failed", "detail": err.Error()})
	}
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		if _, ok := s.authenticate(w, r); !ok {
			return
		}
		writeJSON(w, http.StatusOK, s.snapshotProfiles("", false))
		return
	}
	// Snapshot under the same atomic authorisation as writes, so a
	// revoked node never reads tenant data after revocation lands.
	var out map[string]contracts.HostProfile
	err := s.cfg.Registry.AuthorizeCommit(peerCert(r), func(node pairing.Node) error {
		out = s.snapshotProfiles(node.TenantScope, true)
		return nil
	})
	if err != nil {
		writeJSON(w, authErrorStatus(err), map[string]string{"error": "unauthorized_node", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// decodeStrict decodes exactly one JSON value with a body limit,
// unknown-field rejection and trailing-data rejection.
func (s *Server) decodeStrict(w http.ResponseWriter, r *http.Request, dst any) error {
	s.beginBodyRead(w)
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return fmt.Errorf("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxProfileBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body is required")
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("trailing data after JSON value")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// IsLoopbackAddr reports whether addr binds only loopback. An empty
// host (:8080) binds every interface and is not loopback.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateListenAddr enforces review P1: a non-loopback listener is
// only permitted with mTLS configured. Plain HTTP beyond loopback
// would expose unauthenticated profile read/write to the network.
func ValidateListenAddr(addr string, mtlsEnabled bool) error {
	if IsLoopbackAddr(addr) {
		return nil
	}
	if !mtlsEnabled {
		return fmt.Errorf("refusing to bind %q without mTLS: unauthenticated listeners must stay on loopback (set a loopback -addr or configure TLS)", addr)
	}
	return nil
}
