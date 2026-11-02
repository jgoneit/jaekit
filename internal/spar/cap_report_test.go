//go:build !windows

package spar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Checks of the goal spar-cap-report: the report of a series' code rounds
// (spar cap-report), the cause a code cap extension records (spar extend
// --cause, --reason) and the code cap refusal that names the report. They
// drive the spar command line (Main) on a synthetic repository whose rounds
// have native reviewers that the tests answer with spar record; no reviewer
// CLI or model runs. The opening line of an assertion's failure message
// names the property it observes (jaekit:violation=<id>); a failure that
// starts with "setup:" is a broken setup, not an observation.

const (
	crChurnLimit     = "Counted by file: a finding in a file the fixes changed counts even when it is about another line."
	crRecurringLimit = "Counted by link: the same problem reported under different links is not grouped."
	crUnrelatedLimit = "Uses each reviewer's introduced_by_change as reported."
)

type crWorld struct {
	t        *testing.T
	dir      string
	repo     string
	commit   map[string]string
	criteria string
	rules    string
	config   string
	files    int
}

// newCRWorld makes a repository with four commits: base, head1 (the
// change), head2 (a fix that changes one line of app/greet.py and adds three
// lines to the check file) and head3 (a later change to app/other.py).
func newCRWorld(t *testing.T) *crWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("setup: git is not available: %v", err)
	}
	w := &crWorld{t: t, dir: t.TempDir(), commit: map[string]string{}}
	w.repo = realish(filepath.Join(w.dir, "repo"))
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n")
	w.rules = w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\n- A greeting never ends with a comma.\n")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello ' + name\n")
	w.write(filepath.Join(w.repo, "app", "other.py"), "def other():\n    return 1\n")
	w.write(filepath.Join(w.repo, "checks", "test_greet.py"), "def test_greet():\n    assert True\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.commit["base"] = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello, ' + name\n")
	w.git("commit", "-qam", "head1")
	w.commit["head1"] = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello, ' + name if name else 'hello'\n")
	w.write(filepath.Join(w.repo, "checks", "test_greet.py"),
		"def test_greet():\n    assert True\n\ndef test_empty():\n    assert greet('') == 'hello'\n")
	w.git("commit", "-qam", "head2")
	w.commit["head2"] = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "other.py"), "def other():\n    return 2\n")
	w.git("commit", "-qam", "head3")
	w.commit["head3"] = w.git("rev-parse", "HEAD")
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "# Goal\n\n## Acceptance Criteria\n"+
		"- **AC-1** greet('') returns 'hello'.\n- **AC-2** other() returns 1.\n- **AC-3** greet never fails.\n- **AC-4** Names stay as given.\n")
	w.use("host")
	return w
}

func (w *crWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return path
}

func (w *crWorld) read(path string) []byte {
	w.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return data
}

func (w *crWorld) git(args ...string) string {
	w.t.Helper()
	return crGit(w.t, w.repo, args...)
}

func crGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup: git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// use makes native reviewers with the given names the user's configuration
// of later commands.
func (w *crWorld) use(names ...string) {
	w.t.Helper()
	var body strings.Builder
	for _, n := range names {
		fmt.Fprintf(&body, "[[reviewers]]\nname = %q\npreset = \"native\"\n\n", n)
	}
	w.files++
	w.config = w.write(filepath.Join(w.dir, fmt.Sprintf("spar-%d.toml", w.files)), body.String())
}

// spar runs the spar command line and returns its exit code, standard
// output and standard error.
func (w *crWorld) spar(args ...string) (int, string, string) {
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
	return code, out.String(), errs.String()
}

// must runs spar for the setup; any other exit than want is a broken setup.
func (w *crWorld) must(want int, args ...string) {
	w.t.Helper()
	if code, out, errs := w.spar(args...); code != want {
		w.t.Fatalf("setup: spar %v: exit %d, want %d: %s%s", args, code, want, out, errs)
	}
}

// run starts a round of the configured reviewers in series/name.
func (w *crWorld) run(series, name, stage, base, head string, extra ...string) string {
	w.t.Helper()
	out := filepath.Join(series, name)
	w.must(ExitOK, append([]string{"run", "--stage", stage, "--base", w.commit[base], "--head", w.commit[head],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", out, "--repo", w.repo}, extra...)...)
	return out
}

// record answers a native reviewer of a round whose head is head.
func (w *crWorld) record(round, reviewer, head string, findings ...map[string]any) {
	w.t.Helper()
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	list := []any{}
	for _, f := range findings {
		list = append(list, f)
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": w.commit[head], "verdict": "blocking_findings",
		"angles_checked": checked, "angles_unchecked": []any{}, "findings": list, "limits": "synthetic reviewer"})
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	w.files++
	file := w.write(filepath.Join(w.dir, "results", fmt.Sprintf("%d.json", w.files)), string(data))
	w.must(ExitOK, "record", round, reviewer, "--result", file)
}

func (w *crWorld) triage(round, finding, disposition string, args ...string) {
	w.t.Helper()
	w.must(ExitOK, append([]string{"triage", round, finding, disposition}, args...)...)
}

// crFinding is a reviewer's finding of the given proposed class at line 1
// of path in the reviewed head, linked to a criterion when link is set.
func crFinding(id, class, link, path, introduced string) map[string]any {
	kind := "criterion"
	if link == "" {
		kind = "none"
	}
	return map[string]any{"id": id, "proposed_class": class, "angle": "requirement-counterexample",
		"location": "head/" + path + ":1", "claim": "synthetic claim " + id, "failure_scenario": "synthetic failure " + id,
		"evidence": "head/" + path + ":1", "link_kind": kind, "link": link, "introduced_by_change": introduced,
		"regression": "no", "check_impact": "none", "confidence": "high"}
}

// mainSeries writes three counted code rounds (candidate, fixes, cumulative
// candidate) with the findings and dispositions of REVIEW.md "AC-2 series".
func (w *crWorld) mainSeries(name string) string {
	w.t.Helper()
	series := filepath.Join(w.repo, "reviews", name)
	r1 := w.run(series, "01-candidate", "candidate", "base", "head1")
	w.record(r1, "host", "head1",
		crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"),
		crFinding("F2", "defect", "AC-2", "app/other.py", "no"),
		crFinding("F3", "hardening", "", "checks/test_greet.py", "yes"),
		crFinding("F4", "defect", "AC-1", "app/greet.py", "unknown"))
	w.triage(r1, "host-F1", "fix", "--commit", w.commit["head2"], "--failed-before", "synthetic check before",
		"--passed-after", "synthetic check after")
	w.triage(r1, "host-F4", "reject", "--evidence", "synthetic evidence")
	r2 := w.run(series, "02-fixes", "fixes", "head1", "head2")
	w.record(r2, "host", "head2",
		crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"),
		crFinding("F2", "note", "", "app/other.py", "no"),
		crFinding("F3", "defect", "AC-4", "app/gone.py", "yes")) // a location no copy has
	r3 := w.run(series, "03-candidate", "candidate", "base", "head2")
	w.record(r3, "host", "head2",
		crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"),
		crFinding("F2", "defect", "AC-3", "app/other.py", "unknown"),
		crFinding("F3", "requirement-question", "AC-1", "app/greet.py", "yes"),
		crFinding("F4", "defect", "AC-2", "app/greet.py", "yes"))
	w.triage(r3, "host-F4", "duplicate", "--of", "host-F1")
	return series
}

// report runs spar cap-report on a series of this world's repository.
func (w *crWorld) report(series string, extra ...string) (int, string, string) {
	w.t.Helper()
	return w.spar(append([]string{"cap-report", series, "--repo", w.repo}, extra...)...)
}

// reportJSON runs spar cap-report --format json and decodes it; an exit
// other than 0 or output that is not one JSON object is the violation v.
func (w *crWorld) reportJSON(v, series string) map[string]any {
	w.t.Helper()
	code, out, errs := w.report(series, "--format", "json")
	if code != ExitOK {
		w.t.Fatalf("jaekit:violation=%s spar cap-report %s --format json exits %d: %s", v, series, code, errs)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		w.t.Fatalf("jaekit:violation=%s spar cap-report --format json printed no JSON object: %v\n%s", v, err, out)
	}
	return doc
}

// crSame compares a decoded JSON value with a Go value through JSON.
func crSame(got, want any) bool {
	g, _ := json.Marshal(got)
	var a, b any
	_ = json.Unmarshal(g, &a)
	wb, _ := json.Marshal(want)
	_ = json.Unmarshal(wb, &b)
	return reflect.DeepEqual(a, b)
}

func crExpect(t *testing.T, v, what string, got, want any) {
	t.Helper()
	if !crSame(got, want) {
		g, _ := json.Marshal(got)
		wb, _ := json.Marshal(want)
		t.Errorf("jaekit:violation=%s %s is %s, want %s", v, what, g, wb)
	}
}

// extensions reads a series' extensions.jsonl without recording times, or
// nil when the file does not exist; every line must have a recording time.
func (w *crWorld) extensions(v, series string) []map[string]any {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(series, "extensions.jsonl"))
	if err != nil {
		return nil
	}
	var out []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			w.t.Fatalf("setup: extensions.jsonl: %v", err)
		}
		if s, _ := e["recorded_at"].(string); s == "" {
			w.t.Errorf("jaekit:violation=%s an extension line has no recording time: %s", v, line)
		}
		delete(e, "recorded_at")
		out = append(out, e)
	}
	return out
}

