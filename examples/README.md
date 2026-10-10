# 문서 형식 예시

[empty-input](empty-input/)은 가상의 `textstats` 프로젝트가 제공하는 빈 입력 요약 파일을 고치는 작업을 설명합니다. 실제 프로젝트, 실행 기록 또는 완료 사례가 아닙니다. 선언·설정은 이 저장소의 [참조 생산자](../tools/check-result-reference.py)에 연결되지만, 검사할 `src/textstats/` 파일은 가상이므로 예제만으로 완료를 실행할 수 없습니다. CLI 실행이나 요약 생성 알고리즘까지 검증하는 예시는 아닙니다.

- [SPEC.md](empty-input/SPEC.md): 원하는 결과와 유지할 동작
- [PLAN.md](empty-input/PLAN.md): 조건별 검사와 작업 범위
- [declaration.json](empty-input/declaration.json) · [reference.json](empty-input/reference.json): `/3` change 조건의 대상과 실제 파일 관측 설정
- [REVIEW.md](empty-input/REVIEW.md): 계획 검토 형식
- [PROGRESS.md](empty-input/PROGRESS.md): 진행 기록 형식

실제 목표는 작업할 프로젝트의 `docs/specs/<goal>/`에 작성합니다. 자세한 사용법은 [사용 안내](../guides/USAGE.md)에 있습니다.

가상 상태를 별도 저장소에 구성하면 기준 파일 `src/textstats/empty-output.txt`의 `empty`와 줄바꿈은 선언한 위반이고, 현재 파일의 `0 0 0`과 줄바꿈은 통과입니다. `src/textstats/sample-output.txt`는 두 상태 모두 `1 2 3`과 줄바꿈으로 유지합니다. 선언·설정·생산자는 검사 경로에 넣어 commit하고 변경 대상 요약 파일은 넣지 않습니다. 이 연결은 개발 Core의 `/3`용이며 현재 배포판 v0.1.2의 사용 예시가 아닙니다. 실제 프로젝트의 검사 도구와 검증 방법은 에이전트가 선택합니다.
