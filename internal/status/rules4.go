package status

import (
	"sort"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/gitx"
	"github.com/jgoneit/jaekit/internal/record"
)

// run-rules/4 additions (run-record.md §4.7). Each part reads only stored
// lines, the bundle and Git, like the rest of the computation.

// ExecutorRecoveries is how many recover notes the executor may record for one
// condition and input before the user allows more.
const ExecutorRecoveries = 2

// TestAuthority shows where a run-rules/4 goal's test-change authority came
// from: the quoted start request and the executor's reading of it.
type TestAuthority struct {
	RequestQuote   string   `json:"request_quote"`
	Interpretation string   `json:"interpretation"`
	Allowed        []string `json:"allowed"`
}

// TestChange is a test file that existed at base and changed since, with
// whether the executor's reason exempts it from the user's confirmation.
type TestChange struct {
	Path      string     `json:"path"`
	Change    string     `json:"change"` // modified | deleted
	Blob      string     `json:"blob,omitempty"`
	Exempt    bool       `json:"exempt"`
	Confirmed bool       `json:"confirmed"`
	Reason    string     `json:"reason,omitempty"` // why it is not exempt
	Rationale *Rationale `json:"rationale,omitempty"`
}

// Rationale is the last test-change note for a path. It is the executor's
// claim; Core checks only its links and the file versions it names.
type Rationale struct {
	Seq      int         `json:"seq"`
	Claim    string      `json:"claim"`
	Criteria []string    `json:"criteria"`
	How      string      `json:"how"`
	MovedTo  []MovedFile `json:"moved_to,omitempty"`
	Weakened bool        `json:"weakened"`
}

// MovedFile is a file a moved or split test now lives in, at the version the
// reason was recorded for.
type MovedFile struct {
	Path string `json:"path"`
	Blob string `json:"blob"`
}

// Recovery is the error-limit state of one condition input under run-rules/4.
type Recovery struct {
	Criterion    string `json:"criterion"`
	Input        string `json:"input"`
	Errors       int    `json:"errors"`     // fresh errors and unknowns no recovery named
	ErrorSeqs    []int  `json:"error_seqs"` // their records
	LimitReached bool   `json:"limit_reached"`
	Used         int    `json:"used"`
	Allowed      int    `json:"allowed"`
	Remaining    int    `json:"remaining"`
}

type recoveryState struct {
	used, allowed int
	cleared       map[int]bool // the error and unknown records recoveries named
}

// foldRecoveries applies recover notes in record order, each only while a
// recovery remains for its condition and input.
func foldRecoveries(lines []record.Line) map[string]*recoveryState {
	out := map[string]*recoveryState{}
	get := func(d *record.RecoveryData) *recoveryState {
		key := d.Criterion + "\x00" + d.Input
		if out[key] == nil {
			out[key] = &recoveryState{allowed: ExecutorRecoveries, cleared: map[int]bool{}}
		}
		return out[key]
	}
	execution := record.ExecutionLines(lines)
	bySeq := map[int]record.Line{}
	for _, l := range execution {
		bySeq[l.Seq] = l
	}
	// A recover note clears only earlier error or unknown records of its own
	// condition and input, whatever else its list names.
	clearable := func(d *record.RecoveryData, note, seq int) bool {
		named, ok := bySeq[seq]
		return ok && seq < note && named.Kind == d.Input && named.Target != nil && named.Target.Criterion == d.Criterion &&
			(named.Result == "error" || named.Result == "unknown")
	}
	for _, l := range execution {
		if l.Kind != "note" || l.Recovery == nil {
			continue
		}
		switch l.Note {
		case "allow-recovery":
			if l.Recovery.Count > 0 && strings.TrimSpace(l.Quote) != "" {
				get(l.Recovery).allowed += l.Recovery.Count
			}
		case "recover":
			if st := get(l.Recovery); st.used < st.allowed {
				st.used++
				for _, seq := range l.Recovery.Clears {
					if clearable(l.Recovery, l.Seq, seq) {
						st.cleared[seq] = true
					}
				}
			}
		}
	}
	return out
}

func (e *evaluator) recovery(id, input string) *recoveryState {
	if st := e.recoveries[id+"\x00"+input]; st != nil {
		return st
	}
	return &recoveryState{allowed: ExecutorRecoveries, cleared: map[int]bool{}}
}