// AC-1: when the code cap stops spar run, the refusal still says what it
// said and adds the cap-report command for that series; the plan and
// plan-delta refusals stay as they are.
func TestSparCapReportAC1_CodeCapRefusalNamesReport(t *testing.T) {
	const v = "cap-refusal-no-report"
	w := newCRWorld(t)
	series := w.mainSeries("ac1")
	code, _, errs := w.spar("run", "--stage", "fixes", "--base", w.commit["head2"], "--head", w.commit["head3"],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", filepath.Join(series, "04-fixes"),
		"--repo", w.repo)
	if code != ExitRefused {
		t.Fatalf("setup: a fourth code round exits %d, want the cap refusal 65: %s", code, errs)
	}
	for _, want := range []string{
		"the code cap is reached in " + series + ": 3 of 3 rounds ran; only reviewers that did not complete the same packet may run again",
		"`spar extend " + series + " --stage code --by N --user-quote ...`",
		"`spar cap-report " + series + "`",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("jaekit:violation=%s the code cap refusal does not say %q:\n%s", v, want, errs)
		}
	}
	if _, err := os.Stat(filepath.Join(series, "04-fixes")); err == nil {
		t.Errorf("jaekit:violation=%s the refused code round left a round folder", v)
	}
	for _, stage := range []string{"plan", "plan-delta"} {
		s := filepath.Join(w.repo, "reviews", "ac1-"+stage)
		w.run(s, "01-"+stage, stage, "base", "head1")
		code, _, errs := w.spar("run", "--stage", stage, "--base", w.commit["base"], "--head", w.commit["head2"],
			"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", filepath.Join(s, "02-"+stage),
			"--repo", w.repo)
		if code != ExitRefused {
			t.Fatalf("setup: a second %s round exits %d, want the cap refusal 65: %s", stage, code, errs)
		}
		want := fmt.Sprintf("spar: the %s cap is reached in %s: 1 of 1 rounds ran; only reviewers that did not complete the same packet "+
			"may run again, and more rounds need the user's words recorded with `spar extend %s --stage %s --by N --user-quote ...`\n",
			stage, s, s, stage)
		if errs != want {
			t.Errorf("jaekit:violation=%s the %s cap refusal changed:\n%s\nwant:\n%s", v, stage, errs, want)
		}
	}
}

