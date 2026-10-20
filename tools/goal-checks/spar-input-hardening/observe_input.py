"""Observations for the goal "Spar input hardening": the one input table of
Spar's public contract and every consumer of each row.

Every observation reads the checked tree only. Runtime cases build `spar`
from the checked tree with `go build` into a temporary directory and run it
against a synthetic Git repository that the case creates itself. Reviewer
CLIs (claude, codex, agy and command reviewers) are synthetic fakes written by
this file: they never call a model and only follow a control file. HOME,
XDG_CONFIG_HOME and the clean-review locations point into the temporary
directory, so the user's own files are never read or written.

The representative forms and their results come from one table,
internal/spar/testdata/input-forms.json, which the Go tests of internal/spar
read too. Every consumer of a row is checked against the same cases.

Results (declared before the plan review; see the goal's REVIEW.md):
- A requirement that is not met fails with exactly one `[violation:<id>]`
  and ends the test. Each test has one violation id.
- Records, summaries, packets and contract files that `spar` or the checked
  tree must hold, and that are missing or not the documented format after a
  command ended normally, are observed behavior: the test's violation.
- A missing or unusable tool (go, git, python, sh, mkfifo), a build that
  fails, an unreadable or malformed forms table, a `spar` that panics, is
  ended by a signal or exceeds its time limit raise Unavailable: an
  execution error, never a violation.
"""
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[3]
FORMS_PATH = ROOT / "internal/spar/testdata/input-forms.json"
CONTRACT = "contracts/spar-input.md"
ANGLES = ["requirement-counterexample", "side-effects", "design", "premises", "check-validity"]
QUOTE = "합성 사용자: 이 지적은 이번 변경에서 다룬다"
TABLE_HEADER = "| 행 | 필드 (주는 쪽) | 받는 형식과 정규화 | 형식 밖의 결과 | 소비처 |"
UNREADABLE_MARK = "unreadable location"
UNLISTED = "is not in the gate list"


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def run(argv, cwd=None, env=None, timeout=300, input_bytes=None):
    try:
        result = subprocess.run([str(a) for a in argv], cwd=cwd, env=env, capture_output=True, timeout=timeout,
                                input=input_bytes, stdin=None if input_bytes is not None else subprocess.DEVNULL)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unavailable(f"{argv[0]}: {error}") from None
    if result.returncode < 0:
        raise Unavailable(f"{argv[0]} ended by signal {-result.returncode}")
    return result


def text(result):
    return (result.stdout + result.stderr).decode(errors="replace")


_STARTED = set()


def tool(name):
    """A program the observation runs: on PATH and able to start once with
    --version (any exit status means it started)."""
    path = shutil.which(name)
    if not path:
        raise Unavailable(f"{name} is not on PATH")
    if path not in _STARTED:
        run([path, "--version"], env=dict(os.environ, GOTOOLCHAIN="local"), timeout=20)
        _STARTED.add(path)
    return path


_BUILT = {}


def spar_binary():
    if "spar" not in _BUILT:
        out = Path(tempfile.mkdtemp(prefix="jaekit-spar-input-build-")) / "spar"
        env = dict(os.environ, CGO_ENABLED="0", GOFLAGS="-mod=readonly", GOTOOLCHAIN="local")
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        result = run([tool("go"), "build", "-o", out, "./cmd/spar"], ROOT, env, timeout=900)
        if result.returncode:
            raise Unavailable(f"go build ./cmd/spar: {text(result)[-2000:]}")
        _BUILT["spar"] = out
    return _BUILT["spar"]


_FORMS = {}


def forms():
    """The representative forms: a check input, so a missing or malformed
    table is an execution error."""
    if "forms" not in _FORMS:
        try:
            data = json.loads(FORMS_PATH.read_text(encoding="utf-8"))
        except (OSError, UnicodeDecodeError, ValueError) as error:
            raise Unavailable(f"the forms table cannot be read: {error}") from None
        if not isinstance(data, dict) or data.get("schema") != "spar-input-forms/v1":
            raise Unavailable("the forms table is not spar-input-forms/v1")
        _FORMS["forms"] = data
    return _FORMS["forms"]


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read_doc(case, rel, violation):
    """A document the checked tree must hold; its absence is the violation."""
    path = ROOT / rel
    require(case, path.is_file(), violation, f"{rel} is missing")
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"{rel} cannot be read: {error}") from None


def load_json(case, path, violation, what):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except FileNotFoundError:
        case.fail(f"[violation:{violation}] {what} is missing: {path}")
    except (UnicodeDecodeError, ValueError) as error:
        case.fail(f"[violation:{violation}] {what} is not JSON: {error}")


def read_lines(case, path, violation):
    path = Path(path)
    if not path.exists():
        return []
    try:
        return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
    except (UnicodeDecodeError, ValueError) as error:
        case.fail(f"[violation:{violation}] {path.name} is not JSON lines: {error}")


FAKE = r'''
import hashlib, json, os, sys, time
from pathlib import Path

name = os.path.basename(sys.argv[0])
if len(sys.argv) > 1 and sys.argv[1] == "--fake-name":
    name = sys.argv[2]
    args = sys.argv[3:]
else:
    args = sys.argv[1:]
control = json.loads(Path(os.environ["SPAR_FAKE_CONTROL"]).read_text())
spec = dict(control.get("default", {}))
spec.update(control.get("reviewers", {}).get(name, {}))
if args[:1] == ["--version"]:
    time.sleep(float(spec.get("version_sleep", 0)))
    print(f"{name} 9.9.9 (synthetic)")
    sys.exit(0)
def option(flag):
    return args[args.index(flag) + 1] if flag in args else None
if name == "claude":
    model, prompt = option("--model"), sys.stdin.read()
elif name == "codex":
    model, prompt = option("-m"), sys.stdin.read()
elif name == "agy":
    model, prompt = option("--model"), option("-p")
else:
    model = None
    prompt = Path(option("--prompt")).read_text() if option("--prompt") else ""
entry = {"name": name, "argv": args, "cwd": os.getcwd(), "model": model, "pid": os.getpid(),
         "prompt_sha256": hashlib.sha256(prompt.encode()).hexdigest()}
with open(control["log"], "a") as log:
    log.write(json.dumps(entry) + "\n")
time.sleep(float(spec.get("sleep", 0)))
result = spec.get("result")
if result is None:
    result = {"reviewed_head": control["head"], "verdict": "no_blocking_findings",
              "angles_checked": [{"angle": a, "result": "checked"} for a in control.get("angles", [])],
              "angles_unchecked": [], "findings": spec.get("findings", []), "limits": "synthetic"}
for key, value in spec.get("top", {}).items():
    result[key] = value
for key, value in spec.get("first_finding", {}).items():
    result["findings"][0][key] = value
for key in spec.get("drop_top", []):
    result.pop(key, None)
stderr = spec.get("stderr")
if stderr:
    sys.stderr.write(stderr + "\n")
    sys.exit(int(spec.get("exit", 1)))
if name == "claude":
    print(json.dumps({"type": "system", "subtype": "init", "model": model,
                      "tools": ["Read", "Grep", "Glob", "StructuredOutput"], "mcp_servers": []}), flush=True)
    print(json.dumps({"type": "result", "subtype": "success", "is_error": False, "structured_output": result, "usage": {}}))
elif name == "codex":
    print(json.dumps({"type": "thread.started"}), flush=True)
    Path(option("-o")).write_text(json.dumps(result))
    print(json.dumps({"type": "item.completed", "item": {"type": "agent_message"}}))
    print(json.dumps({"type": "turn.completed", "usage": {}}))
elif name == "agy":
    print(json.dumps({"init": {"model": model}}), flush=True)
    for line in spec.get("commands", []):
        info = {"parameters": {"CommandLine": line}}
        print(json.dumps({"step_update": {"step_type": "tool", "tool_name": "run_command", "tool_info": info}}), flush=True)
    print(json.dumps({"result": {"status": "success", "structured_output": result, "usage": {}}}))
else:
    print(json.dumps(result))
'''


def finding(fid, **values):
    item = {"id": fid, "proposed_class": "defect", "angle": "requirement-counterexample",
            "location": "head/app/main.go:1", "claim": "synthetic claim " + fid,
            "failure_scenario": "a synthetic input gives a wrong result", "evidence": "synthetic evidence",
            "link_kind": "none", "link": "", "introduced_by_change": "no", "regression": "no",
            "check_impact": "none", "confidence": "high"}
    item.update(values)
    return item


def toml_value(value):
    return json.dumps(value, ensure_ascii=False)


def reviewers_toml(*entries, top=None):
    parts = []
    for key, value in (top or {}).items():
        parts.append(f"{key} = {toml_value(value)}")
    for entry in entries:
        parts.append("")
        parts.append("[[reviewers]]")
        for key, value in entry.items():
            parts.append(f"{key} = {toml_value(value)}")
    return "\n".join(parts) + "\n"


