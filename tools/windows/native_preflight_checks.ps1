# Trusted developer-source tests of the public guide's independent preflight.
# Candidate code is never used as the verifier. Synthetic scripts are inert.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$CandidateDirectory,
    [Parameter(Mandatory = $true)][string]$OutputPath
)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$report = [ordered]@{
    schema = 'jaekit-windows-preflight/v1'; status = 'failed'
    shell_version = $PSVersionTable.PSVersion.ToString()
    guides = @(); failures = @()
}
$utf8 = New-Object Text.UTF8Encoding($false)
$canWrite = $false
$oldCanary = $env:JAEKIT_PREFLIGHT_CANARY
$oldEntry = $env:JAEKIT_PREFLIGHT_ENTRY

function Require([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}
function Text-Hash([string]$Text) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function Read-PublicBlock([string]$Path, [string]$Label) {
    $text = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8).Replace("`r`n", "`n")
    $begin = '<!-- BEGIN WINDOWS ' + $Label + ' -->'
    $end = '<!-- END WINDOWS ' + $Label + ' -->'
    Require ([regex]::Matches($text, [regex]::Escape($begin)).Count -eq 1) ('Missing or duplicate public example: ' + $Label)
    Require ([regex]::Matches($text, [regex]::Escape($end)).Count -eq 1) ('Missing or duplicate public example end: ' + $Label)
    $pattern = '(?s)' + [regex]::Escape($begin) + '\s*```powershell\n(.+?)\n```\s*' + [regex]::Escape($end)
    $found = [regex]::Match($text, $pattern)
    Require $found.Success ('Malformed public example: ' + $Label)
    return $found.Groups[1].Value
}
function Invoke-PublicExample([string]$Definition, [string]$Body, [string]$Directory) {
    Push-Location -LiteralPath $Directory
    try {
        # Both blocks are reviewed source documentation, never candidate bytes.
        . ([scriptblock]::Create($Definition))
        & ([scriptblock]::Create($Body))
    } finally { Pop-Location }
}
function Write-Fixture([string]$Directory, [string]$Version) {
    [IO.Directory]::CreateDirectory($Directory) | Out-Null
    $archiveName = 'ha_' + $Version + '_windows_amd64.zip'
    [IO.File]::WriteAllText((Join-Path $Directory $archiveName), 'synthetic archive bytes; never extracted or executed', $utf8)
    $common = '[IO.File]::AppendAllText($env:JAEKIT_PREFLIGHT_CANARY, "common`n")'
    [IO.File]::WriteAllText((Join-Path $Directory 'common.ps1'), $common, $utf8)
    foreach ($action in @('install', 'uninstall', 'acceptance')) {
        # All code in these fixtures is authored here and only records arguments.
        $body = @'
[CmdletBinding()]
param([string]$Archive, [string]$Checksums, [string]$UpdateArchive, [string]$UpdateChecksums,
      [switch]$AddToPath, [switch]$ConsentUserInstallation)
. (Join-Path $PSScriptRoot 'common.ps1')
[IO.File]::AppendAllText($env:JAEKIT_PREFLIGHT_CANARY, '__ACTION__' + "`n")
[IO.File]::WriteAllText($env:JAEKIT_PREFLIGHT_ENTRY, (ConvertTo-Json -Compress -InputObject ([ordered]@{
    action = '__ACTION__'; script = $PSCommandPath; archive = $Archive; checksums = $Checksums
    update_archive = $UpdateArchive; update_checksums = $UpdateChecksums
    add_to_path = [bool]$AddToPath; consent = [bool]$ConsentUserInstallation
})))
'@
        [IO.File]::WriteAllText((Join-Path $Directory ($action + '.ps1')), $body.Replace('__ACTION__', $action), $utf8)
    }
    $lines = @()
    foreach ($name in @($archiveName, 'common.ps1', 'install.ps1', 'uninstall.ps1', 'acceptance.ps1')) {
        $lines += (Get-FileHash -LiteralPath (Join-Path $Directory $name) -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $name
    }
    [IO.File]::WriteAllLines((Join-Path $Directory 'checksums.txt'), [string[]]$lines, [Text.Encoding]::ASCII)
}
function Snapshot-Fixture([string]$Directory) {
    $rows = @()
    foreach ($path in @([IO.Directory]::GetFileSystemEntries($Directory, '*', [IO.SearchOption]::AllDirectories) | Sort-Object)) {
        $name = $path.Substring($Directory.Length + 1)
        if ([IO.Directory]::Exists($path)) { $rows += 'directory:' + $name }
        else { $rows += $name + ':' + (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash }
    }
    return ($rows -join "`n")
}

try {
    Require ($env:OS -eq 'Windows_NT') 'Preflight runtime checks require actual Windows.'
    $version = $PSVersionTable.PSVersion
    Require (($version.Major -eq 5 -and $version.Minor -eq 1) -or $version.Major -eq 7) 'Use PowerShell 5.1 or 7.'
    $output = [IO.Path]::GetFullPath($OutputPath)
    Require (-not (Test-Path -LiteralPath $output)) 'Refuse to replace a preflight report.'
    $work = Join-Path ([IO.Path]::GetDirectoryName($output)) 'preflight-runtime'
    Require (-not (Test-Path -LiteralPath $work)) 'Refuse to replace preflight fixtures.'
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $canWrite = $true
    $root = [IO.Directory]::GetParent([IO.Directory]::GetParent($PSScriptRoot).FullName).FullName
    $acceptance = Read-PublicBlock (Join-Path $PSScriptRoot 'README.md') 'VERIFIED ACCEPTANCE'
    $keys = @('archive', 'common', 'install', 'uninstall', 'acceptance')
    $cases = @('actual-candidate', 'valid-install', 'valid-remove', 'valid-acceptance')
    foreach ($action in @('install', 'remove', 'acceptance-first', 'acceptance-update')) {
        foreach ($key in $keys) { $cases += $action + '-tampered-' + $key }
    }
    foreach ($key in @($keys) + @('checksums')) { $cases += 'missing-file-' + $key }
    foreach ($key in $keys) { $cases += 'missing-entry-' + $key }
    $cases += @('invalid-digest', 'mismatched-digest', 'duplicate-entry', 'ambiguous-case-entry',
                'unsafe-relative-entry', 'unsafe-absolute-entry', 'unexpected-entry', 'unexpected-script', 'directory-entry')
    foreach ($guide in @('guides/WINDOWS.md', 'guides/WINDOWS.en.md')) {
        $path = Join-Path $root $guide
        $definition = Read-PublicBlock $path 'CANDIDATE PREFLIGHT'
        $install = Read-PublicBlock $path 'VERIFIED INSTALL'
        $remove = Read-PublicBlock $path 'VERIFIED REMOVE'
        $observed = [ordered]@{
            path = $guide; required = @($cases); passed = @(); skipped = @()
            definition_sha256 = Text-Hash $definition
            action_sha256 = @{install = (Text-Hash $install); remove = (Text-Hash $remove); acceptance = (Text-Hash $acceptance)}
        }
        $report.guides += $observed
        $actual = Invoke-PublicExample $definition 'Get-JaekitVerifiedCandidate (Get-Location).Path' $CandidateDirectory
        Require ($actual.Directory -ceq [IO.Path]::GetFullPath($CandidateDirectory)) 'Actual candidate identity changed in guide preflight.'
        Require ([IO.File]::Exists($actual.Archive) -and [IO.File]::Exists($actual.Checksums)) 'Guide did not identify the actual archive and checksums.'
        foreach ($property in @('Install', 'Uninstall', 'Acceptance')) {
            Require ([IO.Path]::GetDirectoryName($actual.$property) -ceq $actual.Directory) 'Guide returned a script outside the verified candidate.'
        }
        $looseHashes = @{}
        foreach ($name in @('common.ps1', 'install.ps1', 'uninstall.ps1', 'acceptance.ps1')) {
            $looseHashes[$name] = (Get-FileHash -LiteralPath (Join-Path $actual.Directory $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        }
        $observed.actual_candidate = @{
            archive_sha256 = (Get-FileHash -LiteralPath $actual.Archive -Algorithm SHA256).Hash.ToLowerInvariant()
            checksums_sha256 = (Get-FileHash -LiteralPath $actual.Checksums -Algorithm SHA256).Hash.ToLowerInvariant()
            loose_sha256 = $looseHashes
        }
        $observed.passed += 'actual-candidate'
        $guideWork = Join-Path $work ([IO.Path]::GetFileName($guide))
        [IO.Directory]::CreateDirectory($guideWork) | Out-Null
        foreach ($case in $cases | Where-Object { $_ -ne 'actual-candidate' }) {
            $caseRoot = Join-Path $guideWork $case
            $current = Join-Path $caseRoot 'current'
            $next = Join-Path $caseRoot 'next'
            Write-Fixture $current '0.1.4-dev'
            Write-Fixture $next '0.1.5-dev'
            [IO.File]::WriteAllText((Join-Path $caseRoot 'unrelated-user-data.txt'), 'preserve this synthetic sentinel', $utf8)
            $env:JAEKIT_PREFLIGHT_CANARY = Join-Path $caseRoot 'execution-canary.txt'
            $env:JAEKIT_PREFLIGHT_ENTRY = Join-Path $caseRoot 'entry.json'
            $target = if ($case.StartsWith('acceptance-update-')) { $next } else { $current }
            $version = if ($target -ceq $next) { '0.1.5-dev' } else { '0.1.4-dev' }
            $names = @{archive = ('ha_' + $version + '_windows_amd64.zip'); common = 'common.ps1'; install = 'install.ps1'; uninstall = 'uninstall.ps1'; acceptance = 'acceptance.ps1'; checksums = 'checksums.txt'}
            $checksumPath = Join-Path $target 'checksums.txt'
            $lines = [IO.File]::ReadAllLines($checksumPath)
            $expectedError = $null
            if ($case -match '-tampered-(archive|common|install|uninstall|acceptance)$') {
                [IO.File]::AppendAllText((Join-Path $target $names[$Matches[1]]), "`n# synthetic byte mismatch`n", $utf8)
                $expectedError = 'SHA-256 mismatch:'
            } elseif ($case -match '^missing-file-(.+)$') {
                [IO.File]::Delete((Join-Path $target $names[$Matches[1]]))
                $expectedError = if ($Matches[1] -eq 'archive') { 'Expected exactly one Windows candidate ZIP.' } else { 'Candidate file inventory is incomplete or unexpected.' }
            } elseif ($case -match '^missing-entry-(.+)$') {
                $omit = $names[$Matches[1]]
                $lines = @($lines | Where-Object { -not $_.EndsWith('  ' + $omit) })
                [IO.File]::WriteAllLines($checksumPath, [string[]]$lines, [Text.Encoding]::ASCII)
                $expectedError = 'Missing checksum entry.'
            } elseif ($case -eq 'invalid-digest') {
                $lines[0] = 'not-a-digest  ' + $names.archive
                $expectedError = 'Malformed SHA-256 checksum entry.'
            } elseif ($case -eq 'mismatched-digest') {
                $lines[0] = ('0' * 64) + '  ' + $names.archive
                $expectedError = 'SHA-256 mismatch:'
            } elseif ($case -eq 'duplicate-entry') {
                $lines += $lines[0]; $expectedError = 'Duplicate checksum entry.'
            } elseif ($case -eq 'ambiguous-case-entry') {
                $lines += ('0' * 64) + '  COMMON.ps1'; $expectedError = 'Unexpected or unsafe checksum path.'
            } elseif ($case -eq 'unsafe-relative-entry') {
                $lines[0] = ('0' * 64) + '  ../outside.ps1'; $expectedError = 'Unexpected or unsafe checksum path.'
            } elseif ($case -eq 'unsafe-absolute-entry') {
                $lines[0] = ('0' * 64) + '  C:\outside.ps1'; $expectedError = 'Unexpected or unsafe checksum path.'
            } elseif ($case -eq 'unexpected-entry') {
                $lines += ('0' * 64) + '  extra.ps1'; $expectedError = 'Unexpected or unsafe checksum path.'
            } elseif ($case -eq 'unexpected-script') {
                [IO.File]::WriteAllText((Join-Path $target 'extra.ps1'), '# inert extra file', $utf8)
                $expectedError = 'Candidate file inventory is incomplete or unexpected.'
            } elseif ($case -eq 'directory-entry') {
                [IO.Directory]::CreateDirectory((Join-Path $target 'extra')) | Out-Null
                $expectedError = 'Candidate entries must be ordinary files.'
            }
            if ($case -in @('invalid-digest', 'mismatched-digest', 'duplicate-entry', 'ambiguous-case-entry', 'unsafe-relative-entry', 'unsafe-absolute-entry', 'unexpected-entry')) {
                [IO.File]::WriteAllLines($checksumPath, [string[]]$lines, [Text.Encoding]::ASCII)
            }
            $body = if ($case -match '^(valid-)?remove') { $remove } elseif ($case -match '^(valid-)?acceptance') { $acceptance } else { $install }
            $before = Snapshot-Fixture $caseRoot
            $caught = $null
            try { Invoke-PublicExample $definition $body $current | Out-Null }
            catch { $caught = $_.ToString() }
            if ($case.StartsWith('valid-')) {
                Require ($null -eq $caught) ('Valid guide action failed: ' + $case + ': ' + $caught)
                $action = if ($case -eq 'valid-remove') { 'uninstall' } else { $case.Substring(6) }
                Require ([IO.File]::ReadAllText($env:JAEKIT_PREFLIGHT_CANARY) -ceq ("common`n" + $action + "`n")) 'Expected verified script/helper did not execute exactly once.'
                $entry = [IO.File]::ReadAllText($env:JAEKIT_PREFLIGHT_ENTRY) | ConvertFrom-Json
                Require ($entry.action -ceq $action -and $entry.script -ceq (Join-Path $current ($action + '.ps1'))) 'Guide invoked a different script than the verified candidate.'
                if ($action -ne 'uninstall') {
                    Require ($entry.archive -ceq (Join-Path $current 'ha_0.1.4-dev_windows_amd64.zip') -and $entry.checksums -ceq (Join-Path $current 'checksums.txt')) 'Guide invoked code with different candidate inputs.'
                }
                if ($action -eq 'install') { Require ($entry.add_to_path -eq $true) 'Guide lost explicit PATH consent.' }
                if ($action -eq 'acceptance') {
                    Require ($entry.update_archive -ceq (Join-Path $next 'ha_0.1.5-dev_windows_amd64.zip') -and $entry.update_checksums -ceq (Join-Path $next 'checksums.txt') -and $entry.consent -eq $true) 'Acceptance did not bind both verified candidates and explicit consent.'
                }
            } else {
                Require ($null -ne $expectedError) ('Missing rejection expectation: ' + $case)
                Require ($null -ne $caught -and $caught.Contains($expectedError)) ('Wrong or missing pre-execution rejection: ' + $case + ': ' + $caught)
                Require (-not [IO.File]::Exists($env:JAEKIT_PREFLIGHT_CANARY) -and -not [IO.File]::Exists($env:JAEKIT_PREFLIGHT_ENTRY)) ('Guide executed candidate code after rejection: ' + $case)
                Require ((Snapshot-Fixture $caseRoot) -ceq $before) ('Preflight modified candidate or unrelated data: ' + $case)
            }
            $observed.passed += $case
        }
        Require ($observed.passed.Count -eq $cases.Count) 'Not all public preflight cases executed.'
    }
    $report.status = 'passed'
} catch {
    $report.failures += $_.ToString()
} finally {
    $env:JAEKIT_PREFLIGHT_CANARY = $oldCanary
    $env:JAEKIT_PREFLIGHT_ENTRY = $oldEntry
    if ($canWrite) { [IO.File]::WriteAllText($output, (ConvertTo-Json -Depth 12 -InputObject $report) + "`n", $utf8) }
}
if ($report.status -ne 'passed') { throw ($report.failures -join '; ') }
