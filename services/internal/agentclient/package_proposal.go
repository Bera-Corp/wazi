package agentclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func canonicalPayload(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	parsed, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, err
	}
	return verify.CanonicalJSON(parsed)
}
func contextRecordRef(raw []byte, idKey string) (map[string]any, error) {
	value, err := parseCanonicalObject(raw, idKey)
	if err != nil {
		return nil, err
	}
	digest, err := verify.CanonicalDigest(value)
	if err != nil {
		return nil, err
	}
	version := value["version"]
	if version == nil {
		version = value["schemaVersion"]
	}
	return map[string]any{"recordId": value[idKey], "digest": digest, "version": version}, nil
}
func protectedContextRecord(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 1<<20 {
		return nil, fmt.Errorf("protected package context record unavailable")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, err
	}
	return verify.CanonicalJSON(value)
}

func (r *PackageRegistry) Catalog() []map[string]any {
	result := []map[string]any{}
	if r == nil {
		return result
	}
	keys := []string{}
	for key := range r.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		config := r.entries[key]
		shape, _ := shapeFor(config.Profile, false)
		operations := []string{"install", "upgrade", "backup", "restore", "uninstall"}
		if config.RecoveryReleaseVersion != "" {
			operations = append(operations, "downgrade")
		}
		result = append(result, map[string]any{"project": key, "profile": config.Profile, "applicationId": shape.App, "releaseVersion": config.ReleaseVersion,
			"operations": operations, "scope": "operator-reviewed releases and explicitly configured migration paths"})
	}
	return result
}

