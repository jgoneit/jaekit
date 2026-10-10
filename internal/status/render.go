package status

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// Markdown renders the report for people and for pasting into PROGRESS.md.
// It never contains command output.
func (rep *Report) Markdown(goalArg string) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("## ha status: %s\n\n", rep.Goal)
	w("- status: **%s** (exit %d)\n", rep.Status, rep.ExitCode)
	switch {
	case rep.CompletionRecord != nil:
		w("- completion record: seq %d\n", *rep.CompletionRecord)
	case rep.CompleteWithoutRecord:
		w("- completion record: none; run `ha done %s` to record completion\n", goalArg)
	default:
		w("- completion record: none\n")
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

	w("\n### Criteria\n\n| ID | kind | required | satisfied | reason | records | attempts |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, c := range rep.Criteria {
		a := c.Attempts
		attempts := fmt.Sprintf("checks %d (fail %d, error %d), baselines %d (unexpected pass %d, error %d), stale %d", a.Checks, a.Fails, a.Errors, a.Baselines, a.UnexpectedPasses, a.BaselineErrors, a.Stale)
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
	}
	if len(rep.Logs) > 0 {
		w("\n### Local output\n\nView with `ha log %s <seq>`; output is never copied into the record.\n\n", goalArg)
		for _, l := range rep.Logs {
			w("- seq %d %s %s: %s\n", l.Seq, l.Target, l.Result, l.Path)
		}
	}
	return b.String()
}
