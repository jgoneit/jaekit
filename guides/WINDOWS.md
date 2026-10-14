# Windows 네이티브 후보 설치 안내

**한국어** · [English](WINDOWS.en.md)

이 안내는 **Windows 11 x64, Windows PowerShell 5.1·PowerShell 7**을 대상으로 준비한 로컬 후보 설치 경로입니다. 현재 공개 배포판 `v0.1.4`에 Windows 파일이 있다는 뜻이 아닙니다. 후보 패키지 생성과 교차 빌드는 Windows에서의 설치·실행 관측이나 에이전트의 실제 Skill 사용을 증명하지 않습니다. 해당 후보의 확인 결과와 미확인 범위를 함께 읽습니다.

Windows ARM64·x86, 이전 Windows, 네트워크/UNC 저장소와 같은 checkout의 Windows·WSL 교차 사용은 이 경로의 지원 대상이 아닙니다. WSL 지원과 네이티브 지원은 별개입니다.

## 준비

- 64비트 Windows PowerShell 5.1 또는 PowerShell 7을 엽니다. Git과 사용할 에이전트는 별도로 준비합니다. 설치기는 Git·에이전트·PowerShell·Windows 기능을 설치하거나 관리자 권한을 요청하지 않습니다.
- 검토한 같은 후보 묶음에서 `ha_<version>_windows_amd64.zip`, `checksums.txt`, `install.ps1`, `uninstall.ps1`, `common.ps1`, `acceptance.ps1`을 받습니다. 이 여섯 파일만 들어 있는 전용 폴더를 사용합니다. 설치기는 로컬 파일만 읽고 다운로드나 공개 배포를 수행하지 않습니다. 취득에 실패했으면 설치가 시작된 것이 아닙니다.
- checksum은 가져온 파일들이 후보의 내용과 맞는지 확인합니다. 같은 출처에서 변조된 checksum까지 신뢰할 수 있게 만드는 서명은 아닙니다. 스크립트도 실행할 코드이므로 출처와 내용을 확인합니다.
- 회사 또는 PC의 실행 정책이 스크립트를 막으면 해당 정책에 따라 처리합니다. 설치기는 실행 정책·보안 설정을 바꾸거나 우회 옵션으로 다시 실행하지 않습니다.

일반 설치에 Go·Homebrew·Bash·zsh·WSL이나 Jaekit 소스 checkout은 필요하지 않습니다. 프로젝트 검사가 별도 도구를 요구한다면 그 검사의 의존성입니다. 참조 검사 생산자를 사용하는 경우에만 해당 예시의 Python 요구사항을 확인합니다.

## 설치와 확인

신뢰한 출처에서 이 안내와 `checksums.txt`를 확인한 뒤, 아래 함수 정의를 PowerShell에 붙여 넣습니다. 후보에서 가져온 스크립트나 helper는 아직 실행하지 않습니다. 이 코드는 PowerShell 기본 기능만으로 ZIP과 네 개별 스크립트를 모두 검사하며, 후보 코드에 의존하지 않습니다. 새 PowerShell 세션에서도 먼저 이 정의를 준비합니다.

