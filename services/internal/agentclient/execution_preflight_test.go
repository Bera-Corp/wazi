package agentclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestPrepareExecutionChainsPairedTrustResourceAndSingleApproval(t *testing.T) {
	fixture, resources := newResourceBindingFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	windowStart := now.Add(-time.Minute).Format(time.RFC3339)
	windowEnd := now.Add(20 * time.Minute).Format(time.RFC3339)

	hostProfile := parseReleaseObject(t, resources.HostProfileRaw)
	hostProfile["observedAt"] = windowStart
	hostProfile["expiresAt"] = windowEnd
	hostProfile["filesystem"].(map[string]any)["dockerRootWritable"] = false
	hostProfileRaw := canonicalReleaseObject(t, hostProfile)
	hostDigest, err := canonicalResourceDigest(hostProfileRaw, "host profile")
	if err != nil {
		t.Fatal(err)
	}

	budget := parseReleaseObject(t, resources.ContributionBudgetRaw)
	budget["approvedAt"] = windowStart
	budget["validFrom"] = windowStart
	budget["validUntil"] = windowEnd
	budgetRaw := canonicalReleaseObject(t, budget)
	budgetDigest, err := canonicalResourceDigest(budgetRaw, "contribution budget")
	if err != nil {
		t.Fatal(err)
	}

	qualification := parseReleaseObject(t, resources.ManagedVolumeQualificationRaw)
	qualification["observedAt"] = windowStart
	qualification["expiresAt"] = windowEnd
	qualificationRaw := canonicalReleaseObject(t, qualification)
	qualificationDigest, err := canonicalResourceDigest(qualificationRaw, "managed-volume qualification")
	if err != nil {
		t.Fatal(err)
	}

	fixture.envelope = mutateCanonicalObject(t, fixture.envelope, func(envelope map[string]any) {
		envelope["createdAt"] = windowStart
		envelope["expiresAt"] = windowEnd
		envelope["hostProfileRef"] = recordReference("host_demo01", hostDigest, "1.0.0")
		envelope["contributionBudgetRef"] = recordReference("budget_test", budgetDigest, "1.0.0")
		envelope["managedVolumeQualificationRef"] = recordReference("volqual_test", qualificationDigest, "1.0.0")
	})
	fixture.plan = mutateCanonicalObject(t, fixture.plan, func(plan map[string]any) {
		plan["createdAt"] = windowStart
		plan["expiresAt"] = windowEnd
	})
	envelopeDigest, err := verify.CanonicalDigest(parseReleaseObject(t, fixture.envelope))
	if err != nil {
		t.Fatal(err)
	}
	planDigest, err := verify.CanonicalDigest(parseReleaseObject(t, fixture.plan))
	if err != nil {
		t.Fatal(err)
	}

	policyBundle := canonicalReleaseObject(t, parseReleaseObject(t, readReleaseFixture(t, "policy-bundle.json")))
	policyDigest, err := verify.CanonicalDigest(parseReleaseObject(t, policyBundle))
	if err != nil {
		t.Fatal(err)
	}
	approval := parseReleaseObject(t, readReleaseFixture(t, "approval-record.json"))
	approval["approvalId"] = "appr_pilot01"
	approval["caseId"] = "case_demo01"
	approval["planId"] = "plan_demo01"
	approval["planDigest"] = planDigest
	approval["envelopeId"] = "env_demo01"
	approval["envelopeDigest"] = envelopeDigest
	approval["tenantScope"] = "tenant_demo"
	approval["hostId"] = "host_demo01"
	approval["applicationId"] = "org.example.uptime-demo"
	approval["action"] = "public-exposure"
	approval["decision"] = "approved"
	approval["approvedAt"] = windowStart
	approval["expiresAt"] = now.Add(10 * time.Minute).Format(time.RFC3339)
	approval["approver"] = map[string]any{"approverId": "pilot_maintainer", "role": "maintainer"}
	approvalRaw := canonicalReleaseObject(t, approval)

	revocation := signedAgentRevocation(
		t, fixture.signingKey, 1, nil,
		now.Add(-time.Minute).Format(time.RFC3339),
		now.Add(10*time.Minute).Format(time.RFC3339),
	)
	var approvalRequests atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "paired certificate required", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/policy-bundles/pol_default/1.0.0":
			writePreflightRecord(w, policyBundle)
		case "/v1/manifests/org.example.uptime-demo/1.0.0":
			writePreflightRecord(w, fixture.manifest)
		case "/v1/procedures/proc_install_uptime_demo/1.0.0":
			writePreflightRecord(w, fixture.procedure)
		case "/v1/procedures/proc_rollback_uptime_demo/1.0.0":
			writePreflightRecord(w, fixture.rollback)
		case "/v1/revocations/current":
			writePreflightRecord(w, revocation)
		case "/v1/host-profiles":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			value, parseErr := verify.ParseJSON(hostProfileRaw)
			if parseErr != nil {
				http.Error(w, "invalid host profile fixture", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"host_demo01": value})
		case "/v1/approvals/appr_pilot01/consume":
			if r.Method != http.MethodPost {
				http.Error(w, "POST required", http.StatusMethodNotAllowed)
				return
			}
			body, readErr := io.ReadAll(io.LimitReader(r.Body, 16*1024))
			if readErr != nil {
				http.Error(w, "read approval request", http.StatusBadRequest)
				return
			}
			request := map[string]any{}
			if json.Unmarshal(body, &request) != nil ||
				request["jobId"] != "job_preflight01" ||
				request["caseId"] != "case_demo01" ||
				request["planId"] != "plan_demo01" ||
				request["planDigest"] != planDigest ||
				request["envelopeId"] != "env_demo01" ||
				request["envelopeDigest"] != envelopeDigest ||
				request["applicationId"] != "org.example.uptime-demo" ||
				request["action"] != "public-exposure" {
				http.Error(w, "approval binding mismatch", http.StatusForbidden)
				return
			}
			approvalRequests.Add(1)
			writePreflightRecord(w, approvalRaw)
		default:
			http.NotFound(w, r)
		}
	})

	ca, err := pairing.GenerateCA("execution-preflight-test")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, _, err := ca.IssueClientCert("tenant_demo", "host_demo01", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	clientCertificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Pool(),
	}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	clientTLS := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      roots,
		Certificates: []tls.Certificate{clientCertificate},
	}

	state, err := OpenRevocationStateStore(filepath.Join(t.TempDir(), "revocation-state"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	request := ExecutionPreflightRequest{
		JobID: "job_preflight01", CaseID: "case_demo01",
		EnvelopeID: "env_demo01", PlanID: "plan_demo01",
		Release: fixture.expected.Release,
		RollbackProcedure: ProcedureExpectation{
			ProcedureID: "proc_rollback_uptime_demo", Version: "1.0.0",
			ApplicationID: "org.example.uptime-demo", Operation: "rollback",
		},
		EnvelopeRaw: fixture.envelope, PlanRaw: fixture.plan,
		Approvals:                     []ApprovalReference{{ApprovalID: "appr_pilot01", Action: "public-exposure"}},
		ContributionBudgetRaw:         budgetRaw,
		ManagedVolumeQualificationRaw: qualificationRaw,
	}
	prepared, err := PrepareExecution(context.Background(), ExecutionPreflightConfig{
		ControlPlaneURL: server.URL,
		TLSConfig:       clientTLS,
		SigningPolicy:   fixture.policy,
		CapabilityPolicy: CapabilityPolicyReference{
			PolicyID: "pol_default", Version: "1.0.0", Digest: policyDigest, TenantScope: "tenant_demo",
		},
		RevocationState:     state,
		RevocationFreshness: RevocationFreshness{MaxAge: 5 * time.Minute},
		ClockSkew:           5 * time.Second,
		ApprovalMaxValidity: 15 * time.Minute,
	}, request, resources)
	if err != nil {
		t.Fatalf("valid paired execution preflight rejected: %v", err)
	}
	if !prepared.Verified() {
		t.Fatal("successful preflight returned an unverified prepared execution")
	}
	if prepared.TenantScope() != "tenant_demo" || prepared.HostID() != "host_demo01" {
		t.Fatalf("prepared node identity = %s/%s", prepared.TenantScope(), prepared.HostID())
	}
	expectedManifest, err := verify.ParseJSON(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}
	expectedManifestDigest, err := verify.CanonicalDigest(expectedManifest)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Binding().Release.ManifestDigest != expectedManifestDigest {
		t.Fatal("prepared execution did not retain the verified manifest digest")
	}
	if got := prepared.ApprovalRecords(); len(got) != 1 || !bytes.Equal(got[0], approvalRaw) {
		t.Fatalf("prepared approvals = %d records; want the exact consumed approval", len(got))
	}
	copyOfProfile := prepared.HostProfile()
	copyOfProfile[0] ^= 0xff
	if bytes.Equal(copyOfProfile, prepared.HostProfile()) {
		t.Fatal("host profile accessor did not return a defensive copy")
	}
	if approvalRequests.Load() != 1 {
		t.Fatalf("approval consume requests = %d, want 1", approvalRequests.Load())
	}
	if state.Cursor().Sequence != 1 {
		t.Fatalf("persisted revocation cursor = %+v, want sequence 1", state.Cursor())
	}
}

func writePreflightRecord(w http.ResponseWriter, raw []byte) {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		http.Error(w, "invalid record fixture", http.StatusInternalServerError)
		return
	}
	digest, err := verify.CanonicalDigest(value)
	if err != nil {
		http.Error(w, "invalid record digest", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", digest))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
