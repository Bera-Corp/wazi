"""Closed, versioned public contracts. Local paths and raw records never cross here."""
import hashlib
import json
import re

VERSION = "1.0.0"
OPERATIONS = {"install", "upgrade", "backup", "restore", "rollback", "uninstall", "health-check"}
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
IDENTIFIER = re.compile(r"[a-z][a-z0-9._-]{0,95}\Z")
ENVIRONMENTS = {"linux-engine", "windows-wsl2", "macos-desktop"}


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True,
                      allow_nan=False).encode("ascii")


def digest(value):
    return "sha256:" + hashlib.sha256(canonical(value)).hexdigest()


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate property")
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=pairs,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError("non-finite number")))


def closed(value, fields):
    if not isinstance(value, dict) or set(value) != set(fields.split()):
        raise ValueError("unexpected or missing properties")


def identifier(value):
    if not isinstance(value, str) or not IDENTIFIER.fullmatch(value):
        raise ValueError("invalid public identifier")
    return value


def sha(value):
    if not isinstance(value, str) or not DIGEST.fullmatch(value):
        raise ValueError("invalid digest")
    return value


def version(value):
    if not isinstance(value, str) or not re.fullmatch(r"[0-9][A-Za-z0-9.+_-]{0,63}", value):
        raise ValueError("invalid version")
    return value


def environment(value):
    closed(value, "family architecture desktopVersion guestVersion dockerVersion composeVersion enforcement")
    if value["family"] not in ENVIRONMENTS or value["architecture"] not in {"amd64", "arm64"}:
        raise ValueError("unsupported environment descriptor")
    for name in ("desktopVersion", "guestVersion", "dockerVersion", "composeVersion"):
        version(value[name])
    if value["enforcement"] not in {"verified", "unverified", "unavailable"}:
        raise ValueError("invalid enforcement observation")
    return value


def contribution(value):
    closed(value, "schemaVersion applicationId recipeDigest releaseVersion environment operation outcome proofDigests templateDigest consentVersion license")
    if value["schemaVersion"] != VERSION or value["consentVersion"] != VERSION:
        raise ValueError("unsupported contribution/consent version")
    identifier(value["applicationId"])
    sha(value["recipeDigest"])
    sha(value["templateDigest"])
    version(value["releaseVersion"])
    environment(value["environment"])
    if value["operation"] not in OPERATIONS or value["outcome"] not in {"success", "failure", "aborted"}:
        raise ValueError("invalid terminal outcome")
    proofs = value["proofDigests"]
    if not isinstance(proofs, list) or not 1 <= len(proofs) <= 16 or len(set(proofs)) != len(proofs):
        raise ValueError("bounded unique evidence digests required")
    for proof in proofs:
        sha(proof)
    if value["license"] != "Apache-2.0":
        raise ValueError("original recipe license consent required")
    return value


def project(value):
    closed(value, "applicationId name repository category description deferred")
    identifier(value["applicationId"])
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", value["repository"]):
        raise ValueError("invalid GitHub repository")
    for name, maximum in (("name", 80), ("category", 40), ("description", 240)):
        if not isinstance(value[name], str) or not 1 <= len(value[name]) <= maximum:
            raise ValueError("invalid project text")
    if type(value["deferred"]) is not bool:
        raise ValueError("invalid deferred flag")
    return value
