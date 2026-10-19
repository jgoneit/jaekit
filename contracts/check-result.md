# 계약: change 검사 결과

`run-rules/3`의 change 조건은 baseline과 현재 검사를 같은 선언에 연결한다. 이 계약은 일반 nonzero를 의도한 요구 위반으로 간주하지 않는다. `/1`·`/2`의 기존 실행과 일반 maintain·manual·task 검사의 역할은 그대로다. 새 시작은 `/3`을 사용한다([실행 기록](run-record.md)).

## 1. 선언과 입력

검사 작성자는 실행 전에 요구 대상과 예상 위반을 JSON 파일에 선언하고 PLAN의 `## 결과 계약` 표에서 연결한다.

```markdown
## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | checks/declaration.json |
```

선언 파일과 `producer_paths`의 모든 파일은 조건표의 `검사 경로`에 포함한다. 모두 저장소 상대 경로의 추적된 일반 파일이어야 하며 symlink를 사용하지 않는다. 선언과 생산자·설정 파일의 현재 바이트는 HEAD와 같아야 한다. 다른 실행 문서 제외 경로 안에 있더라도 미커밋 내용으로 분류 의미를 바꿀 수 없다. baseline은 이 파일들을 HEAD에서 기준 commit으로 덧씌운다. 임시 worktree의 의존성과 환경을 자동으로 설치하지 않는다.

아래 선언과 참조 설정은 가상 greeting 예시다. 실제 사용자 자료가 아니다.

### 선언 예시 (`checks/declaration.json`)

```json
{
  "schema": "check-declaration/v1",
  "producer": "jaekit-reference",
  "producer_version": "1",
  "producer_paths": ["tools/check-result-reference.py", "checks/reference.json"],
  "targets": [{"id": "greeting", "violation": "missing-greeting"}]
}
```

`producer`, `producer_version`, 대상 `id`·`violation`은 ASCII 영숫자로 시작하는 1~128자의 ASCII 영숫자·밑줄·점·콜론·슬래시·하이픈 식별자다. 생산자 경로는 1~128개, 대상은 1~1024개다. `targets`의 각 `id`는 고유하며 모든 대상이 필수다. `violation`은 해당 요구가 충족되지 않았을 때 관측할 식별자다. 선언·대상·결과 계약·생산자 버전과 내용은 근거의 정체성에 포함된다. 명령 문자열이 같아도 이 입력이 바뀌면 이전 근거와 완료를 그대로 재사용하지 않는다.

## 2. 호출별 보고

`ha`가 검사 프로세스에 다음 환경 변수를 전달한다.

| 변수 | 의미 |
| --- | --- |
| `HA_EVIDENCE_PATH` | 이번 호출의 구조화 보고를 쓸 로컬 파일 |
| `HA_EVIDENCE_INVOCATION` | 이번 호출의 식별자 |
| `HA_EVIDENCE_DECLARATION_DIGEST` | 이번 선언과 생산자 입력의 digest |

생산자는 보고에 호출 식별자와 digest를 그대로 연결한다. 이전 호출의 파일, 다른 선언·대상·생산자의 보고는 유효하지 않다. stdout/stderr는 기존 로컬 실행 로그로 남으며 보고 파일을 대신하지 않는다.

```json
{
  "schema": "check-result/v1",
  "invocation": "<HA_EVIDENCE_INVOCATION>",
  "declaration_digest": "<HA_EVIDENCE_DECLARATION_DIGEST>",
  "producer": "jaekit-reference",
  "producer_version": "1",
  "attempts_complete": true,
  "observations": [{"target": "greeting", "attempt": 1, "status": "violation", "violation": "missing-greeting"}]
}
```

| `status` | 관측과 추가 필드 |
| --- | --- |
| `pass` | 대상 실행과 통과. `violation`·`reason`을 넣지 않는다 |
| `violation` | 실제 assertion 실패. `violation`에 관측한 위반 식별자를 넣는다 |
| `error` | 검사를 방해한 오류. `reason`은 `dependency_missing`, `collection_error`, `import_error`, `compile_error`, `browser_start_error`, `setup_error`, `execution_error`, `permission_denied` 중 하나 |
| `skip` | 대상이 실행되지 않음. `reason`은 `skipped` 또는 `not_selected` |

