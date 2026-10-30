package agentclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const maxApprovalRecordBytes = 64 << 10

var (
	approvalIDPattern  = regexp.MustCompile("^appr_[A-Za-z0-9_-]+$")
	approvalJobPattern = regexp.MustCompile("^job_[A-Za-z0-9_-]+$")
)

type ApprovalReference struct {
	ApprovalID string
	Action     string
}

type consumeApprovalRequest struct {
	JobID          string `json:"jobId"`
	CaseID         string `json:"caseId"`
	PlanID         string `json:"planId"`
	PlanDigest     string `json:"planDigest"`
	EnvelopeID     string `json:"envelopeId"`
	EnvelopeDigest string `json:"envelopeDigest"`
	ApplicationID  string `json:"applicationId"`
	Action         string `json:"action"`
}

type ApprovalExpectation struct {
	CaseID          string
	PlanID          string
	PlanDigest      string
	EnvelopeID      string
	EnvelopeDigest  string
	TenantScope     string
	HostID          string
	ApplicationID   string
	RequiredActions []string
	Requirements    map[string]string
	Now             time.Time
	ClockSkew       time.Duration
	MaxValidity     time.Duration
}

// VerifyApprovalRecords checks exact plan, envelope, case, tenant, host,
// application, action, principal-role, quorum, and live-time bindings. The
// records must have arrived over an authenticated control-plane channel; the
// approval schema itself is not a digital signature.
func VerifyApprovalRecords(rawRecords [][]byte, expected ApprovalExpectation) error {
	if err := validateApprovalExpectation(expected); err != nil {
		return err
	}
	if len(expected.RequiredActions) == 0 {
		if len(rawRecords) != 0 {
			return fmt.Errorf("approval records were supplied for a plan with no required actions")
		}
		return nil
	}
	required := make(map[string]struct{}, len(expected.RequiredActions))
	for _, action := range expected.RequiredActions {
		if action == "" {
			return fmt.Errorf("required approval action is empty")
		}
		if _, duplicate := required[action]; duplicate {
			return fmt.Errorf("required approval action %q is repeated", action)
		}
		required[action] = struct{}{}
		if _, ok := expected.Requirements[action]; !ok {
			return fmt.Errorf("approval policy has no rule for required action %q", action)
		}
	}
	seenRecords := make(map[string]struct{}, len(rawRecords))
	approvers := make(map[string]map[string]string, len(required))
	for _, raw := range rawRecords {
		if len(raw) == 0 || len(raw) > maxApprovalRecordBytes {
			return fmt.Errorf("approval record size must be between 1 and %d bytes", maxApprovalRecordBytes)
		}
		if err := contracts.ValidateApprovalRecord(raw); err != nil {
			return fmt.Errorf("approval schema rejected: %w", err)
		}
		if _, err := parseCanonicalObject(raw, "approval record"); err != nil {
			return err
		}
		record, err := contracts.DecodeApprovalRecord(raw)
		if err != nil {
			return fmt.Errorf("decode approval record: %w", err)
		}
		if _, duplicate := seenRecords[record.ApprovalID]; duplicate {
			return fmt.Errorf("approval record %q is duplicated", record.ApprovalID)
		}
		seenRecords[record.ApprovalID] = struct{}{}
		if record.CaseID != expected.CaseID || record.PlanID != expected.PlanID ||
			record.PlanDigest != expected.PlanDigest || record.EnvelopeID != expected.EnvelopeID ||
			record.EnvelopeDigest != expected.EnvelopeDigest || record.TenantScope != expected.TenantScope ||
			record.HostID != expected.HostID || record.ApplicationID != expected.ApplicationID {
			return fmt.Errorf("approval record does not match the exact case, plan, envelope, tenant, host, and application")
		}
		if _, ok := required[record.Action]; !ok {
			return fmt.Errorf("approval record action %q is not required by this plan", record.Action)
		}
		if record.Decision != "approved" {
			return fmt.Errorf("approval action %q was denied", record.Action)
		}
		if err := validateApprovalTimeWindow(record, expected); err != nil {
			return err
		}
		rule := expected.Requirements[record.Action]
		if rule == "none" {
			return fmt.Errorf("approval record supplied for action %q whose policy requires none", record.Action)
		}
		if rule != "user" && rule != "maintainer" && rule != "two-maintainers" {
			return fmt.Errorf("approval policy rule %q for action %q is invalid", rule, record.Action)
		}
		byApprover := approvers[record.Action]
		if byApprover == nil {
			byApprover = make(map[string]string)
			approvers[record.Action] = byApprover
		}
		if _, duplicate := byApprover[record.Approver.ID]; duplicate {
			return fmt.Errorf("approver %q supplied more than one record for action %q", record.Approver.ID, record.Action)
		}
		byApprover[record.Approver.ID] = record.Approver.Role
	}
	for action := range required {
		rule := expected.Requirements[action]
		count := 0
		maintainers := 0
		for _, role := range approvers[action] {
			count++
			if role == "maintainer" {
				maintainers++
			}
		}
		switch rule {
		case "none":
			if count != 0 {
				return fmt.Errorf("action %q does not permit approval records under its policy", action)
			}
		case "user":
			if count < 1 {
				return fmt.Errorf("action %q requires one live user approval", action)
			}
		case "maintainer":
			if maintainers < 1 {
				return fmt.Errorf("action %q requires one live maintainer approval", action)
			}
		case "two-maintainers":
			if maintainers < 2 {
				return fmt.Errorf("action %q requires two distinct live maintainer approvals", action)
			}
		default:
			return fmt.Errorf("approval policy rule %q for action %q is invalid", rule, action)
		}
	}
	return nil
}

