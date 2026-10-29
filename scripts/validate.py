#!/usr/bin/env python3
"""Phase 0 contract validation (fail-closed).

Layers, each of which can fail the run:
 1. Required schema inventory (a missing schema is an error, never a skip).
 2. Schema validity (draft 2020-12, date-time format checking active).
 3. Secret scan over examples AND fixtures (keys + synthetic value patterns).
 4. Timestamp ordering and freshness against an injected clock.
 5. Cross-record semantic validation of the example deployment graph.
 6. Compatibility fixtures: real manifest/host/policy records run through
    hard filters at each fixture's evaluationTime.
 7. Adversarial fixtures: records that must FAIL schema validation.

Dependencies are pinned in requirements-dev.txt. The injected evaluation
clock for the example graph is EXAMPLE_EVAL_TIME; historical example
timestamps are judged against it, never against wall-clock time.
"""
import copy
import hashlib
import importlib.util
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EXAMPLE_EVAL_TIME = "2026-10-21T09:05:00Z"

try:
    from jsonschema import Draft202012Validator, FormatChecker
    from referencing import Registry, Resource
except ImportError:
    sys.exit("jsonschema is required: pip install -r requirements-dev.txt")

REQUIRED_SCHEMAS = {
    "common.schema.json",
    "app-manifest.schema.json",
    "deployment-plan.schema.json",
    "execution-context-envelope.schema.json",
    "host-profile.schema.json",
    "policy-bundle.schema.json",
    "evidence-record.schema.json",
    "deployment-case.schema.json",
    "procedure.schema.json",
    "approval-record.schema.json",
    "compose-artifact.schema.json",
    "revocation-record.schema.json",
    "eval-fixture.schema.json",
}

EXAMPLE_MAP = {
    "app-manifest.json": "app-manifest.schema.json",
    "deployment-plan.json": "deployment-plan.schema.json",
    "execution-context-envelope.json": "execution-context-envelope.schema.json",
    "host-profile.json": "host-profile.schema.json",
    "policy-bundle.json": "policy-bundle.schema.json",
    "evidence-record.json": "evidence-record.schema.json",
    "deployment-case.json": "deployment-case.schema.json",
    "procedure-install.json": "procedure.schema.json",
    "procedure-rollback.json": "procedure.schema.json",
    "compose-artifact.json": "compose-artifact.schema.json",
    "approval-record.json": "approval-record.schema.json",
    "revocation-list.json": "revocation-record.schema.json",
}

FORBIDDEN_KEYS = {"secretvalue", "secret_value", "password", "token", "apikey", "api_key"}
SECRET_PATTERNS = [
    re.compile(r"(?i)(password|passwd|secret|token|api[_-]?key)\s*[:=](?!//)\s*[^\s\"']{4,}"),
    re.compile(r"Bearer\s+[A-Za-z0-9._\-]{8,}"),
    re.compile(r"ghp_[A-Za-z0-9]{10,}"),
    re.compile(r"AKIA[0-9A-Z]{16}"),
    re.compile(r"-----BEGIN"),
]
SENSITIVE_NAME = re.compile(r"(SECRET|PASSWORD|PASSWD|TOKEN|API_KEY|PRIVATE_KEY)")


def load(path):
    with open(path, encoding="utf-8-sig") as fh:
        return json.load(fh)


