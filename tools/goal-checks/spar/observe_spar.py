"""Observations for the Spar goal: the `spar` review runner, its plugin,
contracts, guides, build targets and release files.

Every observation reads the checked tree only. Runtime cases build `spar`
(and `ha` where a case needs it) from the checked tree with `go build` into a
temporary directory and run it against a synthetic Git repository that the
case creates itself. Reviewer CLIs (claude, codex, agy and a command
reviewer) are synthetic fakes written by this file: they never call a model
and only follow a control file. HOME, XDG_CONFIG_HOME and the clean-review
locations point into the temporary directory, so the user's own files are
never read or written.

Results:
- A requirement that is not met fails with exactly one `[violation:<id>]`
  and ends the test. The missing `cmd/spar` package is the declared form of a
  missing implementation: it is the violation of every runtime case, because
  the requirement is the behavior of that executable.
- A missing or unusable tool (go, git, bash, python), a build that fails for a
  present package, a `spar` that panics, is killed by a signal or exceeds its
  time limit raises Unavailable: an execution error, never a violation.
- Output that `spar` wrote but that is not the documented record (malformed
  JSON, a missing field) is observed behavior and a violation of the case.
"""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import stat
import struct
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[3]
SPAR_MAIN = ROOT / "cmd/spar/main.go"
ANGLES = ["requirement-counterexample", "side-effects", "design", "premises", "check-validity"]
CLASSES = ["defect", "requirement-question", "hardening", "note"]
COMMIT_TOKEN = "COMMIT-MESSAGE-TOKEN-7d1f"
IGNORED_TOKEN = "IGNORED-FILE-TOKEN-5c2a"
QUOTE = "합성 사용자: 이 지적은 이번 변경에서 고쳐 주세요"


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


def signalled(result, what):
    """A shell block whose command a signal ended exits 128+n: not a violation."""
    if 128 < result.returncode <= 128 + 64:
        raise Unavailable(f"{what}: a command ended by signal {result.returncode - 128}: {text(result)[-600:]}")


def tool(name):
    path = shutil.which(name)
    if not path:
        raise Unavailable(f"{name} is not on PATH")
    return path


_BUILT = {}
# Set only in the platform child mode (see platform_suite): an inherited
# SPAR_OBSERVE_BINARY never replaces the checked tree's spar otherwise.
PLATFORM_CHILD = False


def build(package, name, env_extra=None, ldflags=None):
    key = (package, name, tuple(sorted((env_extra or {}).items())), ldflags)
    if key not in _BUILT:
        out = Path(tempfile.mkdtemp(prefix="jaekit-spar-build-")) / name
        env = dict(os.environ, CGO_ENABLED="0", GOFLAGS="-mod=readonly", GOTOOLCHAIN="local")
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        env.update(env_extra or {})
        argv = [tool("go"), "build"]
        if ldflags:
            argv += ["-ldflags", ldflags]
        argv += ["-o", out, package]
        result = run(argv, ROOT, env, timeout=900)
        if result.returncode:
            raise Unavailable(f"go build {package}: {text(result)[-2000:]}")
        _BUILT[key] = out
    return _BUILT[key]


def spar_binary(case, violation):
    """The checked tree's `spar`, or the declared violation when it has none.

    A platform run (see Platforms) supplies a binary built for its target in
    SPAR_OBSERVE_BINARY instead; it never builds inside the target."""
    given = os.environ.get("SPAR_OBSERVE_BINARY") if PLATFORM_CHILD else None
    if given:
        if not Path(given).is_file():
            raise Unavailable(f"SPAR_OBSERVE_BINARY names no file: {given}")
        return Path(given)
    require(case, SPAR_MAIN.is_file(), violation, "the checked tree has no spar executable (cmd/spar)")
    return build("./cmd/spar", "spar")


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read_output(case, path, violation, what):
    """Text that spar must have written; its absence is the condition's violation."""
    path = Path(path)
    require(case, path.is_file(), violation, f"{what} is missing: {path}")
    return path.read_text(encoding="utf-8")


def path_named(message, path):
    """Whether a message names the file, as given or with symlinks resolved (macOS /var)."""
    return str(path) in message or os.path.realpath(path) in message


def load_json(case, path, violation, what):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except FileNotFoundError:
        case.fail(f"[violation:{violation}] {what} is missing: {path}")
    except (UnicodeDecodeError, ValueError) as error:
        case.fail(f"[violation:{violation}] {what} is not JSON: {error}")


def read_lines(path):
    path = Path(path)
    if not path.exists():
        return []
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


FAKE = r'''
import hashlib, json, os, sys, time
from pathlib import Path

name = os.path.basename(sys.argv[0])
if len(sys.argv) > 1 and sys.argv[1] == "--fake-name":
    name = sys.argv[2]
    args = sys.argv[3:]
else:
    args = sys.argv[1:]
if args[:1] == ["--version"]:
    print(f"{name} 9.9.9 (synthetic)")
    sys.exit(0)
control = json.loads(Path(os.environ["SPAR_FAKE_CONTROL"]).read_text())
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
spec = dict(control.get("default", {}))
spec.update(control.get("reviewers", {}).get(name, {}))
spec.update(spec.get("models", {}).get(model or "", {}))
cwd = os.getcwd()
listing = sorted(str(p.relative_to(cwd)) for p in Path(cwd).rglob("*"))
hashes = {str(p.relative_to(cwd)): hashlib.sha256(p.read_bytes()).hexdigest()
          for p in Path(cwd).rglob("*") if p.is_file() and str(p.relative_to(cwd)).startswith(("head/", "base/"))}
start = time.time()
time.sleep(float(spec.get("sleep", 0)))
end = time.time()
entry = {"name": name, "argv": args, "cwd": cwd, "model": model, "prompt_sha256": hashlib.sha256(prompt.encode()).hexdigest(),
         "start": start, "end": end, "listing": listing, "hashes": hashes, "pid": os.getpid()}
if name not in ("claude", "codex", "agy"):
    entry["workspace"] = option("--workspace")
    entry["schema"] = option("--schema")
with open(control["log"], "a") as log:
    log.write(json.dumps(entry) + "\n")
if spec.get("modify"):
    Path(cwd, "head", "app", "main.py").write_text("changed by the reviewer\n")
result = spec.get("result")
if result is None:
    result = {"reviewed_head": control["head"], "verdict": "no_blocking_findings",
              "angles_checked": [{"angle": a, "result": "checked"} for a in control.get("angles", [])],
              "angles_unchecked": [], "findings": spec.get("findings", []), "limits": "synthetic"}
if spec.get("drop_angles"):
    result = {k: v for k, v in result.items() if k not in ("angles_checked", "angles_unchecked")}
quota = spec.get("quota")
auth = spec.get("auth")
crash = spec.get("crash")
if crash:
    sys.stderr.write(crash + "\n")
    sys.exit(3)
if name == "claude":
    tools = ["Read", "Grep", "Glob", "StructuredOutput"] + ([spec["init_tool"]] if spec.get("init_tool") else [])
    print(json.dumps({"type": "system", "subtype": "init", "model": model, "tools": tools, "mcp_servers": []}), flush=True)
    if spec.get("tool"):
        print(json.dumps({"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "t1", "name": spec["tool"], "input": {}}]}}), flush=True)
    if quota or auth:
        print(json.dumps({"type": "result", "subtype": "error", "is_error": True, "result": quota or auth}))
        sys.exit(1)
    if spec.get("garbage"):
        print(json.dumps({"type": "result", "subtype": "success", "is_error": False, "structured_output": None, "result": "not json"}))
        sys.exit(0)
    print(json.dumps({"type": "result", "subtype": "success", "is_error": False, "structured_output": result, "usage": {}}))
elif name == "codex":
    print(json.dumps({"type": "thread.started"}), flush=True)
    if spec.get("tool"):
        print(json.dumps({"type": "item.completed", "item": {"type": spec["tool"]}}), flush=True)
    if quota or auth:
        print(json.dumps({"type": "error", "message": quota or auth}))
        sys.exit(1)
    if not spec.get("garbage"):
        Path(option("-o")).write_text(json.dumps(result))
    else:
        Path(option("-o")).write_text("{not json")
    print(json.dumps({"type": "item.completed", "item": {"type": "agent_message"}}))
    print(json.dumps({"type": "turn.completed", "usage": {}}))
elif name == "agy":
    print(json.dumps({"init": {"model": model}}), flush=True)
    if spec.get("tool"):
        info = {"parameters": {"CommandLine": spec.get("command_line", "")}}
        print(json.dumps({"step_update": {"step_type": "tool", "tool_name": spec["tool"], "tool_info": info}}), flush=True)
    if quota or auth:
        sys.stderr.write(json.dumps({"error": {"message": quota or auth}}) + "\n")
        sys.exit(1)
    if spec.get("garbage"):
        print(json.dumps({"result": {"status": "success", "structured_output": "not an object"}}))
        sys.exit(0)
    print(json.dumps({"result": {"status": "success", "structured_output": result, "usage": {}}}))
else:
    if quota or auth:
        sys.stderr.write((quota or auth) + "\n")
        sys.exit(1)
    print("not json" if spec.get("garbage") else json.dumps(result))
'''


def finding(fid, **values):
    item = {"id": fid, "proposed_class": "defect", "angle": "requirement-counterexample",
            "location": "head/app/main.py:2", "claim": "synthetic claim " + fid,
            "failure_scenario": "synthetic input gives a wrong result", "evidence": "head/app/main.py:2",
            "link_kind": "none", "link": "", "introduced_by_change": "no", "regression": "no",
            "check_impact": "none", "confidence": "high"}
    item.update(values)
    return item


