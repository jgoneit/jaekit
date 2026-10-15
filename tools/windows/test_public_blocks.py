"""Public Windows block harness regressions that run on any host.

They read the real guides and synthetic archives; no PowerShell, network
beyond 127.0.0.1, installation or policy change.
"""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
import unittest.mock
from urllib.request import urlopen
from urllib.error import HTTPError
import zipfile

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("public_blocks", HERE / "public_blocks.py")
blocks = importlib.util.module_from_spec(spec)
spec.loader.exec_module(blocks)


def release_zip():
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as archive:
        for name in ("ha.exe", "package.json", "tools/windows/common.ps1",
                     "tools/windows/install.ps1", "tools/windows/uninstall.ps1"):
            archive.writestr(name, b"real " + name.encode())
    return out.getvalue()


class GuideBlocks(unittest.TestCase):
    def test_both_guides_carry_identical_install_and_remove_blocks(self):
        found = blocks.public_blocks()
        self.assertEqual(set(found), {"install", "remove"})
        texts = [(blocks.ROOT / guide).read_text(encoding="utf-8") for guide in blocks.GUIDES]
        for kind, label in blocks.LABELS.items():
            self.assertEqual(blocks.block(texts[0], label), blocks.block(texts[1], label))

    def test_blocks_keep_the_trust_order_and_policy_boundary(self):
        for kind, code in blocks.public_blocks().items():
            with self.subTest(kind=kind):
                script = "install.ps1" if kind == "install" else "uninstall.ps1"
                order = ["Invoke-WebRequest -UseBasicParsing", "$listed.Count -ne 1",
                         "Get-FileHash -LiteralPath $archive -Algorithm SHA256",
                         "ExtractToFile", "& $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $work '" + script + "')",
                         "$code = $LASTEXITCODE", "if ($code -ne 0)"]
                positions = [code.index(item) for item in order]
                self.assertEqual(positions, sorted(positions))
                self.assertIn("$shell = (Get-Process -Id $PID).Path", code)
                self.assertIn("'common.ps1', '" + script + "'", code)
                self.assertNotIn("Set-ExecutionPolicy", code)
                self.assertNotIn("Invoke-Expression", code)
                self.assertNotIn("iex ", code)
                self.assertNotIn('"', code)
                self.assertEqual(code.count(blocks.SOURCE), 1)
                self.assertEqual(len(blocks.VERSION_LINE.findall(code)), 1)
                self.assertIn("[IO.Directory]::Delete($work, $true)", code)
        install = blocks.public_blocks()["install"]
        self.assertIn("-AddToPath", install)
        self.assertIn("$env:Path += $bin", install)
        self.assertIn("Write-Warning ('Another ha comes first", install)
        self.assertNotIn("--version", install)
        remove = blocks.public_blocks()["remove"]
        self.assertNotIn("$env:Path +=", remove)

    def test_binding_changes_only_the_source_and_version(self):
        code = blocks.public_blocks()["install"]
        bound = blocks.bind(code, "http://127.0.0.1:1234/case", "0.0.0")
        changed = [(a, b) for a, b in zip(code.splitlines(), bound.splitlines()) if a != b]
        self.assertEqual(len(code.splitlines()), len(bound.splitlines()))
        self.assertEqual(len(changed), 2)
        self.assertIn("$version = '0.0.0'", bound)
        self.assertIn("$source = 'http://127.0.0.1:1234/case/v' + $version", bound)
        with self.assertRaises(blocks.Failure):
            blocks.bind(code.replace(blocks.SOURCE, "'elsewhere'"), "http://x", "0.0.0")
        with self.assertRaises(blocks.Failure):
            blocks.bind(code + "\n$version = '1.2.3'", "http://x", "0.0.0")

    def test_a_divergent_translation_is_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for guide in blocks.GUIDES:
                (root / guide).parent.mkdir(parents=True, exist_ok=True)
                (root / guide).write_text((blocks.ROOT / guide).read_text(encoding="utf-8"), encoding="utf-8")
            english = root / blocks.GUIDES[1]
            english.write_text(english.read_text(encoding="utf-8").replace("is ready in this window", "is ready"),
                               encoding="utf-8")
            with self.assertRaisesRegex(blocks.Failure, "differ"):
                blocks.public_blocks(root)

    def test_wrapper_runs_the_block_unchanged_inside_an_observer(self):
        code = blocks.public_blocks()["remove"]
        text = blocks.wrapper(code, Path("C:/work/it's/observer-probe.ps1"))
        self.assertIn("\n" + code + "\n", text)
        self.assertTrue(text.rstrip().endswith("ConvertTo-Json -Compress -Depth 4))"))
        # PowerShell 7 cannot load the Security module under Restricted, so the
        # window itself never asks for its policy; it tries a script file.
        self.assertNotIn("Get-ExecutionPolicy", text)
        self.assertEqual(text.count("& 'C:/work/it''s/observer-probe.ps1'"), 2)
        before, after = text.index("jaekitScriptsBefore ="), text.index("jaekitScriptsAfter =")
        self.assertLess(before, text.index(code))
        self.assertLess(text.index(code), after)

    def test_environment_drops_inherited_policy_and_module_path_in_any_case(self):
        inherited = {"PSMODULEPATH": "C:/ps7/Modules", "PSExecutionPolicyPreference": "Bypass", "PATH": "C:/bin"}
        with unittest.mock.patch.dict(blocks.os.environ, inherited, clear=True):
            self.assertEqual(blocks.base_env(), {"PATH": "C:/bin"})


