"""A synthetic test runner: one fixed runner output per case.

Usage: python3 tests/runner.py CASE [PATH]

Go cases write `go test -json` events to standard output in the shape of the
named toolchain. JUnit cases write an XML file to PATH, or to the location in
HA_JUNIT_XML when PATH is absent. Process cases fail to start a child, end by
a signal or sleep. Every case exits with the status the real runner would
have. The observers copy this file into their synthetic repositories; it
never reads the observed repository.
"""
import glob
import json
import os
import signal
import subprocess
import sys
import time

PACKAGE = "example.com/demo/forms"
MARK = "jaekit:violation=greeting-missing"
SECRET = "JAEKIT-SECRET-RUNNER-MESSAGE"


def event(action, test=None, output=None, kind=None, package=PACKAGE, **extra):
    item = {"Time": "2026-01-01T00:00:00Z", "Action": action, "Package": package}
    if test:
        item["Test"] = test
    if output is not None:
        item["Output"] = output
    if kind:
        item["OutputType"] = kind
    item.update(extra)
    return json.dumps(item)


def go_stream(test, failure_line, typed=True, passed=False, extra_output=()):
    """Events of one test, typed as Go 1.27 does or untyped as Go 1.26 does."""
    frame = "frame" if typed else None
    lines = [event("start"), event("run", test), event("output", test, f"=== RUN   {test}\n", frame)]
    lines += [event("output", test, text) for text in extra_output]
    if passed:
        lines += [event("output", test, f"--- PASS: {test} (0.00s)\n", frame), event("pass", test, Elapsed=0),
                  event("output", None, "PASS\n", frame), event("pass", Elapsed=0.01)]
    else:
        lines += [event("output", test, f"    demo_test.go:9: {failure_line}\n", "error" if typed else None),
                  event("output", test, f"--- FAIL: {test} (0.00s)\n", frame), event("fail", test, Elapsed=0),
                  event("output", None, "FAIL\n", frame), event("fail", Elapsed=0.01)]
    return lines


def emit_go(lines, ending="\n", code=1):
    sys.stdout.buffer.write("".join(line + ending for line in lines).encode())
    sys.stdout.flush()
    return code


def suite(cases, root="testsuites"):
    body = "".join(cases)
    if root == "testsuite":
        return f'<?xml version="1.0" encoding="utf-8"?>\n<testsuite name="pytest" tests="1">{body}</testsuite>\n'
    return ('<?xml version="1.0" encoding="utf-8"?>\n<testsuites name="demo">\n'
            f'<testsuite name="demo" tests="1">\n{body}</testsuite>\n</testsuites>\n')


def case(name="test_greets", classname="tests.test_greet", inner=""):
    cls = "" if classname is None else f' classname="{classname}"'
    if not inner:
        return f'  <testcase{cls} name="{name}" time="0.01"/>\n'
    return f'  <testcase{cls} name="{name}" time="0.01">{inner}</testcase>\n'


PYTEST_TRACE = ("def test_greets():\n&gt;       assert greet() == &quot;hello&quot;, &quot;" + MARK +
                " wrong&quot;\nE       AssertionError: " + MARK + " wrong\n\ntests/test_greet.py:4: AssertionError")
