# CI 결과 읽기

**한국어** · [English](CI.en.md)

[CI 워크플로](../.github/workflows/ci.yml)는 PR과 `main` push에서 실행됩니다. 공통 검사는 Ubuntu에 두고, Ubuntu·macOS·Windows Server에서 필요한 네이티브 검사를 실행합니다. 특정 커밋과 실행의 결과를 확인하세요. 이 문서는 검사 구성을 설명하며 모든 환경이 항상 통과했다는 보고가 아닙니다.

## 필수 결과

| 검사 | 환경과 범위 |
| --- | --- |
| `Core / Ubuntu` | Ubuntu 24.04 x64: 기존 공통 Go·회귀 검사와 macOS/Linux 교차 빌드, 실제 네이티브 Core와 격리된 직접 설치 수명주기 |
| `Core / macOS` | `macos-15`, arm64: 선택된 네이티브 Core 검사와 실제 네이티브 후보의 격리된 직접 설치 수명주기 |
| `Core / Windows` | Windows Server 2025 x64: Windows PowerShell 5.1과 PowerShell 7 각각의 네이티브 Core·후보 패키지·경로 검사, 미지원 제품 설치 환경의 거부 |
| `Seal Core` | 같은 실행의 세 Core 작업이 모두 `success`여야 성공 |
| `Public documentation` | Ubuntu: 공개 파일·문서 검사와 오프라인 회귀 검사를 한 번 수행 |

main은 기존과 같이 GitHub Actions의 정확한 `Seal Core`, `Public documentation` 결과를 필수로 요구합니다. Core 집계는 선행 작업이 실패해도 실행되며 실패·누락·건너뜀·취소·알 수 없는 결과를 거부합니다. 대기 상태나 이전 실행의 성공은 현재 실행을 충족하지 않습니다. 네이티브 작업은 `continue-on-error`나 경로 필터로 생략한 검사를 성공으로 바꾸지 않습니다.

Ubuntu는 macOS/Linux 네 플랫폼 빌드와 Windows 후보 교차 빌드를 포함한 공통 검사 범위를 유지합니다. 교차 빌드 성공은 컴파일만 확인합니다. 추가 네이티브 작업은 현재 소스를 해당 호스트에서 실행하며, 공통 검사 전체나 문서 검사를 모두 반복하지 않습니다.

## 소스와 실행 근거

현재 PR 변경의 작업 로그를 엽니다. 각 네이티브 작업은 checkout SHA, 이벤트, PR head/base SHA, 실제 OS·CPU, 러너 정보와 셸 버전을 담은 `JAEKIT_CI_CONTEXT` JSON 한 줄을 출력합니다. GitHub가 PR 통합 커밋을 checkout하면 그 SHA는 PR head와 다를 수 있습니다. 로컬 소스와 결과를 비교할 때 이 관계를 함께 확인합니다.

각 네이티브 검사기는 실제 관측과 소스 커밋을 담은 `JAEKIT_CI_REPORT`를 출력합니다. Unix 검사기는 지정한 보고서 파일에도 저장하고, Windows 검사기는 선택한 임시 출력 디렉터리에 보고서와 검사 자료를 둡니다. 필수 네이티브 검사에는 실제 통과 결과가 있어야 합니다. 건너뛰었거나 없는 검사를 선택 목록의 개수·빈 helper 실행·구성 검사로 대신하지 않습니다. 보고서는 해당 작업의 소스와 환경에 관한 것이며 다른 CPU나 셸의 실행까지 확인하지 않습니다.

워크플로는 러너별 작업 공간과 임시 경로를 사용합니다. Actions는 커밋으로 고정하고 Go는 `go.mod`를 따르며, 토큰 권한은 `contents: read`입니다. 추가 비밀 값·비공개 목표·로컬 실행 기록·AI 서비스를 CI 입력으로 요구하지 않습니다. 비공개 개발 자료는 커밋과 공개 로그에 포함하지 않아야 합니다.

## 설치와 지원 범위

Unix 설치 검사는 현재 소스로 실제 네이티브 후보를 만든 뒤 임시 사용자 디렉터리에서 문서의 직접 설치 경로를 실행합니다. 재설치·업데이트·제거와 무관한 프로젝트 자료 보존을 확인합니다. 후보 파일을 공급하는 로컬 전송은 공개 릴리스 다운로드 가능 여부와 구분합니다.

Windows Server에서는 개발자 검사와 미지원 환경의 제품 설치·제거 거부를 확인합니다. 설치기의 **Windows 11 Client x64** 제한은 유지됩니다. GitHub Windows 러너에는 관리자 권한이 있으므로 이 결과는 Windows 11 일반 사용자의 설치 동작을 확인한 것이 아닙니다. 별도 인수 범위는 [후보 안내](WINDOWS.md)를 참고하세요.

Ubuntu 실행은 WSL2 실행이 아닙니다. CI는 WSL2 설치, 설치된 에이전트 Skill의 로딩, 실제 에이전트 세션과 재개를 확인하지 않습니다. 이 관측은 별도로 남습니다. CI 성공이 릴리스 게시·버전 변경·공개 배포 URL에서의 설치 성공을 뜻하지도 않습니다.

## 로컬 검사

커밋·공개 전에 저장소의 공통 검증 진입점을 실행합니다.

```bash
python3 tools/verify.py
```

현재 개발 호스트와 공통 회귀를 검사하며 다른 OS를 실행하지는 않습니다. Unix 네이티브 설치 수명주기는 변경을 커밋한 깨끗한 checkout과 기존 Git·Python·Go·셸 선행 도구가 필요한 별도 소스 개발 검사입니다. Go의 실제 대상은 현재 네이티브 호스트여야 하고 적용되는 `GOFLAGS` 재정의가 없어야 합니다.

```bash
python3 tools/ci/native_unix.py --report /tmp/jaekit-native-report.json
```

checkout 밖의 새 보고서 경로를 사용하세요. 임시 설치 자료를 사용하며 실제 사용자 설치물을 교체하지 않습니다. 적합한 네이티브 Windows 개발 호스트에서는 깨끗한 checkout에서 PowerShell 5.1과 7 각각에 checkout 밖의 새 `-OutputDirectory`를 지정해 `tools/windows/native_checks.ps1`을 실행합니다. 두 검사기는 실행 중 소스나 커밋이 바뀌면 거부합니다. 누락 도구를 진단하며 로컬 선행 도구 설치나 전역 Git·셸·PATH·실행 정책 변경을 수행하지 않습니다.

러너 이름은 선택한 이미지입니다. 매 실행의 이미지와 실제 버전도 확인하세요. 이용 가능한 CPU와 러너 권한은 GitHub의 [러너 안내](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)에 설명되어 있습니다.
