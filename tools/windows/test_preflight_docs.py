"""Public example wiring checks; actual PowerShell cases run in Windows CI."""
from __future__ import annotations

import importlib.util
from pathlib import Path
import re
import unittest

spec = importlib.util.spec_from_file_location("preflight_docs", Path(__file__).with_name("preflight_docs.py"))
docs = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(docs)
ROOT = Path(__file__).resolve().parents[2]


class PreflightDocumentationTests(unittest.TestCase):
    def guides(self):
        return [(ROOT / path).read_text(encoding="utf-8")
                for path in ("guides/WINDOWS.md", "guides/WINDOWS.en.md")]

    def test_translations_execute_identical_preflight_and_action_blocks(self):
        korean, english = self.guides()
        for label in ("CANDIDATE PREFLIGHT", "VERIFIED INSTALL", "VERIFIED REMOVE"):
            with self.subTest(label=label):
                self.assertEqual(docs.block(korean, label), docs.block(english, label))
                docs.block(korean, label).encode("ascii")

    def test_preflight_does_not_invoke_candidate_bootstrap(self):
        code = docs.block(self.guides()[0], "CANDIDATE PREFLIGHT")
        self.assertTrue(code.startswith("function Get-JaekitVerifiedCandidate("))
        self.assertIn("Get-FileHash -LiteralPath $path -Algorithm SHA256", code)
        self.assertNotRegex(code, r"(?m)^\s*[.&]\s")
        self.assertNotIn("Invoke-Expression", code)
        self.assertNotRegex(code, r"(?i)\b(?:python|go|bash|curl|Invoke-WebRequest)\b")
        for name in ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1"):
            self.assertIn("'" + name + "'", code)

    def test_install_and_remove_bind_only_verified_candidate_in_one_block(self):
        for text in self.guides():
            install = docs.block(text, "VERIFIED INSTALL")
            remove = docs.block(text, "VERIFIED REMOVE")
            for code, action in ((install, "Install"), (remove, "Uninstall")):
                with self.subTest(action=action):
                    self.assertTrue(code.startswith("& {\n"))
                    self.assertTrue(code.endswith("\n}"))
                    self.assertLess(code.index("$candidate = Get-JaekitVerifiedCandidate"),
                                    code.index("& $candidate." + action))
            self.assertIn("-Archive $candidate.Archive -Checksums $candidate.Checksums -AddToPath", install)
            # No alternate unchecked install or installed-copy removal example.
            examples = "\n".join(re.findall(r"```powershell\n(.*?)\n```", text, re.DOTALL))
            self.assertNotRegex(examples, r"(?m)^\s*\.\\(?:install|uninstall|acceptance)\.ps1\b")
            self.assertNotIn("& (Join-Path $jaekitData 'tools\\windows\\uninstall.ps1')", examples)

    def test_acceptance_checks_both_candidates_before_entering_candidate_code(self):
        text = (ROOT / "tools/windows/README.md").read_text(encoding="utf-8")
        code = docs.block(text, "VERIFIED ACCEPTANCE")
        self.assertTrue(code.startswith("& {\n") and code.endswith("\n}"))
        entry = code.index("& $first.Acceptance")
        self.assertLess(code.index("$first = Get-JaekitVerifiedCandidate"), entry)
        self.assertLess(code.index("$next = Get-JaekitVerifiedCandidate"), entry)
        self.assertIn("-Archive $first.Archive -Checksums $first.Checksums", code)
        self.assertIn("-UpdateArchive $next.Archive -UpdateChecksums $next.Checksums -UpdateInstaller $next.Install -UpdateUninstaller $next.Uninstall -ConsentUserInstallation", code)
        self.assertNotRegex(text, r"(?m)^\s*\.\\acceptance\.ps1\b")

    def test_missing_duplicate_or_malformed_example_is_rejected(self):
        valid = "<!-- BEGIN WINDOWS X -->\n```powershell\nGet-Date\n```\n<!-- END WINDOWS X -->"
        self.assertEqual(docs.block(valid, "X"), "Get-Date")
        for text in ("", valid + valid, valid.replace("powershell", "text"),
                     valid.replace("Get-Date", ""), valid.replace("<!-- END WINDOWS X -->", ""),
                     "<!-- END WINDOWS X -->" + valid.split("<!-- END WINDOWS X -->")[0]):
            with self.subTest(text=text), self.assertRaises(ValueError):
                docs.block(text, "X")

    def test_developer_runtime_is_ps51_safe_source_not_candidate_bootstrap(self):
        runtime = (ROOT / "tools/windows/native_preflight_checks.ps1").read_bytes().decode("ascii")
        self.assertIn("Read-PublicBlock $path 'CANDIDATE PREFLIGHT'", runtime)
        self.assertIn("Read-PublicBlock $path 'VERIFIED INSTALL'", runtime)
        self.assertIn("Read-PublicBlock $path 'VERIFIED REMOVE'", runtime)
        self.assertIn("'README.md') 'VERIFIED ACCEPTANCE'", runtime)
        self.assertNotIn("-ExecutionPolicy", runtime)
        self.assertNotIn("Set-ExecutionPolicy", runtime)


if __name__ == "__main__":
    unittest.main()