JUNIT = {
    # J1: a testcase without children passes.
    "J1": (suite([case()]), 0),
    # J2: pytest `assert ..., "<marker>"`; the message attribute's first line carries the marker.
    "J2": (suite([case(inner='<failure message="AssertionError: ' + MARK + ' wrong&#10;assert &apos;hi&apos; == &apos;hello&apos;">'
                       + PYTEST_TRACE + "</failure>")], "testsuite"), 1),
    # J3: a message-less pytest assert; the traceback body shows another source line with the marker.
    "J3": (suite([case(inner='<failure message="assert &apos;hi&apos; == &apos;hello&apos;">def test_greets():\n'
                       '    note = &quot;' + MARK + '&quot;\n&gt;       assert greet() == &quot;hello&quot;\nE       assert &apos;hi&apos; == &apos;hello&apos;'
                       "</failure>")], "testsuite"), 1),
    # J4: Vitest `expect(value, "<marker>")`.
    "J4": (suite([case("greets with hello", "tests/greet.test.ts",
                       '<failure message="' + MARK + ': expected &apos;hi&apos; to be &apos;hello&apos; // Object.is equality" type="AssertionError">'
                       "AssertionError: " + MARK + ": expected 'hi' to be 'hello'\n ❯ tests/greet.test.ts:5:30</failure>")]), 1),
    # J5: no message attribute; the body's first line is `Error: <marker>` (jest-junit shape).
    "J5": (suite([case("greets with hello", "greet greets with hello",
                       "<failure><![CDATA[Error: " + MARK + "\n    at Object.<anonymous> (tests/greet.test.js:4:9)]]></failure>")]), 1),
    # J6: a fixture setup error is an error element.
    "J6": (suite([case(inner='<error message="failed on setup with &quot;fixture missing&quot;">E   fixture missing</error>')]), 1),
    # J7: skipped.
    "J7": (suite([case(inner='<skipped type="pytest.skip" message="not now"/>')]), 0),
    # J8: nothing collected.
    "J8": ('<?xml version="1.0" encoding="utf-8"?>\n<testsuites><testsuite name="pytest" tests="0"/></testsuites>\n', 5),
    # J9: the same testcase twice.
    "J9": (suite([case(), case(inner='<failure message="' + MARK + '">x</failure>')]), 1),
    # J10: the marker only in system-out of a failure.
    "J10": (suite([case(inner='<failure message="assert 1 == 2">x</failure><system-out>' + MARK + "</system-out>")]), 1),
    # J11 truncated, DTD and entity declarations.
    "J11-truncated": (suite([case()])[:60], 1),
    "J11-dtd": ('<?xml version="1.0"?>\n<!DOCTYPE testsuites [<!ENTITY m "' + MARK + '">]>\n<testsuites><testsuite>'
                + case(inner="<failure message=\"&m;\">x</failure>") + "</testsuite></testsuites>\n", 1),
    "junit-syntax": (suite([case()]).replace("</testsuite>", "</testsuit>"), 1),
    "junit-two-roots": (suite([case()]) + "<testsuites/>\n", 0),
    "junit-not-junit": ('<?xml version="1.0"?>\n<html><testcase classname="tests.test_greet" name="test_greets"/></html>\n', 0),
    "junit-latin1": ('<?xml version="1.0" encoding="ISO-8859-1"?>\n<testsuites><testsuite>' + case() + "</testsuite></testsuites>\n", 0),
    "junit-rerun": (suite([case(inner='<rerunFailure message="flaky">x</rerunFailure>')]), 0),
    "junit-flaky": (suite([case(inner='<flakyFailure message="flaky">x</flakyFailure>')]), 0),
    "junit-rerun-error": (suite([case(inner='<rerunError message="flaky">x</rerunError>')]), 0),
    "junit-flaky-error": (suite([case(inner='<flakyError message="flaky">x</flakyError>')]), 0),
    "junit-unknown-child": (suite([case(inner="<surprise/>")]), 0),
    "junit-skip-and-failure": (suite([case(inner='<skipped/><failure message="' + MARK + '">x</failure>')]), 1),
    "junit-error-exit0": (suite([case(inner='<error message="boom">x</error>')]), 0),
    "junit-error-and-failure": (suite([case(inner='<failure message="' + MARK + '">x</failure><error message="teardown">x</error>')]), 1),
    "junit-pass-exit1": (suite([case()]), 1),
    "junit-marked-exit0": (suite([case(inner='<failure message="' + MARK + '">x</failure>')]), 0),
    "junit-other-names": (suite([case("test_other")]), 1),
    "junit-no-classname-pass": (suite([case(classname=None)]), 0),
    "junit-empty-classname-pass": (suite([case(classname="")]), 0),
    "junit-other-id": (suite([case(inner='<failure message="jaekit:violation=other-id wrong">x</failure>')]), 1),
    "junit-mixed-ids": (suite([case(inner='<failure message="' + MARK + '">x</failure><failure message="jaekit:violation=other-id">y</failure>')]), 1),
    "junit-two-marked": (suite([case(inner='<failure message="' + MARK + ' a">x</failure><failure message="' + MARK + ' b">y</failure>')]), 1),
    "junit-one-of-two": (suite([case(inner='<failure message="' + MARK + '">x</failure><failure message="assert 1 == 2">y</failure>')]), 1),
    "junit-empty-message": (suite([case(inner='<failure message="">' + MARK + "</failure>")]), 1),
    "junit-blank-body": (suite([case(inner="<failure>\n   Error: " + MARK + "\n  at x</failure>")]), 1),
    "junit-long-id": (suite([case(inner='<failure message="jaekit:violation=' + "a" * 129 + '">x</failure>')]), 1),
    # Two testcases with the same name under different classnames, one passing and one marked.
    "junit-same-name": (suite([case("test_greets", "tests.test_a"),
                               case("test_greets", "tests.test_b", '<failure message="' + MARK + '">x</failure>')]), 1),
    "junit-same-name-flipped": (suite([case("test_greets", "tests.test_a", '<failure message="' + MARK + '">x</failure>'),
                                       case("test_greets", "tests.test_b")]), 1),
    # Messages that must never reach a record.
    "junit-secret": (suite([case(inner=f'<failure message="{MARK} {SECRET}-message">{SECRET}-body</failure>'
                                       f"<system-out>{SECRET}-out</system-out><system-err>{SECRET}-err</system-err>")]), 1),
}


