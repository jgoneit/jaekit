#!/usr/bin/env python3
"""Run the public Windows blocks from the guides against local release files.

Modes:
  ci       Windows Server CI. Checks the exact release archive with the
           product's own package verification in one shell, then pastes the
           public install/remove blocks into that shell: with the real archive
           (the installer must stop at its Windows 11 guard) and with probe
           archives whose installer code only records how it was started.
  serve    Windows 11 check of one candidate release directory: serves its
           files on 127.0.0.1 and prints the blocks from the candidate's own
           guides, bound to that source, for a person to paste
           (tools/windows/README.md).
  version  Print the version the checkout's public blocks install.

Only the block's download source and version are rebound; the block text is
otherwise executed exactly as written in guides/WINDOWS.md and WINDOWS.en.md.
In CI the parent shell runs it as a command (as a paste does), with the
persistent CurrentUser execution policy set to Restricted and no inherited
Process policy.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import zipfile

ROOT = Path(__file__).resolve().parents[2]
GUIDES = ("guides/WINDOWS.md", "guides/WINDOWS.en.md")
LABELS = {"install": "PUBLIC INSTALL", "remove": "PUBLIC REMOVE"}
SOURCE = "'https://github.com/jgoneit/jaekit/releases/download/v'"
VERSION_LINE = re.compile(r"\$version = '([0-9]+\.[0-9]+\.[0-9]+)'")
REPORT = "JAEKIT_BLOCK_REPORT "
SCHEMA = "jaekit-windows-public-blocks/v1"
GUARD = "Windows 11 client is required"
POLICY_FAILURES = ("running scripts is disabled", "cannot be loaded", "not digitally signed")
READY = "is ready in this window"
# What PowerShell reports when the effective policy stops a script file.
SCRIPTS_BLOCKED = "UnauthorizedAccess"
REMOVED = "Removal finished"


class Failure(Exception):
    pass


def require(value, message):
    if not value:
        raise Failure(message)


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


# ---------------------------------------------------------------- blocks

def block(text: str, label: str) -> str:
    start, end = f"<!-- BEGIN WINDOWS {label} -->", f"<!-- END WINDOWS {label} -->"
    require(text.count(start) == 1 and text.count(end) == 1 and text.index(start) < text.index(end),
            f"expected one {label} block")
    body = text.split(start, 1)[1].split(end, 1)[0]
    match = re.fullmatch(r"\s*```powershell\n(.+?)\n```\s*", body, re.DOTALL)
    require(match is not None, f"invalid {label} block")
    return match[1]


def public_blocks(root: Path = ROOT) -> dict[str, str]:
    return blocks_from([(root / guide).read_text(encoding="utf-8") for guide in GUIDES])


def blocks_from(texts: list[str]) -> dict[str, str]:
    """Both guides must carry byte-identical blocks; return them by kind."""
    result = {}
    for kind, label in LABELS.items():
        found = [block(text, label) for text in texts]
        require(found[0] == found[1], f"the Korean and English {label} blocks differ")
        result[kind] = found[0]
    return result


def block_version(found: dict[str, str]) -> str:
    """The one version the install and remove blocks set."""
    versions = set()
    for code in found.values():
        values = VERSION_LINE.findall(code)
        require(len(values) == 1, "each block must set $version exactly once")
        versions.add(values[0])
    require(len(versions) == 1, "the install and remove blocks set different versions")
    return versions.pop()


def bind(code: str, base_url: str, version: str) -> str:
    """Replace only the release source and the version; everything else stays."""
    require(code.count(SOURCE) == 1, "the block must name the release source exactly once")
    require(len(VERSION_LINE.findall(code)) == 1, "the block must set $version exactly once")
    require(re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) is not None, "version must look like 0.1.5")
    code = code.replace(SOURCE, "'" + base_url.rstrip("/") + "/v'")
    return VERSION_LINE.sub(lambda _: f"$version = '{version}'", code)


def wrapper(code: str, probe: Path) -> str:
    """A paste in a console, plus an observer that reports after the block ends.

    The observer avoids the Security module: PowerShell 7 does not load its
    type data under Restricted. Whether the window may run a script file is
    observed by trying one before and after the block.
    """
    literal = str(probe).replace("'", "''")
    blocked = ("try {{ $null = & '{0}'; ${1} = 'ran' }} catch {{ ${1} = [string]$_.FullyQualifiedErrorId }}")
    return "\n".join([
        "$ErrorActionPreference = 'Stop'",
        "$jaekitPathBefore = $env:Path",
        "$jaekitProcessPolicy = [string]$env:PSExecutionPolicyPreference",
        blocked.format(literal, "jaekitScriptsBefore"),
        "$jaekitStatus = 'ok'",
        "$jaekitError = ''",
        "try {",
        code,
        "} catch { $jaekitStatus = 'error'; $jaekitError = [string]$_ }",
        blocked.format(literal, "jaekitScriptsAfter"),
        "$jaekitFound = @(Get-Command ha -ErrorAction SilentlyContinue)",
        "$jaekitHa = $null",
        "if ($jaekitFound.Count -gt 0) { $jaekitHa = [ordered]@{ type = [string]$jaekitFound[0].CommandType; "
        "source = [string]$jaekitFound[0].Source; version = '' }",
        "  if ($jaekitHa.type -eq 'Application') { $jaekitHa.version = [string](& $jaekitFound[0].Source --version) } }",
        "$jaekitReport = [ordered]@{ status = $jaekitStatus; error = $jaekitError; path_before = $jaekitPathBefore; "
        "path_after = [string]$env:Path; shell = [string](Get-Process -Id $PID).Path; "
        "edition = [string]$PSVersionTable.PSEdition; ps_version = [string]$PSVersionTable.PSVersion; "
        "process_policy = $jaekitProcessPolicy; scripts_before = $jaekitScriptsBefore; "
        "scripts_after = $jaekitScriptsAfter; ha = $jaekitHa }",
        f"[Console]::Out.WriteLine('{REPORT}' + ($jaekitReport | ConvertTo-Json -Compress -Depth 4))",
    ]) + "\n"


# ---------------------------------------------------------------- archives

PROBE = r"""param([string]$Archive, [string]$Checksums, [switch]$AddToPath)
$record = [ordered]@{ script = '__SCRIPT__'; shell = [string](Get-Process -Id $PID).Path;
  edition = [string]$PSVersionTable.PSEdition; policy = [string](Get-ExecutionPolicy);
  process_policy = [string](Get-ExecutionPolicy -Scope Process);
  current_user_policy = [string](Get-ExecutionPolicy -Scope CurrentUser);
  archive = $Archive; checksums = $Checksums; add_to_path = [bool]$AddToPath;
  archive_exists = [bool]($Archive -and (Test-Path -LiteralPath $Archive));
  checksums_exists = [bool]($Checksums -and (Test-Path -LiteralPath $Checksums));
  common_present = (Test-Path -LiteralPath (Join-Path $PSScriptRoot 'common.ps1')) }
