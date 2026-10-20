"""`ha report` observations (AC-1, AC-2, AC-3).

See common.py for the fixture and the result rules. Each case builds `ha`
from the checked tree once and walks synthetic goals through fixed states.
"""
import json
from pathlib import Path
import re
import subprocess
import sys
import time
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from common import (GOAL, HEADINGS, LABELS, SECRET, Condition, Repo, Temporary, Unavailable, build,  # noqa: E402
                    expect_details, expect_summary, require, sections, snapshot_diff, summary_lines)

FULL = (Condition("AC-1", "change"), Condition("AC-2", "maintain"), Condition("AC-3", "manual"),
        Condition("AC-4", "maintain", optional=True))
LANGS = (("en", ()), ("ko", ("--lang", "ko")))


class Built(unittest.TestCase):
    """Builds ha once per case; each case makes its own repositories."""

    def setUp(self):
        self.temporary = Temporary("jaekit-report-")
        self.dir = self.temporary.__enter__()
        self.addCleanup(self.temporary.__exit__, None, None, None)
        self.ha = build(self.dir / "bin" / "ha")

    def repo(self, name, **kwargs):
        return Repo(self.dir / name, self.ha, **kwargs)

    def report(self, repo, violation, *args, cwd=None):
        code, out, err = repo.report(*args, cwd=cwd)
        require(self, code == 0, violation, f"ha report {' '.join(args)} exited {code}: {(out + err)[-800:]}")
        return out


