# Tunnd client installer for Windows (PowerShell 5.1+)
#
# Usage:
#   iwr -useb https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.ps1 | iex
#
# The installer:
#   1. Downloads the latest tunnd.exe from GitHub Releases
#   2. Verifies the SHA256 checksum
#   3. Installs to %LOCALAPPDATA%\Programs\tunnd\
#   4. Adds that directory to your user PATH (if not already on it)
#
# Override the install path by setting $env:TUNND_INSTALL_DIR before running.

#Requires -Version 5.1
$ErrorActionPreference = 'Stop'

$Repo    = 'elvonpiko/tunnd'
$Binary  = 'tunnd.exe'
$Default = Join-Path $env:LOCALAPPDATA 'Programs\tunnd'
$Install = if ($env:TUNND_INSTALL_DIR) { $env:TUNND_INSTALL_DIR } else { $Default }

# ── Detect architecture ──────────────────────────────────────────────────────
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

# Tunnd does not ship Windows arm64 builds yet — fall back to amd64
# (which runs under emulation on arm64 Windows just fine).
if ($arch -eq 'arm64') {
    Write-Host '  Note: Windows arm64 build is not available; using amd64 (runs via emulation).' -ForegroundColor Yellow
    $arch = 'amd64'
}

Write-Host ''
Write-Host '  Tunnd client installer' -ForegroundColor Cyan
Write-Host ''

# ── Resolve latest release ───────────────────────────────────────────────────
Write-Host '  Fetching latest release...' -NoNewline
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" `
                                  -UseBasicParsing
    $version = $release.tag_name
    if (-not $version) { throw 'no tag_name in response' }
    Write-Host " $version" -ForegroundColor Green
} catch {
    throw "Could not determine latest version: $_"
}

$cleanVersion = $version.TrimStart('v')
$archiveName  = "tunnd_${cleanVersion}_windows_${arch}.zip"
$baseUrl      = "https://github.com/$Repo/releases/download/$version"

# ── Download to a temp directory ─────────────────────────────────────────────
$tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "tunnd-$([Guid]::NewGuid())")
try {
    $zipPath = Join-Path $tmp $archiveName
    $sumsPath = Join-Path $tmp 'checksums.txt'

    Write-Host "  Downloading $archiveName..." -NoNewline
    Invoke-WebRequest -Uri "$baseUrl/$archiveName" -OutFile $zipPath -UseBasicParsing
    Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $sumsPath -UseBasicParsing
    Write-Host ' ok' -ForegroundColor Green

    # ── Verify checksum ──────────────────────────────────────────────────────
    Write-Host '  Verifying checksum...' -NoNewline
    $expected = (Get-Content $sumsPath | Where-Object { $_ -match $archiveName }) `
                -replace '\s.*$' | Select-Object -First 1
    if (-not $expected) { throw "checksum entry for $archiveName not found" }
    $actual = (Get-FileHash -Algorithm SHA256 $zipPath).Hash.ToLower()
    if ($actual -ne $expected.ToLower()) {
        throw "checksum mismatch (expected $expected, got $actual)"
    }
    Write-Host ' ok' -ForegroundColor Green

    # ── Extract and install ──────────────────────────────────────────────────
    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force
    if (-not (Test-Path $Install)) {
        New-Item -ItemType Directory -Path $Install -Force | Out-Null
    }
    Copy-Item -Path (Join-Path $tmp $Binary) -Destination (Join-Path $Install $Binary) -Force
    Write-Host "  Installed to $Install\$Binary" -ForegroundColor Green
}
finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# ── Add to user PATH (if not already on it) ──────────────────────────────────
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$onPath = $userPath -split ';' | ForEach-Object { $_.TrimEnd('\') } |
          Where-Object { $_ -ieq $Install.TrimEnd('\') }

if (-not $onPath) {
    $newPath = if ($userPath) { "$userPath;$Install" } else { $Install }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Write-Host '  Added to user PATH (open a new shell to pick it up).' -ForegroundColor Green
} else {
    Write-Host '  Already on PATH.' -ForegroundColor DarkGray
}

Write-Host ''
Write-Host '  Get started:' -ForegroundColor Cyan
Write-Host '    tunnd setup'
Write-Host ''
