# Copyright 2026 The Gitea Authors. All rights reserved.
# SPDX-License-Identifier: MIT
#
# install.ps1 downloads bh.exe for this system from the BranchSpire CLI releases, verifies its
# SHA-256 checksum, installs it and adds its directory to the user's PATH.
#
#   irm https://gitlab.com/aayushpokhrel49/branchspire-cli/-/raw/main/scripts/install.ps1 | iex
#   $env:GITEA_CLI_VERSION = "nightly"; irm https://gitlab.com/aayushpokhrel49/branchspire-cli/-/raw/main/scripts/install.ps1 | iex
#
# Environment variables:
#   GITEA_CLI_VERSION       version to install, such as 1.0.0, or nightly for the latest build of
#                           the main branch (default: the latest release)
#   GITEA_CLI_INSTALL_DIR   directory to install bh.exe into (default: %LOCALAPPDATA%\Programs\branchspire-cli)
#   GITEA_CLI_DOWNLOAD_URL  base URL of the release files (default: https://github.com/aayushpokhrel49/branchspire-cli/releases/download)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$downloadUrl = if ($env:GITEA_CLI_DOWNLOAD_URL) { $env:GITEA_CLI_DOWNLOAD_URL } else { 'https://github.com/aayushpokhrel49/branchspire-cli/releases/download' }
$installDir = if ($env:GITEA_CLI_INSTALL_DIR) { $env:GITEA_CLI_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\branchspire-cli' }

$version = $env:GITEA_CLI_VERSION
if (-not $version) {
    try {
        $version = (Invoke-RestMethod 'https://gitlab.com/api/v4/projects/aayushpokhrel49%2Fbranchspire-cli/releases/latest').tag_name
    } catch {
        throw "cannot read the latest release; set GITEA_CLI_VERSION"
    }
}
$version = $version -replace '^v', ''
# Nightly builds of the main branch are published as version main-nightly.
if ($version -eq 'nightly') { $version = 'main-nightly' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "unsupported architecture $($env:PROCESSOR_ARCHITECTURE); download bh from $downloadUrl" }
}
$file = "branchspire-cli-$version-windows-$arch.exe"
$work = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    Write-Host "Downloading bh $version ($file)"
    $binary = Join-Path $work $file
    Invoke-WebRequest -UseBasicParsing -Uri "$downloadUrl/$version/$file" -OutFile $binary
    Invoke-WebRequest -UseBasicParsing -Uri "$downloadUrl/$version/$file.sha256" -OutFile "$binary.sha256"
    $expected = ((Get-Content -Raw "$binary.sha256") -split '\s+')[0]
    if ((Get-FileHash -Algorithm SHA256 $binary).Hash -ne $expected.ToUpperInvariant()) {
        throw "checksum mismatch for $file; not installing"
    }
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    Move-Item -Force $binary (Join-Path $installDir 'bh.exe')
} finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
Write-Host "Installed bh $version to $(Join-Path $installDir 'bh.exe')"

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable('Path', ($(if ($userPath) { "$userPath;" } else { '' }) + $installDir), 'User')
    $env:Path = "$env:Path;$installDir"
    Write-Host "Added $installDir to your PATH; open a new terminal to run bh"
}
