import copy
import unittest

from . import handoff
from .contracts import strict_json
from .packages import inspect_archive
from .test_community import package,observation
from scripts import validate


class HandoffTests(unittest.TestCase):
    def setUp(self):
        self.raw=package();self.calls=[];index,files=inspect_archive(self.raw,lambda raw:None)
        manifest=strict_json(files["manifest.json"]);procedure=strict_json(files["procedure-install.json"])
        self.policy={"tenantScope":"tenant_a","hostId":"host_a"}
        self.catalog=[{"project":"wazi-nginx-01","applicationId":index["applicationId"],"releaseVersion":index["releaseVersion"],"operations":["install"]}]
        self.proposal={"context":{"decision":"accept","executionApproved":False},"job":{"approvals":[],"envelope":{"tenantScope":"tenant_a","hostProfileRef":{"recordId":"host_a"},"applicationId":index["applicationId"],
            "manifestDigest":validate.canonical_digest(manifest),"operation":"install","procedureRef":{"recordId":procedure["procedureId"],"version":procedure["version"],"digest":validate.canonical_digest(procedure)}},
            "plan":{"operation":"install"},"artifact":{"projectName":"wazi-nginx-01"}}}
    def request(self,method,route,body=None):
        self.calls.append((method,route))
        return self.catalog if method=="GET" else self.proposal
    def prepare(self):
        return handoff.prepare(self.raw,lambda raw:None,observation(),"wazi-nginx-01",self.policy,self.request)
    def test_prepares_but_never_deploys_or_approves(self):
        self.assertFalse(self.prepare()["executionApproved"])
        self.assertEqual(self.calls,[("GET","/v1/context-catalog"),("POST","/v1/context-proposals")])
    def test_proposal_cannot_substitute_tenant_release_or_procedure(self):
        for field in ("tenantScope","manifestDigest","procedureRef"):
            with self.subTest(field=field):
                original=copy.deepcopy(self.proposal)
                self.proposal["job"]["envelope"][field]="wrong"
                with self.assertRaises(ValueError):self.prepare()
                self.proposal=original
    def test_unenabled_instance_and_unqualified_platform_block(self):
        self.catalog=[]
        with self.assertRaises(ValueError):self.prepare()
        changed=observation();changed["family"]="macos-desktop"
        self.calls=[]
        with self.assertRaises(ValueError):handoff.prepare(self.raw,lambda raw:None,changed,"wazi-nginx-01",self.policy,self.request)
        self.assertEqual(self.calls,[])
    def test_untrusted_bundle_is_rejected_before_private_api(self):
        def reject(raw):raise ValueError("untrusted")
        with self.assertRaises(ValueError):handoff.prepare(self.raw,reject,observation(),"wazi-nginx-01",self.policy,self.request)
        self.assertEqual(self.calls,[])


if __name__=="__main__":unittest.main()