def junit_path(argv):
    if len(argv) > 2:
        return argv[2]
    return os.environ.get("HA_JUNIT_XML", "")


def probe(argv):
    """AC-9: report what this run saw at its output location, then write a passing file."""
    path_arg = argv[2] if len(argv) > 2 else None
    env_path = os.environ.get("HA_JUNIT_XML")
    target = path_arg or env_path
    parent = os.path.dirname(target) if target else None
    seen = {"argv": argv[1:], "env": env_path, "cwd": os.getcwd(),
            "existed": bool(target) and os.path.lexists(target),
            "parent_entries": sorted(os.listdir(parent)) if parent and os.path.isdir(parent) else None}
    log = os.environ.get("JAEKIT_OBSERVE_LOG")
    if log:
        with open(log, "a", encoding="utf-8") as out:
            out.write(json.dumps(seen) + "\n")
    # A literal placeholder (a condition without an adapter) is not a location to write.
    if target and os.path.isabs(target) and not seen["existed"]:
        with open(target, "x", encoding="utf-8") as out:
            out.write(JUNIT["J1"][0])
    return 0


def lock_log_directory():
    """Make the directory of this run's pending log read-only, so storing the output fails."""
    common = subprocess.run(["git", "rev-parse", "--git-common-dir"], capture_output=True, text=True).stdout.strip()
    pending = glob.glob(os.path.join(os.path.abspath(common), "ha", "runs", "*", ".pending-*.log"))
    for path in pending:
        os.chmod(os.path.dirname(path), 0o500)
    print(json.dumps({"Action": "start", "Package": PACKAGE}))
    return 1 if pending else 70


