"""Shared fixture for the runner-adapters observers.

Every observation builds `ha` from the checked tree and runs that executable
in a synthetic Git repository made in a temporary directory. The repository
holds a goal under docs/specs/demo whose change conditions run real processes:
`go test -json`, Node's built-in test runner, or tools/goal-checks/
runner-adapters/runner.py, which writes one fixed runner output per case.
The observers read only what `ha` wrote: its exit codes, its output, and the
synthetic repository's runs.jsonl. They never read the checked repository's
history, and the checked tree is never written.

Violations come only from explicit `[violation:...]` failures. A tool, build,
Git operation or other input that is missing or unusable raises Unavailable,
an execution error.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent
RUNNER = HERE / "runner.py"
GOAL = "docs/specs/demo"
REQUEST = "어댑터 관측용 합성 목표를 구현해줘"
# The declaration format, adapters, placeholder and variable this goal's
# contract names (PLAN 구현 결정).
SCHEMA = "check-adapter/v1"
PLACEHOLDER = "{HA_JUNIT_XML}"
VARIABLE = "HA_JUNIT_XML"
UNMARKED = "jaekit:unmarked-failure"
FEATURE = "runner-adapters/v1"


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def run(argv, cwd, env=None, timeout=600, stdin=None):
    """Run a tool; a tool that cannot start, times out or ends by a signal is unavailable, never a violation."""
    try:
        result = subprocess.run([str(a) for a in argv], cwd=cwd, env=env, capture_output=True, timeout=timeout,
                                stdin=subprocess.DEVNULL if stdin is None else stdin)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unavailable(f"{argv[0]}: {error}") from None
    if result.returncode < 0:
        raise Unavailable(f"{argv[0]} ended by signal {-result.returncode}")
    return result


def clean_env():
    """The caller's environment without Git, ha evidence or Go selection settings."""
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("GIT_", "HA_")) and k not in ("GOFLAGS", "GOOS", "GOARCH")}
    env.update(GOTOOLCHAIN="local", GOPROXY="off", GOFLAGS="-mod=mod", CGO_ENABLED="0")
    return env


def build(out, overlay=None, goos=None, goarch=None):
    """`ha` built from the checked tree, optionally with files laid over it."""
    out = Path(out)
    out.parent.mkdir(parents=True, exist_ok=True)
    env = clean_env()
    env["GOFLAGS"] = "-mod=readonly"
    if goos:
        env.update(GOOS=goos, GOARCH=goarch)
    flags = []
    if overlay:
        replace = {}
        for index, (path, content) in enumerate(sorted(overlay.items())):
            laid = out.parent / f"overlay-{index}-{Path(path).name}"
            laid.write_text(content, encoding="utf-8")
            replace[str(ROOT / path)] = str(laid)
        spec = out.parent / f"overlay-{out.name}.json"
        spec.write_text(json.dumps({"Replace": replace}), encoding="utf-8")
        flags = ["-overlay", str(spec)]
    result = run(["go", "build", *flags, "-o", out, "./cmd/ha"], ROOT, env, timeout=600)
    if result.returncode:
        raise Unavailable(f"go build: {result.stderr.decode(errors='replace')[-2000:]}")
    return out


