# Contract: change check results

[한국어](check-result.md) · **English**

> This is a translation of the [Korean original](check-result.md). Where the two differ, the Korean original decides.

A change condition under `run-rules/3` links the baseline and the current check to the same declaration. This contract does not treat an ordinary nonzero exit as the intended requirement violation. The role of existing runs under `/1` and `/2`, and of ordinary maintain, manual and task checks, stays as it is. New starts use `/3` ([run record](run-record.en.md)).

## 1. Declaration and input

Before running, the check author declares the required targets and the expected violations in a JSON file and links it from the `## 결과 계약` (result contract) table in PLAN.

```markdown
## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | checks/declaration.json |
```

The declaration file and every file in `producer_paths` go in the condition table's `검사 경로` (check paths). All of them must be tracked regular files at repository-relative paths, and symlinks are not used. The current bytes of the declaration and of the producer and configuration files must equal HEAD. Even when they are inside another execution document exclusion path, uncommitted content cannot change the meaning of the classification. The baseline overlays these files from HEAD onto the base commit. Dependencies and environment of the temporary worktree are not installed automatically.

The declaration and reference configuration below are a hypothetical greeting example. They are not real user data.

### Declaration example (`checks/declaration.json`)

```json
{
  "schema": "check-declaration/v1",
  "producer": "jaekit-reference",
  "producer_version": "1",
  "producer_paths": ["tools/check-result-reference.py", "checks/reference.json"],
  "targets": [{"id": "greeting", "violation": "missing-greeting"}]
}
```

`producer`, `producer_version`, and a target's `id` and `violation` are identifiers of 1 to 128 ASCII alphanumerics, underscores, dots, colons, slashes and hyphens, starting with an ASCII alphanumeric. There are 1 to 128 producer paths and 1 to 1024 targets. Each `id` in `targets` is unique, and every target is required. `violation` is the identifier to observe when that requirement is not met. The declaration, the targets, the result contract, and the producer version and content are part of the evidence identity. Even when the command string is the same, if these inputs change, earlier evidence and completion are not reused as they are.

## 2. Per-invocation report

`ha` passes the following environment variables to the check process.

| Variable | Meaning |
| --- | --- |
| `HA_EVIDENCE_PATH` | Local file to write this invocation's structured report to |
| `HA_EVIDENCE_INVOCATION` | Identifier of this invocation |
| `HA_EVIDENCE_DECLARATION_DIGEST` | Digest of this declaration and the producer inputs |

The producer links the invocation identifier and digest into the report unchanged. A file from an earlier invocation, or a report for another declaration, target or producer, is not valid. stdout/stderr stay in the existing local run log and do not replace the report file.

```json
{
  "schema": "check-result/v1",
  "invocation": "<HA_EVIDENCE_INVOCATION>",
  "declaration_digest": "<HA_EVIDENCE_DECLARATION_DIGEST>",
  "producer": "jaekit-reference",
  "producer_version": "1",
  "attempts_complete": true,
  "observations": [{"target": "greeting", "attempt": 1, "status": "violation", "violation": "missing-greeting"}]
}
```

| `status` | Observation and additional fields |
| --- | --- |
| `pass` | The target ran and passed. Do not include `violation` or `reason` |
| `violation` | An actual assertion failure. Put the observed violation identifier in `violation` |
| `error` | An error that prevented the check. `reason` is one of `dependency_missing`, `collection_error`, `import_error`, `compile_error`, `browser_start_error`, `setup_error`, `execution_error`, `permission_denied` |
| `skip` | The target did not run. `reason` is `skipped` or `not_selected` |

The reference producer observes each target once, without internal retries. To report retries under the common contract, `attempts_complete: true` and the full `attempt` history for each target, increasing from 1 without gaps, are required. There are at most 4096 observations in total. When only the last attempt remains, a number is duplicated or the order is reversed, the result is inconclusive with `attempt_history_invalid`; without the completeness flag, it is inconclusive with `attempt_history_missing`. When pass and violation are both present for the same target, the adverse result is kept as `fail` with `inconsistent_attempts`, even if the final exit is 0. An integration that cannot obtain retry information is unsupported or inconclusive.

