"""The reviewer criteria in AGENTS.md agree with the executor rules beside them.

A reviewer treats an input failure counted as a violation as a defect only
for a producer of a condition requiring a baseline, with the exception the
check result contract allows. Counting an input failure as a pass is always
a defect, and a boundary stated after a finding does not cover it on either
side. Fixed phrases in one list item of `### For reviewers`, as in
test_review_rules.py.
"""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
_spec = importlib.util.spec_from_file_location("review_rules", ROOT / "tools/workflow-checks/test_review_rules.py")
rules = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(rules)


class ReviewerAgreement(rules.Observed):
    def reviewers(self):
        return rules.section(rules.section(rules.agents(), rules.RULES_HEADING), rules.REVIEWERS_HEADING)

    def test_declared_input_failures_are_not_defects(self):
        absent = rules.missing(self.reviewers(), [
            ("input failure", "as a violation is a defect only when",
             "producer of a condition that requires a baseline", "unless the check declares that case as a violation",
             "check result contract allows", "missing: violation"),
            ("any check that counts an input failure as a pass is a defect",),
        ])
        self.require(not absent, "reviewer-contract-conflict", f"AGENTS.md {rules.REVIEWERS_HEADING} lacks: {absent}")

    def test_late_boundaries_do_not_cover_earlier_findings(self):
        absent = rules.missing(self.reviewers(), [
            ("pull request description", "check boundaries", "a boundary added after a finding does not cover that finding"),
        ])
        self.require(not absent, "reviewer-boundary-timing", f"AGENTS.md {rules.REVIEWERS_HEADING} lacks: {absent}")


if __name__ == "__main__":
    unittest.main()
