# Install guide

[한국어](INSTALL.md) · **English**

Installation, updates, and removal for the current release, `v0.1.3`. Supported platforms are macOS and Linux on arm64 and amd64. Run the commands below **in Terminal**. After installing, see [your first task](../README.en.md#your-first-task) for what to send to the agent.

- First installation: [Basic install](#basic-install)
- Without brew: [Install from a Release file](#install-from-a-release-file)
- To use the reference producer and example: [Get the reference check](#get-the-reference-check)
- Moving to a new version: [Update](#update)
- Stopping using it: [Uninstall](#uninstall)
- Moving from a registration that points at a repository checkout: [Move from a local path registration](#move-from-a-local-path-registration)
- Working on jaekit itself: [Build from source (development)](#build-from-source-development)

## Release and development combinations

| Combination | Core | Spec | Seal | Scope |
| --- | --- | --- | --- | --- |
| Current release `v0.1.3` | `ha 0.1.3` | 0.1.11 | 0.1.9 | Default rules `/3`, `nested/1`, dirty/copy-safety and compatibility preflight, run estimation and total-limit changes |
| Previous release `v0.1.2` | `ha 0.1.2` | 0.1.10 | 0.1.8 | Default rules `/2`; does not meet the new Seal's support requirements |
| Source build | `ha 0.1.3-dev` | Checkout version | Checkout version | Development identity; not evidence of installing a release artifact |

The tag-pinned installation commands below install v0.1.3. Seal checks the actual Core executable's capabilities and stops before recording if support is missing or cannot be established. Matching version numbers or switching to older rules does not establish compatibility. See the [Core capabilities contract](../contracts/core-capabilities.md) for the required support.

Spec writes documents without installing Core or querying capabilities. New Spec 0.1.11 documents use `nested/1`, so implementation needs a Core that supports that format. Updates do not change existing goal formats, `/1`, `/2` or `/3` records, usage or time limits. See the [v0.1.3 release notes](https://github.com/jgoneit/jaekit/releases/tag/v0.1.3) for the scope of actual host checks.

## Basic install

You need Git and either Codex or Claude Code. For the brew route, install [Homebrew](https://brew.sh) too. Your project must be a folder whose changes are tracked with Git. You do not need to clone (copy to your computer) the Jaekit repository.

Seal Core `ha` does not support Windows, so Seal cannot run there. Spec can write goal documents without `ha`, but its installation and operation on Windows have not been checked yet.

### Install ha

```bash
brew install jgoneit/tap/jaekit
```

Run one block at a time. Pasting the next commands before brew finishes can cause it to consume those lines. Brew downloads the Release executable and verifies its checksum (a value used to check file contents). It does not install Go or build the source. Without Homebrew, follow [Install from a Release file](#install-from-a-release-file).

### Check the version

```bash
ha --version
which -a ha
```

`ha --version` should print `ha 0.1.3`. The first line from `which -a ha` is the file that actually runs. PATH lists the folders to search for commands, in order. If the version differs, check this path first. If an older file built with `go install` comes first, remove that file or change your PATH order. It lives in the folder printed by `go env GOBIN`, or in `$(go env GOPATH)/bin` when that is empty.

If the command is missing and `which -a ha` is empty, check that installation finished and that its folder is on PATH. Reopen Codex or Claude Code after setting it so the agent can also find `ha`.

### Codex

First [install ha](#install-ha) and [check its version](#check-the-version). The installation commands below require [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), the Terminal tool. Plugins work in Codex CLI and the desktop app; the Codex IDE extension does not support them.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

`@v0.1.3` pins the marketplace (the source of the plugin list) to that tag. Even when main changes, you get the release's spec 0.1.11 and seal 0.1.9.

Reopen Codex, type `$` in the conversation, and choose Jaekit's `spec:spec` or `seal:seal`. The invocation names are `$spec:spec` and `$seal:seal`. Follow [your first task](../README.en.md#your-first-task). If you confuse them with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

### Claude Code

First [install ha](#install-ha) and [check its version](#check-the-version). Then install the plugins in Claude Code with the commands below.

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.3
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin list
```

`#v0.1.3` pins the marketplace to that tag. Check that `spec@jaekit` and `seal@jaekit` are enabled in the list. If you confuse them with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

Reopen Claude Code and use `/spec:spec <request>` in the conversation to define the goal, then `/seal:seal docs/specs/<goal>` in a separate message to start implementation. Plugin skills use `/plugin-name:skill-name`. The [usage guide](USAGE.en.md#sending-commands) explains how to review the goal and continue a task.

## Install from a Release file

Instead of brew, you can install from the file `ha_0.1.3_<os>_<arch>.tar.gz` in the [v0.1.3 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.3). `<os>` is `darwin` (macOS) or `linux`, and `<arch>` is `arm64` (Apple Silicon and others) or `amd64` (Intel and others).

Paste the commands below as they are. They pick the file for your OS and CPU, verify its checksum, and put it at `~/.local/bin/ha`. On an unsupported OS or CPU they stop without downloading or installing anything.

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
  name="ha_0.1.3_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.3"
  tmp="$(mktemp -d)"
  binary_tmp=
  trap 'rm -rf "$tmp"; if [ -n "$binary_tmp" ]; then rm -f "$binary_tmp"; fi' EXIT
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
  mkdir "$tmp/public"
  cp -R "$name/tools" "$name/examples" "$name/guides" "$name/contracts" "$name/assets" "$tmp/public/"
  cp "$name/README.md" "$name/README.en.md" "$name/LICENSE" "$tmp/public/"
  share="$HOME/.local/share/jaekit/0.1.3"
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
      echo "Existing v0.1.3 data differs; nothing was replaced: $share" >&2
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
  binary_tmp="$(mktemp "$bin/.ha.XXXXXX")"
  cp "$name/ha" "$binary_tmp"
  chmod 755 "$binary_tmp"
  mv -f "$binary_tmp" "$bin/ha"
  binary_tmp=
  echo "Installed: $HOME/.local/bin/ha ($name)"
)
```

If `~/.local/bin` is not on your PATH, add `export PATH="$HOME/.local/bin:$PATH"` to your shell config (such as `~/.zshrc`). If macOS blocks a file you downloaded with a browser, clear it with `xattr -d com.apple.quarantine ~/.local/bin/ha`. Then [check the version](#check-the-version) and install the plugins for [Codex](#codex) or [Claude Code](#claude-code).

## Get the reference check

The v0.1.3 archive includes `tools/check-result-reference.py` and `examples/check-result/`. Copy them into your project without a development checkout or Go installation. This producer uses only the Python 3 standard library. Python is needed only when choosing this check, not for Spec or Seal in general. The implementing agent chooses other runners; automatic pytest, Vitest and Playwright adapters are not included.

From your project root, run **one block** for your installation method to select the public data path. Homebrew stores it under `pkgshare`:

```bash
package_root="$(brew --prefix jaekit)/share/jaekit"
```

The direct-install commands above retain the checksum-verified public data in the versioned directory below. If data for that version already exists, it is reused only when it consists of ordinary files and directories with identical contents. Different contents, or other entries such as symlinks, stop installation before replacing the binary and name the path. For a direct installation, use:

```bash
package_root="$HOME/.local/share/jaekit/0.1.3"
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

Move `ha` and both plugins to the new version together, with the tools you installed them with. The commands below move to v0.1.3. Paste one command block at a time.

For an `ha` installed with brew, brew fetches the tap again and then upgrades it.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

For an `ha` installed from a Release file, paste the commands in [Install from a Release file](#install-from-a-release-file) again. The new version's file replaces `~/.local/bin/ha`.

Then check the version. It should print `ha 0.1.3`. If it prints another version, follow [Check the version](#check-the-version) and inspect `which -a ha`.

```bash
ha --version
```

After a Release file installation, the earlier versions' public data stays in `~/.local/share/jaekit/<version>`. Once the check above prints `ha 0.1.3`, the command below removes only the folders of versions older than the installed `0.1.3`. The current version's folder `~/.local/share/jaekit/0.1.3` and the files used by [Get the reference check](#get-the-reference-check), folders of newer versions, and entries whose names are not a three-number version like `0.1.2` stay. Names are checked in full without splitting at newlines or spaces; ordinary files and symlinks with version-like names stay too. Versions are compared as numbers, so `0.1.10` is newer than `0.1.3`. If `~/.local/share/jaekit` is a symlink, nothing is removed. Enumeration, version comparison, or deletion errors make the command fail; older folders already removed successfully are not restored. The data folder first appeared in v0.1.3, so there may be no earlier version to remove yet.

```bash
(
  set -e
  current=0.1.3
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

For the plugins, register the marketplace again pinned to the new tag. Two marketplaces with the same name cannot be registered together, so remove it first. Removing it also removes the spec and seal plugins installed from it, so install them again. Paste only the commands for the host you use.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.3
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

Open the host again to load the new Spec and Seal.

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

Also remove the folder where the direct installation keeps the public data. Reference check files you copied into your project (such as `tools/check-result-reference.py`) and your goal documents and records stay. If `~/.local/share/jaekit` is a symlink, only the link is removed and its target stays.

```bash
rm -rf ~/.local/share/jaekit
```

Codex:

```bash
codex plugin remove spec@jaekit
codex plugin remove seal@jaekit
codex plugin marketplace remove jaekit
```

Claude Code:

```bash
claude plugin uninstall spec@jaekit
claude plugin uninstall seal@jaekit
claude plugin marketplace remove jaekit
```

Installation, updates and removal preserve your project's documents, records and raw output. Whether Git tracks the goal documents and the files Seal made (`SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, and `runs.jsonl` in `docs/specs/<goal>/`) depends on project policy. Both tracked records and ignored private records stay. Raw check output is under `ha/` in that working tree's Git directory, which git does not track. That is usually `.git/ha/`; in a linked worktree, where `.git` is a file, it is under the directory that `git rev-parse --absolute-git-dir` prints.

If you also choose to remove goal documents and records, first check whether they need preserving and whether Git tracks them. For a tracked goal, run `git rm -r docs/specs/<goal>` from that repository's root and commit. This command does not remove untracked goals; handle those separately under the project's local retention policy. Do not force ignored documents into Git.

To remove the raw check output too, paste the command below in that working tree while no Jaekit check is running (while Seal is not working). It removes the raw check output of every goal in that working tree. The output of other working trees and the goal documents and run records stay.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha"
```

## Move from a local path registration

If you registered the `jaekit` marketplace from a repository checkout path, remove that registration and register the GitHub repository pinned to `v0.1.3` instead. Two marketplaces with the same name cannot be registered together. Removing it also removes the spec and seal plugins installed from it, so install them again.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.3
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

If an `ha` you built earlier with `go install` is still around, clean it up as [Check the version](#check-the-version) describes.

## Build from source (development)

An `ha` built from a repository checkout shows a development version (`ha 0.1.3-dev`). You need Go 1.26 or later and git. Run this from the root of the checkout.

```bash
go install ./cmd/ha
ha --version
```

The `ha` file goes to the folder `go env GOBIN` prints, or to `$(go env GOPATH)/bin` when that is empty. `ha --version` prints `ha 0.1.3-dev`. Seal separately checks the actual support information from `ha capabilities --format json`. A Python compatibility helper is optional; Python 3 installation is not a prerequisite for Seal.

To use the plugins while you change the checkout, register the checkout path as the marketplace. That registration uses the plugin files in the checkout as they are. Replace `<path to jaekit>` with the path of your checkout.

Codex:

```bash
codex plugin marketplace add <path to jaekit>
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace add <path to jaekit>
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

Commands and the record format are in [contracts/run-record.md](../contracts/run-record.md).
