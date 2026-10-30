#!/usr/bin/env python3
"""Phase 2 context retrieval engine (Months 5-6: context retrieval).

Implements the retrieval pipeline from docs/CONTEXT_ENGINEERING_PLAN.md:
bind tenant/app/release/operation/host, load policy, reject stale or
revoked manifests, apply hard compatibility filters, retrieve the
signed procedure for the exact operation, retrieve a small number of
comparable success cases and relevant failure signatures, rank with an
explainable score, and assemble a typed ExecutionContextEnvelope.

Hard filters (validate.decide) always run before ranking: semantic
similarity can never override incompatible architecture, insufficient
resources, occupied ports, revoked releases, or missing facts.

Everything here is offline and read-only with respect to hosts: it
never invokes an executor, a live revocation source, or Phase 1
runtime state. Live freshness, real deployment evidence, and any
mutation remain gated on Phase 1.
"""

import json
from dataclasses import dataclass, field
from pathlib import Path

import validate

ROOT = Path(__file__).resolve().parent.parent
EXAMPLES = ROOT / "context" / "examples"
FIXTURES = ROOT / "evals" / "fixtures"
EVAL_TIME = "2026-10-21T09:05:00Z"


# ---------------------------------------------------------------- cases

@dataclass
class Case:
    case_id: str
    outcome: str  # "success" | "failure"
    reason: str
    application_id: str
    evaluation_time: str
    manifest_digest: str
    release_version: str = ""
    host_id: str = ""
    architecture: str = ""
    os_family: str = ""
    tenant_scope: str = ""

    def summary(self):
        return {
            "caseId": self.case_id, "outcome": self.outcome,
            "reason": self.reason, "applicationId": self.application_id,
            "releaseVersion": self.release_version, "hostId": self.host_id,
            "architecture": self.architecture, "osFamily": self.os_family,
            "tenantScope": self.tenant_scope,
            "evaluationTime": self.evaluation_time,
            "manifestDigest": self.manifest_digest,
        }


def ingest_cases(fixtures_dir=FIXTURES):
    """Normalize eval fixtures into success/failure cases.

    A fixture whose expected decision is accept is a success case;
    every rejection fixture is a failure case carrying its reason
    code, which doubles as its failure signature.
    """
    cases = []
    for path in sorted(Path(fixtures_dir).glob("*.json")):
        fixture = validate.load(path)
        expected = fixture.get("expected") or {}
        manifest = fixture.get("manifest") or {}
        host = fixture.get("host") or {}
        cases.append(Case(
            case_id=fixture.get("fixtureId", path.stem),
            outcome="success" if expected.get("decision") == "accept" else "failure",
            reason=expected.get("reasonCode", expected.get("reason", "")),
            application_id=manifest.get("appId", ""),
            evaluation_time=fixture.get("evaluationTime", ""),
            manifest_digest=validate.canonical_digest(manifest) if manifest else "",
            release_version=manifest.get("version", ""),
            host_id=host.get("hostId", ""),
            architecture=host.get("architecture", ""),
            os_family=(host.get("os") or {}).get("family", ""),
            tenant_scope=host.get("tenantScope", ""),
        ))
    return cases


class CaseStore:
    """Append-only JSONL case store with case-id deduplication.

    Success and failure cases remain separate (plan memory model);
    raw logs are never stored, only normalized case summaries.
    """

    def __init__(self, path=None):
        self.path = Path(path) if path else None
        self._cases = {}
        if self.path and self.path.exists():
            for line in self.path.read_text(encoding="utf-8").splitlines():
                if line.strip():
                    record = json.loads(line)
                    self._cases[record["case_id"]] = Case(**record)

    def add(self, case):
        """Store a case; returns False when the id already exists."""
        if case.case_id in self._cases:
            return False
        if self.path is None:
            self._cases[case.case_id] = case
            return True
        self.path.parent.mkdir(parents=True, exist_ok=True)
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(case.__dict__, sort_keys=True) + "\n")
        self._cases[case.case_id] = case
        return True

    def add_many(self, cases):
        return sum(1 for case in cases if self.add(case))

    def __len__(self):
        return len(self._cases)

    def all(self):
        return sorted(self._cases.values(), key=lambda c: c.case_id)

    def for_application(self, application_id, outcome=None):
        return [c for c in self.all()
                if c.application_id == application_id
                and (outcome is None or c.outcome == outcome)]

    def failure_signatures(self, application_id=None, tenant_scope=None):
        """Group failure cases into failure signatures by reason code.

        Scoped to one tenant when tenant_scope is given; counts never
        mix tenants in retrieval (see retrieve_context).
        """
        groups = {}
        pool = self.for_application(application_id or "", None) \
            if application_id else self.all()
        for case in pool:
            if tenant_scope is not None and case.tenant_scope != tenant_scope:
                continue
            if case.outcome != "failure":
                continue
            sig = groups.setdefault(case.reason, {
                "signatureId": f"sig_{case.reason}", "reason": case.reason,
                "count": 0, "caseIds": [], "applicationIds": set(),
            })
            sig["count"] += 1
            sig["caseIds"].append(case.case_id)
            sig["applicationIds"].add(case.application_id)
        result = []
        for sig in groups.values():
            sig = dict(sig)
            sig["applicationIds"] = sorted(sig["applicationIds"])
            result.append(sig)
        return sorted(result, key=lambda s: (-s["count"], s["signatureId"]))

    def comparable_successes(self, manifest, host, tenant_scope=None, limit=3):
        """Top comparable success cases within one tenant: same app
        first, then exact release, then same architecture/OS.

        Tenant scope is mandatory: without it the query fails closed
        instead of leaking another tenant's cases.
        """
        if not tenant_scope:
            raise ValueError("tenant_scope is required for case retrieval")
        def key(case):
            return (
                case.application_id != manifest.get("appId"),
                case.release_version != manifest.get("version"),
                case.architecture != host.get("architecture"),
                case.os_family != (host.get("os") or {}).get("family"),
                case.case_id,
            )
        successes = [c for c in self.all()
                     if c.outcome == "success" and c.tenant_scope == tenant_scope]
        return sorted(successes, key=key)[:limit]

    def relevant_failures(self, manifest, tenant_scope=None, limit=5):
        """Failure cases for the same application within one tenant,
        signature-grouped order (most frequent first), capped."""
        if not tenant_scope:
            raise ValueError("tenant_scope is required for case retrieval")
        failures = [c for c in self.all()
                    if c.outcome == "failure"
                    and c.application_id == manifest.get("appId")
                    and c.tenant_scope == tenant_scope]
        order = {s["reason"]: i for i, s in
                 enumerate(self.failure_signatures(manifest.get("appId"),
                                                   tenant_scope=tenant_scope))}
        return sorted(failures,
                      key=lambda c: (order.get(c.reason, 99), c.case_id))[:limit]


