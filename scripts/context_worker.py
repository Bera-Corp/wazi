#!/usr/bin/env python3
"""Private, bounded JSON worker invoked by the Go agent, never an HTTP server.

Go performs cryptographic/resource verification before calling retrieve. No
JSON flag can make this worker an execution authorization or a trust verifier.
"""
import argparse
import json
import sys
from pathlib import Path

# Python isolated mode excludes the script directory. Only this protected,
# operator-selected directory supplies worker dependencies.
sys.path.insert(0, str(Path(__file__).resolve().parent))

import context_engine as engine
import validate
from context_store import ContextStore


def retrieve(store, payload):
    snapshot = payload["snapshot"]
    envelope = snapshot["envelope"]
    manifest, procedure = snapshot["manifest"], snapshot["procedure"]
    if engine.secret_findings(snapshot):
        raise ValueError("context-secret-pattern")
    tenant, application, operation = (envelope[k] for k in
                                     ("tenantScope", "applicationId", "operation"))
    cases = store.cases(tenant, application, operation)
    # Success history is useful only for the exact signed release/procedure.
    comparable = store.cases(tenant, application, operation, procedure_ref=envelope["procedureRef"])
    successes = [c for c in comparable if c["outcome"] == "success"
                 and c["manifestDigest"] == envelope["manifestDigest"]]
    failures = [c for c in cases if c["outcome"] == "failure"]
    score, breakdown = engine._score_procedure(procedure, manifest)
    history = min(10, len(successes) * 2)
    if history:
        breakdown["verified-release-successes"] = history
    result = {
        "mode": "live-agent-verified", "decision": "accept",
        "executionApproved": False, "evaluatedAt": payload["evaluatedAt"],
        "expiresAt": envelope["expiresAt"], "tenantScope": tenant,
        "hostId": envelope["hostProfileRef"]["recordId"],
        "applicationId": application, "operation": operation,
        "inputDigests": {key: validate.canonical_digest(value)
                         for key, value in snapshot.items()},
        "selected": {"procedureId": procedure["procedureId"],
                     "version": procedure["version"], "score": score + history,
                     "scoreBreakdown": breakdown},
        "successCaseRefs": [{"recordId": c["caseId"], "digest": validate.canonical_digest(c)}
                            for c in successes[:3]],
        "failureCaseRefs": [{"recordId": c["caseId"], "digest": validate.canonical_digest(c)}
                            for c in failures[:5]],
        "explanation": ["Paired Go agent verified the signed inputs and resource-bound plan.",
                        "Only explicitly requested procedures are considered; no catalog-wide search is implied.",
                        "Case citations are inspection context, not changes to the approved envelope.",
                        "Execution requires fresh preflight and case-bound approvals."],
        "snapshot": snapshot,
    }
    return store.save_context(tenant, result)


