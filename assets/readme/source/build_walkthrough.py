#!/usr/bin/env python3
"""Build the frames of the README walkthrough GIF as outlined and editable SVGs.

The walkthrough follows one fictional goal, password-link, from the request to
the completion report. Commands and output lines use the formats `ha` and
`spar` v0.1.7 print, shortened where a line shows "…" or a panel title says
"shortened" (then whole lines or sections are left out); the goal, commits
and record numbers are fictional. render_readme_images.cjs turns the outlined
frames into assets/readme/demo.en.gif.

Requires Python 3 and fonttools and the Noto Sans CJK KR font files; see
readme_svg.py.
"""

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from readme_svg import C, SOURCE, Canvas, load_fonts, write_layout, write_pair  # noqa: E402

W, H = 1280, 720
GOAL = "docs/specs/password-link"
REVIEWS = "docs/reviews/password-link"
STEPS = ["Ask Spec", "Review the goal", "Say yes", "Check", "Spar review", "ha done", "Report"]
TITLE = "From request to recorded completion"
FOOTER = "Illustrative sequence · not a recording of a real run"

# Each frame: the step it belongs to, how long it stays (ms), the left panel
# title, its lines and the explanation on the right. Line kinds:
# user, agent (a chat turn), cmd (a command the agent runs), out (its output),
# fail and pass (a result line), file (a file excerpt), report (a summary
# line of ha report), gap (blank space), note (a remark).
FRAMES = [
    {"step": 0, "delay": 3000, "panel": "Agent conversation",
     "lines": [("user", "/spec:spec Add a “Forgot password” link"), ("user+", "to the login screen")],
     "side": ("Ask Spec", ["Describe the change you want", "in the agent conversation."],
              ["Spec writes the goal and", "completion conditions."])},
    {"step": 1, "delay": 4200, "panel": "Agent conversation",
     "lines": [("user", "/spec:spec Add a “Forgot password” link"), ("user+", "to the login screen"),
               ("agent", "Spec: wrote " + GOAL + "/SPEC.md"),
               ("file", "Status: Ready"), ("file", "Assurance: standard"),
               ("file", "AC-1 The login screen shows a “Forgot password?”"),
               ("file", "       link to /reset-password."),
               ("file", "AC-2 The existing login form keeps working.")],
     "side": ("Review the goal", ["Read the goal and its", "completion conditions. Ask", "for changes until it looks right."],
              ["Assurance: standard means", "Spar reviews the change", "before it can complete."])},
    {"step": 2, "delay": 4200, "panel": "Agent conversation",
     "lines": [("agent", "Spec: Implement " + GOAL), ("agent+", "now? Assurance: standard."),
               ("user", "Yes"),
               ("agent", "Seal: starting " + GOAL),
               ("cmd", "$ ha start " + GOAL + " --rules run-rules/5 …"),
               ("out", "started: seq 1, base f46020ce6f77, rules run-rules/5,"),
               ("out", "budget 50 runs / 14400s")],
     "side": ("Say yes", ["Your “Yes” to Spec's question", "starts Seal on this goal."],
              ["A separate message works too:", "/seal:seal " + GOAL])},
    {"step": 3, "delay": 3800, "panel": "Commands the agent runs",
     "lines": [("note", "Before the change, on the starting commit:"),
               ("cmd", "$ ha check " + GOAL + " --baseline AC-1"),
               ("fail", "AC-1 baseline fail_as_expected (seq 2)"),
               ("out", "classification: declared_violation")],
     "side": ("Check before the change", ["AC-1's check fails the way", "its declaration says, before", "anything is changed."],
              ["ha records every run in", "runs.jsonl."])},
    {"step": 3, "delay": 4200, "panel": "Commands the agent runs",
     "lines": [("note", "Before the change, on the starting commit:"),
               ("cmd", "$ ha check " + GOAL + " --baseline AC-1"),
               ("fail", "AC-1 baseline fail_as_expected (seq 2)"),
               ("gap", ""),
               ("note", "After the change is committed:"),
               ("cmd", "$ ha check " + GOAL + " AC-1 AC-2"),
               ("pass", "AC-1 check pass (seq 3)"),
               ("out", "classification: targets_passed"),
               ("pass", "AC-2 check pass (seq 4)")],
     "side": ("…and after it", ["After the change, AC-1 passes", "and AC-2 still passes."],
              ["Only runs ha records count."])},
    {"step": 4, "delay": 4600, "panel": "Commands the agent runs",
     "lines": [("cmd", "$ spar run --stage candidate --base f46020c \\"),
               ("cmd+", "    --criteria " + GOAL + "/SPEC.md \\"),
               ("cmd+", "    --check-path 'checks/**' \\"),
               ("cmd+", "    --out " + REVIEWS + "/01-candidate"),
               ("out", "spar: round written to " + REVIEWS + "/01-candidate"),
               ("gap", ""),
               ("file", "01-candidate/summary.md"),
               ("file", "- claude: completed, 0 findings (verified isolation, required, …)"),
               ("file", "- codex: completed, 0 findings (verified isolation, required, …)")],
     "side": ("Spar review round", ["Independent reviewers read", "the same packet: goal", "documents, checks and diff."],
              ["A finding is fixed, or comes", "to you as a question.", "Spar does not certify or deploy."])},
    {"step": 4, "delay": 4200, "panel": "Commands the agent runs",
     "lines": [("cmd", "$ spar gate " + REVIEWS + "/01-candidate \\"),
               ("cmd+", "    --expect-head e01fdf1 --out " + REVIEWS + "/gate.json"),
               ("file", "gate.json: \"state\": \"complete\""),
               ("gap", ""),
               ("cmd", "$ ha note " + GOAL + " review \\"),
               ("cmd+", "    --gate " + REVIEWS + "/gate.json"),
               ("out", "noted: review (seq 5)")],
     "side": ("Record the gate", ["The gate covers the last", "commit. ha done needs it", "for standard and strict goals."],
              ["Review records stay in a", "folder Git ignores."])},
    {"step": 5, "delay": 3800, "panel": "Commands the agent runs",
     "lines": [("cmd", "$ ha done " + GOAL),
               ("out", "## ha status: " + GOAL),
               ("pass", "- status: **complete** (exit 0)"),
               ("out", "- completion record: seq 6"),
               ("out", "- assurance: local (…)"),
               ("out", "- check author: executor")],
     "side": ("ha done", ["Completion is recorded only", "when the required conditions", "are met and the gate covers", "the last commit."],
              ["Assurance stays local."])},
    {"step": 6, "delay": 5600, "panel": GOAL + "/PROGRESS.md · shortened",
     "lines": [("file", "## 완료 보고"),
               ("report", "Changed: 1 files and 1 commits outside the bundle …"),
               ("report", "Checked: 2 of 2 required conditions satisfied (…)"),
               ("report", "To review: none"),
               ("report", "Remaining limits: assurance local; check author: executor"),
               ("report", "Next step: none; completion record seq 6 holds …"),
               ("gap", ""),
               ("file", "### Evidence per condition"),
               ("file", "| ID | kind | required | satisfied | records |"),
               ("file", "| --- | --- | --- | --- | --- |"),
               ("file", "| AC-1 | change | yes | yes | 2, 3 |"),
               ("file", "| AC-2 | maintain | yes | yes | 4 |")],
     "side": ("Read the report", ["Seal pastes ha report into", "PROGRESS.md, then adds its", "own judgment below it."],
              ["The agent wrote the checks:", "review the result yourself."])},
]

