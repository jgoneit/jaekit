#!/usr/bin/env python3
"""Observe Unix native fixtures and isolated installs of this source candidate.

Uses the real host OS, Go compiler, processes, Git and installer commands. Only
the guide's destination, candidate download URL and CI predecessor storage
version are rebound. HOME, the checkout and the user's installation stay intact.
The predecessor is the same source with a CI-only version stamp, not an older
release. This is neither release acceptance nor an agent-host/WSL observation.
"""
from __future__ import annotations

import argparse
from contextlib import contextmanager
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import stat
import subprocess
import sys
import tarfile
import tempfile
import threading

sys.dont_write_bytecode = True
from native_results import native_results

ROOT = Path(__file__).resolve().parents[2]
PREFIX = "JAEKIT_CI_REPORT "
MODULE = "github.com/jgoneit/jaekit/"
PACKAGES = ("internal/run", "internal/record", "internal/evidence", "cmd/ha")
REQUIRED = {
    "internal/run": ("TestNativeAdmissionFailureHasNoOutputDigest",
                     "TestNativeUnconfirmedShutdownHasNoOutputDigest",
                     "TestNativeArgvEnvironmentAndOutcomes", "TestNativeTimeoutStopsDescendantWrites"),
    "internal/record": ("TestNativeConcurrentProcessesPreserveRecords", "TestNativeLockIsReleasedWhenOwnerExits"),
    "internal/evidence": ("TestNativeCleanCRLFStillRequiresCommittedBytes",),
    "cmd/ha": ("TestNativeCoreGitLifecycle", "TestNativeCoreGitLifecycle/linked=false",
               "TestNativeCoreGitLifecycle/linked=true"),
}
HELPERS = {MODULE + "internal/run/TestNativeProcessHelper", MODULE + "internal/record/TestNativeRecordHelper"}
GUIDES = {
    "INSTALL.md": ("## Release 파일로 설치", "## 업데이트", "## 제거"),
    "INSTALL.en.md": ("## Install from a Release file", "## Update", "## Uninstall"),
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def public_digest(files):
    return digest(json.dumps({name: digest(body) for name, body in files.items()},
                             sort_keys=True, separators=(",", ":")).encode())


def invoke(argv, *, root=ROOT, env=None, timeout=300, success=True):
    command = [str(item) for item in argv]
    process = subprocess.Popen(command, cwd=root, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except (subprocess.TimeoutExpired, KeyboardInterrupt):
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.communicate()
        raise
    result = subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
    if success and result.returncode:
        # Output is useful for diagnosing actual native failures. Inputs are
        # public source and synthetic fixtures, never local goal documents.
        print(result.stdout, end="", flush=True)
        print(result.stderr, end="", file=sys.stderr, flush=True)
        raise ValueError(f"{Path(argv[0]).name} exited {result.returncode}")
    return result


def guide_blocks(document, headings):
    def blocks(heading):
        require(document.count(heading + "\n") == 1, "guide section is missing or ambiguous: " + heading)
        section = document.split(heading + "\n", 1)[1].split("\n## ", 1)[0]
        return re.findall(r"```bash\n(.*?)```", section, re.S)
    install = blocks(headings[0])
    require(len(install) == 1, "direct installation must contain one command block")
    match = re.search(r'name="ha_([0-9]+\.[0-9]+\.[0-9]+)_\$\{os\}_\$\{arch\}"', install[0])
    require(match is not None, "guide candidate name is not recognized")
    version = match[1]
    url = "https://github.com/jgoneit/jaekit/releases/download/v" + version
    require(install[0].count(url) == 1, "guide release URL is missing or ambiguous")
    cleanup = [b for b in blocks(headings[1]) if ".local/share/jaekit" in b]
    remove = [b for b in blocks(headings[2]) if ".local/bin/ha" in b or ".local/share/jaekit" in b]
    require(len(cleanup) == 1 and len(remove) == 2, "direct update/removal blocks changed")
    return {"install": install[0], "cleanup": cleanup[0], "remove": remove,
            "version": version, "url": url}


def bind_destination(block):
    # Leave the actual process HOME untouched. Quoting keeps spaces and Korean
    # names meaningful when the guide uses an unquoted ~/ destination.
    block = block.replace("$HOME", "$JAEKIT_CI_INSTALL_ROOT")
    block = block.replace("~/", '"$JAEKIT_CI_INSTALL_ROOT"/')
    require("$HOME" not in block and "~/" not in block, "unbound installation destination")
    return block


def install_script(blocks, url, predecessor=False):
    script = blocks["install"].replace(blocks["url"], url)
    if predecessor:
        # The numeric storage version exercises actual versioned-data update.
        # Binary identity remains explicitly CI-only (0.0.0-ci-predecessor).
        script = script.replace(blocks["version"], "0.0.0")
    return bind_destination(script)


def snapshot(root):
    result = {}
    for path in sorted(root.rglob("*")):
        name = path.relative_to(root).as_posix()
        mode = path.lstat().st_mode
        if stat.S_ISLNK(mode):
            result[name] = ("link", str(path.readlink()))
        elif stat.S_ISDIR(mode):
            result[name] = ("directory", stat.S_IMODE(mode))
        elif stat.S_ISREG(mode):
            result[name] = ("file", stat.S_IMODE(mode), digest(path.read_bytes()))
        else:
            raise ValueError("special file in isolated installation: " + name)
    return result


def public_files(root, env):
    fixed = ["LICENSE", "README.md", "README.en.md", "tools/check-result-reference.py"]
    fixed += ["examples/check-result/" + name for name in ("README.md", "declaration.json", "reference.json", "input.txt")]
    names = invoke(["git", "ls-files", "-z", "--", *fixed, "guides", "contracts", "assets"], root=root, env=env).stdout.split("\0")
    require(set(fixed) <= set(names), "candidate is missing a required tracked public file")
    files = {}
    for name in filter(None, names):
        path = root / name
        require(not path.is_symlink() and path.is_file(), "candidate public file is not ordinary: " + name)
        require(all(not parent.is_symlink() for parent in path.parents if parent != root),
                "candidate source contains a directory symlink")
        files[name] = path.read_bytes()
    return files


def make_archive(binary, public, storage_version, target):
    name = f"ha_{storage_version}_{target}.tar.gz"
    prefix = name.removesuffix(".tar.gz")
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode="w:gz") as archive:
        for relative, body in sorted(dict(public, ha=binary.read_bytes()).items()):
            info = tarfile.TarInfo(prefix + "/" + relative)
            info.size, info.mode = len(body), 0o755 if relative == "ha" else 0o644
            archive.addfile(info, io.BytesIO(body))
    data = stream.getvalue()
    return name, data, (digest(data) + "  " + name + "\n").encode()


@contextmanager
def local_transport(routes):
    """Serve only the generated archive/checksum bytes; no filesystem server."""
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = routes.get(self.path)
            if body is None:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *_args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield "http://127.0.0.1:" + str(server.server_port)
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)
        require(not thread.is_alive(), "candidate transport did not stop")


