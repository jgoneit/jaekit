# Windows candidate tools

These tools prepare a local Windows 11 x64 candidate and one per-user PowerShell
installation path. They do not publish a release or change the current public
release's supported platforms. The target shells are Windows PowerShell 5.1 and
PowerShell 7. Actual Windows observations are separate from package tests and
cross-compilation.

`package.py` accepts an already-built Windows amd64 PE32+ `ha.exe`, a version that
matches its runtime version, and a new output directory. It copies only the
selected public files, includes every payload file in a SHA-256 manifest, and
writes a deterministic ZIP, detached checksums and the installer scripts. It
refuses source links, unsafe Windows paths, oversized inputs and an existing
candidate directory. Package paths reserve `CONIN$` and `CONOUT$`, including
case variants and extension forms, in every path component. This conservative
package policy is separate from Core's OS-specific evidence-path rules.
A PE header check establishes file format, not runtime
correctness. Review public inputs before distributing a candidate.

```text
python3 tools/windows/package.py --binary /absolute/build/ha.exe --version 0.1.7-dev --output /absolute/candidates/windows-0.1.7-dev
python3 -m unittest discover -s tools/windows -p 'test_*.py'
```

The package tests use synthetic PE bytes and public files. They check packaging,
hashes, reproducibility, conflict preservation and source boundaries. They do not
execute `ha.exe`, PowerShell or Windows filesystem operations. A successful local
test is never a Windows acceptance result.

Before invoking any candidate script, prepare the independent
`Get-JaekitVerifiedCandidate` function from the trusted [Windows guide](../../guides/WINDOWS.en.md).
Its install, reinstall/update and removal blocks check the ZIP and all four loose
scripts before executing any candidate code, including `common.ps1`. Keep the
complete six-file candidate together. Checksums establish equality to the trusted
list, not authentication if the list and files come from a compromised source.
These candidate tools use no unchecked bootstrap script, policy bypass or new product
prerequisite. The public release blocks described below are a separate path: they run
only checked installer code from a checked release archive, with a process-scoped Bypass.

`install.ps1` accepts local `-Archive` and `-Checksums` files. It verifies the
archive before extraction, checks every manifest file, executes the staged
binary's version and capabilities, and audits any existing managed installation.
It stages replacement in the same filesystem, preserves earlier version data,
and restores the original directory if promotion or PATH update fails. Its
inventory is intentionally strict: unknown or edited files in the managed
installation stop replacement and removal. Keep project copies outside that
directory. A same-user process deliberately changing paths concurrently is not
isolated by this installer.

`-AddToPath` is explicit permission to add one current-user PATH entry. Existing
equivalent entries are not claimed or removed. Current-process PATH is refreshed
and a Windows environment-change notification is sent, but already-running
parent applications may still require restarting. System PATH, execution policy,
Git configuration and host installations are never changed.

`uninstall.ps1` audits the same installation and removes only its managed tree
and one owned PATH entry. Reparse points, edited or unrecognized files and an
unreplaceable executable cause refusal. If final backup cleanup fails, the
operation reports the preserved backup location instead of traversing links or
deleting unrecognized data.

Inventory and cleanup inspect named alternate data streams (ADS) on the tree
root, subdirectories and files. Any named data stream, including an empty stream
or `Zone.Identifier`, causes refusal and preservation; inspection errors never
mean absence. Default unnamed data and filesystem structural metadata are not
additional user data. A filesystem confirmed not to support named streams keeps
its existing behavior. No stream is automatically deleted, unblocked or migrated.
This also prevents a failed candidate inventory audit from being followed by
cleanup that deletes the rejected stream. Cleanup after a committed operation
reports the retained path separately from the installed or removed state.
Read-only original archives, checksums and loose candidate scripts are not
newly rejected merely because they carry download metadata.

On a disposable Windows user account without an existing Jaekit installation or
Jaekit user PATH entry,
prepare that same trusted guide function, then review `acceptance.ps1` and use
two different-version candidates. Place the first candidate in the current
directory and the update candidate in the sibling `next` directory (adjust that
path if needed). Both complete candidates are verified before the acceptance
script or its helper starts. Both PowerShell executables must already be present.
The explicit consent flag permits temporary installation at the real user path
and adding/removing its PATH entry. Run the entire block; verification failure
ends it before candidate code executes:

<!-- BEGIN WINDOWS VERIFIED ACCEPTANCE -->
```powershell
& {
    $first = Get-JaekitVerifiedCandidate (Get-Location).Path
    $next = Get-JaekitVerifiedCandidate '..\next'
    & $first.Acceptance -Archive $first.Archive -Checksums $first.Checksums -UpdateArchive $next.Archive -UpdateChecksums $next.Checksums -UpdateInstaller $next.Install -UpdateUninstaller $next.Uninstall -ConsentUserInstallation
}
```
<!-- END WINDOWS VERIFIED ACCEPTANCE -->

