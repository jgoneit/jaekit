# Developer-only observations of package names and pre-mutation refusal.
# Never opens a console device or changes real installation/PATH/policy state.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$CandidateDirectory,
    [Parameter(Mandatory = $true)][string]$OutputPath
)
. (Join-Path $PSScriptRoot 'common.ps1')
$report = [ordered]@{
    schema = 'jaekit-windows-console-paths/v1'; status = 'failed'
    shell_version = $PSVersionTable.PSVersion.ToString(); source_sha256 = @{}
    required = @(); passed = @(); skipped = @(); observations = @(); failures = @()
    environment_before_sha256 = ''; environment_after_sha256 = ''; environment_preserved = $false
    unverified = @('fixture-platform-root-lock', 'archive-entry-open-observer', 'windows11-install-lifecycle')
}
$utf8 = New-Object Text.UTF8Encoding($false)
$canWrite = $false
$before = $null
function Require([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function String-Digest([string]$Value) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
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
function Tree-Digest([string]$Directory) {
    $rows = @()
    if (Test-Path -LiteralPath $Directory) {
        foreach ($entry in @(Get-ChildItem -LiteralPath $Directory -Recurse -Force | Sort-Object FullName)) {
            Require (-not ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)) 'Unexpected fixture link.'
            $rows += $entry.FullName.Substring($Directory.Length) + ':' + $(if ($entry.PSIsContainer) { 'directory' } else { Get-Sha256 $entry.FullName })
        }
    }
    return (String-Digest ($rows -join "`n"))
}
function Refusal([scriptblock]$Action) {
    try { & $Action | Out-Null }
    catch {
        if ($_.ToString().Contains('Unsafe package path:')) { return }
        throw
    }
    throw 'Expected the product package-name refusal.'
}
function Observe([hashtable]$Row) {
    $report.observations += $Row
    $report.passed += $Row.case
}
function Save-Json([string]$Path, $Value) {
    [IO.File]::WriteAllText($Path, (ConvertTo-Json -Depth 12 -InputObject $Value), $utf8)
}
function Make-Package([string]$Directory, [string]$Version, [string]$BadName = '') {
    [IO.Directory]::CreateDirectory($Directory) | Out-Null
    $names = @('ha.exe', 'LICENSE', 'guides/WINDOWS.md', 'guides/WINDOWS.en.md', 'tools/windows/common.ps1', 'tools/windows/uninstall.ps1')
    $names += @($matrix.allowed | ForEach-Object { $_.path })
    $entries = @()
    foreach ($name in $names) {
        # Only known ordinary spellings are ever materialized by this fixture.
        $path = Join-Path $Directory $name.Replace('/', '\')
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($path)) | Out-Null
        [IO.File]::WriteAllText($path, ('synthetic ordinary data: ' + $name), $utf8)
        $entries += @{path = $name; size = (Get-Item -LiteralPath $path).Length; sha256 = (Get-Sha256 $path)}
    }
    if ($BadName) { $entries += @{path = $BadName; size = 0; sha256 = ('0' * 64)} }
    Save-Json (Join-Path $Directory 'package.json') @{schema = 'jaekit-windows-package/v1'; os = 'windows'; arch = 'amd64'; version = $Version; files = @($entries)}
}
function Make-Installation([string]$Directory, [string]$Version, [string]$BadName = '') {
    $versionPath = Join-Path (Join-Path $Directory 'versions') $Version
    Make-Package $versionPath $Version $BadName
    [IO.Directory]::CreateDirectory((Join-Path $Directory 'bin')) | Out-Null
    [IO.File]::Copy((Join-Path $versionPath 'ha.exe'), (Join-Path $Directory 'bin/ha.exe'), $false)
    Save-Json (Join-Path $Directory 'managed.json') @{
        schema = 'jaekit-windows-install/v1'; current_version = $Version; owns_path = $true; created_path_value = $false
        versions = @(@{version = $Version; manifest_sha256 = (Get-Sha256 (Join-Path $versionPath 'package.json'))})
    }
}
function Run-Child([string]$Script, [string[]]$Arguments, [string]$Log) {
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'; $PSNativeCommandUseErrorActionPreference = $false
        $global:LASTEXITCODE = $null
        $messages = @(& $hostExecutable -NoProfile -NonInteractive -File $Script @Arguments 2>&1)
        $code = $global:LASTEXITCODE
    } finally { $ErrorActionPreference = $previous }
    $text = ($messages | ForEach-Object { $_.ToString() }) -join "`n"
    [IO.File]::WriteAllText($Log, $text, $utf8)
    return @{code = $code; text = $text}
}

