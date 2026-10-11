package status

import (
	"fmt"
	"strconv"
	"strings"
)

// Markdown keeps recorded claims distinct from historical completion and from
// permission to rework. Quoted strings cannot create rows or headings.
func (r *FindingsReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "### Post-completion findings\n\n- unresolved: %d (candidate %d; confirmed %d; confirmed existing-condition reports %d)\n", r.Unresolved, r.Candidate, r.Confirmed, r.ConfirmedExisting)
	b.WriteString("- Reported claims; these counts do not measure every defect or authorize rework.\n")
	for _, v := range r.Findings {
		fmt.Fprintf(&b, "- %s: %s; revision %d; historical completion seq %d (%s); rework: %s\n", v.ID, v.Change.Status, v.Revision, v.Change.Completion.Seq, shortHash(v.Change.Completion.SHA256), v.ReworkStatus)
		fmt.Fprintf(&b, "  - historical code: %s; spec: %s; mapping: %s; criteria: %s\n", strconv.Quote(v.Commit), strconv.Quote(v.SpecDigest), v.Change.Mapping, quotedFindingList(v.Change.Criteria))
		fmt.Fprintf(&b, "  - source: %s; summary: %s\n", strconv.Quote(v.Change.Source), strconv.Quote(v.Change.Summary))
		if v.Change.Rework != nil {
			fmt.Fprintf(&b, "  - explicit rework request: seq %d (%s)\n", v.Change.Rework.Seq, shortHash(v.Change.Rework.SHA256))
		}
		if v.ReworkCompletion != nil {
			fmt.Fprintf(&b, "  - linked rework completion: seq %d (%s)\n", v.ReworkCompletion.Seq, shortHash(v.ReworkCompletion.SHA256))
		}
		for _, event := range v.History {
			fmt.Fprintf(&b, "  - history seq %d (%s), recorded %s: %s; source %s; reason %s; evidence %s; resolution %s\n", event.Seq, shortHash(event.SHA256), event.At, event.Change.Status, strconv.Quote(event.Change.Source), strconv.Quote(event.Change.Reason), quotedFindingList(event.Change.Evidence), quotedFindingList(event.Change.ResolutionRefs))
		}
	}
	return b.String()
}

func quotedFindingList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
