#!/usr/bin/env python3
"""Observe the Windows public release and paste-once installation conditions.

Each change condition first observes, in tracked files, whether its behavior
exists. Its absence is the declared baseline violation. Present behavior is then
observed locally (the release workflow's own build step, tampered archives, the
gate) and through the current HEAD's release workflow run on its pull request:
the built artifact, the build identity and both PowerShell reports. Missing
GitHub access, an unfinished run or an unreadable artifact is an observation
error, never a violation. Local caches are keyed by HEAD and re-verified by
digest before use; nothing is written into the checkout.
"""
from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
DECLARATIONS = ROOT / "tools/verification-checks/windows-release-install"
WORKFLOW = ".github/workflows/release.yml"
GUIDES = ("guides/WINDOWS.md", "guides/WINDOWS.en.md")
# Resolved: package.py refuses sources behind links such as macOS /var.
CACHE = Path(tempfile.gettempdir()).resolve() / "jaekit-windows-release-install"


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(value, message):
    if not value:
        raise Violation(message)


def text(path):
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except OSError:
        return ""


def run(argv, timeout=600, env=None, cwd=ROOT, show=False):
    try:
        result = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, encoding="utf-8",
                                errors="replace", timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ObservationError(f"{type(error).__name__}: {' '.join(map(str, argv[:3]))}") from None
    if show or result.returncode:
        print(result.stdout[-4000:], end="")
        print(result.stderr[-4000:], end="", file=sys.stderr)
    return result


