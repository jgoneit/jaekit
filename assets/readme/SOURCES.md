# README 이미지 안내

[English](SOURCES.en.md)

README에는 제공된 Jaekit 브랜드 이미지와 제품의 역할을 설명하는 개념도를 사용합니다.

## 브랜드 이미지

| 용도 | 미리보기 | 파일 |
| --- | --- | --- |
| 기본 로고 | <img src="brand/logo.svg" width="200" alt="Jaekit 심볼과 워드마크"> | [원본 PNG](brand/logo.png) · [표시용 SVG](brand/logo.svg) |
| 워드마크 | <img src="brand/wordmark.svg" width="160" alt="Jaekit 워드마크"> | [원본 PNG](brand/wordmark.png) · [표시용 SVG](brand/wordmark.svg) |
| 작은 심볼 | <img src="brand/symbol.svg" width="40" alt="Jaekit 심볼"> | [원본 PNG](brand/symbol.png) · [표시용 SVG](brand/symbol.svg) |

제공된 세 PNG 원본은 변경하지 않고 보관합니다. 표시용 SVG는 원본 PNG 바이트를 그대로 포함하고, 검은 로고가 다크 모드에서도 보이도록 둥근 흰색 바탕을 제공합니다. 원본 비율과 투명 여백을 유지합니다. README 상단에는 워드마크가 들어간 대표 이미지를 보여 주고, 로고·워드마크·심볼은 이 안내에서 확인할 수 있습니다.

표시용 SVG는 [build_brand.py](source/build_brand.py)로 다시 만듭니다. Python 3 표준 라이브러리만 필요하며 저장소 루트에서 실행합니다. 이 명령은 PNG를 수정하지 않습니다.

```bash
python3 assets/readme/source/build_brand.py
```

## 개념도

대표 이미지, 사용 흐름, 사용 과정 데모, 폴더 구조, 완료 보고 카드는 Jaekit의 역할과 사용 순서를 설명하는 **개념도**입니다. 실제 화면, 실행 결과나 완료·검사 증명이 아닙니다. 데모와 카드의 `password-link`, 조건, 기록 번호도 가상 값입니다. README의 비밀번호 찾기 요청도 사용법을 위한 가상 예시입니다.

| 이미지 | 한국어 | English | 확대용 SVG |
| --- | --- | --- | --- |
| 대표 이미지 | [hero.ko.png](hero.ko.png) | [hero.en.png](hero.en.png) | — |
| 사용 흐름 | [usage-flow.ko.png](usage-flow.ko.png) | [usage-flow.en.png](usage-flow.en.png) | [ko](usage-flow.ko.svg) · [en](usage-flow.en.svg) |
| 사용 과정 데모 (두 README 공통) | — | [demo.en.gif](demo.en.gif) | [프레임](source/walkthrough/) |
| 폴더 구조 | [goal-folder.ko.png](goal-folder.ko.png) | [goal-folder.en.png](goal-folder.en.png) | [ko](goal-folder.ko.svg) · [en](goal-folder.en.svg) |
| 완료 보고 카드 | [report-card.ko.png](report-card.ko.png) | [report-card.en.png](report-card.en.png) | [ko](report-card.ko.svg) · [en](report-card.en.svg) |

README는 미리보기 호환성을 위해 PNG와 GIF를 사용합니다. 그림의 뜻은 README 대체 텍스트와 본문으로도 읽을 수 있습니다.

## 개념도 출처와 수정

두 대표 이미지는 제공된 README 키트에서 가져왔습니다. 원본 제작 메모에는 OpenAI 이미지 생성으로 한국어판을 만들고 영어판으로 번역했다고 기록되어 있습니다. [이미지 프롬프트](source/IMAGE-PROMPTS.md)에 문구와 구성 지시를 보관합니다. PNG에는 편집 가능한 글자 레이어가 없으므로, 수정 후에는 두 언어의 문구와 가독성을 함께 확인합니다.

사용 흐름, 사용 과정 데모, 폴더 구조, 완료 보고 카드는 SVG로 작성했습니다. 그림마다 생성 스크립트가 있습니다: [사용 흐름](source/build_usage_flow.py), [사용 과정 데모](source/build_walkthrough.py), [폴더 구조](source/build_goal_folder.py), [완료 보고 카드](source/build_report_card.py). 공통 그리기 코드는 [readme_svg.py](source/readme_svg.py)에, 문구와 배치는 각 스크립트에 있습니다. [build_readme_images.py](source/build_readme_images.py)가 네 그림의 SVG를 한 번에 만들고, [render_readme_images.cjs](source/render_readme_images.cjs)가 README가 쓰는 두 언어의 PNG와 데모 GIF를 만듭니다. 데모 GIF는 SVG 프레임을 이어 붙여 만듭니다. 만든 파일이 어느 SVG에서 나왔는지는 [renders.json](source/renders.json)에 sha256으로 남습니다. [원본 안내](source/ARTWORK.md)에 파일 목록과 재생성 방법을 정리했습니다.

완성 SVG는 글자가 윤곽선으로 변환되어 폰트 설치가 필요하지 않습니다. 재생성 도구인 Python 3·fonttools와 Node.js·sharp는 **이미지를 수정할 때만** 필요하며, Jaekit 사용이나 기본 저장소 검사에는 필요하지 않습니다. 사용 폰트는 Noto Sans CJK KR이며 [SIL OFL 1.1 라이선스](source/FONT-LICENSE.txt)를 함께 보관합니다. 폰트 파일은 포함하지 않습니다.

## 수정할 때 유지할 뜻

- Spec은 목표와 완료 조건, `Assurance:` 등급을 정리하고 지금 구현할지 한 번 물은 뒤 멈춥니다. 사용자의 "응"으로 Seal이 시작되며, **별도 메시지로 Seal을 시작**하는 길도 함께 보입니다.
- 계획·구현·검사·수정은 Codex·Claude Code의 에이전트가 수행하고, `ha`가 모든 검사 실행을 기록합니다.
- `standard`·`strict` 목표는 완료 전에 Spar의 독립 검토 라운드를 받고, `strict`는 작업 전 계획 검토도 받습니다. `light`는 이유를 남기고 검토를 건너뛸 수 있습니다. 요구를 바꿔야 하는 질문은 사용자에게 갑니다.
- `ha done`은 필수 완료 조건이 충족될 때 완료를 기록하고, `standard`·`strict`는 기록된 검토 gate가 마지막 commit을 덮어야 합니다. 완료 보고는 `PROGRESS.md`에 남습니다.
- Spar의 라운드와 gate 기록은 Git이 무시하는 폴더(예: `docs/reviews/<goal>/`)에, 검사 출력 원문은 `.git/ha/`에 남습니다.
- 새 대화에서 이어 가려면 같은 프로젝트의 같은 목표를 지정합니다.
- 완료 보고는 `ha report` 출력 모양(다섯 줄 요약과 조건별 근거)이고 보증 수준은 `local`입니다. 실행하지 않은 검사를 통과한 것처럼 그리거나, Spar 검토를 인증된 검증이나 자동 배포처럼 그리지 않습니다.
- 두 언어의 이미지와 대체 텍스트를 함께 갱신합니다.

제품 동작은 [사용 안내](../../guides/USAGE.md)를 따릅니다.
