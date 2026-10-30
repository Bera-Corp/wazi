// Package agentclient joins verified preflight, durable intent journaling, and a
// typed capability boundary. Runtime implementations must bind that boundary
// to the approved rootless daemon and must not expose shell execution.
package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/operationjournal"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type Preflight func(context.Context) (PreparedExecution, error)

// Boundary is the only mutation interface accepted by Executor. An
// implementation must verify the approved rootless runtime immediately before
// each mutation and execute only the closed typed capability set below.
type Boundary interface {
	VerifyRootless(context.Context) error
	Execute(context.Context, PreparedExecution, Step) (Outcome, error)
}

type Config struct {
	Journal   *operationjournal.Journal
	Boundary  Boundary
	Preflight Preflight
}

type Executor struct {
	journal   *operationjournal.Journal
	boundary  Boundary
	preflight Preflight
}

type Request struct {
	JobID  string
	CaseID string
}

type Outcome struct {
	State string
	Code  string
}

type Step struct {
	ID         string
	Capability string
	Parameters Parameters
}

// Parameters is a closed tagged union. Exactly one pointer must be non-nil,
// and it must match Step.Capability.
type Parameters struct {
	HostProbe      *HostProbeParameters
	ManifestVerify *ManifestVerifyParameters
	ComposePull    *ComposePullParameters
	ComposeUp      *ComposeUpParameters
	ComposeDown    *ComposeDownParameters
	ComposePS      *ComposePSParameters
	ComposeLogs    *ComposeLogsParameters
	VolumeBackup   *VolumeParameters
	VolumeRestore  *VolumeParameters
	HealthCheck    *HealthCheckParameters
	ProxyConfigure *ProxyConfigureParameters
	ProxyRemove    *ProxyRemoveParameters
	SecretInject   *SecretInjectParameters
}

