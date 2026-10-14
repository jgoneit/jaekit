"""Check the actual Windows step condition under success/failure/cancel states.

Only the small conjunction used by this workflow is accepted. This is a local
condition evaluation, not a claim that an intentionally failing CI ran.
"""
from pathlib import Path
import re
import subprocess
import sys
import unittest


ROOT = Path(__file__).resolve().parents[2]
PREPARATION = ("Check out repository", "Set up Go", "Set up Python", "Describe tested source and runner")


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def windows_steps(workflow):
    match = re.search(r"(?ms)^  windows:\n(.*?)(?=^  [a-z][a-z_]*:|\Z)", workflow)
    require(match, "Windows job missing")
    body = match.group(1)
    require("name: Core / Windows" in body, "Windows job identity changed")
    steps = {}
    for match in re.finditer(r"(?ms)^      - name: ([^\n]+)\n(.*?)(?=^      - name:|\Z)", body):
        name, source = match.groups()
        require(name not in steps, "duplicate Windows step")
        steps[name] = source
    return steps


def evaluate(expression, outcomes, cancelled):
    fields = expression.split("&&")
    values = []
    for field in fields:
        field = field.strip()
        if field == "!cancelled()":
            values.append(not cancelled)
            continue
        match = re.fullmatch(r"steps\.([a-z_]+)\.outcome == 'success'", field)
        require(match and match[1] in outcomes, "unsupported or unknown condition: " + field)
        values.append(outcomes[match[1]] == "success")
    require("!cancelled()" in [field.strip() for field in fields], "default success gate would hide PS7")
    return all(values)


def check_policy(workflow):
    steps = windows_steps(workflow)
    ids = []
    for name in PREPARATION:
        require(name in steps, "preparation step missing: " + name)
        match = re.search(r"(?m)^        id: ([a-z_]+)$", steps[name])
        require(match, "preparation outcome is not addressable: " + name)
        ids.append(match[1])
    require(len(set(ids)) == len(ids), "ambiguous preparation IDs")
    first = steps["Verify native developer checks on PowerShell 5.1"]
    second = steps["Verify native developer checks on PowerShell 7"]
    require("shell: powershell" in first and "shell: pwsh" in second, "wrong shell binding")
    require("jaekit-windows-ps51" in first and "jaekit-windows-ps7" in second, "shell output separation missing")
    for source, directory in ((first, "ps51"), (second, "ps7")):
        command = './tools/windows/native_checks.ps1 -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-' + directory + '"'
        require(re.search(r"(?m)^        run: " + re.escape(command) + r"$", source),
                "both shells must invoke the complete native suite, including history")
    require("continue-on-error" not in first + second, "shell failure must remain a failure")
    require(not re.search(r"(?m)^        if:", first), "PS5.1 must run after successful preparation")
    match = re.search(r"(?m)^        if: \$\{\{ (.+) \}\}$", second)
    require(match, "independent PS7 condition missing")
    expression = match[1]
    outcomes = {name: "success" for name in ids}
    require(evaluate(expression, outcomes, False), "prepared independent PS7 must run")
    require(not evaluate(expression, outcomes, True), "cancelled workflow must not start PS7")
    for name in ids:
        for failed in ("failure", "skipped", "cancelled", ""):
            require(not evaluate(expression, dict(outcomes, **{name: failed}), False), "PS7 ignores failed preparation: " + name)
    # There is intentionally no reference to a previous check's outcome. With
    # the explicit status function, GitHub's implicit success() is replaced.
    require(set(re.findall(r"steps\.([a-z_]+)\.outcome", expression)) == set(ids), "PS7 depends on a non-preparation step")


class WindowsShellConditions(unittest.TestCase):
    def test_current_workflow(self):
        check_policy((ROOT / ".github/workflows/ci.yml").read_text())

    def test_each_guard_is_required(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        for guard in re.findall(r" && steps\.[a-z_]+\.outcome == 'success'", source):
            with self.subTest(guard=guard), self.assertRaises(AssertionError):
                check_policy(source.replace(guard, "", 1))

    def test_default_success_does_not_collect_second_result(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        with self.assertRaises(AssertionError):
            check_policy(source.replace("!cancelled()", "success()"))

    def test_either_shell_cannot_replace_native_history_with_a_build_or_subset(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        for shell in ("ps51", "ps7"):
            command = './tools/windows/native_checks.ps1 -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-' + shell + '"'
            for replacement in ("go build ./cmd/ha", "Write-Output 'jaekit-windows-" + shell + "'"):
                with self.subTest(shell=shell, replacement=replacement), self.assertRaises(AssertionError):
                    check_policy(source.replace(command, replacement))

    def test_optimized_python_still_rejects_a_missing_preparation_guard(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        source = source.replace(" && steps.checkout.outcome == 'success'", "", 1)
        result = subprocess.run(
            [sys.executable, "-O", "-c", "import sys; from test_windows_shell_conditions import check_policy; check_policy(sys.stdin.read())"],
            input=source, text=True, capture_output=True, cwd=Path(__file__).parent, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("PS7 ignores failed preparation: checkout", result.stderr)


if __name__ == "__main__":
    unittest.main()