def go_test_results(packages, flags=(), env=None, timeout=3600, go="go"):
    """(failed, passed) test names of `go test -json`, each as "package test".

    Only tests that ran to a failure are returned as failures. A build
    failure, a run ended by a signal, output that is not test events, a test
    that started without an end (the test binary crashed, was killed or hit
    Go's own timeout), a package that failed without a failed test, or a
    nonzero exit without any failed test cannot establish a result and raises
    Unavailable.
    """
    result = run([go, "test", "-json", *flags, *packages], ROOT, env, timeout=timeout)
    events = []
    for line in result.stdout.decode(errors="replace").splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            raise Unavailable(f"go test -json printed a line that is not an event: {line[:200]!r}") from None
    built = [e.get("FailedBuild") for e in events if e.get("Action") == "fail" and e.get("FailedBuild")]
    if built:
        raise Unavailable(f"packages did not build: {built[:3]}")
    name = lambda e: f"{e.get('Package')} {e.get('Test', '')}".strip()
    started = {name(e) for e in events if e.get("Action") == "run" and e.get("Test")}
    ended = {name(e) for e in events if e.get("Action") in ("pass", "fail", "skip") and e.get("Test")}
    if started - ended:
        raise Unavailable(f"tests started without an end: {sorted(started - ended)[:3]}")
    failed = sorted({name(e) for e in events if e.get("Action") == "fail" and e.get("Test")})
    passed = sorted({name(e) for e in events if e.get("Action") == "pass" and e.get("Test")})
    failed_packages = {e.get("Package") for e in events if e.get("Action") == "fail" and not e.get("Test")}
    silent = sorted(p for p in failed_packages if not any(f.startswith(f"{p} ") for f in failed))
    if silent:
        raise Unavailable(f"packages failed without a failed test: {silent[:3]}")
    if not passed and not failed:
        raise Unavailable(f"go test ran no test (exit {result.returncode}): {result.stderr.decode(errors='replace')[-500:]}")
    if result.returncode and not failed:
        raise Unavailable(f"go test exit {result.returncode} without a failed test: {result.stderr.decode(errors='replace')[-500:]}")
    return failed, passed


def unittest_failures(directory, pattern, env=None, timeout=1800):
    """Failed test count of a unittest discovery; errors and other outcomes raise Unavailable."""
    result = run([sys.executable, "-m", "unittest", "discover", "-s", directory, "-p", pattern], ROOT, env, timeout=timeout)
    tail = result.stderr.decode(errors="replace").strip().splitlines()[-1:] or [""]
    if result.returncode < 0:
        raise Unavailable(f"unittest ended by signal {-result.returncode}")
    if result.returncode == 0 and tail[0].startswith("OK"):
        return 0
    match = re.fullmatch(r"FAILED \((?:failures=(\d+))?(?:, )?(?:errors=(\d+))?(?:, )?.*\)", tail[0])
    if not match or match.group(2) or not match.group(1):
        raise Unavailable(f"unittest {pattern} did not end with assertion failures only: {tail[0]!r}")
    return int(match.group(1))


def go_typed():
    """Raise Unavailable unless the selected Go marks failure output (Go 1.27 or later)."""
    result = run(["go", "env", "GOVERSION"], ROOT, clean_env(), timeout=120)
    version = result.stdout.decode().strip()
    match = re.fullmatch(r"go(\d+)\.(\d+)(\.\d+)?", version)
    if result.returncode or not match:
        raise Unavailable(f"go env GOVERSION: {version or result.stderr.decode(errors='replace')}")
    if (int(match.group(1)), int(match.group(2))) < (1, 27):
        raise Unavailable(f"{version} does not mark failure output; the observation needs Go 1.27 or later")
    return version


def node_runner():
    """Node's built-in test runner with its junit reporter, or Unavailable."""
    node = shutil.which("node")
    if node is None:
        raise Unavailable("node is required for the real JUnit runner")
    result = run([node, "--version"], ROOT, clean_env(), timeout=60)
    match = re.fullmatch(r"v(\d+)\.\d+\.\d+", result.stdout.decode().strip())
    if result.returncode or not match or int(match.group(1)) < 22:
        raise Unavailable(f"node --version: {result.stdout.decode().strip()} (Node 22 or later has the junit reporter)")
    return node


def criteria_spec(ids):
    lines = [f"- **{i}** The synthetic condition {i} holds." for i in ids]
    return ("# Demo\n\nStatus: Ready\n\n## 목표\n\nA synthetic goal for the adapter observations.\n\n"
            "## 범위와 비목표\n\n- 범위: `src/`, `checks/`, `tests/`\n- 비목표: 없음\n\n"
            "## Acceptance Criteria\n" + "\n".join(lines) + "\n\n## Open Decisions\n\n없음\n")