func (b *ContextBridge) Propose(ctx context.Context, project, operation, backupID string) (PilotJob, error) {
	var empty PilotJob
	if b.Packages == nil {
		return empty, fmt.Errorf("package catalog disabled")
	}
	config, ok := b.Packages.entries[project]
	if !ok {
		return empty, fmt.Errorf("package instance not enabled")
	}
	downgrade := operation == "downgrade"
	if downgrade {
		if config.RecoveryReleaseVersion == "" || config.RecoveryProcedureVersion == "" {
			return empty, fmt.Errorf("package downgrade policy unavailable")
		}
		operation = "restore"
	}
	actions := map[string]string{"install": "install", "upgrade": "upgrade", "backup": "backup", "restore": "destructive-storage", "uninstall": "uninstall"}
	action, ok := actions[operation]
	if !ok {
		return empty, fmt.Errorf("package operation unsupported")
	}
	if (operation == "backup" || operation == "restore") && !pilotBackupID.MatchString(backupID) {
		return empty, fmt.Errorf("explicit immutable checkpoint identity required")
	}
	shape, _ := shapeFor(config.Profile, false)
	procedureVersion := config.ProcedureVersion
	if procedureVersion == "" {
		procedureVersion = "1.1.0"
	}
	releaseVersion := config.ReleaseVersion
	if downgrade {
		releaseVersion = config.RecoveryReleaseVersion
		procedureVersion = config.RecoveryProcedureVersion
	}
	releaseExpected := ReleaseExpectation{ApplicationID: shape.App, ReleaseVersion: releaseVersion, ProcedureID: "proc_" + operation + "_" + config.Profile, ProcedureVersion: procedureVersion, Operation: operation}
	policy, err := FetchCapabilityPolicyBundle(ctx, b.Preflight.ControlPlaneURL, b.Preflight.TLSConfig, b.Preflight.CapabilityPolicy)
	if err != nil {
		return empty, err
	}
	release, err := FetchVerifiedReleaseArtifactsWithRevocation(ctx, b.Preflight.ControlPlaneURL, b.Preflight.TLSConfig, b.Preflight.SigningPolicy, releaseExpected, b.Preflight.RevocationFreshness, b.Preflight.RevocationState)
	if err != nil {
		return empty, err
	}
	if err := policy.ValidateManifestLicense(release.Manifest()); err != nil {
		return empty, err
	}
	if err := verifySourceFreshness(time.Now().UTC(), b.Preflight.ClockSkew, release.Manifest(), release.Procedure()); err != nil {
		return empty, err
	}
	manifest, err := parseCanonicalObject(release.Manifest(), "manifest")
	if err != nil {
		return empty, err
	}
	procedure, err := parseCanonicalObject(release.Procedure(), "procedure")
	if err != nil {
		return empty, err
	}
	var rollbackExpected ProcedureExpectation
	var rollbackRaw []byte
	var rollbackProcedure map[string]any
	if identity, ok := procedure["rollbackProcedureId"].(string); ok {
		rollbackExpected = ProcedureExpectation{ProcedureID: identity, Version: procedureVersion, ApplicationID: shape.App, Operation: "rollback"}
		rollbackRaw, err = FetchVerifiedProcedureRecord(ctx, b.Preflight.ControlPlaneURL, b.Preflight.TLSConfig, b.Preflight.SigningPolicy, rollbackExpected)
		if err != nil {
			return empty, err
		}
		rollbackProcedure, err = parseCanonicalObject(rollbackRaw, "compensation procedure")
		if err != nil {
			return empty, err
		}
	}
	budget, err := protectedContextRecord(config.BudgetFile)
	if err != nil {
		return empty, err
	}
	qualification, err := protectedContextRecord(config.QualificationFile)
	if err != nil {
		return empty, err
	}
	if b.Probe == nil {
		return empty, fmt.Errorf("live host probe required")
	}
	profile, err := b.Probe()
	if err != nil {
		return empty, err
	}
	if err := SubmitProfile(ctx, b.Preflight.ControlPlaneURL, b.Preflight.TLSConfig, profile); err != nil {
		return empty, err
	}
	host, err := FetchHostProfile(ctx, b.Preflight.ControlPlaneURL, b.Preflight.TLSConfig, b.TenantScope, b.HostID)
	if err != nil {
		return empty, err
	}
	boundary, err := NewDockerPackageBoundary(config, []byte("{}"))
	if err != nil {
		return empty, err
	}
	compose, err := boundary.render(release.Manifest())
	if err != nil {
		return empty, err
	}
	rendererInputs, err := boundary.rendererInputsDigest()
	if err != nil {
		return empty, err
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return empty, err
	}
	id := hex.EncodeToString(nonce[:])
	now := time.Now().UTC()
	created := now.Add(-time.Second).Format(time.RFC3339)
	expires := now.Add(10 * time.Minute).Format(time.RFC3339)
	artifactValue := map[string]any{"schemaVersion": "1.0.0", "artifactId": "art_" + id, "applicationId": shape.App, "releaseVersion": releaseVersion,
		"manifestDigest": release.Release.ManifestDigest, "renderer": map[string]any{"name": "wazi-compose-renderer", "version": "0.5.0", "inputsDigest": rendererInputs},
		"composeDigest": fmt.Sprintf("sha256:%x", sha256.Sum256(compose)), "projectName": project, "services": []string{shape.Service}, "createdAt": created, "provenance": manifest["provenance"]}
	artifact, err := canonicalPayload(artifactValue)
	if err != nil {
		return empty, err
	}
	procedureRef, err := contextRecordRef(release.Procedure(), "procedureId")
	if err != nil {
		return empty, err
	}
	hostRef, err := contextRecordRef(host, "hostId")
	if err != nil {
		return empty, err
	}
	policyRef, err := contextRecordRef(policy.Raw(), "policyId")
	if err != nil {
		return empty, err
	}
	artifactRef, err := contextRecordRef(artifact, "artifactId")
	if err != nil {
		return empty, err
	}
	budgetRef, err := contextRecordRef(budget, "budgetId")
	if err != nil {
		return empty, err
	}
	qualificationRef, err := contextRecordRef(qualification, "qualificationId")
	if err != nil {
		return empty, err
	}
	releaseValue, _ := objectValue(manifest["release"])
	environment, _ := optionalObjects(releaseValue["environment"], "environment")
	secretRefs := []any{}
	for _, entry := range environment {
		if ref, ok := entry["secretRef"].(string); ok {
			secretRefs = append(secretRefs, map[string]any{"secretRef": ref, "injectionPoint": shape.Service + " environment " + entry["name"].(string)})
		}
	}
	caps, err := stringArray(procedure["capabilitySequence"], "capabilities")
	if err != nil {
		return empty, err
	}
	rollbackCaps := []string{}
	if rollbackProcedure != nil {
		rollbackCaps, err = stringArray(rollbackProcedure["capabilitySequence"], "compensation capabilities")
		if err != nil {
			return empty, err
		}
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, capability := range append(append([]string{}, caps...), rollbackCaps...) {
		if !seen[capability] {
			unique = append(unique, capability)
			seen[capability] = true
		}
	}
	provenance, _ := objectValue(manifest["provenance"])
	envelopeValue := map[string]any{"schemaVersion": "2.0.0", "envelopeId": "env_" + id, "tenantScope": b.TenantScope, "operation": operation,
		"deploymentMode": "deterministic", "applicationId": shape.App, "releaseVersion": releaseVersion, "manifestDigest": release.Release.ManifestDigest, "manifestReviewState": manifest["reviewState"],
		"hostProfileRef": hostRef, "policyBundleRef": policyRef, "procedureRef": procedureRef, "composeArtifactRef": artifactRef,
		"contributionBudgetRef": budgetRef, "managedVolumeQualificationRef": qualificationRef, "secretReferences": secretRefs,
		"allowedCapabilities": unique, "forbiddenOperations": []string{"shell.exec", "volume.delete-without-checkpoint"},
		"requiredApprovals": []string{action}, "verificationChecks": []any{map[string]any{"id": "package-readiness", "type": "health", "target": shape.Health}},
		"rollbackStrategy": map[string]any{"supported": false, "checkpointRequired": operation == "restore" || (operation == "upgrade" && config.Profile == "uptime-kuma")},
		"citedSources":     []any{map[string]any{"recordId": provenance["sourceId"], "contentHash": provenance["contentHash"]}}, "createdAt": created, "expiresAt": expires}
	if rollbackProcedure != nil {
		ref, err := contextRecordRef(rollbackRaw, "procedureId")
		if err != nil {
			return empty, err
		}
		strategy := envelopeValue["rollbackStrategy"].(map[string]any)
		strategy["supported"] = true
		strategy["procedureRef"] = ref
	}
	checkpointID := backupID
	if operation == "restore" || (operation == "upgrade" && config.Profile == "uptime-kuma") {
		checkpointID = "bak_pre_" + id
		envelopeValue["rollbackStrategy"].(map[string]any)["checkpoint"] = map[string]any{"backupId": checkpointID, "volume": shape.Volume}
	}
	envelope, err := canonicalPayload(envelopeValue)
	if err != nil {
		return empty, err
	}
	artifactDigest := artifactRef["digest"]
	imageDigest := releaseValue["images"].([]any)[0].(map[string]any)["digest"]
	parameters := map[string]any{
		"manifest.verify": map[string]any{"manifestDigest": release.Release.ManifestDigest, "requireReviewState": "signed"},
		"compose.pull":    map[string]any{"service": shape.Service, "imageDigest": imageDigest},
		"compose.up":      map[string]any{"service": shape.Service, "projectName": project, "composeArtifactDigest": artifactDigest},
		"compose.down":    map[string]any{"service": shape.Service, "projectName": project, "removeVolumes": false},
		"health.check":    map[string]any{"target": shape.Health},
		"volume.backup":   map[string]any{"volume": shape.Volume, "backupId": checkpointID},
		"volume.restore":  map[string]any{"volume": shape.Volume, "backupId": backupID},
	}
	steps := []any{}
	rollbackSteps := []any{}
	compensationIDs := map[string]string{}
	for index, capability := range rollbackCaps {
		params := parameters[capability]
		if capability == "volume.backup" {
			params = map[string]any{"volume": shape.Volume, "backupId": "bak_compensate_" + id}
		}
		stepID := fmt.Sprintf("step_compensation_%d", index+1)
		compensationIDs[capability] = stepID
		rollbackSteps = append(rollbackSteps, map[string]any{"stepId": stepID, "capability": capability, "parameters": params, "citations": []any{provenance["sourceId"]}, "requiresApproval": isMutating(capability), "verification": map[string]any{"type": "exit-state"}})
	}
	for index, capability := range caps {
		params, exists := parameters[capability]
		if !exists {
			return empty, fmt.Errorf("procedure needs an unqualified package action")
		}
		verification := "exit-state"
		if capability == "health.check" {
			verification = "health"
		}
		step := map[string]any{"stepId": fmt.Sprintf("step_%d", index+1), "capability": capability, "parameters": params,
			"citations": []any{provenance["sourceId"]}, "requiresApproval": isMutating(capability), "verification": map[string]any{"type": verification}}
		if compensation := map[string]string{"compose.up": "compose.down", "compose.down": "compose.up", "volume.restore": "volume.backup"}[capability]; compensation != "" {
			step["rollbackStepId"] = compensationIDs[compensation]
		}
		steps = append(steps, step)
	}
	plan, err := canonicalPayload(map[string]any{"schemaVersion": "1.0.0", "planId": "plan_" + id, "envelopeId": "env_" + id, "operation": operation,
		"origin": "deterministic-resolver", "unresolvedQuestions": []any{}, "composeArtifactRef": artifactRef, "steps": steps, "rollbackSteps": rollbackSteps, "requiredApprovals": []string{action}, "createdAt": created, "expiresAt": expires})
	if err != nil {
		return empty, err
	}
	if err := contracts.ValidateExecutionContextEnvelopeV2(envelope); err != nil {
		return empty, fmt.Errorf("generated envelope rejected: %w", err)
	}
	if err := contracts.ValidateDeploymentPlan(plan); err != nil {
		return empty, fmt.Errorf("generated plan rejected: %w", err)
	}
	return PilotJob{JobID: "job_" + id, CaseID: "case_" + id, Release: releaseExpected, RollbackProcedure: rollbackExpected, Envelope: envelope, Plan: plan, Artifact: artifact, Budget: budget, Qualification: qualification, Approvals: []ApprovalReference{}}, nil
}