The declaration and the report must be JSON objects of 1 MiB or less; duplicate keys, unknown fields, trailing data and unsupported schemas are refused. The report file must also be a regular file. An unsupported ordinary script can record actual attempts without a declaration, but it does not obtain positive evidence for a `/3` change.

## 3. Classification and retention

| Stage | Stored `result` | Meaning |
| --- | --- | --- |
| Current check | `pass` | Normal exit and all targets passed |
| Current check | `fail` | An actual assertion failure |
| baseline | `fail_as_expected` | A declared violation observed with an abnormal exit, and no other failure, error or non-run |
| baseline | `unexpected_pass` | All targets pass even in the base state |
| baseline | `fail` | An actual assertion failure other than the intended one, or a pass/violation inconsistency among internal attempts |
| Both | `error` | Process start, timeout, signal or output storage error, or a reported environment error |
| Both | `unknown` | Cannot be classified because something is unsupported, missing, conflicting, not run and the like |

The `reason` in the compact evidence is a classification reason code. For example, `unsupported_declaration`, `report_missing`, `report_binding`, `attempt_history_missing`, `attempt_history_invalid`, `target_missing`, `target_skipped` and `report_exit_conflict` explain an inconclusive result. `environment_reported` is an environment error, and `unexpected_assertion` is a failure other than the intended one. Raw messages are not put in reason codes.

A current pass needs both a normal exit and an actual pass of every declared target, and the full internal attempt history must also have no adverse observation. A baseline expected failure needs an abnormal exit, observation of all declared targets, and at least one declared violation. It is not enough when environment errors, non-runs or assertion failures other than the intended one are mixed in. An actual assertion failure other than the expected violation remains an adverse failure.

Process start failure, timeout, termination by an external signal, and import, compile, collection or browser startup failure are not expected failures in themselves. On the other hand, a runnable check that directly observes the absence of a feature and reports it as the declared violation can be valid. Zero targets, all targets skipped, a missing, corrupt or unsupported report, and a conflict between exit and report are not promoted to success or expected failure either.

When an environment error or a skip occurs together with an assertion violation, the run as a whole can be `error` or `unknown`. The per-target adverse observation in the compact evidence still remains. A later normal run on the same input does not hide that assertion violation.

Errors and inconclusive results both count toward the existing error retry limit. On the same evidence input, a later pass does not erase an earlier adverse assertion result, an unexpected baseline pass or an exceeded error history. Renaming the check kind does not increase the retry allowance. Actual check attempts and usage remain whether or not the result is adopted, and baselines count too. Refusals of input or of a dirty tree before the run, and failure to prepare the temporary worktree, are distinguished from checks that ran.

When the log disappears or fails to be stored after an actual attempt, `error` with `process_output` and the usage are recorded if the record store can still be written. In this case `output_path` and `output_digest` are empty, and the raw output may not be viewable. If storing the record itself failed, it is not reported as success, and for a failure after the write, whether it was applied may be uncertain.

The record stores compact evidence in `check-evidence/v1`: `result`, `reason`, `declaration_digest`, `report_digest`, `invocation`, `targets`. Only target observations go in; the actual values of checked files and raw logs are not copied. The current state is rebuilt from this evidence, so a missing raw log or a change in the current time alone does not change the judgment. The executor can write the declaration and the report. The assurance level stays `local`, and it does not guarantee an independent understanding of the requirements or third-party verification.

Mixed results and failure attribution of the goal, review and gap producers used for repository development, and their version 2 target makeup, are defined separately in the [repository producers contract](repository-producers.en.md). That does not change this contract's report format or the installation contract of the general reference producer.

## 4. Actual reference integration

The [reference producer](../tools/check-result-reference.py) uses only the Python 3 standard library. Copy it into the project that needs it, track it, and put it in the check paths together with the declaration and configuration. It does not automatically install other runners or dependencies. This does not mean the reference producer contains pytest, Vitest or Playwright integration. Adapters that read runner output directly are in §5.

