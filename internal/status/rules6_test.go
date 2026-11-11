package status

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/gitx"
	"github.com/jgoneit/jaekit/internal/goaldocs"
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
		{"run-rules/8", false, false, false, false, false},
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
	if message := (&UnsupportedRulesError{Rules: "run-rules/8"}).Error(); !strings.Contains(message, `"run-rules/6"`) {
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

func TestRules6DoneJudgmentAcrossFindings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		between []string
		head    int
		judged6 bool
		judged5 bool
	}{
		{"no intervening lines", nil, 2, true, true},
		{"one finding", []string{"finding"}, 2, true, false},
		{"multiple findings", []string{"finding", "finding"}, 2, true, false},
		{"immediately preceding finding", []string{"finding", "finding"}, 4, true, true},
		{"earlier finding is the record head", []string{"finding", "finding"}, 3, true, false},
		{"record-head finding followed by multiple findings", []string{"finding", "finding", "finding"}, 3, true, false},
		{"note after record-head finding", []string{"finding", "input"}, 3, false, false},
		{"note between record-head finding and later finding", []string{"finding", "input", "finding"}, 3, false, false},
		{"check after record-head finding", []string{"finding", "check", "finding"}, 3, false, false},
		{"baseline after record-head finding", []string{"finding", "baseline", "finding"}, 3, false, false},
		{"review after record-head finding", []string{"finding", "review", "finding"}, 3, false, false},
		{"note", []string{"input"}, 2, false, false},
		{"note before finding", []string{"input", "finding"}, 2, false, false},
		{"note after finding", []string{"finding", "input"}, 2, false, false},
		{"check before finding", []string{"check", "finding"}, 2, false, false},
		{"baseline before finding", []string{"baseline", "finding"}, 2, false, false},
		{"review before finding", []string{"review", "finding"}, 2, false, false},
	} {
		for _, rules := range []string{RulesV5, RulesV6} {
			t.Run(tc.name+"/"+rules, func(t *testing.T) {
				lines := []record.Line{
					{Header: record.Header{Kind: "start"}, Rules: rules, Budget: &record.Budget{Runs: 50, ElapsedSeconds: 50}},
					{Header: record.Header{Kind: "note"}, Note: "input"},
				}
				for _, kind := range tc.between {
					l := record.Line{Header: record.Header{Kind: kind}}
					switch kind {
					case "finding":
						l.Schema = record.FindingSchema
					case "check", "baseline":
						l.StartedAt = time.Unix(10, 0).UTC().Format(time.RFC3339)
					default:
						l.Kind, l.Note = "note", kind
					}
					lines = append(lines, l)
				}
				done := len(lines)
				lines = append(lines,
					record.Line{Header: record.Header{Kind: "done"}, Status: Complete, SpecDigest: "spec", Commit: "code"},
					record.Line{Header: record.Header{Kind: "note"}, Note: "input"})
				for i := range lines {
					if lines[i].Schema == "" {
						lines[i].Schema = record.Schema
					}
					lines[i].At = time.Unix(int64(i)*5, 0).UTC().Format(time.RFC3339)
				}
				lines[done].At = time.Unix(30, 0).UTC().Format(time.RFC3339)
				lines[done+1].At = time.Unix(60, 0).UTC().Format(time.RFC3339)
				linkRules6BudgetLines(t, lines, map[int]int{done + 1: tc.head})
				e := &evaluator{rules: rules, clean: true, same: map[string]bool{"code": true}, in: Input{
					Goal: &goaldocs.Goal{Digest: "spec", Assurance: "strict"}, Lines: ComputationLines(lines),
				}}
				judged := tc.judged6
				if rules == RulesV5 {
					judged = tc.judged5
				}
				completion, err := e.completionRecord(&Report{})
				if err != nil || (completion != nil) != judged || completion != nil && *completion != done+1 {
					t.Fatalf("completion = %v, err = %v; want judged %t at seq %d", completion, err, judged, done+1)
				}
				stale := e.stale(&Report{})
				if rules == RulesV6 && !judged {
					if stale != nil {
						t.Fatalf("unjudged done presented as a stale completion: %+v", stale)
					}
				} else if stale == nil || stale.Seq != done+1 ||
					judged && len(stale.Reasons) != 0 || !judged && !reflect.DeepEqual(stale.Reasons, []string{"a record was appended while ha done computed it"}) {
					t.Fatalf("stale = %+v; want judged %t at seq %d", stale, judged, done+1)
				}
				bud, err := RecordBudget(lines)
				if err != nil {
					t.Fatal(err)
				}
				post := int64(0)
				if rules == RulesV6 && judged {
					post = 30
				}
				if bud.ElapsedSeconds != 60-post || bud.Exceeded != (post == 0) ||
					rules == RulesV6 && (bud.PostCompletionSeconds == nil || *bud.PostCompletionSeconds != post) ||
					rules == RulesV5 && bud.PostCompletionSeconds != nil {
					t.Fatalf("budget = %+v; want elapsed %d and post %d", bud, 60-post, post)
				}
				if rules == RulesV6 && judged {
					// Remove only findings appended after the recorded head, which
					// can itself be a finding, and keep every execution timestamp.
					without := append([]record.Line(nil), lines[:tc.head]...)
					without = append(without, lines[done:]...)
					linkRules6BudgetLines(t, without, nil)
					want, err := RecordBudget(without)
					if err != nil || !reflect.DeepEqual(bud, want) {
						t.Fatalf("findings change budget: with %+v; without %+v; err %v", bud, want, err)
					}
					e.in.Lines = ComputationLines(without)
					completion, err := e.completionRecord(&Report{})
					if err != nil || completion == nil || *completion != tc.head+1 {
						t.Fatalf("completion without intervening findings = %v, err = %v; want seq %d", completion, err, tc.head+1)
					}
				}
			})
		}
	}
}