class World:
    """A temporary home, a synthetic repository and fake reviewer CLIs."""

    def __init__(self, case, violation):
        self.case = case
        self.violation = violation
        self.spar = spar_binary()
        self.git = tool("git")
        self.tmp = Path(tempfile.mkdtemp(prefix="jaekit-spar-input-")).resolve()
        case.addCleanup(shutil.rmtree, self.tmp, True)
        self.home = self.tmp / "home"
        self.xdg = self.tmp / "xdg"
        self.home.mkdir()
        self.xdg.mkdir()
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        self.fake = self.tmp / "fake_reviewer.py"
        self.fake.write_text(FAKE)
        for name in ("claude", "codex", "agy"):
            self.wrapper(name)
        self.log = self.tmp / "invocations.jsonl"
        self.control_path = self.tmp / "control.json"
        self.repo = self.tmp / "repo"
        self.repo.mkdir()
        self.g("init", "-q")
        self.write(".gitignore", "/docs/\n/reviews/\n")
        self.base = None
        self.head = None
        self.criteria = self.repo / "docs/specs/x/SPEC.md"
        self.write("docs/specs/x/SPEC.md", "# Criteria\n\n- **AC-1** one.\n- **AC-2** two.\n")

    def wrapper(self, name):
        script = self.bin / name
        script.write_text(f"#!/bin/sh\nexec {sys.executable} {self.fake} --fake-name {name} \"$@\"\n")
        script.chmod(0o755)
        return script

    # Repository ---------------------------------------------------------
    def g(self, *args, cwd=None, check=True):
        env = dict(os.environ, GIT_AUTHOR_NAME="fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
                   GIT_COMMITTER_NAME="fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid",
                   GIT_CONFIG_NOSYSTEM="1", HOME=str(self.home))
        result = run([self.git, "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
                      "-c", "protocol.file.allow=always", *args], cwd or self.repo, env)
        if check and result.returncode:
            raise Unavailable(f"git {' '.join(args)}: {text(result)}")
        return result.stdout.decode().strip()

    def write(self, rel, content="line\n"):
        path = self.repo / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def remove(self, rel):
        (self.repo / rel).unlink()

    def commit(self, message="synthetic"):
        self.g("add", "-A")
        self.g("commit", "-qm", message, "--allow-empty")
        return self.g("rev-parse", "HEAD")

    def commits(self, base_files, head_files, head_extra=None):
        """A base commit with base_files and a head commit with head_files."""
        for rel in base_files:
            self.write(rel)
        self.base = self.commit("base")
        for rel in base_files:
            if rel not in head_files:
                self.remove(rel)
        for rel in head_files:
            self.write(rel)
        self.g("add", "-A")
        if head_extra:
            head_extra()  # may stage entries add -A cannot, such as a gitlink
        self.g("commit", "-qm", "head", "--allow-empty")
        self.head = self.g("rev-parse", "HEAD")
        self.control({})

    # Configuration and control --------------------------------------------
    def user_config(self, body):
        path = self.xdg / "jaekit/spar.toml"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
        return path

    def command_reviewer(self, name, **extra):
        entry = {"name": name, "preset": "command",
                 "argv": [sys.executable, str(self.fake), "--fake-name", name, "--prompt", "{prompt}",
                          "--workspace", "{workspace}", "--schema", "{schema}"]}
        entry.update(extra)
        return entry

    def project_config(self, body):
        path = self.repo / ".jaekit/spar.toml"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
        return path

    def control(self, reviewers, **extra):
        data = {"log": str(self.log), "head": extra.pop("head", self.head), "angles": ANGLES, "reviewers": reviewers}
        data.update(extra)
        self.control_path.write_text(json.dumps(data))

    def env(self, **extra):
        env = {key: value for key, value in os.environ.items()
               if not key.startswith(("SPAR_", "CLEAN_REVIEW_", "XDG_", "HA_"))}
        env.update(HOME=str(self.home), XDG_CONFIG_HOME=str(self.xdg), SPAR_FAKE_CONTROL=str(self.control_path),
                   PATH=str(self.bin) + os.pathsep + os.path.dirname(self.git) + os.pathsep + "/usr/bin:/bin")
        for key, value in extra.items():
            if value is None:
                env.pop(key, None)
            else:
                env[key] = str(value)
        return env

    def invocations(self):
        return read_lines(self.case, self.log, self.violation)

    def reset_log(self):
        self.log.unlink(missing_ok=True)

    # Running spar ------------------------------------------------------------
    def spar_run(self, *args, env=None, timeout=300, cwd=None):
        result = run([self.spar, *args], cwd or self.repo, env or self.env(), timeout=timeout)
        out = text(result)
        if result.returncode == 2 and "panic:" in out:
            raise Unavailable(f"spar panicked: {out[-2000:]}")
        return result

    def round(self, out, stage="candidate", base=None, head=None, extra=(), env=None, ok=(0, 1)):
        argv = ["run", "--stage", stage, "--base", base or self.base, "--head", head or self.head,
                "--out", out, "--criteria", self.criteria, *extra]
        result = self.spar_run(*argv, env=env)
        if ok is not None and result.returncode not in ok:
            self.case.fail(f"[violation:{self.violation}] spar run exited {result.returncode}: {text(result)[-1500:]}")
        return result

    def round_json(self, out):
        return load_json(self.case, self.repo / out / "round.json", self.violation, "round.json")

    def findings(self, out):
        data = load_json(self.case, self.repo / out / "findings.json", self.violation, "findings.json")
        require(self.case, isinstance(data, list), self.violation, "findings.json is not a list")
        return data

    def ledger(self, out):
        return read_lines(self.case, self.repo / out / "ledger.jsonl", self.violation)

    def summary(self, out):
        path = self.repo / out / "summary.md"
        require(self.case, path.is_file(), self.violation, f"{out}/summary.md is missing")
        return path.read_text(encoding="utf-8")

    def packet(self, out):
        path = self.repo / out / "packet.md"
        require(self.case, path.is_file(), self.violation, f"{out}/packet.md is missing")
        return path.read_text(encoding="utf-8")

    def gate(self, rounds, out, expect=None):
        before = (self.repo / out).read_bytes() if (self.repo / out).exists() else None
        result = self.spar_run("gate", *rounds, "--expect-head", expect or self.head, "--out", out)
        require(self.case, (self.repo / out).is_file() and (self.repo / out).read_bytes() != before, self.violation,
                f"spar gate exited {result.returncode} without writing {out}: {text(result)[-400:]}")
        data = load_json(self.case, self.repo / out, self.violation, "gate file")
        require(self.case, isinstance(data, dict), self.violation, "the gate file is not an object")
        return result, data

    def reviewer_state(self, out, name):
        return ((self.round_json(out) or {}).get("reviewers") or {}).get(name) or {}

    def native_reply(self, findings, head=None):
        return {"reviewed_head": head or self.head, "verdict": "blocking_findings" if findings else "no_blocking_findings",
                "angles_checked": [{"angle": a, "result": "checked"} for a in ANGLES], "angles_unchecked": [],
                "findings": findings, "limits": "synthetic"}

    def record(self, out, name, reply):
        path = self.tmp / f"reply-{name}-{len(list(self.tmp.glob('reply-*')))}.json"
        path.write_text(json.dumps(reply))
        return self.spar_run("record", out, name, "--result", path)


def by_reviewer_id(found):
    return {f.get("reviewer_id"): f for f in found}


def summary_row(summary, fid):
    for line in summary.splitlines():
        if line.startswith(f"| {fid} |"):
            return line
    return None


# ---------------------------------------------------------------------------
# AC-1: one input table in the public contract
# ---------------------------------------------------------------------------

ROWS = [f"G{n}" for n in range(1, 34)]
# Format definitions the three contracts held before the table; each now
# points to the table instead (fixed tokens).
OLD_FORMATS = {
    "contracts/review-round.md": ["## 10. 지원하는 형식", "| 필드 | 받는 형식 | 경로·ID | 떼는 접두 |",
                                  "`location`은 `head/<경로>:<줄>`"],
    "contracts/spar-config.md": ["## 5. 지원하는 형식", "영문자·숫자로 시작하고 영문자·숫자·`.`·`_`·`-`로 된 64자 이하",
                                 "`**`는 여러 폴더, `*`·`?`·`[...]`는 한 이름 안에서 맞춘다"],
    "contracts/review-gate.md": ["`--out`은 저장소 안이면 Git이 무시하는 경로여야 한다"],
}
# The rows each contract points to for its fields.
POINTERS = {
    "contracts/review-round.md": ["G1", "G5", "G7", "G8", "G9", "G10", "G15", "G26", "G28", "G33"],
    "contracts/spar-config.md": ["G16", "G17", "G18", "G19", "G20", "G21", "G22", "G23", "G24", "G25"],
    "contracts/review-gate.md": ["G28", "G29"],
}


def table_rows(case, body, violation):
    """The rows of the one input table: the header line and the rows under it."""
    lines = body.splitlines()
    require(case, lines.count(TABLE_HEADER) == 1, violation, f"{CONTRACT} must hold the input table header once")
    start = lines.index(TABLE_HEADER) + 2
    rows = {}
    for line in lines[start:]:
        if not line.startswith("|"):
            break
        cells = [c.strip() for c in re.split(r"(?<!\\)\|", line)[1:-1]]
        require(case, len(cells) == 5, violation, f"a table row does not have five cells: {line[:80]}")
        require(case, cells[0] not in rows, violation, f"row {cells[0]} appears twice")
        rows[cells[0]] = cells
    return rows


class Contract(unittest.TestCase):
    def test_one_input_table(self):
        v = "input-table-incomplete"
        body = read_doc(self, CONTRACT, v)
        rows = table_rows(self, body, v)
        require(self, list(rows) == ROWS, v, f"the table rows are {list(rows)}, not G1..G33 in order")
        for row, cells in rows.items():
            require(self, all(cells), v, f"{row} has an empty cell")
            require(self, re.search(r"\(.+\)", cells[1]), v, f"{row} does not name who gives the field")
        for phrase in ("보장하지 않", "보장 밖"):
            require(self, phrase not in body, v, f"{CONTRACT} leaves a result unpromised: {phrase}")
        for rel in ("contracts/README.md", *POINTERS):
            require(self, "spar-input.md" in read_doc(self, rel, v), v, f"{rel} does not point to the input table")
        for rel, pointers in POINTERS.items():
            doc = read_doc(self, rel, v)
            for phrase in ("보장 밖", "보장하지 않"):
                require(self, phrase not in doc, v, f"{rel} still lists formats outside its promise: {phrase}")
            for old in OLD_FORMATS[rel]:
                require(self, old not in doc, v, f"{rel} still defines a format the table owns: {old}")
            for row in pointers:
                require(self, re.search(rf"\b{row}\b", doc), v, f"{rel} does not point to row {row}")


# ---------------------------------------------------------------------------
# AC-2, AC-17, AC-19: locations
# ---------------------------------------------------------------------------

def location_world(case, violation, reviewers):
    """The G7 fixture: head and base copies, check paths and a subject document."""
    table = forms()["locations"]
    w = World(case, violation)
    w.commits(table["base"], table["head"])
    w.write(table["subject"], "# Plan\n" + "step\n" * 20)
    w.project_config(f"check_paths = {toml_value(table['check_paths'])}\n")
    w.user_config(reviewers_toml(*reviewers))
    return w, table


def location_findings(cases, **values):
    return [finding(f"F{i + 1}", location=c["location"], **values) for i, c in enumerate(cases)]


def check_locations(case, w, out, cases, violation, names=None):
    """Each G7 case gives the table's path, check-path judgment and mark in
    findings.json and the summary; no reviewer result is dropped for it."""
    found = by_reviewer_id(w.findings(out))
    summary = w.summary(out)
    for i, c in enumerate(cases):
        f = found.get(f"F{i + 1}")
        require(case, f is not None, violation, f"the finding for {c['location']!r} is missing")
        if "unreadable" in c:
            require(case, f.get("location_unreadable") is True and f.get("path") == "" and f.get("in_check_paths") is False,
                    violation, f"{c['location']!r} ({c['unreadable']}) is not an unreadable location: "
                    f"path {f.get('path')!r}, in_check_paths {f.get('in_check_paths')}, mark {f.get('location_unreadable')}")
            require(case, UNREADABLE_MARK in (f.get("class_reason") or ""), violation,
                    f"{c['location']!r}: class_reason does not say the location is unreadable: {f.get('class_reason')!r}")
        else:
            require(case, f.get("path") == c["path"] and f.get("in_check_paths") is c["in_check_paths"]
                    and f.get("location_unreadable") is False, violation,
                    f"{c['location']!r}: path {f.get('path')!r}, in_check_paths {f.get('in_check_paths')}, "
                    f"mark {f.get('location_unreadable')}; expected {c['path']!r}, {c['in_check_paths']}")
        row = summary_row(summary, f.get("id"))
        require(case, row is not None, violation, f"summary.md has no row for {f.get('id')}")
        require(case, (UNREADABLE_MARK in row) == ("unreadable" in c), violation,
                f"summary.md row for {c['location']!r} marks it wrong: {row[:200]}")
    return found


