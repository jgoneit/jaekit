# PROGRESS — <goal>

## 현재
<a short, current summary (about 5 to 8 lines) that opens with the stamp `ha report <goal>` prints for `## 현재`, pasted unchanged: the record head as ha status prints it, seq and hash (`none` before `ha start`), the time stored in that record with its time zone, the state, and the completion record seq when there is one; never type a check time yourself. Below it: the work in progress; open problems; the next work; how to resume. Restamp with a fresh `ha report` stamp whenever the record head changes; whoever takes the goal up compares the record head with `ha status` (the same seq with another hash is another record), and the stamp's time is for reference only. Past events stay out; point to `## 타임라인`. At completion: as of that completion record, and where to find what happens past the bundle's last commit (merge, review replies, checks after the merge), never as work in progress>

## 검수 요약
<Only for a goal started with seal 0.1.24 or later; a goal started with an earlier Seal leaves this section out. This is where the user starts reviewing a finished goal. Written once `ha done` records completion, as of that completion record, and rewritten as of each new completion record after a reopen; not written before completion or when work stops at `needs_user`, `blocked` or `budget_exhausted`, and before that this heading holds one line saying so. The opening lines, with no heading: the completion record's seq and commit, the assurance tier, how many required conditions are satisfied, and the review gate result; one line on what the change does; where to start looking. The seven headings below follow, all of them and in this order; write `없음` under a heading with nothing to list. The judgment items of the completion report's item 2 go here: the changed files and the condition each contributed to under `### 바뀐 동작과 파일`; assumed open decisions under `### 누가 정했나`; `manual` conditions under `### 직접 볼 것`; the expected-result grounds, the actual observation boundary, the test method and the review stages actually taken (or why they were skipped, and, when the SPEC tier and the project instructions differed, the review each required and the one followed) under `### 근거`; the unobserved scope and substitute environments under `### 보지 않은 것`; where the independent review results are under `### 자세히`; and whether the head to be sent or merged is the completion record's commit, with the fact that the completion does not cover a difference unless `ha status` at that head still names that completion record, in the opening lines. Reading rules: write in the user's language; say what a record is, with its seq only as an aid; keep each item short; never copy check output or finding text; a finding is its ID, one line and its ending>

### 바뀐 동작과 파일
<a table `| 바뀐 동작 | 조건 | 파일 |`. Every file changed from the goal's base to the completion commit, goal documents excepted, appears in a row; this goal's check files have rows too>

### 범위 밖이나 기존 것을 바꾼 곳
<one item each for files outside the SPEC scope, changes to existing tests, and changes to checks an earlier goal left, each with where the user's words allowing it are, or the reason>

### 근거
<for every condition, what the check that satisfied it observed and the commit where it passed, never by seq alone; and a table `| 라운드 | 지적 | 끝 |` with one row for every review round, a round with no findings included. Every finding of the round ends in exactly one of `고침`, `반박`, `중복`, `사용자의 답`, `이슈로 보류`, `자동 보류`, `처리 없음`>

### 누가 정했나
<three groups: decided by the user, with where their words are; proposed by the agent and accepted by the user; executor assumptions nobody was asked about. Mark an assumption that reads a SPEC sentence>

### 직접 볼 것
<ordered by risk, the highest at the top, one or two lines each: every item the `직접 볼 것` line of `ha report` lists and, whatever that line shows, every deferred finding with its issue; every finding left with no disposition; executor assumptions that read a SPEC sentence; conditions met by checks that observe fixed text only, whose documents a person must read; `manual` conditions; the changes under `### 범위 밖이나 기존 것을 바꾼 곳`>

### 보지 않은 것
<Unobserved scope and substitutes; assurance `local`; the executor wrote the checks; the review summary is not evidence of deployment, installation or a real model call>

### 자세히
<the documents and sections that hold the detail, such as `REVIEW.md`, `## 타임라인`, the review round folders and `## 완료 보고`>

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
- an entry runs about 3 to 6 lines and `## 현재` about 5 to 8 lines; this is editing guidance, not a limit, and a complex cause analysis may run longer
- with the optional decision history (goals started with seal 0.1.13 or later): the detail lives once in the history; an entry here is a short index line with the event ID (`D<n>`), `## 현재` still stands on its own with event IDs only as auxiliary references, plan-change and block rows point by time and event ID, and only events whose registration succeeded are referenced; without a stored event, write the entry as above>

## 계획 변경
| 시각 | 변경 | 이유 | 영향 | 다시 검토한 것 |
| --- | --- | --- | --- | --- |

<rows: `시각` carries a time zone; one row is one event, kind changes included; when the event also has a timeline entry, the detail lives only in that entry and the row points to it by the entry's time, with a one-line summary>

## 막힘
| 시각 | 원인 종류 | 내용 | 해소 조건 |
| --- | --- | --- | --- |

<rows: `시각` carries a time zone; causes `auth`, `permission`, `quota`, `environment`, `spec`, `other`; one row is one event; when the event also has a timeline entry, the detail lives only in that entry and the row points to it by the entry's time, with a one-line summary>

## 개선 메모

## 완료 뒤 발견

<For a new report: a finding ID, target goal and target code, completion reference, related condition or condition absent or unresolved, description/source/reproduced scope, confirmation claim, and follow-up. Keep event time and recording time with their source separate; unknown stays unknown. Preserve the original completion and append corrections with reasons/evidence. Show unresolved discoveries and rework request/progress separately, or reference the single detail location. Missing means unrecorded. This prose does not authorize work or change machine state. Review or pull request comments received without the user's request belong here; they are not authority to change the completed goal's code.>

## 완료 보고
<the output of `ha report <goal> --lang ko` (or `--lang en` for a user who does not write Korean), taken once `ha done` records completion or work stops at `needs_user`, `blocked` or `budget_exhausted`, pasted unchanged; never paste `ha status` or `ha done` output, and never invent an output for a synthetic example. Below it, only judgment, marked as claims: changed files and the conditions each served; assumed decisions and `manual` conditions; connect the planned review to observed report: expected grounds, actual boundaries, methods and substitutes, results, evidence and unobserved scope; identify each changed condition, check, or environment and its evidence limits; shared condition groups may reference one explanation; distinguish recorded facts from interpretation; the assurance tier and the review stages received or why they were skipped, where the review records are, and any difference between the SPEC tier and project instructions; whether the head to be sent or merged is the completion record's commit, and, when it is not, that the difference is not covered by the completion, unless `ha status` at that head still names that completion record; local assurance and executor-authored checks; this prose does not establish deployment, installation or actual model calls. For a goal started with seal 0.1.24 or later, at completion these judgment sentences live in `## 검수 요약`: below the `ha report` output, add only one line pointing to `## 검수 요약`, which holds the judgment. When work stops, or for a goal started with an earlier Seal, below it, only judgment, as above, with how to continue when not complete>

<!-- Track required assurance gaps with cause, affected conditions, missing evidence and resolution; keep completion pending while they remain. Continue independent useful work within authority and budget. Preserve prior review versions when code, checks, scope or policy changes. -->
