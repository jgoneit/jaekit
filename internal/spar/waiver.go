package spar

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// waiversFile is the round file that holds the user's decisions to go on
// without a required reviewer.
const waiversFile = "waivers.jsonl"

// Waiver is one line of a round's waivers.jsonl: the user's decision to go
// on without a required reviewer in that round.
type Waiver struct {
	Round      string `json:"round"`
	Reviewer   string `json:"reviewer"`
	Preset     string `json:"preset"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	UserQuote  string `json:"user_quote"`
	RecordedAt string `json:"recorded_at"`
}

// WaiveOptions are the inputs of `spar waive`.
type WaiveOptions struct {
	Round     string
	Reviewer  string
	UserQuote string
	Log       io.Writer
}

// Waive records the user's words letting a round go on without one of its
// required reviewers. Only a reviewer that did not complete the round, or a
// rerun of its packet, can be waived, and only with the user's words.
func Waive(o WaiveOptions) error {
	if strings.TrimSpace(o.UserQuote) == "" {
		return refused("a waiver needs --user-quote with the user's words")
	}
	dir := realish(o.Round)
	unlock, err := lockSeries(filepath.Dir(dir), o.Log)
	if err != nil {
		return refused("%s cannot be locked: %v", o.Round, err)
	}
	defer unlock()
	r, err := loadRound(dir)
	if err != nil {
		return refused("%s cannot be read as a round: %v", o.Round, err)
	}
	if r.DryRun {
		return refused("%s is a dry run; it reviewed nothing", o.Round)
	}
	if waiversFileTaken(r) {
		return refused("%s has a reviewer named %s, whose folder takes the waiver file's place", o.Round, waiversFile)
	}
	var refs []RequiredRef
	for _, ref := range requiredOf(r) {
		if ref.Name == o.Reviewer {
			refs = append(refs, ref)
		}
	}
	switch len(refs) {
	case 0:
		return refused("%s is not a required reviewer of %s", o.Reviewer, o.Round)
	case 1:
	default:
		return refused("%s names more than one required reviewer of %s", o.Reviewer, o.Round)
	}
	ref := refs[0]
	// A reviewer completed the round when it completed its packet in any
	// round of the series with the same stage, base, head and packet, as
	// the gate counts it.
	siblings, err := seriesRounds(filepath.Dir(dir))
	if err != nil {
		return refused("the rounds of the series of %s cannot be read: %v", o.Round, err)
	}
	if completedAs(r.Reviewers[ref.Name], ref) {
		return refused("%s completed %s", ref.Name, o.Round)
	}
	for _, s := range siblings {
		if isRerun(s.round, r) && completedAs(s.round.Reviewers[ref.Name], ref) {
			return refused("%s completed the same packet in %s", ref.Name, filepath.Base(s.dir))
		}
	}
	waivers, err := loadWaivers(dir)
	if err != nil {
		return refused("%v", err)
	}
	if waiverFor(waivers, ref) != nil {
		return refused("%s already holds a waiver for %s", o.Round, ref.Name)
	}
	status, reason := waivedStatus(r, ref)
	return appendJSONLine(filepath.Join(dir, waiversFile), Waiver{Round: filepath.Base(dir), Reviewer: ref.Name,
		Preset: ref.Preset, Status: status, Reason: reason, UserQuote: o.UserQuote, RecordedAt: now()})
}

// completedAs reports a reviewer state that completes a required reviewer:
// completed, as a required reviewer of the same preset.
func completedAs(st *ReviewerState, ref RequiredRef) bool {
	return st != nil && st.Status == StatusCompleted && st.Required && st.Preset == ref.Preset
}

// waivedStatus is the status and cause the round gives a required
// reviewer; a reviewer the round did not run under that preset is not_run.
func waivedStatus(r *RoundRecord, ref RequiredRef) (string, string) {
	st := r.Reviewers[ref.Name]
	if st == nil || st.Preset != ref.Preset {
		return StatusNotRun, ""
	}
	return st.Status, st.Reason
}

// loadWaivers reads a round's waivers. A round without the file has none,
// as every round recorded before waivers existed; a line that is not a
// waiver with a reviewer, a preset and the user's words makes the file
// unreadable.
func loadWaivers(dir string) ([]Waiver, error) {
	waivers, err := readJSONLines[Waiver](filepath.Join(dir, waiversFile))
	if err != nil {
		return nil, fmt.Errorf("%s/%s cannot be read: %v", dir, waiversFile, err)
	}
	for i, w := range waivers {
		if w.Reviewer == "" || w.Preset == "" || strings.TrimSpace(w.UserQuote) == "" {
			return nil, fmt.Errorf("%s/%s: entry %d is not a waiver with a reviewer, a preset and the user's words", dir, waiversFile, i+1)
		}
	}
	return waivers, nil
}

// waiversFileTaken reports a round recorded before waivers existed whose
// reviewer was named waivers.jsonl: that path is the reviewer's folder,
// and the round holds no waivers.
func waiversFileTaken(r *RoundRecord) bool {
	for name := range r.Reviewers {
		if strings.EqualFold(name, waiversFile) {
			return true
		}
	}
	return false
}

// waiverFor finds the waiver of a required reviewer: same name and preset.
func waiverFor(waivers []Waiver, ref RequiredRef) *Waiver {
	for i := range waivers {
		if waivers[i].Reviewer == ref.Name && waivers[i].Preset == ref.Preset {
			return &waivers[i]
		}
	}
	return nil
}
