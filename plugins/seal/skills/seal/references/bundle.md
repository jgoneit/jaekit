# Execution bundle

The bundle holds the changing plan and progress of one goal. Its single purpose: another agent, without the earlier conversation, can read it and finish the remaining work. Only the parts `ha` reads have a fixed format; other prose is free. Headings, column names, IDs, and enum values below are contract tokens and stay exactly as written.

```text
docs/specs/<goal>/
  SPEC.md, <aux>.md   goal documents (the user's; read only)
  PLAN.md             condition table, result declarations, scope, tasks, budget
  REVIEW.md           review of the plan
  tasks/T001.md       per-task detail (optional; small goals keep tasks in PLAN.md)
  PROGRESS.md         current state, task states, timeline, changes, blocks, notes, completion report
  runs.jsonl          run record (ha only)
```

Other files in the goal directory, such as check scripts or fixtures, are code: changing them makes earlier results stale.

## PLAN.md

### `## 조건표`

| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |

- `ID`: every `AC-n` from the goal documents exactly once. Do not restate the criterion text. Extra checks of your own are `EX-n` and are always optional.
- `종류`: `change` (new behavior), `maintain` (existing behavior kept), `manual` (cannot be checked by machine here).
- `검증 명령`: one code span, run from the repository root as argv without a shell. No pipes, redirects, `&`, `;`, `$`, or backquotes. Quote words with spaces. Point to a script in the repository when more is needed. For `manual`, write `—` and the reason.
- `검사 경로`: the check files that decide the condition, comma separated, no globs. Required for `change`: the baseline copies exactly these files from HEAD onto the base commit.
- `Task`: the task IDs that implement the condition.

Lint rules: every goal criterion appears once; every condition has a task; every task is grounded in a condition or is a prerequisite of another task; no dependency cycles; `change` has a command and check paths; `maintain` has a command; `manual` has a reason.

Behavior that must be preserved belongs in the goal documents as a required criterion. If it is missing there but must not break, raise it with the user in `REVIEW.md` under open decisions; an `EX-n` row cannot protect it.

### `## 결과 계약`

For `run-rules/3` change conditions, map `ID | 선언 경로` in a Markdown table. The repository-relative JSON declaration identifies required targets, expected violations and the producer. Include the declaration and every producer/configuration file in `검사 경로` so the baseline receives them. See [ha.md](ha.md) for the report contract and actual reference producer. Generic runner nonzero exits alone are insufficient. Existing `/1` and `/2` goals and ordinary maintain, manual and task checks keep their roles.

The declaration and every `producer_paths` input must be tracked regular files whose bytes match HEAD. Commit them before checking; do not put the product behavior being changed among the overlaid check files. An undeclared `/3` command can still run and record `unknown`, but cannot establish change completion. The template is a starting point: replace its placeholders with a producer that actually observes the criterion. No particular runner or adapter is required.

### `## 범위`

- `바꿀 수 있는 경로:` code-span globs relative to the repository root (`**` matches any depth), with the SPEC section they come from. Changes outside them need the user's confirmation.
- `테스트 경로:` code-span globs of test definitions, or `(기본값)` for `tests/**`, `**/*_test.*`, `**/test_*.*`, `**/*.test.*`, `**/*.spec.*`. Editing or deleting a test that existed at base needs the user's confirmation.
- `유지할 동작:` behavior to keep, from the SPEC's non-goals and current behavior.
- `다른 목표 실행 묶음:` optional, only when a check reads other goals' bundle files (`PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, `tasks/`): ``- 다른 목표 실행 묶음: `검사 입력` (reason)``. Under `run-rules/2` and `run-rules/3` those files leave the code state; the declaration keeps them in, so a change to them makes this goal's results stale. `ha` reads it from the plan as it is when it computes or records, so adding or removing it is a plan change. Any other value is the lint problem `other_bundles_invalid`.

### `## Task`

| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |

`ID` is `T001`; `목표` is a behavior, not "create a file"; `근거` lists condition IDs; `제안` is the `W` proposal it follows or `—` (explain differences in `REVIEW.md`); `선행` lists prerequisite tasks; `필수` is `예` or `아니오`; `문서` is `tasks/T001.md` or `—`.

### `## 예산`

