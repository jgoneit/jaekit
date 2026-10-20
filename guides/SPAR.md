# Spar 안내

**한국어** · [English](SPAR.en.md)

Spar는 두 커밋에 고정한 검토 패킷 하나를 여러 리뷰어에게 한꺼번에 보내고, 그들이 찾은 것과 그 처리를 기록합니다. 라운드는 `spar` 실행 파일이 돌리고, `spar` 플러그인은 에이전트에게 언제 라운드를 돌리고 지적을 어떻게 닫는지 알려 줍니다. Spar는 선택 기능이며 Spec과 Seal로 하는 첫 작업에는 필요 없습니다.

`spar`는 v0.1.6 다음 릴리스부터 릴리스 파일과 Homebrew formula에 `ha`와 함께 들어 있습니다. v0.1.6 릴리스 파일과 formula에는 없습니다. [설치 안내](INSTALL.md)를 보세요.

## 컴퓨터 밖으로 나가는 것

리뷰어마다 패킷 전체를 받습니다. 패킷에는 기준 문서, 규칙 문서, 검토 대상 문서, 두 커밋 사이의 diff가 들어 있습니다. 리뷰어 CLI는 작업공간 사본에서 두 커밋의 추적 파일도 읽을 수 있고, 연 내용을 자기 제공자에게 보냅니다.

| 리뷰어 | 패킷을 받는 제공자 |
| --- | --- |
| `claude` | Claude Code CLI를 거쳐 Anthropic |
| `codex` | Codex CLI를 거쳐 OpenAI |
| `agy` | Antigravity CLI를 거쳐 Google |
| `command` | 설정한 명령이 쓰는 서비스 |
| `native` | 지금 쓰는 에이전트의 제공자 |

커밋 메시지, Git 이력, 무시된 파일은 패킷에 없습니다. 기준 문서나 코드가 지금 쓰는 에이전트의 제공자 말고 다른 제공자에게 가면 안 되는 경우에는 `native` 리뷰어만 설정하세요. 컴퓨터 밖으로 아예 나가면 안 되는 경우에는 Spar를 돌리지 마세요. `native`를 포함한 모든 리뷰어가 패킷을 제공자에게 보냅니다.

## 리뷰어 설정

리뷰어는 사용자 설정에서만 옵니다. Spar는 `$SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml`, `~/.config/jaekit/spar.toml` 가운데 처음 찾은 파일을 읽습니다. 없으면 이미 있는 clean-review `config.json`의 `reviewers` 목록을, 그것도 없으면 `claude`, `codex`, `agy`를 씁니다. 빈 목록(`reviewers = []`)은 `native` 리뷰어 하나입니다.

```toml
timeout_seconds = 1800

[[reviewers]]
preset = "claude"

[[reviewers]]
preset = "codex"

[[reviewers]]
name = "agy"
preset = "agy"
model = "gemini-4-argon-high"
fallback_model = "gemini-3.8-flash-high"
```

- `claude`·`codex`·`agy`는 spar가 확인하는 읽기 전용·오프라인 보호로 CLI를 띄웁니다. 격리 등급 `verified`로 기록되고 필수 리뷰어에 듭니다.
- `fallback_model`은 지정 모델이 사용량·요청 한도로 거절될 때만, 같은 라운드에서 같은 패킷으로 한 번 실행합니다. `fallback_model = ""`는 preset의 기본 대체 모델을 끕니다.
- `native`는 지금 쓰는 에이전트의 clean-context 하위 에이전트입니다. Spar가 라운드 폴더에 프롬프트와 작업공간을 쓰고, 에이전트가 가져온 답을 기록합니다. `host`로 기록되고 필수 리뷰어에 듭니다.

### command 리뷰어

그 밖의 리뷰어는 명령으로 붙일 수 있습니다. 셸 없이 실행되고, 자리표시자로 패킷 파일·작업공간 사본·결과 schema를 받으며, schema를 따르는 JSON 객체 하나를 출력합니다. 아래는 합성 예시이고 `example-reviewer`는 직접 준비한 프로그램을 뜻합니다.

