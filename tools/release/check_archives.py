#!/usr/bin/env python3
"""Check release files before upload, optionally exercise the native archive.

The four macOS/Linux archives are always required. The Windows archive is
optional unless --require-windows: a release left without a Windows 11
acceptance link publishes the four archives and their checksums only.

No download, installation, upload, tag changes, or writes to the source tree.
Native smoke files live in a temporary directory and use only packaged inputs.
The tar.gz archives are read only through the shared bounded reader
(archive_reader.py), whose limits are published in
tools/release-checks/README.md. The Windows ZIP has its own bounded in-memory
rule (windows_files), the one the published-release check also applies; it is
no looser than the installer's package check (tools/windows/common.ps1).
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile

TARGETS = ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")
REQUIRED = {
    "ha", "LICENSE", "README.md", "README.en.md",
    "guides/INSTALL.md", "guides/INSTALL.en.md", "guides/USAGE.md",
    "guides/USAGE.en.md", "guides/OPERATIONS.md",
    "contracts/README.md", "contracts/bundle.md", "contracts/goal-docs.md",
    "contracts/run-record.md", "contracts/role-card.md", "contracts/check-result.md",
    "contracts/core-capabilities.md",
    "assets/readme/hero.ko.png", "assets/readme/hero.en.png",
    "assets/readme/brand/logo.png", "assets/readme/brand/logo.svg",
    "assets/readme/brand/wordmark.png", "assets/readme/brand/wordmark.svg",
    "assets/readme/brand/symbol.png", "assets/readme/brand/symbol.svg",
    "assets/readme/source/build_brand.py",
    "assets/readme/usage-flow.ko.png", "assets/readme/usage-flow.en.png",
    "assets/readme/usage-flow.ko.svg", "assets/readme/usage-flow.en.svg",
    "assets/readme/SOURCES.md", "assets/readme/SOURCES.en.md",
    "assets/readme/source/IMAGE-PROMPTS.md", "assets/readme/source/USAGE-FLOW.md",
    "assets/readme/source/build_usage_flow.py", "assets/readme/source/render_usage_flow.cjs",
    "assets/readme/source/usage-flow.layout.json", "assets/readme/source/FONT-LICENSE.txt",
    "assets/readme/source/svg/usage-flow.ko.text.svg", "assets/readme/source/svg/usage-flow.en.text.svg",
    "tools/check-result-reference.py",
    "examples/check-result/README.md", "examples/check-result/declaration.json",
    "examples/check-result/reference.json", "examples/check-result/input.txt",
}
ROOT_FILES = {"ha", "LICENSE", "README.md", "README.en.md"}
ROOT_DIRECTORIES = {"guides", "contracts", "assets", "tools", "examples"}
EXAMPLE_FILES = {name for name in REQUIRED if name.startswith("examples/")}
# Local metadata and generated caches never belong in a release archive.
LOCAL_PARTS = {".git", "__pycache__", "node_modules"}
CACHE_SUFFIXES = {".pyc", ".pyo"}


WINDOWS_SCHEMA = "jaekit-windows-package/v1"
WINDOWS_MAX_ENTRIES = 2048
WINDOWS_MAX_FILE_BYTES = 32 * 1024 * 1024
WINDOWS_MAX_BYTES = 128 * 1024 * 1024
# The installer reads at most this much manifest (common.ps1 Read-PackageManifest).
WINDOWS_MAX_MANIFEST_BYTES = 1024 * 1024
WINDOWS_SHA256 = re.compile(r"[0-9a-f]{64}")
# DOS directory (0x10) and reparse-point (0x400) attributes; the installer refuses both.
WINDOWS_DOS_REFUSED = 0x410
# The fields package.py writes, and nothing else.
WINDOWS_MANIFEST_FIELDS = {"schema", "version", "os", "arch", "files"}
WINDOWS_ENTRY_FIELDS = {"path", "size", "sha256"}


def load_archive_reader():
    path = Path(__file__).resolve().with_name("archive_reader.py")
    spec = importlib.util.spec_from_file_location("jaekit_release_archive_reader", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


archive_reader = load_archive_reader()


def load_windows_package():
    """The Windows package rules (path safety, PE format) have one source."""
    path = Path(__file__).resolve().parents[1] / "windows" / "package.py"
    spec = importlib.util.spec_from_file_location("jaekit_windows_package", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


windows_package = load_windows_package()
# What package.py always packages (the Core, its manifest and the public files
# it requires) and the guides the installer requires (Read-PackageManifest);
# package.public_path() bounds everything else.
WINDOWS_REQUIRED = {"ha.exe", "package.json", "guides/WINDOWS.md", "guides/WINDOWS.en.md",
                    *windows_package.PUBLIC_FILES}
# The same published limits as the acceptance checker; see archive_reader.py.
ARCHIVE_MAX_COMPRESSED_BYTES = archive_reader.MAX_COMPRESSED_BYTES
ARCHIVE_MAX_BYTES = archive_reader.MAX_BYTES
ARCHIVE_MAX_ENTRIES = archive_reader.MAX_ENTRIES
ARCHIVE_MAX_STREAM_BYTES = archive_reader.MAX_STREAM_BYTES
ARCHIVE_MAX_FILE_BYTES = archive_reader.MAX_FILE_BYTES
ARCHIVE_MAX_METADATA_BYTES = archive_reader.MAX_METADATA_BYTES
ARCHIVE_MAX_METADATA_TOTAL_BYTES = archive_reader.MAX_METADATA_TOTAL_BYTES
ARCHIVE_MAX_PAX_KEYS = archive_reader.MAX_PAX_KEYS


def require(ok: bool, message: str):
    if not ok:
        raise ValueError(message)


def release_names(version: str):
    require(re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) is not None,
            "version must be a release number such as 0.1.3")
    return {f"ha_{version}_{target}.tar.gz" for target in TARGETS}


def windows_name(version: str):
    release_names(version)
    return f"ha_{version}_windows_amd64.zip"


def digest(path: Path):
    result = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def checksum_entries(text: str):
    entries = {}
    for line in text.splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  (ha_[^/\\]+\.(?:tar\.gz|zip))", line)
        require(match is not None, "checksums.txt contains an invalid line")
        checksum, name = match.groups()
        require(name not in entries, "checksums.txt repeats an archive")
        entries[name] = checksum
    return entries


def read_archive(path: Path, checksum: str | None = None):
    """Read one archive file once within the compressed limit, then parse it bounded.

    A given checksum is checked against exactly the bytes that are parsed.
    """
    require(path.stat().st_size <= ARCHIVE_MAX_COMPRESSED_BYTES, "archive compressed size limit exceeded")
    with path.open("rb") as source:
        data = source.read(ARCHIVE_MAX_COMPRESSED_BYTES + 1)
    if checksum is not None:
        require(hashlib.sha256(data).hexdigest() == checksum, "checksum mismatch: " + path.name)
    return archive_reader.read(
        data, compressed=ARCHIVE_MAX_COMPRESSED_BYTES, bytes=ARCHIVE_MAX_BYTES, entries=ARCHIVE_MAX_ENTRIES,
        stream=ARCHIVE_MAX_STREAM_BYTES, file=ARCHIVE_MAX_FILE_BYTES, metadata=ARCHIVE_MAX_METADATA_BYTES,
        metadata_total=ARCHIVE_MAX_METADATA_TOTAL_BYTES, pax_keys=ARCHIVE_MAX_PAX_KEYS)


def archive_files(members, prefix: str):
    """Map each packaged file to (member, body); bodies come from the bounded reader."""
    files, names = {}, set()
    for member, body in members:
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
        require(not LOCAL_PARTS.intersection(relative.parts) and relative.suffix not in CACHE_SUFFIXES,
                "archive contains local metadata or generated cache")
        if top == "tools":
            require(str(relative) in {"tools", "tools/check-result-reference.py"},
                    "archive contains an unexpected tool")
        if top == "examples":
            require(str(relative) in EXAMPLE_FILES | {"examples", "examples/check-result"},
                    "archive contains an unexpected example file")
        if member.isfile():
            files[str(relative)] = (member, body)
    require(REQUIRED <= files.keys(), "archive is missing required files: " + ", ".join(sorted(REQUIRED - files.keys())))
    require(files["ha"][0].mode & 0o111 != 0, "archive Core is not executable")
    return files


def check_platform(binary: bytes, target: str):
    header = binary[:64]
    require(len(header) >= 20, "packaged Core has no supported executable header")
    system, arch = target.split("_")
    if system == "linux":
        require(header[:6] == b"\x7fELF\x02\x01" and struct.unpack_from("<H", header, 18)[0] == {"amd64": 62, "arm64": 183}[arch],
                "packaged Core platform differs from archive name: " + target)
    else:
        require(header[:4] == b"\xcf\xfa\xed\xfe" and struct.unpack_from("<I", header, 4)[0] == {"amd64": 0x1000007, "arm64": 0x100000C}[arch],
                "packaged Core platform differs from archive name: " + target)


def windows_manifest(data: bytes):
    """Parse package.json, accepting only the fields package.py writes.

    Python's json is laxer than the installer's ConvertFrom-Json (NaN,
    out-of-range numbers, deep nesting, empty or case-repeated property
    names). With exactly package.py's fields, of their exact types checked
    below, none of that reaches the installer.
    """
    def unique(pairs):
        names = [name for name, _ in pairs]
        require(len(names) == len(set(names)), "Windows archive manifest repeats a property")
        return dict(pairs)

    def constant(value):
        raise ValueError("Windows archive manifest has a non-finite number: " + value)

    try:
        manifest = json.loads(data.decode("utf-8"), object_pairs_hook=unique, parse_constant=constant)
    except (UnicodeError, json.JSONDecodeError, RecursionError):
        raise ValueError("Windows archive manifest is not JSON") from None
    require(isinstance(manifest, dict) and manifest.keys() == WINDOWS_MANIFEST_FIELDS
            and isinstance(manifest["files"], list)
            and all(isinstance(item, dict) and item.keys() == WINDOWS_ENTRY_FIELDS for item in manifest["files"]),
            "Windows archive manifest has fields package.py does not write")
    return manifest


def windows_files(path: Path, checksum: str, version: str):
    """Verify the Windows archive in memory: bounded, manifest-complete and safe.

    The one Windows archive rule, applied before publication and by the
    published-release check. It refuses at least what the installer's own
    checks refuse (common.ps1 Expand-CheckedPackage, Read-PackageManifest) and
    any path package.py would not package; nothing is extracted to disk.
    """
    require(path.stat().st_size <= ARCHIVE_MAX_COMPRESSED_BYTES, "Windows archive compressed size limit exceeded")
    with path.open("rb") as source:
        data = source.read(ARCHIVE_MAX_COMPRESSED_BYTES + 1)
    require(hashlib.sha256(data).hexdigest() == checksum, "checksum mismatch: " + path.name)
    try:
        archive = zipfile.ZipFile(io.BytesIO(data))
    except zipfile.BadZipFile:
        raise ValueError("Windows archive is not a readable ZIP") from None
    files, total = {}, 0
    with archive:
        entries = archive.infolist()
        require(0 < len(entries) <= WINDOWS_MAX_ENTRIES, "Windows archive entry count is out of bounds")
        for entry in entries:
            name = entry.filename
            # zipfile cuts a name at NUL and, on Windows, turns backslashes
            # into slashes; the installer sees the stored name and refuses it.
            require(entry.orig_filename == name, "Windows archive has an unsafe path: " + repr(entry.orig_filename))
            # The installer compares names in a culture-aware PowerShell hashtable
            # and NTFS by its own upper-case table; for non-ASCII text (code
            # pages, normalization forms) and ignorable characters such as DEL,
            # Python cannot reproduce either. package.py's public files are
            # printable ASCII.
            require(name.isascii() and name.isprintable(),
                    "Windows archive has a name that is not printable ASCII: " + repr(name))
            try:
                windows_package.safe_name(name)
            except ValueError:
                raise ValueError("Windows archive has an unsafe path: " + repr(name)) from None
            kind = (entry.external_attr >> 16) & 0o170000
            require(not entry.is_dir() and kind in (0, 0o100000), "Windows archive contains a directory, link or special file")
            require(entry.external_attr & WINDOWS_DOS_REFUSED == 0,
                    "Windows archive entry has a DOS directory or reparse-point attribute: " + name)
            require(entry.flag_bits & 0x1 == 0, "Windows archive contains an encrypted entry")
            require(entry.compress_type in (zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED), "Windows archive uses an unsupported compression")
            require(name not in files, "Windows archive repeats a path")
            require(entry.file_size <= WINDOWS_MAX_FILE_BYTES, "Windows archive file exceeds the size limit")
            total += entry.file_size
            require(total <= WINDOWS_MAX_BYTES, "Windows archive expands beyond the size limit")
            with archive.open(entry) as stream:
                body = stream.read(entry.file_size + 1)
            require(len(body) == entry.file_size, "Windows archive entry size differs from its header")
            files[name] = body
    lowered = {}
    for name in files:
        require(name.lower() not in lowered, "Windows archive has paths that differ only in case")
        lowered[name.lower()] = name
    for name in files:
        # Windows compares parent folders without case, like the file names.
        parts = name.lower().split("/")
        require(not any("/".join(parts[:index]) in lowered for index in range(1, len(parts))),
                "Windows archive has a file/directory collision")
    for name in files:
        # Only what package.py packages: the Core, its manifest and public files.
        require(name in ("ha.exe", "package.json") or windows_package.public_path(name),
                "Windows archive has a path package.py does not package: " + name)
        require(not LOCAL_PARTS.intersection(name.split("/")) and PurePosixPath(name).suffix not in CACHE_SUFFIXES,
                "Windows archive contains local metadata or generated cache: " + name)
    require(WINDOWS_REQUIRED <= files.keys(),
            "Windows archive is missing required files: " + ", ".join(sorted(WINDOWS_REQUIRED - files.keys())))
    require(len(files["package.json"]) <= WINDOWS_MAX_MANIFEST_BYTES, "Windows archive manifest exceeds 1 MiB")
    manifest = windows_manifest(files["package.json"])
    require(isinstance(manifest, dict) and manifest.get("schema") == WINDOWS_SCHEMA
            and manifest.get("os") == "windows" and manifest.get("arch") == "amd64",
            "Windows archive manifest has an unexpected schema or platform")
    require(manifest.get("version") == version, "Windows archive manifest version differs from the release")
    listed = manifest.get("files")
    require(isinstance(listed, list), "Windows archive manifest lists no files")
    expected = {}
    for item in listed:
        require(isinstance(item, dict) and isinstance(item.get("path"), str) and item["path"] not in expected,
                "Windows archive manifest has an invalid or repeated entry")
        # JSON true and 1.0 compare equal to 1 in Python; the installer takes
        # only integer sizes and lowercase digests.
        require(type(item.get("size")) is int and 0 <= item["size"] <= WINDOWS_MAX_FILE_BYTES
                and isinstance(item.get("sha256"), str) and WINDOWS_SHA256.fullmatch(item["sha256"]),
                "Windows archive manifest has an invalid entry: " + item["path"])
        expected[item["path"]] = item
    require(expected.keys() == files.keys() - {"package.json"},
            "Windows archive files differ from its manifest")
    for name, item in expected.items():
        body = files[name]
        require(item.get("size") == len(body) and item.get("sha256") == hashlib.sha256(body).hexdigest(),
                "Windows archive file differs from its manifest: " + name)
    try:
        windows_package.windows_amd64(files["ha.exe"])
    except ValueError as error:
        raise ValueError("Windows archive Core: " + str(error)) from None
    return files


def check(dist: Path, version: str, published_assets: Path | None = None, require_windows: bool = False):
    names = release_names(version)
    windows = windows_name(version)
    require(dist.is_dir(), "release output directory does not exist")
    actual = {path.name for path in dist.iterdir()}
    archives = names | ({windows} if windows in actual else set())
    require(actual == archives | {"checksums.txt"} and (windows in actual or not require_windows),
            "release output must contain exactly four archives, the Windows archive when required, and checksums.txt")
    for name in actual:
        require((dist / name).is_file() and not (dist / name).is_symlink(), "release output must contain regular files")
    checksums = checksum_entries((dist / "checksums.txt").read_text(encoding="utf-8"))
    require(checksums.keys() == archives,
            "checksum file does not name exactly the four release archives and any Windows archive present")
    for name in sorted(names):
        members = read_archive(dist / name, checksums[name])
        files = archive_files(members, name.removesuffix(".tar.gz"))
        check_platform(files["ha"][1], name.removeprefix("ha_" + version + "_").removesuffix(".tar.gz"))
    if windows in archives:
        windows_files(dist / windows, checksums[windows], version)
    if published_assets is not None:
        uploaded = published_assets.read_text(encoding="utf-8").splitlines()
        require(len(uploaded) == len(set(uploaded)) and set(uploaded) == actual,
                "published release asset names differ from the validated local files")
    return sorted(archives)


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
        members = read_archive(dist / (prefix + ".tar.gz"))
        files = archive_files(members, prefix)
        # Only bounded bodies reach the disk, inside the temporary directory.
        for name, (member, body) in files.items():
            destination = package / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            with destination.open("xb") as output:
                output.write(body)
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
    parser.add_argument("--require-windows", action="store_true")
    args = parser.parse_args()
    try:
        archives = check(args.dist, args.version, args.published_assets, args.require_windows)
        target = native_smoke(args.dist, args.version) if args.run_native else None
    except (OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired) as error:
        print("FAIL: " + str(error), file=sys.stderr)
        return 1
    suffix = f"; native {target} Core and reference violation/pass" if target else ""
    windows = "and the Windows archive " if windows_name(args.version) in archives else ""
    print("PASS: four release archives " + windows + "with checksums and required public files" + suffix)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
