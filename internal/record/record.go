// Package record reads and appends the run record `runs.jsonl`
// (contracts/run-record.md, schema run/v1). Each line carries the sha256
// of the previous line's bytes, so a deleted or edited line breaks the chain.
// The chain detects accidents; it does not prevent forgery by an agent with
// the same user permissions.
package record

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jgoneit/jaekit/internal/evidence"
)

// Schema is the run record schema identifier.
const Schema = "run/v1"

// Header holds the fields common to every line, in contract order.
type Header struct {
	Schema    string  `json:"schema"`
	Seq       int     `json:"seq"`
	Prev      *string `json:"prev"`
	Kind      string  `json:"kind"`
	At        string  `json:"at"`
	HaVersion string  `json:"ha_version"`
}

func (h *Header) header() *Header { return h }

// Entry is a line about to be appended.
type Entry interface{ header() *Header }

// Request is the quoted user request recorded at start. It is a claim.
type Request struct {
	Source string `json:"source"`
	Quote  string `json:"quote"`
}

// Host describes the host and model. It is a claim.
type Host struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Model   string `json:"model"`
}

// Skill describes the skill package the agent says it read. It is a claim.
type Skill struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// Budget is the plan budget recorded at start.
type Budget struct {
	Runs           int    `json:"runs"`
	ElapsedSeconds int64  `json:"elapsed_seconds"`
	Cost           string `json:"cost"`
}

// BudgetChangeData replaces the current window's total run cap. Revision is
// the preceding start, reopen, or budget_change sequence, never a check seq.
type BudgetChangeData struct {
	Goal           string `json:"goal"`
	WindowStart    int    `json:"window_start"`
	Revision       int    `json:"revision"`
	PreviousRuns   int    `json:"previous_runs"`
	Runs           int    `json:"runs"`
	Quote          string `json:"quote"`
	Context        string `json:"context"`
	Interpretation string `json:"interpretation"`
	Reason         string `json:"reason"`
}

// Unknown change fields cannot silently introduce unsupported time/cost edits.
func (d *BudgetChangeData) UnmarshalJSON(data []byte) error {
	type plain BudgetChangeData
	var value plain
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&value); err != nil {
		return err
	}
	*d = BudgetChangeData(value)
	return nil
}

// BudgetChange is a /3 budget_change line.
type BudgetChange struct {
	Header
	Change BudgetChangeData `json:"budget_change"`
}

// Target is what a check ran: a criterion, or one command of a task.
type Target struct {
	Criterion string `json:"criterion,omitempty"`
	Task      string `json:"task,omitempty"`
	Index     int    `json:"index,omitempty"`
}

// Overlay is a check file copied from HEAD into the baseline worktree.
type Overlay struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// Bound is the value a confirm note is tied to.
type Bound struct {
	CriterionSHA256 string `json:"criterion_sha256,omitempty"`
	SpecDigest      string `json:"spec_digest,omitempty"`
	Blob            string `json:"blob,omitempty"`
}

// CriterionResult is one criterion in a done line.
type CriterionResult struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Satisfied bool   `json:"satisfied"`
	Records   []int  `json:"records"`
}

// Start is a `start` line.
type Start struct {
	Header
	Goal       string  `json:"goal"`
	BaseCommit string  `json:"base_commit"`
	TreeClean  bool    `json:"tree_clean"`
	SpecDigest string  `json:"spec_digest"`
	Request    Request `json:"request"`
	Host       Host    `json:"host"`
	Skill      Skill   `json:"skill"`
	Rules      string  `json:"rules"`
	Budget     Budget  `json:"budget"`
}

// Check is a `check` line.
type Check struct {
	Header
	Target        Target            `json:"target"`
	CriterionKind string            `json:"criterion_kind,omitempty"`
	Argv          []string          `json:"argv"`
	CommandDigest string            `json:"command_digest"`
	Commit        string            `json:"commit"`
	TreeClean     bool              `json:"tree_clean"`
	SpecDigest    string            `json:"spec_digest"`
	Exit          *int              `json:"exit"`
	Result        string            `json:"result"`
	DurationMS    int64             `json:"duration_ms"`
	OutputDigest  string            `json:"output_digest"`
	OutputPath    string            `json:"output_path"`
	Evidence      *evidence.Summary `json:"evidence,omitempty"`
}

