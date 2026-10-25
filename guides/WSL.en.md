# WSL2 preparation guide

[한국어](WSL.md) · **English**

This route installs Jaekit inside **Ubuntu 24.04 LTS amd64(x64) on WSL2**. **Installation, execution, and agent Skill use in actual WSL2 have not been checked yet.** This document describes preparation of the route, not completed WSL2 support validation.

The current public release is `v0.1.8`. The linked installation commands download that release's Linux files. This guide and the [environment check tool](#optional-environment-check-for-candidate-packages) are in the public data of the v0.1.8 files; files of v0.1.4 and earlier do not have them. Checking a candidate package, publishing a release, and installing files from its public URL are separate steps.

## Working environment

| Item | Scope of this guide |
| --- | --- |
| WSL | WSL2 with Ubuntu 24.04 LTS already installed |
| CPU | amd64(x64), reported as `x86_64` in Linux |
| Terminal | The Linux terminal of that Ubuntu distribution |
| Tools | Linux versions of the agent, Git, and `ha` in the same distribution |
| Project | A Git repository under the Linux home directory in that distribution |
| Temporary files and records | Temporary storage in the same Linux environment and the project's Git directory |

WSL1, other distributions or CPUs, projects on Windows drives such as `/mnt/c`, using one checkout from both Windows and WSL, and moving goals between distributions are outside this route. The existing amd64 and arm64 installation routes for ordinary Linux remain available.

If WSL2 is missing, first follow Microsoft's [WSL installation guide](https://learn.microsoft.com/en-us/windows/wsl/install). Jaekit does not automatically install WSL features, a distribution, or an agent, or request administrator privileges. From Windows PowerShell, the following query shows whether the distribution you will use has `VERSION` set to `2`. It does not install, convert, or remove a distribution.

```powershell
wsl --list --verbose
```

Continue in **Ubuntu's Linux terminal**. Inspect the following values there.

```bash
uname -sr
uname -m
cat /etc/os-release
pwd -P
```

Ubuntu must report version `24.04`, and the CPU must report `x86_64`. Keep the project under the Linux home directory. A name under home that links to a Windows drive is outside this route. If you configure a separate location for Git's temporary worktrees or `TMPDIR`, keep it on the same Linux filesystem. Microsoft also recommends [using the Linux filesystem for Linux tools](https://learn.microsoft.com/en-us/windows/wsl/filesystems).

## Install the Linux release

