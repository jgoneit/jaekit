#!/usr/bin/env python3
"""Opt-in, read-only release acceptance checks; see README.md for local evidence."""
from __future__ import annotations

import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parents[2]
REPO = "jgoneit/jaekit"
TAG = "v0.1.3"
VERSION = TAG[1:]
PLATFORMS = ("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64")
HEX = re.compile(r"[0-9a-f]{64}\Z")
COMMIT = re.compile(r"[0-9a-f]{40}\Z")


class Failure(Exception):
    """An observed requirement is not satisfied."""


class Unavailable(Exception):
    """Evidence could not be obtained; this is not a successful observation."""


def require(condition, message):
    if not condition:
        raise Failure(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key in evidence")
        result[key] = value
    return result


def decode(data):
    try:
        return json.loads(data, object_pairs_hook=unique_object)
    except (ValueError, UnicodeDecodeError):
        raise Failure("malformed JSON evidence") from None


def command(argv, cwd=ROOT, timeout=180):
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GH_PROMPT_DISABLED="1",
               GOTOOLCHAIN="local")
    env.pop("GH_DEBUG", None)
    try:
        return subprocess.run(argv, cwd=cwd, env=env, capture_output=True,
                              timeout=timeout)
    except (OSError, subprocess.TimeoutExpired):
        raise Unavailable(f"could not run read-only check: {Path(argv[0]).name}") from None


def checked(argv, cwd=ROOT, timeout=180):
    result = command(argv, cwd, timeout)
    require(result.returncode == 0, f"local check failed: {Path(argv[0]).name}")
    return result.stdout


def api(path, *, absent=None):
    result = command(["gh", "api", "--hostname", "github.com", "--method", "GET",
                      "-H", "Accept: application/vnd.github+json", path], timeout=45)
    if result.returncode:
        if absent and b"HTTP 404" in result.stderr:
            raise Failure(absent)
        raise Unavailable("GitHub GET failed; release state remains unverified")
    return decode(result.stdout)


def release_snapshot(release, commit):
    assets = release.get("assets")
    require(isinstance(assets, list) and assets, "release has no assets")
    require(all(isinstance(a, dict) for a in assets), "invalid release asset metadata")
    return {"tag": release.get("tag_name"), "commit": commit,
            "release_id": release.get("id"),
            "assets": sorted(({key: a.get(key) for key in ("id", "name", "size", "digest")}
                              for a in assets), key=lambda a: a["name"])}


def native_platform():
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64",
            "AMD64": "amd64"}.get(platform.machine())
    require(system and arch, "native release smoke platform is unsupported")
    return f"{system}_{arch}"


def archive_files(data, target):
    """Inspect in memory; never extract untrusted archive paths."""
    prefix = f"ha_{VERSION}_{target}/"
    files = {}
    try:
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
            for member in archive:
                name = member.name
                path = PurePosixPath(name)
                require(not path.is_absolute() and ".." not in path.parts,
                        "unsafe archive entry")
                require(name == prefix[:-1] or name.startswith(prefix),
                        "archive root disagrees with platform name")
                require(member.isdir() or member.isfile(), "archive contains a non-regular entry")
                if member.isfile():
                    relative = name[len(prefix):]
                    require(relative and relative not in files, "duplicate archive entry")
                    require(member.size <= 64 * 1024 * 1024, "oversized archive entry")
                    files[relative] = archive.extractfile(member).read()
    except (tarfile.TarError, OSError):
        raise Failure("invalid release archive") from None
    for name in ("ha", "LICENSE", "README.md", "README.en.md", "guides/INSTALL.md",
                 "guides/INSTALL.en.md", "contracts/run-record.md"):
        require(name in files, f"release archive omits {name}")
    return files


