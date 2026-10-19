# 계약: Spar 설정

`spar`가 어떤 리뷰어를 띄우고 어떤 상한·각도·규칙·검사 경로를 쓸지는 두 설정 파일이 정한다. 사용자 설정은 리뷰어를 정하고, 저장소의 프로젝트 설정은 리뷰어를 정할 수 없다. 형식은 [TOML 1.0.0](https://toml.io/en/v1.0.0)이고 `spar`는 `github.com/pelletier/go-toml/v2` v2.2.4로 읽는다. 아래 예시의 이름·경로·모델은 합성이다.

## 1. 사용자 설정과 출처 순서

리뷰어 목록은 처음 찾은 출처 하나에서만 온다.

1. 환경 변수 `SPAR_CONFIG`가 가리킨 파일
2. `$XDG_CONFIG_HOME/jaekit/spar.toml`
3. `~/.config/jaekit/spar.toml`
4. 위 셋이 모두 없으면 clean-review가 읽는 `config.json` 하나의 `reviewers` 목록. 그 파일은 clean-review와 같은 방식으로 정한다: 환경 변수 `CLEAN_REVIEW_CONFIG`가 있으면 그 파일, 없으면 `$XDG_CONFIG_HOME/clean-review/config.json`, `XDG_CONFIG_HOME`도 없으면 `~/.config/clean-review/config.json`. 정한 파일이 없으면 다른 경로를 찾지 않고 다음 출처로 넘어간다. 각 이름은 같은 이름의 preset이다(`claude`, `codex`, `agy`, `native`).
5. 그것도 없으면 `claude`, `codex`, `agy`

- 가리킨 파일이 없으면 다음 출처로 넘어가고, 건너뛴 경로를 `source.skipped`에 남긴다. 있는 파일이 잘못되었으면 넘어가지 않고 오류로 끝난다. clean-review `config.json`은 JSON 객체여야 한다(`null`도 잘못된 파일이다).
- 출처 파일에 `reviewers` 키가 없으면 기본 목록, 빈 목록이면 `native` 하나다.
- `spar run`과 `spar config`의 `--reviewers a,b`는 설정보다 앞선다. 설정에 있는 이름은 그 설정을, 없는 이름은 같은 이름의 preset을 쓴다. 빈 값(`--reviewers ""`)은 `native` 하나다. 적용 기록의 `override`에 `{"kind": "--reviewers", "names": [...]}`로 남는다.

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
| `name` | 영문자·숫자로 시작하고 영문자·숫자·`.`·`_`·`-`로 된 64자 이하. 한 파일에서 겹치면 오류. 라운드 폴더의 이름(`round.json`, `packet.md`, `schema.json`, `findings.json`, `ledger.jsonl`, `summary.md`, `workspace`)은 대소문자와 관계없이 쓸 수 없다 |
| `preset` | `claude`, `codex`, `agy`, `native`, `command` 중 하나. 필수 |
| `argv` | `command`에만 쓰고 `command`에는 필수. 셸 없이 그대로 실행하는 인자 배열이며 `{prompt}`·`{workspace}`·`{schema}`를 패킷 파일·작업공간·결과 schema 경로로 바꾼다 |
| `binary` | `claude`·`codex`·`agy`의 실행 파일. 없으면 PATH에서 preset 이름을 찾는다 |
| `model` | 요청할 모델 |
| `fallback_model` | 지정 모델이 사용량·요청 한도로 거절될 때 같은 라운드에서 한 번 쓰는 모델. 빈 문자열이면 대체 모델이 없다 |
| `effort` | `claude`·`codex`의 추론 강도 |
| `timeout_seconds` | 양의 정수. 리뷰어별 값이 최상위 값보다 앞선다 |

preset 기본값:

| preset | 격리 등급 | 필수 | 기본 모델 | 기본 대체 모델 | 기본 강도 |
| --- | --- | --- | --- | --- | --- |
| `claude` | `verified` | 예 | `opus` | 없음 | `high` |
| `codex` | `verified` | 예 | `gpt-6-astra` | 없음 | `high` |
| `agy` | `verified` | 예 | `gemini-4-argon-high` | `gemini-3.8-flash-high` | 없음 |
| `native` | `host` | 예 | 없음 | 없음 | 없음 |
| `command` | `unverified` | 아니요 | 없음 | 없음 | 없음 |

`binary`·`model`·`fallback_model`·`effort`를 쓰지 않는 preset에 주면 오류다(`agy`는 `effort`를 쓰지 않는다).

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

- `caps`: 한 series에서 단계별로 셀 수 있는 라운드 수. 기본은 `plan` 1, `plan_delta` 1, `code` 3(candidate와 fixes 합계). 0 이상의 정수.
- `angles`: 패킷이 묻는 공격 각도. 기본은 다섯 개 `requirement-counterexample`, `side-effects`, `design`, `premises`, `check-validity`이고, 이 가운데 일부로 줄일 수 있다. 순서는 위 순서로 정규화한다.
- `rules`: 매 라운드 패킷에 넣을 규칙 문서. 저장소 안의 상대 경로이고, `..`로 나가거나 심볼릭 링크로 저장소 밖을 가리키면 거부한다.
- `check_paths`: 검사 코드로 보는 경로의 glob. `**`는 여러 폴더, `*`·`?`·`[...]`는 한 이름 안에서 맞춘다. 저장소 밖을 가리킬 수 없다. 분류와 gate의 `growth`가 쓴다.

`reviewers`, `preset`, `command`, `argv`, `binary`, `model`, `fallback_model`, `effort`는 어느 깊이에 있어도 그 키 이름과 위치를 대는 오류로 거부한다. clone한 저장소가 사용자 컴퓨터에서 명령을 실행하거나 사용자 계정의 비용을 바꾸지 못하게 하기 위해서다.

## 3. 오류

설정 오류는 리뷰어를 하나도 띄우지 않고, 라운드 폴더를 만들지 않으며 종료 코드 65로 끝난다. 메시지는 `<파일>:<줄>[:<열>]: <내용>` 형식이고 내용에 문제의 키나 값을 넣는다.

- TOML 문법 오류(TOML 1.0이 허용하지 않는 것, 같은 표를 두 번 정의한 것 포함): 오류가 난 줄
- 모르는 키: 그 키의 줄
- 중복된 리뷰어 이름, 모르는 preset, preset에 쓰이지 않는 키: 그 키의 줄
- `argv`가 없는 `command`, `preset`이 없는 항목: 그 `[[reviewers]]` 머리의 줄
- 프로젝트 설정의 금지된 키: 그 키의 줄

## 4. 적용 기록

`spar config [--repo DIR] [--reviewers a,b]`는 적용될 설정을 JSON으로 출력하고, 라운드 기록 `round.json`의 `config`도 같은 값이다.

| 필드 | 뜻 |
| --- | --- |
| `source` | `kind`(`SPAR_CONFIG`, `XDG_CONFIG_HOME`, `HOME`, `clean-review`, `default`), `path`, `skipped`, `note` |
| `override` | `--reviewers`를 썼으면 그 이름들, 아니면 `null` |
| `configured_required` | `--reviewers`와 관계없이 설정이 필수로 정한 리뷰어의 `name`·`preset`. gate가 필수 리뷰어로 본다 |
| `files` | 읽은 설정 파일의 경로와 sha256 |
| `reviewers` | 적용된 리뷰어(`name`, `preset`, `isolation`, `required`, `model`, `fallback_model`, `effort`, `binary`, `argv`, `timeout_seconds`) |
| `angles`, `caps`, `check_paths`, `rules` | 적용된 값 |
| `digest` | 리뷰어·각도·상한·검사 경로·규칙의 정규 JSON sha256. 출처가 달라도 적용한 값이 같으면 같다 |

`spar run --check-path GLOB`은 그 실행의 `check_paths`에 더하며 digest에 반영된다.
