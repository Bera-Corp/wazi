package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Bera-Corp/wazi/services/internal/candidateworkflow"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
)

var packageCandidateID = regexp.MustCompile("^cand_[0-9a-f]{32}$")

func (s *Server) candidateTenantScope() (string, error) {
	bundle, err := contracts.DecodePolicyBundle(s.cfg.CapabilityPolicyBundle)
	if err != nil {
		return "", fmt.Errorf("configured tenant policy unavailable")
	}
	tenant, ok := bundle["tenantScope"].(string)
	if !ok || strings.TrimSpace(tenant) == "" {
		return "", fmt.Errorf("configured tenant scope unavailable")
	}
	return tenant, nil
}

func (s *Server) candidateSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.contextAuthorized(w, r) {
		return "", false
	}
	if s.cfg.ApproverRole != "maintainer" || s.cfg.ApproverID == "" || len(s.cfg.ApproverCredentials) != 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "candidate_workflow_requires_configured_maintainer"})
		return "", false
	}
	tenant, err := s.candidateTenantScope()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "candidate_tenant_unavailable"})
		return "", false
	}
	if s.cfg.CandidateStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "candidate_store_unavailable"})
		return "", false
	}
	return tenant, true
}

func (s *Server) registerPackageCandidateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/package-candidates", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.candidateSession(w, r)
		if !ok {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, s.cfg.CandidateStore.List(tenant))
	})
	mux.HandleFunc("POST /v1/package-candidates", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.candidateSession(w, r)
		if !ok {
			return
		}
		var request candidateworkflow.ImportRequest
		if err := s.decodeCandidateJSON(w, r, 2<<20, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_import_rejected", "detail": err.Error()})
			return
		}
		record, err := s.cfg.CandidateStore.Import(tenant, request, s.cfg.Now())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "candidate_import_rejected", "detail": err.Error()})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, record)
	})
	mux.HandleFunc("GET /v1/package-candidates/{candidateID}", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.candidateSession(w, r)
		if !ok {
			return
		}
		id := r.PathValue("candidateID")
		if !packageCandidateID.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_id_invalid"})
			return
		}
		record, found := s.cfg.CandidateStore.Get(tenant, id)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate_not_found"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, record)
	})
	mux.HandleFunc("POST /v1/package-candidates/{candidateID}/qualification", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.candidateSession(w, r)
		if !ok {
			return
		}
		if !packageCandidateID.MatchString(r.PathValue("candidateID")) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_id_invalid"})
			return
		}
		var request struct {
			CandidateDigest string          `json:"candidateDigest"`
			Receipt         json.RawMessage `json:"receipt"`
		}
		if err := s.decodeStrict(w, r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "qualification_request_invalid"})
			return
		}
		record, err := s.cfg.CandidateStore.RecordQualification(tenant, r.PathValue("candidateID"), request.CandidateDigest, request.Receipt, s.cfg.Now())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "qualification_rejected", "detail": err.Error()})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, record)
	})
	mux.HandleFunc("POST /v1/package-candidates/{candidateID}/review", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.candidateSession(w, r)
		if !ok {
			return
		}
		if !packageCandidateID.MatchString(r.PathValue("candidateID")) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_id_invalid"})
			return
		}
		var request struct {
			CandidateDigest string `json:"candidateDigest"`
		}
		if err := s.decodeStrict(w, r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "review_request_invalid"})
			return
		}
		record, err := s.cfg.CandidateStore.Review(tenant, r.PathValue("candidateID"), request.CandidateDigest, s.cfg.ApproverID, s.cfg.Now())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "candidate_review_rejected", "detail": err.Error()})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, record)
	})
	mux.HandleFunc("POST /v1/package-candidates/{candidateID}/promotion", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.candidateSession(w, r)
		if !ok {
			return
		}
		if !packageCandidateID.MatchString(r.PathValue("candidateID")) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_id_invalid"})
			return
		}
		if s.cfg.ManifestRegistry == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verified_release_registry_unavailable"})
			return
		}
		var request struct {
			CandidateDigest string          `json:"candidateDigest"`
			SignedManifest  json.RawMessage `json:"signedManifest"`
		}
		if err := s.decodeCandidateJSON(w, r, 2<<20, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "promotion_request_invalid"})
			return
		}
		record, found := s.cfg.CandidateStore.Get(tenant, r.PathValue("candidateID"))
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate_not_found"})
			return
		}
		if request.CandidateDigest != record.CandidateDigest {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "candidate_digest_changed"})
			return
		}
		signedDigest, err := candidateworkflow.ValidatePromotion(record, request.SignedManifest, s.cfg.SigningPolicy)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "signed_release_rejected", "detail": err.Error()})
			return
		}
		entry, err := s.cfg.ManifestRegistry.Put(request.SignedManifest)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, manifestregistry.ErrVersionConflict) {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"error": "signed_release_rejected", "detail": err.Error()})
			return
		}
		if entry.Digest != signedDigest {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "registry_digest_mismatch"})
			return
		}
		record, err = s.cfg.CandidateStore.MarkPromoted(tenant, record.CandidateID, request.CandidateDigest, entry.Digest, s.cfg.Now())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "promotion_receipt_not_persisted"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, record)
	})
}

func (s *Server) decodeCandidateJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	s.beginBodyRead(w)
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return fmt.Errorf("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body required")
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
