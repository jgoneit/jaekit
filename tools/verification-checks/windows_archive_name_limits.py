#!/usr/bin/env python3
"""Observe the Windows archive name limits.

Change conditions observe behavior, not text: committed public files that the
Windows archive rule refuses must fail the release archive regressions that
the common verification runs (in depth-one and full clones and in a source
snapshot); the rule refuses `~` names before and after publication; HEAD's
release run shows the installer's own name refusal on both PowerShell
versions; the guide states it and the registry's new routes select tests that
fail against the old rule. On the base commit the old behavior is the declared
violation. Missing GitHub access, an unfinished, cancelled or broken CI run, a
test that could not be collected or an unreadable input is an observation
error, never a violation. Temporary copies live outside the checkout, and
children write no bytecode into it.
"""
from __future__ import annotations

import ast
import concurrent.futures
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import struct
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import zipfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
DECLARATIONS = ROOT / "tools/verification-checks/windows-archive-name-limits"
REPO = "jgoneit/jaekit"
SCRATCH = Path(tempfile.gettempdir()).resolve()
# Not a public tag: a request that escaped the fake GitHub could not reach a real one.
SYNTHETIC = "9.8.7"
RELEASE_TESTS = ("-m", "unittest", "discover", "-s", "tools/release", "-p", "test_*.py")
# Temporary copies only: no hooks, signing or identity from the host's configuration.
GIT_OPTIONS = ("-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid",
               "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null")
NAME_REFUSAL = "Unsafe package path"


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(value, message):
    if not value:
        raise Violation(message)


def text(path, root=ROOT):
    try:
        return (root / path).read_text(encoding="utf-8")
    except OSError:
        return ""


def child_env(**extra):
    """No evidence file, no Git repository or index chosen by the caller, no bytecode."""
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(("HA_EVIDENCE_", "GIT_", "PYTHONPATH"))}
    env.update(PYTHONDONTWRITEBYTECODE="1", GIT_OPTIONAL_LOCKS="0", **extra)
    return env


def run(argv, timeout=600, env=None, cwd=ROOT, data=None, show=True):
    try:
        result = subprocess.run(argv, cwd=cwd, env=env or child_env(), capture_output=True, timeout=timeout,
                                input=data)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ObservationError(f"{type(error).__name__}: {' '.join(map(str, argv[:4]))}") from None
    result.stdout = result.stdout.decode("utf-8", "replace")
    result.stderr = result.stderr.decode("utf-8", "replace")
    if result.returncode and show:
        print(result.stdout[-2000:], end="")
        print(result.stderr[-2000:], end="", file=sys.stderr)
    return result


def git(cwd, *args, data=None):
    result = run(["git", *GIT_OPTIONS, *args], cwd=cwd, data=data, timeout=300)
    if result.returncode:
        raise ObservationError("git " + " ".join(args[:2]) + " failed in a temporary copy")
    return result.stdout


def unit(argv, cwd=ROOT, timeout=600):
    """Run unittest; (exit code, output). A test that could not be loaded, or
    no test at all, is an observation error rather than a failed check."""
    result = run([sys.executable, *argv], cwd=cwd, timeout=timeout, show=False)
    output = result.stdout + result.stderr
    if "unittest.loader._FailedTest" in output or "unittest.loader.ModuleImportFailure" in output:
        raise ObservationError("a selected test module could not be loaded: " + output[-600:])
    if re.search(r"^Ran 0 tests", output, re.M) or "NO TESTS RAN" in output:
        raise ObservationError("no test was selected: " + " ".join(argv[-4:]))
    return result.returncode, output


