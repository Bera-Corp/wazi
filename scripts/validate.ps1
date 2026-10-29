# One-command Phase 0 validation on Windows PowerShell.
# Creates .venv on first run, installs pinned deps, runs all gates.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
if (-not (Test-Path ".venv\Scripts\python.exe")) {
    python -m venv .venv
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
& ".venv\Scripts\python.exe" -m pip install --quiet -r requirements-dev.txt
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& ".venv\Scripts\python.exe" scripts\validate.py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& ".venv\Scripts\python.exe" scripts\test_adversarial.py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& ".venv\Scripts\python.exe" scripts\test_sprints.py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& ".venv\Scripts\python.exe" scripts\secret_scan.py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& ".venv\Scripts\python.exe" scripts\test_secret_scan.py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& ".venv\Scripts\python.exe" scripts\test_render_compose.py
exit $LASTEXITCODE
