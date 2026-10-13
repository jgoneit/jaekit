# Trusted developer fixture: the public example drives the production planner
# and launcher, extracted by AST, against inert child scripts. No Client guard
# is bypassed and no real installation or user PATH is modified.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$OutputPath)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$utf8 = New-Object Text.UTF8Encoding($false)
$report = [ordered]@{
    schema = 'jaekit-windows-acceptance-dispatch/v1'; status = 'failed'
    shell_version = $PSVersionTable.PSVersion.ToString(); source_sha256 = @{}; source_function_sha256 = @{}
    production_stages = @(); required = @(); passed = @(); skipped = @(); stages = @(); failure_observations = @()
    environment_preserved = $false; environment_before_sha256 = ''; environment_after_sha256 = ''; failures = @()
}
$canWrite = $false
$environmentBefore = $null
$oldEnvironment = @{}
foreach ($name in @('JAEKIT_ACCEPTANCE_CASE', 'JAEKIT_ACCEPTANCE_EVENTS', 'JAEKIT_ACCEPTANCE_FAILURE',
                    'JAEKIT_ACCEPTANCE_STAGE', 'JAEKIT_ACCEPTANCE_SHELL51', 'JAEKIT_ACCEPTANCE_SHELL7')) {
    $oldEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
function Require([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Text-Hash([string]$Text) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
function Snapshot-Environment {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $false)
    try {
        $userPath = if ($null -eq $key) { $null } else { $key.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) }
        $kind = if ($null -eq $userPath) { $null } else { [int]$key.GetValueKind('Path') }
    } finally { if ($null -ne $key) { $key.Dispose() } }
    $local = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    return (ConvertTo-Json -Compress -Depth 6 -InputObject ([ordered]@{
        user_path = $userPath; user_path_kind = $kind
        process_path = $env:Path; machine_path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
        installation_exists = (Test-Path -LiteralPath (Join-Path $local 'Jaekit'))
        installer_scratch = @([IO.Directory]::GetFileSystemEntries($local, '.jaekit-*') | Sort-Object)
        execution_policy = @(Get-ExecutionPolicy -List | ForEach-Object { [ordered]@{scope = [string]$_.Scope; policy = [string]$_.ExecutionPolicy} })
    }))
}
function Read-PublicBlock([string]$Path, [string]$Label) {
    $text = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8).Replace("`r`n", "`n")
    $begin = '<!-- BEGIN WINDOWS ' + $Label + ' -->'; $end = '<!-- END WINDOWS ' + $Label + ' -->'
    Require ([regex]::Matches($text, [regex]::Escape($begin)).Count -eq 1 -and [regex]::Matches($text, [regex]::Escape($end)).Count -eq 1) 'Missing or ambiguous public example.'
    $match = [regex]::Match($text, '(?s)' + [regex]::Escape($begin) + '\s*```powershell\n(.+?)\n```\s*' + [regex]::Escape($end))
    Require $match.Success ('Malformed public example: ' + $Label)
    return $match.Groups[1].Value
}
function Write-Fixture([string]$Directory, [string]$Origin, [string]$Acceptance) {
    [IO.Directory]::CreateDirectory($Directory) | Out-Null
    $version = if ($Origin -eq 'first') { '0.1.4-dev' } else { '0.1.5-dev' }
    $archive = 'ha_' + $version + '_windows_amd64.zip'
    [IO.File]::WriteAllText((Join-Path $Directory $archive), 'inert candidate archive', $utf8)
    $common = @'
$event = @{stage=$env:JAEKIT_ACCEPTANCE_STAGE; origin='__ORIGIN__'; entrypoint='helper'; shell=$PSVersionTable.PSVersion.ToString()}
[IO.File]::AppendAllText($env:JAEKIT_ACCEPTANCE_EVENTS, (ConvertTo-Json -Compress -InputObject $event) + "`n")
if ('__ORIGIN__' -eq 'update' -and $env:JAEKIT_ACCEPTANCE_CASE -eq 'update-helper-failure') { throw 'INJECTED_UPDATE_HELPER_FAILURE' }
'@
    [IO.File]::WriteAllText((Join-Path $Directory 'common.ps1'), $common.Replace('__ORIGIN__', $Origin), $utf8)
    foreach ($action in @('install', 'uninstall')) {
        $body = @'
[CmdletBinding()]
param([string]$Archive, [string]$Checksums, [switch]$AddToPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$event = @{stage=$env:JAEKIT_ACCEPTANCE_STAGE; origin='__ORIGIN__'; entrypoint='__ACTION__'; shell=$PSVersionTable.PSVersion.ToString(); archive=$Archive; checksums=$Checksums; path_consent=[bool]$AddToPath}
[IO.File]::AppendAllText($env:JAEKIT_ACCEPTANCE_EVENTS, (ConvertTo-Json -Compress -InputObject $event) + "`n")
if ('__ORIGIN__' -eq 'update' -and '__ACTION__' -eq 'install' -and $env:JAEKIT_ACCEPTANCE_CASE -eq 'update-installer-failure') { throw 'INJECTED_UPDATE_INSTALLER_FAILURE' }
if ('__ORIGIN__' -eq 'update' -and '__ACTION__' -eq 'uninstall' -and $env:JAEKIT_ACCEPTANCE_CASE -eq 'update-uninstaller-failure') { throw 'INJECTED_UPDATE_UNINSTALLER_FAILURE' }
$diagnostic = switch ($env:JAEKIT_ACCEPTANCE_STAGE) {
    'checksum-refusal' { 'Archive checksum is missing, duplicated or does not match.' }
    'update-unmanaged-refusal' { 'Unmanaged file; nothing will be replaced: inert fixture' }
    'update-locked-refusal' { 'The current executable cannot be replaced (inert fixture)' }
    'junction-install-refusal' { 'Link or reparse-point path is not allowed: inert fixture' }
    'junction-remove-refusal' { 'Link or reparse-point path is not allowed: inert fixture' }
    default { '' }
}
if ($env:JAEKIT_ACCEPTANCE_CASE -eq 'unexpected-refusal-diagnostic' -and $env:JAEKIT_ACCEPTANCE_STAGE -eq 'checksum-refusal') { $diagnostic = 'UNRELATED_SETUP_FAILURE' }
if ($diagnostic) {
    # Exercise the product's Write-Error path, including wrapped messages.
    Write-Error -ErrorAction Continue ('Installation failed: ' + $diagnostic.Replace(' ', " `n"))
    exit 1
}
exit 0
'@
        [IO.File]::WriteAllText((Join-Path $Directory ($action + '.ps1')), $body.Replace('__ORIGIN__', $Origin).Replace('__ACTION__', $action), $utf8)
    }
    [IO.File]::WriteAllText((Join-Path $Directory 'acceptance.ps1'), $Acceptance, $utf8)
    $lines = @()
    foreach ($name in @($archive, 'common.ps1', 'install.ps1', 'uninstall.ps1', 'acceptance.ps1')) {
        $lines += (Get-FileHash -LiteralPath (Join-Path $Directory $name) -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $name
    }
    [IO.File]::WriteAllLines((Join-Path $Directory 'checksums.txt'), [string[]]$lines, [Text.Encoding]::ASCII)
}

try {
    Require ($env:OS -eq 'Windows_NT') 'Acceptance dispatcher checks require actual Windows.'
    $version = $PSVersionTable.PSVersion
    Require (($version.Major -eq 5 -and $version.Minor -eq 1) -or $version.Major -eq 7) 'Use PowerShell 5.1 or 7.'
    $output = [IO.Path]::GetFullPath($OutputPath)
    Require (-not (Test-Path -LiteralPath $output)) 'Refuse to replace acceptance observations.'
    $work = Join-Path ([IO.Path]::GetDirectoryName($output)) ('acceptance-runtime ' + [char]0xd55c + [char]0xae00)
    Require (-not (Test-Path -LiteralPath $work)) 'Refuse to replace acceptance fixtures.'
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $canWrite = $true
    $environmentBefore = Snapshot-Environment
    $report.environment_before_sha256 = Text-Hash $environmentBefore
    $root = [IO.Directory]::GetParent([IO.Directory]::GetParent($PSScriptRoot).FullName).FullName
    $acceptancePath = Join-Path $PSScriptRoot 'acceptance.ps1'
    $readmePath = Join-Path $PSScriptRoot 'README.md'
    foreach ($name in @('tools/windows/acceptance.ps1', 'tools/windows/README.md')) {
        $report.source_sha256[$name] = (Get-FileHash -LiteralPath (Join-Path $root $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    $source = [IO.File]::ReadAllText($acceptancePath, [Text.Encoding]::UTF8).Replace("`r`n", "`n")
    $tokens = $null; $errors = $null
    $ast = [Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors)
    Require ($errors.Count -eq 0) 'The production acceptance script does not parse.'
    $functions = ''
    foreach ($functionName in @('Get-JaekitAcceptanceSteps', 'Invoke-JaekitAcceptanceStep')) {
        $found = @($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $functionName}, $true))
        Require ($found.Count -eq 1) ('Production function is missing or ambiguous: ' + $functionName)
        $text = $found[0].Extent.Text
        $report.source_function_sha256[$functionName] = Text-Hash $text
        $functions += $text + "`n"
    }
    $bindings = [regex]::Matches($source, '(?s)# BEGIN ACCEPTANCE DISPATCH BINDING\n(.*?)\n\s*# END ACCEPTANCE DISPATCH BINDING')
    Require ($bindings.Count -eq 1) 'Production parameter binding is missing or ambiguous.'
    $binding = $bindings[0].Groups[1].Value
    Require ($binding.Trim() -ceq '$steps = Get-JaekitAcceptanceSteps @PSBoundParameters -EntryDirectory $PSScriptRoot -Shell51 $shell51 -Shell7 $shell7 -BadArchive $badZip') 'Production does not bind its actual parameters to the planner.'
    # Verify the real top-level orchestration calls the same planner/launcher.
    # Nested function definitions are excluded from these invocation queries.
    $commands = @($ast.EndBlock.FindAll({param($node) $node -is [Management.Automation.Language.CommandAst]}, $false))
    $planCalls = @($commands | Where-Object { $_.GetCommandName() -eq 'Get-JaekitAcceptanceSteps' })
    $stepCalls = @($commands | Where-Object { $_.GetCommandName() -eq 'Invoke-JaekitAcceptanceStep' })
    Require ($planCalls.Count -eq 1) 'Production does not execute exactly one planner binding.'
    $stageNames = @()
    foreach ($call in $stepCalls) {
        $match = [regex]::Match($call.Extent.Text, '^Invoke-JaekitAcceptanceStep \$steps\[''([^'']+)''\]$')
        Require $match.Success 'Production stage invocation bypasses the bound plan.'
        $stageNames += $match.Groups[1].Value
    }
    $expectedStages = @('first-install', 'first-reinstall', 'checksum-refusal', 'update-unmanaged-refusal', 'update-locked-refusal',
        'update-install', 'update-reinstall', 'update-remove', 'first-install-again', 'first-remove', 'junction-install-refusal', 'junction-remove-refusal')
    Require (($stageNames -join '|') -ceq ($expectedStages -join '|')) 'Production lifecycle stages are missing or reordered.'
    $guardCalls = @($commands | Where-Object { $_.GetCommandName() -eq 'Assert-WindowsPlatform' })
    Require ($guardCalls.Count -eq 1 -and $guardCalls[0].Extent.StartOffset -lt $planCalls[0].Extent.StartOffset) 'Client guard must remain before actual lifecycle execution.'
    $report.production_stages = @($stageNames)
    $report.required = @($stageNames | ForEach-Object { 'dispatch-' + $_ }) + @('update-installer-failure', 'update-helper-failure', 'update-uninstaller-failure', 'missing-update-script', 'wrong-update-script', 'shell-start-failure', 'unexpected-refusal-diagnostic')
    $definition = Read-PublicBlock (Join-Path $root 'guides/WINDOWS.en.md') 'CANDIDATE PREFLIGHT'
    $example = Read-PublicBlock $readmePath 'VERIFIED ACCEPTANCE'
    $env:JAEKIT_ACCEPTANCE_SHELL51 = (Get-Command powershell.exe -CommandType Application -ErrorAction Stop).Source
    $env:JAEKIT_ACCEPTANCE_SHELL7 = (Get-Command pwsh.exe -CommandType Application -ErrorAction Stop).Source
    $wrapper = $ast.ParamBlock.Extent.Text + "`n" + $functions + @'

$shell51 = $env:JAEKIT_ACCEPTANCE_SHELL51
$shell7 = $env:JAEKIT_ACCEPTANCE_SHELL7
$badZip = Join-Path ([IO.Directory]::GetParent($PSScriptRoot).FullName) 'bad-checksum.zip'
$stage = 'binding'
try {
    if ($env:JAEKIT_ACCEPTANCE_CASE -eq 'missing-update-script') { $PSBoundParameters['UpdateInstaller'] = Join-Path ([IO.Path]::GetDirectoryName($UpdateArchive)) 'absent.ps1' }
    if ($env:JAEKIT_ACCEPTANCE_CASE -eq 'wrong-update-script') { $PSBoundParameters['UpdateInstaller'] = Join-Path $PSScriptRoot 'install.ps1' }
    if ($env:JAEKIT_ACCEPTANCE_CASE -eq 'shell-start-failure') { $shell51 = Join-Path $PSScriptRoot 'missing-shell.exe' }
__BINDING__
    foreach ($stage in @(__STAGES__)) {
        $env:JAEKIT_ACCEPTANCE_STAGE = $stage
        Invoke-JaekitAcceptanceStep $steps[$stage] | Out-Null
    }
} catch {
    [IO.File]::WriteAllText($env:JAEKIT_ACCEPTANCE_FAILURE, (ConvertTo-Json -Compress -InputObject @{stage=$stage; message=$_.ToString()}))
    throw
}
'@
    $wrapper = $wrapper.Replace('__BINDING__', $binding).Replace('__STAGES__', (($stageNames | ForEach-Object { "'" + $_ + "'" }) -join ','))
    foreach ($case in @('valid') + @($report.required | Where-Object { -not $_.StartsWith('dispatch-') })) {
        $caseRoot = Join-Path $work $case
        $first = Join-Path $caseRoot 'first'; $next = Join-Path $caseRoot 'next'
        Write-Fixture $first 'first' $wrapper
        Write-Fixture $next 'update' '# Inert update acceptance is verified, not executed.'
        [IO.File]::WriteAllText((Join-Path $caseRoot 'bad-checksum.zip'), 'deliberate checksum fixture', $utf8)
        $env:JAEKIT_ACCEPTANCE_CASE = $case
        $env:JAEKIT_ACCEPTANCE_EVENTS = Join-Path $caseRoot 'events.jsonl'
        $env:JAEKIT_ACCEPTANCE_FAILURE = Join-Path $caseRoot 'failure.json'
        $caught = $null
        Push-Location -LiteralPath $first
        try {
            . ([scriptblock]::Create($definition))
            & ([scriptblock]::Create($example)) | Out-Null
        } catch { $caught = $_.ToString() }
        finally { Pop-Location }
        $events = @()
        if ([IO.File]::Exists($env:JAEKIT_ACCEPTANCE_EVENTS)) {
            $events = @([IO.File]::ReadAllLines($env:JAEKIT_ACCEPTANCE_EVENTS) | ForEach-Object { $_ | ConvertFrom-Json })
        }
        if ($case -eq 'valid') {
            Require ($null -eq $caught) ('Valid public acceptance dispatch failed: ' + $caught)
            Require ($events.Count -eq 2 * $stageNames.Count) 'Missing or extra child/helper execution.'
            foreach ($stage in $stageNames) {
                $observed = @($events | Where-Object { $_.stage -ceq $stage })
                $origin = if ($stage.StartsWith('update-')) { 'update' } else { 'first' }
                $action = if ($stage -in @('update-remove', 'first-remove', 'junction-remove-refusal')) { 'uninstall' } else { 'install' }
                $shellPrefix = if ($stage -in @('first-install', 'checksum-refusal', 'update-reinstall', 'update-remove', 'junction-install-refusal')) { '5.1.' } else { '7.' }
                Require ($observed.Count -eq 2 -and $observed[0].entrypoint -ceq 'helper' -and $observed[1].entrypoint -ceq $action) ('Wrong actual helper/entrypoint: ' + $stage)
                Require ($observed[0].origin -ceq $origin -and $observed[1].origin -ceq $origin) ('Wrong candidate actually executed: ' + $stage)
                Require ($observed[0].shell.StartsWith($shellPrefix) -and $observed[1].shell.StartsWith($shellPrefix)) ('Wrong actual child shell: ' + $stage)
                if ($action -eq 'install') {
                    $directory = if ($origin -eq 'first') { $first } else { $next }
                    $archiveName = if ($origin -eq 'first') { 'ha_0.1.4-dev_windows_amd64.zip' } else { 'ha_0.1.5-dev_windows_amd64.zip' }
                    $expectedArchive = if ($stage -eq 'checksum-refusal') { Join-Path $caseRoot 'bad-checksum.zip' } else { Join-Path $directory $archiveName }
                    Require ($observed[1].archive -ceq $expectedArchive -and $observed[1].checksums -ceq (Join-Path $directory 'checksums.txt') -and $observed[1].path_consent -eq $true) ('Mixed candidate inputs or lost PATH consent: ' + $stage)
                }
                $report.stages += @{stage=$stage; candidate=$origin; child_shell=$observed[1].shell; installer_or_uninstaller=$action; helper=$observed[0].origin}
                $report.passed += 'dispatch-' + $stage
            }
        } else {
            Require ($null -ne $caught -and [IO.File]::Exists($env:JAEKIT_ACCEPTANCE_FAILURE)) ('Failure did not propagate from production dispatch: ' + $case)
            $failure = [IO.File]::ReadAllText($env:JAEKIT_ACCEPTANCE_FAILURE) | ConvertFrom-Json
            $expectedStage = switch ($case) {
                'update-installer-failure' { 'update-unmanaged-refusal' }
                'update-helper-failure' { 'update-unmanaged-refusal' }
                'update-uninstaller-failure' { 'update-remove' }
                'missing-update-script' { 'binding' }
                'wrong-update-script' { 'binding' }
                'shell-start-failure' { 'first-install' }
                'unexpected-refusal-diagnostic' { 'checksum-refusal' }
            }
            $expectedDiagnostic = switch ($case) {
                'update-installer-failure' { 'INJECTED_UPDATE_INSTALLER_FAILURE' }
                'update-helper-failure' { 'INJECTED_UPDATE_HELPER_FAILURE' }
                'update-uninstaller-failure' { 'INJECTED_UPDATE_UNINSTALLER_FAILURE' }
                'missing-update-script' { 'Missing candidate input: update/install' }
                'wrong-update-script' { 'Candidate input points to a different directory: update/install' }
                'shell-start-failure' { 'shell could not start:' }
                'unexpected-refusal-diagnostic' { 'UNRELATED_SETUP_FAILURE' }
            }
            Require ($failure.stage -ceq $expectedStage -and $failure.message.Contains($expectedDiagnostic)) ('Wrong failure stage or diagnostic: ' + $case + ': ' + $failure.message)
            $failedEntrypoint = switch ($case) {
                'update-installer-failure' { 'install' }
                'update-helper-failure' { 'helper' }
                'update-uninstaller-failure' { 'uninstall' }
                default { '' }
            }
            $marker = @($events | Where-Object { $_.stage -ceq $expectedStage -and $_.origin -ceq 'update' -and $_.entrypoint -ceq $failedEntrypoint }).Count -gt 0
            if ($case.StartsWith('update-')) { Require $marker ('Failed update code did not execute: ' + $case) }
            if ($expectedStage -eq 'binding' -or $case -eq 'shell-start-failure') { Require ($events.Count -eq 0) ('Candidate code ran after setup refusal: ' + $case) }
            elseif ($events.Count -gt 0) { Require ($events[-1].stage -ceq $expectedStage) ('Execution continued past the failed stage: ' + $case) }
            $report.failure_observations += @{case=$case; stage=$failure.stage; marker_observed=[bool]$marker; rejected=$true}
            $report.passed += $case
        }
    }
    Require ($report.passed.Count -eq $report.required.Count) 'Required dispatcher observations are incomplete.'
    $report.status = 'passed'
} catch { $report.failures += $_.ToString() }
finally {
    foreach ($name in $oldEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $oldEnvironment[$name], 'Process') }
    if ($null -ne $environmentBefore) {
        try {
            $after = Snapshot-Environment
            $report.environment_after_sha256 = Text-Hash $after
            $report.environment_preserved = $environmentBefore -ceq $after
            if (-not $report.environment_preserved) { $report.status = 'failed'; $report.failures += 'Real user installation/PATH/policy state changed.' }
        } catch { $report.status = 'failed'; $report.failures += $_.ToString() }
    }
    if ($canWrite) { [IO.File]::WriteAllText($output, (ConvertTo-Json -Depth 12 -InputObject $report) + "`n", $utf8) }
}
if ($report.status -ne 'passed') { throw ($report.failures -join '; ') }
