# Install guide

[한국어](INSTALL.md) · **English**

Read this to install Jaekit for the first time, update it to a new version, or remove it. Jaekit consists of two terminal programs, `ha` and `spar`, and three agent plugins, Spec, Seal and Spar. The programs install with brew or a Release file and the plugins with the plugin commands of Codex or Claude Code; both are needed. This guide is for the current release, `v0.1.8`. Supported platforms are macOS and Linux on arm64 and amd64. Run the commands below **in Terminal**. After installing, see [your first task](../README.md#your-first-task) for what to send to the agent.

- First installation: [Basic install](#basic-install)
- Without brew: [Install from a Release file](#install-from-a-release-file)
- For Linux tools on Windows: [WSL2 preparation guide — actual use unverified](WSL.en.md)
- To use the reference producer and example: [Get the reference check](#get-the-reference-check)
- Moving to a new version: [Update](#update)
- What each release contains and covers: [Release and development combinations](#release-and-development-combinations)
- Stopping using it: [Uninstall](#uninstall)
- Moving from a registration that points at a repository checkout: [Move from a local path registration](#move-from-a-local-path-registration)
- Working on Jaekit itself: [Building from source and registering for development](DEVELOPMENT.en.md)

## Basic install

You need Git and either Codex or Claude Code. For the brew route, install [Homebrew](https://brew.sh) too. Your project must be a folder whose changes are tracked with Git. You do not need to clone (copy to your computer) the Jaekit repository.

The current public release of Seal Core `ha` does not support Windows. The [Windows native candidate guide](WINDOWS.en.md) covers a separate candidate package. Actual installation, Core execution and host behavior on Windows 11 x64 with PowerShell 5.1/7 remain unverified; cross-compilation and local tests do not establish them.

This describes native Windows execution. A [separate guide](WSL.en.md) prepares the Linux release route for Ubuntu 24.04 LTS x64 in WSL2. Actual WSL2 installation, execution, and agent use have not been checked yet; ordinary Linux results do not establish them.

### Install ha

```bash
brew install jgoneit/tap/jaekit
```

Run one block at a time. Pasting the next commands before brew finishes can cause it to consume those lines. Brew downloads the Release executable and verifies its checksum (a value used to check file contents). Homebrew installs `ha` and `spar` together. It does not install Go or build the source. Without Homebrew, follow [Install from a Release file](#install-from-a-release-file).

### Check the version

```bash
ha --version
which -a ha
spar --version
which -a spar
```

`ha --version` should print `ha 0.1.8`, and `spar --version` the same version, `spar 0.1.8`. The first line from `which -a ha` is the file that actually runs (for `spar`, the first line from `which -a spar`). PATH lists the folders to search for commands, in order. If the version differs, check this path first. If an older file built with `go install` comes first, remove that file or change your PATH order. It lives in the folder printed by `go env GOBIN`, or in `$(go env GOPATH)/bin` when that is empty.

If the command is missing and `which -a ha` is empty, check that installation finished and that its folder is on PATH. Reopen Codex or Claude Code after setting it so the agent can also find `ha`.

### Codex

First [install ha](#install-ha) and [check its version](#check-the-version). The installation commands below require [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), the Terminal tool. Plugins work in Codex CLI and the desktop app; the Codex IDE extension does not support them.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.8
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

`@v0.1.8` pins the marketplace (the source of the plugin list) to that tag. Even when main changes, you get the release's spec 0.1.13, seal 0.1.23 and spar 0.1.4.

Reopen Codex, type `$` in the conversation, and choose Jaekit's `spec:spec` or `seal:seal`. The invocation names are `$spec:spec` and `$seal:seal`. Follow [your first task](../README.md#your-first-task). If you confuse them with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

### Claude Code

First [install ha](#install-ha) and [check its version](#check-the-version). Then install the plugins in Claude Code with the commands below.

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.8
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
claude plugin list
```

`#v0.1.8` pins the marketplace to that tag. Check that `spec@jaekit`, `seal@jaekit` and `spar@jaekit` are enabled in the list. If you confuse them with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

Reopen Claude Code and use `/spec:spec <request>` in the conversation to define the goal, then `/seal:seal docs/specs/<goal>` in a separate message to start implementation. Plugin skills use `/plugin-name:skill-name`. The [usage guide](USAGE.en.md#sending-commands) explains how to review the goal and continue a task.

### Spar

Spar runs independent review rounds with several reviewer CLIs; see the [Spar guide](SPAR.en.md). From v0.1.7, `standard` and `strict` goals need the Spar executable and the Spar plugin. The v0.1.7 Seal starts new goals under `run-rules/5`, and a `standard` or `strict` goal started under `run-rules/5` completes only with a recorded Spar gate. `light` goals, goals without an `Assurance:` line and goals started under `run-rules/4` or earlier do not need Spar. To finish without review, you decide a goal-document change that lowers the tier to `light` ([Review gate](USAGE.en.md#review-gate-run-rules5)).

From v0.1.7, installing with Homebrew or from a Release file installs `ha` and `spar` together, with the same version. Release files, formulas and marketplace tags of v0.1.6 and earlier do not contain Spar.

```bash
spar --version
which -a spar
```

`spar --version` prints the same version as `ha --version`. The [Codex](#codex) and [Claude Code](#claude-code) installation commands above install the Spar plugin too. If you installed only spec and seal, add it from the marketplace pinned to v0.1.8:

```bash
codex plugin add spar@jaekit
claude plugin install spar@jaekit
```

Reopen the host. Before the first round, list only the CLIs you have as reviewers, as [Configure reviewers](SPAR.en.md#configure-reviewers) shows, and check them with `spar config`. Seal runs the review rounds of `standard` and `strict` goals with Spar. When another plan or change needs an independent review, use `$spar:spar` in Codex or `/spar:spar` in Claude Code.

## Install from a Release file

Instead of brew, you can install from the file `ha_0.1.8_<os>_<arch>.tar.gz` in the [v0.1.8 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.8). `<os>` is `darwin` (macOS) or `linux`, and `<arch>` is `arm64` (Apple Silicon and others) or `amd64` (Intel and others).

Paste the commands below as they are. The block picks the file for your OS and CPU, verifies its checksum, and puts `ha` and `spar` at `~/.local/bin/ha` and `~/.local/bin/spar` (`spar` is in the files from v0.1.7). On an unsupported OS or CPU it stops without downloading or installing anything. If the extracted file has no `spar`, it stops with a message without changing anything.

```bash
(
  set -e
  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) echo "This OS is not supported: $(uname -s)" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) echo "This CPU is not supported: $(uname -m)" >&2; exit 1 ;;
  esac
  name="ha_0.1.8_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.8"
  tmp="$(mktemp -d)"
  binary_tmp=
  spar_tmp=
  trap 'rm -rf "$tmp"; if [ -n "$binary_tmp" ]; then rm -f "$binary_tmp"; fi; if [ -n "$spar_tmp" ]; then rm -f "$spar_tmp"; fi' EXIT
  cd "$tmp"
  curl -fsSLO "$url/$name.tar.gz"
  curl -fsSLO "$url/checksums.txt"
  grep " $name.tar.gz\$" checksums.txt > "$name.sha256"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c "$name.sha256"
  else
    shasum -a 256 -c "$name.sha256"
  fi
  tar -xzf "$name.tar.gz"
  if [ ! -f "$name/spar" ] || [ -L "$name/spar" ]; then
    echo "The archive has no spar executable; nothing was replaced: $name.tar.gz" >&2
    exit 1
  fi
  mkdir "$tmp/public"
  cp -R "$name/tools" "$name/examples" "$name/guides" "$name/contracts" "$name/assets" "$tmp/public/"
  cp "$name/README.md" "$name/README.ko.md" "$name/LICENSE" "$tmp/public/"
  share="$HOME/.local/share/jaekit/0.1.8"
  if [ -e "$share" ] || [ -L "$share" ]; then
    if [ ! -d "$share" ] || [ -L "$share" ]; then
      echo "Release data path is not an ordinary directory: $share" >&2
      exit 1
    fi
    special="$(find "$share" ! -type f ! -type d -print)"
    if [ -n "$special" ]; then
      echo "Release data contains entries that are not ordinary files or directories; nothing was replaced:" >&2
      printf '%s\n' "$special" >&2
      exit 1
    fi
    if ! diff -qr "$tmp/public" "$share"; then
      echo "Existing v0.1.8 data differs; nothing was replaced: $share" >&2
      exit 1
    fi
  else
    mkdir -p "$HOME/.local/share/jaekit"
    mv "$tmp/public" "$share"
  fi
  bin="$HOME/.local/bin"
  mkdir -p "$bin"
  if [ -e "$bin/ha" ] || [ -L "$bin/ha" ]; then
    if [ ! -f "$bin/ha" ] || [ -L "$bin/ha" ]; then
      echo "Existing ha path is not an ordinary file; nothing was replaced: $bin/ha" >&2
      exit 1
    fi
  fi
  if [ -e "$bin/spar" ] || [ -L "$bin/spar" ]; then
    if [ ! -f "$bin/spar" ] || [ -L "$bin/spar" ]; then
      echo "Existing spar path is not an ordinary file; nothing was replaced: $bin/spar" >&2
      exit 1
    fi
  fi
  binary_tmp="$(mktemp "$bin/.ha.XXXXXX")"
  cp "$name/ha" "$binary_tmp"
  chmod 755 "$binary_tmp"
  spar_tmp="$(mktemp "$bin/.spar.XXXXXX")"
  cp "$name/spar" "$spar_tmp"
  chmod 755 "$spar_tmp"
  mv -f "$binary_tmp" "$bin/ha"
  binary_tmp=
  echo "Installed: $HOME/.local/bin/ha ($name)"
  mv -f "$spar_tmp" "$bin/spar"
  spar_tmp=
  echo "Installed: $HOME/.local/bin/spar ($name)"
)
```

The commands prepare both executables next to their destinations before replacing either, so a failed download, copy or permission change leaves both as they were. The two replacements are still separate steps: if the commands stop between them (for example after a crash or a power loss), `ha` can be new while `spar` is old. Run the same commands again to bring them together.

If `~/.local/bin` is not on your PATH, add `export PATH="$HOME/.local/bin:$PATH"` to your shell config (such as `~/.zshrc`). If macOS blocks a file you downloaded with a browser, clear it with `xattr -d com.apple.quarantine ~/.local/bin/ha`. Then [check the version](#check-the-version) and install the plugins for [Codex](#codex) or [Claude Code](#claude-code).

## Release and development combinations

| Combination | Core | Spec | Seal | Spar | Scope |
| --- | --- | --- | --- | --- | --- |
| Current release `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 | Everything in v0.1.7, plus `ha cost` (one goal's verification and review cost summary), Seal asking the decisions it can foresee in one message before the first baseline, the smallest fix direction on Spar findings, and a direct installation that stops when `spar` is missing |
| Previous release `v0.1.7` | `ha 0.1.7`, `spar 0.1.7` | 0.1.13 | 0.1.22 | 0.1.3 | Default rules `/3`; Seal starts new goals under `/5` (a `standard` or `strict` goal completes only with a recorded Spar gate), Spar review rounds and gate, runner adapters, `ha report`, `ha doctor` and the automatic budget, Core support check at start (`--check-skill`), Spec that the agent can call |
| Previous release `v0.1.6` | `ha 0.1.6` | 0.1.12 | 0.1.17 | — | Default rules `/3`; Seal starts new goals under `/4` (proceeding within recorded authority), decision history and run timing, check-authoring guidance, final-candidate evidence, post-completion findings, evidence and assurance-policy reporting; no Spar or `/5` |
| Previous release `v0.1.5` | `ha 0.1.5` | 0.1.12 | 0.1.16 | — | Same Core behavior as v0.1.6; Seal before the run limit of a reopened `/4` window was linked to its contract |
| Previous release `v0.1.4` | `ha 0.1.4` | 0.1.12 | 0.1.12 | — | Default rules `/3`, post-completion findings, evidence and assurance-policy reporting; no `/4`, decision history or run timing |
| Previous release `v0.1.3` | `ha 0.1.3` | 0.1.11 | 0.1.9 | — | Default rules `/3`, capabilities, run estimation and total-limit changes; no post-completion findings extension |
| Previous release `v0.1.2` | `ha 0.1.2` | 0.1.10 | 0.1.8 | — | Default rules `/2`; does not meet the new Seal's support requirements |
| Source build | `ha 0.1.8-dev`, `spar 0.1.8-dev` | Checkout version | Checkout version | Checkout version | Development identity; not evidence of installing a release artifact |

The tag-pinned installation commands below install v0.1.8. Seal checks the actual Core executable's capabilities and stops before recording if support is missing or cannot be established. Matching version numbers or switching to older rules does not establish compatibility. See the [Core capabilities contract](../contracts/core-capabilities.md) for the required support.

Spec writes documents without installing Core or querying capabilities. New Spec 0.1.13 documents use `nested/1`, so implementation needs a Core that supports that format. Updates do not change existing goal formats, `/1`, `/2` or `/3` records, usage or time limits. See the [v0.1.8 release notes](https://github.com/jgoneit/jaekit/releases/tag/v0.1.8) for the scope of actual host checks.

## Get the reference check

The v0.1.8 archive includes `tools/check-result-reference.py` and `examples/check-result/`. Copy them into your project without a development checkout or Go installation. This producer uses only the Python 3 standard library. Python is needed only when choosing this check, not for Spec or Seal in general. The implementing agent chooses other runners; automatic pytest, Vitest and Playwright adapters are not included.

From your project root, run **one block** for your installation method to select the public data path. Homebrew stores it under `pkgshare`:

```bash
package_root="$(brew --prefix jaekit)/share/jaekit"
```

The direct-install commands above retain the checksum-verified public data in the versioned directory below. If data for that version already exists, it is reused only when it consists of ordinary files and directories with identical contents. Different contents, or other entries such as symlinks, stop installation before replacing the binary and name the path. For a direct installation, use:

```bash
package_root="$HOME/.local/share/jaekit/0.1.8"
```

Then run the following from the project root in the same terminal. It stops if any example destination already exists instead of overwriting it.

```bash
(
  set -e
  for file in tools/check-result-reference.py checks/declaration.json checks/reference.json sample/input.txt; do
    if [ -e "$file" ] || [ -L "$file" ]; then
      echo "File already exists; choose other paths before copying: $file" >&2
      exit 1
    fi
  done
  mkdir -p tools checks sample
  cp "$package_root/tools/check-result-reference.py" tools/
  cp "$package_root/examples/check-result/declaration.json" checks/
  cp "$package_root/examples/check-result/reference.json" checks/
  cp "$package_root/examples/check-result/input.txt" sample/
)
```

Follow the [synthetic example's PLAN and observation steps](../examples/check-result/README.md) to preserve the initial file as the baseline before starting the goal. The command is `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json`; `ha check` supplies the invocation-specific report path. It distinguishes the initial file's actual requirement violation from a pass after the change; environment errors are not expected failures. Commit the declaration, configuration and producer before checking and include all three in PLAN's check paths. Choose targets and paths that fit your project. See the [result contract](../contracts/check-result.md) for the format.

## Update

Move `ha` and the plugins to the new version together, with the tools you installed them with. The commands below move to v0.1.8. Paste one command block at a time.

For an `ha` installed with brew, brew fetches the tap again and then upgrades it.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

For an `ha` installed from a Release file, paste the commands in [Install from a Release file](#install-from-a-release-file) again. The new version's file replaces `~/.local/bin/ha` and puts `~/.local/bin/spar` in place.

Then check both executables' versions and which files actually run. They should print `ha 0.1.8` and `spar 0.1.8`. If either prints another version, the first line of `which -a ha` or `which -a spar` is the file that actually runs; follow [Check the version](#check-the-version) to fix PATH.

```bash
ha --version
which -a ha
spar --version
which -a spar
```

<details>
<summary>Optional, Release file installs only: remove earlier versions' data</summary>

After a Release file installation, the earlier versions' public data stays in `~/.local/share/jaekit/<version>`. Once the check above prints `ha 0.1.8`, the command below removes only the folders of versions older than the installed `0.1.8`. The current version's folder `~/.local/share/jaekit/0.1.8` and the files used by [Get the reference check](#get-the-reference-check), folders of newer versions, and entries whose names are not a three-number version like `0.1.2` stay. Names are checked in full without splitting at newlines or spaces; ordinary files and symlinks with version-like names stay too. Versions are compared as numbers, so `0.1.10` is newer than `0.1.8`. If `~/.local/share/jaekit` is a symlink, nothing is removed. Enumeration, version comparison, or deletion errors make the command fail; older folders already removed successfully are not restored. The data folder first appeared in v0.1.3.

```bash
(
  set -e
  current=0.1.8
  data_root="$HOME/.local/share/jaekit"
  if [ -d "$data_root" ] && [ ! -L "$data_root" ]; then
    find "$data_root" -mindepth 1 -maxdepth 1 -type d -exec sh -c '
      set -e
      current=$1
      shift
      for entry do
        [ -d "$entry" ] && [ ! -L "$entry" ] || continue
        version=${entry##*/}
        case "$version" in "" | *[!0-9.]*) continue ;; esac
        older=$(awk -v version="$version" -v current="$current" "
          BEGIN {
            if (version !~ /^[0-9]+[.][0-9]+[.][0-9]+$/) { print 0; exit }
            split(version, v, /[.]/)
            split(current, c, /[.]/)
            for (i = 1; i <= 3; i++) {
              if (v[i] + 0 != c[i] + 0) { print (v[i] + 0 < c[i] + 0); exit }
            }
            print 0
          }")
        case "$older" in
          1) rm -rf "$entry" ;;
          0) ;;
          *) echo "Could not compare version: $version" >&2; exit 1 ;;
        esac
      done
    ' sh "$current" {} +
  fi
)
```

</details>

For the plugins, register the marketplace again pinned to the new tag. Two marketplaces with the same name cannot be registered together, so remove it first. Removing it also removes the spec, seal and spar plugins installed from it, so install them again. Paste only the commands for the host you use.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.8
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.8
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

Open the host again to load the new Spec, Seal and Spar.

## Uninstall

Remove `ha`, the plugins, and the `jaekit` marketplace. Paste the commands for how you installed it and the host you use.

For an `ha` installed with brew:

```bash
brew uninstall jgoneit/tap/jaekit
```

If you no longer use the tap either, remove it with `brew untap jgoneit/tap`.

For an `ha` installed from a Release file:

```bash
rm ~/.local/bin/ha
```

From v0.1.7 the direct installation also put `spar` there; remove it too.

```bash
rm ~/.local/bin/spar
```

Also remove the folder where the direct installation keeps the public data. Reference check files you copied into your project (such as `tools/check-result-reference.py`) and your goal documents and records stay. If `~/.local/share/jaekit` is a symlink, only the link is removed and its target stays.

```bash
rm -rf ~/.local/share/jaekit
```

Codex:

```bash
codex plugin remove spec@jaekit
codex plugin remove seal@jaekit
codex plugin remove spar@jaekit
codex plugin marketplace remove jaekit
```

Claude Code:

```bash
claude plugin uninstall spec@jaekit
claude plugin uninstall seal@jaekit
claude plugin uninstall spar@jaekit
claude plugin marketplace remove jaekit
```

Installation, updates and removal preserve your project's documents, records and raw output. Whether Git tracks the goal documents and the files Seal made (`SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, and `runs.jsonl` in `docs/specs/<goal>/`) depends on project policy. Both tracked records and ignored private records stay. Raw check output is under `ha/runs/` in that working tree's Git directory, which git does not track. That is usually under `.git/ha/`; in a linked worktree, where `.git` is a file, it is under the directory that `git rev-parse --absolute-git-dir` prints.

The optional [decision history](../contracts/decision-history.md) is under `ha/history/`, per run start record, in the common Git directory that every working tree of the repository shares (`git rev-parse --git-common-dir`). In a normal checkout that is the same `.git/ha/` as the raw check output. Removing a linked worktree leaves this history in place, but the remaining history does not preserve or restore goal documents or raw check output. It does not travel to a new clone or another machine, and deleting the whole Git directory removes it with no automatic recovery.

If you also choose to remove goal documents and records, first check whether they need preserving and whether Git tracks them. For a tracked goal, run `git rm -r docs/specs/<goal>` from that repository's root and commit. This command does not remove untracked goals; handle those separately under the project's local retention policy. Do not force ignored documents into Git.

To remove the raw check output too, paste the command below in that working tree while no Jaekit check is running (while Seal is not working). It removes the raw check output of every goal in that working tree. Decision history, the output of other working trees, and the goal documents and run records stay.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha/runs"
```

## Move from a local path registration

If you registered the `jaekit` marketplace from a repository checkout path, remove that registration and register the GitHub repository pinned to `v0.1.8` instead. Two marketplaces with the same name cannot be registered together. Removing it also removes the spec, seal and spar plugins installed from it, so install them again.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.8
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.8
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

If an `ha` you built earlier with `go install` is still around, clean it up as [Check the version](#check-the-version) describes.
