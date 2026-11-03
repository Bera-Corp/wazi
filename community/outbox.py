"""Local opt-in outbox. This database is private and is never the public catalog."""
import json
import base64
import hashlib
import os
from contextlib import contextmanager
import sqlite3
import time
import urllib.parse
from pathlib import Path

from .contracts import VERSION, canonical, contribution, digest
from .packages import inspect_archive


class Outbox:
    def __init__(self, path):
        self.path = str(path)
        target = Path(path).absolute()
        target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        if target.is_symlink():
            raise ValueError("outbox database cannot be a symlink")
        with self.connect() as db:
            db.executescript("""
            CREATE TABLE IF NOT EXISTS consent(singleton INTEGER PRIMARY KEY CHECK(singleton=1),
              enabled INTEGER NOT NULL, version TEXT NOT NULL, changed REAL NOT NULL);
            INSERT OR IGNORE INTO consent VALUES(1,0,'1.0.0',0);
            CREATE TABLE IF NOT EXISTS outbox(digest TEXT PRIMARY KEY,body TEXT NOT NULL,
              state TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt REAL NOT NULL DEFAULT 0,
              receipt TEXT);
            CREATE TABLE IF NOT EXISTS remote_consent(singleton INTEGER PRIMARY KEY CHECK(singleton=1),changed REAL NOT NULL);
            """)
        os.chmod(self.path, 0o600)

    @contextmanager
    def connect(self):
        db = sqlite3.connect(self.path, timeout=10)
        db.row_factory = sqlite3.Row
        db.execute("PRAGMA journal_mode=WAL")
        db.execute("PRAGMA synchronous=FULL")
        try:
            with db:
                yield db
        finally:
            db.close()

    def consent(self, enabled):
        if type(enabled) is not bool:
            raise ValueError("explicit Boolean required")
        with self.connect() as db:
            db.execute("UPDATE consent SET enabled=?,version=?,changed=? WHERE singleton=1", (enabled, VERSION, time.time()))
            if not enabled:
                db.execute("UPDATE outbox SET state='cancelled',body='{}' WHERE state IN ('pending','retry')")

    def capture(self, terminal):
        # Only a closed public record is accepted. Callers must map private receipts
        # to this contract; passing a raw journal always fails closed.
        contribution(terminal)
        fingerprint = digest(terminal)
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            if not db.execute("SELECT enabled FROM consent WHERE singleton=1").fetchone()[0]:
                return None
            if db.execute("SELECT count(*) FROM outbox WHERE state IN ('pending','retry')").fetchone()[0] >= 1000:
                raise ValueError("outbox capacity exceeded")
            db.execute("INSERT OR IGNORE INTO outbox(digest,body,state) VALUES(?,?,'pending')", (fingerprint, canonical(terminal).decode()))
        return fingerprint

    def sync_remote(self, consent):
        with self.connect() as db:
            prior = db.execute("SELECT changed FROM remote_consent WHERE singleton=1").fetchone()
            if not consent["enabled"] or (prior and prior[0] != consent["changed"]):
                db.execute("UPDATE consent SET enabled=0,changed=? WHERE singleton=1", (time.time(),))
                db.execute("UPDATE outbox SET state='cancelled',body='{}' WHERE state IN ('pending','retry')")
            db.execute("INSERT INTO remote_consent VALUES(1,?) ON CONFLICT(singleton) DO UPDATE SET changed=excluded.changed", (consent["changed"] or 0,))

    def capture_candidate(self, raw):
        inspect_archive(raw, candidate=True)
        fingerprint = "sha256:" + hashlib.sha256(raw).hexdigest()
        body = {"kind": "recipe-candidate", "archive": base64.b64encode(raw).decode("ascii")}
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            if not db.execute("SELECT enabled FROM consent WHERE singleton=1").fetchone()[0]:
                return None
            if db.execute("SELECT coalesce(sum(length(body)),0) FROM outbox").fetchone()[0] + len(canonical(body)) > 64 << 20:
                raise ValueError("outbox capacity exceeded")
            db.execute("INSERT OR IGNORE INTO outbox(digest,body,state) VALUES(?,?,'pending')", (fingerprint, canonical(body).decode()))
        return fingerprint

    def flush(self, send, now=None):
        now = time.time() if now is None else now
        results = []
        # Hold the same write transaction as consent-off while sending. When
        # consent-off returns, no queued send can still start under old consent.
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            if not db.execute("SELECT enabled FROM consent WHERE singleton=1").fetchone()[0]:
                return []
            rows = db.execute("SELECT * FROM outbox WHERE state IN ('pending','retry') AND next_attempt<=? ORDER BY rowid LIMIT 1", (now,)).fetchall()
            for row in rows:
                body = json.loads(row["body"])
                if body.get("kind") == "recipe-candidate":
                    inspect_archive(base64.b64decode(body["archive"], validate=True), candidate=True)
                else:
                    contribution(body)
                attempts = row["attempts"] + 1
                try:
                    receipt = send(body)
                    if receipt.get("digest") != row["digest"] or receipt.get("state") not in {"reported", "withdrawn"}:
                        raise ValueError("unbound contribution receipt")
                    state = "submitted" if receipt["state"] == "reported" else "cancelled"
                    db.execute("UPDATE outbox SET state=?,attempts=?,receipt=? WHERE digest=?", (state, attempts, canonical(receipt).decode(), row["digest"]))
                    results.append(row["digest"])
                except (OSError, ValueError, PermissionError):
                    state = "failed" if attempts >= 5 else "retry"
                    db.execute("UPDATE outbox SET state=?,attempts=?,next_attempt=? WHERE digest=?", (state, attempts, now + min(3600, 60 * 2 ** attempts), row["digest"]))
        return results

    def status(self):
        with self.connect() as db:
            return {"consent": dict(db.execute("SELECT enabled,version,changed FROM consent").fetchone()),
                    "items": [dict(row) for row in db.execute("SELECT digest,state,attempts,next_attempt,receipt FROM outbox ORDER BY rowid DESC LIMIT 1000")]}
