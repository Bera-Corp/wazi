package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type ApprovalRecord struct {
	SchemaVersion  string   `json:"schemaVersion"`
	ApprovalID     string   `json:"approvalId"`
	CaseID         string   `json:"caseId"`
	PlanID         string   `json:"planId"`
	PlanDigest     string   `json:"planDigest"`
	EnvelopeID     string   `json:"envelopeId"`
	EnvelopeDigest string   `json:"envelopeDigest"`
	TenantScope    string   `json:"tenantScope"`
	HostID         string   `json:"hostId"`
	ApplicationID  string   `json:"applicationId"`
	Action         string   `json:"action"`
	Decision       string   `json:"decision"`
	Approver       Approver `json:"approver"`
	ApprovedAt     string   `json:"approvedAt"`
	ExpiresAt      string   `json:"expiresAt"`
}

type Approver struct {
	ID   string `json:"approverId"`
	Role string `json:"role"`
}

// DecodeApprovalRecord applies the canonical schema before decoding into
// typed fields. Approval authority and lifecycle state are checked elsewhere.
func DecodeApprovalRecord(raw []byte) (ApprovalRecord, error) {
	var record ApprovalRecord
	if err := ValidateApprovalRecord(raw); err != nil {
		return record, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return ApprovalRecord{}, fmt.Errorf("decode approval record: %w", err)
	}
	return record, nil
}

func ApprovalRecordDigest(raw []byte) (string, error) {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return "", err
	}
	return verify.CanonicalDigest(value)
}
