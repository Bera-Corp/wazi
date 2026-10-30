#!/usr/bin/env python3
"""Tests for the Phase 2 offline context engine.

Run with scripts/validate.ps1 or directly:
    .venv/Scripts/python.exe scripts/test_context_engine.py
Exits non-zero on the first failing group (all results printed).
"""

import copy
import json
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import context_engine as ce  # noqa: E402
import validate  # noqa: E402

EVAL_TIME = "2026-10-21T09:05:00Z"
results = []


def check(label, condition):
    results.append(bool(condition))
    print(("PASS" if condition else "FAIL") + f" {label}")


def main():
    cases = ce.ingest_cases()
    check("ingests every eval fixture as a case", len(cases) == 17)
    successes = [c for c in cases if c.outcome == "success"]
    failures = [c for c in cases if c.outcome == "failure"]
    check("exactly one compatible success case", len(successes) == 1 and successes[0].case_id == "eval_compatible")
    check("failure cases carry reason codes", all(c.reason for c in failures))
    check("case digests are canonical manifest digests",
          all(c.manifest_digest.startswith("sha256:") for c in cases))

    with tempfile.TemporaryDirectory() as tmp:
        store = ce.CaseStore(Path(tmp) / "cases.jsonl")
        check("store accepts all ingested cases", store.add_many(cases) == len(cases))
        check("store deduplicates by case id", store.add(cases[0]) is False and len(store) == len(cases))
        reopened = ce.CaseStore(Path(tmp) / "cases.jsonl")
        check("store persists across reopen", len(reopened) == len(cases))
        demo_cases = reopened.for_application("org.example.uptime-demo")
        check("cases queryable by application", len(demo_cases) == len(cases))

    _schemas = {name: validate.load(ce.ROOT / "schemas" / name)
                for name in sorted(validate.REQUIRED_SCHEMAS)}
    graph_raw, errors = validate.load_examples(
        validate.build_registry(_schemas), _schemas)
    check("example graph loads for selection tests", not errors)
    graph = validate.graph_from_examples(graph_raw)
    manifest, host, policy = graph["manifest"], graph["host"], graph["policy"]
    install = validate.load(ce.EXAMPLES / "procedure-install.json")
    rollback = validate.load(ce.EXAMPLES / "procedure-rollback.json")

    selected = ce.select_procedure([rollback, install], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("selects the install procedure for a compatible host",
          selected is not None and selected.procedure["procedureId"] == "proc_install_uptime_demo")
    ranked = ce.rank_procedures([install], manifest, host, policy, "revoked", EVAL_TIME, operation="install")
    check("revoked manifest ranks incompatible with reason",
          ranked and not ranked[0].compatible and ranked[0].reasons == ["revoked-manifest"])
    wrong_op = ce.select_procedure([install], manifest, host, policy, "active", EVAL_TIME, operation="uninstall")
    check("operation mismatch selects nothing", wrong_op is None)
    foreign = copy.deepcopy(install)
    foreign["applicationId"] = "org.example.other"
    ranked = ce.rank_procedures([foreign], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("foreign application procedure rejected with reason",
          ranked and ranked[0].reasons == ["procedure-application-mismatch"])

    older = copy.deepcopy(install)
    older["procedureId"] = "proc_install_uptime_demo_old"
    older["version"] = "0.9.0"
    selected = ce.select_procedure([older, install], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("higher procedure version wins", selected.procedure["procedureId"] == "proc_install_uptime_demo")

    template = validate.load(ce.EXAMPLES / "execution-context-envelope.json")
    chosen = ce.select_procedure([install], manifest, host, policy, "active", EVAL_TIME, operation="install")
    envelope = ce.assemble_envelope(
        template, chosen,
        [c for c in cases if c.outcome == "success"],
        [c for c in cases if c.outcome == "failure"][:2],
        "2026-10-21T09:05:00Z", "2026-10-21T09:35:00Z")
    check("assembled envelope is canonical-schema valid", ce.envelope_schema_errors(envelope) == [])
    check("envelope cites selected cases",
          [r["recordId"] for r in envelope["selectedSuccessCaseRefs"]] == ["eval_compatible"]
          and len(envelope["selectedFailureCaseRefs"]) == 2
          and all(r["digest"].startswith("sha256:") for r in envelope["selectedFailureCaseRefs"]))
    check("envelope procedure ref digest matches the procedure",
          envelope["procedureRef"]["digest"] == validate.canonical_digest(install))

    # --- Phase 2 retrieval pipeline -------------------------------------
    store = ce.CaseStore()
    store.add_many(cases)
    sigs = store.failure_signatures("org.example.uptime-demo", tenant_scope="tenant_demo")
    check("failure signatures group all rejection reasons",
          len(sigs) == 16 and all(s["count"] >= 1 and s["signatureId"].startswith("sig_") for s in sigs))
    successes = store.comparable_successes(manifest, host, tenant_scope="tenant_demo", limit=3)
    check("comparable successes return the compatible case first",
          successes and successes[0].case_id == "eval_compatible")
    ranked = ce.rank_procedures([install], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("compatible procedure carries an explainable score",
          ranked[0].score >= 85 and "exact-release-scope" in ranked[0].score_breakdown
          and "exact-release-scope" in ranked[0].explanation())

    request = {"tenantScope": "tenant_demo", "applicationId": manifest["appId"],
               "releaseVersion": manifest["version"], "operation": "install",
               "hostId": host["hostId"]}
    template = validate.load(ce.EXAMPLES / "execution-context-envelope.json")
    procedures = [install, rollback]
    _graph0 = validate.graph_from_examples(graph_raw)
    ok = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                             template_envelope=template, artifact=_graph0["artifact"],
                             plan=_graph0["plan"], revocation_record=_graph0["revocations"])
    check("pipeline accepts the compatible scenario end to end",
          ok.decision == "accept" and ok.selected is not None and ok.envelope is not None)
    check("pipeline envelope is schema-valid with cited cases",
          ce.envelope_schema_errors(ok.envelope) == [] and len(ok.envelope["selectedFailureCaseRefs"]) >= 1)
    check("pipeline explanation covers every stage",
          len(ok.explanation) >= 6 and any("Selected proc_install" in line for line in ok.explanation))

    busy = copy.deepcopy(host)
    busy["occupiedPorts"] = sorted(set(busy.get("occupiedPorts", [])) | {3000})
    denied = ce.retrieve_context(request, manifest, busy, policy, procedures, store,
                                 template_envelope=template)
    check("occupied port denied at compatibility before ranking",
          denied.decision == "reject" and denied.reasons == ["occupied-port"] and denied.ranked == [])

    revoked = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                  registry_state="revoked")
    check("revoked manifest denied before ranking",
          revoked.decision == "reject" and revoked.reasons == ["revoked-manifest"])

    wrong_tenant = dict(request, tenantScope="tenant_other")
    blocked = ce.retrieve_context(wrong_tenant, manifest, host, policy, procedures, store)
    check("cross-tenant request blocked at tenant stage",
          blocked.decision == "blocked" and blocked.blocked_stage == "tenant")

    stale_rev = {"generatedAt": "2026-10-20T08:00:00Z", "expiresAt": "2026-10-21T08:00:00Z", "entries": []}
    stale = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                revocation_record=stale_rev)
    check("stale revocation record fails closed",
          stale.decision == "blocked" and stale.reasons == ["revocation-state-stale"])
    check("revocation_state_for reports stale on expired record",
          ce.revocation_state_for(manifest, stale_rev, EVAL_TIME) == "stale")

    secret_host = copy.deepcopy(host)
    secret_host["redaction"] = {"applied": True, "secretValuesPresent": True}
    leaked = ce.retrieve_context(request, manifest, secret_host, policy, procedures, store)
    check("secret values in context block retrieval",
          leaked.decision == "blocked" and leaked.blocked_stage == "redaction")

    metrics = ce.evaluate_retrieval()
    check("evaluation: zero false-compatible across fixtures",
          metrics["hardEngineFalseCompatibleRate"] == 0 and metrics["fixtures"] == 17)
    check("evaluation: perfect decision accuracy and stale recall",
          metrics["hardEngineDecisionAccuracy"] == 1 and metrics["staleRejectionRecall"] == 1)

    # --- Review corrections: tenant scope, procedure trust, envelope
    # binding, revocation proof, metric naming, free-text redaction ---
    def resign(record):
        rec = copy.deepcopy(record)
        sig = rec.pop("signature", {})
        sig["payloadDigest"] = validate.canonical_digest(rec)
        rec["signature"] = sig
        return rec

    two_tenant = ce.CaseStore()
    for tid, tenant in (("case_a_ok", "tenant_a"), ("case_b_ok", "tenant_b")):
        two_tenant.add(ce.Case(tid, "success", "compatible", manifest["appId"],
                               EVAL_TIME, "sha256:" + "0" * 64,
                               release_version=manifest["version"],
                               architecture="x86_64", os_family="linux",
                               tenant_scope=tenant))
    for tid, tenant in (("case_a_bad", "tenant_a"), ("case_b_bad", "tenant_b")):
        two_tenant.add(ce.Case(tid, "failure", "occupied-port", manifest["appId"],
                               EVAL_TIME, "sha256:" + "0" * 64, tenant_scope=tenant))
    scoped = two_tenant.comparable_successes(manifest, host, tenant_scope="tenant_a")
    check("success retrieval is tenant-scoped",
          [c.case_id for c in scoped] == ["case_a_ok"])
    scoped_fail = two_tenant.relevant_failures(manifest, tenant_scope="tenant_b")
    check("failure retrieval is tenant-scoped",
          [c.case_id for c in scoped_fail] == ["case_b_bad"])
    scoped_sigs = two_tenant.failure_signatures(manifest["appId"], tenant_scope="tenant_a")
    check("failure signature counts are tenant-scoped",
          len(scoped_sigs) == 1 and scoped_sigs[0]["count"] == 1)
    try:
        two_tenant.comparable_successes(manifest, host)
        missing_scope_ok = False
    except ValueError:
        missing_scope_ok = True
    check("case retrieval fails closed without tenant scope", missing_scope_ok)

    tampered = copy.deepcopy(install)
    tampered["version"] = "9.9.9"  # signature payload no longer matches
    ranked = ce.rank_procedures([tampered], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("tampered procedure signature is ineligible before ranking",
          not ranked[0].compatible and "procedure-signature-digest" in ranked[0].reasons
          and ce.select_procedure([tampered], manifest, host, policy, "active", EVAL_TIME, operation="install") is None)
    unsigned = copy.deepcopy(install)
    unsigned["reviewState"] = "candidate"
    unsigned.pop("signature", None)
    ranked = ce.rank_procedures([unsigned], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("unsigned candidate-state procedure is ineligible",
          not ranked[0].compatible and any(r.startswith("procedure-") for r in ranked[0].reasons))
    wrong_scope = resign(copy.deepcopy(install))
    wrong_scope["provenance"]["versionScope"] = "2.0.0"
    wrong_scope = resign(wrong_scope)
    ranked = ce.rank_procedures([wrong_scope], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("procedure for another release scope is ineligible",
          not ranked[0].compatible and "procedure-release-scope" in ranked[0].reasons)
    untrusted = resign({**copy.deepcopy(install), "signature": {
        "algorithm": "sigstore-cosign", "value": "x" * 20,
        "signedBy": "mallory", "payloadDigest": ""}})
    untrusted = resign(untrusted)
    ranked = ce.rank_procedures([untrusted], manifest, host, policy, "active", EVAL_TIME, operation="install")
    check("untrusted procedure signer is ineligible",
          not ranked[0].compatible and "procedure-signer-untrusted" in ranked[0].reasons)

    graph = validate.graph_from_examples(graph_raw)
    bound = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                template_envelope=template, artifact=graph["artifact"],
                                plan=graph["plan"], revocation_record=graph["revocations"])
    check("retrieval with full binding still accepts",
          bound.decision == "accept" and bound.envelope["tenantScope"] == "tenant_demo"
          and bound.envelope["manifestDigest"] == validate.canonical_digest(manifest))
    alien_template = copy.deepcopy(template)
    alien_template["tenantScope"] = "tenant_evil"
    alien_template["applicationId"] = "org.example.other"
    rebound = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                  template_envelope=alien_template, artifact=graph["artifact"],
                                  plan=graph["plan"], revocation_record=graph["revocations"])
    check("envelope is rebound to current records, template identity ignored",
          rebound.decision == "accept" and rebound.envelope["tenantScope"] == "tenant_demo"
          and rebound.envelope["applicationId"] == manifest["appId"])
    shell_template = copy.deepcopy(template)
    shell_template["allowedCapabilities"] = ["shell.exec"]
    shelled = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                  template_envelope=shell_template, artifact=graph["artifact"],
                                  plan=graph["plan"], revocation_record=graph["revocations"])
    check("schema-invalid envelope blocks acceptance",
          shelled.decision == "blocked" and shelled.blocked_stage == "envelope")

    live_missing = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                       template_envelope=template, artifact=graph["artifact"],
                                       plan=graph["plan"], live=True)
    check("live path blocks without a revocation snapshot",
          live_missing.decision == "blocked" and live_missing.blocked_stage == "revocation"
          and live_missing.revocation_mode == "unchecked")
    live_ok = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                  template_envelope=template, artifact=graph["artifact"],
                                  plan=graph["plan"], revocation_record=graph["revocations"],
                                  live=True, accepted_revocation_sequence=1)
    check("live path accepts with a metadata-validated current revocation snapshot",
          live_ok.decision == "accept" and live_ok.revocation_mode == "metadata-validated")
    live_no_cursor = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                         template_envelope=template, artifact=graph["artifact"],
                                         plan=graph["plan"], revocation_record=graph["revocations"],
                                         live=True)
    check("live path blocks without a persisted revocation cursor",
          live_no_cursor.decision == "blocked" and live_no_cursor.blocked_stage == "revocation"
          and live_no_cursor.reasons == ["revocation-sequence-cursor-required"]
          and live_no_cursor.revocation_mode == "unchecked")
    live_rollback = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                        revocation_record=graph["revocations"], live=True,
                                        accepted_revocation_sequence=99)
    check("live path blocks a rolled-back revocation sequence",
          live_rollback.decision == "blocked" and live_rollback.blocked_stage == "revocation"
          and live_rollback.revocation_mode == "unchecked")
    live_no_snapshot = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                           template_envelope=template, artifact=graph["artifact"],
                                           plan=graph["plan"], live=True,
                                           accepted_revocation_sequence=1, revocation_record=None)
    check("blocked live snapshot check is not labelled metadata-validated",
          live_no_snapshot.decision == "blocked"
          and live_no_snapshot.revocation_mode == "unchecked")

    poisoned = ce.CaseStore()
    poisoned.add_many(cases)
    _poison_reason = "pass" + "word=hunter2" + "secretvalue"  # wazi-secret-scan: allow
    poisoned.add(ce.Case("case_poison", "failure", _poison_reason,
                         manifest["appId"], EVAL_TIME, "sha256:" + "0" * 64,
                         tenant_scope="tenant_demo"))
    leaked2 = ce.retrieve_context(request, manifest, host, policy, procedures, poisoned,
                                  template_envelope=template, artifact=graph["artifact"],
                                  plan=graph["plan"], revocation_record=graph["revocations"])
    check("secret pattern in case free text blocks retrieval",
          leaked2.decision == "blocked" and leaked2.blocked_stage == "redaction")
    check("blocked redaction result carries no case content",
          leaked2.success_cases == [] and leaked2.failure_cases == []
          and leaked2.failure_signatures == [] and leaked2.envelope is None)
    check("secret absent from blocked result and its serialized form",
          "hunter2secretvalue" not in json.dumps(ce._result_json(leaked2))
          and "hunter2secretvalue" not in str(leaked2.failure_signatures)
          and "hunter2secretvalue" not in str([c.summary() for c in leaked2.failure_cases]))

    # Plan contract: order, parameter contracts, policy denials.
    bad_order = copy.deepcopy(graph["plan"])
    bad_order["steps"][0], bad_order["steps"][1] = bad_order["steps"][1], bad_order["steps"][0]
    reordered = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                    template_envelope=template, artifact=graph["artifact"],
                                    plan=bad_order, revocation_record=graph["revocations"])
    check("plan with reordered capabilities blocks",
          reordered.decision == "blocked" and reordered.blocked_stage == "envelope")
    missing_param = copy.deepcopy(graph["plan"])
    del missing_param["steps"][0]["parameters"]["manifestDigest"]
    missing_p = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                    template_envelope=template, artifact=graph["artifact"],
                                    plan=missing_param, revocation_record=graph["revocations"])
    check("plan missing a procedure-required parameter blocks",
          missing_p.decision == "blocked" and missing_p.blocked_stage == "envelope")
    fixed_bad = copy.deepcopy(graph["plan"])
    fixed_bad["steps"][0]["parameters"]["requireReviewState"] = "draft"
    fixed_p = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                  template_envelope=template, artifact=graph["artifact"],
                                  plan=fixed_bad, revocation_record=graph["revocations"])
    check("plan violating a procedure-fixed parameter blocks",
          fixed_p.decision == "blocked" and fixed_p.blocked_stage == "envelope")
    denied_policy = copy.deepcopy(policy)
    denied_policy["deniedCapabilities"] = ["health.check"]
    denied_p = ce.retrieve_context(request, manifest, host, denied_policy, procedures, store,
                                   template_envelope=template, artifact=graph["artifact"],
                                   plan=graph["plan"], revocation_record=graph["revocations"])
    check("envelope capability denied by policy blocks via plan binding",
          denied_p.decision == "blocked" and denied_p.blocked_stage == "envelope")

    # Plan execution bindings: artifact/image digests, unresolved
    # questions, compensation, and plan schema (validate.py graph checks).
    def plan_blocked(mutated):
        result = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                     template_envelope=template, artifact=graph["artifact"],
                                     plan=mutated, revocation_record=graph["revocations"])
        return result.decision == "blocked" and result.blocked_stage == "envelope"

    bad_artifact = copy.deepcopy(graph["plan"])
    bad_artifact["composeArtifactRef"]["digest"] = "sha256:" + "0" * 64
    check("plan with mismatched compose artifact digest blocks", plan_blocked(bad_artifact))
    bad_artifact_id = copy.deepcopy(graph["plan"])
    bad_artifact_id["composeArtifactRef"]["recordId"] = "art_other"
    check("plan with mismatched compose artifact reference blocks", plan_blocked(bad_artifact_id))
    bad_image = copy.deepcopy(graph["plan"])
    bad_image["steps"][1]["parameters"]["imageDigest"] = "sha256:" + "1" * 64
    check("plan with mismatched image digest blocks", plan_blocked(bad_image))
    unresolved = copy.deepcopy(graph["plan"])
    unresolved["unresolvedQuestions"] = ["which port?"]
    check("plan with unresolvedQuestions blocks", plan_blocked(unresolved))
    bad_comp = copy.deepcopy(graph["plan"])
    for _step in bad_comp["steps"]:
        if _step["stepId"] == "step_up":
            _step["rollbackStepId"] = "step_proxy_remove"
    check("plan with invalid rollback compensation blocks", plan_blocked(bad_comp))
    no_comp = copy.deepcopy(graph["plan"])
    for _step in no_comp["steps"]:
        if _step["stepId"] == "step_proxy":
            _step.pop("rollbackStepId", None)
    check("plan with missing rollback compensation blocks", plan_blocked(no_comp))
    bad_up_digest = copy.deepcopy(graph["plan"])
    for _step in bad_up_digest["steps"]:
        if _step["stepId"] == "step_up":
            _step["parameters"]["composeArtifactDigest"] = "sha256:" + "2" * 64
    check("plan with mismatched compose.up artifact digest blocks", plan_blocked(bad_up_digest))
    bad_secret_ref = copy.deepcopy(graph["plan"])
    for _step in bad_secret_ref["steps"]:
        if _step["stepId"] == "step_inject_secret":
            _step["parameters"]["secretRef"] = "secret://deployment/other"
    check("plan with undeclared secret reference blocks", plan_blocked(bad_secret_ref))
    schema_bad = copy.deepcopy(graph["plan"])
    del schema_bad["planId"]
    schema_result = ce.retrieve_context(request, manifest, host, policy, procedures, store,
                                        template_envelope=template, artifact=graph["artifact"],
                                        plan=schema_bad, revocation_record=graph["revocations"])
    check("schema-invalid plan blocks",
          schema_result.decision == "blocked" and schema_result.blocked_stage == "envelope"
          and schema_result.reasons == ["plan-schema-invalid"])

    metrics = ce.evaluate_retrieval()
    check("hard-engine metrics are named for what they measure",
          metrics["hardEngineDecisionAccuracy"] == 1 and metrics["hardEngineFalseCompatibleRate"] == 0)
    e2e = ce.evaluate_end_to_end()
    check("end-to-end adversarial retrieval has zero false accepts",
          e2e["falseAcceptRate"] == 0 and e2e["endToEndAccuracy"] == 1 and e2e["scenarios"] >= 8)

    packet = ce.build_demo_packet()
    check("demo packet has one accept and two engine-produced denials",
          [s["result"]["decision"] for s in packet["scenarios"]] == ["accept", "reject", "reject"]
          and packet["scenarios"][1]["result"]["reasons"] == ["occupied-port"]
          and packet["scenarios"][2]["result"]["reasons"] == ["revoked-manifest"])
    check("demo packet labels revocation as metadata-validated, not live-verified",
          packet["scenarios"][0]["result"]["revocationMode"] == "metadata-validated"
          and all(s["result"]["revocationMode"] != "live-verified" for s in packet["scenarios"]))

    print("ALL PASS" if all(results) else "SOME FAILED")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