# ------------------------------------------------- trust and validation

_SCHEMA_CACHE = {}


def _validator_for(schema_name):
    if schema_name not in _SCHEMA_CACHE:
        schemas = {name: validate.load(ROOT / "schemas" / name)
                   for name in sorted(validate.REQUIRED_SCHEMAS)}
        _SCHEMA_CACHE[schema_name] = validate.make_validator(
            schemas[schema_name], validate.build_registry(schemas))
    return _SCHEMA_CACHE[schema_name]


def _record_schema_errors(schema_name, record):
    return validate.schema_errors(_validator_for(schema_name), record)


def signature_errors(record, policy, prefix="procedure"):
    """Metadata signature binding only: payloadDigest must equal the
    canonical digest of the record without its signature, the signer
    must be trusted by policy, and the algorithm must be allowed.
    This does NOT cryptographically verify the signature value:
    Cosign verification is the Phase 1 gate and is not wired into
    this offline Python path. Callers must not label a pass here
    as cryptographic or live verification."""
    reasons = []
    signature = record.get("signature")
    if not signature:
        return [f"{prefix}-signature-missing"]
    unsigned = {k: v for k, v in record.items() if k != "signature"}
    if signature.get("payloadDigest") != validate.canonical_digest(unsigned):
        reasons.append(f"{prefix}-signature-digest")
    signing = (policy or {}).get("signing") or {}
    if signature.get("signedBy") not in (signing.get("trustedSigners") or []):
        reasons.append(f"{prefix}-signer-untrusted")
    if signature.get("algorithm") not in (signing.get("algorithms") or []):
        reasons.append(f"{prefix}-algorithm-not-allowed")
    return reasons


ALLOWED_PROCEDURE_STATES = ("signed", "canary", "stable")


def procedure_eligibility(procedure, manifest, policy, evaluation_time):
    """Eligibility gate that runs before any ranking.

    A procedure is eligible only when it is schema-valid, in an
    allowed review state, signature-bound to a trusted signer, and
    scoped to exactly this application release. Ineligible procedures
    are never selected, even as the only candidate.
    """
    reasons = []
    if _record_schema_errors("procedure.schema.json", procedure):
        reasons.append("procedure-schema-invalid")
    if procedure.get("reviewState") not in ALLOWED_PROCEDURE_STATES:
        reasons.append("procedure-review-state")
    reasons.extend(signature_errors(procedure, policy))
    provenance = procedure.get("provenance") or {}
    if provenance.get("applicationScope") not in (None, manifest.get("appId")):
        reasons.append("procedure-application-scope")
    if provenance.get("versionScope") != manifest.get("version"):
        reasons.append("procedure-release-scope")
    expires = provenance.get("expiresAt")
    if expires:
        eval_dt = validate.parse_time(evaluation_time, "evaluation time", [])
        exp_dt = validate.parse_time(expires, "procedure", [])
        if exp_dt and eval_dt and eval_dt >= exp_dt:
            reasons.append("procedure-expired")
    return reasons


def secret_findings(*records):
    """Free-text secret scan over context records.

    The host redaction flag is not trusted on its own: the common
    schema requires secret detection in free-text fields too, so case
    reasons, procedure parameters, and notes are scanned with the
    repository's own patterns (validate.check_secrets).
    """
    errors = []
    for record in records:
        if record is not None:
            validate.check_secrets(record, "context", errors)
    return errors


# ------------------------------------------------------------- ranking

@dataclass
class RankedProcedure:
    procedure: dict
    compatible: bool
    decision: str
    reasons: list = field(default_factory=list)
    score: int = 0
    score_breakdown: dict = field(default_factory=dict)

    def explanation(self):
        parts = [f"{name} +{points}" for name, points
                 in sorted(self.score_breakdown.items(), key=lambda kv: -kv[1])]
        return "; ".join(parts) if parts else "no score (incompatible)"


def _version_key(version):
    parts = []
    for piece in str(version).replace("-", ".").split("."):
        parts.append(int(piece) if piece.isdigit() else 0)
    return tuple(parts)


def _score_procedure(procedure, manifest):
    """Explainable ranking score (compatible procedures only).

    Exact release scope dominates, then signed/stable review state,
    then checkpoint/rollback safety shape. Version and expiry remain
    tie-breakers in the sort, not hidden score inputs.
    """
    breakdown = {}
    provenance = procedure.get("provenance") or {}
    if provenance.get("versionScope") == manifest.get("version"):
        breakdown["exact-release-scope"] = 50
    if procedure.get("reviewState") in ("stable", "canary"):
        breakdown["review-state-stable"] = 20
    elif procedure.get("reviewState") == "signed":
        breakdown["review-state-signed"] = 15
    if procedure.get("requiresCheckpoint"):
        breakdown["checkpoint-before-mutation"] = 10
    if procedure.get("rollbackProcedureId"):
        breakdown["rollback-procedure-declared"] = 10
    caps = procedure.get("capabilitySequence") or []
    if caps and caps[0] == "manifest.verify" and caps[-1] == "health.check":
        breakdown["verify-first-health-last"] = 10
    return sum(breakdown.values()), breakdown


def rank_procedures(procedures, manifest, host, policy, revocation_state,
                    evaluation_time, operation=None):
    """Rank candidate procedures for a manifest/host pair.

    Compatibility is decided by the Phase 0 hard engine
    (validate.decide); ranking then prefers compatible procedures by
    explainable score, higher version, later expiry. Every candidate
    carries its decision reasons so selection is explainable.
    """
    eval_dt = validate.parse_time(evaluation_time, "evaluation time", [])
    ranked = []
    for procedure in procedures:
        # Procedure identity gates run before the manifest/host
        # engine: a procedure for another app or operation can
        # never be selected for this manifest.
        if procedure.get("applicationId") != manifest.get("appId"):
            ranked.append(RankedProcedure(procedure, False, "reject",
                                          ["procedure-application-mismatch"]))
            continue
        if operation is not None and procedure.get("operation") != operation:
            ranked.append(RankedProcedure(procedure, False, "reject",
                                          ["procedure-operation-mismatch"]))
            continue
        # Eligibility (schema, review state, signature binding,
        # release scope) runs before compatibility and ranking: an
        # ineligible procedure can never be selected.
        ineligible = procedure_eligibility(procedure, manifest, policy, evaluation_time)
        if ineligible:
            ranked.append(RankedProcedure(procedure, False, "reject", ineligible))
            continue
        decision, reason = validate.decide(manifest, host, policy,
                                           revocation_state, eval_dt)
        if decision == "accept":
            score, breakdown = _score_procedure(procedure, manifest)
            ranked.append(RankedProcedure(procedure, True, decision, [],
                                          score, breakdown))
        else:
            ranked.append(RankedProcedure(procedure, False, decision, [reason]))
    ranked.sort(key=lambda r: str(r.procedure.get("expiresAt", "")), reverse=True)
    ranked.sort(key=lambda r: _version_key(r.procedure.get("version", "0")), reverse=True)
    ranked.sort(key=lambda r: r.score, reverse=True)
    ranked.sort(key=lambda r: not r.compatible)
    return ranked


