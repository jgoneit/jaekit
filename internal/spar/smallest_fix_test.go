//go:build !windows

package spar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// These tests observe the smallest fix direction a reviewer may give each
// finding (smallest_fix): what the packet asks for, which results every
// preset accepts, what the round records and shows, that no judgment reads
// it, and that rounds written without it read as they did. They drive the
// spar command line (Main) on a synthetic repository. Reviewers are native
// (the tests record their answers), command, and the claude, codex and agy
// presets with their binary pointing at a /bin/sh script that prints the
// test's answer in that CLI's output form; no reviewer CLI or model runs.
// The opening line of an assertion's failure message names the property it
// observes (jaekit:violation=<id>); a failure that starts with "setup:" is a
// broken setup, not an observation.

const sfDirection = "Drop the comma only when the name is empty."

type sfWorld struct {
	t          *testing.T
	dir, repo  string
	base, head string
	criteria   string
	rules      string
	config     string
	configs    int
	files      int
}

// newSFWorld makes a repository whose head adds a trailing comma to a
// greeting, with a check file under checks/ and a rule document. Without
// git, a check of a change condition is skipped and a check of a kept
// behavior fails, so neither passes.
func newSFWorld(t *testing.T, keeps bool) *sfWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if keeps {
			t.Fatalf("setup: git is not available: %v", err)
		}
		t.Skipf("git is not available: %v", err)
	}
	w := &sfWorld{t: t, dir: t.TempDir()}
	w.repo = realish(filepath.Join(w.dir, "repo"))
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n")
	w.rules = w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\n- A greeting never ends with a comma.\n")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello ' + name\n")
	w.write(filepath.Join(w.repo, "checks", "test_greet.py"), "def test_greet():\n    assert True\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello, ' + name\n")
	w.write(filepath.Join(w.repo, "checks", "test_greet.py"), "def test_greet():\n    assert greet('a') == 'hello, a'\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "# Goal\n\n- **AC-1** greet('') returns 'hello'.\n")
	return w
}

func (w *sfWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return path
}

func (w *sfWorld) read(path string) []byte {
	w.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return data
}

func (w *sfWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("setup: git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// use makes the given reviewer entries the user's configuration of later
// runs.
func (w *sfWorld) use(reviewers ...string) {
	w.t.Helper()
	w.configs++
	w.config = w.write(filepath.Join(w.dir, fmt.Sprintf("spar-%d.toml", w.configs)), strings.Join(reviewers, ""))
}

// spar runs the spar command line and returns its exit code and output. An
// exit outside want is a broken setup.
func (w *sfWorld) spar(want []int, args ...string) (int, string) {
	w.t.Helper()
	getenv := func(key string) string {
		switch key {
		case "SPAR_CONFIG":
			return w.config
		case "HOME":
			return w.dir
		case "XDG_CONFIG_HOME":
			return filepath.Join(w.dir, "xdg")
		}
		return ""
	}
	var out, errs bytes.Buffer
	code := Main("test", args, strings.NewReader(""), &out, &errs, getenv)
	for _, ok := range want {
		if code == ok {
			return code, out.String()
		}
	}
	w.t.Fatalf("setup: spar %v: exit %d, want one of %v: %s%s", args, code, want, out.String(), errs.String())
	return code, ""
}

// run starts a candidate round of the configured reviewers.
func (w *sfWorld) run(out string, extra ...string) string {
	w.t.Helper()
	args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--criteria", w.criteria,
		"--rules", w.rules, "--check-path", "checks/**", "--out", out, "--repo", w.repo}, extra...)
	w.spar([]int{ExitOK, ExitNoResult}, args...)
	return out
}

// answer is a reviewer's result for the world's head: every angle checked
// and the given findings.
func (w *sfWorld) answer(findings ...map[string]any) map[string]any {
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	list := []any{}
	for _, f := range findings {
		list = append(list, f)
	}
	return map[string]any{"reviewed_head": w.head, "verdict": "blocking_findings", "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": list, "limits": "synthetic reviewer"}
}

// resultFile writes a result as one line of JSON, the form the fake CLIs
// print.
func (w *sfWorld) resultFile(result map[string]any) string {
	w.t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	w.files++
	return w.write(filepath.Join(w.dir, "results", fmt.Sprintf("%d.json", w.files)), string(data))
}

