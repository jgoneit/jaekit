# WSL2 준비 안내

**한국어** · [English](WSL.en.md)

Windows에서 WSL2의 **Ubuntu 24.04 LTS amd64(x64)** 안에 Jaekit을 설치하는 경로입니다. **실제 WSL2 설치·실행과 에이전트의 Skill 사용은 아직 확인하지 않았습니다.** 이 문서는 사용 경로의 준비 상태를 설명하며, WSL2 지원 검증이 끝났다는 뜻이 아닙니다.

현재 공개 배포판은 `v0.1.6`입니다. 아래에서 연결하는 설치 명령은 그 배포판의 Linux 파일을 받습니다. 이 안내와 [환경 확인 도구](#후보-패키지의-선택적-환경-확인)는 v0.1.6 파일의 공개 자료에 들어 있고, v0.1.4 이하 파일에는 없습니다. 후보 패키지의 확인, 공개 릴리스 게시, 공개 URL에서 받은 파일의 설치는 각각 별도입니다.

## 작업할 환경

| 항목 | 이 안내의 범위 |
| --- | --- |
| WSL | WSL2, 이미 설치된 Ubuntu 24.04 LTS |
| CPU | amd64(x64), Linux에서 `x86_64`로 표시 |
| 터미널 | 해당 Ubuntu 배포판의 Linux 터미널 |
| 도구 | 같은 배포판에 설치한 Linux용 에이전트, Git, `ha` |
| 프로젝트 | 같은 배포판의 Linux 홈 아래 Git 저장소 |
| 임시 파일·기록 | 같은 Linux 환경의 임시 공간과 프로젝트의 Git 디렉터리 |

WSL1, 다른 배포판·CPU, `/mnt/c` 같은 Windows 드라이브의 프로젝트, 동일 checkout의 Windows·WSL 교차 실행과 배포판 간 목표 이동은 이 경로의 범위가 아닙니다. 일반 Linux의 기존 amd64·arm64 설치 경로는 그대로입니다.

WSL2가 없다면 먼저 Microsoft의 [WSL 설치 안내](https://learn.microsoft.com/en-us/windows/wsl/install)를 따릅니다. Jaekit은 WSL 기능·배포판·에이전트를 자동 설치하거나 관리자 권한을 요청하지 않습니다. Windows PowerShell에서 다음 조회로 사용할 배포판의 `VERSION`이 `2`인지 확인할 수 있습니다. 배포판을 설치·변환·삭제하는 명령은 아닙니다.

```powershell
wsl --list --verbose
```

이후 단계는 **Ubuntu의 Linux 터미널**에서 진행합니다. Linux 터미널에서 다음 값을 확인합니다.

```bash
uname -sr
uname -m
cat /etc/os-release
pwd -P
```

Ubuntu 버전은 `24.04`, CPU는 `x86_64`여야 합니다. 프로젝트는 Linux 홈 아래에 둡니다. 홈 아래 이름이라도 Windows 드라이브를 가리키는 링크이면 이 경로에 해당하지 않습니다. Git의 임시 worktree와 `TMPDIR`를 별도로 정했다면 그 공간도 같은 Linux 파일시스템에 둡니다. Microsoft도 [Linux 도구에는 Linux 파일시스템 사용](https://learn.microsoft.com/en-us/windows/wsl/filesystems)을 권장합니다.

## Linux 배포물 설치

1. 해당 Ubuntu 배포판에 Linux용 Git과 사용할 에이전트 CLI를 준비합니다. 현재 공개 설치 안내의 호스트는 Codex CLI와 Claude Code입니다. Windows에 설치한 앱이나 `.exe`가 WSL의 설치를 대신하지 않습니다. 인증과 실행 권한도 사용할 Linux 호스트에서 준비합니다.
2. 같은 Linux 터미널에서 [Release 파일로 설치](INSTALL.md#release-파일로-설치)의 블록을 실행합니다. Git 외에 안내가 사용하는 `curl`, `tar`, checksum 도구와 기본 Linux 명령이 필요합니다. 누락된 도구는 먼저 준비합니다. Jaekit 소스 checkout·Go·Homebrew는 필요하지 않습니다.
3. 이 조합은 `ha_0.1.6_linux_amd64.tar.gz`와 `checksums.txt`를 사용합니다. 실행 파일은 `~/.local/bin/ha`, 공개 자료는 `~/.local/share/jaekit/0.1.6`에 놓입니다. 공개 자료를 수정해 설치본으로 쓰지 말고 필요한 파일을 프로젝트로 가져옵니다.
4. [Codex](INSTALL.md#codex) 또는 [Claude Code](INSTALL.md#claude-code) 설치 절에서 사용하는 쪽만 따릅니다. 플러그인 설치도 같은 배포판의 Linux CLI에서 합니다. 다른 호스트의 패키징이나 호출 이름은 이 안내에서 바꾸지 않습니다.

`~/.local/bin`이 PATH에 없다면 현재 Linux 셸에 다음을 적용할 수 있습니다. 이 설정은 현재 셸과 여기서 시작하는 프로세스에만 적용됩니다.

```bash
export PATH="$HOME/.local/bin:$PATH"
```

새 터미널에도 필요하면 사용하는 Linux 셸의 설정 파일에 같은 내용을 직접 추가합니다. Bash라면 `~/.bashrc` 등을 확인하고 기존 내용을 덮어쓰지 않습니다. Windows PATH, 다른 배포판의 설정, 전역 Git 설정이나 보안 정책을 바꿀 필요는 없습니다.

새 Ubuntu 터미널에서 프로젝트로 이동한 뒤 [버전 확인](INSTALL.md#버전-확인)을 따릅니다. 선택된 `git`과 `ha`의 위치가 Linux 설치인지 확인하고 실제 Core의 지원 정보를 조회합니다.

```bash
type -a git ha
ha --version
ha capabilities --format json
```

v0.1.6을 설치했다면 버전은 `ha 0.1.6`이어야 합니다. 여러 설치가 있으면 PATH에서 앞선 파일이 선택됩니다. Windows 경로·`.exe`나 의도하지 않은 예전 설치가 선택되면 먼저 해당 배포판의 PATH와 설치 위치를 바로잡습니다. Core 지원 정보는 [호환성 계약](../contracts/core-capabilities.md)에 따라 판단하며 버전 문자열만으로 충분하지 않습니다.

[참조 검사 가져오기](INSTALL.md#참조-검사-가져오기)의 직접 설치 경로를 사용하면 공개 자료를 프로젝트에 가져올 수 있습니다. 이미 있는 파일은 덮어쓰지 않습니다. Python 3는 이 참조 검사 생산자를 선택했을 때만 필요합니다.

## 후보 패키지의 선택적 환경 확인

[환경 확인 도구](../tools/wsl/check-environment.sh)는 공개 v0.1.6 배포 파일의 공개 자료(`~/.local/share/jaekit/0.1.6/tools/wsl/check-environment.sh`)에 들어 있습니다. v0.1.4 이하 배포 파일에는 없습니다. 위 배포판 설치에 필요한 도구는 아닙니다. 공개 자료나 검토한 소스, 후보 패키지에 `tools/wsl/check-environment.sh`가 있을 때만 사용할 수 있습니다. 두 자리 표시자를 실제 자료 폴더와 프로젝트 경로로 바꿉니다. 마지막 `codex`는 사용할 Linux 호스트 명령이며, `claude`로 바꾸거나 생략할 수 있습니다.

```bash
bash "<candidate-public-data>/tools/wsl/check-environment.sh" "<project-path>" codex
```

이 도구는 WSL2·Ubuntu 24.04·x86_64 여부와 Git·`ha`의 실제 경로를 확인해 표시합니다. 호스트 명령을 지정하면 그 경로도 확인합니다. 확인한 Linux Git의 `rev-parse`로 프로젝트가 Linux 홈 아래 Git 저장소 안에 있는지, `ha`가 기록을 남기는 Git 디렉터리와 공통 Git 디렉터리도 Linux 파일시스템에 있는지 확인합니다. Windows 드라이브의 저장소에서 만든 linked worktree는 이 조건에서 거부됩니다. 설치나 설정 변경은 하지 않습니다. 이 경로의 조건을 확인할 수 없으면 실패하며, 그 결과가 기존 일반 Linux 설치를 차단하는 것은 아닙니다.

경로 확인은 실행을 독립적으로 증명하지 않습니다. 호스트가 Skill을 불러왔는지, 검사 프로세스가 실제 요구를 검증했는지, 목표가 완료됐는지는 에이전트 사용 단계에서 따로 확인해야 합니다. 도구가 성공해도 실제 WSL2 작업 전체가 검증됐다고 보고하지 않습니다.

### 후보 파일 받기

후보 파일은 GitHub Actions의 [Release workflow](../.github/workflows/release.yml)가 만듭니다. 확인할 브랜치에서 Release workflow를 수동 실행(`workflow_dispatch`)하고 후보 버전(`X.Y.Z`)을 입력합니다. 실행이 끝나면 `release-files` artifact를 새 폴더에 내려받습니다. 그 안의 `ha_<version>_linux_amd64.tar.gz`와 `checksums.txt`가 후보이고, 압축을 푼 `ha_<version>_linux_amd64` 폴더가 위 명령의 공개 자료 폴더입니다. Pull request의 Release workflow 실행도 같은 artifact를 남기며, 이때 버전은 `0.0.0`입니다.

이 workflow는 실행한 commit을 새로 checkout해 태그 빌드와 같은 단계로 파일을 만들고 검사합니다. 그래서 그 commit에 없는 로컬 파일이나 커밋하지 않은 변경은 후보에 들어가지 않습니다. 수동 실행과 pull request 실행은 아무것도 게시하지 않으며, 후보 버전은 공개 릴리스 버전이 아닙니다. 위 v0.1.6 설치 블록의 버전을 후보 버전으로 바꿔 사용하지 않습니다. 후보를 만들었다는 사실은 실제 WSL2 실행 근거가 아닙니다.

## 에이전트에서 사용하기

같은 Linux 프로젝트에서 에이전트를 새로 시작하고 [호스트별 호출 안내](USAGE.md#명령-보내기)를 따릅니다. 실제로 불러온 것이 Jaekit의 Spec·Seal인지, 출처와 버전이 의도한 설치본인지 확인합니다. 셸에서 CLI 경로를 찾았다는 사실만으로 Skill이 실렸다고 판단하지 않습니다.

Spec에 작은 변경을 요청하면 목표 문서를 쓰고 멈춥니다. 내용을 확인한 **뒤 별도 메시지**로 Seal에 구현을 맡깁니다. Seal은 실제 Core의 지원 정보를 확인하고, 수정 전 요구 위반과 수정 뒤 충족을 기록합니다. 완료는 에이전트의 설명만으로 정하지 않고 [완료 보고의 근거](USAGE.md#완료-보고-읽기)를 확인합니다.

중단한 작업은 같은 배포판·프로젝트의 새 에이전트 세션에서 같은 목표를 지정해 이어 갑니다. 완료된 목표는 기록과 원문 로그를 조회할 수 있습니다. 완료 뒤 재작업은 별도로 요청합니다. 기록의 위치와 `ha log` 사용은 [저장되는 파일](USAGE.md#저장되는-파일)에 있습니다. 이 문단은 사용 절차이며, 실제 WSL2에서 수행한 사례를 제시한 것이 아닙니다.

## 재설치·업데이트·제거

같은 버전의 자료가 그대로 있으면 [직접 설치](INSTALL.md#release-파일로-설치)를 다시 사용할 수 있습니다. 같은 버전인데 내용이 다르거나 비정상 경로가 있으면 멈춥니다. 다운로드·checksum·파일 교체 실패를 성공으로 간주하지 말고 오류와 남은 설치 상태를 확인합니다. 기존 자료를 지워 충돌을 숨기지 않습니다.

[업데이트](INSTALL.md#업데이트)는 이 배포판에서 설치할 때 쓴 방법으로 진행합니다. `ha`와 플러그인을 호환되는 조합으로 갱신하고 새 Linux 터미널·에이전트에서 다시 확인합니다. 기존 목표와 실행 이력의 규칙·사용량·시간 한도는 그대로입니다. 이전 버전 공개 자료는 자동 삭제하지 않으며, 별도 정리 안내는 현재·더 새 버전 자료와 사용자 프로젝트를 보존합니다.

[제거](INSTALL.md#제거)도 같은 Ubuntu 배포판에서 직접 설치와 해당 호스트의 명령만 따릅니다. 이때 대상은 그 배포판의 Jaekit 설치물입니다. 프로젝트에 복사한 검사·예시, 목표 문서, `runs.jsonl`과 Git 디렉터리 아래 `ha/`의 원문 로그는 남습니다. 다른 배포판이나 Windows 설치물을 함께 지우지 않습니다. 공개 자료 경로가 링크이면 안내된 제거는 링크만 지우며 연결 대상을 따라 삭제하지 않습니다. WSL 배포판 자체를 삭제하거나 프로젝트·로그를 지우는 일은 Jaekit 제거에 포함되지 않습니다.

## 문제가 있을 때

| 상황 | 확인할 것 |
| --- | --- |
| PowerShell에서 Linux 명령이 동작하지 않음 | 해당 Ubuntu의 Linux 터미널로 돌아갑니다 |
| 배포판·CPU·WSL 버전이 범위와 다름 | 이 경로의 확인 대상으로 표시하지 않습니다. 일반 Linux 설치 가능성과 WSL2 확인 범위는 구분합니다 |
| PATH에 Windows 도구나 여러 설치가 보임 | 같은 배포판의 Linux 실행 파일이 선택되는지 확인하고 에이전트를 다시 시작합니다 |
| 다운로드·checksum·복사·교체 실패 | 안내의 실패 지점을 해결한 뒤 다시 시도합니다. 기존 실행 파일·자료·프로젝트 기록을 임의로 지우지 않습니다 |
| 스킬을 찾지 못함 | Linux 호스트의 플러그인 목록과 실제 불러온 Skill 출처를 확인합니다. [사용 안내](USAGE.md#문제-해결)를 봅니다 |
| 인증·권한·도구가 없음 | 필요한 항목을 해당 Linux 환경에서 준비합니다. 설치나 권한 변경을 자동으로 우회하지 않습니다 |

## 확인 범위

| 근거 | 확인할 수 있는 것 |
| --- | --- |
| 문서 일치·합성 환경 검사 | 안내의 정합성과 제한된 실패·보존 동작 |
| 일반 Linux의 패키지 실행 | 그 Linux 환경에서 사용한 패키지의 실행 결과 |
| 실제 WSL2의 패키지 설치·실행 | 기록한 WSL·배포판·CPU·패키지 조합의 결과 |
| 실제 Linux 호스트의 Skill 사용 | 그 호스트·모델·Skill 버전에서 관측한 흐름 |

표의 마지막 두 단계에 대한 실제 WSL2 관측은 아직 없습니다. 소스나 후보 패키지의 검사 성공을 실제 WSL2·호스트 사용 성공으로 올려 쓰지 않습니다. 현재 공개 배포판의 확인 범위는 [v0.1.6 릴리스 노트](https://github.com/jgoneit/jaekit/releases/tag/v0.1.6)를 따릅니다. 완료 보고의 보증 수준은 계속 `local`이며 독립된 제3자 검증을 뜻하지 않습니다.
