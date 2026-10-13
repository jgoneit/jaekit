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
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
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
// distinct results.
func (s *Store) Load() (string, []Event, error) {
	info, err := os.Lstat(s.file())
	if errors.Is(err, fs.ErrNotExist) {
		return Absent, nil, nil
	}
	if err != nil {
		return "", nil, fail("history_unreadable", "%v", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fail("history_unreadable", "%s is not a regular file", s.file())
	}
	data, err := os.ReadFile(s.file())
	if err != nil {
		return "", nil, fail("history_unreadable", "%v", err)
	}
	events, err := Parse(data, s.start)
	if err != nil {
		return "", nil, err
	}
	return Present, events, nil
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
		var head struct {
			Schema string `json:"schema"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return nil, corrupt(n, "not valid JSON")
		}
		if head.Schema != Schema {
			return nil, fail("history_unsupported", "line %d uses schema %q; this ha reads %s only", n, head.Schema, Schema)
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

// ensureDir creates one directory level, refusing a symlink or a non-directory
// so that a link cannot redirect writes outside the history area.
func ensureDir(p string) error {
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(p, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return fail("history_unreadable", "%v", err)
		}
		info, err = os.Lstat(p)
	}
	if err != nil {
		return fail("history_unreadable", "%v", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fail("history_unreadable", "%s is not a plain directory; refusing to write through it", p)
	}
	return nil
}

func regularOrAbsent(p string) error {
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fail("history_unreadable", "%v", err)
	}
	if !info.Mode().IsRegular() {
		return fail("history_unreadable", "%s is not a regular file; refusing to write through it", p)
	}
	return nil
}

func lock(p string) (func(), error) {
	if err := regularOrAbsent(p); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return nil, fail("history_unreadable", "%v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fail("history_unreadable", "%v", err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
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
	for _, dir := range []string{filepath.Join(s.common, "ha"), filepath.Join(s.common, "ha", "history"), s.Dir()} {
		if err := ensureDir(dir); err != nil {
			return nil, err
		}
	}
	unlock, err := lock(filepath.Join(s.Dir(), "lock"))
	if err != nil {
		return nil, err
	}
	defer unlock()
	_, events, err := s.Load()
	if err != nil {
		return nil, err
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
	if err := regularOrAbsent(s.file()); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.file(), os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return nil, fail("history_unreadable", "%v", err)
	}
	if err := persist(f, append(append([]byte(nil), raw...), '\n')); err != nil {
		return nil, err
	}
	e.Raw = raw
	return e, nil
}
