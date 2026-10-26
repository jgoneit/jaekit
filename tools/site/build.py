#!/usr/bin/env python3
"""Build the Jaekit site from guides/, contracts/ and the release metadata.

    python3 tools/site/build.py [--root DIR] [--out DIR] [--base PATH] [--cache DIR]

The documents are rendered from the tracked Markdown files of --root; nothing
is copied into this directory. Pinned third-party files (the Markdown parser,
fonts and the mermaid renderer) are downloaded once into --cache and verified
by their recorded digests before use.

Exit 0: built. 1: an input problem, named on stderr; nothing is written.
2: usage. 3: a pinned download could not be fetched or verified.
"""
from __future__ import annotations

import argparse
import base64
from dataclasses import dataclass, field
import hashlib
import html
import io
import json
import os
from pathlib import Path
import posixpath
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unicodedata
from html.parser import HTMLParser
from urllib.parse import quote, unquote, urlsplit
import urllib.request

SITE = Path(__file__).resolve().parent
REPO_URL = "https://github.com/jgoneit/jaekit"
SECTIONS = ("guides", "contracts")
# Guides appear in this order; any other guide follows in file-name order.
GUIDE_ORDER = ("INSTALL", "USAGE", "SPAR", "OPERATIONS", "CI", "WINDOWS", "WSL")
LANGS = ("ko", "en")
REQUIRED_CHECKS = ("Seal Core", "Public documentation", "Release candidate")
INSTALL_LINE = re.compile(r"^brew install \S+$")
VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link",
        "meta", "source", "track", "wbr"}

TEXT = {
    "ko": {
        "guides": "안내", "contracts": "계약", "docs": "문서", "home": "홈",
        "releases": "릴리스", "search": "문서 검색", "on_page": "이 페이지",
        "copy_page": "페이지 복사", "view_source": "원본 보기", "only": "한국어만",
        "other_only": "영어만", "menu": "목차 열기", "theme": "밝은 테마와 어두운 테마 바꾸기",
        "lang": "Switch to English", "built": "만든 커밋", "licenses": "라이선스",
        "docs_title": "문서", "docs_lead": "설치, 사용, 검토, 운영 안내와 형식 계약입니다. 모두 저장소의 Markdown 파일에서 그대로 만들어집니다.",
        "move": "이동", "open": "열기", "placeholder": "무엇을 찾나요?", "empty": "일치하는 항목이 없습니다",
    },
    "en": {
        "guides": "Guides", "contracts": "Contracts", "docs": "Docs", "home": "Home",
        "releases": "Releases", "search": "Search docs", "on_page": "On this page",
        "copy_page": "Copy page", "view_source": "View source", "only": "English only",
        "other_only": "Korean only", "menu": "Open navigation", "theme": "Switch light or dark theme",
        "lang": "한국어로 보기", "built": "Built from", "licenses": "Licenses",
        "docs_title": "Documentation", "docs_lead": "Guides for installing, using, reviewing and operating Jaekit, and the format contracts. Every page is built as is from the repository's Markdown files.",
        "move": "move", "open": "open", "placeholder": "What are you looking for?", "empty": "Nothing matches",
    },
}


class InputProblem(Exception):
    """An input the site is built from cannot be used (exit 1)."""


class DownloadProblem(Exception):
    """A pinned file could not be fetched or verified (exit 3)."""


# ---------------------------------------------------------------- git inputs

def git(root: Path, *args: str) -> str:
    try:
        result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, timeout=120)
    except OSError as error:
        raise InputProblem(f"git cannot start: {error}") from None
    if result.returncode:
        detail = result.stderr.decode("utf-8", "replace").strip()
        raise InputProblem(f"git {' '.join(args)} failed in {root}: {detail}")
    return result.stdout.decode("utf-8", "surrogateescape")


def tracked(root: Path) -> list[str]:
    return [name for name in git(root, "ls-files", "-z").split("\0") if name]


def source_commit(root: Path) -> str:
    commit = git(root, "rev-parse", "HEAD").strip()
    dirty = git(root, "status", "--porcelain", "--untracked-files=no", "--",
                *SECTIONS, ".claude-plugin/marketplace.json", "assets/readme/brand", "tools/site")
    return commit + ("-dirty" if dirty.strip() else "")


def read_text(root: Path, name: str) -> str:
    try:
        return (root / name).read_text(encoding="utf-8")
    except FileNotFoundError:
        raise InputProblem(f"{name}: file is missing") from None
    except (OSError, UnicodeError) as error:
        raise InputProblem(f"{name}: cannot be read: {error}") from None


def release_version(root: Path) -> str:
    name = ".claude-plugin/marketplace.json"
    try:
        data = json.loads(read_text(root, name))
    except json.JSONDecodeError as error:
        raise InputProblem(f"{name}: not valid JSON: {error}") from None
    version = data.get("metadata", {}).get("version") if isinstance(data, dict) and isinstance(data.get("metadata"), dict) else None
    if not isinstance(version, str) or not version.strip():
        raise InputProblem(f"{name}: metadata.version is missing or empty")
    return version.strip()


