# 보증 정책과 반례의 연결

가상 형식 예시이며 실제 실행 기록이 아니다. 아래 observed·performed·complete는 가상 목표 안의 설명 값이다. 이 문서는 실제 셸·모델·검토 세션을 수행했다는 증거가 아니며 Core의 충분성 판정도 아니다.

## 정책과 기대 근거

goal-policy-A는 실행 사실을 인증하는 AC-1의 별도 반례 검토와 실제 Bash 비교, 실제 모델 계약을 주장하는 AC-2의 실제 경계 관측을 필수로 정한 가상 사용자 결정이다. project-policy-B는 AC-3의 순수한 입력 정규화 결과만 보장하고 모델 연동은 지원 범위 밖이므로 추가 독립 검토·실제 모델 비교를 선택으로 둔다. 두 대상 모두 잘못된 수락의 영향이 크지만 필수 관측 범위는 다르다. goal-policy-C의 AC-4는 오탈자 수정이고 외부 상태·권한·실행 경계가 바뀌지 않아 낮은 위험으로 별도 검토를 요구하지 않는다.

규범적 기대 근거는 각각 요청 안의 실행 의미·모델 응답 계약·정규화 불변식·문구 수정 범위다. 실제 Bash나 fake 기대값 자체를 정답의 권위로 삼지 않는다. 비교할 때 같은 입력, Bash 버전·환경과 도달성 성질을 연결하고 실제 관측하지 않은 경계는 분리한다. 구조화 실행 원본이 있으면 호출 ID·대상 연결을 사용하며 텍스트만으로 실행을 추정하지 않는다.

## 보증 연결

| 사례 | 조건 | 위험 | 정책 출처 | 독립 검토 | 실제 비교 | 반례 결과 | 완료 | 코드 | 검사 | 검토 | 근거 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| counterexample-fixed | AC-1 | high | goal-policy-A | required:performed | required:observed | reproduced:fixed | complete | code-v2 | check-v2 | R2 | E2 |
| false-positive | AC-1 | high | goal-policy-A | required:performed | required:observed | rejected:contract-permits | complete | code-v2 | check-v2 | R2 | E2 |
| review-unavailable | AC-1 | high | goal-policy-A | required:unavailable | required:observed | none | pending | code-v1 | check-v1 | none | G1 |
| real-unavailable | AC-2 | high | goal-policy-A | optional:not-performed | required:unavailable | none | pending | code-v1 | check-v1 | none | G2 |
| optional-comparison | AC-3 | high | project-policy-B | optional:not-performed | optional:unobserved | none | complete | code-v1 | check-v1 | none | E3 |
| low-risk | AC-4 | low | goal-policy-C | not-required | not-required | none | complete | code-v1 | check-v1 | none | E4 |

## 검토 내역

| ID | 조건 | 코드 | 검사 | 초기 입력 | 추가 맥락 | 역할 | 결과·한계 | 근거 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| R1 | AC-1 | code-v1 | check-v1 | 목표, 계약, 검사, 기대 근거 | 해당 버전 구현·통합 코드 | 별도 반례 검토자 | CP1 후보와 CP2 후보; 전체 셸 지원 보장 아님 | E1 |
| R2 | AC-1 | code-v2 | check-v2 | 목표, 계약, 검사, 기대 근거 | 해당 버전 구현·통합 코드 | 별도 반례 검토자 | CP1 수정 재확인; 해당 성질·환경에 한정 | E2 |

R1을 v2의 검토로 재사용하지 않는다. 실제로 별도 검토가 없으면 이름이나 모델 표기만 바꿔 R2를 만들 수 없다. 이 예시의 역할 분리는 인증된 신원이나 모델 다양성 보증이 아니다. 실행자는 필수 보증 안에서 시점·모델·분업을 선택하고 특정 baseline 전 호출 순서를 따를 의무는 없다.

## 반례 내역

| ID | 입력·상황 | 예상 위반·검사의 공백 | 확인 | 일반 성질 | 처리 | 남은 범위 |
| --- | --- | --- | --- | --- | --- | --- |
| CP1 | exec true 뒤 ha 호출 텍스트 | AC-1; 문자열 존재를 실행으로 인정 | reproduced | 도달하지 않은 명령은 실행 근거가 아니다 | check-v2; code-v2; E2 | 지원 밖 셸 문법은 판정 불가 |
| CP2 | 허용된 대상 ID의 실제 실행 | AC-1 위반 의심이나 공개 계약은 허용 | rejected | 대상과 호출의 연결 유지 | 공개 계약의 허용 입력; E1 | 다른 대상 ID는 별도 경계 |

확인된 위반을 위험 문구만 추가하여 닫지 않는다. CP1의 원인과 같은 도달 불가 계열을 check-v2에서 다루고 실제 비교와 수정 근거에 연결한다. 원래 조건 밖 기능 제안은 별도 결정으로 남긴다. 미관측·환경 오류·skip·실제 위반은 서로 다르고, 판정 불가는 성공이나 baseline 위반으로 승격되지 않는다. 제품이 입력을 거부하는 것과 근거를 충분하다고 보는 것은 다른 판단이다.

## 부족한 필수 근거

| ID | 영향 조건 | 원인 | 부족한 근거 | 해소 조건 | 계속할 작업 |
| --- | --- | --- | --- | --- | --- |
| G1 | AC-1 | 검토 실행 환경 접근 불가 | 별도 반례 검토 | 권한 범위 안에서 검토 환경 확보·수행 | 무관한 문구 수정 |
| G2 | AC-2 | 실제 모델 호출 권한 없음 | 실제 응답 계약 관측 | 사용자 권한 범위 내 실제 경계 관측 | 정규화 단위 검사 |

불가 사유를 적는 것만으로 필수 보증이 충족되지 않는다. 두 경우 완료를 보류하며 자동 설치·비용 증가·권한 확대·허구 검토를 하지 않는다. 다른 작업은 원래 권한과 예산 안에서 진행한다.

## 가상 근거와 인계

E1은 R1이 본 code-v1·check-v1의 지원 Bash 입력과 CP2의 계약상 허용 결과를 가리킨다. E2는 R2·check-v2·code-v2의 가상 근거다. 같은 지원 Bash 환경에서 도달 불가 입력의 실제 호출 없음과 정상 호출의 존재를 비교했다고 가정한 형식이다. 이 문서가 그 실행 자체는 아니다. 미검토 셸·구조화 ID가 없는 로그의 정확성은 보장하지 않는다.

E3의 fake 결과는 실제 모델 관측이 아니다. AC-3의 순수 정규화 결과는 범위 안에서 설명되지만 선택 모델 비교는 미관측으로 남는다. E4는 문구 변경과 기존 내용 보존을 설명한다. 어느 예시도 검사 수·반례 수·모델 이름만으로 충분성이나 미래 결함 예방률을 주장하지 않는다.