참조 생산자는 내부 재시도 없이 대상마다 한 번 관측한다. 공통 계약에서 재시도를 보고하려면 `attempts_complete: true`와 대상별 1부터 빠짐없이 증가하는 전체 `attempt` 이력이 필요하다. 전체 관측은 최대 4096개다. 마지막 attempt만 남거나 같은 번호가 중복되거나 순서가 역전되면 `attempt_history_invalid`, 완전성 표시가 없으면 `attempt_history_missing`으로 판정 불가다. 같은 대상의 pass와 violation이 함께 있으면 최종 exit 0이어도 `fail`·`inconsistent_attempts`로 불리한 결과를 보존한다. 재시도 정보를 확보하지 못하는 연동은 미지원 또는 판정 불가다.

선언과 보고는 1 MiB 이하의 JSON 객체여야 하며 중복 키·알 수 없는 필드·뒤에 붙은 데이터·지원하지 않는 schema는 거부한다. 보고 파일도 일반 파일이어야 한다. 미지원 일반 스크립트는 선언 없이 실제 시도를 기록할 수 있지만 `/3` change의 양의 근거는 얻지 못한다.

## 3. 분류와 보존

| 단계 | 저장하는 `result` | 의미 |
| --- | --- | --- |
| 현재 검사 | `pass` | 정상 종료와 모든 대상 통과 |
| 현재 검사 | `fail` | 실제 assertion 실패 |
| baseline | `fail_as_expected` | 선언한 위반 관측과 비정상 종료, 다른 실패·오류·미실행 없음 |
| baseline | `unexpected_pass` | 기준 상태에서도 모든 대상 통과 |
| baseline | `fail` | 의도와 다른 실제 assertion 실패 또는 내부 시도의 pass·violation 불일치 |
| 둘 다 | `error` | 프로세스 시작·시간 초과·신호·출력 저장 오류 또는 보고된 환경 오류 |
| 둘 다 | `unknown` | 미지원·누락·충돌·미실행 등으로 분류 불가 |

간결한 근거의 `reason`은 분류 이유 코드다. 예를 들어 `unsupported_declaration`, `report_missing`, `report_binding`, `attempt_history_missing`, `attempt_history_invalid`, `target_missing`, `target_skipped`, `report_exit_conflict`는 판정 불가를 설명한다. `environment_reported`는 환경 오류이고 `unexpected_assertion`은 의도와 다른 실패다. 원문 메시지를 사유 코드에 넣지 않는다.

현재 통과는 정상 종료와 선언한 모든 대상의 실제 통과가 함께 필요하며, 전체 내부 시도 이력에도 불리한 관측이 없어야 한다. baseline 기대 실패는 비정상 종료, 선언한 대상 전체의 관측, 적어도 하나의 선언한 위반이 필요하다. 환경 오류·미실행·의도와 다른 assertion 실패가 섞이면 충분하지 않다. 예상 위반과 다른 실제 assertion 실패는 불리한 실패로 남는다.

프로세스 시작 실패·시간 초과·외부 신호 종료, import·컴파일·수집·브라우저 기동 실패는 그 자체로 기대 실패가 아니다. 반면 실행 가능한 검사가 기능의 부재를 직접 관측하고 선언한 위반으로 보고한 경우는 유효할 수 있다. 대상 0개, 모두 skip, 누락·손상·미지원 보고, 종료와 보고의 충돌도 성공이나 기대 실패로 승격하지 않는다.

환경 오류나 skip과 assertion 위반이 함께 있으면 실행 전체는 `error` 또는 `unknown`일 수 있다. 그래도 간결한 근거의 대상별 불리한 관측은 남는다. 같은 입력에서 뒤의 정상 실행으로 그 assertion 위반을 숨기지 않는다.

