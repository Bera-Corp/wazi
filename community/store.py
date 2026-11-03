"""Transactional public catalog. It deliberately has no private case-store connection."""
import hashlib
from contextlib import contextmanager
import json
import os
import secrets
import sqlite3
import time
from pathlib import Path

from .contracts import canonical, contribution, digest, project
from .pilot import evidence


class Store:
    def __init__(self, path):
        self.path = str(path)
        target = Path(path).absolute()
        target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        if target.is_symlink():
            raise ValueError("catalog database cannot be a symlink")
        with self.connect() as db:
            db.executescript("""
            CREATE TABLE IF NOT EXISTS applications(id TEXT PRIMARY KEY, body TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS metadata(id TEXT PRIMARY KEY, body TEXT NOT NULL,
              etag TEXT, checked REAL NOT NULL, next_check REAL NOT NULL, failures INTEGER NOT NULL);
            CREATE TABLE IF NOT EXISTS upstream(name TEXT PRIMARY KEY,next_check REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS reports(id TEXT PRIMARY KEY, owner TEXT NOT NULL,
              digest TEXT NOT NULL, body TEXT NOT NULL, created REAL NOT NULL, withdrawn INTEGER NOT NULL DEFAULT 0,
              UNIQUE(owner,digest));
            CREATE TABLE IF NOT EXISTS consent(owner TEXT PRIMARY KEY, enabled INTEGER NOT NULL,
              version TEXT NOT NULL, changed REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY, owner TEXT NOT NULL,
              login TEXT NOT NULL, csrf TEXT NOT NULL, expires REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS oauth(hash TEXT PRIMARY KEY, verifier TEXT NOT NULL,
              expires REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS recipes(id TEXT PRIMARY KEY, application TEXT NOT NULL,
              body TEXT NOT NULL, artifact TEXT NOT NULL, revoked INTEGER NOT NULL DEFAULT 0);
            CREATE TABLE IF NOT EXISTS removals(id TEXT PRIMARY KEY, owner TEXT NOT NULL,
              created REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS devices(hash TEXT PRIMARY KEY,code TEXT UNIQUE NOT NULL,
              owner TEXT,expires REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS agents(hash TEXT PRIMARY KEY,owner TEXT NOT NULL,expires REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS candidates(id TEXT PRIMARY KEY,owner TEXT NOT NULL,
              digest TEXT NOT NULL,body TEXT NOT NULL,archive BLOB NOT NULL,created REAL NOT NULL,
              UNIQUE(owner,digest));
            """)
            if "withdrawn" not in {row[1] for row in db.execute("PRAGMA table_info(candidates)")}:
                db.execute("ALTER TABLE candidates ADD COLUMN withdrawn INTEGER NOT NULL DEFAULT 0")
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

    def seed(self, applications):
        with self.connect() as db:
            for app in applications:
                project(app)
                db.execute("INSERT INTO applications VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body",
                           (app["applicationId"], canonical(app).decode()))

    def applications(self, now=None):
        now = time.time() if now is None else now
        with self.connect() as db:
            result = []
            for row in db.execute("SELECT id,body FROM applications ORDER BY id"):
                app = json.loads(row["body"])
                app["pilotEvidence"] = evidence(app["applicationId"])
                cached = db.execute("SELECT * FROM metadata WHERE id=?", (row["id"],)).fetchone()
                app["github"] = json.loads(cached["body"]) if cached else None
                if app["github"] is not None:
                    app["github"]["releaseStale"] = now - app["github"].get("releaseObservedAt", 0) > 86400
                app["metadataObservedAt"] = cached["checked"] if cached else None
                app["metadataStale"] = not cached or now - cached["checked"] > 86400
                app["recipes"] = [json.loads(x[0]) | {"revoked": bool(x[1])}
                                  for x in db.execute("SELECT body,revoked FROM recipes WHERE application=?", (row["id"],))]
                app["reportedOutcomes"] = db.execute("SELECT count(*) FROM reports WHERE withdrawn=0 AND json_extract(body,'$.applicationId')=?", (row["id"],)).fetchone()[0]
                result.append(app)
            return result

    def set_consent(self, owner, enabled, version="1.0.0"):
        if type(enabled) is not bool or version != "1.0.0":
            raise ValueError("current explicit consent required")
        with self.connect() as db:
            db.execute("INSERT INTO consent VALUES(?,?,?,?) ON CONFLICT(owner) DO UPDATE SET enabled=excluded.enabled,version=excluded.version,changed=excluded.changed",
                       (owner, enabled, version, time.time()))

    def consent(self, owner):
        with self.connect() as db:
            row = db.execute("SELECT enabled,version,changed FROM consent WHERE owner=?", (owner,)).fetchone()
            return {"enabled": bool(row["enabled"]), "version": row["version"], "changed": row["changed"]} if row else {"enabled": False, "version": "1.0.0", "changed": None}

    @staticmethod
    def authorize_agent(db, owner, credential):
        # This check and the insertion share the write transaction with revocation.
        if credential is not None and not db.execute(
                "SELECT 1 FROM agents WHERE hash=? AND owner=? AND expires>?",
                (Store.key(credential), owner, time.time())).fetchone():
            raise PermissionError("contribution credential revoked or expired")

    def submit(self, owner, body, consent_change=None, *, agent_credential=None):
        contribution(body)
        encoded, fingerprint = canonical(body).decode(), digest(body)
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            self.authorize_agent(db, owner, agent_credential)
            consent = db.execute("SELECT enabled,version,changed FROM consent WHERE owner=?", (owner,)).fetchone()
            if not consent or not consent[0] or consent[1] != body["consentVersion"]:
                raise PermissionError("contribution consent disabled")
            if consent_change is not None and consent_change != consent[2]:
                raise PermissionError("contribution consent changed")
            if not db.execute("SELECT 1 FROM applications WHERE id=?", (body["applicationId"],)).fetchone():
                raise ValueError("unknown application")
            prior = db.execute("SELECT id,withdrawn FROM reports WHERE owner=? AND digest=?", (owner, fingerprint)).fetchone()
            if prior:
                return {"id": prior[0], "digest": fingerprint, "state": "withdrawn" if prior[1] else "reported"}
            if db.execute("SELECT count(*) FROM reports WHERE owner=?", (owner,)).fetchone()[0] >= 1000:
                raise ValueError("contribution capacity exceeded")
            if db.execute("SELECT count(*) FROM reports").fetchone()[0] >= 5000:
                raise ValueError("pilot catalog capacity exceeded")
            report_id = "report_" + secrets.token_hex(16)
            db.execute("INSERT INTO reports(id,owner,digest,body,created) VALUES(?,?,?,?,?)",
                       (report_id, owner, fingerprint, encoded, time.time()))
            return {"id": report_id, "digest": fingerprint, "state": "reported"}

    def reports(self, owner):
        with self.connect() as db:
            return [{"id": row["id"], "digest": row["digest"], "body": json.loads(row["body"]),
                     "state": "withdrawn" if row["withdrawn"] else "reported", "created": row["created"]}
                    for row in db.execute("SELECT * FROM reports WHERE owner=? ORDER BY created DESC LIMIT 1000", (owner,))]

    def withdraw(self, owner, report_id):
        with self.connect() as db:
            cursor = db.execute("UPDATE reports SET withdrawn=1,body='{}' WHERE owner=? AND id=?", (owner, report_id))
            if cursor.rowcount != 1:
                raise LookupError("report not found")
            db.execute("INSERT OR IGNORE INTO removals VALUES(?,?,?)", (report_id, owner, time.time()))

    @staticmethod
    def key(raw):
        return hashlib.sha256(raw.encode()).hexdigest()

    def session(self, owner, login, now=None):
        now = time.time() if now is None else now
        raw, csrf = secrets.token_urlsafe(32), secrets.token_urlsafe(32)
        with self.connect() as db:
            db.execute("DELETE FROM sessions WHERE expires<?", (now,))
            db.execute("INSERT INTO sessions VALUES(?,?,?,?,?)", (self.key(raw), owner, login, csrf, now + 3600))
        return raw

    def authenticate(self, raw, now=None):
        with self.connect() as db:
            row = db.execute("SELECT owner,login,csrf,expires FROM sessions WHERE hash=? AND expires>?",
                             (self.key(raw), time.time() if now is None else now)).fetchone()
            return dict(row) if row else None

    def logout(self, raw):
        with self.connect() as db:
            db.execute("DELETE FROM sessions WHERE hash=?", (self.key(raw),))

    def start_device(self):
        raw, code = secrets.token_urlsafe(32), secrets.token_hex(4).upper()
        with self.connect() as db:
            db.execute("DELETE FROM devices WHERE expires<?", (time.time(),))
            if db.execute("SELECT count(*) FROM devices").fetchone()[0] >= 30:
                raise ValueError("device enrollment capacity exceeded")
            db.execute("INSERT INTO devices VALUES(?,?,NULL,?)", (self.key(raw), code, time.time() + 300))
        return {"deviceCode": raw, "userCode": code, "expiresIn": 300, "interval": 5}

    def approve_device(self, owner, code):
        with self.connect() as db:
            if db.execute("UPDATE devices SET owner=? WHERE code=? AND owner IS NULL AND expires>?",
                          (owner, code, time.time())).rowcount != 1:
                raise LookupError("device unavailable")

    def poll_device(self, raw):
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            row = db.execute("SELECT owner FROM devices WHERE hash=? AND expires>?", (self.key(raw), time.time())).fetchone()
            if not row:
                raise PermissionError("device expired")
            if row[0] is None:
                return {"state": "pending"}
            db.execute("DELETE FROM devices WHERE hash=?", (self.key(raw),))
            credential = secrets.token_urlsafe(32)
            db.execute("INSERT INTO agents VALUES(?,?,?)", (self.key(credential), row[0], time.time() + 30 * 86400))
            return {"state": "authorized", "credential": credential, "expiresIn": 30 * 86400}

    def agent(self, raw):
        with self.connect() as db:
            row = db.execute("SELECT owner FROM agents WHERE hash=? AND expires>?", (self.key(raw), time.time())).fetchone()
            return row[0] if row else None

    def revoke_agents(self, owner):
        with self.connect() as db:
            db.execute("DELETE FROM agents WHERE owner=?", (owner,))

    def submit_candidate(self, owner, raw, consent_change=None, *, agent_credential=None):
        from .packages import inspect_archive
        index, _ = inspect_archive(raw, candidate=True)
        fingerprint = "sha256:" + hashlib.sha256(raw).hexdigest()
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            self.authorize_agent(db, owner, agent_credential)
            consent = db.execute("SELECT enabled,changed FROM consent WHERE owner=?", (owner,)).fetchone()
            if not consent or not consent[0]:
                raise PermissionError("contribution consent disabled")
            if consent_change is not None and consent_change != consent[1]:
                raise PermissionError("contribution consent changed")
            if not db.execute("SELECT 1 FROM applications WHERE id=?", (index["applicationId"],)).fetchone():
                raise ValueError("application must be curated")
            prior = db.execute("SELECT id,withdrawn FROM candidates WHERE owner=? AND digest=?", (owner, fingerprint)).fetchone()
            if prior:
                return {"id": prior[0], "digest": fingerprint, "state": "withdrawn" if prior[1] else "reported"}
            if db.execute("SELECT count(*) FROM candidates WHERE owner=?", (owner,)).fetchone()[0] >= 100 or db.execute("SELECT coalesce(sum(length(archive)),0) FROM candidates").fetchone()[0] + len(raw) > 64 << 20:
                raise ValueError("candidate quarantine capacity exceeded")
            candidate_id = "candidate_" + secrets.token_hex(16)
            db.execute("INSERT INTO candidates(id,owner,digest,body,archive,created) VALUES(?,?,?,?,?,?)", (candidate_id, owner, fingerprint, canonical(index).decode(), raw, time.time()))
            return {"id": candidate_id, "digest": fingerprint, "state": "reported"}

    def candidates(self, owner):
        with self.connect() as db:
            return [{"id": row[0], "digest": row[1], "index": json.loads(row[2]), "state": "candidate; independent qualification required"}
                    for row in db.execute("SELECT id,digest,body FROM candidates WHERE owner=? AND withdrawn=0 ORDER BY created DESC", (owner,))]

    def withdraw_candidate(self, owner, candidate_id):
        with self.connect() as db:
            if db.execute("UPDATE candidates SET withdrawn=1,body='{}',archive=x'' WHERE id=? AND owner=?", (candidate_id, owner)).rowcount != 1:
                raise LookupError("candidate not found")
