# Windows 네이티브 후보 설치 안내

**한국어** · [English](WINDOWS.en.md)

이 안내는 **Windows 11 x64, Windows PowerShell 5.1·PowerShell 7**을 대상으로 두 경로를 다룹니다. 하나는 Windows 파일이 든 공개 릴리스에서 설치하는 [공개 릴리스 경로](#공개-릴리스로-설치)이고, 다른 하나는 그 아래의 로컬 후보 설치 경로입니다. 현재 공개 배포판 `v0.1.8`에 Windows 파일이 있다는 뜻이 아닙니다. 후보 패키지 생성과 교차 빌드는 Windows에서의 설치·실행 관측이나 에이전트의 실제 Skill 사용을 증명하지 않습니다. 해당 후보의 확인 결과와 미확인 범위를 함께 읽습니다.

Windows ARM64·x86, 이전 Windows, 네트워크/UNC 저장소와 같은 checkout의 Windows·WSL 교차 사용은 이 경로의 지원 대상이 아닙니다. WSL 지원과 네이티브 지원은 별개입니다.

## 공개 릴리스로 설치

이 경로는 Go·Python·Git·Jaekit checkout 없이, 공개 릴리스의 파일과 PowerShell 블록 하나로 `ha`를 설치합니다. Windows 11 x64의 Windows PowerShell 5.1 또는 PowerShell 7 창에 블록 전체를 고치지 않고 붙여 넣습니다. 64비트 창을 쓰고, Git과 에이전트는 따로 준비합니다.

블록은 다음 순서로만 진행합니다.

- 블록에 적힌 버전의 `ha_<version>_windows_amd64.zip`과 `checksums.txt` 두 파일만 GitHub Release에서 받습니다.
- `checksums.txt`에 그 ZIP이 정확히 한 번 있고 SHA-256이 맞을 때만 다음으로 넘어갑니다. 받기에 실패하거나 항목이 없거나 둘 이상이거나 값이 다르면, ZIP 안의 어떤 코드도 실행하지 않고 멈춥니다.
- 확인한 ZIP에서 설치 코드(`tools/windows/install.ps1`과 `common.ps1`)만 꺼냅니다. 그 코드를 지금 창과 같은 PowerShell 실행 파일(5.1이면 Windows PowerShell, 7이면 PowerShell 7)의 새 프로세스에서 `-NoProfile -ExecutionPolicy Bypass -File`로 실행합니다. 설치 코드는 ZIP과 체크섬을 다시 확인하고 `-AddToPath`로 현재 사용자 PATH에 설치 폴더 한 항목을 추가합니다.
- 설치 코드가 실패하면 블록도 성공 안내 없이 실패로 끝나고, 이 창의 PATH는 바꾸지 않습니다. 성공하면 이 창의 PATH에 설치 폴더가 없을 때만 덧붙여 바로 `ha`를 쓸 수 있게 합니다. 다른 `ha`가 앞에 있으면 경고합니다. 이미 열린 다른 터미널과 앱은 다시 시작해야 새 PATH를 읽습니다.
- 받은 임시 파일은 끝나면 지우고, 지우지 못하면 그 위치를 알립니다.

**실행 정책.** Windows 11의 기본 실행 정책은 Restricted이지만, 붙여 넣은 블록은 스크립트 파일이 아니라 명령이므로 실행됩니다. 블록은 영구 실행 정책(CurrentUser·LocalMachine)을 바꾸지 않고, 바꾸라고 요구하지도 않습니다. Bypass는 검증한 설치·제거 코드를 실행하는 그 새 프로세스와 그 하위 프로세스에만 적용됩니다. 그룹 정책(MachinePolicy·UserPolicy)으로 정한 실행 정책은 이 옵션보다 우선합니다. 회사 정책이 스크립트를 막으면 블록은 아무것도 설치하지 않고 실패로 끝납니다. 아래 [후보 경로](#준비)는 이 옵션을 쓰지 않습니다.

**체크섬.** 체크섬은 받은 ZIP이 같은 릴리스의 목록과 같은지 확인할 뿐, 출처를 인증하는 서명이 아닙니다. 이 안내와 블록은 신뢰한 출처에서 읽습니다.

아직 Windows 파일이 든 공개 릴리스는 없으며, 현재 배포판 `v0.1.8`에도 Windows 파일이 없습니다. 아래 블록은 Windows 파일이 든 첫 릴리스가 게시된 뒤부터 동작합니다. 블록의 버전 값(예정 `0.1.9`)은 그 릴리스를 준비할 때 확정합니다. 그 전에는 아래의 후보 경로를 씁니다.

<!-- BEGIN WINDOWS PUBLIC INSTALL -->
```powershell
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $version = '0.1.9'
    $source = 'https://github.com/jgoneit/jaekit/releases/download/v' + $version
    $name = 'ha_' + $version + '_windows_amd64.zip'
    $shell = (Get-Process -Id $PID).Path
    if (@('powershell.exe', 'pwsh.exe') -notcontains [IO.Path]::GetFileName($shell).ToLowerInvariant()) {
        throw 'Paste this block into Windows PowerShell 5.1 or PowerShell 7.'
    }
    $work = Join-Path ([IO.Path]::GetTempPath()) ('jaekit-' + [Guid]::NewGuid().ToString('N'))
    [void][IO.Directory]::CreateDirectory($work)
    try {
        $archive = Join-Path $work $name
        $checksums = Join-Path $work 'checksums.txt'
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/' + $name) -OutFile $archive
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/checksums.txt') -OutFile $checksums
        $listed = @([IO.File]::ReadAllLines($checksums) | Where-Object { $_.EndsWith('  ' + $name) })
        if ($listed.Count -ne 1 -or $listed[0] -cnotmatch ('^[0-9a-f]{64}  ' + [regex]::Escape($name) + '$')) {
            throw ('checksums.txt must list ' + $name + ' exactly once.')
        }
        if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $listed[0].Substring(0, 64)) {
            throw ('SHA-256 mismatch: ' + $name)
        }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($archive)
        try {
            foreach ($script in 'common.ps1', 'install.ps1') {
                $entries = @($zip.Entries | Where-Object { $_.FullName -ceq ('tools/windows/' + $script) })
                if ($entries.Count -ne 1 -or $entries[0].Length -gt 1048576) {
                    throw ('The archive must contain one tools/windows/' + $script)
                }
                [IO.Compression.ZipFileExtensions]::ExtractToFile($entries[0], (Join-Path $work $script))
            }
        } finally { $zip.Dispose() }
        Write-Output ('Running the verified installer in ' + $shell)
        $ErrorActionPreference = 'Continue'
        & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $work 'install.ps1') -Archive $archive -Checksums $checksums -AddToPath
        $code = $LASTEXITCODE
        $ErrorActionPreference = 'Stop'
        if ($code -ne 0) { throw ('The installer stopped with exit code ' + $code + '; see its message above.') }
        $bin = Join-Path (Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Jaekit') 'bin'
        $exe = Join-Path $bin 'ha.exe'
        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw ('The installer exited 0, but ' + $exe + ' is missing.') }
        if (@($env:Path -split ';' | Where-Object { $_.TrimEnd('\') -ieq $bin }).Count -eq 0) {
            if ($env:Path -and -not $env:Path.EndsWith(';')) { $env:Path += ';' }
            $env:Path += $bin
        }
        $found = @(Get-Command ha -ErrorAction SilentlyContinue)
        if ($found.Count -eq 0 -or $found[0].CommandType -ne 'Application' -or $found[0].Source -ine $exe) {
            Write-Warning ('Another ha comes first in this window. Run ' + $exe + ' directly or reorder PATH.')
        }
        Write-Output ('ha ' + $version + ' is ready in this window. Restart other open terminals and apps, or sign out and in, so they load the new PATH.')
    } finally {
        try { [IO.Directory]::Delete($work, $true) } catch { Write-Warning ('Remove the temporary files yourself: ' + $work) }
    }
}
```
<!-- END WINDOWS PUBLIC INSTALL -->

### 공개 릴리스 업데이트

새 버전 안내에 있는 설치 블록을 그대로 다시 붙여 넣습니다. 설치 코드는 같은 사용자 위치의 설치를 새 버전으로 바꾸고, 이전 버전 자료는 지우지 않습니다. 같은 버전은 다시 검증한 뒤 재설치합니다. PowerShell 5.1로 설치한 것을 PowerShell 7에서 업데이트해도 같은 관리 상태를 씁니다. 업데이트가 실패하면 설치 코드가 기존 설치를 복구하고, 블록은 실패로 끝납니다.

### 공개 릴리스 제거

`ha`와 검사를 종료한 뒤 아래 블록을 붙여 넣습니다. 설치 블록과 같은 방식으로 받은 파일을 확인한 뒤, 확인한 ZIP의 제거 코드(`tools/windows/uninstall.ps1`과 `common.ps1`)만 실행합니다. 이미 설치된 사본을 검증 없이 실행하지 않습니다. 이 블록도 Windows 파일이 든 첫 공개 릴리스가 게시된 뒤부터 동작합니다.

<!-- BEGIN WINDOWS PUBLIC REMOVE -->
```powershell
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $version = '0.1.9'
    $source = 'https://github.com/jgoneit/jaekit/releases/download/v' + $version
    $name = 'ha_' + $version + '_windows_amd64.zip'
    $shell = (Get-Process -Id $PID).Path
    if (@('powershell.exe', 'pwsh.exe') -notcontains [IO.Path]::GetFileName($shell).ToLowerInvariant()) {
        throw 'Paste this block into Windows PowerShell 5.1 or PowerShell 7.'
    }
    $work = Join-Path ([IO.Path]::GetTempPath()) ('jaekit-' + [Guid]::NewGuid().ToString('N'))
    [void][IO.Directory]::CreateDirectory($work)
    try {
        $archive = Join-Path $work $name
        $checksums = Join-Path $work 'checksums.txt'
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/' + $name) -OutFile $archive
        Invoke-WebRequest -UseBasicParsing -Uri ($source + '/checksums.txt') -OutFile $checksums
        $listed = @([IO.File]::ReadAllLines($checksums) | Where-Object { $_.EndsWith('  ' + $name) })
        if ($listed.Count -ne 1 -or $listed[0] -cnotmatch ('^[0-9a-f]{64}  ' + [regex]::Escape($name) + '$')) {
            throw ('checksums.txt must list ' + $name + ' exactly once.')
        }
        if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $listed[0].Substring(0, 64)) {
            throw ('SHA-256 mismatch: ' + $name)
        }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($archive)
        try {
            foreach ($script in 'common.ps1', 'uninstall.ps1') {
                $entries = @($zip.Entries | Where-Object { $_.FullName -ceq ('tools/windows/' + $script) })
                if ($entries.Count -ne 1 -or $entries[0].Length -gt 1048576) {
                    throw ('The archive must contain one tools/windows/' + $script)
                }
                [IO.Compression.ZipFileExtensions]::ExtractToFile($entries[0], (Join-Path $work $script))
            }
        } finally { $zip.Dispose() }
        Write-Output ('Running the verified uninstaller in ' + $shell)
        $ErrorActionPreference = 'Continue'
        & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $work 'uninstall.ps1')
        $code = $LASTEXITCODE
        $ErrorActionPreference = 'Stop'
        if ($code -ne 0) { throw ('The uninstaller stopped with exit code ' + $code + '; see its message above.') }
        Write-Output 'Removal finished. Terminals and apps that were already open keep their old PATH until they restart.'
    } finally {
        try { [IO.Directory]::Delete($work, $true) } catch { Write-Warning ('Remove the temporary files yourself: ' + $work) }
    }
}
```
<!-- END WINDOWS PUBLIC REMOVE -->

제거 코드는 관리 목록을 확인한 뒤 설치와 설치기가 추가한 PATH 한 항목만 지웁니다. 프로젝트의 목표 문서·기록·로그는 남습니다. 이미 열린 창과 앱은 다시 시작할 때까지 예전 PATH를 유지합니다.

### 공개 경로의 확인 범위

Windows Server 2025의 CI와 릴리스 smoke는 PowerShell 5.1과 7 각각에서 이 블록 텍스트를 실제로 실행합니다. 다운로드, 체크섬 확인, 같은 셸의 자식 실행, 실패 전달, 창의 PATH, 붙여 넣은 뒤에도 그 창에서 스크립트가 실행되지 않음을 관측합니다. 설치 코드는 Windows 11 Client만 허용하므로, Server 결과는 Windows 11 설치 성공이 아닙니다. 실제 설치·재설치·제거는 [Windows 11 확인 절차](../tools/windows/README.md)에서 확인합니다. 공개 릴리스는 그 확인과 연결된 Windows ZIP만 게시하고, 연결이 없으면 macOS·Linux 파일만 게시합니다.

## 준비

- 64비트 Windows PowerShell 5.1 또는 PowerShell 7을 엽니다. Git과 사용할 에이전트는 별도로 준비합니다. 설치기는 Git·에이전트·PowerShell·Windows 기능을 설치하거나 관리자 권한을 요청하지 않습니다.
- 검토한 같은 후보 묶음에서 `ha_<version>_windows_amd64.zip`, `checksums.txt`, `install.ps1`, `uninstall.ps1`, `common.ps1`, `acceptance.ps1`을 받습니다. 이 여섯 파일만 들어 있는 전용 폴더를 사용합니다. 설치기는 로컬 파일만 읽고 다운로드나 공개 배포를 수행하지 않습니다. 취득에 실패했으면 설치가 시작된 것이 아닙니다.
- checksum은 가져온 파일들이 후보의 내용과 맞는지 확인합니다. 같은 출처에서 변조된 checksum까지 신뢰할 수 있게 만드는 서명은 아닙니다. 스크립트도 실행할 코드이므로 출처와 내용을 확인합니다.
- 회사 또는 PC의 실행 정책이 스크립트를 막으면 해당 정책에 따라 처리합니다. 이 후보 경로의 설치기는 실행 정책·보안 설정을 바꾸거나 우회 옵션으로 다시 실행하지 않습니다. 공개 릴리스 경로만 검증한 설치·제거 코드에 한해 프로세스 한정 Bypass를 씁니다([실행 정책](#공개-릴리스로-설치)).

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

## 선택적 판단 이력

현재 소스의 Windows x64 로컬 후보는 [판단 이력](../contracts/decision-history.md)의 등록·재전송·정정·기본/current·list·사건 ID 또는 요청 식별자로 하는 show를 제공합니다. 실제 선택한 실행 파일의 `features`와 `formats.decision_history`에 모두 `decision-history/v1`이 있는지 확인합니다. 공개 배포판 v0.1.8에는 Windows 파일이 없고 v0.1.4 이하에는 이 확장이 없으며 버전 숫자만으로 지원을 추정하지 않습니다.

로컬 드라이브의 일반 checkout과 linked worktree는 공백·한글 경로에서도 같은 Git 공통 디렉터리의 이력과 잠금을 공유합니다. 저장 경로의 junction/reparse point는 `history_unreadable`로 거부합니다. 기존 상대 경로 문법을 통과했지만 Windows 장치·named pipe·ADS·드라이브 상대 경로로 해석되는 파일 근거는 열지 않고 `unreadable`·버전 미식별로 남기며, 유효한 사건 전체를 거부하지 않습니다. 기존 자료와 macOS/Linux 파일 이름의 의미를 바꾸거나 자동으로 복구하지 않습니다.

[CI](CI.md)는 Windows Server에서 PowerShell 5.1·7 각각의 실제 판단 이력과 기존 Core 경계를 검사합니다. 이는 Windows 11 설치·에이전트 Skill 사용이나 공개 Windows 배포를 확인한 결과가 아닙니다. 이 안내의 기존 플랫폼·로컬 저장소 범위는 그대로 적용됩니다.

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
