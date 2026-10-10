# Install guide

[한국어](INSTALL.md) · **English**

Installation, updates, and removal for the current release, `v0.1.2`. Supported platforms are macOS and Linux on arm64 and amd64. Run the commands below **in Terminal**. After installing, see [your first task](../README.en.md#your-first-task) for what to send to the agent.

- First installation: [Basic install](#basic-install)
- Without brew: [Install from a Release file](#install-from-a-release-file)
- Moving to a new version: [Update](#update)
- Stopping using it: [Uninstall](#uninstall)
- Moving from a registration that points at a repository checkout: [Move from a local path registration](#move-from-a-local-path-registration)
- Working on jaekit itself: [Build from source (development)](#build-from-source-development)

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

`ha --version` should print `ha 0.1.2`. The first line from `which -a ha` is the file that actually runs. PATH lists the folders to search for commands, in order. If the version differs, check this path first. If an older file built with `go install` comes first, remove that file or change your PATH order. It lives in the folder printed by `go env GOBIN`, or in `$(go env GOPATH)/bin` when that is empty.

If the command is missing and `which -a ha` is empty, check that installation finished and that its folder is on PATH. Reopen Codex or Claude Code after setting it so the agent can also find `ha`.

### Codex

First [install ha](#install-ha) and [check its version](#check-the-version). The installation commands below require [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), the Terminal tool. Plugins work in Codex CLI and the desktop app; the Codex IDE extension does not support them.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.2
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

`@v0.1.2` pins the marketplace (the source of the plugin list) to that tag. Even when main changes, you get the release's spec 0.1.10 and seal 0.1.8.

Reopen Codex, type `$` in the conversation, and choose Jaekit's `spec:spec` or `seal:seal`. The invocation names are `$spec:spec` and `$seal:seal`. Follow [your first task](../README.en.md#your-first-task). If you confuse them with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

### Claude Code

First [install ha](#install-ha) and [check its version](#check-the-version). Then install the plugins in Claude Code with the commands below.

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.2
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin list
```

`#v0.1.2` pins the marketplace to that tag. Check that `spec@jaekit` and `seal@jaekit` are enabled in the list. If you confuse them with another plugin, see [troubleshooting](USAGE.en.md#troubleshooting).

Reopen Claude Code and use `/spec:spec <request>` in the conversation to define the goal, then `/seal:seal docs/specs/<goal>` in a separate message to start implementation. Plugin skills use `/plugin-name:skill-name`. The [usage guide](USAGE.en.md#sending-commands) explains how to review the goal and continue a task.

## Install from a Release file

Instead of brew, you can install from the file `ha_0.1.2_<os>_<arch>.tar.gz` in the [v0.1.2 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.2). `<os>` is `darwin` (macOS) or `linux`, and `<arch>` is `arm64` (Apple Silicon and others) or `amd64` (Intel and others).

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
  name="ha_0.1.2_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.2"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
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
  mkdir -p "$HOME/.local/bin"
  mv "$name/ha" "$HOME/.local/bin/ha"
  echo "Installed: $HOME/.local/bin/ha ($name)"
)
```

If `~/.local/bin` is not on your PATH, add `export PATH="$HOME/.local/bin:$PATH"` to your shell config (such as `~/.zshrc`). If macOS blocks a file you downloaded with a browser, clear it with `xattr -d com.apple.quarantine ~/.local/bin/ha`. Then [check the version](#check-the-version) and install the plugins for [Codex](#codex) or [Claude Code](#claude-code).

## Update

Move `ha` and both plugins to the new version together, with the tools you installed them with. The commands below move to v0.1.2. Paste one command block at a time.

For an `ha` installed with brew, brew fetches the tap again and then upgrades it.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

For an `ha` installed from a Release file, paste the commands in [Install from a Release file](#install-from-a-release-file) again. The new version's file replaces `~/.local/bin/ha`.

Then check the version. It should print `ha 0.1.2`. If it prints another version, follow [Check the version](#check-the-version) and inspect `which -a ha`.

```bash
ha --version
```

For the plugins, register the marketplace again pinned to the new tag. Two marketplaces with the same name cannot be registered together, so remove it first. Removing it also removes the spec and seal plugins installed from it, so install them again. Paste only the commands for the host you use.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.2
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.2
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

Files created in the repositories you worked on are not removed; they stay. The goal documents and the files Seal made (`SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, and `runs.jsonl` in `docs/specs/<goal>/`) are committed records, and the raw check output is under `ha/` in that working tree's Git directory, which git does not track. That is usually `.git/ha/`; in a linked worktree, where `.git` is a file, it is under the directory that `git rev-parse --absolute-git-dir` prints.

To remove the goal documents and records too, from the root of that repository run `git rm -r docs/specs/<goal>` and commit.

To remove the raw check output too, paste the command below in that working tree while no Jaekit check is running (while Seal is not working). It removes the raw check output of every goal in that working tree. The output of other working trees and the committed goal documents and run records stay.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha"
```

## Move from a local path registration

If you registered the `jaekit` marketplace from a repository checkout path, remove that registration and register the GitHub repository pinned to `v0.1.2` instead. Two marketplaces with the same name cannot be registered together. Removing it also removes the spec and seal plugins installed from it, so install them again.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.2
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.2
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

If an `ha` you built earlier with `go install` is still around, clean it up as [Check the version](#check-the-version) describes.

## Build from source (development)

An `ha` built from a repository checkout shows a development version (`ha 0.1.2-dev`). You need Go 1.26 or later and git. Run this from the root of the checkout.

```bash
go install ./cmd/ha
ha --version
```

The `ha` file goes to the folder `go env GOBIN` prints, or to `$(go env GOPATH)/bin` when that is empty. `ha --version` prints `ha 0.1.2-dev`.

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
