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
    def test_each_required_ci_job_uses_the_shared_entrypoint(self):
        workflow = read(".github/workflows/ci.yml")
        jobs = block(workflow, "jobs")
        self.assertEqual(set(re.findall(r"^  (\w+):$", jobs, flags=re.M)), {"go", "docs"})
        for group, name in (("go", "Seal Core"), ("docs", "Public documentation")):
            with self.subTest(group=group):
                job = block(jobs, group, 2)
                self.assertRegex(job, rf"(?m)^    name: {re.escape(name)}$")
                self.assertEqual(re.findall(r"^\s+run: (.+)$", job, flags=re.M),
                                 [f"python3 tools/verify.py {group}"])

    def test_required_jobs_remain_unconditional_and_use_pinned_actions(self):
        workflow = read(".github/workflows/ci.yml")
        events = block(workflow, "on")
        self.assertRegex(events, r"(?m)^  pull_request:$")
        self.assertRegex(block(events, "push", 2), r"(?m)^      - main$")
        self.assertEqual(block(workflow, "permissions").strip(), "contents: read")
        # A skipped job may satisfy GitHub's required check. Keep both jobs
        # unconditional and never hide a failing step behind continue-on-error.
        self.assertNotRegex(workflow, r"(?m)^\s+(?:if|needs|continue-on-error|paths|paths-ignore|branches-ignore):")
        self.assertEqual(len(re.findall(r"^\s*permissions:", workflow, flags=re.M)), 1)
        actions = re.findall(r"^\s+uses: ([^\s#]+)", workflow, flags=re.M)
        self.assertEqual(len(actions), 3)
        self.assertEqual([action.split("@", 1)[0] for action in actions],
                         ["actions/checkout", "actions/setup-go", "actions/checkout"])
        for action in actions:
            self.assertRegex(action, r"@[0-9a-f]{40}$")
        self.assertRegex(workflow, r"(?m)^\s+go-version-file: go\.mod$")


if __name__ == "__main__":
    unittest.main()
