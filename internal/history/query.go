package history

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Lineage values: an event's recorded run head is in the selected run chain
// (same), names a different line at that seq (different), or cannot be
// checked against it (unverifiable).
const (
	LineageSame         = "same"
	LineageDifferent    = "different"
	LineageUnverifiable = "unverifiable"
)

// Default and maximum page sizes.
const (
	DefaultLimit = 20
	MaxLimit     = 200
)

// Now is the context a query compares recorded contexts with.
type Now struct {
	Goal       string `json:"goal"`
	Commit     string `json:"commit"`
	TreeClean  bool   `json:"tree_clean"`
	SpecDigest string `json:"spec_digest"`
}

// Query selects what to read.
type Query struct {
	Mode      string // current, list, or show
	ID        string // show: event id
	RequestID string // show: stored request id
	Targets   []string
	Keywords  []string
	Since     string
	Page      string
	Limit     int
}

// Chain is the selected run record: line hashes in order, and whether the
// whole chain verified. A broken chain makes every lineage unverifiable.
type Chain struct {
	Hashes []string
	Valid  bool
}

// Grounds names a builds_on event corrected in the selected lineage, regardless
// of whether the correction precedes, is, or follows the dependent event.
type Grounds struct {
	ID string   `json:"id"`
	By []string `json:"by"`
}

// Differences compares an event's recorded context with now.
type Differences struct {
	Code string `json:"code"` // same, changed, uncommitted
	Spec string `json:"spec"` // same, changed
	Goal string `json:"goal"` // same, moved
}

// Item is one entry of a list, current, or changes result: an event summary
// or a replacement notice.
type Item struct {
	Type             string       `json:"type"` // event or replacement
	ID               string       `json:"id"`
	Seq              int          `json:"seq,omitempty"`
	Kind             string       `json:"kind,omitempty"`
	Status           string       `json:"status,omitempty"`
	Summary          string       `json:"summary"`
	Targets          []string     `json:"targets,omitempty"`
	RecordedAt       string       `json:"recorded_at,omitempty"`
	EventAt          string       `json:"event_at,omitempty"`
	Lineage          string       `json:"lineage,omitempty"`
	Current          bool         `json:"current,omitempty"`
	SupersededBy     []string     `json:"superseded_by,omitempty"`
	OtherCorrections []string     `json:"other_corrections,omitempty"`
	GroundsCorrected []Grounds    `json:"grounds_corrected,omitempty"`
	Context          *Differences `json:"context,omitempty"`
	By               string       `json:"by,omitempty"` // replacement: the correcting event
	BySummary        string       `json:"by_summary,omitempty"`
}

// Related is a direct predecessor or successor in a detail view.
type Related struct {
	ID       string `json:"id"`
	Relation string `json:"relation"` // builds_on or supersedes
	Kind     string `json:"kind"`
	Status   string `json:"status,omitempty"`
	Summary  string `json:"summary"`
	Lineage  string `json:"lineage"`
	Current  bool   `json:"current"`
	// After is the last correction the supersedes relation acknowledged.
	After string `json:"after,omitempty"`
}

// EvidenceView is one evidence reference with what was observed at
// registration and how it compares now.
type EvidenceView struct {
	Evidence
	Recorded Observed `json:"recorded"`
	Now      string   `json:"now"`
}

// Detail is the full view of one event.
type Detail struct {
	Item
	Reason     string         `json:"reason,omitempty"`
	BuildsOn   []string       `json:"builds_on,omitempty"`
	Supersedes []Supersede    `json:"supersedes,omitempty"`
	Evidence   []EvidenceView `json:"evidence,omitempty"`
	RequestID  string         `json:"request_id"`
	Recorded   Context        `json:"recorded_context"`
}

// Other summarizes events outside the selected lineage in a current view.
type Other struct {
	Count int          `json:"count"`
	IDs   []OtherEntry `json:"ids"`
}

// OtherEntry is one event outside the selected lineage.
type OtherEntry struct {
	ID      string `json:"id"`
	Lineage string `json:"lineage"`
}