- `검증 실행: <n>` — runs of `ha check`, baselines included, per budget window
- `경과 시간: <duration>` — such as `4시간`, `90분`, `2h30m`, measured between record timestamps
- `비용: <limit or 미관측>`

PLAN budget values initialize a goal. Editing them does not change an active budget. Under `/3`, a recorded `budget_change` changes only the current window's total run limit and retains usage and elapsed time. Increases require a user quote and context; decreases require a reason. An explicit `reopen` carries the latest run limit and the original time limit into a new window. `/1` and `/2` retain their original start limit. See [ha.md](ha.md).

### `## 읽을 자료` (when tasks stay inside PLAN.md)

Same four groups as a task document.

## tasks/T001.md

| Section | Content |
| --- | --- |
| `## 목표` | The behavior this task completes |
| `## 근거` | Condition IDs and the SPEC sections they come from |
| `## 선행 조건` | Prerequisite tasks and their results |
| `## 읽을 자료` | Four groups, below |
| `## 범위` | What changes and what must stay |
| `## 완료 조건` | Which conditions must pass. Task-level commands go in list items that hold only one code span; `ha check <goal> T001` runs them. They never decide completion |

Reading material groups:

| Group | What to leave |
| --- | --- |
| 필수 맥락 | The sections and code locations needed to understand the task |
| 필요할 때 참고 | Detailed design and long background |
| 판단 근거 | Which existing behavior or document a decision rests on |
| 충돌·미확인 | Places where documents and code disagree, facts not yet confirmed |

Summaries point to a document and section. They never replace a criterion.

## REVIEW.md

| Section | Check |
| --- | --- |
| 요구사항 대응 | `ha lint` result; SPEC scope and non-goals reflected in tasks; user scenarios covered by the condition table; differences from the work breakdown proposal and why |
| 불필요한 task | Tasks that serve no condition |
| 문서 충돌 | Conflicts among SPEC, project documents, and code. Never resolve them by editing the goal documents; ask |
| 중요한 미결정 | Open decisions in the goal documents and new outcome-changing ones. Close each with the user's quoted answer or an explicit assumption that the report repeats |
| 검증 방법과 환경 | Whether each command runs here; tools, data, network; how a cross-component flow is checked across the boundary |
| 결론 | `ready` or `needs_user` with the reason |

### Evidence and observation scope

Goals started with seal 0.1.10 or later use these additions in their existing review and report. Existing goals retain their starting format; absent fields mean unrecorded, not failed or unexecuted. Never convert old records or invent past observations.

- **Expected-result grounds**: the requirement, public contract or agreed invariant, with its location. If observed behavior or a reference implementation is the chosen standard, name the agreement and scope supporting that choice. Execution or a fake's expected value is not the authority for correctness.
- **Planned observation boundary**: the shell, DB, HTTP, model or other boundary and scope to observe; include only environments that affect the condition. Distinguish planned, not performed, and observed.
- **Method and substitutes**: synthetic inputs, fakes, fault injection and reference comparisons, including what they control or replace. One example is not the oracle for all inputs.
- **Failure classes not covered**: gaps in general properties, representative examples and environment assumptions. Bound any claim of no remaining risk; check counts are not a sufficiency score.

Map every condition to these axes and gaps. Shared condition groups may reference one explanation; repeated per-condition prose, duplicate tables and additional documents are not required. Show unresolved or conflicting grounds explicitly.

Connect the planned review to observed report. When the condition, check, or environment changes, identify the versions and limits of earlier evidence. Separate recorded facts from the executor's interpretation. Unavailable sources and blocked execution remain unobserved with reasons.

## PROGRESS.md

The check time and record head of `## 현재`, `## 타임라인`, and the table time rules apply to goals whose start record names seal 0.1.6 or later (the `skill:` line of `ha status`). A goal started with an earlier Seal keeps the PROGRESS form it started with.