def main(argv):
    name = argv[1]
    # A `-crlf` case writes the same JUnit file with CRLF line endings (J12).
    crlf = name.endswith("-crlf") and name[:-5] in JUNIT
    if name in JUNIT or crlf:
        text, code = JUNIT[name[:-5] if crlf else name]
        data = text.encode("latin-1" if name == "junit-latin1" else "utf-8")
        with open(junit_path(argv), "wb") as out:
            out.write(data.replace(b"\n", b"\r\n") if crlf else data)
        return code
    if name == "junit-none":
        return 1
    if name == "junit-dir":
        os.mkdir(junit_path(argv))
        return 0
    if name == "junit-symlink":
        target = junit_path(argv) + ".real"
        with open(target, "w", encoding="utf-8") as out:
            out.write(JUNIT["J1"][0])
        os.symlink(target, junit_path(argv))
        return 0
    if name == "junit-bad-utf8":
        with open(junit_path(argv), "wb") as out:
            out.write(JUNIT["J1"][0].encode().replace(b'name="test_greets"', b'name="test_greets\xff"'))
        return 0
    if name == "junit-oversize":
        filler = "<!-- " + "x" * 1000 + " -->\n"
        with open(junit_path(argv), "w", encoding="utf-8") as out:
            out.write('<?xml version="1.0" encoding="utf-8"?>\n<testsuites><testsuite>' + case())
            for _ in range(17 * 1024):
                out.write(filler)
            out.write("</testsuite></testsuites>\n")
        return 0
    if name == "junit-probe":
        return probe(argv)
    if name == "junit-greet":
        # Marked failure while greet/greet.go in the run's tree still says "hi", a pass once it says "hello".
        with open(os.path.join("greet", "greet.go"), encoding="utf-8") as source:
            greeting = "hello" if '"hello"' in source.read() else "hi"
        text, code = (JUNIT["J1"][0], 0) if greeting == "hello" else (JUNIT["junit-marked-exit0"][0], 1)
        with open(junit_path(argv), "w", encoding="utf-8") as out:
            out.write(text)
        return code
    if name == "junit-slow-pass":
        time.sleep(2)
        with open(junit_path(argv), "w", encoding="utf-8") as out:
            out.write(JUNIT["J1"][0])
        return 0
    if name == "junit-slow-marked":
        time.sleep(2)
        with open(junit_path(argv), "w", encoding="utf-8") as out:
            out.write(JUNIT["junit-marked-exit0"][0])
        return 1
    if name == "go-untyped-fail":
        return emit_go(go_stream("TestMarked", MARK + " broke", typed=False))
    if name == "go-untyped-pass":
        return emit_go(go_stream("TestMarked", "", typed=False, passed=True,
                                 extra_output=[f"    demo_test.go:8: {MARK} logged\n"]), code=0)
    if name == "go-typed-fail":
        return emit_go(go_stream("TestMarked", MARK + " broke"))
    if name == "go-typed-unmarked":
        return emit_go(go_stream("TestMarked", "plain failure"))
    if name == "go-typed-pass":
        return emit_go(go_stream("TestMarked", "", passed=True), code=0)
    if name == "go-crlf-fail":
        return emit_go(go_stream("TestMarked", MARK + " broke"), ending="\r\n")
    if name == "go-crlf-unmarked":
        return emit_go(go_stream("TestMarked", "plain failure"), ending="\r\n")
    if name == "go-crlf-pass":
        return emit_go(go_stream("TestMarked", "", passed=True), ending="\r\n", code=0)
    if name == "go-marked-exit0":
        return emit_go(go_stream("TestMarked", MARK + " broke"), code=0)
    if name == "go-garbage":
        lines = go_stream("TestMarked", MARK + " broke")
        return emit_go(lines[:2] + ["ok  \texample.com/demo/forms\t0.01s"] + lines[2:])
    if name == "go-truncated":
        lines = go_stream("TestMarked", MARK + " broke")
        return emit_go(lines[:-1] + [lines[-1][:20]], ending="")
    if name == "go-blank-line":
        lines = go_stream("TestMarked", "", passed=True)
        return emit_go(lines[:3] + [""] + lines[3:], code=0)
    if name == "go-bad-utf8":
        sys.stdout.buffer.write(("\n".join(go_stream("TestMarked", "", passed=True)[:2]) + "\n").encode())
        sys.stdout.buffer.write(b'{"Action":"output","Package":"' + PACKAGE.encode() + b'","Test":"TestMarked","Output":"\xff\\n"}\n')
        sys.stdout.buffer.write(("\n".join(go_stream("TestMarked", "", passed=True)[3:]) + "\n").encode())
        return 0
    if name == "go-stderr-only":
        sys.stderr.write("go: cannot find main module; " + MARK + "\n")
        return 1
    if name == "go-pass-noisy-stderr":
        sys.stderr.write("go: downloading example.com/x v1.0.0\n" + MARK + " on stderr\nnot json\n")
        return emit_go(go_stream("TestMarked", "", passed=True), code=0)
    if name == "go-unknown-action":
        lines = go_stream("TestMarked", "", passed=True)
        return emit_go(lines[:2] + [event("teleport", "TestMarked")] + lines[2:], code=0)
    if name == "go-oversize":
        chunk = (event("output", "TestOther", "x" * 4000 + "\n") + "\n").encode()
        out = sys.stdout.buffer
        out.write((event("start") + "\n").encode())
        for _ in range(65 * 1024 * 1024 // len(chunk) + 2):
            out.write(chunk)
        out.write(("\n".join(go_stream("TestMarked", "", passed=True)[1:]) + "\n").encode())
        return 0
    if name == "go-secret":
        return emit_go(go_stream("TestMarked", f"{MARK} {SECRET}-message",
                                 extra_output=[f"{SECRET}-log\n"]), code=1)
    if name == "signal":
        sys.stdout.flush()
        os.kill(os.getpid(), signal.SIGTERM)
        time.sleep(5)
        return 0
    if name == "sleep":
        time.sleep(30)
        return 0
    if name == "lock-log":
        return lock_log_directory()
    raise SystemExit(f"unknown case {name}")


if __name__ == "__main__":
    sys.exit(main(sys.argv))
