# ha — Seal Core commands

The commands below describe release v0.1.8: ha 0.1.8, spar 0.1.8 and Seal 0.1.23.

## Core compatibility

`ha` runs verification commands without a shell, appends `runs.jsonl`, and computes the goal state from the record, the bundle, and git only. The same inputs always give the same state; the current time is never used.

A new start passes `--check-skill`: `ha start` reads `core-requirements.json` beside the SKILL.md given to `--skill` and compares every rule, format and feature it lists with this executable's `ha capabilities` output. Anything missing is printed as `ha: missing Core support: …` and the start exits 65 with no record. A Core without the check rejects the flag with exit 64 and no record, and a Core without a command Seal uses (`ha report`, `ha doctor`) reports an unknown command with exit 64. In each case record nothing and stop: report the executable (`which -a ha` on Unix, `Get-Command ha -All` on PowerShell) and the message. Never install tools, build `ha`, or fall back to older rules or an older Seal.

An existing goal keeps the rules its start saved, and each command computes or refuses that goal by itself: a saved rule this executable does not compute exits 66 and records nothing, and `ha status` shows the `ha_version` of every record. Report such a refusal. Do not require `/3` declarations for legacy goals or move ordinary maintain, manual and task checks to the structured contract. The optional extensions need their own support, read from the actual executable: findings need `post-completion-findings/v1` and `formats.finding` with `run-finding/v1`; runner adapters need `check-adapter/v1` and `runner-adapters/v1`. Do not send new-format records to an unsupported executable.

When Python 3 is already available, the optional [check_core.py](../scripts/check_core.py) helper checks the same requirements without starting anything. `python3 <skill>/scripts/check_core.py --ha <executable> --operation start` requires everything `core-requirements.json` lists; `--operation resume`, `check`, `budget` or `finding` with `--goal <existing goal>` checks an existing goal's saved rule and format with what that operation needs. It reports `compatible`, `reason`, `executable`, `sha256`, `ha_version`, `operation`, `rules` and `missing`: exit 0 means compatible, 2 refused and 64 invalid arguments. Python is not required; never treat missing Python as missing Core support.

This checks source contracts and executable support. Whether installed Codex or Claude models follow the instructions in practice is a separate host evaluation; capability output does not establish that result. Spec document writing requires neither Core nor this check.

## Commands

