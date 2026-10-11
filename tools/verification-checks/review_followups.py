#!/usr/bin/env python3
"""Goal observer for the integrated review follow-ups.

AC-1 through AC-5 run each component goal's observer on this combined tree;
a component violation or error stays a violation or error here. AC-6 reads
the public review replies and issue comments with the user's gh CLI. AC-7
runs the loaded-source regression. AC-8 reads the public regression registry
in this checkout and in a depth-1 clone of its committed HEAD.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
REPO = "jgoneit/jaekit"
OWNER = "jgoneit"
FINDINGS = "tools/goal-checks/review-findings"
CHILDREN = {
    "AC-1": [["tools/verification-checks/review_producer_results.py", f"AC-{n}"] for n in range(1, 6)],
    "AC-2": [["tools/goal-checks/produce.py", f"{FINDINGS}/AC-{n}.declaration.json", f"{FINDINGS}/AC-{n}.targets.json"]
             for n in range(1, 7)],
    "AC-3": [["tools/verification-checks/review_archive_boundaries.py", f"AC-{n}"] for n in (1, 2, 3, 5)],
    "AC-4": [["tools/verification-checks/review_release_evidence.py", f"AC-{n}"] for n in range(1, 5)],
    "AC-5": [["tools/verification-checks/review_regression_gates.py", f"AC-{n}"] for n in range(1, 7)],
}
IDENTITIES = {"AC-1": "integrated-producer-meaning-lost", "AC-2": "integrated-finding-contract-lost",
              "AC-3": "integrated-archive-limit-lost", "AC-4": "integrated-release-evidence-lost",
              "AC-5": "integrated-regression-evidence-lost", "AC-6": "review-decision-unlinked",
              "AC-7": "integration-source-unbound", "AC-8": "review-regression-unlinked"}
# Every review comment on the seven pull requests, by pull request.
REVIEWS = {
    31: (4238616256, 4238616259, 4238616263, 4238616267, 4238616271),
    32: (4238612456, 4238612458, 4238612460),
    33: (4238640678, 4238640680, 4238640686),
    34: (4238665434,),
    36: (4238712039, 4238712044, 4238712046, 4238712048, 4238712050, 4238712052, 4238712055),
    37: (4238782962, 4238782964, 4238782966),
    38: (4239756437, 4239756439, 4239756443, 4239756447),
}
NEW_ISSUES = (39, 40, 41, 42, 43, 44)
# Existing issues augmented from a review, and the pull request whose review they cite.
AUGMENTED = {13: 38, 16: 33, 27: 36}
# Review regressions that the integrated registry connects to their routes.
LINKS = (
    ("post-completion-recorded-facts", "internal/record/finding_size_test.go", "TestFindingLineBound"),
    ("post-completion-recorded-facts", "cmd/ha/findings_review_test.go", "TestFindingRetryEquivalentScenario"),
    ("post-completion-recorded-facts", "cmd/ha/findings_review_test.go", "TestFindingRequiredListsScenario"),
    ("post-completion-recorded-facts", "cmd/ha/findings_review_test.go", "TestFindingResolutionRefsScenario"),
    ("post-completion-recorded-facts", "cmd/ha/findings_review_test.go", "TestFindingStoredReuseCLI"),
    ("goal-producer-meaning", "tools/producer-checks/test_semantics.py", "ProvidedCommandTargets"),
    ("goal-producer-meaning", "tools/producer-checks/test_semantics.py", "OverflowWithErrors"),
    ("goal-producer-meaning", "tools/producer-checks/test_semantics.py", "ExitContractKeys"),
    ("archive-consumer-input-boundary", "tools/release/test_archive_bounds.py", "ValidatorBounds"),
    ("archive-consumer-input-boundary", "tools/release/test_archive_bounds.py", "Preservation"),
    ("shell-reachability-gap", "tools/release-checks/integration_evidence_boundaries.py", "FunctionScopeEvidence"),
    ("observation-identity-gap", "tools/release-checks/integration_evidence_boundaries.py", "ShallowProductSource"),
    ("observation-identity-gap", "tools/release-checks/integration_evidence_boundaries.py", "LoadedSourceCoverage"),
    ("observation-identity-gap", "tools/release-checks/integration_direct_install.py", "InstalledPathAvailability"),
)
RUNNER = r'''
import json, sys, unittest
class Result(unittest.TestResult):
    def __init__(self):
        super().__init__()
        self.cases = []
    def addSubTest(self, test, subtest, outcome):
        super().addSubTest(test, subtest, outcome)
        if outcome is not None:
            kind = "failure" if issubclass(outcome[0], test.failureException) else "error"
            self.cases.append((kind, subtest.id(), self._exc_info_to_string(outcome, test)))
    def addFailure(self, test, err):
        super().addFailure(test, err)
        self.cases.append(("failure", test.id(), self._exc_info_to_string(err, test)))
    def addError(self, test, err):
        super().addError(test, err)
        self.cases.append(("error", test.id(), self._exc_info_to_string(err, test)))
    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self.cases.append(("skip", test.id(), reason))
result = Result()
unittest.defaultTestLoader.loadTestsFromNames(sys.argv[1:]).run(result)
print("JAEKIT_OBSERVED " + json.dumps({"run": result.testsRun, "cases": result.cases}))
'''


class Violation(Exception):
    pass


class Unobserved(Exception):
    pass


def clean_env():
    env = {key: value for key, value in os.environ.items() if not key.startswith("HA_EVIDENCE_")}
    return dict(env, PYTHONDONTWRITEBYTECODE="1")


def children(criterion):
    """A component violation or error is never upgraded to a pass here."""
    statuses = []
    for argv in CHILDREN[criterion]:
        with tempfile.TemporaryDirectory(prefix="jaekit-followups-child-") as directory:
            report = Path(directory) / "result.json"
            env = dict(clean_env(), HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="review-followups-component",
                       HA_EVIDENCE_DECLARATION_DIGEST="0" * 64)
            process = subprocess.run([sys.executable, "-B", *argv], cwd=ROOT, env=env, capture_output=True,
                                     text=True, timeout=3600)
            found = []
            if report.is_file():
                try:
                    found = [o.get("status") for o in json.loads(report.read_text()).get("observations", [])]
                except (ValueError, AttributeError):
                    found = []
            print(" ".join(argv), "->", found or "no report", file=sys.stderr)
            if not found or set(found) - {"pass", "violation"}:
                print(process.stdout[-3000:] + process.stderr[-3000:], file=sys.stderr)
                raise Unobserved(" ".join(argv))
            statuses.extend(found)
    if "violation" in statuses:
        raise Violation(f"{statuses.count('violation')} component observations are violations on the combined tree")


def gh(*args):
    try:
        process = subprocess.run(["gh", "api", *args], capture_output=True, text=True, timeout=300)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unobserved(f"gh api {args[0]}: {error}") from error
    if process.returncode:
        raise Unobserved(f"gh api {args[0]}: {process.stderr.strip()[:300]}")
    return process.stdout


def lines(*args):
    return [json.loads(line) for line in gh(*args).splitlines() if line.strip()]


def contains(sha, head):
    """Whether a reply's hexadecimal word is a commit contained in the pull request head."""
    try:
        process = subprocess.run(["gh", "api", f"repos/{REPO}/compare/{sha}...{head}", "--jq", ".status"],
                                 capture_output=True, text=True, timeout=300)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unobserved(f"compare {sha}: {error}") from error
    if process.returncode and "HTTP 404" in process.stderr:
        return False
    if process.returncode:
        raise Unobserved(f"compare {sha}: {process.stderr.strip()[:300]}")
    return process.stdout.strip() in ("ahead", "identical")