def install_command(root: Path, name: str) -> str:
    in_fence = False
    for line in read_text(root, name).splitlines():
        if re.match(r"^\s*(```|~~~)", line):
            in_fence = not in_fence
            continue
        if in_fence and INSTALL_LINE.match(line.strip()):
            return line.strip()
    raise InputProblem(f"{name}: no 'brew install' command in a code block")


# ---------------------------------------------------------------- downloads

def cache_path(entry: dict, cache: Path) -> Path:
    url = entry["url"]
    return cache / (hashlib.sha256(url.encode()).hexdigest()[:16] + "-" + posixpath.basename(urlsplit(url).path))


def fetch(entry: dict, cache: Path) -> bytes:
    url = entry["url"]
    target = cache_path(entry, cache)
    if target.is_file():
        data = target.read_bytes()
        if verified(entry, data):
            return data
    try:
        with urllib.request.urlopen(url, timeout=120) as response:
            data = response.read()
    except (OSError, ValueError) as error:
        raise DownloadProblem(f"cannot download {entry['name']} {entry['version']} from {url}: {error}") from None
    if not verified(entry, data):
        raise DownloadProblem(f"{entry['name']} {entry['version']}: downloaded bytes do not match the pinned digest")
    cache.mkdir(parents=True, exist_ok=True)
    partial = target.with_suffix(target.suffix + ".part")
    partial.write_bytes(data)
    partial.replace(target)
    return data


def verified(entry: dict, data: bytes) -> bool:
    if "sha256" in entry:
        return hashlib.sha256(data).hexdigest() == entry["sha256"]
    algorithm, _, expected = entry["integrity"].partition("-")
    if algorithm != "sha512":
        return False
    return base64.b64encode(hashlib.sha512(data).digest()).decode() == expected


def load_markdown(vendor: dict, cache: Path):
    for entry in vendor["python"]:
        fetch(entry, cache)
        sys.path.insert(0, str(cache_path(entry, cache)))
    try:
        from markdown_it import MarkdownIt
    except ImportError as error:
        raise DownloadProblem(f"the pinned Markdown parser cannot be loaded: {error}") from None
    return MarkdownIt("commonmark", {"html": True}).enable(["table", "strikethrough"])


def member(archive: bytes, name: str) -> bytes:
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as tar:
        try:
            handle = tar.extractfile(name)
        except KeyError:
            handle = None
        if handle is None:
            raise DownloadProblem(f"pinned archive has no regular file {name}")
        return handle.read()


def vendor_assets(vendor: dict, cache: Path) -> tuple[dict[str, bytes], str]:
    """Return the third-party files for assets/ and the license notice."""
    files: dict[str, bytes] = {}
    css: list[str] = []
    notice = ["Third-party files published with this site.", ""]
    for entry in vendor["fonts"]:
        archive = fetch(entry, cache)
        text = member(archive, entry["css"]).decode("utf-8")
        base = posixpath.dirname(entry["css"])

        def swap(match: re.Match) -> str:
            name = posixpath.basename(match.group(1))
            files["fonts/" + name] = member(archive, posixpath.normpath(posixpath.join(base, match.group(1))))
            return f"url(fonts/{name})"

        css.append(f"/* {entry['name']} {entry['version']} ({entry['package']}) */\n" +
                   re.sub(r"url\((\./files/[^)]+\.woff2)\)", swap, text).strip() + "\n")
        notice += notice_entry(entry, member(archive, entry["license"]))
    for entry in vendor["scripts"]:
        archive = fetch(entry, cache)
        files[posixpath.basename(entry["file"])] = member(archive, entry["file"])
        notice += notice_entry(entry, member(archive, entry["license"]))
    files["fonts.css"] = "\n".join(css).encode("utf-8")
    return files, "\n".join(notice)


def notice_entry(entry: dict, license_text: bytes) -> list[str]:
    return ["=" * 72, f"{entry['name']} {entry['version']} — npm package {entry['package']}",
            entry["url"], "=" * 72, "", license_text.decode("utf-8").strip(), ""]


# ---------------------------------------------------------------- documents

@dataclass
class Doc:
    path: str
    section: str
    stem: str
    lang: str
    source: str
    tokens: list = field(default_factory=list)
    title: str = ""
    ids: set = field(default_factory=set)
    headings: list = field(default_factory=list)
    body: str = ""
    summary: str = ""
    mermaid: bool = False

    @property
    def slug(self) -> str:
        return "" if self.stem == "README" else self.stem.lower()

    @property
    def url(self) -> str:
        return f"{self.lang}/{self.section}/" + (f"{self.slug}/" if self.slug else "")

    @property
    def key(self) -> tuple[str, str]:
        return self.section, self.stem


