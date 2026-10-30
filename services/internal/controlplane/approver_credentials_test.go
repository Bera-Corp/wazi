package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestLoadApproverCredentialsAcceptsConfiguredPrincipals(t *testing.T) {
	credA := "approval-" + strings.Repeat("a", 32)
	credB := "approval-" + strings.Repeat("b", 32)
	record := map[string]any{
		"schemaVersion": "1.0.0",
		"approvers": []any{
			map[string]any{
				"approverId":       "maintainer_one",
				"role":             "maintainer",
				"credentialSha256": approvalDigestForTest(credA),
			},
			map[string]any{
				"approverId":       "maintainer_two",
				"role":             "maintainer",
				"credentialSha256": approvalDigestForTest(credB),
			},
		},
	}
	raw, err := verify.CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "approvers.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	credentials, err := LoadApproverCredentials(path)
	if err != nil {
		t.Fatalf("load valid approver credential file: %v", err)
	}
	if len(credentials) != 2 || credentials[0].ID != "maintainer_one" || credentials[1].ID != "maintainer_two" {
		t.Fatalf("loaded unexpected approver identities: %+v", credentials)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadApproverCredentials(path); err == nil {
			t.Fatal("group-readable approver credential file was accepted")
		}
	}
}

func TestLoadApproverCredentialsAcceptsSingleApproverPilot(t *testing.T) {
	credential := "approval-" + strings.Repeat("p", 32)
	value := map[string]any{
		"schemaVersion": "1.0.0",
		"approvers": []any{map[string]any{
			"approverId": "pilot_maintainer", "role": "maintainer",
			"credentialSha256": approvalDigestForTest(credential),
		}},
	}
	raw, err := verify.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "single-approver.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	credentials, err := LoadApproverCredentials(path)
	if err != nil {
		t.Fatalf("single configured pilot approver rejected: %v", err)
	}
	if len(credentials) != 1 || credentials[0].ID != "pilot_maintainer" {
		t.Fatalf("loaded unexpected pilot approvers: %+v", credentials)
	}
}

func TestApproverCredentialsRejectDuplicateIdentityOrCredential(t *testing.T) {
	credA := "approval-" + strings.Repeat("c", 32)
	digest := approvalDigestForTest(credA)
	valid := []ApproverCredential{
		{ID: "maintainer_one", Role: "maintainer", CredentialSHA256: digest},
		{ID: "maintainer_two", Role: "maintainer", CredentialSHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	duplicateID := append([]ApproverCredential(nil), valid...)
	duplicateID[1].ID = duplicateID[0].ID
	if err := ValidateApproverCredentials(duplicateID); err == nil {
		t.Fatal("duplicate approver identity was accepted")
	}
	duplicateCredential := append([]ApproverCredential(nil), valid...)
	duplicateCredential[1].CredentialSHA256 = duplicateCredential[0].CredentialSHA256
	if err := ValidateApproverCredentials(duplicateCredential); err == nil {
		t.Fatal("duplicate approver credential was accepted")
	}
}

func approvalDigestForTest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}
