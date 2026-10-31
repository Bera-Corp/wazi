"""Create unsigned, bounded pilot contracts from already verified image exports."""
import argparse
import datetime
import hashlib
import json
from pathlib import Path

import prepare_package_build
import context_engine as engine

SHAPES = {
    "postgresql": dict(service="postgresql", ram=384, cpu=0.5, storage=2048, volume="data", mount="/var/lib/postgresql/data", port=15432, containerPort=5432, health="postgresql://local/wazi", healthType="postgresql-authenticated"),
    "mysql": dict(service="mysql", ram=768, cpu=0.5, storage=2048, volume="data", mount="/var/lib/mysql", port=13306, containerPort=3306, health="mysql://local/wazi", healthType="mysql-authenticated"),
    "nginx": dict(service="nginx", ram=64, cpu=0.25, storage=128, volume="config", mount="/etc/nginx", port=18080, containerPort=80, health="http://127.0.0.1:18080/", healthType="nginx-config-and-http"),
}


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def verify_export(profile, metadata, attestations, archive=None):
    lock = prepare_package_build.locked_source(profile)
    if metadata["source"] != lock or metadata["profile"] != profile:
        raise ValueError("image-source-lock-mismatch")
    image = metadata["image"]
    if image["name"] != "ghcr.io/bera-corp/wazi-phase2-" + profile or image["platform"] != "linux/amd64":
        raise ValueError("image-identity-mismatch")
    statements = [entry.get("verificationResult", {}).get("statement", {}) for entry in attestations]
    if not any(statement.get("predicateType") == "https://wazi.dev/attestations/package-archive/v1"
               and statement.get("predicate") == metadata
               and any(subject.get("digest", {}).get("sha256") == metadata["archive"]["digest"].split(":")[1]
                       for subject in statement.get("subject", [])) for statement in statements):
        raise ValueError("verified-archive-predicate-mismatch")
    if archive:
        with Path(archive).open("rb") as handle:
            header = handle.read(512)
            if len(header) != 512 or header[257:263] not in (b"ustar\x00", b"ustar "):
                raise ValueError("image-archive-framing-invalid")
            handle.seek(0)
            digest = hashlib.file_digest(handle, "sha256").hexdigest()
        if "sha256:" + digest != metadata["archive"]["digest"]:
            raise ValueError("image-archive-digest-mismatch")
    return lock


