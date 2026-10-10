# PLAN — empty-input

가상의 문서 형식 예시입니다. 실제 실행 결과가 아닙니다([설명](../README.md)).

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python -m pytest tests/test_empty.py -q` | tests/test_empty.py | T001 |
| AC-2 | maintain | `python -m pytest -q` | — | T001 |

## 범위
- 바꿀 수 있는 경로: `src/textstats/**`, `tests/test_empty.py` (SPEC: 목표)
- 테스트 경로: (기본값)
- 유지할 동작: 비어 있지 않은 파일의 출력 형식 (SPEC: AC-2)

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | 빈 입력에서 0 0 0을 출력한다 | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측

## 읽을 자료
- 필수 맥락: `src/textstats/count.py`의 `count`, SPEC의 사용 시나리오
- 필요할 때 참고: 없음
- 판단 근거: 기존 출력 형식은 `tests/test_format.py`가 정한다
- 충돌·미확인: 없음