class World:
    """A temporary home, a synthetic repository and fake reviewer CLIs."""

    def __init__(self, case, violation):
        self.case = case
        self.violation = violation
        self.spar = spar_binary(case, violation)
        self.git = tool("git")
        self.tmp = Path(tempfile.mkdtemp(prefix="jaekit-spar-observe-"))
        case.addCleanup(shutil.rmtree, self.tmp, True)
        self.home = self.tmp / "home"
        self.xdg = self.tmp / "xdg"
        self.home.mkdir()
        self.xdg.mkdir()
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        fake = self.tmp / "fake_reviewer.py"
        fake.write_text(FAKE)
        for name in ("claude", "codex", "agy", "example-reviewer"):
            script = self.bin / name
            script.write_text(f"#!/bin/sh\nexec {sys.executable} {fake} --fake-name {name} \"$@\"\n")
            script.chmod(0o755)
        self.log = self.tmp / "invocations.jsonl"
        self.control_path = self.tmp / "control.json"
        self.repo = self.tmp / "repo"
        self.make_repo()
        self.control({})

    # Repository ---------------------------------------------------------
    def g(self, *args, check=True):
        env = dict(os.environ, GIT_AUTHOR_NAME="fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
                   GIT_COMMITTER_NAME="fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid",
                   GIT_CONFIG_NOSYSTEM="1", HOME=str(self.home))
        result = run([self.git, "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", *args],
                     self.repo, env)
        if check and result.returncode:
            raise Unavailable(f"git {' '.join(args)}: {text(result)}")
        return result.stdout.decode().strip()

    def write(self, rel, content):
        path = self.repo / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def commit(self, message):
        self.g("add", "-A")
        self.g("commit", "-qm", message)
        return self.g("rev-parse", "HEAD")

    def make_repo(self):
        self.repo.mkdir()
        self.g("init", "-q")
        self.write(".gitignore", "/docs/\n/reviews/\nsecret.local\n")
        self.write("AGENTS.md", "# Rules\n\n- Rule one: results are deterministic.\n- Rule two: errors are not violations.\n")
        self.write("app/main.py", "def greet(name):\n    return 'hello ' + name\n")
        self.write("checks/test_app.py", "def test_greet():\n    assert True\n")
        self.base = self.commit("base " + COMMIT_TOKEN)
        self.write("app/main.py", "def greet(name):\n    return 'hello, ' + name\n")
        self.write("checks/test_app.py", "def test_greet():\n    assert 'hello' in 'hello, a'\n")
        self.head = self.commit("head " + COMMIT_TOKEN)
        (self.repo / "secret.local").write_text(IGNORED_TOKEN + "\n")
        self.write("docs/specs/demo/SPEC.md",
                   "# Demo\n\n## Acceptance Criteria\n- **AC-1** greet adds a comma.\n- **AC-2** errors stay errors.\n")
        self.criteria = self.repo / "docs/specs/demo/SPEC.md"
        self.rules = self.repo / "AGENTS.md"

    def fix_commit(self, rel="app/main.py", content="def greet(name):\n    return 'hello, ' + name.strip()\n"):
        self.write(rel, content)
        return self.commit("fix " + COMMIT_TOKEN)

    # Configuration and control ----------------------------------------------
    def user_config(self, body):
        path = self.xdg / "jaekit/spar.toml"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
        return path

    def project_config(self, body):
        path = self.repo / ".jaekit/spar.toml"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
        return path

    def control(self, reviewers, angles=None, **extra):
        data = {"log": str(self.log), "head": extra.pop("head", self.head), "angles": angles or ANGLES,
                "reviewers": reviewers}
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
        return read_lines(self.log)

    def reset_log(self):
        self.log.unlink(missing_ok=True)

    # Running spar ------------------------------------------------------------
    def spar_run(self, *args, env=None, timeout=300):
        result = run([self.spar, *args], self.repo, env or self.env(), timeout=timeout)
        out = text(result)
        if result.returncode == 2 and "panic:" in out:
            raise Unavailable(f"spar panicked: {out[-2000:]}")
        if result.returncode < 0:
            raise Unavailable(f"spar ended by signal {-result.returncode}")
        return result

    def round(self, out, stage="candidate", base=None, head=None, extra=(), env=None, ok=(0, 1)):
        argv = ["run", "--stage", stage, "--base", base or self.base, "--head", head or self.head,
                "--out", out, "--criteria", self.criteria, "--rules", self.rules, *extra]
        result = self.spar_run(*argv, env=env)
        if ok is not None and result.returncode not in ok:
            self.case.fail(f"[violation:{self.violation}] spar run exited {result.returncode}: {text(result)[-1500:]}")
        return result

    def round_json(self, out):
        return load_json(self.case, self.repo / out / "round.json", self.violation, "round.json")

    def findings(self, out):
        return load_json(self.case, self.repo / out / "findings.json", self.violation, "findings.json")

    def ledger(self, out):
        try:
            return read_lines(self.repo / out / "ledger.jsonl")
        except ValueError as error:
            self.case.fail(f"[violation:{self.violation}] ledger.jsonl is not JSON lines: {error}")

    def gate(self, rounds, out="reviews/gate.json", expect=None):
        """Run the gate; it writes the gate file in every state."""
        before = (self.repo / out).read_bytes() if (self.repo / out).exists() else None
        result = self.spar_run("gate", *rounds, "--expect-head", expect or self.head, "--out", out)
        require(self.case, (self.repo / out).is_file() and (self.repo / out).read_bytes() != before, self.violation,
                f"spar gate exited {result.returncode} without writing {out}: {text(result)[-400:]}")
        data = load_json(self.case, self.repo / out, self.violation, "gate file")
        require(self.case, isinstance(data, dict), self.violation, "gate file is not an object")
        return result, data

    def output(self, out, name):
        return read_output(self.case, self.repo / out / name, self.violation, name)


def reviewers_toml(*entries):
    parts = []
    for entry in entries:
        parts.append("[[reviewers]]")
        for key, value in entry.items():
            parts.append(f"{key} = {json.dumps(value)}")
        parts.append("")
    return "\n".join(parts)


THREE = reviewers_toml({"name": "claude", "preset": "claude"}, {"name": "codex", "preset": "codex"},
                       {"name": "agy", "preset": "agy"})


# ---------------------------------------------------------------------------
# AC-1, AC-10, AC-20: rounds
# ---------------------------------------------------------------------------

class Round(unittest.TestCase):
    def test_candidate_round_runs_every_reviewer_on_one_packet(self):
        v = "round-not-recorded"
        w = World(self, v)
        w.user_config(THREE + reviewers_toml({"name": "example", "preset": "command",
                                              "argv": [str(w.bin / "example-reviewer"), "--prompt", "{prompt}",
                                                       "--workspace", "{workspace}", "--schema", "{schema}"]}))
        w.control({"claude": {"sleep": 1.5}, "codex": {"sleep": 1.5}, "agy": {"sleep": 1.5},
                   "example-reviewer": {"sleep": 1.5}},
                  default={"findings": [finding("F1", link_kind="criterion", link="AC-1")]})
        w.round("reviews/demo/01-candidate")
        meta = w.round_json("reviews/demo/01-candidate")
        calls = w.invocations()
        names = sorted(c["name"] for c in calls)
        require(self, names == ["agy", "claude", "codex", "example-reviewer"], v, f"reviewers started: {names}")
        require(self, meta.get("schema") == "review-round/v1", v, f"round schema: {meta.get('schema')}")
        packet = w.output("reviews/demo/01-candidate", "packet.md").encode()
        require(self, meta.get("packet_sha256") == sha(packet), v, "packet digest does not match packet.md")
        require(self, {c["prompt_sha256"] for c in calls} == {sha(packet)}, v,
                "reviewers did not all receive the same packet")
        require(self, len({c["cwd"] for c in calls}) == 4, v, "reviewers shared a workspace copy")
        require(self, max(c["start"] for c in calls) < min(c["end"] for c in calls), v,
                "reviewers did not run in parallel")
        head_files = set(w.g("ls-tree", "-r", "--name-only", w.head).splitlines())
        for call in calls:
            listed = {p[len("head/"):] for p in call["listing"] if p.startswith("head/")}
            require(self, head_files <= listed, v, f"{call['name']} workspace lacks head files")
        for name in ("claude", "codex", "agy", "example"):
            state = (meta.get("reviewers") or {}).get(name) or {}
            for key in ("status", "requested_model", "observed_model", "elapsed_seconds", "isolation"):
                require(self, key in state, v, f"{name} record lacks {key}")
            require(self, state.get("status") == "completed", v, f"{name}: {state.get('status')}")
        found = w.findings("reviews/demo/01-candidate")
        require(self, len(found) == 4 and all(f.get("class") in CLASSES for f in found), v,
                f"findings not classified: {found}")
        body = w.output("reviews/demo/01-candidate", "summary.md")
        require(self, all(n in body for n in ("claude", "codex", "agy", "example")), v,
                "summary.md does not show every reviewer")

    def test_workspace_has_only_tracked_files_and_no_history(self):
        v = "workspace-leaks-history"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.control({"codex": {}})
        w.round("reviews/demo/01-candidate")
        calls = w.invocations()
        require(self, len(calls) == 1, v, f"invocations: {len(calls)}")
        listing = calls[0]["listing"]
        require(self, not any(".git" in p.split("/") for p in listing), v, "workspace contains Git metadata")
        require(self, not any(p.endswith("secret.local") for p in listing), v, "workspace contains an ignored file")
        base_files = set(w.g("ls-tree", "-r", "--name-only", w.base).splitlines())
        head_files = set(w.g("ls-tree", "-r", "--name-only", w.head).splitlines())
        listed = set(listing)
        require(self, {"head/" + f for f in head_files} <= listed and {"base/" + f for f in base_files} <= listed, v,
                "workspace lacks the tracked files of the two commits")
        extra = {p for p in listed if p.startswith(("head/", "base/"))} - \
            {"head/" + f for f in head_files} - {"base/" + f for f in base_files}
        extra = {p for p in extra if not any(f.startswith(p.split("/", 1)[1] + "/")
                                             for f in head_files | base_files)}
        require(self, not extra, v, f"workspace has untracked files: {sorted(extra)[:5]}")
        hashes = calls[0].get("hashes") or {}
        for side, commit, files in (("head", w.head, head_files), ("base", w.base, base_files)):
            for name in files:
                content = run([w.git, "show", f"{commit}:{name}"], w.repo).stdout
                require(self, hashes.get(f"{side}/{name}") == sha(content), v,
                        f"{side}/{name} in the workspace is not the file of commit {commit[:12]}")
        packet = w.output("reviews/demo/01-candidate", "packet.md")
        require(self, COMMIT_TOKEN not in packet and IGNORED_TOKEN not in packet, v,
                "packet contains a commit message or an ignored file")
        require(self, "**AC-1**" in packet and "Rule one" in packet, v, "packet lacks the criteria or rules")
        require(self, "hello, " in packet, v, "packet lacks the diff")


class Angles(unittest.TestCase):
    def test_angles_are_required_and_shown(self):
        v = "angles-not-required"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "claude", "preset": "claude"}, {"name": "codex", "preset": "codex"}))
        w.control({"claude": {}, "codex": {"drop_angles": True}})
        w.round("reviews/demo/01-candidate")
        meta = w.round_json("reviews/demo/01-candidate")
        states = meta.get("reviewers") or {}
        require(self, (states.get("codex") or {}).get("status") == "invalid_output", v,
                f"result without angles: {states.get('codex')}")
        require(self, (states.get("claude") or {}).get("status") == "completed", v, f"claude: {states.get('claude')}")
        require(self, sorted((states.get("claude") or {}).get("angles_checked") or []) == sorted(ANGLES), v,
                "checked angles are not recorded")
        packet = w.output("reviews/demo/01-candidate", "packet.md")
        require(self, all(a in packet for a in ANGLES), v, "packet does not ask for every angle")
        summary = w.output("reviews/demo/01-candidate", "summary.md")
        require(self, all(a in summary for a in ANGLES), v, "zero-finding summary does not show checked angles")
        # A result that leaves out one requested angle from both lists is incomplete.
        partial = {"reviewed_head": w.head, "verdict": "no_blocking_findings",
                   "angles_checked": [{"angle": a, "result": "x"} for a in ANGLES[:3]],
                   "angles_unchecked": [{"angle": ANGLES[3], "reason": "no time"}], "findings": [], "limits": ""}
        w.reset_log()
        w.control({"claude": {"result": partial}, "codex": {}})
        w.round("reviews/demo/02-candidate-rerun")
        states = w.round_json("reviews/demo/02-candidate-rerun").get("reviewers") or {}
        require(self, (states.get("claude") or {}).get("status") == "invalid_output", v,
                f"a result missing an angle was accepted: {states.get('claude')}")
        # The project configuration can reduce the angles.
        w.project_config('angles = ["requirement-counterexample", "check-validity"]\n')
        w.reset_log()
        w.control({"claude": {}, "codex": {}}, angles=["requirement-counterexample", "check-validity"])
        w.round("reviews/other/01-candidate")
        meta = w.round_json("reviews/other/01-candidate")
        packet = w.output("reviews/other/01-candidate", "packet.md")
        require(self, ((meta.get("reviewers") or {}).get("claude") or {}).get("status") == "completed", v,
                "a result covering the reduced angles was refused")
        require(self, "premises" not in (meta.get("config") or {}).get("angles", ["premises"]), v,
                "reduced angles are not applied")


class Records(unittest.TestCase):
    def test_tracked_record_folder_is_refused(self):
        v = "tracked-records-accepted"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.control({"codex": {}})
        for out in ("app/reviews/01-candidate", "checks/01-candidate", "records/01-candidate"):
            result = w.round(out, ok=None)
            require(self, result.returncode != 0, v, f"{out}: a record folder Git would track was accepted")
            require(self, not w.invocations(), v, f"{out}: reviewers started before the refusal")
            require(self, not (w.repo / out / "round.json").exists(), v, f"{out}: a round record was written")
        w.round("reviews/demo/01-candidate")
        require(self, len(w.invocations()) == 1, v, "an ignored record folder was refused")


# ---------------------------------------------------------------------------
# AC-2, AC-3, AC-4: configuration
# ---------------------------------------------------------------------------

def config_view(case, w, v, env=None, extra=()):
    result = w.spar_run("config", "--repo", w.repo, *extra, env=env)
    require(case, result.returncode == 0, v, f"spar config exited {result.returncode}: {text(result)[-800:]}")
    try:
        return json.loads(result.stdout)
    except ValueError:
        case.fail(f"[violation:{v}] spar config did not print JSON: {text(result)[-400:]}")