class Judgement(unittest.TestCase):
    def case(self, **observed):
        restricted = {"MachinePolicy": "Undefined", "CurrentUser": "Restricted", "LocalMachine": "RemoteSigned"}
        base = {"shell": "C:/pwsh.exe", "process_policy": "", "scripts_before": blocks.SCRIPTS_BLOCKED,
                "scripts_after": blocks.SCRIPTS_BLOCKED, "status": "error", "path_before": "a", "path_after": "a",
                "policies_before": dict(restricted), "policies_after": dict(restricted), "output": "", "errors": ""}
        base.update(observed)
        return {"expect": {"status": "error", "child": "none"}, "observed": base, "marker": None, "temporary_left": []}

    def test_policy_observations_decide_the_case(self):
        self.assertEqual(blocks.judge(self.case(), "C:/pwsh.exe"), [])
        changed = {"MachinePolicy": "Undefined", "CurrentUser": "Restricted", "LocalMachine": "Bypass"}
        for observed in ({"process_policy": "Bypass"}, {"scripts_before": "ran"}, {"scripts_after": "ran"},
                         {"policies_after": changed}, {"policies_before": dict(changed, CurrentUser="RemoteSigned")}):
            with self.subTest(observed=observed):
                self.assertTrue(blocks.judge(self.case(**observed), "C:/pwsh.exe"))


class Archives(unittest.TestCase):
    def test_probe_archive_replaces_only_the_lifecycle_scripts(self):
        real = release_zip()
        probe = blocks.probe_archive(real)
        with zipfile.ZipFile(io.BytesIO(real)) as before, zipfile.ZipFile(io.BytesIO(probe)) as after:
            self.assertEqual(before.namelist(), after.namelist())
            for name in before.namelist():
                same = before.read(name) == after.read(name)
                self.assertEqual(same, name not in {"tools/windows/install.ps1", "tools/windows/uninstall.ps1"}, name)
            self.assertIn(b"script = 'install'", after.read("tools/windows/install.ps1"))
            self.assertIn(b"script = 'uninstall'", after.read("tools/windows/uninstall.ps1"))

    def test_checksum_variants(self):
        data, name = b"zip", "ha_0.0.0_windows_amd64.zip"
        digest = hashlib.sha256(data).hexdigest()
        lines = {variant: blocks.checksum_lines(name, data, variant).decode().splitlines()
                 for variant in ("valid", "missing", "duplicate", "mismatch")}
        self.assertEqual([line for line in lines["valid"] if line.endswith(name)], [digest + "  " + name])
        self.assertFalse([line for line in lines["missing"] if line.endswith(name)])
        self.assertEqual(len([line for line in lines["duplicate"] if line.endswith(name)]), 2)
        self.assertNotIn(digest, "".join(lines["mismatch"]))

    def test_release_directory_must_list_its_archive_once(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            data = release_zip()
            name = "ha_1.2.3_windows_amd64.zip"
            (root / name).write_bytes(data)
            (root / "checksums.txt").write_bytes(blocks.checksum_lines(name, data))
            self.assertEqual(blocks.release_files(root)[3], "1.2.3")
            (root / "checksums.txt").write_bytes(blocks.checksum_lines(name, data, "duplicate"))
            with self.assertRaises(blocks.Failure):
                blocks.release_files(root)

    def test_local_server_serves_only_published_files(self):
        with blocks.Server() as server:
            base = server.publish("case", "1.2.3", {"checksums.txt": b"listed\n"})
            with urlopen(base + "/v1.2.3/checksums.txt", timeout=10) as response:
                self.assertEqual(response.read(), b"listed\n")
            with self.assertRaises(HTTPError):
                urlopen(base + "/v1.2.3/other.zip", timeout=10)


class Checked(unittest.TestCase):
    def reports(self, directory, **changes):
        paths = []
        for shell in ("powershell", "pwsh"):
            report = {"schema": blocks.SCHEMA, "mode": "ci", "shell": shell, "passed": True,
                      "source_commit": "a" * 40, "expected_sha256": "b" * 64, "archive": {"sha256": "b" * 64}}
            report.update(changes.get(shell, {}))
            path = Path(directory) / f"{shell}.json"
            path.write_text(json.dumps(report))
            paths.append(str(path))
        return paths

    def test_checked_digest_requires_two_passing_reports_for_the_same_archive(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "output"
            args = ["checked", "--expected-sha256", "b" * 64, "--github-output", str(output)]
            for path in self.reports(directory):
                args += ["--report", path]
            self.assertEqual(blocks.main(args), 0)
            self.assertEqual(output.read_text(), "sha256=" + "b" * 64 + "\n")
            for changes in ({"pwsh": {"passed": False}}, {"pwsh": {"archive": {"sha256": "c" * 64}}},
                            {"pwsh": {"source_commit": "d" * 40}}):
                args = ["checked", "--expected-sha256", "b" * 64]
                for path in self.reports(directory, **changes):
                    args += ["--report", path]
                with self.subTest(changes=changes):
                    self.assertEqual(blocks.main(args), 1)


if __name__ == "__main__":
    unittest.main()