def identity(binary, env, expected_version, root):
    version = invoke([binary, "--version"], root=root, env=env).stdout.strip()
    require(version == "ha " + expected_version, "source candidate version does not match its build")
    caps = json.loads(invoke([binary, "capabilities", "--format", "json"], root=root, env=env).stdout)
    require(caps.get("schema") == "ha-capabilities/v1" and caps.get("ha_version") == expected_version
            and caps.get("default_rules") == "run-rules/3", "source candidate capability contract mismatch")
    return {"version": expected_version, "binary_sha256": digest(binary.read_bytes()), "capabilities": caps}


def installed_identity(install_root, expected, env):
    selected_env = dict(env, PATH=str(install_root / ".local/bin") + os.pathsep + env.get("PATH", ""))
    selected = shutil.which("ha", path=selected_env["PATH"])
    binary = install_root / ".local/bin/ha"
    require(selected is not None and Path(selected) == binary and not binary.is_symlink(),
            "PATH did not select the isolated installed candidate")
    actual = identity(Path(selected), selected_env, expected["version"], install_root)
    require(actual == {key: expected[key] for key in actual}, "installed binary/capabilities differ from source candidate")
    data_root = install_root / ".local/share/jaekit" / expected["storage_version"]
    data = {}
    for path in data_root.rglob("*"):
        require(not path.is_symlink(), "installed public data contains a link")
        if path.is_file():
            data[path.relative_to(data_root).as_posix()] = path.read_bytes()
    require(public_digest(data) == expected["public_data_sha256"], "installed public data differs from the source candidate")


