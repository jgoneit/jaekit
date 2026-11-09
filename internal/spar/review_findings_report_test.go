package spar

import (
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

// Checks of the goal review-findings-report on the Spar side: the gate file
// lists the findings a person deferred and the findings without any
// disposition (AC-1), and the round summary shows every claim whole
// (AC-9). The tests run the spar command line (Main) on a temporary
// repository with a native reviewer whose replies they record with
// `spar record`; no reviewer CLI or model runs. A change check names the
// property it observes on the first line of each failure
// (jaekit:violation=<id>). A setup that breaks (no git, a file, repository,
// round or record that cannot be made or read, a spar command that fails
// where the setup needs it) skips, which the adapter reads as inconclusive.

// rfSetup stops a check whose setup broke.
func rfSetup(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Skipf("setup: "+format, args...)
}

type rfWorld struct {
	t          *testing.T
	stop       func(format string, args ...any)
	dir, repo  string
	base, head string
	criteria   string
	config     string
	replies    int
}

// newRFWorld is the world of a change check: a broken setup skips.
func newRFWorld(t *testing.T) *rfWorld {
	t.Helper()
	return rfWorldWith(t, func(format string, args ...any) { t.Helper(); rfSetup(t, format, args...) })
}

// newRFKeptWorld is the world of a check of kept behavior: a broken setup
// fails, so it never passes.
func newRFKeptWorld(t *testing.T) *rfWorld {
	t.Helper()
	return rfWorldWith(t, func(format string, args ...any) { t.Helper(); t.Fatalf("setup: "+format, args...) })
}

func rfWorldWith(t *testing.T, stop func(string, ...any)) *rfWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		stop("git is not available: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	w := &rfWorld{t: t, stop: stop, dir: realish(t.TempDir())}
	w.repo = filepath.Join(w.dir, "repo")
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n")
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('one')\n")
	w.write(filepath.Join(w.repo, "checks", "t.py"), "assert True\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('two')\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "- **AC-1** one\n")
	w.config = w.write(filepath.Join(w.dir, "spar.toml"), "[[reviewers]]\nname = \"native\"\npreset = \"native\"\n")
	return w
}

func (w *rfWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.stop("%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.stop("%v", err)
	}
	return path
}

func (w *rfWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		w.stop("git %v: %v %s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

func (w *rfWorld) spar(args ...string) (int, string, string) {
	var stdout, stderr strings.Builder
	getenv := func(k string) string {
		switch k {
		case "SPAR_CONFIG":
			return w.config
		case "HOME":
			return w.dir
		}
		return ""
	}
	code := Main("test", args, strings.NewReader(""), &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

// must runs a spar command the setup needs.
func (w *rfWorld) must(args ...string) string {
	w.t.Helper()
	code, out, msg := w.spar(args...)
	if code != ExitOK {
		w.stop("spar %v exits %d: %s", args, code, msg)
	}
	return out
}

// round runs a candidate round of the native reviewer into series/name and
// records its findings.
func (w *rfWorld) round(series, name string, findings ...map[string]any) string {
	w.t.Helper()
	out := filepath.Join(w.repo, "reviews", series, name)
	w.must("run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out, "--criteria", w.criteria,
		"--check-path", "checks/**", "--repo", w.repo)
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "checked"})
	}
	list := []any{}
	for _, f := range findings {
		list = append(list, f)
	}
	verdict := "no_blocking_findings"
	if len(list) > 0 {
		verdict = "blocking_findings"
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": w.head, "verdict": verdict, "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": list, "limits": ""})
	if err != nil {
		w.stop("%v", err)
	}
	w.replies++
	reply := w.write(filepath.Join(w.dir, "replies", fmt.Sprintf("%d.json", w.replies)), string(data))
	w.must("record", out, "native", "--result", reply)
	return out
}

// classes reads the class spar gave each finding of a round.
func (w *rfWorld) classes(out string) map[string]string {
	w.t.Helper()
	findings, err := loadFindings(out)
	if err != nil {
		w.stop("%v", err)
	}
	m := map[string]string{}
	for _, f := range findings {
		m[f.ID] = f.Class
	}
	return m
}

const rfIssue = "https://github.com/example/demo/issues/9"

// AC-1: the gate lists every finding whose last disposition is a person's
// defer, with its round, series, ID, class and target, and every finding with
// no disposition, with its round, series, ID and class, across the rounds of
// two series; automatic deferrals stay a count and the other fields keep
// their values.
func TestReviewFindingsReportAC1_GateListsDeferredAndUnhandled(t *testing.T) {
	const id = "gate-findings-unlisted"
	w := newRFWorld(t)
	defect := map[string]any{"proposed_class": "defect", "link_kind": "criterion", "link": "AC-1", "location": "head/app/main.py:1"}
	with := func(base map[string]any, extra map[string]any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	a := w.round("s1", "01-candidate",
		rawFinding("F1", defect),
		rawFinding("F2", map[string]any{"proposed_class": "note", "location": "head/app/main.py:1"}),
		rawFinding("F3", map[string]any{"proposed_class": "hardening", "location": "head/checks/t.py:1"}),
		rawFinding("F4", map[string]any{"proposed_class": "hardening", "location": "head/nowhere/at/all.py:1"}),
		rawFinding("F5", with(defect, map[string]any{"proposed_class": "requirement-question"})),
		rawFinding("F6", defect),
		rawFinding("F7", defect),
		rawFinding("F8", defect))
	classes := w.classes(a)
	if classes["native-F1"] != "defect" || classes["native-F2"] != "note" || classes["native-F3"] != "hardening" ||
		classes["native-F5"] != "requirement-question" || classes["native-F7"] != "defect" {
		rfSetup(t, "the findings were classified otherwise than the setup needs: %v", classes)
	}
	w.must("triage", a, "native-F1", "defer", "--to", rfIssue, "--user-quote", "이슈로 남김")
	w.must("triage", a, "native-F5", "answer", "--user-quote", "AC-1은 그대로 둠")
	w.must("triage", a, "native-F6", "duplicate", "--of", "native-F1")
	w.must("triage", a, "native-F8", "reject", "--evidence", "head/app/main.py already does it")
	b := w.round("s2", "01-candidate", rawFinding("F1", map[string]any{"proposed_class": "note", "location": "head/app/main.py:1"}))
	w.must("triage", b, "native-F1", "defer", "--to", "later\nsecond line")

	file := filepath.Join(w.repo, "reviews", "s1", "gate.json")
	code, _, msg := w.spar("gate", a, b, "--expect-head", w.head, "--out", file, "--repo", w.repo)
	data, err := os.ReadFile(file)
	if err != nil {
		rfSetup(t, "spar gate wrote no file (exit %d): %s", code, msg)
	}
	var gate map[string]any
	if err := json.Unmarshal(data, &gate); err != nil {
		rfSetup(t, "gate.json: %v", err)
	}
	item := func(series, round, finding, class string, to ...string) any {
		m := map[string]any{"series": series, "round": round, "id": finding, "class": class}
		if len(to) > 0 {
			m["to"] = to[0]
		}
		return m
	}
	wantDeferred := []any{item("s1", "01-candidate", "native-F1", "defect", rfIssue),
		item("s2", "01-candidate", "native-F1", "note", "later\nsecond line")}
	wantUnhandled := []any{item("s1", "01-candidate", "native-F2", "note"),
		item("s1", "01-candidate", "native-F4", classes["native-F4"]), item("s1", "01-candidate", "native-F7", "defect")}
	if got, ok := gate["deferred_findings"]; !ok || !reflect.DeepEqual(got, wantDeferred) {
		t.Errorf("jaekit:violation=%s deferred_findings is %v, want %v", id, got, wantDeferred)
	}
	if got, ok := gate["unhandled_findings"]; !ok || !reflect.DeepEqual(got, wantUnhandled) {
		t.Errorf("jaekit:violation=%s unhandled_findings is %v, want %v", id, got, wantUnhandled)
	}

	// The other fields keep their keys and values.
	var keys []string
	for k := range gate {
		if k != "deferred_findings" && k != "unhandled_findings" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	wantKeys := []string{"auto_deferred", "expect_head", "growth", "last_reviewed_head", "open", "open_findings",
		"previous_gate_digest", "reasons", "rounds", "schema", "spar_version", "state", "written_at"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("jaekit:violation=%s the gate's other keys are %v, want %v", id, keys, wantKeys)
	}
	open, _ := gate["open"].(map[string]any)
	findings, _ := gate["open_findings"].([]any)
	if gate["schema"] != GateSchema || gate["state"] != StateBlocked || gate["auto_deferred"] != float64(1) ||
		open["defect"] != float64(1) || open["requirement-question"] != float64(0) ||
		len(findings) != 1 || findings[0] != a+" native-F7" || code != ExitBlocked {
		t.Errorf("jaekit:violation=%s the gate's other fields changed: schema %v, state %v (exit %d), auto_deferred %v, open %v, open_findings %v",
			id, gate["schema"], gate["state"], code, gate["auto_deferred"], gate["open"], gate["open_findings"])
	}
}

// AC-9: the round summary's finding table shows every claim whole, with its
// whitespace collapsed and `|` escaped, `spar summary` prints the file, and
// a summary written in the old form takes the new form when a disposition
// rewrites it.
func TestReviewFindingsReportAC9_SummaryKeepsWholeClaims(t *testing.T) {
	const id = "summary-claim-cut"
	w := newRFWorld(t)
	claim := strings.Repeat("a long claim | with a pipe and  spaces\nacross lines ", 12) + "and its end"
	whole := strings.ReplaceAll(strings.Join(strings.Fields(claim), " "), "|", "\\|")
	if len([]rune(whole)) < 600 {
		rfSetup(t, "the claim is %d characters, not over 600", len([]rune(whole)))
	}
	out := w.round("s1", "01-candidate", rawFinding("F1", map[string]any{"proposed_class": "defect", "link_kind": "criterion",
		"link": "AC-1", "location": "head/app/main.py:1", "claim": claim}))
	read := func() string {
		data, err := os.ReadFile(filepath.Join(out, "summary.md"))
		if err != nil {
			rfSetup(t, "%v", err)
		}
		return string(data)
	}
	row := func(summary string) string {
		for _, line := range strings.Split(summary, "\n") {
			if strings.HasPrefix(line, "| native-F1 |") {
				return line
			}
		}
		return ""
	}
	summary := read()
	if r := row(summary); !strings.HasSuffix(r, "| "+whole+" |") {
		t.Errorf("jaekit:violation=%s the summary row does not end with the whole claim: %q", id, r)
	}
	if _, printed, _ := w.spar("summary", out); printed != summary {
		t.Errorf("jaekit:violation=%s spar summary does not print summary.md as it is", id)
	}
	// A summary from before this change cuts the claim; the next
	// disposition rewrites it in the new form.
	old := strings.Replace(summary, whole, string([]rune(whole)[:199])+"…", 1)
	if err := os.WriteFile(filepath.Join(out, "summary.md"), []byte(old), 0o644); err != nil {
		rfSetup(t, "%v", err)
	}
	w.must("triage", out, "native-F1", "reject", "--evidence", "head/app/main.py already does it")
	if r := row(read()); !strings.HasSuffix(r, "| "+whole+" |") {
		t.Errorf("jaekit:violation=%s after a disposition the summary row does not end with the whole claim: %q", id, r)
	}
}

// AC-10 (kept): a later round's packet shows an earlier round's finding on
// one line with its claim cut at 300 characters, as before this change. A
// broken setup fails this check of kept behavior.
func TestReviewFindingsReportAC10_PriorLinesKeepTheirCut(t *testing.T) {
	w := newRFKeptWorld(t)
	claim := strings.Repeat("a long claim | with a pipe and  spaces\nacross lines ", 12) + "and its end"
	w.round("s1", "01-candidate", rawFinding("F1", map[string]any{"proposed_class": "defect", "link_kind": "criterion",
		"link": "AC-1", "location": "head/app/main.py:1", "claim": claim}))
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('three')\n")
	w.git("commit", "-qam", "fix")
	fixed := w.git("rev-parse", "HEAD")
	out := filepath.Join(w.repo, "reviews", "s1", "02-fixes")
	if code, _, msg := w.spar("run", "--stage", "fixes", "--base", w.head, "--head", fixed, "--out", out, "--criteria", w.criteria,
		"--check-path", "checks/**", "--repo", w.repo); code != ExitOK {
		t.Fatalf("setup: spar run fixes exits %d: %s", code, msg)
	}
	packet, err := os.ReadFile(filepath.Join(out, "packet.md"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	collapsed := []rune(strings.Join(strings.Fields(claim), " "))
	want := "01-candidate native-F1 [defect] at head/app/main.py:1: " + string(collapsed[:299]) + "…"
	if !strings.Contains(string(packet), want) {
		t.Errorf("the fixes packet does not hold the earlier finding line %q", want)
	}
}
