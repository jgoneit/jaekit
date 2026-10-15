#!/usr/bin/env python3
"""Observe the review fixes of the Windows release checks.

Each change condition observes its own behavior: published-tag link checks
through GitHub-shaped responses, one Windows archive rule across the pre- and
post-publication checks and the installer, the v0.1.3 acceptance checker's
asset shape and import boundary, history independence of the earlier
producer, the recurrence guards, guidance and the regression registry. On the
base commit the old behavior is the declared violation. Missing GitHub access,
an unfinished CI run or an unreadable input is an observation error, never a
violation. Temporary copies live outside the checkout; nothing is written into
it.
"""
from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
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

ROOT = Path(__file__).resolve().parents[2]
DECLARATIONS = ROOT / "tools/verification-checks/windows-release-review-fixes"
REPO = "jgoneit/jaekit"
EARLIER = "tools/verification-checks/windows_release_install.py"
SCRATCH = Path(tempfile.gettempdir()).resolve()
# Not a public tag: a request that escaped the fake GitHub could not reach a real one.
SYNTHETIC = "9.8.7"
# Registry routes protected when this goal started; a fix must not demote them.
PROTECTED_ROUTES = (
    "archive-consumer-input-boundary", "current-file-independence", "explicit-retention-inputs",
    "gap-producer-meaning", "goal-producer-meaning", "native-request-and-uncertainty",
    "observation-identity-gap", "parent-result-adapter-meaning", "post-completion-recorded-facts",
    "public-scanner-existing-scope", "public-scanner-path-gap", "reference-producer-to-cli",
    "registry-selection-integrity", "review-producer-meaning", "seal-evidence-and-assurance-contract",
    "shared-command-failures", "shared-tool-preflight", "shell-reachability-gap",
    "spec-observation-boundary-contract", "stored-proof-independence", "stored-result-judgment",
    "structured-classification",
)


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
    """Children must not write into this producer's evidence file."""
    env = {key: value for key, value in os.environ.items() if not key.startswith("HA_EVIDENCE_")}
    env.update(extra)
    return env


