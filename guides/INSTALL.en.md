# Install guide

[한국어](INSTALL.md) · **English**

Read this guide to install Jaekit for the first time, update it to a new version, or remove it.

Jaekit consists of two terminal programs, `ha` and `spar`, and three agent plugins, Spec, Seal and Spar. The programs install with brew or a Release file. The plugins install with the plugin commands of Codex or Claude Code. Jaekit needs both.

This guide is for the current release, `v0.1.9`. It supports macOS and Linux on arm64 and amd64. Run all the commands below **in Terminal**. After installing, see [your first task](../README.md#your-first-task) for what to send to the agent.

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

You need Git 2.36 or later and either Codex or Claude Code. For the brew route, install [Homebrew](https://brew.sh) too. Your project must be a folder whose changes are tracked with Git. You do not need to clone (copy to your computer) the Jaekit repository.

> **Windows.** The current public release of Seal Core `ha` does not support Windows. The [Windows native candidate guide](WINDOWS.en.md) covers a separate candidate package. Actual installation, Core execution and host behavior on Windows 11 x64 with PowerShell 5.1/7 have not been checked yet. Cross-compilation and local tests do not establish them.

> **WSL2.** The note above is about native Windows execution. A [separate guide](WSL.en.md) prepares the Linux release route for Ubuntu 24.04 LTS x64 in WSL2. Actual WSL2 installation, execution and agent use have not been checked yet. Ordinary Linux results do not establish them.

### Install ha

```bash
brew install jgoneit/tap/jaekit
```

Run one block at a time. If you paste the next commands before brew finishes, brew can consume those lines.

Brew installs `ha` and `spar` together. It downloads the Release executable and verifies its checksum (a value used to check file contents). It does not install Go or build the source. Without Homebrew, follow [Install from a Release file](#install-from-a-release-file).

### Check the version

```bash
ha --version
which -a ha
spar --version
which -a spar
```

`ha --version` should print `ha 0.1.9`. `spar --version` should print the same version, `spar 0.1.9`.

The first line from `which -a ha` is the file that actually runs. For `spar`, look at the first line from `which -a spar`. PATH is the list of folders where the terminal looks for commands. It looks in the folders at the front first.

If the version differs, check this path first. If an older file built with `go install` comes first, remove that file or change your PATH order. That file is in the folder that `go env GOBIN` prints. If `go env GOBIN` is empty, it is in `$(go env GOPATH)/bin`.

If the command is not found and `which -a ha` is empty too, first check that installation finished. Then check that the folder you installed into is on PATH. Set PATH so the agent can find `ha` too, then reopen Codex or Claude Code.

### Codex

First [install ha](#install-ha) and [check its version](#check-the-version). To run the installation commands below, you need [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), the Terminal tool. Plugins work in Codex CLI and the desktop app. The Codex IDE extension does not support them.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.9
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

`@v0.1.9` pins the marketplace (the source of the plugin list) to that tag. So even when main changes, you get this release's spec 0.1.16, seal 0.1.36 and spar 0.1.9.

Reopen Codex. Type `$` in the conversation and choose Jaekit's `spec:spec` or `seal:seal`. The invocation names are `$spec:spec` and `$seal:seal`. Then follow [your first task](../README.md#your-first-task). If they get mixed up with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

### Claude Code

First [install ha](#install-ha) and [check its version](#check-the-version). Then install the plugins in Claude Code with the commands below.

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.9
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
claude plugin list
```

`#v0.1.9` pins the marketplace to that tag. In the final list, check that `spec@jaekit`, `seal@jaekit` and `spar@jaekit` are enabled. If they get mixed up with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

Reopen Claude Code. In the conversation, define the goal with `/spec:spec <request>`. When Spec asks whether to implement now, answering yes starts implementation. You can also start later with `/seal:seal docs/specs/<goal>` in a separate message. Plugin skills are invoked as `/plugin-name:skill-name`. The [usage guide](USAGE.en.md#sending-commands) explains how to review the goal and continue a task.

### Spar

Spar runs independent review rounds with several reviewer CLIs. See the [Spar guide](SPAR.en.md) for details.

`standard` and `strict` goals need the Spar executable and the Spar plugin. Such a goal completes only with a recorded Spar gate.

These goals do not need Spar:

- `light` goals
- Goals without an `Assurance:` line

To finish without review, change the goal document to lower the tier to `light`. The user decides this change ([Review gate](RULES.en.md#review-gate)). A goal with a `review` condition also needs that criterion changed. Otherwise it stops at `review_tier_missing` and does not complete.

The Homebrew formula and the Release file install `ha` and `spar` together. Both have the same version. Check them with these commands:

```bash
spar --version
which -a spar
```

`spar --version` should print the same version as `ha --version`.

The [Codex](#codex) and [Claude Code](#claude-code) installation commands above install the Spar plugin too. If you installed only spec and seal, add it from the marketplace pinned to v0.1.9:

```bash
codex plugin add spar@jaekit
claude plugin install spar@jaekit
```

Then reopen the host. Before the first round, list only the CLIs you have as reviewers, as [Configure reviewers](SPAR.en.md#configure-reviewers) shows. Check the result with `spar config`. Seal runs the review rounds of `standard` and `strict` goals with Spar. When some other plan or change needs an independent review, use `$spar:spar` in Codex or `/spar:spar` in Claude Code.

## Install from a Release file

Instead of brew, you can install from the file `ha_0.1.9_<os>_<arch>.tar.gz` in the [v0.1.9 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.9). `<os>` is `darwin` (macOS) or `linux`. `<arch>` is `arm64` (Apple Silicon and others) or `amd64` (Intel and others).

Paste the commands below as they are. They pick the file for your OS and CPU and download it. They verify its checksum, then put `ha` and `spar` at `~/.local/bin/ha` and `~/.local/bin/spar`.

On an unsupported OS or CPU, the commands stop without downloading or installing anything. If the extracted file has no `spar`, they print a message and stop without changing anything.

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
  name="ha_0.1.9_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.9"
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
  share="$HOME/.local/share/jaekit/0.1.9"
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
      echo "Existing v0.1.9 data differs; nothing was replaced: $share" >&2
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

The commands prepare both executables next to their destinations before replacing either. So if a download, copy or permission change fails, both files stay as they were. The two replacements are still separate steps. If the commands stop between them (for example after a crash or a power loss), `ha` can be new while `spar` is old. Run the same commands again to bring both to the same version.

If `~/.local/bin` is not on your PATH, add `export PATH="$HOME/.local/bin:$PATH"` to your shell config file (such as `~/.zshrc`). If macOS blocks a file you downloaded with a browser, clear it with `xattr -d com.apple.quarantine ~/.local/bin/ha ~/.local/bin/spar`. Then [check the version](#check-the-version) and install the plugins for [Codex](#codex) or [Claude Code](#claude-code).

## Release and development combinations

| Combination | Core | Spec | Seal | Spar |
| --- | --- | --- | --- | --- |
| Current release `v0.1.9` | `ha 0.1.9`, `spar 0.1.9` | 0.1.16 | 0.1.36 | 0.1.9 |
| Previous release `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |
| Source build | `ha 0.1.9-dev`, `spar 0.1.9-dev` | Checkout version | Checkout version | Checkout version |

What each release includes:

**Current release `v0.1.9`.** It adds the following to everything in v0.1.8.

- Seal starts new goals under `/8`. So only marked change conditions need a baseline, and there is no check freeze. Elapsed time is informational, and `review` conditions are met by the review gate.
- `PROGRESS.md` has the `## 검수 요약` review summary.
- `ha report` shows review findings.
- Spar has a cap report, `spar cap-report`. `spar extend --cause` records why a cap was extended. `ha cost` shows that cause.
- Where the user's settings leave a reviewer's model and effort unset, the agent chooses them. The user can bound that choice with an allow list.
- Spar reviewers are given standpoints.
- Candidate rounds receive the earlier dispositions.
- Each reviewer has a packet size limit (`max_packet_bytes`).

These changed or were removed:

- `ha history` is removed.
- `spar run --stage plan-delta` and `fixes` are usage errors (64).
- The default code round cap drops from 3 to 2.

**Source build.** A development identity. It is not evidence of installing a release artifact.

What earlier releases contain is in the [release list](https://github.com/jgoneit/jaekit/releases).

The tag-pinned installation commands in this guide install v0.1.9. Seal checks the capabilities of the Core executable it will actually use. If support is missing or cannot be established, Seal stops before recording. Matching version numbers or switching to older rules does not establish compatibility. See the [Core capabilities contract](../contracts/core-capabilities.md) for the required support.

Spec writes documents without installing Core or querying capabilities. New documents from Spec 0.1.16 use `nested/1`. So implementation needs a Core that supports that format. Updates do not change existing goal formats, records, or usage and time limits. See the [v0.1.9 release notes](https://github.com/jgoneit/jaekit/releases/tag/v0.1.9) for the scope of actual host checks.

## Get the reference check

The v0.1.9 archive includes `tools/check-result-reference.py` and `examples/check-result/`. You can copy them into your project without a development checkout or a Go installation. This producer uses only the Python 3 standard library. Python is needed only when you choose this check. It is not a requirement of Spec or Seal. The implementing agent chooses other runners. Automatic pytest, Vitest and Playwright adapters are not included.

First select the public data path. From your project root, run **one block** for your installation method. For a Homebrew installation, use this `pkgshare` location:

```bash
package_root="$(brew --prefix jaekit)/share/jaekit"
```

The direct-install commands above keep the checksum-verified public data in the versioned directory below. If data for that version already exists, it is reused only when it consists of ordinary files and directories with identical contents. If the contents differ, or there are other entries such as symlinks, installation stops before replacing the binary and names the path. For a direct installation, use:

```bash
package_root="$HOME/.local/share/jaekit/0.1.9"
```

Then run the following from the project root in the same terminal. It stops instead of overwriting if any example destination already exists.

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

Next, follow the [synthetic example's PLAN and observation steps](../examples/check-result/README.md). Preserve the initial file as the baseline before you start the goal.

The check command is `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json`. `ha check` supplies the report path for each invocation. The check tells the initial file's actual requirement violation apart from a pass after the change. Environment errors are not expected failures.

Commit the declaration, configuration and producer before checking. Include all three in PLAN's check paths. Choose targets and paths that fit your project. See the [result contract](../contracts/check-result.md) for the format.

## Update

Move `ha` and the plugins to the new version together. Upgrade with the tools you installed them with. The commands below move to v0.1.9. Paste one command block at a time.

For an `ha` installed with brew, brew fetches the tap again first and then upgrades it.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

For an `ha` installed from a Release file, paste the commands in [Install from a Release file](#install-from-a-release-file) again. The new version's file replaces `~/.local/bin/ha`. It also puts the new file at `~/.local/bin/spar`.

Then check both executables' versions and which files actually run. They should print `ha 0.1.9` and `spar 0.1.9`. If either prints another version, follow [Check the version](#check-the-version) to fix PATH. The first line of `which -a ha` or `which -a spar` is the file that actually runs.

```bash
ha --version
which -a ha
spar --version
which -a spar
```

<details>
<summary>Optional, Release file installs only: remove earlier versions' data</summary>

After a Release file installation, the earlier versions' public data stays in `~/.local/share/jaekit/<version>`. Once the check above prints `ha 0.1.9`, the command below removes only the folders of versions older than the installed `0.1.9`. The data folder first appeared in v0.1.3.
- The current version's folder `~/.local/share/jaekit/0.1.9` and the files used by [Get the reference check](#get-the-reference-check), folders of newer versions, and entries whose names are not a three-number version like `0.1.2` stay.
- Names are checked in full without splitting at newlines or spaces; ordinary files and symlinks with version-like names stay too.
- Versions are compared as numbers, so `0.1.10` is newer than `0.1.9`.
- If `~/.local/share/jaekit` is a symlink, nothing is removed.
- Enumeration, version comparison, or deletion errors make the command fail; older folders already removed successfully are not restored.

```bash
(
  set -e
  current=0.1.9
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

For the plugins, register the marketplace again, pinned to the new tag. Two marketplaces with the same name cannot be registered together, so remove it first. Removing it also removes the spec, seal and spar plugins installed from it, so install them again. Paste only the commands for the host you use.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.9
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.9
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

Open the host again to load the new Spec, Seal and Spar.

## Uninstall

Remove `ha`, the plugins, and the `jaekit` marketplace. Paste the commands that match how you installed it and the host you use.

For an `ha` installed with brew:

```bash
brew uninstall jgoneit/tap/jaekit
```

If you no longer use the tap either, remove it with `brew untap jgoneit/tap`.

For an `ha` installed from a Release file:

```bash
rm ~/.local/bin/ha
```

The direct installation also puts `spar` there. Remove it too.

```bash
rm ~/.local/bin/spar
```

Also remove the folder where the direct installation keeps the public data. Reference check files you copied into your project (such as `tools/check-result-reference.py`) stay. Your goal documents and records stay too. If `~/.local/share/jaekit` is a symlink, only the link is removed and its target stays.

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

Installation, updates and removal do not delete your project's documents, records or raw output.

The goal documents and the files Seal made are `SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md` and `runs.jsonl` in `docs/specs/<goal>/`. Whether Git tracks them depends on project policy. Both tracked records and ignored private records stay as they are.

Raw check output is under `ha/runs/` in that working tree's Git directory. git does not track this folder. It is usually under `.git/ha/`. In a linked worktree, where `.git` is a file, it is under the directory that `git rev-parse --absolute-git-dir` prints.

If you also choose to remove goal documents and records, first check whether they need preserving and whether Git tracks them. For a tracked goal, run `git rm -r docs/specs/<goal>` from that repository's root and commit. This command does not remove untracked goals. Handle those separately under the project's local retention policy. Do not force ignored documents into Git.

To remove the raw check output too, paste the command below in that working tree. Run it while no Jaekit check is running, that is, while Seal is not working. It removes the raw check output of every goal in that working tree. The output of other working trees stays. The goal documents and run records stay too.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha/runs"
```

## Move from a local path registration

If you registered the `jaekit` marketplace from a repository checkout path, remove that registration. Then register the GitHub repository pinned to `v0.1.9` instead. Two marketplaces with the same name cannot be registered together. Removing it also removes the spec, seal and spar plugins installed from it, so install them again.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.9
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.9
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

If an `ha` you built earlier with `go install` is still around, clean it up as [Check the version](#check-the-version) describes.
