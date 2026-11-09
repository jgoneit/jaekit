package status

import (
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/record"
)

func TestRules6Inheritance(t *testing.T) {
	for _, tc := range []struct {
		rules                                                  string
		supported, structured, autonomous, reviewed, integrity bool
	}{
		{RulesV1, true, false, false, false, false},
		{RulesV2, true, false, false, false, false},
		{RulesV3, true, true, false, false, false},
		{RulesV4, true, true, true, false, false},
		{RulesV5, true, true, true, true, false},
		{RulesV6, true, true, true, true, true},
		{"run-rules/7", false, false, false, false, false},
		{"", false, false, false, false, false},
	} {
		t.Run(tc.rules, func(t *testing.T) {
			got := [5]bool{Supported(tc.rules), Structured(tc.rules), Autonomous(tc.rules), Reviewed(tc.rules), CompletionIntegrity(tc.rules)}
			want := [5]bool{tc.supported, tc.structured, tc.autonomous, tc.reviewed, tc.integrity}
			if got != want {
				t.Errorf("supported, structured, autonomous, reviewed, completion integrity = %v; want %v", got, want)
			}
		})
	}
	if Rules != RulesV3 {
		t.Errorf("default rules = %s; want %s", Rules, RulesV3)
	}
	if message := (&UnsupportedRulesError{Rules: "run-rules/7"}).Error(); !strings.Contains(message, `"run-rules/6"`) {
		t.Errorf("unsupported-rule diagnostic omits run-rules/6: %s", message)
	}
}

func TestRules6PostCompletionWindows(t *testing.T) {
	line := func(kind string, at int64, value string, started int64) record.Line {
		l := record.Line{Header: record.Header{Schema: record.Schema, Kind: kind, At: time.Unix(at, 0).UTC().Format(time.RFC3339)}}
		switch kind {
		case "done":
			l.Status = value
		case "note":
			l.Note, l.Quote = value, "synthetic user request"
		case "check", "baseline":
			l.StartedAt = time.Unix(started, 0).UTC().Format(time.RFC3339)
		case "finding":
			l.Schema = record.FindingSchema
		}
		return l
	}
	for _, tc := range []struct {
		name                string
		lines               []record.Line
		span, post, elapsed int64
		runs, window        int
	}{
		{"notes and repeated completions", []record.Line{
			line("done", 10, Complete, 0), line("done", 20, Complete, 0),
			line("note", 30, "input", 0), line("note", 40, "resume", 0),
			line("note", 50, "review", 0), line("finding", 70, "", 0),
		}, 50, 40, 10, 0, 1},
		{"baseline ends interval", []record.Line{
			line("done", 10, Complete, 0), line("baseline", 50, "", 40), line("note", 60, "input", 0),
		}, 60, 30, 30, 1, 1},
		{"two completion intervals", []record.Line{
			line("done", 10, Complete, 0), line("check", 30, "", 20),
			line("done", 40, Complete, 0), line("note", 60, "input", 0),
		}, 60, 30, 30, 1, 1},
		{"run starts in completion second", []record.Line{
			line("done", 10, Complete, 0), line("check", 20, "", 10),
		}, 20, 0, 20, 1, 1},
		{"run crosses completion", []record.Line{
			line("done", 10, Complete, 0), line("check", 20, "", 5), line("note", 30, "input", 0),
		}, 30, 0, 30, 1, 1},
		{"run starts before intervening done", []record.Line{
			line("done", 10, Complete, 0), line("done", 30, Incomplete, 0),
			line("check", 40, "", 20), line("note", 50, "input", 0),
		}, 50, 10, 40, 1, 1},
		{"reopen leaves previous interval behind", []record.Line{
			line("done", 10, Complete, 0), line("note", 40, "reopen", 0), line("note", 60, "input", 0),
		}, 20, 0, 20, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := append([]record.Line{{Header: record.Header{Schema: record.Schema, Kind: "start", Seq: 1,
				At: time.Unix(0, 0).UTC().Format(time.RFC3339)}, Rules: RulesV6,
				Budget: &record.Budget{Runs: 50, ElapsedSeconds: 1000}}}, tc.lines...)
			for i := range lines {
				lines[i].Seq = i + 1
			}
			bud, err := RecordBudget(lines)
			if err != nil {
				t.Fatal(err)
			}
			if bud.PostCompletionSeconds == nil || *bud.PostCompletionSeconds != tc.post ||
				bud.SpanSeconds == nil || *bud.SpanSeconds != tc.span || bud.ElapsedSeconds != tc.elapsed ||
				bud.Runs != tc.runs || bud.WindowStart != tc.window {
				t.Fatalf("budget = %+v; want span %d, post %d, elapsed %d, runs %d, window %d", bud, tc.span, tc.post, tc.elapsed, tc.runs, tc.window)
			}
			for _, rules := range []string{RulesV1, RulesV2, RulesV3, RulesV4, RulesV5} {
				lines[0].Rules = rules
				old, err := RecordBudget(lines)
				if err != nil {
					t.Fatal(err)
				}
				if old.PostCompletionSeconds != nil || old.ElapsedSeconds != tc.span || old.Runs != tc.runs || old.WindowStart != tc.window {
					t.Errorf("%s budget changed: %+v", rules, old)
				}
			}
		})
	}
}

// Bundle loading and status must agree on the inherited digest semantics:
// changing a result contract or command limit makes earlier evidence stale.
func TestRules6CommandDigestInheritance(t *testing.T) {
	row := bundle.Row{Kind: bundle.KindChange, Command: "go test ./...",
		EvidencePath: "checks/result.json", EvidenceDigest: "first-contract", Limit: time.Minute}
	initial := row.CommandDigestFor(RulesV6)
	if want := row.CommandDigestFor(RulesV5); initial != want {
		t.Fatalf("run-rules/6 command digest = %s; run-rules/5 = %s", initial, want)
	}
	for _, tc := range []struct {
		name   string
		change func(*bundle.Row)
	}{
		{"contract content", func(r *bundle.Row) { r.EvidenceDigest = "second-contract" }},
		{"contract path", func(r *bundle.Row) { r.EvidencePath = "checks/other.json" }},
		{"command limit", func(r *bundle.Row) { r.Limit = 2 * time.Minute }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := row
			tc.change(&changed)
			got := changed.CommandDigestFor(RulesV6)
			if got == initial || got != changed.CommandDigestFor(RulesV5) {
				t.Errorf("changed %s must invalidate evidence under both rule versions", tc.name)
			}
		})
	}
}