| Section | Content |
| --- | --- |
| `## 현재` | A short, current summary (about 5 to 8 lines): the check time with its time zone (for example `2026-10-09T00:35:00+09:00`), the `ha status` or `ha done` result at that time with its record head as `ha status` prints it, seq and hash (for example `seq 21 (64b7bf74bd68)`; `none` before `ha start`, as `ha status` prints it; and the completion record seq when there is one), the work in progress, the open problems, the next work, and how to resume. Restamp the check time and record head whenever it changes. Whoever takes the goal up compares the record head with `ha status` to see whether the summary is stale: the same seq with another hash is another record, and the check time is for reference only. Past events stay out; point to `## 타임라인`. When `ha done` records completion, say the summary is as of that completion record and where to find what happens past the bundle's last commit (merge, review replies, checks after the merge), never as work in progress |
| `## Task 상태` | Table `Task \| 상태 \| 메모`. States: `todo`, `doing`, `done`, `dropped`, `blocked`. `done` is a claim; `dropped` needs a plan change entry |
| `## 타임라인` | Optional. Events that change a judgment or the next action, appended in time order (below). `ha` does not read it |
| `## 계획 변경` | Table `시각 \| 변경 \| 이유 \| 영향 \| 다시 검토한 것`, including kind changes. `시각` carries a time zone. One row is one event; when the event also has a timeline entry, the detail lives only in that entry and the row points to it by the entry's time, with a one-line summary |
| `## 막힘` | Table `시각 \| 원인 종류 \| 내용 \| 해소 조건`. Causes: `auth`, `permission`, `quota`, `environment`, `spec`, `other`. `시각` carries a time zone. One row is one event; when the event also has a timeline entry, the detail lives only in that entry and the row points to it by the entry's time, with a one-line summary |
| `## 개선 메모` | Optional notes on this skill, its templates, or `ha`. Claims; never act on them inside the goal |
| `## 완료 보고` | Below |

### `## 타임라인`

Optional. Append entries for events that change a judgment or the next action, so that another session learns why the work went the way it did. Entries are claims.

- Heading: `### <time> · seq <record head> — <kind> · <one-line summary>`
  - The time is ISO 8601 with its time zone (`+09:00`, or `Z` for UTC). Never invent a time you do not know.
  - The seq is the record head when the entry is written (the heading carries the seq only; the hash goes in `## 현재`; `seq none` while there is no record); an entry about a check run uses that run's seq. The record's hash chain backs the seq; nothing backs the time.
  - Kinds: `관측`, `원인`, `결정`, `조치`, `검증`, `막힘`, `재개`.
- Time and judgment: distinguish event time and writing time, and name the time source. An unknown event time stays unknown; the heading may use the measured writing time. Include remaining conditions, new facts, hypothesis evidence and uncertainty to reduce where relevant; not every tool call. Repeating the same result with unchanged code, environment and hypothesis is not progress. Explain a changed hypothesis, separated cause or blocked condition when judgment changes.
- Body marks: a cause is `가설`, `확정`, or `기각`; an action is `예정` or `반영`; a verification is `대기`, `통과`, or `실패` (for example `원인 [확정]: …`). Point to evidence by AC, commit, and seq; never copy check output.
- Append only: never edit an earlier entry. When a judgment changes, a new entry says what was rejected and on what grounds. A small event that went from cause to verification at once may be one entry.
- Record what changed a judgment or the next action: a new problem, a cause confirmed or changed, a change of approach, an important verification result, blocks and resumes. Every resume counts: a new session taking the goal up, a cleared block, an answer to a `needs_user` question. A `재개` entry notes where `## 현재` differed from the actual state, if it did.
- Leave out routine edits and reads, repeated runs of the same failure, and progress reports with no change.
- Length: an entry runs about 3 to 6 lines and `## 현재` about 5 to 8 lines. This is editing guidance, not a limit; a complex cause analysis may run longer.

## Assurance policy and counterexamples

For new goals started with Seal 0.1.11 or later, and requested material verification redesigns, REVIEW connects condition groups to risk (false-success impact, boundary, evidence gap), selected policy source, and required or optional guarantees. Explain exclusions instead of classifying by condition count. Preserve older formats and never fabricate retrospective reviews. No universal pre-baseline review order is prescribed.

Keep expected-result grounds, observed boundary, and methods/substitutes separate. A comparison identifies the same property, input scope, and environment, the observed results and differences. A reachable real system alone does not require a call for every condition; the claimed property and policy determine necessity. Prefer structured original evidence with identity links over inferred execution text. A fake result or a single real-model success does not establish the entire external contract.