class Locations(unittest.TestCase):
    def test_forms_in_run(self):
        v = "location-misread"
        w, table = location_world(self, v, [{"name": "codex", "preset": "codex"}])
        cases = table["cases"]
        w.control({"codex": {"findings": location_findings(cases, check_impact="false_pass")}})
        w.round("reviews/loc/01-candidate", extra=("--subject", w.repo / table["subject"]))
        state = w.reviewer_state("reviews/loc/01-candidate", "codex")
        require(self, state.get("status") == "completed" and state.get("findings") == len(cases), v,
                f"unreadable locations changed the reviewer's result: {state.get('status')} {state.get('reason')}")
        check_locations(self, w, "reviews/loc/01-candidate", cases, v)

    def test_forms_in_record(self):
        v = "location-misread"
        w, table = location_world(self, v, [{"name": "native", "preset": "native"}])
        cases = table["cases"]
        out = "reviews/loc/01-candidate"
        w.round(out, extra=("--subject", w.repo / table["subject"]))
        result = w.record(out, "native", w.native_reply(location_findings(cases, check_impact="false_pass")))
        require(self, result.returncode == 0, v, f"spar record exited {result.returncode}: {text(result)[-600:]}")
        state = w.reviewer_state(out, "native")
        require(self, state.get("status") == "completed" and state.get("findings") == len(cases), v,
                f"unreadable locations changed the recorded result: {state.get('status')} {state.get('reason')}")
        check_locations(self, w, out, cases, v)

    def test_unreadable_classes(self):
        v = "unreadable-location-misclassified"
        w, table = location_world(self, v, [{"name": "codex", "preset": "codex"}])
        classes = forms()["unreadable_classes"]
        rule_doc = w.tmp / "rules" / classes["rule_document"][0]
        rule_doc.parent.mkdir(parents=True, exist_ok=True)
        rule_doc.write_text(lines_of(classes["rule_document"][1]))
        findings, expected = [], {}
        for i, c in enumerate(classes["cases"]):
            for kind, location in (("unreadable", classes["location"]), ("readable", classes["readable_location"])):
                fid = f"F{len(findings) + 1}"
                findings.append(finding(fid, location=location, **c["values"]))
                expected[fid] = (kind, c[kind], c["values"])
        w.control({"codex": {"findings": findings}})
        out = "reviews/cls/01-candidate"
        w.round(out, extra=("--rules", rule_doc))
        require(self, w.reviewer_state(out, "codex").get("status") == "completed", v,
                "unreadable locations changed the reviewer's status")
        found = by_reviewer_id(w.findings(out))
        auto = {e.get("finding") for e in w.ledger(out) if e.get("auto") is True}
        anything = {e.get("finding") for e in w.ledger(out)}
        open_ids = set()
        for fid, (kind, cls, values) in expected.items():
            f = found.get(fid) or {}
            require(self, f.get("class") == cls, v, f"{kind} {values}: class {f.get('class')}, expected {cls}")
            if kind == "unreadable":
                require(self, f.get("id") not in anything, v, f"{values}: an unreadable location has a ledger entry")
            else:
                require(self, (f.get("id") in auto) == (cls == "hardening"), v,
                        f"{values}: readable hardening and automatic deferral disagree")
            if cls in ("defect", "requirement-question"):
                open_ids.add(f.get("id"))
        readable_hardening = sum(1 for kind, cls, _ in expected.values() if kind == "readable" and cls == "hardening")
        _, gate = w.gate([out], "reviews/cls/gate.json")
        require(self, gate.get("auto_deferred") == readable_hardening, v,
                f"auto_deferred {gate.get('auto_deferred')}, expected {readable_hardening} (readable hardening only)")
        listed = {item.split()[-1] for item in gate.get("open_findings") or []}
        require(self, gate.get("state") == "blocked" and listed == open_ids, v,
                f"the gate before triage: {gate.get('state')}, open {sorted(listed)}, expected {sorted(open_ids)}")
        for fid in sorted(open_ids):
            cls = next(f for f in found.values() if f.get("id") == fid).get("class")
            args = ["answer", "--user-quote", QUOTE] if cls == "requirement-question" else ["reject", "--evidence", "synthetic"]
            result = w.spar_run("triage", out, fid, *args)
            require(self, result.returncode == 0, v, f"triage {fid}: {text(result)[-400:]}")
        _, gate = w.gate([out], "reviews/cls/gate2.json")
        require(self, gate.get("state") == "complete" and not gate.get("open_findings"), v,
                f"the gate after triage: {gate.get('state')} {gate.get('reasons')}")


# ---------------------------------------------------------------------------
# AC-3, AC-4, AC-19: rule, contract and criterion links
# ---------------------------------------------------------------------------

def lines_of(n):
    return "".join(f"line {i + 1}\n" for i in range(n))


def link_world(case, violation, group, reviewers=None):
    """One G8 group: its rule documents, head files, links, gitlinks and
    base-only files, and the documents passed with --rules."""
    w = World(case, violation)
    # The findings' locations (head/app/main.go:1, head/checks/t.py:1) name
    # files of both copies, so every finding here has a readable location.
    for rel in ("app/main.go", "checks/t.py"):
        w.write(rel, lines_of(3))
    for rel, n in group.get("base_only", []):
        w.write(rel, lines_of(n))
    w.base = w.commit("base")
    for rel, _ in group.get("base_only", []):
        w.remove(rel)
    for rel, n in group.get("rules", []) + group.get("head", []):
        w.write(rel, lines_of(n))
    for rel, target in group.get("head_symlinks", []):
        path = w.repo / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        os.symlink(target, path)
    w.g("add", "-A")
    for rel in group.get("head_submodules", []):
        w.g("update-index", "--add", "--cacheinfo", f"160000,{w.base},{rel}")
    w.g("commit", "-qm", "head", "--allow-empty")
    w.head = w.g("rev-parse", "HEAD")
    rules = [w.repo / rel for rel, _ in group.get("rules", [])]
    outside = w.tmp / "outside"
    for rel, n in group.get("outside_rules", []):
        path = outside / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(lines_of(n))
        rules.append(path)
    w.project_config('check_paths = ["checks/**"]\n')
    w.user_config(reviewers_toml(*(reviewers or [{"name": "codex", "preset": "codex"}])))
    w.control({})
    extra = []
    for path in rules:
        extra += ["--rules", path]
    return w, extra


def check_link(case, f, kind, link, confirmed, violation):
    if confirmed:
        require(case, f.get("link_kind") == kind and f.get("link") == link and f.get("class") == "defect", violation,
                f"{kind} link {link!r} was not confirmed: {f.get('link_kind')} {f.get('link')!r} {f.get('class')}")
    else:
        require(case, f.get("link_kind") == "none" and f.get("link") == "" and f.get("class") == "note", violation,
                f"{kind} link {link!r} was confirmed: {f.get('link_kind')} {f.get('link')!r} {f.get('class')}")
    require(case, f.get("claimed_link_kind") == kind and f.get("claimed_link") == link, violation,
            f"{kind} link {link!r}: the claimed link is not kept: {f.get('claimed_link_kind')} {f.get('claimed_link')!r}")


def run_link_group(case, group, violation, kept_only=False):
    cases = [c for c in group["cases"] if c["kept"] or not kept_only]
    w, extra = link_world(case, violation, group)
    findings = [finding(f"F{i + 1}", link_kind=c["kind"], link=c["link"]) for i, c in enumerate(cases)]
    w.control({"codex": {"findings": findings}})
    out = "reviews/links/01-candidate"
    w.round(out, extra=extra)
    found = by_reviewer_id(w.findings(out))
    for i, c in enumerate(cases):
        check_link(case, found.get(f"F{i + 1}") or {}, c["kind"], c["link"], c["confirmed"], violation)
    return w, out


class Links(unittest.TestCase):
    def test_link_forms(self):
        v = "link-misverified"
        for group in forms()["links"]["groups"]:
            run_link_group(self, group, v)
            # The result does not depend on the order of --rules.
            if len(group.get("rules", [])) > 1:
                run_link_group(self, dict(group, rules=list(reversed(group["rules"]))), v)
        # R6-1: a range past the end of a two-line document is not a link, so
        # a check-path contract violation is deferred hardening and a
        # defect proposal elsewhere is a note.
        group = forms()["links"]["groups"][0]
        w, extra = link_world(self, v, group)
        w.control({"codex": {"findings": [
            finding("F1", location="head/checks/t.py:1", check_impact="contract_violation",
                    link_kind="rule", link="AGENTS.md:1-9999"),
            finding("F2", link_kind="rule", link="AGENTS.md:1-9999"),
            finding("F3", location="head/checks/t.py:1", check_impact="contract_violation",
                    link_kind="rule", link="AGENTS.md:1-2")]}})
        out = "reviews/range/01-candidate"
        w.round(out, extra=extra)
        found = by_reviewer_id(w.findings(out))
        auto = {e.get("finding") for e in w.ledger(out) if e.get("auto") is True}
        require(self, found["F1"].get("class") == "hardening" and found["F1"].get("id") in auto, v,
                f"a check-path contract violation with a range past the end: {found['F1'].get('class')}")
        require(self, found["F2"].get("class") == "note", v,
                f"a defect proposal with a range past the end: {found['F2'].get('class')}")
        require(self, found["F3"].get("class") == "defect", v,
                f"a check-path contract violation with a range inside the document: {found['F3'].get('class')}")


def criteria_round(case, violation, cases):
    table = forms()["criteria"]
    w = World(case, violation)
    w.commits(["app/main.go"], ["app/main.go"])
    w.write("docs/specs/x/SPEC.md", table["text"])
    w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
    w.control({"codex": {"findings": [finding(f"F{i + 1}", link_kind="criterion", link=c["link"])
                                      for i, c in enumerate(cases)]}})
    out = "reviews/crit/01-candidate"
    w.round(out)
    found = by_reviewer_id(w.findings(out))
    for i, c in enumerate(cases):
        check_link(case, found.get(f"F{i + 1}") or {}, "criterion", c["link"], c["confirmed"], violation)
    return w, out, table


class Criteria(unittest.TestCase):
    def test_criterion_ids(self):
        v = "criterion-id-misread"
        w, out, table = criteria_round(self, v, forms()["criteria"]["cases"])
        ids = ((w.round_json(out) or {}).get("links") or {}).get("criteria_ids")
        require(self, ids == table["ids"], v, f"links.criteria_ids is {ids}, expected {table['ids']}")


class Kept(unittest.TestCase):
    """AC-19: the forms the reading before the input table accepted give the
    same path and link results."""

    def test_kept_forms(self):
        v = "kept-form-changed"
        table = forms()["locations"]
        kept = [c for c in table["cases"] if c.get("kept")]
        w, _ = location_world(self, v, [{"name": "codex", "preset": "codex"}])
        w.control({"codex": {"findings": location_findings(kept, check_impact="false_pass")}})
        out = "reviews/kept/01-candidate"
        w.round(out, extra=("--subject", w.repo / table["subject"]))
        found = by_reviewer_id(w.findings(out))
        for i, c in enumerate(kept):
            f = found.get(f"F{i + 1}") or {}
            require(self, f.get("path") == c["path"] and f.get("in_check_paths") is c["in_check_paths"], v,
                    f"{c['location']!r}: path {f.get('path')!r} {f.get('in_check_paths')}, expected {c['path']!r}")
        for group in forms()["links"]["groups"]:
            run_link_group(self, group, v, kept_only=True)
        criteria_round(self, v, [c for c in forms()["criteria"]["cases"] if c["kept"]])


# ---------------------------------------------------------------------------
# AC-5: result keys and limits, every preset
# ---------------------------------------------------------------------------

def changed_result(head, change):
    result = {"reviewed_head": head, "verdict": "blocking_findings",
              "angles_checked": [{"angle": a, "result": "checked"} for a in ANGLES], "angles_unchecked": [],
              "findings": [finding("F1")], "limits": "synthetic"}
    if change["change"] == "top":
        result[change["key"]] = change["value"]
    elif change["change"] == "finding":
        result["findings"][0][change["key"]] = change["value"]
    else:
        result["limits"] = change["value"]
    return result


PROCESS_PRESETS = ("claude", "codex", "agy", "cmd")


