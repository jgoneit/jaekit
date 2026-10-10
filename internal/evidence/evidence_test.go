package evidence

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/run"
)

func testDeclaration() *Declaration {
	return &Declaration{Schema: DeclarationSchema, Producer: "reference", ProducerVersion: "1", ProducerPaths: []string{"producer.py"}, Targets: []Target{{ID: "a", Violation: "missing-a"}, {ID: "b", Violation: "missing-b"}}}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testInvocation(t *testing.T) *Invocation {
	t.Helper()
	i, err := Prepare(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i.Close() })
	return i
}

func reportFor(i *Invocation, d *Declaration) Report {
	return Report{Schema: ReportSchema, Invocation: i.ID, DeclarationDigest: strings.Repeat("a", 64), Producer: d.Producer, ProducerVersion: d.ProducerVersion, AttemptsComplete: true,
		Observations: []Observation{{Target: "a", Attempt: 1, Status: "violation", Violation: "missing-a"}, {Target: "b", Attempt: 1, Status: "pass"}}}
}

func processResult(code int) run.Result {
	outcome := run.Pass
	if code != 0 {
		outcome = run.Fail
	}
	return run.Result{Attempted: true, Outcome: outcome, Exit: &code}
}

func TestClassifyStructuredObservations(t *testing.T) {
	cases := []struct {
		name           string
		baseline       bool
		exit           int
		mutate         func(*Report)
		result, reason string
	}{
		{"baseline", true, 1, nil, "fail_as_expected", "declared_violation"},
		{"current-failure", false, 1, nil, "fail", "assertion_failed"},
		{"current-pass", false, 0, func(r *Report) { r.Observations[0] = Observation{Target: "a", Attempt: 1, Status: "pass"} }, "pass", "targets_passed"},
		{"baseline-pass", true, 0, func(r *Report) { r.Observations[0] = Observation{Target: "a", Attempt: 1, Status: "pass"} }, "unexpected_pass", "baseline_passed"},
		{"mixed-infrastructure", true, 1, func(r *Report) {
			r.Observations[1] = Observation{Target: "b", Attempt: 1, Status: "error", Reason: "import_error"}
		}, "error", "environment_reported"},
		{"mixed-other-assertion", true, 1, func(r *Report) {
			r.Observations[1] = Observation{Target: "b", Attempt: 1, Status: "violation", Violation: "different"}
		}, "fail", "unexpected_assertion"},
		{"skip", true, 1, func(r *Report) {
			r.Observations[1] = Observation{Target: "b", Attempt: 1, Status: "skip", Reason: "skipped"}
		}, "unknown", "target_skipped"},
		{"missing-target", true, 1, func(r *Report) { r.Observations = r.Observations[:1] }, "unknown", "target_missing"},
		{"zero-targets", true, 1, func(r *Report) { r.Observations = nil }, "unknown", "targets_empty"},
		{"wrong-target", true, 1, func(r *Report) { r.Observations[0].Target = "other" }, "unknown", "target_mismatch"},
		{"nonce-replay", true, 1, func(r *Report) { r.Invocation = strings.Repeat("0", 64) }, "unknown", "report_binding"},
		{"wrong-input", true, 1, func(r *Report) { r.DeclarationDigest = strings.Repeat("0", 64) }, "unknown", "report_binding"},
		{"wrong-producer", true, 1, func(r *Report) { r.ProducerVersion = "2" }, "unknown", "report_binding"},
		{"future-schema", true, 1, func(r *Report) { r.Schema = "check-result/99" }, "unknown", "report_schema"},
		{"unknown-status", true, 1, func(r *Report) { r.Observations[0].Status = "successful-ish" }, "unknown", "observation_invalid"},
		{"private-message", true, 1, func(r *Report) {
			r.Observations[0] = Observation{Target: "a", Attempt: 1, Status: "error", Reason: "SECRET-TOKEN"}
		}, "unknown", "observation_invalid"},
		{"retry-history-missing", true, 1, func(r *Report) { r.AttemptsComplete = false }, "unknown", "attempt_history_missing"},
		{"last-attempt-only", false, 0, func(r *Report) { r.Observations[0] = Observation{Target: "a", Attempt: 2, Status: "pass"} }, "unknown", "attempt_history_invalid"},
		{"current-retry-cannot-erase-failure", false, 0, func(r *Report) {
			r.Observations = append(r.Observations, Observation{Target: "a", Attempt: 2, Status: "pass"})
		}, "fail", "inconsistent_attempts"},
		{"baseline-retry-cannot-erase-pass", true, 1, func(r *Report) {
			r.Observations[0] = Observation{Target: "a", Attempt: 1, Status: "pass"}
			r.Observations = append(r.Observations, Observation{Target: "a", Attempt: 2, Status: "violation", Violation: "missing-a"})
		}, "fail", "inconsistent_attempts"},
		{"contradicting-exit", true, 0, nil, "unknown", "report_exit_conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, i := testDeclaration(), testInvocation(t)
			r := reportFor(i, d)
			if tc.mutate != nil {
				tc.mutate(&r)
			}
			writeJSON(t, i.Path, r)
			p := processResult(tc.exit)
			// The caller binds current inputs, never the report's claimed digest.
			s := i.Classify(d, strings.Repeat("a", 64), p, tc.baseline)
			if s.Result != tc.result || s.Reason != tc.reason {
				t.Fatalf("got %+v; want %s/%s", s, tc.result, tc.reason)
			}
			if err := ValidateStored(&s, d, strings.Repeat("a", 64), tc.baseline, s.Result, p.Exit); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(s)
			if strings.Contains(string(encoded), "SECRET") {
				t.Fatal("raw report message escaped into summary")
			}
		})
	}
}

