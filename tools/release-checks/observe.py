#!/usr/bin/env python3
"""Opt-in collection during an original, authorized local shell execution.

The release checker never calls this program. Use `run` instead of launching
the shell directly when new evidence is needed; it is not a replay command.
"""
# The original collector and each launcher child must establish this boundary
# before checkout modules or packages can shadow the trusted standard library.
import sys

_CALLER_IMPORT_PATH = sys.path[:]
_STDLIB_ROOT = getattr(sys, "_stdlib_dir", None)
if not _STDLIB_ROOT:
    _encodings_file = getattr(sys.modules.get("encodings"), "__file__", None)
    if not _encodings_file or "/" not in _encodings_file:
        raise ImportError("interpreter standard library location is unavailable")
    _STDLIB_ROOT = _encodings_file.rsplit("/", 2)[0]
_STDLIB_IMPORT_PATH = (_STDLIB_ROOT, _STDLIB_ROOT + "/lib-dynload")
sys.path[:] = _STDLIB_IMPORT_PATH

import argparse
from contextlib import contextmanager
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import uuid

LOADED_SOURCE = {str(Path(__file__).resolve()): Path(__file__).read_bytes()}


def load_policy(name):
    """Read and execute a named source file, never a package or cached code."""
    path = Path(__file__).resolve().with_name(name + ".py")
    source = path.read_bytes()
    module = type(sys)("jaekit_observe_" + name)
    module.__file__ = str(path)
    exec(compile(source, str(path), "exec"), module.__dict__)
    LOADED_SOURCE[str(path.resolve())] = source
    return module


covered = load_policy("shell_reader").covered
execution_provenance = load_policy("execution_provenance")
sys.path[:] = _CALLER_IMPORT_PATH
del _CALLER_IMPORT_PATH

SCHEMA = "jaekit-execution/v2"


def digest(data):
    return hashlib.sha256(data).hexdigest()


@contextmanager
def trusted_imports():
    previous = sys.path[:]
    sys.path[:] = _STDLIB_IMPORT_PATH
    try:
        yield
    finally:
        sys.path[:] = previous


def loaded_identity(path):
    path = Path(path).resolve(strict=True)
    source = LOADED_SOURCE[str(path)]
    if path.read_bytes() != source:
        raise ValueError("observer source changed after loading")
    return {"path": str(path), "sha256": digest(source)}


def identity(path):
    path = Path(path).resolve(strict=True)
    return {"path": str(path), "sha256": digest(path.read_bytes())}


