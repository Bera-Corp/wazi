#!/usr/bin/env python3
"""Prepare pilot release records from verified image-export statements.

This command does not sign or install trust. Its caller must verify the GitHub
attestation before passing the predicate and must sign every prepared record.
"""
import argparse
import copy
import datetime
import json
from pathlib import Path

from validate import canonical_digest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("predicate", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    predicate = json.loads(args.predicate.read_text(encoding="utf-8-sig"))
    tag = predicate["source"]["tag"]
    expected = {
        "v28.1.0": ("41113830342", "sha256:afd05701641f238dc198f98d7733b1d3b09cb6ae1a3f1cd9b3bc2058e706570b"),
        "v28.2.0": ("41113830385", "sha256:7b7f6f93ac9f4d0a50abd1c0c5f204631f0813723e13529d5e14754288d53ce2"),
    }
    if tag not in expected:
        raise SystemExit("unapproved source tag")
    run, image_digest = expected[tag]
    if (predicate["image"]["digest"] != image_digest
            or predicate["exporter"]["runId"] != run
            or predicate["exporter"]["workflowCommit"] != "3217408b0e6026638af4561276c6e8950cdd5d27"
            or predicate["image"]["name"] != "ghcr.io/bera-corp/wazi-gitea-phase1"):
        raise SystemExit("predicate does not match the approved A/B build")
    args.output.mkdir(parents=True, exist_ok=True)
    now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
    manifest = json.loads((root / "releases/phase1/gitea/28.2.0/manifest.json").read_text(encoding="utf-8-sig"))
    manifest.pop("signature", None)
    version = tag.removeprefix("v") + "+wazipilot.2"
    manifest["version"] = version
    manifest["release"]["images"][0]["digest"] = image_digest
    manifest["release"]["ports"] = [{"service": "gitea", "containerPort": 3000, "hostPort": 13000, "exposure": "loopback"}]
    manifest["release"]["healthCheck"]["target"] = "http://127.0.0.1:13000/api/healthz"
    manifest["release"]["environment"] = [
        {"service": "gitea", "name": name, "default": value}
        for name, value in {
            "GITEA__DATABASE__DB_TYPE": "sqlite3",
            "GITEA__DATABASE__PATH": "/var/lib/gitea/data/gitea.db",
            "GITEA__SECURITY__INSTALL_LOCK": "true",
            "GITEA__SERVER__ROOT_URL": "http://127.0.0.1:13000/",
            "GITEA__SERVER__DISABLE_SSH": "true",
            "GITEA__SERVICE__DISABLE_REGISTRATION": "false",
            "GITEA__SERVICE__REGISTER_EMAIL_CONFIRM": "false",
        }.items()
    ]
    provenance = manifest["provenance"]
    provenance.update(sourceId="src_gitea_" + tag.replace(".", "_"), sourceType="container-registry",
                      contentHash=canonical_digest(predicate), versionScope=version, observedAt=now,
                      authorityLevel="wazi-reviewed")
    records = {"manifest": manifest}
    suffix = tag.removeprefix("v").replace(".", "_")
    for operation, sequence in {
        "install": ["manifest.verify", "compose.pull", "compose.up", "health.check"],
        "upgrade": ["manifest.verify", "volume.backup", "compose.pull", "compose.up", "health.check"],
        "backup": ["manifest.verify", "volume.backup", "volume.backup"],
        "restore": ["manifest.verify", "volume.restore", "volume.restore", "compose.up", "health.check"],
        "rollback": ["manifest.verify", "compose.up", "health.check"],
        "uninstall": ["manifest.verify", "compose.down"],
        "health-check": ["manifest.verify", "health.check"],
    }.items():
        parameters = {
            "manifest.verify": ["manifestDigest", "requireReviewState"],
            "compose.pull": ["service", "imageDigest"],
            "compose.up": ["service", "projectName", "composeArtifactDigest"],
            "compose.down": ["service", "projectName"],
            "health.check": ["target"],
            "volume.backup": ["volume", "backupId"],
            "volume.restore": ["volume", "backupId"],
        }
        records["procedure-" + operation] = {
            "schemaVersion": "1.0.0", "procedureId": "proc_gitea_" + operation.replace("-", "_") + "_" + suffix,
            "version": "1.0.0", "operation": operation, "applicationId": "org.gitea.gitea",
            "capabilitySequence": sequence,
            "parameterContracts": [{"capability": cap, "requiredParameters": parameters[cap]} for cap in dict.fromkeys(sequence)],
            "requiresCheckpoint": operation in ("upgrade", "restore", "rollback"),
            "provenance": copy.deepcopy(provenance), "reviewState": "signed",
        }
    for name, record in records.items():
        (args.output / (name + ".unsigned.json")).write_text(json.dumps(record, ensure_ascii=False, sort_keys=True, separators=(",", ":")), encoding="utf-8")
    (args.output / "image-export.json").write_text(json.dumps(predicate, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    print(f"Prepared {len(records)} unsigned records for {version}; signing still required")


if __name__ == "__main__":
    main()
