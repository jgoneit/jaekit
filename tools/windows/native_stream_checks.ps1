# Developer-only NTFS observations. Real product flow uses isolated root/lock/PATH
# adapters; named streams and their byte snapshots use the actual filesystem.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$CandidateDirectory,
    [Parameter(Mandatory = $true)][string]$OutputPath
)
. (Join-Path $PSScriptRoot 'common.ps1')
$utf8 = New-Object Text.UTF8Encoding($false)
$report = [ordered]@{
    schema = 'jaekit-windows-streams/v1'; status = 'failed'; shell_version = $PSVersionTable.PSVersion.ToString()
    source_sha256 = @{}; filesystem = ''; actual_named_stream_roundtrip = $false
    required = @(); passed = @(); skipped = @(); observations = @(); failures = @()
    environment_before_sha256 = ''; environment_after_sha256 = ''; environment_preserved = $false
    unrelated_before = $null; unrelated_after = $null; unrelated_preserved = $false
    unverified = @('fixture-platform-root-lock-path', 'injected-errors-and-stage-timing', 'unsupported-filesystem-simulated', 'windows11-install-lifecycle')
}
$canWrite = $false; $beforeEnvironment = $null; $unrelated = ''
function Require([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Bytes-Digest([byte[]]$Bytes) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash($Bytes)).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function String-Digest([string]$Value) { return (Bytes-Digest ([Text.Encoding]::UTF8.GetBytes($Value))) }
function Save-Json([string]$Path, $Value) { [IO.File]::WriteAllText($Path, (ConvertTo-Json -Depth 16 -InputObject $Value), $utf8) }
function Comparison([string]$Text) { return [regex]::Replace([regex]::Replace($Text, '\x1B\[[0-?]*[ -/]*[@-~]', ''), '\s+', ' ') }
function Snapshot-Environment {
    $local = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    $user = Read-UserPath
    return (ConvertTo-Json -Compress -Depth 5 -InputObject ([ordered]@{
        install_exists = (Test-Path -LiteralPath (Join-Path $local 'Jaekit'))
        artifacts = @([IO.Directory]::GetFileSystemEntries($local, '.jaekit-*') | Sort-Object)
        user_value = $user.value; user_exists = $user.exists; user_kind = [int]$user.kind
        machine_path = [Environment]::GetEnvironmentVariable('Path', 'Machine'); process_path = $env:Path
        policy = @(Get-ExecutionPolicy -List | ForEach-Object { [ordered]@{scope = [string]$_.Scope; policy = [string]$_.ExecutionPolicy} })
    }))
}
function Observe([hashtable]$Row) {
    Require ($Row.before.default_sha256 -ceq $Row.after.default_sha256) ('Default data changed: ' + $Row.case)
    Require ((ConvertTo-Json -Compress -Depth 8 -InputObject @($Row.before.streams)) -ceq (ConvertTo-Json -Compress -Depth 8 -InputObject @($Row.after.streams))) ('Stream bytes or names changed: ' + $Row.case)
    $Row.preserved = $true; $Row.outcome_verified = $true
    $report.observations += $Row; $report.passed += $Row.case
}
function Copy-FixtureTree([string]$From, [string]$To) {
    [IO.Directory]::CreateDirectory($To) | Out-Null
    foreach ($item in @(Get-ChildItem -LiteralPath $From -Force -Recurse)) {
        Require (-not ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) 'Fixture source contains a link.'
        $target = Join-Path $To $item.FullName.Substring($From.Length + 1)
        if ($item.PSIsContainer) { [IO.Directory]::CreateDirectory($target) | Out-Null }
        else { [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null; [IO.File]::Copy($item.FullName, $target, $false) }
    }
}
function Make-Installation([string]$Directory, [string]$Current) {
    $versions = @()
    foreach ($version in @($previousVersion, $Current)) {
        $versionPath = Join-Path (Join-Path $Directory 'versions') $version
        Copy-FixtureTree $template $versionPath
        $manifestPath = Join-Path $versionPath 'package.json'
        if ($version -cne $candidateVersion) {
            $manifest = [IO.File]::ReadAllText($manifestPath, [Text.Encoding]::UTF8) | ConvertFrom-Json
            $manifest.version = $version
            Save-Json $manifestPath $manifest
        }
        $versions += @{version = $version; manifest_sha256 = (Get-Sha256 $manifestPath)}
    }
    [IO.Directory]::CreateDirectory((Join-Path $Directory 'bin')) | Out-Null
    [IO.File]::Copy((Join-Path $template 'ha.exe'), (Join-Path $Directory 'bin/ha.exe'), $false)
    Save-Json (Join-Path $Directory 'managed.json') @{
        schema = 'jaekit-windows-install/v1'; current_version = $Current; owns_path = $false
        created_path_value = $false; versions = @($versions)
    }
    Read-Installation $Directory | Out-Null
}
function Target-Path([string]$Root, [string]$Name, [string]$Current) {
    if ($Name -eq '.') { return $Root }
    return (Join-Path $Root $Name.Replace('{current}', $Current).Replace('{previous}', $previousVersion).Replace('/', '\'))
}
function Add-FixtureStream([string]$Path, [string]$Name, [string]$Hex) {
    [byte[]]$bytes = @(for ($i = 0; $i -lt $Hex.Length; $i += 2) { [Convert]::ToByte($Hex.Substring($i, 2), 16) })
    [Jaekit.StreamFixtureOracle]::WriteData($Path, (':' + $Name + ':$DATA'), $bytes)
}
function Refusal([scriptblock]$Action, [string]$Expected) {
    try { & $Action | Out-Null }
    catch { Require ((Comparison $_.ToString()).Contains($Expected)) ('Unexpected refusal: ' + $_); return }
    throw ('Expected product refusal: ' + $Expected)
}
function Run-Child([string]$CasePath, [string]$ResultPath, [string]$Log) {
    $old = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'; $PSNativeCommandUseErrorActionPreference = $false
        $global:LASTEXITCODE = $null
        $messages = @(& $hostExecutable -NoProfile -NonInteractive -File $probePath -Common $common -Oracle $oraclePath -CasePath $CasePath -ResultPath $ResultPath 2>&1)
        $exitCode = $global:LASTEXITCODE
    } finally { $ErrorActionPreference = $old }
    $text = ($messages | ForEach-Object { $_.ToString() }) -join "`n"
    [IO.File]::WriteAllText($Log, $text, $utf8)
    Require (Test-Path -LiteralPath $ResultPath) ('Operation did not emit observations: ' + $text)
    return @{code = $exitCode; text = $text; observed = ([IO.File]::ReadAllText($ResultPath, [Text.Encoding]::UTF8) | ConvertFrom-Json)}
}
function Operation([string]$Case, [string]$Directory, [string]$Action, [string]$Mode = '', [string]$FaultPath = '', [string]$SelectedCandidate = '') {
    if (-not $SelectedCandidate) { $SelectedCandidate = $candidate }
    $selectedArchive = @(Get-ChildItem -LiteralPath $SelectedCandidate -File -Filter '*.zip')
    Require ($selectedArchive.Count -eq 1) 'Expected one operation candidate.'
    $casePath = Join-Path $work ($Case + '.json')
    $resultPath = Join-Path $work ($Case + '-result.json')
    Save-Json $casePath @{
        root = $Directory; product = (Join-Path $PSScriptRoot ($Action + '.ps1')); action = $Action; mode = $Mode
        fault_path = $FaultPath; archive = $selectedArchive[0].FullName; checksums = (Join-Path $SelectedCandidate 'checksums.txt')
    }
    return (Run-Child $casePath $resultPath (Join-Path $work ($Case + '.log')))
}

try {
    Require ($env:OS -eq 'Windows_NT') 'Stream observations require native Windows.'
    $version = $PSVersionTable.PSVersion
    Require (($version.Major -eq 5 -and $version.Minor -eq 1) -or $version.Major -eq 7) 'Use PowerShell 5.1 or 7.'
    $beforeEnvironment = Snapshot-Environment
    $report.environment_before_sha256 = String-Digest $beforeEnvironment
    $candidate = Assert-OrdinaryPath $CandidateDirectory
    $output = Assert-OrdinaryPath $OutputPath
    Require (-not (Test-Path -LiteralPath $output)) 'Refuse to overwrite a stream report.'
    $work = Join-Path ([IO.Path]::GetDirectoryName($output)) 'stream-runtime'
    Require (-not (Test-Path -LiteralPath $work)) 'Refuse to overwrite stream observations.'
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $canWrite = $true
    $drive = New-Object IO.DriveInfo([IO.Path]::GetPathRoot($work))
    $report.filesystem = $drive.DriveFormat
    Require ($report.filesystem -ceq 'NTFS') 'Actual NTFS is required; other filesystems are not skipped or impersonated.'
    $common = Join-Path $PSScriptRoot 'common.ps1'
    foreach ($name in @('common.ps1', 'install.ps1', 'uninstall.ps1', 'native_stream_checks.ps1', 'stream_cases.json')) {
        $report.source_sha256['tools/windows/' + $name] = Get-Sha256 (Join-Path $PSScriptRoot $name)
    }
    $report.source_sha256['guides/WINDOWS.en.md'] = Get-Sha256 (Join-Path $PSScriptRoot '../../guides/WINDOWS.en.md')
    $matrix = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'stream_cases.json'), [Text.Encoding]::UTF8) | ConvertFrom-Json
    foreach ($target in $matrix.targets) { foreach ($boundary in $matrix.boundaries) { $report.required += $boundary + '-' + $target.id } }
    $report.required += @($matrix.normal) + @($matrix.errors) + @($matrix.late)
    Require ($report.required.Count -eq 56) 'Unexpected stream case matrix.'
    $hostExecutable = (Get-Process -Id $PID).Path

    # This independent observer enumerates the real filesystem and reads every
    # named stream's bytes; it does not reuse the product's presence decision.
    $oraclePath = Join-Path $work 'stream-oracle.ps1'
    $oracle = @'
Add-Type -TypeDefinition @"
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
namespace Jaekit {
 public static class StreamFixtureOracle {
  [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
  private struct Data { public long Size; [MarshalAs(UnmanagedType.ByValTStr, SizeConst=296)] public string Name; }
  [DllImport("kernel32.dll", CharSet=CharSet.Unicode, ExactSpelling=true, SetLastError=true)]
  private static extern IntPtr FindFirstStreamW(string name, int level, out Data data, uint flags);
  [DllImport("kernel32.dll", ExactSpelling=true, CharSet=CharSet.Unicode, SetLastError=true)]
  [return: MarshalAs(UnmanagedType.Bool)] private static extern bool FindNextStreamW(IntPtr handle, out Data data);
  [DllImport("kernel32.dll", ExactSpelling=true, SetLastError=true)]
  [return: MarshalAs(UnmanagedType.Bool)] private static extern bool FindClose(IntPtr handle);
  [DllImport("kernel32.dll", CharSet=CharSet.Unicode, ExactSpelling=true, SetLastError=true)]
  private static extern SafeFileHandle CreateFileW(string path, uint access, uint share, IntPtr security, uint disposition, uint flags, IntPtr template);
  private static SafeFileHandle OpenData(string path, string name, uint access, uint disposition) {
   if(!name.StartsWith(":", StringComparison.Ordinal) || !name.EndsWith(":`$DATA", StringComparison.Ordinal) || name=="::`$DATA")
    throw new ArgumentException("The fixture opens named DATA streams only.");
   SafeFileHandle handle=CreateFileW(path+name, access, 7, IntPtr.Zero, disposition, 0x80, IntPtr.Zero);
   if(handle.IsInvalid) { int e=Marshal.GetLastWin32Error(); handle.Dispose(); throw new Win32Exception(e); }
   return handle;
  }
  public static byte[] ReadData(string path, string name) {
   using(SafeFileHandle handle=OpenData(path, name, 0x80000000, 3))
   using(FileStream stream=new FileStream(handle, FileAccess.Read))
   using(MemoryStream bytes=new MemoryStream()) { stream.CopyTo(bytes); return bytes.ToArray(); }
  }
  public static void WriteData(string path, string name, byte[] bytes) {
   using(SafeFileHandle handle=OpenData(path, name, 0x40000000, 1))
   using(FileStream stream=new FileStream(handle, FileAccess.Write)) { stream.Write(bytes, 0, bytes.Length); }
  }
  public static string[] Names(string path) {
   Data data; IntPtr handle=FindFirstStreamW(path, 0, out data, 0);
   if(handle == new IntPtr(-1)) { int e=Marshal.GetLastWin32Error(); if(e==38) return new string[0]; throw new Win32Exception(e); }
   try {
    List<string> names=new List<string>();
    do { if(data.Name != "::`$DATA") names.Add(data.Name); } while(FindNextStreamW(handle, out data));
    int e=Marshal.GetLastWin32Error(); if(e!=38) throw new Win32Exception(e);
    names.Sort(StringComparer.Ordinal); return names.ToArray();
   } finally { if(!FindClose(handle)) throw new Win32Exception(Marshal.GetLastWin32Error()); }
  }
 }
}
"@
function Oracle-Digest([byte[]]$Bytes) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash($Bytes)).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function Snapshot-Tree([string]$Directory, [string]$Current = '', [string]$Previous = '') {
    $default = @(); $streams = @()
    if ([IO.Directory]::Exists($Directory)) {
        $paths = @($Directory) + @([IO.Directory]::GetFileSystemEntries($Directory, '*', [IO.SearchOption]::AllDirectories) | Sort-Object)
        foreach ($path in $paths) {
            $attrs = [IO.File]::GetAttributes($path)
            if ($attrs -band [IO.FileAttributes]::ReparsePoint) { throw 'Unexpected fixture link.' }
            $relative = if ($path -ceq $Directory) { '.' } else { $path.Substring($Directory.Length + 1).Replace('\', '/') }
            foreach ($pair in @(@($Current, '{current}'), @($Previous, '{previous}'))) {
                if ($pair[0]) {
                    $prefix = 'versions/' + $pair[0]
                    if ($relative -ceq $prefix) { $relative = 'versions/' + $pair[1] }
                    elseif ($relative.StartsWith($prefix + '/')) { $relative = 'versions/' + $pair[1] + $relative.Substring($prefix.Length) }
                }
            }
            $hash = if ($attrs -band [IO.FileAttributes]::Directory) { 'directory' } else { Oracle-Digest ([IO.File]::ReadAllBytes($path)) }
            $default += $relative + ':' + $hash
            foreach ($name in @([Jaekit.StreamFixtureOracle]::Names($path))) {
                [byte[]]$bytes = @([Jaekit.StreamFixtureOracle]::ReadData($path, $name))
                $streams += [ordered]@{path = $relative; name = $name; size = $bytes.Length; sha256 = (Oracle-Digest $bytes)}
            }
        }
    }
    return [ordered]@{default_sha256 = (Oracle-Digest ([Text.Encoding]::UTF8.GetBytes(($default -join "`n")))); streams = @($streams)}
}
'@
    [IO.File]::WriteAllText($oraclePath, $oracle, $utf8)
    . $oraclePath
    $unrelated = Join-Path $work 'unrelated-project'
    [IO.Directory]::CreateDirectory($unrelated) | Out-Null
    [IO.File]::WriteAllText((Join-Path $unrelated 'project.txt'), 'preserve unrelated project bytes', $utf8)
    Add-FixtureStream $unrelated 'project-notes' '70726f6a656374206e6f746573'
    $report.unrelated_before = Snapshot-Tree $unrelated
    $roundtrip = Join-Path $work 'roundtrip'
    [IO.Directory]::CreateDirectory($roundtrip) | Out-Null
    Add-FixtureStream $roundtrip 'probe' '4e544653'
    $roundtripSnapshot = Snapshot-Tree $roundtrip
    Require (@($roundtripSnapshot.streams).Count -eq 1 -and $roundtripSnapshot.streams[0].size -eq 4 -and $roundtripSnapshot.streams[0].sha256 -ceq (String-Digest 'NTFS')) 'Actual directory ADS roundtrip failed.'
    $report.actual_named_stream_roundtrip = $true

    $archives = @(Get-ChildItem -LiteralPath $candidate -File -Filter '*.zip')
    Require ($archives.Count -eq 1) 'Expected one built candidate ZIP.'
    $template = Join-Path $work 'template'
    $package = Expand-CheckedPackage $archives[0].FullName (Join-Path $candidate 'checksums.txt') $template
    $candidateVersion = [string]$package.manifest.version
    $previousVersion = '0.0.0-previous'; $olderVersion = '0.0.0-before'
    Require ($candidateVersion -cne $previousVersion -and $candidateVersion -cne $olderVersion) 'Candidate version collides with fixture history.'

    # Preserve actual product initialization and try/catch/finally. The only
    # default adapters are platform, fixture installation root/lock and PATH.
    # Stream decisions, copy/move/delete and ha identity execution remain real.
    $probePath = Join-Path $work 'stream-operation.ps1'
    $probe = @'
param([string]$Common, [string]$Oracle, [string]$CasePath, [string]$ResultPath)
. $Common
. $Oracle
$item = [IO.File]::ReadAllText($CasePath, [Text.Encoding]::UTF8) | ConvertFrom-Json
$script:fixtureRoot = [string]$item.root
$script:inventoryCalls = 0; $script:pathWrites = 0; $script:streamFaults = 0
$script:preservedTarget = ''; $script:targetBefore = $null; $script:targetAfter = $null
$script:cleanupRefused = $false; $script:stageInjected = $false
$script:realInventory = ${function:Read-Installation}
$script:realCleanup = ${function:Remove-OrdinaryTree}
$script:realStreams = ${function:Get-ItemDataStreams}
function Assert-WindowsPlatform {}
function Get-InstallRoot { return $script:fixtureRoot }
function Enter-InstallLock([string]$Root) { if ($Root -cne $script:fixtureRoot) { throw 'Wrong fixture lock.' }; return $null }
function Read-UserPath { return @{exists = $true; value = 'C:\synthetic-existing-path'; kind = [Microsoft.Win32.RegistryValueKind]::String} }
function Write-UserPath { $script:pathWrites++; throw 'FORBIDDEN_REAL_PATH_WRITE' }
function Update-ProcessPath { $script:pathWrites++; throw 'FORBIDDEN_PROCESS_PATH_WRITE' }
function Get-ItemDataStreams([string]$Path) {
    if ($item.mode -eq 'enumeration' -and $Path -ceq $item.fault_path) {
        $script:streamFaults++; throw ('INJECTED_STREAM_ENUMERATION_FAILURE: ' + $Path)
    }
    return (& $script:realStreams $Path)
}
function Read-Installation([string]$Root) {
    $script:inventoryCalls++
    if ($item.mode -eq 'staged' -and [IO.Path]::GetFileName($Root) -ceq 'next') {
        [Jaekit.StreamFixtureOracle]::WriteData($Root, ':late-notes:$DATA', [Text.Encoding]::UTF8.GetBytes('preserve staged data'))
        $script:preservedTarget = $Root; $script:targetBefore = Snapshot-Tree $Root
        $script:stageInjected = $true
    }
    return (& $script:realInventory $Root)
}
function Remove-OrdinaryTree([string]$Directory) {
    if ($item.mode -eq 'backup' -and ([IO.Path]::GetFileName($Directory) -like '.jaekit-previous-*' -or [IO.Path]::GetFileName($Directory) -like '.jaekit-remove-*')) {
        [Jaekit.StreamFixtureOracle]::WriteData($Directory, ':late-notes:$DATA', [Text.Encoding]::UTF8.GetBytes('preserve backup data'))
        $script:preservedTarget = $Directory; $script:targetBefore = Snapshot-Tree $Directory
        $script:stageInjected = $true
    }
    try { & $script:realCleanup $Directory }
    catch {
        if ($script:preservedTarget -and [IO.Directory]::Exists($script:preservedTarget)) {
            $script:targetAfter = Snapshot-Tree $script:preservedTarget
            $script:cleanupRefused = $true
        }
        throw
    }
}
$tokens = $null; $errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile([string]$item.product, [ref]$tokens, [ref]$errors)
if ($errors.Count -ne 0) { throw 'Cannot parse actual product operation.' }
$tries = @($ast.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.TryStatementAst] })
$initializers = @($ast.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.AssignmentStatementAst] })
$expected = if ($item.action -eq 'install') { '$mutex,$scratch,$backup,$committed' } else { '$mutex,$backup,$committed' }
if ($tries.Count -ne 1 -or (@($initializers | ForEach-Object { $_.Left.Extent.Text }) -join ',') -cne $expected) { throw 'Unexpected product control flow.' }
$commands = @($tries[0].Body.Statements | Where-Object { $_ -is [Management.Automation.Language.PipelineAst] })
if ($commands[0].Extent.Text -cne 'Assert-WindowsPlatform') { throw 'Missing production platform guard.' }
$flow = [scriptblock]::Create((@($initializers | ForEach-Object { $_.Extent.Text }) + @($tries[0].Extent.Text)) -join "`n")
$Archive = [string]$item.archive; $Checksums = [string]$item.checksums; $AddToPath = $false
try { & $flow }
finally {
    $value = @{shell_version = $PSVersionTable.PSVersion.ToString(); inventory_calls = $script:inventoryCalls
        path_write_count = $script:pathWrites; injected_error_count = $script:streamFaults
        stage_injected = $script:stageInjected; cleanup_refused = $script:cleanupRefused
        preserved_target = $script:preservedTarget; target_before = $script:targetBefore; target_after = $script:targetAfter}
    [IO.File]::WriteAllText($ResultPath, (ConvertTo-Json -Depth 16 -InputObject $value), (New-Object Text.UTF8Encoding($false)))
}
'@
    [IO.File]::WriteAllText($probePath, $probe, $utf8)

    foreach ($target in $matrix.targets) {
        foreach ($boundary in $matrix.boundaries) {
            $case = $boundary + '-' + $target.id
            $directory = Join-Path $work $case
            $current = if ($boundary -eq 'update') { $olderVersion } else { $candidateVersion }
            Make-Installation $directory $current
            $path = Target-Path $directory $target.path $current
            Add-FixtureStream $path $target.stream $target.hex
            $before = Snapshot-Tree $directory $current $previousVersion
            $row = @{case = $case; kind = 'actual_ntfs'; boundary = $boundary; rejected = $true; before = $before; diagnostic_identifies_target = $true}
            if ($boundary -eq 'inventory') { Refusal { Read-Installation $directory } ('Named data stream; nothing will be replaced or deleted: ' + $path) }
            elseif ($boundary -eq 'cleanup') { Refusal { Remove-OrdinaryTree $directory } ('Named data stream; nothing will be replaced or deleted: ' + $path) }
            else {
                $action = if ($boundary -eq 'uninstall') { 'uninstall' } else { 'install' }
                $result = Operation $case $directory $action
                Require ($result.code -eq 1 -and (Comparison $result.text).Contains(('Named data stream; nothing will be replaced or deleted: ' + $path))) ('Wrong lifecycle stream refusal: ' + $case + ': ' + $result.text)
                Require ($result.observed.shell_version -ceq $report.shell_version -and $result.observed.inventory_calls -eq 1 -and $result.observed.path_write_count -eq 0) 'Inventory refusal did not precede lifecycle mutations.'
                $row.exit_code = $result.code; $row.source_inventory_called = $true; $row.path_write_count = $result.observed.path_write_count
            }
            $row.after = Snapshot-Tree $directory $current $previousVersion
            Observe $row
        }
    }

    $sourceBefore = Snapshot-Tree $candidate
    foreach ($case in @($matrix.normal | Where-Object { $_ -ne 'read-only-inputs' })) {
        $directory = Join-Path $work $case
        if ($case -ne 'normal-install') { Make-Installation $directory $(if ($case -eq 'normal-update') { $olderVersion } else { $candidateVersion }) }
        if ($case -eq 'normal-cleanup') { Remove-OrdinaryTree $directory; Require (-not (Test-Path -LiteralPath $directory)) 'Ordinary tree cleanup failed.' }
        else {
            $action = if ($case -eq 'normal-uninstall') { 'uninstall' } else { 'install' }
            $result = Operation $case $directory $action
            Require ($result.code -eq 0 -and $result.observed.path_write_count -eq 0 -and $result.observed.inventory_calls -gt 0) ('Ordinary lifecycle failed: ' + $case + ': ' + $result.text)
            if ($action -eq 'uninstall') { Require (-not (Test-Path -LiteralPath $directory)) 'Ordinary uninstall left its target.' }
            else {
                $installed = Read-Installation $directory
                Require ($installed.state.current_version -ceq $candidateVersion) 'Normal installation has the wrong version.'
                if ($case -eq 'normal-update') { Require (@($installed.state.versions | Where-Object { $_.version -ceq $olderVersion }).Count -eq 1) 'Update discarded previous version data.' }
            }
        }
        Observe @{case = $case; kind = 'actual_ntfs'; rejected = $false; before = $sourceBefore; after = (Snapshot-Tree $candidate)}
    }

    $inputs = Join-Path $work 'downloaded-inputs'
    Copy-FixtureTree $candidate $inputs
    $inputFiles = @(Get-ChildItem -LiteralPath $inputs -File)
    Require ($inputFiles.Count -eq 6) 'Expected ZIP, checksums and four loose scripts.'
    foreach ($file in $inputFiles) { Add-FixtureStream $file.FullName 'Zone.Identifier' '5b5a6f6e655472616e736665725d0d0a5a6f6e6549643d330d0a' }
    $before = Snapshot-Tree $inputs
    # Run the source-checked public preflight itself; its candidate script bytes
    # remain untouched, while all six external input files carry actual ADS.
    $guide = [IO.File]::ReadAllText((Join-Path $PSScriptRoot '../../guides/WINDOWS.en.md'), [Text.Encoding]::UTF8)
    $preflight = [regex]::Match($guide, '(?s)<!-- BEGIN WINDOWS CANDIDATE PREFLIGHT -->\s*```powershell\s*(.*?)\s*```')
    Require $preflight.Success 'Public preflight block is missing.'
    . ([scriptblock]::Create($preflight.Groups[1].Value))
    Get-JaekitVerifiedCandidate $inputs | Out-Null
    $result = Operation 'read-only-inputs' (Join-Path $work 'input-install') 'install' '' '' $inputs
    Require ($result.code -eq 0) ('Download metadata caused a new input refusal: ' + $result.text)
    Observe @{case = 'read-only-inputs'; kind = 'actual_ntfs'; rejected = $false; before = $before; after = (Snapshot-Tree $inputs); input_stream_count = $inputFiles.Count}

    foreach ($boundary in @('inventory', 'cleanup', 'reinstall', 'update', 'uninstall')) {
        $case = 'enumeration-' + $boundary
        $directory = Join-Path $work $case
        $current = if ($boundary -eq 'update') { $olderVersion } else { $candidateVersion }
        Make-Installation $directory $current
        $faultPath = Join-Path $directory 'managed.json'
        $before = Snapshot-Tree $directory
        if ($boundary -eq 'inventory' -or $boundary -eq 'cleanup') {
            $real = ${function:Get-ItemDataStreams}; $script:injectedCalls = 0; $script:faultPath = $faultPath
            try {
                function Get-ItemDataStreams([string]$Path) {
                    if ($Path -ceq $script:faultPath) { $script:injectedCalls++; throw 'INJECTED_STREAM_ENUMERATION_FAILURE' }
                    return (& $real $Path)
                }
                if ($boundary -eq 'inventory') { Refusal { Read-Installation $directory } 'INJECTED_STREAM_ENUMERATION_FAILURE' }
                else { Refusal { Remove-OrdinaryTree $directory } 'INJECTED_STREAM_ENUMERATION_FAILURE' }
                Require ($script:injectedCalls -eq 1) 'Injected stream error was not reached.'
            } finally { Set-Item -Path Function:\Get-ItemDataStreams -Value $real }
        } else {
            $action = if ($boundary -eq 'uninstall') { 'uninstall' } else { 'install' }
            $result = Operation $case $directory $action 'enumeration' $faultPath
            Require ($result.code -eq 1 -and $result.observed.injected_error_count -eq 1 -and $result.observed.path_write_count -eq 0 -and (Comparison $result.text).Contains('INJECTED_STREAM_ENUMERATION_FAILURE')) 'Enumeration uncertainty did not stop the product operation.'
        }
        Observe @{case = $case; kind = 'injected_error'; rejected = $true; injected_branch_observed = $true; before = $before; after = (Snapshot-Tree $directory)}
    }

    # Compile the actual product C# with just the capability P/Invoke replaced.
    # Its Read decision body stays identical; no public product bypass is added.
    $tokens = $null; $errors = $null
    $ast = [Management.Automation.Language.Parser]::ParseFile($common, [ref]$tokens, [ref]$errors)
    Require ($errors.Count -eq 0) 'Cannot parse product stream inspector.'
    $initializers = @($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Initialize-InstallStreams'}, $true))
    Require ($initializers.Count -eq 1) 'Expected one stream initializer.'
    $strings = @($initializers[0].FindAll({param($n) $n -is [Management.Automation.Language.StringConstantExpressionAst] -and $n.Value.Contains('class InstallStreams')}, $true))
    Require ($strings.Count -eq 1) 'Expected one product stream C# body.'
    $source = $strings[0].Value
    $declaration = '(?s)\[DllImport\("kernel32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true\)\]\s*\[return: MarshalAs\(UnmanagedType.Bool\)\]\s*private static extern bool GetVolumeInformationW\(.*?\);'
    $capabilityImports = [regex]::Matches($source, $declaration)
    Require ($capabilityImports.Count -eq 1) 'Expected one capability import declaration.'
    foreach ($case in @('capability-api-failure', 'unsupported-filesystem')) {
        $className = if ($case -eq 'capability-api-failure') { 'StreamCapabilityFailureFixture' } else { 'StreamUnsupportedFixture' }
        $success = if ($case -eq 'capability-api-failure') { 'false' } else { 'true' }
        $stub = 'private static bool GetVolumeInformationW(string root, StringBuilder volumeName, uint volumeNameSize, out uint serial, out uint maximumComponentLength, out uint flags, StringBuilder fileSystemName, uint fileSystemNameSize) { serial=0; maximumComponentLength=255; flags=0; return ' + $success + '; }'
        $changed = $source.Remove($capabilityImports[0].Index, $capabilityImports[0].Length).Insert($capabilityImports[0].Index, $stub)
        Require ($changed.Remove($capabilityImports[0].Index, $stub.Length).Insert($capabilityImports[0].Index, $capabilityImports[0].Value) -ceq $source) 'Capability fixture changed the decision algorithm.'
        $changed = $changed.Replace('class InstallStreams', ('class ' + $className))
        Add-Type -TypeDefinition $changed
        $before = Snapshot-Tree $template
        if ($case -eq 'capability-api-failure') { Refusal { [Jaekit.StreamCapabilityFailureFixture]::Read([string]$template) } 'Cannot inspect named data streams:' }
        else { Require (@([Jaekit.StreamUnsupportedFixture]::Read([string]$template)).Count -eq 0) 'Confirmed unsupported capability was rejected.' }
        Observe @{case = $case; kind = $(if ($success -eq 'true') { 'injected_unsupported' } else { 'injected_error' }); rejected = ($success -ne 'true'); injected_branch_observed = $true; before = $before; after = (Snapshot-Tree $template)}
    }

    foreach ($case in $matrix.late) {
        $directory = Join-Path $work $case
        Make-Installation $directory $candidateVersion
        $installationBefore = Snapshot-Tree $directory
        $action = if ($case -eq 'uninstall-backup-preserved') { 'uninstall' } else { 'install' }
        $mode = if ($case -eq 'staged-stream-refusal') { 'staged' } else { 'backup' }
        $result = Operation $case $directory $action $mode
        $observed = $result.observed
        $committed = $mode -eq 'backup'
        Require ($result.code -eq $(if ($committed) { 0 } else { 1 }) -and $observed.stage_injected -and $observed.cleanup_refused -and $observed.path_write_count -eq 0) ('Wrong late stream outcome: ' + $case + ': ' + $result.text)
        $preservedTarget = [string]$observed.preserved_target
        Require ($preservedTarget.StartsWith($work + '\') -and [IO.Directory]::Exists($preservedTarget)) 'Late refusal lost its preserved tree.'
        $diagnostic = Comparison $result.text
        Require ($diagnostic.Contains('Named data stream; nothing will be replaced or deleted:') -and $diagnostic.Contains($preservedTarget)) 'Cleanup warning omitted its reason or preserved location.'
        if ($action -eq 'uninstall') { Require (-not (Test-Path -LiteralPath $directory) -and $diagnostic.Contains('Removal committed')) 'Removal outcome and cleanup warning were conflated.' }
        else {
            Read-Installation $directory | Out-Null
            Require ($diagnostic.Contains($(if ($committed) { 'Installed local candidate' } else { 'Installation failed:' }))) 'Install outcome was not reported.'
            if (-not $committed) { Require ((ConvertTo-Json -Compress -Depth 12 -InputObject $installationBefore) -ceq (ConvertTo-Json -Compress -Depth 12 -InputObject (Snapshot-Tree $directory))) 'Staged refusal changed the original installation.' }
        }
        Observe @{case = $case; kind = 'actual_ntfs_injected_stage'; rejected = (-not $committed); committed = $committed; exit_code = $result.code
            cleanup_preserved = $true; warning_identifies_preserved_tree = $true; before = $observed.target_before; after = (Snapshot-Tree $preservedTarget)}
    }
    Require ($report.passed.Count -eq $report.required.Count) 'Not all stream cases executed.'
    $report.status = 'passed'
} catch { $report.failures += $_.ToString() }
finally {
    if ($unrelated -and $null -ne $report.unrelated_before) {
        try {
            $report.unrelated_after = Snapshot-Tree $unrelated
            $report.unrelated_preserved = (ConvertTo-Json -Compress -Depth 12 -InputObject $report.unrelated_before) -ceq (ConvertTo-Json -Compress -Depth 12 -InputObject $report.unrelated_after)
            if (-not $report.unrelated_preserved) { $report.status = 'failed'; $report.failures += 'Unrelated project data changed.' }
        } catch { $report.status = 'failed'; $report.failures += $_.ToString() }
    }
    if ($null -ne $beforeEnvironment) {
        try {
            $afterEnvironment = Snapshot-Environment
            $report.environment_after_sha256 = String-Digest $afterEnvironment
            $report.environment_preserved = $beforeEnvironment -ceq $afterEnvironment
            if (-not $report.environment_preserved) { $report.status = 'failed'; $report.failures += 'Real user state changed.' }
        } catch { $report.status = 'failed'; $report.failures += $_.ToString() }
    }
    if ($canWrite) { Save-Json $output $report }
}
if ($report.status -ne 'passed') { throw ('Stream observations failed: ' + ($report.failures -join '; ')) }