// AC-2: the report of a series with three counted code rounds shows the cap
// use, each round, the open findings and the four signals as REVIEW.md
// "AC-2 series" works them out; a second series shows that a rerun of the
// same packet is one round for recurring links and that a dry run and a
// round stopped before its findings are left out.
func TestSparCapReportAC2_SignalsFromRecords(t *testing.T) {
	const v = "cap-report-signals-missing"
	w := newCRWorld(t)
	series := w.mainSeries("ac2")
	doc := w.reportJSON(v, series)
	b, h1, h2 := w.commit["base"], w.commit["head1"], w.commit["head2"]
	crExpect(t, v, "schema", doc["schema"], "spar-cap-report/v1")
	crExpect(t, v, "series", doc["series"], series)
	crExpect(t, v, "cap", doc["cap"], map[string]any{"code": 3, "extensions": 0, "allowed": 3, "counted": 3})
	crExpect(t, v, "rounds", doc["rounds"], []any{
		map[string]any{"round": "01-candidate", "stage": "candidate", "sequence": 1, "base": b, "head": h1, "counted": true,
			"classes": map[string]any{"defect": 3, "requirement-question": 0, "hardening": 1, "note": 0}},
		map[string]any{"round": "02-fixes", "stage": "fixes", "sequence": 2, "base": h1, "head": h2, "counted": true,
			"classes": map[string]any{"defect": 2, "requirement-question": 0, "hardening": 0, "note": 1}},
		map[string]any{"round": "03-candidate", "stage": "candidate", "sequence": 3, "base": b, "head": h2, "counted": true,
			"classes": map[string]any{"defect": 3, "requirement-question": 1, "hardening": 0, "note": 0}},
	})
	crExpect(t, v, "skipped", doc["skipped"], []any{})
	crExpect(t, v, "open", doc["open"], []any{
		map[string]any{"round": "01-candidate", "finding": "host-F2", "class": "defect"},
		map[string]any{"round": "02-fixes", "finding": "host-F1", "class": "defect"},
		map[string]any{"round": "02-fixes", "finding": "host-F3", "class": "defect"},
		map[string]any{"round": "03-candidate", "finding": "host-F1", "class": "defect"},
		map[string]any{"round": "03-candidate", "finding": "host-F2", "class": "defect"},
		map[string]any{"round": "03-candidate", "finding": "host-F3", "class": "requirement-question"},
		map[string]any{"round": "03-candidate", "finding": "host-F4", "class": "defect"},
	})
	// Churn: findings after the first counted candidate round in files that
	// changed from its head (head1) to the round's head; the duplicate and
	// the finding whose location cannot be read are left out.
	crExpect(t, v, "churn", doc["churn"], []any{
		map[string]any{"round": "02-fixes", "count": 1, "findings": []any{"host-F1"}},
		map[string]any{"round": "03-candidate", "count": 2, "findings": []any{"host-F1", "host-F3"}},
	})
	// Recurring: AC-1 in all three packets (the rejected F4 is left out, the
	// fixed F1 is not); AC-2 only once, its second finding is a duplicate.
	crExpect(t, v, "recurring", doc["recurring"], []any{
		map[string]any{"link_kind": "criterion", "link": "AC-1", "rounds": []any{"01-candidate", "02-fixes", "03-candidate"}},
	})
	// Unrelated: defects whose introduced_by_change is no or unknown, but
	// not the rejected one.
	crExpect(t, v, "unrelated", doc["unrelated"], []any{
		map[string]any{"round": "01-candidate", "finding": "host-F2"},
		map[string]any{"round": "03-candidate", "finding": "host-F2"},
	})
	crExpect(t, v, "check_growth", doc["check_growth"], map[string]any{"check": 3, "other": 1, "over_hardening": true})
	crExpect(t, v, "limits", doc["limits"], map[string]any{"churn": crChurnLimit, "recurring": crRecurringLimit,
		"unrelated": crUnrelatedLimit})

	// A rerun of the same packet, a dry run and a stopped round.
	w.use("host", "aide")
	edge := filepath.Join(w.repo, "reviews", "ac2-edge")
	r1 := w.run(edge, "01-candidate", "candidate", "base", "head1")
	w.record(r1, "host", "head1", crFinding("F1", "defect", "AC-4", "app/greet.py", "yes"))
	r2 := w.run(edge, "02-candidate", "candidate", "base", "head1", "--reviewers", "aide")
	w.record(r2, "aide", "head1", crFinding("F1", "defect", "AC-4", "app/greet.py", "yes"))
	var rerun struct {
		Counted bool     `json:"counted"`
		RerunOf []string `json:"rerun_of"`
	}
	if json.Unmarshal(w.read(filepath.Join(r2, "round.json")), &rerun) != nil || rerun.Counted || len(rerun.RerunOf) != 1 {
		t.Fatalf("setup: 02-candidate is not an uncounted rerun of 01-candidate: %+v", rerun)
	}
	w.run(edge, "03-candidate", "candidate", "base", "head2", "--dry-run")
	stopped := w.run(edge, "04-candidate", "candidate", "base", "head2")
	// A round stopped before its findings: round.json without finished_at,
	// and neither findings.json nor ledger.jsonl.
	var rec map[string]any
	if err := json.Unmarshal(w.read(filepath.Join(stopped, "round.json")), &rec); err != nil {
		t.Fatalf("setup: %v", err)
	}
	delete(rec, "finished_at")
	data, _ := json.MarshalIndent(rec, "", "  ")
	w.write(filepath.Join(stopped, "round.json"), string(data)+"\n")
	for _, name := range []string{"findings.json", "ledger.jsonl"} {
		if err := os.Remove(filepath.Join(stopped, name)); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	doc = w.reportJSON(v, edge)
	crExpect(t, v, "cap of the edge series", doc["cap"], map[string]any{"code": 3, "extensions": 0, "allowed": 3, "counted": 2})
	crExpect(t, v, "rounds of the edge series", doc["rounds"], []any{
		map[string]any{"round": "01-candidate", "stage": "candidate", "sequence": 1, "base": b, "head": h1, "counted": true,
			"classes": map[string]any{"defect": 1, "requirement-question": 0, "hardening": 0, "note": 0}},
		map[string]any{"round": "02-candidate", "stage": "candidate", "sequence": 2, "base": b, "head": h1, "counted": false,
			"classes": map[string]any{"defect": 1, "requirement-question": 0, "hardening": 0, "note": 0}},
	})
	crExpect(t, v, "skipped rounds of the edge series", doc["skipped"], []any{
		map[string]any{"round": "03-candidate", "reason": "dry run"},
		map[string]any{"round": "04-candidate", "reason": "stopped before recording findings"},
	})
	crExpect(t, v, "recurring links of the edge series", doc["recurring"], []any{})
	crExpect(t, v, "churn of the edge series", doc["churn"], []any{
		map[string]any{"round": "02-candidate", "count": 0, "findings": []any{}},
	})
	crExpect(t, v, "open findings of the edge series", doc["open"], []any{
		map[string]any{"round": "01-candidate", "finding": "host-F1", "class": "defect"},
		map[string]any{"round": "02-candidate", "finding": "aide-F1", "class": "defect"},
	})
	crExpect(t, v, "check growth of the edge series", doc["check_growth"], map[string]any{"check": 0, "other": 0, "over_hardening": false})

	// Rounds come in sequence order, by folder name when sequences are
	// equal. spar numbers a round after the rounds it finds, so a deleted
	// round lets a later one share a sequence.
	w.use("host")
	order := filepath.Join(w.repo, "reviews", "ac2-order")
	w.run(order, "x-dry", "candidate", "base", "head1", "--dry-run")
	rb := w.run(order, "b-candidate", "candidate", "base", "head1")
	w.record(rb, "host", "head1")
	// The tied rounds are named against their creation order: c-fixes
	// first, a-candidate after the deletion.
	rc := w.run(order, "c-fixes", "fixes", "head1", "head2")
	w.record(rc, "host", "head2")
	if err := os.RemoveAll(filepath.Join(order, "x-dry")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ra := w.run(order, "a-candidate", "candidate", "base", "head2")
	w.record(ra, "host", "head2")
	seqs := map[string]int{}
	for _, r := range []string{rb, ra, rc} {
		var rec struct {
			Sequence int `json:"sequence"`
		}
		if json.Unmarshal(w.read(filepath.Join(r, "round.json")), &rec) != nil {
			t.Fatalf("setup: %s/round.json", r)
		}
		seqs[filepath.Base(r)] = rec.Sequence
	}
	if !reflect.DeepEqual(seqs, map[string]int{"b-candidate": 2, "c-fixes": 3, "a-candidate": 3}) {
		t.Fatalf("setup: the sequences are %v, want b-candidate 2, c-fixes 3, a-candidate 3", seqs)
	}
	var names []any
	rounds, _ := w.reportJSON(v, order)["rounds"].([]any)
	for _, r := range rounds {
		m, _ := r.(map[string]any)
		names = append(names, m["round"])
	}
	crExpect(t, v, "the order of rounds sharing and not sharing a sequence", names, []any{"b-candidate", "a-candidate", "c-fixes"})

	// A link that recurs over two packets, one of them run twice, lists
	// each packet once, by its first round. Folders whose round.json is
	// another tool's JSON are not rounds.
	w.use("host", "aide")
	twice := filepath.Join(w.repo, "reviews", "ac2-twice")
	t1 := w.run(twice, "01-candidate", "candidate", "base", "head1")
	w.record(t1, "host", "head1", crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	t2 := w.run(twice, "02-candidate", "candidate", "base", "head1", "--reviewers", "aide")
	w.record(t2, "aide", "head1", crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	w.use("host")
	t3 := w.run(twice, "03-fixes", "fixes", "head1", "head2")
	w.record(t3, "host", "head2", crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	w.write(filepath.Join(twice, "other-list", "round.json"), "[]\n")
	w.write(filepath.Join(twice, "other-schema", "round.json"), `{"schema": 1}`+"\n")
	doc = w.reportJSON(v, twice)
	crExpect(t, v, "the recurring links over a rerun packet", doc["recurring"], []any{
		map[string]any{"link_kind": "criterion", "link": "AC-1", "rounds": []any{"01-candidate", "03-fixes"}},
	})
	if rounds, _ := doc["rounds"].([]any); len(rounds) != 3 {
		t.Errorf("jaekit:violation=%s the series with other tools' folders reports %d rounds, want 3", v, len(rounds))
	}
}

// crLines is the set of lines of a text.
func crLines(text string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		out[line] = true
	}
	return out
}

// crSection lists the lines of a Markdown section that start with prefix,
// from the heading line up to the next heading.
func crSection(md, heading, prefix string) []string {
	out := []string{}
	in := false
	for _, line := range strings.Split(md, "\n") {
		switch {
		case line == heading:
			in = true
		case in && strings.HasPrefix(line, "#"):
			return out
		case in && strings.HasPrefix(line, prefix):
			out = append(out, line)
		}
	}
	return out
}

// crParity compares every row of the Markdown rounds table, its skipped
// lines and every Markdown list with the JSON report's entries, in order:
// nothing left out, repeated or added.
func crParity(t *testing.T, v string, doc map[string]any, md string) {
	t.Helper()
	str := func(x any) string { return fmt.Sprint(x) }
	list := func(key string) []map[string]any {
		var out []map[string]any
		items, _ := doc[key].([]any)
		for _, item := range items {
			m, _ := item.(map[string]any)
			out = append(out, m)
		}
		return out
	}
	join := func(x any) string {
		var parts []string
		items, _ := x.([]any)
		for _, item := range items {
			parts = append(parts, str(item))
		}
		return strings.Join(parts, ", ")
	}
	rows, skipped, open, churn, recurring, unrelated := []string{}, []string{}, []string{}, []string{}, []string{}, []string{}
	for _, r := range list("rounds") {
		classes, _ := r["classes"].(map[string]any)
		counted := map[bool]string{true: "yes", false: "no"}[r["counted"] == true]
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s | %v | %v | %v | %v |", r["round"], r["stage"], counted,
			str(r["base"])[:12], str(r["head"])[:12], classes["defect"], classes["requirement-question"], classes["hardening"], classes["note"]))
	}
	for _, r := range list("skipped") {
		skipped = append(skipped, fmt.Sprintf("- skipped %s: %s", r["round"], r["reason"]))
	}
	for _, f := range list("open") {
		open = append(open, fmt.Sprintf("- %s %s (%s)", f["round"], f["finding"], f["class"]))
	}
	for _, c := range list("churn") {
		if c["count"] == float64(0) {
			churn = append(churn, fmt.Sprintf("- %s: 0", c["round"]))
			continue
		}
		churn = append(churn, fmt.Sprintf("- %s: %v (%s)", c["round"], c["count"], join(c["findings"])))
	}
	for _, r := range list("recurring") {
		recurring = append(recurring, fmt.Sprintf("- %s %s: %s", r["link_kind"], r["link"], join(r["rounds"])))
	}
	for _, f := range list("unrelated") {
		unrelated = append(unrelated, fmt.Sprintf("- %s %s", f["round"], f["finding"]))
	}
	none := func(l []string) []string {
		if len(l) == 0 {
			return []string{"- none"}
		}
		return l
	}
	tableRows := []string{}
	for _, line := range crSection(md, "## Rounds", "| ") {
		if !strings.HasPrefix(line, "| Round |") && !strings.HasPrefix(line, "| --- |") {
			tableRows = append(tableRows, line)
		}
	}
	crExpect(t, v, "the rows of the Markdown rounds table", tableRows, rows)
	crExpect(t, v, "the skipped lines of the Markdown report", crSection(md, "## Rounds", "- "), skipped)
	crExpect(t, v, "the Markdown open findings", crSection(md, "## Open findings", "- "), none(open))
	crExpect(t, v, "the Markdown churn", crSection(md, "### Churn", "- "), none(churn))
	crExpect(t, v, "the Markdown recurring links", crSection(md, "### Recurring links", "- "), none(recurring))
	crExpect(t, v, "the Markdown unrelated defects", crSection(md, "### Unrelated defects", "- "), none(unrelated))
}

// AC-3: the Markdown report, the default form, holds the values of the JSON
// report; neither form names a cause or advises an extension; another
// format is a usage error.
func TestSparCapReportAC3_TwoFormsNoCause(t *testing.T) {
	const v = "cap-report-form-wrong"
	w := newCRWorld(t)
	series := w.mainSeries("ac3")
	doc := w.reportJSON(v, series)
	code, md, errs := w.report(series, "--format", "md")
	if code != ExitOK {
		t.Fatalf("jaekit:violation=%s spar cap-report --format md exits %d: %s", v, code, errs)
	}
	if _, plain, _ := w.report(series); plain != md {
		t.Errorf("jaekit:violation=%s spar cap-report without --format differs from --format md", v)
	}
	b, h1, h2 := w.commit["base"][:12], w.commit["head1"][:12], w.commit["head2"][:12]
	lines := crLines(md)
	for _, want := range []string{
		"# Cap report: " + series,
		"- code rounds counted: 3 of 3 (cap 3, extensions 0)",
		"- open: defect 6, requirement-question 1",
		"| Round | Stage | Counted | Base | Head | defect | requirement-question | hardening | note |",
		"| 01-candidate | candidate | yes | " + b + " | " + h1 + " | 3 | 0 | 1 | 0 |",
		"| 02-fixes | fixes | yes | " + h1 + " | " + h2 + " | 2 | 0 | 0 | 1 |",
		"| 03-candidate | candidate | yes | " + b + " | " + h2 + " | 3 | 1 | 0 | 0 |",
		"- 01-candidate host-F2 (defect)",
		"- 03-candidate host-F3 (requirement-question)",
		"- 02-fixes: 1 (host-F1)",
		"- 03-candidate: 2 (host-F1, host-F3)",
		"- criterion AC-1: 01-candidate, 02-fixes, 03-candidate",
		"- 01-candidate host-F2",
		"- 03-candidate host-F2",
		"- added lines: check 3, other 1; over hardening: yes",
		"- churn: " + crChurnLimit,
		"- recurring: " + crRecurringLimit,
		"- unrelated: " + crUnrelatedLimit,
	} {
		if !lines[want] {
			t.Errorf("jaekit:violation=%s the Markdown report has no line %q:\n%s", v, want, md)
		}
	}
	crParity(t, v, doc, md)
	// A second series has a dry run and a round without churn.
	edge := filepath.Join(w.repo, "reviews", "ac3-edge")
	r1 := w.run(edge, "01-candidate", "candidate", "base", "head1")
	w.record(r1, "host", "head1", crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	w.run(edge, "02-candidate", "candidate", "base", "head2", "--dry-run")
	r3 := w.run(edge, "03-fixes", "fixes", "head1", "head2")
	w.record(r3, "host", "head2", crFinding("F1", "defect", "AC-2", "app/other.py", "yes"))
	edgeDoc := w.reportJSON(v, edge)
	_, edgeMD, _ := w.report(edge)
	if skipped, _ := edgeDoc["skipped"].([]any); len(skipped) != 1 {
		t.Fatalf("setup: the edge series has %d skipped rounds, want the dry run", len(skipped))
	}
	crParity(t, v, edgeDoc, edgeMD)
	for _, heading := range []string{"## Rounds", "## Open findings", "## Signals", "### Churn", "### Recurring links",
		"### Unrelated defects", "### Check growth", "## Limits"} {
		if !lines[heading] {
			t.Errorf("jaekit:violation=%s the Markdown report has no heading %q", v, heading)
		}
	}
	var keys []string
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	crExpect(t, v, "the JSON report's keys", keys, []string{"cap", "check_growth", "churn", "limits", "open", "recurring",
		"rounds", "schema", "series", "skipped", "unrelated"})
	// The series path is the test's own folder name and may hold any word.
	lower := strings.ToLower(strings.ReplaceAll(md, series, ""))
	for _, word := range []string{"cause", "recommend", "extend"} {
		if strings.Contains(lower, word) {
			t.Errorf("jaekit:violation=%s the Markdown report says %q: the report never picks a cause or advises an extension", v, word)
		}
	}
	if code, out, _ := w.report(series, "--format", "xml"); code != ExitUsage || out != "" {
		t.Errorf("jaekit:violation=%s spar cap-report --format xml exits %d with %q, want the usage error 64 and no report", v, code, out)
	}
}

// crSnapshot lists every file under dir with its mode and content.
func crSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			out[rel] = info.Mode().String()
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = info.Mode().String() + " " + string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// AC-4: the same records and Git give the same report, a second later too;
// the command changes no record, file or Git state and starts no reviewer;
// a series below the cap, one with only a plan round and an empty one are
// reported in the same form with empty signals and exit 0.
func TestSparCapReportAC4_DeterministicReadOnly(t *testing.T) {
	const v = "cap-report-not-read-only"
	w := newCRWorld(t)
	series := w.mainSeries("ac4")
	marker := filepath.Join(w.dir, "reviewer-started")
	w.files++
	w.config = w.write(filepath.Join(w.dir, fmt.Sprintf("spar-%d.toml", w.files)),
		fmt.Sprintf("[[reviewers]]\nname = \"cmd\"\npreset = \"command\"\nargv = [\"/usr/bin/touch\", %q]\n", marker))
	// Git's own status refreshes the index, so it runs before the snapshot
	// and after the second one.
	gitBefore := w.git("status", "--porcelain=v1", "--ignored", "--untracked-files=all") + "\n" + w.git("rev-parse", "HEAD") +
		"\n" + w.git("show-ref")
	before := crSnapshot(t, w.repo)
	first := w.reportJSON(v, series)
	_, md1, _ := w.report(series)
	time.Sleep(1100 * time.Millisecond)
	second := w.reportJSON(v, series)
	_, md2, _ := w.report(series)
	if !crSame(first, second) || md1 != md2 {
		t.Errorf("jaekit:violation=%s two reports of the same records a second apart differ", v)
	}
	if after := crSnapshot(t, w.repo); !reflect.DeepEqual(before, after) {
		var changed []string
		for k := range before {
			if before[k] != after[k] {
				changed = append(changed, k)
			}
		}
		for k := range after {
			if _, ok := before[k]; !ok {
				changed = append(changed, k)
			}
		}
		sort.Strings(changed)
		t.Errorf("jaekit:violation=%s spar cap-report changed files: %v", v, changed)
	}
	gitAfter := w.git("status", "--porcelain=v1", "--ignored", "--untracked-files=all") + "\n" + w.git("rev-parse", "HEAD") +
		"\n" + w.git("show-ref")
	if gitBefore != gitAfter {
		t.Errorf("jaekit:violation=%s spar cap-report changed the Git state:\n%s\n%s", v, gitBefore, gitAfter)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("jaekit:violation=%s spar cap-report started a reviewer", v)
	}

	crClockScanWorks(t)
	reads, ok := crClockReads(t, ".", "CapReport", "dispatch cap-report")
	switch {
	case !ok:
		t.Errorf("jaekit:violation=%s the package has no CapReport function or no cap-report branch in dispatch", v)
	case len(reads) > 0:
		t.Errorf("jaekit:violation=%s spar cap-report reads the current time: %v", v, reads)
	}

	w.use("host")
	empty := map[string]any{"churn": []any{}, "recurring": []any{}, "unrelated": []any{}, "open": []any{}, "skipped": []any{},
		"check_growth": map[string]any{"check": 0, "other": 0, "over_hardening": false}}
	// Below the cap the signals are computed as they are at the cap.
	below := filepath.Join(w.repo, "reviews", "ac4-below")
	r1 := w.run(below, "01-candidate", "candidate", "base", "head1")
	w.record(r1, "host", "head1", crFinding("F1", "defect", "AC-1", "app/greet.py", "no"))
	r2 := w.run(below, "02-fixes", "fixes", "head1", "head2")
	w.record(r2, "host", "head2", crFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	belowSignals := map[string]any{
		"churn":     []any{map[string]any{"round": "02-fixes", "count": 1, "findings": []any{"host-F1"}}},
		"recurring": []any{map[string]any{"link_kind": "criterion", "link": "AC-1", "rounds": []any{"01-candidate", "02-fixes"}}},
		"unrelated": []any{map[string]any{"round": "01-candidate", "finding": "host-F1"}},
		"open": []any{map[string]any{"round": "01-candidate", "finding": "host-F1", "class": "defect"},
			map[string]any{"round": "02-fixes", "finding": "host-F1", "class": "defect"}},
		"skipped":      []any{},
		"check_growth": map[string]any{"check": 3, "other": 1, "over_hardening": true}}
	planOnly := filepath.Join(w.repo, "reviews", "ac4-plan")
	w.run(planOnly, "01-plan", "plan", "base", "head1")
	none := filepath.Join(w.repo, "reviews", "ac4-none")
	if err := os.MkdirAll(none, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, c := range []struct {
		series  string
		cap     map[string]any
		rounds  int
		signals map[string]any
	}{
		{below, map[string]any{"code": 3, "extensions": 0, "allowed": 3, "counted": 2}, 2, belowSignals},
		{planOnly, map[string]any{"code": 3, "extensions": 0, "allowed": 3, "counted": 0}, 0, empty},
		{none, map[string]any{"code": nil, "extensions": 0, "allowed": nil, "counted": 0}, 0, empty},
	} {
		doc := w.reportJSON(v, c.series)
		crExpect(t, v, "cap of "+filepath.Base(c.series), doc["cap"], c.cap)
		if rounds, _ := doc["rounds"].([]any); len(rounds) != c.rounds {
			t.Errorf("jaekit:violation=%s %s reports %d rounds, want %d", v, filepath.Base(c.series), len(rounds), c.rounds)
		}
		for k, want := range c.signals {
			crExpect(t, v, k+" of "+filepath.Base(c.series), doc[k], want)
		}
		if code, md, errs := w.report(c.series); code != ExitOK || !crLines(md)["## Signals"] {
			t.Errorf("jaekit:violation=%s the Markdown report of %s exits %d without the same form: %s%s", v, filepath.Base(c.series), code, md, errs)
		}
	}
}

// crClockReads lists the reads of the current time reachable from the
// function root in the Go package in dir (test files left out): time.Now,
// time.Since and time.Until under any import name, a dot import of time,
// and the package's own now. The walk follows every package function, method
// and package variable whose name a reachable body uses (a method by its
// name, whatever its receiver), so it may follow more than is called; it does
// not follow calls into other packages.
func crClockReads(t *testing.T, dir string, roots ...string) ([]string, bool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	type node struct {
		name  string
		body  ast.Node
		times map[string]bool // the names package time has in the node's file
		dot   bool            // the file dot-imports time
		pkgs  map[string]bool // every package name the file imports
	}
	decls := map[string][]node{}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		times, pkgs, dot := map[string]bool{}, map[string]bool{}, false
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			local := path[strings.LastIndex(path, "/")+1:]
			if imp.Name != nil {
				local = imp.Name.Name
			}
			switch {
			case local == "." && path == "time":
				dot = true
			case path == "time":
				times[local] = true
			}
			pkgs[local] = true
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body != nil {
					decls[d.Name.Name] = append(decls[d.Name.Name], node{d.Name.Name, d.Body, times, dot, pkgs})
				}
				// A command's branch of the CLI's dispatch is a root of its
				// own, named "dispatch <command>".
				if d.Name.Name == "dispatch" && d.Recv == nil && d.Body != nil {
					ast.Inspect(d.Body, func(x ast.Node) bool {
						c, ok := x.(*ast.CaseClause)
						if !ok {
							return true
						}
						for _, e := range c.List {
							if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
								key := "dispatch " + strings.Trim(lit.Value, `"`)
								decls[key] = append(decls[key], node{key, &ast.BlockStmt{List: c.Body}, times, dot, pkgs})
							}
						}
						return true
					})
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, id := range vs.Names {
							for _, value := range vs.Values {
								decls[id.Name] = append(decls[id.Name], node{id.Name, value, times, dot, pkgs})
							}
						}
					}
				}
			}
		}
	}
	for _, root := range roots {
		if len(decls[root]) == 0 {
			return nil, false
		}
	}
	var reads []string
	seen := map[string]bool{}
	queue := append([]string{}, roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		for _, n := range decls[name] {
			ast.Inspect(n.body, func(x ast.Node) bool {
				switch x := x.(type) {
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && n.pkgs[id.Name] {
						if n.times[id.Name] && (x.Sel.Name == "Now" || x.Sel.Name == "Since" || x.Sel.Name == "Until") {
							reads = append(reads, n.name+": "+id.Name+"."+x.Sel.Name)
						}
						return false // another package's name, not this package's
					}
				case *ast.Ident:
					switch {
					case x.Name == "now" && len(decls["now"]) > 0:
						reads = append(reads, n.name+": now")
					case n.dot && (x.Name == "Now" || x.Name == "Since" || x.Name == "Until"):
						reads = append(reads, n.name+": "+x.Name)
					case len(decls[x.Name]) > 0 && !seen[x.Name]:
						queue = append(queue, x.Name)
					}
				}
				return true
			})
		}
	}
	sort.Strings(reads)
	return reads, true
}

