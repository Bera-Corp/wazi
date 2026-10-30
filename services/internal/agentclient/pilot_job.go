package agentclient

import "encoding/json"

// PilotJob is an inspectable, digest-bound request. It carries no signing key,
// server anchor, operator credential, runtime endpoint, or executable command.
type PilotJob struct {
	JobID             string               `json:"jobId"`
	CaseID            string               `json:"caseId"`
	Release           ReleaseExpectation   `json:"release"`
	RollbackProcedure ProcedureExpectation `json:"rollbackProcedure"`
	Envelope          json.RawMessage      `json:"envelope"`
	Plan              json.RawMessage      `json:"plan"`
	Artifact          json.RawMessage      `json:"artifact"`
	Budget            json.RawMessage      `json:"budget"`
	Qualification     json.RawMessage      `json:"qualification"`
	Approvals         []ApprovalReference  `json:"approvals"`
}
