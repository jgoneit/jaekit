# Contract: Run record (`run/v1`) and completion judgment

[한국어](run-record.md) · **English**

> This is a translation of the [Korean original](run-record.md). Where the two differ, the Korean original decides.

This document is the canonical definition of the following.
- The line format of the run record `runs.jsonl`
- The rules that compute condition satisfaction and status from that record
- The completion record

The execution bundle is in [bundle.md](bundle.en.md). Post-completion findings are linked through an [explicit record extension](findings.en.md). Data that uses this extension also contains `run-finding/v1` lines; it does not change existing `run/v1` lines or the stored run rules.

Only `ha` appends to the run record. The agent does not write the record directly; it calls `ha` commands. An agent with the same user permissions can still edit the file, so the assurance level of this record does not exceed `local`.

## 1. `ha` commands

| Command | What it does | Record |
| --- | --- | --- |
| `ha lint <goal>` | Checks the goal document format (condition IDs, the `Assurance:` line), the execution bundle structure, that every condition ID maps to the condition table, the two-way mapping between conditions and tasks, cycles, that referenced files exist, and missing verification methods. Error codes are in [bundle.md](bundle.en.md) §8. | None |
| `ha estimate <goal> [AC-n \| EX-n \| T001 ...] [--baseline] [--format md\|json]` | Computes, read-only, the current PLAN's required minimum run count, the limit and the shortfall, and the run counts of the default selection and of the given targets (§1.1). When PLAN's run limit is automatic, it also shows the computed value, the pre-approved limit and whether approval is needed (§1.5). | None |
| `ha start <goal> --request <인용> [--skill <SKILL.md> [--check-skill]] [--rules run-rules/4 \| run-rules/5] [--test-authority <해석>] [--host-name …] [--host-version …] [--model …]` | Records the start. Without `--rules`, the default rules are `run-rules/3` (§4.6). `--test-authority` is accepted only with `run-rules/4` and `run-rules/5` (§4.7). `run-rules/5` is in §4.8. If one already exists, it refuses. When a new start is needed, the user creates a new bundle. It also refuses before creating the record when there is a lint error or a PLAN limit smaller than the required minimum run count. A dirty start is allowed with a warning, as before. With `--skill`, `ha` reads the name and version from that file's front matter and computes the digest of the Skill directory (§2.1). When PLAN's run limit is automatic, it computes the limit, and refuses before creating the record if the limit exceeds the pre-approved limit or that setting cannot be used (§1.5). If the skill's Core requirements declaration requires something this executable does not support, or is corrupt, it refuses before creating the record (§1.6). | `start` |
| `ha check <goal> [AC-n \| T00n ...]` | Runs the verification commands of the given conditions or tasks. With no target, it runs all required conditions (integrated verification). | `check` |
| `ha check <goal> --baseline [AC-n ...]` | Overlays only the check paths of `change` conditions onto a temporary worktree of the base commit and runs them. | `baseline` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote <인용> --context <문맥>] [--reason <이유>] [--interpretation <해석>]` | Changes the total run limit of the current window for `/3`, `/4` and `/5`. All positive integer arguments are required; an increase needs a quote and context, and a decrease needs a reason (§5.2). `/4` and `/5` accept an increase within the pre-approved limit with a reason alone (§4.7). | `budget_change` |
| `ha budget <goal> --preapprove N --window W --revision R --quote <인용> --context <문맥>` | `/4` and `/5` only: records the pre-approved limit of that window. The limit itself stays the same (§4.7). | `budget_change` |
| `ha note <goal> <block \| unblock \| confirm \| reopen \| input> [대상] --quote <인용> [--cause <원인>]` | Records a block, its resolution, a user confirmation, the user's rework request, or other user input. The user's words are quoted. `block` needs `--cause`, and `confirm` needs a subject (§2.4) and a quote. | `note` |
| `ha note <goal> <test-change \| wait \| resume \| recover \| allow-recovery> …` | `/4` and `/5` only: records the grounds for a test change, the start and end of a wait for the user, and recovery from the error limit and the user's permission for it (§4.7). Goals under `/1`, `/2` and `/3` are refused with 65. | `note` |
| `ha note <goal> review --gate <file>` | `/5` only: records the digest of the `review-gate/v1` file that Spar wrote and the summary the judgment uses (§4.8). Goals under `/1` to `/4` are refused with 65. A file that cannot be used is refused with 65 and its reason, and nothing is recorded. | `note` |
| `ha status <goal> [--format md\|json]` | Computes the status from the record, the bundle and git, and prints it (§4). | None |
| `ha done <goal>` | Claims completion. It does the same computation as `ha status` and records the result. When the result is `complete`, this line is the **completion record** (§5). The exit code is the same as for `ha status`. | `done` |
| `ha log <goal> <seq> [--tail N]` | Shows the raw local output of that run. It is for reading locally. | None |
| `ha timing <goal> [--format md\|json]` | Only reads the record and shows, for each run, the time limit, the elapsed time of the limited interval, and CPU (§1.2). It runs no check and is not used for judgment or budget. | None |
| `ha report <goal> [--lang ko\|en]` | Shows a five-line summary for people, a stamp to write in `PROGRESS.md` `## 현재`, the evidence per condition, the completion coverage, and where the output of unmet conditions is (§1.3). It only reads the same computation as `ha status`. | None |
| `ha doctor <goal>` | Runs the check commands written in PLAN's optional section `## 실행 환경` (execution environment) and shows `사용 가능` (available) or `사용 불가` (unavailable) for each tool (§1.4, [bundle.md](bundle.en.md) §2.6). It has nothing to do with judgment or budget. | None |
| `ha cost <goal> [--series <folder>]... [--format md\|json] [--lang ko\|en]` | Gathers from the records and shows what one goal cost in verification and review (§1.7). It reads only the Spar series folders a person points to, and writes nothing. | None |
| `ha history <goal> [add \| current \| list \| show] …` | Registers and queries the optional [decision history](decision-history.en.md). It writes to a separate history in the common Git directory, and status computation does not read this history. | None (outside `runs.jsonl`) |

`ha` runs verification commands as argv, without a shell. Each command has a time limit (900 seconds by default). When the time limit is reached, it terminates the command's process group on Unix, and on the native Windows candidate, the kill-on-close Job Object assigned before the run. On Windows it finalizes the output after confirming that ordinary child processes have ended; if process control or the confirmation of termination fails, it produces no pass evidence and no final output digest. A cleanup failure after the start is also returned as an error within a bounded time. This is not a guarantee that arbitrary external services or processes run with separate permissions are isolated. `check`, `baseline`, `note`, `start` and `budget_change` take `ha/lock` in the git directory before appending to the record. Unix uses `flock`, and the native Windows candidate uses an exclusive `LockFileEx` lock on the same file; even when the OS lock is released because a process ended, a corrupted record is refused by the existing chain verification. So several `ha` processes appending at once do not break the chain.

**Exit codes**
- `ha check`: 0 when every result is as expected (`pass`; for a baseline, `fail_as_expected`), otherwise 1
- `ha lint`: 0 with no errors, 1 with any
- `ha estimate`: 0 when the PLAN limit accommodates the required minimum, 1 when it falls short or there is a lint error. With an automatic budget, 0 when the computed value is at or below the pre-approved limit, 1 when user approval is needed, 65 when the pre-approved limit setting cannot be used (§1.5)
- `ha status`, `ha done`: §4.3
- `ha timing`: 0 on success. A broken record is 65 (refusal) and nothing is shown. 70 when the view cannot be written to the output
- `ha report`: 0 whenever it prints the report, whatever the status. 64 for a usage error (including `--lang` other than `ko` or `en`), 65 for a missing goal directory, 66 for unsupported rules, 70 when Git cannot compare base and HEAD (the five-line summary is printed, §1.3), for an internal error, or for an output failure. A broken record is not refused; it shows the computed result that has `record_integrity`
- `ha doctor`: 0 when every tool is `사용 가능` or there is no section or no item, 1 when any is `사용 불가`, 65 for an input error in the section or a missing PLAN.md (no check is run), 64 for a usage error, 70 when it cannot control or confirm a process. It does not use the record for judgment, so this is the same before the start, for unsupported rules, and for a goal with a broken record
- `ha cost`: 0 when it prints the summary. 64 for a usage error (including `--format` or `--lang` values, and two `--series` pointing to the same folder), 65 for a missing goal directory, a broken record or a series that cannot be used (no summary is printed, §1.7), 66 for unsupported rules, 70 for an internal error or an output failure
- `ha budget`: 0 on success, 64 for invalid arguments, 65 for refusals on value, window, revision, completed state and the like (including `--preapprove` on a `/3` goal), 66 for old or unsupported rules, 70 for an internal error or uncertainty whether the write was applied
- All commands: 64 usage error, 65 refusal (not started, already started, lint error, broken record, confirmation conditions not met, dirty verification, start budget too small, user approval needed for an automatic budget, pre-approved limit setting error, skill Core requirements unsupported or declaration corrupt, unusable review gate file), 66 unsupported rules version, 70 internal error

Raw output is kept only in `ha/runs/<goal-key>/<seq>.log` in the git directory. `<goal-key>` is the repository-relative goal path, URL-path-escaped (for example `docs%2Fspecs%2Fempty-input`). This is because seq values overlap when there are several goals. The run record and the output of `ha status` and `ha done` contain results, reasons, digests, local paths, and explicitly recorded user confirmation and budget change information. They do not contain raw output. That is because output may contain secrets, and status output is preserved in the completion report. Whether goal documents and the record are tracked by Git follows the project's policy. Raw output is viewed only with `ha log`.

### 1.1 Diagnostics before a run

**Working state.** Just before starting each target's process, `check`, `baseline` and task verification check the Git status, taking into account the excluded paths of the applicable rules. If there is a change that is not excluded, they refuse with exit 65. If the Git status cannot be read, the exit is 70 and nothing runs. No check record, run count or output file is created for a refused target. If only earlier targets among several ran, the numbers of run and not-run targets are printed, and the earlier results are kept as they are. Errors or stale results that arise after a run also remain in the record and the usage.

The diagnostics distinguish repository-relative paths, tracked/untracked, and whether a symlink was observed, and also show the paths of deletions and renames and the submodule state. For an untracked path they may suggest reviewing that path's ignore rules, but they do not change them automatically. They do not suggest that a tracked change can be resolved by adding an ignore. This is not a feature that atomically prevents file changes during a run, and the freshness judgment of §3 still applies to current completion evidence.

This refusal applies to checks to be run from now on in supported `/1`, `/2` and `/3` goals. It does not delete or reinterpret the results and usage of past `tree_clean: false` records. The dirty warning at the start and the existing Git ignore and per-rules exclusion of run documents also stay.

**Run count.** The required minimum of `ha estimate` is `필수 change × 2 + 필수 maintain`. manual counts as 0 runs, and 34 change conditions make 68 runs. It is a lower bound for new runs without failures, so it does not guarantee retries, environment preparation, manual confirmation time, the actual total time, or completion. It runs no check or environment probe, and does not change the record, the budget window or working files.

The default `check` selects the required change/maintain conditions, and the default `--baseline` selects every change condition in the condition table (including optional ACs and EXs). `estimate` shows this default run count together with the additional runs above the required lower bound. Explicit targets are selected in the same order as an actual `check`. Optional ACs and EXs are additional runs, each task command is one run, and duplicate targets are counted repeatedly, as in an actual run. Explicit targets of `--baseline` may only be change conditions. A default selection with only manual conditions estimates 0, and the no-target error of an actual `check` does not change.