def discover(root: Path, names: list[str]) -> list[Doc]:
    docs = []
    for name in names:
        section, _, file = name.partition("/")
        if section not in SECTIONS or "/" in file or not file.endswith(".md"):
            continue
        lang, stem = ("en", file[:-6]) if file.endswith(".en.md") else ("ko", file[:-3])
        if not stem:
            raise InputProblem(f"{name}: no document name before the extension")
        docs.append(Doc(name, section, stem, lang, read_text(root, name)))
    seen: dict[str, str] = {}
    for doc in docs:
        if doc.url in seen:
            raise InputProblem(f"{doc.path} and {seen[doc.url]} would both be {doc.url}")
        seen[doc.url] = doc.path
    if not docs:
        raise InputProblem("no tracked Markdown files under guides/ or contracts/")
    return docs


def slug_text(title: str) -> str:
    """The heading anchor GitHub uses, computed as tools/check_public_docs.py does."""
    title = re.sub(r"\[([^\]]+)\]\([^)]*\)", r"\1", title)
    title = re.sub(r"<[^>]*>", "", title)
    title = html.unescape(title).replace("`", "").replace("*", "")
    title = re.sub(r"(?<!\w)_{1,2}(.+?)_{1,2}(?!\w)", r"\1", title)
    return "".join(c for c in title.lower()
                   if c in " -_" or unicodedata.category(c)[0] in "LNM").replace(" ", "-")


def plain(inline) -> str:
    parts = []
    for child in inline.children or []:
        if child.type in ("text", "code_inline"):
            parts.append(child.content)
        elif child.type in ("softbreak", "hardbreak"):
            parts.append(" ")
    return "".join(parts).strip()


HTML_ID = re.compile(r"\b(?:id|name)=[\"']([^\"']+)[\"']")
HTML_LINK = re.compile(r"\b(href|src)(\s*=\s*)([\"'])([^\"']+)\3")


def prepare(doc: Doc, md) -> None:
    """Parse once, give every heading its GitHub anchor and collect titles."""
    doc.tokens = md.parse(doc.source)
    counts: dict[str, int] = {}
    tokens = doc.tokens
    for index, token in enumerate(tokens):
        if token.type in ("html_block", "html_inline"):
            doc.ids.update(HTML_ID.findall(token.content))
        if token.type == "inline":
            for child in token.children or []:
                if child.type == "html_inline":
                    doc.ids.update(HTML_ID.findall(child.content))
        if token.type != "heading_open":
            continue
        inline = tokens[index + 1]
        slug = slug_text(inline.content)
        anchor = slug + (f"-{counts[slug]}" if counts.get(slug) else "")
        counts[slug] = counts.get(slug, 0) + 1
        token.attrSet("id", anchor)
        doc.ids.add(anchor)
        text = plain(inline)
        level = int(token.tag[1])
        doc.headings.append((level, anchor, text))
        if level == 1 and not doc.title:
            doc.title = text
    if not doc.title:
        raise InputProblem(f"{doc.path}: no '#' title")


class Links:
    def __init__(self, docs: list[Doc], names: list[str]):
        self.pages = {doc.path: doc for doc in docs}
        self.files = set(names)
        self.dirs = {posixpath.dirname(name) for name in names}
        for name in list(self.dirs):
            while name:
                name = posixpath.dirname(name)
                self.dirs.add(name)
        self.problems: list[str] = []

    def rewrite(self, doc: Doc, href: str) -> str:
        parts = urlsplit(href)
        if parts.scheme or parts.netloc:
            return href
        fragment = unquote(parts.fragment)
        if not parts.path:
            if fragment and fragment not in doc.ids:
                self.problems.append(f"{doc.path}: link to a missing heading: {href}")
            return href
        target = posixpath.normpath(posixpath.join(posixpath.dirname(doc.path), unquote(parts.path)))
        if target == ".." or target.startswith("../"):
            self.problems.append(f"{doc.path}: link leaves the repository: {href}")
            return href
        suffix = f"#{parts.fragment}" if parts.fragment else ""
        if target in self.pages:
            page = self.pages[target]
            if fragment and fragment not in page.ids:
                self.problems.append(f"{doc.path}: link to a missing heading: {href}")
            return relative(doc.url, page.url) + suffix
        if target in self.files:
            return f"{REPO_URL}/blob/main/{quote(target)}{suffix}"
        if target in self.dirs and target != ".":
            return f"{REPO_URL}/tree/main/{quote(target)}{suffix}"
        self.problems.append(f"{doc.path}: link target is missing: {href}")
        return href

    def rewrite_html(self, doc: Doc, text: str) -> str:
        return HTML_LINK.sub(lambda m: m.group(1) + m.group(2) + m.group(3) +
                             html.escape(self.rewrite(doc, html.unescape(m.group(4))), quote=True) + m.group(3), text)