<!-- BEGIN WINDOWS CANDIDATE PREFLIGHT -->
```powershell
function Get-JaekitVerifiedCandidate([string]$Directory) {
    $ErrorActionPreference = 'Stop'
    $root = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Directory)
    $root = [IO.Path]::GetFullPath($root)
    if ($root -notmatch '^[A-Za-z]:\\' -or -not [IO.Directory]::Exists($root)) {
        throw 'Candidate must be an existing local drive directory.'
    }
    $ancestor = $root
    while ($ancestor) {
        if (([IO.File]::GetAttributes($ancestor) -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Candidate path must not contain links or reparse points.'
        }
        $parent = [IO.Directory]::GetParent($ancestor)
        if ($null -eq $parent) { break }
        $ancestor = $parent.FullName
    }
    $names = @()
    foreach ($path in [IO.Directory]::GetFileSystemEntries($root)) {
        $attributes = [IO.File]::GetAttributes($path)
        if (($attributes -band ([IO.FileAttributes]::Directory -bor [IO.FileAttributes]::ReparsePoint)) -ne 0) {
            throw 'Candidate entries must be ordinary files.'
        }
        $names += [IO.Path]::GetFileName($path)
    }
    $archives = @($names | Where-Object { $_ -cmatch '^ha_[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)?_windows_amd64\.zip$' })
    if ($archives.Count -ne 1) { throw 'Expected exactly one Windows candidate ZIP.' }
    $expected = @($archives[0], 'common.ps1', 'install.ps1', 'uninstall.ps1', 'acceptance.ps1')
    if ($names.Count -ne 6 -or $names -cnotcontains 'checksums.txt') {
        throw 'Candidate file inventory is incomplete or unexpected.'
    }
    foreach ($name in $names) {
        if ($name -cne 'checksums.txt' -and $expected -cnotcontains $name) {
            throw 'Candidate file inventory is incomplete or unexpected.'
        }
    }
    $checksums = Join-Path $root 'checksums.txt'
    if ((Get-Item -LiteralPath $checksums).Length -gt 1048576) { throw 'Checksum file is too large.' }
    $lines = [IO.File]::ReadAllLines($checksums, [Text.Encoding]::ASCII)
    $seen = @{}
    foreach ($line in $lines) {
        if ($line -cnotmatch '^([a-fA-F0-9]{64})  (.+)$') { throw 'Malformed SHA-256 checksum entry.' }
        $digest = $Matches[1]
        $name = $Matches[2]
        if ($expected -cnotcontains $name) { throw 'Unexpected or unsafe checksum path.' }
        if ($seen.ContainsKey($name)) { throw 'Duplicate checksum entry.' }
        $seen[$name] = $digest
    }
    if ($seen.Count -ne $expected.Count) { throw 'Missing checksum entry.' }
    foreach ($name in $expected) {
        $path = Join-Path $root $name
        if (-not [IO.File]::Exists($path)) { throw ('Missing candidate file: ' + $name) }
        $limit = if ($name -ceq $archives[0]) { 134217728 } else { 33554432 }
        if ((Get-Item -LiteralPath $path).Length -gt $limit) { throw ('Candidate file is too large: ' + $name) }
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ine $seen[$name]) {
            throw ('SHA-256 mismatch: ' + $name)
        }
    }
    return [pscustomobject]@{
        Directory = $root; Archive = (Join-Path $root $archives[0]); Checksums = $checksums
        Install = (Join-Path $root 'install.ps1'); Uninstall = (Join-Path $root 'uninstall.ps1')
        Acceptance = (Join-Path $root 'acceptance.ps1')
    }
}
```
<!-- END WINDOWS CANDIDATE PREFLIGHT -->

후보 파일 폴더로 이동한 뒤 아래 블록 전체를 실행합니다. ZIP과 네 스크립트 중 하나라도 검증되지 않으면 블록이 중단되어 설치 코드를 실행하지 않습니다. checksum 목록의 누락·중복·잘못된 digest·후보 밖 경로와 예상하지 않은 파일도 거부합니다. 검증 직후 같은 사용자 권한의 다른 프로세스가 파일을 바꾸는 공격을 격리하는 절차는 아닙니다.

<!-- BEGIN WINDOWS VERIFIED INSTALL -->
```powershell
& {
    $candidate = Get-JaekitVerifiedCandidate (Get-Location).Path
    & $candidate.Install -Archive $candidate.Archive -Checksums $candidate.Checksums -AddToPath
}
```
<!-- END WINDOWS VERIFIED INSTALL -->

`-AddToPath`는 **현재 사용자 PATH에 설치 폴더 한 항목을 추가**하는 선택입니다. 생략하면 PATH는 바뀌지 않으며 실행 파일의 절대 경로를 사용합니다. 같은 경로가 이미 있으면 중복 추가하거나 기존 항목을 설치기의 소유로 바꾸지 않습니다. 시스템 PATH·전역 Git 설정은 변경하지 않습니다.

설치 위치는 Windows가 알려 주는 현재 사용자의 LocalApplicationData 아래 `Jaekit`입니다. 보통 `%LOCALAPPDATA%\Jaekit`이며, 다음과 같이 찾습니다.

```powershell
$jaekitRoot = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Jaekit'
$jaekitExe = Join-Path $jaekitRoot 'bin\ha.exe'
& $jaekitExe --version
& $jaekitExe capabilities --format json
Get-Command ha -All
```

설치기는 교체 전에 후보 실행 파일의 버전과 지원 정보를 확인합니다. 위 결과의 버전은 후보의 버전과 같아야 합니다. `Get-Command`에서 다른 `ha`가 먼저 선택된다면 그 경로와 새 설치 경로를 구분합니다. 임의로 기존 파일을 삭제하지 말고 사용할 경로를 선택합니다. Seal에도 확인한 실제 실행 파일을 사용해야 합니다.

설치기는 현재 PowerShell의 PATH를 갱신하고 Windows에 환경 변경을 알립니다. 이미 열린 에이전트·터미널의 부모 앱은 옛 PATH를 유지할 수 있으므로 부모 앱까지 종료한 뒤 새 세션을 엽니다. 계속 옛 환경을 상속하면 다시 로그인하거나 절대 경로를 사용합니다. PATH·버전 확인 성공은 목표 완료나 에이전트 Skill 로딩 결과가 아닙니다.