type HostProbeParameters struct {
	Checks []string `json:"checks"`
}
type ManifestVerifyParameters struct {
	ManifestDigest     string `json:"manifestDigest"`
	RequireReviewState string `json:"requireReviewState"`
}
type ComposePullParameters struct {
	Service     string `json:"service"`
	ImageDigest string `json:"imageDigest"`
}
type ComposeUpParameters struct {
	Service               string `json:"service"`
	ProjectName           string `json:"projectName"`
	ComposeArtifactDigest string `json:"composeArtifactDigest"`
}
type ComposeDownParameters struct {
	Service       string `json:"service"`
	ProjectName   string `json:"projectName"`
	RemoveVolumes bool   `json:"removeVolumes,omitempty"`
}
type ComposePSParameters struct {
	Service     string `json:"service"`
	ProjectName string `json:"projectName"`
}
type ComposeLogsParameters struct {
	Service          string `json:"service"`
	ProjectName      string `json:"projectName"`
	SinceMinutes     int    `json:"sinceMinutes,omitempty"`
	RedactionProfile string `json:"redactionProfile"`
}
type VolumeParameters struct {
	Volume   string `json:"volume"`
	BackupID string `json:"backupId"`
}
type HealthCheckParameters struct {
	Target         string `json:"target"`
	ExpectedStatus int    `json:"expectedStatus,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
}
type ProxyConfigureParameters struct {
	Hostname     string `json:"hostname"`
	UpstreamPort int    `json:"upstreamPort"`
	TLS          string `json:"tls,omitempty"`
	Exposure     string `json:"exposure"`
}
type ProxyRemoveParameters struct {
	Hostname string `json:"hostname"`
}
type SecretInjectParameters struct {
	SecretRef string `json:"secretRef"`
	TargetEnv string `json:"targetEnv"`
	Service   string `json:"service"`
}

type planDocument struct {
	PlanID             string              `json:"planId"`
	EnvelopeID         string              `json:"envelopeId"`
	Operation          string              `json:"operation"`
	ComposeArtifactRef planRecordReference `json:"composeArtifactRef"`
	Steps              []planStep          `json:"steps"`
}

type planRecordReference struct {
	Digest string `json:"digest"`
}

type planStep struct {
	StepID     string          `json:"stepId"`
	Capability string          `json:"capability"`
	Parameters json.RawMessage `json:"parameters"`
}

func New(config Config) (*Executor, error) {
	if config.Journal == nil || config.Boundary == nil || config.Preflight == nil {
		return nil, fmt.Errorf("executor requires a durable journal, rootless boundary, and preflight function")
	}
	return &Executor{journal: config.Journal, boundary: config.Boundary, preflight: config.Preflight}, nil
}

// Execute accepts a job only after verified preflight, durably records its
// identity, and re-runs the complete preflight before each mutating step.
// Ambiguous boundary results leave an unresolved journal intent for explicit
// reconciliation and cannot advance to another step.
func (e *Executor) Execute(ctx context.Context, request Request) (operationjournal.Snapshot, error) {
	if e == nil || e.journal == nil || e.boundary == nil || e.preflight == nil {
		return operationjournal.Snapshot{}, fmt.Errorf("executor is not configured")
	}
	if request.JobID == "" || request.CaseID == "" || !strings.HasPrefix(request.JobID, "job_") {
		return operationjournal.Snapshot{}, fmt.Errorf("executor request requires a job and case identity")
	}
	prepared, err := e.preflight(ctx)
	if err != nil {
		return operationjournal.Snapshot{}, fmt.Errorf("execution preflight: %w", err)
	}
	if !prepared.Verified() {
		return operationjournal.Snapshot{}, fmt.Errorf("execution preflight returned an unverified value")
	}
	plan, appID, err := decodePrepared(prepared)
	if err != nil {
		return operationjournal.Snapshot{}, err
	}
	identity, err := makeIdentity(request, prepared, plan, appID)
	if err != nil {
		return operationjournal.Snapshot{}, err
	}
	operation, _, err := e.journal.Begin(identity)
	if err != nil {
		return operationjournal.Snapshot{}, fmt.Errorf("record operation: %w", err)
	}
	operation, err = e.journal.Start(operation.Identity.OperationID)
	if err != nil {
		return e.snapshot(operation.Identity.OperationID), fmt.Errorf("start operation: %w", err)
	}

	for _, rawStep := range plan.Steps {
		step, parametersValue, err := decodeStep(rawStep)
		if err != nil {
			return e.fail(operation.Identity.OperationID, "plan.step_invalid", err)
		}
		mutating := isMutating(step.Capability)
		if mutating {
			fresh, preflightErr := e.preflight(ctx)
			if preflightErr != nil {
				return e.fail(operation.Identity.OperationID, "preflight.stale", preflightErr)
			}
			if !fresh.Verified() || !sameAuthorization(prepared, fresh) {
				return e.fail(operation.Identity.OperationID, "preflight.changed", fmt.Errorf("verified authorization changed before mutation"))
			}
			if err := e.boundary.VerifyRootless(ctx); err != nil {
				return e.fail(operation.Identity.OperationID, "runtime.boundary_unavailable", err)
			}
			resourceRef, err := resourceReference(step)
			if err != nil {
				return e.fail(operation.Identity.OperationID, "plan.resource_invalid", err)
			}
			parameterDigest, err := verify.CanonicalDigest(parametersValue)
			if err != nil {
				return e.fail(operation.Identity.OperationID, "plan.parameters_invalid", err)
			}
			if _, err := e.journal.RecordIntent(operation.Identity.OperationID, step.ID, step.Capability, resourceRef, parameterDigest); err != nil {
				return e.snapshot(operation.Identity.OperationID), fmt.Errorf("persist mutation intent before %s: %w", step.ID, err)
			}
		}

		outcome, executeErr := e.boundary.Execute(ctx, prepared, step)
		if mutating {
			if executeErr != nil {
				_, recordErr := e.journal.RecordOutcome(operation.Identity.OperationID, step.ID, "ambiguous", "runtime.ambiguous")
				return e.snapshot(operation.Identity.OperationID), errors.Join(fmt.Errorf("runtime result is ambiguous for %s: %w", step.ID, executeErr), recordErr)
			}
			if outcome.State != "succeeded" && outcome.State != "failed" && outcome.State != "ambiguous" {
				outcome = Outcome{State: "ambiguous", Code: "runtime.invalid_result"}
			}
			code := ""
			if outcome.State == "failed" {
				code = "runtime.step_failed"
			} else if outcome.State == "ambiguous" {
				code = "runtime.ambiguous"
			}
			if _, err := e.journal.RecordOutcome(operation.Identity.OperationID, step.ID, outcome.State, code); err != nil {
				return e.snapshot(operation.Identity.OperationID), fmt.Errorf("persist mutation outcome for %s: %w", step.ID, err)
			}
			if outcome.State != "succeeded" {
				if outcome.State == "ambiguous" {
					return e.snapshot(operation.Identity.OperationID), fmt.Errorf("runtime result is ambiguous for %s; operation requires reconciliation", step.ID)
				}
				return e.fail(operation.Identity.OperationID, "runtime.step_failed", fmt.Errorf("runtime rejected %s with code %s", step.ID, code))
			}
		} else if executeErr != nil {
			return e.fail(operation.Identity.OperationID, "runtime.read_failed", executeErr)
		} else if outcome.State != "succeeded" {
			return e.fail(operation.Identity.OperationID, "runtime.read_failed", fmt.Errorf("runtime did not verify read-only step %s", step.ID))
		}
	}

	if _, err := e.journal.Complete(operation.Identity.OperationID); err != nil {
		return e.snapshot(operation.Identity.OperationID), fmt.Errorf("complete journal operation: %w", err)
	}
	return e.snapshot(operation.Identity.OperationID), nil
}

func decodePrepared(prepared PreparedExecution) (planDocument, string, error) {
	var plan planDocument
	planRaw := prepared.Plan()
	if err := requireCanonical(planRaw, "deployment plan"); err != nil {
		return plan, "", err
	}
	if err := contracts.ValidateDeploymentPlan(planRaw); err != nil {
		return plan, "", fmt.Errorf("verified deployment plan failed schema validation: %w", err)
	}
	if err := json.Unmarshal(planRaw, &plan); err != nil {
		return plan, "", fmt.Errorf("decode verified deployment plan: %w", err)
	}
	if plan.PlanID == "" || plan.EnvelopeID == "" || plan.Operation == "" || len(plan.Steps) == 0 {
		return plan, "", fmt.Errorf("verified deployment plan is incomplete")
	}
	if err := requireCanonical(prepared.Envelope(), "execution envelope"); err != nil {
		return plan, "", err
	}
	var envelope struct {
		EnvelopeID string `json:"envelopeId"`
	}
	if err := json.Unmarshal(prepared.Envelope(), &envelope); err != nil || envelope.EnvelopeID != plan.EnvelopeID {
		return plan, "", fmt.Errorf("verified plan does not match its envelope identity")
	}
	if err := requireCanonical(prepared.Manifest(), "release manifest"); err != nil {
		return plan, "", err
	}
	var manifest struct {
		AppID string `json:"appId"`
	}
	if err := json.Unmarshal(prepared.Manifest(), &manifest); err != nil || manifest.AppID == "" {
		return plan, "", fmt.Errorf("verified release manifest has no application identity")
	}
	if err := validateStepsAgainstManifest(prepared, plan); err != nil {
		return plan, "", err
	}
	return plan, manifest.AppID, nil
}

func validateStepsAgainstManifest(prepared PreparedExecution, plan planDocument) error {
	manifest, err := parseCanonicalObject(prepared.Manifest(), "verified release manifest")
	if err != nil {
		return err
	}
	release, ok := objectValue(manifest["release"])
	if !ok {
		return fmt.Errorf("verified release manifest has no release object")
	}
	images, err := optionalObjects(release["images"], "manifest release images")
	if err != nil {
		return err
	}
	volumes, err := optionalObjects(release["volumes"], "manifest release volumes")
	if err != nil {
		return err
	}
	ports, err := optionalObjects(release["ports"], "manifest release ports")
	if err != nil {
		return err
	}
	for _, raw := range plan.Steps {
		step, _, err := decodeStep(raw)
		if err != nil {
			return fmt.Errorf("decode verified plan step: %w", err)
		}
		if err := validateStepAgainstManifest(step, images, volumes, ports, release, plan.ComposeArtifactRef.Digest, prepared.Binding().Release.ManifestDigest, manifest["reviewState"]); err != nil {
			return fmt.Errorf("plan step %q is not bound to the verified manifest: %w", step.ID, err)
		}
	}
	return nil
}

func optionalObjects(value any, label string) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	return stringObjects(value, label)
}

func validateStepAgainstManifest(step Step, images, volumes, ports []map[string]any, release map[string]any, artifactDigest, manifestDigest string, manifestReviewState any) error {
	serviceExists := func(service string) bool {
		for _, image := range images {
			if image["service"] == service {
				return true
			}
		}
		return false
	}
	volumeIsBackup := func(name string) bool {
		for _, volume := range volumes {
			if volume["name"] == name && volume["backup"] == true {
				return true
			}
		}
		return false
	}
	switch step.Capability {
	case "manifest.verify":
		if step.Parameters.ManifestVerify.ManifestDigest != manifestDigest || step.Parameters.ManifestVerify.RequireReviewState != manifestReviewState {
			return fmt.Errorf("manifest verification target does not match the verified manifest")
		}
	case "compose.pull":
		for _, image := range images {
			if image["service"] == step.Parameters.ComposePull.Service && image["digest"] == step.Parameters.ComposePull.ImageDigest {
				return nil
			}
		}
		return fmt.Errorf("image pull service or digest is not in the verified manifest")
	case "compose.up":
		if !serviceExists(step.Parameters.ComposeUp.Service) || artifactDigest == "" || step.Parameters.ComposeUp.ComposeArtifactDigest != artifactDigest {
			return fmt.Errorf("compose up service or artifact digest is not bound to the verified release")
		}
	case "compose.down":
		if !serviceExists(step.Parameters.ComposeDown.Service) || step.Parameters.ComposeDown.RemoveVolumes {
			return fmt.Errorf("compose down target is unknown or requests volume deletion")
		}
	case "compose.ps":
		if !serviceExists(step.Parameters.ComposePS.Service) {
			return fmt.Errorf("compose status service is not in the verified manifest")
		}
	case "compose.logs-redacted":
		if !serviceExists(step.Parameters.ComposeLogs.Service) {
			return fmt.Errorf("compose logs service is not in the verified manifest")
		}
	case "volume.backup":
		if !volumeIsBackup(step.Parameters.VolumeBackup.Volume) {
			return fmt.Errorf("backup volume is not declared backup-capable by the verified manifest")
		}
	case "volume.restore":
		if !volumeIsBackup(step.Parameters.VolumeRestore.Volume) {
			return fmt.Errorf("restore volume is not declared backup-capable by the verified manifest")
		}
	case "health.check":
		health, _ := objectValue(release["healthCheck"])
		if step.Parameters.HealthCheck.Target == "" || health["target"] != step.Parameters.HealthCheck.Target {
			return fmt.Errorf("health check target does not match the verified manifest")
		}
	case "proxy.configure":
		allowedPort := false
		for _, port := range ports {
			if port["hostPort"] == json.Number(fmt.Sprint(step.Parameters.ProxyConfigure.UpstreamPort)) && port["exposure"] == "private" {
				allowedPort = true
			}
		}
		if !allowedPort || step.Parameters.ProxyConfigure.Exposure != "private" {
			return fmt.Errorf("proxy target is not a private manifest port")
		}
	case "secret.inject":
		environment, err := optionalObjects(release["environment"], "manifest release environment")
		if err != nil {
			return err
		}
		matched := false
		for _, item := range environment {
			if item["name"] == step.Parameters.SecretInject.TargetEnv && item["service"] == step.Parameters.SecretInject.Service && item["secretRef"] == step.Parameters.SecretInject.SecretRef {
				matched = true
			}
		}
		if !matched || !serviceExists(step.Parameters.SecretInject.Service) {
			return fmt.Errorf("secret target does not match the verified manifest")
		}
	case "host.probe", "proxy.remove":
		// These capabilities carry no release-specific image or volume target.
	default:
		return fmt.Errorf("unsupported capability %q", step.Capability)
	}
	return nil
}

func requireCanonical(raw []byte, label string) error {
	if len(raw) == 0 {
		return fmt.Errorf("verified %s is empty", label)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return fmt.Errorf("parse verified %s: %w", label, err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return fmt.Errorf("verified %s is not canonical JSON", label)
	}
	return nil
}

func makeIdentity(request Request, prepared PreparedExecution, plan planDocument, appID string) (operationjournal.Identity, error) {
	binding := prepared.Binding()
	if binding.Release.ManifestDigest == "" || binding.PlanDigest == "" || binding.EnvelopeDigest == "" {
		return operationjournal.Identity{}, fmt.Errorf("verified preflight binding is incomplete")
	}
	jobSuffix := strings.TrimPrefix(request.JobID, "job_")
	return operationjournal.Identity{
		OperationID: "op_" + jobSuffix, JobID: request.JobID, CaseID: request.CaseID,
		PlanID: plan.PlanID, PlanDigest: binding.PlanDigest, EnvelopeID: plan.EnvelopeID,
		EnvelopeDigest: binding.EnvelopeDigest, TenantScope: prepared.TenantScope(), HostID: prepared.HostID(),
		ApplicationID: appID, ManifestDigest: binding.Release.ManifestDigest, Operation: plan.Operation,
	}, nil
}

func decodeStep(raw planStep) (Step, any, error) {
	if raw.StepID == "" || raw.Capability == "" || len(raw.Parameters) == 0 {
		return Step{}, nil, fmt.Errorf("deployment plan step is incomplete")
	}
	var parameters any
	step := Step{ID: raw.StepID, Capability: raw.Capability}
	decode := func(target any) error {
		decoder := json.NewDecoder(bytes.NewReader(raw.Parameters))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			if err == nil {
				return fmt.Errorf("parameters contain multiple JSON values")
			}
			return err
		}
		value, err := verify.ParseJSON(raw.Parameters)
		if err != nil {
			return err
		}
		parameters = value
		return nil
	}
	switch raw.Capability {
	case "host.probe":
		var value HostProbeParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.HostProbe = &value
	case "manifest.verify":
		var value ManifestVerifyParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ManifestVerify = &value
	case "compose.pull":
		var value ComposePullParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ComposePull = &value
	case "compose.up":
		var value ComposeUpParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ComposeUp = &value
	case "compose.down":
		var value ComposeDownParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ComposeDown = &value
	case "compose.ps":
		var value ComposePSParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ComposePS = &value
	case "compose.logs-redacted":
		var value ComposeLogsParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ComposeLogs = &value
	case "volume.backup":
		var value VolumeParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.VolumeBackup = &value
	case "volume.restore":
		var value VolumeParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.VolumeRestore = &value
	case "health.check":
		var value HealthCheckParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.HealthCheck = &value
	case "proxy.configure":
		var value ProxyConfigureParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ProxyConfigure = &value
	case "proxy.remove":
		var value ProxyRemoveParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.ProxyRemove = &value
	case "secret.inject":
		var value SecretInjectParameters
		if err := decode(&value); err != nil {
			return Step{}, nil, err
		}
		step.Parameters.SecretInject = &value
	default:
		return Step{}, nil, fmt.Errorf("unsupported typed capability %q", raw.Capability)
	}
	return step, parameters, nil
}

func isMutating(capability string) bool {
	switch capability {
	case "compose.pull", "compose.up", "compose.down", "volume.backup", "volume.restore", "proxy.configure", "proxy.remove", "secret.inject":
		return true
	default:
		return false
	}
}

func resourceReference(step Step) (string, error) {
	ref := ""
	switch step.Capability {
	case "compose.pull":
		ref = "image:" + step.Parameters.ComposePull.Service
	case "compose.up":
		ref = "compose-project:" + step.Parameters.ComposeUp.ProjectName
	case "compose.down":
		if step.Parameters.ComposeDown.RemoveVolumes {
			return "", fmt.Errorf("volume removal is not enabled by the Phase 1 executor")
		}
		ref = "compose-project:" + step.Parameters.ComposeDown.ProjectName
	case "volume.backup":
		ref = "volume:" + step.Parameters.VolumeBackup.Volume
	case "volume.restore":
		ref = "volume:" + step.Parameters.VolumeRestore.Volume
	case "proxy.configure":
		ref = "proxy:" + step.Parameters.ProxyConfigure.Hostname
	case "proxy.remove":
		ref = "proxy:" + step.Parameters.ProxyRemove.Hostname
	case "secret.inject":
		ref = "secret:" + strings.ReplaceAll(step.Parameters.SecretInject.SecretRef, "/", ":")
	default:
		return "", fmt.Errorf("capability %q has no mutation resource reference", step.Capability)
	}
	if ref == "" || strings.Contains(ref, "..") || strings.Contains(ref, "//") {
		return "", fmt.Errorf("capability %q has an unsafe mutation resource reference", step.Capability)
	}
	return ref, nil
}

func sameAuthorization(left, right PreparedExecution) bool {
	if !left.Verified() || !right.Verified() || left.TenantScope() != right.TenantScope() || left.HostID() != right.HostID() {
		return false
	}
	if left.Binding() != right.Binding() {
		return false
	}
	leftPolicy := left.CapabilityPolicy().BindingSnapshot()
	rightPolicy := right.CapabilityPolicy().BindingSnapshot()
	return leftPolicy.BundleID == rightPolicy.BundleID && leftPolicy.Version == rightPolicy.Version &&
		leftPolicy.Digest == rightPolicy.Digest && leftPolicy.TenantScope == rightPolicy.TenantScope &&
		equalStrings(leftPolicy.AllowedCapabilities, rightPolicy.AllowedCapabilities) &&
		equalStrings(leftPolicy.DeniedCapabilities, rightPolicy.DeniedCapabilities)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (e *Executor) fail(operationID, code string, cause error) (operationjournal.Snapshot, error) {
	if _, err := e.journal.Fail(operationID, code); err != nil {
		return e.snapshot(operationID), errors.Join(cause, fmt.Errorf("record operation failure: %w", err))
	}
	return e.snapshot(operationID), cause
}

func (e *Executor) snapshot(operationID string) operationjournal.Snapshot {
	snapshot, _ := e.journal.Get(operationID)
	return snapshot
}
