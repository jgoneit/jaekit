#!/usr/bin/env python3
"""Build the Jaekit site from guides/, contracts/ and the release metadata.

    python3 tools/site/build.py [--root DIR] [--out DIR] [--base PATH] [--cache DIR]

The documents are rendered from the files of the commit checked out in --root
(its HEAD), read from Git as committed: files that are only staged, changed or
untracked are not used. Nothing is copied into this directory. Pinned third-party files (the Markdown parser,
fonts and the mermaid renderer) are downloaded once into --cache and verified
by their recorded digests before use.

Exit 0: built. 1: a problem in the commit's documents or metadata, named on
stderr; nothing is written. 2: usage, including an --out folder that is not an
earlier build. 3: a pinned download could not be fetched or verified. 4: a tool
the build needs cannot run (Python older than 3.10, Git, the pinned parser)
or the build stopped unexpectedly; nothing is written either.
"""
from __future__ import annotations

import argparse
import base64
from dataclasses import dataclass, field
import hashlib
import html
import http.client
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
import traceback
import unicodedata
from html.parser import HTMLParser
from urllib.parse import quote, unquote, urlsplit
import urllib.request

SITE = Path(__file__).resolve().parent
REPO_URL = "https://github.com/jgoneit/jaekit"
SECTIONS = ("guides", "contracts")
# The docs page and the navigation group documents by what their reader is
# doing, not by folder. A group names its documents by path without ".md";
# a document no group names joins its folder's group in GROUP_OF_FOLDER, in
# file-name order. A folded group is closed in the navigation unless the
# page being read is in it.
GROUPS = (
    ("start", False, ("guides/OVERVIEW", "guides/INSTALL", "guides/WSL")),
    ("use", False, ("guides/USAGE", "guides/OPERATIONS", "guides/SPAR")),
    ("formats", True, ("contracts/README", "contracts/goal-docs", "contracts/bundle", "contracts/run-record",
                       "contracts/check-result", "contracts/role-card", "contracts/review-round",
                       "contracts/review-gate", "contracts/spar-config", "contracts/spar-input",
                       "contracts/core-capabilities", "contracts/decision-history", "contracts/findings")),
    ("develop", True, ("guides/DEVELOPMENT", "guides/CI", "contracts/regressions",
                       "contracts/repository-producers", "guides/WINDOWS")),
)
GROUP_OF_FOLDER = {"guides": "use", "contracts": "formats"}
# The contracts keep their own titles and first paragraphs, which an earlier
# goal's preservation check pins; the lists show these instead.
LISTED = {
    "contracts/README": {"ko": ("형식과 규칙 개요", "이 묶음의 문서와 각 문서가 다루는 것."),
                         "en": ("Formats and rules overview", "The documents in this group and what each covers.")},
    "contracts/goal-docs": {"ko": ("목표 문서 형식 (SPEC.md)", "Spec이 쓰고 Seal이 읽는 SPEC.md의 절과 완료 조건(AC-n)의 형식."),
                            "en": ("Goal document format (SPEC.md)", "The sections of SPEC.md and its completion conditions (AC-n), written by Spec and read by Seal.")},
    "contracts/bundle": {"ko": ("실행 묶음 형식 (PLAN·REVIEW·PROGRESS)", "Seal이 쓰는 PLAN.md·REVIEW.md·PROGRESS.md의 구성과 완료 보고의 형식."),
                         "en": ("Execution bundle format (PLAN, REVIEW, PROGRESS)", "What PLAN.md, REVIEW.md and PROGRESS.md hold, and the completion report, written by Seal.")},
    "contracts/run-record": {"ko": ("실행 기록 형식과 완료 판정 (runs.jsonl)", "runs.jsonl의 줄 형식, ha 명령, 기록으로 상태와 완료를 계산하는 규칙."),
                             "en": ("Run record format and completion (runs.jsonl)", "The lines of runs.jsonl, the ha commands, and how status and completion are computed from the record.")},
    "contracts/check-result": {"ko": ("검사 선언과 결과 형식", "change 조건의 검사가 무엇을 확인하는지 선언하고 위반과 통과를 알리는 형식."),
                               "en": ("Check declarations and results", "How a check for a change condition declares what it checks and reports a violation or a pass.")},
    "contracts/role-card": {"ko": ("실행자의 권한과 완료 선언 규칙", "Seal을 따르는 에이전트가 할 수 있는 일과 완료를 선언하는 규칙."),
                            "en": ("The executor's authority and completion rules", "What an agent following Seal may do, and when it may declare completion.")},
    "contracts/review-round": {"ko": ("검토 라운드 기록 형식", "spar run이 남기는 라운드 기록, 지적의 분류와 처리."),
                               "en": ("Review round record format", "The round record spar run leaves, and how findings are classified and handled.")},
    "contracts/review-gate": {"ko": ("검토 gate 형식", "spar gate가 검토가 끝났는지 판단해 쓰는 gate 파일."),
                              "en": ("Review gate format", "The gate file spar gate writes when it judges whether review is finished.")},
    "contracts/spar-config": {"ko": ("Spar 설정 형식 (spar.toml)", "리뷰어와 검토 설정을 정하는 spar.toml."),
                              "en": ("Spar configuration format (spar.toml)", "The spar.toml files that choose reviewers and review settings.")},
    "contracts/spar-input": {"ko": ("Spar 입력 형식", "spar가 받는 입력의 형식과, 형식 밖 입력의 결과."),
                             "en": ("Spar input formats", "The inputs spar accepts, and what happens to input outside them.")},
    "contracts/core-capabilities": {"ko": ("ha 지원 기능과 호환 확인", "설치된 ha가 지원하는 기능을 ha capabilities로 확인하는 방법."),
                                    "en": ("ha capabilities and compatibility", "How ha capabilities shows what the installed ha supports.")},
    "contracts/decision-history": {"ko": ("판단 이력 형식 (선택 기능)", "ha history로 남기는 관측·원인·결정 같은 판단 이력의 형식."),
                                   "en": ("Decision history format (optional)", "The decision history recorded with ha history: observations, causes, decisions and more.")},
    "contracts/findings": {"ko": ("완료 뒤 발견 기록 형식", "완료 뒤에 발견한 문제를 원래 완료와 따로 남기는 기록."),
                           "en": ("Post-completion findings format", "How a problem found after completion is recorded apart from that completion.")},
    "contracts/regressions": {"ko": ("공통 규칙과 회귀 검사 연결", "Jaekit의 공통 규칙과 그것을 지키는 회귀 검사의 연결."),
                              "en": ("Common invariants and regression checks", "Jaekit's common invariants and the regression checks that guard them.")},
    "contracts/repository-producers": {"ko": ("저장소 검사 도구의 관측 범위", "Jaekit 저장소의 검사 도구가 무엇을 관측하는지."),
                                       "en": ("What the repository's check tools observe", "What the check tools of the Jaekit repository observe.")},
}
LANGS = ("ko", "en")
REQUIRED_CHECKS = ("Seal Core", "Public documentation", "Release candidate")
INSTALL_LINE = re.compile(r"^brew install \S+$")
# An output folder holding this file is an earlier build that may be replaced.
MARKER = ".jaekit-site"
VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link",
        "meta", "source", "track", "wbr"}

