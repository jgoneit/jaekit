"""The Windows CI layout, its aggregate and the readers of its reports.

Every observation reads the checked tree only. The workflow is read as text in
this repository's indentation-based layout; the aggregate and the readers run
in this process or as child processes on synthetic inputs. No GitHub request is
made: runs and job logs are synthetic, and the readers' `gh` and `git` calls are
answered by a stand-in, as tools/workflow-checks/test_ci_observer.py does.
Violations come only from explicit `[violation:...]` failures. A module that
cannot be loaded, a reader that raises something other than its own
`Violation`, or a tool that cannot start raises Unavailable, an execution error.
"""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
CI = ".github/workflows/ci.yml"
NATIVE = "./tools/windows/native_checks.ps1"
# Planned job names, used only to build a two-job run when the checked
# workflow has fewer than two Windows jobs (PLAN 판단 근거).
PLANNED = {"powershell": "Core / Windows PowerShell 5.1", "pwsh": "Core / Windows PowerShell 7"}
VERSIONS = {"powershell": "5.1.26100.1", "pwsh": "7.5.3"}
PREPARATION = ("actions/checkout@", "actions/setup-go@", "actions/setup-python@", "tools/ci/describe_run.py")
SOURCE, HEAD, BASE, TREE = "a" * 40, "b" * 40, "c" * 40, "d" * 40
# Files that must stay as they are (AC-6), by content at the goal's base commit.
UNCHANGED = {
    ".github/workflows/release.yml": "702ded1163bf98f156a31e99868140a6080eb04b0804af28e5d229eab4ea6b46",
    "tools/windows/native_checks.ps1": "409693b3585fba47bb9401513b0b279d5d29c153ab8e7a759b54eecf1c3eb029",
}
# The CI jobs AC-6 keeps, by the sha256 of their whole block at the base commit,
# multi-line run blocks included.
UNCHANGED_JOBS = {
    "go": "af96675d567bc4ca1144bb414f0fb657ce0ec51a64d3e03d6cb34092c42d0510",
    "macos": "b53160f568bcf63ff1c23aa3733cdca04b498e8423679c40b669d96de1c32c02",
    "docs": "cce3e44596601f637dc5ff3f6a7fd4fb0aa2084dc49c3cb6c7065a7523671e6e",
}
# What a recorded report carried at the base commit (cross_platform_ci.py there).
# Synthetic reports are built from these, not from the reader under test, so a
# reader that starts requiring more of recorded reports is caught (AC-4).
RECORDED = {
    "scenarios": {"TestNativeAdmissionFailureHasNoOutputDigest", "TestNativeUnconfirmedShutdownHasNoOutputDigest",
                  "TestNativeArgvEnvironmentAndOutcomes", "TestNativeTimeoutStopsDescendantWrites",
                  "TestNativeConcurrentProcessesPreserveRecords", "TestNativeLockIsReleasedWhenOwnerExits",
                  "TestNativeCleanCRLFStillRequiresCommittedBytes", "TestNativeCoreGitLifecycle"},
    "windows": {"TestNativeJobLayout", "TestNativeBaselineRejectsJunction", "TestNativeWindowsEvidencePaths"},
    "package_checks": {"candidate-manifest-sha256", "candidate-native-version-capabilities",
                       "space-unicode-drive-path", "checksum-refusal", "archive-traversal-refusal",
                       "archive-case-collision-refusal", "archive-file-directory-collision-refusal",
                       "archive-reserved-name-refusal", "archive-stream-name-refusal", "junction-path-refusal"},
    "install": {"fresh", "reinstall", "checksum_rejection_preserves", "conflict_rejection_preserves",
                "update", "old_data_cleanup", "uninstall_preserves"},
}


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def read(path):
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"{path}: {error}") from None


