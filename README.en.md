<h1 align="center">
  <img src="assets/readme/brand/logo.svg" alt="Jaekit" width="280" />
</h1>

<p align="center">
  <a href="https://github.com/jgoneit/jaekit/releases"><img src="https://img.shields.io/github/v/release/jgoneit/jaekit?display_name=tag&amp;label=release&amp;color=4F6CF7" alt="Latest release" /></a>
  <a href="guides/INSTALL.en.md"><img src="https://img.shields.io/badge/macOS%20%C2%B7%20Linux-arm64%20%C2%B7%20amd64-596579" alt="macOS and Linux, arm64 and amd64" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-168975" alt="MIT license" /></a>
</p>

<p align="center"><a href="README.md">한국어</a> · <strong>English</strong><br /><a href="#install">Install</a> · <a href="#update">Update</a> · <a href="#your-first-task">First task</a> · <a href="guides/USAGE.en.md">Usage guide</a></p>

Jaekit **carries work you delegate to Codex or Claude Code from a goal to a completion record**.

- **Spec** turns what you want into a goal document with completion conditions.
- **Seal** has the agent implement, check, and fix the work toward that goal, and judges completion from check records.

The current release is **v0.1.6**, an early version being refined through real work.

<details>
<summary>View the Jaekit introduction illustration</summary>

![Jaekit — AI coding work, from goals to completion evidence. Use it with Codex or Claude Code.](assets/readme/hero.en.png)

</details>

## Why use it

- **You want "done" backed by records**
  Completion is recorded only when the required completion conditions are met by recorded checks or necessary user confirmations. The completion report shows what changed and how it was checked.
- **You want to pick up where a conversation ended**
  Goals, plans, and progress records stay in the project folder. In a new conversation, specify the same goal to continue from the saved point.
- **You do not want to approve every step**
  The agent plans, implements, and checks on its own. It asks when a decision affects the outcome or a check needs your confirmation.
- **You want to define the request before implementing it**
  Spec works on its own. Keep the goal and completion conditions as a document and start Seal when you are ready.

## How it works

![1. Ask Spec and review the goal and completion conditions. 2. In a separate message, specify the goal folder and start Seal; the agent implements, checks, and fixes. 3. Review the changes and check records. To resume in a new conversation, specify the same goal in the same project.](assets/readme/usage-flow.en.png)

1. Spec writes the goal document and **stops**. Read it and say what needs changing.
2. When it looks right, **start Seal in a separate message**. The agent implements, checks, and fixes the work.
3. When the required completion conditions are met, Seal saves a completion record and report. Review **what changed and how it was checked**, along with the actual result.

## Get started

### What you need