TEXT = {
    "ko": {
        "docs": "문서", "home": "홈",
        "releases": "릴리스", "search": "문서 검색", "on_page": "이 페이지",
        "copy_page": "페이지 복사", "view_source": "원본 보기", "only": "한국어만",
        "other_only": "영어만", "menu": "목차 열기", "theme": "밝은 테마와 어두운 테마 바꾸기",
        "lang": "Switch to English", "built": "만든 커밋", "licenses": "라이선스",
        "docs_title": "문서", "docs_lead": "처음이라면 '시작하기'의 문서를 순서대로 읽으세요. 이미 쓰고 있다면 '사용하기'에서 필요한 안내를 고릅니다. 파일 형식과 정확한 동작 규칙, Jaekit 개발 자료는 뒤에 따로 모았습니다. 모든 페이지는 저장소의 Markdown 파일에서 그대로 만들어집니다.",
        "group_start": "시작하기", "group_use": "사용하기", "group_formats": "파일·기록 형식과 동작 규칙", "group_develop": "Jaekit 개발",
        "intro_start": "Jaekit이 어떤 문제를 푸는지 이해하고, 설치한 뒤 첫 목표를 맡깁니다.",
        "intro_use": "목표를 고치거나 이어 가고, 멈춘 작업과 완료 보고를 읽고, 독립 검토를 준비할 때 찾아봅니다.",
        "intro_formats": "Jaekit이 읽고 쓰는 목표 문서, 작업 파일, 검사·검토 기록의 형식과, 그 기록으로 상태와 완료를 판단하는 규칙을 정확히 적은 문서입니다. 에이전트에게 일을 맡겨 쓰는 데는 읽지 않아도 됩니다. 기록을 읽는 도구를 만들거나 어떤 동작의 정확한 조건을 확인할 때 찾아봅니다. 저장소의 <code>contracts/</code> 폴더에 있으며, 문서 제목의 '계약'은 프로그램과 도구가 지키는 이 형식과 규칙을 뜻합니다.",
        "intro_develop": "Jaekit 자체를 빌드하고 고치고 검증할 때 읽습니다. Jaekit을 쓰는 데는 필요 없습니다.",
        "note_formats": "형식과 규칙 문서입니다. 에이전트에게 일을 맡겨 쓰는 데는 읽지 않아도 되고, 기록을 다루는 도구를 만들거나 정확한 동작을 확인할 때 봅니다. 처음이라면 <a href=\"{overview}\">Jaekit 이해하기</a>부터 읽으세요.",
        "note_develop": "Jaekit 개발 문서입니다. Jaekit을 쓰는 데는 필요 없습니다. 처음이라면 <a href=\"{overview}\">Jaekit 이해하기</a>부터 읽으세요.",
        "show": "문서 {n}개 보기",
        "move": "이동", "open": "열기", "placeholder": "무엇을 찾나요?", "empty": "일치하는 항목이 없습니다",
    },
    "en": {
        "docs": "Docs", "home": "Home",
        "releases": "Releases", "search": "Search docs", "on_page": "On this page",
        "copy_page": "Copy page", "view_source": "View source", "only": "English only",
        "other_only": "Korean only", "menu": "Open navigation", "theme": "Switch light or dark theme",
        "lang": "한국어로 보기", "built": "Built from", "licenses": "Licenses",
        "docs_title": "Documentation", "docs_lead": "New here? Read the documents under Get started in order. Already using Jaekit? Pick what you need under Use. File formats, exact rules and material for developing Jaekit come last. Every page is built as is from the repository's Markdown files.",
        "group_start": "Get started", "group_use": "Use", "group_formats": "File and record formats and rules", "group_develop": "Developing Jaekit",
        "intro_start": "Learn what problem Jaekit solves, install it, and hand over your first goal.",
        "intro_use": "Look here to change or continue a goal, read a stopped task or a completion report, or set up independent review.",
        "intro_formats": "These documents state exactly the formats of the goal documents, working files and check and review records Jaekit reads and writes, and the rules that compute status and completion from those records. You do not need them to hand work to your agent. Read them when you build a tool that reads the records or need the exact condition for a behavior. They live in the repository's <code>contracts/</code> folder; \"contract\" in their titles means these formats and rules the programs and tools keep. They are written in Korean.",
        "intro_develop": "Read these to build, change and verify Jaekit itself. You do not need them to use Jaekit.",
        "note_formats": "This is a formats and rules document. You do not need it to hand work to your agent; read it when you build a tool that handles the records or need the exact behavior. New here? Start with <a href=\"{overview}\">Understanding Jaekit</a>.",
        "note_develop": "This document is for developing Jaekit. You do not need it to use Jaekit. New here? Start with <a href=\"{overview}\">Understanding Jaekit</a>.",
        "show": "Show {n} documents",
        "move": "move", "open": "open", "placeholder": "What are you looking for?", "empty": "Nothing matches",
    },
}