class Checker:
    def __init__(self):
        common = checked(["git", "rev-parse", "--git-common-dir"]).decode().strip()
        self.private = (ROOT / common).resolve() / "ha" / "release-0-1-3"
        self._evidence = None
        self._release = None
        self._downloads = {}
        self._source = {}

    def evidence(self):
        if self._evidence is None:
            try:
                self._evidence = decode((self.private / "evidence.json").read_bytes())
            except OSError:
                raise Unavailable("private release evidence index is absent") from None
            require(self._evidence.get("schema") == "jaekit-release-evidence/v1",
                    "unsupported release evidence schema")
            require(self._evidence.get("release", {}).get("tag") == TAG,
                    "evidence names a different release")
        return self._evidence

    def artifact(self, value):
        require(isinstance(value, dict) and isinstance(value.get("path"), str)
                and isinstance(value.get("sha256"), str) and HEX.fullmatch(value["sha256"]),
                "artifact must identify a file and SHA256, not a success assertion")
        path = Path(value["path"])
        if not path.is_absolute():
            path = self.private / path
        try:
            data = path.read_bytes()
        except OSError:
            raise Unavailable("indexed observation file is absent or unreadable") from None
        require(digest(data) == value["sha256"], "indexed observation changed after capture")
        return path.resolve(), data

    def capture(self, value):
        require(isinstance(value, dict), "missing native command capture")
        argv = value.get("argv")
        require(isinstance(argv, list) and argv and all(isinstance(x, str) and x for x in argv),
                "capture must contain the actual argv")
        require(type(value.get("exit_code")) is int and value["exit_code"] == 0,
                "captured command did not succeed")
        _, out = self.artifact(value.get("stdout"))
        self.artifact(value.get("stderr"))
        return argv, out

    def release(self):
        if self._release is None:
            self._release = api(f"repos/{REPO}/releases/tags/{TAG}",
                                absent="public v0.1.3 release does not exist")
            require(self._release.get("tag_name") == TAG
                    and self._release.get("draft") is False
                    and self._release.get("prerelease") is False,
                    "v0.1.3 is not a public final release")
        return self._release

    def assets(self):
        values = self.release().get("assets", [])
        names = [a.get("name") for a in values]
        expected = {f"ha_{VERSION}_{p}.tar.gz" for p in PLATFORMS} | {"checksums.txt"}
        require(set(names) == expected and len(names) == len(expected),
                "public release must contain exactly four archives and checksums.txt")
        return {a["name"]: a for a in values}

    def download(self, name):
        if name not in self._downloads:
            item = self.assets()[name]
            url = f"https://github.com/{REPO}/releases/download/{TAG}/{name}"
            require(item.get("browser_download_url") == url, "asset URL disagrees with tag")
            try:
                with urlopen(url, timeout=45) as response:
                    data = response.read(64 * 1024 * 1024 + 1)
            except OSError:
                raise Unavailable("public release asset download failed") from None
            require(len(data) <= 64 * 1024 * 1024 and len(data) == item.get("size"),
                    "downloaded asset size differs from public metadata")
            require(item.get("digest") == "sha256:" + digest(data),
                    "downloaded asset digest differs from public metadata")
            self._downloads[name] = data
        return self._downloads[name]

    def archives(self):
        checksums = self.download("checksums.txt").decode("ascii")
        rows = {}
        for line in checksums.splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})\s+\*?(ha_[A-Za-z0-9_.]+\.tar\.gz)", line)
            require(match and match[2] not in rows, "invalid or duplicate checksum entry")
            rows[match[2]] = match[1]
        require(set(rows) == set(self.assets()) - {"checksums.txt"}, "checksum set differs from assets")
        result = {}
        for target in PLATFORMS:
            name = f"ha_{VERSION}_{target}.tar.gz"
            data = self.download(name)
            require(digest(data) == rows[name], "archive checksum does not match checksums.txt")
            result[target] = archive_files(data, target)
        return result

    def source(self, path):
        if path not in self._source:
            value = api(f"repos/{REPO}/contents/{path}?ref={TAG}")
            require(value.get("type") == "file" and value.get("encoding") == "base64",
                    "tagged source file is unavailable")
            self._source[path] = base64.b64decode(value["content"], validate=False)
        return self._source[path]

    def installation(self, name):
        value = self.evidence().get("installations", {}).get(name)
        require(isinstance(value, dict), f"missing {name} installation observation")
        binary, data = self.artifact(value.get("binary"))
        target = native_platform()
        files = archive_files(self.download(f"ha_{VERSION}_{target}.tar.gz"), target)
        require(data == files["ha"], "selected installed Core differs from the public native asset")
        install, _ = self.capture(value.get("install"))
        if name.startswith("brew_"):
            require(Path(install[0]).name == "brew" and "jaekit" in " ".join(install)
                    and ("install" if name == "brew_fresh" else "upgrade") in install,
                    "Homebrew observation is not the requested install or upgrade")
        for key, suffix in (("version", ["--version"]),
                            ("capabilities", ["capabilities", "--format", "json"])):
            argv, out = self.capture(value.get(key))
            require(Path(argv[0]).resolve() == binary and argv[1:] == suffix,
                    "installation capture queried a different executable or command")
            current = checked([str(binary), *suffix])
            require(current == out, "installed Core query changed since capture")
            if key == "version":
                require(out.strip() == b"ha 0.1.3", "installed Core has the wrong release version")
            else:
                info = decode(out)
                require(info.get("schema") == "ha-capabilities/v1"
                        and info.get("ha_version") == VERSION
                        and info.get("default_rules") == "run-rules/3",
                        "installed Core does not identify the released /3 capability contract")
        if name == "brew_upgrade":
            _, before = self.capture(value.get("before_version"))
            require(before.strip() == b"ha 0.1.2", "upgrade does not establish the old installed version")
        return binary

    def host(self, name):
        value = self.evidence().get("hosts", {}).get(name)
        require(isinstance(value, dict), f"missing {name} host observation")
        argv, out = self.capture(value.get("version"))
        require(Path(argv[0]).name == name and out.strip(), "host version capture has no native output")
        installs = value.get("install")
        require(isinstance(installs, list) and len(installs) >= 2,
                "both plugin install captures are required")
        installed = set()
        for install in installs:
            argv, _ = self.capture(install)
            require(Path(argv[0]).name == name and "plugin" in argv
                    and any(action in argv for action in ("add", "install")),
                    "host capture is not a native plugin installation")
            installed.update(plugin for plugin in ("spec", "seal") if f"{plugin}@jaekit" in argv)
        require(installed == {"spec", "seal"}, "host did not install both Jaekit plugins")
        argv, _ = self.capture(value.get("update"))
        require(Path(argv[0]).name == name and "plugin" in argv
                and "marketplace" in argv and "add" in argv
                and any(arg in (f"{REPO}@{TAG}", f"{REPO}#{TAG}") for arg in argv),
                "host marketplace observation is not pinned to the new tag")
        _, inventory = self.artifact(value.get("inventory"))
        registered = decode(inventory)
        entries = registered.get("installed", []) if name == "codex" and isinstance(registered, dict) else registered
        require(isinstance(entries, list) and entries, "native host inventory is empty")
        manifest_dir = ".codex-plugin" if name == "codex" else ".claude-plugin"
        for plugin in ("spec", "seal"):
            path, data = self.artifact(value.get("plugins", {}).get(plugin))
            require(data == self.source(f"plugins/{plugin}/skills/{plugin}/SKILL.md"),
                    "installed Skill content differs from the public tag")
            plugin_root = path.parent.parent.parent
            manifest = plugin_root / manifest_dir / "plugin.json"
            published = self.source(f"plugins/{plugin}/{manifest_dir}/plugin.json")
            require(manifest.read_bytes() == published,
                    "installed host manifest differs from the public tag")
            matches = [row for row in entries if isinstance(row, dict)
                       and row.get("pluginId" if name == "codex" else "id") == f"{plugin}@jaekit"]
            require(len(matches) == 1 and matches[0].get("enabled") is True
                    and matches[0].get("version") == decode(published).get("version"),
                    "native inventory does not enable the tagged component version")
            if name == "claude":
                require(Path(matches[0].get("installPath", "")).resolve() == plugin_root,
                        "Claude inventory identifies a different plugin installation")
            else:
                registered_path = Path(matches[0].get("source", {}).get("path", ""))
                require((registered_path / "skills" / plugin / "SKILL.md").read_bytes() == data,
                        "Codex registered marketplace source differs from the loaded Skill")
            if plugin == "seal":
                for relative in ("references/ha.md", "scripts/check_core.py"):
                    require((path.parent / relative).read_bytes()
                            == self.source(f"plugins/seal/skills/seal/{relative}"),
                            "installed Seal support file differs from the public tag")
        return value

    def check_1(self):
        current = self.release()
        commit = api(f"repos/{REPO}/commits/{TAG}")
        expected = self.evidence()["release"].get("source_commit")
        require(isinstance(expected, str) and COMMIT.fullmatch(expected)
                and expected == commit.get("sha"), "release tag differs from the identified source commit")
        tree = checked(["git", "rev-parse", "HEAD^{tree}"]).decode().strip()
        require(tree == commit.get("commit", {}).get("tree", {}).get("sha"),
                "checked source tree differs from the released source")
        _, previous = self.artifact(self.evidence()["release"].get("previous"))
        old = api(f"repos/{REPO}/releases/tags/v0.1.2")
        old_commit = api(f"repos/{REPO}/commits/v0.1.2").get("sha")
        require(decode(previous) == release_snapshot(old, old_commit),
                "previous public release identity or assets changed")
        require(current.get("body", "").strip(), "release has no public notes")

    def check_2(self):
        self.release()
        self.archives()
        self.installation("direct")

    def check_3(self):
        self.release()
        value = api("repos/jgoneit/homebrew-tap/contents/Formula/jaekit.rb?ref=main")
        formula = base64.b64decode(value["content"]).decode()
        validate_formula(formula, self.assets())
        self.installation("brew_fresh")
        self.installation("brew_upgrade")
        target = native_platform()
        files = archive_files(self.download(f"ha_{VERSION}_{target}.tar.gz"), target)
        validate_selected_core(files["ha"])

    def check_4(self):
        self.release()
        for name in ("codex", "claude"):
            value = self.host(name)
            self.trace(name, value, "spec")
            self.trace(name, value, "seal")

    def check_5(self):
        self.release()
        archives = self.archives()
        for installation in ("direct", "brew_fresh", "brew_upgrade"):
            value = self.evidence().get("installations", {}).get(installation, {})
            root = value.get("package_root")
            require(isinstance(root, str) and Path(root).is_absolute(),
                    "installation support-file location is missing")
            validate_support_files(Path(root), archives[native_platform()])
        validator = ROOT / "tools/release/check_archives.py"
        require(validator.is_file(), "release archive validator is missing")
        with tempfile.TemporaryDirectory(prefix="jaekit-release-read-") as directory:
            for name in self.assets():
                (Path(directory) / name).write_bytes(self.download(name))
            checked([sys.executable, str(validator), "--version", VERSION,
                     "--dist", directory, "--run-native"], timeout=300)

    def trace(self, name, value, phase):
        capture = value.get(phase)
        argv, raw = self.capture(capture)
        require(Path(argv[0]).name == name, "flow capture is not a native host invocation")
        if name == "codex":
            require("exec" in argv and "--json" in argv, "Codex observation is not native JSON output")
        else:
            require("-p" in argv and "--output-format" in argv and "stream-json" in argv,
                    "Claude observation is not native stream JSON output")
        events = json_lines(raw)
        require(events, "native host trace has no events")
        if name == "codex":
            terminal = [event for event in events if event.get("type") in ("turn.completed", "turn.failed")]
            require(terminal and terminal[-1].get("type") == "turn.completed",
                    "native Codex turn did not complete")
        else:
            terminal = [event for event in events if event.get("type") == "result"]
            require(terminal and terminal[-1].get("subtype") == "success"
                    and terminal[-1].get("is_error") is False,
                    "native Claude turn did not complete")
        session = value.get("session_id")
        model = value.get("model")
        require(isinstance(session, str) and len(session) > 8 and isinstance(model, str) and model,
                "actual host model/session identity is missing")
        metadata = list(events)
        phase_events = list(events)
        rollout = capture.get("session_trace") or value.get("session_trace")
        if rollout:
            _, additional = self.artifact(rollout)
            session_events = json_lines(additional)
            require(session in native_identity(session_events)[0],
                    "rollout metadata belongs to a different native session")
            metadata.extend(session_events)
            # A final whole-session rollout contains both requests. Do not
            # attribute the later implementation commands to the Spec turn.
            # A phase-specific snapshot, or the final Seal turn, can supply
            # native function-call details absent from exec's summary events.
            if capture.get("session_trace") or phase == "seal":
                phase_events.extend(session_events)
        sessions, models = native_identity(events), native_identity(metadata)
        require(session in sessions[0] and model in models[1],
                "model or session is not corroborated by native trace metadata")
        skill = "spec:spec" if phase == "spec" else "seal:seal"
        require(skill in " ".join(argv) or skill.encode() in raw,
                "host observation does not identify the requested Skill")
        installed_path, _ = self.artifact(value.get("plugins", {}).get(phase))
        metadata_text = json.dumps(metadata, ensure_ascii=False)
        require(str(installed_path) in metadata_text
                or (name == "claude" and str(installed_path.parent.parent.parent) in metadata_text),
                "native host trace does not identify the actual loaded Skill path")
        if phase == "seal":
            require(("resume" if name == "codex" else "--resume") in argv,
                    "implementation is not a separate resumed host request")
        commands = tool_commands(phase_events)
        require(native_tools(phase_events), "native host trace has no observed tool execution")
        if phase == "spec":
            require(not any(re.search(r"\bha[\"']?\s+(?:start|check|done|note|budget)\b", c)
                            for c in commands), "Spec observation performed execution work")
        else:
            text = "\n".join(commands)
            for operation in ("start", "check", "done"):
                require(re.search(r"\bha[\"']?\s+" + operation + r"\b", text),
                        f"native Seal trace does not observe ha {operation}")
            require("--baseline" in text, "native Seal trace does not observe a baseline attempt")
        return commands

    def check_6(self):
        self.release()
        binary = self.installation("brew_upgrade")
        for name in ("codex", "claude"):
            value = self.host(name)
            self.trace(name, value, "spec")
            self.trace(name, value, "seal")
            project = Path(value.get("project", ""))
            goal = value.get("goal")
            require(project.is_absolute() and isinstance(goal, str) and goal
                    and not PurePosixPath(goal).is_absolute() and ".." not in PurePosixPath(goal).parts,
                    "synthetic project and goal must be explicit")
            require(project.resolve() != ROOT.resolve(), "host flow reused the product development checkout")
            spec_commit, final_commit = value.get("spec_commit"), value.get("final_commit")
            require(all(isinstance(c, str) and COMMIT.fullmatch(c) for c in (spec_commit, final_commit)),
                    "synthetic source snapshots are missing")
            require(checked(["git", "rev-parse", "HEAD"], project).decode().strip() == final_commit,
                    "synthetic final checkout differs from the observed source")
            checked(["git", "merge-base", "--is-ancestor", spec_commit, final_commit], project)
            files = checked(["git", "ls-tree", "-r", "--name-only", spec_commit, "--", goal], project).decode().splitlines()
            require(f"{goal}/SPEC.md" in files
                    and not any(Path(p).name in ("PLAN.md", "REVIEW.md", "PROGRESS.md", "runs.jsonl") for p in files),
                    "Spec snapshot includes execution artifacts or omits the goal")
            spec = checked(["git", "show", f"{spec_commit}:{goal}/SPEC.md"], project)
            require(b"Criteria-Format: nested/1" in spec, "actual Spec output omits the new format")
            changed = checked(["git", "diff-tree", "--no-commit-id", "--name-only", "-r", spec_commit], project).decode().splitlines()
            require(changed and all(p.startswith(goal + "/") for p in changed),
                    "Spec snapshot includes implementation changes")
            records = (project / goal / "runs.jsonl").read_bytes()
            gitdir = checked(["git", "rev-parse", "--absolute-git-dir"], project).decode().strip()
            done_seq = validate_records(records, project, Path(gitdir), value)
            report = decode(checked([str(binary), "status", goal, "--format", "json"], project))
            validate_completion(report, done_seq)

    def check_7(self):
        checked(["go", "test", "-mod=readonly", "./cmd/ha", "./plugins", "-run",
                 "^(TestCompatibilityAC7_|TestCompatibilityAC8_|TestCompatibilityAC12_)", "-count=1"], timeout=300)

    def check_8(self):
        checked(["go", "test", "-mod=readonly", "./cmd/ha", "./internal/status", "./internal/record",
                 "./internal/goaldocs", "./plugins", "-run",
                 "^(TestCompatibilityAC9_|TestBaselineBudget|TestBudget|TestBaselineSafetyAC5_|"
                 "TestNestedCriteria|TestRelease011UninstallGuide|TestRelease011UpdateGuide)", "-count=1"], timeout=300)

    def check_9(self):
        market = decode((ROOT / ".claude-plugin/marketplace.json").read_bytes())
        require(market.get("metadata", {}).get("version") == VERSION,
                "source marketplace still identifies the previous release")
        checked(["go", "test", "-mod=readonly", "./plugins", "-run",
                 "^(TestCompatibilityAC10_|TestCompatibilityAC11_|TestRelease|TestReadme)", "-count=1"], timeout=300)
        note = self.release().get("body", "")
        require("0.1.3" in note and ("Spec" in note and "Seal" in note),
                "public release notes do not identify the product combination")

    def check_10(self):
        for directory in ("tools/release", "tools/release-checks"):
            result = command([sys.executable, "-m", "unittest", "discover", "-s", directory,
                              "-p", "test_*.py"], timeout=300)
            require(result.returncode == 0 and re.search(rb"Ran [1-9][0-9]* tests?", result.stderr),
                    "release failure-protection regressions failed or did not run")

    def check_11(self):
        checked([sys.executable, "tools/check_public_tree.py"])
        checked([sys.executable, "tools/check_public_docs.py"])
        # Before publication the maintained boundary is covered by source and
        # packaging regressions. Once published, additionally inspect real assets.
        result = command(["gh", "api", "--method", "GET", f"repos/{REPO}/releases/tags/{TAG}"])
        if result.returncode:
            require(b"HTTP 404" in result.stderr, "public release boundary could not be queried")
            return
        self._release = decode(result.stdout)
        for files in self.archives().values():
            for path, data in files.items():
                parts = PurePosixPath(path).parts
                require(not {"docs", ".git", ".private", ".claude"}.intersection(parts)
                        and Path(path).name != "runs.jsonl", "private execution material in release archive")
                if path != "ha":
                    private_text(data)
        private_text(self.release().get("body", "").encode())
        formula = api("repos/jgoneit/homebrew-tap/contents/Formula/jaekit.rb?ref=main")
        private_text(base64.b64decode(formula["content"]))


