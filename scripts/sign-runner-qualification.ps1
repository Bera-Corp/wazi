[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$InputReceipt,
    [Parameter(Mandatory=$true)][string]$OutputReceipt,
    [Parameter(Mandatory=$true)][string]$PublicPolicy
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$cosign=Join-Path $root 'artifacts/phase1-closure/tools/source-build/cosign.exe'
$recordtool=Join-Path $root 'artifacts/phase2/recordtool.exe'
$python=Join-Path $root '.venv/Scripts/python.exe'
foreach($path in @($cosign,$recordtool,$python,$InputReceipt)){
    if(-not(Test-Path -LiteralPath $path -PathType Leaf)){throw 'Required qualification signing input unavailable'}
}
$signerHome=Join-Path $env:LOCALAPPDATA 'Wazi/qualification-signer-20261104'
$fresh=-not(Test-Path -LiteralPath $signerHome)
if($fresh){
    New-Item -ItemType Directory -Path $signerHome | Out-Null
    $account=[Security.Principal.WindowsIdentity]::GetCurrent().Name
    & icacls $signerHome /inheritance:r /grant:r ($account+':(OI)(CI)F') | Out-Null
    if($LASTEXITCODE){throw 'Private qualification directory ACL failed'}
    $bytes=[byte[]]::new(48)
    [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $secure=ConvertTo-SecureString ([Convert]::ToBase64String($bytes)) -AsPlainText -Force
    [Array]::Clear($bytes,0,$bytes.Length)
    $secure | ConvertFrom-SecureString | Set-Content -LiteralPath (Join-Path $signerHome 'cosign-password.dpapi') -Encoding utf8
}
$encoded=(Get-Content -LiteralPath (Join-Path $signerHome 'cosign-password.dpapi') -Raw -Encoding utf8).Trim()
$secure=ConvertTo-SecureString $encoded
$handle=[Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
$previous=$env:COSIGN_PASSWORD
$prefix=Join-Path $signerHome 'wazi-qualification'
try{
    $plain=[Runtime.InteropServices.Marshal]::PtrToStringBSTR($handle)
    $env:COSIGN_PASSWORD=$plain
    if(-not(Test-Path -LiteralPath ($prefix+'.key')) -and -not(Test-Path -LiteralPath ($prefix+'.pub'))){
        & $cosign generate-key-pair --output-key-prefix $prefix
        if($LASTEXITCODE){throw 'Qualification-only key generation failed'}
    }
    foreach($extension in @('.key','.pub')){
        if(-not(Test-Path -LiteralPath ($prefix+$extension))){throw 'Incomplete qualification signer; preserve directory and inspect manually'}
    }
    $buildPolicy=@'
import base64,hashlib,json,pathlib,sys
public=pathlib.Path(sys.argv[1]).read_text(encoding='ascii')
der=base64.b64decode(''.join(public.splitlines()[1:-1]),validate=True)
policy={'schemaVersion':'1.0.0','algorithms':['sigstore-cosign'],'trustedSigners':{'wazi-qualification-20261104':{'keyId':'sha256:'+hashlib.sha256(der).hexdigest(),'publicKeyPem':public}}}
with pathlib.Path(sys.argv[2]).open('x',encoding='utf-8') as output:json.dump(policy,output,sort_keys=True,separators=(',',':'))
'@
    & $python -c $buildPolicy ($prefix+'.pub') $PublicPolicy
    if($LASTEXITCODE){throw 'Public qualification policy creation failed'}
    & $python (Join-Path $root 'scripts/sign_qualification.py') --input $InputReceipt --output $OutputReceipt --key ($prefix+'.key') --signer-id wazi-qualification-20261104 --policy $PublicPolicy --recordtool $recordtool --cosign $cosign
    if($LASTEXITCODE){throw 'Qualification receipt signing or verification failed'}
}finally{
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($handle)
    if($null -eq $previous){Remove-Item Env:COSIGN_PASSWORD -ErrorAction SilentlyContinue}else{$env:COSIGN_PASSWORD=$previous}
}