def names(view):
    return [r.get("name") for r in view.get("reviewers") or []]


class Config(unittest.TestCase):
    def test_reviewer_source_order_and_digest(self):
        v = "config-source-order-wrong"
        w = World(self, v)
        explicit = w.tmp / "explicit.toml"
        explicit.write_text(reviewers_toml({"name": "codex", "preset": "codex"}))
        xdg = w.user_config(reviewers_toml({"name": "agy", "preset": "agy"}))
        home_file = w.home / ".config/jaekit/spar.toml"
        home_file.parent.mkdir(parents=True)
        home_file.write_text(reviewers_toml({"name": "claude", "preset": "claude"}))
        view = config_view(self, w, v, w.env(SPAR_CONFIG=explicit))
        require(self, names(view) == ["codex"] and str(explicit) in json.dumps(view.get("source")), v,
                f"SPAR_CONFIG is not first: {view.get('source')} {names(view)}")
        view = config_view(self, w, v)
        require(self, names(view) == ["agy"] and str(xdg) in json.dumps(view.get("source")), v,
                f"XDG file is not second: {view.get('source')}")
        xdg.unlink()
        view = config_view(self, w, v)
        require(self, names(view) == ["claude"] and str(home_file) in json.dumps(view.get("source")), v,
                f"~/.config file is not third: {view.get('source')}")
        home_file.unlink()
        clean = w.xdg / "clean-review/config.json"
        clean.parent.mkdir(parents=True)
        clean.write_text(json.dumps({"reviewers": ["codex", "agy"]}))
        view = config_view(self, w, v)
        require(self, names(view) == ["codex", "agy"] and "clean-review" in json.dumps(view.get("source")), v,
                f"clean-review list is not the fallback: {view.get('source')} {names(view)}")
        require(self, [r.get("preset") for r in view["reviewers"]] == ["codex", "agy"], v,
                "clean-review names are not read as presets")
        clean.unlink()
        view = config_view(self, w, v)
        require(self, names(view) == ["claude", "codex", "agy"], v, f"default list: {names(view)}")
        w.user_config("reviewers = []\n")
        view = config_view(self, w, v)
        require(self, names(view) == ["native"], v, f"an empty list is not native only: {names(view)}")
        w.user_config(THREE)
        first = config_view(self, w, v)
        again = config_view(self, w, v)
        require(self, first.get("digest") and first.get("digest") == again.get("digest"), v,
                "the applied configuration digest is missing or not deterministic")
        override = config_view(self, w, v, extra=("--reviewers", "agy"))
        require(self, names(override) == ["agy"] and "--reviewers" in json.dumps(override), v,
                f"--reviewers does not take precedence: {names(override)}")
        require(self, override.get("digest") != first.get("digest"), v, "the digest ignores the override")
        w.control({"claude": {}, "codex": {}, "agy": {}})
        w.round("reviews/demo/01-candidate", extra=("--dry-run",))
        meta = w.round_json("reviews/demo/01-candidate")
        config = meta.get("config") or {}
        require(self, config.get("digest") == first.get("digest") and config.get("source") == first.get("source"), v,
                f"round record lacks the config source and digest: {config}")
        w.round("reviews/demo/02-candidate", extra=("--dry-run", "--reviewers", "agy"))
        meta = w.round_json("reviews/demo/02-candidate")
        require(self, "--reviewers" in json.dumps(meta.get("config")), v, "round record does not show the override")

    def test_project_config_cannot_define_reviewers(self):
        v = "project-config-overreach"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.control({"codex": {}})
        cases = {
            "reviewers": '[[reviewers]]\nname = "x"\npreset = "codex"\n',
            "command": 'command = ["sh", "-c", "touch pwned"]\n',
            "argv": 'argv = ["sh"]\n',
            "binary": 'binary = "/tmp/codex"\n',
            "model": 'model = "cheap"\n',
            "fallback_model": 'fallback_model = "cheap"\n',
        }
        for key, body in cases.items():
            w.project_config(body)
            w.reset_log()
            out = f"reviews/p-{key}/01-candidate"
            result = w.round(out, ok=None)
            require(self, result.returncode != 0, v, f"project config set {key} and spar accepted it")
            require(self, key in text(result), v, f"the refusal does not name {key}: {text(result)[-300:]}")
            require(self, not w.invocations() and not (w.repo / out / "round.json").exists(), v,
                    f"reviewers ran despite a project {key}")
        w.project_config('angles = ["requirement-counterexample"]\nrules = ["AGENTS.md"]\n'
                         'check_paths = ["checks/**"]\n[caps]\nplan = 1\nplan_delta = 1\ncode = 2\n')
        w.reset_log()
        w.control({"codex": {}}, angles=["requirement-counterexample"])
        w.round("reviews/allowed/01-candidate")
        meta = w.round_json("reviews/allowed/01-candidate")
        config = meta.get("config") or {}
        require(self, config.get("check_paths") == ["checks/**"] and (config.get("caps") or {}).get("code") == 2, v,
                f"allowed project keys are not applied: {config}")

    def test_config_errors_name_file_and_location(self):
        v = "config-error-unreported"
        w = World(self, v)
        w.control({"codex": {}, "claude": {}})
        # The location is the line of the offending key or of its [[reviewers]] header.
        cases = [
            ('[[reviewers]]\nname = "codex"\npreset = "codex\n', {3}, "syntax"),
            ('[[reviewers]]\nname = "codex"\npreset = "codex"\nmodle = "x"\n', {4}, "modle"),
            ('[[reviewers]]\nname = "codex"\npreset = "codex"\n\n[[reviewers]]\nname = "codex"\npreset = "claude"\n',
             {5, 6}, "codex"),
            ('[[reviewers]]\nname = "g"\npreset = "gemini"\n', {1, 3}, "gemini"),
            ('[[reviewers]]\nname = "c"\npreset = "command"\n', {1, 3}, "argv"),
            ('# comment\ntimeout = 3\n', {2}, "timeout"),
        ]

        def names_line(message, lines):
            return any(re.search(rf"(?:line {n}\b|:{n}(?::\d+)?:)", message) for n in lines)
        for body, lines, token in cases:
            path = w.user_config(body)
            w.reset_log()
            out = "reviews/err/01-candidate"
            result = w.round(out, ok=None)
            message = text(result)
            require(self, result.returncode != 0, v, f"config error accepted ({token}): {body!r}")
            require(self, path_named(message, path), v, f"error does not name the file ({token}): {message[-300:]}")
            require(self, names_line(message, lines), v,
                    f"error does not name line {sorted(lines)} ({token}): {message[-300:]}")
            require(self, token.lower() in message.lower(), v, f"error does not name {token}: {message[-300:]}")
            require(self, not w.invocations() and not (w.repo / out / "round.json").exists(), v,
                    f"reviewers ran despite a config error ({token})")
        project = w.project_config('angles = ["requirement-counterexample"]\nunknown_key = 1\n')
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        result = w.round("reviews/err/02-candidate", ok=None)
        message = text(result)
        require(self, result.returncode != 0 and path_named(message, project) and "unknown_key" in message
                and names_line(message, {2}), v,
                f"project config error is not reported with file and line: {message[-300:]}")

    def test_full_toml_syntax_is_accepted(self):
        v = "toml-syntax-refused"
        w = World(self, v)
        body = (
            "# a comment line\r\n"
            "timeout_seconds = 1_800 # underscore integer\r\n"
            "\r\n"
            "[[reviewers]]\r\n"
            "name = 'codex'\r\n"
            "\"preset\" = \"\"\"codex\"\"\"\r\n"
            "model = '''gpt-x'''\r\n"
            "timeout_seconds = 0x384\r\n"
            "\r\n"
            "[[reviewers]]\r\n"
            "name = \"cmd\"\r\n"
            "preset = \"command\"\r\n"
            "argv = [\r\n"
            "  \"" + str(w.bin / "example-reviewer").replace("\\", "\\\\") + "\",\r\n"
            "  \"--prompt\", \"{prompt}\", # trailing comment\r\n"
            "  \"--workspace\", '{workspace}', \"--schema\", \"{schema}\",\r\n"
            "]\r\n"
            "timeout_seconds = 0o1600\r\n"
        )
        w.user_config(body)
        view = config_view(self, w, v)
        require(self, names(view) == ["codex", "cmd"], v, f"TOML 1.0 forms were not read: {names(view)}")
        timeouts = [r.get("timeout_seconds") for r in view["reviewers"]]
        require(self, timeouts == [900, 896], v, f"integer forms were misread: {timeouts}")
        w.user_config('reviewers = [ { name = "agy", preset = "agy", fallback_model = "b" },\n'
                      '  { name = "codex", preset = "codex" } ]\n')
        view = config_view(self, w, v)
        require(self, names(view) == ["agy", "codex"], v, f"inline tables were not read: {names(view)}")
        w.project_config('caps.code = 2\n"check_paths" = [ "checks/**", ]\n[caps]\n')
        result = w.spar_run("config", "--repo", w.repo)
        require(self, result.returncode != 0, v, "a TOML 1.0 error (table defined twice) was accepted")
        w.project_config('caps.code = 2\n"check_paths" = [ "checks/**", ]\n')
        view = config_view(self, w, v)
        require(self, (view.get("caps") or {}).get("code") == 2 and view.get("check_paths") == ["checks/**"], v,
                f"dotted and quoted keys were not read: {view.get('caps')}")


# ---------------------------------------------------------------------------
# AC-5 .. AC-9: reviewers
# ---------------------------------------------------------------------------

CLAUDE_FLAGS = ["-p", "--safe-mode", "--restricted", "--strict-mcp-config", "--no-session-persistence",
                "--disable-slash-commands"]
CODEX_FLAGS = ["exec", "--ignore-user-config", "--ignore-rules", "--ephemeral", "--skip-git-repo-check", "--json", "-"]
CODEX_DISABLED = ["apps", "browser_use", "browser_use_external", "computer_use", "in_app_browser",
                  "image_generation", "memories", "multi_agent", "plugins", "hooks"]
AGY_FLAGS = ["--sandbox", "--disable-slash-commands"]


def pairs(argv):
    return {(a, b) for a, b in zip(argv, argv[1:])}


class Presets(unittest.TestCase):
    def test_presets_run_read_only_and_verified(self):
        v = "preset-isolation-missing"
        w = World(self, v)
        w.user_config(THREE)
        w.control({"claude": {}, "codex": {}, "agy": {}})
        w.round("reviews/demo/01-candidate")
        calls = {c["name"]: c["argv"] for c in w.invocations()}
        require(self, set(calls) == {"claude", "codex", "agy"}, v, f"reviewers: {sorted(calls)}")
        claude, codex, agy = calls["claude"], calls["codex"], calls["agy"]
        require(self, all(f in claude for f in CLAUDE_FLAGS) and ("--tools", "Read,Grep,Glob") in pairs(claude)
                and ("--permission-mode", "dontAsk") in pairs(claude), v, f"claude protections: {claude}")
        require(self, all(f in codex for f in CODEX_FLAGS) and ("-s", "read-only") in pairs(codex)
                and ("-c", 'web_search="disabled"') in pairs(codex)
                and all(("--disable", f) in pairs(codex) for f in CODEX_DISABLED), v, f"codex protections: {codex}")
        require(self, all(f in agy for f in AGY_FLAGS), v, f"agy protections: {agy}")
        states = w.round_json("reviews/demo/01-candidate").get("reviewers") or {}
        for name in ("claude", "codex", "agy"):
            require(self, (states.get(name) or {}).get("isolation") == "verified", v,
                    f"{name} isolation: {(states.get(name) or {}).get('isolation')}")
            require(self, (states.get(name) or {}).get("required") is True, v, f"{name} is not required")

    def test_tool_use_and_workspace_change_discard_results(self):
        v = "violation-result-used"
        w = World(self, v)
        w.user_config(THREE)
        bad = [finding("F1", link_kind="criterion", link="AC-1")]
        w.control({"claude": {"tool": "Bash", "findings": bad}, "codex": {"tool": "web_search", "findings": bad},
                   "agy": {"tool": "run_command", "command_line": "rm -rf head", "findings": bad}})
        w.round("reviews/demo/01-candidate")
        states = w.round_json("reviews/demo/01-candidate").get("reviewers") or {}
        for name in ("claude", "codex", "agy"):
            require(self, (states.get(name) or {}).get("status") == "tool_violation", v,
                    f"{name} forbidden tool: {states.get(name)}")
        require(self, w.findings("reviews/demo/01-candidate") == [], v, "findings of a tool violation were used")
        w.reset_log()
        w.control({"claude": {"modify": True, "findings": bad}, "codex": {"modify": True, "findings": bad},
                   "agy": {"modify": True, "findings": bad}})
        w.round("reviews/demo/02-candidate-rerun")
        states = w.round_json("reviews/demo/02-candidate-rerun").get("reviewers") or {}
        for name in ("claude", "codex", "agy"):
            require(self, (states.get(name) or {}).get("status") == "workspace_modified", v,
                    f"{name} workspace change: {states.get(name)}")
        require(self, w.findings("reviews/demo/02-candidate-rerun") == [], v,
                "findings of a modified workspace were used")
        require(self, (w.repo / "app/main.py").read_text().startswith("def greet"), v,
                "a reviewer changed the repository itself")