def load(name, path):
    try:
        spec = importlib.util.spec_from_file_location(name, ROOT / path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    except Exception as error:
        raise Unavailable(f"{path} cannot be loaded: {error!r}") from None
    return module


def block(text, key, indent=0):
    lines = text.splitlines()
    start = next((i for i, line in enumerate(lines) if line == " " * indent + key + ":"), None)
    if start is None:
        return None
    body = []
    for line in lines[start + 1:]:
        if line.strip() and len(line) - len(line.lstrip()) <= indent:
            break
        body.append(line)
    return "\n".join(body)


def jobs(text):
    """{key: body} for every job of the workflow."""
    body = block(text, "jobs")
    if body is None:
        raise Unavailable("the CI workflow has no jobs block")
    return {key: block(body, key, 2) for key in re.findall(r"(?m)^  ([A-Za-z0-9_-]+):$", body)}


def steps(body):
    return re.findall(r"(?ms)^      - name: [^\n]+\n.*?(?=^      - name:|\Z)", body)


def native_steps(body):
    """(shell, run line) of each step that runs the native Windows entrypoint."""
    found = []
    for step in steps(body):
        run = re.search(r"(?m)^        run: (.+)$", step)
        if run and NATIVE in run[1]:
            shell = re.search(r"(?m)^        shell: (\S+)$", step)
            found.append((shell[1] if shell else None, run[1], step))
    return found


def windows_jobs(text):
    """{key: (name, body)} of the jobs that run on Windows."""
    return {key: (re.search(r"(?m)^    name: (.+)$", body)[1] if re.search(r"(?m)^    name: (.+)$", body) else key, body)
            for key, body in jobs(text).items() if re.search(r"(?m)^    runs-on: windows-", body)}


def shell_jobs(text):
    """{shell: (key, name)} when each Windows job runs the native entrypoint in exactly one shell."""
    found = {}
    for key, (name, body) in windows_jobs(text).items():
        shells = {shell for shell, _, _ in native_steps(body)}
        if len(shells) == 1:
            found.setdefault(shells.pop(), []).append((key, name))
    return {shell: rows[0] for shell, rows in found.items() if len(rows) == 1}


# --- AC-1 -----------------------------------------------------------------

class Layout(unittest.TestCase):
    """AC-1: each PowerShell runs the full native suite in its own job, without waiting."""

    def test_each_powershell_runs_the_native_suite_in_its_own_job(self):
        text = read(CI)
        windows = windows_jobs(text)
        problems = []
        if len(windows) != 2:
            problems.append(f"{len(windows)} Windows job(s), not two")
        shells = []
        for key, (name, body) in windows.items():
            native = native_steps(body)
            if len(native) != 1:
                problems.append(f"{key}: {len(native)} native suite steps, not one")
                continue
            shell, run, step = native[0]
            shells.append(shell)
            if run != f'{NATIVE} -OutputDirectory "$env:RUNNER_TEMP/jaekit-windows-{ {"powershell": "ps51", "pwsh": "ps7"}.get(shell, "?") }"':
                problems.append(f"{key}: native suite is not the complete developer entrypoint with its own output: {run}")
            before = body.split(step, 1)[0]
            missing = [prep for prep in PREPARATION if prep not in before]
            if missing:
                problems.append(f"{key}: preparation before the native suite lacks {missing}")
            if re.search(r"(?m)^    (?:needs|if|concurrency):", body) or re.search(r"(?m)^        if:", step):
                problems.append(f"{key}: waits on another job or conditions its native suite")
            if "continue-on-error" in body:
                problems.append(f"{key}: continue-on-error")
        if sorted(filter(None, shells)) != ["powershell", "pwsh"]:
            problems.append(f"native suite shells {shells}, not one job each for powershell and pwsh")
        require(self, not problems, "windows-shells-share-a-job", "; ".join(problems))


# --- AC-2 -----------------------------------------------------------------

def aggregate(needs):
    """Exit code of tools/ci/require_success.py for a synthetic needs context."""
    env = dict(os.environ, JAEKIT_CI_NEEDS=json.dumps(needs))
    try:
        result = subprocess.run([sys.executable, "tools/ci/require_success.py"], cwd=ROOT, env=env,
                                capture_output=True, text=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unavailable(f"require_success.py: {error}") from None
    passed = result.returncode == 0 and result.stdout.startswith("PASS: Seal Core")
    rejected = result.returncode == 1 and result.stderr.startswith("FAIL: Seal Core:")
    if not (passed or rejected):
        # A crash or a syntax error also exits 1; only the gate's own lines count.
        raise Unavailable(f"require_success.py exited {result.returncode}: {result.stderr[-1000:]}")
    return result.returncode


class Aggregate(unittest.TestCase):
    """AC-2: Seal Core succeeds only when Ubuntu, macOS and both shell jobs of the run succeed."""

    def test_seal_core_requires_every_native_job_including_both_shells(self):
        text = read(CI)
        shells = shell_jobs(text)
        core = jobs(text).get("core")
        if core is None:
            raise Unavailable("the CI workflow has no core job")
        problems = []
        if set(shells) != {"powershell", "pwsh"}:
            problems.append(f"shell jobs {sorted(shells)}: the two shells do not have their own jobs")
        required = {"go", "macos", *(key for key, _ in shells.values())}
        needs = re.search(r"(?m)^    needs: \[([^\]]*)\]$", core)
        listed = {item.strip() for item in needs[1].split(",")} if needs else set()
        if listed != required:
            problems.append(f"core needs {sorted(listed)}, not {sorted(required)}")
        if not re.search(r"(?m)^    if: \$\{\{ always\(\) \}\}$", core):
            problems.append("core does not run after unsuccessful needs")
        success = {key: {"result": "success"} for key in required}
        if aggregate(success) != 0:
            problems.append("all native jobs succeeded but the aggregate failed")
        for key in sorted(required):
            for result in ("failure", "cancelled", "skipped"):
                if aggregate(dict(success, **{key: {"result": result}})) != 1:
                    problems.append(f"aggregate accepted {key} {result}")
            if aggregate({k: v for k, v in success.items() if k != key}) != 1:
                problems.append(f"aggregate accepted a missing {key}")
        if aggregate(dict(success, unexpected={"result": "success"})) != 1:
            problems.append("aggregate accepted an unexpected job result")
        require(self, not problems, "seal-core-misses-a-shell-job", "; ".join(problems))


# --- AC-3, AC-4 -------------------------------------------------------------

def report(shell_version, windows=True, source=SOURCE):
    names = RECORDED["scenarios"] | {"TestNativeCoreGitLifecycle/linked=false", "TestNativeCoreGitLifecycle/linked=true"}
    if windows:
        names |= RECORDED["windows"]
    row = {"schema": "jaekit-native-windows/v1" if windows else "jaekit-native-unix/v1", "status": "passed",
           "source_commit": source,
           "environment": {"shell_version": shell_version, "goarch": "amd64",
                           "goos": "windows" if windows else "linux", "installation_type": "Server"},
           "native_tests": {"required": sorted(names), "passed": sorted(names), "skipped": []}}
    if windows:
        row.update(package_checks=sorted(RECORDED["package_checks"]),
                   candidate={"source_commit": source, "capabilities": {"synthetic": True},
                              "binary_sha256": "digest", "archive_sha256": "digest"},
                   server_rejection={"install": {"rejected": True, "preserved": True},
                                     "uninstall": {"rejected": True, "preserved": True}})
    else:
        row.update(candidates={"current": {"version": "synthetic", "capabilities": {"synthetic": True},
                                           "binary_sha256": "digest"}},
                   installations=[{"guide": guide, "checks": sorted(RECORDED["install"]), "binary_sha256": "digest",
                                   "installed_version": "synthetic"}
                                  for guide in ("guides/INSTALL.md", "guides/INSTALL.en.md")])
    return row


class SyntheticRun:
    """One synthetic pull request run, answering the readers' `gh` and `git` calls."""

    def __init__(self, windows):
        """windows: {job name: [shell versions of its reports]}."""
        self.reports = {"Core / Ubuntu": [report("5.2", windows=False)],
                        "Core / macOS": [report("3.2", windows=False)]}
        self.reports["Core / macOS"][0]["environment"]["goos"] = "darwin"
        self.os = {"Core / Ubuntu": "Linux", "Core / macOS": "macOS"}
        for name, versions in windows.items():
            self.reports[name] = [report(version) for version in versions]
            self.os[name] = "Windows"
        self.sources = {}
        names = sorted(set(self.reports) | {"Seal Core", "Public documentation"})
        self.jobs = [{"id": index, "name": name, "status": "completed", "conclusion": "success",
                      "steps": [{"status": "completed", "conclusion": "success"}]}
                     for index, name in enumerate(names)]
        self.run = {"id": 7, "run_attempt": 1, "head_sha": HEAD, "event": "pull_request", "status": "completed",
                    "conclusion": "success", "html_url": "https://example.invalid/run/7"}

    def api(self, path):
        if path.endswith("/pulls/1"):
            return {"head": {"sha": HEAD}, "base": {"sha": BASE}, "state": "open"}
        if "/ci.yml/runs?" in path:
            return {"workflow_runs": [dict(self.run)]}
        if "/jobs?" in path:
            return {"total_count": len(self.jobs), "jobs": json.loads(json.dumps(self.jobs))}
        if "/git/commits/" in path:
            return {"parents": [{"sha": HEAD}, {"sha": BASE}], "tree": {"sha": TREE}}
        raise Unavailable("unexpected GitHub path " + path)

    def command(self, argv, **kwargs):
        if argv == ["gh", "api", "--help"]:
            return "FLAGS\n  --allow-escape-sequences  Allow escape sequences in API output\n"
        if argv[:3] == ["gh", "repo", "view"]:
            return json.dumps({"nameWithOwner": "jgoneit/jaekit"})
        if argv[:3] == ["gh", "pr", "view"]:
            return json.dumps({"number": 1, "headRefOid": HEAD})
        if argv == ["git", "rev-parse", "HEAD"]:
            return HEAD
        if argv == ["git", "rev-parse", "HEAD^{tree}"]:
            return TREE
        if argv == ["git", "status", "--porcelain"]:
            return ""
        if argv[:4] == ["gh", "api", "--method", "GET"]:
            job = next(j for j in self.jobs if j["id"] == int(argv[-1].split("/")[-2]))
            context = {"source_sha": self.sources.get(job["name"], SOURCE), "head_sha": HEAD, "base_sha": BASE,
                       "event": "pull_request",
                       "runner_os": self.os.get(job["name"], "Linux"), "os": "synthetic", "arch": "synthetic",
                       "shell": "synthetic"}
            return "JAEKIT_CI_CONTEXT " + json.dumps(context) + "\n" + "\n".join(
                "JAEKIT_CI_REPORT " + json.dumps(r) for r in self.reports.get(job["name"], []))
        raise Unavailable("unexpected command " + " ".join(argv))


class Readers(unittest.TestCase):
    """AC-3 and AC-4: the CI report readers on synthetic runs of both layouts."""

    ci = None

    @classmethod
    def setUpClass(cls):
        Readers.ci = load("observed_cross_platform_ci", "tools/verification-checks/cross_platform_ci.py")
        cls.validation = load("observed_windows_validation", "tools/verification-checks/windows_validation.py")

    def read(self, run):
        """'pass' or 'rejected' for cross_platform_ci and windows_validation on one synthetic run."""
        outcomes = {}
        with patch.object(self.ci, "api", run.api), patch.object(self.ci, "command", run.command):
            try:
                self.ci.live_run()
                outcomes["cross_platform_ci"] = "pass"
            except self.ci.Violation:
                outcomes["cross_platform_ci"] = "rejected"
            except Unavailable:
                raise
            except Exception as error:
                raise Unavailable(f"cross_platform_ci raised {error!r}") from None
            try:
                reports = self.validation.live_reports(observer=self.ci)
                outcomes["windows_validation"] = "pass" if len(reports) == 2 else "rejected"
            except self.validation.Violation:
                outcomes["windows_validation"] = "rejected"
            except Unavailable:
                raise
            except Exception as error:
                raise Unavailable(f"windows_validation raised {error!r}") from None
        return outcomes

    def two_job_names(self):
        shells = shell_jobs(read(CI))
        if set(shells) == {"powershell", "pwsh"}:
            return {shell: name for shell, (_, name) in shells.items()}
        return dict(PLANNED)

    def test_two_job_runs_read_one_report_of_each_shell(self):
        names = self.two_job_names()
        five, seven = names["powershell"], names["pwsh"]
        good = self.read(SyntheticRun({five: [VERSIONS["powershell"]], seven: [VERSIONS["pwsh"]]}))
        bad = {
            "a shell's report is missing": SyntheticRun({five: [], seven: [VERSIONS["pwsh"]]}),
            "a shell's report appears twice": SyntheticRun({five: [VERSIONS["powershell"]] * 2, seven: [VERSIONS["pwsh"]]}),
            "the same shell in both jobs": SyntheticRun({five: [VERSIONS["pwsh"]], seven: [VERSIONS["pwsh"]]}),
        }
        other = SyntheticRun({five: [VERSIONS["powershell"]], seven: [VERSIONS["pwsh"]]})
        other.reports[seven][0]["source_commit"] = "e" * 40
        bad["a report from another commit"] = other
        bad["each shell's report in the other shell's job"] = SyntheticRun({five: [VERSIONS["pwsh"]],
                                                                            seven: [VERSIONS["powershell"]]})
        apart = SyntheticRun({five: [VERSIONS["powershell"]], seven: [VERSIONS["pwsh"]]})
        apart.sources[seven] = "e" * 40
        apart.reports[seven][0]["source_commit"] = "e" * 40
        bad["one shell job tested another source"] = apart
        bad["both layouts in one run"] = SyntheticRun({"Core / Windows": [VERSIONS["powershell"], VERSIONS["pwsh"]],
                                                       five: [VERSIONS["powershell"]], seven: [VERSIONS["pwsh"]]})
        accepted = {case: outcomes for case, run in bad.items()
                    if "pass" in (outcomes := self.read(run)).values()}
        require(self, set(good.values()) == {"pass"} and not accepted, "two-job-reports-not-read",
                f"valid two-job run: {good}; accepted invalid runs: {accepted}")

    def test_one_job_runs_keep_their_judgments(self):
        name = "Core / Windows"
        five, seven = VERSIONS["powershell"], VERSIONS["pwsh"]
        # Judgments of the base readers (cross_platform_ci.py:188,194-196 there):
        # exactly two reports, one of each shell, in either order.
        cases = {"both shells": ([five, seven], "pass"), "both shells, 7 first": ([seven, five], "pass"),
                 "the same shell twice": ([seven, seven], "rejected"), "one report": ([seven], "rejected"),
                 "a third report": ([five, seven, five], "rejected")}
        judged = {case: self.read(SyntheticRun({name: versions})) for case, (versions, _) in cases.items()}
        expected = {case: {"cross_platform_ci": outcome, "windows_validation": outcome}
                    for case, (_, outcome) in cases.items()}
        self.assertEqual(judged, expected, "a recorded one-job run is judged differently")


# --- AC-5 -----------------------------------------------------------------

# Runs one test file: 0 passed, 1 only assertion failures, 2 errors, 3 nothing collected.
RUNNER = r'''
import sys, unittest
loader = unittest.TestLoader()
suite = loader.discover(sys.argv[1], pattern=sys.argv[2])
if loader.errors or not suite.countTestCases():
    sys.exit(3)
result = unittest.TextTestRunner(verbosity=2).run(suite)
sys.exit(0 if result.wasSuccessful() else 2 if result.errors else 1)
'''


def workflow_tests(root):
    """0 (passed) or 1 (an assertion failed) for each workflow regression that fixes the CI layout, run in root.

    An import, fixture or test error is not a rejection: it raises Unavailable.
    """
    codes = {}
    for name in ("test_repository.py", "test_windows_shell_conditions.py"):
        try:
            result = subprocess.run([sys.executable, "-c", RUNNER, "tools/workflow-checks", name], cwd=root,
                                    capture_output=True, text=True, timeout=300,
                                    env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1"))
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"{name}: {error}") from None
        if result.returncode not in (0, 1):
            raise Unavailable(f"{name} exited {result.returncode}: {result.stderr[-1000:]}")
        codes[name] = result.returncode
    return codes


def in_job(text, key, anchor, line):
    """text with line inserted after the first line of job key that matches anchor."""
    lines = text.split("\n")
    start = lines.index(f"  {key}:")
    for index in range(start + 1, len(lines)):
        if lines[index].startswith("  ") and not lines[index].startswith("   ") and lines[index].strip():
            break
        if re.fullmatch(anchor, lines[index]):
            return "\n".join(lines[:index + 1] + [line] + lines[index + 1:])
    raise Unavailable(f"{key}: no line matches {anchor}")


def layout_mutations(text, shells):
    """{case: workflow text} that each break the two-job layout in one way."""
    keys = {shell: key for shell, (key, _) in shells.items()}
    mutations = {}
    for shell, key in keys.items():
        other = next(k for s, k in keys.items() if s != shell)
        _, run, _ = native_steps(windows_jobs(text)[key][1])[0]
        for replacement in ("go build ./cmd/ha", f"Write-Output 'jaekit-windows-{shell}'"):
            mutations[f"{key} running {replacement!r}"] = text.replace(run, replacement, 1)
        mutations[f"{key} waiting on {other}"] = in_job(text, key, r"    name: .+", f"    needs: [{other}]")
        mutations[f"{key} sharing a concurrency group"] = in_job(text, key, r"    name: .+",
                                                                 "    concurrency: jaekit-windows-native")
        step = re.escape(f"        run: {run}")
        mutations[f"{key} continuing after a native failure"] = in_job(text, key, step, "        continue-on-error: true")
        mutations[f"{key} conditioning its native suite"] = in_job(text, key, step, "        if: ${{ always() }}")
    # Both shells back in one job: the second job's native step moves into the first.
    first, second = keys["powershell"], keys["pwsh"]
    bodies = jobs(text)
    step = native_steps(bodies[second])[0][2].rstrip("\n")
    merged = text.replace(f"  {second}:\n{bodies[second]}\n", "", 1)
    merged = merged.replace(f"  {first}:\n{bodies[first]}", f"  {first}:\n{bodies[first].rstrip()}\n\n{step}", 1)
    merged = re.sub(r"(?m)^(    needs: \[.*), " + re.escape(second) + r"\]$", r"\1]", merged)
    mutations["both shells in one job"] = merged
    return mutations


def tracked_copy(directory):
    try:
        names = subprocess.run(["git", "ls-files", "-z"], cwd=ROOT, capture_output=True, check=True,
                               timeout=60).stdout.decode("utf-8", "surrogateescape").split("\0")
    except (OSError, subprocess.SubprocessError) as error:
        raise Unavailable(f"git ls-files: {error}") from None
    copy = Path(directory) / "tree"
    for name in filter(None, names):
        source = ROOT / name
        if source.is_file() and not source.is_symlink():
            target = copy / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)
    return copy


class Regressions(unittest.TestCase):
    """AC-5: the workflow regressions fix the two-job layout and reject a build or subset in either job."""

    def test_workflow_regressions_fix_the_two_shell_jobs(self):
        text = read(CI)
        shells = shell_jobs(text)
        problems = []
        if set(shells) != {"powershell", "pwsh"}:
            problems.append(f"shell jobs {sorted(shells)}: the regressions pass a workflow without a job per shell")
        current = workflow_tests(ROOT)
        if any(current.values()):
            problems.append(f"the workflow regressions fail on the checked workflow: {current}")
        accepted = []
        if not problems:
            with tempfile.TemporaryDirectory(prefix="jaekit-workflow-mutation-") as directory:
                copy = tracked_copy(directory)
                for case, mutated in layout_mutations(text, shells).items():
                    if mutated == text:
                        raise Unavailable(f"mutation {case} did not change the workflow")
                    (copy / CI).write_text(mutated, encoding="utf-8")
                    if not any(workflow_tests(copy).values()):
                        accepted.append(case)
        problems += [f"the workflow regressions accept {case}" for case in accepted]
        try:
            routes = {r["id"]: r for r in json.loads(read("contracts/regressions.json"))["routes"]}
        except (ValueError, KeyError, TypeError) as error:
            raise Unavailable(f"contracts/regressions.json: {error!r}") from None
        for route in ("shared-command-failures", "shared-tool-preflight", "entry-name-ci-observation"):
            if routes.get(route, {}).get("status") != "protected":
                problems.append(f"route {route} is not protected")
        require(self, not problems, "workflow-regressions-miss-two-jobs", "; ".join(problems))


# --- AC-6 -----------------------------------------------------------------

class Unchanged(unittest.TestCase):
    """AC-6: the other jobs, the required check names and the developer entrypoint stay as they were."""

    EXPECTED = {
        "go": ("Core / Ubuntu", "ubuntu-24.04", [
            "python3 tools/ci/describe_run.py --shell bash", "|", "python3 tools/verify.py go",
            'python3 tools/ci/native_unix.py --report "$RUNNER_TEMP/native-unix.json"']),
        "macos": ("Core / macOS", "macos-15", [
            "python3 tools/ci/describe_run.py --shell bash",
            'python3 tools/ci/native_unix.py --report "$RUNNER_TEMP/native-unix.json"']),
        "core": ("Seal Core", "ubuntu-24.04", ["python3 tools/ci/require_success.py"]),
        "docs": ("Public documentation", "ubuntu-24.04", ["python3 tools/verify.py docs"]),
    }

    def test_other_jobs_required_names_and_entrypoint_are_unchanged(self):
        workflow = jobs(read(CI))
        differences = []
        for key, (name, runner, commands) in self.EXPECTED.items():
            body = workflow.get(key) or ""
            observed = (re.search(r"(?m)^    name: (.+)$", body), re.search(r"(?m)^    runs-on: (.+)$", body),
                        re.findall(r"(?m)^\s+run: (.+)$", body))
            if (observed[0] and observed[0][1], observed[1] and observed[1][1], observed[2]) != (name, runner, commands):
                differences.append(key)
        for key, digest in UNCHANGED_JOBS.items():
            if hashlib.sha256((workflow.get(key) or "").encode()).hexdigest() != digest:
                differences.append(f"{CI} job {key}")
        for path, digest in UNCHANGED.items():
            try:
                if hashlib.sha256((ROOT / path).read_bytes()).hexdigest() != digest:
                    differences.append(path)
            except OSError as error:
                raise Unavailable(f"{path}: {error}") from None
        if "name: Release candidate" not in read(".github/workflows/release.yml"):
            differences.append("Release candidate")
        self.assertEqual(differences, [], "changed: " + ", ".join(differences))


# --- AC-7 -----------------------------------------------------------------

def section(text, heading):
    lines = text.splitlines()
    try:
        start = lines.index(heading)
    except ValueError:
        return ""
    end = next((i for i in range(start + 1, len(lines)) if lines[i].startswith("## ")), len(lines))
    return "\n".join(lines[start:end])


class Documents(unittest.TestCase):
    """AC-7: AGENTS.md and both CI guides name the two Windows jobs that Seal Core requires."""

    def test_rules_and_guides_describe_the_two_windows_jobs(self):
        shells = shell_jobs(read(CI))
        names = [name for _, name in shells.values()] if set(shells) == {"powershell", "pwsh"} else list(PLANNED.values())
        missing = []
        agents = next((p for p in read("AGENTS.md").split("\n\n") if "`Seal Core` succeeds only when" in p), "")
        missing += [f"AGENTS.md Main requires: `{name}`" for name in names if f"`{name}`" not in agents]
        if "`Core / Windows` from" in agents:
            missing.append("AGENTS.md Main requires still names the single `Core / Windows` job")
        for path, heading in (("guides/CI.en.md", "## Required results"), ("guides/CI.md", "## 필수 결과")):
            body = section(read(path), heading)
            missing += [f"{path} {heading}: `{name}`" for name in names if f"`{name}`" not in body]
            if "| `Core / Windows` |" in body:
                missing.append(f"{path} {heading}: a single `Core / Windows` row")
            seal = next((line for line in body.splitlines() if line.startswith("| `Seal Core` |")), "")
            missing += [f"{path} Seal Core row: `{name}`" for name in names if f"`{name}`" not in seal]
        require(self, not missing, "ci-docs-name-one-windows-job", "\n".join(missing))


if __name__ == "__main__":
    unittest.main()
