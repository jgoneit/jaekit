# Native developer observations of product PATH selection. No installation or
# user/machine PATH writes; process updates run only in isolated child shells.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$OutputPath)
. (Join-Path $PSScriptRoot 'common.ps1')
$caseNames = @('first-exact-order', 'multiple-later-duplicates', 'unowned-exact-preserved',
    'owned-literal-missing', 'alias-preserved', 'other-spellings-preserved',
    'empty-segments-preserved', 'only-owned-entry', 'empty-path-preserved')
$processNames = @($caseNames | ForEach-Object {
    if ($_ -eq 'unowned-exact-preserved') { 'process-add-existing' } else { 'process-remove-' + $_ }
})
$uninstallNames = @($caseNames | ForEach-Object { 'uninstall-' + $_ })
$uninstallNames += @('uninstall-owned-preexisting-empty', 'uninstall-already-absent-value',
    'uninstall-string-kind-preserved', 'uninstall-concurrent-value',
    'uninstall-concurrent-kind', 'uninstall-concurrent-existence')
$required = @($caseNames) + @($processNames) + @($uninstallNames)
$report = [ordered]@{
    schema = 'jaekit-windows-path-order/v1'; status = 'failed'
    shell_version = $PSVersionTable.PSVersion.ToString()
    required = @($required); passed = @(); skipped = @(); failures = @()
    source_sha256 = @{}; before_sha256 = ''; after_sha256 = ''; environment_preserved = $false
    unverified = @('real-registry-write', 'platform-inventory-lock-fixture-seams', 'windows11-install-lifecycle')
}
$utf8 = New-Object Text.UTF8Encoding($false)
$canWrite = $false
$before = $null