def json_lines(data):
    result = []
    for line in data.splitlines():
        if line.strip():
            value = decode(line)
            require(isinstance(value, dict), "native trace event is not an object")
            result.append(value)
    return result


def native_identity(events):
    sessions, models = set(), set()
    for event in events:
        if event.get("type") == "thread.started" and isinstance(event.get("thread_id"), str):
            sessions.add(event["thread_id"])
        if isinstance(event.get("session_id"), str):
            sessions.add(event["session_id"])
        payload = event.get("payload", {})
        if event.get("type") == "session_meta" and isinstance(payload.get("id"), str):
            sessions.add(payload["id"])
        if event.get("type") == "turn_context" and isinstance(payload.get("model"), str):
            models.add(payload["model"])
        if event.get("type") == "system" and event.get("subtype") == "init" and isinstance(event.get("model"), str):
            models.add(event["model"])
    return sessions, models


def validate_formula(formula, assets):
    require(re.search(r'^\s*version\s+"0\.1\.3"\s*$', formula, re.M),
            "public formula has the old version")
    for system, selector in (("darwin", "macos"), ("linux", "linux")):
        section = re.search(r"\bon_" + selector + r"\s+do\b(.*?)(?=\bon_(?:macos|linux)\b|\bdef install\b|\Z)",
                            formula, re.S)
        require(section, "formula omits an operating system")
        for arch, architecture in (("arm64", "arm"), ("amd64", "intel")):
            block = re.search(r"\bon_" + architecture + r"\s+do\b(.*?)\bend\b", section[1], re.S)
            require(block, "formula omits a processor architecture")
            name = f"ha_{VERSION}_{system}_{arch}.tar.gz"
            urls = re.findall(r'\burl\s+"([^"\n]+)"', block[1])
            hashes = re.findall(r'\bsha256\s+"([^"\n]+)"', block[1])
            require(urls == [f"https://github.com/{REPO}/releases/download/{TAG}/{name}"]
                    and hashes == [assets[name]["digest"].removeprefix("sha256:")],
                    "formula platform URL/checksum pair differs from the public release")


