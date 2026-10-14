// Package history stores and reads decision history
// (contracts/decision-history.md, schema decision-history/v1): judgment events
// that an executor records about one started goal. The history lives in the Git
// common directory, keyed by the exact start record, apart from the run record.
//
// Events are claims. Nothing here judges their content, calls a model, reads
// the network, or changes the run record, completion, budget, or findings.
package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// Schema identifies one stored event line.
	Schema = "decision-history/v1"
	// ReceiptSchema identifies the output of a successful registration.
	ReceiptSchema = "decision-receipt/v1"
	// ViewSchema identifies a JSON query result.
	ViewSchema = "decision-history-view/v1"
)

// Documented bounds. Inputs past them are refused with limit_exceeded.
const (
	MaxInput   = 256 * 1024  // one JSON input file
	MaxLine    = 1024 * 1024 // one stored event line
	MaxSummary = 1024        // bytes, one line
	MaxReason  = 16 * 1024   // bytes
	MaxText    = 2048        // bytes of a path, section, reference, version, or repository
	MaxList    = 64          // targets, evidence, builds_on, and supersedes each
)

// Kinds are the event kinds, the same tokens as the PROGRESS timeline.
var Kinds = []string{"관측", "원인", "결정", "조치", "검증", "막힘", "재개"}

// claims lists the claim states a kind may carry. Other kinds carry none.
var claims = map[string][]string{
	"원인": {"가설", "확정", "기각"},
	"조치": {"예정", "반영"},
	"검증": {"대기", "통과", "실패"},
}

// ClaimStates returns the claim states the kind accepts, or nil.
func ClaimStates(kind string) []string { return claims[kind] }

var (
	eventID   = regexp.MustCompile(`^D[1-9][0-9]*$`)
	requestID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}$`)
	targetID  = regexp.MustCompile(`^(?:(?:AC|EX)-[1-9][0-9]*|T[0-9]{3,})$`)
	hexID     = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Ref binds a reference to exact stored bytes: a sequence and its sha256.
type Ref struct {
	Seq    int    `json:"seq"`
	SHA256 string `json:"sha256"`
}

// Evidence is one reference an event rests on. Type selects the fields:
// run (seq, optional sha256), file (path, optional section), commit (commit,
// optional repository), external (ref, optional version). Checked is the
// executor's claim that the content was read; it certifies nothing.
type Evidence struct {
	Type       string `json:"type"`
	Seq        int    `json:"seq,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Path       string `json:"path,omitempty"`
	Section    string `json:"section,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Repository string `json:"repository,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Version    string `json:"version,omitempty"`
	Checked    bool   `json:"checked,omitempty"`
}

// Supersede names an earlier judgment this event corrects, rejects, or
// replaces. After is the latest correction of that judgment the executor saw,
// or empty when it saw none; a different stored latest correction conflicts.
type Supersede struct {
	ID    string `json:"id"`
	After string `json:"after,omitempty"`
}

// Decision is what the executor submits. It is stored unchanged, so a retried
// request compares equal to the stored one.
type Decision struct {
	Kind       string      `json:"kind"`
	Status     string      `json:"status,omitempty"`
	Summary    string      `json:"summary"`
	Reason     string      `json:"reason,omitempty"`
	Targets    []string    `json:"targets,omitempty"`
	Evidence   []Evidence  `json:"evidence,omitempty"`
	BuildsOn   []string    `json:"builds_on,omitempty"`
	Supersedes []Supersede `json:"supersedes,omitempty"`
	EventAt    string      `json:"event_at,omitempty"`
}

// Observed is what the product found for one evidence reference when the
// event was registered: a version identifier ("" when none could be
// identified) and the observation state named in the contract.
type Observed struct {
	Version string `json:"version"`
	State   string `json:"state"`
}

// Context is the product-observed context of an event at registration.
type Context struct {
	Start      Ref        `json:"start"`
	RunHead    Ref        `json:"run_head"`
	Goal       string     `json:"goal"`
	Commit     string     `json:"commit"`
	TreeClean  bool       `json:"tree_clean"`
	SpecDigest string     `json:"spec_digest"`
	Evidence   []Observed `json:"evidence"`
}

// Event is one stored line. Seq and prev chain the lines like the run record,
// so an edited or deleted line is reported as damage.
type Event struct {
	Schema    string   `json:"schema"`
	Seq       int      `json:"seq"`
	Prev      *string  `json:"prev"`
	At        string   `json:"at"`
	HaVersion string   `json:"ha_version"`
	ID        string   `json:"id"`
	RequestID string   `json:"request_id"`
	Decision  Decision `json:"decision"`
	Context   Context  `json:"context"`
	Raw       []byte   `json:"-"`
}

