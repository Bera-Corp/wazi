[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Operation,
    [Parameter(Mandatory = $true)][string]$ReleaseDir,
    [Parameter(Mandatory = $true)][string]$FactsFile,
    [Parameter(Mandatory = $true)][string]$JobId,
    [Parameter(Mandatory = $true)][string]$CaseId,
    [int]$PollSeconds = 150,
    # Adversarial-evidence hook: when set, rewrite the assembled job's
    # volume.backup backupId after approval issuance. The approval binds
    # the untampered plan digest, so the pilot must reject the job
    # before any mutation. Never set for lifecycle runs.
    [string]$TamperPlanBackupId = ''
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
$connection = @{}
foreach ($line in Get-Content -LiteralPath (Join-Path $root '.env.local') -Encoding utf8) {
    $t = $line.Trim(); if (-not $t -or $t.StartsWith('#')) { continue }
    $p = $t -split '=', 2; if ($p.Count -eq 2) { $connection[$p[0].Trim()] = $p[1].Trim().Trim('"').Trim("'") }
}
$dest = '{0}@{1}' -f $connection.WAZI_DEV_SSH_USER, $connection.WAZI_DEV_HOST
$scpOpts = @('-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ForwardAgent=no', '-i', $connection.WAZI_DEV_SSH_KEY_PATH, '-P', '22')
$action = @{install = 'install'; upgrade = 'upgrade'; restore = 'destructive-storage'; rollback = 'rollback'; uninstall = 'uninstall'; backup = 'backup'}[$Operation]
if (-not $action) { throw "unsupported lifecycle operation $Operation" }
$releaseName = (Split-Path (Split-Path $ReleaseDir -Parent) -Leaf) + "-" + (Split-Path $ReleaseDir -Leaf)
$jobName = $Operation + "-" + $releaseName
$jobDir = Join-Path $root ("artifacts\phase1-closure\pilot-jobs\" + $jobName)

function Invoke-Host([string]$Command) {
    $out = & .\scripts\ssh-dev.ps1 $Command
    if ($LASTEXITCODE -ne 0) { throw "host command failed ($LASTEXITCODE): $Command`n$out" }
    return $out
}
function Copy-ToHost([string]$Local, [string]$Remote) {
    & scp @scpOpts $Local ('{0}:{1}' -f $dest, $Remote)
    if ($LASTEXITCODE -ne 0) { throw "scp failed: $Local" }
}
function Copy-FromHost([string]$Remote, [string]$Local) {
    & scp @scpOpts ('{0}:{1}' -f $dest, $Remote) $Local
    if ($LASTEXITCODE -ne 0) { throw "scp download failed: $Remote" }
}

# 1. Fresh runtime profile. Stage the host helper first: /tmp does not
# survive a reboot, and the profile step already needs it.
Copy-ToHost 'scripts\bera-003\submit-pilot-job.sh' '/tmp/submit-pilot-job.sh'
Invoke-Host 'sudo -n runuser -u wazi-runtime -- env HOME=/srv/wazi-runtime/home XDG_RUNTIME_DIR=/run/user/1002 DOCKER_CONTEXT=rootless PATH=/usr/bin:/bin /opt/wazi-dev/20261030T201437Z/wazi-agent -tenant tenant_live_20261030T064834Z -host-id host_bera-003 -state-dir /srv/wazi-runtime/probe-test -cert /etc/wazi-phase1/identity/node.crt -key /etc/wazi-phase1/identity/node.key -ca /etc/wazi-phase1/identity/ca.crt -issuer-ca /etc/wazi-phase1/identity/issuer.crt -controlplane https://localhost:9443 -server-name localhost' | Out-Null
Invoke-Host 'sudo -n bash /tmp/submit-pilot-job.sh get-profile' | Out-Null
Copy-FromHost '/tmp/pilot-profiles.json' (Join-Path $env:TEMP 'pilot-profiles.json')
& .venv\Scripts\python.exe -c "import json; d=json.load(open(r'$env:TEMP\pilot-profiles.json')); json.dump(d['host_bera-003'], open(r'$env:TEMP\profile-runtime.json','w'), sort_keys=True)"
if ($LASTEXITCODE -ne 0) { throw 'profile extraction failed' }

# 2. Build records. The owner-pinned budget (pilot-agent config) is shared.
New-Item -ItemType Directory -Force $jobDir | Out-Null
$pinnedBudget = Join-Path $root 'artifacts\phase1-closure\pilot-jobs\install-b\budget.json'
if (Test-Path $pinnedBudget) {
    Copy-Item $pinnedBudget (Join-Path $jobDir 'budget.json') -Force
}
& .venv\Scripts\python.exe scripts\bera-003\build_pilot_job.py --operation $Operation --release-dir $ReleaseDir --host-profile (Join-Path $env:TEMP 'profile-runtime.json') --facts $FactsFile --out-dir $jobDir --rollback-procedure (Join-Path $root 'artifacts\phase1-closure\pilot-jobs\rollback-procedure\procedure-rollback.json') --case-id $CaseId
if ($LASTEXITCODE -ne 0) { throw 'job builder failed' }

# 3. Canonical approval request.
& .venv\Scripts\python.exe -c "import json,pathlib,sys; d=pathlib.Path(sys.argv[1]); req={'caseId':sys.argv[3],'plan':json.loads((d/'plan.json').read_text()),'envelope':json.loads((d/'envelope.json').read_text()),'hostId':'host_bera-003','action':sys.argv[2],'decision':'approved','validForSeconds':900}; (d/'approval-request.json').write_text(json.dumps(req,sort_keys=True,separators=(',',':')))" $jobDir $action $CaseId
if ($LASTEXITCODE -ne 0) { throw 'approval request failed' }

# 4. Fresh signed revocation (monotonic sequence).
& .venv\Scripts\python.exe -c "import json,pathlib,datetime; base=pathlib.Path('artifacts/phase1-closure/revocation-signer'); seqfile=base/'sequence.txt'; seq=int(seqfile.read_text())+1; seqfile.write_text(str(seq)); now=datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0); rec={'schemaVersion':'1.0.0','listId':'revlist_gitea_pilot','generatedAt':now.strftime('%Y-%m-%dT%H:%M:%SZ'),'expiresAt':(now+datetime.timedelta(hours=2)).strftime('%Y-%m-%dT%H:%M:%SZ'),'entries':[],'sequence':seq}; (base/'record/revocation-list.unsigned.json').write_text(json.dumps(rec,sort_keys=True,separators=(',',':')))"
$digest = & C:\Users\Austi\AppData\Local\Temp\recordtool.exe digest artifacts\phase1-closure\revocation-signer\record\revocation-list.unsigned.json
[IO.File]::WriteAllText("$PWD\artifacts\phase1-closure\revocation-signer\record\payloadDigest.txt", $digest, [Text.Encoding]::ASCII)
# The revocation signing key lives outside the workspace; only the
# public key, sequence counter, and signed records stay in artifacts.
$revocationKey = Join-Path $env:USERPROFILE '.wazi-local\wazi-phase1-revocation.key'
if (-not (Test-Path $revocationKey)) { throw 'revocation signing key unavailable outside workspace' }
$sig = & C:\Users\Austi\AppData\Local\Temp\revsigner\revsigner.exe sign $revocationKey artifacts\phase1-closure\revocation-signer\record\payloadDigest.txt
& .venv\Scripts\python.exe -c "import json,pathlib,datetime,sys; base=pathlib.Path('artifacts/phase1-closure/revocation-signer'); rec=json.loads((base/'record/revocation-list.unsigned.json').read_text()); now=datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).strftime('%Y-%m-%dT%H:%M:%SZ'); rec['signature']={'algorithm':'sigstore-cosign','signedBy':'wazi-phase1-revocation-20261030','signedAt':now,'payloadDigest':sys.argv[2],'value':sys.argv[1]}; (base/'record/revocation-list.json').write_text(json.dumps(rec,sort_keys=True,separators=(',',':')))" $sig $digest
& C:\Users\Austi\AppData\Local\Temp\recordtool.exe canonicalize artifacts\phase1-closure\revocation-signer\record\revocation-list.json artifacts\phase1-closure\revocation-signer\record\revocation-list.json

# 5. Publish, approve, assemble, submit.
Copy-ToHost 'scripts\bera-003\submit-pilot-job.sh' '/tmp/submit-pilot-job.sh'
Copy-ToHost 'artifacts\phase1-closure\revocation-signer\record\revocation-list.json' '/tmp/revocation-list.json'
Copy-ToHost (Join-Path $jobDir 'approval-request.json') '/tmp/approval-request.json'
Invoke-Host 'sudo -n bash /tmp/submit-pilot-job.sh revocation /tmp/revocation-list.json' | Out-Null
$approvalRaw = Invoke-Host 'sudo -n bash /tmp/submit-pilot-job.sh approve /tmp/approval-request.json'
$approvalId = ($approvalRaw | ConvertFrom-Json).approvalId
if (-not $approvalId) { throw "approval not issued: $approvalRaw" }
& .venv\Scripts\python.exe scripts\bera-003\build_pilot_job.py --assemble --out-dir $jobDir --approval-id $approvalId --action $action --job-id $JobId
if ($LASTEXITCODE -ne 0) { throw 'job assembly failed' }
if ($TamperPlanBackupId) {
    & .venv\Scripts\python.exe -c "import json,pathlib,sys; p=pathlib.Path(sys.argv[1]); d=json.loads(p.read_text()); [s['parameters'].__setitem__('backupId', sys.argv[2]) for s in d['plan']['steps'] if s['capability']=='volume.backup']; p.write_text(json.dumps(d,sort_keys=True,separators=(',',':')))" (Join-Path $jobDir 'job.json') $TamperPlanBackupId
    if ($LASTEXITCODE -ne 0) { throw 'adversarial tamper failed' }
    Write-Output "adversarial: plan backupId tampered after approval"
}
Copy-ToHost (Join-Path $jobDir 'job.json') '/tmp/pilot-job.json'
$submit = Invoke-Host 'sudo -n bash /tmp/submit-pilot-job.sh submit /tmp/pilot-job.json'
Write-Output $submit

# 6. Poll to terminal state.
$deadline = (Get-Date).AddSeconds($PollSeconds)
do {
    Start-Sleep -Seconds 15
    $statusRaw = Invoke-Host "sudo -n bash /tmp/submit-pilot-job.sh status $JobId"
    $status = $statusRaw | ConvertFrom-Json
    Write-Output ("{0} {1} {2}" -f $JobId, $status.state, $status.detail)
    if ($status.state -in @('completed', 'rejected-or-reconciliation-required', 'failed')) { break }
} while ((Get-Date) -lt $deadline)
