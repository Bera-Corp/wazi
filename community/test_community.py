import copy
import hashlib
import http.client
import io
import json
import sqlite3
import stat
import tempfile
import threading
import time
import unittest
import zipfile
from pathlib import Path

from .cli import capture_cases
from .contracts import canonical, contribution, digest, strict_json
from .github import OAuth, refresh
from .outbox import Outbox
from .packages import configure, inspect_archive
from .platforms import compatibility
from .server import Application
from .store import Store


def observation():
    return {"family": "linux-engine", "architecture": "amd64", "desktopVersion": "0",
            "guestVersion": "26.04", "dockerVersion": "29.1.3", "composeVersion": "2.40.3", "enforcement": "verified"}


def report():
    return {"schemaVersion": "1.0.0", "applicationId": "org.nginx.nginx", "recipeDigest": digest("recipe"),
            "releaseVersion": "1.30.6", "environment": observation(), "operation": "install", "outcome": "success",
            "proofDigests": [digest("proof")], "templateDigest": digest("template"), "consentVersion": "1.0.0", "license": "Apache-2.0"}


def package(changes=None):
    release = Path(__file__).resolve().parent.parent / "releases/phase2/nginx"
    manifest = strict_json((release / "manifest.json").read_bytes())
    licenses = {"Apache-2.0": {"text": (Path(__file__).parent / "recipes/LICENSE").read_text(encoding="utf-8"), "notices": "Original test recipe fixture only."},
                "BSD-2-Clause": {"text": "Test fixture: this is not an upstream license attestation. " * 4, "notices": "Synthetic test data; not a published release."}}
    files = {"manifest.json": canonical(manifest), "procedure-install.json": canonical(strict_json((release / "procedure-install.json").read_bytes())),
             "licenses.json": canonical(licenses),
             "config-instance.json": canonical({"instance": {"waziInput": "instance"}})}
    index = {"schemaVersion": "1.0.0", "recipeId": "nginx", "applicationId": "org.nginx.nginx",
             "releaseVersion": manifest["version"], "revision": 1, "license": "Apache-2.0", "authors": ["test-contributor"],
             "upstreamSources": [{"repository": "nginx/nginx", "commit": "a" * 40, "license": "BSD-2-Clause"}],
             "inputs": {"instance": {"type": "instance", "required": True}},
             "files": {name: "sha256:" + hashlib.sha256(body).hexdigest() for name, body in files.items()},
             "environments": [{"environment": observation(), "operations": ["install"], "qualificationDigest": digest("qualification")}],
             "operations": ["install"], "operationProcedures": {"install": "procedure-install.json"}, "evidenceRefs": [digest("evidence")], "signature": {"fixture": "not-a-real-signature"}}
    if changes:
        changes(index, files)
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w") as archive:
        archive.writestr("recipe.json", canonical(index))
        for name, body in files.items():
            archive.writestr(name, body)
    return stream.getvalue()


def candidate_package(config=None):
    def unsigned(index, files):
        index.pop("signature")
        for name in ("manifest.json", "procedure-install.json"):
            document = strict_json(files[name])
            document.pop("signature")
            document["reviewState"] = "candidate"
            files[name] = canonical(document)
        if config is not None:
            files["config-instance.json"] = canonical(config)
        index["files"] = {name: "sha256:" + hashlib.sha256(raw).hexdigest() for name, raw in files.items()}
    return package(unsigned)


class CommunityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.store = Store(self.root / "catalog.sqlite")
        self.store.seed(strict_json((Path(__file__).parent / "seeds.json").read_bytes()))

    def tearDown(self):
        self.temp.cleanup()

    def test_consent_is_default_off_and_reports_remain_unqualified(self):
        with self.assertRaises(PermissionError):
            self.store.submit("1", report())
        self.store.set_consent("1", True)
        first = self.store.submit("1", report())
        self.assertEqual(first, self.store.submit("1", report()))
        failure = report() | {"outcome": "failure"}
        self.store.submit("1", failure)
        nginx = next(app for app in self.store.applications() if app["name"] == "Nginx")
        self.assertEqual(nginx["reportedOutcomes"], 2)
        self.assertEqual(nginx["recipes"], [])
        self.assertEqual(Store(self.root / "catalog.sqlite").reports("1")[0]["state"], "reported")

    def test_cross_identity_isolation_and_withdrawal(self):
        self.store.set_consent("1", True)
        receipt = self.store.submit("1", report())
        self.assertEqual(self.store.reports("2"), [])
        with self.assertRaises(LookupError):
            self.store.withdraw("2", receipt["id"])
        self.store.withdraw("1", receipt["id"])
        self.assertEqual(self.store.reports("1")[0]["body"], {})

    def test_closed_contract_rejects_private_data_null_aliases_and_duplicates(self):
        for extra in ("hostname", "rawLogs", "password", "tenantScope"):
            with self.subTest(extra=extra), self.assertRaises(ValueError):
                contribution(report() | {extra: "private"})
        with self.assertRaises(ValueError):
            strict_json('{"outcome":"success","outcome":"failure"}')
        bad = report()
        bad["environment"]["desktopVersion"] = "private-host"
        with self.assertRaises(ValueError):
            contribution(bad)

    def test_outbox_off_retry_restart_revoke(self):
        outbox = Outbox(self.root / "outbox.sqlite")
        self.assertIsNone(outbox.capture(report()))
        outbox.consent(True)
        fingerprint = outbox.capture(report())
        self.assertEqual(fingerprint, outbox.capture(report()))
        def unavailable(_):
            raise OSError("offline")
        outbox.flush(unavailable, now=100)
        restarted = Outbox(self.root / "outbox.sqlite")
        self.assertEqual(restarted.status()["items"][0]["state"], "retry")
        restarted.consent(False)
        self.assertEqual(restarted.status()["items"][0]["state"], "cancelled")
        self.assertEqual(restarted.flush(lambda _: self.fail("send after consent revoked")), [])

    def test_outbox_requires_bound_receipt_and_bounded_retries(self):
        outbox = Outbox(self.root / "outbox.sqlite")
        outbox.consent(True)
        outbox.capture(report())
        for attempt in range(5):
            outbox.flush(lambda _: {"digest": digest("different"), "state": "reported"}, now=100000 * (attempt + 1))
        self.assertEqual(outbox.status()["items"][0]["state"], "failed")

    def test_automatic_case_capture_uses_actual_schema_without_private_fields(self):
        db = sqlite3.connect(self.root / "cases.sqlite")
        db.executescript("CREATE TABLE cases(tenant TEXT,id TEXT,digest TEXT,document TEXT); CREATE TABLE evidence(tenant TEXT,id TEXT,case_id TEXT,digest TEXT);")
        future = "2099-01-01T00:00:00Z"
        case = {"applicationId": "org.nginx.nginx", "releaseVersion": "1.30.6", "manifestDigest": digest("manifest"),
                "caseId": "case_private", "tenantScope": "private-tenant", "hostId": "private-host", "finishedAt": future,
                "operation": "install", "outcome": "success"}
        db.execute("INSERT INTO cases VALUES(?,?,?,?)", ("private-tenant", "case_private", digest(case), canonical(case).decode()))
        db.execute("INSERT INTO evidence VALUES(?,?,?,?)", ("private-tenant", "ev_private", "case_private", digest("proof")))
        db.commit()
        db.close()
        mapping = {case["applicationId"]: {"releaseVersion": case["releaseVersion"], "manifestDigest": case["manifestDigest"],
                   "tenantScope": "private-tenant", "recipeDigest": digest("recipe"), "templateDigest": digest("template"), "environment": observation()}}
        outbox = Outbox(self.root / "local.sqlite")
        outbox.consent(True)
        self.assertEqual(capture_cases(outbox, self.root / "cases.sqlite", mapping), 1)
        sent = []
        outbox.flush(lambda value: sent.append(value) or {"digest": digest(value), "state": "reported"})
        self.assertEqual(len(sent), 1)
        self.assertNotIn("private", canonical(sent).decode())

    def test_metadata_conditional_request_and_outage_preserve_observation(self):
        def transport(url, **kwargs):
            repository = url.split("/repos/")[1]
            return 200, {"ETag": "test-etag"}, {"full_name": repository, "stargazers_count": 42, "license": {"spdx_id": "MIT"}}
        refresh(self.store, now=1000, transport=transport)
        def unchanged(url, **kwargs):
            if url.endswith("/releases/latest"):
                self.assertNotIn("If-None-Match", kwargs["headers"])
                return 404, {}, {}
            self.assertEqual(kwargs["headers"]["If-None-Match"], "test-etag")
            return 304, {}, {}
        refresh(self.store, now=90000, transport=unchanged)
        self.assertFalse(self.store.applications(90001)[0]["metadataStale"])
        refresh(self.store, now=180000, transport=lambda *a, **k: (429, {}, {}))
        item = self.store.applications(180001)[0]
        self.assertEqual(item["github"]["stars"], 42)
        self.assertTrue(item["metadataStale"])
        self.assertEqual(item["metadataObservedAt"], 90000)
        self.assertEqual(refresh(self.store, now=180002, transport=lambda *a, **k: self.fail("request during GitHub cooldown")), 0)

    def test_oauth_browser_binding_pkce_and_state_replay(self):
        calls = []
        def transport(url, **kwargs):
            calls.append((url, kwargs))
            if "access_token" in url:
                self.assertIn(b"code_verifier=", kwargs["data"])
                fake = {"scope": ""}
                fake["access_token"] = "short" + "-fixture"
                return 200, {}, fake
            return 200, {}, {"id": 7, "login": "contributor"}
        oauth = OAuth(self.store, "fixture-client", "short-fixture", "https://localhost:9444", transport)
        state, destination = oauth.begin()
        self.assertIn("code_challenge_method=S256", destination)
        with self.assertRaises(PermissionError):
            oauth.finish(state, "wrong-browser", "code")
        session = oauth.finish(state, state, "code")
        self.assertEqual(self.store.authenticate(session)["owner"], "7")
        with self.assertRaises(PermissionError):
            oauth.finish(state, state, "code")
        self.assertEqual(len(calls), 2)

    def test_device_identity_cannot_become_browser_session(self):
        device = self.store.start_device()
        self.assertEqual(self.store.poll_device(device["deviceCode"])["state"], "pending")
        self.store.approve_device("1", device["userCode"])
        result = self.store.poll_device(device["deviceCode"])
        self.assertEqual(self.store.agent(result["credential"]), "1")
        self.assertIsNone(self.store.authenticate(result["credential"]))
        with self.assertRaises(PermissionError):
            self.store.poll_device(device["deviceCode"])
        self.store.revoke_agents("1")
        self.assertIsNone(self.store.agent(result["credential"]))

    def test_package_requires_signature_and_detects_tampering(self):
        with self.assertRaises(ValueError):
            inspect_archive(package())
        index, files = inspect_archive(package(), verifier=lambda _: None)
        self.assertEqual(configure(index, files, {"instance": "test-instance"})["config-instance.json"], b'{"instance":"test-instance"}')
        with self.assertRaises(ValueError):
            inspect_archive(package(lambda index, files: files.update({"manifest.json": b'{}'})), verifier=lambda _: None)
        with self.assertRaises(ValueError):
            configure(index, files, {"instance": "../../escape"})

    def test_archive_rejects_traversal_and_extra_entries(self):
        with self.assertRaises(ValueError):
            inspect_archive(package(lambda index, files: files.update({"../escape.json": b'{}'})), verifier=lambda _: None)
        with self.assertRaises(ValueError):
            inspect_archive(package(lambda index, files: files.update({"script.json": b'{}'})), verifier=lambda _: None)

    def test_guest_success_does_not_qualify_desktop_or_other_version(self):
        index, _ = inspect_archive(package(), verifier=lambda _: None)
        self.assertTrue(compatibility(index, observation(), "install")["eligible"])
        for family in ("windows-wsl2", "macos-desktop"):
            self.assertFalse(compatibility(index, observation() | {"family": family}, "install")["eligible"])
        self.assertFalse(compatibility(index, observation() | {"dockerVersion": "30.0.0"}, "install")["eligible"])
        self.assertFalse(compatibility(index, observation(), "restore")["eligible"])
        self.assertFalse(compatibility(index, observation() | {"enforcement": "unavailable"}, "install")["eligible"])

    def test_real_templates_are_quarantined_deduplicated_and_not_promoted(self):
        raw = candidate_package()
        with self.assertRaises(PermissionError):
            self.store.submit_candidate("1", raw)
        self.store.set_consent("1", True)
        result = self.store.submit_candidate("1", raw)
        self.assertEqual(result, self.store.submit_candidate("1", raw))
        self.store.submit_candidate("1", candidate_package({"instance": "another-template"}))
        self.assertEqual(len(self.store.candidates("1")), 2)
        self.assertEqual(self.store.candidates("2"), [])
        self.assertEqual(next(app for app in self.store.applications() if app["name"] == "Nginx")["recipes"], [])
        with self.assertRaises(LookupError):
            self.store.withdraw_candidate("2", result["id"])
        self.store.withdraw_candidate("1", result["id"])
        self.assertEqual(len(self.store.candidates("1")), 1)

    def test_candidate_privacy_rejects_values_paths_and_signed_bypass(self):
        for config in ({"password": "short"}, {"directory": "/home/private-owner/data"}, {"directory": "C:\\Users\\private-owner\\data"}):
            with self.subTest(config=config), self.assertRaises(ValueError):
                inspect_archive(candidate_package(config), candidate=True)
        with self.assertRaises(ValueError):
            inspect_archive(package(), candidate=True)

    def test_signed_recipe_cannot_widen_environment_or_operation_claims(self):
        def unverified(index, files):
            index["environments"][0]["environment"]["enforcement"] = "unverified"
        with self.assertRaises(ValueError):
            inspect_archive(package(unverified), verifier=lambda _: None)
        def operation(index, files):
            index["environments"][0]["operations"].append("restore")
        with self.assertRaises(ValueError):
            inspect_archive(package(operation), verifier=lambda _: None)

    def test_automatic_candidate_outbox_and_remote_consent_generation(self):
        outbox = Outbox(self.root / "outbox.sqlite")
        outbox.consent(True)
        outbox.sync_remote({"enabled": True, "changed": 1})
        raw = candidate_package()
        fingerprint = outbox.capture_candidate(raw)
        self.assertEqual(fingerprint, outbox.capture_candidate(raw))
        outbox.sync_remote({"enabled": True, "changed": 2})
        self.assertEqual(outbox.status()["consent"]["enabled"], 0)
        self.assertEqual(outbox.status()["items"][0]["state"], "cancelled")

    def test_agent_contribution_cannot_cross_consent_revision(self):
        self.store.set_consent("1", True)
        old = self.store.consent("1")["changed"]
        self.store.set_consent("1", False)
        self.store.set_consent("1", True)
        with self.assertRaises(PermissionError):
            self.store.submit("1", report(), old)

    def test_contract_inventory_matches_public_report_shape(self):
        from jsonschema import Draft202012Validator
        from .schema import contracts
        validator = Draft202012Validator(contracts())
        validator.validate(report())
        self.assertFalse(validator.is_valid(report() | {"rawLogs": []}))

    def test_desktop_probe_binds_host_guest_and_refuses_remote_daemon(self):
        from .desktop_probe import observe
        def command(executable, *args):
            if args == ("context", "show"):
                return "desktop-linux"
            if args[:2] == ("context", "inspect"):
                return json.dumps([{"Endpoints": {"docker": {"Host": "npipe:////./pipe/dockerDesktopLinuxEngine"}}}])
            if args[0] == "info":
                return json.dumps({"OSType": "linux", "Architecture": "aarch64", "ServerVersion": "29.1.3", "KernelVersion": "6.10.0-linuxkit"})
            return "2.40.3"
        result = observe("windows-wsl2", invoke=command, system="Windows", desktop_version="11.0")
        self.assertEqual(result["environment"]["architecture"], "arm64")
        self.assertEqual(result["environment"]["enforcement"], "unverified")
        with self.assertRaises(ValueError):
            observe("macos-desktop", invoke=command, system="Windows", desktop_version="11.0")
        def remote(executable, *args):
            if args[:2] == ("context", "inspect"):
                return json.dumps([{"Endpoints": {"docker": {"Host": "ssh://another-machine"}}}])
            return command(executable, *args)
        with self.assertRaises(ValueError):
            observe("windows-wsl2", invoke=remote, system="Windows", desktop_version="11.0")


