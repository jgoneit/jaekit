# 공개 제품 계약

현재 Spec·Seal의 문서 형식과 상태 계산, Spar의 설정과 검토 기록 형식을 정의한다. 예시의 사용자 요청·시각·기록 번호는 형식을 설명하기 위한 것이며 실제 작업의 증거가 아니다.

- [목표 문서](goal-docs.md): Spec이 만들고 Seal이 읽는 Markdown
- [실행 묶음](bundle.md): 계획, 진행과 완료 보고
- [실행 기록](run-record.md): `ha` 명령, `run/v1`, `status/v1`, 규칙 버전
- [판단 이력](decision-history.md): 선택적 `ha history`, `decision-history/v1`, 사건 관계와 현재 판단
- [공통 회귀 연결](regressions.md): I1~I5의 제품 경로, 보호 회귀와 미해결 공백
- [Seal 실행자의 역할](role-card.md): 권한과 완료 규칙
- [Spar 입력 형식](spar-input.md): `spar`가 받는 모든 입력 필드의 형식, 형식 밖의 결과와 소비처(G1~G33)
- [Spar 설정](spar-config.md): `spar.toml`의 위치·형식, 리뷰어 preset, 프로젝트 설정의 범위
- [검토 라운드](review-round.md): `review-round/v1`, 패킷, 리뷰어 상태, 지적 분류와 처리, 상한
- [검토 gate](review-gate.md): `review-gate/v1`, 상태와 종료 코드, 검토 수정의 줄 수

설치와 실제 사용은 [사용 안내](../guides/USAGE.md)에서 시작한다.
