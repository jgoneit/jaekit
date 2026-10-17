"""The CI jobs that run the full Go suite and the docs group check out one commit without history.

The go group's public copies no longer repeat those suites (guides/CI.md,
`## 로컬 검사`). Whether ordinary Go tests or documentation regressions need
history shows only in these required jobs, so their checkout stays at the
default depth of one commit and nothing fetches more.
"""
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
SUITE_JOBS = {"go": "python3 tools/verify.py go", "docs": "python3 tools/verify.py docs"}


def job(text, key):
    """One job block in this repository's indentation-based CI configuration."""
    lines = text.splitlines()
    start = next((index for index, line in enumerate(lines) if line == f"  {key}:"), None)
    if start is None:
        raise AssertionError(f"CI is missing job {key!r}")
    body = []
    for line in lines[start + 1:]:
        if line.strip() and len(line) - len(line.lstrip()) <= 2:
            break
        body.append(line)
    return "\n".join(body)


def history_problems(body, command):
    problems = []
    if f"        run: {command}" not in body.splitlines():
        problems.append(f"does not run {command}")
    if len(re.findall(r"(?m)^        uses: actions/checkout@", body)) != 1:
        problems.append("does not check out exactly once")
    depth = re.findall(r"(?m)^          fetch-depth:\s*(\S+)", body)
    if any(value.strip("'\"") != "1" for value in depth):
        problems.append(f"checks out fetch-depth {depth}")
    if re.search(r"(?m)^          fetch-tags:\s*['\"]?true", body):
        problems.append("fetches tags")
    if re.search(r"git\s+(?:fetch|pull)|--unshallow|--deepen", body):
        problems.append("fetches more history")
    return problems


class HistoryLessCheckouts(unittest.TestCase):
    def test_suite_jobs_check_out_one_commit_without_history(self):
        text = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        for key, command in SUITE_JOBS.items():
            with self.subTest(job=key):
                self.assertEqual(history_problems(job(text, key), command), [])

    def test_more_history_in_a_suite_job_is_rejected(self):
        text = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        checkout = "        with:\n          persist-credentials: false"
        for key, command in SUITE_JOBS.items():
            body = job(text, key)
            self.assertIn(checkout, body)
            for case, changed in {
                    "full history": body.replace(checkout, checkout + "\n          fetch-depth: 0", 1),
                    "tags": body.replace(checkout, checkout + "\n          fetch-tags: true", 1),
                    "fetch step": body + "\n\n      - name: Fetch history\n        run: git fetch --unshallow origin",
                    "second checkout": body + "\n\n      - name: Check out again\n"
                                              "        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"}.items():
                with self.subTest(job=key, case=case):
                    self.assertNotEqual(history_problems(changed, command), [])


if __name__ == "__main__":
    unittest.main()