class HTTPTests(unittest.TestCase):
    def setUp(self):
        CommunityTests.setUp(self)
        from http.server import ThreadingHTTPServer
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Application(self.store, "https://catalog.test", self.root / "objects").handler())
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        CommunityTests.tearDown(self)

    def request(self, path, body=None, headers=None):
        conn = http.client.HTTPConnection("127.0.0.1", self.server.server_port, timeout=5)
        data = canonical(body) if body is not None else None
        conn.request("POST" if body is not None else "GET", path, data,
                     {"Host": "catalog.test", "Content-Type": "application/json", **(headers or {})})
        response = conn.getresponse()
        result = response.status, dict(response.headers), response.read()
        conn.close()
        return result

    def test_public_root_and_api_work_without_host_execution(self):
        status, headers, body = self.request("/")
        self.assertEqual(status, 200)
        self.assertIn(b"Application library", body)
        self.assertIn("frame-ancestors 'none'", headers["Content-Security-Policy"])
        status, _, body = self.request("/api/applications")
        self.assertEqual(len(json.loads(body)["applications"]), 10)
        self.assertEqual(self.request("/api/deploy", {})[0], 404)
        self.assertEqual(self.request("/api/contributions")[0], 403)

    def test_session_csrf_and_origin_are_required(self):
        session = self.store.session("1", "contributor")
        csrf = self.store.authenticate(session)["csrf"]
        headers = {"Cookie": "__Host-wazi-community=" + session}
        body = {"enabled": True, "version": "1.0.0", "licenseAccepted": True}
        self.assertEqual(self.request("/api/consent", body, headers)[0], 403)
        headers |= {"Origin": "https://catalog.test", "X-CSRF-Token": csrf}
        self.assertEqual(self.request("/api/consent", body, headers)[0], 200)
        self.assertEqual(self.request("/api/contributions", report(), headers)[0], 201)
        self.assertEqual(self.request("/api/consent", body, headers | {"Origin": "https://evil.test"})[0], 403)

    def test_worker_is_contribution_only_and_consent_bound(self):
        self.store.set_consent("1", True)
        revision = self.store.consent("1")["changed"]
        pairing = self.store.start_device()
        self.store.approve_device("1", pairing["userCode"])
        identity = self.store.poll_device(pairing["deviceCode"])
        headers = {"Authorization": "Bearer " + identity["credential"], "X-Wazi-Consent-Revision": str(revision)}
        self.assertEqual(self.request("/api/agent/consent", headers=headers)[0], 200)
        self.assertEqual(self.request("/api/agent/contributions", report(), headers)[0], 201)
        self.assertEqual(self.request("/api/consent", {"enabled": True}, headers)[0], 403)
        self.store.set_consent("1", False)
        self.assertEqual(self.request("/api/agent/contributions", report(), headers)[0], 403)
        self.store.revoke_agents("1")
        self.assertEqual(self.request("/api/agent/consent", headers=headers)[0], 403)


if __name__ == "__main__":
    unittest.main()
