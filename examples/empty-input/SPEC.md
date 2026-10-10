# 빈 입력 처리

가상의 문서 형식 예시입니다. 실제 실행 결과가 아닙니다([설명](../README.md)).

Status: Ready

## 목표

빈 파일을 넘기면 `textstats count`가 오류 대신 0을 출력하게 한다. 지금은 빈 파일에서 오류로 끝난다(`src/textstats/count.py`의 `count`).

## 사용 시나리오

사용자가 빈 파일로 `textstats count`를 실행하면 줄·단어·글자 수가 모두 0으로 나온다. 비어 있지 않은 파일의 출력은 지금과 같다.

## Acceptance Criteria
- **AC-1** 빈 파일이면 `textstats count`가 `0 0 0`을 출력하고 0으로 끝난다.
- **AC-2** 비어 있지 않은 파일의 출력 형식은 바뀌지 않는다.

## Open Decisions

없음

## 관련 맥락

- `src/textstats/count.py`: 세는 함수
- `tests/`: 기존 출력 형식 시험
