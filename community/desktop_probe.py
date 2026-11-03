"""Read-only desktop/guest observations; this probe never awards qualification."""
import argparse
import json
import os
import platform
import re
import subprocess
import urllib.parse
from pathlib import Path

from .contracts import canonical, environment


def run(executable, *arguments):
    result = subprocess.run([executable, *arguments], stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15, check=False)
    if result.returncode or len(result.stdout) > 1 << 20:
        raise ValueError("Docker observation failed")
    return result.stdout.decode("utf-8").strip()


def observe(family, executable="docker", invoke=run, system=None, desktop_version=None):
    system = platform.system() if system is None else system
    expected = {"windows-wsl2": "Windows", "macos-desktop": "Darwin", "linux-engine": "Linux"}
    if expected.get(family) != system:
        raise ValueError("desktop family does not match this machine")
    name = invoke(executable, "context", "show")
    contexts = json.loads(invoke(executable, "context", "inspect", name))
    if not isinstance(contexts, list) or len(contexts) != 1:
        raise ValueError("one explicit Docker context required")
    endpoint = contexts[0]["Endpoints"]["docker"]["Host"]
    if not isinstance(endpoint, str) or not endpoint.startswith(("unix://", "npipe://")):
        raise ValueError("remote Docker contexts cannot supply local desktop observations")
    if family in {"windows-wsl2", "macos-desktop"} and name != "desktop-linux":
        raise ValueError("desktop adapter requires explicit desktop-linux context")
    info = json.loads(invoke(executable, "info", "--format", "{{json .}}"))
    if info.get("OSType") != "linux":
        raise ValueError("Linux execution guest required")
    architecture = {"x86_64": "amd64", "aarch64": "arm64", "amd64": "amd64", "arm64": "arm64"}.get(info.get("Architecture"))
    compose = invoke(executable, "compose", "version", "--short").removeprefix("v")
    if desktop_version is None:
        desktop_version = platform.mac_ver()[0] if system == "Darwin" else platform.version() if system == "Windows" else "0"
    public = {"family": family, "architecture": architecture, "desktopVersion": desktop_version,
              "guestVersion": info.get("KernelVersion", ""), "dockerVersion": info.get("ServerVersion", ""),
              "composeVersion": compose, "enforcement": "unverified"}
    environment(public)
    return {"schemaVersion": "1.0.0", "environment": public, "qualification": "not-qualified",
            "reason": "Read-only observations do not prove resource limits, storage quotas, persistence or recovery."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--family", required=True, choices=["linux-engine", "windows-wsl2", "macos-desktop"])
    parser.add_argument("--docker", default="docker")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    result = observe(args.family, args.docker)
    target = Path(args.output)
    with target.open("xb") as stream:
        stream.write(canonical(result))
    os.chmod(target, 0o600)
    print("Desktop/guest observations saved. No containers were started and no qualification was granted.")


if __name__ == "__main__":
    main()