class Condition:
    """One condition row: command (argv words), check paths and an optional declaration."""

    def __init__(self, cid, command, checks=(), declaration=None, kind="change", limit=None):
        self.id, self.command, self.checks, self.kind, self.limit = cid, command, list(checks), kind, limit
        self.declaration = declaration

    def cell(self):
        return "`" + " ".join(self.command) + "`"


class Repo:
    """A synthetic goal repository and the `ha` executable that records in it."""

    def __init__(self, directory, ha, conditions, files=None, rules="run-rules/4", runs=400):
        self.root = Path(directory) / "repo"
        self.root.mkdir(parents=True)
        self.ha_path, self.conditions, self.rules = Path(ha), conditions, rules
        self.env = clean_env()
        self.env.update(GIT_AUTHOR_NAME="Observer", GIT_AUTHOR_EMAIL="observer@example.invalid",
                        GIT_COMMITTER_NAME="Observer", GIT_COMMITTER_EMAIL="observer@example.invalid",
                        GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=str(Path(directory) / "gitconfig"))
        (Path(directory) / "gitconfig").write_text("", encoding="utf-8")
        self.runs = runs
        for path, content in (files or {}).items():
            self.write(path, content)
        self.write_goal()
        self.git("init", "-q", "-b", "main")
        self.commit("base")

    # --- files and Git -------------------------------------------------------

    def write(self, path, content):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        if isinstance(content, bytes):
            target.write_bytes(content)
        else:
            target.write_text(content, encoding="utf-8")

    def write_goal(self):
        ids = [c.id for c in self.conditions if c.id.startswith("AC-")]
        rows, contract, checks = [], [], set()
        for c in self.conditions:
            paths = list(c.checks)
            if c.declaration is not None:
                decl = f"checks/{c.id}.json"
                self.write(decl, json.dumps(c.declaration, indent=1, ensure_ascii=False) + "\n")
                paths.append(decl)
                contract.append(f"| {c.id} | {decl} |")
            if c.kind == "change" and not paths:
                raise Unavailable(f"{c.id} needs check paths")
            rows.append(f"| {c.id} | {c.kind} | {c.cell()} | {', '.join(paths) if paths else '—'} | T001 |")
            checks.update(paths)
        plan = ["# PLAN — demo", "", "## 조건표", "| ID | 종류 | 검증 명령 | 검사 경로 | Task |",
                "| --- | --- | --- | --- | --- |", *rows, ""]
        if contract:
            plan += ["## 결과 계약", "| ID | 선언 경로 |", "| --- | --- |", *contract, ""]
        limits = [f"- {c.id}: {c.limit}" for c in self.conditions if c.limit]
        if limits:
            plan += ["## 시간 제한", *limits, ""]
        task_ids = ", ".join(c.id for c in self.conditions)
        plan += ["## 범위", "- 바꿀 수 있는 경로: `**` (SPEC: 범위와 비목표)",
                 "- 테스트 경로: (기본값)", "- 유지할 동작: 없음", "",
                 "## Task", "| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |", "| --- | --- | --- | --- | --- | --- | --- |",
                 f"| T001 | synthetic conditions hold | {task_ids} | — | — | 예 | — |", "",
                 "## 예산", f"- 검증 실행: {self.runs}", "- 경과 시간: 10시간", "- 비용: 미관측", ""]
        self.write(f"{GOAL}/SPEC.md", criteria_spec(ids))
        self.write(f"{GOAL}/PLAN.md", "\n".join(plan))
        self.write(f"{GOAL}/REVIEW.md", "# REVIEW — demo\n\n## 결론\nready\n")
        self.write(f"{GOAL}/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n"
                                          "| --- | --- | --- |\n| T001 | todo | |\n")

    def git(self, *args):
        result = run(["git", *args], self.root, self.env, timeout=120)
        if result.returncode:
            raise Unavailable(f"git {args[0]}: {result.stderr.decode(errors='replace')}")
        return result.stdout.decode()

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "--allow-empty", "-m", message)
        return self.git("rev-parse", "HEAD").strip()

    # --- ha -----------------------------------------------------------------

    def ha(self, *args, env=None, timeout=900):
        result = run([self.ha_path, *args], self.root, env or self.env, timeout=timeout)
        return result.returncode, result.stdout.decode(errors="replace"), result.stderr.decode(errors="replace")

    def start(self):
        """(exit, output) of the goal's start; rules other than /4 use the default."""
        args = ["start", GOAL, "--request", REQUEST, "--skill-name", "seal", "--skill-version", "0.1.0"]
        if self.rules == "run-rules/4":
            args += ["--rules", "run-rules/4"]
        code, out, err = self.ha(*args)
        return code, out + err

    def check(self, *ids, baseline=False, env=None):
        args = ["check", GOAL, *(["--baseline"] if baseline else []), *ids]
        code, out, err = self.ha(*args, env=env)
        return code, out + err

    def records(self):
        path = self.root / GOAL / "runs.jsonl"
        if not path.exists():
            return []
        try:
            return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
        except (OSError, ValueError) as error:
            raise Unavailable(f"runs.jsonl: {error}") from None

    def last(self, cid, kind):
        found = [r for r in self.records() if r.get("kind") == kind and (r.get("target") or {}).get("criterion") == cid]
        return found[-1] if found else None

    def status(self):
        code, out, err = self.ha("status", GOAL, "--format", "json")
        try:
            return code, json.loads(out)
        except ValueError:
            raise Unavailable(f"ha status --format json: exit {code}: {(out + err)[-1000:]}") from None


