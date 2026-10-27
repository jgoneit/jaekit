# What repository check producers observe

[한국어](repository-producers.md) · **English**

> This is a translation of the [Korean original](repository-producers.md). Where the two differ, the Korean original decides.

This document describes the goal, review and gap check producers of the Jaekit repository. The general reference producer that is installed and Core's `check-result/v1` format do not change. These tools are development tools that observe local check material. They do not guarantee independent certification or that checks are sufficient.

## Targets and results

Producer version 2 records distinct facts from one run of a test suite as separate declared targets. Each target has `attempt: 1`, and test cases or result kinds are not expressed as internal retries.

| Target | Fact observed |
| --- | --- |
| The original target and `additional_violations` | Actual assertion violation identifiers. Remaining slots show that there were no additional violations. When execution or selection itself does not succeed, the original target does not pass either. |
| `execution` | Execution, environment and collection errors. Each is kept even when assertion failures are also present. |
| `selection` | Cases that could not run. A skip does not disappear when other cases passed or failed. |
| `attribution` | Whether a failure can be linked to a specific requirement violation. An unclassified assertion keeps its fact and records an error on this target. |

Name the same targets in the configuration's `result_targets` and in the declaration's `targets`. The default configuration uses 8 assertion slots and 3 other properties. Set the number of slots needed, from 2 to 64, before the run. When more failures are observed than there are slots, the failures in the declared slots and a `collection_error` are recorded. When another execution or configuration error already occupies `execution`, the last assertion slot reports `collection_error`, so that error and the missing observation are both recorded. The full facts and the original diagnostics remain in the local output, and it does not claim that everything that overflowed was stored in the concise report. An incomplete observation is neither a success nor a normal expected baseline failure.

The observed identifiers of several failures are each recorded in a separate slot. Two slots with the same identifier are not a retry. A declaration may name the expected violation for each slot, but the producer never copies that value as an actual violation identifier.

## Identifying actual assertions

For Python, identifiers come only from exceptions that were actually raised and that unittest classified as failures. `RequirementViolation` in `tools/result_observations.py` is preferred. A string tag is supported only as the explicit prefix of an actual exception from a call that the test code makes directly to `self.fail("[violation:식별자] 설명")`. A `fail` called inside a standard unittest comparison function, a tag in a compared value, a repr or later in a message, and a plain `AssertionError` raised directly are not evidence for an identifier. The existing `[requirement]` also means the fixed observation identifier `requirement-not-met`, but only as a prefix passed directly to `self.fail`. Printing the same string to standard output, or putting a message on a passing assertion, is not evidence of a violation. The broad string reading that searched the whole exception message for a tag is not supported.

An assertion without a name is recorded as an `unclassified-assertion` violation and an attribution error. Changing the declaration to this name does not remove the attribution error, so it is not promoted to a normal expected failure. Expected-failure decorators and unexpected successes are configuration errors, as before. Failed subtests, teardown errors and skips are also preserved as separate facts.

Go test JSON can observe passes, failures and skips, and compile and process errors, but the output text alone cannot establish the requirement identifier of a failed assertion. Logs and assertions use the same output path, so no name is assigned by guessing from a tag or a source line. A Go failure without specific evidence is recorded as an unclassified failure and an attribution error. The package and the test name are identified together, so the same test name in several packages does not overwrite another package's result. There is no guarantee of reproducing the earlier baseline acceptance that turned an ordinary Go failure into a declared ID. The distinction between passing Go tests and existing runtime errors is kept. A broader Go adapter is not part of this contract.

## Commands and compatibility

A command's nonzero exit alone never decides a requirement violation. A command used as structured evidence sets `pass`, `violations` and `unverified` of `jaekit-command-exits/v1` without overlap. `violations` maps exit code strings to the actual violation identifiers that the command observes, and it is separate from the Core declaration that holds the expected violations. Undocumented exits, signals, failure to run and timeouts are never converted into violations. The keys of `violations` write exit codes 1 to 255 as ASCII decimal without leading zeros (`"1"`, `"255"`). `"01"`, digits from other numeral systems, keys that contain spaces or signs, and duplicate keys in the same JSON object are refused as configuration errors before the command runs. Every command target that the repository provides has an exit contract. The public documentation check ends with 1 for a document problem, UNVERIFIED 3 when a document is missing or unreadable, and 70 for an internal error; the retention and history audits use 70 for an internal error. Exit 1 therefore means only a problem that the tool actually found. The audits' UNVERIFIED exit 2 is unobserved, not a success or an intended baseline failure.

The goal configuration uses `jaekit-goal-targets/v2` and producer version `2`. An existing goal configuration v1 can still check the same exit success, but only for maintain runs without a structured report. A v1 single-target structured declaration is refused before the run with a message that it is not supported. The review and gap configurations also need explicit `result_targets`. The currently tracked configurations, declarations and producer inputs are updated together, but past records are never edited. The bytes of the new producer and declarations have changed, so past evidence is not reused as an observation of the new code.

The standalone Go result CLI declares the same observation targets and supports only producer `repository-go-check`, version `2`. The declaration's producer inputs include all of `go-result.py`, `produce.py` and `result_observations.py` that it actually reads, together with the related configuration and check files. The report file is newly created, and an existing file is never overwritten. `classify()` is only a v1 debugging function that past fixtures use; the current CLI does not use it to judge evidence. The file list and the link between declaration and report follow the existing [result contract](check-result.en.md).

## Verification scope

`tools/producer-checks/test_semantics.py` covers normal, unobserved, error and mixed results and failure attribution for real Python, Go and command processes. `cmd/ha/producer_semantics_test.go` covers the boundary where a report from a real producer process carries through to Core's current and baseline classification and the stored target observations. These regressions are selected by the existing common verification and the Go CI group. They are tests with synthetic inputs and local processes, and they do not replace an audit of past usage history, a real model run or a remote CI run.