class Command(unittest.TestCase):
    def test_command_reviewer_runs_without_shell_and_is_never_required(self):
        v = "command-reviewer-counted"
        w = World(self, v)
        literal = "literal;$(touch pwned)*"
        argv = [str(w.bin / "example-reviewer"), "--prompt", "{prompt}", "--workspace", "{workspace}",
                "--schema", "{schema}", "--note", literal]
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"},
                                     {"name": "example", "preset": "command", "argv": argv}))
        w.project_config('check_paths = ["checks/**"]\n')
        w.control({"codex": {}, "example-reviewer": {"findings": [
            finding("F1", location="head/checks/test_app.py:2"),
            finding("F2", link_kind="criterion", link="AC-1")]}})
        w.round("reviews/demo/01-candidate")
        calls = {c["name"]: c for c in w.invocations()}
        call = calls.get("example-reviewer")
        require(self, call is not None, v, "the command reviewer did not run")
        packet = w.output("reviews/demo/01-candidate", "packet.md").encode()
        require(self, call["argv"][-1] == literal and not (Path(call["cwd"]) / "pwned").exists()
                and not (w.repo / "pwned").exists(), v, f"arguments went through a shell: {call['argv']}")
        require(self, call["prompt_sha256"] == sha(packet), v, "the {prompt} file is not the packet")
        require(self, call.get("workspace") and Path(call["workspace"]).name and "{" not in call["workspace"], v,
                "{workspace} was not replaced")
        require(self, call.get("schema") and "{" not in call["schema"], v, "{schema} was not replaced")
        meta = w.round_json("reviews/demo/01-candidate")
        state = (meta.get("reviewers") or {}).get("example") or {}
        require(self, state.get("isolation") == "unverified" and state.get("required") is False, v,
                f"command reviewer record: {state}")
        found = {f["reviewer_id"]: f for f in w.findings("reviews/demo/01-candidate") if f.get("reviewer") == "example"}
        require(self, (found.get("F1") or {}).get("class") == "hardening" and (found.get("F2") or {}).get("class")
                == "defect", v, f"command findings were not classified by the same rules: {found}")
        for fid in list(found):
            full = f"example-{fid}"
            w.spar_run("triage", "reviews/demo/01-candidate", full, "reject", "--evidence", "synthetic disproof")
        result, gate = w.gate(["reviews/demo/01-candidate"])
        require(self, gate.get("state") == "complete" and result.returncode == 0, v,
                f"gate with a completed required reviewer: {gate and gate.get('state')} {text(result)[-300:]}")
        rounds = gate.get("rounds") or [{}]
        require(self, rounds[0].get("required_reviewers") == 1, v, f"required count: {rounds[0]}")
        # A failing command reviewer never degrades the gate, and a round with
        # only command reviewers has no required reviewer.
        w.reset_log()
        w.control({"codex": {}, "example-reviewer": {"crash": "synthetic crash"}})
        w.round("reviews/two/01-candidate")
        result, gate = w.gate(["reviews/two/01-candidate"], out="reviews/two/gate.json")
        require(self, gate.get("state") == "complete", v,
                f"a failed command reviewer changed the gate: {gate and gate.get('state')}")
        w.user_config(reviewers_toml({"name": "example", "preset": "command", "argv": argv}))
        w.reset_log()
        w.control({"example-reviewer": {}})
        w.round("reviews/three/01-candidate")
        result, gate = w.gate(["reviews/three/01-candidate"], out="reviews/three/gate.json")
        require(self, gate.get("state") != "complete" and result.returncode != 0, v,
                f"a round with only command reviewers completed the gate: {gate and gate.get('state')}")


def transcript(path, round_dir, reply, extra_tool=None, git_status=False):
    prompt = str((round_dir / "native/prompt.md").resolve())
    lines = [{"type": "attachment", "attachment": {"type": "environment", "snapshot": {
        "workingDirectory": str(path.parent), "isGitRepo": False}}}]
    if git_status:
        lines.append({"type": "attachment", "attachment": {"type": "session_context",
                                                           "context": {"userEmail": "x", "gitStatus": "y"}}})
    lines.append({"type": "assistant", "cwd": str(path.parent), "message": {
        "id": "m1", "role": "assistant", "model": "synthetic-model",
        "content": [{"type": "tool_use", "id": "u1", "name": "Read", "input": {"file_path": prompt}}]}})
    if extra_tool:
        lines.append({"type": "assistant", "cwd": str(path.parent), "message": {
            "id": "m2", "role": "assistant", "model": "synthetic-model",
            "content": [{"type": "tool_use", "id": "u2", "name": extra_tool, "input": {"command": "ls"}}]}})
    lines.append({"type": "assistant", "cwd": str(path.parent), "message": {
        "id": "m3", "role": "assistant", "model": "synthetic-model", "stop_reason": "end_turn",
        "content": [{"type": "text", "text": json.dumps(reply)}]}})
    path.write_text("\n".join(json.dumps(line) for line in lines) + "\n")
    agent = path.name[len("agent-"):-len(".jsonl")]
    (path.parent / f"agent-{agent}.meta.json").write_text(json.dumps({"agentType": "clean-reviewer"}))
    tools = ["Read", "Grep", "Glob"] + ([extra_tool] if extra_tool else [])
    (path.parent / f"agent-{agent}.prefix.json").write_text(json.dumps({"state": {"toolNames": tools}}))


class Native(unittest.TestCase):
    def test_native_prepares_and_records_host_reviewer(self):
        v = "native-not-recorded"
        w = World(self, v)
        w.user_config("reviewers = []\n")
        result = w.round("reviews/demo/01-candidate")
        require(self, result.returncode == 0, v, f"a pending native round exited {result.returncode}")
        round_dir = w.repo / "reviews/demo/01-candidate"
        prompt = round_dir / "native/prompt.md"
        packet = w.output("reviews/demo/01-candidate", "packet.md")
        prompt_text = w.output("reviews/demo/01-candidate", "native/prompt.md")
        require(self, packet.strip() in prompt_text, v,
                "native prompt does not carry the packet")
        require(self, (round_dir / "native/workspace/head/app/main.py").is_file(), v, "native workspace is missing")
        state = (w.round_json("reviews/demo/01-candidate").get("reviewers") or {}).get("native") or {}
        require(self, state.get("status") == "pending" and state.get("isolation") == "host"
                and state.get("required") is True, v, f"native pending state: {state}")
        result, gate = w.gate(["reviews/demo/01-candidate"])
        require(self, gate.get("state") == "degraded", v,
                f"a pending native reviewer did not degrade the gate: {gate and gate.get('state')}")
        reply = {"reviewed_head": w.head, "verdict": "no_blocking_findings",
                 "angles_checked": [{"angle": a, "result": "ok"} for a in ANGLES], "angles_unchecked": [],
                 "findings": [], "limits": "none"}
        reply_file = w.tmp / "reply.json"
        reply_file.write_text(json.dumps(reply))
        result = w.spar_run("record", "reviews/demo/01-candidate", "native", "--result", reply_file,
                            "--model", "host-model", "--agent", "synthetic subagent")
        require(self, result.returncode == 0, v, f"record --result exited {result.returncode}: {text(result)[-400:]}")
        state = (w.round_json("reviews/demo/01-candidate").get("reviewers") or {}).get("native") or {}
        require(self, state.get("status") == "completed" and state.get("isolation") == "host", v,
                f"native result was not recorded: {state}")
        result, gate = w.gate(["reviews/demo/01-candidate"], out="reviews/demo/gate2.json")
        require(self, gate.get("state") == "complete", v,
                f"a completed native reviewer is not required-complete: {gate and gate.get('state')}")
        # A transcript that shows another tool and added context is recorded with warnings.
        w.round("reviews/demo/02-candidate-again")
        sub = w.tmp / "session/subagents"
        sub.mkdir(parents=True)
        path = sub / "agent-abc123.jsonl"
        transcript(path, w.repo / "reviews/demo/02-candidate-again", reply, extra_tool="Bash", git_status=True)
        result = w.spar_run("record", "reviews/demo/02-candidate-again", "native", "--transcript", path)
        require(self, result.returncode == 0, v, f"record --transcript exited {result.returncode}: {text(result)[-400:]}")
        state = (w.round_json("reviews/demo/02-candidate-again").get("reviewers") or {}).get("native") or {}
        warnings = " ".join(state.get("warnings") or [])
        require(self, state.get("status") == "completed" and "Bash" in warnings and "git status" in warnings.lower(),
                v, f"transcript warnings were not recorded: {state}")


class Fallback(unittest.TestCase):
    def test_fallback_model_runs_once_on_quota_refusal(self):
        v = "fallback-not-applied"
        w = World(self, v)
        quota = "RESOURCE_EXHAUSTED (code 429): Individual quota reached. Resets in 2h13m."
        w.user_config(reviewers_toml({"name": "agy", "preset": "agy", "model": "primary-m",
                                      "fallback_model": "fallback-m"}))
        w.control({"agy": {"models": {"primary-m": {"quota": quota}, "fallback-m": {}}}})
        w.round("reviews/demo/01-candidate")
        state = (w.round_json("reviews/demo/01-candidate").get("reviewers") or {}).get("agy") or {}
        calls = w.invocations()
        require(self, [c["model"] for c in calls] == ["primary-m", "fallback-m"], v,
                f"models run: {[c['model'] for c in calls]}")
        require(self, len({c["prompt_sha256"] for c in calls}) == 1, v, "the fallback got a different packet")
        attempts = state.get("attempts") or []
        require(self, state.get("status") == "completed" and state.get("fallback_used") is True
                and state.get("requested_model") == "primary-m" and len(attempts) == 2, v, f"agy record: {state}")
        require(self, attempts[0].get("model") == "primary-m" and attempts[0].get("status") == "unavailable"
                and "2h13m" in json.dumps(attempts[0]), v, f"refusal and reset are not recorded: {attempts[0]}")
        summary = w.output("reviews/demo/01-candidate", "summary.md")
        require(self, all(t in summary for t in ("primary-m", "fallback-m", "2h13m")), v,
                "summary does not show the designated model, reset and fallback")
        for case, control, expected in (
                ("both refused", {"models": {"primary-m": {"quota": quota}, "fallback-m": {"quota": quota}}},
                 ["primary-m", "fallback-m"]),
                ("auth is not a quota", {"models": {"primary-m": {"auth": "401 Unauthorized: run agy login"}}},
                 ["primary-m"])):
            w.reset_log()
            w.control({"agy": control})
            out = "reviews/" + case.replace(" ", "-") + "/01-candidate"
            w.round(out)
            state = (w.round_json(out).get("reviewers") or {}).get("agy") or {}
            require(self, [c["model"] for c in w.invocations()] == expected and state.get("status") == "unavailable",
                    v, f"{case}: {[c['model'] for c in w.invocations()]} {state.get('status')}")
        # An empty fallback_model turns a preset's default fallback off.
        w.user_config(reviewers_toml({"name": "agy", "preset": "agy", "model": "primary-m", "fallback_model": ""}))
        w.reset_log()
        w.control({"agy": {"models": {"primary-m": {"quota": quota}}}})
        w.round("reviews/nofallback/01-candidate")
        state = (w.round_json("reviews/nofallback/01-candidate").get("reviewers") or {}).get("agy") or {}
        require(self, len(w.invocations()) == 1 and state.get("status") == "unavailable", v,
                f"without a fallback: {state}")