1. Prepare Linux Git and the agent CLI you will use inside that Ubuntu distribution. The current public installation guide covers Codex CLI and Claude Code. An app or `.exe` installed in Windows does not substitute for a WSL installation. Prepare authentication and execution permissions in the Linux host you will use.
2. In the same Linux terminal, run the block under [Install from a Release file](INSTALL.en.md#install-from-a-release-file). Besides Git, the guide uses `curl`, `tar`, a checksum tool, and standard Linux utilities. Prepare missing tools first. You do not need a Jaekit source checkout, Go, or Homebrew.
3. This combination uses `ha_0.1.8_linux_amd64.tar.gz` and `checksums.txt`. The executables go to `~/.local/bin/ha` and `~/.local/bin/spar`, and public data goes to `~/.local/share/jaekit/0.1.8`. Copy needed files into your project instead of modifying the installed public data.
4. Follow the installation section for either [Codex](INSTALL.en.md#codex) or [Claude Code](INSTALL.en.md#claude-code). Install plugins through the Linux CLI in the same distribution. This guide does not change other hosts' packaging or invocation names.

If `~/.local/bin` is missing from PATH, you can apply the following to the current Linux shell. It affects only this shell and processes started from it.

```bash
export PATH="$HOME/.local/bin:$PATH"
```

To apply it in new terminals, manually add the same setting to the configuration file for your Linux shell. For Bash, inspect `~/.bashrc` or the appropriate file and preserve its existing contents. You do not need to change Windows PATH, another distribution's settings, global Git settings, or security policy.

Open a new Ubuntu terminal, enter your project, and follow [Check the version](INSTALL.en.md#check-the-version). Confirm that the selected `git` and `ha` paths are Linux installations, then query the actual Core executable's capabilities.

```bash
type -a git ha
ha --version
ha capabilities --format json
```

For a v0.1.8 installation, the version must be `ha 0.1.8`. If several installations exist, PATH selects the first one. If it selects a Windows path, an `.exe`, or an unintended older installation, correct the installation location and PATH in that distribution first. Assess Core capabilities using the [compatibility contract](../contracts/core-capabilities.md); a matching version string is insufficient.

Use the direct-install data path in [Get the reference check](INSTALL.en.md#get-the-reference-check) to copy public examples into your project. Existing files are not overwritten. Python 3 is needed only if you choose this reference producer.

## Optional environment check for candidate packages

The [environment check tool](../tools/wsl/check-environment.sh) is in the public data of the v0.1.8 release files (`~/.local/share/jaekit/0.1.8/tools/wsl/check-environment.sh`). Release files of v0.1.4 and earlier do not have it. It is not required to install the release. Use it only when the public data, reviewed source or a candidate package includes `tools/wsl/check-environment.sh`. Replace both placeholders with the actual public data directory and project path. The final `codex` is the Linux host command you will use; replace it with `claude` or omit it.

```bash
bash "<candidate-public-data>/tools/wsl/check-environment.sh" "<project-path>" codex
```

The tool checks for WSL2, Ubuntu 24.04, and x86_64, then inspects and displays the resolved paths of Git and `ha`. If you supply a host command, it also checks that path. It runs that Linux Git's `rev-parse` to check that the project is inside a Git repository under Linux home, and that the Git directory and common Git directory, where `ha` keeps its records, are also on the Linux filesystem. A linked worktree of a repository on a Windows drive is refused by this condition. It does not install anything or change settings. It fails if it cannot establish this route's conditions; that result does not block an existing ordinary Linux installation.

Path checks do not independently attest execution. You must separately observe whether the host loaded the Skill, a check process actually checked the requirement, and a goal reached completion. A successful environment check does not establish the whole WSL2 workflow.

### Get candidate files

The [Release workflow](../.github/workflows/release.yml) in GitHub Actions builds candidate files. Run it manually (`workflow_dispatch`) on the branch to check and enter a candidate version (`X.Y.Z`). When the run finishes, download its `release-files` artifact into a new directory. Its `ha_<version>_linux_amd64.tar.gz` and `checksums.txt` are the candidate, and the extracted `ha_<version>_linux_amd64` directory is the public data directory for the command above. A pull request's Release workflow run leaves the same artifact with version `0.0.0`.

The workflow checks out the commit it runs on and builds and checks the files with the same steps as a tag build, so local files and uncommitted changes that are not in that commit cannot enter the candidate. Manual and pull request runs publish nothing, and a candidate version is not a public release version. Do not substitute it into the v0.1.8 installation block above. Building a candidate is not evidence of actual WSL2 execution.

## Use it with an agent

Start a new agent session from the same Linux project and follow the [host-specific invocation guide](USAGE.en.md#sending-commands). Confirm that the Skill actually loaded is Jaekit's Spec or Seal and that its source and version match the intended installation. Finding a CLI on PATH does not establish that a Skill was loaded.

Ask Spec for a small change. It writes goal documents and stops. Review the result, then ask Seal to implement it **in a separate message**. Seal checks the actual Core's capabilities and records the requirement violation before the change and satisfaction afterward. Read the [completion evidence](USAGE.en.md#reading-the-completion-report) rather than relying only on the agent's description.

To resume unfinished work, name the same goal in a new agent session in the same distribution and project. For completed work, you can inspect records and raw logs. Rework after completion needs a separate request. See [Saved files](USAGE.en.md#saved-files) for locations and `ha log` usage. These are usage instructions, not a report of an observed WSL2 run.

## Reinstall, update, and remove

You can repeat the [direct installation](INSTALL.en.md#install-from-a-release-file) when the same version's data is unchanged. Conflicting contents for that version or abnormal paths stop installation. Do not treat download, checksum, or file replacement failures as success; inspect the error and remaining installation state. Do not delete existing data to hide a conflict.

For an [update](INSTALL.en.md#update), use the installation method chosen in this distribution. Update `ha` and plugins to a compatible combination, then check again in a new Linux terminal and agent session. Existing goals and run histories keep their rules, usage, and time limits. Previous public data versions are not automatically deleted; the separate cleanup instructions preserve current and newer data and user projects.

For [removal](INSTALL.en.md#uninstall), run only the direct-install and relevant host commands in the same Ubuntu distribution. They target that distribution's Jaekit installation. Checks and examples copied to projects, goal documents, `runs.jsonl`, and raw logs under `ha/` in the Git directory stay. Do not also remove installations in Windows or other distributions. If the public data path is a link, the documented removal deletes the link without following it to delete the target. Removing the WSL distribution itself, projects, or logs is not part of uninstalling Jaekit.

## Troubleshooting

| Situation | What to check |
| --- | --- |
| Linux commands do not work in PowerShell | Return to the Linux terminal of the intended Ubuntu distribution |
| Distribution, CPU, or WSL version is outside this route | Do not label it as a checked combination. Ordinary Linux installation and the WSL2 observation scope are separate |
| PATH lists Windows tools or multiple installations | Confirm that Linux executables in the same distribution are selected, then restart the agent |
| Download, checksum, copy, or replacement fails | Resolve the reported failure before retrying. Do not arbitrarily delete existing executables, data, or project records |
| A Skill is missing | Check the Linux host's plugin list and the source of the Skill actually loaded. See the [usage guide](USAGE.en.md#troubleshooting) |
| Authentication, permissions, or a tool is missing | Prepare it in the intended Linux environment. Do not automatically bypass installation or permission requirements |

## Observation scope

| Evidence | What it establishes |
| --- | --- |
| Document consistency and synthetic environment checks | Consistency of instructions and bounded failure or preservation behavior |
| Package execution on ordinary Linux | The package's result in that Linux environment |
| Package installation and execution in actual WSL2 | Results for the recorded WSL, distribution, CPU, and package combination |
| Actual Linux host Skill use | The observed flow for that host, model, and Skill version |

The last two stages have no actual WSL2 observations yet. Successful source or candidate-package checks do not establish successful WSL2 or host use. The current public release's observation scope is described in the [v0.1.8 release notes](https://github.com/jgoneit/jaekit/releases/tag/v0.1.8). Completion reports still use assurance `local`, which does not mean independent third-party verification.
