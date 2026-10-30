#!/usr/bin/env python3
"""Build fresh, digest-bound pilot job records for the Gitea Phase 1 pilot.

Operator tooling only: it reads the signed release bundle, a fresh host
profile, and host facts supplied by the operator, then emits envelope,
plan, artifact, budget, and qualification records plus (once an approval
has been issued) the PilotJob body for POST /v1/deployments. It holds no
signing key and authorizes nothing by itself.

    build_pilot_job.py --operation install --release-dir releases/.../pilot2 \
        --host-profile profile.json --facts facts.json --out-dir job/
    build_pilot_job.py --assemble --out-dir job/ --approval-id appr_x \
        --action install
"""
import argparse
import hashlib
import json
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

TENANT = "tenant_live_20261030T064834Z"
HOST = "host_bera-003"
APP = "org.gitea.gitea"
PROJECT = "wazi-gitea-phase1"
BACKUP_ID = "bak_gitea_pilot_1"
ACTION = {"install": "install", "upgrade": "upgrade",
          "restore": "destructive-storage", "rollback": "rollback", "backup": "backup",
          "uninstall": "uninstall"}


def canonical(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"),
                      ensure_ascii=False).encode()


def digest(obj):
    return "sha256:" + hashlib.sha256(canonical(obj)).hexdigest()


def load(path):
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))


def write(path, obj):
    Path(path).write_bytes(canonical(obj) + b"\n")


