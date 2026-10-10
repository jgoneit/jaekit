#!/usr/bin/env python3
"""Build Jaekit's bilingual README usage diagrams as outlined and editable SVGs.

Requires Python 3 and fonttools. Supply a directory containing the unmodified
NotoSansCJKkr-Regular.otf and NotoSansCJKkr-Bold.otf font files:
https://github.com/notofonts/noto-cjk/tree/main/Sans/OTF/Korean
License: https://github.com/notofonts/noto-cjk/blob/main/Sans/LICENSE (SIL OFL 1.1).
Fonts are not included in this package. The publishable SVGs have outlined text
and require no installed fonts. Editable text versions live in
assets/readme/source/svg/.
"""

import argparse
import html
import json
from pathlib import Path

from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

W, H = 1600, 840
C = {
    "bg": "#FAFBFD", "ink": "#122033", "muted": "#546276",
    "line": "#D7DFE9", "blue": "#4F6CF7", "blue_tint": "#EDF0FF",
    "teal": "#168975", "teal_tint": "#E6F5F1", "white": "#FFFFFF",
}

COPY = {
    "ko": {
        "headline": "한 번의 작업, 이렇게 진행됩니다.",
        "titles": ["요청하고 목표 확인", "Seal을 별도로 시작", "결과와 기록 확인"],
        "body": [
            ["Spec이 목표와 완료 조건을 정리하면,", "내가 읽고 확인합니다."],
            ["새 메시지에 목표 폴더를 지정합니다.", "에이전트가 구현·검사·수정하고", "진행 상황을 기록합니다."],
            ["내가 변경 결과와 검사 기록을 읽고,", "목표대로 끝났는지 확인합니다."],
        ],
        "request": "요청", "goal": "목표", "criteria": "완료 조건",
        "message": "새 메시지", "folder": "목표 폴더", "changes": "변경 결과",
        "records": "검사 기록", "resume": "새 대화에서도, 같은 프로젝트의 같은 목표를 지정해 이어갑니다.",
        "concept": "사용 흐름 · 개념도",
        "description": "1. 요청을 Spec으로 정리한 후 목표와 완료 조건을 읽고 확인합니다. "
                       "2. 목표 폴더를 지정해 새 메시지로 Seal을 별도로 시작합니다. 에이전트가 구현, 검사, 수정하며 진행 상황을 기록합니다. "
                       "3. 변경 결과와 검사 기록을 읽고 목표대로 끝났는지 확인합니다. "
                       "새 대화에서는 같은 프로젝트의 같은 목표를 지정해 이어갑니다. 실제 실행 결과가 아닌 개념도입니다.",
    },
    "en": {
        "headline": "One task, from request to review.",
        "titles": ["Request & review the goal", "Start Seal separately", "Review changes & records"],
        "body": [
            ["Spec drafts the goal and criteria.", "Read them and request changes."],
            ["New message: specify the goal folder.", "Your agent builds, checks, fixes,", "and records progress."],
            ["Read the changes and check records.", "Review whether the goal was met."],
        ],
        "request": "Request", "goal": "Goal", "criteria": "Criteria",
        "message": "New message", "folder": "Goal folder", "changes": "Changes",
        "records": "Check records", "resume": "To resume, open the same project and name the same goal in a new conversation.",
        "concept": "Usage flow · Concept illustration",
        "description": "1. Describe your request to Spec, then review the goal and completion criteria. "
                       "2. Start Seal separately by specifying the goal folder in a new message. The agent builds, checks, fixes, and records progress. "
                       "3. Read the changes and check records, and review the result against the goal. "
                       "To resume in a new conversation, open the same project and name the same goal. This is a concept illustration, not an actual execution result.",
    },
}


