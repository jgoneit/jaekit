#!/usr/bin/env python3
"""Observe Windows source boundaries and current-source native PR results.

Only public source, synthetic Git fixtures and live GitHub results are inputs.
Neither private goal documents nor a supplied success report are accepted.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import struct
import subprocess
import sys
import tempfile
import zipfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
MARKER = "<!-- BEGIN WINDOWS CANDIDATE PREFLIGHT -->"
VIOLATION = "windows-review-boundary-missing"
SCENARIO = "TestNativeWindowsEvidencePathScenarios"
REPOSITORY = "jgoneit/jaekit"
PULL_REQUEST = 67
REVIEWS = {4260903124, 4260903143, 4260903159}
PREFLIGHT_FILES = ("archive", "common", "install", "uninstall", "acceptance")
PREFLIGHT_CASES = {"actual-candidate", "valid-install", "valid-remove", "valid-acceptance",
                   "invalid-digest", "mismatched-digest", "duplicate-entry", "ambiguous-case-entry",
                   "unsafe-relative-entry", "unsafe-absolute-entry", "unexpected-entry",
                   "unexpected-script", "directory-entry"}
PREFLIGHT_CASES.update(action + "-tampered-" + name for action in
                       ("install", "remove", "acceptance-first", "acceptance-update") for name in PREFLIGHT_FILES)
PREFLIGHT_CASES.update("missing-file-" + name for name in (*PREFLIGHT_FILES, "checksums"))
PREFLIGHT_CASES.update("missing-entry-" + name for name in PREFLIGHT_FILES)
PUBLIC = ["LICENSE", "README.md", "README.ko.md", "tools/check-result-reference.py",
          "guides/WINDOWS.md", "guides/WINDOWS.en.md", "contracts/run-record.md", "assets/logo.svg"]
PUBLIC += ["examples/check-result/" + name for name in
           ("README.md", "declaration.json", "reference.json", "input.txt")]
PUBLIC += ["tools/windows/" + name for name in
           ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1", "README.md")]


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(value, message):
    if not value:
        raise Violation(message)


def run(argv, cwd=ROOT, env=None, timeout=180):
    try:
        result = subprocess.run(argv, cwd=cwd, env=env, capture_output=True,
                                text=True, encoding="utf-8", timeout=timeout)
    except (OSError, UnicodeError, subprocess.TimeoutExpired) as error:
        raise ObservationError(type(error).__name__) from None
    if result.returncode:
        if argv[0] == "gh":
            raise ObservationError("GitHub request failed; inspect authentication/permission")
        print(result.stdout, end="")
        print(result.stderr, end="", file=sys.stderr)
        raise ObservationError(f"execution failed: {argv[0]} (exit {result.returncode})")
    return result.stdout


def load(name, relative):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def package_boundary():
    package = load("candidate_packager", "tools/windows/package.py")
    with tempfile.TemporaryDirectory(prefix="windows-source-boundary-") as directory:
        base = Path(directory).resolve()
        source = base / "source"
        source.mkdir()
        env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
                   GIT_AUTHOR_NAME="Synthetic", GIT_AUTHOR_EMAIL="synthetic@example.invalid",
                   GIT_COMMITTER_NAME="Synthetic", GIT_COMMITTER_EMAIL="synthetic@example.invalid")
        def git(*args):
            return run(["git", *args], cwd=source, env=env)
        for name in PUBLIC:
            path = source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(("synthetic public: " + name + "\n").encode())
        (source / ".gitignore").write_text("*.private\n", encoding="utf-8")
        git("init", "-q")
        git("-c", "core.autocrlf=false", "add", ".")
        git("-c", "commit.gpgsign=false", "commit", "-qm", "synthetic public source")
        pe = bytearray(128)
        pe[:2] = b"MZ"
        struct.pack_into("<I", pe, 0x3c, 64)
        pe[64:68] = b"PE\0\0"
        struct.pack_into("<H", pe, 68, 0x8664)
        struct.pack_into("<H", pe, 88, 0x20b)
        binary = base / "ha.exe"
        binary.write_bytes(pe)
        private = []
        for parent in ("guides", "contracts", "assets"):
            for name in ("draft.txt", "secret.private"):
                path = source / parent / name
                path.write_bytes(b"synthetic-not-for-distribution")
                private.append(path)
        archive = package.prepare(binary, "0.0.0-synthetic", base / "clean", source)
        with zipfile.ZipFile(archive) as zf:
            require(set(zf.namelist()) == set(PUBLIC) | {"ha.exe", "package.json"},
                    "untracked or ignored source entered candidate")
            for name in PUBLIC:
                require(zf.read(name) == (source / name).read_bytes(), "public source differs")
        require(all(p.read_bytes() == b"synthetic-not-for-distribution" for p in private),
                "local source was changed")
        for mode in ("unstaged", "staged", "deleted", "staged-delete", "staged-add"):
            changed = source / "guides/WINDOWS.md"
            new = source / "guides/new.md"
            original = changed.read_bytes()
            if mode in ("unstaged", "staged"):
                changed.write_bytes(original + b"local edit\n")
            elif mode in ("deleted", "staged-delete"):
                changed.unlink()
            else:
                new.write_bytes(b"not yet committed\n")
            if mode.startswith("staged"):
                git("-c", "core.autocrlf=false", "add", "-A", "guides/WINDOWS.md", "guides/new.md") if new.exists() else git("-c", "core.autocrlf=false", "add", "-A", "guides/WINDOWS.md")
            before = git("status", "--porcelain", "--untracked-files=all")
            index = (source / ".git/index").read_bytes()
            output = base / mode
            rejected = False
            try:
                package.prepare(binary, "0.0.0-synthetic", output, source)
            except (ValueError, OSError):
                rejected = True
            require(rejected and not output.exists(), "dirty public input was packaged: " + mode)
            require(before == git("status", "--porcelain", "--untracked-files=all") and
                    index == (source / ".git/index").read_bytes(), "packaging changed source/index")
            # Reset only this isolated synthetic repository between observations.
            git("reset", "-q", "HEAD", "--", "guides")
            changed.write_bytes(original)
            if new.exists():
                new.unlink()
        print("Observed tracked public content, excluded local files and dirty-input refusal.")


def feature_boundary(criterion):
    if criterion in {"AC-3", "AC-6", "AC-9"}:
        fixtures = ROOT / "testdata/scenarios/_native/windows-evidence-paths"
        require(fixtures.is_dir() and any(fixtures.iterdir()), "command-level evidence scenario absent")
        source = (ROOT / "tools/windows/native_checks.py").read_text()
        require(SCENARIO in source, "Windows command scenario not required by native entrypoint")
    if criterion in {"AC-4", "AC-5", "AC-6", "AC-8", "AC-9"}:
        for name in ("guides/WINDOWS.md", "guides/WINDOWS.en.md"):
            require(MARKER in (ROOT / name).read_text(encoding="utf-8"),
                    "independent public pre-execution verification absent: " + name)
        require((ROOT / "tools/windows/native_preflight_checks.ps1").is_file(),
                "real guide execution observer absent")


def live():
    ci = load("current_ci_observer", "tools/verification-checks/cross_platform_ci.py")
    repo = json.loads(run(["gh", "repo", "view", "--json", "nameWithOwner"]))["nameWithOwner"]
    require(repo == REPOSITORY, "wrong repository for the reviewed Windows change")
    head = run(["git", "rev-parse", "HEAD"]).strip()
    pulls = json.loads(run(["gh", "api", "--method", "GET", f"repos/{repo}/commits/{head}/pulls"]))
    matching = [pr for pr in pulls if pr.get("state") == "open" and pr["head"]["sha"] == head]
    require(len(matching) == 1, "current commit must identify one open PR")
    pr = matching[0]["number"]
    require(pr == PULL_REQUEST, "current code must be reflected in the reviewed PR")
    original = ci.command
    reports = []
    def command(argv, *args, **kwargs):
        if argv[:3] == ["gh", "pr", "view"]:
            argv = [*argv[:3], str(pr), "--repo", repo, *argv[3:]]
        result = original(argv, *args, **kwargs)
        if argv[:3] == ["gh", "run", "view"] and "--log" in argv:
            reports.extend(ci.records(result, "JAEKIT_CI_REPORT "))
        return result
    ci.command = command
    try:
        ci.configuration_present()
        ci.live_run()
    except ci.Violation as error:
        raise Violation(str(error)) from None
    except ci.ObservationError as error:
        raise ObservationError(str(error)) from None
    windows = [r for r in reports if r.get("schema") == "jaekit-native-windows/v1"]
    require(len(windows) == 2, "both actual Windows shell reports required")
    for report in windows:
        tests = report.get("native_tests", {})
        for field in ("required", "passed"):
            require(any(name.endswith("/" + SCENARIO) for name in tests.get(field, [])),
                    "command-level Windows scenario was not required and passed")
        preflight = report.get("preflight_checks", {})
        require(preflight.get("status") == "passed" and preflight.get("schema") == "jaekit-windows-preflight/v1"
                and preflight.get("shell_version") == report["environment"]["shell_version"]
                and not preflight.get("failures"), "same-shell pre-execution observation missing")
        guides = preflight.get("guides", [])
        require({g.get("path") for g in guides} == {"guides/WINDOWS.md", "guides/WINDOWS.en.md"},
                "both public guide execution results required")
        for guide in guides:
            require(not guide.get("skipped"), "guide execution skipped cases")
            for field in ("required", "passed"):
                require(len(guide.get(field, [])) == len(PREFLIGHT_CASES) and
                        set(guide.get(field, [])) == PREFLIGHT_CASES, "guide cases missing or duplicated")
    return repo, pr, head


def replies(repo, pr, head):
    comments = json.loads(run(["gh", "api", "--method", "GET", f"repos/{repo}/pulls/{pr}/comments?per_page=100"]))
    require(len(comments) < 100, "review page truncated")
    author = json.loads(run(["gh", "api", "user"]))["login"]
    for review in sorted(REVIEWS):
        parents = [c for c in comments if c.get("id") == review and not c.get("in_reply_to_id")]
        require(len(parents) == 1, "target P1 review missing: " + str(review))
        found = [c for c in comments if c.get("in_reply_to_id") == review and head in c["body"]
                 and c.get("user", {}).get("login") == author]
        require(found, "current-commit review reply missing: " + str(review))
        require(any("Windows 11" in c["body"] and "/actions/runs/" in c["body"] for c in found),
                "reply must identify native CI and remaining Windows 11 boundary")
    print("All three P1 threads have current-commit replies with CI and Windows 11 scope.")


def main():
    criterion = sys.argv[1]
    structured = criterion not in {"AC-2", "AC-7"}
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    code = 0
    try:
        if criterion == "AC-1":
            package_boundary()
        elif criterion == "AC-2":
            print(run([sys.executable, "-m", "unittest", "discover", "-s", "tools/windows", "-p", "test_package.py"]))
        elif criterion == "AC-7":
            # The same-source Ubuntu/common, documentation and native OS jobs
            # cover preservation. Re-observe their live results, without a
            # second identical local full-suite execution or cached success file.
            live()
        else:
            feature_boundary(criterion)
            if criterion == "AC-8":
                print(run([sys.executable, "tools/check_public_tree.py"]))
                print(run([sys.executable, "tools/check_public_docs.py"]))
            else:
                repo, pr, head = live()
                if criterion == "AC-9":
                    replies(repo, pr, head)
    except Violation as error:
        observation.update(status="violation", violation=VIOLATION)
        print(str(error), file=sys.stderr)
        code = 1
    except (ObservationError, OSError, ValueError, KeyError) as error:
        observation.update(status="error", reason="execution_error")
        print(str(error), file=sys.stderr)
        code = 2
    if structured and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "windows-review-p1", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