class InputProblem(Exception):
    """An input the site is built from cannot be used (exit 1)."""


class DownloadProblem(Exception):
    """A pinned file could not be fetched or verified (exit 3)."""


class ToolProblem(Exception):
    """A tool the build needs cannot run (exit 4)."""


class UsageProblem(Exception):
    """The command line cannot be followed (exit 2)."""


# ---------------------------------------------------------------- git inputs

def git(root: Path, *args: str) -> str:
    return git_bytes(root, *args).decode("utf-8", "surrogateescape")


def git_bytes(root: Path, *args: str) -> bytes:
    try:
        result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, timeout=120)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ToolProblem(f"git cannot run: {error}") from None
    if result.returncode < 0:
        raise ToolProblem(f"git {' '.join(args)} ended by signal {-result.returncode}")
    if result.returncode:
        # Not a problem of the documents: Git cannot read this repository.
        detail = result.stderr.decode("utf-8", "replace").strip()
        raise ToolProblem(f"git {' '.join(args)} failed in {root}: {detail}")
    return result.stdout


class Tree:
    """The files of the commit the site is built from, as Git stores them."""

    def __init__(self, root: Path):
        self.root = root
        self.commit = git(root, "rev-parse", "HEAD").strip()
        self.entries: dict[str, tuple[str, str]] = {}
        for item in git_bytes(root, "ls-tree", "-r", "-z", "--full-tree", self.commit).split(b"\0"):
            meta, _, path = item.partition(b"\t")
            fields = meta.decode().split()
            if len(fields) == 3 and fields[1] == "blob":
                self.entries[path.decode("utf-8", "surrogateescape")] = (fields[0], fields[2])
        self.names = sorted(self.entries)
        self.dirs = {""}
        for name in self.names:
            while name:
                name = posixpath.dirname(name)
                self.dirs.add(name)
        self.blobs: dict[str, bytes] = {}

    def data(self, name: str, via: tuple[str, ...] = ()) -> bytes:
        if name not in self.entries:
            raise InputProblem(f"{name}: file is missing")
        mode, sha = self.entries[name]
        if sha not in self.blobs:
            self.blobs[sha] = git_bytes(self.root, "cat-file", "blob", sha)
        if mode != "120000":
            return self.blobs[sha]
        # A symbolic link is followed to the file it names in the same commit.
        target = posixpath.normpath(posixpath.join(posixpath.dirname(name), self.blobs[sha].decode("utf-8", "replace")))
        if target not in self.entries or target in via:
            raise InputProblem(f"{name}: a symbolic link to {target}, which is not a file of the commit")
        return self.data(target, via + (name,))

    def text(self, name: str) -> str:
        try:
            return self.data(name).decode("utf-8")  # line endings stay as committed
        except UnicodeError as error:
            raise InputProblem(f"{name}: not UTF-8 text: {error}") from None


