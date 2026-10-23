#!/usr/bin/env python3
"""Build Jaekit's bilingual README usage diagrams as outlined and editable SVGs.

Requires Python 3 and fonttools. Supply a directory containing the unmodified
NotoSansCJKkr-Regular.otf and NotoSansCJKkr-Bold.otf font files:
https://github.com/notofonts/noto-cjk/tree/main/Sans/OTF/Korean
License: https://github.com/notofonts/noto-cjk/blob/main/Sans/LICENSE (SIL OFL 1.1).
Fonts are not included in this package. The publishable SVGs have outlined text
and require no installed fonts. Editable text versions live in
assets/readme/source/svg/. Render the PNGs with render_readme_images.cjs.
"""

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from readme_svg import ASSETS, C, SOURCE, Canvas, load_fonts, write_layout, write_pair  # noqa: E402

W, H = 1600, 960
COLUMNS = [48, 424, 800, 1176]
COLUMN_W = 376
CAPTION_W = 340

COPY = {
    "ko": {
        "headline": "한 번의 작업, 이렇게 진행됩니다.",
        "titles": ["요청하고 목표 확인", "“응”으로 Seal 시작", "구현하고 ha로 검사", "검토 뒤 완료 기록"],
        "body": [
            ["Spec이 목표와 완료 조건,", "검토 등급(Assurance)을 쓰고", "지금 구현할지 묻습니다."],
            ["“응”이라고 답하면 시작됩니다.", "나중에 별도 메시지로 목표", "폴더를 지정해도 됩니다."],
            ["에이전트가 계획·구현하고", "ha로 검사합니다. 모든 실행이", "runs.jsonl에 기록됩니다."],
            ["필수 조건이 충족되면 ha done이", "완료를 기록하고, 보고는", "PROGRESS.md에 남습니다."],
        ],
        "request": "요청", "goal": "목표", "criteria": "완료 조건", "tier": "Assurance: standard",
        "ask_now": "지금 구현할까요?", "yes": "응", "or_separate": "또는 별도 메시지",
        "separate_cmd": "/seal:seal docs/specs/<goal>",
        "plan": "계획", "build": "구현", "check": "검사", "records": "실행 기록",
        "report": "완료 보고", "done": "ha done",
        "review_title": "독립 검토는 Assurance 등급에 따라 다릅니다",
        "tiers": [
            ("light", ["이유를 남기고", "검토를 생략할 수 있음"]),
            ("standard", ["완료 전 Spar 검토 라운드,", "gate가 마지막 commit을 덮음"]),
            ("strict", ["standard에 더해", "작업 전 계획 검토"]),
            ("?", ["요구를 바꿔야 하는", "질문은 내게 옵니다"]),
        ],
        "resume": "새 대화에서도, 같은 프로젝트의 같은 목표를 지정해 이어갑니다.",
        "concept": "사용 흐름 · 개념도",
        "description": "1. Spec에 요청하면 목표와 완료 조건, 검토 등급(Assurance)을 정리하고 지금 구현할지 묻습니다. 내가 읽고 확인합니다. "
                       "2. “응”이라고 답하면 Seal이 시작됩니다. 나중에 별도 메시지로 목표 폴더를 지정해 시작해도 됩니다. "
                       "3. 에이전트가 계획·구현하고 ha로 검사하며, 모든 실행이 기록됩니다. "
                       "standard·strict 목표는 완료 전에 Spar의 독립 검토 라운드를 받고 strict는 작업 전 계획 검토도 받습니다. light는 이유를 남기고 검토를 생략할 수 있습니다. "
                       "요구를 바꿔야 하는 질문은 내게 옵니다. "
                       "4. 필수 조건이 충족되고, standard·strict는 기록된 검토 gate가 마지막 commit을 덮으면 ha done이 완료를 기록합니다. 완료 보고는 PROGRESS.md에 남습니다. "
                       "새 대화에서는 같은 프로젝트의 같은 목표를 지정해 이어갑니다. 실제 실행 결과가 아닌 개념도입니다.",
    },
    "en": {
        "headline": "One task, from request to recorded completion.",
        "titles": ["Request & review the goal", "Say yes to start Seal", "Build & check through ha", "Review, then record done"],
        "body": [
            ["Spec drafts the goal, criteria", "and an Assurance tier, then asks", "whether to implement it now."],
            ["Answer “Yes” and Seal starts.", "Naming the goal folder in a", "separate message works too."],
            ["Your agent plans, implements", "and checks through ha, which", "records every run in runs.jsonl."],
            ["When required conditions are", "met, ha done records completion;", "the report lands in PROGRESS.md."],
        ],
        "request": "Request", "goal": "Goal", "criteria": "Criteria", "tier": "Assurance: standard",
        "ask_now": "Implement it now?", "yes": "Yes", "or_separate": "or a separate message",
        "separate_cmd": "/seal:seal docs/specs/<goal>",
        "plan": "Plan", "build": "Build", "check": "Check", "records": "Run record",
        "report": "Report", "done": "ha done",
        "review_title": "Independent review depends on the Assurance tier",
        "tiers": [
            ("light", ["may skip review,", "with a reason"]),
            ("standard", ["Spar review rounds; the gate", "must cover the last commit"]),
            ("strict", ["standard, plus a plan", "review before work"]),
            ("?", ["Questions that change the", "requirements come to you"]),
        ],
        "resume": "To resume, open the same project and name the same goal in a new conversation.",
        "concept": "Usage flow · Concept illustration",
        "description": "1. Ask Spec: it drafts the goal, completion criteria and an Assurance tier, then asks whether to implement it now. Review them. "
                       "2. Answer Yes and Seal starts. Naming the goal folder in a separate message works too. "
                       "3. The agent plans, implements and checks through ha, which records every run. "
                       "standard and strict goals get independent Spar review rounds before completion, and strict goals also get a plan review before work. light goals may skip review with a reason. "
                       "Questions that change the requirements come to you. "
                       "4. When the required conditions are met and, for standard and strict, a recorded review gate covers the last commit, ha done records completion. The report lands in PROGRESS.md. "
                       "To resume in a new conversation, open the same project and name the same goal. This is a concept illustration, not an actual execution result.",
    },
}


