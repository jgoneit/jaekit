# Details and rules

[한국어](RULES.md) · **English**

A reference for behavior the [usage guide](USAGE.en.md) does not spell out. Fields and judgments are defined in the [run record contract](../contracts/run-record.md).

> You do not need this page to get started. How to hand over a goal and read the report is in the [usage guide](USAGE.en.md).

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

## Review gate (run-rules/5)

From v0.1.7, Seal (0.1.21 or later) started new goals with `ha start --rules run-rules/5`; Seal 0.1.36 in v0.1.9 starts them under `run-rules/8`. `run-rules/5` keeps all of `run-rules/4`'s autonomy and adds a recorded independent review as a completion condition for goals whose SPEC tier is `standard` or `strict`. So a `standard` or `strict` goal started under `run-rules/5` or later completes only with a recorded Spar gate. `light` goals, goals without an `Assurance:` line and goals started under `run-rules/4` or earlier do not need Spar. To finish without review, you decide a goal-document change that lowers the tier to `light` (see the list below).

When the installed `ha` does not know `run-rules/5` or other support Seal requires, Seal stops without a record and does not fall back to older rules. What to do is install an `ha` that has that support.

- Once the cumulative round ends with no open finding, Seal makes a gate with `spar gate` from that series' rounds for HEAD, records the gate file with `ha note <goal> review --gate <file>`, and then runs `ha done`. The record keeps the file digest and the summary the judgment needs, never the findings' text.
- Without a gate the status shows `review_missing`; with a commit after the review, `review_head_stale`; when the goal documents changed after the review or a round did not receive them all, `review_docs_stale`. Completion is not recorded. A `blocked` or `degraded` gate, an open `defect` or `requirement-question`, a gate with no code round, and a `strict` goal with no plan round each have their own reason too. User confirmations and other notes do not stand in for a gate.
- To finish without review when Spar cannot run, you decide a SPEC change that lowers the tier to `Assurance: light`. After `confirm spec` and fresh checks, the goal completes without a gate. A `run-rules/8` goal with a `review` condition also needs that criterion changed, or it stops at `review_tier_missing`.
- A `light` goal, a goal without the tier line, and goals started under `run-rules/1` to `run-rules/4` are judged without a gate, as before.

Seal 0.1.26 starts new goals with `ha start --rules run-rules/6 … --check-skill` instead of the start rule above. This needs a Core that supports `run-rules/6`; ha 0.1.8 does not. Without support, Seal stops without a record and does not fall back to earlier rules. `/6` changes only these four judgments from `/5`.

- The last candidate round (excluding dry runs) for a `standard` or `strict` goal must review from the start record's `base_commit`. A mismatch gives `review_base_mismatch` with the gate seq and both bases and blocks completion. A gate with only recorded fixes rounds gives the same reason. Recorded fixes rounds after a candidate from the start base stay valid; Spar checks their chain. `reopen` does not move the start base.
- For a `light` goal or one without a tier line, a `review` note after completion leaves an otherwise valid completion record in place. For `standard` and `strict`, the record is invalidated and the new gate is judged, as before.
- A `done` line is judged when its `record_head` is the hash of an earlier line and every line strictly between that line and the `done` line is a finding line (`run-finding/v1`), including when there are no intervening lines. The named line may itself be a finding line. Every `done` line is judged on the full record first, so a line may be judged even when its `record_head` names an unjudged line. Unjudged complete and non-complete `done` lines are then treated as absent from status computation, including the completion record and its explanation, the latest done line, the budget window, elapsed time, post-completion intervals and budget-change refusal. Status, report, cost and budget changes use this same view. An earlier record still needs every other validity condition; a judged line invalid for another reason still stops the search.
- Budget elapsed time excludes the interval from a completion `done` to the next check or baseline start or a non-complete `done`. With neither endpoint, it ends at the window's last budget record; notes and findings in between do not end it. `post_completion_seconds` shows the additional excluded seconds that do not overlap reported waits or run intervals; status, report and cost use the same elapsed time. Run counts and limits stay unchanged; code rework after completion opens a new window with `reopen`.

