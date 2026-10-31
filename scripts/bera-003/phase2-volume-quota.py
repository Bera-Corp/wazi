#!/usr/bin/python3
"""Fixed Phase 2 volume quotas; reuses the protected descriptor-based helper."""
import os
import json
import runpy
import subprocess
import sys


def main():
    if os.getuid() != 0:
        raise SystemExit("quota request outside the fixed Phase 2 boundary")
    if len(sys.argv) == 2 and sys.argv[1] == "capacity":
        report = subprocess.run(["/usr/sbin/repquota", "-P", "/srv/wazi-runtime"],
                                check=True, capture_output=True, text=True, timeout=15).stdout
        rows = [line.split() for line in report.splitlines() if line.startswith("#30000 ")]
        if len(rows) != 1:
            raise SystemExit("shared pool quota unavailable")
        print(json.dumps({"usedKiB": int(rows[0][2]), "hardKiB": int(rows[0][4])}))
        return
    if len(sys.argv) != 3:
        raise SystemExit("quota request outside the fixed Phase 2 boundary")
    allowed, projects = {}, {}
    for profile, start, limit, volume in (("postgresql", 32010, 2048, "data"),
                                           ("mysql", 32020, 2048, "data"),
                                           ("nginx", 32030, 128, "config")):
        for index in (1, 2):
            project = "wazi-" + profile + "-0" + str(index)
            name = project + "_" + volume
            allowed[name] = (start + index - 1, limit)
            projects[name] = project
    if sys.argv[1] not in allowed or sys.argv[2] != str(allowed[sys.argv[1]][1]):
        raise SystemExit("quota request outside the fixed Phase 2 boundary")
    core = runpy.run_path("/usr/local/libexec/wazi-phase1-quota")
    core["ALLOWED"].clear()
    core["ALLOWED"].update(allowed)
    core["PROJECTS"].clear()
    core["PROJECTS"].update(projects)
    core["main"]()


if __name__ == "__main__":
    main()
