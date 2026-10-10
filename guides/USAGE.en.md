# Usage guide

[한국어](USAGE.md) · **English**

This guide follows [your first task in the README](../README.en.md#your-first-task). For installation, updates, and removal, see the [install guide](INSTALL.en.md).

The current release v0.1.3 combines ha 0.1.3, spec 0.1.11, seal 0.1.9 and default rules `/3`. Preflight refusal, run estimation, `/3` result classification and budget policy apply to this release. Existing goals keep their saved rules. See [Release and development combinations](INSTALL.en.md#release-and-development-combinations).

## Sending commands

Send these **in the agent conversation**, not Terminal. Replace `<request>` with the work you want and `<goal>` with the goal folder name Spec reports.

| What to do | Codex | Claude Code |
| --- | --- | --- |
| Write the goal | `$spec:spec <request>` | `/spec:spec <request>` |
| Start implementation | `$seal:seal docs/specs/<goal>` | `/seal:seal docs/specs/<goal>` |

In Codex, type `$`, select `spec:spec` or `seal:seal` installed from Jaekit, then add your request. `@Spec` is a plugin mention. Use the skill invocation above to explicitly select Spec's instructions for writing goal documents only; a plugin mention alone does not guarantee the same instructions are selected.

`spec:spec` means **plugin name:skill name**. This prefix is part of the plugin naming scheme, even when Jaekit is the only installation. Codex uses `$`; Claude Code uses `/`. Short names for separately installed skills and invocation in other agents may differ. See the [Codex plugin naming rules](https://developers.openai.com/plugins/build/plugins#create-a-plugin-manually), [Codex skill invocation example](https://learn.chatgpt.com/docs/security/plugin/code-changes#automate-reviews-in-cicd), and [Claude Code skill locations and names](https://code.claude.com/docs/en/skills#choose-where-skills-load).

## Changing a goal or continuing

Spec saves the goal, scope, and completion conditions directly to `docs/specs/<goal>/SPEC.md`, reports what it wrote and any decisions it made with their sources, then stops. Even if you also ask it to implement the work, it only writes the goal documents. After reading the report, answer or ask for changes to revise the same document. Decisions Spec proposes become yours when you accept them or ask to implement that goal.

If a goal already covers the same behavior and you have not chosen which document to write, Spec first asks whether to create a new goal or edit the existing one. If an outcome-changing decision remains, it leaves the document at `Status: Draft` and asks. `Status: Ready` means the document is ready; implementation still needs a separate start request.

You can also tell Seal `Implement docs/specs/<goal>` instead of using the start command. You do not approve each step during the work. Answer decisions that affect the outcome and conditions that need your own confirmation.

If the conversation ends, open a new one in the same project folder and send the following. Seal reads the saved goal, plan, progress, and current state, then continues the remaining work.

```text
continue docs/specs/<goal>
```

## Reading the completion report

The report appears in the conversation and in `PROGRESS.md` in the goal folder. Start with what changed and what was verified. Required completion conditions need recorded checks or necessary user confirmations. Optional conditions do not block completion and are identified separately. The agent writes the automated checks.

The evidence of completion is the record written by `ha done`. The assurance level `local` refers to evidence obtained on that computer. An agent with the same user permissions can change the checks and records, so it is not independent third-party verification. Review the assumptions, changed checks, remaining limitations, and the actual result together. The [operations guide](OPERATIONS.md#완료-보고-읽기) (Korean) gives the detailed review procedure.

If something is missing, say what needs fixing. Seal records the rework request and continues. If the goal document itself needs changing, revise it with Spec and then separately ask to continue.

Seal's default result currently ends at local changes and commits (saved entries in the change history). You can push and create pull requests yourself or ask a general-purpose agent separately. Jaekit does not provide an independent verification environment or unattended operation.

## Saved files

The goal folder is `docs/specs/<goal>/`. Spec's `SPEC.md`, Seal's `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, and any needed `tasks/` files hold the goal, plan, plan review, progress and completion report, run record, and individual task descriptions. Git tracking depends on project policy. Commit them when the project tracks them; keep them locally when the project treats them as private or ignored. Do not force ignored documents into Git. A commit saves history locally; publishing it is a separate step. Records can include user statements and working paths, so review them before sending them to a public repository. The Jaekit repository's own private `docs/` policy is not a requirement for other projects.

Raw check output stays in `.git/ha/`, which Git does not track; you do not need to add anything to `.gitignore`. In a linked worktree (a separate working folder sharing the same Git history), it is under `ha/` in the Git directory printed by `git rev-parse --absolute-git-dir`. Output stays local. To inspect it, run `ha log <goal> <seq>` in Terminal: `<goal>` is the goal path and `<seq>` is the record number from the report.

## Troubleshooting

| Symptom | What to do |
| --- | --- |
| `ha: command not found`, or an unexpected version | Follow [Check the version](INSTALL.en.md#check-the-version) to check the installation and which executable runs |
| A skill is missing, or a short name runs another command | Reopen the agent after installation. Use the full names in the table above; in Codex, select the Jaekit skill from the `$` menu |
| Older `spec` or `seal` installations also appear | In Codex, check `codex plugin list` and remove the old installation with `codex plugin remove <plugin>`. In Claude Code, use `claude plugin list` and `claude plugin uninstall <plugin>`. Replace `<plugin>` with the old installation's ID; keep `spec@jaekit` and `seal@jaekit`. If you also copied standalone skills manually, remove their installation folders separately, then reopen the agent |
| `@Spec` in Codex behaves differently than expected | Select Jaekit's `spec:spec` from the `$` menu, then send your request |
| `Cannot add marketplace "jaekit"` | The name is already registered. Follow [Move from a local path registration](INSTALL.en.md#move-from-a-local-path-registration) |
| Seal stops and asks you to decide something | Answer the question. Your words are recorded before work continues. Some completion conditions may need your own confirmation |
| An expired login, denied permission, or usage limit stops the work | Resolve the cause, then send `continue docs/specs/<goal>`. The same terminal failure is not retried |
| Verification is refused because of uncommitted changes | Review the listed paths and tracked/untracked or symlink details, then resolve them according to the intended work. Unstarted checks consume no runs |
| The plan budget is too small to start | Read the minimum, limit and shortfall from `ha estimate docs/specs/<goal>`. Decide a sufficient budget or split the goal before starting; this estimate does not change an existing run budget |
| The budget ran out | Read what remains and decide whether to continue. `/3` can change the total run limit with your words and context, retaining usage and the time limit. Changing the elapsed-time limit is unsupported. Request an explicit reopen when a new window is needed. Existing `/1` or `/2` goals retain their reopen behavior with the original limits |
| Seal stopped for Core compatibility | Read the executable it checked and the missing support. Follow [Update](INSTALL.en.md#update) to install the v0.1.3 combination, then check the capabilities of the Core actually selected. Do not bypass the check by automatically installing tools or switching rules |

The [operations guide](OPERATIONS.md) (Korean) describes the detailed reasons for user confirmation, blocked work, and budget stops. The steps above cover what you need to respond and continue.

## Source reporting format

Seal 0.1.10 source separates expected-result grounds, observation boundaries, methods and substitutes, and unobserved scope in the existing review and report. Condition groups may share an explanation; a plan is not an observation. Existing goal formats and the current released installation remain unchanged. See the [synthetic reporting example](../examples/verification-reporting.md); it is not an actual execution record.
