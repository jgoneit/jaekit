//go:build !windows

package spar

// Checks of the spar-stage-merge goal in the spar package. Every test drives
// spar.Main, the entry cmd/spar calls, with string arguments, on a synthetic
// repository whose commits never change (fixed names, emails and dates), and
// rounds whose only reviewer is a native one that the tests answer with spar
// record: no reviewer CLI or model runs, and the file builds on the commit
// before the change. A change test (AC-3, AC-5) gathers what it observes and
// reports a violation in one failure whose first line starts with
// jaekit:violation=<id>: . A setup that breaks (no git, a file, repository or
// round that cannot be written or read, an exit of spar the setup does not
// expect) is an unmarked t.Fatalf before any violation is reported, so an
// input failure is never a pass or a declared violation. The kept behaviors
// (AC-6, AC-7, AC-8) are checked without a marker.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// smGoldenEnv, set to "update", makes TestSparStageMergeAC7 write its golden
// files instead of comparing with them.
const smGoldenEnv = "SPAR_STAGE_MERGE_GOLDEN"

// smLegacyDir holds the legacy series fixture and its golden outputs.
var smLegacyDir = filepath.Join("testdata", "legacy-series")

// smLegacyCommits are the commits c0 to c3 of the synthetic repository. The
// legacy fixture names them, so the repository must give them again.
var smLegacyCommits = []string{
	"6f153db7ffe18958da1d62a0844e1db0ffd42547",
	"8a807f9e98214f58b30fc0199bfe4b57d7496ea8",
	"93cac67dc43b88547e3c13cb3f9d4dc3f373d3ff",
	"fdab52c8730c3b39fa79414b1d1fea95272aa0d6",
}

// smViolations gathers what a change test observes against its condition
// and reports it as one marked failure.
type smViolations struct {
	t    *testing.T
	id   string
	list []string
}

func (v *smViolations) add(format string, args ...any) {
	v.list = append(v.list, fmt.Sprintf(format, args...))
}

func (v *smViolations) report() {
	v.t.Helper()
	if len(v.list) > 0 {
		v.t.Errorf("jaekit:violation=%s: %s", v.id, strings.Join(v.list, "\n"))
	}
}

// smWorld is a repository of four commits under dir/repo, the goal's
// criteria and plan beside it, and a user configuration with one native
// reviewer, host.
type smWorld struct {
	t        *testing.T
	dir      string
	repo     string
	commits  []string
	criteria string
	rules    string
	plan     string
	config   string
	version  string
	files    int
}

// smGitEnv is the environment of the world's git commands: no user or
// system configuration, SHA-1 objects, and the fixed names, emails and the
// date of commit n, so that the commit IDs never change.
func smGitEnv(n int) []string {
	date := fmt.Sprintf("%d +0000", 1767225600+3600*n)
	return append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_DEFAULT_HASH=sha1",
		"GIT_AUTHOR_NAME=Synthetic Author", "GIT_AUTHOR_EMAIL=author@example.invalid", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=Synthetic Committer", "GIT_COMMITTER_EMAIL=committer@example.invalid", "GIT_COMMITTER_DATE="+date)
}

func newSMWorld(t *testing.T) *smWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("setup: git is not available: %v", err)
	}
	w := &smWorld{t: t, dir: realish(t.TempDir()), version: "test"}
	w.repo = filepath.Join(w.dir, "repo")
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n")
	w.rules = w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\n- A greeting never ends with a comma.\n")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello ' + name\n")
	w.write(filepath.Join(w.repo, "app", "other.py"), "def other():\n    return 1\n")
	w.write(filepath.Join(w.repo, "checks", "test_greet.py"), "def test_greet():\n    assert True\n")
	w.git(0, "init", "-q")
	w.commit(0)
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello, ' + name\n")
	w.commit(1)
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello, ' + name if name else 'hello'\n")
	w.write(filepath.Join(w.repo, "checks", "test_greet.py"),
		"def test_greet():\n    assert True\n\ndef test_empty():\n    assert greet('') == 'hello'\n")
	w.commit(2)
	w.write(filepath.Join(w.repo, "app", "other.py"), "def other():\n    return 2\n")
	w.commit(3)
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "# Goal\n\n## Acceptance Criteria\n"+
		"- **AC-1** greet('') returns 'hello'.\n- **AC-2** other() returns 1.\n- **AC-3** greet never fails.\n")
	w.plan = w.write(filepath.Join(w.dir, "PLAN.md"), "# Plan\n\nChange the greeting and keep other() as it is.\n")
	w.config = w.write(filepath.Join(w.dir, "spar.toml"), "[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	return w
}

