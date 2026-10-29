#!/usr/bin/env python3
"""Experimental S-Print contract resolver. Reads JSON; never executes package code.

Compatibility is an offline metadata decision, not signature verification,
evaluation success, permission enforcement, or authority to deploy.
"""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parent.parent
SCHEMA_NAMES = ("sprint-manifest.schema.json", "world-composition.schema.json")


def digest(record):
    return "sha256:" + hashlib.sha256(
        json.dumps(record, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    ).hexdigest()


def validators():
    schemas = {}
    registry = Registry()
    for name in SCHEMA_NAMES:
        schema = json.loads((ROOT / "schemas" / "experimental" / name).read_text(encoding="utf-8"))
        Draft202012Validator.check_schema(schema)
        schemas[name] = schema
        registry = registry.with_resource(schema["$id"], Resource.from_contents(schema))
    return {name: Draft202012Validator(schema, registry=registry, format_checker=FormatChecker())
            for name, schema in schemas.items()}


def resolve(composition):
    errors = ["schema: " + e.message for e in validators()["world-composition.schema.json"].iter_errors(composition)]
    if errors:
        return {"decision": "reject", "executionAuthorized": False, "errors": errors}
    runtime = composition["runtime"]
    packages = composition["packages"]
    by_id = {p["packageId"]: p for p in packages}
    if len(by_id) != len(packages):
        errors.append("duplicate-package-id")
    providers = {}
    edges = {key: set() for key in by_id}

    def key(interface):
        return (interface["id"], interface["version"], interface["contractDigest"])

    def provide(interface, owner):
        # One provider per interface ID; routing/override composition is deferred.
        if interface["id"] in providers:
            errors.append("ambiguous-interface-provider: " + interface["id"])
        else:
            providers[interface["id"]] = (key(interface), owner)

    for interface in runtime["provides"]:
        provide(interface, None)
    for package in packages:
        for interface in package["provides"]:
            provide(interface, package["packageId"])

    ram = runtime["baseResources"]["ramMb"]
    gpu = runtime["baseResources"]["gpuVramMb"]
    for package in packages:
        pid = package["packageId"]
        if package["baseModelDigest"] != runtime["baseModelDigest"]:
            errors.append("base-model-mismatch: " + pid)
        if "training" in package["classes"] or package["depth"] == "training":
            errors.append("training-not-supported: " + pid)
        if not set(package["permissions"]) <= set(runtime["allowedPermissions"]):
            errors.append("permission-denied: " + pid)
        ram += package["resources"]["ramMb"]
        gpu += package["resources"]["gpuVramMb"]
        for asset in package["assets"]:
            path = asset["path"]
            if path.startswith("/") or "\\" in path or ":" in path or any(part in ("", ".", "..") for part in path.split("/")):
                errors.append("unsafe-asset-path: " + pid)
            if PurePosixPath(path).is_absolute():
                errors.append("absolute-asset-path: " + pid)
        for dependency in package["dependencies"]:
            target = by_id.get(dependency["packageId"])
            if target is None:
                errors.append("missing-dependency: " + dependency["packageId"])
            elif target["version"] != dependency["version"] or digest(target) != dependency["digest"]:
                errors.append("dependency-pin-mismatch: " + dependency["packageId"])
            else:
                edges[pid].add(target["packageId"])
        for interface in package["requires"]:
            provider = providers.get(interface["id"])
            if provider is None or provider[0] != key(interface):
                errors.append("missing-or-incompatible-interface: " + interface["id"])
            elif provider[1] == pid:
                errors.append("self-provided-requirement: " + pid)
            elif provider[1] is not None:
                edges[pid].add(provider[1])
    if ram > runtime["availableResources"]["ramMb"] or gpu > runtime["availableResources"]["gpuVramMb"]:
        errors.append("insufficient-composition-resources")
    order = []
    pending = {pid: set(deps) for pid, deps in edges.items()}
    while pending:
        ready = sorted(pid for pid, deps in pending.items() if not deps)
        if not ready:
            errors.append("composition-cycle")
            break
        order.extend(ready)
        for pid in ready:
            del pending[pid]
        for deps in pending.values():
            deps.difference_update(ready)
    result = {
        "decision": "reject" if errors else "compatible-offline",
        "executionAuthorized": False,
        "errors": sorted(set(errors)),
        "compositionDigest": digest(composition),
        "evaluationStatus": "not-run",
    }
    if not errors:
        result["loadOrder"] = order
        result["packagePins"] = [{"packageId": pid, "version": by_id[pid]["version"], "digest": digest(by_id[pid])} for pid in order]
        result["reservedResources"] = {"ramMb": ram, "gpuVramMb": gpu}
    return result


def validate_examples():
    path = ROOT / "context" / "worlds" / "simulation-composition.json"
    record = json.loads(path.read_text(encoding="utf-8"))
    result = resolve(record)
    if result["decision"] != "compatible-offline":
        raise ValueError("S-Print example: " + "; ".join(result["errors"]))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("composition", type=Path)
    args = parser.parse_args()
    try:
        result = resolve(json.loads(args.composition.read_text(encoding="utf-8-sig")))
    except (ValueError, OSError) as exc:
        parser.exit(1, "Cannot read/validate composition: " + str(exc) + "\n")
    print(json.dumps(result, indent=2))
    return 1 if result["decision"] == "reject" else 0


if __name__ == "__main__":
    raise SystemExit(main())
