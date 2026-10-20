"""Existing commands and the protected regression routes stay as they are (AC-23, AC-33).

AC-23 runs one fixed flow on a synthetic goal with a numeric budget and a
skill without a declaration, using `ha` built from the checked tree, and
compares each command's exit code and normalized output, and every
runs.jsonl line, with kept-outputs.json. That file holds the same flow's
result from `ha` built at the commit this goal started from; it was written
once by `python3 observe_kept.py --write-kept <ha>` before the implementation
and is the content this condition must keep. The synthetic repository's
commits use fixed dates, so commit and tree ids are the same on every run and
are compared as they are. Normalization replaces only what differs between two
runs of the same build: times, record chain hashes (each becomes
<RECORD-seq>, so which record a value names is still compared), the fresh
evidence `invocation` id, temporary paths, and numbers measured by the clock
(`duration_ms`, `span_seconds`, `limit_elapsed_ms`, `limit_use_permille`,
`user_ms`, `system_ms` and `total_duration_ms`; a status budget's
`elapsed_seconds` and the same numbers in the Markdown budget lines). Every other value, including digests,
seqs, limits, `limit_ms`, `limit_reached`, CPU status, scope and source,
counts, exit codes and versions, is compared as it is. AC-23
also runs the existing Go tests of ./cmd/ha and ./internal/..., which include
the behavior scenarios.

AC-33 compares every route and invariant of contracts/regressions.json
before this goal (the copy kept next to this file) with the current registry,
runs tools/regression_map.py against that copy, and runs every check of every
protected route. Routes may be added; none may change or disappear. No
history is read.
"""
import hashlib
import json
from pathlib import Path
import re
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from budget_common import GOAL, HERE, ROOT, Repo, Unavailable, build, go_env, require, run, temporary

KEPT = HERE / "kept-outputs.json"
HEX = re.compile(r"\b[0-9a-f]{7,64}\b")
TIME = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})")
# Elapsed seconds in the Markdown budget lines: "N/14400 seconds" and "window span N seconds".
ELAPSED = re.compile(r"\b\d+(?=/\d+ seconds)|(?<=window span )\d+(?= seconds)")
# Numbers measured by the clock, wherever they appear. A status budget's
# elapsed_seconds is measured too; a start record's budget elapsed_seconds is
# the limit and is compared.
MEASURED = {"duration_ms", "span_seconds", "limit_elapsed_ms", "limit_use_permille", "user_ms", "system_ms",
            "total_duration_ms"}
# Identifiers drawn fresh for every run.
FRESH = {"invocation"}
# Fixed commit dates make the synthetic repository's commit ids repeatable.
DATES = {"GIT_AUTHOR_DATE": "2026-01-01T00:00:00+0000", "GIT_COMMITTER_DATE": "2026-01-01T00:00:00+0000"}


def measured(key, value, siblings):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return False
    return key in MEASURED or (key == "elapsed_seconds" and "elapsed_limit_seconds" in siblings)


class Normal:
    """Replaces what differs between two runs of the same build, and nothing else."""

    def __init__(self, temp, raw_lines):
        self.temp = str(temp)
        # A record's chain hash is sha256 of its line; it depends on the line's time.
        self.records = {hashlib.sha256(line).hexdigest(): f"<RECORD-{seq}>"
                        for seq, line in enumerate(raw_lines, start=1)}

    def hex(self, match):
        token = match.group(0)
        for full, name in self.records.items():
            if full.startswith(token):
                return name if len(token) == len(full) else f"{name[:-1]}:{len(token)}>"
        return token

    def text(self, text):
        text = text.replace(self.temp, "<TMP>")
        text = TIME.sub("<TIME>", text)
        text = HEX.sub(self.hex, text)
        return ELAPSED.sub("<SECONDS>", text)

    def shape(self, value):
        if isinstance(value, dict):
            return {k: "<MEASURED>" if measured(k, v, value) else "<FRESH>" if k in FRESH and v
                    else self.shape(v) for k, v in value.items()}
        if isinstance(value, list):
            return [self.shape(v) for v in value]
        if isinstance(value, str):
            return self.text(value)
        return value

    def output(self, text):
        try:
            return {"json": self.shape(json.loads(text))}
        except ValueError:
            return {"text": self.text(text)}