// Error is a refusal or failure with a stable code (contracts/decision-history.md).
type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string { return e.Code + ": " + e.Detail }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, a...)}
}

// Code returns the stable code of err, or "" for an unclassified error.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// singleLine refuses empty text, control characters, and invalid UTF-8.
func singleLine(field, s string, max int, required bool) error {
	if s == "" {
		if required {
			return fail("invalid_input", "%s is required", field)
		}
		return nil
	}
	if !utf8.ValidString(s) {
		return fail("invalid_input", "%s is not valid UTF-8", field)
	}
	if len(s) > max {
		return fail("limit_exceeded", "%s exceeds %d bytes", field, max)
	}
	if strings.TrimSpace(s) == "" {
		return fail("invalid_input", "%s is blank", field)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fail("invalid_input", "%s must be one line without control characters", field)
		}
	}
	return nil
}

func uniqueList(field string, values []string, valid *regexp.Regexp) error {
	if len(values) > MaxList {
		return fail("limit_exceeded", "%s has more than %d entries", field, MaxList)
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !valid.MatchString(v) {
			return fail("invalid_input", "%s entry %q is not a valid identifier", field, v)
		}
		if seen[v] {
			return fail("invalid_input", "%s repeats %s", field, v)
		}
		seen[v] = true
	}
	return nil
}

// cleanRelative accepts a slash-separated path inside a repository.
func cleanRelative(p string) bool {
	return p != "" && p != "." && !strings.HasPrefix(p, "/") && !strings.Contains(p, "\\") &&
		path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

// CheckRequestID validates an executor-supplied request identifier.
func CheckRequestID(id string) error {
	if !requestID.MatchString(id) {
		return fail("invalid_input", "request id must start with an ASCII letter or digit and use only letters, digits and _.:/- (1-128 characters)")
	}
	return nil
}

// Check validates a decision's own form, independent of stored history.
func Check(d Decision) error {
	known := false
	for _, k := range Kinds {
		known = known || d.Kind == k
	}
	if !known {
		return fail("invalid_input", "kind must be one of %s", strings.Join(Kinds, ", "))
	}
	if d.Status != "" {
		ok := false
		for _, s := range claims[d.Kind] {
			ok = ok || d.Status == s
		}
		if !ok {
			if len(claims[d.Kind]) == 0 {
				return fail("invalid_input", "kind %s carries no claim state", d.Kind)
			}
			return fail("invalid_input", "kind %s accepts claim states %s", d.Kind, strings.Join(claims[d.Kind], ", "))
		}
	}
	if err := singleLine("summary", d.Summary, MaxSummary, true); err != nil {
		return err
	}
	if !utf8.ValidString(d.Reason) {
		return fail("invalid_input", "reason is not valid UTF-8")
	}
	if len(d.Reason) > MaxReason {
		return fail("limit_exceeded", "reason exceeds %d bytes", MaxReason)
	}
	for _, r := range d.Reason {
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return fail("invalid_input", "reason contains control characters")
		}
	}
	if err := uniqueList("targets", d.Targets, targetID); err != nil {
		return err
	}
	if err := uniqueList("builds_on", d.BuildsOn, eventID); err != nil {
		return err
	}
	if len(d.Supersedes) > MaxList {
		return fail("limit_exceeded", "supersedes has more than %d entries", MaxList)
	}
	seen := map[string]bool{}
	for _, s := range d.Supersedes {
		if !eventID.MatchString(s.ID) || (s.After != "" && !eventID.MatchString(s.After)) {
			return fail("invalid_input", "supersedes entry %q is not a valid event identifier", s.ID+"@"+s.After)
		}
		if seen[s.ID] {
			return fail("invalid_input", "supersedes repeats %s", s.ID)
		}
		seen[s.ID] = true
	}
	if d.EventAt != "" {
		if _, err := time.Parse(time.RFC3339, d.EventAt); err != nil {
			return fail("invalid_input", "event time must be RFC 3339 with a time zone")
		}
	}
	if len(d.Evidence) > MaxList {
		return fail("limit_exceeded", "evidence has more than %d entries", MaxList)
	}
	keys := map[string]bool{}
	for i, e := range d.Evidence {
		if err := checkEvidence(e); err != nil {
			var he *Error
			if errors.As(err, &he) {
				return &Error{Code: he.Code, Detail: fmt.Sprintf("evidence %d: %s", i+1, he.Detail)}
			}
			return err
		}
		key, _ := json.Marshal(Evidence{Type: e.Type, Seq: e.Seq, Path: e.Path, Section: e.Section, Commit: strings.ToLower(e.Commit), Repository: e.Repository, Ref: e.Ref})
		if keys[string(key)] {
			return fail("invalid_input", "evidence %d repeats an earlier reference", i+1)
		}
		keys[string(key)] = true
	}
	return nil
}

