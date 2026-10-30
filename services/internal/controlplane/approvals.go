package controlplane

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/approvalregistry"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const (
	MaxApprovalIssueBodyBytes = 2 << 20
	MaxApprovalLifetime       = 15 * time.Minute
)

type issueApprovalRequest struct {
	CaseID          string          `json:"caseId"`
	Plan            json.RawMessage `json:"plan"`
	Envelope        json.RawMessage `json:"envelope"`
	HostID          string          `json:"hostId"`
	Action          string          `json:"action"`
	Decision        string          `json:"decision"`
	ValidForSeconds int64           `json:"validForSeconds"`
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

func (s *Server) handleIssueApproval(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ApprovalRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval_service_unavailable"})
		return
	}
	approver, ok := s.authenticateApprover(w, r)
	if !ok {
		return
	}
	var request issueApprovalRequest
	if err := s.decodeApprovalIssueRequest(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_request", "detail": err.Error()})
		return
	}
	if request.CaseID == "" || request.HostID == "" || request.Action == "" ||
		(request.Decision != "approved" && request.Decision != "denied") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_request", "detail": "caseId, hostId, action, and decision are required"})
		return
	}
	plan, err := parseApprovalContext(request.Plan, "deployment plan", contracts.ValidateDeploymentPlan)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_plan", "detail": err.Error()})
		return
	}
	envelope, err := parseApprovalContext(request.Envelope, "execution context envelope", validateApprovalEnvelope)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_envelope", "detail": err.Error()})
		return
	}
	if plan["envelopeId"] != envelope["envelopeId"] || plan["operation"] != envelope["operation"] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_context_mismatch"})
		return
	}
	if !sameApprovalActions(plan["requiredApprovals"], envelope["requiredApprovals"]) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_action_sets_mismatch"})
		return
	}
	if envelope["tenantScope"] == nil || envelope["applicationId"] == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_context_identity_missing"})
		return
	}
	hostRef, ok := approvalObject(envelope["hostProfileRef"])
	if !ok || hostRef["recordId"] != request.HostID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_host_mismatch"})
		return
	}
	if !approvalListContains(plan["requiredApprovals"], request.Action) || !approvalListContains(envelope["requiredApprovals"], request.Action) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_action_not_required"})
		return
	}
	now := s.cfg.Now().UTC()
	validFor := MaxApprovalLifetime
	if request.ValidForSeconds < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_lifetime_out_of_range"})
		return
	}
	if request.ValidForSeconds > 0 {
		if request.ValidForSeconds > int64(MaxApprovalLifetime/time.Second) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_lifetime_out_of_range"})
			return
		}
		validFor = time.Duration(request.ValidForSeconds) * time.Second
	}
	if validFor <= 0 || validFor > MaxApprovalLifetime {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_lifetime_out_of_range"})
		return
	}
	planExpiry, err := approvalTime(plan["expiresAt"], "plan expiresAt")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_plan", "detail": err.Error()})
		return
	}
	envelopeExpiry, err := approvalTime(envelope["expiresAt"], "envelope expiresAt")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_envelope", "detail": err.Error()})
		return
	}
	planCreated, err := approvalTime(plan["createdAt"], "plan createdAt")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_plan", "detail": err.Error()})
		return
	}
	envelopeCreated, err := approvalTime(envelope["createdAt"], "envelope createdAt")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_envelope", "detail": err.Error()})
		return
	}
	if planCreated.After(now) || envelopeCreated.After(now) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval_context_future_dated"})
		return
	}
	if !now.Before(planExpiry) || !now.Before(envelopeExpiry) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval_context_expired"})
		return
	}
	expiresAt := now.Add(validFor)
	if planExpiry.Before(expiresAt) {
		expiresAt = planExpiry
	}
	if envelopeExpiry.Before(expiresAt) {
		expiresAt = envelopeExpiry
	}
	if !expiresAt.After(now) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval_context_expires_too_soon"})
		return
	}
	planDigest, err := verify.CanonicalDigest(plan)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_plan", "detail": err.Error()})
		return
	}
	envelopeDigest, err := verify.CanonicalDigest(envelope)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_envelope", "detail": err.Error()})
		return
	}
	approvalID, err := newApprovalID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval_id_generation_failed"})
		return
	}
	record := contracts.ApprovalRecord{
		SchemaVersion:  "1.0.0",
		ApprovalID:     approvalID,
		CaseID:         request.CaseID,
		PlanID:         plan["planId"].(string),
		PlanDigest:     planDigest,
		EnvelopeID:     envelope["envelopeId"].(string),
		EnvelopeDigest: envelopeDigest,
		TenantScope:    envelope["tenantScope"].(string),
		HostID:         request.HostID,
		ApplicationID:  envelope["applicationId"].(string),
		Action:         request.Action,
		Decision:       request.Decision,
		Approver:       contracts.Approver{ID: approver.ID, Role: approver.Role},
		ApprovedAt:     now.Format(time.RFC3339Nano),
		ExpiresAt:      expiresAt.Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval_encoding_failed"})
		return
	}
	value, err := verify.ParseJSON(encoded)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval_encoding_failed"})
		return
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval_encoding_failed"})
		return
	}
	entry, err := s.cfg.ApprovalRegistry.Put(canonical)
	switch {
	case err == nil:
		writeApprovalRecord(w, http.StatusCreated, canonical, entry.Digest)
	case errors.Is(err, approvalregistry.ErrInvalidApproval):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval_rejected", "detail": err.Error()})
	case errors.Is(err, approvalregistry.ErrCapacity):
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": "approval_registry_limit"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval_registry_error", "detail": err.Error()})
	}
}