def select_procedure(procedures, manifest, host, policy, revocation_state,
                     evaluation_time, operation=None):
    """Return the best compatible procedure, or None (reasons are
    available from rank_procedures)."""
    for candidate in rank_procedures(procedures, manifest, host, policy,
                                     revocation_state, evaluation_time, operation):
        if candidate.compatible:
            return candidate
    return None


# ------------------------------------------------------------ revocation

def revocation_state_for(manifest, revocation_record, evaluation_time):
    """Derive registry state from a revocation record.

    Returns "revoked" when an entry matches the manifest digest or
    app/release, "active" otherwise, and "stale" when the record
    itself is expired or not yet generated at evaluation time —
    callers must fail closed on "stale". A missing record returns
    "active" for fixture/offline use only: retrieve_context(live=True)
    blocks when no verified snapshot is supplied, so no live
    consumer may rely on this default.
    """
    if not revocation_record:
        return "active"
    eval_dt = validate.parse_time(evaluation_time, "evaluation time", [])
    generated = validate.parse_time(revocation_record.get("generatedAt"), "revocation", [])
    expires = validate.parse_time(revocation_record.get("expiresAt"), "revocation", [])
    if not generated or not expires or generated > eval_dt or eval_dt >= expires:
        return "stale"
    digest = validate.canonical_digest(manifest)
    for entry in revocation_record.get("entries") or []:
        if entry.get("manifestDigest") == digest:
            return "revoked"
        if entry.get("applicationId") == manifest.get("appId") and \
                entry.get("releaseVersion") in (None, manifest.get("version")):
            return "revoked"
    return "active"


# ------------------------------------------------------------- retrieval

@dataclass
class RetrievalResult:
    decision: str  # "accept" | "reject" | "blocked"
    reasons: list = field(default_factory=list)
    explanation: list = field(default_factory=list)
    blocked_stage: str = ""
    ranked: list = field(default_factory=list)
    selected: object = None
    success_cases: list = field(default_factory=list)
    failure_cases: list = field(default_factory=list)
    failure_signatures: list = field(default_factory=list)
    envelope: dict = None
    revocation_mode: str = ""  # unchecked | metadata-validated | fixture-record | fixture-no-record


def _case_ref(case):
    """Digest-bound recordRef for a stored case."""
    return {"recordId": case.case_id,
            "digest": validate.canonical_digest(case.summary()),
            "version": "1.0.0"}


def _record_ref(record, id_field):
    return {"recordId": record[id_field],
            "digest": validate.canonical_digest(record),
            "version": record.get("version") or record.get("schemaVersion", "1.0.0")}


def assemble_envelope(template, procedure_case, success_cases, failure_cases,
                      created_at, expires_at, cited_sources=None, binding=None):
    """Assemble an envelope from a template and selected context.

    The template supplies reviewed shape (capabilities, checks,
    rollback strategy); every identity and reference is (re)bound
    from the current records via `binding`, never trusted from the
    template. Callers must run envelope_binding_errors plus schema
    validation before accepting the result.
    """
    envelope = json.loads(json.dumps(template))
    procedure = procedure_case.procedure
    envelope["procedureRef"] = {
        "recordId": procedure["procedureId"],
        "digest": validate.canonical_digest(procedure),
        "version": procedure["version"],
    }
    envelope["selectedSuccessCaseRefs"] = [
        _case_ref(c) for c in sorted(success_cases, key=lambda c: c.case_id)]
    envelope["selectedFailureCaseRefs"] = [
        _case_ref(c) for c in sorted(failure_cases, key=lambda c: c.case_id)]
    if cited_sources is not None:
        envelope["citedSources"] = cited_sources
    if binding:
        envelope.update(binding)
        envelope["procedureRef"] = {
            "recordId": procedure["procedureId"],
            "digest": validate.canonical_digest(procedure),
            "version": procedure["version"],
        }
    envelope["createdAt"] = created_at
    envelope["expiresAt"] = expires_at
    return envelope


def envelope_binding_errors(envelope, request, manifest, host, policy,
                            procedure, artifact):
    """Semantic binding validation: every envelope identity and
    reference must equal the current records and request. Returns a
    list of mismatches; empty means fully bound."""
    errors = []

    def expect(label, got, want):
        if got != want:
            errors.append(f"{label}: envelope has {got!r}, expected {want!r}")

    expect("tenantScope", envelope.get("tenantScope"), request.get("tenantScope"))
    expect("operation", envelope.get("operation"), request.get("operation"))
    expect("applicationId", envelope.get("applicationId"), manifest.get("appId"))
    expect("releaseVersion", envelope.get("releaseVersion"), manifest.get("version"))
    expect("manifestDigest", envelope.get("manifestDigest"),
           validate.canonical_digest(manifest))
    expect("manifestReviewState", envelope.get("manifestReviewState"),
           manifest.get("reviewState"))
    for label, ref, record, id_field in (
            ("hostProfileRef", envelope.get("hostProfileRef") or {}, host, "hostId"),
            ("policyBundleRef", envelope.get("policyBundleRef") or {}, policy, "policyId"),
            ("procedureRef", envelope.get("procedureRef") or {}, procedure, "procedureId")):
        expect(f"{label}.recordId", ref.get("recordId"), record.get(id_field))
        expect(f"{label}.digest", ref.get("digest"), validate.canonical_digest(record))
    if artifact is None:
        errors.append("composeArtifactRef: no current artifact record to bind to")
    else:
        ref = envelope.get("composeArtifactRef") or {}
        expect("composeArtifactRef.recordId", ref.get("recordId"), artifact.get("artifactId"))
        expect("composeArtifactRef.digest", ref.get("digest"),
               validate.canonical_digest(artifact))
    created = validate.parse_time(envelope.get("createdAt"), "envelope", [])
    expires = validate.parse_time(envelope.get("expiresAt"), "envelope", [])
    if not created or not expires or expires <= created:
        errors.append("validity window: expiresAt must be after createdAt")
    return errors


