"""Disposable negative tests for privacy, revocation and bounded intake."""
import hashlib
import base64
import http.client
import io
import socket
import sqlite3
import threading
import time
import unittest
import zipfile
from contextlib import closing

from .contracts import canonical, strict_json
from .github import OAuth
from .outbox import Outbox
from .packages import inspect_archive
from .server import Application, BoundedServer
from .store import Store
from . import test_community as fixtures
from .test_community import candidate_package, report


def rewrite(raw, edit):
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        files = {name: archive.read(name) for name in archive.namelist()}
    index = strict_json(files.pop("recipe.json"))
    edit(index, files)
    index["files"] = {name: "sha256:" + hashlib.sha256(body).hexdigest() for name, body in files.items()}
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w") as archive:
        archive.writestr("recipe.json", canonical(index))
        for name, body in files.items():
            archive.writestr(name, body)
    return stream.getvalue()


class IntakeTests:
    def test_candidate_unknown_roles_and_metadata_paths_are_withheld(self):
        outbox = Outbox(self.root / "outbox.sqlite")
        outbox.consent(True)
        for name in ("notes.json", "backup.json", "config-notes.json", "procedure-notes.json"):
            def edit(index, files):
                files[name] = canonical({"password": "short", "directory": "/home/private-owner/data"})
            with self.subTest(name=name), self.assertRaises(ValueError):
                outbox.capture_candidate(rewrite(candidate_package(), edit))
        def private_notice(index, files):
            licenses = strict_json(files["licenses.json"])
            licenses["Apache-2.0"]["notices"] = "/home/private-owner/data"
            files["licenses.json"] = canonical(licenses)
        with self.assertRaises(ValueError):
            outbox.capture_candidate(rewrite(candidate_package(), private_notice))
        self.assertEqual(outbox.status()["items"], [])

    def test_missing_index_and_bad_record_types_are_validation_errors(self):
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, "w") as archive:
            archive.writestr("manifest.json", "{}")
            archive.writestr("licenses.json", "{}")
        with self.assertRaises(ValueError):
            inspect_archive(stream.getvalue(), candidate=True)
        def wrong_files(index, files):
            index["environments"][0]["environment"]["family"] = []
        with self.assertRaises(ValueError):
            inspect_archive(rewrite(candidate_package(), wrong_files), candidate=True)

    def test_invalid_legacy_queue_item_is_withheld_and_safe_items_continue(self):
        outbox = Outbox(self.root / "outbox.sqlite")
        outbox.consent(True)
        legacy = {"kind": "recipe-candidate", "archive": base64.b64encode(b"invalid archive").decode()}
        with outbox.connect() as db:
            db.execute("INSERT INTO outbox(digest,body,state) VALUES('legacy',?,'pending')", (canonical(legacy).decode(),))
        sent = []
        self.assertEqual(outbox.flush(lambda value: sent.append(value)), [])
        self.assertEqual(sent, [])
        self.assertEqual(outbox.status()["items"][0]["state"], "failed")
        outbox.capture(report())
        from .contracts import digest
        outbox.flush(lambda value: sent.append(value) or {"digest": digest(value), "state": "reported"})
        self.assertEqual(sent, [report()])

    def test_withdrawal_tombstone_survives_restart_and_worker_retry(self):
        self.store.set_consent("1", True)
        raw = candidate_package()
        first = self.store.submit_candidate("1", raw)
        self.store.withdraw_candidate("1", first["id"])
        restarted = Store(self.root / "catalog.sqlite")
        receipt = restarted.submit_candidate("1", raw)
        self.assertEqual(receipt, first | {"state": "withdrawn"})
        self.assertEqual(restarted.candidates("1"), [])
        with restarted.connect() as db:
            row = db.execute("SELECT body,archive FROM candidates WHERE id=?", (first["id"],)).fetchone()
            self.assertEqual(tuple(row), ("{}", b""))
        outbox = Outbox(self.root / "retry.sqlite")
        outbox.consent(True)
        outbox.capture_candidate(raw)
        outbox.flush(lambda _: restarted.submit_candidate("1", raw))
        self.assertEqual(outbox.status()["items"][0]["state"], "cancelled")

    def test_existing_candidate_database_migrates_without_losing_rows(self):
        path = self.root / "legacy.sqlite"
        with closing(sqlite3.connect(path)) as db, db:
            db.execute("CREATE TABLE candidates(id TEXT PRIMARY KEY,owner TEXT NOT NULL,digest TEXT NOT NULL,body TEXT NOT NULL,archive BLOB NOT NULL,created REAL NOT NULL,UNIQUE(owner,digest))")
            db.execute("INSERT INTO candidates VALUES('existing','1','digest','{}',x'00',1)")
        migrated = Store(path)
        self.assertEqual(migrated.candidates("1")[0]["id"], "existing")
        migrated.withdraw_candidate("1", "existing")
        self.assertEqual(Store(path).candidates("1"), [])

    def test_revoked_or_expired_agent_cannot_commit_parked_upload(self):
        entered = threading.Event()
        original = self.store.agent
        def observed(raw):
            owner = original(raw)
            if owner is not None:
                entered.set()
            return owner
        self.store.agent = observed
        server = BoundedServer(("127.0.0.1", 0), Application(self.store, "https://catalog.test", self.root / "objects").handler())
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        self.store.set_consent("1", True)
        try:
            for change in ("revocation", "expiry"):
                for path, body, mime in (("/api/agent/contributions", canonical(report()), "application/json"),
                                         ("/api/agent/recipe-candidates", candidate_package(), "application/zip")):
                    with self.subTest(change=change, path=path):
                        device = self.store.start_device()
                        self.store.approve_device("1", device["userCode"])
                        credential = self.store.poll_device(device["deviceCode"])["credential"]
                        entered.clear()
                        with socket.create_connection(server.server_address, timeout=3) as connection:
                            header = ("POST " + path + " HTTP/1.1\r\nHost: catalog.test\r\nAuthorization: Bearer " + credential +
                                      "\r\nX-Wazi-Consent-Revision: " + str(self.store.consent("1")["changed"]) +
                                      "\r\nContent-Type: " + mime + "\r\nContent-Length: " + str(len(body)) + "\r\nConnection: close\r\n\r\n")
                            connection.sendall(header.encode() + body[:1])
                            self.assertTrue(entered.wait(2), "request never passed fail-fast authentication")
                            if change == "revocation":
                                self.store.revoke_agents("1")
                            else:
                                with self.store.connect() as db:
                                    db.execute("UPDATE agents SET expires=0")
                            connection.sendall(body[1:])
                            response = http.client.HTTPResponse(connection)
                            response.begin()
                            self.assertEqual(response.status, 403)
                            response.read()
            self.assertEqual(self.store.reports("1"), [])
            self.assertEqual(self.store.candidates("1"), [])
        finally:
            server.shutdown()
            server.server_close()
            worker.join()

    def test_total_deadline_releases_worker_during_header_and_body_trickles(self):
        base = Application(self.store, "https://catalog.test", self.root / "objects").handler()
        class ShortDeadline(base):
            request_timeout = .2
            def setup(self):
                super().setup()
                self.connection.settimeout(.1)
        server = BoundedServer(("127.0.0.1", 0), ShortDeadline)
        server.slots = threading.BoundedSemaphore(1)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            for prefix in (b"POST /api/device/start HTTP/1.1\r\nHost: catalog.test\r\nX-Slow: ",
                           b"POST /api/device/start HTTP/1.1\r\nHost: catalog.test\r\nContent-Type: application/json\r\nContent-Length: 10000\r\n\r\n"):
                with self.subTest(prefix=prefix):
                    with socket.create_connection(server.server_address, timeout=2) as connection:
                        connection.sendall(prefix)
                        start = time.monotonic()
                        while time.monotonic() - start < 1:
                            try:
                                connection.sendall(b" ")
                            except OSError:
                                break
                            time.sleep(.02)
                        self.assertLess(time.monotonic() - start, 1)
                    conn = http.client.HTTPConnection(*server.server_address, timeout=2)
                    try:
                        conn.request("GET", "/healthz", headers={"Host": "catalog.test"})
                        response = conn.getresponse()
                        self.assertEqual(response.status, 200)
                        response.read()
                    finally:
                        conn.close()
            with self.store.connect() as db:
                self.assertEqual(db.execute("SELECT count(*) FROM devices").fetchone()[0], 0)
        finally:
            server.shutdown()
            server.server_close()
            worker.join()

    def test_oauth_pending_state_is_bounded_without_network_calls(self):
        oauth = OAuth(self.store, "fixture", "short-fixture", "https://catalog.test",
                      transport=lambda *args, **kwargs: self.fail("network call"))
        for _ in range(256):
            oauth.begin()
        with self.assertRaises(ValueError):
            oauth.begin()
        with self.store.connect() as db:
            db.execute("UPDATE oauth SET expires=0")
        self.assertTrue(oauth.begin()[0])


class AdversarialIntakeTests(IntakeTests, unittest.TestCase):
    setUp = fixtures.CommunityTests.setUp
    tearDown = fixtures.CommunityTests.tearDown
