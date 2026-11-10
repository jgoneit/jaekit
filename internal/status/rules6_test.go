package status

import (
	"encoding/json"
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
		}, 30, 10, 20, 1, 1},
		{"crossing run preserves later endpoint", []record.Line{
			line("done", 10, Complete, 0), line("check", 20, "", 5),
			line("check", 50, "", 40), line("note", 60, "input", 0),
		}, 60, 20, 40, 2, 1},
		{"crossing baseline preserves later endpoint", []record.Line{
			line("done", 10, Complete, 0), line("baseline", 20, "", 5),
			line("check", 50, "", 40), line("note", 60, "input", 0),
		}, 60, 20, 40, 2, 1},
		{"crossing run preserves incomplete endpoint", []record.Line{
			line("done", 10, Complete, 0), line("baseline", 20, "", 5),
			line("done", 40, Incomplete, 0), line("note", 60, "input", 0),
		}, 60, 20, 40, 1, 1},
		{"overlapping runs count once", []record.Line{
			line("done", 10, Complete, 0), line("check", 20, "", 5),
			line("baseline", 25, "", 2), line("check", 50, "", 40),
		}, 50, 15, 35, 3, 1},
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
			linkRules6BudgetLines(t, lines, nil)
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
				linkRules6BudgetLines(t, lines, nil)
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

// Give every line distinct bytes and a linked predecessor so done judgments
// exercise the original chain, including finding lines. heads overrides the
// judged sequence for individual done lines to model concurrent appends.
func linkRules6BudgetLines(t *testing.T, lines []record.Line, heads map[int]int) {
	t.Helper()
	for i := range lines {
		lines[i].Seq = i + 1
		if i > 0 {
			prev := lines[i-1].Hash()
			lines[i].Prev = &prev
			if lines[i].Kind == "done" {
				head := i
				if seq, ok := heads[i+1]; ok {
					head = seq
				}
				lines[i].RecordHead = lines[head-1].Hash()
			}
		}
		raw, err := json.Marshal(lines[i])
		if err != nil {
			t.Fatal(err)
		}
		lines[i].Raw = raw
	}
}

func TestRules6PostCompletionRequiresJudgedDone(t *testing.T) {
	for _, predecessor := range []string{"note", "finding"} {
		for _, judged := range []bool{false, true} {
			name := predecessor + "/unjudged"
			if judged {
				name = predecessor + "/judged"
			}
			t.Run(name, func(t *testing.T) {
				lines := []record.Line{
					{Header: record.Header{Kind: "start"}, Rules: RulesV6, Budget: &record.Budget{Runs: 50, ElapsedSeconds: 35}},
					{Header: record.Header{Kind: predecessor}, Note: "input"},
					{Header: record.Header{Kind: "done"}, Status: Complete},
					{Header: record.Header{Kind: "note"}, Note: "input"},
				}
				for i := range lines {
					lines[i].Schema = record.Schema
					lines[i].At = time.Unix(int64(i)*10, 0).UTC().Format(time.RFC3339)
				}
				if predecessor == "finding" {
					lines[1].Schema = record.FindingSchema
				}
				lines[3].At = time.Unix(40, 0).UTC().Format(time.RFC3339)
				heads := map[int]int{3: 1}
				if judged {
					heads = nil
				}
				linkRules6BudgetLines(t, lines, heads)
				bud, err := RecordBudget(lines)
				if err != nil {
					t.Fatal(err)
				}
				post := int64(0)
				if judged {
					post = 20
				}
				if bud.PostCompletionSeconds == nil || *bud.PostCompletionSeconds != post ||
					bud.ElapsedSeconds != 40-post || bud.Exceeded == judged {
					t.Fatalf("budget = %+v; want post %d, elapsed %d, exceeded %t", bud, post, 40-post, !judged)
				}
				if !judged {
					without := append(append([]record.Line(nil), lines[:2]...), lines[3:]...)
					linkRules6BudgetLines(t, without, nil)
					want, err := RecordBudget(without)
					if err != nil {
						t.Fatal(err)
					}
					if bud.ElapsedSeconds != want.ElapsedSeconds || bud.Exceeded != want.Exceeded ||
						*bud.PostCompletionSeconds != *want.PostCompletionSeconds {
						t.Fatalf("unjudged done changes budget: with %+v; without %+v", bud, want)
					}
				}
				for _, rules := range []string{RulesV1, RulesV2, RulesV3, RulesV4, RulesV5} {
					lines[0].Rules = rules
					linkRules6BudgetLines(t, lines, heads)
					old, err := RecordBudget(lines)
					if err != nil {
						t.Fatal(err)
					}
					if old.PostCompletionSeconds != nil || old.ElapsedSeconds != 40 || !old.Exceeded {
						t.Errorf("%s budget changed: %+v", rules, old)
					}
				}
			})
		}
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