// unrecovered lists the fresh errors and unknowns of an input that no
// recovery named. An error that becomes fresh again after its input changed
// and came back still counts unless a recovery named it.
func (e *evaluator) unrecovered(id, input string, ls []*record.Line) []int {
	cleared := e.recovery(id, input).cleared
	var seqs []int
	for _, l := range ls {
		if (l.Result == "error" || l.Result == "unknown") && !cleared[l.Seq] {
			seqs = append(seqs, l.Seq)
		}
	}
	return seqs
}

func (e *evaluator) errorsSince(id, input string, ls []*record.Line) int {
	return len(e.unrecovered(id, input, ls))
}

func (e *evaluator) reportRecoveries(rep *Report, id, kind string, C, B []*record.Line) {
	inputs := map[string][]*record.Line{"check": C}
	names := []string{"check"}
	if kind == bundle.KindChange {
		inputs["baseline"] = B
		names = append(names, "baseline")
	}
	for _, input := range names {
		st := e.recovery(id, input)
		seqs := e.unrecovered(id, input, inputs[input])
		errs := len(seqs)
		if st.used == 0 && st.allowed == ExecutorRecoveries && errs <= ErrorReruns {
			continue
		}
		rep.Recoveries = append(rep.Recoveries, Recovery{Criterion: id, Input: input, Errors: errs, ErrorSeqs: seqs,
			LimitReached: errs > ErrorReruns, Used: st.used, Allowed: st.allowed, Remaining: max(0, st.allowed-st.used)})
	}
}

// testChanges classifies base test files by the start record's copy of the
// plan's test paths and exempts a change whose last test-change note is a
// complete, unweakened reason for the file as it is at HEAD, inside the
// recorded authority. It returns the paths still held for the user.
func (e *evaluator) testChanges(rep *Report, changes []gitx.Change) ([]string, error) {
	scope := e.start.Scope
	if scope == nil {
		scope = &record.ScopeCopy{}
	}
	if a := e.start.TestAuthority; a != nil {
		quote := ""
		if e.start.Request != nil {
			quote = e.start.Request.Quote
		}
		rep.TestAuthority = &TestAuthority{RequestQuote: quote, Interpretation: a.Interpretation,
			Allowed: append([]string{}, scope.Allowed...)}
	}
	last := map[string]*record.Line{}
	for i := range e.in.Lines {
		l := &e.in.Lines[i]
		if l.Kind == "note" && l.Note == "test-change" && l.TestChange != nil && strings.HasPrefix(l.Subject, "tests:") {
			last[strings.TrimPrefix(l.Subject, "tests:")] = l
		}
	}
	var held []string
	for _, ch := range changes {
		if ch.Status == "A" || !bundle.MatchAny(scope.Tests, ch.Path) {
			continue
		}
		tc := TestChange{Path: ch.Path, Change: "modified", Confirmed: e.confirms["tests:"+ch.Path]}
		blob, err := e.in.Repo.Blob(e.head, ch.Path)
		if err != nil {
			return nil, err
		}
		if blob == gitx.Deleted {
			tc.Change = "deleted"
		} else {
			tc.Blob = blob
		}
		note := last[ch.Path]
		if note != nil {
			d := note.TestChange
			r := &Rationale{Seq: note.Seq, Claim: "executor", Criteria: append([]string{}, d.Criteria...), How: d.How, Weakened: d.Weakened}
			for _, m := range d.MovedTo {
				r.MovedTo = append(r.MovedTo, MovedFile{Path: m.Path, Blob: m.Blob})
			}
			tc.Rationale = r
		}
		if tc.Reason, err = e.testExemption(scope, tc, note); err != nil {
			return nil, err
		}
		tc.Exempt = tc.Reason == ""
		if !tc.Exempt && !tc.Confirmed {
			held = append(held, ch.Path)
		}
		rep.TestChanges = append(rep.TestChanges, tc)
	}
	return held, nil
}

