"""Prepare a read-only health procedure for an existing immutable signed release.

This adds a new procedure identity; it does not reissue manifests or replace any
published lifecycle procedure. Normal signing and registry verification follow.
"""
import argparse
import copy
from pathlib import Path
import context_engine as engine
from prepare_package_build import locked_source
from prepare_phase2_release import SHAPES,canonical
import validate


def record(profile,manifest,version):
    shape=SHAPES[profile]
    if manifest.get("reviewState")!="signed" or not manifest.get("signature") or manifest["appId"]!=locked_source(profile)["applicationId"]:
        raise ValueError("existing-signed-profile-release-required")
    if manifest["release"]["healthCheck"]["target"]!=shape["health"]:raise ValueError("fixed-profile-health-target-required")
    value={"schemaVersion":"1.0.0","procedureId":"proc_health-check_"+profile,"version":version,"operation":"health-check","applicationId":manifest["appId"],
           "capabilitySequence":["manifest.verify","health.check"],"parameterContracts":[{"capability":"manifest.verify","requiredParameters":["manifestDigest","requireReviewState"],"fixedParameters":{"requireReviewState":"signed"}},
           {"capability":"health.check","requiredParameters":["target"],"fixedParameters":{"target":shape["health"]}}],"requiresCheckpoint":False,"provenance":copy.deepcopy(manifest["provenance"]),"reviewState":"signed"}
    errors=engine._record_schema_errors("procedure.schema.json",dict(value,reviewState="candidate"))
    if errors:raise ValueError("health-procedure-schema-invalid")
    return value


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile",required=True,choices=sorted(SHAPES))
    for name in ("manifest","version","output"):parser.add_argument("--"+name,required=True)
    args=parser.parse_args()
    with Path(args.output).open("xb") as output:output.write(canonical(record(args.profile,validate.load(Path(args.manifest)),args.version)))