// Baseline is a `baseline` line.
type Baseline struct {
	Header
	Target        Target            `json:"target"`
	CriterionKind string            `json:"criterion_kind"`
	Argv          []string          `json:"argv"`
	CommandDigest string            `json:"command_digest"`
	Commit        string            `json:"commit"`
	TreeClean     bool              `json:"tree_clean"`
	SpecDigest    string            `json:"spec_digest"`
	BaseCommit    string            `json:"base_commit"`
	Overlay       []Overlay         `json:"overlay"`
	Exit          *int              `json:"exit"`
	Result        string            `json:"result"`
	DurationMS    int64             `json:"duration_ms"`
	OutputDigest  string            `json:"output_digest"`
	OutputPath    string            `json:"output_path"`
	Evidence      *evidence.Summary `json:"evidence,omitempty"`
}

// Note is a `note` line.
type Note struct {
	Header
	Note    string `json:"note"`
	Cause   string `json:"cause,omitempty"`
	Subject string `json:"subject,omitempty"`
	Bound   *Bound `json:"bound,omitempty"`
	Quote   string `json:"quote"`
}

// Done is a `done` line.
type Done struct {
	Header
	Status         string            `json:"status"`
	Reasons        []string          `json:"reasons"`
	Commit         string            `json:"commit"`
	TreeClean      bool              `json:"tree_clean"`
	SpecDigest     string            `json:"spec_digest"`
	RecordHead     string            `json:"record_head"`
	Criteria       []CriterionResult `json:"criteria"`
	CommandDigests map[string]string `json:"command_digests"`
}

// Line is any decoded line. Fields that do not apply to the line's kind are
// zero. Raw is the exact line bytes without the newline.
type Line struct {
	Header
	Goal           string            `json:"goal"`
	BaseCommit     string            `json:"base_commit"`
	TreeClean      bool              `json:"tree_clean"`
	SpecDigest     string            `json:"spec_digest"`
	Request        *Request          `json:"request"`
	Host           *Host             `json:"host"`
	Skill          *Skill            `json:"skill"`
	Rules          string            `json:"rules"`
	Budget         *Budget           `json:"budget"`
	BudgetChange   *BudgetChangeData `json:"budget_change"`
	Target         *Target           `json:"target"`
	CriterionKind  string            `json:"criterion_kind"`
	Argv           []string          `json:"argv"`
	CommandDigest  string            `json:"command_digest"`
	Commit         string            `json:"commit"`
	Overlay        []Overlay         `json:"overlay"`
	Exit           *int              `json:"exit"`
	Result         string            `json:"result"`
	DurationMS     int64             `json:"duration_ms"`
	OutputDigest   string            `json:"output_digest"`
	OutputPath     string            `json:"output_path"`
	Evidence       *evidence.Summary `json:"evidence"`
	Note           string            `json:"note"`
	Cause          string            `json:"cause"`
	Subject        string            `json:"subject"`
	Bound          *Bound            `json:"bound"`
	Quote          string            `json:"quote"`
	Status         string            `json:"status"`
	Reasons        []string          `json:"reasons"`
	RecordHead     string            `json:"record_head"`
	Criteria       []CriterionResult `json:"criteria"`
	CommandDigests map[string]string `json:"command_digests"`
	Raw            []byte            `json:"-"`
}

// Hash is the hex sha256 of the line bytes; the next line's prev.
func (l Line) Hash() string { return hashBytes(l.Raw) }

// IntegrityError describes the first place where the chain breaks.
type IntegrityError struct {
	Line   int // 1-based line number in the file
	Detail string
}

