# 설치 안내

**한국어** · [English](INSTALL.en.md)

현재 배포판 `v0.1.2`의 설치·업데이트·제거 안내입니다. macOS·Linux의 arm64·amd64를 지원합니다. 아래 명령은 **터미널에서** 실행합니다. 설치 뒤 에이전트에게 보낼 말은 [첫 사용 안내](../README.md#첫-작업-맡기기)에 있습니다.

- 처음 설치할 때: [기본 설치](#기본-설치)
- brew를 쓰지 않을 때: [Release 파일로 설치](#release-파일로-설치)
- 새 버전으로 옮길 때: [업데이트](#업데이트)
- 그만 쓸 때: [제거](#제거)
- 저장소 checkout 경로로 등록해 쓰던 것을 옮길 때: [로컬 경로 등록에서 옮기기](#로컬-경로-등록에서-옮기기)
- jaekit 자체를 고칠 때: [소스에서 빌드 (개발)](#소스에서-빌드-개발)

## 기본 설치

Git과 Claude Code 또는 Codex가 필요합니다. brew로 설치하려면 [Homebrew](https://brew.sh)도 준비합니다. 작업할 프로젝트는 Git으로 변경 이력을 관리하는 폴더여야 합니다. Jaekit 저장소를 clone(내 컴퓨터로 복사)할 필요는 없습니다.

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

`ha --version`은 `ha 0.1.2`을 출력해야 합니다. `which -a ha`의 첫 줄이 실제로 실행되는 파일입니다. PATH는 명령을 찾을 폴더의 순서입니다. 버전이 다르면 먼저 이 경로를 확인합니다. 예전에 `go install`로 만든 파일이 앞에 있다면 해당 파일을 지우거나 PATH 순서를 바꿉니다. 그 파일은 `go env GOBIN`이 가리키는 폴더, 비어 있으면 `$(go env GOPATH)/bin`에 있습니다.

명령을 못 찾고 `which -a ha`도 비어 있다면 설치가 끝났는지 확인하고, 설치한 폴더가 PATH에 있는지 봅니다. 에이전트도 `ha`를 찾을 수 있도록 설정한 뒤 Claude Code 또는 Codex를 새로 엽니다.

### Claude Code

```bash
claude plugin marketplace add jgoneit/jaekit#v0.1.2
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin list
```

마지막 목록에서 `spec@jaekit`, `seal@jaekit`가 enabled인지 봅니다. 기존의 다른 `spec`·`seal` 플러그인이 설치되어 있으면 끕니다. Skill 이름이 겹치기 때문입니다([충돌 해결](USAGE.md#문제-해결)).

### Codex

먼저 [ha 설치](#ha-설치)와 [버전 확인](#버전-확인)을 마칩니다. 그다음 아래 명령으로 Codex에 플러그인을 설치합니다.

```bash
codex plugin marketplace add jgoneit/jaekit@v0.1.2
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

기존의 다른 `spec`·`seal` 플러그인이 있으면 끕니다([충돌 해결](USAGE.md#문제-해결)). Claude Code의 `#v0.1.2`와 Codex의 `@v0.1.2`는 플러그인을 제공하는 marketplace(플러그인 목록)를 해당 태그에 고정합니다. main이 바뀌어도 배포판의 spec 0.1.10·seal 0.1.8을 받습니다.

설치한 에이전트를 새로 열고 [첫 작업을 맡깁니다](../README.md#첫-작업-맡기기). host별 호출 방법은 [사용 안내](USAGE.md#명령-보내기)에 있습니다.

## Release 파일로 설치

brew 대신 [v0.1.2 Release](https://github.com/jgoneit/jaekit/releases/tag/v0.1.2)의 파일 `ha_0.1.2_<os>_<arch>.tar.gz`로 설치할 수 있습니다. `<os>`는 `darwin`(macOS) 또는 `linux`, `<arch>`는 `arm64`(Apple Silicon 등) 또는 `amd64`(Intel 등)입니다.

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
  name="ha_0.1.2_${os}_${arch}"
  url="https://github.com/jgoneit/jaekit/releases/download/v0.1.2"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
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
  mkdir -p "$HOME/.local/bin"
  mv "$name/ha" "$HOME/.local/bin/ha"
  echo "설치했습니다: $HOME/.local/bin/ha ($name)"
)
```

`~/.local/bin`이 PATH에 없으면 셸 설정(`~/.zshrc` 등)에 `export PATH="$HOME/.local/bin:$PATH"`를 더합니다. 브라우저로 직접 받은 파일을 macOS가 막으면 `xattr -d com.apple.quarantine ~/.local/bin/ha`로 풉니다. 그다음 [버전 확인](#버전-확인)과 [Claude Code](#claude-code) 또는 [Codex](#codex) 설치로 갑니다.

## 업데이트

`ha`와 두 plugin을 함께 새 버전으로 옮깁니다. 설치할 때 쓴 도구로 올립니다. 아래는 v0.1.2로 옮기는 명령입니다. 명령 블록은 하나씩 붙여 넣습니다.

brew로 설치한 `ha`는 brew가 tap을 새로 받은 뒤 올립니다.

```bash
brew update
```

```bash
brew upgrade jgoneit/tap/jaekit
```

Release 파일로 설치한 `ha`는 [Release 파일로 설치](#release-파일로-설치)의 명령을 다시 붙여 넣습니다. 새 버전의 파일이 `~/.local/bin/ha`를 덮어씁니다.

그다음 버전을 확인합니다. `ha 0.1.2`이 나오면 됩니다. 다른 버전이 나오면 [버전 확인](#버전-확인)대로 `which -a ha`를 봅니다.

```bash
ha --version
```

plugin은 marketplace를 새 태그에 고정해 다시 등록합니다. 같은 이름의 marketplace는 둘을 함께 둘 수 없어서 먼저 지웁니다. 지우면 그 marketplace에서 설치한 spec·seal도 함께 지워지므로 다시 설치합니다. 쓰는 host의 명령만 붙여 넣으면 됩니다.

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.2
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.2
codex plugin add spec@jaekit
codex plugin add seal@jaekit
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

Claude Code:

```bash
claude plugin uninstall spec@jaekit
claude plugin uninstall seal@jaekit
claude plugin marketplace remove jaekit
```

Codex:

```bash
codex plugin remove spec@jaekit
codex plugin remove seal@jaekit
codex plugin marketplace remove jaekit
```

작업한 저장소에 생긴 파일은 지우지 않고 그대로 남습니다. 목표 문서와 Seal이 만든 파일(`docs/specs/<goal>/`의 `SPEC.md`, `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`)은 commit한 기록이고, 검사 출력 원문은 git이 추적하지 않는, 그 작업 트리의 Git 디렉토리 아래 `ha/`에 있습니다. 보통은 `.git/ha/`이고, `.git`이 파일인 linked worktree에서는 `git rev-parse --absolute-git-dir`이 알려 주는 디렉토리 아래에 있습니다.

목표 문서와 기록도 지우려면 그 저장소의 루트에서 `git rm -r docs/specs/<goal>`로 목표를 지운 뒤 commit합니다.

검사 출력 원문도 지우려면 Jaekit의 검사가 돌고 있지 않을 때(Seal이 작업 중이 아닐 때) 그 작업 트리에서 아래 명령을 붙여 넣습니다. 그 작업 트리에 있는 모든 목표의 검사 출력 원문이 지워집니다. 다른 작업 트리의 출력과 commit한 목표 문서·실행 기록은 남습니다.

```bash
gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha"
```

## 로컬 경로 등록에서 옮기기

이미 저장소 checkout 경로로 `jaekit` marketplace를 등록했으면, 그 등록을 지우고 `v0.1.2`에 고정한 GitHub 저장소로 다시 등록합니다. 같은 이름의 marketplace는 둘을 함께 둘 수 없습니다. 지우면 그 marketplace에서 설치한 spec·seal도 함께 지워지므로 다시 설치합니다.

Claude Code:

```bash
claude plugin marketplace remove jaekit
claude plugin marketplace add jgoneit/jaekit#v0.1.2
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

Codex:

```bash
codex plugin marketplace remove jaekit
codex plugin marketplace add jgoneit/jaekit@v0.1.2
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

예전에 `go install`로 만든 `ha`가 남아 있으면 [버전 확인](#버전-확인)대로 정리합니다.

## 소스에서 빌드 (개발)

저장소 checkout에서 빌드한 `ha`는 개발 버전(`ha 0.1.2-dev`)을 표시합니다. Go 1.26 이상과 git이 필요합니다. checkout의 루트에서 실행합니다.

```bash
go install ./cmd/ha
ha --version
```

`ha` 파일은 `go env GOBIN`이 가리키는 폴더에 생기고, 그 값이 비어 있으면 `$(go env GOPATH)/bin`에 생깁니다. `ha --version`은 `ha 0.1.2-dev`를 출력합니다.

checkout을 고치며 plugin을 쓰려면 checkout 경로를 marketplace로 등록합니다. 이 등록은 checkout의 plugin 파일을 그대로 씁니다. `<jaekit 경로>`는 checkout의 경로로 바꿉니다.

Claude Code:

```bash
claude plugin marketplace add <jaekit 경로>
claude plugin install spec@jaekit
claude plugin install seal@jaekit
```

Codex:

```bash
codex plugin marketplace add <jaekit 경로>
codex plugin add spec@jaekit
codex plugin add seal@jaekit
```

명령과 기록 형식은 [contracts/run-record.md](../contracts/run-record.md)에 있습니다.
