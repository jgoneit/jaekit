"""Require completed Go -json test observations; package/helper success is insufficient."""
import json


def require(condition, message):
    if not condition:
        raise ValueError(message)


def native_results(output, required, packages, helpers=()):
    """Require actual run/pass events, including both Git lifecycle subtests.

    Package success, test counts, helper no-ops and parent-only success cannot
    substitute for any required behavior. A skip/failure anywhere is fatal.
    """
    needed, packages, helpers = set(required), set(packages), set(helpers)
    require(needed and packages, "native selection must declare required tests and packages")
    require(not needed.intersection(helpers), "helper no-ops cannot be required behavior")
    started, passed, terminal, package_pass = set(), set(), set(), set()
    for line in output.splitlines():
        event = json.loads(line)
        require(isinstance(event, dict), "Go test event is not an object")
        action, package, name = event.get("Action"), event.get("Package"), event.get("Test")
        require(isinstance(action, str) and isinstance(package, str)
                and (name is None or isinstance(name, str)), "Go test event field types are invalid")
        require(action not in ("fail", "skip"), "native selection contains a failure or skip")
        require(package in packages, "unexpected package in native selection")
        if name:
            require(name.startswith("TestNative"), "unexpected test in native selection")
            key = package + "/" + name
            if action == "run":
                require(key not in started, "native test repeated unexpectedly")
                started.add(key)
            elif action == "pass":
                require(key in started and key not in terminal, "test passed without one actual run")
                terminal.add(key)
                passed.add(key)
        elif action == "pass":
            require(package not in package_pass, "package completed more than once")
            package_pass.add(package)
    require(package_pass == packages, "native package did not complete successfully")
    require(started == terminal, "native test started without completing")
    require(needed <= passed, "required native behavior did not run: " + ", ".join(sorted(needed - passed)))
    return {"required": sorted(needed), "passed": sorted(passed - helpers),
            "helpers": sorted(passed & helpers), "skipped": [],
            "synthetic_failure_injection": sorted(key for key in needed if
                "AdmissionFailure" in key or "UnconfirmedShutdown" in key)}
