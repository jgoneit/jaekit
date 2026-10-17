"""Repository rules for check boundaries, review findings and reviewers.

These are document checks: each rule is a set of fixed phrases that must
appear together in one list item of `## Checks and review` in AGENTS.md, or in
the Review focus section of the pull request template. They establish that the
rule text is present and consistent, not that a reader follows it. A missing
file is an error, never a violation; only an absent rule is a violation.
"""
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
RULES_HEADING = "## Checks and review"
REVIEWERS_HEADING = "### For reviewers"
REVIEW_FOCUS = "## 리뷰 포인트 / Review focus"


def normalized(text):
    text = text.replace("’", "'").replace("**", "").replace("`", "")
    return re.sub(r"\s+", " ", text).strip().lower()


def section(text, heading):
    """Return the body under a heading up to the next heading of the same level, or ""."""
    level = len(heading) - len(heading.lstrip("#"))
    lines = text.splitlines()
    try:
        start = lines.index(heading) + 1
    except ValueError:
        return ""
    end = start
    while end < len(lines):
        match = re.match(r"(#{1,6}) ", lines[end])
        if match and len(match.group(1)) <= level:
            break
        end += 1
    return "\n".join(lines[start:end])


def items(text):
    """Split Markdown into list items, each joined with its continuation lines."""
    found = []
    current = None
    for line in text.splitlines():
        marker = re.match(r" {0,3}[-*] ", line)
        if marker:
            current = line[marker.end():]
            found.append(current)
        elif current is not None and line.strip() and not line.lstrip().startswith("#"):
            current += " " + line.strip()
            found[-1] = current
        else:
            current = None
    return [normalized(item) for item in found]


def missing(text, rules):
    """Name each rule whose phrases do not all appear in one list item."""
    listed = items(text)
    return [" + ".join(rule) for rule in rules
            if not any(all(phrase in item for phrase in rule) for item in listed)]


def agents():
    return (ROOT / "AGENTS.md").read_text(encoding="utf-8")


class Observed(unittest.TestCase):
    def require(self, condition, violation, detail=""):
        # The tag names the observed requirement for structured results.
        if not condition:
            self.fail(f"[violation:{violation}] {detail}")


class CheckBoundaries(Observed):
    def test_boundaries_are_recorded_in_review_before_findings(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("for each new check", "observation boundary", "failure kinds", "review.md", "before the plan round"),
            ("grounds for classifying a finding", "only when", "recorded before that finding"),
        ])
        self.require(not absent, "boundary-record-rule-missing", f"AGENTS.md lacks: {absent}")

    def test_boundaries_cannot_exclude_required_inputs(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("spec criterion", "contract", "protected regression path", "selected assurance", "outside",
             "boundary"),
            ("counterexample", "fixed", "blocks completion"),
            ("narrowing a boundary", "scope change", "user decides"),
        ])
        self.require(not absent, "boundary-limit-rule-missing", f"AGENTS.md lacks: {absent}")

    def test_input_failures_have_declared_result_meanings(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("for each producer", "before the plan round", "violation", "error", "inconclusive",
             "missing target file", "corrupt input", "cannot start", "signal"),
            ("check result contract", "does not declare as a violation", "never counted"),
        ])
        self.require(not absent, "input-failure-rule-missing", f"AGENTS.md lacks: {absent}")

    def test_new_checks_avoid_open_scans_and_reuse_ci_observation(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("open-ended", "goal documents", "representative forms"),
            ("new checks", "fixed tokens", "commands", "links", "version", "instead of scanning an open category"),
            ("new ci observer", "reuses", "tools/verification-checks/cross_platform_ci.py",
             "only its goal-specific judgment"),
            ("protected path", "changes only in a separate goal"),
        ])
        self.require(not absent, "check-authoring-rule-missing", f"AGENTS.md lacks: {absent}")


class ReviewFindings(Observed):
    def test_remote_findings_use_the_local_triage(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("remote pull request review", "same triage", "fix", "reject", "defer", "duplicate", "linked",
             "criterion", "rule", "contract", "introduced"),
            ("defer only", "outside the recorded boundary", "linked to nothing"),
            ("deferred findings", "pull request description", "completion report"),
        ])
        self.require(not absent, "remote-triage-rule-missing", f"AGENTS.md lacks: {absent}")

    def test_pull_request_rounds_stop_after_three(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("more than three", "remote review rounds", "criterion-linked fixes", "stop", "recurring findings",
             "criterion they trace to", "two readings", "wait for the answer"),
        ])
        self.require(not absent, "pr-round-limit-missing", f"AGENTS.md lacks: {absent}")

    def test_classification_uses_the_reviewed_commit(self):
        absent = missing(section(agents(), RULES_HEADING), [
            ("as of the reviewed commit", "boundary records", "protected paths", "widened",
             "does not reclassify"),
        ])
        self.require(not absent, "classification-time-rule-missing", f"AGENTS.md lacks: {absent}")

    def test_reviewers_have_their_own_criteria(self):
        reviewers = section(section(agents(), RULES_HEADING), REVIEWERS_HEADING)
        absent = missing(reviewers, [
            ("links to a criterion, rule or contract", "introduced"),
            ("counterexample", "input that a spec criterion, a contract or a protected regression path requires",
             "is a defect"),
            ("edge case", "outside the stated check boundary", "links to nothing", "low severity"),
            ("input failure", "missing file", "corrupt input", "cannot start", "signal", "violation", "defect"),
            ("pull request description", "check boundaries", "grounds"),
        ])
        self.require(reviewers.strip() and not absent, "reviewer-criteria-missing",
                     f"AGENTS.md {REVIEWERS_HEADING} lacks: {absent or 'the section'}")


class PullRequestTemplate(Observed):
    def test_review_focus_asks_for_boundaries_and_deferred_findings(self):
        template = (ROOT / ".github/pull_request_template.md").read_text(encoding="utf-8")
        focus = section(template, REVIEW_FOCUS)
        text = normalized(focus)
        korean = ("관측", "경계", "덮지 않는", "defer한", "비공개 목표", "기록", "로컬 경로", "넣지 말")
        english = ("observation boundary", "does not cover", "deferred review findings", "private goal names",
                   "records", "local paths", "leave out")
        absent = [phrase for phrase in korean + english if phrase not in text]
        self.require(focus.strip() and not absent, "pr-template-boundary-missing",
                     f"pull request template Review focus lacks: {absent or 'the section'}")


if __name__ == "__main__":
    unittest.main()