def unsigned_records(profile, metadata, now):
    shape, source = SHAPES[profile], metadata["source"]
    version = source["upstreamVersion"]
    if version.count(".") == 1:
        version += ".0"
    version += "+wazi.1"
    provenance = {
        "sourceId": "https://github.com/" + source["repository"] + "/blob/" + source["commit"] + "/" + source["dockerfile"],
        "sourceType": "dockerfile", "contentHash": "sha256:" + source["dockerfileSha256"],
        "extractionMethod": "deterministic-parser", "applicationScope": source["applicationId"],
        "versionScope": version, "observedAt": now, "confidence": 0.95,
        "authorityLevel": "wazi-reviewed", "sensitivity": "public",
    }
    environment = []
    if profile != "nginx":
        prefix = "POSTGRES" if profile == "postgresql" else "MYSQL"
        environment = [
            {"service": shape["service"], "name": prefix + ("_DB" if profile == "postgresql" else "_DATABASE"), "default": "wazi"},
            {"service": shape["service"], "name": prefix + "_USER", "default": "wazi"},
            {"service": shape["service"], "name": prefix + "_PASSWORD_FILE", "secretRef": "secret://" + profile + "/password"},
        ]
        if profile == "mysql":
            environment.append({"service": shape["service"], "name": "MYSQL_ROOT_PASSWORD_FILE", "secretRef": "secret://mysql/root-password"})
    manifest = {
        "schemaVersion": "1.0.0", "appId": source["applicationId"],
        "name": profile.title() + " (bounded Wazi pilot)", "version": version,
        "license": source["license"], "reviewState": "signed", "provenance": provenance,
        "release": {
            "images": [{"service": shape["service"], "image": metadata["image"]["name"], "digest": metadata["image"]["digest"]}],
            "ports": [{"service": shape["service"], "hostPort": shape["port"], "containerPort": shape["containerPort"], "exposure": "loopback"}],
            "volumes": [{"service": shape["service"], "name": shape["volume"], "mountPath": shape["mount"], "backup": True, "storageLimitMb": shape["storage"]}],
            "resourceLimits": [{"service": shape["service"], "cpuCores": shape["cpu"], "ramMb": shape["ram"]}],
            "environment": environment,
            "healthCheck": {"type": shape["healthType"], "target": shape["health"]},
            "backup": {"supported": True, "procedureRef": "proc_backup_" + profile},
            "rollback": {"supported": False, "notes": "No cross-version downgrade or automatic compensation is qualified. Checkpoint restore is a separately approved operation."},
        },
        "requirements": {"architectures": ["x86_64"], "osFamilies": ["linux"],
                         "minCpuCores": shape["cpu"], "minRamMb": max(128, shape["ram"]),
                         "minStorageMb": shape["storage"], "minDockerVersion": "29.1.3",
                         "minComposeVersion": "2.40.3", "privileged": False},
    }
    restore = ["manifest.verify", "volume.restore"] + (["compose.up"] if profile == "nginx" else []) + ["health.check"]
    sequences = {"install": ["manifest.verify", "compose.pull", "compose.up", "health.check"],
                 "upgrade": ["manifest.verify", "compose.pull", "compose.up", "health.check"],
                 "backup": ["manifest.verify", "health.check", "volume.backup", "health.check"],
                 "restore": ["manifest.verify"] + ([] if profile == "nginx" else ["health.check"]) + restore[1:],
                 "uninstall": ["manifest.verify", "compose.down"]}
    required = {"manifest.verify": ["manifestDigest", "requireReviewState"], "compose.pull": ["service", "imageDigest"],
                "compose.up": ["service", "projectName", "composeArtifactDigest"], "compose.down": ["service", "projectName", "removeVolumes"],
                "health.check": ["target"], "volume.backup": ["volume", "backupId"], "volume.restore": ["volume", "backupId"]}
    fixed = {"manifest.verify": {"requireReviewState": "signed"}, "compose.pull": {"service": shape["service"], "imageDigest": metadata["image"]["digest"]},
             "compose.up": {"service": shape["service"]}, "compose.down": {"service": shape["service"], "removeVolumes": False},
             "health.check": {"target": shape["health"]}, "volume.backup": {"volume": shape["volume"]}, "volume.restore": {"volume": shape["volume"]}}
    records = {"manifest": manifest}
    expires = (datetime.datetime.fromisoformat(now.replace("Z", "+00:00")) + datetime.timedelta(days=7)).isoformat(timespec="seconds").replace("+00:00", "Z")
    provenance["expiresAt"] = expires
    for operation, sequence in sequences.items():
        records["procedure-" + operation] = {
            "schemaVersion": "1.0.0", "procedureId": "proc_" + operation + "_" + profile,
            "version": "1.0.0", "operation": operation, "applicationId": source["applicationId"],
            "capabilitySequence": sequence,
            "parameterContracts": [{"capability": capability, "requiredParameters": required[capability], "fixedParameters": fixed[capability]}
                                   for capability in dict.fromkeys(sequence)],
            "requiresCheckpoint": operation == "restore", "provenance": provenance,
            "reviewState": "signed",
        }
    return records


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile", choices=sorted(SHAPES), required=True)
    parser.add_argument("--export", type=Path, required=True)
    parser.add_argument("--verified-attestation", type=Path, required=True)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    metadata = json.loads(args.export.read_text())
    attestations = json.loads(args.verified_attestation.read_text())
    verify_export(args.profile, metadata, attestations, args.archive)
    now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
    args.out.mkdir(parents=True, exist_ok=True)
    for name, record in unsigned_records(args.profile, metadata, now).items():
        schema = "app-manifest.schema.json" if name == "manifest" else "procedure.schema.json"
        # Validate shape before signing without fabricating a signature.
        errors = engine._record_schema_errors(schema, dict(record, reviewState="candidate"))
        if errors:
            raise ValueError("unsigned-contract-schema-invalid: " + str(errors))
        (args.out / (name + ".unsigned.json")).write_bytes(canonical(record))
    (args.out / "image-export.json").write_bytes(canonical(metadata))
    (args.out / "verified-archive-attestation.json").write_bytes(canonical(attestations))
    print(args.profile + ": unsigned contracts prepared; signing does not imply live qualification")


if __name__ == "__main__":
    main()
