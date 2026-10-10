# PLAN — empty-input

가상의 문서 형식 예시입니다. 실제 실행 결과가 아닙니다([설명](../README.md)).

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 tools/check-result-reference.py examples/empty-input/declaration.json examples/empty-input/reference.json` | examples/empty-input/declaration.json, examples/empty-input/reference.json, tools/check-result-reference.py | T001 |
| AC-2 | maintain | `python3 -c "assert __import__('pathlib').Path('src/textstats/sample-output.txt').read_text().strip() == '1 2 3'"` | — | T001 |

## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | examples/empty-input/declaration.json |

이 예시는 `/3`의 선언·생산자·설정 연결을 보여 준다. 선언과 `producer_paths`에 든 파일은 검사 경로에 함께 넣고 commit한다. 변경할 요약 파일은 검사 경로에 넣지 않아 baseline에서 기준 상태를 읽는다. 참조 생산자는 요약 파일을 실제로 읽지만 CLI 실행이나 생성 알고리즘까지 검증하지 않는다. 실제 목표에서는 그 목표를 관측하는 검사 방법을 에이전트가 선택한다.

## 범위
- 바꿀 수 있는 경로: `src/textstats/empty-output.txt` (SPEC: 목표)
- 테스트 경로: (기본값)
- 유지할 동작: 기존 샘플 요약 값 (SPEC: AC-2)

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | 빈 입력 요약 파일에 0 0 0을 저장한다 | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측

## 읽을 자료
- 필수 맥락: SPEC의 사용 시나리오와 `src/textstats/`의 요약 파일
- 필요할 때 참고: 없음
- 판단 근거: 기존 샘플 요약 값은 SPEC의 AC-2가 정한다
- 충돌·미확인: 없음
