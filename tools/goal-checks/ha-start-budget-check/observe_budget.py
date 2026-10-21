"""Automatic run budgets, the pre-approved limit and numeric budgets (AC-7 to AC-10).

Each test builds `ha` from the checked tree and runs it in synthetic goal
repositories whose user configuration folder and home are temporary
directories (budget_common.Repo). The tests read only `ha`'s exit codes and
output, `ha status --format json` and the synthetic runs.jsonl. Each test's
first requirement is the one the base commit violates; later requirements
check the remaining forms the contract names (PLAN 구현 결정 R1-R6).
"""
import json
import os
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from budget_common import (AUTO, CONFIG_DIR, CONFIG_FILE, CONFIG_KEY, GOAL, ROOT, Repo, Unavailable, auto_value, build,
                           go_env, require, run, temporary)


def config(value):
    return json.dumps({CONFIG_KEY: value})


class AutoStart(unittest.TestCase):
    """AC-7: an automatic PLAN budget starts with max(50, ceil(minimum x 1.5)) under the pre-approved limit."""

    def test_auto_start(self):
        with temporary("jaekit-auto-start-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "c34", ha, changes=34)
            path = repo.write_config(config(120))
            code, out, err = repo.start()
            require(self, code == 0, "auto-budget-missing",
                    f"an automatic budget with 34 change conditions and a pre-approved limit of 120 did not start: exit {code}: {(out + err)[-800:]}")
            problems = []
            for token in ("minimum required: 68 runs", "auto limit: 102", f"pre-approved limit: 120 from {path}"):
                if token not in out:
                    problems.append(f"start output lacks {token!r}: {out[-600:]}")
            records = repo.records()
            budget = records[0].get("budget") if records else None
            want = {"runs": 102, "elapsed_seconds": 36000, "cost": "미관측",
                    "auto": {"minimum_runs": 68, "computed_runs": 102, "preapproved_runs": 120, "source": "user_config"}}
            if budget != want:
                problems.append(f"start record budget: got {budget}, want {want}")
            if str(path) in json.dumps(records):
                problems.append("the start record contains the user configuration path")
            _, report = repo.status()
            if (report.get("budget") or {}).get("runs_limit") != 102:
                problems.append(f"status budget.runs_limit: {(report.get('budget') or {}).get('runs_limit')}")
            # Representative minimums: 0 (manual only, no configuration) and 35 (rounded up).
            for name, kinds, cap, minimum in (("manual", dict(manuals=2), None, 0),
                                              ("c17m1", dict(changes=17, maintains=1), 60, 35)):
                other = Repo(Path(directory) / name, ha, **kinds)
                if cap is not None:
                    source = f"pre-approved limit: {cap} from {other.write_config(config(cap))}"
                else:
                    source = "pre-approved limit: 50 from default"
                expected = auto_value(minimum)
                code, out, err = other.start()
                records = other.records()
                budget = records[0].get("budget") if records else {}
                if code != 0 or source not in out or f"auto limit: {expected}" not in out:
                    problems.append(f"{name}: exit {code}, want auto limit {expected} and {source!r}: {(out + err)[-600:]}")
                elif (budget or {}).get("runs") != expected or (budget.get("auto") or {}).get("minimum_runs") != minimum:
                    problems.append(f"{name}: start record budget {budget}")
                elif (budget.get("auto") or {}).get("source") != ("default" if cap is None else "user_config"):
                    problems.append(f"{name}: source {(budget.get('auto') or {}).get('source')}")
        require(self, not problems, "auto-budget-missing", "; ".join(problems))


class AutoRefusal(unittest.TestCase):
    """AC-8: a computed limit above the pre-approved limit refuses the start without a record; estimate agrees."""

    def test_over_limit(self):
        with temporary("jaekit-auto-refusal-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "c34", ha, changes=34)
            path = repo.write_config(config(101))
            code, out, err = repo.start()
            require(self, code == 65 and "auto budget 102 runs" in err and not repo.records_path().exists(),
                    "auto-refusal-missing",
                    f"start over the pre-approved limit: exit {code}, records {repo.records_path().exists()}: {(out + err)[-800:]}")
            problems = []
            for token in ("minimum 68", f"pre-approved limit 101 from {path}", "approval"):
                if token not in err:
                    problems.append(f"refusal lacks {token!r}: {err[-600:]}")
            code, report, text = repo.estimate()
            auto = (report or {}).get("auto") or {}
            want = {"computed_runs": 102, "preapproved_runs": 101, "source": "user_config", "source_path": str(path),
                    "approval_needed": True}
            if code != 1 or auto != want or (report or {}).get("minimum_runs") != 68:
                problems.append(f"estimate json: exit {code}, auto {auto}, want {want}: {text[-400:]}")
            code, text = repo.estimate_md()
            if code != 1 or "auto budget: 102 runs" not in text or "user approval needed: yes" not in text:
                problems.append(f"estimate markdown: exit {code}: {text[-600:]}")
            if repo.records_path().exists():
                problems.append("estimate or the refused start wrote runs.jsonl")
            # Without a configuration the limit is 50 from the default.
            repo.clear_config()
            code, out, err = repo.start()
            if code != 65 or "pre-approved limit 50 from default" not in err or repo.records_path().exists():
                problems.append(f"start without configuration: exit {code}: {err[-600:]}")
            code, report, text = repo.estimate()
            auto = (report or {}).get("auto") or {}
            want = {"computed_runs": 102, "preapproved_runs": 50, "source": "default", "approval_needed": True}
            if code != 1 or auto != want:
                problems.append(f"estimate without configuration: exit {code}, auto {auto}, want {want}")
            # A limit equal to the computed value starts.
            repo.write_config(config(102))
            code, report, text = repo.estimate()
            if code != 0 or ((report or {}).get("auto") or {}).get("approval_needed") is not False:
                problems.append(f"estimate at an equal limit: exit {code}: {text[-400:]}")
            code, out, err = repo.start()
            if code != 0 or [r.get("budget", {}).get("runs") for r in repo.records()] != [102]:
                problems.append(f"start at an equal limit: exit {code}: {(out + err)[-400:]}")
        require(self, not problems, "auto-refusal-missing", "; ".join(problems))


def bad_forms(repo):
    """(name, prepare) for the representative unusable configurations of R3 and R5."""
    def text(content):
        return lambda: repo.write_config(content)

    def directory():
        path = repo.config_path()
        path.mkdir(parents=True)
        return path

    def not_a_directory():
        folder = repo.xdg / CONFIG_DIR
        folder.write_text("", encoding="utf-8")
        return folder / CONFIG_FILE

    def dangling():
        path = repo.config_path()
        path.parent.mkdir(parents=True)
        try:
            path.symlink_to(repo.base / "moved-away.json")
        except OSError:
            raise Unavailable("cannot create a symbolic link here") from None
        return path

    forms = [("not-json", text("preapproved_runs = 120\n")),
             ("duplicate-key", text('{"preapproved_runs": 120, "preapproved_runs": 130}')),
             ("unknown-key", text('{"preapproved_runs": 120, "note": "x"}')),
             ("typo-key", text('{"preapprove_runs": 120}')),
             ("empty-object", text("{}")),
             ("string", text('{"preapproved_runs": "120"}')),
             ("zero", text('{"preapproved_runs": 0}')),
             ("negative", text('{"preapproved_runs": -1}')),
             ("fraction", text('{"preapproved_runs": 1.5}')),
             ("exponent", text('{"preapproved_runs": 1e2}')),
             ("too-large", text('{"preapproved_runs": 2147483648}')),
             ("null", text('{"preapproved_runs": null}')),
             ("array", text("[120]")),
             ("trailing", text('{"preapproved_runs": 120} {}')),
             ("invalid-utf8", text(b'{"preapproved_runs": 120, "\xff": 1}')),
             ("directory", directory),
             ("not-a-directory", not_a_directory)]
    def dangling_folder():
        folder = repo.xdg / CONFIG_DIR
        try:
            folder.symlink_to(repo.base / "moved-dotfiles", target_is_directory=True)
        except OSError:
            raise Unavailable("cannot create a symbolic link here") from None
        return folder / CONFIG_FILE

    if os.name != "nt":
        forms += [("dangling-symlink", dangling), ("dangling-folder-symlink", dangling_folder)]
    if os.name != "nt" and hasattr(os, "geteuid") and os.geteuid() != 0:
        def unreadable():
            path = repo.write_config(config(120))
            path.chmod(0)
            return path

        def unsearchable_folder():
            path = repo.write_config(config(120))
            path.parent.chmod(0)
            return path
        forms += [("unreadable", unreadable), ("unsearchable-folder", unsearchable_folder)]
    return forms


class CapSource(unittest.TestCase):
    """AC-9: the pre-approved limit comes only from <config folder>/jaekit/ha.json; unusable files refuse."""

    def test_cap_source(self):
        with temporary("jaekit-cap-source-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "c34", ha, changes=34)
            path = repo.write_config(config(120))
            code, out, err = repo.start()
            records = repo.records()
            stored = ((records[0].get("budget") or {}).get("auto") or {}) if records else {}
            require(self, code == 0 and stored.get("preapproved_runs") == 120, "cap-source-missing",
                    f"the limit in {path} was not used: exit {code}, auto {stored}: {(out + err)[-600:]}")
            repo.reset_records()
            problems = []

            def estimate_auto(env=None):
                code, report, text = repo.estimate(env=env)
                return code, (report or {}).get("auto") or {}, text

            # The folder is $XDG_CONFIG_HOME/jaekit when that is set and not empty, otherwise ~/.config/jaekit.
            repo.clear_config()
            home_file = repo.write_config(config(120), base=repo.home / ".config")
            for label, env in (("empty XDG_CONFIG_HOME", dict(repo.env, XDG_CONFIG_HOME="")),
                               ("unset XDG_CONFIG_HOME", {k: v for k, v in repo.env.items() if k != "XDG_CONFIG_HOME"})):
                code, auto, text = estimate_auto(env)
                if code != 0 or auto.get("preapproved_runs") != 120 or auto.get("source_path") != str(home_file):
                    problems.append(f"{label}: exit {code}, auto {auto}: {text[-300:]}")
            # With XDG_CONFIG_HOME set, the home file is not consulted.
            code, auto, text = estimate_auto()
            if auto.get("preapproved_runs") != 50 or auto.get("source") != "default":
                problems.append(f"XDG set, file only under HOME: exit {code}, auto {auto}")
            repo.clear_config()
            # A leading UTF-8 byte order mark is accepted; a symlink to a file outside the repository is followed.
            repo.write_config(b"\xef\xbb\xbf" + config(120).encode())
            code, auto, text = estimate_auto()
            if code != 0 or auto.get("preapproved_runs") != 120:
                problems.append(f"BOM: exit {code}, auto {auto}: {text[-300:]}")
            repo.clear_config()
            outside = Path(directory) / "dotfiles" / "ha.json"
            outside.parent.mkdir()
            outside.write_text(config(120), encoding="utf-8")
            link = repo.config_path()
            link.parent.mkdir(parents=True)
            try:
                link.symlink_to(outside)
                linked = True
            except OSError:
                linked = False  # Windows without symlink rights: the form is not observed here.
            if linked:
                code, auto, text = estimate_auto()
                if code != 0 or auto.get("preapproved_runs") != 120:
                    problems.append(f"symlink outside the repository: exit {code}, auto {auto}: {text[-300:]}")
            repo.clear_config()
            # Unusable files refuse in both commands, naming the file; nothing is recorded.
            for name, prepare in bad_forms(repo):
                repo.clear_config()
                shown = prepare()
                code, auto, text = estimate_auto()
                if code != 65 or "pre-approved run limit" not in text or str(shown) not in text:
                    problems.append(f"{name}: estimate exit {code}: {text[-300:]}")
                code, out, err = repo.start()
                if code != 65 or str(shown) not in err or repo.records_path().exists():
                    problems.append(f"{name}: start exit {code}, records {repo.records_path().exists()}: {err[-300:]}")
                    repo.reset_records()
                for path, mode in ((shown.parent, 0o755), (shown, 0o644)):
                    try:
                        if not path.is_symlink():
                            path.chmod(mode)
                    except OSError:
                        pass
            repo.clear_config()
            # A relative XDG_CONFIG_HOME is refused.
            env = dict(repo.env, XDG_CONFIG_HOME="relative/config")
            (repo.root / "relative" / "config" / CONFIG_DIR).mkdir(parents=True)
            (repo.root / "relative" / "config" / CONFIG_DIR / CONFIG_FILE).write_text(config(500), encoding="utf-8")
            code, auto, text = estimate_auto(env)
            if code != 65 or "XDG_CONFIG_HOME" not in text:
                problems.append(f"relative XDG_CONFIG_HOME: estimate exit {code}: {text[-300:]}")
            code, out, err = repo.start(env=env)
            if code != 65 or repo.records_path().exists():
                problems.append(f"relative XDG_CONFIG_HOME: start exit {code}: {err[-300:]}")
                repo.reset_records()
            # A file inside the repository never sets the limit, directly or through a symlink.
            inside = repo.root / "cfg"
            (inside / CONFIG_DIR).mkdir(parents=True)
            (inside / CONFIG_DIR / CONFIG_FILE).write_text(config(500), encoding="utf-8")
            env = dict(repo.env, XDG_CONFIG_HOME=str(inside))
            code, auto, text = estimate_auto(env)
            if code != 65 or "repository" not in text:
                problems.append(f"configuration inside the repository: estimate exit {code}: {text[-300:]}")
            code, out, err = repo.start(env=env)
            if code != 65 or repo.records_path().exists():
                problems.append(f"configuration inside the repository: start exit {code}: {err[-300:]}")
                repo.reset_records()
            if linked:
                link = repo.config_path()
                link.parent.mkdir(parents=True, exist_ok=True)
                link.symlink_to(inside / CONFIG_DIR / CONFIG_FILE)
                code, auto, text = estimate_auto()
                if code != 65 or "repository" not in text:
                    problems.append(f"symlink into the repository: estimate exit {code}: {text[-300:]}")
                repo.clear_config()
            # The same folders named with other letter case, where the file system ignores case: the
            # configuration folder, and the repository's own folder above it.
            for label, spelled in (("configuration folder", inside.parent / inside.name.upper()),
                                   ("repository folder", repo.root.parent / repo.root.name.upper() / inside.name)):
                if spelled.exists():
                    code, auto, text = estimate_auto(dict(repo.env, XDG_CONFIG_HOME=str(spelled)))
                    if code != 65 or "repository" not in text:
                        problems.append(f"{label} named in other case: estimate exit {code}: {text[-300:]}")
            # A file in another working tree of the same repository (the goal runs in a linked worktree).
            other = repo.base / "linked"
            repo.git("worktree", "add", "-q", str(other))
            env = dict(repo.env, XDG_CONFIG_HOME=str(inside))
            code, out, err = repo.ha("estimate", GOAL, "--format", "json", env=env, cwd=other)
            if code != 65 or "repository" not in out + err:
                problems.append(f"configuration in the main checkout, goal in a linked worktree: estimate exit {code}: {(out + err)[-300:]}")
        require(self, not problems, "cap-source-missing", "; ".join(problems))


# Existing budget tests of ./cmd/ha, by name. Scenarios are subtests of TestScenarios.
SELECTED_SCENARIOS = ("start-budget-minimum", "budget-runs", "budget-v3-change", "budget-only-complete",
                      "rules-v4-preapprove", "rules-v4-preapprove-quoted", "rules-v4-reopen-decrease", "reopen-after-done")
SELECTED_TESTS = ("TestEstimateAC8", "TestStartBudgetAC9", "TestEstimateAC10", "TestBaselineBudgetAC8_TotalLimit",
                  "TestBaselineBudgetAC9_TimeUnchanged", "TestBaselineBudgetAC10_ConsentAndValidation",
                  "TestBaselineBudgetAC13_ReopenAndCompletion", "TestBaselineBudgetAC14_Legacy")
SELECTED_RUNS = (("^TestScenarios$/^(" + "|".join(SELECTED_SCENARIOS) + ")$",
                  ["TestScenarios/" + name for name in SELECTED_SCENARIOS]),
                 ("^(" + "|".join(SELECTED_TESTS) + ")$", list(SELECTED_TESTS)))


class NumericKept(unittest.TestCase):
    """AC-10: numeric PLAN budgets, the elapsed limit and the rules after the start stay as they are."""

    def test_numeric_kept(self):
        problems = []
        with temporary("jaekit-numeric-kept-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            # A numeric limit is not compared with the pre-approved limit, and the file is not read.
            for name, content in (("corrupt", "not json"), ("smaller", config(10))):
                repo = Repo(Path(directory) / name, ha, changes=10, runs="60")
                repo.write_config(content)
                code, out, err = repo.start()
                records = repo.records()
                budget = records[0].get("budget") if records else None
                if code != 0 or budget != {"runs": 60, "elapsed_seconds": 36000, "cost": "미관측"}:
                    problems.append(f"numeric 60 with a {name} configuration: exit {code}, budget {budget}: {(out + err)[-300:]}")
                code, out, err = repo.ha("estimate", GOAL, "--format", "json")
                if code != 0 or '"auto"' in out or '"plan_runs_limit": 60' not in out:
                    problems.append(f"numeric estimate with a {name} configuration: exit {code}: {(out + err)[-300:]}")
            # A numeric item keeps today's meaning when an automatic item comes before or after it.
            for name, runs in (("numeric-first", "60\n- 검증 실행: " + AUTO), ("auto-first", AUTO + "\n- 검증 실행: 60")):
                repo = Repo(Path(directory) / name, ha, changes=10, runs=runs)
                repo.write_config(config(10))
                code, out, err = repo.start()
                records = repo.records()
                if code != 0 or (records[0].get("budget") if records else None) != {"runs": 60, "elapsed_seconds": 36000, "cost": "미관측"}:
                    problems.append(f"numeric 60 and {AUTO} ({name}): exit {code}: {(out + err)[-300:]}")
            # Below the minimum the start is refused as before.
            repo = Repo(Path(directory) / "short", ha, changes=34, runs="60")
            repo.write_config(config(1000))
            code, out, err = repo.start()
            if code != 65 or "minimum 68 runs exceeds plan limit 60 (shortfall 8)" not in err or repo.records_path().exists():
                problems.append(f"numeric below the minimum: exit {code}: {err[-300:]}")
            # The elapsed limit stays required and numeric.
            for label, elapsed in (("missing", None), ("automatic", AUTO)):
                repo = Repo(Path(directory) / f"elapsed-{label}", ha, changes=1, runs="60", elapsed=elapsed)
                code, out, err = repo.ha("lint", GOAL)
                if code != 1 or "budget_missing" not in out:
                    problems.append(f"elapsed {label}: lint exit {code}: {(out + err)[-300:]}")
                code, out, err = repo.start()
                if code != 65 or repo.records_path().exists():
                    problems.append(f"elapsed {label}: start exit {code}")
            # Editing the PLAN after the start does not change the active limit.
            repo = Repo(Path(directory) / "edit", ha, changes=10, runs="80")
            code, out, err = repo.start()
            if code != 0:
                problems.append(f"numeric 80: start exit {code}: {err[-300:]}")
            else:
                repo.set_budget(AUTO)
                repo.write_config(config(500))
                repo.commit("plan edit")
                _, report = repo.status()
                if (report.get("budget") or {}).get("runs_limit") != 80:
                    problems.append(f"after a PLAN edit runs_limit is {(report.get('budget') or {}).get('runs_limit')}")
        with temporary("jaekit-numeric-go-") as directory:
            self.selected(go_env(directory), problems)
        require(self, not problems, "numeric-budget-changed", "; ".join(problems))

    def selected(self, env, problems):
        for pattern, names in SELECTED_RUNS:
            result = run(["go", "test", "-count=1", "-v", "-run", pattern, "./cmd/ha"], ROOT, env, timeout=1500)
            output = (result.stdout + result.stderr).decode(errors="replace")
            passed = {line.split("--- PASS: ", 1)[1].split(" ", 1)[0] for line in output.splitlines() if "--- PASS: " in line}
            failed = {line.split("--- FAIL: ", 1)[1].split(" ", 1)[0] for line in output.splitlines() if "--- FAIL: " in line}
            if result.returncode and not failed:
                raise Unavailable(f"go test exit {result.returncode} without a failed test: {output[-1500:]}")
            if failed:
                problems.append(f"existing budget tests failed: {sorted(failed)}")
            elif not set(names) <= passed:
                raise Unavailable(f"the selected budget tests did not run: {sorted(set(names) - passed)}")


if __name__ == "__main__":
    unittest.main()
