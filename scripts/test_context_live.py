"""Adversarial tests for the private live-context worker and durable store."""
import copy
import json
import tempfile
import unittest
from pathlib import Path

import context_engine as engine
import context_worker as worker
import validate
from context_store import ContextStore


class LiveContextTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name) / "context.sqlite"
        self.store = ContextStore(self.path)
        self.addCleanup(self.temp.cleanup)
        self.addCleanup(lambda: self.store.close())
        self.graph = engine.load_example_graph()

    def records(self, tenant="tenant_demo", case_id="case_test", operation="install"):
        digest = validate.canonical_digest(self.graph["manifest"])
        case = {"schemaVersion": "1.0.0", "caseId": case_id,
                "tenantScope": tenant, "applicationId": self.graph["manifest"]["appId"],
                "releaseVersion": "1.0.0", "hostId": "host_demo01", "operation": operation,
                "outcome": "success", "manifestDigest": digest,
                "envelopeId": "env_demo01", "planId": "plan_demo01",
                "startedAt": "2026-10-21T09:00:00Z", "finishedAt": "2026-10-21T09:01:00Z",
                "evidenceRefs": ["ev_test"],
                "redaction": {"applied": True, "secretValuesPresent": False, "rawLogsIncluded": False}}
        evidence = {"schemaVersion": "1.0.0", "evidenceId": "ev_test", "deploymentCaseId": case_id,
                    "type": "deployment-test", "result": "pass",
                    "artifact": {"digest": "sha256:" + "a" * 64},
                    "tool": {"name": "test", "version": "1.0.0"},
                    "commitOrReleaseDigest": digest, "createdAt": case["finishedAt"],
                    "redaction": {"applied": True, "secretValuesPresent": False}}
        return case, evidence

    def test_restart_and_exact_replay(self):
        case, evidence = self.records()
        self.assertTrue(self.store.import_case(case, [evidence]))
        self.assertFalse(self.store.import_case(case, [evidence]))
        self.store.close()
        self.store = ContextStore(self.path)
        self.assertEqual(self.store.cases("tenant_demo", case["applicationId"], "install"), [case])

    def test_case_conflict(self):
        case, evidence = self.records()
        self.store.import_case(case, [evidence])
        changed = dict(case, hostId="host_other")
        with self.assertRaisesRegex(ValueError, "case-conflict"):
            self.store.import_case(changed, [evidence])

    def test_evidence_conflict_rolls_back_new_case(self):
        case, evidence = self.records()
        self.store.import_case(case, [evidence])
        other, conflicting = self.records(case_id="case_other")
        with self.assertRaisesRegex(ValueError, "evidence-conflict"):
            self.store.import_case(other, [conflicting])
        self.assertEqual(len(self.store.cases("tenant_demo", case["applicationId"], "install")), 1)

    def test_same_case_id_can_exist_in_two_tenants(self):
        for tenant in ("tenant_demo", "tenant_other"):
            case, evidence = self.records(tenant=tenant)
            self.assertTrue(self.store.import_case(case, [evidence]))

    def test_operation_application_and_tenant_isolation(self):
        case, evidence = self.records()
        self.store.import_case(case, [evidence])
        for tenant, application, operation in (("tenant_other", case["applicationId"], "install"),
                                               ("tenant_demo", "org.other.app", "install"),
                                               ("tenant_demo", case["applicationId"], "restore")):
            self.assertEqual(self.store.cases(tenant, application, operation), [])
        with self.assertRaisesRegex(ValueError, "scope-required"):
            self.store.cases("", case["applicationId"], "install")

    def test_bad_evidence_and_time_rejected(self):
        case, evidence = self.records()
        for changed_case, changed_evidence in (
            (case, dict(evidence, result="fail")),
            (case, dict(evidence, deploymentCaseId="case_other")),
            (case, dict(evidence, commitOrReleaseDigest="sha256:" + "b" * 64)),
            (dict(case, evidenceRefs=[]), evidence),
            (dict(case, finishedAt="2026-10-20T09:00:00Z"), evidence),
            (dict(case, manifestDigest=None), evidence),
        ):
            with self.assertRaises(ValueError):
                self.store.import_case(changed_case, [changed_evidence])

    def test_secret_pattern_rejected(self):
        case, evidence = self.records()
        case["failureSignatureId"] = "password=" + "synthetic" + "-credential-value"
        with self.assertRaisesRegex(ValueError, "secret-pattern"):
            self.store.import_case(case, [evidence])

    def payload(self):
        procedure = next(p for p in self.graph["procedures"] if p["operation"] == "install")
        return {"action": "retrieve", "evaluatedAt": engine.EVAL_TIME,
                "snapshot": {"envelope": self.graph["envelope"], "manifest": self.graph["manifest"],
                             "procedure": procedure, "host": self.graph["host"]}}

    def test_retrieval_case_score_and_durable_inspection(self):
        empty = worker.retrieve(self.store, self.payload())
        case, evidence = self.records()
        self.store.import_case(case, [evidence], procedure_ref=self.graph["envelope"]["procedureRef"])
        result = worker.retrieve(self.store, self.payload())
        self.assertEqual(result["selected"]["score"], empty["selected"]["score"] + 2)
        self.assertFalse(result["executionApproved"])
        self.assertEqual(result["successCaseRefs"][0]["digest"], validate.canonical_digest(case))
        self.assertIsNone(self.store.context("tenant_other", result["contextId"]))
        self.assertEqual(self.store.context("tenant_demo", result["contextId"]), result)

    def test_aborted_does_not_raise_success_score(self):
        case, evidence = self.records()
        case["outcome"] = "aborted"
        evidence["result"] = "inconclusive"
        self.store.import_case(case, [evidence])
        self.assertEqual(worker.retrieve(self.store, self.payload())["successCaseRefs"], [])

    def test_other_procedure_cannot_claim_success(self):
        case, evidence = self.records()
        ref = dict(self.graph["envelope"]["procedureRef"], digest="sha256:" + "b" * 64)
        self.store.import_case(case, [evidence], procedure_ref=ref)
        self.assertEqual(worker.retrieve(self.store, self.payload())["successCaseRefs"], [])
        with self.assertRaisesRegex(ValueError, "procedure-conflict"):
            self.store.import_case(case, [evidence], procedure_ref=self.graph["envelope"]["procedureRef"])

    def test_verified_search_includes_rejected_alternatives(self):
        accepted = worker.retrieve(self.store, self.payload())
        result = worker.dispatch(self.store, {"action": "search", "accepted": [accepted],
                                 "rejected": [{"candidateIndex": 1, "decision": "reject", "reason": "signed-release-or-revocation-rejected"}]})
        self.assertEqual(result["selected"], accepted["selected"])
        self.assertEqual(len(result["ranked"]), 2)
        self.assertEqual(self.store.context("tenant_demo", result["contextId"]), result)
        denied = worker.dispatch(self.store, {"action": "search", "accepted": [], "rejected": result["ranked"][1:]})
        self.assertEqual(denied["decision"], "reject")
        self.assertFalse(denied["executionApproved"])

    def receipt_payload(self):
        envelope, plan = self.graph["envelope"], self.graph["plan"]
        identity = {"jobId": "job_test", "caseId": "case_test", "planId": plan["planId"],
                    "planDigest": validate.canonical_digest(plan), "envelopeId": envelope["envelopeId"],
                    "envelopeDigest": validate.canonical_digest(envelope),
                    "manifestDigest": envelope["manifestDigest"], "tenantScope": envelope["tenantScope"],
                    "applicationId": envelope["applicationId"], "hostId": envelope["hostProfileRef"]["recordId"],
                    "operation": envelope["operation"]}
        operation = {"identity": identity, "state": "completed", "createdAt": "2026-10-21T09:00:00Z",
                     "updatedAt": "2026-10-21T09:01:00Z", "events": [{"kind": "completed"}]}
        return {"receipt": {"Operation": operation, "RecoveryRequired": False, "PendingIntent": None},
                "job": {"jobId": "job_test", "caseId": "case_test", "envelope": envelope, "plan": plan}}

    def test_receipt_binding_and_terminal_requirements(self):
        payload = self.receipt_payload()
        self.assertTrue(worker.record_terminal(self.store, payload)["imported"])
        payload["receipt"]["Operation"]["identity"]["manifestDigest"] = "sha256:" + "0" * 64
        with self.assertRaisesRegex(ValueError, "binding-mismatch"):
            worker.record_terminal(self.store, payload)

    def test_null_and_unresolved_receipts_not_success(self):
        self.assertEqual(worker.record_terminal(self.store, {"receipt": {}, "job": {}})["classification"], "preflight-denial")
        payload = self.receipt_payload()
        payload["receipt"]["Operation"]["state"] = "reconciliation-required"
        self.assertFalse(worker.record_terminal(self.store, payload)["imported"])


if __name__ == "__main__":
    unittest.main()
