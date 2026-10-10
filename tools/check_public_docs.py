#!/usr/bin/env python3
"""Check local links and headings in the public documentation; no network or Git history."""
from __future__ import annotations

import argparse
from collections import Counter
import html
from pathlib import Path
import re
import sys
import unicodedata
from urllib.parse import unquote, urlsplit

PUBLIC_DIRS = ("guides", "contracts", "plugins", "evals", "examples", "assets")
PUBLIC_FILES = ("README.md", "README.en.md", "AGENTS.md", "CLAUDE.md")


def prose(source: str) -> str:
    """Remove fenced examples while retaining line breaks and inline heading text."""
    lines: list[str] = []
    fence = ""
    for line in source.splitlines():
        match = re.match(r"^\s*(`{3,}|~{3,})", line)
        if not fence and match:
            fence = match[1]
            lines.append("")
        elif fence:
            if match and match[1][0] == fence[0] and len(match[1]) >= len(fence):
                fence = ""
            lines.append("")
        else:
            lines.append(line)
    return re.sub(r"<!--.*?-->", "", "\n".join(lines), flags=re.S)


def anchors(source: str) -> set[str]:
    body = prose(source)
    result = set(re.findall(r"\b(?:id|name)=[\"']([^\"']+)[\"']", body))
    seen: Counter[str] = Counter()
    for match in re.finditer(r"(?m)^#{1,6}\s+(.+?)\s*#*\s*$", body):
        title = match[1]
        title = re.sub(r"\[([^\]]+)\]\([^)]*\)", r"\1", title)
        title = re.sub(r"<[^>]*>", "", title)
        title = html.unescape(title).replace("`", "").replace("*", "")
        title = re.sub(r"(?<!\w)_{1,2}(.+?)_{1,2}(?!\w)", r"\1", title)
        slug = "".join(c for c in title.lower()
                       if c in " -_" or unicodedata.category(c)[0] in "LNM").replace(" ", "-")
        suffix = "" if not seen[slug] else f"-{seen[slug]}"
        result.add(slug + suffix)
        seen[slug] += 1
    return result


def links(source: str) -> list[str]:
    body = prose(source)
    # Inline code can contain illustrative link syntax, not rendered links.
    body = re.sub(r"(`+).*?\1", "", body)
    targets: list[str] = []
    patterns = (
        r"!?\[[^\]\n]*\]\(\s*(?:<([^>]+)>|([^\s)]+))(?:\s+\"[^\"]*\")?\s*\)",
        r"\b(?:href|src)\s*=\s*[\"']([^\"']+)[\"']",
        r"(?m)^\s{0,3}\[[^\]]+\]:\s*(?:<([^>]+)>|(\S+))",
    )
    for pattern in patterns:
        for match in re.finditer(pattern, body):
            targets.append(html.unescape(next(x for x in match.groups() if x is not None)))
    return targets


def documents(root: Path) -> list[Path]:
    found = {root / name for name in PUBLIC_FILES if (root / name).is_file()}
    for name in PUBLIC_DIRS:
        for path in (root / name).rglob("*.md"):
            if not any(part in {"node_modules", ".git", "__pycache__"} for part in path.parts):
                found.add(path)
    return sorted(found)


def check(root: Path, paths: list[Path] | None = None) -> tuple[int, list[str]]:
    root = root.resolve()
    files = documents(root) if paths is None else [path.resolve() for path in paths]
    problems: list[str] = []
    anchor_cache: dict[Path, set[str]] = {}
    count = 0
    if not files:
        return 0, ["no public documentation found"]
    for path in files:
        name = str(path.relative_to(root))
        for target in links(path.read_text(encoding="utf-8")):
            url = urlsplit(target)
            if url.scheme or url.netloc:
                continue
            count += 1
            dest = (path.parent / unquote(url.path)).resolve() if url.path else path.resolve()
            if not dest.is_relative_to(root):
                problems.append(f"{name}: link leaves the repository: {target}")
                continue
            if not dest.exists():
                problems.append(f"{name}: missing target: {target}")
                continue
            if url.fragment and dest.is_file():
                if dest.suffix.lower() not in {".md", ".html", ".svg"}:
                    problems.append(f"{name}: unsupported fragment target: {target}")
                    continue
                if dest not in anchor_cache:
                    anchor_cache[dest] = anchors(dest.read_text(encoding="utf-8"))
                if unquote(url.fragment) not in anchor_cache[dest]:
                    problems.append(f"{name}: missing anchor: {target}")
    return count, problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", nargs="?", type=Path, default=Path(__file__).resolve().parent.parent)
    args = parser.parse_args()
    count, problems = check(args.root)
    for problem in problems:
        print(problem, file=sys.stderr)
    print(f"public documents: {len(documents(args.root.resolve()))}; local links: {count}; problems: {len(problems)}")
    return int(bool(problems))


if __name__ == "__main__":
    raise SystemExit(main())