// ConsumeAndVerifyApprovalRecords consumes each requested approval over the
// paired TLS channel, checks canonical response digests, then applies the
// exact case bindings and caller-supplied distinct-principal quorum. A
// partial network failure is retryable with the same job ID because server
// consumption is idempotent for that job.
func ConsumeAndVerifyApprovalRecords(
	ctx context.Context,
	baseURL string,
	tlsConfig *tls.Config,
	jobID string,
	references []ApprovalReference,
	expected ApprovalExpectation,
) ([][]byte, error) {
	if err := requirePairedTLS(tlsConfig, "approval consumption"); err != nil {
		return nil, err
	}
	if !approvalJobPattern.MatchString(jobID) {
		return nil, fmt.Errorf("approval consumption requires a valid job ID")
	}
	if err := validateApprovalExpectation(expected); err != nil {
		return nil, err
	}
	if len(references) > 32 {
		return nil, fmt.Errorf("approval reference count exceeds the limit")
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	client := newSecureClient(tlsConfig)
	seenIDs := make(map[string]struct{}, len(references))
	rawRecords := make([][]byte, 0, len(references))
	for _, reference := range references {
		if !approvalIDPattern.MatchString(reference.ApprovalID) || reference.Action == "" {
			return nil, fmt.Errorf("approval reference identity or action is invalid")
		}
		if _, exists := seenIDs[reference.ApprovalID]; exists {
			return nil, fmt.Errorf("approval reference ID is duplicated")
		}
		seenIDs[reference.ApprovalID] = struct{}{}
		if !containsApprovalAction(expected.RequiredActions, reference.Action) {
			return nil, fmt.Errorf("approval reference action is not required by the authenticated policy")
		}
		payload, err := json.Marshal(consumeApprovalRequest{
			JobID: jobID, CaseID: expected.CaseID, PlanID: expected.PlanID,
			PlanDigest: expected.PlanDigest, EnvelopeID: expected.EnvelopeID,
			EnvelopeDigest: expected.EnvelopeDigest, ApplicationID: expected.ApplicationID,
			Action: reference.Action,
		})
		if err != nil {
			return nil, fmt.Errorf("encode approval consumption request: %w", err)
		}
		endpoint := base + "/v1/approvals/" + url.PathEscape(reference.ApprovalID) + "/consume"
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create approval consumption request: %w", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("consume approval %q: %w", reference.ApprovalID, err)
		}
		responseRaw, responseErr := readApprovalRecordResponse(response)
		if responseErr != nil {
			return nil, fmt.Errorf("consume approval %q: %w", reference.ApprovalID, responseErr)
		}
		rawRecords = append(rawRecords, responseRaw)
	}
	if err := VerifyApprovalRecords(rawRecords, expected); err != nil {
		return nil, err
	}
	return rawRecords, nil
}

func readApprovalRecordResponse(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return nil, fmt.Errorf("approval consumption rejected (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("approval response content-type must be application/json")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxApprovalRecordBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read approval response: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxApprovalRecordBytes {
		return nil, fmt.Errorf("approval response size is invalid")
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("approval response JSON is invalid: %w", err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, fmt.Errorf("approval response is not canonical JSON")
	}
	digest, err := contracts.ApprovalRecordDigest(raw)
	if err != nil {
		return nil, fmt.Errorf("digest approval response: %w", err)
	}
	if response.Header.Get("ETag") != fmt.Sprintf("\"%s\"", digest) {
		return nil, fmt.Errorf("approval response ETag does not match its canonical digest")
	}
	if err := contracts.ValidateApprovalRecord(raw); err != nil {
		return nil, fmt.Errorf("approval response schema rejected: %w", err)
	}
	return raw, nil
}

func containsApprovalAction(actions []string, wanted string) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func validateApprovalExpectation(expected ApprovalExpectation) error {
	if expected.CaseID == "" || expected.PlanID == "" || expected.PlanDigest == "" ||
		expected.EnvelopeID == "" || expected.EnvelopeDigest == "" || expected.TenantScope == "" ||
		expected.HostID == "" || expected.ApplicationID == "" {
		return fmt.Errorf("approval expectation must identify case, plan, envelope, tenant, host, and application")
	}
	if expected.Now.IsZero() || expected.ClockSkew < 0 || expected.MaxValidity <= 0 {
		return fmt.Errorf("approval expectation requires current time, non-negative clock skew, and positive max validity")
	}
	return nil
}

func validateApprovalTimeWindow(record contracts.ApprovalRecord, expected ApprovalExpectation) error {
	approvedAt, err := time.Parse(time.RFC3339Nano, record.ApprovedAt)
	if err != nil {
		return fmt.Errorf("approval approvedAt is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("approval expiresAt is invalid: %w", err)
	}
	now := expected.Now.UTC()
	if !expiresAt.After(approvedAt) || approvedAt.After(now.Add(expected.ClockSkew)) {
		return fmt.Errorf("approval is future-dated or has an invalid validity interval")
	}
	if !now.Before(expiresAt) {
		return fmt.Errorf("approval is expired")
	}
	if expiresAt.Sub(approvedAt) > expected.MaxValidity {
		return fmt.Errorf("approval validity exceeds the configured maximum")
	}
	return nil
}
