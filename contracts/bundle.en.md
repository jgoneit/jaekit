# Contract: Execution bundle

[한국어](bundle.md) · **English**

> This is a translation of the [Korean original](bundle.md). Where the two differ, the Korean original decides.

This document is the canonical definition of the execution bundle's files and fields. The execution bundle is Seal's output. The goal documents the execution bundle reads (Spec's output) are in [goal-docs.md](goal-docs.en.md). The format of the run record (`runs.jsonl`) is in [run-record.md](run-record.en.md).

The execution bundle holds one goal's **changing plan and progress**. It has one purpose: **another agent, without the earlier conversation, must be able to read only this bundle and finish the work from what remains**. Adding more documents is not the purpose.

## 1. Location and ownership

```text
docs/specs/<goal>/
  SPEC.md        목표 문서 진입점: 조건 ID, (선택) 작업 분해 제안 (Spec이 작성, 사용자 소유)
  <보조 문서>.md SPEC.md가 링크한 목표 문서 (Spec, 사용자 소유)
  amendments/    수정안 문서 (Spec이 작성, 사용자가 정하기 전까지 목표 밖, goal-docs.md §6.6)
  PLAN.md        조건표, 범위, task 목록, 예산
  REVIEW.md      실행 전 검토
  tasks/T001.md  task별 상세 (작은 목표는 PLAN.md 안에 합쳐도 된다)
  PROGRESS.md    현재 상태, task 상태, 타임라인, 계획 변경, 막힘, 완료 보고
  runs.jsonl     실행 기록 (`ha`만 덧붙인다)
```

| File | Writer | When it may change |
| --- | --- | --- |
| Goal documents (`SPEC.md` and the auxiliary documents it links) | The user (written with Spec) | Only when the user decides. A change changes the goal digest, and every record becomes stale. Seal only reads them. |
| `PLAN.md` | The executor | Any time. The condition table, however, refers to the goal documents' condition IDs, and plan changes are recorded in `PROGRESS.md`. |
| `REVIEW.md` | The executor (a review subagent may be used) | Before execution, and after a plan change with a large impact |
| `tasks/` | The executor | Together with a plan change |
| `PROGRESS.md` | The executor | Any time |
| `runs.jsonl` | `ha` only | Append only. |

Other files in the goal directory (verification scripts, fixtures and so on) are treated as **code**, not as bundle documents. They enter the freshness and scope checks as they are ([run-record.md](run-record.en.md) §2). Even when a file that affects verification sits in the goal directory, a change to it makes earlier results stale. Whether another goal's execution bundle files under the same parent enter the code state depends on the rules version ([run-record.md](run-record.en.md) §4.6). As an exception, the amendment documents in `amendments/` are neither goal documents nor code. Even when SPEC.md links them, they are not part of the goal document set, and both this goal's and those of other goals under the same parent are left out of the code state whatever the rules version ([goal-docs.md](goal-docs.en.md) §6.6, [run-record.md](run-record.en.md) §2).

Every file is plain Markdown. Only the parts `ha lint` must read follow the table formats below. Other prose is free.

## 2. `PLAN.md`

### 2.1 Condition table (`## 조건표`)

| Column | Meaning |
| --- | --- |
| `ID` | The goal documents' `AC-n`. Do not rewrite the condition text. `ha` reads it from the goal documents. Whether the condition is required also follows the `(선택)` (optional) mark in the goal documents ([goal-docs.md](goal-docs.en.md) §2.1). Checks the executor adds are `EX-n` and are always optional. |
| `종류` | `change` (new behavior), `maintain` (existing behavior kept), `manual` (cannot be checked by machine in this environment) |
| `검증 명령` | The command to run from the repository root, written as one code span. `ha` runs it as argv without a shell, so do not use pipes (`\|`), redirects, `&`, `;`, `$` or backquotes. Words are split on spaces and can be grouped with single or double quotes. When it is complex, point to a script in the repository. For `manual`, write `—` and the reason. |
| `검사 경로` | Paths of the check files that judge this condition (no globs). Required for `change` conditions. The baseline check overlays only these files on the base. |
| `Task` | The list of task IDs that implement this condition |

The rules are as follows (`ha lint` checks them).
1. Every `AC-n` of the goal documents appears in the condition table exactly once. A missing ID, and an `AC` ID that is not in the goal documents, are errors. A condition without an ID in the goal documents is an error, and the executor tells the user instead of editing the goal documents.
2. Every condition has at least one task. Every task rests on a condition or on another task's prerequisite output. An orphan task is an error.
3. A `change` condition has a verification command and check paths. A `maintain` condition has a verification command. A `manual` condition has a reason.
4. To use the whole existing test suite as a preservation condition, add one `maintain` condition. When the goal documents have no such condition, add it as `EX-n`. `EX-n` is the executor's extra check, so it does not block the status. Behavior that must be preserved must be a required condition in the goal documents ([goal-docs.md](goal-docs.en.md) §2.3). When there is no such condition but you see behavior that must not break, ask the user in the pre-execution review (`중요한 미결정`, important open decisions, in §4).

### 2.1.1 Result contract (`## 결과 계약`)

Link `run-rules/3` change checks to the [structured result contract](check-result.en.md). PLAN holds an `ID | 선언 경로` table that maps each such change condition to a repository-relative JSON declaration path. Include the declaration and the producer and configuration files in the condition table's check paths as well. A plain nonzero exit alone is not an expected failure, and ordinary maintain, manual and task checks are not forced over to this contract. Existing `/1` and `/2` goals keep their earlier result semantics.

The declaration path may also point to a runner adapter declaration (`check-adapter/v1`) instead of a producer declaration (`check-declaration/v1`). Then each target is linked to one test that already exists, and the verification command is the runner command itself that uses `go test -json …` or JUnit XML. There are no producer files, so the check paths hold the declaration and the linked test files. The format, the output location (`{HA_JUNIT_XML}`) and the judgment are in §5 of [change check results](check-result.en.md#5-runner-adapters). Use it only when the actual executable's capabilities list `check-adapter/v1`.

### 2.1.2 Check inputs and observation

These rules apply to checks written or changed from Seal 0.1.14 on. Which Seal version started the goal does not matter. They never require rewriting or converting an existing goal's checks, records, PROGRESS or REVIEW for this rule alone. They add no user confirmation, approval or question step. The executor applies the rules and judges whether to fix existing checks.

- **What a check reads.** Every check in the condition table (`change`, `maintain`, `EX-n`, task commands), and the producer, configuration and helper files its command runs or reads, judge only from the tree being checked and the external inputs the condition names. Do not put the goal's start or base commit, or any other fixed commit, into a check. The same goes for reading the base from the start record or Core output and using it. Do not judge by reading other commits, refs or objects of the repository. Examples are a diff against a past commit, a merge-base, a remote-tracking branch, a relative ref such as `HEAD~n`, a log range and another worktree. The general property is "a full clone and a history-less clone with the same tree give the same verdict". The history of a synthetic repository the check builds itself is not covered.
- **Maintain conditions.** A `maintain` condition observes the property the goal documents require (content, output, format, behavior) in the tree being checked. Confirming "no change since the base commit" in check code is not an observation of that property. When a path must not change at all, leave it outside `바꿀 수 있는 경로` (paths that may change). Core then reports changes committed since the base as `out_of_scope`, and only the user resolves them. A check can look at the property the file must keep, or compare the file with expected content written in a tracked check file.
- **Change conditions and the baseline.** A violation on the base commit is observed only by Core's baseline run. The baseline runs on a temporary worktree of the base commit with HEAD's check paths overlaid. Check code never imitates the baseline by checking out, reading or comparing the base commit. The same check observes the required property in the tree it runs on, so it gives a violation on the base and a pass after the change.
- **When history itself is the condition.** When a condition is about history or refs, such as a tag's type or a commit message format, the check reads only the range and refs the condition names. When that history is missing or cannot be read, the result is neither a violation nor a pass but an error or unobserved. A `change` check that produces a structured result writes that state in its result report. `maintain`, `EX-n` and task commands are recorded by exit code only, and every nonzero exit is recorded as a failure. So before `ha check`, the executor confirms that the named range and refs can be read. When they cannot, record no result. For a required condition or task, hold that condition and completion and continue independent work; when that is all the work left, record a block with cause `environment`. An `EX-n` check stays unobserved and does not block completion. This distinction is the same as the distinction in §4 among unobserved, environment error, skip and actual violation. Write in the pre-execution review why such a check is needed and the range it reads.
- **Declared boundaries.** A component the goal documents point to or intend to change may have declared its own scope or trust boundary. Examples are pinning to a specific version or release, preserving past verdicts, isolating imports and limited inputs. That declaration is current behavior, so keep it unless the goal documents explicitly say to change it. When the goal documents can be satisfied both by a reading that widens the component and by a reading that adds a separate component, choose the reading that keeps the declaration, leave it in the pre-execution review as an explicit assumption and repeat it in the report. Do not ask the user anew. When the goal documents explicitly say to change the component, keep new code inside the declared boundary. For example, a new import also goes through the same import boundary. When no reading satisfies both the declaration and the goal documents, follow the existing document-conflict rule.

### 2.1.3 Final candidate evidence

These are planning rules, not a fixed order. The executor chooses the method and the order.

- While exploring, use narrow checks that fit what changed. Their results are working information, not evidence.
- Write in PLAN that the expensive full verification that completion needs is obtained once with `ha check` on the final candidate. Running the full verification for development and then again for the record on the same candidate is a cost to avoid.
- `ha check` runs each condition's command separately. When several required conditions need the same full regression, it is recommended that one condition owns that regression and the other conditions observe their own properties. This is a recommendation, not a required procedure. It only assigns the shared run; it does not drop or weaken a condition, and each condition needs its own recorded result.

### 2.2 Scope (`## 범위`)

- `바꿀 수 있는 경로` (paths that may change): a list of globs. It is the basis of the scope check. Derive it from the SPEC's scope and non-goals, and write the SPEC section it rests on. Seal 0.1.23 or later puts the test and check files the plan will create, and their helper files, here from the start, so adding those files raises no scope question. Do not include paths the goal documents say must not change.
- `테스트 경로` (test paths): a list of globs treated as test definitions. It is the basis of the check for changes to existing test definitions. When it is empty, `ha`'s defaults (`tests/**`, `**/*_test.*`, `**/test_*.*`, `**/*.test.*`, `**/*.spec.*`) are used.
- `유지할 동작` (behavior to keep): what must be kept, from the SPEC's non-goals and current behavior
- `다른 목표 실행 묶음` (other goals' execution bundles): optional. Add it only when this goal's checks read another goal's execution bundle files (`PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, `tasks/`) as input. The value is the single code span `검사 입력` (check input). Example: ``- 다른 목표 실행 묶음: `검사 입력` (이전 목표의 PROGRESS.md를 검사한다)``
  - When it is declared, other goals' execution bundle files enter this goal's code state under `run-rules/2` and `run-rules/3` too. A change to those files alone makes earlier results stale, and changes to them also enter the scope check ([run-record.md](run-record.en.md) §4.6).
  - `ha` reads this declaration from PLAN as it is when computing the status and when recording. Added during the work, it applies from then on. Removed, it leaves other goals' execution bundles out again, and results that were stale because of them can become fresh again. Record adding or removing it as a plan change.
  - The item must start with exactly `다른 목표 실행 묶음:`. When other words follow this name or the colon is missing, or the value is different or has no code span, it is the lint error `other_bundles_invalid`.
  - In a `run-rules/1` goal, other goals' execution bundles are code already, so the result does not change.

Widening `바꿀 수 있는 경로` is a plan change. When the widened scope conflicts with the SPEC's non-goals or scope, however, it needs the user's decision.

### 2.3 Task list (`## Task`)

| Column | Meaning |
| --- | --- |
| `ID` | The `T001` form |
| `목표` | What behavior it completes. Avoid goals that are not behavior, such as "create a file" or "add a class". |
| `근거` | The condition IDs it implements |
| `제안` | The matching work breakdown proposal's `W` ID, or `—`. When the goal documents have a proposal and you planned differently, leave the reason in `요구사항 대응` (requirement mapping) of `REVIEW.md`. |
| `선행` | Task IDs that must finish first |
| `필수` | `예` (yes) \| `아니오` (no) |
| `문서` | `tasks/T001.md`, or `—` (when merged into PLAN) |

A cycle in the dependencies is an error.

### 2.4 Budget (`## 예산`)

- `검증 실행` (verification runs): the limit on the number of `ha check` runs within the budget window (baselines included). When the value is written exactly as `자동` (automatic), as in `검증 실행: 자동`, `ha start` sets it to `max(50, ⌈필수 최소량 × 1.5⌉)` (the larger of 50 and the required minimum × 1.5, rounded up), and when that exceeds the pre-approved limit written in `ha.json` in the user configuration folder (50 without one), it refuses the start without a record. When the `검증 실행` item holds a number, that number is used ([run-record.md](run-record.en.md) §1.5)
- `경과 시간` (elapsed time): the limit on the time from the first record to the last record within the budget window. Write it like `4시간`, `90분`, `1시간 30분` or `2h30m`. It is measured by record times, not by the current time. So it does not grow while you only query the status during a wait, but when a record is added to the same window after the wait, the waited time counts too. Time passing after completion does not cancel the completion ([run-record.md](run-record.en.md) §5)
- `비용` (cost): the limit, if it can be known. `미관측` (unobserved) when there is no means to observe it

The budget window starts at `ha start` or at the last `reopen` (the user's rework request). The Seal Skill sets the defaults. When the user sets values in the request, those values are used. When the user sets no budget, Seal 0.1.20 or later writes `검증 실행: 자동`; when the start stops because it needs the user's approval, Seal asks once, showing the computed value and the pre-approved limit, and writes the number the user approved in `검증 실행`. It does not ask the same question again for the same PLAN. Seal never writes or changes the pre-approved limit setting. A budget can be lowered before the start; raising it needs the user's decision. Editing PLAN during the work does not by itself change the limits. Under `/3`, `ha budget` changes the total run-count limit of the same window and keeps the usage and the time limit. An increase records the user's original words and context; a decrease records a reason. An explicit resume under `/3` carries over the latest run-count limit. `/1` and `/2` keep the original limit and the existing resume meaning ([run-record.md](run-record.en.md) §5.2).

In `/4` and `/5` goals, user waits the executor reports are left out of the elapsed time, and within the pre-approved limit the user set with a quote, the executor raises the run-count limit for a reason alone ([run-record.md](run-record.en.md) §4.7).

### 2.5 Time limits (`## 시간 제한`)

An optional section. It is read only in `run-rules/4` and `run-rules/5` goals; a goal started under other rules runs every command with 900 seconds even when this section exists. Write one per line in PLAN's `## 시간 제한` section, like `- AC-3: 30분`, `- T002 명령 1: 20분` or `- EX-4: 1시간 30분`.

- A target is a condition ID that has a command (`AC-n`, `EX-n`), or a task and a command number (`T002 명령 1`, the command order in the task document's `## 완료 조건`).
- A value is a positive integer with the unit `시간` (hours), `분` (minutes) or `초` (seconds), or a combination of them. The maximum is 7200 seconds (2 hours).
- A value over the maximum, a malformed value, a target that does not exist and a second declaration for the same target are the lint problem `time_limit_invalid`, so `ha start` and `ha check` refuse before running.
- A command without a declaration gets 900 seconds. The baseline runs with that condition's limit. The applied limit is part of the command digest, so after a limit changes, earlier results are not fresh ([run-record.md](run-record.en.md) §4.7).

### 2.6 Execution environment (`## 실행 환경`)

An optional section. To confirm the tools that checks use before the start, put a `## 실행 환경` section in PLAN and write one tool per line in the form `` - <이름>: `<점검 명령>` `` (a name, a colon and the probe command). Example: `` - 브라우저: `node scripts/probe-browser.mjs` ``.

- The section's extent is the same as for other PLAN sections: from the heading line up to the next level-1 or level-2 heading outside a fenced code block. Blank lines are skipped, and every other line must be a list item in this form. A fenced code block in the section is not an item either, so it is a malformed line: `ha doctor` reports the opening line of each block (also separately for a block that immediately follows another).
- The name is the text before the first `:` with leading and trailing spaces removed. It cannot be empty or contain a backtick, and the same name is used only once.
- The probe command follows the same rules as the verification command of the condition table: one code span, no shell syntax, and quotes around words with spaces.
- Seal 0.1.20 or later lists the tools checks need (browser, database, container, external service and so on) in this section, and runs `ha doctor` before recording any baseline and before the first check after a resume. It gathers unusable tools into one question, never widens permissions or settings on its own, and keeps doing independent work while it waits.
- Before recording any baseline, Seal 0.1.23 or later gathers the decisions that can be known in advance from the goal documents, the plan, the project instructions and the code and that need the user's words, together with unusable tools and a budget that needs approval, and asks them in one message. When a condition can be met only by user confirmation, the same message names that condition and what the user will be asked to confirm at the end. When there is nothing to ask, it asks nothing.
- Only `ha doctor` reads this section. `ha doctor` runs each probe with a fixed 60-second limit, shows `사용 가능` (available) or `사용 불가` (unavailable) with the reason for each tool, and leaves nothing in the record, the run count or the execution bundle ([run-record.md](run-record.en.md) §1.4).
- A malformed line (`environment_item_invalid`), shell syntax (`shell_syntax`), an empty command (`command_empty`), a command that cannot be split into words (`command_parse`), a duplicate name (`environment_name_duplicate`) and a second `## 실행 환경` section (`environment_section_duplicate`) are reported only by `ha doctor`, with file and line, and then no probe runs. They are not lint errors: with or without the section, and whatever its content, the results of `ha lint`, `ha start`, `ha check` and `ha status` are the same.

## 3. `tasks/T00n.md`

| Section | Content |
| --- | --- |
| `목표` (goal) | The behavior this task completes |
| `근거` (basis) | The condition IDs and the SPEC sections those conditions come from |
| `선행 조건` (prerequisites) | The tasks needed and their outputs |
| `읽을 자료` (reading material) | Written in the four groups below. The purpose is to let an agent that does not know the earlier conversation find the evidence it needs without rereading all the repository's documents. |
| `범위` (scope) | What to change and the behavior that must be kept |
| `완료 조건` (completion criteria) | Which conditions (`AC-n`) must pass. When the task step has its own commands to check, write each in a list item of the `## 완료 조건` section that holds only one code span (for example `` - `go test ./pkg/x` ``). `ha check <goal> T001` runs these commands in order and records them. These records are not used to satisfy conditions. |

**Groups in `읽을 자료`**

| Group | What to leave |
| --- | --- |
| `필수 맥락` (essential context) | The relevant sections of documents and the code locations needed to understand the goal |
| `필요할 때 참고` (reference when needed) | Detailed design, long background material |
| `판단 근거` (grounds for decisions) | Which existing behavior or document a decision rests on |
| `충돌·미확인` (conflicts and unconfirmed) | Where documents and code differ, facts not yet confirmed |

Use summaries only as a guide to the original. Write the document path and section with them, and never replace the original completion criteria with a summary. When a small goal merges its tasks into PLAN, put the same groups in the `## 읽을 자료` section of `PLAN.md`.

## 4. `REVIEW.md`

The result of the pre-execution review. Passing the review means "ready to execute", not that the implementation is right.

Its subject differs from Spec's consistency check ([goal-docs.md](goal-docs.en.md) §3).
- Spec's consistency check: is the goal itself clear?
- Pre-execution review (this document): are the plan and the verification method for implementing that goal sufficient?

When the review finds a contradiction or ambiguity in the goal documents, do not edit the goal documents. Ask the user.

| Section | What to check |
| --- | --- |
| `요구사항 대응` (requirement mapping) | The `ha lint` result (missing, orphan, cycle). Whether the SPEC's scope and non-goals are reflected in the tasks. Whether the condition table covers the goal documents' user scenarios. Where the plan differs from the work breakdown proposal, and why |
| `불필요한 task` (unneeded tasks) | Tasks that contribute to no condition |
| `문서 충돌` (document conflicts) | Conflicts among the SPEC, project documents and the actual code |
| `중요한 미결정` (important open decisions) | Decisions left in the goal documents' `## Open Decisions`, and outcome-changing decisions that came up newly during planning (public behavior, data, permissions, irreversible design). Close each item with the user's answer (quoted) or an "assumption". Assumptions appear again in the report. A reading that keeps a component's declared scope or trust boundary is not a question but such an assumption (§2.1.2). |
| `검증 방법과 환경` (verification method and environment) | Whether each condition's command can run in this environment; the tools, data, network and repository history it needs (history only when the condition is about history, together with the range it reads). For a goal that connects several components, by what method the conditions checked across the boundary are verified |
| `독립 검토` (independent review) | The SPEC's `Assurance:` tier (none when absent), the floor the project instructions require and the one followed, the planned review stages (plan, candidate, fixes, cumulative round) or the reason they were skipped under `light`, the product's invariants and decision rules the plan review will look at, and the supported input forms of the open-ended inputs a new contract or check covers. Once rounds run, point to those round and gate records and do not copy the list of findings |
| `결론` (conclusion) | `ready` or `needs_user` (reason) |

### Assurance tier and review order

Seal 0.1.20 or later reads the SPEC's `Assurance:` line ([goal-docs.md](goal-docs.en.md) §2.5) and applies the independent review stages that match the tier. Spar decides how review runs (packet, reviewers, classification rules, stage caps); this contract decides only the timing and the disposition.

- `standard`: takes a candidate stage on the final candidate whose narrow checks pass, a fixes stage after the review-fix commits, and a cumulative round before `ha done`.
- `strict`: in addition to `standard`, takes a plan stage before recording any baseline. It reviews the plan and the check declarations, and the product's invariants and decision rules that this change introduces or alters (target identification, location resolution, retries, caps and so on).
- `light`: may skip independent review; when it does, it leaves the reason in `REVIEW.md` and the completion report. When it takes review, it follows the completion order below.
- A goal without the line follows the selected assurance policy. The assurance requirements of project instructions (`AGENTS.md` and the like) are a floor. The tier can only raise it, never lower it; when the two differ, follow the one that asks for more review, and write in the completion report the review each required and the one followed.
- When the tier changes during implementation and the user asks to continue, apply the changed tier. A stage that the raised tier requires but that was not taken is taken on the current head before completion, and the records of reviews already taken are not deleted.
- The cumulative round is a round that calls Spar's candidate stage again over the goal's base commit to the last head. A goal that needs independent review runs `ha done` only after the cumulative round ends with no open `defect` or `requirement-question`. A fixes round that saw only the fixes cannot replace it; when there were no fixes after the candidate round, that round is the cumulative round. When the cumulative round hits a cap, ask the user; never go past the cap or skip the round without the user's words.
- When a contract or check about open-ended input (sentences, paths, names, formats and so on) is added or changed, the plan writes the supported input forms before the plan review. Those forms cannot be narrower than the input that a SPEC condition, a contract or a protected path requires; narrowing them is for the user to decide. Ask the user about an open-ended condition whose forms are not written.
- Seal decides the disposition of findings. Fix a `defect` or rebut it with evidence; ask the user about a `requirement-question` (without editing the goal documents); never fix an automatically deferred `hardening` without the user's words. A finding about input outside the supported forms is `hardening`; when it was classified as a `defect`, rebut it by citing where the contract wrote the supported forms.
- A `standard` or `strict` goal started with `run-rules/5`, after the cumulative round, makes a `spar gate` for HEAD from that series' rounds, records it with `ha note <goal> review --gate <file>` and then runs `ha done`. The rounds get all goal documents as `--criteria`. When no gate is recorded, or the recorded gate is stale, Core does not record completion ([run-record.md](run-record.en.md) §4.8). When Spar is missing or cannot run, completion is held; to finish without review, the user decides a goal-document change that lowers the tier to `light`. The acceptance of a separate review below does not apply to these goals.
- When Spar is missing or cannot run and review is not required, proceed and write in the completion report that there was no independent review. In a goal started with `run-rules/4` or earlier where review is required, hold completion and record the cause, the affected conditions, the missing evidence and the resolution condition. Another separate review that records its role, inputs, reviewed version, findings or no findings, limits and checkable references counts as the required review; the completion report states that Spar was not used and the method that was used, and that review too covers the whole last head before `ha done`.
- When the user asks to apply remote review after a completion record, proceed in this order: `reopen` quoting that request, the fix, one cumulative round, re-recording stale required checks, and `ha done`.
- In goals started with `run-rules/4` or earlier, the tier does not change the judgment or the output of `ha status` and `ha done`; in goals started with `run-rules/5`, the gate above becomes a completion condition. The `REVIEW.md` of a goal started with an earlier Seal keeps the format it started with.

### Verification evidence and observation scope

New goals started with Seal 0.1.10 or later use the distinctions below in their existing review and completion report. Existing goals keep the format they started with, and the absence of the new items means unrecorded, not evidence of failure or non-execution. Do not convert existing records automatically or fill in past observations.

- **Grounds for the expected result**: the user's requirement, a public contract or an agreed invariant, and its location. When actual behavior or a reference implementation was taken as the standard, also state the contract and scope of that choice. An actual run or a fake's expected value is not in itself the authority on correctness.
- **Boundary to observe**: the boundary and scope of the actual shell, database, HTTP, model or other system to be checked. Write only the environments that affect the result, and distinguish planned, not performed and actually performed.
- **Test method and substitute environment**: what synthetic inputs, fakes, fault injection and comparison with a reference implementation control or stand in for. Do not stretch an example into the right answer for all inputs.
- **Failure kinds not covered**: gaps in the general property, the representative examples and the environment assumptions, and risky boundaries. Even a claim that no risk remains carries a scope, and the number of checks is not evidence of sufficiency.

Link every condition to these three axes and the gaps. A group of conditions that shares the same content may refer to one explanation; repetition per AC, or copies of the same table in several documents, are not required. When the expected grounds are undecided or conflicting, show that state, and do not write them as a finished answer.

Link what the review planned to the actual results in the report. When a condition, check or environment changes, also distinguish the version and the applicable scope of the evidence, and do not use earlier results as new observations. Distinguish facts confirmed by records from the executor's interpretation. Leave the scope where access or execution was blocked, and the reason, as unobserved.

While an important open decision at `needs_user` remains, do not execute; ask the user. Do not ask about what the implementer can decide, such as class names, file layout or implementation order (the same as Spec's questioning principle).

### Assurance policy and counterexamples

New goals started with Seal 0.1.11 or later, and high-impact verification design changes the user requested, link to each condition or condition group the source of the assurance policy, whether it is required or optional, and the reason for the risk. Explain the impact of a false success, the relevant boundary and the gaps in the current evidence, and also record why high-risk candidates were excluded. Do not impose on every goal the same agent, model or pre-baseline-check order on risk level alone. Do not lower a required assurance the executor has already set; leave only undecided outcomes for the user's decision.

Keep the expected grounds, the observation boundary, and the method and substitute environment separate; an actual comparison links the results and differences for the same property, input range and environment. Obtain evidence at the actual boundary when the property or policy a condition claims needs it, and do not require a call for every condition only because a real system exists. When structured originals linked to their targets exist, do not replace them with incomplete inference from strings. Do not promote the success of a fake, a synthetic setup, a reference implementation or a single model run into a guarantee of the whole real contract.

Independent review is a separate review, and its initial input is the goal, the public contract, the checks and the expected grounds. It does not fix its starting point on the implementer's explanation of the solution, but it may also examine the implementation and integration code it needs. Record the condition, check and code versions, the context provided, the role, checkable evidence, what was found, not found or not performed, and the limits. A self-review with only a different role name attached is not independent review, and no claim is made of model diversity, certified identity or a guarantee of sufficiency.

Link a counterexample candidate to its input, the expected violation and why the check misses it, and distinguish reproduction, rejection, fix and remaining gaps. When a violation of the original condition is confirmed, do not complete. Do not close it by passing the existing checks again and only adding risk wording to the report; record which check and which fix evidence handled the missed general property. Leave a proposal that changes the scope for the user to decide.

A check that accepts evidence does not use inconclusive as success, and a prohibition check does not hide inconclusive as no execution. Preserve unobserved, environment error, skip and actual violation. Inconclusive is not evidence of an intended baseline violation either. Evidence sufficiency and whether the product accepts or rejects an input are separate matters.

When a required review or actual evidence is missing, record the cause, the affected conditions, the missing evidence and the resolution condition, leave that required work unfinished and hold completion. Unrelated work continues within the existing authority and budget, and an optional comparison may leave its unobserved scope and risk to a later evaluation. Do not fill the gap with automatic installation, increased cost, widened permissions or a review record without facts.

When the goal, checks, code, expected grounds, environment or policy change, preserve the reviewed version and scope, and distinguish the new applicable scope. Do not reuse a past review for new inputs, or create an unrecorded review retroactively. Core's determinism, local assurance, result distinctions, adverse history, budget and authority stay; the executor chooses the method, timing, reviewers, model and mutation.

## 5. `PROGRESS.md`

The format below is used for goals started with seal 0.1.20 or later. A goal started with an earlier Seal keeps the PROGRESS format it started with. The Skill version of the start record is on the `skill:` line of `ha status`. In a goal started with seal 0.1.6 to 0.1.19, `## 현재` is a short summary in which the check time (with time zone), the `ha status` and `ha done` judgment of that time and the record head are written by hand; each edit writes the check time and the record head as of then, and the check time is for reference. The other sections (`## 타임라인`, the time rules of the two tables) are as below.

| Section | Content |
| --- | --- |
| `## 현재` | A short, current summary (about 5 to 8 lines). At the top, paste unchanged the values `ha report <goal>` shows as the `## 현재` stamp: the record head (seq and hash exactly as `ha status` shows them, for example `seq 2 (0123456789ab)`; while there is no record yet (before `ha start`), `none`, as in `ha status`), the time stored in that record (with time zone), the state, and the completion record seq when there is a completion record. Do not write a check time by hand. Below it go the work in progress, the open problems, the next work and how to resume. Update it with a new stamp whenever the record head changes. Whoever takes over compares the record head with `ha status` to see whether it is stale. The same seq with a different hash is a different record. The stamp's time is for reference. Do not write past events; point to `## 타임라인`. When a completion record is left, state that the summary is as of that completion record, and point to where to see what happens after the bundle's last commit and does not stay in the bundle (merge, PR replies, checks after the merge and so on). Do not write work to do after completion as if it were in progress. |
| `## Task 상태` | Table: `Task`, `상태` (state: `todo` \| `doing` \| `done` \| `dropped` \| `blocked`), `메모` (note). `done` is a claim. `dropped` needs a plan change record. |
| `## 타임라인` | Optional. A history to which events that changed a judgment or the next action are appended. See 5.1 below. `ha` does not read this section. |
| `## 계획 변경` | Table: `시각` (time), `변경` (change), `이유` (reason), `영향` (impact), `다시 검토한 것` (what was reviewed again). A condition kind change (for example `change` → `maintain`) is recorded here too. `시각` includes the time zone. One row is one event. When the same event is also written in the timeline, the detailed description stays only in that timeline entry, and the row points to it with a one-line summary and that entry's time. |
| `## 막힘` | Table: `시각` (time), `원인 종류` (cause kind: `auth` \| `permission` \| `quota` \| `environment` \| `spec` \| `other`), `내용` (description), `해소 조건` (resolution condition). Terminal failures are not retried but recorded here. `시각` includes the time zone. One row is one event. When the same event is also written in the timeline, the detailed description stays only in that timeline entry, and the row points to it with a one-line summary and that entry's time. |
| `## 개선 메모` | Optional. Problems with, and suggestions for, the Seal Skill, its templates and `ha` that the executor noticed. These are claims. The Skill and the templates are not changed inside this goal. |
| `## 완료 보고` | Section 6 below |

### 5.1 Timeline

`## 타임라인` is an optional section. Events that changed a judgment or the next action are appended in time order, so that someone taking over in another session can learn "why it was changed this way". Its content is the executor's claim.

- **Entry heading**: `### <시각> · seq <기록 head> — <종류> · <한 줄 요약>` (time, record head, kind, one-line summary)
  - The time is ISO 8601 with the time zone (for example `2026-01-01T09:00:00+09:00`, or `Z` for UTC). When you do not know the exact time, do not make one up.
  - The seq is the record head when the entry is written (the heading carries only the seq; the hash goes in `## 현재`). While there is no record yet, write `seq none`. An entry tied to a check run uses that run's seq. The seq is an order confirmed by the hash chain of `runs.jsonl`, so it is easier to confirm than the time, which is a claim.
  - The kind is one of `관측` (observation), `원인` (cause), `결정` (decision), `조치` (action), `검증` (verification), `막힘` (block) and `재개` (resume).
- **Time source and judgment**: distinguish the event time from the writing time, and record the time source. When the original event time is unknown, leave it unknown; the heading may use the actual writing time. Write the remaining conditions, new facts, the grounds for a hypothesis and the uncertainty to reduce next, as far as they apply. Do not record every tool call, and do not dress up the same result from the same code, environment and hypothesis as progress. After a repetition, a changed judgment must show, such as a separated cause, a different hypothesis or a blocked condition.
- **Body marks**: mark a `원인` (cause) as `가설` (hypothesis), `확정` (confirmed) or `기각` (rejected); a `조치` (action) as `예정` (planned) or `반영` (applied); and a `검증` (verification) as `대기` (pending), `통과` (passed) or `실패` (failed) (for example `원인 [확정]: …`). Point to evidence by AC, commit and seq, and do not copy raw check output (section 6).
- **Append only.** Do not edit earlier entries. That something was `대기` (pending) at that point is history too. When a judgment changes, write in a new entry what was rejected and on what grounds. A small event that went from confirming the cause to verification at once may be one entry.
- **What to record**: events that changed a judgment or the next action. These are a new problem, a cause confirmed or changed, a change of approach, an important verification result, and blocks and resumes. A resume is any case of starting work again. This covers a new session taking over, a block being cleared and an answer received to `needs_user`. A `재개` entry notes where `## 현재` differed from the actual state, if it did.
- **What not to record**: routine edits and reads, repeated runs of the same failure, progress reports with no change
- **Length**: an entry is about 3 to 6 lines and `## 현재` about 5 to 8 lines. This is editing guidance, not a limit. A complex cause analysis may run longer.

Below is a hypothetical example to explain the format. It does not represent an actual run record.

```markdown
### 2026-01-01T09:00:00+09:00 · seq 2 — 원인 · AC-1 빈 입력 처리
- 관측: 빈 입력을 읽는 검사에서 오류가 났다(seq 2).
- 원인 [확정]: 첫 항목이 항상 있다고 가정했다. 근거: `src/count.go`의 입력 처리
- 조치 [예정]: 빈 입력일 때 0을 반환한다.
```

### 5.2 Post-completion findings

New reports from Seal 0.1.12 on may write details in `## 완료 뒤 발견` or refer to details kept in one place. The format, completion report and run record of existing goals are not converted automatically, and a missing new section is read as unrecorded. Even when appending a new finding to an older goal, do not create past observations or overwrite the original completion report.

Link the finding identifier; the target code, goal and reference to the completion at the time; the related condition, or no condition included, or link undecided; and the description, source, reproduction evidence, observed scope, current confirmation claim and follow-up work. Leave unknown values as undecided or none, and do not fill in arbitrary versions, conditions or check results. Distinguish the event time from the recording time and record the time source. An unknown event time is unknown, and the writing time does not stand in for it.

Distinguish candidate, confirmed, duplicate, rejected and resolved, and tell a new requirement apart from a violation of an existing condition. When a judgment changes, preserve the original and append the correction target, the new judgment, the reason and the evidence. A duplicate points to the canonical finding, and a resolution records the fix and recheck evidence and the unconfirmed scope. Do not describe the number of reports as the number of unique confirmed defects. A document's state names do not guarantee a mechanically current state or the resolution of conflicts from concurrent edits.

The current summary distinguishes the completion at the time, the current findings, and rework requests and whether they are in progress. When a confirmed violation of an existing condition exists, do not put the completion record forward and describe things as if there were no current defect. Keep the details in one place and refer to them from the summary, and do not apply later fixes or checks retroactively as evidence for the past completion.

Recording, confirming, rejecting or resolving in documents does not change the existing completion, execution budget or user authority, and does not start rework. When there is an explicit rework request, link its original source, the target finding and the follow-up result, and distinguish requested, planned, in progress, complete and not requested. Raw logs and private conversations keep their existing storage boundaries and are referred to by permitted locations and identifiers. Public examples state that they are synthetic. This document format is not an implementation of Core's structured queries or state transitions, and does not replace a separate feature contract.

Review or PR findings received without the user's request are not authority to change the completed goal's code. Record them as post-completion findings in this format and report them. Changing the code needs the user's request and `reopen`.

### 5.3 PROGRESS with decision history

A goal started with Seal 0.1.13 or later may record detailed judgments once in the optional [decision history](decision-history.en.md) and refer to them from PROGRESS by event ID (`D<n>`). A goal started with an earlier Seal keeps the PROGRESS format and documentation duties it started with. An environment that supports the feature may add an auxiliary history, but does not retroactively omit originally required explanations or convert past timelines. `ha` does not read the prose sections of PROGRESS, and an event ID reference does not change a task state or the completion judgment.

| Section | With decision history |
| --- | --- |
| `## 현재` | Leaves the current state, important decisions with short reasons, the open problems and how to resume so that they are understandable without the history. Related event IDs are auxiliary references. |
| `## 타임라인` | A short index of the important events a handover needs. It adds the event ID to the heading form of 5.1 above (time, seq, kind, one-line summary) to point to the detail, and does not copy every event. |
| `## 계획 변경`, `## 막힘` | Keep the existing columns and the needed summary, and refer to the detail by time and event ID. |

- Write the detailed reasons, evidence and relations once in the history, and read them with the Markdown query of `ha history`. Do not save the query's Markdown as a separately maintained source, and do not generate the whole of PROGRESS automatically. No number of sentences or fixed writing order is set.
- PROGRESS refers by ID only to events whose storage succeeded. For a judgment where the history was not used, the feature cannot be used, storage failed, or whether it was applied could not be confirmed, leave the explanation in Markdown in the form of 5.1. Required handover content must never remain only as an event ID.
- The decision history is local data in the Git common directory, so it does not follow to a new clone or another machine. There, the detailed reasons, evidence and relations are absent, and only the current state, the important decisions with short reasons and how to resume, kept in PROGRESS, partly make up for this loss. This is not a restoration of the detailed history.
- The executor decides which events to record and when, which relations to link, and how to group small events. The criteria for which events to record are the same as in 5.1. This feature has no human approval step or retrospective duty of its own.

## 6. Completion report

Written when `ha done` has left a completion record, or when work stops because it cannot go further (`needs_user`, `blocked`, `budget_exhausted`).

The completion report of a goal started with seal 0.1.20 or later has these two parts.

1. The `ha report <goal>` output of that moment. Run it with `--lang ko` when the user's language is Korean and with `--lang en` otherwise, and paste it unchanged. It holds the five-line summary, the `## 현재` stamp, the evidence per condition, the completion coverage (the completion record's seq and commit, and whether it covers HEAD) and where the original output of unmet conditions is, and no raw verification output. Do not paste the raw `ha status` or `ha done` output.
2. Below it, add only the executor's judgment sentences: the changed files and the condition each contributed to (a claim); assumed open decisions and `manual` conditions; the grounds for the expected result, the actual observation boundary, the test method and substitute environment, and the unobserved scope; the assurance tier and the review stages actually taken, or the reason they were skipped; where the independent review results are; when the SPEC tier and the project instructions differed, the review each required and the one followed; whether the head to be sent or merged is the completion record's commit and, when it differs, the fact that the completion does not cover the difference (it does cover it when `ha status` at that head keeps showing the same completion record); the assurance level `local` and the fact that the executor wrote the checks; the fact that the report is not evidence of deployment, installation or a real model run; and, when not complete, how to continue.

A goal started with an earlier Seal keeps the report format it started with, in the order below.

1. On completion, the output of `ha done`; when stopping, the output of `ha status --format md`. Paste it unchanged and do not rewrite it. This output holds only the result, reasons, digests and record seqs, and no raw verification output.
2. Completion coverage (goals started with Seal 0.1.15 or later): the completion record's seq and commit, whether the head to be sent or merged is the same as that commit, and, when it differs, the fact that the completion does not cover the difference. When `ha status` at that head keeps showing the same completion record, however, the completion covers it. A goal started with an earlier Seal keeps the report format it started with.
3. Evidence per condition: condition, result, record seq. Omit it when the status output has it.
4. The changed files and the condition each contributed to. **This is the agent's explanation; mark it as a claim.**
5. Remaining limits: assumed open decisions, `manual` conditions, the assurance level (`local`), the fact that the executor wrote the checks
6. In the new report format, link the review's expected grounds, planned boundary and method to what was actually performed, the results, the evidence and what was unobserved. Present what was confirmed, what could not be confirmed and the risky boundaries by condition or shared group. Also state the checks, environments and versions that differ from the plan, and the valid scope of the evidence. The report does not stand in for evidence of deployment, installation or a real model run.
7. How to continue: when not complete, what remains and under what conditions the work starts again

When the user asks, after a completion record, to apply remote review or PR findings, that is a rework request. Record `reopen` quoting that request; after the fix, a goal that needs independent review takes one cumulative round; re-record the stale required checks on the final head; and before sending or merging, record completion again with `ha done`. Core's freshness decides which checks to re-record (`run-record.md` §3).

**Never paste raw verification output anywhere in the execution bundle.** The execution bundle is committed, and the user may share it with others. When a failure's cause needs explaining, summarize it, and let the raw output be viewed locally with `ha log <goal> <seq>`.

## 7. Example (a small goal, with its task merged into PLAN)

Below is an example of ordinary runner commands for `/1` and `/2`. For a new `/3` change, this pytest exit code alone is not enough. Link a declaration and a producer, as in [the result contract's actual reference integration](check-result.en.md#4-actual-reference-integration). A maintain condition that preserves all existing checks can keep using an ordinary runner.

```markdown
# PLAN — empty-input

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python -m pytest tests/test_empty.py -q` | tests/test_empty.py | T001 |
| AC-2 | maintain | `python -m pytest -q` | — | T001 |

## 범위
- 바꿀 수 있는 경로: `src/textstats/**`, `tests/test_empty.py` (SPEC: Proposed Behavior)
- 테스트 경로: (기본값)
- 유지할 동작: 비어 있지 않은 파일의 출력 형식 (SPEC: Non-goals)

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | 빈 입력에서 0 0 0을 출력한다 | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 20
- 경과 시간: 2시간
- 비용: 미관측
```

## 8. `ha lint` error codes

`ha lint` prints one line per error with the code, the file and line, and a description. With no errors, it prints the single line `ok: no lint problems`, and when the goal documents have a valid `Assurance:` line ([goal-docs.md](goal-docs.en.md) §2.5), it adds `assurance tier: <등급>` (tier) on the next line. With even one error, `ha start` and `ha check` are refused, and `ha status` gives `lint_error`.

Goal documents ([goal-docs.md](goal-docs.en.md))

| Code | Meaning |
| --- | --- |
| `spec_missing` | The goal directory has no `SPEC.md` |
| `criteria_missing` | There is no `## Acceptance Criteria` section, or it has no conditions |
| `criterion_without_id` | A condition without an `AC-n` ID |
| `criterion_duplicate_id` | The same `AC-n` appears twice |
| `criteria_format` | The goal documents' list format value, or the position of its selection marker, is invalid |
| `criteria_format_duplicate` | The list format selection marker appears more than once in the goal documents |
| `assurance_format` | The value of the `Assurance:` line is not `light`, `standard` or `strict`, or the line is indented, or it comes after the first level-2 section |
| `assurance_format_duplicate` | The `Assurance:` line appears more than once |
| `criterion_indentation` | Indentation in the new list format is ambiguous or unsupported |
| `goal_doc_link_missing` | A document inside the goal directory that SPEC.md or an auxiliary document links does not exist |
| `proposal_invalid_id` | A work breakdown proposal's ID is not in the `W1` form |
| `proposal_duplicate_id` | The same `W` ID appears twice |
| `proposal_unknown_criterion` | A proposal points to an `AC-n` that does not exist |
| `proposal_unknown_dependency` | A proposal depends on a `W` that does not exist |
| `proposal_cycle` | The proposals' dependencies have a cycle |
| `table_columns` | A contract table's column names differ (common to goal documents and execution bundles) |

Execution bundle

| Code | Meaning |
| --- | --- |
| `plan_missing` | There is no `PLAN.md` |
| `bundle_file_missing` | `REVIEW.md` or `PROGRESS.md` is missing |
| `plan_section_missing` | A section or table among `## 조건표`, `## 범위`, `## Task` and `## 예산` is missing |
| `criterion_invalid_id` | An ID in the condition table is not in the `AC-n` or `EX-n` form |
| `criterion_duplicate_row` | The same condition appears twice in the condition table |
| `criterion_unknown` | An `AC-n` in the condition table is not in the goal documents |
| `criterion_not_in_plan` | An `AC-n` in the goal documents is not in the condition table |
| `invalid_kind` | The kind is not `change`, `maintain` or `manual` |
| `command_missing` | A `change` or `maintain` condition has no verification command |
| `command_multiple` | One cell holds more than one verification command |
| `shell_syntax` | A verification command contains shell syntax |
| `command_parse` | A verification command cannot be split into words (an unclosed quote and so on) |
| `check_paths_missing` | A `change` condition has no check paths |
| `check_path_glob` | A check path is a glob |
| `manual_reason_missing` | A `manual` condition has no reason |
| `criterion_without_task` | A condition without a task |
| `scope_missing` | `바꿀 수 있는 경로` is missing |
| `task_invalid_id` | A task ID is not in the `T001` form |
| `task_duplicate_id` | The same task ID appears twice |
| `task_required_invalid` | `필수` is not `예` or `아니오` |
| `task_unknown` | The condition table points to a task that does not exist |
| `task_unknown_criterion` | A task cites, as its basis, a condition that is not in the condition table |
| `task_unknown_dependency` | A task depends on a task that does not exist |
| `task_orphan` | A task that rests on no condition and is not a prerequisite of another task |
| `task_cycle` | The task dependencies have a cycle |
| `task_doc_missing` | The task document that the `문서` column points to does not exist |
| `proposal_unknown` | A task's `제안` is a `W` that is not in the goal documents |
| `budget_missing` | `검증 실행` or `경과 시간` is missing or cannot be read |
| `time_limit_invalid` | A `## 시간 제한` item of a `run-rules/4` or `run-rules/5` goal is malformed, exceeds 7200 seconds, has a target that does not exist, or declares the same target twice |
| `evidence_invalid` | A condition, declaration or producer input in the result contract table is invalid |
| `evidence_overlay_missing` | A declaration or producer input is missing from that condition's check paths |
| `other_bundles_invalid` | The `다른 목표 실행 묶음` item in `## 범위` does not start with the exact key `다른 목표 실행 묶음:`, or its value is not the single code span `검사 입력` |
| `progress_state_invalid` | A task state in `PROGRESS.md` is not an allowed value |