func (w *smWorld) git(n int, args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.DevNull,
		"-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = smGitEnv(n)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		w.t.Fatalf("setup: git %v: %v %s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

func (w *smWorld) commit(n int) {
	w.t.Helper()
	w.git(n, "add", "-A")
	w.git(n, "commit", "-q", "-m", fmt.Sprintf("c%d", n))
	w.commits = append(w.commits, w.git(n, "rev-parse", "HEAD"))
}

func (w *smWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return path
}

func (w *smWorld) read(path string) []byte {
	w.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return data
}

// exists reports whether path exists; an error other than its absence is a
// broken setup.
func (w *smWorld) exists(path string) bool {
	w.t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.t.Fatalf("setup: %v", err)
	}
	return err == nil
}

// spar runs the spar command line with the world's configuration and
// returns its exit code, standard output and standard error.
func (w *smWorld) spar(args ...string) (int, string, string) {
	var stdout, stderr strings.Builder
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
	code := Main(w.version, args, strings.NewReader(""), &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

// must runs spar for the setup; any other exit than want is a broken setup.
func (w *smWorld) must(want int, args ...string) string {
	w.t.Helper()
	code, out, errs := w.spar(args...)
	if code != want {
		w.t.Fatalf("setup: spar %v: exit %d, want %d: %s%s", args, code, want, out, errs)
	}
	return out
}

// runArgs are the arguments of spar run for a round of stage from base to
// head into out, with the plan as subject of a plan or plan-delta round.
func (w *smWorld) runArgs(stage, base, head, out string) []string {
	args := []string{"run", "--stage", stage, "--base", base, "--head", head, "--out", out,
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--repo", w.repo}
	if stage == "plan" || stage == "plan-delta" {
		args = append(args, "--subject", w.plan)
	}
	return args
}

// run starts a round of the native reviewer in series/name; spar must
// write it.
func (w *smWorld) run(series, name, stage, base, head string, extra ...string) string {
	w.t.Helper()
	out := filepath.Join(series, name)
	w.must(ExitOK, append(w.runArgs(stage, base, head, out), extra...)...)
	return out
}

// next is the folder name of the next round of stage in series: its
// sequence, as spar gives it, and the stage.
func (w *smWorld) next(series, stage string) string {
	w.t.Helper()
	entries, err := os.ReadDir(series)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.t.Fatalf("setup: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && w.exists(filepath.Join(series, e.Name(), "round.json")) {
			n++
		}
	}
	return fmt.Sprintf("%02d-%s", n+1, stage)
}

// record answers the native reviewer host of a round whose head is head
// with the findings given.
func (w *smWorld) record(round, head string, findings ...map[string]any) {
	w.t.Helper()
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	list := []any{}
	for _, f := range findings {
		list = append(list, f)
	}
	verdict := "no_blocking_findings"
	if len(list) > 0 {
		verdict = "blocking_findings"
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": head, "verdict": verdict, "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": list, "limits": "synthetic reviewer"})
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	w.files++
	file := w.write(filepath.Join(w.dir, "results", fmt.Sprintf("%d.json", w.files)), string(data))
	w.must(ExitOK, "record", round, "host", "--result", file)
}

func (w *smWorld) triage(round, finding, disposition string, args ...string) {
	w.t.Helper()
	w.must(ExitOK, append([]string{"triage", round, finding, disposition}, args...)...)
}

// rawRound reads round.json as JSON, so that a check reads the fields spar
// wrote whatever its own types are.
func (w *smWorld) rawRound(dir string) map[string]any {
	w.t.Helper()
	var r map[string]any
	if err := json.Unmarshal(w.read(filepath.Join(dir, "round.json")), &r); err != nil {
		w.t.Fatalf("setup: round.json of %s: %v", filepath.Base(dir), err)
	}
	return r
}

// smFinding is a reviewer's finding of the given proposed class at line 1
// of path in the reviewed head, linked to a criterion when link is set.
func smFinding(id, class, link, path, introduced string) map[string]any {
	kind := "criterion"
	if link == "" {
		kind = "none"
	}
	return map[string]any{"id": id, "proposed_class": class, "angle": "requirement-counterexample",
		"location": "head/" + path + ":1", "claim": "synthetic claim " + id, "failure_scenario": "synthetic failure " + id,
		"evidence": "head/" + path + ":1", "link_kind": kind, "link": link, "introduced_by_change": introduced,
		"regression": "no", "check_impact": "none", "confidence": "high"}
}

// smCaps are the caps of a configuration as JSON gives them: config.caps of
// a round record, caps of spar config.
func smCaps(cfg map[string]any) map[string]any {
	caps, _ := cfg["caps"].(map[string]any)
	return caps
}

// smConfig runs spar config in the world's repository and decodes it; an
// exit other than 0 or output that is not one JSON object returns nil and
// the exit.
func (w *smWorld) smConfig() (map[string]any, int, string) {
	code, out, errs := w.spar("config", "--repo", w.repo)
	if code != ExitOK {
		return nil, code, errs
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		w.t.Fatalf("setup: spar config printed no JSON object: %v\n%s", err, out)
	}
	return cfg, code, errs
}

func smFirstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// smSnapshot is every entry under dir by its path from dir: a folder as
// "<path>/", a file with its mode and bytes, a symbolic link with its
// target.
func (w *smWorld) smSnapshot(dir string) map[string]string {
	w.t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			snap[rel] = "-> " + target
		case d.IsDir():
			snap[rel+"/"] = ""
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			snap[rel] = info.Mode().String() + "\n" + string(data)
		}
		return nil
	})
	if err != nil {
		w.t.Fatalf("setup: the files of %s cannot be read: %v", dir, err)
	}
	return snap
}