class Results(unittest.TestCase):
    def test_result_keys_and_limits(self):
        v = "result-shape-accepted"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "claude", "preset": "claude"}, {"name": "codex", "preset": "codex"},
                                     {"name": "agy", "preset": "agy"}, w.command_reviewer("cmd"),
                                     {"name": "native", "preset": "native"}))
        table = forms()["results"]
        for i, (change, valid) in enumerate([(c, False) for c in table["invalid"]] + [(c, True) for c in table["valid"]]):
            result = changed_result(w.head, change)
            w.control({name: {"result": result} for name in PROCESS_PRESETS})
            out = f"reviews/result-{i}/01-candidate"
            w.round(out)
            recorded = w.record(out, "native", result)
            require(self, recorded.returncode in (0, 1), v, f"spar record exited {recorded.returncode}: {text(recorded)[-400:]}")
            for name in PROCESS_PRESETS + ("native",):
                state = w.reviewer_state(out, name)
                if valid:
                    require(self, state.get("status") == "completed", v,
                            f"{name}: a result with {change['key']} = {change['value']!r} was not accepted: "
                            f"{state.get('status')} {state.get('reason')}")
                else:
                    require(self, state.get("status") == "invalid_output" and change["key"] in (state.get("reason") or ""),
                            v, f"{name}: a result with {change['change']} {change['key']} = {change['value']!r}: "
                            f"{state.get('status')} {state.get('reason')!r}")


# ---------------------------------------------------------------------------
# AC-6: native transcripts that read outside the workspace
# ---------------------------------------------------------------------------

OUTSIDE_WARNING = "outside the native workspace"


def transcript_file(path, prompt, reply, reads):
    """A synthetic host subagent transcript: the prompt read, the given tool
    uses (name, input, cwd or None) and the reply."""
    lines = [{"type": "attachment", "attachment": {"type": "environment", "snapshot": {"isGitRepo": False}}}]
    for i, (name, tool_input, cwd) in enumerate([("Read", {"file_path": str(prompt)}, str(prompt.parent))] + reads):
        entry = {"type": "assistant", "message": {"id": f"m{i}", "role": "assistant", "model": "synthetic-model",
                                                  "content": [{"type": "tool_use", "id": f"u{i}", "name": name,
                                                               "input": tool_input}]}}
        if cwd is not None:
            entry["cwd"] = cwd
        lines.append(entry)
    lines.append({"type": "assistant", "cwd": str(prompt.parent), "message": {
        "id": "last", "role": "assistant", "model": "synthetic-model", "stop_reason": "end_turn",
        "content": [{"type": "text", "text": json.dumps(reply)}]}})
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(json.dumps(line) for line in lines) + "\n")
    return path


class Transcripts(unittest.TestCase):
    def test_reads_outside_workspace(self):
        v = "outside-read-not-warned"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "native", "preset": "native"}))
        secret = w.tmp / "secret.txt"
        secret.write_text("SECRET-CONTENT-TOKEN\n")
        cases = [
            ("inside", lambda ws: [("Read", {"file_path": str(ws / "head/app/main.go")}, str(ws)),
                                   ("Grep", {"pattern": "x", "path": "head"}, str(ws)),
                                   ("Glob", {"pattern": "**/*.go"}, str(ws))], None),
            ("absolute", lambda ws: [("Read", {"file_path": "/etc/hosts"}, str(ws))], "Read"),
            ("relative", lambda ws: [("Read", {"file_path": "../../../secret.txt"}, str(ws))], "Read"),
            ("no path", lambda ws: [("Grep", {"pattern": "x"}, str(w.repo))], "Grep"),
            ("glob path", lambda ws: [("Glob", {"pattern": "*", "path": str(w.tmp)}, str(ws))], "Glob"),
            ("no cwd", lambda ws: [("Read", {"file_path": "head/app/main.go"}, None)], "Read"),
        ]
        for i, (label, reads, tool_name) in enumerate(cases):
            out = f"reviews/t{i}/01-candidate"
            w.round(out)
            native = w.repo / out / "native"
            path = transcript_file(w.tmp / f"session{i}/subagents/agent-t{i}.jsonl", native / "prompt.md",
                                   w.native_reply([]), reads(native / "workspace"))
            result = w.spar_run("record", out, "native", "--transcript", path)
            require(self, result.returncode == 0, v, f"{label}: spar record exited {result.returncode}: {text(result)[-400:]}")
            warnings = w.reviewer_state(out, "native").get("warnings") or []
            outside = [x for x in warnings if OUTSIDE_WARNING in x]
            if tool_name is None:
                require(self, not outside, v, f"{label}: reads inside the workspace were warned: {warnings}")
            else:
                require(self, len(outside) == 1 and tool_name in outside[0], v,
                        f"{label}: a {tool_name} read outside the workspace was not warned: {warnings}")
                joined = " ".join(warnings)
                for leaked in ("/etc/hosts", "secret.txt", "SECRET-CONTENT-TOKEN", str(w.tmp)):
                    require(self, leaked not in joined, v, f"{label}: a warning holds a path or content: {joined}")


# ---------------------------------------------------------------------------
# AC-7: the contract states what spar reads from a reviewer's run
# ---------------------------------------------------------------------------

QUOTA_PHRASES = ["RESOURCE_EXHAUSTED", "code 429", "status 429", "429 Too Many Requests", "too many requests",
                 "rate limit", "rate_limit", "rate-limit", "ratelimit", "quota reached", "quota exceeded",
                 "quota exhausted", "insufficient_quota", "hit your limit", "hit your usage limit", "usage limit reached"]
AUTH_PHRASES = ["UNAUTHENTICATED", "code 401", "status 401", "401 Unauthorized", "not logged in",
                "authentication required", "authentication failed", "login again", "log in again", "sign in again",
                "codex login", "agy login", "claude login", "run /login", "oauth token expired",
                "oauth token has expired", "invalid api key", "incorrect api key"]
RESET_PHRASES = ["resets in", "reset at", "try again in", "try again at"]


class Sentences(unittest.TestCase):
    def test_current_behavior_in_contract(self):
        v = "current-behavior-unstated"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        # G11: agy's shell commands are checked after 2>/dev/null and 2>&1 are removed.
        w.user_config(reviewers_toml({"name": "agy", "preset": "agy"}))
        for i, (line, status) in enumerate([("grep -rn greet head 2>/dev/null", "completed"),
                                            ("cat head/app/main.go 2>&1", "completed"),
                                            ("cat head/app/main.go > copy.txt", "tool_violation")]):
            w.control({"agy": {"commands": [line]}})
            out = f"reviews/agy{i}/01-candidate"
            w.round(out)
            state = w.reviewer_state(out, "agy")
            require(self, state.get("status") == status, v, f"agy command {line!r}: {state.get('status')}, expected {status}")
        # G12: codex output names no model, so the observed model is unknown.
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}, {"name": "claude", "preset": "claude"}))
        w.control({})
        w.round("reviews/model/01-candidate")
        codex = w.reviewer_state("reviews/model/01-candidate", "codex")
        claude = w.reviewer_state("reviews/model/01-candidate", "claude")
        require(self, codex.get("observed_model") is None and claude.get("observed_model") == "opus", v,
                f"observed models: codex {codex.get('observed_model')!r}, claude {claude.get('observed_model')!r}")
        # G13: the refusals spar recognizes; anything else is not a refusal.
        phrases = [(p, "quota or rate limit:") for p in QUOTA_PHRASES] + [(p, "authentication:") for p in AUTH_PHRASES]
        phrases.append(("disk is full", None))
        reviewers = [w.command_reviewer(f"r{i}") for i in range(len(phrases) + len(RESET_PHRASES))]
        w.user_config(reviewers_toml(*reviewers))
        control = {f"r{i}": {"stderr": f"error: {p}"} for i, (p, _) in enumerate(phrases)}
        for j, p in enumerate(RESET_PHRASES):
            control[f"r{len(phrases) + j}"] = {"stderr": f"error: rate limit reached, {p} 3h"}
        w.control(control)
        w.round("reviews/refusals/01-candidate")
        for i, (p, prefix) in enumerate(phrases):
            state = w.reviewer_state("reviews/refusals/01-candidate", f"r{i}")
            if prefix:
                require(self, state.get("status") == "unavailable" and (state.get("reason") or "").startswith(prefix), v,
                        f"refusal {p!r} was not recognized: {state.get('status')} {state.get('reason')!r}")
            else:
                require(self, state.get("status") == "failed", v, f"{p!r} was taken for a refusal: {state.get('status')}")
        for j, p in enumerate(RESET_PHRASES):
            state = w.reviewer_state("reviews/refusals/01-candidate", f"r{len(phrases) + j}")
            require(self, (state.get("resets") or "").lower().startswith(p), v,
                    f"reset phrase {p!r} was not recorded: {state.get('resets')!r}")
        # G14: finding the executable.
        plain = w.tmp / "plain-codex"
        plain.write_text("#!/bin/sh\nexit 0\n")
        plain.chmod(0o644)
        folder = w.tmp / "folder-codex"
        folder.mkdir()
        lookup = w.tmp / "lookup"
        lookup.mkdir()
        (lookup / "agy").write_text("#!/bin/sh\nexit 0\n")
        w.user_config(reviewers_toml({"name": "c-missing", "preset": "codex", "binary": str(w.tmp / "none/codex")},
                                     {"name": "c-plain", "preset": "codex", "binary": str(plain)},
                                     {"name": "c-folder", "preset": "codex", "binary": str(folder)},
                                     {"name": "agy", "preset": "agy"}))
        # PATH holds the non-executable agy, git and the system folders only,
        # so no installed reviewer CLI can be found.
        gitonly = w.tmp / "gitonly"
        gitonly.mkdir()
        (gitonly / "git").write_text(f"#!/bin/sh\nexec {w.git} \"$@\"\n")
        (gitonly / "git").chmod(0o755)
        for program in ("agy", "claude", "codex"):
            for folder in ("/usr/bin", "/bin"):
                if Path(folder, program).exists():
                    raise Unavailable(f"{folder}/{program} exists; the PATH lookup cannot be observed")
        env = w.env(PATH=str(lookup) + os.pathsep + str(gitonly) + os.pathsep + "/usr/bin:/bin")
        w.round("reviews/binaries/01-candidate", env=env)
        for name, status in (("c-missing", "unavailable"), ("c-plain", "failed"), ("c-folder", "failed"), ("agy", "unavailable")):
            state = w.reviewer_state("reviews/binaries/01-candidate", name)
            require(self, state.get("status") == status, v, f"{name}: {state.get('status')}, expected {status}")
        # The packet lists earlier findings by round finding IDs, which carry the reviewer's name.
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.control({"codex": {"findings": [finding("F1")]}})
        w.round("reviews/prior/01-candidate")
        fix = w.write("app/main.go", "fixed\n") and w.commit("fix")
        w.control({}, head=fix)
        w.round("reviews/prior/02-fixes", stage="fixes", base=w.head, head=fix)
        require(self, "codex-F1" in w.packet("reviews/prior/02-fixes"), v,
                "the fixes packet does not list the earlier finding by its round ID")
        # The contract says all of this.
        body = read_doc(self, CONTRACT, v)
        rows = table_rows(self, body, v)
        for row, tokens in (("G11", ["2>/dev/null", "2>&1"]), ("G12", ["observed_model", "모름", "null"]),
                            ("G14", ["unavailable", "failed", "실행 권한"])):
            cell = " ".join(rows.get(row) or [])
            for token in tokens:
                require(self, token in cell, v, f"row {row} does not say {token!r}")
        lowered = body.lower()
        for p in QUOTA_PHRASES + AUTH_PHRASES + RESET_PHRASES:
            require(self, p.lower() in lowered, v, f"{CONTRACT} does not list the refusal phrase {p!r}")
        require(self, "리뷰어 이름이 든 지적 ID" in read_doc(self, "contracts/review-round.md", v), v,
                "review-round.md does not say earlier findings appear by IDs that carry reviewer names")


# ---------------------------------------------------------------------------
# AC-8 to AC-13: configuration
# ---------------------------------------------------------------------------