def source_commit(tree: Tree) -> str:
    """The commit, marked -dirty when the site source used here differs from it."""
    try:
        own = SITE.relative_to(tree.root).as_posix()
    except ValueError:
        return tree.commit
    dirty = git(tree.root, "status", "--porcelain", "--untracked-files=no", "--", own)
    return tree.commit + ("-dirty" if dirty.strip() else "")


def release_version(tree: Tree) -> str:
    name = ".claude-plugin/marketplace.json"
    try:
        data = json.loads(tree.text(name))
    except json.JSONDecodeError as error:
        raise InputProblem(f"{name}: not valid JSON: {error}") from None
    version = data.get("metadata", {}).get("version") if isinstance(data, dict) and isinstance(data.get("metadata"), dict) else None
    if not isinstance(version, str) or not version.strip():
        raise InputProblem(f"{name}: metadata.version is missing or empty")
    return version.strip()


def release_url(version: str) -> str:
    return f"{REPO_URL}/releases/tag/v{version}"


def install_command(tree: Tree, name: str) -> str:
    in_fence = False
    for line in tree.text(name).splitlines():
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
    except (OSError, ValueError, http.client.HTTPException) as error:
        raise DownloadProblem(f"cannot download {entry['name']} {entry['version']} from {url}: {error}") from None
    if not verified(entry, data):
        raise DownloadProblem(f"{entry['name']} {entry['version']}: downloaded bytes do not match the pinned digest")
    try:
        cache.mkdir(parents=True, exist_ok=True)
        partial = target.with_suffix(target.suffix + ".part")
        partial.write_bytes(data)
        partial.replace(target)
    except OSError as error:
        raise DownloadProblem(f"cannot write the download cache {cache}: {error}") from None
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
        from markdown_it.token import Token
    except Exception as error:  # the verified wheel does not run on this Python
        raise ToolProblem(f"the pinned Markdown parser cannot be loaded: {type(error).__name__}: {error}") from None
    # As on GitHub: tables, strikethrough with one or two tildes, and bare
    # web and e-mail addresses as links.
    md = MarkdownIt("commonmark", {"html": True, "strikethrough_single_tilde": True}).enable(["table", "strikethrough"])
    md.core.ruler.before("text_join", "github_autolinks", lambda state: github_autolinks(state, Token))
    return md


# GitHub's autolinks (the GFM autolink extension): www. and http(s)://
# addresses and e-mail addresses in text, ended as GitHub ends them. GitHub
# finds and shows them as written: an entity such as &amp; stays as typed.

ASCII_PUNCT = set("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")
AUTOLINK_AFTER = {None, "softbreak", "hardbreak", "em_open", "em_close", "strong_open", "strong_close", "s_open", "s_close"}


def host_char(c: str) -> bool:
    return not c.isspace() and c not in ASCII_PUNCT and unicodedata.category(c)[0] != "P"


def ascii_alnum(c: str) -> bool:
    return c.isascii() and c.isalnum()


def domain_length(text: str, start: int, short: bool) -> int:
    size = len(text) - start
    if size < 1:
        return 0
    i, periods, under_last, under = 1, 0, 0, 0
    while i < size - 1:
        if text[start + i] == "\\" and i < size - 2:
            i += 1
        c = text[start + i]
        if c == "_":
            under += 1
        elif c == ".":
            under_last, under, periods = under, 0, periods + 1
        elif not host_char(c) and c != "-":
            break
        i += 1
    if (under_last or under) and periods <= 10:
        return 0  # an underscore in the last two parts of a host name
    return i if short or periods else 0


def autolink_end(text: str, start: int, end: int) -> int:
    """Leave out trailing punctuation, an unmatched ')' and an entity-like '&name;'."""
    if "<" in text[start:end]:
        end = text.index("<", start, end)
    opening, closing = text.count("(", start, end), text.count(")", start, end)
    while end > start:
        c = text[end - 1]
        if c == ")":
            if closing <= opening:
                break
            closing -= 1
            end -= 1
        elif c in "?!.,:*_~'\"":
            end -= 1
        elif c == ";":
            j = end - 2
            while j > start and text[j].isascii() and text[j].isalpha():
                j -= 1
            end = j if j < end - 2 and text[j] == "&" else end - 1
        else:
            break
    return end


def path_end(text: str, end: int) -> int:
    while end < len(text) and not text[end].isspace() and text[end] != "<":
        end += 1
    return end


