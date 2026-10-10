#!/usr/bin/env python3
"""Present the supplied Jaekit PNG branding on white rounded SVG surfaces.

Run from the repository root with Python 3. Only standard-library modules are
required. The PNG files are embedded byte for byte, without raster editing.
"""

import base64
import html
from pathlib import Path
import struct


BRAND = Path(__file__).resolve().parent.parent / "brand"
LABELS = {
    "logo": ("Jaekit logo", "Jaekit symbol and wordmark"),
    "wordmark": ("Jaekit wordmark", "Jaekit wordmark"),
    "symbol": ("Jaekit symbol", "Jaekit symbol"),
}


def build(name, title, label):
    png = (BRAND / f"{name}.png").read_bytes()
    if png[:8] != b"\x89PNG\r\n\x1a\n" or png[12:16] != b"IHDR":
        raise ValueError(f"{name}.png is not a PNG with an IHDR header")
    width, height = struct.unpack(">II", png[16:24])
    radius = min(width, height) // 16
    encoded = base64.b64encode(png).decode("ascii")
    description = (
        f"Provided {label}, displayed unmodified on a white rounded surface "
        "so the black artwork stays legible in light and dark themes. "
        "The original aspect ratio and transparent padding are preserved."
    )
    svg = (
        '<svg xmlns="http://www.w3.org/2000/svg" '
        f'width="{width}" height="{height}" viewBox="0 0 {width} {height}" '
        'role="img" aria-labelledby="title description">\n'
        f'<title id="title">{html.escape(title)}</title>\n'
        f'<desc id="description">{html.escape(description)}</desc>\n'
        f'<rect width="{width}" height="{height}" rx="{radius}" fill="#fff"/>\n'
        f'<image width="{width}" height="{height}" '
        f'href="data:image/png;base64,{encoded}"/>\n'
        '</svg>\n'
    )
    (BRAND / f"{name}.svg").write_text(svg, encoding="utf-8")


def main():
    for name, (title, label) in LABELS.items():
        build(name, title, label)


if __name__ == "__main__":
    main()
