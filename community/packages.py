"""Portable, inert archives; only a trusted verifier can admit a signed index."""
import hashlib
import functools
import io
import json
import re
import stat
import subprocess
import tempfile
import zipfile
from pathlib import Path
from scripts.secret_scan import scan_text

from .contracts import OPERATIONS, VERSION, canonical, closed, environment, identifier, sha, strict_json, version

MAX_ARCHIVE = 4 << 20
FILE_NAME = re.compile(r"[a-z][a-z0-9_-]{0,63}\.json\Z")


def supported_file(name):
    return name in {"manifest.json", "licenses.json"} or name.startswith(("procedure-", "config-"))


def validate_index(value, signed=True):
    fields = "schemaVersion recipeId applicationId releaseVersion revision license authors upstreamSources inputs files environments operations operationProcedures evidenceRefs"
    closed(value, fields + (" signature" if signed else ""))
    if value["schemaVersion"] != VERSION or value["license"] != "Apache-2.0":
        raise ValueError("recipe version/license required")
    identifier(value["recipeId"])
    identifier(value["applicationId"])
    version(value["releaseVersion"])
    if type(value["revision"]) is not int or not 1 <= value["revision"] <= 100000:
        raise ValueError("invalid recipe revision")
    authors = value["authors"]
    if not isinstance(authors, list) or not 1 <= len(authors) <= 16:
        raise ValueError("explicit recipe attribution required")
    for author in authors:
        if not isinstance(author, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9-]{0,38}", author):
            raise ValueError("public GitHub attribution required")
    sources = value["upstreamSources"]
    if not isinstance(sources, list) or not 1 <= len(sources) <= 16:
        raise ValueError("pinned upstream sources required")
    for source in sources:
        closed(source, "repository commit license")
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", source["repository"]) or not re.fullmatch(r"[0-9a-f]{40}", source["commit"]):
            raise ValueError("pinned source required")
        if not re.fullmatch(r"[A-Za-z0-9.+-]{1,96}", source["license"]):
            raise ValueError("upstream license required")
    inputs = value["inputs"]
    if not isinstance(inputs, dict) or len(inputs) > 32:
        raise ValueError("bounded typed inputs required")
    for name, spec in inputs.items():
        identifier(name)
        closed(spec, "type required")
        if spec["type"] not in {"instance", "port", "secret-reference"} or type(spec["required"]) is not bool:
            raise ValueError("unsupported input")
    files = value["files"]
    if not isinstance(files, dict) or not 1 <= len(files) <= 32 or not {"manifest.json", "licenses.json"} <= set(files):
        raise ValueError("manifest and bounded files required")
    for name, fingerprint in files.items():
        if not FILE_NAME.fullmatch(name) or not supported_file(name):
            raise ValueError("unsafe package filename")
        sha(fingerprint)
    if not isinstance(value["environments"], list) or not 1 <= len(value["environments"]) <= 32:
        raise ValueError("explicit environment observations required")
    for item in value["environments"]:
        closed(item, "environment operations qualificationDigest")
        environment(item["environment"])
        if not isinstance(item["operations"], list) or not item["operations"] or not set(item["operations"]) <= OPERATIONS:
            raise ValueError("invalid environment operations")
        sha(item["qualificationDigest"])
        if signed and item["environment"]["enforcement"] != "verified":
            raise ValueError("signed installation coverage requires verified enforcement")
    if not isinstance(value["operations"], list) or not value["operations"] or not set(value["operations"]) <= OPERATIONS:
        raise ValueError("explicit lifecycle operations required")
    if len(set(value["operations"])) != len(value["operations"]):
        raise ValueError("duplicate lifecycle operation")
    if any(not set(item["operations"]) <= set(value["operations"]) for item in value["environments"]):
        raise ValueError("environment advertises an undeclared operation")
    bindings = value["operationProcedures"]
    if not isinstance(bindings, dict) or set(bindings) != set(value["operations"]):
        raise ValueError("operation-to-procedure binding required")
    for operation, name in bindings.items():
        if not isinstance(name, str) or not name.startswith("procedure-") or name not in files:
            raise ValueError("operation procedure missing from archive")
    if not isinstance(value["evidenceRefs"], list) or not 1 <= len(value["evidenceRefs"]) <= 32:
        raise ValueError("evidence references required")
    for item in value["evidenceRefs"]:
        sha(item)
    if signed and not isinstance(value["signature"], dict):
        raise ValueError("signed package index required")
    return value


