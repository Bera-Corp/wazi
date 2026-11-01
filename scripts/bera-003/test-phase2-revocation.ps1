[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$RemoteDirectory)
$ErrorActionPreference='Stop'
if($RemoteDirectory -notmatch '^/home/wazi-dev/wazi-phase2/[0-9]{8}T[0-9]{6}Z$'){throw 'Invalid stage directory'}
function Publish-Probe([string]$mode,[string]$check){
    & ./scripts/ssh-dev.ps1 -BashScriptPath scripts/bera-003/test-phase2-package.sh -BashArguments @($mode,$RemoteDirectory) -BashAsRoot
    if($LASTEXITCODE -ne 0){throw 'Revocation probe preparation failed'}
    $raw=& ./scripts/ssh-dev.ps1 ('sudo -n cat '+$RemoteDirectory+'/package-evidence/revocation-list.unsigned.json')
    if($LASTEXITCODE -ne 0){throw 'Revocation record read failed'}
    $directory=Join-Path (Get-Location) ('artifacts/phase2/'+$mode+'-'+(Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ'))
    New-Item -ItemType Directory -Path $directory -Force|Out-Null
    [IO.File]::WriteAllText((Join-Path $directory 'revocation-list.unsigned.json'),($raw -join [char]10),(New-Object Text.UTF8Encoding($false)))
    & ./scripts/sign-phase1-records.ps1 -RecordDirectory $directory -CosignPath artifacts/phase1-closure/tools/source-build/cosign.exe -RecordToolPath artifacts/phase2/recordtool.exe
    if($LASTEXITCODE -ne 0){throw 'Revocation probe signing failed'}
    $connection=@{}
    foreach($line in Get-Content .env.local -Encoding utf8){$parts=$line -split '=',2;if($parts.Count -eq 2){$connection[$parts[0]]=$parts[1].Trim().Trim('"').Trim("'")}}
    $options=@('-o','BatchMode=yes','-o','IdentitiesOnly=yes','-o','StrictHostKeyChecking=yes','-i',$connection.WAZI_DEV_SSH_KEY_PATH)
    & scp @options (Join-Path $directory 'revocation-list.json') ('{0}@{1}:{2}/revocation-list.json' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST,$RemoteDirectory)
    if($LASTEXITCODE -ne 0){throw 'Revocation probe upload failed'}
    & ./scripts/ssh-dev.ps1 -BashScriptPath scripts/bera-003/test-phase2-package.sh -BashArguments @($check,$RemoteDirectory) -BashAsRoot
    if($LASTEXITCODE -ne 0){throw 'Revocation probe check failed'}
}
try { Publish-Probe 'export-test-revocation' 'check-revoked-denial' }
finally { Publish-Probe 'export-clear-revocation' 'check-revocation-cleared' }
