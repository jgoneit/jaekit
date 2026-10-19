# Reading work status and the completion report

[한국어](OPERATIONS.md) · **English**

This document describes preflight diagnostics, capability checks, `nested/1`, `/3` results, and total run-limit changes in v0.1.6 (ha 0.1.6, spec 0.1.12, seal 0.1.17). Existing goals keep their saved rules and formats. The difference between release distributions and source builds is in [Release and development combinations](INSTALL.en.md#release-and-development-combinations).

For first-time users, invocation, resuming, and troubleshooting are in the [usage guide](USAGE.en.md). For installation and checking versions, see the [install guide](INSTALL.en.md). This document explains in more detail how to read questions asked during work and the completion report.

## Handing over a goal

Spec organizes your request into goals, scope, and completion criteria in `docs/specs/<goal>/SPEC.md`, reports what it wrote, and then stops. Refine the goal by answering outcome-changing questions or stating what to change. `Status: Ready` indicates that the document is ready; it does not start implementation.

When ready, select `seal:seal` from the `$` list in a Codex conversation and send `$seal:seal docs/specs/<goal>` as a separate message. In Claude Code, use `/seal:seal docs/specs/<goal>`. If a goal document already exists, you can hand it over to Seal directly. The agent reviews the plan and check methods, then proceeds with implementation, checks, and fixes. You do not need to approve every operation.

The user decides whether to change the goal and scope. Seal never modifies goal documents. If you revised the goal with Spec, make a separate request such as `docs/specs/<goal> 이어서 해줘` (continue `docs/specs/<goal>`). For detailed document formats, see the [goal documents contract](../contracts/goal-docs.md) and [execution bundle contract](../contracts/bundle.md) (Korean).

## What to check before running

Seal resolves the absolute path of the Core executable it will actually use, and reads `capabilities --format json` from that executable. A fresh start requires both that the default rule is understood and that all required capabilities are present. Existing goals are checked against their saved rules and the requested action.

Seal stops before recording and explains the target and reason if any of the following occurs:
- Querying capabilities is unsupported or fails.
- The query returns invalid information.
- Required capabilities are missing.

Seal never reuses query results from another executable or starts by switching to `/1` or `/2`. If the path or the executable content changes, Seal checks capabilities again. For the exact fields, see the [Core capabilities contract](../contracts/core-capabilities.md) (Korean).

This capability check is never bypassed through automatic installation, privilege escalation, or altering historical records. Even after an update, Seal re-reads the capability information from the actually selected executable.

The Python compatibility helper is optional. Even without Python, you can directly verify the contract against the actual capability information. Spec does not need Core or this query to write documents. Whether an installed Codex or Claude model actually follows instructions is subject to separate host evaluation; capability output alone does not guarantee it.

The agent can compare the minimum verification run count in PLAN with the limit using `ha estimate docs/specs/<goal>`. For example, 34 required `change` conditions need at least 68 runs: one before and one after the change for each. `manual` conditions are not included in the check execution count. The estimate also separately displays:
- Optional conditions included in default comparative verification.
- Additional run counts for explicitly selected `AC`, `EX`, or task commands.

`--baseline` shows the runs for the baseline (comparison) selection, and `--format json` gives machine-readable output.

If the limit is less than the required minimum, `start` refuses before recording. In this case, split the goal within the existing allowed scope, or decide on an adequate budget before starting. The estimate does not include retries or preparation time. Querying an in-progress goal only explains the current PLAN; it does not increase an already recorded limit or remaining budget.

If verification is refused before running due to `uncommitted changes`, check the printed paths:
- `tracked`: Changes to files already tracked by Git. Commit or revert them according to your work intent.
- `untracked dependency symlink`: Look at the path of the link itself to decide whether to keep or ignore it. For example, if `apps/web/node_modules` is displayed, review the ignore rules for that path.

Guidance alone will not ignore all `node_modules` or clean up files automatically.

Refused checks do not count toward the execution count. If execution stopped while running multiple checks, previously executed results and usage remain, and the output indicates how many were executed. The paths and types also appear under `worktree_dirty` in `ha status`. A Git query failure is never treated as a clean state. Resolve the problem, then rerun verification.

New Spec examples specify `Criteria-Format: nested/1`. This reads description lists under an `AC` as the body of that same condition. Existing goals without this marker keep their previous interpretation. Adding this marker to an existing goal changes the goal document. Therefore, previous verifications and manual confirmations cannot be reused for the new interpretation. For indentation examples, refer to the [goal documents contract](../contracts/goal-docs.md).

## What a failure before the change means

Starting with Seal 0.1.16, Seal starts new goals under `run-rules/4`. `/4` preserves the `/3` result rules while adding [autonomy](USAGE.en.md#autonomy-run-rules4), including error recovery.

A `change` check must link declared targets and requirement violations through structured results. The mere fact that a command failed does not constitute pre-modification evidence. The following are not accepted as expected failures:
- Missing dependencies
- Errors in import, test collection, or browser startup
- Unexecuted or skipped checks (`skip`)
- Script failures with uninterpretable meaning

Results where an actually executed check directly observes the absence of functionality can be distinguished.

All declared targets must run. Before the change, there must be the intended violation, without errors or other assertion failures. Current checks require that all of the same targets pass. See the [check result contract and reference check](../contracts/check-result.md) (Korean) for how to link them. The reference check is a small example that actually reads files; it does not automatically install specific test runners or support all runners.

To use a test you already have, a Core whose `ha capabilities` lists `check-adapter/v1` can link a `change` condition to that one test through a [runner adapter](../contracts/check-result.md#5-러너-어댑터) (Korean). `ha` reads the output of `go test -json …`, or the JUnit XML a runner writes at the `{HA_JUNIT_XML}` location, without a producer program. Put the marker `jaekit:violation=<violation id>` on the first line of the assertion's failure message for when the property breaks. For a hypothetical greeting CLI test, write `t.Fatalf("jaekit:violation=greeting-missing the CLI says %q", got)`. A failure without the marker, or with the marker only in a log or standard output, is not accepted as the intended violation. Adapters are optional; without support, use the reference check or a dedicated producer.

`error` and inconclusive verdicts are included in limited error retries. Actually attempted failures, errors, stale results, and usage remain. A later pass alone does not erase an unfavorable result produced on the same input. Look at the reason in the status and the corresponding `ha log` to decide what must change among the check, the product, or the environment. Existing goals started under `/1` or `/2` keep their previous classifications and are not automatically migrated.

## Reading the completion report

The completion report (`## 완료 보고`) appears in `PROGRESS.md` in the goal folder and in the conversation. Check the following:

1. **What changed.** Check whether actual files, screens, and behaviors match the request.
2. **How required conditions were satisfied.** Distinguish between automated check results and items confirmed manually. Optional conditions do not block completion and are marked separately.
3. **What the checks covered.** Review checks that verified new behavior, behavior that must be preserved, and unverified scope. If verification commands or condition kinds were changed, check the reason.
4. **What assumptions and limitations exist.** Review whether agent-chosen assumptions match your intent, and whether remaining limitations affect the result.

The evidence of completion is the completion record left by `ha done`. In Terminal, run `ha status <goal>` to check the status based on the current goal and records. Replace `<goal>` with the actual goal folder path, such as `docs/specs/login-link`.

The assurance level `local` means evidence obtained on that computer. Checks are authored by the executing agent, which can modify files and records with the same user permissions. It does not mean that the whole result is defect-free or guaranteed by an independent third party.

If something is missing, state what to fix. Seal records `reopen` citing the user's request and reworks the goal. Previous completion records are not used as current completion evidence, and a new budget window opens. If the goal document itself must change, update it with Spec and separately ask to continue.

If you need the raw check output, view it in Terminal with `ha log <goal> <seq>`. `<seq>` is the record number shown in the report. For `process_output` records where saving output failed after execution, the raw output path may be empty even though the actual attempt and usage remain. If applying changes is uncertain due to a record-saving error, do not assume success; verify the save state. Preserved raw outputs live under `ha/` in the Git directory of the worktree and are not tracked by Git (see [Saved files](USAGE.en.md#saved-files)).

## Handling needs_user

When items remain that the agent cannot decide on its own, it asks along with the reason. Make the required judgment and reply, and that response is recorded.

| Reason | What to check | How to proceed |
| --- | --- | --- |
| `manual_unconfirmed` | Did you manually confirm conditions that cannot be verified by automated checks? | Check the result and let the agent know. If the condition text changes, it must be re-confirmed. |
| `out_of_scope` | Is a change outside the planned scope required? | Allow or revert the change. If the goal itself has changed, update it with Spec. |
| `test_definition_changed` | Is changing existing tests justified? | Review the reason and contents of the change, then allow or revert it. |
| `spec_changed` | Was the change to the goal document intentional? | Confirm if the change was intentional. A request to continue or implement sent after editing is also recorded as confirmation. |
| `error_limit` | What caused repeated check errors on the same code? | Fix the environment, then have the agent rerun. Review the goal first if switching to a condition that requires manual confirmation. |
| `record_integrity` | Is the run record corrupted? | Check the cause. Do not arbitrarily edit records to pass; if needed, start in a new goal folder. |

## Blocks and budget

- **`blocked`**: Stopped due to expired authentication, denied permissions, usage limits, or similar issues. Read the clearing conditions recorded by the agent, resolve the cause, and tell the agent to continue.
- **`budget_exhausted`**: Exceeded the budget for check run count or elapsed time. Review remaining work and which limit was exceeded, then decide whether to allocate more.

`run-rules/4` follows the `/3` budget rules below while separately setting pre-approved limits, reported waits, and limits for resumed windows. For those differences, see [Autonomy (run-rules/4)](USAGE.en.md#autonomy-run-rules4) and the [run record contract](../contracts/run-record.md#47-run-rules4) (Korean).

In `/3`, to use more runs only, change the total run limit of the current window:
- For example, if usage is 48 runs against a limit of 50 runs, changing the limit to 100 gives 48 used and 52 remaining.
- If reduced to 40 runs, there are 0 remaining and 8 in excess; recorded usage does not disappear.
- An increase retains the user's raw words allowing the limit and the request context; a decrease retains the reason.
- Even a short "okay" ("좋아") is not rejected on form alone if context shows what was agreed to.
- If the quote source cannot be confirmed, it remains an agent-recorded claim.

Changes are linked to the status window number, budget revision, and current limit. If a change conflicts with another change or resume, it is never overwritten or summed automatically. Re-check current status to decide. If a storage error signals uncertainty of application, verify the record before reapplying. See the product contract for [exact commands and fields](../contracts/run-record.md#52-예산).

This change does not open a new window. It does not reset the time limit, elapsed time, failure or error limits, or blocked state. Changing the time limit is unsupported. Editing PLAN alone does not change in-progress limits. A completed window requires explicit rework or resumption; a new `/3` window inherits the latest run limit and the original time limit. Existing `/1` and `/2` goals open a new window with original limits via `reopen` quoting the user's statement, as before.

Budget is measured from the last start or `reopen` to the timestamp of the last saved record. A status check alone does not increase elapsed time, but if a new record is appended—including a budget change—the waiting time is included. Checks executed across resumptions are counted in the window where they are recorded. Actual attempts of checks and baselines, errors and inconclusive verdicts, and valid versus stale results are separate aggregation axes. Baselines are also included in usage.

Reaching the limit means usage equals the limit; exceeding it means usage actually surpassed the limit. A goal with a completion record does not enter `budget_exhausted` as time passes. Under `/2`, `/3` and `/4`, if all required conditions are satisfied and only budget exhaustion remains, completion can be recorded. This is not authority for additional execution; nor is budget a mechanism that automatically blocks check execution or terminates running checks.

## Continuing in a new conversation

Open a new conversation in the same project folder and send `docs/specs/<goal> 이어서 해줘` (or `continue docs/specs/<goal>`). Seal reads the goal, plan, progress records, and current status, then continues the remaining work. If any items remain that require user judgment, answer them first.

## Reporting verification evidence

The reporting format introduced in Seal 0.1.10 distinguishes grounds for expected results, actual observation boundaries, test methods, and alternative environments. Condition groups can reference the same grounds. Reviews record planned items; completion and discontinuation reports record actual results, unobserved scope, and remaining risks. Actual execution is not in itself grounds for correctness, and fake results do not turn into real model observations. Time sources and new facts that altered judgments are also distinguished.

Existing goals keep their starting formats, and omitted new items remain unrecorded. Past completions are never overwritten, nor are past observations backfilled. This reporting format is included starting with Seal 0.1.12 and is present in the current release v0.1.6. See the [synthetic reporting example](../examples/verification-reporting.md) (Korean).

## Assurance policy

The assurance policy introduced in Seal 0.1.11 links the required scope of an assurance policy specified by a goal or project to conditions. If required independent reviews or actual boundary evidence are missing, completion is withheld; unrelated work can continue. Optional comparisons record unobserved scope and limitations. The executor decides the model and execution order; Core does not treat reviewers' agreement as sufficient evidence. The [synthetic assurance case](../examples/verification-design.md) (Korean) is an example of linking the contract, not an actual model observation.

## Post-completion finding documents

The new reporting format in Seal 0.1.12 preserves the original completion report while linking subsequent findings to target versions, conditions, reproduction scope, judgment history, and rework requests. Unresolved findings and historical completions can be displayed at the same time. Document status does not substitute for Core structured queries or concurrent change handling; finding records alone do not resume execution or change budgets. The [synthetic finding history](../examples/verification-findings.md) (Korean) is not an actual run record.
