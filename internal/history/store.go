package history

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Store is the history of one start record in one repository:
// <git-common-dir>/ha/history/<start-sha256>/events.jsonl. Every working tree
// of the repository reaches the same files and the same lock.
type Store struct {
	common string
	start  string
}

// Open names the history of the start record whose line bytes hash to start.
// It touches nothing on disk.
func Open(commonDir, start string) (*Store, error) {
	if !sha256Hex.MatchString(start) {
		return nil, fail("not_started", "the start record identity is not a sha256")
	}
	return &Store{common: commonDir, start: start}, nil
}

// Dir is the directory of this history.
func (s *Store) Dir() string { return filepath.Join(s.common, "ha", "history", s.start) }

func (s *Store) file() string { return filepath.Join(s.Dir(), "events.jsonl") }

// last names the event Core stored most recently, so that losing whole
// trailing lines of events.jsonl shows as damage instead of a shorter history.
func (s *Store) last() string { return filepath.Join(s.Dir(), "last") }

// dirs are the directories from the common Git directory down to this history.
func (s *Store) dirs() []string {
	return []string{filepath.Join(s.common, "ha"), filepath.Join(s.common, "ha", "history"), s.Dir()}
}

// Present reports whether a history file exists. Absent means only that no
// history is stored here; it does not show that none was ever recorded.
const (
	Present = "present"
	Absent  = "absent"
)

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Hash is the sha256 of the stored line bytes; the next line's prev.
func (e Event) Hash() string { return hashBytes(e.Raw) }

// Load reads and verifies the stored history without creating or changing
// anything. Absence, unreadable storage, damage, and an unsupported schema are
// distinct results. It waits for a registration in progress, so a line being
// written is never mistaken for damage.
func (s *Store) Load() (string, []Event, error) {
	tx, err := s.openStorage(false)
	if errors.Is(err, fs.ErrNotExist) {
		return Absent, nil, nil
	}
	if err != nil {
		return "", nil, fail("history_unreadable", "%v", err)
	}
	defer tx.close()
	for {
		unlock, held, err := tx.acquire(true)
		if err != nil {
			return "", nil, fail("history_unreadable", "%v", err)
		}
		if !held {
			unlockedRead()
		}
		state, events, _, loadErr := s.load(tx)
		unlock()
		if held {
			return state, events, loadErr
		}
		// The first writer may have created its lock while this unlocked read
		// saw a partial line. Retry under that lock rather than report corruption.
		present, err := tx.lockPresent()
		if err != nil {
			return "", nil, fail("history_unreadable", "%v", err)
		}
		if !present {
			return state, events, loadErr
		}
	}
}

// storage supplies the platform I/O for one complete operation. Windows pins
// its directory and uses handle-relative no-reparse access for descendants.
type storage interface {
	close()
	read(string) ([]byte, error)
	appendFile() (appendFile, error)
	replaceLast([]byte) error
	acquire(shared bool) (unlock func(), held bool, err error)
	lockPresent() (bool, error)
}

// unlockedRead separates a read without the lock for tests.
var unlockedRead = func() {}

// load reads the history and checks it against the last stored event. lag
// reports that the record of the last event is one event behind, which a
// registration interrupted after writing its line leaves behind.
func (s *Store) load(tx storage) (state string, events []Event, lag bool, err error) {
	last, err := s.readLast(tx)
	if err != nil {
		return "", nil, false, err
	}
	data, err := tx.read("events.jsonl")
	if errors.Is(err, fs.ErrNotExist) {
		if last.Seq > 0 {
			return "", nil, false, fail("history_corrupt", "events.jsonl is missing but D%d was stored; the stored history is not repaired", last.Seq)
		}
		return Absent, nil, false, nil
	}
	if Code(err) == "history_corrupt" {
		return "", nil, false, err
	}
	if err != nil {
		return "", nil, false, fail("history_unreadable", "%v", err)
	}
	if events, err = Parse(data, s.start); err != nil {
		return "", nil, false, err
	}
	n := len(events)
	switch {
	case last.Seq > n:
		return "", nil, false, fail("history_corrupt", "the history ends at D%d but D%d was stored; trailing events were removed and are not restored", n, last.Seq)
	case last.Seq > 0 && events[last.Seq-1].Hash() != last.SHA256:
		return "", nil, false, fail("history_corrupt", "D%d differs from the event stored as D%d", last.Seq, last.Seq)
	case n > last.Seq+1:
		return "", nil, false, fail("history_corrupt", "the record of the last stored event names D%d but the history has %d events", last.Seq, n)
	}
	return Present, events, n == last.Seq+1, nil
}

