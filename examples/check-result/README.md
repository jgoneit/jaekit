# 참조 검사 예시 / Reference check example

실제 작업 기록이 아닌 합성 예시입니다. `sample/input.txt`의 내용이 `hello`와 줄바꿈인지 읽는 검사이며 애플리케이션 전체 동작을 검증하지 않습니다. Python 3 표준 라이브러리만 사용합니다. Python은 이 생산자를 선택할 때만 필요하며 Spec·Seal의 공통 필수 조건이 아닙니다.

This is a synthetic example, not a record of completed work. It reads whether `sample/input.txt` contains `hello` followed by a newline; it does not verify an entire application. It uses only the Python 3 standard library. Python is needed only if you choose this producer, not for Spec or Seal in general.

## 가져오기 / Copy the files

[한국어 설치 안내](../../guides/INSTALL.md#참조-검사-가져오기) 또는 [English installation guide](../../guides/INSTALL.en.md#get-the-reference-check)의 경로와 복사 명령을 따릅니다. 프로젝트에 이미 같은 파일이 있으면 덮어쓰지 말고 다른 경로를 정해 선언·설정·PLAN을 함께 맞춥니다.

Follow the paths and copy commands in the installation guide. If files already exist at those paths in your project, choose other paths and update the declaration, configuration and PLAN together instead of overwriting them.

| 배포 파일 / Packaged file | 프로젝트 경로 / Project path |
| --- | --- |
| `tools/check-result-reference.py` | `tools/check-result-reference.py` |
| `examples/check-result/declaration.json` | `checks/declaration.json` |
| `examples/check-result/reference.json` | `checks/reference.json` |
| `examples/check-result/input.txt` | `sample/input.txt` |

## 계획과 관측 / Plan and observations

아래는 준비된 목표의 PLAN에 넣는 두 표입니다. 나머지 PLAN 항목과 목표 문서는 별도로 필요합니다. 새 목표를 시작하기 전에 초기 `sample/input.txt`를 commit해 기준 상태로 보존합니다. 선언·설정·생산자는 검사 전에 commit하고 세 파일을 모두 검사 경로로 선언합니다. `sample/input.txt`는 관측 대상이므로 baseline에 덧씌우는 검사 경로에 넣지 않습니다.

These two tables belong in the PLAN for a prepared goal; its remaining sections and goal documents are still required. Commit the initial `sample/input.txt` before starting the new goal to preserve the baseline state. Commit the declaration, configuration and producer before checking and list all three as check paths. The observed `sample/input.txt` is not a check path: it must not be copied over the baseline.

```markdown
## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json` | checks/declaration.json, checks/reference.json, tools/check-result-reference.py | T001 |

## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | checks/declaration.json |
```

실제 명령은 `ha check <goal> --baseline AC-1`과 `ha check <goal> AC-1`로 실행합니다. 초기 `not ready`와 줄바꿈은 `missing-greeting` 위반입니다. 대상을 `hello`와 줄바꿈으로 바꾸고 commit한 뒤 현재 검사를 하면 통과합니다. 이 두 관측만으로 목표 전체의 완료를 주장하지 않으며 모든 필수 조건을 충족한 뒤 `ha done` 기록을 확인합니다.

Run the command through `ha check <goal> --baseline AC-1` and `ha check <goal> AC-1`. The initial `not ready` plus newline produces the `missing-greeting` violation. Change the target to `hello` plus newline, commit, then run the current check to observe a pass. These observations alone do not complete an entire goal; satisfy every required condition and confirm the `ha done` record.

`ha`가 호출별 환경 변수를 전달합니다. 수동으로 만든 보고를 완료 근거로 쓰지 않습니다. `missing: violation`은 이 예시에서 대상 파일 부재 자체가 요구 위반이라는 선택입니다. 실행 환경 의존성 누락은 `requires`로 구분하며, pytest·Vitest·Playwright를 자동 연동하거나 설치하지 않습니다. [결과 계약](../../contracts/check-result.md)을 참고하세요.

`ha` supplies the invocation-specific environment variables. Do not use a hand-made report as completion evidence. `missing: violation` explicitly treats a missing target file as the requirement violation in this example. Use `requires` to distinguish missing environment dependencies. This producer does not automatically adapt or install pytest, Vitest or Playwright. See the [result contract](../../contracts/check-result.md).
