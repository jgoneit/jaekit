"""Bounded synthetic tar/gzip inputs; no network or disk extraction."""
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


def package(extra=(), *, pax=None):
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz", format=tarfile.PAX_FORMAT,
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

    def test_pax_long_names_and_normal_padding_remain_supported(self):
        name = "nested/" + "n" * 180
        self.assertIn(name, check.archive_files(package([(name, b"ok", tarfile.REGTYPE)]), TARGET))