def validate_support_files(root, files):
    require("tools/check-result-reference.py" in files
            and "examples/check-result/reference.json" in files,
            "published archive omits the reference producer or example")
    for relative, data in files.items():
        if relative != "ha":
            path = root / relative
            require(path.is_file() and path.read_bytes() == data,
                    "installed support files differ from the public release archive")


def validate_selected_core(public_binary):
    selected = shutil.which("ha")
    require(selected, "the current PATH does not select a Core executable")
    binary = Path(selected).resolve()
    require(binary.read_bytes() == public_binary, "current PATH selects a different Core than the public native asset")
    require(checked([str(binary), "--version"]).strip() == b"ha 0.1.3",
            "current PATH selects the wrong Core version")
    info = decode(checked([str(binary), "capabilities", "--format", "json"]))
    require(info.get("schema") == "ha-capabilities/v1" and info.get("ha_version") == VERSION
            and info.get("default_rules") == "run-rules/3",
            "current PATH Core does not expose the release capabilities")


def tool_commands(events):
    commands = []
    for event in events:
        # Codex exec emits completed command_execution items.
        item = event.get("item", {})
        if event.get("type") == "item.completed" and item.get("type") == "command_execution":
            if isinstance(item.get("command"), str):
                commands.append(item["command"])
        if event.get("type") == "item.completed" and item.get("type") == "mcp_tool_call":
            commands.extend(call_commands(item.get("tool", ""), item.get("arguments", {})))
        payload = event.get("payload", {})
        if event.get("type") == "response_item" and payload.get("type") in ("function_call", "custom_tool_call"):
            commands.extend(call_commands(payload.get("name", ""),
                                          payload.get("arguments", payload.get("input", {}))))
        # Claude native stream emits assistant tool_use blocks.
        message = event.get("message", {})
        if isinstance(message, dict):
            for block in message.get("content", []) if isinstance(message.get("content"), list) else []:
                if isinstance(block, dict) and block.get("type") == "tool_use":
                    command_text = block.get("input", {}).get("command")
                    if isinstance(command_text, str):
                        commands.append(command_text)
    return commands