The required update script paths come from the same verified update candidate.
The first candidate supplies initial installation and its later removal; update
failures, update/reinstall and removal after update execute the update candidate's
installer/uninstaller and their own helper. Missing or mismatched paths stop the
procedure; they never fall back to the first candidate's scripts. Expected refusal
cases require their product diagnostic as well as exit 1, so shell startup or
helper failures cannot masquerade as a successful safety check.

This entrypoint exercises both shells, cross-shell updates, repeated install,
checksum failure, an unmanaged file conflict, a locked executable, junction
refusal, prior-version retention, removal and exact PATH order when a user adds an intervening item, an identical
PATH entry and an equivalent environment-variable alias later. Only the earlier
installer-owned literal is removed; the later entries retain their order. It refuses
an existing installation. On failure it preserves diagnostics and avoids forcing
cleanup or restoring a stale PATH snapshot. It does not test Core goal execution,
agent Skill loading, every filesystem race, or public download availability.

For developer fixtures, use a clean Windows source checkout with Git, Python 3
and the Go version in `go.mod`. Run this command separately in Windows PowerShell
5.1 and PowerShell 7, choosing a new output directory outside the checkout for
each run:

```powershell
.\tools\windows\native_checks.ps1 -OutputDirectory "$env:TEMP\jaekit-native-ps51"
```

This developer entrypoint supports actual Windows Server x64 as well as Windows
11 Client x64. It reads the native CPU, actual Windows installation type, shell
version, effective Go target and source commit; it does not bypass or replace the
product installer guard. Product installation still requires Windows 11 Client
x64. The developer helpers are not included in the candidate payload.

Each invocation executes the selected native Core tests, including both ordinary
and linked Git worktrees, and retains the raw Go JSON stream. Missing, skipped,
failed or unfinished selected tests cannot produce a passing report. The
unconfirmed-shutdown and admission-failure fixtures include deliberate injected
failures; the ordinary descendant timeout fixture uses actual child processes.
Command-level process scenarios also connect actual cleanup and timeout to CLI
exit, records, stable output and status for current and baseline checks. A
confirmed timeout retains a stable digest. A test-only execution boundary injects
unconfirmed output finalization separately; this observes command error recording,
not a real Windows API failure. Product timeout defaults and public options stay
unchanged, as do legacy rule error semantics.
It then builds and executes the current-source Windows candidate and checks its
manifest, checksums, Unicode/spaced drive paths, unsafe archive entries and
junction refusal using that shell's actual .NET/filesystem behavior.

The trusted developer runner `native_acceptance_checks.ps1` extracts the current
production acceptance planner and launcher through PowerShell AST, checks that
the real orchestration invokes them for every stage, and drives them from the
exact public acceptance example using inert child candidates. It observes each
candidate/helper identity and shell, plus valid-checksum updater installer, helper
and uninstaller failures. Missing inputs, a failed shell and unrelated refusal
diagnostics cannot pass. It preserves real user installation, PATH and policy
state; it is developer evidence, not a Windows 11 user installation lifecycle.
There is no product Server bypass option.

The trusted developer runner `native_console_checks.ps1` exercises the product
name, manifest, inventory, copy and ZIP boundaries, plus extracted install/remove
orchestration with temporary environment adapters. Rejection happens before a
console device or archive entry data stream is opened. Fixtures preserve existing
candidate and installation data; they do not migrate incompatible inventory or
change real user PATH, policy or installation. Source hashes and the actual shell
bind the observations to the current product code.

The developer stream checks use actual NTFS data streams in both PowerShell
versions. They connect product inventory and lifecycle refusal to the preserved
stream names and bytes, including file, root and subdirectory cases, and cover
temporary/backup cleanup and normal operations. Inspection failures or unsupported
filesystem results introduced at a test boundary are labeled as injected, not
reported as actual disk or Windows API failures. Required missing, skipped or
failed cases cannot pass. These isolated observations preserve real user state
and do not establish a Windows 11 ordinary-user installation lifecycle.

The trusted developer runner `native_preflight_checks.ps1` reads the exact
preflight and action blocks from both public Windows guides and this acceptance
example. In each shell it verifies the actual built candidate without executing
its code, then uses inert synthetic candidates to observe install/remove/acceptance
entrypoint binding and rejection before the execution canary. ZIP and all four
script mismatches, missing files or entries, malformed/duplicate/unsafe checksum
entries and unexpected files must stop the complete action block and preserve
fixture contents. Both acceptance inputs are tested. These developer helpers
are source tools, not an unchecked candidate bootstrap or Windows 11 lifecycle
acceptance. Their fixtures never install or change user PATH or policy.

On actual Server, the candidate installer and uninstaller must both reject the
platform with their original Windows 11 diagnostic. The probe refuses an existing
Jaekit installation and compares installation/scratch presence, raw user PATH
value and kind, machine/process PATH and execution policies before and after each
refusal. Only hashes and preservation booleans enter the summary. On Client,
these two Server probes are explicitly inapplicable and no install is attempted.

