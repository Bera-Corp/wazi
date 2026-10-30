[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$RecordDirectory,
    [Parameter(Mandatory=$true)][string]$CosignPath,
    [Parameter(Mandatory=$true)][string]$RecordToolPath
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$directory = (Resolve-Path -LiteralPath $RecordDirectory).Path
$cosign = (Resolve-Path -LiteralPath $CosignPath).Path
$recordTool = (Resolve-Path -LiteralPath $RecordToolPath).Path
$python = Join-Path $root '.venv/Scripts/python.exe'
$signerHome = Join-Path $env:LOCALAPPDATA 'Wazi/phase1-pilot-signer-20261030'
$key = Join-Path $signerHome 'wazi-phase1-pilot.key'
$publicKey = Join-Path $signerHome 'wazi-phase1-pilot.pub'
$policy = Join-Path $root 'releases/phase1/gitea/28.2.0/signing-policy.json'
foreach ($file in @($key,$publicKey,$python,$policy)) {
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Missing required file: $file" }
}
$secure = Get-Content -LiteralPath (Join-Path $signerHome 'cosign-password.dpapi') -Raw -Encoding utf8 | ConvertTo-SecureString
$handle = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
$previous = $env:COSIGN_PASSWORD
$finalize = @'
import datetime,json,pathlib,sys
source,payload,signature,output=sys.argv[1:]
value=json.loads(pathlib.Path(source).read_text(encoding='utf-8-sig'))
value['signature']={'algorithm':'sigstore-cosign','signedBy':'wazi-phase1-demo-pilot-20261030','signedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds').replace('+00:00','Z'),'payloadDigest':pathlib.Path(payload).read_text(encoding='ascii'),'value':pathlib.Path(signature).read_text(encoding='ascii').strip()}
pathlib.Path(output).write_text(json.dumps(value,ensure_ascii=False,sort_keys=True,separators=(',',':')),encoding='utf-8')
'@
try {
    [Environment]::SetEnvironmentVariable('COSIGN_PASSWORD',[Runtime.InteropServices.Marshal]::PtrToStringBSTR($handle),'Process')
    foreach ($source in Get-ChildItem -LiteralPath $directory -Filter '*.unsigned.json' -File) {
        $stem = $source.Name.Replace('.unsigned.json','')
        $payload = Join-Path $directory ($stem+'.payloadDigest.txt')
        $signature = Join-Path $directory ($stem+'.sig')
        $output = Join-Path $directory ($stem+'.json')
        $digest = & $recordTool digest $source.FullName
        if ($LASTEXITCODE -ne 0) { throw 'Payload digest failed' }
        [IO.File]::WriteAllText($payload,$digest,[Text.Encoding]::ASCII)
        & $cosign sign-blob --key $key --new-bundle-format=false --use-signing-config=false --tlog-upload=false --output-signature $signature $payload
        if ($LASTEXITCODE -ne 0) { throw "Signing failed for $stem" }
        & $cosign verify-blob --key $publicKey --signature $signature --insecure-ignore-tlog $payload
        if ($LASTEXITCODE -ne 0) { throw "Cosign verification failed for $stem" }
        & $python -c $finalize $source.FullName $payload $signature $output
        if ($LASTEXITCODE -ne 0) { throw 'Record assembly failed' }
        & $recordTool canonicalize $output $output
        if ($LASTEXITCODE -ne 0) { throw 'Canonicalization failed' }
        & $recordTool verify $output $policy
        if ($LASTEXITCODE -ne 0) { throw "Wazi verification failed for $stem" }
    }
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($handle)
    if ($null -eq $previous) { Remove-Item Env:COSIGN_PASSWORD -ErrorAction SilentlyContinue }
    else { $env:COSIGN_PASSWORD = $previous }
}
