package spar

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Schema names of the public records.
const (
	RoundSchema = "review-round/v1"
	GateSchema  = "review-gate/v1"
)

// AngleNote is one angle a reviewer did or did not check.
type AngleNote struct {
	Angle  string `json:"angle"`
	Result string `json:"result,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ReviewerState is a reviewer's outcome in a round.
type ReviewerState struct {
	Name            string      `json:"name"`
	Preset          string      `json:"preset"`
	Isolation       string      `json:"isolation"`
	Required        bool        `json:"required"`
	Status          string      `json:"status"`
	Reason          string      `json:"reason,omitempty"`
	RequestedModel  *string     `json:"requested_model"`
	ModelSource     string      `json:"model_source,omitempty"`
	ObservedModel   *string     `json:"observed_model"`
	FallbackModel   string      `json:"fallback_model,omitempty"`
	FallbackUsed    bool        `json:"fallback_used"`
	Effort          string      `json:"effort,omitempty"`
	EffortSource    string      `json:"effort_source,omitempty"`
	ElapsedSeconds  float64     `json:"elapsed_seconds"`
	TimeoutSeconds  int64       `json:"timeout_seconds,omitempty"`
	Resets          string      `json:"resets,omitempty"`
	Attempts        []Attempt   `json:"attempts"`
	Findings        *int        `json:"findings"`
	Verdict         string      `json:"verdict,omitempty"`
	AnglesChecked   []string    `json:"angles_checked"`
	AnglesUnchecked []AngleNote `json:"angles_unchecked"`
	Limits          string      `json:"limits,omitempty"`
	Warnings        []string    `json:"warnings"`
	Agent           string      `json:"agent,omitempty"`
	RecordedFrom    string      `json:"recorded_from,omitempty"`
	WorkspaceDigest string      `json:"workspace_digest,omitempty"`
}

// RoundRecord is round.json.
type RoundRecord struct {
	Schema             string                    `json:"schema"`
	SparVersion        string                    `json:"spar_version"`
	Stage              string                    `json:"stage"`
	Label              string                    `json:"label,omitempty"`
	Series             string                    `json:"series"`
	Sequence           int                       `json:"sequence"`
	Repo               string                    `json:"repo"`
	Base               string                    `json:"base"`
	Head               string                    `json:"head"`
	BaseTree           string                    `json:"base_tree"`
	HeadTree           string                    `json:"head_tree"`
	DiffSHA256         string                    `json:"diff_sha256"`
	Criteria           []Document                `json:"criteria"`
	CriteriaDigest     string                    `json:"criteria_digest"`
	Rules              []Document                `json:"rules"`
	Subjects           []Document                `json:"subjects"`
	Questions          []string                  `json:"questions"`
	Prior              []string                  `json:"prior"`
	PacketSHA256       string                    `json:"packet_sha256"`
	Config             *Config                   `json:"config"`
	Links              LinkTargets               `json:"links"`
	RequestedReviewers []string                  `json:"requested_reviewers"`
	RequiredReviewers  []string                  `json:"required_reviewers"`
	DryRun             bool                      `json:"dry_run"`
	Counted            bool                      `json:"counted"`
	RerunOf            []string                  `json:"rerun_of"`
	StartedAt          string                    `json:"started_at"`
	FinishedAt         string                    `json:"finished_at,omitempty"`
	Reviewers          map[string]*ReviewerState `json:"reviewers"`
}

// LedgerEntry is one line of ledger.jsonl.
type LedgerEntry struct {
	Finding      string `json:"finding"`
	Disposition  string `json:"disposition"`
	Auto         bool   `json:"auto"`
	Reason       string `json:"reason,omitempty"`
	Commit       string `json:"commit,omitempty"`
	FailedBefore string `json:"failed_before,omitempty"`
	PassedAfter  string `json:"passed_after,omitempty"`
	Evidence     string `json:"evidence,omitempty"`
	Of           string `json:"of,omitempty"`
	To           string `json:"to,omitempty"`
	UserQuote    string `json:"user_quote,omitempty"`
	Note         string `json:"note,omitempty"`
	RecordedAt   string `json:"recorded_at"`
}

// Extension is one line of a series' extensions.jsonl.
type Extension struct {
	Stage      string `json:"stage"`
	By         int    `json:"by"`
	UserQuote  string `json:"user_quote"`
	RecordedAt string `json:"recorded_at"`
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func appendJSONLine(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func loadRound(dir string) (*RoundRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, "round.json"))
	if err != nil {
		return nil, fmt.Errorf("%s: no round record: %v", dir, err)
	}
	var r RoundRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s/round.json is not a round record: %v", dir, err)
	}
	if r.Schema != RoundSchema {
		return nil, fmt.Errorf("%s/round.json has schema %q, not %s", dir, r.Schema, RoundSchema)
	}
	if r.Reviewers == nil {
		r.Reviewers = map[string]*ReviewerState{}
	}
	return &r, nil
}

func loadFindings(dir string) ([]Finding, error) {
	data, err := os.ReadFile(filepath.Join(dir, "findings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Finding
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s/findings.json: %v", dir, err)
	}
	return out, nil
}

func readJSONLines[T any](path string) ([]T, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []T
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, fmt.Errorf("%s line %d: %v", path, n, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func loadLedger(dir string) ([]LedgerEntry, error) {
	return readJSONLines[LedgerEntry](filepath.Join(dir, "ledger.jsonl"))
}

// finalDispositions maps each finding to its last ledger entry.
func finalDispositions(ledger []LedgerEntry) map[string]LedgerEntry {
	out := map[string]LedgerEntry{}
	for _, e := range ledger {
		out[e.Finding] = e
	}
	return out
}

type sibling struct {
	dir   string
	round *RoundRecord
}

// seriesRounds reads the review-round/v1 records in a series folder, in
// sequence order. Folders holding other records are left alone.
func seriesRounds(series string) ([]sibling, error) {
	entries, err := os.ReadDir(series)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []sibling
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(series, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, "round.json"))
		if err != nil {
			continue
		}
		var probe struct {
			Schema string `json:"schema"`
		}
		if json.Unmarshal(data, &probe) != nil || probe.Schema != RoundSchema {
			if json.Valid(data) {
				continue // another tool's record
			}
			return nil, fmt.Errorf("%s/round.json cannot be read", dir)
		}
		r, err := loadRound(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, sibling{dir, r})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].round.Sequence != out[j].round.Sequence {
			return out[i].round.Sequence < out[j].round.Sequence
		}
		return out[i].dir < out[j].dir
	})
	return out, nil
}

func loadExtensions(series string) ([]Extension, error) {
	return readJSONLines[Extension](filepath.Join(series, "extensions.jsonl"))
}

// stageGroup names the cap a stage counts against.
func stageGroup(stage string) string {
	switch stage {
	case "plan":
		return "plan"
	case "plan-delta":
		return "plan_delta"
	default:
		return "code"
	}
}

func capFor(c Caps, group string) int64 {
	switch group {
	case "plan":
		return c.Plan
	case "plan_delta":
		return c.PlanDelta
	default:
		return c.Code
	}
}

func extensionsFor(exts []Extension, group string) int64 {
	var n int64
	for _, e := range exts {
		g := e.Stage
		if g == "plan-delta" {
			g = "plan_delta"
		}
		if g == group && e.By > 0 {
			n += int64(e.By)
		}
	}
	return n
}

func sameIdentity(a, b *RoundRecord) bool {
	return a.Stage == b.Stage && a.Base == b.Base && a.Head == b.Head
}

// realish resolves symbolic links in the part of a path that exists.
func realish(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	rest := ""
	for p := abs; ; {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return abs
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// insideRepo returns a path relative to the repository, or "" outside it.
func insideRepo(repo, path string) string {
	rel, err := filepath.Rel(repo, path)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	return rel
}

// underGitFolder reports whether dir or a folder above it holds a .git
// entry: a repository's folder, or the file of a linked worktree.
func underGitFolder(dir string) bool {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// setupGuide is the Spar guide's section on making a project ignore its
// records folder; the refusal of a folder Git does not ignore points to it.
const setupGuide = "https://github.com/jgoneit/jaekit/blob/main/guides/SPAR.en.md#set-up-a-project"

// localRecordPath refuses a record path that Git would track (G28). Every
// repository that holds the path counts, from the innermost one out: the
// reviewed repository, another one around the path, and a repository nested
// in an ignored folder. Each must ignore the path and track nothing under
// it, and a .git above the path that Git cannot judge refuses it.
func localRecordPath(repo, path string) error {
	path = realish(path)
	// repo is empty when no reviewed repository was found (spar extend
	// outside one): the path is then checked only against the
	// repositories that hold it.
	if repo != "" {
		if filepath.Clean(path) == filepath.Clean(realish(repo)) {
			return fmt.Errorf("%s is the repository itself; review records go to a Git-ignored folder", path)
		}
		if rel := insideRepo(realish(repo), path); rel != "" {
			if first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]; first == ".git" {
				return fmt.Errorf("%s is inside .git", path)
			}
		}
	}
	dir := path
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
	for {
		root, err := repoRoot(dir)
		if err != nil {
			if !underGitFolder(dir) {
				return nil
			}
			// A .git above the path but Git refused to say (ownership, a
			// broken or missing linked worktree): the records do not go
			// where they cannot be checked.
			return fmt.Errorf("%s cannot be checked against its Git repository: %v", path, err)
		}
		root = realish(root)
		rel := insideRepo(root, path)
		if rel == "" {
			return fmt.Errorf("%s is the top of the Git repository %s; review records go to a Git-ignored folder", path, root)
		}
		if first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]; first == ".git" {
			return fmt.Errorf("%s is inside .git of %s", path, root)
		}
		ignored, err := ignoredPath(root, rel)
		if err != nil {
			return err
		}
		if !ignored {
			return fmt.Errorf("%s is not ignored by the Git repository %s, or that repository tracks files under it; review records can hold private documents, so they go to a Git-ignored folder (to set one up, see %s)", rel, root, setupGuide)
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil
		}
		dir = parent
	}
}

// lockSeries holds a series folder's lock, the one every write to the
// series takes (G33). When another write holds it, it says so on log and
// waits.
func lockSeries(series string, log io.Writer) (func(), error) {
	return lockFile(series, func() {
		if log != nil {
			fmt.Fprintf(log, "spar: waiting for another write of %s to finish\n", series)
		}
	})
}
