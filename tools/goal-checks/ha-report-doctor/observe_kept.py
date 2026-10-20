"""Existing behavior and protected routes stay as they are (AC-28, AC-29).

The earlier content is the expected content tracked next to this file:
capabilities-before.json and usage-before.txt (the output of `ha` built
before this goal) and regressions-before.json (the registry before this
goal). Nothing reads another commit. A test failure in the existing suites,
a capability or usage line that changed or disappeared, a registry that
differs, or a protected check that fails is a violation; a build failure,
undecodable output, an unreadable expected file or a tool that cannot start
is unavailable.
"""
import json
import os
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from common import FEATURES, HERE, ROOT, Temporary, Unavailable, build, require, run, text  # noqa: E402


def expected(name):
    try:
        return (HERE / name).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"{name}: {error}") from None


def parsed(data, what):
    try:
        return json.loads(data)
    except ValueError:
        raise Unavailable(f"{what} is not JSON") from None


def only_added(before, after, path="capabilities"):
    """Differences other than added keys and added list items, as text."""
    problems = []
    if isinstance(before, dict):
        if not isinstance(after, dict):
            return [f"{path} is no longer an object"]
        for key, value in before.items():
            if key not in after:
                problems.append(f"{path}.{key} was removed")
            else:
                problems += only_added(value, after[key], f"{path}.{key}")
    elif isinstance(before, list):
        if not isinstance(after, list):
            return [f"{path} is no longer a list"]
        position = 0
        for item in before:
            try:
                position = after.index(item, position) + 1
            except ValueError:
                problems.append(f"{path} lost {item!r} or changed its order")
    elif before != after:
        problems.append(f"{path} changed from {before!r} to {after!r}")
    return problems


class Kept(unittest.TestCase):
    def test_existing_suites_pass(self):
        """AC-28: the Go tests of the existing commands, records and scenarios pass."""
        v = "existing-behavior-changed"
        env = {k: val for k, val in os.environ.items() if not k.startswith(("HA_", "GIT_"))}
        env.update(GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
        result = run(["go", "test", "-json", "-count=1", "./cmd/ha", "./internal/..."], ROOT, env, timeout=2400)
        events = []
        for line in text(result.stdout).splitlines():
            try:
                events.append(json.loads(line))
            except ValueError:
                raise Unavailable(f"go test printed a line that is not an event: {line[:200]!r}") from None
        if any(e.get("Action") == "fail" and e.get("FailedBuild") for e in events):
            raise Unavailable("a package did not build")
        failed = sorted({f"{e.get('Package')} {e['Test']}" for e in events if e.get("Action") == "fail" and e.get("Test")})
        passed = [e for e in events if e.get("Action") == "pass" and e.get("Test")]
        if result.returncode and not failed:
            raise Unavailable(f"go test exited {result.returncode} without a failed test: {text(result.stderr)[-500:]}")
        if not passed:
            raise Unavailable("go test ran no test")
        require(self, not failed, v, f"existing tests failed: {failed[:10]}")

    def test_capabilities_only_add(self):
        """AC-28: capabilities keep every earlier key, value and list item in order and add report/v1 and doctor/v1."""
        v = "existing-behavior-changed"
        before = parsed(expected("capabilities-before.json"), "capabilities-before.json")
        with Temporary("jaekit-kept-") as directory:
            ha = build(directory / "ha")
            result = run([ha, "capabilities", "--format", "json"], directory, timeout=60)
            if result.returncode:
                raise Unavailable(f"ha capabilities exited {result.returncode}")
            after = parsed(text(result.stdout), "ha capabilities output")
        problems = only_added(before, after)
        problems += [f"features lacks {f}" for f in FEATURES if f not in after.get("features", [])]
        require(self, not problems, v, "; ".join(problems))

    def test_usage_only_adds_command_lines(self):
        """AC-28: the usage text changes only by the two new command lines."""
        v = "existing-behavior-changed"
        before = expected("usage-before.txt")
        with Temporary("jaekit-kept-") as directory:
            ha = build(directory / "ha")
            shown = run([ha, "--help"], directory, timeout=60)
            bare = run([ha], directory, timeout=60)
        if shown.returncode != 0:
            raise Unavailable(f"ha --help exited {shown.returncode}")
        for name, out, code in (("ha --help", text(shown.stdout), shown.returncode),
                                ("ha", text(bare.stderr), bare.returncode)):
            lines = out.split("\n")
            new = [l for l in lines if l.startswith(("  ha report ", "  ha doctor "))]
            rest = "\n".join(l for l in lines if not l.startswith(("  ha report ", "  ha doctor ")))
            require(self, rest == before, v, f"{name}: usage changed beyond the new command lines")
            require(self, len(new) == 2, v, f"{name}: {len(new)} new command lines")
        require(self, bare.returncode == 64, v, f"ha without arguments exited {bare.returncode}")

    def test_protected_routes_unchanged(self):
        """AC-29: the registry is the same value as before this goal."""
        v = "protected-route-changed"
        before = parsed(expected("regressions-before.json"), "regressions-before.json")
        try:
            after = json.loads((ROOT / "contracts/regressions.json").read_text(encoding="utf-8"))
        except FileNotFoundError:
            self.fail(f"[violation:{v}] contracts/regressions.json is missing")
        except (OSError, UnicodeDecodeError, ValueError) as error:
            raise Unavailable(f"contracts/regressions.json: {error}") from None
        if after != before:
            ids = lambda d: {r.get("id"): r for r in d.get("routes", [])}
            changed = sorted(i for i in set(ids(before)) | set(ids(after)) if ids(before).get(i) != ids(after).get(i))
            self.fail(f"[violation:{v}] the registry changed: routes {changed[:10]}")

    def test_protected_checks_pass(self):
        """AC-29: every check a protected route selects passes in this tree."""
        v = "protected-route-changed"
        env = {k: val for k, val in os.environ.items() if not k.startswith(("HA_", "GIT_"))}
        result = run([sys.executable, "tools/regression-checks/check.py", "--criterion", "2"], ROOT, env, timeout=2400)
        last = (text(result.stdout).strip().splitlines() or [""])[-1]
        if last == "regression contract 2 pass" and result.returncode == 0:
            return
        if last == "regression contract 2 violation":
            self.fail(f"[violation:{v}] a protected check failed: {text(result.stderr)[-800:]}")
        raise Unavailable(f"the protected checks could not be observed ({last!r}): {text(result.stderr)[-800:]}")


if __name__ == "__main__":
    unittest.main()