목표 문서와 검사 코드는 로컬 Windows Git 프로젝트에 둡니다. [기존 호스트 설치 안내](INSTALL.md)의 플러그인 설치·활성화 계약은 그대로 적용하되, Linux/macOS에서 확인한 호스트 동작을 Windows에서의 실사용 결과로 대신하지 않습니다. 이 후보 설치기는 플러그인을 설치하거나 재설계하지 않습니다.

## 공개 자료와 검사 명령

각 버전의 안내·계약·참조 예시는 `versions\<version>`에 있습니다. 현재 버전 경로를 확인하고, 예시는 프로젝트의 **존재하지 않는 새 폴더**로 복사합니다.

```powershell
$jaekitState = Get-Content -LiteralPath (Join-Path $jaekitRoot 'managed.json') -Raw | ConvertFrom-Json
$jaekitData = Join-Path (Join-Path $jaekitRoot 'versions') $jaekitState.current_version
$jaekitExample = Join-Path (Get-Location).Path 'jaekit-reference-example'
if (Test-Path -LiteralPath $jaekitExample) { throw '복사 대상이 이미 있습니다.' }
Copy-Item -LiteralPath (Join-Path $jaekitData 'examples\check-result') -Destination $jaekitExample -Recurse
```

참조 생산자는 같은 버전의 `tools\check-result-reference.py`에 있습니다. 프로젝트에서 쓸 때는 예시 README의 선언·생산자 경로를 프로젝트 배치에 맞춥니다. 관리되는 설치 폴더 안에서 예시나 문서를 편집하지 않습니다. 그 폴더의 파일을 바꾸거나 새 파일을 넣으면 업데이트·제거가 자료 보존을 위해 멈춥니다.

PowerShell에서 `ha`를 호출하더라도 PLAN의 검사 명령은 기존 argv 형식입니다. 파이프·리다이렉트·PowerShell 식을 자동 해석하지 않습니다. 절대 Windows 경로는 역슬래시를 유지하는 따옴표 표기를 쓰거나 `/` 구분자를 사용합니다. 아래는 명령 표기의 합성 예시이며 실제 검사 결과가 아닙니다.

```text
'C:\Program Files\Example\checker.exe' 'C:\작업 폴더\input.txt'
C:/Tools/checker.exe 'C:/작업 폴더/input.txt'
```

셸 스크립트를 실행하려면 실행할 셸과 스크립트 인수를 명시합니다. 필요한 셸과 실행 정책은 프로젝트의 선택이며 `ha`가 이를 준비하거나 변경하지 않습니다.

Git이 clean이라고 해도 CRLF 변환 때문에 선언·생산자 파일의 바이트가 HEAD와 다를 수 있습니다. 기존 증거의 바이트 동일성 조건을 유지하므로, 이때는 검사 파일의 실제 내용과 프로젝트의 `.gitattributes`·checkout 조건을 확인합니다. 승인된 프로젝트 정책에 따라 checkout을 맞추고 다시 검사합니다. 설치기나 Core가 전역 `core.autocrlf`, 파일 바이트, 과거 digest·기록을 자동으로 바꾸지 않습니다.

## 재설치·업데이트·실패

같은 버전의 같은 후보는 위 설치 블록 전체를 다시 실행하여 재검증한 뒤 재설치합니다. 다른 내용의 동일 버전 후보는 기존 자료와 충돌하므로 거부됩니다. 업데이트할 때는 새 후보의 여섯 파일만 있는 폴더로 이동한 뒤 위 설치 블록 전체를 실행합니다. 이전 후보의 검증 결과를 새 후보에 재사용하지 않습니다. PowerShell 5.1에서 설치한 결과를 PowerShell 7로 업데이트하거나 반대로 사용할 수 있도록 같은 사용자 경로와 관리 상태를 사용합니다.

설치기는 checksum·전체 파일 목록·내용을 확인하고 새 트리를 준비한 뒤 기존 설치를 교체합니다. 이전 버전 자료는 자동 삭제하지 않습니다. 무결성 불일치, 링크/junction 경로, 관리되지 않거나 편집된 파일, 사용 중이어서 교체할 수 없는 실행 파일, 파일 접근 실패는 성공으로 표시하지 않습니다. 사용 중인 `ha`와 검사를 종료하거나 충돌 원인을 확인한 뒤 다시 시도합니다. 기존 정상 설치와 프로젝트 파일은 보존하며, 교체 후 실패하면 이전 설치를 복구합니다.

패키지 경로에서는 대소문자와 확장자에 관계없이 `CONIN$`·`CONOUT$` 구성요소를 거부합니다. 이런 이름이 기존 설치 목록에 있으면 재설치·업데이트·제거를 중단하고 기존 파일을 보존합니다. 이 검사는 이름이 포함된 자료를 자동으로 삭제하거나 바꾸지 않습니다.

