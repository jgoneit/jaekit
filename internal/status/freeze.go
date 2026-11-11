package status

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jgoneit/jaekit/internal/gitx"
	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/reviewgate"
)

// CheckFreeze describes check changes for human review, never a completion
// requirement. Only run-rules/7 standard and strict goals display it.
type CheckFreeze struct {
	Stage           string            `json:"stage"`
	Series          string            `json:"series"`
	Round           string            `json:"round"`
	Head            string            `json:"head"`
	RemovedObserved bool              `json:"removed_observed"`
	TableFrom       *int              `json:"table_from"`
	Files           []FrozenCheckFile `json:"files"`
}

// FrozenCheckFile is a current content change or a removed check path.
type FrozenCheckFile struct {
	Path   string             `json:"path"`
	State  string             `json:"state"`
	Reason *CheckChangeReason `json:"reason"`
}

// CheckChangeReason is a recorded reason still bound to the current blob.
type CheckChangeReason struct {
	Seq    int    `json:"seq"`
	Reason string `json:"reason"`
}

func (e *evaluator) checkFreeze() (*CheckFreeze, error) {
	if e.rules != RulesV7 || e.start == nil {
		return nil, nil
	}
	stage := ""
	switch e.in.Goal.Assurance {
	case "strict":
		stage = "plan"
	case "standard":
		stage = "candidate"
	default:
		return nil, nil
	}
	var gate *record.ReviewData
	for _, l := range e.in.Lines {
		if l.Kind == "note" && l.Note == "review" {
			gate = l.Review
		}
	}
	if reviewgate.Valid(gate) != nil {
		return nil, nil
	}
	var frozen *record.ReviewRound
	frozenIndex := -1
	for i := range gate.Rounds {
		r := &gate.Rounds[i]
		if !r.DryRun && r.Stage == stage {
			frozen = r
			frozenIndex = i
			break
		}
	}
	if frozen == nil {
		return nil, nil
	}
	f := &CheckFreeze{Stage: stage, Series: frozen.Series, Round: frozen.Round, Head: frozen.Head, Files: []FrozenCheckFile{}}
	// The earliest note whose final round is this freeze round owns the
	// table. Matching the gate order, series, round, stage, base and head
	// avoids borrowing another round's table when a head was reviewed again.
	old := map[string]bool{}
	for _, l := range e.in.Lines {
		if l.Kind != "note" || l.Note != "review" || reviewgate.Valid(l.Review) != nil {
			continue
		}
		rounds := l.Review.Rounds
		if len(rounds) != frozenIndex+1 {
			continue
		}
		r := rounds[len(rounds)-1]
		if r.DryRun || r.Sequence != frozen.Sequence || r.Series != frozen.Series || r.Round != frozen.Round || r.Stage != frozen.Stage || r.Base != frozen.Base || r.Head != frozen.Head {
			continue
		}
		if l.CheckPaths == nil {
			break
		}
		f.RemovedObserved = true
		seq := l.Seq
		f.TableFrom = &seq
		for _, paths := range l.CheckPaths {
			for _, path := range paths {
				old[path] = true
			}
		}
		break
	}
	current := map[string]bool{}
	for _, row := range e.in.Bundle.Rows {
		for _, path := range row.CheckPaths {
			current[path] = true
		}
	}
	paths := map[string]bool{}
	for path := range old {
		paths[path] = true
	}
	for path := range current {
		paths[path] = true
	}
	var ordered []string
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		now, err := e.in.Repo.Blob(e.head, path)
		if err != nil {
			return nil, err
		}
		state := "removed"
		if current[path] {
			before, err := e.in.Repo.Blob(frozen.Head, path)
			if err != nil {
				return nil, err
			}
			if before == now {
				continue
			}
			state = "changed"
			if before == gitx.Deleted {
				state = "added"
			}
		}
		f.Files = append(f.Files, FrozenCheckFile{Path: path, State: state, Reason: e.checkChangeReason(path, now)})
	}
	return f, nil
}

func (e *evaluator) checkChangeReason(path, blob string) *CheckChangeReason {
	for i := len(e.in.Lines) - 1; i >= 0; i-- {
		l := &e.in.Lines[i]
		if l.Kind != "note" || l.Note != "check-change" || l.Subject != "checks:"+path || l.CheckChange == nil || strings.TrimSpace(l.Reason) == "" {
			continue
		}
		bound := l.CheckChange.Blob
		if (bound == nil && blob == gitx.Deleted) || (bound != nil && *bound == blob) {
			return &CheckChangeReason{Seq: l.Seq, Reason: l.Reason}
		}
	}
	return nil
}

func (rep *Report) renderCheckFreeze(w func(string, ...any)) {
	f := rep.CheckFreeze
	if f == nil {
		return
	}
	source := ""
	if f.TableFrom != nil {
		source = fmt.Sprintf("; condition table from review note seq %d", *f.TableFrom)
	}
	w("- check freeze: %s round %s/%s at %s%s\n", f.Stage, f.Series, f.Round, shortHash(f.Head), source)
	if !f.RemovedObserved {
		w("  - freeze-time condition table was not recorded\n")
	}
	for _, file := range f.Files {
		reason := "no reason"
		if file.Reason != nil {
			reason = fmt.Sprintf("seq %d: %s", file.Reason.Seq, file.Reason.Reason)
		}
		w("  - %s %s: %s\n", file.Path, file.State, reason)
	}
}
