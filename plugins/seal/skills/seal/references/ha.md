# ha — Seal Core commands

`ha` runs verification commands without a shell, appends `runs.jsonl`, and computes the goal state from the record, the bundle, and git only. The same inputs always give the same state; the current time is never used.

The commands below describe release v0.1.4: ha 0.1.4 and Seal 0.1.12. This release includes the optional post-completion findings extension; verify its additional capabilities on the actual executable before use. Seal 0.1.13 adds guidance for the optional decision history extension, which ha 0.1.4 does not provide; use it only with an executable that declares it (see [Decision history](#decision-history)). Core v0.1.3 does not provide that extension. Seal 0.1.14 adds check-authoring guidance ([What checks read](#what-checks-read)) without a Core change. Seal 0.1.16 starts new goals under `run-rules/4` ([run-rules/4](#run-rules4)), which needs a Core that declares it; the Core's default rule stays `run-rules/3`. Existing `/1`, `/2` and `/3` goals keep their saved rules; installation does not migrate their records or reset usage or time limits.

## Core compatibility

Resolve the actual executable selected for `ha` to an absolute path. Run that exact executable with `capabilities --format json`, and use the same path for later Core commands. Check again when its path or contents change. A successful version command is not compatibility evidence; neither is a response obtained from a different executable. Do not cache approval across such changes.

The response uses `schema: ha-capabilities/v1`, `ha_version`, `supported_rules`, `default_rules`, `formats` and `features`. These required fields must have the types and values described in the [Core capabilities contract](https://github.com/jgoneit/jaekit/blob/v0.1.4/contracts/core-capabilities.md). Missing or malformed information, an unsupported schema, query failure, missing executable or unmet requirements mean stop before writing any record. Identify the executable and the reason; never fall back to older rules, install tools or alter records automatically.

For a **new start**, require all of the following:

- `default_rules` is `run-rules/3`, and `supported_rules` includes it and `run-rules/4`. A different default is unsupported by this skill even if `/3` appears in the supported list. The start passes `--rules run-rules/4`.
- `formats.criteria` includes `legacy` and `nested/1`.
- `formats.check_declaration`, `formats.check_result` and `formats.check_evidence` include `check-declaration/v1`, `check-result/v1` and `check-evidence/v1`, respectively.
- `features` includes `estimate/v1`, `dirty-preflight/v1`, `baseline-safe-copy/v1`, `structured-change-results/v1` and `budget-change/v1`, and the five `run-rules/4` features `test-authority/v1`, `reported-wait/v1`, `preapproved-runs/v1`, `error-recovery/v1` and `command-time-limits/v1`.

For an **existing goal**, read its saved start rule and selected SPEC format without changing them. Seal 0.1.16 understands `run-rules/1` to `run-rules/4`; refuse any other saved rule even if Core lists it as supported. Require that rule in `supported_rules` and the selected format (`legacy` when absent) in `formats.criteria`. Resume and check operations require `baseline-safe-copy/v1` and `dirty-preflight/v1`; under `/3` they also require the three structured-result formats and `structured-change-results/v1`. A budget operation instead requires `/3` or `/4`, its three structured-result formats and `budget-change/v1`; `/1` and `/2` do not support that operation. Under `/4`, resume, check and budget operations also require the five `run-rules/4` features. A different default rule does not invalidate an otherwise supported existing goal. Do not require `/3` declarations for legacy goals or move ordinary maintain, manual and task checks to the structured contract.

For a **finding query or mutation**, also require `features` to include `post-completion-findings/v1` and `formats.finding` to include `run-finding/v1`. Resume, check and budget operations on a chain containing this extension require the same two additions. Finding operations keep the saved rule and selected criteria format; they do not require execution features or `/3` declarations just to register or read a claim. An unknown record schema means stop, leaving full chain validation to Core. Do not send new-format records to an unsupported executable or treat its ordinary completion output as a successful findings query.

For **decision history**, also require `features` to include `decision-history/v1` and `formats.decision_history` to include `decision-history/v1`, with the saved rule and selected criteria format supported. The history lives outside `runs.jsonl`, so these two additions are never required to start, resume, check, budget, or record findings. Without them, keep explanations in PROGRESS Markdown.

When Python 3 is already available, the optional [check_core.py](../scripts/check_core.py) helper checks this same contract. From the project directory, use `python3 <skill>/scripts/check_core.py --ha <actual executable> --operation start`, substituting the actual skill path. For an existing goal, choose `--operation resume`, `check`, `budget`, `finding` or `history` and add `--goal <existing goal>`. It reports `compatible`, `reason`, `executable`, `sha256`, `ha_version`, `operation`, `rules` and `missing`: exit 0 means compatible, 2 means refused and 64 means invalid arguments. Use the successful response's absolute `executable` afterward. Python is not required: the agent can read the actual Core JSON and apply the contract directly. Never treat missing Python as missing Core support.

This checks source contracts and executable support. Whether installed Codex or Claude models follow the instructions in practice is a separate host evaluation; capability output does not establish that result. Spec document writing requires neither Core nor this query.

## Commands

| Command | Use |
| --- | --- |
| `ha capabilities [--format json]` | Read-only support information without a goal or record; JSON is the default |
| `ha lint <goal>` | Lists goal document and bundle format problems, one per line: code, file:line, detail. Exit 1 when any exist |
| `ha estimate <goal> [AC-n \| EX-n \| T001 …] [--baseline] [--format md\|json]` | Reads the PLAN lower bound, limit and shortfall, plus default and selected runs. Executes nothing; does not change records or budgets |
| `ha start <goal> --request "<quote>" --skill <SKILL.md> [--rules run-rules/4] [--test-authority "…"] [--host-name …] [--host-version …] [--model …]` | Records the start once per goal. Refuses lint problems, a second start, and a PLAN limit below the required minimum before writing a start record. Without `--rules` the goal uses the default `run-rules/3` |
| `ha check <goal> [AC-n \| EX-n \| T001 …]` | Runs the conditions' commands at HEAD and records each result. With no targets it runs every required `change` and `maintain` condition |
| `ha check <goal> --baseline [AC-n …]` | Runs `change` conditions on a temporary worktree of the base commit with their check paths copied from HEAD. Expected result: `fail_as_expected` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote "…" --context "…"] [--reason "…"] [--interpretation "…"]` | `/3` and `/4`: changes the current window's total run limit while retaining usage; quote and context for increases, reason for decreases |
| `ha budget <goal> --preapprove N --window W --revision R --quote "…" --context "…"` | `/4` only: records the user's pre-approved run limit for the window; the limit itself stays |
| `ha note <goal> confirm <AC-n \| scope:<path> \| tests:<path> \| spec> --quote "…"` | Records a quoted user confirmation, bound to the current content. It loses effect if that content changes |
| `ha note <goal> reopen --quote "…"` | Records a rework request; voids the completion record and opens a new budget window |
| `ha note <goal> input --quote "…"` | Records any other user instruction or answer |
| `ha note <goal> block --cause <cause> [--quote "…"]` / `unblock` | Records a stop that cannot be fixed by retrying, and its end |
| `ha note <goal> test-change tests:<path> --criteria AC-n[,…] --how "…" [--moved-to <path> …] [--weakened]` | `/4` only: the executor's reason for changing an existing test, bound to the files at HEAD |
| `ha note <goal> wait --reason "…"` / `resume` | `/4` only: the start and end of a wait for the user that the executor reports |
| `ha note <goal> recover <AC-n> --input check\|baseline --change "…"` | `/4` only: resets an input past its error limit; earlier errors leave the limit, failures stay |
| `ha note <goal> allow-recovery <AC-n> --input check\|baseline --count N --quote "…"` | `/4` only: the user allows N more recoveries for that input |
| `ha status <goal> [--format md\|json]` | Prints the computed state |
| `ha done <goal> [--format md\|json]` | Computes the state and records it. When the state is `complete`, the line is the completion record |
| `ha log <goal> <seq> [--tail N]` | Shows the local output of one run. Output never leaves this clone |
| `ha timing <goal> [--format md\|json]` | Optional extension: a read-only view of each recorded run's duration, applied time limit, time inside the limit with its proximity, whether the limit fired, and CPU time. It executes nothing, and decisions and budgets never use these values |
| `ha finding <goal> [--format json\|md]` | Optional extension: read historical completion anchors, current findings, and explicit rework relations; default JSON |
| `ha finding <goal> --input <JSON file>` | Optional extension: append an explicitly sourced claim or revision and return a JSON receipt |
| `ha history <goal> add --kind <kind> --summary "…" [--status …] [--reason "…"] [--target …] [--ref …] [--checked-ref …] [--builds-on Dn] [--supersedes Dn[@Dm]] [--request-id …]` | Optional decision history: records one judgment event in one call and returns a JSON receipt. Never touches `runs.jsonl` |
| `ha history <goal> [current \| list \| show <Dn>] [--target …] [--keyword …] [--since <cursor>] [--page <cursor>] [--limit N] [--format md\|json]` | Optional decision history: reads current judgments, all events, or one event with its direct predecessors and successors; default `current` in Markdown |

`ha check` exits 0 when every result is as expected and 1 otherwise. Other exits: 64 usage, 65 refused, 66 unsupported rules version, 70 internal error. `ha status`, `ha done`, `ha check`, and `ha note` exit 66 for a goal started under rules this `ha` does not compute, and record nothing. A goal under `/1`, `/2` or `/3` refuses the `/4` note kinds and `--preapprove` with 65 and records nothing.

### Post-completion findings

Use the [findings contract](https://github.com/jgoneit/jaekit/blob/v0.1.4/contracts/findings.md) only after checking the exact executable's optional support. The goal is explicit; do not guess another goal from a review link. Base a revision on the existing findings and historical completion actually read. New entries start as candidates with a stable finding ID and request ID, exact completion seq/hash, historical criteria mapping, source and summary. Later judgments include the revision actually read, reason and evidence references. Core checks the relationships and stored structure, not whether a review claim is true. Document-only discoveries remain document claims until an explicit sourced registration; do not import free text as verified facts or edit the original completion report.

Report the historical completion, current unresolved findings, and linked explicit rework status together. Registering a finding preserves the original completion and creates no rework authority. Only a user's rework request authorizes `note reopen`; afterward a finding may reference that saved request. A later completion is linked only within that reopen's window, stopping at the next reopen, and does not automatically resolve a finding. Resuming an existing goal requires querying its current findings as well as its computed execution state.

On a revision conflict, read the stored history and reassess; do not overwrite the competing judgment. If writing or printing the receipt fails, the change may have applied. Requery by finding/request ID; an identical request can return its original receipt without an additional event. A broken chain requires explicit recovery, not an automatic retry. Old binaries reject the extension as record integrity failure, so retain a supporting executable for later queries and writes. No automatic migration, installation, reopen, or budget reset is implied.

### Decision history

The [decision history contract](https://github.com/jgoneit/jaekit/blob/main/contracts/decision-history.md) defines an optional, local record of judgment events for one started goal, kept in the Git common directory per start record and shared by every working tree of the repository. Use it only after checking the exact executable's support. It needs an existing start record and never starts a run.

The current-source Windows x64 local candidate provides the same add, replay, correction, default/current, list and event-ID/request-ID show behavior as macOS/Linux. Ordinary checkouts and linked worktrees share the common-directory history and lock, including local paths with spaces and Unicode. Windows ARM64/x86, network/UNC repositories and sharing one checkout between Windows and WSL are outside this candidate's scope. Windows Server developer CI through PowerShell 5.1 and 7 is distinct from Windows 11 installation, host Skill use and public release availability; see the [Windows candidate guide](https://github.com/jgoneit/jaekit/blob/main/guides/WINDOWS.en.md).

Windows junctions and other reparse points in history storage are refused as `history_unreadable`, including parent, event, last-event, temporary and lock paths. Queries observe a complete state before or after concurrent registration, waiting when a registration lock exists. An absent storage directory may report the earlier absent state; an in-progress initial registration is not mistaken for damage. Process exit releases its lock; an abandoned lock file alone never blocks later access forever. An interrupted partial event remains damage, and a failure after an attempted append retains `write_uncertain` semantics without automatic repair. File evidence that passes the existing relative-path grammar but denotes a Windows device, named pipe, alternate data stream or drive-relative path is `unreadable` with no version, without opening or waiting on it. This observation does not reject an otherwise valid event or rewrite past references. Missing, nonregular and outside files retain their distinct states.

- **Recording.** One `add` call records one event: a kind from the timeline tokens (`관측`, `원인`, `결정`, `조치`, `검증`, `막힘`, `재개`), an optional claim state for that kind (`원인`: `가설`/`확정`/`기각`; `조치`: `예정`/`반영`; `검증`: `대기`/`통과`/`실패`), a one-line summary, an optional reason, targets (`AC-n`, `EX-n`, `T001`), and references `run:<seq>`, `file:<path>#<section>`, `commit:<sha>[@<repository>]` or `external:<reference>` (`--checked-ref` claims the content was read). A plan change is a `결정`; a correction is a relation, not a kind. Record the events the timeline rules name; small events may be folded into one. Relations, kinds and order are never required steps.
- **Relations.** `--builds-on Dn` links an earlier stored event that grounds this judgment. `--supersedes Dn` corrects, rejects or replaces an earlier judgment; when Dn already has a stored correction Dm, write `--supersedes Dn@Dm` after reading it. A different stored latest correction is `correction_conflict`; reread instead of overwriting. Both relations point only at events already stored in the same history.
- **File evidence.** New `add` requests and the current comparison in `show` identify only content actually read safely from a regular file inside the repository. Replacement of the file or a parent path must not redirect the read outside it or to a final symlink or nonregular file, including a FIFO that could wait for a writer. Exactly 64 MiB is allowed; a file that starts or grows beyond the limit is `too_large`, with only bounded extra reading to detect the limit. An unavailable version does not reject an otherwise valid event. `show` preserves the stored observation: a previously identified version that cannot now be read safely is `unavailable`; an originally unidentified version stays `unidentified`. `current` and `list` do not reread file contents. An identical request returns its original receipt before rereading evidence. No atomic snapshot of concurrent content changes or repair of past observations is promised.
- **Current judgments.** `current` computes the judgments not superseded within the selected run lineage before applying `--target`, `--keyword`, `--since` and pages, so a filter never resurrects a replaced judgment. Events of a diverged or unverifiable run chain are listed apart, not merged. A judgment whose `builds_on` grounds have a correction is marked `linked grounds … corrected by …` without automatic rejection. Preserve all correcting IDs in stored order within the selected lineage and history boundary, whether stored before, in, or after the judgment; timestamps do not filter them. For example, D1 corrected by D2 before D3 builds on D1 still marks D3 with D2, and one event may both build on and correct D1. This has the same meaning in default/current, list and both show forms; JSON `grounds_corrected` is unchanged. Filters do not hide these corrections, and page cursors keep the original boundary. Not being superseded and a matching context do not make a judgment true or applicable. A missing follow-up is not evidence of unfinished work.
- **Resuming.** Read `current` (and `show <Dn>` for the parts you need) instead of the whole history. Keep the returned `--since` cursor; a later `--since` query lists new events plus replacement notices for matching judgments even when the correcting event is outside your filters. Follow `next` page cursors of one query; a stale cursor asks for a fresh query.
- **Failures.** A receipt with `replayed: true` means the request was already stored. `write_uncertain` may have applied, or names an event that was stored before its last-event record could be updated: repeat the same request with the printed `--request-id` or `show --request-id` it before writing again. Damaged or unsupported history stops queries and writes; do not repair or truncate it automatically. A missing, null, empty or ambiguously keyed schema is damage. An unsupported start rule or history schema yields exit 66 and `history_unsupported:` with an explanation for every otherwise valid add, replay, default/current, list and show request. It changes no existing data and creates no history storage, even when history is absent. File evidence `too_large` is an unidentified observation, not the input rejection `limit_exceeded`. Removed trailing events count as damage, so their IDs are never reused; restoring the whole history directory from an older copy is not detected. Absence of a history file does not prove nothing was ever recorded.
- **Boundaries.** Events are executor claims. Recording or correcting them changes no run record, rule, completion, approval, reopen, budget, check or finding, and adds no approval step. References certify neither truth, test results, review, nor user approval. Raw logs are referenced by run seq, never copied.

## Before execution

The minimum is twice the required change count plus the required maintain count; manual conditions contribute zero. Default baseline selection also includes optional change rows. Explicit optional AC, EX and task commands add runs, and repeated targets count as repeated executions. Estimates use the current PLAN limit, not an existing run's remaining budget. They exclude retries, preparation and manual time and guarantee neither total time nor completion. `estimate` exits 1 for a shortfall or lint problems; `start` exits 65 for an insufficient limit. Neither automatically raises a limit or changes an existing budget window.

Before each check, baseline or task process, `ha` inspects the working tree using the goal's exclusions. Dirty paths refuse that target and the remaining targets with exit 65; failed Git inspection exits 70. Diagnostics list repository-relative paths with tracked/untracked and observed symlink details. Review the specific path; do not automatically ignore or revert it. A refused target creates no check record, run usage or output file. Already executed results and usage remain, including errors and results that later become stale. This admission rule applies to future executions for all supported rule versions; historical records retain their meaning. Dirty `start` remains a warning.

## Structured change results

New starts use `run-rules/3`. Existing `/1` and `/2` goals retain their result semantics. Ordinary maintain, manual and task checks keep their roles. A `/3` change condition maps to a tracked JSON declaration in PLAN's `## 결과 계약` table (`ID | 선언 경로`). Include the declaration and its producer files in the condition's check paths. These tracked regular files must match HEAD byte for byte, even within an excluded bundle path; symlink inputs are refused.

The `check-declaration/v1` object contains `producer`, `producer_version`, `producer_paths` and required `targets` (`id`, expected `violation`). The producer writes `check-result/v1` to `HA_EVIDENCE_PATH`, echoing `HA_EVIDENCE_INVOCATION` and `HA_EVIDENCE_DECLARATION_DIGEST`. It reports `attempts_complete: true` and every observation for each required target, with consecutive `attempt` numbers starting at 1 and `status` of `pass`, `violation`, `error` or `skip`. The reference producer makes one attempt; integrations with retries must preserve the full history, with at most 4096 observations. Missing, duplicated or reversed attempt numbers are inconclusive. Pass and violation on the same target retain a `fail` with `inconsistent_attempts`, even when the process exits 0. No report, mismatched identity, unsupported values, skipped targets or contradictory process exits are positive evidence.

Current success needs exit 0 and every target passing. Expected baseline failure needs nonzero exit and a declared violation, every required target observed, and no environment error or unintended assertion failure. Missing implementation can be a violation directly observed by a runnable check; import, collection, compilation and browser startup errors are not such evidence. `unknown` shares the error rerun limit. Later positive results do not erase adverse results on the same inputs, including observed assertion failures retained within an error or inconclusive report.

The [public contract and executable reference](https://github.com/jgoneit/jaekit/blob/v0.1.4/contracts/check-result.md) describe the JSON fields and `tools/check-result-reference.py`. The Python reference reads actual files and reports observations, without installing runner adapters. A generic script's nonzero exit alone is unsupported evidence. The compact `check-evidence/v1` summary preserves the classification without raw logs or file contents. An output-file failure following an actual attempt records `error` with `process_output` and retains its usage when record storage is writable; `output_path` and `output_digest` may be empty. Record-storage failure itself is not reported as successful recording, and a failed write can have uncertain application. Declaration, producer and target changes invalidate old evidence; missing raw logs or changing the current time do not. These are executor-authored local claims.

Since release v0.1.3, direct and Homebrew installations include the optional Python reference producer and a synthetic file-observation example. Follow [Get the reference check](https://github.com/jgoneit/jaekit/blob/v0.1.4/guides/INSTALL.en.md#get-the-reference-check) to copy them into the project without a development checkout. The reference needs Python 3; choosing it is optional and does not make Python or any particular runner a prerequisite for Seal.

## run-rules/4

A goal chooses `run-rules/4` with `ha start --rules run-rules/4`. It keeps every `run-rules/3` rule and adds the following; the guidance on when to use them is in the skill's autonomy section.

- **Test-change authority.** `--test-authority "<reading>"` records the executor's reading of the start request as permission to change tests. A `/4` start line also keeps `scope` (`allowed` and `tests`, the plan's paths at start); test files are classified by that copy, and later plan edits do not change the classification. A changed base test inside `scope.allowed` needs no confirmation when its last `test-change` note names conditions and a check method, is not marked `--weakened`, and is bound to the file as it is now: its current blob, or its deletion. Every `--moved-to` path must stay inside the authority and unchanged since the note, whether the original remains or not, and a deletion needs at least one. Status shows each changed test under `test_changes` (path, blob, `exempt`, the reason it is held, and the executor's `rationale` with each moved-to path and blob) and the authority under `test_authority`; the Markdown form shows the same versions. Each new note option belongs to its note kind, and another kind refuses it with 64.
- **Reported waits.** `/4` check and baseline lines keep `started_at`. A `wait` note starts a wait at the next second; it ends at the earliest of a quoted answer (`input`, `confirm`, `allow-recovery`, a quoted budget change), a `resume` note, or the `started_at` of a run that begins in the note's second or later, and never crosses a window. Overlapping waits count once and runs stay elapsed. `budget` shows `elapsed_seconds` (the budget figure), `reported_wait_seconds`, `span_seconds` and `wait_basis: "executor_reported"`.
- **Pre-approved runs.** `--preapprove` records the user's limit for the window; an increase within it needs `--reason` and no quote. `budget.preapproved_runs` shows it. A new window starts from the last limit the user set and carries no pre-approval.
- **Error recovery.** `recover` is accepted only for an input past its error limit with a recovery remaining: two per condition and input, plus what `allow-recovery` adds. The note keeps the error and unknown records that counted toward that input's limit when it was recorded (`recovery.clears`), and only those leave the limit; errors of an input that changed and came back count again. `fail` results, unexpected baseline passes and other inputs are unaffected. Status lists `recoveries` (`errors` no recovery named and their `error_seqs`, `limit_reached`, `used`, `allowed`, `remaining`).
- **Command time limits.** The plan's optional `## 시간 제한` section declares limits such as `- AC-3: 30분` or `- T002 명령 1: 20분`, in 시간, 분 and 초, up to 7200 seconds; a larger or unknown entry is a lint problem. Baselines use their condition's limit. The command digest binds the applied limit, so changing it makes earlier results stale. Undeclared commands keep 900 seconds. Before each run `ha check` prints `time-limit notice: <target> …` for the latest earlier run that reached its limit or used 800‰ of it (`approximate` when only `duration_ms` is known), or `risk unknown` when no earlier run kept a limit. The notice runs nothing and uses no budget.

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

## What checks read

`ha` uses the base commit in three places: the baseline, which runs `change` conditions on a temporary worktree of the base commit with their check paths copied from HEAD; `out_of_scope`, the files changed since base outside `바꿀 수 있는 경로`; and `test_definition_changed`. Checks themselves judge the tree they run in and never read the base commit or other history, so the same check gives the baseline's violation and the later pass. The exception is a condition about history or refs itself, such as a tag's type, whose check reads only the range and refs the condition names. See [bundle.md](bundle.md#check-inputs).

## Freshness

A check result counts only while all of these hold: the tree is clean now and was clean when it ran; the code outside the bundle documents equals its commit; the goal documents and the condition's command are unchanged. A baseline counts while it ran on the start's base commit, its check files equal HEAD's, and the goal documents and command are unchanged. Committing only bundle documents keeps results fresh.

Under `run-rules/2` and `run-rules/3`, the bundle files of other goals count as bundle documents too: `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, and `tasks/` of every other directory with a `SPEC.md` under the same parent. That holds for freshness, the clean tree, scope, and the completion record, so another goal on the same branch committing its progress leaves these results fresh. Their `SPEC.md`, linked documents, and `checks/` stay code. When a check of this goal reads other goals' bundle files, the plan declares it in `## 범위` with ``다른 목표 실행 묶음: `검사 입력` ``; with the declaration those files stay in the code state. `ha` reads the declaration from the plan as it is when it computes or records. Under `run-rules/1` every other goal's file is code.

Every status output states assurance `local`: an agent with the same user permissions can change checks and records.

## Final head

`ha check` runs each condition's command as its own process, so conditions that need the same full regression each run it. It is recommended that one condition owns it and the others observe their own property; this is not a required procedure, and it does not drop or weaken a condition (Seal bundle reference, final candidate evidence).

A completion record covers its commit while it stays valid and `ha status` names it: among other conditions, the code outside the bundle documents equals HEAD and the goal documents and required commands are unchanged. When the user asks, after completion, to apply remote review or pull request comments, record that request with `ha note <goal> reopen --quote "…"`. After the fixes are committed, a code change leaves earlier `check` records stale, and a baseline stays fresh while it ran on the start's base commit and its check files equal HEAD's. Re-record the stale required checks on the final head and run `ha done` again before the head is sent or merged. Comments without such a request do not reopen the goal.