def lifecycle(directory, filename, blocks, candidates, base_url, bash, env, report):
    install_root = directory / (filename + " 설치 root")
    install_root.mkdir()
    sentinels = {"project/docs/specs/example/runs.jsonl": "synthetic project record\n",
                 "project/.git/ha/raw.log": "synthetic project output\n",
                 "project/tools/check-result-reference.py": "copied project producer\n",
                 "user-notes/keep.txt": "unrelated user data\n",
                 ".local/bin/unrelated-tool": "unrelated executable\n"}
    for name, body in sentinels.items():
        path = install_root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body, encoding="utf-8")
    original = snapshot(install_root)
    item = {"guide": "guides/" + filename, "checks": [], "selected_binary": "isolated-user-root/.local/bin/ha"}
    report.append(item)
    child_env = dict(env, JAEKIT_CI_INSTALL_ROOT=str(install_root), NO_PROXY="127.0.0.1", no_proxy="127.0.0.1")

    def shell(script, success=True):
        return invoke([bash, "-c", "set -e\n" + script], root=install_root / "project", env=child_env,
                      timeout=90, success=success)

    prior, current = candidates["predecessor"], candidates["current"]
    prior_script = install_script(blocks, base_url + "/predecessor", predecessor=True)
    shell(prior_script)
    installed_identity(install_root, prior, env)
    item["checks"].append("fresh")
    before = snapshot(install_root)
    shell(prior_script)
    installed_identity(install_root, prior, env)
    require(snapshot(install_root) == before, "identical reinstall changed installed/user data")
    item["checks"].append("reinstall")

    failed = shell(install_script(blocks, base_url + "/invalid"), success=False)
    require(failed.returncode != 0 and "FAILED" in failed.stdout + failed.stderr,
            "invalid checksum did not produce a checksum rejection")
    require(snapshot(install_root) == before, "checksum rejection changed the prior installation")
    installed_identity(install_root, prior, env)
    item["checks"].append("checksum_rejection_preserves")

    share = install_root / ".local/share/jaekit"
    current_data = share / blocks["version"]
    current_data.mkdir()
    (current_data / "user-conflict.txt").write_text("preserve conflicting user data\n", encoding="utf-8")
    before = snapshot(install_root)
    failed = shell(install_script(blocks, base_url + "/current"), success=False)
    require(failed.returncode != 0 and "nothing was replaced" in failed.stdout + failed.stderr,
            "data conflict was not explicitly rejected")
    require(snapshot(install_root) == before, "conflict rejection changed prior installation/data")
    installed_identity(install_root, prior, env)
    item["checks"].append("conflict_rejection_preserves")
    # This is our known synthetic collision, never a discovered user path.
    (current_data / "user-conflict.txt").unlink()
    current_data.rmdir()

    shell(install_script(blocks, base_url + "/current"))
    installed_identity(install_root, current, env)
    require((share / "0.0.0").is_dir() and current_data.is_dir(), "update did not retain the predecessor data")
    item["checks"].append("update")
    shell(bind_destination(blocks["cleanup"]))
    require(not (share / "0.0.0").exists() and current_data.is_dir(), "update cleanup did not preserve current data")
    installed_identity(install_root, current, env)
    item["checks"].append("old_data_cleanup")
    item.update(installed_version=current["version"], binary_sha256=current["binary_sha256"])
    for block in blocks["remove"]:
        shell(bind_destination(block))
    require(not (install_root / ".local/bin/ha").exists() and not share.exists(), "uninstall left the candidate installed")
    after = snapshot(install_root)
    require(all(after.get(name) == value for name, value in original.items()), "uninstall changed unrelated project/user data")
    item["checks"].append("uninstall_preserves")