// smChanged names the entries that differ between two snapshots.
func smChanged(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if old, ok := before[k]; !ok {
			out = append(out, "added "+k)
		} else if old != v {
			out = append(out, "changed "+k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			out = append(out, "removed "+k)
		}
	}
	sort.Strings(out)
	if len(out) > 8 {
		out = append(out[:8], fmt.Sprintf("and %d more", len(out)-8))
	}
	return out
}

// smUsageRun is the run line the usage text gives once only plan and
// candidate rounds start; smUsageOldRun is the line before the change.
const (
	smUsageRun    = "spar run --stage plan|candidate --base REV"
	smUsageOldRun = "spar run --stage plan|plan-delta|candidate|fixes"
)

// smUsage adds to v what the usage text printed in stderr does wrong: it
// lacks the run line for plan and candidate only, or keeps the old one.
func smUsage(v *smViolations, label, stderr string) {
	if !strings.Contains(stderr, smUsageRun) {
		v.add("%s prints a usage text without the line %q", label, smUsageRun)
	}
	if strings.Contains(stderr, smUsageOldRun) {
		v.add("%s prints a usage text with the old line %q", label, smUsageOldRun)
	}
}

// smTryOldStage runs one round of stage (plan-delta or fixes) from base to
// head into series, where it is valid but for its stage, and adds to v what
// spar did other than refuse it as a usage error (exit 64) whose message
// names candidate and whose usage text lists only plan and candidate,
// before anything is written: an --out that exists, a series that changed,
// or a series folder that did not exist and now does.
func (w *smWorld) smTryOldStage(v *smViolations, where, series, stage, base, head string, dry bool) {
	w.t.Helper()
	label := "spar run --stage " + stage
	args := w.runArgs(stage, base, head, "")
	if dry {
		label += " --dry-run"
		args = append(args, "--dry-run")
	}
	label += " in " + where
	out := filepath.Join(series, w.next(series, stage))
	args[slices.Index(args, "--out")+1] = out
	existed := w.exists(series)
	var before map[string]string
	if existed {
		before = w.smSnapshot(series)
	}
	code, _, stderr := w.spar(args...)
	switch {
	case code == ExitOK || code == ExitNoResult:
		v.add("%s started a round (exit %d)", label, code)
	case code != ExitUsage:
		v.add("%s exits %d, want 64: %s", label, code, smFirstLine(stderr))
	case !strings.Contains(smFirstLine(stderr), "candidate"):
		v.add("%s is refused with %q, which does not name candidate", label, smFirstLine(stderr))
	}
	if code == ExitUsage {
		smUsage(v, label, stderr)
	}
	if w.exists(out) {
		v.add("%s wrote %s", label, filepath.Base(out))
	}
	switch {
	case !existed && w.exists(series):
		v.add("%s created the series folder %s", label, filepath.Base(series))
	case existed:
		if changed := smChanged(before, w.smSnapshot(series)); len(changed) > 0 {
			v.add("%s changed the series: %s", label, strings.Join(changed, ", "))
		}
	}
}

