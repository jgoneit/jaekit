# CI 결과 읽기

**한국어** · [English](CI.en.md)

[CI 워크플로](../.github/workflows/ci.yml)는 PR과 `main` push에서 실행됩니다. 공통 검사는 Ubuntu에 두고, Ubuntu·macOS·Windows Server에서 필요한 네이티브 검사를 실행합니다. 특정 커밋과 실행의 결과를 확인하세요. 이 문서는 검사 구성을 설명하며 모든 환경이 항상 통과했다는 보고가 아닙니다.

## 필수 결과

| 검사 | 환경과 범위 |
| --- | --- |
| `Core / Ubuntu` | Ubuntu 24.04 x64: 기존 공통 Go·회귀 검사와 macOS/Linux 교차 빌드, 실제 네이티브 Core와 격리된 직접 설치 수명주기 |
| `Core / macOS` | `macos-15`, arm64: 선택된 네이티브 Core 검사와 실제 네이티브 후보의 격리된 직접 설치 수명주기 |
| `Core / Windows PowerShell 5.1` | Windows Server 2025 x64, Windows PowerShell 5.1: 네이티브 Core·판단 이력·후보 패키지·경로 검사, 미지원 제품 설치 환경의 거부 |
| `Core / Windows PowerShell 7` | 같은 Windows Server 2025 x64 검사를 PowerShell 7에서 실행. 5.1 작업을 기다리지 않는 별도 작업 |
| `Seal Core` | 같은 실행의 `Core / Ubuntu`, `Core / macOS`, `Core / Windows PowerShell 5.1`, `Core / Windows PowerShell 7`이 모두 `success`여야 성공 |
| `Public documentation` | Ubuntu: 공개 파일·문서 검사와 오프라인 회귀 검사를 한 번 수행 |
| `Release candidate` | 같은 [릴리스 workflow](#릴리스-workflow) 실행의 `Build release files`와 `Windows release archive`가 모두 `success`로 끝나야 함 |

main은 GitHub Actions의 정확한 `Seal Core`, `Public documentation`, `Release candidate` 결과를 필수로 요구합니다. `Release candidate`는 [릴리스 workflow](#릴리스-workflow)에서 나옵니다. Core 집계는 선행 작업이 실패해도 실행되며 실패·누락·건너뜀·취소·알 수 없는 결과를 거부합니다. 대기 상태나 이전 실행의 성공은 현재 실행을 충족하지 않습니다. 네이티브 작업은 `continue-on-error`나 경로 필터로 생략한 검사를 성공으로 바꾸지 않습니다.

Ubuntu는 macOS/Linux 네 플랫폼 빌드와 Windows 후보 교차 빌드를 포함한 공통 검사 범위를 유지합니다. 교차 빌드 성공은 컴파일만 확인합니다. 추가 네이티브 작업은 현재 소스를 해당 호스트에서 실행하며, 공통 검사 전체나 문서 검사를 모두 반복하지 않습니다.

## 소스와 실행 근거

현재 PR 변경의 작업 로그를 엽니다. 각 네이티브 작업은 checkout SHA, 이벤트, PR head/base SHA, 실제 OS·CPU, 러너 정보와 셸 버전을 담은 `JAEKIT_CI_CONTEXT` JSON 한 줄을 출력합니다. GitHub가 PR 통합 커밋을 checkout하면 그 SHA는 PR head와 다를 수 있습니다. 로컬 소스와 결과를 비교할 때 이 관계를 함께 확인합니다.

각 네이티브 검사기는 실제 관측과 소스 커밋을 담은 `JAEKIT_CI_REPORT`를 출력합니다. Unix 검사기는 지정한 보고서 파일에도 저장하고, Windows 검사기는 선택한 임시 출력 디렉터리에 보고서와 검사 자료를 둡니다. 필수 네이티브 검사에는 실제 통과 결과가 있어야 합니다. 건너뛰었거나 없는 검사를 선택 목록의 개수·빈 helper 실행·구성 검사로 대신하지 않습니다. 보고서는 해당 작업의 소스와 환경에 관한 것이며 다른 CPU나 셸의 실행까지 확인하지 않습니다.

Windows 판단 이력 검사는 두 PowerShell 각각에서 실제 CLI·Git·공용 이력 저장소·파일 근거를 연결합니다. 일반 checkout과 linked worktree의 등록·재전송·정정·기본/current·list·두 방식의 show, 동시 등록·조회와 잠금 보유자 종료, 손상·적용 불확실성 및 파일 안전 경계를 필수 이름으로 선택합니다. 판단 이력 패키지나 필수 사례의 누락·skip·실패·실행 중단은 해당 셸을 통과시키지 않으며 그 셸의 Windows 작업과 `Seal Core` 집계에 반영됩니다. 기존 Core 시험과 교차 빌드만 성공한 결과로 대체하지 않습니다.

CI 결과 수집은 두 PowerShell의 기계 판정용 전체 보고서를 셸별 작업에서 하나씩 받아 기존 Windows 검증 소비자에 전달합니다. 셸별 작업으로 나누기 전에 기록된 실행은 한 `Core / Windows` 작업에 두 보고서가 있으며, 그 구성대로 읽습니다. 긴 줄의 표시 제한으로 일부 보고서가 사라진 결과, 누락·중복·잘림·수집 실패는 성공이 아닙니다. 현재 head/base와 실제 검사한 통합 commit의 관계·Git tree, 실행과 재실행 회차를 함께 확인하며 수집 중 기준이 바뀌면 이전 성공으로 대신하지 않습니다.

Windows 실패 진단은 먼저 실패한 시험의 출력을 선택하고 공개 합성 로그의 제한된 끝부분을 제공합니다. 긴 정상 출력이 앞선 실패 이유를 가리지 않게 하며, 분량 제한·파일 접근·해석 오류로 진단이 생략되면 이를 표시합니다. 진단 수집 실패는 원래 실패를 유지합니다. 제품 실행 실패, 준비·수집 실패와 관측하지 못한 결과를 구별하고, 진단만으로 원인을 확정하지 못하면 그 한계를 유지합니다. 전체 원문 로그나 비공개 작업 자료를 공개하는 기능은 아닙니다.

워크플로는 러너별 작업 공간과 임시 경로를 사용합니다. Actions는 커밋으로 고정하고 Go는 `go.mod`를 따르며, 토큰 권한은 `contents: read`입니다. 추가 비밀 값·비공개 목표·로컬 실행 기록·AI 서비스를 CI 입력으로 요구하지 않습니다. 비공개 개발 자료는 커밋과 공개 로그에 포함하지 않아야 합니다.

## 설치와 지원 범위

Unix 설치 검사는 현재 소스로 실제 네이티브 후보를 만든 뒤 임시 사용자 디렉터리에서 문서의 직접 설치 경로를 실행합니다. 재설치·업데이트·제거와 무관한 프로젝트 자료 보존을 확인합니다. 후보 파일을 공급하는 로컬 전송은 공개 릴리스 다운로드 가능 여부와 구분합니다.

Windows Server에서는 개발자 검사와 미지원 환경의 제품 설치·제거 거부를 확인합니다. 설치기의 **Windows 11 Client x64** 제한은 유지됩니다. GitHub Windows 러너에는 관리자 권한이 있으므로 이 결과는 Windows 11 일반 사용자의 설치 동작을 확인한 것이 아닙니다. 별도 인수 범위는 [후보 안내](WINDOWS.md)를 참고하세요.

각 Windows 작업은 최초 checkout 단계에만 줄바꿈 변환을 끄는 Git 환경 옵션을 전달해 커밋 바이트를 그대로 꺼냅니다. Git 설정 파일이나 사용자 프로젝트를 바꾸지 않으며 이후 검사와 fixture에 옵션을 강제하지 않습니다. 네이티브 명령 시나리오는 잘못된 증거 선언·생산자 경로의 거부와 기록 보존을 확인합니다. 두 PowerShell은 한국어·영어 안내의 사전 검증 코드를 실제로 실행해 정상 후보와 ZIP·각 스크립트의 변조를 구분합니다. 실제 후보는 사전 검증만 하고, 설치·제거·acceptance 안내의 실행 연결은 격리된 무해한 합성 코드로 관측합니다. 이 결과는 Windows 11 설치 생명주기 관측과 구분됩니다.

후보 생명주기 개발자 검사는 현재 acceptance가 사용하는 단계 선택·하위 실행 로직을 격리된 후보 코드와 연결해 첫 후보와 업데이트 후보를 구별합니다. checksum이 맞아도 새 후보 코드가 실패하면 실패로 나타나야 합니다. PATH 검사는 제품 제거 로직과 실제 호출 연결을 사용해 뒤에 추가된 동일 문자열·alias·무관한 항목의 순서 보존을 관측합니다. 이 fixture들은 실제 사용자 설치·PATH·정책을 바꾸지 않으며 Windows 11 전체 설치 수용을 대신하지 않습니다.

Windows의 명령 단위 프로세스 시나리오는 하위 프로세스 정리·timeout을 명령 종료 코드·기록·안정된 출력·상태에 연결합니다. 종료가 확인된 timeout은 안정된 로그 digest를 유지합니다. 종료 확인 실패는 명령의 실행 경계에 별도로 주입해 유효한 출력 근거 없이 오류가 기록되는지 확인합니다. 이 주입은 실제 Windows API 장애 재현이 아니며 공개 timeout·장애 주입 옵션을 추가하지 않습니다.

두 Windows 셸은 제품 패키지 이름 검사와 격리된 설치·제거 호출도 관측합니다. 패키지 정책이 정한 확장자형을 포함한 콘솔 장치명은 추출 쓰기 전에 거부해야 합니다. 정책에 맞지 않는 기존 inventory는 이관·삭제하지 않고 보존합니다. 이 fixture는 콘솔 장치를 열거나 실제 사용자 설치·PATH·정책을 바꾸지 않습니다.

추가 스트림(ADS) 검사는 실제 NTFS의 파일·설치 루트·하위 디렉터리에서 이름과 바이트를 확인합니다. 두 PowerShell 각각에서 제품 inventory와 재설치·업데이트·제거 및 임시·백업 정리 경계를 연결해 거부·보존과 정상 허용을 관측합니다. 확인 실패나 스트림 미지원 결과를 검사 경계에 주입한 사례는 실제 파일시스템 관측과 구분합니다. 필수 사례 누락·skip·실패는 통과가 아니며 실제 사용자 설치·PATH·정책은 보존합니다. 이 결과는 Windows 11 일반 사용자 설치 수용을 대신하지 않습니다.

셸마다 작업이 checkout·Go·Python 준비와 소스 확인을 따로 하므로, PS5.1 검사가 실패해도 PS7 검사를 실행하고 그 반대도 같습니다. 어느 셸의 실패도 그 셸의 작업과 필수 집계에 남습니다. 준비 실패·작업 취소·시간제한 종료로 얻지 못한 결과는 성공이 아닙니다. 두 셸 전체 보고서는 ADS 뒤의 패키지 검사까지 포함합니다.

진단 대조는 올바른 거부와 잘못된 이유·대상·종료값을 구분합니다. 격리된 ADS 자식의 표시 설정과 공개 acceptance의 기본 표시 설정은 각각 관측합니다. baseline junction 시나리오는 실제 CLI가 만든 임시 작업 트리에서 거부·미실행·기록과 외부 자료 보존을 확인하며, 안전한 일반 파일의 실행을 함께 확인합니다. 이 시나리오는 제품 우회 옵션을 추가하지 않습니다.

Ubuntu 실행은 WSL2 실행이 아닙니다. CI는 WSL2 설치, 설치된 에이전트 Skill의 로딩, 실제 에이전트 세션과 재개를 확인하지 않습니다. 이 관측은 별도로 남습니다. CI 성공이 릴리스 게시·버전 변경·공개 배포 URL에서의 설치 성공을 뜻하지도 않습니다.

## 릴리스 workflow

[릴리스 workflow](../.github/workflows/release.yml)는 태그 push뿐 아니라 PR과 수동 실행에서도 같은 빌드와 검사를 실행합니다. 게시는 태그 push에서만 합니다.

- `Build release files`(Ubuntu): macOS·Linux tar.gz 4개와 Windows ZIP 1개를 만들고, 다섯 아카이브를 나열한 `checksums.txt` 하나를 만듭니다. 게시 전 검사로 ZIP 내부 manifest·파일 해시·경로·Windows 실행 파일 형식까지 확인합니다. PR 실행은 PR head 커밋을 빌드합니다.
- `Windows release archive`(Windows Server 2025): 같은 ZIP을 PowerShell 5.1과 7 각각에서 설치기 자체의 패키지 검증으로 확인하고 `ha --version`·`capabilities`를 실행합니다. 그다음 [공개 설치·제거 블록](WINDOWS.md#공개-릴리스로-설치)을 내려받는 출처와 버전만 바꿔 그대로 붙여 넣습니다. 실제 ZIP으로는 설치기가 Windows 11 가드에서 멈추는 것을, 시험용 ZIP으로는 성공·실패·체크섬 거부에서 부모 PATH·실행 정책·자식 셸을 관측합니다. 보고서는 `release-windows-reports`에 남습니다.
- `Publish release`: 태그 push에서만 실행합니다. Windows job이 확인한 ZIP과 게시할 ZIP이 다르면 아무것도 게시하지 않습니다. 태그의 릴리스 노트(`tools/release/notes/vX.Y.Z.md`)에 그 ZIP의 Windows 11 확인 줄이 없으면 Windows ZIP을 빼고 macOS·Linux 파일만 게시합니다. ZIP을 포함할 때는 가이드의 공개 블록이 그 릴리스 버전을 설치하지 않으면 아무것도 게시하지 않습니다. 원격 태그가 더 이상 파일을 빌드한 커밋을 가리키지 않아도 멈춥니다. 게시 뒤에는 게시된 자산 이름을 확인한 파일과 대조합니다.

이 workflow의 `Release candidate` job은 `main`의 필수 검사입니다. `Build release files`와 `Windows release archive` 뒤에 둘 중 하나가 실패해도 실행되며, 같은 실행에서 둘 다 성공해야만 성공합니다. 건너뛰거나 취소되거나 실패한 job은 이 검사를 충족하지 않고, workflow에 path filter가 없어 모든 PR이 이 결과를 보고합니다. Server에서의 확인은 Windows 11 설치 성공이 아니며, 실제 설치·재설치·제거는 [Windows 11 확인 절차](../tools/windows/README.md)에서 확인합니다.

## 사이트 workflow

[사이트 workflow](../.github/workflows/pages.yml)는 `guides/`, `contracts/`와 `.claude-plugin/marketplace.json`의 릴리스 버전으로 [문서 사이트](https://jgoneit.github.io/jaekit/)를 만듭니다. pull request, `main` push, 수동 실행에서 돌고, `main`에서만 게시합니다. pull request 실행은 사이트를 만들고 시험하기만 합니다.

- `Site build`: 사이트를 만듭니다. 링크 대상이나 릴리스 버전을 읽을 수 없으면 실패합니다.
- `Site test`: 만든 사이트의 문서 집합, 링크, 화면 동작을 시험합니다. 게시를 막지는 않습니다.
- `Site deploy`: `main`에서 `Site build`가 성공했을 때만 GitHub Pages에 게시합니다. 그 커밋이 아직 `main`의 최신일 때만 게시하고, 더 새 커밋은 그 커밋의 실행이 게시합니다.

세 job은 필수 체크가 아니라서 실패해도 병합을 막지 않습니다. 빌드나 게시가 실패하면 이전에 게시된 사이트가 그대로 남습니다. 사이트를 로컬에서 만드는 방법은 [사이트 소스 안내](../tools/site/README.md)에 있습니다.

## 로컬 검사

푸시·공개 전에 최종 후보와 같은 검증 입력에서 저장소의 공통 검증 진입점이 한 번 통과해야 합니다. 그 한 번이 커밋 전·푸시 전 요구를 함께 충족하며, 커밋 전후 어느 쪽에서 실행해도 됩니다. 같은 입력에 대해 전체 검증을 두 번 실행할 필요는 없습니다.

```bash
python3 tools/verify.py
```

검증 입력은 추적 파일의 내용·경로·모드·추적 상태, 검증 명령과 그룹, 결과에 영향을 주는 도구·설정·환경입니다. 이 중 하나라도 바뀌면 이전 통과는 새 입력의 근거가 아닙니다. 커밋 식별자가 바뀐 것만으로 입력이 바뀌었다고 보지 않습니다. 개발 중에는 변경에 맞는 좁은 검사를 써도 됩니다. 이 재사용은 이 안내의 중복 실행만 없앱니다. 다른 도구·설정·환경에서의 성공을 보증하지 않고, 일반 검증 실행을 Seal 기록으로 인정하거나 기존 신선도·baseline·실패 이력·독립 검토·필수 CI 요구를 바꾸지 않습니다.

Git이 무시하지 않는 추적되지 않은 파일이 있으면 어떤 검사도 시작하기 전에 그 경로를 보이고 멈춥니다. 검증에 필요한 제품·검사 파일은 `git add`로 추적하고, 공개하지 않을 로컬 파일은 `.gitignore`나 `.git/info/exclude`를 검토하세요. 검증은 파일을 추가·제외·삭제하지 않습니다. Git 상태를 읽지 못하면 깨끗하다고 추정하지 않고 멈춥니다.

`go` 그룹은 이 checkout에서 전체 Go 시험을 한 번 실행합니다. 그 안의 현재 파일 독립성 검사는 추적 파일로 공개 사본 두 개를 만듭니다. 그 파일로 만든 2-commit 저장소와 그 저장소의 depth 1 clone이며, 둘 다 `docs/`와 `.git/ha`가 없습니다. 각 사본에서는 TrackedFiles 검사, 공개 파일·문서 검사, `current-file-independence` 경로가 든 생산자의 현재 파일 판정을 실행합니다. 사본에서 전체 Go 시험과 `docs` 그룹을 되풀이하지 않습니다. 일반 Go 시험이나 문서 회귀가 history나 로컬 목표 문서에 기대는지는, history 없이 commit 하나를 checkout하는 필수 CI job에서 드러납니다. workflow 회귀가 그 checkout 깊이를 고정합니다.

각 단계의 통과·실패 줄에는 그 단계의 시간이, 마지막 줄에는 전체 시간이 초 단위(소수 첫째 자리)로 표시됩니다. 단계 시간은 그 단계의 `RUN:` 줄부터 결과까지, 전체 시간은 검증 시작부터 마지막 줄까지이며 단조 시계로 잽니다. 키보드 인터럽트로 멈추면 중단된 단계와 두 시간이, 검사 전에 멈추면 검사가 시작되지 않았다는 사실과 전체 시간이 보입니다. 시간은 검사 결과를 정하는 데 쓰이지 않습니다.

현재 개발 호스트와 공통 회귀를 검사하며 다른 OS를 실행하지는 않습니다. Unix 네이티브 설치 수명주기는 변경을 커밋한 깨끗한 checkout과 기존 Git·Python·Go·셸 선행 도구가 필요한 별도 소스 개발 검사입니다. Go의 실제 대상은 현재 네이티브 호스트여야 하고 적용되는 `GOFLAGS` 재정의가 없어야 합니다.

```bash
python3 tools/ci/native_unix.py --report /tmp/jaekit-native-report.json
```

checkout 밖의 새 보고서 경로를 사용하세요. 임시 설치 자료를 사용하며 실제 사용자 설치물을 교체하지 않습니다. 적합한 네이티브 Windows 개발 호스트에서는 깨끗한 checkout에서 PowerShell 5.1과 7 각각에 checkout 밖의 새 `-OutputDirectory`를 지정해 `tools/windows/native_checks.ps1`을 실행합니다. 두 검사기는 실행 중 소스나 커밋이 바뀌면 거부합니다. 누락 도구를 진단하며 로컬 선행 도구 설치나 전역 Git·셸·PATH·실행 정책 변경을 수행하지 않습니다.

러너 이름은 선택한 이미지입니다. 매 실행의 이미지와 실제 버전도 확인하세요. 이용 가능한 CPU와 러너 권한은 GitHub의 [러너 안내](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)에 설명되어 있습니다.