[IO.File]::WriteAllText($env:JAEKIT_PROBE_MARKER, ($record | ConvertTo-Json -Compress))
$code = [int]$env:JAEKIT_PROBE_EXIT
if ($code -eq 0 -and '__SCRIPT__' -eq 'install') {
  $bin = Join-Path (Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Jaekit') 'bin'
  [void][IO.Directory]::CreateDirectory($bin)
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
  try { [IO.Compression.ZipFileExtensions]::ExtractToFile($zip.GetEntry('ha.exe'), (Join-Path $bin 'ha.exe'), $true) }
  finally { $zip.Dispose() }
}
if ($code -ne 0) { [Console]::Error.WriteLine('probe stopped with ' + $code) }
exit $code
"""


def probe_archive(real: bytes) -> bytes:
    """The real archive with installer and uninstaller replaced by recorders."""
    out = io.BytesIO()
    replaced = {"tools/windows/install.ps1": PROBE.replace("__SCRIPT__", "install").encode(),
                "tools/windows/uninstall.ps1": PROBE.replace("__SCRIPT__", "uninstall").encode()}
    with zipfile.ZipFile(io.BytesIO(real)) as source, zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as target:
        names = {info.filename for info in source.infolist()}
        require(set(replaced) <= names, "the release archive has no installer scripts to replace")
        for info in source.infolist():
            target.writestr(info, replaced.get(info.filename) or source.read(info))
    return out.getvalue()


def checksum_lines(name: str, data: bytes, variant: str = "valid") -> bytes:
    line = f"{sha256(data)}  {name}\n"
    other = "0" * 64 + "  ha_0.0.0_linux_amd64.tar.gz\n"
    return {"valid": other + line, "missing": other, "duplicate": other + line + line,
            "mismatch": other + f"{'1' * 64}  {name}\n",
            # The right digest, but for another file whose name ends like it.
            "foreign": other + f"{sha256(data)}  other  {name}\n"}[variant].encode()


def release_files(directory: Path) -> tuple[str, bytes, bytes, str]:
    """Return (name, archive, checksums, version) of one release directory."""
    archives = sorted(directory.glob("ha_*_windows_amd64.zip"))
    require(len(archives) == 1 and (directory / "checksums.txt").is_file(),
            f"{directory} must hold one Windows archive and checksums.txt")
    name = archives[0].name
    version = name.removeprefix("ha_").removesuffix("_windows_amd64.zip")
    data, checksums = archives[0].read_bytes(), (directory / "checksums.txt").read_bytes()
    listed = [line for line in checksums.decode("ascii").splitlines() if line.endswith("  " + name)]
    require(listed == [f"{sha256(data)}  {name}"], f"checksums.txt does not list {name} with its digest once")
    return name, data, checksums, version


# ---------------------------------------------------------------- serving

class Server:
    """Serve fixed bytes at fixed paths on 127.0.0.1; everything else is 404."""

    def __init__(self):
        self.routes: dict[str, bytes] = {}
        routes = self.routes

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                body = routes.get(self.path)
                self.send_response(200 if body is not None else 404)
                self.send_header("Content-Length", str(len(body or b"")))
                self.end_headers()
                self.wfile.write(body or b"")

            def log_message(self, *_args):
                pass

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_exc):
        self.httpd.shutdown()
        self.httpd.server_close()

    def publish(self, case: str, version: str, files: dict[str, bytes]) -> str:
        base = f"http://127.0.0.1:{self.httpd.server_port}/{case}"
        for name, data in files.items():
            self.routes[f"/{case}/v{version}/{name}"] = data
        return base


# ---------------------------------------------------------------- shells

def shell_path(name: str) -> str:
    if name == "powershell":
        path = Path(os.environ.get("SystemRoot", r"C:\Windows")) / "System32/WindowsPowerShell/v1.0/powershell.exe"
        require(path.is_file(), "Windows PowerShell 5.1 is not available")
        return str(path)
    found = shutil.which("pwsh")
    require(found is not None, "PowerShell 7 (pwsh) is not available")
    return found


def base_env() -> dict[str, str]:
    """The runner's environment without an inherited Process policy or module
    path, so each edition starts like a new window. Windows names ignore case."""
    drop = {"PSEXECUTIONPOLICYPREFERENCE", "PSMODULEPATH"}
    return {key: value for key, value in os.environ.items() if key.upper() not in drop}


def command(shell: str, text: str, env: dict[str, str], timeout: int = 300, bypass: bool = False):
    args = [shell, "-NoProfile", "-NonInteractive"]
    if bypass:
        args += ["-ExecutionPolicy", "Bypass"]
    encoded = base64.b64encode(text.encode("utf-16-le")).decode("ascii")
    result = subprocess.run(args + ["-EncodedCommand", encoded], env=env, capture_output=True,
                            timeout=timeout, encoding="utf-8", errors="replace")
    return result.returncode, result.stdout, result.stderr


def policies(shell: str, env: dict[str, str]) -> dict[str, str]:
    """Each policy scope, read by a separate process the persistent policy cannot stop."""
    text = ("Get-ExecutionPolicy -List | ForEach-Object { "
            "[Console]::Out.WriteLine([string]$_.Scope + '=' + [string]$_.ExecutionPolicy) }")
    code, out, err = command(shell, text, env, bypass=True)
    found = dict(line.strip().split("=", 1) for line in out.splitlines() if "=" in line)
    require(code == 0 and "CurrentUser" in found, "could not read the execution policies: " + (err or out)[-2000:])
    return found


def set_current_user_policy(shell: str, policy: str, env: dict[str, str]):
    # The helper's own process-scoped Bypass outranks CurrentUser, which
    # PowerShell reports as an override error; the value read back decides.
    command(shell, f"Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy {policy} -Force "
                   "-ErrorAction SilentlyContinue", env, bypass=True)
    found = policies(shell, env).get("CurrentUser")
    require(found == policy, f"could not set the CurrentUser execution policy to {policy} (it is {found})")


def paste(shell: str, code: str, env: dict[str, str], work: Path) -> dict:
    """Run the text as a pasted command (Invoke-Expression), not a script file."""
    source, probe = work / "paste.txt", work / "observer-probe.ps1"
    probe.write_text("'observer probe ran'\n", encoding="utf-8")
    source.write_text(wrapper(code, probe), encoding="utf-8-sig")
    literal = str(source).replace("'", "''")
    result = subprocess.run([shell, "-NoProfile", "-NonInteractive", "-Command",
                             f"Invoke-Expression ([IO.File]::ReadAllText('{literal}'))"],
                            env=env, capture_output=True, timeout=600, encoding="utf-8", errors="replace")
    reports = [line[len(REPORT):] for line in result.stdout.splitlines() if line.startswith(REPORT)]
    require(len(reports) == 1, "the pasted block produced no observer report: " + result.stderr[-2000:])
    observed = json.loads(reports[0])
    observed["exit_code"] = result.returncode
    observed["output"] = "\n".join(line for line in result.stdout.splitlines() if not line.startswith(REPORT))[-6000:]
    observed["errors"] = result.stderr[-6000:]
    return observed


def path_key(env: dict[str, str]) -> str:
    """Windows environment names ignore case; reuse the existing PATH key."""
    return next((key for key in env if key.upper() == "PATH"), "PATH")


def local_appdata() -> Path:
    return Path(os.environ["LOCALAPPDATA"])


def install_bin() -> Path:
    return local_appdata() / "Jaekit" / "bin"


# ---------------------------------------------------------------- CI cases

def smoke(shell: str, env: dict[str, str], directory: Path, name: str, version: str, work: Path) -> dict:
    """The installer's own package check on the exact archive, then the Core."""
    common = str(ROOT / "tools/windows/common.ps1").replace("'", "''")
    destination = work / "smoke"
    text = "\n".join([
        f". '{common}'",
        f"$package = Expand-CheckedPackage '{directory / name}' '{directory / 'checksums.txt'}' '{destination}'",
        f"$exe = Join-Path '{destination}' 'ha.exe'",
        "[Console]::Out.WriteLine('JAEKIT_SMOKE_VERSION ' + [string](& $exe --version))",
        "[Console]::Out.WriteLine('JAEKIT_SMOKE_CAPABILITIES ' + [string](& $exe capabilities --format json))",
        "[Console]::Out.WriteLine('JAEKIT_SMOKE_MANIFEST ' + [string]$package.manifest.version)",
    ])
    code, out, err = command(shell, text, env, bypass=True)
    values = {}
    for line in out.splitlines():
        for key in ("VERSION", "CAPABILITIES", "MANIFEST"):
            if line.startswith(f"JAEKIT_SMOKE_{key} "):
                values[key] = line.split(" ", 1)[1]
    capabilities = json.loads(values.get("CAPABILITIES") or "{}")
    observed = {"exit_code": code, "version_output": values.get("VERSION", ""),
                "manifest_version": values.get("MANIFEST", ""),
                "capabilities_version": capabilities.get("ha_version"),
                "default_rules": capabilities.get("default_rules"), "errors": err[-2000:]}
    observed["passed"] = (code == 0 and observed["version_output"] == "ha " + version
                          and observed["manifest_version"] == version and observed["capabilities_version"] == version
                          and observed["default_rules"] == "run-rules/3")
    return observed


def judge(case: dict, shell: str) -> list[str]:
    """Problems with one CI case; an empty list means the expectation held."""
    expect, observed, problems = case["expect"], case["observed"], []
    text = (observed.get("output", "") + "\n" + observed.get("errors", "") + "\n" + observed.get("error", "")).lower()
    marker = case.get("marker")

    def check(ok, message):
        if not ok:
            problems.append(message)

    check(observed.get("shell", "").lower() == shell.lower(), "the block did not run in the selected shell")
    check(observed.get("process_policy") == "", "the parent inherited a Process policy")
    check(observed.get("scripts_before") == SCRIPTS_BLOCKED, "the parent could run a script file before the block")
    check(observed.get("scripts_after") == SCRIPTS_BLOCKED, "the parent could run a script file after the block")
    check(observed.get("status") == expect["status"], f"block status {observed.get('status')}, expected {expect['status']}")
    check(case.get("temporary_left") == [], "the block left temporary files: " + repr(case.get("temporary_left")))
    if expect["status"] == "error" or expect.get("path") == "unchanged":
        check(observed.get("path_after") == observed.get("path_before"), "the parent PATH changed")
    if expect.get("path") == "appended":
        before, bin_path = observed.get("path_before", ""), str(install_bin())
        joined = before + ("" if not before or before.endswith(";") else ";") + bin_path
        check(observed.get("path_after") == joined, "the parent PATH was not appended exactly once")
    if expect.get("ha"):
        ha = observed.get("ha") or {}
        check(ha.get("type") == "Application" and ha.get("source", "").lower() == str(install_bin() / "ha.exe").lower(),
              "ha does not resolve to the installation in the same window")
    for phrase in expect.get("contains", ()):
        check(phrase.lower() in text, "missing output: " + phrase)
    for phrase in expect.get("absent", ()):
        check(phrase.lower() not in text, "unexpected output: " + phrase)
    if expect.get("child") == "none":
        check(marker is None, "code from the archive ran although it should not")
    elif expect.get("child") == "ran":
        check(marker is not None, "the archive's code did not run")
        if marker is not None:
            check(marker.get("shell", "").lower() == shell.lower(), "the child used another PowerShell executable")
            check(marker.get("edition") == observed.get("edition"), "the child used another PowerShell edition")
            check(marker.get("policy") == "Bypass" and marker.get("process_policy") == "Bypass",
                  "the child did not run with a process-scoped Bypass")
            check(marker.get("current_user_policy") == "Restricted", "the child saw a changed CurrentUser policy")
            check(marker.get("common_present") is True, "the child ran without the extracted helper")
            check(marker.get("script") == expect["script"], "the wrong archive script ran")
            if expect["script"] == "install":
                check(marker.get("add_to_path") is True and marker.get("archive_exists") and marker.get("checksums_exists"),
                      "the installer was not given the checked archive, checksums and -AddToPath")
    return problems


def run_case(server, shell, env, work, name, kind, code, version, files, expect, extra_env=None):
    case_dir = work / name
    temporary = case_dir / "temp"
    temporary.mkdir(parents=True)
    marker = case_dir / "marker.json"
    child_env = dict(env, TEMP=str(temporary), TMP=str(temporary), JAEKIT_PROBE_MARKER=str(marker),
                     JAEKIT_PROBE_EXIT="0", NO_PROXY="127.0.0.1", no_proxy="127.0.0.1")
    child_env.update(extra_env or {})
    base = server.publish(name, version, files)
    observed = paste(shell, bind(code, base, version), child_env, case_dir)
    case = {"name": name, "block": kind, "expect": expect, "observed": observed,
            "marker": json.loads(marker.read_text(encoding="utf-8-sig")) if marker.exists() else None,
            "temporary_left": sorted(path.name for path in temporary.iterdir())}
    case["problems"] = judge(case, shell)
    case["passed"] = not case["problems"]
    print(("PASS " if case["passed"] else "FAIL ") + name + "".join("\n  - " + p for p in case["problems"]), flush=True)
    jaekit = local_appdata() / "Jaekit"
    if jaekit.exists():
        shutil.rmtree(jaekit)
    return case


def ci(args) -> int:
    shell = shell_path(args.shell)
    directory = Path(args.dist).resolve()
    name, real, checksums, version = release_files(directory)
    require(version == args.version, "the release files are for another version")
    require(sha256(real) == args.expected_sha256, "the downloaded archive differs from the built archive")
    blocks = public_blocks()
    env = base_env()
    require(not (local_appdata() / "Jaekit").exists(), "an existing Jaekit installation is on this runner")
    report = {"schema": SCHEMA, "mode": "ci", "shell": args.shell, "shell_path": shell, "version": version,
              "archive": {"name": name, "sha256": sha256(real)}, "expected_sha256": args.expected_sha256,
              "block_sha256": {kind: sha256(code.encode()) for kind, code in blocks.items()},
              "blocks_identical": True, "cases": []}
    original = policies(shell, env)["CurrentUser"]
    report["original_current_user_policy"] = original
    probe = probe_archive(real)
    with tempfile.TemporaryDirectory(prefix="jaekit-public-blocks-") as temp, Server() as server:
        work = Path(temp)
        report["smoke"] = smoke(shell, env, directory, name, version, work)
        try:
            set_current_user_policy(shell, "Restricted", env)
            real_files = {name: real, "checksums.txt": checksums}
            probe_files = {name: probe, "checksums.txt": checksum_lines(name, probe)}
            failing = ["The installer stopped with exit code"]
            cases = [
                ("real-install", "install", real_files, {"status": "error", "child": "real",
                 "contains": ["Running the verified installer in", GUARD] + failing, "absent": list(POLICY_FAILURES)}, None),
                ("real-remove", "remove", real_files, {"status": "error", "child": "real",
                 "contains": ["Running the verified uninstaller in", GUARD, "The uninstaller stopped with exit code"],
                 "absent": list(POLICY_FAILURES)}, None),
                ("probe-install", "install", probe_files, {"status": "ok", "child": "ran", "script": "install",
                 "path": "appended", "ha": True, "contains": [READY]}, None),
                ("probe-install-path-present", "install", probe_files, {"status": "ok", "child": "ran",
                 "script": "install", "path": "unchanged", "ha": True, "contains": [READY]},
                 {path_key(env): env.get(path_key(env), "") + ";" + str(install_bin())}),
                ("probe-install-fails", "install", probe_files, {"status": "error", "child": "ran", "script": "install",
                 "contains": ["The installer stopped with exit code 3"], "absent": [READY]}, {"JAEKIT_PROBE_EXIT": "3"}),
                ("checksum-missing", "install", {name: probe, "checksums.txt": checksum_lines(name, probe, "missing")},
                 {"status": "error", "child": "none", "contains": ["exactly once"],
                  "absent": ["Running the verified installer"]}, None),
                ("checksum-mismatch", "install", {name: probe, "checksums.txt": checksum_lines(name, probe, "mismatch")},
                 {"status": "error", "child": "none", "contains": ["SHA-256 mismatch"],
                  "absent": ["Running the verified installer"]}, None),
                ("checksum-foreign-name", "install", {name: probe, "checksums.txt": checksum_lines(name, probe, "foreign")},
                 {"status": "error", "child": "none", "contains": ["exactly once"],
                  "absent": ["Running the verified installer"]}, None),
                ("download-missing", "install", {"checksums.txt": checksum_lines(name, probe)},
                 {"status": "error", "child": "none", "absent": ["Running the verified installer"]}, None),
                ("probe-remove", "remove", probe_files, {"status": "ok", "child": "ran", "script": "uninstall",
                 "path": "unchanged", "contains": [REMOVED]}, None),
                ("probe-remove-fails", "remove", probe_files, {"status": "error", "child": "ran", "script": "uninstall",
                 "contains": ["The uninstaller stopped with exit code 4"], "absent": [REMOVED]}, {"JAEKIT_PROBE_EXIT": "4"}),
            ]
            # The remove block verifies downloads with the install block's text
            # (test_public_blocks.py), so those failures run with the install block only.
            for case_name, kind, files, expect, extra in cases:
                report["cases"].append(run_case(server, shell, env, work, case_name, kind, blocks[kind], version,
                                                files, expect, extra))
        except Exception as error:
            report["error"] = f"{type(error).__name__}: {error}"
            raise
        finally:
            try:
                set_current_user_policy(shell, original, env)
                report["restored_current_user_policy"] = policies(shell, env)["CurrentUser"]
            finally:
                report["passed"] = (report["smoke"]["passed"] and "error" not in report
                                    and all(case["passed"] for case in report["cases"])
                                    and report.get("restored_current_user_policy") == original)
                write_report(Path(args.report), report)
    print(("PASS" if report["smoke"]["passed"] else "FAIL") + " smoke of " + name)
    return 0 if report["passed"] else 1


def write_report(path: Path, report: dict):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")


# ---------------------------------------------------------------- serve

def candidate(directory: Path):
    """One candidate's files and the blocks of the guides inside its archive.

    Those blocks are what that release publishes, so they must install its
    version.
    """
    name, data, checksums, version = release_files(directory)
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        names = set(archive.namelist())
        require(all(guide in names for guide in GUIDES), "the candidate archive has no Windows guides")
        found = blocks_from([archive.read(guide).decode("utf-8") for guide in GUIDES])
    shipped = block_version(found)
    require(shipped == version, f"the candidate's guides install {shipped}, not {version}")
    return name, data, checksums, version, found


def serve(args) -> int:
    """Serve one candidate's files and print its own blocks bound to them.

    A person pastes exactly what that release publishes, with only the
    download source changed. Runs until interrupted.
    """
    name, data, checksums, version, found = candidate(Path(args.dist).resolve())
    with Server() as server:
        base = server.publish("candidate", version, {name: data, "checksums.txt": checksums})
        for kind in ("install", "remove"):
            print(f"---- {kind} block for {name} ----")
            print(bind(found[kind], base, version))
        print(f"When every check in tools/windows/README.md passes, add this line to tools/release/notes/v{version}.md:")
        print(f"Windows-Accepted-SHA256: {sha256(data)}", flush=True)
        stop = threading.Event()
        try:
            # A timed wait lets Ctrl+C through on Windows.
            while not stop.wait(0.5):
                pass
        except KeyboardInterrupt:
            pass
    return 0


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="mode", required=True)
    run = sub.add_parser("ci")
    run.add_argument("--dist", required=True)
    run.add_argument("--version", required=True)
    run.add_argument("--expected-sha256", required=True)
    run.add_argument("--shell", required=True, choices=("powershell", "pwsh"))
    run.add_argument("--report", required=True)
    sub.add_parser("serve").add_argument("--dist", required=True)
    sub.add_parser("version")
    args = parser.parse_args(argv)
    try:
        if args.mode == "version":
            print(block_version(public_blocks()))
            return 0
        return {"ci": ci, "serve": serve}[args.mode](args)
    except (Failure, OSError, ValueError, KeyError, zipfile.BadZipFile, subprocess.SubprocessError) as error:
        print("FAIL: " + str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
