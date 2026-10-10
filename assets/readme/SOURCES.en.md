# About the README illustrations

[한국어](SOURCES.md)

The README illustrations are SVGs drawn by the generator scripts in this repository. They use no external images, actual user conversations, or project execution records.

| Illustration | What it shows | Editable source |
| --- | --- | --- |
| [Workflow](flow.en.svg) · [한국어](flow.ko.svg) | A conceptual diagram: request → Spec writes the goal and stops → you review and separately start → Seal implements, checks, and fixes → you review the result | [generate-flow.py](generate-flow.py) |
| [Login example](example.en.svg) · [한국어](example.ko.svg) | A fictional before-and-after change that adds a Forgot password link to an existing login screen. It is not an executed result or proof of completion or verification | [generate-example.py](generate-example.py) |

## Edit and generate

Only Python 3 is needed. Run from the repository root.

```bash
python3 assets/readme/generate-flow.py
python3 assets/readme/generate-example.py
```

Edit the Korean and English text inside the scripts and run them again. Each SVG includes a description and readable text. Open both language versions in a browser to check for clipped or overlapping text.
