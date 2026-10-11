"""Bounded synthetic tar/gzip inputs; no network or disk extraction."""
import contextlib
import gzip
import importlib.util
import io
from pathlib import Path
import tarfile
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("bounded_archive_consumer", HERE / "check.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)
TARGET = "linux_amd64"
PREFIX = "ha_0.1.3_" + TARGET + "/"
REQUIRED = ("ha", "LICENSE", "README.md", "README.en.md", "guides/INSTALL.md",
            "guides/INSTALL.en.md", "contracts/run-record.md")


class ArchiveViolation(AssertionError):
    def __init__(self, code, message):
        self.code = code
        super().__init__(message)


def require(value, code, message):
    if not value:
        raise ArchiveViolation(code, message)


def package(extra=(), *, pax=None, fmt=tarfile.PAX_FORMAT):
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz", format=fmt,
                      pax_headers=pax) as archive:
        for name, body, kind in [(n, b"x", tarfile.REGTYPE) for n in REQUIRED] + list(extra):
            member = tarfile.TarInfo(PREFIX + name)
            member.type, member.size = kind, len(body)
            archive.addfile(member, io.BytesIO(body))
    return out.getvalue()


def reject(data, expected, code="archive-boundary-not-enforced"):
    try:
        check.archive_files(data, TARGET)
    except check.Failure as error:
        require(expected in str(error), code, "archive rejection lost the resource/error kind: " + str(error))
    else:
        raise ArchiveViolation(code, "archive accepted input beyond the declared boundary")


def limits(**values):
    return patch.multiple(check, create=True, **values)


def finite_contract():
    for name in ("ARCHIVE_MAX_BYTES", "ARCHIVE_MAX_ENTRIES", "ARCHIVE_MAX_STREAM_BYTES"):
        value = getattr(check, name, None)
        require(type(value) is int and 0 < value < 2**31, "archive-limits-absent", name + " is not finite")
    text = (HERE / "README.md").read_text()
    require(all(word in text for word in ("256 MiB", "10,000", "512 MiB", "directories", "metadata")),
            "archive-limits-absent", "public limits or counted entry kinds missing")
    with limits(ARCHIVE_MAX_ENTRIES=8):
        check.archive_files(package([("empty", b"", tarfile.DIRTYPE)]), TARGET)
        reject(package([("empty", b"", tarfile.DIRTYPE), ("zero", b"", tarfile.REGTYPE)]),
               "entry count", "archive-limits-absent")


def before_body():
    calls = []
    extract = tarfile.TarFile.extractfile
    def capture(archive, member):
        calls.append(member.name)
        return extract(archive, member)
    with limits(ARCHIVE_MAX_BYTES=7), patch.object(tarfile.TarFile, "extractfile", capture):
        reject(package([("overflow", b"x", tarfile.REGTYPE)]), "cumulative byte",
               "archive-read-before-limit")
    require(PREFIX + "overflow" not in calls, "archive-read-before-limit", "overflow body was extracted")
    with limits(ARCHIVE_MAX_ENTRIES=7), patch.object(tarfile.TarFile, "extractfile", capture):
        reject(package([("overflow", b"", tarfile.REGTYPE)]), "entry count", "archive-read-before-limit")
    require(PREFIX + "overflow" not in calls, "archive-read-before-limit", "entry beyond limit was read")
    data = package()
    reject(data[:-4], "invalid or incomplete", "archive-read-before-limit")
    raw = gzip.decompress(data)
    reject(gzip.compress(raw[:512 + 1]), "invalid or incomplete", "archive-read-before-limit")


def families():
    with limits(ARCHIVE_MAX_BYTES=31):
        accepted = package([("a", b"a" * 12, tarfile.REGTYPE), ("b", b"b" * 12, tarfile.REGTYPE)])
        check.archive_files(accepted, TARGET)
        reject(package([("a", b"a" * 12, tarfile.REGTYPE), ("b", b"b" * 13, tarfile.REGTYPE)]),
               "cumulative byte", "archive-family-accepted")
    with limits(ARCHIVE_MAX_ENTRIES=12):
        check.archive_files(package([(str(n), b"", tarfile.REGTYPE) for n in range(5)]), TARGET)
        reject(package([(str(n), b"", tarfile.DIRTYPE) for n in range(6)]), "entry count", "archive-family-accepted")
    with limits(ARCHIVE_MAX_BYTES=1024):
        reject(package([("compressed", b"\0" * 8192, tarfile.REGTYPE)]), "cumulative byte", "archive-family-accepted")
        reject(package(pax={"comment": "x" * 2048}), "cumulative byte", "archive-family-accepted")
    # A global PAX header counts even though TarFile iteration hides it.
    with limits(ARCHIVE_MAX_ENTRIES=7):
        reject(package(pax={"comment": "synthetic"}), "entry count", "archive-family-accepted")
    data = package()
    expanded = len(gzip.decompress(data))
    with limits(ARCHIVE_MAX_STREAM_BYTES=expanded):
        check.archive_files(data, TARGET)
    with limits(ARCHIVE_MAX_STREAM_BYTES=expanded - 1):
        reject(data, "expanded stream", "archive-family-accepted")
    reject(data + gzip.compress(b"nonzero trailing input"), "nonzero", "archive-family-accepted")


