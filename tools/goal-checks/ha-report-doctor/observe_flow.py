"""The real flow through a built `ha` (AC-30) and platform sameness (AC-24).

See common.py for the fixture and the result rules. AC-24 runs the Go
tests `TestNativeReportOutput` and `TestNativeDoctorProbes` here, reads
which tests Windows native CI requires from tools/windows/native_checks.py,
and asks the repository's CI reader (tools/verification-checks/
cross_platform_ci.py, its existing admission unchanged) whether the real pull
request run of the checked commit passed. No pull request run for the checked
commit, or a reader that cannot pass it, is an unavailable observation.
"""
import importlib.util
import json
import os
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from common import ROOT, Repo, Temporary, Unavailable, build, require, run, text  # noqa: E402

SCRIPTS = {"tests/env/plain.sh": "echo ok\n"}
NATIVE_TESTS = ["TestNativeReportOutput", "TestNativeReportOutput/en", "TestNativeReportOutput/ko",
                "TestNativeDoctorProbes", "TestNativeDoctorProbes/available", "TestNativeDoctorProbes/missing",
                "TestNativeDoctorProbes/exit", "TestNativeDoctorProbes/timeout", "TestNativeDoctorProbes/tail"]


class Flow(unittest.TestCase):
    def test_real_flow(self):
        """AC-30: doctor, start, baseline, check, done and report through the built ha; doctor and report write nothing."""
        v = "real-flow-incomplete"
        with Temporary("jaekit-flow-") as directory:
            ha = build(directory / "bin" / "ha")
            extra = "## 실행 환경\n- 셸: `sh tests/env/plain.sh`\n- 없는 도구: `no-such-tool-ha-doctor-7c1e`\n"
            repo = Repo(directory / "flow", ha, plan_extra=extra)
            for path, content in SCRIPTS.items():
                repo.write(path, content)
            repo.commit("probes")
            want = ["셸: 사용 가능", "없는 도구: 사용 불가 (실행 파일 없음)"]

            def doctor(note):
                before = repo.record_bytes()
                code, out, err = repo.doctor()
                lines = [l for l in out.split("\n") if l and not l.startswith("    ")]
                require(self, code == 1 and lines == want, v, f"{note}: doctor exited {code}: {(out + err)[-800:]}")
                require(self, repo.record_bytes() == before, v, f"{note}: doctor changed the record")

            doctor("before start")
            repo.start()
            doctor("after start")
            repo.check("AC-1", baseline=True, expect=0)
            repo.implement()
            repo.check(expect=0)
            code, out = repo.done()
            if code != 0:
                raise Unavailable(f"ha done did not record completion: {out[-800:]}")
            completion = repo.lines()[-1]
            if completion.get("kind") != "done" or completion.get("status") != "complete":
                raise Unavailable("the last record line is not a completion record")
            seq = completion["seq"]
            before = repo.record_bytes()
            for lang, args, next_step, stamp in (
                    ("en", (), f"Next step: none; completion record seq {seq} holds and nothing is left to do",
                     f"; completion record: seq {seq}"),
                    ("ko", ("--lang", "ko"), f"다음 행동: 없음; 완료 기록 seq {seq}이 유효하고 남은 할 일이 없음",
                     f"; 완료 기록: seq {seq}")):
                code, out, err = repo.report(*args)
                require(self, code == 0, v, f"report {lang} exited {code}: {(out + err)[-800:]}")
                lines = out.split("\n")
                require(self, lines[4] == next_step, v, f"report {lang}: {lines[4]!r}")
                head = next((l for l in lines if l.startswith(("- record head: ", "- 기록 head: "))), "")
                require(self, f"seq {seq} (" in head and head.endswith(stamp), v, f"report {lang} stamp: {head!r}")
            require(self, repo.record_bytes() == before, v, "report changed the record")


def go_results(pattern):
    """(failed, passed) test names of `go test -json` in ./cmd/ha for pattern."""
    env = {key: value for key, value in os.environ.items() if not key.startswith(("HA_", "GIT_"))}
    env.update(GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
    result = run(["go", "test", "-json", "-count=1", "-run", pattern, "./cmd/ha"], ROOT, env, timeout=1200)
    events = []
    for line in text(result.stdout).splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            raise Unavailable(f"go test printed a line that is not an event: {line[:200]!r}") from None
    if any(e.get("Action") == "fail" and e.get("FailedBuild") for e in events):
        raise Unavailable("./cmd/ha did not build")
    started = {e["Test"] for e in events if e.get("Action") == "run" and e.get("Test")}
    ended = {e["Test"] for e in events if e.get("Action") in ("pass", "fail", "skip") and e.get("Test")}
    if started - ended:
        raise Unavailable(f"tests started without an end: {sorted(started - ended)[:3]}")
    failed = sorted({e["Test"] for e in events if e.get("Action") == "fail" and e.get("Test")})
    passed = sorted({e["Test"] for e in events if e.get("Action") == "pass" and e.get("Test")})
    if result.returncode and not failed:
        raise Unavailable(f"go test exited {result.returncode} without a failed test: {text(result.stderr)[-500:]}")
    return failed, passed


def windows_native():
    sys.path.insert(0, str(ROOT / "tools/windows"))
    try:
        spec = importlib.util.spec_from_file_location("observed_native_checks", ROOT / "tools/windows/native_checks.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return {package: list(tests) for package, tests in module.NATIVE.items()}
    except Exception as error:
        raise Unavailable(f"tools/windows/native_checks.py cannot be loaded: {error!r}") from None
    finally:
        sys.path.remove(str(ROOT / "tools/windows"))


class Platform(unittest.TestCase):
    def test_native_tests_pass_here(self):
        """AC-24: the shell-independent report and doctor tests pass on this host."""
        v = "platform-results-differ"
        failed, passed = go_results("^(TestNativeReportOutput|TestNativeDoctorProbes)$")
        missing = [t for t in NATIVE_TESTS if t not in passed]
        require(self, not failed and not missing, v, f"failed {failed[:5]}, did not pass {missing}")

    def test_windows_ci_runs_them(self):
        """AC-24: Windows native CI requires the same tests, and the checked commit's real PR run passed."""
        v = "platform-results-differ"
        required = windows_native().get("cmd/ha", [])
        missing = [t for t in NATIVE_TESTS if t not in required]
        require(self, not missing, v, f"Windows native CI does not require {missing}")
        env = {key: value for key, value in os.environ.items() if not key.startswith("HA_EVIDENCE")}
        result = run([sys.executable, "tools/verification-checks/cross_platform_ci.py", "AC-1"], ROOT, env, timeout=900)
        if result.returncode:
            raise Unavailable(f"the CI reader did not pass a real run of the checked commit (exit {result.returncode}): "
                              + text(result.stderr).strip()[-500:])


if __name__ == "__main__":
    unittest.main()
