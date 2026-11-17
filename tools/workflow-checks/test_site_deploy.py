"""Check that Site deploy publishes only main's current commit.

GitHub's "Re-run failed jobs" reruns an older run's failed Site deploy with
that run's build. The deploy job therefore compares the commit its build
recorded with the commit refs/heads/main points to now, in the same job as the
publish step, and publishes only when they match.

Boundary: this reads the text of .github/workflows/pages.yml and observes
fixed step names, ids, conditions, environment lines and command lines. The
build job must record the checked-out commit as its `commit` output before it
builds; the deploy job's steps must be exactly the comparison and then the
publish step; the comparison must run unconditionally in bash, must not
continue on error, must exit 1 when the recorded commit is not 40 hex digits,
when `git ls-remote --exit-code` fails, or when the listing names no 40-hex
commit for refs/heads/main, and must write `deploy=true` only as its last line
after the branch for a different commit has exited; the publish step must run
only on `deploy == 'true'` with GitHub's implicit success(). It does not run
the workflow or bash, so it does not cover GitHub's handling of reruns and job
outputs, network behavior of git ls-remote, or bash semantics. Other YAML
spellings of the same workflow (quoted keys, flow style, other indentation)
fail the check rather than pass it. A missing workflow file is an error, not a
pass.
"""
from pathlib import Path
import re
import subprocess
import sys
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ".github/workflows/pages.yml"
CHECKOUT = "Check out the commit (on main, main's head)"
RECORD = "Record the built commit"
COMPARE = "Compare the built commit with main's current commit"
PUBLISH = "Publish to GitHub Pages"


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def job(workflow, key, name):
    match = re.search(r"(?ms)^  " + re.escape(key) + r":\n(.*?)(?=^  [a-z][a-z0-9_-]*:|\Z)", workflow)
    require(match, "pages.yml has no job " + key)
    body = match.group(1)
    require(re.search(r"(?m)^    name: " + re.escape(name) + "$", body), key + " is not named " + name)
    head, separator, steps = body.partition("\n    steps:\n")
    require(separator, key + " has no steps")
    found = []
    for match in re.finditer(r"(?ms)^      - name: ([^\n]+)\n(.*?)(?=^      - name:|\Z)", steps):
        found.append(match.groups())
    require(len({name for name, _ in found}) == len(found), key + " has duplicate step names")
    return head, found


def script(source):
    """Return the lines of a step's `run: |` block without their indentation."""
    _, separator, block = source.partition("\n        run: |\n")
    require(separator, "the step has no run block")
    lines = []
    for line in block.splitlines():
        if line and not line.startswith("          "):
            break
        lines.append(line[10:])
    while lines and not lines[-1]:
        lines.pop()
    return lines


def branch(lines, opening):
    """Return the stripped lines between an exact top-level `if ...; then` line and its `fi`."""
    require(lines.count(opening) == 1, "the comparison lacks exactly one line " + opening)
    start = lines.index(opening) + 1
    require("fi" in lines[start:], "the comparison does not close " + opening)
    return start, [line.strip() for line in lines[start:lines.index("fi", start)]]


def check_policy(workflow):
    build_head, build = job(workflow, "build", "Site build")
    require(re.search(r"(?m)^    outputs:\n      commit: \$\{\{ steps\.commit\.outputs\.sha \}\}$", build_head),
            "Site build does not output the commit it built")
    names = [name for name, _ in build]
    require(names[:2] == [CHECKOUT, RECORD], "Site build does not record the commit right after checking it out")
    record = build[1][1]
    require(re.search(r"(?m)^        id: commit$", record), "the record step is not id commit")
    require(not re.search(r"(?m)^        (?:if|continue-on-error):", record), "the record step can be skipped or fail silently")
    require(script(record) == ['sha="$(git rev-parse --verify HEAD)"', 'echo "sha=$sha" >> "$GITHUB_OUTPUT"'],
            "the record step does not record the checked-out commit")

    deploy_head, deploy = job(workflow, "deploy", "Site deploy")
    require(not re.search(r"(?m)^    continue-on-error:", deploy_head), "Site deploy continues on error")
    require([name for name, _ in deploy] == [COMPARE, PUBLISH],
            "Site deploy must compare the built commit with main before publishing, with no other step")
    compare, publish = deploy[0][1], deploy[1][1]
    require(re.search(r"(?m)^        id: current$", compare), "the comparison is not id current")
    require(re.search(r"(?m)^        shell: bash$", compare), "the comparison does not run in bash")
    require(not re.search(r"(?m)^        (?:if|continue-on-error):", compare),
            "the comparison can be skipped or its failure ignored")
    for line in ("          BUILT: ${{ needs.build.outputs.commit }}",
                 "          REPOSITORY: ${{ github.server_url }}/${{ github.repository }}.git"):
        require(line in compare.splitlines(), "the comparison lacks " + line.strip())

    lines = script(compare)
    for opening in ('if ! [[ "$BUILT" =~ ^[0-9a-f]{40}$ ]]; then',
                    'if ! listing="$(git ls-remote --exit-code "$REPOSITORY" refs/heads/main)"; then',
                    'if ! [[ "$current" =~ ^[0-9a-f]{40}$ ]]; then'):
        _, body = branch(lines, opening)
        require(body and body[-1] == "exit 1", "the comparison does not fail when " + opening + " holds")
    require(lines.count("current=\"$(awk '$2 == \"refs/heads/main\" { print $1 }' <<< \"$listing\")\"") == 1,
            "the comparison does not read main's commit from the listing")
    start, differ = branch(lines, 'if [ "$current" != "$BUILT" ]; then')
    require(differ[-2:] == ['echo "deploy=false" >> "$GITHUB_OUTPUT"', "exit 0"],
            "a different commit does not end the comparison without deploying")
    require('echo "$message" >> "$GITHUB_STEP_SUMMARY"' in differ, "a skipped deploy does not say why in the summary")
    deploys = [index for index, line in enumerate(lines) if "deploy=true" in line]
    require(deploys == [len(lines) - 1] and lines[-1] == 'echo "deploy=true" >> "$GITHUB_OUTPUT"' and start < deploys[0],
            "deploy=true is written other than as the last line after the comparison")

    require(re.search(r"(?m)^        id: deployment$", publish), "the publish step is not id deployment")
    require(re.findall(r"(?m)^        if: (.*)$", publish) == ["steps.current.outputs.deploy == 'true'"],
            "the publish step does not run only when the comparison allows it")
    require("continue-on-error" not in publish, "the publish step continues on error")
    require(re.search(r"(?m)^        uses: actions/deploy-pages@", publish), "the publish step does not deploy")


