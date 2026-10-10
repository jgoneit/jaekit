#!/usr/bin/env python3
"""Check release files before upload, optionally exercise the native archive.

No download, installation, upload, tag changes, or writes to the source tree.
Native smoke files live in a temporary directory and use only packaged inputs.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile

TARGETS = ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")
REQUIRED = {
    "ha", "LICENSE", "README.md", "README.en.md",
    "guides/INSTALL.md", "guides/INSTALL.en.md", "guides/USAGE.md",
    "guides/USAGE.en.md", "guides/OPERATIONS.md",
    "contracts/README.md", "contracts/bundle.md", "contracts/goal-docs.md",
    "contracts/run-record.md", "contracts/role-card.md", "contracts/check-result.md",
    "contracts/core-capabilities.md",
    "assets/readme/flow.ko.svg", "assets/readme/flow.en.svg",
    "assets/readme/example.ko.svg", "assets/readme/example.en.svg",
    "assets/readme/SOURCES.md", "assets/readme/SOURCES.en.md",
    "assets/readme/generate-flow.py", "assets/readme/generate-example.py",
    "tools/check-result-reference.py",
    "examples/check-result/README.md", "examples/check-result/declaration.json",
    "examples/check-result/reference.json", "examples/check-result/input.txt",
}
ROOT_FILES = {"ha", "LICENSE", "README.md", "README.en.md"}
ROOT_DIRECTORIES = {"guides", "contracts", "assets", "tools", "examples"}
EXAMPLE_FILES = {name for name in REQUIRED if name.startswith("examples/")}


def require(ok: bool, message: str):
    if not ok:
        raise ValueError(message)


def release_names(version: str):
    require(re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) is not None,
            "version must be a release number such as 0.1.3")
    return {f"ha_{version}_{target}.tar.gz" for target in TARGETS}


def digest(path: Path):
    result = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def checksum_entries(text: str):
    entries = {}
    for line in text.splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  (ha_[^/\\]+\.tar\.gz)", line)
        require(match is not None, "checksums.txt contains an invalid line")
        checksum, name = match.groups()
        require(name not in entries, "checksums.txt repeats an archive")
        entries[name] = checksum
    return entries


def archive_files(archive: tarfile.TarFile, prefix: str):
    files, names = {}, set()
    for member in archive.getmembers():
        name = member.name.rstrip("/")
        path = PurePosixPath(name)
        require(name and not path.is_absolute() and ".." not in path.parts
                and str(path) == name and "\\" not in name,
                "archive has an unsafe or noncanonical path")
        require(name not in names, "archive repeats a path")
        names.add(name)
        require(path.parts[0] == prefix, "archive has an unexpected root directory")
        require(member.isdir() or member.isfile(), "archive contains a link or special file")
        require(member.mode & 0o7000 == 0, "archive contains special permission bits")
        if len(path.parts) == 1:
            require(member.isdir(), "archive root is not a directory")
            continue
        relative = PurePosixPath(*path.parts[1:])
        top = relative.parts[0]
        require(top in ROOT_DIRECTORIES or (len(relative.parts) == 1 and top in ROOT_FILES),
                "archive contains a file outside the release allowlist")
        require(not {".git", "__pycache__", "node_modules"}.intersection(relative.parts)
                and relative.suffix not in {".pyc", ".pyo"},
                "archive contains local metadata or generated cache")
        if top == "tools":
            require(str(relative) in {"tools", "tools/check-result-reference.py"},
                    "archive contains an unexpected tool")
        if top == "examples":
            require(str(relative) in EXAMPLE_FILES | {"examples", "examples/check-result"},
                    "archive contains an unexpected example file")
        if member.isfile():
            files[str(relative)] = member
    require(REQUIRED <= files.keys(), "archive is missing required files: " + ", ".join(sorted(REQUIRED - files.keys())))
    require(files["ha"].mode & 0o111 != 0, "archive Core is not executable")
    return files


def check_platform(archive: tarfile.TarFile, binary: tarfile.TarInfo, target: str):
    with archive.extractfile(binary) as source:
        header = source.read(64)
    require(len(header) >= 20, "packaged Core has no supported executable header")
    system, arch = target.split("_")
    if system == "linux":
        require(header[:6] == b"\x7fELF\x02\x01" and struct.unpack_from("<H", header, 18)[0] == {"amd64": 62, "arm64": 183}[arch],
                "packaged Core platform differs from archive name: " + target)
    else:
        require(header[:4] == b"\xcf\xfa\xed\xfe" and struct.unpack_from("<I", header, 4)[0] == {"amd64": 0x1000007, "arm64": 0x100000C}[arch],
                "packaged Core platform differs from archive name: " + target)


def check(dist: Path, version: str, published_assets: Path | None = None):
    names = release_names(version)
    require(dist.is_dir(), "release output directory does not exist")
    actual = {path.name for path in dist.iterdir()}
    require(actual == names | {"checksums.txt"}, "release output must contain exactly four archives and checksums.txt")
    for name in actual:
        require((dist / name).is_file() and not (dist / name).is_symlink(), "release output must contain regular files")
    checksums = checksum_entries((dist / "checksums.txt").read_text(encoding="utf-8"))
    require(checksums.keys() == names, "checksum file does not name exactly the four release archives")
    for name in sorted(names):
        require(digest(dist / name) == checksums[name], "checksum mismatch: " + name)
        with tarfile.open(dist / name, "r:gz") as archive:
            files = archive_files(archive, name.removesuffix(".tar.gz"))
            check_platform(archive, files["ha"], name.removeprefix("ha_" + version + "_").removesuffix(".tar.gz"))
    if published_assets is not None:
        uploaded = published_assets.read_text(encoding="utf-8").splitlines()
        require(len(uploaded) == len(set(uploaded)) and set(uploaded) == actual,
                "published release asset names differ from the validated local files")
    return sorted(names)


def invoke(argv, cwd: Path, env=None, expected=0):
    result = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, timeout=20)
    require(result.returncode == expected,
            f"packaged {Path(argv[0]).name} exited {result.returncode}, expected {expected}")
    return result.stdout


def native_smoke(dist: Path, version: str):
    systems = {"Linux": "linux", "Darwin": "darwin"}
    architectures = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}
    system, arch = systems.get(platform.system()), architectures.get(platform.machine().lower())
    require(system and arch, "native smoke supports macOS/Linux on amd64/arm64")
    target = f"{system}_{arch}"
    prefix = f"ha_{version}_{target}"
    with tempfile.TemporaryDirectory(prefix="jaekit-release-smoke-") as directory:
        root = Path(directory)
        package = root / "package"
        with tarfile.open(dist / (prefix + ".tar.gz"), "r:gz") as archive:
            files = archive_files(archive, prefix)
            for name, member in files.items():
                destination = package / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                with archive.extractfile(member) as source, destination.open("xb") as output:
                    shutil.copyfileobj(source, output)
                destination.chmod(member.mode & 0o777)
        binary = str(package / "ha")
        require(invoke([binary, "--version"], root).strip() == "ha " + version,
                "packaged Core version does not match the release")
        capabilities = json.loads(invoke([binary, "capabilities", "--format", "json"], root))
        require(isinstance(capabilities, dict), "packaged Core capabilities are not an object")
        require(capabilities.get("schema") == "ha-capabilities/v1" and capabilities.get("ha_version") == version
                and capabilities.get("default_rules") == "run-rules/3"
                and {"run-rules/1", "run-rules/2", "run-rules/3"} <= set(capabilities.get("supported_rules", [])),
                "packaged Core does not expose the release capability contract")
        require({"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1"}
                <= set(capabilities.get("features", [])), "packaged Core lacks Seal's required features")
        for kind, formats in {"criteria": {"legacy", "nested/1"}, "check_declaration": {"check-declaration/v1"},
                              "check_result": {"check-result/v1"}, "check_evidence": {"check-evidence/v1"}}.items():
            require(formats <= set(capabilities.get("formats", {}).get(kind, [])), "packaged Core lacks a required format: " + kind)
        project = root / "project"
        for source, destination in (
            ("tools/check-result-reference.py", "tools/check-result-reference.py"),
            ("examples/check-result/declaration.json", "checks/declaration.json"),
            ("examples/check-result/reference.json", "checks/reference.json"),
            ("examples/check-result/input.txt", "sample/input.txt"),
        ):
            path = project / destination
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(package / source, path)
        declaration = json.loads((project / "checks/declaration.json").read_text(encoding="utf-8"))
        config = json.loads((project / "checks/reference.json").read_text(encoding="utf-8"))
        require(set(declaration["producer_paths"]) == {"tools/check-result-reference.py", "checks/reference.json"},
                "packaged example does not bind the copied producer and configuration")
        declared = {item["id"]: item["violation"] for item in declaration["targets"]}
        require(declared and {item["target"] for item in config["checks"]} == declared.keys(),
                "packaged example configuration does not match its declared targets")
        for state, expected in (("violation", 1), ("pass", 0)):
            if state == "pass":
                for item in config["checks"]:
                    require(item["path"] == "sample/input.txt", "unexpected packaged example input path")
                    (project / item["path"]).write_text(item["equals"], encoding="utf-8")
            report_path = root / (state + ".json")
            invocation = "release-smoke-" + state
            binding = digest(project / "checks/declaration.json")
            env = dict(os.environ, HA_EVIDENCE_PATH=str(report_path), HA_EVIDENCE_INVOCATION=invocation,
                       HA_EVIDENCE_DECLARATION_DIGEST=binding, PYTHONDONTWRITEBYTECODE="1")
            invoke([sys.executable, "tools/check-result-reference.py", "checks/declaration.json", "checks/reference.json"],
                   project, env, expected)
            report = json.loads(report_path.read_text(encoding="utf-8"))
            require(report.get("schema") == "check-result/v1" and report.get("invocation") == invocation
                    and report.get("declaration_digest") == binding and report.get("attempts_complete") is True,
                    "packaged producer did not bind its actual invocation")
            observations = report.get("observations", [])
            require(len(observations) == len(declared) and {item.get("target") for item in observations} == declared.keys(),
                    "packaged producer did not observe the declared targets")
            for observation in observations:
                require(observation.get("attempt") == 1 and observation.get("status") == state,
                        "packaged producer did not observe " + state)
                if state == "violation":
                    require(observation.get("violation") == declared[observation["target"]],
                            "packaged producer reported an unintended violation")
        # This exercises packaged files, not a model's behavior or ha completion.
    return target


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", required=True, type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--run-native", action="store_true")
    parser.add_argument("--published-assets", type=Path)
    args = parser.parse_args()
    try:
        check(args.dist, args.version, args.published_assets)
        target = native_smoke(args.dist, args.version) if args.run_native else None
    except (OSError, ValueError, KeyError, TypeError, tarfile.TarError, subprocess.TimeoutExpired) as error:
        print("FAIL: " + str(error), file=sys.stderr)
        return 1
    suffix = f"; native {target} Core and reference violation/pass" if target else ""
    print("PASS: four release archives, checksums and required public files" + suffix)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