// AC-3: a new round of the stage plan-delta or fixes is refused as a usage
// error (exit 64) whose message names candidate, before anything is
// written, also as a dry run: in a series with a recorded candidate round,
// in a series folder that does not exist yet and in a series recorded
// before the change with plan-delta and fixes rounds (the legacy fixture).
// The usage text lists only plan and candidate for spar run; plan and
// candidate rounds still start.
func TestSparStageMergeAC3_OldStagesRefused(t *testing.T) {
	v := &smViolations{t: t, id: "old-stage-started"}
	w := newSMWorld(t)
	c := w.commits
	// Each run is valid but for its stage: the plan-delta round reviews the
	// plan, the fixes round continues the chain from the head of the
	// series' last code round.
	tryAll := func(where string, series func(i int) string, fixesBase, fixesHead string) {
		for i, form := range []struct {
			stage, base, head string
			dry               bool
		}{
			{"plan-delta", c[0], c[0], false},
			{"plan-delta", c[0], c[0], true},
			{"fixes", fixesBase, fixesHead, false},
			{"fixes", fixesBase, fixesHead, true},
		} {
			w.smTryOldStage(v, where, series(i), form.stage, form.base, form.head, form.dry)
		}
	}

	series := filepath.Join(w.repo, "reviews", "old-stages")
	first := w.run(series, "01-candidate", "candidate", c[0], c[1])
	w.record(first, c[1], smFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	tryAll("a series with a recorded candidate round", func(int) string { return series }, c[1], c[2])

	// A series folder of its own for each run, so that every run meets one
	// that does not exist yet.
	tryAll("a series folder that does not exist", func(i int) string {
		return filepath.Join(w.repo, "reviews", fmt.Sprintf("fresh-%d", i+1))
	}, c[1], c[2])

	if !slices.Equal(c, smLegacyCommits) {
		t.Fatalf("setup: the synthetic repository has the commits %v, want %v that the legacy fixture names", c, smLegacyCommits)
	}
	legacy := filepath.Join(w.repo, "reviews", "legacy")
	w.smCopyFixture(legacy)
	// 05-candidate, the legacy series' last code round, reviewed c2.
	tryAll("the legacy series with plan-delta and fixes rounds", func(int) string { return legacy }, c[2], c[3])

	// The usage text of any usage error of spar run.
	code, _, stderr := w.spar("run", "--no-such-option")
	if code != ExitUsage {
		t.Fatalf("setup: spar run --no-such-option exits %d, want 64: %s", code, smFirstLine(stderr))
	}
	smUsage(v, "spar run --no-such-option", stderr)

	for _, form := range []struct{ stage, base, head string }{{"plan", c[0], c[0]}, {"candidate", c[0], c[2]}} {
		out := filepath.Join(series, w.next(series, form.stage))
		code, _, stderr := w.spar(w.runArgs(form.stage, form.base, form.head, out)...)
		if code != ExitOK || !w.exists(filepath.Join(out, "round.json")) {
			t.Errorf("spar run --stage %s in the series exits %d and wrote round.json: %t: %s; want a started round",
				form.stage, code, w.exists(filepath.Join(out, "round.json")), smFirstLine(stderr))
		}
	}
	v.report()
}

// AC-5: without a project configuration the caps are one plan round and two
// code rounds: a third counted candidate round is refused at the code cap,
// a second plan round at the plan cap, and only an extension with the
// user's words lets a third candidate round start.
func TestSparStageMergeAC5_DefaultCodeCapTwo(t *testing.T) {
	v := &smViolations{t: t, id: "code-cap-not-two"}
	w := newSMWorld(t)
	c := w.commits
	cfg, code, stderr := w.smConfig()
	if cfg == nil {
		t.Fatalf("setup: spar config exits %d: %s", code, stderr)
	}
	if caps := smCaps(cfg); caps["plan"] != float64(1) || caps["code"] != float64(2) {
		v.add("spar config without a project configuration shows the caps %v, want plan 1 and code 2", caps)
	}
	series := filepath.Join(w.repo, "reviews", "default-caps")
	candidate := func(base, head string) (string, int, string) {
		out := filepath.Join(series, w.next(series, "candidate"))
		code, _, stderr := w.spar(w.runArgs("candidate", base, head, out)...)
		return out, code, stderr
	}
	for _, head := range []string{c[1], c[2]} {
		out, code, stderr := candidate(c[0], head)
		if code != ExitOK {
			v.add("candidate round %s exits %d: %s; want it started under the default code cap", filepath.Base(out), code, smFirstLine(stderr))
		} else if r := w.rawRound(out); r["counted"] != true {
			v.add("candidate round %s is recorded with counted %v, want true", filepath.Base(out), r["counted"])
		}
	}
	third, code, stderr := candidate(c[0], c[3])
	switch {
	case code == ExitOK || code == ExitNoResult:
		v.add("the third counted candidate round started (exit %d); want the code cap refusal (65) naming 2 of 2", code)
	case code != ExitRefused || !strings.Contains(stderr, "2 of 2"):
		v.add("the third counted candidate round exits %d: %s; want the code cap refusal (65) naming 2 of 2", code, smFirstLine(stderr))
	}
	if code != ExitOK && w.exists(third) {
		v.add("the refused third candidate round wrote %s", filepath.Base(third))
	}

	plan := filepath.Join(series, w.next(series, "plan"))
	if code, _, stderr := w.spar(w.runArgs("plan", c[0], c[0], plan)...); code != ExitOK {
		v.add("the first plan round exits %d: %s; want it started", code, smFirstLine(stderr))
	}
	again := filepath.Join(series, w.next(series, "plan"))
	if code, _, stderr := w.spar(w.runArgs("plan", c[0], c[1], again)...); code != ExitRefused || !strings.Contains(stderr, "plan cap") {
		v.add("a second plan round exits %d: %s; want the plan cap refusal (65)", code, smFirstLine(stderr))
	}
	if w.exists(again) {
		v.add("the refused second plan round wrote %s", filepath.Base(again))
	}

	exts := filepath.Join(series, "extensions.jsonl")
	if w.exists(exts) {
		t.Fatalf("setup: %s has extensions before any spar extend", series)
	}
	extend := []string{"extend", series, "--stage", "code", "--by", "1", "--cause", "churn", "--repo", w.repo}
	if code, _, _ := w.spar(extend...); code == ExitOK || w.exists(exts) {
		v.add("spar extend without --user-quote exits %d and records an extension: %t; want it refused with nothing recorded",
			code, w.exists(exts))
	}
	if code, _, stderr := w.spar(append(extend, "--user-quote", "synthetic words of the user")...); code != ExitOK {
		v.add("spar extend with --user-quote exits %d: %s", code, smFirstLine(stderr))
	}
	extended, code, stderr := candidate(c[1], c[3])
	if code != ExitOK {
		v.add("a candidate round after the extension exits %d: %s; want it started", code, smFirstLine(stderr))
	} else if r := w.rawRound(extended); r["counted"] != true {
		v.add("the candidate round after the extension is recorded with counted %v, want true", r["counted"])
	}
	v.report()
}

// AC-6: a series of a plan round and two candidate rounds, the last from the
// original base to the new head with the first one's defect fixed, gives a
// complete gate without reasons or fixes growth.
func TestSparStageMergeAC6_PlanAndCandidateGate(t *testing.T) {
	w := newSMWorld(t)
	c := w.commits
	series := filepath.Join(w.repo, "reviews", "merged")
	plan := w.run(series, "01-plan", "plan", c[0], c[0])
	w.record(plan, c[0])
	first := w.run(series, "02-candidate", "candidate", c[0], c[1])
	w.record(first, c[1], smFinding("F1", "defect", "AC-1", "app/greet.py", "yes"))
	w.triage(first, "host-F1", "fix", "--commit", c[2], "--failed-before", "synthetic check failed",
		"--passed-after", "synthetic check passed")
	last := w.run(series, "03-candidate", "candidate", c[0], c[2])
	w.record(last, c[2])

	out := filepath.Join(w.dir, "gate.json")
	code, _, stderr := w.spar("gate", plan, first, last, "--expect-head", c[2], "--out", out, "--repo", w.repo)
	var g map[string]any
	if err := json.Unmarshal(w.read(out), &g); err != nil {
		t.Fatalf("setup: the gate file is not JSON (exit %d, %s): %v", code, stderr, err)
	}
	reasons, _ := g["reasons"].([]any)
	if code != ExitOK || g["state"] != StateComplete || reasons == nil || len(reasons) != 0 {
		t.Errorf("spar gate over 01-plan, 02-candidate and 03-candidate exits %d with state %v and reasons %v: %s; want complete without reasons",
			code, g["state"], g["reasons"], smFirstLine(stderr))
	}
	growth, _ := g["growth"].(map[string]any)
	rounds, ok := growth["rounds"].([]any)
	if !ok || len(rounds) != 0 || growth["over_hardening"] != false {
		t.Errorf("the gate's growth is %v; want no rounds and over_hardening false", g["growth"])
	}
}

// smLegacyRounds are the legacy series' rounds a gate lists: every one but
// the dry run 06-fixes.
var smLegacyRounds = []string{"01-plan", "02-plan-delta", "03-candidate", "04-fixes", "05-candidate"}

// smCopyFixture copies the legacy series fixture into series, with the
// world's folder in place of @DIR@.
func (w *smWorld) smCopyFixture(series string) {
	w.t.Helper()
	src := filepath.Join(smLegacyDir, "series")
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(series, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(series, rel), []byte(strings.ReplaceAll(string(data), "@DIR@", w.dir)), 0o644)
	})
	if err != nil {
		w.t.Fatalf("setup: the legacy series fixture cannot be copied: %v", err)
	}
	for _, name := range append(slices.Clone(smLegacyRounds), "06-fixes") {
		for _, file := range []string{"round.json", "findings.json", "ledger.jsonl"} {
			if !w.exists(filepath.Join(series, name, file)) {
				w.t.Fatalf("setup: the legacy series fixture has no %s/%s", name, file)
			}
		}
	}
}

