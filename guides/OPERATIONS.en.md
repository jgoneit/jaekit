# Reading work status and the completion report

[한국어](OPERATIONS.md) · **English**

Read this when you want to know why work stopped, or what exactly a completion report rests on. It covers pre-start checks, what a failure before the change means, the evidence behind the report, the state that needs your confirmation (`needs_user`), blocks and budgets, and the assurance policy. For first-time users, invocation, resuming and troubleshooting are in the [usage guide](USAGE.en.md). For installation and checking versions, see the [install guide](INSTALL.en.md).

This document describes the behavior of v0.1.9 (ha 0.1.9, spar 0.1.9, spec 0.1.16, seal 0.1.36, spar plugin 0.1.9). It covers preflight diagnostics and capability checks, `nested/1`, `/3` results, and total run-limit changes. Existing goals keep their saved rules and formats. The difference between release distributions and source builds is in [Release and development combinations](INSTALL.en.md#release-and-development-combinations).

## Handing over a goal

Spec organizes your request in `docs/specs/<goal>/SPEC.md`. It writes the goal, scope and completion criteria, reports what it wrote, and then stops. Refine the goal by answering outcome-changing questions or saying what to change. `Status: Ready` marks the document as ready. It does not start implementation.

When it is ready, in a Codex conversation, select `seal:seal` from the `$` list, then send `$seal:seal docs/specs/<goal>` as a separate message. In Claude Code, use `/seal:seal docs/specs/<goal>`. If a goal document already exists, you can hand it to Seal directly. The agent reviews the plan and check methods, then carries on with implementation, checks and fixes. You do not need to approve every operation.

You decide whether to change the goal and scope. Seal never modifies goal documents. If you revised the goal with Spec, make a separate request such as `docs/specs/<goal> 이어서 해줘` (continue `docs/specs/<goal>`). The document formats are detailed in the [goal documents contract](../contracts/goal-docs.md) and the [execution bundle contract](../contracts/bundle.md).

## Checking tools beforehand

If your checks use a browser or another external tool, confirm the tool is there before the checks actually run. List the tools in the PLAN section `## 실행 환경` in the form `` - <이름>: `<점검 명령>` `` and run `ha doctor <goal>`.

Each tool gets `사용 가능` (available) or `사용 불가` (unavailable). The reason for an unavailable tool is one of `실행 파일 없음` (no executable), `실행 권한 없음` (no permission to run), `시간 초과` (timed out), `0이 아닌 종료` (nonzero exit) and `신호로 종료` (ended by a signal). Each check gets at most 60 seconds. Checks count toward neither the record nor the budget. `ha doctor` installs nothing. Install an unavailable tool or fix its path before the checks.

## What to check before running

Before recording anything, Seal looks at three things:

- whether the installed `ha` has the support it needs
- whether the budget is enough
- whether the working tree has uncommitted changes

If any of them is off, Seal stops and says why.

### Core support

Seal does not compare Core support by hand. It declares the support it needs in `core-requirements.json` in its skill folder, and starts with `ha start --check-skill`. When support is missing, `ha` refuses without a record. Seal then reports the executable and the message, and stops. It does not fall back to older rules. What to do is install an `ha` that has that support.

Seal 0.1.36 in v0.1.9 requires `run-rules/8` for new goals. So it needs ha 0.1.9, which supports `/8`. For a goal already started, each command computes its saved rules or refuses them with exit 66.

When the skill folder that Seal names with `--skill` has a `core-requirements.json`, `start` compares the Core support that skill needs with the actual executable. When any support is missing, it refuses without a record. With `--check-skill`, the declaration is required ([Core capabilities contract](../contracts/core-capabilities.md) §8).

### Budget and tools

- Without a budget, it starts with `검증 실행: 자동`. It asks once, only when approval is needed.
- It checks the tools the checks need with `ha doctor` before any baseline. It asks about unavailable tools in one question.

The agent can compare the minimum verification run count in PLAN with the limit using `ha estimate docs/specs/<goal>`. A required `change` condition that requires a baseline runs once before and once after the change, so it counts as 2 runs. The formula is in [Baseline selection](RULES.en.md#baseline-selection). `manual` and `review` conditions are not included in the check execution count.

The estimate also shows these separately:

- Optional conditions included in default comparative verification.
- Additional run counts for explicitly selected `AC`, `EX`, or task commands.

`--baseline` shows the runs for the baseline (comparison) selection. `--format json` gives machine-readable output.

If PLAN gives `- 검증 실행: 자동` (automatic), `start` sets the limit to `max(50, ⌈required minimum × 1.5⌉)`. This value is compared with `preapproved_runs` in `ha.json` in the user configuration folder. That folder is `~/.config/jaekit/`, or `$XDG_CONFIG_HOME/jaekit/` when `XDG_CONFIG_HOME` is set.

- When the value is at or below `preapproved_runs`, the goal starts without a question. Without the file, that limit is 50.
- Above it, `start` stops with `exceeds the pre-approved limit` and records nothing. It then waits for the user's approval.
- A file that cannot be used stops with a `pre-approved run limit` error. It does not fall back to 50.

The format and examples are in [Automatic budget](RULES.en.md#automatic-budget).

If the limit is less than the required minimum, `start` refuses before recording. In this case, split the goal within the existing allowed scope, or decide on an adequate budget before starting. The estimate does not include retries or preparation time. Querying an in-progress goal only explains the current PLAN. It does not increase an already recorded limit or remaining budget.

### Uncommitted changes

If verification is refused before running due to `uncommitted changes`, check the printed paths:

- `tracked`: changes to files Git already tracks. Commit or revert them according to your work intent.
- `untracked dependency symlink`: look at the path of the link itself to decide whether to keep or ignore it. For example, if `apps/web/node_modules` is displayed, review the ignore rules for that path.

Guidance alone will not ignore all `node_modules` or clean up files automatically.

Refused checks do not count toward the execution count. If execution stopped while running multiple checks, earlier results and usage remain. The output shows how many were executed. The paths and types also appear under `worktree_dirty` in `ha status`. A Git query failure is never treated as a clean state. Resolve the problem, then rerun verification.

### Criteria format in goal documents

New Spec examples specify `Criteria-Format: nested/1`. With this marker, description lists under an `AC` are read as the body of that same condition. Existing goals without this marker keep their previous interpretation.

Adding this marker to an existing goal changes the goal document. Therefore, previous verifications and manual confirmations cannot be reused for the new interpretation. For indentation examples, see the [goal documents contract](../contracts/goal-docs.md).

## What a failure before the change means

The check for a change condition that requires a baseline must show the **intended violation** before the change. Which conditions require a baseline is in [Baseline selection](#baseline-selection). The mere fact that a command failed is not evidence from before the change.

A `change` check must link declared targets and requirement violations through structured results. The following are not accepted as expected failures:

- Missing dependencies
- Errors in import, test collection, or browser startup
- Unexecuted or skipped checks (`skip`)
- Script failures with uninterpretable meaning

A result where an actually executed check directly observes the absence of functionality can be distinguished from these.

When recording a baseline, all declared targets must run. They must show the intended violation, without errors or other assertion failures. Whether or not a baseline is required, current checks require all declared targets to pass.

How to link them is in the [check result contract and reference check](../contracts/check-result.md). The reference check is a small example that actually reads files. It does not automatically install specific test runners, nor does it support all runners.

### Linking a test you already have

You can also use a test you already have. On a Core whose `ha capabilities` lists `check-adapter/v1`, a [runner adapter](../contracts/check-result.md#5-runner-adapters) links a `change` condition to that one test. No producer program is needed. `ha` reads the output of `go test -json …`, or the JUnit XML a runner writes at the `{HA_JUNIT_XML}` location, directly.

Put the marker `jaekit:violation=<violation id>` on the first line of the assertion's failure message for when the property breaks. For a hypothetical greeting CLI test, write `t.Fatalf("jaekit:violation=greeting-missing the CLI says %q", got)`.

A failure without the marker is not accepted as the intended violation. The same holds when the marker is only in a log or standard output. Adapters are optional. Without support, use the reference check or a dedicated producer.

### Errors and retries

`error` and inconclusive verdicts both count toward the limited error retries. Actually attempted failures, errors, stale results and their usage remain. A later pass alone does not erase an unfavorable result produced on the same input.

Look at the reason in the status and the corresponding `ha log`. Then decide what must change: the check, the product, or the environment.

> A goal started with an earlier release is judged by the rules it started with. What differs between rule versions is in the [rules version table of the run record contract](../contracts/run-record.md#46-rules-versions).

## Baseline selection

Only change conditions with `요구` in the condition table's optional `baseline` column require a baseline. Seal marks only a bug fix or regression whose defect is already present at the base commit, and a prohibition such as "X does not happen". The marking rules, the minimum run count, when the baseline is recorded and how input failures are handled are in [Details and rules](RULES.en.md#baseline-selection).

## Reading the completion report

The completion report (`## 완료 보고`) appears in `PROGRESS.md` in the goal folder and in the conversation. Check the following:

1. **What changed.** Check whether actual files, screens and behaviors match the request.
2. **How required conditions were satisfied.** Tell apart automated check results, items confirmed manually and `review` conditions met by the review gate. `review` conditions appear under `To review` with the seq of the gate record. Optional conditions do not block completion and are marked separately.
3. **What the checks covered.** Look at checks that verified new behavior, behavior that must be preserved, and unverified scope. If verification commands or condition kinds were changed, check the reason.
4. **What assumptions and limitations exist.** See whether the assumptions the agent chose match your intent. Also see whether remaining limitations affect the result.

The evidence of completion is the completion record left by `ha done`. In Terminal, run `ha status <goal>` to see the status based on the current goal and records. Replace `<goal>` with the actual goal folder path, such as `docs/specs/login-link`.

The completion report pastes the `ha report` output unchanged. Read the five lines at the top first, then the completion record below them. `ha report <goal>` summarizes the same records in five lines: `Changed`, `Checked`, `To review`, `Remaining limits` and `Next step`. It is the same computation as `ha status` and does not change the record. The stamp below the summary shows which record head `## 현재` in `PROGRESS.md` should be checked against.

For a finished goal, read `## 검수 요약` first. Seal's judgment is only in that section. The layout of that section is in the [usage guide](USAGE.en.md#reading-the-completion-report).

The assurance level `local` means evidence obtained on that computer. Checks are written by the executing agent. That agent can modify files and records with the same user permissions. `local` does not mean that the whole result is defect-free, or that an independent third party guarantees it.

If something is missing, say what to fix. Seal then records `reopen` citing your request and reworks the goal. Previous completion records are not used as current completion evidence, and a new budget window opens. If the goal document itself must change, update it with Spec and then separately ask to continue.

If you need the raw check output, view it in Terminal with `ha log <goal> <seq>`. `<seq>` is the record number shown in the report. For `process_output` records where saving output failed after execution, the raw output path may be empty. The actual attempt and usage still remain. If a record-saving error leaves it uncertain whether a change applied, do not assume success; verify the save state. Preserved raw outputs live under `ha/` in the Git directory of the worktree, and Git does not track them (see [Saved files](USAGE.en.md#saved-files)).

## Handling needs_user

When items remain that the agent cannot decide on its own, it asks and gives the reason. Make the required judgment and reply, and your reply is recorded.

| Reason | What to check | How to proceed |
| --- | --- | --- |
| `manual_unconfirmed` | Did you manually confirm conditions that automated checks cannot verify? | Check the result and let the agent know. If the condition text changes, it must be re-confirmed. |
| `review_tier_missing` | Does a goal whose review tier is `light` or missing have a `review` condition? A `review` condition is about document content that no product behavior reads. Only the independent review gate of a `standard` or `strict` goal meets it. | With Spec, raise the goal document's `Assurance:` tier to `standard` or `strict`, or change that criterion. |
| `out_of_scope` | Is a change outside the planned scope required? | Allow or revert the change. If the goal itself has changed, update it with Spec. |
| `test_definition_changed` | Is changing existing tests justified? | Review the reason and contents of the change, then allow or revert it. |
| `spec_changed` | Was the change to the goal document intentional? | Confirm if the change was intentional. A request to continue or implement sent after editing is also recorded as confirmation. |
| `error_limit` | What caused repeated check errors on the same code? | Fix the environment, then have the agent rerun. Review the goal first if switching to a condition that requires manual confirmation. |
| `record_integrity` | Is the run record corrupted? | Check the cause. Do not arbitrarily edit records to pass; if needed, start in a new goal folder. |
| `review_open_question` | What does the open `requirement-question` in the recorded review gate ask? | Answer it. The agent records your answer, has the review run again and records a new gate ([Review gate](RULES.en.md#review-gate)). |

`review_pending` is not in this table, because the agent resolves it, not you. It means that for a `review` condition the review gate does not yet cover the current HEAD and goal documents, or another review reason remains. The agent runs the remaining review rounds and records the gate.

## Blocks and budget

- **`blocked`**: Stopped due to expired authentication, denied permissions, usage limits or similar issues. Read the clearing conditions the agent recorded, resolve the cause, and tell the agent to continue.
- **`budget_exhausted`**: Exceeded the budget for check run count. Look at the remaining work, then decide whether to allocate more. `경과 시간` (elapsed time) is only informational; going over it does not lead to this state.

To use more runs only, change the total run limit of the current window. Reported waits and the limits for resumed windows are in [Autonomy](RULES.en.md#autonomy) and the [run record contract](../contracts/run-record.md#47-run-rules4).

- For example, if usage is 48 runs against a limit of 50 runs, changing the limit to 100 gives 48 used and 52 remaining.
- If reduced to 40 runs, there are 0 remaining and 8 in excess. Recorded usage does not disappear.
- An increase keeps the user's raw words allowing the limit and the request context. A decrease keeps the reason.
- Even a short "okay" ("좋아") is not rejected on form alone if context shows what was agreed to.
- If the quote source cannot be confirmed, it remains an agent-recorded claim.

Changes are linked to the status window number, budget revision and current limit. So if a change conflicts with another change or a resume, it is never overwritten or summed automatically. Re-check the current status to decide. If a storage error signals that application is uncertain, verify the record before reapplying. See the product contract for [exact commands and fields](../contracts/run-record.md#52-budget).

This change does not open a new window. It does not reset elapsed time, failure or error limits, or blocked state. Changing the time limit is unsupported. Editing PLAN alone does not change in-progress limits. A completed window requires explicit rework or resumption. The run limit of a resumed window is the start limit, or the limit last set with a quote of the user's words.

The budget window runs from the last start or `reopen` to the timestamp of the last record subject to the run budget. Finding lines are excluded from budget input. A status check alone does not increase elapsed time. But a new record, including a budget change, extends the window. Elapsed time is the window span minus reported waits and intervals after completion. Actual attempts of checks and baselines, errors and inconclusive verdicts, and valid versus stale results are separate aggregation axes. Baselines are also included in usage.

**The interval after completion.** It runs from a completion `done` to the next start of a check or baseline, or to a non-complete `done`. It is excluded from elapsed time, and the excluded seconds appear as `budget.post_completion_seconds`. The exact boundaries are in [run record contract §4.9](../contracts/run-record.md#49-run-rules6). Code rework after completion proceeds in a new window opened with `reopen` quoting the user's request. Run counts, limits and refusal to change a completed window's budget stay unchanged.

Reaching the limit means usage equals the limit; exceeding it means usage actually went past the limit. A goal with a completion record does not enter `budget_exhausted` as time passes. If all required conditions are satisfied and only budget exhaustion remains, completion can be recorded. This is not authority for additional execution. Nor is the budget a mechanism that automatically blocks check execution or terminates running checks.

## Continuing in a new conversation

Open a new conversation in the same project folder and send `docs/specs/<goal> 이어서 해줘` (or `continue docs/specs/<goal>`). Seal reads the goal, plan, progress records and current status, then continues the remaining work. If any items remain that need your judgment, answer them first.

## Reporting verification evidence

The reporting format distinguishes grounds for expected results, actual observation boundaries, test methods, and alternative environments. Condition groups can reference the same grounds. Reviews record planned items. Completion and discontinuation reports record actual results, unobserved scope and remaining risks. Actual execution is not in itself grounds for correctness. Fake results do not turn into real model observations either. Time sources and new facts that altered judgments are also distinguished.

Existing goals keep their starting formats. Omitted new items remain unrecorded. Past completions are never overwritten, nor are past observations backfilled. See the [synthetic reporting example](../examples/verification-reporting.md) (Korean).

## Assurance policy

If required independent reviews or actual boundary evidence are missing, completion is withheld. Unrelated work can continue.

The SPEC's `Assurance:` tier sets the independent review stages:

- `standard` takes a candidate round on the final candidate, and a candidate round from the base commit after review fixes. The last candidate round is the cumulative round.
- `strict` adds one plan round before any baseline.
- `light` may skip review with a reason.
- When project instructions ask for more review, Seal follows them.

A goal that needs independent review runs `ha done` only once a cumulative round over the whole last head ends with no open finding ([usage guide](USAGE.en.md#seals-pre-start-checks-and-review-order)).

<details>
<summary>The scope of the assurance policy and a synthetic case</summary>

The assurance policy links the required scope of an assurance policy specified by a goal or project to conditions. Optional comparisons record unobserved scope and limitations. The executor decides the model and execution order. Core does not treat reviewers' agreement as sufficient evidence.

The [synthetic assurance case](../examples/verification-design.md) (Korean) is an example of linking the contract. It is not an actual model observation.

</details>

## Post-completion finding documents

This reporting format preserves the original completion report. It links subsequent findings to target versions, conditions, reproduction scope, judgment history and rework requests. Unresolved findings and historical completions can be displayed at the same time. Document status does not substitute for Core structured queries or concurrent change handling. Finding records alone do not resume execution or change budgets. The [synthetic finding history](../examples/verification-findings.md) (Korean) is not an actual run record.