// Filters echoes the query selection.
type Filters struct {
	Targets  []string `json:"targets"`
	Keywords []string `json:"keywords"`
	Since    *string  `json:"since"`
}

// View is a query result. The JSON and Markdown forms carry the same content.
type View struct {
	Schema       string    `json:"schema"`
	Goal         string    `json:"goal"`
	Mode         string    `json:"mode"`
	History      string    `json:"history"`
	Start        Ref       `json:"start"`
	Events       int       `json:"events"`
	RunHead      *Ref      `json:"run_head"`
	RunChain     string    `json:"run_chain"`
	Now          Now       `json:"now"`
	Filters      Filters   `json:"filters"`
	Items        []Item    `json:"items"`
	OtherLineage *Other    `json:"other_lineage,omitempty"`
	Event        *Detail   `json:"event,omitempty"`
	Predecessors []Related `json:"predecessors,omitempty"`
	Successors   []Related `json:"successors,omitempty"`
	Shown        int       `json:"shown"`
	Total        int       `json:"total"`
	Truncated    bool      `json:"truncated"`
	Next         *string   `json:"next"`
	Cursor       string    `json:"cursor"`
}

// Input is everything a query reads; it is gathered without writing.
type Input struct {
	State       string
	Events      []Event
	Start       Ref
	Chain       Chain
	Now         Now
	FileVersion func(path string) (version, state string)
}

type engine struct {
	in        Input
	end       int // events in range
	pinned    int // run lines in the selected chain
	index     map[string]int
	lineage   []string
	correctBy map[string][]int // event id -> indexes of correcting events (all lineages)
}

func short(s string) string {
	if len(s) > 16 {
		return s[:16]
	}
	return s
}

func cursorErr(code, format string, a ...any) error { return fail(code, format, a...) }

// position is the pinned boundary a cursor carries.
type position struct {
	end     int
	runSeq  int
	runHash string
}

func (p position) text(prefix, startHash string) string {
	return fmt.Sprintf("%s.%s.%d.%d.%s", prefix, short(startHash), p.end, p.runSeq, short(p.runHash))
}

func parsePosition(fields []string, start string) (position, error) {
	if len(fields) < 5 || fields[1] != short(start) {
		return position{}, cursorErr("cursor_invalid", "the cursor belongs to another history or is malformed")
	}
	end, err1 := strconv.Atoi(fields[2])
	seq, err2 := strconv.Atoi(fields[3])
	if err1 != nil || err2 != nil || end < 0 || seq < 1 || len(fields[4]) != 16 {
		return position{}, cursorErr("cursor_invalid", "the cursor is malformed")
	}
	return position{end: end, runSeq: seq, runHash: fields[4]}, nil
}

func (g *engine) checkPosition(p position, what string) error {
	if p.end > len(g.in.Events) {
		return cursorErr("cursor_invalid", "the %s cursor names %d events but this history has %d", what, p.end, len(g.in.Events))
	}
	if !g.in.Chain.Valid || p.runSeq > len(g.in.Chain.Hashes) || short(g.in.Chain.Hashes[p.runSeq-1]) != p.runHash {
		return cursorErr("cursor_stale", "the run lineage of the %s cursor cannot be confirmed in the current run record; query again without it", what)
	}
	return nil
}

func queryDigest(q Query, since string) string {
	targets := append([]string(nil), q.Targets...)
	sort.Strings(targets)
	keywords := make([]string, len(q.Keywords))
	for i, k := range q.Keywords {
		keywords[i] = strings.ToLower(k)
	}
	sort.Strings(keywords)
	h := sha256.Sum256([]byte(strings.Join([]string{q.Mode, q.ID, q.RequestID, strings.Join(targets, ","), strings.Join(keywords, "\x00"), since}, "\x1f")))
	return hex.EncodeToString(h[:8])
}

func (g *engine) lineageOf(head Ref) string {
	if !g.in.Chain.Valid || head.Seq < 1 || head.Seq > g.pinned {
		return LineageUnverifiable
	}
	if g.in.Chain.Hashes[head.Seq-1] != head.SHA256 {
		return LineageDifferent
	}
	return LineageSame
}