Record a separate review's condition/check/code versions, provided context, role, inspectable reference, findings or no findings, not performed status, and limits. The initial input is the goal, public contract, check and expected grounds; it need not contain the implementer's solution narrative. Implementation and integration code may be inspected to follow actual paths. Different names or models do not certify independence or quality.

Distinguish a candidate from reproduced counterexample: record input, expected violated condition, why the current check misses it, reproduction or rejection evidence, repair and remaining gaps. A confirmed original-condition violation prevents completion; reporting risk alone does not repair it. Link the changed check to the missed general property and repair evidence. A scope-changing proposal remains a decision, not automatic implementation.

Generalize regressions to the shared input, state, or boundary property and state what they omit. Uninterpretable input is not successful evidence or an intended baseline violation. Preserve unobserved, environment error, skip, and observed violation separately. Explain evidence sufficiency separately from product allow/deny policy: uncertainty is neither execution absence nor an observed requirement failure.

Required review or actual-boundary evidence gaps keep completion pending, with cause, affected conditions, missing evidence and resolution. Unrelated useful work may continue within authority and budget; optional comparisons can remain unobserved with their risk. Do not invent performance, auto-install dependencies, expand permissions or spending, or mark affected required work done without its evidence.

When the goal, checks, code, grounds, environment, or assurance policy changes, preserve the reviewed versions and split the new applicable scope. In particular, do not reuse old review or comparison results as if they observed changed inputs. Native Agent chooses strategy; Core retains deterministic facts, local assurance, result distinctions, adverse history, authority and budget semantics.

## Post-completion discovery documents

Seal 0.1.12 introduces an optional `## 완료 뒤 발견` detail section or a reference to the single detail location. New reports can use it; existing goals keep their original format and missing means unrecorded, not zero defects. Do not migrate old records or overwrite the original completion report. New details may be appended to older documents without inventing historical execution.

A discovery identifies itself, target goal/code/completion reference, related condition or condition absent or unresolved, description, report source, reproduction evidence and observed scope, current confirmation claim, and follow-up. Unknown fields say unknown or none. Distinguish event time from recording time with their source; an unknown event time stays unknown. References identify permitted evidence locations; raw logs remain in existing local storage, not in shared bundle prose.

Distinguish candidate, confirmed, duplicate, dismissed, and resolved claims; separate a new requirement from an existing-condition violation. Preserve the original report and append the correction target, new judgment, reason, and evidence. A duplicate references its canonical finding; resolution links repair and recheck evidence, including unobserved scope. Report count is not a unique confirmed-defect count. These are document claims, not a machine-validated latest state or concurrency resolution.

The current summary shows historical completion, unresolved discoveries, and rework request/progress separately. A confirmed original-condition violation cannot be hidden behind a historical complete label. Keep detail in one place and link summaries rather than copying tables everywhere. Later fixes or checks never become evidence supposedly observed by the historical completion.

A discovery, confirmation, dismissal or resolution note in prose does not reopen work or change the completion decision, budget, or permission. An explicit rework request links its actual quoted source, affected finding and later result; distinguish requested, planned, in progress, complete, and not requested. This document contract does not implement structured Core discovery queries or state transitions. Its synthetic public example demonstrates traceability, not actual host compliance.

## Completion report

Written when `ha done` records completion, or when work stops at `needs_user`, `blocked`, or `budget_exhausted`:

1. The `ha done` output (completion) or `ha status --format md` output (stop), pasted unchanged. It contains results, reasons, digests, and seqs, never command output.
2. Per-condition evidence (condition, result, record seq) when the output above does not already show it.
3. Changed files and the conditions each served, marked as the agent's claim.
4. Remaining limits: assumed decisions, `manual` conditions, assurance `local`, and that the executor wrote the checks.
5. In the new reporting format, connect the planned review to observed report: expected grounds, actual boundaries, methods and substitutes, results, evidence and unobserved scope. Identify changed condition, check, or environment versions and evidence limits. State what was confirmed, not confirmed, and risky, with condition or shared-group references. A report does not establish deployment, installation or a real model call.
6. How to continue, when not complete.

Never paste verification output into any bundle file. Summarize the cause and point to `ha log <goal> <seq>`.
