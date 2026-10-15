"""Windows archive names: the committed public files, `~` and the installer cases.

The committed public files are judged as the release build judges them:
package.py's layout with a synthetic Core, through the Windows archive rule.
A file the rule refuses fails here, at the pull request, rather than stopping
the release build. The installer itself runs only on Windows (release
workflow); here the comparison's cases, verdicts and report fields are checked.
"""
import importlib.util
import os
import sys
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
BODY = b"Synthetic public document.\n"
GIT = ("git", "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid",
       "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null")


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


contract = load("windows_names_contract", HERE / "windows_contract.py")
archives = contract.check_archives
package = archives.windows_package
releases = load("windows_names_release", HERE / "test_windows_release.py")


def required():
    """The public files the rule requires, without the Core and its manifest."""
    return {name: BODY for name in archives.WINDOWS_REQUIRED - {"ha.exe", "package.json"}}


def judge(files):
    """The release build's Windows archive of `files`, judged by the rule.
    Like package.py, each name is checked first, so a name a Windows-hosted
    zipfile would rewrite (a backslash) is still reported as written."""
    for name in files:
        package.safe_name(name)
    files = dict(files, **{"ha.exe": contract.pe_amd64()})
    data = contract.archive(files, contract.manifest(files))
    with tempfile.TemporaryDirectory(prefix="jaekit-windows-names-") as folder:
        path = Path(folder) / f"ha_{contract.VERSION}_windows_amd64.zip"
        path.write_bytes(data)
        return archives.windows_files(path, contract.sha256(data), contract.VERSION)


REFUSED = (
    (("guides/café.md",), "not printable ASCII"),
    (("guides/Node_Modules/notes.md",), "local metadata or generated cache"),
    (("guides/tool.PYC",), "local metadata or generated cache"),
    (("guides/CON.md",), "unsafe Windows package path"),
    (("guides/notes.md.",), "unsafe Windows package path"),
    (("guides/a\\b.md",), "unsafe Windows package path"),
    (("guides/Extra.md", "guides/extra.md"), "differ only in case"),
    (("guides/Extra", "guides/extra/notes.md"), "file/directory collision"),
    (("guides/abcdef~1.md",), "short name"),
)


