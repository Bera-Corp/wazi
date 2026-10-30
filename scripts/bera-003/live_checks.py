#!/usr/bin/env python3
"""Authorized, loopback-only development checks. Never exports credentials."""
import argparse
import copy
import json
import os
from pathlib import Path
import pwd
import shutil
import ssl
import subprocess
import time
import urllib.error
import urllib.request

BASE = "https://127.0.0.1:9443"
CONFIG = Path("/etc/wazi-controlplane-dev")
SERVICE = "wazi-controlplane-dev.service"
AGENT = "/opt/wazi-dev/current/wazi-agent"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    if os.geteuid() != 0 or os.uname().nodename != "bera-003":
        raise SystemExit("Run as the development operator on bera-003 only.")
    user = pwd.getpwnam("wazi-dev")
    root = Path(user.pw_dir) / "wazi-live" / "checks" / args.run_id
    root.mkdir(parents=True, exist_ok=True)
    root.chmod(0o700)
    os.chown(root, user.pw_uid, user.pw_gid)
    report = {"run": args.run_id, "endpoint": BASE, "checks": [], "deployment": "disabled"}
    report_path = root / "results.json"
    token = (CONFIG / "operator.env").read_text().strip().split("=", 1)[1]
    tenant = "tenant_live_" + args.run_id.replace("-", "_")
    host = "host_bera-003"

    def record(name, passed, detail):
        report["checks"].append({"name": name, "passed": bool(passed), "detail": detail})
        report_path.write_text(json.dumps(report, indent=2) + "\n")
        os.chown(report_path, user.pw_uid, user.pw_gid)
        print(("PASS " if passed else "FAIL ") + name + ": " + str(detail), flush=True)
        if not passed:
            raise RuntimeError(name)

    def context(identity=None):
        ctx = ssl.create_default_context(cafile=str(CONFIG / "server-ca.crt"))
        if identity:
            ctx.load_cert_chain(str(identity / "node.crt"), str(identity / "node.key"))
        return ctx

    def request(path, method="GET", data=None, identity=None, operator=False, headers=None, raw=None):
        body = raw if raw is not None else (json.dumps(data).encode() if data is not None else None)
        h = {"Content-Type": "application/json"}
        if operator:
            h["X-Wazi-Operator-Token"] = token
        h.update(headers or {})
        req = urllib.request.Request(BASE + path, data=body, method=method, headers=h)
        try:
            with urllib.request.urlopen(req, context=context(identity), timeout=12) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.read()

    def mint(t=tenant, node=host):
        status, body = request("/v1/pairing/codes", "POST",
                               {"tenantScope": t, "hostId": node}, operator=True)
        if status != 201:
            raise RuntimeError("operator mint failed, HTTP " + str(status))
        return json.loads(body)["pairingCode"]

    def state(name):
        directory = root / name
        directory.mkdir(mode=0o700, exist_ok=True)
        os.chown(directory, user.pw_uid, user.pw_gid)
        shutil.copyfile(CONFIG / "server-ca.crt", directory / "server-ca.crt")
        os.chown(directory / "server-ca.crt", user.pw_uid, user.pw_gid)
        return directory

    def agent(identity, code=None, renew=False, t=tenant, node=host):
        cmd = ["runuser", "-u", "wazi-dev", "--", AGENT, "-tenant", t, "-host-id", node,
               "-controlplane", BASE, "-state-dir", str(identity),
               "-ca", str(identity / "server-ca.crt")]
        if code:
            cmd += ["-pairing-code", code]
        if renew:
            cmd += ["-renew"]
        result = subprocess.run(cmd, text=True, capture_output=True, timeout=40)
        return result.returncode, result.stderr.strip()

    status, _ = request("/healthz")
    record("health", status == 200, status)
    status, _ = request("/v1/host-profiles")
    record("missing-client-certificate", status == 401, status)
    status, _ = request("/v1/pairing/codes", "POST", {"tenantScope": tenant, "hostId": host})
    record("missing-operator-token", status == 401, status)
    status, _ = request("/v1/deployments", "POST", {})
    record("application-execution-disabled", status == 501, status)
    identity = state("primary")
    used = mint()
    spare = mint()
    rc, message = agent(identity, used)
    record("real-agent-enrollment-and-inventory-submission", rc == 0, message)
    status, body = request("/v1/host-profiles", identity=identity)
    profiles = json.loads(body)
    record("authenticated-listing", status == 200 and host in profiles, status)
    profile = profiles[host]
    (root / "host-profile.json").write_text(json.dumps(profile, indent=2) + "\n")
    os.chown(root / "host-profile.json", user.pw_uid, user.pw_gid)
    record("real-host-binding", profile["hostId"] == host and profile["tenantScope"] == tenant,
           {"hostId": host, "filesystem": profile["filesystem"], "docker": profile["docker"]})
    rc, message = agent(state("replay"), used)
    record("consumed-code-replay", rc != 0 and "already used" in message, "rejected without exposing code")
    changed = copy.deepcopy(profile)
    changed["tenantScope"] = "tenant_other"
    status, _ = request("/v1/host-profiles", "POST", changed, identity)
    record("cross-tenant-submission", status == 403, status)
    changed = copy.deepcopy(profile)
    changed["hostId"] = "host_other"
    status, _ = request("/v1/host-profiles", "POST", changed, identity)
    record("cross-node-submission", status == 403, status)
    changed = copy.deepcopy(profile)
    changed["Redaction"] = {"secretValuesPresent": None}
    status, _ = request("/v1/host-profiles", "POST", changed, identity)
    record("case-alias-rejected", status == 400, status)
    changed = copy.deepcopy(profile)
    del changed["resources"]["ramMb"]
    status, _ = request("/v1/host-profiles", "POST", changed, identity)
    record("missing-resource-rejected", status == 400, status)
    minimal = copy.deepcopy(profile)
    minimal.pop("occupiedPorts", None)
    minimal["network"] = {}
    status, _ = request("/v1/host-profiles", "POST", minimal, identity)
    status_get, body = request("/v1/host-profiles", identity=identity)
    listed = json.loads(body)[host]
    record("optional-field-roundtrip", status == 202 and status_get == 200 and
           listed.get("occupiedPorts", []) is not None and "mode" not in listed["network"], status)
    (root / "roundtrip-profile.json").write_text(json.dumps(listed, indent=2) + "\n")
    os.chown(root / "roundtrip-profile.json", user.pw_uid, user.pw_gid)
    before_renewal = state("before-renewal")
    for name in ("node.crt", "node.key"):
        shutil.copyfile(identity / name, before_renewal / name)
        (before_renewal / name).chmod(0o600)
    rc, message = agent(identity, renew=True)
    record("two-phase-renewal", rc == 0, message)
    status, _ = request("/v1/host-profiles", identity=before_renewal)
    record("old-certificate-retired", status == 401, status)
    status, _ = request("/v1/host-profiles", identity=identity)
    record("renewed-certificate-accepted", status == 200, status)
    other_tenant = tenant + "_other"
    other_identity = state("other-tenant")
    rc, message = agent(other_identity, mint(other_tenant, "host_other"), t=other_tenant, node="host_other")
    record("second-tenant-enrollment", rc == 0, message)
    status, body = request("/v1/host-profiles", identity=identity)
    record("tenant-scoped-listing", status == 200 and "host_other" not in json.loads(body), status)
    status, _ = request("/v1/pairing/revoke", "POST", {"tenantScope": tenant, "hostId": host}, operator=True)
    record("operator-revocation", status == 200, status)
    status, _ = request("/v1/host-profiles", identity=identity)
    record("revoked-certificate-rejected", status == 403, status)
    restart = subprocess.run(["systemctl", "restart", SERVICE], text=True, capture_output=True, timeout=25)
    time.sleep(1)
    active = subprocess.run(["systemctl", "is-active", SERVICE], text=True, capture_output=True)
    record("service-restart", restart.returncode == 0 and active.stdout.strip() == "active",
           {"restartExit": restart.returncode, "state": active.stdout.strip()})
    status, _ = request("/v1/host-profiles", identity=identity)
    record("revocation-survives-service-restart", status == 403, status)
    rc, message = agent(state("spare-after-revoke"), spare)
    record("spare-code-invalidated-after-restart", rc != 0 and "invalidated" in message, "rejected")
    rc, message = agent(identity, mint())
    record("fresh-code-repair", rc == 0, message)
    rc, message = agent(identity)
    record("existing-identity-reuse", rc == 0, message)
    status, _ = request("/v1/deployments", "POST", {}, identity)
    record("final-application-execution-disabled", status == 501, status)
    report["identityDirectory"] = str(identity)
    report["tenant"] = tenant
    report["profilePersistence"] = "observations re-submitted after restart; pairing registry persisted"
    report_path.write_text(json.dumps(report, indent=2) + "\n")
    os.chown(report_path, user.pw_uid, user.pw_gid)
    print("RESULTS=" + str(report_path), flush=True)


if __name__ == "__main__":
    main()
