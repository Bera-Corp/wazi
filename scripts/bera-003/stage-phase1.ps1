[CmdletBinding()]
param([switch]$IncludeImages)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
$connection=@{}
foreach($line in Get-Content -LiteralPath (Join-Path $root '.env.local') -Encoding utf8){
    $trimmed=$line.Trim()
    if(-not $trimmed -or $trimmed.StartsWith('#')){continue}
    $parts=$trimmed -split '=',2
    if($parts.Count -eq 2){$connection[$parts[0].Trim()]=$parts[1].Trim().Trim('"').Trim("'")}
}
if($connection.WAZI_DEV_EXPECTED_HOST -ne 'bera-003'){throw 'Unexpected target host'}
& (Join-Path $root 'scripts/ssh-dev.ps1') -Check
if($LASTEXITCODE -ne 0){throw 'Verified SSH host check failed'}
$stamp=(Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$local=Join-Path $root ('artifacts/phase1-closure/staging/'+$stamp)
New-Item -ItemType Directory -Force $local | Out-Null
$archive=Join-Path $local 'source.tar.gz'
& tar --exclude=phase1record-temp.go --exclude=__pycache__ -czf $archive services scripts/bera-003 releases/phase1 context/examples schemas
if($LASTEXITCODE -ne 0){throw 'Source staging archive failed'}
$remote='/home/wazi-dev/wazi-phase1-closure/'+$stamp
& (Join-Path $root 'scripts/ssh-dev.ps1') ('umask 077; mkdir -p '+$remote)
if($LASTEXITCODE -ne 0){throw 'Remote staging directory failed'}
$port='22';if($connection.WAZI_DEV_SSH_PORT){$port=$connection.WAZI_DEV_SSH_PORT}
$destination='{0}@{1}' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST
$options=@('-o','BatchMode=yes','-o','IdentitiesOnly=yes','-o','StrictHostKeyChecking=yes','-o','ForwardAgent=no','-o','ConnectTimeout=10','-i',$connection.WAZI_DEV_SSH_KEY_PATH,'-P',$port)
& scp @options $archive ('{0}:{1}/source.tar.gz' -f $destination,$remote)
if($LASTEXITCODE -ne 0){throw 'Source upload failed'}
$digest=(Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
if($IncludeImages){
    foreach($name in @('a','b')){
        $folder=Join-Path $root ('artifacts/phase1-closure/images/'+$name)
        $files=@(Get-ChildItem -LiteralPath $folder -Recurse -Filter gitea-image.tar -File)
        if($files.Count -ne 1){throw 'Expected exactly one verified image archive'}
        & scp @options $files[0].FullName ('{0}:/home/wazi-dev/wazi-phase1-closure/image-{1}.tar' -f $destination,$name)
        if($LASTEXITCODE -ne 0){throw 'Image archive upload failed'}
    }
}
[ordered]@{remote=$remote;sourceSha256=$digest;recordedAt=(Get-Date).ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content (Join-Path $local 'staging.json') -Encoding utf8
Write-Output ('STAGED_REMOTE='+$remote)
Write-Output ('STAGED_SHA256='+$digest)
Write-Output ('STAGED_EVIDENCE='+$local)
