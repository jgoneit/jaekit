"""What the verification groups run, and what the public copies observe.

Static observations call the group functions of tools/verify.py with `run`
replaced by a recorder; no step executes. Dynamic observations run the go-group
checks of the protected `current-file-independence` route, as the checked
tree's contracts/regressions.json lists them, with a process log:

- a `sitecustomize` module on PYTHONPATH records every Python process whose
  script, or else whose working directory, lies inside a public copy (a Git
  work tree, other than this checkout, whose tools/verify.py matches this
  checkout's), with its script, arguments and exit code;
- a `go` executable first on PATH records every Go command whose directory
  (`-C` or the working directory) lies inside a public copy.

A full Go suite (`go test` with `./...` and no -run/-list filter) and a
tools/verify.py run inside a copy are recorded and then replaced: the suite by a
successful no-op, a `docs` or `all` verification by the two public file and
documentation checks it starts with. A replaced `go` or `all` verification
counts as a full Go suite. Violations come only from explicit
`[violation:...]` failures. Missing tools, a route that cannot be read, checks
that cannot be collected or that fail for reasons other than the observed
property raise Unavailable, an execution error.
"""
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
ROUTE = "current-file-independence"
# The current-file judgments that AC-4 names, by their script in the copy.
JUDGMENTS = {
    "tools/goal-checks/produce.py": "goal test producer",
    "tools/review-checks/produce.py": "review regression producer",
    "tools/gap-checks/produce.py": "gap regression producer",
    "tools/gap-checks/preservation.py": "preservation producer",
    "tools/check_public_tree.py": "public file boundary",
    "tools/check_public_docs.py": "public documentation",
}
PUBLIC = ("tools/check_public_tree.py", "tools/check_public_docs.py")
TRACKED = "tools/goal-checks/release-0-1-3-review-fixes/test_private_independence.py"
TIMEOUT = 900


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


# --- static observations -------------------------------------------------

