"""Prepare only the pinned official package recipes in the reviewed source lock."""
import argparse
import hashlib
import json
import os
import re
import subprocess
import io
import tarfile
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
REPOSITORIES = {"postgresql": "docker-library/postgres", "mysql": "docker-library/mysql", "nginx": "nginx/docker-nginx", "uptime-kuma": "louislam/uptime-kuma"}


def locked_source(profile):
    source = json.loads((ROOT / "releases/phase2/source-lock.json").read_text())["packages"][profile]
    if source["repository"] != REPOSITORIES[profile] or not re.fullmatch("[0-9a-f]{40}", source["commit"]):
        raise ValueError("source-identity-invalid")
    if not re.fullmatch("[a-z0-9./_-]+@sha256:[0-9a-f]{64}", source["baseImage"]):
        raise ValueError("base-image-not-pinned")
    for key in ("context", "dockerfile"):
        path = Path(source[key])
        if path.is_absolute() or ".." in path.parts or not re.fullmatch("[A-Za-z0-9_./-]+", source[key]):
            raise ValueError("source-path-invalid")
    return source


def patch_recipe(raw, source):
    if hashlib.sha256(raw).hexdigest() != source["dockerfileSha256"]:
        raise ValueError("source-recipe-digest-mismatch")
    text = raw.decode("utf-8")
    if source["repository"] == "louislam/uptime-kuma":
        helper = source["helperImage"]
        if not re.fullmatch("louislam/uptime-kuma@sha256:[0-9a-f]{64}", helper):
            raise ValueError("helper-image-not-pinned")
        text = text.replace("ARG BASE_IMAGE=louislam/uptime-kuma:base2", "ARG BASE_IMAGE=" + source["baseImage"])
        text = text.replace("louislam/uptime-kuma:builder-go", helper)
        text = text.replace("louislam/uptime-kuma:base2", source["baseImage"])
        return (text + '\nFROM rootless AS qualification\nUSER root\nCOPY .wazi/qualify /wazi/qualify\nRUN chmod 0555 /wazi/qualify\nUSER 65534:65534\n').encode()
    old = "FROM " + source["baseTag"] + "\n"
    if text.count(old) != 1 or len(re.findall(r"^FROM ", text, re.MULTILINE)) != 1:
        raise ValueError("source-base-surface-changed")
    return text.replace(old, "FROM " + source["baseImage"] + "\n", 1).encode()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profile", choices=sorted(REPOSITORIES))
    parser.add_argument("--checkout", type=Path)
    args = parser.parse_args()
    source = locked_source(args.profile)
    if args.checkout:
        checkout = args.checkout.resolve()
        head = subprocess.check_output(["git", "-C", str(checkout), "rev-parse", "HEAD"], text=True).strip()
        if head != source["commit"]:
            raise ValueError("source-checkout-mismatch")
        path = checkout / source["dockerfile"]
        if args.profile == "uptime-kuma":
            with urllib.request.urlopen(source["distUrl"], timeout=120) as response:
                raw = response.read((64 << 20) + 1)
            if len(raw) > 64 << 20 or hashlib.sha256(raw).hexdigest() != source["distSha256"]:
                raise ValueError("upstream-dist-digest-mismatch")
            with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as archive:
                total = 0
                for member in archive.getmembers():
                    parts = Path(member.name).parts
                    total += member.size
                    if not parts or parts[0] != "dist" or ".." in parts or member.name.startswith("/") or "\\" in member.name or not (member.isfile() or member.isdir()) or total > 256 << 20:
                        raise ValueError("upstream-dist-shape-invalid")
                archive.extractall(checkout, filter="data")
            suite = checkout / ".wazi" / "qualify"
            suite.parent.mkdir(exist_ok=True)
            suite.write_bytes((ROOT / "scripts/uptime_kuma_qualification.mjs").read_bytes())
        patched = patch_recipe(path.read_bytes(), source)
        path.with_name(path.name + ".wazi").write_bytes(patched)
        predicate = {"schemaVersion": "1.0.0", "source": source,
                     "patchedRecipeSha256": hashlib.sha256(patched).hexdigest(),
                     "builderCommit": os.environ.get("GITHUB_SHA"),
                     "note": "Pinned official recipe over authenticated GitHub transport; no verified upstream commit signature is claimed."}
        Path("package-source-provenance.json").write_text(json.dumps(predicate, sort_keys=True))
    else:
        outputs = dict(repository=source["repository"], commit=source["commit"],
                       context="upstream/" + source["context"], dockerfile="upstream/" + source["dockerfile"] + ".wazi",
                       image="ghcr.io/bera-corp/wazi-phase2-" + args.profile)
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as handle:
            for key, value in outputs.items():
                handle.write(key + "=" + value + "\n")


if __name__ == "__main__":
    main()
