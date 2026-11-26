package main

// Checks of the goal ha-review-gate that read the checked tree: Seal's start
// rule and Core requirements, its compatibility helper, and the fixed
// tokens the public contracts, guides and Seal skill hold about the review
// gate. They read files of the tree and never its history. The opening
// line of an assertion's failure message names the property it observes
// (jaekit:violation=<id>); a failure without it is a broken setup.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// rgTreeFile reads a file of the checked tree by its repository path.
func rgTreeFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// rgSection returns the text from the line that starts with heading up to the
// next heading of the same or a higher level, or "" when there is none.
func rgSection(text, heading string) string {
	level := strings.IndexByte(heading, ' ')
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, heading) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if n := strings.IndexByte(lines[j], ' '); n > 0 && n <= level && strings.Trim(lines[j][:n], "#") == "" {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

// rgMissing returns the tokens text lacks.
func rgMissing(text string, tokens ...string) []string {
	var out []string
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			out = append(out, token)
		}
	}
	return out
}

// The review reason codes of run-rules/5.
var rgReasonCodes = []string{"`review_missing`", "`review_blocked`", "`review_degraded`", "`review_open_defect`",
	"`review_open_question`", "`review_code_missing`", "`review_head_stale`", "`review_docs_stale`", "`review_plan_missing`"}

const rgRecordCommand = "`ha note <goal> review --gate <file>`"

// The run-rules/4 features Seal keeps requiring under run-rules/5.
var rgRules4Features = []string{"test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1"}

// AC-13 (run-rules-8): this Core lists run-rules/8 and starts a goal under it.
// Only a missing listing or a refusal of the rule is the violation.
func TestHaReviewGateAC13_CoreSupportsTheNewRules(t *testing.T) {
	t.Parallel()
	const v = "rules-v8-unsupported"
	var out, errb bytes.Buffer
	if code := runMain([]string{"capabilities", "--format", "json"}, &out, &errb); code != exitOK {
		t.Fatalf("setup: capabilities exit %d: %s", code, errb.String())
	}
	var caps struct {
		Rules []string `json:"supported_rules"`
	}
	if err := json.Unmarshal(out.Bytes(), &caps); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !slices.Contains(caps.Rules, "run-rules/8") {
		t.Errorf("jaekit:violation=%s ha capabilities lists supported_rules %v without run-rules/8", v, caps.Rules)
	}
	f := rgRepo(t, "")
	code, msg := f.ha("start", rgGoal, "--request", "implement the synthetic goal", "--rules", "run-rules/8")
	switch {
	case code == exitUsage && strings.Contains(msg, "--rules"):
		t.Fatalf("jaekit:violation=%s ha start --rules run-rules/8: exit %d: %s", v, code, strings.TrimSpace(msg))
	case code != exitOK:
		t.Fatalf("setup: ha start exit %d: %s", code, strings.TrimSpace(msg))
	}
	if r := ciLines(f)[0].m["rules"]; r != "run-rules/8" {
		t.Errorf("jaekit:violation=%s ha start --rules run-rules/8 records rules %v", v, r)
	}
}