func TestAdverseObservationsSurviveMixedResults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		baseline  bool
		violation string
		retryPass bool
		want      bool
	}{
		{"current-assertion-and-environment", false, "missing-a", false, true},
		{"baseline-expected-and-environment", true, "missing-a", false, false},
		{"baseline-other-assertion-and-environment", true, "other-assertion", false, true},
		{"baseline-retry-and-environment", true, "missing-a", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, i := testDeclaration(), testInvocation(t)
			r := reportFor(i, d)
			r.Observations[0].Violation = tc.violation
			r.Observations[1] = Observation{Target: "b", Attempt: 1, Status: "error", Reason: "setup_error"}
			if tc.retryPass {
				r.Observations = append(r.Observations, Observation{Target: "a", Attempt: 2, Status: "pass"})
			}
			writeJSON(t, i.Path, r)
			process := processResult(1)
			s := i.Classify(d, r.DeclarationDigest, process, tc.baseline)
			if s.Result != "error" || s.Reason != "environment_reported" {
				t.Fatalf("mixed environment was hidden: %+v", s)
			}
			if err := ValidateStored(&s, d, r.DeclarationDigest, tc.baseline, s.Result, process.Exit); err != nil {
				t.Fatal(err)
			}
			if got := HasAdverse(&s, d, tc.baseline); got != tc.want {
				t.Fatalf("adverse=%v, want %v: %+v", got, tc.want, s)
			}
		})
	}
}

