# Details and rules

[한국어](RULES.md) · **English**

A reference for behavior the [usage guide](USAGE.en.md) does not spell out. It describes goals started with v0.1.9 (`run-rules/8`). Fields and judgments are defined in the [run record contract](../contracts/run-record.md).

> You do not need this page to get started. How to hand over a goal and read the report is in the [usage guide](USAGE.en.md).

> **Goals started with an earlier release.** A goal is computed under the rules it started with until it ends. What differs between rules versions is in the [rules versions table of the run record contract](../contracts/run-record.md#46-rules-versions).

## Reports and tool checks

`ha report <goal>` summarizes a goal's state in five lines. It prints English by default and Korean with `--lang ko`. It only reads the same computation as `ha status` and never changes the record.

| Label | What it tells you |
| --- | --- |
| `Changed` | Files and commits changed outside the bundle documents since the start |
| `Checked` | Required conditions satisfied (by check records, by user confirmation and by the review gate), and change conditions whose failure before the change was observed |
| `To review` | Items for a person: decisions for the user, conditions confirmed by a quote, test changes exempted by the executor's reason alone, changed verification commands, review findings that were deferred or left without a disposition. A review finding shows its round, ID, class and deferral target |
| `Remaining limits` | Assurance `local`, the check author, optional conditions not satisfied, an exceeded budget, and whether no completion record covers HEAD |
| `Next step` | The one thing to do in the current state |

When Git cannot compare the base with HEAD, as in a shallow clone without the base commit, it prints only the five lines and exits 70. `Next step` then says how to fetch the base (for example `git fetch --unshallow`).

After the summary come a stamp to copy into `## 현재` in `PROGRESS.md` (the record head's seq and hash, the recorded time and the status), the evidence per condition, the completion coverage, and where the output of unmet conditions is (`ha log <goal> <seq>`). A synthetic example:

```text
Changed: 2 files and 2 commits outside the bundle documents since base 3c1f0a9e7b25
Checked: 2 of 2 required conditions satisfied (2 by check records, 0 by user confirmation); 1 of 1 change conditions require a baseline; 1 of them have a fresh baseline
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

## Review gate

A `standard` or `strict` goal completes only with a recorded Spar gate. `light` goals and goals without an `Assurance:` line need no gate. When the installed `ha` does not know support Seal requires, Seal stops without a record and does not fall back to older rules. What to do is install an `ha` that has that support.

- Once the cumulative round ends with no open finding, Seal makes a gate with `spar gate` from that series' rounds for HEAD, records the gate file with `ha note <goal> review --gate <file>`, and then runs `ha done`. The record keeps the file digest and the summary the judgment needs, never the findings' text.
- Without a gate the status shows `review_missing`; with a commit after the review, `review_head_stale`; when the goal documents changed after the review or a round did not receive them all, `review_docs_stale`. Completion is not recorded. A `blocked` or `degraded` gate, an open `defect` or `requirement-question`, a gate with no code round, and a `strict` goal with no plan round each have their own reason too. User confirmations and other notes do not stand in for a gate.
- The last candidate round (excluding dry runs) must review from the start record's `base_commit`. A mismatch gives `review_base_mismatch` with the gate seq and both bases and blocks completion. A gate with only recorded fixes rounds gives the same reason. Recorded fixes rounds after a candidate from the start base stay valid; Spar checks their chain. `reopen` does not move the start base.
- A `review` note after completion invalidates the completion record of a `standard` or `strict` goal, and the new gate is judged. For a `light` goal or one without a tier line, an otherwise valid completion record stays in place.
- To finish without review when Spar cannot run, you decide a SPEC change that lowers the tier to `Assurance: light`. After `confirm spec` and fresh checks, the goal completes without a gate. A goal with a `review` condition also needs that criterion changed, or it stops at `review_tier_missing`.

Which `done` line counts as the completion record, and how the interval after completion leaves the elapsed time, are in the [run record contract §4.9](../contracts/run-record.md#49-run-rules6).

## Baseline selection

Only change conditions with `요구` in the condition table's optional `baseline` column require a baseline. An empty cell, `—`, or no column means none is required. Seal marks only two kinds: a bug fix or regression whose defect is already present at the base commit, and a prohibition such as "X does not happen". A condition that adds a new feature is not marked by default. REVIEW gives the reason for each marked condition before the plan round; an unmarked condition needs no reason. Unmarked change conditions still need check paths and the structured result contract, but can link to tests of new functions without an entry point that also exists at base.

The required minimum is marked required change × 2 + unmarked required change + required maintain (`표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain`). Manual and review conditions count as 0 runs. One marked change, four unmarked changes and three maintains, all required, need at least 9 runs; the automatic limit is `max(50, ⌈9 × 1.5⌉) = 50`. Before starting, view this with `ha estimate <goal> --rules run-rules/8`. Started goals use their saved rules and refuse a different `--rules`.

With no target, `ha check <goal> --baseline` selects only marked change conditions, including marked optional conditions. With none marked, it refuses without a record. An explicitly named unmarked condition can also run a baseline, and actual runs count toward the budget. Fresh adverse results from that baseline still affect judgment.

Whichever condition it records, a baseline copies the check paths of every marked change condition from HEAD onto the base commit and runs there. A commit that changes a file in those paths leaves every earlier baseline stale. Seal records a marked condition's baseline once, after the last review round and right before the completion record. If that baseline passes or errs instead of showing the declared violation, Seal fixes the check, has the fixed check reviewed like fixed code, and records the baseline again. This reduces the habit of recording after every check fix; it does not make stale evidence fresh.

The "before any baseline" steps (gathering questions, `ha doctor` and the `strict` plan round) all happen before the goal has any `ha check` record, current check or baseline, even when the goal needs no baseline. Seal gathers foreseeable questions before work; with nothing to ask, it proceeds.

- An input failure—a missing target file, corrupt input, a tool that cannot start, or termination by a signal—is never a pass.
- Checks that require no baseline (maintain, `EX-n`, task commands and unmarked change conditions) may count an input failure as a violation, error or inconclusive result; that alone is not a check defect. A failure recorded as a violation remains until the input changes.
- A dedicated producer of a marked change condition reports none of them as a violation unless the result contract allows it, such as `missing: violation`; record that choice in `REVIEW.md` before the plan round, or before any review round when there is no plan round.

Runner adapters classify input failures themselves; the classifications and input-failure table of the [result contract](../contracts/check-result.md) stay unchanged.

`ha status` shows `baseline_required` on each change condition and a human-readable `baseline required` line. `Checked` in `ha report` separates how many of all change conditions require a baseline from how many of those have fresh baselines. Removing a mark after the start removes the requirement, but `To review` shows it. The review summary's `### 보지 않은 것` lists change conditions satisfied without a baseline. The exact fields are in [run record §4.10](../contracts/run-record.md#410-run-rules7) and [§4.11](../contracts/run-record.md#411-run-rules8).

## Reading one goal's cost

`ha cost <goal>` gathers what one goal cost in verification and review on one screen. It only reads records and writes nothing; it never reads the current time and calls no model. It reads Spar review records only from the series folders you name with `--series`. It does not guess where they are, so give `--series` once per series.

```bash
ha cost docs/specs/<goal> --series docs/reviews/<goal>
```

- **Kind and size:** the tier, the rules version, the required and optional criteria, the files and commits changed since the base (the same as `Changed` in `ha report`), and a valid completion record.
- **Verification:** runs and their time, runs per condition, and for each re-recorded run the inputs that differ from the run before it (code, goal documents, verification command, overlaid check files). When no input differs, it shows the earlier run's result. The budget window shows the values `ha status` shows.
- **Questions and waits:** each reported wait with the decision it asked and its length, Spar's `requirement-question` findings with their answers, cap extensions and reviewer waivers. They are counted by source and listed. Whether one was a scope or authority decision or a needless re-check is for the person reading the list to judge.
- **Review rounds and findings:** rounds by stage (counted rounds and reruns), reviewer time, findings by class and location with their dispositions, and the lines recorded fixes rounds added (the gate's `growth`). Without `--series`, these parts say they were not given and show no zeros.
- **Tokens:** no record holds them, so they show as "not recorded".

`--format json` shows the same values as a `cost/v1` document, and `--lang ko` changes only labels and sentences. The format and the source of each value are in [the run record contract](../contracts/run-record.md) §1.7.

**When you compare.** The summary shows numbers only; it never compares the goal with another or says it improved. To compare goals, put goals of the same tier and similar size (criteria, changed files and commits) side by side. A difference between a few goals before and after a process change mixes in what each goal was about and chance, so it is not a causal effect of that change.

## Automatic budget

If the plan's `## 예산` gives the run count as `- 검증 실행: 자동` (automatic) instead of a number, `ha start` sets the limit. The limit is `max(50, ⌈required minimum × 1.5⌉)`. A goal whose required minimum is 68 runs gets a limit of 102. With a number, that number is the limit. `경과 시간` (elapsed time) is optional and informational; going over it never exceeds the budget.

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

If you approve that count, write it as a number in the plan or raise `preapproved_runs` in `ha.json`. A malformed or unreadable file stops with `ha: pre-approved run limit: <path>: <reason>`; it is never replaced by 50. The limit applies only to the start. After the start, every increase of the run limit needs your words and the context of the request. The exact rules are in the [run record contract](../contracts/run-record.md#15-automatic-budget).

## Autonomy

Seal does not ask again about work the authority it already has covers.

- When your request allows test changes inside the scope, Seal records its reading with `--test-authority`. When it changes or moves an existing test, it records which conditions the file checks and how with `ha note <goal> test-change`, and the change needs no confirmation. A weakening change or one outside the scope asks for your confirmation.
- While it waits for your answer to a question, it reports the wait with `ha note <goal> wait`, and records `resume` if it continues without the answer. Reported waits leave the elapsed time.
- When a check passed its error limit because of something Seal can fix in how it runs, such as a missing tool, Seal records `recover`. Each condition and input allows two; `allow-recovery` lets you allow more. Earlier failures stay in the record.
- A long check gets its own limit, up to two hours, in the plan's `## 시간 제한` section. When an earlier run reached or neared its limit, a notice appears before the run.

## Release and rules

The current release v0.1.9 combines ha 0.1.9, spar 0.1.9, spec 0.1.16, seal 0.1.36 and the spar plugin 0.1.9. Seal starts new goals under `run-rules/8`. A direct `ha start` defaults to `run-rules/3`. See [Release and development combinations](INSTALL.en.md#release-and-development-combinations).

Rules (`run-rules/N`) are the version of how `ha` computes a goal's records. A goal is computed under the rules it started with until it ends, so a new `ha` does not change the meaning of a goal in progress.
