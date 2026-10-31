"""Transactional, tenant-scoped canonical deployment cases and context receipts.

Only the private agent bridge imports live receipts. This store establishes
integrity and isolation, not the authenticity of caller-supplied evidence.
"""
import json
import os
import sqlite3
from pathlib import Path

import context_engine as engine
import validate


class ContextStore:
    def __init__(self, path):
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path, timeout=10)
        os.chmod(path, 0o600)
        self.db.execute("PRAGMA foreign_keys=ON")
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("PRAGMA synchronous=FULL")
        self.db.executescript("""
            CREATE TABLE IF NOT EXISTS cases (
                tenant TEXT NOT NULL, id TEXT NOT NULL, digest TEXT NOT NULL,
                document TEXT NOT NULL, PRIMARY KEY(tenant,id));
            CREATE TABLE IF NOT EXISTS evidence (
                tenant TEXT NOT NULL, id TEXT NOT NULL, case_id TEXT NOT NULL,
                digest TEXT NOT NULL, document TEXT NOT NULL,
                PRIMARY KEY(tenant,id),
                FOREIGN KEY(tenant,case_id) REFERENCES cases(tenant,id));
            CREATE TABLE IF NOT EXISTS contexts (
                tenant TEXT NOT NULL, id TEXT NOT NULL, document TEXT NOT NULL,
                PRIMARY KEY(tenant,id));
            CREATE TABLE IF NOT EXISTS case_bindings (
                tenant TEXT NOT NULL, id TEXT NOT NULL, procedure_digest TEXT NOT NULL,
                PRIMARY KEY(tenant,id),
                FOREIGN KEY(tenant,id) REFERENCES cases(tenant,id));
        """)

    def close(self):
        self.db.close()

    def import_case(self, case, evidence, procedure_ref=None):
        """Validate and atomically insert all records; exact replay is a no-op."""
        if engine._record_schema_errors("deployment-case.schema.json", case):
            raise ValueError("case-schema-invalid")
        for key in ("manifestDigest", "envelopeId", "planId"):
            if not case.get(key):
                raise ValueError("case-binding-required")
        start = validate.parse_time(case["startedAt"], "case", [])
        finish = validate.parse_time(case["finishedAt"], "case", [])
        if finish < start:
            raise ValueError("case-time-order")
        if engine.secret_findings(case, *evidence):
            raise ValueError("case-secret-pattern")
        refs = case["evidenceRefs"]
        if len(set(refs)) != len(refs) or sorted(refs) != sorted(e["evidenceId"] for e in evidence):
            raise ValueError("case-evidence-mismatch")
        for record in evidence:
            if engine._record_schema_errors("evidence-record.schema.json", record):
                raise ValueError("evidence-schema-invalid")
            if record["deploymentCaseId"] != case["caseId"]:
                raise ValueError("evidence-case-mismatch")
            if case["outcome"] == "success" and record["result"] != "pass":
                raise ValueError("success-evidence-not-passed")
            if record.get("commitOrReleaseDigest") != case["manifestDigest"]:
                raise ValueError("evidence-release-mismatch")
        tenant = case["tenantScope"]
        digest = validate.canonical_digest(case)
        with self.db:
            found = self.db.execute("SELECT digest FROM cases WHERE tenant=? AND id=?",
                                    (tenant, case["caseId"])).fetchone()
            if found and found[0] != digest:
                raise ValueError("case-conflict")
            self.db.execute("INSERT OR IGNORE INTO cases VALUES (?,?,?,?)",
                            (tenant, case["caseId"], digest, json.dumps(case, sort_keys=True)))
            for record in evidence:
                edigest = validate.canonical_digest(record)
                found_e = self.db.execute("SELECT digest FROM evidence WHERE tenant=? AND id=?",
                                          (tenant, record["evidenceId"])).fetchone()
                if found_e and found_e[0] != edigest:
                    raise ValueError("evidence-conflict")
                self.db.execute("INSERT OR IGNORE INTO evidence VALUES (?,?,?,?,?)",
                                (tenant, record["evidenceId"], case["caseId"], edigest,
                                 json.dumps(record, sort_keys=True)))
            if procedure_ref:
                binding_digest = validate.canonical_digest(procedure_ref)
                previous = self.db.execute("SELECT procedure_digest FROM case_bindings WHERE tenant=? AND id=?",
                                           (tenant, case["caseId"])).fetchone()
                if previous and previous[0] != binding_digest:
                    raise ValueError("case-procedure-conflict")
                self.db.execute("INSERT OR IGNORE INTO case_bindings VALUES (?,?,?)",
                                (tenant, case["caseId"], binding_digest))
        return found is None

    def cases(self, tenant, application, operation, procedure_ref=None):
        if not tenant or not application or not operation:
            raise ValueError("case-scope-required")
        if procedure_ref:
            rows = self.db.execute("SELECT c.document FROM cases c JOIN case_bindings b ON c.tenant=b.tenant AND c.id=b.id WHERE c.tenant=? AND b.procedure_digest=? ORDER BY c.id",
                                   (tenant, validate.canonical_digest(procedure_ref))).fetchall()
        else:
            rows = self.db.execute("SELECT document FROM cases WHERE tenant=? ORDER BY id",
                                   (tenant,)).fetchall()
        return [c for (raw,) in rows if (c := json.loads(raw))["applicationId"] == application
                and c["operation"] == operation]

    def save_context(self, tenant, record):
        if not tenant or engine.secret_findings(record):
            raise ValueError("context-scope-or-redaction")
        context_id = "ctx_" + validate.canonical_digest(record).split(":", 1)[1]
        with self.db:
            self.db.execute("INSERT OR IGNORE INTO contexts VALUES (?,?,?)",
                            (tenant, context_id, json.dumps(record, sort_keys=True)))
        return dict(record, contextId=context_id)

    def context(self, tenant, context_id):
        row = self.db.execute("SELECT document FROM contexts WHERE tenant=? AND id=?",
                              (tenant, context_id)).fetchone()
        return dict(json.loads(row[0]), contextId=context_id) if row else None
