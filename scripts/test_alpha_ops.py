import copy
import json
from pathlib import Path
import tempfile
import unittest

import alpha_ops as alpha


def observed():
    return {"family":"linux-engine","architecture":"amd64","desktopVersion":"0","guestVersion":"6.8","dockerVersion":"29.1.3","composeVersion":"2.40.3","enforcement":"verified"}


def event(name,kind="deployment",outcome="success",app="app.0",target="vps"):
    return {"schemaVersion":"1.0.0","eventId":name,"trialId":name,"tenantScope":"tenant_a","applicationId":app,"releaseDigest":"sha256:"+"a"*64,
            "environment":observed(),"target":target,"kind":kind,"outcome":outcome,"attempt":1,"deterministic":True,"interventions":0,"elapsedMs":1000,"evidenceDigests":["sha256:"+"b"*64]}


class AlphaTests(unittest.TestCase):
    def setUp(self):
        self.folder=tempfile.TemporaryDirectory();self.root=Path(self.folder.name);self.ledger=alpha.Ledger(self.root/"alpha.sqlite")
        self.apps=["app."+str(i) for i in range(5)];self.ledger.curate("tenant_a",self.apps)
    def tearDown(self):
        self.ledger.close();self.folder.cleanup()
    def test_empty_dataset_never_passes(self):
        report=self.ledger.report("tenant_a")
        self.assertFalse(report["alphaThresholdsMet"])
        self.assertTrue(all(m["state"]=="not-measured" for m in report["metrics"].values()))
    def test_retry_does_not_rewrite_first_attempt_success(self):
        first=event("first",outcome="failure");self.ledger.record(first)
        retry=dict(first,eventId="retry",attempt=2,outcome="success");self.ledger.record(retry)
        report=self.ledger.report("tenant_a",1)
        self.assertEqual(report["metrics"]["firstAttemptSuccess"]["numerator"],0)
        self.assertEqual(report["metrics"]["firstAttemptSuccess"]["denominator"],1)
        with self.assertRaises(ValueError):self.ledger.record(dict(retry,eventId="third",attempt=3))
    def test_replay_and_tenant_isolation(self):
        value=event("same");self.assertTrue(self.ledger.record(value));self.assertFalse(self.ledger.record(value))
        with self.assertRaises(ValueError):self.ledger.record(dict(value,elapsedMs=2))
        self.assertEqual(self.ledger.report("tenant_other")["observations"],0)
    def test_later_clear_audit_cannot_hide_secret_exposure(self):
        self.ledger.record(event("audit_bad","safety","secret-exposure"))
        self.ledger.record(event("audit_clear","safety","clear"))
        self.assertEqual(self.ledger.report("tenant_a",1)["metrics"]["secret-exposure"]["state"],"fail")
    def test_two_percent_false_compatible_is_failure(self):
        for i in range(50):self.ledger.record(event("decision_"+str(i),"compatibility","false-compatible" if i==0 else "compatible"))
        self.assertEqual(self.ledger.report("tenant_a",1)["metrics"]["falseCompatible"]["state"],"fail")
    def test_five_app_gate_requires_same_release_and_environment_recovery(self):
        for app in self.apps:
            for target in ("local","vps"):
                for kind in ("deployment","restore"):self.ledger.record(event(app+target+kind,kind,"success",app,target))
        for kind,outcome in (("rollback","success"),("planner","valid"),("compatibility","compatible"),("safety","clear")):
            self.ledger.record(event(kind,kind,outcome))
        report=self.ledger.report("tenant_a",1)
        self.assertEqual(report["appsWithSameReleaseLocalAndVpsRecovery"],self.apps)
        self.assertTrue(report["alphaThresholdsMet"]) # Synthetic fixture only.
        self.assertFalse(self.ledger.report("tenant_a",20)["alphaThresholdsMet"])
    def test_changed_environment_restore_does_not_close_coverage(self):
        for target in ("local","vps"):
            self.ledger.record(event(target+"deploy",target=target))
            value=event(target+"restore","restore",target=target);value["environment"]["guestVersion"]="6.9";self.ledger.record(value)
        self.assertEqual(self.ledger.report("tenant_a",1)["appsWithSameReleaseLocalAndVpsRecovery"],[])
    def test_support_state_machine_and_explicit_redacted_feedback(self):
        ticket={"schemaVersion":"1.0.0","ticketId":"ticket_one","tenantScope":"tenant_a","applicationId":"app.0","releaseDigest":"sha256:"+"a"*64,
                "environment":observed(),"category":"restore","severity":"P1","evidenceDigests":["sha256:"+"b"*64]}
        self.ledger.ticket(ticket)
        change={"eventId":"transition_one","tenantScope":"tenant_a","ticketId":"ticket_one","from":"open","to":"resolved","evidenceDigests":ticket["evidenceDigests"]}
        with self.assertRaises(ValueError):self.ledger.transition(change)
        change["to"]="triaged";self.assertTrue(self.ledger.transition(change));self.assertFalse(self.ledger.transition(change))
        with self.assertRaises(ValueError):self.ledger.feedback("tenant_a","ticket_one")
        exported=self.ledger.feedback("tenant_a","ticket_one",True)
        self.assertNotIn("tenantScope",exported);self.assertNotIn("ticketId",exported);self.assertFalse(exported["sent"])
        with self.assertRaises(ValueError):self.ledger.ticket(dict(ticket,notes="unbounded private log"))
    def test_receipt_import_requires_matching_identity_and_no_pending_intent(self):
        queue=self.root/"queue";(queue/"requests").mkdir(parents=True);(queue/"results").mkdir()
        job={"jobId":"job_one","caseId":"case_one","plan":{"operation":"install"},"envelope":{"tenantScope":"tenant_a","applicationId":"app.0","manifestDigest":"sha256:"+"a"*64,"hostProfileRef":{"recordId":"host_one"},"envelopeId":"env_one"}}
        identity={"jobId":"job_one","caseId":"case_one","tenantScope":"tenant_a","applicationId":"app.0","manifestDigest":job["envelope"]["manifestDigest"],"operation":"install","hostId":"host_one","envelopeId":"env_one",
                  "planDigest":alpha.validate.canonical_digest(job["plan"]),"envelopeDigest":alpha.validate.canonical_digest(job["envelope"])}
        result={"state":"completed","operation":{"Operation":{"identity":identity,"state":"completed","createdAt":"2026-11-04T00:00:00Z","updatedAt":"2026-11-04T00:00:01Z"}}}
        for part,body in (("requests",job),("results",result)):(queue/part/"job_one.json").write_text(json.dumps(body),encoding="utf-8")
        mapping={"schemaVersion":"1.0.0","jobs":{"job_one":{"trialId":"trial_one","attempt":1,"deterministic":True,"interventions":0}}}
        self.assertEqual(alpha.import_queue(self.ledger,queue,"tenant_a",observed(),"vps",mapping)["imported"],1)
        result["operation"]["RecoveryRequired"]=True;(queue/"results/job_one.json").write_text(json.dumps(result),encoding="utf-8")
        with self.assertRaises(ValueError):alpha.import_queue(self.ledger,queue,"tenant_a",observed(),"vps",mapping)


if __name__=="__main__":unittest.main()
