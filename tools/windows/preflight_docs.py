"""Read the public preflight examples; this does not execute PowerShell."""
from __future__ import annotations

import re


def block(text: str, label: str) -> str:
    start = f"<!-- BEGIN WINDOWS {label} -->"
    end = f"<!-- END WINDOWS {label} -->"
    if text.count(start) != 1 or text.count(end) != 1:
        raise ValueError(f"expected one {label} example")
    if text.index(end) < text.index(start):
        raise ValueError(f"reversed {label} example boundaries")
    body = text.split(start, 1)[1].split(end, 1)[0]
    match = re.fullmatch(r"\s*```powershell\n(.+?)\n```\s*", body, re.DOTALL)
    if not match:
        raise ValueError(f"invalid {label} PowerShell example")
    return match[1]