class Failures(unittest.TestCase):
    def test_unfinished_reviewers_are_never_zero_findings(self):
        v = "failure-hidden"
        w = World(self, v)
        missing = w.tmp / "no-such-cli"
        not_exec = w.tmp / "not-executable"
        not_exec.write_text("plain file\n")
        w.user_config(
            reviewers_toml({"name": "claude", "preset": "claude", "timeout_seconds": 2},
                           {"name": "codex", "preset": "codex"},
                           {"name": "agy", "preset": "agy", "binary": str(missing)},
                           {"name": "blocked", "preset": "codex", "binary": str(not_exec)}))
        w.control({"claude": {"sleep": 8}, "codex": {"garbage": True}})
        w.round("reviews/demo/01-candidate")
        meta = w.round_json("reviews/demo/01-candidate")
        states = meta.get("reviewers") or {}
        expected = {"claude": "timed_out", "codex": "invalid_output", "agy": "unavailable", "blocked": "failed"}
        for name, status in expected.items():
            state = states.get(name) or {}
            require(self, state.get("status") == status, v, f"{name}: {state.get('status')} (expected {status})")
            require(self, state.get("reason"), v, f"{name}: no reason recorded")
            require(self, state.get("findings") is None, v, f"{name}: recorded findings {state.get('findings')}")
        require(self, str(missing) in states["agy"]["reason"] or "not found" in states["agy"]["reason"], v,
                "the missing CLI is not quoted")
        summary = w.output("reviews/demo/01-candidate", "summary.md")
        for name, status in expected.items():
            line = next((l for l in summary.splitlines() if l.startswith(f"- {name}:")), "")
            require(self, status in line and "0 findings" not in line and "no findings" not in line.lower(), v,
                    f"summary line for {name}: {line!r}")
        result, gate = w.gate(["reviews/demo/01-candidate"])
        require(self, gate.get("state") == "degraded", v,
                f"gate after unfinished reviewers: {gate and gate.get('state')}")


# ---------------------------------------------------------------------------
# AC-11, AC-12: classes and dispositions
# ---------------------------------------------------------------------------

CLASS_CASES = {
    "F1": ({"location": "head/checks/test_app.py:2", "link_kind": "criterion", "link": "AC-1"}, "hardening"),
    "F2": ({"location": "head/checks/test_app.py:2", "check_impact": "false_pass"}, "defect"),
    "F3": ({"location": "head/checks/test_app.py:2", "check_impact": "contract_violation",
            "link_kind": "rule", "link": "AGENTS.md:3"}, "defect"),
    "F4": ({"location": "head/checks/test_app.py:2", "check_impact": "contract_violation",
            "link_kind": "contract", "link": "contracts/none.md:1"}, "hardening"),
    "F5": ({"proposed_class": "note", "introduced_by_change": "yes", "regression": "yes"}, "defect"),
    "F6": ({"link_kind": "criterion", "link": "AC-99"}, "note"),
    "F7": ({"link_kind": "criterion", "link": "AC-2"}, "defect"),
    "F8": ({"proposed_class": "requirement-question"}, "requirement-question"),
    "F9": ({"proposed_class": "note"}, "note"),
    "F10": ({"location": "head/checks/test_app.py:2", "check_impact": "meaning_inverted"}, "defect"),
    # A rule document that exists, at a line it does not have, is not a rule location.
    "F11": ({"location": "head/checks/test_app.py:2", "check_impact": "contract_violation",
             "link_kind": "rule", "link": "AGENTS.md:999"}, "hardening"),
    "F12": ({"link_kind": "rule", "link": "AGENTS.md:999"}, "note"),
}


class Classes(unittest.TestCase):
    def test_findings_are_classified_by_rule(self):
        v = "finding-unclassified"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.project_config('check_paths = ["checks/**"]\n')
        w.control({"codex": {"findings": [finding(fid, **values) for fid, (values, _) in CLASS_CASES.items()]}})
        w.round("reviews/demo/01-candidate")
        found = {f.get("reviewer_id"): f for f in w.findings("reviews/demo/01-candidate")}
        for fid, (_, expected) in CLASS_CASES.items():
            got = (found.get(fid) or {}).get("class")
            require(self, got == expected, v, f"{fid}: class {got}, expected {expected}")
        require(self, found["F6"].get("link_kind") == "none", v, "an unverifiable AC link was kept")
        require(self, found["F4"].get("link_kind") == "none", v, "an unverifiable contract link was kept")
        require(self, found["F12"].get("link_kind") == "none", v, "a rule link to a missing line was kept")
        require(self, found["F3"].get("link_kind") == "rule", v, "a real rule location was dropped")
        require(self, found["F7"].get("link_kind") == "criterion" and found["F7"].get("link") == "AC-2", v,
                "a real AC link was dropped")
        ledger = w.ledger("reviews/demo/01-candidate")
        auto = {e.get("finding") for e in ledger if e.get("disposition") == "defer" and e.get("auto") is True}
        hardening = {f["id"] for f in found.values() if f.get("class") == "hardening"}
        require(self, hardening and auto == hardening, v, f"hardening is not deferred automatically: {auto}")


class Ledger(unittest.TestCase):
    def test_dispositions_follow_class_rules(self):
        v = "disposition-unrestricted"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.project_config('check_paths = ["checks/**"]\n')
        w.control({"codex": {"findings": [
            finding("F1", link_kind="criterion", link="AC-1"),
            finding("F2", link_kind="criterion", link="AC-2"),
            finding("F3", proposed_class="requirement-question"),
            finding("F4", location="head/checks/test_app.py:2"),
            finding("F5", link_kind="criterion", link="AC-1")]}})
        out = "reviews/demo/01-candidate"
        w.round(out)
        fix = w.fix_commit()

        def triage(*args):
            before = len(w.ledger(out))
            result = w.spar_run("triage", out, *args)
            return result, len(w.ledger(out)) - before

        refused = [
            ("codex-F1", "fix", "--commit", fix),
            ("codex-F1", "fix", "--commit", fix, "--failed-before", "test_x failed at " + w.head[:12]),
            ("codex-F1", "fix", "--failed-before", "a", "--passed-after", "b"),
            ("codex-F2", "reject"),
            ("codex-F3", "answer"),
            ("codex-F3", "reject", "--evidence", "it is fine"),
            ("codex-F3", "defer", "--to", "follow-ups"),
            ("codex-F3", "fix", "--commit", fix, "--failed-before", "a", "--passed-after", "b"),
            ("codex-F3", "duplicate", "--of", "codex-F1"),
            ("codex-F4", "fix", "--commit", fix, "--failed-before", "a", "--passed-after", "b"),
            ("codex-F5", "duplicate", "--of", "codex-F9"),
            ("codex-F5", "duplicate", "--of", "codex-F5"),
        ]
        for args in refused:
            result, added = triage(*args)
            require(self, result.returncode != 0 and added == 0, v, f"accepted: {args}")
        accepted = [
            ("codex-F1", "fix", "--commit", fix, "--failed-before", "test_greet failed at the reviewed head",
             "--passed-after", "test_greet passed at the fix commit"),
            ("codex-F2", "reject", "--evidence", "head/app/main.py:2 already handles it"),
            ("codex-F3", "answer", "--user-quote", QUOTE),
            ("codex-F4", "fix", "--commit", fix, "--failed-before", "a", "--passed-after", "b", "--user-quote", QUOTE),
            ("codex-F5", "duplicate", "--of", "codex-F1"),
        ]
        for args in accepted:
            result, added = triage(*args)
            require(self, result.returncode == 0 and added == 1, v, f"refused: {args}: {text(result)[-300:]}")
        entry = next(e for e in w.ledger(out) if e.get("finding") == "codex-F1" and e.get("disposition") == "fix")
        require(self, entry.get("commit") == fix and entry.get("failed_before") and entry.get("passed_after"), v,
                f"fix record is not structured: {entry}")


# ---------------------------------------------------------------------------
# AC-13, AC-14, AC-15: caps and gate
# ---------------------------------------------------------------------------

class Caps(unittest.TestCase):
    def test_stage_caps_and_fixes_chain(self):
        v = "stage-cap-missing"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.control({"codex": {}})
        s = "reviews/demo"

        def attempt(out, stage, base, head, allowed):
            w.reset_log()
            w.control({"codex": {}}, head=head)
            result = w.round(f"{s}/{out}", stage=stage, base=base, head=head, ok=None,
                             extra=("--subject", str(w.criteria)) if stage.startswith("plan") else ())
            ran = bool(w.invocations())
            if allowed:
                require(self, result.returncode == 0 and ran, v,
                        f"{out}: an allowed {stage} round was refused: {text(result)[-300:]}")
            else:
                require(self, result.returncode != 0 and not ran and not (w.repo / s / out / "round.json").exists(),
                        v, f"{out}: a {stage} round past the cap ran")
        heads = [w.head]
        for i in range(5):
            heads.append(w.fix_commit(content=f"def greet(name):\n    return 'hello, ' + name  # {i}\n"))
        attempt("01-plan", "plan", w.base, heads[0], True)
        attempt("02-plan", "plan", w.base, heads[1], False)
        attempt("03-plan-delta", "plan-delta", heads[0], heads[1], True)
        attempt("04-plan-delta", "plan-delta", heads[1], heads[2], False)
        result = w.spar_run("extend", s, "--stage", "plan-delta", "--by", "1", "--user-quote", QUOTE)
        require(self, result.returncode == 0, v, f"extend refused: {text(result)[-300:]}")
        attempt("05-plan-delta", "plan-delta", heads[1], heads[2], True)
        attempt("06-candidate", "candidate", w.base, heads[2], True)
        attempt("07-fixes", "fixes", heads[1], heads[3], False)
        attempt("08-fixes", "fixes", heads[2], heads[3], True)
        attempt("09-fixes", "fixes", heads[3], heads[4], True)
        attempt("10-fixes", "fixes", heads[4], heads[5], False)
        result = w.spar_run("extend", s, "--stage", "code", "--by", "1")
        require(self, result.returncode != 0, v, "an extension without the user's words was accepted")
        result = w.spar_run("extend", s, "--stage", "code", "--by", "1", "--user-quote", QUOTE)
        require(self, result.returncode == 0, v, f"extend refused: {text(result)[-300:]}")
        attempt("11-fixes", "fixes", heads[4], heads[5], True)
        # The same packet may run again only for reviewers that did not complete
        # it; a plan document edited in place (no new commit) is a new packet.
        subject = w.repo / "docs/specs/demo/PLAN.md"
        subject.write_text("plan v1\n")
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}, {"name": "claude", "preset": "claude"}))

        def plan_run(out, reviewers, allowed):
            w.reset_log()
            extra = ["--subject", str(subject)] + (["--reviewers", reviewers] if reviewers else [])
            result = w.round(f"reviews/rerun/{out}", stage="plan", base=w.base, head=heads[0], ok=None,
                             extra=tuple(extra))
            ran = bool(w.invocations())
            if allowed:
                require(self, result.returncode == 0 and ran, v, f"{out}: an allowed rerun was refused: "
                        f"{text(result)[-300:]}")
            else:
                require(self, result.returncode != 0 and not ran
                        and not (w.repo / "reviews/rerun" / out / "round.json").exists(), v,
                        f"{out}: a plan run past the cap ran")
        w.control({"codex": {"quota": "429 Too Many Requests"}, "claude": {}}, head=heads[0])
        plan_run("01-plan", None, True)
        # codex has not completed, but an edited plan is another packet.
        subject.write_text("plan v2\n")
        plan_run("02-plan-edited-unfinished", "codex", False)
        subject.write_text("plan v1\n")
        w.control({"codex": {}, "claude": {}}, head=heads[0])
        plan_run("03-plan-codex", "codex", True)
        plan_run("04-plan-claude", "claude", False)
        plan_run("05-plan-all", None, False)
        subject.write_text("plan v2\n")
        plan_run("06-plan-edited", "codex", False)


