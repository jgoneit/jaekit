"""Archive bounds of the standalone validator and its native smoke.

Synthetic archives only; no network, installation or publication. A failure at
an observed boundary raises ArchiveViolation with the property it names, so a
goal observer can tell it apart from a broken fixture.
"""
import contextlib
import gzip
import importlib.util
import io
import json
from pathlib import Path
import platform
import struct
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
spec = importlib.util.spec_from_file_location("bounded_release_archives", HERE / "check_archives.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
VERSION = "0.1.3"
# Published in tools/release-checks/README.md (Archive resource limits).
ENTRIES = 10000
METADATA = 64 * 1024
METADATA_TOTAL = 1024 * 1024
LIMIT_NAMES = ("ARCHIVE_MAX_COMPRESSED_BYTES", "ARCHIVE_MAX_BYTES", "ARCHIVE_MAX_ENTRIES",
               "ARCHIVE_MAX_STREAM_BYTES", "ARCHIVE_MAX_FILE_BYTES", "ARCHIVE_MAX_METADATA_BYTES",
               "ARCHIVE_MAX_METADATA_TOTAL_BYTES")
# A second gzip member with a valid header and a reserved deflate block type.
CORRUPT_MEMBER = gzip.compress(b"\0" * 1024)[:10] + b"\xff" * 16


class ArchiveViolation(AssertionError):
    def __init__(self, code, message):
        self.code = code
        super().__init__(f"[violation:{code}] {message}")


def require(value, code, message):
    if not value:
        raise ArchiveViolation(code, message)


def native_target():
    system = {"Linux": "linux", "Darwin": "darwin"}.get(platform.system())
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower())
    if not (system and arch):
        raise unittest.SkipTest("native smoke supports macOS/Linux on amd64/arm64")
    return f"{system}_{arch}"


def executable_header(target):
    header = bytearray(64)
    system, arch = target.split("_")
    if system == "linux":
        header[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", header, 18, {"amd64": 62, "arm64": 183}[arch])
    else:
        header[:4] = b"\xcf\xfa\xed\xfe"
        struct.pack_into("<I", header, 4, {"amd64": 0x1000007, "arm64": 0x100000C}[arch])
    return bytes(header)


def regular(name, body=b""):
    info = tarfile.TarInfo(name)
    info.size, info.mode = len(body), 0o644
    return info, body


def directory(name):
    info = tarfile.TarInfo(name)
    info.type, info.mode = tarfile.DIRTYPE, 0o755
    return info, None


class Dist:
    """Four release archives and checksums; the native archive may hold a real Core."""

    def __init__(self, root, binary=None):
        self.binary = binary
        self.dist = Path(root) / "dist"
        self.dist.mkdir()
        self.contents = {name: b"Synthetic public document.\n" for name in release.REQUIRED}
        for name in ("tools/check-result-reference.py", "examples/check-result/declaration.json",
                     "examples/check-result/reference.json", "examples/check-result/input.txt"):
            self.contents[name] = (ROOT / name).read_bytes()
        self.raw = {}
        for target in release.TARGETS:
            self.write(target)

    @staticmethod
    def prefix(target):
        return f"ha_{VERSION}_{target}"

    def path(self, target):
        return self.dist / (self.prefix(target) + ".tar.gz")

    def write(self, target, extra=(), *, fmt=tarfile.PAX_FORMAT, pax=None, raw_tail=b"", gzip_tail=b"", cut=0):
        out = io.BytesIO()
        with tarfile.open(fileobj=out, mode="w", format=fmt, pax_headers=pax) as archive:
            for name, value in self.contents.items():
                if name == "ha":
                    native = self.binary is not None and target == native_target()
                    value = self.binary if native else executable_header(target)
                info = tarfile.TarInfo(self.prefix(target) + "/" + name)
                info.size, info.mode = len(value), 0o755 if name == "ha" else 0o644
                archive.addfile(info, io.BytesIO(value))
            for info, body in extra:
                archive.addfile(info, None if body is None else io.BytesIO(body))
        self.raw[target] = out.getvalue() + raw_tail
        data = gzip.compress(self.raw[target]) + gzip_tail
        self.path(target).write_bytes(data[:len(data) - cut])
        lines = [release.digest(path) + "  " + path.name for path in sorted(self.dist.glob("*.tar.gz"))]
        (self.dist / "checksums.txt").write_text("\n".join(lines) + "\n")

    def filled(self, target, entries):
        """Directory entries that bring the archive to exactly `entries` physical entries."""
        return [directory(f"{self.prefix(target)}/guides/d{n}") for n in range(entries - len(self.contents))]

    def snapshot(self):
        return {path.name: path.read_bytes() for path in sorted(self.dist.iterdir())}


@contextlib.contextmanager
def workspace():
    with tempfile.TemporaryDirectory(prefix="jaekit-archive-bounds-") as name:
        root = Path(name)
        scratch = root / "tmp"
        scratch.mkdir()
        # Native smoke creates its temporary directory here, so cleanup is observable.
        with mock.patch.object(tempfile, "tempdir", str(scratch)):
            yield root, scratch


def limits(**values):
    return mock.patch.multiple(release, create=True, **values)


@contextlib.contextmanager
def parsed():
    """Record the size of every PAX/GNU metadata body tarfile starts to parse."""
    sizes = []

    def recorded(original):
        def method(self, archive):
            sizes.append(self.size)
            return original(self, archive)
        return method

    with mock.patch.object(tarfile.TarInfo, "_proc_pax", recorded(tarfile.TarInfo._proc_pax)), \
         mock.patch.object(tarfile.TarInfo, "_proc_gnulong", recorded(tarfile.TarInfo._proc_gnulong)):
        yield sizes


def capabilities():
    return json.dumps({
        "schema": "ha-capabilities/v1", "ha_version": VERSION,
        "default_rules": "run-rules/3", "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3"],
        "features": ["estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1",
                     "structured-change-results/v1", "budget-change/v1"],
        "formats": {"criteria": ["legacy", "nested/1"], "check_declaration": ["check-declaration/v1"],
                    "check_result": ["check-result/v1"], "check_evidence": ["check-evidence/v1"]},
    })


