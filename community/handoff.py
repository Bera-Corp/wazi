"""Verify a catalog bundle and prepare a private, unapproved installation job."""
import argparse
import json
import os
from pathlib import Path
import ssl
import urllib.parse
import urllib.request

from .contracts import canonical, closed, environment, sha, strict_json
from .packages import MAX_ARCHIVE, inspect_archive, recordtool_verifier
from .platforms import compatibility
from scripts import validate


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("private-api-redirect-forbidden")


def client(policy):
    closed(policy, "origin caFile certificateFile keyFile credentialEnvironment tenantScope hostId")
    parsed=urllib.parse.urlsplit(policy["origin"])
    if parsed.scheme!="https" or not parsed.hostname or parsed.username or parsed.password or parsed.path not in ("","/") or parsed.query or parsed.fragment:
        raise ValueError("fixed-private-https-origin-required")
    context=ssl.create_default_context(cafile=policy["caFile"])
    context.load_cert_chain(policy["certificateFile"],policy["keyFile"])
    opener=urllib.request.build_opener(NoRedirect,urllib.request.HTTPSHandler(context=context))
    def request(method,route,body=None):
        if (method,route) not in (("GET","/v1/context-catalog"),("POST","/v1/context-proposals")):
            raise ValueError("handoff-route-forbidden")
        req=urllib.request.Request(policy["origin"].rstrip("/")+route,None if body is None else validate.canonical_bytes(body),method=method)
        req.add_header("Content-Type","application/json")
        req.add_header("X-Wazi-Operator-Token",os.environ[policy["credentialEnvironment"]])
        with opener.open(req,timeout=30) as reply:
            raw=reply.read((4<<20)+1)
            if len(raw)>4<<20:raise ValueError("private-api-response-bound")
            return strict_json(raw)
    return request


def prepare(raw,verifier,observed,project,policy,request,operation="install",backup_id=""):
    environment(observed)
    index,files=inspect_archive(raw,verifier)
    if not compatibility(index,observed,operation)["eligible"]:
        raise ValueError("exact-signed-platform-coverage-required")
    if operation not in ("install","upgrade","backup","restore","uninstall"):
        raise ValueError("unsupported-handoff-operation")
    catalog=request("GET","/v1/context-catalog")
    if not isinstance(catalog,list) or len(catalog)>8:raise ValueError("bounded-private-instance-catalog-required")
    entries=[item for item in catalog if item.get("project")==project and item.get("applicationId")==index["applicationId"] and item.get("releaseVersion")==index["releaseVersion"]]
    if len(entries)!=1 or operation not in entries[0]["operations"]:
        raise ValueError("package-not-enabled-in-local-operator-policy")
    manifest=strict_json(files["manifest.json"])
    procedure=strict_json(files[index["operationProcedures"][operation]])
    proposal=request("POST","/v1/context-proposals",{"project":project,"operation":operation,"backupId":backup_id})
    job=proposal["job"];envelope=job["envelope"];ref=envelope["procedureRef"]
    if proposal["context"]["decision"]!="accept" or proposal["context"]["executionApproved"] is not False or job["approvals"]:
        raise ValueError("unapproved-accepted-context-required")
    if envelope["tenantScope"]!=policy["tenantScope"] or envelope["hostProfileRef"]["recordId"]!=policy["hostId"]:
        raise ValueError("private-target-binding-mismatch")
    if envelope["applicationId"]!=index["applicationId"] or envelope["manifestDigest"]!=validate.canonical_digest(manifest) or envelope["operation"]!=operation or job["plan"]["operation"]!=operation or job["artifact"]["projectName"]!=project:
        raise ValueError("proposal-package-binding-mismatch")
    if ref!={"recordId":procedure["procedureId"],"version":procedure["version"],"digest":validate.canonical_digest(procedure)}:
        raise ValueError("proposal-procedure-binding-mismatch")
    return {"schemaVersion":"1.0.0","state":"prepared-owner-approval-required","recipeDigest":validate.canonical_digest(index),
            "archiveDigest":"sha256:"+__import__("hashlib").sha256(raw).hexdigest(),"environment":observed,
            "project":project,"applicationId":index["applicationId"],"releaseVersion":index["releaseVersion"],
            "context":proposal["context"],"job":job,"executionApproved":False}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ("archive","recordtool","signing-policy","connection","environment","project","output"):parser.add_argument("--"+name,required=True)
    parser.add_argument("--operation",default="install",choices=["install","upgrade","backup","restore","uninstall"])
    parser.add_argument("--backup-id",default="")
    args=parser.parse_args();path=Path(args.archive)
    if path.is_symlink() or path.stat().st_size>MAX_ARCHIVE:raise ValueError("bounded-regular-package-required")
    policy=strict_json(Path(args.connection).read_bytes())
    result=prepare(path.read_bytes(),recordtool_verifier(args.recordtool,args.signing_policy),strict_json(Path(args.environment).read_bytes()),args.project,policy,client(policy),args.operation,args.backup_id)
    if os.name=="posix":os.umask(0o077)
    with Path(args.output).open("xb") as output:output.write(canonical(result))
    print("Private job prepared. Review and approve through the existing workbench; nothing was deployed.")


if __name__=="__main__":main()
