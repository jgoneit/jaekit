"""Goal observer for release archive resource boundaries.

AC-1..3 run the probe functions of the common regressions in
tools/release/test_archive_bounds.py and tools/release-checks/test_archive_limits.py.
AC-5 checks the public validator entry point with a Core built from this
checkout. Only an ArchiveViolation raised at an observed boundary is a
violation; fixture, build and process failures remain errors.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import traceback

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / "tools/release"), str(ROOT / "tools/release-checks")]
import test_archive_bounds as validator  # noqa: E402
import test_archive_limits as observation  # noqa: E402

VIOLATIONS = (validator.ArchiveViolation, observation.ArchiveViolation)
DISCOVER = """import sys, unittest
def ids(suite):
    for test in suite:
        yield from ids(test) if isinstance(test, unittest.TestSuite) else [test.id()]
print("\\n".join(ids(unittest.defaultTestLoader.discover(sys.argv[1], "test_*.py"))))
"""
SELECTED = {
    "tools/release": ("test_archive_bounds.ValidatorBounds.", "test_archive_bounds.Preservation."),
    "tools/release-checks": ("test_archive_limits.ArchiveLimits.test_metadata_is_bounded_before_tarfile_parses_it",
                             "test_archive_limits.ArchiveLimits.test_corrupt_or_trailing_data_is_a_handled_failure",
                             "test_archive_limits.ArchiveLimits.test_download_collection_and_installation_use_common_parser"),
}


def entry_points():
    """AC-5: every consumer publishes the same limits; the public validator CLI and a real
    native Core keep normal, over-limit and corrupt results; the common check selects the regressions."""
    code = "archive-entry-point-inconsistent"
    require = validator.require
    native = validator.native_target()
    with tempfile.TemporaryDirectory(prefix="jaekit-archive-entry-") as name:
        root = Path(name)
        binary = root / "ha"
        build = subprocess.run(["go", "build", "-mod=readonly", "-ldflags", "-X main.version=" + validator.VERSION,
                                "-o", str(binary), "./cmd/ha"], cwd=ROOT, capture_output=True, text=True, timeout=900)
        if build.returncode:
            raise RuntimeError("go build failed: " + build.stderr[-500:])
        (root / "work").mkdir()
        scratch = root / "tmp"
        scratch.mkdir()
        dist = validator.Dist(root / "work", binary=binary.read_bytes())

        def run(label):
            before = dist.snapshot()
            process = subprocess.run([sys.executable, str(ROOT / "tools/release/check_archives.py"), "--dist",
                                      str(dist.dist), "--version", validator.VERSION, "--run-native"],
                                     cwd=root, env=dict(os.environ, TMPDIR=str(scratch)),
                                     capture_output=True, text=True, timeout=600)
            require("Traceback" not in process.stderr, code, f"{label}: unhandled error: {process.stderr[-400:]}")
            require(dist.snapshot() == before, code, f"{label}: the validator changed its input")
            require(not any(scratch.iterdir()), code, f"{label}: the validator left temporary files")
            return process

        process = run("normal release")
        require(process.returncode == 0 and f"native {native} Core" in process.stdout, code,
                f"normal release with a real Core: exit {process.returncode}: {process.stderr[-400:]}")
        prefix = dist.prefix(native)
        for label, options, cause in (
                ("entry count", {"extra": dist.filled(native, validator.ENTRIES + 1)}, "archive entry count limit exceeded"),
                ("GNU long name", {"extra": [validator.regular(prefix + "/guides/" + "g" * validator.METADATA, b"x")],
                                   "fmt": tarfile.GNU_FORMAT}, "archive metadata limit exceeded"),
                ("corrupt gzip member", {"gzip_tail": validator.CORRUPT_MEMBER}, "invalid or incomplete release archive")):
            dist.write(native, **options)
            process = run(label)
            require(process.returncode == 1 and f"FAIL: {cause}" in process.stderr, code,
                    f"{label}: exit {process.returncode}: {process.stderr[-400:]}")
    for name in validator.LIMIT_NAMES:
        values = getattr(validator.release, name, None), getattr(observation.check, name, None)
        require(type(values[0]) is int and values[0] == values[1], code,
                f"{name} differs between the validator and the acceptance checker: {values}")
    source = (ROOT / "tools/verify.py").read_text(encoding="utf-8")
    require('("Release archive regressions", "release")' in source
            and '("Release acceptance checker regressions", "release-checks")' in source, code,
            "the common check no longer runs the archive regressions")
    for directory, prefixes in SELECTED.items():
        listed = subprocess.run([sys.executable, "-c", DISCOVER, str(ROOT / directory)], cwd=ROOT,
                                capture_output=True, text=True, timeout=120)
        ids = listed.stdout.split()
        if listed.returncode or any("_FailedTest" in test for test in ids):
            raise RuntimeError(f"{directory}: regressions cannot be loaded: {listed.stderr[-400:]}")
        for wanted in prefixes:
            require(any(test.startswith(wanted) for test in ids), code, f"{directory} does not select {wanted}")


PROBES = {
    "AC-1": (validator.validator_limits, validator.smoke_limits, observation.families, observation.consumer_paths),
    "AC-2": (observation.metadata, validator.validator_metadata),
    "AC-3": (observation.corruption, validator.validator_corruption),
    "AC-5": (entry_points,),
}


def main():
    criterion = sys.argv[1]
    observation_record = {"target": criterion, "attempt": 1}
    try:
        for probe in PROBES[criterion]:
            probe()
        observation_record["status"] = "pass"
    except VIOLATIONS as error:
        observation_record.update(status="violation", violation=error.code)
        print(str(error), file=sys.stderr)
    except Exception:
        traceback.print_exc()
        observation_record.update(status="error", reason="execution_error")
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-review-archive-observer", "producer_version": "1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "attempts_complete": True, "observations": [observation_record]}
        with Path(os.environ["HA_EVIDENCE_PATH"]).open("x") as output:
            json.dump(report, output)
    print(criterion, observation_record["status"])
    return 0 if observation_record["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