def relative(start: str, target: str) -> str:
    if start == target:
        return "./"
    rel = posixpath.relpath(target or ".", start or ".")
    return rel if rel.endswith("/") else rel + "/"


def up(url: str) -> str:
    return "../" * url.count("/")


def render(doc: Doc, md, links: Links) -> None:
    for token in doc.tokens:
        if token.type == "inline":
            for child in token.children or []:
                if child.type == "link_open" and child.attrGet("href") is not None:
                    child.attrSet("href", links.rewrite(doc, child.attrGet("href")))
                elif child.type == "image" and child.attrGet("src") is not None:
                    src = links.rewrite(doc, child.attrGet("src"))
                    if src.startswith(f"{REPO_URL}/blob/"):
                        src += "?raw=true"
                    child.attrSet("src", src)
                elif child.type == "html_inline":
                    child.content = links.rewrite_html(doc, child.content)
        elif token.type == "html_block":
            token.content = links.rewrite_html(doc, token.content)
        elif token.type == "fence" and token.info.strip().split()[:1] == ["mermaid"]:
            doc.mermaid = True
    doc.body = md.renderer.render(doc.tokens, md.options, {"doc": doc})
    doc.summary = summary_of(doc)


def summary_of(doc: Doc) -> str:
    """The first paragraph after the title, without the language switch line."""
    tokens = doc.tokens
    for index, token in enumerate(tokens):
        if token.type != "paragraph_open" or token.level != 0:
            continue
        text = plain(tokens[index + 1])
        if "한국어" in text and "English" in text and len(text) < 40:
            continue
        return text if len(text) <= 180 else text[:177].rstrip() + "…"
    return ""


def install_rules(md) -> None:
    def fence(renderer, tokens, index, options, env):
        token = tokens[index]
        info = token.info.strip().split()
        lang = info[0] if info else ""
        code = html.escape(token.content)
        if lang == "mermaid":
            return f'<pre class="mermaid" data-source="{html.escape(token.content, quote=True)}">{code}</pre>\n'
        label = f'<span class="code-lang">{html.escape(lang)}</span>' if lang else "<span></span>"
        attr = f' class="language-{html.escape(lang, quote=True)}"' if lang else ""
        return (f'<div class="codeblock"><div class="codeblock-head">{label}'
                f'<button type="button" class="copy" data-copy-code>copy</button></div>'
                f'<pre><code{attr}>{code}</code></pre></div>\n')

    def table_open(renderer, tokens, index, options, env):
        return '<div class="table-scroll"><table>\n'

    def table_close(renderer, tokens, index, options, env):
        return "</table></div>\n"

    def heading_close(renderer, tokens, index, options, env):
        opening = tokens[index - 2]
        anchor = opening.attrGet("id")
        mark = f'<a class="anchor" href="#{anchor}" aria-hidden="true" tabindex="-1">#</a>' if anchor and opening.tag != "h1" else ""
        return f"{mark}</{tokens[index].tag}>\n"

    md.add_render_rule("fence", fence)
    md.add_render_rule("code_block", lambda renderer, tokens, index, options, env:
                       f'<div class="codeblock"><div class="codeblock-head"><span></span><button type="button" class="copy" data-copy-code>copy</button></div><pre><code>{html.escape(tokens[index].content)}</code></pre></div>\n')
    md.add_render_rule("table_open", table_open)
    md.add_render_rule("table_close", table_close)
    md.add_render_rule("heading_close", heading_close)


# ---------------------------------------------------------------- page shell

def esc(text: str) -> str:
    return html.escape(text, quote=True)


ICON_SEARCH = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><circle cx="7" cy="7" r="4.6"/><path d="M10.5 10.5 14 14"/></svg>'
ICON_GITHUB = '<svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 .2a8 8 0 0 0-2.5 15.6c.4 0 .5-.2.5-.4v-1.5c-2.2.5-2.7-1-2.7-1-.4-.9-.9-1.2-.9-1.2-.7-.5.1-.5.1-.5.8.1 1.2.8 1.2.8.7 1.3 1.9.9 2.4.7 0-.5.3-.9.5-1.1-1.8-.2-3.6-.9-3.6-4 0-.9.3-1.6.8-2.1-.1-.2-.4-1 .1-2.1 0 0 .7-.2 2.2.8a7.5 7.5 0 0 1 4 0c1.5-1 2.2-.8 2.2-.8.4 1.1.2 1.9.1 2.1.5.6.8 1.3.8 2.1 0 3.1-1.9 3.8-3.6 4 .3.3.6.8.6 1.5v2.2c0 .2.1.5.6.4A8 8 0 0 0 8 .2Z"/></svg>'
ICON_THEME = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="8" cy="8" r="5.5"/><path d="M8 2.5v11"/><path d="M8 2.5a5.5 5.5 0 0 1 0 11z" fill="currentColor"/></svg>'
ICON_MENU = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M2 4h12M2 8h12M2 12h12"/></svg>'
THEME_BOOT = ('<script>try{var t=localStorage.getItem("jaekit-theme");'
              'if(t==="light"||t==="dark")document.documentElement.setAttribute("data-theme",t)}catch(e){}</script>')


