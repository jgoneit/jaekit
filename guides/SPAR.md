# Spar 독립 검토 안내

**한국어** · [English](SPAR.en.md)

Spar는 독립 검토 도구입니다. 작업한 에이전트가 놓친 문제를 그 대화 밖의 리뷰어들이 찾습니다. 리뷰어는 요구사항을 바꾸지 못합니다. 리뷰어가 찾은 것은 `defect`, `requirement-question`, `hardening`, `note`로 나뉘고, 각각의 처리가 기록됩니다. 끝난 검토는 gate 파일로 남고, `ha`가 이 파일을 완료 판정에 씁니다.

`standard`·`strict` 목표를 처음 맡기기 전에 이 안내를 읽습니다. 리뷰어 설정을 바꾸거나 검토 기록을 읽을 때도 읽습니다. 언제 필요한지는 아래와 [Jaekit 이해하기의 검토 등급](OVERVIEW.md#검토-등급)에 있습니다.

Spar는 두 커밋에 고정한 검토 패킷 하나를 여러 리뷰어에게 한꺼번에 보냅니다. 그리고 리뷰어들이 찾은 것과 그 처리를 기록합니다. 라운드는 `spar` 실행 파일이 돌립니다. `spar` 플러그인은 언제 라운드를 돌리고 지적을 어떻게 닫는지 에이전트에게 알려 줍니다.

`standard`·`strict` 목표는 기록된 Spar gate가 있어야 완료됩니다. `light` 목표와 `Assurance:` 줄이 없는 목표에는 Spar가 필요 없습니다.

검토 없이 끝내려면 목표 문서에서 등급을 `light`로 낮춰야 합니다. 이 수정은 사용자가 정합니다([검토 gate](RULES.md#검토-gate)). `review` 조건이 있으면 그 기준도 바꿔야 합니다. 바꾸지 않으면 `review_tier_missing`으로 완료되지 않습니다.

`spar`는 릴리스 파일과 Homebrew formula에 `ha`와 함께 들어 있습니다. [설치 안내](INSTALL.md)를 보세요.

## 컴퓨터 밖으로 나가는 것

리뷰어는 저마다 패킷 전체를 받습니다. 패킷에는 다음이 들어 있습니다.

- 기준 문서, 규칙 문서, 검토 대상 문서, 검사 경계 문서
- 넘긴 사용자의 결정(사용자의 말 그대로)과 질문
- 앞선 라운드의 지적과 기록된 처리(근거와 사용자의 말 포함)
- 두 커밋 사이의 diff

리뷰어 CLI는 작업공간 사본에서 두 커밋의 추적 파일도 읽을 수 있습니다. 그렇게 연 내용은 자기 제공자에게 보냅니다.

| 리뷰어 | 패킷을 받는 제공자 |
| --- | --- |
| `claude` | Claude Code CLI를 거쳐 Anthropic |
| `codex` | Codex CLI를 거쳐 OpenAI |
| `agy` | Antigravity CLI를 거쳐 Google |
| `command` | 설정한 명령이 쓰는 서비스 |
| `native` | 지금 쓰는 에이전트의 제공자 |

커밋 메시지, Git 이력, 무시된 파일은 패킷에 없습니다. 기준 문서나 코드가 지금 쓰는 에이전트의 제공자 말고 다른 제공자에게 가면 안 되는 경우에는 `native` 리뷰어만 설정하세요. 컴퓨터 밖으로 아예 나가면 안 되는 경우에는 Spar를 돌리지 마세요. `native`를 포함한 모든 리뷰어가 패킷을 제공자에게 보냅니다.

리뷰어는 패킷과 함께 자기 지시도 받습니다. 관점이 있는 라운드에서는 리뷰어마다 받은 입력이 그대로 `<round>/<reviewer>/input.md`에 남습니다. 리뷰어마다 지시와 패킷을 어떻게 건네는지는 [라운드 계약](../contracts/review-round.md#2-packet-and-workspaces)에 있습니다.

## 리뷰어 설정

리뷰어는 사용자 설정에서만 옵니다. Spar는 `$SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml`, `~/.config/jaekit/spar.toml` 가운데 처음 찾은 파일을 읽습니다. 없으면 이미 있는 clean-review `config.json`의 `reviewers` 목록을 씁니다. 그것도 없으면 `claude`, `codex`, `agy`를 씁니다. 빈 목록(`reviewers = []`)은 `native` 리뷰어 하나입니다.

이런 사용자 설정이 하나도 없으면 `claude`, `codex`, `agy` 셋이 모두 필수 리뷰어입니다. 필수 리뷰어의 CLI가 없거나 그 리뷰어가 라운드를 끝내지 못하면 gate는 `degraded`입니다. 그 라운드에 사용자의 결정(`spar waive`)이 있으면 예외입니다. gate가 `degraded`이면 `standard`·`strict` 목표가 완료되지 않습니다. 첫 라운드 전에, 가진 CLI만 리뷰어로 적은 사용자 설정을 `~/.config/jaekit/spar.toml`에 만드세요. 예를 들어 claude CLI만 있으면 그 리뷰어 하나만 적습니다.

```toml
[[reviewers]]
preset = "claude"
```

리뷰어를 하나만 적으면 그 리뷰어가 유일한 필수 리뷰어입니다. 그 리뷰어가 끝낸 라운드는 `complete`가 될 수 있습니다. 리뷰어 CLI가 하나도 없으면 빈 목록으로 `native` 리뷰어 하나를 씁니다. 그러면 지금 쓰는 에이전트의 clean-context 하위 에이전트가 검토합니다.

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

- `claude`·`codex`·`agy`는 읽기 전용·오프라인 보호를 걸고 CLI를 띄웁니다. 그 보호는 spar가 확인합니다. 격리 등급 `verified`로 기록되고, 필수 리뷰어에 듭니다.
- `agy`는 파일 읽기·목록·검색 도구와 셸 명령만 쓸 수 있습니다. 셸 명령에는 읽기 전용 프로그램(`ls`, `grep`, `cat`, `diff`, `true` 등)과 `sed -n`만 쓸 수 있고, 이들을 `|`, `&&`, `||`, `;`로 이을 수 있습니다. 코드·시험·빌드·Git이나 그 밖의 프로그램을 실행하면 spar가 그 리뷰어를 멈춥니다. 그리고 `tool_violation`으로 기록하고 결과를 버립니다.
- `fallback_model`은 지정 모델이 사용량·요청 한도로 거절될 때만 씁니다. 그때 같은 라운드에서 같은 패킷으로 한 번 실행합니다. `fallback_model = ""`는 preset의 기본 대체 모델을 끕니다.
- `native`는 지금 쓰는 에이전트의 clean-context 하위 에이전트입니다. Spar가 라운드 폴더에 프롬프트와 작업공간을 씁니다. 에이전트가 답을 가져오면 Spar가 그 답을 기록합니다. `host`로 기록되고, 필수 리뷰어에 듭니다.
- 리뷰어마다 `max_packet_bytes`로 받을 패킷의 최대 바이트 수를 적을 수 있습니다. 적지 않으면 `claude`·`codex`는 1,200,000바이트, `agy`는 4,000,000바이트입니다. `native`·`command`는 한도가 없습니다. 패킷이 이번 실행의 어느 리뷰어의 한도보다 크면, `spar run`은 리뷰어를 띄우기 전에 65로 멈춥니다. 라운드를 쓰지 않으므로 상한도 쓰지 않습니다. 그때는 변경이나 넘긴 문서를 줄입니다. 그 리뷰어가 더 큰 패킷을 받는다는 것을 알면 한도를 올려도 됩니다.

### 모델과 강도

`claude`·`codex`·`agy`의 모델과 `claude`·`codex`의 강도는 사용자 설정에 적은 값이 먼저입니다. 위 예시처럼 `model`을 적으면 그 모델로 고정됩니다. 적지 않은 리뷰어의 모델과 강도는 라운드를 돌리는 에이전트가 고릅니다. 고르지 않으면 preset 값으로 돕니다(모델 `opus`·`gpt-6-astra`·`gemini-4-argon-high`, 강도 `high`).

에이전트가 고를 수 있는 값을 묶으려면 `allowed_models`와 `allowed_efforts`를 적습니다. 목록이 있으면 에이전트는 그 안의 값만 고를 수 있습니다.

```toml
[[reviewers]]
preset = "codex"
allowed_models = ["gpt-6-astra", "example-model-mini"]
allowed_efforts = ["medium", "high"]
```

사용자 설정을 덮는 값이나 허용 목록 밖의 값을 받으면, spar는 리뷰어를 하나도 띄우지 않고 거부합니다. 어떤 값을 거부하는지는 [설정 계약](../contracts/spar-config.md#how-the-model-and-effort-are-decided)에 있습니다. 라운드 기록과 요약은 리뷰어마다 모델·강도와 그 출처(`user`, `agent`, `preset`)를 남깁니다.

### command 리뷰어

그 밖의 리뷰어는 명령으로 붙일 수 있습니다. 명령은 셸 없이 실행됩니다. 패킷 파일, 작업공간 사본, 결과 schema는 자리표시자로 받습니다. 명령은 schema를 따르는 JSON 객체 하나를 출력합니다. 아래는 합성 예시입니다. `example-reviewer`는 직접 준비한 프로그램을 뜻합니다.

```toml
[[reviewers]]
name = "example-reviewer"
preset = "command"
argv = ["/opt/example/bin/example-reviewer", "--prompt", "{prompt}", "--workspace", "{workspace}", "--schema", "{schema}"]
timeout_seconds = 600
```

- `{prompt}`: 관점 없는 라운드에서는 `packet.md` 경로입니다. 관점이 있는 라운드에서는 그 리뷰어의 `input.md` 경로입니다.
- `{workspace}`: 두 커밋의 사본(`head/`, `base/`)입니다.
- `{schema}`: 결과 JSON schema 경로입니다.
- 명령이 무엇을 하는지 spar는 확인할 수 없습니다. 그래서 `unverified`로 기록되고, 필수 리뷰어 수에 들지 않습니다. 지적은 같은 규칙으로 분류되고 처리됩니다.

라운드를 돌리기 전에 `spar config`로 띄울 리뷰어를 확인합니다. `spar config`는 라운드가 띄울 리뷰어, 목록의 출처와 digest, 리뷰어마다 패킷 한도를 출력합니다. `spar run --reviewers codex,agy`는 한 번의 실행에서 목록을 바꿉니다. 라운드 기록에도 그렇게 남습니다.

### 프로젝트 설정

저장소의 `.jaekit/spar.toml`은 라운드 상한, 공격 각도, 규칙 문서, 검사 경로만 정할 수 있습니다.

```toml
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**"]

[caps]
code = 2
```

리뷰어, 명령, 실행 파일, 모델, 강도, 허용 목록은 정할 수 없습니다. 정하면 spar가 그 키를 알리며 멈춥니다. 전체 형식은 [설정 계약](../contracts/spar-config.md)에 있습니다.

## 프로젝트 준비

Spar는 Git이 무시하는 폴더에만 검토 기록을 씁니다. 패킷에 비공개 문서가 들어갈 수 있기 때문입니다. Spar를 처음 쓰는 프로젝트에서는 Git이 아직 그 폴더를 무시하지 않습니다. 그래서 첫 라운드는 리뷰어를 띄우기 전에 65로 멈추고, 아무것도 쓰지 않습니다.

프로젝트에 첫 `standard`·`strict` 목표를 맡기기 전에, 프로젝트의 `.gitignore`에 한 줄을 더합니다. 그리고 목표를 시작하기 전에 그 변경을 커밋합니다. Seal은 커밋된 깨끗한 트리에서만 검사를 돌립니다.

```text
docs/reviews/
```

`docs/reviews/<goal>/`은 Spar skill이 목표의 라운드와 gate를 두는 곳입니다. 이 준비는 `light` 목표와 Spar를 쓰지 않는 목표에는 필요 없습니다.

- **다른 폴더.** 기록은 Git이 무시하는 어느 폴더에나 둘 수 있습니다. 그 폴더를 `.gitignore`에 더하고, 에이전트에게 라운드를 그곳에 두라고 말합니다.
- **내 컴퓨터에서만.** `.gitignore`를 바꾸지 않으려면 같은 줄을 `.git/info/exclude`에 더합니다. 이 파일은 commit되지 않으므로, 다른 clone이나 컴퓨터에서는 다시 더해야 합니다.
- **추적 중인 파일.** Git이 추적하는 파일이 아래에 있는 폴더도 Spar는 거부합니다. 폴더를 무시하게 해도 이미 commit된 파일의 추적은 풀리지 않습니다.

## 검토 실행

에이전트에게 요청하세요. Codex에서는 `$spar:spar`, Claude Code에서는 `/spar:spar`입니다. skill은 다음 단계를 돌립니다.

| 단계 | 시점 |
| --- | --- |
| `plan` | 계획과 검사를 쓴 뒤, 구현 전에 한 번 |
| `candidate` | 모든 기준을 만족해야 하는 커밋에서, 그리고 검토 수정 뒤 다시. base는 늘 목표의 base commit |

검토 수정 뒤에는 목표의 base commit부터 새 head까지 `candidate` 라운드를 한 번 돌립니다. 이 라운드는 앞선 코드 라운드의 지적과 그 마지막 처리를 받습니다. 그런 라운드 가운데 열린 `defect`·`requirement-question` 없이 끝난 마지막 라운드가 누적 라운드입니다.

plan 라운드로 목표 문서나 검사가 바뀌면, 다음 `candidate` 라운드가 그때의 목표 문서를 검토합니다. 계획이나 검사만 바뀌었으면 plan 라운드를 다시 돌리지 않습니다.

다만 `strict` 목표에서 Seal Core는 마지막 `plan` 라운드도 지금의 목표 문서를 받았기를 요구합니다(`review_docs_stale`). 그래서 plan 라운드 뒤에 SPEC.md나 그것이 연결한 목표 문서가 바뀌면, 사용자에게 plan 상한 연장을 묻습니다(`spar extend <series> --stage plan --by 1 --user-quote "…"`). 그리고 지금의 head에서 `plan` 라운드를 한 번 더 돌립니다.

### 상한과 연장

한 series는 plan 1회, 코드 라운드 2회까지입니다. 상한을 넘는 라운드는 리뷰어를 띄우기 전에 거부됩니다. 더 돌리려면 `spar extend`로 기록한 사용자의 말이 있어야 합니다. 사용량 거절 등으로 같은 패킷을 끝내지 못한 리뷰어만 다시 돌리는 실행은 세지 않습니다.

코드 라운드 상한을 다 쓰면, `spar run`의 거부 메시지가 그 series의 상한 보고를 안내합니다. `spar cap-report`는 그 series의 기록과 Git만 읽고, 아무것도 쓰지 않습니다. 보고에는 상한 사용, 라운드, 열린 지적과 네 신호(수정 연쇄, 반복 기준, 변경 무관, 검사 비대)가 나옵니다.

```bash
spar cap-report docs/reviews/example
```

보고는 원인을 고르지 않습니다. 원인(`--cause`)은 에이전트가 신호를 보고 하나 고릅니다. 원인은 `churn`, `recurring`, `unrelated`, `hardening`, `other` 가운데 하나입니다. 에이전트는 권고와 함께 사용자에게 묻습니다. 더 돌리라는 사용자의 말이 있을 때만 `spar extend <series> --stage code --by 1 --cause <원인> --user-quote "…"`로 기록합니다. `--by 2` 이상이나 `--cause other`에는 `--reason`이 필요합니다. `ha cost`에 `--series`로 그 series를 주면, 기록한 원인과 이유가 상한 연장 목록에 보입니다.

### 사용자의 결정

사용자가 정한 것은 그 뒤의 모든 라운드에 결정으로 넘깁니다. `requirement-question`에 대한 사용자의 답이 대표적인 결정입니다. Seal 없이 Spar만 쓸 때도 같습니다. 결정마다 `--decision`(정한 대상)과 바로 다음의 `--decision-quote`(사용자의 말)를 한 쌍으로 줍니다.

```bash
spar run --stage candidate --base 1111111 --head 3333333 \
  --criteria docs/specs/example/SPEC.md --rules AGENTS.md \
  --decision "AC-2의 빈 입력" --decision-quote "빈 입력은 오류로 둬" \
  --out docs/reviews/example/03-candidate
```

결정은 패킷의 `## Decisions the user made` 절에 받은 그대로 들어갑니다. 이 절은 리뷰어에게 세 가지를 안내합니다.

- 결정이 정한 대로 한 것은 지적하지 않습니다.
- 결정을 넘는 변경은 평소대로 지적합니다.
- 결정을 따르면 그 결정이 다루지 않은 기준·규칙·계약이 깨질 때는, 그것을 연결한 `requirement-question`으로 보고합니다.

spar는 그 말이 사용자의 말인지 확인하지 않습니다.

### 검사 경계와 고정 글 검사

검사의 경계를 적은 문서가 있으면 `--boundary`로 줍니다. 경계 문서는 각 검사가 무엇을 관찰하고, 어떤 입력을 다루지 않는지 적은 문서입니다. 문서는 패킷의 `## Check boundaries` 절에 들어갑니다. 이 절은 리뷰어에게 두 가지를 안내합니다.

- 경계 밖 입력의 반례 가운데 어떤 기준·규칙·계약·보호된 회귀 경로도 요구하지 않는 것은, 그 경계를 밝혀 `check_impact` `none`인 `hardening`으로 보고합니다.
- 기준·계약·보호된 회귀 경로가 요구하는 입력의 반례는, 경계가 무엇이라 하든 `defect`로 보고합니다.

spar는 경계 문서가 맞는지 확인하지 않습니다.

파일의 고정 글만 읽는 검사는 `--text-check`로 줍니다. 문서에서 토큰을 찾는 시험이 그런 검사입니다. 리뷰어는 그 검사가 맞는 파일의 맞는 고정 글을 읽는지 봅니다. 고정 글로는 동작을 증명할 수 없다는 지적은, 그것을 요구하는 기준·계약·보호 경로를 밝힌 `requirement-question`으로만 보고합니다.

### 앞선 지적과 처리

candidate 라운드의 패킷은 같은 series의 앞선 코드 라운드에서 나온 지적과, 처리가 기록된 지적의 마지막 처리를 보입니다. 리뷰어는 처리가 기록된 지적을, 기록된 처리가 답하지 못하는 실패 시나리오가 있을 때만 다시 보고합니다. plan 라운드는 앞선 지적을 받지 않습니다.

다음 라운드를 돌리기 전에 처리를 `spar triage`로 기록하면, 리뷰어가 그 처리를 보고 검토합니다. 처리는 패킷의 일부입니다. 그래서 처리를 바꾼 뒤 같은 base·head로 돌린 candidate 라운드는 새 라운드로 상한에 셉니다.

라운드마다 Git이 무시하는 새 폴더가 생기고, 그 안에 `round.json`, `packet.md`, `findings.json`, `ledger.jsonl`, `summary.md`가 생깁니다. `summary.md`의 지적 표는 주장을 자르지 않고 `Claim` 칸에 한 줄로 보입니다. 그래서 그 파일만 읽고도 지적을 분류하고 처리할 수 있습니다. Git이 추적할 폴더는 거부합니다. 패킷에 비공개 문서가 들어갈 수 있기 때문입니다([프로젝트 준비](#프로젝트-준비)).

<details>
<summary>라운드·입력 형식과 위치를 읽을 수 없는 지적</summary>

라운드 형식은 [라운드 계약](../contracts/review-round.md)에 있습니다. spar가 받는 입력의 형식은 [입력 형식 계약](../contracts/spar-input.md)의 표 하나에 있습니다. 리뷰어 결과의 위치·연결, 설정 값, 명령 인자가 그런 입력입니다. 형식 밖 입력이 어떻게 되는지도 그 표에 있습니다.

위치가 어떤 파일도 가리키지 않는 지적도 버리지 않습니다. 읽을 수 없는 위치로 표시하고, 자동 보류하지 않습니다.

</details>

### 리뷰어별 관점

plan과 candidate 라운드마다(누적 라운드 포함) 에이전트가 변경의 주된 위험을 고릅니다. 그리고 리뷰어마다 서로 다른 관점을 하나씩 줍니다. 위험보다 리뷰어가 많으면 나머지는 관점 없이 검토합니다. 관점에는 공격할 위험, 먼저 볼 곳, 보는 입장을 적습니다. 해결이 맞다는 주장이나 풀이 설명은 넣지 않습니다. 관점은 `spar run`에 `--standpoint NAME=TEXT`로 줍니다.

관점을 받은 리뷰어는 그 관점을 확인했는지 보고합니다. `spar gate`는 같은 패킷의 라운드 어디에서도 리뷰어가 완료하지 않은 관점을 `uncovered_standpoints`에 보입니다. gate를 보고할 때는 이 관점도 함께 알립니다. 관점은 각도, 분류, 처리, 상한, gate의 상태와 사유를 바꾸지 않습니다. 인자 형식, 거부 규칙과 기록되는 필드는 [라운드 계약](../contracts/review-round.md#2-packet-and-workspaces)에 있습니다.

## 지적과 gate

gate는 검토 기록이지 완료 근거가 아닙니다. Seal의 완료는 여전히 `ha done`에서만 나옵니다. `standard`·`strict` 목표에서는 Seal이 gate 파일을 `ha note <goal> review --gate <file>`로 기록합니다. 기록된 gate가 지금 HEAD와 목표 문서를 덮어야 `ha done`이 완료를 기록합니다([검토 gate](RULES.md#검토-gate)). 그래서 라운드에는 SPEC.md와, 목표 폴더 안에서 SPEC.md가 직접 또는 다른 링크된 문서를 거쳐 링크하는 Markdown 문서를 모두 `--criteria`로 줍니다.

### 지적의 분류

Spar는 지적을 `defect`, `requirement-question`, `hardening`, `note` 가운데 하나로 분류합니다. 요구 질문은 사용자의 말로만 닫힙니다. 검사 코드의 엣지 케이스 가운데 거짓 통과·결과 의미 뒤바뀜·이름을 댄 규칙 위반을 주장하지 않는 것은 `hardening`으로 보류됩니다. 끝내지 못한 리뷰어는 원인과 함께 보이고, 지적 0건으로 세지 않습니다.

### 가장 작은 수정 방향

리뷰어는 지적마다 `smallest_fix`에 가장 작은 수정 방향을 적습니다. 실패 시나리오가 맞게 나오게 하는 가장 작은 변경을, 패치가 아닌 문장으로 적습니다. 말할 수 없으면 비워 둘 수 있습니다.

방향은 `findings.json`에 남고, `summary.md`의 `## Smallest fixes`에 보입니다. 방향은 제안이라 분류·처리·gate에 쓰지 않습니다. 에이전트는 수정을 그 방향 안에서 시작합니다. 더 크게 고치면 그 까닭을 `spar triage … --note`로 남깁니다.

### gate의 상태

`spar gate`는 라운드들과 완료하려는 head를 읽고 `review-gate/v1` 파일을 씁니다. 상태는 다음 가운데 하나입니다.

- `complete`(종료 0)
- `degraded`(종료 3): 필수 리뷰어가 끝내지 못했고, 그 라운드에 사용자의 결정이 없을 때
- `blocked`(종료 4): 열린 지적, 상한 초과, 검토되지 않은 head가 있을 때

[gate 계약](../contracts/review-gate.md)을 보세요.

### 리뷰어 없이 진행하기

필수 리뷰어가 사용량 한도 같은 이유로 라운드를 끝내지 못할 수 있습니다. 이때 사용자가 그 리뷰어 없이 진행하라고 하면, 에이전트가 `spar waive <라운드> <리뷰어> --user-quote "…"`로 그 말을 그 라운드에 기록합니다. 사용자의 말 없이는 기록하지 않습니다.

결정이 기록된 리뷰어는 그 라운드의 `degraded` 사유가 되지 않습니다. gate 파일에서는 그 라운드의 `waived_reviewers`에 상태와 사용자의 말이 함께 보입니다. 결정은 그 라운드에만 적용됩니다. 완료한 필수 리뷰어가 하나도 없는 라운드는 결정이 있어도 `degraded`입니다.
