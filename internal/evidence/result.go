package evidence

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/jgoneit/jaekit/internal/run"
)

type Observation struct {
	Target    string `json:"target"`
	Attempt   int    `json:"attempt"`
	Status    string `json:"status"`
	Violation string `json:"violation,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type Report struct {
	Schema            string        `json:"schema"`
	Invocation        string        `json:"invocation"`
	DeclarationDigest string        `json:"declaration_digest"`
	Producer          string        `json:"producer"`
	ProducerVersion   string        `json:"producer_version"`
	AttemptsComplete  bool          `json:"attempts_complete"`
	Observations      []Observation `json:"observations"`
}

// Summary contains only bounded identifiers, normalized observations and codes.
// Raw report bytes and arbitrary runner messages never enter a run record.
type Summary struct {
	Schema            string        `json:"schema"`
	Result            string        `json:"result"`
	Reason            string        `json:"reason"`
	DeclarationDigest string        `json:"declaration_digest"`
	ReportDigest      string        `json:"report_digest,omitempty"`
	Invocation        string        `json:"invocation"`
	Targets           []Observation `json:"targets,omitempty"`
}

type Invocation struct {
	ID   string
	Path string
	dir  string
}

// Prepare reserves a private, previously unused location and unpredictable ID.
// The caller closes it after storing the compact classification.
func Prepare(parent string) (*Invocation, error) {
	dir, err := os.MkdirTemp(parent, ".evidence-")
	if err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &Invocation{ID: hex.EncodeToString(nonce[:]), Path: filepath.Join(dir, "report.json"), dir: dir}, nil
}

func (i *Invocation) Env(binding string) []string {
	return []string{"HA_EVIDENCE_PATH=" + i.Path, "HA_EVIDENCE_INVOCATION=" + i.ID, "HA_EVIDENCE_DECLARATION_DIGEST=" + binding}
}

func (i *Invocation) Close() error { return os.RemoveAll(i.dir) }

func (i *Invocation) Classify(d *Declaration, binding string, process run.Result, baseline bool) Summary {
	s := Summary{Schema: SummarySchema, Invocation: i.ID, DeclarationDigest: binding}
	set := func(result, reason string) Summary { s.Result, s.Reason = result, reason; return s }
	if process.Outcome == run.Error {
		reason := process.Reason
		if reason != "process_start" && reason != "process_timeout" && reason != "process_wait" && reason != "process_output" {
			reason = "process_error"
		}
		return set("error", reason)
	}
	if process.Signal != "" {
		return set("error", "process_signal")
	}
	if d == nil || d.Validate() != nil || !digestPattern.MatchString(binding) {
		return set("unknown", "unsupported_declaration")
	}
	info, err := os.Lstat(i.Path)
	if err != nil {
		return set("unknown", "report_missing")
	}
	if !info.Mode().IsRegular() || info.Size() > maxJSONBytes {
		return set("unknown", "report_invalid_file")
	}
	f, err := os.Open(i.Path)
	if err != nil {
		return set("unknown", "report_unreadable")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxJSONBytes+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return set("unknown", "report_unreadable")
	}
	s.ReportDigest = digest(data)
	var report Report
	if strictJSON(data, &report) != nil {
		return set("unknown", "report_invalid_json")
	}
	if report.Schema != ReportSchema {
		return set("unknown", "report_schema")
	}
	if report.Invocation != i.ID || report.DeclarationDigest != binding || report.Producer != d.Producer || report.ProducerVersion != d.ProducerVersion {
		return set("unknown", "report_binding")
	}
	if !report.AttemptsComplete {
		return set("unknown", "attempt_history_missing")
	}
	if reason := validateObservations(d, report.Observations); reason != "" {
		return set("unknown", reason)
	}
	s.Targets = append([]Observation(nil), report.Observations...)
	sort.Slice(s.Targets, func(i, j int) bool {
		if s.Targets[i].Target == s.Targets[j].Target {
			return s.Targets[i].Attempt < s.Targets[j].Attempt
		}
		return s.Targets[i].Target < s.Targets[j].Target
	})
	s.Result, s.Reason = classifyObservations(d, s.Targets, process.Exit, baseline)
	return s
}

var environmentReasons = map[string]bool{
	"dependency_missing": true, "collection_error": true, "import_error": true,
	"compile_error": true, "browser_start_error": true, "setup_error": true,
	"execution_error": true, "permission_denied": true,
}

func validateObservations(d *Declaration, observations []Observation) string {
	if len(observations) == 0 {
		return "targets_empty"
	}
	if len(observations) > 4096 {
		return "targets_invalid"
	}
	want := map[string]bool{}
	for _, t := range d.Targets {
		want[t.ID] = true
	}
	seen := map[string]int{}
	for _, o := range observations {
		if !want[o.Target] {
			return "target_mismatch"
		}
		if o.Attempt != seen[o.Target]+1 {
			return "attempt_history_invalid"
		}
		seen[o.Target] = o.Attempt
		switch o.Status {
		case "pass":
			if o.Violation != "" || o.Reason != "" {
				return "observation_invalid"
			}
		case "violation":
			if !identifier.MatchString(o.Violation) || o.Reason != "" {
				return "observation_invalid"
			}
		case "error":
			if o.Violation != "" || !environmentReasons[o.Reason] {
				return "observation_invalid"
			}
		case "skip":
			if o.Violation != "" || (o.Reason != "skipped" && o.Reason != "not_selected") {
				return "observation_invalid"
			}
		default:
			return "observation_invalid"
		}
	}
	if len(seen) != len(want) {
		return "target_missing"
	}
	return ""
}

func classifyObservations(d *Declaration, observations []Observation, exit *int, baseline bool) (string, string) {
	if exit == nil || *exit < 0 {
		return "unknown", "process_exit_missing"
	}
	want := map[string]string{}
	for _, t := range d.Targets {
		want[t.ID] = t.Violation
	}
	violations, unexpected, environment, skipped := 0, false, false, false
	passed, failed := map[string]bool{}, map[string]bool{}
	for _, o := range observations {
		switch o.Status {
		case "violation":
			violations++
			failed[o.Target] = true
			unexpected = unexpected || want[o.Target] != o.Violation
		case "error":
			environment = true
		case "skip":
			skipped = true
		case "pass":
			passed[o.Target] = true
		}
	}
	// A complete internal retry history may explain an exit-zero last attempt,
	// but cannot turn an earlier observed assertion into current/baseline success.
	if !environment && !skipped {
		for target := range passed {
			if failed[target] {
				return "fail", "inconsistent_attempts"
			}
		}
	}
	if (*exit == 0 && (violations > 0 || environment)) || (*exit != 0 && violations == 0 && !environment && !skipped) {
		return "unknown", "report_exit_conflict"
	}
	if environment {
		return "error", "environment_reported"
	}
	if skipped {
		return "unknown", "target_skipped"
	}
	if unexpected {
		return "fail", "unexpected_assertion"
	}
	if violations > 0 {
		if baseline {
			return "fail_as_expected", "declared_violation"
		}
		return "fail", "assertion_failed"
	}
	if baseline {
		return "unexpected_pass", "baseline_passed"
	}
	return "pass", "targets_passed"
}

var earlyReasons = map[string]string{
	"process_start": "error", "process_timeout": "error", "process_wait": "error", "process_error": "error", "process_signal": "error", "process_output": "error",
	"unsupported_declaration": "unknown", "report_missing": "unknown", "report_invalid_file": "unknown", "report_unreadable": "unknown",
	"report_invalid_json": "unknown", "report_schema": "unknown", "report_binding": "unknown", "attempt_history_missing": "unknown",
	"targets_empty": "unknown", "targets_invalid": "unknown", "target_mismatch": "unknown", "attempt_history_invalid": "unknown",
	"observation_invalid": "unknown", "target_missing": "unknown",
}

// HasAdverse preserves assertions even when another observation makes the
// overall result error or unknown. Callers must first validate the stored
// summary against its current declaration with ValidateStored.
func HasAdverse(s *Summary, d *Declaration, baseline bool) bool {
	if s == nil || d == nil {
		return false
	}
	want := map[string]string{}
	for _, target := range d.Targets {
		want[target.ID] = target.Violation
	}
	passed, failed := map[string]bool{}, map[string]bool{}
	for _, observation := range s.Targets {
		switch observation.Status {
		case "violation":
			if !baseline || observation.Violation != want[observation.Target] {
				return true
			}
			failed[observation.Target] = true
		case "pass":
			passed[observation.Target] = true
		}
	}
	for target := range passed {
		if failed[target] {
			return true
		}
	}
	return false
}

// ValidateStored rechecks compact evidence without the original report/log.
// It does not reinterpret historical /1 or /2 records; callers select the rule.
func ValidateStored(s *Summary, d *Declaration, binding string, baseline bool, result string, exit *int) error {
	invalid := errors.New("invalid or unbound structured evidence")
	if s == nil || s.Schema != SummarySchema || !digestPattern.MatchString(s.Invocation) || s.DeclarationDigest != binding || s.Result != result {
		return invalid
	}
	if s.ReportDigest != "" && !digestPattern.MatchString(s.ReportDigest) {
		return invalid
	}
	if expected, ok := earlyReasons[s.Reason]; ok {
		if result != expected || len(s.Targets) != 0 {
			return invalid
		}
		return nil
	}
	if d == nil || d.Validate() != nil || !digestPattern.MatchString(binding) || !digestPattern.MatchString(s.ReportDigest) || validateObservations(d, s.Targets) != "" {
		return invalid
	}
	wantResult, wantReason := classifyObservations(d, s.Targets, exit, baseline)
	if result != wantResult || s.Reason != wantReason {
		return invalid
	}
	return nil
}
