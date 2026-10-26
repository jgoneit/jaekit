# 계약: Spar 설정

`spar`가 어떤 리뷰어를 띄우고 어떤 상한·각도·규칙·검사 경로를 쓸지는 두 설정 파일이 정한다. 사용자 설정은 리뷰어를 정하고, 저장소의 프로젝트 설정은 리뷰어를 정할 수 없다. 형식은 [TOML 1.0.0](https://toml.io/en/v1.0.0)이고 `spar`는 `github.com/pelletier/go-toml/v2` v2.2.4로 읽는다(G17). 각 값의 받는 형식과 형식 밖의 결과는 [입력 형식](spar-input.md)의 표가 정하고, 이 문서는 그 행(G16~G25)을 가리킨다. 아래 예시의 이름·경로·모델은 합성이다.

## 1. 사용자 설정과 출처 순서

리뷰어 목록은 처음 찾은 출처 하나에서만 온다.

1. 환경 변수 `SPAR_CONFIG`가 가리킨 파일
2. `$XDG_CONFIG_HOME/jaekit/spar.toml`
3. `~/.config/jaekit/spar.toml`
4. 위 셋이 모두 없으면 clean-review가 읽는 `config.json` 하나의 `reviewers` 목록. 그 파일은 clean-review와 같은 방식으로 정한다: 환경 변수 `CLEAN_REVIEW_CONFIG`가 있으면 그 파일, 없으면 `$XDG_CONFIG_HOME/clean-review/config.json`, `XDG_CONFIG_HOME`도 없으면 `~/.config/clean-review/config.json`. 정한 파일이 없으면 다른 경로를 찾지 않고 다음 출처로 넘어간다. 각 이름은 같은 이름의 preset이다(`claude`, `codex`, `agy`, `native`).
5. 그것도 없으면 `claude`, `codex`, `agy`

- 출처 파일(G16): 가리킨 경로가 없으면(깨진 심볼릭 링크 포함) 다음 출처로 넘어가고, 건너뛴 경로를 `source.skipped`에 남긴다. 있는 경로가 일반 파일이 아니거나(폴더, FIFO 등) 파일이 잘못되었으면 넘어가지 않고 설정 오류로 끝난다. clean-review `config.json`(G18)은 JSON 객체이고 `reviewers`는 이름 문자열의 배열이다(`null`도 잘못된 값이다).
- 출처 파일에 `reviewers` 키가 없으면 기본 목록, 빈 목록이면 `native` 하나다. 그 파일의 최상위 `timeout_seconds`는 이 경우에도 모든 리뷰어의 기본값이다(G19).
- `spar run`과 `spar config`의 `--reviewers a,b`(G25)는 설정보다 앞선다. 설정에 있는 이름은 그 설정을, 없는 이름은 같은 이름의 preset을 쓴다. 빈 값(`--reviewers ""`)과 빈 항목만 있는 값은 `native` 하나다. 적용 기록의 `override`에 `{"kind": "--reviewers", "names": [...]}`로 남는다.

### 형식

```toml
timeout_seconds = 1800          # 모든 리뷰어의 기본 시간 제한(초)

[[reviewers]]
name = "codex"                  # 기록과 --reviewers에서 쓰는 이름, 생략하면 preset 이름
preset = "codex"

[[reviewers]]
name = "agy"
preset = "agy"
model = "example-model-pro"
fallback_model = "example-model-flash"

[[reviewers]]
name = "example-reviewer"
preset = "command"
argv = ["/opt/example/bin/example-reviewer", "--prompt", "{prompt}", "--workspace", "{workspace}", "--schema", "{schema}"]
timeout_seconds = 600
```

| 키 | 뜻 |
| --- | --- |
| `name` | 기록과 `--reviewers`에서 쓰는 이름(G20). 생략하면 preset 이름. 한 파일에서 대소문자를 무시하고 겹치면 오류. 라운드 폴더에 spar가 쓰는 이름과 그 `.tmp` 이름은 쓸 수 없다 |
| `preset` | `claude`, `codex`, `agy`, `native`, `command` 중 하나. 필수(G21) |
| `argv` | `command`에만 쓰고 `command`에는 필수. 셸 없이 그대로 실행하는 인자 배열이며 `{prompt}`·`{workspace}`·`{schema}`를 패킷 파일·작업공간·결과 schema 경로로 바꾼다 |
| `binary` | `claude`·`codex`·`agy`의 실행 파일. 없으면 PATH에서 preset 이름을 찾는다 |
| `model` | 요청할 모델. 적으면 에이전트가 고를 수 없다 |
| `fallback_model` | 지정 모델이 사용량·요청 한도로 거절될 때 같은 라운드에서 한 번 쓰는 모델. 빈 문자열이면 대체 모델이 없다 |
| `effort` | `claude`·`codex`의 추론 강도. 적으면 에이전트가 고를 수 없다 |
| `allowed_models`, `allowed_efforts` | 에이전트가 고를 수 있는 모델·강도의 목록(아래 "모델과 강도를 정하는 순서") |
| `timeout_seconds` | 양의 정수(G19). 리뷰어별 값이 최상위 값보다 앞서고, 최상위 값은 그 파일로 정해지는 모든 리뷰어의 기본값이다. 실행 파일의 버전 확인도 이 시간 안에서 한다 |

preset 기본값:

| preset | 격리 등급 | 필수 | 기본 모델 | 기본 대체 모델 | 기본 강도 |
| --- | --- | --- | --- | --- | --- |
| `claude` | `verified` | 예 | `opus` | 없음 | `high` |
| `codex` | `verified` | 예 | `gpt-6-astra` | 없음 | `high` |
| `agy` | `verified` | 예 | `gemini-4-argon-high` | `gemini-3.8-flash-high` | 없음 |
| `native` | `host` | 예 | 없음 | 없음 | 없음 |
| `command` | `unverified` | 아니요 | 없음 | 없음 | 없음 |

`binary`·`model`·`fallback_model`·`effort`를 쓰지 않는 preset에 주면 오류다(`agy`는 `effort`를 쓰지 않는다). 실행 파일을 찾는 규칙: G14.

### 모델과 강도를 정하는 순서

`claude`·`codex`·`agy` 리뷰어의 모델과 `claude`·`codex` 리뷰어의 강도는 이 순서로 정한다(G25).

1. 사용자 설정 파일의 그 리뷰어 항목에 `model`(강도는 `effort`)이 적혀 있으면 그 값. preset 기본값과 같은 값이나 `effort = ""`를 적어도 적힌 것이다.
2. 적혀 있지 않으면 이번 실행에서 에이전트가 `spar run`이나 `spar config`에 준 `--model 이름=모델`(강도는 `--effort 이름=강도`)의 값. 적히지 않은 리뷰어는 이 넷이 전부다: 파일 항목에 그 키가 없는 것, clean-review 출처의 이름, 기본 목록의 이름, `--reviewers`로 준 설정 밖의 이름.
3. 둘 다 없으면 preset 값.

에이전트가 모델을 고른 리뷰어의 대체 모델은 사용자가 적은 `fallback_model`뿐이고, 적혀 있지 않으면 대체 모델이 없다. preset의 대체 모델은 preset 모델과 짝이라 붙지 않는다. 대체 실행의 조건(한도 거절, 지정 모델과 다름)과 같은 강도로 도는 것은 그대로다. 에이전트가 고르지 않은 리뷰어는 이 변경 전과 같이 적힌 값과 preset 값으로 돈다.

인자는 여러 번 줄 수 있고 첫 `=`에서 나눈다. `=`가 없거나 이름이 비면 사용법 오류(64)다. 다음은 거부(65)이고, 리뷰어를 하나도 띄우지 않고 라운드 폴더를 만들지 않으며, 메시지에 리뷰어 이름과 값을 `%q`로 감싸 보인다. 이 목록이 전부다.

- 사용자 설정에 `model`이 적힌 리뷰어의 모델, `effort`가 적힌 리뷰어의 강도
- `native`·`command` 리뷰어의 모델이나 강도, `agy` 리뷰어의 강도
- 이번 실행의 리뷰어가 아닌 이름(이름은 글자 그대로 맞춘다)
- 같은 리뷰어에 같은 키를 두 번
- 허용 목록이 적힌 리뷰어에 그 목록에 없는 값
- 비었거나, 올바른 UTF-8이 아니거나(기록이 받은 그대로 담을 수 없다), `-`로 시작하거나, 공백 문자(`unicode.IsSpace`)나 제어 문자(`unicode.IsControl`)를 품은 값

이 밖의 값은 바꾸지 않고 인자 하나로 CLI에 넘긴다. spar는 모델 이름이나 강도 값이 실제로 있는지 확인하지 않고, CLI가 받지 않으면 그 리뷰어는 실패로 기록된다.

사용자 설정 파일의 항목은 허용 목록을 가질 수 있다: `allowed_models`(`claude`·`codex`·`agy`)와 `allowed_efforts`(`claude`·`codex`). 목록이 있으면 에이전트의 값은 목록의 한 항목과 대소문자까지 같아야 한다. 목록은 에이전트의 선택만 묶고, 에이전트가 고르지 않으면 목록과 관계없이 preset 값이다. 다음은 그 키의 줄을 대는 설정 오류다: 빈 목록, 문자열이 아닌 항목, 위의 값 형식을 어기는 항목, 같은 항목에 `model`과 `allowed_models`(또는 `effort`와 `allowed_efforts`)를 함께 적은 것, 그 키를 쓰지 않는 preset의 목록.

```toml
[[reviewers]]
preset = "codex"
allowed_models = ["example-model-pro", "example-model-mini"]
allowed_efforts = ["low", "medium", "high"]
```

## 2. 프로젝트 설정

저장소 최상위의 `.jaekit/spar.toml`은 다음 키만 가진다.

```toml
angles = ["requirement-counterexample", "check-validity"]
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**", "**/*_test.go"]

[caps]
plan = 1
plan_delta = 1
code = 3
```

프로젝트 설정은 작업 트리의 파일을 읽는다(G16).

- `caps`: 한 series에서 단계별로 셀 수 있는 라운드 수. 기본은 `plan` 1, `plan_delta` 1, `code` 3(candidate와 fixes 합계). 0 이상의 정수(G22).
- `angles`: 패킷이 묻는 공격 각도. 기본은 다섯 개 `requirement-counterexample`, `side-effects`, `design`, `premises`, `check-validity`이고, 이 가운데 일부로 줄일 수 있다. 순서는 위 순서로 정규화한다(G22).
- `rules`: 매 라운드 패킷에 넣을 규칙 문서(G23). 저장소 안에서 읽을 수 있는 파일이어야 하고, 아니면 `spar config`와 `spar run` 모두 설정 오류다.
- `check_paths`: 검사 코드로 보는 경로의 glob(G24). 잘못된 glob은 설정 오류다. 분류와 gate의 `growth`, 패킷이 쓴다.

`reviewers`, `preset`, `command`, `argv`, `binary`, `model`, `fallback_model`, `effort`, `allowed_models`, `allowed_efforts`는 어느 깊이에 있어도 그 키 이름과 위치를 대는 오류로 거부한다. clone한 저장소가 사용자 컴퓨터에서 명령을 실행하거나 사용자 계정의 비용을 바꾸지 못하게 하기 위해서다.

## 3. 오류

설정 오류는 리뷰어를 하나도 띄우지 않고, 라운드 폴더를 만들지 않으며 종료 코드 65로 끝난다. 메시지는 `<파일>:<줄>[:<열>]: <내용>` 형식이고 내용에 문제의 키나 값을 넣는다.

- TOML 문법 오류(TOML 1.0이 허용하지 않는 것, 같은 표를 두 번 정의한 것 포함): 오류가 난 줄
- 모르는 키: 그 키의 줄
- 중복된 리뷰어 이름(대소문자 무시), 예약 이름, 모르는 preset, preset에 쓰이지 않는 키: 그 키의 줄
- 잘못된 `rules`·`check_paths` 항목: 그 키의 줄과 항목
- 일반 파일이 아닌 출처 경로: 그 경로
- `argv`가 없는 `command`, `preset`이 없는 항목: 그 `[[reviewers]]` 머리의 줄
- 프로젝트 설정의 금지된 키: 그 키의 줄

## 4. 적용 기록

`spar config [--repo DIR] [--reviewers a,b] [--model 이름=모델]... [--effort 이름=강도]...`는 적용될 설정을 JSON으로 출력하고, 같은 인자를 준 라운드 기록 `round.json`의 `config`도 같은 값이다. `spar config`는 `spar run`이 거부하는 값을 같은 이유로 거부한다.

| 필드 | 뜻 |
| --- | --- |
| `source` | `kind`(`SPAR_CONFIG`, `XDG_CONFIG_HOME`, `HOME`, `clean-review`, `default`), `path`, `skipped`, `note` |
| `override` | `--reviewers`를 썼으면 그 이름들, 아니면 `null` |
| `choices` | 에이전트가 이번 실행에 준 값을 받은 그대로: 항목마다 `reviewer`, `key`(`model`·`effort`), `value`. 모델이 먼저, 키마다 준 순서. 없으면 빈 목록 `[]`. 이 필드가 생기기 전에 쓴 라운드에는 없고, 그 라운드를 다시 써도 생기지 않는다 |
| `configured_required` | `--reviewers`와 관계없이 설정이 필수로 정한 리뷰어의 `name`·`preset`. gate가 필수 리뷰어로 본다 |
| `files` | 읽은 설정 파일의 경로와 sha256 |
| `reviewers` | 적용된 리뷰어(`name`, `preset`, `isolation`, `required`, `model`, `fallback_model`, `effort`, `binary`, `argv`, `timeout_seconds`). `claude`·`codex`·`agy`는 `model_source`(`user`: 사용자 설정, `agent`: 에이전트의 `--model`, `preset`: preset 값)를, `claude`·`codex`는 `effort_source`(같은 세 값)를 가진다. 에이전트가 고를 수 있으면(적히지 않았으면) 고르지 않을 때 쓰일 `default_model`·`default_effort`를, 적었으면 `allowed_models`·`allowed_efforts`를 가진다. `native`·`command`에는 이 필드들이 없다 |
| `angles`, `caps`, `check_paths`, `rules` | 적용된 값 |
| `digest` | 리뷰어·`configured_required`·각도·상한·검사 경로·규칙의 정규 JSON sha256. 출처가 달라도 적용한 값이 같으면 같다: 리뷰어의 `model_source`·`effort_source`·`default_model`·`default_effort`와 `choices`는 넣지 않고, 에이전트가 고른 값은 적용한 `model`·`effort`로 들어간다. `--reviewers`가 같아도 설정이 필수로 정한 리뷰어가 다르면 digest가 다르다 |

`spar run --check-path GLOB`은 그 실행의 `check_paths`에 더하며 digest에 반영된다. 형식은 `check_paths`와 같고(G24), 잘못된 glob은 리뷰어를 띄우기 전에 거부한다.