try {
    Require ($env:OS -eq 'Windows_NT') 'Console-name observations require native Windows.'
    $v = $PSVersionTable.PSVersion
    Require (($v.Major -eq 5 -and $v.Minor -eq 1) -or $v.Major -eq 7) 'Use PowerShell 5.1 or 7.'
    $before = Snapshot-Environment
    $report.environment_before_sha256 = String-Digest $before
    $candidate = Assert-OrdinaryPath $CandidateDirectory
    $output = Assert-OrdinaryPath $OutputPath
    Require (-not (Test-Path -LiteralPath $output)) 'Refuse to overwrite a console report.'
    $work = Join-Path ([IO.Path]::GetDirectoryName($output)) 'console-runtime'
    Require (-not (Test-Path -LiteralPath $work)) 'Refuse to overwrite console fixtures.'
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $canWrite = $true
    $root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
    $common = Join-Path $PSScriptRoot 'common.ps1'
    foreach ($name in @('package.py', 'common.ps1', 'install.ps1', 'uninstall.ps1')) {
        $report.source_sha256['tools/windows/' + $name] = Get-Sha256 (Join-Path $PSScriptRoot $name)
    }
    $matrix = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'console_cases.json'), [Text.Encoding]::UTF8) | ConvertFrom-Json
    $boundaries = @('name', 'join', 'manifest', 'inventory', 'copy', 'archive')
    foreach ($item in $matrix.rejected) { foreach ($boundary in $boundaries) { $report.required += $boundary + '-' + $item.id } }
    $report.required += @($matrix.allowed | ForEach-Object { 'allowed-' + $_.id })
    $report.required += @('normal-manifest', 'normal-inventory', 'normal-copy', 'normal-archive', 'install-archive-refusal', 'reinstall-inventory-refusal', 'update-inventory-refusal', 'uninstall-inventory-refusal')
    Require ($report.required.Count -eq 84) 'Unexpected console case matrix.'
    $hostExecutable = (Get-Process -Id $PID).Path

    # The actual expander is retained except for one AST-identified member
    # stream-open call. Its observer delegates on normal input and throws on
    # negative fixtures, ensuring even a broken guard never opens a device.
    $tokens = $null; $errors = $null
    $ast = [Management.Automation.Language.Parser]::ParseFile($common, [ref]$tokens, [ref]$errors)
    Require ($errors.Count -eq 0) 'Cannot parse product package functions.'
    $functions = @($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -ceq 'Expand-CheckedPackage'}, $true))
    Require ($functions.Count -eq 1) 'Expected one product expander.'
    $function = $functions[0]
    $opens = @($function.FindAll({param($node) $node -is [Management.Automation.Language.InvokeMemberExpressionAst] -and $node.Extent.Text -ceq '$entry.Open()'}, $true))
    Require ($opens.Count -eq 1) 'Expected one archive member data-open call.'
    $original = $function.Extent.Text
    $offset = $opens[0].Extent.StartOffset - $function.Extent.StartOffset
    $replacement = '(Open-ObservedArchiveEntry $entry)'
    $adapted = $original.Remove($offset, $opens[0].Extent.Text.Length).Insert($offset, $replacement)
    Require ($adapted.Remove($offset, $replacement.Length).Insert($offset, '$entry.Open()') -ceq $original) 'Observer changed other expander statements.'
    $expanderPath = Join-Path $work 'observed-expander.ps1'
    $observer = @'