class Gate(unittest.TestCase):
    def three_round(self, w, out, control, base=None, head=None, stage="candidate"):
        w.reset_log()
        w.control(control, head=head or w.head)
        w.round(out, stage=stage, base=base, head=head)

    def test_gate_states_and_fields(self):
        v = "gate-missing"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}, {"name": "claude", "preset": "claude"}))
        w.project_config('check_paths = ["checks/**"]\n')
        self.three_round(w, "reviews/demo/01-candidate", {"codex": {"findings": [
            finding("F1", link_kind="criterion", link="AC-1"),
            finding("F2", location="head/checks/test_app.py:2"),
            finding("F3", proposed_class="requirement-question")]}, "claude": {}})
        result, gate = w.gate(["reviews/demo/01-candidate"])
        require(self, gate.get("schema") == "review-gate/v1", v, f"gate file: {gate}")
        require(self, gate.get("state") == "blocked" and result.returncode == 4, v,
                f"open findings: {gate.get('state')} exit {result.returncode}")
        require(self, (gate.get("open") or {}).get("defect") == 1 and (gate.get("open") or {}).get(
            "requirement-question") == 1 and gate.get("auto_deferred") == 1, v, f"open counts: {gate.get('open')}")
        first_digest = sha((w.repo / "reviews/gate.json").read_bytes())
        fix = w.fix_commit()
        w.spar_run("triage", "reviews/demo/01-candidate", "codex-F1", "fix", "--commit", fix,
                   "--failed-before", "test failed before", "--passed-after", "test passed after")
        w.spar_run("triage", "reviews/demo/01-candidate", "codex-F3", "answer", "--user-quote", QUOTE)
        result, gate = w.gate(["reviews/demo/01-candidate"], expect=fix)
        require(self, gate.get("state") == "blocked", v, f"an unreviewed head completed: {gate.get('state')}")
        require(self, gate.get("previous_gate_digest") == first_digest, v,
                f"previous gate digest: {gate.get('previous_gate_digest')} != {first_digest}")
        self.three_round(w, "reviews/demo/02-fixes", {"codex": {}, "claude": {}}, base=w.head, head=fix,
                         stage="fixes")
        result, gate = w.gate(["reviews/demo/01-candidate", "reviews/demo/02-fixes"], expect=fix)
        require(self, gate.get("state") == "complete" and result.returncode == 0, v,
                f"complete gate: {gate.get('state')} {gate.get('reasons')}")
        require(self, gate.get("last_reviewed_head") == fix, v, "last reviewed head")
        rounds = gate.get("rounds") or []
        metas = [w.round_json("reviews/demo/01-candidate"), w.round_json("reviews/demo/02-fixes")]
        require(self, len(rounds) == 2 and all(
            r.get("criteria_digest") == m.get("criteria_digest") and r.get("packet_digest") == m.get("packet_sha256")
            and r.get("config_digest") == (m.get("config") or {}).get("digest")
            and r.get("requested_reviewers") == ["codex", "claude"] and r.get("required_reviewers") == 2
            for r, m in zip(rounds, metas)), v, f"round fields: {rounds}")
        require(self, (gate.get("open") or {}) == {"defect": 0, "requirement-question": 0}
                and gate.get("auto_deferred") == 1, v, f"counts: {gate.get('open')} {gate.get('auto_deferred')}")
        # One required reviewer unavailable: degraded.
        self.three_round(w, "reviews/deg/01-candidate",
                         {"codex": {}, "claude": {"quota": "429 Too Many Requests"}})
        result, gate = w.gate(["reviews/deg/01-candidate"], out="reviews/deg/gate.json")
        require(self, gate.get("state") == "degraded" and result.returncode == 3, v,
                f"degraded: {gate.get('state')} exit {result.returncode}")
        # A broken fixes chain is blocked.
        other = w.fix_commit(content="def greet(name):\n    return 'hi'\n")
        self.three_round(w, "reviews/chain/01-candidate", {"codex": {}, "claude": {}}, head=fix)
        shutil.copytree(w.repo / "reviews/chain/01-candidate", w.repo / "reviews/chain/02-fixes")
        meta = json.loads((w.repo / "reviews/chain/02-fixes/round.json").read_text())
        meta.update(stage="fixes", base=w.base, head=other, sequence=2)
        (w.repo / "reviews/chain/02-fixes/round.json").write_text(json.dumps(meta))
        result, gate = w.gate(["reviews/chain/01-candidate", "reviews/chain/02-fixes"], out="reviews/chain/gate.json",
                              expect=other)
        require(self, gate.get("state") == "blocked" and result.returncode == 4, v,
                f"broken chain: {gate.get('state')}")
        # The criteria digest is present and follows the criteria documents.
        for m in metas:
            require(self, re.fullmatch(r"[0-9a-f]{64}", str(m.get("criteria_digest") or "")), v,
                    f"round record lacks a criteria digest: {m.get('criteria_digest')!r}")
            require(self, [e.get("sha256") for e in m.get("criteria") or []] == [sha(w.criteria.read_bytes())], v,
                    f"criteria entries do not name the criteria document: {m.get('criteria')}")
        w.criteria.write_text(w.criteria.read_text() + "- **AC-3** a later criterion.\n")
        self.three_round(w, "reviews/crit/01-candidate", {"codex": {}, "claude": {}})
        changed = w.round_json("reviews/crit/01-candidate").get("criteria_digest")
        require(self, changed and changed != metas[0].get("criteria_digest"), v,
                "the criteria digest does not change with the criteria")
        # More code rounds than the cap allows are blocked once the extension is gone.
        heads = [w.base]
        for i in range(4):
            heads.append(w.fix_commit(content=f"def greet(name):\n    return 'over {i}'\n"))
        rounds = []
        for i, stage in enumerate(("candidate", "fixes", "fixes", "fixes")):
            if i == 3:
                w.spar_run("extend", "reviews/over", "--stage", "code", "--by", "1", "--user-quote", QUOTE)
            out = f"reviews/over/0{i + 1}-{stage}"
            self.three_round(w, out, {"codex": {}, "claude": {}}, base=heads[0] if i == 0 else heads[i],
                             head=heads[i + 1], stage=stage)
            rounds.append(out)
        result, gate = w.gate(rounds, out="reviews/over/gate.json", expect=heads[4])
        require(self, gate.get("state") == "complete", v, f"four code rounds with an extension: {gate.get('state')} "
                f"{gate.get('reasons')}")
        extensions = w.repo / "reviews/over/extensions.jsonl"
        require(self, extensions.is_file(), v, "the extension record is missing")
        extensions.unlink()
        result, gate = w.gate(rounds, out="reviews/over/gate.json", expect=heads[4])
        require(self, gate.get("state") == "blocked" and result.returncode == 4, v,
                f"code rounds past the cap: {gate.get('state')} exit {result.returncode}")

    def test_gate_records_review_fix_growth(self):
        v = "growth-not-recorded"
        w = World(self, v)
        w.user_config(reviewers_toml({"name": "codex", "preset": "codex"}))
        w.project_config('check_paths = ["checks/**"]\n')
        w.control({"codex": {}})
        w.round("reviews/demo/01-candidate")
        w.write("checks/test_app.py", "def test_greet():\n" + "".join(f"    assert {i} == {i}\n" for i in range(9)))
        w.write("app/main.py", "def greet(name):\n    return 'hello, ' + name\n\n\ndef bye():\n    return 1\n")
        fix = w.commit("review fix " + COMMIT_TOKEN)
        w.reset_log()
        w.control({"codex": {}}, head=fix)
        w.round("reviews/demo/02-fixes", stage="fixes", base=w.head, head=fix)
        result, gate = w.gate(["reviews/demo/01-candidate", "reviews/demo/02-fixes"], expect=fix)
        growth = (gate or {}).get("growth") or {}
        added = growth.get("added_lines") or {}
        numstat = w.g("diff", "--numstat", w.head, fix).splitlines()
        expected = {"check": 0, "other": 0}
        for line in numstat:
            add, _, path = line.split("\t")
            expected["check" if path.startswith("checks/") else "other"] += int(add)
        require(self, added == expected, v, f"added lines {added}, expected {expected}")
        require(self, growth.get("over_hardening") is True, v, f"over-hardening mark: {growth}")
        w.write("app/main.py", "def greet(name):\n" + "".join(f"    x{i} = {i}\n" for i in range(20))
                + "    return 'hello, ' + name\n")
        fix2 = w.commit("review fix 2 " + COMMIT_TOKEN)
        w.reset_log()
        w.control({"codex": {}}, head=fix2)
        w.round("reviews/demo/03-fixes", stage="fixes", base=fix, head=fix2)
        result, gate = w.gate(["reviews/demo/01-candidate", "reviews/demo/02-fixes", "reviews/demo/03-fixes"],
                              expect=fix2, out="reviews/gate2.json")
        growth = (gate or {}).get("growth") or {}
        require(self, growth.get("over_hardening") is False, v, f"over-hardening with more product lines: {growth}")


# ---------------------------------------------------------------------------
# AC-16, AC-17: preserved behavior
# ---------------------------------------------------------------------------

class Boundary(unittest.TestCase):
    def test_ha_has_no_reviewer_code(self):
        v = "core-imports-reviewers"
        env = dict(os.environ, GOFLAGS="-mod=readonly", GOTOOLCHAIN="local")
        result = run([tool("go"), "list", "-deps", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .GoFiles \",\"}}",
                      "./cmd/ha"], ROOT, env)
        if result.returncode:
            raise Unavailable(f"go list: {text(result)[-1000:]}")
        module = "github.com/jgoneit/jaekit/"
        own = []
        for line in result.stdout.decode().splitlines():
            path, directory, files = line.split("\t")
            require(self, "internal/spar" not in path and "cmd/spar" not in path and "go-toml" not in path, v,
                    f"ha depends on {path}")
            if path.startswith(module):
                own.append((path, Path(directory), [f for f in files.split(",") if f]))
        require(self, own, v, "no in-module packages listed for ha")
        literals = re.compile(r'"(?:claude|codex|agy)"')
        for path, directory, files in own:
            for name in files:
                body = (directory / name).read_text(encoding="utf-8")
                require(self, not literals.search(body), v, f"{path}/{name} names a reviewer CLI")


SPEC_SEAL = {"spec": "0.1.12", "seal": "0.1.17"}