// record records a native reviewer's result in a round.
func (w *sfWorld) record(round, name string, result map[string]any) {
	w.t.Helper()
	w.spar([]int{ExitOK, ExitNoResult}, "record", round, name, "--result", w.resultFile(result))
}

// sfFinding is a reviewer's finding with the given fields changed; a nil
// value removes the key.
func sfFinding(id string, values map[string]any) map[string]any {
	f := map[string]any{"id": id, "proposed_class": "defect", "angle": "requirement-counterexample",
		"location": "head/app/greet.py:2", "claim": "synthetic claim " + id,
		"failure_scenario": "greet('') returns 'hello, '", "evidence": "head/app/greet.py:2",
		"link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes", "regression": "no",
		"check_impact": "none", "confidence": "high"}
	for k, v := range values {
		if v == nil {
			delete(f, k)
		} else {
			f[k] = v
		}
	}
	return f
}

func sfQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// fakes writes /bin/sh scripts for the claude, codex and agy presets that
// print the result in result in that CLI's output form, and returns the
// configuration entries of those reviewers and a command reviewer.
func (w *sfWorld) fakes(tag string, result map[string]any) []string {
	w.t.Helper()
	file := sfQuote(w.resultFile(result))
	version := "if [ \"$1\" = --version ]; then echo \"%s 0.0.0 (synthetic)\"; exit 0; fi\n"
	scripts := map[string]string{
		"claude": fmt.Sprintf(version, "claude") + "cat >/dev/null\n" +
			"printf '%s\\n' '{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"synthetic\",\"tools\":[\"Read\",\"Grep\",\"Glob\"],\"mcp_servers\":[]}'\n" +
			"printf '%s' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"structured_output\":'\n" +
			"cat " + file + "\nprintf '%s\\n' '}'\n",
		"codex": fmt.Sprintf(version, "codex") + "cat >/dev/null\nout=\n" +
			"while [ $# -gt 0 ]; do if [ \"$1\" = -o ]; then out=$2; fi; shift; done\n" +
			"cat " + file + " > \"$out\"\n" +
			"printf '%s\\n' '{\"type\":\"thread.started\"}' '{\"type\":\"turn.completed\",\"usage\":{}}'\n",
		"agy": fmt.Sprintf(version, "agy") +
			"printf '%s\\n' '{\"init\":{\"model\":\"synthetic\"}}'\n" +
			"printf '%s' '{\"result\":{\"status\":\"success\",\"structured_output\":'\n" +
			"cat " + file + "\nprintf '%s\\n' '}}'\n",
	}
	var entries []string
	for _, preset := range []string{"claude", "codex", "agy"} {
		path := w.write(filepath.Join(w.dir, "bin-"+tag, preset), "#!/bin/sh\n"+scripts[preset])
		if err := os.Chmod(path, 0o755); err != nil {
			w.t.Fatalf("setup: %v", err)
		}
		entries = append(entries, fmt.Sprintf("[[reviewers]]\nname = %q\npreset = %q\nbinary = %q\nfallback_model = \"\"\n\n",
			preset, preset, path))
	}
	entries = append(entries, fmt.Sprintf("[[reviewers]]\nname = \"cmd\"\npreset = \"command\"\nargv = [\"/bin/cat\", %q]\n\n",
		w.resultFile(result)))
	return entries
}

func sfNative(name string) string {
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = \"native\"\n\n", name)
}

