[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$RemoteDirectory)
$ErrorActionPreference='Stop'
if ($RemoteDirectory -notmatch '^/home/wazi-dev/wazi-phase2/[0-9]{8}T[0-9]{6}Z$') {throw 'Invalid stage directory'}
$sourceCheck=& ./scripts/ssh-dev.ps1 ('test -d '+$RemoteDirectory+' && test ! -e '+$RemoteDirectory+'/source.tar.gz && test ! -e '+$RemoteDirectory+'/releases')
if($LASTEXITCODE -ne 0){throw 'Use a fresh evidence directory without an existing source archive or release tree'}
& ./scripts/ssh-dev.ps1 -BashScriptPath scripts/bera-003/check-phase2-context.sh -BashArguments @('export-revocation',$RemoteDirectory) -BashAsRoot
if($LASTEXITCODE -ne 0){throw 'Revocation export failed'}
$raw=& ./scripts/ssh-dev.ps1 ('cat '+$RemoteDirectory+'/live/revocation-list.unsigned.json')
if($LASTEXITCODE -ne 0){throw 'Revocation transfer failed'}
$directory=Join-Path (Get-Location) ('artifacts/phase2/revocation-'+(Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ'))
New-Item -ItemType Directory -Path $directory -Force | Out-Null
[IO.File]::WriteAllText((Join-Path $directory 'revocation-list.unsigned.json'),($raw -join [char]10),(New-Object Text.UTF8Encoding($false)))
& ./scripts/sign-phase1-records.ps1 -RecordDirectory $directory -CosignPath artifacts/phase1-closure/tools/source-build/cosign.exe -RecordToolPath artifacts/phase2/recordtool.exe
if($LASTEXITCODE -ne 0){throw 'Revocation signing failed'}
$archive=Join-Path $directory 'source.tar.gz'
& git -c core.autocrlf=false -c core.eol=lf archive --format=tar.gz -o $archive HEAD services scripts schemas context evals releases requirements-dev.txt
if($LASTEXITCODE -ne 0){throw 'Committed source archive failed'}
$archiveHash=(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
$sourceCommit=(& git rev-parse HEAD).Trim()
if($LASTEXITCODE -ne 0){throw 'Committed source identity failed'}
$identityPath=Join-Path $directory 'source-identity.json'
$identity=[ordered]@{remote=$RemoteDirectory;sourceSha256=$archiveHash;sourceCommit=$sourceCommit;committedSource=$true;recordedAt=(Get-Date).ToUniversalTime().ToString('o')}
[IO.File]::WriteAllText($identityPath,(($identity|ConvertTo-Json -Depth 5)+[char]10),[Text.UTF8Encoding]::new($false))
$connection=@{}
foreach($line in Get-Content .env.local -Encoding utf8){
    $parts=$line -split '=',2
    if($parts.Count -eq 2){$connection[$parts[0]]=$parts[1].Trim().Trim('"').Trim("'")}
}
if($connection.WAZI_DEV_EXPECTED_HOST -ne 'bera-003'){throw 'Unexpected host'}
$port='22'
if($connection.WAZI_DEV_SSH_PORT){$port=$connection.WAZI_DEV_SSH_PORT}
$options=@('-o','BatchMode=yes','-o','IdentitiesOnly=yes','-o','StrictHostKeyChecking=yes','-o','ForwardAgent=no','-P',$port,'-i',$connection.WAZI_DEV_SSH_KEY_PATH)
& scp @options $archive ('{0}@{1}:{2}/source.tar.gz' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST,$RemoteDirectory)
if($LASTEXITCODE -ne 0){throw 'Committed source upload failed'}
& scp @options $identityPath ('{0}@{1}:{2}/source-identity.json' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST,$RemoteDirectory)
if($LASTEXITCODE -ne 0){throw 'Source identity upload failed'}
& scp @options (Join-Path $directory 'revocation-list.json') ('{0}@{1}:{2}/revocation-list.json' -f $connection.WAZI_DEV_SSH_USER,$connection.WAZI_DEV_HOST,$RemoteDirectory)
if($LASTEXITCODE -ne 0){throw 'Revocation upload failed'}
$remoteArchive=& ./scripts/ssh-dev.ps1 ('sha256sum '+$RemoteDirectory+'/source.tar.gz')
if($LASTEXITCODE -ne 0){throw 'Remote source archive hash failed'}
$remoteHash=(($remoteArchive -join '') -split '\s+')[0]
if($remoteHash -ne $archiveHash){throw 'Remote source archive digest mismatch'}
$unpack=& ./scripts/ssh-dev.ps1 ('cd '+$RemoteDirectory+' && tar -xzf source.tar.gz && test -f releases/phase2/nginx/manifest.json')
if($LASTEXITCODE -ne 0){throw 'Remote committed source extraction failed'}
& ./scripts/ssh-dev.ps1 -BashScriptPath scripts/bera-003/test-phase2-package.sh -BashArguments @('publish',$RemoteDirectory) -BashAsRoot
if($LASTEXITCODE -ne 0){throw 'Signed record publication failed'}
