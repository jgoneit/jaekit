"""Shared fixture for the ha-start-budget-check observers.

Every observation builds `ha` from the checked tree and runs that executable
in a synthetic Git repository made in a temporary directory. Each `ha` run
gets a temporary user configuration folder (`XDG_CONFIG_HOME`), a temporary
home (`HOME`, and `USERPROFILE` on Windows) and no `GIT_`, `HA_` or
`XDG_CONFIG_HOME` value of the caller, so the real user settings are never
read. The observers read only what `ha` wrote: its exit codes, its output,
`ha status --format json` and the synthetic repository's runs.jsonl. They
never read the checked repository's history, and the checked tree is never
written.

Violations come only from explicit `[violation:...]` failures (require). A
tool, build, Git operation or other input that is missing or unusable raises
Unavailable, an execution error.
"""
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent
GOAL = "docs/specs/demo"
REQUEST = "자동 예산 관측용 합성 목표를 구현해줘"
# The names this goal's contract gives (PLAN 구현 결정 R1-R9).
AUTO = "자동"
CONFIG_DIR = "jaekit"
CONFIG_FILE = "ha.json"
CONFIG_KEY = "preapproved_runs"
DECLARATION = "core-requirements.json"
DECLARATION_SCHEMA = "core-requirements/v1"
CHECK_FLAG = "--check-skill"
FEATURES = ("auto-budget/v1", "core-requirements/v1")
FORMAT_KEY = "core_requirements"


# One lint problem line of `ha lint` and of a refused `ha estimate`.
LINT_LINE = re.compile(r"^[a-z][a-z0-9_]*\t[^\t\n]+\t", re.MULTILINE)


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
    """The caller's environment without Git, ha evidence, user config or Go selection settings."""
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("GIT_", "HA_", "JAEKIT_")) and k not in (
               "GOFLAGS", "GOOS", "GOARCH", "XDG_CONFIG_HOME", "SPAR_CONFIG", "CLEAN_REVIEW_CONFIG")}
    env.update(GOTOOLCHAIN="local", GOPROXY="off", GOFLAGS="-mod=mod", CGO_ENABLED="0")
    return env


def build(out):
    """`ha` built from the checked tree."""
    out = Path(out)
    out.parent.mkdir(parents=True, exist_ok=True)
    env = clean_env()
    env["GOFLAGS"] = "-mod=readonly"
    if os.name == "nt" and out.suffix != ".exe":
        out = out.with_suffix(".exe")
    result = run(["go", "build", "-o", out, "./cmd/ha"], ROOT, env, timeout=600)
    if result.returncode:
        raise Unavailable(f"go build: {result.stderr.decode(errors='replace')[-2000:]}")
    return out


def capabilities(ha, cwd):
    result = run([ha, "capabilities", "--format", "json"], cwd, clean_env(), timeout=60)
    try:
        report = json.loads(result.stdout)
    except ValueError:
        raise Unavailable(f"ha capabilities: exit {result.returncode}: {result.stderr.decode(errors='replace')}") from None
    if result.returncode or not isinstance(report, dict):
        raise Unavailable(f"ha capabilities: exit {result.returncode}")
    return report


def temporary(prefix):
    return tempfile.TemporaryDirectory(prefix=prefix)


def go_env(directory):
    """The environment for `go test` and the repository's own checks.

    Git and ha settings are dropped, and XDG_CONFIG_HOME names an empty folder
    under directory, so no test reads the caller's pre-approved run limit from
    $XDG_CONFIG_HOME or ~/.config. HOME stays, so Go keeps its build cache.
    """
    folder = Path(directory) / "empty-config"
    folder.mkdir(exist_ok=True)
    env = {k: v for k, v in os.environ.items() if not k.startswith(("GIT_", "HA_"))}
    env.update(GOTOOLCHAIN="local", GOFLAGS="-mod=readonly", XDG_CONFIG_HOME=str(folder))
    return env


def criteria_spec(ids):
    lines = [f"- **{i}** The synthetic condition {i} holds." for i in ids]
    return ("# Demo\n\nStatus: Ready\n\n## 목표\n\nA synthetic goal for the budget observations.\n\n"
            "## 범위와 비목표\n\n- 범위: `tests/`\n- 비목표: 없음\n\n"
            "## Acceptance Criteria\n" + "\n".join(lines) + "\n\n## Open Decisions\n\n없음\n")