| Command | Use |
| --- | --- |
| `ha capabilities [--format json]` | Read-only support information without a goal or record; JSON is the default |
| `ha lint <goal>` | Lists goal document and bundle format problems, one per line: code, file:line, detail. Exit 1 when any exist |
| `ha estimate <goal> [AC-n \| EX-n \| T001 …] [--baseline] [--format md\|json]` | Reads the PLAN lower bound, limit and shortfall, plus default and selected runs. Executes nothing; does not change records or budgets |
| `ha start <goal> --request "<quote>" --skill <SKILL.md> [--check-skill] [--rules run-rules/4 \| run-rules/5 \| run-rules/6 \| run-rules/7 \| run-rules/8] [--test-authority "…"] [--host-name …] [--host-version …] [--model …]` | Records the start once per goal. Refuses lint problems, a second start, a PLAN limit below the required minimum, an automatic limit above the user's pre-approved limit, and, with `--check-skill`, missing Core support the skill declares, before writing a start record. Add `--host-name`, `--host-version` and `--model` when you know them. Without `--rules` the goal uses the default `run-rules/3` |
| `ha check <goal> [AC-n \| EX-n \| T001 …]` | Runs the conditions' commands at HEAD and records each result. With no targets it runs every required `change` and `maintain` condition |
| `ha check <goal> --baseline [AC-n …]` | Runs `change` conditions on a temporary worktree of the base commit with check paths copied from HEAD: up to `/7` the condition's own, under `/8` those of every marked change condition. Expected result: `fail_as_expected` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote "…" --context "…"] [--reason "…"] [--interpretation "…"]` | `/3` to `/8`: changes the current window's total run limit while retaining usage; quote and context for increases, reason for decreases |
| `ha budget <goal> --preapprove N --window W --revision R --quote "…" --context "…"` | `/4` to `/7` only: records the user's pre-approved run limit for the window; the limit itself stays |
| `ha note <goal> confirm <AC-n \| scope:<path> \| tests:<path> \| spec> --quote "…"` | Records a quoted user confirmation, bound to the current content. It loses effect if that content changes |
| `ha note <goal> reopen --quote "…"` | Records a rework request; voids the completion record and opens a new budget window |
| `ha note <goal> input --quote "…"` | Records any other user instruction or answer |
| `ha note <goal> block --cause <cause> [--quote "…"]` / `unblock` | Records a stop that cannot be fixed by retrying, and its end |
| `ha note <goal> test-change tests:<path> --criteria AC-n[,…] --how "…" [--moved-to <path> …] [--weakened]` | `/4` to `/8` only: the executor's reason for changing an existing test, bound to the files at HEAD |
| `ha note <goal> wait --reason "…"` / `resume` | `/4` to `/8` only: the start and end of a wait for the user that the executor reports |
| `ha note <goal> recover <AC-n> --input check\|baseline --change "…"` | `/4` to `/8` only: resets an input past its error limit; earlier errors leave the limit, failures stay |
| `ha note <goal> allow-recovery <AC-n> --input check\|baseline --count N --quote "…"` | `/4` to `/8` only: the user allows N more recoveries for that input |
| `ha note <goal> review --gate <file>` | `/5` to `/8` only: records a `review-gate/v1` file's digest and the summary the review judgment reads. An unusable file exits 65 and records nothing |
| `ha note <goal> check-change checks:<path> --reason "…"` | `/7` only: records the reason for a committed change to a check file after the check freeze, bound to that file at HEAD. Other rules refuse it with 64 |
| `ha status <goal> [--format md\|json]` | Prints the computed state |
| `ha report <goal> [--lang ko\|en]` | Prints the same state for people: a five-line summary, the stamp for `## 현재`, evidence per condition, completion coverage and where unmet output is. Read-only and deterministic; records nothing |
| `ha cost <goal> [--series <folder>]… [--format md\|json] [--lang ko\|en]` | Shows what one goal cost in verification and review, from its records, its goal documents, Git and the Spar series folders named with `--series`. Read-only |
| `ha doctor <goal>` | Runs each probe of PLAN `## 실행 환경` once with a 60-second limit and prints `사용 가능` or `사용 불가` with the reason. Records nothing and uses no runs |
| `ha done <goal> [--format md\|json]` | Computes the state and records it. When the state is `complete`, the line is the completion record |
| `ha log <goal> <seq> [--tail N]` | Shows the local output of one run. Output never leaves this clone |
| `ha timing <goal> [--format md\|json]` | Optional extension: a read-only view of each recorded run's duration, applied time limit, time inside the limit with its proximity, whether the limit fired, and CPU time. It executes nothing, and decisions and budgets never use these values |
| `ha finding <goal> [--format json\|md]` | Optional extension: read historical completion anchors, current findings, and explicit rework relations; default JSON |
| `ha finding <goal> --input <JSON file>` | Optional extension: append an explicitly sourced claim or revision and return a JSON receipt |

`ha check` exits 0 when every result is as expected and 1 otherwise. Other exits: 64 usage, 65 refused, 66 unsupported rules version, 70 internal error. `ha status`, `ha done`, `ha check`, and `ha note` exit 66 for a goal started under rules this `ha` does not compute, and record nothing. A goal under `/1`, `/2` or `/3` refuses the `/4` note kinds and `--preapprove` with 65 and records nothing; a `/8` goal refuses `--preapprove` the same way.

### Post-completion findings

Use the [findings contract](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/findings.md) only after checking the exact executable's optional support. The goal is explicit; do not guess another goal from a review link. Base a revision on the existing findings and historical completion actually read. New entries start as candidates with a stable finding ID and request ID, exact completion seq/hash, historical criteria mapping, source and summary. Later judgments include the revision actually read, reason and evidence references. Core checks the relationships and stored structure, not whether a review claim is true. Document-only discoveries remain document claims until an explicit sourced registration; do not import free text as verified facts or edit the original completion report.

Report the historical completion, current unresolved findings, and linked explicit rework status together. Registering a finding preserves the original completion and creates no rework authority. Only a user's rework request authorizes `note reopen`; afterward a finding may reference that saved request. A later completion is linked only within that reopen's window, stopping at the next reopen, and does not automatically resolve a finding. Resuming an existing goal requires querying its current findings as well as its computed execution state.

