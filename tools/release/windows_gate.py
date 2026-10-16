#!/usr/bin/env python3
"""Select the files a tag publishes, before anything is uploaded.

The Windows job must have checked exactly the archive about to be published;
otherwise nothing is published. The Windows archive stays only when the
tag's reviewed release notes (tools/release/notes/v<version>.md, read from the
tagged checkout) carry exactly one line

    Windows-Accepted-SHA256: <64 lowercase hex>

whose value is that archive's SHA-256, the value the Windows 11 acceptance
procedure (tools/windows/README.md) reports. Without such a link the archive
is removed and checksums.txt lists the four macOS/Linux archives only.
No download, upload or tag change happens here.
"""
from __future__ import annotations

import argparse
import hashlib
import os
from pathlib import Path
import re
import sys

LINK = re.compile(r"Windows-Accepted-SHA256:(.*)")
HEX = re.compile(r"[0-9a-f]{64}")
VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+")


class Refused(Exception):
    """Publication must stop."""


def sha256(path: Path) -> str:
    result = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def accepted(message: str) -> tuple[str | None, str]:
    """Return the linked digest, or None with the reason there is no link."""
    values = []
    for line in message.splitlines():
        match = LINK.fullmatch(line.rstrip("\r"))
        if match:
            values.append(match[1].strip())
    if not values:
        return None, "the release notes have no Windows-Accepted-SHA256 line"
    if len(values) > 1:
        return None, "the release notes have more than one Windows-Accepted-SHA256 line"
    if not HEX.fullmatch(values[0]):
        return None, "the Windows-Accepted-SHA256 value is not 64 lowercase hex digits"
    return values[0], ""


def select(dist: Path, version: str, checked: str, message: str) -> tuple[bool, str]:
    """Apply the decision to dist. Returns (Windows archive kept, reason)."""
    if not VERSION.fullmatch(version):
        raise Refused("version must be a release number such as 0.1.5")
    if not HEX.fullmatch(checked or ""):
        raise Refused("the Windows job reported no checked archive digest")
    archive = dist / f"ha_{version}_windows_amd64.zip"
    checksums = dist / "checksums.txt"
    if not archive.is_file() or archive.is_symlink() or not checksums.is_file() or checksums.is_symlink():
        raise Refused("the release files have no Windows archive or checksums.txt")
    actual = sha256(archive)
    if actual != checked:
        raise Refused("the Windows job checked a different archive; nothing is published")
    lines = checksums.read_text(encoding="utf-8").splitlines()
    windows = [line for line in lines if line.endswith("  " + archive.name)]
    if windows != [actual + "  " + archive.name]:
        raise Refused("checksums.txt does not list the checked Windows archive exactly once")
    link, reason = accepted(message)
    if link is not None and link == actual:
        return True, "the release notes link the Windows 11 acceptance of this archive"
    if link is not None:
        reason = "the release notes link another archive digest"
    archive.unlink()
    rest = [line for line in lines if line not in windows]
    checksums.write_text("".join(line + "\n" for line in rest), encoding="utf-8")
    return False, reason


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", required=True, type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--checked-sha256", required=True)
    parser.add_argument("--notes", required=True, type=Path)
    args = parser.parse_args(argv)
    try:
        message = args.notes.read_text(encoding="utf-8")
        kept, reason = select(args.dist, args.version, args.checked_sha256, message)
    except (OSError, UnicodeError, Refused) as error:
        print("REFUSED: " + str(error), file=sys.stderr)
        return 1
    decision = ("Windows archive published: " if kept else "Windows archive left out: ") + reason
    print(decision)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as output:
            output.write(decision + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
