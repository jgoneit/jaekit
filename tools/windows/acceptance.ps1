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
    [Parameter(Mandatory = $true)][string]$UpdateInstaller,
    [Parameter(Mandatory = $true)][string]$UpdateUninstaller,
    [switch]$ConsentUserInstallation
)
function Get-JaekitAcceptanceSteps {
    param(
        [string]$Archive, [string]$Checksums, [string]$UpdateArchive, [string]$UpdateChecksums,
        [string]$UpdateInstaller, [string]$UpdateUninstaller, [switch]$ConsentUserInstallation,
        [string]$EntryDirectory, [string]$Shell51, [string]$Shell7, [string]$BadArchive
    )
    if (-not $ConsentUserInstallation) { throw 'Acceptance requires explicit user installation consent.' }
    $firstDirectory = [IO.Path]::GetFullPath($EntryDirectory)
    $updateDirectory = [IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($UpdateArchive))
    $first = @{name = 'first'; directory = $firstDirectory; archive = $Archive; checksums = $Checksums
        install = (Join-Path $firstDirectory 'install.ps1'); uninstall = (Join-Path $firstDirectory 'uninstall.ps1')}
    $update = @{name = 'update'; directory = $updateDirectory; archive = $UpdateArchive; checksums = $UpdateChecksums
        install = $UpdateInstaller; uninstall = $UpdateUninstaller}
    foreach ($candidate in @($first, $update)) {
        foreach ($key in @('archive', 'checksums', 'install', 'uninstall')) {
            if ([string]::IsNullOrWhiteSpace($candidate[$key])) { throw ('Missing candidate input: ' + $candidate.name + '/' + $key) }
            $candidate[$key] = [IO.Path]::GetFullPath($candidate[$key])
            if (-not [IO.File]::Exists($candidate[$key])) { throw ('Missing candidate input: ' + $candidate.name + '/' + $key) }
            if (-not [IO.Path]::GetDirectoryName($candidate[$key]).Equals($candidate.directory, [StringComparison]::OrdinalIgnoreCase)) {
                throw ('Candidate input points to a different directory: ' + $candidate.name + '/' + $key)
            }
        }
        if ([IO.Path]::GetFileName($candidate.install) -cne 'install.ps1' -or
            [IO.Path]::GetFileName($candidate.uninstall) -cne 'uninstall.ps1' -or
            [IO.Path]::GetFileName($candidate.checksums) -cne 'checksums.txt' -or
            -not [IO.File]::Exists((Join-Path $candidate.directory 'common.ps1'))) { throw 'Candidate scripts or helper are missing or misnamed.' }
    }
    function New-Step([string]$Name, [hashtable]$Candidate, [string]$Shell, [string]$Action,
                      [string]$ExpectedDiagnostic = '', [string]$Zip = '') {
        if (-not $Zip) { $Zip = $Candidate.archive }
        $arguments = @()
        if ($Action -eq 'install') { $arguments = @('-Archive', $Zip, '-Checksums', $Candidate.checksums, '-AddToPath') }
        return [pscustomobject]@{Name = $Name; Candidate = $Candidate.name; Shell = $Shell
            Script = $Candidate[$Action]; Arguments = @($arguments); ExpectedDiagnostic = $ExpectedDiagnostic}
    }
    return [ordered]@{
        'first-install' = New-Step 'first-install' $first $Shell51 'install'
        'first-reinstall' = New-Step 'first-reinstall' $first $Shell7 'install'
        'checksum-refusal' = New-Step 'checksum-refusal' $first $Shell51 'install' 'Archive checksum is missing, duplicated or does not match.' $BadArchive
        'update-unmanaged-refusal' = New-Step 'update-unmanaged-refusal' $update $Shell7 'install' 'Unmanaged file; nothing will be replaced:'
        'update-locked-refusal' = New-Step 'update-locked-refusal' $update $Shell7 'install' 'The current executable cannot be replaced'
        'update-install' = New-Step 'update-install' $update $Shell7 'install'
        'update-reinstall' = New-Step 'update-reinstall' $update $Shell51 'install'
        'update-remove' = New-Step 'update-remove' $update $Shell51 'uninstall'
        'first-install-again' = New-Step 'first-install-again' $first $Shell7 'install'
        'first-remove' = New-Step 'first-remove' $first $Shell7 'uninstall'
        'junction-install-refusal' = New-Step 'junction-install-refusal' $first $Shell51 'install' 'Link or reparse-point path is not allowed:'
        'junction-remove-refusal' = New-Step 'junction-remove-refusal' $first $Shell7 'uninstall' 'Link or reparse-point path is not allowed:'
    }
}

