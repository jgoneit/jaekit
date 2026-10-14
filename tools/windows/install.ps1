<#
.SYNOPSIS
Install a verified local Windows x64 Jaekit candidate for the current user.
.DESCRIPTION
Accepts an archive and its checksums from a reviewed candidate. Does not download,
publish, change execution policy, install prerequisites, or request elevation.
AddToPath explicitly permits adding one user PATH entry. Existing entries stay.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Archive,
    [Parameter(Mandatory = $true)][string]$Checksums,
    [switch]$AddToPath
)
. (Join-Path $PSScriptRoot 'common.ps1')

$mutex = $null
$scratch = $null
$backup = $null
$committed = $false
try {
    Assert-WindowsPlatform
    $root = Get-InstallRoot
    $bin = Join-Path $root 'bin'
    $mutex = Enter-InstallLock $root
    $installed = Read-Installation $root
    Assert-ExecutableReplaceable $root
    $parent = [IO.Path]::GetDirectoryName($root)
    $scratch = Join-Path $parent ('.jaekit-install-' + [Guid]::NewGuid().ToString('N'))
    Assert-OrdinaryPath $scratch | Out-Null
    [IO.Directory]::CreateDirectory($scratch) | Out-Null
    $candidate = Join-Path $scratch 'candidate'
    $package = Expand-CheckedPackage $Archive $Checksums $candidate
    $version = $package.manifest.version
    $candidateExe = Join-Path $candidate 'ha.exe'
    $versionOutput = @(& $candidateExe --version)
    if ($LASTEXITCODE -ne 0 -or ($versionOutput -join "`n").Trim() -cne ('ha ' + $version)) {
        throw 'The executable version does not match the candidate manifest.'
    }
    $capsOutput = @(& $candidateExe capabilities --format json)
    if ($LASTEXITCODE -ne 0) { throw 'The candidate executable cannot report its capabilities.' }
    $caps = ($capsOutput -join "`n") | ConvertFrom-Json
    if ($caps.schema -ne 'ha-capabilities/v1' -or $caps.ha_version -cne $version -or @($caps.supported_rules) -notcontains 'run-rules/3') {
        throw 'The candidate executable has unsupported capabilities.'
    }

    $beforePath = Read-UserPath
    $afterPath = $beforePath.value
    $ownsPath = $false
    $createdPath = $false
    $versions = @()
    if ($null -ne $installed) {
        $ownsPath = $installed.state.owns_path
        $createdPath = $installed.state.created_path_value
        $versions = @($installed.state.versions)
    }
    $alreadyOnPath = $false
    foreach ($entry in $beforePath.value.Split(';')) {
        if (Test-PathEntry $entry $bin) { $alreadyOnPath = $true }
    }
    if ($AddToPath -and -not $alreadyOnPath) {
        if ($afterPath) { $afterPath += ';' }
        $afterPath += $bin
        $ownsPath = $true
        if (-not $beforePath.exists) { $createdPath = $true }
    }
    $next = Join-Path $scratch 'next'
    [IO.Directory]::CreateDirectory($next) | Out-Null
    if ($null -ne $installed) { Copy-Inventory $root $next $installed.files }
    $manifestHash = Get-Sha256 (Join-Path $candidate 'package.json')
    $existingVersion = @($versions | Where-Object { $_.version -ceq $version })
    if ($existingVersion.Count -gt 0) {
        if ($existingVersion[0].manifest_sha256 -cne $manifestHash) { throw "Existing version $version contains different data; nothing was replaced." }
    } else {
        $versionDir = Join-Path (Join-Path $next 'versions') $version
        $candidateFiles = @{'package.json' = $manifestHash}
        foreach ($name in $package.files.Keys) { $candidateFiles[$name] = $package.files[$name].sha256 }
        Copy-Inventory $candidate $versionDir $candidateFiles
        $versions += @{version = $version; manifest_sha256 = $manifestHash}
    }
    [IO.Directory]::CreateDirectory((Join-Path $next 'bin')) | Out-Null
    [IO.File]::Copy($candidateExe, (Join-Path $next 'bin\ha.exe'), $true)
    Save-InstallState (Join-Path $next 'managed.json') ([ordered]@{
        schema = 'jaekit-windows-install/v1'; current_version = $version
        versions = @($versions); owns_path = [bool]$ownsPath; created_path_value = [bool]$createdPath
    })
    Read-Installation $next | Out-Null
    # Inspect again immediately before replacement. A changed installation or
    # executable locked by a running check is a failure, never a forced update.
    Read-Installation $root | Out-Null
    Assert-ExecutableReplaceable $root
    if ([IO.Directory]::Exists($root)) {
        $backup = Join-Path $parent ('.jaekit-previous-' + [Guid]::NewGuid().ToString('N'))
        [IO.Directory]::Move($root, $backup)
    }
    $promoted = $false
    try {
        [IO.Directory]::Move($next, $root)
        $promoted = $true
        if ($afterPath -cne $beforePath.value) { Write-UserPath $beforePath $afterPath $false }
        $committed = $true
    } catch {
        $failure = $_
        try {
            if ($promoted) { [IO.Directory]::Move($root, (Join-Path $scratch 'failed-new')) }
            if ($null -ne $backup) { [IO.Directory]::Move($backup, $root); $backup = $null }
        } catch { throw "Install failed and rollback needs attention. Preserved backup: $backup. Original error: $failure. Rollback error: $_" }
        throw $failure
    }
    if ($AddToPath) {
        try { Update-ProcessPath $bin $true }
        catch { Write-Warning "Installation committed; reopen the terminal's parent app to load user PATH: $_" }
    }
    Write-Output "Installed local candidate $version at $root"
    Write-Output "Executable: $bin\ha.exe"
    Write-Output "Public data: $root\versions\$version"
    if ($afterPath -cne $beforePath.value) {
        Write-Output "Added only this user PATH entry: $bin. Restart the terminal's parent app (or sign out and in) so new processes inherit it."
    } elseif (-not $alreadyOnPath) {
        Write-Output "PATH was not changed. Invoke the absolute executable path, or rerun with -AddToPath."
    }
    Write-Output 'No public release was published. Install identity checks do not verify a project goal or host Skill loading.'
} catch {
    Write-Error -ErrorAction Continue ("Installation failed: " + $_)
    exit 1
} finally {
    foreach ($path in @($scratch, $(if ($committed) { $backup }))) {
        if ($path) {
            try { Remove-OrdinaryTree $path }
            catch { Write-Warning "Preserved temporary installation data; remove only after reviewing it: $path ($_)" }
        }
    }
    if ($null -ne $mutex) { $mutex.ReleaseMutex(); $mutex.Dispose() }
}