On a revision conflict, read the stored history and reassess; do not overwrite the competing judgment. If writing or printing the receipt fails, the change may have applied. Requery by finding/request ID; an identical request can return its original receipt without an additional event. A broken chain requires explicit recovery, not an automatic retry. Old binaries reject the extension as record integrity failure, so retain a supporting executable for later queries and writes. No automatic migration, installation, reopen, or budget reset is implied.

## Before execution

If `ha` is missing, record nothing and stop. Use `brew install jgoneit/tap/jaekit` or the Release file described in https://github.com/jgoneit/jaekit/blob/v0.1.8/guides/INSTALL.md, and read the actual executable's support once it is installed: it must declare everything in `core-requirements.json`, which a released package may not provide yet, and `ha start --check-skill` then refuses. Windows is not supported by that release; it has no Windows artifact. The current-source Windows x64 local candidate is a separate route described in https://github.com/jgoneit/jaekit/blob/main/guides/WINDOWS.en.md; candidate CI does not establish Windows 11 installation or host Skill use. If installation is complete but `ha` is not found, inspect PATH and executable selection (`which -a ha` on Unix, `Get-Command ha -All` on PowerShell) and reopen the host. Building from a checkout (`go install ./cmd/ha`) is only for developing Jaekit.

[run-record.md §1.5](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#15-automatic-budget) defines how `start` sets the run limit for `검증 실행: 자동`, when it refuses it against the user's pre-approved limit (`preapproved_runs` in `ha.json`), and what `ha estimate` shows. Seal asks the user once, writes the approved number in `검증 실행`, and never writes or changes `ha.json`.

[run-record.md §1.4](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#14-environment-check-ha-doctor) defines what `ha doctor <goal>` reports and refuses.

[run-record.md §1.1](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#11-diagnostics-before-a-run) defines the minimum under each rule, which runs it counts, and what an estimate excludes; [run-rules/8](#run-rules8) gives the current one.

[run-record.md §1.1](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#11-diagnostics-before-a-run) defines how `ha` refuses a dirty working tree before each check, baseline or task process, and what a refusal leaves. Review the specific path; do not automatically ignore or revert it.

## Structured change results

The Core default is `run-rules/3`; Seal selects `run-rules/8` explicitly. The structured change-result contract applies from `/3` onward, including unmarked `/7` and `/8` change conditions. Existing `/1` and `/2` goals retain their result semantics. Ordinary maintain, manual and task checks keep their roles. A `/3` change condition maps to a tracked JSON declaration in PLAN's `## 결과 계약` table (`ID | 선언 경로`). Include the declaration and its producer files in the condition's check paths. These tracked regular files must match HEAD byte for byte, even within an excluded bundle path; symlink inputs are refused.

The [check-result contract](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/check-result.md) and its `tools/check-result-reference.py` define the `check-declaration/v1` object, the `check-result/v1` report the producer writes to `HA_EVIDENCE_PATH`, and which results are success, an expected baseline failure or neither ([§3](https://github.com/jgoneit/jaekit/blob/main/contracts/check-result.md#3-classification-and-retention)).

A change condition may also link an existing test through a runner adapter, an optional alternative to a producer. Use it only when the actual executable's `ha capabilities` lists `check-adapter/v1` in `formats.check_declaration` and `runner-adapters/v1` in `features`; otherwise use the reference producer or a dedicated producer as before. The `check-adapter/v1` declaration names `adapter` and `targets`, each with `id`, expected `violation` and the linked `test`, and has no producer files. With `go-test-json` the command is `go test -json …` as it is and the test is `package` plus the full test name; with `junit-xml` the runner writes JUnit XML where ha names, through `{HA_JUNIT_XML}` in an argument or the `HA_JUNIT_XML` variable, and the test is `classname` plus `name`. Put the declaration and the linked test file in the check paths. Write `jaekit:violation=<declared violation>` only on the opening line of the failure message of the assertion that checks the declared property, never in compared values or logs: ha reads markers on the opening line of each failure message only, and a failure without the same marker on every opening line is an unexpected assertion. A test used for a baseline must observe the behavior through an entry point that also exists at the base commit, such as a CLI, an HTTP interface or a public API that already existed; a test that cannot, for example one that calls a function the change adds, records an error at the base commit, so use a dedicated producer for that condition. Under `run-rules/7` and `run-rules/8`, the constraint to observe through an entry point that also exists at the base commit applies only to marked conditions; an unmarked condition may directly test a function or type added by the change. The output classification and its input failures are in §5 of the public check-result contract of a release that includes the adapters.

Direct and Homebrew installations include the optional Python reference producer and a synthetic file-observation example. Follow [Get the reference check](https://github.com/jgoneit/jaekit/blob/v0.1.8/guides/INSTALL.en.md#get-the-reference-check) to copy them into the project without a development checkout. The reference needs Python 3; choosing it is optional and does not make Python or any particular runner a prerequisite for Seal.

## run-rules/8

A `run-rules/8` goal keeps the items below except the freeze and its display, and its baseline copies the check paths of every marked change condition ([run-record.md §4.11](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#411-run-rules8)). [run-record.md §4.7](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4), [§4.8](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#48-run-rules5), [§4.9](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#49-run-rules6) and [§4.10](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#410-run-rules7) define the judgments and status fields of these rules: test-change authority, reported waits, pre-approved runs, error recovery, command time limits, the review gate and `review_base_mismatch`, unjudged completion lines, the time after completion that `post_completion_seconds` counts, baseline marks and, under `/7` only, `check_freeze`.

- **Test-change authority.** `--test-authority "<reading>"` records the executor's reading of the start request as permission to change tests. A `/4` start line also keeps `scope` (`allowed` and `tests`, the plan's paths at start); test files are classified by that copy, and later plan edits do not change the classification. A changed base test inside `scope.allowed` needs no confirmation when its last `test-change` note names conditions and a check method, is not marked `--weakened`, and is bound to the file as it is now: its current blob, or its deletion. Every `--moved-to` path must stay inside the authority and unchanged since the note, whether the original remains or not, and a deletion needs at least one.
- **How Seal uses them.** Act within the authority the start recorded without asking the user again, and stop only for what it does not cover. When the user's request authorizes changing tests inside the goal's scope, start with `--test-authority "<your reading of the request>"`, and for each existing test file changed inside that scope record `ha note <goal> test-change tests:<path> --criteria AC-n --how "<how the file checks them>"` on a clean, committed tree, with `--moved-to <path>` for every file a moved or split test now lives in. Mark a change that weakens a check with `--weakened` and ask the user to confirm it, as for a change outside the recorded authority or one without its reason. When you ask the user a question the remaining work depends on, record `ha note <goal> wait --reason "<the decision asked>"`, and `ha note <goal> resume` when you continue without the answer; only reported waits leave the elapsed budget, so keep working on independent work while you wait. In a `/4` to `/7` goal, within a pre-approved limit (`ha budget <goal> --preapprove <N> --window <seq> --revision <seq> --quote "…" --context "…"`), raise the limit with `--runs` and a `--reason`. When an input passes its error limit because of how the check runs, such as a missing tool or a wrong path, fix that and record `ha note <goal> recover <AC-n> --input check|baseline --change "<what changed>"`; past the recovery limit, ask the user, who can allow more with `ha note <goal> allow-recovery <AC-n> --input check|baseline --count <N> --quote "…"`. Declare a command that needs more than 900 seconds in `## 시간 제한`, and split a check or change its limit instead of rerunning it as it is after a time-limit notice. Record a block only when the user, a required review or a real-boundary environment is all that remains.
- **Review gate.** When the current SPEC's `Assurance:` tier is `standard` or `strict`, the last recorded review gate must cover HEAD and the current goal documents before `ha done` records completion; a `light` goal or one without the line needs no gate. Give every goal document to each round as criteria. After the cumulative round ends with no open `defect` or `requirement-question`, make the gate from that series' rounds with `spar gate … --expect-head <HEAD>`, record it, and run `ha done`. Commit nothing between the gate's head and `ha done`. To finish without review, the user decides a goal-document change that lowers the tier to `light`. Every review round gets `REVIEW.md` as `--boundary`, which needs a Spar that takes `--boundary` and no Core change.
- **Baseline selection.** The optional final PLAN `baseline` column marks a change condition with `요구` when it requires a baseline, which Seal does only in two cases: a bug fix or regression whose defect is already present at the base commit, and a prohibition such as "X does not happen"; an empty cell, `—` or an absent column leaves it unmarked. REVIEW gives the reason for marked conditions only ([bundle guidance](bundle.md#reviewmd)). Only marked conditions need a fresh `fail_as_expected` baseline, and `ha check <goal> --baseline` without targets selects only marked change conditions, optional ones included. With none marked it refuses with exit 64 and no record. An explicit unmarked target can still record a baseline, consuming a run, and its adverse results still affect judgment. Unmarked current checks keep the structured result contract.
- **Minimum.** The required minimum is `표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain`: twice the marked required change count, plus unmarked required change and required maintain counts. Start admission and automatic budgeting use it. `ha estimate <goal> --rules run-rules/8` selects it before Seal's start; an existing goal uses its saved rule and refuses a conflicting `--rules` with 64.
- **Timing and inputs.** The skill's "before any baseline" means before any recorded current check or baseline ([questions](bundle.md#questions-for-the-user)). Record marked baselines once, after the last review round and right before the completion record; keep input failures out of passes, and apply the narrower producer rule in [Check inputs](bundle.md#check-inputs).
- **Freeze (`/7` only).** [Check inputs](bundle.md#check-inputs) states the freeze head, the frozen files and the steps after a change, as [run-record.md §4.10](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#410-run-rules7) defines them.
- **Inspection.** Status exposes each change condition's `baseline_required`. Report counts the marked conditions and their fresh baselines, and lists marks removed since start under `직접 볼 것`. For a frozen `/7` goal, `check_freeze` compares check files to the freeze head and reports changed, added and removed check paths with any reason bound to their current state; report separates changes with and without a reason. Removed paths use the condition table saved by the review note immediately following the freeze round; its seq is shown as the table source. Without that note the table was not recorded and removed paths are unobserved. A `light` goal, a goal without a tier or without a recorded freeze round has no freeze display. These displays do not block completion. The review summary lists change conditions met without a baseline under `### 보지 않은 것`.

## Budget changes

Use the current `budget.window_start`, `revision` and `runs_limit` from `ha status --format json` as `--window`, `--revision` and `--from`. [run-record.md §1](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#1-ha-commands), [§2.6](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#26-budget_change) and [§5.2](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#52-budget) define a change's exit codes and what it checks, records and leaves under each rule. Read the resulting state before deciding on another change.

Increase quotes need the request context supporting that specific limit; decreases need a reason. A completed window needs explicit rework and `reopen`. `/3` reopens with the latest run limit and original elapsed limit; `/1` and `/2` retain the original start limit. [run-record.md §4.7](https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4) defines the run limit of a reopened `/4` window.

A write/sync/close failure can leave an applied change; inspect the record instead of assuming success or non-application and automatically retrying.

Status exposes budget revision, remaining/excess runs, reached state and change history. `usage` separates total attempts, check/baseline kinds, errors/unknowns, current/stale inputs and valid positive evidence. These are overlapping axes, not numbers to sum. Baselines and unsuccessful attempts still count as use.

## States

| State | Exit | Meaning |
| --- | --- | --- |
| `complete` | 0 | A valid completion record exists, or every required condition is satisfied with no remaining reasons; only `ha done` turns the latter into a completion record |
| `incomplete` | 1 | Something the executor can fix remains |
| `needs_user` | 2 | Only items that need the user remain |
| `blocked` | 3 | A `block` note has no later `unblock` |
| `budget_exhausted` | 4 | The budget window passed its run count or, up to `run-rules/7`, its elapsed time, and under `run-rules/2` through `run-rules/8` another reason remains as well |

A goal is computed under the rules its start record names (`rules:` in `ha status`). Under `run-rules/2` and `run-rules/3`, a goal whose required conditions are all satisfied and whose only remaining reason is `budget_exceeded` is `complete`: `ha done` records the evidence already gathered, and the status still shows the budget as exceeded. That is not leave to keep working past the budget; more checks or work past it still need the user's words. `/3` can record a same-window run-limit increase with `ha budget`; an explicit new window uses `reopen`. `/1` and `/2` retain the quoted `reopen` behavior. Under `run-rules/1` the same goal is `budget_exhausted`.

## Status output

The Markdown form of `ha status` lists what keeps a goal from completing ahead of the criteria table, under `### Unmet`. This unmet summary names each unsatisfied required condition with its reason and who resolves it, and each goal-level reason. For a condition whose reason rests on a fresh record it gives that record's seq and, when the run's output was stored, the exact command `ha log <goal> <seq>` to open it. The command names the goal argument as given, so it runs from the same directory; an absolute argument becomes the repository-relative goal path, run from the repository root, so human output carries no absolute path. A goal path with characters other than letters, digits and `-_./@%+=:,` is single-quoted so a copied command passes it as one argument. A record without stored output says `no stored output` and gets no command; a condition without a fresh record says `no fresh record` and gets no seq. A complete goal has no summary.

When the latest `done` record was not a completion, the completion line names its seq, commit and recorded state. When an earlier completion record no longer holds, the line names that record's seq and commit and why it is no longer valid: a broken record chain, invalid stored records, uncommitted changes outside the bundle documents, code changed since that commit, changed goal documents, a changed required command, a later `reopen`, or, under `run-rules/3`, required conditions that are no longer all satisfied. A goal without any `done` record still shows `none`. `ha done` prints its earlier form: no unmet summary, and no `done` details on its completion line. A synthetic example of `ha status`:

```text
- completion record: none; latest completion seq 6 (commit 8f3a2d91c0b7) is no longer valid: code outside the bundle documents changed since that commit; required conditions are not all satisfied

### Unmet

- AC-2 `criterion_failed` (executor): seq 8, output `ha log docs/specs/example 8`
- AC-3 `criterion_error` (executor): seq 9, no stored output
- `task_open` (executor)
```

A failed `ha check` prints, for each target that did not meet its expectation, `output for <target>: ha log <goal> <seq>`, or `no stored output` with the seq, also when a later target is refused or cannot be recorded. The per-target result lines and exit codes are unchanged. None of this changes judgment, `status/v1`, the completion record or `ha log`; the human output never contains command output.

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

`ha` uses the base commit in three places: the baseline, which runs `change` conditions on a temporary worktree of the base commit with check paths copied from HEAD (up to `/7` the condition's own, under `/8` the sorted union of every marked change condition's); `out_of_scope`, the files changed since base outside `바꿀 수 있는 경로`; and `test_definition_changed`. Checks themselves judge the tree they run in and never read the base commit or other history, so the same check gives the baseline's violation and the later pass. The exception is a condition about history or refs itself, such as a tag's type, whose check reads only the range and refs the condition names. See [bundle.md](bundle.md#check-inputs).

## Freshness

A check result counts only while all of these hold: the tree is clean now and was clean when it ran; the code outside the bundle documents equals its commit; the goal documents and the condition's command are unchanged. A baseline counts while it ran on the start's base commit, its copied check files equal HEAD's, and the goal documents and command are unchanged; up to `/7` those are the condition's own check paths, and under `/8` its overlay must be exactly the sorted union of the check paths of the currently marked change conditions. Committing only bundle documents keeps results fresh.

Under `run-rules/2` and `run-rules/3`, the bundle files of other goals count as bundle documents too: `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, and `tasks/` of every other directory with a `SPEC.md` under the same parent. That holds for freshness, the clean tree, scope, and the completion record, so another goal on the same branch committing its progress leaves these results fresh. Their `SPEC.md`, linked documents, and `checks/` stay code. When a check of this goal reads other goals' bundle files, the plan declares it in `## 범위` with ``다른 목표 실행 묶음: `검사 입력` ``; with the declaration those files stay in the code state. `ha` reads the declaration from the plan as it is when it computes or records. Under `run-rules/1` every other goal's file is code.

Every status output states assurance `local`: an agent with the same user permissions can change checks and records.

## Final head

[Final candidate evidence](bundle.md#final-candidate-evidence) says how conditions that need the same full regression share it.

A completion record covers its commit while it stays valid and `ha status` names it: among other conditions, the code outside the bundle documents equals HEAD and the goal documents and required commands are unchanged. When the user asks, after completion, to apply remote review or pull request comments, record that request with `ha note <goal> reopen --quote "…"`. After the fixes are committed, a code change leaves earlier `check` records stale, and a baseline stays fresh while it ran on the start's base commit and its copied check files equal HEAD's (under `/8`, while its overlay is also still exactly the union of the marked conditions' check paths). Re-record the stale required checks on the final head and run `ha done` again before the head is sent or merged. Comments without such a request do not reopen the goal.
