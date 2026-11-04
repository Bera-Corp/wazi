import copy
import unittest
import platform_lifecycle as portable
from community.contracts import canonical
import validate


def fixture():
    observed={"family":"linux-engine","architecture":"amd64","desktopVersion":"0","guestVersion":"6.8","dockerVersion":"29.1.3","composeVersion":"2.40.3","enforcement":"verified"}
    packages={}
    for name in ("old","new"):
        manifest={"appId":"app.demo","version":name}
        procedures={op:{"procedureId":"proc_"+op,"version":"1.0.0","operation":op} for op in ("backup","upgrade","restore","health-check")}
        files={"manifest.json":canonical(manifest)}|{"procedure-"+op+".json":canonical(p) for op,p in procedures.items()}
        index={"applicationId":"app.demo","environments":[{"environment":observed,"operations":list(procedures),"qualificationDigest":"sha256:"+"c"*64}],"operationProcedures":{op:"procedure-"+op+".json" for op in procedures}}
        packages[name]=(index,files)
    digests={name:validate.canonical_digest({"appId":"app.demo","version":name}) for name in packages}
    def action(name,kind,operation,release,capabilities):
        proc={"procedureId":"proc_"+operation,"version":"1.0.0","operation":operation}
        return {"kind":kind,"job":{"jobId":"job_"+name,"approvals":[] if kind=="health" else [{"approvalId":"approval_fixture"}],
            "envelope":{"tenantScope":"tenant_a","hostProfileRef":{"recordId":"host_a"},"applicationId":"app.demo","manifestDigest":digests[release],
                        "procedureRef":{"recordId":proc["procedureId"],"version":"1.0.0","digest":validate.canonical_digest(proc)}},
            "plan":{"operation":operation,"steps":[{"capability":cap} for cap in capabilities]}}}
    target={"targetId":"canary","tenantScope":"tenant_a","hostId":"host_a","applicationId":"app.demo","fromManifestDigest":digests["old"],"toManifestDigest":digests["new"],
            "forward":[action("backup","checkpoint","backup","old",["volume.backup"]),action("upgrade","upgrade","upgrade","new",["compose.up"]),action("health","health","health-check","new",["manifest.verify","health.check"])],
            "recovery":[action("restore","restore","restore","old",["volume.restore"]),action("recovery_health","health","health-check","old",["manifest.verify","health.check"])]}
    plan={"rolloutId":"rollout_fixture","automaticRecovery":True,"waves":[{"holdSeconds":0,"targets":[target]}]}
    bindings={"schemaVersion":"1.0.0","targets":{"canary":{"environment":observed,"fromArchive":"old","toArchive":"new"}}}
    return plan,bindings,packages


class PortableTests(unittest.TestCase):
    def test_preparation_keeps_execution_off_and_all_platform_bindings(self):
        plan,bindings,packages=fixture();result=portable.prepare(plan,bindings,packages.__getitem__)
        self.assertFalse(result["executed"]);self.assertEqual(len(result["platformBindings"]),2)
    def test_guest_success_does_not_qualify_windows_or_macos(self):
        for family in ("windows-wsl2","macos-desktop"):
            plan,bindings,packages=fixture();bindings["targets"]["canary"]["environment"]=dict(bindings["targets"]["canary"]["environment"],family=family)
            with self.assertRaises(ValueError):portable.prepare(plan,bindings,packages.__getitem__)
    def test_substituted_procedure_or_missing_platform_binding_rejected(self):
        plan,bindings,packages=fixture()
        plan["waves"][0]["targets"][0]["forward"][1]["job"]["envelope"]["procedureRef"]["digest"]="sha256:"+"d"*64
        with self.assertRaises(ValueError):portable.prepare(plan,bindings,packages.__getitem__)
        plan,bindings,packages=fixture();bindings["targets"]={}
        with self.assertRaises(ValueError):portable.prepare(plan,bindings,packages.__getitem__)
    def test_timed_observation_cannot_contain_mutation(self):
        plan,bindings,packages=fixture()
        health=plan["waves"][0]["targets"][0]["forward"][-1];health["observeAfterSeconds"]=60
        health["job"]["plan"]["steps"].append({"capability":"compose.up"})
        with self.assertRaises(ValueError):portable.prepare(plan,bindings,packages.__getitem__)


if __name__=="__main__":unittest.main()
