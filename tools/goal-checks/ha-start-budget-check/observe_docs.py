"""Contracts and guides describe the changed behavior (AC-32).

The test reads the named files of the checked tree and looks for fixed tokens
in each: the names and messages this goal's contract gives (PLAN 구현 결정
R1-R9). A token must be in the named file, and for contracts/bundle.md inside
its §2.4 section. contracts/core-capabilities.md must no longer say that the
Core adds no start refusal. The guides' examples use the synthetic goal path
docs/specs/example. Wording beyond the tokens is left to review.
"""
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from budget_common import ROOT, Unavailable, require

MARKER = "검증 실행: 자동"
CONFIG = ["ha.json", "preapproved_runs", "XDG_CONFIG_HOME", "~/.config/jaekit"]
MESSAGES = ["exceeds the pre-approved limit", "pre-approved run limit"]
TOKENS = {
    "contracts/run-record.md": [MARKER, *CONFIG, *MESSAGES, "max(50, ⌈", "approval_needed", "--check-skill",
                                "core-requirements.json", "missing Core support", '"auto"'],
    "contracts/core-capabilities.md": ["core-requirements/v1", "core-requirements.json", "core_requirements",
                                       "auto-budget/v1", "--check-skill", "missing Core support"],
    "guides/USAGE.md": [MARKER, *CONFIG, *MESSAGES, "docs/specs/example"],
    "guides/USAGE.en.md": [MARKER, *CONFIG, *MESSAGES, "docs/specs/example"],
    "guides/OPERATIONS.md": [MARKER, *CONFIG, *MESSAGES],
    "guides/OPERATIONS.en.md": [MARKER, *CONFIG, *MESSAGES],
}
BUNDLE_SECTION = "### 2.4 예산 (`## 예산`)"
BUNDLE_TOKENS = [MARKER, "ha.json"]
REMOVED = {"contracts/core-capabilities.md": "Core 자체에 새 시작 강제 차단을 추가하는 계약도 아니다"}


def text(path):
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except OSError as error:
        raise Unavailable(f"{path}: {error}") from None


def section(document, heading):
    start = document.find(heading)
    if start < 0:
        return None
    level = heading.split(" ", 1)[0]
    rest = document[start + len(heading):]
    ends = [rest.find("\n" + "#" * n + " ") for n in range(1, len(level) + 1)]
    ends = [e for e in ends if e >= 0]
    return rest[:min(ends)] if ends else rest


class Docs(unittest.TestCase):
    """AC-32: run-record, bundle §2.4, core-capabilities and the four guides carry the changed behavior."""

    def test_contracts_and_guides(self):
        problems = []
        for path, tokens in TOKENS.items():
            body = text(path)
            absent = [t for t in tokens if t not in body]
            if absent:
                problems.append(f"{path} lacks {absent}")
        bundle = section(text("contracts/bundle.md"), BUNDLE_SECTION)
        if bundle is None:
            problems.append(f"contracts/bundle.md has no {BUNDLE_SECTION}")
        else:
            absent = [t for t in BUNDLE_TOKENS if t not in bundle]
            if absent:
                problems.append(f"contracts/bundle.md §2.4 lacks {absent}")
        for path, sentence in REMOVED.items():
            if sentence in text(path):
                problems.append(f"{path} still says {sentence!r}")
        require(self, not problems, "docs-missing", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