// AC-13: Seal declares run-rules/5 and the gate support, starts with them
// and records the gate; this Core accepts that start.
func TestHaReviewGateAC13_SealStartsUnderTheNewRules(t *testing.T) {
	t.Parallel()
	const v = "seal-start-rules-not-new"
	var decl struct {
		Rules    []string            `json:"rules"`
		Formats  map[string][]string `json:"formats"`
		Features []string            `json:"features"`
	}
	if err := json.Unmarshal([]byte(rgTreeFile(t, "plugins/seal/skills/seal/core-requirements.json")), &decl); err != nil {
		t.Fatalf("setup: %v", err)
	}
	features := strings.Join(decl.Features, " ") + " "
	if len(decl.Rules) != 1 || decl.Rules[0] != "run-rules/8" || len(rgMissing(strings.Join(decl.Formats["review_gate"], " "), "review-gate/v1")) > 0 ||
		len(rgMissing(features, append([]string{"review-gate/v1 "}, rgRules4Features...)...)) > 0 {
		t.Errorf("jaekit:violation=%s Seal's Core requirements are rules %v, review_gate formats %v, features %v; want run-rules/8 with review-gate/v1 and the run-rules/4 features",
			v, decl.Rules, decl.Formats["review_gate"], decl.Features)
	}
	skill := rgTreeFile(t, "plugins/seal/skills/seal/SKILL.md")
	if missing := rgMissing(skill, "`ha start <goal> --rules run-rules/8 --request", "--check-skill`", rgRecordCommand); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the Seal skill lacks %v", v, missing)
	}
	review := rgSection(skill, "## Independent review")
	if missing := rgMissing(review, rgRecordCommand, "`spar gate`"); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the Seal skill's Independent review section lacks %v", v, missing)
	}
	// This Core accepts a start with the skill as the tree has it.
	skillDir := t.TempDir()
	source := filepath.Join("..", "..", "plugins", "seal", "skills", "seal")
	err := filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(skillDir, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(skillDir, rel), data, 0o644)
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	// The start Seal makes: run-rules/5, its Core requirements checked, and
	// the run-rules/4 test authority it keeps. Only a refusal of the rule, of
	// the test authority or of the skill's declared requirements is the
	// violation; any other failure (an internal error, a lint problem of the
	// fixture) is a broken setup.
	f := rgRepo(t, "standard")
	code, out := f.ha("start", rgGoal, "--request", "implement the synthetic goal", "--skill", filepath.Join(skillDir, "SKILL.md"),
		"--check-skill", "--rules", "run-rules/5", "--test-authority", "the request allows changing tests inside the scope")
	switch {
	case code == exitOK:
	case code == exitUsage && (strings.Contains(out, "--rules") || strings.Contains(out, "--test-authority")),
		code == exitRefused && (strings.Contains(out, "missing Core support") || strings.Contains(out, "skill requirements")):
		t.Fatalf("jaekit:violation=%s ha start with the Seal skill, run-rules/5 and test authority: exit %d: %s", v, code, strings.TrimSpace(out))
	default:
		t.Fatalf("setup: ha start exit %d: %s", code, strings.TrimSpace(out))
	}
	// The run-rules/4 notes stay available under run-rules/5.
	for _, note := range [][]string{{"wait", "--reason", "a decision asked of the user"}, {"resume"}} {
		code, out := f.ha(append([]string{"note", rgGoal}, note...)...)
		switch {
		case code == exitOK:
		case code == exitUsage || code == exitRefused:
			t.Errorf("jaekit:violation=%s a run-rules/5 goal refuses the run-rules/4 note %v: exit %d: %s", v, note, code, strings.TrimSpace(out))
		default:
			t.Fatalf("setup: ha note %v exit %d: %s", note, code, strings.TrimSpace(out))
		}
	}
}