def load(name, path):
    """Load a repository module; its own directory resolves sibling imports."""
    directory = str((ROOT / path).parent)
    sys.path.insert(0, directory)
    try:
        spec = importlib.util.spec_from_file_location(name, ROOT / path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    finally:
        sys.path.remove(directory)
    return module


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def head():
    """HEAD's commit; without a Git checkout (a source snapshot) the run, the
    build cache and the CI identity are unavailable, not violated."""
    result = run(["git", "rev-parse", "--verify", "HEAD^{commit}"])
    if result.returncode or not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", result.stdout.strip()):
        raise ObservationError("HEAD is unreadable; this observation needs a Git checkout")
    return result.stdout.strip()


# ------------------------------------------------------------ presence

def block_text(guide, label):
    match = re.search(rf"<!-- BEGIN WINDOWS {label} -->\s*```powershell\n(.+?)\n```\s*<!-- END WINDOWS {label} -->",
                      text(guide), re.S)
    return match[1] if match else ""


def present(criterion):
    """Directly observe each condition's own behavior in tracked files."""
    workflow = text(WORKFLOW)
    install = block_text(GUIDES[0], "PUBLIC INSTALL")
    remove = block_text(GUIDES[0], "PUBLIC REMOVE")
    if criterion == "AC-1":
        require("GOOS=windows" in workflow and "tools/windows/package.py" in workflow
                and "sha256sum -- *.tar.gz *.zip > checksums.txt" in workflow,
                "the release build produces no Windows archive in the shared checksums")
    elif criterion == "AC-2":
        require("def windows_files" in text("tools/release/check_archives.py"),
                "pre-publication checks do not inspect a Windows archive")
    elif criterion == "AC-3":
        require("runs-on: windows-2025" in workflow and "needs: [build, windows]" in workflow
                and "--checked-sha256" in workflow, "publication does not wait for a Windows check of the same archive")
    elif criterion == "AC-4":
        require(install and "-AddToPath" in install and "$env:Path += $bin" in install,
                "the guides have no public installation block that installs and updates this window")
    elif criterion == "AC-5":
        require(install and "$listed.Count -ne 1" in install and "$code -ne 0" in install,
                "the public block does not refuse checksum problems or a failed child")
    elif criterion == "AC-6":
        require(install and "-ExecutionPolicy Bypass -File" in install and "Set-ExecutionPolicy" not in install,
                "the public block has no process-scoped execution policy boundary")
    elif criterion == "AC-7":
        require(remove and "uninstall.ps1" in remove and ("공개 릴리스 업데이트" in text(GUIDES[0])),
                "the guides have no public update and removal path")
    elif criterion == "AC-8":
        require("tools/windows/public_blocks.py ci" in workflow and (ROOT / "tools/windows/public_blocks.py").is_file(),
                "no CI job executes the guides' public blocks")
    elif criterion == "AC-9":
        require("프로세스 한정 Bypass" in text(GUIDES[0]) and "process-scoped Bypass" in text(GUIDES[1]),
                "the guides do not state the public path's execution policy boundary")
    elif criterion == "AC-10":
        require("workflow_dispatch:" in workflow and "public_blocks.py acceptance" in text("tools/windows/README.md"),
                "there is no unpublished candidate build or Windows 11 acceptance procedure")
    elif criterion == "AC-11":
        require((ROOT / "tools/release/windows_gate.py").is_file() and "windows_gate.py" in workflow,
                "no gate links a published Windows archive to its Windows 11 acceptance")
    elif criterion == "AC-13":
        # The v0.1.3 acceptance checker stays pinned to its four archives; a
        # later tag is checked by check_published.py with the gate's rule.
        require("windows_gate" in text("tools/release/check_published.py"),
                "no published-release check verifies a Windows archive's acceptance link")


# ------------------------------------------------------------ local builds

def build_step():
    """The run script of the release workflow's 'Build release files' step."""
    lines = text(WORKFLOW).splitlines()
    start = lines.index("      - name: Build release files")
    index = lines.index("        run: |", start)
    script = []
    for line in lines[index + 1:]:
        if line.strip() and not line.startswith("          "):
            break
        script.append(line[10:])
    return "\n".join(script) + "\n"


def local_dist(version):
    """Run the workflow's build step for HEAD; cached per HEAD and version."""
    directory = CACHE / f"local-{head()}-{version}"
    dist = directory / "dist"
    if (directory / "complete").is_file():
        return dist
    if directory.exists():
        shutil.rmtree(directory)
    directory.mkdir(parents=True)
    # COPYFILE_DISABLE keeps macOS tar from adding AppleDouble entries that the
    # release runner's GNU tar never writes.
    env = dict(os.environ, VERSION=version, RUNNER_TEMP=str(directory), GOTOOLCHAIN="local",
               PYTHONDONTWRITEBYTECODE="1", COPYFILE_DISABLE="1")
    result = run(["bash", "-c", "set -euo pipefail\n" + build_step()], timeout=900, env=env)
    if result.returncode:
        raise Violation("the release build step failed for version " + version)
    (directory / "complete").write_text(json.dumps(sorted(p.name for p in dist.iterdir())))
    return dist


def check_archives():
    return load("windows_release_install_archives", "tools/release/check_archives.py")


def files_in_zip(data):
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        return {name: archive.read(name) for name in archive.namelist()}


def release_shape(dist, version):
    archives = check_archives()
    names = {path.name for path in dist.iterdir()}
    windows = f"ha_{version}_windows_amd64.zip"
    require(names == archives.release_names(version) | {windows, "checksums.txt"},
            "the release files are not four tar.gz archives, one Windows ZIP and checksums.txt")
    lines = (dist / "checksums.txt").read_text(encoding="utf-8").splitlines()
    require(len(lines) == 5 and sorted(line.split("  ", 1)[1] for line in lines) == sorted(names - {"checksums.txt"}),
            "checksums.txt does not list each of the five archives exactly once")
    try:
        archives.check(dist, version, require_windows=True)
    except ValueError as error:
        raise Violation("pre-publication check refused the release files: " + str(error)) from None
    return windows


# ------------------------------------------------------------ CI run

def repository():
    url = run(["git", "remote", "get-url", "origin"]).stdout.strip()
    match = re.search(r"github\.com[:/]([^/]+/[^/]+?)(?:\.git)?$", url)
    if not match:
        raise ObservationError("origin is not a GitHub repository")
    return match[1]


def api(path):
    result = run(["gh", "api", "--method", "GET", path], timeout=120)
    if result.returncode:
        raise ObservationError("GitHub request failed; check authentication and permission")
    return json.loads(result.stdout)


def release_run():
    """The completed release workflow run of HEAD's pull request, with its files."""
    repo, sha = repository(), head()
    runs = api(f"repos/{repo}/actions/workflows/release.yml/runs?event=pull_request&head_sha={sha}&per_page=100")
    matches = [item for item in runs.get("workflow_runs", []) if item.get("head_sha") == sha]
    if not matches:
        raise ObservationError("no release workflow run for HEAD's pull request yet")
    latest = max(matches, key=lambda item: (item.get("run_number", 0), item.get("run_attempt", 0)))
    if latest.get("status") != "completed":
        raise ObservationError("the release workflow run for HEAD has not finished")
    jobs = api(f"repos/{repo}/actions/runs/{latest['id']}/attempts/{latest['run_attempt']}/jobs?per_page=100")
    by_name = {job.get("name"): job for job in jobs.get("jobs", [])}
    directory = CACHE / f"run-{latest['id']}-{latest['run_attempt']}"
    if not (directory / "complete").is_file():
        if directory.exists():
            shutil.rmtree(directory)
        for name in ("release-files", "release-build-info", "release-windows-reports"):
            result = run(["gh", "run", "download", str(latest["id"]), "-R", repo, "-n", name, "-D",
                          str(directory / name)], timeout=600)
            if result.returncode:
                raise ObservationError("could not download artifact " + name)
        (directory / "complete").write_text("downloaded\n")
    build = json.loads((directory / "release-build-info/build.json").read_text(encoding="utf-8"))
    reports = {}
    for path in sorted((directory / "release-windows-reports").glob("*.json")):
        report = json.loads(path.read_text(encoding="utf-8"))
        reports[report.get("shell")] = report
    return {"repo": repo, "head": sha, "run": latest, "jobs": by_name, "dist": directory / "release-files",
            "build": build, "reports": reports}


def ci_identity(observed):
    """Bind the run's jobs, artifact and reports to HEAD and to one archive."""
    jobs = observed["jobs"]
    require(jobs.get("Build release files", {}).get("conclusion") == "success",
            "the release build job did not succeed for HEAD")
    require(jobs.get("Windows release archive", {}).get("conclusion") == "success",
            "the Windows release archive job did not succeed for HEAD")
    require(jobs.get("Publish release", {}).get("conclusion") in (None, "skipped"),
            "a pull request run attempted publication")
    build = observed["build"]
    require(build.get("source_commit") == observed["head"] == build.get("requested_source"),
            "the release run built a different source")
    version = build.get("version")
    windows = release_shape(observed["dist"], version)
    digest = sha256((observed["dist"] / windows).read_bytes())
    require(digest == build.get("windows_sha256"), "the downloaded archive is not the built archive")
    require(set(observed["reports"]) == {"powershell", "pwsh"}, "both PowerShell reports are required")
    for shell, report in observed["reports"].items():
        require(report.get("passed") is True and report.get("mode") == "ci", f"the {shell} report did not pass")
        require(report.get("source_commit") == observed["head"], f"the {shell} report checked another source")
        require(report.get("archive", {}).get("sha256") == digest == report.get("expected_sha256"),
                f"the {shell} report checked another archive")
    return version, digest


def cases(observed, shell):
    return {case["name"]: case for case in observed["reports"][shell]["cases"]}


def each_case(observed, names, check):
    for shell in ("powershell", "pwsh"):
        found = cases(observed, shell)
        for name in names:
            require(name in found, f"{shell} did not run case {name}")
            check(shell, found[name])


# ------------------------------------------------------------ conditions

def ac1():
    dist = local_dist("0.0.0")
    windows = release_shape(dist, "0.0.0")
    require(not any(path.suffix == ".ps1" for path in dist.iterdir()), "loose candidate scripts are release files")
    released = files_in_zip((dist / windows).read_bytes())
    CACHE.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="jaekit-package-compare-", dir=CACHE) as temp:
        binary = Path(temp) / "ha.exe"
        binary.write_bytes(released["ha.exe"])
        output = Path(temp) / "candidate"
        result = run([sys.executable, "tools/windows/package.py", "--binary", str(binary), "--version", "0.0.0",
                      "--output", str(output)], timeout=300)
        require(result.returncode == 0, "package.py could not package the released Core")
        candidate = files_in_zip((output / windows).read_bytes())
    require(candidate == released, "the released Windows ZIP differs from package.py's candidate for the same source")
    observed = release_run()
    ci_identity(observed)
    require(not any(path.suffix == ".ps1" for path in observed["dist"].iterdir()),
            "the CI release files contain loose candidate scripts")


