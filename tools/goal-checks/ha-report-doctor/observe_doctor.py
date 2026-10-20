"""`ha doctor` observations (AC-4, AC-5, AC-6).

See common.py for the fixture and the result rules. The probes are small
shell scripts under tests/env/ in the synthetic repository. `ha doctor`
itself not ending within the time its fixed 60-second limit allows is the
observed property of AC-4 and AC-5, so it is a violation there, not an
unavailable observation.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from common import (AVAILABLE, GOAL, UNAVAILABLE, Condition, Repo, Temporary, Unavailable, build, require,  # noqa: E402
                    snapshot_diff, text)

TOKEN = "DOCTOR-OUTPUT-TOKEN-91b3"
SCRIPTS = {
    "tests/env/ok.sh": 'test "$#" -eq 2 && test "$1" = "a b" && test "$2" = "c d" || exit 9\necho VISIBLE-OK-OUTPUT\n',
    "tests/env/plain.sh": "echo VISIBLE-OK-OUTPUT\n",
    "tests/env/noexec.sh": "echo never\n",
    "tests/env/exit3.sh": f'echo "failing {TOKEN}"\nexit 3\n',
    "tests/env/killed.sh": "kill -KILL $$\n",
    "tests/env/root.sh": "test -f tests/env/root-marker\n",
    "tests/env/root-marker": "here\n",
    "tests/env/slow.sh": "sleep 50\n",
    "tests/env/hang.sh": "sleep 1000\n",
    "tests/env/parent.sh": '( while :; do echo beat >> "$1"; sleep 0.2; done ) &\nsleep 1000\n',
    "tests/env/many.sh": f'i=1\nwhile [ $i -le 100 ]; do echo "line $i"; i=$((i+1)); done\necho "last {TOKEN}"\nexit 1\n',
    "tests/env/wide.sh": 'i=1\nwhile [ $i -le 20 ]; do printf "%s\\n" "$(printf "가%.0s" $(seq 1 100))"; i=$((i+1)); done\necho END-OF-WIDE\nexit 1\n',
    "tests/env/crlf.sh": "printf 'first\\r\\nsecond\\r\\n'\nexit 1\n",
    "tests/env/mark.sh": "echo made > marker-made\n",
}


def section(*items):
    return "## 실행 환경\n" + "".join(item + "\n" for item in items)


def tool_lines(out):
    return [l for l in out.split("\n") if l and not l.startswith("    ")]


def tail_after(out, name):
    """The indented lines under one tool's line."""
    lines, found, tail = out.split("\n"), False, []
    for line in lines:
        if found:
            if line.startswith("    "):
                tail.append(line[4:])
                continue
            break
        found = line.startswith(name + ": ")
    return tail


class Built(unittest.TestCase):
    def setUp(self):
        self.temporary = Temporary("jaekit-doctor-")
        self.dir = self.temporary.__enter__()
        self.addCleanup(self.temporary.__exit__, None, None, None)
        self.ha = build(self.dir / "bin" / "ha")

    def repo(self, name, extra="", **kwargs):
        repo = Repo(self.dir / name, self.ha, plan_extra=extra, **kwargs)
        for path, content in SCRIPTS.items():
            repo.write(path, content)
        os.chmod(repo.root / "tests/env/noexec.sh", 0o644)
        repo.commit("probes")
        return repo

    def doctor(self, repo, violation, expect_code, cwd=None, env=None, goal=GOAL, timeout=180):
        """Run `ha doctor`; not ending within timeout is a violation of the fixed limit."""
        started = time.monotonic()
        try:
            result = subprocess.run([str(self.ha), "doctor", goal], cwd=cwd or repo.root, env=env or repo.env(),
                                    capture_output=True, timeout=timeout, stdin=subprocess.DEVNULL)
        except subprocess.TimeoutExpired:
            self.fail(f"[violation:{violation}] ha doctor did not end within {timeout} seconds")
        except OSError as error:
            raise Unavailable(f"ha doctor: {error}") from None
        elapsed = time.monotonic() - started
        out, err = text(result.stdout), text(result.stderr)
        require(self, result.returncode == expect_code, violation,
                f"ha doctor exited {result.returncode}, want {expect_code}: {(out + err)[-1500:]}")
        return out, err, elapsed


