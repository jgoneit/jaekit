"""Shared fixture for the ha-report-doctor observers.

Every observation builds `ha` from the checked tree and runs that executable
in a synthetic Git repository that the case creates in a temporary
directory. The repository holds one goal under docs/specs/demo. Its change
conditions run a small synthetic producer (checks/produce.py) that reads a
source file; its maintain conditions run small shell scripts. Commit dates,
author and committer are fixed, global and system Git configuration is not
read, and automatic Git maintenance is off.

Results:
- A requirement that is not met fails with exactly one `[violation:<id>]`.
  `ha report` or `ha doctor` ending with "unknown command" (64) is the
  declared form of a missing implementation: it is the violation of the case,
  because the requirement is the behavior of that command.
- A tool that cannot start (go, git, sh, python3), a build that fails, a
  fixture step that does not end as the fixture expects, a tool ended by a
  signal and undecodable output raise Unavailable: an execution error, never
  a violation. One exception: `ha doctor`'s own output is the observed
  property, so observe_doctor.py decodes it with replacement and a tail cut
  inside a character is that case's violation.

Expected lines are built from PLAN `## 불변식과 판정 규칙` and `### 구현 결정`
with values read from `ha status --format json` of the same inputs and from
the fixture's own construction, never from `ha report` output.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent
GOAL = "docs/specs/demo"
REQUEST = "합성 목표를 구현해줘"
LABELS = {"en": ["Changed", "Checked", "To review", "Remaining limits", "Next step"],
          "ko": ["바뀐 것", "확인한 것", "직접 볼 것", "남은 한계", "다음 행동"]}
HEADINGS = {"en": ["### Stamp for `## 현재`", "### Evidence per condition", "### Completion coverage",
                   "### Output of unmet conditions"],
            "ko": ["### `## 현재` 도장", "### 조건별 근거", "### 완료 범위", "### 미충족 조건의 원문 위치"]}
AVAILABLE, UNAVAILABLE = "사용 가능", "사용 불가"
REASONS = ("실행 파일 없음", "실행 권한 없음", "시간 초과", "0이 아닌 종료", "신호로 종료")
FEATURES = ("report/v1", "doctor/v1")
SECRET = "SECRET-OUTPUT-TOKEN-4f2a"


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def run(argv, cwd, env=None, timeout=600, allow_signal=False):
    """Run a tool. A tool that cannot start or times out is unavailable, never a violation."""
    try:
        result = subprocess.run([str(a) for a in argv], cwd=cwd, env=env, capture_output=True, timeout=timeout,
                                stdin=subprocess.DEVNULL)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unavailable(f"{argv[0]}: {error}") from None
    if result.returncode < 0 and not allow_signal:
        raise Unavailable(f"{argv[0]} ended by signal {-result.returncode}")
    return result


def text(data):
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise Unavailable(f"output is not UTF-8: {error}") from None


def base_env(home, extra=None):
    """The caller's environment without Git, ha evidence or Go selection settings."""
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("GIT_", "HA_")) and k not in ("GOFLAGS", "GOOS", "GOARCH")}
    env.update(HOME=str(home), GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull, LC_ALL="C",
               GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
               GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid")
    env.update(extra or {})
    return env


def build(out, goos=None, goarch=None):
    """`ha` built from the checked tree."""
    out = Path(out)
    out.parent.mkdir(parents=True, exist_ok=True)
    env = {k: v for k, v in os.environ.items() if not k.startswith("HA_")}
    env.update(GOTOOLCHAIN="local", GOFLAGS="-mod=readonly", CGO_ENABLED="0")
    if goos:
        env.update(GOOS=goos, GOARCH=goarch)
    result = run(["go", "build", "-o", out, "./cmd/ha"], ROOT, env, timeout=600)
    if result.returncode:
        raise Unavailable(f"go build: {result.stderr.decode(errors='replace')[-2000:]}")
    return out


class Temporary:
    """A temporary directory removed on exit, also when a case fails."""

    def __init__(self, prefix):
        self.path = Path(tempfile.mkdtemp(prefix=prefix)).resolve()

    def __enter__(self):
        return self.path

    def __exit__(self, *exc):
        def writable(function, path, _):
            os.chmod(path, stat.S_IRWXU)
            function(path)
        shutil.rmtree(self.path, onerror=writable)
        return False


PRODUCER = r'''import json, os, sys
target = sys.argv[1]
with open(os.path.join("src", target + ".txt"), encoding="utf-8") as f:
    state = f.read().strip()
print("observed", target, state, "SECRET-OUTPUT-TOKEN-4f2a")
observation = {"target": "state", "attempt": 1, "status": "pass"}
if state != "done":
    observation.update(status="violation", violation="not-done")
path = os.environ.get("HA_EVIDENCE_PATH")
if path:
    report = {"schema": "check-result/v1", "producer": "synthetic", "producer_version": "1",
              "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
              "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
              "attempts_complete": True, "observations": [observation]}
    with open(path, "w", encoding="utf-8") as f:
        json.dump(report, f)
sys.exit(0 if observation["status"] == "pass" else 1)
'''
# A maintain check fails when its source file changes or, without any code
# change, when a flag file exists in the fixture's HOME.
MAINTAIN = ('echo "checked {id} SECRET-OUTPUT-TOKEN-4f2a"\ntest "$(cat src/{id}.txt)" = ok\n'
            'test ! -e "$HOME/break-{id}"\n')


class Condition:
    """One condition: ID, kind (change, maintain, manual) and whether it is optional."""

    def __init__(self, id, kind, optional=False):
        self.id, self.kind, self.optional = id, kind, optional

    def row(self):
        if self.kind == "change":
            return (f"| {self.id} | change | `python3 checks/produce.py {self.id}` | "
                    f"checks/produce.py, checks/{self.id}.json | T001 |")
        if self.kind == "maintain":
            return f"| {self.id} | maintain | `sh tests/{self.id}.sh` | — | T001 |"
        return f"| {self.id} | manual | — 사람이 결과를 본다 | — | T001 |"


DEFAULT = (Condition("AC-1", "change"), Condition("AC-2", "maintain"))


class Repo:
    """A synthetic repository with one goal at docs/specs/demo."""

    def __init__(self, root, ha, conditions=DEFAULT, runs=50, elapsed="4시간", plan_extra="",
                 track_bundle=False, tests_glob="tests/never/**"):
        self.root, self.ha, self.conditions = Path(root), Path(ha), list(conditions)
        self.root.mkdir(parents=True, exist_ok=True)
        self.home = self.root.parent / (self.root.name + "-home")
        self.home.mkdir(exist_ok=True)
        self.tick = 0
        self.runs, self.elapsed, self.plan_extra, self.tests_glob = runs, elapsed, plan_extra, tests_glob
        self.git("init", "-q", "-b", "main")
        for key, value in (("gc.auto", "0"), ("maintenance.auto", "false"), ("core.autocrlf", "false")):
            self.git("config", key, value)
        files = {".gitattributes": "* -text\n", "checks/produce.py": PRODUCER}
        if not track_bundle:
            files[".gitignore"] = "/docs/\n"
        for c in self.conditions:
            if c.kind == "change":
                files[f"src/{c.id}.txt"] = "todo\n"
                files[f"checks/{c.id}.json"] = json.dumps({
                    "schema": "check-declaration/v1", "producer": "synthetic", "producer_version": "1",
                    "producer_paths": ["checks/produce.py"],
                    "targets": [{"id": "state", "violation": "not-done"}]}) + "\n"
            elif c.kind == "maintain":
                files[f"src/{c.id}.txt"] = "ok\n"
                files[f"tests/{c.id}.sh"] = MAINTAIN.format(id=c.id)
        for path, content in files.items():
            self.write(path, content)
        self.write_goal()
        self.commit("synthetic base")

    # -- files and Git --------------------------------------------------------
    def env(self, extra=None):
        return base_env(self.home, extra)

    def write(self, rel, content):
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content.encode("utf-8") if isinstance(content, str) else content)

    def read(self, rel):
        return (self.root / rel).read_text(encoding="utf-8")

    def git(self, *args):
        result = run(["git", *args], self.root, self.env(), timeout=120)
        if result.returncode:
            raise Unavailable(f"git {' '.join(args)}: {result.stderr.decode(errors='replace')}")
        return text(result.stdout)

    def commit(self, message):
        self.tick += 1
        date = f"2026-01-01T00:{self.tick // 60:02d}:{self.tick % 60:02d}Z"
        self.git("add", "-A")
        result = run(["git", "commit", "-q", "--allow-empty", "-m", message], self.root,
                     self.env({"GIT_AUTHOR_DATE": date, "GIT_COMMITTER_DATE": date}), timeout=120)
        if result.returncode:
            raise Unavailable(f"git commit: {result.stderr.decode(errors='replace')}")
        return self.git("rev-parse", "HEAD").strip()

    def head(self):
        return self.git("rev-parse", "HEAD").strip()

    def flag(self, condition, present=True):
        """Make a maintain check fail (or pass again) without changing code."""
        path = self.home / f"break-{condition}"
        if present:
            path.write_text("break\n", encoding="utf-8")
        elif path.exists():
            path.unlink()

    # -- goal documents ---------------------------------------------------------
    def spec(self):
        lines = []
        for c in self.conditions:
            if c.id.startswith("AC-"):
                marker = " (선택)" if c.optional else ""
                lines.append(f"- **{c.id}**{marker} The synthetic condition {c.id} holds.")
        return ("# Demo\n\nStatus: Ready\n\n## 목표\n\nA synthetic goal.\n\n"
                "## 범위와 비목표\n\n- 범위: `src/`, `checks/`, `tests/`\n- 비목표: 없음\n\n"
                "## Acceptance Criteria\n" + "\n".join(lines) + "\n\n## Open Decisions\n\n없음\n")

    def plan(self, extra=None):
        rows = "\n".join(c.row() for c in self.conditions)
        contract = "\n".join(f"| {c.id} | checks/{c.id}.json |" for c in self.conditions if c.kind == "change")
        ids = ", ".join(c.id for c in self.conditions)
        result = ("# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n"
                  f"{rows}\n\n")
        if contract:
            result += f"## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n{contract}\n\n"
        result += ("## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/**`, `checks/**` (SPEC: 범위)\n"
                   f"- 테스트 경로: `{self.tests_glob}`\n- 유지할 동작: 없음\n\n"
                   "## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n"
                   f"| T001 | 조건이 성립한다 | {ids} | — | — | 예 | — |\n\n"
                   f"## 예산\n- 검증 실행: {self.runs}\n- 경과 시간: {self.elapsed}\n- 비용: 미관측\n")
        extra = self.plan_extra if extra is None else extra
        return result + (("\n" + extra) if extra else "")

    def write_goal(self, plan=None):
        self.write(f"{GOAL}/SPEC.md", self.spec())
        self.write(f"{GOAL}/PLAN.md", self.plan() if plan is None else plan)
        self.write(f"{GOAL}/REVIEW.md", "# REVIEW — demo\n\n## 결론\n\nready\n")
        self.write(f"{GOAL}/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n"
                                          "| --- | --- | --- |\n| T001 | done | |\n")

    # -- ha ---------------------------------------------------------------------
    def ha_run(self, *args, cwd=None, timeout=600, env=None, allow_signal=False):
        result = run([self.ha, *args], cwd or self.root, env or self.env(), timeout=timeout, allow_signal=allow_signal)
        return result.returncode, text(result.stdout), text(result.stderr)

    def start(self, rules="run-rules/4", extra=()):
        args = ["start", GOAL, "--request", REQUEST, "--skill-name", "seal", "--skill-version", "0.1.17"]
        if rules:
            args += ["--rules", rules]
        code, out, err = self.ha_run(*args, *extra)
        if code:
            raise Unavailable(f"ha start: {code} {out}{err}")
        return out

    def implement(self, *ids):
        for i in ids or [c.id for c in self.conditions if c.kind == "change"]:
            self.write(f"src/{i}.txt", "done\n")
        return self.commit("implement")

    def check(self, *ids, baseline=False, expect=None):
        args = ["check", GOAL, *ids] + (["--baseline"] if baseline else [])
        code, out, err = self.ha_run(*args)
        if expect is not None and code != expect:
            raise Unavailable(f"ha {' '.join(args)}: {code} {out}{err}")
        return code, out

    def note(self, *args):
        code, out, err = self.ha_run("note", GOAL, *args)
        if code:
            raise Unavailable(f"ha note {' '.join(args)}: {code} {out}{err}")
        return out

    def done(self):
        code, out, err = self.ha_run("done", GOAL)
        if code not in (0, 1, 2, 3, 4):
            raise Unavailable(f"ha done: {code} {out}{err}")
        return code, out

    def status(self):
        code, out, err = self.ha_run("status", GOAL, "--format", "json")
        if code not in (0, 1, 2, 3, 4):
            raise Unavailable(f"ha status: {code} {err}")
        try:
            return json.loads(out)
        except ValueError:
            raise Unavailable("ha status printed invalid JSON") from None

    def status_md(self):
        code, out, err = self.ha_run("status", GOAL)
        if code not in (0, 1, 2, 3, 4):
            raise Unavailable(f"ha status: {code} {err}")
        return out

    def report(self, *args, cwd=None):
        return self.ha_run("report", GOAL, *args, cwd=cwd)

    def doctor(self, cwd=None, timeout=600, env=None):
        return self.ha_run("doctor", GOAL, cwd=cwd, timeout=timeout, env=env)

    # -- the record -------------------------------------------------------------
    def records_path(self):
        return self.root / GOAL / "runs.jsonl"

    def record_bytes(self):
        path = self.records_path()
        return path.read_bytes() if path.exists() else None

    def lines(self):
        data = self.record_bytes()
        if not data:
            return []
        try:
            return [json.loads(line) for line in data.decode("utf-8").splitlines()]
        except ValueError:
            raise Unavailable("runs.jsonl is not JSON lines") from None

    def rewrite(self, change):
        """Apply change(lines) to the decoded record and chain prev hashes again."""
        lines = change(self.lines())
        out, prev = [], None
        for line in lines:
            line["prev"] = prev
            raw = json.dumps(line, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            out.append(raw)
            prev = hashlib.sha256(raw).hexdigest()
        self.records_path().write_bytes(b"".join(r + b"\n" for r in out))

    def set_rules(self, rules):
        def change(lines):
            for line in lines:
                if line.get("kind") == "start":
                    line["rules"] = rules
                    line.pop("scope", None)
                    line.pop("test_authority", None)
            return lines
        self.rewrite(change)

    def snapshot(self):
        """Every path under the repository (Git directory included) with its mode and content digest."""
        found = {}
        for path in sorted(self.root.rglob("*")):
            rel = str(path.relative_to(self.root))
            mode = path.lstat().st_mode
            if stat.S_ISREG(mode):
                found[rel] = (stat.S_IMODE(mode), hashlib.sha256(path.read_bytes()).hexdigest())
            elif stat.S_ISLNK(mode):
                found[rel] = ("link", os.readlink(path))
            else:
                found[rel] = ("dir", stat.S_IMODE(mode))
        return found

    def contains(self, needle):
        """Paths under the repository whose bytes contain needle."""
        hits = []
        for path in self.root.rglob("*"):
            if path.is_file() and not path.is_symlink() and needle.encode() in path.read_bytes():
                hits.append(str(path.relative_to(self.root)))
        return sorted(hits)


def snapshot_diff(before, after):
    changed = sorted(k for k in set(before) | set(after) if before.get(k) != after.get(k))
    return changed[:10]


# -- expected report lines (PLAN 구현 결정) ---------------------------------------

def short(value):
    return value[:12]


def display_goal():
    return GOAL


def summary_lines(out):
    return out.split("\n")[:5]


def stored_criteria(repo, status):
    """The criteria of the evidence source (R1)."""
    seq = status.get("completion_record")
    if seq is not None:
        for line in repo.lines():
            if line.get("seq") == seq and line.get("kind") == "done":
                return [{"id": c["id"], "kind": c.get("kind", ""), "satisfied": c["satisfied"],
                         "records": c.get("records") or []} for c in line.get("criteria", [])], seq
        raise Unavailable(f"completion record seq {seq} is not in the record")
    return [{"id": c["id"], "kind": c["kind"], "satisfied": c["satisfied"], "records": c["records"]}
            for c in status["criteria"]], None


def required_ids(repo):
    return {c.id for c in repo.conditions if c.id.startswith("AC-") and not c.optional}


def expect_summary(repo, status, lang, changed, fresh_baselines, confirmations=None, blocked=None):
    """The five summary lines for lang.

    changed is ("counts", files, commits), ("not_started",) or ("cannot_compare", base, head).
    fresh_baselines is (B, N); confirmations maps manual IDs to confirmation seqs;
    blocked is (seq, cause) of the last block note.
    """
    ko = lang == "ko"
    goal = display_goal()
    lines = []
    start = next((l for l in repo.lines() if l.get("kind") == "start"), None)
    if changed[0] == "not_started":
        lines.append("바뀐 것: 시작 전이라 base commit이 없음" if ko else "Changed: not started; there is no base commit yet")
    elif changed[0] == "cannot_compare":
        _, base, head = changed
        lines.append(f"바뀐 것: Git이 base {short(base)}와 HEAD {short(head)}를 비교할 수 없음" if ko else
                     f"Changed: Git cannot compare base {short(base)} with HEAD {short(head)}")
    else:
        _, files, commits = changed
        base = short(start["base_commit"])
        lines.append(f"바뀐 것: base {base} 이후 실행 묶음 문서 밖에서 파일 {files}개, commit {commits}개" if ko else
                     f"Changed: {files} files and {commits} commits outside the bundle documents since base {base}")
    criteria, source = stored_criteria(repo, status)
    req = required_ids(repo)
    required = [c for c in criteria if c["id"] in req]
    satisfied = [c for c in required if c["satisfied"]]
    by_check = sum(1 for c in satisfied if c["kind"] in ("change", "maintain"))
    by_user = sum(1 for c in satisfied if c["kind"] == "manual")
    b, n = fresh_baselines
    lines.append(f"확인한 것: 필수 조건 {len(required)}개 중 {len(satisfied)}개 충족(검사 기록 {by_check}개, 사용자 확인 {by_user}개); "
                 f"change 조건 {n}개 중 {b}개에 신선한 baseline" if ko else
                 f"Checked: {len(satisfied)} of {len(required)} required conditions satisfied ({by_check} by check records, "
                 f"{by_user} by user confirmation); {b} of {n} change conditions have a fresh baseline")
    items = []
    user = [r for r in status["reasons"] if r["class"] == "user"]
    if user:
        items.append(("사용자 결정: " if ko else "user decisions: ") + reason_list(user))
    manual = [c for c in criteria if c["kind"] == "manual" and c["satisfied"]]
    if manual:
        parts = [f"{c['id']} (seq {(confirmations or {})[c['id']]})" for c in manual]
        items.append(("인용 확인으로 충족한 manual 조건: " if ko else "manual conditions satisfied by a quoted confirmation: ")
                     + ", ".join(parts))
    exempt = [t for t in status.get("test_changes") or [] if t.get("exempt")]
    if exempt:
        items.append(("실행자 근거로 면제된 시험 변경: " if ko else "test changes exempt by the executor's reason: ")
                     + ", ".join(f"{t['path']} (seq {t['rationale']['seq']})" for t in exempt))
    weakened = [t for t in status.get("test_changes") or [] if (t.get("rationale") or {}).get("weakened")]
    if weakened:
        items.append(("약화 표시가 있는 시험 변경: " if ko else "test changes marked as weakening: ")
                     + ", ".join(f"{t['path']} (seq {t['rationale']['seq']})" for t in weakened))
    commands = status["changes"]["command_changed"]
    if commands:
        items.append(("시작 뒤 바뀐 검증 명령: " if ko else "verification commands changed since start: ")
                     + ", ".join(f"{c['criterion']} (seq {c['seq']})" for c in commands))
    unresolved = (status.get("findings") or {}).get("unresolved") or 0
    if unresolved:
        items.append(f"해결되지 않은 완료 뒤 발견: {unresolved}개" if ko else f"unresolved post-completion findings: {unresolved}")
    if items:
        lines.append(("직접 볼 것: " if ko else "To review: ") + "; ".join(items))
    else:
        lines.append("직접 볼 것: 없음" if ko else "To review: none")
    limits = "남은 한계: 보증 수준 local; 검사 작성자: 실행자" if ko else "Remaining limits: assurance local; check author: executor"
    optional = [c["id"] for c in criteria if c["id"] not in req and not c["satisfied"]]
    if optional:
        limits += ("; 충족되지 않은 선택 조건: " if ko else "; optional conditions not satisfied: ") + ", ".join(optional)
    budget = status.get("budget")
    if budget and budget.get("exceeded"):
        r, rl, e, el = budget["runs"], budget["runs_limit"], budget["elapsed_seconds"], budget["elapsed_limit_seconds"]
        limits += (f"; 예산 초과: 실행 {r}/{rl}회, 경과 {e}/{el}초" if ko else f"; budget exceeded: {r}/{rl} runs, {e}/{el} seconds")
    if status.get("completion_record") is None:
        limits += "; 지금 HEAD를 덮는 완료 기록이 없음" if ko else "; HEAD is not covered by a completion record"
    lines.append(limits)
    state = status["status"]
    if start is None:
        line = f"다음 행동: `ha start {goal}` 실행" if ko else f"Next step: run `ha start {goal}`"
        if status["lint"]:
            line += f" (먼저 lint 문제 해결: `ha lint {goal}`)" if ko else f" after fixing the lint problems (`ha lint {goal}`)"
    elif state == "incomplete":
        first = next(r for r in status["reasons"] if r["class"] == "executor")
        target = (first["criterion"] + " ") if first.get("criterion") else ""
        line = (f"다음 행동: {target}`{first['code']}` 해결 (executor)" if ko else
                f"Next step: resolve {target}`{first['code']}` (executor)")
    elif state == "needs_user":
        line = ("다음 행동: 사용자에게 물을 것: " if ko else "Next step: ask the user about ") + reason_list(user)
    elif state == "complete" and status.get("completion_record") is None:
        line = f"다음 행동: `ha done {goal}`으로 완료 기록" if ko else f"Next step: run `ha done {goal}` to record completion"
    elif state == "complete":
        seq = status["completion_record"]
        line = (f"다음 행동: 없음; 완료 기록 seq {seq}이 유효하고 남은 할 일이 없음" if ko else
                f"Next step: none; completion record seq {seq} holds and nothing is left to do")
    elif state == "blocked":
        seq, cause = blocked
        line = (f"다음 행동: seq {seq}부터 막힘 (cause: {cause}); 원인을 해소해야 진행할 수 있음" if ko else
                f"Next step: blocked since seq {seq} (cause: {cause}); the cause must be resolved before work continues")
    else:
        r, rl, e, el = budget["runs"], budget["runs_limit"], budget["elapsed_seconds"], budget["elapsed_limit_seconds"]
        line = (f"다음 행동: 사용자의 예산 결정이 필요함; 실행 {r}/{rl}회, 경과 {e}/{el}초 사용" if ko else
                f"Next step: the user must decide the budget; used {r}/{rl} runs and {e}/{el} seconds")
    lines.append(line)
    return lines


def reason_list(reasons):
    return ", ".join((f"{r['criterion']} `{r['code']}`" if r.get("criterion") else f"`{r['code']}`") for r in reasons)


def sections(out):
    """The report's detail sections keyed by heading, in order. Any non-blank line
    between the summary and the first heading is kept under the heading None, so
    it never matches an expected section list."""
    lines = out.split("\n")[5:]
    first = next((i for i, l in enumerate(lines) if l.startswith("### ")), len(lines))
    lead = [l for l in lines[:first] if l.strip()]
    found = []
    for line in lines[first:]:
        if line.startswith("### "):
            found.append([line, []])
        else:
            found[-1][1].append(line)
    return ([(None, lead)] if lead else []) + [(heading, [l for l in body if l.strip()]) for heading, body in found]


def expect_stamp(repo, status, lang):
    ko = lang == "ko"
    lines = repo.lines()
    if not lines:
        return f"- 기록 head: none; 상태: {status['status']}" if ko else f"- record head: none; status: {status['status']}"
    head = status["record_head"]
    at = lines[-1]["at"]
    line = (f"- 기록 head: seq {head['seq']} ({short(head['sha256'])}), 기록 시각 {at}; 상태: {status['status']}" if ko else
            f"- record head: seq {head['seq']} ({short(head['sha256'])}), recorded at {at}; status: {status['status']}")
    if status.get("completion_record") is not None:
        line += (f"; 완료 기록: seq {status['completion_record']}" if ko else
                 f"; completion record: seq {status['completion_record']}")
    return line


def expect_table(repo, status, lang):
    ko = lang == "ko"
    criteria, source = stored_criteria(repo, status)
    req = required_ids(repo)
    yes, no = ("예", "아니오") if ko else ("yes", "no")
    if source is None:
        head = "근거 출처: 지금 상태" if ko else "Source: current state"
    else:
        head = f"근거 출처: 완료 기록 seq {source}" if ko else f"Source: completion record seq {source}"
    rows = [head, "| ID | 종류 | 필수 | 충족 | 근거 기록 |" if ko else "| ID | kind | required | satisfied | records |",
            "| --- | --- | --- | --- | --- |"]
    for c in criteria:
        records = ", ".join(str(s) for s in c["records"]) or "—"
        rows.append(f"| {c['id']} | {c['kind'] or '—'} | {yes if c['id'] in req else no} | "
                    f"{yes if c['satisfied'] else no} | {records} |")
    return rows


# The fixed Korean wording of `ha status`'s reasons why a completion record
# no longer holds (PLAN 구현 결정), by English prefix.
STALE_KO = (("record chain broken", "기록 연결이 끊김"),
            ("stored records are invalid", "저장된 기록이 무효함"),
            ("working tree has uncommitted changes outside the bundle documents", "실행 묶음 문서 밖에 커밋하지 않은 변경이 있음"),
            ("that commit cannot be compared with HEAD", "그 commit을 HEAD와 비교할 수 없음"),
            ("code outside the bundle documents changed since that commit", "그 commit 뒤 실행 묶음 문서 밖의 코드가 바뀜"),
            ("goal documents changed", "목표 문서가 바뀜"),
            ("required command changed: ", "필수 조건의 명령이 바뀜: "),
            ("reopened at seq ", "재작업 요청 seq "),
            ("required conditions are not all satisfied", "필수 조건이 모두 충족되지는 않음"))
STALE = re.compile(r"latest completion seq (\d+) \(commit ([0-9a-f]+)\) is no longer valid: (.*?)(?:; run `ha done [^`]*` to record completion)?$")


def stale_completion(repo):
    """(seq, commit12, reasons) of `ha status`'s no-longer-valid completion, or None."""
    for line in repo.status_md().splitlines():
        if line.startswith("- completion record: none"):
            match = STALE.search(line)
            if match:
                reasons = [r for r in match.group(3).split("; ") if r]
                return int(match.group(1)), match.group(2), reasons
    return None


def translate_stale(reason):
    for english, korean in STALE_KO:
        if reason.startswith(english):
            return korean + reason[len(english):]
    raise Unavailable(f"unknown status reason {reason!r}")


def expect_coverage(repo, status, lang):
    ko = lang == "ko"
    head = short(status["commit"])
    seq = status.get("completion_record")
    if seq is not None:
        done = next(l for l in repo.lines() if l.get("seq") == seq)
        return (f"- 완료 기록 seq {seq}, commit {short(done['commit'])}; 지금 HEAD {head}를 덮음" if ko else
                f"- completion record seq {seq}, commit {short(done['commit'])}; HEAD {head} is covered")
    stale = stale_completion(repo)
    if stale is None:
        return "- 완료 기록: none" if ko else "- completion record: none"
    seq, commit, reasons = stale
    if ko:
        why = (": " + "; ".join(translate_stale(r) for r in reasons)) if reasons else ""
        return f"- 가장 최근 완료 기록 seq {seq}, commit {commit}; 지금 HEAD {head}를 덮지 않음{why}; 그 commit 뒤의 차이는 완료에 들지 않음"
    why = (": " + "; ".join(reasons)) if reasons else ""
    return (f"- latest completion record seq {seq}, commit {commit}; HEAD {head} is not covered{why}; "
            "the difference since that commit is not part of the completion")


def expect_unmet(repo, lang):
    lines = unmet_status_lines(repo)
    if lang == "ko":
        lines = [l.replace(", output `ha log ", ", 원문 `ha log ").replace(": no fresh record", ": 신선한 기록 없음")
                 for l in lines]
    return lines or (["- 없음"] if lang == "ko" else ["- none"])


def expect_details(repo, status, lang):
    """The four detail sections as (heading, body lines)."""
    return list(zip(HEADINGS[lang], [[expect_stamp(repo, status, lang)], expect_table(repo, status, lang),
                                     [expect_coverage(repo, status, lang)], expect_unmet(repo, lang)]))


def unmet_status_lines(repo):
    """The condition lines of `ha status`'s `### Unmet` summary."""
    out = repo.status_md()
    if "### Unmet" not in out:
        return []
    body = out.split("### Unmet", 1)[1].split("\n### ", 1)[0]
    return [l for l in body.splitlines() if re.match(r"- (AC|EX)-\d+ `", l)]
