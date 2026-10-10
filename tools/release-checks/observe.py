#!/usr/bin/env python3
"""Opt-in collection during an original, authorized local shell execution.

The release checker never calls this program. Use `run` instead of launching
the shell directly when new evidence is needed; it is not a replay command.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile
import uuid

from shell_reader import covered

SCHEMA = "jaekit-execution/v1"


def digest(data):
    return hashlib.sha256(data).hexdigest()


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


def child(arguments):
    journal = os.environ["JAEKIT_OBSERVATION_JOURNAL"]
    binary = os.environ["JAEKIT_OBSERVATION_BINARY"]
    expected = os.environ["JAEKIT_OBSERVATION_DIGEST"]
    call = uuid.uuid4().hex
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
                     "executable": actual, "argv": [binary, *arguments]})
    code = process.wait()
    append(journal, {"kind": "core_exit", "call": call, "pid": process.pid, "returncode": code})
    return exit_code(code)


def collect(args):
    output = Path(args.output)
    # Existing artifacts are immutable, including failed observations.
    with output.open("x", encoding="utf-8") as destination:
        core, shell = identity(args.core), identity(args.shell)
        if Path(shell["path"]).name not in ("bash", "zsh"):
            raise ValueError("only bash and zsh are supported")
        script = args.script
        report = {"schema": SCHEMA, "invocation": args.invocation, "run_id": uuid.uuid4().hex,
                  "launcher_argv": [sys.executable, str(Path(__file__).resolve()), *sys.argv[1:]],
                  "producer": identity(__file__), "core": core, "shell": shell,
                  "reader": identity(Path(__file__).with_name("shell_reader.py")),
                  "script_sha256": digest(script.encode()), "coverage": "bounded-launcher/v1",
                  "supported": covered(script), "events": []}
        with tempfile.TemporaryDirectory(prefix="jaekit-observe-") as directory:
            journal = Path(directory) / "events.jsonl"
            launcher = Path(directory) / "ha-observed"
            launcher.write_text("#!/bin/sh\nexec " + shlex.join([sys.executable, str(Path(__file__).resolve()), "child"]) + ' "$@"\n')
            launcher.chmod(0o700)
            env = {key: val for key, val in os.environ.items()
                   if not key.startswith(("BASH_FUNC_", "JAEKIT_"))
                   and key not in ("ENV", "BASH_ENV", "SHELLOPTS", "BASHOPTS", "ZDOTDIR")}
            env.update(JAEKIT_CORE=str(launcher), JAEKIT_OBSERVATION_JOURNAL=str(journal),
                       JAEKIT_OBSERVATION_BINARY=core["path"], JAEKIT_OBSERVATION_DIGEST=core["sha256"])
            append(journal, {"kind": "shell_start", "invocation": args.invocation,
                             "script_sha256": report["script_sha256"]})
            shell_argv = [shell["path"]] + (["--noprofile", "--norc"] if Path(shell["path"]).name == "bash" else ["-f"])
            try:
                process = subprocess.Popen([*shell_argv, "-c", script], env=env)
                code = process.wait()
                append(journal, {"kind": "shell_exit", "returncode": code})
            except OSError:
                code = 127
                append(journal, {"kind": "shell_start_error"})
            # A terminated observer leaves an empty artifact, never a made-up
            # completion. The caller retains it and any failed native capture.
            report["events"] = [json.loads(line) for line in journal.read_text().splitlines()]
            try:
                report["identity_unchanged"] = identity(core["path"]) == core and identity(shell["path"]) == shell
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


def main():
    if sys.argv[1:2] == ["child"]:
        return child(sys.argv[2:])
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["run"])
    parser.add_argument("--core", required=True)
    parser.add_argument("--shell", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--invocation", required=True)
    try:
        separator = sys.argv.index("--")
    except ValueError:
        separator = len(sys.argv)
    args = parser.parse_args(sys.argv[1:separator])
    if len(sys.argv) != separator + 2:
        parser.error("provide exactly one original shell script after --")
    args.script = sys.argv[-1]
    return collect(args)


if __name__ == "__main__":
    sys.exit(main())