def autolinks(text: str, open_start: bool) -> list[tuple[int, int, str]]:
    """(start, end, address) of each autolink GitHub finds in the text."""
    found, i, last = [], 0, 0
    while i < len(text):
        c, link = text[i], None
        before = text[i - 1] if i else None
        if c == "w" and text.startswith("www.", i) and (open_start if before is None else before.isspace() or before in "*_~("):
            length = domain_length(text, i, False)
            if length:
                end = autolink_end(text, i, path_end(text, i + length))
                if end > i:
                    link = (i, end, "http://")
        elif c == ":" and text.startswith("//", i + 1):
            start = i
            while start > last and text[start - 1].isascii() and text[start - 1].isalpha():
                start -= 1
            if text[start:i + 3].lower() in ("http://", "https://") and i + 3 < len(text) and ascii_alnum(text[i + 3]):
                length = domain_length(text, i + 3, True)
                if length:
                    end = autolink_end(text, start, path_end(text, i + 3 + length))
                    if end > start:
                        link = (start, end, "")
        elif c == "@":
            start = i
            while start > last and (ascii_alnum(text[start - 1]) or text[start - 1] in ".+-_"):
                start -= 1
            if start < i:
                end, ats, dots = i, 0, 0
                while end < len(text):
                    d = text[end]
                    if d == "@":
                        ats += 1
                    elif d == "." and end < len(text) - 1 and ascii_alnum(text[end + 1]):
                        dots += 1
                    elif not ascii_alnum(d) and d not in "-_":
                        break
                    end += 1
                if end - i >= 2 and ats == 1 and dots and (text[end - 1].isascii() and text[end - 1].isalpha() or text[end - 1] == "."):
                    end = autolink_end(text, i, end)
                    if end > i:
                        link = (start, end, "mailto:")
        if link:
            found.append(link)
            i = last = link[1]
        else:
            i += 1
    return found


def github_autolinks(state, Token) -> None:
    md = state.md
    for block in state.tokens:
        if block.type != "inline" or not block.children:
            continue
        children, out, depth, i = block.children, [], 0, 0
        while i < len(children):
            child = children[i]
            if child.type not in ("text", "text_special") or depth:
                # Not inside a Markdown link; GitHub does link inside raw HTML.
                if child.type == "link_open":
                    depth += 1
                elif child.type == "link_close":
                    depth -= 1
                out.append(child)
                i += 1
                continue
            # A run of text as written: entities and escapes keep their markup.
            run = []
            while i < len(children) and children[i].type in ("text", "text_special"):
                run.append(children[i])
                i += 1
            pieces, written = [], ""
            for token in run:
                start = len(written)
                written += token.markup if token.type == "text_special" else token.content
                pieces.append((start, len(written), token))
            before = out[-1].type if out else None
            links = autolinks(written, before in AUTOLINK_AFTER)

            def shown(a: int, b: int) -> str:
                text = []
                for start, end, token in pieces:
                    if end <= a or start >= b:
                        continue
                    text.append(token.content if token.type == "text_special" else token.content[max(a, start) - start:min(b, end) - start])
                return "".join(text)

            def text_token(content: str) -> None:
                if content:
                    token = Token("text", "", 0)
                    token.content = content
                    out.append(token)

            if not links:
                out.extend(run)
                continue
            at = 0
            for start, end, scheme in links:
                href = md.normalizeLink(scheme + written[start:end])
                if not md.validateLink(href):
                    continue
                text_token(shown(at, start))
                opening = Token("link_open", "a", 1)
                opening.attrs = {"href": href}
                opening.markup, opening.info = "linkify", "auto"
                out.append(opening)
                text_token(written[start:end])
                closing = Token("link_close", "a", -1)
                closing.markup, closing.info = "linkify", "auto"
                out.append(closing)
                at = end
            text_token(shown(at, len(written)))
        block.children = out


def load_public_docs():
    """tools/check_public_docs.py beside this builder: its links() and
    anchors() are the rule the repository's documents are checked by."""
    sys.path.insert(0, str(SITE.parent))
    try:
        import check_public_docs
    except Exception as error:
        raise ToolProblem(f"tools/check_public_docs.py cannot be loaded: {type(error).__name__}: {error}") from None
    finally:
        sys.path.pop(0)
    return check_public_docs


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
        if self.stem == "README":
            return ""
        slug = self.stem.lower()
        # A name of dots alone would be the folder itself or the one above.
        return "_" + slug if not slug.strip(".") else slug

    @property
    def dir(self) -> str:
        """Where the page is written."""
        return f"{self.lang}/{self.section}/" + (f"{self.slug}/" if self.slug else "")

    @property
    def url(self) -> str:
        """The page's address, with characters such as # or ? in the name encoded."""
        return f"{self.lang}/{self.section}/" + (f"{quote(self.slug, safe='')}/" if self.slug else "")

    @property
    def key(self) -> tuple[str, str]:
        return self.section, self.stem


