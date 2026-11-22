# ha — Seal Core commands

The commands below describe release v0.1.8: ha 0.1.8, spar 0.1.8 and Seal 0.1.23.

## Core compatibility

`ha` runs verification commands without a shell, appends `runs.jsonl`, and computes the goal state from the record, the bundle, and git only. The same inputs always give the same state; the current time is never used.

A new start passes `--check-skill`: `ha start` reads `core-requirements.json` beside the SKILL.md given to `--skill` and compares every rule, format and feature it lists with this executable's `ha capabilities` output. Anything missing is printed as `ha: missing Core support: …` and the start exits 65 with no record. A Core without the check rejects the flag with exit 64 and no record, and a Core without a command Seal uses (`ha report`, `ha doctor`) reports an unknown command with exit 64. In each case record nothing and stop: report the executable (`which -a ha` on Unix, `Get-Command ha -All` on PowerShell) and the message. Never install tools, build `ha`, or fall back to older rules or an older Seal.

An existing goal keeps the rules its start saved, and each command computes or refuses that goal by itself: a saved rule this executable does not compute exits 66 and records nothing, and `ha status` shows the `ha_version` of every record. Report such a refusal. Do not require `/3` declarations for legacy goals or move ordinary maintain, manual and task checks to the structured contract. The optional extensions need their own support, read from the actual executable: findings need `post-completion-findings/v1` and `formats.finding` with `run-finding/v1`; runner adapters need `check-adapter/v1` and `runner-adapters/v1`; `ha timing` needs `run-timing/v1`. Do not send new-format records to an unsupported executable.

When Python 3 is already available, the optional [check_core.py](../scripts/check_core.py) helper checks the same requirements without starting anything. `python3 <skill>/scripts/check_core.py --ha <executable> --operation start` requires everything `core-requirements.json` lists; `--operation resume`, `check`, `budget` or `finding` with `--goal <existing goal>` checks an existing goal's saved rule and format with what that operation needs. It reports `compatible`, `reason`, `executable`, `sha256`, `ha_version`, `operation`, `rules` and `missing`: exit 0 means compatible, 2 refused and 64 invalid arguments. Python is not required; never treat missing Python as missing Core support.

This checks source contracts and executable support, not whether installed Codex or Claude models follow the instructions in practice. Spec document writing requires neither Core nor this check.

## Commands