def links():
    """Each review decision names a commit on its pull request and an issue; new and augmented issues exist."""
    problems = []
    for pr, comments in REVIEWS.items():
        head = gh(f"repos/{REPO}/pulls/{pr}", "--jq", ".head.sha").strip()
        replies = lines("--paginate", f"repos/{REPO}/pulls/{pr}/comments",
                        "--jq", ".[] | {in_reply_to_id, user: .user.login, body}")
        contained = {}
        for comment in comments:
            ok = False
            for reply in replies:
                if reply["in_reply_to_id"] != comment or reply["user"] != OWNER:
                    continue
                if not re.search(r"#\d+", reply["body"]):
                    continue
                # ASCII boundaries: a commit is often followed by a Korean particle.
                for sha in re.findall(r"(?<![0-9A-Za-z])[0-9a-f]{7,40}(?![0-9A-Za-z])", reply["body"]):
                    if sha not in contained:
                        contained[sha] = contains(sha, head)
                    ok = ok or contained[sha]
            if not ok:
                problems.append(f"PR #{pr} review {comment} has no owner reply naming its commit and issue")
    for issue in NEW_ISSUES:
        kind = gh(f"repos/{REPO}/issues/{issue}", "--jq", "if .pull_request then \"pull\" else \"issue\" end").strip()
        if kind != "issue":
            problems.append(f"#{issue} is not an issue")
    for issue, pr in AUGMENTED.items():
        bodies = lines("--paginate", f"repos/{REPO}/issues/{issue}/comments",
                       "--jq", ".[] | {user: .user.login, body}")
        if not any(b["user"] == OWNER and f"/pull/{pr}#discussion_r" in b["body"] for b in bodies):
            problems.append(f"#{issue} has no augmentation citing the PR #{pr} review")
    if problems:
        print("\n".join(problems), file=sys.stderr)
        raise Violation(f"{len(problems)} review decisions are not linked")


