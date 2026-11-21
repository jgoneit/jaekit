# Review evidence regressions

These synthetic checks cover unsupported execution, command extraction, history
range completeness, unittest result classification and development prerequisites.
They do not run a native model or replay historical user commands.

`produce.py AC-N` selects a declared class once. Only ordinary assertion failures
become declared violations. Runtime errors, expected-failure decorators and
skipped cases do not provide positive evidence.

The ordinary regression classes are offline and use the repository's developer
prerequisites.

## Explicit local retention audit

`preservation.py --retention INPUT.json` checks only the explicitly supplied
local input, offline; `--retention-only` is accepted and means the same. Without
`--retention` it exits 2. It never searches for local records.

The input contains an absolute `checkout` and a nonempty `documents` map from
relative file paths to lowercase SHA-256 digests. A supplied `runs.jsonl` is
read as records, and each recorded output is checked against `output_digest`.
Logical `.git/…` paths use `git -C CHECKOUT rev-parse --absolute-git-dir`, which
selects the linked worktree's Git directory. Logs from the common Git directory
or another worktree are never substituted. Absolute output paths and ordinary
checkout-relative output paths keep their existing meaning. Inherited Git
location overrides do not redirect the explicit checkout lookup.

A digest mismatch exits 1 and identifies document or output integrity. Invalid
input, unavailable Git lookup, missing files, and unreadable files exit 2 with
distinct reason names. The whole audit must succeed before printing PASS.
This is a read-only comparison, not a copy or repair operation. No records,
logs, repositories, or private documents are discovered or transmitted.