def observe(report, root=ROOT):
    root = root.resolve()
    env = dict(os.environ, GOTOOLCHAIN="local", PYTHONDONTWRITEBYTECODE="1")
    target_os = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    target_arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(platform.machine().lower())
    require(target_os is not None and target_arch is not None, "native Unix checks require macOS/Linux amd64/arm64")
    for tool in ("go", "git", "bash", "uname", "curl", "tar"):
        require(shutil.which(tool) is not None, "required developer tool is missing: " + tool)
    bash = shutil.which("bash")
    settings = json.loads(invoke(["go", "env", "-json", "GOFLAGS", "GOOS", "GOARCH", "GOHOSTOS", "GOHOSTARCH"], root=root, env=env).stdout)
    require(not settings["GOFLAGS"].strip(), "native checks require empty effective GOFLAGS")
    require(settings["GOOS"] == settings["GOHOSTOS"] == target_os
            and settings["GOARCH"] == settings["GOHOSTARCH"] == target_arch,
            "effective Go target/compiler must match the real native OS/CPU")
    report["source_commit"] = invoke(["git", "rev-parse", "HEAD"], root=root, env=env).stdout.strip()
    require(not invoke(["git", "status", "--porcelain"], root=root, env=env).stdout,
            "native evidence requires a clean source checkout")
    report["environment"] = {"goos": target_os, "goarch": target_arch,
        "system": platform.system(), "machine": platform.machine(), "os_release": platform.release(),
        "uname": invoke(["uname", "-s", "-m"], root=root, env=env).stdout.strip(),
        "shell_version": invoke([bash, "--version"], root=root, env=env).stdout.splitlines()[0],
        "go_version": invoke(["go", "version"], root=root, env=env).stdout.strip()}
    print("RUN: actual native Core fixtures (including ordinary and linked Git worktrees)", flush=True)
    result = invoke(["go", "test", "-json", "-mod=readonly", "-count=1", "-timeout=180s", "-run", "^TestNative",
                     *("./" + p for p in PACKAGES)], root=root, env=env)
    report["native_tests"] = native_results(result.stdout,
        {MODULE + package + "/" + name for package, names in REQUIRED.items() for name in names},
        {MODULE + package for package in PACKAGES}, HELPERS)
    guide_inputs = {name: guide_blocks((root / "guides" / name).read_text(encoding="utf-8"), headings)
                    for name, headings in GUIDES.items()}
    storage_versions = {blocks["version"] for blocks in guide_inputs.values()}
    require(len(storage_versions) == 1 and "0.0.0" not in storage_versions, "guide release versions differ or collide with CI predecessor")
    storage_version = storage_versions.pop()
    source_version = re.findall(r'^var version = "([^"\n]+)"$', (root / "cmd/ha/main.go").read_text(encoding="utf-8"), re.M)
    require(len(source_version) == 1, "source version declaration changed")
    public = public_files(root, env)
    report["candidates"], report["installations"] = {}, []
    report["scope"] = {"transport": "loopback HTTP serving only generated candidate bytes",
        "guide_bindings": "isolated destination, loopback URL, predecessor storage version only",
        "predecessor": "same source with CI-only version stamp; not an older public release",
        "not_observed": ["public release installation", "agent host", "WSL2", "other CPU architectures"]}
    with tempfile.TemporaryDirectory(prefix="jaekit-native-unix-") as temporary:
        directory = Path(temporary).resolve()
        routes = {}
        for label, version, storage in (("predecessor", "0.0.0-ci-predecessor", "0.0.0"),
                                        ("current", source_version[0], storage_version)):
            binary = directory / ("ha-" + label)
            command = ["go", "build", "-mod=readonly", "-trimpath", "-o", binary]
            if label == "predecessor":
                command += ["-ldflags=-X main.version=" + version]
            invoke([*command, "./cmd/ha"], root=root, env=env)
            metadata = identity(binary, env, version, directory)
            name, archive, checksums = make_archive(binary, public, storage, target_os + "_" + target_arch)
            metadata.update(storage_version=storage, archive_sha256=digest(archive), public_data_sha256=public_digest(public))
            report["candidates"][label] = metadata
            routes["/" + label + "/" + name] = archive
            routes["/" + label + "/checksums.txt"] = checksums
            if label == "current":
                routes["/invalid/" + name] = archive
                routes["/invalid/checksums.txt"] = ("0" * 64 + "  " + name + "\n").encode()
        require(report["candidates"]["predecessor"]["binary_sha256"] != report["candidates"]["current"]["binary_sha256"],
                "update candidate is identical to predecessor")
        with local_transport(routes) as base_url:
            for name, blocks in guide_inputs.items():
                print("RUN: real current-source candidate lifecycle via " + name, flush=True)
                lifecycle(directory, name, blocks, report["candidates"], base_url, bash, env, report["installations"])
    require(invoke(["git", "rev-parse", "HEAD"], root=root, env=env).stdout.strip() == report["source_commit"]
            and not invoke(["git", "status", "--porcelain"], root=root, env=env).stdout,
            "source commit or checkout changed during native observation")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", type=Path, required=True, help="new JSON output outside the checkout")
    args = parser.parse_args()
    report = {"schema": "jaekit-native-unix/v1", "status": "failed"}
    code, destination = 1, None
    try:
        path = args.report.absolute()
        require(not path.is_relative_to(ROOT.resolve()) and not path.parent.resolve().is_relative_to(ROOT.resolve()),
                "report must be outside the checkout")
        destination = path.open("x", encoding="utf-8")
        observe(report)
        report["status"], code = "passed", 0
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        report["error"] = str(error)
        print("FAIL: Unix native checks: " + str(error), file=sys.stderr, flush=True)
    except KeyboardInterrupt:
        report["error"], code = "interrupted", 130
    serialized = json.dumps(report, sort_keys=True, ensure_ascii=True)
    if destination is not None:
        try:
            with destination:
                destination.write(serialized + "\n")
        except OSError as error:
            report.update(status="failed", error="report output failed: " + str(error))
            serialized, code = json.dumps(report, sort_keys=True, ensure_ascii=True), 1
    print(PREFIX + serialized, flush=True)
    return code


if __name__ == "__main__":
    raise SystemExit(main())
