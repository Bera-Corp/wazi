#!/usr/bin/env python3
"""Recompute every digest binding in the example deployment graph.

Run after editing any example record:
    python scripts/bind_examples.py

Bindings recomputed: compose artifact bytes (via render_compose),
manifest/artifact/envelope/plan/approval/case/evidence digests, and
signature payloadDigests (canonical digest of the record without its
top-level signature field). This is a demo convenience, not a signer:
signature values remain placeholders.
"""
import hashlib
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EX = ROOT / "context" / "examples"

spec = importlib.util.spec_from_file_location("render_compose", ROOT / "scripts" / "render_compose.py")
renderer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(renderer)


def load(name):
    return json.loads((EX / name).read_text(encoding="utf-8-sig"))


def save(name, obj):
    (EX / name).write_text(json.dumps(obj, indent=2) + "\n", encoding="utf-8")


def canonical(obj):
    raw = json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def payload_digest(record):
    unsigned = {k: v for k, v in record.items() if k != "signature"}
    return canonical(unsigned)


def main():
    manifest = load("app-manifest.json")
    host, policy = load("host-profile.json"), load("policy-bundle.json")
    proc_i, proc_r = load("procedure-install.json"), load("procedure-rollback.json")
    artifact, envelope, plan = load("compose-artifact.json"), load("execution-context-envelope.json"), load("deployment-plan.json")
    approval, case, evidence = load("approval-record.json"), load("deployment-case.json"), load("evidence-record.json")

    for record in (manifest, proc_i, proc_r):
        if "signature" in record:
            record["signature"]["payloadDigest"] = payload_digest(record)

    compose_bytes = renderer.render(manifest)
    compose_path = EX / "compose-artifact.yaml"
    compose_path.write_bytes(compose_bytes)
    artifact["composeDigest"] = "sha256:" + hashlib.sha256(compose_bytes).hexdigest()
    artifact["localPath"] = "context/examples/compose-artifact.yaml"
    artifact["manifestDigest"] = canonical(manifest)

    envelope["manifestDigest"] = canonical(manifest)
    envelope["hostProfileRef"]["digest"] = canonical(host)
    envelope["policyBundleRef"]["digest"] = canonical(policy)
    envelope["procedureRef"]["digest"] = canonical(proc_i)
    envelope["composeArtifactRef"]["digest"] = canonical(artifact)
    envelope["rollbackStrategy"]["procedureRef"]["digest"] = canonical(proc_r)
    for source in envelope["citedSources"]:
        if source["recordId"] == host["hostId"]:
            source["contentHash"] = canonical(host)

    plan["composeArtifactRef"]["digest"] = canonical(artifact)
    for step in plan["steps"]:
        if step["capability"] == "manifest.verify":
            step["parameters"]["manifestDigest"] = canonical(manifest)
        if step["capability"] == "compose.up":
            step["parameters"]["composeArtifactDigest"] = artifact["composeDigest"]

    approval["planDigest"] = canonical(plan)
    approval["envelopeDigest"] = canonical(envelope)
    case["manifestDigest"] = canonical(manifest)
    evidence["commitOrReleaseDigest"] = canonical(manifest)

    for name, obj in [
        ("app-manifest.json", manifest), ("policy-bundle.json", policy),
        ("procedure-install.json", proc_i), ("procedure-rollback.json", proc_r),
        ("compose-artifact.json", artifact), ("execution-context-envelope.json", envelope),
        ("deployment-plan.json", plan), ("approval-record.json", approval),
        ("deployment-case.json", case), ("evidence-record.json", evidence),
    ]:
        save(name, obj)
    print("bound: manifest", canonical(manifest))
    print("bound: compose bytes sha256:" + hashlib.sha256(compose_bytes).hexdigest())


if __name__ == "__main__":
    main()
