"""The installer's name-rule characters are refused, and CI requires it in both shells.

The native checks place every character of the installer's name rule in an
entry name of the actual candidate and require the installer's own refusal in
the shell being checked. These tests keep that set equal to the rule in
common.ps1, keep the report validator strict, and keep both CI shells running it.
"""
import re
import unittest
from pathlib import Path

import native_checks as native

ROOT = Path(__file__).resolve().parents[2]
CASE = "archive-name-character-refusal"


def rule_characters():
    """Code points in Assert-PackageName's character class (tools/windows/common.ps1)."""
    source = (ROOT / "tools/windows/common.ps1").read_text(encoding="utf-8")
    start = source.index("function Assert-PackageName")
    body = source[start:source.index("\nfunction ", start + 1)]
    classes = re.findall(r"\$part -match '\[([^\]']+)\]'", body)
    if len(classes) != 1:
        raise ValueError("expected one character class in Assert-PackageName")
    codes = set()
    for low, high, literal in re.findall(r"\\x([0-9a-fA-F]{2})(?:-\\x([0-9a-fA-F]{2}))?|(.)", classes[0]):
        if literal:
            codes.add(ord(literal))
        else:
            codes.update(range(int(low, 16), int(high or low, 16) + 1))
    return codes


class RuleCharacters(unittest.TestCase):
    def test_required_characters_are_the_installer_rule(self):
        rule = rule_characters()
        self.assertIn(0, rule)
        required = getattr(native, "NAME_RULE_CHARACTERS", None)
        if required is None or len(required) != len(set(required)) or set(required) != rule:
            self.fail("[violation:name-rule-characters-not-required] native checks require "
                      f"{sorted(required) if required is not None else None}, the installer rule refuses {sorted(rule)}")


class ReportValidation(unittest.TestCase):
    def report(self, refused):
        checks = list(dict.fromkeys([*native.PACKAGE_CHECKS, CASE]))
        return {"schema": "jaekit-native-windows-package/v1", "status": "passed", "shell_version": "5.1.1",
                "package_checks": checks, "name_character_refusals": refused, "failures": [],
                "server_rejection": {"status": "not-applicable"}}

    def test_package_report_requires_every_character_refusal(self):
        rule = sorted(rule_characters())
        if CASE not in native.PACKAGE_CHECKS:
            self.fail(f"[violation:incomplete-name-refusal-accepted] {CASE} is not a required package check")
        native.validate_package_report(self.report(rule), "5.1.1", False)
        incomplete = {f"without 0x{code:02x}": [other for other in rule if other != code] for code in rule}
        incomplete.update({"absent": None, "empty": [], "duplicated": rule + rule[:1], "as text": [str(code) for code in rule]})
        for label, refused in incomplete.items():
            report = self.report(refused)
            if refused is None:
                del report["name_character_refusals"]
            with self.subTest(case=label):
                try:
                    native.validate_package_report(report, "5.1.1", False)
                except RuntimeError:
                    continue
                self.fail(f"[violation:incomplete-name-refusal-accepted] report {label} was accepted")


class ContinuousIntegration(unittest.TestCase):
    def test_both_ci_shells_run_the_name_character_cases(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        job = workflow[workflow.index("\n  windows:\n"):]
        job = job[:re.search(r"\n  [a-z][a-z0-9_-]*:\n", job[1:]).start() + 1]
        steps = re.findall(r"\n      - name: [^\n]*\n(?:        [^\n]*\n)*", job)
        shells = {re.search(r"\n        shell: (\S+)", step).group(1) for step in steps
                  if "run: ./tools/windows/native_checks.ps1 " in step and re.search(r"\n        shell: ", step)}
        launcher = (ROOT / "tools/windows/native_checks.py").read_text(encoding="utf-8")
        script = (ROOT / "tools/windows/native_package_checks.ps1").read_text(encoding="utf-8")
        missing = [name for name, present in (
            ("Windows PowerShell 5.1 step", "powershell" in shells),
            ("PowerShell 7 step", "pwsh" in shells),
            ("launcher passes the rule characters", '"-NameCharacterCodes"' in launcher),
            ("package checks take the rule characters", "$NameCharacterCodes" in script),
            ("package checks report the case", f"'{CASE}'" in script)) if not present]
        if missing:
            self.fail(f"[violation:name-refusals-not-run-in-ci] missing: {missing}")


if __name__ == "__main__":
    unittest.main()