class Repo:
    """A synthetic goal repository, its user folders and the `ha` that records in it.

    The goal has `changes` required change rows, `maintains` maintain rows and
    `manuals` manual rows. Commands are never run by the budget observations.
    """

    def __init__(self, directory, ha, changes=0, maintains=0, manuals=0, runs=AUTO, elapsed="10시간", git_env=None):
        self.base = Path(directory)
        self.root = self.base / "repo"
        self.root.mkdir(parents=True)
        self.ha_path = Path(ha)
        self.xdg = self.base / "xdg"
        self.home = self.base / "home"
        self.xdg.mkdir()
        self.home.mkdir()
        self.env = clean_env()
        self.env.update(GIT_AUTHOR_NAME="Observer", GIT_AUTHOR_EMAIL="observer@example.invalid",
                        GIT_COMMITTER_NAME="Observer", GIT_COMMITTER_EMAIL="observer@example.invalid",
                        GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=str(self.base / "gitconfig"),
                        XDG_CONFIG_HOME=str(self.xdg), HOME=str(self.home))
        if os.name == "nt":
            self.env["USERPROFILE"] = str(self.home)
        self.env.update(git_env or {})
        (self.base / "gitconfig").write_text("", encoding="utf-8")
        self.kinds = (["change"] * changes) + (["maintain"] * maintains) + (["manual"] * manuals)
        if not self.kinds:
            raise Unavailable("a synthetic goal needs at least one condition")
        self.write("tests/x.sh", "exit 0\n")
        self.write_goal(runs, elapsed)
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

    def plan(self, runs, elapsed):
        rows = []
        for index, kind in enumerate(self.kinds, start=1):
            if kind == "manual":
                rows.append(f"| AC-{index} | manual | — 사람이 확인한다 | — | T001 |")
            elif kind == "change":
                rows.append(f"| AC-{index} | change | `sh tests/x.sh` | tests/x.sh | T001 |")
            else:
                rows.append(f"| AC-{index} | maintain | `sh tests/x.sh` | — | T001 |")
        ids = ", ".join(f"AC-{i}" for i in range(1, len(self.kinds) + 1))
        budget = [f"- 검증 실행: {runs}"]
        if elapsed is not None:
            budget.append(f"- 경과 시간: {elapsed}")
        return "\n".join(["# PLAN — demo", "", "## 조건표", "| ID | 종류 | 검증 명령 | 검사 경로 | Task |",
                          "| --- | --- | --- | --- | --- |", *rows, "",
                          "## 범위", "- 바꿀 수 있는 경로: `**` (SPEC: 범위와 비목표)",
                          "- 테스트 경로: (기본값)", "- 유지할 동작: 없음", "",
                          "## Task", "| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |",
                          "| --- | --- | --- | --- | --- | --- | --- |",
                          f"| T001 | synthetic conditions hold | {ids} | — | — | 예 | — |", "",
                          "## 예산", *budget, "- 비용: 미관측", ""])

    def write_goal(self, runs, elapsed):
        ids = [f"AC-{i}" for i in range(1, len(self.kinds) + 1)]
        self.write(f"{GOAL}/SPEC.md", criteria_spec(ids))
        self.write(f"{GOAL}/PLAN.md", self.plan(runs, elapsed))
        self.write(f"{GOAL}/REVIEW.md", "# REVIEW — demo\n\n## 결론\nready\n")
        self.write(f"{GOAL}/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n"
                                          "| --- | --- | --- |\n| T001 | todo | |\n")

    def set_budget(self, runs, elapsed="10시간"):
        self.write(f"{GOAL}/PLAN.md", self.plan(runs, elapsed))

    def git(self, *args):
        result = run(["git", *args], self.root, self.env, timeout=120)
        if result.returncode:
            raise Unavailable(f"git {args[0]}: {result.stderr.decode(errors='replace')}")
        return result.stdout.decode()

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "--allow-empty", "-m", message)
        return self.git("rev-parse", "HEAD").strip()

    # --- user configuration -------------------------------------------------

    def config_path(self, base=None):
        return Path(base or self.xdg) / CONFIG_DIR / CONFIG_FILE

    def write_config(self, content, base=None):
        path = self.config_path(base)
        path.parent.mkdir(parents=True, exist_ok=True)
        if isinstance(content, bytes):
            path.write_bytes(content)
        else:
            path.write_text(content, encoding="utf-8")
        return path

    def clear_config(self):
        for base in (self.xdg, self.home / ".config"):
            folder = Path(base) / CONFIG_DIR
            if folder.is_symlink() or folder.is_file():
                folder.unlink()
            elif folder.exists():
                shutil.rmtree(folder)

    # --- skills -------------------------------------------------------------

    def skill(self, name, declaration=None, raw=None):
        """A synthetic skill folder outside the repository; returns its SKILL.md."""
        folder = self.base / "skills" / name
        folder.mkdir(parents=True)
        (folder / "SKILL.md").write_text(f"---\nname: {name}\nmetadata:\n  version: 1.2.3\n---\n\n# {name}\n",
                                         encoding="utf-8")
        if raw is not None:
            (folder / DECLARATION).write_bytes(raw if isinstance(raw, bytes) else raw.encode("utf-8"))
        elif declaration is not None:
            (folder / DECLARATION).write_text(json.dumps(declaration), encoding="utf-8")
        return folder / "SKILL.md"

    # --- ha -----------------------------------------------------------------

    def ha(self, *args, env=None, timeout=300, cwd=None):
        result = run([self.ha_path, *args], cwd or self.root, env or self.env, timeout=timeout)
        return result.returncode, result.stdout.decode(errors="replace"), result.stderr.decode(errors="replace")

    def start(self, *extra, env=None):
        """(exit, stdout, stderr) of the goal's run-rules/4 start."""
        return self.ha("start", GOAL, "--request", REQUEST, "--rules", "run-rules/4", *extra, env=env)

    def estimate(self, env=None):
        """(exit, parsed JSON or None, stdout+stderr) of `ha estimate --format json`.

        The contract prints a JSON object at exit 0 and at exit 1 for a
        shortfall or a needed approval; exit 1 may instead list lint problems
        (`<code>\t<path>\t<message>` lines), as the earlier `ha` does for an
        automatic PLAN. A JSON object or those lint lines are the observation;
        at exit 0 or 1 anything else, including empty output, cannot be read
        and is Unavailable. Output that starts as JSON but does not parse is
        Unavailable at any exit. At other exits, such as a configuration
        refusal, the text is the observation and the report is None.
        """
        code, out, err = self.ha("estimate", GOAL, "--format", "json", env=env)
        try:
            report = json.loads(out)
        except ValueError:
            report = None
            if out.lstrip().startswith(("{", "[")):
                raise Unavailable(f"ha estimate exit {code}: output that starts as JSON does not parse: {out[-500:]}") from None
        if not isinstance(report, dict):
            report = None
            if code == 0 or (code == 1 and not LINT_LINE.search(out)):
                raise Unavailable(f"ha estimate exited {code} with neither a JSON object nor lint problems: {(out + err)[-500:]}")
        return code, report, out + err

    def estimate_md(self, env=None):
        code, out, err = self.ha("estimate", GOAL, env=env)
        return code, out + err

    def records_path(self):
        return self.root / GOAL / "runs.jsonl"

    def records(self):
        path = self.records_path()
        if not path.exists():
            return []
        try:
            return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
        except (OSError, ValueError) as error:
            raise Unavailable(f"runs.jsonl: {error}") from None

    def status(self):
        code, out, err = self.ha("status", GOAL, "--format", "json")
        try:
            return code, json.loads(out)
        except ValueError:
            raise Unavailable(f"ha status --format json: exit {code}: {(out + err)[-1000:]}") from None

    def reset_records(self):
        path = self.records_path()
        if path.exists():
            path.unlink()


def auto_value(minimum):
    """The contract's computed limit: max(50, ceil(minimum * 1.5))."""
    return max(50, (3 * minimum + 1) // 2)
