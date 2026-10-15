#!/usr/bin/env python3
"""Check a published release tag with the same rules as before publication.

Reads the public release metadata, downloads each asset within the archive
limits into a temporary directory, compares sizes and GitHub's digests, then
applies check_archives.check. Both published shapes are accepted: the four
macOS/Linux archives alone, or with the Windows archive. A published Windows
archive also needs its Windows 11 acceptance link: the tag must be annotated
and its message must link exactly that archive's SHA-256, by the rule the
publication gate applies (windows_gate.accepted). The tag is read only when
the Windows archive is present. Nothing is installed or uploaded. Exit 1 means
the release failed a check; exit 2 means the release or its tag could not be
read or interpreted.
"""
from __future__ import annotations

import argparse
import hashlib
from http.client import HTTPException
import importlib.util
import json
from pathlib import Path
import re
import sys
import tempfile
from urllib.request import Request, urlopen

TAG = re.compile(r"v([0-9]+\.[0-9]+\.[0-9]+)")
OBJECT = re.compile(r"[0-9a-f]{40}|[0-9a-f]{64}")


class Unreadable(OSError):
    """A response could not be interpreted: no judgement about the release."""


def load_sibling(name: str, filename: str):
    path = Path(__file__).resolve().with_name(filename)
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


check_archives = load_sibling("jaekit_check_archives", "check_archives.py")
windows_gate = load_sibling("jaekit_windows_gate", "windows_gate.py")


def fetch(url: str, limit: int, accept: str = "application/octet-stream") -> bytes:
    request = Request(url, headers={"Accept": accept, "User-Agent": "jaekit-release-check"})
    try:
        with urlopen(request, timeout=60) as response:
            data = response.read(limit + 1)
    except HTTPException as error:
        # A truncated or malformed HTTP response, such as IncompleteRead.
        raise Unreadable(f"the response could not be read ({type(error).__name__}): {url}") from None
    if len(data) > limit:
        raise ValueError("download exceeds the release archive limit: " + url)
    return data


def fetch_json(url: str, limit: int) -> dict:
    try:
        data = fetch(url, limit, "application/vnd.github+json")
    except ValueError:
        raise Unreadable("the response exceeds its read limit: " + url) from None
    try:
        value = json.loads(data.decode("utf-8"))
    except (UnicodeError, json.JSONDecodeError):
        raise Unreadable("the response is not JSON: " + url) from None
    if not isinstance(value, dict):
        raise Unreadable("the response is not a JSON object: " + url)
    return value


def release(repo: str, tag: str):
    value = fetch_json(f"https://api.github.com/repos/{repo}/releases/tags/{tag}", 4 * 1024 * 1024)
    if value.get("tag_name") != tag or value.get("draft") is not False:
        raise ValueError("the tag has no published release")
    return value


def read_tag(repo: str, tag: str) -> tuple[bool, str]:
    """Return (annotated, message) for the tag.

    For an annotated tag this is the message of the tag object the reference
    names, the same object the publication job reads with `git cat-file tag`.
    A lightweight tag has no message.
    """
    ref = fetch_json(f"https://api.github.com/repos/{repo}/git/ref/tags/{tag}", 4 * 1024 * 1024)
    target = ref.get("object")
    if (ref.get("ref") != "refs/tags/" + tag or not isinstance(target, dict)
            or not isinstance(target.get("type"), str) or not OBJECT.fullmatch(str(target.get("sha")))):
        raise Unreadable("the tag reference response is malformed: " + tag)
    if target["type"] != "tag":
        return False, ""
    value = fetch_json(f"https://api.github.com/repos/{repo}/git/tags/{target['sha']}", 4 * 1024 * 1024)
    if value.get("sha") != target["sha"] or not isinstance(value.get("message"), str):
        raise Unreadable("the tag object response is malformed: " + tag)
    return True, value["message"]


def verify_link(annotated: bool, message: str, digest: str):
    """The publication gate's rule for keeping the Windows archive."""
    if not annotated:
        raise ValueError("the Windows archive is published under a lightweight tag, without its acceptance link")
    link, reason = windows_gate.accepted(message)
    if link is None:
        raise ValueError("the Windows archive is published without its acceptance link: " + reason)
    if link != digest:
        raise ValueError("the tag's acceptance link names another archive digest")


def check_release(repo: str, tag: str, metadata=None, download=None, tag_reader=None):
    match = TAG.fullmatch(tag)
    if not match:
        raise ValueError("tag must look like v0.1.5")
    version = match[1]
    metadata = metadata or release(repo, tag)
    download = download or (lambda url: fetch(url, check_archives.ARCHIVE_MAX_COMPRESSED_BYTES))
    tag_reader = tag_reader or read_tag
    assets = metadata.get("assets", [])
    names = [asset.get("name") for asset in assets]
    windows = check_archives.windows_name(version)
    allowed = check_archives.release_names(version) | {windows, "checksums.txt"}
    if len(names) != len(set(names)) or not set(names) <= allowed:
        raise ValueError("the published release has unexpected or repeated assets")
    with tempfile.TemporaryDirectory(prefix="jaekit-published-") as directory:
        dist = Path(directory) / "dist"
        dist.mkdir()
        for asset in assets:
            name = asset["name"]
            url = f"https://github.com/{repo}/releases/download/{tag}/{name}"
            if asset.get("browser_download_url") != url:
                raise ValueError("asset URL disagrees with the tag: " + name)
            data = download(url)
            if len(data) != asset.get("size"):
                raise ValueError("downloaded size differs from public metadata: " + name)
            digest = asset.get("digest")
            if digest is not None and digest != "sha256:" + hashlib.sha256(data).hexdigest():
                raise ValueError("downloaded digest differs from public metadata: " + name)
            (dist / name).write_bytes(data)
        published = Path(directory) / "published-assets.txt"
        published.write_text("".join(name + "\n" for name in names), encoding="utf-8")
        archives = check_archives.check(dist, version, published)
        if windows in archives:
            annotated, message = tag_reader(repo, tag)
            verify_link(annotated, message, check_archives.digest(dist / windows))
        return archives


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repo", default="jgoneit/jaekit")
    args = parser.parse_args(argv)
    try:
        archives = check_release(args.repo, args.tag)
    except OSError as error:
        # The release or its tag could not be read: no judgement about its files.
        print("ERROR: " + str(error), file=sys.stderr)
        return 2
    except (ValueError, KeyError, TypeError) as error:
        print("FAIL: " + str(error), file=sys.stderr)
        return 1
    windows = ("with the Windows archive linked to its Windows 11 acceptance"
               if any(name.endswith(".zip") for name in archives) else "without the Windows archive")
    print(f"PASS: {args.tag} publishes {len(archives)} archives {windows}, and checksums.txt")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