func checkEvidence(e Evidence) error {
	only := func(allowed ...string) error {
		set := map[string]bool{}
		for _, a := range allowed {
			set[a] = true
		}
		fields := map[string]bool{"seq": e.Seq != 0, "sha256": e.SHA256 != "", "path": e.Path != "", "section": e.Section != "",
			"commit": e.Commit != "", "repository": e.Repository != "", "ref": e.Ref != "", "version": e.Version != ""}
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if fields[name] && !set[name] {
				return fail("invalid_input", "%s evidence does not take %s", e.Type, name)
			}
		}
		return nil
	}
	switch e.Type {
	case "run":
		if e.Seq < 1 {
			return fail("invalid_input", "run evidence needs a positive seq")
		}
		if e.SHA256 != "" && !sha256Hex.MatchString(e.SHA256) {
			return fail("invalid_input", "run evidence sha256 must be 64 lowercase hex digits")
		}
		return only("seq", "sha256")
	case "file":
		if err := singleLine("file path", e.Path, MaxText, true); err != nil {
			return err
		}
		if !cleanRelative(e.Path) {
			return fail("invalid_input", "file path must be a clean repository-relative path without ..")
		}
		if err := singleLine("file section", e.Section, MaxText, false); err != nil {
			return err
		}
		return only("path", "section")
	case "commit":
		if !hexID.MatchString(e.Commit) {
			return fail("invalid_input", "commit must be 7 to 64 hexadecimal digits")
		}
		if err := singleLine("commit repository", e.Repository, MaxText, false); err != nil {
			return err
		}
		return only("commit", "repository")
	case "external":
		if err := singleLine("external reference", e.Ref, MaxText, true); err != nil {
			return err
		}
		if err := singleLine("external version", e.Version, MaxText, false); err != nil {
			return err
		}
		return only("ref", "version")
	}
	return fail("invalid_input", "evidence type must be run, file, commit or external")
}

// DecodeInput reads one strict JSON request: an object with "decision" and an
// optional "request_id". Duplicate or unknown fields (including case
// variants), invalid UTF-8, trailing data, and deep nesting are refused.
func DecodeInput(data []byte) (Decision, string, error) {
	var in struct {
		RequestID string   `json:"request_id"`
		Decision  Decision `json:"decision"`
	}
	if len(data) > MaxInput {
		return Decision{}, "", fail("limit_exceeded", "input exceeds %d bytes", MaxInput)
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return Decision{}, "", fail("invalid_input", "input must be one JSON object")
	}
	if err := strictJSON(data, &in); err != nil {
		return Decision{}, "", fail("invalid_input", "%v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Decision{}, "", fail("invalid_input", "%v", err)
	}
	if _, ok := fields["decision"]; !ok {
		return Decision{}, "", fail("invalid_input", "input needs a decision object")
	}
	if _, supplied := fields["request_id"]; supplied {
		if err := CheckRequestID(in.RequestID); err != nil {
			return Decision{}, "", err
		}
	}
	return in.Decision, in.RequestID, nil
}

func strictJSON(data []byte, out any) error {
	if !utf8.Valid(data) {
		return errors.New("JSON must be valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 16 {
			return errors.New("JSON nesting exceeds 16")
		}
		tok, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, _ := key.(string)
				if seen[name] {
					return fmt.Errorf("duplicate field %q", name)
				}
				seen[name] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		}
		_, err = d.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing data after the JSON object")
	}
	if err := exactFields(data, reflect.TypeOf(out)); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

// encoding/json matches field names case-insensitively; the contract requires
// the exact wire names at every level.
func exactFields(data []byte, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Slice:
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil
		}
		for _, item := range items {
			if err := exactFields(item, typ.Elem()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
	default:
		return nil
	}
	fields := map[string]reflect.Type{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = f.Type
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t, ok := fields[key]
		if !ok {
			return fmt.Errorf("unknown field %q", key)
		}
		if err := exactFields(object[key], t); err != nil {
			return err
		}
	}
	return nil
}