def consumer_paths():
    data = package([("overflow", b"x", tarfile.REGTYPE)])
    collector = check.Checker()
    name = "ha_0.1.3_" + TARGET + ".tar.gz"
    metadata = {"browser_download_url": f"https://github.com/{check.REPO}/releases/download/{check.TAG}/{name}",
                "size": len(data), "digest": "sha256:" + check.digest(data)}
    with limits(ARCHIVE_MAX_BYTES=7), patch.object(collector, "assets", return_value={name: metadata}), \
         patch.object(check, "urlopen", return_value=io.BytesIO(data)):
        downloaded = collector.download(name)
        reject(downloaded, "cumulative byte", "archive-consumer-bypassed-limit")
    with limits(ARCHIVE_MAX_BYTES=7), patch.object(check, "PLATFORMS", (TARGET,)), \
         patch.object(collector, "assets", return_value={name: {}, "checksums.txt": {}}), \
         patch.object(collector, "download", side_effect=lambda n: data if n == name else
                      (check.digest(data) + "  " + name + "\n").encode()):
        try:
            collector.archives()
        except check.Failure as error:
            require("cumulative byte" in str(error), "archive-consumer-bypassed-limit", str(error))
        else:
            raise ArchiveViolation("archive-consumer-bypassed-limit", "release archives bypassed the parser limit")
    # Installation reuses the same parser before accepting any binary/capture.
    with limits(ARCHIVE_MAX_BYTES=7), patch.object(check, "native_platform", return_value=TARGET), \
         patch.object(collector, "evidence", return_value={"installations": {"direct": {"binary": {}}}}), \
         patch.object(collector, "artifact", return_value=(Path("synthetic"), b"x")), \
         patch.object(collector, "download", return_value=data):
        try:
            collector.installation("direct")
        except check.Failure as error:
            require("cumulative byte" in str(error), "archive-consumer-bypassed-limit", str(error))
        else:
            raise ArchiveViolation("archive-consumer-bypassed-limit", "installation bypassed the parser limit")


# Published in README.md (Archive resource limits).
METADATA = 64 * 1024
METADATA_TOTAL = 1024 * 1024
# A second gzip member with a valid header and a reserved deflate block type.
CORRUPT_MEMBER = gzip.compress(b"\0" * 1024)[:10] + b"\xff" * 16


@contextlib.contextmanager
def parsed():
    """Record the size of every PAX/GNU metadata body tarfile starts to parse."""
    sizes = []

    def recorded(original):
        def method(self, archive):
            sizes.append(self.size)
            return original(self, archive)
        return method

    with patch.object(tarfile.TarInfo, "_proc_pax", recorded(tarfile.TarInfo._proc_pax)), \
         patch.object(tarfile.TarInfo, "_proc_gnulong", recorded(tarfile.TarInfo._proc_gnulong)):
        yield sizes


