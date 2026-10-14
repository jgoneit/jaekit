package history

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Facts are what the product observes at registration. The callbacks only
// read: FileVersion hashes a repository file, ResolveCommit looks up a local
// commit without fetching.
type Facts struct {
	Start         Ref
	RunHashes     []string // sha256 of each run record line, in order
	Goal          string
	Commit        string
	TreeClean     bool
	SpecDigest    string
	Targets       map[string]bool
	FileVersion   func(path string) (version, state string)
	ResolveCommit func(id string) (full string, found bool)
}

func sameDecision(a, b Decision) bool {
	x, errX := json.Marshal(a)
	y, errY := json.Marshal(b)
	return errX == nil && errY == nil && bytes.Equal(x, y)
}

// LatestCorrection returns the last stored event that supersedes id, or "".
func LatestCorrection(events []Event, id string) string {
	last := ""
	for _, e := range events {
		for _, s := range e.Decision.Supersedes {
			if s.ID == id {
				last = e.ID
			}
		}
	}
	return last
}

// Prepare validates a request against the verified stored events and returns
// the event to append. A stored request with the same id and content is a
// *Replay; the same id with different content is refused.
func Prepare(d Decision, request string, events []Event, f Facts) (*Event, error) {
	if err := CheckRequestID(request); err != nil {
		return nil, err
	}
	if err := Check(d); err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.RequestID == request {
			if sameDecision(e.Decision, d) {
				return nil, &Replay{Event: e}
			}
			return nil, fail("request_conflict", "request id %s already names %s with different content", request, e.ID)
		}
	}
	if len(f.RunHashes) == 0 {
		return nil, fail("not_started", "the goal has no valid start record")
	}
	for _, t := range d.Targets {
		if !f.Targets[t] {
			return nil, fail("invalid_reference", "target %s is not a criterion, extra check, or task of this goal", t)
		}
	}
	ids := map[string]bool{}
	for _, e := range events {
		ids[e.ID] = true
	}
	for _, id := range d.BuildsOn {
		if !ids[id] {
			return nil, fail("invalid_relation", "builds_on %s is not an already stored earlier event of this history", id)
		}
	}
	for _, s := range d.Supersedes {
		if !ids[s.ID] {
			return nil, fail("invalid_relation", "supersedes %s is not an already stored earlier event of this history", s.ID)
		}
		latest := LatestCorrection(events, s.ID)
		if s.After != "" && !ids[s.After] {
			return nil, fail("invalid_relation", "%s is not a stored event", s.After)
		}
		if s.After != latest {
			if latest == "" {
				return nil, fail("correction_conflict", "%s has no stored correction, not %s; review it and pass --supersedes %s", s.ID, s.After, s.ID)
			}
			return nil, fail("correction_conflict", "%s was last corrected by %s; review %s and pass --supersedes %s@%s", s.ID, latest, latest, s.ID, latest)
		}
	}
	observed := make([]Observed, len(d.Evidence))
	for i, e := range d.Evidence {
		switch e.Type {
		case "run":
			if e.Seq > len(f.RunHashes) {
				return nil, fail("invalid_reference", "run record seq %d does not exist", e.Seq)
			}
			hash := f.RunHashes[e.Seq-1]
			if e.SHA256 != "" && e.SHA256 != hash {
				return nil, fail("invalid_reference", "run record seq %d has a different hash", e.Seq)
			}
			observed[i] = Observed{Version: hash, State: "matched"}
		case "file":
			version, state := f.FileVersion(e.Path)
			observed[i] = Observed{Version: version, State: state}
		case "commit":
			if e.Repository != "" {
				observed[i] = Observed{State: "other_repository"}
			} else if full, ok := f.ResolveCommit(strings.ToLower(e.Commit)); ok {
				observed[i] = Observed{Version: full, State: "found"}
			} else {
				observed[i] = Observed{State: "not_found"}
			}
		case "external":
			observed[i] = Observed{Version: e.Version, State: "not_checked"}
		}
	}
	head := Ref{Seq: len(f.RunHashes), SHA256: f.RunHashes[len(f.RunHashes)-1]}
	return &Event{
		RequestID: request,
		Decision:  d,
		Context: Context{Start: f.Start, RunHead: head, Goal: f.Goal, Commit: f.Commit,
			TreeClean: f.TreeClean, SpecDigest: f.SpecDigest, Evidence: observed},
	}, nil
}