Since v0.1.3, the direct installation and Homebrew provide this producer together with the [synthetic example](../examples/check-result/README.md). Without a development checkout, you can copy them from the release path in [Get the reference check](../guides/INSTALL.en.md#get-the-reference-check). Python is needed only when choosing this producer and is not a common required dependency of Spec and Seal.

### Reference configuration example (`checks/reference.json`)

```json
{
  "checks": [{
    "target": "greeting",
    "path": "src/greeting.txt",
    "equals": "hello\n",
    "violation": "missing-greeting",
    "missing": "violation"
  }]
}
```

`missing: violation` declares that this check directly observes the absence of the target file as a requirement violation. When it is omitted or `error`, a missing file is an environment error. The optional `requires` lists file paths that this check's environment needs. When such a file is missing, the result is `dependency_missing`, and it is not turned into a requirement violation. Actual file content is not put in the report.

Use the following command and check paths in the condition table. Do not create the environment variables by hand to use a standalone producer's report as completion evidence.

```markdown
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json` | checks/declaration.json, checks/reference.json, tools/check-result-reference.py | T001 |
```

In a base state where `src/greeting.txt` is missing or has another value, the declared violation is observed. When the current code stores `hello` and a newline, the same target passes. This example does not stop at injecting the declaration into the judge; it reads the actual file in a separate process. The check author must decide the actual behavior and verification targets of the project it is applied to.

When CRLF/LF conversion of check inputs makes the actual bytes differ from HEAD, the input is refused even if Git sees the tree as clean. The newline difference is diagnosed and checkout conditions are suggested, but files, Git settings and existing digests are not changed automatically, and normalized bytes are not accepted as the same evidence.

## 5. Runner adapters

A change condition under `run-rules/3`, `run-rules/4` or `run-rules/5` can link a target to one existing test through a runner adapter instead of a producer. Only an executable whose `ha capabilities` has `check-adapter/v1` in `formats.check_declaration` and `runner-adapters/v1` in `features` supports this. Adapters are optional. The format and meaning of producer declarations (§1 to §4) stay as they are; without support, use the reference producer or a dedicated producer. `/1` and `/2` goals, and maintain, manual, task and `EX-n` checks, are judged by exit code as now even when the result contract has an adapter declaration, and placeholder words are not replaced.

### Declaration

The `결과 계약` table and the `검사 경로` rules are the same as in §1. An adapter declaration has no list of producer files. Below is a hypothetical greeting example.

```json
{
  "schema": "check-adapter/v1",
  "adapter": "go-test-json",
  "targets": [{"id": "greeting", "violation": "greeting-missing",
               "test": {"package": "example.com/greet/cmd/greet", "name": "TestGreetsWithHello"}}]
}
```

| `adapter` | Verification command | `test` |
| --- | --- | --- |
| `go-test-json` | `go test -json …` as is. `ha` reads its standard output | `package` (import path) and `name` (the full test name that `go test -json` reports; a subtest is `TestX/case`) |
| `junit-xml` | A runner command that writes JUnit XML. It receives the output location through the notation below | `classname` (empty allowed) and `name` |

There are 1 to 1024 targets, and `id` and `violation` are identifiers as in §1. `ha lint` shows the following as `evidence_invalid`: an unknown adapter, a target without `test`, a name field that does not fit the adapter, an empty name, a name containing control characters (C0, DEL, C1), a name over 1024 bytes, a Go package path containing whitespace, a name that does not fit the Go test name format (the part before the first `/` is a Go identifier that starts with `Test`, `Fuzz` or `Example` and whose first character after that prefix is not lowercase, and there is no whitespace anywhere in the name; subtest names can contain `/`, so empty segments as in `TestX/a//b` are allowed), two targets linked to the same test, a `violation` starting with `jaekit:` (reserved), and a `violation` ending with `:` or `.` (it cannot be read from a marker). The `violation` of a producer declaration (`check-declaration/v1`) may still end with `:` or `.`. Unknown fields, duplicate keys, trailing data and more than 1 MiB are refused as in §2.

The evidence identity includes both the declaration bytes and the version of the adapter's judgment meaning. When an adapter comes to judge the same output differently, that version goes up, and earlier evidence from that adapter is not reused ([run record](run-record.en.md)).

### Output location

For a `junit-xml` condition, each run creates a new private directory under `ha/runs/<goal>/` in the Git directory and sets `junit.xml` in that empty directory as the output location. `{HA_JUNIT_XML}` in the command arguments (it may be part of an argument) is replaced with that path, and the same path is also passed in the environment variable `HA_JUNIT_XML`. The record's `argv` and command digest are those of the command before replacement, and the path is not kept in the record. Files from earlier runs and files of the same name in the working tree are not read. Example:

```markdown
| AC-2 | change | `node --test --test-reporter=junit --test-reporter-destination={HA_JUNIT_XML} test/greet.test.mjs` | test/greet.test.mjs, checks/AC-2.json | T001 |
```

For a `go-test-json` condition, the command's standard output is captured separately and read to the end. When a child process keeps standard output open after the command ends, it is read for at most 5 more seconds, and if it has not ended by then, the truncated output is not judged (`process_wait` in the table below). This read happens before `ha` cleans up remaining child processes, so it is the same on every operating system. The log keeps standard output and standard error together as now, and `ha log` shows it. Standard error (module download notices, vet warnings, raw build errors) is not used for judgment.

### Marker

A marker is the fixed string `jaekit:violation=` followed by an identifier. The identifier is the longest run of identifier characters (`A-Z a-z 0-9 _ . : / -`) immediately following it, with all trailing `:` and `.` removed. So the identifier of `jaekit:violation=greeting-missing: got "hi"` and of `jaekit:violation=greeting-missing.` is `greeting-missing`, and `:` and `.` in the middle (`a.b:c`) stay. Matching is case-sensitive, and when the identifier after removal is empty, longer than 128 characters or does not start with an alphanumeric, it is not a marker. Markers are read only from the first line of a failure message, at any position within that line.

- Go: in the linked test's own output, each line whose `OutputType` is `error` is the first line of one `Error`- or `Fatal`-family call. `error-continue` lines, lines without a type (`t.Log`, standard output written directly) and `frame` lines are not first lines of a failure message.
- JUnit: for each `failure` element of the linked testcase, it is the first line of the `message` attribute when that attribute exists (even if empty), and otherwise the literal first line of the element body. `system-out`, `system-err` and the body from its second line on (traceback, stack trace, source excerpts) are not read.

When the linked test fails, the first line of every failure message has a marker, and all those markers have the same identifier, that identifier is the observed violation. When there is no failure message, any one of them lacks a marker, or different identifiers are mixed, it is an unmarked failure and the observed value is `jaekit:unmarked-failure`. Only when the observed violation equals the declared `violation` is it the declared violation. Other identifiers and unmarked failures are `unexpected_assertion`. A declared violation cannot start with `jaekit:`, so an unmarked failure never becomes any declared violation.

**Limit.** A case where a compared value or repr contains the marker string and is printed on the first line is not distinguished. Use the marker only in the failure message of an assertion that checks the declared property, and do not put it in compared values or output values.

### Targets and attempts

- One target links to one test. Names must match exactly; regular expressions and prefix matches are not used.
- Go tests are found by package import path and full test name. Linking to a subtest observes that subtest; linking to a parent test looks only at the parent test's own failure messages. A parent test that failed only because a subtest failed has no failure message, so it is an unmarked failure. When the linked test runs several times in one command (`-count`), the runs become attempts 1, 2, … in order.
- JUnit matches a testcase's `classname` and `name` exactly. When the declaration's `classname` is empty, both a testcase without a `classname` attribute and a testcase with `classname=""` match, and a testcase with another `classname` does not. When two testcases with the same `name` both match this way, it is a duplicate of the same testcase. The `file` attribute is not used. JUnit XML has no attempt history, so when two or more testcases are linked or retry elements (`rerunFailure`, `rerunError`, `flakyFailure`, `flakyError`) are present, the result is inconclusive.
- Results of tests that are not linked are not target observations. In a current check, when all linked tests pass but the command fails, it is `report_exit_conflict`. In a baseline, failures of tests that are not linked do not change the target judgment.

Observations are classified exactly by the rules of §3 and stored as a `check-evidence/v1` summary. `report_digest` is the digest of the runner output that was read. The runner's failure messages, test output and XML content are not put in the record or the reason codes.

### Results of input failures

The following table is the complete list. Output forms outside the list (in the package events of the linked test, a Go action the adapter does not know (other than `start`, `run`, `pause`, `cont`, `pass`, `fail`, `skip`, `output`, `bench`, `build-output`, `build-fail`, `attr` and `artifacts`, which Go 1.27 `go test -json` writes) or an `OutputType` value (other than `error`, `error-continue`, `frame`) appears, or an `output` event without `Output` appears; an unsupported child element of the linked testcase (namespaced elements included), a combination of `skipped` with `failure` or `error`, or an element inside `failure`) are `observation_invalid`. None of them is a pass, an expected failure or the declared violation.

| Case | Result (same for current and baseline) | Reason |
| --- | --- | --- |
| The runner cannot start (not on PATH, no execute permission) | `error` | `process_start` |
| Hit the `ha` time limit | `error` | `process_timeout` |
| Terminated by a signal | `error` | `process_signal` |
| Failed to store the command output | `error` | `process_output` |
| After a `go-test-json` command ends, a child process keeps standard output open for more than 5 seconds. The exit code and the output read up to then are not used for judgment. When the command ended by a signal, the signal row above takes precedence | `error` | `process_wait` |
| The package of the linked Go test does not build | `error` | `environment_reported`, target `compile_error` |
| The linked Go test started but has no end record (pass, fail, skip) (abnormal exit of the test binary, Go's own test timeout) | `error` | `environment_reported`, target `execution_error` |
| The linked testcase has an `error` element. The same holds when `failure` is also present | `error` | `environment_reported`, target `execution_error` |
| The linked test is not in the output (name mismatch, not selected, only other names because collection failed) | `unknown` | `target_missing` |
| The linked test was skipped (Go skip, JUnit `skipped`) | `unknown` | `target_skipped` |
| No runner output. Go: no events on standard output. JUnit: no file at the announced location, or not a regular file (directory, symlink) | `unknown` | `output_missing` |
| Runner output is corrupt. An unparsable JSON line, an empty line, truncated JSON, non-UTF-8 bytes, an XML syntax error or truncation, a non-UTF-8 encoding declaration, a DTD or entity declaration, two roots or a root that is not `testsuites` or `testsuite`, duplicate attributes | `unknown` | `output_corrupt` |
| Runner output exceeds the size limit (64 MiB of `go test -json` standard output, 16 MiB of JUnit XML). The truncated first part is not judged | `unknown` | `output_too_large` |
| Duplicate of the same JUnit testcase, retry elements | `unknown` | `attempt_history_missing` |
| Exit code and observation disagree (nonzero although all targets pass, 0 although there is a violation or error) | `unknown` | `report_exit_conflict` |

As in the current rules, an exit code conflict applies before environment errors and skips. For example, when there is an `error` element but the runner exits 0, the result is `unknown` (`report_exit_conflict`), and the target observation remains. When different pre-observation reasons overlap across several targets, one is chosen in the order `attempt_history_missing`, `observation_invalid`, `target_missing`.

### Supported output

- `go test -json` adds the output type (`OutputType`) to failure message lines starting with Go 1.27. When the package events of the linked test have no `OutputType` at all (a Go 1.26 or earlier toolchain), a linked test that failed in that package is an unmarked failure, and `ha check` prints, after the result line, one `notice: <조건> go-test-json: …` line that names that package (and mentions `OutputType`).
- JUnit XML is UTF-8 (BOM allowed) and has one root element (`testsuites` or `testsuite`), and testcases may be at any depth. Namespaced elements (`ext:failure`, `ext:testcase` and so on) are not JUnit elements. They do not match as testcases, and inside a linked testcase they are unsupported children. The forms confirmed once with actual output are the Node.js 26.11.0 built-in runner (`node --test --test-reporter=junit`), pytest 9.1.1 (`--junitxml=…`), Vitest 5.0.5 (`--reporter=junit --outputFile=…`), and Jest 30.5.2 with jest-junit 17.0.0 (`JEST_JUNIT_OUTPUT_FILE`). In that output, passes, marked failures, unmarked failures, skips and a pytest fixture error (`error`) were judged as in the representative forms table. With default settings, pytest writes the module path (`tests.test_greet`) in `classname`; Vitest writes the file path in `classname` and `describe > test` in `name`; and jest-junit writes `describe test` in both `classname` and `name`. pytest and Vitest failures are read from the first line of the `message` attribute, and jest-junit failures, which have no `message` attribute, from the first body line (`Error: …`). The repository's checks confirm this with synthetic documents of the same shape, without installing these runners. Output from other versions and user settings has not been checked.

### Representative forms

These are representative examples of an open-ended input. The rules apply in the same way to forms not in the list.

| ID | Adapter | Form | Expected |
| --- | --- | --- | --- |
| G1 | go | The linked test passes and `t.Log` has a marker | Target passes |
| G2 | go | Fails with `t.Fatalf("jaekit:violation=<선언 id> …")` | Declared violation |
| G3 | go | Marker in `t.Log`, then fails with an unmarked `t.Fatal` | `unexpected_assertion` |
| G4 | go | `t.Error` twice, only one marked | `unexpected_assertion` |
| G5 | go | Marker only on the second line of a multi-line error | `unexpected_assertion` |
| G6 | go | Panic inside the test | `unexpected_assertion` |
| G7 | go | A subtest fails with a marker; linked to the parent test | `unexpected_assertion` |
| G8 | go | Linked to a subtest, and that subtest fails with a marker | Declared violation |
| G9 | go | The overlaid test file calls a function missing from the base commit, so the build fails | `error`, `compile_error` |
| G10 | go | The test binary ends while the linked test is running (`os.Exit`, crash) | `error`, `execution_error` |
| G11 | go | With `-count=2`, one pass and one declared violation | `fail`, `inconsistent_attempts` |
| G12 | go | The same test name in two packages, linked in only one package | Only the linked package's result |
| G13 | go | A non-JSON line or truncated JSON on standard output | `unknown`, `output_corrupt` |
| G14 | go | Nothing on standard output; text only on standard error | `unknown`, `output_missing` |
| G15 | go | Fails with a marker on a toolchain that does not mark the output type | `unexpected_assertion`, one `notice:` line |
| G16 | go | A `:` right after the marker, as in `t.Fatalf("jaekit:violation=<선언 id>: got %q", …)`, or a message that ends with `jaekit:violation=<선언 id>.` | Declared violation |
| J1 | junit | A testcase without children | Target passes |
| J2 | junit | A failing pytest `assert …, "jaekit:violation=<선언 id>"` | Declared violation |
| J3 | junit | A pytest assert failure without a message. Another source line containing a marker appears in the traceback body | `unexpected_assertion` |
| J4 | junit | A failing Vitest `expect(값, "jaekit:violation=<선언 id>")`. The message's first line is `jaekit:violation=<선언 id>: expected …` | Declared violation |
| J5 | junit | A failure without a `message` attribute whose first body line is `Error: jaekit:violation=<선언 id>` | Declared violation |
| J6 | junit | A fixture setup error appears as an `error` element | `error`, `execution_error` |
| J7 | junit | A `skipped` element | `unknown`, `target_skipped` |
| J8 | junit | No tests collected (empty testsuite) | `unknown`, `target_missing` |
| J9 | junit | Two of the same testcase | `unknown`, `attempt_history_missing` |
| J10 | junit | A failure with a marker only in `system-out` | `unexpected_assertion` |
| J11 | junit | No file at the announced location, a truncated file, a DTD declaration | `unknown` (`output_missing`, `output_corrupt`) |
| J12 | both | The same output with CRLF line endings | Same result as LF |
| J13 | junit | The declaration's `classname` is empty, and the testcase has `classname=""` or no `classname` | That testcase's result |
| J14 | junit | The declaration's `classname` is empty, and there are two testcases with the same `name`, one without `classname` and one with `classname=""` | `unknown`, `attempt_history_missing` |
