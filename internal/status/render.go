package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// JSON renders the report as indented JSON with a trailing newline. Field
// order follows the struct definitions and is fixed.
func (rep *Report) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func ints(xs []int) string {
	if len(xs) == 0 {
		return "—"
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return strings.Join(parts, ", ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func cell(s string) string {
	if s == "" {
		return "—"
	}
	return strings.ReplaceAll(s, "|", `\|`)
}

// DisplayGoal is the goal path that commands in human output name: the
// argument as given, so a command works from the same directory, or the
// repository-relative goal path (run from the repository root) when the
// argument is absolute. Human output never carries an absolute path.
func DisplayGoal(arg, repoGoal string) string {
	if arg == "" || filepath.IsAbs(arg) || strings.HasPrefix(arg, "/") {
		return repoGoal
	}
	return arg
}

// unmet lists what keeps the goal from completing before the criteria table:
// each unsatisfied required criterion with its reason, class and the fresh
// record the reason rests on, then each goal-level reason. A record whose
// output was stored gets the exact `ha log` command (see DisplayGoal);
// others say that no output is stored. Nothing is listed once the goal is
// complete.
func (rep *Report) unmet(w func(string, ...any), goal string) {
	if rep.Status == Complete || len(rep.Reasons) == 0 {
		return
	}
	w("\n### Unmet\n\n")
	for _, r := range rep.Reasons {
		if r.Criterion == "" {
			w("- `%s` (%s)\n", r.Code, r.Class)
			continue
		}
		line := fmt.Sprintf("- %s `%s` (%s)", r.Criterion, r.Code, r.Class)
		switch j, ok := rep.judged[r.Criterion]; {
		case ok && j.Output != "":
			line += fmt.Sprintf(": seq %d, output `ha log %s %d`", j.Seq, goal, j.Seq)
		case ok:
			line += fmt.Sprintf(": seq %d, no stored output", j.Seq)
		case r.Code != "manual_unconfirmed":
			line += ": no fresh record"
		}
		w("%s\n", line)
	}
}

// Markdown renders the report for people and for pasting into PROGRESS.md.
// It never contains command output.
func (rep *Report) Markdown(goalArg string) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("## ha status: %s\n\n", rep.Goal)
	w("- status: **%s** (exit %d)\n", rep.Status, rep.ExitCode)
	goal := DisplayGoal(goalArg, rep.Goal)
	if rep.CompletionRecord != nil {
		w("- completion record: seq %d\n", *rep.CompletionRecord)
	} else {
		line := "- completion record: none"
		if d := rep.doneState; d != nil {
			if d.LatestStatus != Complete {
				line += fmt.Sprintf("; latest done seq %d (commit %s) recorded %s, not a completion",
					d.LatestSeq, shortHash(d.LatestCommit), d.LatestStatus)
			}
			if s := d.Completion; s != nil {
				line += fmt.Sprintf("; latest completion seq %d (commit %s) is no longer valid: %s",
					s.Seq, shortHash(s.Commit), strings.Join(s.Reasons, "; "))
			}
		}
		if rep.CompleteWithoutRecord {
			line += fmt.Sprintf("; run `ha done %s` to record completion", goal)
		}
		w("%s\n", line)
	}
	if rep.Findings != nil && len(rep.Findings.Findings) > 0 {
		w("\n%s\n", rep.Findings.Markdown())
	}
	w("- assurance: local (an agent with the same user permissions can change checks and records)\n")
	w("- check author: executor\n")
	if rep.RecordHead != nil {
		w("- record head: seq %d (%s)\n", rep.RecordHead.Seq, shortHash(rep.RecordHead.SHA256))
	} else {
		w("- record head: none\n")
	}
	skill := "not recorded"
	if rep.Skill != nil && rep.Skill.Name != "" {
		skill = fmt.Sprintf("%s %s (claimed)", rep.Skill.Name, rep.Skill.Version)
	}
	w("- rules: %s; skill: %s; ha: %s\n", rep.Rules, skill, rep.HaVersion)
	w("- code: %s, tree clean: %s; goal digest: %s\n", shortHash(rep.Commit), yesNo(rep.TreeClean), shortHash(rep.SpecDigest))
	u := rep.Usage
	w("- attempts: %d total; types: %d check, %d baseline; outcomes: %d environment errors, %d unknown; inputs: %d current, %d stale; valid positive evidence: %d (overlapping axes)\n", u.Total, u.Checks, u.Baselines, u.EnvironmentErrors, u.Unknowns, u.CurrentInputs, u.StaleInputs, u.ValidEvidence)

	rep.unmet(w, goal)

	w("\n### Criteria\n\n| ID | kind | required | satisfied | reason | records | attempts |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, c := range rep.Criteria {
		a := c.Attempts
		attempts := fmt.Sprintf("checks %d (fail %d, error %d), baselines %d (unexpected pass %d, error %d), stale %d", a.Checks, a.Fails, a.Errors, a.Baselines, a.UnexpectedPasses, a.BaselineErrors, a.Stale)
		if a.Unknowns+a.BaselineUnknowns+a.BaselineFails > 0 {
			attempts += fmt.Sprintf("; unknown check %d, baseline %d; baseline fail %d", a.Unknowns, a.BaselineUnknowns, a.BaselineFails)
		}
		if len(a.FailCommits) > 0 {
			attempts += "; failed at " + strings.Join(a.FailCommits, ", ")
		}
		w("| %s | %s | %s | %s | %s | %s | %s |\n", c.ID, cell(c.Kind), yesNo(c.Required), yesNo(c.Satisfied), cell(c.Reason), ints(c.Records), attempts)
	}

	w("\n### Reasons\n\n")
	if len(rep.Reasons) == 0 {
		w("None.\n")
	}
	for _, r := range rep.Reasons {
		line := fmt.Sprintf("- `%s` (%s)", r.Code, r.Class)
		if r.Criterion != "" {
			line += " " + r.Criterion
		}
		if r.Detail != "" {
			line += ": " + r.Detail
		}
		// Dirty details already contain quoted paths and their Git metadata.
		if len(r.Paths) > 0 && r.Code != "worktree_dirty" {
			line += " — " + strings.Join(r.Paths, ", ")
		}
		w("%s\n", line)
	}
	if len(rep.Lint) > 0 {
		w("\n### Lint\n\n")
		for _, p := range rep.Lint {
			loc := p.File
			if p.Line > 0 {
				loc = fmt.Sprintf("%s:%d", p.File, p.Line)
			}
			w("- `%s` %s: %s\n", p.Code, loc, p.Detail)
		}
	}

	ch := rep.Changes
	if len(ch.CommandChanged)+len(ch.KindChanged)+len(ch.Optional)+len(ch.InvalidConfirmations)+len(ch.HaVersionChanged) > 0 {
		w("\n### Changes to review\n\n")
		for _, c := range ch.CommandChanged {
			w("- command changed: %s was `%s` at seq %d, now `%s`\n", c.Criterion, strings.Join(c.Before, " "), c.Seq, c.After)
		}
		for _, c := range ch.KindChanged {
			w("- kind changed: %s was %s at seq %d, now %s\n", c.Criterion, c.Before, c.Seq, c.After)
		}
		if len(ch.Optional) > 0 {
			w("- optional conditions (do not block completion): %s\n", strings.Join(ch.Optional, ", "))
		}
		for _, c := range ch.InvalidConfirmations {
			w("- invalid confirmation: seq %d `%s`: %s\n", c.Seq, c.Subject, c.Reason)
		}
		for _, c := range ch.HaVersionChanged {
			w("- ha version changed: seq %d written by ha %s\n", c.Seq, c.HaVersion)
		}
	}
	if rep.Budget != nil {
		bd := rep.Budget
		w("\n### Budget\n\n- window from seq %d: %d/%d runs, %d/%d seconds%s\n", bd.WindowStart, bd.Runs, bd.RunsLimit, bd.ElapsedSeconds, bd.ElapsedLimitSeconds, map[bool]string{true: " (exceeded)", false: ""}[bd.Exceeded])
		w("- budget revision: %d; remaining: %d runs; excess: %d runs; run cap reached: %s\n", bd.Revision, bd.RemainingRuns, bd.ExcessRuns, yesNo(bd.Reached))
		for _, event := range bd.History {
			w("- budget change seq %d, window %d: %d → %d runs (user attribution is a claim)\n", event.Seq, event.Change.WindowStart, event.Change.PreviousRuns, event.Change.Runs)
		}
	}
	if len(rep.Logs) > 0 {
		w("\n### Local output\n\nView with `ha log %s <seq>`; output is never copied into the record.\n\n", goal)
		for _, l := range rep.Logs {
			reason := ""
			if l.Reason != "" {
				reason = " (" + l.Reason + ")"
			}
			w("- seq %d %s %s%s: %s\n", l.Seq, l.Target, l.Result, reason, l.Path)
		}
	}
	return b.String()
}
