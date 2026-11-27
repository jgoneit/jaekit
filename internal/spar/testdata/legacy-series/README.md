# Legacy review series

`series/` is a review series that spar of commit eb093be recorded with `spar run`, `spar record`,
`spar triage` and `spar extend`, before new rounds could only start as `plan` or `candidate`:

| Round | Stage | Base → head | Findings and dispositions |
| --- | --- | --- | --- |
| `01-plan` | plan | c0 → c0 | a requirement-question answered, a defect rejected |
| `02-plan-delta` | plan-delta | c0 → c0 | none |
| `03-candidate` | candidate | c0 → c1 | a defect fixed in c2, its duplicate, a hardening item deferred by Spar, a requirement-question answered |
| `04-fixes` | fixes | c1 → c2 | a defect deferred to `#901` with the user's words, a note without a disposition |
| `05-candidate` | candidate | c0 → c2 | a defect rejected |
| `06-fixes` | fixes, dry run | c2 → c3 | none |

`extensions.jsonl` holds one code extension with the cause `churn`; the caps are plan 1,
plan-delta 1 and code 3. After recording, the test's folder became `@DIR@` and every recorded time
became `2026-01-01T00:00:00Z`. c0 to c3 are the commits `newSMWorld` makes (`smLegacyCommits`).

`TestSparStageMergeAC7_LegacySeriesJudged` in `internal/spar/stage_merge_test.go` copies the series
into its repository, with its folder in place of `@DIR@`, and checks today's judgments of
`spar gate` and `spar cap-report`, not the bytes those commands printed then.

Spar can no longer start `plan-delta` or `fixes` rounds, so these files are kept as they are and
are not regenerated.
