"""Read-only, byte-level source identity for a selected release acceptance run."""
from contextlib import contextmanager
import hashlib
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile


class SourceMismatch(Exception):
    pass


class SourceUnavailable(Exception):
    pass


def git(root, *arguments, input=None):
    # Repository selection belongs to the explicit cwd. Inherited Git path or
    # config overrides must never redirect snapshot writes into another repo.
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    env.update(GIT_OPTIONAL_LOCKS="0", GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
               GIT_NO_LAZY_FETCH="1", GIT_TERMINAL_PROMPT="0")
    try:
        result = subprocess.run(["git", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", *arguments],
                                cwd=root, input=input, capture_output=True, env=env, timeout=30)
    except (OSError, subprocess.TimeoutExpired):
        raise SourceUnavailable("source repository could not be read") from None
    if result.returncode:
        raise SourceUnavailable("source repository could not be read")
    return result.stdout


def snapshot(root, expected_tree, loaded):
    root = Path(root).resolve()
    tree = git(root, "rev-parse", "HEAD^{tree}").decode().strip()
    if tree != expected_tree:
        raise SourceMismatch("checked source tree differs from the verified release source")
    if git(root, "diff", "--name-only", "-z", "HEAD", "--"):
        raise SourceMismatch("checked source has staged or unstaged changes")
    entries = []
    for row in git(root, "ls-tree", "-rz", "HEAD").split(b"\0"):
        if row:
            metadata, name = row.split(b"\t", 1)
            mode, kind, oid = metadata.split()
            if kind != b"blob":
                raise SourceUnavailable("unsupported source entry")
            entries.append((mode, oid, os.fsdecode(name)))
    if not entries:
        raise SourceMismatch("verified source tree is empty")
    objects = git(root, "cat-file", "--batch", input=b"".join(oid + b"\n" for _, oid, _ in entries))
    cursor, files, originals = 0, {}, {}
    for mode, oid, name in entries:
        end = objects.find(b"\n", cursor)
        header = objects[cursor:end].split()
        if len(header) != 3 or header[:2] != [oid, b"blob"]:
            raise SourceMismatch("source object response is incomplete")
        size = int(header[2]); cursor = end + 1
        expected = objects[cursor:cursor + size]; cursor += size + 1
        path = root / name
        try:
            info = path.lstat()
            if mode == b"120000":
                if not stat.S_ISLNK(info.st_mode):
                    raise SourceMismatch("source file type changed")
                actual = os.fsencode(os.readlink(path))
            else:
                if not stat.S_ISREG(info.st_mode) or bool(info.st_mode & 0o111) != (mode == b"100755"):
                    raise SourceMismatch("source file mode changed")
                actual = path.read_bytes()
        except FileNotFoundError:
            raise SourceMismatch("source file is missing") from None
        except OSError:
            raise SourceUnavailable("source file is unreadable") from None
        if actual != expected:
            raise SourceMismatch("checked source bytes differ from the verified source")
        files[name] = hashlib.sha256(actual).hexdigest()
        originals[name] = (mode, expected)
    # Root docs are ignored by repository policy. Untracked Python source next
    # to the reader can shadow a normal import, so it cannot supply acceptance.
    for name in git(root, "ls-files", "--others", "--exclude-standard", "-z").split(b"\0"):
        if name.startswith(b"tools/release-checks/") and name.endswith(b".py"):
            raise SourceMismatch("untracked observation source may change executed imports")
    for path, original in loaded.items():
        try:
            relative = Path(path).resolve().relative_to(root).as_posix()
        except ValueError:
            raise SourceMismatch("executed observation module is outside the verified source") from None
        if files.get(relative) != hashlib.sha256(original).hexdigest():
            raise SourceMismatch("loaded observation module differs from the verified source")
    return {"head": git(root, "rev-parse", "HEAD"), "index": git(root, "ls-files", "--stage", "-z"),
            "files": files, "originals": originals}


@contextmanager
def verified(root, expected_tree, loaded):
    before = snapshot(root, expected_tree, loaded)
    try:
        yield before
    finally:
        if snapshot(root, expected_tree, loaded) != before:
            raise SourceMismatch("source identity changed during the selected observation")


def check_snapshot_links(selected, originals):
    """Only relative links terminating at a verified regular file are supported.

    Resolution never reads a link target's contents. No product command or Git
    setup runs until every link is confined to this newly materialized tree.
    Directory aliases are unsupported, so traversal cycles cannot enter a run.
    """
    root = selected.resolve()
    for name, (mode, data) in originals.items():
        if mode != b"120000":
            continue
        if Path(os.fsdecode(data)).is_absolute():
            raise SourceUnavailable("absolute snapshot symlinks are unsupported")
        try:
            resolved = (selected / name).resolve(strict=True)
            relative = resolved.relative_to(root).as_posix()
        except (OSError, RuntimeError, ValueError):
            raise SourceUnavailable("snapshot symlink escapes, is dangling, or is cyclic") from None
        target = originals.get(relative)
        if target is None or target[0] not in (b"100644", b"100755"):
            raise SourceUnavailable("snapshot symlink does not identify a verified regular file")


def check_materialization_paths(originals):
    names = set(originals)
    for name in names:
        path = Path(name)
        if (path.is_absolute() or path.as_posix() != name
                or any(part in (".", "..") or part.casefold() == ".git" for part in path.parts)
                or any(parent.as_posix() in names for parent in path.parents if parent != Path("."))):
            raise SourceUnavailable("snapshot contains an unsupported or conflicting path")


@contextmanager
def pinned(root, expected_tree, loaded):
    """Execute later reads from verified object bytes, not a mutable checkout.

    Only the current tree is copied. Its independent synthetic Git commit gives
    source tools a repository without depending on, or modifying, old history.
    A change and restoration of the original checkout cannot alter these bytes.
    """
    with verified(root, expected_tree, loaded) as before:
        check_materialization_paths(before["originals"])
        with tempfile.TemporaryDirectory(prefix="jaekit-verified-source-") as directory:
            selected = Path(directory)
            for name, (mode, data) in before["originals"].items():
                destination = selected / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                if mode == b"120000":
                    destination.symlink_to(os.fsdecode(data))
                else:
                    destination.write_bytes(data)
                    destination.chmod(0o755 if mode == b"100755" else 0o644)
            check_snapshot_links(selected, before["originals"])
            git(selected, "init", "-q")
            git(selected, "config", "core.hooksPath", "/dev/null")
            git(selected, "config", "user.name", "Jaekit source snapshot")
            git(selected, "config", "user.email", "snapshot@example.invalid")
            git(selected, "config", "commit.gpgsign", "false")
            # Recreate the exact tree rather than passing verified bytes
            # through ignore rules, clean filters or line-ending conversion.
            # NUL-delimited index input preserves every supported path name.
            index = []
            for name, (mode, data) in before["originals"].items():
                oid = git(selected, "hash-object", "-w", "--stdin", input=data).strip()
                index.append(mode + b" " + oid + b"\t" + os.fsencode(name) + b"\0")
            git(selected, "update-index", "-z", "--index-info", input=b"".join(index))
            git(selected, "commit", "-qm", "Verified current source bytes")
            if git(selected, "rev-parse", "HEAD^{tree}").decode().strip() != expected_tree:
                raise SourceMismatch("isolated source copy differs from the verified tree")
            yield selected


@contextmanager
def pinned_product(root, commit, tree):
    """Read explicit public commit objects into an independent product tree.

    This is separate from the reader's checked worktree. It never fetches,
    resets, cleans, or checks out files in the source repository. The caller
    supplies the commit and tree already identified by the public release.
    Missing local objects remain unavailable instead of falling back to HEAD.
    """
    if not all(isinstance(value, str) and re.fullmatch(r"[0-9a-f]{40}", value)
               for value in (commit, tree)):
        raise SourceUnavailable("public product commit and tree identities are missing or unsupported")
    root = Path(root).resolve()
    kind = git(root, "cat-file", "-t", commit).strip()
    if kind != b"commit":
        raise SourceMismatch("public product identity does not identify a commit")
    actual_tree = git(root, "rev-parse", commit + "^{tree}").decode().strip()
    if actual_tree != tree:
        raise SourceMismatch("public product commit differs from the verified release tree")
    entries = []
    names = set()
    for row in git(root, "ls-tree", "-rz", tree).split(b"\0"):
        if not row:
            continue
        metadata, name = row.split(b"\t", 1)
        mode, object_kind, oid = metadata.split()
        decoded = os.fsdecode(name)
        path = Path(decoded)
        if (object_kind != b"blob" or mode not in (b"100644", b"100755", b"120000")
                or path.is_absolute() or any(part in (".", "..") or part.casefold() == ".git" for part in path.parts)
                or decoded in names or path.as_posix() != decoded):
            raise SourceUnavailable("public product tree contains an unsupported entry")
        names.add(decoded)
        entries.append((mode, oid, decoded))
    if not entries:
        raise SourceUnavailable("public product tree has no files")
    if any(parent.as_posix() in names for _, _, name in entries
           for parent in Path(name).parents if parent != Path(".")):
        raise SourceUnavailable("public product tree has conflicting file and directory paths")
    objects = git(root, "cat-file", "--batch", input=b"".join(oid + b"\n" for _, oid, _ in entries))
    cursor, files = 0, []
    for mode, oid, name in entries:
        end = objects.find(b"\n", cursor)
        header = objects[cursor:end].split()
        if len(header) != 3 or header[:2] != [oid, b"blob"]:
            raise SourceUnavailable("public product object is missing or unreadable")
        try:
            size = int(header[2])
        except ValueError:
            raise SourceUnavailable("public product object size is unreadable") from None
        cursor = end + 1
        data = objects[cursor:cursor + size]
        if size < 0 or len(data) != size or objects[cursor + size:cursor + size + 1] != b"\n":
            raise SourceUnavailable("public product object is truncated")
        cursor += size + 1
        files.append((mode, oid, name, data))
    if cursor != len(objects):
        raise SourceUnavailable("public product object response contains unrelated bytes")
    with tempfile.TemporaryDirectory(prefix="jaekit-public-product-") as directory:
        selected = Path(directory)
        try:
            originals = {name: (mode, data) for mode, _, name, data in files}
            check_materialization_paths(originals)
            for mode, _, name, data in files:
                destination = selected / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                if mode == b"120000":
                    destination.symlink_to(os.fsdecode(data))
                else:
                    with destination.open("xb") as output:
                        output.write(data)
                    destination.chmod(0o755 if mode == b"100755" else 0o644)
            check_snapshot_links(selected, originals)
            git(selected, "init", "-q")
            git(selected, "config", "core.hooksPath", "/dev/null")
            git(selected, "config", "user.name", "Jaekit public product snapshot")
            git(selected, "config", "user.email", "snapshot@example.invalid")
            git(selected, "config", "commit.gpgsign", "false")
            index = []
            for mode, expected_oid, name, data in files:
                oid = git(selected, "hash-object", "-w", "--stdin", input=data).strip()
                if oid != expected_oid:
                    raise SourceMismatch("materialized public product object differs")
                index.append(mode + b" " + oid + b"\t" + os.fsencode(name) + b"\0")
            git(selected, "update-index", "-z", "--index-info", input=b"".join(index))
            git(selected, "commit", "-qm", "Verified public product source")
            if git(selected, "rev-parse", "HEAD^{tree}").decode().strip() != tree:
                raise SourceMismatch("materialized public product differs from the verified tree")
        except OSError:
            raise SourceUnavailable("public product files could not be materialized") from None
        yield selected
