// Package reviewseries reads one Spar review series folder: its
// review-round/v1 rounds (contracts/review-round.md §5) and the
// review-gate/v1 files beside them (contracts/review-gate.md), for the cost
// summary `ha cost` shows (contracts/run-record.md §1.7). It reads the
// public file formats only: Seal Core never imports Spar's packages, starts a
// reviewer or calls a model, and this package writes nothing.
//
// A value an older Spar did not write is nil, so the summary can say it was
// not recorded instead of counting it as zero. A file that holds a round's
// records but cannot be read is an error that names it: a summary never
// counts part of a series as if it were all of it.
package reviewseries

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Schemas this package reads.
const (
	RoundSchema = "review-round/v1"
	GateSchema  = "review-gate/v1"
)

// Fixed values of review-round/v1. A record with another value is not one
// this package can read.
var (
	Stages       = []string{"plan", "plan-delta", "candidate", "fixes"}
	Classes      = []string{"defect", "requirement-question", "hardening", "note"}
	Dispositions = []string{"fix", "reject", "defer", "duplicate", "answer"}
)

// Series is one series folder as given.
type Series struct {
	Path       string
	Rounds     []Round
	Extensions []Extension
	Gates      []Gate
	// Skipped names the entries of the folder that are neither a
	// review-round/v1 round, a review-gate/v1 file nor the series'
	// extensions.jsonl, with why, in name order.
	Skipped []Skipped
}

// Skipped is an entry of a series folder this package did not read.
type Skipped struct {
	Name string `json:"name"`
	Why  string `json:"why"`
}

// Round is one review-round/v1 round folder.
type Round struct {
	Name     string
	Stage    string
	Sequence int
	DryRun   bool
	// Counted and RerunOf are nil when the record does not have them.
	Counted *bool
	RerunOf []string
	// Base and Head are the reviewed commits, empty when not recorded.
	Base       string
	Head       string
	StartedAt  string
	FinishedAt string
	Reviewers  []Reviewer
	// HasFindings is false for a round without findings.json: a dry run, or
	// a round stopped before it recorded findings.
	HasFindings bool
	Findings    []Finding
	Ledger      []LedgerEntry
	Waivers     []Waiver
}

// Reviewer is one reviewer's outcome in a round. Pointer fields are nil
// when the record does not have them.
type Reviewer struct {
	Name           string
	Status         string
	Required       *bool
	ElapsedSeconds *float64
	Attempts       *int
	FallbackUsed   *bool
}

// Finding is one classified finding of findings.json. InCheckPaths is nil
// when the record does not have it; a finding without location_unreadable
// had its location read (contracts/review-round.md §6).
type Finding struct {
	ID                 string
	Class              string
	InCheckPaths       *bool
	LocationUnreadable bool
}

// LedgerEntry is one disposition line of ledger.jsonl.
type LedgerEntry struct {
	Finding     string
	Disposition string
	Auto        bool
	UserQuote   string
}

// Waiver is one line of a round's waivers.jsonl.
type Waiver struct {
	Reviewer  string
	UserQuote string
}

// Extension is one line of the series' extensions.jsonl.
type Extension struct {
	Stage     string
	By        int
	UserQuote string
}

// Gate is a review-gate/v1 file at the top of the series folder.
type Gate struct {
	Name   string
	Digest string
	State  string
	// Growth is nil when the file has no growth record.
	Growth *Growth
	// Measured names the commit ranges whose added lines the gate measured,
	// as "<base>..<head>" from each growth.rounds entry, sorted without
	// repeats; nil when the file has no readable growth.rounds.
	Measured []string
}

// Growth is a gate's growth.added_lines: the lines its fixes rounds added
// inside the check paths and outside them.
type Growth struct {
	Check int64
	Other int64
}