def tampered(source: bytes, change):
    files = files_in_zip(source)
    change(files)
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as archive:
        for name, body in files.items():
            archive.writestr(name, body)
    return out.getvalue()


def ac2():
    archives = check_archives()
    dist = local_dist("0.0.0")
    windows = release_shape(dist, "0.0.0")
    real = (dist / windows).read_bytes()

    def manifest_version(files):
        value = json.loads(files["package.json"])
        value["version"] = "9.9.9"
        files["package.json"] = json.dumps(value).encode()

    variants = {
        "changed file": lambda files: files.__setitem__("tools/windows/install.ps1", b"Write-Output changed\n"),
        "missing manifest": lambda files: files.pop("package.json"),
        "other manifest version": manifest_version,
        "path traversal": lambda files: files.__setitem__("../outside.txt", b"x"),
        "reserved name": lambda files: files.__setitem__("guides/NUL.md", b"x"),
        "unlisted file": lambda files: files.__setitem__("guides/extra.md", b"x"),
        "not a Windows executable": lambda files: files.__setitem__("ha.exe", b"\x7fELF" + b"\0" * 60),
    }
    with tempfile.TemporaryDirectory(prefix="jaekit-tampered-") as temp:
        copy = Path(temp) / "dist"
        shutil.copytree(dist, copy)
        for label, change in variants.items():
            data = tampered(real, change)
            (copy / windows).write_bytes(data)
            lines = [line for line in (copy / "checksums.txt").read_text().splitlines() if not line.endswith(windows)]
            (copy / "checksums.txt").write_text("".join(l + "\n" for l in lines + [sha256(data) + "  " + windows]))
            try:
                archives.check(copy, "0.0.0", require_windows=True)
            except ValueError:
                continue
            raise Violation("pre-publication checks accepted a tampered archive: " + label)
        (copy / windows).write_bytes(real + b"changed")
        try:
            archives.check(copy, "0.0.0", require_windows=True)
        except ValueError:
            pass
        else:
            raise Violation("pre-publication checks accepted an archive with another checksum")
    result = run([sys.executable, "-m", "unittest", "tools/release/test_windows_release.py"], timeout=300)
    require(result.returncode == 0, "Windows release archive regressions failed")
    ci_identity(release_run())