class Doctor(Built):
    def test_probe_results(self):
        """AC-4: each tool gets one line in order; the five reasons; argv without a shell from the repository root."""
        v = "doctor-results-wrong"
        items = ("- 브라우저 확인: `sh tests/env/ok.sh \"a b\" 'c d'`",
                 "- 없는 도구: `no-such-tool-ha-doctor-7c1e`",
                 "- 없는 스크립트: `tests/env/missing.sh`",
                 "- 권한 없음: `tests/env/noexec.sh`",
                 "- 로컬 DB: `sh tests/env/exit3.sh`",
                 "- 컨테이너 실행: `sh tests/env/killed.sh`",
                 "* 루트 기준: `sh tests/env/root.sh`")
        repo = self.repo("results", section(*items))
        out, _, _ = self.doctor(repo, v, 1, cwd=repo.root / "src", goal="../" + GOAL)
        want = ["브라우저 확인: 사용 가능", "없는 도구: 사용 불가 (실행 파일 없음)", "없는 스크립트: 사용 불가 (실행 파일 없음)",
                "권한 없음: 사용 불가 (실행 권한 없음)", "로컬 DB: 사용 불가 (0이 아닌 종료, 종료 코드 3)",
                "컨테이너 실행: 사용 불가 (신호로 종료, 신호 9)", "루트 기준: 사용 가능"]
        require(self, tool_lines(out) == want, v, f"got {tool_lines(out)} want {want}")
        require(self, "VISIBLE-OK-OUTPUT" not in out, v, "the output of an available tool is shown")
        ok = self.repo("all-ok", section("- 셸: `sh tests/env/plain.sh`", "- 인자: `sh tests/env/ok.sh \"a b\" \"c d\"`"))
        out, _, _ = self.doctor(ok, v, 0)
        require(self, tool_lines(out) == ["셸: 사용 가능", "인자: 사용 가능"], v, f"all available: {tool_lines(out)}")
        none = self.repo("no-section")
        out, _, _ = self.doctor(none, v, 0)
        require(self, AVAILABLE not in out and UNAVAILABLE not in out and len(tool_lines(out)) == 1, v,
                f"no section: {out!r}")
        empty = self.repo("empty-section", section())
        out, _, _ = self.doctor(empty, v, 0)
        require(self, AVAILABLE not in out and UNAVAILABLE not in out and len(tool_lines(out)) == 1, v,
                f"empty section: {out!r}")

    def test_fixed_limit(self):
        """AC-4: every check has a fixed 60-second limit."""
        v = "doctor-limit-wrong"
        repo = self.repo("limit", section("- 느린 도구: `sh tests/env/slow.sh`", "- 멈춘 도구: `sh tests/env/hang.sh`")
                         + "\n## 시간 제한\n- AC-1: 2초\n")
        out, _, elapsed = self.doctor(repo, v, 1, timeout=180)
        want = ["느린 도구: 사용 가능", "멈춘 도구: 사용 불가 (시간 초과, 60초)"]
        require(self, tool_lines(out) == want, v, f"got {tool_lines(out)} want {want}")
        require(self, 108 <= elapsed < 135, v, f"a 50-second and a stuck check took {elapsed:.1f} seconds")