def load(name, path, root=ROOT):
    spec = importlib.util.spec_from_file_location(name, root / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def head(root=ROOT):
    result = run(["git", "rev-parse", "HEAD"], cwd=root)
    if result.returncode:
        raise ObservationError("HEAD is unreadable")
    return result.stdout.strip()


def repository(root=ROOT):
    url = run(["git", "remote", "get-url", "origin"], cwd=root).stdout.strip()
    match = re.search(r"github\.com[:/]([^/]+/[^/]+?)(?:\.git)?$", url)
    if not match:
        raise ObservationError("origin is not a GitHub repository")
    return match[1]


def api(path):
    result = run(["gh", "api", "--method", "GET", path], timeout=120)
    if result.returncode:
        raise ObservationError("GitHub request failed; check authentication and permission")
    return json.loads(result.stdout)


# ------------------------------------------------------------ copies of HEAD

@contextlib.contextmanager
def clone(depth=True):
    """A clone of HEAD outside the checkout (depth one unless `depth` is false)."""
    expected = head()
    with tempfile.TemporaryDirectory(prefix="jaekit-name-limits-clone-", dir=SCRATCH) as temp:
        target = Path(temp) / "clone"
        argv = ["git", "clone", "--quiet", "--no-tags", *(["--depth", "1"] if depth else []), ROOT.as_uri(), str(target)]
        if run(argv, timeout=300).returncode:
            raise ObservationError("a clone of HEAD could not be made")
        shallow = run(["git", "rev-parse", "--is-shallow-repository"], cwd=target).stdout.strip()
        if shallow != ("true" if depth else "false") or head(target) != expected:
            raise ObservationError("the clone is not HEAD with the requested history")
        yield target


@contextlib.contextmanager
def snapshot():
    """HEAD's tracked files without a Git directory."""
    with tempfile.TemporaryDirectory(prefix="jaekit-name-limits-snapshot-", dir=SCRATCH) as temp:
        target = Path(temp) / "source"
        target.mkdir()
        archive = subprocess.run(["git", "archive", "--format=tar", "HEAD"], cwd=ROOT, capture_output=True,
                                 env=child_env(), timeout=300)
        if archive.returncode:
            raise ObservationError("HEAD's files could not be archived")
        with tarfile.open(fileobj=io.BytesIO(archive.stdout)) as source:
            source.extractall(target, filter="data")
        yield target


def stage(target, files):
    """Put `files` into the index through Git's object store only. The working
    tree keeps none of them, so names a case-insensitive file system cannot
    hold still reach the index, as they would from another host."""
    for name, data in files.items():
        oid = git(target, "hash-object", "-w", "--stdin", data=data).strip()
        git(target, "update-index", "--add", "--cacheinfo", f"100644,{oid},{name}")


def commit(target, files):
    stage(target, files)
    git(target, "commit", "--quiet", "--no-verify", "-m", "synthetic public files")
    listed = git(target, "ls-tree", "-r", "-z", "--name-only", "HEAD").split("\0")
    if not set(files) <= set(listed):
        raise ObservationError("a synthetic public file did not reach the commit")


def release_tests(target):
    """The release archive regressions, as tools/verify.py's documentation group runs them."""
    return unit(RELEASE_TESTS, cwd=target)


# ------------------------------------------------------------ AC-1

BODY = b"Synthetic public document.\n"
COMMITTED = (
    ("a non-ASCII name", ("guides/café.md",), r"printable ASCII|non-ASCII"),
    ("local metadata in another case", ("guides/Node_Modules/notes.md",), r"local metadata|generated cache"),
    ("a generated cache in another case", ("guides/tool.PYC",), r"local metadata|generated cache"),
    ("a reserved device name", ("guides/CON.md",), r"unsafe"),
    ("a trailing dot", ("guides/notes.md.",), r"unsafe"),
    ("names differing only in case", ("guides/Extra.md", "guides/extra.md"), r"differ only in case"),
    ("a file named like a folder in another case", ("guides/Extra", "guides/extra/notes.md"),
     r"file/directory collision"),
    ("a short-name-like name", ("guides/abcdef~1.md",), r"short name|8\.3"),
)
# A case-insensitive file system cannot hold these pairs in a source snapshot.
ON_DISK = tuple(case for case in COMMITTED if len(case[1]) == 1)


def judged_failure(label, names, reason, code, output):
    """None when the run failed naming every path and the reason, else why not."""
    if code == 0:
        return f"a committed public file with {label} ({', '.join(names)}) does not fail the release archive regressions"
    if not all(name in output for name in names) or not re.search(reason, output, re.I):
        return (f"the failure for {label} does not name {', '.join(names)} with the reason: "
                + output.strip()[-300:])
    return None


def ac1():
    verify = text("tools/verify.py")
    if '("Release archive regressions", "release")' not in verify:
        raise ObservationError("tools/verify.py no longer names the release archive regressions this check runs")

    def committed_case(case, depth):
        with clone(depth) as target:
            commit(target, {name: BODY for name in case[1]})
            return case, release_tests(target)

    def detached_case(case):
        with snapshot() as source:
            for name in case[1] if case else ():
                (source / name).parent.mkdir(parents=True, exist_ok=True)
                (source / name).write_bytes(BODY)
            return case, release_tests(source)

    def clean():
        with clone() as target:
            return release_tests(target)

    def unrelated():
        # Untracked, ignored and staged-but-uncommitted files the rule would refuse if committed.
        with clone() as target:
            (target / "guides/untracked-café.md").write_bytes(b"untracked\n")
            (target / "guides/__pycache__").mkdir()
            (target / "guides/__pycache__/notes.cpython-314.pyc").write_bytes(b"ignored\n")
            stage(target, {"guides/Staged_Node_Modules/node_modules/notes.md": BODY, "guides/staged~1.md": BODY})
            others = git(target, "ls-files", "-z", "--others", "--exclude-standard")
            if "guides/untracked-café.md" not in others or "notes.cpython-314.pyc" in others:
                raise ObservationError("the synthetic untracked and ignored files are not seen as such")
            return release_tests(target)

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        controls = [pool.submit(clean), pool.submit(detached_case, None)]
        unrelated_files = pool.submit(unrelated)
        shallow = [pool.submit(committed_case, case, True) for case in COMMITTED]
        full = [pool.submit(committed_case, case, False) for case in COMMITTED]
        loose = [pool.submit(detached_case, case) for case in ON_DISK]
        control_results = [future.result() for future in controls]
        if any(code for code, _ in (control_results[0], control_results[1][1])):
            raise ObservationError("the release archive regressions fail without any added file: "
                                   + (control_results[0][1] + control_results[1][1][1])[-600:])
        failures = []
        for mode, futures in (("a depth-one clone", shallow), ("a full clone", full), ("a source snapshot", loose)):
            for future in futures:
                case, (code, output) = future.result()
                problem = judged_failure(*case, code, output)
                if problem:
                    failures.append(f"in {mode}: {problem}")
        code, output = unrelated_files.result()
        if code:
            failures.append("untracked, ignored or staged-but-uncommitted files changed the release archive "
                            "regressions' verdict: " + output.strip()[-300:])
    require(not failures, "\n".join(failures))


# ------------------------------------------------------------ synthetic release (AC-2)

def pe_amd64():
    data = bytearray(512)
    data[:2] = b"MZ"
    struct.pack_into("<I", data, 0x3C, 0x80)
    data[0x80:0x84] = b"PE\0\0"
    struct.pack_into("<H", data, 0x84, 0x8664)
    struct.pack_into("<H", data, 0x80 + 24, 0x20B)
    return bytes(data)


def write_zip(path, files, version):
    """package.py's layout: the files, their manifest, regular-file attributes."""
    manifest = {"schema": "jaekit-windows-package/v1", "version": version, "os": "windows", "arch": "amd64",
                "files": [{"path": name, "size": len(body), "sha256": sha256(body)}
                          for name, body in sorted(files.items())]}
    entries = dict(files, **{"package.json": (json.dumps(manifest, sort_keys=True, indent=2) + "\n").encode()})
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for name, data in sorted(entries.items()):
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (stat.S_IFREG | 0o644) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(info, data)


@contextlib.contextmanager
def synthetic(extra=None, version=SYNTHETIC, windows=True, tar_extra=None, archive=None):
    """Four synthetic tar.gz archives, an optional Windows ZIP (`archive`
    replaces it with given bytes) and checksums."""
    fixtures = load("name_limits_fixtures", "tools/release/test_archives.py")
    fixtures.VERSION = version
    fixture = fixtures.ArchiveChecks("test_four_platform_archives_and_uploaded_names")
    fixture.setUp()
    try:
        dist = fixture.dist
        if tar_extra:
            fixture.contents.update(tar_extra)
            for target in fixtures.release.TARGETS:
                fixture.archive(target)
        name = f"ha_{version}_windows_amd64.zip"
        if archive is not None:
            (dist / name).write_bytes(archive)
        elif windows:
            archives = load("name_limits_archives_required", "tools/release/check_archives.py")
            files = {path: BODY for path in archives.WINDOWS_REQUIRED - {"package.json"}}
            files["ha.exe"] = pe_amd64()
            files.update(extra or {})
            write_zip(dist / name, files, version)
        paths = sorted(dist.glob("*.tar.gz")) + sorted(dist.glob("*.zip"))
        (dist / "checksums.txt").write_text("".join(f"{sha256(p.read_bytes())}  {p.name}\n" for p in paths))
        yield dist, version
    finally:
        fixture.doCleanups()


class Response(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()


def routes(dist, version, message):
    """GitHub-shaped responses for one release under an annotated tag."""
    name = f"v{version}"
    table, assets = {}, []
    for path in sorted(dist.iterdir()):
        data = path.read_bytes()
        url = f"https://github.com/{REPO}/releases/download/{name}/{path.name}"
        table[url] = data
        assets.append({"name": path.name, "size": len(data), "digest": "sha256:" + sha256(data),
                       "browser_download_url": url})
    table[f"https://api.github.com/repos/{REPO}/releases/tags/{name}"] = json.dumps(
        {"tag_name": name, "draft": False, "prerelease": False, "assets": assets}).encode()
    tag_sha, commit_sha = "7" * 40, "c" * 40
    ref = json.dumps({"ref": f"refs/tags/{name}", "object": {"type": "tag", "sha": tag_sha}}).encode()
    table[f"https://api.github.com/repos/{REPO}/git/tags/{tag_sha}"] = json.dumps(
        {"tag": name, "sha": tag_sha, "message": message, "object": {"type": "commit", "sha": commit_sha}}).encode()
    for prefix in ("ref", "refs"):
        table[f"https://api.github.com/repos/{REPO}/git/{prefix}/tags/{name}"] = ref
    return table


def published_cli(published, table, version):
    """Run the published-release check's command line against `table`."""
    unrouted = []

    def urlopen(request, timeout=None):
        url = getattr(request, "full_url", request)
        value = table.get(url)
        if value is None:
            unrouted.append(url)
            raise urllib.error.URLError("unrouted " + url)
        return Response(value)

    original = published.urlopen
    published.urlopen = urlopen
    errors = io.StringIO()
    try:
        with contextlib.redirect_stderr(errors), contextlib.redirect_stdout(io.StringIO()):
            code = published.main(["--tag", f"v{version}", "--repo", REPO])
    finally:
        published.urlopen = original
    if unrouted:
        raise ObservationError("the published-release check requested what the fake GitHub does not serve: "
                               + ", ".join(unrouted))
    return code, errors.getvalue()


def judged(archives, published, dist, version):
    """(pre-publication verdict, post-publication verdict) of one synthetic release."""
    try:
        archives.check(dist, version, require_windows=True)
        before = "accept"
    except (ValueError, KeyError, OSError) as error:
        before = "reject: " + str(error)
    digest = sha256((dist / f"ha_{version}_windows_amd64.zip").read_bytes())
    message = f"Jaekit v{version}\n\nWindows-Accepted-SHA256: {digest}\n"
    code, errors = published_cli(published, routes(dist, version, message), version)
    if code == 2:
        raise ObservationError("the published-release check could not read the synthetic release: " + errors[-300:])
    return before, ("accept" if code == 0 else "reject: " + errors.strip()[-300:])


# ------------------------------------------------------------ AC-2

TILDE = ("guides/abcdef~1.md", "guides/notes~/WINDOWS.md", "contracts/archive~.md")


def package_archive(name, version):
    """The ZIP package.py makes from a depth-one clone with `name` committed."""
    with clone() as target, tempfile.TemporaryDirectory(prefix="jaekit-name-limits-package-", dir=SCRATCH) as temp:
        (target / name).parent.mkdir(parents=True, exist_ok=True)
        (target / name).write_bytes(BODY)
        git(target, "add", "--", name)
        git(target, "commit", "--quiet", "--no-verify", "-m", "synthetic public file")
        binary = Path(temp) / "ha.exe"
        binary.write_bytes(pe_amd64())
        package = load("name_limits_package", "tools/windows/package.py", root=target)
        try:
            archive = package.prepare(binary, version, Path(temp) / "candidate", root=target)
        except ValueError as error:
            raise ObservationError(f"package.py could not prepare a candidate with {name}: {error}") from None
        return archive.read_bytes()


def ac2():
    archives = load("name_limits_archives", "tools/release/check_archives.py")
    published = load("name_limits_published", "tools/release/check_published.py")
    with synthetic() as (dist, version):
        if judged(archives, published, dist, version) != ("accept", "accept"):
            raise ObservationError("a normal synthetic Windows archive is not accepted, so a refusal says nothing")
    failures = []
    for name in TILDE:
        with synthetic({name: BODY}) as (dist, version):
            before, after = judged(archives, published, dist, version)
        if not (before.startswith("reject") and after.startswith("reject") and name in before and name in after):
            failures.append(f"a Windows archive with {name} was not refused with its name by both checks: "
                            f"{before[:200]} / {after[:200]}")
    name = TILDE[0]
    data = package_archive(name, SYNTHETIC)
    with synthetic(archive=data) as (dist, version):
        before, after = judged(archives, published, dist, version)
    if not (before.startswith("reject") and after.startswith("reject") and name in before and name in after):
        failures.append(f"a package.py archive with {name} was not refused with its name by both checks: "
                        f"{before[:200]} / {after[:200]}")
    require(not failures, "\n".join(failures))


# ------------------------------------------------------------ remote runs

def latest_run(workflow):
    repo, sha = repository(), head()
    runs = api(f"repos/{repo}/actions/workflows/{workflow}/runs?event=pull_request&head_sha={sha}&per_page=100")
    matches = [item for item in runs.get("workflow_runs", []) if item.get("head_sha") == sha]
    if not matches:
        raise ObservationError(f"no {workflow} run for HEAD's pull request yet")
    latest = max(matches, key=lambda item: (item.get("run_number", 0), item.get("run_attempt", 0)))
    if latest.get("status") != "completed":
        raise ObservationError(f"the {workflow} run for HEAD has not finished")
    return repo, sha, latest


def jobs(workflow):
    repo, _, latest = latest_run(workflow)
    listed = api(f"repos/{repo}/actions/runs/{latest['id']}/attempts/{latest['run_attempt']}/jobs?per_page=100")
    return {job.get("name"): job for job in listed.get("jobs", [])}


def judged_steps(job, label, judged):
    """A failure of a judged step is a violation. A missing, cancelled, skipped
    or timed-out job, or any other failed step (checkout, setup, download), is
    no answer."""
    if not job:
        raise ObservationError(f"{label} is missing from HEAD's run")
    steps = {step.get("name"): step.get("conclusion") for step in job.get("steps", [])}
    failed = [name for name in judged if steps.get(name) == "failure"]
    if failed:
        raise Violation(f"{label}: {', '.join(failed)} failed")
    if job.get("conclusion") != "success" or any(steps.get(name) != "success" for name in judged):
        raise ObservationError(f"{label} did not run to an end ({job.get('conclusion')}): "
                               + json.dumps({name: steps.get(name) for name in judged}))


def contract_report():
    """HEAD's installer comparison report, complete, for both shells and without `error` verdicts."""
    repo, sha, latest = latest_run("release.yml")
    listed = api(f"repos/{repo}/actions/runs/{latest['id']}/artifacts?per_page=100")
    if "release-windows-contract" not in {item.get("name") for item in listed.get("artifacts", [])}:
        raise ObservationError("HEAD's release run kept no installer comparison report")
    with tempfile.TemporaryDirectory(prefix="jaekit-name-limits-contract-", dir=SCRATCH) as temp:
        result = run(["gh", "run", "download", str(latest["id"]), "--repo", repo, "--name", "release-windows-contract",
                      "--dir", temp], timeout=300)
        path = Path(temp) / "windows-contract.json"
        if result.returncode or not path.is_file():
            raise ObservationError("the installer comparison report of HEAD's release run is unavailable")
        report = json.loads(path.read_text(encoding="utf-8"))
    if report.get("schema") != "jaekit-windows-contract/v1" or report.get("source_commit") != sha:
        raise ObservationError("the installer comparison report does not describe HEAD")
    cases = {case.get("name"): case for case in report.get("cases", [])}
    incomplete = [name for name, case in cases.items() if set((case.get("installer") or {})) != set(SHELLS)]
    verdicts = [value for case in cases.values() for value in (case.get("python"), *(case.get("installer") or {}).values())]
    if report.get("complete") is not True or incomplete or "error" in verdicts:
        raise ObservationError("the installer comparison did not judge every case in both shells: "
                               + json.dumps([case for case in cases.values() if "error" in json.dumps(case)]
                                            + incomplete)[:600])
    return report, cases


# ------------------------------------------------------------ AC-3

SHELLS = ("powershell", "pwsh")
NAME_CHARACTERS = {"\0": "NUL", "\x01": "control character", '"': "quote", "<": "less-than sign",
                   ">": "greater-than sign", "|": "vertical bar"}


# A report from before this goal names no stored entry names; its one such case is known.
KNOWN_NAME_CASES = {"a NUL in an entry name": "\0"}


def name_case(case):
    """The character of an installer comparison case whose stored entry name holds it."""
    stored = case.get("stored_name") or KNOWN_NAME_CASES.get(case.get("name"), "")
    return next((char for char in NAME_CHARACTERS if char in stored), None)


def ac3():
    report, cases = contract_report()
    failures = []
    by_character = {}
    for name, case in cases.items():
        char = name_case(case)
        if char is not None:
            by_character.setdefault(char, []).append(name)
    missing = [label for char, label in NAME_CHARACTERS.items() if char not in by_character]
    if missing:
        failures.append("the installer comparison has no case whose entry name holds a " + ", a ".join(missing))
    normal = cases.get("normal") or {}
    if (normal.get("installer_wrote") or {}) != {shell: True for shell in SHELLS}:
        failures.append("the comparison does not show that the installer wrote the normal archive in both shells: "
                        + json.dumps(normal.get("installer_wrote")))
    for char, names in sorted(by_character.items()):
        for name in names:
            case = cases[name]
            for shell in SHELLS:
                verdict = case["installer"][shell]
                reason = (case.get("installer_reasons") or {}).get(shell, "")
                message = reason.split(": ", 1)[1] if ": " in reason else reason
                if verdict != "reject" or not message.startswith(NAME_REFUSAL):
                    failures.append(f"{shell} did not refuse {name} with the installer's name refusal: "
                                    f"{verdict} {reason[:240]}")
                if (case.get("installer_wrote") or {}).get(shell) is not False:
                    failures.append(f"the comparison does not show that {shell} wrote nothing for {name}")
    require(not failures, "\n".join(failures))


# ------------------------------------------------------------ AC-4

AT_START = {"tools/release/check_archives.py": DECLARATIONS / "check_archives.py.at-start",
            "tools/release/windows_contract.py": DECLARATIONS / "windows_contract.py.at-start"}


def selected(check, cwd=ROOT):
    """Run one registry selection the way the documentation group discovers it."""
    path = PurePosixPath(check["file"])
    return unit(["-m", "unittest", "discover", "-s", str(path.parent), "-p", path.name, "-k", check["selector"]],
                cwd=cwd)


@contextlib.contextmanager
def at_start(path):
    """A snapshot of HEAD whose `path` is the file as it was when this goal started."""
    with snapshot() as source:
        shutil.copyfile(AT_START[path], source / path)
        yield source


def ac4():
    failures = []
    readme = text("tools/release-checks/README.md")
    if not ("`~`" in readme and re.search(r"short name|8\.3", readme)):
        failures.append("tools/release-checks/README.md does not state the `~` name limit and its reason")
    if not (re.search(r"committed public files", readme, re.I)
            and re.search(r"common verification|tools/verify\.py", readme)):
        failures.append("tools/release-checks/README.md does not state that common verification judges "
                        "committed public files")
    result = run([sys.executable, "tools/regression_map.py", "--format", "json"], timeout=300, show=False)
    if result.returncode:
        if "regression map invalid" not in result.stdout + result.stderr:
            raise ObservationError("the regression registry could not be validated: " + result.stderr[-300:])
        failures.append("the regression registry is invalid: " + result.stderr.strip()[-300:])
        require(False, "\n".join(failures))
    registry = load("name_limits_registry", "tools/regression_map.py")
    before = json.loads((DECLARATIONS / "registry-at-start.json").read_text(encoding="utf-8"))
    after = json.loads(text("contracts/regressions.json"))
    compared = registry.compare(before, after)
    if compared["lost_protection"] or compared["changed_invariants"]:
        failures.append("the registry changed what it protected at the start: "
                        + json.dumps({key: compared[key] for key in ("lost_protection", "changed_invariants")}))
    started = {route["id"] for route in before["routes"]}
    added = [route for route in after["routes"] if route["id"] not in started and route["status"] == "protected"]
    # Each property has its own new route whose selected tests pass at HEAD and
    # fail against the old behavior: a committed refused file, or the file as
    # it was when this goal started.
    properties = (
        ("committed public files", re.compile(r"committed public files", re.I), None),
        ("`~` names", re.compile(r"`~`"), "tools/release/check_archives.py"),
        ("installer name refusals", re.compile(r"PowerShell 5\.1"), "tools/release/windows_contract.py"),
    )
    owners = {}
    for label, pattern, _ in properties:
        found = [route for route in added if pattern.search(route["property"]) and route["id"] not in owners.values()]
        if not found:
            failures.append(f"no new protected registry route of its own states {label}")
        else:
            owners[label] = found[0]["id"]
    require(not failures, "\n".join(failures))
    routes_by_id = {route["id"]: route for route in after["routes"]}
    for label, _, old in properties:
        checks = [check for check in routes_by_id[owners[label]]["checks"] if check["file"].endswith(".py")]
        if not checks:
            failures.append(f"the route for {label} selects no Python test")
            continue
        for check in checks:
            code, output = selected(check)
            if code:
                failures.append(f"the selected test {check['selector']} for {label} fails at HEAD: " + output[-300:])
        if old is None:
            with clone() as target:
                commit(target, {"guides/Node_Modules/notes.md": BODY})
                caught = [check for check in checks if selected(check, cwd=target)[0]]
        else:
            with at_start(old) as source:
                caught = [check for check in checks if selected(check, cwd=source)[0]]
        if not caught:
            failures.append(f"no selected test for {label} fails against the old behavior")
    installer = routes_by_id.get(owners["installer name refusals"], {})
    if "tools/windows/common.ps1" not in installer.get("sources", []):
        failures.append("the installer name refusal route does not name the installer as a source")
    require(not failures, "\n".join(failures))


# ------------------------------------------------------------ maintained behavior

def ac5():
    code, output = release_tests(ROOT)
    require(code == 0, "HEAD's committed public files fail the release archive regressions: " + output[-600:])
    judged_steps(jobs("release.yml").get("Build release files"), "the release build",
                 ("Verify archives and native smoke",))
    judged_steps(jobs("ci.yml").get("Public documentation"), "the common verification in CI",
                 ("Verify public documentation",))


def ac6():
    archives = load("name_limits_tar_rules", "tools/release/check_archives.py")
    failures = []
    for extra, accepted in (({}, True), ({"guides/Node_Modules/notes.md": BODY}, True),
                            ({"guides/tool.PYC": BODY}, True), ({"guides/abcdef~1.md": BODY}, True),
                            ({"guides/node_modules/notes.md": BODY}, False), ({"guides/tool.pyc": BODY}, False)):
        with synthetic(windows=False, tar_extra=extra) as (dist, version):
            try:
                archives.check(dist, version)
                verdict, reason = True, ""
            except ValueError as error:
                verdict, reason = False, str(error)
        if verdict != accepted or (not accepted and "local metadata or generated cache" not in reason):
            failures.append(f"the tar.gz rule changed its verdict for {sorted(extra) or 'a normal release'}: {reason}")
    code, output = unit(["-m", "unittest", "discover", "-s", "tools/release", "-p", "test_archives.py"])
    if code:
        failures.append("the tar.gz archive regressions failed: " + output[-300:])
    require(not failures, "\n".join(failures))


ORIGINAL_CASES = {"normal": "accept", "an oversized manifest": "reject", "a boolean size": "reject",
                  "a DOS directory attribute": "reject", "a DOS reparse-point attribute": "reject",
                  "a NUL in an entry name": "reject", "a backslash in an entry name": "reject"}


def ac7():
    judged_steps(jobs("release.yml").get("Windows release archive"), "the release Windows job",
                 ("Check the archive and public blocks on Windows PowerShell 5.1",
                  "Check the archive and public blocks on PowerShell 7"))
    report, cases = contract_report()
    failures = []
    for name, expected in ORIGINAL_CASES.items():
        case = cases.get(name)
        if case is None:
            failures.append(f"the installer comparison lost its case {name}")
        elif case.get("python") != expected or case.get("installer") != {shell: expected for shell in SHELLS}:
            failures.append(f"the verdicts for {name} changed: {case.get('python')} {case.get('installer')}")
    if report.get("passed") is not True:
        failures.append("the installer comparison report did not pass")
    try:
        judged_steps(jobs("ci.yml").get("Core / Windows"), "HEAD's native Windows checks",
                     ("Verify native developer checks on PowerShell 5.1", "Verify native developer checks on PowerShell 7"))
    except Violation as error:
        failures.append(str(error))
    code, output = unit(["-m", "unittest", "discover", "-s", "tools/windows", "-p", "test_*.py"])
    if code:
        failures.append("the Windows packaging and public block regressions failed: " + output[-300:])
    require(not failures, "\n".join(failures))


def ac8():
    code, output = unit(["-m", "unittest", "discover", "-s", "tools/release", "-p", "test_windows_release.py"])
    require(code == 0, "the publication gate and published-release regressions failed: " + output[-300:])
    for tag, expected, reason in (("v0.1.4", 0, ""), ("v0.1.3", 1, "archive is missing required files")):
        result = run([sys.executable, "tools/release/check_published.py", "--tag", tag], timeout=600, show=False)
        if result.returncode == 2:
            raise ObservationError(f"the public {tag} release could not be read")
        require(result.returncode == expected and reason in result.stdout + result.stderr,
                f"the published-release check changed its verdict for {tag}: exit {result.returncode}: "
                + (result.stdout + result.stderr)[-300:])


CONDITIONS = {"AC-1": ac1, "AC-2": ac2, "AC-3": ac3, "AC-4": ac4, "AC-5": ac5, "AC-6": ac6, "AC-7": ac7, "AC-8": ac8}


def main():
    criterion = sys.argv[1] if len(sys.argv) == 2 else ""
    declaration = DECLARATIONS / f"{criterion}.json"
    structured = declaration.is_file()
    violation = ""
    if structured:
        violation = json.loads(declaration.read_text(encoding="utf-8"))["targets"][0]["violation"]
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    code = 0
    try:
        if criterion not in CONDITIONS:
            raise ObservationError("unknown criterion")
        CONDITIONS[criterion]()
    except Violation as error:
        observation.update(status="violation", violation=violation)
        print(str(error), file=sys.stderr)
        code = 1
    except Exception as error:  # an observation that could not finish is an error, not a verdict
        observation.update(status="error", reason="execution_error")
        print(f"{type(error).__name__}: {error}", file=sys.stderr)
        code = 2
    if structured and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "windows-archive-name-limits", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
