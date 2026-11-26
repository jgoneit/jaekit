# Usage guide

[한국어](USAGE.md) · **English**

Look here to change a goal, continue work that stopped, answer Seal's questions, or read a completion report. It follows one goal from handing it over to the end.

New here? Read [Understanding Jaekit](OVERVIEW.en.md) and [your first task in the README](../README.md#your-first-task) first. Installation, updates and removal are in the [install guide](INSTALL.en.md). Exactly how reports, the review gate, cost and budgets work is collected in [Details and rules](RULES.en.md).

## Sending commands

Send these **in the agent conversation**, not Terminal. Replace `<request>` with the work you want. Replace `<goal>` with the goal folder name Spec reports.

| What to do | Codex | Claude Code |
| --- | --- | --- |
| Write the goal | `$spec:spec <request>` | `/spec:spec <request>` |
| Start implementation | `$seal:seal docs/specs/<goal>` | `/seal:seal docs/specs/<goal>` |

In Codex, type `$`, select `spec:spec` or `seal:seal` installed from Jaekit, then add your request.

`@Spec` is a plugin mention. A mention alone does not guarantee that Spec's instructions for writing goal documents only are selected. To select them for sure, use the skill invocation above.

> **When the agent calls Spec.** There are two cases: when you ask it to work through Jaekit or goal documents ("do this with jaekit"), and when a review finding can only be resolved by changing a requirement. Spec then writes a `Draft` document with a `Proposed-By: agent` line, or an amendment under `amendments/`, and asks whether you accept it. The agent does not call Spec for an ordinary implementation request or bug report. Whoever called Spec, `Ready` and the start of implementation come only from your words.

<details>
<summary>Skill names and invocation prefixes</summary>

`spec:spec` means **plugin name:skill name**. This prefix is part of the plugin naming scheme, even when Jaekit is the only installation.

Codex uses `$`; Claude Code uses `/`. Short names for separately installed skills and invocation in other agents may differ.

See the [Codex plugin naming rules](https://developers.openai.com/plugins/build/plugins#create-a-plugin-manually), [Codex skill invocation example](https://learn.chatgpt.com/docs/security/plugin/code-changes#automate-reviews-in-cicd), and [Claude Code skill locations and names](https://code.claude.com/docs/en/skills#choose-where-skills-load).

</details>

## Changing a goal or continuing

### Spec saves, then stops

Spec saves the goal, scope and completion conditions directly to `docs/specs/<goal>/SPEC.md`. It reports what it wrote and any decisions it made, with their sources, and stops. Read the report, then answer or ask for changes; Spec revises the same document.

Even if you also ask it to implement the work, it only writes the goal documents. Decisions Spec proposes become yours when you accept them or ask to implement that goal.

If a goal already covers the same behavior and you have not chosen which document to write, Spec first asks whether to create a new goal or edit the existing one.

- `Status: Draft`: an outcome-changing decision remains, and Spec asks about it.
- `Status: Ready`: the document is ready. It does not start implementation by itself.
- `Assurance:`: how much independent review the implementation must get. Use `light` for small fixes and `standard` for ordinary changes. Use `strict` for goals that change a public contract, stored data, permissions, or a boundary between components. Spec proposes a tier and reports why. To use another tier, say so, for example "make it strict".

### Implementation starts on a separate request

When no outcome-changing decision is left, Spec ends its report by asking whether to implement now. It names the goal path and its tier. If you answer yes, implementation starts.

- If your answer also asks for a change ("yes, but drop AC-2"), Spec edits the documents first and asks again.
- The start command still works. You can also say `Implement docs/specs/<goal>` instead.
- You do not approve each step during the work. You answer only decisions that affect the outcome and conditions that need your own confirmation.

If the conversation ends, open a new one in the same project folder and send the following. Seal reads the saved goal, plan, progress and current state, then continues the remaining work.

```text
continue docs/specs/<goal>
```

## Seal's pre-start checks and review order

Seal asks the decisions it can foresee once, before it starts. During work it asks only questions that come up new. Below is when it asks what, and in what order independent review comes for each review tier.

### What Seal asks before the start

- **One budget question.** When the request names no budget, Seal writes `검증 실행: 자동` in PLAN. Within the pre-approved limit it starts without asking. Above it, Seal asks once, showing the computed and pre-approved limits.
  - The number you give goes into PLAN, and the same PLAN never gets the question again.
  - Seal does not write or change `ha.json`.
- **Environment check before the start.** When checks use a browser, database, container or external service, Seal lists it in the PLAN section `## 실행 환경`. It runs `ha doctor` before recording any baseline and, after a resume, before the next check. It gathers unavailable tools into one question. It never widens permissions or settings on its own.
- **Questions before the start, in one message.** From Seal 0.1.23, before recording any baseline, Seal gathers the decisions that need your words and asks them in one message. These are decisions it can foresee from the goal documents, the plan, the project instructions and the code. When there is nothing to ask, Seal asks nothing and starts the work.
  - Examples are a plan that changes a file on a protected path, a change to an existing test that the request does not authorize, a place where the goal documents and the code seem to disagree, unusable tools and a budget that needs approval.
  - Each question gives its options, the recommended one leading, and what the answer changes.
  - When a condition can be met only by your confirmation, the same message names it and what you will be asked to confirm at the end.
  - Test and check files the plan will add are in `바꿀 수 있는 경로` from the start. So adding them raises no scope question later.

### Questions during work, and reports

- **Later questions when they arise.** A question no one could foresee is asked when it arises, such as a `requirement-question` from review or a round cap. Questions pending at the same moment go in one message. The confirmations needed at the end are also asked together in one message, once the results exist.
  - Having asked early does not by itself confirm a change outside Seal's authority or one that weakens a check.
  - Once the change an up-front answer allows is committed, Seal records that answer as the confirmation, quoting it. It does not ask about that change again.
  - When the confirmed content changes later, that is a new change and Seal asks again.
- **Five-line report.** Goals started with Seal 0.1.20 or later use `ha report` in `PROGRESS.md`. `## 현재` opens with the `ha report` stamp, pasted unchanged. Seal does not type the time.
  - In `## 완료 보고`, start with the five lines at the top (`Changed`, `Checked`, `To review`, `Remaining limits`, `Next step`). The completion record and Seal's judgment are below them. In a finished goal started with Seal 0.1.24 or later, the judgment is in `## 검수 요약`.
  - A Korean request gets `--lang ko`.

### Review order by tier

The SPEC's `Assurance:` line sets the independent review stages. Spar decides how a review runs. Seal decides when it runs and what happens with the findings.

| Tier | Reviews |
| --- | --- |
| `light` | Independent review may be skipped; the reason goes in `REVIEW.md` and the completion report |
| `standard` | A candidate round on the final candidate and a candidate round from the goal's base commit to the new head after review fixes; the last candidate round before `ha done` is the cumulative round |
| `strict` | `standard` plus one plan round before any baseline, which covers the product's invariants and decision rules as well as the check plan. When the goal documents or checks change after it, the next candidate round reviews the current goal documents. When SPEC.md or a linked goal document changed, Seal takes one more plan round after a plan cap extension in your words, so that `review_docs_stale` clears |

The **cumulative round** is a candidate round over the whole change, from the goal's base commit to the last head. Of the candidate rounds after review fixes, it is the last one that ends with no open `defect` or `requirement-question`. When no fixes followed the first candidate round, that round is the cumulative round. In a goal under review, Seal runs `ha done` only after that round.

- When project instructions such as `AGENTS.md` ask for more review, Seal follows them and says so in the completion report.
- A goal without the line keeps the selected assurance policy as before.
- When you ask for remote review comments to be applied after completion, Seal records `reopen`, fixes, runs one cumulative round, re-records stale checks and runs `ha done` again.

<details>
<summary>The Core support check from Seal 0.1.20</summary>

Seal 0.1.20 declares the Core support it needs in `core-requirements.json` in its skill folder, and starts with `ha start --check-skill`. When the executable lacks support such as `ha report`, `ha doctor`, the automatic budget or the assurance tier check, `ha` refuses the start without a record. Seal then reports the executable and the message and stops. It does not install tools or fall back to older rules. The `ha` of release v0.1.7 has all of this support. The `ha` of v0.1.6 and earlier does not accept this start.

</details>

## Reading the completion report

Start with what changed and what was verified. The report appears in the conversation and in `PROGRESS.md` in the goal folder.

**Review summary.** In goals started with Seal 0.1.24 or later, after completion, `## 검수 요약` right after `## 현재` is where you start reviewing.

- Top lines: the completion record, the assurance tier, how many required conditions are satisfied, the review gate result, what the change does and where to start looking.
- Below them: what changed, evidence, what to inspect and what was not observed, in that order. A heading with nothing to list says `없음`.
- It is written in the user's language and says what each record is. Each item is short, with seq numbers only as an aid.
- No check output or finding text is copied. A finding is its ID, one line and its ending.
- Seal's judgment is only there. `## 완료 보고` holds the `ha report` output and one line pointing to it.
- After a reopen, the section is rewritten as of the new completion record.

For a finished goal, read `## 검수 요약` first. For a goal that stopped, or one started with an earlier Seal, read `## 완료 보고` as before.

- Required completion conditions need recorded checks, necessary user confirmations or, for a `review` condition, the review gate.
- Optional conditions do not block completion and are identified separately.
- The agent writes the automated checks.

### What to do next

Use the result of `ha status` or `ha done` to find your next step. The numbers in parentheses are exit codes.

> A `complete` from `ha status` alone is not a completion. Until the output names a completion record, Seal records one with `ha done`.

| `ha status` result (exit code) | Who | Next step |
| --- | --- | --- |
| `complete` (0), no completion record yet | Seal | Records completion with `ha done`. |
| `complete` (0), completion record `seq N` | You | Review the result. |
| `incomplete` (1) | Seal | Keeps working. You wait. |
| `needs_user` (2) | You | Answer the question. |
| `blocked` (3) | You | Fix the cause, then send `continue docs/specs/<goal>`. |
| `budget_exhausted` (4) | You | Decide on the budget. |
| error (64 and above) | You | Read the error message, then see [Troubleshooting](#troubleshooting). |

Exit codes 64 and above are errors, not work statuses.

- 64: a usage error
- 65: a refusal, such as a goal that has not started or a broken record
- 66: a rule version the Core does not support
- 70: an internal error

Read the message, then see [Troubleshooting](#troubleshooting).

### Completion evidence and rework

Review the assumptions, changed checks, remaining limitations, and the actual result together. The evidence of completion is the record written by `ha done`.

The assurance level `local` refers to evidence obtained on that computer. An agent with the same user permissions can change the checks and records. So it is not independent third-party verification.

The [operations guide](OPERATIONS.en.md#reading-the-completion-report) gives the detailed review procedure.

If something is missing, say what needs fixing. Seal records the rework request and continues. If the goal document itself needs changing, revise it with Spec and then separately ask to continue.

Seal's default result currently ends at local changes and commits (saved entries in the change history). Push and create pull requests yourself, or ask a general-purpose agent separately. Jaekit does not provide an independent verification environment or unattended operation.

## Saved files

Find the goal and its records in `docs/specs/<goal>/`:

| File | Written by | What it holds |
| --- | --- | --- |
| `SPEC.md` | Spec | Goal, scope, and completion conditions |
| `PLAN.md` | Seal | Plan |
| `REVIEW.md` | Seal | Plan review |
| `PROGRESS.md` | Seal | Progress, review summary (`## 검수 요약`) and completion report |
| `runs.jsonl` | `ha` | Run record |
| `tasks/` files, when needed | Seal | Individual task descriptions |

- **Git tracking:** follow project policy. Commit these files when the project tracks them. Keep them locally when the project treats them as private or ignored. Do not force ignored documents into Git.
- **Before publishing:** a commit saves history locally; publishing it is a separate step. Records can include user statements and working paths. Review them before sending them to a public repository.
- **Jaekit's policy:** this repository's own private `docs/` policy is not a requirement for other projects.
- **Raw output:** check output stays local, in `.git/ha/`, which Git does not track. So you do not need to add anything to `.gitignore`.
- **Linked worktrees:** in a separate working folder sharing the same Git history, output is under `ha/` in the Git directory printed by `git rev-parse --absolute-git-dir`.
- **Read output:** run `ha log <goal> <seq>` in Terminal. Here `<goal>` is the goal path, and `<seq>` is the record number from the report.
- **Run timing:** use `ha timing <goal>` to see which checks took long and how close they came to their time limit. It requires a Core with capability `run-timing/v1`. Measurement details are below.
- **Earlier decision records:** Decision records an earlier release left under `ha/history/` in the common Git directory are not deleted. To read them, use `v0.1.8`, the last release with that feature.

<details>
<summary>Run timing: measurements and limits</summary>

For each check record, a Core that supports the time view keeps:

- The time limit it applied.
- The elapsed time of the limit interval and whether the limit fired.
- The CPU time the operating system reports.

`ha timing <goal>` shows these values as a table, and `--format json` as a `timing/v1` document. The view runs no check and does not change the record.

The limit interval runs from the limit timer's start until the process exit is confirmed or the timeout is decided. Cleanup after that is not included.

The values are measurements only: completion status and the budget do not use them. The sum of run durations adds overlapping runs, so it is not the goal's elapsed time or budget.

CPU covers different processes on each system, so do not compare it across systems as the same quantity:

| System | CPU scope |
| --- | --- |
| macOS and Linux | The process and the descendants it waited for |
| Windows | Every process in its Job Object |

CPU that was not measured shows as unavailable or failed, never as 0. Checks recorded before this feature show as not recorded.

</details>

## Troubleshooting

| Symptom | What to do |
| --- | --- |
| `ha: command not found`, or an unexpected version | Follow [Check the version](INSTALL.en.md#check-the-version) to check the installation and which executable runs |
| A skill is missing, or a short name runs another command | Reopen the agent after installation. Use the full names in the table above; in Codex, select the Jaekit skill from the `$` menu |
| Older `spec` or `seal` installations also appear | Remove the old installations using the steps below, then reopen the agent |
| `@Spec` in Codex behaves differently than expected | Select Jaekit's `spec:spec` from the `$` menu, then send your request |
| `Cannot add marketplace "jaekit"` | The name is already registered. Follow [Move from a local path registration](INSTALL.en.md#move-from-a-local-path-registration) |
| Seal stops and asks you to decide something | Answer the question. Your words are recorded before work continues. Some completion conditions may need your own confirmation |
| An expired login, denied permission, or usage limit stops the work | Resolve the cause, then send `continue docs/specs/<goal>`. The same terminal failure is not retried |
| Verification is refused because of uncommitted changes | Review the listed paths and tracked/untracked or symlink details, then resolve them according to the intended work. Unstarted checks consume no runs |
| The plan budget is too small to start | Read the minimum, limit and shortfall from `ha estimate docs/specs/<goal>`. Decide a sufficient budget or split the goal before starting; this estimate does not change an existing run budget |
| The start stops with `exceeds the pre-approved limit` | Decide whether to approve the computed count. If you do, write it as the plan's `검증 실행` number or raise `preapproved_runs` in `ha.json`. See [Automatic budget](RULES.en.md#automatic-budget) |
| The start stops with a `pre-approved run limit` error | Read the `ha.json` path and reason it shows, then fix the file to `{"preapproved_runs": <whole number>}` or remove it |
| The budget ran out | Read what remains and decide whether to continue; see the budget details below |
| Seal stopped for Core compatibility | Read the executable it checked and the missing support. Follow [Update](INSTALL.en.md#update) to install the v0.1.9 combination, then check the capabilities of the Core actually selected. Do not bypass the check by automatically installing tools or switching rules |

<details>
<summary>Removing older installations</summary>

| Agent | List installations | Remove the old installation |
| --- | --- | --- |
| Codex | `codex plugin list` | `codex plugin remove <plugin>` |
| Claude Code | `claude plugin list` | `claude plugin uninstall <plugin>` |

Replace `<plugin>` with the old installation's ID; keep `spec@jaekit`, `seal@jaekit` and `spar@jaekit`. If you also copied standalone skills manually, remove their installation folders separately, then reopen the agent.

</details>

<details>
<summary>Budget changes and reopening</summary>

- `/3` can change the total run limit with your words and context, retaining usage and the time limit.
- Changing the elapsed-time limit is unsupported.
- Request an explicit reopen when a new window is needed.
- Existing `/1` or `/2` goals retain their reopen behavior with the original limits.

</details>

The [operations guide](OPERATIONS.en.md) describes the detailed reasons for user confirmation, blocked work, and budget stops. The steps above cover what you need to respond and continue.

The `ha report` labels, the run environment check, the review gate, baseline selection, reading a goal's cost, the automatic budget, autonomy, the verification reporting format, and releases and rules are in [Details and rules](RULES.en.md).