| Command | Use |
| --- | --- |
| `ha capabilities [--format json]` | Read-only support information without a goal or record; JSON is the default |
| `ha lint <goal>` | Lists goal document and bundle format problems, one per line: code, file:line, detail. Exit 1 when any exist |
| `ha estimate <goal> [AC-n \| EX-n \| T001 …] [--baseline] [--rules <rule>] [--format md\|json]` | Reads the PLAN lower bound, limit and shortfall, plus default and selected runs. `--rules` picks the rule before the start; a started goal uses its saved rule and refuses a different one with 64. Executes nothing; does not change records or budgets |
| `ha start <goal> --request "<quote>" --skill <SKILL.md> [--check-skill] [--rules run-rules/4 \| run-rules/5 \| run-rules/6 \| run-rules/7 \| run-rules/8] [--test-authority "<your reading of the request>"] [--host-name …] [--host-version …] [--model …]` | Records the start once per goal. Refuses lint problems, a second start, a PLAN limit below the required minimum, an automatic limit above the user's pre-approved limit, and, with `--check-skill`, missing Core support the skill declares, before writing a start record. Add `--host-name`, `--host-version` and `--model` when you know them. Without `--rules` the goal uses the default `run-rules/3` |
| `ha check <goal> [AC-n \| EX-n \| T001 …]` | Runs the conditions' commands at HEAD and records each result. With no targets it runs every required `change` and `maintain` condition |
| `ha check <goal> --baseline [AC-n …]` | Runs `change` conditions on a temporary worktree of the base commit with check paths copied from HEAD: up to `/7` the condition's own, under `/8` those of every marked change condition. Expected result: `fail_as_expected` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote "…" --context "…"] [--reason "…"] [--interpretation "…"]` | `/3` to `/8`: changes the current window's total run limit while retaining usage; quote and context for increases, reason for decreases |
| `ha budget <goal> --preapprove <N> --window <seq> --revision <seq> --quote "…" --context "…"` | `/4` to `/7` only: records the user's pre-approved run limit for the window, within which an increase needs only `--reason`; the limit itself stays. `/8` refuses it with 65 |
| `ha note <goal> confirm <AC-n \| scope:<path> \| tests:<path> \| spec> --quote "…"` | Records a quoted user confirmation, bound to the current content. It loses effect if that content changes |
| `ha note <goal> reopen --quote "…"` | Records a rework request; voids the completion record and opens a new budget window |
| `ha note <goal> input --quote "…"` | Records any other user instruction or answer |
| `ha note <goal> block --cause <cause> [--quote "…"]` / `unblock` | Records a stop that cannot be fixed by retrying, and its end |
| `ha note <goal> test-change tests:<path> --criteria AC-n[,…] --how "…" [--moved-to <path> …] [--weakened]` | `/4` to `/8` only: the executor's reason for changing an existing test, bound to the files at HEAD |
| `ha note <goal> wait --reason "<the decision asked>"` / `ha note <goal> resume` | `/4` to `/8` only: the start and end of a wait for the user that the executor reports |
| `ha note <goal> recover <AC-n> --input check\|baseline --change "…"` | `/4` to `/8` only: resets an input past its error limit; earlier errors leave the limit, failures stay |
| `ha note <goal> allow-recovery <AC-n> --input check\|baseline --count N --quote "…"` | `/4` to `/8` only: the user allows N more recoveries for that input |
| `ha note <goal> review --gate <file>` | `/5` to `/8` only: records a `review-gate/v1` file's digest and the summary the review judgment reads. An unusable file exits 65 and records nothing |
| `ha note <goal> check-change checks:<path> --reason "…"` | `/7` only: records the reason for a committed change to a check file after the check freeze, bound to that file at HEAD. Other rules refuse it with 64 |
| `ha status <goal> [--format md\|json]` | Prints the computed state |
| `ha report <goal> [--lang ko\|en]` | Prints the same state for people: a five-line summary, the stamp for `## 현재`, evidence per condition, completion coverage and where unmet output is. Read-only and deterministic; records nothing |
| `ha cost <goal> [--series <folder>]… [--format md\|json] [--lang ko\|en]` | Shows what one goal cost in verification and review, from its records, its goal documents, Git and the Spar series folders named with `--series`. Read-only |
| `ha doctor <goal>` | Runs each probe of PLAN `## 실행 환경` once with a 60-second limit and prints `사용 가능` or `사용 불가` with one of five reasons. Records nothing, uses no runs and installs nothing; a malformed section is reported line by line with exit 65 and no probe runs |
| `ha done <goal> [--format md\|json]` | Computes the state and records it. When the state is `complete`, the line is the completion record |
| `ha log <goal> <seq> [--tail N]` | Shows the local output of one run. Output never leaves this clone |
| `ha timing <goal> [--format md\|json]` | Optional extension: a read-only view of each recorded run's duration, applied time limit, time inside the limit with its proximity, whether the limit fired, and CPU time. It executes nothing, and decisions and budgets never use these values |
| `ha finding <goal> [--format json\|md]` | Optional extension: read historical completion anchors, current findings, and explicit rework relations; default JSON |
| `ha finding <goal> --input <JSON file>` | Optional extension: append an explicitly sourced claim or revision and return a JSON receipt |

`ha check` exits 0 when every result is as expected and 1 otherwise. Other exits: 64 usage, 65 refused, 66 unsupported rules version, 70 internal error. `ha status`, `ha done`, `ha check`, and `ha note` exit 66 for a goal started under rules this `ha` does not compute, and record nothing. A goal under `/1`, `/2` or `/3` refuses the `/4` note kinds and `--preapprove` with 65 and records nothing.

### Post-completion findings

Use the [findings contract](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/findings.md) only after checking the exact executable's optional support, with the goal named explicitly, never guessed from a review link. Base a revision on the findings and historical completion actually read: a new entry starts as a candidate with stable finding and request IDs, the exact completion seq and hash, the historical criteria mapping, its source and a summary, and a later judgment adds the revision read, its reason and evidence references. Core checks relationships and stored structure, not whether a claim is true. A document-only discovery stays a document claim until an explicit sourced registration; never import free text as verified fact or edit the original completion report.