// crClockScanWorks runs the scan on a synthetic package that reads the
// clock only through an aliased import, a method, a package variable, a dot
// import and a command's dispatch branch; a scan that misses one of them
// observes nothing.
func crClockScanWorks(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	src := map[string]string{
		"a.go": "package p\n\nimport clock \"time\"\n\nvar started = clock.Now\n\ntype r struct{}\n\n" +
			"func (r) stamp() any { return clock.Since(clock.Time{}) }\n\nfunc Root() { var x r; x.stamp(); _ = started; dotted() }\n\n" +
			"func dispatch(cmd string) any {\n\tswitch cmd {\n\tcase \"report\":\n\t\treturn clock.Now()\n\tcase \"other\":\n\t\treturn nil\n\t}\n\treturn nil\n}\n",
		"b.go": "package p\n\nimport . \"time\"\n\nfunc dotted() any { return Until(Time{}) }\n",
	}
	for name, body := range src {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	reads, ok := crClockReads(t, dir, "Root", "dispatch report")
	want := []string{"dispatch report: clock.Now", "dotted: Until", "stamp: clock.Since", "started: clock.Now"}
	if !ok || !reflect.DeepEqual(reads, want) {
		t.Fatalf("setup: the clock scan finds %v in the synthetic package, want %v", reads, want)
	}
}

// AC-5: a missing or non-folder series, unreadable records, a commit the
// repository does not have (a shallow clone too) and a Git that cannot start
// end with the refusal 65, a reason that names what failed and no report.
func TestSparCapReportAC5_InputFailuresRefused(t *testing.T) {
	const v = "cap-report-input-failure-hidden"
	w := newCRWorld(t)
	series := w.mainSeries("ac5")
	// The working series reports; each case below breaks one input of a copy.
	if code, _, errs := w.report(series, "--format", "json"); code != ExitOK {
		t.Fatalf("jaekit:violation=%s spar cap-report of a readable series exits %d: %s", v, code, errs)
	}
	clone := func(name string) string {
		t.Helper()
		dst := filepath.Join(w.repo, "reviews", name)
		if err := os.CopyFS(dst, os.DirFS(series)); err != nil {
			t.Fatalf("setup: %v", err)
		}
		return dst
	}
	expectRefused := func(what, s, repo, names string) {
		t.Helper()
		code, out, errs := w.spar("cap-report", s, "--repo", repo, "--format", "json")
		if code != ExitRefused || out != "" || !strings.Contains(errs, names) {
			t.Errorf("jaekit:violation=%s %s: exit %d, report %q, reason %q; want the refusal 65, no report and a reason naming %q",
				v, what, code, out, errs, names)
		}
	}
	missing := filepath.Join(w.repo, "reviews", "no-such-series")
	expectRefused("a missing series", missing, w.repo, missing)
	file := w.write(filepath.Join(w.repo, "reviews", "a-file"), "not a folder\n")
	expectRefused("a series that is a file", file, w.repo, file)
	broken := clone("bad-round")
	w.write(filepath.Join(broken, "02-fixes", "round.json"), "{not json\n")
	expectRefused("a round.json that is not JSON", broken, w.repo, filepath.Join(broken, "02-fixes", "round.json"))
	broken = clone("bad-findings")
	w.write(filepath.Join(broken, "03-candidate", "findings.json"), "[{\"id\": 1}\n")
	expectRefused("a findings.json that is not JSON", broken, w.repo, filepath.Join(broken, "03-candidate", "findings.json"))
	broken = clone("bad-ledger")
	w.write(filepath.Join(broken, "01-candidate", "ledger.jsonl"), "{\"finding\": \"host-F1\"\n")
	expectRefused("a ledger.jsonl line that is not JSON", broken, w.repo, filepath.Join(broken, "01-candidate", "ledger.jsonl"))
	broken = clone("bad-extensions")
	w.write(filepath.Join(broken, "extensions.jsonl"), "{\"stage\": \"code\", \"by\":\n")
	expectRefused("an extensions.jsonl line that is not JSON", broken, w.repo, filepath.Join(broken, "extensions.jsonl"))
	if os.Geteuid() != 0 { // root reads a file without permissions
		broken = clone("closed-round")
		if err := os.Chmod(filepath.Join(broken, "01-candidate", "round.json"), 0); err != nil {
			t.Fatalf("setup: %v", err)
		}
		expectRefused("a round.json that cannot be opened", broken, w.repo, filepath.Join(broken, "01-candidate", "round.json"))
		if err := os.Chmod(filepath.Join(broken, "01-candidate", "round.json"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	broken = clone("lost-findings")
	if err := os.Remove(filepath.Join(broken, "02-fixes", "findings.json")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	expectRefused("a finished round without findings.json", broken, w.repo, filepath.Join(broken, "02-fixes"))
	// Valid JSON outside the record form.
	for i, line := range []string{"null", `{"stage":"code","by":0,"user_quote":"q"}`, `{"stage":"code","by":1}`,
		`{"stage":"code","by":1,"cause":"Churn","user_quote":"q"}`, `{"stage":"plan","by":1,"cause":"churn","user_quote":"q"}`,
		`{"stage":"review","by":1,"user_quote":"q"}`, `{"stage":"code","by":1,"cause":"other","user_quote":"q"}`,
		`{"stage":"code","by":2,"cause":"churn","user_quote":"q"}`} {
		broken = clone(fmt.Sprintf("form-extension-%d", i))
		w.write(filepath.Join(broken, "extensions.jsonl"), line+"\n")
		expectRefused("the extension line "+line, broken, w.repo, filepath.Join(broken, "extensions.jsonl"))
	}
	// The signal fields of an otherwise valid finding of 02-fixes.
	var findings []map[string]any
	if err := json.Unmarshal(w.read(filepath.Join(series, "02-fixes", "findings.json")), &findings); err != nil || len(findings) == 0 {
		t.Fatalf("setup: findings.json of 02-fixes: %v", err)
	}
	bodies := []string{"null", `[{"id": "host-F1"}]`, `[{"class": "defect"}]`, `[null]`}
	for _, change := range []map[string]any{{"introduced_by_change": "maybe"}, {"link_kind": "issue"}} {
		mutated := map[string]any{}
		for k, val := range findings[0] {
			mutated[k] = val
		}
		for k, val := range change {
			mutated[k] = val
		}
		data, _ := json.Marshal(append([]map[string]any{mutated}, findings[1:]...))
		bodies = append(bodies, string(data))
	}
	for i, body := range bodies {
		broken = clone(fmt.Sprintf("form-findings-%d", i))
		w.write(filepath.Join(broken, "02-fixes", "findings.json"), body+"\n")
		expectRefused("the findings "+body, broken, w.repo, filepath.Join(broken, "02-fixes", "findings.json"))
	}
	for i, line := range []string{"null", `{"finding":"host-F1","disposition":"maybe"}`, `{"disposition":"fix"}`} {
		broken = clone(fmt.Sprintf("form-ledger-%d", i))
		w.write(filepath.Join(broken, "01-candidate", "ledger.jsonl"), line+"\n")
		expectRefused("the ledger line "+line, broken, w.repo, filepath.Join(broken, "01-candidate", "ledger.jsonl"))
	}
	broken = clone("form-round")
	var rec map[string]any
	if err := json.Unmarshal(w.read(filepath.Join(broken, "02-fixes", "round.json")), &rec); err != nil {
		t.Fatalf("setup: %v", err)
	}
	rec["stage"] = "review"
	data, _ := json.Marshal(rec)
	w.write(filepath.Join(broken, "02-fixes", "round.json"), string(data)+"\n")
	expectRefused("a round.json with an unknown stage", broken, w.repo, filepath.Join(broken, "02-fixes", "round.json"))
	broken = clone("lost-ledger")
	if err := os.Remove(filepath.Join(broken, "02-fixes", "ledger.jsonl")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	expectRefused("a finished round without ledger.jsonl", broken, w.repo, filepath.Join(broken, "02-fixes"))

	// Another repository without the series' commits, and a shallow clone
	// that has only head2.
	other := filepath.Join(w.dir, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	crGit(t, other, "init", "-q")
	w.write(filepath.Join(other, "x.txt"), "x\n")
	crGit(t, other, "add", "-A")
	crGit(t, other, "commit", "-qm", "other")
	expectRefused("a repository without the series' commits", series, other, w.commit["base"][:12])
	w.git("branch", "shallow-head2", w.commit["head2"])
	shallow := filepath.Join(w.dir, "shallow")
	crGit(t, w.dir, "clone", "-q", "--depth", "1", "--branch", "shallow-head2", "file://"+w.repo, shallow)
	expectRefused("a shallow clone", series, shallow, w.commit["base"][:12])
	expectRefused("a --repo that is not a Git repository", series, w.dir, w.dir)

	// A Git that cannot start.
	t.Setenv("PATH", filepath.Join(w.dir, "empty-path"))
	expectRefused("a Git that cannot start", series, w.repo, "git")
}

// AC-6: a code extension records its cause, one of five exact values, and a
// reason with the spaces around it removed; the user's words stay required.
func TestSparCapReportAC6_CodeExtensionKeepsCause(t *testing.T) {
	const v = "extend-cause-not-recorded"
	w := newCRWorld(t)
	for i, c := range []struct {
		args []string
		want map[string]any
	}{
		{[]string{"--by", "1", "--cause", "churn"}, map[string]any{"stage": "code", "by": 1, "cause": "churn", "user_quote": "synthetic words"}},
		{[]string{"--by", "1", "--cause", "recurring"}, map[string]any{"stage": "code", "by": 1, "cause": "recurring", "user_quote": "synthetic words"}},
		{[]string{"--by", "1", "--cause", "unrelated"}, map[string]any{"stage": "code", "by": 1, "cause": "unrelated", "user_quote": "synthetic words"}},
		{[]string{"--by", "1", "--cause", "hardening"}, map[string]any{"stage": "code", "by": 1, "cause": "hardening", "user_quote": "synthetic words"}},
		{[]string{"--by", "1", "--cause", "other", "--reason", "the reviewer quota ran out"},
			map[string]any{"stage": "code", "by": 1, "cause": "other", "reason": "the reviewer quota ran out", "user_quote": "synthetic words"}},
		{[]string{"--by", "1", "--cause", "churn", "--reason", "  two fixes touched greet.py \n"},
			map[string]any{"stage": "code", "by": 1, "cause": "churn", "reason": "two fixes touched greet.py", "user_quote": "synthetic words"}},
		{[]string{"--by", "2", "--cause", "churn", "--reason", "the fix needs its own fixes round"},
			map[string]any{"stage": "code", "by": 2, "cause": "churn", "reason": "the fix needs its own fixes round", "user_quote": "synthetic words"}},
		{[]string{"--by", "1", "--cause", "churn", "--reason", "   "}, map[string]any{"stage": "code", "by": 1, "cause": "churn", "user_quote": "synthetic words"}},
	} {
		series := filepath.Join(w.repo, "reviews", fmt.Sprintf("ac6-%d", i))
		args := append([]string{"extend", series, "--stage", "code"}, c.args...)
		code, _, errs := w.spar(append(args, "--user-quote", "synthetic words", "--repo", w.repo)...)
		if code != ExitOK {
			t.Errorf("jaekit:violation=%s spar extend --stage code %v exits %d: %s", v, c.args, code, errs)
			continue
		}
		crExpect(t, v, fmt.Sprintf("the extension line of %v", c.args), w.extensions(v, series), []any{c.want})
	}
	// The cap counts the extension: a fourth code round runs.
	series := w.mainSeries("ac6-cap")
	if code, _, errs := w.spar("extend", series, "--stage", "code", "--by", "1", "--cause", "churn", "--user-quote", "synthetic words",
		"--repo", w.repo); code != ExitOK {
		t.Fatalf("jaekit:violation=%s spar extend with a cause exits %d: %s", v, code, errs)
	}
	w.run(series, "04-fixes", "fixes", "head2", "head3")
}

// AC-7: a code extension without a valid cause, without a reason for
// --by 2 or more or for the cause other, and the extensions refused today
// fail with their reason and record nothing.
func TestSparCapReportAC7_CodeExtensionRules(t *testing.T) {
	const v = "extend-rule-not-enforced"
	w := newCRWorld(t)
	for i, c := range []struct {
		args  []string
		code  int
		names string
	}{
		{[]string{"--by", "1", "--user-quote", "synthetic words"}, ExitUsage, "--cause"},
		{[]string{"--by", "1", "--cause", "Churn", "--user-quote", "synthetic words"}, ExitUsage, "--cause"},
		{[]string{"--by", "1", "--cause", "regression", "--user-quote", "synthetic words"}, ExitUsage, "--cause"},
		{[]string{"--by", "1", "--cause", "", "--user-quote", "synthetic words"}, ExitUsage, "--cause"},
		{[]string{"--by", "2", "--cause", "churn", "--user-quote", "synthetic words"}, ExitRefused, "--reason"},
		{[]string{"--by", "3", "--cause", "churn", "--reason", "   ", "--user-quote", "synthetic words"}, ExitRefused, "--reason"},
		{[]string{"--by", "1", "--cause", "other", "--user-quote", "synthetic words"}, ExitRefused, "--reason"},
		{[]string{"--by", "1", "--cause", "other", "--reason", " \t ", "--user-quote", "synthetic words"}, ExitRefused, "--reason"},
		{[]string{"--by", "1", "--cause", "churn", "--user-quote", "   "}, ExitRefused, "--user-quote"},
		{[]string{"--by", "0", "--cause", "churn", "--user-quote", "synthetic words"}, ExitUsage, "--by"},
	} {
		series := filepath.Join(w.repo, "reviews", fmt.Sprintf("ac7-%d", i))
		code, _, errs := w.spar(append(append([]string{"extend", series, "--stage", "code"}, c.args...), "--repo", w.repo)...)
		if code != c.code || !strings.Contains(errs, c.names) {
			t.Errorf("jaekit:violation=%s spar extend --stage code %q exits %d (%q), want %d naming %s", v, c.args, code, errs, c.code, c.names)
		}
		if lines := w.extensions(v, series); lines != nil {
			t.Errorf("jaekit:violation=%s spar extend --stage code %q recorded %v", v, c.args, lines)
		}
	}
	// A series Git would track is refused as today (G28).
	tracked := filepath.Join(w.repo, "app", "series")
	code, _, errs := w.spar("extend", tracked, "--stage", "code", "--by", "1", "--cause", "churn", "--user-quote", "synthetic words",
		"--repo", w.repo)
	if code != ExitRefused {
		t.Errorf("jaekit:violation=%s spar extend into a tracked folder exits %d (%q), want the refusal 65", v, code, errs)
	}
	if _, err := os.Stat(filepath.Join(tracked, "extensions.jsonl")); err == nil {
		t.Errorf("jaekit:violation=%s spar extend recorded into a folder Git would track", v)
	}
}

// AC-8: plan and plan-delta extensions record as they do today, and refuse
// --cause and --reason as usage errors without recording anything.
func TestSparCapReportAC8_PlanExtensionsUnchanged(t *testing.T) {
	w := newCRWorld(t)
	for _, stage := range []string{"plan", "plan-delta"} {
		series := filepath.Join(w.repo, "reviews", "ac8-"+stage)
		w.must(ExitOK, "extend", series, "--stage", stage, "--by", "2", "--user-quote", "synthetic words", "--repo", w.repo)
		if got := w.extensions("plan-extension-changed", series); !crSame(got, []any{map[string]any{"stage": stage, "by": 2,
			"user_quote": "synthetic words"}}) {
			t.Errorf("AC-8: a %s extension records %v", stage, got)
		}
		for _, extra := range [][]string{{"--cause", "churn"}, {"--reason", "synthetic reason"}} {
			s := filepath.Join(w.repo, "reviews", "ac8-"+stage+extra[0])
			code, _, errs := w.spar(append([]string{"extend", s, "--stage", stage, "--by", "1", "--user-quote", "synthetic words",
				"--repo", w.repo}, extra...)...)
			if code != ExitUsage || !strings.Contains(errs, extra[0]) {
				t.Errorf("AC-8: a %s extension with %s exits %d (%q), want the usage error 64 naming it", stage, extra[0], code, errs)
			}
			if lines := w.extensions("plan-extension-changed", s); lines != nil {
				t.Errorf("AC-8: a %s extension with %s recorded %v", stage, extra[0], lines)
			}
		}
	}
}

// AC-10 (spar): an extension line without a cause, as spar wrote it before,
// counts in spar run, spar gate and the report; a line with a cause and a
// reason counts the same in spar run and spar gate.
func TestSparCapReportAC10_LinesWithoutCauseInSpar(t *testing.T) {
	const v = "extension-compat-broken"
	w := newCRWorld(t)
	gate := func(series, out string) map[string]any {
		t.Helper()
		code, _, errs := w.spar("gate", filepath.Join(series, "01-candidate"), filepath.Join(series, "02-fixes"),
			filepath.Join(series, "03-candidate"), filepath.Join(series, "04-fixes"), "--expect-head", w.commit["head3"],
			"--out", out, "--repo", w.repo)
		if code != ExitBlocked {
			t.Fatalf("setup: spar gate exits %d, want blocked (open findings): %s", code, errs)
		}
		var doc map[string]any
		if err := json.Unmarshal(w.read(out), &doc); err != nil {
			t.Fatalf("setup: %v", err)
		}
		delete(doc, "written_at")
		delete(doc, "previous_gate_digest")
		return doc
	}
	series := w.mainSeries("ac10")
	old := `{"stage":"code","by":1,"user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}` + "\n"
	w.write(filepath.Join(series, "extensions.jsonl"), old)
	w.use("host", "aide")
	r4 := w.run(series, "04-fixes", "fixes", "head2", "head3")
	w.record(r4, "host", "head3")
	withOld := gate(series, filepath.Join(w.repo, "reviews", "gate-old.json"))
	w.write(filepath.Join(series, "extensions.jsonl"),
		`{"stage":"code","by":1,"cause":"churn","reason":"synthetic reason","user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`+"\n")
	withCause := gate(series, filepath.Join(w.repo, "reviews", "gate-new.json"))
	if !reflect.DeepEqual(withOld, withCause) {
		t.Errorf("jaekit:violation=%s spar gate differs between an extension line with a cause and one without:\n%v\n%v", v, withOld, withCause)
	}
	// With the line with a cause, the reviewer that did not complete
	// 04-fixes reruns the same packet uncounted.
	code, _, errs := w.spar("run", "--stage", "fixes", "--base", w.commit["head2"], "--head", w.commit["head3"],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", filepath.Join(series, "04b-fixes"),
		"--repo", w.repo, "--reviewers", "aide")
	var rerun struct {
		Counted bool     `json:"counted"`
		RerunOf []string `json:"rerun_of"`
	}
	if code != ExitOK {
		t.Errorf("jaekit:violation=%s with an extension line with a cause a rerun of the same packet exits %d: %s", v, code, errs)
	} else if json.Unmarshal(w.read(filepath.Join(series, "04b-fixes", "round.json")), &rerun) != nil || rerun.Counted ||
		!reflect.DeepEqual(rerun.RerunOf, []string{"04-fixes"}) {
		t.Errorf("jaekit:violation=%s with an extension line with a cause a rerun of the same packet is %+v, want uncounted of 04-fixes", v, rerun)
	}
	// Both lines count: a fifth code round is refused at 4 of 4.
	code, _, errs = w.spar("run", "--stage", "candidate", "--base", w.commit["base"], "--head", w.commit["head3"],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", filepath.Join(series, "05-candidate"),
		"--repo", w.repo)
	if code != ExitRefused || !strings.Contains(errs, "4 of 4 rounds ran") {
		t.Errorf("jaekit:violation=%s with one extension line a fifth code round exits %d (%q), want the refusal at 4 of 4", v, code, errs)
	}
	w.write(filepath.Join(series, "extensions.jsonl"), old)
	doc := w.reportJSON(v, series)
	crExpect(t, v, "the cap of a series with a line without a cause", doc["cap"],
		map[string]any{"code": 3, "extensions": 1, "allowed": 4, "counted": 4})
	if _, md, _ := w.report(series); !crLines(md)["- code rounds counted: 4 of 4 (cap 3, extensions 1)"] {
		t.Errorf("jaekit:violation=%s the Markdown report does not count the line without a cause:\n%s", v, md)
	}
}
