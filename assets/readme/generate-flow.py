#!/usr/bin/env python3
"""Generate the two editable, static README flow diagrams (stdlib only)."""
from html import escape
from pathlib import Path

HERE = Path(__file__).resolve().parent
CONTENT = {
    'ko': {
        'title': '요청에서 결과 확인까지',
        'subtitle': 'Spec으로 정리하고, Seal로 실행합니다',
        'steps': [
            ('사용자', '원하는 일을 말합니다', ['무엇을 바꾸고 싶은지', '에이전트에게 요청합니다.']),
            ('Spec · 목표 정리', '목표를 쓰고 멈춥니다', ['원하는 결과와 완료 조건을 정리합니다.', '정리된 목표를 사용자에게 돌려줍니다.']),
            ('사용자', '확인하고, 시작을 요청합니다', ['목표를 읽고 필요한 내용을 고친 뒤', '별도로 “구현해줘”라고 요청합니다.']),
            ('Seal · 작업 실행', '구현 → 검사 → 수정', ['필요한 조건을 충족할 때까지 반복합니다.', '작업마다 승인을 받을 필요는 없습니다.']),
            ('Seal → 사용자', '완료 보고를 읽고 확인합니다', ['바뀐 결과와 확인한 내용을 받습니다.', '사용자가 직접 결과를 살펴봅니다.']),
        ],
        'foot_title': '작업 중에도 이어집니다',
        'foot': ['결과를 바꾸는 질문에는 답해 주세요.', '대화가 끊겨도 같은 목표를 이어 갈 수 있어요.'],
        'desc': '사용자의 요청, Spec의 목표 정리 후 멈춤, 사용자의 확인과 별도 시작, Seal의 구현 검사 수정 반복, 완료 보고와 결과 확인의 다섯 단계. 결과를 바꾸는 질문에 답하고, 대화가 끊겨도 같은 목표를 이어 간다.',
    },
    'en': {
        'title': 'From request to a result',
        'subtitle': 'Define the goal with Spec. Run it with Seal.',
        'steps': [
            ('YOU', 'Describe what you need', ['Tell your agent what you want', 'to change or create.']),
            ('SPEC · DEFINE THE GOAL', 'Write the goal, then stop', ['Spec defines the desired result', 'and the conditions for completion.']),
            ('YOU', 'Review, then ask to start', ['Read the goal and ask for changes.', 'Then ask separately to implement it.']),
            ('SEAL · DO THE WORK', 'Implement → check → fix', ['Repeat until required conditions', 'are met. No approval for each task.']),
            ('SEAL → YOU', 'Read the report. Try the result.', ['Get the result and what was checked.', 'Review the result for yourself.']),
        ],
        'foot_title': 'Keep the work moving',
        'foot': ['Answer questions that affect the result.', 'Resume the same goal in a new chat.'],
        'desc': 'Five stages: request, Spec defines the goal and stops, you review and separately ask to start, Seal repeats implementation checks and fixes, then reports completion for you to review. Answer questions affecting the result and resume the same goal after a conversation ends.',
    },
}

for lang, c in CONTENT.items():
    parts = [f'''<svg xmlns="http://www.w3.org/2000/svg" width="400" height="1032" viewBox="0 0 400 1032" role="img" aria-labelledby="title desc" xml:lang="{lang}">
  <title id="title">{escape(c['title'])}</title>
  <desc id="desc">{escape(c['desc'])}</desc>
  <style>text {{ font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans KR', 'Apple SD Gothic Neo', sans-serif; }} .label {{ font-size: 14px; font-weight: 700; letter-spacing: .3px; }} .heading {{ font-size: 20px; font-weight: 700; fill: #172b4d; }} .body {{ font-size: 18px; fill: #31435b; }}</style>
  <defs><marker id="arrow" viewBox="0 0 8 8" refX="6" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M1 1 L6 4 L1 7" fill="none" stroke="#6f849e" stroke-width="1.5"/></marker></defs>
  <rect width="400" height="1032" rx="18" fill="#f6f8fc"/>
  <text x="24" y="43" font-size="27" font-weight="750" fill="#172b4d">{escape(c['title'])}</text>
  <text x="24" y="73" font-size="15" fill="#52657e">{escape(c['subtitle'])}</text>''']
    for i, (label, heading, lines) in enumerate(c['steps']):
        y = 100 + i * 166
        fill = '#ffffff' if i in [0, 2] else '#edf5ff' if i == 1 else '#eaf7f4'
        accent = '#3e5e89' if i in [0, 2] else '#295fa7' if i == 1 else '#17715f'
        parts += [f'  <rect x="24" y="{y}" width="352" height="138" rx="14" fill="{fill}" stroke="#cad5e2"/>',
                  f'  <circle cx="46" cy="{y+25}" r="11" fill="{accent}"/>',
                  f'  <text x="46" y="{y+29}" text-anchor="middle" font-size="12" font-weight="700" fill="#fff">{i+1}</text>',
                  f'  <text class="label" x="65" y="{y+30}" fill="{accent}">{escape(label)}</text>',
                  f'  <text class="heading" x="44" y="{y+62}">{escape(heading)}</text>']
        parts += [f'  <text class="body" x="44" y="{y+91+j*24}">{escape(line)}</text>' for j, line in enumerate(lines)]
        if i < 4:
            parts.append(f'  <path d="M200 {y+145} V{y+160}" stroke="#6f849e" stroke-width="1.8" marker-end="url(#arrow)"/>')
    parts += [f'  <text x="24" y="944" font-size="17" font-weight="700" fill="#172b4d">{escape(c["foot_title"])}</text>']
    parts += [f'  <text x="24" y="{975+j*24}" font-size="16" fill="#31435b">{escape(line)}</text>' for j, line in enumerate(c['foot'])]
    parts.append('</svg>\n')
    (HERE / f'flow.{lang}.svg').write_text('\n'.join(parts))