PANEL_X, PANEL_Y, PANEL_W, PANEL_H = 44, 204, 792, 418
SIDE_X, SIDE_W = 860, 376
TEXT_W = PANEL_W - 48


def draw_frame(index):
    frame = FRAMES[index]
    description = (f"Frame {index + 1} of {len(FRAMES)}, {STEPS[frame['step']]}: "
                   + " ".join(text for _, text in frame["lines"] if text)
                   + " — " + frame["side"][0] + ". " + " ".join(frame["side"][1] + frame["side"][2])
                   + " Fictional example; not a recording of a real run.")

    def draw(fonts, language, outline):
        d = Canvas(fonts, language, outline, W, H, f"Jaekit walkthrough, frame {index + 1}: {STEPS[frame['step']]}",
                   description)
        d.rect(0, 0, W, H, C["bg"])
        d.text(44, 56, "Jaekit", 26, "bold")
        d.rect(1004, 28, 232, 42, C["blue_tint"], C["line"], 2, 21)
        d.text(1120, 56, "Fictional walkthrough", 18, "regular", C["muted"], "middle")
        d.text(44, 112, TITLE, 36, "bold", max_width=900)

        # Step bar: done steps and the current one are coloured.
        step_w = (1236 - 44 - 6 * 12) / 7
        for i, label in enumerate(STEPS):
            x = 44 + i * (step_w + 12)
            active = i == frame["step"]
            reached = i <= frame["step"]
            d.rect(x, 140, step_w, 6, C["blue"] if reached else C["line"], r=3)
            d.text(x, 172, label, 16, "bold" if active else "regular",
                   C["blue"] if active else (C["ink"] if reached else C["muted"]), max_width=step_w)

        # Left panel: the conversation, the commands or the file.
        dark = frame["panel"] == "Commands the agent runs"
        d.rect(PANEL_X, PANEL_Y, PANEL_W, PANEL_H, C["term"] if dark else C["white"], None if dark else C["line"], 2, 16)
        d.text(PANEL_X + 24, PANEL_Y + 34, frame["panel"], 16, "bold", C["term_muted"] if dark else C["muted"], max_width=TEXT_W)
        d.line(PANEL_X + 24, PANEL_Y + 50, PANEL_X + PANEL_W - 24, PANEL_Y + 50, '#2C3B52' if dark else C["line"], 2)
        # Lay out the lines first, so a run of user lines or of file lines
        # shares one background block.
        x = PANEL_X + 24
        placed, y = [], PANEL_Y + 84
        for kind, value in frame["lines"]:
            if kind == "gap":
                y += 14
                continue
            if kind in ("user", "agent"):
                speaker = "USER" if kind == "user" else value.partition(": ")[0].upper()
                placed.append(("speaker", speaker, y, kind))
                y += 22
                if kind == "agent":
                    value = value.partition(": ")[2]
            placed.append((kind.rstrip("+"), value, y, kind))
            y += 34 if kind.startswith("user") else (28 if kind in ("note", "file") else 30)
        if y > PANEL_Y + PANEL_H:
            raise ValueError(f"frame {index + 1} panel overflows")

        blocks = []
        for kind, value, ty, _ in placed:
            family = kind if kind in ("user", "file") and not (kind == "file" and dark) else None
            size = 18 if kind == "user" else 16
            if family and blocks and blocks[-1][0] == family and blocks[-1][2] == previous:
                blocks[-1][2] = ty
                blocks[-1][3] = max(blocks[-1][3], d.width_of(value, size))
            elif family:
                blocks.append([family, ty, ty, d.width_of(value, size)])
            previous = ty
        for family, first, last, width in blocks:
            if family == "user":
                d.rect(x - 4, first - 23, width + 24, last - first + 34, C["blue_tint"], r=8)
            else:
                d.rect(x - 8, first - 20, width + 20, last - first + 29, C["panel"], r=6)

        for kind, value, ty, raw in placed:
            if kind == "speaker":
                d.text(x, ty, value, 13, "bold", C["blue"] if raw == "user" else C["teal"])
            elif kind == "user":
                d.text(x + 8, ty, value, 18, "regular", C["blue_ink"], max_width=TEXT_W)
            elif kind == "agent":
                d.text(x, ty, value, 18, "regular", C["ink"], max_width=TEXT_W)
            elif kind == "cmd":
                d.text(x, ty, value, 17, "bold", C["term_text"] if dark else C["ink"], max_width=TEXT_W)
            elif kind == "out":
                d.text(x, ty, value, 17, "regular", C["term_muted"] if dark else C["muted"], max_width=TEXT_W)
            elif kind == "fail":
                d.text(x, ty, value, 17, "regular", C["term_red"] if dark else C["red"], max_width=TEXT_W)
            elif kind == "pass":
                d.text(x, ty, value, 17, "regular", C["term_green"] if dark else C["teal"], max_width=TEXT_W)
            elif kind == "note":
                d.text(x, ty, value, 15, "regular", C["term_amber"] if dark else C["amber_ink"], max_width=TEXT_W)
            elif kind == "file":
                d.text(x, ty, value, 16, "regular", C["term_blue"] if dark else C["muted"], max_width=TEXT_W)
            elif kind == "report":
                label, _, rest = value.partition(": ")
                lw = d.text(x, ty, label + ":", 16, "bold", C["teal_ink"])
                d.text(x + lw + 8, ty, rest, 16, "regular", C["ink"], max_width=TEXT_W - lw - 8)

        # Right panel: what happens and who acts.
        title, lead, more = frame["side"]
        d.rect(SIDE_X, PANEL_Y, SIDE_W, PANEL_H, C["blue_tint"], C["line"], 2, 16)
        d.text(SIDE_X + 24, PANEL_Y + 46, title, 25, "bold", C["blue"], max_width=SIDE_W - 48)
        y = PANEL_Y + 92
        for line in lead:
            d.text(SIDE_X + 24, y, line, 19, "regular", C["ink"], max_width=SIDE_W - 48)
            y += 29
        y += 18
        for line in more:
            d.text(SIDE_X + 24, y, line, 17, "regular", C["muted"], max_width=SIDE_W - 48)
            y += 27
        if y > PANEL_Y + PANEL_H:
            raise ValueError(f"frame {index + 1} side panel overflows")

        d.text(44, 660, FOOTER, 16, "regular", C["muted"])
        d.text(1236, 660, "password-link", 16, "bold", C["muted"], "end")
        d.rect(44, 684, 1192, 8, C["line"], r=4)
        d.rect(44, 684, 1192 * (index + 1) / len(FRAMES), 8, C["blue"], r=4)
        return d

    return draw


def build(fonts):
    frames, layout = [], {}
    folder = SOURCE / "walkthrough"
    for index in range(len(FRAMES)):
        name = f"frame-{index + 1:02d}.en"
        layout[name] = write_pair(draw_frame(index), fonts, "en", folder / f"{name}.svg",
                                  SOURCE / "svg" / "walkthrough" / f"{name}.text.svg")
        frames.append({"svg": f"assets/readme/source/walkthrough/{name}.svg", "delay_ms": FRAMES[index]["delay"]})
    (folder / "frames.json").write_text(json.dumps({"output": "assets/readme/demo.en.gif", "width": W, "height": H,
                                                    "frames": frames}, indent=2) + "\n", encoding="utf-8")
    write_layout(SOURCE / "walkthrough.layout.json", (W, H), layout)
    print(f"Built {len(FRAMES)} walkthrough frames; no missing glyphs or line overflows.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font-dir", type=Path, required=True)
    build(load_fonts(parser.parse_args().font_dir))


if __name__ == "__main__":
    main()
