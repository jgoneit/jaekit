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
- Body marks: a cause is `가설`, `확정`, or `기각`; an action is `예정` or `반영`; a verification is `대기`, `통과`, or `실패` (for example `원인 [확정]: …`). Point to evidence by AC, commit, and seq; never copy check output.
- Append only: never edit an earlier entry. When a judgment changes, a new entry says what was rejected and on what grounds. A small event that went from cause to verification at once may be one entry.
- Record what changed a judgment or the next action: a new problem, a cause confirmed or changed, a change of approach, an important verification result, blocks and resumes. Every resume counts: a new session taking the goal up, a cleared block, an answer to a `needs_user` question. A `재개` entry notes where `## 현재` differed from the actual state, if it did.
- Leave out routine edits and reads, repeated runs of the same failure, and progress reports with no change.
- Length: an entry runs about 3 to 6 lines and `## 현재` about 5 to 8 lines. This is editing guidance, not a limit; a complex cause analysis may run longer.

## Completion report

Written when `ha done` records completion, or when work stops at `needs_user`, `blocked`, or `budget_exhausted`:

1. The `ha done` output (completion) or `ha status --format md` output (stop), pasted unchanged. It contains results, reasons, digests, and seqs, never command output.
2. Per-condition evidence (condition, result, record seq) when the output above does not already show it.
3. Changed files and the conditions each served, marked as the agent's claim.
4. Remaining limits: assumed decisions, `manual` conditions, assurance `local`, and that the executor wrote the checks.
5. How to continue, when not complete.

Never paste verification output into any bundle file. Summarize the cause and point to `ha log <goal> <seq>`.