class CommittedPublicFiles(unittest.TestCase):
    def test_committed_public_files_pass_the_windows_rule(self):
        # package.py packages exactly these files for the release; a file the
        # rule refuses would stop the whole release at its archive check. The
        # files staged for the next commit are judged as well, so a check run
        # before the commit already sees them; in a fresh checkout they are
        # the same files.
        for staged in (False, True):
            with self.subTest(staged=staged):
                try:
                    judge(package.committed_public_files(ROOT, staged=staged))
                except ValueError as error:
                    self.fail(f"a committed public file would stop the release's Windows archive: {error}")

    def repository(self, committed, staged=None, links=()):
        """A synthetic repository; files go through Git's objects only, so pairs a
        case-insensitive file system cannot hold still reach the commit."""
        folder = tempfile.TemporaryDirectory(prefix="jaekit-committed-names-")
        self.addCleanup(folder.cleanup)
        root = Path(folder.name)
        env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}

        def git(*args, data=None):
            return subprocess.run([*GIT, *args], cwd=root, env=env, input=data, capture_output=True,
                                  check=True).stdout

        def add(files, mode="100644"):
            for name, body in files.items():
                oid = git("hash-object", "-w", "--stdin", data=body).decode("ascii").strip()
                git("update-index", "--add", "--cacheinfo", f"{mode},{oid},{name}")

        git("init", "--quiet")
        add(committed)
        add({name: b"guides/WINDOWS.md" for name in links}, mode="120000")
        git("commit", "--quiet", "--no-verify", "-m", "synthetic public files")
        add(staged or {})
        return root

    def test_each_refused_committed_name_fails_with_its_path_and_reason(self):
        for names, reason in REFUSED:
            with self.subTest(names=names):
                root = self.repository(dict(required(), **dict.fromkeys(names, BODY)))
                with self.assertRaises(ValueError) as caught:
                    judge(package.committed_public_files(root))
                message = str(caught.exception)
                self.assertIn(reason, message)
                for name in names:
                    self.assertTrue(name in message or repr(name) in message, message)

    def test_a_committed_file_over_the_size_limit_is_named(self):
        large = b"\0" * (archives.WINDOWS_MAX_FILE_BYTES + 1)
        root = self.repository(dict(required(), **{"guides/large.bin": large}))
        with self.assertRaisesRegex(ValueError, "size limit: guides/large.bin"):
            judge(package.committed_public_files(root))

    def test_untracked_ignored_and_unstaged_files_do_not_count(self):
        root = self.repository(dict(required(), **{"docs/private.md": BODY}),
                               staged={"guides/WINDOWS.md": b"staged\n"})
        (root / "guides").mkdir()
        (root / "guides/untracked~1.md").write_bytes(BODY)
        (root / "guides/WINDOWS.md").write_bytes(b"changed in the working tree\n")
        (root / "guides/__pycache__").mkdir()
        (root / "guides/__pycache__/notes.pyc").write_bytes(BODY)
        self.assertEqual(required(), package.committed_public_files(root))
        staged = package.committed_public_files(root, staged=True)
        self.assertEqual(dict(required(), **{"guides/WINDOWS.md": b"staged\n"}), staged)
        judge(staged)

    def test_a_staged_file_the_rule_refuses_is_named(self):
        root = self.repository(required(), staged={"guides/CON.md": BODY})
        judge(package.committed_public_files(root))
        with self.assertRaisesRegex(ValueError, "unsafe Windows package path: 'guides/CON.md'"):
            judge(package.committed_public_files(root, staged=True))

    @unittest.skipIf(sys.platform == "win32", "Windows passes arguments as text, not bytes")
    def test_a_committed_path_that_is_not_utf8_is_named(self):
        root = self.repository(dict(required(), **{"guides/caf\udce9.md": BODY}))
        with self.assertRaisesRegex(ValueError, r"not UTF-8: 'guides/caf\\udce9\.md'"):
            package.committed_public_files(root)

    def test_a_source_snapshot_without_git_gives_its_public_files(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-snapshot-names-") as folder:
            root = Path(folder)
            for name, body in dict(required(), **{"guides/Node_Modules/notes.md": BODY,
                                                  "docs/private.md": BODY}).items():
                (root / name).parent.mkdir(parents=True, exist_ok=True)
                (root / name).write_bytes(body)
            files = package.committed_public_files(root)
            self.assertNotIn("docs/private.md", files)
            self.assertIn("guides/Node_Modules/notes.md", files)
            with self.assertRaisesRegex(ValueError, "local metadata or generated cache"):
                judge(files)

    def test_committed_links_are_refused(self):
        root = self.repository(required(), links=("guides/link.md",))
        with self.assertRaisesRegex(ValueError, "not a regular file: guides/link.md"):
            package.committed_public_files(root)


class ShortNames(releases.Fixture):
    def test_a_tilde_anywhere_in_an_entry_name_is_refused(self):
        # Every Windows short name holds `~`; an entry named like another
        # entry's short name would be the same file on such a volume.
        for name in ("guides/abcdef~1.md", "guides/notes~/WINDOWS.md", "contracts/archive~.md", "guides/~notes.md"):
            with self.subTest(name=name):
                with self.assertRaisesRegex(ValueError, "short name") as caught:
                    judge(dict(required(), **{name: BODY}))
                self.assertIn(name, str(caught.exception))

    def test_the_published_release_check_refuses_the_same_archive(self):
        # check_published.py judges a downloaded release with the same rule.
        page = releases.PublishedRelease
        name = "guides/abcdef~1.md"
        self.write(dict(self.files, **{name: BODY}))

        def unread(repo, tag):
            raise AssertionError("a refused archive is judged before its tag is read")

        with self.assertRaisesRegex(ValueError, "short name") as caught:
            releases.published.check_release("jgoneit/jaekit", "v" + releases.VERSION, page.metadata(self),
                                             lambda url: page.download(self, url), tag_reader=unread)
        self.assertIn(name, str(caught.exception))

    def test_collisions_name_both_paths(self):
        for names, reason in REFUSED[5:7]:
            with self.subTest(names=names):
                with self.assertRaisesRegex(ValueError, reason) as caught:
                    judge(dict(required(), **dict.fromkeys(names, BODY)))
                for name in names:
                    self.assertIn(repr(name), str(caught.exception))


class InstallerNameCases(unittest.TestCase):
    CHARACTERS = "\0\x01\x1f\\\":<>|?*"

    def test_each_name_character_has_a_case_the_rule_refuses(self):
        cases = {name: (expected, data) for name, expected, data in contract.cases()}
        for char in self.CHARACTERS:
            with self.subTest(char=repr(char)):
                names = [name for name, stored in contract.STORED_NAMES.items() if char in stored]
                self.assertEqual(1, len(names))
                expected, data = cases[names[0]]
                self.assertEqual("reject", expected)
                with tempfile.TemporaryDirectory() as folder:
                    path = Path(folder) / f"ha_{contract.VERSION}_windows_amd64.zip"
                    path.write_bytes(data)
                    with zipfile.ZipFile(path) as archive:
                        self.assertIn(contract.STORED_NAMES[names[0]],
                                      [entry.orig_filename for entry in archive.infolist()])
                    self.assertEqual("reject", contract.python_verdict(path)[0])

    def test_a_name_case_refused_without_the_installers_name_refusal_is_no_agreement(self):
        def incidental(shell, directory, destination):
            if shell == "powershell":
                return "reject", ("System.Management.Automation.PropertyNotFoundException: "
                                  "The property 'Count' cannot be found on this object.")
            return "reject", "System.Management.Automation.RuntimeException: Unsafe package path: LICENSE x"

        report = contract.contract(incidental)
        cases = {case["name"]: case for case in report["cases"]}
        for name in contract.STORED_NAMES:
            self.assertEqual({"powershell": "other", "pwsh": "reject"}, cases[name]["installer"])
        self.assertFalse(report["passed"])
        denied = contract.classify("reject", "System.IO.IOException: The process cannot access the file.",
                                   contract.NAME_REFUSAL)
        self.assertEqual("error", denied)

    def test_the_report_names_stored_entries_and_whether_the_installer_wrote(self):
        def installer(shell, directory, destination):
            if self.current(directory) == "normal":
                destination.mkdir()
                return "accept", ""
            return "reject", "System.Management.Automation.RuntimeException: Unsafe package path: x"

        report = contract.contract(installer)
        cases = {case["name"]: case for case in report["cases"]}
        self.assertEqual({"powershell": True, "pwsh": True}, cases["normal"]["installer_wrote"])
        for name, stored in contract.STORED_NAMES.items():
            self.assertEqual(stored, cases[name]["stored_name"])
            self.assertEqual({"powershell": False, "pwsh": False}, cases[name]["installer_wrote"])
        self.assertIsNone(cases["normal"]["stored_name"])

    def current(self, directory):
        number = int(directory.name.removeprefix("case-"))
        return contract.cases()[number][0]


if __name__ == "__main__":
    unittest.main()
