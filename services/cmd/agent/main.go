// Command agent is the Wazi host agent (Phase 1).
//
// Current capability: live inventory -> host-profile record, with
// optional mTLS pairing and submission to the control plane:
//
//	wazi-agent -controlplane https://cp.example:8443 \
//	  -pairing-code <one-time-code> -tenant tenant_demo -ca server-ca.crt
//
// One node identity is resolved per run (explicit -host-id, the
// existing certificate's binding, or the hostname) and is used for
// enrollment, inventory and submission alike. The node private key
// is generated on this host during enrollment and never leaves it;
// only a CSR is sent. -ca is the operator-supplied server trust
// anchor and is never overwritten by a response. Without
// -controlplane the agent prints the profile to stdout. This
// binary performs no host mutation and no deployment. Agent-side
// pre-mutation signature, revocation, approval, runtime-boundary,
// and lifecycle checks are still required.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/probe"
)

func main() {
	tenant := flag.String("tenant", "tenant_demo", "tenant scope for the host profile")
	controlplane := flag.String("controlplane", "", "control plane base URL (https://...); empty = print profile only")
	pairingCode := flag.String("pairing-code", "", "one-time pairing code (enrolls on first run)")
	hostIDFlag := flag.String("host-id", "", "node hostId (default: enrolled certificate binding, else hostname-derived)")
	renew := flag.Bool("renew", false, "rotate the node certificate/key before submission")
	stateDir := flag.String("state-dir", ".wazi-agent", "directory for node identity files")
	certFile := flag.String("cert", "", "node certificate PEM (default: <state-dir>/node.crt)")
	keyFile := flag.String("key", "", "node private key PEM (default: <state-dir>/node.key)")
	caFile := flag.String("ca", "", "server trust anchor PEM, operator-supplied (default: <state-dir>/ca.crt)")
	issuerFile := flag.String("issuer-ca", "", "where the client issuing CA is persisted (default: <state-dir>/issuer.crt)")
	serverName := flag.String("server-name", "", "expected control-plane TLS server name (default: URL host)")
	flag.Parse()

	if *certFile == "" {
		*certFile = filepath.Join(*stateDir, "node.crt")
	}
	if *keyFile == "" {
		*keyFile = filepath.Join(*stateDir, "node.key")
	}
	if *caFile == "" {
		*caFile = filepath.Join(*stateDir, "ca.crt")
	}
	if *issuerFile == "" {
		*issuerFile = filepath.Join(*stateDir, "issuer.crt")
	}
	identity := agentclient.Identity{CertFile: *certFile, KeyFile: *keyFile, CAFile: *caFile, IssuerCAFile: *issuerFile}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Resolve the one identity for this run before any use of it.
	hostID, err := agentclient.ResolveHostID(*hostIDFlag, *tenant, identity)
	if err != nil {
		fatal("resolve node identity:", err)
	}

	if *controlplane != "" && *pairingCode != "" {
		name := *serverName
		if name == "" {
			name = urlHost(*controlplane)
		}
		trust, bootstrapPEM, err := agentclient.LoadBootstrap(*caFile, name)
		if err != nil {
			fatal("enrollment trust:", err)
		}
		result, err := agentclient.Enroll(ctx, *controlplane, trust, bootstrapPEM, *pairingCode, *tenant, hostID, identity)
		if err != nil {
			fatal("pairing enrollment failed:", err)
		}
		fmt.Fprintf(os.Stderr, "enrolled tenant=%s host=%s certExpires=%s\n", result.TenantScope, result.HostID, result.ExpiresAt)
		// The persisted certificate is now the identity source.
		hostID, err = agentclient.ResolveHostID("", *tenant, identity)
		if err != nil {
			fatal("verify enrolled identity:", err)
		}
	}

	profile, err := probe.BuildForHost(*tenant, hostID)
	if err != nil {
		fatal("probe failed:", err)
	}

	if *controlplane == "" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(profile); err != nil {
			fatal("encode failed:", err)
		}
		return
	}

	// Certificate-derived identity is validated before use.
	certHost, err := agentclient.ResolveHostID("", *tenant, identity)
	if err != nil {
		fatal("validate certificate identity (enroll first with -pairing-code):", err)
	}
	if certHost != hostID {
		fatal(fmt.Sprintf("certificate identity %q disagrees with resolved identity %q", certHost, hostID), nil)
	}

	name := *serverName
	if name == "" {
		name = urlHost(*controlplane)
	}
	tlsConfig, err := agentclient.LoadTLS(identity, name)
	if err != nil {
		fatal("load identity (enroll first with -pairing-code):", err)
	}
	// Complete any renewal whose activation was lost earlier
	// before trying to use the (possibly still pending) identity.
	if identity.HasPendingRenewal() {
		activated, err := agentclient.ActivatePending(ctx, *controlplane, tlsConfig, identity)
		if err != nil {
			fatal("complete pending renewal activation:", err)
		}
		if activated {
			fmt.Fprintln(os.Stderr, "completed pending renewal activation")
			tlsConfig, err = agentclient.LoadTLS(identity, name)
			if err != nil {
				fatal("load activated identity:", err)
			}
		}
	}
	if *renew {
		result, err := agentclient.Renew(ctx, *controlplane, tlsConfig, *tenant, hostID, identity)
		if err != nil {
			fatal("certificate renewal failed:", err)
		}
		fmt.Fprintf(os.Stderr, "renewed tenant=%s host=%s certExpires=%s\n", result.TenantScope, result.HostID, result.ExpiresAt)
		tlsConfig, err = agentclient.LoadTLS(identity, name)
		if err != nil {
			fatal("load renewed identity:", err)
		}
	}
	if err := agentclient.SubmitProfile(ctx, *controlplane, tlsConfig, profile); err != nil {
		fatal(err.Error(), nil)
	}
	fmt.Fprintf(os.Stderr, "submitted host profile %s for tenant %s\n", profile.HostID, profile.TenantScope)
}

func urlHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return parsed.Hostname()
}

func fatal(prefix string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, prefix, err)
	} else {
		fmt.Fprintln(os.Stderr, prefix)
	}
	os.Exit(1)
}
