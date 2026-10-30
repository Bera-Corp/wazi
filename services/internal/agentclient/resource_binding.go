package agentclient

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// ExecutionResourceRecords are the exact records referenced by a v2 execution
// envelope. Their identities and canonical digests are checked before the
// caller may obtain approval digests.
type ExecutionResourceRecords struct {
	HostProfileRaw                []byte
	ContributionBudgetRaw         []byte
	ManagedVolumeQualificationRaw []byte
}

// VerifyExecutionBindingWithAuthenticatedPolicy requires a policy bundle
// fetched through the paired control-plane client before running the existing
// resource-bound signature, plan, host, budget, and storage checks.
func VerifyExecutionBindingWithAuthenticatedPolicy(
	manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw []byte,
	signingPolicy verify.Policy,
	expected ExecutionBindingExpectation,
	resources ExecutionResourceRecords,
	capabilityPolicy VerifiedCapabilityPolicy,
) (VerifiedExecutionBinding, error) {
	if !capabilityPolicy.valid() {
		return VerifiedExecutionBinding{}, fmt.Errorf("authenticated capability policy is required")
	}
	expected.Policy = capabilityPolicy.BindingSnapshot()
	return VerifyExecutionBindingWithResources(
		manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw,
		signingPolicy, expected, resources,
	)
}

