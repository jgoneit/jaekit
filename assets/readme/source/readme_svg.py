"""Shared drawing code for Jaekit's README artwork.

Every README image except the hero is drawn here as SVG: the usage flow, the
walkthrough frames, the goal folder and the completion report card. Text is
measured with the real font so that a label that would overflow its line, or
a character the font lacks, stops the build instead of producing a clipped
picture.

Two SVGs come out of each drawing:

- an outlined SVG, whose text is turned into vector paths so that it renders
  the same everywhere without installed fonts. Each glyph is defined once and
  reused. Every text run keeps its words in `aria-label` and `data-text`.
- an editable SVG with ordinary <text> elements for vector editors. Install
  the font before opening it.

Requires Python 3 and fonttools. The font is Noto Sans CJK KR (SIL OFL 1.1),
Regular and Bold, from https://github.com/notofonts/noto-cjk/tree/main/Sans/OTF/Korean.
The font files are not part of the repository.
"""

import html
import json
from pathlib import Path

from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

FONT_FILES = {"regular": "NotoSansCJKkr-Regular.otf", "bold": "NotoSansCJKkr-Bold.otf"}
FONT_FAMILY = "Noto Sans CJK KR, Noto Sans KR, sans-serif"

# One palette for every image, taken from the usage flow and the hero.
C = {
    "bg": "#FAFBFD", "ink": "#122033", "muted": "#546276", "faint": "#8A96A8",
    "line": "#D7DFE9", "panel": "#F0F3F8", "white": "#FFFFFF",
    "blue": "#4F6CF7", "blue_tint": "#EDF0FF", "blue_ink": "#2B3FA8",
    "teal": "#168975", "teal_tint": "#E6F5F1", "teal_ink": "#0B5A4C",
    "amber": "#C9761F", "amber_tint": "#FFF3E3", "amber_ink": "#7A4408",
    "red": "#C2413A", "red_tint": "#FDECEA",
    "term": "#122033", "term_text": "#E8EDF5", "term_muted": "#9AA8BD",
    "term_green": "#7FD9C1", "term_amber": "#F2C27B", "term_blue": "#A9B8FF", "term_red": "#F4A39C",
}

ROOT = Path(__file__).resolve().parents[3]
ASSETS = ROOT / "assets" / "readme"
SOURCE = ASSETS / "source"


def load_fonts(font_dir):
    fonts = {}
    for weight, name in FONT_FILES.items():
        path = Path(font_dir) / name
        if not path.is_file():
            raise SystemExit(f"missing font file: {path}")
        fonts[weight] = TTFont(path)
    return fonts