// correctors returns the ids of events in range that supersede id, split into
// the selected lineage and others.
func (g *engine) correctors(id string) (same, other []string) {
	for _, i := range g.correctBy[id] {
		if i >= g.end {
			continue
		}
		e := g.in.Events[i]
		if g.lineage[i] == LineageSame {
			same = append(same, e.ID)
		} else {
			other = append(other, e.ID)
		}
	}
	return same, other
}

func (g *engine) current(i int) bool {
	if g.lineage[i] != LineageSame {
		return false
	}
	same, _ := g.correctors(g.in.Events[i].ID)
	return len(same) == 0
}

func (g *engine) differences(c Context) *Differences {
	d := &Differences{Code: "same", Spec: "same", Goal: "same"}
	switch {
	case c.Commit != g.in.Now.Commit:
		d.Code = "changed"
	case !c.TreeClean || !g.in.Now.TreeClean:
		d.Code = "uncommitted"
	}
	if c.SpecDigest != g.in.Now.SpecDigest {
		d.Spec = "changed"
	}
	if c.Goal != g.in.Now.Goal {
		d.Goal = "moved"
	}
	return d
}

func (g *engine) item(i int) Item {
	e := g.in.Events[i]
	same, other := g.correctors(e.ID)
	it := Item{Type: "event", ID: e.ID, Seq: e.Seq, Kind: e.Decision.Kind, Status: e.Decision.Status,
		Summary: e.Decision.Summary, Targets: e.Decision.Targets, RecordedAt: e.At, EventAt: e.Decision.EventAt,
		Lineage: g.lineage[i], Current: g.current(i), SupersededBy: same, OtherCorrections: other,
		Context: g.differences(e.Context)}
	for _, b := range e.Decision.BuildsOn {
		if by, _ := g.correctors(b); len(by) > 0 {
			it.GroundsCorrected = append(it.GroundsCorrected, Grounds{ID: b, By: by})
		}
	}
	return it
}

func (g *engine) matches(i int, q Query) bool {
	d := g.in.Events[i].Decision
	if len(q.Targets) > 0 {
		hit := false
		for _, t := range q.Targets {
			for _, have := range d.Targets {
				hit = hit || t == have
			}
		}
		if !hit {
			return false
		}
	}
	text := strings.ToLower(d.Summary + "\n" + d.Reason)
	for _, k := range q.Keywords {
		if !strings.Contains(text, strings.ToLower(k)) {
			return false
		}
	}
	return true
}

func (g *engine) related(id, relation string) Related {
	i := g.index[id]
	e := g.in.Events[i]
	return Related{ID: id, Relation: relation, Kind: e.Decision.Kind, Status: e.Decision.Status,
		Summary: e.Decision.Summary, Lineage: g.lineage[i], Current: g.current(i)}
}