def envelope_schema_errors(envelope):
    schemas = {name: validate.load(ROOT / "schemas" / name)
               for name in sorted(validate.REQUIRED_SCHEMAS)}
    registry = validate.build_registry(schemas)
    validator = validate.make_validator(
        schemas["execution-context-envelope.schema.json"], registry)
    return validate.schema_errors(validator, envelope)


def plan_schema_errors(plan):
    schemas = {name: validate.load(ROOT / "schemas" / name)
               for name in sorted(validate.REQUIRED_SCHEMAS)}
    registry = validate.build_registry(schemas)
    validator = validate.make_validator(
        schemas["deployment-plan.schema.json"], registry)
    return validate.schema_errors(validator, plan)


def plan_binding_errors(plan, envelope, procedure, all_procedures, policy,
                        manifest=None, artifact=None):
    """Plan contract + execution-binding validation.

    Mirrors the plan-relevant sections of validate.semantic_errors:
    exact forward (and rollback) capability order, parameter
    contracts (required + fixed, exact coverage), envelope
    capabilities within policy allowlists minus denials, plan
    composeArtifactRef binding, step parameter agreement with the
    manifest/artifact (service, projectName, manifest/image/artifact
    digests, secret references), unresolvedQuestions, step-id and
    rollback-pointer integrity, and rollback compensation
    (validate.py:399,475,517,746). Full graph semantics that need
    records this path does not own (approvals, case/evidence,
    artifact bytes/render, freshness) remain the validate.py graph
    gate; an accept here is plan-bound, not full-graph validated.
    Returns a list of errors; empty means the plan fully binds.
    """
    errors = []
    if plan.get("envelopeId") != envelope.get("envelopeId"):
        errors.append("plan envelopeId does not match the envelope")
    if plan.get("operation") != envelope.get("operation"):
        errors.append("plan operation does not match envelope operation")
    if plan.get("unresolvedQuestions"):
        errors.append("plan has unresolvedQuestions; execution is blocked")

    # Plan artifact binding (validate.py:398-401), via the artifact
    # record when supplied and via the bound envelope ref otherwise.
    plan_ref = plan.get("composeArtifactRef") or {}
    if artifact is not None:
        if plan_ref.get("recordId") != artifact.get("artifactId"):
            errors.append("plan composeArtifactRef does not reference the envelope compose artifact")
        elif plan_ref.get("digest") != validate.canonical_digest(artifact):
            errors.append("plan composeArtifactRef digest mismatch")
    else:
        env_ref = envelope.get("composeArtifactRef") or {}
        if plan_ref.get("recordId") != env_ref.get("recordId"):
            errors.append("plan composeArtifactRef does not reference the envelope compose artifact")
        elif env_ref.get("digest") and plan_ref.get("digest") != env_ref.get("digest"):
            errors.append("plan composeArtifactRef digest mismatch")

    steps = plan.get("steps") or []
    rollback_steps = plan.get("rollbackSteps") or []

    # Exact capability order (not just membership).
    forward_caps = [s.get("capability") for s in steps]
    proc_sequence = procedure.get("capabilitySequence") or []
    if forward_caps != proc_sequence:
        errors.append("forward capability sequence does not match procedure order")
    # Membership retained for precise per-step diagnostics.
    allowed_proc = set(proc_sequence)
    for step in steps:
        if step.get("capability") not in allowed_proc:
            errors.append(
                f"plan step {step.get('stepId')} capability is outside the signed procedure")

    # Rollback order when a rollback procedure is known.
    rollback_proc = None
    rollback_id = procedure.get("rollbackProcedureId") or (
        (envelope.get("rollbackStrategy") or {}).get("procedureRef") or {}
    ).get("recordId")
    if rollback_id:
        rollback_proc = next(
            (p for p in (all_procedures or []) if p.get("procedureId") == rollback_id),
            None)
    if rollback_steps and rollback_proc:
        rollback_caps = [s.get("capability") for s in rollback_steps]
        if rollback_caps != (rollback_proc.get("capabilitySequence") or []):
            errors.append("rollback capability sequence does not match procedure order")

    # Envelope capabilities must stay within policy allowlist minus denials.
    allowed_env = set(envelope.get("allowedCapabilities") or [])
    allowed_policy = set(policy.get("allowedCapabilities") or [])
    denied_policy = set(policy.get("deniedCapabilities") or [])
    forbidden = set(envelope.get("forbiddenOperations") or [])
    if allowed_env - (allowed_policy - denied_policy):
        errors.append("envelope capabilities exceed policy allowlist or include policy denials")
    for step in steps + rollback_steps:
        cap = step.get("capability")
        if cap not in allowed_env:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' not allowed by envelope")
        if cap not in allowed_policy or cap in denied_policy:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' not allowed by policy")
        if cap in forbidden:
            errors.append(f"step {step.get('stepId')}: capability '{cap}' is forbidden by envelope")

    # Parameter contracts: the signed program surface.
    def check_contracts(sequence_steps, proc, label):
        if not proc:
            return
        contracts = {c["capability"]: c for c in proc.get("parameterContracts", [])}
        sequence = proc.get("capabilitySequence", [])
        if set(contracts) != set(sequence):
            errors.append(f"{label}: parameterContracts must cover exactly the capability sequence")
        for step in sequence_steps:
            contract = contracts.get(step.get("capability"))
            if not contract:
                continue
            params = step.get("parameters") or {}
            missing = [name for name in contract.get("requiredParameters", []) if name not in params]
            if missing:
                errors.append(f"step {step['stepId']}: missing procedure-required parameters {missing}")
            for key, value in (contract.get("fixedParameters") or {}).items():
                if params.get(key) != value:
                    errors.append(f"step {step['stepId']}: parameter '{key}' must equal the procedure-fixed value")

    check_contracts(steps, procedure, "forward procedure")
    if rollback_steps:
        check_contracts(rollback_steps, rollback_proc, "rollback procedure")

    # Step-id and rollback-pointer integrity (validate.py:442-473).
    step_ids = [s.get("stepId") for s in steps]
    rollback_ids = [s.get("stepId") for s in rollback_steps]
    if len(set(step_ids)) != len(steps) or len(set(rollback_ids)) != len(rollback_steps):
        errors.append("step ids must be unique within each sequence")
    if set(step_ids) & set(rollback_ids):
        errors.append("step ids collide between steps and rollbackSteps")
    for step in steps:
        target = step.get("rollbackStepId")
        if target:
            if target in set(step_ids):
                errors.append(f"step {step['stepId']}: rollbackStepId '{target}' points into steps, not rollbackSteps")
            elif target not in set(rollback_ids):
                errors.append(f"step {step['stepId']}: rollbackStepId '{target}' does not exist in rollbackSteps")
    for step in rollback_steps:
        if step.get("rollbackStepId"):
            errors.append(f"rollback step {step['stepId']} must not itself declare a rollbackStepId")

    # Step parameter agreement with manifest/artifact (validate.py:475-498,626-637).
    manifest_digest = validate.canonical_digest(manifest) if manifest else envelope.get("manifestDigest")
    image_digests = {}
    declared_env = set()
    declared_owned = set()
    if manifest:
        image_digests = {img["service"]: img["digest"]
                         for img in (manifest.get("release") or {}).get("images", [])}
        env_entries = (manifest.get("release") or {}).get("environment", [])
        declared_env = {(e.get("name"), e.get("secretRef")) for e in env_entries}
        service_names = list(image_digests)
        sole_service = service_names[0] if len(service_names) == 1 else None
        declared_owned = {(e.get("service") or sole_service, e.get("name"), e.get("secretRef"))
                          for e in env_entries}
    for step in steps + rollback_steps:
        params = step.get("parameters") or {}
        cap = step.get("capability")
        if artifact is not None and "service" in params and params["service"] not in artifact.get("services", []):
            errors.append(f"step {step['stepId']}: service is not declared in compose artifact")
        if artifact is not None and "projectName" in params and params["projectName"] != artifact.get("projectName"):
            errors.append(f"step {step['stepId']}: projectName does not match artifact")
        if cap == "manifest.verify" and params.get("manifestDigest") != manifest_digest:
            errors.append(f"step {step['stepId']}: manifest.verify digest does not match manifest")
        if cap == "compose.pull" and manifest and params.get("imageDigest") != image_digests.get(params.get("service")):
            errors.append(f"step {step['stepId']}: compose.pull imageDigest does not match manifest image digest")
        if cap == "compose.up":
            if artifact is not None and params.get("composeArtifactDigest") != artifact.get("composeDigest"):
                errors.append(f"step {step['stepId']}: compose.up composeArtifactDigest does not match artifact")
            if artifact is not None and params.get("projectName") != artifact.get("projectName"):
                errors.append(f"step {step['stepId']}: compose.up projectName does not match artifact")
        if cap == "secret.inject":
            known = {ref["secretRef"] for ref in envelope.get("secretReferences", [])}
            if params.get("secretRef") not in known:
                errors.append(f"step {step['stepId']}: secret.inject references a secretRef not declared in the envelope")
            if manifest and (params.get("targetEnv"), params.get("secretRef")) not in declared_env:
                errors.append(f"step {step['stepId']}: secret target does not match manifest environment")
            if manifest and (params.get("service"), params.get("targetEnv"), params.get("secretRef")) not in declared_owned:
                errors.append(f"step {step['stepId']}: secret.inject (service, environment, secretRef) does not match a manifest environment entry owned by that service")

    # Rollback compensation (validate.py:746-777).
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
            continue
        want_capability, resource_keys = expected
        if target.get("capability") != want_capability:
            errors.append(f"step {step['stepId']}: compensation must be '{want_capability}' but rollbackStepId points to '{target.get('capability')}'")
            continue
        for key in resource_keys:
            if (target.get("parameters") or {}).get(key) != (step.get("parameters") or {}).get(key):
                errors.append(f"step {step['stepId']}: compensation resource '{key}' does not match the mutated resource")
    return errors


