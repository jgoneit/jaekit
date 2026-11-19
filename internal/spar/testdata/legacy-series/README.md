# Legacy review series

`TestSparStageMergeAC7_LegacySeriesReadUnchanged` in `internal/spar/stage_merge_test.go` reads this
fixture. It is a review series that Spar recorded with the stages `plan-delta` and `fixes`, before
new rounds could only start as `plan` or `candidate`. The test checks that `spar gate` and
`spar cap-report` still read such a series byte for byte as they did then.

## The series

`series/` holds the record files of each round (`round.json`, `findings.json`, `ledger.jsonl`) and
the series' `extensions.jsonl`:

| Round | Stage | Base → head | Findings and dispositions |
| --- | --- | --- | --- |
| `01-plan` | plan | c0 → c0 | a requirement-question answered, a defect rejected |
| `02-plan-delta` | plan-delta | c0 → c0 | none |
| `03-candidate` | candidate | c0 → c1 | a defect fixed in c2, its duplicate, a hardening item deferred by Spar, a requirement-question answered |
| `04-fixes` | fixes | c1 → c2 | a defect deferred to `#901` with the user's words, a note without a disposition |
| `05-candidate` | candidate | c0 → c2 | a defect rejected |
| `06-fixes` | fixes, dry run | c2 → c3 | none |

`extensions.jsonl` records one code extension by one round with the cause `churn`. Every round has
one native reviewer, `host`, answered with `spar record`. The rounds record the caps of that Spar
(plan 1, plan-delta 1, code 3) and the check path `checks/**`.

c0 to c3 are the commits that `newSMWorld` makes. Their author and committer names, emails and
dates are fixed and Git reads no user or system configuration, so the commit IDs are always the
ones in `smLegacyCommits`. The test stops with a setup failure when they differ.

The files were written once by `spar run`, `spar record`, `spar triage` and `spar extend` of
commit eb093be in that repository, with the spar version `eb093be`. Two things were then replaced:

- the test's temporary folder with `@DIR@`, which the test replaces with its own folder when it
  copies the series into `<folder>/repo/reviews/legacy`;
- every recorded time (`started_at`, `finished_at`, `recorded_at`) with `2026-01-01T00:00:00Z`.
  Neither the gate nor the cap report shows these times.

Spar after that commit can no longer start `plan-delta` or `fixes` rounds, so these files are kept
as they are and are not regenerated.

## Golden outputs

The test runs these commands through `spar.Main` and compares their output with the files here:

| File | Command |
| --- | --- |
| `gate.golden.json` | `spar gate` over `01-plan` to `05-candidate`, `--expect-head` c2: the file it writes |
| `cap-report.golden.md` | `spar cap-report <series> --format md`: standard output |
| `cap-report.golden.json` | `spar cap-report <series> --format json`: standard output |

The golden files were generated from the code at commit eb093be, the tree before the change. Before
the comparison the test normalizes only these fields:

- the test's temporary folder, in the gate's round `path` fields and growth round `path`, and in
  the cap report's series, becomes `@DIR@`;
- the gate's top-level `written_at` becomes `@WRITTEN_AT@`.

To regenerate the golden files, run from the repository root:

```bash
SPAR_STAGE_MERGE_GOLDEN=update go test -count=1 -run '^TestSparStageMergeAC7_' ./internal/spar
```

Regenerate them only when a change is meant to alter how Spar reads a recorded series, and review
the difference: the check exists to show that such reading did not change.
