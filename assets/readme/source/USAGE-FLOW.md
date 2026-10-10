# Usage flow artwork

The usage diagram is an exact, native SVG illustration. It is not a screenshot,
an execution transcript, or a claim that any checks passed. The three stages are:

1. Ask Spec, then review the goal and completion criteria.
2. In a separate message, specify the goal folder and start Seal. The agent carries
   out implementation, checks, fixes, and progress recording.
3. Read the changes and check records and review the result against the goal.

Resuming requires opening the same project and explicitly specifying the same
goal in a new conversation. The illustration does not promise automatic resume.

## Files

- `assets/readme/usage-flow.ko.svg` and `.en.svg`: outlined text, font independent.
- `assets/readme/usage-flow.ko.png` and `.en.png`: 1600 × 840 PNG exports.
- `assets/readme/source/svg/usage-flow.ko.text.svg` and `.en.text.svg`: editable text versions.
- `assets/readme/source/build_usage_flow.py`: layouts, artwork, and both languages' copy.
- `assets/readme/source/render_usage_flow.cjs`: SVG-to-PNG renderer.
- `assets/readme/source/usage-flow.layout.json`: measured text bounds for layout inspection.

The SVG includes an accessible title and full description. Publishable SVG text
is converted to vector paths so that Korean characters do not depend on the
reader's installed fonts. The text SVGs remain editable in a vector editor;
install the font below before opening them.

## Font and license

Typeface: **Noto Sans CJK KR**, Regular and Bold, licensed under SIL OFL 1.1.
The font binaries are not included in this repository. A copy of the license is
in [FONT-LICENSE.txt](FONT-LICENSE.txt).

- [Official fonts](https://github.com/notofonts/noto-cjk/tree/main/Sans/OTF/Korean)
- [SIL Open Font License](https://github.com/notofonts/noto-cjk/blob/main/Sans/LICENSE)

## Rebuild

For a repeatable update, edit `COPY` for wording and `draw()` for layout in
`build_usage_flow.py`, then regenerate both languages. Direct edits to the
generated SVGs will be overwritten by the generator.

Install Python's `fonttools` package and Node's `sharp` package. Download
`NotoSansCJKkr-Regular.otf` and `NotoSansCJKkr-Bold.otf` from the official font
directory above, then run from the repository root:

```bash
python3 assets/readme/source/build_usage_flow.py --font-dir /path/to/fonts
node assets/readme/source/render_usage_flow.cjs
```

The generator rejects missing glyphs and text that exceeds its allotted line
width. Visual inspection at README width is still recommended after copy edits.
These dependencies are optional artwork tools; they are not required to use
Jaekit or run the standard repository checks. Rebuilding overwrites the published
usage SVGs, editable SVGs, layout measurements, and PNG exports. It does not
change the hero illustrations.
