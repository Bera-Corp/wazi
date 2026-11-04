"""Record manual/Wazi trials without importing logs or secret-bearing context."""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import sqlite3
import uuid


def connect(path):
    connection = sqlite3.connect(path)
    if os.name == "posix":
        os.chmod(path, 0o600)
    connection.execute("PRAGMA busy_timeout=5000")
    connection.execute("CREATE TABLE IF NOT EXISTS trials (id TEXT PRIMARY KEY, task TEXT NOT NULL, mode TEXT NOT NULL, source_commit TEXT NOT NULL, release_digest TEXT NOT NULL, started TEXT NOT NULL, completed TEXT, seconds REAL, interventions INTEGER, outcome TEXT)")
    connection.commit()
    return connection


def main():
    if os.name == "posix":
        os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", required=True)
    actions = parser.add_subparsers(dest="action", required=True)
    start = actions.add_parser("start")
    start.add_argument("--task", required=True, help="Opaque task ID; no beneficiary names or free text")
    start.add_argument("--mode", choices=("manual", "wazi"), required=True)
    start.add_argument("--source-commit", required=True)
    start.add_argument("--release-digest", required=True)
    finish = actions.add_parser("finish")
    finish.add_argument("--trial", required=True)
    finish.add_argument("--interventions", type=int, required=True)
    finish.add_argument("--outcome", choices=("success", "failure", "abandoned"), required=True)
    actions.add_parser("report")
    args = parser.parse_args()
    connection = connect(Path(args.database))
    try:
        current = dt.datetime.now(dt.timezone.utc)
        if args.action == "start":
            if not re.fullmatch(r"task_[A-Za-z0-9_-]{1,64}", args.task) or not re.fullmatch(r"[0-9a-f]{40}", args.source_commit) or not re.fullmatch(r"sha256:[0-9a-f]{64}", args.release_digest):
                raise ValueError("measurement-binding-invalid")
            identity = "trial_" + uuid.uuid4().hex
            with connection:
                connection.execute("INSERT INTO trials(id,task,mode,source_commit,release_digest,started) VALUES(?,?,?,?,?,?)", (identity,args.task,args.mode,args.source_commit,args.release_digest,current.isoformat()))
            print(identity)
        elif args.action == "finish":
            if not 0 <= args.interventions <= 10000:
                raise ValueError("intervention-count-invalid")
            with connection:
                connection.execute("BEGIN IMMEDIATE")
                row = connection.execute("SELECT started,completed FROM trials WHERE id=?", (args.trial,)).fetchone()
                if row is None or row[1] is not None:
                    raise ValueError("trial-missing-or-already-terminal")
                seconds = (current-dt.datetime.fromisoformat(row[0])).total_seconds()
                if seconds < 0:
                    raise ValueError("clock-moved-backwards")
                connection.execute("UPDATE trials SET completed=?,seconds=?,interventions=?,outcome=? WHERE id=? AND completed IS NULL", (current.isoformat(),seconds,args.interventions,args.outcome,args.trial))
        else:
            rows = connection.execute("SELECT task,mode,source_commit,release_digest,count(*),avg(seconds),avg(interventions),sum(CASE WHEN outcome='success' THEN 1 ELSE 0 END) FROM trials WHERE completed IS NOT NULL GROUP BY task,mode,source_commit,release_digest ORDER BY task,mode").fetchall()
            print(json.dumps({"observedAt":current.isoformat(),"groups":[dict(zip(("task","mode","sourceCommit","releaseDigest","trials","meanSeconds","meanInterventions","successes"),row)) for row in rows],"limitation":"Observed trials only; no causal or population-wide time-savings claim."}, indent=2))
    finally:
        connection.close()


if __name__ == "__main__":
    main()