// Read reads the series folder at path.
func Read(path string) (*Series, error) {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("review series %s does not exist", path)
	case err != nil:
		return nil, fmt.Errorf("review series %s cannot be read: %v", path, err)
	case !info.IsDir():
		return nil, fmt.Errorf("review series %s is not a folder", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("review series %s cannot be read: %v", path, err)
	}
	s := &Series{Path: path, Rounds: []Round{}, Extensions: []Extension{}, Gates: []Gate{}, Skipped: []Skipped{}}
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(path, name)
		if name == "extensions.jsonl" {
			if s.Extensions, err = readExtensions(full); err != nil {
				return nil, err
			}
			continue
		}
		if e.IsDir() {
			r, why, err := readRound(full, name)
			if err != nil {
				return nil, err
			}
			if r == nil {
				s.Skipped = append(s.Skipped, Skipped{name + "/", why})
				continue
			}
			s.Rounds = append(s.Rounds, *r)
			continue
		}
		g, why := readGate(full, name, e)
		if g == nil {
			s.Skipped = append(s.Skipped, Skipped{name, why})
			continue
		}
		s.Gates = append(s.Gates, *g)
	}
	// Spar reads a series in sequence order, then by folder name.
	sort.SliceStable(s.Rounds, func(i, j int) bool {
		if s.Rounds[i].Sequence != s.Rounds[j].Sequence {
			return s.Rounds[i].Sequence < s.Rounds[j].Sequence
		}
		return s.Rounds[i].Name < s.Rounds[j].Name
	})
	return s, nil
}

// readRound reads a folder as Spar does: one without round.json, or whose
// round.json is JSON of another schema, is another tool's folder and is
// skipped; a round.json that is not JSON cannot be read. A round.json that
// exists but cannot be opened cannot be read either: the summary cannot
// tell whose it is.
func readRound(dir, name string) (*Round, string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "round.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, "no round.json", nil
	case err != nil:
		return nil, "", fmt.Errorf("%s: round.json cannot be read: %v", dir, err)
	}
	var probe struct {
		Schema any `json:"schema"`
	}
	if json.Unmarshal(data, &probe) != nil {
		if json.Valid(data) {
			return nil, "round.json is not a review-round/v1 record", nil
		}
		return nil, "", fmt.Errorf("%s: round.json cannot be read: not JSON", dir)
	}
	if schema, _ := probe.Schema.(string); schema != RoundSchema {
		return nil, "round.json is not a review-round/v1 record", nil
	}
	r, err := parseRound(data, name)
	if err != nil {
		return nil, "", fmt.Errorf("%s: round.json cannot be read: %v", dir, err)
	}
	findings, err := os.ReadFile(filepath.Join(dir, "findings.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// A dry run and a round stopped before it recorded findings have
		// none; any other round has lost its findings.
		if !r.DryRun && r.FinishedAt != "" {
			return nil, "", fmt.Errorf("%s: findings.json is missing", dir)
		}
	case err != nil:
		return nil, "", fmt.Errorf("%s: findings.json cannot be read: %v", dir, err)
	default:
		r.HasFindings = true
		if r.Findings, err = parseFindings(findings); err != nil {
			return nil, "", fmt.Errorf("%s: findings.json cannot be read: %v", dir, err)
		}
		ledger := filepath.Join(dir, "ledger.jsonl")
		if _, err := os.Stat(ledger); err != nil {
			return nil, "", fmt.Errorf("%s: ledger.jsonl is missing", dir)
		}
		if r.Ledger, err = readLedger(ledger); err != nil {
			return nil, "", err
		}
	}
	waivers := filepath.Join(dir, "waivers.jsonl")
	// A reviewer folder named waivers.jsonl, from before waivers existed,
	// holds no waivers (contracts/review-gate.md §2).
	if info, err := os.Stat(waivers); err == nil && !info.IsDir() {
		if r.Waivers, err = readWaivers(waivers); err != nil {
			return nil, "", err
		}
	}
	return r, "", nil
}

func parseRound(data []byte, name string) (*Round, error) {
	o, err := object(data, "the record")
	if err != nil {
		return nil, err
	}
	r := &Round{Name: name}
	if r.Stage, err = o.text("stage"); err != nil {
		return nil, err
	}
	if !oneOf(r.Stage, Stages) {
		return nil, fmt.Errorf("stage %q is not one of %s", r.Stage, strings.Join(Stages, ", "))
	}
	if r.Sequence, err = o.count("sequence"); err != nil {
		return nil, err
	}
	if r.DryRun, err = o.flag("dry_run"); err != nil {
		return nil, err
	}
	if r.Counted, err = o.optionalFlag("counted"); err != nil {
		return nil, err
	}
	if raw, ok := o.present("rerun_of"); ok {
		r.RerunOf = []string{}
		if err := json.Unmarshal(raw, &r.RerunOf); err != nil {
			return nil, errors.New("rerun_of is not a list of names")
		}
	}
	if r.Base, err = o.optionalText("base"); err != nil {
		return nil, err
	}
	if r.Head, err = o.optionalText("head"); err != nil {
		return nil, err
	}
	for _, t := range []struct {
		key string
		at  *string
	}{{"started_at", &r.StartedAt}, {"finished_at", &r.FinishedAt}} {
		if *t.at, err = o.optionalText(t.key); err != nil {
			return nil, err
		}
		if _, perr := time.Parse(time.RFC3339, *t.at); *t.at != "" && perr != nil {
			return nil, fmt.Errorf("%s %q is not an RFC 3339 time", t.key, *t.at)
		}
	}
	reviewers := map[string]json.RawMessage{}
	if raw, ok := o.present("reviewers"); ok {
		if err := json.Unmarshal(raw, &reviewers); err != nil {
			return nil, errors.New("reviewers is not an object")
		}
	}
	names := make([]string, 0, len(reviewers))
	for n := range reviewers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rv, err := parseReviewer(n, reviewers[n])
		if err != nil {
			return nil, err
		}
		r.Reviewers = append(r.Reviewers, rv)
	}
	return r, nil
}