def retrieve_context(request, manifest, host, policy, procedures, store,
                     template_envelope=None, artifact=None, plan=None,
                     revocation_record=None,
                     registry_state="active", evaluation_time=EVAL_TIME,
                     top_k=3, live=False, accepted_revocation_sequence=None):
    """Run the full retrieval pipeline for one operation request.

    request keys: tenantScope, applicationId, releaseVersion,
    operation, hostId. Decision is "accept" only when binding, tenant
    isolation, redaction (flag plus free-text scan), revocation,
    the hard engine, eligible procedure selection, tenant-scoped
    case retrieval, and envelope schema+binding validation all pass.

    Revocation modes: with live=True a current, non-rolled-back
    revocation snapshot is mandatory and its absence blocks. The
    snapshot check in this offline path is metadata validation
    only (payloadDigest, signer, algorithm) — it does NOT
    cryptographically verify the signature value, so the result
    is labelled "metadata-validated", never "live-verified".
    Live mode also requires the caller to supply a persisted
    accepted_revocation_sequence cursor; when it is None the
    call blocks instead of defaulting to 0. With live=False
    (the default, fixture/offline mode) a missing record is
    explicitly reported in the result and explanation —
    fixture-only behavior that no live consumer may rely on.
    """
    explain, reasons = [], []
    result = RetrievalResult(decision="reject")

    def block(stage, reason, note):
        result.decision = "blocked"
        result.blocked_stage = stage
        result.reasons = reasons + [reason]
        explain.append(note)
        result.explanation = explain
        # Clear every field that can carry free-text record content
        # so a secret that triggered (or merely coexists with) the
        # block cannot reach the caller via the result or _result_json.
        result.success_cases = []
        result.failure_cases = []
        result.failure_signatures = []
        result.envelope = None
        if stage == "redaction":
            result.ranked = []
            result.selected = None
        return result

    # 1. Bind tenant, app, release, operation, and target host.
    if not request.get("tenantScope"):
        return block("bind", "tenant-scope-missing",
                     "Request carries no tenant scope; failing closed.")
    if manifest.get("appId") != request.get("applicationId") or \
            manifest.get("version") != request.get("releaseVersion"):
        return block("bind", "binding-mismatch",
                     "Manifest does not match the requested app/release; stopped at binding.")
    if host.get("hostId") != request.get("hostId"):
        return block("bind", "binding-mismatch",
                     "Host profile does not match the requested host; stopped at binding.")
    explain.append(f"Bound {request.get('applicationId')} {request.get('releaseVersion')} "
                   f"({request.get('operation')}) to {request.get('hostId')} "
                   f"for tenant {request.get('tenantScope')}.")

    # 2. Tenant isolation: every record must share the request tenant.
    tenant = request.get("tenantScope")
    if host.get("tenantScope") != tenant or policy.get("tenantScope") != tenant:
        return block("tenant", "tenant-mismatch",
                     "Host or policy tenant differs from the request tenant; isolation enforced.")
    explain.append(f"Tenant isolation verified for {tenant}.")

    # 3. Redaction: the host flag plus a free-text secret scan over
    # every context record (the flag alone is never trusted).
    if (host.get("redaction") or {}).get("secretValuesPresent"):
        return block("redaction", "secret-values-in-context",
                     "Host profile carries secret values; retrieval blocked until redacted.")
    if secret_findings(manifest, host, policy, *procedures):
        return block("redaction", "secret-pattern-in-context",
                     "Free-text secret pattern found in context records; values suppressed, retrieval blocked.")
    explain.append("Redaction verified: no secret values or free-text secret patterns in context.")

    # 4-5. Revocation metadata, freshness, then stale/revoked rejection.
    # NOTE: this path checks payloadDigest/signer/algorithm metadata
    # only; it does not cryptographically verify the signature value.
    if live:
        # Only a snapshot that passes every check below earns
        # "metadata-validated"; blocked live results stay "unchecked".
        result.revocation_mode = "unchecked"
        if accepted_revocation_sequence is None:
            return block("revocation", "revocation-sequence-cursor-required",
                         "Live path requires a persisted accepted revocation sequence cursor; none supplied.")
        if revocation_record is None:
            return block("revocation", "revocation-snapshot-required",
                         "Live path requires a revocation snapshot; none supplied.")
        if _record_schema_errors("revocation-record.schema.json", revocation_record):
            return block("revocation", "revocation-schema-invalid",
                         "Revocation snapshot fails its schema; failing closed.")
        sig_reasons = signature_errors(revocation_record, policy, prefix="revocation")
        if sig_reasons:
            return block("revocation", sig_reasons[0],
                         "Revocation snapshot signature metadata (digest/signer/algorithm) failed.")
        sequence = revocation_record.get("sequence") or 0
        if sequence < 1 or sequence < accepted_revocation_sequence:
            return block("revocation", "revocation-sequence-rollback",
                         "Revocation sequence is missing or rolled back; failing closed.")
        registry_state = revocation_state_for(manifest, revocation_record, evaluation_time)
        if registry_state == "stale":
            return block("revocation", "revocation-state-stale",
                         "Revocation record is expired or not yet valid; failing closed.")
        result.revocation_mode = "metadata-validated"
        explain.append(f"Revocation snapshot metadata validated (sequence {sequence}, "
                       f"payloadDigest/signer/algorithm checked, current; "
                       f"cryptographic Cosign verification not performed in this offline path).")
    elif revocation_record is not None:
        result.revocation_mode = "fixture-record"
        registry_state = revocation_state_for(manifest, revocation_record, evaluation_time)
        if registry_state == "stale":
            return block("revocation", "revocation-state-stale",
                         "Revocation record is expired or not yet valid; failing closed.")
        explain.append("Revocation record checked for freshness (fixture mode: "
                       "signature/sequence not enforced — live path requires them).")
    else:
        result.revocation_mode = "fixture-no-record"
        explain.append("Fixture mode: no revocation record supplied, defaulting to "
                       "the caller's registry state. A live consumer must use live=True, "
                       "which blocks in this situation.")
    eval_dt = validate.parse_time(evaluation_time, "evaluation time", [])

    # 6. Hard compatibility filters before any ranking.
    decision, reason = validate.decide(manifest, host, policy, registry_state, eval_dt)
    if decision != "accept":
        # Scan before attaching: failure cases/signatures carry
        # free-text reasons that must not leak on this path either.
        _failures = store.relevant_failures(manifest, tenant_scope=tenant)
        _signatures = store.failure_signatures(
            manifest.get("appId"), tenant_scope=tenant)
        _pool = [c.summary() for c in store.all() if c.tenant_scope == tenant]
        if secret_findings(*_pool, *_signatures):
            return block("redaction", "secret-pattern-in-context",
                         "Free-text secret pattern found in a cited case; retrieval blocked.")
        result.decision = "reject"
        result.reasons = [reason]
        result.blocked_stage = "compatibility"
        explain.append(f"Hard compatibility engine rejected: {reason}. Ranking never ran.")
        result.explanation = explain
        result.failure_signatures = _signatures
        result.failure_cases = _failures
        return result
    explain.append("Hard compatibility engine accepted the manifest/host pair.")

    # 7. Eligible signed procedure for the exact operation, ranked.
    result.ranked = rank_procedures(procedures, manifest, host, policy,
                                    registry_state, evaluation_time,
                                    operation=request.get("operation"))
    result.selected = next((r for r in result.ranked if r.compatible), None)
    if result.selected is None:
        result.reasons = sorted({reason for r in result.ranked for reason in r.reasons})
        return block("procedure", "no-eligible-procedure",
                     "No eligible, compatible signed procedure for this operation; "
                     "ineligible candidates were excluded before ranking.")
    proc = result.selected.procedure
    explain.append(f"Selected {proc['procedureId']} v{proc['version']} "
                   f"(score {result.selected.score}: {result.selected.explanation()}).")

    # 8-9. Tenant-scoped comparable successes and failure signatures.
    # Scan BEFORE attaching to the result: the tenant's whole case
    # pool is scanned (not only the cited cap) so a secret in an
    # uncited case reason/signature cannot leak via the result.
    _successes = store.comparable_successes(
        manifest, host, tenant_scope=tenant, limit=top_k)
    _failures = store.relevant_failures(manifest, tenant_scope=tenant)
    _signatures = store.failure_signatures(
        manifest.get("appId"), tenant_scope=tenant)
    tenant_pool = [c.summary() for c in store.all() if c.tenant_scope == tenant]
    if secret_findings(*tenant_pool, *_signatures):
        return block("redaction", "secret-pattern-in-context",
                     "Free-text secret pattern found in a cited case; retrieval blocked.")
    result.success_cases = _successes
    result.failure_cases = _failures
    result.failure_signatures = _signatures
    explain.append(f"Cited {len(result.success_cases)} success and "
                   f"{len(result.failure_cases)} failure cases from tenant {tenant} across "
                   f"{len(result.failure_signatures)} failure signatures.")

    # 10. Assemble, bind, and validate the envelope before accepting.
    result.decision = "accept"
    result.explanation = explain
    if template_envelope is not None:
        if artifact is None:
            return block("envelope", "artifact-record-required",
                         "An envelope template was supplied without the current "
                         "compose artifact record; cannot bind or accept.")
        cited = [{"recordId": (manifest.get("provenance") or {}).get("sourceId", ""),
                  "contentHash": (manifest.get("provenance") or {}).get("contentHash", "")},
                 {"recordId": host["hostId"], "contentHash": validate.canonical_digest(host)}]
        cited = [c for c in cited if c["recordId"] and c["contentHash"]]
        binding = {
            "tenantScope": tenant,
            "operation": request.get("operation"),
            "applicationId": manifest.get("appId"),
            "releaseVersion": manifest.get("version"),
            "manifestDigest": validate.canonical_digest(manifest),
            "manifestReviewState": manifest.get("reviewState"),
            "hostProfileRef": _record_ref(host, "hostId"),
            "policyBundleRef": _record_ref(policy, "policyId"),
            "composeArtifactRef": _record_ref(artifact, "artifactId"),
        }
        envelope = assemble_envelope(
            template_envelope, result.selected, result.success_cases,
            result.failure_cases, evaluation_time,
            host.get("expiresAt", evaluation_time),
            cited_sources=cited, binding=binding)
        if envelope_schema_errors(envelope):
            return block("envelope", "envelope-schema-invalid",
                         "Assembled envelope fails the canonical schema; not accepting.")
        binding_errors = envelope_binding_errors(
            envelope, request, manifest, host, policy, proc, artifact)
        if binding_errors:
            result.reasons = binding_errors
            return block("envelope", "envelope-binding-mismatch",
                         "Assembled envelope does not bind to the current records; not accepting.")
        if plan is not None:
            if plan_schema_errors(plan):
                return block("envelope", "plan-schema-invalid",
                             "Supplied plan fails the canonical deployment-plan schema; not accepting.")
            plan_errors = plan_binding_errors(plan, envelope, proc, procedures, policy,
                                              manifest=manifest, artifact=artifact)
            if plan.get("operation") != request.get("operation"):
                if "plan operation does not match envelope operation" not in plan_errors:
                    plan_errors.append("plan operation does not match the request")
            if plan_errors:
                result.reasons = plan_errors
                return block("envelope", "envelope-plan-mismatch",
                             "Plan does not bind to the envelope and signed procedure; not accepting.")
        if secret_findings(envelope):
            return block("redaction", "secret-pattern-in-context",
                         "Free-text secret pattern found in the assembled envelope; not accepting.")
        result.envelope = envelope
        explain.append("Envelope assembled, rebound to the current records, and "
                       "schema+binding validated before acceptance.")
    return result


