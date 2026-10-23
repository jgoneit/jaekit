#!/usr/bin/env python3
"""Build the README goal-folder image in Korean and English as outlined and
editable SVGs.

It shows who writes what for one fictional goal, password-link: Spec writes
SPEC.md, Seal writes PLAN.md, REVIEW.md and PROGRESS.md, ha writes runs.jsonl,
Spar keeps its rounds and gate in a folder Git ignores, and raw check output
stays in .git/ha/. Render the PNGs with render_readme_images.cjs.

Requires Python 3 and fonttools and the Noto Sans CJK KR font files; see
readme_svg.py.
"""

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from readme_svg import ASSETS, C, SOURCE, Canvas, load_fonts, write_layout, write_pair  # noqa: E402

W, H = 1600, 740

COPY = {
    "ko": {
        "title": "폴더 구조",
        "tracked": "목표마다 하나",
        "writers": ["Spec이 씀", "Seal이 씀", "ha가 씀"],
        "files": [
            [("SPEC.md", ["목표와 완료 조건"])],
            [("PLAN.md", ["검사 방법·범위·예산"]), ("REVIEW.md", ["검토 계획, Spar 기록 요약"]), ("PROGRESS.md", ["진행 상황과 완료 보고"])],
            [("runs.jsonl", ["덧붙이기만 하는 실행 기록"])],
        ],
        "local": "Git이 추적하지 않음",
        "spar_head": "Spar 검토 기록",
        "spar_rows": [("01-candidate/", "검토 라운드 하나"), ("gate.json", "검토 gate")],
        "spar_note": "Git이 무시하는 폴더에 둡니다",
        "ha_head": "검사 출력 원문",
        "ha_note": "ha log로 봅니다",
        "machine": "내 컴퓨터에만 남음",
        "caption": "이해를 돕기 위한 가상 구성 · password-link",
        "fictional": "가상 예시",
        "description": "가상 목표 폴더 docs/specs/password-link/: Spec이 SPEC.md를, Seal이 PLAN.md·REVIEW.md·PROGRESS.md를, ha가 runs.jsonl을 씁니다. "
                       "REVIEW.md는 검토 계획과 Spar 기록의 요약·위치를 담습니다. "
                       "Spar의 검토 라운드(01-candidate/)와 gate(gate.json)는 Git이 무시하는 docs/reviews/password-link/에, "
                       "검사 출력 원문은 .git/ha/에 남고 Git이 추적하지 않습니다. 실제 기록이 아닌 가상 예시입니다.",
    },
    "en": {
        "title": "Goal folder",
        "tracked": "one folder per goal",
        "writers": ["Written by Spec", "Written by Seal", "Written by ha"],
        "files": [
            [("SPEC.md", ["goal and completion", "conditions"])],
            [("PLAN.md", ["checks, scope, budget"]), ("REVIEW.md", ["review plan;", "Spar summary"]), ("PROGRESS.md", ["progress and the report"])],
            [("runs.jsonl", ["append-only run record"])],
        ],
        "local": "Not tracked by Git",
        "spar_head": "Spar review records",
        "spar_rows": [("01-candidate/", "one review round"), ("gate.json", "the review gate")],
        "spar_note": "kept in a folder Git ignores",
        "ha_head": "Raw check output",
        "ha_note": "read it with ha log",
        "machine": "Stays on your computer",
        "caption": "Fictional layout for illustration · password-link",
        "fictional": "Fictional example",
        "description": "Fictional goal folder docs/specs/password-link/: Spec writes SPEC.md; Seal writes PLAN.md, REVIEW.md and PROGRESS.md; ha writes runs.jsonl. "
                       "REVIEW.md holds the review plan and summarizes and points to the Spar records. "
                       "Spar's review rounds (01-candidate/) and gate (gate.json) stay in docs/reviews/password-link/, a folder Git ignores, "
                       "and raw check output stays in .git/ha/, which Git does not track. A fictional example, not a real record.",
    },
}

ACCENTS = [(C["blue"], C["blue_tint"], "Spec"), (C["teal"], C["teal_tint"], "Seal"), (C["ink"], C["panel"], "ha")]


def document_icon(d, x, y, color):
    d.path(f'M{x} {y}H{x + 20}L{x + 30} {y + 10}V{y + 38}Q{x + 30} {y + 40} {x + 28} {y + 40}H{x + 2}Q{x} {y + 40} {x} {y + 38}Z',
           C["white"], color, 2.6)
    d.path(f'M{x + 8} {y + 20}H{x + 22}M{x + 8} {y + 28}H{x + 22}', stroke=color, sw=2.6)


def folder_icon(d, x, y, fill, stroke):
    d.path(f'M{x} {y + 8}Q{x} {y} {x + 8} {y}H{x + 26}L{x + 34} {y + 8}H{x + 60}Q{x + 68} {y + 8} {x + 68} {y + 16}V{y + 48}'
           f'Q{x + 68} {y + 56} {x + 60} {y + 56}H{x + 8}Q{x} {y + 56} {x} {y + 48}Z', fill, stroke, 2.6)


