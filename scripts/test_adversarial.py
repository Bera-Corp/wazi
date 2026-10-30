#!/usr/bin/env python3
"""Semantic adversarial tests for the Phase 0 example deployment graph.

Each case mutates a copy of the valid example graph and asserts that
cross-record validation rejects it. Run after validate.py:
    python scripts/test_adversarial.py
"""
import copy
import importlib.util
import sys
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("wazi_validate", HERE / "validate.py")
validate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(validate)

ROOT = HERE.parent


def load_graph():
    raw = {}
    examples = ROOT / "context" / "examples"
    for name in validate.EXAMPLE_MAP:
        raw[name] = validate.load(examples / name)
    graph = validate.graph_from_examples(raw)
    import hashlib
    import importlib.util as _ilu
    artifact_path = ROOT / graph["artifact"]["localPath"]
    graph["artifactFileDigest"] = "sha256:" + hashlib.sha256(artifact_path.read_bytes()).hexdigest()
    _spec = _ilu.spec_from_file_location("render_compose", ROOT / "scripts" / "render_compose.py")
    _mod = _ilu.module_from_spec(_spec)
    _spec.loader.exec_module(_mod)
    graph["artifactRenderedDigest"] = "sha256:" + hashlib.sha256(_mod.render(graph["manifest"])).hexdigest()
    graph["rendererVersion"] = _mod.RENDERER_VERSION
    return graph


EVAL_TIME = datetime(2026, 10, 21, 9, 5, tzinfo=timezone.utc)


def expect_rejected(label, mutate, expected_error=None, rebind=False):
    graph = load_graph()
    mutate(graph)
    if rebind:
        bind_graph(graph)
    errors = validate.semantic_errors(graph, EVAL_TIME)
    rejected = bool(errors) and (expected_error is None or any(expected_error in e for e in errors))
    status = "PASS" if rejected else "FAIL"
    print(f"{status} {label}" + (f" -> {errors[0]}" if errors else " (unexpectedly accepted)"))
    return rejected


def bind_graph(graph):
    """Build fresh bindings so a negative test cannot pass on an incidental stale hash."""
    envelope = graph["envelope"]
    for key, record in (("hostProfileRef", graph["host"]), ("policyBundleRef", graph["policy"])):
        envelope[key]["digest"] = validate.canonical_digest(record)
    procedures = {p["procedureId"]: p for p in graph["procedures"]}
    for ref in (envelope["procedureRef"], envelope["rollbackStrategy"]["procedureRef"]):
        if ref["recordId"] in procedures:
            ref["digest"] = validate.canonical_digest(procedures[ref["recordId"]])
    envelope["composeArtifactRef"]["digest"] = validate.canonical_digest(graph["artifact"])
    graph["plan"]["composeArtifactRef"]["digest"] = validate.canonical_digest(graph["artifact"])
    for source in envelope["citedSources"]:
        if source["recordId"] == graph["host"]["hostId"]:
            source["contentHash"] = validate.canonical_digest(graph["host"])
    for approval in graph["approvals"]:
        approval["planDigest"] = validate.canonical_digest(graph["plan"])
        approval["envelopeDigest"] = validate.canonical_digest(envelope)


def two_maintainers_one_expired(graph):
    for rule in graph["policy"]["approvalRules"]:
        if rule["action"] == "public-exposure":
            rule["approval"] = "two-maintainers"
    first = graph["approvals"][0]
    first["approver"] = {"approverId": "maintainer_one", "role": "maintainer"}
    second = copy.deepcopy(first)
    second["approvalId"] = "appr_expired"
    second["approver"]["approverId"] = "maintainer_two"
    second["expiresAt"] = "2026-10-21T09:04:00Z"
    graph["approvals"].append(second)


