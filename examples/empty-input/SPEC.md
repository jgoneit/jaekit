# 빈 입력 요약 파일

가상의 문서 형식 예시입니다. 실제 실행 결과가 아닙니다([설명](../README.md)).

Status: Ready
Criteria-Format: nested/1

## 목표

가상의 `textstats` 프로젝트가 제공하는 빈 입력 요약 파일 `src/textstats/empty-output.txt`에 `0 0 0`과 줄바꿈 하나가 저장되게 한다. 지금은 이 파일에 `empty`와 줄바꿈이 저장돼 있다. 이 예시의 검증 범위는 저장된 요약 파일이며 CLI 실행이나 요약 생성 알고리즘은 포함하지 않는다.

## 사용 시나리오

사용자가 빈 입력 요약 파일을 열면 줄·단어·글자 수가 모두 0인 것을 읽는다(AC-1). 기존 샘플 요약 파일의 값은 유지된다(AC-2).

## Acceptance Criteria
- **AC-1** `src/textstats/empty-output.txt`의 내용은 `0 0 0`과 줄바꿈 하나다.
- **AC-2** `src/textstats/sample-output.txt`의 기존 요약 값 `1 2 3`은 바뀌지 않는다.

## Open Decisions

없음

## 관련 맥락

- `src/textstats/`: 가상의 프로젝트가 제공하는 요약 파일