- **Git** and **a project folder managed with Git.** You do not need to clone the Jaekit repository.
- **Claude Code** or **[Codex CLI](https://learn.chatgpt.com/docs/codex/cli)**
- On macOS and Linux, **[Homebrew](https://brew.sh)** (a program installer) is recommended. Without it, install from a Release file.

### Install

First check what you can use in your environment.

| Environment | Spec | Seal | How to install |
| --- | --- | --- | --- |
| macOS (Apple Silicon, Intel) | Yes | Yes | The Homebrew command below |
| Linux (arm64, amd64) | Yes | Yes | The command below with Homebrew; otherwise [Install from a Release file](guides/INSTALL.en.md#install-from-a-release-file) |
| WSL2 on Windows (Ubuntu 24.04 x64) | Prepared, not yet checked in use | Prepared, not yet checked in use | Follow the [WSL2 preparation guide](guides/WSL.en.md) and install from a Release file |
| Windows (native) | Not checked | Not available | No public installation files yet |

**Windows users**

- In the current public release, the record tool `ha` does not support Windows natively, so Seal cannot run there. This limitation refers to native execution.
- Spec does not need `ha`, but its installation and operation on Windows have not been checked yet.
- In WSL2 with Ubuntu 24.04 LTS x64, install from the Linux Release file; Homebrew is not needed. Installation and agent use in actual WSL2 have not been checked yet. → [WSL2 preparation guide](guides/WSL.en.md)
- Native Windows (Windows 11 x64, PowerShell 5.1 and 7) has only a candidate under development; actual execution has not been checked. → [Windows native candidate guide](guides/WINDOWS.en.md)

**1. In Terminal, install the record tool `ha`.** Run one block at a time and wait for it to finish.

```bash
brew install jgoneit/tap/jaekit
```

```bash
ha --version
which -a ha
```

It should print `ha 0.1.6`. The first line of `which -a ha` is the file that actually runs. If the command is missing or the version differs, see [Check the version](guides/INSTALL.en.md#check-the-version).

To install without Homebrew, paste the block under [Install from a Release file](guides/INSTALL.en.md#install-from-a-release-file) instead of this step. It downloads the file for your OS and CPU, checks its checksum, and puts it at `~/.local/bin/ha`. Other methods and details are in the [install guide](guides/INSTALL.en.md).

**2. In the same Terminal, install the plugins for your agent.** Run only the block for the agent you use.

If you already installed Jaekit, follow [Update](#update) below. If you registered a repository checkout path, first see [Move from a local path registration](guides/INSTALL.en.md#move-from-a-local-path-registration).

**Claude Code**

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.6
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin list
```

The final list should show `spec@jaekit` and `seal@jaekit` as enabled. **Reopen Claude Code after installation.** `#v0.1.6` pins the release to use. For details, see [Claude Code installation](guides/INSTALL.en.md#claude-code).

**Codex**

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.6
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

**Reopen Codex after installation.** `@v0.1.6` pins the release to use.

### Update

When a new release comes out, update `ha` and both plugins **together**. With mixed versions, Seal may stop before recording.

**1. Update `ha`.** If you installed it with Homebrew, run these two blocks in order.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

If you installed from a Release file, paste the block under [Install from a Release file](guides/INSTALL.en.md#install-from-a-release-file) again.

**2. Check which `ha` actually runs.**

```bash
ha --version
which -a ha
```

If the new version does not appear, look at the first line of `which -a ha`. That is the file that actually runs.

If you installed both with Homebrew and from a Release file, remove the one you do not use or update it to the same version. Updating only one can leave the older one earlier on PATH still running.

**3. Register the plugins again for the new release.** Removing the marketplace also removes its spec and seal, so install them again.

**Claude Code**

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.6
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

**Codex**

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.6
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

**4. Reopen Claude Code or Codex.** The new Spec and Seal load in newly opened sessions.

Updating does not delete goal documents or run records. To clean up earlier versions' data or uninstall, see [Update](guides/INSTALL.en.md#update) and [Uninstall](guides/INSTALL.en.md#uninstall).

### Your first task

Open Claude Code, Codex CLI, or the Codex desktop app in your project folder.

**From here on, type in the agent conversation, not Terminal.**

**1. Ask Spec.** Use the example for your agent and describe the work you want.

**Claude Code**

```text
/spec:spec Add a "Forgot password" link to the login screen
```

**Codex** — Type `$` and choose Jaekit's `spec:spec` from the list.

```text
$spec:spec Add a "Forgot password" link to the login screen
```

This is a **fictional example** of a project that already has a password reset page, not an executed or verified case. Replace it with the work you want in your own project.

**2. Review the goal.** Read the document Spec gives you and say what needs changing.

In this example, the completion conditions are that the new link opens the existing reset page and login keeps working as before.

**3. Start Seal in a separate message.** Specify the goal folder.

**Claude Code**

```text
/seal:seal docs/specs/<goal>
```

**Codex** — Choose Jaekit's `seal:seal` from the `$` list.

```text
$seal:seal docs/specs/<goal>
```

Replace `<goal>` with **the actual goal folder name Spec reported**. If its name is `password-link`, the full path is `docs/specs/password-link`.

For more on sending commands, see the [usage guide](guides/USAGE.en.md#sending-commands).

### If the conversation ends

Open **a new conversation in the same project folder** and specify the same goal.

```text
continue docs/specs/<goal>
```

Replace `<goal>` with its actual folder name to continue from the saved goal and progress records.

## Before you start

<details>
<summary><strong>What should I look for in the completion report?</strong></summary>

Required completion conditions need recorded checks or necessary user confirmations.

The agent writes the automated checks, so completion is not a guarantee that the entire result is flawless or independently verified by a third party. Review the report's assumptions and remaining limitations alongside the actual result.

The default result ends at local changes and commits (saved entries in the change history). Publish to GitHub, create pull requests, and deploy yourself, or ask a general-purpose agent separately. [Reading the completion report](guides/USAGE.en.md#reading-the-completion-report)

</details>

<details>
<summary><strong>Can I just define a goal?</strong></summary>

Yes. Spec works without Seal or `ha`. Define the goal now and start Seal when you are ready to implement it.

</details>

<details>
<summary><strong>What records are saved?</strong></summary>

The project keeps goal documents, work plans, progress and completion reports, and run records. Read the progress report in `PROGRESS.md` in the goal folder.

Installing, updating, or uninstalling does not delete these records. Git tracking follows project policy; review the contents before publishing. [Saved files](guides/USAGE.en.md#saved-files)

</details>

## More

- [Install, update, and uninstall](guides/INSTALL.en.md) · [Releases and compatibility](guides/INSTALL.en.md#release-and-development-combinations)
- [Usage, changing a goal, and resuming](guides/USAGE.en.md) · [Troubleshooting](guides/USAGE.en.md#troubleshooting)
- [Detailed operations](guides/OPERATIONS.md) · [Command and record formats](contracts/run-record.md) (both in Korean)
- [WSL2 preparation guide](guides/WSL.en.md) · [Windows native candidate guide](guides/WINDOWS.en.md)
- [About the images and how to edit them](assets/readme/SOURCES.en.md)
- [What Ubuntu, macOS and Windows CI checks](guides/CI.en.md)

Share your experience or where you got stuck in a [usage report or install/bug issue](https://github.com/jgoneit/jaekit/issues/new/choose). Issues are public: leave out company code, internal names, secrets, and raw logs.

[MIT license](LICENSE)