def ac3():
    workflow = text(WORKFLOW)
    publish = workflow.split("\n  publish:\n", 1)[1]
    require("if: ${{ github.event_name == 'push' && startsWith(github.ref, 'refs/tags/v') }}" in publish
            and "CHECKED: ${{ needs.windows.outputs.sha256 }}" in publish
            and publish.index("windows_gate.py") < publish.index("gh release create"),
            "publication is not limited to tags after the Windows check of the same archive")
    require(workflow.count("contents: write") == 1 and "permissions:\n  contents: read" in workflow,
            "write permission is not limited to the publication job")
    gate = load("windows_release_install_gate", "tools/release/windows_gate.py")
    dist = local_dist("0.0.0")
    with tempfile.TemporaryDirectory(prefix="jaekit-gate-") as temp:
        copy = Path(temp) / "dist"
        shutil.copytree(dist, copy)
        before = {path.name: path.read_bytes() for path in copy.iterdir()}
        try:
            gate.select(copy, "0.0.0", "1" * 64, "")
        except gate.Refused:
            pass
        else:
            raise Violation("a different checked archive did not stop publication")
        require({path.name: path.read_bytes() for path in copy.iterdir()} == before, "a refused gate changed files")
    observed = release_run()
    version, digest = ci_identity(observed)
    for shell, report in observed["reports"].items():
        smoke = report.get("smoke", {})
        require(smoke.get("passed") is True and smoke.get("version_output") == "ha " + version
                and smoke.get("default_rules") == "run-rules/3", f"{shell} smoke did not run the packaged Core")


def ac4():
    observed = release_run()
    ci_identity(observed)

    def check(shell, case):
        require(case.get("passed") is True and not case.get("problems"), f"{shell} {case['name']} failed")
        obs, marker = case["observed"], case.get("marker") or {}
        require(obs.get("status") == "ok", f"{shell} {case['name']} did not succeed")
        require(marker.get("shell", "").lower() == obs.get("shell", "").lower()
                and marker.get("edition") == obs.get("edition"), f"{shell} child used another PowerShell")
        require(marker.get("add_to_path") is True and marker.get("archive_exists") and marker.get("checksums_exists"),
                f"{shell} installer lacked the checked files or -AddToPath")
        ha = obs.get("ha") or {}
        require(ha.get("type") == "Application" and ha.get("source", "").lower().endswith("\\jaekit\\bin\\ha.exe"),
                f"{shell} ha does not resolve to the installation in the same window")
        require("is ready in this window" in obs.get("output", ""), f"{shell} gave no restart guidance")
        require(case.get("temporary_left") == [], f"{shell} left temporary files")
        before, after = obs.get("path_before", ""), obs.get("path_after", "")
        if case["name"] == "probe-install":
            require(after.startswith(before) and after[len(before):].lstrip(";").lower().endswith("\\jaekit\\bin")
                    and after.lower().count("\\jaekit\\bin") == before.lower().count("\\jaekit\\bin") + 1,
                    f"{shell} did not append the installation to the window PATH once")
        else:
            require(after == before, f"{shell} changed a window PATH that already had the installation")

    each_case(observed, ("probe-install", "probe-install-path-present"), check)

    def real(shell, case):
        output = (case["observed"].get("output", "") + case["observed"].get("errors", "")).lower()
        require("running the verified installer in" in output and "windows 11 client is required" in output,
                f"{shell} real archive did not reach the installer")

    each_case(observed, ("real-install",), real)


def ac5():
    observed = release_run()
    ci_identity(observed)

    def check(shell, case):
        obs = case["observed"]
        require(case.get("passed") is True and obs.get("status") == "error", f"{shell} {case['name']} did not fail")
        require(obs.get("path_after") == obs.get("path_before"), f"{shell} {case['name']} changed the window PATH")
        require("is ready in this window" not in obs.get("output", "") and "Removal finished" not in obs.get("output", ""),
                f"{shell} {case['name']} reported success")
        if case["name"] in ("checksum-missing", "checksum-duplicate", "checksum-mismatch", "checksum-foreign-name",
                            "download-missing", "remove-checksum-missing", "remove-checksum-mismatch",
                            "remove-download-missing"):
            require(case.get("marker") is None and "Running the verified" not in obs.get("output", ""),
                    f"{shell} ran archive code after {case['name']}")
        if case["name"] in ("probe-install-fails", "probe-remove-fails", "real-install", "real-remove"):
            require("stopped with exit code" in obs.get("error", "") + obs.get("output", ""),
                    f"{shell} {case['name']} did not pass on the child's exit code")

    each_case(observed, ("checksum-missing", "checksum-duplicate", "checksum-mismatch", "checksum-foreign-name",
                         "download-missing", "remove-checksum-missing", "remove-checksum-mismatch",
                         "remove-download-missing", "probe-install-fails", "probe-remove-fails", "real-install",
                         "real-remove"), check)