def inspect_archive(raw, verifier=None, *, candidate=False):
    try:
        return _inspect_archive(raw, verifier, candidate=candidate)
    except (KeyError, TypeError, UnicodeError, RecursionError, NotImplementedError) as error:
        raise ValueError("malformed portable package") from error


def _inspect_archive(raw, verifier=None, *, candidate=False):
    if len(raw) > MAX_ARCHIVE:
        raise ValueError("package archive too large")
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        entries = archive.infolist()
        if not 2 <= len(entries) <= 33 or len({entry.filename for entry in entries}) != len(entries):
            raise ValueError("duplicate or unbounded archive entries")
        total, files = 0, {}
        for entry in entries:
            if not FILE_NAME.fullmatch(entry.filename) or entry.flag_bits & 1:
                raise ValueError("unsafe archive entry")
            mode = entry.external_attr >> 16
            if stat.S_ISLNK(mode) or (stat.S_IFMT(mode) and not stat.S_ISREG(mode)):
                raise ValueError("regular files only")
            total += entry.file_size
            if entry.file_size > 1 << 20 or total > MAX_ARCHIVE:
                raise ValueError("expanded archive too large")
            contents = archive.read(entry)
            parsed_contents = strict_json(contents)
            validate_public_paths(parsed_contents)
            decoded_text = canonical(parsed_contents).decode("ascii")
            if "wazi-secret-scan: allow" in decoded_text or scan_text(decoded_text):
                raise ValueError("package contains a credential pattern")
            files[entry.filename] = contents
    index = validate_index(strict_json(files.pop("recipe.json")), signed=not candidate)
    if set(index["files"]) != set(files):
        raise ValueError("package inventory mismatch")
    for name, contents in files.items():
        if "sha256:" + hashlib.sha256(contents).hexdigest() != index["files"][name]:
            raise ValueError("package content digest mismatch")
    if verifier is None and not candidate:
        raise ValueError("trusted signature verifier required")
    if not candidate:
        verifier(canonical(index))
    manifest = strict_json(files["manifest.json"])
    validate_record("app-manifest.schema.json", manifest)
    license_texts = strict_json(files["licenses.json"])
    required_licenses = {"Apache-2.0", manifest["license"]} | {source["license"] for source in index["upstreamSources"]}
    if not isinstance(license_texts, dict) or set(license_texts) != required_licenses:
        raise ValueError("original and upstream license copies required")
    for license_id, entry in license_texts.items():
        closed(entry, "text notices")
        if not isinstance(entry["text"], str) or not 100 <= len(entry["text"]) <= 65536 or not isinstance(entry["notices"], str) or len(entry["notices"]) > 4096:
            raise ValueError("bounded license text and attribution required")
    if "Apache License" not in license_texts["Apache-2.0"]["text"] or "Version 2.0" not in license_texts["Apache-2.0"]["text"]:
        raise ValueError("original recipe Apache-2.0 license copy required")
    if manifest.get("appId") != index["applicationId"] or manifest.get("version") != index["releaseVersion"]:
        raise ValueError("package manifest binding mismatch")
    for item in index["environments"]:
        observed = item["environment"]
        architecture = {"amd64": "x86_64", "arm64": "arm64"}[observed["architecture"]]
        if architecture not in manifest["requirements"]["architectures"]:
            raise ValueError("environment architecture contradicts signed manifest")
        from scripts.validate import version_tuple
        for observed_field, minimum_field in (("dockerVersion", "minDockerVersion"), ("composeVersion", "minComposeVersion")):
            if version_tuple(observed[observed_field]) < version_tuple(manifest["requirements"][minimum_field]):
                raise ValueError("environment runtime is below manifest minimum")
    if candidate:
        if manifest.get("reviewState") != "candidate" or "signature" in manifest:
            raise ValueError("candidate archive requires an unsigned candidate manifest")
    else:
        verifier(canonical(manifest))
    for name, contents in files.items():
        if name.startswith("procedure-"):
            procedure = strict_json(contents)
            validate_record("procedure.schema.json", procedure)
            if procedure.get("applicationId") != index["applicationId"]:
                raise ValueError("procedure application binding mismatch")
            if candidate:
                if procedure.get("reviewState") != "candidate" or "signature" in procedure:
                    raise ValueError("candidate procedures must be unsigned")
            else:
                verifier(canonical(procedure))
        if name.startswith("config-"):
            validate_template(strict_json(contents), index["inputs"])
    for operation, name in index["operationProcedures"].items():
        if strict_json(files[name]).get("operation") != operation:
            raise ValueError("operation procedure binding mismatch")
    return index, files


