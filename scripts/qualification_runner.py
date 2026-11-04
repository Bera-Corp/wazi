"""Operator-controlled Linux qualification runner. Produces unsigned receipts only.

The suite image is trusted operator code; the candidate is JSON on stdin, never
a Docker command, mount, image choice, or executable supplied by a contributor.
"""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
import uuid

import validate


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def read_json(path, limit):
    raw = Path(path).read_bytes()
    if len(raw) > limit:
        raise ValueError("runner-input-exceeds-bound")
    return json.loads(raw)


def run(policy, manifest, output):
    if os.name != "posix" or not Path("/proc/sys/kernel").is_dir():
        raise ValueError("native-linux-runner-required")
    allowed = {"image", "tenantScope", "runnerId", "runnerVersion", "suiteDigest", "testIds", "cpuMilli", "ramMb", "storageMb", "timeoutSeconds", "endpoint"}
    if set(policy) != allowed:
        raise ValueError("closed-runner-policy-required")
    if not re.fullmatch(r"(?:[a-z0-9./_-]+@)?sha256:[0-9a-f]{64}", policy["image"]):
        raise ValueError("suite-image-must-be-immutable")
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", policy["suiteDigest"]):
        raise ValueError("suite-digest-required")
    for key, low, high in (("cpuMilli", 1, 2000), ("ramMb", 64, 4096), ("storageMb", 64, 10240), ("timeoutSeconds", 1, 1800)):
        if type(policy[key]) is not int or not low <= policy[key] <= high:
            raise ValueError("runner-limit-invalid:" + key)
    ids = policy["testIds"]
    if not isinstance(ids, list) or not 1 <= len(ids) <= 128 or len(set(ids)) != len(ids) or any(not isinstance(x, str) or not 1 <= len(x) <= 128 for x in ids):
        raise ValueError("suite-test-inventory-invalid")
    endpoint = policy["endpoint"]
    if not re.fullmatch(r"unix:///run/user/[1-9][0-9]*/docker.sock", endpoint):
        raise ValueError("rootless-endpoint-required")
    if manifest.get("reviewState") != "candidate" or "signature" in manifest:
        raise ValueError("unsigned-candidate-required")
    schemas = {p.name: validate.load(p) for p in (Path(validate.__file__).resolve().parent.parent / "schemas").glob("*.schema.json")}
    validator = validate.make_validator(schemas["app-manifest.schema.json"], validate.build_registry(schemas))
    errors = validate.schema_errors(validator, manifest)
    validate.check_secrets(manifest, "candidate", errors)
    validate.check_sensitive_env(manifest, "candidate", errors)
    if errors:
        raise ValueError("candidate-contract-or-secret-policy-rejected")
    docker = ["docker", "--host", endpoint]

    def command(*args):
        return subprocess.check_output(docker + list(args), timeout=30, stderr=subprocess.DEVNULL)

    info = json.loads(command("info", "--format", "{{json .SecurityOptions}}"))
    if not any("rootless" in entry for entry in info):
        raise ValueError("rootless-engine-required")
    image = json.loads(command("image", "inspect", policy["image"]))[0]
    if image.get("Config", {}).get("Volumes"):
        raise ValueError("suite-image-must-not-declare-volumes")
    directory = Path(output)
    directory.mkdir(mode=0o700, parents=False, exist_ok=False)
    os.chmod(directory, 0o700)
    name = "wazi-qual-" + uuid.uuid4().hex
    started = now()
    tests = [{"testId": name, "result": "failed"} for name in ids]
    status = "runner-failed"
    failure_reason = None
    process = None
    created = False
    try:
        # All writable container space is memory-backed and bounded. There are
        # no host mounts, Docker socket, devices, ports, secrets, or live data.
        created = True
        command("create", "--name", name, "--pull", "never", "--network", "none",
                "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
                "--user", "65534:65534", "--pids-limit", "128", "--memory", str(policy["ramMb"]) + "m",
                "--memory-swap", str(policy["ramMb"]) + "m", "--cpus", str(policy["cpuMilli"] / 1000),
                "--tmpfs", "/work:rw,noexec,nosuid,nodev,mode=1777,size=" + str(policy["storageMb"] * 1024 * 1024),
                "--workdir", "/work", "--log-driver", "none", "--interactive",
                "--entrypoint", "/wazi/qualify", policy["image"])
        inspected = json.loads(command("inspect", name))[0]
        host = inspected["HostConfig"]
        if host.get("Privileged") or host.get("Binds") or host.get("Devices") or host.get("NetworkMode") != "none" or not host.get("ReadonlyRootfs") or any(m.get("Type") != "tmpfs" or m.get("Destination") != "/work" for m in inspected.get("Mounts", [])):
            raise ValueError("runner-container-boundary-mismatch")
        with tempfile.TemporaryFile() as source, tempfile.TemporaryFile() as result, tempfile.TemporaryFile() as errors:
            source.write(json.dumps({"manifest": manifest, "candidateDigest": validate.canonical_digest(manifest)}).encode())
            source.seek(0)
            process = subprocess.Popen(docker + ["start", "--attach", "--interactive", name], stdin=source, stdout=result, stderr=errors)
            deadline = time.monotonic() + policy["timeoutSeconds"]
            while process.poll() is None:
                if time.monotonic() >= deadline or max(os.fstat(result.fileno()).st_size,os.fstat(errors.fileno()).st_size) > 65536:
                    process.kill()
                    process.wait()
                    raise ValueError("runner-time-or-output-limit")
                time.sleep(0.1)
            if os.fstat(result.fileno()).st_size > 65536:
                raise ValueError("runner-output-limit")
            if process.returncode:
                errors.seek(0)
                diagnostic=directory/'runner.stderr.private.txt'
                with diagnostic.open('xb') as output:
                    os.chmod(diagnostic,0o600)
                    output.write(errors.read(65536))
                if os.fstat(result.fileno()).st_size == 0:
                    raise ValueError('qualification-entrypoint-failed')
            result.seek(0)
            report = json.load(result)
            if set(report) != {"tests"} or not isinstance(report["tests"], list) or len(report["tests"]) != len(ids):
                raise ValueError("suite-output-invalid")
            by_id = {}
            for item in report["tests"]:
                if set(item) != {"testId", "result"} or item["testId"] in by_id or item["testId"] not in ids or item["result"] not in ("passed", "failed"):
                    raise ValueError("suite-results-invalid")
                by_id[item["testId"]] = item["result"]
            tests = [{"testId": key, "result": by_id[key]} for key in ids]
            state = json.loads(command("inspect", "--format", "{{json .State}}", name))
            if state.get("OOMKilled") or ((process.returncode or state.get("ExitCode") != 0) and all(item['result']=='passed' for item in tests)):
                tests = [{"testId": key, "result": "failed"} for key in ids]
            status = "completed"
    except Exception as exc:
        status = type(exc).__name__
        failure_reason = str(exc) if type(exc) is ValueError else type(exc).__name__
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait()
        if created:
            try:
                command("rm", "--force", name)
            except Exception:
                status = "cleanup-failed"
                tests = [{"testId": key, "result": "failed"} for key in ids]
    if status != "completed":
        tests = [{"testId": key, "result": "failed"} for key in ids]
    passed = status == "completed" and all(x["result"] == "passed" for x in tests)
    receipt = {"schemaVersion": "1.0.0", "qualificationId": "qual_" + uuid.uuid4().hex,
               "tenantScope": policy["tenantScope"], "candidateDigest": validate.canonical_digest(manifest),
               "result": "passed" if passed else "failed", "runnerId": policy["runnerId"],
               "runnerVersion": policy["runnerVersion"], "suiteDigest": policy["suiteDigest"],
               "startedAt": started, "completedAt": now(), "tests": tests,
               "isolation": {"syntheticProject": True, "network": "deny-egress", "liveVolumeAccess": False,
                             "privileged": False, **{k: policy[k] for k in ("cpuMilli", "ramMb", "storageMb")}}}
    for filename, value in (("qualification.unsigned.json", receipt), ("execution.json", {"state": status, "failureReason":failure_reason, "suiteImage": policy["image"], "policyDigest": validate.canonical_digest(policy), "candidateDigest": receipt["candidateDigest"], "completedAt": now()})):
        with (directory / filename).open("x", encoding="utf-8") as handle:
            os.chmod(handle.name, 0o600)
            json.dump(value, handle, sort_keys=True, indent=2)
    return passed


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for key in ("policy", "manifest", "output"):
        parser.add_argument("--" + key, required=True)
    args = parser.parse_args()
    passed = run(read_json(args.policy, 65536), read_json(args.manifest, 1 << 20), args.output)
    raise SystemExit(0 if passed else 1)


if __name__ == "__main__":
    main()