def metadata():
    """PAX/GNU metadata is bounded before tarfile reads or parses it."""
    code = "archive-metadata-unbounded"
    many = METADATA_TOTAL // (METADATA - 1024) + 1
    for label, value in (
            ("PAX path", package([("nested/" + "p" * METADATA, b"x", tarfile.REGTYPE)])),
            ("GNU long name", package([("nested/" + "g" * METADATA, b"x", tarfile.REGTYPE)], fmt=tarfile.GNU_FORMAT)),
            ("PAX records", package(pax={f"r{n}": "v" for n in range(METADATA // 8)})),
            ("metadata total", package([(f"nested/{n:02d}" + "t" * (METADATA - 1024), b"", tarfile.REGTYPE)
                                        for n in range(many)]))):
        with parsed() as sizes:
            reject(value, "metadata limit", code)
        require(max(sizes, default=0) <= METADATA and sum(sizes) <= METADATA_TOTAL, code,
                f"{label}: tarfile parsed {sum(sizes)} metadata bytes before the limit")
    for fmt in (tarfile.PAX_FORMAT, tarfile.GNU_FORMAT):
        name = "nested/" + "/".join(["n" * 200] * 3)
        value = package([(name, b"ok", tarfile.REGTYPE)], fmt=fmt)
        with parsed() as sizes:
            require(name in check.archive_files(value, TARGET), code, "a supported long name was lost")
        size = max(sizes)
        for limit in ("ARCHIVE_MAX_METADATA_BYTES", "ARCHIVE_MAX_METADATA_TOTAL_BYTES"):
            with limits(**{limit: size}):
                check.archive_files(value, TARGET)
            with limits(**{limit: size - 1}):
                reject(value, "metadata limit", code)


def corruption():
    """Corrupt or trailing data is a handled requirement failure, not a traceback or success."""
    code = "archive-corruption-accepted"
    data = package()
    for label, value, cause in (
            ("corrupt concatenated gzip member", data + CORRUPT_MEMBER, "invalid or incomplete"),
            ("truncated gzip trailer", data[:-4], "invalid or incomplete"),
            ("nonzero data after the tar end", gzip.compress(gzip.decompress(data) + b"trailing"), "nonzero")):
        try:
            check.archive_files(value, TARGET)
        except check.Failure as error:
            require(cause in str(error), code, f"{label}: refusal lost its cause: {error}")
        except Exception as error:
            raise ArchiveViolation(code, f"{label}: unhandled {type(error).__name__}: {error}")
        else:
            raise ArchiveViolation(code, f"{label}: accepted")
    check.archive_files(data + gzip.compress(b"\0" * 4096), TARGET)
    # A corrupt public asset is a requirement failure; a failed download stays unverified.
    collector = check.Checker()
    name = "ha_0.1.3_" + TARGET + ".tar.gz"
    corrupt = data + CORRUPT_MEMBER
    with patch.object(check, "PLATFORMS", (TARGET,)), \
         patch.object(collector, "assets", return_value={name: {}, "checksums.txt": {}}), \
         patch.object(collector, "download", side_effect=lambda n: corrupt if n == name else
                      (check.digest(corrupt) + "  " + name + "\n").encode()):
        try:
            collector.archives()
        except check.Failure as error:
            require("invalid or incomplete" in str(error), code, f"release archives: {error}")
        except Exception as error:
            raise ArchiveViolation(code, f"release archives: unhandled {type(error).__name__}: {error}")
        else:
            raise ArchiveViolation(code, "release archives accepted a corrupt gzip member")
    url = f"https://github.com/{check.REPO}/releases/download/{check.TAG}/{name}"
    with patch.object(collector, "assets", return_value={name: {"browser_download_url": url}}), \
         patch.object(check, "urlopen", side_effect=OSError("offline")):
        try:
            collector.download(name)
        except check.Unavailable:
            pass
        else:
            raise ArchiveViolation(code, "a failed download was not reported as unverified")


class ArchiveLimits(unittest.TestCase):
    def test_finite_public_limits_count_empty_and_directory_entries(self):
        finite_contract()
    def test_limit_is_checked_before_body_and_truncation_is_rejected(self):
        before_body()
    def test_cumulative_count_compression_metadata_and_stream_families(self):
        families()
    def test_download_collection_and_installation_use_common_parser(self):
        consumer_paths()
    def test_normal_files_unchanged_and_existing_rejections_preserved(self):
        data = package()
        before = bytes(data)
        self.assertEqual(check.archive_files(data, TARGET), {name: b"x" for name in REQUIRED})
        self.assertEqual(data, before)
        reject(package([("ha", b"y", tarfile.REGTYPE)]), "duplicate")
        reject(package([("../escape", b"", tarfile.REGTYPE)]), "unsafe")
        reject(package([("link", b"", tarfile.SYMTYPE)]), "non-regular")
        with limits(ARCHIVE_MAX_FILE_BYTES=1):
            reject(package([("large", b"yy", tarfile.REGTYPE)]), "oversized")
    def test_malformed_size_and_sparse_metadata_do_not_bypass_bounds(self):
        reject(package(pax={"size": "not-a-number"}), "invalid archive PAX size")
        reject(package(pax={"size": "-1"}), "invalid archive PAX size")
        reject(package(pax={"size": "9999999999999999999999"}), "oversized")
        reject(package(pax={"size": "9" * 5000}), "oversized")
        check.archive_files(package(pax={"size": "0" * 5000 + "1"}), TARGET)
        for metadata in ({"GNU.sparse.size": "999999"},
                         {"GNU.sparse.map": "0,999999", "GNU.sparse.size": "999999"},
                         {"GNU.sparse.major": "1", "GNU.sparse.minor": "0", "GNU.sparse.realsize": "999999"}):
            reject(package(pax=metadata), "unsupported sparse")
        reject(package([("sparse", b"", tarfile.GNUTYPE_SPARSE)]), "unsupported sparse")

    def test_metadata_is_bounded_before_tarfile_parses_it(self):
        metadata()

    def test_corrupt_or_trailing_data_is_a_handled_failure(self):
        corruption()

    def test_pax_long_names_and_normal_padding_remain_supported(self):
        name = "nested/" + "n" * 180
        self.assertIn(name, check.archive_files(package([(name, b"ok", tarfile.REGTYPE)]), TARGET))