관리되는 설치 루트·하위 디렉터리·파일에 이름 있는 추가 데이터 스트림(ADS)이 있으면 재설치·업데이트·제거를 변경 전에 중단합니다. 기본 파일 내용이 같아도 빈 스트림과 `Zone.Identifier`를 포함한 추가 스트림은 보존 대상입니다. 스트림 조사에 실패한 경우도 “없음”으로 처리하지 않습니다. 기본 unnamed 데이터와 파일시스템 자체의 구조 metadata는 추가 자료로 오인하지 않으며, 스트림 미지원이 확인된 기존 로컬 파일시스템은 계속 사용할 수 있습니다.

이 정책은 관리·교체·삭제할 트리와 추출된 후보에 적용됩니다. 읽기만 하는 원본 ZIP·checksum·개별 후보 스크립트의 다운로드 metadata까지 새로 거부하거나 지우지 않습니다. 거부 진단의 대상과 스트림을 확인하세요. 설치기는 자동 스트림 삭제·차단 해제·이관을 하지 않습니다. 실패 후 임시 폴더나 이전 백업 정리에서도 추가 스트림 또는 조사 오류가 있으면 그 트리를 보존하고 위치를 알립니다. 이미 설치·제거가 적용된 뒤 정리만 실패했다면 적용 결과와 남은 자료를 구분해 확인하세요.

복구 자체나 마지막 임시 자료 정리가 실패하면 남은 백업 위치를 알립니다. 그 상태에서는 표시된 경로를 확인하고 보존된 자료를 비교한 뒤 정리합니다. 무관한 사용자 자료를 포함한 폴더를 강제로 삭제하거나 링크 대상을 따라가 삭제하지 않습니다. 이 설치기는 같은 사용자 권한의 다른 프로세스가 의도적으로 경로를 바꾸는 상황을 격리하지 않습니다.

## 제거

`ha`와 검사를 종료합니다. 위 함수 정의를 준비하고, 신뢰한 checksum과 여섯 파일을 보관한 후보 폴더에서 아래 블록 전체를 실행합니다. 제거 코드와 `common.ps1`도 실행 전에 다시 검증합니다. 검증을 생략하고 설치 내부 사본을 바로 실행하지 않습니다.

<!-- BEGIN WINDOWS VERIFIED REMOVE -->
```powershell
& {
    $candidate = Get-JaekitVerifiedCandidate (Get-Location).Path
    & $candidate.Uninstall
}
```
<!-- END WINDOWS VERIFIED REMOVE -->

제거기는 전체 관리 목록을 확인한 뒤 해당 사용자 설치와 설치기가 추가한 PATH 한 항목만 제거합니다. 설치 뒤 같은 문자열의 PATH를 사용자가 뒤쪽에 추가했다면, 앞의 설치기 항목 하나만 제거하고 뒤의 사용자 항목과 나머지 표기·순서를 보존합니다. 따라서 사용자가 추가한 동일 경로 문자열은 남을 수 있습니다. 사용자 항목의 임의 재배열 이력을 추적하는 기능은 아닙니다. 예전 버전의 관리 자료도 함께 제거됩니다. 기존부터 있던 PATH 항목과 무관한 설정·다른 사용자 설치·WSL 설치는 제거하지 않습니다. 프로젝트에 복사한 검사·예시, 목표 문서, `runs.jsonl`, Git 디렉터리의 원문 로그는 설치 관리 대상이 아니므로 보존됩니다.

편집된 파일·미등록 파일·링크·사용 중인 실행 파일 때문에 멈추면 그 이유를 먼저 확인합니다. 이를 우회하는 강제 삭제 옵션은 없습니다.

## 확인 범위

[후보 도구 안내](../tools/windows/README.md)는 패키지 생성과 실제 Windows 설치 생명주기를 확인하는 별도 진입점을 설명합니다. `acceptance.ps1`을 사용할 때는 그 안내의 블록으로 첫 후보와 업데이트 후보의 여섯 파일을 모두 먼저 검증하고, 실제 사용자 설치에 명시적으로 동의합니다. 각 단계는 해당 후보의 설치·제거 스크립트와 helper를 사용합니다. 업데이트·새 후보 재설치·업데이트 후 제거는 새 후보 코드의 실행과 실패를 확인하며, 첫 후보 코드로 대신하지 않습니다. Server 개발자 CI의 사전 검증 성공은 Windows 11 설치 성공이 아닙니다. 로컬 패키지 회귀, Windows 교차 빌드, PowerShell 5.1·7의 실제 설치·업데이트·제거, Core 목표 실행, 에이전트 Skill 사용, 공개 URL 설치를 각각 구분합니다. 수행하지 않은 항목은 미확인으로 남깁니다.