def call_commands(name, arguments):
    if isinstance(arguments, str):
        try:
            parsed = json.loads(arguments)
        except ValueError:
            parsed = None
        if isinstance(parsed, dict):
            arguments = parsed
        elif name in ("functions.exec", "exec") and "exec_command(" in arguments:
            # The native custom-tool payload is JavaScript. Treat the observed
            # invocation text as evidence, never evaluate or execute it here.
            return [arguments]
    if isinstance(arguments, dict):
        if name.endswith("exec_command") or name in ("Bash", "bash", "shell_command"):
            text = arguments.get("cmd", arguments.get("command"))
            return [text] if isinstance(text, str) else []
        if name in ("functions.exec", "exec"):
            text = arguments.get("code", arguments.get("input", ""))
            if isinstance(text, str) and "exec_command(" in text:
                return [text]
    return []


def native_tools(events):
    for event in events:
        if event.get("type") == "item.completed" and event.get("item", {}).get("type") in (
                "command_execution", "file_change", "mcp_tool_call"):
            return True
        if event.get("type") == "response_item" and event.get("payload", {}).get("type") in (
                "function_call", "custom_tool_call"):
            return True
        content = event.get("message", {}).get("content", [])
        if isinstance(content, list) and any(isinstance(b, dict) and b.get("type") == "tool_use" for b in content):
            return True
    return False


