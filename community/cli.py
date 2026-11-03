"""Local catalog tooling. Never executes downloaded package content."""
import argparse
import base64
import datetime
import hashlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import time
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path

from .contracts import VERSION, canonical, contribution, digest, environment, strict_json
from .github import NoRedirect, refresh
from .outbox import Outbox
from .packages import configure, inspect_archive, recordtool_verifier
from .store import Store


def private_directory(path):
    path = Path(path).absolute()
    if path.is_symlink():
        raise ValueError("private directory cannot be a symlink")
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    if path.is_symlink() or (os.name != "nt" and path.stat().st_mode & 0o077):
        raise ValueError("existing private directory required")
    return path


def send(origin, endpoint, body, credential=None, ca_file=None, consent_change=None):
    import ssl
    parsed = urllib.parse.urlsplit(origin)
    if parsed.scheme != "https" or not parsed.netloc or parsed.path or parsed.query or parsed.fragment or parsed.username or parsed.password:
        raise ValueError("fixed HTTPS catalog origin required")
    headers = {"Content-Type": "application/zip" if isinstance(body, bytes) else "application/json"}
    if credential:
        headers["Authorization"] = "Bearer " + credential
    if consent_change is not None:
        headers["X-Wazi-Consent-Revision"] = str(consent_change)
    context = ssl.create_default_context(cafile=ca_file)
    request = urllib.request.Request(origin + endpoint, body if isinstance(body, bytes) else canonical(body) if body is not None else None, headers=headers, method="POST" if body is not None else "GET")
    opener = urllib.request.build_opener(NoRedirect, urllib.request.HTTPSHandler(context=context))
    with opener.open(request, timeout=15) as response:
        raw = response.read(65537)
        if len(raw) > 65536:
            raise ValueError("catalog response too large")
        return strict_json(raw)


