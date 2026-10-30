#!/usr/bin/python3
"""Root-owned fixed pilot quota helper; no arbitrary paths, IDs or limits.

Installed by the operator with a sudo rule for wazi-runtime. File descriptors
opened with O_NOFOLLOW pin the target inode across metadata checks and chattr.
"""
import json
import fcntl
import os
import pwd
import subprocess
import struct
import sys

RUNTIME = "wazi-runtime"
ROOT = "/srv/wazi-runtime/home/.local/share/docker/volumes"
ALLOWED = {
    "wazi-gitea-phase1_gitea-data": (31001, 18432),
    "wazi-gitea-phase1_gitea-config": (31002, 2048),
}


def execute(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, text=True,
                          timeout=15, env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin"}, **kwargs).stdout


def subuid_range(user):
    try:
        with open("/etc/subuid") as handle:
            for line in handle:
                parts = line.strip().split(":")
                if len(parts) == 3 and parts[0] == user:
                    return int(parts[1]), int(parts[2])
    except OSError:
        pass
    return None


def main():
    if os.getuid() != 0 or len(sys.argv) != 3 or sys.argv[1] not in ALLOWED:
        raise SystemExit("quota request outside the fixed pilot boundary")
    name = sys.argv[1]
    project_id, limit_mb = ALLOWED[name]
    if sys.argv[2] != str(limit_mb):
        raise SystemExit("quota differs from the owner-approved allocation")
    uid = pwd.getpwnam(RUNTIME).pw_uid
    raw = execute(["/usr/bin/sudo", "-n", "-u", RUNTIME, "/usr/bin/env",
                   "HOME=/srv/wazi-runtime/home", "XDG_RUNTIME_DIR=/run/user/" + str(uid),
                   "/usr/bin/docker", "--host", "unix:///run/user/" + str(uid) + "/docker.sock",
                   "volume", "inspect", name])
    record, = json.loads(raw)
    expected = ROOT + "/" + name + "/_data"
    labels = record.get("Labels") or {}
    if (record.get("Driver") != "local" or record.get("Options")
            or record.get("Mountpoint") != expected
            or labels.get("dev.wazi.project") != "wazi-gitea-phase1"
            or labels.get("dev.wazi.tenant") != "tenant_live_20261030T064834Z"):
        raise SystemExit("volume is not owned by the approved pilot")
    descriptors = []
    try:
        parent = os.open("/", os.O_RDONLY | os.O_DIRECTORY)
        descriptors.append(parent)
        for component in expected.strip("/").split("/"):
            parent = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=parent)
            descriptors.append(parent)
        # Before the application starts the volume root belongs to the
        # runtime user; afterwards the application (a container UID mapped
        # into the runtime user's subuid range) owns it. Both are the
        # approved pilot allocation; anything else is foreign.
        owner = os.fstat(parent).st_uid
        mapped = subuid_range(RUNTIME)
        if owner != uid and not (mapped and mapped[0] <= owner < mapped[0] + mapped[1]):
            raise SystemExit("volume directory owner mismatch")
        # linux/fs.h: struct fsxattr is five u32 fields and eight pad bytes.
        # Operate on the pinned descriptor, never a path resolved a second time.
        metadata = bytearray(28)
        fcntl.ioctl(parent, 0x801C581F, metadata, True)
        fields = list(struct.unpack("=IIIII8s", metadata))
        fields[0] |= 0x200  # FS_XFLAG_PROJINHERIT
        fields[3] = project_id
        fcntl.ioctl(parent, 0x401C5820, struct.pack("=IIIII8s", *fields))
        execute(["/usr/sbin/setquota", "-P", str(project_id), "0", str(limit_mb * 1024), "0", "0", "/srv/wazi-runtime"])
        fcntl.ioctl(parent, 0x801C581F, metadata, True)
        observed = struct.unpack("=IIIII8s", metadata)
        if observed[3] != project_id or not observed[0] & 0x200:
            raise SystemExit("project inheritance verification failed")
        quota = execute(["/usr/sbin/repquota", "-P", "/srv/wazi-runtime"])
        rows = [line.split() for line in quota.splitlines() if line.startswith("#" + str(project_id) + " ")]
        if len(rows) != 1 or int(rows[0][4]) != limit_mb * 1024:
            raise SystemExit("hard quota verification failed")
        print(json.dumps({"volume": name, "projectId": project_id, "hardLimitMb": limit_mb, "verified": True}))
    finally:
        for descriptor in reversed(descriptors):
            os.close(descriptor)


if __name__ == "__main__":
    main()