// smWrittenAt is the gate's own time, the one field of its output that
// changes from run to run.
var smWrittenAt = regexp.MustCompile(`(?m)^  "written_at": "[^"]*"`)

// smNormalize puts @DIR@ in place of the world's folder, and for the gate
// @WRITTEN_AT@ in place of its written_at.
func (w *smWorld) smNormalize(out string) string {
	out = strings.ReplaceAll(out, w.dir, "@DIR@")
	return smWrittenAt.ReplaceAllString(out, `  "written_at": "@WRITTEN_AT@"`)
}

// smGolden compares got with the golden file name, or writes it when
// SPAR_STAGE_MERGE_GOLDEN is update.
func smGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(smLegacyDir, name)
	if os.Getenv(smGoldenEnv) == "update" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Logf("wrote %s", path)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: the golden file %s cannot be read: %v", path, err)
	}
	if want := string(data); got != want {
		gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
		i := 0
		for i < len(gl) && i < len(wl) && gl[i] == wl[i] {
			i++
		}
		line := func(lines []string) string {
			if i < len(lines) {
				return strconv.Quote(lines[i])
			}
			return "the end"
		}
		t.Errorf("%s differs from the legacy series' golden %s at line %d: got %s, want %s", name, path, i+1, line(gl), line(wl))
	}
}