class SiteDeploy(unittest.TestCase):
    def source(self):
        return (ROOT / WORKFLOW).read_text(encoding="utf-8")

    def test_current_workflow(self):
        check_policy(self.source())

    def test_a_deploy_that_can_publish_an_older_build_is_rejected(self):
        source = self.source()
        compare = re.search(r"(?ms)^      - name: " + re.escape(COMPARE) + r"\n.*?(?=^      - name:)", source).group(0)
        publish = "      - name: " + PUBLISH + "\n"
        condition = "        if: steps.current.outputs.deploy == 'true'\n"
        listing = 'if ! listing="$(git ls-remote --exit-code "$REPOSITORY" refs/heads/main)"; then\n'
        failure = '            echo "::error::Site deploy stopped: cannot read main\'s current commit."\n            exit 1\n'
        mutations = {
            "comparison removed": source.replace(compare, "", 1),
            "comparison after publish": re.sub(r"(?ms)(^      - name: " + re.escape(PUBLISH) + r"\n.*)\Z",
                                               lambda m: m.group(1) + compare, source.replace(compare, "", 1), count=1),
            "step between comparison and publish": source.replace(publish, "      - name: Wait\n        run: sleep 1\n" + publish, 1),
            "publish unconditional": source.replace(condition, "", 1),
            "publish always": source.replace(condition, "        if: always() && steps.current.outputs.deploy == 'true'\n", 1),
            "publish on failure": source.replace(condition, condition + "        continue-on-error: true\n", 1),
            "comparison may fail": source.replace("        id: current\n", "        id: current\n        continue-on-error: true\n", 1),
            "comparison skipped": source.replace("        id: current\n", "        id: current\n        if: github.run_attempt == 1\n", 1),
            "comparison in sh": source.replace("        id: current\n        shell: bash\n", "        id: current\n        shell: sh\n", 1),
            "unreadable main passes": source.replace(listing + failure, listing + failure.replace("exit 1", "exit 0"), 1),
            "unreadable main ignored": source.replace("git ls-remote --exit-code", "git ls-remote", 1),
            "malformed main commit passes": source.replace(
                'if ! [[ "$current" =~ ^[0-9a-f]{40}$ ]]; then\n' + failure,
                'if ! [[ "$current" =~ ^[0-9a-f]{40}$ ]]; then\n' + failure.replace("exit 1", "true"), 1),
            "other repository": source.replace("${{ github.server_url }}/${{ github.repository }}.git",
                                               "https://github.com/example/site.git", 1),
            "different commit deploys": source.replace('echo "deploy=false" >> "$GITHUB_OUTPUT"',
                                                       'echo "deploy=true" >> "$GITHUB_OUTPUT"', 1),
            "deploy decided first": source.replace('          if ! [[ "$BUILT" =~',
                                                   '          echo "deploy=true" >> "$GITHUB_OUTPUT"\n          if ! [[ "$BUILT" =~', 1),
            "build commit not recorded": source.replace("    outputs:\n      commit: ${{ steps.commit.outputs.sha }}\n", "", 1),
            "build commit from the trigger": source.replace('sha="$(git rev-parse --verify HEAD)"', 'sha="$GITHUB_SHA"', 1),
        }
        for case, mutated in mutations.items():
            with self.subTest(case=case):
                self.assertNotEqual(mutated, source)
                with self.assertRaises(AssertionError):
                    check_policy(mutated)

    def test_an_empty_workflow_does_not_pass(self):
        with self.assertRaises(AssertionError):
            check_policy("")

    def test_optimized_python_still_rejects_a_removed_comparison(self):
        source = self.source()
        compare = re.search(r"(?ms)^      - name: " + re.escape(COMPARE) + r"\n.*?(?=^      - name:)", source).group(0)
        result = subprocess.run(
            [sys.executable, "-O", "-c", "import sys; from test_site_deploy import check_policy; check_policy(sys.stdin.read())"],
            input=source.replace(compare, "", 1), text=True, capture_output=True, cwd=Path(__file__).parent, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Site deploy must compare the built commit with main before publishing", result.stderr)


if __name__ == "__main__":
    unittest.main()