def flow(ha, directory):
    """The fixed command sequence: [{command, exit, stdout, stderr}] and the record shapes."""
    temp = Path(directory).resolve()
    repo = Repo(Path(directory) / "flow", ha, changes=1, maintains=1, manuals=1, runs="20", git_env=DATES)
    skill = repo.skill("plain")
    repo.write("tests/x.sh", "exit 0\n")
    repo.commit("check")
    raw = []

    def step(label, *args):
        code, out, err = repo.ha(*args)
        raw.append((label, code, out, err))
        return code, out

    step("lint", "lint", GOAL)
    step("estimate", "estimate", GOAL)
    step("estimate json", "estimate", GOAL, "--format", "json")
    step("start", "start", GOAL, "--request", "숫자 예산 목표를 구현해줘", "--rules", "run-rules/4", "--skill", str(skill))
    step("baseline", "check", GOAL, "--baseline")
    step("check", "check", GOAL)
    step("status", "status", GOAL)
    step("status json", "status", GOAL, "--format", "json")
    step("budget", "budget", GOAL, "--runs", "30", "--from", "20", "--window", "1", "--revision", "1",
         "--quote", "상한을 30회로 올려줘", "--context", "합성 목표의 현재 창 총상한 20회를 30회로")
    step("note input", "note", GOAL, "input", "--quote", "계속해줘")
    step("note confirm", "note", GOAL, "confirm", "AC-3", "--quote", "AC-3을 확인했어")
    step("log", "log", GOAL, "3")
    step("timing", "timing", GOAL, "--format", "json")
    step("finding", "finding", GOAL)
    step("history", "history", GOAL)
    step("done", "done", GOAL)
    step("status after done", "status", GOAL, "--format", "json")
    try:
        lines = repo.records_path().read_bytes().removesuffix(b"\n").split(b"\n")
    except OSError as error:
        raise Unavailable(f"runs.jsonl: {error}") from None
    normal = Normal(temp, lines)
    steps = [{"command": label, "exit": code, "stdout": normal.output(out), "stderr": normal.output(err)}
             for label, code, out, err in raw]
    records = [normal.shape(r) for r in repo.records()]
    return {"steps": steps, "records": records}