@dataclass
class Build:
    commit: str
    version: str
    install: dict
    docs: list
    by_path: dict
    groups: dict


def head(lang: str, title: str, description: str, root: str, build: Build, extra: str = "") -> str:
    return f"""<!doctype html>
<html lang="{lang}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{esc(title)}</title>
<meta name="description" content="{esc(description)}">
<meta name="jaekit-commit" content="{esc(build.commit)}">
<meta name="jaekit-version" content="{esc(build.version)}">
<meta name="theme-color" content="#0A0A0B">
<link rel="icon" href="{root}assets/symbol.svg" type="image/svg+xml">
<link rel="stylesheet" href="{root}assets/fonts.css">
<link rel="stylesheet" href="{root}assets/site.css">
{THEME_BOOT}{extra}
</head>
"""


def nav(lang: str, root: str, switch: str, page: str, menu: bool) -> str:
    t = TEXT[lang]
    other = "en" if lang == "ko" else "ko"
    on = lambda name: ' class="on"' if page == name else ""
    labels = '<b>KO</b><span>/</span><span>EN</span>' if lang == "ko" else '<span>KO</span><span>/</span><b>EN</b>'
    menu_button = (f'<button type="button" class="ghost-btn menu-btn" id="menu-btn" aria-label="{esc(t["menu"])}" '
                   f'aria-expanded="false" aria-controls="side">{ICON_MENU}</button>') if menu else ""
    home_links = (f'<a href="{root}{lang}/#how">{"작동 방식" if lang == "ko" else "How it works"}</a>'
                  f'<a href="{root}{lang}/#try">{"직접 해보기" if lang == "ko" else "Try it"}</a>')
    return f"""<header class="nav">
  <div class="wrap">
    {menu_button}<a class="brand" href="{root}{lang}/" aria-label="Jaekit {esc(t['home'])}"><i></i><b>Jaekit</b></a>
    <nav class="nav-links" aria-label="Main">{home_links}<a href="{root}{lang}/docs/"{on('docs')}>{t['docs']}</a><a href="{REPO_URL}/releases">{t['releases']}</a></nav>
    <div class="nav-tools">
      <button type="button" class="search" id="search-btn" aria-haspopup="dialog">{ICON_SEARCH}<span>{t['search']}</span><kbd>⌘K</kbd></button>
      <a class="ghost-btn gh" href="{REPO_URL}" aria-label="GitHub">{ICON_GITHUB}</a>
      <a class="ghost-btn lang" href="{switch}" hreflang="{other}" data-set-lang="{other}" aria-label="{esc(t['lang'])}">{labels}</a>
      <button type="button" class="ghost-btn" id="theme-btn" aria-label="{esc(t['theme'])}">{ICON_THEME}</button>
    </div>
  </div>
</header>
"""


def palette(lang: str, root: str) -> str:
    t = TEXT[lang]
    return f"""<div class="veil" id="palette" hidden data-index="{root}{lang}/search.json" data-root="{root}" data-empty="{esc(t['empty'])}">
  <div class="palette" role="dialog" aria-modal="true" aria-label="{esc(t['search'])}">
    <div class="palette-in">{ICON_SEARCH}<input id="palette-input" type="text" spellcheck="false" autocomplete="off" placeholder="{esc(t['placeholder'])}" aria-controls="palette-list"><kbd>esc</kbd></div>
    <ul id="palette-list" role="listbox"></ul>
    <div class="palette-foot"><span>↑↓ {t['move']}</span><span>enter {t['open']}</span><span>⌘K · /</span></div>
  </div>
</div>
"""


def footer(lang: str, root: str, build: Build) -> str:
    t = TEXT[lang]
    short = build.commit[:12]
    return f"""<footer class="foot">
  <div class="wrap">
    <a class="brand" href="{root}{lang}/" aria-label="Jaekit {esc(t['home'])}"><i></i><b>Jaekit</b></a>
    <nav aria-label="Project">
      <a href="{root}{lang}/docs/">docs</a>
      <a href="{REPO_URL}">github</a>
      <a href="{REPO_URL}/releases">releases</a>
      <a href="{REPO_URL}/issues/new/choose">issues</a>
      <a href="{REPO_URL}/blob/main/LICENSE">MIT</a>
      <a href="{root}assets/licenses.txt">{t['licenses']}</a>
    </nav>
    <p class="built">{t['built']} <code title="{esc(build.commit)}">{esc(short)}</code> · v{esc(build.version)}</p>
  </div>
</footer>
"""


def scripts(root: str, mermaid: bool, home: bool) -> str:
    out = f'<script src="{root}assets/site.js" defer></script>\n'
    if mermaid:
        out += f'<script src="{root}assets/mermaid.min.js" defer></script>\n'
    if home:
        out += f'<script src="{root}assets/home.js" defer></script>\n'
    return out


