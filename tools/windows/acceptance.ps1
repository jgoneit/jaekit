<#
.SYNOPSIS
Run the real Windows installer lifecycle against two reviewed local candidates.
.DESCRIPTION
Requires Windows 11 x64, both PowerShell 5.1 and 7, and no existing Jaekit
installation. ConsentUserInstallation explicitly authorizes temporarily using
the real current-user install directory and adding/removing its user PATH entry.
Before invoking this script, use the trusted guide preflight on both complete
candidates, including all loose scripts. This script cannot retroactively verify
its own already-executed code. See the verified block in tools/windows/README.md.
This does not test Core goal execution or actual host Skill loading.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Archive,
    [Parameter(Mandatory = $true)][string]$Checksums,
    [Parameter(Mandatory = $true)][string]$UpdateArchive,
    [Parameter(Mandatory = $true)][string]$UpdateChecksums,
    [switch]$ConsentUserInstallation
)
. (Join-Path $PSScriptRoot 'common.ps1')
if (-not $ConsentUserInstallation) { throw 'Pass -ConsentUserInstallation only after reviewing this script and its installation/PATH effects.' }
Assert-WindowsPlatform
$root = Get-InstallRoot
if (Test-Path -LiteralPath $root) { throw "Acceptance refuses an existing installation or path: $root" }
$shell51 = (Get-Command powershell.exe -CommandType Application -ErrorAction Stop).Source
$shell7 = (Get-Command pwsh.exe -CommandType Application -ErrorAction Stop).Source
$Archive = [IO.Path]::GetFullPath($Archive)
$Checksums = [IO.Path]::GetFullPath($Checksums)
$UpdateArchive = [IO.Path]::GetFullPath($UpdateArchive)
$UpdateChecksums = [IO.Path]::GetFullPath($UpdateChecksums)
$installer = Join-Path $PSScriptRoot 'install.ps1'
$uninstaller = Join-Path $PSScriptRoot 'uninstall.ps1'
$bin = Join-Path $root 'bin'
$pathBefore = Read-UserPath
foreach ($entry in $pathBefore.value.Split(';')) {
    if (Test-PathEntry $entry $bin) { throw 'Acceptance requires a disposable profile without a pre-existing Jaekit user PATH entry.' }
}
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('jaekit acceptance ' + [char]0xAC00 + ' ' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($scratch) | Out-Null
$projectSentinel = Join-Path $scratch 'project.txt'
[IO.File]::WriteAllText($projectSentinel, 'synthetic user project sentinel')
$sentinelHash = Get-Sha256 $projectSentinel
$passed = $false

function Invoke-InstallCase([string]$Shell, [string]$Zip, [string]$Sums, [bool]$ExpectSuccess) {
    & $Shell -NoProfile -NonInteractive -File $installer -Archive $Zip -Checksums $Sums -AddToPath
    if (($LASTEXITCODE -eq 0) -ne $ExpectSuccess) { throw "Unexpected install exit $LASTEXITCODE in $Shell" }
}
function Get-Snapshot {
    $installation = Read-Installation $root
    if ($null -eq $installation) { throw 'Expected an installation.' }
    return (($installation.files.Keys | Sort-Object | ForEach-Object { $_ + ':' + $installation.files[$_] }) -join "`n")
}
function Assert-Preserved([string]$Snapshot) {
    if ((Get-Snapshot) -cne $Snapshot) { throw 'A failed operation changed installation contents.' }
    if ((Get-Sha256 $projectSentinel) -cne $sentinelHash) { throw 'Project sentinel changed.' }
}

try {
    Invoke-InstallCase $shell51 $Archive $Checksums $true
    $initial = Read-Installation $root
    $firstVersion = $initial.state.current_version
    $firstManifest = Get-Sha256 (Join-Path $root "versions\$firstVersion\package.json")
    Invoke-InstallCase $shell7 $Archive $Checksums $true
    $snapshot = Get-Snapshot

    $badDir = Join-Path $scratch 'bad checksum'
    [IO.Directory]::CreateDirectory($badDir) | Out-Null
    $badZip = Join-Path $badDir ([IO.Path]::GetFileName($Archive))
    [IO.File]::WriteAllText($badZip, 'not the candidate archive')
    Invoke-InstallCase $shell51 $badZip $Checksums $false
    Assert-Preserved $snapshot

    $foreign = Join-Path $root 'unmanaged-user-file.txt'
    [IO.File]::WriteAllText($foreign, 'synthetic unmanaged data')
    Invoke-InstallCase $shell7 $UpdateArchive $UpdateChecksums $false
    if ([IO.File]::ReadAllText($foreign) -cne 'synthetic unmanaged data') { throw 'Unmanaged data changed.' }
    [IO.File]::Delete($foreign)
    Assert-Preserved $snapshot

    $locked = [IO.File]::Open((Join-Path $bin 'ha.exe'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try { Invoke-InstallCase $shell7 $UpdateArchive $UpdateChecksums $false }
    finally { $locked.Dispose() }
    Assert-Preserved $snapshot

    Invoke-InstallCase $shell7 $UpdateArchive $UpdateChecksums $true
    $updated = Read-Installation $root
    if ($updated.state.current_version -ceq $firstVersion) { throw 'The update candidate must have a distinct executable/package version.' }
    if ((Get-Sha256 (Join-Path $root "versions\$firstVersion\package.json")) -cne $firstManifest) { throw 'The previous version data was not preserved.' }
    Invoke-InstallCase $shell51 $UpdateArchive $UpdateChecksums $true
    & $shell51 -NoProfile -NonInteractive -File $uninstaller
    if ($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $root)) { throw 'PowerShell 5.1 removal did not finish.' }

    Invoke-InstallCase $shell7 $Archive $Checksums $true
    # A later user-authored equivalent entry is not installer-owned. Removal
    # must remove its own literal entry and preserve this different spelling.
    $userAlias = '%LOCALAPPDATA%\Jaekit\bin'
    if (-not (Test-PathEntry $userAlias $bin)) { throw 'This profile has inconsistent LocalApplicationData and LOCALAPPDATA paths.' }
    $beforeAlias = Read-UserPath
    Write-UserPath $beforeAlias ($beforeAlias.value + ';' + $userAlias) $false
    & $shell7 -NoProfile -NonInteractive -File $uninstaller
    if ($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $root)) { throw 'PowerShell 7 removal did not finish.' }
    $afterAliasRemoval = Read-UserPath
    if ($afterAliasRemoval.value.Split(';') -cnotcontains $userAlias -or $afterAliasRemoval.value.Split(';') -ccontains $bin) {
        throw 'Removal deleted a user alias or left its own literal PATH entry.'
    }
    # Remove only the exact alias authored by this acceptance run, preserving
    # concurrent unrelated additions instead of restoring a whole snapshot.
    $aliasEntries = New-Object 'Collections.Generic.List[string]'
    foreach ($entry in $afterAliasRemoval.value.Split(';')) { $aliasEntries.Add($entry) }
    for ($i = $aliasEntries.Count - 1; $i -ge 0; $i--) {
        if ($aliasEntries[$i] -ceq $userAlias) { $aliasEntries.RemoveAt($i); break }
    }
    $withoutAlias = $aliasEntries -join ';'
    Write-UserPath $afterAliasRemoval $withoutAlias (-not $withoutAlias -and -not $pathBefore.exists)
    if ((Get-Sha256 $projectSentinel) -cne $sentinelHash) { throw 'Project sentinel changed during removal.' }

    $outside = Join-Path $scratch 'junction target'
    [IO.Directory]::CreateDirectory($outside) | Out-Null
    [IO.File]::WriteAllText((Join-Path $outside 'keep.txt'), 'keep')
    New-Item -ItemType Junction -Path $root -Target $outside -ErrorAction Stop | Out-Null
    try {
        Invoke-InstallCase $shell51 $Archive $Checksums $false
        & $shell7 -NoProfile -NonInteractive -File $uninstaller
        if ($LASTEXITCODE -eq 0) { throw 'Removal accepted a junction installation root.' }
        if ([IO.File]::ReadAllText((Join-Path $outside 'keep.txt')) -cne 'keep') { throw 'Junction target was changed.' }
    } finally { [IO.Directory]::Delete($root, $false) }

    $pathAfter = Read-UserPath
    if ($pathAfter.value -cne $pathBefore.value -or $pathAfter.exists -ne $pathBefore.exists -or $pathAfter.kind -ne $pathBefore.kind) {
        throw 'User PATH differs after lifecycle; inspect it without restoring a stale whole-PATH snapshot.'
    }
    $passed = $true
    Write-Output 'PASS: real Windows installer lifecycle on PowerShell 5.1 and 7, cross-shell updates, checksum/unmanaged/locked-file failures, junction preservation, and exact PATH ownership with a later user alias.'
    Write-Output 'This result does not establish Core goal execution, installed host Skill loading, or public release availability.'
} finally {
    if ($passed) { Remove-OrdinaryTree $scratch }
    else { Write-Warning "Acceptance did not pass. Preserved diagnostic files: $scratch. Inspect the managed installation at $root; no forced cleanup or PATH restore was attempted." }
}