def draw(fonts, language, outline):
    c = COPY[language]
    ko = language == "ko"
    d = Canvas(fonts, language, outline, W, H, c["headline"], c["description"])
    d.rect(0, 0, W, H, C["bg"])
    d.text(48, 61, "JAEKIT / HOW TO USE", 19, "bold", C["muted"], max_width=420)
    d.text(1552, 65, "Jaekit", 32, "bold", C["ink"], "end")
    d.text(48, 135, c["headline"], 51 if ko else 49, "bold", max_width=1504)

    # Four stages in equal columns. Each heading names what the user does or sees.
    for index, x in enumerate(COLUMNS):
        accent = C["blue"] if index < 2 else C["teal"]
        d.circle(x + 23, 218, 23, accent)
        d.text(x + 23, 227, f"0{index + 1}", 23, "bold", C["white"], "middle")
        d.text(x + 58, 229, c["titles"][index], 28 if ko else 22, "bold", max_width=COLUMN_W - 70)

    for x in [408, 784, 1160]:
        d.line(x, 272, x, 668, C["line"], 2)
        d.circle(x, 400, 19, C["bg"])
        d.path(f'M{x - 9} 400H{x + 9}M{x + 2} 393L{x + 9} 400L{x + 2} 407', stroke=C["muted"], sw=3)

    # Stage 1: a request becomes a goal document with an Assurance tier.
    d.path('M70 400C78 318 196 286 300 300C392 312 410 410 370 472C326 540 160 536 104 490C80 470 66 440 70 400Z', C["blue_tint"])
    d.path('M70 314H156Q172 314 172 330V366Q172 382 156 382H146L156 398L130 382H70Q54 382 54 366V330Q54 314 70 314Z', C["white"], C["ink"], 3.5)
    d.text(113, 358, c["request"], 26 if ko else 22, "bold", anchor="middle", max_width=108)
    d.path('M178 360C190 368 196 368 210 367M202 359L212 367L203 376', stroke=C["blue"], sw=4.5)
    d.path('M226 282H350L384 316V506Q384 518 372 518H226Q214 518 214 506V294Q214 282 226 282Z', C["white"], C["ink"], 3.5)
    d.path('M350 282V316H384', C["blue_tint"], C["ink"], 3.5)
    d.text(230, 330, "Spec", 31, "bold", C["blue"])
    d.text(231, 366, c["goal"], 22, "bold")
    d.line(231, 384, 360, 384, C["line"], 6)
    d.text(231, 420, c["criteria"], 21, "bold")
    d.rect(231, 436, 17, 17, C["white"], C["blue"], 2.5, 3)
    d.line(260, 445, 360, 445, C["line"], 6)
    d.rect(224, 470, 152, 32, C["blue_tint"], r=8)
    d.text(300, 491, c["tier"], 14, "bold", C["blue_ink"], "middle", max_width=148)

    # Stage 2: Spec asks once; the user's yes starts Seal. A separate message is the other door.
    x = COLUMNS[1]
    d.path(f'M{x + 22} 290H{x + 254}Q{x + 270} 290 {x + 270} 306V340Q{x + 270} 356 {x + 254} 356H{x + 60}L{x + 44} 372V356H{x + 22}Q{x + 6} 356 {x + 6} 340V306Q{x + 6} 290 {x + 22} 290Z',
           C["white"], C["blue"], 3)
    d.text(x + 22, 316, "Spec", 17, "bold", C["blue"])
    d.text(x + 22, 343, c["ask_now"], 22 if ko else 21, "bold", max_width=240)
    d.path(f'M{x + 230} 378H{x + 330}Q{x + 346} 378 {x + 346} 394V420Q{x + 346} 436 {x + 330} 436H{x + 248}L{x + 202} 452L{x + 226} 436H{x + 230}'
           f'Q{x + 214} 436 {x + 214} 420V394Q{x + 214} 378 {x + 230} 378Z', C["blue"], None)
    d.text(x + 280, 417, c["yes"], 26 if ko else 24, "bold", C["white"], "middle", max_width=120)
    d.rect(x + 6, 400, 180, 118, C["ink"], r=14)
    for cx in [x + 26, x + 40, x + 54]:
        d.circle(cx, 419, 4, C["white"])
    d.text(x + 26, 466, "Seal", 31, "bold", C["white"])
    d.line(x + 26, 494, x + 76, 494, '#79D0BA', 6)
    d.line(x + 88, 494, x + 160, 494, C["blue"], 6)
    d.rect(x + 196, 468, 160, 50, C["white"], C["muted"], 2, 10, dash="6 5")
    d.text(x + 276, 489, c["or_separate"], 14 if ko else 13, "bold", C["muted"], "middle", max_width=150)
    d.text(x + 276, 508, "/seal:seal", 14, "bold", C["ink"], "middle", max_width=150)

    # Stage 3: the agent plans, builds and checks; ha records every run.
    x = COLUMNS[2]
    d.path(f'M{x + 10} 400C{x + 18} 318 {x + 136} 288 {x + 240} 300C{x + 336} 312 {x + 360} 400 {x + 330} 466C{x + 296} 538 {x + 130} 538 {x + 64} 498C{x + 30} 476 {x + 6} 446 {x + 10} 400Z', C["teal_tint"])
    for i, (label, y) in enumerate([(c["plan"], 300), (c["build"], 352), (c["check"], 404)]):
        d.rect(x + 22, y, 142, 40, C["white"], C["teal"] if i == 2 else C["ink"], 3, 10)
        d.text(x + 93, y + 27, label, 20 if ko else 19, "bold", C["teal_ink"] if i == 2 else C["ink"], "middle", max_width=130)
    d.path(f'M{x + 93} 340V352M{x + 93} 392V404', stroke=C["ink"], sw=3)
    d.path(f'M{x + 164} 424C{x + 186} 424 {x + 186} 372 {x + 164} 372M{x + 170} 366L{x + 162} 372L{x + 170} 378', stroke=C["teal"], sw=3)
    d.path(f'M{x + 214} 300H{x + 318}L{x + 346} 328V500Q{x + 346} 512 {x + 334} 512H{x + 214}Q{x + 202} 512 {x + 202} 500V312Q{x + 202} 300 {x + 214} 300Z', C["white"], C["ink"], 3.5)
    d.path(f'M{x + 318} 300V328H{x + 346}', C["teal_tint"], C["ink"], 3.5)
    d.text(x + 216, 342, "ha", 25, "bold", C["teal"])
    d.text(x + 216, 368, c["records"], 15 if ko else 14, "bold", C["muted"], max_width=100)
    for y in [394, 424, 454, 484]:
        d.circle(x + 224, y, 5, C["teal"])
        d.line(x + 238, y, x + (330 if y != 484 else 300), y, C["line"], 6)
    d.text(x + 22, 478, "runs.jsonl", 17, "bold", C["muted"], max_width=150)

    # Stage 4: Spar review gate, then ha done and the report. No pass badge.
    x = COLUMNS[3]
    d.path(f'M{x + 12} 404C{x + 20} 320 {x + 132} 290 {x + 236} 302C{x + 340} 316 {x + 362} 404 {x + 330} 470C{x + 292} 540 {x + 130} 540 {x + 66} 500C{x + 30} 478 {x + 8} 448 {x + 12} 404Z', C["teal_tint"])
    d.path(f'M{x + 26} 300H{x + 168}V372C{x + 168} 410 {x + 132} 432 {x + 97} 444C{x + 62} 432 {x + 26} 410 {x + 26} 372Z', C["amber_tint"], C["amber"], 3.5)
    d.text(x + 97, 340, "Spar", 26, "bold", C["amber_ink"], "middle")
    d.text(x + 97, 370, "gate", 19, "bold", C["amber_ink"], "middle")
    d.circle(x + 70, 398, 8, C["amber"])
    d.circle(x + 97, 404, 8, C["amber"])
    d.circle(x + 124, 398, 8, C["amber"])
    d.rect(x + 26, 462, 142, 40, C["ink"], r=10)
    d.text(x + 97, 489, c["done"], 20, "bold", C["white"], "middle")
    d.path(f'M{x + 97} 446V462', stroke=C["ink"], sw=3)
    d.path(f'M{x + 168} 482H{x + 196}M{x + 188} 474L{x + 196} 482L{x + 188} 490', stroke=C["ink"], sw=3)
    d.path(f'M{x + 214} 300H{x + 318}L{x + 346} 328V500Q{x + 346} 512 {x + 334} 512H{x + 214}Q{x + 202} 512 {x + 202} 500V312Q{x + 202} 300 {x + 214} 300Z', C["white"], C["ink"], 3.5)
    d.path(f'M{x + 318} 300V328H{x + 346}', C["teal_tint"], C["ink"], 3.5)
    d.text(x + 216, 344, c["report"], 21 if ko else 20, "bold", max_width=104)
    d.text(x + 216, 368, "PROGRESS.md", 13, "bold", C["muted"], max_width=128)
    for y in [394, 418, 442, 466, 490]:
        d.line(x + 218, y, x + (330 if y not in (442, 490) else 296), y, C["line"], 6)

    # Captions stay readable at README width.
    for i, x in enumerate(COLUMNS):
        for j, line in enumerate(c["body"][i]):
            d.text(x, 566 + 34 * j, line, 23 if ko else 21, fill=C["muted"], max_width=CAPTION_W)

    # Review depends on the tier; questions about the requirements reach the user.
    d.rect(48, 694, 1504, 156, C["white"], C["line"], 2, 16)
    d.text(76, 730, c["review_title"], 22 if ko else 21, "bold", C["amber_ink"], max_width=1000)
    for i, (name, lines) in enumerate(c["tiers"]):
        x = 76 + 370 * i
        question = name == "?"
        fill, ink = (C["blue_tint"], C["blue_ink"]) if question else (C["amber_tint"], C["amber_ink"])
        pill = 40 if question else d.width_of(name, 17, "bold") + 28
        d.rect(x, 748, pill, 30, fill, r=15)
        d.text(x + pill / 2, 769, name, 17, "bold", ink, "middle")
        for j, line in enumerate(lines):
            d.text(x, 806 + 27 * j, line, 19 if ko else 18, fill=C["ink"], max_width=340)

    # The return path is explicit: the user names the same project and goal.
    d.rect(48, 866, 1504, 56, C["panel"], r=14)
    d.path('M76 885V879Q76 875 80 875H94L99 881H120Q124 881 124 885V900Q124 904 120 904H80Q76 904 76 900Z', C["white"], C["muted"], 2.4)
    d.path('M76 887H124L120 904H76Z', C["blue_tint"], C["muted"], 2.4)
    d.text(142, 900, c["resume"], 23 if ko else 22, "bold", max_width=1390)
    d.text(1552, 946, c["concept"], 16, fill=C["muted"], anchor="end")
    return d


def build(fonts):
    languages = {}
    for language in COPY:
        languages[language] = write_pair(draw, fonts, language,
                                         ASSETS / f"usage-flow.{language}.svg",
                                         SOURCE / "svg" / f"usage-flow.{language}.text.svg")
        print(f"Built usage-flow.{language}.svg: {len(languages[language])} text labels; no missing glyphs or line overflows.")
    write_layout(SOURCE / "usage-flow.layout.json", (W, H), languages)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font-dir", type=Path, required=True)
    build(load_fonts(parser.parse_args().font_dir))


if __name__ == "__main__":
    main()
