<#
.SYNOPSIS
Connect to the Wazi dev host using metadata from ignored .env.local.
.DESCRIPTION
No environment-file expressions are evaluated. SSH uses the local agent and
key file with strict host-key checking. This is a developer bootstrap tool,
not a Wazi runtime capability or a production credential channel.
.EXAMPLE
.\scripts\ssh-dev.ps1 -Check
.EXAMPLE
.\scripts\ssh-dev.ps1 'uname -sm'
#>
[CmdletBinding()]
param(
    [switch]$Check,
    [string]$BashScriptPath,
    [string[]]$BashArguments = @(),
    [switch]$BashAsRoot,
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)]
    [string[]]$RemoteCommand
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $repoRoot '.env.local'
if (-not (Test-Path -LiteralPath $configPath)) {
    throw 'Create .env.local from .env.example before connecting.'
}

$connection = @{}
foreach ($line in Get-Content -LiteralPath $configPath) {
    $trimmed = $line.Trim()
    if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }
    $parts = $trimmed -split '=', 2
    if ($parts.Count -ne 2) { throw 'Invalid connection metadata line in .env.local.' }
    $connection[$parts[0].Trim()] = $parts[1].Trim().Trim('"').Trim("'")
}
foreach ($field in @('WAZI_DEV_HOST', 'WAZI_DEV_SSH_USER', 'WAZI_DEV_SSH_KEY_PATH', 'WAZI_DEV_EXPECTED_HOST')) {
    if (-not $connection[$field]) { throw "Missing $field in .env.local." }
}
if ($connection.WAZI_DEV_HOST -notmatch '^[a-zA-Z0-9][a-zA-Z0-9.:-]*$' -or
    $connection.WAZI_DEV_SSH_USER -notmatch '^[a-z_][a-z0-9_-]*$') {
    throw 'Invalid SSH hostname or username in .env.local.'
}
if ($connection.WAZI_DEV_EXPECTED_HOST -notmatch '^[a-zA-Z0-9][a-zA-Z0-9.-]*$') {
    throw 'Invalid expected hostname in .env.local.'
}
if (-not (Test-Path -LiteralPath $connection.WAZI_DEV_SSH_KEY_PATH -PathType Leaf)) {
    throw 'The configured SSH key file is unavailable on this machine.'
}
$sshPort = 22
if ($connection.WAZI_DEV_SSH_PORT) {
    if (-not [int]::TryParse($connection.WAZI_DEV_SSH_PORT, [ref]$sshPort) -or
        $sshPort -lt 1 -or $sshPort -gt 65535) { throw 'Invalid SSH port.' }
}

$sshOptions = @(
    '-o', 'BatchMode=yes',
    '-o', 'IdentitiesOnly=yes',
    '-o', 'StrictHostKeyChecking=yes',
    '-o', 'ForwardAgent=no',
    '-o', 'ConnectTimeout=10',
    '-o', 'ServerAliveInterval=15',
    '-o', 'ServerAliveCountMax=3',
    '-p', "$sshPort",
    '-i', $connection.WAZI_DEV_SSH_KEY_PATH
)
$destination = '{0}@{1}' -f $connection.WAZI_DEV_SSH_USER, $connection.WAZI_DEV_HOST
if ($BashScriptPath) {
    if ($Check -or $RemoteCommand) { throw 'Choose a Bash script or a remote command, not both.' }
    $scriptPath = (Resolve-Path -LiteralPath $BashScriptPath -ErrorAction Stop).Path
    $repoPrefix = [IO.Path]::GetFullPath($repoRoot).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $scriptPath.StartsWith($repoPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Bash script must be inside the Wazi workspace.'
    }
    foreach ($argument in $BashArguments) {
        if ($argument -notmatch '^[A-Za-z0-9_./:=+-]+$') {
            throw 'Bash script arguments may contain only letters, digits, underscore, dot, slash, colon, equals, plus, and hyphen.'
        }
    }
    $scriptText = [IO.File]::ReadAllText($scriptPath, [Text.UTF8Encoding]::new($false, $true))
    if ($scriptText.StartsWith([char]0xFEFF)) { $scriptText = $scriptText.Substring(1) }
    $scriptText = [regex]::Replace($scriptText, "`r`n?", "`n")
    if (-not $scriptText.EndsWith("`n")) { $scriptText += "`n" }
    $argumentSuffix = ''
    if ($BashArguments.Count -gt 0) { $argumentSuffix = ' -- ' + ($BashArguments -join ' ') }
    $commandText = 'bash -s' + $argumentSuffix
    if ($BashAsRoot) { $commandText = 'sudo -n ' + $commandText }

    $sshCommand = (Get-Command ssh -CommandType Application -ErrorAction Stop).Source
    $processInfo = [Diagnostics.ProcessStartInfo]::new()
    $processInfo.FileName = $sshCommand
    $processInfo.UseShellExecute = $false
    $processInfo.RedirectStandardInput = $true
    $processInfo.StandardInputEncoding = [Text.UTF8Encoding]::new($false)
    foreach ($option in $sshOptions) { [void]$processInfo.ArgumentList.Add($option) }
    [void]$processInfo.ArgumentList.Add($destination)
    [void]$processInfo.ArgumentList.Add($commandText)
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $processInfo
    if (-not $process.Start()) { throw 'Could not start the SSH script transport.' }
    try {
        $process.StandardInput.Write($scriptText)
    } finally {
        $process.StandardInput.Close()
    }
    $process.WaitForExit()
    $scriptExitCode = $process.ExitCode
    $process.Dispose()
    exit $scriptExitCode
} elseif ($Check) {
    if ($RemoteCommand) { throw 'Choose -Check or a remote command, not both.' }
    $expectedHost = $connection.WAZI_DEV_EXPECTED_HOST
    $commandText = "set -e; test `"`$(hostname)`" = '$expectedHost'; id; sudo -n id; uname -sm"
} elseif ($RemoteCommand) {
    if ($BashArguments.Count -gt 0 -or $BashAsRoot) { throw 'Bash script options require -BashScriptPath.' }
    $commandText = $RemoteCommand -join ' '
} else {
    if ($BashArguments.Count -gt 0 -or $BashAsRoot) { throw 'Bash script options require -BashScriptPath.' }
    $commandText = 'hostname'
}
& ssh @sshOptions $destination $commandText
exit $LASTEXITCODE
