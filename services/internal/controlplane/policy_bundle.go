package controlplane

import (
	"fmt"
	"net/http"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// handleGetCapabilityPolicyBundle serves one operator-managed capability
// policy only to a paired node in the bundle's tenant. The authenticated TLS
// channel and configured server CA establish the source; the bundle cannot
// supply or replace the trust anchor used to reach this handler.
func (s *Server) handleGetCapabilityPolicyBundle(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CA == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "capability_policy_requires_paired_node",
		})
		return
	}
	node, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if len(s.cfg.CapabilityPolicyBundle) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "capability_policy_unavailable",
		})
		return
	}
	bundle, err := contracts.DecodePolicyBundle(s.cfg.CapabilityPolicyBundle)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "capability_policy_invalid",
		})
		return
	}
	policyID, _ := bundle["policyId"].(string)
	version, _ := bundle["version"].(string)
	tenantScope, _ := bundle["tenantScope"].(string)
	if policyID != r.PathValue("policyID") || version != r.PathValue("version") {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "capability_policy_not_found",
		})
		return
	}
	if node.TenantScope != tenantScope {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "cross_tenant_capability_policy_read",
		})
		return
	}
	canonical, err := verify.CanonicalJSON(bundle)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "capability_policy_canonicalization_failed",
		})
		return
	}
	digest, err := verify.CanonicalDigest(bundle)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "capability_policy_digest_failed",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", digest))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(canonical)
}