Report the historical completion, current unresolved findings and linked rework status together, and query current findings when resuming. Registering a finding creates no rework authority: only the user's rework request authorizes `note reopen`, which a finding may then reference, and a later completion is linked only within that reopen's window, up to the next reopen, and resolves no finding by itself.

On a revision conflict, reread the stored history and reassess; never overwrite the competing judgment. When writing or printing the receipt fails, the change may have applied: requery by finding and request ID, since an identical request returns its original receipt without another event. A broken chain needs explicit recovery, not an automatic retry. Old binaries reject the extension as a record integrity failure, so keep a supporting executable for later queries and writes. Nothing here migrates, installs, reopens or resets a budget.

## Before execution

If `ha` is missing, record nothing and stop. Install it with `brew install jgoneit/tap/jaekit` or the Release file described in https://github.com/jgoneit/jaekit/blob/v0.1.8/guides/INSTALL.md, then read the actual executable's support. That release has no Windows artifact; the current-source Windows x64 local candidate is a separate route described in https://github.com/jgoneit/jaekit/blob/main/guides/WINDOWS.en.md, and candidate CI does not establish Windows 11 installation or host Skill use. If installation is complete but `ha` is not found, inspect PATH and executable selection (`which -a ha` on Unix, `Get-Command ha -All` on PowerShell) and reopen the host. Building from a checkout (`go install ./cmd/ha`) is only for developing Jaekit.

With `검증 실행: 자동`, `start` sets the run limit to `max(50, ⌈minimum × 1.5⌉)` and refuses with exit 65 and no record when it exceeds the user's pre-approved limit (`preapproved_runs` in `ha.json` of the user's `jaekit` configuration folder, 50 when the file is absent); the message shows the minimum, the computed limit and the pre-approved limit with its source. Seal never writes or changes `ha.json`.

