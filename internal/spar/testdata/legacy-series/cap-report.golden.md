# Cap report: @DIR@/repo/reviews/legacy

- code rounds counted: 3 of 4 (cap 3, extensions 1)
- open: defect 0, requirement-question 0

## Rounds

| Round | Stage | Sequence | Counted | Base | Head | defect | requirement-question | hardening | note |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 03-candidate | candidate | 3 | yes | 6f153db7ffe18958da1d62a0844e1db0ffd42547 | 8a807f9e98214f58b30fc0199bfe4b57d7496ea8 | 2 | 1 | 1 | 0 |
| 04-fixes | fixes | 4 | yes | 8a807f9e98214f58b30fc0199bfe4b57d7496ea8 | 93cac67dc43b88547e3c13cb3f9d4dc3f373d3ff | 1 | 0 | 0 | 1 |
| 05-candidate | candidate | 5 | yes | 6f153db7ffe18958da1d62a0844e1db0ffd42547 | 93cac67dc43b88547e3c13cb3f9d4dc3f373d3ff | 1 | 0 | 0 | 0 |

- skipped 06-fixes: dry run

## Open findings

- none

## Signals

### Churn

- 04-fixes: 1 (host-F1)
- 05-candidate: 0

### Recurring links

- none

### Unrelated defects

- count: 1
- 04-fixes host-F1

### Check growth

- added lines: check 3, other 1; over hardening: yes

## Limits

- churn: Counted by file: a finding in a file the fixes changed counts even when it is about another line.
- recurring: Counted by link: the same problem reported under different links is not grouped.
- unrelated: Uses each reviewer's introduced_by_change as reported.
