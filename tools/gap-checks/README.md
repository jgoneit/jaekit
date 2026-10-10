# Review evidence regressions

These synthetic checks cover unsupported execution, command extraction, history
range completeness, unittest result classification and development prerequisites.
They do not run a native model or replay historical user commands.

`produce.py AC-N` selects a declared class once. Only ordinary assertion failures
become declared violations. Runtime errors, expected-failure decorators and
skipped cases do not provide positive evidence.

The ordinary regression classes are offline and use the repository's developer
prerequisites. `preservation.py` also queries public release identity and runs
standard verification; its optional local retention input is explicit and is
never searched for automatically. `delivery.py` reads the public issues and
review replies for the specified PR. These two acceptance commands require
authenticated GitHub reads and are excluded from ordinary CI.
