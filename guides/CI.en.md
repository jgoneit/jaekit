# Reading CI results

[한국어](CI.md) · **English**

The [CI workflow](../.github/workflows/ci.yml) runs on pull requests and pushes to `main`. It keeps common checks on Ubuntu and runs selected native checks on Ubuntu, macOS and Windows Server. Read the results for the specific commit and run; this guide describes the checks, not a permanent claim that every environment has passed.

## Required results

| Check | Environment and scope |
| --- | --- |
| `Core / Ubuntu` | Ubuntu 24.04 x64: the existing common Go/regression suite and macOS/Linux cross-builds, plus actual native Core and isolated direct-install lifecycle checks |
| `Core / macOS` | `macos-15`, arm64: selected native Core fixtures and the actual native candidate's isolated direct-install lifecycle |
| `Core / Windows` | Windows Server 2025 x64: selected native Core, candidate/package/path checks and unsupported-product-installation refusal, through both Windows PowerShell 5.1 and PowerShell 7 |
| `Seal Core` | Requires all three Core jobs in the same run to finish with `success` |
| `Public documentation` | Ubuntu: public file/document checks and their offline regressions, performed once |

Main continues to require the exact `Seal Core` and `Public documentation` checks from GitHub Actions. The Core gate runs even after a prerequisite fails and rejects failures, missing results, skipped jobs, cancellation and unknown states. Waiting or a successful older run does not satisfy the current run. Native workers do not use `continue-on-error` or path filters to turn omitted work into a successful check.

Ubuntu retains the common suite, including the four macOS/Linux build targets and the Windows candidate cross-build. Cross-build success only establishes compilation. The additional native jobs run current-source code on their actual host; they do not repeat the entire common suite or all documentation checks.

## Source and execution evidence

Open the job logs for the current PR revision. Each native job prints one `JAEKIT_CI_CONTEXT` JSON line containing its checkout SHA, event, PR head/base SHAs, actual OS/CPU, runner metadata and shell version. GitHub can check out a PR integration commit, so its SHA may differ from the PR head. Keep that relationship when comparing a result with local source.

Each native checker prints `JAEKIT_CI_REPORT` with its actual observations and source commit. The Unix checker also writes the requested report file; the Windows checker keeps reports and fixture data in its selected temporary output directory. Required native test names must have actual passing results. A skipped or absent fixture is not replaced by a count of selected tests, an empty helper run, or a configuration check. A report belongs to that job's source and environment, and does not establish results on another architecture or shell.

The workflow uses isolated runner workspaces and temporary paths. Actions are pinned to commits, Go follows `go.mod`, and the workflow token has `contents: read`. No additional secrets, private goals, local execution records or AI service are CI inputs. Private development files must still stay out of commits and published logs.

## Installation and support boundaries

The Unix lifecycle builds the current source into real native candidates, then exercises the documented direct-install route in temporary user directories. It checks reinstall/update/removal and the preservation of unrelated project data. Local transport used to supply candidate files is identified separately from public release download availability.

Windows Server runs developer fixtures and checks that product install/remove commands safely refuse the unsupported environment. It does not change the installer's **Windows 11 Client x64** requirement. GitHub's Windows runners have administrator privileges, so these results do not establish ordinary-user installation behavior on Windows 11. See the [candidate guide](WINDOWS.en.md) for that separate acceptance scope.

The Windows job disables newline conversion only in its disposable checkout's local Git configuration and restores the committed bytes. It does not change global Git settings or user projects. Native command scenarios observe unsafe declaration/producer-path refusal and record preservation. Both PowerShell versions execute the Korean and English guides' preflight code against a valid candidate and altered archives/scripts. The real candidate is verified without execution; installation, removal and acceptance entrypoint connections use isolated inert synthetic code. These observations remain separate from Windows 11 installation lifecycle acceptance.

An Ubuntu run is not a WSL2 run. CI does not establish WSL2 installation, installed agent Skill loading, a real agent session or session resumption. Those observations remain separate. No CI success publishes a release, changes a version, or proves installation from a public release URL.

## Local checks

Before committing or publishing, run the repository's common verification entrypoint:

```bash
python3 tools/verify.py
```

This checks the current development host and shared regressions. It does not run other operating systems. The Unix native lifecycle is a separate source-development check, requiring a clean, committed checkout and the existing Git, Python, Go and shell prerequisites. Go must target the current native host without effective `GOFLAGS` overrides:

```bash
python3 tools/ci/native_unix.py --report /tmp/jaekit-native-report.json
```

Use a fresh report destination outside the checkout. It uses temporary installation data and does not replace a real user installation. On an appropriate native Windows developer host, use `tools/windows/native_checks.ps1` from a clean checkout with a fresh `-OutputDirectory` outside it, separately in PowerShell 5.1 and 7. Both checkers reject changed source or a changed commit during execution. They diagnose missing tools; they do not install local prerequisites or change global Git, shell, PATH or execution-policy settings.

Runner labels describe selected images; inspect the recorded image and actual versions for each run. GitHub's [runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) explains the available architectures and runner privileges.
