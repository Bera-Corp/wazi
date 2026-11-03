"""Read-only projection of checked-in pilot evidence, never current host state."""
import hashlib
from pathlib import Path

from .contracts import strict_json

REPORT = Path(__file__).resolve().parent.parent / "releases/phase2/live-evidence/2026-11-01/host-report.json"
RELEASES = {"org.gitea.gitea": "28.1.0", "org.postgresql.postgresql": "17.11.0+wazi.1",
            "com.mysql.mysql": "8.4.13+wazi.1", "org.nginx.nginx": "1.30.6+wazi.2"}


def evidence(application):
    if application not in RELEASES or not REPORT.is_file():
        return None
    raw = REPORT.read_bytes()
    report = strict_json(raw)
    if application == "org.gitea.gitea":
        operations = ["install", "upgrade", "backup", "restore", "rollback"]
    else:
        operations = sorted({row["operation"] for row in report["packageOperations"]
                             if row["applicationId"] == application and row["state"] == "completed"})
    return {"kind": "historical-pilot-evidence", "releaseVersion": RELEASES[application],
            "environment": "Linux Docker Engine / amd64 / bera-003 pilot",
            "operations": operations, "verifiedAt": report["verifiedAt"],
            "sourceCommit": report["sourceIdentity"]["sourceCommit"],
            "evidenceDigest": "sha256:" + hashlib.sha256(raw).hexdigest(),
            "portableDownloadPublished": False,
            "limitations": "Historical single-host pilot; desktop compatibility and a portable package release are not established."}