The minimum follows the [current formula](#run-rules8); manual conditions contribute zero. Explicit optional AC, EX and task commands add runs, and repeated targets count as repeated executions. Estimates use the current PLAN limit, not an existing window's remaining budget; they exclude retries, preparation and manual time and guarantee neither total time nor completion. `estimate` exits 1 for a shortfall or lint problems and `start` exits 65 for an insufficient limit; neither raises a limit or changes an existing window.

Before each check, baseline or task process, `ha` inspects the working tree using the goal's exclusions. Dirty paths refuse that target and the remaining targets with exit 65; failed Git inspection exits 70. Diagnostics list repository-relative paths with tracked/untracked and observed symlink details: review the specific path, never ignore or revert it automatically. A refused target creates no check record, run usage or output file; results and usage already recorded remain, errors and later-stale results included. A dirty `start` stays a warning.

## Structured change results

Every change condition, marked or not, maps to a tracked JSON declaration in PLAN's `## 결과 계약` table (`ID | 선언 경로`); `/1` and `/2` goals keep their exit-code results, and ordinary maintain, manual and task checks keep their roles. Include the declaration and its producer files in the condition's check paths. These tracked regular files must match HEAD byte for byte, even within an excluded bundle path; symlink inputs are refused.

The `check-declaration/v1` object contains `producer`, `producer_version`, `producer_paths` and required `targets` (`id`, expected `violation`). The producer writes `check-result/v1` to `HA_EVIDENCE_PATH`, echoing `HA_EVIDENCE_INVOCATION` and `HA_EVIDENCE_DECLARATION_DIGEST`, with `attempts_complete: true` and every observation of each required target: consecutive `attempt` numbers from 1 and a `status` of `pass`, `violation`, `error` or `skip`. An integration that retries keeps every attempt, at most 4096 observations. Missing, duplicated or reversed attempt numbers are inconclusive, and pass and violation on one target keep a `fail` with `inconsistent_attempts`, even when the process exits 0. No report, a mismatched identity, unsupported values, skipped targets or contradictory exits are never positive evidence.

Current success needs exit 0 and every target passing. An expected baseline failure needs a nonzero exit and a declared violation, every required target observed, and no environment error or unintended assertion failure. Missing implementation can be a violation a runnable check observes directly; import, collection, compilation and browser startup errors are not. `unknown` shares the error rerun limit. Later positive results never erase adverse results on the same inputs, including assertion failures kept within an error or inconclusive report.

The [check-result contract](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/check-result.md) describes the JSON fields and `tools/check-result-reference.py`, which reads actual files and reports observations without installing runner adapters. A generic script's nonzero exit alone is unsupported evidence. The compact `check-evidence/v1` summary keeps the classification without raw logs or file contents. An output-file failure after an actual attempt records `error` with `process_output` and keeps its usage when record storage is writable; `output_path` and `output_digest` may be empty. A failed record write can have uncertain application and is never reported as recorded. Declaration, producer and target changes invalidate old evidence; missing raw logs or a changed current time do not. These are executor-authored local claims.

A change condition may instead link an existing test through a runner adapter when the actual executable's `ha capabilities` lists `check-adapter/v1` in `formats.check_declaration` and `runner-adapters/v1` in `features`; otherwise use a producer. The `check-adapter/v1` declaration names `adapter` and `targets`, each with `id`, expected `violation` and the linked `test`, and has no producer files. With `go-test-json` the command is `go test -json …` as it is and the test is `package` plus the full test name; with `junit-xml` the runner writes JUnit XML where ha names, through `{HA_JUNIT_XML}` in an argument or the `HA_JUNIT_XML` variable, and the test is `classname` plus `name`. Put the declaration and the linked test file in the check paths. Write `jaekit:violation=<declared violation>` only on the opening line of the failure message of the assertion that checks the declared property, never in compared values or logs: ha reads markers on the opening line of each failure message only, and a failure without the same marker on every opening line is an unexpected assertion. A marked condition's test observes the behavior through an entry point that also exists at the base commit, such as a CLI, an HTTP interface or a public API that already existed; a test that calls a function the change adds records an error at the base commit, so use a dedicated producer for that condition. An unmarked condition may test a function or type the change adds directly. [§5 of the check-result contract](https://github.com/jgoneit/jaekit/blob/main/contracts/check-result.md#5-runner-adapters) classifies the output and its input failures.

Direct and Homebrew installations include the optional Python reference producer and a synthetic file-observation example; [Get the reference check](https://github.com/jgoneit/jaekit/blob/v0.1.8/guides/INSTALL.en.md#get-the-reference-check) copies them into the project without a development checkout. The reference needs Python 3; choosing it is optional and makes neither Python nor any particular runner a prerequisite for Seal.

## run-rules/8

Seal starts every goal under `run-rules/8`. It keeps the test-change authority, reported waits, error recovery and command time limits of [`/4`](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4), the review gate of [`/5`](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#48-run-rules5), the review base and post-completion judgments of [`/6`](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#49-run-rules6) and the baseline marks of [`/7`](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#410-run-rules7), without the pre-approved limit or the check freeze, and gives every baseline one overlay ([§4.11](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#411-run-rules8)). The linked sections hold the exact judgments.

- **Authority.** Act within the authority the start recorded without asking the user again, and stop only for what it does not cover. When the user's request authorizes changing tests inside the goal's scope, start with `--test-authority "<your reading of the request>"`; the start keeps a copy of the plan's `바꿀 수 있는 경로` and `테스트 경로`, so later plan edits do not change which files count as tests. For each existing test file changed inside that scope, record `ha note <goal> test-change tests:<path> --criteria AC-n --how "<how the file checks them>"` on a clean, committed tree, with `--moved-to <path>` for every file inside that scope a moved or split test now lives in, at least one for a deletion. The note covers the file and each moved-to path only as they are at that commit. Mark a change that weakens a check with `--weakened` and ask the user to confirm it, as for a change outside the recorded authority or one without its reason.
- **Waits.** When you ask the user a question the remaining work depends on, record `ha note <goal> wait --reason "<the decision asked>"`, and `ha note <goal> resume` when you continue without the answer; a quoted answer or a later run also ends the wait. Keep doing independent work while you wait.
- **Budget.** Only the run limit stops a goal: the PLAN's `경과 시간` is optional and informational and never makes `budget_exceeded`. Status still shows the elapsed time, which leaves out `reported_wait_seconds` and `post_completion_seconds`, the time after a completion record until the next run. Every increase of the run limit needs the user's quote and its context ([Budget changes](#budget-changes)).
- **Error recovery.** When an input passes its error limit because of how the check runs, such as a missing tool or a wrong path, fix that and record `ha note <goal> recover <AC-n> --input check|baseline --change "<what changed>"`. Only the errors and unknowns counted when the note was recorded leave the limit; `fail` results and unexpected baseline passes stay. Each condition and input has two recoveries; past them, ask the user, who can allow more with `ha note <goal> allow-recovery <AC-n> --input check|baseline --count <N> --quote "…"`.
- **Time limits.** Declare a command that needs more than the default 900 seconds in the plan's optional `## 시간 제한` section, such as `- AC-3: 30분` or `- T002 명령 1: 20분`, in 시간, 분 and 초 up to 7200 seconds; a larger or unknown entry is a lint problem. A baseline uses its condition's limit, and changing a limit makes earlier results stale. Before each run `ha check` prints a `time-limit notice:` for the latest earlier run that reached its limit or used 800‰ of it, or `risk unknown`; after one, split the check or change its limit instead of rerunning it as it is.
- **Review gate.** A `standard` or `strict` goal completes only when the last recorded gate covers HEAD and the current goal documents. Give every goal document to each round as criteria. Every review round gets `REVIEW.md` as `--boundary`, which needs a Spar that takes `--boundary` and no Core change. After the cumulative round ends with no open `defect` or `requirement-question`, make the gate from that series' rounds with `spar gate … --expect-head <HEAD>`, record it with `ha note <goal> review --gate <file>`, and run `ha done`; commit nothing between the gate's head and `ha done`. The last non-dry-run candidate round starts at the start record's base commit, also after `reopen`; otherwise `review_base_mismatch` keeps completion pending. A later `review` note voids a completion record only while the tier is `standard` or `strict`. Gates over series with `plan-delta` or `fixes` rounds are still read. To finish without review, the user decides a goal-document change that lowers the tier to `light`.
- **Baselines.** A change condition requires a baseline only when the PLAN `baseline` column marks it `요구`, which Seal does in two cases: a bug fix or regression whose defect is already present at the base commit, and a prohibition such as "X does not happen" ([REVIEW.md](bundle.md#reviewmd)). `ha check <goal> --baseline` without targets selects only the marked change conditions, optional ones included, and refuses with 64 and no record when none is marked; an explicit unmarked target still records a baseline and uses a run, and its adverse results still count. Every baseline copies from HEAD the check paths of every marked change condition, so a commit to any of them makes every baseline stale. Record marked baselines once, after the last review round and right before the completion record. "Before any baseline" means before the goal's first `ha check` record ([questions](bundle.md#questions-for-the-user)). Keep input failures out of passes, and apply the producer rule in [Check inputs](bundle.md#check-inputs).
- **Minimum.** The required minimum is `표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain`: twice the marked required change count, plus the unmarked required change and required maintain counts. Start admission and the automatic budget use it, and `ha estimate <goal> --rules run-rules/8` computes it before the start.
- **Inspection.** Status shows each change condition's `baseline_required`; `ha report` counts the marked conditions and their fresh baselines and lists marks removed since the start under `직접 볼 것`. The review summary lists change conditions met without a baseline under `### 보지 않은 것`. There is no check freeze: status shows no `check_freeze`, and `ha note <goal> check-change checks:<path> --reason "…"` exits 64 without a record.
- **Blocks.** Record a block only when the user, a required review or a real-boundary environment is all that remains.

## Budget changes

Use the current `budget.window_start`, `revision` and `runs_limit` from `ha status --format json` as `--window`, `--revision` and `--from`; all four numbers, the new `--runs` included, are required positive integers. The change compares the latest window and budget revision under the record lock: concurrent checks keep their actual usage, and a concurrent budget change or reopen gives a conflict instead of an overwrite. Read the resulting state before another change.

`--runs` sets a total: 48 uses out of 50 changed to 100 leaves 52, and lowering it to 40 leaves zero and an excess of 8. It changes neither the time limit nor the window's usage, and editing PLAN alone has no effect. An increase quotes the user with the request context supporting that limit; a decrease needs a reason. Optional `--interpretation` is separate from the quote, and unverified host provenance stays an executor claim.

A change appends `budget_change`; it uses no check run and does not reopen, unblock or reset errors, but it advances the last-record time used for elapsed time. Empty or same-value requests and stale window, revision or previous-limit inputs record nothing. Time-limit changes are unsupported. A completed window needs explicit rework and `reopen`; [run-record.md §4.7](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4) defines the run limit a reopened window starts with. Checks belong to the window in which their records are appended, even when execution straddles a reopen.

Budget exits: 0 success, 64 invalid arguments, 65 refused input or state, 66 legacy or unsupported rules, 70 internal failure or uncertain application. A write, sync or close failure can leave an applied change: inspect the record instead of assuming either outcome or retrying automatically. There is no execution hard cap or concurrent slot reservation.

Status shows the budget revision, remaining or excess runs, reached state and change history. `usage` separates total attempts, check and baseline kinds, errors and unknowns, current and stale inputs and valid positive evidence; these overlap and are not summed. Baselines and unsuccessful attempts still count as use.

## States

| State | Exit | Meaning |
| --- | --- | --- |
| `complete` | 0 | A valid completion record exists, or every required condition is satisfied with no remaining reasons; only `ha done` turns the latter into a completion record |
| `incomplete` | 1 | Something the executor can fix remains |
| `needs_user` | 2 | Only items that need the user remain |
| `blocked` | 3 | A `block` note has no later `unblock` |
| `budget_exhausted` | 4 | The budget window passed its run count or, up to `run-rules/7`, its elapsed time, and under `run-rules/2` through `run-rules/8` another reason remains as well |

A goal is computed under the rules its start record names (`rules:` in `ha status`). From `run-rules/2` on, a goal whose required conditions are all satisfied and whose only remaining reason is `budget_exceeded` is `complete`: `ha done` records the evidence already gathered, and status still shows the budget as exceeded. That is not leave to keep working past the budget; more checks or work need the user's words. Under `run-rules/1` the same goal is `budget_exhausted`.

## Status output

The Markdown form of `ha status` lists, under `### Unmet` ahead of the criteria table, each unsatisfied required condition with its reason and who resolves it, and each goal-level reason. A condition whose reason rests on a fresh record gets that record's seq and, when its output was stored, the command `ha log <goal> <seq>`. The command names the goal as given, so it runs from the same directory; an absolute argument becomes the repository-relative goal path, run from the repository root, and a path with characters other than letters, digits and `-_./@%+=:,` is single-quoted. Otherwise the line says `no stored output` or `no fresh record`. A complete goal has no summary.

When the latest `done` record was not a completion, the completion line names its seq, commit and recorded state. When an earlier completion record no longer holds, the line names that record's seq and commit and why: a broken record chain, invalid stored records, uncommitted changes outside the bundle documents, code changed since that commit, changed goal documents, a changed required command, a later `reopen`, or, under `run-rules/3`, required conditions that are no longer all satisfied. A goal without any `done` record shows `none`. `ha done` prints the earlier form: no unmet summary, and no `done` details on its completion line. A synthetic example of `ha status`:

```text
- completion record: none; latest completion seq 6 (commit 8f3a2d91c0b7) is no longer valid: code outside the bundle documents changed since that commit; required conditions are not all satisfied

### Unmet

- AC-2 `criterion_failed` (executor): seq 8, output `ha log docs/specs/example 8`
- AC-3 `criterion_error` (executor): seq 9, no stored output
- `task_open` (executor)
```

A failed `ha check` prints, for each target that did not meet its expectation, `output for <target>: ha log <goal> <seq>`, or `no stored output` with the seq, also when a later target is refused or cannot be recorded. None of this changes judgment, `status/v1`, the completion record or `ha log`; the human output never contains command output.

A `/7` goal with a recorded freeze round also shows `check_freeze`: the check files changed since that round, each with any reason recorded by `ha note <goal> check-change checks:<path> --reason "…"`, compared with the condition table that `ha note <goal> review --gate <file>` saved right after it ([§4.10](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#410-run-rules7)). The display never blocks completion.

## Reasons

| Reason | Who resolves it | Typical cause |
| --- | --- | --- |
| `criterion_missing` | executor | No fresh check result for the condition |
| `criterion_flaky` | executor | Pass and fail on the same code |
| `criterion_failed` | executor | A fresh check failed |
| `criterion_error` | executor | The latest fresh result is an error within the rerun limit |
| `baseline_missing` | executor | A `change` condition has no fresh baseline |
| `baseline_unexpected_pass` | executor | The check already passes on the base commit, so it does not test the change |
| `baseline_failed` | executor | `/3` baseline retained an unintended assertion failure or inconsistent internal attempts |
| `evidence_invalid` | executor | Structured evidence is invalid |
| `record_invalid` | user | An unknown result or inconsistent stored record cannot support completion |
| `baseline_error` | executor | The latest baseline errored within the rerun limit |
| `lint_error` | executor | `ha lint` reports problems |
| `not_started` | executor | No `ha start` |
| `worktree_dirty` | executor | Uncommitted changes outside the bundle documents; paths and tracking/file-type details identify the changes |
| `task_open` | executor | A required task is not `done` or `dropped` in `PROGRESS.md` |
| `error_limit` | user | Errors exceeded the rerun limit on the same code |
| `manual_unconfirmed` | user | A `manual` condition lacks a valid quoted confirmation |
| `record_integrity` | user | `runs.jsonl` was edited; its chain is broken |
| `out_of_scope` | user | Files changed since base outside `바꿀 수 있는 경로` |
| `test_definition_changed` | user | Tests that existed at base were modified or deleted |
| `spec_changed` | user | The goal documents differ from the start |
| `blocked` | — | See states |
| `budget_exceeded` | — | See states |
| `review_missing`, `review_blocked`, `review_degraded`, `review_open_defect`, `review_code_missing`, `review_head_stale`, `review_docs_stale`, `review_plan_missing` | executor | `/5` to `/8` with a `standard` or `strict` tier: the recorded review gate is missing or does not cover HEAD and the current goal documents ([run-rules/8](#run-rules8)) |
| `review_base_mismatch` | executor | `/6` to `/8`, with a `standard` or `strict` tier: the last non-dry-run candidate's base differs from the goal's start base commit, or the gate has only fixes rounds; run the cumulative candidate round from that start base commit and record its gate ([run-rules/8](#run-rules8)) |
| `review_open_question` | user | `/5` to `/8`: the recorded gate has an open `requirement-question` |

## What checks read

`ha` uses the base commit in three places: the baseline, which runs `change` conditions on a temporary worktree of the base commit with the check paths of every marked change condition copied from HEAD; `out_of_scope`, the files changed since base outside `바꿀 수 있는 경로`; and `test_definition_changed`. Checks themselves judge the tree they run in and never read the base commit or other history, so the same check gives the baseline's violation and the later pass. The exception is a condition about history or refs itself, such as a tag's type, whose check reads only the range and refs the condition names. See [bundle.md](bundle.md#check-inputs).

## Freshness

A check result counts only while all of these hold: the tree is clean now and was clean when it ran; the code outside the bundle documents equals its commit; the goal documents and the condition's command are unchanged. A baseline counts while it ran on the start's base commit, the goal documents and command are unchanged, and its overlay is exactly the sorted union of the check paths of the currently marked change conditions, each equal to HEAD's file. Committing only bundle documents keeps results fresh.

The bundle files of other goals count as bundle documents too: `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl` and `tasks/` of every other directory with a `SPEC.md` under the same parent, for freshness, the clean tree, scope and the completion record, so another goal on the same branch committing its progress leaves these results fresh. Their `SPEC.md`, linked documents and `checks/` stay code. A plan whose check reads other goals' bundle files declares ``다른 목표 실행 묶음: `검사 입력` `` in `## 범위`, which keeps them in the code state; `ha` reads the declaration from the plan as it is when it computes or records.

Every status output states assurance `local`: an agent with the same user permissions can change checks and records.

## Final head

`ha check` runs each condition's command as its own process, so conditions that need the same full regression each run it; the recommended layout is that one condition owns it and the others observe their own property ([final candidate evidence](bundle.md#final-candidate-evidence)).

A completion record covers its commit while it stays valid and `ha status` names it: among other conditions, the code outside the bundle documents equals HEAD and the goal documents and required commands are unchanged. When the user asks, after completion, to apply remote review or pull request comments, record that request with `ha note <goal> reopen --quote "…"`. After the fixes are committed, earlier `check` records are stale, while a baseline stays fresh as long as it ran on the start's base commit and its overlay still matches HEAD. Re-record the stale required checks on the final head and run `ha done` again before the head is sent or merged. Comments without such a request do not reopen the goal.
