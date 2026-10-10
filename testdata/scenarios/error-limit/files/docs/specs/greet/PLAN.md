# PLAN — greet

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `sh tests/greeting.sh` | tests/greeting.sh | T001 |
| AC-2 | maintain | `jaekit-test-runner tests/keep.sh` | — | T001 |

## 범위
- 바꿀 수 있는 경로: `src/**`, `tests/**` (SPEC: 범위와 비목표)
- 테스트 경로: (기본값)
- 유지할 동작: `src/keep.txt`

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | greeting says hello and keep stays | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측