def kept():
    try:
        return json.loads(KEPT.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise Unavailable(f"{KEPT.name}: {error}") from None


def first_difference(got, want, where="$"):
    if type(got) is not type(want):
        return f"{where}: {json.dumps(got, ensure_ascii=False)[:300]} != {json.dumps(want, ensure_ascii=False)[:300]}"
    if isinstance(got, dict):
        if list(got) != list(want):
            return f"{where}: keys {list(got)} != {list(want)}"
        for key in got:
            found = first_difference(got[key], want[key], f"{where}.{key}")
            if found:
                return found
        return None
    if isinstance(got, list):
        if len(got) != len(want):
            return f"{where}: {len(got)} items != {len(want)}"
        for index, (a, b) in enumerate(zip(got, want)):
            found = first_difference(a, b, f"{where}[{index}]")
            if found:
                return found
        return None
    return None if got == want else f"{where}: {str(got)[:300]!r} != {str(want)[:300]!r}"


class Kept(unittest.TestCase):
    """AC-23: existing commands keep their output, exit codes and record lines for existing inputs."""

    def test_outputs_and_suites(self):
        want = kept()
        problems = []
        with temporary("jaekit-kept-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            got = flow(ha, directory)
        with temporary("jaekit-kept-go-") as directory:
            result = run(["go", "test", "-count=1", "./cmd/ha", "./internal/..."], ROOT, go_env(directory), timeout=1700)
        for index, (a, b) in enumerate(zip(got["steps"], want["steps"])):
            found = first_difference(a, b, f"step {b.get('command')}")
            if found:
                problems.append(found)
        if len(got["steps"]) != len(want["steps"]):
            problems.append(f"{len(got['steps'])} steps != {len(want['steps'])}")
        found = first_difference(got["records"], want["records"], "records")
        if found:
            problems.append(found)
        text = (result.stdout + result.stderr).decode(errors="replace")
        if result.returncode:
            if "--- FAIL" not in text:
                raise Unavailable(f"go test exit {result.returncode} without a failed test: {text[-1500:]}")
            problems.append("existing Go tests failed: " + "\n".join(l for l in text.splitlines() if "FAIL" in l)[:1500])
        require(self, not problems, "existing-output-changed", "; ".join(problems))


def registry_before():
    try:
        return json.loads((HERE / "regressions-before.json").read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise Unavailable(f"regressions-before.json: {error}") from None


class Registry(unittest.TestCase):
    """AC-33: protected routes are neither demoted nor removed, keep sources and checks, and pass."""

    def test_protected_routes(self):
        with temporary("jaekit-routes-") as directory:
            self.routes(go_env(directory))

    def routes(self, env):
        problems = []
        try:
            registry = json.loads((ROOT / "contracts/regressions.json").read_text(encoding="utf-8"))
        except (OSError, ValueError) as error:
            raise Unavailable(f"contracts/regressions.json: {error}") from None
        before = registry_before()
        if registry.get("invariants") != before.get("invariants"):
            problems.append("the invariants changed")
        old = {r["id"]: r for r in before["routes"]}
        new = {r["id"]: r for r in registry.get("routes", [])}
        for rid, entry in old.items():
            if new.get(rid) != entry:
                problems.append(f"route {rid} changed or was removed")
        result = run([sys.executable, "tools/regression_map.py", "--previous", HERE / "regressions-before.json",
                      "--format", "json"], ROOT, env, timeout=600)
        try:
            report = json.loads(result.stdout)
        except ValueError:
            raise Unavailable(f"regression_map.py: {result.stderr.decode(errors='replace')[-800:]}") from None
        comparison = report.get("comparison", {})
        if result.returncode or any(comparison.get(k) for k in ("lost_protection", "changed_invariants", "affected_routes")):
            problems.append(f"registry comparison: exit {result.returncode}, {comparison}")
        if not set(old) <= set(report.get("protected", [])):
            problems.append(f"routes no longer protected: {sorted(set(old) - set(report.get('protected', [])))}")
        require(self, not problems, "protected-route-changed", "; ".join(problems))
        go_runs, python_runs = {}, {}
        for route in registry["routes"]:
            for check in route.get("checks", []):
                if check["file"].endswith("_test.go"):
                    go_runs.setdefault("./" + str(Path(check["file"]).parent), set()).add(check["selector"])
                elif check["file"].endswith(".py"):
                    python_runs.setdefault(check["file"], set()).add(check["selector"])
                else:
                    raise Unavailable(f"unknown check file kind: {check['file']}")
        failures = []
        for package, names in sorted(go_runs.items()):
            pattern = "^(" + "|".join(sorted(names)) + ")$"
            result = run(["go", "test", "-count=1", "-run", pattern, package], ROOT, env, timeout=1200)
            text = (result.stdout + result.stderr).decode(errors="replace")
            if "no tests to run" in text:
                failures.append(f"{package}: {pattern} ran no test")
            elif result.returncode:
                if "--- FAIL" not in text:
                    raise Unavailable(f"go test {package} exit {result.returncode}: {text[-800:]}")
                failures.append(f"{package}: " + "\n".join(l for l in text.splitlines() if "FAIL" in l)[:600])
        for file, selectors in sorted(python_runs.items()):
            path = Path(file)
            for selector in sorted(selectors):
                result = run([sys.executable, "-m", "unittest", "discover", "-s", path.parent, "-p", path.name,
                              "-k", selector], ROOT, env, timeout=1200)
                tail = result.stderr.decode(errors="replace").strip().splitlines()[-1:] or [""]
                ran = re.search(r"^Ran (\d+) tests?", result.stderr.decode(errors="replace"), re.MULTILINE)
                if result.returncode == 0 and tail[0].startswith("OK") and ran and int(ran.group(1)) > 0:
                    continue
                if result.returncode == 0 or not tail[0].startswith("FAILED") or "errors=" in tail[0]:
                    raise Unavailable(f"{file} -k {selector}: exit {result.returncode}: {tail[0]!r}")
                failures.append(f"{file} -k {selector}: {tail[0]}")
        require(self, not failures, "protected-check-failed", "; ".join(failures))


def write_kept(ha):
    with temporary("jaekit-kept-write-") as directory:
        got = flow(Path(ha).resolve(), directory)
    KEPT.write_text(json.dumps(got, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")


if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--write-kept":
        write_kept(sys.argv[2])
    else:
        unittest.main()
