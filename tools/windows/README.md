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
candidate directory. A PE header check establishes file format, not runtime
correctness. Review public inputs before distributing a candidate.

```text
python3 tools/windows/package.py --binary /absolute/build/ha.exe --version 0.1.4-dev --output /absolute/candidates/windows-0.1.4-dev
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
No unchecked bootstrap script, policy bypass or new product prerequisite is used.

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
    & $first.Acceptance -Archive $first.Archive -Checksums $first.Checksums -UpdateArchive $next.Archive -UpdateChecksums $next.Checksums -ConsentUserInstallation
}
```
<!-- END WINDOWS VERIFIED ACCEPTANCE -->

This entrypoint exercises both shells, cross-shell updates, repeated install,
checksum failure, an unmanaged file conflict, a locked executable, junction
refusal, prior-version retention, removal and exact PATH ownership even when a
user adds an equivalent environment-variable alias later. It refuses
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
It then builds and executes the current-source Windows candidate and checks its
manifest, checksums, Unicode/spaced drive paths, unsafe archive entries and
junction refusal using that shell's actual .NET/filesystem behavior.

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