def tagged(identity, names):
    process = subprocess.run([sys.executable, "-B", "-c", RUNNER, *names], cwd=ROOT / "tools/release-checks",
                             env=clean_env(), capture_output=True, text=True, timeout=1800)
    found = [line for line in process.stdout.splitlines() if line.startswith("JAEKIT_OBSERVED ")]
    if process.returncode or len(found) != 1:
        print(process.stdout + process.stderr, file=sys.stderr)
        raise Unobserved("selected regressions did not report")
    summary = json.loads(found[0][len("JAEKIT_OBSERVED "):])
    for kind, case, detail in summary["cases"]:
        print(f"{kind.upper()}: {case}\n{detail}", file=sys.stderr)
    kinds = {kind for kind, _, _ in summary["cases"]}
    if not summary["run"] or kinds - {"failure"}:
        raise Unobserved("selected regressions errored, skipped or were empty")
    if kinds:
        if all(f"[violation:{identity}]" in detail for _, _, detail in summary["cases"]):
            raise Violation(identity)
        raise Unobserved("untagged failure")


def registry(root):
    process = subprocess.run([sys.executable, "-B", "tools/regression_map.py", "--format", "json"], cwd=root,
                             env=clean_env(), capture_output=True, text=True, timeout=1800)
    if process.returncode == 1:
        raise Violation(f"registry invalid in {root.name}: {process.stderr.strip()[:500]}")
    if process.returncode:
        raise Unobserved(process.stderr.strip()[:500])
    return json.loads(process.stdout)


def regressions():
    data = json.loads((ROOT / "contracts/regressions.json").read_text(encoding="utf-8"))
    routes = {route["id"]: route for route in data["routes"]}
    missing = [f"{route}: {file} {selector}" for route, file, selector in LINKS
               if routes.get(route, {}).get("status") != "protected"
               or not any(c["file"] == file and c["selector"] == selector for c in routes[route]["checks"])]
    if missing:
        print("\n".join(missing), file=sys.stderr)
        raise Violation(f"{len(missing)} review regressions are not connected to their routes")
    expected = registry(ROOT)
    with tempfile.TemporaryDirectory(prefix="jaekit-followups-clone-") as directory:
        clone = Path(directory) / "public"
        head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True).stdout.strip()
        process = subprocess.run(["git", "clone", "-q", "--depth", "1", "--no-tags", ROOT.as_uri(), str(clone)],
                                 capture_output=True, text=True, timeout=600)
        cloned = subprocess.run(["git", "rev-parse", "HEAD"], cwd=clone, capture_output=True, text=True).stdout.strip() \
            if not process.returncode else ""
        if process.returncode or not head or cloned != head:
            raise Unobserved("a depth-1 clone of the committed tree could not be made")
        if registry(clone) != expected:
            raise Violation("a depth-1 public clone gives a different registry result")


def observe(criterion):
    try:
        if criterion in CHILDREN:
            children(criterion)
        elif criterion == "AC-6":
            links()
        elif criterion == "AC-7":
            tagged(IDENTITIES["AC-7"], ["integration_evidence_boundaries.LoadedSourceCoverage"])
        elif criterion == "AC-8":
            regressions()
        else:
            return {"status": "error", "reason": "execution_error"}
    except Violation as error:
        print(error, file=sys.stderr)
        return {"status": "violation", "violation": IDENTITIES[criterion]}
    except (Unobserved, OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
        print(f"unobserved: {error}", file=sys.stderr)
        return {"status": "error", "reason": "execution_error"}
    return {"status": "pass"}


def main():
    criterion = sys.argv[1]
    observation = {"target": criterion, "attempt": 1, **observe(criterion)}
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-review-followups-observer", "producer_version": "1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "attempts_complete": True, "observations": [observation]}
        with Path(os.environ["HA_EVIDENCE_PATH"]).open("x") as output:
            json.dump(report, output)
    print(criterion, observation["status"])
    return 0 if observation["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