def sidebar(lang: str, url: str, build: Build, current: Doc | None) -> str:
    t = TEXT[lang]
    here = ' aria-current="page"' if current is None else ""
    parts = ['<nav class="side" id="side" aria-label="Documentation">',
             f'<a class="side-home" href="{relative(url, f"{lang}/docs/")}"{here}>{t["docs_title"]}</a>']
    for section in SECTIONS:
        parts.append(f"<h5>{t[section]}</h5><ul>")
        for doc, foreign in build.groups[(lang, section)]:
            mark = f' <span class="only">{t["other_only"]}</span>' if foreign else ""
            on = ' class="on" aria-current="page"' if current is not None and doc.path == current.path else ""
            parts.append(f'<li><a href="{relative(url, doc.url)}"{on} hreflang="{doc.lang}">{esc(doc.title)}{mark}</a></li>')
        parts.append("</ul>")
    parts.append("</nav>")
    return "\n".join(parts)


def counterpart(build: Build, doc: Doc) -> Doc | None:
    other = "en" if doc.lang == "ko" else "ko"
    for candidate in build.docs:
        if candidate.key == doc.key and candidate.lang == other:
            return candidate
    return None


def doc_page(build: Build, doc: Doc, lang: str) -> str:
    """A document page. A document shown in the other language's list is
    published once, under its own language."""
    t = TEXT[lang]
    root = up(doc.url)
    pair = counterpart(build, doc)
    other = "en" if lang == "ko" else "ko"
    switch = root + (pair.url if pair else f"{other}/docs/")
    toc = [h for h in doc.headings if h[0] in (2, 3)]
    toc_items = "".join(f'<li class="l{level}"><a href="#{anchor}">{esc(text)}</a></li>' for level, anchor, text in toc)
    source = json.dumps(doc.source, ensure_ascii=False).replace("<", "\\u003c")
    section_label = t[doc.section]
    only = f'<span class="only">{t["only"]}</span>' if pair is None else ""
    return (head(lang, f"{doc.title} · Jaekit", doc.summary or doc.title, root, build) +
            f'<body class="page-doc">\n' + nav(lang, root, switch, "docs", True) +
            f'<div class="docs">\n{sidebar(lang, doc.url, build, doc)}\n'
            f'<article class="article" id="article" data-source-path="{esc(doc.path)}">\n'
            f'<div class="page-bar"><span class="eyebrow">{section_label}{only}</span>'
            f'<span class="page-actions"><button type="button" class="btn" id="copy-page">{t["copy_page"]}</button>'
            f'<a class="btn" href="{REPO_URL}/blob/main/{quote(doc.path)}">{t["view_source"]}</a></span></div>\n'
            + (f'<details class="toc-mobile"><summary>{t["on_page"]}</summary><ul>{toc_items}</ul></details>\n' if toc else "")
            + f'<div class="markdown">\n{doc.body}</div>\n'
            f'<div class="doc-foot"><span><code>{esc(doc.path)}</code></span><span>{t["built"]} <code>{esc(build.commit[:12])}</code></span></div>\n'
            f'</article>\n<aside class="toc" aria-label="{esc(t["on_page"])}"><h5>{t["on_page"]}</h5><ul id="toc-list">{toc_items}</ul></aside>\n'
            f'</div>\n' + footer(lang, root, build) + palette(lang, root) +
            f'<script type="application/json" id="page-source">{source}</script>\n' +
            scripts(root, doc.mermaid, False) + "</body>\n</html>\n")


def docs_index(build: Build, lang: str) -> str:
    t = TEXT[lang]
    url = f"{lang}/docs/"
    root = up(url)
    other = "en" if lang == "ko" else "ko"
    sections = []
    for section in SECTIONS:
        items = []
        for doc, foreign in build.groups[(lang, section)]:
            mark = f' <span class="only">{t["other_only"]}</span>' if foreign else ""
            items.append(f'<a href="{relative(url, doc.url)}" hreflang="{doc.lang}"><b>{esc(doc.title)}{mark}</b>'
                         f'<span>{esc(doc.summary)}</span><code>{esc(doc.path)}</code></a>')
        sections.append(f'<h2 id="{section}">{t[section]}</h2>\n<div class="guides">{"".join(items)}</div>')
    return (head(lang, f"{t['docs_title']} · Jaekit", t["docs_lead"], root, build) +
            '<body class="page-doc">\n' + nav(lang, root, f"{root}{other}/docs/", "docs", True) +
            f'<div class="docs">\n{sidebar(lang, url, build, None)}\n<article class="article" id="article">\n'
            f'<h1>{t["docs_title"]}</h1>\n<p class="lead">{t["docs_lead"]}</p>\n' + "\n".join(sections) +
            f'\n</article>\n<aside class="toc" aria-label="{esc(t["on_page"])}"><h5>{t["on_page"]}</h5><ul id="toc-list">'
            + "".join(f'<li class="l2"><a href="#{s}">{t[s]}</a></li>' for s in SECTIONS) +
            '</ul></aside>\n</div>\n' + footer(lang, root, build) + palette(lang, root) +
            scripts(root, False, False) + "</body>\n</html>\n")


