#!/usr/bin/env python3
"""Build the README completion report card in Korean and English as outlined
and editable SVGs.

The card shows the shape of `ha report` output for the README's fictional
password-link goal: the five summary lines in the order `ha report` prints
them, then the evidence per condition. Its values are the README's own
example, so the card and the text below it agree. Render the PNGs with
render_readme_images.cjs.

Requires Python 3 and fonttools and the Noto Sans CJK KR font files; see
readme_svg.py.
"""

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from readme_svg import ASSETS, C, SOURCE, Canvas, load_fonts, write_layout, write_pair  # noqa: E402

W, H = 1600, 900

COPY = {
    "ko": {
        "title": "완료 보고",
        "where": "password-link · PROGRESS.md의 ## 완료 보고 · ha report --lang ko 출력",
        "summary": [
            ("바뀐 것:", ["base fe4597cf233f 이후 실행 묶음 문서 밖에서", "파일 1개, commit 1개"]),
            ("확인한 것:", ["필수 조건 2개 중 2개 충족(검사 기록 2개, 사용자 확인 0개);",
                         "change 조건 1개 중 1개에 신선한 baseline"]),
            ("직접 볼 것:", ["없음"]),
            ("남은 한계:", ["보증 수준 local; 검사 작성자: 실행자"]),
            ("다음 행동:", ["없음; 완료 기록 seq 5이 유효하고 남은 할 일이 없음"]),
        ],
        "evidence": "### 조건별 근거",
        "source": "근거 출처: 완료 기록 seq 5",
        "columns": ["ID", "종류", "필수", "충족", "근거 기록"],
        "rows": [["AC-1", "change", "예", "예", "2, 3"], ["AC-2", "maintain", "예", "예", "4"]],
        "notes": [
            ["시작 commit 이후", "바뀐 파일과 commit"],
            ["필수 조건은 검사 기록으로 충족,", "change 조건은 신선한 baseline"],
            ["내가 직접 볼 것"],
            ["보증 수준 local: 에이전트가 내 컴퓨터에서", "검사를 쓰고 실행했습니다"],
            ["다음에 할 일"],
            ["근거 기록은 runs.jsonl의 seq입니다.", "AC-1: seq 2에서 선언대로 실패, seq 3에서 통과"],
        ],
        "footer": "설명용 가상 보고 · password-link",
        "fictional": "가상 예시",
        "description": "password-link의 가상 완료 보고. ha report 출력 모양입니다. 다섯 줄 요약: 바뀐 것, 확인한 것, 직접 볼 것, 남은 한계(보증 수준 local; 검사 작성자: 실행자), 다음 행동. "
                       "그 아래 완료 기록 seq 5의 조건별 근거: AC-1 change는 근거 기록 2, 3, AC-2 maintain은 4. 실제 기록이 아닌 가상 예시입니다.",
    },
    "en": {
        "title": "Completion report",
        "where": "password-link · ## 완료 보고 in PROGRESS.md · output of ha report",
        "summary": [
            ("Changed:", ["1 files and 1 commits outside the bundle documents", "since base fe4597cf233f"]),
            ("Checked:", ["2 of 2 required conditions satisfied (2 by check records,",
                          "0 by user confirmation); 1 of 1 change conditions have a fresh baseline"]),
            ("To review:", ["none"]),
            ("Remaining limits:", ["assurance local; check author: executor"]),
            ("Next step:", ["none; completion record seq 5 holds and nothing is left to do"]),
        ],
        "evidence": "### Evidence per condition",
        "source": "Source: completion record seq 5",
        "columns": ["ID", "kind", "required", "satisfied", "records"],
        "rows": [["AC-1", "change", "yes", "yes", "2, 3"], ["AC-2", "maintain", "yes", "yes", "4"]],
        "notes": [
            ["What changed since", "the starting commit"],
            ["Required conditions met by check", "records; each change condition has", "a fresh baseline"],
            ["What to look at yourself"],
            ["Assurance local: the agent wrote and", "ran the checks on your computer"],
            ["What to do next"],
            ["Records are seq numbers in runs.jsonl.", "AC-1 failed as declared at 2, passed at 3"],
        ],
        "footer": "Illustrative report · password-link",
        "fictional": "Fictional example",
        "description": "Fictional completion report for password-link, in the shape of ha report output. Five summary lines: Changed, Checked, To review, "
                       "Remaining limits (assurance local; check author: executor) and Next step. "
                       "Then the evidence per condition under completion record seq 5: AC-1 change with records 2, 3 and AC-2 maintain with record 4. A fictional example, not a real record.",
    },
}