// Approval issuance must validate the resource-bound v2 envelope with its own
// schema. Never fall back to a different version after schema validation fails.
func validateApprovalEnvelope(raw []byte) error {
	var header struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return err
	}
	switch header.SchemaVersion {
	case "1.0.0":
		return contracts.ValidateExecutionContextEnvelope(raw)
	case "2.0.0":
		return contracts.ValidateExecutionContextEnvelopeV2(raw)
	default:
		return fmt.Errorf("unsupported execution envelope schema version %q", header.SchemaVersion)
	}
}

func (s *Server) authenticateApprover(w http.ResponseWriter, r *http.Request) (contracts.Approver, bool) {
	if !isLoopbackRemote(r.RemoteAddr) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "approvals may only be issued by a local approver"})
		return contracts.Approver{}, false
	}
	if len(s.cfg.ApproverCredentials) > 0 {
		if err := ValidateApproverCredentials(s.cfg.ApproverCredentials); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval_credentials_invalid"})
			return contracts.Approver{}, false
		}
		credential, matched := matchApproverCredential(s.cfg.ApproverCredentials, r.Header.Get("X-Wazi-Approver-Credential"))
		if !matched {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "approver credential required"})
			return contracts.Approver{}, false
		}
		return contracts.Approver{ID: credential.ID, Role: credential.Role}, true
	}
	if s.cfg.ApproverID == "" || (s.cfg.ApproverRole != "user" && s.cfg.ApproverRole != "maintainer") || s.cfg.OperatorToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval_service_unavailable"})
		return contracts.Approver{}, false
	}
	if !s.requireOperator(w, r) {
		return contracts.Approver{}, false
	}
	return contracts.Approver{ID: s.cfg.ApproverID, Role: s.cfg.ApproverRole}, true
}

func (s *Server) handleConsumeApproval(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "paired_node_certificate_required"})
		return
	}
	if s.cfg.ApprovalRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval_registry_unavailable"})
		return
	}
	var request consumeApprovalRequest
	if err := s.decodeStrict(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_consumption", "detail": err.Error()})
		return
	}
	if request.JobID == "" || request.CaseID == "" || request.PlanID == "" || request.PlanDigest == "" ||
		request.EnvelopeID == "" || request.EnvelopeDigest == "" || request.ApplicationID == "" || request.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_approval_consumption", "detail": "all approval and job binding fields are required"})
		return
	}
	var record []byte
	var consumedEntry approvalregistry.Entry
	err := s.cfg.Registry.AuthorizeCommit(peerCert(r), func(node pairing.Node) error {
		binding := approvalregistry.ConsumptionBinding{
			JobID: request.JobID, CaseID: request.CaseID, PlanID: request.PlanID,
			PlanDigest: request.PlanDigest, EnvelopeID: request.EnvelopeID,
			EnvelopeDigest: request.EnvelopeDigest, TenantScope: node.TenantScope,
			HostID: node.HostID, ApplicationID: request.ApplicationID, Action: request.Action,
		}
		var consumeErr error
		record, consumedEntry, consumeErr = s.cfg.ApprovalRegistry.Consume(r.PathValue("approvalID"), binding)
		return consumeErr
	})
	switch {
	case err == nil:
		writeApprovalRecord(w, http.StatusOK, record, consumedEntry.Digest)
	case errors.Is(err, approvalregistry.ErrBindingMismatch):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "approval_binding_mismatch"})
	case errors.Is(err, approvalregistry.ErrNotApproved):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "approval_denied"})
	case errors.Is(err, approvalregistry.ErrAlreadyConsumed):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval_already_consumed"})
	case errors.Is(err, approvalregistry.ErrExpired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval_expired"})
	case errors.Is(err, os.ErrNotExist):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval_not_found"})
	default:
		status := authErrorStatus(err)
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			writeJSON(w, status, map[string]string{"error": "unauthorized_node"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval_consumption_failed", "detail": err.Error()})
	}
}

func (s *Server) decodeApprovalIssueRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	s.beginBodyRead(w)
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxApprovalIssueBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body is required")
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("trailing data after JSON value")
		}
		return err
	}
	return nil
}

func parseApprovalContext(raw json.RawMessage, label string, validate func([]byte) error) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s is required", label)
	}
	if err := validate(raw); err != nil {
		return nil, fmt.Errorf("%s schema rejected: %w", label, err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, err
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(raw, canonical) {
		return nil, fmt.Errorf("%s is not canonical JSON", label)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return object, nil
}

func approvalObject(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func approvalListContains(value any, wanted string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func sameApprovalActions(left, right any) bool {
	leftItems, leftOK := left.([]any)
	rightItems, rightOK := right.([]any)
	if !leftOK || !rightOK || len(leftItems) != len(rightItems) {
		return false
	}
	values := make(map[string]struct{}, len(leftItems))
	for _, item := range leftItems {
		value, ok := item.(string)
		if !ok {
			return false
		}
		values[value] = struct{}{}
	}
	for _, item := range rightItems {
		value, ok := item.(string)
		if !ok {
			return false
		}
		if _, exists := values[value]; !exists {
			return false
		}
	}
	return true
}

func approvalTime(value any, label string) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("%s is invalid", label)
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s is invalid: %w", label, err)
	}
	return parsed, nil
}

func newApprovalID() (string, error) {
	var entropy [18]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "appr_" + base64.RawURLEncoding.EncodeToString(entropy[:]), nil
}

func writeApprovalRecord(w http.ResponseWriter, status int, raw []byte, digest string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", digest))
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