def validate_public_paths(value):
    if isinstance(value, dict):
        for key, item in value.items():
            validate_public_paths(key)
            validate_public_paths(item)
    elif isinstance(value, list):
        for item in value:
            validate_public_paths(item)
    elif isinstance(value, str) and re.search(r"(?i)(/users/|/home/|[a-z]:[\\/]users[\\/])", value):
        raise ValueError("private home paths forbidden in every package role")


@functools.lru_cache(maxsize=2)
def record_validator(name):
    from scripts import validate
    root = Path(__file__).resolve().parent.parent / "schemas"
    schemas = {filename: strict_json((root / filename).read_bytes())
               for filename in ("common.schema.json", "app-manifest.schema.json", "procedure.schema.json")}
    return validate.make_validator(schemas[name], validate.build_registry(schemas))


def validate_record(name, record):
    if not record_validator(name).is_valid(record):
        raise ValueError("package record does not satisfy canonical execution schema")


def validate_template(value, inputs):
    if isinstance(value, dict):
        if set(value) == {"waziInput"}:
            if value["waziInput"] not in inputs:
                raise ValueError("template references unknown input")
            return
        for key, item in value.items():
            if re.search(r"(?i)(password|passwd|secret|token|api.?key|private.?key)", key):
                if isinstance(item, dict) and set(item) == {"waziInput"}:
                    if inputs.get(item["waziInput"], {}).get("type") != "secret-reference":
                        raise ValueError("secret input must be a protected reference")
                elif not isinstance(item, str) or not item.startswith("secret://"):
                    raise ValueError("secret values forbidden in portable configuration")
            validate_template(item, inputs)
    elif isinstance(value, list):
        for item in value:
            validate_template(item, inputs)
    elif isinstance(value, str) and (re.search(r"(?i)(/users/|/home/|[a-z]:[\\/]users[\\/])", value) or "wazi-secret-scan: allow" in value):
        raise ValueError("machine-specific paths or scanner bypass markers forbidden")


def recordtool_verifier(executable, policy):
    executable, policy = Path(executable).resolve(strict=True), Path(policy).resolve(strict=True)
    def verify(raw):
        with tempfile.TemporaryDirectory(prefix="wazi-community-verify-") as folder:
            record = Path(folder) / "recipe.json"
            record.write_bytes(raw)
            command = "verify-community-package" if "recipeId" in strict_json(raw) else "verify"
            result = subprocess.run([str(executable), command, str(record), str(policy)],
                                    stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL, timeout=20, check=False)
            if result.returncode != 0:
                raise ValueError("package signature untrusted")
    return verify


def configure(index, files, inputs):
    """Generate inert JSON files only. Installation still needs local runtime approval."""
    if not isinstance(inputs, dict) or set(inputs) - set(index["inputs"]):
        raise ValueError("unknown configuration input")
    for name, spec in index["inputs"].items():
        if spec["required"] and name not in inputs:
            raise ValueError("missing configuration input")
        if name not in inputs:
            continue
        value = inputs[name]
        if spec["type"] == "port":
            if type(value) is not int or not 1024 <= value <= 65535:
                raise ValueError("private unprivileged port required")
        elif spec["type"] == "instance":
            identifier(value)
        elif not isinstance(value, str) or not re.fullmatch(r"secret://[a-z0-9_./-]{1,160}", value):
            raise ValueError("protected secret reference required")
    def replace(value):
        if isinstance(value, dict):
            if set(value) == {"waziInput"}:
                if value["waziInput"] not in inputs:
                    raise ValueError("unresolved input")
                return inputs[value["waziInput"]]
            return {key: replace(item) for key, item in value.items()}
        if isinstance(value, list):
            return [replace(item) for item in value]
        return value
    return {name: canonical(replace(strict_json(raw))) for name, raw in files.items()
            if name.startswith("config-")}
