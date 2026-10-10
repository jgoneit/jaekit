# ha — Seal Core commands

`ha` runs verification commands without a shell, appends `runs.jsonl`, and computes the goal state from the record, the bundle, and git only. The same inputs always give the same state; the current time is never used.

## Commands

| Command | Use |
| --- | --- |
| `ha lint <goal>` | Lists goal document and bundle format problems, one per line: code, file:line, detail. Exit 1 when any exist |
| `ha estimate <goal> [AC-n \| EX-n \| T001 …] [--baseline] [--format md\|json]` | Reads the PLAN lower bound, limit and shortfall, plus default and selected runs. Executes nothing; does not change records or budgets |
| `ha start <goal> --request "<quote>" --skill <SKILL.md> [--host-name …] [--host-version …] [--model …]` | Records the start once per goal. Refuses lint problems, a second start, and a PLAN limit below the required minimum before writing a start record |
| `ha check <goal> [AC-n \| EX-n \| T001 …]` | Runs the conditions' commands at HEAD and records each result. With no targets it runs every required `change` and `maintain` condition |
| `ha check <goal> --baseline [AC-n …]` | Runs `change` conditions on a temporary worktree of the base commit with their check paths copied from HEAD. Expected result: `fail_as_expected` |
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

Before each check, baseline or task process, `ha` inspects the working tree using the goal's exclusions. Dirty paths refuse that target and the remaining targets with exit 65; failed Git inspection exits 70. Diagnostics list repository-relative paths with tracked/untracked and observed symlink details. Review the specific path; do not automatically ignore or revert it. A refused target creates no check record, run usage or output file. Already executed results and usage remain, including errors and results that later become stale. This admission rule applies to future executions for both supported rule versions; historical records retain their meaning. Dirty `start` remains a warning.

## States

| State | Exit | Meaning |
| --- | --- | --- |
| `complete` | 0 | A valid completion record exists, or every required condition is satisfied with no remaining reasons; only `ha done` turns the latter into a completion record |
| `incomplete` | 1 | Something the executor can fix remains |
| `needs_user` | 2 | Only items that need the user remain |
| `blocked` | 3 | A `block` note has no later `unblock` |
| `budget_exhausted` | 4 | The budget window passed its run count or elapsed time, and under `run-rules/2` another reason remains as well |

A goal is computed under the rules its start record names (`rules:` in `ha status`). Under `run-rules/2`, a goal whose required conditions are all satisfied and whose only remaining reason is `budget_exceeded` is `complete`: `ha done` records the evidence already gathered, and the status still shows the budget as exceeded. That is not leave to keep working past the budget; more checks or work past it still need the user's words, recorded with `reopen`. Under `run-rules/1` the same goal is `budget_exhausted`.

## Reasons

| Reason | Who resolves it | Typical cause |
| --- | --- | --- |
| `criterion_missing` | executor | No fresh check result for the condition |
| `criterion_flaky` | executor | Pass and fail on the same code |
| `criterion_failed` | executor | A fresh check failed |
| `criterion_error` | executor | The latest fresh result is an error within the rerun limit |
| `baseline_missing` | executor | A `change` condition has no fresh baseline |
| `baseline_unexpected_pass` | executor | The check already passes on the base commit, so it does not test the change |
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

Under `run-rules/2`, the bundle files of other goals count as bundle documents too: `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, and `tasks/` of every other directory with a `SPEC.md` under the same parent. That holds for freshness, the clean tree, scope, and the completion record, so another goal on the same branch committing its progress leaves these results fresh. Their `SPEC.md`, linked documents, and `checks/` stay code. When a check of this goal reads other goals' bundle files, the plan declares it in `## 범위` with ``다른 목표 실행 묶음: `검사 입력` ``; with the declaration those files stay in the code state. `ha` reads the declaration from the plan as it is when it computes or records. Under `run-rules/1` every other goal's file is code.

Every status output states assurance `local`: an agent with the same user permissions can change checks and records.
