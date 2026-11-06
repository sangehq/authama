<#
.SYNOPSIS
  Authentication Access Manager installer for Windows.

.DESCRIPTION
  Installs authama.exe. The sshmgr.exe alias that older releases installed
  beside it is no longer installed; one already there is left alone, and
  named, so it can be removed by hand.

.EXAMPLE
  irm https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.ps1 | iex

.EXAMPLE
  # Pin a version / custom dir (AUTHAMA_*, or the deprecated SSHMGR_*)
  $env:AUTHAMA_VERSION = 'v0.1.0'; irm https://.../install.ps1 | iex
#>
#Requires -Version 5
# Progress lines go to the host, not the pipeline: this runs as `irm | iex`, where
# anything written to the output stream would be echoed back as a result.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '',
  Justification = 'Installer progress for an interactive irm | iex run')]
[CmdletBinding()]
param(
  [string]$Version,
  [string]$InstallDir,
  # Where the executables and checksums.txt are read from instead of the GitHub
  # release: a URL, or a local directory. CI points it at a binary it just built,
  # since binary releases are on hold and the release has none to download.
  [string]$ReleaseBase
)

$ErrorActionPreference = 'Stop'
$owner   = 'simtabi'
$repo    = 'authama'
# Releases up to 0.1.0 installed into Programs\ssh-manager; 0.1.1 and later
# install into Programs\authama and take the old folder off the user PATH (its
# files are left alone, so nothing a user put there is lost).
$dirName    = 'authama'
$oldDirName = 'ssh-manager'
$binBase = 'authama'
# The name releases before 0.1.0 shipped as; only ever reported, never installed.
$legacy  = 'sshmgr'

# AUTHAMA_*, else the deprecated SSHMGR_*, matching the POSIX installer.
foreach ($prefix in 'AUTHAMA', 'SSHMGR') {
  if (-not $Version)    { $Version    = [Environment]::GetEnvironmentVariable("${prefix}_VERSION") }
  if (-not $InstallDir) { $InstallDir = [Environment]::GetEnvironmentVariable("${prefix}_INSTALL_DIR") }
}
if (-not $ReleaseBase) { $ReleaseBase = $env:AUTHAMA_RELEASE_BASE }

# Go-style GOARCH, matching the release artifact names.
switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { $arch = 'amd64' }
  'ARM64' { $arch = 'arm64' }
  'x86'   { $arch = '386' }
  default { throw "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$defaultDir = -not $InstallDir
if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\$dirName" }

if (-not $Version) {
  try {
    $rel = Invoke-RestMethod -Headers @{ 'Accept' = 'application/vnd.github+json' } `
      "https://api.github.com/repos/$owner/$repo/releases/latest"
    $Version = $rel.tag_name
  } catch { $Version = $null }

  # The API allows 60 unauthenticated requests an hour per IP, and anyone behind a
  # shared NAT - or on a CI runner - can find that spent and get a 403. The
  # releases/latest web redirect is not counted against it and names the same tag,
  # so fall back to it rather than failing the install (install.sh does the same).
  # HttpWebRequest with redirects off reads the Location header identically on
  # Windows PowerShell 5.1 and PowerShell 7; a 3xx can surface as an exception on
  # the latter, so its response is read from there too.
  if (-not $Version) {
    $req = [System.Net.HttpWebRequest]::Create("https://github.com/$owner/$repo/releases/latest")
    $req.AllowAutoRedirect = $false
    $req.Method = 'HEAD'
    try { $resp = $req.GetResponse() } catch [System.Net.WebException] { $resp = $_.Exception.Response }
    if ($resp) {
      try { $location = [string]$resp.Headers['Location'] } finally { $resp.Close() }
      if ($location -match '/releases/tag/([^/?#]+)$') { $Version = $Matches[1] }
    }
  }
  if (-not $Version) { throw 'could not determine the version to install.' }
}
if ($Version -notlike 'v*') { $Version = "v$Version" }

# Bare, ready-to-run executables: <name>_windows_<arch>.exe.
$base  = if ($ReleaseBase) { $ReleaseBase } else { "https://github.com/$owner/$repo/releases/download/$Version" }
# Get-Asset fetches one file of the release: by HTTP, or a copy from a local
# directory (Invoke-WebRequest does not read file:// paths).
function Get-Asset([string]$name, [string]$out) {
  if ($base -match '^https?://') { Invoke-WebRequest "$base/$name" -OutFile $out }
  else { Copy-Item -LiteralPath (Join-Path $base $name) -Destination $out -ErrorAction Stop }
}
$tmp   = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
$installed = @()

try {
  $sums = Join-Path $tmp 'checksums.txt'
  $haveSums = $true
  try { Get-Asset 'checksums.txt' $sums } catch { $haveSums = $false; Write-Warning 'checksums.txt not found; skipping verification.' }

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  foreach ($name in @($binBase)) {
    $asset = "${name}_windows_${arch}.exe"
    $exe = Join-Path $tmp $asset
    Write-Host "Downloading $asset ($Version) ..."
    try { Get-Asset $asset $exe }
    catch { throw "$Version has no ${binBase}_windows_${arch}.exe (releases before 0.1.0 shipped as $legacy)" }
    if ($haveSums) {
      # Anchored on the whole filename field, not a substring. checksums.txt lists
      # the binary, its archive and the archive's SBOM, so a bare name like
      # "authama_macos_arm64" appears in three lines; -First 1 on an unanchored match
      # would compare the download against whichever hash came first in the file.
      $line = Select-String -Path $sums -Pattern ('\s' + [regex]::Escape($asset) + '$') | Select-Object -First 1
      if (-not $line) { throw "no checksum entry for $asset" }
      $want = (($line.Line -split '\s+')[0]).ToLower()
      $got  = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
      if ($got -ne $want) { throw "checksum mismatch for $asset" }
    }
    Copy-Item $exe (Join-Path $InstallDir "$name.exe") -Force
    $installed += $name
  }
}
finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# Add the install dir to the user PATH if it isn't already there.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($defaultDir) {
  # An earlier install's folder would shadow or duplicate this one: drop it from
  # the user PATH (only the entry - its files stay where they are).
  $oldDir = Join-Path $env:LOCALAPPDATA "Programs\$oldDirName"
  $parts = @($userPath -split ';' | Where-Object { $_ -and $_ -ne $oldDir })
  if ($parts.Count -ne @($userPath -split ';' | Where-Object { $_ }).Count) {
    $userPath = $parts -join ';'
    [Environment]::SetEnvironmentVariable('Path', $userPath, 'User')
    Write-Host "Removed the old install folder $oldDir from your user PATH (its files are untouched; delete it when you like)."
  }
}
if (($userPath -split ';') -notcontains $InstallDir) {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
  Write-Host "Added $InstallDir to your user PATH (restart your shell to pick it up)."
}
$env:Path += ";$InstallDir"

& (Join-Path $InstallDir "$($installed[0]).exe") version
foreach ($name in $installed) { Write-Host "Installed $name to $InstallDir\$name.exe" }
$old = Join-Path $InstallDir "$legacy.exe"
if (Test-Path $old) {
  Write-Host "An older $legacy.exe is still at $old; $binBase replaces it."
  Write-Host "Remove it with: Remove-Item '$old'  (then: $binBase ssh config render)"
}
Write-Host "Settings live in $env:USERPROFILE\.config\authama (.env and authama.json, loaded at startup);"
Write-Host "a 0.1.0 home in $env:APPDATA\ssh-manager is copied there on the first run and kept."