class LangFilter(HTMLParser):
    """Drop every element whose lang attribute names the other language."""

    def __init__(self, drop: str):
        super().__init__(convert_charrefs=False)
        self.drop, self.depth, self.out = drop, 0, []

    def handle_starttag(self, tag, attrs):
        if self.depth:
            if tag not in VOID:
                self.depth += 1
            return
        if dict(attrs).get("lang") == self.drop:
            if tag not in VOID:
                self.depth = 1
            return
        self.out.append(self.get_starttag_text())

    def handle_startendtag(self, tag, attrs):
        if not self.depth and dict(attrs).get("lang") != self.drop:
            self.out.append(self.get_starttag_text())

    def handle_endtag(self, tag):
        if self.depth:
            self.depth -= 1
            return
        self.out.append(f"</{tag}>")

    def emit(self, text):
        if not self.depth:
            self.out.append(text)

    def handle_data(self, data):
        self.emit(data)

    def handle_entityref(self, name):
        self.emit(f"&{name};")

    def handle_charref(self, name):
        self.emit(f"&#{name};")

    def handle_comment(self, data):
        pass  # template comments are not published

    def handle_decl(self, decl):
        self.emit(f"<!{decl}>")


def home_page(build: Build, lang: str) -> str:
    url = f"{lang}/"
    root = up(url)
    other = "en" if lang == "ko" else "ko"
    template = (SITE / "home.html").read_text(encoding="utf-8")
    parser = LangFilter(other)
    parser.feed(template)
    parser.close()
    body = "".join(parser.out)
    values = {"root": root, "lang": lang, "version": build.version, "install": build.install[lang],
              "install_page": f"{root}{lang}/guides/install/", "docs": f"{root}{lang}/docs/",
              "releases": f"{REPO_URL}/releases", "release": f"{REPO_URL}/releases/tag/v{build.version}",
              "repo": REPO_URL}
    for key, value in values.items():
        body = body.replace("{{" + key + "}}", esc(value))
    if "{{" in body:
        raise InputProblem(f"tools/site/home.html: unknown placeholder near {body[body.index('{{'):][:40]!r}")
    description = ("Spec이 목표 문서를 쓰고, Seal이 검사 기록으로 완료를 판단합니다." if lang == "ko" else
                   "Spec writes the goal document; Seal judges completion from check records.")
    title = "Jaekit — AI 코딩 작업, 목표부터 완료 기록까지" if lang == "ko" else "Jaekit — AI coding work, from goal to completion record"
    return (head(lang, title, description, root, build) + '<body class="page-home">\n' +
            nav(lang, root, f"{root}{other}/", "home", False) + body + footer(lang, root, build) +
            palette(lang, root) + scripts(root, False, True) + "</body>\n</html>\n")


def root_page(build: Build) -> str:
    return f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Jaekit</title>
<meta name="jaekit-commit" content="{esc(build.commit)}">
<meta name="jaekit-version" content="{esc(build.version)}">
<link rel="icon" href="assets/symbol.svg" type="image/svg+xml">
<link rel="stylesheet" href="assets/fonts.css">
<link rel="stylesheet" href="assets/site.css">
{THEME_BOOT}
<script>(function(){{var l=null;try{{l=localStorage.getItem("jaekit-lang")}}catch(e){{}}
if(l!=="ko"&&l!=="en"){{var n=(navigator.languages&&navigator.languages[0])||navigator.language||"";l=/^ko\\b/i.test(n)?"ko":"en"}}
location.replace(l+"/"+location.hash)}})();</script>
</head>
<body class="page-choose">
<main class="choose">
  <a class="brand" href="en/"><i></i><b>Jaekit</b></a>
  <p><a href="ko/" hreflang="ko" lang="ko" data-set-lang="ko">한국어</a> · <a href="en/" hreflang="en" data-set-lang="en">English</a></p>
</main>
<script src="assets/site.js" defer></script>
</body>
</html>
"""


def not_found(build: Build, base: str) -> str:
    return f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Not found · Jaekit</title>
<meta name="jaekit-commit" content="{esc(build.commit)}">
<link rel="icon" href="{base}assets/symbol.svg" type="image/svg+xml">
<link rel="stylesheet" href="{base}assets/fonts.css">
<link rel="stylesheet" href="{base}assets/site.css">
{THEME_BOOT}
</head>
<body class="page-choose">
<main class="choose">
  <a class="brand" href="{base}"><i></i><b>Jaekit</b></a>
  <p class="code">404</p>
  <h1><span lang="ko">페이지를 찾을 수 없습니다</span> · <span>Page not found</span></h1>
  <p><a href="{base}ko/" hreflang="ko" lang="ko" data-set-lang="ko">한국어 홈</a> · <a href="{base}en/" hreflang="en" data-set-lang="en">English home</a></p>
</main>
<script src="{base}assets/site.js" defer></script>
</body>
</html>
"""