class Canvas:
    """One SVG drawing. Coordinates are pixels of the exported PNG."""

    def __init__(self, fonts, language, outline, width, height, title, description):
        self.fonts = fonts
        self.language = language
        self.outline = outline
        self.width = width
        self.height = height
        self.title = title
        self.description = description
        self.parts = []
        self.measurements = []
        self.glyph_ids = {}
        self.glyph_defs = []

    def add(self, markup):
        self.parts.append(markup)

    def rect(self, x, y, w, h, fill, stroke=None, sw=2, r=0, dash=None):
        extra = f' stroke="{stroke}" stroke-width="{sw}"' if stroke else ''
        if dash:
            extra += f' stroke-dasharray="{dash}"'
        self.add(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"{extra}/>')

    def path(self, d, fill="none", stroke=None, sw=4):
        extra = (f' stroke="{stroke}" stroke-width="{sw}" stroke-linecap="round" stroke-linejoin="round"'
                 if stroke else '')
        self.add(f'<path d="{d}" fill="{fill}"{extra}/>')

    def circle(self, cx, cy, r, fill, stroke=None, sw=4):
        extra = f' stroke="{stroke}" stroke-width="{sw}"' if stroke else ''
        self.add(f'<circle cx="{cx}" cy="{cy}" r="{r}" fill="{fill}"{extra}/>')

    def line(self, x1, y1, x2, y2, color, width=4):
        self.path(f'M{x1} {y1}H{x2}' if y1 == y2 else f'M{x1} {y1}L{x2} {y2}', stroke=color, sw=width)

    def arrow(self, x1, y1, x2, y2, color, width=3):
        """A straight arrow from (x1, y1) to (x2, y2) with an open head."""
        self.line(x1, y1, x2, y2, color, width)
        dx, dy = x2 - x1, y2 - y1
        length = (dx * dx + dy * dy) ** 0.5 or 1
        ux, uy = dx / length, dy / length
        size = 9
        left = (x2 - ux * size - uy * size * 0.7, y2 - uy * size + ux * size * 0.7)
        right = (x2 - ux * size + uy * size * 0.7, y2 - uy * size - ux * size * 0.7)
        self.path(f'M{left[0]:.1f} {left[1]:.1f}L{x2} {y2}L{right[0]:.1f} {right[1]:.1f}', stroke=color, sw=width)

    def width_of(self, value, size, weight="regular"):
        font = self.fonts[weight]
        cmap = font.getBestCmap()
        for char in value:
            if ord(char) not in cmap:
                raise ValueError(f"Missing glyph {char!r} ({ord(char):04x}) in {value!r}")
        advance = sum(font["hmtx"].metrics[cmap[ord(ch)]][0] for ch in value)
        return advance * size / font["head"].unitsPerEm

    def text(self, x, y, value, size=24, weight="regular", fill=None, anchor="start", max_width=None):
        """Draw one line of text. It never wraps: a line wider than max_width
        or the canvas is an error, so copy edits cannot clip silently."""
        fill = fill or C["ink"]
        width = self.width_of(value, size, weight)
        if max_width is not None and width > max_width + 0.01:
            raise ValueError(f"Text overflow ({width:.1f}>{max_width}): {value}")
        actual_x = x - width / 2 if anchor == "middle" else (x - width if anchor == "end" else x)
        if actual_x < 0 or actual_x + width > self.width + 0.01:
            raise ValueError(f"Text exceeds canvas: {value}")
        self.measurements.append({"text": value, "x": round(actual_x, 2), "y": y,
                                  "width": round(width, 2), "font_size": size})
        escaped = html.escape(value, quote=True)
        if not self.outline:
            self.add(f'<text x="{x}" y="{y}" font-family="{FONT_FAMILY}" font-size="{size}" '
                     f'font-weight="{700 if weight == "bold" else 400}" fill="{fill}" '
                     f'text-anchor="{anchor}" xml:space="preserve">{html.escape(value)}</text>')
            return width
        font = self.fonts[weight]
        cmap = font.getBestCmap()
        scale = size / font["head"].unitsPerEm
        self.add(f'<g aria-label="{escaped}" data-text="{escaped}" fill="{fill}" '
                 f'transform="translate({actual_x:.3f} {y}) scale({scale:.6f} {-scale:.6f})">')
        cursor = 0
        for ch in value:
            name = cmap[ord(ch)]
            glyph = self.glyph(weight, name)
            if glyph:
                self.add(f'<use href="#{glyph}" x="{cursor}"/>')
            cursor += font["hmtx"].metrics[name][0]
        self.add('</g>')
        return width

    def glyph(self, weight, name):
        """Define a glyph outline once and return its id, or None if empty."""
        key = (weight, name)
        if key not in self.glyph_ids:
            glyphs = self.fonts[weight].getGlyphSet()
            pen = SVGPathPen(glyphs)
            glyphs[name].draw(pen)
            commands = pen.getCommands()
            if commands:
                ident = f"{weight[0]}{len(self.glyph_defs)}"
                self.glyph_defs.append(f'<path id="{ident}" d="{commands}"/>')
                self.glyph_ids[key] = ident
            else:
                self.glyph_ids[key] = None
        return self.glyph_ids[key]

    def finish(self):
        defs = '<defs>\n' + '\n'.join(self.glyph_defs) + '\n</defs>\n' if self.glyph_defs else ''
        return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.width}" height="{self.height}" '
                f'viewBox="0 0 {self.width} {self.height}" role="img" '
                f'aria-labelledby="title description" xml:lang="{self.language}">\n'
                f'<title id="title">{html.escape(self.title)}</title>\n'
                f'<desc id="description">{html.escape(self.description)}</desc>\n'
                + defs + '\n'.join(self.parts) + '\n</svg>\n')


def write_pair(draw, fonts, language, outlined_path, editable_path):
    """Draw once outlined and once editable; return the outlined measurements."""
    outlined = draw(fonts, language, True)
    outlined_path.parent.mkdir(parents=True, exist_ok=True)
    outlined_path.write_text(outlined.finish(), encoding="utf-8")
    editable = draw(fonts, language, False)
    editable_path.parent.mkdir(parents=True, exist_ok=True)
    editable_path.write_text(editable.finish(), encoding="utf-8")
    return outlined.measurements


def write_layout(path, canvas, languages):
    """Measured text bounds, kept for inspecting the layout after copy edits."""
    report = {"canvas": list(canvas), "font": "Noto Sans CJK KR", "languages": languages}
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
