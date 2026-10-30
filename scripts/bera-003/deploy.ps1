[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $repoRoot
$connection = @{}
foreach ($line in Get-Content -LiteralPath (Join-Path $repoRoot '.env.local') -Encoding utf8) {
    $trimmed = $line.Trim()
    if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }
    $parts = $trimmed -split '=', 2
    if ($parts.Count -eq 2) { $connection[$parts[0].Trim()] = $parts[1].Trim().Trim('"').Trim("'") }
}
if ($connection.WAZI_DEV_EXPECTED_HOST -ne 'bera-003') { throw 'This deployment is only for bera-003.' }
$sshPort = 22
if ($connection.WAZI_DEV_SSH_PORT) { $sshPort = [int]$connection.WAZI_DEV_SSH_PORT }
$destination = '{0}@{1}' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST
$common = @('-o','BatchMode=yes','-o','IdentitiesOnly=yes','-o','StrictHostKeyChecking=yes','-o','ForwardAgent=no','-o','ConnectTimeout=10','-i',$connection.WAZI_DEV_SSH_KEY_PATH)
$stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$evidence = Join-Path $repoRoot "artifacts/bera-003/$stamp"
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$utf8 = New-Object System.Text.UTF8Encoding($false)
foreach ($name in @('deploy.sh','live_checks.py')) {
    $path = Join-Path $PSScriptRoot $name
    $content = [IO.File]::ReadAllText($path).Replace(([char]13).ToString(), '').TrimEnd() + [char]10
    [IO.File]::WriteAllText($path,$content,$utf8)
}
$manifest = @(Get-ChildItem -LiteralPath services -Recurse -File | Sort-Object FullName | ForEach-Object {
    [ordered]@{path=$_.FullName.Substring($repoRoot.Length+1).Replace('\','/');sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower()}
})
$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $evidence 'source-manifest.json') -Encoding utf8
$archive = Join-Path $evidence 'source.tar.gz'
& tar --exclude=__pycache__ -czf $archive services scripts/bera-003
if ($LASTEXITCODE -ne 0) { throw 'Source archive failed.' }
$archiveHash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLower()
$remote = "/home/$($connection.WAZI_DEV_SSH_USER)/wazi-live/runs/$stamp"
$remoteCheck = 'umask 077; mkdir -p ''{0}''; test "$(hostname)" = ''bera-003''' -f $remote
& ssh @common -p "$sshPort" $destination $remoteCheck
if ($LASTEXITCODE -ne 0) { throw 'Remote staging failed.' }
& scp @common -P "$sshPort" $archive ('{0}:{1}/source.tar.gz' -f $destination,$remote)
if ($LASTEXITCODE -ne 0) { throw 'Upload failed.' }
& ssh @common -p "$sshPort" $destination "set -eu; cd '$remote'; echo '$archiveHash  source.tar.gz' | sha256sum -c -; tar -xzf source.tar.gz; bash scripts/bera-003/deploy.sh '$stamp'" 2>&1 | Tee-Object -FilePath (Join-Path $evidence 'deploy.log')
$result = $LASTEXITCODE
[ordered]@{run=$stamp;remote=$remote;sourceArchiveSha256=$archiveHash;exitCode=$result} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $evidence 'deployment.json') -Encoding utf8
Write-Output "Evidence: $evidence"
if ($result -ne 0) { throw "Deployment failed with exit $result; inspect retained evidence." }