// lastEvent is the content of the last file: the seq and line hash of the
// event Core stored most recently. An absent file means none yet.
type lastEvent struct {
	Seq    int    `json:"seq"`
	SHA256 string `json:"sha256"`
}

func (s *Store) readLast(tx storage) (lastEvent, error) {
	var last lastEvent
	data, err := tx.read("last")
	if errors.Is(err, fs.ErrNotExist) {
		return last, nil
	}
	if err != nil {
		return last, fail("history_unreadable", "%v", err)
	}
	if err := strictJSON(bytes.TrimSuffix(data, []byte("\n")), &last); err != nil || last.Seq < 1 || !sha256Hex.MatchString(last.SHA256) {
		return lastEvent{}, fail("history_corrupt", "the record of the last stored event is invalid")
	}
	return last, nil
}

// writeLast replaces the last file atomically after the event line is stored.
func (s *Store) writeLast(tx storage, e Event) error {
	data, _ := json.Marshal(lastEvent{Seq: e.Seq, SHA256: e.Hash()})
	if err := tx.replaceLast(append(data, '\n')); err != nil {
		return fail("history_unreadable", "%v", err)
	}
	return nil
}

// storedSchema requires an unambiguous exact schema key without applying the
// current event payload's field or nesting restrictions to unknown versions.
func storedSchema(raw []byte) (string, error) {
	if !utf8.Valid(raw) {
		return "", errors.New("JSON must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return "", errors.New("history line must be a JSON object")
	}
	seen := map[string]bool{}
	var schema string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return "", errors.New("history object has an invalid or duplicate field")
		}
		seen[key] = true
		if key != "schema" && strings.EqualFold(key, "schema") {
			return "", errors.New("schema field must use its exact wire name")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return "", err
		}
		if key == "schema" {
			if err := json.Unmarshal(value, &schema); err != nil || schema == "" {
				return "", errors.New("schema is empty or not a string")
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return "", err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return "", errors.New("trailing data after the history object")
	}
	if !seen["schema"] {
		return "", errors.New("schema is missing")
	}
	return schema, nil
}

// Parse verifies stored bytes: one schema, consecutive seq, the prev chain,
// identifiers, the start identity, and relations to earlier events only.
func Parse(data []byte, start string) ([]Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	corrupt := func(n int, format string, a ...any) error {
		return fail("history_corrupt", "line %d: %s; the stored history is not repaired or truncated", n, fmt.Sprintf(format, a...))
	}
	if data[len(data)-1] != '\n' {
		return nil, corrupt(bytes.Count(data, []byte("\n"))+1, "the last line is not terminated")
	}
	var events []Event
	ids := map[string]int{}
	requests := map[string]bool{}
	var prev *string
	for i, raw := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		n := i + 1
		if len(raw) > MaxLine {
			return nil, corrupt(n, "the line exceeds %d bytes", MaxLine)
		}
		// Inspect only the envelope before dispatching. A future version may
		// define payload fields and nesting that the current Event cannot read.
		schema, err := storedSchema(raw)
		if err != nil {
			return nil, corrupt(n, "%v", err)
		}
		if schema != Schema {
			return nil, fail("history_unsupported", "line %d uses schema %q; this ha reads %s only", n, schema, Schema)
		}
		var e Event
		if err := strictJSON(raw, &e); err != nil {
			return nil, corrupt(n, "%v", err)
		}
		e.Raw = append([]byte(nil), raw...)
		switch {
		case e.Seq != n:
			return nil, corrupt(n, "seq is %d", e.Seq)
		case prev == nil && e.Prev != nil, prev != nil && (e.Prev == nil || *e.Prev != *prev):
			return nil, corrupt(n, "prev does not match the previous line")
		case e.ID != "D"+strconv.Itoa(n):
			return nil, corrupt(n, "id is %q", e.ID)
		case CheckRequestID(e.RequestID) != nil || requests[e.RequestID]:
			return nil, corrupt(n, "request id is invalid or repeated")
		case e.Context.Start.SHA256 != start:
			return nil, corrupt(n, "the event belongs to another start record")
		case len(e.Context.Evidence) != len(e.Decision.Evidence):
			return nil, corrupt(n, "evidence observations do not match the evidence")
		}
		if _, err := time.Parse(time.RFC3339, e.At); err != nil {
			return nil, corrupt(n, "recorded time is invalid")
		}
		if err := Check(e.Decision); err != nil {
			return nil, corrupt(n, "%v", err)
		}
		for _, id := range e.Decision.BuildsOn {
			if ids[id] == 0 {
				return nil, corrupt(n, "builds_on %s is not an earlier event", id)
			}
		}
		for _, s := range e.Decision.Supersedes {
			if ids[s.ID] == 0 || (s.After != "" && ids[s.After] == 0) {
				return nil, corrupt(n, "supersedes %s is not an earlier event", s.ID)
			}
		}
		h := e.Hash()
		prev = &h
		ids[e.ID] = n
		requests[e.RequestID] = true
		events = append(events, e)
	}
	return events, nil
}