def smoke(dist):
    """Run the native smoke with a stand-in Core; the packaged producer still runs."""
    original = release.invoke

    def invoke(argv, cwd, env=None, expected=0):
        if Path(argv[0]).name == "ha":
            return "ha " + VERSION + "\n" if argv[1:] == ["--version"] else capabilities()
        return original(argv, cwd, env, expected)

    with mock.patch.object(release, "invoke", side_effect=invoke):
        return release.native_smoke(dist.dist, VERSION)


def validate(dist):
    return release.check(dist.dist, VERSION)


def accepted(call, code, label):
    try:
        call()
    except Exception as error:
        raise ArchiveViolation(code, f"{label} was refused: {type(error).__name__}: {error}")


def refused(call, cause, code, label):
    try:
        call()
    except ValueError as error:
        require(cause in str(error), code, f"{label}: refusal lost its cause {cause!r}: {error}")
    except Exception as error:
        raise ArchiveViolation(code, f"{label}: unhandled {type(error).__name__}: {error}")
    else:
        raise ArchiveViolation(code, f"{label}: accepted input beyond the published boundary")


def metadata_cases(prefix):
    """Each case exceeds one published metadata limit inside a small archive."""
    many = METADATA_TOTAL // (METADATA - 1024) + 1
    return (
        ("PAX path", tarfile.PAX_FORMAT, None, [regular(f"{prefix}/guides/" + "p" * METADATA, b"x")]),
        ("GNU long name", tarfile.GNU_FORMAT, None, [regular(f"{prefix}/guides/" + "g" * METADATA, b"x")]),
        ("PAX records", tarfile.PAX_FORMAT, {f"r{n}": "v" for n in range(METADATA // 8)}, []),
        ("metadata total", tarfile.PAX_FORMAT, None,
         [regular(f"{prefix}/guides/{n:02d}" + "t" * (METADATA - 1024)) for n in range(many)]),
    )


def validator_limits():
    """AC-1: the validator bounds compressed input, entries, bodies and the expanded stream."""
    code = "archive-consumer-unbounded"
    native = native_target()
    with workspace() as (root, _):
        dist = Dist(root)
        accepted(lambda: validate(dist), code, "normal release")
        dist.write(native, dist.filled(native, ENTRIES))
        accepted(lambda: validate(dist), code, f"{ENTRIES} physical entries")
        dist.write(native, dist.filled(native, ENTRIES + 1))
        refused(lambda: validate(dist), "entry count", code, f"{ENTRIES + 1} physical entries")
        dist.write(native)
        with tarfile.open(fileobj=io.BytesIO(dist.raw[native])) as archive:
            members = archive.getmembers()
        exact = {"ARCHIVE_MAX_BYTES": (sum(m.size for m in members), "cumulative byte"),
                 "ARCHIVE_MAX_FILE_BYTES": (max(m.size for m in members), "oversized"),
                 "ARCHIVE_MAX_STREAM_BYTES": (max(map(len, dist.raw.values())), "expanded stream"),
                 "ARCHIVE_MAX_COMPRESSED_BYTES": (max(dist.path(t).stat().st_size for t in release.TARGETS),
                                                  "compressed size")}
        for name, (value, cause) in exact.items():
            with limits(**{name: value}):
                accepted(lambda: validate(dist), code, f"{name} at its exact value")
            with limits(**{name: value - 1}):
                refused(lambda: validate(dist), cause, code, f"{name} exceeded by one byte")
        # The body that crosses the cumulative limit is never read.
        calls, extract = [], tarfile.TarFile.extractfile
        with limits(ARCHIVE_MAX_BYTES=exact["ARCHIVE_MAX_BYTES"][0] - 1), \
             mock.patch.object(tarfile.TarFile, "extractfile",
                               lambda archive, member: calls.append(member.name) or extract(archive, member)):
            refused(lambda: validate(dist), "cumulative byte", code, "body beyond the cumulative limit")
        # The first archive in order is refused; its last body crosses the limit.
        last = members[-1].name.split("/", 1)[1]
        require(not any(name.endswith("/" + last) for name in calls), code,
                "the validator read a body beyond the cumulative limit")
    for name in LIMIT_NAMES:
        value = getattr(release, name, None)
        require(type(value) is int and value > 0, code, f"the validator publishes no finite {name}")


def smoke_limits():
    """AC-1: native smoke applies the same limits; reading only what it needs is no exception."""
    code = "archive-consumer-unbounded"
    native = native_target()
    with workspace() as (root, _):
        dist = Dist(root)
        accepted(lambda: smoke(dist), code, "normal native smoke")
        with tarfile.open(fileobj=io.BytesIO(dist.raw[native])) as archive:
            total = sum(member.size for member in archive.getmembers())
        with limits(ARCHIVE_MAX_BYTES=total - 1):
            refused(lambda: smoke(dist), "cumulative byte", code, "native smoke beyond the cumulative limit")
        with limits(ARCHIVE_MAX_STREAM_BYTES=len(dist.raw[native]) - 1):
            refused(lambda: smoke(dist), "expanded stream", code, "native smoke beyond the expanded stream")
        dist.write(native, dist.filled(native, ENTRIES + 1))
        refused(lambda: smoke(dist), "entry count", code, "native smoke beyond the entry count")


def validator_metadata():
    """AC-2: metadata is bounded before tarfile reads or parses it; normal long names remain."""
    code = "archive-metadata-unbounded"
    native = native_target()
    prefix = Dist.prefix(native)
    with workspace() as (root, _):
        dist = Dist(root)
        for label, fmt, pax, extra in metadata_cases(prefix):
            dist.write(native, extra, fmt=fmt, pax=pax)
            for consumer, call in (("validator", lambda: validate(dist)), ("native smoke", lambda: smoke(dist))):
                with parsed() as sizes:
                    refused(call, "metadata limit", code, f"{consumer}: {label}")
                require(max(sizes, default=0) <= METADATA and sum(sizes) <= METADATA_TOTAL, code,
                        f"{consumer}: {label}: tarfile parsed {sum(sizes)} metadata bytes before the limit")
        for fmt in (tarfile.PAX_FORMAT, tarfile.GNU_FORMAT):
            # Longer than a ustar name, within file-system name and path limits for native smoke.
            dist.write(native, [regular(f"{prefix}/guides/" + "/".join(["n" * 200] * 3), b"ok")], fmt=fmt)
            with parsed() as sizes:
                accepted(lambda: validate(dist), code, "a supported long name")
            accepted(lambda: smoke(dist), code, "a supported long name in native smoke")
            size = max(sizes)
            for name in ("ARCHIVE_MAX_METADATA_BYTES", "ARCHIVE_MAX_METADATA_TOTAL_BYTES"):
                with limits(**{name: size}):
                    accepted(lambda: validate(dist), code, f"metadata at the exact {name}")
                with limits(**{name: size - 1}):
                    refused(lambda: validate(dist), "metadata limit", code, f"metadata over {name}")
    text = (ROOT / "tools/release-checks/README.md").read_text(encoding="utf-8")
    require(getattr(release, "ARCHIVE_MAX_METADATA_BYTES", None) == METADATA
            and getattr(release, "ARCHIVE_MAX_METADATA_TOTAL_BYTES", None) == METADATA_TOTAL
            and "64 KiB" in text and "1 MiB" in text, code, "metadata limits are not published")


def validator_corruption():
    """AC-3: corrupt or trailing data is a handled refusal with its cause; padding remains."""
    code = "archive-corruption-accepted"
    native = native_target()
    with workspace() as (root, _):
        dist = Dist(root)
        for label, options, cause in (
                ("corrupt concatenated gzip member", {"gzip_tail": CORRUPT_MEMBER}, "invalid or incomplete"),
                ("nonzero data after the tar end", {"raw_tail": b"trailing input"}, "nonzero data after archive end"),
                ("truncated gzip trailer", {"cut": 4}, "invalid or incomplete")):
            dist.write(native, **options)
            refused(lambda: validate(dist), cause, code, f"validator: {label}")
            refused(lambda: smoke(dist), cause, code, f"native smoke: {label}")
        for label, options in (("zero tar padding", {"raw_tail": b"\0" * 512}),
                               ("zero concatenated gzip member", {"gzip_tail": gzip.compress(b"\0" * 4096)})):
            dist.write(native, **options)
            accepted(lambda: validate(dist), code, label)
            accepted(lambda: smoke(dist), code, f"native smoke: {label}")
        dist.write(native, gzip_tail=CORRUPT_MEMBER)
        process = subprocess.run([sys.executable, str(HERE / "check_archives.py"), "--dist", str(dist.dist),
                                  "--version", VERSION], capture_output=True, text=True, timeout=300)
        require(process.returncode == 1 and "FAIL: invalid or incomplete release archive" in process.stderr
                and "Traceback" not in process.stderr, code,
                f"validator entry point: exit {process.returncode}: {process.stderr[-400:]}")


class ValidatorBounds(unittest.TestCase):
    def test_validator_applies_every_published_archive_limit(self):
        validator_limits()

    def test_native_smoke_applies_the_same_limits(self):
        smoke_limits()

    def test_metadata_is_bounded_before_tarfile_parses_it(self):
        validator_metadata()

    def test_corrupt_or_trailing_data_is_refused_with_its_cause(self):
        validator_corruption()


class Preservation(unittest.TestCase):
    def test_normal_release_and_native_smoke_keep_inputs(self):
        with workspace() as (root, scratch):
            dist = Dist(root)
            before = dist.snapshot()
            self.assertEqual(set(validate(dist)), release.release_names(VERSION))
            self.assertEqual(smoke(dist), native_target())
            self.assertEqual(dist.snapshot(), before)
            self.assertEqual(list(scratch.iterdir()), [])

    def test_refusals_keep_inputs_and_remove_temporary_files(self):
        native = native_target()
        with workspace() as (root, scratch):
            dist = Dist(root)
            del dist.contents["LICENSE"]
            for extra in ((), dist.filled(native, ENTRIES + 1)):
                dist.write(native, extra)
                before = dist.snapshot()
                with self.assertRaises(ValueError):
                    smoke(dist)
                self.assertEqual(dist.snapshot(), before)
                self.assertEqual(list(scratch.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
