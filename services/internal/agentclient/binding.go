package agentclient

import (
	"bytes"
	"fmt"
	"reflect"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// ExecutionBindingExpectation is the caller-authenticated scope for one
// preflight. TenantScope and the record IDs must come from the control-plane
// request or another authenticated source, not from the supplied documents.
type ExecutionBindingExpectation struct {
	Release     ReleaseExpectation
	TenantScope string
	HostID      string
	Policy      AuthenticatedCapabilityPolicy
	EnvelopeID  string
	PlanID      string
	Now         time.Time
	ClockSkew   time.Duration
}

// AuthenticatedCapabilityPolicy is the operator-authenticated policy snapshot
// corresponding to the envelope policyBundleRef. Callers must not derive it
// from the envelope, plan, manifest, or procedure being checked.
type AuthenticatedCapabilityPolicy struct {
	BundleID            string
	Version             string
	Digest              string
	TenantScope         string
	AllowedCapabilities []string
	DeniedCapabilities  []string
}

// VerifiedExecutionBinding exposes the exact digests a future approval must
// bind. It does not itself establish approval, host freshness, artifact-byte
// validity, runtime authority, or permission to execute.
type VerifiedExecutionBinding struct {
	Release                          VerifiedRelease
	EnvelopeDigest                   string
	PlanDigest                       string
	RollbackProcedureDigest          string
	HostProfileDigest                string
	ContributionBudgetDigest         string
	ManagedVolumeQualificationDigest string
}

// VerifyExecutionBinding verifies signatures and schemas for the manifest
// and procedures, then binds the plan to the exact envelope, release,
// procedures, and typed capability contracts. This is a read-only preflight;
// it is not a mutation executor or approval check.
func VerifyExecutionBinding(
	manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw []byte,
	policy verify.Policy,
	expected ExecutionBindingExpectation,
) (VerifiedExecutionBinding, error) {
	return verifyExecutionBinding(manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw, policy, expected, contracts.ValidateExecutionContextEnvelope)
}

func verifyExecutionBinding(
	manifestRaw, procedureRaw, rollbackProcedureRaw, envelopeRaw, planRaw []byte,
	policy verify.Policy,
	expected ExecutionBindingExpectation,
	validateEnvelope func([]byte) error,
) (VerifiedExecutionBinding, error) {
	var result VerifiedExecutionBinding
	if err := validateExecutionBindingExpectation(expected); err != nil {
		return result, err
	}
	release, err := VerifyReleaseRecords(manifestRaw, procedureRaw, policy, expected.Release)
	if err != nil {
		return result, err
	}
	if err := validateEnvelope(envelopeRaw); err != nil {
		return result, fmt.Errorf("execution context envelope schema rejected: %w", err)
	}
	if err := contracts.ValidateDeploymentPlan(planRaw); err != nil {
		return result, fmt.Errorf("deployment plan schema rejected: %w", err)
	}
	envelope, err := parseCanonicalObject(envelopeRaw, "execution context envelope")
	if err != nil {
		return result, err
	}
	plan, err := parseCanonicalObject(planRaw, "deployment plan")
	if err != nil {
		return result, err
	}
	manifest, err := parseCanonicalObject(manifestRaw, "signed manifest")
	if err != nil {
		return result, err
	}
	procedure, err := parseCanonicalObject(procedureRaw, "signed procedure")
	if err != nil {
		return result, err
	}
	if err := bindEnvelopeToRelease(envelope, plan, manifest, procedure, release, expected); err != nil {
		return result, err
	}
	if err := validateRecordWindow(envelope, "execution context envelope", expected.Now, expected.ClockSkew); err != nil {
		return result, err
	}
	if err := validateRecordWindow(plan, "deployment plan", expected.Now, expected.ClockSkew); err != nil {
		return result, err
	}
	rollbackProcedure, err := verifyRollbackProcedure(rollbackProcedureRaw, envelope, procedure, expected, policy)
	if err != nil {
		return result, err
	}
	if err := bindPlanSteps(envelope, plan, procedure, rollbackProcedure); err != nil {
		return result, err
	}
	if err := bindPlanApprovals(envelope, plan); err != nil {
		return result, err
	}
	result.Release = release
	result.EnvelopeDigest, err = verify.CanonicalDigest(envelope)
	if err != nil {
		return VerifiedExecutionBinding{}, fmt.Errorf("digest execution context envelope: %w", err)
	}
	result.PlanDigest, err = verify.CanonicalDigest(plan)
	if err != nil {
		return VerifiedExecutionBinding{}, fmt.Errorf("digest deployment plan: %w", err)
	}
	if rollbackProcedure != nil {
		result.RollbackProcedureDigest, err = verify.CanonicalDigest(rollbackProcedure)
		if err != nil {
			return VerifiedExecutionBinding{}, fmt.Errorf("digest rollback procedure: %w", err)
		}
	}
	return result, nil
}

func validateExecutionBindingExpectation(expected ExecutionBindingExpectation) error {
	if err := validateReleaseExpectation(expected.Release); err != nil {
		return err
	}
	policy := expected.Policy
	if expected.TenantScope == "" || expected.EnvelopeID == "" || expected.PlanID == "" ||
		policy.BundleID == "" || policy.Version == "" || policy.Digest == "" || policy.TenantScope == "" ||
		len(policy.AllowedCapabilities) == 0 {
		return fmt.Errorf("execution binding expectation must identify tenant, policy, envelope, and plan")
	}
	if expected.Now.IsZero() || expected.ClockSkew < 0 {
		return fmt.Errorf("execution binding expectation requires a current time and non-negative clock skew")
	}
	return nil
}

func parseCanonicalObject(raw []byte, label string) (map[string]any, error) {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", label, err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize %s: %w", label, err)
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

func bindEnvelopeToRelease(envelope, plan, manifest, procedure map[string]any, release VerifiedRelease, expected ExecutionBindingExpectation) error {
	if envelope["envelopeId"] != expected.EnvelopeID || plan["planId"] != expected.PlanID {
		return fmt.Errorf("envelope or plan identity does not match the authenticated request")
	}
	if envelope["tenantScope"] != expected.TenantScope {
		return fmt.Errorf("envelope tenant does not match the authenticated request")
	}
	if expected.Policy.TenantScope != expected.TenantScope {
		return fmt.Errorf("authenticated policy tenant does not match the request")
	}
	if err := requireRecordReference(envelope["policyBundleRef"], expected.Policy.BundleID, expected.Policy.Digest, expected.Policy.Version, "envelope policyBundleRef"); err != nil {
		return err
	}
	if envelope["applicationId"] != expected.Release.ApplicationID ||
		envelope["releaseVersion"] != expected.Release.ReleaseVersion ||
		envelope["operation"] != expected.Release.Operation {
		return fmt.Errorf("envelope identity does not match the requested application release and operation")
	}
	if envelope["manifestDigest"] != release.ManifestDigest ||
		envelope["manifestReviewState"] != manifest["reviewState"] {
		return fmt.Errorf("envelope manifest digest or review state does not match the signed manifest")
	}
	if plan["envelopeId"] != envelope["envelopeId"] || plan["operation"] != envelope["operation"] {
		return fmt.Errorf("deployment plan envelope or operation does not match the execution context")
	}
	if procedure["procedureId"] != expected.Release.ProcedureID ||
		procedure["version"] != expected.Release.ProcedureVersion ||
		procedure["operation"] != envelope["operation"] ||
		procedure["applicationId"] != envelope["applicationId"] {
		return fmt.Errorf("signed procedure identity does not match the execution context")
	}
	if err := requireRecordReference(envelope["procedureRef"], expected.Release.ProcedureID, release.ProcedureDigest, expected.Release.ProcedureVersion, "envelope procedureRef"); err != nil {
		return err
	}
	if !sameRecordReference(envelope["composeArtifactRef"], plan["composeArtifactRef"]) {
		return fmt.Errorf("deployment plan composeArtifactRef does not exactly match the envelope")
	}
	policyAllowed := make(map[string]struct{}, len(expected.Policy.AllowedCapabilities))
	for _, capability := range expected.Policy.AllowedCapabilities {
		policyAllowed[capability] = struct{}{}
	}
	policyDenied := make(map[string]struct{}, len(expected.Policy.DeniedCapabilities))
	for _, capability := range expected.Policy.DeniedCapabilities {
		policyDenied[capability] = struct{}{}
	}
	envelopeAllowed, err := stringSet(envelope["allowedCapabilities"], "envelope allowedCapabilities")
	if err != nil {
		return err
	}
	for capability := range envelopeAllowed {
		if _, ok := policyAllowed[capability]; !ok {
			return fmt.Errorf("envelope capability %q exceeds authenticated policy", capability)
		}
		if _, denied := policyDenied[capability]; denied {
			return fmt.Errorf("envelope capability %q is denied by authenticated policy", capability)
		}
	}
	return nil
}

func verifyRollbackProcedure(raw []byte, envelope, procedure map[string]any, expected ExecutionBindingExpectation, policy verify.Policy) (map[string]any, error) {
	strategy, ok := objectValue(envelope["rollbackStrategy"])
	if !ok {
		return nil, fmt.Errorf("execution context rollbackStrategy is invalid")
	}
	supported, ok := strategy["supported"].(bool)
	if !ok {
		return nil, fmt.Errorf("execution context rollback support is invalid")
	}
	if !supported {
		if len(raw) != 0 {
			return nil, fmt.Errorf("unreferenced rollback procedure was supplied")
		}
		return nil, nil
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("supported rollback requires its separately signed procedure")
	}
	if err := verify.VerifyRecord(raw, policy); err != nil {
		return nil, fmt.Errorf("rollback procedure signature rejected: %w", err)
	}
	if err := contracts.ValidateProcedure(raw); err != nil {
		return nil, fmt.Errorf("rollback procedure schema rejected: %w", err)
	}
	rollbackProcedure, err := parseCanonicalObject(raw, "signed rollback procedure")
	if err != nil {
		return nil, err
	}
	if rollbackProcedure["operation"] != "rollback" || rollbackProcedure["applicationId"] != expected.Release.ApplicationID {
		return nil, fmt.Errorf("rollback procedure operation or application does not match the request")
	}
	rollbackID, rollbackVersion, err := referenceIdentity(strategy["procedureRef"], "rollbackStrategy.procedureRef")
	if err != nil {
		return nil, err
	}
	rollbackDigest, err := verify.CanonicalDigest(rollbackProcedure)
	if err != nil {
		return nil, fmt.Errorf("digest signed rollback procedure: %w", err)
	}
	if err := requireRecordReference(strategy["procedureRef"], rollbackID, rollbackDigest, rollbackVersion, "rollbackStrategy.procedureRef"); err != nil {
		return nil, err
	}
	if rollbackProcedure["procedureId"] != rollbackID || rollbackProcedure["version"] != rollbackVersion {
		return nil, fmt.Errorf("rollback procedure identity does not match its envelope reference")
	}
	if declared, exists := procedure["rollbackProcedureId"]; exists && declared != rollbackID {
		return nil, fmt.Errorf("main procedure rollbackProcedureId does not match the envelope rollback reference")
	}
	return rollbackProcedure, nil
}

func bindPlanSteps(envelope, plan, procedure, rollbackProcedure map[string]any) error {
	steps, err := stringObjects(plan["steps"], "deployment plan steps")
	if err != nil {
		return err
	}
	rollbackSteps, err := stringObjects(plan["rollbackSteps"], "deployment plan rollbackSteps")
	if err != nil {
		return err
	}
	strategy, _ := objectValue(envelope["rollbackStrategy"])
	supported, _ := strategy["supported"].(bool)
	if supported {
		if rollbackProcedure == nil || len(rollbackSteps) == 0 {
			return fmt.Errorf("supported rollback requires a signed rollback procedure and non-empty rollback plan")
		}
	} else if len(rollbackSteps) != 0 {
		return fmt.Errorf("rollback steps are present but the envelope does not support rollback")
	}
	allowed, err := stringSet(envelope["allowedCapabilities"], "envelope allowedCapabilities")
	if err != nil {
		return err
	}
	forbidden, err := stringSet(envelope["forbiddenOperations"], "envelope forbiddenOperations")
	if err != nil {
		return err
	}
	for _, sequence := range [][]map[string]any{steps, rollbackSteps} {
		for _, step := range sequence {
			capability, _ := step["capability"].(string)
			if _, ok := allowed[capability]; !ok {
				return fmt.Errorf("plan capability %q is not allowed by the envelope", capability)
			}
			if _, ok := forbidden[capability]; ok {
				return fmt.Errorf("plan capability %q is forbidden by the envelope", capability)
			}
		}
	}
	if questions, ok := plan["unresolvedQuestions"].([]any); ok && len(questions) != 0 {
		return fmt.Errorf("deployment plan has unresolved questions")
	}
	if err := bindProcedureSteps(steps, procedure, "forward"); err != nil {
		return err
	}
	if rollbackProcedure != nil {
		if err := bindProcedureSteps(rollbackSteps, rollbackProcedure, "rollback"); err != nil {
			return err
		}
	}
	if err := validateStepReferences(steps, rollbackSteps); err != nil {
		return err
	}
	return nil
}

func bindProcedureSteps(steps []map[string]any, procedure map[string]any, label string) error {
	sequence, err := stringArray(procedure["capabilitySequence"], label+" procedure capabilitySequence")
	if err != nil {
		return err
	}
	if len(sequence) != len(steps) {
		return fmt.Errorf("%s plan steps do not match the signed procedure sequence length", label)
	}
	contracts, ok := procedure["parameterContracts"].([]any)
	if !ok {
		return fmt.Errorf("%s procedure parameterContracts is invalid", label)
	}
	contractByCapability := make(map[string]map[string]any, len(contracts))
	for _, rawContract := range contracts {
		contract, ok := objectValue(rawContract)
		if !ok {
			return fmt.Errorf("%s procedure contains an invalid parameter contract", label)
		}
		capability, _ := contract["capability"].(string)
		if _, duplicate := contractByCapability[capability]; duplicate {
			return fmt.Errorf("%s procedure repeats a parameter contract for %q", label, capability)
		}
		contractByCapability[capability] = contract
	}
	if len(contractByCapability) != len(sequence) {
		return fmt.Errorf("%s procedure parameterContracts must cover exactly the capability sequence", label)
	}
	for index, step := range steps {
		capability, _ := step["capability"].(string)
		if capability != sequence[index] {
			return fmt.Errorf("%s plan capability order does not match the signed procedure", label)
		}
		contract, ok := contractByCapability[capability]
		if !ok {
			return fmt.Errorf("%s procedure lacks a parameter contract for %q", label, capability)
		}
		parameters, ok := objectValue(step["parameters"])
		if !ok {
			return fmt.Errorf("plan step parameters for %q are invalid", capability)
		}
		required, err := stringArray(contract["requiredParameters"], label+" required parameters")
		if err != nil {
			return err
		}
		for _, name := range required {
			if _, exists := parameters[name]; !exists {
				return fmt.Errorf("plan step %q lacks procedure-required parameter %q", capability, name)
			}
		}
		if fixed, exists := contract["fixedParameters"]; exists {
			fixedValues, ok := objectValue(fixed)
			if !ok {
				return fmt.Errorf("%s procedure fixedParameters for %q are invalid", label, capability)
			}
			for name, value := range fixedValues {
				if actual, exists := parameters[name]; !exists || !reflect.DeepEqual(actual, value) {
					return fmt.Errorf("plan step %q parameter %q differs from the signed procedure", capability, name)
				}
			}
		}
	}
	return nil
}

func bindPlanApprovals(envelope, plan map[string]any) error {
	envelopeApprovals, err := stringSet(envelope["requiredApprovals"], "envelope requiredApprovals")
	if err != nil {
		return err
	}
	planApprovals, err := stringSet(plan["requiredApprovals"], "plan requiredApprovals")
	if err != nil {
		return err
	}
	if !equalStringSets(envelopeApprovals, planApprovals) {
		return fmt.Errorf("deployment plan requiredApprovals do not match the envelope")
	}
	return nil
}

func validateStepReferences(steps, rollbackSteps []map[string]any) error {
	forwardIDs := make(map[string]struct{}, len(steps))
	rollbackIDs := make(map[string]struct{}, len(rollbackSteps))
	rollbackByID := make(map[string]map[string]any, len(rollbackSteps))
	for _, step := range steps {
		id, _ := step["stepId"].(string)
		if _, exists := forwardIDs[id]; exists {
			return fmt.Errorf("forward plan step ID %q is repeated", id)
		}
		forwardIDs[id] = struct{}{}
	}
	for _, step := range rollbackSteps {
		id, _ := step["stepId"].(string)
		if _, exists := rollbackIDs[id]; exists {
			return fmt.Errorf("rollback plan step ID %q is repeated", id)
		}
		if _, exists := forwardIDs[id]; exists {
			return fmt.Errorf("plan step ID %q is shared across forward and rollback sequences", id)
		}
		rollbackIDs[id] = struct{}{}
		rollbackByID[id] = step
		if _, exists := step["rollbackStepId"]; exists {
			return fmt.Errorf("rollback plan step %q cannot declare another rollback step", id)
		}
	}
	for _, step := range steps {
		if target, exists := step["rollbackStepId"]; exists {
			id, ok := target.(string)
			if !ok {
				return fmt.Errorf("plan step rollbackStepId is invalid")
			}
			if _, exists := rollbackIDs[id]; !exists {
				return fmt.Errorf("plan step rollbackStepId %q does not identify a rollback step", id)
			}
		}
		if capability, _ := step["capability"].(string); capabilityNeedsCompensation(capability) {
			targetID, ok := step["rollbackStepId"].(string)
			if !ok {
				return fmt.Errorf("mutating plan capability %q has no compensation step", capability)
			}
			target := rollbackByID[targetID]
			if target == nil {
				return fmt.Errorf("mutating plan capability %q has no valid compensation step", capability)
			}
			if err := validateCompensation(step, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func capabilityNeedsCompensation(capability string) bool {
	switch capability {
	case "compose.up", "compose.down", "secret.inject", "proxy.configure", "volume.restore":
		return true
	default:
		return false
	}
}

func validateCompensation(step, rollbackStep map[string]any) error {
	forwardCapability, _ := step["capability"].(string)
	rollbackCapability, _ := rollbackStep["capability"].(string)
	forwardParameters, _ := objectValue(step["parameters"])
	rollbackParameters, _ := objectValue(rollbackStep["parameters"])
	var expectedCapability string
	var resourceKeys []string
	switch forwardCapability {
	case "compose.up":
		expectedCapability, resourceKeys = "compose.down", []string{"service", "projectName"}
	case "compose.down":
		expectedCapability, resourceKeys = "compose.up", []string{"service", "projectName"}
	case "secret.inject":
		expectedCapability, resourceKeys = "compose.down", []string{"service"}
	case "proxy.configure":
		expectedCapability, resourceKeys = "proxy.remove", []string{"hostname"}
	case "volume.restore":
		expectedCapability, resourceKeys = "volume.backup", []string{"volume"}
	default:
		return nil
	}
	if rollbackCapability != expectedCapability {
		return fmt.Errorf("plan step %q requires compensation capability %q, got %q", step["stepId"], expectedCapability, rollbackCapability)
	}
	for _, key := range resourceKeys {
		if !reflect.DeepEqual(forwardParameters[key], rollbackParameters[key]) {
			return fmt.Errorf("plan step %q compensation resource %q does not match", step["stepId"], key)
		}
	}
	return nil
}

func validateRecordWindow(record map[string]any, label string, now time.Time, skew time.Duration) error {
	createdText, _ := record["createdAt"].(string)
	expiresText, _ := record["expiresAt"].(string)
	created, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return fmt.Errorf("%s createdAt is invalid: %w", label, err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresText)
	if err != nil {
		return fmt.Errorf("%s expiresAt is invalid: %w", label, err)
	}
	if !expires.After(created) {
		return fmt.Errorf("%s expiresAt must be after createdAt", label)
	}
	now = now.UTC()
	if created.After(now.Add(skew)) {
		return fmt.Errorf("%s is future-dated", label)
	}
	if !now.Before(expires) {
		return fmt.Errorf("%s is expired", label)
	}
	return nil
}

func requireRecordReference(value any, recordID, digest, version, label string) error {
	actualID, actualVersion, err := referenceIdentity(value, label)
	if err != nil {
		return err
	}
	ref, _ := objectValue(value)
	if actualID != recordID || actualVersion != version || ref["digest"] != digest {
		return fmt.Errorf("%s does not match the verified record identity and digest", label)
	}
	return nil
}

func referenceIdentity(value any, label string) (string, string, error) {
	ref, ok := objectValue(value)
	if !ok {
		return "", "", fmt.Errorf("%s is invalid", label)
	}
	id, idOK := ref["recordId"].(string)
	version, versionOK := ref["version"].(string)
	if !idOK || id == "" || !versionOK || version == "" {
		return "", "", fmt.Errorf("%s must include recordId and version", label)
	}
	return id, version, nil
}

func sameRecordReference(left, right any) bool {
	leftRef, leftOK := objectValue(left)
	rightRef, rightOK := objectValue(right)
	if !leftOK || !rightOK {
		return false
	}
	for _, key := range []string{"recordId", "digest", "version"} {
		if !reflect.DeepEqual(leftRef[key], rightRef[key]) {
			return false
		}
	}
	return true
}

func stringObjects(value any, label string) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is invalid", label)
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := objectValue(item)
		if !ok {
			return nil, fmt.Errorf("%s contains a non-object item", label)
		}
		result = append(result, object)
	}
	return result, nil
}

func stringArray(value any, label string) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is invalid", label)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s contains a non-string item", label)
		}
		result = append(result, value)
	}
	return result, nil
}

func stringSet(value any, label string) (map[string]struct{}, error) {
	values, err := stringArray(value, label)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result, nil
}

func equalStringSets(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if _, exists := right[value]; !exists {
			return false
		}
	}
	return true
}

func objectValue(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}
