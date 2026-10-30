package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestFetchVerifiedProcedureRecordChecksSignatureAndIdentity(t *testing.T) {
	policy, key := newReleaseTestSigner(t)
	procedure := signReleaseFixture(t, readReleaseFixture(t, "procedure-install.json"), key)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/procedures/proc_install_uptime_demo/1.0.0" {
			http.NotFound(w, r)
			return
		}
		value, err := verify.ParseJSON(procedure)
		if err != nil {
			http.Error(w, "bad fixture", http.StatusInternalServerError)
			return
		}
		digest, err := verify.CanonicalDigest(value)
		if err != nil {
			http.Error(w, "bad fixture digest", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("ETag", fmt.Sprintf("\"%s\"", digest))
		_, _ = w.Write(procedure)
	}))
	defer server.Close()
	tlsConfig := pairedTestTLSConfig(t, server)
	expected := ProcedureExpectation{
		ProcedureID: "proc_install_uptime_demo", Version: "1.0.0",
		ApplicationID: "org.example.uptime-demo", Operation: "install",
	}
	got, err := FetchVerifiedProcedureRecord(context.Background(), server.URL, tlsConfig, policy, expected)
	if err != nil {
		t.Fatalf("valid procedure rejected: %v", err)
	}
	if !bytes.Equal(got, procedure) {
		t.Fatal("procedure accessor returned bytes other than those verified")
	}
	expected.Operation = "upgrade"
	if _, err := FetchVerifiedProcedureRecord(context.Background(), server.URL, tlsConfig, policy, expected); err == nil {
		t.Fatal("procedure with the wrong operation identity was accepted")
	}
}

func TestFetchHostProfileReturnsCanonicalAuthenticatedHostRecord(t *testing.T) {
	profileValue, err := verify.ParseJSON(readReleaseFixture(t, "host-profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := contracts.DecodeHostProfile(readReleaseFixture(t, "host-profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := json.Marshal(map[string]any{profile.HostID: profileValue})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/host-profiles" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(profiles)
	}))
	defer server.Close()
	tlsConfig := pairedTestTLSConfig(t, server)

	got, err := FetchHostProfile(context.Background(), server.URL, tlsConfig, profile.TenantScope, profile.HostID)
	if err != nil {
		t.Fatalf("valid host profile rejected: %v", err)
	}
	want, err := verify.CanonicalJSON(profileValue)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("host profile was not returned in canonical form")
	}
	if _, err := FetchHostProfile(context.Background(), server.URL, tlsConfig, "tenant_other", profile.HostID); err == nil {
		t.Fatal("cross-tenant host profile was accepted")
	}
}
