# 형식과 규칙 한눈에 보기

**한국어** · [English](FORMATS.en.md)

Jaekit이 쓰고 읽는 파일의 형식과 완료를 판단하는 규칙은 계약 문서에 정해져 있습니다. 계약 문서는 영어 한 벌로만 관리합니다. 이 페이지는 각 계약이 무엇을 정하는지 한국어로 소개하고, 정확한 정의가 있는 영어 원문으로 안내합니다.

> 계약은 정확한 형식과 판정을 적은 참고 문서입니다. Jaekit을 쓰려고 먼저 읽을 필요는 없습니다. 쓰는 법은 [사용 안내](USAGE.md)에 있습니다.

## 언제 무엇을 읽나요

| 궁금한 것 | 읽을 계약 |
| --- | --- |
| `SPEC.md`에 무엇을 어떻게 쓰나요 | [목표 문서](../contracts/goal-docs.md) |
| `PLAN.md`, `REVIEW.md`, `PROGRESS.md`와 완료 보고는 어떤 모양인가요 | [실행 묶음](../contracts/bundle.md) |
| `ha` 명령, `runs.jsonl`의 한 줄, 완료 계산, 규칙 버전 | [실행 기록](../contracts/run-record.md) |
| 바꾸기 전 실패(baseline)를 검사 결과로 어떻게 가리나요 | [change 검사 결과](../contracts/check-result.md) |
| Seal을 따르는 에이전트가 할 수 있는 것과 없는 것 | [Seal 실행자의 역할](../contracts/role-card.md) |
| `spar.toml`과 리뷰어 설정 | [Spar 설정](../contracts/spar-config.md) |
| Spar 라운드, 패킷, 지적의 분류와 처리 | [검토 라운드](../contracts/review-round.md) |
| 검토가 끝났는지 판정하는 gate 파일 | [검토 gate](../contracts/review-gate.md) |
| `spar`가 받는 입력 하나하나의 형식 | [Spar 입력 형식](../contracts/spar-input.md) |
| 설치된 `ha`가 필요한 기능을 지원하는지 Seal이 확인하는 방법 | [Core 기능](../contracts/core-capabilities.md) |
| 완료 뒤에 발견한 결함을 남기는 방법 | [완료 뒤 발견](../contracts/findings.md) |

## 계약마다 다루는 것

### 목표 문서

Spec이 쓰고 Seal이 읽는 Markdown의 형식입니다. 두 제품 사이의 유일한 접점입니다. [원문: Goal documents](../contracts/goal-docs.md)

- 목표 폴더의 위치와 문서 묶음
- `SPEC.md`의 내용: 완료 조건(`AC-n`)과 사용 시나리오
- 문서 사이의 일관성과 Spec이 문서를 쓰는 순서

### 실행 묶음

Seal이 목표 폴더에 남기는 파일의 형식입니다. [원문: Execution bundle](../contracts/bundle.md)

- `PLAN.md`: 조건표, 바꿀 수 있는 범위, 예산
- `tasks/T00n.md`, `REVIEW.md`, `PROGRESS.md`
- 완료 보고와 `ha lint`의 오류 코드

### 실행 기록

`ha` 명령, `runs.jsonl` 한 줄의 형식, 그리고 완료를 계산하는 규칙입니다. 가장 긴 계약이고, 규칙 버전마다 달라지는 판정이 여기에 모여 있습니다. [원문: Run record](../contracts/run-record.md)

- `ha` 명령
- 줄 형식과 신선도(지금 코드에 맞는 기록인지)
- 상태 계산(`ha status`, `ha done`), 완료 기록과 예산
- 규칙 버전

### change 검사 결과

change 조건의 바꾸기 전 검사와 지금 검사를 같은 선언에 묶는 형식입니다. 0이 아닌 종료 코드를 곧바로 요구 위반으로 보지 않고, 검사가 낸 보고로 가립니다. [원문: Change check results](../contracts/check-result.md)

- 선언과 입력, 결과 계약
- 실행마다 남기는 보고와 그 분류
- Go 테스트 같은 러너 어댑터

### Seal 실행자의 역할

Seal skill이 일하는 에이전트에게 요구하는 권한의 경계와 완료 규칙입니다. 따로 돌아가는 역할 관리자나 독립 검증 서비스가 있다는 뜻은 아닙니다. [원문: Role of the Seal executor](../contracts/role-card.md)

### Spar 설정

`spar`가 어떤 리뷰어를 시작하고 어떤 상한·관점·규칙·검사 경로를 쓰는지 정하는 두 설정 파일의 형식입니다. 리뷰어는 사용자 설정에서만 고를 수 있고, 저장소의 프로젝트 설정은 리뷰어를 고르지 못합니다. [원문: Spar configuration](../contracts/spar-config.md)

### 검토 라운드

`spar run`이 라운드 하나를 돌리고 남기는 `review-round/v1` 기록입니다. [원문: Review round](../contracts/review-round.md)

- 패킷과 작업 공간, 리뷰어의 결과와 상태
- 지적의 분류와 처리
- 상한, 재실행, 이전 라운드와의 연결

### 검토 gate

`spar gate`가 라운드 목록과 기대하는 head로 검토가 끝났는지 판정해 쓰는 `review-gate/v1` 파일입니다. 검토 기록이지 완료의 근거는 아닙니다. 완료는 Seal의 `ha done`처럼 그 규칙을 가진 쪽이 정합니다. [원문: Review gate](../contracts/review-gate.md)

### Spar 입력 형식

`spar`가 리뷰어 결과, 설정 파일, 명령 인자, 다시 읽는 기록에서 받는 입력 필드마다 받아들이는 형식과, 형식 밖 입력의 결과를 정합니다. [원문: Spar input formats](../contracts/spar-input.md)

### Core 기능

설치된 `ha`가 어떤 규칙과 기능을 지원하는지 Seal이 목표 없이 묻고 확인하는 방법입니다. 필요한 기능이 없으면 Seal은 기록을 남기지 않고 멈춥니다. [원문: Core capabilities](../contracts/core-capabilities.md)

### 완료 뒤 발견

완료 뒤에 발견한 결함 주장을 출처와 함께, 그때의 완료와 따로 남기는 형식입니다. Core는 참조와 이력의 구조만 확인하고, 결함이 진짜인지는 판정하지 않습니다. [원문: Post-completion findings](../contracts/findings.md)

## 저장소 개발용 문서

[공통 회귀 연결](../contracts/regressions.md)과 [저장소 검사 생산자](../contracts/repository-producers.md)는 Jaekit 저장소를 개발할 때 쓰는 문서라 "Jaekit 개발" 묶음에 있습니다. 계약 전체의 영어 목록은 [계약 개요](../contracts/README.md)에 있습니다.