The limit is **the whole limit of the current PLAN** (the computed value of §1.5 when PLAN's run limit is automatic). It is not the remaining budget of a goal in progress. When the limit is smaller than the required lower bound, a new `start` prints the minimum, the limit and the shortfall, and refuses with exit 65 before creating the record. An equal value can start. It does not raise the limit automatically for optional checks or retry headroom. The limits and resume windows of existing goals stay as in §5.2, and no new budget restriction is applied to checks in progress.

The JSON output is `schema: estimate/v1` and provides the following.

| Field | Meaning |
| --- | --- |
| `goal`, `budget_source` | Repository-relative goal path; `PLAN.md` |
| `required_change`, `required_maintain`, `minimum_runs` | Required counts by kind, and the minimum run count |
| `plan_runs_limit`, `shortfall`, `headroom` | PLAN limit, shortfall, and headroom above the lower bound. Shortfall and headroom are never negative |
| `default_check_runs`, `default_baseline_runs`, `default_additional_runs` | Run counts of the two default commands, and their total minus the lower bound |
| `selected` | `mode`, `targets` with order and duplicates preserved, `runs`, `additional_runs`, `optional_runs`, `task_runs`, `repeated_required_runs` |
| `limitation` | A statement of the estimate's scope and that it does not change existing budget windows |

`selected.additional_runs` is the sum of optional targets, task targets and duplicate required targets. The run that first selects a required target in the same phase is already included in the lower bound, so it is not additional. The combined cost of several CLI calls is not tracked.

### 1.2 Timing view (`ha timing`)

For each `check` and `baseline` line of the record, `ha timing <goal>` shows the sequence number, the target (a condition ID, or a task and its command number), the kind (`check` or `baseline`), the result, `duration_ms`, the applied limit, the elapsed time of the limited interval and its proximity to the limit, whether the limit fired, and CPU. The values mean what `timing` means in §2.2. This view runs no check and does not change the record, logs, lock or working tree. Status computation, freshness, usage and budget do not read these values. If the record is broken, it shows nothing and exits 65.

`--format json` is one `timing/v1` document.

| Field | Meaning |
| --- | --- |
| `schema` | `"timing/v1"` |
| `goal` | Repository-relative goal directory. It is relative even when the command is given an absolute path |
| `runs[]` | Runs in record order. `seq`, `target`, `kind`, `result`, `duration_ms`, `limit_ms`, `limit_elapsed_ms`, `limit_use_permille`, `limit_reached`, `cpu`, `unavailable` |
| `total_duration_ms` | Sum of `duration_ms` of the listed runs |
| `total_meaning` | What the total means. Runs can overlap, and this sum is not the goal's elapsed time or budget |

- Units are integer milliseconds. `limit_use_permille` is the integer quotient (fraction discarded) of `limit_elapsed_ms × 1000 ÷ limit_ms`. If either value is missing, it is `null`. If the quotient exceeds a 64-bit integer, it is `null` and `invalid`.
- When a value is missing, that field is `null` and `unavailable` holds the reason. `not_recorded` means the value is not in the line (for example a line recorded before this field existed). `not_measured` means a recorded `null`, that is, the limit timer was not started. `invalid` means the type or value does not fit. It never fills in a value such as 0 or 900 seconds, and never infers one from the rules version.
- When one value is corrupt, only that value is `invalid`. The other values of the same line and other lines are shown as they are. Without the `timing` field, all five values are `not_recorded`, and a key missing from the object is also `not_recorded`. When `timing` is `null` or not an object, all five values are `invalid`.
- `cpu` reads and shows the record's `{ "status", "user_ms", "system_ms", "scope", "source" }` key by key. Keys in the record appear as they are; when a key's value is corrupt, only that key is `null` and `cpu.<키>` in `unavailable` is `invalid`. The other keys of the same CPU are shown as they are. When `status` is `measured`, all five keys appear, and a key missing from the record is also `null` and `invalid`. With `unavailable` or `failed`, only the keys in the record appear, and if that status has times, those times are `null` and `invalid`. When `status` is not one of the three values, it is `null` and `invalid`. `scope` and `source` show only the fixed codes of §2.2, and any other value is `null` and `invalid`. So arbitrary strings in the record never appear in the view. When `cpu` is not an object, `cpu` is `null` and `cpu` in `unavailable` is `invalid`.

The human-readable format shows the same values in one table, and adds the sum of run times and a sentence saying that the sum is not the goal's elapsed time or budget. Whatever the status, the CPU column shows the keys the JSON shows, with the same values or the same unavailable marks.

### 1.3 Human-readable report (`ha report`)

`ha report <goal> [--lang ko|en]` shows one goal in a form for people. It reads the same record, execution bundle, goal documents and Git as `ha status`, with the same computation (§4), and uses only its result. It does not judge condition satisfaction, status, reasons or the validity of completion records separately, so the report never states a status different from `ha status`. It adds no line to the record, does not take `ha/lock`, and does not change `.git/ha`, the execution bundle, the working tree, the index or refs. It does not read the current time: with the same record, bundle, Git and `--lang`, the output is the same. `status/v1` and the output of other commands do not change because of this command.

`--lang` is `ko` or `en`, and `en` when absent. The language changes only labels, headings and explanatory sentences. Reason codes, classifications (`executor`, `user`), status names, condition IDs, kinds, seq, hash and commit, cause codes, the `ha log`, `ha start`, `ha done` and `ha lint` commands, `none`, `no stored output`, `local` and `## 현재` are the same in both languages.

**Five-line summary.** These are the first five lines of the output, with a fixed order and fixed labels. In English they are `Changed`, `Checked`, `To review`, `Remaining limits` and `Next step`; in Korean, `바뀐 것`, `확인한 것`, `직접 볼 것`, `남은 한계` and `다음 행동`. Each label is followed by `: `.

| Label | Content |
| --- | --- |
| `Changed` / `바뀐 것` | The number of files changed outside the execution bundle documents from the start record's base commit to HEAD (the length of the same list as `out_of_scope` in `ha status`), and the number of non-merge commits that changed those paths. Before the start, the fact that there is no base; when Git cannot compare, that fact (below) |
| `Checked` / `확인한 것` | The number of required conditions and how many of them are satisfied (the number of `change` and `maintain` conditions satisfied by check records, and the number of `manual` conditions satisfied by user confirmation), and the number of all `change` conditions in the condition table and how many of them have a fresh `fail_as_expected` baseline. That baseline must be one that status computation accepts as evidence (the result and exit code match, and for a `change` condition with structured results, the stored evidence summary passes verification) |
| `To review` / `직접 볼 것` | The complete set, in this order: remaining reasons classified `user` and their targets, `manual` conditions satisfied by a quoted confirmation and the seq of that confirmation, test changes exempted on the executor's grounds (§4.7), test changes marked `--weakened`, verification commands changed after the start (`changes.command_changed`), and the number of unresolved post-completion findings. `none` / `없음` when there are none |
| `Remaining limits` / `남은 한계` | Always the assurance level `local` and the check author, the executor. When they apply, it adds unsatisfied optional conditions, an exceeded budget (usage and limit of runs and elapsed time), and the fact that no completion record covers the current HEAD |
| `Next step` / `다음 행동` | One per status: before the start, `ha start <goal>` (`ha lint <goal>` first when there is a lint problem); any other `incomplete`, the first reason classified `executor` and its target; `needs_user`, all reasons classified `user` and their targets; `complete` without a completion record, `ha done <goal>`; with a valid completion record, its seq and the fact that nothing is left to do; `blocked`, the seq and cause of the last `block`; `budget_exhausted`, the user's budget decision with usage and limit |

A reason's target is the condition ID for a condition reason (`` AC-2 `criterion_failed` ``), and for a goal-level reason, the task or path that the reason points to (`` `task_open` (T002) ``, `` `out_of_scope` (lib/other.txt) ``). A line break in a summary value is written as `\n`, so that each line stays one line.

**When Git cannot compare base and HEAD.** When Git cannot read that range, as in a shallow clone without the base commit, `ha status`, as now, cannot compute and exits 70. `ha report` shows only the five lines and exits 70: `Changed` states that the comparison is impossible, `Checked` and `To review` state that they could not be computed, `Remaining limits` states, after `local` and the check author, that the remaining limits could not be computed, and `Next step` gives how to fetch the history that contains the base (for example `git fetch --unshallow`) and the command to run again. Details are not shown. Any single failure to read the range is this case: both when the computation shared with `ha status` fails while reading the changed files, and when `ha report` fails while reading the file count and commit count. A failed read is not retried.

```text
Changed: Git cannot compare base 3c1f0a9e7b25 with HEAD 9a8b7c6d5e4f
Checked: not computed; Git cannot compare base 3c1f0a9e7b25 with HEAD 9a8b7c6d5e4f
To review: not computed; Git cannot compare base 3c1f0a9e7b25 with HEAD 9a8b7c6d5e4f
Remaining limits: assurance local; check author: executor; the other limits were not computed
Next step: fetch the history that contains base 3c1f0a9e7b25 (for example `git fetch --unshallow` in a shallow clone), then run `ha report docs/specs/example` again
```

**Evidence source.** With a valid completion record, it uses the conditions stored in that `done` line (§2.5); otherwise, the conditions computed now. The satisfied counts, `manual` conditions confirmed by quote, unsatisfied optional conditions and the evidence-per-condition table all read this one source. Whether a condition is required is read from the goal documents.

**Details.** After the summary come four parts separated by blank lines, in this order.
- `` ### Stamp for `## 현재` `` / `` ### `## 현재` 도장 ``: the record head's seq and the first 12 characters of its hash, the time (`at`) string stored in that line, the status, and the valid completion record's seq (if any). These are the values to copy into `## 현재` of `PROGRESS.md` as the check time and the record head. Without records, the record head is `none`; when the stored time cannot be read as RFC 3339, it shows that it cannot be read.
- `### Evidence per condition` / `### 조건별 근거`: the evidence source (a completion record seq or the current status) and a table of ID, kind, required, satisfied and evidence record seqs for each condition.
- `### Completion coverage` / `### 완료 범위`: the seq and commit of the valid completion record and the fact that it covers HEAD; or the seq and commit of the most recent completion record that became invalid, the fact that it does not cover HEAD, the invalidity reason as in §4.4, and the fact that differences after that commit are not part of the completion. `none` when there is no completion record.
- `### Output of unmet conditions` / `### 미충족 조건의 원문 위치`: for each unsatisfied required condition, the same record selection and notation as `### Unmet` (§4.4) (`ha log <goal> <seq>`, or `no stored output` when there is no raw output). The English lines are the same as the corresponding lines of `ha status`. `none` / `없음` when there are none.

Synthetic example (a goal with a valid completion record):

```text
Changed: 2 files and 2 commits outside the bundle documents since base 3c1f0a9e7b25
Checked: 2 of 2 required conditions satisfied (2 by check records, 0 by user confirmation); 1 of 1 change conditions have a fresh baseline
To review: none
Remaining limits: assurance local; check author: executor
Next step: none; completion record seq 5 holds and nothing is left to do
```

```text
바뀐 것: base 3c1f0a9e7b25 이후 실행 묶음 문서 밖에서 파일 2개, commit 2개
확인한 것: 필수 조건 2개 중 2개 충족(검사 기록 2개, 사용자 확인 0개); change 조건 1개 중 1개에 신선한 baseline
직접 볼 것: 없음
남은 한계: 보증 수준 local; 검사 작성자: 실행자
다음 행동: 없음; 완료 기록 seq 5이 유효하고 남은 할 일이 없음
```

### 1.4 Environment check (`ha doctor`)

`ha doctor <goal>` runs the check commands written in PLAN's optional section `## 실행 환경` ([bundle.md](bundle.en.md) §2.6) one at a time, in item order. It runs them from the repository root, as an argument array without a shell, with standard input closed, inheriting `ha`'s environment variables. The time limit of each check is fixed at 60 seconds and is unrelated to PLAN's `## 시간 제한` (time limit). When the time limit is reached, it ends that process and its child processes with the same process control as `ha check` (a process group on Unix, a kill-on-close Job Object on the Windows candidate).

It shows one line per tool, `<이름>: 사용 가능` or `<이름>: 사용 불가 (<이유>)`. Exit 0 is `사용 가능` (available). The reasons for `사용 불가` (unavailable) are exactly these five.
- `실행 파일 없음` (no executable): a start failure that is not a permission denial (not on PATH, no file, not an executable format)
- `실행 권한 없음` (no permission to run): could not start because permission was denied
- `시간 초과` (timed out): hit the 60-second limit. Shown as `시간 초과, 60초`
- `신호로 종료` (ended by a signal): ended by a signal. Shown as `신호로 종료, 신호 <번호>`
- `0이 아닌 종료` (nonzero exit): any other nonzero exit. Shown as `0이 아닌 종료, 종료 코드 <코드>`

Under a `사용 불가` line, it shows the end of that check's output, with four spaces in front of each line. The end is the last 20 lines, or, when those 20 lines exceed 2,000 bytes, the last 2,000 bytes (starting at a UTF-8 character boundary). A `\r` at the end of a line is removed. For a start failure, one start-failure diagnostic line appears, the same as in `ha check`'s output file. The output of a `사용 가능` check is not shown.

`ha doctor` leaves nothing behind. It receives check output in a new file in the operating system's temporary directory and deletes it after writing that check's line. No record line, run count, `ha/lock`, output file in `ha/runs` or execution bundle change is produced. It does not use the record for judgment, so it is the same before the start and for a goal under any rules. When `ha` cannot control a process or confirm its termination (including failing to create or configure a Job Object on the Windows candidate), it does not count that tool as `사용 불가`; it keeps the lines written so far and exits 70.

Without the section, it writes one line saying so; without items, one line saying so; then it exits 0. Input errors in the section are all shown in the same `코드<탭>파일:줄<탭>설명` (code, tab, file:line, tab, description) format as lint, and it exits 65 without running any check. Only `ha doctor` reads this section: with or without the section, and whatever it contains, the results of `ha lint`, `ha start`, `ha check` and `ha status` are the same.

Synthetic example:

```text
셸: 사용 가능
브라우저: 사용 불가 (실행 파일 없음)
    ha: could not start "synthetic-browser": exec: "synthetic-browser": executable file not found in $PATH
```

### 1.5 Automatic budget

When the `검증 실행` (verification runs) value in PLAN `## 예산` (budget) is exactly `자동` (automatic) (`- 검증 실행: 자동`), `ha start` sets the run limit. If there is any `검증 실행` item that contains a number, that number is used as now (the last one when there are several), and `자동` applies only when no item contains a number. So every PLAN that an `ha` from before this feature read as a number is read as the same number. `경과 시간` (elapsed time) has no automatic value and is required as now. `ha lint` does not treat the automatic mark as a problem and does not read the user configuration.

**Computation.** The limit is `max(50, ⌈최소량 × 1.5⌉)`. The minimum is the required minimum of §1.1 (`필수 change × 2 + 필수 maintain`). Example: with 34 required change conditions and 0 maintain conditions, the minimum is 68 and the limit 102. When all required conditions are manual, 50.

**Pre-approved limit.** The limit the user approved in advance is read from `ha.json` in the user configuration folder. The folder is `$XDG_CONFIG_HOME/jaekit/` when `XDG_CONFIG_HOME` is not empty, otherwise `~/.config/jaekit/`. Home is the value of `HOME` when it is not empty, otherwise the operating system's home (`USERPROFILE` on Windows). The rule is the same on macOS, Linux and Windows, and it never falls through to another candidate. It is the same `jaekit` folder as the Spar configuration (`spar.toml`). The file is one JSON object with exactly one member, `preapproved_runs`, whose value is an integer from 1 to 2147483647 without sign, decimal point or exponent. One leading UTF-8 BOM is allowed.

```json
{"preapproved_runs": 120}
```

- When the file does not exist (there is no entry at that path, and the nearest existing ancestor entry is a folder), the limit is 50 and the source is `default`. A user who set nothing is not asked a question for a goal that 50 covers.
- Everything else is refused: a symlink without a target (at that path or a parent folder), an entry that is not a regular file, a file over 64 KiB, a read failure, an ancestor entry that is not a folder, an `XDG_CONFIG_HOME` or home that is not an absolute path, a home that cannot be determined, non-JSON or invalid UTF-8, duplicate keys, unknown keys (including `preapproved_runs` that differs only in case), a missing key, trailing data, and a value that is not an integer literal or is out of range.
- If the file's real path (with symlinks resolved) is inside any working tree of the goal's repository (the top levels from `git worktree list --porcelain -z`), it is refused. Folders are compared by identity, not by name. A file inside the repository cannot set this limit. If the file exists but that list cannot be read (for example Git before 2.36, which lacks `-z`), it is refused. If the file does not exist, the list is not read.
- Only `ha start` and `ha estimate` read this file, and only when PLAN is automatic. A numeric PLAN and other commands do not read it, so a corrupt setting never blocks a goal with a numeric budget.

When the setting cannot be used, both commands show `ha: pre-approved run limit: <경로>: <이유>` and exit 65 without a record. The default does not stand in for it.

**Start.** When the computed value is at or below the pre-approved limit, the goal starts. The start record's `budget.runs` is the computed value, and the informational object `auto` keeps the basis of the computation (§2.1). The second output line is `minimum required: <n> runs; auto limit: <m> (minimum × 1.5, at least 50); pre-approved limit: <c> from <설정 파일 경로 또는 default>`. When the computed value exceeds the limit, it exits 65 without a record and shows on standard error the minimum, the computed value, the limit and its source, and that the user's approval is needed. Synthetic example:

```text
ha: auto budget 102 runs (minimum 68 × 1.5, at least 50) exceeds the pre-approved limit 50 from default; the user's approval is needed: write the approved number in PLAN `검증 실행`, or have the user raise preapproved_runs in <설정 폴더>/jaekit/ha.json
```

When the user approves, that number is written in PLAN `검증 실행` (a numeric limit is not compared with the pre-approved limit), or the user raises `preapproved_runs` in the setting. This pre-approved limit sets only the start limit, separately from `/4`'s `ha budget --preapprove` (§4.7). The increase and resume rules after the start (§4.7, §5.2) and status computation are the same as for a goal started with a number.

**Estimate.** `ha estimate` does the same computation without a record. With an automatic budget, it adds an `auto` object to the JSON (`computed_runs`, `preapproved_runs`, `source`, `source_path` only when the setting file exists, and `approval_needed`), and `plan_runs_limit` is the computed value. The Markdown format adds the line `- auto budget: <m> runs (minimum <n> × 1.5, at least 50); pre-approved limit: <c> from <출처>; user approval needed: yes|no`. The output for a numeric PLAN is unchanged.

### 1.6 Checking a skill's Core requirements

`ha start --skill <SKILL.md>` reads `core-requirements.json` in the same folder as that file. The format is `core-requirements/v1` in §8 of the [Core capabilities contract](core-capabilities.en.md).

- If there is no entry at that path, the skill has no declaration and the start is as now.
- If any declared rules, format or feature is missing from this executable's capabilities (the same values as `ha capabilities`), it shows `ha: missing Core support: <rules|formats.<키>|features> <값> (ha <버전>)` for each missing item (`formats.<키>` without a value when the `formats` key itself is missing and its list is empty) and exits 65 without a record.
- If the declaration cannot be read or does not fit the format, it shows `ha: skill requirements <경로>: <이유>; nothing recorded` and exits 65. It never ignores unknown values and starts.
- With `--check-skill`, the declaration is required. Without a declaration it is 65; given without `--skill`, it is usage error 64. An `ha` from before this feature ends with 64 on this flag as an unknown flag and records nothing. So a start where the check must happen uses this form.
- The check happens after the working tree check and before the record. When there is an entry at the declaration path, it is checked before the Skill directory digest is computed, so an unusable declaration (a symlink without a target, a folder, a FIFO and so on) is refused with the reason above. A skill without a declaration is read first, as now, and its errors are also as now. The start record's `skill` (`name`, `version`, `digest`) is unchanged, and the digest includes the declaration file.

### 1.7 Cost summary (`ha cost`)

`ha cost <goal> [--series <folder>]... [--format md|json] [--lang ko|en]` shows what one goal cost in verification and review. It reads only that goal's `runs.jsonl`, the goal documents, Git, and the Spar series folders a person points to with `--series`. It does not guess series locations, and `--series` can be given several times. It adds no line to the record, does not take `ha/lock`, and does not change the record, the working tree, the index, refs or Spar records. It does not read the current time, call a model or launch a reviewer: with the same record, Git, series and arguments, the output is the same. The summary shows numbers only and never writes a sentence that compares with another goal or speaks of improvement or effect. This command does not change the output, judgments or records of other commands.

**Spar formats read.** `ha` does not import Spar packages; it reads only the public file formats.
- [`review-round/v1`](review-round.en.md) §5: in a round folder, `round.json` (`stage`, `sequence`, `dry_run`, `counted`, `rerun_of`, `started_at`, `finished_at`, and `status`, `elapsed_seconds`, `attempts` and `fallback_used` of `reviewers.<이름>`), `findings.json` (`id`, `class`, `in_check_paths`, `location_unreadable`), `ledger.jsonl` (`finding`, `disposition`, `auto`, `user_quote`) and `waivers.jsonl` (`reviewer`, `user_quote`); and the series' `extensions.jsonl` (`stage`, `by`, `user_quote`).
- [`review-gate/v1`](review-gate.en.md) §3: gate files at the top of a series folder. Only `state` and `growth.added_lines` are read.

Inside a series folder, it reads as Spar does. A folder whose `round.json` has the `schema` `review-round/v1` is a round; a folder without `round.json`, or with JSON of another format, belongs to another tool and is skipped. Of the top-level files, only `extensions.jsonl` and JSON objects whose `schema` is `review-gate/v1` are read, and the rest are skipped. Skipped entries are shown in the summary with their names and reasons. Rounds are ordered by `sequence`, and by folder name when equal. Dry-run rounds are not included in any count; only their number is shown for each series.

**Items and sources.**

| Item | Value | Source |
| --- | --- | --- |
| Kind and size | The `Assurance:` tier in the current SPEC.md (none when absent), the rules version of the start record, the numbers of required and optional conditions, the numbers of files and commits changed outside the execution bundle documents since the base, the valid completion record seq | Goal documents, the `start` line, the same Git comparison as `Changed` in `ha report` (§1.3), the completion record from the same computation as `ha status` |
| Runs | The number of `check` and `baseline` lines (by kind and by result), the sums of `duration_ms` by kind and in total, the number of runs per target (condition ID; `T001#1` for a task) | `check` and `baseline` lines. Runs can overlap, so the sum is not elapsed time (§1.2) |
| Re-recorded runs | For each line that has a previous line with the same target and the same kind: its seq, the previous line's seq, the inputs that differ from the previous line (`code`: `commit` for check, `base_commit` for baseline; `goal_documents`: `spec_digest`; `command`: `command_digest`; `overlay`: the set of overlaid paths and digests of a baseline), and the previous line's result. When no input differs, an empty list and the previous line's result | Only the fields of the two lines are compared |
| note | The numbers of `test-change`, `recover`, `allow-recovery`, `reopen`, `input` and `confirm` notes. A goal started before `/4` cannot record the first three kinds, so they are not recorded | `note` lines |
| Record span | The seq and `at` of the first and last lines, and the seconds between them | Lines other than finding lines (`run-finding/v1`) |
| Budget window | `window_start`, `span_seconds`, `reported_wait_seconds`, `elapsed_seconds`, `elapsed_limit_seconds`, `runs`, `runs_limit` | The same computation as `budget` in `ha status` (§4.5, §5.2). Before `/4`, the span and waits are not recorded |
| Waits | For each `wait` note: its seq, the decision asked (`reason`), and the seconds of that one wait. The seconds are the computation of §4.7 (from the second after the note to the earliest end, not past the window, minus run intervals) applied in that window to that wait alone. The reported wait of the whole record is the sum in which overlapping waits are counted once per window. A goal started before `/4` cannot report waits, so the list, the sum and the counts by source are not recorded | `wait` and the line that ends it |
| Questions | Counts by source: reported waits, Spar `requirement-question` findings, cap extensions, reviewer waivers. Then lists: for each finding, its last disposition and the user's quote; for each extension, the stage, count and quote; for each waiver, the round, reviewer and quote. They are not split into scope or authority decisions and re-checks | `wait` notes; the series' `findings.json`, `ledger.jsonl`, `extensions.jsonl` and `waivers.jsonl` |
| Rounds | For each stage (`plan`, `plan-delta`, `candidate`, `fixes`): the number with `counted` true, the number of reruns with it false, and the number without `counted`. For each round, the seconds between `started_at` and `finished_at` (not finished when `finished_at` is absent), and for each reviewer, the status, `elapsed_seconds`, the number of attempts and fallback model use | `round.json` |
| Reviewer time | For each reviewer name: the sum of `elapsed_seconds` (each rounded to milliseconds, then added), the number of rounds, and the number of rounds without a time | `round.json` |
| Findings | For each class, the count and the counts by location: check paths (`in_check_paths` true), elsewhere (false), unreadable location (`location_unreadable` true), no location recorded (`in_check_paths` absent) | `findings.json` |
| Dispositions | For each class, the last ledger entry of each finding: `fix`, `reject`, `defer` (automatic deferrals, `auto: true`, counted separately), `duplicate`, `answer`, no disposition | `ledger.jsonl` |
| Lines added by fixes | For each series: when there is no fixes round (one that is not a dry run and has `findings.json`), `check` and `other` are 0. Otherwise, it is `growth.added_lines` of the gate file of that series whose set of measured ranges (`base`..`head` of `growth.rounds[]`) is exactly the set of the series' fixes round ranges (`base`..`head` of `round.json`). That is because a gate adds only the ranges it measured and counts the same range only once. How the gate wrote the round paths is not considered. When there are several such files, it is the file with the digest that a `review` note recorded last, or, when none is recorded, the first file by name. It is not recorded when there is no gate file (`no_gate_file`), when there is no such file (`gate_misses_rounds`: a fixes round run after the gate, a range the gate did not measure and so on), or when the chosen file has no `growth` (`gate_without_growth`). The total is shown only when every series has a value | Gate files; `gate_digest` of `review` notes |
| Gate files | For each series: the name, the digest, `state`, and the seq of the last `review` note with that digest | Gate files, `review` notes |
| Tokens | New input, cached input and output tokens are always not recorded. They are not shown as 0 | No record holds them. Recording them is [issue #174](https://github.com/jgoneit/jaekit/issues/174) |

**Output.** The default is Markdown. After the title (`# Cost summary: <goal>`, or `# 비용 요약: <goal>` in Korean) come six sections in this order: kind and size, verification, questions and waits, review rounds, findings and fixes, and tokens. `--lang` is `ko` or `en`, `en` when absent, and it changes only labels and sentences. Condition IDs, seq, results, stages, classes, dispositions, reviewer names and statuses, file names and quotes are the same in both languages. Line breaks inside quotes and `reason` are written as `\n`. `--format json` shows the same values as one `cost/v1` document. The top-level keys are `schema`, `goal`, `kind`, `verification`, `questions`, `review`, `findings`, `tokens` and `unavailable`; where a value is missing it is `null`, and `unavailable` shows that location (keys joined with dots) and the reason: `not_started` (no start record), `not_given` (no `--series`), `not_recorded` (the record does not have that value), `cannot_compare` (Git cannot compare base and HEAD), `not_computed` (that value cannot be computed), `none` (no tier line or no valid completion record). `unavailable` names only locations outside lists. A `null` inside a list item means that item's record has no value: for a round's `counted`, `rerun_of`, `started_at` and `finished_at` and a reviewer's `elapsed_seconds`, `attempts` and `fallback_used`, the field is absent from `round.json`; for a round's `seconds`, the round did not finish or has no time; for `growth[].lines`, that item's `source` is the reason; for a gate's `recorded_seq`, there is no `review` note that recorded that digest; for a requirement question's `disposition` and `user_quote`, there is no disposition or quote. The JSON does not change with `--lang`. 0 and not recorded are kept distinct.

**Input problems.**
- A goal without a start record: it shows only the tier and the condition counts read from the goal documents, and says there is no run record (`verification` is `not_started`). It does not show 0.
- Without `--series`, the review round and finding sections only say that none was given, and the Spar sources of questions are `not_given`.
- A series path that does not exist, a path that is not a folder, a folder that cannot be read: it exits 65 with an error naming that path, and prints no summary.
- A broken `review-round/v1` round (a `round.json` that is not JSON, a `round.json` that cannot be opened, a field that does not fit the format, a `started_at` or `finished_at` that is not an RFC 3339 time, a `findings.json` that cannot be parsed, a `ledger.jsonl`, `waivers.jsonl` or `extensions.jsonl` line that cannot be parsed, no `findings.json` in a finished round that is not a dry run, no `ledger.jsonl` while `findings.json` exists): it exits 65 with an error naming that file, and prints no summary. It never shows a partially counted summary. A round without both `findings.json` and `finished_at` stopped before recording findings, so it has no findings. A folder named `waivers.jsonl` means there are no waivers (review-gate.md §2).
- A broken `runs.jsonl`: it exits 65 with an error naming that file, and prints no summary.
- Records left by an older `ha` or `spar` without some fields: only those values are shown as not recorded (for example `counted`, `elapsed_seconds`, `attempts`, `fallback_used`, `in_check_paths`, waits before `/4`), and the rest are counted. A finding without `location_unreadable` is a finding whose location was read (review-round.md §6).
- When Git cannot compare base and HEAD, as in a shallow clone, or cannot read a commit that the record points to: the changed file and commit counts are shown as impossible to compare (`cannot_compare`), the completion record is shown as not computed, and the values read from the record are shown as they are. The exit code is 0.

## 2. Line format

Every line has the following common fields.

| Field | Meaning |
| --- | --- |
| `schema` | `"run/v1"` |
| `seq` | An integer increasing from 1 |
| `prev` | The sha256 of the previous line's bytes. `null` for the first line |
| `kind` | `start` \| `check` \| `baseline` \| `note` \| `done` \| `budget_change` |
| `at` | UTC time |
| `ha_version` | The `ha` version that recorded the line |

In this document, **bundle documents** means only the following files in the goal directory.
- Goal documents: `SPEC.md` and the linked supporting documents ([goal-docs.md](goal-docs.en.md) §1)
- `PLAN.md`
- `REVIEW.md`
- `PROGRESS.md`
- `tasks/**`
- `runs.jsonl`
- `amendments/**`: amendment documents ([goal-docs.md](goal-docs.en.md) §6.6). They are not part of the goal document set even when SPEC.md links to them.

`amendments/**` of other goal directories under the same parent (§4.6) is also excluded from the code state, whatever the rules version. Other files in the goal directory (verification scripts, fixtures and so on) are treated as code. The **code state** is these two combined.
- HEAD's tree without the bundle documents
- Changes in the working tree: modifications to tracked files, and untracked files. Ignored files (`.gitignore`) are left out.

### 2.1 `start`

| Field | Meaning |
| --- | --- |
| `goal` | The goal directory path |
| `base_commit` | HEAD at the start |
| `tree_clean` | Whether the working tree outside the bundle documents was clean at the start |
| `spec_digest` | The digest of the goal document set: the sha256 of each document's path and content sha256, concatenated in path order ([goal-docs.md](goal-docs.en.md) §1) |
| `request` | `{ "source": "conversation", "quote": "<사용자 요청 인용>" }`. It is a claim. |
| `host` | `{ "name", "version", "model" }`. It is a claim. |
| `skill` | `{ "name", "version", "digest" }`. The Seal Skill and template package the agent read. With `--skill`, `ha` reads `name` and `metadata.version` from the front matter, and digests all files in the Skill directory the same way as the goal digest. The agent decides which file to point to, so it is still a claim. |
| `rules` | The status computation rules version (for example `"run-rules/1"`). `ha` sets it. This goal's status is computed with this version to the end (§4). |
| `budget` | PLAN's budget `{ "runs", "elapsed_seconds", "cost" }`. It is the initial limit. `/1` and `/2` keep using this value, and the run limit of `/3` reflects valid change records (§5.2). With an automatic budget (§1.5), `runs` is the computed value and the informational object `"auto": { "minimum_runs", "computed_runs", "preapproved_runs", "source" }` is added. `source` is `default` or `user_config`, and the user's path is not included. Status computation does not read `auto`. |

A `/4` start line has `scope` (`allowed`, `tests`: copies of the paths in PLAN `## 범위` (scope) at the start), and, with `--test-authority`, also `test_authority` (`interpretation`) (§4.7).

### 2.2 `check`

| Field | Meaning |
| --- | --- |
| `target` | `{ "criterion": "AC-1" }` or `{ "task": "T001", "index": 1 }` |
| `criterion_kind` | The condition kind at run time. Task checks have none. It is kept so that a change of kind shows from the record alone. |
| `argv` | The command that was run |
| `command_digest` | The sha256 of the command string written in PLAN |
| `commit` | HEAD at run time |
| `tree_clean` | Whether the working tree outside the bundle documents was clean at run time |
| `spec_digest` | The SPEC digest at run time |
| `exit` | The exit code. `null` on a start failure or a timeout |
| `result` | For `/1` and `/2`, and for ordinary maintain and task checks: `pass` (exit 0) \| `fail` (exit ≠ 0) \| `error` (start failure, timeout). A `/3` change is classified according to the [structured result contract](check-result.en.md). |
| `timing` | Optional process measurements (below). Not used for judgment, freshness, usage or budget. |
| `duration_ms`, `output_digest`, `output_path` | Run time, output digest, local output location (a repository-relative path, for example `.git/ha/runs/<goal-key>/7.log`). `output_path` never holds an absolute path. The format is the same in a linked worktree, where `.git` means that working tree's git directory. Absolute paths left by an earlier `ha` are read as they are, not rewritten. In `/3`, when saving the output fails after an actual attempt, the attempt and usage are recorded with `error` and `process_output`, and `output_path` and `output_digest` may be empty. |

`timing` holds the measurements of this run. A line without this field (a line from an earlier `ha`) has those values not recorded.

| Field | Meaning |
| --- | --- |
| `limit_ms` | The time limit applied to this run. One integer in milliseconds; it is the only recorded value of the applied limit that the two goals share |
| `limit_elapsed_ms` | From when the limit timer started (after the process started successfully) until the process's exit was confirmed or the timeout was decided. Time spent afterward cleaning up child processes and confirming their exit is not included. `null` when the process could not start and the timer was not started |
| `limit_reached` | Whether the time limit fired |
| `cpu` | `{ "status", "user_ms", "system_ms", "scope", "source" }`. `status` is `measured`, `unavailable` (no process ran, or the operating system does not provide it) or `failed` (reading failed). `user_ms` and `system_ms` are present only for `measured` |

`cpu.scope` and `cpu.source` are fixed codes. On Unix (macOS, Linux) they are `process-and-waited-descendants` and `wait4-rusage`. These are the values `wait4` returned, and they count only the process that was run and the child processes it waited for. Child processes that were not waited for, terminated with the whole process group by the time limit, are not included. On Windows they are `job-process-tree` and `job-object-accounting`. All processes that ran in the Job Object assigned before the run are counted after they have all ended. The two scopes differ, so values are not compared across operating systems as having the same meaning.

The meaning of `duration_ms` is unchanged. Timing starts before command preparation and process start, and ends after waiting for exit and cleanup. So `duration_ms` divided by `limit_ms` is not the exact headroom; proximity is read from `limit_elapsed_ms` (§1.2). A refusal before a run and a preparation failure are not runs, so they produce neither a line nor measurements. Measurements contain no absolute paths, environment variable values or command output.

`timing` values are not judged, so a `timing` with a corrupt type or value is not a record integrity error. Readers treat only that value as unavailable. Corruption of the JSON itself and a break in the `seq`/`prev` chain are integrity errors, as now. An earlier `ha` (v0.1.4 or earlier), which ignores unknown fields and chains hashes over the original bytes, reads records that contain this field and keeps appending to them within its own supported rules.

A `/3` change line adds `evidence`. It is the compact `check-evidence/v1` evidence of [check-result.md](check-result.en.md), and links the declaration and producer inputs, the invocation, the report digest and the per-target observations. `unknown` is inconclusive, not a pass.

A `/4` line adds the run start time `started_at` (§4.7). So does a baseline line.

### 2.3 `baseline`

The following are added to the fields of `check`. `commit` is the HEAD that the overlaid check files were taken from.

| Field | Meaning |
| --- | --- |
| `base_commit` | The base commit of `start` |
| `overlay` | `[{ "path", "digest" }]`. Check files taken from HEAD and overlaid |
| `result` | `/1`, `/2`: `fail_as_expected` (exit ≠ 0) \| `unexpected_pass` (exit 0) \| `error`. `/3` applies the [structured result contract](check-result.en.md). |

`/1` and `/2` do not distinguish the cause of a failure (for example an import failure of a module that does not exist yet). A `/3` change requires an actual observation of the declared target. Import, collection and startup errors, not running, and unclear failures of ordinary scripts are not expected failures. When a runnable check directly observes that a feature is absent, it can be a violation of the declared requirement. A person who wants to see the raw output uses `ha log`.

For baselines run from now on, in `/1`, `/2` and `/3` alike, the full copy list of one check target is checked before any file is applied. The HEAD source must be a committed regular file (`100644` or `100755`), and if a copy destination or its parent in the base state is a symlink, it is refused, whether the link is internal, external or broken. The same conditions apply to declaration, producer and additional check paths. Safe replacement of regular files, creation of missing parents and files, and execute permission are kept, and symlinks unrelated to the copy are allowed.

Irregular entries such as Windows junctions are not treated as safe regular files or directories either. When safety cannot be established because of a preparation error or an observed unsafe path change, the command is not started. It reports the cause of the failure and the repository-relative path concerned, and preserves the content and permissions of the original working tree and of files outside the temporary worktree. No check record or usage is added for targets that did not run, and in a batch request the records and usage of targets that already ran are kept. This is a safety condition for new runs and does not retroactively reclassify past records. Errors and failures after an actual attempt to start a process follow the existing record and usage rules. This is not a feature that isolates arbitrary uncooperative external processes or restricts the writes of the check command itself.

### 2.4 `note`

| Field | Meaning |
| --- | --- |
| `note` | `block` \| `unblock` \| `confirm` \| `reopen` \| `input`. The notes `/4` and `/5` goals use, and those only `/5` goals use, are below |
| `cause` | For `block`, the cause kind ([bundle.md](bundle.en.md) §5) |
| `subject` | For `confirm`, the subject: `AC-n` (a manual condition), `scope:<path>`, `tests:<path>`, `spec` |
| `bound` | For `confirm`, the value that `ha` computes and binds (table below). The fields are whichever of `criterion_sha256`, `spec_digest` and `blob` apply. The agent does not set it. |
| `quote` | A quote of the user's words (`confirm`, `reopen`, `input`) or an explanation. It is a claim. |

`confirm` is valid only for **the exact content confirmed**. When recording, `ha` binds the subject's current value in `bound`. If that value differs from the current one when the status is computed, the confirmation is invalid. Invalidated confirmations are listed separately in the status output.

| Subject | `bound` | Valid when |
| --- | --- | --- |
| `AC-n` | The sha256 of that condition's sentence in the goal documents, and the goal digest. Usable only for `manual` conditions | Both equal the current values |
| `scope:<path>` | The HEAD blob id of that path (`deleted` if deleted). The working tree must be clean when recording | The same path in the current HEAD is the same blob |
| `tests:<path>` | Same | Same |
| `spec` | The current SPEC digest | Equals the current SPEC digest |

`input` is any of the user's words during a run that is not `confirm` or `reopen`. Examples: an instruction to proceed, an answer to a question, a new requirement, a change request. `ha` does not classify it. When a classification is needed, a person reads the record and decides. It is not used in status computation. It serves as grounds for improvement candidates, and as material for answering the same questions when a past goal is run again.

`/5` goals also write the `review` note (`review`: the review gate summary, §4.8). Its quote is empty.

`/4` and `/5` goals also write the `test-change` (subject `tests:<path>`, `bound.blob`, `test_change`: `criteria`, `how`, `moved_to`, `weakened`), `wait` (`reason`), `resume`, and `recover` and `allow-recovery` (`recovery`: `criterion`, `input`, `change` or `count`) notes (§4.7).

### 2.5 `done`

| Field | Meaning |
| --- | --- |
| `status` | The computed status (§4) |
| `reasons` | The list of remaining reasons |
| `commit`, `tree_clean`, `spec_digest` | The code state and SPEC at the time of computation |
| `record_head` | The record head up to just before this line |
| `criteria` | `{ "id", "kind", "satisfied", "records": [seq…] }` per condition. The records that served as evidence |
| `command_digests` | The current command digest per condition |

### 2.6 `budget_change`

A `/3`-only line. It records the common fields and the following `budget_change` object.

| Field | Meaning |
| --- | --- |
| `goal` | The same goal as the start record |
| `window_start` | The start/reopen seq that the change targets |
| `revision` | The latest start/reopen/budget_change seq just before the change |
| `previous_runs`, `runs` | The previous and new total run limits |
| `quote`, `context` | The user's original words that support an increase, and the request context. If the source is not verified, the executor's claim |
| `interpretation` | An optional agent interpretation, kept distinct from the user's original words |
| `reason` | The reason for the change. Required for a decrease |

The command's `--window`, `--revision` and `--from` must match the current window, revision and limit respectively. It reads the latest record under the lock, compares, and then records. Merely adding check records is not a budget revision; it is reflected in the actual cumulative usage. The seq of a new change becomes the budget revision from then on.

A `/4` pre-approval is a `budget_change` with `preapproved_runs`, and its `runs` equals `previous_runs` (§4.7).

## 3. Freshness

A result is bound to the code state, SPEC and verification command it came from.

**Premise: the working tree is clean now.** If the working tree outside the bundle documents has changes (modified tracked files, untracked files), the current code state matches no commit. Then no record is fresh, and the status reason is `worktree_dirty`.

Here the **bundle documents** are the goal's own goal documents and execution bundle (including `amendments/**`), and the `amendments/**` of other goals under the same parent. Under `run-rules/2` and `run-rules/3`, the execution bundle files of other goals are included too (§4.6). Everywhere in this contract, "outside the bundle documents" means outside this list.

A `check` line r is **fresh** when all of the following hold.
1. The working tree is clean now (the premise).
2. `r.tree_clean` is true.
3. There is no difference outside the bundle documents between the record's commit and HEAD. For example: `git diff --quiet r.commit HEAD -- . ':(exclude)<goal>/SPEC.md' ':(exclude)<goal>/PLAN.md' …`
4. `r.spec_digest` equals the current SPEC digest.
5. `r.command_digest` equals the digest of the corresponding command in the current PLAN.

A `baseline` line b is fresh when all of the following hold. Item 3 is not required. The baseline ran at the base commit, and the digests of the overlaid check files are its link to HEAD.
- Items 1, 2, 4 and 5 above hold.
- `b.base_commit` equals the base commit of `start`.
- The set of paths in `b.overlay` equals the check paths of the current condition table, and each digest equals the content of the same path at the current HEAD.

A `/3` change also needs its link to the declaration, the result contract, the targets, and the producer version and content ([check-result.md](check-result.en.md)). Even when the command string is the same, earlier checks and completion evidence are not reused if this meaning changes. For a runner adapter declaration (`check-adapter/v1`), the declaration bytes and the adapter's judgment semantics version are linked instead of the producer. When Core comes to judge the same runner output differently and the adapter's judgment semantics version goes up, earlier baselines, checks and completion evidence obtained with that adapter are not fresh. The same holds when the declaration's violation or the linked test name changes. The path of the output location is not recorded, so it is not used for freshness ([check-result.md](check-result.en.md#5-runner-adapters) §5). Status is reconstructed from the stored compact evidence, so a judgment does not change merely because the raw log is gone or the current time is different.

Lines that are not fresh are never deleted. They are only left out of the satisfaction computation. Earlier failures and errors remain as attempt history (§4.4).

**Limitation**: ignored files (`.gitignore`) are not part of the code state. If a check depends on an ignored file (for example, local settings), its result can differ even in the same code state.

## 4. Status computation (`ha status`, `ha done`)

The only inputs are `runs.jsonl`, the execution bundle and git. It calls no model and **never uses the current time.** The same input gives the same result whenever it is computed.

The computation rules follow the `rules` version of `start`. Even when `ha` gains a new rules version, a goal in progress uses the version it started with. An `ha` that does not support that version does not compute, and exits with code 66 (unsupported rules version).

### 4.1 Satisfaction per condition

First, each condition is decided as satisfied or not. The overall status is computed after that.

C is the condition's fresh `check` lines, and B its fresh `baseline` lines. E is the number of error reruns allowed in the same code state (default 1). An error (`error`) means the check reached no judgment. It is evidence of neither failure nor pass. So it can be run again up to E times. A failure (`fail`) is not erased by running again.

| Kind | Satisfied when |
| --- | --- |
| `maintain` | C has at least one `pass`, no `fail`, at most E `error`s, and the most recent line of C is `pass`. |
| `change` | C meets the `maintain` condition. In addition, B has at least one `fail_as_expected`, no `unexpected_pass`, at most E `error`s, and the most recent line of B is `fail_as_expected`. |
| `manual` | The condition has a valid `confirm` (§2.4). No `check` record is required. |

A condition that is not satisfied gets one reason: the first that matches, in the following order.

| Reason | Applies to | When | Class |
| --- | --- | --- | --- |
| `criterion_missing` | change, maintain | C is empty | executor fixes |
| `criterion_flaky` | change, maintain | C has both `pass` and `fail`. The results split on the same code | executor fixes (fix the check or the code and verify again) |
| `criterion_failed` | change, maintain | C has a `fail` | executor fixes |
| `criterion_error` | change, maintain | C has at most E `error`s, and the most recent line is `error` | executor fixes (run again) |
| `baseline_missing` | change | B is empty | executor fixes |
| `baseline_unexpected_pass` | change | B has an `unexpected_pass` | executor fixes (fix the check, or change the kind to `maintain` through a plan change) |
| `baseline_failed` | change | A fresh `/3` baseline has an assertion failure other than the intended one, or an internal attempt mismatch | executor fixes |
| `evidence_invalid` | change | The stored compact evidence of `/3` is not valid | executor fixes |
| `baseline_error` | change | B has at most E `error`s, and the most recent line is `error` | executor fixes (run again) |
| `error_limit` | change, maintain | The `error`s in C or B exceed E. The check environment keeps failing to reach a judgment on the same code | user confirmation |
| `manual_unconfirmed` | manual | There is no valid `confirm` (there is none, or the content changed after it was confirmed) | user confirmation |

A `/3` change applies the [structured result contract](check-result.en.md) together with the positive-evidence conditions above. `unknown` uses the same retry limit as `error`. In a baseline, an assertion failure other than the intended one, or an internal attempt mismatch, stays a `fail` and gives `baseline_failed`. Even when mixed with environment errors or inconclusive results, unfavorable observations per target are preserved, and a later pass on the same input does not clear them. If the compact evidence for the current input is itself inconsistent, the reason is `evidence_invalid`. A record whose input went stale because the declaration changed is excluded by freshness. Unknown result values and inconsistent stored data show up as `record_invalid` (user confirmation) under every supported rules version, and never become completion evidence.

Optional conditions (marked `(선택)` in the goal documents) and `EX-n` added by the executor are computed and printed the same way, but do not block the status. Whether a condition is required is read only from the goal documents.

### 4.2 Goal-level reasons

| Reason | When | Class |
| --- | --- | --- |
| `lint_error` | `ha lint` has errors | executor fixes |
| `not_started` | There is no `start` line | executor fixes |
| `record_integrity` | A `prev` link is broken or a `seq` is out of sequence | user confirmation |
| `worktree_dirty` | The working tree outside the bundle documents is not clean (§3 premise) | executor fixes (commit or revert) |
| `out_of_scope` | Among the files changed since base (excluding the bundle documents), there is one that does not match `바꿀 수 있는 경로` (paths that may change) and has no valid `confirm scope:` for its path | user confirmation |
| `test_definition_changed` | A test-path file that existed at base was modified or deleted, and its path has no valid `confirm tests:` | user confirmation |
| `spec_changed` | The current SPEC digest differs from the digest of `start`, and there is no valid `confirm spec` | user confirmation |
| `task_open` | A required task is not `done` or `dropped` in `PROGRESS.md` | executor fixes |
| `record_invalid` | There is an unknown result value or inconsistent stored data | user confirmation |
| `blocked` | The last `block` has no `unblock` after it | cause resolved |
| `budget_exceeded` | The number of `check` and `baseline` lines, or the elapsed time, within the budget window (§5.2) exceeds the budget | user decision |
| `review_missing` | `/5` only: the tier requires review but no gate is recorded (§4.8) | executor fixes |
| `review_blocked` | `/5` only: the last recorded gate is `blocked` (§4.8) | executor fixes |
| `review_degraded` | `/5` only: the last recorded gate is `degraded` (§4.8) | executor fixes |
| `review_open_defect` | `/5` only: that gate has an open `defect` (§4.8) | executor fixes |
| `review_open_question` | `/5` only: that gate has an open `requirement-question` (§4.8) | user confirmation |
| `review_code_missing` | `/5` only: that gate has no code round (§4.8) | executor fixes |
| `review_head_stale` | `/5` only: the head that gate reviewed is not HEAD (§4.8) | executor fixes |
| `review_docs_stale` | `/5` only: that gate's rounds did not receive the current goal documents (§4.8) | executor fixes |
| `review_plan_missing` | `/5` only: the goal is `strict` but that gate has no plan round (§4.8) | executor fixes |

### 4.3 Status

The status is the first of the following that matches, in order. All reasons are printed with the status.

1. There is a valid completion record (§5.1) → `complete`. In this case the budget is not computed.
2. All required conditions are satisfied and there is no goal-level reason → `complete`. Under `run-rules/2` and `run-rules/3`, this also applies when the only remaining reason is `budget_exceeded` (§4.6).
3. There is a `blocked` reason → `blocked`
4. There is a `budget_exceeded` reason → `budget_exhausted`
5. There is at least one "executor fixes" reason → `incomplete`
6. Otherwise (only "user confirmation" remains) → `needs_user`

Even in case 2, declaring completion requires leaving a completion record with `ha done`. In that case `ha status` also prints "no completion record".

**exit code**: `complete` 0, `incomplete` 1, `needs_user` 2, `blocked` 3, `budget_exhausted` 4, usage and internal errors 64 or above (§1).

### 4.4 Output

The output includes the following. **It never includes raw output (log contents)** (§1).
- **Header**
  - `보증 수준: local` (assurance level: local) and "an agent with the same user permissions can change checks and records"
  - `검사 작성자: 실행자` (check author: executor)
  - record head
  - completion record seq (if any)
  - rules version, Skill name and version (claimed)
- **Per condition**
  - kind, whether required, whether satisfied, reason, evidence record seqs
  - attempt history: the number of `fail`s and `error`s, including those that are not fresh, and their commits. This exposes erasing an earlier failure with a trivial commit and passing again.
- **Goal-level reasons**
- **Changes a person needs to look at**
  - conditions whose verification command changed since `start`, with the commands before and after the change
  - conditions whose kind changed, and the list of optional conditions
  - user confirmations that became invalid (§2.4)
  - lines where `ha_version` changed while the goal was in progress
- **Local output location**: the local output paths of failure and error records. View their content with `ha log`.

When the status is not `complete`, the human-readable output puts an **unmet summary** (`### Unmet`) before the condition table.
- For each unmet required condition, it shows the ID, the reason code and the class (`executor`, `user` and so on from §4.1).
- If there is a fresh record that the reason relies on, it shows that seq. The record is chosen as follows.
  - If a record is invalid, choose the line that caused the problem.
  - For a baseline reason, choose the most recent baseline whose result differed from the expectation.
  - For `error_limit`, choose the most recent error or inconclusive record among checks and baselines.
  - Otherwise, choose the most recent check that did not pass.
  - If there is no such record, choose the most recent fresh record of that kind.
- If that record has stored raw output, it also shows the exact command `ha log <goal> <seq>`.
  - `<goal>` is the goal argument exactly as given to the command. Run that command from the same directory.
  - If the argument is an absolute path, it uses the repository-relative goal path. Run this command from the repository root. Human-readable output never contains an absolute path.
  - The `ha done` hint and the `ha log` hint at the local output location use the same goal notation.
  - If the goal notation contains characters other than ASCII letters, digits and `-_./@%+=:,`, it is wrapped in single quotes, so that a copied command passes it as one argument.
- A record without raw output shows `no stored output`, and no command is generated. A record lacks raw output when saving the output failed or when the record has no output path.
- A condition without a fresh record shows `no fresh record`, and no seq is generated. An attempt rejected before running leaves no record, so it falls in this case. A `manual` condition waits for a confirmation instead of a record, so it does not get this mark.
- Goal-level reasons show their code and class.
- The table and the `### Reasons` list stay as they are.
- The human-readable output of `ha done` does not include this summary, and its completion record line does not get the `done` information below either. The output of `ha done` keeps its earlier format.

When there is no valid completion record, the completion record line shows the following.
- If the status of the most recent `done` line is not `complete`, it shows its seq and commit, and the fact that the line was recorded with a status other than complete. Such a line was never a completion.
- If there is a most recent completion record (a `done` line with `status: complete`), it shows its seq and commit, and every reason it became invalid. The reasons are the broken validity conditions of §5.1, shown in this order.
  1. broken record link
  2. invalid stored record
  3. uncommitted changes outside the bundle documents
  4. code changes after that commit (including a commit Git cannot compare)
  5. goal document changes
  6. required command changes (condition ID)
  7. a following `reopen` (seq)
  8. under `/3`, not all required conditions are satisfied
- If there is no `done` line at all, it is `none` as before. A goal that has only satisfied the completion conditions gets the `ha done` hint, as before.

Synthetic example (a goal whose code changed after completion, and where running `ha done` again recorded an incomplete result):

```text
- completion record: none; latest done seq 9 (commit 2b7e01c4a9d3) recorded incomplete, not a completion; latest completion seq 6 (commit 8f3a2d91c0b7) is no longer valid: code outside the bundle documents changed since that commit; required conditions are not all satisfied

### Unmet

- AC-2 `criterion_failed` (executor): seq 8, output `ha log docs/specs/example 8`
- AC-3 `criterion_missing` (executor): no fresh record
```

For each target that did not end as expected, `ha check` prints `output for <대상>: ha log <goal> <seq>` after that target's result line. Even if the batch ends early because a later target is rejected before running or cannot be recorded, the lines for targets recorded before that are printed. The `<goal>` notation is the same as above. For a target whose raw output was not stored, it prints `output for <대상>: no stored output (seq N)`. The per-target result lines and the exit code do not change.

This output does not change the judgment, `status/v1`, the completion record or `ha log`. With the same record, bundle and Git, it is the same.

`--format md` (the default) is the format people read and paste into `PROGRESS.md`. Labels and reason codes are written exactly as in the contract, and explanatory sentences are written in English.

### 4.5 `--format json`

A structured format that external tools can read. The field order is fixed to the order of the tables below, and indentation is two spaces. It does not read the current time anew. Finding and budget history include only stored times, so the same input gives byte-identical output. Tests compare the fields of the tables below with the actual output.

#### Status document (`status/v1`)

| Field | Meaning |
| --- | --- |
| `schema` | `"status/v1"` |
| `goal` | goal directory (repository-relative) |
| `status` | the status from §4.3 |
| `exit_code` | the exit code from §4.3 |
| `assurance` | always `"local"` |
| `check_author` | always `"executor"` |
| `rules` | the rules version used for the computation |
| `ha_version` | the version of the `ha` that computed it |
| `skill` | the Skill of `start` (claimed), or `null` |
| `record_head` | `{ "seq", "sha256" }`, or `null` if there is no record |
| `completion_record` | the seq of the valid completion record, or `null` |
| `complete_without_record` | `true` if all conditions are satisfied but there is no completion record |
| `commit` | the current HEAD |
| `tree_clean` | whether the working tree outside the bundle documents is clean now |
| `spec_digest` | the current goal digest |
| `criteria` | per-condition evaluation (below) |
| `reasons` | the reasons that set the status: unmet reasons of required conditions, and goal-level reasons |
| `lint` | lint errors `{ "code", "file", "line", "detail" }` |
| `changes` | changes a person needs to look at (below) |
| `budget` | the budget window (below), or `null` before the start |
| `findings` | the `findings/v1` query of [post-completion findings](findings.en.md): completion at the time, what is unresolved now, and rework relations. It does not replace the existing status or exit result |
| `usage` | separate aggregation axes of all attempts, results and evidence (below) |
| `logs` | `{ "seq", "target", "result", "path" }` of failure and error records, with an optional `reason` (a verified reason code, or `evidence_invalid` or `record_invalid`) |

#### Conditions (`criteria[]`)

| Field | Meaning |
| --- | --- |
| `id` | `AC-n` or `EX-n` |
| `kind` | `change` \| `maintain` \| `manual`. An empty string if it is not in the condition table |
| `required` | whether the condition is required |
| `satisfied` | whether it is satisfied, per §4.1 |
| `reason` | the §4.1 reason if not satisfied, an empty string if satisfied |
| `records` | the seqs of the fresh records that were evidence for satisfaction |
| `attempts` | attempt history (below) |

#### Attempts (`attempts`)

| Field | Meaning |
| --- | --- |
| `checks` | the number of `check` lines of this condition (including those not fresh) |
| `fails` | of those, `fail` |
| `errors` | of those, `error` |
| `baselines` | the number of `baseline` lines |
| `unexpected_passes` | of those, `unexpected_pass` |
| `baseline_errors` | of those, `error` |
| `unknowns` | checks that were inconclusive (`unknown`) |
| `baseline_unknowns` | baselines that were inconclusive (`unknown`) |
| `baseline_fails` | baselines with an assertion failure other than the intended one (`fail`) |
| `stale` | the number of `check` and `baseline` lines that are not fresh |
| `fail_commits` | the commits where a `fail` or `error` occurred (first 12 characters) |

#### Usage (`usage`)

| Field | Meaning |
| --- | --- |
| `total` | the total number of check and baseline attempts |
| `checks`, `baselines` | the number per attempt kind |
| `environment_errors`, `unknowns` | the number of error and inconclusive results |
| `current_inputs`, `stale_inputs` | the number of attempts linked to the current input, and of attempts on stale input |
| `valid_evidence` | the number of valid pass and expected-baseline evidence for the current input |

Kind, result and freshness are overlapping axes. The columns are never added together into a total run count. `valid_evidence` is the subset of positive evidence for the current input, not a count of satisfied conditions or of the whole goal.

#### Reasons (`reasons[]`)

| Field | Meaning |
| --- | --- |
| `code` | the reason from §4.1 or §4.2 |
| `class` | `executor` (executor fixes) \| `user` (user confirmation) \| `blocked` \| `budget` |
| `criterion` | the condition ID, when it is a condition's reason |
| `paths` | repository-relative paths for `out_of_scope`, `test_definition_changed` and `worktree_dirty` |
| `detail` | description (in English). For `worktree_dirty` it includes tracked/untracked per path and the observed file type and change state |

#### Changes (`changes`)

| Field | Meaning |
| --- | --- |
| `command_changed` | `{ "criterion", "seq", "before", "after" }`: conditions whose verification command differs from the earlier record |
| `kind_changed` | `{ "criterion", "seq", "before", "after" }`: conditions whose kind differs from the earlier record |
| `optional` | optional conditions and `EX-n` |
| `invalid_confirmations` | `{ "seq", "subject", "reason" }`: confirmations that lost their effect |
| `ha_version_changed` | `{ "seq", "ha_version" }`: lines written by an `ha` version different from that of `start` |

#### Budget (`budget`)

| Field | Meaning |
| --- | --- |
| `window_start` | the seq of the line where the budget window started |
| `runs` | the number of `check` and `baseline` lines in the window |
| `runs_limit` | `/1` and `/2`: the initial limit; `/3`: the latest valid run limit; `/4`: the run limit of the current window. A resumed window under `/4` starts from the new window's limit set by §4.7 |
| `revision` | the latest start/reopen/budget_change seq of the current window |
| `remaining_runs` | remaining runs (minimum 0) |
| `excess_runs` | excess runs (minimum 0) |
| `reached` | whether usage equals the run limit |
| `history` | a list of `{ "seq", "at", "change": <budget_change 객체> }` (the budget change object). Earlier changes are kept too |
| `elapsed_seconds` | the elapsed time in the window |
| `elapsed_limit_seconds` | `budget.elapsed_seconds` of `start` |
| `exceeded` | whether the budget was exceeded |

### 4.6 Rules versions

The `rules` of `start` set the computation rules. A new `ha start` records `run-rules/3`. `ha start --rules run-rules/4` records `run-rules/4` (§4.7), and `ha start --rules run-rules/5` records `run-rules/5` (§4.8). The default rule does not change; a new Seal chooses the new rules with this option. A goal started under `/1` or `/2` is computed under that rule even by a new binary, and later runs do not mix in the new result or budget semantics. There is no automatic migration and no rewriting of past records. A new change check gets valid evidence only when it is connected to the structured result contract.

For a rules version that `ha` does not support, it neither computes nor records. `ha status`, `ha done`, `ha check`, `ha note` and `ha budget` exit with 66. If the start rule can be read, rejecting the unsupported rule takes precedence over integrity errors after it. Even under supported rules, unknown results and inconsistent data are not evidence of success. Only the meaning of valid old records is preserved for compatibility.

| | `run-rules/1` | `run-rules/2` | `run-rules/3` | `run-rules/4` | `run-rules/5` |
| --- | --- | --- | --- | --- | --- |
| Bundle documents (paths left out of the code state) | The goal's own goal documents and execution bundle, and other goals' `amendments/**` | Also includes other goals' execution bundle files. The PLAN's `다른 목표 실행 묶음` (other goals' execution bundles) declaration stays an input | Same as `/2` | Same as `/2` | Same as `/2` |
| When the only remaining reason is `budget_exceeded` | `budget_exhausted` | `complete`. `ha done` leaves a completion record | Same as `/2` | Same as `/2` | Same as `/2` |
| Failure and pass evidence for change | Process exit result | Same as `/1` | Declared targets, required violations, errors and invocation link are confirmed through structured results | Same as `/3` | Same as `/3` |
| Changing the run limit of the current window | Unsupported | Unsupported | `budget_change`, usage kept | `/3` plus increases within the pre-approved limit that need only a reason | Same as `/4` |
| Run limit of a resumed window | Initial start limit | Initial start limit | Inherits the latest valid run limit | The start limit or the last limit set by a user quote. Pre-approval is not inherited | Same as `/4` |
| Changing existing tests | User confirmation | User confirmation | User confirmation | Satisfied without confirmation when there is a rationale within the recorded authority | Same as `/4` |
| Elapsed time | Whole window | Whole window | Whole window | Minus the waits the executor reported | Same as `/4` |
| Error limit | E=1 on the same code | Same as `/1` | Inconclusive results count too | `/3` plus recovery notes | Same as `/4` |
| Per-command time limit | 900 seconds | 900 seconds | 900 seconds | PLAN `## 시간 제한`, or 900 seconds without it | Same as `/4` |
| Review gate | Not used in judgment | Not used in judgment | Not used in judgment | Not used in judgment | A `standard` or `strict` goal completes only when the last recorded gate covers the current HEAD and goal documents (§4.8) |

**Other goals' execution bundle files.** These are `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl` and `tasks/` directly under another directory that is under the same parent as the goal's own directory and has a `SPEC.md` in the current working tree. That directory's `SPEC.md`, the documents it links, and other files such as `checks/` are code. A directory that is not under the same parent, or that has no `SPEC.md` (for example, a goal-shaped directory under `examples/` or `testdata/`), is also code. The `amendments/` of that other goal directory is not an execution bundle file, but under every rules version it is left out of the code state regardless of the PLAN's `다른 목표 실행 묶음` declaration (§2). This list is used alike in every judgment that uses the bundle documents.
- Whether the working tree is clean: the §3 premise, the `tree_clean` that `check` and `baseline` leave, and path confirmations (`confirm`)
- Freshness: item 3 of §3
- Files changed since base: `out_of_scope` and `test_definition_changed` in §4.2
- Validity of the completion record: §5.1

The purpose is that, when several goals progress together on the same branch, this goal's results do not go stale just because another goal committed only its progress records. A goal that reads other goals' execution bundles as check input puts a `다른 목표 실행 묶음:` item in PLAN `## 범위` and writes `검사 입력` (check input) as its value ([bundle.md](bundle.en.md) §2.2). The declaration is read from the PLAN at the time of computing and recording.

**Completion with only the budget exceeded.** When all required conditions are satisfied and the only remaining reason is `budget_exceeded`, the status is `complete`, and `ha done` leaves a completion record. If even one other reason remains as well, the §4.3 order applies unchanged. For example, together with a user-confirmation reason the status is `budget_exhausted`, and both reasons appear. Budget usage and whether it was exceeded keep appearing in `budget`. This records completion with evidence already obtained; it is not authority to verify or work further beyond the budget. Further use needs grounds from the user. Under `/3`, a run increase in the same window is `budget_change`, and explicit rework or a new window is `reopen`; `/1` and `/2` use the existing `reopen` (§5.2). `ha check` does not check the budget before running.


### 4.7 run-rules/4

A goal started with `ha start --rules run-rules/4` keeps all the rules of `run-rules/3` and adds the following. A `run-rules/5` goal also follows all of this section and adds §4.8. The executor's rationale, wait reports and recovery reasons are claims, and Core does not verify them as facts. Core looks only at the records and their links. New notes and `--preapprove` are rejected with 65 and not recorded for `/1`, `/2` and `/3` goals, and the status JSON of those goals does not show the fields below. The new note options (`--reason`, `--criteria`, `--how`, `--moved-to`, `--weakened`, `--input`, `--change`, `--count`) are accepted only for the note kinds that use them; given to another kind, they are rejected with 64 regardless of the rules.

**Test change authority.** `--test-authority <해석>` records in `start.test_authority.interpretation` the executor's interpretation that reads the start request as test change authority. Without it, there is no authority. Regardless of authority, the `/4` start line stores copies of `바꿀 수 있는 경로` (paths that may change) and `테스트 경로` (test paths) from PLAN `## 범위`, defaults included, in `start.scope.allowed` and `start.scope.tests`. When a file that existed at base and matches `scope.tests` is modified or deleted, that is a test change. Editing PLAN after the start neither widens nor narrows this classification. A test change is satisfied without user confirmation when all of the following hold.
- There is authority, and the path is inside `scope.allowed`.
- The last `ha note <goal> test-change tests:<path> --criteria <AC-n,…> --how <확인 방법> [--moved-to <path> …] [--weakened]` note for that path states the condition IDs and how they are checked, and has no `--weakened`. The note is recorded only on a clean tree, and binds the file blob at that HEAD (`bound.blob`) and the blob of each moved-to path (`test_change.moved_to`).
- That path at HEAD is the same as what the note bound. For a modification the blob is the same; for a deletion the note was also bound to the deleted path (`bound.blob` is `deleted`). A rationale left for a modified version does not exempt a later deletion.
- For a deletion, the note states at least one new path the test moved to. For a modification or a deletion alike, each new moved-to path is inside `scope.allowed`, and its blob at HEAD equals the one in the note. If even one linked file changes again, the change is not exempt.

For each test change, `test_changes` in the status JSON shows `path`, `change` (`modified` or `deleted`), `blob`, `exempt`, `confirmed`, the reason it is not exempt `reason`, and the executor's claim `rationale` (`seq`, `claim: "executor"`, `criteria`, `how`, the `path` and `blob` of `moved_to`, `weakened`). The human-readable format also shows, per path, the final blob (`deleted` for a deletion) and the blobs of the moved-to paths, and `test_authority` shows `request_quote`, `interpretation` and `allowed`. Only paths that are neither exempt nor confirmed remain in `test_definition_changed`.

**Reported waits.** Under `/4`, `check` and `baseline` lines record the run start time `started_at` (RFC 3339, to the second). `ha note <goal> wait --reason <물은 결정>` starts a wait and `ha note <goal> resume` ends it. A wait runs from the second after the note's time to the earliest end: an `input`, `confirm` or `allow-recovery` note with a quote, a `budget_change` with a quote, a `resume` note, or the `started_at` of a run that started in the same second as the note or later. The order within one second cannot be known, so that run is taken to have ended the wait. A wait does not cross a window (`reopen`). Overlapping waits are counted once, and the run interval `[started_at, at]` is not subtracted even when it overlaps a wait. Finding lines do not affect waits or the budget. `elapsed_seconds` in the status `budget` is the budget's elapsed time (the window length minus reported waits), and `reported_wait_seconds`, `span_seconds` and `wait_basis: "executor_reported"` appear with it.

**Pre-approved limit.** `ha budget <goal> --preapprove <N> --window <W> --revision <R> --quote <인용> --context <문맥>` records the window's pre-approved limit as `budget_change.preapproved_runs`. N must be greater than the current limit. In that window, raising `--runs` to N or less needs only `--reason`. The status `budget.preapproved_runs` shows that value. A new window's limit is the start limit or the last limit set by a user quote (with later decreases applied); the pre-approval and the increases within it are not inherited.

**Error recovery.** `ha note <goal> recover <AC-n> --input check|baseline --change <바뀐 실행 조건>` is recorded only when the fresh errors and inconclusive results of that condition and input have exceeded the limit (E=1). It then stores in `recovery.clears` the seqs of the error and inconclusive lines counted in that input's limit, and only those lines leave that input's limit computation. The computation accepts only error and inconclusive lines of the same condition and the same input that precede the note, and ignores other seqs in the list. Other errors that became fresh again after the input (command, limit, check paths and so on) changed and came back are still counted. The same input's `fail`s and baseline `unexpected_pass`es, other conditions and other inputs stay as they are. Executor recoveries are 2 per condition and input. `ha note <goal> allow-recovery <AC-n> --input check|baseline --count <N> --quote <인용>` adds N. A recovery for an input that has not exceeded the limit, and a recovery with no remaining count, are rejected with 65. For each input that has a recovery or an allowance or has exceeded the limit, `recoveries` in the status JSON shows `criterion`, `input`, `errors` (the number of fresh errors and inconclusive results that no recovery named) with their seqs `error_seqs`, `limit_reached`, `used`, `allowed` and `remaining`. User input, budget changes, `reopen` and later passes do not lift the error limit.

**Per-command time limits.** The optional PLAN section `## 시간 제한` ([bundle.md](bundle.en.md) §2.5) sets the time limit of a condition or task command. The maximum is 7200 seconds; a declaration that exceeds it or has no target is a lint problem and is rejected before running. A baseline uses its condition's limit. The applied limit (in milliseconds) is part of the command digest, so changing the limit makes earlier results not fresh. An undeclared command gets 900 seconds.

**Pre-run notice.** Before running each target, `ha check` reads the earlier `check` and `baseline` lines of the same target (a condition, or a task and its command index), and prints one line to standard output before that target's result line.
- For the most recent run in which the limit fired or the limit interval use was 800‰ or more: `time-limit notice: <대상> seq <n> limit reached` or `time-limit notice: <대상> seq <n> <p>‰ of the limit`. If there was no limit interval and the value was computed from `duration_ms`, ` (approximate)` is appended.
- If there is no earlier run, or no earlier run that recorded its applied limit: `time-limit notice: <대상> risk unknown`.

`<대상>` (the target) uses the same notation as the result line (`AC-3`, `T002#1`). The notice line is exactly one of these three forms, with nothing added. So it never says a run is safe when there is no earlier run. The notice does not block the run, and neither writes a record nor uses a run.

### 4.8 run-rules/5

A goal started with `ha start --rules run-rules/5` keeps all the rules of `run-rules/4` (§4.7), and when the SPEC's `Assurance:` tier ([goal-docs.md](goal-docs.en.md) §2.5) requires independent review, it uses the recorded review gate as a completion condition. The default start rule stays `run-rules/3`; `/5` is chosen only with this option. For goals started under `run-rules/1` to `run-rules/4`, whether in progress or complete, judgment, status output and completion records stay as they were, and a `review` note is rejected with 65 and not recorded. The status JSON of a `/5` goal has the same fields as `/4`, with only the reasons below added.

**Recording a gate.** `ha note <goal> review --gate <file>` reads once the `review-gate/v1` file ([review-gate.md](review-gate.en.md)) written by Spar's `spar gate`, and stores only the following in the note line's `review`. Judgment reads only this summary and never rereads the file. The record is the executor's claim, and Core cannot prevent the executor from not recording a later gate. The assurance level is `local`.

| Field | Meaning |
| --- | --- |
| `gate_digest` | the sha256 of the file bytes |
| `schema`, `spar_version` | the values from the file |
| `state` | `complete` \| `degraded` \| `blocked` |
| `expect_head`, `last_reviewed_head` | the expected head and the last reviewed head |
| `rounds[]` | in the file's order: `stage`, `sequence`, `base`, `head`, `criteria_digest`, `criteria` (`name`, `sha256`), `required_reviewers`, `completed_required`, `dry_run` |
| `open` | the number of open `defect` and `requirement-question` findings |

The gate's `reasons`, `open_findings`, the rounds' `path`, `waived_reviewers` (including user quotes), `growth`, `written_at` and `previous_gate_digest` are not stored. Spar's record is the authoritative copy of the findings' text. Recording does not compare the file against HEAD, the working tree or the goal documents, and recording the same file again stores the same summary once more.

If the file is missing, is not a regular file or cannot be read; is not exactly one JSON object (including an empty file or trailing data); has a `schema` other than `review-gate/v1`; or lacks a field above or has the wrong type (a `state` outside the three values, a `stage` outside `plan`, `plan-delta`, `candidate` and `fixes`, a number that is not an integer or is negative, an empty `name` or a `sha256` that is not 64 lowercase hex digits in a `criteria` item), the command prints `ha: review gate <file>: <이유>; nothing recorded`, exits with 65 and records nothing. Other unknown fields are ignored. Giving `--quote` is 64 regardless of the rules, and on a `/5` goal a missing or empty `--gate` is 64.

**Judgment.** If the SPEC.md tier at judgment time is `standard` or `strict`, the reasons below are computed from the latest `review` note in the whole record (the last recorded gate). If the tier is `light` or there is no tier line, no gate is needed, and these reasons do not appear even when a gate was recorded. The tier is read from the current SPEC.md, not from the start record. Changing the tier changes the goal digest, so `spec_changed` and `confirm spec` apply as usual.

| Reason | When | Class |
| --- | --- | --- |
| `review_missing` | There is no `review` note. Other notes, such as `confirm` and `input`, do not stand in for a gate | executor fixes |
| `review_blocked` | The gate state is `blocked` | executor fixes |
| `review_degraded` | The gate state is `degraded` | executor fixes |
| `review_open_defect` | There is an open `defect` | executor fixes |
| `review_open_question` | There is an open `requirement-question` | user confirmation |
| `review_code_missing` | There is no `candidate` or `fixes` round that is not a dry run | executor fixes |
| `review_head_stale` | `last_reviewed_head`, or the `head` of the last code round in the list (dry runs excluded), differs from the current HEAD. Commit IDs are compared as they are, so a commit that changed only bundle documents is also a different head | executor fixes |
| `review_docs_stale` | The last code round, and for `strict` also the last `plan` or `plan-delta` round, did not receive all of the current goal documents | executor fixes |
| `review_plan_missing` | The goal is `strict` but there is no `plan` or `plan-delta` round that is not a dry run | executor fixes |

A round received the current goal documents when, for each document in the goal document set ([goal-docs.md](goal-docs.en.md) §1), the round's `criteria` has an item whose `sha256` is the sha256 of the current content and whose name is one of these two: the document's repository-relative path (`/`-separated), or its name with symbolic links resolved (the repository-relative path if the resolved file is inside the repository, the file name if it is outside). The latter is how Spar names criteria documents. Extra criteria documents or a different order are fine. Spar's `criteria_digest` and the SPEC digest have different definitions, so they are not compared. All goal documents are given with `--criteria` in the repository under review. "Last" refers to the round order in the gate file.

The gate state `complete` is what covers checking the fixes chain and the expected head ([review-gate.md](review-gate.en.md) §2). Core does not recompute the chain, and does not judge whether findings are right or wrong.

If any of these reasons exists, `ha done` does not record completion. Recording a `review` note after a completion record invalidates that completion record, as `reopen` does, and judgment is made again with the new gate (§5.1). If another `ha` appended a line while `ha done` was computing the status, that `done` line's `record_head` differs from the line just before it, and that completion record is invalid too. This keeps a `review` note appended in between from sitting unjudged before the completion record. While a valid completion record exists, these reasons are left out of the status reasons. That completion record fixes the code state and goal documents it judged, so committing only bundle documents after completion keeps the completion. If the stored `review` summary has a `gate_digest` that is not a sha256, a `schema`, `state` or `stage` outside the values above, a negative number, a `criteria` item without a name or sha256, or a field from the table above that is missing or `null`, the reason is `record_invalid`. Only `ha` writes the record, so such a summary is a hand-edited record.

To finish without review, the user decides a goal document change that lowers the tier to `light`. After that, the goal completes without a gate through `confirm spec` and fresh checks.

## 5. Completion record and budget

### 5.1 Completion record

When `ha done` computes `complete`, that `done` line is the completion record. A completion record fixes the fact that "`ha` confirmed completion at this code state, this SPEC and this record head".

A completion record d is **valid** while all of the following hold.
- The working tree is clean now.
- There is no difference outside the bundle documents between d.commit and HEAD. The bundle documents follow the rules version (§4.6).
- The SPEC digest and the command digests of the required conditions are the same as in d. For a `/3` change, the structured declarations and producer inputs must also have the same meaning.
- There is no `reopen` note after d. Under `/5`, there is no `review` note either, and d's `record_head` is the line just before d (§4.8).
- The `prev` links of d and the lines before it are intact. If they are broken, the completion record is not evidence either (`record_integrity`).

With a valid completion record, the status is `complete`, regardless of how much time has passed since. When a completion record becomes invalid (the code or SPEC changed, rework was requested), the record remains as history, and the status is computed again per §4.1–§4.3.

Under `/5`, the computation that records completion also checks the review reasons (§4.8).

Even when the result of `ha done` is not `complete`, the `done` line remains. It is a record that "completion was claimed but it was not complete".

### 5.2 Budget

The budget applies to work in progress. Editing PLAN alone does not change in-progress limits. The number of check runs is the number of `check` and `baseline` lines, not the number of CLI calls. Failures, errors, inconclusive results and stale results count if they were actual attempts, and baselines are not exempt. Rejection before running and worktree preparation failures are kept apart from check runs. Under `/3`, a rejection before running because a declared overlay file cannot be prepared creates no check record and no usage.

- **Window**: from the last `start` or `reopen` to the last saved record. Check usage is attributed by the order in which records were appended. A check that started before a resume and was recorded after it is included in the new window.
- **Finding records**: `run-finding/v1` lines are excluded from run budget input. The sequence numbers and head of the whole chain stay as they are. Registering, judging or correcting findings alone does not change usage, window, limits or elapsed time. The meaning of existing `run/v1` data is kept.
- **Elapsed time**: the `at` of the last record subject to the run budget minus the `at` of the window start. It does not grow by querying the current time. When a new record is appended, including a budget change, the time spent waiting is included too.
- **Reached and exceeded**: when usage equals the limit, the limit is reached; when `사용량 > 상한` (usage > limit), it is exceeded. Remaining runs are never shown below 0, and the excess is shown separately.
- **No forced limit before running**: the budget does not reserve concurrent run slots, and does not terminate running checks or forcibly block checks from starting. Completion with only the budget exceeded is not authority for further runs.

Under `/1` and `/2`, the limits are always `start.budget`. A `reopen` that quotes the user's words opens a new window with the same initial limits.

A change compares against `budget.window_start`, `revision` and `runs_limit` from `ha status <goal> --format json`. For example, the change command for a synthetic goal with current window 1, revision 1 and limit 50 is the following.

```bash
ha budget docs/specs/example --runs 100 --from 50 --window 1 --revision 1 --quote "검증 상한을 100회로 늘려줘" --context "이 목표의 현재 창 총상한 50회를 100회로 변경"
```

If the window or revision changed after the status was read, the command is rejected. Do not update the numbers at will and retry; check the changed status and the scope the user's request applies to.

A `/3` `budget_change` changes only **the total run limit of the current window**. Changing 48/50 to 100 gives 48/100 with 52 remaining. Lowering it to 40 gives 48/40, 0 remaining and 8 in excess. The elapsed-time limit, window start and actual usage are kept, and the change itself is not a check run. An increase needs the user's original words and the request context supporting the target and the limit; a decrease leaves a reason. The agent's interpretation is kept apart from the original words, and a quote whose host source is not verified is a claim recorded by the executor. A short agreement or an invocation token is not rejected on its form alone.

A change is linked to the current window, the previous budget revision and the previous limit. Even when check records are added to the same window, usage is not taken out. If another change or a resume changed the window, revision or limit, nothing is recorded and the conflict is reported. Changes are never summed or reapplied automatically. An empty change, the same value, a value that is not positive, and an unsupported time change also create no limit change record.

A budget change alone does not create rework, a new window or an unblock, and does not reset failure or error limits. A window with a completion record needs an explicit resume. Under `/3`, `reopen` inherits the latest valid run limit and the initial elapsed-time limit, and counts the new window's usage. The previous window's usage and change history remain.

Even if an output file disappears or renaming it fails, under `/3` the actual attempt is preserved as an `error` record with `process_output` and an empty output reference. This does not mean that writing to the record store itself is guaranteed to succeed.

When syncing or closing fails after a record write, the write may already have been applied. Such a failure is never assumed to be a success or to be unapplied, and it is not applied again without checking the stored state. Lossless records are not guaranteed when the storage is damaged or cannot be written.

Under `/2` and `/3`, completion can be recorded when all required conditions are satisfied and only the budget excess remains (§4.6). A rework request after completion is recorded with `ha note reopen`, which invalidates the earlier completion record.

## 6. Rules

1. The run record is append-only. A failed run is never deleted or covered over by running again. Only errors can be run again, within the limit of §4.1. `ha` appends nothing more to a record whose link is broken.
2. Test results from runs outside `ha` are not used in status computation.
3. Completion is declared only after `ha done` has left a completion record.
4. The content of a `note` is a claim. `confirm` and `reopen` must quote the user's words, and they are shown in reports as "user confirmation (claimed)". A `confirm` takes effect only while the `bound` of §2.4 is the same as now.
5. Raw output is never put in committed files (the execution bundle, `runs.jsonl`). When a person or an agent needs to see raw output, use `ha log`.
6. The status computation rules are fixed at `start`. A change to the judgment rules is a new rules version, used from the next `start`.
