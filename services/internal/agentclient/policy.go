package agentclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type CapabilityPolicyReference struct {
	PolicyID    string
	Version     string
	Digest      string
	TenantScope string
}

// VerifiedCapabilityPolicy can only be populated by a successful paired-TLS
// fetch. Accessors return copies so the authenticated policy cannot be changed
// after its identity and digest have been checked.
type VerifiedCapabilityPolicy struct {
	snapshot     AuthenticatedCapabilityPolicy
	requirements map[string]string
}

func FetchCapabilityPolicyBundle(
	ctx context.Context,
	baseURL string,
	tlsConfig *tls.Config,
	expected CapabilityPolicyReference,
) (VerifiedCapabilityPolicy, error) {
	var empty VerifiedCapabilityPolicy
	if expected.PolicyID == "" || expected.Version == "" || expected.Digest == "" || expected.TenantScope == "" {
		return empty, fmt.Errorf("capability policy reference must identify policy, version, digest, and tenant")
	}
	if err := requirePairedTLS(tlsConfig, "capability policy fetch"); err != nil {
		return empty, err
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return empty, err
	}
	endpoint := base + "/v1/policy-bundles/" + url.PathEscape(expected.PolicyID) + "/" + url.PathEscape(expected.Version)
	raw, err := fetchCanonicalRecord(ctx, endpoint, tlsConfig)
	if err != nil {
		return empty, fmt.Errorf("fetch capability policy bundle: %w", err)
	}
	bundle, err := contracts.DecodePolicyBundle(raw)
	if err != nil {
		return empty, fmt.Errorf("capability policy bundle rejected: %w", err)
	}
	policyID, _ := bundle["policyId"].(string)
	version, _ := bundle["version"].(string)
	tenantScope, _ := bundle["tenantScope"].(string)
	if policyID != expected.PolicyID || version != expected.Version || tenantScope != expected.TenantScope {
		return empty, fmt.Errorf("capability policy identity does not match the authenticated reference")
	}
	digest, err := verify.CanonicalDigest(bundle)
	if err != nil {
		return empty, fmt.Errorf("digest capability policy bundle: %w", err)
	}
	if digest != expected.Digest {
		return empty, fmt.Errorf("capability policy digest does not match the execution envelope")
	}
	allowed, err := capabilityStrings(bundle["allowedCapabilities"], "allowedCapabilities")
	if err != nil {
		return empty, err
	}
	denied, err := capabilityStrings(bundle["deniedCapabilities"], "deniedCapabilities")
	if err != nil {
		return empty, err
	}
	rules, ok := bundle["approvalRules"].([]any)
	if !ok {
		return empty, fmt.Errorf("capability policy approvalRules is invalid")
	}
	requirements := make(map[string]string, len(rules))
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return empty, fmt.Errorf("capability policy approval rule is invalid")
		}
		action, actionOK := rule["action"].(string)
		requirement, requirementOK := rule["approval"].(string)
		if !actionOK || !requirementOK || action == "" || requirement == "" {
			return empty, fmt.Errorf("capability policy approval rule is incomplete")
		}
		requirements[action] = requirement
	}
	return VerifiedCapabilityPolicy{
		snapshot: AuthenticatedCapabilityPolicy{
			BundleID:            policyID,
			Version:             version,
			Digest:              digest,
			TenantScope:         tenantScope,
			AllowedCapabilities: allowed,
			DeniedCapabilities:  denied,
		},
		requirements: requirements,
	}, nil
}

func (p VerifiedCapabilityPolicy) BindingSnapshot() AuthenticatedCapabilityPolicy {
	return AuthenticatedCapabilityPolicy{
		BundleID:            p.snapshot.BundleID,
		Version:             p.snapshot.Version,
		Digest:              p.snapshot.Digest,
		TenantScope:         p.snapshot.TenantScope,
		AllowedCapabilities: append([]string(nil), p.snapshot.AllowedCapabilities...),
		DeniedCapabilities:  append([]string(nil), p.snapshot.DeniedCapabilities...),
	}
}

func (p VerifiedCapabilityPolicy) ApprovalRequirements() map[string]string {
	copy := make(map[string]string, len(p.requirements))
	for action, requirement := range p.requirements {
		copy[action] = requirement
	}
	return copy
}

func (p VerifiedCapabilityPolicy) valid() bool {
	return p.snapshot.BundleID != "" && p.snapshot.Version != "" &&
		p.snapshot.Digest != "" && p.snapshot.TenantScope != "" &&
		len(p.snapshot.AllowedCapabilities) > 0 && len(p.requirements) > 0
}

func capabilityStrings(value any, label string) ([]string, error) {
	items, ok := value.([]any)
	if !ok && value == nil {
		return nil, nil
	}
	if !ok {
		return nil, fmt.Errorf("capability policy %s is invalid", label)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		capability, ok := item.(string)
		if !ok || capability == "" {
			return nil, fmt.Errorf("capability policy %s contains an invalid value", label)
		}
		result = append(result, capability)
	}
	return result, nil
}
