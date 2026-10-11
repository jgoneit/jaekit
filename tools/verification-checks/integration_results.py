"""Compose actual invocation-bound repository probes without changing their facts.

Only target names are namespaced. Expected violations belong to declarations;
they are never used to construct an observed violation. This adapter does not
read private goals, old runs or another goal's completion state.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import posixpath
import re
from pathlib import Path
import subprocess
import sys
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[2]
LIMIT = 1024 * 1024
IDENTIFIER = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}\Z")
ERRORS = {"dependency_missing", "collection_error", "import_error", "compile_error",
          "browser_start_error", "setup_error", "execution_error", "permission_denied"}


def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def read_bytes(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > LIMIT:
        raise ValueError("input must be a bounded regular file")
    data = path.read_bytes()
    if len(data) > LIMIT:
        raise ValueError("input exceeds limit")
    return data


def decode(data):
    value = json.loads(data.decode("utf-8"), object_pairs_hook=unique)
    def bounded(item, depth=0):
        if depth > 32:
            raise ValueError("JSON nesting exceeds limit")
        if isinstance(item, dict):
            for child in item.values():
                bounded(child, depth + 1)
        elif isinstance(item, list):
            for child in item:
                bounded(child, depth + 1)
    bounded(value)
    return value


def read_json(path):
    return decode(read_bytes(path))


def identifier(value):
    return isinstance(value, str) and bool(IDENTIFIER.fullmatch(value))


def valid_path(value):
    return (isinstance(value, str) and value not in ("", ".", "..")
            and not value.startswith(("/", "../")) and posixpath.normpath(value) == value
            and not any(char in value for char in ("\\", "\x00", "\r", "\n", "\t"))
            and ".git" not in value.split("/"))


def load_declaration(root, name):
    if not valid_path(name):
        raise ValueError("invalid declaration path")
    def read_input(relative):
        current = root
        for part in relative.split("/"):
            current /= part
            if current.is_symlink():
                raise ValueError("symlink evidence input")
        return read_bytes(current)
    raw = read_input(name)
    value = decode(raw)
    fields = {"schema", "producer", "producer_version", "producer_paths", "targets"}
    if (not isinstance(value, dict) or set(value) != fields
            or value["schema"] != "check-declaration/v1"
            or not identifier(value["producer"]) or not identifier(value["producer_version"])):
        raise ValueError("unsupported declaration")
    paths, targets = value["producer_paths"], value["targets"]
    if (not isinstance(paths, list) or not 1 <= len(paths) <= 128
            or not isinstance(targets, list) or not 1 <= len(targets) <= 1024):
        raise ValueError("invalid declaration size")
    seen_paths, seen_ids = set(), set()
    inputs = {name: hashlib.sha256(raw).hexdigest()}
    for path in paths:
        if not valid_path(path) or path == name or path in seen_paths:
            raise ValueError("invalid or duplicate producer path")
        seen_paths.add(path)
        inputs[path] = hashlib.sha256(read_input(path)).hexdigest()
    for target in targets:
        if (not isinstance(target, dict) or set(target) != {"id", "violation"}
                or not identifier(target["id"]) or not identifier(target["violation"])
                or target["id"] in seen_ids or target["id"] == "invocation"):
            raise ValueError("invalid, duplicate or reserved target")
        seen_ids.add(target["id"])
    binding = "".join(path + "\x00" + inputs[path] + "\n" for path in sorted(inputs))
    return value, hashlib.sha256(binding.encode()).hexdigest()


def validate(report, declaration, invocation, digest, code):
    expected = {"schema", "producer", "producer_version", "invocation", "declaration_digest",
                "attempts_complete", "observations"}
    if not isinstance(report, dict) or set(report) != expected:
        raise ValueError("unsupported report fields")
    if (report["schema"] != "check-result/v1"
            or report["producer"] != declaration["producer"]
            or report["producer_version"] != declaration["producer_version"]
            or report["invocation"] != invocation or report["declaration_digest"] != digest
            or report["attempts_complete"] is not True):
        raise ValueError("report does not bind to this complete invocation")
    ids = {target["id"] for target in declaration["targets"]}
    history = {target: [] for target in ids}
    observations = report["observations"]
    if not isinstance(observations, list) or not 1 <= len(observations) <= 4096:
        raise ValueError("invalid observation count")
    for item in observations:
        if not isinstance(item, dict):
            raise ValueError("observation must be an object")
        status = item.get("status")
        fields = {"target", "attempt", "status"}
        if status == "violation":
            fields.add("violation")
        elif status in ("error", "skip"):
            fields.add("reason")
        elif status != "pass":
            raise ValueError("unknown status")
        if set(item) != fields or item["target"] not in ids:
            raise ValueError("unexpected observation target or fields")
        attempts = history[item["target"]]
        if type(item["attempt"]) is not int or item["attempt"] != len(attempts) + 1:
            raise ValueError("incomplete or repeated attempt history")
        attempts.append(item)
        if status == "violation" and (not isinstance(item["violation"], str) or not IDENTIFIER.fullmatch(item["violation"])):
            raise ValueError("missing observed violation")
        if status == "error" and item["reason"] not in ERRORS:
            raise ValueError("unknown environment reason")
        if status == "skip" and item["reason"] not in ("skipped", "not_selected"):
            raise ValueError("unknown skip reason")
    if not all(history.values()):
        raise ValueError("missing target")
    return observations


def invoke(entry, *, root=ROOT, legacy=None):
    prefix = entry["id"] + "."
    transport = {"target": prefix + "invocation", "attempt": 1, "status": "pass"}
    try:
        declaration, digest = load_declaration(root, entry["declaration"])
    except (ValueError, TypeError, KeyError, OSError, RecursionError) as error:
        print(entry["id"] + ": " + str(error), file=sys.stderr)
        return [dict(transport, status="error", reason="setup_error")]
    ids = [prefix + target["id"] for target in declaration["targets"]]
    with tempfile.TemporaryDirectory(prefix="jaekit-integrated-probe-") as temporary:
        output = Path(temporary) / "result.json"
        invocation = hashlib.sha256(uuid.uuid4().bytes).hexdigest()
        env = {key: value for key, value in os.environ.items() if not key.startswith("HA_EVIDENCE_")}
        env.update(HA_EVIDENCE_PATH=str(output), HA_EVIDENCE_INVOCATION=invocation,
                   HA_EVIDENCE_DECLARATION_DIGEST=digest, PYTHONDONTWRITEBYTECODE="1")
        try:
            argv = [legacy if arg == "{legacy}" else arg for arg in entry["argv"]]
            if any(arg is None for arg in argv):
                raise ValueError("the legacy compatibility boundary requires an explicit binary")
            result = subprocess.run(argv, cwd=root, env=env, timeout=1800)
            observations = validate(read_json(output), declaration, invocation, digest, result.returncode)
            passed = {item["target"] for item in observations if item["status"] == "pass"}
            violated = {item["target"] for item in observations if item["status"] == "violation"}
            environment = any(item["status"] == "error" for item in observations)
            skipped = any(item["status"] == "skip" for item in observations)
            # Core retains contradictory retries as assertions even if the
            # producer's last attempt exits zero. Skips also remain unknown,
            # rather than becoming an invented transport/environment failure.
            inconsistent_attempts = bool(passed & violated) and not environment and not skipped
            exit_conflict = (not inconsistent_attempts and
                             ((result.returncode == 0 and (violated or environment)) or
                              (result.returncode != 0 and not violated and not environment and not skipped)))
            if result.returncode < 0 or exit_conflict:
                transport.update(status="error", reason="execution_error")
            return [*[dict(item, target=prefix + item["target"]) for item in observations], transport]
        except (ValueError, TypeError, KeyError, OSError, RecursionError, subprocess.TimeoutExpired) as error:
            # An unavailable or invalid report cannot establish either success
            # or the expected baseline failure. No declaration ID is copied.
            print(entry["id"] + ": " + str(error), file=sys.stderr)
            return [{"target": target, "attempt": 1, "status": "error", "reason": "execution_error"}
                    for target in [*ids, transport["target"]]]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("criterion")
    parser.add_argument("--legacy")
    args = parser.parse_args()
    manifest = read_json(Path(__file__).with_name("integration-map.json"))
    entries = manifest[args.criterion]
    observations = []
    for entry in entries:
        observations.extend(invoke(entry, legacy=args.legacy))
    report = {"schema": "check-result/v1", "producer": "jaekit-integration-probes", "producer_version": "1",
              "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
              "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
              "attempts_complete": True, "observations": observations}
    with Path(os.environ["HA_EVIDENCE_PATH"]).open("x") as output:
        json.dump(report, output)
    return 0 if all(item["status"] == "pass" for item in observations) else 1


if __name__ == "__main__":
    raise SystemExit(main())