def search_index(build: Build, lang: str) -> str:
    entries = []
    for section in SECTIONS:
        for doc, _ in build.groups[(lang, section)]:
            entries.append([doc.title, TEXT[lang][section], doc.url])
            for level, anchor, text in doc.headings:
                if level in (2, 3, 4):
                    entries.append([text, doc.title, f"{doc.url}#{anchor}"])
    return json.dumps(entries, ensure_ascii=False, separators=(",", ":")) + "\n"


def groups_for(docs: list[Doc]) -> dict:
    """Each language lists every document; one missing in that language
    links to the language it exists in."""
    def order(doc: Doc):
        if doc.section == "guides":
            rank = GUIDE_ORDER.index(doc.stem) if doc.stem in GUIDE_ORDER else len(GUIDE_ORDER)
        else:
            rank = 0 if doc.stem == "README" else 1
        return rank, doc.stem

    result = {}
    keys = sorted({doc.key for doc in docs}, key=lambda key: order(next(d for d in docs if d.key == key)))
    for lang in LANGS:
        for section in SECTIONS:
            row = []
            for key in keys:
                if key[0] != section:
                    continue
                same = [d for d in docs if d.key == key and d.lang == lang]
                row.append((same[0], False) if same else (next(d for d in docs if d.key == key), True))
            result[(lang, section)] = row
    return result


# ---------------------------------------------------------------- build

def build_site(root: Path, out: Path, base: str, cache: Path) -> None:
    vendor = json.loads((SITE / "vendor.json").read_text(encoding="utf-8"))
    names = tracked(root)
    version = release_version(root)
    install = {"ko": install_command(root, "guides/INSTALL.md"), "en": install_command(root, "guides/INSTALL.en.md")}
    docs = discover(root, names)
    for brand in ("assets/readme/brand/symbol.png", "assets/readme/brand/symbol.svg"):
        if brand not in names:
            raise InputProblem(f"{brand}: file is missing")
    commit = source_commit(root)
    md = load_markdown(vendor, cache)
    install_rules(md)
    for doc in docs:
        prepare(doc, md)
    links = Links(docs, names)
    for doc in docs:
        render(doc, md, links)
    if links.problems:
        raise InputProblem("\n".join(links.problems))
    assets, notice = vendor_assets(vendor, cache)
    build = Build(commit, version, install, docs, {d.path: d for d in docs}, groups_for(docs))

    pages: dict[str, bytes] = {}
    def put(name: str, text: str) -> None:
        pages[name] = text.encode("utf-8")

    put("index.html", root_page(build))
    put("404.html", not_found(build, base))
    for lang in LANGS:
        put(f"{lang}/index.html", home_page(build, lang))
        put(f"{lang}/docs/index.html", docs_index(build, lang))
        put(f"{lang}/search.json", search_index(build, lang))
    for doc in docs:
        put(f"{doc.url}index.html", doc_page(build, doc, doc.lang))
    for name in ("site.css", "site.js", "home.js"):
        pages[f"assets/{name}"] = (SITE / "static" / name).read_bytes()
    pages["assets/symbol.png"] = (root / "assets/readme/brand/symbol.png").read_bytes()
    pages["assets/symbol.svg"] = (root / "assets/readme/brand/symbol.svg").read_bytes()
    for name, data in assets.items():
        pages[f"assets/{name}"] = data
    pages["assets/licenses.txt"] = notice.encode("utf-8")

    out.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix=".site-", dir=out.parent))
    try:
        for name in sorted(pages):
            target = staging / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(pages[name])
        if out.exists():
            shutil.rmtree(out)
        staging.replace(out)
    finally:
        if staging.exists():
            shutil.rmtree(staging)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--root", type=Path, default=SITE.parents[1], help="repository to build from")
    parser.add_argument("--out", type=Path, default=SITE / "_site", help="output directory (replaced)")
    parser.add_argument("--base", default="/", help="URL path the site is served under, used by 404.html")
    parser.add_argument("--cache", type=Path, default=SITE / ".cache", help="download cache")
    args = parser.parse_args(argv)
    base = "/" + args.base.strip("/") + "/" if args.base.strip("/") else "/"
    try:
        build_site(args.root.resolve(), args.out.resolve(), base, args.cache.resolve())
    except InputProblem as error:
        print(f"site: {error}", file=sys.stderr)
        return 1
    except DownloadProblem as error:
        print(f"site: {error}", file=sys.stderr)
        return 3
    print(f"site: built {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