def record_terminal(store, payload):
    """Normalize a trusted agent journal receipt; never import raw logs."""
    snapshot, job = payload["receipt"], payload["job"]
    operation = snapshot.get("Operation") or {}
    if not operation.get("identity"):
        return {"imported": False, "classification": "preflight-denial"}
    identity = operation["identity"]
    state = operation["state"]
    if state not in ("completed", "failed", "blocked", "cancelled"):
        return {"imported": False, "classification": "unresolved-operation"}
    plan, envelope = job["plan"], job["envelope"]
    bindings = {"jobId": job["jobId"], "caseId": job["caseId"],
                "planId": plan["planId"], "planDigest": validate.canonical_digest(plan),
                "envelopeId": envelope["envelopeId"],
                "envelopeDigest": validate.canonical_digest(envelope),
                "manifestDigest": envelope["manifestDigest"],
                "tenantScope": envelope["tenantScope"], "applicationId": envelope["applicationId"],
                "hostId": envelope["hostProfileRef"]["recordId"], "operation": envelope["operation"]}
    if any(identity.get(k) != v for k, v in bindings.items()):
        raise ValueError("receipt-binding-mismatch")
    if identity["operation"] not in ("install", "upgrade", "backup", "restore", "rollback", "uninstall"):
        return {"imported": False, "classification": "non-deployment-operation"}
    if snapshot.get("RecoveryRequired") or snapshot.get("PendingIntent"):
        return {"imported": False, "classification": "unresolved-operation"}
    events = operation.get("events", [])
    if not events or (state == "completed" and events[-1]["kind"] != "completed"):
        raise ValueError("receipt-terminal-evidence-required")
    outcome = {"completed": "success", "failed": "failure"}.get(state, "aborted")
    receipt_digest = validate.canonical_digest(snapshot)
    evidence_id = "ev_" + receipt_digest.split(":")[1]
    case = {"schemaVersion": "1.0.0", "caseId": identity["caseId"],
            "tenantScope": identity["tenantScope"], "applicationId": identity["applicationId"],
            "releaseVersion": envelope["releaseVersion"], "hostId": identity["hostId"],
            "manifestDigest": identity["manifestDigest"], "envelopeId": identity["envelopeId"],
            "planId": identity["planId"], "operation": identity["operation"], "outcome": outcome,
            "startedAt": operation["createdAt"], "finishedAt": operation["updatedAt"],
            "evidenceRefs": [evidence_id],
            "redaction": {"applied": True, "secretValuesPresent": False, "rawLogsIncluded": False}}
    evidence = {"schemaVersion": "1.0.0", "evidenceId": evidence_id,
                "deploymentCaseId": identity["caseId"], "type": "deployment-test",
                "result": {"success": "pass", "failure": "fail", "aborted": "inconclusive"}[outcome],
                "artifact": {"digest": receipt_digest, "mediaType": "application/json"},
                "tool": {"name": "wazi-pilot-agent-journal", "version": "1.0.0"},
                "commitOrReleaseDigest": identity["manifestDigest"],
                "createdAt": operation["updatedAt"],
                "redaction": {"applied": True, "secretValuesPresent": False}}
    return {"imported": store.import_case(case, [evidence], procedure_ref=envelope["procedureRef"]), "classification": outcome}


def dispatch(store, payload):
    action = payload["action"]
    if action == "retrieve":
        return retrieve(store, payload)
    if action == "record":
        return record_terminal(store, payload)
    if action == "search":
        accepted = payload["accepted"]
        if not accepted:
            return {"decision": "reject", "executionApproved": False,
                    "ranked": payload["rejected"], "reasons": ["no-verified-candidate"]}
        accepted.sort(key=lambda item: (-item["selected"]["score"],
                                       item["selected"]["procedureId"], item["contextId"]))
        winner = dict(accepted[0])
        winner.pop("contextId", None)
        winner["ranked"] = [{"contextId": item["contextId"], "decision": "accept",
                             "selected": item["selected"], "inputDigests": item["inputDigests"]}
                            for item in accepted] + payload["rejected"]
        return store.save_context(winner["tenantScope"], winner)
    if action == "inspect":
        return store.context(payload["tenantScope"], payload["contextId"])
    if action == "sync":
        # The queue directory is operator configuration from the private Go
        # bridge, never an HTTP parameter. Replays after restart are idempotent.
        queue = Path(payload["queueDirectory"])
        counts = {"imported": 0, "skipped": 0, "rejected": 0}
        files = sorted((queue / "results").glob("job_*.json"))
        if len(files) > 1000:
            raise ValueError("receipt-capacity")
        for path in files:
            request = queue / "requests" / path.name
            if any(p.is_symlink() or not p.is_file() or p.stat().st_size > (4 << 20)
                   for p in (path, request)):
                raise ValueError("receipt-file-invalid")
            result = json.loads(path.read_text(encoding="utf-8"))
            job = json.loads(request.read_text(encoding="utf-8"))
            try:
                record = record_terminal(store, {"receipt": result.get("operation") or {}, "job": job})
            except (ValueError, KeyError, TypeError):
                # Quarantine this record: invalid/conflicting evidence never
                # enters ranking. Database I/O failures still fail the call.
                counts["rejected"] += 1
                continue
            counts["imported" if record["imported"] else "skipped"] += 1
        return counts
    raise ValueError("unknown-worker-action")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", required=True)
    args = parser.parse_args()
    raw = sys.stdin.buffer.read((1 << 20) + 1)
    if len(raw) > 1 << 20:
        raise ValueError("worker-input-too-large")
    store = ContextStore(args.database)
    try:
        print(json.dumps(dispatch(store, json.loads(raw)), sort_keys=True))
    finally:
        store.close()


if __name__ == "__main__":
    try:
        main()
    except Exception:
        # Never send record contents, filesystem paths or subprocess stderr to callers.
        print('{"error":"context-worker-rejected"}')
        sys.exit(1)
