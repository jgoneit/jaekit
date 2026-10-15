#!/usr/bin/env python3
"""Check a published release tag with the same rules as before publication.

Reads the public release metadata, downloads each asset within the archive
limits into a temporary directory, compares sizes and GitHub's digests, then
applies check_archives.check. Both published shapes are accepted: the four
macOS/Linux archives alone, or with the Windows archive. Nothing is installed
or uploaded. Exit 1 means the release failed a check; exit 2 means it could
not be read.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sys
import tempfile
from urllib.request import Request, urlopen

TAG = re.compile(r"v([0-9]+\.[0-9]+\.[0-9]+)")


def load_check_archives():
    path = Path(__file__).resolve().with_name("check_archives.py")
    spec = importlib.util.spec_from_file_location("jaekit_check_archives", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


check_archives = load_check_archives()


def fetch(url: str, limit: int, accept: str = "application/octet-stream") -> bytes:
    request = Request(url, headers={"Accept": accept, "User-Agent": "jaekit-release-check"})
    with urlopen(request, timeout=60) as response:
        data = response.read(limit + 1)
    if len(data) > limit:
        raise ValueError("download exceeds the release archive limit: " + url)
    return data


def release(repo: str, tag: str):
    data = fetch(f"https://api.github.com/repos/{repo}/releases/tags/{tag}", 4 * 1024 * 1024,
                 "application/vnd.github+json")
    value = json.loads(data.decode("utf-8"))
    if value.get("tag_name") != tag or value.get("draft") is not False:
        raise ValueError("the tag has no published release")
    return value


def check_release(repo: str, tag: str, metadata=None, download=None):
    match = TAG.fullmatch(tag)
    if not match:
        raise ValueError("tag must look like v0.1.5")
    version = match[1]
    metadata = metadata or release(repo, tag)
    download = download or (lambda url: fetch(url, check_archives.ARCHIVE_MAX_COMPRESSED_BYTES))
    assets = metadata.get("assets", [])
    names = [asset.get("name") for asset in assets]
    allowed = check_archives.release_names(version) | {check_archives.windows_name(version), "checksums.txt"}
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
        return check_archives.check(dist, version, published)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repo", default="jgoneit/jaekit")
    args = parser.parse_args(argv)
    try:
        archives = check_release(args.repo, args.tag)
    except OSError as error:
        # The release could not be read: no judgement about its files.
        print("ERROR: " + str(error), file=sys.stderr)
        return 2
    except (ValueError, KeyError, TypeError) as error:
        print("FAIL: " + str(error), file=sys.stderr)
        return 1
    windows = "with" if any(name.endswith(".zip") for name in archives) else "without"
    print(f"PASS: {args.tag} publishes {len(archives)} archives {windows} the Windows archive, and checksums.txt")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
