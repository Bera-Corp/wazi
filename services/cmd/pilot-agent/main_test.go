package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/operationjournal"
)

func TestReconcileHandlerBlocksAmbiguousOperation(t *testing.T) {
	journal, err := operationjournal.New(filepath.Join(t.TempDir(), "journal"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	digest := "sha256:" + strings.Repeat("a", 64)
	identity := operationjournal.Identity{
		OperationID: "op_test_1", JobID: "job_test_1", CaseID: "case_test_1",
		PlanID: "plan_test_1", PlanDigest: digest, EnvelopeID: "env_test_1",
		EnvelopeDigest: digest, TenantScope: "tenant_test", HostID: "host_test",
		ApplicationID: "org.gitea.gitea", ManifestDigest: digest, Operation: "install",
	}
	if _, _, err := journal.Begin(identity); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Start(identity.OperationID); err != nil {
		t.Fatal(err)
	}
	params := "sha256:" + strings.Repeat("b", 64)
	if _, err := journal.RecordIntent(identity.OperationID, "step_1_compose_up", "compose.up", "compose-project:wazi-gitea-phase1", params); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.RecordOutcome(identity.OperationID, "step_1_compose_up", "ambiguous", "runtime.ambiguous"); err != nil {
		t.Fatal(err)
	}
	handler := reconcileHandler(journal)
	body := `{"operationId":"op_test_1","stepId":"step_1_compose_up","observedState":"absent","snapshotDigest":"` + digest + `"}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reconcile", strings.NewReader(body)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"blocked"`) {
		t.Fatalf("reconcile response = %d %s", response.Code, response.Body.String())
	}
	// A blocked operation can never be started again.
	if _, err := journal.Start(identity.OperationID); err == nil {
		t.Fatal("blocked operation restarted")
	}
	// Malformed requests are rejected without touching the journal.
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reconcile", strings.NewReader(`{"operationId":1}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed request status = %d", response.Code)
	}
}
