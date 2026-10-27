# 소스 빌드와 개발 등록

**한국어** · [English](DEVELOPMENT.en.md)

Jaekit 자체를 고치거나 시험할 때 읽는 문서입니다. Jaekit을 쓰는 데는 필요 없습니다. 쓰려면 [설치 안내](INSTALL.md)를 따릅니다.

## 소스에서 빌드 (개발)

저장소 checkout에서 빌드한 `ha`는 개발 버전(`ha 0.1.8-dev`)을 표시합니다. Go 1.27 이상과 git이 필요합니다. checkout의 루트에서 실행합니다.

```bash
go install ./cmd/ha
ha --version
```

`ha` 파일은 `go env GOBIN`이 가리키는 폴더에 생기고, 그 값이 비어 있으면 `$(go env GOPATH)/bin`에 생깁니다. `ha --version`은 `ha 0.1.8-dev`를 출력합니다. Seal은 별도로 `ha capabilities --format json`의 실제 지원 정보를 확인합니다. 선택적인 Python 호환 검사 도우미도 있지만 Python 3 설치가 Seal의 필수 조건은 아닙니다.

checkout을 고치며 plugin을 쓰려면 checkout 경로를 marketplace로 등록합니다. 이 등록은 checkout의 plugin 파일을 그대로 씁니다. `<jaekit 경로>`는 checkout의 경로로 바꿉니다.

Codex:

```bash
codex plugin marketplace add <jaekit 경로>
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace add <jaekit 경로>
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

명령과 기록 형식은 [contracts/run-record.md](../contracts/run-record.md)에 있습니다.

## 저장소 검사

바꾼 뒤에는 저장소 루트에서 공통 검사를 실행합니다. 필요한 도구와 검사 범위는 [저장소 지침의 Validation](../AGENTS.md#validation)에 있습니다.

```bash
python3 tools/verify.py
```

## 더 읽을 것

- [CI 결과 읽기](CI.md): PR과 main에서 도는 검사와 환경별 관측 범위
- [Windows 네이티브 후보 안내](WINDOWS.md): 개발 중인 Windows 네이티브 후보의 검증 경로
- [공통 규칙과 회귀 검사 연결](../contracts/regressions.md): Jaekit의 공통 규칙과 그것을 지키는 회귀 검사
- [저장소 검사 생산자의 관측 의미](../contracts/repository-producers.md): 저장소 검사 도구가 무엇을 관측하는지