func parseReviewer(name string, raw json.RawMessage) (Reviewer, error) {
	where := "reviewers." + name
	rv := Reviewer{Name: name}
	o, err := object(raw, where)
	if err != nil {
		return rv, err
	}
	if rv.Status, err = o.text("status"); err != nil {
		return rv, err
	}
	if rv.Required, err = o.optionalFlag("required"); err != nil {
		return rv, err
	}
	if rv.FallbackUsed, err = o.optionalFlag("fallback_used"); err != nil {
		return rv, err
	}
	if raw, ok := o.present("elapsed_seconds"); ok {
		var v float64
		if json.Unmarshal(raw, &v) != nil || v < 0 {
			return rv, fmt.Errorf("%s.elapsed_seconds is not a number of seconds", where)
		}
		rv.ElapsedSeconds = &v
	}
	if raw, ok := o.present("attempts"); ok {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) != nil {
			return rv, fmt.Errorf("%s.attempts is not a list", where)
		}
		n := len(list)
		rv.Attempts = &n
	}
	return rv, nil
}

func parseFindings(data []byte) ([]Finding, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil || items == nil {
		return nil, errors.New("not a JSON list")
	}
	out := []Finding{}
	for i, item := range items {
		where := fmt.Sprintf("finding %d", i+1)
		o, err := object(item, where)
		if err != nil {
			return nil, err
		}
		f := Finding{}
		if f.ID, err = o.text("id"); err != nil {
			return nil, err
		}
		if f.Class, err = o.text("class"); err != nil {
			return nil, err
		}
		if !oneOf(f.Class, Classes) {
			return nil, fmt.Errorf("%s: class %q is not one of %s", where, f.Class, strings.Join(Classes, ", "))
		}
		if f.InCheckPaths, err = o.optionalFlag("in_check_paths"); err != nil {
			return nil, err
		}
		unreadable, err := o.optionalFlag("location_unreadable")
		if err != nil {
			return nil, err
		}
		f.LocationUnreadable = unreadable != nil && *unreadable
		out = append(out, f)
	}
	return out, nil
}