def validate_records(raw, project, gitdir, host):
    lines = raw.splitlines()
    require(lines and raw.endswith(b"\n"), "host goal has no complete record chain")
    previous = None
    records = []
    for seq, line in enumerate(lines, 1):
        record = decode(line)
        require(record.get("schema") == "run/v1" and record.get("seq") == seq
                and record.get("prev") == previous and record.get("ha_version") == VERSION,
                "host record chain or recorder version is invalid")
        previous = digest(line)
        records.append(record)
        if record.get("kind") in ("check", "baseline"):
            path = record.get("output_path", "")
            if path == "" and record.get("output_digest") == "":
                require(record.get("result") == "error"
                        and record.get("evidence", {}).get("result") == "error"
                        and record.get("evidence", {}).get("reason") == "process_output",
                        "only a recorded output-storage failure may omit a raw log")
                continue
            require(path.startswith(".git/ha/") and ".." not in PurePosixPath(path).parts,
                    "host raw output path is outside recorder storage")
            try:
                output = (gitdir / path.removeprefix(".git/")).read_bytes()
            except OSError:
                raise Unavailable("host raw output is missing") from None
            require(digest(output) == record.get("output_digest"), "host raw output digest mismatch")
    start = records[0]
    require(start.get("kind") == "start" and start.get("rules") == "run-rules/3",
            "host goal did not actually start with /3")
    require(start.get("host", {}).get("model") == host.get("model"),
            "recorded model differs from the native host observation")
    baseline = [r for r in records if r.get("kind") == "baseline" and r.get("result") == "fail_as_expected"]
    checks = [r for r in records if r.get("kind") == "check" and r.get("result") == "pass"]
    require(any(b.get("target") == c.get("target") and b["seq"] < c["seq"]
                and b.get("evidence", {}).get("result") == "fail_as_expected"
                and c.get("evidence", {}).get("result") == "pass" for b in baseline for c in checks),
            "host goal lacks actual structured baseline/current proof for the same condition")
    require(records[-1].get("kind") == "done" and records[-1].get("status") == "complete",
            "host goal has no recorded complete result")
    return records[-1]["seq"]