def main():
    base_errors = validate.semantic_errors(load_graph(), EVAL_TIME)
    print(("PASS base graph accepted" if not base_errors else f"FAIL base graph: {base_errors}"))
    results = [not base_errors]

    results.append(expect_rejected(
        "capability outside envelope allowlist",
        lambda g: g["plan"]["steps"].append({
            "stepId": "step_sneak_backup", "capability": "volume.backup",
            "parameters": {"volume": "app-data", "backupId": "bak_sneak"},
            "citations": ["host_demo01"], "requiresApproval": False,
            "verification": {"type": "exit-state"}})))
    results.append(expect_rejected(
        "cross-tenant host profile",
        lambda g: g["host"].update(tenantScope="tenant_other")))
    results.append(expect_rejected(
        "unresolved citation",
        lambda g: g["plan"]["steps"][0].update(citations=["no_such_record"])))
    results.append(expect_rejected(
        "rollbackStepId pointing into steps",
        lambda g: g["plan"]["steps"][4].update(rollbackStepId="step_pull")))
    results.append(expect_rejected(
        "rollbackStepId pointing nowhere (original Phase 0 defect)",
        lambda g: g["plan"]["steps"][4].update(rollbackStepId="step_missing")))
    results.append(expect_rejected(
        "plan changed after approval (approval replay on edited plan)",
        lambda g: g["plan"]["steps"][6]["parameters"].update(target="http://localhost:9999/evil")))
    results.append(expect_rejected(
        "approval for a different tenant (cross-tenant approval replay)",
        lambda g: g["approvals"][0].update(tenantScope="tenant_other")))
    results.append(expect_rejected(
        "approval missing entirely",
        lambda g: g.update(approvals=[])))
    results.append(expect_rejected(
        "approval expired at evaluation time",
        lambda g: g["approvals"][0].update(expiresAt="2026-10-21T09:04:00Z")))
    results.append(expect_rejected(
        "host changed between preflight and execution (digest binding)",
        lambda g: g["host"]["resources"].update(ramMb=1024)))
    results.append(expect_rejected(
        "host profile expires before it was observed (timestamp ordering)",
        lambda g: g["host"].update(expiresAt="2026-10-21T08:00:00Z")))
    results.append(expect_rejected(
        "secret.inject with undeclared secretRef",
        lambda g: g["plan"]["steps"][3]["parameters"].update(secretRef="secret://deployment/undeclared")))
    results.append(expect_rejected(
        "undefined procedure reference",
        lambda g: g["envelope"]["procedureRef"].update(recordId="proc_undefined")))

    results.append(expect_rejected(
        "policy public denial overrides approved plan",
        lambda g: g["policy"]["network"].update(allowPublicExposure=False),
        "public exposure is denied by policy", rebind=True))
    results.append(expect_rejected(
        "procedure order is enforced even with fresh approval",
        lambda g: g["plan"]["steps"].reverse(),
        "forward capability sequence", rebind=True))
    results.append(expect_rejected(
        "duplicate step ids",
        lambda g: g["plan"]["steps"][1].update(stepId="step_verify_manifest"),
        "step ids must be unique", rebind=True))
    results.append(expect_rejected(
        "two maintainer quorum counts live approvals only",
        two_maintainers_one_expired,
        "two distinct live maintainer approvals", rebind=True))
    results.append(expect_rejected(
        "future approval is not live",
        lambda g: g["approvals"][0].update(approvedAt="2026-10-21T09:06:00Z"),
        "not yet valid"))
    results.append(expect_rejected(
        "approval expires exactly at boundary",
        lambda g: g["approvals"][0].update(expiresAt="2026-10-21T09:05:00Z"),
        "expired"))
    results.append(expect_rejected(
        "future host rejected with rebuilt envelope",
        lambda g: g["host"].update(observedAt="2026-10-21T09:06:00Z"),
        "cannot originate in the future", rebind=True))
    results.append(expect_rejected(
        "host age policy enforced by semantic gate",
        lambda g: g["policy"]["freshness"].update(maxHostProfileAgeMinutes=1),
        "exceeds policy max age", rebind=True))
    results.append(expect_rejected(
        "unused envelope capability still must be policy-authorised",
        lambda g: g["policy"]["deniedCapabilities"].append("compose.down"),
        "envelope capabilities exceed policy", rebind=True))
    results.append(expect_rejected(
        "envelope edits invalidate existing approval",
        lambda g: g["envelope"]["forbiddenOperations"].append("new-restriction"),
        "no approval bound"))
    results.append(expect_rejected(
        "rollback cannot target another project",
        lambda g: g["plan"]["rollbackSteps"][1]["parameters"].update(projectName="another-project"),
        "projectName does not match artifact", rebind=True))
    results.append(expect_rejected(
        "forged cited source hash",
        lambda g: g["envelope"]["citedSources"][0].update(contentHash="sha256:" + "0" * 64),
        "citation contentHash", rebind=True))

    results.append(expect_rejected(
        "case references missing evidence record",
        lambda g: g["case"].update(evidenceRefs=["ev_missing"]),
        "has no evidence record"))
    results.append(expect_rejected(
        "evidence belongs to a different case",
        lambda g: g["evidence"][0].update(deploymentCaseId="case_other"),
        "different deployment case"))
    results.append(expect_rejected(
        "failing evidence cannot support a success case",
        lambda g: g["evidence"][0].update(result="fail"),
        "does not pass"))
    results.append(expect_rejected(
        "evidence bound to a different release",
        lambda g: g["evidence"][0].update(commitOrReleaseDigest="sha256:" + "0" * 64),
        "release digest does not match"))
    results.append(expect_rejected(
        "case bound to a different plan",
        lambda g: g["case"].update(planId="plan_other"),
        "planId does not match"))
    results.append(expect_rejected(
        "mutating proxy step without compensation",
        lambda g: g["plan"]["steps"][5].pop("rollbackStepId"),
        "has no compensation", rebind=True))
    results.append(expect_rejected(
        "procedure-fixed parameter changed",
        lambda g: g["plan"]["steps"][6]["parameters"].update(expectedStatus=201),
        "must equal the procedure-fixed value", rebind=True))
    results.append(expect_rejected(
        "revoked manifest in graph",
        lambda g: g["revocations"].update(entries=[{
            "applicationId": "org.example.uptime-demo",
            "revokedAt": "2026-10-21T08:30:00Z", "reason": "test revocation"}]),
        "manifest is revoked"))
    results.append(expect_rejected(
        "stale revocation list fails closed",
        lambda g: g["revocations"].update(expiresAt="2026-10-21T09:04:00Z"),
        "revocation list is stale"))
    results.append(expect_rejected(
        "licence outside policy allowlist (semantic)",
        lambda g: g["policy"]["licensing"].update(allowedLicenses=["Apache-2.0"]),
        "is not in policy allowedLicenses", rebind=True))
    results.append(expect_rejected(
        "compose artifact bytes tampered",
        lambda g: g.update(artifactFileDigest="sha256:" + "0" * 64),
        "compose artifact bytes do not match"))
    results.append(expect_rejected(
        "compose artifact location removed cannot skip byte validation",
        lambda g: g["artifact"].pop("localPath"),
        "artifact byte validation cannot be skipped", rebind=True))
    results.append(expect_rejected(
        "unrelated bytes rebound with consistent hashes still fail render comparison",
        lambda g: (g.update(artifactFileDigest=g["artifact"]["composeDigest"]), g["artifact"].pop("localPath")),
        "cannot be skipped", rebind=True))
    results.append(expect_rejected(
        "bytes that are not render(manifest) are rejected",
        lambda g: g.update(artifactRenderedDigest="sha256:" + "1" * 64),
        "do not match render(manifest)"))
    results.append(expect_rejected(
        "unsigned rollback procedure",
        lambda g: (g["procedures"][1].update(reviewState="candidate"), g["procedures"][1].pop("signature")),
        "must have a signed review state", rebind=True))
    results.append(expect_rejected(
        "missing policy signing configuration",
        lambda g: g["policy"].pop("signing"),
        "signing configuration", rebind=True))
    def public_port_denied(graph):
        graph["policy"]["network"].update(allowPublicExposure=False)
        graph["plan"]["steps"][5]["parameters"].update(exposure="private")
    results.append(expect_rejected(
        "public manifest port denied despite private proxy and approval",
        public_port_denied, "public exposure is denied by policy", rebind=True))
    results.append(expect_rejected(
        "checkpoint step removed from checkpoint-dependent procedure",
        lambda g: g["plan"].update(steps=[s for s in g["plan"]["steps"] if s["stepId"] != "step_checkpoint"]),
        "no volume.backup checkpoint step", rebind=True))
    results.append(expect_rejected(
        "declared renderer version does not match installed renderer",
        lambda g: g["artifact"]["renderer"].update(version="99.0.0"),
        "version dispatch is not supported", rebind=True))
    results.append(expect_rejected(
        "complete graph with 1 MB RAM fails compatibility despite valid bindings",
        lambda g: g["host"]["resources"].update(ramMb=1),
        "deployment graph is not compatible: insufficient-ram", rebind=True))
    results.append(expect_rejected(
        "complete graph with a Windows host fails compatibility despite valid bindings",
        lambda g: g["host"]["os"].update(family="windows"),
        "deployment graph is not compatible: unsupported-os", rebind=True))

    def secret_wrong_service(graph):
        digest = "sha256:" + "3" * 64
        graph["manifest"]["release"]["images"].append(
            {"service": "worker", "image": "ghcr.io/example/worker", "digest": digest})
        graph["manifest"]["release"]["environment"][0]["service"] = "app"
        graph["manifest"]["release"]["volumes"][0]["service"] = "app"
        graph["artifact"]["services"].append("worker")
        new_digest = validate.canonical_digest(graph["manifest"])
        graph["artifact"]["manifestDigest"] = new_digest
        graph["envelope"]["manifestDigest"] = new_digest
        graph["case"]["manifestDigest"] = new_digest
        graph["evidence"][0]["commitOrReleaseDigest"] = new_digest
        for step in graph["plan"]["steps"]:
            if step["capability"] == "manifest.verify":
                step["parameters"]["manifestDigest"] = new_digest
            if step["capability"] == "secret.inject":
                step["parameters"]["service"] = "worker"
    results.append(expect_rejected(
        "secret.inject targeted at a service that does not own the secret",
        secret_wrong_service,
        "does not match a manifest environment entry owned by that service", rebind=True))

    def unknown_owner(graph):
        graph["manifest"]["release"]["environment"][0]["service"] = "ghost"
        new_digest = validate.canonical_digest(graph["manifest"])
        graph["artifact"]["manifestDigest"] = new_digest
        graph["envelope"]["manifestDigest"] = new_digest
        graph["case"]["manifestDigest"] = new_digest
        graph["evidence"][0]["commitOrReleaseDigest"] = new_digest
        for step in graph["plan"]["steps"]:
            if step["capability"] == "manifest.verify":
                step["parameters"]["manifestDigest"] = new_digest
    results.append(expect_rejected(
        "environment entry with unknown service owner",
        unknown_owner, "unknown service owner", rebind=True))

    def secret_wrong_service_rollback(graph):
        secret_wrong_service(graph)
        for step in graph["plan"]["steps"]:
            if step["capability"] == "secret.inject":
                step["parameters"]["service"] = "app"
        rollback = graph["procedures"][1]
        rollback["capabilitySequence"].append("secret.inject")
        rollback["parameterContracts"].append({
            "capability": "secret.inject",
            "requiredParameters": ["secretRef", "targetEnv", "service"]})
        graph["plan"]["rollbackSteps"].append({
            "stepId": "step_secret_rollback", "capability": "secret.inject",
            "parameters": {"secretRef": "secret://deployment/app-secret",
                           "targetEnv": "APP_SECRET", "service": "worker"},
            "citations": ["proc_rollback_uptime_demo"], "requiresApproval": False,
            "verification": {"type": "exit-state", "target": "rollback secret injected"}})
    results.append(expect_rejected(
        "rollback secret.inject targeted at a service that does not own the secret",
        secret_wrong_service_rollback,
        "does not match a manifest environment entry owned by that service", rebind=True))

    results.append(expect_rejected(
        "proxy compensation pointing at compose.down instead of proxy.remove",
        lambda g: g["plan"]["steps"][5].update(rollbackStepId="step_down"),
        "compensation must be 'proxy.remove'", rebind=True))

    def untrusted_signer(graph):
        graph["manifest"]["signature"]["signedBy"] = "untrusted-signer"
        unsigned = {k: v for k, v in graph["manifest"].items() if k != "signature"}
        graph["manifest"]["signature"]["payloadDigest"] = validate.canonical_digest(unsigned)
        digest = validate.canonical_digest(graph["manifest"])
        graph["artifact"]["manifestDigest"] = digest
        graph["envelope"]["manifestDigest"] = digest
        graph["case"]["manifestDigest"] = digest
        graph["evidence"][0]["commitOrReleaseDigest"] = digest
        for step in graph["plan"]["steps"]:
            if step["capability"] == "manifest.verify":
                step["parameters"]["manifestDigest"] = digest
    results.append(expect_rejected(
        "untrusted signer rejected even with rebuilt payload digest",
        untrusted_signer, "is not in policy trustedSigners", rebind=True))

    graph = load_graph()
    for version in ("27.3.1-rc1", "garbage", "v27.3.1"):
        graph["host"]["docker"]["engineVersion"] = version
        got = validate.decide(graph["manifest"], graph["host"], graph["policy"], "active", EVAL_TIME)
        results.append(got == ("reject", "unsupported-docker-version"))
        print(("PASS" if results[-1] else "FAIL") + " unsupported Docker version " + version)

    secret_errors = []
    validate.check_secrets("Bearer SYNTHETIC_REVIEW_VALUE", "test", secret_errors)
    results.append(bool(secret_errors) and all("SYNTHETIC_REVIEW_VALUE" not in e for e in secret_errors))
    print(("PASS" if results[-1] else "FAIL") + " detected secret value is suppressed in diagnostics")

    # Contribution budget / managed-volume semantics (NODE-002):
    # schema-valid records whose arithmetic or qualification
    # claims are false must still be rejected.
    budget = validate.load(ROOT / "context" / "examples" / "contribution-budget.json")
    results.append(validate.contribution_budget_errors(copy.deepcopy(budget)) == [])
    print(("PASS" if results[-1] else "FAIL") + " valid contribution budget accepted semantically")

    def expect_budget_rejected(label, mutate):
        record = copy.deepcopy(budget)
        mutate(record)
        errs = validate.contribution_budget_errors(record)
        results.append(bool(errs))
        print(("PASS" if results[-1] else "FAIL") + f" {label}" + (f" -> {errs[0]}" if errs else " (unexpectedly accepted)"))

    expect_budget_rejected(
        "allocations plus reserve exceeding owner budget",
        lambda r: r["allocations"][0]["resources"].update(ramMb=4096))
    expect_budget_rejected(
        "owner budget exceeding physical totals",
        lambda r: r["ownerBudget"].update(storageMb=99999999))
    expect_budget_rejected(
        "duplicate allocation role",
        lambda r: r["allocations"].append(copy.deepcopy(r["allocations"][0])))
    expect_budget_rejected(
        "inverted budget validity window",
        lambda r: r.update(validUntil=r["validFrom"]))
    expect_budget_rejected(
        "budget validity window inverted across UTC offsets",
        lambda r: r.update(
            validFrom="2026-10-30T08:00:00Z",
            validUntil="2026-10-30T10:00:00+03:00"))
    offset_budget = copy.deepcopy(budget)
    offset_budget.update(
        validFrom="2026-10-30T06:00:00Z",
        validUntil="2026-10-30T10:00:00+03:00")
    results.append(validate.contribution_budget_errors(offset_budget) == [])
    print(("PASS" if results[-1] else "FAIL") + " increasing budget validity window across UTC offsets accepted")
    expect_budget_rejected(
        "budget validity window inverted below microsecond precision",
        lambda r: r.update(
            validFrom="2026-10-30T08:00:00.0000002Z",
            validUntil="2026-10-30T08:00:00.0000001Z"))
    precise_budget = copy.deepcopy(budget)
    precise_budget.update(
        validFrom="2026-10-30T08:00:00.0000001Z",
        validUntil="2026-10-30T08:00:00.0000002Z")
    results.append(validate.contribution_budget_errors(precise_budget) == [])
    print(("PASS" if results[-1] else "FAIL") + " increasing sub-microsecond budget validity window accepted")

    volqual = validate.load(ROOT / "context" / "examples" / "managed-volume-qualification.json")
    results.append(validate.managed_volume_errors(copy.deepcopy(volqual)) == [])
    print(("PASS" if results[-1] else "FAIL") + " valid preflight-only managed-volume record accepted semantically")
    qualified_without_quota = copy.deepcopy(volqual)
    qualified_without_quota["result"] = "qualified"
    vol_errs = validate.managed_volume_errors(qualified_without_quota)
    results.append(any("qualified without enforceable storage quota" in err for err in vol_errs))
    print(("PASS" if results[-1] else "FAIL") + " qualified result rejected without enforceable storage quota")
    offset_volqual = copy.deepcopy(volqual)
    offset_volqual["observedAt"] = "2026-10-30T08:00:00Z"
    offset_volqual["expiresAt"] = "2026-10-30T10:00:00+03:00"
    vol_errs = validate.managed_volume_errors(offset_volqual)
    results.append(bool(vol_errs))
    print(("PASS" if results[-1] else "FAIL") + " inverted managed-volume window across UTC offsets rejected" + (f" -> {vol_errs[0]}" if vol_errs else ""))
    offset_volqual["observedAt"] = "2026-10-30T06:00:00Z"
    offset_volqual["expiresAt"] = "2026-10-30T10:00:00+03:00"
    vol_errs = validate.managed_volume_errors(offset_volqual)
    results.append(vol_errs == [])
    print(("PASS" if results[-1] else "FAIL") + " increasing managed-volume window across UTC offsets accepted")
    offset_volqual["observedAt"] = "2026-10-30T08:00:00.0000002Z"
    offset_volqual["expiresAt"] = "2026-10-30T08:00:00.0000001Z"
    vol_errs = validate.managed_volume_errors(offset_volqual)
    results.append(bool(vol_errs))
    print(("PASS" if results[-1] else "FAIL") + " inverted sub-microsecond managed-volume window rejected" + (f" -> {vol_errs[0]}" if vol_errs else ""))
    offset_volqual["observedAt"] = "2026-10-30T08:00:00.0000001Z"
    offset_volqual["expiresAt"] = "2026-10-30T08:00:00.0000002Z"
    vol_errs = validate.managed_volume_errors(offset_volqual)
    results.append(vol_errs == [])
    print(("PASS" if results[-1] else "FAIL") + " increasing sub-microsecond managed-volume window accepted")
    expired_qual = copy.deepcopy(volqual)
    expired_qual["expiresAt"] = expired_qual["observedAt"]
    vol_errs = validate.managed_volume_errors(expired_qual)
    results.append(bool(vol_errs))
    print(("PASS" if results[-1] else "FAIL") + " non-increasing qualification window rejected" + (f" -> {vol_errs[0]}" if vol_errs else ""))

    print("ALL PASS" if all(results) else "SOME FAILED")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