// Run answers a query from verified stored events and the selected run chain.
// It reads nothing else and never writes.
func Run(in Input, q Query) (*View, error) {
	if q.Mode == "show" {
		if (q.ID == "") == (q.RequestID == "") {
			return nil, fail("invalid_input", "show requires exactly one event id or request id")
		}
		if q.ID != "" && !eventID.MatchString(q.ID) {
			return nil, fail("invalid_input", "event id must be D followed by a positive integer without leading zeros")
		}
		if q.RequestID != "" {
			if err := CheckRequestID(q.RequestID); err != nil {
				return nil, err
			}
		}
	}
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit {
		return nil, fail("invalid_input", "limit must be between 1 and %d", MaxLimit)
	}
	for _, t := range q.Targets {
		if !targetID.MatchString(t) {
			return nil, fail("invalid_input", "target %q is not a criterion, extra check, or task identifier", t)
		}
	}
	for _, k := range q.Keywords {
		if err := singleLine("keyword", k, MaxText, true); err != nil {
			return nil, err
		}
	}
	g := &engine{in: in, end: len(in.Events), pinned: len(in.Chain.Hashes), index: map[string]int{}, correctBy: map[string][]int{}}
	digest := queryDigest(q, q.Since)
	offset := 0
	if q.Page != "" {
		fields := strings.Split(q.Page, ".")
		if len(fields) != 7 || fields[0] != "p1" {
			return nil, cursorErr("cursor_invalid", "the page cursor is malformed")
		}
		p, err := parsePosition(fields[:5], in.Start.SHA256)
		if err != nil {
			return nil, err
		}
		if err := g.checkPosition(p, "page"); err != nil {
			return nil, err
		}
		if fields[5] != digest {
			return nil, cursorErr("cursor_mismatch", "the page cursor belongs to a different query; repeat the original selection")
		}
		if offset, err = strconv.Atoi(fields[6]); err != nil || offset < 0 {
			return nil, cursorErr("cursor_invalid", "the page cursor is malformed")
		}
		g.end, g.pinned = p.end, p.runSeq
	}
	sinceEnd := -1
	if q.Since != "" {
		fields := strings.Split(q.Since, ".")
		if len(fields) != 5 || fields[0] != "c1" {
			return nil, cursorErr("cursor_invalid", "the since cursor is malformed")
		}
		p, err := parsePosition(fields, in.Start.SHA256)
		if err != nil {
			return nil, err
		}
		if err := g.checkPosition(p, "since"); err != nil {
			return nil, err
		}
		if p.end > g.end || p.runSeq > g.pinned {
			return nil, cursorErr("cursor_invalid", "the since cursor is newer than this query's range")
		}
		sinceEnd = p.end
	}
	for i, e := range in.Events {
		g.index[e.ID] = i
		g.lineage = append(g.lineage, "")
		for _, s := range e.Decision.Supersedes {
			g.correctBy[s.ID] = append(g.correctBy[s.ID], i)
		}
	}
	for i := range in.Events {
		g.lineage[i] = g.lineageOf(in.Events[i].Context.RunHead)
	}
	v := &View{Schema: ViewSchema, Goal: in.Now.Goal, Mode: q.Mode, History: in.State, Start: in.Start,
		Events: g.end, RunChain: "valid", Now: in.Now, Items: []Item{},
		Filters: Filters{Targets: append([]string{}, q.Targets...), Keywords: append([]string{}, q.Keywords...)}}
	if !in.Chain.Valid {
		v.RunChain = "broken"
	}
	if q.Since != "" {
		since := q.Since
		v.Filters.Since = &since
	}
	pos := position{end: g.end, runSeq: g.pinned}
	if g.pinned > 0 && g.pinned <= len(in.Chain.Hashes) {
		pos.runHash = in.Chain.Hashes[g.pinned-1]
		v.RunHead = &Ref{Seq: g.pinned, SHA256: pos.runHash}
	}
	v.Cursor = pos.text("c1", in.Start.SHA256)
	var items []Item
	switch q.Mode {
	case "show":
		target := -1
		for i := 0; i < g.end; i++ {
			e := in.Events[i]
			if (q.ID != "" && e.ID == q.ID) || (q.RequestID != "" && e.RequestID == q.RequestID) {
				target = i
			}
		}
		if target < 0 {
			name := q.ID
			if name == "" {
				name = "request " + q.RequestID
			}
			return nil, fail("unknown_event", "%s is not stored in this history", name)
		}
		e := in.Events[target]
		d := &Detail{Item: g.item(target), Reason: e.Decision.Reason, BuildsOn: e.Decision.BuildsOn,
			Supersedes: e.Decision.Supersedes, RequestID: e.RequestID, Recorded: e.Context}
		for k, ev := range e.Decision.Evidence {
			d.Evidence = append(d.Evidence, EvidenceView{Evidence: ev, Recorded: e.Context.Evidence[k], Now: g.evidenceNow(ev, e.Context.Evidence[k])})
		}
		v.Event = d
		for _, b := range e.Decision.BuildsOn {
			v.Predecessors = append(v.Predecessors, g.related(b, "builds_on"))
		}
		for _, s := range e.Decision.Supersedes {
			r := g.related(s.ID, "supersedes")
			r.After = s.After
			v.Predecessors = append(v.Predecessors, r)
		}
		var successors []Related
		for i := target + 1; i < g.end; i++ {
			later := in.Events[i].Decision
			for _, b := range later.BuildsOn {
				if b == e.ID {
					successors = append(successors, g.related(in.Events[i].ID, "builds_on"))
				}
			}
			for _, s := range later.Supersedes {
				if s.ID == e.ID {
					r := g.related(in.Events[i].ID, "supersedes")
					r.After = s.After
					successors = append(successors, r)
				}
			}
		}
		v.Total = len(successors)
		if offset > len(successors) {
			offset = len(successors)
		}
		page := successors[offset:]
		if len(page) > q.Limit {
			page = page[:q.Limit]
		}
		v.Successors = page
		v.Shown = len(page)
		g.paginate(v, pos, digest, offset, len(page), len(successors))
		return v, nil
	case "list", "current":
	default:
		return nil, fail("invalid_input", "unknown query mode %q", q.Mode)
	}
	from := 0
	if sinceEnd >= 0 {
		from = sinceEnd
	}
	other := &Other{IDs: []OtherEntry{}}
	for i := from; i < g.end; i++ {
		if !g.matches(i, q) {
			continue
		}
		if q.Mode == "list" || g.current(i) {
			items = append(items, g.item(i))
		} else if q.Mode == "current" && g.lineage[i] != LineageSame {
			other.Count++
			if len(other.IDs) < 50 {
				other.IDs = append(other.IDs, OtherEntry{ID: in.Events[i].ID, Lineage: g.lineage[i]})
			}
		}
	}
	if sinceEnd >= 0 {
		// Replacement notices: corrections stored in the since range that
		// replace a matching judgment of the selected lineage, even when the
		// correcting event itself is outside the filters.
		var merged []Item
		next := 0
		for i := sinceEnd; i < g.end; i++ {
			for next < len(items) && items[next].Seq <= i+1 {
				merged = append(merged, items[next])
				next++
			}
			if g.lineage[i] != LineageSame {
				continue
			}
			s := in.Events[i]
			for _, sup := range s.Decision.Supersedes {
				x, ok := g.index[sup.ID]
				if !ok || g.lineage[x] != LineageSame || !g.matches(x, q) {
					continue
				}
				merged = append(merged, Item{Type: "replacement", ID: sup.ID, Summary: in.Events[x].Decision.Summary,
					By: s.ID, BySummary: s.Decision.Summary, Seq: s.Seq})
			}
		}
		merged = append(merged, items[next:]...)
		items = merged
	}
	if q.Mode == "current" {
		v.OtherLineage = other
	}
	v.Total = len(items)
	if offset > len(items) {
		offset = len(items)
	}
	page := items[offset:]
	if len(page) > q.Limit {
		page = page[:q.Limit]
	}
	v.Items = append([]Item{}, page...)
	v.Shown = len(page)
	g.paginate(v, pos, digest, offset, len(page), len(items))
	return v, nil
}

func (g *engine) paginate(v *View, pos position, digest string, offset, shown, total int) {
	if offset+shown < total {
		next := fmt.Sprintf("%s.%s.%d", pos.text("p1", g.in.Start.SHA256), digest, offset+shown)
		v.Next = &next
		v.Truncated = true
	}
}

func (g *engine) evidenceNow(e Evidence, recorded Observed) string {
	switch e.Type {
	case "run":
		if !g.in.Chain.Valid || e.Seq > len(g.in.Chain.Hashes) {
			return "unverifiable"
		}
		if g.in.Chain.Hashes[e.Seq-1] != recorded.Version {
			return "different"
		}
		return "same"
	case "file":
		if g.in.FileVersion == nil || recorded.Version == "" {
			return "unidentified"
		}
		version, _ := g.in.FileVersion(e.Path)
		switch version {
		case "":
			return "unavailable"
		case recorded.Version:
			return "unchanged"
		}
		return "changed"
	case "commit":
		return recorded.State
	}
	return "not_checked"
}