def config_view(case, w, violation, env=None, extra=()):
    result = w.spar_run("config", *extra, env=env)
    require(case, result.returncode == 0, violation, f"spar config exited {result.returncode}: {text(result)[-600:]}")
    try:
        return json.loads(result.stdout.decode())
    except ValueError:
        case.fail(f"[violation:{violation}] spar config printed no JSON: {text(result)[-400:]}")


def names(view):
    return [r.get("name") for r in view.get("reviewers") or []]


def refuses_config(case, w, violation, label, needles, env=None, extra=(), run_extra=(), timeout=60):
    """spar config and spar run both end with a configuration error that
    names each needle, start no reviewer and write no round folder."""
    w.refusals = getattr(w, "refusals", 0) + 1
    out = f"reviews/refused-{w.refusals}/01"
    w.reset_log()
    for argv in (["config", *extra], ["run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out,
                                      "--criteria", w.criteria, *extra, *run_extra]):
        try:
            result = subprocess.run([str(a) for a in [w.spar, *argv]], cwd=w.repo, env=env or w.env(),
                                    capture_output=True, timeout=timeout, stdin=subprocess.DEVNULL)
        except subprocess.TimeoutExpired:
            # Declared: a spar that waits on a configuration input does not reject it.
            case.fail(f"[violation:{violation}] {label}: spar {argv[0]} did not end within {timeout}s")
        except OSError as error:
            raise Unavailable(f"spar: {error}") from None
        if result.returncode < 0:
            raise Unavailable(f"spar ended by signal {-result.returncode}")
        message = text(result)
        require(case, result.returncode == 65, violation,
                f"{label}: spar {argv[0]} exited {result.returncode}, expected 65: {message[-400:]}")
        for needle in needles:
            require(case, str(needle) in message or os.path.realpath(str(needle)) in message, violation,
                    f"{label}: spar {argv[0]} does not name {needle}: {message[-400:]}")
    require(case, not w.invocations(), violation, f"{label}: a reviewer started")
    require(case, not (w.repo / out).exists(), violation, f"{label}: a round folder was written")


class Sources(unittest.TestCase):
    def test_non_regular_sources(self):
        v = "config-source-misread"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        folder = w.tmp / "a-folder"
        folder.mkdir()
        fifo = w.tmp / "a-fifo"
        try:
            os.mkfifo(fifo)
        except (AttributeError, OSError) as error:
            raise Unavailable(f"mkfifo: {error}") from None
        empty_xdg = w.tmp / "empty-xdg"
        empty_xdg.mkdir()
        refuses_config(self, w, v, "SPAR_CONFIG folder", [folder], env=w.env(SPAR_CONFIG=folder))
        refuses_config(self, w, v, "SPAR_CONFIG FIFO", [fifo], env=w.env(SPAR_CONFIG=fifo), timeout=30)
        xdg_toml = w.xdg / "jaekit/spar.toml"
        xdg_toml.mkdir(parents=True)
        refuses_config(self, w, v, "XDG folder", [xdg_toml])
        home_toml = w.home / ".config/jaekit/spar.toml"
        home_toml.mkdir(parents=True)
        refuses_config(self, w, v, "HOME folder", [home_toml], env=w.env(XDG_CONFIG_HOME=None))
        clean = w.tmp / "clean-folder"
        clean.mkdir()
        refuses_config(self, w, v, "clean-review folder", [clean],
                       env=w.env(XDG_CONFIG_HOME=empty_xdg, HOME=w.tmp / "nohome", CLEAN_REVIEW_CONFIG=clean))
        shutil.rmtree(home_toml)
        shutil.rmtree(xdg_toml)
        project = w.repo / ".jaekit/spar.toml"
        project.mkdir(parents=True)
        refuses_config(self, w, v, "project folder", [project])
        project.rmdir()
        # A missing path and a broken symbolic link are skipped.
        missing = w.tmp / "missing.toml"
        broken = w.tmp / "broken.toml"
        os.symlink(w.tmp / "nowhere.toml", broken)
        loop = w.tmp / "loop.toml"
        os.symlink(loop, loop)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        for path in (missing, broken, loop):
            view = config_view(self, w, v, env=w.env(SPAR_CONFIG=path))
            skipped = (view.get("source") or {}).get("skipped") or []
            require(self, str(path) in skipped and names(view) == ["codex"], v,
                    f"{path.name}: not skipped to the next source: {view.get('source')} {names(view)}")


class CleanReview(unittest.TestCase):
    def test_reviewer_list_shape(self):
        v = "clean-review-list-misread"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        empty_xdg = w.tmp / "empty-xdg"
        empty_xdg.mkdir()
        clean = w.tmp / "clean.json"
        env = w.env(XDG_CONFIG_HOME=empty_xdg, HOME=w.tmp / "nohome", CLEAN_REVIEW_CONFIG=clean)
        table = forms()["clean_review"]
        for body in table["errors"]:
            clean.write_text(body)
            refuses_config(self, w, v, f"clean-review {body}", [clean], env=env)
        for case in table["lists"]:
            clean.write_text(case["json"])
            view = config_view(self, w, v, env=env)
            require(self, names(view) == case["names"], v, f"clean-review {case['json']}: reviewers {names(view)}")


class Timeouts(unittest.TestCase):
    def test_top_level_timeout(self):
        v = "timeout-misapplied"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config("timeout_seconds = 60\n")
        view = config_view(self, w, v)
        require(self, [(r.get("name"), r.get("timeout_seconds")) for r in view.get("reviewers") or []]
                == [("claude", 60), ("codex", 60), ("agy", 60)], v,
                f"a file with only timeout_seconds: {[(r.get('name'), r.get('timeout_seconds')) for r in view.get('reviewers') or []]}")
        w.round("reviews/timeout/01-candidate")
        recorded = {name: state.get("timeout_seconds")
                    for name, state in ((w.round_json("reviews/timeout/01-candidate") or {}).get("reviewers") or {}).items()}
        require(self, recorded == {"claude": 60, "codex": 60, "agy": 60}, v, f"round.json timeouts: {recorded}")
        w.user_config(reviewers_toml({"name": "claude", "preset": "claude"}, top={"timeout_seconds": 45}))
        for flag, expected in (("codex", [("codex", 45)]), ("native", [("native", 45)]), ("", [("native", 45)])):
            view = config_view(self, w, v, extra=("--reviewers", flag))
            got = [(r.get("name"), r.get("timeout_seconds")) for r in view.get("reviewers") or []]
            require(self, got == expected, v, f"--reviewers {flag!r} with a top-level timeout: {got}")
        w.user_config("timeout_seconds = 30\nreviewers = []\n")
        got = [(r.get("name"), r.get("timeout_seconds")) for r in config_view(self, w, v).get("reviewers") or []]
        require(self, got == [("native", 30)], v, f"an empty list with a top-level timeout: {got}")

    def test_version_check_in_limit(self):
        v = "version-check-outside-limit"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "agy", "preset": "agy", "timeout_seconds": 5}))
        w.control({"agy": {"version_sleep": 120}})
        started = time.monotonic()
        w.round("reviews/version/01-candidate", ok=None)
        elapsed = time.monotonic() - started
        state = w.reviewer_state("reviews/version/01-candidate", "agy")
        require(self, state.get("status") == "timed_out" and elapsed < 25, v,
                f"a version check that never ends with timeout_seconds = 5: {state.get('status')} after {elapsed:.1f}s")


class Names(unittest.TestCase):
    def test_reviewer_names(self):
        v = "reviewer-name-misjudged"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        table = forms()["names"]
        for group in table["config_errors"]:
            entries = [{"name": n, "preset": n.lower() if n.lower() in ("codex", "claude", "agy") else "native"} for n in group]
            path = w.user_config(reviewers_toml(*entries))
            refuses_config(self, w, v, f"names {group}", [path, group[-1]])
        w.user_config(reviewers_toml(*[{"name": n, "preset": "native"} for n in table["accepted"]]))
        require(self, names(config_view(self, w, v)) == table["accepted"], v, "accepted names were refused")
        for group in table["override_errors"]:
            path = w.user_config(reviewers_toml({"name": group[0], "preset": "codex"}))
            refuses_config(self, w, v, f"--reviewers {group}", [group[-1]], extra=("--reviewers", ",".join(group)))
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        for case in table["override_lists"]:
            view = config_view(self, w, v, extra=("--reviewers", case["flag"]))
            require(self, names(view) == case["names"], v, f"--reviewers {case['flag']!r}: {names(view)}")


class Rules(unittest.TestCase):
    def test_rules_entries(self):
        v = "rules-entry-accepted"
        w = World(self, v)
        w.commits(["app/main.go", "AGENTS.md"], ["app/main.go", "AGENTS.md"])
        outside = w.tmp / "outside.md"
        outside.write_text("outside rule\n")
        os.symlink(outside, w.repo / "linked.md")
        (w.repo / "somedir").mkdir()
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        for entry in ("linked.md", "", "/abs.md", "../x.md", "missing.md", "somedir"):
            path = w.project_config(f"rules = [{toml_value(entry)}]\n")
            refuses_config(self, w, v, f"rules entry {entry!r}", [".jaekit/spar.toml:1"] + ([entry] if entry else []))
        w.project_config('rules = ["AGENTS.md"]\n')
        require(self, config_view(self, w, v).get("rules") == ["AGENTS.md"], v, "a rules entry inside the repository was refused")


class Globs(unittest.TestCase):
    def test_glob_errors(self):
        v = "glob-error-accepted"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        table = forms()["globs"]
        for pattern in table["project_errors"]:
            w.project_config(f"check_paths = [{toml_value(pattern)}]\n")
            refuses_config(self, w, v, f"check_paths {pattern!r}", [".jaekit/spar.toml:1"])
        (w.repo / ".jaekit/spar.toml").unlink()
        for pattern in table["flag_errors"]:
            w.reset_log()
            out = f"reviews/flag-{table['flag_errors'].index(pattern)}/01"
            result = w.round(out, extra=("--check-path", pattern), ok=None)
            require(self, result.returncode == 65 and pattern in text(result), v,
                    f"--check-path {pattern!r}: exit {result.returncode}: {text(result)[-300:]}")
            require(self, not w.invocations() and not (w.repo / out).exists(), v,
                    f"--check-path {pattern!r}: a reviewer started or a round folder was written")

    def test_glob_consumers(self):
        v = "glob-consumers-differ"
        table = forms()["globs"]
        paths = list(table["paths"])
        for how in ("project", "flag"):
            w = World(self, v)
            w.commits(["app/main.go"], ["app/main.go", *paths])
            for i, rel in enumerate(paths):
                w.write(rel, "line\n" * (1 + 2 ** i))
            fix = w.commit("fix")
            w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
            extra = []
            if how == "project":
                w.project_config(f"check_paths = {toml_value(table['patterns'])}\n")
            else:
                for pattern in table["patterns"]:
                    extra += ["--check-path", pattern]
            w.control({"codex": {"findings": [finding(f"F{i + 1}", location=f"head/{rel}:1") for i, rel in enumerate(paths)]}})
            w.round("reviews/glob/01-candidate", extra=extra)
            found = by_reviewer_id(w.findings("reviews/glob/01-candidate"))
            for i, rel in enumerate(paths):
                got = (found.get(f"F{i + 1}") or {}).get("in_check_paths")
                require(self, got is table["paths"][rel], v, f"{how}: in_check_paths of {rel}: {got}")
            packet = w.packet("reviews/glob/01-candidate")
            for pattern in table["patterns"]:
                require(self, f"`{pattern}`" in packet, v, f"{how}: the packet does not list {pattern}")
            w.control({}, head=fix)
            w.round("reviews/glob/02-fixes", stage="fixes", base=w.head, head=fix, extra=extra)
            _, gate = w.gate(["reviews/glob/01-candidate", "reviews/glob/02-fixes"], "reviews/glob/gate.json", expect=fix)
            check = sum(2 ** i for i, rel in enumerate(paths) if table["paths"][rel])
            other = sum(2 ** i for i, rel in enumerate(paths) if not table["paths"][rel])
            added = (gate.get("growth") or {}).get("added_lines")
            require(self, added == {"check": check, "other": other}, v,
                    f"{how}: growth {added}, expected check {check} and other {other}")