// reviewers reads each reviewer's status and reason from round.json.
func (w *sfWorld) reviewers(round string) map[string][2]string {
	w.t.Helper()
	var rec struct {
		Reviewers map[string]struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"reviewers"`
	}
	if err := json.Unmarshal(w.read(filepath.Join(round, "round.json")), &rec); err != nil {
		w.t.Fatalf("setup: round.json: %v", err)
	}
	out := map[string][2]string{}
	for name, st := range rec.Reviewers {
		out[name] = [2]string{st.Status, st.Reason}
	}
	return out
}

// findings reads findings.json by finding ID.
func (w *sfWorld) findings(round string) map[string]map[string]any {
	w.t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(w.read(filepath.Join(round, "findings.json")), &list); err != nil {
		w.t.Fatalf("setup: findings.json: %v", err)
	}
	out := map[string]map[string]any{}
	for _, f := range list {
		id, _ := f["id"].(string)
		out[id] = f
	}
	return out
}

// ledger reads ledger.jsonl without its recording times.
func (w *sfWorld) ledger(round string) []map[string]any {
	w.t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(string(w.read(filepath.Join(round, "ledger.jsonl"))), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			w.t.Fatalf("setup: ledger.jsonl: %v", err)
		}
		delete(e, "recorded_at")
		out = append(out, e)
	}
	return out
}

// AC-1: the packet asks every finding for the smallest change that would
// make its failure scenario come out right, as a direction and not a patch,
// and says it may be left empty; the result schema carries it as a string.
func TestSparSmallestFixAC1_PacketAsksForDirection(t *testing.T) {
	const v = "smallest-fix-not-asked"
	w := newSFWorld(t, false)
	w.use(sfNative("host"))
	round := w.run(filepath.Join(w.repo, "reviews", "ac1", "01-candidate"), "--dry-run")
	var schema struct {
		Properties struct {
			Findings struct {
				Items struct {
					Required   []string                  `json:"required"`
					Properties map[string]map[string]any `json:"properties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(w.read(filepath.Join(round, "schema.json")), &schema); err != nil {
		t.Fatalf("setup: schema.json: %v", err)
	}
	item := schema.Properties.Findings.Items
	if p, ok := item.Properties["smallest_fix"]; !ok || !reflect.DeepEqual(p, map[string]any{"type": "string"}) {
		t.Errorf("jaekit:violation=%s the result schema has no string smallest_fix for a finding: %v", v, p)
	}
	// A provider that enforces the schema strictly needs every property
	// required; an empty string stays valid.
	required := false
	for _, k := range item.Required {
		required = required || k == "smallest_fix"
	}
	if !required {
		t.Errorf("jaekit:violation=%s the result schema does not list smallest_fix in required: %v", v, item.Required)
	}
	packet := string(w.read(filepath.Join(round, "packet.md")))
	for _, want := range []string{
		"`smallest_fix` is the smallest change that would make the `failure_scenario` come out right",
		"a direction in words, never a patch",
		"Leave it empty when you cannot name one",
	} {
		if !strings.Contains(packet, want) {
			t.Errorf("jaekit:violation=%s packet.md does not say %q", v, want)
		}
	}
}

// AC-2: every preset's result is accepted with a string, an empty string or
// no smallest_fix; any other value is invalid output naming the key; the
// other keys of a finding are checked as before.
func TestSparSmallestFixAC2_EveryPresetAcceptsDirection(t *testing.T) {
	const v = "smallest-fix-not-accepted"
	w := newSFWorld(t, false)
	presets := []string{"host", "cmd", "claude", "codex", "agy"}
	cases := []struct {
		name  string
		value any // nil: no key
		ok    bool
	}{
		{"direction", sfDirection, true},
		{"empty", "", true},
		{"missing", nil, true},
		{"number", 5, false},
	}
	for _, c := range cases {
		result := w.answer(sfFinding("F1", map[string]any{"smallest_fix": c.value}))
		w.use(append(w.fakes(c.name, result), sfNative("host"))...)
		round := w.run(filepath.Join(w.repo, "reviews", "ac2-"+c.name, "01-candidate"))
		w.record(round, "host", result)
		states := w.reviewers(round)
		for _, name := range presets {
			st := states[name]
			switch {
			case c.ok && st[0] != StatusCompleted:
				t.Errorf("jaekit:violation=%s %s: a finding with %s smallest_fix is %s, not completed: %s", v, name, c.name, st[0], st[1])
			case !c.ok && (st[0] != StatusInvalidOutput || !strings.Contains(st[1], "smallest_fix")):
				t.Errorf("jaekit:violation=%s %s: a finding whose smallest_fix is a %s is %s (%s), not invalid output naming smallest_fix",
					v, name, c.name, st[0], st[1])
			}
		}
	}
	// The other values that are not strings, and the other keys, through the
	// native reviewer's result.
	type nativeCase struct {
		name    string
		finding map[string]any
		reason  string
	}
	// A nil value removes a key in sfFinding, so null is set afterwards.
	null := sfFinding("F1", nil)
	null["smallest_fix"] = nil
	natives := []nativeCase{
		{"null", null, "smallest_fix"},
		{"bool", sfFinding("F1", map[string]any{"smallest_fix": true}), "smallest_fix"},
		{"list", sfFinding("F1", map[string]any{"smallest_fix": []any{"a"}}), "smallest_fix"},
		{"object", sfFinding("F1", map[string]any{"smallest_fix": map[string]any{"a": "b"}}), "smallest_fix"},
		{"unknown", sfFinding("F1", map[string]any{"smallest_fix": sfDirection, "extra": "x"}), `unknown key "extra"`},
		{"noclaim", sfFinding("F1", map[string]any{"smallest_fix": sfDirection, "claim": nil}), "lacks claim"},
		{"claimnumber", sfFinding("F1", map[string]any{"smallest_fix": sfDirection, "claim": 5}), "lacks claim"},
		{"blankscenario", sfFinding("F1", map[string]any{"smallest_fix": sfDirection, "failure_scenario": "  "}), "no failure scenario"},
		{"badenum", sfFinding("F1", map[string]any{"smallest_fix": sfDirection, "confidence": "certain"}), "confidence"},
	}
	var entries []string
	for _, n := range natives {
		entries = append(entries, sfNative("n-"+n.name))
	}
	w.use(entries...)
	round := w.run(filepath.Join(w.repo, "reviews", "ac2-native", "01-candidate"))
	for _, n := range natives {
		w.record(round, "n-"+n.name, w.answer(n.finding))
	}
	states := w.reviewers(round)
	for _, n := range natives {
		st := states["n-"+n.name]
		if st[0] != StatusInvalidOutput || !strings.Contains(st[1], n.reason) {
			t.Errorf("jaekit:violation=%s native %s: the result is %s (%s), not invalid output naming %s", v, n.name, st[0], st[1], n.reason)
		}
	}
}