// AC-7: a series recorded before the change, with plan-delta and fixes
// rounds, a dry run and a code extension, is read by spar gate and spar
// cap-report byte for byte as the code before the change read it.
func TestSparStageMergeAC7_LegacySeriesReadUnchanged(t *testing.T) {
	w := newSMWorld(t)
	if !slices.Equal(w.commits, smLegacyCommits) {
		t.Fatalf("setup: the synthetic repository has the commits %v, want %v that the legacy fixture names", w.commits, smLegacyCommits)
	}
	series := filepath.Join(w.repo, "reviews", "legacy")
	w.smCopyFixture(series)

	out := filepath.Join(w.dir, "gate.json")
	args := []string{"gate"}
	for _, name := range smLegacyRounds {
		args = append(args, filepath.Join(series, name))
	}
	code, stdout, stderr := w.spar(append(args, "--expect-head", w.commits[2], "--out", out, "--repo", w.repo)...)
	if code != ExitOK || stdout != "" || stderr != "" {
		t.Errorf("spar gate over the legacy series exits %d with output %q %q; want 0 and none", code, stdout, stderr)
	}
	if w.exists(out) {
		smGolden(t, "gate.golden.json", w.smNormalize(string(w.read(out))))
	} else {
		t.Errorf("spar gate over the legacy series wrote no gate file")
	}
	for _, format := range []struct{ name, golden string }{{"md", "cap-report.golden.md"}, {"json", "cap-report.golden.json"}} {
		code, stdout, stderr := w.spar("cap-report", series, "--format", format.name, "--repo", w.repo)
		if code != ExitOK || stderr != "" {
			t.Errorf("spar cap-report --format %s of the legacy series exits %d: %s", format.name, code, stderr)
			continue
		}
		smGolden(t, format.golden, w.smNormalize(stdout))
	}
}