```toml
[[reviewers]]
name = "example-reviewer"
preset = "command"
argv = ["/opt/example/bin/example-reviewer", "--prompt", "{prompt}", "--workspace", "{workspace}", "--schema", "{schema}"]
timeout_seconds = 600
```

- `{prompt}`는 라운드의 `packet.md` 경로, `{workspace}`는 두 커밋의 사본(`head/`, `base/`), `{schema}`는 결과 JSON schema 경로입니다.
- 명령이 무엇을 하는지 spar가 확인할 수 없으므로 `unverified`로 기록되고 필수 리뷰어 수에 들지 않습니다. 지적은 같은 규칙으로 분류되고 처리됩니다.

`spar config`는 라운드가 띄울 리뷰어, 목록의 출처와 digest를 출력합니다. `spar run --reviewers codex,agy`는 한 번의 실행에서 목록을 바꾸고, 라운드 기록에 그렇게 남습니다.

### 프로젝트 설정

저장소의 `.jaekit/spar.toml`은 라운드 상한, 공격 각도, 규칙 문서, 검사 경로만 정할 수 있습니다.

```toml
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**"]

[caps]
code = 2
```

리뷰어·명령·실행 파일·모델은 정할 수 없고, 정하면 spar가 그 키를 알리며 멈춥니다. 전체 형식은 [설정 계약](../contracts/spar-config.md)에 있습니다.

## 검토 실행

에이전트에게 요청하세요. Codex에서는 `$spar:spar`, Claude Code에서는 `/spar:spar`입니다. skill은 다음 단계를 돌립니다.

| 단계 | 시점 |
| --- | --- |
| `plan` | 계획과 검사를 쓴 뒤 한 번 |
| `plan-delta` | plan 라운드로 계획이 바뀌었을 때 최대 한 번 |
| `candidate` | 모든 기준을 만족해야 하는 커밋에서 |
| `fixes` | 지적을 고친 뒤. base는 이전 코드 라운드의 head |

한 series는 plan 1회, plan delta 1회, 코드 라운드 3회까지입니다. 상한을 넘는 라운드는 리뷰어를 띄우기 전에 거부되고, `spar extend`로 기록한 사용자의 말이 있어야 더 돌릴 수 있습니다. 사용량 거절 등으로 같은 패킷을 끝내지 못한 리뷰어만 다시 돌리는 실행은 세지 않습니다.

라운드마다 Git이 무시하는 새 폴더에 `round.json`, `packet.md`, `findings.json`, `ledger.jsonl`, `summary.md`가 생깁니다. 패킷에 비공개 문서가 들어갈 수 있으므로 Git이 추적할 폴더는 거부합니다. 형식은 [라운드 계약](../contracts/review-round.md)에 있습니다. 리뷰어 결과의 위치·연결, 설정 값, 명령 인자처럼 spar가 받는 입력의 형식과 형식 밖 입력의 결과는 [입력 형식 계약](../contracts/spar-input.md)의 표 하나에 있습니다. 위치가 어떤 파일도 가리키지 않는 지적은 버리지 않고 읽을 수 없는 위치로 표시하며, 자동 보류하지 않습니다.

## 지적과 gate

Spar는 지적을 `defect`, `requirement-question`, `hardening`, `note` 가운데 하나로 분류합니다. 요구 질문은 사용자의 말로만 닫히고, 검사 코드에서 거짓 통과·결과 의미 뒤바뀜·이름을 댄 규칙 위반을 주장하지 않는 엣지 케이스는 `hardening`으로 보류됩니다. 끝내지 못한 리뷰어는 원인과 함께 보이고 지적 0건으로 세지 않습니다.

`spar gate`는 라운드들과 완료하려는 head를 읽고 `review-gate/v1` 파일을 씁니다. `complete`(종료 0), 필수 리뷰어가 끝내지 못했으면 `degraded`(3), 열린 지적·상한 초과·검토되지 않은 head가 있으면 `blocked`(4)입니다. 검토 수정이 검사 코드와 그 밖에 더한 줄 수도 남깁니다. [gate 계약](../contracts/review-gate.md)을 보세요.

gate는 검토 기록이지 완료 근거가 아닙니다. Seal의 완료는 여전히 `ha done`에서만 나옵니다.