// persist separates the write boundary for fault injection in tests.
var persist = persistLine

type appendFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// persistLine reports every failure after a write was attempted as
// potentially applied: the caller must not assume success or non-application.
func persistLine(f appendFile, data []byte) error {
	n, err := f.Write(data)
	if err != nil || n != len(data) {
		f.Close()
		if err == nil {
			err = io.ErrShortWrite
		}
		return fail("write_uncertain", "%v", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fail("write_uncertain", "%v", err)
	}
	if err := f.Close(); err != nil {
		return fail("write_uncertain", "%v", err)
	}
	return nil
}

// Replay reports that a request was already stored. The caller returns the
// original receipt and appends nothing.
type Replay struct{ Event Event }

func (r *Replay) Error() string { return "request already stored as " + r.Event.ID }

// Append adds one event while holding the history lock shared by every working
// tree. build runs under the lock with the verified stored events and returns
// the event to add, a *Replay, or a refusal. Append fills seq, prev, id, the
// recorded time, and the ha version.
func (s *Store) Append(now time.Time, haVersion string, build func([]Event) (*Event, error)) (*Event, error) {
	tx, err := s.openStorage(true)
	if err != nil {
		return nil, fail("history_unreadable", "%v", err)
	}
	defer tx.close()
	unlock, _, err := tx.acquire(false)
	if err != nil {
		return nil, fail("history_unreadable", "%v", err)
	}
	defer unlock()
	_, events, lag, err := s.load(tx)
	if err != nil {
		return nil, err
	}
	// An earlier registration stored its line but not the record of it. Catch
	// the record up first so it never falls more than one event behind.
	if lag {
		if err := s.writeLast(tx, events[len(events)-1]); err != nil {
			return nil, err
		}
	}
	e, err := build(events)
	if err != nil {
		return nil, err
	}
	e.Schema = Schema
	e.Seq = len(events) + 1
	e.ID = "D" + strconv.Itoa(e.Seq)
	e.Prev = nil
	if len(events) > 0 {
		p := events[len(events)-1].Hash()
		e.Prev = &p
	}
	e.At = now.UTC().Format(time.RFC3339)
	e.HaVersion = haVersion
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	raw := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(raw) > MaxLine {
		return nil, fail("limit_exceeded", "the stored event would exceed %d bytes", MaxLine)
	}
	f, err := tx.appendFile()
	if err != nil {
		return nil, fail("history_unreadable", "%v", err)
	}
	if err := persist(f, append(append([]byte(nil), raw...), '\n')); err != nil {
		return nil, err
	}
	e.Raw = raw
	// The event is stored, but until its record is written the loss of this
	// last line would go unnoticed. Say so instead of returning a receipt; the
	// same request replays once the record catches up.
	if err := s.writeLast(tx, *e); err != nil {
		return nil, fail("write_uncertain", "%s was stored, but the record of the last stored event was not updated (%v); until it is, removing %s from the end of the history is not detected", e.ID, err, e.ID)
	}
	return e, nil
}