function Invoke-JaekitAcceptanceStep($Step) {
    if (-not [IO.File]::Exists($Step.Script)) { throw ('Acceptance step ' + $Step.Name + ': candidate script is missing.') }
    try { $shell = (Get-Command $Step.Shell -CommandType Application -ErrorAction Stop).Source }
    catch { throw ('Acceptance step ' + $Step.Name + ': shell could not start: ' + $_) }
    $arguments = $Step.Arguments
    $previousPreference = $ErrorActionPreference
    $code = $null
    try {
        # Preserve native stderr on 5.1; an expected refusal still needs the
        # exact product diagnostic, never just a setup error or arbitrary exit.
        $ErrorActionPreference = 'Continue'
        $PSNativeCommandUseErrorActionPreference = $false
        # Native commands update the global automatic variable. A local reset
        # would shadow that update, leaving a successful child's code null.
        $global:LASTEXITCODE = $null
        $messages = @(& $shell -NoProfile -NonInteractive -File $Step.Script @arguments 2>&1)
        $code = $global:LASTEXITCODE
    } finally { $ErrorActionPreference = $previousPreference }
    $diagnostic = ($messages | ForEach-Object { $_.ToString() }) -join "`n"
    # Keep child warnings and preserved-backup locations visible even when the
    # stage succeeds or produces its expected refusal and the result is ignored.
    if ($diagnostic) { Write-Host $diagnostic }
    if ($Step.ExpectedDiagnostic) {
        # PowerShell's native error formatter can wrap the product message.
        # Normalize presentation whitespace only; keep the original output.
        $comparison = [regex]::Replace($diagnostic, '\s+', ' ')
        if ($code -ne 1 -or -not $comparison.Contains($Step.ExpectedDiagnostic)) {
            throw ('Acceptance step ' + $Step.Name + ': expected product refusal was not observed. Exit=' + $code + '. ' + $diagnostic)
        }
    } elseif ($code -ne 0) {
        throw ('Acceptance step ' + $Step.Name + ': candidate execution failed. Exit=' + $code + '. ' + $diagnostic)
    }
    return [pscustomobject]@{stage = $Step.Name; candidate = $Step.Candidate; exit_code = $code}
}

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
    $badDir = Join-Path $scratch 'bad checksum'
    [IO.Directory]::CreateDirectory($badDir) | Out-Null
    $badZip = Join-Path $badDir ([IO.Path]::GetFileName($Archive))
    [IO.File]::WriteAllText($badZip, 'not the candidate archive')
    # BEGIN ACCEPTANCE DISPATCH BINDING
    $steps = Get-JaekitAcceptanceSteps @PSBoundParameters -EntryDirectory $PSScriptRoot -Shell51 $shell51 -Shell7 $shell7 -BadArchive $badZip
    # END ACCEPTANCE DISPATCH BINDING
    Invoke-JaekitAcceptanceStep $steps['first-install'] | Out-Null
    $initial = Read-Installation $root
    $firstVersion = $initial.state.current_version
    $firstManifest = Get-Sha256 (Join-Path $root "versions\$firstVersion\package.json")
    Invoke-JaekitAcceptanceStep $steps['first-reinstall'] | Out-Null
    $snapshot = Get-Snapshot

    Invoke-JaekitAcceptanceStep $steps['checksum-refusal'] | Out-Null
    Assert-Preserved $snapshot

    $foreign = Join-Path $root 'unmanaged-user-file.txt'
    [IO.File]::WriteAllText($foreign, 'synthetic unmanaged data')
    Invoke-JaekitAcceptanceStep $steps['update-unmanaged-refusal'] | Out-Null
    if ([IO.File]::ReadAllText($foreign) -cne 'synthetic unmanaged data') { throw 'Unmanaged data changed.' }
    [IO.File]::Delete($foreign)
    Assert-Preserved $snapshot

    $locked = [IO.File]::Open((Join-Path $bin 'ha.exe'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try {
        Invoke-JaekitAcceptanceStep $steps['update-locked-refusal'] | Out-Null
    }
    finally { $locked.Dispose() }
    Assert-Preserved $snapshot

    Invoke-JaekitAcceptanceStep $steps['update-install'] | Out-Null
    $updated = Read-Installation $root
    if ($updated.state.current_version -ceq $firstVersion) { throw 'The update candidate must have a distinct executable/package version.' }
    if ((Get-Sha256 (Join-Path $root "versions\$firstVersion\package.json")) -cne $firstManifest) { throw 'The previous version data was not preserved.' }
    Invoke-JaekitAcceptanceStep $steps['update-reinstall'] | Out-Null
    Invoke-JaekitAcceptanceStep $steps['update-remove'] | Out-Null
    if (Test-Path -LiteralPath $root) { throw 'PowerShell 5.1 removal did not finish.' }

    Invoke-JaekitAcceptanceStep $steps['first-install-again'] | Out-Null
    # Later user entries include an exact duplicate separated by another path
    # and a differently spelled alias. Their complete order must survive.
    $userAlias = '%LOCALAPPDATA%\Jaekit\bin'
    if (-not (Test-PathEntry $userAlias $bin)) { throw 'This profile has inconsistent LocalApplicationData and LOCALAPPDATA paths.' }
    $beforeAlias = Read-UserPath
    $userMarker = Join-Path $scratch 'later-user-tool'
    $addedTail = $userMarker + ';' + $bin + ';' + $userAlias
    Write-UserPath $beforeAlias ($beforeAlias.value + ';' + $addedTail) $false
    Invoke-JaekitAcceptanceStep $steps['first-remove'] | Out-Null
    if (Test-Path -LiteralPath $root) { throw 'PowerShell 7 removal did not finish.' }
    $afterAliasRemoval = Read-UserPath
    $expectedPath = if ($pathBefore.value) { $pathBefore.value + ';' + $addedTail } else { $addedTail }
    if ($afterAliasRemoval.value -cne $expectedPath) {
        throw 'Removal changed the later duplicate, alias, or unrelated PATH order.'
    }
    # Remove only the three exact entries authored by this acceptance run.
    # Never restore an earlier whole-PATH snapshot over concurrent edits.
    $aliasEntries = New-Object 'Collections.Generic.List[string]'
    foreach ($entry in $afterAliasRemoval.value.Split(';')) { $aliasEntries.Add($entry) }
    foreach ($ownedFixtureEntry in @($userAlias, $bin, $userMarker)) {
        for ($i = $aliasEntries.Count - 1; $i -ge 0; $i--) {
            if ($aliasEntries[$i] -ceq $ownedFixtureEntry) { $aliasEntries.RemoveAt($i); break }
        }
    }
    $withoutAlias = $aliasEntries -join ';'
    Write-UserPath $afterAliasRemoval $withoutAlias (-not $withoutAlias -and -not $pathBefore.exists)
    if ((Get-Sha256 $projectSentinel) -cne $sentinelHash) { throw 'Project sentinel changed during removal.' }

    $outside = Join-Path $scratch 'junction target'
    [IO.Directory]::CreateDirectory($outside) | Out-Null
    [IO.File]::WriteAllText((Join-Path $outside 'keep.txt'), 'keep')
    New-Item -ItemType Junction -Path $root -Target $outside -ErrorAction Stop | Out-Null
    try {
        Invoke-JaekitAcceptanceStep $steps['junction-install-refusal'] | Out-Null
        Invoke-JaekitAcceptanceStep $steps['junction-remove-refusal'] | Out-Null
        if ([IO.File]::ReadAllText((Join-Path $outside 'keep.txt')) -cne 'keep') { throw 'Junction target was changed.' }
    } finally { [IO.Directory]::Delete($root, $false) }

    $pathAfter = Read-UserPath
    if ($pathAfter.value -cne $pathBefore.value -or $pathAfter.exists -ne $pathBefore.exists -or $pathAfter.kind -ne $pathBefore.kind) {
        throw 'User PATH differs after lifecycle; inspect it without restoring a stale whole-PATH snapshot.'
    }
    $passed = $true
    Write-Output 'PASS: real Windows installer lifecycle on PowerShell 5.1 and 7, cross-shell updates, checksum/unmanaged/locked-file failures, junction preservation, and exact PATH order with a later user duplicate and alias.'
    Write-Output 'This result does not establish Core goal execution, installed host Skill loading, or public release availability.'
} finally {
    if ($passed) { Remove-OrdinaryTree $scratch }
    else { Write-Warning "Acceptance did not pass. Preserved diagnostic files: $scratch. Inspect the managed installation at $root; no forced cleanup or PATH restore was attempted." }
}
