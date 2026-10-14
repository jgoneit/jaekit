# Native process-control command scenarios

`cases.json` describes observable CLI outcomes. The shell-free drivers in
`cmd/ha/native_process_test.go` and `native_process_windows_test.go` read these
cases and invoke the current command handlers in separate native processes.
Each case creates its own synthetic Git repository, committed producer and
declaration, goal, records, and private raw logs. Nothing uses a user's goal or
installed recorder. The `_native` location keeps these cases separate from the
generic scenario runner's `sh`-based `steps` format.

| Entry point | Cases | Observation |
| --- | --- | --- |
| `TestNativeWindowsProcessControlScenarios` | `parent-pass-check`, `parent-violation-check`, `parent-pass-baseline`, `parent-violation-baseline` | Actual Windows Job cleanup after the checker parent exits; original exit/result retained, descendant output stops, saved log bytes match the stable digest. |
| `TestNativeProcessTimeoutScenarios` | `timeout-check`, `timeout-baseline` | Actual native execution and descendant termination with a test-only three-second deadline; CLI exit 1, `error/process_timeout`, null checker exit, stable log/digest and one attempted run. A valid pass report written before timeout cannot override the process error. |
| `TestNativeProcessUnconfirmedShutdownScenarios` | `rules-3-check`, `rules-3-baseline`, `rules-2-check`, `rules-2-baseline`, `rules-1-check`, `rules-1-baseline` | After actual native execution succeeds, a test-only command-boundary adapter returns a synthetic unconfirmed-output error. `/3` records `error/process_output`, null checker exit, no digest and one attempted run; `/1` and `/2` preserve exit 70 with no new record or finalized log. |

The `TestProcessScenarioDriver` helper exists only in the Go test executable.
Ordinary commands use the actual `runMain` dispatch. Timeout and injected-error
checks call the same `cli.check` handler with an instance-local execution
dependency. Production has no fault-injection flag/environment switch and
retains its 900-second timeout. The timeout adapter changes only the deadline;
native starter, process containment, termination, output storage, classification,
recording and status computation remain actual product code.

The unconfirmed-output adapter verifies that the native executor really ran
and completed before replacing its boundary result. This observes CLI handling
of that failure contract, **not an actual Windows API failure**. The direct
`internal/run` unconfirmed-shutdown tests separately cover the executor's
no-digest behavior. The native parent and timeout cases use no synthetic
process-control result. The parent's child writes both an external heartbeat
and inherited stdout/stderr; observations require both to stop before the
child's finite fallback lifetime. Failed cases cannot pass by never starting a
child, relying on its fallback exit, or recording a still-changing log.

The parent-pass case also runs the same committed checker against the original
baseline source. It requires a declared violation there and a satisfied
criterion with both record IDs and both attempts in status, so matching raw
record result strings alone cannot establish usable completion evidence.

Legacy cases use a standalone synthetic start record with its supported rules
version set before any check runs. All later record bytes and record-chain
links are preserved. Actual Windows execution is required for the Windows-only
entry point; a cross-build or an empty/skipped test is not runtime evidence.
