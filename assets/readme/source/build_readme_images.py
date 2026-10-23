#!/usr/bin/env python3
"""Build every SVG behind the README artwork in one run: the usage flow, the
walkthrough frames, the goal folder and the completion report card.

Requires Python 3 and fonttools and a directory with the unmodified
NotoSansCJKkr-Regular.otf and NotoSansCJKkr-Bold.otf font files; see
readme_svg.py. Then render the PNGs and the GIF with
render_readme_images.cjs. These tools are needed only to edit the artwork.
"""

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_goal_folder  # noqa: E402
import build_report_card  # noqa: E402
import build_usage_flow  # noqa: E402
import build_walkthrough  # noqa: E402
from readme_svg import load_fonts  # noqa: E402


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font-dir", type=Path, required=True)
    fonts = load_fonts(parser.parse_args().font_dir)
    for module in (build_usage_flow, build_walkthrough, build_goal_folder, build_report_card):
        module.build(fonts)


if __name__ == "__main__":
    main()
