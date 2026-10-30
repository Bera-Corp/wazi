#!/usr/bin/env python3
"""Phase 1 Compose renderer tests for hard per-service resource ceilings."""
import copy
import importlib.util
import json
import sys
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
sys.path.insert(0, str(HERE))
spec = importlib.util.spec_from_file_location("render_compose_v1", HERE / "render_compose_v1.py")
renderer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(renderer)
old_spec = importlib.util.spec_from_file_location("render_compose", HERE / "render_compose.py")
old_renderer = importlib.util.module_from_spec(old_spec)
old_spec.loader.exec_module(old_renderer)
validate_spec = importlib.util.spec_from_file_location("wazi_validate_renderer_test", HERE / "validate.py")
validate_module = importlib.util.module_from_spec(validate_spec)
validate_spec.loader.exec_module(validate_module)


def base_manifest():
    manifest = json.loads((ROOT / "context" / "examples" / "app-manifest.json").read_text(encoding="utf-8-sig"))
    manifest["release"]["resourceLimits"] = [
        {"service": "app", "cpuCores": 1.5, "ramMb": 1024}
    ]
    return manifest


def check(label, condition):
    print(("PASS " if condition else "FAIL ") + label)
    return bool(condition)


def main():
    results = []
    manifest = base_manifest()
    rendered = renderer.render(manifest)
    parsed = yaml.safe_load(rendered.decode("utf-8"))
    service = parsed["services"]["app"]
    results.append(check("Phase 1 renderer version is explicit", b"renderer 0.3.0" in rendered.splitlines()[0]))
    results.append(check("validator dispatch recognizes renderer 0.3.0",
                         validate_module.load_compose_renderer("0.3.0").RENDERER_VERSION == "0.3.0"))
    results.append(check("CPU ceiling is rendered", service.get("cpus") == 1.5))
    results.append(check("MiB memory ceiling is rendered as exact bytes", service.get("mem_limit") == "1073741824b"))
    results.append(check("rendered bytes are deterministic", rendered == renderer.render(base_manifest())))

    try:
        old_renderer.render(manifest)
        old_renderer_rejected = False
    except ValueError:
        old_renderer_rejected = True
    results.append(check("0.2.0 renderer refuses resource-limited manifests", old_renderer_rejected))

    invalid_cases = [
        ("missing limits", lambda value: value["release"].pop("resourceLimits")),
        ("missing service limit", lambda value: value["release"]["resourceLimits"].clear()),
        ("duplicate service limit", lambda value: value["release"]["resourceLimits"].append(copy.deepcopy(value["release"]["resourceLimits"][0]))),
        ("unknown service limit", lambda value: value["release"]["resourceLimits"][0].update(service="ghost")),
        ("zero CPU limit", lambda value: value["release"]["resourceLimits"][0].update(cpuCores=0)),
        ("fractional MiB limit", lambda value: value["release"]["resourceLimits"][0].update(ramMb=1.5)),
    ]
    for label, mutate in invalid_cases:
        candidate = base_manifest()
        mutate(candidate)
        try:
            renderer.render(candidate)
            rejected = False
        except (TypeError, ValueError):
            rejected = True
        results.append(check(f"{label} is rejected", rejected))

    multi = base_manifest()
    multi["release"]["images"].append(
        {"service": "worker", "image": "ghcr.io/example/worker", "digest": "sha256:" + "3" * 64}
    )
    try:
        renderer.render(multi)
        incomplete_rejected = False
    except ValueError:
        incomplete_rejected = True
    results.append(check("new service without its own limit is rejected", incomplete_rejected))

    print("ALL PASS" if all(results) else "SOME FAILED")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