class Preserved(unittest.TestCase):
    def test_spec_seal_and_clean_review_unchanged(self):
        v = "products-changed"
        market = json.loads((ROOT / ".claude-plugin/marketplace.json").read_text())
        entries = {p["name"]: p for p in market.get("plugins", [])}
        codex = json.loads((ROOT / ".agents/plugins/marketplace.json").read_text())
        codex_entries = {p["name"]: p for p in codex.get("plugins", [])}
        for name, version in SPEC_SEAL.items():
            require(self, entries.get(name, {}).get("version") == version
                    and entries[name].get("source") == f"./plugins/{name}", v, f"{name} marketplace entry changed")
            require(self, codex_entries.get(name, {}).get("source", {}).get("path") == f"./plugins/{name}", v,
                    f"{name} Codex marketplace entry changed")
            for host in (".claude-plugin", ".codex-plugin"):
                manifest = json.loads((ROOT / "plugins" / name / host / "plugin.json").read_text())
                require(self, manifest.get("name") == name and manifest.get("version") == version, v,
                        f"{name} {host} manifest changed")
        digests = json.loads((Path(__file__).with_name("preserved.json")).read_text())
        for rel, digest in digests["files"].items():
            path = ROOT / rel
            require(self, path.is_file() and sha(path.read_bytes()) == digest, v, f"{rel} changed")
        listed = {str(p.relative_to(ROOT)) for d in ("plugins/spec", "plugins/seal")
                  for p in (ROOT / d).rglob("*") if p.is_file()}
        require(self, listed == set(digests["files"]), v, f"Spec/Seal files added or removed: "
                f"{sorted(listed ^ set(digests['files']))[:5]}")

    def test_spar_leaves_clean_review_and_records_alone(self):
        v = "products-changed"
        w = World(self, v)
        skill = w.home / ".claude/skills/clean-review"
        (skill / "scripts").mkdir(parents=True)
        (skill / "SKILL.md").write_text("synthetic clean-review skill\n")
        (skill / "scripts/clean_review.py").write_text("print('synthetic')\n")
        clean = w.xdg / "clean-review/config.json"
        clean.parent.mkdir(parents=True)
        clean.write_text(json.dumps({"reviewers": ["codex"]}))
        old_round = w.repo / "docs/reviews/demo/01-impl"
        old_round.mkdir(parents=True)
        (old_round / "round.json").write_text('{"schema": "clean-review/round/v1"}\n')
        (old_round / "ledger.jsonl").write_text('{"action": "disposition"}\n')
        runs = w.repo / "docs/specs/demo/runs.jsonl"
        runs.write_text('{"schema": "run/v1", "seq": 1}\n')

        def snapshot():
            roots = (skill, clean.parent, old_round, runs.parent)
            files = {}
            for base in roots:
                for p in sorted(base.rglob("*")):
                    if p.is_file():
                        files[str(p)] = (sha(p.read_bytes()), stat.S_IMODE(p.stat().st_mode))
            return files
        before = snapshot()
        w.control({"codex": {"findings": [finding("F1", link_kind="criterion", link="AC-1")]}})
        w.round("docs/reviews/demo/spar/01-candidate")
        w.spar_run("triage", "docs/reviews/demo/spar/01-candidate", "codex-F1", "reject", "--evidence", "x")
        w.gate(["docs/reviews/demo/spar/01-candidate"], out="docs/reviews/demo/spar/gate.json")
        require(self, snapshot() == before, v, "spar changed clean-review files or existing records")
        meta = w.round_json("docs/reviews/demo/spar/01-candidate")
        require(self, "clean-review" in json.dumps((meta.get("config") or {}).get("source")), v,
                "the clean-review configuration was not read")


# ---------------------------------------------------------------------------
# AC-18, AC-19, AC-23: plugin, contracts and guides
# ---------------------------------------------------------------------------

def section(body, heading):
    lines = body.splitlines()
    for i, line in enumerate(lines):
        if line.strip() == heading:
            level = len(heading) - len(heading.lstrip("#"))
            out = []
            for later in lines[i + 1:]:
                if later.startswith("#") and len(later) - len(later.lstrip("#")) <= level:
                    break
                out.append(later)
            return "\n".join(out)
    return None


def read_doc(case, rel, violation):
    path = ROOT / rel
    require(case, path.is_file(), violation, f"{rel} is missing")
    return path.read_text(encoding="utf-8")


SKILL_TOKENS = [
    "`spar run --stage plan`", "`--stage plan-delta`", "`--stage candidate`", "`--stage fixes`",
    "`defect`", "`requirement-question`", "`hardening`", "`note`",
    "`fix`", "`reject`", "`duplicate`", "`answer`", "`--user-quote`", "`--failed-before`", "`--passed-after`",
    "`growth`", "`over_hardening`", "not evidence of completion",
]


class Plugin(unittest.TestCase):
    def test_spar_plugin_is_listed_and_skill_carries_rules(self):
        v = "spar-plugin-missing"
        market = json.loads((ROOT / ".claude-plugin/marketplace.json").read_text())
        entry = next((p for p in market.get("plugins", []) if p.get("name") == "spar"), None)
        require(self, entry is not None and entry.get("source") == "./plugins/spar", v,
                "Claude Code marketplace has no spar entry")
        codex = json.loads((ROOT / ".agents/plugins/marketplace.json").read_text())
        centry = next((p for p in codex.get("plugins", []) if p.get("name") == "spar"), None)
        require(self, centry is not None and (centry.get("source") or {}).get("path") == "./plugins/spar", v,
                "Codex marketplace has no spar entry")
        manifests = []
        for host in (".claude-plugin", ".codex-plugin"):
            body = read_doc(self, f"plugins/spar/{host}/plugin.json", v)
            manifests.append(json.loads(body))
        claude, cdx = manifests
        require(self, claude.get("name") == cdx.get("name") == "spar" and claude.get("version")
                and claude.get("version") == cdx.get("version") == entry.get("version")
                and claude.get("description") and claude.get("description") == cdx.get("description")
                == entry.get("description"), v, "manifests disagree on name, version or description")
        skill = read_doc(self, "plugins/spar/skills/spar/SKILL.md", v)
        front, _, body = skill.partition("\n---\n")
        require(self, skill.startswith("---\n") and re.search(r"^name: spar$", front, re.M), v,
                "skill front matter does not name spar")
        require(self, re.search(r"^\s+version: \"?" + re.escape(claude.get("version", "")) + r"\"?$", front, re.M), v,
                "skill version differs from the manifests")
        require(self, not re.search(r"[가-힣]", body), v, "skill body is not English")
        for token in SKILL_TOKENS:
            require(self, token in body, v, f"skill body lacks {token}")
        lines = [l for l in body.splitlines() if "same premise" in l and "two rounds" in l]
        require(self, lines and any("test the premise" in l for l in lines), v, "skill lacks the premise rule")


class Docs(unittest.TestCase):
    def test_contracts_products_and_usage_guides(self):
        v = "spar-contracts-missing"
        index = read_doc(self, "contracts/README.md", v)
        for rel, token in (("contracts/spar-config.md", "spar.toml"), ("contracts/review-round.md", "review-round/v1"),
                           ("contracts/review-gate.md", "review-gate/v1")):
            body = read_doc(self, rel, v)
            require(self, token in body, v, f"{rel} lacks {token}")
            require(self, f"]({Path(rel).name})" in index, v, f"contracts/README.md does not link {rel}")
        agents = read_doc(self, "AGENTS.md", v)
        products = section(agents, "## Products") or ""
        require(self, "Spar (`plugins/spar` and the `spar` binary)" in products
                and "Seal Core never imports Spar" in products, v, "AGENTS.md Products lacks Spar and its boundary")
        for rel, provider in (("guides/SPAR.md", "제공자"), ("guides/SPAR.en.md", "provider")):
            body = read_doc(self, rel, v)
            for token in ("[[reviewers]]", 'preset = "command"', "example-reviewer", "{prompt}", "{workspace}",
                          "{schema}", "Anthropic", "OpenAI", "Google", provider):
                require(self, token in body, v, f"{rel} lacks {token}")

    def test_install_guides_and_readme_link(self):
        v = "spar-install-guide-missing"
        for rel, together in (("guides/INSTALL.md", "`ha`와 `spar`를 함께 설치"),
                              ("guides/INSTALL.en.md", "installs `ha` and `spar` together")):
            body = read_doc(self, rel, v)
            for token in (together, "brew install jgoneit/tap/jaekit", "codex plugin add spar@jaekit",
                          "claude plugin install spar@jaekit", "spar --version", "which -a spar"):
                require(self, token in body, v, f"{rel} lacks {token}")
        for rel, more, start, link in (("README.md", "## More", "## Get started", "guides/SPAR.en.md"),
                                       ("README.ko.md", "## 더 알아보기", "## 시작하기", "guides/SPAR.md")):
            body = read_doc(self, rel, v)
            more_text = section(body, more) or ""
            require(self, f"]({link})" in more_text, v, f"{rel} {more} does not link {link}")
            first = section(body, start)
            require(self, first is not None and "spar" not in first.lower(), v, f"{rel} first-use flow mentions Spar")
        for rel, heading in (("guides/INSTALL.md", "## Release 파일로 설치"),
                             ("guides/INSTALL.en.md", "## Install from a Release file")):
            block = install_block(self, rel, heading, v)
            for with_spar in (True, False):
                installed = run_install(self, block, with_spar, v)
                require(self, installed == (["ha", "spar"] if with_spar else ["ha"]), v,
                        f"{rel}: release install placed {installed} (archive with spar: {with_spar})")


def install_block(case, rel, heading, violation):
    body = read_doc(case, rel, violation)
    part = section(body, heading)
    require(case, part is not None, violation, f"{rel} lacks {heading}")
    blocks = re.findall(r"```bash\n(.*?)```", part, re.S)
    require(case, len(blocks) == 1, violation, f"{rel} {heading} must hold one shell block")
    return blocks[0].replace("$HOME", "$INSTALL_TEST_ROOT")


def run_install(case, block, with_spar, violation):
    match = re.search(r'name="ha_([0-9.]+)_\$\{os\}_\$\{arch\}"', block)
    require(case, match, violation, "install block does not name its archive")
    version = match.group(1)
    with tempfile.TemporaryDirectory(prefix="jaekit-spar-install-") as directory:
        root = Path(directory)
        name = f"ha_{version}_linux_amd64"
        package = root / name
        package.mkdir()
        for folder in ("tools", "examples", "guides", "contracts", "assets"):
            (package / folder).mkdir()
            (package / folder / "fixture.txt").write_text("synthetic\n")
        for doc in ("README.md", "README.en.md", "README.ko.md", "LICENSE"):
            (package / doc).write_text("synthetic\n")
        for binary in (["ha", "spar"] if with_spar else ["ha"]):
            (package / binary).write_text(f"#!/bin/sh\necho {binary} {version}\n")
            (package / binary).chmod(0o755)
        dist = root / "dist"
        dist.mkdir()
        archive = dist / (name + ".tar.gz")
        with tarfile.open(archive, "w:gz") as output:
            output.add(package, arcname=name)
        (dist / "checksums.txt").write_text(sha(archive.read_bytes()) + "  " + archive.name + "\n")
        commands = root / "commands"
        commands.mkdir()
        (commands / "uname").write_text('#!/bin/sh\nif [ "$1" = "-s" ]; then echo Linux; else echo x86_64; fi\n')
        (commands / "curl").write_text(f'#!/bin/sh\nfor last; do :; done\ncp "{dist}/${{last##*/}}" .\n')
        for c in ("uname", "curl"):
            (commands / c).chmod(0o755)
        install_root = root / "installation"
        install_root.mkdir()
        env = dict(os.environ, INSTALL_TEST_ROOT=str(install_root),
                   PATH=str(commands) + os.pathsep + os.environ.get("PATH", ""))
        result = run([tool("bash"), "-c", block], root, env, timeout=120)
        signalled(result, "release install")
        require(case, result.returncode == 0, violation,
                f"release install failed (archive with spar: {with_spar}): {text(result)[-600:]}")
        bin_dir = install_root / ".local/bin"
        installed = sorted(p.name for p in bin_dir.iterdir()) if bin_dir.is_dir() else []
        for name_ in installed:
            require(case, os.access(bin_dir / name_, os.X_OK), violation, f"{name_} is not executable")
        return installed


# ---------------------------------------------------------------------------
# AC-21, AC-22: builds and release files
# ---------------------------------------------------------------------------

TARGETS = ("linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64")


def platform_of(header):
    if header[:6] == b"\x7fELF\x02\x01":
        return "linux/" + {62: "amd64", 183: "arm64"}.get(struct.unpack_from("<H", header, 18)[0], "?")
    if header[:4] == b"\xcf\xfa\xed\xfe":
        return "darwin/" + {0x1000007: "amd64", 0x100000C: "arm64"}.get(struct.unpack_from("<I", header, 4)[0], "?")
    return "?"


RUNTIME_CLASSES = ("Round", "Angles", "Records", "Config", "Presets", "Command", "Native", "Fallback",
                   "Failures", "Classes", "Ledger", "Caps", "Gate")
LINUX_IMAGE = "python:3.14"


