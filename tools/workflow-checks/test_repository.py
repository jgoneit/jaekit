"""Offline checks for shared repository guidance and CI wiring."""
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]


def read(path):
    return (ROOT / path).read_text(encoding="utf-8")


def block(text, key, indent=0):
    """Read one block in this repository's indentation-based CI configuration."""
    lines = text.splitlines()
    start = next((index for index, line in enumerate(lines)
                  if line == " " * indent + key + ":"), None)
    if start is None:
        raise AssertionError(f"CI is missing {key!r} at indentation {indent}")
    result = []
    for line in lines[start + 1:]:
        if line.strip() and len(line) - len(line.lstrip()) <= indent:
            break
        result.append(line)
    return "\n".join(result)


class Guidance(unittest.TestCase):
    def test_goals_stay_local_without_automatic_backup(self):
        instructions = read("AGENTS.md").lower()
        self.assertRegex(instructions, r"root `docs/`[^\n]*local[^\n]*ignored")
        self.assertRegex(instructions, r"`docs/`[^\n]*plain folder[^\n]*nested git repository[^\n]*symlink")
        self.assertRegex(instructions, r"do not (?:create|set up) a (?:separate )?backup repository")
        self.assertRegex(instructions, r"(?:no|do not)[^\n]*automatic(?:ally)?[^\n]*(?:sync|transmit)")
        self.assertNotIn("back up private goals to a separate repository", instructions)
        self.assertIn("never force-add", instructions)

    def test_goal_ignore_and_claude_reference_are_preserved(self):
        self.assertIn("/docs/", read(".gitignore").splitlines())
        self.assertEqual(read("CLAUDE.md").strip(), "@AGENTS.md")

    def test_default_validation_is_documented_before_publication(self):
        validation = read("AGENTS.md").split("## Validation", 1)[1]
        commands = re.findall(r"```bash\n(.*?)```", validation, flags=re.S)
        self.assertEqual([command.strip() for command in commands], ["python3 tools/verify.py"])
        self.assertRegex(validation.lower(), r"before[^\n]*(?:push|publish)")


