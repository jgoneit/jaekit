# Spar 독립 검토 안내

**한국어** · [English](SPAR.en.md)

`standard`·`strict` 목표를 처음 맡기기 전에, 또는 리뷰어 설정을 바꾸거나 검토 기록을 읽을 때 읽습니다. Spar는 작업한 에이전트가 놓친 문제를 그 대화 밖의 리뷰어들이 찾게 하는 독립 검토 도구입니다. 리뷰어는 요구사항을 바꾸지 못하고, 찾은 것은 `defect`, `requirement-question`, `hardening`, `note`로 나뉘고 그 처리가 기록됩니다. 끝난 검토는 gate 파일로 남고 `ha`가 완료 판정에 씁니다. 언제 필요한지는 아래와 [Jaekit 이해하기의 검토 등급](OVERVIEW.md#검토-등급)에 있습니다.

자세히는, Spar는 두 커밋에 고정한 검토 패킷 하나를 여러 리뷰어에게 한꺼번에 보내고, 그들이 찾은 것과 그 처리를 기록합니다. 라운드는 `spar` 실행 파일이 돌리고, `spar` 플러그인은 에이전트에게 언제 라운드를 돌리고 지적을 어떻게 닫는지 알려 줍니다.

v0.1.7부터 Seal은 새 목표를 `run-rules/5`로 시작하고, `run-rules/5`로 시작한 `standard`·`strict` 목표는 기록된 Spar gate가 있어야 완료됩니다. `light` 목표, `Assurance:` 줄이 없는 목표, `run-rules/4` 이하로 시작한 목표에는 Spar가 필요 없습니다. 검토 없이 끝내려면 등급을 `light`로 낮추는 목표 문서 수정을 사용자가 정합니다([검토 gate](USAGE.md#검토-gate-run-rules5)).

`spar`는 v0.1.7부터 릴리스 파일과 Homebrew formula에 `ha`와 함께 들어 있습니다. v0.1.6 이하의 릴리스 파일과 formula에는 없습니다. [설치 안내](INSTALL.md)를 보세요.

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

이런 사용자 설정이 하나도 없으면 `claude`, `codex`, `agy` 셋이 모두 필수 리뷰어입니다. 필수 리뷰어의 CLI가 없거나 그 리뷰어가 라운드를 끝내지 못하면, 그 라운드에 사용자의 결정(`spar waive`)이 없는 한 gate는 `degraded`이고, 그래서 `standard`·`strict` 목표가 완료되지 않습니다. 첫 라운드 전에 가진 CLI만 리뷰어로 적은 사용자 설정을 `~/.config/jaekit/spar.toml`에 만드세요. 예를 들어 claude CLI만 있으면 그 리뷰어 하나만 적습니다.

```toml
[[reviewers]]
preset = "claude"
```

리뷰어를 하나만 적으면 그 리뷰어가 유일한 필수 리뷰어이고, 그 리뷰어가 끝낸 라운드는 `complete`가 될 수 있습니다. 리뷰어 CLI가 하나도 없으면 빈 목록으로 `native` 리뷰어 하나를 씁니다. 지금 쓰는 에이전트의 clean-context 하위 에이전트가 검토합니다.

```toml
reviewers = []
```

세 CLI가 모두 있으면 이렇게 적을 수 있습니다.

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

라운드를 돌리기 전에 `spar config`로 띄울 리뷰어를 확인합니다. `spar config`는 라운드가 띄울 리뷰어, 목록의 출처와 digest를 출력합니다. `spar run --reviewers codex,agy`는 한 번의 실행에서 목록을 바꾸고, 라운드 기록에 그렇게 남습니다.

### 프로젝트 설정

저장소의 `.jaekit/spar.toml`은 라운드 상한, 공격 각도, 규칙 문서, 검사 경로만 정할 수 있습니다.

```toml
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**"]

[caps]
code = 2
```

리뷰어·명령·실행 파일·모델은 정할 수 없고, 정하면 spar가 그 키를 알리며 멈춥니다. 전체 형식은 [설정 계약](../contracts/spar-config.md)에 있습니다.

## 프로젝트 준비

패킷에 비공개 문서가 들어갈 수 있으므로 Spar는 Git이 무시하는 폴더에만 검토 기록을 씁니다. Spar를 처음 쓰는 프로젝트의 Git은 아직 그 폴더를 무시하지 않아서, 첫 라운드가 리뷰어를 띄우기 전에 65로 멈추고 아무것도 쓰지 않습니다. 프로젝트에 첫 `standard`·`strict` 목표를 맡기기 전에 프로젝트의 `.gitignore`에 한 줄을 더합니다.

```text
docs/reviews/
```

`docs/reviews/<goal>/`은 Spar skill이 목표의 라운드와 gate를 두는 곳입니다. `light` 목표와 Spar를 쓰지 않는 목표에는 필요 없습니다.

- **다른 폴더.** 기록은 Git이 무시하는 어느 폴더에나 둘 수 있습니다. 그 폴더를 `.gitignore`에 더하고, 에이전트에게 라운드를 그곳에 두라고 말합니다.
- **내 컴퓨터에서만.** `.gitignore`를 바꾸지 않으려면 같은 줄을 `.git/info/exclude`에 더합니다. 이 파일은 commit되지 않으므로 다른 clone이나 컴퓨터에서는 다시 더해야 합니다.
- Git이 추적하는 파일이 아래에 있는 폴더도 Spar는 거부합니다. 폴더를 무시하게 해도 이미 commit된 파일의 추적은 풀리지 않습니다.

## 검토 실행

에이전트에게 요청하세요. Codex에서는 `$spar:spar`, Claude Code에서는 `/spar:spar`입니다. skill은 다음 단계를 돌립니다.

| 단계 | 시점 |
| --- | --- |
| `plan` | 계획과 검사를 쓴 뒤 한 번 |
| `plan-delta` | plan 라운드로 계획이 바뀌었을 때 최대 한 번 |
| `candidate` | 모든 기준을 만족해야 하는 커밋에서 |
| `fixes` | 지적을 고친 뒤. base는 이전 코드 라운드의 head |

한 series는 plan 1회, plan delta 1회, 코드 라운드 3회까지입니다. 상한을 넘는 라운드는 리뷰어를 띄우기 전에 거부되고, `spar extend`로 기록한 사용자의 말이 있어야 더 돌릴 수 있습니다. 사용량 거절 등으로 같은 패킷을 끝내지 못한 리뷰어만 다시 돌리는 실행은 세지 않습니다.

라운드마다 Git이 무시하는 새 폴더에 `round.json`, `packet.md`, `findings.json`, `ledger.jsonl`, `summary.md`가 생깁니다. 패킷에 비공개 문서가 들어갈 수 있으므로 Git이 추적할 폴더는 거부합니다([프로젝트 준비](#프로젝트-준비)).

<details>
<summary>라운드·입력 형식과 위치를 읽을 수 없는 지적</summary>

형식은 [라운드 계약](../contracts/review-round.md)에 있습니다. 리뷰어 결과의 위치·연결, 설정 값, 명령 인자처럼 spar가 받는 입력의 형식과 형식 밖 입력의 결과는 [입력 형식 계약](../contracts/spar-input.md)의 표 하나에 있습니다.

위치가 어떤 파일도 가리키지 않는 지적은 버리지 않고 읽을 수 없는 위치로 표시하며, 자동 보류하지 않습니다.

</details>

## 지적과 gate

gate는 검토 기록이지 완료 근거가 아닙니다. Seal의 완료는 여전히 `ha done`에서만 나옵니다. `run-rules/5`로 시작한 `standard`·`strict` 목표에서는 Seal이 gate 파일을 `ha note <goal> review --gate <file>`로 기록하고, 기록된 gate가 지금 HEAD와 목표 문서를 덮어야 `ha done`이 완료를 기록합니다([검토 gate](USAGE.md#검토-gate-run-rules5)). 그래서 라운드에는 SPEC.md와 그것이 링크한 목표 문서를 모두 `--criteria`로 줍니다.

### 지적의 분류

Spar는 지적을 `defect`, `requirement-question`, `hardening`, `note` 가운데 하나로 분류합니다. 요구 질문은 사용자의 말로만 닫히고, 검사 코드에서 거짓 통과·결과 의미 뒤바뀜·이름을 댄 규칙 위반을 주장하지 않는 엣지 케이스는 `hardening`으로 보류됩니다. 끝내지 못한 리뷰어는 원인과 함께 보이고 지적 0건으로 세지 않습니다.

### 가장 작은 수정 방향

리뷰어는 지적마다 `smallest_fix`에 가장 작은 수정 방향을 적습니다. 실패 시나리오가 맞게 나오게 하는 가장 작은 변경을 패치가 아닌 문장으로 적고, 말할 수 없으면 비워 둘 수 있습니다.

방향은 `findings.json`에 남고 `summary.md`의 `## Smallest fixes`에 보입니다. 방향은 제안이라 분류·처리·gate에 쓰지 않습니다. 에이전트는 수정을 그 방향 안에서 시작하고, 더 크게 고치면 그 까닭을 `spar triage … --note`로 남깁니다.

### gate의 상태

`spar gate`는 라운드들과 완료하려는 head를 읽고 `review-gate/v1` 파일을 씁니다. `complete`(종료 0), 필수 리뷰어가 끝내지 못했고 그 라운드에 사용자의 결정이 없으면 `degraded`(3), 열린 지적·상한 초과·검토되지 않은 head가 있으면 `blocked`(4)입니다. 검토 수정이 검사 코드와 그 밖에 더한 줄 수도 남깁니다. [gate 계약](../contracts/review-gate.md)을 보세요.

### 리뷰어 없이 진행하기

필수 리뷰어가 사용량 한도 같은 이유로 라운드를 끝내지 못했고 사용자가 그 리뷰어 없이 진행하라고 하면, 에이전트가 `spar waive <라운드> <리뷰어> --user-quote "…"`로 그 말을 그 라운드에 기록합니다. 사용자의 말 없이는 기록하지 않습니다. 결정이 기록된 리뷰어는 그 라운드의 `degraded` 사유가 되지 않고, gate 파일의 그 라운드 `waived_reviewers`에 상태와 사용자의 말이 함께 보입니다. 결정은 그 라운드에만 적용되고, 완료한 필수 리뷰어가 하나도 없는 라운드는 결정이 있어도 `degraded`입니다.

