#!/usr/bin/env python3
"""Deterministic Phase 1 Compose renderer with hard CPU and memory limits."""
import json
import math
import sys
from pathlib import Path

import yaml

from render_compose import build_compose

RENDERER_VERSION = "0.3.0"
MIB_BYTES = 1024 * 1024


def apply_resource_limits(compose, manifest):
    release = manifest["release"]
    images = release["images"]
    limits = release.get("resourceLimits")
    if not isinstance(limits, list) or not limits:
        raise ValueError("Phase 1 manifests require one resource limit per service")

    services = [image["service"] for image in images]
    if any(not isinstance(service, str) or not service for service in services):
        raise ValueError("every image must declare a non-empty service name")
    if len(set(services)) != len(services):
        raise ValueError("manifest image services must be unique")
    if len(limits) != len(services):
        raise ValueError("resourceLimits must cover every service exactly once")

    expected = set(services)
    seen = set()
    for limit in limits:
        if not isinstance(limit, dict):
            raise ValueError("each resource limit must be an object")
        if set(limit) != {"service", "cpuCores", "ramMb"}:
            raise ValueError("each resource limit must declare only service, cpuCores, and ramMb")
        service_name = limit.get("service")
        if service_name not in expected:
            raise ValueError(f"resource limit declares unknown service {service_name!r}")
        if service_name in seen:
            raise ValueError(f"duplicate resource limit for service {service_name!r}")
        cpu = limit.get("cpuCores")
        ram_mb = limit.get("ramMb")
        if isinstance(cpu, bool) or not isinstance(cpu, (int, float)) or not math.isfinite(cpu) or cpu <= 0:
            raise ValueError(f"service {service_name!r} has an invalid CPU limit")
        if isinstance(ram_mb, bool) or not isinstance(ram_mb, int) or ram_mb <= 0:
            raise ValueError(f"service {service_name!r} has an invalid memory limit")
        service = compose["services"].get(service_name)
        if service is None:
            raise ValueError(f"resource limit service {service_name!r} was not rendered")
        service["cpus"] = cpu
        service["mem_limit"] = f"{ram_mb * MIB_BYTES}b"
        seen.add(service_name)

    if seen != expected:
        raise ValueError("resourceLimits must cover every service exactly once")
    return compose


def render(manifest):
    if not isinstance(manifest, dict) or not isinstance(manifest.get("release"), dict):
        raise ValueError("manifest release must be an object")
    images = manifest["release"].get("images")
    if not isinstance(images, list) or not images or any(not isinstance(image, dict) for image in images):
        raise ValueError("manifest images must be a non-empty array of objects")
    if any(not isinstance(image.get("service"), str) or not image["service"] for image in images):
        raise ValueError("every image must declare a non-empty service name")
    compose = apply_resource_limits(build_compose(manifest), manifest)
    text = yaml.safe_dump(
        compose,
        sort_keys=False,
        allow_unicode=True,
        default_flow_style=False,
    )
    header = f"# Rendered by wazi-compose-renderer {RENDERER_VERSION} - do not edit by hand.\n"
    return (header + text).encode("utf-8")


def main(argv):
    if len(argv) != 2:
        print(__doc__)
        return 2
    manifest = json.loads(Path(argv[1]).read_text(encoding="utf-8-sig"))
    sys.stdout.buffer.write(render(manifest))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