Goals started under `run-rules/1` through `run-rules/5` keep their existing judgments and are not migrated automatically. The exact interval and output definitions are in the [run record contract](../contracts/run-record.md#49-run-rules6).

## Baseline selection and check freeze (run-rules/7)

Current Seal starts new goals under `run-rules/8`, which keeps the baseline selection below but has no check freeze ([run record contract](../contracts/run-record.md#411-run-rules8)). It stops without a record on a Core that lacks support and does not fall back to earlier rules. A direct `ha start` still defaults to `run-rules/3`; judgments, output and records of goals already started under `/1` through `/7` stay unchanged.

Only change conditions with `요구` in the condition table's optional `baseline` column require a baseline. An empty cell, `—`, or no column means none is required. Seal marks only two kinds: a bug fix or regression whose defect is already present at the base commit, and a prohibition such as "X does not happen". A condition that adds a new feature is not marked by default. REVIEW gives the reason for each marked condition before the plan round; an unmarked condition needs no reason. A goal already started keeps the marks it started with. Unmarked change conditions still need check paths and the structured result contract, but can link to tests of new functions without an entry point that also exists at base.

The required minimum is marked required change × 2 + unmarked required change + required maintain (`표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain`). Manual and review conditions count as 0 runs. One marked change, four unmarked changes and three maintains, all required, need at least 9 runs; the automatic limit is `max(50, ⌈9 × 1.5⌉) = 50`. Before starting, view this with `ha estimate <goal> --rules run-rules/8`. Started goals use their saved rules and refuse a different `--rules`.

With no target, `ha check <goal> --baseline` selects only marked change conditions, including marked optional conditions. With none marked, it refuses without a record. An explicitly named unmarked condition can also run a baseline, and actual runs count toward the budget. Fresh adverse results from that baseline still affect judgment. Seal records a marked condition's baseline once, after the last review round and right before the completion record. If that baseline passes or errs instead of showing the declared violation, Seal fixes the check, has the fixed check reviewed like fixed code, and records the baseline again. This reduces the habit of recording after every check fix; it does not make stale evidence fresh.

Under `/7` and `/8`, the "before any baseline" questions, `ha doctor` and the `strict` plan round all happen before the goal has any `ha check` record, current check or baseline, even when the goal needs no baseline. Seal gathers foreseeable questions before work; with nothing to ask, it proceeds.

- An input failure—a missing target file, corrupt input, a tool that cannot start, or termination by a signal—is never a pass.
- Checks that require no baseline (maintain, `EX-n`, task commands and unmarked change conditions) may count an input failure as a violation, error or inconclusive result; that alone is not a check defect. A failure recorded as a violation remains until the input changes.
- A dedicated producer of a marked change condition reports none of them as a violation unless the result contract allows it, such as `missing: violation`; record that choice in `REVIEW.md` before the plan round, or before any review round when there is no plan round.

Read the baseline requirement under the goal's rules: all change conditions require it up to `run-rules/6`, and only marked conditions do under `/7` and `/8`. Runner adapters classify input failures themselves; the classifications and input-failure table of the [result contract](../contracts/check-result.md) stay unchanged.

Under `run-rules/7`, the check freeze uses the earliest `plan` round for `strict` or the earliest `candidate` round for `standard`, in gate order among the last recorded gate's non-dry-run rounds. It covers declarations, producers, tests, helpers and settings in the condition table's check paths, from every row that lists any. Right after the freeze round, Seal records the gate and current condition table's check paths with `ha note <goal> review --gate <file>`. Without a gate note ending at that round, removed check paths are unobserved and status and report show `freeze-time condition table was not recorded`.

After changing a check file following the freeze, commit it and record `ha note <goal> check-change checks:<path> --reason "<finding ID, or condition and reason>"`. A review fix names its finding ID. The reason binds to the current blob, so another edit needs a reason for that edit. Give the next candidate round the changed checks and reasons together, and that round reviews the changed checks. No such reason is needed before the freeze; check defects may still be fixed afterward. The freeze display itself does not block completion.

`ha status` shows `baseline_required` on each change condition and a human-readable `baseline required` line. `Checked` in `ha report` separates how many of all change conditions require a baseline from how many of those have fresh baselines. Read `To review` for baseline marks removed since the start, check files changed after the freeze with reasons, and changes without reasons. Removed paths identify the source `review` note seq. `light` goals and goals without a recorded gate have no freeze display. The review summary's `### 보지 않은 것` lists change conditions satisfied without a baseline. [Run record §4.10](../contracts/run-record.md#410-run-rules7) defines exact fields and wording.

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

If the plan's `## 예산` gives the run count as `- 검증 실행: 자동` (automatic) instead of a number, `ha start` sets the limit. The limit is `max(50, ⌈required minimum × 1.5⌉)`: up to `run-rules/6`, a goal with 34 required change conditions needs at least 68 runs and gets a limit of 102. You still write `경과 시간` (elapsed time) yourself; under `run-rules/8` it is optional and informational. A plan with a number works as before.

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

If you approve that count, write it as a number in the plan or raise `preapproved_runs` in `ha.json`. A malformed or unreadable file stops with `ha: pre-approved run limit: <path>: <reason>`; it is never replaced by 50. The limit applies only to the start; raising the limit and reopening after the start work as before. The exact rules are in the [run record contract](../contracts/run-record.md#15-automatic-budget).

## Autonomy (run-rules/4)

Starting with Seal 0.1.16, Seal starts new goals with `ha start --rules run-rules/4` when the Core supports `run-rules/4` and its five features. The Core's default rule stays `run-rules/3`, and a goal that has already started keeps the rule it started with. Under the new rule, Seal does not ask again about work the authority it already has covers.

- When your request allows test changes inside the scope, Seal records its reading with `--test-authority`. When it changes or moves an existing test, it records which conditions the file checks and how with `ha note <goal> test-change`, and the change needs no confirmation. A weakening change or one outside the scope still asks for your confirmation.
- While it waits for your answer to a question, it reports the wait with `ha note <goal> wait`, and records `resume` if it continues without the answer. Only reported waits leave the budget's elapsed time.
- Up to `run-rules/7`, a run limit you approve in advance is recorded with `ha budget <goal> --preapprove`; within it, Seal raises the limit with a reason.
- When a check passed its error limit because of something Seal can fix in how it runs, such as a missing tool, Seal records `recover`. Each condition and input allows two; `allow-recovery` lets you allow more. Earlier failures stay in the record.
- A long check gets its own limit, up to two hours, in the plan's `## 시간 제한` section. When an earlier run reached or neared its limit, a notice appears before the run.

## Verification reporting format

The reporting format introduced in Seal 0.1.10 separates these four things in the existing review and report:

- Expected-result grounds.
- Observation boundaries.
- Methods and substitutes.
- Unobserved scope.

Condition groups may share an explanation; a plan is not an observation. The current release v0.1.9 includes this format in Seal 0.1.36; existing goal formats stay unchanged.

See the [synthetic reporting example](../examples/verification-reporting.md); it is not an actual execution record.

## Release and rules

The current release v0.1.9 combines ha 0.1.9, spar 0.1.9, spec 0.1.16, seal 0.1.36 and the spar plugin 0.1.9. The default rules are `/3`; Seal starts new goals under `run-rules/4` from 0.1.16, under `run-rules/5` from 0.1.21 and under `run-rules/6` from 0.1.26; Seal 0.1.36 in v0.1.9 starts them under `run-rules/8`. Preflight refusal, run estimation, `/3` result classification and budget policy apply to this release. Existing goals keep their saved rules. See [Release and development combinations](INSTALL.en.md#release-and-development-combinations).

Rules (`run-rules/N`) are the version of how `ha` computes a goal's records. A goal is computed under the rules it started with until it ends, so a new `ha` does not change the meaning of a goal in progress.

The Seal skill body went from about 4,600 words to at most 2,200 words. Seal 0.1.23 raised that limit to 2,300 words to add the pre-start question rule. The formats of goals started with an earlier Seal moved to a reference read only when continuing such a goal (`references/earlier-goals.md`); the records and documents of those goals do not change.