class Report(Built):
    def test_five_labels_in_both_languages(self):
        """AC-1: five labelled lines in a fixed order; en by default; other values exit 64 without output."""
        v = "report-summary-missing"
        repo = self.repo("labels", conditions=FULL)
        repo.start()
        repo.check("AC-1", baseline=True, expect=0)
        repo.implement()
        repo.check("AC-1", "AC-2", expect=0)
        repo.flag("AC-2")
        repo.check("AC-2", expect=1)
        for lang, args in LANGS + (("en", ("--lang", "en")),):
            out = self.report(repo, v, *args)
            lines = out.split("\n")
            require(self, len(lines) > 6 and lines[5] == "", v, f"{lang}: the summary is not five lines then a blank line")
            for label, line in zip(LABELS[lang], lines[:5]):
                require(self, line.startswith(label + ": ") and "\n" not in line, v, f"{lang}: {line!r} does not start with {label!r}")
        default = self.report(repo, v)
        require(self, default == self.report(repo, v, "--lang", "en"), v, "without --lang the output is not the en output")
        for value in ("fr", "KO", "", "ko,en", "english"):
            code, out, _ = repo.report("--lang", value)
            require(self, code == 64 and out == "", v, f"--lang {value!r} exited {code} with output {out[:200]!r}")
        code, out, _ = repo.report("--lang")
        require(self, code == 64 and out == "", v, f"--lang without a value exited {code} with output {out[:200]!r}")
        tokens = lambda s: sorted(t for t in re.findall(
            r"`[^`]*`|\b[0-9a-f]{12}\b|\bseq \d+\b|\b(?:AC|EX)-\d+\b|\b(?:incomplete|needs_user|budget_exhausted)\b", s))
        en, ko = self.report(repo, v), self.report(repo, v, "--lang", "ko")
        require(self, tokens(en) == tokens(ko), v, f"tokens differ by language: en {tokens(en)[:20]} ko {tokens(ko)[:20]}")

    def test_summary_line_contents(self):
        """AC-1: each line's content for each state, in both languages."""
        v = "report-summary-wrong"

        def expect(repo, changed, fresh, confirmations=None, blocked=None, note=""):
            status = repo.status()
            for lang, args in LANGS:
                got = summary_lines(self.report(repo, v, *args))
                want = expect_summary(repo, status, lang, changed, fresh, confirmations, blocked)
                require(self, got == want, v, f"{note} {lang}: got {got} want {want}")

        repo = self.repo("states", conditions=FULL)
        good_plan = repo.plan()
        repo.write_goal(plan=good_plan.replace(next(l for l in good_plan.splitlines() if l.startswith("| AC-2 ")) + "\n", ""))
        expect(repo, ("not_started",), (0, 1), note="not started with a lint problem")
        repo.write_goal(plan=good_plan)
        expect(repo, ("not_started",), (0, 1), note="not started")
        repo.start()
        expect(repo, ("counts", 0, 0), (0, 1), note="started")
        code, _ = repo.check("AC-1", baseline=True, expect=0)
        repo.note("block", "--cause", "permission", "--quote", "권한 창이 떴어")
        block_seq = repo.lines()[-1]["seq"]
        expect(repo, ("counts", 0, 0), (1, 1), blocked=(block_seq, "permission"), note="blocked")
        repo.note("unblock")
        repo.implement()
        repo.write("other/outside.txt", "outside\n")
        repo.commit("outside the scope")
        repo.check("AC-1", "AC-2", expect=0)
        expect(repo, ("counts", 2, 2), (1, 1), note="manual and out of scope")
        repo.note("confirm", "AC-3", "--quote", "화면을 봤고 맞아")
        confirm = repo.lines()[-1]["seq"]
        expect(repo, ("counts", 2, 2), (1, 1), {"AC-3": confirm}, note="out of scope only")
        repo.note("confirm", "scope:other/outside.txt", "--quote", "그 파일은 괜찮아")
        expect(repo, ("counts", 2, 2), (1, 1), {"AC-3": confirm}, note="complete without a record")
        progress = repo.read(f"{GOAL}/PROGRESS.md")
        repo.write(f"{GOAL}/PROGRESS.md", progress.replace("| T001 | done |", "| T001 | todo |"))
        if [r["code"] for r in repo.status()["reasons"]] != ["task_open"]:
            raise Unavailable("the open-task fixture did not leave task_open alone")
        expect(repo, ("counts", 2, 2), (1, 1), {"AC-3": confirm}, note="an open task")
        repo.write(f"{GOAL}/PROGRESS.md", progress)
        repo.done()
        expect(repo, ("counts", 2, 2), (1, 1), {"AC-3": confirm}, note="complete with a record")
        repo.flag("AC-2")
        repo.check("AC-2", expect=1)
        expect(repo, ("counts", 2, 2), (1, 1), {"AC-3": confirm}, note="failed after completion")

        budget = self.repo("budget", conditions=(Condition("AC-1", "maintain"), Condition("AC-2", "maintain")), runs=2)
        budget.start()
        budget.check("AC-1", expect=0)
        budget.flag("AC-2")
        budget.check("AC-2", expect=1)
        budget.check("AC-1", expect=0)
        expect(budget, ("counts", 0, 0), (0, 0), note="budget")

        compare = self.repo("compare")
        compare.start()
        middle = compare.implement()
        compare.write("src/later.txt", "later\n")
        compare.commit("later")
        compare.check("AC-2", expect=0)
        loose = compare.root / ".git" / "objects" / middle[:2] / middle[2:]
        if not loose.is_file():
            raise Unavailable("the middle commit is not a loose object")
        loose.unlink()
        start = next(l for l in compare.lines() if l["kind"] == "start")
        expect(compare, ("cannot_compare", start["base_commit"], compare.head()), (0, 1), note="cannot compare")

        tracked = self.repo("tracked", track_bundle=True)
        tracked.start()
        tracked.implement()
        tracked.write(f"{GOAL}/PROGRESS.md", tracked.read(f"{GOAL}/PROGRESS.md") + "\n진행 메모\n")
        tracked.commit("progress only")
        tracked.git("checkout", "-q", "-b", "side")
        tracked.write("src/side.txt", "side\n")
        tracked.commit("side change")
        tracked.git("checkout", "-q", "main")
        tracked.tick += 1
        result = subprocess.run(
            ["git", "merge", "-q", "--no-ff", "-m", "merge side", "side"], cwd=tracked.root,
            env=tracked.env({"GIT_AUTHOR_DATE": "2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE": "2026-01-01T01:00:00Z"}),
            capture_output=True)
        if result.returncode:
            raise Unavailable(f"git merge: {result.stderr.decode(errors='replace')}")
        expect(tracked, ("counts", 2, 2), (0, 1), note="bundle-only and merge commits")

        review = self.repo("review", conditions=(Condition("AC-1", "change"), Condition("AC-2", "maintain"),
                                                  Condition("AC-3", "maintain")), tests_glob="tests/**")
        review.start(extra=("--test-authority", "the request allows changing this goal's tests"))
        review.check("AC-1", baseline=True, expect=0)
        review.implement()
        for i in ("AC-2", "AC-3"):
            review.write(f"tests/{i}.sh", review.read(f"tests/{i}.sh") + "# reviewed\n")
        review.commit("touch the maintain tests")
        review.note("test-change", "tests:tests/AC-2.sh", "--criteria", "AC-2", "--how", "it still compares src/AC-2.txt with ok")
        review.note("test-change", "tests:tests/AC-3.sh", "--criteria", "AC-3", "--how", "it compares less", "--weakened")
        review.check("AC-2", "AC-3", expect=0)
        review.write_goal(plan=review.plan().replace("`sh tests/AC-3.sh`", "`sh ./tests/AC-3.sh`"))
        status = review.status()
        kinds = {(t["path"], bool(t.get("exempt")), bool((t.get("rationale") or {}).get("weakened")))
                 for t in status.get("test_changes") or []}
        if (kinds != {("tests/AC-2.sh", True, False), ("tests/AC-3.sh", False, True)}
                or [c["criterion"] for c in status["changes"]["command_changed"]] != ["AC-3"]):
            raise Unavailable(f"the test-change fixture is not as intended: {kinds} {status['changes']['command_changed']}")
        expect(review, ("counts", 3, 2), (1, 1), note="test changes and a changed command")

    def test_details_after_summary(self):
        """AC-2: stamp, evidence per condition, completion coverage and unmet output, in order."""
        v = "report-details-wrong"

        def expect(repo, note):
            status = repo.status()
            for lang, args in LANGS:
                got = sections(self.report(repo, v, *args))
                want = expect_details(repo, status, lang)
                require(self, got == want, v, f"{note} {lang}: got {got} want {want}")

        conditions = (Condition("AC-1", "change"), Condition("AC-2", "maintain"), Condition("AC-3", "manual"))
        repo = self.repo("details", conditions=conditions)
        expect(repo, "no record")
        repo.start()
        repo.check("AC-1", baseline=True, expect=0)
        repo.implement()
        repo.check("AC-1", expect=0)
        repo.flag("AC-2")
        repo.check("AC-2", expect=1)
        expect(repo, "a failed check with stored output")
        failed = repo.lines()[-1]["seq"]

        def blank_output(lines):
            for line in lines:
                if line.get("seq") == failed:
                    line["output_path"] = ""
            return lines
        repo.rewrite(blank_output)
        expect(repo, "a failed check without stored output")
        repo.flag("AC-2", present=False)
        repo.write("src/fixed.txt", "fixed\n")
        repo.commit("fix after the failure")
        repo.check("AC-1", "AC-2", expect=0)
        repo.note("confirm", "AC-3", "--quote", "화면을 봤고 맞아")
        repo.done()
        if repo.status().get("completion_record") is None:
            raise Unavailable("the fixture did not record a completion")
        expect(repo, "a valid completion record")
        repo.write("src/after.txt", "after\n")
        repo.commit("after completion")
        expect(repo, "a completion record that no longer holds")

        legacy = self.repo("legacy")
        legacy.start(rules=None)
        legacy.set_rules("run-rules/2")
        legacy.check("AC-1", baseline=True, expect=0)
        legacy.implement()
        legacy.check(expect=0)
        legacy.done()
        legacy.flag("AC-2")
        legacy.check("AC-2", expect=1)
        status = legacy.status()
        current = {c["id"]: c["satisfied"] for c in status["criteria"]}
        if status.get("completion_record") is None or current.get("AC-2") is not False:
            raise Unavailable(f"the run-rules/2 fixture did not keep its completion with a later failure: {status['status']}")
        expect(legacy, "stored evidence that differs from the current state")

    def test_read_only_and_deterministic(self):
        """AC-3: no change to the record, bundle, tree, Git or ha/runs; same output for the same inputs; no raw output."""
        v = "report-not-read-only"
        repo = self.repo("readonly", conditions=FULL)

        def observe(note):
            before, record = repo.snapshot(), repo.record_bytes()
            outputs = {}
            for lang, args in LANGS + (("en", ()),):
                first = self.report(repo, v, *args)
                time.sleep(1.1)
                second = self.report(repo, v, *args)
                require(self, first == second, v, f"{note}: two runs of {lang} differ")
                require(self, SECRET not in first, v, f"{note}: the report contains command output")
                outputs[lang] = first
            after = repo.snapshot()
            require(self, repo.record_bytes() == record, v, f"{note}: the record changed")
            require(self, before == after, v, f"{note}: files changed: {snapshot_diff(before, after)}")
            return outputs

        observe("not started")
        repo.start()
        repo.check("AC-1", baseline=True, expect=0)
        repo.implement()
        repo.flag("AC-2")
        repo.check("AC-1", "AC-2", expect=1)
        observe("failed check")
        repo.flag("AC-2", present=False)
        repo.write("src/fixed.txt", "fixed\n")
        repo.commit("fix after the failure")
        repo.check("AC-1", "AC-2", expect=0)
        repo.note("confirm", "AC-3", "--quote", "화면을 봤고 맞아")
        repo.done()
        if repo.status().get("completion_record") is None:
            raise Unavailable("the fixture did not record a completion")
        observe("complete")

    def test_rules_and_integrity(self):
        """AC-3: every saved rule works; unsupported rules exit 66 silently; a broken record shows record_integrity."""
        v = "report-rules-wrong"
        for rules in ("run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4"):
            repo = self.repo(rules.replace("/", "-"))
            repo.start(rules="run-rules/4" if rules == "run-rules/4" else None)
            if rules in ("run-rules/1", "run-rules/2"):
                repo.set_rules(rules)
            repo.check("AC-1", baseline=True, expect=0)
            repo.implement()
            repo.check(expect=0)
            status = repo.status()
            out = self.report(repo, v)
            lines = out.split("\n")
            stamp = next((l for l in lines if l.startswith("- record head: ")), "")
            require(self, all(l.startswith(label + ": ") for l, label in zip(lines[:5], LABELS["en"])), v,
                    f"{rules}: no five-line summary")
            require(self, f"seq {status['record_head']['seq']} " in stamp and f"status: {status['status']}" in stamp, v,
                    f"{rules}: stamp {stamp!r} does not match ha status")
        repo = self.repo("unsupported")
        repo.start()
        repo.set_rules("run-rules/9")
        code, out, err = repo.report()
        status_code, _, _ = repo.ha_run("status", GOAL)
        require(self, code == 66 and out == "" and status_code == 66, v,
                f"run-rules/9: report exited {code} with output {out[:200]!r} (status {status_code})")
        for lang, args in LANGS:
            code, out, _ = repo.report(*args)
            require(self, code == 66 and out == "", v, f"run-rules/9 {lang}: exited {code}")
        broken = self.repo("broken")
        broken.start()
        broken.check("AC-1", baseline=True, expect=0)
        broken.implement()
        broken.check(expect=0)
        broken.done()
        data = broken.record_bytes().decode("utf-8").split("\n")
        data[1] = data[1].replace('"kind":"baseline"', '"kind":"baseline","edited":true', 1)
        broken.records_path().write_bytes("\n".join(data).encode("utf-8"))
        status = broken.status()
        if not any(r["code"] == "record_integrity" for r in status["reasons"]):
            raise Unavailable("the edited record is not reported as broken by ha status")
        for lang, args in LANGS:
            out = self.report(broken, v, *args)
            summary = out.split("\n")[:5]
            stamp = next((l for l in out.split("\n") if l.startswith(("- record head: ", "- 기록 head: "))), "")
            require(self, "`record_integrity`" in summary[2] and "`record_integrity`" in summary[4], v,
                    f"broken record {lang}: {summary}")
            state = re.search(r"(?:status|상태): (\w+)", stamp)
            require(self, state is not None and state.group(1) != "complete", v,
                    f"broken record {lang}: the stamp shows complete: {stamp!r}")

    def test_findings_listed(self):
        """AC-3: unresolved post-completion findings are counted under To review, beside the historical completion."""
        v = "report-findings-hidden"
        repo = self.repo("findings")
        repo.start()
        repo.check("AC-1", baseline=True, expect=0)
        repo.implement()
        repo.check(expect=0)
        repo.done()
        status = repo.status()
        done = status["record_head"]
        request = {"id": "F-1", "request_id": "r-1", "revision": 0,
                   "completion": {"seq": done["seq"], "sha256": done["sha256"]}, "criteria": ["AC-1"],
                   "mapping": "linked", "status": "candidate", "category": "existing_condition",
                   "summary": "합성 발견", "source": "합성 검토", "evidence": []}
        path = self.dir / "finding.json"
        path.write_text(json.dumps(request, ensure_ascii=False), encoding="utf-8")
        code, out, err = repo.ha_run("finding", GOAL, "--input", path)
        if code:
            raise Unavailable(f"ha finding: {code} {out}{err}")
        status = repo.status()
        if (status.get("findings") or {}).get("unresolved") != 1:
            raise Unavailable("ha status does not count the synthetic finding as unresolved")
        for lang, args in LANGS:
            summary = self.report(repo, v, *args).split("\n")[:5]
            want = "해결되지 않은 완료 뒤 발견: 1개" if lang == "ko" else "unresolved post-completion findings: 1"
            require(self, want in summary[2], v, f"{lang}: {summary[2]!r} does not show the unresolved finding")


if __name__ == "__main__":
    unittest.main()