// AC-13: Seal's compatibility helper starts new goals under run-rules/5 and
// refuses a Core that lacks it or the gate support.
func TestHaReviewGateAC13_SealCheckerNeedsTheNewRules(t *testing.T) {
	t.Parallel()
	const v = "seal-checker-accepts-old-core"
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 is not available: %v", err)
	}
	helper, err := filepath.Abs(filepath.Join("..", "..", "plugins", "seal", "skills", "seal", "scripts", "check_core.py"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var out, errb bytes.Buffer
	if code := runMain([]string{"capabilities", "--format", "json"}, &out, &errb); code != exitOK {
		t.Fatalf("setup: capabilities exit %d: %s", code, errb.String())
	}
	core := func(change func(map[string]any)) string {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal(out.Bytes(), &value); err != nil {
			t.Fatalf("setup: %v", err)
		}
		change(value)
		data, err := json.Marshal(value)
		if err != nil || bytes.ContainsRune(data, '\'') {
			t.Fatalf("setup: capabilities for a fake Core: %v", err)
		}
		path := filepath.Join(t.TempDir(), "ha")
		script := "#!/bin/sh\nif [ \"$*\" != 'capabilities --format json' ]; then exit 99; fi\nprintf '%s\\n' '" + string(data) + "'\n"
		writeExecutable(t, path, script)
		return path
	}
	decide := func(path string, args ...string) (int, map[string]any) {
		t.Helper()
		if len(args) == 0 {
			args = []string{"--operation", "start"}
		}
		cmd := exec.Command(python, append([]string{helper, "--ha", path}, args...)...)
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		cmd.Dir = t.TempDir()
		data, err := cmd.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("setup: check_core.py: %v", err)
		}
		var report map[string]any
		if json.Unmarshal(data, &report) != nil {
			t.Fatalf("setup: check_core.py exit %d gave no decision: %s", code, data)
		}
		// Only a decision about the capabilities is observed; a fake Core
		// that cannot start, times out or answers badly is a broken setup.
		switch report["reason"] {
		case "compatible", "missing_support", "unsupported_default_rule", "unsupported_stored_rule":
		default:
			t.Fatalf("setup: check_core.py exit %d decided %v, not about the capabilities: %s", code, report["reason"], data)
		}
		return code, report
	}
	if code, report := decide(core(func(map[string]any) {})); code != 0 || report["rules"] != "run-rules/8" {
		t.Errorf("jaekit:violation=%s on this Core the helper decides exit %d, rules %v; want a run-rules/8 start", v, code, report["rules"])
	}
	without := func(list []any, value string) []any {
		var out []any
		for _, item := range list {
			if item != value {
				out = append(out, item)
			}
		}
		return out
	}
	for _, form := range []struct {
		label  string
		change func(map[string]any)
	}{
		{"a Core without run-rules/8", func(m map[string]any) {
			m["supported_rules"] = without(m["supported_rules"].([]any), "run-rules/8")
		}},
		{"a Core without the review-gate/v1 feature", func(m map[string]any) {
			m["features"] = without(m["features"].([]any), "review-gate/v1")
		}},
		{"a Core without the review gate format", func(m map[string]any) {
			delete(m["formats"].(map[string]any), "review_gate")
		}},
	} {
		if code, report := decide(core(form.change)); code != 2 || report["reason"] != "missing_support" {
			t.Errorf("jaekit:violation=%s %s: the helper decides exit %d, reason %v; want a refusal for missing support", v, form.label, code, report["reason"])
		}
	}
	// A stored run-rules/8 goal resumes, checks and changes its budget on this
	// Core, needs the review gate feature for it as /5 to /7 goals do, and is
	// refused by a Core that does not list run-rules/8.
	goal := t.TempDir()
	compatibilityWrite(t, filepath.Join(goal, "SPEC.md"), "# Synthetic goal\nStatus: Ready\n", 0o644)
	compatibilityWrite(t, filepath.Join(goal, "runs.jsonl"), `{"schema":"run/v1","kind":"start","rules":"run-rules/8"}`+"\n", 0o644)
	this := core(func(map[string]any) {})
	noGate := core(func(m map[string]any) { m["features"] = without(m["features"].([]any), "review-gate/v1") })
	noRules8 := core(func(m map[string]any) { m["supported_rules"] = without(m["supported_rules"].([]any), "run-rules/8") })
	for _, operation := range []string{"resume", "check", "budget"} {
		if code, report := decide(noRules8, "--operation", operation, "--goal", goal); code != 2 || report["reason"] != "unsupported_stored_rule" {
			t.Errorf("jaekit:violation=%s %s of a stored run-rules/8 goal on a Core without run-rules/8: the helper decides exit %d, reason %v; want unsupported_stored_rule",
				v, operation, code, report["reason"])
		}
		if code, report := decide(this, "--operation", operation, "--goal", goal); code != 0 || report["rules"] != "run-rules/8" {
			t.Errorf("jaekit:violation=%s %s of a stored run-rules/8 goal on this Core: the helper decides exit %d, reason %v; want compatible", v, operation, code, report["reason"])
		}
		code, report := decide(noGate, "--operation", operation, "--goal", goal)
		if missing, _ := report["missing"].([]any); code != 2 || report["reason"] != "missing_support" || !slices.Contains(missing, any("features:review-gate/v1")) {
			t.Errorf("jaekit:violation=%s %s of a stored run-rules/8 goal without review-gate/v1: the helper decides exit %d, reason %v, missing %v; want a refusal naming the feature",
				v, operation, code, report["reason"], missing)
		}
	}
}

