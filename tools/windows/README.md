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
review and run `acceptance.ps1` with two different-version candidates. Both
PowerShell executables must already be present. The explicit consent flag permits
temporary installation at the real user path and adding/removing its PATH entry:

```powershell
.\acceptance.ps1 -Archive .\ha_0.1.4-dev_windows_amd64.zip -Checksums .\checksums.txt -UpdateArchive ..\next\ha_0.1.5-dev_windows_amd64.zip -UpdateChecksums ..\next\checksums.txt -ConsentUserInstallation
```

This entrypoint exercises both shells, cross-shell updates, repeated install,
checksum failure, an unmanaged file conflict, a locked executable, junction
refusal, prior-version retention, removal and exact PATH ownership even when a
user adds an equivalent environment-variable alias later. It refuses
an existing installation. On failure it preserves diagnostics and avoids forcing
cleanup or restoring a stale PATH snapshot. It does not test Core goal execution,
agent Skill loading, every filesystem race, or public download availability.

For developer fixtures from a Windows source checkout with Git and the Go version
in `go.mod`, run `tools\windows\native_checks.ps1` in PowerShell. This is a separate
source-development path, not an installed-user prerequisite or a substitute for
`acceptance.ps1`, installed Core goal execution, or host Skill observations. The
developer entrypoint is not included in the candidate payload.

User instructions: [한국어](../../guides/WINDOWS.md),
[English](../../guides/WINDOWS.en.md).
