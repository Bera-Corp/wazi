#!/usr/bin/env python3
"""Renderer safety tests: parsed Compose output, not string matching.

Covers environment-value injection (YAML injection through a default),
multiline values, and exposure-aware port bindings.
"""
import copy
import importlib.util
import json
import sys
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
spec = importlib.util.spec_from_file_location("render_compose", HERE / "render_compose.py")
renderer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(renderer)


def base_manifest():
    return json.loads((ROOT / "context" / "examples" / "app-manifest.json").read_text(encoding="utf-8-sig"))


def parse(manifest):
    return yaml.safe_load(renderer.render(manifest).decode("utf-8"))


def check(label, condition):
    print(("PASS " if condition else "FAIL ") + label)
    return bool(condition)


def main():
    results = []

    manifest = base_manifest()
    manifest["release"]["environment"] = [
        {"name": "DEBUG", "default": "on\n    privileged: true"},
        {"name": "MULTILINE", "default": "line one\nline two"},
        {"name": "APP_SECRET", "secretRef": "secret://deployment/app-secret"},
    ]
    parsed = parse(manifest)
    service = parsed["services"]["app"]
    results.append(check("environment injection cannot create privileged service",
                         "privileged" not in service))
    env = dict(item.split("=", 1) if "=" in item else (item, None) for item in service["environment"])
    results.append(check("multiline default survives as one value",
                         env.get("DEBUG") == "on\n    privileged: true" and env.get("MULTILINE") == "line one\nline two"))
    results.append(check("manifest still declares privileged false",
                         manifest["requirements"]["privileged"] is False))

    for exposure, expected_ports in [
        ("internal", None),
        ("loopback", ["127.0.0.1:3000:3000"]),
        ("private", ["127.0.0.1:3000:3000"]),
        ("public", ["3000:3000"]),
    ]:
        mutated = base_manifest()
        mutated["release"]["ports"] = [
            {"service": "app", "containerPort": 3000, "hostPort": 3000, "exposure": exposure}
        ]
        svc = parse(mutated)["services"]["app"]
        if expected_ports is None:
            ok = "ports" not in svc and [str(p) for p in svc.get("expose", [])] == ["3000"]
        else:
            ok = svc.get("ports") == expected_ports
        results.append(check(f"exposure {exposure} renders safe binding", ok))

    interp = base_manifest()
    interp["release"]["environment"] = [
        {"name": "LITERAL", "default": "${WAZI_HOST_PRIVATE_VALUE}"},
        {"name": "PLAIN", "default": "costs $5"},
    ]
    parsed = parse(interp)
    env_items = parsed["services"]["app"]["environment"]
    results.append(check("interpolation syntax is escaped to a literal",
                         "LITERAL=$${WAZI_HOST_PRIVATE_VALUE}" in env_items
                         and "PLAIN=costs $$5" in env_items))

    multi = base_manifest()
    multi["release"]["images"].append(
        {"service": "worker", "image": "ghcr.io/example/worker",
         "digest": "sha256:" + "3" * 64})
    try:
        renderer.render(multi)
        unowned_rejected = False
    except ValueError:
        unowned_rejected = True
    results.append(check("multi-service manifest with unowned env/volumes is rejected", unowned_rejected))

    multi["release"]["environment"] = [
        {"name": "APP_SECRET", "secretRef": "secret://deployment/app-secret", "service": "app"}]
    multi["release"]["volumes"] = [
        {"name": "app-data", "mountPath": "/data", "backup": True, "service": "app"}]
    parsed = parse(multi)
    worker = parsed["services"]["worker"]
    results.append(check("scoped env and volumes are not copied to other services",
                         "environment" not in worker and "volumes" not in worker
                         and parsed["services"]["app"]["environment"] == ["APP_SECRET"]))

    ghost = base_manifest()
    ghost["release"]["environment"] = [
        {"name": "APP_SECRET", "secretRef": "secret://deployment/app-secret", "service": "ghost"}]
    try:
        renderer.render(ghost)
        ghost_rejected = False
    except ValueError:
        ghost_rejected = True
    results.append(check("unknown service owner is rejected, not silently omitted", ghost_rejected))

    results.append(check("rendered bytes are deterministic",
                         renderer.render(base_manifest()) == renderer.render(base_manifest())))

    print("ALL PASS" if all(results) else "SOME FAILED")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
