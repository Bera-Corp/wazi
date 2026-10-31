#!/bin/sh
# One-command Phase 0 validation on Linux/macOS.
# Creates .venv on first run, installs pinned deps, runs all gates.
set -eu
cd "$(dirname "$0")/.."
if [ ! -x .venv/bin/python ]; then
  python3 -m venv .venv
fi
.venv/bin/python -m pip install --quiet -r requirements-dev.txt
.venv/bin/python scripts/validate.py
.venv/bin/python scripts/test_adversarial.py
.venv/bin/python scripts/test_sprints.py
.venv/bin/python scripts/secret_scan.py
.venv/bin/python scripts/test_secret_scan.py
.venv/bin/python scripts/test_render_compose.py
.venv/bin/python scripts/test_context_engine.py
.venv/bin/python scripts/test_context_live.py
.venv/bin/python scripts/test_package_composition.py
.venv/bin/python scripts/test_candidate_author.py
.venv/bin/python scripts/test_prepare_package_build.py