class Traces(Built):
    def test_leaves_nothing(self):
        """AC-5: no record line, run use, bundle change or ha/runs output; also before start and under any rules."""
        v = "doctor-leaves-traces"
        repo = self.repo("traces", section("- 셸: `sh tests/env/plain.sh`", "- 실패 도구: `sh tests/env/exit3.sh`"))
        want = ["셸: 사용 가능", "실패 도구: 사용 불가 (0이 아닌 종료, 종료 코드 3)"]

        def observe(note):
            temporary = self.dir / f"tmp-{len(list(self.dir.glob('tmp-*')))}"
            temporary.mkdir()
            before, record = repo.snapshot(), repo.record_bytes()
            out, _, _ = self.doctor(repo, v, 1, env=repo.env({"TMPDIR": str(temporary), "TMP": str(temporary),
                                                               "TEMP": str(temporary)}))
            require(self, tool_lines(out) == want, v, f"{note}: {tool_lines(out)}")
            require(self, TOKEN in out, v, f"{note}: the failing tool's output is not shown")
            after = repo.snapshot()
            require(self, repo.record_bytes() == record, v, f"{note}: the record changed")
            require(self, before == after, v, f"{note}: files changed: {snapshot_diff(before, after)}")
            require(self, not any(temporary.iterdir()), v, f"{note}: files left in the temporary directory")
            require(self, not repo.contains(TOKEN), v, f"{note}: the tool output is stored in {repo.contains(TOKEN)}")

        observe("before start")
        repo.start()
        repo.check("AC-1", baseline=True, expect=0)
        observe("after start")
        status = repo.status()
        for rules in ("run-rules/1", "run-rules/2", "run-rules/9"):
            repo.set_rules(rules)
            observe(rules)
        repo.set_rules("run-rules/4")
        data = repo.record_bytes().decode("utf-8").split("\n")
        data[0] = data[0].replace('"kind":"start"', '"kind":"start","edited":true', 1)
        repo.records_path().write_bytes("\n".join(data).encode("utf-8"))
        observe("broken record")
        if status["budget"]["runs"] != 1:
            raise Unavailable(f"the fixture used {status['budget']['runs']} runs")

    def test_timeout_stops_descendants(self):
        """AC-5: a check over its limit ends with its descendants, the way ha check ends them."""
        v = "doctor-timeout-leaks"
        heartbeat = self.dir / "heartbeat"
        repo = self.repo("descendants", section(f"- 멈춘 도구: `sh tests/env/parent.sh '{heartbeat}'`"))
        out, _, _ = self.doctor(repo, v, 1, timeout=180)
        require(self, tool_lines(out) == ["멈춘 도구: 사용 불가 (시간 초과, 60초)"], v, f"got {tool_lines(out)}")
        if not heartbeat.exists():
            raise Unavailable("the descendant never wrote its heartbeat")
        first = heartbeat.stat().st_size
        time.sleep(1.5)
        require(self, heartbeat.stat().st_size == first, v, "a descendant still writes after the timeout")

    def test_failure_tail(self):
        """AC-5: only a bounded end of an unavailable tool's output reaches the terminal."""
        v = "doctor-tail-wrong"
        repo = self.repo("tail", section("- 많은 줄: `sh tests/env/many.sh`", "- 넓은 줄: `sh tests/env/wide.sh`",
                                         "- 줄 끝: `sh tests/env/crlf.sh`", "- 셸: `sh tests/env/plain.sh`"))
        out, _, _ = self.doctor(repo, v, 1)
        many = tail_after(out, "많은 줄")
        require(self, many and many[-1] == f"last {TOKEN}" and len(many) == 20 and many[0] == "line 82", v,
                f"the tail of 101 lines is {many[:2]}…{many[-2:]} ({len(many)} lines)")
        wide = tail_after(out, "넓은 줄")
        joined = "\n".join(wide).encode("utf-8")
        require(self, wide and wide[-1] == "END-OF-WIDE" and len(joined) <= 2000 and len(joined) > 1500, v,
                f"the wide tail has {len(joined)} bytes in {len(wide)} lines")
        require(self, all(set(l) <= {"가"} for l in wide[:-1]), v, "the wide tail does not start at a character boundary")
        require(self, tail_after(out, "줄 끝") == ["first", "second"], v, f"CRLF tail: {tail_after(out, '줄 끝')}")
        require(self, "VISIBLE-OK-OUTPUT" not in out, v, "the output of an available tool is shown")
        require(self, not repo.contains(TOKEN), v, f"the tool output is stored in {repo.contains(TOKEN)}")