오류·판정 불가는 기존 오류 재시도 한도에 함께 포함한다. 같은 근거 입력에서 뒤의 통과가 앞선 불리한 assertion 결과·예상 밖 baseline 통과·초과한 오류 이력을 지우지 않는다. 검사 종류 이름을 바꿔 재시도 권한을 늘리지 않는다. 실제 검사 시도와 사용량은 결과 채택 여부와 무관하게 남고 baseline도 센다. 실행 전 입력·dirty 거부와 임시 worktree 준비 실패는 실행한 검사와 구분한다.

실제 시도 뒤 로그가 사라지거나 저장에 실패해도 기록 저장소를 쓸 수 있으면 `error`·`process_output`과 사용량을 남긴다. 이 경우 `output_path`·`output_digest`가 비어 있어 원문을 조회할 수 없을 수 있다. 기록 저장 자체가 실패했다면 성공으로 보고하지 않으며 쓰기 후 실패는 적용 여부가 불확실할 수 있다.

기록에는 `check-evidence/v1`의 간결한 근거를 저장한다: `result`, `reason`, `declaration_digest`, `report_digest`, `invocation`, `targets`. 대상 관측만 넣고 검사한 파일의 실제 값이나 원문 로그를 복사하지 않는다. 현재 상태는 이 근거로 재구성하므로 원문 로그 부재나 현재 시각 변화만으로 판정이 달라지지 않는다. 선언과 보고서는 실행자가 작성할 수 있다. 보증 수준은 계속 `local`이며 독립된 요구 이해나 제3자 검증을 보증하지 않는다.

저장소 개발용 goal·review·gap 생산자의 혼합 결과·실패 귀속과 버전 2 대상 구성은 [저장소 생산자 계약](repository-producers.md)에 별도로 정의한다. 이 계약의 보고 형식이나 일반 참조 생산자의 설치 계약을 바꾸지 않는다.

## 4. 실제 참조 연동

[참조 생산자](../tools/check-result-reference.py)는 Python 3 표준 라이브러리만 사용한다. 필요한 프로젝트에 복사해 추적하고 선언·설정과 함께 검사 경로에 넣는다. 다른 러너·의존성을 자동 설치하지 않는다. 참조 생산자에 pytest·Vitest·Playwright 연동이 들어 있다는 뜻이 아니다. 러너 출력을 직접 읽는 어댑터는 §5다.