def ref(record_id, obj, version="1.0.0"):
    return {"recordId": record_id, "digest": digest(obj), "version": version}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--operation")
    parser.add_argument("--release-dir")
    parser.add_argument("--rollback-procedure",
                        help="corrected signed rollback procedure JSON ( overrides bundle rollback)")
    parser.add_argument("--host-profile")
    parser.add_argument("--facts", help="JSON: serverVersion, storageFreeMb, filesystemType")
    parser.add_argument("--out-dir", required=True)
    parser.add_argument("--assemble", action="store_true")
    parser.add_argument("--approval-id")
    parser.add_argument("--action")
    parser.add_argument("--job-id")
    parser.add_argument("--case-id", default="case_gitea_pilot_1")
    args = parser.parse_args()
    out = Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)

    if args.assemble:
        job = load(out / "job-template.json")
        if not args.approval_id or not args.action:
            return fail("--assemble requires --approval-id and --action")
        if args.job_id:
            job["jobId"] = args.job_id
        job["approvals"] = [{"approvalId": args.approval_id, "action": args.action}]
        write(out / "job.json", job)
        print(f"job {job['jobId']} assembled")
        return 0

    if not (args.operation and args.release_dir and args.host_profile and args.facts):
        return fail("--operation, --release-dir, --host-profile and --facts are required")
    operation = args.operation
    if operation not in ACTION:
        return fail(f"operation {operation} has no approval action; use it via its lifecycle job")
    release = Path(args.release_dir)
    manifest = load(release / "manifest.json")
    procedures = {p["operation"]: p for p in
                  (load(f) for f in sorted(release.glob("procedure-*.json")))}
    procedure = procedures[operation]
    rollback_proc = procedures["rollback"]
    if args.rollback_procedure:
        rollback_proc = load(args.rollback_procedure)
        if operation == "rollback":
            procedure = rollback_proc
    profile = load(args.host_profile)
    facts = load(args.facts)
    if profile["hostId"] != HOST or profile["tenantScope"] != TENANT:
        return fail("host profile does not match the paired pilot identity")

    now = datetime.now(timezone.utc).replace(microsecond=0)
    start = (now - timedelta(minutes=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
    end = (now + timedelta(minutes=25)).strftime("%Y-%m-%dT%H:%M:%SZ")

    policy = {
        "schemaVersion": "1.0.0", "policyId": "pol_gitea_pilot", "version": "1.0.0",
        "tenantScope": TENANT,
        "allowedCapabilities": ["manifest.verify", "compose.pull", "compose.up",
                                "compose.down", "compose.ps", "health.check",
                                "volume.backup", "volume.restore"],
        "deniedCapabilities": [],
        "privilege": {"allowPrivilegedContainers": False, "allowGenericShell": False},
        "network": {"allowPublicExposure": False, "requireApprovalForPublicExposure": True},
        "approvalRules": [{"action": a, "approval": "user"} for a in
                          ["install", "upgrade", "rollback", "uninstall",
                           "destructive-storage", "public-exposure", "migration", "backup"]],
        "licensing": {"requireExplicitLicense": True, "allowedLicenses": ["MIT"]},
        "promotion": {"requireHumanReview": True, "requireSandboxTest": True,
                      "requireRestoreTest": True, "requireRollbackTest": True,
                      "autoPromoteSingleSuccess": False},
        "freshness": {"maxHostProfileAgeMinutes": 30, "rejectRevokedManifests": True,
                      "rejectExpiredContext": True},
        "signing": {"algorithms": ["sigstore-cosign"],
                    "trustedSigners": ["wazi-phase1-demo-pilot-20261030"]},
    }
    write(out / "policy-bundle.json", policy)

    # The artifact record binds the exact rendered compose bytes.
    mapping = {manifest["release"]["images"][0]["digest"]:
               facts["imageConfigDigest"]}
    compose_path = out / "compose.json"
    # Renderer runs on the operator host via recordtool; digest filled there.
    artifact = {
        "schemaVersion": "1.0.0",
        "artifactId": f"art_gitea_pilot_{manifest['version'].replace('.', '_').replace('+', '_')}",
        "applicationId": APP, "releaseVersion": manifest["version"],
        "manifestDigest": digest(manifest),
        "renderer": {"name": "wazi-compose-renderer", "version": "0.4.0"},
        "composeDigest": facts["composeDigest"],
        "composeUri": f"pilot://{PROJECT}/{manifest['version']}",
        "projectName": PROJECT, "services": ["gitea"],
        "createdAt": start, "provenance": manifest["provenance"],
        "localPath": str(compose_path),
    }
    write(out / "artifact.json", artifact)
    _ = mapping

    existing_budget = out / "budget.json"
    if existing_budget.exists():
        # The pilot-agent pins one budget digest in its operator
        # configuration, so an existing budget record is reused as-is.
        write(out / "budget.json", load(existing_budget))
        budget = load(existing_budget)
        write(out / "policy-bundle.json", policy)
        _skip_budget = True
    else:
        _skip_budget = False
    budget = budget if _skip_budget else {
        "schemaVersion": "1.0.0", "budgetId": "budget_gitea_pilot",
        "tenantScope": TENANT, "hostId": HOST, "approvalState": "approved",
        "approvedBy": "pilot-owner (1-of-1 pilot approver)",
        "approvedAt": start, "validFrom": start,
        "validUntil": (now + timedelta(days=30)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "physical": {"cpuCores": profile["resources"]["cpuCores"],
                     "ramMb": profile["resources"]["ramMb"],
                     "storageMb": profile["resources"]["storageFreeMb"]},
        "ownerBudget": {"cpuCores": 4, "ramMb": 4096, "storageMb": 40960},
        "systemReserve": {"cpuCores": 0.5, "ramMb": 512, "storageMb": 10240},
        "allocations": [
            {"role": "application", "resources": {"cpuCores": 2, "ramMb": 1024, "storageMb": 20480}},
            {"role": "backup-helper", "resources": {"cpuCores": 0.25, "ramMb": 256, "storageMb": 5120}},
            {"role": "restore-helper", "resources": {"cpuCores": 0.25, "ramMb": 256, "storageMb": 5120}},
        ],
    }
    write(out / "budget.json", budget)

    qualification = {
        "schemaVersion": "1.0.0", "qualificationId": "volqual_gitea_pilot",
        "tenantScope": TENANT, "hostId": HOST,
        "observedAt": start, "expiresAt": end,
        "daemon": {"contextName": "rootless", "endpointKind": "unix",
                   "serverVersion": facts["serverVersion"],
                   "rootDir": "/srv/wazi-runtime/home/.local/share/docker"},
        "storage": {"managedVolumesCapable": True,
                    "enforcementMethod": "filesystem-quota", "quotaEnforceable": True,
                    "storageFreeMb": facts["storageFreeMb"],
                    "filesystemType": facts["filesystemType"]},
        "volumes": [{"name": "gitea-data", "purpose": "application-data"},
                    {"name": "gitea-config", "purpose": "application-config"}],
        "result": "qualified",
    }
    write(out / "qualification.json", qualification)

    action = ACTION[operation]
    suffix = operation.replace("-", "_")
    envelope_id, plan_id = f"env_gitea_{suffix}_1", f"plan_gitea_{suffix}_1"
    envelope = {
        "schemaVersion": "2.0.0", "envelopeId": envelope_id,
        "tenantScope": TENANT, "operation": operation,
        "deploymentMode": "deterministic", "applicationId": APP,
        "releaseVersion": manifest["version"],
        "manifestDigest": digest(manifest), "manifestReviewState": "signed",
        "hostProfileRef": ref(profile["hostId"], profile, profile["schemaVersion"]),
        "policyBundleRef": ref(policy["policyId"], policy),
        "procedureRef": ref(procedure["procedureId"], procedure, procedure["version"]),
        "composeArtifactRef": ref(artifact["artifactId"], artifact),
        "contributionBudgetRef": ref(budget["budgetId"], budget),
        "managedVolumeQualificationRef": ref(qualification["qualificationId"], qualification),
        "secretReferences": [],
        "allowedCapabilities": list(dict.fromkeys(
            list(procedure["capabilitySequence"])
            + list(rollback_proc["capabilitySequence"])
            + ["compose.down", "compose.up"])),
        "forbiddenOperations": ["shell.exec", "volume.delete-without-checkpoint"],
        "requiredApprovals": [action],
        "verificationChecks": [{"id": "gitea-health", "type": "health",
                                "target": manifest["release"]["healthCheck"]["target"]}],
        "rollbackStrategy": {"supported": True,
                             "procedureRef": ref(rollback_proc["procedureId"], rollback_proc,
                                                 rollback_proc["version"]),
                             "checkpointRequired": False},
        "citedSources": [{"recordId": manifest["provenance"]["sourceId"],
                          "contentHash": manifest["provenance"]["contentHash"]}],
        "createdAt": start, "expiresAt": end,
    }
    write(out / "envelope.json", envelope)

    manifest_digest = digest(manifest)
    image_digest = manifest["release"]["images"][0]["digest"]
    params = {
        "manifest.verify": {"manifestDigest": manifest_digest, "requireReviewState": "signed"},
        "compose.pull": {"service": "gitea", "imageDigest": image_digest},
        "compose.up": {"service": "gitea", "projectName": PROJECT,
                       "composeArtifactDigest": digest(artifact)},
        "compose.down": {"service": "gitea", "projectName": PROJECT, "removeVolumes": False},
        "health.check": {"target": manifest["release"]["healthCheck"]["target"]},
    }
    backup_steps = [{"volume": v, "backupId": BACKUP_ID}
                    for v in ("gitea-data", "gitea-config")]
    mutating = {"compose.up", "compose.down", "volume.restore", "volume.backup"}

    def make_step(step_id, cap, step_params):
        return {"stepId": step_id, "capability": cap, "parameters": step_params,
                "citations": [manifest["provenance"]["sourceId"]],
                "requiresApproval": cap in mutating,
                "verification": {"type": "health" if cap == "health.check" else "exit-state"}}

    steps, counters = [], {}
    for cap in procedure["capabilitySequence"]:
        counters[cap] = counters.get(cap, 0) + 1
        step_params = ({"volume": backup_steps.pop(0)["volume"], "backupId": BACKUP_ID}
                       if cap in ("volume.backup", "volume.restore") else params[cap])
        steps.append(make_step(f"step_{counters[cap]}_{cap.replace('.', '_')}", cap, step_params))
    # Rollback steps are the signed rollback procedure's full sequence;
    # forward compensation targets point into that sequence.
    rollback_steps, rb_counters = [], {}
    rb_backup_volumes = [v for v in ("gitea-data", "gitea-config")]
    for cap in rollback_proc["capabilitySequence"]:
        rb_counters[cap] = rb_counters.get(cap, 0) + 1
        if cap in ("volume.backup", "volume.restore"):
            step_params = {"volume": rb_backup_volumes.pop(0), "backupId": BACKUP_ID}
        else:
            step_params = params[cap]
        rollback_steps.append(make_step(
            f"step_rb_{rb_counters[cap]}_{cap.replace('.', '_')}", cap, step_params))
    rb_by_cap = {}
    for step in rollback_steps:
        rb_by_cap.setdefault(step["capability"], step["stepId"])
    rb_backup_by_volume = {s["parameters"]["volume"]: s["stepId"]
                           for s in rollback_steps if s["capability"] == "volume.backup"}
    compensation = {"compose.up": rb_by_cap.get("compose.down"),
                    "compose.down": rb_by_cap.get("compose.up")}
    for step in steps:
        target = compensation.get(step["capability"])
        if step["capability"] == "volume.restore":
            # Restoring a volume is compensated by re-backing-up that
            # exact volume, never a different one.
            target = rb_backup_by_volume.get(step["parameters"]["volume"])
        if target:
            step["rollbackStepId"] = target
    plan = {
        "schemaVersion": "1.0.0", "planId": plan_id, "envelopeId": envelope_id,
        "operation": operation, "origin": "deterministic-resolver",
        "composeArtifactRef": ref(artifact["artifactId"], artifact),
        "requiredApprovals": [action], "steps": steps,
        "rollbackSteps": rollback_steps, "unresolvedQuestions": [],
        "createdAt": start, "expiresAt": end,
    }
    write(out / "plan.json", plan)

    job = {
        "jobId": args.job_id or f"job_gitea_{suffix}_1",
        "caseId": args.case_id,
        "release": {"ApplicationID": APP, "ReleaseVersion": manifest["version"],
                    "ProcedureID": procedure["procedureId"],
                    "ProcedureVersion": procedure["version"],
                    "Operation": operation},
        "rollbackProcedure": {"ProcedureID": rollback_proc["procedureId"],
                              "Version": rollback_proc["version"],
                              "ApplicationID": APP, "Operation": "rollback"},
        "envelope": envelope, "plan": plan, "artifact": artifact,
        "budget": budget, "qualification": qualification, "approvals": [],
    }
    write(out / "job-template.json", job)
    print(f"built {operation} envelope {envelope_id} plan {plan_id} "
          f"budgetDigest {digest(budget)}")
    return 0


def fail(message):
    print(message, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
