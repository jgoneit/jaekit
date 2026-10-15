# Native Windows candidate installation

[한국어](WINDOWS.md) · **English**

This guide covers two paths targeting **Windows 11 x64 with Windows PowerShell 5.1 and PowerShell 7**: the [public release path](#install-from-a-public-release), which installs from a public release with Windows files, and the local candidate installation path after it. It does not mean the current public release, `v0.1.4`, includes Windows files. Candidate packaging and cross-compilation do not establish Windows installation, execution, or actual agent Skill loading. Read the observations and remaining gaps for the particular candidate.

Windows ARM64/x86, earlier Windows versions, network/UNC repositories and using the same checkout from both Windows and WSL are outside this path's support scope. WSL and native Windows support are separate.

## Install from a public release

This path installs `ha` from public release files with one PowerShell block, without Go, Python, Git or a Jaekit checkout. Paste the whole block, unchanged, into Windows PowerShell 5.1 or PowerShell 7 on Windows 11 x64. Use a 64-bit window, and prepare Git and your agent separately.

The block proceeds only in this order:

- It downloads only two files from the GitHub Release of the version written in the block: `ha_<version>_windows_amd64.zip` and `checksums.txt`.
- It continues only when `checksums.txt` lists that ZIP exactly once and the SHA-256 matches. A failed download, a missing or repeated entry or a different value stops it before any code from the ZIP runs.
- From the checked ZIP it extracts only the installer code (`tools/windows/install.ps1` and `common.ps1`). It runs that code in a new process of the same PowerShell executable as this window (Windows PowerShell for 5.1, PowerShell 7 for 7) with `-NoProfile -ExecutionPolicy Bypass -File`. The installer checks the ZIP and checksums again and, with `-AddToPath`, adds one installation entry to the current user's PATH.
- If the installer fails, the block also ends as a failure without a success message and leaves this window's PATH unchanged. On success it appends the installation folder to this window's PATH only when it is missing, so `ha` works here at once. It warns when another `ha` comes first. Other terminals and apps that are already open read the new PATH only after they restart.
- It removes the downloaded temporary files at the end and reports their location if it cannot.

**Execution policy.** Windows 11 defaults to the Restricted execution policy, yet a pasted block runs because it is a command, not a script file. The block does not change the persistent execution policy (CurrentUser, LocalMachine) and does not ask you to change it. Bypass applies only to the new process that runs the checked installer or uninstaller code, and to its children. An execution policy set by Group Policy (MachinePolicy, UserPolicy) takes precedence over this option. If your organization's policy blocks scripts, the block installs nothing and ends as a failure. The [candidate path](#prerequisites) below does not use this option.

**Checksums.** Checksums establish that the downloaded ZIP matches the same release's list. They are not signatures that authenticate the source. Read this guide and the block from a trusted source.

No public release with Windows files exists yet, and the current release, `v0.1.4`, has no Windows files. The block below works once the first release with Windows files is published. Its version value (planned `0.1.5`) is confirmed when that release is prepared. Until then, use the candidate path below.

<!-- BEGIN WINDOWS PUBLIC INSTALL -->
```powershell
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $version = '0.1.5'
    $source = 'https://github.com/jgoneit/jaekit/releases/download/v' + $version
    $name = 'ha_' + $version + '_windows_amd64.zip'
    $shell = (Get-Process -Id $PID).Path
    if (@('powershell.exe', 'pwsh.exe') -notcontains [IO.Path]::GetFileName($shell).ToLowerInvariant()) {
        throw 'Paste this block into Windows PowerShell 5.1 or PowerShell 7.'
    }
    $work = Join-Path ([IO.Path]::GetTempPath()) ('jaekit-' + [Guid]::NewGuid().ToString('N'))
    [void][IO.Directory]::CreateDirectory($work)
    try {
        $archive = Join-Path $work $name
        $checksums = Join-Path $work 'checksums.txt'
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/' + $name) -OutFile $archive
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/checksums.txt') -OutFile $checksums
        $listed = @([IO.File]::ReadAllLines($checksums) | Where-Object { $_.EndsWith('  ' + $name) })
        if ($listed.Count -ne 1 -or $listed[0] -cnotmatch '^[0-9a-f]{64}  ') {
            throw ('checksums.txt must list ' + $name + ' exactly once.')
        }
        if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $listed[0].Substring(0, 64)) {
            throw ('SHA-256 mismatch: ' + $name)
        }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($archive)
        try {
            foreach ($script in 'common.ps1', 'install.ps1') {
                $entries = @($zip.Entries | Where-Object { $_.FullName -ceq ('tools/windows/' + $script) })
                if ($entries.Count -ne 1 -or $entries[0].Length -gt 1048576) {
                    throw ('The archive must contain one tools/windows/' + $script)
                }
                [IO.Compression.ZipFileExtensions]::ExtractToFile($entries[0], (Join-Path $work $script))
            }
        } finally { $zip.Dispose() }
        Write-Output ('Running the verified installer in ' + $shell)
        $ErrorActionPreference = 'Continue'
        & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $work 'install.ps1') -Archive $archive -Checksums $checksums -AddToPath
        $code = $LASTEXITCODE
        $ErrorActionPreference = 'Stop'
        if ($code -ne 0) { throw ('The installer stopped with exit code ' + $code + '; see its message above.') }
        $bin = Join-Path (Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Jaekit') 'bin'
        $exe = Join-Path $bin 'ha.exe'
        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw ('The installer exited 0, but ' + $exe + ' is missing.') }
        if (@($env:Path -split ';' | Where-Object { $_.TrimEnd('\') -ieq $bin }).Count -eq 0) {
            if ($env:Path -and -not $env:Path.EndsWith(';')) { $env:Path += ';' }
            $env:Path += $bin
        }
        $found = @(Get-Command ha -ErrorAction SilentlyContinue)
        if ($found.Count -eq 0 -or $found[0].CommandType -ne 'Application' -or $found[0].Source -ine $exe) {
            Write-Warning ('Another ha comes first in this window. Run ' + $exe + ' directly or reorder PATH.')
        }
        Write-Output ('ha ' + $version + ' is ready in this window. Restart other open terminals and apps, or sign out and in, so they load the new PATH.')
    } finally {
        try { [IO.Directory]::Delete($work, $true) } catch { Write-Warning ('Remove the temporary files yourself: ' + $work) }
    }
}
```
<!-- END WINDOWS PUBLIC INSTALL -->

### Update from a public release

Paste the installation block from the newer version's guide again, unchanged. The installer replaces the installation at the same user location with the new version and keeps earlier version data. The same version is checked again and reinstalled. An installation made with PowerShell 5.1 can be updated from PowerShell 7 with the same managed state. If an update fails, the installer restores the earlier installation and the block ends as a failure.

### Remove a public release installation

Close `ha` and running checks, then paste the block below. It checks the downloaded files the same way as the installation block and then runs only the checked ZIP's removal code (`tools/windows/uninstall.ps1` and `common.ps1`). It never runs the installed copy without checking it. This block also works once the first public release with Windows files is published.

<!-- BEGIN WINDOWS PUBLIC REMOVE -->
```powershell
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $version = '0.1.5'
    $source = 'https://github.com/jgoneit/jaekit/releases/download/v' + $version
    $name = 'ha_' + $version + '_windows_amd64.zip'
    $shell = (Get-Process -Id $PID).Path
    if (@('powershell.exe', 'pwsh.exe') -notcontains [IO.Path]::GetFileName($shell).ToLowerInvariant()) {
        throw 'Paste this block into Windows PowerShell 5.1 or PowerShell 7.'
    }
    $work = Join-Path ([IO.Path]::GetTempPath()) ('jaekit-' + [Guid]::NewGuid().ToString('N'))
    [void][IO.Directory]::CreateDirectory($work)
    try {
        $archive = Join-Path $work $name
        $checksums = Join-Path $work 'checksums.txt'
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/' + $name) -OutFile $archive
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/checksums.txt') -OutFile $checksums
        $listed = @([IO.File]::ReadAllLines($checksums) | Where-Object { $_.EndsWith('  ' + $name) })
        if ($listed.Count -ne 1 -or $listed[0] -cnotmatch '^[0-9a-f]{64}  ') {
            throw ('checksums.txt must list ' + $name + ' exactly once.')
        }
        if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $listed[0].Substring(0, 64)) {
            throw ('SHA-256 mismatch: ' + $name)
        }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($archive)
        try {
            foreach ($script in 'common.ps1', 'uninstall.ps1') {
                $entries = @($zip.Entries | Where-Object { $_.FullName -ceq ('tools/windows/' + $script) })
                if ($entries.Count -ne 1 -or $entries[0].Length -gt 1048576) {
                    throw ('The archive must contain one tools/windows/' + $script)
                }
                [IO.Compression.ZipFileExtensions]::ExtractToFile($entries[0], (Join-Path $work $script))
            }
        } finally { $zip.Dispose() }
        Write-Output ('Running the verified uninstaller in ' + $shell)
        $ErrorActionPreference = 'Continue'
        & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $work 'uninstall.ps1')
        $code = $LASTEXITCODE
        $ErrorActionPreference = 'Stop'
        if ($code -ne 0) { throw ('The uninstaller stopped with exit code ' + $code + '; see its message above.') }
        Write-Output 'Removal finished. Terminals and apps that were already open keep their old PATH until they restart.'
    } finally {
        try { [IO.Directory]::Delete($work, $true) } catch { Write-Warning ('Remove the temporary files yourself: ' + $work) }
    }
}
```
<!-- END WINDOWS PUBLIC REMOVE -->

The removal code checks the managed inventory and removes only the installation and the one PATH entry the installer added. Your project's goal documents, records and logs remain. Windows and apps that are already open keep their old PATH until they restart.

### Evidence scope of the public path

CI and release smoke on Windows Server 2025 execute this block text in PowerShell 5.1 and 7. They observe the download, checksum check, child process in the same shell, failure propagation and unchanged PATH and policy. The installer accepts only Windows 11 Client, so a Server result is not a Windows 11 installation success. Actual installation, update and removal are checked by the [Windows 11 acceptance procedure](../tools/windows/README.md). A public release includes only a Windows ZIP linked to that acceptance; without the link it publishes the macOS and Linux files only.

## Prerequisites

- Open 64-bit Windows PowerShell 5.1 or PowerShell 7. Prepare Git and your agent separately. The installer does not install Git, agents, PowerShell or Windows features, and does not request elevation.
- Obtain `ha_<version>_windows_amd64.zip`, `checksums.txt`, `install.ps1`, `uninstall.ps1`, `common.ps1` and `acceptance.ps1` from the same reviewed candidate. Use a dedicated directory containing only these six files. The installer reads local files; it does not download or publish anything. A failed download is not a started installation.
- Checksums establish that files match the candidate contents. They are not signatures that authenticate a compromised checksum source. Review the scripts' source and contents as code before running them.
- If your PC or organization's execution policy blocks scripts, follow that policy. This candidate path's installer does not change security settings or relaunch itself with policy-bypass options. Only the public release path uses a process-scoped Bypass, and only for checked installer and uninstaller code ([execution policy](#install-from-a-public-release)).

Ordinary installation does not require Go, Homebrew, Bash, zsh, WSL or a Jaekit source checkout. Dependencies of a selected project check remain that check's dependencies. Python is needed only if you choose to use the reference producer that requires it.

## Install and identify

Review this guide and `checksums.txt` from a trusted source, then paste the function definition below into PowerShell. Do not run or dot-source any candidate script or helper first. This code uses only built-in PowerShell facilities to check the ZIP and all four loose scripts, independently of candidate code. Prepare this definition again in each new PowerShell session.

<!-- BEGIN WINDOWS CANDIDATE PREFLIGHT -->
```powershell
function Get-JaekitVerifiedCandidate([string]$Directory) {
    $ErrorActionPreference = 'Stop'
    $root = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Directory)
    $root = [IO.Path]::GetFullPath($root)
    if ($root -notmatch '^[A-Za-z]:\\' -or -not [IO.Directory]::Exists($root)) {
        throw 'Candidate must be an existing local drive directory.'
    }
    $ancestor = $root
    while ($ancestor) {
        if (([IO.File]::GetAttributes($ancestor) -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Candidate path must not contain links or reparse points.'
        }
        $parent = [IO.Directory]::GetParent($ancestor)
        if ($null -eq $parent) { break }
        $ancestor = $parent.FullName
    }
    $names = @()
    foreach ($path in [IO.Directory]::GetFileSystemEntries($root)) {
        $attributes = [IO.File]::GetAttributes($path)
        if (($attributes -band ([IO.FileAttributes]::Directory -bor [IO.FileAttributes]::ReparsePoint)) -ne 0) {
            throw 'Candidate entries must be ordinary files.'
        }
        $names += [IO.Path]::GetFileName($path)
    }
    $archives = @($names | Where-Object { $_ -cmatch '^ha_[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)?_windows_amd64\.zip$' })
    if ($archives.Count -ne 1) { throw 'Expected exactly one Windows candidate ZIP.' }
    $expected = @($archives[0], 'common.ps1', 'install.ps1', 'uninstall.ps1', 'acceptance.ps1')
    if ($names.Count -ne 6 -or $names -cnotcontains 'checksums.txt') {
        throw 'Candidate file inventory is incomplete or unexpected.'
    }
    foreach ($name in $names) {
        if ($name -cne 'checksums.txt' -and $expected -cnotcontains $name) {
            throw 'Candidate file inventory is incomplete or unexpected.'
        }
    }
    $checksums = Join-Path $root 'checksums.txt'
    if ((Get-Item -LiteralPath $checksums).Length -gt 1048576) { throw 'Checksum file is too large.' }
    $lines = [IO.File]::ReadAllLines($checksums, [Text.Encoding]::ASCII)
    $seen = @{}
    foreach ($line in $lines) {
        if ($line -cnotmatch '^([a-fA-F0-9]{64})  (.+)$') { throw 'Malformed SHA-256 checksum entry.' }
        $digest = $Matches[1]
        $name = $Matches[2]
        if ($expected -cnotcontains $name) { throw 'Unexpected or unsafe checksum path.' }
        if ($seen.ContainsKey($name)) { throw 'Duplicate checksum entry.' }
        $seen[$name] = $digest
    }
    if ($seen.Count -ne $expected.Count) { throw 'Missing checksum entry.' }
    foreach ($name in $expected) {
        $path = Join-Path $root $name
        if (-not [IO.File]::Exists($path)) { throw ('Missing candidate file: ' + $name) }
        $limit = if ($name -ceq $archives[0]) { 134217728 } else { 33554432 }
        if ((Get-Item -LiteralPath $path).Length -gt $limit) { throw ('Candidate file is too large: ' + $name) }
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ine $seen[$name]) {
            throw ('SHA-256 mismatch: ' + $name)
        }
    }
    return [pscustomobject]@{
        Directory = $root; Archive = (Join-Path $root $archives[0]); Checksums = $checksums
        Install = (Join-Path $root 'install.ps1'); Uninstall = (Join-Path $root 'uninstall.ps1')
        Acceptance = (Join-Path $root 'acceptance.ps1')
    }
}
```
<!-- END WINDOWS CANDIDATE PREFLIGHT -->

Change to the candidate directory and run the entire block below. If any ZIP or script check fails, the block stops before invoking installation code. Missing or duplicate checksum entries, malformed digests, paths outside the candidate and unexpected files are rejected. This procedure does not isolate another same-user process that deliberately changes files after verification.

<!-- BEGIN WINDOWS VERIFIED INSTALL -->
```powershell
& {
    $candidate = Get-JaekitVerifiedCandidate (Get-Location).Path
    & $candidate.Install -Archive $candidate.Archive -Checksums $candidate.Checksums -AddToPath
}
```
<!-- END WINDOWS VERIFIED INSTALL -->

`-AddToPath` explicitly permits adding **one current-user PATH entry**. Omit it to leave PATH unchanged and invoke the absolute executable path. An equivalent pre-existing entry is neither duplicated nor claimed by the installer. System PATH and global Git settings do not change.

The fixed installation directory is `Jaekit` under the current user's Windows LocalApplicationData directory, normally `%LOCALAPPDATA%\Jaekit`. Locate it with:

```powershell
$jaekitRoot = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Jaekit'
$jaekitExe = Join-Path $jaekitRoot 'bin\ha.exe'
& $jaekitExe --version
& $jaekitExe capabilities --format json
Get-Command ha -All
```

Before replacement the installer executes the candidate's version and capability queries. The displayed version must match the candidate. If `Get-Command` selects a different `ha` first, distinguish that path from the new installation and choose the intended executable; do not delete other installations blindly. Seal must use the actual executable whose capabilities were checked.

The installer refreshes PATH in the current PowerShell process and sends a Windows environment-change notification. Already-running parent applications may retain the old environment. Close the terminal's or agent's parent app and reopen it; sign out and in, or use the absolute path, if the old environment persists. PATH and version checks establish neither goal completion nor host Skill loading.

Keep goal documents and checks in a local Windows Git project. The plugin installation and activation contract in the [existing host guide](INSTALL.en.md) still applies, but observations from Linux/macOS do not establish Windows host use. This candidate installer does not install or redesign plugins.

## Optional decision history

The current-source Windows x64 local candidate provides [decision history](../contracts/decision-history.md): add, replay, correction, default/current, list and show by event ID or request ID. Verify that the selected executable declares `decision-history/v1` in both `features` and `formats.decision_history`. Public release v0.1.4 does not contain this extension; a version number alone does not establish support.

Ordinary and linked worktrees on local drives share the Git common directory's history and lock, including paths with spaces and Unicode. Junctions and reparse points in storage paths are refused as `history_unreadable`. File references passing the existing relative-path grammar that Windows interprets as devices, named pipes, alternate data streams or drive-relative paths are not opened: they remain `unreadable` observations without a version, without rejecting an otherwise valid event. Existing data and valid macOS/Linux names are not rewritten or automatically repaired.

[CI](CI.en.md) checks actual history and existing Core boundaries on Windows Server in both PowerShell 5.1 and 7. Those results do not establish Windows 11 installation, agent Skill use or a public Windows release. This guide's existing platform and local-repository scope still applies.

## Public files and check commands

Each version's guides, contracts and reference example are in `versions\<version>`. Locate the current data and copy the example to a **new, absent directory** in your project:

```powershell
$jaekitState = Get-Content -LiteralPath (Join-Path $jaekitRoot 'managed.json') -Raw | ConvertFrom-Json
$jaekitData = Join-Path (Join-Path $jaekitRoot 'versions') $jaekitState.current_version
$jaekitExample = Join-Path (Get-Location).Path 'jaekit-reference-example'
if (Test-Path -LiteralPath $jaekitExample) { throw 'The copy destination already exists.' }
Copy-Item -LiteralPath (Join-Path $jaekitData 'examples\check-result') -Destination $jaekitExample -Recurse
```

The producer is in that version's `tools\check-result-reference.py`. Follow the example README and adjust declaration/producer locations to your project layout. Do not edit the managed installation or store project files inside it. Modified or additional files cause update and removal to stop rather than discard your data.

Even when you invoke `ha` from PowerShell, PLAN check commands retain the existing argv format. Pipes, redirection and PowerShell expressions are not interpreted automatically. Quote absolute Windows paths to preserve backslashes, or use `/` separators. These are synthetic command-format examples, not executed checks:

```text
'C:\Program Files\Example\checker.exe' 'C:\Work folder\input.txt'
C:/Tools/checker.exe 'C:/Work folder/input.txt'
```

To run a shell script, explicitly name its shell and script arguments. The chosen shell and its execution policy belong to the project; `ha` does not prepare or change them.

Git may consider a tree clean while CRLF conversion makes declaration or producer bytes differ from HEAD. The existing byte-identity requirement still applies. Inspect the check files, project `.gitattributes` and checkout conditions, then use the project's approved policy and check again. Neither the installer nor Core automatically changes global `core.autocrlf`, file bytes, old digests or records.

## Reinstall, update and failures

Rerun the entire installation block to verify and reinstall identical contents of the same version. Different contents under the same version are rejected as a conflict. To update, change to the directory containing only the new candidate's six files and run the entire installation block. Do not reuse an earlier candidate's verification result. PowerShell 5.1 and 7 share the same per-user directory and management state, including when one shell updates an installation made by the other.

The installer validates checksums and the full file inventory, stages a new tree, then replaces the existing installation. Earlier version data is not automatically deleted. Integrity failures, links/junctions, unknown or edited files, a running executable that cannot be replaced, and file-access failures are not reported as success. Stop users of `ha` or resolve the conflict before retrying. Existing installation and project contents are preserved; if promotion fails, the previous installation is restored.

Package paths reject `CONIN$` and `CONOUT$` components regardless of case or filename extension. If an existing installation inventory contains one, reinstall, update and removal stop while preserving its files. This check does not automatically delete or rename incompatible data.

Reinstall, update and removal stop before changing an installation whose root, subdirectories or files have named alternate data streams (ADS). Even when the default file content is unchanged, empty streams and metadata such as `Zone.Identifier` are additional data to preserve. A failed stream inspection is not treated as absence. The default unnamed data and filesystem structural metadata are not additional user streams; existing local filesystems confirmed not to support named streams remain usable.

This policy covers managed, replacement and deletion trees, including extracted candidates. It does not newly reject or delete download metadata on read-only source ZIPs, checksums or loose candidate scripts. Inspect the target and stream identified by the diagnostic. The installer does not automatically delete, unblock or migrate streams. Temporary or previous-backup cleanup also preserves the tree and reports its location when a stream is present or inspection fails. If installation or removal already committed before cleanup failed, distinguish the applied operation from the remaining data.

If rollback itself or final temporary-data cleanup fails, the remaining backup location is reported. Inspect and compare preserved data before cleanup. Do not forcibly delete folders containing unrelated files or follow links to delete their targets. The installer does not isolate another same-user process deliberately changing paths concurrently.

## Remove

Stop `ha` and its checks. Prepare the function definition above and run the entire block below from a candidate directory retaining the trusted checksums and all six files. Removal code and `common.ps1` are verified again before execution. Do not bypass this check by directly invoking an installed copy.

<!-- BEGIN WINDOWS VERIFIED REMOVE -->
```powershell
& {
    $candidate = Get-JaekitVerifiedCandidate (Get-Location).Path
    & $candidate.Uninstall
}
```
<!-- END WINDOWS VERIFIED REMOVE -->

Removal audits the complete inventory, then removes only the current user's managed tree and one installer-owned PATH entry. If the user appends the exact same PATH string later, removal deletes the earlier installer entry and preserves the later user entries, spelling and order. A user-added copy of that path may therefore remain. This does not reconstruct ownership after arbitrary user reordering. Managed data for older versions is removed with that tree. Pre-existing PATH entries, unrelated settings, other users' installations and WSL installations remain. Checks/examples copied into projects, goals, `runs.jsonl` and raw output in Git directories are not installation-managed files and remain intact.

If removal stops on modified or unknown files, links or a running executable, inspect the cause first. There is no forced-deletion option.

## Evidence scope

The [candidate tools guide](../tools/windows/README.md) describes package creation and a separate real-Windows installer lifecycle entrypoint. Its `acceptance.ps1` block first verifies all six files in both the first and update candidates, and requires explicit consent for the real user installation. Each stage uses its candidate's installer or uninstaller and helper. Update, update-candidate reinstall and post-update removal execute the new candidate's code and propagate its failures instead of using the first candidate's code. Successful preflight in Server developer CI is not Windows 11 installation acceptance. Distinguish local package regressions, Windows cross-compilation, actual PowerShell 5.1/7 installation/update/removal, Core goal execution, host Skill use and installation from a public URL. Leave unobserved items unverified.