class Input(Built):
    BAD = (
        "- 콜론 없음 `sh tests/env/plain.sh`",
        "- 스팬 없음: sh tests/env/plain.sh",
        "- 스팬 둘: `sh tests/env/plain.sh` `sh tests/env/plain.sh`",
        "- 뒤 글: `sh tests/env/plain.sh` 설명",
        "- : `sh tests/env/plain.sh`",
        "- 이름`안`: `sh tests/env/plain.sh`",
        "- 파이프: `sh tests/env/plain.sh | cat`",
        "- 세미콜론: `sh tests/env/plain.sh; sh tests/env/plain.sh`",
        "- 변수: `echo $HOME`",
        "- 따옴표: `sh \"tests/env/plain.sh`",
        "- 빈 명령: ` `",
        "- 표식: `sh tests/env/plain.sh`",
        "이 절은 도구를 적는다.",
        "  이어지는 줄",
    )

    def test_input_errors(self):
        """AC-6: malformed items are reported with file and line; nothing runs; the exit is neither 0 nor 1."""
        v = "doctor-input-unreported"
        cases = [(bad, None) for bad in self.BAD] + [(None, "second")]
        for index, (bad, kind) in enumerate(cases):
            if kind == "second":
                extra = section("- 표식: `sh tests/env/mark.sh`") + "\n## 다른 절\n\n" + section("- 셸: `sh tests/env/plain.sh`")
            else:
                extra = section("- 표식: `sh tests/env/mark.sh`", bad)
            repo = self.repo(f"input-{index}", extra)
            plan = repo.read(f"{GOAL}/PLAN.md").split("\n")
            target = (bad if kind is None else "## 실행 환경")
            numbers = [n for n, l in enumerate(plan, 1) if l == target]
            line = numbers[-1]
            code, out, err = repo.doctor()
            require(self, code not in (0, 1), v, f"{bad or 'second section'}: exited {code}")
            require(self, f"{GOAL}/PLAN.md:{line}" in out + err, v,
                    f"{bad or 'second section'}: {GOAL}/PLAN.md:{line} is not reported: {(out + err)[-600:]!r}")
            require(self, not (repo.root / "marker-made").exists(), v, f"{bad or 'second section'}: a check ran")

    def test_section_ignored_elsewhere(self):
        """AC-6: with or without the section, whatever it holds, lint, start, check and status give the same results."""
        v = "section-changes-other-commands"
        variants = (
            section("- 셸: `sh tests/env/plain.sh`", "- 없는 도구: `no-such-tool-ha-doctor-7c1e`"),
            section("- 파이프: `sh a | b`", "- 파이프: `sh a | b`", "산문 줄", "- : ``"),
            section("| ID | 종류 | 검증 명령 | 검사 경로 | Task |", "| --- | --- | --- | --- | --- |",
                    "| AC-1 | maintain | `sh tests/env/plain.sh` | — | T009 |", "AC-1과 T001을 언급한다."),
            section("### 조건표", "| ID | 종류 | 검증 명령 | 검사 경로 | Task |", "| --- | --- | --- | --- | --- |",
                    "| AC-9 | change | `sh x` | x | T001 |"),
            section("```", "## 조건표", "| ID | 종류 | 검증 명령 | 검사 경로 | Task |", "```", "- 셸: `sh tests/env/plain.sh`"),
        )

        def flow(repo):
            seen = []
            for args in (("lint", GOAL), ("start", GOAL, "--request", "합성", "--skill-name", "seal", "--skill-version",
                                         "0.1.17", "--rules", "run-rules/4"),
                         ("check", GOAL, "--baseline"), None, ("check", GOAL), ("status", GOAL, "--format", "json")):
                if args is None:
                    repo.implement()
                    continue
                code, out, err = repo.ha_run(*args)
                if args[0] == "status":
                    try:
                        data = json.loads(out)
                    except ValueError:
                        raise Unavailable("ha status printed invalid JSON") from None
                    data.pop("record_head", None)
                    for key in ("elapsed_seconds", "span_seconds", "reported_wait_seconds"):
                        (data.get("budget") or {}).pop(key, None)
                    out = json.dumps(data, sort_keys=True)
                seen.append((args[0], code, out, err))
            return seen

        for index, extra in enumerate(variants):
            for place in ("first", "last"):
                plain = self.repo(f"plain-{index}-{place}")
                with_section = self.repo(f"section-{index}-{place}")
                plan = with_section.plan(extra="")
                if place == "first":
                    title, rest = plan.split("\n", 1)
                    plan = title + "\n\n" + extra + "\n" + rest
                else:
                    plan = plan + "\n" + extra
                with_section.write(f"{GOAL}/PLAN.md", plan)
                a, b = flow(plain), flow(with_section)
                differences = [(x[0], x[1], y[1]) for x, y in zip(a, b) if x != y]
                require(self, not differences, v, f"variant {index} ({place}): {differences} {[x for x, y in zip(a, b) if x != y][:1]}")


if __name__ == "__main__":
    unittest.main()