def ac6():
    for guide in GUIDES:
        for label in ("PUBLIC INSTALL", "PUBLIC REMOVE"):
            code = block_text(guide, label)
            require(code.count("-ExecutionPolicy") == 1 and "Set-ExecutionPolicy" not in code
                    and "-ExecutionPolicy Bypass -File" in code, "a block changes or widens the execution policy")
    observed = release_run()
    ci_identity(observed)
    for shell, report in observed["reports"].items():
        require(report.get("restored_current_user_policy") == report.get("original_current_user_policy"),
                f"{shell} CurrentUser policy was not restored")
        for case in report["cases"]:
            obs = case["observed"]
            before, after = obs.get("policies_before") or {}, obs.get("policies_after") or {}
            require(before.get("CurrentUser") == "Restricted" and obs.get("process_policy") == "",
                    f"{shell} {case['name']} did not start with a persistent Restricted policy and no Process policy")
            require(all(before.get(name) == after.get(name) for name in ("CurrentUser", "LocalMachine")),
                    f"{shell} {case['name']} changed a persistent policy")
            require(obs.get("scripts_before") == "UnauthorizedAccess" == obs.get("scripts_after"),
                    f"{shell} {case['name']} window could run script files")
            marker = case.get("marker")
            if marker:
                require(marker.get("policy") == "Bypass" and marker.get("process_policy") == "Bypass"
                        and marker.get("current_user_policy") == "Restricted",
                        f"{shell} child Bypass was not process-scoped")
        found = {case["name"]: case for case in report["cases"]}
        for gpo in (found.get("group-policy-allsigned"), found.get("group-policy-allsigned-remove")):
            require(gpo is not None and gpo.get("passed") is True
                    and gpo.get("group_policy") == "AllSigned"
                    and gpo["observed"].get("status") == "error" and gpo.get("marker") is None
                    and gpo["observed"].get("path_after") == gpo["observed"].get("path_before"),
                    f"{shell} Group Policy did not take precedence over a block")
        real = (found["real-install"]["observed"].get("output", "") + found["real-install"]["observed"].get("errors", "")).lower()
        require(not any(phrase in real for phrase in ("running scripts is disabled", "cannot be loaded", "not digitally signed")),
                f"{shell} the Restricted policy blocked the checked installer")


def ac7():
    for guide, heading in ((GUIDES[0], "### 공개 릴리스 업데이트"), (GUIDES[1], "### Update from a public release")):
        section = text(guide).split(heading, 1)
        require(len(section) == 2 and ("설치 블록을 그대로 다시" in section[1].split("###", 1)[0]
                                       or "installation block from the newer version's guide again" in section[1].split("###", 1)[0]),
                "the guide does not describe the public update")
    remove = block_text(GUIDES[0], "PUBLIC REMOVE")
    require(remove == block_text(GUIDES[1], "PUBLIC REMOVE") and "'common.ps1', 'uninstall.ps1'" in remove
            and "$env:Path" not in remove and "keep their old PATH" in remove,
            "the removal block does not run only the checked uninstaller with PATH guidance")
    observed = release_run()
    ci_identity(observed)

    def check(shell, case):
        obs, marker = case["observed"], case.get("marker") or {}
        require(case.get("passed") is True, f"{shell} {case['name']} failed")
        if case["name"] == "probe-remove":
            require(obs.get("status") == "ok" and marker.get("script") == "uninstall"
                    and marker.get("edition") == obs.get("edition") and "Removal finished" in obs.get("output", "")
                    and obs.get("path_after") == obs.get("path_before"), f"{shell} removal path failed")
        elif case["name"].startswith("remove-"):
            # The removal block downloads and checks its own archive too.
            require(obs.get("status") == "error" and case.get("marker") is None
                    and "Running the verified uninstaller" not in obs.get("output", "")
                    and "Removal finished" not in obs.get("output", "")
                    and obs.get("path_after") == obs.get("path_before"), f"{shell} {case['name']} ran unchecked code")
        else:
            output = (obs.get("output", "") + obs.get("errors", "")).lower()
            require(obs.get("status") == "error" and "running the verified uninstaller in" in output
                    and "windows 11 client is required" in output, f"{shell} real removal did not reach the uninstaller")

    each_case(observed, ("probe-remove", "real-remove", "remove-checksum-missing", "remove-checksum-mismatch",
                         "remove-download-missing"), check)


def ac8():
    workflow = text(WORKFLOW)
    windows = workflow.split("\n  windows:\n", 1)[1].split("\n  publish:\n", 1)[0]
    for shell in ("powershell", "pwsh"):
        require(f"--shell {shell}" in windows and 'public_blocks.py ci --dist "$env:RUNNER_TEMP/dist"' in windows,
                "the Windows job does not run the public blocks in " + shell)
    require("name: release-files" in windows, "the Windows job does not use the built release files")
    harness = load("windows_release_install_blocks", "tools/windows/public_blocks.py")
    current = {kind: sha256(code.encode()) for kind, code in harness.public_blocks().items()}
    observed = release_run()
    ci_identity(observed)
    editions = {}
    for shell, report in observed["reports"].items():
        require(report.get("block_sha256") == current, f"{shell} executed other block text than HEAD's guides")
        found = {case["name"]: case for case in report["cases"]}
        require({"real-install", "real-remove"} <= found.keys(), f"{shell} did not paste the blocks with the real archive")
        editions[shell] = {case["observed"].get("edition") for case in report["cases"]}
        require(all(case["observed"].get("shell", "").lower() == report.get("shell_path", "").lower()
                    for case in report["cases"]), f"{shell} cases ran in another shell")
    require(editions == {"powershell": {"Desktop"}, "pwsh": {"Core"}}, "the blocks did not run in both editions")


