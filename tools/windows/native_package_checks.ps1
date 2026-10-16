# Actual PowerShell/.NET package and path checks against a current-source PE.
# Server runs exercise refusal only; this script never impersonates Client.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$CandidateDirectory,
    [Parameter(Mandatory = $true)][string]$OutputPath,
    [Parameter(Mandatory = $true)][string]$NameCharacterCodes
)
. (Join-Path $PSScriptRoot 'common.ps1')
$report = [ordered]@{
    schema = 'jaekit-native-windows-package/v1'
    status = 'failed'
    shell_version = $PSVersionTable.PSVersion.ToString()
    package_checks = @()
    name_character_refusals = @()
    server_rejection = @{}
    candidate = @{}
    failures = @()
}
$utf8 = New-Object Text.UTF8Encoding($false)
$canWriteReport = $false

function Require([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Require-Refusal([scriptblock]$Action, [string]$Expected) {
    $refused = $false
    try { & $Action | Out-Null }
    catch {
        if ($_.ToString().Contains($Expected)) { $refused = $true }
        else { throw }
    }
    Require $refused ('Expected refusal was not observed: ' + $Expected)
}

function Snapshot-UserState {
    $local = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    $root = Join-Path $local 'Jaekit'
    $artifacts = @([IO.Directory]::GetFileSystemEntries($local, '.jaekit-*') | Sort-Object)
    $path = Read-UserPath
    # Compare raw PATH values in memory. Artifacts contain only a digest and
    # booleans, never the user's PATH or another installation's file contents.
    return (ConvertTo-Json -Compress -Depth 6 -InputObject ([ordered]@{
        install_exists = [IO.Directory]::Exists($root) -or [IO.File]::Exists($root)
        install_artifacts = $artifacts
        user_path = $path.value; user_path_exists = $path.exists; user_path_kind = [int]$path.kind
        machine_path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
        process_path = $env:Path
        execution_policy = @(Get-ExecutionPolicy -List | ForEach-Object { [ordered]@{scope = [string]$_.Scope; policy = [string]$_.ExecutionPolicy} })
    }))
}

function String-Digest([string]$Value) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Check-ServerRefusal([string]$Action, [string]$Script, [string[]]$Arguments) {
    $before = Snapshot-UserState
    $identity = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    Require ($identity.InstallationType -in @('Server', 'Server Core')) 'Server refusal must execute on actual Server.'
    $hostExecutable = (Get-Process -Id $PID).Path
    $oldPreference = $ErrorActionPreference
    try {
        # Windows PowerShell emits NativeCommandError records for native stderr.
        # Preserve them as diagnostics without mistaking an expected nonzero
        # child exit for a failed attempt to start the child.
        $ErrorActionPreference = 'Continue'
        $PSNativeCommandUseErrorActionPreference = $false
        $messages = @(& $hostExecutable -NoProfile -NonInteractive -File $Script @Arguments 2>&1)
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $oldPreference }
    $text = ($messages | ForEach-Object { $_.ToString() }) -join "`n"
    [IO.File]::WriteAllText((Join-Path $work ($Action + '-refusal.log')), $text + "`n", $utf8)
    $after = Snapshot-UserState
    $rejected = $code -ne 0 -and $text.Contains('Windows 11 client is required.')
    $preserved = $before -ceq $after
    $report.server_rejection[$Action] = [ordered]@{
        rejected = [bool]$rejected; preserved = [bool]$preserved; exit_code = $code
        before_sha256 = String-Digest $before; after_sha256 = String-Digest $after
        observed = @('installation-absent', 'installer-scratch-entries', 'user-path-value-and-kind', 'machine-path', 'process-path', 'execution-policy')
    }
    Require $rejected ($Action + ' did not report the unsupported Server platform.')
    Require $preserved ($Action + ' changed installation/PATH/policy state while refusing Server.')
}

try {
    $candidate = Assert-OrdinaryPath $CandidateDirectory
    $output = Assert-OrdinaryPath $OutputPath
    Require (-not [IO.File]::Exists($output)) 'Refuse to replace an existing package report.'
    $canWriteReport = $true
    $work = Join-Path ([IO.Path]::GetDirectoryName($output)) 'package-runtime'
    Require (-not (Test-Path -LiteralPath $work)) 'Runtime scratch directory already exists.'
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $archives = @(Get-ChildItem -LiteralPath $candidate -Filter '*.zip' -File)
    Require ($archives.Count -eq 1) 'Expected one actual candidate archive.'
    $archive = $archives[0].FullName
    $checksums = Join-Path $candidate 'checksums.txt'
    $destination = Join-Path $work ('space ' + [char]0xd55c + [char]0xae00)
    Require ($destination -match '^[A-Za-z]:\\') 'Native candidate path must be on a Windows drive.'
    $package = Expand-CheckedPackage $archive $checksums $destination
    $report.package_checks += 'candidate-manifest-sha256'
    $report.package_checks += 'space-unicode-drive-path'
    $exe = Join-Path $destination 'ha.exe'
    $version = @(& $exe --version)
    Require ($LASTEXITCODE -eq 0 -and ($version -join "`n").Trim() -ceq ('ha ' + $package.manifest.version)) 'Current candidate version did not execute or match.'
    $capsOutput = @(& $exe capabilities --format json)
    Require ($LASTEXITCODE -eq 0) 'Current candidate capabilities did not execute.'
    $caps = ($capsOutput -join "`n") | ConvertFrom-Json
    Require ($caps.schema -ceq 'ha-capabilities/v1' -and $caps.ha_version -ceq $package.manifest.version -and @($caps.supported_rules) -contains 'run-rules/3') 'Current candidate capabilities differ.'
    $report.candidate = @{
        manifest_sha256 = Get-Sha256 (Join-Path $destination 'package.json')
        executed_binary_sha256 = Get-Sha256 $exe
        capabilities = $caps
    }
    $report.package_checks += 'candidate-native-version-capabilities'

    $badSums = Join-Path $work 'wrong-checksums.txt'
    [IO.File]::WriteAllText($badSums, ('0' * 64) + '  ' + [IO.Path]::GetFileName($archive) + "`n", $utf8)
    $badDestination = Join-Path $work 'bad-checksum-output'
    Require-Refusal { Expand-CheckedPackage $archive $badSums $badDestination } 'Archive checksum'
    Require (-not (Test-Path -LiteralPath $badDestination)) 'Checksum refusal wrote an extraction directory.'
    $report.package_checks += 'checksum-refusal'

    # Each malformed archive starts with the actual built candidate, then adds
    # one unsafe entry and receives a valid checksum for that modified archive.
    $mutations = @(
        @{id = 'archive-traversal-refusal'; name = '../outside.txt'; reason = 'Unsafe package path'},
        @{id = 'archive-case-collision-refusal'; name = 'license'; reason = 'Unsafe or duplicate archive entry'},
        @{id = 'archive-file-directory-collision-refusal'; name = 'LICENSE/child.txt'; reason = 'Archive file/directory collision'},
        @{id = 'archive-reserved-name-refusal'; name = 'assets/NUL.txt'; reason = 'Unsafe package path'},
        @{id = 'archive-stream-name-refusal'; name = 'assets/item:stream'; reason = 'Unsafe package path'}
    )
    foreach ($mutation in $mutations) {
        $directory = Join-Path $work $mutation.id
        [IO.Directory]::CreateDirectory($directory) | Out-Null
        $modified = Join-Path $directory ([IO.Path]::GetFileName($archive))
        [IO.File]::Copy($archive, $modified, $false)
        $zip = [IO.Compression.ZipFile]::Open($modified, [IO.Compression.ZipArchiveMode]::Update)
        try {
            $entry = $zip.CreateEntry($mutation.name)
            $stream = $entry.Open()
            try { $stream.WriteByte(120) } finally { $stream.Dispose() }
        } finally { $zip.Dispose() }
        $sums = Join-Path $directory 'checksums.txt'
        [IO.File]::WriteAllText($sums, (Get-Sha256 $modified) + '  ' + [IO.Path]::GetFileName($modified) + "`n", $utf8)
        $rejectedDestination = Join-Path $directory 'must-not-exist'
        Require-Refusal { Expand-CheckedPackage $modified $sums $rejectedDestination } $mutation.reason
        Require (-not (Test-Path -LiteralPath $rejectedDestination)) 'Unsafe archive wrote an extraction directory.'
        $report.package_checks += $mutation.id
    }

    # Every character of the installer's name rule, in an entry name of the
    # actual candidate. .NET Framework will not create such an entry, so it is
    # written under a placeholder of the same length and the name bytes are
    # patched in the local header and the central directory record.
    $codes = @($NameCharacterCodes.Split(',') | ForEach-Object { [int]$_ })
    Require ($codes.Count -gt 0) 'No name characters were given to check.'
    $placeholder = 'assets/name-probe-@.txt'
    $marker = $placeholder.IndexOf('@')
    $latin1 = [Text.Encoding]::GetEncoding(28591)
    foreach ($code in $codes) {
        Require ($code -ge 0 -and $code -le 127) ('Name character is outside ASCII: ' + $code)
        $label = 'name character 0x' + $code.ToString('x2')
        $directory = Join-Path $work ('name-character-' + $code.ToString('x2'))
        [IO.Directory]::CreateDirectory($directory) | Out-Null
        $modified = Join-Path $directory ([IO.Path]::GetFileName($archive))
        [IO.File]::Copy($archive, $modified, $false)
        $zip = [IO.Compression.ZipFile]::Open($modified, [IO.Compression.ZipArchiveMode]::Update)
        try {
            $entry = $zip.CreateEntry($placeholder)
            $stream = $entry.Open()
            try { $stream.WriteByte(120) } finally { $stream.Dispose() }
        } finally { $zip.Dispose() }
        $bytes = [IO.File]::ReadAllBytes($modified)
        $text = $latin1.GetString($bytes)
        $offsets = @()
        $at = $text.IndexOf($placeholder, [StringComparison]::Ordinal)
        while ($at -ge 0) {
            $offsets += $at
            $at = $text.IndexOf($placeholder, $at + 1, [StringComparison]::Ordinal)
        }
        Require ($offsets.Count -eq 2) ($label + ': the placeholder must appear once in a local header and once in the central directory.')
        foreach ($offset in $offsets) { $bytes[$offset + $marker] = [byte]$code }
        [IO.File]::WriteAllBytes($modified, $bytes)
        $sums = Join-Path $directory 'checksums.txt'
        [IO.File]::WriteAllText($sums, (Get-Sha256 $modified) + '  ' + [IO.Path]::GetFileName($modified) + "`n", $utf8)
        $rejectedDestination = Join-Path $directory 'must-not-exist'
        $before = @(Get-ChildItem -LiteralPath $directory -Recurse -Force | ForEach-Object { $_.FullName }) -join "`n"
        try {
            Require-Refusal { Expand-CheckedPackage $modified $sums $rejectedDestination } 'Unsafe package path'
        } catch {
            throw ($label + ': ' + $_.ToString())
        }
        Require (-not (Test-Path -LiteralPath $rejectedDestination)) ($label + ': the refusal wrote an extraction directory.')
        $after = @(Get-ChildItem -LiteralPath $directory -Recurse -Force | ForEach-Object { $_.FullName }) -join "`n"
        Require ($after -ceq $before) ($label + ': the refusal left files behind.')
        $report.name_character_refusals += $code
    }
    $report.package_checks += 'archive-name-character-refusal'
    $outside = Join-Path $work 'outside target'
    [IO.Directory]::CreateDirectory($outside) | Out-Null
    $sentinel = Join-Path $outside 'sentinel.txt'
    [IO.File]::WriteAllText($sentinel, 'unchanged', $utf8)
    $junction = Join-Path $work 'junction'
    try {
        New-Item -ItemType Junction -Path $junction -Value $outside | Out-Null
        Require-Refusal { Assert-OrdinaryPath (Join-Path $junction 'sentinel.txt') } 'Link or reparse-point path'
        Require ([IO.File]::ReadAllText($sentinel) -ceq 'unchanged') 'Junction refusal changed the external sentinel.'
    } finally {
        if ([IO.Directory]::Exists($junction)) { [IO.Directory]::Delete($junction) }
    }
    $report.package_checks += 'junction-path-refusal'

    $windows = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    if ($windows.InstallationType -in @('Server', 'Server Core')) {
        $root = Get-InstallRoot
        Require (-not (Test-Path -LiteralPath $root)) 'Refuse to run Server refusal probes with an existing Jaekit installation.'
        Check-ServerRefusal 'install' (Join-Path $candidate 'install.ps1') @('-Archive', $archive, '-Checksums', $checksums, '-AddToPath')
        Check-ServerRefusal 'uninstall' (Join-Path $candidate 'uninstall.ps1') @()
    } else {
        # Do not invoke the installer on Client: developer checks never grant
        # consent for a real per-user installation or count lifecycle success.
        $report.server_rejection = @{status = 'not-applicable'; reason = 'actual Windows Client; no Server refusal claim'}
    }
    $report.status = 'passed'
} catch {
    $report.failures += $_.ToString()
} finally {
    if ($canWriteReport) { [IO.File]::WriteAllText($OutputPath, (ConvertTo-Json -Depth 12 -InputObject $report) + "`n", $utf8) }
}
if ($report.status -ne 'passed') { throw ($report.failures -join '; ') }