# ------------------------------------------------------------ evaluation

def evaluate_retrieval(fixtures_dir=FIXTURES):
    """Score ONLY the hard compatibility engine against the fixtures.

    These metrics describe validate.decide in isolation — not
    end-to-end retrieval. They are named accordingly; end-to-end
    outcomes (tenant filtering, procedure eligibility, revocation,
    envelope binding) are measured by evaluate_end_to_end().
    """
    total = correct = false_compatible = 0
    stale_total = stale_correct = 0
    per_fixture = []
    _schemas = {name: validate.load(ROOT / "schemas" / name)
                for name in sorted(validate.REQUIRED_SCHEMAS)}
    _registry = validate.build_registry(_schemas)
    raw, _ = validate.load_examples(_registry, _schemas)
    default_policy = raw.get("policy-bundle.json") or {}
    # Mirror scripts/validate.py fixture semantics exactly: schema-
    # invalid records are missing-mandatory-fact before the engine.
    _mv = validate.make_validator(_schemas["app-manifest.schema.json"], _registry)
    _hv = validate.make_validator(_schemas["host-profile.schema.json"], _registry)
    _pv = validate.make_validator(_schemas["policy-bundle.schema.json"], _registry)
    for path in sorted(Path(fixtures_dir).glob("*.json")):
        fixture = validate.load(path)
        manifest, host = fixture.get("manifest") or {}, fixture.get("host") or {}
        policy = fixture.get("policy") or default_policy
        eval_dt = validate.parse_time(fixture.get("evaluationTime"), path.name, [])
        record_errors = (validate.schema_errors(_mv, manifest)
                         + validate.schema_errors(_hv, host)
                         + (validate.schema_errors(_pv, policy) if fixture.get("policy") else []))
        if (manifest.get("requirements") or {}).get("privileged") is True:
            got = ("reject", "privileged-not-supported")
        elif record_errors:
            got = ("reject", "missing-mandatory-fact")
        else:
            got = validate.decide(manifest, host, policy,
                                  fixture.get("registryState", "active"), eval_dt)
        want = ((fixture.get("expected") or {}).get("decision"),
                (fixture.get("expected") or {}).get("reasonCode"))
        total += 1
        correct += got == want
        if got[0] == "accept" and want[0] != "accept":
            false_compatible += 1
        if want[1] in ("stale-host-profile", "host-profile-expired", "revoked-manifest"):
            stale_total += 1
            stale_correct += got == want
        per_fixture.append({"fixture": path.stem, "expected": list(want),
                            "got": list(got), "correct": got == want})
    return {
        "fixtures": total, "correct": correct,
        "hardEngineDecisionAccuracy": round(correct / total, 4) if total else 0,
        "hardEngineFalseCompatibleRate": round(false_compatible / total, 4) if total else 0,
        "staleRejectionRecall": round(stale_correct / stale_total, 4) if stale_total else None,
        "perFixture": per_fixture,
    }


