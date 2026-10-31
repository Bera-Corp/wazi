package agentclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/pairing"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type ExecutionPreflightConfig struct {
	ControlPlaneURL     string
	TLSConfig           *tls.Config
	SigningPolicy       verify.Policy
	CapabilityPolicy    CapabilityPolicyReference
	RevocationState     *RevocationStateStore
	RevocationFreshness RevocationFreshness
	ClockSkew           time.Duration
	ApprovalMaxValidity time.Duration
}

type ExecutionPreflightRequest struct {
	JobID                         string
	CaseID                        string
	EnvelopeID                    string
	PlanID                        string
	Release                       ReleaseExpectation
	RollbackProcedure             ProcedureExpectation
	EnvelopeRaw                   []byte
	PlanRaw                       []byte
	Approvals                     []ApprovalReference
	ContributionBudgetRaw         []byte
	ManagedVolumeQualificationRaw []byte
}

// PreparedExecution is returned only after the paired policy, exact release,
// current revocation state, host/resource records, typed plan, and one-job
// approval records have all passed their existing verification helpers.
// Accessors return copies so later code cannot change the bytes that were
// authorized.
type PreparedExecution struct {
	valid          bool
	binding        VerifiedExecutionBinding
	release        VerifiedReleaseArtifacts
	rollbackRaw    []byte
	envelopeRaw    []byte
	planRaw        []byte
	hostProfileRaw []byte
	policy         VerifiedCapabilityPolicy
	approvalRaw    [][]byte
	tenantScope    string
	hostID         string
}

// Verified reports whether this value came from a successful preflight.
// The zero value is deliberately not executable.
func (p PreparedExecution) Verified() bool { return p.valid }

func (p PreparedExecution) Binding() VerifiedExecutionBinding {
	return p.binding
}

func (p PreparedExecution) Manifest() []byte {
	return p.release.Manifest()
}

func (p PreparedExecution) Procedure() []byte {
	return p.release.Procedure()
}

func (p PreparedExecution) RollbackProcedure() []byte {
	return append([]byte(nil), p.rollbackRaw...)
}

func (p PreparedExecution) Envelope() []byte {
	return append([]byte(nil), p.envelopeRaw...)
}

func (p PreparedExecution) Plan() []byte {
	return append([]byte(nil), p.planRaw...)
}

func (p PreparedExecution) HostProfile() []byte {
	return append([]byte(nil), p.hostProfileRaw...)
}

func (p PreparedExecution) CapabilityPolicy() VerifiedCapabilityPolicy {
	return VerifiedCapabilityPolicy{
		snapshot: AuthenticatedCapabilityPolicy{
			BundleID:            p.policy.snapshot.BundleID,
			Version:             p.policy.snapshot.Version,
			Digest:              p.policy.snapshot.Digest,
			TenantScope:         p.policy.snapshot.TenantScope,
			AllowedCapabilities: append([]string(nil), p.policy.snapshot.AllowedCapabilities...),
			DeniedCapabilities:  append([]string(nil), p.policy.snapshot.DeniedCapabilities...),
		},
		requirements: p.policy.ApprovalRequirements(),
	}
}

func (p PreparedExecution) ApprovalRecords() [][]byte {
	records := make([][]byte, len(p.approvalRaw))
	for i := range p.approvalRaw {
		records[i] = append([]byte(nil), p.approvalRaw[i]...)
	}
	return records
}

func (p PreparedExecution) TenantScope() string { return p.tenantScope }
func (p PreparedExecution) HostID() string      { return p.hostID }

// PrepareExecution wires the read-only trust, resource, and approval helpers
// into one decision. It must be called again immediately before every
// mutating step; approval consumption is idempotent for the same JobID.
func PrepareExecution(
	ctx context.Context,
	config ExecutionPreflightConfig,
	request ExecutionPreflightRequest,
	resources ExecutionResourceRecords,
) (PreparedExecution, error) {
	return prepareExecution(ctx, config, request, resources, true)
}

