# ha — Seal Core commands

`ha` runs verification commands without a shell, appends `runs.jsonl`, and computes the goal state from the record, the bundle, and git only. The same inputs always give the same state; the current time is never used.

This reference describes release v0.1.3: ha 0.1.3 and Seal 0.1.9. Existing `/1`, `/2` and `/3` goals keep their saved rules; installation does not migrate their records or reset usage or time limits.

## Core compatibility

Resolve the actual executable selected for `ha` to an absolute path. Run that exact executable with `capabilities --format json`, and use the same path for later Core commands. Check again when its path or contents change. A successful version command is not compatibility evidence; neither is a response obtained from a different executable. Do not cache approval across such changes.

The response uses `schema: ha-capabilities/v1`, `ha_version`, `supported_rules`, `default_rules`, `formats` and `features`. These required fields must have the types and values described in the [Core capabilities contract](https://github.com/jgoneit/jaekit/blob/v0.1.3/contracts/core-capabilities.md). Missing or malformed information, an unsupported schema, query failure, missing executable or unmet requirements mean stop before writing any record. Identify the executable and the reason; never fall back to older rules, install tools or alter records automatically.

For a **new start**, require all of the following:

- `default_rules` is `run-rules/3`, and `supported_rules` includes it. A different default is unsupported by this skill even if `/3` appears in the supported list.
- `formats.criteria` includes `legacy` and `nested/1`.
- `formats.check_declaration`, `formats.check_result` and `formats.check_evidence` include `check-declaration/v1`, `check-result/v1` and `check-evidence/v1`, respectively.
- `features` includes `estimate/v1`, `dirty-preflight/v1`, `baseline-safe-copy/v1`, `structured-change-results/v1` and `budget-change/v1`.

For an **existing goal**, read its saved start rule and selected SPEC format without changing them. Seal 0.1.9 understands only `run-rules/1`, `run-rules/2` and `run-rules/3`; refuse any other saved rule even if Core lists it as supported. Require that rule in `supported_rules` and the selected format (`legacy` when absent) in `formats.criteria`. Resume and check operations require `baseline-safe-copy/v1` and `dirty-preflight/v1`; under `/3` they also require the three structured-result formats and `structured-change-results/v1`. A budget operation instead requires `/3`, its three structured-result formats and `budget-change/v1`; `/1` and `/2` do not support that operation. A different default rule does not invalidate an otherwise supported existing goal. Do not require `/3` declarations for legacy goals or move ordinary maintain, manual and task checks to the structured contract.

When Python 3 is already available, the optional [check_core.py](../scripts/check_core.py) helper checks this same contract. From the project directory, use `python3 <skill>/scripts/check_core.py --ha <actual executable> --operation start`, substituting the actual skill path. For an existing goal, choose `--operation resume`, `check` or `budget` and add `--goal <existing goal>`. It reports `compatible`, `reason`, `executable`, `sha256`, `ha_version`, `operation`, `rules` and `missing`: exit 0 means compatible, 2 means refused and 64 means invalid arguments. Use the successful response's absolute `executable` afterward. Python is not required: the agent can read the actual Core JSON and apply the contract directly. Never treat missing Python as missing Core support.

This checks source contracts and executable support. Whether installed Codex or Claude models follow the instructions in practice is a separate host evaluation; capability output does not establish that result. Spec document writing requires neither Core nor this query.

## Commands

| Command | Use |
| --- | --- |
| `ha capabilities [--format json]` | Read-only support information without a goal or record; JSON is the default |
| `ha lint <goal>` | Lists goal document and bundle format problems, one per line: code, file:line, detail. Exit 1 when any exist |
| `ha estimate <goal> [AC-n \| EX-n \| T001 …] [--baseline] [--format md\|json]` | Reads the PLAN lower bound, limit and shortfall, plus default and selected runs. Executes nothing; does not change records or budgets |
| `ha start <goal> --request "<quote>" --skill <SKILL.md> [--host-name …] [--host-version …] [--model …]` | Records the start once per goal. Refuses lint problems, a second start, and a PLAN limit below the required minimum before writing a start record |
| `ha check <goal> [AC-n \| EX-n \| T001 …]` | Runs the conditions' commands at HEAD and records each result. With no targets it runs every required `change` and `maintain` condition |
| `ha check <goal> --baseline [AC-n …]` | Runs `change` conditions on a temporary worktree of the base commit with their check paths copied from HEAD. Expected result: `fail_as_expected` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote "…" --context "…"] [--reason "…"] [--interpretation "…"]` | `/3` only: changes the current window's total run limit while retaining usage; quote and context for increases, reason for decreases |
| `ha note <goal> confirm <AC-n \| scope:<path> \| tests:<path> \| spec> --quote "…"` | Records a quoted user confirmation, bound to the current content. It loses effect if that content changes |
| `ha note <goal> reopen --quote "…"` | Records a rework request; voids the completion record and opens a new budget window |
| `ha note <goal> input --quote "…"` | Records any other user instruction or answer |
| `ha note <goal> block --cause <cause> [--quote "…"]` / `unblock` | Records a stop that cannot be fixed by retrying, and its end |
| `ha status <goal> [--format md\|json]` | Prints the computed state |
| `ha done <goal> [--format md\|json]` | Computes the state and records it. When the state is `complete`, the line is the completion record |
| `ha log <goal> <seq> [--tail N]` | Shows the local output of one run. Output never leaves this clone |

`ha check` exits 0 when every result is as expected and 1 otherwise. Other exits: 64 usage, 65 refused, 66 unsupported rules version, 70 internal error. `ha status`, `ha done`, `ha check`, and `ha note` exit 66 for a goal started under rules this `ha` does not compute, and record nothing.

## Before execution

The minimum is twice the required change count plus the required maintain count; manual conditions contribute zero. Default baseline selection also includes optional change rows. Explicit optional AC, EX and task commands add runs, and repeated targets count as repeated executions. Estimates use the current PLAN limit, not an existing run's remaining budget. They exclude retries, preparation and manual time and guarantee neither total time nor completion. `estimate` exits 1 for a shortfall or lint problems; `start` exits 65 for an insufficient limit. Neither automatically raises a limit or changes an existing budget window.

Before each check, baseline or task process, `ha` inspects the working tree using the goal's exclusions. Dirty paths refuse that target and the remaining targets with exit 65; failed Git inspection exits 70. Diagnostics list repository-relative paths with tracked/untracked and observed symlink details. Review the specific path; do not automatically ignore or revert it. A refused target creates no check record, run usage or output file. Already executed results and usage remain, including errors and results that later become stale. This admission rule applies to future executions for all supported rule versions; historical records retain their meaning. Dirty `start` remains a warning.

## Structured change results

New starts use `run-rules/3`. Existing `/1` and `/2` goals retain their result semantics. Ordinary maintain, manual and task checks keep their roles. A `/3` change condition maps to a tracked JSON declaration in PLAN's `## 결과 계약` table (`ID | 선언 경로`). Include the declaration and its producer files in the condition's check paths. These tracked regular files must match HEAD byte for byte, even within an excluded bundle path; symlink inputs are refused.