def evaluate_end_to_end():
    """Adversarial end-to-end retrieval evaluation.

    Each scenario runs the full retrieve_context pipeline (binding,
    tenant isolation, redaction scan, revocation, hard filters,
    procedure eligibility, tenant-scoped cases, envelope binding).
    falseAcceptRate counts scenarios that must not accept but did —
    the safety-critical end-to-end number.
    """
    import copy
    graph = load_example_graph()
    manifest, host, policy = graph["manifest"], graph["host"], graph["policy"]
    procedures, template = graph["procedures"], graph["envelope"]
    artifact, plan, revocations = graph["artifact"], graph["plan"], graph["revocations"]
    store = CaseStore()
    store.add_many(ingest_cases())
    request = {"tenantScope": "tenant_demo", "applicationId": manifest["appId"],
               "releaseVersion": manifest["version"], "operation": "install",
               "hostId": host["hostId"]}
    install = next(p for p in procedures if p["operation"] == "install")

    def run(**overrides):
        args = dict(request=request, manifest=manifest, host=host, policy=policy,
                    procedures=procedures, store=store, template_envelope=template,
                    artifact=artifact, plan=plan, revocation_record=revocations)
        args.update(overrides)
        return retrieve_context(**args)

    busy = copy.deepcopy(host)
    busy["occupiedPorts"] = sorted(set(busy.get("occupiedPorts", [])) | {3000})
    tampered = copy.deepcopy(install)
    tampered["version"] = "9.9.9"
    unsigned = copy.deepcopy(install)
    unsigned["reviewState"] = "candidate"
    unsigned.pop("signature", None)
    secret_host = copy.deepcopy(host)
    secret_host["redaction"] = {"applied": True, "secretValuesPresent": True}
    shell_template = copy.deepcopy(template)
    shell_template["allowedCapabilities"] = ["shell.exec"]

    scenarios = [
        ("compatible-live", run(live=True, accepted_revocation_sequence=1), "accept"),
        ("occupied-port", run(host=busy), "not-accept"),
        ("revoked-manifest", run(registry_state="revoked", revocation_record=None), "not-accept"),
        ("cross-tenant", run(request=dict(request, tenantScope="tenant_other")), "not-accept"),
        ("missing-tenant-scope", run(request={k: v for k, v in request.items()
                                              if k != "tenantScope"}), "not-accept"),
        ("tampered-procedure-only", run(procedures=[tampered]), "not-accept"),
        ("unsigned-procedure-only", run(procedures=[unsigned]), "not-accept"),
        ("live-without-revocation", run(live=True, revocation_record=None,
                                       accepted_revocation_sequence=1), "not-accept"),
        ("live-without-cursor", run(live=True), "not-accept"),
        ("revocation-sequence-rollback",
         run(live=True, accepted_revocation_sequence=99), "not-accept"),
        ("secret-values-in-host", run(host=secret_host), "not-accept"),
        ("shell-capability-envelope", run(template_envelope=shell_template), "not-accept"),
        ("wrong-release-binding",
         run(request=dict(request, releaseVersion="9.9.9")), "not-accept"),
    ]
    results, false_accepts = [], 0
    for name, result, want in scenarios:
        ok = (result.decision == "accept") if want == "accept" else (result.decision != "accept")
        if result.decision == "accept" and want != "accept":
            false_accepts += 1
        results.append({"scenario": name, "expected": want,
                        "decision": result.decision, "reasons": result.reasons,
                        "blockedStage": result.blocked_stage, "correct": ok})
    total = len(scenarios)
    correct = sum(1 for r in results if r["correct"])
    return {
        "scenarios": total, "correct": correct,
        "endToEndAccuracy": round(correct / total, 4) if total else 0,
        "falseAcceptRate": round(false_accepts / total, 4) if total else 0,
        "perScenario": results,
    }