`summary.json`, `go-test.jsonl` and per-command logs remain in the output directory.
Failures also print bounded tails of the relevant public fixture logs to the job
output; raw PATH snapshots are never printed or saved.
The `JAEKIT_CI_REPORT` line identifies the source, environment, actual required
scenario results, package checks and scope limits. Use the summary and raw Go
stream for CI artifacts; `candidate/` and `package-runtime/` contain disposable
synthetic test data, not release assets. A successful Server developer result is
not Windows 11 installation lifecycle, WSL2, host Skill, or public release
acceptance. Run `acceptance.ps1` separately on the supported Client environment
for its explicitly authorized installation lifecycle.

User instructions: [한국어](../../guides/WINDOWS.md),
[English](../../guides/WINDOWS.en.md).

## Public release path

The public install and removal blocks in the [Windows guide](../../guides/WINDOWS.en.md#install-from-a-public-release)
download only the release's `ha_<version>_windows_amd64.zip` and `checksums.txt`.
They require exactly one matching checksum line and SHA-256 before extracting
only `tools/windows/install.ps1` (or `uninstall.ps1`) and `common.ps1` from that
archive. The extracted code runs in a new process of the same PowerShell
executable with `-NoProfile -ExecutionPolicy Bypass -File`; the persistent
CurrentUser and LocalMachine policies are never changed, and a Group Policy
execution policy still takes precedence. A nonzero child exit ends the block as a
failure and leaves the window's PATH unchanged. The installer itself, its Windows
11 Client guard and its managed state are the same as for candidates.

The release workflow (`.github/workflows/release.yml`) builds this archive with
`tools/windows/package.py` from a `-buildvcs=false` Windows build, adds it to
the release's single `checksums.txt`, and checks it with
`tools/release/check_archives.py` before upload. Pull requests and manual runs
build and check the same files without publishing. Its Windows job runs
`public_blocks.py ci` in Windows PowerShell 5.1 and PowerShell 7 on Windows
Server: it checks the exact built archive with the installer's own package
verification and Core identity, then pastes the guide's blocks (rebinding only
the download source and version) under a persistent Restricted CurrentUser
policy. With the real archive the installer must stop at its Windows 11 guard;
probe archives, whose installer code only records how it started, show the
success path, failures and checksum refusals. Server results are not Windows 11
installation results.

## Windows 11 check of a public release candidate

A Windows archive is published only when the release notes of its tag
(`tools/release/notes/v<version>.md`) link a Windows 11 check of exactly that
archive. The check needs a Windows 11 Client x64 user account without an
existing Jaekit installation or Jaekit user PATH entry, both PowerShell
editions, Python 3 and a checkout of this repository.

1. Set the release version in both guides' public blocks, then run the Release
   workflow manually (`workflow_dispatch`) with that version on the commit to
   be released. Nothing is published. Download the run's `release-files`
   artifact into a new directory.
2. On Windows 11, run:

   ```text
   python tools/windows/public_blocks.py serve --dist <release-files>
   ```

   It serves that archive and its `checksums.txt` on 127.0.0.1 and prints the
   install and removal blocks from the guides inside the archive, with only the
   download source changed. It refuses an archive whose guides install another
   version, so a stale guide is found here rather than when the tag is pushed.
3. Windows PowerShell 5.1 and PowerShell 7 keep separate CurrentUser
   policies, and a new account usually shows `Undefined` there. In each
   edition, note the value from `Get-ExecutionPolicy -List`, run
   `Set-ExecutionPolicy -Scope CurrentUser Restricted`, and restore the noted
   value after the checks. Paste the install block into Windows PowerShell 5.1
   and check `ha --version` in the same window and the user PATH entry. Paste
   it into PowerShell 7, which reinstalls over the existing installation, and
   check again. Paste the removal block and check that the installation and its
   PATH entry are gone. Once a public release with Windows files exists, also
   install that release with its own published block first, to check an
   update from it.
4. When every check passes, add the line `serve` printed to
   `tools/release/notes/v<version>.md`:

   ```text
   Windows-Accepted-SHA256: <sha256 of the archive>
   ```

   Commit only that change and tag the commit. The publication job reads the
   notes from the tagged checkout, and `tools/release/windows_gate.py` keeps
   the Windows archive only when they carry exactly one such line equal to the
   archive the Windows job checked; otherwise it publishes the four
   macOS/Linux archives with their checksums. It then publishes nothing at all
   if the kept archive's guides install another version. The notes are
   published as the release description, so the line is public. The Windows
   build omits VCS stamps and the notes are not packaged, so the tagged
   commit's archive equals the checked one while no packaged file changes; if
   they still differ, the archive is left out rather than published unchecked.

The digest line is the maintainer's statement that these checks passed.
Review a change that adds or edits it as a release attestation: any commit the
tag points to can carry it.
