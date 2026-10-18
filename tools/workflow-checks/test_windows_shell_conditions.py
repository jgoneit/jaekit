"""Check that each Windows shell runs the complete native suite in its own job.

Each shell job prepares its own checkout, Go, Python and source description and
then runs the native developer entrypoint once. A shell job neither waits on
the other (through needs, a job condition or a shared concurrency group) nor
conditions its native step, so one shell's failure, skip or cancellation cannot
hide the other shell's result. This reads the workflow
text; it is not a claim that an intentionally failing CI ran.
"""
from pathlib import Path
import re
import subprocess
import sys
import unittest


ROOT = Path(__file__).resolve().parents[2]
PREPARATION = ("Check out repository", "Set up Go", "Set up Python", "Describe tested source and runner")
# Job key, job name, shell and output directory of each Windows shell job.
SHELL_JOBS = (("windows_ps51", "Core / Windows PowerShell 5.1", "powershell", "ps51", "PowerShell 5.1"),
              ("windows_ps7", "Core / Windows PowerShell 7", "pwsh", "ps7", "PowerShell 7"))


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def job_steps(workflow, key, name):
    match = re.search(r"(?ms)^  " + re.escape(key) + r":\n(.*?)(?=^  [a-z][a-z0-9_]*:|\Z)", workflow)
    require(match, "Windows job missing: " + key)
    body = match.group(1)
    require(re.search(r"(?m)^    name: " + re.escape(name) + "$", body), "Windows job identity changed: " + key)
    require(re.search(r"(?m)^    runs-on: windows-", body), key + " does not run on Windows")
    require(not re.search(r"(?m)^    (?:needs|if|concurrency):", body), key + " must not wait on another job")
    require("continue-on-error" not in body, "shell failure must remain a failure")
    require(not re.search(r"(?m)^        if:", body), key + ": a step condition could skip or hide the native suite")
    steps = {}
    for match in re.finditer(r"(?ms)^      - name: ([^\n]+)\n(.*?)(?=^      - name:|\Z)", body):
        name, source = match.groups()
        require(name not in steps, "duplicate Windows step")
        steps[name] = source
    return steps


def check_policy(workflow):
    require(len(re.findall(r"(?m)^    runs-on: windows-", workflow)) == len(SHELL_JOBS),
            "each shell needs exactly one Windows job")
    for key, name, shell, directory, title in SHELL_JOBS:
        steps = job_steps(workflow, key, name)
        native = "Verify native developer checks on " + title
        require(list(steps) == [*PREPARATION, native], key + ": steps must be the preparation, then one native suite")
        describe = steps["Describe tested source and runner"]
        require(re.search(r"(?m)^        shell: " + shell + "$", describe) and
                re.search(r"(?m)^        run: python tools/ci/describe_run\.py --shell " + shell + "$", describe),
                "wrong shell binding: " + key)
        require(re.search(r"(?m)^        shell: " + shell + "$", steps[native]), "wrong shell binding: " + key)
        command = './tools/windows/native_checks.ps1 -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-' + directory + '"'
        require(re.search(r"(?m)^        run: " + re.escape(command) + r"$", steps[native]),
                "both shells must invoke the complete native suite, including history")


class WindowsShellConditions(unittest.TestCase):
    def test_current_workflow(self):
        check_policy((ROOT / ".github/workflows/ci.yml").read_text())

    def test_a_shell_job_cannot_wait_on_hide_or_share_the_other(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        seven = "    name: Core / Windows PowerShell 7\n"
        native = "      - name: Verify native developer checks on PowerShell 7\n"
        mutations = {
            "waits on the other job": source.replace(seven, seven + "    needs: [windows_ps51]\n", 1),
            "job condition": source.replace(seven, seven + "    if: ${{ always() }}\n", 1),
            "shared concurrency group": source.replace(seven, seven + "    concurrency: jaekit-windows-native\n", 1),
            "step condition": source.replace(native, native + "        if: ${{ !cancelled() }}\n", 1),
            "continue-on-error": source.replace(native, native + "        continue-on-error: true\n", 1),
            "other shell": source.replace('pwsh\n        run: ./tools/windows/', 'powershell\n        run: ./tools/windows/', 1),
            "shared output": source.replace("jaekit-windows-ps7", "jaekit-windows-ps51", 1),
            "one job": re.sub(r"(?ms)^  windows_ps7:\n.*?(?=^  core:)", "", source, count=1),
        }
        for case, mutated in mutations.items():
            with self.subTest(case=case):
                self.assertNotEqual(mutated, source)
                with self.assertRaises(AssertionError):
                    check_policy(mutated)

    def test_either_shell_cannot_replace_native_history_with_a_build_or_subset(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        for shell in ("ps51", "ps7"):
            command = './tools/windows/native_checks.ps1 -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-' + shell + '"'
            for replacement in ("go build ./cmd/ha", "Write-Output 'jaekit-windows-" + shell + "'"):
                with self.subTest(shell=shell, replacement=replacement), self.assertRaises(AssertionError):
                    check_policy(source.replace(command, replacement))

    def test_optimized_python_still_rejects_a_waiting_shell_job(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        seven = "    name: Core / Windows PowerShell 7\n"
        source = source.replace(seven, seven + "    needs: [windows_ps51]\n", 1)
        result = subprocess.run(
            [sys.executable, "-O", "-c", "import sys; from test_windows_shell_conditions import check_policy; check_policy(sys.stdin.read())"],
            input=source, text=True, capture_output=True, cwd=Path(__file__).parent, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("windows_ps7 must not wait on another job", result.stderr)


if __name__ == "__main__":
    unittest.main()