# --------------------------------------------------------- demo packet

def load_example_graph():
    schemas = {name: validate.load(ROOT / "schemas" / name)
               for name in sorted(validate.REQUIRED_SCHEMAS)}
    raw, errors = validate.load_examples(validate.build_registry(schemas), schemas)
    if errors:
        raise RuntimeError(f"example graph failed to load: {errors[:3]}")
    return validate.graph_from_examples(raw)


def _ranked_json(ranked):
    return [{
        "procedureId": r.procedure.get("procedureId"),
        "version": r.procedure.get("version"),
        "operation": r.procedure.get("operation"),
        "compatible": r.compatible, "decision": r.decision,
        "reasons": r.reasons, "score": r.score,
        "scoreBreakdown": r.score_breakdown,
        "explanation": r.explanation(),
    } for r in ranked]


def _result_json(result):
    # Defence in depth: a redaction block never serializes case
    # content, even if a caller hand-built the result object.
    redacted = result.decision == "blocked" and result.blocked_stage == "redaction"
    return {
        "decision": result.decision, "reasons": result.reasons,
        "blockedStage": result.blocked_stage,
        "revocationMode": result.revocation_mode,
        "explanation": result.explanation,
        "ranked": [] if redacted else _ranked_json(result.ranked),
        "selectedProcedure": None if redacted else (
            result.selected.procedure.get("procedureId")
            if result.selected else None),
        "successCases": [] if redacted else [c.summary() for c in result.success_cases],
        "failureCases": [] if redacted else [c.summary() for c in result.failure_cases],
        "failureSignatures": [] if redacted else result.failure_signatures,
        "envelope": None if redacted else result.envelope,
    }


def build_demo_packet():
    """Three retrieval scenarios for the demo: one accept, two denies.

    Denials are produced by the real pipeline (occupied port, revoked
    manifest), never hand-written into the packet.
    """
    import copy
    graph = load_example_graph()
    manifest, host, policy = graph["manifest"], graph["host"], graph["policy"]
    procedures = graph["procedures"]
    store = CaseStore()
    store.add_many(ingest_cases())
    request = {"tenantScope": "tenant_demo",
               "applicationId": manifest["appId"],
               "releaseVersion": manifest["version"],
               "operation": "install", "hostId": host["hostId"]}
    template = graph["envelope"]

    # Demo uses the offline metadata-validated path: the fixture
    # revocation record's signature value is not cryptographic, so
    # it is labelled metadata-validated (never live-verified) and
    # requires an explicit persisted cursor (fixture sequence is 1).
    common = dict(template_envelope=template, artifact=graph["artifact"],
                  plan=graph["plan"], revocation_record=graph["revocations"],
                  live=True, accepted_revocation_sequence=1)
    accepted = retrieve_context(request, manifest, host, policy, procedures,
                                store, **common)

    busy_host = copy.deepcopy(host)
    busy_host["occupiedPorts"] = sorted(set(busy_host.get("occupiedPorts", [])) | {3000})
    busy = retrieve_context(request, manifest, busy_host, policy, procedures,
                            store, **common)

    revoked = retrieve_context(request, manifest, host, policy, procedures,
                               store, template_envelope=template,
                               artifact=graph["artifact"], plan=graph["plan"],
                               registry_state="revoked")

    return {
        "generatedFrom": "scripts/context_engine.py (Phase 2 retrieval pipeline)",
        "evaluationTime": EVAL_TIME,
        "request": request,
        "records": {
            "manifest": {"appId": manifest["appId"], "name": manifest.get("name"),
                         "version": manifest["version"],
                         "reviewState": manifest.get("reviewState"),
                         "digest": validate.canonical_digest(manifest)},
            "host": {"hostId": host["hostId"], "tenantScope": host["tenantScope"],
                     "os": host.get("os"), "architecture": host.get("architecture"),
                     "resources": host.get("resources"),
                     "occupiedPorts": host.get("occupiedPorts")},
        },
        "planSteps": [{"stepId": s["stepId"], "capability": s["capability"],
                       "requiresApproval": bool(s.get("requiresApproval"))}
                      for s in (graph["plan"] or {}).get("steps", [])],
        "approval": graph["approvals"][0] if graph["approvals"] else None,
        "case": graph["case"],
        "evidence": graph["evidence"],
        "metrics": {**evaluate_retrieval(), **evaluate_end_to_end()},
        "scenarios": [
            {"id": "compatible", "label": "Compatible host — install proceeds to approval",
             "result": _result_json(accepted)},
            {"id": "occupied-port", "label": "Port 3000 already occupied — denied at compatibility",
             "result": _result_json(busy)},
            {"id": "revoked", "label": "Manifest revoked — denied before ranking",
             "result": _result_json(revoked)},
        ],
    }


if __name__ == "__main__":
    import sys
    if "--packet" in sys.argv:
        print(json.dumps(build_demo_packet(), indent=1))
    elif "--evaluate" in sys.argv:
        print(json.dumps({**evaluate_retrieval(), **evaluate_end_to_end()}, indent=1))
    else:
        print("usage: context_engine.py --packet | --evaluate")