def discover(tree: Tree) -> list[Doc]:
    docs = []
    for name in tree.names:
        section, _, file = name.partition("/")
        if section not in SECTIONS or "/" in file or not file.endswith(".md"):
            continue
        lang, stem = ("en", file[:-6]) if file.endswith(".en.md") else ("ko", file[:-3])
        if not stem:
            raise InputProblem(f"{name}: no document name before the extension")
        docs.append(Doc(name, section, stem, lang, tree.text(name)))
    seen: dict[str, str] = {}
    for doc in docs:
        if doc.dir in seen:
            raise InputProblem(f"{doc.path} and {seen[doc.dir]} would both be {doc.dir}")
        seen[doc.dir] = doc.path
    if not docs:
        raise InputProblem("no Markdown files under guides/ or contracts/ in the commit")
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
HTML_COMMENT = re.compile(r"(<!--.*?-->)", re.S)


def html_ids(text: str) -> list[str]:
    """The ids an HTML fragment gives the page; comments give none."""
    return HTML_ID.findall(HTML_COMMENT.sub("", text))


def prepare(doc: Doc, md) -> None:
    """Parse once, give every heading GitHub's anchor and collect titles.
    Every heading GitHub shows gets one, by the rule tools/check_public_docs.py
    computes with anchors(), including the forms anchors() does not read."""
    doc.tokens = md.parse(doc.source)
    counts: dict[str, int] = {}
    tokens = doc.tokens
    for index, token in enumerate(tokens):
        if token.type in ("html_block", "html_inline"):
            doc.ids.update(html_ids(token.content))
        if token.type == "inline":
            for child in token.children or []:
                if child.type == "html_inline":
                    doc.ids.update(html_ids(child.content))
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
        doc.title = doc.stem  # no '#' title: the file's name


