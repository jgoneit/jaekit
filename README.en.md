<h1 align="center">
  <img src="assets/readme/brand/logo.svg" alt="Jaekit" width="280" />
</h1>

<p align="center">
  <a href="https://github.com/jgoneit/jaekit/releases"><img src="https://img.shields.io/github/v/release/jgoneit/jaekit?display_name=tag&amp;label=release&amp;color=4F6CF7" alt="Latest release" /></a>
  <a href="guides/INSTALL.en.md"><img src="https://img.shields.io/badge/macOS%20%C2%B7%20Linux-arm64%20%C2%B7%20amd64-596579" alt="macOS and Linux, arm64 and amd64" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-168975" alt="MIT license" /></a>
</p>

<p align="center"><a href="README.md">한국어</a> · <strong>English</strong><br /><a href="#install">Install</a> · <a href="#your-first-task">First task</a> · <a href="guides/USAGE.en.md">Usage guide</a></p>

Jaekit keeps **goals and check records for project work you delegate to Codex or Claude Code**. **Spec** defines the work and its completion conditions; **Seal** guides the agent through that goal and keeps a record of its checks. If the conversation ends, you can continue from saved progress.

The current release is **v0.1.3**, an early version being refined through real work.

<details>
<summary>View the Jaekit introduction illustration</summary>

![Jaekit — AI coding work, from goals to completion evidence. Use it with Codex or Claude Code.](assets/readme/hero.en.png)

</details>

## How it works

![1. Ask Spec and review the goal and completion conditions. 2. In a separate message, specify the goal folder and start Seal; the agent implements, checks, and fixes. 3. Review the changes and check records. To resume in a new conversation, specify the same goal in the same project.](assets/readme/usage-flow.en.png)

Spec writes the goal document and **stops**. Review it, then **start Seal in a separate message** for the Codex or Claude Code agent to implement, check, and fix the work. You do not approve each step; respond when a decision affects the outcome or a check needs your confirmation.

When the required completion conditions are met, Seal saves a completion record and report. Review **what changed and how it was checked**, along with the actual result.

## Get started

### Install

These steps use **macOS + Codex**. You need [Homebrew](https://brew.sh) (a program installer), Git, [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), and **a project folder managed with Git**. You do not need to clone the Jaekit repository.

For Claude Code, follow the [Claude Code installation](guides/INSTALL.en.md#claude-code). For Linux or an install without Homebrew, see the [install guide](guides/INSTALL.en.md). The record tool `ha` does not support Windows, so Seal cannot run there. Spec does not need `ha`, but its installation and operation on Windows have not been checked yet.

**1. In Terminal, install the record tool `ha`.** Run one block at a time and wait for it to finish.

```bash
brew install jgoneit/tap/jaekit
```

```bash
ha --version
```

It should print `ha 0.1.3`. If the command is missing or the version differs, see [Check the version](guides/INSTALL.en.md#check-the-version).

**2. In the same Terminal, install the Codex plugins.**

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

**Reopen Codex after installation.** `@v0.1.3` pins the release to use. If you already installed Jaekit, first see [Update](guides/INSTALL.en.md#update) or [Move from a local path registration](guides/INSTALL.en.md#move-from-a-local-path-registration).

### Your first task

Open Codex CLI or the desktop app in your project folder. **From here on, type in the agent conversation, not Terminal.**

**1. Ask Spec.** Type `$`, choose Jaekit's `spec:spec` from the list, and describe the work you want.

```text
$spec:spec Add a "Forgot password" link to the login screen
```

This is a **fictional example** of a project that already has a password reset page, not an executed or verified case. Replace it with the work you want in your own project.

**2. Review the goal.** Read the document Spec gives you and say what needs changing. In this example, the completion conditions are that the new link opens the existing reset page and login keeps working as before.

**3. Start Seal in a separate message.** Choose Jaekit's `seal:seal` from the `$` list and specify the goal folder.

```text
$seal:seal docs/specs/<goal>
```

Replace `<goal>` with **the actual goal folder name Spec reported**. If its name is `password-link`, the full path is `docs/specs/password-link`. For Claude Code's `/spec:spec` and `/seal:seal` commands, see the [usage guide](guides/USAGE.en.md#sending-commands).

### If the conversation ends

Open **a new conversation in the same project folder** and specify the same goal. Replace `<goal>` with its actual folder name to continue from the saved goal and progress records.

```text
continue docs/specs/<goal>
```

## Before you start

<details>
<summary><strong>What should I look for in the completion report?</strong></summary>

Required completion conditions need recorded checks or necessary user confirmations. The agent writes the automated checks, so completion is not a guarantee that the entire result is flawless or independently verified by a third party. Review the report's assumptions and remaining limitations alongside the actual result.

The default result ends at local changes and commits (saved entries in the change history). Publish to GitHub, create pull requests, and deploy yourself, or ask a general-purpose agent separately. [Reading the completion report](guides/USAGE.en.md#reading-the-completion-report)

</details>

<details>
<summary><strong>Can I just define a goal?</strong></summary>

Yes. Spec works without Seal or `ha`. Define the goal now and start Seal when you are ready to implement it.

</details>

<details>
<summary><strong>What records are saved?</strong></summary>

The project keeps goal documents, work plans, progress and completion reports, and run records. Read the progress report in `PROGRESS.md` in the goal folder. Installing, updating, or uninstalling does not delete these records. Git tracking follows project policy; review the contents before publishing. [Saved files](guides/USAGE.en.md#saved-files)

</details>

## More

- [Install, update, and uninstall](guides/INSTALL.en.md) · [Releases and compatibility](guides/INSTALL.en.md#release-and-development-combinations)
- [Usage, changing a goal, and resuming](guides/USAGE.en.md) · [Troubleshooting](guides/USAGE.en.md#troubleshooting)
- [Detailed operations](guides/OPERATIONS.md) · [Command and record formats](contracts/run-record.md) (both in Korean)
- [About the images and how to edit them](assets/readme/SOURCES.en.md)

Share your experience or where you got stuck in a [usage report or install/bug issue](https://github.com/jgoneit/jaekit/issues/new/choose). Issues are public: leave out company code, internal names, secrets, and raw logs.

[MIT license](LICENSE)