v0.1.3부터 직접 설치와 Homebrew는 이 생산자와 [합성 예시](../examples/check-result/README.md)를 함께 제공한다. 개발 checkout 없이 [참조 검사 가져오기](../guides/INSTALL.md#참조-검사-가져오기)의 배포 경로에서 복사할 수 있다([English](../guides/INSTALL.en.md#get-the-reference-check)). Python은 이 생산자를 선택할 때만 필요하며 Spec·Seal의 공통 필수 의존성이 아니다.

### 참조 설정 예시 (`checks/reference.json`)

```json
{
  "checks": [{
    "target": "greeting",
    "path": "src/greeting.txt",
    "equals": "hello\n",
    "violation": "missing-greeting",
    "missing": "violation"
  }]
}
```

`missing: violation`은 이 검사가 대상 파일의 부재를 요구 위반으로 직접 관측한다는 선언이다. 생략하거나 `error`이면 파일 부재는 환경 오류다. 선택 `requires`에는 이 검사의 환경에 필요한 파일 경로 목록을 넣는다. 그 파일이 없으면 `dependency_missing`이며 요구 위반으로 바꾸지 않는다. 실제 파일 내용은 보고에 넣지 않는다.

조건표에서 다음 명령과 검사 경로를 사용한다. 환경 변수를 수동으로 만들어 단독 생산자의 보고를 완료 근거로 삼지 않는다.

```markdown
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json` | checks/declaration.json, checks/reference.json, tools/check-result-reference.py | T001 |
```

`src/greeting.txt`가 없거나 다른 값을 갖는 기준 상태에서는 선언한 위반을 관측한다. 현재 코드가 `hello`와 줄바꿈을 저장하면 같은 대상이 통과한다. 이 예시는 선언을 판정기에 주입하는 것만으로 끝나지 않고 별도 프로세스에서 실제 파일을 읽는다. 적용 프로젝트의 실제 동작과 검증 대상은 검사 작성자가 정해야 한다.

검사 입력의 CRLF/LF 변환으로 HEAD와 실제 바이트가 다르면 Git이 clean으로 보더라도 입력을 거부한다. 줄바꿈 차이를 진단하고 checkout 조건을 안내하되 파일·Git 설정·기존 digest를 자동 변경하거나 정규화한 바이트를 동일한 증거로 인정하지 않는다.

## 5. 러너 어댑터

`run-rules/3`·`run-rules/4`의 change 조건은 생산자 대신 러너 어댑터로 이미 있는 시험 하나에 대상을 연결할 수 있다. `ha capabilities`의 `formats.check_declaration`에 `check-adapter/v1`이, `features`에 `runner-adapters/v1`이 있는 실행 파일만 이를 지원한다. 어댑터는 선택이다. 생산자 선언(§1~§4)의 형식과 의미는 그대로이고, 지원이 없으면 참조 생산자나 전용 생산자를 쓴다. `/1`·`/2` 목표와 maintain·manual·task·`EX-n` 검사는 결과 계약에 어댑터 선언이 있어도 지금처럼 종료 코드로 판정하고 자리표시 낱말을 바꾸지 않는다.

### 선언

결과 계약 표와 검사 경로 규칙은 §1과 같다. 어댑터 선언에는 생산자 파일 목록이 없다. 아래는 가상 greeting 예시다.

```json
{
  "schema": "check-adapter/v1",
  "adapter": "go-test-json",
  "targets": [{"id": "greeting", "violation": "greeting-missing",
               "test": {"package": "example.com/greet/cmd/greet", "name": "TestGreetsWithHello"}}]
}
```

| `adapter` | 검증 명령 | `test` |
| --- | --- | --- |
| `go-test-json` | `go test -json …` 그대로. `ha`가 표준 출력을 읽는다 | `package`(import 경로)와 `name`(`go test -json`이 보고하는 전체 시험 이름, 하위 시험은 `TestX/case`) |
| `junit-xml` | JUnit XML을 쓰는 러너 명령. 출력 위치는 아래 표기로 받는다 | `classname`(빈 값 허용)과 `name` |

대상은 1~1024개이고 `id`·`violation`은 §1의 식별자다. `ha lint`는 다음을 `evidence_invalid`로 보인다: 모르는 어댑터, `test`가 없는 대상, 어댑터와 맞지 않는 이름 필드, 빈 이름, 제어 문자(C0·DEL·C1)를 담은 이름, 1024바이트를 넘는 이름, 공백을 담은 Go 패키지 경로, Go 시험 이름 형식(첫 `/` 앞이 `Test`·`Fuzz`·`Example`로 시작하고 접두 뒤 첫 글자가 소문자가 아닌 Go 식별자이고, 이름 어디에도 공백이 없음. 하위 시험 이름은 `/`를 담을 수 있어 `TestX/a//b`처럼 빈 마디가 있어도 된다)에 맞지 않는 이름, 같은 시험을 연결한 두 대상, `jaekit:`로 시작하는 `violation`(예약), `:`나 `.`로 끝나는 `violation`(표식에서 읽힐 수 없음). 생산자 선언(`check-declaration/v1`)의 `violation`은 지금처럼 `:`·`.`로 끝나도 된다. 알 수 없는 필드·중복 키·뒤에 붙은 데이터·1 MiB 초과는 §2처럼 거부한다.

근거의 정체성에는 선언 바이트와 어댑터의 판정 의미 버전이 함께 들어간다. 어댑터가 같은 출력을 다르게 판정하게 되면 그 버전이 오르고, 그 어댑터의 이전 근거는 다시 쓰이지 않는다([실행 기록](run-record.md)).

### 출력 위치

`junit-xml` 조건은 실행마다 Git 디렉터리 안 `ha/runs/<goal>/` 아래에 새 비공개 디렉터리를 만들고, 비어 있는 그 디렉터리의 `junit.xml`을 출력 위치로 정한다. 명령 인자 안의 `{HA_JUNIT_XML}`(인자의 일부여도 된다)를 그 경로로 바꾸고, 같은 경로를 환경 변수 `HA_JUNIT_XML`로도 넘긴다. 기록의 `argv`와 명령 digest는 바꾸기 전 명령이며, 경로는 기록에 남지 않는다. 지난 실행의 파일이나 작업 트리의 같은 이름 파일은 읽지 않는다. 예:

```markdown
| AC-2 | change | `node --test --test-reporter=junit --test-reporter-destination={HA_JUNIT_XML} test/greet.test.mjs` | test/greet.test.mjs, checks/AC-2.json | T001 |
```

`go-test-json` 조건은 명령의 표준 출력을 따로 받아 끝까지 읽는다. 명령이 끝난 뒤에도 하위 프로세스가 표준 출력을 열어 두면 5초까지만 더 읽고, 그때까지 끝나지 않으면 잘린 출력을 판정하지 않는다(아래 표의 `process_wait`). 이 읽기는 `ha`가 남은 하위 프로세스를 정리하기 전에 하므로 운영체제와 관계없이 같다. 로그에는 지금처럼 표준 출력과 표준 오류가 함께 남고 `ha log`가 보인다. 표준 오류(모듈 내려받기 안내, vet 경고, 빌드 오류 원문)는 판정에 쓰지 않는다.

### 표식

표식은 고정 문자열 `jaekit:violation=`와 그 뒤의 식별자다. 식별자는 바로 뒤에 이어진 식별자 문자(`A-Z a-z 0-9 _ . : / -`)의 가장 긴 연속에서 끝에 붙은 `:`·`.`를 모두 뗀 것이다. 그래서 `jaekit:violation=greeting-missing: got "hi"`와 `jaekit:violation=greeting-missing.`의 식별자는 `greeting-missing`이고, 가운데의 `:`·`.`(`a.b:c`)는 그대로 남는다. 대소문자를 구분하고, 뗀 뒤의 식별자가 비어 있거나 128자를 넘거나 영숫자로 시작하지 않으면 표식이 아니다. 표식은 실패 메시지의 첫 줄에서만 읽으며 첫 줄 안의 위치는 묻지 않는다.

- Go: 연결한 시험 자신의 출력 중 `OutputType`이 `error`인 줄 하나가 `Error`·`Fatal` 계열 호출 하나의 첫 줄이다. `error-continue` 줄, 종류 없는 줄(`t.Log`, 직접 쓴 표준 출력), `frame` 줄은 실패 메시지의 첫 줄이 아니다.
- JUnit: 연결한 testcase의 각 `failure` 요소에서 `message` 속성이 있으면(빈 값이어도) 그 첫 줄, 없으면 요소 본문의 글자 그대로의 첫 줄이다. `system-out`·`system-err`와 본문의 둘째 줄 이후(traceback, stack trace, 소스 발췌)는 읽지 않는다.

연결한 시험이 실패하고 모든 실패 메시지 첫 줄에 표식이 있으며 그 표식이 모두 같은 식별자일 때, 그 식별자가 관측한 위반이다. 실패 메시지가 없거나, 하나라도 표식이 없거나, 다른 식별자가 섞이면 표식 없는 실패이고 관측값은 `jaekit:unmarked-failure`다. 관측한 위반이 선언한 `violation`과 같을 때만 선언한 위반이다. 다른 식별자와 표식 없는 실패는 `unexpected_assertion`이다. 선언 위반은 `jaekit:`로 시작할 수 없으므로 표식 없는 실패는 어떤 선언 위반도 되지 않는다.

**한계.** 비교값·repr에 표식 문자열이 들어가 첫 줄에 찍히는 경우는 구분하지 않는다. 표식은 선언한 성질을 확인하는 assertion의 실패 메시지에만 쓰고, 비교값이나 출력 값에 넣지 않는다.

### 대상과 시도

- 대상 하나는 시험 하나에 연결한다. 이름은 정확히 일치시키며 정규식·접두 일치는 쓰지 않는다.
- Go는 패키지 import 경로와 전체 시험 이름으로 찾는다. 하위 시험에 연결하면 그 하위 시험을 관측하고, 상위 시험에 연결하면 상위 시험 자신의 실패 메시지만 본다. 하위 시험의 실패만으로 실패한 상위 시험은 실패 메시지가 없으므로 표식 없는 실패다. 한 명령에서 연결한 시험이 여러 번 실행되면(`-count`) 각 실행이 순서대로 시도 1, 2, …가 된다.
- JUnit은 testcase의 `classname`과 `name`을 정확히 일치시킨다. 선언의 `classname`이 빈 값이면 `classname` 속성이 없는 testcase와 `classname=""`인 testcase가 모두 맞고, 다른 `classname`을 가진 testcase는 맞지 않는다. 같은 `name`의 두 testcase가 이렇게 함께 맞으면 같은 testcase의 중복이다. `file` 속성은 쓰지 않는다. JUnit XML은 시도 이력을 갖지 않으므로, 연결한 testcase가 둘 이상이거나 재시도 요소(`rerunFailure`, `rerunError`, `flakyFailure`, `flakyError`)가 있으면 판정 불가다.
- 연결하지 않은 시험의 결과는 대상 관측이 아니다. 현재 검사에서 연결한 시험이 모두 통과했는데 명령이 실패하면 `report_exit_conflict`다. baseline에서는 연결하지 않은 시험의 실패가 대상 판정을 바꾸지 않는다.

관측은 §3의 규칙 그대로 분류하고 `check-evidence/v1` 요약으로 저장한다. `report_digest`는 읽은 러너 출력의 digest다. 러너의 실패 메시지·시험 출력·XML 내용은 기록과 사유 코드에 넣지 않는다.

### 입력 실패의 결과

다음 표가 완전한 목록이다. 목록 밖의 출력 형태(연결한 시험의 패키지 사건에 어댑터가 모르는 Go 동작(Go 1.27 `go test -json`이 쓰는 `start`·`run`·`pause`·`cont`·`pass`·`fail`·`skip`·`output`·`bench`·`build-output`·`build-fail`·`attr`·`artifacts` 밖)이나 `OutputType` 값(`error`, `error-continue`, `frame` 밖)이 나오거나 `Output`이 없는 `output` 사건이 나옴, 연결한 testcase의 지원하지 않는 자식 요소(네임스페이스가 붙은 요소 포함)나 `skipped`와 `failure`·`error`의 조합, `failure` 안의 요소)는 `observation_invalid`다. 어느 경우도 통과, 기대 실패, 선언한 위반이 아니다.

| 경우 | 결과(현재·baseline 같음) | 사유 |
| --- | --- | --- |
| 러너가 시작하지 못함(PATH에 없음, 실행 권한 없음) | `error` | `process_start` |
| `ha`의 시간 제한에 걸림 | `error` | `process_timeout` |
| 신호로 종료됨 | `error` | `process_signal` |
| 명령 출력 저장 실패 | `error` | `process_output` |
| `go-test-json` 명령이 끝난 뒤 하위 프로세스가 표준 출력을 5초 넘게 열어 둠. 종료 코드와 그때까지 읽은 출력은 판정에 쓰지 않는다. 명령이 신호로 끝났으면 위의 신호 행이 앞선다 | `error` | `process_wait` |
| 연결한 Go 시험의 패키지가 빌드되지 않음 | `error` | `environment_reported`, 대상 `compile_error` |
| 연결한 Go 시험이 시작했지만 끝 기록(통과·실패·skip)이 없음(시험 바이너리의 비정상 종료, Go 자체의 시험 시간 초과) | `error` | `environment_reported`, 대상 `execution_error` |
| 연결한 testcase에 `error` 요소가 있음. `failure`가 함께 있어도 같다 | `error` | `environment_reported`, 대상 `execution_error` |
| 연결한 시험이 출력에 없음(이름 불일치, 선택되지 않음, 수집 실패로 다른 이름만 있음) | `unknown` | `target_missing` |
| 연결한 시험이 skip됨(Go skip, JUnit `skipped`) | `unknown` | `target_skipped` |
| 러너 출력이 없음. Go: 표준 출력에 사건이 없음. JUnit: 알린 위치에 파일이 없거나 일반 파일이 아님(디렉터리, symlink) | `unknown` | `output_missing` |
| 러너 출력이 손상됨. 해석할 수 없는 JSON 줄, 빈 줄, 잘린 JSON, UTF-8이 아닌 바이트, XML 문법 오류·잘림, UTF-8이 아닌 인코딩 선언, DTD·엔티티 선언, 루트가 둘이거나 `testsuites`·`testsuite`가 아님, 중복 속성 | `unknown` | `output_corrupt` |
| 러너 출력이 크기 상한을 넘음(`go test -json` 표준 출력 64 MiB, JUnit XML 16 MiB). 잘라 읽은 앞부분으로 판정하지 않는다 | `unknown` | `output_too_large` |
| JUnit의 같은 testcase 중복, 재시도 요소 | `unknown` | `attempt_history_missing` |
| 종료 코드와 관측이 어긋남(모든 대상 통과인데 nonzero, 위반이나 오류인데 0) | `unknown` | `report_exit_conflict` |

종료 코드 충돌은 지금 규칙처럼 환경 오류·skip보다 먼저 적용된다. 예를 들어 `error` 요소가 있는데 러너가 0으로 끝나면 `unknown`(`report_exit_conflict`)이고, 대상 관측은 남는다. 여러 대상에 서로 다른 관측 전 사유가 겹치면 `attempt_history_missing`, `observation_invalid`, `target_missing` 순서로 하나를 고른다.

### 지원하는 출력

- `go test -json`이 실패 메시지 줄에 출력 종류(`OutputType`)를 붙이는 것은 Go 1.27부터다. 연결한 시험의 패키지 사건에 `OutputType`이 하나도 없으면(Go 1.26 이전 도구 체인) 그 패키지에서 실패한 연결 시험은 표식 없는 실패이고, `ha check`가 결과 줄 뒤에 그 패키지를 말하는 `notice: <조건> go-test-json: …` 한 줄(`OutputType`을 언급)을 낸다.
- JUnit XML은 UTF-8(BOM 허용)이고 루트 요소 하나(`testsuites` 또는 `testsuite`)를 가지며 testcase는 어느 깊이에 있어도 된다. 네임스페이스가 붙은 요소(`ext:failure`, `ext:testcase` 등)는 JUnit 요소가 아니다. testcase로 맞지 않고, 연결한 testcase 안에서는 지원하지 않는 자식이다. 실제 출력으로 한 번 확인한 형태는 Node.js 26.11.0 내장 러너(`node --test --test-reporter=junit`), pytest 9.1.1(`--junitxml=…`), Vitest 5.0.5(`--reporter=junit --outputFile=…`), Jest 30.5.2와 jest-junit 17.0.0(`JEST_JUNIT_OUTPUT_FILE`)이다. 그 출력에서 통과, 표식 있는 실패, 표식 없는 실패, skip, pytest fixture 오류(`error`)가 대표 형태 표대로 판정됐다. 기본 설정에서 pytest는 `classname`에 모듈 경로(`tests.test_greet`)를, Vitest는 `classname`에 파일 경로와 `name`에 `describe > test`를, jest-junit은 `classname`과 `name` 둘 다에 `describe test`를 쓴다. pytest와 Vitest의 실패는 `message` 속성의 첫 줄로, jest-junit의 실패는 `message` 속성이 없어 본문 첫 줄(`Error: …`)로 읽는다. 저장소의 검사는 이 러너를 설치하지 않고 같은 모양의 합성 문서로 확인한다. 다른 버전과 사용자 설정의 출력은 확인하지 않았다.

### 대표 형태

열린 입력의 대표 예다. 규칙은 목록에 없는 형태에도 같은 성질로 적용된다.

| ID | 어댑터 | 형태 | 기대 |
| --- | --- | --- | --- |
| G1 | go | 연결한 시험이 통과하고 `t.Log`에 표식이 있음 | 대상 통과 |
| G2 | go | `t.Fatalf("jaekit:violation=<선언 id> …")`로 실패 | 선언한 위반 |
| G3 | go | `t.Log`에 표식, 이어서 표식 없는 `t.Fatal`로 실패 | `unexpected_assertion` |
| G4 | go | `t.Error` 두 번, 하나만 표식 | `unexpected_assertion` |
| G5 | go | 여러 줄 오류의 둘째 줄에만 표식 | `unexpected_assertion` |
| G6 | go | 시험 안에서 panic | `unexpected_assertion` |
| G7 | go | 하위 시험이 표식과 함께 실패, 상위 시험에 연결 | `unexpected_assertion` |
| G8 | go | 하위 시험에 연결, 그 하위 시험이 표식과 함께 실패 | 선언한 위반 |
| G9 | go | 덧씌운 시험 파일이 기준 커밋에 없는 함수를 불러 빌드 실패 | `error`, `compile_error` |
| G10 | go | 연결한 시험이 실행되는 중에 시험 바이너리가 끝남(`os.Exit`, 충돌) | `error`, `execution_error` |
| G11 | go | `-count=2`에서 통과 한 번, 선언 위반 한 번 | `fail`, `inconsistent_attempts` |
| G12 | go | 두 패키지에 같은 시험 이름, 한 패키지에만 연결 | 연결한 패키지의 결과만 |
| G13 | go | 표준 출력에 JSON이 아닌 줄이나 잘린 JSON | `unknown`, `output_corrupt` |
| G14 | go | 표준 출력에 아무것도 없고 표준 오류에만 글이 있음 | `unknown`, `output_missing` |
| G15 | go | 출력 종류를 표시하지 않는 도구 체인에서 표식과 함께 실패 | `unexpected_assertion`, `notice:` 한 줄 |
| G16 | go | `t.Fatalf("jaekit:violation=<선언 id>: got %q", …)`처럼 표식 바로 뒤에 `:`가 붙거나, 메시지가 `jaekit:violation=<선언 id>.`로 끝남 | 선언한 위반 |
| J1 | junit | 자식 없는 testcase | 대상 통과 |
| J2 | junit | pytest `assert …, "jaekit:violation=<선언 id>"` 실패 | 선언한 위반 |
| J3 | junit | 메시지 없는 pytest assert 실패. traceback 본문에 표식이 든 다른 소스 줄이 보임 | `unexpected_assertion` |
| J4 | junit | Vitest `expect(값, "jaekit:violation=<선언 id>")` 실패. 메시지 첫 줄은 `jaekit:violation=<선언 id>: expected …` | 선언한 위반 |
| J5 | junit | `message` 속성 없이 본문 첫 줄이 `Error: jaekit:violation=<선언 id>`인 failure | 선언한 위반 |
| J6 | junit | fixture 준비 오류가 `error` 요소로 나옴 | `error`, `execution_error` |
| J7 | junit | `skipped` 요소 | `unknown`, `target_skipped` |
| J8 | junit | 수집된 시험 없음(빈 testsuite) | `unknown`, `target_missing` |
| J9 | junit | 같은 testcase 두 개 | `unknown`, `attempt_history_missing` |
| J10 | junit | `system-out`에만 표식이 있는 failure | `unexpected_assertion` |
| J11 | junit | 알린 위치에 파일이 없음, 파일이 잘림, DTD 선언이 있음 | `unknown` (`output_missing`, `output_corrupt`) |
| J12 | 둘 다 | 같은 출력의 줄 끝이 CRLF | LF와 같은 결과 |
| J13 | junit | 선언의 `classname`이 빈 값이고, testcase가 `classname=""`이거나 `classname`이 없음 | 그 testcase의 결과 |
| J14 | junit | 선언의 `classname`이 빈 값이고, 같은 `name`의 testcase가 `classname` 없는 것과 `classname=""`인 것 둘 | `unknown`, `attempt_history_missing` |