// testExemption is empty when the change needs no confirmation, and otherwise
// says why it does.
func (e *evaluator) testExemption(scope *record.ScopeCopy, tc TestChange, note *record.Line) (string, error) {
	switch {
	case e.start.TestAuthority == nil:
		return "the goal started without test-change authority", nil
	case !bundle.MatchAny(scope.Allowed, tc.Path):
		return "outside the recorded authority", nil
	case note == nil:
		return "no test-change reason", nil
	case note.TestChange.Weakened:
		return "marked as weakening the check", nil
	case len(note.TestChange.Criteria) == 0 || strings.TrimSpace(note.TestChange.How) == "":
		return "the reason names no condition or check method", nil
	}
	// The reason covers the path as it is now, its content or its deletion,
	// and every file it names as the test's new home.
	now := tc.Blob
	if tc.Change == "deleted" {
		now = gitx.Deleted
	}
	if note.Bound == nil || note.Bound.Blob != now {
		return "the file changed after the reason", nil
	}
	if tc.Change == "deleted" && len(note.TestChange.MovedTo) == 0 {
		return "a deletion whose reason names no new path", nil
	}
	for _, m := range note.TestChange.MovedTo {
		if !bundle.MatchAny(scope.Allowed, m.Path) {
			return "moved outside the recorded authority: " + m.Path, nil
		}
		blob, err := e.in.Repo.Blob(e.head, m.Path)
		if err != nil {
			return "", err
		}
		if blob != m.Blob {
			return "a moved-to file changed after the reason: " + m.Path, nil
		}
	}
	return "", nil
}

// Waits under run-rules/4: a reported wait runs from the second after its
// note to the first that comes of the user's quoted answer, a resume note, or
// the start of a later run; it never crosses its window. Runs stay elapsed
// where they overlap a wait, and overlapping waits count once.
type interval struct{ from, to int64 } // [from, to) in Unix seconds

func unix(at string) (int64, bool) {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return 0, false
	}
	return t.Unix(), true
}

func answers(l record.Line) bool {
	switch l.Kind {
	case "note":
		switch l.Note {
		case "input", "confirm", "allow-recovery":
			return strings.TrimSpace(l.Quote) != ""
		case "resume":
			return true
		}
	case "budget_change":
		return l.BudgetChange != nil && strings.TrimSpace(l.BudgetChange.Quote) != ""
	}
	return false
}

// reportedWait returns the excluded wait seconds of a window's lines, which
// span from its first line to its last.
func reportedWait(window []record.Line) int64 {
	if len(window) == 0 {
		return 0
	}
	begin, ok1 := unix(window[0].At)
	end, ok2 := unix(window[len(window)-1].At)
	if !ok1 || !ok2 || end <= begin {
		return 0
	}
	var waits, runs []interval
	for i, l := range window {
		if l.Kind == "check" || l.Kind == "baseline" {
			from, okFrom := unix(l.StartedAt)
			to, okTo := unix(l.At)
			if okFrom && okTo && to > from {
				runs = append(runs, interval{from, to})
			}
		}
		if l.Kind != "note" || l.Note != "wait" {
			continue
		}
		at, ok := unix(l.At)
		if !ok {
			continue
		}
		stop := end
		for _, next := range window[i+1:] {
			var t int64
			var ok bool
			switch {
			case answers(next):
				t, ok = unix(next.At)
			case next.Kind == "check" || next.Kind == "baseline":
				// A run that starts in the note's own second ends the wait
				// there: the order inside one second is unknown.
				if t, ok = unix(next.StartedAt); ok && t < at {
					ok = false
				}
			}
			if ok && t < stop {
				stop = t
			}
		}
		if from := at + 1; stop > from {
			waits = append(waits, interval{from, stop})
		}
	}
	excluded := subtract(union(waits), union(runs))
	var total int64
	for _, w := range excluded {
		from, to := max(w.from, begin), min(w.to, end)
		if to > from {
			total += to - from
		}
	}
	return total
}

func union(xs []interval) []interval {
	sort.Slice(xs, func(i, j int) bool { return xs[i].from < xs[j].from })
	var out []interval
	for _, x := range xs {
		if n := len(out); n > 0 && x.from <= out[n-1].to {
			out[n-1].to = max(out[n-1].to, x.to)
			continue
		}
		out = append(out, x)
	}
	return out
}

// subtract removes ys from xs; both are sorted and disjoint.
func subtract(xs, ys []interval) []interval {
	var out []interval
	for _, x := range xs {
		parts := []interval{x}
		for _, y := range ys {
			var next []interval
			for _, p := range parts {
				if y.to <= p.from || y.from >= p.to {
					next = append(next, p)
					continue
				}
				if y.from > p.from {
					next = append(next, interval{p.from, y.from})
				}
				if y.to < p.to {
					next = append(next, interval{y.to, p.to})
				}
			}
			parts = next
		}
		out = append(out, parts...)
	}
	return out
}
