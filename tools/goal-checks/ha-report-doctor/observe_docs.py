"""Contracts and guides describe both commands (AC-27).

Only fixed tokens are observed: command lines, the ten labels, the doctor
results and reasons, the section form, the limit, exit-code items and the
feature identifiers. A missing file or token is a violation of the document
property itself; an unreadable (non-UTF-8) file is unavailable.
"""
from pathlib import Path
import re
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from common import FEATURES, LABELS, REASONS, ROOT, Unavailable, require  # noqa: E402

SECTION_FORM = "- <이름>: `<점검 명령>`"


def read(rel):
    path = ROOT / rel
    if not path.is_file():
        return ""
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError as error:
        raise Unavailable(f"{rel}: {error}") from None


def missing(doc, tokens):
    return [t for t in tokens if t not in doc]


def headings(doc):
    return [l for l in doc.splitlines() if re.match(r"#{2,4} ", l)]


class Docs(unittest.TestCase):
    def test_contracts(self):
        """AC-27: run-record.md, bundle.md and core-capabilities.md describe the commands, the section and the features."""
        v = "contract-tokens-missing"
        record = read("contracts/run-record.md")
        tokens = ["`ha report <goal> [--lang ko|en]`", "`ha doctor <goal>`", "`--lang`", "`## 현재`",
                  "`사용 가능`", "`사용 불가`", "60초", "- `ha report`:", "- `ha doctor`:", "`no stored output`"]
        tokens += [f"`{label}`" for lang in ("en", "ko") for label in LABELS[lang]]
        tokens += [f"`{reason}`" for reason in REASONS]
        gaps = missing(record, tokens)
        for name in ("`ha report`", "`ha doctor`"):
            if not any(name in h for h in headings(record)):
                gaps.append(f"a run-record.md heading naming {name}")
        bundle = read("contracts/bundle.md")
        gaps += [f"bundle.md: {t}" for t in missing(bundle, [SECTION_FORM, "`ha doctor`", "60초"])]
        if not any("`## 실행 환경`" in h for h in headings(bundle)):
            gaps.append("a bundle.md heading naming `## 실행 환경`")
        capabilities = read("contracts/core-capabilities.md")
        rows = [l for l in capabilities.splitlines() if l.startswith("| `")]
        for feature in FEATURES:
            if not any(r.startswith(f"| `{feature}` |") for r in rows):
                gaps.append(f"core-capabilities.md: a feature row for `{feature}`")
        require(self, not gaps, v, f"missing: {gaps}")

    def test_guides(self):
        """AC-27: the four guides explain reading the five-line report and checking the environment."""
        v = "guide-tokens-missing"
        gaps = []
        for rel, lang in (("guides/USAGE.md", "ko"), ("guides/USAGE.en.md", "en"),
                          ("guides/OPERATIONS.md", "ko"), ("guides/OPERATIONS.en.md", "en")):
            doc = read(rel)
            tokens = ["`ha report", "`ha doctor", "`## 실행 환경`", SECTION_FORM]
            if rel.startswith("guides/USAGE"):
                tokens += ["--lang"] + [f"`{label}`" for label in LABELS[lang]]
            gaps += [f"{rel}: {t}" for t in missing(doc, tokens)]
        require(self, not gaps, v, f"missing: {gaps}")


if __name__ == "__main__":
    unittest.main()