def validate_completion(report, done_seq):
    require(report.get("schema") == "status/v1" and report.get("status") == "complete"
            and report.get("rules") == "run-rules/3"
            and type(report.get("completion_record")) is int
            and report["completion_record"] == done_seq,
            "installed Core does not validate the host goal's recorded /3 completion")


def private_text(data):
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        return
    require(not re.search(r"/(?:Users|home)/(?!runner/|user/|example/)[^\s/]+/", text)
            and not re.search(r"github\.com[/:]jgoneit/jaekit-[\w.-]+", text, re.I),
            "public release contains private environment references")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("criterion", choices=[f"AC-{n}" for n in range(1, 12)])
    args = parser.parse_args(argv)
    try:
        checker = Checker()
        getattr(checker, "check_" + args.criterion[3:])()
    except Failure as error:
        print(f"FAIL {args.criterion}: {error}", file=sys.stderr)
        return 1
    except (Unavailable, OSError, ValueError, KeyError, TypeError) as error:
        # File paths, raw stderr, prompts and trace content remain private.
        message = str(error) if isinstance(error, Unavailable) else "evidence is missing or malformed"
        print(f"UNVERIFIED {args.criterion}: {message}", file=sys.stderr)
        return 2
    print(f"PASS {args.criterion}: observed release requirement satisfied")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
