"""Acceptance checks using published text and isolated temporary data only.

The real HOME, installed files, and repository documents are never modified.
These checks deliberately live outside the ordinary regression discovery paths.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
GUIDES = {"INSTALL.md": "## 업데이트", "INSTALL.en.md": "## Update"}
CURRENT = "0.1.3"
OLDER = ("0.0.9", "0.0.10", "0.1.1")
NEWER = ("0.1.4", "0.1.10", "0.2.0", "1.0.0")
SHELLS = ("/bin/bash", "/bin/zsh")


def cleanup_block(case, filename):
    path = ROOT / "guides" / filename
    case.assertTrue(path.is_file(), "published installation guide is missing")
    document = path.read_text(encoding="utf-8")
    heading = GUIDES[filename] + "\n"
    case.assertIn(heading, document, "published update section is missing")
    section = document.split(heading, 1)[1].split("\n## ", 1)[0]
    blocks = [b for b in re.findall(r"```bash\n(.*?)```", section, re.S)
              if ".local/share/jaekit" in b]
    case.assertEqual(len(blocks), 1, "update needs one public data cleanup block")
    return blocks[0]


def run_cleanup(block, directory, shell, commands=None):
    if not Path(shell).is_file():
        raise RuntimeError("required test shell is unavailable: " + shell)
    env = dict(os.environ, REVIEW_INSTALL_ROOT=str(directory))
    if commands:
        env["PATH"] = str(commands) + os.pathsep + env.get("PATH", "")
    script = block.replace("$HOME", "$REVIEW_INSTALL_ROOT").replace("~/", '"$REVIEW_INSTALL_ROOT"/')
    if "$HOME" in script or "${HOME}" in script or "~/" in script:
        raise RuntimeError("fixture did not isolate all installation paths")
    argv = [shell, "-f", "-c", script] if shell.endswith("zsh") else [shell, "-c", script]
    return subprocess.run(argv, cwd=directory, env=env, capture_output=True, text=True, timeout=20)


def data_tree(directory):
    share = directory / ".local/share/jaekit"
    share.mkdir(parents=True)
    for version in OLDER + (CURRENT,) + NEWER:
        (share / version).mkdir()
        (share / version / "kept.txt").write_bytes(b"synthetic release data\n")
    return share


def assert_bytes(case, path, expected):
    case.assertTrue(path.is_file(), "required preserved file is missing: " + str(path))
    case.assertEqual(path.read_bytes(), expected)


class AC7(unittest.TestCase):
    def test_whole_names_and_entry_types_in_both_shells(self):
        for filename in GUIDES:
            for shell in SHELLS:
                for kind in ("file", "file-link", "directory-link"):
                    with self.subTest(guide=filename, shell=shell, entry=kind), \
                            tempfile.TemporaryDirectory(prefix="jaekit-review-cleanup-") as name:
                        directory = Path(name)
                        share = data_tree(directory)
                        protected = ("notes\n0.1.2", "notes old", "0.1.2-rc1", "v0.1.2")
                        for entry in protected:
                            (share / entry).mkdir()
                            (share / entry / "kept.txt").write_bytes(b"unrelated data\n")
                        target = directory / "outside"
                        collision = share / "0.1.2"
                        if kind == "file":
                            collision.write_bytes(b"ordinary file must stay\n")
                        elif kind == "file-link":
                            target.write_bytes(b"link target must stay\n")
                            collision.symlink_to(target)
                        else:
                            target.mkdir()
                            (target / "kept.txt").write_bytes(b"link target must stay\n")
                            collision.symlink_to(target, target_is_directory=True)
                        block = cleanup_block(self, filename)
                        result = run_cleanup(block, directory, shell)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertTrue(all(not (share / v).exists() for v in OLDER),
                                        "ordinary older versions were not removed")
                        for version in (CURRENT,) + NEWER:
                            assert_bytes(self, share / version / "kept.txt", b"synthetic release data\n")
                        self.assertTrue(all((share / p / "kept.txt").is_file() for p in protected),
                                        "a non-version name was removed")
                        if kind == "file":
                            self.assertTrue(collision.is_file(), "newline name selected a separate ordinary file")
                            self.assertEqual(collision.read_bytes(), b"ordinary file must stay\n")
                        else:
                            self.assertTrue(collision.is_symlink(), "newline name removed a separate symlink")
                            self.assertEqual(collision.readlink(), target)
                            kept = target if kind == "file-link" else target / "kept.txt"
                            assert_bytes(self, kept, b"link target must stay\n")
                        before = sorted(p.name for p in share.iterdir())
                        result = run_cleanup(block, directory, shell)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertEqual(sorted(p.name for p in share.iterdir()), before)

    def test_missing_and_linked_roots_preserve_data(self):
        for filename in GUIDES:
            for shell in SHELLS:
                with self.subTest(guide=filename, shell=shell), \
                        tempfile.TemporaryDirectory(prefix="jaekit-review-roots-") as name:
                    directory = Path(name)
                    block = cleanup_block(self, filename)
                    result = run_cleanup(block, directory, shell)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    share = directory / ".local/share/jaekit"
                    self.assertFalse(share.exists())
                    outside = directory / "outside"
                    (outside / "0.1.2").mkdir(parents=True)
                    (outside / "0.1.2/kept.txt").write_bytes(b"outside stays\n")
                    share.parent.mkdir(parents=True)
                    share.symlink_to(outside, target_is_directory=True)
                    result = run_cleanup(block, directory, shell)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertTrue(share.is_symlink())
                    assert_bytes(self, outside / "0.1.2/kept.txt", b"outside stays\n")

    def test_current_reference_files_can_still_be_copied(self):
        files = {"tools/check-result-reference.py": "tools/check-result-reference.py",
                 "examples/check-result/declaration.json": "checks/declaration.json",
                 "examples/check-result/reference.json": "checks/reference.json",
                 "examples/check-result/input.txt": "sample/input.txt"}
        for filename in GUIDES:
            for shell in SHELLS:
                with self.subTest(guide=filename, shell=shell), \
                        tempfile.TemporaryDirectory(prefix="jaekit-review-reference-") as name:
                    directory = Path(name)
                    share = data_tree(directory)
                    for relative in files:
                        path = share / CURRENT / relative
                        path.parent.mkdir(parents=True, exist_ok=True)
                        path.write_bytes(b"synthetic copied reference\n")
                    result = run_cleanup(cleanup_block(self, filename), directory, shell)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    document = (ROOT / "guides" / filename).read_text(encoding="utf-8")
                    blocks = re.findall(r"```bash\n(.*?)```", document, re.S)
                    selection = [b for b in blocks if b.startswith('package_root="$HOME/.local/share/jaekit/')]
                    copying = [b for b in blocks if 'cp "$package_root/' in b]
                    self.assertEqual((len(selection), len(copying)), (1, 1), "reference acquisition blocks are missing")
                    result = run_cleanup(selection[0] + copying[0], directory, shell)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    for destination in files.values():
                        assert_bytes(self, directory / destination, b"synthetic copied reference\n")


class AC8(unittest.TestCase):
    def test_published_blocks_match(self):
        self.assertEqual(cleanup_block(self, "INSTALL.md"), cleanup_block(self, "INSTALL.en.md"))

    def test_enumeration_classification_and_deletion_failures_are_non_success(self):
        # Inject failures in the tools used by the published cleanup. No real
        # commands or installed files are replaced, and HOME is unchanged.
        for filename in GUIDES:
            for shell in SHELLS:
                for command in ("find", "awk", "rm"):
                    with self.subTest(guide=filename, shell=shell, command=command), \
                            tempfile.TemporaryDirectory(prefix="jaekit-review-errors-") as name:
                        directory = Path(name)
                        share = data_tree(directory)
                        commands = directory / "commands"
                        commands.mkdir()
                        wrapper = commands / command
                        wrapper.write_text("#!/bin/sh\nprintf '%s\\n' 'synthetic " + command + " failure' >&2\nexit 73\n")
                        wrapper.chmod(0o755)
                        block = cleanup_block(self, filename)
                        result = run_cleanup(block, directory, shell, commands)
                        self.assertIn("synthetic " + command + " failure", result.stderr,
                                      "failure injection did not reach the cleanup tool")
                        self.assertNotEqual(result.returncode, 0, "cleanup hid a " + command + " failure")
                        for version in (CURRENT,) + NEWER:
                            assert_bytes(self, share / version / "kept.txt", b"synthetic release data\n")
                        # The error is not sticky: a later ordinary invocation
                        # can finish eligible cleanup without changing survivors.
                        result = run_cleanup(block, directory, shell)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertTrue(all(not (share / v).exists() for v in OLDER))
                        self.assertTrue(all((share / v / "kept.txt").is_file() for v in (CURRENT,) + NEWER))


GO_HELPERS = '''package plugins
import ("encoding/json"; "os"; "testing")
func read(t *testing.T, path string) string {
 t.Helper(); data, err := os.ReadFile(path); if err != nil { t.Fatal(err) }; return string(data)
}
func readJSON(t *testing.T, path string, value any) {
 t.Helper(); if err := json.Unmarshal([]byte(read(t, path)), value); err != nil { t.Fatal(err) }
}
'''


class AC12(unittest.TestCase):
    def check_claim(self, relative, claim, rejected):
        source = ROOT / "plugins/current_release_test.go"
        self.assertTrue(source.is_file(), "current release checker is missing")
        if not shutil.which("go"):
            raise RuntimeError("Go is required for the release checker fixture")
        with tempfile.TemporaryDirectory(prefix="jaekit-review-claims-") as name:
            fixture = Path(name)
            (fixture / "plugins").mkdir()
            shutil.copyfile(source, fixture / "plugins/current_release_test.go")
            (fixture / "plugins/helpers_test.go").write_text(GO_HELPERS)
            (fixture / "go.mod").write_text("module example.invalid/releaseclaims\n\ngo 1.20\n")
            (fixture / ".claude-plugin").mkdir()
            (fixture / ".claude-plugin/marketplace.json").write_text(json.dumps({"metadata": {"version": CURRENT}}))
            for path in ("README.md", "README.en.md", "guides/USAGE.md", "examples/README.md"):
                file = fixture / path
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text("# Synthetic public document\n\n")
            (fixture / relative).write_text("# Synthetic public document\n\n" + claim)
            env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
            result = subprocess.run(["go", "test", "-json", "-count=1", "./plugins", "-run",
                                     "^TestPublicGuidanceNamesOnlyTheCurrentRelease$"],
                                    cwd=fixture, env=env, capture_output=True, text=True, timeout=60)
            events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]
            test = "TestPublicGuidanceNamesOnlyTheCurrentRelease"
            final = [e["Action"] for e in events if e.get("Test") == test and e.get("Action") in ("pass", "fail")]
            if len(final) != 1:
                raise RuntimeError("release checker did not run: " + result.stdout + result.stderr)
            output = "".join(e.get("Output", "") for e in events if e.get("Test") == test)
            if rejected:
                self.assertEqual(final, ["fail"], "stale visible release claim was accepted: " + claim)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(relative + ":3 names v0.1.2", output,
                              "rejection omitted the file or original statement line")
            else:
                self.assertEqual(final, ["pass"], "non-contradictory text was rejected: " + output)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_linked_visible_versions_are_rejected(self):
        claims = (
            ("README.en.md", "The current release is [v0.1.2](https://example.invalid/releases).\n"),
            ("guides/USAGE.md", "현재 배포판은\n**[v0.1.2](https://example.invalid/releases)**입니다.\n"),
            ("examples/README.md", "현재\n배포판은 [`v0.1.2`](https://example.invalid/releases)입니다.\n"),
            ("README.en.md", "The current release\nis [**v0.1.2**](https://example.invalid/releases).\n"),
            ("README.en.md", "The current release is `[v0.1.2](https://example.invalid/releases)`.\n"),
        )
        for path, claim in claims:
            with self.subTest(path=path, claim=claim):
                self.check_claim(path, claim, True)

    def test_urls_and_separate_paragraphs_are_not_release_claims(self):
        claims = (
            "The current release is [v0.1.3](https://example.invalid/v0.1.2).\n",
            "The current release is [download](https://example.invalid/v0.1.2).\n",
            "현재 배포판은 [**v0.1.3**](https://example.invalid/v0.1.2)입니다.\n",
            "The current release\n\n[v0.1.2](https://example.invalid/old).\n",
            "The current release\r\n\r\n[v0.1.2](https://example.invalid/old).\r\n",
        )
        for claim in claims:
            with self.subTest(claim=claim):
                self.check_claim("README.md", claim, False)

    def test_existing_plain_and_wrapped_claims_remain_rejected(self):
        for claim in ("The current release is v0.1.2.\n", "현재 배포판은\n**v0.1.2**입니다.\n"):
            with self.subTest(claim=claim):
                self.check_claim("guides/USAGE.md", claim, True)


if __name__ == "__main__":
    unittest.main()