func TestSummaryRetainsInterleavedRetryOrder(t *testing.T) {
	d, i := testDeclaration(), testInvocation(t)
	r := reportFor(i, d)
	r.Observations = []Observation{
		{Target: "b", Attempt: 1, Status: "error", Reason: "setup_error"},
		{Target: "a", Attempt: 1, Status: "violation", Violation: "missing-a"},
		{Target: "b", Attempt: 2, Status: "pass"},
		{Target: "a", Attempt: 2, Status: "pass"},
	}
	writeJSON(t, i.Path, r)
	process := processResult(1)
	s := i.Classify(d, r.DeclarationDigest, process, false)
	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var stored Summary
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStored(&stored, d, r.DeclarationDigest, false, s.Result, process.Exit); err != nil {
		t.Fatalf("serialized retry history became invalid: %v", err)
	}
	if len(stored.Targets) != 4 {
		t.Fatalf("lost attempts: %+v", stored)
	}
	for index, observation := range stored.Targets {
		wantTarget := "a"
		if index >= 2 {
			wantTarget = "b"
		}
		if observation.Target != wantTarget || observation.Attempt != index%2+1 {
			t.Fatalf("retry history reordered: %+v", stored.Targets)
		}
	}
	if !HasAdverse(&stored, d, false) {
		t.Fatal("stored retry history lost the assertion")
	}
}

