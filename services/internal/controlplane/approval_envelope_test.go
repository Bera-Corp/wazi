package controlplane_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func TestApprovalIssueAcceptsResourceBoundV2AndRejectsMissingReferences(t *testing.T) {
	f, _ := newApprovalMTLSFixture(t)
	cert, key := f.enrollViaHTTP(t, "tenant_demo", "host_demo01")
	client := f.certClient(t, cert, key, "localhost")
	for _, invalid := range []bool{false, true} {
		envelope := parseContextObject(t, canonicalContextExample(t, "execution-context-envelope.json"))
		envelope["schemaVersion"] = "2.0.0"
		ref := map[string]any{"recordId": "budget_pilot", "digest": "sha256:" + strings.Repeat("a", 64), "version": "1.0.0"}
		envelope["contributionBudgetRef"] = ref
		envelope["managedVolumeQualificationRef"] = ref
		if invalid {
			delete(envelope, "managedVolumeQualificationRef")
		}
		raw, err := verify.CanonicalJSON(envelope)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(map[string]any{
			"caseId": "case_v2", "plan": json.RawMessage(canonicalContextExample(t, "deployment-plan.json")),
			"envelope": json.RawMessage(raw), "hostId": "host_demo01",
			"action": "public-exposure", "decision": "approved", "validForSeconds": 600,
		})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, f.baseURL+"/v1/approvals", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Wazi-Operator-Token", testApprovalOperatorAuth())
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		result, _ := io.ReadAll(response.Body)
		response.Body.Close()
		want := http.StatusCreated
		if invalid {
			want = http.StatusBadRequest
		}
		if response.StatusCode != want {
			t.Fatalf("invalid=%v status=%d want=%d body=%s", invalid, response.StatusCode, want, result)
		}
	}
}