def canonical_bytes(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def canonical_digest(obj):
    return "sha256:" + hashlib.sha256(canonical_bytes(obj)).hexdigest()


def parse_time(value, where, errors):
    try:
        parsed = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            raise ValueError("missing timezone")
        return parsed
    except Exception:  # noqa: BLE001
        errors.append(f"{where}: invalid or timezone-less timestamp '{value}'")
        return None


def check_secrets(node, where, errors):
    if isinstance(node, dict):
        for key, value in node.items():
            if key.lower() in FORBIDDEN_KEYS:
                errors.append(f"{where}: forbidden secret-bearing key '{key}'")
            check_secrets(value, where, errors)
    elif isinstance(node, list):
        for item in node:
            check_secrets(item, where, errors)
    elif isinstance(node, str):
        for pattern in SECRET_PATTERNS:
            if pattern.search(node):
                errors.append(f"{where}: string matches a secret pattern (value suppressed)")
                break


def check_sensitive_env(manifest, where, errors):
    for entry in manifest.get("release", {}).get("environment", []):
        if SENSITIVE_NAME.search(entry.get("name", "")) and "secretRef" not in entry:
            errors.append(f"{where}: sensitive env '{entry.get('name')}' must use secretRef")


def build_registry(schemas):
    registry = Registry()
    for schema in schemas.values():
        registry = registry.with_resource(schema["$id"], Resource.from_contents(schema))
    return registry


def make_validator(schema, registry):
    checker = FormatChecker()
    if "date-time" not in checker.checkers:
        raise RuntimeError(
            "date-time format checking is inactive; install rfc3339-validator "
            "(see requirements-dev.txt). Refusing to validate timestamps unchecked."
        )
    return Draft202012Validator(schema, registry=registry, format_checker=checker)


def schema_errors(validator, data):
    return [f"{err.message} at {list(err.path)}" for err in validator.iter_errors(data)]


def version_tuple(value):
    # The initial supported matrix accepts stable numeric versions only.
    # Stripping non-digits would turn 24.0.0-rc1 into 24.0.1.
    if not re.fullmatch(r"\d+\.\d+(?:\.\d+)?", str(value)):
        raise ValueError("unsupported runtime version format")
    parts = [int(piece) for piece in str(value).split(".")]
    return tuple(parts + [0] * (3 - len(parts)))


def decide(manifest, host, policy, registry_state, eval_time):
    """Hard filters on real records. Runs before any ranking."""
    req = manifest.get("requirements", {})
    if req.get("privileged") is True:
        return "reject", "privileged-not-supported"
    if registry_state == "revoked":
        return "reject", "revoked-manifest"
    if manifest.get("reviewState") not in ("signed", "canary", "stable"):
        return "reject", "unsigned-manifest"
    if manifest.get("reviewState") in ("signed", "canary", "stable") and "signature" not in manifest:
        return "reject", "unsigned-manifest"
    licensing = policy.get("licensing") or {}
    allowed_licences = licensing.get("allowedLicenses") or []
    if allowed_licences and manifest.get("license") not in allowed_licences:
        return "reject", "licence-not-allowed"
    observed = parse_time(host.get("observedAt"), "host", [])
    expires = parse_time(host.get("expiresAt"), "host", [])
    if not observed or not expires or not eval_time or observed > eval_time:
        return "reject", "missing-mandatory-fact"
    if observed and expires and expires <= observed:
        return "reject", "host-profile-expired"
    if expires and eval_time and eval_time >= expires:
        return "reject", "host-profile-expired"
    max_age = (policy.get("freshness") or {}).get("maxHostProfileAgeMinutes")
    if max_age is None:
        return "reject", "missing-mandatory-fact"
    if observed and eval_time and max_age is not None:
        if (eval_time - observed).total_seconds() > max_age * 60:
            return "reject", "stale-host-profile"
    if host.get("os", {}).get("family") != "linux":
        return "reject", "unsupported-os"
    if host.get("architecture") not in req.get("architectures", []):
        return "reject", "wrong-architecture"
    res = host.get("resources", {})
    if res.get("cpuCores", 0) < req.get("minCpuCores", 0):
        return "reject", "insufficient-cpu"
    if res.get("ramMb", 0) < req.get("minRamMb", 0):
        return "reject", "insufficient-ram"
    if res.get("storageFreeMb", 0) < req.get("minStorageMb", 0):
        return "reject", "insufficient-storage"
    docker = host.get("docker", {})
    try:
        engine_version = version_tuple(docker.get("engineVersion"))
        min_engine = version_tuple(req.get("minDockerVersion"))
    except ValueError:
        return "reject", "unsupported-docker-version"
    if engine_version < min_engine:
        return "reject", "unsupported-docker-version"
    try:
        compose_version = version_tuple(docker.get("composeVersion"))
        min_compose = version_tuple(req.get("minComposeVersion"))
    except ValueError:
        return "reject", "unsupported-compose-version"
    if compose_version < min_compose:
        return "reject", "unsupported-compose-version"
    volumes = manifest.get("release", {}).get("volumes", [])
    if volumes and (host.get("filesystem") or {}).get("dockerRootWritable") is not True:
        return "reject", "filesystem-not-writable"
    required_ports = {p.get("hostPort") for p in manifest.get("release", {}).get("ports", []) if p.get("hostPort")}
    if required_ports & set(host.get("occupiedPorts", [])):
        return "reject", "occupied-port"
    return "accept", "compatible"


def semantic_errors(records, eval_time):
    """Cross-record validation of one deployment graph.

    records keys: manifest, host, policy, envelope, plan, artifact,
    procedures (list), approvals (list), case (optional).
    Returns a list of human-readable errors; empty means the graph is
    internally authorised, digest-bound, and fresh at eval_time.
    """
    errors = []
    manifest, host, policy = records["manifest"], records["host"], records["policy"]
    envelope, plan, artifact = records["envelope"], records["plan"], records["artifact"]
    procedures = {p["procedureId"]: p for p in records.get("procedures", [])}
    approvals = records.get("approvals", [])

    # Digest binding: a changed referenced record invalidates every ref.
    manifest_digest = canonical_digest(manifest)
    if artifact.get("manifestDigest") != manifest_digest:
        errors.append("compose artifact manifestDigest does not match canonical manifest digest")
    if envelope.get("manifestDigest") != manifest_digest:
        errors.append("envelope manifestDigest does not match canonical manifest digest")
    digests = {
        host["hostId"]: canonical_digest(host),
        policy["policyId"]: canonical_digest(policy),
        artifact["artifactId"]: canonical_digest(artifact),
    }
    for proc in procedures.values():
        digests[proc["procedureId"]] = canonical_digest(proc)
    for label, ref in [
        ("hostProfileRef", envelope.get("hostProfileRef", {})),
        ("policyBundleRef", envelope.get("policyBundleRef", {})),
        ("procedureRef", envelope.get("procedureRef", {})),
        ("composeArtifactRef", envelope.get("composeArtifactRef", {})),
    ]:
        rid = ref.get("recordId")
        if rid not in digests:
            errors.append(f"envelope {label} references unknown record '{rid}'")
        elif ref.get("digest") != digests[rid]:
            errors.append(f"envelope {label} digest mismatch for '{rid}' (record changed after envelope was built)")
    rollback_ref = (envelope.get("rollbackStrategy") or {}).get("procedureRef")
    if (envelope.get("rollbackStrategy") or {}).get("supported") and rollback_ref:
        rid = rollback_ref.get("recordId")
        if rid not in procedures:
            errors.append(f"rollback procedure '{rid}' has no typed procedure record")
        elif rollback_ref.get("digest") != digests.get(rid):
            errors.append(f"rollback procedureRef digest mismatch for '{rid}'")
    if plan.get("composeArtifactRef", {}).get("recordId") != artifact["artifactId"]:
        errors.append("plan composeArtifactRef does not reference the envelope compose artifact")
    elif plan["composeArtifactRef"].get("digest") != digests[artifact["artifactId"]]:
        errors.append("plan composeArtifactRef digest mismatch")

    # Identity and tenant agreement.
    if envelope.get("applicationId") != manifest.get("appId"):
        errors.append("envelope applicationId does not match manifest appId")
    if envelope.get("releaseVersion") != manifest.get("version"):
        errors.append("envelope releaseVersion does not match manifest version")
    if artifact.get("applicationId") != manifest.get("appId"):
        errors.append("compose artifact applicationId does not match manifest appId")
    tenants = {
        envelope.get("tenantScope"), host.get("tenantScope"), policy.get("tenantScope"),
    }
    if len(tenants) != 1:
        errors.append(f"tenant scopes disagree across envelope/host/policy: {sorted(t for t in tenants if t)}")
    tenant = envelope.get("tenantScope")
    if plan.get("envelopeId") != envelope.get("envelopeId"):
        errors.append("plan envelopeId does not match envelope")
    if plan.get("operation") != envelope.get("operation"):
        errors.append("plan operation does not match envelope operation")

    # Procedure agreement.
    proc = procedures.get(envelope.get("procedureRef", {}).get("recordId"))
    if proc:
        if proc.get("operation") != envelope.get("operation"):
            errors.append("procedure operation does not match envelope operation")
        if proc.get("applicationId") != envelope.get("applicationId"):
            errors.append("procedure applicationId does not match envelope applicationId")
        if proc.get("reviewState") not in ("signed", "canary", "stable"):
            errors.append("procedure must have a signed review state")
        if [s.get("capability") for s in plan.get("steps", [])] != proc.get("capabilitySequence"):
            errors.append("forward capability sequence does not match procedure order")

    # Capability authorisation: plan <= envelope <= policy, minus denials.
    allowed_env = set(envelope.get("allowedCapabilities", []))
    allowed_policy = set(policy.get("allowedCapabilities", []))
    denied_policy = set(policy.get("deniedCapabilities", []))
    forbidden = set(envelope.get("forbiddenOperations", []))
    if allowed_env - (allowed_policy - denied_policy):
        errors.append("envelope capabilities exceed policy allowlist or include policy denials")
    steps = plan.get("steps", [])
    rollback_steps = plan.get("rollbackSteps", [])
    step_ids = {s["stepId"] for s in steps}
    rollback_ids = {s["stepId"] for s in rollback_steps}
    if len(step_ids) != len(steps) or len(rollback_ids) != len(rollback_steps):
        errors.append("step ids must be unique within each sequence")
    rollback_proc = procedures.get((rollback_ref or {}).get("recordId"))
    if rollback_proc and rollback_steps:
        if [s.get("capability") for s in rollback_steps] != rollback_proc.get("capabilitySequence"):
            errors.append("rollback capability sequence does not match procedure order")
        if rollback_proc.get("operation") != "rollback" or rollback_proc.get("applicationId") != manifest.get("appId"):
            errors.append("rollback procedure operation/application does not match graph")
    if step_ids & rollback_ids:
        errors.append("step ids collide between steps and rollbackSteps")
    for step in steps + rollback_steps:
        cap = step.get("capability")
        if cap not in allowed_env:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' not allowed by envelope")
        if cap not in allowed_policy or cap in denied_policy:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' not allowed by policy")
        if cap in forbidden:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' is forbidden by envelope")
        if proc and cap not in proc.get("capabilitySequence", []) and step in steps:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' not in procedure sequence")
    for step in steps:
        target = step.get("rollbackStepId")
        if target:
            if target in step_ids:
                errors.append(f"step {step['stepId']}: rollbackStepId '{target}' points into steps, not rollbackSteps")
            elif target not in rollback_ids:
                errors.append(f"step {step['stepId']}: rollbackStepId '{target}' does not exist in rollbackSteps")
    for step in rollback_steps:
        if step.get("rollbackStepId"):
            errors.append(f"rollback step {step['stepId']} must not itself declare a rollbackStepId")

    # Step parameter agreement with the manifest and artifact.
    image_digests = {img["service"]: img["digest"] for img in manifest.get("release", {}).get("images", [])}
    for step in steps + rollback_steps:
        params = step.get("parameters", {})
        if "service" in params and params["service"] not in artifact.get("services", []):
            errors.append(f"step {step['stepId']}: service is not declared in compose artifact")
        if "projectName" in params and params["projectName"] != artifact.get("projectName"):
            errors.append(f"step {step['stepId']}: projectName does not match artifact")
        if step.get("capability") == "manifest.verify" and params.get("manifestDigest") != manifest_digest:
            errors.append(f"step {step['stepId']}: manifest.verify digest does not match manifest")
        if step.get("capability") == "compose.pull" and params.get("imageDigest") != image_digests.get(params.get("service")):
            errors.append(f"step {step['stepId']}: compose.pull imageDigest does not match manifest image digest")
        if step.get("capability") == "compose.up":
            if params.get("composeArtifactDigest") != artifact.get("composeDigest"):
                errors.append(f"step {step['stepId']}: compose.up composeArtifactDigest does not match artifact")
            if params.get("projectName") != artifact.get("projectName"):
                errors.append(f"step {step['stepId']}: compose.up projectName does not match artifact")
        if step.get("capability") == "secret.inject":
            known = {ref["secretRef"] for ref in envelope.get("secretReferences", [])}
            if params.get("secretRef") not in known:
                errors.append(f"step {step['stepId']}: secret.inject references a secretRef not declared in the envelope")
            declared = {(e.get("name"), e.get("secretRef")) for e in manifest.get("release", {}).get("environment", [])}
            if (params.get("targetEnv"), params.get("secretRef")) not in declared:
                errors.append(f"step {step['stepId']}: secret target does not match manifest environment")

    # Citations must resolve to records in the graph.
    known_ids = set(digests) | {envelope["envelopeId"], plan["planId"], manifest["appId"]}
    known_ids |= {p.get("provenance", {}).get("sourceId") for p in [manifest] if p.get("provenance")}
    for step in steps + rollback_steps:
        for citation in step.get("citations", []):
            if citation not in known_ids:
                errors.append(f"step {step.get('stepId')}: citation '{citation}' does not resolve to a known record")
    for source in envelope.get("citedSources", []):
        if source.get("recordId") not in known_ids:
            errors.append(f"envelope citation '{source.get('recordId')}' does not resolve to a known record")
        expected_hash = digests.get(source.get("recordId"))
        if source.get("recordId") == manifest.get("provenance", {}).get("sourceId"):
            expected_hash = manifest["provenance"]["contentHash"]
        if expected_hash and source.get("contentHash") != expected_hash:
            errors.append("envelope citation contentHash does not match cited record")

    # Approvals: every required action needs a live, digest-bound approval.
    if plan.get("unresolvedQuestions"):
        errors.append("plan has unresolvedQuestions; execution is blocked")
    required_actions = set(envelope.get("requiredApprovals", [])) | set(plan.get("requiredApprovals", []))
    if any(p.get("exposure") == "public" for p in manifest.get("release", {}).get("ports", [])):
        required_actions.add("public-exposure")
    public_sources = (
        any(p.get("exposure") == "public" for p in manifest.get("release", {}).get("ports", []))
        or any(
            s.get("capability") == "proxy.configure" and s.get("parameters", {}).get("exposure") == "public"
            for s in steps
        )
    )
    if public_sources and (policy.get("network") or {}).get("allowPublicExposure") is not True:
        errors.append("public exposure is denied by policy; approval cannot override denial")
    for step in steps + rollback_steps:
        params = step.get("parameters", {})
        if step.get("capability") == "proxy.configure" and params.get("exposure") == "public":
            required_actions.add("public-exposure")
        if step.get("capability") == "compose.down" and params.get("removeVolumes"):
            required_actions.add("destructive-storage")
        if step.get("capability") == "volume.restore":
            required_actions.add("destructive-storage")
    if plan.get("operation") in ("upgrade", "rollback", "uninstall"):
        required_actions.add(plan["operation"])
    actions = [r["action"] for r in policy.get("approvalRules", [])]
    if len(set(actions)) != len(actions):
        errors.append("policy has duplicate approval rules for an action")
    rules = {rule["action"]: rule["approval"] for rule in policy.get("approvalRules", [])}
    plan_digest = canonical_digest(plan)
    for action in sorted(required_actions):
        needed = rules.get(action, "user")
        if needed == "none":
            continue
        matches = [
            a for a in approvals
            if a.get("action") == action and a.get("decision") == "approved"
            and a.get("planId") == plan["planId"] and a.get("planDigest") == plan_digest
            and a.get("envelopeId") == envelope["envelopeId"]
            and a.get("envelopeDigest") == canonical_digest(envelope)
            and a.get("tenantScope") == tenant and a.get("hostId") == host["hostId"]
            and a.get("applicationId") == manifest["appId"]
        ]
        if needed in ("maintainer", "two-maintainers"):
            matches = [a for a in matches if a.get("approver", {}).get("role") == "maintainer"]
        if not matches:
            errors.append(f"action '{action}' has no approval bound to this exact plan digest, tenant, host, and envelope")
            continue
        live = []
        for approval in matches:
            approved = parse_time(approval.get("approvedAt"), "approval", errors)
            expires = parse_time(approval.get("expiresAt"), "approval", errors)
            if approved and expires and eval_time and approved <= eval_time < expires:
                live.append(approval)
        if not live:
            errors.append(f"action '{action}' approval is expired or not yet valid at evaluation time")
        if needed == "two-maintainers" and len({a["approver"]["approverId"] for a in live}) < 2:
            errors.append(f"action '{action}' requires two distinct live maintainer approvals")

    # Freshness and ordering at the injected clock.
    def ordered(start, end, label):
        first = parse_time(start, label, errors)
        last = parse_time(end, label, errors)
        if first and last and last <= first:
            errors.append(f"{label}: expiresAt must be after its start timestamp")
        return first, last

    host_start, host_expires = ordered(host.get("observedAt"), host.get("expiresAt"), "host profile")
    env_start, env_expires = ordered(envelope.get("createdAt"), envelope.get("expiresAt"), "envelope")
    plan_start, plan_expires = ordered(plan.get("createdAt"), plan.get("expiresAt"), "plan")
    if eval_time:
        if any(t and t > eval_time for t in (host_start, env_start, plan_start)):
            errors.append("host/envelope/plan cannot originate in the future")
        if host_expires and eval_time >= host_expires:
            errors.append("host profile is expired at evaluation time (host may have changed since preflight)")
        if env_expires and eval_time >= env_expires:
            errors.append("envelope is expired at evaluation time")
        if plan_expires and eval_time >= plan_expires:
            errors.append("plan is expired at evaluation time")
        max_age = (policy.get("freshness") or {}).get("maxHostProfileAgeMinutes")
        if max_age is None or (host_start and (eval_time - host_start).total_seconds() > max_age * 60):
            errors.append("host profile exceeds policy max age or freshness policy is missing")

    # Compatibility: the complete deployment graph itself must pass the
    # same hard filters as the fixtures. A semantically consistent graph
    # for an incompatible host is still rejected.
    decision, reason = decide(manifest, host, policy, records.get("registryState", "active"), eval_time)
    if decision != "accept":
        errors.append(f"deployment graph is not compatible: {reason}")

    # Service ownership: every declared owner must resolve to a manifest
    # service, and secret injection is bound to (service, env, secretRef).
    service_names = [img["service"] for img in manifest.get("release", {}).get("images", [])]
    service_set = set(service_names)
    sole_service = service_names[0] if len(service_names) == 1 else None
    for entry in manifest.get("release", {}).get("environment", []):
        owner = entry.get("service")
        if owner is not None and owner not in service_set:
            errors.append(f"environment entry '{entry.get('name')}' declares unknown service owner '{owner}'")
        if owner is None and sole_service is None:
            errors.append(f"environment entry '{entry.get('name')}' has no service owner in a multi-service manifest")
    for volume in manifest.get("release", {}).get("volumes", []):
        owner = volume.get("service")
        if owner is not None and owner not in service_set:
            errors.append(f"volume '{volume.get('name')}' declares unknown service owner '{owner}'")
        if owner is None and sole_service is None:
            errors.append(f"volume '{volume.get('name')}' has no service owner in a multi-service manifest")
    for port in manifest.get("release", {}).get("ports", []):
        if port.get("service") not in service_set:
            errors.append(f"port for service '{port.get('service')}' does not resolve to a manifest service")
    for step in steps + rollback_steps:
        if step.get("capability") == "secret.inject":
            params = step.get("parameters", {})
            declared = {
                (entry.get("service") or sole_service, entry.get("name"), entry.get("secretRef"))
                for entry in manifest.get("release", {}).get("environment", [])
            }
            if (params.get("service"), params.get("targetEnv"), params.get("secretRef")) not in declared:
                errors.append(
                    f"step {step['stepId']}: secret.inject (service, environment, secretRef) "
                    "does not match a manifest environment entry owned by that service"
                )

    case = records.get("case")
    if case:
        if case.get("tenantScope") != tenant:
            errors.append("deployment case tenantScope does not match envelope tenant")
        if case.get("hostId") != host["hostId"] or case.get("envelopeId") != envelope["envelopeId"]:
            errors.append("deployment case host/envelope does not match the graph")
        if case.get("planId") != plan["planId"]:
            errors.append("deployment case planId does not match plan")
        if case.get("applicationId") != manifest["appId"] or case.get("releaseVersion") != manifest["version"]:
            errors.append("deployment case application/release does not match manifest")
        if case.get("operation") != plan.get("operation"):
            errors.append("deployment case operation does not match plan operation")
        if case.get("manifestDigest") != manifest_digest:
            errors.append("deployment case manifestDigest does not match canonical manifest digest")

    # Evidence must be linked to the case and to this exact release.
    evidence_records = {e["evidenceId"]: e for e in records.get("evidence", [])}
    if case:
        for ref in case.get("evidenceRefs", []):
            evidence = evidence_records.get(ref)
            if evidence is None:
                errors.append(f"deployment case evidenceRef '{ref}' has no evidence record in the graph")
                continue
            if evidence.get("deploymentCaseId") != case["caseId"]:
                errors.append(f"evidence '{ref}' belongs to a different deployment case")
            if evidence.get("commitOrReleaseDigest") != manifest_digest:
                errors.append(f"evidence '{ref}' release digest does not match canonical manifest digest")
            if case.get("outcome") == "success" and evidence.get("result") != "pass":
                errors.append(f"evidence '{ref}' does not pass but the case claims success")
        if case.get("outcome") == "success" and case.get("operation") == "install":
            linked = [evidence_records[r] for r in case.get("evidenceRefs", []) if r in evidence_records]
            if not any(e.get("type") == "health-check" and e.get("result") == "pass" for e in linked):
                errors.append("successful install case requires a passing health-check evidence record")

    # Licence policy semantics.
    licensing = policy.get("licensing") or {}
    allowed_licences = licensing.get("allowedLicenses") or []
    if allowed_licences and manifest.get("license") not in allowed_licences:
        errors.append(f"manifest licence '{manifest.get('license')}' is not in policy allowedLicenses")

    # Signature payload binding and signer trust.
    signing = policy.get("signing") or {}
    def check_signed(record, label):
        signature = record.get("signature")
        if not signature:
            return
        unsigned = {k: v for k, v in record.items() if k != "signature"}
        if signature.get("payloadDigest") != canonical_digest(unsigned):
            errors.append(f"{label}: signature payloadDigest does not match the signed record content")
        if signing:
            if signature.get("signedBy") not in signing.get("trustedSigners", []):
                errors.append(f"{label}: signer '{signature.get('signedBy')}' is not in policy trustedSigners")
            if signature.get("algorithm") not in signing.get("algorithms", []):
                errors.append(f"{label}: signature algorithm is not allowed by policy")
    if not signing or not signing.get("trustedSigners") or not signing.get("algorithms"):
        errors.append("policy signing configuration (trustedSigners and algorithms) is required for signed deployment graphs")
    check_signed(manifest, "manifest")
    for procedure in procedures.values():
        if procedure.get("reviewState") not in ("signed", "canary", "stable"):
            errors.append(f"procedure {procedure['procedureId']} must have a signed review state")
        if procedure.get("reviewState") in ("signed", "canary", "stable") and not procedure.get("signature"):
            errors.append(f"procedure {procedure['procedureId']} is in a signed state but has no signature")
        check_signed(procedure, f"procedure {procedure['procedureId']}")

    # Revocation freshness: fail closed when policy requires the check.
    freshness = policy.get("freshness") or {}
    if freshness.get("rejectRevokedManifests"):
        revocations = records.get("revocations")
        if not revocations:
            errors.append("policy requires revocation checks but no revocation list is in the graph")
        else:
            generated = parse_time(revocations.get("generatedAt"), "revocation list", errors)
            rev_expires = parse_time(revocations.get("expiresAt"), "revocation list", errors)
            if generated and rev_expires and rev_expires <= generated:
                errors.append("revocation list expiresAt must be after generatedAt")
            if eval_time and rev_expires and eval_time >= rev_expires:
                errors.append("revocation list is stale at evaluation time; refusing to deploy without fresh revocation data")
            if eval_time and generated and generated > eval_time:
                errors.append("revocation list cannot originate in the future")
            for entry in revocations.get("entries", []):
                if entry.get("applicationId") == manifest["appId"] and (
                    not entry.get("releaseVersion") or entry.get("releaseVersion") == manifest["version"]
                ):
                    errors.append(f"manifest is revoked ({entry.get('reason')})")

    # Procedure parameter contracts: the signed program surface.
    def check_contracts(sequence_steps, procedure, label):
        if not procedure:
            return
        contracts = {c["capability"]: c for c in procedure.get("parameterContracts", [])}
        sequence = procedure.get("capabilitySequence", [])
        if set(contracts) != set(sequence):
            errors.append(f"{label}: parameterContracts must cover exactly the capability sequence")
        for step in sequence_steps:
            contract = contracts.get(step.get("capability"))
            if not contract:
                continue
            params = step.get("parameters", {})
            missing = [name for name in contract.get("requiredParameters", []) if name not in params]
            if missing:
                errors.append(f"step {step['stepId']}: missing procedure-required parameters {missing}")
            for key, value in (contract.get("fixedParameters") or {}).items():
                if params.get(key) != value:
                    errors.append(f"step {step['stepId']}: parameter '{key}' must equal the procedure-fixed value")
    check_contracts(steps, proc, "forward procedure")
    check_contracts(rollback_steps, rollback_proc, "rollback procedure")

    # Compensation: a rollbackStepId identifies the specific compensation
    # step for a mutation (not merely any step in the rollback sequence).
    # Capability and resource must correspond, per this contract:
    compensation_contract = {
        "compose.up": ("compose.down", ("service", "projectName")),
        "compose.down": ("compose.up", ("service", "projectName")),
        "secret.inject": ("compose.down", ("service",)),
        "proxy.configure": ("proxy.remove", ("hostname",)),
        "volume.restore": ("volume.backup", ("volume",)),
    }
    rollback_by_id = {s["stepId"]: s for s in rollback_steps}
    for step in steps:
        expected = compensation_contract.get(step.get("capability"))
        if not expected:
            continue
        target_id = step.get("rollbackStepId")
        if not target_id:
            errors.append(f"step {step['stepId']}: mutating capability '{step.get('capability')}' has no compensation (rollbackStepId)")
            continue
        target = rollback_by_id.get(target_id)
        if target is None:
            continue  # unresolved pointers are reported above
        want_capability, resource_keys = expected
        if target.get("capability") != want_capability:
            errors.append(
                f"step {step['stepId']}: compensation must be '{want_capability}' "
                f"but rollbackStepId points to '{target.get('capability')}'"
            )
            continue
        for key in resource_keys:
            if target.get("parameters", {}).get(key) != step.get("parameters", {}).get(key):
                errors.append(f"step {step['stepId']}: compensation resource '{key}' does not match the mutated resource")

    # Compose artifact: bytes are required for this gate and must equal
    # both the recorded digest and a fresh render of the manifest.
    if not artifact.get("localPath"):
        errors.append("compose artifact localPath is required; artifact byte validation cannot be skipped")
    file_digest = records.get("artifactFileDigest")
    if file_digest is None:
        errors.append("compose artifact bytes were not supplied; artifact validation cannot be skipped")
    elif file_digest != artifact.get("composeDigest"):
        errors.append("compose artifact bytes do not match composeDigest (artifact changed after rendering)")
    rendered_digest = records.get("artifactRenderedDigest")
    if rendered_digest is None:
        errors.append("rendered compose bytes were not supplied; cannot prove the artifact came from the manifest")
    elif rendered_digest != artifact.get("composeDigest"):
        errors.append("compose artifact bytes do not match render(manifest) from the declared renderer")
    renderer_version = records.get("rendererVersion")
    if renderer_version is None:
        errors.append("installed renderer version was not supplied; declared renderer version cannot be verified")
    elif artifact.get("renderer", {}).get("version") != renderer_version:
        errors.append(
            f"compose artifact declares renderer version '{artifact.get('renderer', {}).get('version')}' "
            f"but the installed renderer is '{renderer_version}'; version dispatch is not supported"
        )

    # Checkpoint: a checkpoint-dependent procedure needs a typed,
    # verified checkpoint step before the first mutating compose step.
    needs_checkpoint = any(
        p.get("requiresCheckpoint") for p in procedures.values()
    ) or (envelope.get("rollbackStrategy") or {}).get("checkpointRequired")
    if needs_checkpoint:
        checkpoint = (envelope.get("rollbackStrategy") or {}).get("checkpoint")
        if not checkpoint:
            errors.append("checkpoint-dependent procedure has no typed checkpoint reference in the envelope")
        else:
            mutating_caps = {"compose.up", "compose.down", "volume.restore"}
            first_mutation = next(
                (i for i, s in enumerate(steps) if s.get("capability") in mutating_caps), len(steps)
            )
            matched = any(
                s.get("capability") == "volume.backup"
                and s.get("parameters", {}).get("backupId") == checkpoint.get("backupId")
                and s.get("parameters", {}).get("volume") == checkpoint.get("volume")
                and i < first_mutation
                for i, s in enumerate(steps)
            )
            if not matched:
                errors.append("no volume.backup checkpoint step matching the envelope checkpoint reference before the first mutation")
    return errors


def load_examples(registry, schemas):
    errors, records_raw = [], {}
    examples_dir = ROOT / "context" / "examples"
    for example_name, schema_name in EXAMPLE_MAP.items():
        path = examples_dir / example_name
        if not path.exists():
            errors.append(f"missing example {example_name}")
            continue
        data = load(path)
        records_raw[example_name] = data
        check_secrets(data, example_name, errors)
        validator = make_validator(schemas[schema_name], registry)
        for err in schema_errors(validator, data):
            errors.append(f"{example_name}: {err}")
    if "app-manifest.json" in records_raw:
        check_sensitive_env(records_raw["app-manifest.json"], "app-manifest.json", errors)
    return records_raw, errors


def graph_from_examples(raw):
    return {
        "manifest": raw["app-manifest.json"],
        "host": raw["host-profile.json"],
        "policy": raw["policy-bundle.json"],
        "envelope": raw["execution-context-envelope.json"],
        "plan": raw["deployment-plan.json"],
        "artifact": raw["compose-artifact.json"],
        "procedures": [raw["procedure-install.json"], raw["procedure-rollback.json"]],
        "approvals": [raw["approval-record.json"]],
        "case": raw.get("deployment-case.json"),
        "evidence": [raw["evidence-record.json"]] if raw.get("evidence-record.json") else [],
        "revocations": raw.get("revocation-list.json"),
    }


def main():
    errors = []
    schemas = {}
    schemas_dir = ROOT / "schemas"
    for name in sorted(REQUIRED_SCHEMAS):
        path = schemas_dir / name
        if not path.exists():
            errors.append(f"missing required schema {name} (validation is fail-closed; nothing is skipped)")
            continue
        try:
            schema = load(path)
            Draft202012Validator.check_schema(schema)
            schemas[name] = schema
        except Exception as exc:  # noqa: BLE001
            errors.append(f"schema {name}: {exc}")
    for path in sorted(schemas_dir.glob("*.schema.json")):
        if path.name not in REQUIRED_SCHEMAS:
            errors.append(f"unregistered schema {path.name}; add it to REQUIRED_SCHEMAS or remove it")
    if errors:
        print("VALIDATION FAILED")
        for err in errors:
            print(f" - {err}")
        return 1

    vectors_path = ROOT / "schemas" / "canonicalization-vectors.json"
    if not vectors_path.exists():
        errors.append("missing schemas/canonicalization-vectors.json (ADR-001 test vectors are required)")
    else:
        for vector in load(vectors_path).get("vectors", []):
            if canonical_bytes(vector["input"]).decode("utf-8") != vector["canonical"]:
                errors.append(f"canonicalization vector '{vector['name']}': canonical form mismatch")
            if canonical_digest(vector["input"]) != vector["sha256"]:
                errors.append(f"canonicalization vector '{vector['name']}': sha256 mismatch")

    registry = build_registry(schemas)
    try:
        make_validator(schemas["common.schema.json"], registry)
    except RuntimeError as exc:
        print(f"VALIDATION FAILED\n - {exc}")
        return 1

    raw, example_errors = load_examples(registry, schemas)
    errors += example_errors
    eval_time = parse_time(EXAMPLE_EVAL_TIME, "evaluation clock", errors)
    if not example_errors and len(raw) == len(EXAMPLE_MAP):
        graph = graph_from_examples(raw)
        local_path = graph["artifact"].get("localPath")
        if local_path and (ROOT / local_path).exists():
            graph["artifactFileDigest"] = "sha256:" + hashlib.sha256((ROOT / local_path).read_bytes()).hexdigest()
        render_spec = importlib.util.spec_from_file_location("render_compose", ROOT / "scripts" / "render_compose.py")
        render_module = importlib.util.module_from_spec(render_spec)
        render_spec.loader.exec_module(render_module)
        graph["rendererVersion"] = render_module.RENDERER_VERSION
        try:
            graph["artifactRenderedDigest"] = "sha256:" + hashlib.sha256(render_module.render(graph["manifest"])).hexdigest()
        except Exception as exc:  # noqa: BLE001 - renderer rejection is a validation failure, not a crash
            errors.append(f"example graph: renderer rejected the manifest: {exc}")
        for err in semantic_errors(graph, eval_time):
            errors.append(f"example graph: {err}")

    # Compatibility fixtures on real records.
    fixture_validator = make_validator(schemas["eval-fixture.schema.json"], registry)
    manifest_validator = make_validator(schemas["app-manifest.schema.json"], registry)
    host_validator = make_validator(schemas["host-profile.schema.json"], registry)
    policy_validator = make_validator(schemas["policy-bundle.schema.json"], registry)
    default_policy = raw.get("policy-bundle.json")
    fixtures = sorted((ROOT / "evals" / "fixtures").glob("*.json"))
    if not fixtures:
        errors.append("no evaluation fixtures found")
    for path in fixtures:
        data = load(path)
        check_secrets(data, path.name, errors)
        for err in schema_errors(fixture_validator, data):
            errors.append(f"{path.name}: {err}")
        manifest, host = data.get("manifest", {}), data.get("host", {})
        policy = data.get("policy") or default_policy or {}
        check_sensitive_env(manifest, path.name, errors)
        fixture_time = parse_time(data.get("evaluationTime"), path.name, errors)
        expected = data.get("expected", {})
        want = (expected.get("decision"), expected.get("reasonCode"))
        record_errors = (
            schema_errors(manifest_validator, manifest)
            + schema_errors(host_validator, host)
            + (schema_errors(policy_validator, policy) if data.get("policy") else [])
        )
        privileged = (manifest.get("requirements") or {}).get("privileged") is True
        if privileged:
            got = ("reject", "privileged-not-supported")
        elif record_errors:
            got = ("reject", "missing-mandatory-fact")
        else:
            got = decide(manifest, host, policy, data.get("registryState", "active"), fixture_time)
        if got != want:
            detail = f"; record schema errors: {record_errors[:2]}" if record_errors and not privileged else ""
            errors.append(f"{path.name}: hard filters give {got}, fixture expects {want}{detail}")

    # Adversarial fixtures must fail schema validation.
    adv_dir = ROOT / "evals" / "adversarial"
    adv_files = sorted(adv_dir.glob("*.json")) if adv_dir.exists() else []
    if not adv_files:
        errors.append("no adversarial fixtures found in evals/adversarial")
    for path in adv_files:
        data = load(path)
        schema_name = data.get("schema")
        record = data.get("record")
        if schema_name not in schemas or record is None:
            errors.append(f"{path.name}: adversarial fixture must name a known schema and a record")
            continue
        validator = make_validator(schemas[schema_name], registry)
        if not schema_errors(validator, record):
            errors.append(f"{path.name}: adversarial record unexpectedly PASSES {schema_name}")

    # Experimental contracts remain isolated from production capability enums.
    import sprints
    try:
        sprints.validate_examples()
        world_examples = ROOT / "context" / "worlds"
        for path in world_examples.glob("*.json"):
            check_secrets(load(path), path.name, errors)
    except (OSError, ValueError) as exc:
        errors.append(f"experimental S-Print contracts: {exc}")

    if errors:
        print("VALIDATION FAILED")
        for err in errors:
            print(f" - {err}")
        return 1
    print(
        f"OK: {len(schemas)} schemas, {len(EXAMPLE_MAP)} examples, "
        f"{len(fixtures)} fixtures, {len(adv_files)} adversarial fixtures; "
        f"example graph semantically valid at {EXAMPLE_EVAL_TIME}"
    )
    print("OK: 2 experimental S-Print schemas; offline composition example compatible (execution not authorised)")
    return 0


if __name__ == "__main__":
    sys.exit(main())