func TestRules6DoneJudgmentRequiresEarlierRecordHead(t *testing.T) {
	for _, tc := range []struct {
		name string
		i    int
		head int
	}{
		{"first line with null head", 0, -1},
		{"first line with own head", 0, 0},
		{"null head after finding", 2, -1},
		{"own head after finding", 2, 2},
		{"later head after finding", 2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []record.Line{
				{Header: record.Header{Schema: record.Schema, Kind: "done"}, Raw: []byte("first done")},
				{Header: record.Header{Schema: record.FindingSchema, Kind: "finding"}, Raw: []byte("finding")},
				{Header: record.Header{Schema: record.Schema, Kind: "done"}, Raw: []byte("second done")},
				{Header: record.Header{Schema: record.Schema, Kind: "note"}, Raw: []byte("later note")},
			}
			if tc.head >= 0 {
				lines[tc.i].RecordHead = lines[tc.head].Hash()
			}
			if judgedCompletion(lines, tc.i) {
				t.Fatal("done without an earlier record head is judged")
			}
		})
	}
}

func TestRules6PostCompletionRequiresJudgedDone(t *testing.T) {
	for _, predecessor := range []string{"note", "finding"} {
		for _, immediateHead := range []bool{false, true} {
			name := predecessor + "/earlier head"
			if immediateHead {
				name = predecessor + "/immediate head"
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
				if immediateHead {
					heads = nil
				}
				linkRules6BudgetLines(t, lines, heads)
				bud, err := RecordBudget(lines)
				if err != nil {
					t.Fatal(err)
				}
				post := int64(0)
				// A finding appended during done does not make it unjudged;
				// an intervening execution line still does.
				judged := immediateHead || predecessor == "finding"
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

// These fixtures compute real reports against a clean synthetic repository.
// No goal or execution record is written: the input is the saved-line view.
func rules6ComputationInput(t *testing.T, rules string) Input {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	repo, err := gitx.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	return Input{Repo: repo, HaVersion: "fixture", Goal: &goaldocs.Goal{
		Dir: "example", Status: "Ready", Digest: "spec",
		Criteria: []goaldocs.Criterion{{ID: "AC-1"}},
	}, Bundle: &bundle.Bundle{HasPlan: true, Rows: []bundle.Row{{ID: "AC-1", Kind: bundle.KindMaintain, Command: "true"}}},
		Lines: []record.Line{{Header: record.Header{Schema: record.Schema, Kind: "start", HaVersion: "fixture",
			At: time.Unix(0, 0).UTC().Format(time.RFC3339)}, Goal: "example", Rules: rules,
			BaseCommit: head, Budget: &record.Budget{Runs: 50, ElapsedSeconds: 10 * 3600}}}}
}

func rules6ComputationLine(in Input, kind, value string, hour int64) record.Line {
	l := record.Line{Header: record.Header{Schema: record.Schema, Kind: kind, HaVersion: in.HaVersion,
		At: time.Unix(hour*3600, 0).UTC().Format(time.RFC3339)}, Commit: in.Lines[0].BaseCommit, SpecDigest: in.Goal.Digest}
	if kind == "done" {
		l.Status = value
	} else {
		l.Note = value
	}
	return l
}

func rules6Compute(t *testing.T, in Input) *Report {
	t.Helper()
	rep, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	last := in.Lines[len(in.Lines)-1]
	wantHead := &Head{Seq: last.Seq, SHA256: last.Hash()}
	if !reflect.DeepEqual(rep.RecordHead, wantHead) || !reflect.DeepEqual(rep.Findings.RecordHead, wantHead) {
		t.Fatalf("full record head lost: status %+v; findings %+v; want %+v", rep.RecordHead, rep.Findings.RecordHead, wantHead)
	}
	if rep.DoneEntry(in.HaVersion).RecordHead != last.Hash() {
		t.Fatal("next done would name the filtered record head")
	}
	return rep
}

func rules6EqualComputation(t *testing.T, got, want *Report) {
	t.Helper()
	// Both output formats intentionally expose full-record append heads.
	// Compare everything else, including the human-only latest-done detail.
	copy := *got
	copy.RecordHead = want.RecordHead
	findings := *got.Findings
	findings.RecordHead = want.Findings.RecordHead
	copy.Findings = &findings
	if a, b := copy.Markdown(copy.Goal), want.Markdown(want.Goal); a != b {
		t.Fatalf("unjudged done changes human output:\n%s\nwant:\n%s", a, b)
	}
	a, err := copy.JSON()
	if err != nil {
		t.Fatal(err)
	}
	b, err := want.JSON()
	if err != nil || string(a) != string(b) {
		t.Fatalf("unjudged done changes JSON:\n%s\nwant:\n%s\nerror: %v", a, b, err)
	}
}

func TestRules6UnjudgedDoneIsAbsentFromComputation(t *testing.T) {
	t.Run("latest incomplete done and stale explanation", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Lines = append(in.Lines, rules6ComputationLine(in, "done", Complete, 1),
			rules6ComputationLine(in, "done", Incomplete, 2), rules6ComputationLine(in, "done", Complete, 12))
		in.Lines[1].SpecDigest = "earlier-spec"
		in.Lines[3].HaVersion = "unjudged-version"
		linkRules6BudgetLines(t, in.Lines, map[int]int{4: 2})
		got := rules6Compute(t, in)
		without := in
		without.Lines = in.Lines[:3]
		want := rules6Compute(t, without)
		rules6EqualComputation(t, got, want)
		if d := got.Done(); d == nil || d.LatestSeq != 3 || d.LatestStatus != Incomplete || d.Completion == nil || d.Completion.Seq != 2 {
			t.Fatalf("latest judged incomplete and completion explanation lost: %+v", d)
		}
		if !strings.Contains(got.Markdown(got.Goal), "recorded incomplete, not a completion") {
			t.Fatal("human output hides the earlier incomplete done")
		}
	})
	t.Run("late unjudged completion cannot exhaust or close budget", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Lines = append(in.Lines, rules6ComputationLine(in, "note", "input", 2), rules6ComputationLine(in, "done", Complete, 12))
		linkRules6BudgetLines(t, in.Lines, map[int]int{3: 1})
		got := rules6Compute(t, in)
		without := in
		without.Lines = in.Lines[:2]
		want := rules6Compute(t, without)
		rules6EqualComputation(t, got, want)
		if got.Status != Incomplete || got.Budget.ElapsedSeconds != 2*3600 || got.Budget.Exceeded || got.Done() != nil {
			t.Fatalf("unjudged done changes status or budget: %+v", got)
		}
		state, err := FoldBudget(in.Lines)
		baseline, baseErr := FoldBudget(without.Lines)
		if err != nil || baseErr != nil || state.Completed || !reflect.DeepEqual(state, baseline) {
			t.Fatalf("budget fold: got %+v (%v); want %+v (%v)", state, err, baseline, baseErr)
		}
		req := BudgetChangeRequest{WindowStart: 1, Revision: 1, PreviousRuns: 50, Runs: 40, Reason: "reduce limit"}
		change, err := NewBudgetChange(in.Lines, req)
		baseChange, baseErr := NewBudgetChange(without.Lines, req)
		if err != nil || baseErr != nil || !reflect.DeepEqual(change, baseChange) {
			t.Fatalf("budget change: got %+v (%v); want %+v (%v)", change, err, baseChange, baseErr)
		}
		stored := append(append([]record.Line(nil), in.Lines...), record.Line{
			Header: record.Header{Kind: "budget_change", Seq: 4, At: in.Lines[2].At}, BudgetChange: &change.Change})
		if state, err := FoldBudget(stored); err != nil || state.Completed || state.RunsLimit != 40 {
			t.Fatalf("accepted budget change cannot be folded: %+v, %v", state, err)
		}
	})
	t.Run("judged completion can name unjudged predecessor", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Goal.Criteria, in.Bundle.Rows = nil, nil
		in.Lines = append(in.Lines, rules6ComputationLine(in, "note", "input", 1),
			rules6ComputationLine(in, "done", Complete, 2), rules6ComputationLine(in, "done", Complete, 3),
			rules6ComputationLine(in, "note", "input", 8))
		linkRules6BudgetLines(t, in.Lines, map[int]int{3: 1})
		got := rules6Compute(t, in)
		if got.Status != Complete || got.CompletionRecord == nil || *got.CompletionRecord != 4 ||
			got.Budget.ElapsedSeconds != 3*3600 || *got.Budget.PostCompletionSeconds != 5*3600 {
			t.Fatalf("later judged done was re-judged on the filtered record: %+v", got)
		}
		state, err := FoldBudget(in.Lines)
		if err != nil || !state.Completed {
			t.Fatalf("judged completion does not close budget: %+v, %v", state, err)
		}
	})
	t.Run("unjudged done does not extend reported waits", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Lines = append(in.Lines, rules6ComputationLine(in, "note", "wait", 1),
			rules6ComputationLine(in, "note", "input", 2), rules6ComputationLine(in, "done", Incomplete, 12))
		linkRules6BudgetLines(t, in.Lines, map[int]int{4: 2})
		without := in
		without.Lines = in.Lines[:3]
		rules6EqualComputation(t, rules6Compute(t, in), rules6Compute(t, without))
		if got, want := WaitSpans(in.Lines), WaitSpans(without.Lines); !reflect.DeepEqual(got, want) {
			t.Fatalf("cost wait spans changed: got %+v; want %+v", got, want)
		}
		if got := ReportedWaitSeconds(in.Lines); got != 3599 || got != ReportedWaitSeconds(without.Lines) {
			t.Fatalf("cost reported waits = %d; want 3599 (the wait starts after its own second)", got)
		}
	})
	t.Run("unjudged non-complete done cannot end interval", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Goal.Criteria, in.Bundle.Rows = nil, nil
		in.Lines = append(in.Lines, rules6ComputationLine(in, "done", Complete, 2),
			rules6ComputationLine(in, "note", "input", 3), rules6ComputationLine(in, "done", Incomplete, 4),
			rules6ComputationLine(in, "note", "input", 8))
		linkRules6BudgetLines(t, in.Lines, map[int]int{4: 2})
		got := rules6Compute(t, in)
		without := in
		without.Lines = append(append([]record.Line(nil), in.Lines[:3]...), in.Lines[4:]...)
		want := rules6Compute(t, without)
		rules6EqualComputation(t, got, want)
		if got.Budget.ElapsedSeconds != 2*3600 || *got.Budget.PostCompletionSeconds != 6*3600 {
			t.Fatalf("unjudged incomplete done ended interval: %+v", got.Budget)
		}
	})
	t.Run("rules5 retains unjudged done behavior", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV5)
		in.Lines = append(in.Lines, rules6ComputationLine(in, "note", "input", 2), rules6ComputationLine(in, "done", Complete, 12))
		linkRules6BudgetLines(t, in.Lines, map[int]int{3: 1})
		if !reflect.DeepEqual(ComputationLines(in.Lines), in.Lines) {
			t.Fatal("rules5 record view changed")
		}
		got := rules6Compute(t, in)
		if got.Status != BudgetExhausted || got.Budget.ElapsedSeconds != 12*3600 || got.Budget.PostCompletionSeconds != nil ||
			got.Done() == nil || got.Done().LatestSeq != 3 || got.CompletionRecord != nil {
			t.Fatalf("rules5 behavior changed: %+v", got)
		}
		state, err := FoldBudget(in.Lines)
		if err != nil || !state.Completed {
			t.Fatalf("rules5 completion fold changed: %+v, %v", state, err)
		}
		if _, err := NewBudgetChange(in.Lines, BudgetChangeRequest{WindowStart: 1, Revision: 1, PreviousRuns: 50, Runs: 40, Reason: "reduce limit"}); err == nil {
			t.Fatal("rules5 accepted budget change after complete done")
		}
	})
	t.Run("unjudged done cannot hide invalid record data", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Lines = append(in.Lines, rules6ComputationLine(in, "note", "input", 2), rules6ComputationLine(in, "done", Complete, 12))
		in.Lines[2].BudgetChange = &record.BudgetChangeData{}
		linkRules6BudgetLines(t, in.Lines, map[int]int{3: 1})
		got := rules6Compute(t, in)
		for _, reason := range got.Reasons {
			if reason.Code == "record_invalid" && strings.Contains(reason.Detail, "budget data on a done record") {
				return
			}
		}
		t.Fatalf("invalid record data was hidden: %+v", got.Reasons)
	})
	t.Run("unjudged done cannot hide malformed last timestamp", func(t *testing.T) {
		in := rules6ComputationInput(t, RulesV6)
		in.Lines = append(in.Lines, rules6ComputationLine(in, "note", "input", 2), rules6ComputationLine(in, "done", Complete, 12))
		in.Lines[2].At = "invalid time"
		linkRules6BudgetLines(t, in.Lines, map[int]int{3: 1})
		got := rules6Compute(t, in)
		for _, reason := range got.Reasons {
			if reason.Code == "record_invalid" && strings.Contains(reason.Detail, "bad last record time") {
				return
			}
		}
		t.Fatalf("invalid last timestamp was hidden: %+v", got.Reasons)
	})
}