def append(path, event):
    with open(path, "a+", encoding="utf-8") as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        stream.seek(0)
        previous = [json.loads(line) for line in stream]
        event = dict(event, seq=len(previous) + 1,
                     prev=digest(json.dumps(previous[-1], sort_keys=True, separators=(",", ":")).encode()) if previous else None)
        stream.seek(0, os.SEEK_END)
        stream.write(json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n")
        stream.flush()
        os.fsync(stream.fileno())


def exit_code(code):
    return 128 - code if code < 0 else code


@trusted_imports()
def child(arguments):
    journal = os.environ["JAEKIT_OBSERVATION_JOURNAL"]
    binary = os.environ["JAEKIT_OBSERVATION_BINARY"]
    expected = os.environ["JAEKIT_OBSERVATION_DIGEST"]
    call = uuid.uuid4().hex
    records = os.environ.get("JAEKIT_OBSERVATION_RECORDS")
    before = execution_provenance.observed_prefix(records) if records else None
    cwd = str(Path.cwd().resolve())
    try:
        actual = identity(binary)
        if actual["sha256"] != expected:
            raise OSError("Core identity changed before start")
        # Popen reports exec/spawn failures before returning a process. A
        # candidate or failed launch is never written as core_start.
        process = subprocess.Popen([binary, *arguments])
    except OSError:
        append(journal, {"kind": "core_start_error", "call": call})
        return 127
    append(journal, {"kind": "core_start", "call": call, "pid": process.pid,
                     "executable": actual, "argv": [binary, *arguments], "cwd": cwd, "records_before": before})
    code = process.wait()
    append(journal, {"kind": "core_exit", "call": call, "pid": process.pid, "returncode": code,
                     "cwd": str(Path.cwd().resolve()),
                     "records_after": execution_provenance.observed_prefix(records) if records else None})
    return exit_code(code)


@trusted_imports()
def collect(args):
    output = Path(args.output)
    # Existing artifacts are immutable, including failed observations.
    with output.open("x", encoding="utf-8") as destination:
        core, shell = identity(args.core), identity(args.shell)
        if Path(shell["path"]).name not in ("bash", "zsh"):
            raise ValueError("only bash and zsh are supported")
        script = args.script
        context = execution_provenance.context(args.project, args.goal) if args.project else None
        report = {"schema": SCHEMA, "invocation": args.invocation, "run_id": uuid.uuid4().hex,
                  "launcher_argv": [sys.executable, str(Path(__file__).resolve()), *sys.argv[1:]],
                  "producer": loaded_identity(__file__), "core": core, "shell": shell,
                  "reader": loaded_identity(Path(__file__).with_name("shell_reader.py")),
                  "provenance": loaded_identity(Path(__file__).with_name("execution_provenance.py")), "context": context,
                  "script_sha256": digest(script.encode()), "coverage": "bounded-launcher/v1",
                  "supported": covered(script), "events": []}
        with tempfile.TemporaryDirectory(prefix="jaekit-observe-") as directory:
            journal = Path(directory) / "events.jsonl"
            # Launch the verified bytes from an isolated copy. Replacement and
            # restoration of the requested path between hash and exec cannot
            # change the actual Core image used by this original observation.
            image = Path(directory) / "core-image"
            image_bytes = Path(core["path"]).read_bytes()
            if digest(image_bytes) != core["sha256"]:
                raise ValueError("Core identity changed before isolated launch")
            image.write_bytes(image_bytes)
            image.chmod(0o500)
            report["execution_image"] = identity(image)
            launcher = Path(directory) / "ha-observed"
            launcher.write_text("#!/bin/sh\nexec " + shlex.join([sys.executable, str(Path(__file__).resolve()), "child"]) + ' "$@"\n')
            launcher.chmod(0o700)
            env = {key: val for key, val in os.environ.items()
                   if not key.startswith(("BASH_FUNC_", "JAEKIT_", "GIT_"))
                   and key not in ("ENV", "BASH_ENV", "SHELLOPTS", "BASHOPTS", "ZDOTDIR")}
            env.update(JAEKIT_CORE=str(launcher), JAEKIT_OBSERVATION_JOURNAL=str(journal),
                       JAEKIT_OBSERVATION_BINARY=report["execution_image"]["path"], JAEKIT_OBSERVATION_DIGEST=core["sha256"])
            if context:
                env["JAEKIT_OBSERVATION_RECORDS"] = context["records"]
            append(journal, {"kind": "shell_start", "invocation": args.invocation,
                             "script_sha256": report["script_sha256"]})
            shell_argv = [shell["path"]] + (["--noprofile", "--norc"] if Path(shell["path"]).name == "bash" else ["-f"])
            try:
                process = subprocess.Popen([*shell_argv, "-c", script], env=env,
                                           cwd=context["project"] if context else None)
                code = process.wait()
                append(journal, {"kind": "shell_exit", "returncode": code})
            except OSError:
                code = 127
                append(journal, {"kind": "shell_start_error"})
            # A terminated observer leaves an empty artifact, never a made-up
            # completion. The caller retains it and any failed native capture.
            report["events"] = [json.loads(line) for line in journal.read_text().splitlines()]
            try:
                report["identity_unchanged"] = (identity(core["path"]) == core and identity(shell["path"]) == shell
                                                and identity(image) == report["execution_image"]
                                                and all(identity(path)["sha256"] == digest(source)
                                                        for path, source in LOADED_SOURCE.items()))
            except OSError:
                report["identity_unchanged"] = False
            encoded = (json.dumps(report, sort_keys=True) + "\n").encode()
            destination.write(encoded.decode())
            destination.flush()
            os.fsync(destination.fileno())
            # Native tool results carry this receipt. An index author cannot
            # bind an old report to a later identical command by changing the
            # sidecar tool ID alone. Original stdout/exit remain untouched;
            # stderr has this explicitly documented collector diagnostic.
            receipt = {"schema": "jaekit-execution-receipt/v1", "invocation": args.invocation,
                       "run_id": report["run_id"], "sha256": digest(encoded)}
            sys.stderr.write("\nJAEKIT_EXECUTION_RECEIPT " + json.dumps(receipt, sort_keys=True) + "\n")
            sys.stderr.flush()
        return exit_code(code)


@trusted_imports()
def main():
    if sys.argv[1:2] == ["child"]:
        return child(sys.argv[2:])
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["run"])
    parser.add_argument("--core", required=True)
    parser.add_argument("--shell", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--invocation", required=True)
    parser.add_argument("--project")
    parser.add_argument("--goal")
    try:
        separator = sys.argv.index("--")
    except ValueError:
        separator = len(sys.argv)
    args = parser.parse_args(sys.argv[1:separator])
    if bool(args.project) != bool(args.goal):
        parser.error("--project and --goal must be provided together")
    if len(sys.argv) != separator + 2:
        parser.error("provide exactly one original shell script after --")
    args.script = sys.argv[-1]
    return collect(args)


if __name__ == "__main__":
    sys.exit(main())