class Links:
    def __init__(self, docs: list[Doc], tree: Tree, public_docs):
        self.pages = {doc.path: doc for doc in docs}
        self.tree = tree
        self.files = set(tree.names)
        self.dirs = tree.dirs
        self.public_docs = public_docs
        self.anchors: dict[str, set[str]] = {}
        self.problems: list[str] = []

    def check(self, doc: Doc) -> None:
        """Every relative link tools/check_public_docs.py reads in the file,
        checked by its rules against the commit: the target exists, stays in
        the repository, and a fragment names an anchor of a Markdown, HTML or
        SVG file. Reference definitions are links too, used or not."""
        for target in self.public_docs.links(doc.source):
            url = urlsplit(target)
            if url.scheme or url.netloc:
                continue
            fragment = unquote(url.fragment)
            dest = posixpath.normpath(posixpath.join(posixpath.dirname(doc.path), unquote(url.path))) if url.path else doc.path
            if dest == ".." or dest.startswith("../"):
                self.problems.append(f"{doc.path}: link leaves the repository: {target}")
            elif dest in self.files:
                if not fragment:
                    continue
                if posixpath.splitext(dest)[1].lower() not in (".md", ".html", ".svg"):
                    self.problems.append(f"{doc.path}: link fragment into a file without headings: {target}")
                elif fragment not in self.anchors_of(dest):
                    self.problems.append(f"{doc.path}: link to a missing heading: {target}")
            elif dest != "." and dest not in self.dirs:
                self.problems.append(f"{doc.path}: link target is missing: {target}")

    def anchors_of(self, name: str) -> set[str]:
        if name not in self.anchors:
            self.anchors[name] = self.public_docs.anchors(self.tree.text(name))
        return self.anchors[name]

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
        if target == ".":
            return f"{REPO_URL}/tree/main{suffix}"
        if target in self.dirs:
            return f"{REPO_URL}/tree/main/{quote(target)}{suffix}"
        self.problems.append(f"{doc.path}: link target is missing: {href}")
        return href

    def rewrite_html(self, doc: Doc, text: str) -> str:
        # Comments are not shown, and tools/check_public_docs.py reads no links in them.
        parts = HTML_COMMENT.split(text)
        return "".join(part if index % 2 else HTML_LINK.sub(
            lambda m: m.group(1) + m.group(2) + m.group(3) +
            html.escape(self.rewrite(doc, html.unescape(m.group(4))), quote=True) + m.group(3), part)
            for index, part in enumerate(parts))


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
    <p class="built">{t['built']} <code title="{esc(build.commit)}">{esc(short)}</code> · <a href="{esc(release_url(build.version))}">v{esc(build.version)}</a></p>
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
    for group, folded, _ in GROUPS:
        rows = build.groups[(lang, group)]
        if not rows:
            continue
        label = t["group_" + group]
        if folded:
            here = current is not None and any(doc.path == current.path for doc, _ in rows)
            parts.append(f'<details class="side-group"{" open" if here else ""}><summary>{label}</summary><ul>')
        else:
            parts.append(f"<h5>{label}</h5><ul>")
        for doc, foreign in rows:
            mark = f' <span class="only">{t["other_only"]}</span>' if foreign else ""
            on = ' class="on" aria-current="page"' if current is not None and doc.path == current.path else ""
            parts.append(f'<li><a href="{relative(url, doc.url)}"{on} hreflang="{doc.lang}">{esc(listed(doc, lang)[0])}{mark}</a></li>')
        parts.append("</ul></details>" if folded else "</ul>")
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
    group = group_of(doc)
    section_label = t["group_" + group]
    only = f'<span class="only">{t["only"]}</span>' if pair is None else ""
    note = ""
    if "note_" + group in t:
        overview = relative(doc.url, f"{lang}/guides/overview/")
        note = '<p class="doc-note">' + t["note_" + group].replace("{overview}", overview) + "</p>\n"
    return (head(lang, f"{doc.title} · Jaekit", doc.summary or doc.title, root, build) +
            f'<body class="page-doc">\n' + nav(lang, root, switch, "docs", True) +
            f'<div class="docs">\n{sidebar(lang, doc.url, build, doc)}\n'
            f'<article class="article" id="article" data-source-path="{esc(doc.path)}">\n'
            f'<div class="page-bar"><span class="eyebrow">{section_label}{only}</span>'
            f'<span class="page-actions"><button type="button" class="btn" id="copy-page">{t["copy_page"]}</button>'
            f'<a class="btn" href="{REPO_URL}/blob/main/{quote(doc.path)}">{t["view_source"]}</a></span></div>\n'
            + note
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
    sections, shown = [], []
    for group, folded, _ in GROUPS:
        rows = build.groups[(lang, group)]
        if not rows:
            continue
        items = []
        for doc, foreign in rows:
            mark = f' <span class="only">{t["other_only"]}</span>' if foreign else ""
            title, summary = listed(doc, lang)
            items.append(f'<a href="{relative(url, doc.url)}" hreflang="{doc.lang}"><b>{esc(title)}{mark}</b>'
                         f'<span>{esc(summary)}</span><code>{esc(doc.path)}</code></a>')
        grid = f'<div class="guides">{"".join(items)}</div>'
        if folded:
            grid = f'<details class="group-docs"><summary>{t["show"].format(n=len(rows))}</summary>{grid}</details>'
        shown.append(group)
        sections.append(f'<h2 id="{group}">{t["group_" + group]}</h2>\n<p class="group-intro">{t["intro_" + group]}</p>\n{grid}')
    return (head(lang, f"{t['docs_title']} · Jaekit", t["docs_lead"], root, build) +
            '<body class="page-doc">\n' + nav(lang, root, f"{root}{other}/docs/", "docs", True) +
            f'<div class="docs">\n{sidebar(lang, url, build, None)}\n<article class="article" id="article">\n'
            f'<h1>{t["docs_title"]}</h1>\n<p class="lead">{t["docs_lead"]}</p>\n' + "\n".join(sections) +
            f'\n</article>\n<aside class="toc" aria-label="{esc(t["on_page"])}"><h5>{t["on_page"]}</h5><ul id="toc-list">'
            + "".join(f'<li class="l2"><a href="#{g}">{t["group_" + g]}</a></li>' for g in shown) +
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
              "install_page": f"{root}{lang}/guides/install/", "overview": f"{root}{lang}/guides/overview/",
              "docs": f"{root}{lang}/docs/",
              "releases": f"{REPO_URL}/releases", "release": release_url(build.version),
              "repo": REPO_URL}
    for key, value in values.items():
        body = body.replace("{{" + key + "}}", esc(value))
    if "{{" in body:
        raise InputProblem(f"tools/site/home.html: unknown placeholder near {body[body.index('{{'):][:40]!r}")
    description = ("무엇을 하면 끝인지 작업 전에 목표로 정해 두고, 조건마다 남은 검사 기록으로 완료를 판단합니다." if lang == "ko" else
                   "Write down what finished means before the work starts, and judge completion from a check record for each condition.")
    title = "Jaekit — 맡긴 작업이 정말 끝났는지, 기록으로 확인합니다" if lang == "ko" else "Jaekit — Is the work really done? Check the record"
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
<script>(function(){{var l;try{{l=localStorage.getItem("jaekit-lang")}}catch(e){{return}}
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


TAG = re.compile(r"<[^>]+>")


def section_texts(doc: Doc) -> dict[str, str]:
    """The text under each subheading (levels 2 to 6), code included, keyed
    by its anchor; "" holds the text before the first one."""
    parts: dict[str, list[str]] = {"": []}
    current, in_heading = "", False
    for token in doc.tokens:
        if token.type == "heading_open":
            in_heading = True
            if int(token.tag[1]) >= 2:
                current = token.attrGet("id")
                parts[current] = []
        elif token.type == "heading_close":
            in_heading = False
        elif token.type == "inline" and not in_heading:
            parts[current].append(plain(token))
        elif token.type in ("fence", "code_block"):
            parts[current].append(token.content)
        elif token.type == "html_block":
            parts[current].append(html.unescape(TAG.sub(" ", token.content)))
    return {key: " ".join(" ".join(chunks).split()) for key, chunks in parts.items()}


def search_index(build: Build, lang: str) -> str:
    """Entries of [title, label, url, text]: each document and each of its
    subheadings (levels 2 to 6), with the text of that part of the page."""
    entries = []
    for group, _, _ in GROUPS:
        for doc, _ in build.groups[(lang, group)]:
            texts = section_texts(doc)
            title = listed(doc, lang)[0]
            lead = texts[""] if title == doc.title else f"{doc.title} {texts['']}"
            entries.append([title, TEXT[lang]["group_" + group], doc.url, lead])
            for level, anchor, text in doc.headings:
                if level >= 2:
                    entries.append([text, doc.title, f"{doc.url}#{anchor}", texts.get(anchor, "")])
    return json.dumps(entries, ensure_ascii=False, separators=(",", ":")) + "\n"


def group_of(doc: Doc) -> str:
    name = f"{doc.section}/{doc.stem}"
    for group, _, names in GROUPS:
        if name in names:
            return group
    return GROUP_OF_FOLDER[doc.section]


def listed(doc: Doc, lang: str) -> tuple[str, str]:
    """The title and one-line summary a list shows for a document."""
    return LISTED.get(f"{doc.section}/{doc.stem}", {}).get(lang, (doc.title, doc.summary))


def groups_for(docs: list[Doc]) -> dict:
    """Each language lists every document; one missing in that language
    links to the language it exists in."""
    first = {}
    for doc in docs:
        first.setdefault(doc.key, doc)

    def order(key: tuple[str, str]):
        names = next(names for g, _, names in GROUPS if g == group_of(first[key]))
        name = f"{key[0]}/{key[1]}"
        return (names.index(name), "") if name in names else (len(names), key[1])

    result = {}
    keys = sorted(first, key=order)
    for lang in LANGS:
        for group, _, _ in GROUPS:
            row = []
            for key in keys:
                if group_of(first[key]) != group:
                    continue
                same = [d for d in docs if d.key == key and d.lang == lang]
                row.append((same[0], False) if same else (first[key], True))
            result[(lang, group)] = row
    return result


# ---------------------------------------------------------------- build

def build_site(root: Path, out: Path, base: str, cache: Path) -> None:
    if out.exists() and not (out / MARKER).is_file() and (not out.is_dir() or any(out.iterdir())):
        raise UsageProblem(f"{out} exists and is not an earlier site build; choose an empty or new --out")
    vendor = json.loads((SITE / "vendor.json").read_text(encoding="utf-8"))
    public_docs = load_public_docs()
    tree = Tree(root)
    version = release_version(tree)
    install = {"ko": install_command(tree, "guides/INSTALL.md"), "en": install_command(tree, "guides/INSTALL.en.md")}
    docs = discover(tree)
    brand = {name: tree.data(f"assets/readme/brand/{name}") for name in ("symbol.png", "symbol.svg")}
    commit = source_commit(tree)
    md = load_markdown(vendor, cache)
    install_rules(md)
    for doc in docs:
        prepare(doc, md)
    links = Links(docs, tree, public_docs)
    for doc in docs:
        links.check(doc)
        render(doc, md, links)
    if links.problems:
        raise InputProblem("\n".join(dict.fromkeys(links.problems)))
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
        put(f"{doc.dir}index.html", doc_page(build, doc, doc.lang))
    for name in ("site.css", "site.js", "home.js"):
        pages[f"assets/{name}"] = (SITE / "static" / name).read_bytes()
    pages["assets/symbol.png"] = brand["symbol.png"]
    pages["assets/symbol.svg"] = brand["symbol.svg"]
    for name, data in assets.items():
        pages[f"assets/{name}"] = data
    pages["assets/licenses.txt"] = notice.encode("utf-8")

    pages[MARKER] = b"Built by tools/site/build.py; the next build replaces this folder.\n"
    out.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix=".site-", dir=out.parent))
    try:
        # mkdtemp makes a private folder; Pages needs the site readable by others.
        staging.chmod(0o755)
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
    if sys.version_info < (3, 10):
        print(f"site: Python 3.10 or later is needed; this is Python {sys.version.split()[0]}", file=sys.stderr)
        return 4
    base = "/" + args.base.strip("/") + "/" if args.base.strip("/") else "/"
    try:
        build_site(args.root.resolve(), args.out.resolve(), base, args.cache.resolve())
    except InputProblem as error:
        print(f"site: {error}", file=sys.stderr)
        return 1
    except UsageProblem as error:
        print(f"site: {error}", file=sys.stderr)
        return 2
    except DownloadProblem as error:
        print(f"site: {error}", file=sys.stderr)
        return 3
    except ToolProblem as error:
        print(f"site: {error}", file=sys.stderr)
        return 4
    except Exception:
        # Not an input problem the build can name: report it as a tool failure.
        traceback.print_exc()
        print("site: the build stopped unexpectedly", file=sys.stderr)
        return 4
    print(f"site: built {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
