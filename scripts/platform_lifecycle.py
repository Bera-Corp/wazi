"""Prepare exact-platform lifecycle bindings for the existing approved-job runner.

Supports preparing Linux Engine, Windows/WSL2 and macOS Desktop targets without
inferring desktop qualification from Linux guest success. This CLI does not run
containers or make deployments; qualified hosts execute through their agent API.
"""
import argparse
import json
import os
from pathlib import Path
import sys

sys.path.insert(0,str(Path(__file__).resolve().parents[1]))

from community.contracts import canonical, closed, environment, strict_json
from community.packages import MAX_ARCHIVE, inspect_archive, recordtool_verifier
from community.platforms import compatibility
import lifecycle_rollout
import validate


def prepare(plan,bindings,load):
    lifecycle_rollout.checked_plan(plan)
    closed(bindings,"schemaVersion targets")
    if bindings["schemaVersion"]!="1.0.0" or not isinstance(bindings["targets"],dict):raise ValueError("platform-bindings-invalid")
    targets=[target for wave in plan["waves"] for target in wave["targets"]]
    if set(bindings["targets"])!={target["targetId"] for target in targets}:raise ValueError("complete-target-platform-bindings-required")
    observations=[]
    for target in targets:
        binding=bindings["targets"][target["targetId"]]
        closed(binding,"environment fromArchive toArchive")
        observed=environment(binding["environment"])
        releases={}
        for key,digest in (("fromArchive","fromManifestDigest"),("toArchive","toManifestDigest")):
            index,files=load(binding[key])
            manifest=strict_json(files["manifest.json"])
            if index["applicationId"]!=target["applicationId"] or validate.canonical_digest(manifest)!=target[digest]:
                raise ValueError("platform-release-binding-mismatch")
            releases[target[digest]]=(index,files)
        for direction in ("forward","recovery"):
            proofs=[]
            for action in target[direction]:
                job=action["job"];operation=job["plan"]["operation"]
                selected,records=releases[job["envelope"]["manifestDigest"]]
                match=compatibility(selected,observed,operation)
                if not match["eligible"]:raise ValueError("lifecycle-operation-platform-not-qualified")
                if operation not in selected["operationProcedures"]:raise ValueError("lifecycle-procedure-missing")
                procedure=strict_json(records[selected["operationProcedures"][operation]])
                if job["envelope"]["procedureRef"]!={"recordId":procedure["procedureId"],"version":procedure["version"],"digest":validate.canonical_digest(procedure)}:
                    raise ValueError("lifecycle-procedure-binding-mismatch")
                proofs.append(match["qualificationDigest"])
            observations.append({"targetId":target["targetId"],"direction":direction,"environment":observed,
                                 "recipeDigests":sorted({validate.canonical_digest(index) for index,files in releases.values()}),"qualificationDigests":sorted(set(proofs))})
    return {"schemaVersion":"1.0.0","state":"prepared-preapproved-jobs-only","planDigest":validate.canonical_digest(plan),
            "platformBindings":observations,"plan":plan,"executed":False}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ("plan","bindings","recordtool","signing-policy","output"):parser.add_argument("--"+name,required=True)
    args=parser.parse_args();verifier=recordtool_verifier(args.recordtool,args.signing_policy);cache={}
    def load(name):
        if name not in cache:
            path=Path(name)
            if path.is_symlink() or path.stat().st_size>MAX_ARCHIVE:raise ValueError("bounded-regular-package-required")
            cache[name]=inspect_archive(path.read_bytes(),verifier)
        return cache[name]
    result=prepare(strict_json(Path(args.plan).read_bytes()),strict_json(Path(args.bindings).read_bytes()),load)
    if os.name=="posix":os.umask(0o077)
    with Path(args.output).open("xb") as output:output.write(canonical(result))
    print("Platform-bound rollout prepared. No platform was qualified and no job was submitted by this command.")


if __name__=="__main__":main()
