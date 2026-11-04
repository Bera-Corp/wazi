[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$assetRoot = Join-Path $repoRoot 'services\internal\controlplane\assets'
$wordmarkPath = Join-Path $assetRoot 'wazi-wordmark-open.png'
$faviconPath = Join-Path $assetRoot 'favicon.ico'
$guidePath = Join-Path $repoRoot 'graphics\README.md'
$outputPath = Join-Path $assetRoot 'wazi-brand-assets.zip'

foreach ($path in @($wordmarkPath, $faviconPath, $guidePath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Required brand asset is missing: $path"
    }
}

if (-not (Get-Command Compress-Archive -ErrorAction SilentlyContinue)) {
    throw 'Compress-Archive is unavailable.'
}

$tempBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$stagingPath = Join-Path $tempBase ('wazi-brand-kit-' + [Guid]::NewGuid().ToString('N'))
[void](New-Item -ItemType Directory -Path $stagingPath)

try {
    Copy-Item -LiteralPath $wordmarkPath -Destination $stagingPath
    Copy-Item -LiteralPath $faviconPath -Destination $stagingPath
    Copy-Item -LiteralPath $guidePath -Destination (Join-Path $stagingPath 'README.md')

    Compress-Archive -Path (Join-Path $stagingPath '*') -DestinationPath $outputPath -CompressionLevel Optimal -Force
    $archive = [IO.Compression.ZipFile]::OpenRead($outputPath)
    try {
        $requiredNames = @('favicon.ico', 'README.md', 'wazi-wordmark-open.png')
        $archiveNames = @($archive.Entries | ForEach-Object { $_.FullName })
        foreach ($name in $requiredNames) {
            if ($archiveNames -notcontains $name) {
                throw "Brand package is missing $name"
            }
        }
    } finally {
        $archive.Dispose()
    }

    Write-Output "Created brand package: $outputPath"
} finally {
    $resolvedBase = [IO.Path]::GetFullPath($tempBase).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $resolvedStage = [IO.Path]::GetFullPath($stagingPath)
    if (-not $resolvedStage.StartsWith($resolvedBase, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Split-Path -Leaf $resolvedStage).StartsWith('wazi-brand-kit-', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Refusing to remove an unexpected brand-package staging directory.'
    }
    Remove-Item -LiteralPath $resolvedStage -Recurse -Force
}
