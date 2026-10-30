#!/usr/bin/env python3
"""Deterministic Compose renderer (Phase 0 reference implementation).

Builds a structured Compose object from one app-manifest record and
serializes it with a safe YAML emitter. Manifest strings are data:
a default containing newlines or YAML-looking text can never become
Compose structure. Same manifest in -> same bytes out.

Port exposure contract:
  internal  -> no host publication; container port listed under expose
  loopback  -> bound to 127.0.0.1 only
  private   -> bound to 127.0.0.1; wider access is via the reverse proxy
               (proxy.configure), never a direct all-interfaces binding
  public    -> published on all interfaces; semantic validation derives
               a public-exposure approval requirement from this

Usage: python scripts/render_compose.py context/examples/app-manifest.json
"""
import json
import sys
from pathlib import Path

import yaml

RENDERER_VERSION = "0.2.0"


def port_binding(port):
    exposure = port["exposure"]
    container = port["containerPort"]
    if exposure == "internal":
        return None
    if "hostPort" not in port:
        raise ValueError(f"port {container} with exposure {exposure} requires hostPort")
    host = port["hostPort"]
    if exposure in ("loopback", "private"):
        return f"127.0.0.1:{host}:{container}"
    if exposure == "public":
        return f"{host}:{container}"
    raise ValueError(f"unknown port exposure {exposure}")


def escape_interpolation(value):
    """Escape literal dollars so Compose cannot substitute host values.

    Compose interpolation treats $VAR and ${VAR} as host substitution;
    $$ is the documented literal-dollar escape. Every literal dollar in
    a manifest-provided value is escaped, so rendered values mean
    exactly what the manifest says and nothing is pulled from the host
    environment (declared or secret) implicitly.
    """
    return str(value).replace("$", "$$")


def owned(item, service_name, service_names, multi_service, kind):
    owner = item.get("service")
    if owner is not None and owner not in service_names:
        raise ValueError(f"{kind} entry '{item.get('name')}' declares unknown service owner '{owner}'")
    if owner is not None:
        return owner == service_name
    if multi_service:
        raise ValueError(
            f"{kind} entry '{item.get('name')}' has no service owner; "
            "explicit service ownership is required when a manifest has multiple services"
        )
    return True


def build_compose(manifest):
    release = manifest["release"]
    multi_service = len(release["images"]) > 1
    service_names = {image["service"] for image in release["images"]}
    services = {}
    volume_names = []
    for image in release["images"]:
        service_name = image["service"]
        service = {"image": f"{image['image']}@{image['digest']}"}
        bindings, exposed = [], []
        for port in release.get("ports", []):
            if port["service"] != service_name:
                continue
            binding = port_binding(port)
            if binding is None:
                exposed.append(str(port["containerPort"]))
            else:
                bindings.append(binding)
        if bindings:
            service["ports"] = bindings
        if exposed:
            service["expose"] = exposed
        environment = []
        for entry in release.get("environment", []):
            if not owned(entry, service_name, service_names, multi_service, "environment"):
                continue
            if "secretRef" in entry:
                environment.append(entry["name"])
            else:
                environment.append(f"{entry['name']}={escape_interpolation(entry.get('default', ''))}")
        if environment:
            service["environment"] = environment
        svc_volumes = []
        for volume in release.get("volumes", []):
            if not owned(volume, service_name, service_names, multi_service, "volume"):
                continue
            svc_volumes.append(f"{volume['name']}:{volume['mountPath']}")
            if volume["name"] not in volume_names:
                volume_names.append(volume["name"])
        if svc_volumes:
            service["volumes"] = svc_volumes
        services[service_name] = service
    compose = {"services": services}
    if volume_names:
        compose["volumes"] = {name: {} for name in volume_names}
    return compose


def render(manifest):
    if "resourceLimits" in manifest.get("release", {}):
        raise ValueError("resourceLimits requires wazi-compose-renderer 0.3.0")
    text = yaml.safe_dump(
        build_compose(manifest),
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
