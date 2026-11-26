# 설치 안내

**한국어** · [English](INSTALL.en.md)

Jaekit을 처음 설치하거나, 새 버전으로 업데이트하거나, 제거할 때 읽는 안내입니다.

Jaekit은 터미널 프로그램 `ha`·`spar`와 에이전트 플러그인 Spec·Seal·Spar로 이루어집니다. 프로그램은 brew나 Release 파일로 설치합니다. 플러그인은 Codex나 Claude Code의 플러그인 명령으로 설치합니다. 둘 다 있어야 동작합니다.

이 문서는 현재 배포판 `v0.1.9` 기준입니다. macOS와 Linux의 arm64·amd64를 지원합니다. 아래 명령은 모두 **터미널에서** 실행합니다. 설치한 뒤 에이전트에게 보낼 말은 [첫 사용 안내](../README.ko.md#첫-작업-맡기기)에 있습니다.

- 처음 설치할 때: [기본 설치](#기본-설치)
- brew를 쓰지 않을 때: [Release 파일로 설치](#release-파일로-설치)
- Windows에서 Linux 도구를 쓸 때: [WSL2 준비 안내 — 실제 사용 미확인](WSL.md)
- 참조 생산자와 예시를 쓸 때: [참조 검사 가져오기](#참조-검사-가져오기)
- 새 버전으로 옮길 때: [업데이트](#업데이트)
- 배포판마다 들어 있는 구성 요소와 적용 범위: [배포판과 개발 조합](#배포판과-개발-조합)
- 그만 쓸 때: [제거](#제거)
- 저장소 checkout 경로로 등록해 쓰던 것을 옮길 때: [로컬 경로 등록에서 옮기기](#로컬-경로-등록에서-옮기기)
- Jaekit 자체를 고칠 때: [소스 빌드와 개발 등록](DEVELOPMENT.md)

## 기본 설치

Git 2.36 이상과 Codex 또는 Claude Code가 필요합니다. brew로 설치하려면 [Homebrew](https://brew.sh)도 준비합니다. 작업할 프로젝트는 Git으로 변경 이력을 관리하는 폴더여야 합니다. Jaekit 저장소를 clone(내 컴퓨터로 복사)할 필요는 없습니다.

> **Windows.** 현재 공개 릴리스의 Seal Core `ha`는 Windows를 지원하지 않습니다. 별도 후보 패키지는 [Windows 네이티브 후보 안내](WINDOWS.md)에서 다룹니다. Windows 11 x64와 PowerShell 5.1/7에서의 실제 설치, Core 실행, 호스트 동작은 아직 확인하지 않았습니다. 교차 빌드와 로컬 검사는 그 근거를 대신하지 않습니다.

> **WSL2.** 위 내용은 네이티브 Windows 실행에 관한 것입니다. WSL2의 Ubuntu 24.04 LTS x64에서 Linux 배포물을 쓰는 방법은 [별도 안내](WSL.md)에 정리했습니다. 실제 WSL2 설치와 실행, 에이전트 사용은 아직 확인하지 않았습니다. 일반 Linux에서 얻은 결과로 이를 대신하지 않습니다.

### ha 설치

```bash
brew install jgoneit/tap/jaekit
```

명령 블록은 하나씩 실행합니다. brew 설치가 끝나기 전에 다음 명령까지 붙여 넣으면, brew가 뒤의 줄을 읽어 버릴 수 있습니다.

brew는 `ha`와 `spar`를 함께 설치합니다. Release의 실행 파일을 받아 checksum(파일 내용 확인값)을 검사합니다. Go를 설치하거나 소스를 빌드하지는 않습니다. Homebrew가 없으면 [Release 파일로 설치](#release-파일로-설치)를 따릅니다.

### 버전 확인

```bash
ha --version
which -a ha
spar --version
which -a spar
```

`ha --version`은 `ha 0.1.9`를 출력해야 합니다. `spar --version`도 같은 버전인 `spar 0.1.9`를 출력해야 합니다.

`which -a ha`의 첫 줄이 실제로 실행되는 파일입니다. `spar`는 `which -a spar`의 첫 줄을 봅니다. PATH는 터미널이 명령을 찾는 폴더의 목록입니다. 목록의 앞쪽 폴더부터 찾습니다.

버전이 다르면 먼저 이 경로를 확인합니다. 예전에 `go install`로 만든 파일이 앞에 있으면, 그 파일을 지우거나 PATH 순서를 바꿉니다. 그 파일은 `go env GOBIN`이 가리키는 폴더에 있습니다. `go env GOBIN`이 비어 있으면 `$(go env GOPATH)/bin`에 있습니다.

명령을 찾지 못하고 `which -a ha`도 비어 있으면, 먼저 설치가 끝났는지 확인합니다. 그다음 설치한 폴더가 PATH에 있는지 봅니다. 에이전트도 `ha`를 찾을 수 있게 PATH를 설정한 뒤, Codex나 Claude Code를 새로 엽니다.

### Codex

먼저 [ha 설치](#ha-설치)와 [버전 확인](#버전-확인)을 마칩니다. 아래 설치 명령을 실행하려면 터미널용 [Codex(Codex CLI)](https://learn.chatgpt.com/docs/codex/cli)가 필요합니다. 플러그인은 Codex CLI와 데스크톱 앱에서 쓸 수 있습니다. Codex IDE 확장에서는 지원되지 않습니다.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.9
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

`@v0.1.9`는 marketplace(플러그인 목록)를 그 태그에 고정합니다. 그래서 main이 바뀌어도 이 배포판의 spec 0.1.16·seal 0.1.36·spar 0.1.9를 받습니다.

Codex를 새로 엽니다. 대화에 `$`를 입력하고 Jaekit의 `spec:spec` 또는 `seal:seal`을 고릅니다. 호출 이름은 `$spec:spec`과 `$seal:seal`입니다. 그다음 [첫 작업 맡기기](../README.ko.md#첫-작업-맡기기)를 따라 해 봅니다. 다른 플러그인과 헷갈리면 [문제 해결](USAGE.md#문제-해결)을 봅니다.

### Claude Code

먼저 [ha 설치](#ha-설치)와 [버전 확인](#버전-확인)을 마칩니다. 그다음 아래 명령으로 Claude Code에 플러그인을 설치합니다.

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.9
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
claude plugin list
```

`#v0.1.9`는 marketplace를 그 태그에 고정합니다. 마지막 목록에서 `spec@jaekit`, `seal@jaekit`, `spar@jaekit`가 enabled인지 봅니다. 다른 플러그인과 헷갈리면 [문제 해결](USAGE.md#문제-해결)을 봅니다.

Claude Code를 새로 엽니다. 대화에서 `/spec:spec <요청>`으로 목표를 정리합니다. Spec이 지금 구현할지 물을 때 "응"이라고 답하면 구현이 시작됩니다. 나중에 별도 메시지에서 `/seal:seal docs/specs/<goal>`로 시작해도 됩니다. 플러그인의 스킬은 `/플러그인-이름:스킬-이름`으로 호출합니다. 목표를 확인하고 이어 가는 방법은 [사용 안내](USAGE.md#명령-보내기)에 있습니다.

### Spar

Spar는 여러 리뷰어 CLI로 독립 검토 라운드를 돌립니다. 자세한 내용은 [Spar 안내](SPAR.md)를 보세요.

v0.1.7부터 `standard`·`strict` 목표에는 Spar 실행 파일과 Spar 플러그인이 필요합니다. v0.1.9의 Seal 0.1.36은 새 목표를 `run-rules/8`로 시작합니다. `run-rules/5` 이후로 시작한 `standard`·`strict` 목표는 기록된 Spar gate가 있어야 완료됩니다.

다음 목표에는 Spar가 필요 없습니다.

- `light` 목표
- `Assurance:` 줄이 없는 목표
- `run-rules/4` 이하로 시작한 목표

검토 없이 끝내려면 목표 문서를 고쳐 등급을 `light`로 낮춥니다. 이 수정은 사용자가 정합니다([검토 gate](RULES.md#검토-gate-run-rules5)). `run-rules/8` 목표에 `review` 조건이 있으면 그 기준도 바꿔야 합니다. 바꾸지 않으면 `review_tier_missing`으로 완료되지 않습니다.

> **이전 배포판.** v0.1.7의 Seal은 새 목표를 `run-rules/5`로 시작했습니다. v0.1.6 이하의 릴리스 파일, formula, marketplace 태그에는 Spar가 없습니다.

v0.1.7부터 Homebrew formula와 Release 파일은 `ha`와 `spar`를 함께 설치합니다. 두 프로그램의 버전은 같습니다. 다음 명령으로 확인합니다.

```bash
spar --version
which -a spar
```

`spar --version`은 `ha --version`과 같은 버전을 출력해야 합니다.

위 [Codex](#codex)와 [Claude Code](#claude-code)의 설치 명령은 Spar 플러그인도 설치합니다. spec과 seal만 설치했다면, v0.1.9에 고정한 marketplace에서 다음 명령으로 더합니다.

```bash
codex plugin add spar@jaekit
claude plugin install spar@jaekit
```

그다음 호스트를 새로 엽니다. 첫 라운드 전에 [리뷰어 설정](SPAR.md#리뷰어-설정)을 따라, 가지고 있는 CLI만 리뷰어로 적습니다. 적은 설정은 `spar config`로 확인합니다. `standard`·`strict` 목표의 검토 라운드는 Seal이 Spar로 돌립니다. 그 밖의 계획이나 변경에 독립 검토가 필요하면, Codex에서는 `$spar:spar`를, Claude Code에서는 `/spar:spar`를 씁니다.

## Release 파일로 설치

brew 대신 [v0.1.9 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.9)의 파일 `ha_0.1.9_<os>_<arch>.tar.gz`로 설치할 수 있습니다. `<os>`는 `darwin`(macOS) 또는 `linux`입니다. `<arch>`는 `arm64`(Apple Silicon 등) 또는 `amd64`(Intel 등)입니다.

아래 명령은 고치지 않고 그대로 붙여 넣습니다. 명령은 OS와 CPU를 보고 자기 플랫폼의 파일을 받습니다. 받은 파일의 checksum을 확인한 뒤, `ha`와 `spar`를 `~/.local/bin/ha`와 `~/.local/bin/spar`에 둡니다. `spar`는 v0.1.7 파일부터 들어 있습니다.

지원하지 않는 OS나 CPU에서는 아무것도 받거나 설치하지 않고 멈춥니다. 압축을 푼 파일에 `spar`가 없으면, 아무것도 바꾸지 않고 메시지를 보인 뒤 멈춥니다.

```bash
(
  set -e
  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) echo "지원하지 않는 OS입니다: $(uname -s)" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) echo "지원하지 않는 CPU입니다: $(uname -m)" >&2; exit 1 ;;
  esac
  name="ha_0.1.9_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.9"
  tmp="$(mktemp -d)"
  binary_tmp=
  spar_tmp=
  trap 'rm -rf "$tmp"; if [ -n "$binary_tmp" ]; then rm -f "$binary_tmp"; fi; if [ -n "$spar_tmp" ]; then rm -f "$spar_tmp"; fi' EXIT
  cd "$tmp"
  curl -fsSLO "$url/$name.tar.gz"
  curl -fsSLO "$url/checksums.txt"
  grep " $name.tar.gz\$" checksums.txt > "$name.sha256"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c "$name.sha256"
  else
    shasum -a 256 -c "$name.sha256"
  fi
  tar -xzf "$name.tar.gz"
  if [ ! -f "$name/spar" ] || [ -L "$name/spar" ]; then
    echo "The archive has no spar executable; nothing was replaced: $name.tar.gz" >&2
    exit 1
  fi
  mkdir "$tmp/public"
  cp -R "$name/tools" "$name/examples" "$name/guides" "$name/contracts" "$name/assets" "$tmp/public/"
  cp "$name/README.md" "$name/README.ko.md" "$name/LICENSE" "$tmp/public/"
  share="$HOME/.local/share/jaekit/0.1.9"
  if [ -e "$share" ] || [ -L "$share" ]; then
    if [ ! -d "$share" ] || [ -L "$share" ]; then
      echo "Release data path is not an ordinary directory: $share" >&2
      exit 1
    fi
    special="$(find "$share" ! -type f ! -type d -print)"
    if [ -n "$special" ]; then
      echo "Release data contains entries that are not ordinary files or directories; nothing was replaced:" >&2
      printf '%s\n' "$special" >&2
      exit 1
    fi
    if ! diff -qr "$tmp/public" "$share"; then
      echo "Existing v0.1.9 data differs; nothing was replaced: $share" >&2
      exit 1
    fi
  else
    mkdir -p "$HOME/.local/share/jaekit"
    mv "$tmp/public" "$share"
  fi
  bin="$HOME/.local/bin"
  mkdir -p "$bin"
  if [ -e "$bin/ha" ] || [ -L "$bin/ha" ]; then
    if [ ! -f "$bin/ha" ] || [ -L "$bin/ha" ]; then
      echo "Existing ha path is not an ordinary file; nothing was replaced: $bin/ha" >&2
      exit 1
    fi
  fi
  if [ -e "$bin/spar" ] || [ -L "$bin/spar" ]; then
    if [ ! -f "$bin/spar" ] || [ -L "$bin/spar" ]; then
      echo "Existing spar path is not an ordinary file; nothing was replaced: $bin/spar" >&2
      exit 1
    fi
  fi
  binary_tmp="$(mktemp "$bin/.ha.XXXXXX")"
  cp "$name/ha" "$binary_tmp"
  chmod 755 "$binary_tmp"
  spar_tmp="$(mktemp "$bin/.spar.XXXXXX")"
  cp "$name/spar" "$spar_tmp"
  chmod 755 "$spar_tmp"
  mv -f "$binary_tmp" "$bin/ha"
  binary_tmp=
  echo "설치했습니다: $HOME/.local/bin/ha ($name)"
  mv -f "$spar_tmp" "$bin/spar"
  spar_tmp=
  echo "설치했습니다: $HOME/.local/bin/spar ($name)"
)
```

명령은 두 실행 파일을 모두 설치 위치 옆에 준비한 뒤에 바꿉니다. 그래서 받기, 복사, 권한 설정 중 하나가 실패하면 두 파일 모두 그대로 남습니다. 다만 두 파일을 바꾸는 일은 따로따로 일어납니다. 그 사이에 명령이 멈추면(강제 종료, 전원 차단 등) `ha`만 새 버전일 수 있습니다. 같은 명령을 다시 실행하면 두 파일의 버전이 맞춰집니다.

`~/.local/bin`이 PATH에 없으면 셸 설정 파일(`~/.zshrc` 등)에 `export PATH="$HOME/.local/bin:$PATH"`를 더합니다. 브라우저로 직접 받은 파일을 macOS가 막으면 `xattr -d com.apple.quarantine ~/.local/bin/ha ~/.local/bin/spar`로 풉니다. 그다음 [버전 확인](#버전-확인)을 하고 [Codex](#codex) 또는 [Claude Code](#claude-code)에 플러그인을 설치합니다.

## 배포판과 개발 조합

| 구분 | Core | Spec | Seal | Spar |
| --- | --- | --- | --- | --- |
| 현재 배포판 `v0.1.9` | `ha 0.1.9`, `spar 0.1.9` | 0.1.16 | 0.1.36 | 0.1.9 |
| 이전 배포판 `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |
| 이전 배포판 `v0.1.7` | `ha 0.1.7`, `spar 0.1.7` | 0.1.13 | 0.1.22 | 0.1.3 |
| 이전 배포판 `v0.1.6` | `ha 0.1.6` | 0.1.12 | 0.1.17 | — |
| 이전 배포판 `v0.1.5` | `ha 0.1.5` | 0.1.12 | 0.1.16 | — |
| 이전 배포판 `v0.1.4` | `ha 0.1.4` | 0.1.12 | 0.1.12 | — |
| 이전 배포판 `v0.1.3` | `ha 0.1.3` | 0.1.11 | 0.1.9 | — |
| 이전 배포판 `v0.1.2` | `ha 0.1.2` | 0.1.10 | 0.1.8 | — |
| 소스 빌드 | `ha 0.1.9-dev`, `spar 0.1.9-dev` | checkout의 버전 | checkout의 버전 | checkout의 버전 |

배포판마다 들어 있는 기능은 다음과 같습니다.

**현재 배포판 `v0.1.9`.** v0.1.8의 범위에 다음이 더해졌습니다.

- Seal이 새 목표를 `/8`로 시작합니다. `run-rules/6`~`/8`에서는 표시한 change 조건만 baseline이 필요합니다. 검사 동결은 없습니다. 경과 시간은 참고값입니다. `review` 조건은 검토 gate로 충족합니다.
- `PROGRESS.md`에 `## 검수 요약`이 들어갑니다.
- `ha report`가 검토 지적을 보여 줍니다.
- Spar 상한 보고 `spar cap-report`가 생겼습니다. `spar extend --cause`는 상한을 연장한 원인을 남깁니다. `ha cost`는 그 원인을 보여 줍니다.
- 사용자 설정이 비워 둔 리뷰어의 모델과 강도는 에이전트가 고릅니다. 사용자는 허용 목록으로 그 범위를 묶을 수 있습니다.
- Spar 리뷰어에게 관점이 주어집니다.
- candidate 라운드가 이전 처리를 받습니다.
- 리뷰어마다 패킷 크기 한도(`max_packet_bytes`)가 있습니다.

다음은 바뀌거나 빠졌습니다.

- `ha history`가 없어졌습니다.
- `spar run --stage plan-delta`와 `fixes`는 사용 오류(64)입니다.
- 기본 code 라운드 상한이 3에서 2로 줄었습니다.

**소스 빌드.** 개발용 식별자입니다. 배포 파일을 설치했다는 근거가 아닙니다.

<details>
<summary>이전 배포판에 들어 있는 기능</summary>

- **이전 배포판 `v0.1.8`**: v0.1.7의 범위에 더해 `ha cost`(목표 하나의 검증·검토 비용 요약), Seal이 미리 알 수 있는 결정을 첫 baseline 전에 한 메시지로 묻기, Spar 지적의 가장 작은 수정 방향, `spar`가 없으면 멈추는 직접 설치
- **이전 배포판 `v0.1.7`**: 기본 규칙 `/3`, Seal의 새 목표는 `/5`(`standard`·`strict` 목표는 기록된 Spar gate가 있어야 완료), Spar 검토 라운드와 gate, 러너 어댑터, `ha report`·`ha doctor`·자동 예산, 시작 때 Core 지원 확인(`--check-skill`), 에이전트가 부르는 Spec
- **이전 배포판 `v0.1.6`**: 기본 규칙 `/3`, Seal의 새 목표는 `/4`(기록된 권한 안의 자율 처리), 실행 시간 보기, 검사 작성 안내, 최종 후보 근거, 완료 뒤 발견·검증 근거와 보증 정책 보고; Spar·`/5` 없음
- **이전 배포판 `v0.1.5`**: v0.1.6과 같은 Core 동작; `/4` 재개 창의 횟수 상한을 계약에 연결하기 전의 Seal
- **이전 배포판 `v0.1.4`**: 기본 규칙 `/3`, 완료 뒤 발견·검증 근거와 보증 정책 보고; `/4`·실행 시간 보기 없음
- **이전 배포판 `v0.1.3`**: 기본 규칙 `/3`, 지원 조회·횟수 추정·총상한 변경; 완료 뒤 발견 확장 없음
- **이전 배포판 `v0.1.2`**: 기본 규칙 `/2`; 새 Seal의 지원 요구를 충족하지 않음

</details>

이 문서의 태그 고정 설치 명령은 v0.1.9를 받습니다. Seal은 실제로 쓸 Core의 지원 정보를 확인합니다. 지원 정보를 확인할 수 없거나 지원이 부족하면, 기록하기 전에 멈춥니다. 버전 숫자만 맞추거나 이전 규칙으로 바꿔서 진행하지 않습니다. 지원 조건은 [Core 지원 정보 계약](../contracts/core-capabilities.md)에 있습니다.

Spec은 Core를 설치하거나 지원 정보를 조회하지 않고도 문서를 씁니다. Spec 0.1.16이 새로 쓰는 문서는 `nested/1` 형식입니다. 그래서 구현을 맡길 때는 이 형식을 지원하는 Core가 필요합니다. 업데이트는 기존 목표의 형식, `/1`·`/2`·`/3` 기록, 사용량과 시간 한도를 바꾸지 않습니다. 실제 호스트에서 확인한 범위는 [v0.1.9 릴리스 노트](https://github.com/jgoneit/jaekit/releases/tag/v0.1.9)를 따릅니다.

## 참조 검사 가져오기

v0.1.9 배포 파일에는 `tools/check-result-reference.py`와 `examples/check-result/`가 들어 있습니다. 개발 checkout이나 Go 설치 없이 자기 프로젝트로 복사할 수 있습니다. 이 생산자는 Python 3 표준 라이브러리만 씁니다. Python은 이 검사를 고를 때만 필요합니다. Spec과 Seal의 필수 조건은 아닙니다. 다른 러너는 구현자가 고릅니다. pytest, Vitest, Playwright 자동 연동은 제공하지 않습니다.

먼저 자료 경로를 정합니다. 프로젝트 루트에서 설치 방식에 맞는 **한 블록만** 실행합니다. Homebrew로 설치했다면 다음 `pkgshare` 위치를 씁니다.

```bash
package_root="$(brew --prefix jaekit)/share/jaekit"
```

위의 직접 설치 명령은 checksum을 확인한 공개 자료를 아래 버전별 폴더에 보관합니다. 같은 버전의 자료가 이미 있으면, 일반 파일과 폴더로만 이루어지고 내용이 같을 때만 재사용합니다. 내용이 다르거나 symlink 같은 다른 항목이 있으면, 바이너리를 바꾸기 전에 멈추고 그 경로를 알려 줍니다. 직접 설치했다면 다음을 씁니다.

```bash
package_root="$HOME/.local/share/jaekit/0.1.9"
```

그다음 같은 터미널의 프로젝트 루트에서 다음을 실행합니다. 예시 파일이 이미 있으면 덮어쓰지 않고 멈춥니다.

```bash
(
  set -e
  for file in tools/check-result-reference.py checks/declaration.json checks/reference.json sample/input.txt; do
    if [ -e "$file" ] || [ -L "$file" ]; then
      echo "File already exists; choose other paths before copying: $file" >&2
      exit 1
    fi
  done
  mkdir -p tools checks sample
  cp "$package_root/tools/check-result-reference.py" tools/
  cp "$package_root/examples/check-result/declaration.json" checks/
  cp "$package_root/examples/check-result/reference.json" checks/
  cp "$package_root/examples/check-result/input.txt" sample/
)
```

이어서 [합성 예시의 PLAN 연결과 관측 순서](../examples/check-result/README.md)를 따릅니다. 초기 파일을 기준 상태로 보존한 뒤 목표를 시작합니다.

검사 명령은 `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json`입니다. 호출마다 결과 보고 경로는 `ha check`가 전달합니다. 이 검사는 초기 파일의 실제 요구 위반과 수정 뒤의 통과를 구분합니다. 환경 오류는 기대한 실패로 삼지 않습니다.

선언, 설정, 생산자는 검사 전에 commit합니다. 셋 다 PLAN의 검사 경로에 넣습니다. 요구 대상과 경로는 프로젝트에 맞게 정합니다. 세부 형식은 [결과 계약](../contracts/check-result.md)에 있습니다.

## 업데이트

`ha`와 플러그인을 함께 새 버전으로 옮깁니다. 설치할 때 쓴 도구로 올립니다. 아래는 v0.1.9로 옮기는 명령입니다. 명령 블록은 하나씩 붙여 넣습니다.

brew로 설치한 `ha`는 brew가 tap을 새로 받은 뒤에 올립니다.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

Release 파일로 설치한 `ha`는 [Release 파일로 설치](#release-파일로-설치)의 명령을 다시 붙여 넣습니다. 새 버전의 파일이 `~/.local/bin/ha`를 덮어씁니다. `~/.local/bin/spar`에도 새 파일을 둡니다.

그다음 두 실행 파일의 버전과, 실제로 실행되는 위치를 확인합니다. `ha 0.1.9`와 `spar 0.1.9`가 나오면 됩니다. 다른 버전이 나오면 [버전 확인](#버전-확인)대로 PATH를 봅니다. `which -a ha`와 `which -a spar`의 첫 줄이 실제로 실행되는 파일입니다.

```bash
ha --version
which -a ha
spar --version
which -a spar
```

<details>
<summary>선택, Release 파일 설치만: 이전 버전 자료 지우기</summary>

Release 파일로 설치했다면 이전 버전의 공개 자료가 `~/.local/share/jaekit/<버전>`에 그대로 남습니다. 위에서 `ha 0.1.9`를 확인한 뒤 아래 명령으로 설치한 버전 `0.1.9`보다 오래된 버전 폴더만 지울 수 있습니다. 자료 폴더는 v0.1.3부터 생겼습니다.
- 현재 버전 폴더 `~/.local/share/jaekit/0.1.9`와 [참조 검사 가져오기](#참조-검사-가져오기)에 쓰는 파일, 더 새 버전 폴더, `0.1.2`처럼 세 숫자로 된 버전 이름이 아닌 항목은 남습니다.
- 이름은 줄바꿈이나 공백으로 나누지 않고 전체를 판단하며, 버전처럼 보이는 일반 파일과 symlink도 남깁니다.
- 버전은 숫자로 비교하므로 `0.1.10`은 `0.1.9`보다 새 버전입니다.
- `~/.local/share/jaekit`이 symlink이면 아무것도 지우지 않습니다.
- 열거·버전 비교·삭제가 실패하면 명령은 실패로 종료하며, 그 전에 정상적으로 삭제한 구버전 폴더를 복원하지는 않습니다.

```bash
(
  set -e
  current=0.1.9
  data_root="$HOME/.local/share/jaekit"
  if [ -d "$data_root" ] && [ ! -L "$data_root" ]; then
    find "$data_root" -mindepth 1 -maxdepth 1 -type d -exec sh -c '
      set -e
      current=$1
      shift
      for entry do
        [ -d "$entry" ] && [ ! -L "$entry" ] || continue
        version=${entry##*/}
        case "$version" in "" | *[!0-9.]*) continue ;; esac
        older=$(awk -v version="$version" -v current="$current" "
          BEGIN {
            if (version !~ /^[0-9]+[.][0-9]+[.][0-9]+$/) { print 0; exit }
            split(version, v, /[.]/)
            split(current, c, /[.]/)
            for (i = 1; i <= 3; i++) {
              if (v[i] + 0 != c[i] + 0) { print (v[i] + 0 < c[i] + 0); exit }
            }
            print 0
          }")
        case "$older" in
          1) rm -rf "$entry" ;;
          0) ;;
          *) echo "Could not compare version: $version" >&2; exit 1 ;;
        esac
      done
    ' sh "$current" {} +
  fi
)
```

</details>

플러그인은 marketplace를 새 태그에 고정해 다시 등록합니다. 같은 이름의 marketplace는 둘을 함께 둘 수 없으므로 먼저 지웁니다. 지우면 그 marketplace에서 설치한 spec·seal·spar도 함께 지워지므로 다시 설치합니다. 쓰는 호스트의 명령만 붙여 넣으면 됩니다.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.9
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.9
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

호스트를 새로 열면 새 버전의 Spec·Seal·Spar가 실립니다.

## 제거

`ha`와 플러그인, `jaekit` marketplace를 지웁니다. 설치한 방법과 쓰는 호스트에 맞는 명령을 붙여 넣습니다.

brew로 설치한 `ha`:

```bash
brew uninstall jgoneit/tap/jaekit
```

tap도 더 쓰지 않으면 `brew untap jgoneit/tap`으로 지웁니다.

Release 파일로 설치한 `ha`:

```bash
rm ~/.local/bin/ha
```

v0.1.7부터는 직접 설치가 `spar`도 함께 두었습니다. 이것도 지웁니다.

```bash
rm ~/.local/bin/spar
```

직접 설치가 공개 자료를 보관한 폴더도 지웁니다. 자기 프로젝트에 복사한 참조 검사 파일(`tools/check-result-reference.py` 등)은 남습니다. 목표 문서와 기록도 남습니다. `~/.local/share/jaekit`이 symlink이면 link만 지우고, link가 가리키는 내용은 남깁니다.

```bash
rm -rf ~/.local/share/jaekit
```

Codex:

```bash
codex plugin remove spec@jaekit
codex plugin remove seal@jaekit
codex plugin remove spar@jaekit
codex plugin marketplace remove jaekit
```

Claude Code:

```bash
claude plugin uninstall spec@jaekit
claude plugin uninstall seal@jaekit
claude plugin uninstall spar@jaekit
claude plugin marketplace remove jaekit
```

설치, 업데이트, 제거는 작업한 프로젝트의 문서, 기록, 출력 원문을 지우지 않습니다.

목표 문서와 Seal이 만든 파일은 `docs/specs/<goal>/`의 `SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`입니다. 이 파일을 Git이 추적할지는 프로젝트 정책에 따릅니다. Git이 추적하는 기록도, ignore한 비공개 기록도 모두 그대로 남습니다.

검사 출력 원문은 그 작업 트리의 Git 디렉토리 아래 `ha/runs/`에 있습니다. git은 이 폴더를 추적하지 않습니다. 보통은 `.git/ha/` 아래입니다. `.git`이 파일인 linked worktree에서는 `git rev-parse --absolute-git-dir`이 알려 주는 디렉토리 아래에 있습니다.

목표 문서와 기록도 지우기로 했다면, 먼저 보관해야 하는지와 Git 추적 상태를 확인합니다. Git이 추적하는 목표는 그 저장소의 루트에서 `git rm -r docs/specs/<goal>`로 지운 뒤 commit합니다. 추적하지 않는 목표는 이 명령의 대상이 아닙니다. 프로젝트의 로컬 보관 정책에 따라 따로 정리합니다. ignore한 문서를 강제로 Git에 추가하지 않습니다.

검사 출력 원문도 지우려면 그 작업 트리에서 아래 명령을 붙여 넣습니다. Jaekit의 검사가 돌고 있지 않을 때, 곧 Seal이 작업 중이 아닐 때 실행합니다. 이 명령은 그 작업 트리에 있는 모든 목표의 검사 출력 원문을 지웁니다. 다른 작업 트리의 출력은 남습니다. 목표 문서와 실행 기록도 남습니다.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha/runs"
```

## 로컬 경로 등록에서 옮기기

저장소 checkout 경로로 `jaekit` marketplace를 등록해 두었다면, 그 등록을 지웁니다. 그다음 `v0.1.9`에 고정한 GitHub 저장소로 다시 등록합니다. 같은 이름의 marketplace는 둘을 함께 둘 수 없습니다. 지우면 그 marketplace에서 설치한 spec·seal·spar도 함께 지워지므로 다시 설치합니다.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.9
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.9
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

예전에 `go install`로 만든 `ha`가 남아 있으면 [버전 확인](#버전-확인)대로 정리합니다.