# ---------------------------------------------------------------------------
# AC-14: one record path rule
# ---------------------------------------------------------------------------

class RecordPaths(unittest.TestCase):
    def test_one_rule(self):
        v = "record-path-accepted"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        # A repository nested in the ignored reviews/ folder tracks a file and a folder.
        nested = w.repo / "reviews/nested"
        nested.mkdir(parents=True)
        w.g("init", "-q", cwd=nested)
        (nested / ".gitignore").write_text("/out/\n")
        (nested / "tracked.txt").write_text("nested record\n")
        (nested / "dir").mkdir()
        (nested / "dir/a.txt").write_text("nested\n")
        w.g("add", "-A", cwd=nested)
        w.g("commit", "-qm", "nested", cwd=nested)
        w.round("reviews/demo/01-candidate")
        notes = w.repo / "reviews/notes.txt"
        notes.write_text("not a gate file\n")

        def unchanged(paths):
            return {p: (p.read_bytes() if p.is_file() else None) for p in paths}

        protected = [nested / "tracked.txt", w.repo / "reviews/demo/01-candidate/round.json", notes,
                     w.repo / "reviews/demo/extensions.jsonl"]
        before = unchanged(protected)
        for out in ("reviews/nested/tracked.txt", "reviews/demo/01-candidate/round.json",
                    "reviews/demo/01-candidate/gate.json", "reviews/demo/extensions.jsonl", "reviews/notes.txt"):
            result = w.spar_run("gate", "reviews/demo/01-candidate", "--expect-head", w.head, "--out", out)
            require(self, result.returncode == 65, v, f"gate --out {out}: exit {result.returncode}: {text(result)[-300:]}")
        require(self, unchanged(protected) == before, v, "a refused gate --out changed a file")
        require(self, not (w.repo / "reviews/demo/01-candidate/gate.json").exists(), v,
                "a refused gate --out wrote a file in a given round folder")
        w.reset_log()
        result = w.round("reviews/nested/dir/r1", ok=None)
        require(self, result.returncode == 65 and not w.invocations() and not (nested / "dir/r1").exists(), v,
                f"run --out under a folder the nested repository tracks: exit {result.returncode}")
        result = w.spar_run("extend", "reviews/nested/dir", "--stage", "code", "--by", "1", "--user-quote", QUOTE)
        require(self, result.returncode == 65 and not (nested / "dir/extensions.jsonl").exists(), v,
                f"extend on a folder the nested repository tracks: exit {result.returncode}")
        # Paths both repositories ignore are accepted, and a gate file is replaced.
        result = w.round("reviews/nested/out/r1", ok=None)
        require(self, result.returncode in (0, 1), v, f"run --out ignored by both repositories: exit {result.returncode}: "
                f"{text(result)[-300:]}")
        _, first = w.gate(["reviews/demo/01-candidate"], "reviews/demo/gate.json")
        previous = (w.repo / "reviews/demo/gate.json").read_bytes()
        _, second = w.gate(["reviews/demo/01-candidate"], "reviews/demo/gate.json")
        require(self, second.get("previous_gate_digest") == sha(previous), v,
                "replacing a gate file does not record the previous one's digest")


# ---------------------------------------------------------------------------
# AC-15: the gate needs every round of a given round's series
# ---------------------------------------------------------------------------

class GateList(unittest.TestCase):
    def test_series_rounds_listed(self):
        v = "series-round-unlisted"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.write("docs/specs/x/PLAN.md", "# Plan\n")
        w.control({"codex": {"findings": [finding("F1", link_kind="criterion", link="AC-1")]}})
        w.round("reviews/s/01-plan", stage="plan", extra=("--subject", w.repo / "docs/specs/x/PLAN.md"))
        w.control({})
        w.round("reviews/s/02-candidate")
        w.round("reviews/s/00-dry", extra=("--dry-run",))
        # A round that stopped before recording any finding (round.json
        # without finished_at, no findings.json) is left out like a dry run.
        stopped = w.repo / "reviews/s/03-stopped"
        stopped.mkdir()
        record = dict(w.round_json("reviews/s/02-candidate") or {})
        record.pop("finished_at", None)
        record["sequence"] = 3
        (stopped / "round.json").write_text(json.dumps(record))
        result, gate = w.gate(["reviews/s/02-candidate"], "reviews/s/gate.json")
        missing = [r for r in gate.get("reasons") or [] if UNLISTED in r]
        require(self, gate.get("state") == "blocked" and result.returncode == 4 and len(missing) == 1
                and "01-plan" in missing[0], v,
                f"a series round left out of the list: {gate.get('state')} {gate.get('reasons')}")
        result, gate = w.gate(["reviews/s/01-plan", "reviews/s/02-candidate"], "reviews/s/gate2.json")
        require(self, gate.get("state") == "blocked" and not [r for r in gate.get("reasons") or [] if UNLISTED in r], v,
                f"every round given: {gate.get('state')} {gate.get('reasons')}")
        triage = w.spar_run("triage", "reviews/s/01-plan", "codex-F1", "reject", "--evidence", "synthetic")
        require(self, triage.returncode == 0, v, f"triage: {text(triage)[-300:]}")
        result, gate = w.gate(["reviews/s/01-plan", "reviews/s/02-candidate"], "reviews/s/gate3.json")
        require(self, gate.get("state") == "complete", v, f"every round given and closed: {gate.get('state')} {gate.get('reasons')}")
        result, gate = w.gate(["reviews/s/01-plan", "reviews/s/02-candidate", "reviews/s/03-stopped"], "reviews/s/gate4.json")
        require(self, gate.get("state") == "blocked", v, f"a stopped round given in the list: {gate.get('state')} {gate.get('reasons')}")


# ---------------------------------------------------------------------------
# AC-16: writers of a series take its lock
# ---------------------------------------------------------------------------

def hold_lock(folder):
    fd = os.open(folder, os.O_RDONLY)
    fcntl.flock(fd, fcntl.LOCK_EX)
    return fd


def release_lock(fd):
    fcntl.flock(fd, fcntl.LOCK_UN)
    os.close(fd)


def start(w, argv, label):
    err = w.tmp / f"stderr-{label}.txt"
    try:
        with err.open("wb") as stderr:
            proc = subprocess.Popen([str(a) for a in [w.spar, *argv]], cwd=w.repo, env=w.env(),
                                    stdout=subprocess.DEVNULL, stderr=stderr, stdin=subprocess.DEVNULL)
    except OSError as error:
        raise Unavailable(f"spar: {error}") from None
    return proc, err


def waits(case, proc, err, violation, label):
    """The writer reports that it waits and is still running while the lock is held."""
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        if "waiting" in err.read_text(errors="replace"):
            break
        if proc.poll() is not None:
            break
        time.sleep(0.1)
    if proc.poll() is not None and proc.returncode < 0:
        raise Unavailable(f"{label} ended by signal {-proc.returncode}")
    require(case, proc.poll() is None and "waiting" in err.read_text(errors="replace"), violation,
            f"{label} did not wait for the series lock (exit {proc.poll()}): {err.read_text(errors='replace')[-300:]}")


def finish(case, proc, err, violation, label):
    try:
        code = proc.wait(timeout=120)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait()
        raise Unavailable(f"{label} did not end after the lock was released") from None
    if code < 0:
        raise Unavailable(f"{label} ended by signal {-code}")
    require(case, code == 0, violation, f"{label} exited {code}: {err.read_text(errors='replace')[-300:]}")


class Locks(unittest.TestCase):
    def test_series_writes_wait(self):
        v = "series-write-unlocked"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "native", "preset": "native"}, {"name": "native2", "preset": "native"}))
        out = "reviews/lock/01-candidate"
        w.round(out)
        series = w.repo / "reviews/lock"
        replies = []
        for name in ("native", "native2"):
            path = w.tmp / f"reply-{name}.json"
            path.write_text(json.dumps(w.native_reply([finding("F1", claim=f"from {name}")])))
            replies.append((name, path))
        fd = hold_lock(series)
        procs = [start(w, ["record", out, name, "--result", path], f"record-{name}") for name, path in replies]
        try:
            for (proc, err), (name, _) in zip(procs, replies):
                waits(self, proc, err, v, f"spar record {name}")
            gate, gate_err = start(w, ["gate", out, "--expect-head", w.head, "--out", "reviews/lock/gate.json"], "gate")
            try:
                gate.wait(timeout=30)
            except subprocess.TimeoutExpired:
                pass
        finally:
            release_lock(fd)
        if gate.poll() is None:
            # Still running under the lock: it finishing soon after the lock
            # is released shows that it waited for it.
            try:
                gate.wait(timeout=30)
            except subprocess.TimeoutExpired:
                gate.kill()
                gate.wait()
                raise Unavailable("spar gate did not end, with or without the series lock") from None
            require(self, False, v, "spar gate waited for the series lock")
        if gate.returncode < 0:
            raise Unavailable(f"spar gate ended by signal {-gate.returncode}")
        require(self, gate.returncode in (0, 3, 4) and "waiting" not in gate_err.read_text(errors="replace"), v,
                f"spar gate under the lock: exit {gate.returncode}: {gate_err.read_text(errors='replace')[-300:]}")
        for (proc, err), (name, _) in zip(procs, replies):
            finish(self, proc, err, v, f"spar record {name}")
        states = {name: w.reviewer_state(out, name).get("status") for name, _ in replies}
        claims = sorted(f.get("claim") for f in w.findings(out))
        require(self, states == {"native": "completed", "native2": "completed"} and claims == ["from native", "from native2"],
                v, f"two concurrent records: {states} {claims}")
        # Two dispositions at once both reach the ledger and the summary.
        ids = [f.get("id") for f in w.findings(out)]
        fd = hold_lock(series)
        procs = [start(w, ["triage", out, fid, "reject", "--evidence", f"synthetic {fid}"], f"triage-{fid}") for fid in ids]
        try:
            for (proc, err), fid in zip(procs, ids):
                waits(self, proc, err, v, f"spar triage {fid}")
        finally:
            release_lock(fd)
        for (proc, err), fid in zip(procs, ids):
            finish(self, proc, err, v, f"spar triage {fid}")
        ledger = [e for e in w.ledger(out) if e.get("disposition") == "reject"]
        summary = w.summary(out)
        require(self, sorted(e.get("finding") for e in ledger) == sorted(ids)
                and all("reject" in (summary_row(summary, fid) or "") for fid in ids), v,
                f"two concurrent dispositions: ledger {ledger}, summary rows {[summary_row(summary, fid) for fid in ids]}")
        fd = hold_lock(series)
        proc, err = start(w, ["extend", "reviews/lock", "--stage", "code", "--by", "1", "--user-quote", QUOTE], "extend")
        try:
            waits(self, proc, err, v, "spar extend")
        finally:
            release_lock(fd)
        finish(self, proc, err, v, "spar extend")


# ---------------------------------------------------------------------------
# AC-17: consumers of each row
# ---------------------------------------------------------------------------