def draw(fonts, language, outline):
    c = COPY[language]
    ko = language == "ko"
    d = Canvas(fonts, language, outline, W, H, c["title"], c["description"])
    d.rect(0, 0, W, H, C["bg"])
    badge = d.width_of("ha report", 20, "bold") + 32
    d.rect(48, 34, badge, 40, C["teal"], r=9)
    d.text(48 + badge / 2, 61, "ha report", 20, "bold", C["white"], "middle")
    d.text(48, 136, c["title"], 54, "bold", max_width=900)
    fw = d.width_of(c["fictional"], 19, "bold") + 40
    d.rect(1552 - fw, 40, fw, 42, C["white"], C["muted"], 2, 10)
    d.text(1552 - fw / 2, 68, c["fictional"], 19, "bold", C["muted"], "middle")

    # The report itself, as ha report prints it.
    d.rect(44, 164, 1028, 660, C["white"], C["line"], 2, 18)
    d.text(76, 206, c["where"], 17, fill=C["muted"], max_width=964)
    d.path('M76 224H1040', stroke=C["line"], sw=2)
    label_w = max(d.width_of(label, 21, "bold") for label, _ in c["summary"]) + 18
    y = 262
    anchors = []
    for label, lines in c["summary"]:
        d.text(76, y, label, 21, "bold", C["teal_ink"])
        anchors.append(y - 7)
        for j, line in enumerate(lines):
            d.text(76 + label_w, y + 31 * j, line, 20 if ko else 19.5, fill=C["ink"], max_width=1040 - 76 - label_w)
        y += 31 * len(lines) + 20
    if y > 600:
        raise ValueError("summary overflows")

    y += 6
    d.path(f'M76 {y - 22}H1040', stroke=C["line"], sw=2)
    d.text(76, y + 12, c["evidence"], 21, "bold")
    d.text(76, y + 46, c["source"], 18, fill=C["muted"])
    xs = [76, 196, 356, 516, 676]
    top = y + 64
    d.rect(76, top, 964, 42, C["panel"], r=8)
    for x, name in zip(xs, c["columns"]):
        d.text(x + 16, top + 28, name, 18, "bold", C["muted"], max_width=150)
    for r, row in enumerate(c["rows"]):
        ry = top + 42 + r * 46
        d.path(f'M76 {ry + 46}H1040', stroke=C["line"], sw=1.5)
        for x, value in zip(xs, row):
            d.text(x + 16, ry + 31, value, 20, "bold" if x == 76 else "regular", C["ink"], max_width=150)
    anchors.append(top + 42 + 46 - 8)
    if top + 42 + 2 * 46 > 840:
        raise ValueError("evidence overflows")

    # Notes on the right say how to read each line.
    note_y = 186
    for i, lines in enumerate(c["notes"]):
        h = 22 + 29 * len(lines)
        d.rect(1108, note_y, 448, h, C["blue_tint"], C["line"], 2, 12)
        d.circle(1134, note_y + 26, 13, C["blue"])
        d.text(1134, note_y + 32, str(i + 1), 15, "bold", C["white"], "middle")
        for j, line in enumerate(lines):
            d.text(1158, note_y + 33 + 29 * j, line, 19 if ko else 18, "bold" if j == 0 else "regular",
                   C["ink"], max_width=384)
        # Connector from the line it explains.
        ay = anchors[i]
        d.circle(1056, ay, 6, C["blue"])
        d.path(f'M1062 {ay}C1084 {ay} 1086 {note_y + 26} 1108 {note_y + 26}', stroke=C["blue"], sw=2)
        note_y += h + 14
    if note_y > 862:
        raise ValueError("notes overflow")

    d.text(48, 870, c["footer"], 17, fill=C["muted"], max_width=900)
    return d


def build(fonts):
    languages = {}
    for language in COPY:
        languages[language] = write_pair(draw, fonts, language,
                                         ASSETS / f"report-card.{language}.svg",
                                         SOURCE / "svg" / f"report-card.{language}.text.svg")
        print(f"Built report-card.{language}.svg: {len(languages[language])} text labels; no missing glyphs or line overflows.")
    write_layout(SOURCE / "report-card.layout.json", (W, H), languages)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font-dir", type=Path, required=True)
    build(load_fonts(parser.parse_args().font_dir))


if __name__ == "__main__":
    main()