def ac9():
    ko, en = text(GUIDES[0]), text(GUIDES[1])
    readme, ci_ko, ci_en = text("tools/windows/README.md"), text("guides/CI.md"), text("guides/CI.en.md")
    for document, phrases in (
        (ko, ("영구 실행 정책(CurrentUser·LocalMachine)을 바꾸지 않고", "그룹 정책(MachinePolicy·UserPolicy)으로 정한 실행 정책은 이 옵션보다 우선합니다",
              "이 후보 경로의 설치기는 실행 정책·보안 설정을 바꾸거나 우회 옵션으로 다시 실행하지 않습니다",
              "출처를 인증하는 서명이 아닙니다", "Server 결과는 Windows 11 설치 성공이 아닙니다", "Windows 11 x64",
              "Windows ARM64·x86")),
        (en, ("does not change the persistent execution policy (CurrentUser, LocalMachine)",
              "set by Group Policy (MachinePolicy, UserPolicy) takes precedence over this option",
              "This candidate path's installer does not change security settings or relaunch itself with policy-bypass options",
              "They are not signatures that authenticate the source", "not a Windows 11 installation success",
              "Windows 11 x64", "Windows ARM64/x86")),
        (readme, ("process-scoped Bypass", "These candidate tools use no unchecked bootstrap script, policy bypass")),
        (ci_ko, ("## 릴리스 workflow", "Windows 11 설치 성공이 아니며")),
        (ci_en, ("## Release workflow", "not a Windows 11 installation success")),
    ):
        for phrase in phrases:
            require(phrase in document, "missing boundary statement: " + phrase)
    for document, marker, phrases in (
        (ko, "<!-- BEGIN WINDOWS PUBLIC INSTALL -->", ("아직 Windows 파일이 든 공개 릴리스는 없으며", "현재 배포판 `v0.1.4`에도 Windows 파일이 없습니다")),
        (en, "<!-- BEGIN WINDOWS PUBLIC INSTALL -->", ("No public release with Windows files exists yet", "the current release, `v0.1.4`, has no Windows files")),
    ):
        before = document.split(marker, 1)[0].rstrip().rsplit("\n\n", 1)[-1]
        for phrase in phrases:
            require(phrase in before, "the paragraph right above the block does not say: " + phrase)


def ac10():
    workflow = text(WORKFLOW)
    require("workflow_dispatch:\n    inputs:\n      version:" in workflow and "  pull_request:\n" in workflow,
            "the release workflow cannot build an unpublished candidate")
    first, second = local_dist("0.0.0"), local_dist("0.1.4")
    digests = set()
    for version, dist in (("0.0.0", first), ("0.1.4", second)):
        windows = release_shape(dist, version)
        digests.add(sha256((dist / windows).read_bytes()))
    require(len(digests) == 2, "two candidate versions produced the same archive")
    readme = text("tools/windows/README.md")
    for phrase in ("workflow_dispatch", "public_blocks.py acceptance --first", "--consent-user-installation",
                   "Windows-Accepted-SHA256:", "Restricted"):
        require(phrase in readme, "the acceptance procedure does not state: " + phrase)
    source = text("tools/windows/public_blocks.py")
    require('steps = [("powershell", "install", first), ("pwsh", "install", newer), ("pwsh", "remove", newer),' in source
            and '("pwsh", "install", first), ("powershell", "install", newer), ("powershell", "remove", newer)]' in source
            and 'print("Windows-Accepted-SHA256: " + report["accepted_sha256"])' in source,
            "the acceptance procedure does not install, update and remove in both shells and report the digest")
    observed = release_run()
    ci_identity(observed)


def ac11():
    gate = load("windows_release_install_gate11", "tools/release/windows_gate.py")
    archives = check_archives()
    workflow = text(WORKFLOW)
    require('git cat-file -t "refs/tags/${TAG}"' in workflow and "--tag-message" in workflow,
            "publication does not read the tag object's message")
    dist = local_dist("0.0.0")
    windows = release_shape(dist, "0.0.0")
    digest = sha256((dist / windows).read_bytes())
    rebuilt = CACHE / f"rebuild-{head()}"
    if rebuilt.exists():
        shutil.rmtree(rebuilt)
    env = dict(os.environ, VERSION="0.0.0", RUNNER_TEMP=str(rebuilt), GOTOOLCHAIN="local", PYTHONDONTWRITEBYTECODE="1",
               COPYFILE_DISABLE="1")
    rebuilt.mkdir(parents=True)
    result = run(["bash", "-c", "set -euo pipefail\n" + build_step()], timeout=900, env=env)
    require(result.returncode == 0, "the release build step failed on rebuild")
    require(sha256((rebuilt / "dist" / windows).read_bytes()) == digest,
            "a second build of the same source and version produced another Windows archive")
    shutil.rmtree(rebuilt)
    cases = {"linked": ("Windows-Accepted-SHA256: " + digest + "\n", True),
             "lightweight tag": ("", False), "two lines": (f"Windows-Accepted-SHA256: {digest}\n" * 2, False),
             "other digest": ("Windows-Accepted-SHA256: " + "0" * 64 + "\n", False),
             "uppercase": ("Windows-Accepted-SHA256: " + digest.upper() + "\n", False)}
    with tempfile.TemporaryDirectory(prefix="jaekit-gate11-") as temp:
        for label, (message, keep) in cases.items():
            copy = Path(temp) / label.replace(" ", "-")
            shutil.copytree(dist, copy)
            kept, _ = gate.select(copy, "0.0.0", digest, message)
            require(kept is keep, "the gate decided wrongly for: " + label)
            try:
                names = archives.check(copy, "0.0.0")
            except ValueError as error:
                raise Violation("the gated release files fail the pre-publication check: " + str(error)) from None
            require((windows in names) is keep and len((copy / "checksums.txt").read_text().splitlines()) == (5 if keep else 4),
                    "the gated files do not match the decision for: " + label)


