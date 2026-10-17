package status

import (
	"strings"
	"testing"
)

// The unmet summary precedes the criteria table and names the record each
// reason rests on, or says that none exists; it never invents a seq.
func TestMarkdownUnmetSummary(t *testing.T) {
	rep := &Report{
		Goal: "docs/specs/g", Status: Incomplete, Criteria: []Criterion{},
		Reasons: []Reason{
			{Code: "criterion_failed", Class: ClassExecutor, Criterion: "AC-1"},
			{Code: "criterion_error", Class: ClassExecutor, Criterion: "AC-2"},
			{Code: "criterion_missing", Class: ClassExecutor, Criterion: "AC-3"},
			{Code: "manual_unconfirmed", Class: ClassUser, Criterion: "AC-4"},
			{Code: "task_open", Class: ClassExecutor},
		},
		judged: map[string]JudgedRecord{
			"AC-1": {Seq: 7, Output: ".git/ha/runs/docs%2Fspecs%2Fg/7.log"},
			"AC-2": {Seq: 9},
		},
	}
	out := rep.Markdown("/absolute/argument")
	summary, _, found := strings.Cut(out, "### Criteria")
	if !found || !strings.Contains(summary, "### Unmet") {
		t.Fatalf("no unmet summary before the table:\n%s", out)
	}
	for _, want := range []string{
		"- AC-1 `criterion_failed` (executor): seq 7, output `ha log docs/specs/g 7`\n",
		"- AC-2 `criterion_error` (executor): seq 9, no stored output\n",
		"- AC-3 `criterion_missing` (executor): no fresh record\n",
		"- AC-4 `manual_unconfirmed` (user)\n",
		"- `task_open` (executor)\n",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "/absolute/argument") {
		t.Errorf("new lines must use the repository-relative goal path:\n%s", summary)
	}
	rep.Status, rep.Reasons = Complete, nil
	if out := rep.Markdown("docs/specs/g"); strings.Contains(out, "### Unmet") {
		t.Errorf("a complete goal has no unmet summary:\n%s", out)
	}
}

// A stale completion keeps the existing none and ha done forms around it.
func TestMarkdownStaleCompletionLine(t *testing.T) {
	rep := &Report{Goal: "docs/specs/g", Status: Complete, CompleteWithoutRecord: true, Criteria: []Criterion{},
		doneState: &DoneState{LatestSeq: 5, LatestCommit: "0123456789abcdef", LatestStatus: Complete,
			Completion: &StaleCompletion{Seq: 5, Commit: "0123456789abcdef", Reasons: []string{"reopened at seq 6"}}}}
	want := "- completion record: none; latest completion seq 5 (commit 0123456789ab) is no longer valid: reopened at seq 6; run `ha done docs/specs/g` to record completion\n"
	if out := rep.Markdown("docs/specs/g"); !strings.Contains(out, want) {
		t.Errorf("missing %q:\n%s", want, out)
	}
	rep.doneState = &DoneState{LatestSeq: 8, LatestCommit: "fedcba9876543210", LatestStatus: Incomplete,
		Completion: &StaleCompletion{Seq: 5, Commit: "0123456789abcdef", Reasons: []string{"goal documents changed"}}}
	want = "- completion record: none; latest done seq 8 (commit fedcba987654) recorded incomplete, not a completion; latest completion seq 5 (commit 0123456789ab) is no longer valid: goal documents changed; run `ha done docs/specs/g` to record completion\n"
	if out := rep.Markdown("/repo/docs/specs/g"); !strings.Contains(out, want) {
		t.Errorf("missing %q:\n%s", want, out)
	}
	rep.doneState = nil
	if out := rep.Markdown("docs/specs/g"); !strings.Contains(out, "- completion record: none; run `ha done docs/specs/g` to record completion\n") {
		t.Errorf("the ha done hint changed:\n%s", out)
	}
	rep.CompleteWithoutRecord = false
	if out := rep.Markdown("docs/specs/g"); !strings.Contains(out, "- completion record: none\n") {
		t.Errorf("the none line changed:\n%s", out)
	}
}
