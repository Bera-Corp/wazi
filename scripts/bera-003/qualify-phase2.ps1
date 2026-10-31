[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
$connection = @{}
foreach ($line in Get-Content -LiteralPath (Join-Path $root '.env.local') -Encoding utf8) {
    $parts = $line.Trim() -split '=',2
    if ($parts.Count -eq 2 -and -not $parts[0].StartsWith('#')) { $connection[$parts[0]] = $parts[1].Trim().Trim('"').Trim("'") }
}
if ($connection.WAZI_DEV_EXPECTED_HOST -ne 'bera-003') { throw 'Unexpected qualification host' }
$stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$local = Join-Path $root ('artifacts/phase2/' + $stamp)
New-Item -ItemType Directory -Path $local -Force | Out-Null
$archive = Join-Path $local 'source.tar.gz'
& tar --exclude=phase1record-temp.go --exclude=__pycache__ -czf $archive services scripts schemas context evals releases requirements-dev.txt
if ($LASTEXITCODE -ne 0) { throw 'Qualification archive failed' }
$digest = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
$remote = '/home/wazi-dev/wazi-phase2/' + $stamp
& ./scripts/ssh-dev.ps1 ('umask 077; mkdir -p ' + $remote)
if ($LASTEXITCODE -ne 0) { throw 'Qualification staging failed' }
$port = '22'
if ($connection.WAZI_DEV_SSH_PORT) { $port = $connection.WAZI_DEV_SSH_PORT }
$destination = '{0}@{1}' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST
$options = @('-o','BatchMode=yes','-o','IdentitiesOnly=yes','-o','StrictHostKeyChecking=yes','-o','ForwardAgent=no','-o','ConnectTimeout=10','-i',$connection.WAZI_DEV_SSH_KEY_PATH,'-P',$port)
& scp @options $archive ('{0}:{1}/source.tar.gz' -f $destination,$remote)
if ($LASTEXITCODE -ne 0) { throw 'Qualification upload failed' }
[ordered]@{remote=$remote;sourceSha256=$digest;recordedAt=(Get-Date).ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $local 'staging.json') -Encoding utf8
& ./scripts/ssh-dev.ps1 -BashScriptPath scripts/bera-003/qualify-phase2.sh -BashArguments @($remote,$digest)
if ($LASTEXITCODE -ne 0) { throw 'Native qualification failed; retain the staging receipt' }
Write-Output ('QUALIFIED_REMOTE=' + $remote)
