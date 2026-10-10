"""Bounded, independent facts for repository check-result/v1 producers.

Each declared slot is an independent property of one run, never a retry.
Assertions are identified by the exception actually raised, not the declaration.
"""
import re
import unittest

IDENTIFIER = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}\Z")
TAG = re.compile(r"\[violation:([A-Za-z0-9][A-Za-z0-9_.:/-]{0,127})\]")
ERRORS = {"dependency_missing", "collection_error", "import_error", "compile_error",
          "browser_start_error", "setup_error", "execution_error", "permission_denied"}


class RequirementViolation(AssertionError):
    """An assertion whose observed requirement identity is supplied at failure."""
    def __init__(self, violation, message=""):
        if not isinstance(violation, str) or not IDENTIFIER.fullmatch(violation):
            raise ValueError("invalid observed violation identifier")
        self.violation = violation
        super().__init__(message or violation)


def facts():
    return {"passed": 0, "violations": [], "errors": [], "skipped": [], "unclassified": False}


def assertion_identity(error, traceback):
    # Read only the actual exception, never captured stdout or declaration data.
    if isinstance(error, RequirementViolation):
        return error.violation, False
    # An assertion's generated message can contain arbitrary compared reprs and
    # suffix messages. Only an explicit TestCase.fail call may use a text tag.
    # TestCase's own assertion helpers sometimes call fail internally; those
    # generated messages are not explicit observer declarations either.
    frames = []
    while traceback is not None:
        frames.append(traceback.tb_frame)
        traceback = traceback.tb_next
    explicit_fail = (len(frames) >= 2
                     and frames[-1].f_code is unittest.TestCase.fail.__code__
                     and frames[-2].f_globals.get("__name__") != "unittest.case")
    if explicit_fail and len(error.args) == 1 and type(error.args[0]) is str:
        message = error.args[0]
        tag = TAG.match(message)
        if tag and len(TAG.findall(message)) == 1:
            return tag.group(1), False
        if message.startswith("[requirement]") and not TAG.search(message):
            return "requirement-not-met", False
    return "unclassified-assertion", True


class ObservationResult(unittest.TextTestResult):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.facts = facts()

    def addSuccess(self, test):
        super().addSuccess(test)
        self.facts["passed"] += 1

    def addFailure(self, test, err):
        # unittest formats failures by deleting its internal traceback frames.
        # Capture the actual origin before that presentation-only cleanup.
        identity, unknown = assertion_identity(err[1], err[2])
        super().addFailure(test, err)
        self.facts["violations"].append(identity)
        self.facts["unclassified"] |= unknown

    def addError(self, test, err):
        super().addError(test, err)
        self.facts["errors"].append("execution_error")

    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self.facts["skipped"].append("skipped")

    def addExpectedFailure(self, test, err):
        super().addExpectedFailure(test, err)
        self.facts["errors"].append("setup_error")

    def addUnexpectedSuccess(self, test):
        super().addUnexpectedSuccess(test)
        self.facts["errors"].append("setup_error")

    def addSubTest(self, test, subtest, err):
        if err is None:
            super().addSubTest(test, subtest, err)
            return
        if issubclass(err[0], test.failureException):
            identity, unknown = assertion_identity(err[1], err[2])
            super().addSubTest(test, subtest, err)
            self.facts["violations"].append(identity)
            self.facts["unclassified"] |= unknown
        else:
            super().addSubTest(test, subtest, err)
            self.facts["errors"].append("execution_error")


def run_suite(suite, stream):
    result = unittest.TextTestRunner(stream=stream, verbosity=2,
                                     resultclass=ObservationResult).run(suite)
    if result.testsRun == 0 and not result.facts["errors"] and not result.facts["skipped"]:
        result.facts["errors"].append("collection_error")
    return result.facts


def failure(reason):
    result = facts()
    result["errors"].append(reason)
    return result


def summary(result):
    """Compatibility view for debugging only; not a structured evidence report."""
    if result["errors"]:
        reason = "execution_error" if "execution_error" in result["errors"] else result["errors"][0]
        return {"status": "error", "reason": reason}
    if result["violations"]:
        return {"status": "violation"}
    if result["skipped"]:
        return {"status": "skip", "reason": result["skipped"][0]}
    return {"status": "pass"} if result["passed"] else {"status": "error", "reason": "collection_error"}


def target_names(primary, slots=8):
    return {"additional_violations": [primary + ".assertion-" + str(i) for i in range(2, slots + 1)],
            "execution": primary + ".execution", "selection": primary + ".selection",
            "attribution": primary + ".attribution"}


def target_ids(primary, names):
    if set(names) != {"additional_violations", "execution", "selection", "attribution"}:
        raise ValueError("result_targets must declare all observation properties")
    extra = names["additional_violations"]
    if not isinstance(extra, list) or not 1 <= len(extra) <= 63:
        raise ValueError("declare 2 to 64 assertion slots")
    ids = [primary, *extra, names["execution"], names["selection"], names["attribution"]]
    if len(set(ids)) != len(ids) or any(not isinstance(v, str) or not IDENTIFIER.fullmatch(v) for v in ids):
        raise ValueError("invalid or duplicate observation target")
    return ids


def observations(result, primary, names):
    ids = target_ids(primary, names)
    slots = [primary, *names["additional_violations"]]
    report = []
    for index, target in enumerate(slots):
        item = {"target": target, "attempt": 1, "status": "pass"}
        if index < len(result["violations"]):
            item.update(status="violation", violation=result["violations"][index])
        # A failed launch/empty selection cannot establish the primary assertion.
        elif index == 0 and not result["passed"]:
            if result["errors"]:
                item.update(status="error", reason=result["errors"][0])
            else:
                item.update(status="skip", reason="not_selected")
        report.append(item)
    errors = list(result["errors"])
    if "execution_error" in errors:
        errors.remove("execution_error")
        errors.insert(0, "execution_error")
    if len(result["violations"]) > len(slots):
        errors.append("collection_error")
    facets = [(names["execution"], "error", errors[0] if errors else None),
              (names["selection"], "skip", result["skipped"][0] if result["skipped"] else None),
              (names["attribution"], "error", "setup_error" if result["unclassified"] else None)]
    for target, status, reason in facets:
        item = {"target": target, "attempt": 1, "status": status if reason else "pass"}
        if reason:
            item["reason"] = reason
        report.append(item)
    return report


def declaration_targets(primary, expected, names):
    ids = target_ids(primary, names)
    return [{"id": value, "violation": expected if index == 0 else "no-additional-violation"}
            for index, value in enumerate(ids)]