// AC-14: the run record contract states the new rules, the gate record and
// the reasons.
func TestHaReviewGateAC14_RunRecordContractStatesTheGate(t *testing.T) {
	t.Parallel()
	const v = "run-record-gate-missing"
	text := rgTreeFile(t, "contracts/run-record.md")
	rules5 := rgSection(text, "### 4.8 run-rules/5")
	if missing := rgMissing(rules5, append([]string{rgRecordCommand, "`review-gate/v1`", "`light`", "`standard`", "`strict`",
		"`run-rules/1`", "`run-rules/4`"}, rgReasonCodes...)...); rules5 == "" || len(missing) > 0 {
		t.Errorf("jaekit:violation=%s section 4.8 run-rules/5 is missing or lacks %v", v, missing)
	}
	if missing := rgMissing(rgSection(text, "## 1. `ha` commands"), rgRecordCommand); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the command table lacks %v", v, missing)
	}
	if missing := rgMissing(rgSection(text, "### 4.2 Goal-level reasons"), rgReasonCodes...); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the goal-level reasons table lacks %v", v, missing)
	}
	if missing := rgMissing(rgSection(text, "### 4.6 Rules versions"), "| `run-rules/5` |"); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the rules table lacks a run-rules/5 column", v)
	}
	if missing := rgMissing(rgSection(text, "### 2.4 `note`"), "`review`"); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the note fields do not name the review note", v)
	}
}

// AC-8 (run-rules-8): the run record contract adds a run-rules/8 column.
func TestHaReviewGateAC14_RunRecordStatesRunRules8(t *testing.T) {
	t.Parallel()
	const v = "run-record-v8-missing"
	text := rgTreeFile(t, "contracts/run-record.md")
	versions := rgSection(text, "### 4.6 Rules versions")
	if missing := rgMissing(versions, "| `run-rules/7` | `run-rules/8` |"); versions == "" || len(missing) > 0 {
		t.Errorf("jaekit:violation=%s section 4.6 Rules versions is missing or lacks %q", v, missing)
	}
	// Every row of its table has the label and /1 to /8.
	var uneven []string
	for _, line := range strings.Split(versions, "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), `\|`, "")
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|"), "|")
		if len(cells) != 9 {
			uneven = append(uneven, strings.TrimSpace(cells[0]))
		}
	}
	if len(uneven) > 0 {
		t.Errorf("jaekit:violation=%s %d rows of the rules table do not have 9 cells (the label and run-rules/1 to /8), the first %q", v, len(uneven), uneven[0])
	}
}

// AC-14: the Core capability contract names the rule, the format and the
// feature.
func TestHaReviewGateAC14_CapabilitiesContractStatesTheGate(t *testing.T) {
	t.Parallel()
	const v = "capabilities-gate-missing"
	text := rgTreeFile(t, "contracts/core-capabilities.md")
	section := rgSection(text, "## 9. run-rules/5")
	if missing := rgMissing(section, "`run-rules/5`", "`review-gate/v1`", "`formats.review_gate`", rgRecordCommand); section == "" || len(missing) > 0 {
		t.Errorf("jaekit:violation=%s section 9 run-rules/5 is missing or lacks %v", v, missing)
	}
	if missing := rgMissing(text, "| `review-gate/v1` |", "| `formats.review_gate` |"); len(missing) > 0 {
		t.Errorf("jaekit:violation=%s the capability tables lack %v", v, missing)
	}
}

// AC-14: both usage guides explain the new rules, the gate record, the
// reasons and how to finish without review.
func TestHaReviewGateAC14_GuidesStateTheGate(t *testing.T) {
	t.Parallel()
	const v = "guide-gate-missing"
	for path, heading := range map[string]string{"guides/USAGE.md": "## 검토 gate (run-rules/5)", "guides/USAGE.en.md": "## Review gate (run-rules/5)"} {
		section := rgSection(rgTreeFile(t, path), heading)
		if missing := rgMissing(section, rgRecordCommand, "`run-rules/5`", "`review_missing`", "`review_head_stale`", "`review_docs_stale`",
			"`Assurance: light`", "`spar gate`"); section == "" || len(missing) > 0 {
			t.Errorf("jaekit:violation=%s %s: section %q is missing or lacks %v", v, path, heading, missing)
		}
	}
}
