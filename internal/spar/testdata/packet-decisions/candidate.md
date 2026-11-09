# Review packet

You are an independent reviewer. Attack the change: look for the ways it fails its criteria and rules. You cannot change the criteria; when meeting them needs a different requirement, report a `requirement-question`.

Read only the files in the review workspace: `head/` holds the tracked files of the head commit and `base/` those of the base commit. There is no Git history. Do not change, create or delete any file, and do not use the network.

## Round

- Stage: `candidate`. The head is a candidate implementation of the criteria. Review the whole change from base to head.
- Base commit: `<commit 0>` (tree `<tree 0>`)
- Head commit: `<commit 1>` (tree `<tree 1>`)

## Attack angles

Check every angle below. Report each one in `angles_checked` with what you checked, or in `angles_unchecked` with the reason; a result that leaves an angle out of both is not accepted.

- `requirement-counterexample`: Find an input, state or sequence for which the head violates a criterion or rule while its checks still pass, or fails where the behavior is correct.
- `side-effects`: Look outside the diff: callers, other commands, installers, documents and records that depend on what changed and now behave differently.
- `design`: Find a design flaw that makes a criterion impossible or fragile to meet: wrong ownership, duplicated rules that can drift, missing failure handling.
- `premises`: Name the assumptions the change rests on and test each one against the files. A premise that is false is a finding even when the code follows it.
- `check-validity`: For each check the change adds or changes: does it observe the required property at the checked tree, or a proxy? Can it pass on wrong code, or report a tool failure as a violation?

## How to report

- Reply with one JSON object that follows the result schema: `reviewed_head` (the head commit ID above), `verdict`, `angles_checked`, `angles_unchecked`, `findings` and `limits`.
- A finding needs a concrete `failure_scenario`: a specific input or state and the wrong outcome at the head. Without one, write the concern in `limits`.
- `smallest_fix` is the smallest change that would make the `failure_scenario` come out right: a direction in words, never a patch or code. Leave it empty when you cannot name one.
- `location` is `head/<path>:<line>`, `head/<path>:<line>-<line>` or `base/<path>:<line>`, or `head/<path>` for a whole file or folder. The path is a file or folder of that copy, and the line number after it is followed by a space, a comma, a semicolon or the end. A location that names no file or folder is an unreadable location: the finding is kept, but it is never counted as in the check paths and never deferred automatically.
- Give findings the IDs `F1`, `F2`, … in report order.
- `link_kind` is `criterion` with an `AC-n` from the criteria below, `rule` or `contract` with a document and a line (`AGENTS.md:12`) or a range inside the document (`AGENTS.md:12-14`), or `none`. A link that does not exist is recorded as `none`.
- `proposed_class`: `defect` (violates a linked criterion, rule or contract, or a regression this change made), `requirement-question` (only a change of requirement resolves it), `hardening` (an edge case nothing requires) or `note`.
- `introduced_by_change` is `yes` only when `base/` does not have the same problem; `regression` is `yes` when behavior that worked at base breaks at head.
- `check_impact` says what a finding in a check means: `false_pass` (the check passes on wrong code), `meaning_inverted` (it reports a pass as a failure or the reverse), `contract_violation` (it breaks a named contract or rule) or `none`.

## Criteria

### docs/SPEC.md

```
# Goal

- **AC-1** one
- **AC-2** two
```

## Rules

### AGENTS.md

```
# Rules

Keep reviews honest.
```

## Changes from base to head

```diff
diff --git a/app/main.py b/app/main.py
index 65489e7..07b8a7c 100644
--- a/app/main.py
+++ b/app/main.py
@@ -1 +1 @@
-print('one')
+print('two')
```