def run(argv, timeout=600, env=None, cwd=ROOT):
    try:
        result = subprocess.run(argv, cwd=cwd, env=env or child_env(), capture_output=True, text=True,
                                encoding="utf-8", errors="replace", timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ObservationError(f"{type(error).__name__}: {' '.join(map(str, argv[:4]))}") from None
    if result.returncode:
        print(result.stdout[-3000:], end="")
        print(result.stderr[-3000:], end="", file=sys.stderr)
    return result


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


# ------------------------------------------------------------ synthetic release

def pe_amd64(optional=0x20B):
    data = bytearray(512)
    data[:2] = b"MZ"
    struct.pack_into("<I", data, 0x3C, 0x80)
    data[0x80:0x84] = b"PE\0\0"
    struct.pack_into("<H", data, 0x84, 0x8664)
    struct.pack_into("<H", data, 0x80 + 24, optional)
    return bytes(data)


def windows_files():
    archives = load("review_fix_archives_names", "tools/release/check_archives.py")
    files = {name: b"Synthetic public document.\n" for name in archives.WINDOWS_REQUIRED - {"package.json"}}
    files["ha.exe"] = pe_amd64()
    return files


def manifest_for(files, version):
    return {"schema": "jaekit-windows-package/v1", "version": version, "os": "windows", "arch": "amd64",
            "files": [{"path": name, "size": len(body), "sha256": sha256(body)} for name, body in sorted(files.items())]}


def write_zip(path, files, manifest, attributes=None, flags=None, written=None):
    """The package.py layout. `attributes` overrides an entry's external
    attributes; `flags` sets general-purpose bits in both of its headers after
    writing (ZipFile clears them); `written` stores an entry under another raw
    name of the same length, such as one with a NUL that ZIP readers cut."""
    attributes, flags, written = attributes or {}, flags or {}, written or {}
    body = manifest if isinstance(manifest, bytes) else (json.dumps(manifest, sort_keys=True, indent=2) + "\n").encode()
    entries = dict(files, **{"package.json": body})
    placeholders = {name: raw.replace("\0", "!") for name, raw in written.items()}
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for name, data in sorted(entries.items()):
            info = zipfile.ZipInfo(placeholders.get(name, name), date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = attributes.get(name, (stat.S_IFREG | 0o644) << 16)
            info.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(info, data)
    data = bytearray(output.getvalue())
    end = data.rfind(b"PK\x05\x06")
    count, _, offset = struct.unpack_from("<HII", data, end + 10)
    for _ in range(count):
        length, extra, comment = struct.unpack_from("<HHH", data, offset + 28)
        name = bytes(data[offset + 46:offset + 46 + length]).decode("utf-8")
        local = struct.unpack_from("<I", data, offset + 42)[0]
        for header in (offset + 8, local + 6):
            struct.pack_into("<H", data, header, struct.unpack_from("<H", data, header)[0] | flags.get(name, 0))
        offset += 46 + length + extra + comment
    for name, raw in written.items():
        placeholder = placeholders[name].encode("utf-8")
        if len(placeholder) != len(raw.encode("utf-8")) or data.count(placeholder) != 2:
            raise ObservationError("a synthetic raw entry name could not be written")
        data = bytearray(bytes(data).replace(placeholder, raw.encode("utf-8")))
    path.write_bytes(bytes(data))
    with zipfile.ZipFile(path) as archive:
        stored = {info.orig_filename: info for info in archive.infolist()}
    if any(stored[written.get(name, name)].flag_bits & bits != bits for name, bits in flags.items()) \
            or not set(written.values()) <= stored.keys():
        raise ObservationError("the synthetic ZIP does not carry its tampered fields")


@contextlib.contextmanager
def synthetic(windows=True, mutate=None, version=None):
    """Four synthetic tar.gz archives, an optional Windows ZIP and checksums."""
    fixtures = load("review_fix_fixtures", "tools/release/test_archives.py")
    if version:
        fixtures.VERSION = version
    fixture = fixtures.ArchiveChecks("test_four_platform_archives_and_uploaded_names")
    fixture.setUp()
    try:
        dist, version = fixture.dist, fixtures.VERSION
        if windows:
            files = windows_files()
            manifest = manifest_for(files, version)
            options = {}
            if mutate:
                files, manifest, options = mutate(files, manifest)
            write_zip(dist / f"ha_{version}_windows_amd64.zip", files, manifest, **options)
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


def routes(dist, version, message=None, tag="lightweight"):
    """GitHub-shaped responses for one release; `tag` is lightweight, annotated,
    unreachable or malformed."""
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
    if tag == "unreachable":
        ref = urllib.error.URLError("tag service unreachable")
    elif tag == "malformed":
        ref = b"{not json"
    elif tag == "annotated":
        ref = json.dumps({"ref": f"refs/tags/{name}", "object": {"type": "tag", "sha": tag_sha}}).encode()
        table[f"https://api.github.com/repos/{REPO}/git/tags/{tag_sha}"] = json.dumps(
            {"tag": name, "sha": tag_sha, "message": message, "object": {"type": "commit", "sha": commit_sha}}).encode()
    else:
        ref = json.dumps({"ref": f"refs/tags/{name}", "object": {"type": "commit", "sha": commit_sha}}).encode()
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
        if isinstance(value, BaseException):
            raise value
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


# ------------------------------------------------------------ AC-1

def ac1():
    published = load("review_fix_published", "tools/release/check_published.py")
    gate = load("review_fix_gate", "tools/release/windows_gate.py")
    with synthetic(version=SYNTHETIC) as (dist, version):
        digest = sha256((dist / f"ha_{version}_windows_amd64.zip").read_bytes())
        line = "Windows-Accepted-SHA256: " + digest
        opening = f"Jaekit v{version}\n\n"
        # None: the expected verdict is the gate's own reading of the message.
        cases = (
            ("a lightweight tag", None, "lightweight", 1),
            ("no acceptance line", f"Jaekit v{version}\n", "annotated", 1),
            ("two acceptance lines", f"{opening}{line}\n{line}\n", "annotated", 1),
            ("an uppercase digest", f"{opening}Windows-Accepted-SHA256: {digest.upper()}\n", "annotated", 1),
            ("another digest", f"{opening}Windows-Accepted-SHA256: {'0' * 64}\n", "annotated", 1),
            ("its acceptance link", f"{opening}{line}\n", "annotated", 0),
            ("its acceptance link with CRLF line ends", f"Jaekit v{version}\r\n\r\n{line}\r\n", "annotated", None),
            ("its acceptance link without a space", f"{opening}Windows-Accepted-SHA256:{digest}\n", "annotated", None),
            ("an indented acceptance line", f"{opening} {line}\n", "annotated", None),
            ("text before the acceptance key", f"{opening}See {line}\n", "annotated", None),
            ("an unreachable tag", None, "unreachable", 2),
            ("a malformed tag response", None, "malformed", 2),
        )
        for label, message, tag, expected in cases:
            if expected is None:
                expected = 0 if gate.accepted(message)[0] == digest else 1
            code, errors = published_cli(published, routes(dist, version, message, tag), version)
            if code == 2 and expected != 2:
                raise ObservationError(f"the published-release check could not read the synthetic release ({label}): "
                                       + errors.strip()[-300:])
            require(code == expected, f"the published-release check exited {code} for a Windows archive with {label}; "
                                      f"expected {expected}: {errors.strip()[-300:]}")
            if message is not None:
                link, _ = gate.accepted(message)
                require((link == digest) == (expected == 0),
                        f"the gate and the published-release check disagree on {label}")
    with synthetic(windows=False, version=SYNTHETIC) as (dist, version):
        for tag in ("lightweight", "unreachable"):
            code, errors = published_cli(published, routes(dist, version, None, tag), version)
            require(code == 0, f"a release without the Windows archive depended on its tag ({tag}): {errors.strip()[-300:]}")
    reader = getattr(published, "read_tag", None)
    require(callable(reader), "the published-release check has no tag reader")
    try:
        annotated, message = reader(REPO, "v0.1.4")
    except OSError as error:
        raise ObservationError(f"the public v0.1.4 tag could not be read: {error}") from None
    require(annotated is True and gate.accepted(message)[0] is None,
            "the public v0.1.4 tag was not read as an annotated tag without an acceptance link")


# ------------------------------------------------------------ AC-2

def mutation(kind):
    def apply(files, manifest):
        files, options = dict(files), {}
        version = manifest["version"]
        if kind == "only the Core and its manifest":
            files = {"ha.exe": files["ha.exe"]}
        elif kind == "another operating system":
            return files, dict(manifest_for(files, version), os="linux"), options
        elif kind == "another architecture":
            return files, dict(manifest_for(files, version), arch="arm64"), options
        elif kind == "another schema":
            return files, dict(manifest_for(files, version), schema="jaekit-windows-package/v2"), options
        elif kind == "a repeated manifest entry":
            built = manifest_for(files, version)
            built["files"].append(dict(built["files"][0]))
            return files, built, options
        elif kind == "a malformed manifest entry":
            built = manifest_for(files, version)
            built["files"][0]["sha256"] = built["files"][0]["sha256"].upper()
            return files, built, options
        elif kind == "a boolean size":
            files["LICENSE"] = b"x"
            built = manifest_for(files, version)
            for item in built["files"]:
                if item["path"] == "LICENSE":
                    item["size"] = True
            return files, built, options
        elif kind == "a fractional size":
            files["LICENSE"] = b"x"
            built = manifest_for(files, version)
            for item in built["files"]:
                if item["path"] == "LICENSE":
                    item["size"] = 1.0
            return files, built, options
        elif kind == "an oversized manifest":
            body = json.dumps(manifest_for(files, version), sort_keys=True, indent=2).encode()
            return files, body + b" " * (1024 * 1024) + b"\n", options
        elif kind == "a reserved name":
            files["guides/CON.md"] = b"x\n"
        elif kind == "names differing only in case":
            files["guides/windows.md"] = b"x\n"
        elif kind == "a link entry":
            options["attributes"] = {"LICENSE": (stat.S_IFLNK | 0o777) << 16}
        elif kind == "an encrypted entry":
            options["flags"] = {"LICENSE": 0x1}
        elif kind == "a DOS directory attribute":
            options["attributes"] = {"LICENSE": ((stat.S_IFREG | 0o644) << 16) | 0x10}
        elif kind == "a DOS reparse-point attribute":
            options["attributes"] = {"LICENSE": ((stat.S_IFREG | 0o644) << 16) | 0x400}
        elif kind == "a NUL in an entry name":
            options["written"] = {"LICENSE": "LICENSE\0x"}
        elif kind == "a PE32 Core":
            files["ha.exe"] = pe_amd64(optional=0x10B)
        elif kind == "a path outside the package scope":
            files["docs/specs/goal/SPEC.md"] = b"private goal\n"
        elif kind == "a cache file":
            files["assets/__pycache__/build.cpython-312.pyc"] = b"\0cache"
        else:
            raise ValueError(kind)
        return files, manifest_for(files, version), options
    return apply


TAMPERED = ("only the Core and its manifest", "another operating system", "another architecture", "another schema",
            "a repeated manifest entry", "a malformed manifest entry", "a boolean size", "a fractional size",
            "an oversized manifest",
            "a reserved name", "names differing only in case", "a link entry", "an encrypted entry",
            "a DOS directory attribute", "a DOS reparse-point attribute", "a NUL in an entry name", "a PE32 Core",
            "a path outside the package scope", "a cache file")


def judged(archives, published, dist, version):
    try:
        archives.check(dist, version, require_windows=True)
        before = "accept"
    except (ValueError, KeyError, OSError) as error:
        before = "reject: " + str(error)
    digest = sha256((dist / f"ha_{version}_windows_amd64.zip").read_bytes())
    message = f"Jaekit v{version}\n\nWindows-Accepted-SHA256: {digest}\n"
    code, errors = published_cli(published, routes(dist, version, message, "annotated"), version)
    if code == 2:
        raise ObservationError("the published-release check could not read the synthetic release: " + errors[-300:])
    return before, ("accept" if code == 0 else "reject: " + errors.strip()[-200:])


def release_run(root=ROOT):
    repo, sha = repository(root), head(root)
    runs = api(f"repos/{repo}/actions/workflows/release.yml/runs?event=pull_request&head_sha={sha}&per_page=100")
    matches = [item for item in runs.get("workflow_runs", []) if item.get("head_sha") == sha]
    if not matches:
        raise ObservationError("no release workflow run for HEAD's pull request yet")
    latest = max(matches, key=lambda item: (item.get("run_number", 0), item.get("run_attempt", 0)))
    if latest.get("status") != "completed":
        raise ObservationError("the release workflow run for HEAD has not finished")
    return repo, sha, latest


def artifact(name, directory):
    repo, sha, latest = release_run()
    result = run(["gh", "run", "download", str(latest["id"]), "--repo", repo, "--name", name, "--dir", str(directory)],
                 timeout=300)
    if result.returncode:
        raise ObservationError(f"the {name} artifact of HEAD's release run is unavailable")
    return sha, latest


def ac2():
    archives = load("review_fix_archives", "tools/release/check_archives.py")
    published = load("review_fix_published_rules", "tools/release/check_published.py")
    with synthetic(version=SYNTHETIC) as (dist, version):
        before, after = judged(archives, published, dist, version)
        require(before == after == "accept", f"a normal Windows archive was not accepted: {before} / {after}")
    for kind in TAMPERED:
        with synthetic(mutate=mutation(kind), version=SYNTHETIC) as (dist, version):
            before, after = judged(archives, published, dist, version)
            require(before.startswith("reject") and after.startswith("reject"),
                    f"a Windows archive with {kind} was not refused by both checks: {before} / {after}")
    package_archive(archives)
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-contract-", dir=SCRATCH) as temp:
        repo, sha, latest = release_run()
        listed = api(f"repos/{repo}/actions/runs/{latest['id']}/artifacts?per_page=100")
        require("release-windows-contract" in {item.get("name") for item in listed.get("artifacts", [])},
                "HEAD's release run did not compare the archive rule with the installer")
        artifact("release-windows-contract", Path(temp))
        path = Path(temp) / "windows-contract.json"
        require(path.is_file(), "HEAD's release run left no Windows contract report")
        report = json.loads(path.read_text(encoding="utf-8"))
    require(report.get("schema") == "jaekit-windows-contract/v1" and report.get("source_commit") == sha,
            "the Windows contract report does not describe HEAD")
    cases = {case.get("name"): case for case in report.get("cases", [])}
    for name in ("normal", "an oversized manifest", "a boolean size", "a DOS directory attribute",
                 "a DOS reparse-point attribute"):
        case = cases.get(name)
        require(case is not None, f"the Windows contract report has no case for {name}")
        expected = "accept" if name == "normal" else "reject"
        require(case.get("python") == expected and case.get("installer") == {"powershell": expected, "pwsh": expected},
                f"the checker and the installer disagree on {name}: {case}")
    require(report.get("passed") is True, "the Windows contract report did not pass")


def package_archive(archives):
    """A ZIP that package.py makes from HEAD's public files passes the rule."""
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-package-", dir=SCRATCH) as temp:
        binary = Path(temp) / "ha.exe"
        build = run(["go", "build", "-buildvcs=false", "-o", str(binary), "./cmd/ha"], timeout=600,
                    env=child_env(GOOS="windows", GOARCH="amd64", CGO_ENABLED="0"))
        if build.returncode:
            raise ObservationError("the Windows Core could not be built for the package check")
        package = load("review_fix_package", "tools/windows/package.py")
        output = Path(temp) / "candidate"
        try:
            package.prepare(binary, "0.0.0", output)
        except ValueError as error:
            raise ObservationError(f"package.py could not prepare a candidate: {error}") from None
        archive = output / "ha_0.0.0_windows_amd64.zip"
        try:
            archives.windows_files(archive, sha256(archive.read_bytes()), "0.0.0")
        except ValueError as error:
            raise Violation(f"the pre-publication rule refused a package.py archive: {error}") from None


# ------------------------------------------------------------ AC-3 and AC-4

SHAPES = r"""
import importlib.util, json, sys
path, shadow = sys.argv[1], sys.argv[2]
shadows = ("zipfile", "tarfile", "gzip", "bz2", "lzma", "csv")
result = {"preloaded": sorted(name for name in shadows if name in sys.modules)}
if shadow:
    sys.path.insert(0, shadow)
spec = importlib.util.spec_from_file_location("embedded_acceptance_check", path)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)
four = [f"ha_{check.VERSION}_{p}.tar.gz" for p in check.PLATFORMS] + ["checksums.txt"]
windows = f"ha_{check.VERSION}_windows_amd64.zip"
result.update(tag=check.TAG, windows_rule=hasattr(check, "windows_archive") or hasattr(check, "WINDOWS_ARCHIVE"))
def outcome(call):
    try:
        call()
        return "accept"
    except check.Failure as error:
        return "reject: " + str(error)
    except Exception as error:
        return f"error: {type(error).__name__}: {error}"
def shape(names):
    checker = check.Checker()
    checker._release = {"tag_name": check.TAG, "draft": False, "prerelease": False,
                        "assets": [{"name": name} for name in names]}
    return outcome(checker.assets)
for label, names in (("four", four), ("windows", four + [windows]), ("extra", four + ["install.ps1"])):
    result[label] = shape(names)
if len(sys.argv) > 3:
    for label, directory in (("four", sys.argv[3]), ("windows", sys.argv[4])):
        names = four + ([windows] if label == "windows" else [])
        data = {name: open(directory + "/" + name, "rb").read() for name in names}
        checker = check.Checker()
        checker._release = {"tag_name": check.TAG, "draft": False, "prerelease": False,
                            "assets": [{"name": name} for name in names]}
        checker.download = lambda name, data=data: data[name]
        steps = [("assets", checker.assets), ("archives", checker.archives)]
        if label == "windows" and hasattr(check, "windows_archive"):
            steps.append(("windows_archive", lambda: check.windows_archive(data[windows])))
        result[label + "_steps"] = {step: outcome(call) for step, call in steps}
    module = sys.modules.get("zipfile")
    result["zipfile"] = getattr(module, "__file__", None)
print(json.dumps(result))
"""


def acceptance_shapes(shadow="", data=()):
    argv = [sys.executable, "-c", SHAPES, str(ROOT / "tools/release-checks/check.py"), shadow, *data]
    result = run(argv, timeout=300, cwd=shadow or ROOT)
    if result.returncode:
        raise ObservationError("the acceptance checker could not be loaded for observation")
    shapes = json.loads(result.stdout.strip().splitlines()[-1])
    if shapes["preloaded"]:
        raise ObservationError("the interpreter loaded " + ", ".join(shapes["preloaded"])
                               + " before the acceptance checker; caller modules cannot be observed")
    return shapes


def ac3():
    shapes = acceptance_shapes()
    require(shapes["tag"] == "v0.1.3", "the acceptance checker is no longer pinned to v0.1.3")
    require(shapes["four"] == "accept", "the acceptance checker refused the real v0.1.3 shape: " + shapes["four"])
    require(shapes["windows"].startswith("reject: ") and "archive" in shapes["windows"],
            "the v0.1.3 acceptance checker accepted a Windows archive: " + shapes["windows"])
    require(shapes["extra"].startswith("reject: "), "the v0.1.3 acceptance checker accepted another asset")
    require(not shapes["windows_rule"], "the v0.1.3 acceptance checker keeps a separate Windows archive rule")


SHADOWS = ("zipfile", "tarfile", "gzip", "bz2", "lzma", "csv")


def ac4():
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-shadow-", dir=SCRATCH) as temp:
        shadow, marker = Path(temp) / "caller", Path(temp) / "shadow-executed"
        shadow.mkdir()
        for name in SHADOWS:
            (shadow / f"{name}.py").write_text(
                f"open({str(marker)!r}, 'a').write({name!r} + '\\n')\nraise ImportError('shadow {name}')\n")
        directories = []
        for label, windows in (("four", False), ("windows", True)):
            target = Path(temp) / label
            target.mkdir()
            with synthetic(windows=windows, version="0.1.3") as (dist, version):
                for path in dist.iterdir():
                    shutil.copyfile(path, target / path.name)
            directories.append(str(target))
        shadowed = acceptance_shapes(str(shadow), directories)
        executed = marker.read_text().split() if marker.exists() else []
        loaded = str(shadowed.get("zipfile") or "")
        control = acceptance_shapes("", directories)
    require(not executed, "embedding the acceptance checker executed caller modules: " + ", ".join(executed))
    require(not loaded.startswith(temp), "the acceptance checker loaded zipfile from the caller's import path")
    for label in ("four", "windows"):
        steps = control[label + "_steps"]
        if any(value.startswith("error: ") for value in steps.values()):
            raise ObservationError(f"the acceptance checker could not judge the synthetic {label} release: {steps}")
    require(control["four_steps"] == {"assets": "accept", "archives": "accept"},
            "the acceptance checker refused synthetic v0.1.3 archives: " + json.dumps(control["four_steps"]))
    for label in ("four", "windows"):
        require(shadowed[label + "_steps"] == control[label + "_steps"],
                f"caller modules changed the acceptance checker's judgment of the {label} release")


# ------------------------------------------------------------ AC-5

EARLIER_RUN = r"""
import runpy, sys
script = sys.argv[1]
sys.argv = sys.argv[1:]
sys.path[0] = script.rsplit("/", 1)[0]
try:
    runpy.run_path(script, run_name="__main__")
except SystemExit:
    raise
except BaseException as error:
    print(f"earlier producer crashed: {type(error).__name__}: {error}", file=sys.stderr)
    sys.exit(3)
"""


def earlier(criterion, root):
    """Run the earlier producer for one criterion; return its exit code.

    An uncaught exception would also exit 1, so it is told apart from a
    reported violation and treated as an observation error.
    """
    result = run([sys.executable, "-c", EARLIER_RUN, str(root / EARLIER), criterion], timeout=600, cwd=root)
    output = (result.stdout + result.stderr).strip()[-400:]
    if result.returncode not in (0, 1, 2):
        raise ObservationError(f"the earlier producer did not finish {criterion}: {output}")
    return result.returncode, output


@contextlib.contextmanager
def shallow_clone():
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-clone-", dir=SCRATCH) as temp:
        clone = Path(temp) / "clone"
        result = run(["git", "clone", "--quiet", "--depth", "1", "--no-tags", ROOT.as_uri(), str(clone)], timeout=300)
        if result.returncode:
            raise ObservationError("a depth-one clone of HEAD could not be made")
        origin = run(["git", "remote", "get-url", "origin"]).stdout.strip()
        if origin:
            run(["git", "remote", "set-url", "origin", origin], cwd=clone)
        if run(["git", "rev-parse", "--is-shallow-repository"], cwd=clone).stdout.strip() != "true":
            raise ObservationError("the clone of HEAD is not shallow")
        yield clone


@contextlib.contextmanager
def snapshot():
    """HEAD's tracked files without a Git directory."""
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-snapshot-", dir=SCRATCH) as temp:
        target = Path(temp) / "source"
        target.mkdir()
        archive = subprocess.run(["git", "archive", "--format=tar", "HEAD"], cwd=ROOT, capture_output=True,
                                 env=child_env(), timeout=300)
        if archive.returncode:
            raise ObservationError("HEAD's files could not be archived")
        with tarfile.open(fileobj=io.BytesIO(archive.stdout)) as source:
            source.extractall(target, filter="data")
        yield target


EARLIER_CRITERIA = tuple(f"AC-{number}" for number in range(1, 16))


def ac5():
    full = {criterion: earlier(criterion, ROOT) for criterion in EARLIER_CRITERIA}
    with shallow_clone() as clone:
        shallow = {criterion: earlier(criterion, clone) for criterion in EARLIER_CRITERIA}
    with snapshot() as source:
        detached = {criterion: earlier(criterion, source) for criterion in EARLIER_CRITERIA}
    for criterion in EARLIER_CRITERIA:
        require(shallow[criterion][0] != 1 or full[criterion][0] == 1,
                f"the earlier producer reports a violation for {criterion} in a depth-one clone: {shallow[criterion][1]}")
    for criterion in EARLIER_CRITERIA:
        require(detached[criterion][0] != 1 or full[criterion][0] == 1,
                f"the earlier producer reports a violation for {criterion} without a Git directory: "
                + detached[criterion][1])
    # README, INSTALL and the Skill are judged from file contents alone; the
    # other conditions may need HEAD's identity or its CI and then stop with
    # an observation error instead.
    require(detached["AC-15"][0] == full["AC-15"][0],
            "AC-15 differs between a checkout and a source snapshot: " + detached["AC-15"][1])
    for criterion in EARLIER_CRITERIA:
        if 2 in (full[criterion][0], shallow[criterion][0]):
            raise ObservationError(f"the earlier producer could not observe {criterion}: "
                                   + (full[criterion][1] if full[criterion][0] == 2 else shallow[criterion][1]))
        require(shallow[criterion][0] == full[criterion][0], f"{criterion} differs between full and depth-one clones")
    require("7476c87" not in text(EARLIER), "the earlier producer still names its goal-start commit")


# ------------------------------------------------------------ AC-6

HISTORY_FIXTURE = '''import subprocess
BASE = "7476c87"
def changed():
    return subprocess.run(["git", "diff", "--quiet", BASE, "HEAD"])
def shown():
    return subprocess.run(["git", "show", "0123abcd4567:README.md"])
'''
HISTORY_NORMAL = '''import subprocess
DIGEST = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
OID = "0123456789abcdef0123456789abcdef01234567"
def current():
    print(OID, DIGEST)
    return subprocess.run(["git", "rev-parse", "HEAD"])
'''
IMPORT_FIXTURE = '''import sys
_CALLER_IMPORT_PATH = sys.path[:]
_STDLIB_IMPORT_PATH = (sys.prefix,)
sys.path[:] = _STDLIB_IMPORT_PATH
import json
from contextlib import contextmanager
@contextmanager
def trusted_imports():
    yield
sys.path[:] = _CALLER_IMPORT_PATH
def late():
    import zipfile
    return zipfile
def cached():
    import json
    return json
def guarded():
    with trusted_imports():
        import csv
    return csv
import gzip
import json.decoder
'''


def ac6():
    guard = ROOT / "tools/check_repository_checks.py"
    require(guard.is_file(), "no repository check rejects caller imports or fixed history commits")
    current = run([sys.executable, str(guard)], timeout=300)
    require(current.returncode == 0, "the current repository fails its own check boundaries: " + current.stdout[-500:])
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-guard-", dir=SCRATCH) as temp:
        root = Path(temp)
        for relative, body in (("tools/verification-checks/history_fixture.py", HISTORY_FIXTURE),
                               ("tools/verification-checks/history_normal.py", HISTORY_NORMAL),
                               ("tools/release-checks/bootstrap_fixture.py", IMPORT_FIXTURE)):
            (root / relative).parent.mkdir(parents=True, exist_ok=True)
            (root / relative).write_text(body, encoding="utf-8")
        result = run([sys.executable, str(guard), "--root", str(root)], timeout=120)
        output = result.stdout + result.stderr
    require(result.returncode == 1, "the repository check did not fail on synthetic violations")
    for location in ("tools/verification-checks/history_fixture.py:4", "tools/verification-checks/history_fixture.py:6",
                     "tools/release-checks/bootstrap_fixture.py:12", "tools/release-checks/bootstrap_fixture.py:21"):
        require(f"{location}:" in output, f"the repository check did not report {location}")
    for location in ("history_normal.py", "bootstrap_fixture.py:15:", "bootstrap_fixture.py:19:",
                     "bootstrap_fixture.py:22:"):
        require(location not in output, f"the repository check reported a normal case: {location}")
    # The docs group discovers test_*.py under tools/public-checks; the
    # regression must exist there and scan the actual checkout.
    regression = ROOT / "tools/public-checks/test_repository_checks.py"
    require('("Public checker regressions", "public-checks")' in text("tools/verify.py") and regression.is_file()
            and "def test_the_repository_passes" in text("tools/public-checks/test_repository_checks.py"),
            "the common verification does not run the repository check against this checkout")
    result = run([sys.executable, "-m", "unittest", "discover", "-s", "tools/public-checks", "-p",
                  "test_repository_checks.py"], timeout=600)
    require(result.returncode == 0, "the repository check regressions failed")


# ------------------------------------------------------------ AC-7

def ac7():
    release_checks = " ".join(text("tools/release-checks/README.md").split(" "))
    windows = text("tools/windows/README.md")
    flat = " ".join(release_checks.split())
    require("Both shapes are accepted here" not in flat,
            "the acceptance checker guide still says it accepts the Windows archive")
    section = release_checks.split("## Windows archive", 1)[-1].split("\n## ", 1)[0]
    require("check_published.py" in section and "Windows-Accepted-SHA256" in section and "v0.1.3" in section,
            "the acceptance checker guide does not say which check covers which release and the tag link")
    limits = release_checks.split("## Archive resource limits", 1)[-1].split("\n## ", 1)[0]
    require("Every archive consumer reads through the same read-only parser" not in " ".join(limits.split()),
            "the guide still says every archive consumer uses the tar parser")
    require("check_published.py" in windows and "Windows-Accepted-SHA256" in windows,
            "the Windows guide does not say the published-release check reads the acceptance link")
    for guide in ("guides/WINDOWS.md", "guides/WINDOWS.en.md"):
        require("Windows-Accepted-SHA256" in text(guide) or "확인과 연결된" in text(guide) or "linked to that acceptance" in text(guide),
                f"{guide} lost its acceptance-link statement")
    result = run([sys.executable, "tools/check_public_docs.py"], timeout=300)
    require(result.returncode == 0, "the public documentation check failed")


# ------------------------------------------------------------ AC-13

def ac13():
    with shallow_clone() as clone:
        target = clone / "tools/release/check_published.py"
        source = target.read_text(encoding="utf-8")
        if "def verify_link(" in source:
            # Mutation: the published-release check ignores the acceptance link.
            target.write_text(source + "\n\ndef verify_link(*args, **kwargs):\n    return None\n", encoding="utf-8")
        elif "accepted(" in source:
            raise ObservationError("the published-release check reads the link outside verify_link; "
                                   "the mutation cannot disable it")
        code, output = earlier("AC-13", clone)
    if code == 2:
        raise ObservationError("the earlier producer could not observe AC-13: " + output)
    require(code == 1, "the earlier producer does not detect a published Windows archive without its acceptance link")
    if "acceptance link" not in output:
        raise ObservationError("the earlier producer's AC-13 failed for another reason: " + output)
    for number in range(1, 16):
        code, output = earlier(f"AC-{number}", ROOT)
        if code == 2:
            raise ObservationError(f"the earlier producer could not observe AC-{number}: {output}")
        require(code == 0, f"the earlier producer's AC-{number} does not pass: {output}")


# ------------------------------------------------------------ AC-14

REGISTRY_AT_START = DECLARATIONS / "registry-at-start.json"
WINDOWS_CONSUMERS = {"tools/release/check_archives.py", "tools/release/check_published.py"}
IMPORT_BOUNDARY = {"tools/check_repository_checks.py", "tools/release-checks/check.py",
                   "tools/release-checks/install_trace.py", "tools/release-checks/observe.py"}
GUARD_REGRESSION = "tools/public-checks/test_repository_checks.py"


def ac14():
    module = load("review_fix_regression_map", "tools/regression_map.py")
    try:
        registry = module.load(ROOT)
        report = module.validate(ROOT, registry)
    except module.Invalid as error:
        if "requires git" in str(error) or "Git source checkout" in str(error):
            raise ObservationError(f"the regression registry could not be read: {error}") from None
        raise Violation(f"the regression registry does not validate: {error}") from None
    start = json.loads(REGISTRY_AT_START.read_text(encoding="utf-8"))
    require(registry["invariants"] == start["invariants"], "the registry invariants changed")
    routes_by_id = {route["id"]: route for route in registry["routes"]}
    for old in start["routes"]:
        identifier, new = old["id"], routes_by_id.get(old["id"])
        require(new is not None and identifier in report["protected"], f"registry route {identifier} lost its protection")
        require(new["invariant"] == old["invariant"] and set(old["sources"]) <= set(new["sources"])
                and all(check in new["checks"] for check in old["checks"]),
                f"registry route {identifier} narrowed its invariant, consumers or regressions")
        if identifier != "archive-consumer-input-boundary":
            require(all(new[key] == old[key] for key in ("property", "families", "boundary")),
                    f"registry route {identifier} changed its protected property")
    archive_route = routes_by_id["archive-consumer-input-boundary"]
    windows = [route for route in registry["routes"] if route["invariant"] == "I3"
               and WINDOWS_CONSUMERS <= set(route["sources"])
               and re.search(r"windows|zip", route["property"], re.I)]
    require(windows, "no I3 route reflects the Windows archive consumers, as protection or as a gap")
    require(archive_route in windows or re.search(r"\btar\b|tar\.gz|tar/gzip", archive_route["property"], re.I),
            "the archive parser route still claims every archive consumer reads through its parser")

    def guarded(route, sources):
        return (route["id"] in report["protected"] and sources <= set(route["sources"])
                and any(check["file"] == GUARD_REGRESSION for check in route["checks"]))

    require(any(route["invariant"] == "I1" and guarded(route, {"tools/check_repository_checks.py"})
                for route in registry["routes"]),
            "the fixed-commit check is not connected to I1 as protection")
    require(any(guarded(route, IMPORT_BOUNDARY) for route in registry["routes"]),
            "the late-import check is not connected to the import boundary as protection")


# ------------------------------------------------------------ maintain

def ac8():
    code, output = earlier("AC-12", ROOT)
    if code == 2:
        raise ObservationError("the earlier producer could not observe AC-12: " + output)
    require(code == 0, "macOS and Linux installation changed: " + output)
    archives = load("review_fix_archives_targets", "tools/release/check_archives.py")
    require(tuple(archives.TARGETS) == ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64"),
            "the macOS/Linux archive set changed")


def ac9():
    result = run([sys.executable, "-m", "unittest", "discover", "-s", "tools/release", "-p", "test_windows_release.py"],
                 timeout=600)
    require(result.returncode == 0, "the publication gate regressions failed")
    for criterion in ("AC-3", "AC-11"):
        code, output = earlier(criterion, ROOT)
        if code == 2:
            raise ObservationError(f"the earlier producer could not observe {criterion}: {output}")
        require(code == 0, f"the release workflow's {criterion} behavior changed: {output}")
    archives = load("review_fix_archives_run", "tools/release/check_archives.py")
    published = load("review_fix_published_run", "tools/release/check_published.py")
    with tempfile.TemporaryDirectory(prefix="jaekit-review-fix-run-", dir=SCRATCH) as temp:
        dist = Path(temp) / "dist"
        artifact("release-files", dist)
        zips = sorted(dist.glob("ha_*_windows_amd64.zip"))
        if len(zips) != 1:
            raise ObservationError("HEAD's release run has no single Windows archive")
        version = zips[0].name.split("_")[1]
        try:
            archives.check(dist, version, require_windows=True)
        except ValueError as error:
            raise Violation(f"the release run's files failed the pre-publication check: {error}") from None
        message = f"Jaekit v{version}\n\nWindows-Accepted-SHA256: {sha256(zips[0].read_bytes())}\n"
        code, errors = published_cli(published, routes(dist, version, message, "annotated"), version)
        require(code == 0, "the release run's files with their link failed the published-release check: " + errors[-300:])


def ac10():
    for pattern in ("test_*.py", "integration_install_imports.py", "integration_observe_imports.py",
                    "integration_child_imports.py"):
        result = run([sys.executable, "-m", "unittest", "discover", "-s", "tools/release-checks", "-p", pattern],
                     timeout=600)
        require(result.returncode == 0, f"the acceptance checker regressions ({pattern}) failed")
    real = api(f"repos/{REPO}/releases/tags/v0.1.3")
    names = sorted(asset.get("name") for asset in real.get("assets", []))
    shapes = acceptance_shapes()
    four = sorted(f"ha_0.1.3_{p}.tar.gz" for p in ("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"))
    require(names == sorted(four + ["checksums.txt"]) and shapes["four"] == "accept",
            "the acceptance checker no longer accepts the real v0.1.3 asset shape")


def ac11():
    result = run([sys.executable, "-m", "unittest", "discover", "-s", "tools/release", "-p", "test_*.py"], timeout=600)
    require(result.returncode == 0, "the release check regressions failed")
    result = run([sys.executable, "tools/release/check_published.py", "--tag", "v0.1.4"], timeout=600)
    if result.returncode == 2:
        raise ObservationError("the public v0.1.4 release could not be read")
    require(result.returncode == 0, "the published-release check refused the existing v0.1.4 release")


def ac12():
    repo, sha = repository(), head()
    runs = api(f"repos/{repo}/actions/workflows/ci.yml/runs?event=pull_request&head_sha={sha}&per_page=100")
    matches = [item for item in runs.get("workflow_runs", []) if item.get("head_sha") == sha]
    if not matches:
        raise ObservationError("no CI run for HEAD's pull request yet")
    latest = max(matches, key=lambda item: (item.get("run_number", 0), item.get("run_attempt", 0)))
    if latest.get("status") != "completed":
        raise ObservationError("the CI run for HEAD has not finished")
    jobs = api(f"repos/{repo}/actions/runs/{latest['id']}/attempts/{latest['run_attempt']}/jobs?per_page=100")
    conclusions = {job.get("name"): job.get("conclusion") for job in jobs.get("jobs", [])}
    for name in ("Seal Core", "Public documentation", "Core / Ubuntu", "Core / macOS", "Core / Windows"):
        require(conclusions.get(name) == "success", f"the required CI job {name} did not succeed for HEAD")
    _, _, release = release_run()
    jobs = api(f"repos/{repo}/actions/runs/{release['id']}/attempts/{release['run_attempt']}/jobs?per_page=100")
    release_jobs = {job.get("name"): job.get("conclusion") for job in jobs.get("jobs", [])}
    require(release_jobs.get("Windows release archive") == "success", "the release run's Windows job did not succeed")
    result = run([sys.executable, "-m", "unittest", "discover", "-s", "tools/windows", "-p", "test_*.py"], timeout=600)
    require(result.returncode == 0, "the Windows packaging and public block regressions failed")
    readme = text("tools/windows/README.md")
    require("public_blocks.py acceptance" in readme and "Windows-Accepted-SHA256" in readme,
            "the Windows 11 acceptance procedure changed")
    for guide in ("guides/WINDOWS.md", "guides/WINDOWS.en.md"):
        body = text(guide)
        for label in ("PUBLIC INSTALL", "PUBLIC REMOVE", "VERIFIED INSTALL", "VERIFIED REMOVE"):
            require(f"<!-- BEGIN WINDOWS {label} -->" in body, f"{guide} lost its {label} block")


CONDITIONS = {"AC-1": ac1, "AC-2": ac2, "AC-3": ac3, "AC-4": ac4, "AC-5": ac5, "AC-6": ac6, "AC-7": ac7,
              "AC-8": ac8, "AC-9": ac9, "AC-10": ac10, "AC-11": ac11, "AC-12": ac12, "AC-13": ac13, "AC-14": ac14}


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
            "schema": "check-result/v1", "producer": "windows-release-review-fixes", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