function Require([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Snapshot-Environment {
    $user = Read-UserPath
    # Keep raw environment values in memory only, never in the JSON report.
    return (ConvertTo-Json -Compress -Depth 5 -InputObject ([ordered]@{
        user_value = $user.value; user_exists = $user.exists; user_kind = [int]$user.kind
        machine = [Environment]::GetEnvironmentVariable('Path', 'Machine')
        process = $env:Path
        policy = @(Get-ExecutionPolicy -List | ForEach-Object { [ordered]@{scope = [string]$_.Scope; policy = [string]$_.ExecutionPolicy} })
    }))
}

function String-Digest([string]$Value) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Run-Child([string]$Script, [string[]]$Arguments, [string]$Log) {
    $oldPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $PSNativeCommandUseErrorActionPreference = $false
        $messages = @(& $hostExecutable -NoProfile -NonInteractive -File $Script @Arguments 2>&1)
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $oldPreference }
    $text = ($messages | ForEach-Object { $_.ToString() }) -join "`n"
    [IO.File]::WriteAllText($Log, $text, $utf8)
    return @{code = $code; text = $text}
}

try {
    Require ($env:OS -eq 'Windows_NT') 'Native PATH observations require actual Windows.'
    $version = $PSVersionTable.PSVersion
    Require (($version.Major -eq 5 -and $version.Minor -eq 1) -or $version.Major -eq 7) 'Use PowerShell 5.1 or 7.'
    $before = Snapshot-Environment
    $report.before_sha256 = String-Digest $before
    $output = Assert-OrdinaryPath $OutputPath
    Require (-not (Test-Path -LiteralPath $output)) 'Refuse to replace a PATH report.'
    $work = Join-Path ([IO.Path]::GetDirectoryName($output)) 'path-runtime'
    Require (-not (Test-Path -LiteralPath $work)) 'Refuse to replace PATH fixtures.'
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $canWrite = $true
    $common = Join-Path $PSScriptRoot 'common.ps1'
    $uninstall = Join-Path $PSScriptRoot 'uninstall.ps1'
    $report.source_sha256 = @{common = (Get-Sha256 $common); uninstall = (Get-Sha256 $uninstall)}
    $hostExecutable = (Get-Process -Id $PID).Path
    $childScript = Join-Path $work 'process-probe.ps1'
    # The notification seam prevents a broadcast; the PATH transformation is
    # the actual Update-ProcessPath function loaded from the product source.
    $child = @'
param([string]$Common, [string]$CasePath, [string]$ResultPath)
. $Common
Add-Type -TypeDefinition 'namespace Jaekit { public static class InstallPlatform { public static int Notifications; public static void NotifyEnvironment() { Notifications++; } } }'
$item = [IO.File]::ReadAllText($CasePath, [Text.Encoding]::UTF8) | ConvertFrom-Json
$env:Path = [string]$item.value
Update-ProcessPath -Bin ([string]$item.bin) -Add ([bool]$item.add)
$result = [ordered]@{value = [string]$env:Path; notifications = [Jaekit.InstallPlatform]::Notifications; shell_version = $PSVersionTable.PSVersion.ToString()}
[IO.File]::WriteAllText($ResultPath, (ConvertTo-Json -Compress -InputObject $result), (New-Object Text.UTF8Encoding($false)))
'@
    [IO.File]::WriteAllText($childScript, $child, $utf8)
    $uninstallScript = Join-Path $work 'uninstall-probe.ps1'
    # Execute the actual uninstall orchestration, with fixture-only platform,
    # root, inventory and lock dependencies. The real Write-UserPath function
    # retains every statement except one AST-identified registry factory call.
    # The replacement object implements only its SetValue/DeleteValue/Dispose
    # effects in memory; no real registry writer is reachable by this probe.
    $uninstallChild = @'
param([string]$Common, [string]$Uninstall, [string]$CasePath, [string]$ResultPath)
. $Common
function Require([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
$tokens = $null; $errors = $null
$commonAst = [Management.Automation.Language.Parser]::ParseFile($Common, [ref]$tokens, [ref]$errors)
Require ($errors.Count -eq 0) 'Cannot parse product common source.'
$writeFunctions = @($commonAst.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -ceq 'Write-UserPath'}, $true))
Require ($writeFunctions.Count -eq 1) 'Expected one product Write-UserPath function.'
$write = $writeFunctions[0]
$factoryText = "[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')"
$factories = @($write.FindAll({param($node) $node -is [Management.Automation.Language.InvokeMemberExpressionAst] -and $node.Extent.Text -ceq $factoryText}, $true))
Require ($factories.Count -eq 1) 'Expected exactly one product registry factory.'
$original = $write.Extent.Text
Require ([regex]::Matches($original, [regex]::Escape('[Microsoft.Win32.Registry]')).Count -eq 1) 'Unexpected additional registry access.'
$offset = $factories[0].Extent.StartOffset - $write.Extent.StartOffset
$adapter = '(Open-FixtureRegistry)'
$adapted = $original.Remove($offset, $factoryText.Length).Insert($offset, $adapter)
Require ($adapted.Remove($offset, $adapter.Length).Insert($offset, $factoryText) -ceq $original) 'Registry adapter changed other product statements.'
Require (-not $adapted.Contains('[Microsoft.Win32.Registry]')) 'Real registry writer remains in probe.'
$uninstallAst = [Management.Automation.Language.Parser]::ParseFile($Uninstall, [ref]$tokens, [ref]$errors)
Require ($errors.Count -eq 0) 'Cannot parse product uninstaller.'
$tryBlocks = @($uninstallAst.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.TryStatementAst] })
$initializers = @($uninstallAst.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.AssignmentStatementAst] })
Require ($tryBlocks.Count -eq 1 -and $initializers.Count -eq 3) 'Unexpected product uninstaller orchestration.'
Require ((@($initializers | ForEach-Object { $_.Left.Extent.Text }) -join ',') -ceq '$mutex,$backup,$committed') 'Unexpected product uninstaller state.'
$flowSource = (@($initializers | ForEach-Object { $_.Extent.Text }) + @($tryBlocks[0].Extent.Text)) -join "`n"
$flow = [scriptblock]::Create($flowSource)
$item = [IO.File]::ReadAllText($CasePath, [Text.Encoding]::UTF8) | ConvertFrom-Json
$script:fixtureRoot = [string]$item.root
$script:fixtureItem = $item
$script:readCount = 0; $script:factoryCount = 0; $script:writeCount = 0
$script:pathState = @{value = [string]$item.value; exists = [bool]$item.exists; kind = [Microsoft.Win32.RegistryValueKind][int]$item.kind}
$script:registry = New-Object PSObject
$script:registry | Add-Member ScriptMethod SetValue {
    param($Name, $Value, $Kind)
    Require ($Name -ceq 'Path') 'Unexpected fixture registry value.'
    $script:writeCount++
    $script:pathState = @{value = [string]$Value; exists = $true; kind = $Kind}
}
$script:registry | Add-Member ScriptMethod DeleteValue {
    param($Name, $MissingThrows)
    Require ($Name -ceq 'Path' -and -not $MissingThrows) 'Unexpected fixture registry deletion.'
    $script:writeCount++
    $script:pathState = @{value = ''; exists = $false; kind = [Microsoft.Win32.RegistryValueKind]::ExpandString}
}
$script:registry | Add-Member ScriptMethod Dispose {}
function Open-FixtureRegistry { $script:factoryCount++; return $script:registry }
function Read-UserPath {
    $script:readCount++
    if ($script:readCount -eq 2) {
        switch ($script:fixtureItem.concurrent) {
            'value' { $script:pathState.value += ';concurrent-user-entry' }
            'kind' { $script:pathState.kind = [Microsoft.Win32.RegistryValueKind]::String }
            'existence' { $script:pathState.value = ''; $script:pathState.exists = $false }
        }
    }
    return @{value = $script:pathState.value; exists = $script:pathState.exists; kind = $script:pathState.kind}
}
function Assert-WindowsPlatform {}
function Get-InstallRoot { return $script:fixtureRoot }
function Enter-InstallLock([string]$Root) { Require ($Root -ceq $script:fixtureRoot) 'Unexpected lock target.'; return $null }
function Read-Installation([string]$Root) {
    Require ($Root -ceq $script:fixtureRoot) 'Unexpected inventory target.'
    return @{state = @{owns_path = [bool]$script:fixtureItem.owned; created_path_value = [bool]$script:fixtureItem.created}}
}
. ([scriptblock]::Create($adapted))
Add-Type -TypeDefinition 'namespace Jaekit { public static class InstallPlatform { public static int Notifications; public static void NotifyEnvironment() { Notifications++; } } }'
$env:Path = [string]$item.value
try { & $flow }
finally {
    $sentinel = Join-Path $script:fixtureRoot 'sentinel.txt'
    $result = [ordered]@{
        value = $script:pathState.value; exists = $script:pathState.exists; kind = [int]$script:pathState.kind
        factory_calls = $script:factoryCount; writes = $script:writeCount
        install_exists = [IO.Directory]::Exists($script:fixtureRoot)
        sentinel = $(if ([IO.File]::Exists($sentinel)) { [IO.File]::ReadAllText($sentinel) } else { $null })
        backups = @([IO.Directory]::GetFileSystemEntries([IO.Path]::GetDirectoryName($script:fixtureRoot), '.jaekit-remove-*')).Count
        process_value = [string]$env:Path; notifications = [Jaekit.InstallPlatform]::Notifications
        shell_version = $PSVersionTable.PSVersion.ToString()
    }
    [IO.File]::WriteAllText($ResultPath, (ConvertTo-Json -Compress -Depth 5 -InputObject $result), (New-Object Text.UTF8Encoding($false)))
}
'@
    [IO.File]::WriteAllText($uninstallScript, $uninstallChild, $utf8)
    $suffix = '\jaekit synthetic ' + [char]0xAC00 + '\bin'
    $bin = $env:SystemDrive + $suffix
    $alias = '%SystemDrive%' + $suffix
    Require (Test-PathEntry $alias $bin) 'Synthetic alias does not resolve to the test literal.'
    $quoted = '"' + $bin + '"'
    $upper = $bin.ToUpperInvariant()
    $trailing = $bin + '\'
    $cases = @(
        @{id = 'first-exact-order'; value = "A;$bin;X;$bin;Y"; owned = $true; expected = "A;X;$bin;Y"; changed = $true},
        @{id = 'multiple-later-duplicates'; value = "A;$bin;X;$bin;Y;$bin;Z"; owned = $true; expected = "A;X;$bin;Y;$bin;Z"; changed = $true},
        @{id = 'unowned-exact-preserved'; value = "A;$bin;X;$bin;Y"; owned = $false; expected = "A;$bin;X;$bin;Y"; changed = $false},
        @{id = 'owned-literal-missing'; value = "A;$alias;X;$quoted;Y"; owned = $true; expected = "A;$alias;X;$quoted;Y"; changed = $false},
        @{id = 'alias-preserved'; value = "A;$bin;X;$alias;Y"; owned = $true; expected = "A;X;$alias;Y"; changed = $true},
        @{id = 'other-spellings-preserved'; value = "A;$quoted;$bin;X;$upper;$trailing;Y"; owned = $true; expected = "A;$quoted;X;$upper;$trailing;Y"; changed = $true},
        @{id = 'empty-segments-preserved'; value = ";A;;$bin;X;;$bin;"; owned = $true; expected = ";A;;X;;$bin;"; changed = $true},
        @{id = 'only-owned-entry'; value = $bin; owned = $true; expected = ''; changed = $true},
        @{id = 'empty-path-preserved'; value = ''; owned = $true; expected = ''; changed = $false}
    )
    Require ($cases.Count -eq $caseNames.Count) 'PATH fixture inventory differs.'
    foreach ($item in $cases) {
        $actual = Get-OwnedPathRemoval $item.value $bin $item.owned
        Require ($actual.value -ceq $item.expected -and $actual.changed -eq $item.changed) ('Product removal result differs: ' + $item.id)
        $report.passed += $item.id
        $casePath = Join-Path $work ($item.id + '.json')
        $resultPath = Join-Path $work ($item.id + '-result.json')
        # No-owned removal is a pure ownership decision. The child instead
        # observes that the existing Add branch does not claim/duplicate an
        # already-present literal. All owned cases use actual removal.
        $add = -not $item.owned
        $inputCase = [ordered]@{value = $item.value; bin = $bin; add = [bool]$add}
        [IO.File]::WriteAllText($casePath, (ConvertTo-Json -Compress -InputObject $inputCase), $utf8)
        $result = Run-Child $childScript @('-Common', $common, '-CasePath', $casePath, '-ResultPath', $resultPath) (Join-Path $work ($item.id + '.log'))
        Require ($result.code -eq 0 -and [IO.File]::Exists($resultPath)) ('Actual product process PATH branch failed: ' + $item.id)
        $processResult = [IO.File]::ReadAllText($resultPath, [Text.Encoding]::UTF8) | ConvertFrom-Json
        Require ($processResult.shell_version -ceq $report.shell_version) 'Child used a different PowerShell version.'
        Require ($processResult.value -ceq $item.expected -and $processResult.notifications -eq 1) ('Actual process PATH order/notification differs: ' + $item.id)
        $report.passed += $(if ($add) { 'process-add-existing' } else { 'process-remove-' + $item.id })
    }
    $uninstallCases = @($cases)
    $uninstallCases += @(
        @{id = 'owned-preexisting-empty'; value = $bin; owned = $true; expected = ''; changed = $true},
        @{id = 'already-absent-value'; value = ''; owned = $true; expected = ''; changed = $false},
        @{id = 'string-kind-preserved'; value = "A;$bin;X;$bin;Y"; owned = $true; expected = "A;X;$bin;Y"; changed = $true}
    )
    foreach ($field in @('value', 'kind', 'existence')) {
        $uninstallCases += @{id = ('concurrent-' + $field); value = "A;$bin;X;$bin;Y"; owned = $true; expected = ''; changed = $true; concurrent = $field}
    }
    foreach ($item in $uninstallCases) {
        $name = 'uninstall-' + $item.id
        $caseRoot = Join-Path $work $name
        $installRoot = Join-Path $caseRoot 'install'
        [IO.Directory]::CreateDirectory((Join-Path $installRoot 'bin')) | Out-Null
        [IO.File]::WriteAllText((Join-Path $installRoot 'sentinel.txt'), 'synthetic managed sentinel', $utf8)
        [IO.File]::WriteAllText((Join-Path $installRoot 'bin\ha.exe'), 'synthetic unlocked file', $utf8)
        $outside = Join-Path $caseRoot 'outside.txt'
        [IO.File]::WriteAllText($outside, 'unrelated synthetic data', $utf8)
        # The actual uninstaller derives its literal from Get-InstallRoot.
        # Substitute that fixture path in the data, never in product code.
        $literal = Join-Path $installRoot 'bin'
        $value = $item.value.Replace($bin, $literal)
        $expected = $item.expected.Replace($bin, $literal)
        $kind = if ($item.id -eq 'string-kind-preserved') { 1 } else { 2 }
        $exists = $item.id -ne 'already-absent-value'
        $created = $item.id -eq 'only-owned-entry'
        $concurrent = if ($item.ContainsKey('concurrent')) { $item.concurrent } else { '' }
        $inputCase = [ordered]@{root = $installRoot; value = $value; owned = [bool]$item.owned; created = [bool]$created; exists = [bool]$exists; kind = $kind; concurrent = $concurrent}
        $casePath = Join-Path $caseRoot 'case.json'
        $resultPath = Join-Path $caseRoot 'result.json'
        [IO.File]::WriteAllText($casePath, (ConvertTo-Json -Compress -InputObject $inputCase), $utf8)
        $result = Run-Child $uninstallScript @('-Common', $common, '-Uninstall', $uninstall, '-CasePath', $casePath, '-ResultPath', $resultPath) (Join-Path $caseRoot 'child.log')
        Require ([IO.File]::Exists($resultPath)) ('Product uninstall probe did not finish: ' + $name)
        $observed = [IO.File]::ReadAllText($resultPath, [Text.Encoding]::UTF8) | ConvertFrom-Json
        Require ($observed.shell_version -ceq $report.shell_version) 'Uninstall probe used a different PowerShell.'
        if ($concurrent) {
            Require ($result.code -ne 0 -and $result.text.Contains('User PATH changed concurrently; nothing was overwritten.')) ('Wrong or missing concurrency refusal: ' + $name)
            $currentValue = if ($concurrent -eq 'value') { $value + ';concurrent-user-entry' } elseif ($concurrent -eq 'existence') { '' } else { $value }
            $currentKind = if ($concurrent -eq 'kind') { 1 } else { $kind }
            $currentExists = $concurrent -ne 'existence'
            Require ($observed.value -ceq $currentValue -and $observed.kind -eq $currentKind -and $observed.exists -eq $currentExists) ('Concurrent user state was overwritten: ' + $name)
            Require ($observed.factory_calls -eq 0 -and $observed.writes -eq 0 -and $observed.notifications -eq 0) ('Refusal reached a write/notification: ' + $name)
            Require ($observed.install_exists -and $observed.sentinel -ceq 'synthetic managed sentinel') ('Product rollback did not preserve installation: ' + $name)
            Require ($observed.process_value -ceq $value) ('Refusal changed child process PATH: ' + $name)
        } else {
            Require ($result.code -eq 0 -and -not $observed.install_exists) ('Product uninstall did not succeed: ' + $name)
            $expectedExists = if ($created -and -not $expected) { $false } else { $exists }
            Require ($observed.value -ceq $expected -and $observed.exists -eq $expectedExists -and $observed.kind -eq $kind) ('Product user PATH value/kind/presence differs: ' + $name)
            $writes = if ($item.changed) { 1 } else { 0 }
            Require ($observed.writes -eq $writes -and $observed.factory_calls -eq $writes -and $observed.notifications -eq $writes) ('Product ownership/write decision differs: ' + $name)
            Require ($observed.process_value -ceq $expected) ('Uninstall/process PATH connection differs: ' + $name)
        }
        Require ($observed.backups -eq 0) ('Product left a removal backup: ' + $name)
        Require ([IO.File]::ReadAllText($outside) -ceq 'unrelated synthetic data') ('Product removal changed unrelated data: ' + $name)
        $report.passed += $name
    }
    $after = Snapshot-Environment
    $report.after_sha256 = String-Digest $after
    $report.environment_preserved = $after -ceq $before
    Require $report.environment_preserved 'PATH observations changed host user/machine/process PATH or policy.'
    Require ($report.passed.Count -eq $required.Count) 'Not all PATH cases executed.'
    $report.status = 'passed'
} catch {
    $report.failures += $_.ToString()
} finally {
    if ($canWrite) { [IO.File]::WriteAllText($output, (ConvertTo-Json -Depth 8 -InputObject $report) + "`n", $utf8) }
}
if ($report.status -ne 'passed') { throw ($report.failures -join '; ') }
