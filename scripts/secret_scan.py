#!/usr/bin/env python3
"""Dedicated in-repo secret scanner (Phase 0 safeguard).

Scans text content for known credential shapes and for high-entropy
values assigned to secret-named keys, including quoted JSON keys.
A `secret://` value is exempt only for that matched assignment; a
reference appearing elsewhere on the same line does not suppress a
finding. This is a backstop: it cannot prove arbitrary text is
secret-free. The primary control is structural - Wazi contracts have
no field that can carry a secret value.

Usage: python scripts/secret_scan.py [root]
Exit 1 with file:line findings (values are never printed).
"""
import math
import re
import sys
from collections import Counter
from pathlib import Path

# Exclusions are reviewed paths relative to the scanned root, never
# basenames: a validate.py anywhere else is scanned like any file.
EXCLUDED_PATHS = {
    "scripts/secret_scan.py",
    "scripts/validate.py",
    "scripts/test_adversarial.py",
    "scripts/test_secret_scan.py",
    "scripts/test_render_compose.py",
}
SKIP_DIRS = {".git", ".venv", "__pycache__", "node_modules"}
# Reviewed intentional-content prefixes: adversarial fixtures deliberately
# contain synthetic credentials to prove other validators reject them.
EXCLUDED_PREFIXES = ("evals/adversarial/",)

TOKEN_PATTERNS = [
    ("github-token", re.compile(r"gh[pousr]_[A-Za-z0-9]{10,}")),
    ("aws-access-key", re.compile(r"AKIA[0-9A-Z]{16}")),
    ("private-key", re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY")),
    ("bearer-token", re.compile(r"Bearer\s+[A-Za-z0-9._\-]{16,}")),
]
ASSIGNMENT = re.compile(
    r"(?i)[\"']?(password|passwd|secret|token|api[_-]?key)[\"']?\s*[:=]\s*[\"']?([^\s\"',}]{8,})"
)
ALLOW_MARKER = "wazi-secret-scan: allow"


def entropy(text):
    counts = Counter(text)
    total = len(text)
    return -sum((c / total) * math.log2(c / total) for c in counts.values())


def scan_text(text):
    findings = []
    for lineno, line in enumerate(text.splitlines(), 1):
        if ALLOW_MARKER in line:
            continue
        for label, pattern in TOKEN_PATTERNS:
            if pattern.search(line):
                findings.append((lineno, label))
                break
        else:
            for match in ASSIGNMENT.finditer(line):
                if line[match.end(1):].startswith("://"):
                    continue  # the "key" is a URI scheme, not an assignment
                value = match.group(2)
                if value.startswith("secret://"):
                    continue  # an identifier, not a value; exempt this match only
                if entropy(value) >= 3.5 or len(value) >= 12:
                    findings.append((lineno, "secret-assignment"))
                    break
    return findings


def main(argv):
    root = Path(argv[1]) if len(argv) > 1 else Path(__file__).resolve().parent.parent
    problems = []
    for path in sorted(root.rglob("*")):
        if not path.is_file() or any(part in SKIP_DIRS for part in path.parts):
            continue
        relative = path.relative_to(root).as_posix()
        if relative in EXCLUDED_PATHS or relative.startswith(EXCLUDED_PREFIXES) or path.suffix in {".pyc", ".png", ".jpg", ".ico"}:
            continue
        try:
            text = path.read_text(encoding="utf-8")
        except (UnicodeDecodeError, OSError):
            continue
        for lineno, label in scan_text(text):
            problems.append(f"{relative}:{lineno}: {label} (value suppressed)")
    if problems:
        print("SECRET SCAN FAILED")
        for problem in problems:
            print(f" - {problem}")
        return 1
    print("secret scan passed")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
