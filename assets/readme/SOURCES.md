# README 그림 안내

[English](SOURCES.en.md)

README의 그림은 이 저장소의 생성 스크립트로 그린 SVG입니다. 외부 이미지, 실제 사용자 대화, 프로젝트 실행 기록을 사용하지 않습니다.

| 그림 | 설명 | 편집할 파일 |
| --- | --- | --- |
| [사용 흐름](flow.ko.svg) · [English](flow.en.svg) | 요청 → Spec의 목표 정리와 멈춤 → 사용자의 확인과 별도 시작 → Seal의 구현·검사·수정 → 사용자의 결과 확인을 설명하는 개념도입니다 | [generate-flow.py](generate-flow.py) |
| [로그인 예시](example.ko.svg) · [English](example.en.svg) | 기존 로그인 화면에 비밀번호 찾기 링크를 더하는 가상의 변경 전후를 그렸습니다. 실행 결과나 완료·검사 증명이 아닙니다 | [generate-example.py](generate-example.py) |

## 수정과 생성

Python 3만 필요합니다. 저장소 루트에서 실행합니다.

```bash
python3 assets/readme/generate-flow.py
python3 assets/readme/generate-example.py
```

두 스크립트 안의 한국어·영어 문장을 수정하고 다시 실행합니다. SVG에는 그림 설명과 읽을 수 있는 텍스트가 포함됩니다. 브라우저에서 두 언어 그림을 열어 글자가 잘리거나 겹치지 않는지 확인합니다.
