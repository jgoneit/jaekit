#!/usr/bin/env python3
"""Build the frames of the README walkthrough GIF as outlined and editable SVGs.

The walkthrough follows one fictional goal, password-link, from the request to
the completion report. Its scenes follow what the person does; the commands
the agent runs share one scene. Commands and output lines use the formats `ha`
and `spar` v0.1.7 print, shortened where a line shows "…" or a panel title says
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
STEPS = ["You ask Spec", "You review the goal", "You start Seal", "The agent works", "You read the report"]
AGENT_STEP = 3
TITLE = "What you do, from request to report"
FOOTER = "Illustrative sequence · not a recording of a real run"

# Each frame is one scene: the step it shows, how long it stays (ms), the
# left panel title, its lines and the explanation on the right. Line kinds:
# user, agent (a chat turn), cmd (a command the agent runs), out (its output),
# fail and pass (a result line), file (a file excerpt), report (a summary
# line of ha report), gap (blank space), note (a remark). A line may carry a
# third item, a short label drawn at the right edge of the panel. A dense
# panel uses tighter line spacing.
FRAMES = [
    {"step": 0, "delay": 2200, "panel": "Agent conversation",
     "lines": [("user", "/spec:spec Add a “Forgot password” link"), ("user+", "to the login screen")],
     "side": ("You ask Spec", ["Describe the change you want", "in the agent conversation."],
              ["Spec writes the goal and", "completion conditions."])},
    {"step": 1, "delay": 2700, "panel": "Agent conversation",
     "lines": [("user", "/spec:spec Add a “Forgot password” link"), ("user+", "to the login screen"),
               ("agent", "Spec: wrote " + GOAL + "/SPEC.md"),
               ("file", "Status: Ready"), ("file", "Assurance: standard"),
               ("file", "AC-1 The login screen shows a “Forgot password?”"),
               ("file", "       link to /reset-password."),
               ("file", "AC-2 The existing login form keeps working.")],
     "side": ("You review the goal", ["Read the goal and its", "completion conditions.", "Ask for changes until it", "looks right."],
              ["Assurance: standard means", "Spar reviews the change", "before it can complete."])},
    {"step": 2, "delay": 2700, "panel": "Agent conversation",
     "lines": [("agent", "Spec: Implement " + GOAL), ("agent+", "now? Assurance: standard."),
               ("user", "Yes"),
               ("agent", "Seal: starting " + GOAL),
               ("gap", ""),
               ("note", "Or start it later, in a separate message:"),
               ("user", "/seal:seal " + GOAL)],
     "side": ("You start Seal", ["Answer “Yes” to Spec's", "question, or call Seal in a", "separate message later."],
              ["Either way, the agent takes", "the goal from here."])},
    {"step": 3, "delay": 3200, "panel": "Commands the agent runs · shortened", "dense": True,
     "lines": [("cmd", "$ ha check " + GOAL + " --baseline AC-1"),
               ("fail", "AC-1 baseline fail_as_expected (seq 2)", "before the change"),
               ("cmd", "$ ha check " + GOAL + " AC-1 AC-2"),
               ("pass", "AC-1 check pass (seq 3)", "after the change"),
               ("pass", "AC-2 check pass (seq 4)", "still passes"),
               ("gap", ""),
               ("cmd", "$ spar run --stage candidate --base f46020c …", "Spar review round"),
               ("cmd", "$ spar gate " + REVIEWS + "/01-candidate …"),
               ("cmd", "$ ha note " + GOAL + " review --gate …"),
               ("out", "noted: review (seq 5)", "gate recorded"),
               ("gap", ""),
               ("cmd", "$ ha done " + GOAL),
               ("pass", "- status: **complete** (exit 0)"),
               ("out", "- completion record: seq 6", "completion recorded")],
     "side": ("The agent works", ["You do not type these commands.", "The agent runs them while", "you wait; ha records each run."],
              ["Spar reviewers read the goal,", "checks and diff.", "Spar does not certify or deploy."])},
    {"step": 4, "delay": 4200, "panel": GOAL + "/PROGRESS.md · shortened",
     "lines": [("file", "## 완료 보고"),
               ("report", "Changed: 1 files and 1 commits outside the bundle documents since base f46020ce6f77"),
               ("report", "Checked: 2 of 2 required conditions satisfied (…)"),
               ("report", "To review: none"),
               ("report", "Remaining limits: assurance local; check author: executor"),
               ("report", "Next step: none; completion record seq 6 holds and nothing is left to do"),
               ("gap", ""),
               ("file", "### Evidence per condition"),
               ("file", "| ID | kind | required | satisfied | records |"),
               ("file", "| --- | --- | --- | --- | --- |"),
               ("file", "| AC-1 | change | yes | yes | 2, 3 |"),
               ("file", "| AC-2 | maintain | yes | yes | 4 |")],
     "side": ("You read the report", ["Seal pastes ha report into", "PROGRESS.md, then adds its", "own judgment below it."],
              ["The agent wrote the checks:", "review the result yourself."])},
]

PANEL_X, PANEL_Y, PANEL_W, PANEL_H = 44, 204, 792, 418
SIDE_X, SIDE_W = 860, 376
TEXT_W = PANEL_W - 48


def draw_frame(index):
    frame = FRAMES[index]
    description = (f"Frame {index + 1} of {len(FRAMES)}, {STEPS[frame['step']]}: "
                   + " ".join(" ".join(line[1:]) for line in frame["lines"] if line[1])
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

        # Step bar: done steps and the current one are coloured; the agent's
        # step is teal, the steps the person takes are blue.
        step_w = (1236 - 44 - (len(STEPS) - 1) * 12) / len(STEPS)
        for i, label in enumerate(STEPS):
            x = 44 + i * (step_w + 12)
            active = i == frame["step"]
            reached = i <= frame["step"]
            accent = C["teal"] if i == AGENT_STEP else C["blue"]
            d.rect(x, 140, step_w, 6, accent if reached else C["line"], r=3)
            d.text(x, 172, label, 16, "bold" if active else "regular",
                   accent if active else (C["ink"] if reached else C["muted"]), max_width=step_w)

        # Left panel: the conversation, the commands or the file.
        dark = frame["panel"].startswith("Commands the agent runs")
        d.rect(PANEL_X, PANEL_Y, PANEL_W, PANEL_H, C["term"] if dark else C["white"], None if dark else C["line"], 2, 16)
        d.text(PANEL_X + 24, PANEL_Y + 34, frame["panel"], 16, "bold", C["term_muted"] if dark else C["muted"], max_width=TEXT_W)
        d.line(PANEL_X + 24, PANEL_Y + 50, PANEL_X + PANEL_W - 24, PANEL_Y + 50, '#2C3B52' if dark else C["line"], 2)
        # Lay out the lines first, so a run of user lines or of file lines
        # shares one background block.
        x = PANEL_X + 24
        dense = frame.get("dense", False)
        placed, labels, y = [], [], PANEL_Y + 84
        for kind, value, *label in frame["lines"]:
            if kind == "gap":
                y += 10 if dense else 14
                continue
            if kind in ("user", "agent"):
                speaker = "USER" if kind == "user" else value.partition(": ")[0].upper()
                placed.append(("speaker", speaker, y, kind))
                y += 22
                if kind == "agent":
                    value = value.partition(": ")[2]
            placed.append((kind.rstrip("+"), value, y, kind))
            labels.append(label[0] if label else None)
            y += 34 if kind.startswith("user") else (28 if kind in ("note", "file") else (26 if dense else 30))
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

        tags = iter(labels)
        for kind, value, ty, raw in placed:
            size = 16 if dense else 17
            tag = next(tags) if kind != "speaker" else None
            if kind == "speaker":
                d.text(x, ty, value, 13, "bold", C["blue"] if raw == "user" else C["teal"])
            elif kind == "user":
                d.text(x + 8, ty, value, 18, "regular", C["blue_ink"], max_width=TEXT_W)
            elif kind == "agent":
                d.text(x, ty, value, 18, "regular", C["ink"], max_width=TEXT_W)
            elif kind == "cmd":
                d.text(x, ty, value, size, "bold", C["term_text"] if dark else C["ink"], max_width=TEXT_W)
            elif kind == "out":
                d.text(x, ty, value, size, "regular", C["term_muted"] if dark else C["muted"], max_width=TEXT_W)
            elif kind == "fail":
                d.text(x, ty, value, size, "regular", C["term_red"] if dark else C["red"], max_width=TEXT_W)
            elif kind == "pass":
                d.text(x, ty, value, size, "regular", C["term_green"] if dark else C["teal"], max_width=TEXT_W)
            elif kind == "note":
                d.text(x, ty, value, 15, "regular", C["term_amber"] if dark else C["amber_ink"], max_width=TEXT_W)
            elif kind == "file":
                d.text(x, ty, value, 16, "regular", C["term_blue"] if dark else C["muted"], max_width=TEXT_W)
            elif kind == "report":
                label, _, rest = value.partition(": ")
                lw = d.text(x, ty, label + ":", 16, "bold", C["teal_ink"])
                d.text(x + lw + 8, ty, rest, 16, "regular", C["ink"], max_width=TEXT_W - lw - 8)
            if tag:
                # A short label at the right edge says what the line shows.
                right = PANEL_X + PANEL_W - 24
                if d.width_of(value, size, "bold" if kind == "cmd" else "regular") > right - d.width_of(tag, 15) - x - 24:
                    raise ValueError(f"frame {index + 1}: {value!r} runs into its label {tag!r}")
                d.text(right, ty, tag, 15, "regular", C["term_amber"] if dark else C["amber_ink"], "end")

        # Right panel: what happens and who acts. On the agent's step it is
        # teal and its first line says the person types none of the commands.
        title, lead, more = frame["side"]
        agent = frame["step"] == AGENT_STEP
        d.rect(SIDE_X, PANEL_Y, SIDE_W, PANEL_H, C["teal_tint"] if agent else C["blue_tint"], C["line"], 2, 16)
        d.text(SIDE_X + 24, PANEL_Y + 46, title, 25, "bold", C["teal"] if agent else C["blue"], max_width=SIDE_W - 48)
        y = PANEL_Y + 92
        for i, line in enumerate(lead):
            d.text(SIDE_X + 24, y, line, 19, "bold" if agent and i == 0 else "regular", C["ink"], max_width=SIDE_W - 48)
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