def draw(fonts, language, outline):
    c = COPY[language]
    ko = language == "ko"
    d = Canvas(fonts, language, outline, W, H, c["title"], c["description"])
    d.rect(0, 0, W, H, C["bg"])
    d.text(48, 78, c["title"], 46, "bold", max_width=700)
    d.text(1552, 74, "Jaekit", 32, "bold", C["ink"], "end")

    # The goal folder: the files the goal documents and the run record live in.
    d.rect(44, 110, 990, 560, C["white"], C["line"], 2, 18)
    d.rect(68, 132, 942, 82, C["blue_tint"], r=14)
    folder_icon(d, 92, 145, C["blue"], C["blue"])
    d.text(184, 186, "docs/specs/password-link/", 34, "bold", C["ink"], max_width=700)
    d.text(986, 186, c["tracked"], 18, "bold", C["blue_ink"], "end", max_width=200)
    d.path('M539 214V248M213 248H865M213 248V270M539 248V270M865 248V270', stroke=C["ink"], sw=2.4)
    for x in [213, 539, 865]:
        d.path(f'M{x - 7} 262L{x} 270L{x + 7} 262', stroke=C["ink"], sw=2.4)

    for i, x in enumerate([68, 394, 720]):
        color, tint, badge = ACCENTS[i]
        d.rect(x, 278, 290, 368, tint, r=16)
        d.text(x + 24, 324, c["writers"][i], 27 if ko else 24, "bold", max_width=250)
        bw = d.width_of(badge, 17, "bold") + 24
        d.rect(x + 24, 340, bw, 30, color, r=7)
        d.text(x + 24 + bw / 2, 361, badge, 17, "bold", C["white"], "middle")
        y = 390
        for name, notes in c["files"][i]:
            document_icon(d, x + 24, y, color)
            d.text(x + 70, y + 29, name, 24, "bold", max_width=200)
            for j, note in enumerate(notes):
                d.text(x + 70, y + 54 + 21 * j, note, 18 if ko else 17, fill=C["muted"], max_width=208)
            y += 80 + 21 * (len(notes) - 1)
        if y > 660:
            raise ValueError(f"{c['writers'][i]} column overflows")

    # Outside Git: Spar's review folder and ha's raw output.
    d.rect(1058, 110, 498, 560, C["white"], C["line"], 2, 18)
    d.text(1082, 154, c["local"], 28 if ko else 27, "bold", max_width=450)
    d.path('M1088 168H1530', stroke=C["line"], sw=2)

    d.rect(1082, 186, 450, 262, C["amber_tint"], r=14)
    d.rect(1102, 204, 70, 30, C["amber"], r=7)
    d.text(1137, 225, "Spar", 17, "bold", C["white"], "middle")
    d.text(1186, 226, c["spar_head"], 20, "bold", C["amber_ink"], max_width=330)
    folder_icon(d, 1102, 252, C["white"], C["amber"])
    d.text(1186, 290, "docs/reviews/password-link/", 22, "bold", C["ink"], max_width=332)
    for j, (name, note) in enumerate(c["spar_rows"]):
        y = 336 + j * 44
        d.path(f'M1124 {y - 26}V{y - 6}H1142', stroke=C["amber"], sw=2.4)
        d.text(1152, y, name, 20, "bold", max_width=170)
        d.text(1326, y, note, 18 if ko else 17, fill=C["muted"], max_width=196)
    d.text(1102, 428, c["spar_note"], 17, "bold", C["amber_ink"], max_width=410)

    d.rect(1082, 466, 450, 128, C["panel"], r=14)
    d.rect(1102, 484, 46, 30, C["ink"], r=7)
    d.text(1125, 505, "ha", 17, "bold", C["white"], "middle")
    d.text(1162, 506, c["ha_head"], 20, "bold", max_width=350)
    d.rect(1102, 528, 64, 48, C["ink"], r=8)
    d.path('M1114 542L1124 552L1114 562M1130 564H1150', stroke=C["white"], sw=3)
    d.text(1182, 556, ".git/ha/", 26, "bold", max_width=150)
    d.text(1182, 582, c["ha_note"], 17, fill=C["muted"], max_width=330)

    d.rect(1092, 612, 40, 28, C["white"], C["muted"], 2.2, 4)
    d.path('M1086 644H1138', stroke=C["muted"], sw=2.4)
    d.text(1150, 636, c["machine"], 20 if ko else 19, "bold", C["muted"], max_width=380)

    d.text(539, 712, c["caption"], 19 if ko else 18, fill=C["muted"], anchor="middle", max_width=900)
    d.text(1552, 712, c["fictional"], 18, "bold", C["muted"], "end")
    return d


def build(fonts):
    languages = {}
    for language in COPY:
        languages[language] = write_pair(draw, fonts, language,
                                         ASSETS / f"goal-folder.{language}.svg",
                                         SOURCE / "svg" / f"goal-folder.{language}.text.svg")
        print(f"Built goal-folder.{language}.svg: {len(languages[language])} text labels; no missing glyphs or line overflows.")
    write_layout(SOURCE / "goal-folder.layout.json", (W, H), languages)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font-dir", type=Path, required=True)
    build(load_fonts(parser.parse_args().font_dir))


if __name__ == "__main__":
    main()
