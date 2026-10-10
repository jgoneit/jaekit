#!/usr/bin/env python3
"""Draw fictional login screens for the README; no app or history is read."""
from html import escape
from pathlib import Path

HERE = Path(__file__).resolve().parent
CONTENT = {
    'ko': {
        'title': '가상 예시 · 로그인 화면',
        'subtitle': '기존 로그인은 그대로, 새 링크를 더합니다',
        'before': '변경 전', 'after': '변경 후 · 목표로 하는 모습',
        'email': '이메일', 'password': '비밀번호', 'login': '로그인',
        'link': '비밀번호 찾기',
        'foot': '사용법 설명을 위한 그림 · 실행 결과가 아닙니다',
        'desc': '가상 로그인 화면 두 개. 변경 전에는 이메일, 비밀번호, 로그인 버튼이 있다. 목표로 하는 변경 후에는 같은 요소 아래 비밀번호 찾기 링크가 더해진다. 실제 앱 화면이나 검사 결과가 아니다.',
    },
    'en': {
        'title': 'Fictional example · Login',
        'subtitle': 'Keep login working. Add a recovery link.',
        'before': 'BEFORE', 'after': 'AFTER · INTENDED RESULT',
        'email': 'Email', 'password': 'Password', 'login': 'Log in',
        'link': 'Forgot password?',
        'foot': 'Workflow illustration · Not an executed result',
        'desc': 'Two fictional login screens. Before: email and password fields and a login button. The intended after state keeps those elements and adds a Forgot password link. These are not app screenshots or test results.',
    },
}

for lang, c in CONTENT.items():
    parts = [f'''<svg xmlns="http://www.w3.org/2000/svg" width="400" height="642" viewBox="0 0 400 642" role="img" aria-labelledby="title desc" xml:lang="{lang}">
  <title id="title">{escape(c['title'])}</title>
  <desc id="desc">{escape(c['desc'])}</desc>
  <style>text {{ font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans KR', 'Apple SD Gothic Neo', sans-serif; }}</style>
  <rect width="400" height="642" rx="18" fill="#f6f8fc"/>
  <text x="24" y="38" font-size="24" font-weight="700" fill="#172b4d">{escape(c['title'])}</text>
  <text x="24" y="66" font-size="16" fill="#52657e">{escape(c['subtitle'])}</text>''']
    for y, after in [(90, False), (342, True)]:
        label = c['after'] if after else c['before']
        parts += [f'  <rect x="24" y="{y}" width="352" height="226" rx="14" fill="#ffffff" stroke="#cad5e2"/>',
                  f'  <text x="44" y="{y+30}" font-size="14" font-weight="700" fill="#3e5e89">{escape(label)}</text>']
        for offset, text in [(46, c['email']), (94, c['password'])]:
            parts += [f'  <rect x="44" y="{y+offset}" width="312" height="38" rx="6" fill="#f6f8fc" stroke="#cad5e2"/>',
                      f'  <text x="57" y="{y+offset+25}" font-size="17" fill="#52657e">{escape(text)}</text>']
        parts += [f'  <rect x="44" y="{y+144}" width="312" height="38" rx="6" fill="#295fa7"/>',
                  f'  <text x="200" y="{y+169}" text-anchor="middle" font-size="17" font-weight="700" fill="#ffffff">{escape(c["login"])}</text>']
        if after:
            parts += [f'  <text x="200" y="{y+210}" text-anchor="middle" font-size="17" text-decoration="underline" fill="#295fa7">{escape(c["link"])}</text>']
    parts += [f'  <text x="200" y="613" text-anchor="middle" font-size="14" fill="#52657e">{escape(c["foot"])}</text>', '</svg>\n']
    (HERE / f'example.{lang}.svg').write_text('\n'.join(parts))