func TestMissingMalformedAndUnsafeReports(t *testing.T) {
	for _, tc := range []struct{ name, content, reason string }{
		{"missing", "", "report_missing"},
		{"malformed", "{", "report_invalid_json"},
		{"duplicate", "{\"schema\":\"check-result/v1\",\"schema\":\"check-result/v1\"}", "report_invalid_json"},
		{"unknown-field", "{\"surprise\":true}", "report_invalid_json"},
		{"oversized", strings.Repeat("x", maxJSONBytes+1), "report_invalid_file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := testInvocation(t)
			if tc.content != "" {
				if err := os.WriteFile(i.Path, []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s := i.Classify(testDeclaration(), strings.Repeat("a", 64), processResult(1), true)
			if s.Result != "unknown" || s.Reason != tc.reason {
				t.Fatalf("%+v", s)
			}
		})
	}
	i := testInvocation(t)
	if err := os.Symlink("/does-not-exist", i.Path); err != nil {
		t.Fatal(err)
	}
	if got := i.Classify(testDeclaration(), strings.Repeat("a", 64), processResult(1), true); got.Reason != "report_invalid_file" {
		t.Fatalf("%+v", got)
	}
}

func TestProcessFailuresAndUnsupportedDeclaration(t *testing.T) {
	for _, p := range []run.Result{
		{Outcome: run.Error, Reason: "process_start"}, {Outcome: run.Error, Reason: "process_timeout"}, {Outcome: run.Error, Reason: "process_output"}, {Outcome: run.Fail, Signal: "15"},
	} {
		i := testInvocation(t)
		s := i.Classify(nil, "", p, true)
		if s.Result != "error" {
			t.Fatalf("%+v", s)
		}
		if err := ValidateStored(&s, nil, "", true, s.Result, p.Exit); err != nil {
			t.Fatal(err)
		}
	}
	i := testInvocation(t)
	s := i.Classify(nil, "", processResult(1), true)
	if s.Result != "unknown" || s.Reason != "unsupported_declaration" {
		t.Fatalf("%+v", s)
	}
	if err := ValidateStored(&s, nil, "", true, s.Result, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStoredSummaryRejectsChangedEvidence(t *testing.T) {
	i, d := testInvocation(t), testDeclaration()
	r := reportFor(i, d)
	writeJSON(t, i.Path, r)
	p := processResult(1)
	s := i.Classify(d, r.DeclarationDigest, p, true)
	if err := os.Remove(i.Path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStored(&s, d, r.DeclarationDigest, true, s.Result, p.Exit); err != nil {
		t.Fatal("summary depends on original report", err)
	}
	for _, mutate := range []func(*Summary){
		func(s *Summary) { s.Result = "pass" }, func(s *Summary) { s.DeclarationDigest = strings.Repeat("b", 64) }, func(s *Summary) { s.ReportDigest = "" }, func(s *Summary) { s.Targets = s.Targets[:1] }, func(s *Summary) { s.Reason = "targets_passed" },
	} {
		copy := s
		mutate(&copy)
		if ValidateStored(&copy, d, r.DeclarationDigest, true, copy.Result, p.Exit) == nil {
			t.Fatalf("accepted changed summary %+v", copy)
		}
	}
}

func TestDeclarationIdentityAndPathSafety(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	git("init", "-q")
	d := testDeclaration()
	writeJSON(t, filepath.Join(root, "declaration.json"), d)
	if err := os.WriteFile(filepath.Join(root, "producer.py"), []byte("version one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "fixture")
	_, before, err := Load(root, "declaration.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "producer.py"), []byte("version two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(root, "declaration.json"); err == nil {
		t.Fatal("accepted producer bytes differing from HEAD")
	}
	git("add", ".")
	git("commit", "-qm", "producer update")
	_, after, err := Load(root, "declaration.json")
	if err != nil || before == after {
		t.Fatalf("producer change not bound: %v", err)
	}
	d.Targets[0].Violation = "changed-meaning"
	writeJSON(t, filepath.Join(root, "declaration.json"), d)
	if _, _, err := Load(root, "declaration.json"); err == nil {
		t.Fatal("accepted declaration bytes differing from HEAD")
	}
	git("add", ".")
	git("commit", "-qm", "declaration update")
	_, changed, err := Load(root, "declaration.json")
	if err != nil || changed == after {
		t.Fatalf("declaration change not bound: %v", err)
	}
	for _, p := range []string{"../declaration.json", "/tmp/outside", ".git/config", "a/../declaration.json"} {
		if _, _, err := Load(root, p); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
	if err := os.Remove(filepath.Join(root, "producer.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "declaration.json"), filepath.Join(root, "producer.py")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(root, "declaration.json"); err == nil {
		t.Fatal("accepted producer symlink")
	}
	writeJSON(t, filepath.Join(root, "untracked.json"), d)
	if _, _, err := Load(root, "untracked.json"); err == nil {
		t.Fatal("accepted untracked declaration")
	}
}

func TestReferenceChecksActualFiles(t *testing.T) {
	root := t.TempDir()
	script, err := os.ReadFile(filepath.Join("..", "..", "tools", "check-result-reference.py"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference.py"), script, 0600); err != nil {
		t.Fatal(err)
	}
	d := &Declaration{Schema: DeclarationSchema, Producer: "jaekit-reference", ProducerVersion: "1", ProducerPaths: []string{"reference.py", "checks.json"}, Targets: []Target{{ID: "greeting", Violation: "missing-greeting"}}}
	writeJSON(t, filepath.Join(root, "declaration.json"), d)
	check := map[string]any{"target": "greeting", "path": "greeting.txt", "equals": "hello\n", "violation": "missing-greeting", "missing": "violation"}
	runCase := func(baseline bool) Summary {
		t.Helper()
		writeJSON(t, filepath.Join(root, "checks.json"), map[string]any{"checks": []any{check}})
		i := testInvocation(t)
		p, err := run.ExecWithEnv(root, []string{"python3", "reference.py", "declaration.json", "checks.json"}, 10*time.Second, filepath.Join(t.TempDir(), "log"), i.Env(strings.Repeat("a", 64)))
		if err != nil {
			t.Fatal(err)
		}
		return i.Classify(d, strings.Repeat("a", 64), p, baseline)
	}
	if got := runCase(true); got.Result != "fail_as_expected" {
		t.Fatalf("missing behavior: %+v", got)
	}
	check["requires"] = []string{"environment.txt"}
	if got := runCase(true); got.Result != "error" {
		t.Fatalf("missing environment: %+v", got)
	}
	delete(check, "requires")
	if err := os.WriteFile(filepath.Join(root, "greeting.txt"), []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := runCase(true); got.Result != "fail_as_expected" {
		t.Fatalf("old behavior: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "greeting.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := runCase(false); got.Result != "pass" {
		t.Fatalf("implemented behavior: %+v", got)
	}
	check["path"] = "."
	if got := runCase(false); got.Result != "error" || got.Reason != "environment_reported" {
		t.Fatalf("invalid reference input was not a reported setup error: %+v", got)
	}
}
