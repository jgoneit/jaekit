# Reading CI results

[한국어](CI.md) · **English**

Read this when you send a change to the Jaekit repository or interpret its CI results. You do not need it to use Jaekit. The [CI workflow](../.github/workflows/ci.yml) runs on pull requests and pushes to `main`. It keeps common checks on Ubuntu and runs selected native checks on Ubuntu, macOS and Windows Server. Read the results for the specific commit and run; this guide describes the checks, not a permanent claim that every environment has passed.

## Required results

| Check | Environment and scope |
| --- | --- |
| `Core / Ubuntu` | Ubuntu 24.04 x64: the existing common Go/regression suite and macOS/Linux cross-builds, plus actual native Core and isolated direct-install lifecycle checks |
| `Core / macOS` | `macos-15`, arm64: selected native Core fixtures and the actual native candidate's isolated direct-install lifecycle |
| `Core / Windows PowerShell 5.1` | Windows Server 2025 x64, in Windows PowerShell 5.1: selected native Core, decision history, candidate/package/path checks and unsupported-product-installation refusal |
| `Core / Windows PowerShell 7` | The same Windows Server 2025 x64 checks in PowerShell 7, as a separate job that does not wait for the 5.1 job |
| `Seal Core` | Requires `Core / Ubuntu`, `Core / macOS`, `Core / Windows PowerShell 5.1` and `Core / Windows PowerShell 7` in the same run to finish with `success` |
| `Public documentation` | Ubuntu: public file/document checks and their offline regressions, performed once |
| `Release candidate` | Requires `Build release files` and `Windows release archive` in the same [release workflow](#release-workflow) run to finish with `success` |

Main requires the exact `Seal Core`, `Public documentation` and `Release candidate` checks from GitHub Actions; `Release candidate` comes from the [release workflow](#release-workflow). The Core gate runs even after a prerequisite fails and rejects failures, missing results, skipped jobs, cancellation and unknown states. Waiting or a successful older run does not satisfy the current run. Native workers do not use `continue-on-error` or path filters to turn omitted work into a successful check.

Ubuntu retains the common suite, including the four macOS/Linux build targets and the Windows candidate cross-build. Cross-build success only establishes compilation. The additional native jobs run current-source code on their actual host; they do not repeat the entire common suite or all documentation checks.

## Source and execution evidence

Open the job logs for the current PR revision. Each native job prints one `JAEKIT_CI_CONTEXT` JSON line containing its checkout SHA, event, PR head/base SHAs, actual OS/CPU, runner metadata and shell version. GitHub can check out a PR integration commit, so its SHA may differ from the PR head. Keep that relationship when comparing a result with local source.

Each native checker prints `JAEKIT_CI_REPORT` with its actual observations and source commit. The Unix checker also writes the requested report file; the Windows checker keeps reports and fixture data in its selected temporary output directory. Required native test names must have actual passing results. A skipped or absent fixture is not replaced by a count of selected tests, an empty helper run, or a configuration check. A report belongs to that job's source and environment, and does not establish results on another architecture or shell.

Windows decision-history checks connect the actual CLI, Git, shared history store and file evidence in each PowerShell version. Required names cover ordinary and linked worktrees, add/replay/correction, default/current/list and both show forms, concurrent writes/reads, lock-owner exit, integrity/uncertain writes and file-safety boundaries. An absent history package or required case, skip, failure or interrupted test cannot pass that shell and propagates to that shell's Windows job and the `Seal Core` aggregate. Existing Core tests and cross-build success cannot substitute for these observations.

CI collection delivers both complete machine-readable PowerShell reports, one from each shell's job, to the existing Windows validation consumer. A run recorded before the shells had separate jobs has one `Core / Windows` job with both reports, and is still read that way. Reports lost to a long-line display limit, missing or duplicate reports, truncation and collection errors are not success. The observer ties the current head/base to the tested integration commit and Git tree, run and attempt, and rejects a changed observation basis instead of reusing earlier success.

Windows failure diagnostics select failed tests' output before bounded tails of public synthetic fixture logs. Long passing output must not hide an earlier failure reason; output limits, inaccessible files and malformed diagnostics are identified. A diagnostic collection failure preserves the original failure. Product failures, preparation or collection failures and unobserved results remain distinct, and incomplete diagnostics do not establish a cause. This does not publish full transcripts or private development data.

The workflow uses isolated runner workspaces and temporary paths. Actions are pinned to commits, Go follows `go.mod`, and the workflow token has `contents: read`. No additional secrets, private goals, local execution records or AI service are CI inputs. Private development files must still stay out of commits and published logs.

## Installation and support boundaries

The Unix lifecycle builds the current source into real native candidates, then exercises the documented direct-install route in temporary user directories. It checks reinstall/update/removal and the preservation of unrelated project data. Local transport used to supply candidate files is identified separately from public release download availability.

Windows Server runs developer fixtures and checks that product install/remove commands safely refuse the unsupported environment. It does not change the installer's **Windows 11 Client x64** requirement. GitHub's Windows runners have administrator privileges, so these results do not establish ordinary-user installation behavior on Windows 11. See the [candidate guide](WINDOWS.en.md) for that separate acceptance scope.

Each Windows job disables newline conversion through Git environment options scoped to the initial checkout step, preserving committed bytes. It does not modify Git configuration files or user projects, and later checks and fixtures do not inherit these options. Native command scenarios observe unsafe declaration/producer-path refusal and record preservation. Both PowerShell versions execute the Korean and English guides' preflight code against a valid candidate and altered archives/scripts. The real candidate is verified without execution; installation, removal and acceptance entrypoint connections use isolated inert synthetic code. These observations remain separate from Windows 11 installation lifecycle acceptance.

Developer lifecycle checks connect the current acceptance stage selection and child execution logic to isolated candidate code, distinguishing the first and update candidates. A valid checksum cannot hide a failure in the new candidate's code. PATH checks use the product removal logic and its callers to observe preservation of later identical entries, aliases and unrelated ordering. These fixtures do not change real user installation, PATH or policy and do not replace full Windows 11 installation acceptance.

Windows command-level process scenarios connect child cleanup and timeout to CLI exit, records, stable output and status. A confirmed timeout retains its stable log digest; unconfirmed shutdown is separately injected at the command's execution boundary to check error recording without usable output evidence. That injection is not a reproduced Windows API failure, and it adds no public timeout or fault-injection option.

Both Windows shells also exercise product package-name checks and isolated install/remove orchestration. Console-device names, including the package policy's extension forms, must be refused before extraction writes. Existing incompatible inventory is preserved rather than migrated or deleted. These fixtures do not open console devices or modify real user installation, PATH or policy.

Alternate data stream (ADS) checks observe stream names and bytes on actual NTFS files, installation roots and subdirectories. Each PowerShell version connects product inventory and reinstall/update/removal with temporary/backup cleanup, refusal, preservation and normal acceptance. Inspection failures or unsupported-stream results injected at a test boundary remain distinct from actual filesystem observations. Missing, skipped or failed required cases cannot pass, and real user installation, PATH and policy are preserved. These results do not replace Windows 11 ordinary-user installation acceptance.

Each shell job prepares its own checkout, Go, Python and source identification, so PS7 checks still run if PS5.1 checks fail, and the reverse. Either shell's failure remains a failure of that shell's job and the required aggregate. Missing results after preparation failure, cancellation or timeout are not successes. Both complete shell reports include the package checks following ADS observations.

Diagnostic controls distinguish correct refusals from wrong reasons, targets and exit values. The isolated ADS child's display setting and public acceptance's default display setting are observed separately. Baseline junction scenarios use the temporary worktree created by the actual CLI to observe refusal before execution, record and external-data preservation, and safe ordinary-file execution. They add no product bypass option.

An Ubuntu run is not a WSL2 run. CI does not establish WSL2 installation, installed agent Skill loading, a real agent session or session resumption. Those observations remain separate. No CI success publishes a release, changes a version, or proves installation from a public release URL.

## Release workflow

The [release workflow](../.github/workflows/release.yml) runs the same build and checks for pull requests and manual runs as for a tag push. Only a tag push publishes.

- `Build release files` (Ubuntu) builds the four macOS/Linux tar.gz archives and one Windows ZIP, and writes one `checksums.txt` that lists all five archives. The pre-publication check verifies the ZIP down to its manifest, file hashes, paths and Windows executable format. A pull request builds its head commit.
- `Windows release archive` (Windows Server 2025) checks the same ZIP in PowerShell 5.1 and 7 with the installer's own package verification and runs `ha --version` and `capabilities`. It then pastes the [public install and removal blocks](WINDOWS.en.md#install-from-a-public-release) unchanged except for the download source and version. With the real ZIP it observes the installer stopping at its Windows 11 guard; with probe ZIPs it observes the parent PATH, execution policy and child shell for success, failure and checksum refusal. Reports are kept as `release-windows-reports`.
- `Publish release` runs only for a tag push. If the ZIP the Windows job checked differs from the ZIP to be published, nothing is published. Without a Windows 11 acceptance line for that ZIP in the tag's release notes (`tools/release/notes/vX.Y.Z.md`), it leaves out the Windows ZIP and publishes the macOS and Linux files only. When it keeps the ZIP, it publishes nothing unless the guides' public blocks install that release version. It also stops if the remote tag no longer names the commit the files were built from. After publishing it compares the published asset names with the checked files.

Its `Release candidate` job is one of `main`'s required checks: it runs after `Build release files` and `Windows release archive`, even when one of them fails, and succeeds only when both succeed in the same run. A skipped, cancelled or failed job does not satisfy it, and the workflow has no path filter, so every pull request reports it. A Server result is not a Windows 11 installation success; actual installation, reinstallation and removal are checked by the [Windows 11 check procedure](../tools/windows/README.md).

## Site workflow

The [site workflow](../.github/workflows/pages.yml) builds the [documentation site](https://jgoneit.github.io/jaekit/) from `guides/`, `contracts/` and the release version in `.claude-plugin/marketplace.json`. It runs on every pull request, push to `main` and manual run, and publishes only from `main`; a pull request run builds and tests the site without publishing it.

- `Site build` builds the site. It fails when a link target or the release version cannot be read. On `main` it builds the head of `main` when the run starts, not the commit that started the run, so the last run publishes the latest commit.
- `Site test` tests the built site's documents, links and page behavior. It does not hold back publishing.
- `Site deploy` publishes to GitHub Pages only after `Site build` succeeds on `main`.

These jobs are not required checks, so a failure does not block a merge. When a build or deployment fails, the previously published site stays. The [site source guide](../tools/site/README.md) explains how to build the site locally.

## Local checks

Before pushing or publishing, the repository's common verification entrypoint must pass once on the same verification input as the final candidate. That one pass satisfies both the commit and the push requirement, whether it runs before or after the commit; do not run the full verification twice for the same input:

```bash
python3 tools/verify.py
```

The verification input is the tracked files (content, path, mode and tracking state), the verification command and group, and the tools, settings and environment that affect the result. If any of them changes, an earlier pass is not evidence for the new input; a different commit ID alone is not such a change. Narrower checks that fit the change are fine during development. This reuse only removes duplicate runs from this guidance: it does not establish success with other tools, settings or environments, does not make an ordinary run a Seal record, and does not change freshness, baseline, failure history, independent review or required CI requirements.

Untracked files that Git does not ignore stop verification before any check starts, and their paths are listed. Track product and check files that verification needs with `git add`; for local files that are not published, review `.gitignore` or `.git/info/exclude`. Verification does not add, exclude or delete files. If Git state cannot be read, verification stops instead of assuming a clean tree.

The `go` group runs the full Go test suite once, in this checkout. Its current-file independence check then makes two public-only copies of the tracked files: a two-commit repository of those files and a depth-1 clone of it, both without `docs/` or `.git/ha`. In each copy it runs the TrackedFiles check, the public file and documentation checks, and the current-file judgments of the producers that the `current-file-independence` route names. It does not repeat the full Go suite or the `docs` group there. Whether ordinary Go tests or documentation regressions need history or local goal documents shows in the required CI jobs, which check out one commit without history; a workflow regression keeps those checkouts at that depth.

Each step's pass or failure line shows that step's elapsed time, and the last line shows the total time, in seconds with one decimal. A step runs from its `RUN:` line to its result; the total time runs from the start of verification to its last line. Both use a monotonic clock. A keyboard interrupt reports the interrupted step and both times; a stop before any check reports that no check started and the total time. Times are not used to decide results.

This checks the current development host and shared regressions. It does not run other operating systems. The Unix native lifecycle is a separate source-development check, requiring a clean, committed checkout and the existing Git, Python, Go and shell prerequisites. Go must target the current native host without effective `GOFLAGS` overrides:

```bash
python3 tools/ci/native_unix.py --report /tmp/jaekit-native-report.json
```

Use a fresh report destination outside the checkout. It uses temporary installation data and does not replace a real user installation. On an appropriate native Windows developer host, use `tools/windows/native_checks.ps1` from a clean checkout with a fresh `-OutputDirectory` outside it, separately in PowerShell 5.1 and 7. Both checkers reject changed source or a changed commit during execution. They diagnose missing tools; they do not install local prerequisites or change global Git, shell, PATH or execution-policy settings.

Runner labels describe selected images; inspect the recorded image and actual versions for each run. GitHub's [runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) explains the available architectures and runner privileges.