// VerifyExecutionBindingWithResources performs the signed release and plan
// checks used by VerifyExecutionBinding, then binds a fresh host profile, an
// active approved budget, and a live qualified volume record to a v2 envelope.
// It is read-only; approval verification, runtime enforcement, and execution
// remain separate mandatory checks.
func VerifyExecutionBindingWithResources(
	manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw []byte,
	policy verify.Policy,
	expected ExecutionBindingExpectation,
	resources ExecutionResourceRecords,
) (VerifiedExecutionBinding, error) {
	var empty VerifiedExecutionBinding
	if expected.HostID == "" {
		return empty, fmt.Errorf("resource-bound execution requires an authenticated host ID")
	}
	result, err := verifyExecutionBinding(
		manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw,
		policy, expected, contracts.ValidateExecutionContextEnvelopeV2,
	)
	if err != nil {
		return empty, err
	}
	envelope, err := parseCanonicalObject(envelopeRaw, "resource-bound execution context envelope")
	if err != nil {
		return empty, err
	}
	hostProfile, err := contracts.DecodeHostProfile(resources.HostProfileRaw)
	if err != nil {
		return empty, fmt.Errorf("host profile rejected: %w", err)
	}
	if err := contracts.ValidateHostProfile(hostProfile, expected.Now); err != nil {
		return empty, fmt.Errorf("host profile is not fresh and valid: %w", err)
	}
	if hostProfile.HostID != expected.HostID || hostProfile.TenantScope != expected.TenantScope {
		return empty, fmt.Errorf("host profile does not match the authenticated host and tenant")
	}
	budget, err := contracts.DecodeContributionBudget(resources.ContributionBudgetRaw)
	if err != nil {
		return empty, fmt.Errorf("contribution budget rejected: %w", err)
	}
	if budget.ApprovalState != "approved" {
		return empty, fmt.Errorf("contribution budget is not approved")
	}
	if budget.HostID != expected.HostID || budget.TenantScope != expected.TenantScope {
		return empty, fmt.Errorf("contribution budget does not match the authenticated host and tenant")
	}
	active, err := contracts.IsWindowActive(budget.ValidFrom, budget.ValidUntil, expected.Now)
	if err != nil || !active {
		return empty, fmt.Errorf("contribution budget is not currently valid")
	}
	manifest, err := parseCanonicalObject(manifestRaw, "signed manifest")
	if err != nil {
		return empty, err
	}
	if err := validateApplicationResourceLimits(manifest, budget); err != nil {
		return empty, fmt.Errorf("application resource limits rejected: %w", err)
	}
	qualification, err := contracts.DecodeManagedVolumeQualification(resources.ManagedVolumeQualificationRaw)
	if err != nil {
		return empty, fmt.Errorf("managed-volume qualification rejected: %w", err)
	}
	if qualification.HostID != expected.HostID || qualification.TenantScope != expected.TenantScope {
		return empty, fmt.Errorf("managed-volume qualification does not match the authenticated host and tenant")
	}
	if qualification.Result != "qualified" || !qualification.Storage.ManagedVolumesCapable ||
		!qualification.Storage.QuotaEnforceable || qualification.Storage.EnforcementMethod == "preflight-only" {
		return empty, fmt.Errorf("managed-volume qualification does not enforce a hard storage limit")
	}
	active, err = contracts.IsWindowActive(qualification.ObservedAt, qualification.ExpiresAt, expected.Now)
	if err != nil || !active {
		return empty, fmt.Errorf("managed-volume qualification is not currently valid")
	}
	if err := requireResourceLimit(budget.OwnerBudget.CPUCores, strconv.FormatFloat(hostProfile.Resources.CPUCores, 'f', -1, 64), "host profile CPU"); err != nil {
		return empty, err
	}
	if err := requireResourceLimit(budget.OwnerBudget.RAMMb, strconv.FormatInt(hostProfile.Resources.RAMMb, 10), "host profile RAM"); err != nil {
		return empty, err
	}
	if err := requireResourceLimit(budget.OwnerBudget.StorageMb, strconv.FormatInt(hostProfile.Resources.StorageFreeMb, 10), "host profile free storage"); err != nil {
		return empty, err
	}
	if err := requireResourceLimit(budget.OwnerBudget.StorageMb, string(qualification.Storage.StorageFreeMb), "managed-volume free storage"); err != nil {
		return empty, err
	}
	hostDigest, err := canonicalResourceDigest(resources.HostProfileRaw, "host profile")
	if err != nil {
		return empty, err
	}
	budgetDigest, err := canonicalResourceDigest(resources.ContributionBudgetRaw, "contribution budget")
	if err != nil {
		return empty, err
	}
	qualificationDigest, err := canonicalResourceDigest(resources.ManagedVolumeQualificationRaw, "managed-volume qualification")
	if err != nil {
		return empty, err
	}
	if err := requireRecordReference(envelope["hostProfileRef"], hostProfile.HostID, hostDigest, hostProfile.SchemaVersion, "envelope hostProfileRef"); err != nil {
		return empty, err
	}
	if err := requireRecordReference(envelope["contributionBudgetRef"], budget.BudgetID, budgetDigest, budget.SchemaVersion, "envelope contributionBudgetRef"); err != nil {
		return empty, err
	}
	if err := requireRecordReference(envelope["managedVolumeQualificationRef"], qualification.QualificationID, qualificationDigest, qualification.SchemaVersion, "envelope managedVolumeQualificationRef"); err != nil {
		return empty, err
	}
	result.HostProfileDigest = hostDigest
	result.ContributionBudgetDigest = budgetDigest
	result.ManagedVolumeQualificationDigest = qualificationDigest
	return result, nil
}

func canonicalResourceDigest(raw []byte, label string) (string, error) {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", label, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return "", fmt.Errorf("%s must be a JSON object", label)
	}
	digest, err := verify.CanonicalDigest(value)
	if err != nil {
		return "", fmt.Errorf("digest %s: %w", label, err)
	}
	return digest, nil
}

func requireResourceLimit(limit json.Number, available, label string) error {
	requested, ok := new(big.Rat).SetString(string(limit))
	if !ok {
		return fmt.Errorf("%s budget is not a valid number", label)
	}
	capacity, ok := new(big.Rat).SetString(available)
	if !ok {
		return fmt.Errorf("%s capacity is not a valid number", label)
	}
	if requested.Cmp(capacity) > 0 {
		return fmt.Errorf("owner budget exceeds %s", label)
	}
	return nil
}
