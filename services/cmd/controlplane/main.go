// Command controlplane is the Wazi control plane (Phase 1).
//
// Capability: mTLS node pairing (one-time codes, CSR issuance,
// revocation), authenticated, tenant-bound host-profile intake, and
// signed manifest/procedure ingest verified against operator policy;
// deployment stays disabled until approval, revocation, runtime,
// and lifecycle gates pass.
//
// Listener safety (review P1): the default bind is loopback. A
// non-loopback bind is refused unless mTLS is configured, so the
// unauthenticated skeleton can never be exposed by accident.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/approvalregistry"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/controlplane"
	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

// Return through run so registry cleanup also runs on startup/serve errors.
// log.Fatal inside this scope would skip deferred lock release.
func run(ctx context.Context) error {
	addr := flag.String("addr", envOr("WAZI_CONTROLPLANE_ADDR", "127.0.0.1:8080"), "listen address (loopback by default)")
	tlsCertFile := flag.String("tls-cert", os.Getenv("WAZI_TLS_CERT"), "server certificate PEM (enables mTLS)")
	tlsKeyFile := flag.String("tls-key", os.Getenv("WAZI_TLS_KEY"), "server private key PEM")
	caCertFile := flag.String("ca-cert", os.Getenv("WAZI_CA_CERT"), "pairing CA certificate PEM")
	caKeyFile := flag.String("ca-key", os.Getenv("WAZI_CA_KEY"), "pairing CA private key PEM (secret; never commit)")
	allowPlain := flag.Bool("dev-allow-plaintext-profiles", false, "allow unauthenticated profile intake (loopback listeners only)")
	registryFile := flag.String("registry-file", os.Getenv("WAZI_REGISTRY_FILE"), "durable pairing registry file (recommended with a file-based CA)")
	signingPolicyFile := flag.String("signing-policy-file", os.Getenv("WAZI_SIGNING_POLICY_FILE"), "versioned operator-managed signer policy JSON")
	capabilityPolicyFile := flag.String("capability-policy-file", os.Getenv("WAZI_CAPABILITY_POLICY_FILE"), "operator-managed tenant capability policy bundle JSON")
	manifestRegistryDir := flag.String("manifest-registry-dir", os.Getenv("WAZI_MANIFEST_REGISTRY_DIR"), "private directory for verified signed manifests")
	approvalRegistryDir := flag.String("approval-registry-dir", os.Getenv("WAZI_APPROVAL_REGISTRY_DIR"), "private directory for issued and consumed approvals")
	approverCredentialsFile := flag.String("approver-credentials-file", os.Getenv("WAZI_APPROVER_CREDENTIALS_FILE"), "private file containing distinct approver credential hashes")
	approverID := flag.String("approver-id", os.Getenv("WAZI_APPROVER_ID"), "configured local operator identity recorded on approvals")
	approverRole := flag.String("approver-role", os.Getenv("WAZI_APPROVER_ROLE"), "configured approver role: user or maintainer")
	flag.Parse()

	mtlsEnabled := *tlsCertFile != "" && *tlsKeyFile != ""
	if *capabilityPolicyFile != "" && !mtlsEnabled {
		return fmt.Errorf("capability policy serving requires a TLS listener with paired-node client authentication")
	}
	var capabilityPolicyBundle []byte
	if *capabilityPolicyFile != "" {
		info, err := os.Lstat(*capabilityPolicyFile)
		if err != nil {
			return fmt.Errorf("inspect capability policy bundle: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() <= 0 || info.Size() > 256*1024 {
			return fmt.Errorf("capability policy bundle must be a regular, non-group/world-writable file no larger than 256 KiB")
		}
		capabilityPolicyBundle, err = os.ReadFile(*capabilityPolicyFile)
		if err != nil {
			return fmt.Errorf("read capability policy bundle: %w", err)
		}
		if err := contracts.ValidatePolicyBundle(capabilityPolicyBundle); err != nil {
			return fmt.Errorf("validate capability policy bundle: %w", err)
		}
	}
	if err := controlplane.ValidateListenAddr(*addr, mtlsEnabled); err != nil {
		return err
	}
	if *allowPlain && !controlplane.IsLoopbackAddr(*addr) {
		return fmt.Errorf("-dev-allow-plaintext-profiles is only allowed on a loopback listener")
	}
	if (*signingPolicyFile == "") != (*manifestRegistryDir == "") {
		return fmt.Errorf("-signing-policy-file and -manifest-registry-dir must be configured together")
	}
	legacyApproverConfigured := *approverID != "" || *approverRole != ""
	if *approverCredentialsFile != "" && legacyApproverConfigured {
		return fmt.Errorf("use either -approver-credentials-file or the legacy single-approver flags")
	}
	if *approvalRegistryDir == "" && (*approverCredentialsFile != "" || legacyApproverConfigured) {
		return fmt.Errorf("approver credentials require -approval-registry-dir")
	}
	if *approvalRegistryDir != "" && *approverCredentialsFile == "" && (*approverID == "" || *approverRole == "") {
		return fmt.Errorf("approval registry requires -approver-credentials-file or both legacy single-approver flags")
	}
	if *approvalRegistryDir != "" && os.Getenv("WAZI_OPERATOR_TOKEN") == "" {
		return fmt.Errorf("WAZI_OPERATOR_TOKEN is required when the approval registry is enabled")
	}
	if *approvalRegistryDir != "" && !mtlsEnabled {
		return fmt.Errorf("approval registry requires a TLS listener with paired-node client authentication")
	}
	if *approverRole != "" && *approverRole != "user" && *approverRole != "maintainer" {
		return fmt.Errorf("-approver-role must be user or maintainer")
	}

	cfg := controlplane.Config{
		AllowPlaintextProfiles: *allowPlain,
		CapabilityPolicyBundle: capabilityPolicyBundle,
		// Operator token comes from the environment only: it must
		// never appear in flags, process listings or shell history.
		OperatorToken: os.Getenv("WAZI_OPERATOR_TOKEN"),
		ApproverID:    *approverID,
		ApproverRole:  *approverRole,
	}
	if *approverCredentialsFile != "" {
		credentials, err := controlplane.LoadApproverCredentials(*approverCredentialsFile)
		if err != nil {
			return fmt.Errorf("load approver credentials: %w", err)
		}
		cfg.ApproverCredentials = credentials
	}
	if *registryFile != "" {
		registry, err := pairing.NewRegistryWithStore(nil, *registryFile)
		if err != nil {
			return fmt.Errorf("load pairing registry: %w", err)
		}
		cfg.Registry = registry
		log.Printf("pairing registry persisted at %s (codes stored hashed)", *registryFile)
	}
	defer func() {
		if cfg.ManifestRegistry != nil {
			if err := cfg.ManifestRegistry.Close(); err != nil {
				log.Printf("close manifest registry: %v", err)
			}
		}
		if cfg.ApprovalRegistry != nil {
			if err := cfg.ApprovalRegistry.Close(); err != nil {
				log.Printf("close approval registry: %v", err)
			}
		}
		if cfg.Registry != nil {
			if err := cfg.Registry.Close(); err != nil {
				log.Printf("close pairing registry: %v", err)
			}
		}
	}()
	if *signingPolicyFile != "" {
		if os.Getenv("WAZI_OPERATOR_TOKEN") == "" {
			return fmt.Errorf("WAZI_OPERATOR_TOKEN is required when the signed manifest registry is enabled")
		}
		policy, err := verify.LoadPolicyFile(*signingPolicyFile)
		if err != nil {
			return fmt.Errorf("load signing policy: %w", err)
		}
		manifestStore, err := manifestregistry.New(*manifestRegistryDir, policy)
		if err != nil {
			return fmt.Errorf("open verified manifest registry: %w", err)
		}
		cfg.ManifestRegistry = manifestStore
		log.Printf("verified manifest registry loaded at %s (%d trusted signer(s))", *manifestRegistryDir, len(policy.TrustedSigners))
	}
	if *approvalRegistryDir != "" {
		approvals, err := approvalregistry.New(*approvalRegistryDir, nil)
		if err != nil {
			return fmt.Errorf("open approval registry: %w", err)
		}
		cfg.ApprovalRegistry = approvals
		if len(cfg.ApproverCredentials) > 0 {
			log.Printf("approval registry loaded at %s for %d configured approver principals", *approvalRegistryDir, len(cfg.ApproverCredentials))
		} else {
			log.Printf("approval registry loaded at %s for legacy configured approver %s (%s role)", *approvalRegistryDir, *approverID, *approverRole)
		}
	}
	var serverCert tls.Certificate
	if mtlsEnabled {
		cert, err := tls.LoadX509KeyPair(*tlsCertFile, *tlsKeyFile)
		if err != nil {
			return fmt.Errorf("load server key pair: %w", err)
		}
		serverCert = cert
		ca, err := loadCA(*caCertFile, *caKeyFile)
		if err != nil {
			return fmt.Errorf("load pairing CA: %w", err)
		}
		cfg.CA = ca
	} else if *caCertFile != "" || *caKeyFile != "" {
		return fmt.Errorf("-ca-cert/-ca-key require -tls-cert/-tls-key (mTLS)")
	}

	server := controlplane.New(cfg)
	handler := server.Handler()
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	if mtlsEnabled {
		httpServer.TLSConfig = server.TLSConfig(serverCert)
		log.Printf("wazi control plane listening on %s (mTLS, deployment disabled)", *addr)
	} else {
		log.Printf("wazi control plane listening on %s (plain HTTP, loopback dev mode, deployment disabled)", *addr)
	}
	return serve(ctx, httpServer, mtlsEnabled)
}

func serve(ctx context.Context, server *http.Server, useTLS bool) error {
	done := make(chan error, 1)
	go func() {
		if useTLS {
			done <- server.ListenAndServeTLS("", "")
		} else {
			done <- server.ListenAndServe()
		}
	}()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		<-done
		return nil
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// loadCA loads the pairing CA from files, or generates an ephemeral
// dev CA (with a loud warning) when no files are configured. An
// ephemeral CA invalidates all enrollments on restart: nodes must
// re-pair. That fail-closed behaviour is intentional for dev.
func loadCA(certFile, keyFile string) (*pairing.CA, error) {
	if certFile == "" && keyFile == "" {
		log.Print("WARNING: no -ca-cert/-ca-key configured; using an ephemeral dev CA. Enrollments will not survive restart.")
		return pairing.GenerateCA("wazi-dev-ca")
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	return pairing.LoadCA(certPEM, keyPEM)
}