// AC-8: a project configuration that still sets caps.plan_delta loads; a
// candidate round records it, and it lends nothing to the plan or code cap:
// with plan_delta 5 the plan cap stays 1 and the code cap 2.
func TestSparStageMergeAC8_OldPlanDeltaCapLoads(t *testing.T) {
	w := newSMWorld(t)
	c := w.commits
	w.write(filepath.Join(w.repo, ".jaekit", "spar.toml"), "[caps]\nplan = 1\nplan_delta = 5\ncode = 2\n")
	cfg, code, stderr := w.smConfig()
	if cfg == nil {
		t.Fatalf("spar config with caps.plan_delta in the project configuration exits %d: %s; want it loaded", code, stderr)
	}
	if caps := smCaps(cfg); caps["plan_delta"] != float64(5) || caps["plan"] != float64(1) || caps["code"] != float64(2) {
		t.Errorf("spar config shows the caps %v, want plan 1, plan_delta 5 and code 2 from the project configuration", caps)
	}
	series := filepath.Join(w.repo, "reviews", "old-config")
	for _, head := range []string{c[1], c[2]} {
		out := filepath.Join(series, w.next(series, "candidate"))
		code, _, stderr := w.spar(w.runArgs("candidate", c[0], head, out)...)
		if code != ExitOK {
			t.Errorf("candidate round %s under the project configuration exits %d: %s; want it started", filepath.Base(out), code, smFirstLine(stderr))
			continue
		}
		r := w.rawRound(out)
		recorded, _ := r["config"].(map[string]any)
		if caps := smCaps(recorded); caps["plan_delta"] != float64(5) || caps["code"] != float64(2) || r["counted"] != true {
			t.Errorf("candidate round %s records config.caps %v and counted %v; want plan_delta 5, code 2 and counted",
				filepath.Base(out), caps, r["counted"])
		}
	}
	third := filepath.Join(series, w.next(series, "candidate"))
	code, _, stderr = w.spar(w.runArgs("candidate", c[0], c[3], third)...)
	if code != ExitRefused || !strings.Contains(stderr, "2 of 2") || w.exists(third) {
		t.Errorf("the third candidate round under code = 2 exits %d and wrote its folder: %t: %s; want the code cap refusal (65) naming 2 of 2",
			code, w.exists(third), smFirstLine(stderr))
	}
	plan := filepath.Join(series, w.next(series, "plan"))
	if code, _, stderr := w.spar(w.runArgs("plan", c[0], c[0], plan)...); code != ExitOK {
		t.Errorf("the first plan round under plan = 1 exits %d: %s; want it started", code, smFirstLine(stderr))
	}
	again := filepath.Join(series, w.next(series, "plan"))
	code, _, stderr = w.spar(w.runArgs("plan", c[0], c[1], again)...)
	if code != ExitRefused || !strings.Contains(stderr, "plan cap") || !strings.Contains(stderr, "1 of 1") || w.exists(again) {
		t.Errorf("a second plan round under plan = 1 and plan_delta = 5 exits %d and wrote its folder: %t: %s; want the plan cap refusal (65) naming 1 of 1",
			code, w.exists(again), smFirstLine(stderr))
	}
}