def platform_suite(out):
    """Child mode: run the runtime observations against SPAR_OBSERVE_BINARY and write their outcome."""
    global PLATFORM_CHILD
    PLATFORM_CHILD = True
    module = sys.modules[__name__]
    loader = unittest.TestLoader()
    suite = unittest.TestSuite(loader.loadTestsFromTestCase(getattr(module, name)) for name in RUNTIME_CLASSES)
    # AC-17's runtime part: spar leaves clean-review files and records alone.
    suite.addTest(Preserved("test_spar_leaves_clean_review_and_records_alone"))
    result = unittest.TextTestRunner(stream=sys.stderr, verbosity=2).run(suite)
    Path(out).write_text(json.dumps({
        "tests": result.testsRun, "skipped": len(result.skipped),
        "failures": [[case.id(), detail[-1500:]] for case, detail in result.failures],
        "errors": [[case.id(), detail[-1500:]] for case, detail in result.errors]}))
    return 0


class Platforms(unittest.TestCase):
    """AC-21: spar builds for the four targets and behaves the same on each.

    Each target runs the runtime observations (RUNTIME_CLASSES) against the
    binary built for it: darwin/arm64 natively, darwin/amd64 through Rosetta,
    linux/arm64 and linux/amd64 in the local python:3.14 image (never pulled
    here). A target that cannot execute on this host is an execution error.
    """

    def test_common_verification_builds_spar_for_four_targets(self):
        v = "spar-build-missing"
        require(self, SPAR_MAIN.is_file(), v, "the checked tree has no spar executable (cmd/spar)")
        spec = importlib.util.spec_from_file_location("spar_verify_view", ROOT / "tools/verify.py")
        module = importlib.util.module_from_spec(spec)
        try:
            spec.loader.exec_module(module)
        except Exception as error:
            raise Unavailable(f"tools/verify.py does not import: {error}") from None
        require(self, tuple(module.TARGETS) == TARGETS, v, f"verify targets: {module.TARGETS}")
        commands = []
        module.run = lambda label, command, env, *, formatting=False: commands.append((label, list(command), env))
        module.verify_go({})
        for target in TARGETS:
            goos, goarch = target.split("/")
            matching = [c for _, c, env in commands if c[:2] == ["go", "build"] and env.get("GOOS") == goos
                        and env.get("GOARCH") == goarch and "./cmd/spar" in c]
            require(self, matching, v, f"common verification does not build spar for {target}")

    def behaves(self, target):
        v = "spar-platform-behavior"
        require(self, SPAR_MAIN.is_file(), v, "the checked tree has no spar executable (cmd/spar)")
        goos, goarch = target.split("/")
        binary = build("./cmd/spar", f"spar-{goos}-{goarch}", {"GOOS": goos, "GOARCH": goarch})
        require(self, platform_of(binary.read_bytes()[:64]) == target, v, f"spar for {target} has another format")
        work = Path(tempfile.mkdtemp(prefix="jaekit-spar-platform-"))
        self.addCleanup(shutil.rmtree, work, True)
        observer = work / "tools/goal-checks/spar/observe_spar.py"
        observer.parent.mkdir(parents=True)
        shutil.copyfile(Path(__file__), observer)
        shutil.copyfile(binary, work / "spar")
        (work / "spar").chmod(0o755)
        if goos == "darwin":
            if os.uname().sysname != "Darwin":
                raise Unavailable(f"{target} needs a macOS host")
            # run() turns an executable this host cannot start (no Rosetta)
            # into Unavailable; how spar then behaves is the suite's to judge.
            run([work / "spar", "--version"], work)
            argv = [sys.executable, observer, "--platform-suite", work / "outcome.json"]
            env = {key: value for key, value in os.environ.items() if not key.startswith("HA_")}
            env["SPAR_OBSERVE_BINARY"] = str(work / "spar")
        else:
            docker = tool("docker")
            probe = run([docker, "run", "--rm", "--pull", "never", "--platform", target, LINUX_IMAGE,
                         "python3", "--version"], work, timeout=300)
            if probe.returncode:
                raise Unavailable(f"{LINUX_IMAGE} for {target} is not available here: {text(probe)[-300:]}")
            argv = [docker, "run", "--rm", "--pull", "never", "--platform", target, "-v", f"{work}:/work",
                    "-e", "SPAR_OBSERVE_BINARY=/work/spar", LINUX_IMAGE, "python3",
                    "/work/tools/goal-checks/spar/observe_spar.py", "--platform-suite", "/work/outcome.json"]
            env = None
        result = run(argv, work, env, timeout=840)
        try:
            outcome = json.loads((work / "outcome.json").read_text())
        except (OSError, ValueError):
            raise Unavailable(f"{target}: the runtime observations did not report: {text(result)[-1500:]}") from None
        if outcome["failures"]:
            # One subtest keeps the violation while a later execution error
            # still ends the test; the child's own tag is quoted without its
            # brackets so this message carries exactly one tag.
            case, detail = outcome["failures"][0]
            found = re.search(r"\[violation:[^\]]+\][^\n]*", detail)
            quoted = re.sub(r"\[violation:([^\]]+)\]", r"(\1)", found.group(0) if found else detail[-300:])
            with self.subTest(target=target):
                self.fail(f"[violation:{v}] {target}: {case} ({len(outcome['failures'])} failed): {quoted}")
        if outcome["errors"] or not outcome["tests"]:
            raise Unavailable(f"{target}: runtime observations could not run: {outcome['errors'][:1]}")

    def test_darwin_arm64(self):
        self.behaves("darwin/arm64")

    def test_darwin_amd64(self):
        self.behaves("darwin/amd64")

    def test_linux_arm64(self):
        self.behaves("linux/arm64")

    def test_linux_amd64(self):
        self.behaves("linux/amd64")


def step_script(workflow, name):
    lines = workflow.splitlines()
    for i, line in enumerate(lines):
        if line.strip() == f"- name: {name}":
            indent = len(line) - len(line.lstrip())
            for j in range(i + 1, len(lines)):
                stripped = lines[j].strip()
                if stripped.startswith("- name:") and len(lines[j]) - len(lines[j].lstrip()) == indent:
                    break
                if stripped == "run: |":
                    body_indent = None
                    out = []
                    for later in lines[j + 1:]:
                        if not later.strip():
                            out.append("")
                            continue
                        current = len(later) - len(later.lstrip())
                        if body_indent is None:
                            body_indent = current
                        if current < body_indent:
                            break
                        out.append(later[body_indent:])
                    return "\n".join(out) + "\n"
    return None


class Release(unittest.TestCase):
    def test_release_archives_carry_spar_next_to_ha(self):
        v = "release-spar-missing"
        workflow = (ROOT / ".github/workflows/release.yml").read_text()
        build_script = step_script(workflow, "Build release files")
        verify_script = step_script(workflow, "Verify archives and native smoke")
        if build_script is None or verify_script is None:
            raise Unavailable("release workflow steps were not found")
        require(self, "./cmd/spar" in build_script, v, "the release build does not build spar")
        version = "9.8.7"
        temp = Path(tempfile.mkdtemp(prefix="jaekit-spar-release-"))
        self.addCleanup(shutil.rmtree, temp, True)
        shims = temp / "shims"
        shims.mkdir()
        if not shutil.which("sha256sum"):
            (shims / "sha256sum").write_text(
                f"#!{sys.executable}\nimport hashlib, sys\nfor n in sys.argv[1:]:\n"
                "    if n == '--': continue\n"
                "    print(hashlib.sha256(open(n,'rb').read()).hexdigest() + '  ' + n)\n")
            (shims / "sha256sum").chmod(0o755)
        env = dict(os.environ, VERSION=version, RUNNER_TEMP=str(temp), GOFLAGS="-mod=readonly", GOTOOLCHAIN="local",
                   PATH=str(shims) + os.pathsep + os.environ.get("PATH", ""))
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        result = run([tool("bash"), "-c", "set -euo pipefail\n" + build_script], ROOT, env, timeout=1500)
        if result.returncode:
            raise Unavailable(f"release build step failed: {text(result)[-2000:]}")
        dist = temp / "dist"
        result = run([tool("bash"), "-c", "set -euo pipefail\n" + verify_script], ROOT, env, timeout=900)
        signalled(result, "release verification")
        require(self, result.returncode == 0, v, f"release verification failed: {text(result)[-1200:]}")
        expected_names = {f"ha_{version}_{t.replace('/', '_')}.tar.gz" for t in TARGETS} | \
            {f"ha_{version}_windows_amd64.zip", "checksums.txt"}
        require(self, {p.name for p in dist.iterdir()} == expected_names, v,
                f"release file names: {sorted(p.name for p in dist.iterdir())}")
        # The workflow copies these folders whole (`cp -R`), so the expected
        # contents are the files the checked tree holds there.
        copied = {str(p.relative_to(ROOT)) for d in ("guides", "contracts", "assets")
                  for p in (ROOT / d).rglob("*") if p.is_file()}
        public = copied | {"LICENSE", "README.md", "README.ko.md", "tools/check-result-reference.py",
                           "tools/wsl/check-environment.sh"} | \
            {f"examples/check-result/{n}" for n in ("README.md", "declaration.json", "reference.json", "input.txt")}
        native = None
        for target in TARGETS:
            prefix = f"ha_{version}_{target.replace('/', '_')}"
            with tarfile.open(dist / (prefix + ".tar.gz")) as archive:
                members = {m.name: m for m in archive.getmembers()}
                require(self, all(n == prefix or n.startswith(prefix + "/") for n in members), v,
                        f"{prefix}: another top folder")
                files = {n[len(prefix) + 1:] for n, m in members.items() if m.isfile()}
                require(self, files == public | {"ha", "spar"}, v,
                        f"{prefix}: contents differ: {sorted(files ^ (public | {'ha', 'spar'}))[:6]}")
                spar_member = members[prefix + "/spar"]
                require(self, spar_member.mode & 0o111, v, f"{prefix}: spar is not executable")
                header = archive.extractfile(spar_member).read(64)
                require(self, platform_of(header) == target, v, f"{prefix}: spar is for another platform")
        with zipfile.ZipFile(dist / f"ha_{version}_windows_amd64.zip") as archive:
            require(self, not any(Path(n).name.lower() in ("spar", "spar.exe") for n in archive.namelist()), v,
                    "the Windows candidate archive carries spar")
        machine = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(
            os.uname().machine.lower())
        system = {"Darwin": "darwin", "Linux": "linux"}.get(os.uname().sysname)
        if machine and system:
            prefix = f"ha_{version}_{system}_{machine}"
            unpack = temp / "unpack"
            with tarfile.open(dist / (prefix + ".tar.gz")) as archive:
                archive.extractall(unpack, filter="data")
            ha_version = run([unpack / prefix / "ha", "--version"], temp).stdout.decode().strip()
            spar_version = run([unpack / prefix / "spar", "--version"], temp).stdout.decode().strip()
            require(self, ha_version == f"ha {version}" and spar_version == f"spar {version}", v,
                    f"versions in one archive: {ha_version!r} and {spar_version!r}")
        else:
            raise Unavailable("the native platform has no release archive")
        # The release check refuses an archive without spar.
        prefix = f"ha_{version}_linux_amd64"
        source = dist / (prefix + ".tar.gz")
        rebuilt = io.BytesIO()
        with tarfile.open(source) as archive, tarfile.open(fileobj=rebuilt, mode="w:gz") as output:
            for member in archive.getmembers():
                if member.name != prefix + "/spar":
                    output.addfile(member, archive.extractfile(member) if member.isfile() else None)
        source.write_bytes(rebuilt.getvalue())
        sums = []
        for line in (dist / "checksums.txt").read_text().splitlines():
            digest, name = line.split(None, 1)
            name = name.lstrip("*")
            sums.append((sha((dist / name).read_bytes()) if name == source.name else digest) + "  " + name)
        (dist / "checksums.txt").write_text("\n".join(sums) + "\n")
        result = run([tool("bash"), "-c", "set -euo pipefail\n" + verify_script], ROOT, env, timeout=900)
        signalled(result, "release verification")
        require(self, result.returncode != 0 and "spar" in text(result), v,
                f"the release check accepted an archive without spar: {text(result)[-400:]}")


if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--platform-suite":
        raise SystemExit(platform_suite(sys.argv[2]))
    unittest.main()
