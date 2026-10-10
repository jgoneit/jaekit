# 설치 안내

**한국어** · [English](INSTALL.en.md)

현재 배포판 `v0.1.3`의 설치·업데이트·제거 안내입니다. macOS·Linux의 arm64·amd64를 지원합니다. 아래 명령은 **터미널에서** 실행합니다. 설치 뒤 에이전트에게 보낼 말은 [첫 사용 안내](../README.md#첫-작업-맡기기)에 있습니다.

- 처음 설치할 때: [기본 설치](#기본-설치)
- brew를 쓰지 않을 때: [Release 파일로 설치](#release-파일로-설치)
- 참조 생산자와 예시를 쓸 때: [참조 검사 가져오기](#참조-검사-가져오기)
- 새 버전으로 옮길 때: [업데이트](#업데이트)
- 그만 쓸 때: [제거](#제거)
- 저장소 checkout 경로로 등록해 쓰던 것을 옮길 때: [로컬 경로 등록에서 옮기기](#로컬-경로-등록에서-옮기기)
- jaekit 자체를 고칠 때: [소스에서 빌드 (개발)](#소스에서-빌드-개발)

## 배포판과 개발 조합

| 구분 | Core | Spec | Seal | 적용 범위 |
| --- | --- | --- | --- | --- |
| 현재 배포판 `v0.1.3` | `ha 0.1.3` | 0.1.11 | 0.1.9 | 기본 규칙 `/3`, `nested/1`, 실행 전 dirty·복사 안전성·지원 확인, 횟수 추정·총상한 변경 |
| 이전 배포판 `v0.1.2` | `ha 0.1.2` | 0.1.10 | 0.1.8 | 기본 규칙 `/2`; 새 Seal의 지원 요구를 충족하지 않음 |
| 소스 빌드 | `ha 0.1.3-dev` | checkout의 버전 | checkout의 버전 | 개발용 식별자이며 배포 파일의 설치 근거가 아님 |

아래 태그 고정 설치 명령은 v0.1.3을 받습니다. Seal은 실제 사용할 Core의 지원 정보를 확인하고, 확인할 수 없거나 부족하면 기록 전에 멈춥니다. 버전 숫자만 맞추거나 구규칙으로 바꿔 진행하지 않습니다. 지원 조건은 [Core 지원 정보 계약](../contracts/core-capabilities.md)에 있습니다.

Spec은 Core 설치나 지원 조회 없이 문서를 작성합니다. Spec 0.1.11의 새 문서는 `nested/1`을 사용하므로 구현을 맡길 때는 그 형식을 지원하는 Core가 필요합니다. 기존 목표의 형식과 `/1`·`/2`·`/3` 기록, 사용량과 시간 한도는 업데이트로 바꾸지 않습니다. 실제 호스트 확인 범위는 [v0.1.3 릴리스 노트](https://github.com/jgoneit/jaekit/releases/tag/v0.1.3)에 따릅니다.

## 기본 설치

Git과 Codex 또는 Claude Code가 필요합니다. brew로 설치하려면 [Homebrew](https://brew.sh)도 준비합니다. 작업할 프로젝트는 Git으로 변경 이력을 관리하는 폴더여야 합니다. Jaekit 저장소를 clone(내 컴퓨터로 복사)할 필요는 없습니다.

Windows에서는 Seal Core `ha`가 지원되지 않아 Seal을 쓸 수 없습니다. Spec은 `ha` 없이 목표 문서를 만들 수 있지만 Windows에서의 설치·동작은 아직 확인하지 않았습니다.

### ha 설치

```bash
brew install jgoneit/tap/jaekit
```

블록을 하나씩 실행합니다. brew 설치가 끝나기 전에 다음 명령까지 붙여 넣으면 뒤의 줄을 읽어 버릴 수 있습니다. brew는 Release의 실행 파일을 받아 checksum(파일 내용 확인값)을 검사합니다. Go를 설치하거나 소스를 빌드하지 않습니다. Homebrew가 없으면 [Release 파일로 설치](#release-파일로-설치)를 따릅니다.

### 버전 확인

```bash
ha --version
which -a ha
```

`ha --version`은 `ha 0.1.3`를 출력해야 합니다. `which -a ha`의 첫 줄이 실제로 실행되는 파일입니다. PATH는 명령을 찾을 폴더의 순서입니다. 버전이 다르면 먼저 이 경로를 확인합니다. 예전에 `go install`로 만든 파일이 앞에 있다면 해당 파일을 지우거나 PATH 순서를 바꿉니다. 그 파일은 `go env GOBIN`이 가리키는 폴더, 비어 있으면 `$(go env GOPATH)/bin`에 있습니다.

명령을 못 찾고 `which -a ha`도 비어 있다면 설치가 끝났는지 확인하고, 설치한 폴더가 PATH에 있는지 봅니다. 에이전트도 `ha`를 찾을 수 있도록 설정한 뒤 Codex 또는 Claude Code를 새로 엽니다.

### Codex

먼저 [ha 설치](#ha-설치)와 [버전 확인](#버전-확인)을 마칩니다. 아래 설치 명령에는 터미널용 [Codex(Codex CLI)](https://learn.chatgpt.com/docs/codex/cli)가 필요합니다. 플러그인은 Codex CLI와 데스크톱 앱에서 사용할 수 있으며, Codex IDE 확장에서는 지원되지 않습니다.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

`@v0.1.3`는 marketplace(플러그인 목록)를 해당 태그에 고정합니다. main이 바뀌어도 배포판의 spec 0.1.11·seal 0.1.9을 받습니다.

Codex를 새로 열고 대화에 `$`를 입력해 Jaekit의 `spec:spec` 또는 `seal:seal`을 고릅니다. 호출 이름은 `$spec:spec`과 `$seal:seal`입니다. [첫 작업 맡기기](../README.md#첫-작업-맡기기)를 따라 해 봅니다. 다른 플러그인과 혼동되면 [문제 해결](USAGE.md#문제-해결)을 봅니다.

### Claude Code

먼저 [ha 설치](#ha-설치)와 [버전 확인](#버전-확인)을 마칩니다. 그다음 아래 명령으로 Claude Code에 플러그인을 설치합니다.

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.3
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin list
```

`#v0.1.3`는 marketplace를 해당 태그에 고정합니다. 마지막 목록에서 `spec@jaekit`, `seal@jaekit`가 enabled인지 봅니다. 다른 플러그인과 혼동되면 [문제 해결](USAGE.md#문제-해결)을 봅니다.

Claude Code를 새로 열고 대화에서 `/spec:spec <요청>`으로 목표를 정리한 뒤, 별도 메시지의 `/seal:seal docs/specs/<goal>`로 구현을 시작합니다. 플러그인의 스킬은 `/플러그인-이름:스킬-이름`으로 호출합니다. 목표 확인과 이어 가기는 [사용 안내](USAGE.md#명령-보내기)에 있습니다.

## Release 파일로 설치

brew 대신 [v0.1.3 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.3)의 파일 `ha_0.1.3_<os>_<arch>.tar.gz`로 설치할 수 있습니다. `<os>`는 `darwin`(macOS) 또는 `linux`, `<arch>`는 `arm64`(Apple Silicon 등) 또는 `amd64`(Intel 등)입니다.

아래 명령은 고치지 않고 그대로 붙여 넣습니다. OS와 CPU를 보고 자기 플랫폼 파일을 받아 checksum을 확인한 뒤 `~/.local/bin/ha`에 둡니다. 지원하지 않는 OS나 CPU에서는 아무것도 받거나 설치하지 않고 멈춥니다.

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
  name="ha_0.1.3_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.3"
  tmp="$(mktemp -d)"
  binary_tmp=
  trap 'rm -rf "$tmp"; if [ -n "$binary_tmp" ]; then rm -f "$binary_tmp"; fi' EXIT
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
  mkdir "$tmp/public"
  cp -R "$name/tools" "$name/examples" "$name/guides" "$name/contracts" "$name/assets" "$tmp/public/"
  cp "$name/README.md" "$name/README.en.md" "$name/LICENSE" "$tmp/public/"
  share="$HOME/.local/share/jaekit/0.1.3"
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
      echo "Existing v0.1.3 data differs; nothing was replaced: $share" >&2
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
  binary_tmp="$(mktemp "$bin/.ha.XXXXXX")"
  cp "$name/ha" "$binary_tmp"
  chmod 755 "$binary_tmp"
  mv -f "$binary_tmp" "$bin/ha"
  binary_tmp=
  echo "설치했습니다: $HOME/.local/bin/ha ($name)"
)
```

`~/.local/bin`이 PATH에 없으면 셸 설정(`~/.zshrc` 등)에 `export PATH="$HOME/.local/bin:$PATH"`를 더합니다. 브라우저로 직접 받은 파일을 macOS가 막으면 `xattr -d com.apple.quarantine ~/.local/bin/ha`로 풉니다. 그다음 [버전 확인](#버전-확인)과 [Codex](#codex) 또는 [Claude Code](#claude-code) 설치로 갑니다.

## 참조 검사 가져오기

v0.1.3 배포 파일에는 `tools/check-result-reference.py`와 `examples/check-result/`가 들어 있습니다. 개발 checkout이나 Go 설치 없이 자기 프로젝트로 복사할 수 있습니다. 이 생산자는 Python 3 표준 라이브러리만 사용합니다. Python은 이 검사를 선택할 때만 필요하며 Spec·Seal의 필수 조건이 아닙니다. 다른 러너는 구현자가 선택합니다. pytest·Vitest·Playwright 자동 연동은 제공하지 않습니다.

프로젝트 루트에서 설치 방식에 맞는 **한 블록만** 실행해 자료 경로를 정합니다. Homebrew의 `pkgshare` 위치는 다음과 같습니다.

```bash
package_root="$(brew --prefix jaekit)/share/jaekit"
```

위의 직접 설치 명령은 checksum을 확인한 공개 자료를 다음 버전별 폴더에 보관합니다. 같은 버전의 자료가 이미 있으면 일반 파일과 폴더로만 이루어지고 내용이 같을 때만 재사용합니다. 내용이 다르거나 symlink 같은 다른 항목이 있으면 바이너리를 교체하기 전에 멈추고 그 경로를 알려 줍니다. 직접 설치했다면 다음을 씁니다.

```bash
package_root="$HOME/.local/share/jaekit/0.1.3"
```

이어 같은 터미널의 프로젝트 루트에서 다음을 실행합니다. 예시 파일이 이미 있으면 덮어쓰지 않고 멈춥니다.

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

[합성 예시의 PLAN 연결과 관측 순서](../examples/check-result/README.md)를 따라 초기 파일을 기준 상태로 보존하고 목표를 시작합니다. 명령은 `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json`이며 `ha check`가 호출별 결과 보고 경로를 전달합니다. 초기 파일의 실제 요구 위반과 수정 뒤 통과를 구분하며, 환경 오류를 기대 실패로 삼지 않습니다. 선언·설정·생산자는 검사 전에 commit하고 모두 PLAN의 검사 경로에 포함합니다. 요구 대상과 경로는 프로젝트에 맞게 정합니다. 세부 형식은 [결과 계약](../contracts/check-result.md)에 있습니다.

## 업데이트

`ha`와 두 plugin을 함께 새 버전으로 옮깁니다. 설치할 때 쓴 도구로 올립니다. 아래는 v0.1.3로 옮기는 명령입니다. 명령 블록은 하나씩 붙여 넣습니다.

brew로 설치한 `ha`는 brew가 tap을 새로 받은 뒤 올립니다.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

Release 파일로 설치한 `ha`는 [Release 파일로 설치](#release-파일로-설치)의 명령을 다시 붙여 넣습니다. 새 버전의 파일이 `~/.local/bin/ha`를 덮어씁니다.

그다음 버전을 확인합니다. `ha 0.1.3`가 나오면 됩니다. 다른 버전이 나오면 [버전 확인](#버전-확인)대로 `which -a ha`를 봅니다.

```bash
ha --version
```

Release 파일로 설치했다면 이전 버전의 공개 자료가 `~/.local/share/jaekit/<버전>`에 그대로 남습니다. 위에서 `ha 0.1.3`를 확인한 뒤 아래 명령으로 이전 버전 자료만 지울 수 있습니다. 현재 버전 폴더 `~/.local/share/jaekit/0.1.3`와 [참조 검사 가져오기](#참조-검사-가져오기)에 쓰는 파일은 남습니다. `~/.local/share/jaekit`이 symlink이면 아무것도 지우지 않습니다.

```bash
find ~/.local/share/jaekit -mindepth 1 -maxdepth 1 ! -name 0.1.3 -exec rm -rf {} +
```

plugin은 marketplace를 새 태그에 고정해 다시 등록합니다. 같은 이름의 marketplace는 둘을 함께 둘 수 없어서 먼저 지웁니다. 지우면 그 marketplace에서 설치한 spec·seal도 함께 지워지므로 다시 설치합니다. 쓰는 host의 명령만 붙여 넣으면 됩니다.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.3
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

host를 새로 열면 새 버전의 Spec과 Seal이 실립니다.

## 제거

`ha`와 plugin, `jaekit` marketplace를 지웁니다. 설치한 방법과 쓰는 host에 맞는 명령을 붙여 넣습니다.

brew로 설치한 `ha`:

```bash
brew uninstall jgoneit/tap/jaekit
```

tap도 더 쓰지 않으면 `brew untap jgoneit/tap`으로 지웁니다.

Release 파일로 설치한 `ha`:

```bash
rm ~/.local/bin/ha
```

직접 설치가 공개 자료를 보관한 폴더도 지웁니다. 자기 프로젝트에 복사한 참조 검사 파일(`tools/check-result-reference.py` 등)과 목표 문서·기록은 남습니다. `~/.local/share/jaekit`이 symlink이면 link만 지우고 가리키는 내용은 남깁니다.

```bash
rm -rf ~/.local/share/jaekit
```

Codex:

```bash
codex plugin remove spec@jaekit
codex plugin remove seal@jaekit
codex plugin marketplace remove jaekit
```

Claude Code:

```bash
claude plugin uninstall spec@jaekit
claude plugin uninstall seal@jaekit
claude plugin marketplace remove jaekit
```

설치·업데이트·제거는 작업한 프로젝트의 문서·기록·출력 원문을 지우지 않습니다. 목표 문서와 Seal이 만든 파일(`docs/specs/<goal>/`의 `SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`)의 Git 추적 여부는 프로젝트 정책에 따릅니다. 추적된 기록과 ignore한 비공개 기록 모두 그대로 남습니다. 검사 출력 원문은 git이 추적하지 않는, 그 작업 트리의 Git 디렉토리 아래 `ha/`에 있습니다. 보통은 `.git/ha/`이고, `.git`이 파일인 linked worktree에서는 `git rev-parse --absolute-git-dir`이 알려 주는 디렉토리 아래에 있습니다.

목표 문서와 기록도 지우기로 했다면 먼저 보관 필요 여부와 Git 추적 상태를 확인합니다. 추적 중인 목표는 그 저장소의 루트에서 `git rm -r docs/specs/<goal>`로 지운 뒤 commit합니다. 추적하지 않는 목표는 이 명령의 대상이 아니므로 프로젝트의 로컬 보관 정책에 따라 별도로 정리합니다. ignore한 문서를 강제로 Git에 추가하지 않습니다.

검사 출력 원문도 지우려면 Jaekit의 검사가 돌고 있지 않을 때(Seal이 작업 중이 아닐 때) 그 작업 트리에서 아래 명령을 붙여 넣습니다. 그 작업 트리에 있는 모든 목표의 검사 출력 원문이 지워집니다. 다른 작업 트리의 출력과 목표 문서·실행 기록은 남습니다.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha"
```

## 로컬 경로 등록에서 옮기기

이미 저장소 checkout 경로로 `jaekit` marketplace를 등록했으면, 그 등록을 지우고 `v0.1.3`에 고정한 GitHub 저장소로 다시 등록합니다. 같은 이름의 marketplace는 둘을 함께 둘 수 없습니다. 지우면 그 marketplace에서 설치한 spec·seal도 함께 지워지므로 다시 설치합니다.

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.3
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.3
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

예전에 `go install`로 만든 `ha`가 남아 있으면 [버전 확인](#버전-확인)대로 정리합니다.

## 소스에서 빌드 (개발)

저장소 checkout에서 빌드한 `ha`는 개발 버전(`ha 0.1.3-dev`)을 표시합니다. Go 1.26 이상과 git이 필요합니다. checkout의 루트에서 실행합니다.

```bash
go install ./cmd/ha
ha --version
```

`ha` 파일은 `go env GOBIN`이 가리키는 폴더에 생기고, 그 값이 비어 있으면 `$(go env GOPATH)/bin`에 생깁니다. `ha --version`은 `ha 0.1.3-dev`를 출력합니다. Seal은 별도로 `ha capabilities --format json`의 실제 지원 정보를 확인합니다. 선택적인 Python 호환 검사 도우미도 있지만 Python 3 설치가 Seal의 필수 조건은 아닙니다.

checkout을 고치며 plugin을 쓰려면 checkout 경로를 marketplace로 등록합니다. 이 등록은 checkout의 plugin 파일을 그대로 씁니다. `<jaekit 경로>`는 checkout의 경로로 바꿉니다.

Codex:

```bash
codex plugin marketplace add <jaekit 경로>
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

Claude Code:

```bash
claude plugin marketplace add <jaekit 경로>
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

명령과 기록 형식은 [contracts/run-record.md](../contracts/run-record.md)에 있습니다.