# The consumers each row's consumer cell must name (fixed tokens).
CONSUMERS = {
    "G1": ["spar run", "spar record"], "G2": ["spar run", "spar record"], "G3": ["spar run", "spar record"],
    "G4": ["spar run", "spar record", "summary.md"], "G5": ["spar run", "spar record"], "G6": ["spar run", "spar record"],
    "G7": ["findings.json", "in_check_paths", "class", "ledger.jsonl", "summary.md", "auto_deferred", "packet.md",
           "spar run", "spar record"],
    "G8": ["findings.json", "class", "packet.md", "spar run", "spar record"],
    "G9": ["links.criteria_ids", "findings.json", "class", "packet.md"],
    "G10": ["spar run"], "G11": ["agy"], "G12": ["round.json", "summary.md"],
    "G13": ["fallback_model", "attempts", "summary.md"],
    "G14": ["spar run"], "G15": ["spar record --transcript", "warnings"],
    "G16": ["spar config", "spar run"], "G17": ["spar config", "spar run"], "G18": ["spar config", "spar run"],
    "G19": ["spar config", "spar run", "round.json", "timeout_seconds"],
    "G20": ["spar config", "spar run", "--reviewers", "-F<n>", "spar record", "spar gate"],
    "G21": ["spar config", "spar run"], "G22": ["spar config", "spar run", "caps", "packet.md"],
    "G23": ["spar config", "spar run", "G26"], "G24": ["spar config", "spar run", "in_check_paths", "growth", "packet.md"],
    "G25": ["spar config", "spar run"], "G26": ["packet.md", "criteria_digest", "G7", "G8"],
    "G27": ["spar run", "spar triage", "spar gate"], "G28": ["spar run", "spar extend", "spar gate"],
    "G29": ["spar gate"], "G30": ["spar triage", "spar gate"], "G31": ["spar run", "spar extend"],
    "G32": ["spar run", "spar record", "spar triage", "spar extend", "spar gate"],
    "G33": ["spar run", "spar record", "spar triage", "spar extend"],
}
# The report bullets of the packet name only forms the table accepts.
REPORT_FORMS = {
    "- `location`": ({"location", "head/<path>:<line>", "head/<path>:<line>-<line>", "base/<path>:<line>", "head/<path>"},
                     {"head/<path>:<line>", "head/<path>:<line>-<line>", "base/<path>:<line>"}),
    "- `link_kind`": ({"link_kind", "criterion", "AC-n", "rule", "contract", "AGENTS.md:12", "AGENTS.md:12-14", "none"},
                      {"criterion", "AC-n", "rule", "contract", "AGENTS.md:12", "none"}),
}


def section(body, heading):
    lines = body.splitlines()
    for i, line in enumerate(lines):
        if line.strip() == heading:
            rest = []
            for later in lines[i + 1:]:
                if later.startswith("#") and later.lstrip("#").startswith(" "):
                    break
                rest.append(later)
            return "\n".join(rest)
    return None


class Consumers(unittest.TestCase):
    def test_consumer_column(self):
        v = "consumer-unlisted"
        rows = table_rows(self, read_doc(self, CONTRACT, v), v)
        for row, tokens in CONSUMERS.items():
            cell = (rows.get(row) or ["", "", "", "", ""])[4]
            for token in tokens:
                require(self, f"`{token}`" in cell or token in cell, v, f"row {row} does not list the consumer {token!r}")

    def test_packet_report_forms(self):
        v = "packet-form-outside"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go"])
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.project_config('check_paths = ["checks/**"]\n')
        w.round("reviews/packet/01-dry", extra=("--dry-run",))
        report = section(w.packet("reviews/packet/01-dry"), "## How to report")
        require(self, report is not None, v, "the packet has no How to report section")
        bullets = [line for line in report.splitlines() if line.startswith("- ")]
        for start, (allowed, needed) in REPORT_FORMS.items():
            bullet = [b for b in bullets if b.startswith(start)]
            require(self, len(bullet) == 1, v, f"the packet has {len(bullet)} bullets starting {start}")
            spans = set(re.findall(r"`([^`]+)`", bullet[0]))
            require(self, spans <= allowed, v, f"{start} names forms outside the table: {sorted(spans - allowed)}")
            require(self, needed <= spans, v, f"{start} lacks the forms {sorted(needed - spans)}")
        location = next(b for b in bullets if b.startswith("- `location`"))
        require(self, "unreadable" in location and "never" in location, v,
                "the location bullet does not say what happens to a location that names nothing")

    def test_consumers_agree(self):
        v = "consumers-disagree"
        cases = forms()["locations"]["cases"]
        w, table = location_world(self, v, [{"name": "codex", "preset": "codex"}, {"name": "native", "preset": "native"}])
        findings = location_findings(cases)
        w.control({"codex": {"findings": findings}})
        out = "reviews/agree/01-candidate"
        w.round(out, extra=("--subject", w.repo / table["subject"]))
        result = w.record(out, "native", w.native_reply(findings))
        require(self, result.returncode == 0, v, f"spar record exited {result.returncode}: {text(result)[-400:]}")
        keys = ("path", "in_check_paths", "location_unreadable", "class", "class_reason")
        by = {}
        for f in w.findings(out):
            by.setdefault(f.get("reviewer"), {})[f.get("reviewer_id")] = tuple(f.get(k) for k in keys)
        require(self, by.get("codex") and by.get("codex") == by.get("native"), v,
                "spar run and spar record read the same locations differently")
        auto = {}
        for e in w.ledger(out):
            if e.get("auto") is True:
                reviewer, fid = e.get("finding").rsplit("-", 1)
                auto.setdefault(reviewer, set()).add(fid)
        require(self, auto.get("codex") == auto.get("native"), v, "run and record defer different findings")
        # The classification of unreadable locations is the same in run and
        # record, neither defers it, and the summary does not show an
        # unreadable hardening finding as waiting for a disposition.
        classes = forms()["unreadable_classes"]
        w3, _ = location_world(self, v, [{"name": "codex", "preset": "codex"}, {"name": "native", "preset": "native"}])
        rule_doc = w3.tmp / "rules" / classes["rule_document"][0]
        rule_doc.parent.mkdir(parents=True, exist_ok=True)
        rule_doc.write_text(lines_of(classes["rule_document"][1]))
        unreadable = [finding(f"F{i + 1}", location=classes["location"], **c["values"]) for i, c in enumerate(classes["cases"])]
        w3.control({"codex": {"findings": unreadable}})
        out3 = "reviews/agree/02-classes"
        w3.round(out3, extra=("--rules", rule_doc))
        result = w3.record(out3, "native", w3.native_reply(unreadable))
        require(self, result.returncode == 0, v, f"spar record exited {result.returncode}: {text(result)[-400:]}")
        by = {}
        for f in w3.findings(out3):
            by.setdefault(f.get("reviewer"), {})[f.get("reviewer_id")] = tuple(f.get(k) for k in keys)
        require(self, by.get("codex") and by.get("codex") == by.get("native"), v,
                "spar run and spar record classify unreadable locations differently")
        entries = [e.get("finding") for e in w3.ledger(out3)]
        require(self, not entries, v, f"unreadable locations have ledger entries: {entries}")
        summary = w3.summary(out3)
        for f in w3.findings(out3):
            if f.get("class") == "hardening":
                cells = [c.strip() for c in (summary_row(summary, f.get("id")) or "").strip("|").split("|")]
                require(self, len(cells) > 3 and "open" not in cells[3], v,
                        f"summary.md shows an unreadable hardening finding as open: {cells[:4]}")
        # Criterion IDs: the IDs of the criteria document are exactly the ones a link can name.
        crit = forms()["criteria"]
        w2, out2, _ = criteria_round(self, v, [{"link": i, "confirmed": True} for i in crit["ids"]]
                                     + [{"link": "AC-9a", "confirmed": False}, {"link": "XAC-7", "confirmed": False}])
        ids = ((w2.round_json(out2) or {}).get("links") or {}).get("criteria_ids")
        require(self, ids == crit["ids"], v, f"links.criteria_ids {ids} and the links disagree")


# ---------------------------------------------------------------------------
# AC-18: a real reviewer process through record, triage and gate
# ---------------------------------------------------------------------------

class Flow(unittest.TestCase):
    def test_command_reviewer_flow(self):
        v = "flow-misjudged"
        w = World(self, v)
        def documents():
            w.write("AGENTS.md", lines_of(2))
            w.write("base/api.md", lines_of(3))
            w.g("add", "-A")
        w.commits(["app/main.go", "checks/t.py"], ["app/main.go", "checks/t.py", "checks/my test.py"],
                  head_extra=documents)
        w.project_config('check_paths = ["checks/**"]\n')
        w.user_config(reviewers_toml(w.command_reviewer("rev")))
        w.control({"rev": {"findings": [
            finding("F1", location="head/checks/t.py:2", check_impact="contract_violation", link_kind="rule",
                    link="AGENTS.md:1-9999"),
            finding("F2", link_kind="rule", link="AGENTS.md:1-9999"),
            finding("F3", location="head/checks/t.py:2", check_impact="contract_violation", link_kind="contract",
                    link="base/api.md:3"),
            finding("F4", location="head/checks/my test.py:3, head/app/main.go:9"),
            finding("F5", location="head/checks/t.py:10:5")]}})
        out = "reviews/flow/01-candidate"
        w.round(out, extra=("--rules", w.repo / "AGENTS.md"))
        state = w.reviewer_state(out, "rev")
        require(self, state.get("status") == "completed", v, f"the command reviewer: {state.get('status')} {state.get('reason')}")
        require(self, w.invocations(), v, "no reviewer process ran")
        found = by_reviewer_id(w.findings(out))
        expected = {"F1": "hardening", "F2": "note", "F3": "defect", "F4": "hardening", "F5": "defect"}
        for fid, cls in expected.items():
            require(self, (found.get(fid) or {}).get("class") == cls, v,
                    f"{fid}: class {(found.get(fid) or {}).get('class')}, expected {cls}")
        require(self, found["F4"].get("path") == "checks/my test.py", v, f"F4 path {found['F4'].get('path')!r}")
        require(self, found["F5"].get("location_unreadable") is True, v, "F5 is not marked unreadable")
        auto = {e.get("finding") for e in w.ledger(out) if e.get("auto") is True}
        require(self, auto == {found["F1"]["id"], found["F4"]["id"]}, v, f"automatic deferrals: {sorted(auto)}")
        result, gate = w.gate([out], "reviews/flow/gate.json")
        listed = {item.split()[-1] for item in gate.get("open_findings") or []}
        require(self, gate.get("state") == "blocked" and listed == {found["F3"]["id"], found["F5"]["id"]}
                and gate.get("auto_deferred") == 2, v,
                f"the gate before triage: {gate.get('state')} open {sorted(listed)} auto {gate.get('auto_deferred')}")
        for fid in ("F3", "F5"):
            result = w.spar_run("triage", out, found[fid]["id"], "reject", "--evidence", "synthetic")
            require(self, result.returncode == 0, v, f"triage {fid}: {text(result)[-300:]}")
        result, gate = w.gate([out], "reviews/flow/gate2.json")
        require(self, gate.get("state") == "degraded" and not gate.get("open_findings"), v,
                f"the gate after triage: {gate.get('state')} {gate.get('reasons')}")
        # A configuration error of each kind ends spar run with 65 before a reviewer starts.
        folder = w.tmp / "folder"
        folder.mkdir()
        empty_xdg = w.tmp / "empty-xdg"
        empty_xdg.mkdir()
        clean = w.tmp / "clean.json"
        clean.write_text('{"reviewers": null}')
        host = w.tmp / "host.toml"
        host.write_text(reviewers_toml({"name": "Host", "preset": "native"}, {"name": "host", "preset": "native"}))
        for label, env, extra in (("G16", w.env(SPAR_CONFIG=folder), ()),
                                  ("G18", w.env(XDG_CONFIG_HOME=empty_xdg, HOME=w.tmp / "nohome", CLEAN_REVIEW_CONFIG=clean), ()),
                                  ("G20", w.env(SPAR_CONFIG=host), ()),
                                  ("G24", None, ("--check-path", "checks/[a"))):
            w.reset_log()
            run_out = f"reviews/flow-{label}/01"
            result = w.round(run_out, extra=extra, env=env, ok=None)
            require(self, result.returncode == 65 and not w.invocations() and not (w.repo / run_out).exists(), v,
                    f"{label}: spar run exited {result.returncode} and started {len(w.invocations())} reviewer(s)")