def ac13():
    workflow = text(WORKFLOW)
    publish = workflow.split("\n  publish:\n", 1)[1]
    require('[[ "${TAG}" =~ ^v[0-9]+\\.[0-9]+\\.[0-9]+$ ]]' in workflow and 'test -f "tools/release/notes/${TAG}.md"' in workflow
            and "--verify-tag" in publish and "--published-assets" in publish
            and len(re.findall(r"--clobber", workflow)) == 1 and "never delete assets or use --clobber" in workflow,
            "the release workflow lost a tag, note, overwrite or uploaded-name guarantee")
    published = load("windows_release_install_published", "tools/release/check_published.py")
    dist = local_dist("0.0.0")

    def unread(repo, tag):
        raise Violation("the published-release check read the tag of a release without the Windows archive")

    for keep in (True, False):
        with tempfile.TemporaryDirectory(prefix="jaekit-published-shape-") as temp:
            copy = Path(temp) / "dist"
            shutil.copytree(dist, copy)
            if not keep:
                windows = copy / "ha_0.0.0_windows_amd64.zip"
                windows.unlink()
                lines = [l for l in (copy / "checksums.txt").read_text().splitlines() if not l.endswith(".zip")]
                (copy / "checksums.txt").write_text("".join(l + "\n" for l in lines))
            assets = [{"name": p.name, "size": p.stat().st_size, "digest": "sha256:" + sha256(p.read_bytes()),
                       "browser_download_url": f"https://github.com/jgoneit/jaekit/releases/download/v0.0.0/{p.name}"}
                      for p in sorted(copy.iterdir())]
            metadata = {"tag_name": "v0.0.0", "draft": False, "assets": assets}

            def download(url):
                return (copy / url.rsplit("/", 1)[1]).read_bytes()

            digest = sha256((dist / "ha_0.0.0_windows_amd64.zip").read_bytes())
            linked = lambda repo, tag: (True, "Jaekit v0.0.0\n\nWindows-Accepted-SHA256: " + digest + "\n")
            try:
                names = published.check_release("jgoneit/jaekit", "v0.0.0", metadata, download,
                                                tag_reader=linked if keep else unread)
            except ValueError as error:
                raise Violation("the published-release check refused a valid shape: " + str(error)) from None
            require(any(n.endswith(".zip") for n in names) is keep, "the published check misread the release shape")
            if keep:
                for label, reader in (("a lightweight tag", lambda repo, tag: (False, "")),
                                      ("no link", lambda repo, tag: (True, "Jaekit v0.0.0\n")),
                                      ("another digest", lambda repo, tag: (True, "Windows-Accepted-SHA256: " + "0" * 64))):
                    try:
                        published.check_release("jgoneit/jaekit", "v0.0.0", metadata, download, tag_reader=reader)
                    except ValueError:
                        continue
                    raise Violation(f"the published-release check accepted a Windows archive under {label}, "
                                    "without its acceptance link")
    result = run([sys.executable, "tools/release/check_published.py", "--tag", "v0.1.4"], timeout=600, show=True)
    if result.returncode not in (0, 1):
        raise ObservationError("could not read the public v0.1.4 release")
    require(result.returncode == 0, "the published-release check refused the existing v0.1.4 release")
    for directory in ("tools/release-checks", "tools/release"):
        result = run([sys.executable, "-m", "unittest", "discover", "-s", directory, "-p", "test_*.py"], timeout=900)
        require(result.returncode == 0, directory + " regressions failed")


# ------------------------------------------------------------ maintained

