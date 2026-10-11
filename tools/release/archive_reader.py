"""Bounded, read-only reader shared by every release archive consumer.

The release acceptance checker, the standalone archive validator and its
native smoke read archives only through this module. It inspects one gzip
compressed tar held in memory, never extracts paths and never seeks back.
"""
import gzip
import io
import re
import tarfile
import zlib

# Published limits; see tools/release-checks/README.md (Archive resource limits).
MAX_COMPRESSED_BYTES = 64 * 1024 * 1024
MAX_BYTES = 256 * 1024 * 1024
MAX_ENTRIES = 10000
MAX_STREAM_BYTES = 512 * 1024 * 1024
MAX_FILE_BYTES = 64 * 1024 * 1024
MAX_METADATA_BYTES = 64 * 1024
MAX_METADATA_TOTAL_BYTES = 1024 * 1024
MAX_PAX_KEYS = 128
METADATA_TYPES = (tarfile.XHDTYPE, tarfile.XGLTYPE, tarfile.SOLARIS_XHDTYPE,
                  tarfile.GNUTYPE_LONGNAME, tarfile.GNUTYPE_LONGLINK)


class Rejected(ValueError):
    """The archive exceeds a published limit or is not one complete archive."""


def require(condition, message):
    if not condition:
        raise Rejected(message)


class Stream:
    """Bound decompression, including metadata and tar padding, without seeking back."""
    def __init__(self, source, limits):
        self.source, self.limits, self.position = source, limits, 0
        self.entries = self.physical = self.metadata = 0

    def tell(self):
        return self.position

    def read(self, size=-1):
        remaining = self.limits["stream"] - self.position
        requested = remaining + 1 if size < 0 else min(size, remaining + 1)
        value = self.source.read(requested)
        self.position += len(value)
        require(self.position <= self.limits["stream"], "archive expanded stream limit exceeded")
        return value

    def seek(self, offset, whence=0):
        target = offset if whence == 0 else self.position + offset
        require(whence in (0, 1) and target >= self.position, "invalid archive seek")
        while self.position < target:
            if not self.read(min(65536, target - self.position)):
                raise EOFError("incomplete archive body")
        return self.position


class BoundedTarInfo(tarfile.TarInfo):
    stream = None  # set on a per-read subclass

    def _proc_member(self, archive):
        # tarfile reads and parses PAX/GNU metadata as one block before it
        # yields a logical member. Count and check each physical header here,
        # before that body is read, decoded or split into records.
        stream, limits = self.stream, self.stream.limits
        stream.entries += 1
        stream.physical += self.size
        require(stream.entries <= limits["entries"], "archive entry count limit exceeded")
        require(self.size >= 0, "invalid negative archive size")
        require(stream.physical <= limits["bytes"], "archive cumulative byte limit exceeded")
        if self.type in METADATA_TYPES:
            # One body and all bodies together stay small, which also bounds
            # long-name strings and the number of PAX records parsed.
            stream.metadata += self.size
            require(self.size <= limits["metadata"] and stream.metadata <= limits["metadata_total"],
                    "archive metadata limit exceeded")
        # Global keys accumulate across headers. Check before the next PAX
        # header can snapshot that state, including recursive local headers.
        require(len(archive.pax_headers) <= limits["pax_keys"], "archive PAX key limit exceeded")
        return super()._proc_member(archive)

    def _apply_pax_info(self, pax_headers, encoding, errors):
        # Also bound the combined global/local map before tarfile copies it
        # into the retained member. The entry cap bounds the number of copies.
        require(len(pax_headers) <= self.stream.limits["pax_keys"], "archive PAX key limit exceeded")
        if "size" in pax_headers:
            require(re.fullmatch(r"[0-9]+", pax_headers["size"]) is not None,
                    "invalid archive PAX size")
            size = pax_headers["size"].lstrip("0") or "0"
            maximum = str(self.stream.limits["file"])
            require(len(size) < len(maximum) or (len(size) == len(maximum) and size <= maximum),
                    "oversized archive entry")
            # Avoid Python's integer digit limit being silently interpreted as
            # size zero by tarfile, even for a long but valid leading-zero value.
            pax_headers["size"] = size
        require(not any(key.startswith("GNU.sparse.") for key in pax_headers),
                "unsupported sparse archive entry")
        # tarfile strips these slashes per member. Normalize shared global
        # state once so a long path is not allocated again for every member.
        if "path" in pax_headers:
            pax_headers["path"] = pax_headers["path"].rstrip("/")
        return super()._apply_pax_info(pax_headers, encoding, errors)

    def _proc_sparse(self, *args):
        raise Rejected("unsupported sparse archive entry")

    # Sparse maps can consume hidden extension blocks or logical body bytes
    # before TarFile yields the entry. Release packages contain regular files;
    # reject these extensions before their maps are interpreted.
    _proc_gnusparse_00 = _proc_sparse
    _proc_gnusparse_01 = _proc_sparse
    _proc_gnusparse_10 = _proc_sparse


LIMITS = {"compressed": MAX_COMPRESSED_BYTES, "bytes": MAX_BYTES, "entries": MAX_ENTRIES,
          "stream": MAX_STREAM_BYTES, "file": MAX_FILE_BYTES, "metadata": MAX_METADATA_BYTES,
          "metadata_total": MAX_METADATA_TOTAL_BYTES, "pax_keys": MAX_PAX_KEYS}


def read(data, **overrides):
    """Return [(member, body)] in archive order; body is None for a non-file.

    A consumer may pass its own values for LIMITS keys. The whole input is one
    checked archive: limit, corruption and trailing data errors raise Rejected
    even after every expected file was read.
    """
    if set(overrides) - set(LIMITS):
        raise TypeError("unknown archive limit")
    limits = dict(LIMITS, **overrides)
    require(len(data) <= limits["compressed"], "archive compressed size limit exceeded")
    members, logical = [], 0
    try:
        with gzip.GzipFile(fileobj=io.BytesIO(data), mode="rb") as source:
            reader = Stream(source, limits)
            info = type("BoundedTarInfo", (BoundedTarInfo,), {"stream": reader})
            with tarfile.open(fileobj=reader, mode="r:", tarinfo=info) as archive:
                for member in archive:
                    require(member.size >= 0, "invalid negative archive size")
                    body = None
                    if member.isfile():
                        require(member.size <= limits["file"], "oversized archive entry")
                        logical += member.size
                        require(logical <= limits["bytes"], "archive cumulative byte limit exceeded")
                        body = archive.extractfile(member).read()
                        require(len(body) == member.size, "incomplete archive entry")
                    members.append((member, body))
                # Tar iteration stops at the first zero header. Validate the
                # entire remainder and gzip trailer, including concatenated
                # streams, rather than accepting an early set of required files.
                padding = reader.tell() - archive.offset
                while chunk := reader.read(65536):
                    require(not any(chunk), "nonzero data after archive end")
                    padding += len(chunk)
                require(padding >= 1024, "incomplete archive end markers")
    except Rejected:
        raise
    except (tarfile.TarError, OSError, EOFError, zlib.error, ValueError, OverflowError, RecursionError):
        # zlib.error is not an OSError: a corrupt concatenated gzip member
        # raises it directly while the tail is drained.
        raise Rejected("invalid or incomplete release archive") from None
    return members