class Diagram:
    def __init__(self, fonts, language, outline):
        self.fonts = fonts
        self.language = language
        self.outline = outline
        self.parts = []
        self.measurements = []

    def add(self, markup):
        self.parts.append(markup)

    def rect(self, x, y, w, h, fill, stroke=None, sw=4, r=0):
        self.add(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" '
                 f'fill="{fill}"' + (f' stroke="{stroke}" stroke-width="{sw}"' if stroke else '') + '/>')

    def path(self, d, fill="none", stroke=None, sw=4):
        self.add(f'<path d="{d}" fill="{fill}"' +
                 (f' stroke="{stroke}" stroke-width="{sw}" stroke-linecap="round" stroke-linejoin="round"' if stroke else '') + '/>')

    def circle(self, cx, cy, r, fill, stroke=None, sw=4):
        self.add(f'<circle cx="{cx}" cy="{cy}" r="{r}" fill="{fill}"' +
                 (f' stroke="{stroke}" stroke-width="{sw}"' if stroke else '') + '/>')

    def text(self, x, y, value, size=28, weight="regular", fill=None, anchor="start", max_width=None):
        fill = fill or C["ink"]
        font = self.fonts[weight]
        cmap = font.getBestCmap()
        upm = font["head"].unitsPerEm
        for char in value:
            if ord(char) not in cmap:
                raise ValueError(f"Missing glyph {char!r} ({ord(char):04x})")
        advance = sum(font["hmtx"].metrics[cmap[ord(ch)]][0] for ch in value)
        width = advance * size / upm
        if max_width is not None and width > max_width + 0.01:
            raise ValueError(f"Text overflow ({width:.1f}>{max_width}): {value}")
        actual_x = x - width / 2 if anchor == "middle" else (x - width if anchor == "end" else x)
        if actual_x < 0 or actual_x + width > W:
            raise ValueError(f"Text exceeds canvas: {value}")
        self.measurements.append({"text": value, "x": round(actual_x, 2), "y": y,
                                  "width": round(width, 2), "font_size": size})
        escaped = html.escape(value, quote=True)
        if not self.outline:
            self.add(f'<text x="{x}" y="{y}" font-family="Noto Sans CJK KR, Noto Sans KR, sans-serif" '
                     f'font-size="{size}" font-weight="{700 if weight == "bold" else 400}" '
                     f'fill="{fill}" text-anchor="{anchor}">{html.escape(value)}</text>')
            return
        glyphs = font.getGlyphSet()
        scale = size / upm
        self.add(f'<g aria-label="{escaped}" data-text="{escaped}" fill="{fill}" '
                 f'transform="translate({actual_x:.4f} {y}) scale({scale:.6f} {-scale:.6f})">')
        cursor = 0
        for ch in value:
            name = cmap[ord(ch)]
            pen = SVGPathPen(glyphs)
            glyphs[name].draw(pen)
            commands = pen.getCommands()
            if commands:
                self.add(f'<path d="{commands}" transform="translate({cursor} 0)"/>')
            cursor += font["hmtx"].metrics[name][0]
        self.add('</g>')

    def line(self, x1, y1, x2, y2, color, width=4):
        self.path(f'M{x1} {y1}H{x2}' if y1 == y2 else f'M{x1} {y1}L{x2} {y2}', stroke=color, sw=width)

    def finish(self):
        copy = COPY[self.language]
        desc = html.escape(copy["description"])
        return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}" '
                f'role="img" aria-labelledby="title description" xml:lang="{self.language}">\n'
                f'<title id="title">{html.escape(copy["headline"])}</title>\n'
                f'<desc id="description">{desc}</desc>\n' + '\n'.join(self.parts) + '\n</svg>\n')


