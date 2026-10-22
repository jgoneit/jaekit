package status

import (
	"fmt"
	"strings"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/reviewgate"
)

// Review reasons under run-rules/5 (run-record.md §4.8). A goal whose SPEC
// now asks for independent review (`Assurance: standard` or `strict`)
// completes only when the last recorded review gate is complete, reviewed
// HEAD through its last code round, has no open defect or
// requirement-question, and its last code round (for strict also its last
// plan round) received every current goal document. The gate is the
// executor's claim; the judgment reads only the stored summary.

// reviewCodes are the review reasons, in the order they are reported.
var reviewCodes = []string{"review_missing", "review_blocked", "review_degraded", "review_open_defect",
	"review_open_question", "review_code_missing", "review_head_stale", "review_docs_stale", "review_plan_missing"}

func isReviewReason(code string) bool {
	for _, c := range reviewCodes {
		if c == code {
			return true
		}
	}
	return false
}

// checkReviews marks review notes whose stored summary is not one a gate
// file could give as invalid record data.
func (e *evaluator) checkReviews() {
	if !Reviewed(e.rules) {
		return
	}
	for _, l := range e.in.Lines {
		if l.Kind != "note" || l.Note != "review" {
			continue
		}
		if err := reviewgate.Valid(l.Review); err != nil {
			e.invalid = append(e.invalid, fmt.Sprintf("record seq %d: review note: %v", l.Seq, err))
		}
	}
}

// reviewReasons judges the last recorded gate of a run-rules/5 goal whose
// current tier asks for review.
func (e *evaluator) reviewReasons() []Reason {
	tier := e.in.Goal.Assurance
	if !Reviewed(e.rules) || e.start == nil || (tier != "standard" && tier != "strict") {
		return nil
	}
	var note *record.Line
	for i := range e.in.Lines {
		if l := &e.in.Lines[i]; l.Kind == "note" && l.Note == "review" {
			note = l
		}
	}
	var rs []Reason
	add := func(code, detail string) {
		rs = append(rs, Reason{Code: code, Class: classOf[code], Detail: detail})
	}
	if note == nil {
		add("review_missing", fmt.Sprintf("Assurance: %s needs a recorded review gate; record one with ha note <goal> review --gate <file>", tier))
		return rs
	}
	g := note.Review
	if reviewgate.Valid(g) != nil {
		return nil // record_invalid already reports it
	}
	at := fmt.Sprintf("the review gate recorded at seq %d", note.Seq)
	switch g.State {
	case "blocked":
		add("review_blocked", at+" is blocked; its reasons are in the gate file")
	case "degraded":
		add("review_degraded", at+" is degraded; its reasons are in the gate file")
	}
	if g.Open.Defect > 0 {
		add("review_open_defect", fmt.Sprintf("%s has %d open defect finding(s)", at, g.Open.Defect))
	}
	if g.Open.RequirementQuestion > 0 {
		add("review_open_question", fmt.Sprintf("%s has %d open requirement-question finding(s)", at, g.Open.RequirementQuestion))
	}
	code := lastRound(g, "candidate", "fixes")
	if code == nil {
		add("review_code_missing", at+" has no candidate or fixes round")
	} else if g.LastReviewedHead != e.head || code.Head != e.head {
		add("review_head_stale", fmt.Sprintf("%s reviewed %s (last code round %s); HEAD is %s", at, short(g.LastReviewedHead), short(code.Head), short(e.head)))
	}
	plan := lastRound(g, "plan", "plan-delta")
	var stale []string
	for _, r := range []*record.ReviewRound{code, plan} {
		if r == nil || (r == plan && tier != "strict") {
			continue
		}
		if missing := e.uncovered(r); len(missing) > 0 {
			stale = append(stale, fmt.Sprintf("%s round %d did not review %s as it is now", r.Stage, r.Sequence, strings.Join(missing, ", ")))
		}
	}
	if len(stale) > 0 {
		add("review_docs_stale", at+": "+strings.Join(stale, "; "))
	}
	if tier == "strict" && plan == nil {
		add("review_plan_missing", at+" has no plan or plan-delta round; Assurance: strict needs one")
	}
	return rs
}

// lastRound is the last round of the given stages that is not a dry run, in
// the gate's order.
func lastRound(g *record.ReviewData, stages ...string) *record.ReviewRound {
	var last *record.ReviewRound
	for i := range g.Rounds {
		r := &g.Rounds[i]
		if r.DryRun {
			continue
		}
		for _, s := range stages {
			if r.Stage == s {
				last = r
			}
		}
	}
	return last
}

// uncovered names the current goal documents a round did not receive as
// they are now: a criteria document of the same content sha256 named by the
// document's repository path or by the name its symbolic links resolve to.
// Other criteria documents and their order do not matter.
func (e *evaluator) uncovered(r *record.ReviewRound) []string {
	got := map[record.ReviewDocument]bool{}
	for _, c := range r.Criteria {
		got[c] = true
	}
	var missing []string
	for _, p := range e.in.Goal.Docs {
		sum := e.in.Goal.DocSHA256[p]
		if !got[record.ReviewDocument{Name: p, SHA256: sum}] && !got[record.ReviewDocument{Name: e.in.Goal.DocResolved[p], SHA256: sum}] {
			missing = append(missing, p)
		}
	}
	return missing
}