// AC-3: findings.json keeps each finding's direction as the reviewer gave
// it, an empty string when there is none, and summary.md shows the
// directions of the findings that have one.
func TestSparSmallestFixAC3_RoundKeepsDirection(t *testing.T) {
	const v = "smallest-fix-not-kept"
	w := newSFWorld(t, false)
	multi := "Check the name first.\nThen drop the comma | keep `greet` as it is."
	claude := w.answer(
		sfFinding("F1", map[string]any{"smallest_fix": multi}),
		sfFinding("F2", nil),
	)
	host := w.answer(
		sfFinding("F1", map[string]any{"smallest_fix": sfDirection}),
		sfFinding("F2", map[string]any{"smallest_fix": ""}),
		sfFinding("F3", nil),
		sfFinding("F4", map[string]any{"smallest_fix": "   "}),
	)
	fakes := w.fakes("ac3", claude)
	w.use(fakes[0], sfNative("host"))
	round := w.run(filepath.Join(w.repo, "reviews", "ac3", "01-candidate"))
	w.record(round, "host", host)
	findings := w.findings(round)
	for id, want := range map[string]string{"claude-F1": multi, "claude-F2": "", "host-F1": sfDirection, "host-F2": "",
		"host-F3": "", "host-F4": "   "} {
		f, ok := findings[id]
		if !ok {
			t.Errorf("jaekit:violation=%s findings.json has no finding %s", v, id)
			continue
		}
		if got, present := f["smallest_fix"]; !present || got != want {
			t.Errorf("jaekit:violation=%s findings.json keeps smallest_fix %v (present %v) for %s, want %q", v, got, present, id, want)
		}
	}
	summary := string(w.read(filepath.Join(round, "summary.md")))
	_, section, found := strings.Cut(summary, "\n## Smallest fixes\n\n")
	if !found {
		t.Fatalf("jaekit:violation=%s summary.md has no smallest fix section:\n%s", v, summary)
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var items []string
	for _, line := range strings.Split(strings.TrimRight(section, "\n"), "\n") {
		if strings.HasPrefix(line, "  ") && len(items) > 0 {
			items[len(items)-1] += "\n" + line
		} else {
			items = append(items, line)
		}
	}
	sort.Strings(items)
	want := []string{
		"- claude-F1: Check the name first.\n  Then drop the comma | keep `greet` as it is.",
		"- host-F1: " + sfDirection,
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("jaekit:violation=%s summary.md shows the directions %q, want %q", v, items, want)
	}
}

// sfJudged are findings that differ in every class rule's input: a false
// pass in a check path, a check-path edge case, a linked requirement
// question, a defect proposal without a link that exists, a linked defect,
// an unreadable location and a rule link.
func sfJudged(direction bool) []map[string]any {
	list := []map[string]any{
		sfFinding("F1", map[string]any{"location": "head/checks/test_greet.py:2", "check_impact": "false_pass", "link_kind": "none", "link": ""}),
		sfFinding("F2", map[string]any{"location": "head/checks/test_greet.py:1", "link_kind": "none", "link": "", "proposed_class": "hardening"}),
		sfFinding("F3", map[string]any{"proposed_class": "requirement-question"}),
		sfFinding("F4", map[string]any{"link": "AC-9"}),
		sfFinding("F5", nil),
		sfFinding("F6", map[string]any{"location": "somewhere in the code", "check_impact": "false_pass", "proposed_class": "hardening"}),
		sfFinding("F7", map[string]any{"location": "head/app/greet.py:1", "link_kind": "rule", "link": "AGENTS.md:3"}),
	}
	if direction {
		for i, f := range list {
			f["smallest_fix"] = fmt.Sprintf("synthetic direction %d", i+1)
		}
	}
	return list
}

// AC-4: findings whose other fields are the same get the same class, link,
// automatic deferral, disposition results and gate, with a direction or
// without one.
func TestSparSmallestFixAC4_JudgmentIgnoresDirection(t *testing.T) {
	const v = "smallest-fix-changes-judgment"
	w := newSFWorld(t, false)
	w.use(sfNative("host"))
	type seriesResult struct {
		findings map[string]map[string]any
		auto     []map[string]any
		triage   []int
		ledger   []map[string]any
		gateCode int
		gate     map[string]any
	}
	steps := [][]string{
		{"host-F1", "defer", "--to", "later"},
		{"host-F1", "reject", "--evidence", "synthetic evidence"},
		{"host-F3", "answer"},
		{"host-F3", "answer", "--user-quote", "synthetic answer"},
		{"host-F5", "duplicate", "--of", "host-F4"},
		{"host-F5", "duplicate", "--of", "host-F7"},
		{"host-F6", "reject", "--evidence", "synthetic evidence"},
		{"host-F2", "reject", "--evidence", "synthetic evidence"},
	}
	run := func(name string, direction bool) seriesResult {
		series := filepath.Join(w.repo, "reviews", name)
		round := w.run(filepath.Join(series, "01-candidate"))
		w.record(round, "host", w.answer(sfJudged(direction)...))
		var r seriesResult
		r.findings = w.findings(round)
		for id := range r.findings {
			delete(r.findings[id], "smallest_fix")
		}
		r.auto = w.ledger(round)
		for _, step := range steps {
			code, _ := w.spar([]int{ExitOK, ExitRefused}, append([]string{"triage", round}, step...)...)
			r.triage = append(r.triage, code)
		}
		r.ledger = w.ledger(round)
		gateFile := filepath.Join(series, "gate.json")
		r.gateCode, _ = w.spar([]int{ExitOK, ExitDegraded, ExitBlocked}, "gate", round, "--expect-head", w.head,
			"--out", gateFile, "--repo", w.repo)
		var gate map[string]any
		if err := json.Unmarshal([]byte(strings.ReplaceAll(string(w.read(gateFile)), series, "<series>")), &gate); err != nil {
			t.Fatalf("setup: gate.json: %v", err)
		}
		r.gate = map[string]any{}
		for _, key := range []string{"state", "reasons", "open", "open_findings", "auto_deferred", "growth"} {
			r.gate[key] = gate[key]
		}
		return r
	}
	plain := run("plain", false)
	with := run("with", true)
	if len(plain.findings) != 7 {
		t.Fatalf("setup: the series without directions recorded %d findings, want 7", len(plain.findings))
	}
	for id, p := range plain.findings {
		if got := with.findings[id]; !reflect.DeepEqual(got, p) {
			t.Errorf("jaekit:violation=%s %s is recorded as %v with a direction, %v without one", v, id, got, p)
		}
	}
	if !reflect.DeepEqual(with.auto, plain.auto) {
		t.Errorf("jaekit:violation=%s the automatic deferrals are %v with directions, %v without", v, with.auto, plain.auto)
	}
	if !reflect.DeepEqual(with.triage, plain.triage) {
		t.Errorf("jaekit:violation=%s the dispositions end %v with directions, %v without", v, with.triage, plain.triage)
	}
	if !reflect.DeepEqual(with.ledger, plain.ledger) {
		t.Errorf("jaekit:violation=%s the ledgers are %v with directions, %v without", v, with.ledger, plain.ledger)
	}
	if with.gateCode != plain.gateCode || !reflect.DeepEqual(with.gate, plain.gate) {
		t.Errorf("jaekit:violation=%s the gate is %d %v with directions, %d %v without", v, with.gateCode, with.gate, plain.gateCode, plain.gate)
	}
}

// sfOldFindings is the findings part of summary.md as spar wrote it before
// smallest_fix, for the round AC-6 records and triages.
const sfOldFindings = `## Findings

defect 1, requirement-question 1, hardening 1, note 0.

| ID | Class | Link | Disposition | Location | Claim |
| --- | --- | --- | --- | --- | --- |
| host-F1 | defect | criterion AC-1 | reject | head/app/greet.py:2 | synthetic claim F1 |
| host-F2 | hardening | none | defer (auto) | head/checks/test_greet.py:1 | synthetic claim F2 |
| host-F3 | requirement-question | criterion AC-1 | answer | head/app/greet.py:2 | synthetic claim F3 |
`

// AC-6: a round written before smallest_fix existed, whose findings.json has
// no such key, gives the summary, dispositions and gate it gave before.
func TestSparSmallestFixAC6_OldRoundsReadTheSame(t *testing.T) {
	w := newSFWorld(t, true)
	w.use(sfNative("host"))
	series := filepath.Join(w.repo, "reviews", "old")
	round := w.run(filepath.Join(series, "01-candidate"))
	w.record(round, "host", w.answer(
		sfFinding("F1", nil),
		sfFinding("F2", map[string]any{"location": "head/checks/test_greet.py:1", "link_kind": "none", "link": "", "proposed_class": "hardening"}),
		sfFinding("F3", map[string]any{"proposed_class": "requirement-question"}),
	))
	// The findings as spar wrote them before smallest_fix: without the key.
	var list []map[string]any
	if err := json.Unmarshal(w.read(filepath.Join(round, "findings.json")), &list); err != nil {
		t.Fatalf("setup: findings.json: %v", err)
	}
	for _, f := range list {
		delete(f, "smallest_fix")
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	w.write(filepath.Join(round, "findings.json"), string(data)+"\n")

	for _, step := range []struct {
		args []string
		want int
	}{
		{[]string{"host-F1", "reject", "--evidence", "synthetic evidence"}, ExitOK},
		{[]string{"host-F3", "defer", "--to", "later"}, ExitRefused},
		{[]string{"host-F3", "answer", "--user-quote", "synthetic answer"}, ExitOK},
	} {
		if code, _ := w.spar([]int{ExitOK, ExitRefused}, append([]string{"triage", round}, step.args...)...); code != step.want {
			t.Errorf("AC-6: spar triage %v on a round without smallest_fix exits %d, want %d", step.args, code, step.want)
		}
	}
	summary := string(w.read(filepath.Join(round, "summary.md")))
	if _, part, _ := strings.Cut(summary, "\n## Findings\n"); "## Findings\n"+part != sfOldFindings {
		t.Errorf("AC-6: summary.md of a round without smallest_fix ends\n%s\nwant\n%s", "## Findings\n"+part, sfOldFindings)
	}
	if strings.Contains(summary, "## Smallest fixes") {
		t.Errorf("AC-6: summary.md of a round without smallest_fix shows a smallest fix section:\n%s", summary)
	}
	if _, out := w.spar([]int{ExitOK}, "summary", round); out != summary {
		t.Errorf("AC-6: spar summary prints\n%s\nnot the round's summary.md\n%s", out, summary)
	}
	gateFile := filepath.Join(series, "gate.json")
	code, _ := w.spar([]int{ExitOK, ExitDegraded, ExitBlocked}, "gate", round, "--expect-head", w.head, "--out", gateFile, "--repo", w.repo)
	var gate struct {
		State        string         `json:"state"`
		Open         map[string]int `json:"open"`
		AutoDeferred int            `json:"auto_deferred"`
	}
	if err := json.Unmarshal(w.read(gateFile), &gate); err != nil {
		t.Fatalf("setup: gate.json: %v", err)
	}
	if code != ExitOK || gate.State != StateComplete || gate.Open["defect"] != 0 || gate.Open["requirement-question"] != 0 || gate.AutoDeferred != 1 {
		t.Errorf("AC-6: the gate of a round without smallest_fix is %d %s, open %v, auto-deferred %d; want 0 complete, nothing open, 1",
			code, gate.State, gate.Open, gate.AutoDeferred)
	}
}
