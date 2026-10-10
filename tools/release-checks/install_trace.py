#!/usr/bin/env python3
"""Explicit local direct installation and read-only receipt consumption.

The checker never starts this collector. Its fixed procedure installs a local,
validated archive into a fresh directory; no command from evidence is executed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import uuid

SCHEMA = "jaekit-direct-install/v1"
PROCEDURE = "fresh-release-directory/v1"
RECEIPT = "JAEKIT_INSTALL_RECEIPT "
TARGETS = {f"{osname}_{arch}" for osname in ("darwin", "linux") for arch in ("arm64", "amd64")}
LOADED_SELF = Path(__file__).read_bytes()


class InvalidInstallation(Exception):
    pass


class UnavailableInstallation(Exception):
    pass


def need(condition, reason):
    if not condition:
        raise InvalidInstallation(reason)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def regular(path, limit=None):
    path = Path(path)
    info = path.lstat()
    if path.is_symlink() or not stat.S_ISREG(info.st_mode):
        raise OSError("installation input must be a regular file, not a symlink")
    if limit is None:
        return path.read_bytes()
    if info.st_size > limit:
        raise OSError("installation archive compressed byte limit exceeded")
    with path.open("rb") as stream:
        data = stream.read(limit + 1)
    if len(data) > limit:
        raise OSError("installation archive compressed byte limit exceeded")
    return data


def identity(path, limit=None):
    path = Path(path)
    data = regular(path, limit)
    return {"path": str(path.resolve(strict=True)), "sha256": digest(data)}


def relative_name(name):
    path = PurePosixPath(name)
    return (isinstance(name, str) and name and not path.is_absolute()
            and str(path) == name and not any(part in (".", "..") for part in path.parts)
            and "\\" not in name and "\0" not in name)


def install_files(destination, files, created=None):
    """Create only a new tree. Never remove an original or partial result."""
    need(files and "ha" in files and all(relative_name(name) for name in files),
         "unsupported installation member path")
    # Creation is the pre-install absence observation, not a caller assertion.
    destination.mkdir(mode=0o700)
    if created:
        created()
    installed = []
    for name, data in sorted(files.items()):
        path = destination / name
        current = destination
        for part in Path(name).parts[:-1]:
            current = current / part
            try:
                current.mkdir(mode=0o755)
            except FileExistsError:
                if current.is_symlink() or not current.is_dir():
                    raise OSError("installation parent is not a regular directory")
        with path.open("xb") as output:
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        path.chmod(0o755 if name == "ha" else 0o644)
        need(regular(path) == data, "installed bytes differ from the archive")
        installed.append({"path": name, "sha256": digest(data)})
    return installed


def collect(args, launcher_argv=None):
    # Import only in the explicitly invoked collector. The read-only consumer
    # shares the validator through its checked caller, avoiding circular import.
    import check as validator
    archive_limit = getattr(validator, "ARCHIVE_MAX_BYTES", 256 * 1024 * 1024)
    archive = Path(args.archive)
    destination = Path(args.destination)
    output = Path(args.output)
    need(archive.is_absolute() and destination.is_absolute() and output.is_absolute(),
         "collector paths must be absolute")
    need(args.target in TARGETS, "unsupported installation platform")
    need(isinstance(args.invocation, str) and re.fullmatch(r"[A-Za-z0-9._-]{1,128}", args.invocation),
         "invalid installation invocation")
    report = {"schema": SCHEMA, "procedure": PROCEDURE,
              "invocation": args.invocation, "run_id": uuid.uuid4().hex,
              "launcher_argv": launcher_argv or [sys.executable, sys.argv[0], *sys.argv[1:]],
              "producer": {"path": str(Path(__file__).resolve()), "sha256": digest(LOADED_SELF)},
              "target": args.target,
              "destination": str(destination), "before_absent": False,
              "completed": False, "identity_unchanged": False,
              "archive": None, "installed": [], "core": None, "failure": None,
              "validator": {"path": str(Path(validator.__file__).resolve()),
                            "sha256": digest(validator.LOADED_SOURCE[str(Path(validator.__file__).resolve())])}}
    # Existing reports, including failed ones, are never overwritten.
    with output.open("x", encoding="utf-8") as stream:
        try:
            raw = regular(archive, archive_limit)
            report["archive"] = {"path": str(archive.resolve()), "sha256": digest(raw)}
            files = validator.archive_files(raw, args.target)
            need(not destination.is_symlink() and not destination.exists(),
                 "fresh installation destination already exists")
            destination = destination.resolve()
            report["destination"] = str(destination)
            need(destination.parent.is_dir() and not destination.parent.is_symlink(),
                 "installation destination parent must exist without a symlink")
            report["installed"] = install_files(destination, files,
                                                 lambda: report.update(before_absent=True))
            report["core"] = identity(destination / "ha")
            report["identity_unchanged"] = (identity(archive, archive_limit) == report["archive"]
                                            and identity(__file__) == report["producer"]
                                            and identity(validator.__file__) == report["validator"])
            report["completed"] = report["identity_unchanged"]
        except Exception as error:
            # Failure is preserved as an incomplete report; it never supplies
            # a successful install receipt or permits deleting partial output.
            report["failure"] = type(error).__name__ + ": " + str(error)
        encoded = (json.dumps(report, sort_keys=True) + "\n").encode()
        stream.write(encoded.decode())
        stream.flush()
        os.fsync(stream.fileno())
    receipt = {"schema": "jaekit-install-receipt/v1", "invocation": report["invocation"],
               "run_id": report["run_id"], "sha256": digest(encoded)}
    sys.stderr.write(RECEIPT + json.dumps(receipt, sort_keys=True) + "\n")
    sys.stderr.flush()
    return 0 if report["completed"] else 1


def _decode(raw):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            need(key not in value, "duplicate installation report field")
            value[key] = item
        return value
    try:
        return json.loads(raw, object_pairs_hook=unique)
    except (ValueError, UnicodeDecodeError, RecursionError):
        raise UnavailableInstallation("installation report is malformed or incomplete") from None


def consume(checker, installation, public_archive_bytes, target):
    """Read an original command capture; never replay it or repair its output."""
    import check as validator
    capture = installation.get("install")
    if not isinstance(capture, dict) or not isinstance(capture.get("report"), dict):
        raise UnavailableInstallation("original direct installation report is missing")
    if not {"argv", "exit_code", "stdout", "stderr", "report"} <= set(capture):
        raise UnavailableInstallation("original direct installation capture is incomplete")
    argv, _ = checker.capture(capture)
    report_path, raw = checker.artifact(capture["report"])
    report = _decode(raw)
    fields = {"schema", "procedure", "invocation", "run_id", "launcher_argv", "producer", "validator",
              "target", "destination", "before_absent", "completed", "identity_unchanged",
              "archive", "installed", "core", "failure"}
    if not isinstance(report, dict) or set(report) != fields:
        raise UnavailableInstallation("installation report has incomplete or unsupported fields")
    if report["schema"] != SCHEMA or report["procedure"] != PROCEDURE:
        raise UnavailableInstallation("unsupported direct installation report or procedure")
    if any(type(report[key]) is not bool for key in ("before_absent", "completed", "identity_unchanged")):
        raise UnavailableInstallation("installation outcome observations are malformed")
    need(isinstance(argv, list) and len(argv) == 13 and argv[2] == "run"
         and argv == report.get("launcher_argv"), "installation capture is not the original collector command")
    options = dict(zip(argv[3::2], argv[4::2]))
    need(len(options) == 5 and set(options) == {"--archive", "--target", "--destination", "--output", "--invocation"},
         "unsupported direct installation command")
    need(all(Path(options[key]).is_absolute() for key in ("--archive", "--destination", "--output")),
         "installation paths are not absolute")
    need(Path(argv[1]).is_absolute() and Path(argv[0]).is_absolute(), "collector identity is not explicit")
    need(report_path == Path(options["--output"]).resolve(), "installation report belongs to another output")
    need(report.get("invocation") == options["--invocation"]
         and isinstance(report.get("invocation"), str)
         and re.fullmatch(r"[A-Za-z0-9._-]{1,128}", report["invocation"])
         and isinstance(report.get("run_id"), str) and re.fullmatch(r"[0-9a-f]{32}", report["run_id"]),
         "installation run identity is missing or conflicts")
    _, stderr = checker.artifact(capture.get("stderr"))
    expected = {"schema": "jaekit-install-receipt/v1", "invocation": report["invocation"],
                "run_id": report["run_id"], "sha256": digest(raw)}
    try:
        stderr_text = stderr.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        raise UnavailableInstallation("installation capture output is unreadable") from None
    receipts = [_decode(line[len(RECEIPT):]) for line in stderr_text.splitlines() if line.startswith(RECEIPT)]
    if not receipts:
        raise UnavailableInstallation("original installation capture receipt is missing")
    need(receipts == [expected], "original installation capture does not corroborate this report")
    producer_path, producer = checker.artifact(report.get("producer"))
    need(producer_path == Path(argv[1]).resolve() and producer == LOADED_SELF,
         "installation collector differs from checked source")
    validator_path, validator_bytes = checker.artifact(report.get("validator"))
    loaded_validator = validator.LOADED_SOURCE[str(Path(validator.__file__).resolve())]
    need(validator_path == producer_path.with_name("check.py") and validator_bytes == loaded_validator,
         "installation archive validator differs from checked source")
    need(report.get("target") == target == options["--target"], "installation platform differs")
    need(report.get("before_absent") is True and report.get("completed") is True
         and report.get("identity_unchanged") is True and report.get("failure") is None,
         "fresh installation was not observed to completion")
    archive_path, archive = checker.artifact(report.get("archive"))
    need(archive_path == Path(options["--archive"]).resolve() and not Path(options["--archive"]).is_symlink()
         and archive == public_archive_bytes, "installation used a different archive")
    destination = Path(options["--destination"])
    need(not destination.is_symlink(), "installation destination is a symlink")
    destination = destination.resolve()
    need(report.get("destination") == str(destination)
         and destination.is_dir(), "installation destination differs or is missing")
    need(isinstance(installation.get("package_root"), str)
         and Path(installation["package_root"]).resolve() == destination,
         "support files belong to another installation")
    files = validator.archive_files(public_archive_bytes, target)
    expected_files = [{"path": name, "sha256": digest(data)} for name, data in sorted(files.items())]
    need(report.get("installed") == expected_files, "installation member evidence differs from the archive")
    for name, data in files.items():
        need(relative_name(name), "unsupported installed member path")
        path = destination / name
        try:
            need(all(not parent.is_symlink() for parent in path.parents if parent != destination.parent),
                 "installed member traverses a symlink")
            need(regular(path) == data, "installed member differs from original installation")
        except OSError:
            raise UnavailableInstallation("installed member is absent or unreadable") from None
    core_path, core = checker.artifact(installation.get("binary"))
    need(core_path == destination / "ha" and core == files["ha"]
         and report.get("core") == {"path": str(core_path), "sha256": digest(core)},
         "installed Core is not the observed installation result")
    need(core_path.stat().st_mode & 0o111, "installed Core is not executable")
    return core_path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["run"])
    for name in ("archive", "target", "destination", "output", "invocation"):
        parser.add_argument("--" + name, required=True)
    return collect(parser.parse_args())


if __name__ == "__main__":
    raise SystemExit(main())