// prepareExecution also supports read-only context inspection. That path never
// consumes approvals and deliberately returns a non-executable value.
func prepareExecution(ctx context.Context, config ExecutionPreflightConfig, request ExecutionPreflightRequest, resources ExecutionResourceRecords, consume bool) (PreparedExecution, error) {
	var empty PreparedExecution
	if request.JobID == "" || request.CaseID == "" || request.EnvelopeID == "" || request.PlanID == "" {
		return empty, fmt.Errorf("execution preflight requires job, case, envelope, and plan identities")
	}
	if config.ClockSkew < 0 || config.ApprovalMaxValidity <= 0 {
		return empty, fmt.Errorf("execution preflight requires non-negative clock skew and positive approval validity")
	}
	if config.RevocationFreshness.MaxAge <= 0 {
		return empty, fmt.Errorf("execution preflight requires a positive revocation maximum age")
	}
	if config.RevocationState == nil {
		return empty, fmt.Errorf("execution preflight requires persistent revocation state")
	}
	if len(request.EnvelopeRaw) == 0 || len(request.PlanRaw) == 0 ||
		len(request.ContributionBudgetRaw) == 0 || len(request.ManagedVolumeQualificationRaw) == 0 {
		return empty, fmt.Errorf("execution preflight requires envelope, plan, budget, and managed-volume qualification records")
	}
	if err := requirePairedTLS(config.TLSConfig, "execution preflight"); err != nil {
		return empty, err
	}
	if len(config.TLSConfig.Certificates) == 0 || len(config.TLSConfig.Certificates[0].Certificate) == 0 {
		return empty, fmt.Errorf("execution preflight requires a directly inspectable paired certificate")
	}
	nodeCertificate, err := x509.ParseCertificate(config.TLSConfig.Certificates[0].Certificate[0])
	if err != nil {
		return empty, fmt.Errorf("parse paired node certificate: %w", err)
	}
	tenantScope, hostID, ok := pairing.CertBinding(nodeCertificate)
	if !ok {
		return empty, fmt.Errorf("paired node certificate has no tenant and host binding")
	}
	if config.CapabilityPolicy.TenantScope != tenantScope {
		return empty, fmt.Errorf("configured capability policy tenant does not match the paired node")
	}

	now := time.Now().UTC()
	freshness := config.RevocationFreshness
	freshness.Now = func() time.Time { return now }
	freshness.ClockSkew = config.ClockSkew
	policy, err := FetchCapabilityPolicyBundle(
		ctx, config.ControlPlaneURL, config.TLSConfig, config.CapabilityPolicy,
	)
	if err != nil {
		return empty, fmt.Errorf("authenticated capability policy preflight: %w", err)
	}
	release, err := FetchVerifiedReleaseArtifactsWithRevocation(
		ctx, config.ControlPlaneURL, config.TLSConfig, config.SigningPolicy,
		request.Release, freshness, config.RevocationState,
	)
	if err != nil {
		return empty, fmt.Errorf("signed release and revocation preflight: %w", err)
	}
	rollbackRaw, err := FetchVerifiedProcedureRecord(
		ctx, config.ControlPlaneURL, config.TLSConfig, config.SigningPolicy,
		request.RollbackProcedure,
	)
	if err != nil {
		return empty, fmt.Errorf("rollback procedure preflight: %w", err)
	}
	hostProfileRaw, err := FetchHostProfile(
		ctx, config.ControlPlaneURL, config.TLSConfig, tenantScope, hostID,
	)
	if err != nil {
		return empty, fmt.Errorf("fresh host profile preflight: %w", err)
	}
	resources.HostProfileRaw = hostProfileRaw
	resources.ContributionBudgetRaw = append([]byte(nil), request.ContributionBudgetRaw...)
	resources.ManagedVolumeQualificationRaw = append([]byte(nil), request.ManagedVolumeQualificationRaw...)

	expected := ExecutionBindingExpectation{
		Release:     request.Release,
		TenantScope: tenantScope,
		HostID:      hostID,
		EnvelopeID:  request.EnvelopeID,
		PlanID:      request.PlanID,
		Now:         now,
		ClockSkew:   config.ClockSkew,
	}
	binding, err := VerifyExecutionBindingWithAuthenticatedPolicy(
		release.Manifest(), release.Procedure(), rollbackRaw,
		request.EnvelopeRaw, request.PlanRaw, config.SigningPolicy,
		expected, resources, policy,
	)
	if err != nil {
		return empty, fmt.Errorf("resource-bound execution plan preflight: %w", err)
	}
	if !consume {
		return PreparedExecution{
			binding: binding, release: release, rollbackRaw: append([]byte(nil), rollbackRaw...),
			envelopeRaw: append([]byte(nil), request.EnvelopeRaw...), planRaw: append([]byte(nil), request.PlanRaw...),
			hostProfileRaw: append([]byte(nil), hostProfileRaw...), policy: policy, tenantScope: tenantScope, hostID: hostID,
		}, nil
	}
	envelope, err := parseCanonicalObject(request.EnvelopeRaw, "execution context envelope")
	if err != nil {
		return empty, err
	}
	requiredActions, err := stringArray(envelope["requiredApprovals"], "envelope requiredApprovals")
	if err != nil {
		return empty, err
	}
	approvalExpectation := ApprovalExpectation{
		CaseID:          request.CaseID,
		PlanID:          request.PlanID,
		PlanDigest:      binding.PlanDigest,
		EnvelopeID:      request.EnvelopeID,
		EnvelopeDigest:  binding.EnvelopeDigest,
		TenantScope:     tenantScope,
		HostID:          hostID,
		ApplicationID:   request.Release.ApplicationID,
		RequiredActions: requiredActions,
		Requirements:    policy.ApprovalRequirements(),
		Now:             now,
		ClockSkew:       config.ClockSkew,
		MaxValidity:     config.ApprovalMaxValidity,
	}
	approvalRaw, err := ConsumeAndVerifyApprovalRecords(
		ctx, config.ControlPlaneURL, config.TLSConfig, request.JobID,
		request.Approvals, approvalExpectation,
	)
	if err != nil {
		return empty, fmt.Errorf("case-bound approval preflight: %w", err)
	}
	return PreparedExecution{
		valid: true, binding: binding, release: release, rollbackRaw: append([]byte(nil), rollbackRaw...),
		envelopeRaw:    append([]byte(nil), request.EnvelopeRaw...),
		planRaw:        append([]byte(nil), request.PlanRaw...),
		hostProfileRaw: append([]byte(nil), hostProfileRaw...),
		policy:         policy, approvalRaw: approvalRaw, tenantScope: tenantScope, hostID: hostID,
	}, nil
}