The `check-declaration/v1` object contains `producer`, `producer_version`, `producer_paths` and required `targets` (`id`, expected `violation`). The producer writes `check-result/v1` to `HA_EVIDENCE_PATH`, echoing `HA_EVIDENCE_INVOCATION` and `HA_EVIDENCE_DECLARATION_DIGEST`. It reports `attempts_complete: true` and every observation for each required target, with consecutive `attempt` numbers starting at 1 and `status` of `pass`, `violation`, `error` or `skip`. The reference producer makes one attempt; integrations with retries must preserve the full history, with at most 4096 observations. Missing, duplicated or reversed attempt numbers are inconclusive. Pass and violation on the same target retain a `fail` with `inconsistent_attempts`, even when the process exits 0. No report, mismatched identity, unsupported values, skipped targets or contradictory process exits are positive evidence.

Current success needs exit 0 and every target passing. Expected baseline failure needs nonzero exit and a declared violation, every required target observed, and no environment error or unintended assertion failure. Missing implementation can be a violation directly observed by a runnable check; import, collection, compilation and browser startup errors are not such evidence. `unknown` shares the error rerun limit. Later positive results do not erase adverse results on the same inputs, including observed assertion failures retained within an error or inconclusive report.

The [public contract and executable reference](https://github.com/jgoneit/jaekit/blob/v0.1.3/contracts/check-result.md) describe the JSON fields and `tools/check-result-reference.py`. The Python reference reads actual files and reports observations, without installing runner adapters. A generic script's nonzero exit alone is unsupported evidence. The compact `check-evidence/v1` summary preserves the classification without raw logs or file contents. An output-file failure following an actual attempt records `error` with `process_output` and retains its usage when record storage is writable; `output_path` and `output_digest` may be empty. Record-storage failure itself is not reported as successful recording, and a failed write can have uncertain application. Declaration, producer and target changes invalidate old evidence; missing raw logs or changing the current time do not. These are executor-authored local claims.

Release v0.1.3 includes the optional Python reference producer and a synthetic file-observation example for both direct and Homebrew installations. Follow [Get the reference check](https://github.com/jgoneit/jaekit/blob/v0.1.3/guides/INSTALL.en.md#get-the-reference-check) to copy them into the project without a development checkout. The reference needs Python 3; choosing it is optional and does not make Python or any particular runner a prerequisite for Seal.

## Budget changes

Use the current `budget.window_start`, `revision` and `runs_limit` from `ha status --format json` as `--window`, `--revision` and `--from`. All four numeric arguments, including the new `--runs`, are required positive integers. The mutation compares the latest window and budget revision under the record lock. Concurrent checks retain their actual usage; a concurrent budget change or reopen produces a conflict instead of an overwrite. Read the resulting state before deciding on another change.

This sets a total limit: 48 uses out of 50 changed to 100 leaves 52, while lowering it to 40 leaves zero and an excess of 8. It changes neither the time limit nor the window's existing usage. Editing PLAN alone has no effect. Increase quotes need the request context supporting that specific limit; decreases need a reason. Optional `--interpretation` is separate from the user quote. Unverified host provenance remains an executor claim.

A change appends `budget_change`; it does not consume a check run, reopen, unblock or reset errors. It advances the last-record time used for elapsed time. Empty/same-value requests and stale window/revision/previous-limit inputs record no change. Time-limit changes are unsupported. A completed window needs explicit rework and `reopen`. `/3` reopens with the latest run limit and original elapsed limit; `/1` and `/2` retain the original start limit. Checks belong to the window in which their records are appended, even when execution straddles a reopen.

Budget exits: 0 success, 64 invalid arguments, 65 refused input or state, 66 legacy/unsupported rules, 70 internal failure or uncertain application. A write/sync/close failure can leave an applied change; inspect the record instead of assuming success or non-application and automatically retrying. No execution hard cap or concurrent slot reservation is provided.

Status exposes budget revision, remaining/excess runs, reached state and change history. `usage` separates total attempts, check/baseline kinds, errors/unknowns, current/stale inputs and valid positive evidence. These are overlapping axes, not numbers to sum. Baselines and unsuccessful attempts still count as use.

## States

| State | Exit | Meaning |
| --- | --- | --- |
| `complete` | 0 | A valid completion record exists, or every required condition is satisfied with no remaining reasons; only `ha done` turns the latter into a completion record |
| `incomplete` | 1 | Something the executor can fix remains |
| `needs_user` | 2 | Only items that need the user remain |
| `blocked` | 3 | A `block` note has no later `unblock` |
| `budget_exhausted` | 4 | The budget window passed its run count or elapsed time, and under `run-rules/2` or `run-rules/3` another reason remains as well |

A goal is computed under the rules its start record names (`rules:` in `ha status`). Under `run-rules/2` and `run-rules/3`, a goal whose required conditions are all satisfied and whose only remaining reason is `budget_exceeded` is `complete`: `ha done` records the evidence already gathered, and the status still shows the budget as exceeded. That is not leave to keep working past the budget; more checks or work past it still need the user's words. `/3` can record a same-window run-limit increase with `ha budget`; an explicit new window uses `reopen`. `/1` and `/2` retain the quoted `reopen` behavior. Under `run-rules/1` the same goal is `budget_exhausted`.

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

## Freshness

A check result counts only while all of these hold: the tree is clean now and was clean when it ran; the code outside the bundle documents equals its commit; the goal documents and the condition's command are unchanged. A baseline counts while it ran on the start's base commit, its check files equal HEAD's, and the goal documents and command are unchanged. Committing only bundle documents keeps results fresh.

Under `run-rules/2` and `run-rules/3`, the bundle files of other goals count as bundle documents too: `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, and `tasks/` of every other directory with a `SPEC.md` under the same parent. That holds for freshness, the clean tree, scope, and the completion record, so another goal on the same branch committing its progress leaves these results fresh. Their `SPEC.md`, linked documents, and `checks/` stay code. When a check of this goal reads other goals' bundle files, the plan declares it in `## 범위` with ``다른 목표 실행 묶음: `검사 입력` ``; with the declaration those files stay in the code state. `ha` reads the declaration from the plan as it is when it computes or records. Under `run-rules/1` every other goal's file is code.

Every status output states assurance `local`: an agent with the same user permissions can change checks and records.
