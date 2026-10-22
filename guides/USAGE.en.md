# Usage guide

[한국어](USAGE.md) · **English**

This guide follows [your first task in the README](../README.md#your-first-task). For installation, updates, and removal, see the [install guide](INSTALL.en.md).

The current release v0.1.6 combines ha 0.1.6, spec 0.1.12, seal 0.1.17 and default rules `/3`. Starting with Seal 0.1.16, Seal starts new goals under `run-rules/4`. Preflight refusal, run estimation, `/3` result classification and budget policy apply to this release. Existing goals keep their saved rules. See [Release and development combinations](INSTALL.en.md#release-and-development-combinations).

## Sending commands

Send these **in the agent conversation**, not Terminal. Replace `<request>` with the work you want and `<goal>` with the goal folder name Spec reports.

| What to do | Codex | Claude Code |
| --- | --- | --- |
| Write the goal | `$spec:spec <request>` | `/spec:spec <request>` |
| Start implementation | `$seal:seal docs/specs/<goal>` | `/seal:seal docs/specs/<goal>` |

In Codex, type `$`, select `spec:spec` or `seal:seal` installed from Jaekit, then add your request.

`@Spec` is a plugin mention. Use the skill invocation above to explicitly select Spec's instructions for writing goal documents only; a plugin mention alone does not guarantee the same instructions are selected.

The host agent can also call Spec: when you ask it to work through Jaekit or goal documents ("do this with jaekit"), or when a review finding can only be resolved by changing a requirement. Spec then writes a `Draft` document with a `Proposed-By: agent` line, or an amendment document under `amendments/`, and asks whether you accept it. The agent does not call Spec for an ordinary implementation request or bug report. Whoever called Spec, `Ready` and the start of implementation come only from your words.

<details>
<summary>Skill names and invocation prefixes</summary>

`spec:spec` means **plugin name:skill name**. This prefix is part of the plugin naming scheme, even when Jaekit is the only installation.

Codex uses `$`; Claude Code uses `/`. Short names for separately installed skills and invocation in other agents may differ.

See the [Codex plugin naming rules](https://developers.openai.com/plugins/build/plugins#create-a-plugin-manually), [Codex skill invocation example](https://learn.chatgpt.com/docs/security/plugin/code-changes#automate-reviews-in-cicd), and [Claude Code skill locations and names](https://code.claude.com/docs/en/skills#choose-where-skills-load).

</details>

## Changing a goal or continuing

Read Spec's report, then answer or ask for changes to revise the same document. Spec saves the goal, scope, and completion conditions directly to `docs/specs/<goal>/SPEC.md`, reports what it wrote and any decisions it made with their sources, then stops.

Even if you also ask it to implement the work, it only writes the goal documents. Decisions Spec proposes become yours when you accept them or ask to implement that goal.

If a goal already covers the same behavior and you have not chosen which document to write, Spec first asks whether to create a new goal or edit the existing one.

- `Status: Draft`: an outcome-changing decision remains, and Spec asks about it.
- `Status: Ready`: the document is ready. It does not start implementation by itself.
- `Assurance:`: how much independent review the implementer must get. Use `light` for small fixes, `standard` for ordinary changes, and `strict` for goals that change a public contract, stored data, permissions, or a boundary between components. Spec proposes a tier and reports why. To use another tier, say so, for example "make it strict".

When no outcome-changing decision is left, Spec ends its report with a separate question: it asks whether to implement now, naming the goal path and its tier. If you answer yes, implementation starts. If your answer also asks for a change ("yes, but drop AC-2"), Spec edits the documents first and asks again. The start command still works, and you can also tell Seal `Implement docs/specs/<goal>` instead of using the start command. You do not approve each step during the work. Answer decisions that affect the outcome and conditions that need your own confirmation.

If the conversation ends, open a new one in the same project folder and send the following. Seal reads the saved goal, plan, progress, and current state, then continues the remaining work.

```text
continue docs/specs/<goal>
```

## Reading the completion report

Start with what changed and what was verified. The report appears in the conversation and in `PROGRESS.md` in the goal folder.

- Required completion conditions need recorded checks or necessary user confirmations.
- Optional conditions do not block completion and are identified separately.
- The agent writes the automated checks.

### What to do next

Use the `ha status` / `ha done` result to find your next step. The numbers are exit codes. A `complete` from `ha status` alone is not a completion: until the output names a completion record, Seal records one with `ha done`.

```mermaid
flowchart LR
    S["ha status / ha done"] -->|complete · 0| R{"Completion record?"}
    R -->|"none yet"| D["Seal runs ha done"]
    R -->|"seq N"| C["Review the result"]
    S -->|incomplete · 1| I["Let Seal keep working"]
    S -->|needs_user · 2| U["Answer the question"]
    S -->|blocked · 3| B["Fix the cause, then send<br/>continue docs/specs/#lt;goal#gt;"]
    S -->|budget_exhausted · 4| E["Decide on the budget"]
    S -->|"error · 64+"| X["Read the error message,<br/>then see Troubleshooting"]
    classDef you fill:#EEF1FE,stroke:#4F6CF7,color:#1B2559
    classDef sealStep fill:#FFF3E3,stroke:#C9761F,color:#4A2A06
    classDef record fill:#F1F3F5,stroke:#596579,color:#1F2328
    class S record
    class C,U,B,E,R,X you
    class I,D sealStep
```

Exit codes 64 and above are errors, not work statuses: 64 for a usage error, 65 for a refusal such as a goal that has not started or a broken record, 66 for a rule version the Core does not support, and 70 for an internal error. Read the message, then see [Troubleshooting](#troubleshooting).

### Completion evidence and rework

Review the assumptions, changed checks, remaining limitations, and the actual result together. The evidence of completion is the record written by `ha done`.

The assurance level `local` refers to evidence obtained on that computer. An agent with the same user permissions can change the checks and records, so it is not independent third-party verification.

The [operations guide](OPERATIONS.en.md#reading-the-completion-report) gives the detailed review procedure.

If something is missing, say what needs fixing. Seal records the rework request and continues. If the goal document itself needs changing, revise it with Spec and then separately ask to continue.

Seal's default result currently ends at local changes and commits (saved entries in the change history). You can push and create pull requests yourself or ask a general-purpose agent separately. Jaekit does not provide an independent verification environment or unattended operation.

## Saved files

Find the goal and its records in `docs/specs/<goal>/`:

| File | Written by | What it holds |
| --- | --- | --- |
| `SPEC.md` | Spec | Goal, scope, and completion conditions |
| `PLAN.md` | Seal | Plan |
| `REVIEW.md` | Seal | Plan review |
| `PROGRESS.md` | Seal | Progress and completion report |
| `runs.jsonl` | `ha` | Run record |
| `tasks/` files, when needed | Seal | Individual task descriptions |

- **Git tracking:** follow project policy. Commit these files when the project tracks them; keep them locally when the project treats them as private or ignored. Do not force ignored documents into Git.
- **Before publishing:** a commit saves history locally; publishing it is a separate step. Records can include user statements and working paths, so review them before sending them to a public repository.
- **Jaekit's policy:** this repository's own private `docs/` policy is not a requirement for other projects.
- **Raw output:** check output stays local in `.git/ha/`, which Git does not track. You do not need to add anything to `.gitignore`.
- **Linked worktrees:** in a separate working folder sharing the same Git history, output is under `ha/` in the Git directory printed by `git rev-parse --absolute-git-dir`.
- **Read output:** run `ha log <goal> <seq>` in Terminal. Here `<goal>` is the goal path, and `<seq>` is the record number from the report.
- **Run timing:** use `ha timing <goal>` to see which checks took long and how close they came to their time limit. It requires a Core with capability `run-timing/v1`; measurement details are below.
- **Decision history:** with capability `decision-history/v1`, Seal can keep summaries, evidence and correction links in the optional [decision history](../contracts/decision-history.md). Use `ha history <goal>` for current judgments, or `ha history <goal> show <event ID>` for one event's evidence and neighboring links.

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

<details>
<summary>Decision history: availability, storage, and limits</summary>

The history covers observations, causes, decisions, actions and verifications. Release v0.1.5 (`ha 0.1.5`) first provides it; v0.1.4 and earlier do not.

The history stays local under `ha/history/` in the common Git directory that every working folder of the repository shares. It does not travel to a new clone or another computer.

So `PROGRESS.md` still holds the current state, important decisions with short reasons, and how to resume without it. Recording or correcting history changes no completion, check result or budget, and the feature adds no approval step.

</details>

## Autonomy (run-rules/4)

Starting with Seal 0.1.16, Seal starts new goals with `ha start --rules run-rules/4` when the Core supports `run-rules/4` and its five features. The Core's default rule stays `run-rules/3`, and a goal that has already started keeps the rule it started with. Under the new rule, Seal does not ask again about work the authority it already has covers.

- When your request allows test changes inside the scope, Seal records its reading with `--test-authority`. When it changes or moves an existing test, it records which conditions the file checks and how with `ha note <goal> test-change`, and the change needs no confirmation. A weakening change or one outside the scope still asks for your confirmation.
- While it waits for your answer to a question, it reports the wait with `ha note <goal> wait`, and records `resume` if it continues without the answer. Only reported waits leave the budget's elapsed time.
- A run limit you approve in advance is recorded with `ha budget <goal> --preapprove`; within it, Seal raises the limit with a reason.
- When a check passed its error limit because of something Seal can fix in how it runs, such as a missing tool, Seal records `recover`. Each condition and input allows two; `allow-recovery` lets you allow more. Earlier failures stay in the record.
- A long check gets its own limit, up to two hours, in the plan's `## 시간 제한` section. When an earlier run reached or neared its limit, a notice appears before the run.

## Reports and tool checks

`ha report <goal>` summarizes a goal's state in five lines. It prints English by default and Korean with `--lang ko`. It only reads the same computation as `ha status` and never changes the record.

| Label | What it tells you |
| --- | --- |
| `Changed` | Files and commits changed outside the bundle documents since the start |
| `Checked` | Required conditions satisfied (by check records and by user confirmation), and change conditions whose failure before the change was observed |
| `To review` | Items for a person: decisions for the user, conditions confirmed by a quote, test changes exempted by the executor's reason alone, changed verification commands |
| `Remaining limits` | Assurance `local`, the check author, optional conditions not satisfied, an exceeded budget, and whether no completion record covers HEAD |
| `Next step` | The one thing to do in the current state |

When Git cannot compare the base with HEAD, as in a shallow clone without the base commit, it prints only the five lines and exits 70. `Next step` then says how to fetch the base (for example `git fetch --unshallow`).

After the summary come a stamp to copy into `## 현재` in `PROGRESS.md` (the record head's seq and hash, the recorded time and the status), the evidence per condition, the completion coverage, and where the output of unmet conditions is (`ha log <goal> <seq>`). A synthetic example:

```text
Changed: 2 files and 2 commits outside the bundle documents since base 3c1f0a9e7b25
Checked: 2 of 2 required conditions satisfied (2 by check records, 0 by user confirmation); 1 of 1 change conditions have a fresh baseline
To review: none
Remaining limits: assurance local; check author: executor
Next step: none; completion record seq 5 holds and nothing is left to do
```

To check the tools your checks need beforehand, add a `## 실행 환경` section to PLAN with one tool per line in the form `` - <이름>: `<점검 명령>` `` (a tool name, a colon, and its check command in one code span).

```markdown
## 실행 환경
- 브라우저: `node scripts/probe-browser.mjs`
- 셸: `sh tests/env/plain.sh`
```

`ha doctor <goal>` runs these check commands in order with a 60-second limit each and prints `사용 가능` (available) or `사용 불가 (<reason>)` (unavailable) for each tool. For an unavailable tool, the end of its output follows. It works before the start too, and leaves nothing in the record or the run count. A malformed line is reported with its file and line, and then no check runs. Judgments never read this section.

## Automatic budget

If the plan's `## 예산` gives the run count as `- 검증 실행: 자동` (automatic) instead of a number, `ha start` sets the limit. The limit is `max(50, ⌈required minimum × 1.5⌉)`: a goal with 34 required change conditions needs at least 68 runs and gets a limit of 102. You still write `경과 시간` (elapsed time) yourself. A plan with a number works as before.

When the computed limit is at or below the limit you approved in advance, the goal starts without a question. Write that pre-approved limit once in `ha.json` in your user configuration folder: `~/.config/jaekit/`, or `$XDG_CONFIG_HOME/jaekit/` when `XDG_CONFIG_HOME` is set. Without the file, the limit is 50 runs. No file inside a repository can set this limit. To check that, ha lists the working trees with `git worktree list -z` when the file exists, which needs Git 2.36 or later.

```bash
folder="${XDG_CONFIG_HOME:-$HOME/.config}/jaekit"
mkdir -p "$folder"
printf '{"preapproved_runs": 120}\n' > "$folder/ha.json"
```

The home is the `HOME` environment variable when it is set, otherwise `USERPROFILE` on Windows. In Windows PowerShell, pick the folder in the same order:

```powershell
$folder = if ($env:XDG_CONFIG_HOME) { "$env:XDG_CONFIG_HOME\jaekit" } elseif ($env:HOME) { "$env:HOME\.config\jaekit" } else { "$env:USERPROFILE\.config\jaekit" }
New-Item -ItemType Directory -Force $folder | Out-Null
Set-Content -Path "$folder\ha.json" -Value '{"preapproved_runs": 120}'
```

`ha estimate docs/specs/example` shows the same computed limit before the start, and whether your approval is needed. When the computed limit is above your limit, `ha start` records nothing and stops:

```text
ha: auto budget 102 runs (minimum 68 × 1.5, at least 50) exceeds the pre-approved limit 50 from default; the user's approval is needed: write the approved number in PLAN `검증 실행`, or have the user raise preapproved_runs in ~/.config/jaekit/ha.json
```

If you approve that count, write it as a number in the plan or raise `preapproved_runs` in `ha.json`. A malformed or unreadable file stops with `ha: pre-approved run limit: <path>: <reason>`; it is never replaced by 50. The limit applies only to the start; raising the limit and reopening after the start work as before. The exact rules are in the [run record contract](../contracts/run-record.md#15-자동-예산) (Korean).

## Seal's pre-start checks and review order

Seal 0.1.20 declares the Core support it needs in `core-requirements.json` in its skill folder and starts with `ha start --check-skill`. When the executable lacks support such as `ha report`, `ha doctor`, the automatic budget or the assurance tier check, `ha` refuses the start without a record, and Seal reports the executable and the message and stops. It does not install tools or fall back to older rules. The `ha` of release v0.1.6 does not accept this start, so this combination needs an `ha` that includes those commands.

- **One budget question.** When the request names no budget, Seal writes `검증 실행: 자동` in PLAN. Within the pre-approved limit it starts without asking; above it, Seal asks once, showing the computed and pre-approved limits. The number you give goes into PLAN, and the same PLAN never gets the question again. Seal does not write or change `ha.json`.
- **Environment check before the start.** When checks use a browser, database, container or external service, Seal lists it in the PLAN section `## 실행 환경` and runs `ha doctor` before recording any baseline and, after a resume, before the next check. It gathers unavailable tools into one question and never widens permissions or settings on its own.
- **Five-line report.** Goals started with 0.1.20 or later use `ha report` in `PROGRESS.md`. `## 현재` opens with the `ha report` stamp, pasted unchanged; Seal does not type the time. In `## 완료 보고`, read the five lines at the top (`Changed`, `Checked`, `To review`, `Remaining limits`, `Next step`), then the completion record and Seal's judgment below them. A Korean request gets `--lang ko`.

The SPEC's `Assurance:` line sets the independent review stages. Spar decides how a review runs; Seal decides when and what happens with the findings.

| Tier | Reviews |
| --- | --- |
| `light` | Independent review may be skipped; the reason goes in `REVIEW.md` and the completion report |
| `standard` | A candidate round on the final candidate, a fixes round after review fixes, and a cumulative round before `ha done` |
| `strict` | `standard` plus a plan round before any baseline, which covers the product's invariants and decision rules as well as the check plan |

The cumulative round is a candidate round over the whole change, from the goal's base commit to the last head. In a goal under review, Seal runs `ha done` only once that round ends with no open `defect` or `requirement-question`. When project instructions such as `AGENTS.md` ask for more review, Seal follows them and says so in the completion report. A goal without the line keeps the selected assurance policy as before. When you ask for remote review comments to be applied after completion, Seal records `reopen`, fixes, runs one cumulative round, re-records stale checks and runs `ha done` again.

The Seal skill body went from about 4,600 words to at most 2,200 words. The formats of goals started with an earlier Seal moved to a reference read only when continuing such a goal (`references/earlier-goals.md`); the records and documents of those goals do not change.

## Review gate (run-rules/5)

Starting with Seal 0.1.21, Seal starts new goals with `ha start --rules run-rules/5` when the Core supports `run-rules/5` and the review gate. `run-rules/5` keeps all of `run-rules/4`'s autonomy and adds a recorded independent review as a completion condition for goals whose SPEC tier is `standard` or `strict`.

- Once the cumulative round ends with no open finding, Seal makes a gate with `spar gate` from that series' rounds for HEAD, records the gate file with `ha note <goal> review --gate <file>`, and then runs `ha done`. The record keeps the file digest and the summary the judgment needs, never the findings' text.
- Without a gate the status shows `review_missing`; with a commit after the review, `review_head_stale`; when the goal documents changed after the review or a round did not receive them all, `review_docs_stale`. Completion is not recorded. A `blocked` or `degraded` gate, an open `defect` or `requirement-question`, a gate with no code round, and a `strict` goal with no plan round each have their own reason too. User confirmations and other notes do not stand in for a gate.
- To finish without review when Spar cannot run, you decide a SPEC change that lowers the tier to `Assurance: light`. After `confirm spec` and fresh checks, the goal completes without a gate.
- A `light` goal, a goal without the tier line, and goals started under `run-rules/1` to `run-rules/4` are judged without a gate, as before.

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
| The start stops with `exceeds the pre-approved limit` | Decide whether to approve the computed count. If you do, write it as the plan's `검증 실행` number or raise `preapproved_runs` in `ha.json`. See [Automatic budget](#automatic-budget) |
| The start stops with a `pre-approved run limit` error | Read the `ha.json` path and reason it shows, then fix the file to `{"preapproved_runs": <whole number>}` or remove it |
| The budget ran out | Read what remains and decide whether to continue; see the budget details below |
| Seal stopped for Core compatibility | Read the executable it checked and the missing support. Follow [Update](INSTALL.en.md#update) to install the v0.1.6 combination, then check the capabilities of the Core actually selected. Do not bypass the check by automatically installing tools or switching rules |

<details>
<summary>Removing older installations</summary>

| Agent | List installations | Remove the old installation |
| --- | --- | --- |
| Codex | `codex plugin list` | `codex plugin remove <plugin>` |
| Claude Code | `claude plugin list` | `claude plugin uninstall <plugin>` |

Replace `<plugin>` with the old installation's ID; keep `spec@jaekit` and `seal@jaekit`. If you also copied standalone skills manually, remove their installation folders separately, then reopen the agent.

</details>

<details>
<summary>Budget changes and reopening</summary>

- `/3` can change the total run limit with your words and context, retaining usage and the time limit.
- Changing the elapsed-time limit is unsupported.
- Request an explicit reopen when a new window is needed.
- Existing `/1` or `/2` goals retain their reopen behavior with the original limits.

</details>

The [operations guide](OPERATIONS.en.md) describes the detailed reasons for user confirmation, blocked work, and budget stops. The steps above cover what you need to respond and continue.

## Verification reporting format

The reporting format introduced in Seal 0.1.10 separates these four things in the existing review and report:

- Expected-result grounds.
- Observation boundaries.
- Methods and substitutes.
- Unobserved scope.

Condition groups may share an explanation; a plan is not an observation. The current release v0.1.6 includes this format in Seal 0.1.17; existing goal formats stay unchanged.

See the [synthetic reporting example](../examples/verification-reporting.md); it is not an actual execution record.
