# Review boundary regressions

These synthetic fixtures check the release observer, safe installation cleanup,
current-file and history-audit boundaries, and Go result classification. They do
not invoke a native model, change the user's installation, or read private goal
records.

`python3 tools/verify.py` runs the offline observation, installation and producer
suites and the three offline `tools/gap-checks/test_*.py` suites in its Go group.
They require Go, Python 3, Git, bash and zsh. The default and `go` entrypoints
resolve bash and zsh from PATH, check that each executable can start before
running checks, and pass those same absolute paths to the installation and
observation regressions. A missing or unusable shell fails with its name and
affected check scope; no shell coverage is silently skipped. Direct regression
invocations use PATH when no verified shell path was supplied. The `docs` group
does not require Go or zsh.

The ordinary
verification path does not require Docker or query GitHub release evidence.
The Ubuntu Go CI job prepares zsh before running these regressions. The
verification command itself never installs tools.

`produce.py AC-N` selects one acceptance suite from `suites.json`. Its structured
report treats assertion failures as declared violations; collection failures,
process errors, skipped selection and incomplete execution cannot prove an
expected baseline failure.

The execution collector tests launch local shells and a tiny fixture executable.
Passing them is local regression evidence, not a new Codex/Claude session run or
an installation/release certification.
