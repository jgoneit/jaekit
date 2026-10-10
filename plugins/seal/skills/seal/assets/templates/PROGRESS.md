# PROGRESS — <goal>

## 현재
<a short, current summary (about 5 to 8 lines): check time with its time zone; the last ha status or ha done result with its record head as ha status prints it, seq and hash (`none` before `ha start`), and the completion record seq when there is one; the work in progress; open problems; the next work; how to resume. Restamp the check time and record head whenever it changes; whoever takes the goal up compares the record head with `ha status` (the same seq with another hash is another record), and the check time is for reference only. Past events stay out; point to `## 타임라인`. At completion: as of that completion record, and where to find what happens past the bundle's last commit (merge, review replies, checks after the merge), never as work in progress>

## Task 상태
| Task | 상태 | 메모 |
| --- | --- | --- |
| T001 | todo | |

## 타임라인
<optional entries, appended in time order; entries are claims; the rules are in the Seal bundle formats (references/bundle.md, `## 타임라인`):
- heading: ### <time with its time zone> · seq <record head> — <kind> · <one-line summary>; never invent a time you do not know; `seq none` while there is no record, and an entry about a check run uses that run's seq; kinds `관측`, `원인`, `결정`, `조치`, `검증`, `막힘`, `재개`
- time source: distinguish event time and writing time; unknown event time stays unknown and a measured writing time may head the entry; remaining conditions, new facts, hypothesis evidence and uncertainty to reduce belong only where relevant, not every tool call
- marks: causes `가설`, `확정`, `기각`; actions `예정`, `반영`; checks `대기`, `통과`, `실패`
- point to evidence by AC, commit, and seq; never copy check output
- append only and never edit an earlier entry; when a judgment changes, a new entry says what was rejected and on what grounds; a small event that went from cause to verification at once may be one entry
- record events that change a judgment or the next action: a new problem, a cause confirmed or changed, a change of approach, an important verification result, blocks and resumes (every resume counts: a new session taking the goal up, a cleared block, an answer to a `needs_user` question; a `재개` entry notes where `## 현재` differed from the actual state, if it did)
- leave out routine edits and reads, repeated runs of the same failure, and progress reports with no change
- an entry runs about 3 to 6 lines and `## 현재` about 5 to 8 lines; this is editing guidance, not a limit, and a complex cause analysis may run longer>

## 계획 변경
| 시각 | 변경 | 이유 | 영향 | 다시 검토한 것 |
| --- | --- | --- | --- | --- |

<rows: `시각` carries a time zone; one row is one event, kind changes included; when the event also has a timeline entry, the detail lives only in that entry and the row points to it by the entry's time, with a one-line summary>

## 막힘
| 시각 | 원인 종류 | 내용 | 해소 조건 |
| --- | --- | --- | --- |

<rows: `시각` carries a time zone; causes `auth`, `permission`, `quota`, `environment`, `spec`, `other`; one row is one event; when the event also has a timeline entry, the detail lives only in that entry and the row points to it by the entry's time, with a one-line summary>

## 개선 메모

## 완료 보고
<for the reporting form introduced with seal 0.1.10: connect the planned review to observed report; expected grounds, actual boundaries, methods and substitutes, results, evidence and unobserved scope; identify each changed condition, check, or environment and its evidence limits; shared condition groups may reference one explanation; distinguish recorded facts from interpretation; retain local assurance and executor-authored checks; show confirmed, unconfirmed and risky boundaries; this prose does not establish deployment, installation or actual model calls; paste the actual recorder result unchanged when one exists, never invent an output for a synthetic example>