def summary(record):
    """(result, reason, [(target, attempt, status, violation, reason)]) of a stored run, or None."""
    if record is None:
        return None
    evidence = record.get("evidence") or {}
    targets = [(t.get("target"), t.get("attempt"), t.get("status"), t.get("violation", ""), t.get("reason", ""))
               for t in evidence.get("targets") or []]
    return record.get("result"), evidence.get("reason"), targets


def go_declaration(targets):
    """check-adapter/v1 for go-test-json: [(id, violation, package, name)]."""
    return {"schema": SCHEMA, "adapter": "go-test-json",
            "targets": [{"id": i, "violation": v, "test": {"package": p, "name": n}} for i, v, p, n in targets]}


def junit_declaration(targets):
    """check-adapter/v1 for junit-xml: [(id, violation, classname, name)]."""
    return {"schema": SCHEMA, "adapter": "junit-xml",
            "targets": [{"id": i, "violation": v, "test": {"classname": c, "name": n}} for i, v, c, n in targets]}


def runner_command(case, *extra):
    """The argv words of a runner.py case; the runner file is copied into the repository."""
    return ["python3", "tests/runner.py", case, *extra]


def runner_file():
    try:
        return RUNNER.read_text(encoding="utf-8")
    except OSError as error:
        raise Unavailable(f"{RUNNER}: {error}") from None


def digest_tree(path, skip=(".git",)):
    found = {}
    for item in sorted(Path(path).rglob("*")):
        rel = item.relative_to(path)
        if rel.parts and rel.parts[0] in skip:
            continue
        if item.is_symlink():
            found[str(rel)] = "symlink:" + os.readlink(item)
        elif item.is_file():
            found[str(rel)] = hashlib.sha256(item.read_bytes()).hexdigest()
        elif item.is_dir():
            found[str(rel)] = "dir"
    return found


def temporary(prefix):
    return tempfile.TemporaryDirectory(prefix=prefix)


def python():
    return sys.executable