def capture_cases(outbox, case_database, mappings):
    """Read normalized terminal cases only, never logs or private host facts."""
    consent = outbox.status()["consent"]
    if not consent["enabled"]:
        return 0
    database = Path(case_database).resolve(strict=True)
    db = sqlite3.connect(database.as_uri() + "?mode=ro", uri=True)
    db.row_factory = sqlite3.Row
    imported = 0
    try:
        # Query the actual case schema independently of the private store API.
        columns = {row[1] for row in db.execute("PRAGMA table_info(cases)")}
        if not {"document", "tenant", "id", "digest"} <= columns:
            raise ValueError("unrecognized private case store; no contribution emitted")
        for row in db.execute("SELECT document FROM cases ORDER BY rowid"):
            case = strict_json(row[0])
            mapping = mappings.get(case.get("applicationId"))
            if mapping is None or case.get("outcome") not in {"success", "failure", "aborted"}:
                continue
            if case.get("tenantScope") != mapping.get("tenantScope"):
                continue
            if case.get("releaseVersion") != mapping["releaseVersion"] or case.get("manifestDigest") != mapping["manifestDigest"]:
                continue
            finished = datetime.datetime.fromisoformat(case["finishedAt"].replace("Z", "+00:00"))
            if finished.tzinfo is None or finished.timestamp() < consent["changed"]:
                continue
            evidence = db.execute("SELECT digest FROM evidence WHERE tenant=? AND case_id=? ORDER BY id", (case["tenantScope"], case["caseId"])).fetchall()
            proofs = sorted({record[0] for record in evidence})[:16]
            if not proofs:
                continue
            public = {"schemaVersion": VERSION, "applicationId": case["applicationId"],
                      "recipeDigest": mapping["recipeDigest"], "releaseVersion": case["releaseVersion"],
                      "environment": mapping["environment"], "operation": case["operation"],
                      "outcome": case["outcome"], "proofDigests": proofs,
                      "templateDigest": mapping["templateDigest"], "consentVersion": VERSION, "license": "Apache-2.0"}
            if outbox.capture(public):
                imported += 1
    finally:
        db.close()
    return imported


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    for action in ("inspect", "configure", "publish"):
        command = sub.add_parser(action)
        command.add_argument("--archive", required=True)
        command.add_argument("--recordtool", required=True)
        command.add_argument("--policy", required=True)
        if action == "configure":
            command.add_argument("--inputs", required=True)
            command.add_argument("--output", required=True)
        if action == "publish":
            command.add_argument("--database", required=True)
            command.add_argument("--objects", required=True)
    command = sub.add_parser("refresh")
    command.add_argument("--database", required=True)
    command = sub.add_parser("revoke")
    command.add_argument("--database", required=True)
    command.add_argument("--recipe", required=True)
    command = sub.add_parser("draft-index")
    command.add_argument("--directory", required=True)
    command.add_argument("--metadata", required=True)
    command.add_argument("--output", required=True)
    command = sub.add_parser("bundle-candidate")
    command.add_argument("--index", required=True)
    command.add_argument("--directory", required=True)
    command.add_argument("--output", required=True)
    command = sub.add_parser("export-candidate")
    command.add_argument("--database", required=True)
    command.add_argument("--id", required=True)
    command.add_argument("--output", required=True)
    for action in ("consent-on", "consent-off", "outbox", "connect", "watch"):
        command = sub.add_parser(action)
        command.add_argument("--state", required=True)
        if action in {"connect", "watch"}:
            command.add_argument("--origin", required=True)
            command.add_argument("--ca-file")
        if action == "consent-on":
            command.add_argument("--accept-apache-2-original-recipes", action="store_true", required=True)
        if action == "watch":
            command.add_argument("--case-database")
            command.add_argument("--mapping")
            command.add_argument("--candidate-directory")
            command.add_argument("--once", action="store_true")
    args = parser.parse_args(argv)
    if args.action == "bundle-candidate":
        index = strict_json(Path(args.index).read_bytes())
        directory = Path(args.directory).resolve(strict=True)
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            archive.writestr("recipe.json", canonical(index))
            for name in index["files"]:
                from .packages import FILE_NAME
                if not FILE_NAME.fullmatch(name):
                    raise ValueError("unsafe bundle filename")
                path = directory / name
                if path.is_symlink() or not path.is_file() or path.stat().st_size > 1 << 20:
                    raise ValueError("bounded regular source files required")
                archive.writestr(name, path.read_bytes())
        raw = stream.getvalue()
        inspect_archive(raw, candidate=True)
        with Path(args.output).open("xb") as target:
            target.write(raw)
        print("Inert candidate archive created. It is not an installable signed release.")
    elif args.action == "export-candidate":
        with Store(args.database).connect() as db:
            row = db.execute("SELECT archive FROM candidates WHERE id=?", (args.id,)).fetchone()
        if not row:
            raise ValueError("candidate unavailable")
        inspect_archive(row[0], candidate=True)
        with Path(args.output).open("xb") as target:
            target.write(row[0])
        print("Untrusted candidate exported for independent qualification; nothing was executed.")
    elif args.action == "draft-index":
        from .packages import FILE_NAME, validate_index
        directory = Path(args.directory).resolve(strict=True)
        index = strict_json(Path(args.metadata).read_bytes())
        files = {}
        for path in sorted(directory.iterdir()):
            if not FILE_NAME.fullmatch(path.name) or path.name == "recipe.json" or path.is_symlink() or not path.is_file() or path.stat().st_size > 1 << 20:
                raise ValueError("draft directory contains unsupported content")
            raw = path.read_bytes()
            strict_json(raw)
            files[path.name] = "sha256:" + hashlib.sha256(raw).hexdigest()
        index["files"] = files
        validate_index(index, signed=False)
        with Path(args.output).open("xb") as stream:
            stream.write(canonical(index))
        print("Unsigned recipe index created. Independent qualification, review and signing are still required.")
    elif args.action in {"inspect", "configure", "publish"}:
        raw = Path(args.archive).read_bytes()
        index, files = inspect_archive(raw, recordtool_verifier(args.recordtool, args.policy))
        if args.action == "inspect":
            print(json.dumps(index, indent=2))
        elif args.action == "configure":
            output = Path(args.output).resolve()
            generated = configure(index, files, strict_json(Path(args.inputs).read_bytes()))
            # An exclusive new directory prevents overwriting existing instance configuration.
            output.mkdir(mode=0o700)
            for name, body in generated.items():
                with (output / name).open("xb") as stream:
                    stream.write(body)
            print("Configuration generated. Local policy validation and explicit installation approval are still required.")
        else:
            store = Store(args.database)
            if not any(app["applicationId"] == index["applicationId"] for app in store.applications()):
                raise ValueError("application must be curated before publication")
            objects = private_directory(args.objects)
            artifact = hashlib.sha256(raw).hexdigest()
            destination = objects / artifact
            if destination.is_symlink():
                raise ValueError("object path cannot be a symlink")
            if not destination.exists():
                with destination.open("xb") as stream:
                    stream.write(raw)
                    stream.flush()
                    os.fsync(stream.fileno())
            elif destination.read_bytes() != raw:
                raise ValueError("content-address conflict")
            summary = {key: index[key] for key in ("recipeId", "applicationId", "releaseVersion", "revision", "authors", "environments", "operations", "upstreamSources", "license", "evidenceRefs")}
            summary["artifact"] = artifact
            summary["requirements"] = strict_json(files["manifest.json"])["requirements"]
            release_id = index["recipeId"] + ":" + str(index["revision"])
            encoded = canonical(summary).decode()
            with store.connect() as db:
                previous = db.execute("SELECT body FROM recipes WHERE id=?", (release_id,)).fetchone()
                if previous and previous[0] != encoded:
                    raise ValueError("published recipe revisions are immutable")
                db.execute("INSERT OR IGNORE INTO recipes VALUES(?,?,?,?,0)", (release_id, index["applicationId"], encoded, artifact))
            print(json.dumps({"recipe": release_id, "artifact": artifact, "state": "signed"}))
    elif args.action == "refresh":
        print(json.dumps({"updated": refresh(Store(args.database))}))
    elif args.action == "revoke":
        with Store(args.database).connect() as db:
            if db.execute("UPDATE recipes SET revoked=1 WHERE id=?", (args.recipe,)).rowcount != 1:
                raise ValueError("unknown recipe revision")
        print("Recipe download revoked. Private execution must also receive a signed revocation record.")
    else:
        folder = private_directory(args.state)
        outbox = Outbox(folder / "outbox.sqlite")
        if args.action in {"consent-on", "consent-off"}:
            outbox.consent(args.action == "consent-on")
            print("Local contribution consent updated.")
        elif args.action == "outbox":
            print(json.dumps(outbox.status(), indent=2))
        elif args.action == "connect":
            pair = send(args.origin, "/api/device/start", {}, ca_file=args.ca_file)
            print("Open " + args.origin + "/dashboard?tab=contributions and approve device code " + pair["userCode"], flush=True)
            deadline = time.monotonic() + min(300, pair["expiresIn"])
            while time.monotonic() < deadline:
                time.sleep(5)
                result = send(args.origin, "/api/device/poll", {"deviceCode": pair["deviceCode"]}, ca_file=args.ca_file)
                if result["state"] == "authorized":
                    path = folder / "agent-identity.json"
                    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
                    with os.fdopen(fd, "wb") as stream:
                        stream.write(canonical({"origin": args.origin, "credential": result["credential"]}))
                    print("Contribution-only identity stored locally. It cannot approve deployments.")
                    break
            else:
                raise ValueError("device approval expired")
        elif args.action == "watch":
            identity_path = folder / "agent-identity.json"
            if identity_path.is_symlink() or (os.name != "nt" and identity_path.stat().st_mode & 0o077):
                raise ValueError("protected agent identity required")
            identity = strict_json(identity_path.read_bytes())
            if identity["origin"] != args.origin:
                raise ValueError("contribution identity origin mismatch")
            if bool(args.case_database) != bool(args.mapping) or not (args.case_database or args.candidate_directory):
                raise ValueError("configure case database/mapping together or a candidate directory")
            mappings = strict_json(Path(args.mapping).read_bytes()) if args.mapping else {}
            while True:
                try:
                    remote_consent = send(args.origin, "/api/agent/consent", None, identity["credential"], args.ca_file)
                except OSError:
                    if args.once:
                        raise
                    time.sleep(30)
                    continue
                outbox.sync_remote(remote_consent)
                if args.case_database:
                    capture_cases(outbox, args.case_database, mappings)
                if args.candidate_directory:
                    consent = outbox.status()["consent"]
                    if consent["enabled"]:
                        for path in sorted(Path(args.candidate_directory).glob("*.zip"))[:100]:
                            if path.is_symlink() or path.stat().st_size > 4 << 20 or path.stat().st_mtime < consent["changed"]:
                                continue
                            try:
                                outbox.capture_candidate(path.read_bytes())
                            except (ValueError, zipfile.BadZipFile):
                                print("An unsafe or unqualified candidate was withheld locally.", file=sys.stderr)
                def deliver(body):
                    if body.get("kind") == "recipe-candidate":
                        return send(args.origin, "/api/agent/recipe-candidates", base64.b64decode(body["archive"], validate=True), identity["credential"], args.ca_file, remote_consent["changed"])
                    return send(args.origin, "/api/agent/contributions", body, identity["credential"], args.ca_file, remote_consent["changed"])
                outbox.flush(deliver)
                if args.once:
                    break
                time.sleep(30)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, PermissionError, OSError) as error:
        print("Community operation failed: " + type(error).__name__, file=sys.stderr)
        sys.exit(1)
