package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

// FindingSchema is an explicit record extension. Legacy readers reject this
// schema instead of silently ignoring an unknown run/v1 kind or note.
const FindingSchema = "run-finding/v1"

// RecordRef binds a relation to exact historical bytes, not a reusable seq.
type RecordRef struct {
	Seq    int    `json:"seq"`
	SHA256 string `json:"sha256"`
}

type FindingDocument struct {
	Ref        string `json:"ref"`
	RecordedAt string `json:"recorded_at"`
	EventAt    string `json:"event_at"`
}

// FindingChange is a complete claimed revision. No field grants execution
// authority or certifies a defect's truth. Revision is the preceding event seq.
type FindingChange struct {
	ID             string           `json:"id"`
	RequestID      string           `json:"request_id"`
	Revision       int              `json:"revision"`
	Completion     RecordRef        `json:"completion"`
	Criteria       []string         `json:"criteria"`
	Mapping        string           `json:"mapping"`
	Status         string           `json:"status"`
	Category       string           `json:"category"`
	Summary        string           `json:"summary"`
	Source         string           `json:"source"`
	Evidence       []string         `json:"evidence"`
	Reason         string           `json:"reason,omitempty"`
	EventAt        string           `json:"event_at,omitempty"`
	DuplicateOf    string           `json:"duplicate_of,omitempty"`
	ResolutionRefs []string         `json:"resolution_refs,omitempty"`
	Rework         *RecordRef       `json:"rework,omitempty"`
	Document       *FindingDocument `json:"document,omitempty"`
}

type Finding struct {
	Header
	Finding FindingChange `json:"finding"`
}

func (v *FindingChange) UnmarshalJSON(data []byte) error {
	type plain FindingChange
	var out plain
	if err := strictFindingJSON(data, &out); err != nil {
		return err
	}
	*v = FindingChange(out)
	return nil
}

// MaxFindingInput bounds one finding request. MaxFindingLine bounds the stored
// line made from it: re-encoding at most doubles the input (U+2028 and U+2029
// become six-byte escapes) and the record header adds far less than 64 KiB.
const (
	MaxFindingInput = 1024 * 1024
	MaxFindingLine  = 2*MaxFindingInput + 64*1024
)

// ErrFindingTooLarge refuses a finding line past MaxFindingLine before it is
// written, so support checks never meet a line the Core produced past the bound.
var ErrFindingTooLarge = fmt.Errorf("finding line exceeds %d bytes", MaxFindingLine)

// DecodeFinding accepts one bounded JSON object with no duplicate or unknown
// fields. It does not read references or infer claims from a document's text.
// The required lists must be written as arrays; omission and null are refused.
func DecodeFinding(data []byte) (FindingChange, error) {
	var out FindingChange
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return out, errors.New("finding input must be one JSON object")
	}
	if len(data) > MaxFindingInput {
		return out, errors.New("finding input exceeds 1 MiB")
	}
	if err := strictFindingJSON(data, &out); err != nil {
		return out, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return out, err
	}
	for _, name := range []string{"criteria", "evidence"} {
		if value := bytes.TrimSpace(fields[name]); len(value) == 0 || value[0] != '[' {
			return out, fmt.Errorf("%s must be a JSON array", name)
		}
	}
	return out, nil
}

func strictFindingJSON(data []byte, out any) error {
	if !utf8.Valid(data) {
		return errors.New("finding JSON must be valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return errors.New("finding JSON nesting exceeds 32")
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
				name, ok := key.(string)
				if !ok {
					return errors.New("invalid object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate finding field %q", name)
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
		default:
			return errors.New("invalid JSON delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing finding JSON")
	}
	if err := exactFindingFields(data, reflect.TypeOf(out)); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

// encoding/json accepts case-insensitive aliases for struct fields. The public
// extension requires the exact wire names, including embedded header fields and
// nested references, so those aliases must be rejected before struct decoding.
func exactFindingFields(data []byte, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	fields := map[string]reflect.Type{}
	var collect func(reflect.Type)
	collect = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				collect(f.Type)
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
	}
	collect(typ)
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := object[key]
		t, ok := fields[key]
		if !ok {
			return fmt.Errorf("unknown finding field %q", key)
		}
		if err := exactFindingFields(value, t); err != nil {
			return err
		}
	}
	return nil
}

// ExecutionLines leaves the saved execution rules unchanged. Only events of
// this explicit extension are omitted from budget time and execution counts.
// Sequence numbers and the complete stored hash chain are never rewritten.
func ExecutionLines(lines []Line) []Line {
	out := make([]Line, 0, len(lines))
	for _, line := range lines {
		if line.Schema != FindingSchema {
			out = append(out, line)
		}
	}
	return out
}