$script:entryOpenCount = 0
$script:allowEntryOpen = $false
function Open-ObservedArchiveEntry($Entry) {
    $script:entryOpenCount++
    if (-not $script:allowEntryOpen) { throw 'FORBIDDEN_ARCHIVE_DATA_OPEN' }
    return $Entry.Open()
}
'@
    [IO.File]::WriteAllText($expanderPath, ($observer + "`n" + $adapted), $utf8)
    . $expanderPath
    $archives = @(Get-ChildItem -LiteralPath $candidate -Filter '*.zip' -File)
    Require ($archives.Count -eq 1) 'Expected one actual candidate ZIP.'
    $archive = $archives[0].FullName; $checksums = Join-Path $candidate 'checksums.txt'
    $normalDestination = Join-Path $work ('normal space ' + [char]0xd55c + [char]0xae00)
    $script:allowEntryOpen = $true
    $normal = Expand-CheckedPackage $archive $checksums $normalDestination
    Require ($script:entryOpenCount -gt 0) 'Normal package did not execute the observed data-open boundary.'
    $candidateVersion = $normal.manifest.version
    Observe @{case = 'normal-archive'; rejected = $false; preserved = $true}
    $script:allowEntryOpen = $false

    foreach ($item in $matrix.allowed) {
        Assert-PackageName $item.path
        $joined = Join-PackagePath $normalDestination $item.path
        Require ($joined.StartsWith($normalDestination + '\')) 'Ordinary join escaped its destination.'
        Observe @{case = ('allowed-' + $item.id); path = $item.path; rejected = $false; preserved = $true}
    }
    $ordinary = Join-Path $work 'ordinary-package'
    Make-Package $ordinary $candidateVersion
    $ordinaryBefore = Tree-Digest $ordinary
    $ordinaryPackage = Read-PackageManifest $ordinary
    Observe @{case = 'normal-manifest'; rejected = $false; preserved = ((Tree-Digest $ordinary) -ceq $ordinaryBefore)}
    $copyFiles = @{'package.json' = (Get-Sha256 (Join-Path $ordinary 'package.json'))}
    foreach ($name in $ordinaryPackage.files.Keys) { $copyFiles[$name] = $ordinaryPackage.files[$name].sha256 }
    $ordinaryCopy = Join-Path $work 'ordinary-copy'
    Copy-Inventory $ordinary $ordinaryCopy $copyFiles
    Assert-TreeInventory $ordinaryCopy $copyFiles
    Observe @{case = 'normal-copy'; rejected = $false; preserved = ((Tree-Digest $ordinary) -ceq $ordinaryBefore)}
    $ordinaryInstall = Join-Path $work 'ordinary-install'
    Make-Installation $ordinaryInstall $candidateVersion
    $ordinaryBefore = Tree-Digest $ordinaryInstall
    Read-Installation $ordinaryInstall | Out-Null
    Observe @{case = 'normal-inventory'; rejected = $false; preserved = ((Tree-Digest $ordinaryInstall) -ceq $ordinaryBefore)}

    foreach ($item in $matrix.rejected) {
        $name = [string]$item.path
        $caseDirectory = Join-Path $work $item.id
        [IO.Directory]::CreateDirectory($caseDirectory) | Out-Null
        Refusal { Assert-PackageName $name }
        Observe @{case = ('name-' + $item.id); boundary = 'name'; path = $name; rejected = $true; preserved = $true}
        Refusal { Join-PackagePath $caseDirectory $name }
        Observe @{case = ('join-' + $item.id); boundary = 'join'; path = $name; rejected = $true; preserved = $true}
        $manifestDirectory = Join-Path $caseDirectory 'manifest'
        Make-Package $manifestDirectory $candidateVersion $name
        $snapshot = Tree-Digest $manifestDirectory
        Refusal { Read-PackageManifest $manifestDirectory }
        Observe @{case = ('manifest-' + $item.id); boundary = 'manifest'; path = $name; rejected = $true; preserved = ((Tree-Digest $manifestDirectory) -ceq $snapshot)}
        $installation = Join-Path $caseDirectory 'installation'
        Make-Installation $installation $candidateVersion $name
        $snapshot = Tree-Digest $installation
        Refusal { Read-Installation $installation }
        Observe @{case = ('inventory-' + $item.id); boundary = 'inventory'; path = $name; rejected = $true; preserved = ((Tree-Digest $installation) -ceq $snapshot)}
        $copy = Join-Path $caseDirectory 'must-not-copy'
        $files = @{}; $files[$name] = ('0' * 64)
        Refusal { Copy-Inventory $ordinary $copy $files }
        Require (-not (Test-Path -LiteralPath $copy)) 'Unsafe copy created a destination.'
        Observe @{case = ('copy-' + $item.id); boundary = 'copy'; path = $name; rejected = $true; preserved = $true}
        $modified = Join-Path $caseDirectory ([IO.Path]::GetFileName($archive))
        [IO.File]::Copy($archive, $modified, $false)
        $zip = [IO.Compression.ZipFile]::Open($modified, [IO.Compression.ZipArchiveMode]::Update)
        try {
            $entry = $zip.CreateEntry($name)
            $stream = $entry.Open()
            try { $stream.WriteByte(120) } finally { $stream.Dispose() }
        } finally { $zip.Dispose() }
        $sums = Join-Path $caseDirectory 'checksums.txt'
        [IO.File]::WriteAllText($sums, (Get-Sha256 $modified) + '  ' + [IO.Path]::GetFileName($modified) + "`n", $utf8)
        $destination = Join-Path $caseDirectory 'must-not-extract'
        $snapshot = Tree-Digest $caseDirectory
        $script:entryOpenCount = 0
        Refusal { Expand-CheckedPackage $modified $sums $destination }
        Require ($script:entryOpenCount -eq 0 -and -not (Test-Path -LiteralPath $destination)) 'Unsafe archive reached a member stream or destination.'
        Observe @{case = ('archive-' + $item.id); boundary = 'archive'; path = $name; rejected = $true; preserved = ((Tree-Digest $caseDirectory) -ceq $snapshot); entry_open_count = $script:entryOpenCount; destination_absent = $true}
    }

    # Child probes execute the current install/uninstall try/catch/finally AST,
    # not a copied decision algorithm. Only platform/root/lock are fixtures;
    # real inventory parsing and the expander above remain in the call chain.
    # Fail-fast boundary functions prevent any user/PATH/candidate execution
    # if the intended early package-name refusal regresses.
    $probePath = Join-Path $work 'operation-probe.ps1'
    $probe = @'
param([string]$Common, [string]$Product, [string]$Expander, [string]$CasePath, [string]$ResultPath)
. $Common
. $Expander
function Require([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
$item = [IO.File]::ReadAllText($CasePath, [Text.Encoding]::UTF8) | ConvertFrom-Json
$script:fixtureRoot = [string]$item.root
$script:inventoryCalls = 0; $script:writeBoundaries = 0
$script:realInventory = ${function:Read-Installation}
function Read-Installation([string]$Root) {
    Require ($Root -ceq $script:fixtureRoot) 'Unexpected inventory target.'
    $script:inventoryCalls++
    return (& $script:realInventory $Root)
}
function Assert-WindowsPlatform {}
function Get-InstallRoot { return $script:fixtureRoot }
function Enter-InstallLock([string]$Root) { Require ($Root -ceq $script:fixtureRoot) 'Unexpected lock target.'; return $null }
function Read-UserPath { $script:writeBoundaries++; throw 'UNEXPECTED_POST_VALIDATION_BOUNDARY' }
function Write-UserPath { $script:writeBoundaries++; throw 'FORBIDDEN_USER_PATH_WRITE' }
function Update-ProcessPath { $script:writeBoundaries++; throw 'FORBIDDEN_PROCESS_PATH_WRITE' }
function Save-InstallState { $script:writeBoundaries++; throw 'FORBIDDEN_INSTALL_STATE_WRITE' }
function Copy-Inventory { $script:writeBoundaries++; throw 'FORBIDDEN_INSTALL_COPY' }
$tokens = $null; $errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($Product, [ref]$tokens, [ref]$errors)
Require ($errors.Count -eq 0) 'Cannot parse product operation.'
$tries = @($ast.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.TryStatementAst] })
$initializers = @($ast.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.AssignmentStatementAst] })
Require ($tries.Count -eq 1) 'Expected one product operation try block.'
$expectedState = if ($item.action -eq 'install') { '$mutex,$scratch,$backup,$committed' } else { '$mutex,$backup,$committed' }
Require ((@($initializers | ForEach-Object { $_.Left.Extent.Text }) -join ',') -ceq $expectedState) 'Unexpected product initializers.'
$commands = @($tries[0].Body.Statements | Where-Object { $_ -is [Management.Automation.Language.PipelineAst] })
Require ($commands[0].Extent.Text -ceq 'Assert-WindowsPlatform') 'Product platform guard is absent or reordered.'
$flow = [scriptblock]::Create((@($initializers | ForEach-Object { $_.Extent.Text }) + @($tries[0].Extent.Text)) -join "`n")
$Archive = [string]$item.archive; $Checksums = [string]$item.checksums; $AddToPath = $true
try { & $flow }
finally {
    $result = @{shell_version = $PSVersionTable.PSVersion.ToString(); inventory_calls = $script:inventoryCalls; write_boundary_count = $script:writeBoundaries; entry_open_count = $script:entryOpenCount}
    [IO.File]::WriteAllText($ResultPath, (ConvertTo-Json -Compress -InputObject $result), (New-Object Text.UTF8Encoding($false)))
}
'@
    [IO.File]::WriteAllText($probePath, $probe, $utf8)
    foreach ($case in @('install-archive-refusal', 'reinstall-inventory-refusal', 'update-inventory-refusal', 'uninstall-inventory-refusal')) {
        $directory = Join-Path $work $case
        [IO.Directory]::CreateDirectory($directory) | Out-Null
        $installation = Join-Path $directory 'Jaekit'
        $selectedArchive = $archive; $selectedChecksums = $checksums
        $action = if ($case -eq 'uninstall-inventory-refusal') { 'uninstall' } else { 'install' }
        if ($case -eq 'install-archive-refusal') {
            $selectedArchive = Join-Path (Join-Path $work 'conin-root') ([IO.Path]::GetFileName($archive))
            $selectedChecksums = Join-Path (Join-Path $work 'conin-root') 'checksums.txt'
        } else {
            $legacyVersion = if ($case -eq 'update-inventory-refusal') { '0.0.0-legacy' } else { $candidateVersion }
            Require ($case -ne 'update-inventory-refusal' -or $legacyVersion -cne $candidateVersion) 'Update fixture must use a different version.'
            Make-Installation $installation $legacyVersion 'assets/CONOUT$.safe'
        }
        $sentinel = Join-Path $directory 'project-unrelated.txt'
        [IO.File]::WriteAllText($sentinel, 'preserve synthetic project and unrelated data', $utf8)
        $snapshot = Tree-Digest $directory
        $casePath = Join-Path $work ($case + '.json')
        $resultPath = Join-Path $work ($case + '-result.json')
        Save-Json $casePath @{root = $installation; archive = $selectedArchive; checksums = $selectedChecksums; action = $action}
        $result = Run-Child $probePath @('-Common', $common, '-Product', (Join-Path $PSScriptRoot ($action + '.ps1')), '-Expander', $expanderPath, '-CasePath', $casePath, '-ResultPath', $resultPath) (Join-Path $work ($case + '.log'))
        $observed = [IO.File]::ReadAllText($resultPath, [Text.Encoding]::UTF8) | ConvertFrom-Json
        $comparison = [regex]::Replace([regex]::Replace($result.text, '\x1B\[[0-?]*[ -/]*[@-~]', ''), '\s+', ' ')
        Require ($result.code -eq 1 -and $comparison.Contains('Unsafe package path:')) ('Wrong product refusal: ' + $case + ': ' + $result.text)
        Require ($observed.shell_version -ceq $report.shell_version -and $observed.inventory_calls -eq 1) 'Product inventory boundary was not called in this shell.'
        Require ($observed.write_boundary_count -eq 0 -and $observed.entry_open_count -eq 0) 'Product refusal reached a later mutation/data-open boundary.'
        $preserved = (Tree-Digest $directory) -ceq $snapshot
        Require $preserved ('Refusal changed installation, scratch, or project data: ' + $case)
        Observe @{case = $case; rejected = $true; preserved = $preserved; exit_code = $result.code; write_boundary_count = $observed.write_boundary_count; entry_open_count = $observed.entry_open_count; source_inventory_called = $true}
    }
    Require ($report.passed.Count -eq $report.required.Count) 'Not all console path cases executed.'
    Require (@($report.observations | Where-Object { -not $_.preserved }).Count -eq 0) 'A console fixture changed its input data.'
    $report.status = 'passed'
} catch { $report.failures += $_.ToString() }
finally {
    if ($null -ne $before) {
        try {
            $after = Snapshot-Environment
            $report.environment_after_sha256 = String-Digest $after
            $report.environment_preserved = $before -ceq $after
            if (-not $report.environment_preserved) { $report.status = 'failed'; $report.failures += 'Real user state changed.' }
        } catch { $report.status = 'failed'; $report.failures += $_.ToString() }
    }
    if ($canWrite) { Save-Json $output $report }
}
if ($report.status -ne 'passed') { throw ('Console path observations failed: ' + ($report.failures -join '; ')) }