def draw(fonts, language, outline):
    d = Diagram(fonts, language, outline)
    c = COPY[language]
    d.rect(0, 0, W, H, C["bg"])
    d.text(64, 61, "JAEKIT / HOW TO USE", 19, "bold", C["muted"], max_width=420)
    d.text(1536, 65, "Jaekit", 32, "bold", C["ink"], "end")
    d.text(64, 135, c["headline"], 51 if language == "ko" else 49, "bold", max_width=1472)

    # Three stages occupy equal columns. Each heading describes a user action.
    for index, x in enumerate([64, 572, 1080]):
        accent = C["blue"] if index == 0 else C["teal"]
        d.circle(x + 23, 220, 23, accent)
        d.text(x + 23, 229, f"0{index+1}", 23, "bold", C["white"], "middle")
        d.text(x + 58, 231, c["titles"][index], 31 if language == "ko" else 27, "bold", max_width=400)

    # Subtle dividers separate stages without enclosing the content in cards.
    for x in [546, 1054]:
        d.line(x, 283, x, 658, C["line"], 2)
        d.circle(x, 426, 23, C["bg"])
        d.path(f'M{x-10} 426H{x+10}M{x+3} 418L{x+11} 426L{x+3} 434', stroke=C["muted"], sw=3.2)

    # Stage 1: a request becomes a goal document that the user reviews.
    d.path('M103 407C114 313 242 282 371 306C475 326 503 424 451 489C391 564 195 553 133 499C108 477 99 441 103 407Z', C["blue_tint"])
    d.path('M90 342H184Q202 342 202 360V402Q202 420 184 420H172L184 438L154 420H90Q72 420 72 402V360Q72 342 90 342Z', C["white"], C["ink"], 4)
    d.text(137, 391, c["request"], 29 if language == "ko" else 25, "bold", anchor="middle", max_width=120)
    d.path('M211 402C225 411 230 411 248 410M238 401L250 410L240 420', stroke=C["blue"], sw=5)
    d.path('M279 298H426L464 336V512Q464 526 450 526H279Q265 526 265 512V312Q265 298 279 298Z', C["white"], C["ink"], 4)
    d.path('M426 298V336H464', C["blue_tint"], C["ink"], 4)
    d.text(285, 352, "Spec", 34, "bold", C["blue"])
    d.text(286, 391, c["goal"], 26, "bold")
    d.line(286, 411, 434, 411, C["line"], 7)
    d.text(286, 451, c["criteria"], 24, "bold")
    d.rect(286, 470, 20, 20, C["white"], C["blue"], 3, 4)
    d.line(320, 480, 434, 480, C["line"], 7)
    # Review cursor is deliberately not a pass or completion badge.
    d.path('M466 486L466 529L478 518L490 538L500 532L488 512L505 508Z', C["white"], C["ink"], 4)
    d.line(478, 470, 480, 458, C["blue"], 5)
    d.line(493, 479, 505, 474, C["blue"], 5)

    # Stage 2: a separate user message names the goal. The agent carries out work.
    d.path('M603 411C612 340 734 310 855 323C957 333 1011 407 982 486C953 563 809 558 704 539C632 526 594 476 603 411Z', C["teal_tint"])
    d.path('M615 289H770Q787 289 787 306V347Q787 364 770 364H744L728 382V364H615Q598 364 598 347V306Q598 289 615 289Z', C["white"], C["ink"], 4)
    d.text(693, 336, c["message"], 27 if language == "ko" else 23, "bold", anchor="middle", max_width=176)
    d.rect(733, 354, 245, 165, C["ink"], r=16)
    for x in [755, 771, 787]:
        d.circle(x, 375, 4.5, C["white"])
    d.text(755, 423, "Seal", 33, "bold", C["white"])
    d.path('M854 438L840 452L854 466M898 438L912 452L898 466M882 432L869 472', stroke='#A5E6D5', sw=6)
    d.line(755, 491, 810, 491, '#79D0BA', 7)
    d.line(824, 491, 896, 491, C["blue"], 7)
    d.path('M617 438V421Q617 412 626 412H672L688 430H752Q762 430 762 440V509Q762 520 751 520H628Q617 520 617 509Z', C["white"], C["teal"], 4)
    d.path('M617 443H762L746 520H617Z', C["teal_tint"], C["teal"], 4)
    d.text(686, 486, c["folder"], 25 if language == "ko" else 22, "bold", anchor="middle", max_width=132)
    d.circle(963, 520, 33, C["white"], C["ink"], 3.5)
    d.path('M945 517A19 19 0 0 1 977 507M977 498V509H966M980 523A19 19 0 0 1 948 534M948 543V532H959', stroke=C["teal"], sw=4)

    # Stage 3: changes and check records are read. No fictional pass results.
    d.path('M1109 418C1118 333 1230 297 1352 318C1474 339 1526 410 1490 485C1451 566 1286 568 1190 528C1133 505 1103 468 1109 418Z', C["teal_tint"])
    d.path('M1128 300H1259L1293 334V503Q1293 515 1281 515H1128Q1116 515 1116 503V312Q1116 300 1128 300Z', C["white"], C["ink"], 4)
    d.path('M1259 300V334H1293', C["blue_tint"], C["ink"], 4)
    d.text(1134, 352, c["changes"], 26 if language == "ko" else 27, "bold", max_width=145)
    d.rect(1135, 375, 136, 109, C["bg"], C["line"], 2, 5)
    d.line(1135, 397, 1271, 397, C["line"], 2)
    for x in [1147, 1158, 1169]:
        d.circle(x, 386, 2.5, C["muted"])
    d.rect(1148, 411, 109, 14, C["blue_tint"], r=3)
    d.line(1149, 439, 1255, 439, C["line"], 6)
    d.line(1149, 458, 1222, 458, C["line"], 6)
    d.path('M1317 346H1448L1482 380V526Q1482 538 1470 538H1317Q1305 538 1305 526V358Q1305 346 1317 346Z', C["white"], C["ink"], 4)
    d.path('M1448 346V380H1482', C["teal_tint"], C["ink"], 4)
    d.text(1320, 402, c["records"], 26 if language == "ko" else 21, "bold", max_width=150)
    for y in [432, 465, 498]:
        d.circle(1331, y, 5, C["teal"])
        d.line(1349, y, 1461 if y != 498 else 1436, y, C["line"], 7)
    d.circle(1198, 502, 31, C["white"], C["ink"], 4)
    d.circle(1195, 499, 12, C["white"], C["teal"], 4)
    d.line(1204, 508, 1217, 521, C["teal"], 5)

    # Captions remain useful when the image is viewed at GitHub README width.
    for i, x in enumerate([64, 572, 1080]):
        for j, line in enumerate(c["body"][i]):
            d.text(x, 600 + 37 * j, line, 27 if language == "ko" else 25.5,
                   fill=C["muted"], max_width=456)

    # The return path is explicit: the user chooses the same project and goal.
    d.rect(64, 718, 1472, 70, '#F0F3F8', r=14)
    d.path('M92 748V741Q92 737 96 737H112L118 744H142Q146 744 146 748V766Q146 770 142 770H96Q92 770 92 766Z', C["white"], C["muted"], 2.6)
    d.path('M92 750H146L141 770H92Z', C["blue_tint"], C["muted"], 2.6)
    d.text(166, 763, c["resume"], 26 if language == "ko" else 24, "bold", max_width=1340)
    d.text(1536, 819, c["concept"], 17, fill=C["muted"], anchor="end")
    return d


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font-dir", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=Path(__file__).resolve().parents[3])
    args = parser.parse_args()
    fonts = {weight: TTFont(args.font_dir / f"NotoSansCJKkr-{name}.otf")
             for weight, name in [("regular", "Regular"), ("bold", "Bold")]}
    assets = args.output_root / "assets" / "readme"
    source = assets / "source" / "svg"
    assets.mkdir(parents=True, exist_ok=True)
    source.mkdir(parents=True, exist_ok=True)
    report = {"canvas": [W, H], "font": "Noto Sans CJK KR", "languages": {}}
    for language in COPY:
        outlined = draw(fonts, language, True)
        (assets / f"usage-flow.{language}.svg").write_text(outlined.finish(), encoding="utf-8")
        editable = draw(fonts, language, False)
        (source / f"usage-flow.{language}.text.svg").write_text(editable.finish(), encoding="utf-8")
        report["languages"][language] = outlined.measurements
        print(f"Built usage-flow.{language}.svg: {len(outlined.measurements)} text labels; no missing glyphs or line overflows.")
    (assets / "source" / "usage-flow.layout.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