def load_verify():
    path = ROOT / "tools/verify.py"
    try:
        spec = importlib.util.spec_from_file_location("observed_verify", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    except Exception as error:
        raise Unavailable(f"tools/verify.py cannot be loaded: {error!r}") from None
    return module


def group_steps(*functions):
    """The (label, command) of each step the named group functions would run."""
    verify = load_verify()
    recorded = []
    with patch.object(verify, "run", lambda label, command, *args, **kwargs: recorded.append((label, list(command)))):
        for name in functions:
            try:
                getattr(verify, name)({})
            except Exception as error:
                raise Unavailable(f"{name} cannot be read with a recorder: {error!r}") from None
    return recorded


def dispatch(group):
    """The group functions main() calls for a group, with checks before them stubbed."""
    verify = load_verify()
    called = []
    out, err = io.StringIO(), io.StringIO()
    with patch.object(verify, "verify_go", lambda env: called.append("verify_go")), \
            patch.object(verify, "verify_docs", lambda env: called.append("verify_docs")), \
            patch.object(verify, "prepare_shells", lambda env: None), \
            patch.object(verify, "check_input", lambda env: None), \
            patch.object(verify.shutil, "which", lambda name, path=None: "/bin/" + name), \
            patch.object(sys, "argv", ["verify.py", group]), redirect_stdout(out), redirect_stderr(err):
        try:
            code = verify.main()
        except Exception as error:
            raise Unavailable(f"verify.py main({group}) cannot be read: {error!r}") from None
    if code != 0:
        raise Unavailable(f"verify.py main({group}) exited {code} with stubbed checks: {err.getvalue()}")
    return called


def full_suite(args):
    """`go test` over ./... without a -run or -list filter; args exclude `go`.

    A benchmark flag does not filter: the ordinary tests still run first.
    """
    filters = ("-run", "-list")
    return (args[:1] == ["test"] and "./..." in args
            and not any(a in filters or a.startswith(tuple(f + "=" for f in filters)) for a in args))


def judging(script, args):
    """Whether a run of a watched script is the judgment AC-4 names, not help or a no-op."""
    if any(a in ("-h", "--help") for a in args):
        return False
    if script == "tools/goal-checks/produce.py":
        return len(args) == 2 and all(a.endswith(".json") for a in args)
    if script in ("tools/review-checks/produce.py", "tools/gap-checks/produce.py"):
        return len(args) == 1 and re.fullmatch(r"AC-[0-9]+", args[0]) is not None
    if script == "tools/gap-checks/preservation.py":
        return "--retention" in args and args.index("--retention") + 1 < len(args)
    if script == "tools/check_public_tree.py":
        return not args
    if script == "tools/check_public_docs.py":
        return len(args) <= 1
    return True


# --- dynamic observations ------------------------------------------------

COMMON = r'''
import json, os, subprocess
from pathlib import Path

def _public_copy(start, root):
    """The public copy holding start, or None; this checkout is never a copy."""
    try:
        reference = (root / "tools/verify.py").read_bytes()
    except OSError:
        return None
    for directory in (start, *start.parents):
        if directory == root:
            return None
        if (directory / ".git").exists() and (directory / "tools/verify.py").is_file():
            try:
                same = (directory / "tools/verify.py").read_bytes() == reference
            except OSError:
                return None
            return directory if same else None
    return None

def _facts(copy):
    env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
    try:
        count = subprocess.run(["git", "-C", str(copy), "rev-list", "--count", "HEAD"], env=env,
                               capture_output=True, text=True, timeout=60).stdout.strip()
    except (OSError, subprocess.SubprocessError):
        count = ""
    return {"path": str(copy), "shallow": (copy / ".git/shallow").is_file(),
            "commits": int(count) if count.isdigit() else None,
            "docs": (copy / "docs").exists(), "ha": (copy / ".git/ha").exists()}

def _log(entry):
    data = (json.dumps(entry) + "\n").encode()
    fd = os.open(os.environ["JAEKIT_OBSERVE_LOG"], os.O_WRONLY | os.O_APPEND | os.O_CREAT, 0o600)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)
'''

SITE = COMMON + r'''
import sys

WATCHED = set(json.loads(os.environ.get("JAEKIT_OBSERVE_WATCHED", "[]")))
PUBLIC = json.loads(os.environ.get("JAEKIT_OBSERVE_PUBLIC", "[]"))

def _script(argv, cwd):
    i = 0
    while i < len(argv):
        a = argv[i]
        if a in ("-m", "-c"):
            return None, []
        if a in ("-W", "-X"):
            i += 2
            continue
        if a.startswith("-"):
            i += 1
            continue
        return (Path(a) if os.path.isabs(a) else cwd / a).resolve(), argv[i + 1:]
    return None, []

def _kind(facts):
    if facts["shallow"] and facts["commits"] == 1:
        return "shallow"
    if not facts["shallow"] and (facts["commits"] or 0) >= 2:
        return "full"
    return "other"

def _run(argv, cwd):
    # Only this direct child skips observation; its own children are observed.
    env = dict(os.environ, JAEKIT_OBSERVE_WRAPPER=str(os.getpid()))
    child = subprocess.run([sys.executable, *argv], cwd=cwd, env=env, capture_output=True)
    os.write(1, child.stdout)
    os.write(2, child.stderr)
    return child

def _fail(entry, message):
    os.write(2, (message + "\n").encode())
    entry.update(rc=1, stderr=message)
    _log(entry)
    os._exit(1)

def _observe():
    root = Path(os.environ["JAEKIT_OBSERVE_ROOT"])
    cwd = Path.cwd().resolve()
    argv = list(sys.orig_argv[1:])
    path, rest = _script(argv, cwd)
    # The copy a process judges: where its script lives, else where it runs.
    copy = (_public_copy(path.parent, root) if path is not None else None) or _public_copy(cwd, root)
    if copy is None:
        return
    script = None
    if path is not None:
        try:
            script = path.relative_to(copy).as_posix()
        except ValueError:
            script = "outside:" + str(path)
    facts = _facts(copy)
    entry = {"kind": "python", "argv": argv, "script": script, "args": rest, "copy": facts}
    if script == "tools/verify.py":
        group = rest[0] if rest else "all"
        entry.update(group=group, replaced=True)
        code = 0
        if group in ("all", "docs"):
            # A docs verification starts with these two checks; only they run.
            for check in PUBLIC:
                code |= subprocess.run([sys.executable, check], cwd=copy).returncode != 0
        entry["rc"] = int(code)
        _log(entry)
        os._exit(int(code))
    if script not in WATCHED:
        _log(entry)
        return
    target, kind, marker, where = (os.environ.get("JAEKIT_OBSERVE_MUTATE", "") + "|||").split("|")[:4]
    if target == script and where == _kind(facts):
        entry["mutation"] = kind
        if kind == "private":
            try:
                (copy / "docs/specs" / marker / "SPEC.md").read_bytes()
            except OSError as error:
                _fail(entry, f"{script}: needs private goal document docs/specs/{marker}/SPEC.md: {error}")
        elif kind == "history":
            check = subprocess.run(["git", "-C", str(copy), "rev-parse", "--verify", "--quiet", "HEAD~1^{commit}"],
                                   capture_output=True, text=True)
            if check.returncode != 0:
                _fail(entry, f"{script}: needs history beyond the checked commit (HEAD~1, {marker})")
    elif os.environ.get("JAEKIT_OBSERVE_FAST") and script in json.loads(os.environ.get("JAEKIT_OBSERVE_JUDGMENTS", "[]")):
        entry.update(rc=0, replaced=True)
        _log(entry)
        os._exit(0)
    child = _run(argv, cwd)
    stderr = child.stderr.decode("utf-8", "replace")
    entry.update(rc=child.returncode, fail="FAIL:" in stderr, error="ERROR:" in stderr, stderr=stderr[-2000:])
    _log(entry)
    os._exit(child.returncode)

if os.environ.get("JAEKIT_OBSERVE_LOG") and os.environ.get("JAEKIT_OBSERVE_WRAPPER") != str(os.getppid()):
    _observe()
'''

GO = COMMON + r'''
import sys

def _main():
    real, root = os.environ["JAEKIT_OBSERVE_GO"], Path(os.environ["JAEKIT_OBSERVE_ROOT"])
    args = sys.argv[1:]
    where, rest = Path.cwd(), args
    if args[:1] == ["-C"] and len(args) > 1:
        where, rest = where / args[1], args[2:]
    elif args[:1] and args[0].startswith("-C="):
        where, rest = where / args[0][3:], args[1:]
    copy = _public_copy(where.resolve(), root)
    filters = ("-run", "-list")
    full = (rest[:1] == ["test"] and "./..." in rest
            and not any(a in filters or a.startswith(tuple(f + "=" for f in filters)) for a in rest))
    if copy is not None:
        _log({"kind": "go", "argv": args, "full_suite": full, "replaced": full, "copy": _facts(copy)})
        if full:
            sys.exit(0)
    os.execv(real, [real, *args])

_main()
'''

RUNNER = r'''
import importlib.util, sys, unittest
from pathlib import Path
path, selector = Path(sys.argv[1]), sys.argv[2]
sys.path.insert(0, str(path.parent))
spec = importlib.util.spec_from_file_location(path.stem, path)
module = importlib.util.module_from_spec(spec)
sys.modules[path.stem] = module
spec.loader.exec_module(module)
loader = unittest.TestLoader()
suite = loader.loadTestsFromName(selector, module)
if loader.errors or not suite.countTestCases():
    sys.exit(3)
result = unittest.TextTestRunner(verbosity=2).run(suite)
sys.exit(0 if result.wasSuccessful() else 2 if result.errors else 1)
'''


def route_checks():
    """The go-group (file, selector) checks of the route in the checked tree."""
    try:
        data = json.loads((ROOT / "contracts/regressions.json").read_text(encoding="utf-8"))
        route = next(r for r in data["routes"] if r["id"] == ROUTE)
        checks = [(c["file"], c["selector"]) for c in route["checks"] if c.get("group") == "go"]
    except (OSError, ValueError, KeyError, TypeError, StopIteration) as error:
        raise Unavailable(f"route {ROUTE} cannot be read: {error!r}") from None
    if not checks:
        raise Unavailable(f"route {ROUTE} lists no go-group checks")
    return checks


class Run:
    """One run of selected route checks under the process log."""

    def __init__(self, checks, mutate=None, fast=False):
        for tool in ("git", "go"):
            if shutil.which(tool) is None:
                raise Unavailable(f"missing required tool: {tool}")
        self.entries, self.results = [], {}
        with tempfile.TemporaryDirectory(prefix="jaekit-copy-observation-") as directory:
            base = Path(directory)
            site, shims = base / "site", base / "bin"
            site.mkdir()
            shims.mkdir()
            (site / "sitecustomize.py").write_text(SITE, encoding="utf-8")
            go = shims / "go"
            go.write_text(f"#!{sys.executable} -I\n" + GO, encoding="utf-8")
            go.chmod(0o755)
            runner = base / "run_selector.py"
            runner.write_text(RUNNER, encoding="utf-8")
            log = base / "log.jsonl"
            env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", JAEKIT_OBSERVE_LOG=str(log),
                       JAEKIT_OBSERVE_ROOT=str(ROOT.resolve()), JAEKIT_OBSERVE_GO=shutil.which("go"),
                       JAEKIT_OBSERVE_WATCHED=json.dumps(sorted({*JUDGMENTS, TRACKED})),
                       JAEKIT_OBSERVE_PUBLIC=json.dumps(list(PUBLIC)),
                       JAEKIT_OBSERVE_JUDGMENTS=json.dumps(sorted(JUDGMENTS)),
                       PATH=str(shims) + os.pathsep + os.environ.get("PATH", ""),
                       PYTHONPATH=os.pathsep.join(filter(None, (str(site), os.environ.get("PYTHONPATH")))))
            for key in ("JAEKIT_OBSERVE_MUTATE", "JAEKIT_OBSERVE_FAST", "JAEKIT_OBSERVE_WRAPPER",
                        "HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
                env.pop(key, None)
            if mutate:
                env["JAEKIT_OBSERVE_MUTATE"] = "|".join(mutate)
            if fast:
                env["JAEKIT_OBSERVE_FAST"] = "1"
            for file, selector in checks:
                before = len(log.read_bytes().splitlines()) if log.exists() else 0
                try:
                    proc = subprocess.run([sys.executable, str(runner), str(ROOT / file), selector], cwd=ROOT,
                                          env=env, capture_output=True, text=True, timeout=TIMEOUT)
                except (OSError, subprocess.TimeoutExpired) as error:
                    raise Unavailable(f"{file} {selector}: {error}") from None
                if proc.returncode == 3:
                    raise Unavailable(f"{file} {selector} cannot be collected: {proc.stderr[-2000:]}")
                lines = log.read_bytes().splitlines() if log.exists() else []
                entries = [json.loads(line) for line in lines[before:]]
                self.results[(file, selector)] = (proc.returncode, proc.stdout + proc.stderr, entries)
                self.entries += entries

    def copies(self):
        found = {}
        for entry in self.entries:
            found.setdefault(entry["copy"]["path"], entry["copy"])
        return found

    @staticmethod
    def kind(copy):
        if copy["shallow"] and copy["commits"] == 1:
            return "shallow"
        if not copy["shallow"] and (copy["commits"] or 0) >= 2:
            return "full"
        return "other"

    def python(self, script):
        return [e for e in self.entries if e["kind"] == "python" and e["script"] == script and not e.get("replaced")]

    def creators(self):
        return [check for check, (_, _, entries) in self.results.items() if entries]


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def ran_in_both(run, script):
    """The copy kinds in which script made its judgment and exited 0."""
    return {Run.kind(e["copy"]) for e in run.python(script) if e.get("rc") == 0 and judging(script, e.get("args", []))}


def nested_full_suites(run):
    """Full Go suites started inside copies, directly or through a go or all verification."""
    return ([[e["copy"]["path"], e["argv"]] for e in run.entries if e["kind"] == "go" and e["full_suite"]]
            + [[e["copy"]["path"], e["argv"]] for e in run.entries
               if e["kind"] == "python" and e["script"] == "tools/verify.py" and e.get("group") in ("go", "all")])


def assert_route_passed(run):
    """Called after the observed property: a route failure for another reason is an error."""
    failed = {f"{file} {selector}": output[-3000:] for (file, selector), (code, output, _) in run.results.items()
              if code != 0}
    if failed:
        raise Unavailable("route checks did not pass under observation: " + json.dumps(failed)[:6000])


class SuiteRuns(unittest.TestCase):
    """AC-1: one full Go suite per group run, in this checkout only."""

    def test_go_and_all_groups_run_the_full_go_suite_once_in_this_checkout(self):
        direct = {group: [command for _, command in group_steps(*functions)
                          if command[0] == "go" and full_suite(command[1:])]
                  for group, functions in (("go", ("verify_go",)), ("all", ("verify_go", "verify_docs")))}
        run = Run(route_checks())
        nested = nested_full_suites(run)
        for group in ("go", "all"):
            require(self, len(direct[group]) == 1 and not nested, "full-go-suite-repeated",
                    f"{group} group: {len(direct[group])} full Go suite(s) in this checkout and {len(nested)} "
                    f"inside public copies: " + json.dumps(nested))
        assert_route_passed(run)


class DocsRuns(unittest.TestCase):
    """AC-2: copies do not repeat the docs group; public checks run there; `all` runs docs once."""

    def test_copies_do_not_rerun_the_docs_group(self):
        run = Run(route_checks())
        nested = [e for e in run.entries if e["kind"] == "python" and e["script"] == "tools/verify.py"
                  and e.get("group") in ("all", "docs")]
        go_steps = {tuple(command) for _, command in group_steps("verify_go")}
        docs_steps = [tuple(command) for _, command in group_steps("verify_docs")]
        require(self, not nested and not go_steps.intersection(docs_steps), "docs-group-repeated-in-copies",
                f"{len(nested)} docs-group verification(s) inside public copies: "
                + json.dumps([[e['copy']['path'], e['argv']] for e in nested])
                + f"; go-group steps shared with the docs group: {sorted(go_steps.intersection(docs_steps))}")
        assert_route_passed(run)

    def test_public_checks_run_and_pass_in_both_copies(self):
        run = Run(route_checks())
        missing = {script: sorted({"full", "shallow"} - ran_in_both(run, script)) for script in PUBLIC}
        missing = {script: kinds for script, kinds in missing.items() if kinds}
        require(self, not missing, "public-checks-not-in-copies",
                f"public checks that did not run and pass in a copy: {missing}")
        assert_route_passed(run)

    def test_all_group_runs_the_docs_group_once_here(self):
        calls = {group: dispatch(group) for group in ("all", "go", "docs")}
        require(self, calls == {"all": ["verify_go", "verify_docs"], "go": ["verify_go"], "docs": ["verify_docs"]},
                "docs-group-not-run-once", f"group functions called by main(): {calls}")


class CopyBoundary(unittest.TestCase):
    """AC-3: full and shallow public-only copies accept valid and reject forbidden current files."""

    def test_two_public_only_copies_accept_and_reject_current_files(self):
        run = Run(route_checks())
        assert_route_passed(run)
        copies = run.copies()
        kinds = sorted(Run.kind(copy) for copy in copies.values())
        private = sorted(path for path, copy in copies.items() if copy["docs"] or copy["ha"])
        self.assertTrue({"full", "shallow"} <= set(kinds) and not private,
                        f"copies {kinds}; with docs/ or .git/ha: {private}")
        tracked = [e for e in run.python(TRACKED) if "TrackedFiles" in e["argv"]]
        accepted = {Run.kind(e["copy"]) for e in tracked if e.get("rc") == 0}
        rejected = {Run.kind(e["copy"]) for e in tracked if e.get("rc") == 1 and e.get("fail") and not e.get("error")}
        self.assertTrue({"full", "shallow"} <= accepted and {"full", "shallow"} <= rejected,
                        f"TrackedFiles accepted in {sorted(accepted)}, rejected with FAIL: in {sorted(rejected)}")


class Judgments(unittest.TestCase):
    """AC-4: each current-file judgment runs in both copies, and a dependency fails the group."""

    def test_judgments_run_and_pass_in_both_copies(self):
        run = Run(route_checks())
        missing = {script: sorted({"full", "shallow"} - ran_in_both(run, script)) for script in JUDGMENTS}
        missing = {script: kinds for script, kinds in missing.items() if kinds}
        require(self, not missing, "current-file-judgment-not-in-copies",
                f"current-file judgments that did not run and pass in a copy: {missing}")
        assert_route_passed(run)

    def test_private_input_or_history_dependency_fails_the_go_group(self):
        run = Run(route_checks())
        assert_route_passed(run)
        creators = run.creators()
        unreported = []
        # A private input is absent from both copies, so it must fail the group
        # from each copy on its own; history beyond the checked commit is
        # absent only from the shallow copy.
        cases = (("private", "full"), ("private", "shallow"), ("history", "shallow"))
        for index, script in enumerate(sorted(JUDGMENTS)):
            if {"full", "shallow"} - ran_in_both(run, script):
                # Not run in both copies: a dependency there cannot fail the group.
                unreported.append(f"{JUDGMENTS[script]} ({script}) does not run in both public copies")
                continue
            for kind, where in cases:
                marker = f"jaekit-observer-{kind}-{where}-{index}"
                mutated = Run(creators, mutate=(script, kind, marker, where), fast=True)
                codes = [code for code, _, _ in mutated.results.values()]
                if any(code not in (0, 1) for code in codes):
                    raise Unavailable(f"{script} {kind} in {where}: route checks ended with {codes}: "
                                      + json.dumps([o[-2000:] for _, o, _ in mutated.results.values()])[:4000])
                applied = [e for e in mutated.entries if e.get("mutation") == kind and e["script"] == script
                           and e.get("rc") == 1 and Run.kind(e["copy"]) == where]
                if not applied:
                    raise Unavailable(f"{script}: the {kind} dependency was not reached in the {where} copy")
                failed = [output for code, output, _ in mutated.results.values() if code == 1]
                if not (failed and any(marker in output for output in failed)):
                    unreported.append(f"{JUDGMENTS[script]} ({script}) needing {kind} input in the {where} copy: "
                                      f"route checks {'failed' if failed else 'passed'}, the failure "
                                      f"{'shows' if any(marker in o for o in failed) else 'does not show'} "
                                      "the needed input")
        require(self, not unreported, "dependency-failure-not-reported", "\n".join(unreported))


class RegressionRoute(unittest.TestCase):
    """AC-6: the protected route states which checks run in which copy."""

    PHRASES = ("full and shallow public-only copies", "producers' current-file judgments",
               "public file and documentation checks", "run once in the original tree",
               "explicit audits do not fetch absent history")
    CHECKS = {("tools/review-checks/test_producers.py", "AC9"),
              ("tools/review-checks/test_producers.py",
               "AC10.test_both_audits_require_an_explicit_range_and_do_not_fetch_missing_history"),
              ("tools/gap-checks/test_history_results.py", "AC4"),
              ("tools/gap-checks/test_history_results.py", "AC5")}

    def test_route_records_the_new_observation(self):
        try:
            data = json.loads((ROOT / "contracts/regressions.json").read_text(encoding="utf-8"))
            route = next(r for r in data["routes"] if r["id"] == ROUTE)
            prop, status = route["property"], route["status"]
            checks = {(c["file"], c["selector"]) for c in route["checks"] if c.get("group") == "go"}
        except (OSError, ValueError, KeyError, TypeError, StopIteration) as error:
            raise Unavailable(f"route {ROUTE} cannot be read: {error!r}") from None
        missing = [p for p in self.PHRASES if p not in prop]
        require(self, status == "protected" and not missing and self.CHECKS <= checks,
                "route-observation-not-recorded",
                f"status {status!r}; property lacks {missing}; go checks lack {sorted(self.CHECKS - checks)}")


def section(text, heading):
    lines = text.splitlines()
    try:
        start = lines.index(heading)
    except ValueError:
        return ""
    end = next((i for i in range(start + 1, len(lines)) if lines[i].startswith("## ")), len(lines))
    return "\n".join(lines[start:end])


class Guides(unittest.TestCase):
    """AC-8: both CI guides state what the go group and the public copies observe."""

    GUIDES = {
        "guides/CI.en.md": ("## Local checks", (
            "runs the full Go test suite once", "two public-only copies", "a depth-1 clone",
            "current-file judgments", "does not repeat the full Go suite or the `docs` group",
            "required CI jobs")),
        "guides/CI.md": ("## 로컬 검사", (
            "전체 Go 시험을 한 번", "공개 사본 두 개", "depth 1 clone", "현재 파일 판정",
            "전체 Go 시험과 `docs` 그룹을 되풀이하지 않습니다", "필수 CI job")),
    }

    def test_ci_guides_describe_the_go_group_scope(self):
        missing = []
        for name, (heading, phrases) in self.GUIDES.items():
            try:
                body = section((ROOT / name).read_text(encoding="utf-8"), heading)
            except (OSError, UnicodeDecodeError) as error:
                raise Unavailable(f"{name}: {error}") from None
            missing += [f"{name} {heading}: {p}" for p in phrases if p not in body]
        require(self, not missing, "ci-guide-scope-missing", "\n".join(missing))


if __name__ == "__main__":
    unittest.main()