class Integration(unittest.TestCase):
    def test_common_checks_and_native_checks_are_connected_once(self):
        workflow = read(".github/workflows/ci.yml")
        jobs = block(workflow, "jobs")
        self.assertEqual(set(re.findall(r"^  (\w+):$", jobs, flags=re.M)),
                         {"go", "macos", "windows_ps51", "windows_ps7", "core", "docs"})
        expected = {
            "go": ("Core / Ubuntu", "ubuntu-24.04", [
                "python3 tools/ci/describe_run.py --shell bash", "|",
                "python3 tools/verify.py go",
                'python3 tools/ci/native_unix.py --report "$RUNNER_TEMP/native-unix.json"']),
            "macos": ("Core / macOS", "macos-15", [
                "python3 tools/ci/describe_run.py --shell bash",
                'python3 tools/ci/native_unix.py --report "$RUNNER_TEMP/native-unix.json"']),
            "windows_ps51": ("Core / Windows PowerShell 5.1", "windows-2025", [
                "python tools/ci/describe_run.py --shell powershell",
                './tools/windows/native_checks.ps1 -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-ps51"']),
            "windows_ps7": ("Core / Windows PowerShell 7", "windows-2025", [
                "python tools/ci/describe_run.py --shell pwsh",
                './tools/windows/native_checks.ps1 -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-ps7"']),
            "core": ("Seal Core", "ubuntu-24.04", ["python3 tools/ci/require_success.py"]),
            "docs": ("Public documentation", "ubuntu-24.04", ["python3 tools/verify.py docs"]),
        }
        for key, (name, runner, commands) in expected.items():
            with self.subTest(job=key):
                job = block(jobs, key, 2)
                self.assertRegex(job, rf"(?m)^    name: {re.escape(name)}$")
                self.assertRegex(job, rf"(?m)^    runs-on: {re.escape(runner)}$")
                self.assertEqual(re.findall(r"^\s+run: (.+)$", job, flags=re.M),
                                 commands)
        for key, shell in (("windows_ps51", "powershell"), ("windows_ps7", "pwsh")):
            with self.subTest(job=key):
                windows = block(jobs, key, 2)
                self.assertEqual(re.findall(r"^\s+shell: (.+)$", windows, flags=re.M), [shell, shell])
                checkout = windows.split('      - name: Set up Go')[0]
                self.assertIn("GIT_CONFIG_COUNT: '1'", checkout)
                self.assertIn('GIT_CONFIG_KEY_0: core.autocrlf', checkout)
                self.assertIn("GIT_CONFIG_VALUE_0: 'false'", checkout)
                self.assertNotRegex(windows, r'(?m)^    env:')
                self.assertNotIn('GIT_CONFIG_', windows.split('      - name: Set up Go')[1])
                self.assertNotIn('git config --global', windows)
        self.assertEqual(workflow.count("python3 tools/verify.py go"), 1)
        self.assertEqual(workflow.count("python3 tools/verify.py docs"), 1)
        self.assertNotIn("python3 tools/verify.py all", workflow)

    def test_core_gate_runs_after_all_native_jobs_even_if_one_fails(self):
        jobs = block(read(".github/workflows/ci.yml"), "jobs")
        core = block(jobs, "core", 2)
        self.assertRegex(core, r"(?m)^    needs: \[go, macos, windows_ps51, windows_ps7\]$")
        self.assertRegex(core, r"(?m)^    if: \$\{\{ always\(\) \}\}$")
        self.assertRegex(core, r"(?m)^          JAEKIT_CI_NEEDS: \$\{\{ toJSON\(needs\) \}\}$")
        self.assertEqual(len(re.findall(r"^\s+if:", core, flags=re.M)), 1)
        for key in ("go", "macos", "windows_ps51", "windows_ps7", "docs"):
            job = block(jobs, key, 2)
            self.assertNotRegex(job, r"(?m)^    (?:if|needs):")

    def test_release_candidate_gate_requires_both_release_jobs_from_the_run(self):
        jobs = block(read(".github/workflows/release.yml"), "jobs")
        gate = block(jobs, "candidate", 2)
        self.assertRegex(gate, r"(?m)^    name: Release candidate$")
        self.assertRegex(gate, r"(?m)^    needs: \[build, windows\]$")
        self.assertRegex(gate, r"(?m)^    if: \$\{\{ always\(\) \}\}$")
        self.assertIn("BUILD: ${{ needs.build.result }}", gate)
        self.assertIn("WINDOWS: ${{ needs.windows.result }}", gate)
        self.assertIn('if [ "${BUILD}" != success ] || [ "${WINDOWS}" != success ]; then', gate)
        for key in ("build", "windows"):
            self.assertNotRegex(block(jobs, key, 2), r"(?m)^\s+continue-on-error:")
        events = block(read(".github/workflows/release.yml"), "on")
        # A path filter would leave the required check missing on other pull requests.
        self.assertRegex(events, r"(?m)^  pull_request:$")
        self.assertNotIn("paths", events)

    def test_required_jobs_remain_unconditional_and_use_pinned_actions(self):
        workflow = read(".github/workflows/ci.yml")
        events = block(workflow, "on")
        self.assertEqual(set(re.findall(r"^  (\w+):$", events, flags=re.M)),
                         {"pull_request", "push"})
        self.assertRegex(events, r"(?m)^  pull_request:$")
        self.assertRegex(block(events, "push", 2), r"(?m)^      - main$")
        self.assertEqual(block(workflow, "permissions").strip(), "contents: read")
        # Only the aggregate job uses a condition, to run after unsuccessful needs.
        # Each Windows shell has its own job, so no step needs a condition.
        # Worker skips and continue-on-error must never turn failure into green.
        self.assertEqual(len(re.findall(r"^    if:", workflow, flags=re.M)), 1)
        self.assertEqual(len(re.findall(r"^        if:", workflow, flags=re.M)), 0)
        self.assertEqual(len(re.findall(r"^\s+needs:", workflow, flags=re.M)), 1)
        self.assertNotRegex(workflow, r"(?m)^\s+(?:continue-on-error|paths|paths-ignore|branches-ignore):")
        self.assertEqual(len(re.findall(r"^\s*permissions:", workflow, flags=re.M)), 1)
        actions = re.findall(r"^\s+uses: ([^\s#]+)", workflow, flags=re.M)
        action_names = [action.split("@", 1)[0] for action in actions]
        self.assertEqual(action_names.count("actions/checkout"), 6)
        self.assertEqual(action_names.count("actions/setup-go"), 4)
        self.assertEqual(action_names.count("actions/setup-python"), 2)
        self.assertEqual(set(action_names), {"actions/checkout", "actions/setup-go", "actions/setup-python"})
        for action in actions:
            self.assertRegex(action, r"@[0-9a-f]{40}$")
        self.assertEqual(len(re.findall(r"^\s+go-version-file: go\.mod$", workflow, flags=re.M)), 4)
        self.assertEqual(len(re.findall(r"^\s+persist-credentials: false$", workflow, flags=re.M)), 6)


if __name__ == "__main__":
    unittest.main()
