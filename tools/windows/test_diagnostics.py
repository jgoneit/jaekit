"""Console-encoding regressions; these do not claim Windows runtime success."""
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location("windows_diagnostics", Path(__file__).with_name("native_checks.py"))
native = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(native)


class ConsoleDiagnosticsTests(unittest.TestCase):
    def test_real_strict_console_preserves_raw_logs_and_escapes_only_unsupported_text(self):
        raw = "caf\u00e9: \ud55c\uae00 \uacbd\ub85c\nfixture failure\n".encode("utf-8")
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            log = directory / "acceptance-runtime.stderr.log"
            log.write_bytes(raw)
            for encoding in ("cp1252", "utf-8"):
                with self.subTest(encoding=encoding):
                    buffer = io.BytesIO()
                    with io.TextIOWrapper(buffer, encoding=encoding, errors="strict") as console:
                        with mock.patch.object(native.sys, "stdout", console):
                            native.failure_tails(directory)
                        console.flush()
                        rendered = buffer.getvalue().decode(encoding)
                    self.assertIn("Captured public fixture log tail: acceptance-runtime.stderr.log", rendered)
                    self.assertIn("caf\u00e9", rendered)
                    self.assertIn("fixture failure", rendered)
                    expected = r"\ud55c\uae00 \uacbd\ub85c" if encoding == "cp1252" else "\ud55c\uae00 \uacbd\ub85c"
                    self.assertIn(expected, rendered)
                    self.assertEqual(raw, log.read_bytes())

    def test_main_still_emits_failed_report_after_cp1252_diagnostic(self):
        raw = "synthetic \ud55c\uae00 failure\n".encode("utf-8")
        original_tails = native.failure_tails

        def captured_failure(directory):
            # Model captured subprocess output without starting any subprocess
            # or impersonating a supported Windows verification environment.
            (directory / "acceptance-runtime.stderr.log").write_bytes(raw)
            original_tails(directory)

        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary).resolve() / "new-observation"
            argv = ["native_checks.py", "--output", str(output), "--shell-executable", "unused-fixture-shell",
                    "--shell-version", "5.1.1", "--shell-edition", "Desktop", "--installation-type", "Server",
                    "--os-build", "26100"]
            buffer = io.BytesIO()
            with io.TextIOWrapper(buffer, encoding="cp1252", errors="strict") as console:
                with mock.patch.object(native.sys, "stdout", console), mock.patch.object(native.sys, "argv", argv), \
                        mock.patch.object(native.platform, "machine", return_value="unsupported-fixture"), \
                        mock.patch.object(native, "checked", side_effect=AssertionError("Unexpected external command")), \
                        mock.patch.object(native, "failure_tails", side_effect=captured_failure):
                    code = native.main()
                console.flush()
                rendered = buffer.getvalue().decode("cp1252")
            self.assertEqual(1, code)
            self.assertIn(r"synthetic \ud55c\uae00 failure", rendered)
            prefix = "JAEKIT_CI_REPORT "
            reports = [json.loads(line[len(prefix):]) for line in rendered.splitlines() if line.startswith(prefix)]
            self.assertEqual(1, len(reports))
            self.assertEqual("failed", reports[0]["status"])
            self.assertTrue(reports[0]["failures"])
            self.assertEqual(json.loads((output / "summary.json").read_text(encoding="utf-8")), reports[0])
            self.assertEqual(raw, (output / "acceptance-runtime.stderr.log").read_bytes())


if __name__ == "__main__":
    unittest.main()