def ac12():
    native = load("windows_release_install_native", "tools/ci/native_unix.py")
    document = text("guides/INSTALL.md")
    blocks = native.guide_blocks(document, native.GUIDES["INSTALL.md"])
    machine = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[os.uname().machine.lower()]
    target = f"{'darwin' if sys.platform == 'darwin' else 'linux'}_{machine}"
    dists = {"predecessor": local_dist("0.0.0"), "current": local_dist(blocks["version"])}
    for version, dist in (("0.0.0", dists["predecessor"]), (blocks["version"], dists["current"])):
        release_shape(dist, version)
    routes = {}
    for label, dist in dists.items():
        for path in dist.iterdir():
            routes[f"/{label}/{path.name}"] = path.read_bytes()
    with tempfile.TemporaryDirectory(prefix="jaekit-direct-install-") as temp, native.local_transport(routes) as base:
        root = Path(temp) / "user root"
        root.mkdir()
        env = dict(os.environ, JAEKIT_CI_INSTALL_ROOT=str(root), NO_PROXY="127.0.0.1", no_proxy="127.0.0.1")
        for label, predecessor, version in (("predecessor", True, "0.0.0"), ("current", False, blocks["version"])):
            script = native.install_script(blocks, base + "/" + label, predecessor=predecessor)
            result = run(["bash", "-c", "set -e\n" + script], timeout=300, env=env, cwd=root)
            require(result.returncode == 0, f"the unchanged direct installation block failed with five-archive checksums ({label})")
            output = run([str(root / ".local/bin/ha"), "--version"], timeout=30).stdout.strip()
            require(output == "ha " + version, f"direct installation did not install {version} for {target}")
    formula = run(["gh", "api", "repos/jgoneit/homebrew-tap/contents/Formula/jaekit.rb", "--jq", ".content"], timeout=120)
    if formula.returncode:
        raise ObservationError("could not read the Homebrew formula")
    import base64
    urls = re.findall(r'url "https://github\.com/jgoneit/jaekit/releases/download/v([0-9.]+)/([^"]+)"',
                      base64.b64decode(formula.stdout).decode("utf-8"))
    require(urls and all(name in check_archives().release_names(version) for version, name in urls),
            "the Homebrew formula names files outside the unchanged tar.gz set")
    result = run(["go", "test", "-count=1", "./plugins/", "-run", "TestReleaseGuide"], timeout=600)
    require(result.returncode == 0, "the release guide tests failed")


def ac14():
    # The full common verification outlasts one recorded check, so it is
    # observed where CI runs it for HEAD: Core / Ubuntu runs `verify.py go`
    # and Public documentation runs `verify.py docs`; Core / Windows runs the
    # native candidate checks on PowerShell 5.1 and 7.
    # The candidate's properties are judged from the current files, never by
    # comparing with an earlier commit: package.py alone and its candidate,
    # the verified-candidate blocks, and the installer's guards and rules.
    observed_ci = ci_run()
    for name in ("Seal Core", "Public documentation", "Core / Ubuntu", "Core / macOS", "Core / Windows"):
        require(observed_ci.get(name) == "success", f"the required CI job {name} did not succeed for HEAD")
    for guide in GUIDES:
        document = text(guide)
        for label in ("VERIFIED INSTALL", "VERIFIED REMOVE"):
            require(f"<!-- BEGIN WINDOWS {label} -->" in document and "Get-JaekitVerifiedCandidate" in document,
                    f"{guide} lost its verified-candidate {label} block")
    for directory in ("tools/windows", "tools/release"):
        result = run([sys.executable, "-m", "unittest", "discover", "-s", directory, "-p", "test_*.py"], timeout=600)
        require(result.returncode == 0, f"the {directory} regressions failed")


def ci_run():
    repo, sha = repository(), head()
    runs = api(f"repos/{repo}/actions/workflows/ci.yml/runs?event=pull_request&head_sha={sha}&per_page=100")
    matches = [item for item in runs.get("workflow_runs", []) if item.get("head_sha") == sha]
    if not matches:
        raise ObservationError("no CI run for HEAD's pull request yet")
    latest = max(matches, key=lambda item: (item.get("run_number", 0), item.get("run_attempt", 0)))
    if latest.get("status") != "completed":
        raise ObservationError("the CI run for HEAD has not finished")
    jobs = api(f"repos/{repo}/actions/runs/{latest['id']}/attempts/{latest['run_attempt']}/jobs?per_page=100")
    return {job.get("name"): job.get("conclusion") for job in jobs.get("jobs", [])}


def ac15():
    # The release guidance is judged by its properties in the current files:
    # the existing guide tests pin README and INSTALL to the current release
    # and its platforms, and the Skill says that release has no Windows file.
    require("Windows is not supported by that release; it has no Windows artifact"
            in text("plugins/seal/skills/seal/SKILL.md"),
            "the Seal Skill no longer says the current release has no Windows artifact")
    result = run(["go", "test", "-count=1", "./plugins/", "-run",
                  "TestPublicGuidanceNamesOnlyTheCurrentRelease|TestRelease011|TestReadme"], timeout=600)
    require(result.returncode == 0, "the release and README guidance tests failed")


CONDITIONS = {"AC-1": ac1, "AC-2": ac2, "AC-3": ac3, "AC-4": ac4, "AC-5": ac5, "AC-6": ac6, "AC-7": ac7,
              "AC-8": ac8, "AC-9": ac9, "AC-10": ac10, "AC-11": ac11, "AC-12": ac12, "AC-13": ac13,
              "AC-14": ac14, "AC-15": ac15}


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
        if structured:
            present(criterion)
        CONDITIONS[criterion]()
    except Violation as error:
        observation.update(status="violation", violation=violation)
        print(str(error), file=sys.stderr)
        code = 1
    except (ObservationError, OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        observation.update(status="error", reason="execution_error")
        print(f"{type(error).__name__}: {error}", file=sys.stderr)
        code = 2
    if structured and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "windows-release-install", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
