package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
)

type FindingEvent struct {
	Seq    int                  `json:"seq"`
	At     string               `json:"at"`
	SHA256 string               `json:"sha256"`
	Change record.FindingChange `json:"change"`
}

type FindingView struct {
	ID               string               `json:"id"`
	Revision         int                  `json:"revision"`
	Commit           string               `json:"commit"`
	SpecDigest       string               `json:"spec_digest"`
	Change           record.FindingChange `json:"change"`
	History          []FindingEvent       `json:"history"`
	ReworkStatus     string               `json:"rework_status"`
	ReworkCompletion *record.RecordRef    `json:"rework_completion"`
}

type FindingsReport struct {
	Schema            string        `json:"schema"`
	Goal              string        `json:"goal"`
	RecordHead        *Head         `json:"record_head"`
	Candidate         int           `json:"candidate"`
	Confirmed         int           `json:"confirmed"`
	ConfirmedExisting int           `json:"confirmed_existing"`
	Unresolved        int           `json:"unresolved"`
	Findings          []FindingView `json:"findings"`
}

var findingID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}$`)

func findingError(format string, args ...any) error { return fmt.Errorf("finding: "+format, args...) }

func findingRef(lines []record.Line, ref record.RecordRef) (*record.Line, error) {
	if ref.Seq < 1 || ref.Seq > len(lines) {
		return nil, findingError("record reference does not exist")
	}
	line := &lines[ref.Seq-1]
	if line.Seq != ref.Seq || ref.SHA256 != line.Hash() {
		return nil, findingError("record reference hash mismatch")
	}
	return line, nil
}

func findingTime(value string) bool {
	if value == "" {
		return true
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func findingList(values []string) bool {
	if len(values) > 256 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range values {
		if strings.TrimSpace(s) == "" || len(s) > 8192 || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

func validateFinding(lines []record.Line, c record.FindingChange, old *FindingView, views map[string]*FindingView) (*record.Line, error) {
	if !findingID.MatchString(c.ID) || !findingID.MatchString(c.RequestID) || c.Revision < 0 {
		return nil, findingError("invalid id, request_id or revision")
	}
	if strings.TrimSpace(c.Summary) == "" || strings.TrimSpace(c.Source) == "" || strings.TrimSpace(c.Category) == "" {
		return nil, findingError("summary, source and category are required claims")
	}
	// Required lists are arrays. A stored null or omitted list is reported,
	// never read as an empty one.
	if c.Criteria == nil || c.Evidence == nil {
		return nil, findingError("criteria and evidence must be arrays")
	}
	if !findingTime(c.EventAt) || !findingList(c.Criteria) || !findingList(c.Evidence) || !findingList(c.ResolutionRefs) {
		return nil, findingError("invalid time or reference list")
	}
	if c.Document != nil && (strings.TrimSpace(c.Document.Ref) == "" || !findingTime(c.Document.RecordedAt) || !findingTime(c.Document.EventAt)) {
		return nil, findingError("invalid document reference or claimed times")
	}
	d, err := findingRef(lines, c.Completion)
	if err != nil {
		return nil, err
	}
	if d.Kind != "done" || d.Status != Complete {
		return nil, findingError("reference is not a historical completion")
	}
	if old == nil {
		if c.Revision != 0 || c.Status != "candidate" {
			return nil, findingError("new findings must be candidates at revision zero")
		}
	} else {
		if c.Revision != old.Revision {
			return nil, findingError("revision conflict: current revision is %d", old.Revision)
		}
		if c.Completion != old.Change.Completion {
			return nil, findingError("the completion anchor cannot change")
		}
		if strings.TrimSpace(c.Reason) == "" {
			return nil, findingError("a revision needs its reason")
		}
	}
	switch c.Mapping {
	case "linked":
		if len(c.Criteria) == 0 {
			return nil, findingError("linked mapping needs historical criteria")
		}
		for _, id := range c.Criteria {
			found := false
			for _, criterion := range d.Criteria {
				if criterion.ID == id {
					found = true
				}
			}
			if !found {
				return nil, findingError("criterion %s did not belong to the target completion", id)
			}
		}
	case "unmapped", "outside":
		if len(c.Criteria) != 0 {
			return nil, findingError("unmapped/outside findings cannot assert linked criteria")
		}
	default:
		return nil, findingError("unsupported mapping")
	}
	switch c.Status {
	case "candidate":
	case "confirmed", "duplicate", "dismissed", "resolved":
		if len(c.Evidence) == 0 {
			return nil, findingError("a judgment needs explicit evidence references")
		}
	default:
		return nil, findingError("unsupported finding status")
	}
	if c.Status == "resolved" && len(c.ResolutionRefs) == 0 {
		return nil, findingError("resolution needs explicit follow-up references")
	}
	// Earlier resolution references stay in history; the current revision
	// carries them only while it is resolved.
	if c.Status != "resolved" && len(c.ResolutionRefs) != 0 {
		return nil, findingError("resolution_refs require resolved status")
	}
	if c.Status == "duplicate" {
		if c.DuplicateOf == "" {
			return nil, findingError("duplicate needs its original finding")
		}
		seen := map[string]bool{c.ID: true}
		id := c.DuplicateOf
		for id != "" {
			if seen[id] {
				return nil, findingError("duplicate relation is cyclic")
			}
			seen[id] = true
			other := views[id]
			if other == nil {
				return nil, findingError("duplicate reference does not exist")
			}
			id = other.Change.DuplicateOf
		}
	} else if c.DuplicateOf != "" {
		return nil, findingError("duplicate_of requires duplicate status")
	}
	if c.Rework != nil {
		r, err := findingRef(lines, *c.Rework)
		if err != nil {
			return nil, err
		}
		if r.Kind != "note" || r.Note != "reopen" || strings.TrimSpace(r.Quote) == "" || r.Seq <= d.Seq {
			return nil, findingError("rework must reference a later explicit reopen request")
		}
	}
	return d, nil
}

// FoldFindings computes claims and relationships from saved bytes alone. It
// never interprets a source document, contacts a model, or starts rework.
func FoldFindings(lines []record.Line, goal string) (*FindingsReport, error) {
	out := &FindingsReport{Schema: "findings/v1", Goal: goal, Findings: []FindingView{}}
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		out.RecordHead = &Head{Seq: last.Seq, SHA256: last.Hash()}
	}
	views := map[string]*FindingView{}
	requests := map[string]bool{}
	for i, l := range lines {
		if l.Schema != record.FindingSchema {
			continue
		}
		if l.Kind != "finding" || l.Finding == nil {
			return nil, findingError("invalid extension at seq %d", l.Seq)
		}
		c := *l.Finding
		if requests[c.RequestID] {
			return nil, findingError("duplicate stored request id at seq %d", l.Seq)
		}
		started := false
		for _, prior := range lines[:i] {
			if prior.Kind == "start" && prior.Goal == goal {
				started = true
				break
			}
		}
		if !started {
			return nil, findingError("finding has no matching goal start")
		}
		old := views[c.ID]
		d, err := validateFinding(lines[:i], c, old, views)
		if err != nil {
			return nil, fmt.Errorf("seq %d: %w", l.Seq, err)
		}
		if old == nil {
			old = &FindingView{ID: c.ID, Commit: d.Commit, SpecDigest: d.SpecDigest, History: []FindingEvent{}}
			views[c.ID] = old
		}
		old.Revision = l.Seq
		old.Change = c
		old.History = append(old.History, FindingEvent{Seq: l.Seq, At: l.At, SHA256: l.Hash(), Change: c})
		requests[c.RequestID] = true
	}
	keys := make([]string, 0, len(views))
	for id := range views {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		v := views[id]
		v.ReworkStatus = "not_requested"
		if v.Change.Rework != nil {
			v.ReworkStatus = "requested"
			for _, l := range lines {
				if l.Seq <= v.Change.Rework.Seq {
					continue
				}
				if l.Kind == "note" && l.Note == "reopen" {
					break
				}
				if l.Kind == "done" && l.Status == Complete {
					v.ReworkStatus = "completed"
					v.ReworkCompletion = &record.RecordRef{Seq: l.Seq, SHA256: l.Hash()}
				}
			}
		}
		switch v.Change.Status {
		case "candidate":
			out.Candidate++
			out.Unresolved++
		case "confirmed":
			out.Confirmed++
			out.Unresolved++
			if v.Change.Mapping == "linked" && v.Change.Category == "existing_condition" {
				out.ConfirmedExisting++
			}
		}
		out.Findings = append(out.Findings, *v)
	}
	return out, nil
}

// FindingReplay signals an already-applied logical request. Callers report the
// original receipt and append nothing, including after an uncertain write.
type FindingReplay struct {
	Seq    int
	SHA256 string
}

func (e *FindingReplay) Error() string {
	return fmt.Sprintf("finding request already recorded at seq %d", e.Seq)
}

func NewFinding(lines []record.Line, goal string, change record.FindingChange) (*record.Finding, error) {
	report, err := FoldFindings(lines, goal)
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		if l.Finding != nil && l.Finding.RequestID == change.RequestID {
			// Compare the stored wire content: an empty optional list and its
			// omission are the same request once written.
			stored, err := json.Marshal(*l.Finding)
			if err != nil {
				return nil, err
			}
			retried, err := json.Marshal(change)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(stored, retried) {
				return nil, findingError("request id already names different content")
			}
			return nil, &FindingReplay{Seq: l.Seq, SHA256: l.Hash()}
		}
	}
	started := false
	for _, l := range lines {
		if l.Kind == "start" && l.Goal == goal {
			started = true
		}
	}
	if !started {
		return nil, findingError("goal has not started")
	}
	views := map[string]*FindingView{}
	for i := range report.Findings {
		v := &report.Findings[i]
		views[v.ID] = v
	}
	if _, err = validateFinding(lines, change, views[change.ID], views); err != nil {
		return nil, err
	}
	return &record.Finding{Header: record.Header{Kind: "finding"}, Finding: change}, nil
}

func (r *FindingsReport) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return append(data, '\n'), err
}
