// Package reviewgate reads a review-gate/v1 file, the public record that
// `spar gate` writes (contracts/review-gate.md), into the summary a
// run-rules/5 review note keeps (contracts/run-record.md §4.8). It reads the
// file format only: Seal Core never imports Spar's packages, starts a
// reviewer or calls a model.
package reviewgate

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
	"strings"

	"github.com/jgoneit/jaekit/internal/record"
)

// Schema is the gate file schema this package reads.
const Schema = "review-gate/v1"

// States are the gate states, Stages the round stages, of review-gate/v1.
var (
	States = []string{"complete", "degraded", "blocked"}
	Stages = []string{"plan", "plan-delta", "candidate", "fixes"}
)

// Read reads the gate file at path. Any problem is an error that names it;
// the caller records nothing.
func Read(path string) (*record.ReviewData, error) {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, errors.New("the file does not exist")
	case err != nil:
		return nil, fmt.Errorf("the file cannot be read: %v", err)
	case info.IsDir():
		return nil, errors.New("is a folder, not a file")
	case !info.Mode().IsRegular():
		return nil, errors.New("is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("the file cannot be read: %v", err)
	}
	return Parse(data)
}

// Parse reads gate file bytes into the summary a review note keeps. Fields
// it does not need are not read, and unknown fields are allowed.
func Parse(data []byte) (*record.ReviewData, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, errors.New("not a JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("data follows the JSON object")
	}
	top, err := object(raw, "the file")
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	d := &record.ReviewData{GateDigest: hex.EncodeToString(sum[:])}
	if d.Schema, err = top.text("schema"); err != nil {
		return nil, err
	}
	if d.Schema != Schema {
		return nil, fmt.Errorf("schema is %q, not %s", d.Schema, Schema)
	}
	if d.SparVersion, err = top.text("spar_version"); err != nil {
		return nil, err
	}
	if d.State, err = top.text("state"); err != nil {
		return nil, err
	}
	if !oneOf(d.State, States) {
		return nil, fmt.Errorf("state %q is not one of %s", d.State, strings.Join(States, ", "))
	}
	if d.ExpectHead, err = top.text("expect_head"); err != nil {
		return nil, err
	}
	if d.LastReviewedHead, err = top.text("last_reviewed_head"); err != nil {
		return nil, err
	}
	rounds, err := top.list("rounds")
	if err != nil {
		return nil, err
	}
	d.Rounds = []record.ReviewRound{}
	for i, item := range rounds {
		r, err := readRound(item, fmt.Sprintf("rounds[%d]", i))
		if err != nil {
			return nil, err
		}
		d.Rounds = append(d.Rounds, r)
	}
	openRaw, err := top.value("open")
	if err != nil {
		return nil, err
	}
	open, err := object(openRaw, "open")
	if err != nil {
		return nil, err
	}
	if d.Open.Defect, err = open.count("defect"); err != nil {
		return nil, err
	}
	if d.Open.RequirementQuestion, err = open.count("requirement-question"); err != nil {
		return nil, err
	}
	return d, nil
}

func readRound(raw json.RawMessage, where string) (record.ReviewRound, error) {
	var r record.ReviewRound
	o, err := object(raw, where)
	if err != nil {
		return r, err
	}
	if r.Stage, err = o.text("stage"); err != nil {
		return r, err
	}
	if !oneOf(r.Stage, Stages) {
		return r, fmt.Errorf("%s.stage %q is not one of %s", where, r.Stage, strings.Join(Stages, ", "))
	}
	if r.Sequence, err = o.count("sequence"); err != nil {
		return r, err
	}
	if r.Base, err = o.text("base"); err != nil {
		return r, err
	}
	if r.Head, err = o.text("head"); err != nil {
		return r, err
	}
	if r.CriteriaDigest, err = o.text("criteria_digest"); err != nil {
		return r, err
	}
	criteria, err := o.list("criteria")
	if err != nil {
		return r, err
	}
	r.Criteria = []record.ReviewDocument{}
	for j, item := range criteria {
		at := fmt.Sprintf("%s.criteria[%d]", where, j)
		c, err := object(item, at)
		if err != nil {
			return r, err
		}
		var doc record.ReviewDocument
		if doc.Name, err = c.text("name"); err != nil {
			return r, err
		}
		if doc.Name == "" {
			return r, fmt.Errorf("%s.name is empty", at)
		}
		if doc.SHA256, err = c.text("sha256"); err != nil {
			return r, err
		}
		if !IsSHA256(doc.SHA256) {
			return r, fmt.Errorf("%s.sha256 is not a sha256 digest", at)
		}
		r.Criteria = append(r.Criteria, doc)
	}
	if r.RequiredReviewers, err = o.count("required_reviewers"); err != nil {
		return r, err
	}
	if r.CompletedRequired, err = o.count("completed_required"); err != nil {
		return r, err
	}
	if r.DryRun, err = o.flag("dry_run"); err != nil {
		return r, err
	}
	return r, nil
}

