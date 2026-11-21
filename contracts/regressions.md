# Common regression connections

[regressions.json](regressions.json) lists the protected tests that guard five Jaekit invariants. It has `schema` (`regression-map/v1`), `invariants` and `routes`.

Each invariant has `id`, `basis` (the files that state it) and `property`:

| ID | Property |
| --- | --- |
| I1 | Current-file checks are independent of private material and unnecessary history |
| I2 | Actual result meaning survives producer, current/baseline judgment and saved evidence |
| I3 | Request binding and uncertainty survive observation consumers |
| I4 | Required tools and selected failures remain visible |
| I5 | Public scanners distinguish rules and normal fixtures without broad evasion |

Each route has `id`, `invariant`, `sources` and `checks`. `invariant` is one of I1–I5, and `sources` lists the files whose behavior the route protects. Each check has `file`, `selector` and `group`: `selector` is a Go test name in `file` (`TestName` or `TestName/subtest`) or a Python unittest class or method in it, and `group` is the `tools/verify.py` group that runs the file, `go` or `docs`. Every basis, source and check file is tracked by Git. `tools/workflow-checks/test_regressions.py` checks this shape.

## Adding a regression

To add a regression, write the test and list it under its route's `checks`. Full verification (`python3 tools/verify.py`) must run the file: a Go test file runs in `go test ./...`, and a Python test file sits in a directory and pattern that a discover step of `tools/verify.py` covers. Full verification fails a discover step that collects no tests, so a renamed or emptied test file cannot pass unnoticed. A property that no route covers gets a new route under its invariant.

## Protection

Deleting or weakening a protected test needs the user's words in the pull request. A check bound to a protected path changes only in a separate goal that changes that path. Renaming a goal or a branch is not a reason to remove a route.
