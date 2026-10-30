package controlplane

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Bera-Corp/wazi/services/internal/manifestregistry"
)

func (s *Server) handlePostRevocation(w http.ResponseWriter, r *http.Request) {
	if s.cfg.OperatorToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "revocation_write_requires_operator_token",
		})
		return
	}
	if !s.requireOperator(w, r) {
		return
	}
	if s.cfg.ManifestRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "revocation_registry_unavailable",
		})
		return
	}
	body, err := s.readLimitedRevocationBody(w, r)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "revocation_too_large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_revocation_request", "detail": err.Error()})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "revocation_body_required"})
		return
	}
	entry, err := s.cfg.ManifestRegistry.PutRevocation(body)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{
			"listId": entry.ListID, "sequence": entry.Sequence, "revocationDigest": entry.Digest,
		})
	case errors.Is(err, manifestregistry.ErrInvalidRecord):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "revocation_rejected", "detail": err.Error()})
	case errors.Is(err, manifestregistry.ErrRevocationRollback):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "revocation_sequence_rollback"})
	case errors.Is(err, manifestregistry.ErrRevocationListConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "revocation_list_identity_conflict"})
	case errors.Is(err, manifestregistry.ErrVersionConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "revocation_sequence_conflict"})
	case errors.Is(err, manifestregistry.ErrRegistryLimit):
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": "revocation_registry_limit"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "revocation_registry_error", "detail": err.Error()})
	}
}

func (s *Server) handleGetRevocation(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		if s.cfg.OperatorToken == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "revocation_read_requires_authenticated_node"})
			return
		}
		if !s.requireOperator(w, r) {
			return
		}
	} else if _, ok := s.authenticate(w, r); !ok {
		return
	}
	if s.cfg.ManifestRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "revocation_registry_unavailable"})
		return
	}
	record, entry, ok := s.cfg.ManifestRegistry.GetRevocation()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "revocation_list_not_found"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", entry.Digest))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(record)
}

func (s *Server) readLimitedRevocationBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	s.beginBodyRead(w)
	if contentType := r.Header.Get("Content-Type"); contentType != "" &&
		!strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxManifestBodyBytes)
	return io.ReadAll(r.Body)
}