func (e *IntegrityError) Error() string {
	return fmt.Sprintf("run record chain broken at line %d: %s", e.Line, e.Detail)
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Read returns the lines of path. A missing file is an empty record. When the
// chain is broken the lines that could be decoded are still returned together
// with the integrity error.
func Read(path string) ([]Line, *IntegrityError, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return Parse(data)
}

// Parse decodes record bytes and verifies seq and prev.
func Parse(data []byte) ([]Line, *IntegrityError, error) {
	if len(data) == 0 {
		return nil, nil, nil
	}
	var bad *IntegrityError
	fail := func(n int, format string, a ...any) {
		if bad == nil {
			bad = &IntegrityError{Line: n, Detail: fmt.Sprintf(format, a...)}
		}
	}
	if data[len(data)-1] != '\n' {
		fail(bytes.Count(data, []byte("\n"))+1, "the last line is not terminated by a newline")
	}
	var lines []Line
	var prev *string
	for i, raw := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		n := i + 1
		var l Line
		if err := json.Unmarshal(raw, &l); err != nil {
			fail(n, "not valid JSON")
			continue
		}
		l.Raw = append([]byte(nil), raw...)
		if l.Schema != Schema {
			fail(n, "schema is %q, want %q", l.Schema, Schema)
		}
		if l.Seq != n {
			fail(n, "seq is %d, want %d", l.Seq, n)
		}
		switch {
		case prev == nil && l.Prev != nil:
			fail(n, "the first line must have prev null")
		case prev != nil && (l.Prev == nil || *l.Prev != *prev):
			fail(n, "prev does not match the previous line")
		}
		h := l.Hash()
		prev = &h
		lines = append(lines, l)
	}
	return lines, bad, nil
}

// ErrBroken is returned by Append when the existing chain is broken.
var ErrBroken = errors.New("run record chain is broken; refusing to append")

func marshal(e Entry) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Append adds e to the record at path while holding an exclusive lock on
// lockPath. It fills schema, seq, prev, and at; the caller sets kind and
// ha_version. When prepare is not nil it runs under the lock with the new seq
// before the line is encoded, so the caller can place files named by seq.
// Append refuses to append to a broken chain.
func Append(path, lockPath string, e Entry, now time.Time, prepare func(seq int) error) (Line, error) {
	return AppendChecked(path, lockPath, e, now, nil, prepare)
}

// UncertainWriteError means an append may already be present. Read the stored
// chain before deciding whether it applied; blindly retrying can duplicate it.
type UncertainWriteError struct{ Err error }

func (e *UncertainWriteError) Error() string {
	return fmt.Sprintf("record write may have applied; inspect stored records before retrying: %v", e.Err)
}
func (e *UncertainWriteError) Unwrap() error { return e.Err }

// AppendChecked validates against the latest complete chain under the append
// lock, before prepare or writing. The callback may finish populating e.
func AppendChecked(path, lockPath string, e Entry, now time.Time, validate func([]Line) error, prepare func(seq int) error) (Line, error) {
	unlock, err := lock(lockPath)
	if err != nil {
		return Line{}, err
	}
	defer unlock()
	lines, bad, err := Read(path)
	if err != nil {
		return Line{}, err
	}
	if bad != nil {
		return Line{}, fmt.Errorf("%w: %v", ErrBroken, bad)
	}
	if validate != nil {
		if err := validate(lines); err != nil {
			return Line{}, err
		}
	}
	h := e.header()
	h.Schema = Schema
	h.Seq = len(lines) + 1
	h.Prev = nil
	if len(lines) > 0 {
		p := lines[len(lines)-1].Hash()
		h.Prev = &p
	}
	h.At = now.UTC().Format(time.RFC3339)
	if prepare != nil {
		if err := prepare(h.Seq); err != nil {
			return Line{}, err
		}
	}
	raw, err := marshal(e)
	if err != nil {
		return Line{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return Line{}, err
	}
	data := append(append([]byte(nil), raw...), '\n')
	if err := persistLine(f, data); err != nil {
		return Line{}, err
	}
	var l Line
	if err := json.Unmarshal(raw, &l); err != nil {
		return Line{}, err
	}
	l.Raw = raw
	return l, nil
}

type appendFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// persistLine separates the write boundary for fault injection. Every failure
// after attempting a write is conservatively reported as potentially applied.
func persistLine(f appendFile, data []byte) error {
	n, err := f.Write(data)
	if err != nil || n != len(data) {
		f.Close()
		if err == nil {
			err = io.ErrShortWrite
		}
		return &UncertainWriteError{Err: err}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return &UncertainWriteError{Err: err}
	}
	if err := f.Close(); err != nil {
		return &UncertainWriteError{Err: err}
	}
	return nil
}

// lock takes an exclusive advisory lock so concurrent ha processes (for
// example parallel subagents) cannot interleave appends.
func lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
