# README artwork sources

The usage flow, the walkthrough GIF, the goal folder and the completion report
card are drawn as SVG by the scripts in this folder. They are conceptual
illustrations: not screenshots, execution transcripts, or claims that any check
passed. The `password-link` goal, its commits and its record numbers are
fictional. The walkthrough's commands and output lines use the formats `ha` and
`spar` v0.1.7 print, shortened where it shows "…".

## Files

| Image | Generator | Outlined SVG | Editable SVG | README file |
| --- | --- | --- | --- | --- |
| Usage flow | `build_usage_flow.py` | `../usage-flow.{ko,en}.svg` | `svg/usage-flow.{ko,en}.text.svg` | `../usage-flow.{ko,en}.png` |
| Walkthrough | `build_walkthrough.py` | `walkthrough/frame-NN.en.svg` | `svg/walkthrough/frame-NN.en.text.svg` | `../demo.en.gif` |
| Goal folder | `build_goal_folder.py` | `../goal-folder.{ko,en}.svg` | `svg/goal-folder.{ko,en}.text.svg` | `../goal-folder.{ko,en}.png` |
| Report card | `build_report_card.py` | `../report-card.{ko,en}.svg` | `svg/report-card.{ko,en}.text.svg` | `../report-card.{ko,en}.png` |

- `readme_svg.py`: shared drawing code, palette, text measurement and outlining.
- `build_readme_images.py`: runs the four generators.
- `render_readme_images.cjs`: renders the outlined SVGs to PNG and assembles the
  walkthrough GIF from `walkthrough/frames.json` (frame order and delays).
  `render_usage_flow.cjs` renders only the usage flow.
- `renders.json`: for every rendered PNG and the GIF, its sha256, size and the
  sha256 of each SVG (and the frame list) it came from, plus the sharp and
  libvips versions. A rendered file whose sources changed since is stale.
- `*.layout.json`: measured text bounds, for inspecting the layout after copy edits.

The outlined SVGs turn text into vector paths, defining each glyph once, so
they render without installed fonts. Every text run keeps its words in
`aria-label` and `data-text`, and each SVG has an accessible title and
description. The editable SVGs use ordinary text; install the font below
before opening them in a vector editor.

## Font and license

Typeface: **Noto Sans CJK KR**, Regular and Bold, licensed under SIL OFL 1.1.
The font binaries are not included in this repository. A copy of the license is
in [FONT-LICENSE.txt](FONT-LICENSE.txt).

- [Official fonts](https://github.com/notofonts/noto-cjk/tree/main/Sans/OTF/Korean)
- [SIL Open Font License](https://github.com/notofonts/noto-cjk/blob/main/Sans/LICENSE)

## Rebuild

Edit the copy (`COPY` or `FRAMES`) and the layout (`draw()`) in the generator,
then rebuild. Direct edits to generated SVGs, PNGs or the GIF are overwritten.

Install Python's `fonttools` package and Node's `sharp` package where Python and
Node can find them. Download `NotoSansCJKkr-Regular.otf` and
`NotoSansCJKkr-Bold.otf` from the official font directory above, then run from
the repository root:

```bash
python3 assets/readme/source/build_readme_images.py --font-dir /path/to/fonts
node assets/readme/source/render_readme_images.cjs
```

Each generator can also run alone with `--font-dir`. The generators stop on a
missing glyph and on text wider than its line or the canvas. Look at every PNG
and every walkthrough frame at README width in both languages after an edit,
and update the README alt text and prose with the artwork.

These tools are needed only to edit the artwork. They are not required to use
Jaekit or to run the standard repository checks. Rebuilding overwrites the
outlined and editable SVGs, the layout files, the PNGs, the GIF and
`renders.json`. It does not change the hero illustrations.