# ---------------------------------------------------------------------------
# AC-20, AC-21, AC-22: what stays
# ---------------------------------------------------------------------------

PRESERVED = Path(__file__).with_name("preserved.json")


class Products(unittest.TestCase):
    def test_untouched(self):
        v = "products-changed"
        try:
            digests = json.loads(PRESERVED.read_text(encoding="utf-8"))["files"]
        except (OSError, ValueError, KeyError) as error:
            raise Unavailable(f"preserved.json cannot be read: {error}") from None
        for rel, digest in sorted(digests.items()):
            path = ROOT / rel
            require(self, path.is_file() and sha(path.read_bytes()) == digest, v, f"{rel} changed")
        # Tracked files only: an ignored local file (.DS_Store) is not a product file.
        try:
            tracked = subprocess.run(["git", "-C", str(ROOT), "ls-files", "-z", "--", "plugins/spec", "plugins/seal",
                                      "plugins/spar", "tools/release"], capture_output=True, check=True).stdout
        except (OSError, subprocess.CalledProcessError) as error:
            raise Unavailable(f"git ls-files: {error}") from None
        listed = {rel for rel in tracked.decode("utf-8", "surrogateescape").split("\0") if rel}
        kept = {rel for rel in digests if rel.startswith(("plugins/spec/", "plugins/seal/", "plugins/spar/", "tools/release/"))}
        require(self, listed == kept, v, f"files added or removed: {sorted(listed ^ kept)[:5]}")


def guide_block(case, rel, heading, violation):
    body = read_doc(case, rel, violation)
    part = section(body, heading)
    require(case, part is not None, violation, f"{rel} lacks {heading}")
    blocks = re.findall(r"```bash\n(.*?)```", part, re.S)
    require(case, len(blocks) == 1, violation, f"{rel} {heading} must hold one shell block")
    return body, blocks[0].replace("$HOME", "$INSTALL_TEST_ROOT")


GUIDES = (("guides/INSTALL.md", "## Release 파일로 설치", "두 파일을 바꾸는 것은 따로 일어나서"),
          ("guides/INSTALL.en.md", "## Install from a Release file", "The two replacements are still separate steps"))
INSTALL_PROGRAMS = ("mktemp", "rm", "grep", "tar", "gzip", "mkdir", "cp", "find", "diff", "mv", "chmod", "bash")


class Install(unittest.TestCase):
    def install(self, block, root, shims):
        """Run a Release block against a synthetic archive with ha and spar 2.0."""
        match = re.search(r'name="ha_([0-9.]+)_\$\{os\}_\$\{arch\}"', block)
        if not match:
            raise Unavailable("the install block does not name its archive")
        version = match.group(1)
        name = f"ha_{version}_linux_amd64"
        work = root / f"work-{len(list(root.glob('work-*')))}"
        package = work / name
        package.mkdir(parents=True)
        for folder in ("tools", "examples", "guides", "contracts", "assets"):
            (package / folder).mkdir()
            (package / folder / "fixture.txt").write_text("synthetic\n")
        for doc in ("README.md", "README.en.md", "README.ko.md", "LICENSE"):
            (package / doc).write_text("synthetic\n")
        for binary in ("ha", "spar"):
            (package / binary).write_text(f"#!/bin/sh\necho {binary} new\n")
            (package / binary).chmod(0o755)
        dist = work / "dist"
        dist.mkdir()
        archive = dist / (name + ".tar.gz")
        with tarfile.open(archive, "w:gz") as output:
            output.add(package, arcname=name)
        (dist / "checksums.txt").write_text(sha(archive.read_bytes()) + "  " + archive.name + "\n")
        commands = work / "commands"
        commands.mkdir()
        (commands / "uname").write_text('#!/bin/sh\nif [ "$1" = "-s" ]; then echo Linux; else echo x86_64; fi\n')
        (commands / "curl").write_text(f'#!/bin/sh\nfor last; do :; done\ncp "{dist}/${{last##*/}}" .\n')
        for c in ("uname", "curl"):
            (commands / c).chmod(0o755)
        for program, script in shims.items():
            (commands / program).write_text(script)
            (commands / program).chmod(0o755)
        for program in INSTALL_PROGRAMS:
            tool(program)
        tool("sha256sum" if shutil.which("sha256sum") or not shutil.which("shasum") else "shasum")
        env = dict(os.environ, INSTALL_TEST_ROOT=str(root / "installation"),
                   PATH=str(commands) + os.pathsep + os.environ.get("PATH", ""))
        result = run([tool("bash"), "-c", block], work, env, timeout=120)
        if 128 < result.returncode <= 192 or result.returncode in (126, 127):
            raise Unavailable(f"the install block could not run a command: {text(result)[-400:]}")
        return result

    def test_staged_replacement(self):
        v = "install-not-staged"
        real = {p: tool(p) for p in ("cp", "chmod", "mktemp")}
        failures = {
            "copy": {"cp": f'#!/bin/sh\ncase "$1" in */spar) exit 1;; esac\nexec {real["cp"]} "$@"\n'},
            "permission": {"chmod": f'#!/bin/sh\ncase "$2" in *.spar.*) exit 1;; esac\nexec {real["chmod"]} "$@"\n'},
            "temporary file": {"mktemp": f'#!/bin/sh\ncase "$1" in *.spar.*) exit 1;; esac\nexec {real["mktemp"]} "$@"\n'},
            "download": {"curl": "#!/bin/sh\nexit 22\n"},
        }
        for rel, heading, boundary in GUIDES:
            body, block = guide_block(self, rel, heading, v)
            require(self, boundary in body, v, f"{rel} does not state that the two replacements are separate steps")
            with tempfile.TemporaryDirectory(prefix="jaekit-spar-input-install-") as directory:
                root = Path(directory)
                bin_dir = root / "installation/.local/bin"
                result = self.install(block, root, {})
                require(self, result.returncode == 0, v, f"{rel}: the install failed: {text(result)[-400:]}")
                require(self, sorted(p.name for p in bin_dir.iterdir()) == ["ha", "spar"], v,
                        f"{rel}: installed {sorted(p.name for p in bin_dir.iterdir())}")
                for label, shims in failures.items():
                    for binary in ("ha", "spar"):
                        (bin_dir / binary).write_text(f"#!/bin/sh\necho {binary} old\n")
                    result = self.install(block, root, shims)
                    require(self, result.returncode != 0, v, f"{rel}: a failed {label} still exited 0")
                    left = sorted(p.name for p in bin_dir.iterdir())
                    require(self, left == ["ha", "spar"], v, f"{rel}: a failed {label} left {left}")
                    for binary in ("ha", "spar"):
                        require(self, "old" in (bin_dir / binary).read_text(), v,
                                f"{rel}: a failed {label} replaced {binary}")


def load_old_observer():
    path = ROOT / "tools/goal-checks/spar/observe_spar.py"
    try:
        spec = importlib.util.spec_from_file_location("observe_spar_for_starts", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    except Exception as error:
        raise Unavailable(f"the Spar observer cannot be loaded: {error}") from None
    return module, path.read_text(encoding="utf-8")


class ObserverStarts(unittest.TestCase):
    def test_start_checks(self):
        v = "start-check-missing"
        old, source = load_old_observer()
        with tempfile.TemporaryDirectory(prefix="jaekit-spar-input-starts-") as directory:
            root = Path(directory)
            def refused(call, *args):
                try:
                    call(*args)
                except old.Unavailable:
                    return True
                return False
            require(self, refused(old.tool, "jaekit-no-such-program-7f3a"), v, "a program missing from PATH passed")
            broken = root / "broken"
            broken.write_text("#!/nonexistent/interpreter\n")
            broken.chmod(0o755)
            plain = root / "plain"
            plain.write_text("#!/bin/sh\nexit 0\n")
            plain.chmod(0o644)
            for path in (broken, plain):
                require(self, refused(old.starts, str(path)), v,
                        f"a program that cannot start passed the start check: {path.name}")
            probe = root / "probe"
            seen = root / "seen.txt"
            probe.write_text(f'#!/bin/sh\nprintf %s "$GOTOOLCHAIN" > {seen}\n')
            probe.chmod(0o755)
            old.starts(str(probe))
            require(self, seen.read_text() == "local", v, "the start check does not keep go from fetching a toolchain")

            class Result:
                def __init__(self, code):
                    self.returncode, self.stdout, self.stderr = code, b"", b""
            for code in (126, 127):
                require(self, refused(old.not_started, Result(code), "block"), v,
                        f"a block command that could not start (exit {code}) is not an execution error")
            require(self, not refused(old.not_started, Result(1), "block"), v, "exit 1 is taken for a command that could not start")
        for token in ("for program in INSTALL_PROGRAMS:", "starts(str(probe))", 'GOTOOLCHAIN="local"',
                      'for program in ("mkdir", "go", "cp", "tar", "python3"):'):
            require(self, token in source, v, f"the Spar observer lost its start check: {token}")


# ---------------------------------------------------------------------------
# AC-23: the table records where three inputs come from
# ---------------------------------------------------------------------------

class TableSources(unittest.TestCase):
    def test_sources_recorded(self):
        v = "source-unrecorded"
        w = World(self, v)
        w.commits(["app/main.go"], ["app/main.go", "contracts/x.md"])
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.control({"codex": {"findings": [finding("F1", link_kind="contract", link="contracts/x.md:1")]}})
        project = w.project_config('check_paths = ["checks/**"]\n')
        out = "reviews/src/01-candidate"
        w.round(out, extra=("--check-path", "tools/**"))
        record = w.round_json(out) or {}
        config = record.get("config") or {}
        found = by_reviewer_id(w.findings(out))
        require(self, (found.get("F1") or {}).get("link_kind") == "contract", v,
                "a contract link to a regular file of the head commit was not kept")
        view = config_view(self, w, v)
        require(self, config.get("check_paths") == ["checks/**", "tools/**"] and config.get("digest") != view.get("digest"), v,
                f"--check-path is not in the round's check paths and digest: {config.get('check_paths')}")
        files = {f.get("path"): f.get("sha256") for f in config.get("files") or []}
        require(self, files.get(str(project)) == sha(project.read_bytes()), v,
                f"the working-tree project configuration is not in config.files: {files}")
        rows = table_rows(self, read_doc(self, CONTRACT, v), v)
        for row, tokens in (("G8", ["head 커밋의 일반 파일", "100644", "100755"]),
                            ("G24", ["--check-path", "round.json", "digest"]),
                            ("G16", ["작업 트리", "config.files", "digest"])):
            cell = " ".join(rows.get(row) or [])
            for token in tokens:
                require(self, token in cell, v, f"row {row} does not record {token!r}")


if __name__ == "__main__":
    unittest.main()