func readLedger(path string) ([]LedgerEntry, error) {
	var out []LedgerEntry
	err := eachLine(path, func(o fields) error {
		var e LedgerEntry
		var err error
		if e.Finding, err = o.text("finding"); err != nil {
			return err
		}
		if e.Disposition, err = o.text("disposition"); err != nil {
			return err
		}
		if !oneOf(e.Disposition, Dispositions) {
			return fmt.Errorf("disposition %q is not one of %s", e.Disposition, strings.Join(Dispositions, ", "))
		}
		auto, err := o.optionalFlag("auto")
		if err != nil {
			return err
		}
		e.Auto = auto != nil && *auto
		if e.UserQuote, err = o.optionalText("user_quote"); err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	return out, err
}

func readWaivers(path string) ([]Waiver, error) {
	var out []Waiver
	err := eachLine(path, func(o fields) error {
		var w Waiver
		var err error
		if w.Reviewer, err = o.text("reviewer"); err != nil {
			return err
		}
		if w.UserQuote, err = o.text("user_quote"); err != nil {
			return err
		}
		out = append(out, w)
		return nil
	})
	return out, err
}

func readExtensions(path string) ([]Extension, error) {
	out := []Extension{}
	err := eachLine(path, func(o fields) error {
		var e Extension
		var err error
		if e.Stage, err = o.text("stage"); err != nil {
			return err
		}
		if e.By, err = o.count("by"); err != nil {
			return err
		}
		if e.UserQuote, err = o.text("user_quote"); err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	return out, err
}

// eachLine reads a JSON lines file as Spar does: blank lines are skipped and
// any other line must be one JSON object.
func eachLine(path string, read func(fields) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s cannot be read: %v", path, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		o, err := object(line, "the line")
		if err == nil {
			err = read(o)
		}
		if err != nil {
			return fmt.Errorf("%s line %d cannot be read: %v", path, n, err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s cannot be read: %v", path, err)
	}
	return nil
}

// readGate reads a file at the top of the series. A file that is not a
// review-gate/v1 JSON object is skipped, with why.
func readGate(path, name string, e fs.DirEntry) (*Gate, string) {
	if !e.Type().IsRegular() {
		return nil, "not a regular file"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "cannot be read"
	}
	o, err := object(data, "the file")
	if err != nil {
		return nil, "not a review-gate/v1 file"
	}
	if schema, err := o.text("schema"); err != nil || schema != GateSchema {
		return nil, "not a review-gate/v1 file"
	}
	sum := sha256.Sum256(data)
	g := &Gate{Name: name, Digest: hex.EncodeToString(sum[:])}
	g.State, _ = o.optionalText("state")
	if raw, ok := o.present("growth"); ok {
		var growth struct {
			AddedLines map[string]json.RawMessage `json:"added_lines"`
			Rounds     json.RawMessage            `json:"rounds"`
		}
		if json.Unmarshal(raw, &growth) == nil {
			g.Measured = measuredRanges(growth.Rounds)
		}
		if growth.AddedLines != nil {
			var check, other int64
			okCheck := json.Unmarshal(growth.AddedLines["check"], &check) == nil && check >= 0
			okOther := json.Unmarshal(growth.AddedLines["other"], &other) == nil && other >= 0
			if okCheck && okOther {
				g.Growth = &Growth{Check: check, Other: other}
			}
		}
	}
	return g, ""
}

// fields is one JSON object, with where it is for messages.
type fields struct {
	where  string
	values map[string]json.RawMessage
}

func object(raw []byte, where string) (fields, error) {
	var values map[string]json.RawMessage
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &values) != nil {
		return fields{}, fmt.Errorf("%s is not a JSON object", where)
	}
	return fields{where, values}, nil
}

// present returns a key's value unless it is absent or null.
func (f fields) present(key string) (json.RawMessage, bool) {
	v, ok := f.values[key]
	if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return nil, false
	}
	return v, true
}

func (f fields) text(key string) (string, error) {
	v, ok := f.present(key)
	if !ok {
		return "", fmt.Errorf("%s has no %s", f.where, key)
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", fmt.Errorf("%s: %s is not a string", f.where, key)
	}
	return s, nil
}

func (f fields) optionalText(key string) (string, error) {
	if _, ok := f.present(key); !ok {
		return "", nil
	}
	return f.text(key)
}

func (f fields) count(key string) (int, error) {
	v, ok := f.present(key)
	if !ok {
		return 0, fmt.Errorf("%s has no %s", f.where, key)
	}
	var n int
	if json.Unmarshal(v, &n) != nil || n < 0 {
		return 0, fmt.Errorf("%s: %s is not a whole number", f.where, key)
	}
	return n, nil
}

func (f fields) flag(key string) (bool, error) {
	b, err := f.optionalFlag(key)
	if err != nil {
		return false, err
	}
	return b != nil && *b, nil
}

func (f fields) optionalFlag(key string) (*bool, error) {
	v, ok := f.present(key)
	if !ok {
		return nil, nil
	}
	var b bool
	if json.Unmarshal(v, &b) != nil {
		return nil, fmt.Errorf("%s: %s is not true or false", f.where, key)
	}
	return &b, nil
}

func oneOf(s string, list []string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}

// measuredRanges reads growth.rounds as "<base>..<head>" names, sorted
// without repeats; nil when it is missing or an entry lacks either commit.
func measuredRanges(raw json.RawMessage) []string {
	var rounds []struct {
		Base *string `json:"base"`
		Head *string `json:"head"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &rounds) != nil || rounds == nil {
		return nil
	}
	names := []string{}
	for _, r := range rounds {
		if r.Base == nil || r.Head == nil {
			return nil
		}
		names = append(names, *r.Base+".."+*r.Head)
	}
	sort.Strings(names)
	out := names[:0]
	for i, n := range names {
		if i == 0 || n != names[i-1] {
			out = append(out, n)
		}
	}
	return out
}
