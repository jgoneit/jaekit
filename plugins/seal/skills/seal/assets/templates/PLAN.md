# PLAN — <goal>

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `<command that reports the declared AC-1 observations>` | <declaration JSON>, <producer and configuration files>, <other check files> | T001 |
| AC-2 | maintain | `<command that checks AC-2>` | — | T001 |

## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | <repository-relative declaration JSON> |

For `/3` change conditions, declare the required target IDs, intended violation IDs and producer inputs using `check-declaration/v1`. Include this JSON and every `producer_paths` entry in `검사 경로`; commit these regular files before checking. The command must write the invocation-bound `check-result/v1` report. Choose a producer that observes this goal's actual behavior; a command's exit code alone does not establish the result. See the skill's `references/ha.md` and `references/bundle.md`. Omit this section for existing `/1` or `/2` goals; maintain, manual and task checks keep their roles.

## 범위
- 바꿀 수 있는 경로: `<glob>`, `<glob>` (SPEC: <section>)
- 테스트 경로: (기본값)
- 유지할 동작: <behavior to keep, from the SPEC>
- <only when a check reads other goals' PLAN.md, REVIEW.md, PROGRESS.md, runs.jsonl, or tasks/: 다른 목표 실행 묶음: `검사 입력` (reason); otherwise leave this line out>

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | <behavior this task completes> | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측

## 읽을 자료
- 필수 맥락: <document section or code location>
- 필요할 때 참고: <longer background>
- 판단 근거: <existing behavior or document a decision rests on>
- 충돌·미확인: <none, or what disagrees or is unconfirmed>