// Valid checks a stored review summary as Parse would have made it, so a
// review note edited by hand is invalid data rather than a judgment input.
func Valid(d *record.ReviewData) error {
	switch {
	case d == nil:
		return errors.New("no review data")
	case !IsSHA256(d.GateDigest):
		return errors.New("gate_digest is not a sha256 digest")
	case d.Schema != Schema:
		return fmt.Errorf("schema is %q, not %s", d.Schema, Schema)
	case !oneOf(d.State, States):
		return fmt.Errorf("state %q is not one of %s", d.State, strings.Join(States, ", "))
	case d.Open.Defect < 0 || d.Open.RequirementQuestion < 0:
		return errors.New("open counts are negative")
	}
	for i, r := range d.Rounds {
		if !oneOf(r.Stage, Stages) {
			return fmt.Errorf("rounds[%d].stage %q is not one of %s", i, r.Stage, strings.Join(Stages, ", "))
		}
		if r.Sequence < 0 || r.RequiredReviewers < 0 || r.CompletedRequired < 0 {
			return fmt.Errorf("rounds[%d] has a negative sequence or reviewer count", i)
		}
		for j, c := range r.Criteria {
			if c.Name == "" || !IsSHA256(c.SHA256) {
				return fmt.Errorf("rounds[%d].criteria[%d] has no name or no sha256 digest", i, j)
			}
		}
	}
	return nil
}

// IsSHA256 reports whether s is a lowercase hex sha256 digest.
func IsSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func oneOf(s string, list []string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}

// fields is one JSON object of the gate file, with where it is for messages.
type fields struct {
	where  string
	values map[string]json.RawMessage
}

func object(raw json.RawMessage, where string) (fields, error) {
	var values map[string]json.RawMessage
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &values) != nil {
		return fields{}, fmt.Errorf("%s is not a JSON object", where)
	}
	return fields{where, values}, nil
}

func (f fields) name(key string) string {
	if f.where == "the file" {
		return key
	}
	return f.where + "." + key
}

func (f fields) value(key string) (json.RawMessage, error) {
	v, ok := f.values[key]
	if !ok {
		return nil, fmt.Errorf("%s: missing", f.name(key))
	}
	return v, nil
}

func (f fields) text(key string) (string, error) {
	v, err := f.value(key)
	if err != nil {
		return "", err
	}
	var s string
	if bytes.HasPrefix(bytes.TrimSpace(v), []byte(`"`)) && json.Unmarshal(v, &s) == nil {
		return s, nil
	}
	return "", fmt.Errorf("%s: not a string", f.name(key))
}

func (f fields) integer(key string) (int, error) {
	v, err := f.value(key)
	if err != nil {
		return 0, err
	}
	// A quoted number is a string, not a count.
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	var n json.Number
	if bytes.HasPrefix(bytes.TrimSpace(v), []byte(`"`)) || dec.Decode(&n) != nil {
		return 0, fmt.Errorf("%s: not an integer", f.name(key))
	}
	i, err := n.Int64()
	if err != nil || int64(int(i)) != i {
		return 0, fmt.Errorf("%s: not an integer", f.name(key))
	}
	return int(i), nil
}

func (f fields) count(key string) (int, error) {
	n, err := f.integer(key)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("%s: negative", f.name(key))
	}
	return n, nil
}

func (f fields) flag(key string) (bool, error) {
	v, err := f.value(key)
	if err != nil {
		return false, err
	}
	var b bool
	if t := bytes.TrimSpace(v); (bytes.Equal(t, []byte("true")) || bytes.Equal(t, []byte("false"))) && json.Unmarshal(v, &b) == nil {
		return b, nil
	}
	return false, fmt.Errorf("%s: not true or false", f.name(key))
}

func (f fields) list(key string) ([]json.RawMessage, error) {
	v, err := f.value(key)
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if !bytes.HasPrefix(bytes.TrimSpace(v), []byte("[")) || json.Unmarshal(v, &items) != nil {
		return nil, fmt.Errorf("%s: not a list", f.name(key))
	}
	return items, nil
}
