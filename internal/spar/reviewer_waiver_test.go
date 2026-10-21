package spar

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// These tests observe the user's recorded decision to go on without a
// required reviewer in one round (a waiver): how `spar waive` records and
// refuses it, and how `spar gate` reads it. They drive the spar command line
// (Main) on a synthetic repository. Native reviewers answer with JSON the
// tests record; agy and command reviewers point at executables that do not
// exist, so spar records them without starting anything. The gate tests
// write a waiver line in the form the round contract gives (section 10), so
// the gate is observed on its own; the AC-1 test records one with `spar
// waive` and reads it back through the gate. The opening line of an
// assertion's failure message names the property it observes
// (jaekit:violation=<id>); a failure without it is a broken setup, not an
// observation.

const rwQuote = "synthetic user decision: go on without this reviewer"

// rwRecordedAt is the recording time of a waiver line these tests write.
const rwRecordedAt = "2026-01-01T00:00:00Z"

type rwWorld struct {
	t                *testing.T
	dir, repo        string
	base, head, fix  string
	criteria, config string
	series, configs  int
	replies          int
}

// newRWWorld makes a repository with three commits (base, head and fix) and
// a criteria document with AC-1. Without git, a check of a change condition
// is skipped and a check of a kept behavior fails, so neither passes.
func newRWWorld(t *testing.T, keeps bool) *rwWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if keeps {
			t.Fatalf("git is not available: %v", err)
		}
		t.Skipf("git is not available: %v", err)
	}
	w := &rwWorld{t: t, dir: t.TempDir()}
	w.repo = realish(filepath.Join(w.dir, "repo"))
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n/.jaekit/\n")
	w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\nKeep reviews honest.\n")
	w.write(filepath.Join(w.repo, "app/main.py"), "print('one')\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app/main.py"), "print('two')\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app/main.py"), "print('three')\n")
	w.git("commit", "-qam", "fix")
	w.fix = w.git("rev-parse", "HEAD")
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "- **AC-1** one\n")
	return w
}

func (w *rwWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatal(err)
	}
	return path
}

func (w *rwWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Reviewer entries of the user's configuration.

func rwNative(name string) string {
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = \"native\"\n\n", name)
}

// missingAgy is an agy reviewer whose executable does not exist: spar
// records it as unavailable.
func (w *rwWorld) missingAgy() string {
	return fmt.Sprintf("[[reviewers]]\nname = \"agy\"\npreset = \"agy\"\nbinary = %q\nfallback_model = \"\"\n\n",
		filepath.Join(w.dir, "missing", "agy"))
}

// brokenAgy is an agy reviewer whose executable cannot run: spar records it
// as failed.
func (w *rwWorld) brokenAgy() string {
	path := w.write(filepath.Join(w.dir, "broken", "agy"), "not a program\n")
	return fmt.Sprintf("[[reviewers]]\nname = \"agy\"\npreset = \"agy\"\nbinary = %q\nfallback_model = \"\"\n\n", path)
}

// missingCommand is a command reviewer whose program does not exist: it is
// unavailable and never a required reviewer.
func (w *rwWorld) missingCommand(name string) string {
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = \"command\"\nargv = [%q, \"{prompt}\"]\n\n",
		name, filepath.Join(w.dir, "missing", name))
}

// use makes the given reviewers the user's configuration of later runs.
func (w *rwWorld) use(reviewers ...string) {
	w.t.Helper()
	w.configs++
	w.config = w.write(filepath.Join(w.dir, fmt.Sprintf("spar-%d.toml", w.configs)), strings.Join(reviewers, ""))
}

func (w *rwWorld) getenv(k string) string {
	switch k {
	case "SPAR_CONFIG":
		return w.config
	case "HOME":
		return w.dir
	}
	return ""
}

// spar runs the spar command line and returns its exit code and messages.
func (w *rwWorld) spar(args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
	code := Main("test", args, strings.NewReader(""), &stdout, &stderr, w.getenv)
	return code, stdout.String() + stderr.String()
}

// newSeries names a new series folder.
func (w *rwWorld) newSeries() string {
	w.series++
	return filepath.Join(w.repo, "reviews", fmt.Sprintf("s%d", w.series))
}

// run runs a round into out, a candidate round from base to head unless the
// options say otherwise. spar run exits 1 when no reviewer completed or
// waits, which still writes the round.
func (w *rwWorld) run(out string, options ...string) string {
	w.t.Helper()
	args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out,
		"--criteria", w.criteria, "--repo", w.repo}, options...)
	code, msg := w.spar(args...)
	if code != ExitOK && code != 1 {
		w.t.Fatalf("spar run %v: %d %s", options, code, msg)
	}
	if _, err := os.Stat(filepath.Join(out, "round.json")); err != nil {
		w.t.Fatalf("spar run %v wrote no round: %v", options, err)
	}
	return out
}

// record records a native reviewer's answer to a round's packet.
func (w *rwWorld) record(out, name string, findings ...map[string]any) {
	w.t.Helper()
	r, err := loadRound(out)
	if err != nil {
		w.t.Fatal(err)
	}
	var checked []any
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
	data, err := json.Marshal(map[string]any{"reviewed_head": r.Head, "verdict": verdict, "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": list, "limits": ""})
	if err != nil {
		w.t.Fatal(err)
	}
	w.replies++
	reply := w.write(filepath.Join(w.dir, "replies", fmt.Sprintf("%d.json", w.replies)), string(data)+"\n")
	if code, msg := w.spar("record", out, name, "--result", reply); code != ExitOK {
		w.t.Fatalf("spar record %s: %d %s", name, code, msg)
	}
}

// waive writes the user's decision for a reviewer of a round in the form
// the round contract gives (section 10): one line of the round's
// waivers.jsonl.
func (w *rwWorld) waive(out, name, preset, status string) {
	w.t.Helper()
	line, err := json.Marshal(map[string]any{"round": filepath.Base(out), "reviewer": name, "preset": preset,
		"status": status, "user_quote": rwQuote, "recorded_at": rwRecordedAt})
	if err != nil {
		w.t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(out, "waivers.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		w.t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		w.t.Fatal(err)
	}
}

type rwGate struct {
	code int
	rec  GateRecord
	raw  map[string]any
	data []byte
}

// gate writes the gate of the rounds to the series of the first one and
// reads it back. A reason outside the families the gate gives for its
// reviewers (and the extra reasons a test expects) means the gate could not
// read a round or the repository: a broken setup, never an observation.
func (w *rwWorld) gate(expect string, rounds []string, extra ...string) rwGate {
	w.t.Helper()
	file := filepath.Join(filepath.Dir(rounds[0]), "gate.json")
	args := append(append([]string{"gate"}, rounds...), "--expect-head", expect, "--out", file, "--repo", w.repo)
	code, msg := w.spar(args...)
	g := rwGate{code: code}
	data, err := os.ReadFile(file)
	if err != nil {
		w.t.Fatalf("spar gate wrote no file: %d %s", code, msg)
	}
	g.data = data
	if err := json.Unmarshal(data, &g.rec); err != nil {
		w.t.Fatalf("gate.json: %v", err)
	}
	if err := json.Unmarshal(data, &g.raw); err != nil {
		w.t.Fatalf("gate.json: %v", err)
	}
	for _, reason := range g.rec.Reasons {
		if !rwReviewerReason(reason, rounds) && !rwHas(extra, reason) {
			w.t.Fatalf("the gate did not read the rounds cleanly (exit %d): %v", code, g.rec.Reasons)
		}
	}
	want := map[string]int{StateComplete: ExitOK, StateDegraded: ExitDegraded, StateBlocked: ExitBlocked}
	if exit, ok := want[g.rec.State]; !ok || exit != code {
		w.t.Fatalf("spar gate: exit %d for state %q: %s", code, g.rec.State, msg)
	}
	return g
}

// rwReviewerReason reports a gate reason about a given round's required
// reviewers: one that did not run or complete, a round without one, or a
// round where none completed.
func rwReviewerReason(reason string, rounds []string) bool {
	for _, p := range rounds {
		rest, ok := strings.CutPrefix(reason, p+": ")
		if !ok {
			continue
		}
		if rest == "the round has no required reviewer" || rest == "no required reviewer completed this round" {
			return true
		}
		if name, ok := strings.CutPrefix(rest, "required reviewer "); ok && !strings.Contains(name, ": ") &&
			(strings.HasSuffix(name, " did not run in this round") || strings.Contains(name, " is ")) {
			return true
		}
	}
	return false
}

// waived lists the waived_reviewers the gate file gives round i, and
// whether the field is there as a list of objects.
func (g rwGate) waived(i int) ([]map[string]any, bool) {
	rounds, _ := g.raw["rounds"].([]any)
	if i >= len(rounds) {
		return nil, false
	}
	round, _ := rounds[i].(map[string]any)
	value, present := round["waived_reviewers"]
	if !present {
		return nil, true
	}
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	var out []map[string]any
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, entry)
	}
	return out, true
}

func rwHas(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func rwSame(a, b []string) bool {
	return strings.Join(a, "\n") == strings.Join(b, "\n") && len(a) == len(b)
}

func rwUnavailable(out, name string) string {
	return out + ": required reviewer " + name + " is unavailable"
}

func rwViolation(t *testing.T, id, format string, args ...any) {
	t.Helper()
	t.Errorf("jaekit:violation=%s: "+format, append([]any{id}, args...)...)
}

// expectReasons stops the test when a gate before the waiver does not give
// the reasons the scenario sets up: its reviewers did not finish as the
// test meant, so nothing about the waiver would be observed.
func (w *rwWorld) expectReasons(g rwGate, want ...string) {
	w.t.Helper()
	if !rwSame(g.rec.Reasons, want) {
		w.t.Fatalf("the scenario does not give the reasons it sets up: %v, want %v", g.rec.Reasons, want)
	}
}

// AC-1: the executor records the user's decision for one required reviewer
// of one round, and the record keeps the round, the reviewer, the user's
// words and the time. A second decision in the same round keeps the first,
// and the gate reads both.
func TestSparWaiverAC1_RecordsTheUserDecision(t *testing.T) {
	const id = "waiver-not-recorded"
	w := newRWWorld(t, false)
	w.use(rwNative("host"), w.missingAgy(), rwNative("second"))
	unfinished := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(unfinished, "host")
	notRun := w.run(filepath.Join(w.newSeries(), "01-candidate"), "--reviewers", "host")
	w.record(notRun, "host")
	for _, c := range []struct {
		out      string
		statuses map[string]string
	}{
		{unfinished, map[string]string{"agy": "unavailable", "second": "pending"}},
		{notRun, map[string]string{"agy": "not_run", "second": "not_run"}},
	} {
		order := []string{"agy", "second"}
		presets := map[string]string{"agy": "agy", "second": "native"}
		recorded := true
		for _, name := range order {
			code, msg := w.spar("waive", c.out, name, "--user-quote", rwQuote)
			if code == ExitInternal {
				t.Fatalf("%s: spar waive %s stopped on an internal error: %s", filepath.Base(filepath.Dir(c.out)), name, msg)
			}
			if code != ExitOK {
				rwViolation(t, id, "%s %s: spar waive with the user's words exited %d: %s", c.statuses[name], name, code, msg)
				recorded = false
			}
		}
		if !recorded {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.out, "waivers.jsonl"))
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if err != nil || len(lines) != len(order) {
			rwViolation(t, id, "%v: the round holds %d waiver lines, want one per decision: %v %q", order, len(lines), err, data)
			continue
		}
		for i, name := range order {
			var entry map[string]any
			if json.Unmarshal([]byte(lines[i]), &entry) != nil {
				rwViolation(t, id, "%s: waiver line %d is not a JSON object: %q", name, i+1, lines[i])
				continue
			}
			want := map[string]string{"round": filepath.Base(c.out), "reviewer": name, "preset": presets[name],
				"status": c.statuses[name], "user_quote": rwQuote}
			for key, value := range want {
				if entry[key] != value {
					rwViolation(t, id, "%s: %s is %v, want %q", name, key, entry[key], value)
				}
			}
			at, _ := entry["recorded_at"].(string)
			if _, err := time.Parse(time.RFC3339, at); err != nil {
				rwViolation(t, id, "%s: recorded_at %v is not a time: %v", name, entry["recorded_at"], err)
			}
		}
		g := w.gate(w.head, []string{c.out})
		listed, ok := g.waived(0)
		names := map[string]bool{}
		for _, e := range listed {
			if e["user_quote"] == rwQuote {
				names[fmt.Sprint(e["name"])] = true
			}
		}
		if g.rec.State != StateComplete || !ok || len(listed) != len(order) || len(names) != len(order) {
			rwViolation(t, id, "%v: the gate does not read the recorded waivers: %s %v, waived %v", order,
				g.rec.State, g.rec.Reasons, listed)
		}
	}
}

// AC-2: a reviewer with the user's decision in a round is not a degraded
// reason of that round, whether it did not run or did not complete, and the
// gate is complete when nothing else is open.
func TestSparWaiverAC2_WaivedReviewerDoesNotDegradeTheGate(t *testing.T) {
	const id = "waived-reviewer-degrades-gate"
	w := newRWWorld(t, false)
	check := func(label, expect string, rounds []string) {
		t.Helper()
		if g := w.gate(expect, rounds); g.code != ExitOK || g.rec.State != StateComplete || len(g.rec.Reasons) != 0 {
			rwViolation(t, id, "%s: the gate is %s (exit %d) with the waiver recorded: %v", label, g.rec.State, g.code, g.rec.Reasons)
		}
	}

	w.use(rwNative("host"), w.missingAgy())
	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	w.expectReasons(w.gate(w.head, []string{out}), rwUnavailable(out, "agy"))
	w.waive(out, "agy", "agy", "unavailable")
	check("unavailable agy", w.head, []string{out})

	out = w.run(filepath.Join(w.newSeries(), "01-candidate"), "--reviewers", "host")
	w.record(out, "host")
	w.expectReasons(w.gate(w.head, []string{out}), out+": required reviewer agy (agy) did not run in this round")
	w.waive(out, "agy", "agy", "not_run")
	check("agy that did not run", w.head, []string{out})

	w.use(rwNative("host"), w.brokenAgy())
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	w.expectReasons(w.gate(w.head, []string{out}), out+": required reviewer agy is failed")
	w.waive(out, "agy", "agy", "failed")
	check("failed agy", w.head, []string{out})

	w.use(rwNative("host"), rwNative("second"))
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	w.expectReasons(w.gate(w.head, []string{out}), out+": required reviewer second is pending")
	w.waive(out, "second", "native", "pending")
	check("pending native reviewer", w.head, []string{out})

	// A rerun of the same packet is its own round: each round holds its own
	// waiver, and the rerun's other reviewers completed the packet before.
	w.use(rwNative("host"), w.missingAgy())
	series := w.newSeries()
	first := w.run(filepath.Join(series, "01-candidate"))
	w.record(first, "host")
	rerun := w.run(filepath.Join(series, "02-rerun"), "--reviewers", "agy")
	w.expectReasons(w.gate(w.head, []string{first, rerun}), rwUnavailable(first, "agy"), rwUnavailable(rerun, "agy"))
	w.waive(first, "agy", "agy", "unavailable")
	w.waive(rerun, "agy", "agy", "unavailable")
	check("a rerun with its own waiver", w.head, []string{first, rerun})
}

// AC-3: the gate file lists, per round, the reviewers the user's decision
// covers apart from the completed ones, with their status and the user's
// words. A reviewer that completed the packet after all is not listed.
func TestSparWaiverAC3_GateListsWaivedReviewersApart(t *testing.T) {
	const id = "waived-reviewer-not-shown"
	w := newRWWorld(t, false)
	w.use(rwNative("host"), w.missingAgy())
	series := w.newSeries()
	unavailable := w.run(filepath.Join(series, "01-candidate"))
	w.record(unavailable, "host")
	notRun := w.run(filepath.Join(series, "02-candidate"), "--reviewers", "host")
	w.record(notRun, "host")
	w.waive(unavailable, "agy", "agy", "unavailable")
	w.waive(notRun, "agy", "agy", "not_run")
	r, err := loadRound(unavailable)
	if err != nil || r.Reviewers["agy"] == nil || r.Reviewers["agy"].Reason == "" {
		t.Fatalf("the round does not record why agy is unavailable: %v", err)
	}
	g := w.gate(w.head, []string{unavailable, notRun})
	for i, want := range []struct{ status, reason string }{{"unavailable", r.Reviewers["agy"].Reason}, {"not_run", ""}} {
		listed, ok := g.waived(i)
		if !ok || len(listed) != 1 {
			rwViolation(t, id, "round %d: waived reviewers %v, want agy alone", i+1, listed)
			continue
		}
		e := listed[0]
		reason, _ := e["reason"].(string)
		if e["name"] != "agy" || e["preset"] != "agy" || e["status"] != want.status || reason != want.reason ||
			e["user_quote"] != rwQuote || e["recorded_at"] != rwRecordedAt {
			rwViolation(t, id, "round %d: waived reviewer %v, want agy (agy) %s with the user's words", i+1, e, want.status)
		}
		if gr := g.rec.Rounds[i]; gr.RequiredReviewers != 2 || gr.CompletedRequired != 1 {
			rwViolation(t, id, "round %d: %d of %d required reviewers counted as completed, want host alone", i+1,
				gr.CompletedRequired, gr.RequiredReviewers)
		}
	}

	// A waived reviewer that then completes the same packet in a rerun is a
	// completed reviewer, not a waived one.
	w.use(rwNative("host"), rwNative("second"))
	series = w.newSeries()
	first := w.run(filepath.Join(series, "01-candidate"))
	w.record(first, "host")
	w.waive(first, "second", "native", "pending")
	rerun := w.run(filepath.Join(series, "02-rerun"), "--reviewers", "second")
	w.record(rerun, "second")
	g = w.gate(w.head, []string{first, rerun})
	if listed, ok := g.waived(0); !ok || len(listed) != 0 || g.rec.Rounds[0].CompletedRequired != 2 {
		rwViolation(t, id, "a reviewer that completed the packet in a rerun: waived %v, %d completed", listed,
			g.rec.Rounds[0].CompletedRequired)
	}
}

// AC-4: spar waive records nothing and says why when the user's words are
// missing, the reviewer completed the round or a rerun of its packet, the
// reviewer is not a required reviewer of the round, or the round cannot be
// read.
func TestSparWaiverAC4_RefusalsRecordNothing(t *testing.T) {
	const id = "waiver-not-refused"
	w := newRWWorld(t, false)
	refuse := func(label, because string, round string, args ...string) {
		t.Helper()
		file := filepath.Join(round, "waivers.jsonl")
		before, beforeErr := os.ReadFile(file)
		code, msg := w.spar(append([]string{"waive", round}, args...)...)
		after, afterErr := os.ReadFile(file)
		if code == ExitInternal {
			t.Fatalf("%s: spar waive stopped on an internal error: %s", label, msg)
		}
		if code != ExitRefused || !strings.Contains(msg, because) {
			rwViolation(t, id, "%s: exit %d, want %d with a reason naming %q: %s", label, code, ExitRefused, because, msg)
		}
		if (beforeErr == nil) != (afterErr == nil) || !bytes.Equal(before, after) {
			rwViolation(t, id, "%s: the refused waiver changed waivers.jsonl", label)
		}
	}
	w.use(rwNative("host"), w.missingAgy(), w.missingCommand("extra"))
	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	refuse("no user words", "--user-quote", out, "agy")
	refuse("empty user words", "--user-quote", out, "agy", "--user-quote", "")
	refuse("blank user words", "--user-quote", out, "agy", "--user-quote", " \t ")
	refuse("a reviewer that completed", "completed", out, "host", "--user-quote", rwQuote)
	refuse("a command reviewer", "not a required reviewer", out, "extra", "--user-quote", rwQuote)
	refuse("an unknown reviewer", "not a required reviewer", out, "nobody", "--user-quote", rwQuote)
	// A rerun of the packet whose reviewers completed it in the round it
	// repeats: host is a required reviewer of the rerun through the
	// configuration and completed the packet before.
	rerunOfOut := w.run(filepath.Join(filepath.Dir(out), "02-rerun"), "--reviewers", "agy")
	refuse("a reviewer that completed the packet before its rerun", "completed", rerunOfOut, "host", "--user-quote", rwQuote)

	w.use(rwNative("host"), rwNative("second"))
	series := w.newSeries()
	first := w.run(filepath.Join(series, "01-candidate"))
	w.record(first, "host")
	rerun := w.run(filepath.Join(series, "02-rerun"), "--reviewers", "second")
	w.record(rerun, "second")
	if r, err := loadRound(rerun); err != nil || r.Reviewers["second"] == nil || r.Reviewers["second"].Status != "completed" {
		t.Fatalf("the rerun did not complete second: %v", err)
	}
	refuse("a reviewer that completed a rerun of the packet", "completed", first, "second", "--user-quote", rwQuote)

	junk := w.newSeries()
	missing := filepath.Join(junk, "01-missing")
	empty := filepath.Join(junk, "02-empty")
	corrupt := filepath.Join(junk, "03-corrupt")
	other := filepath.Join(junk, "04-other")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	w.write(filepath.Join(corrupt, "round.json"), "{")
	w.write(filepath.Join(other, "round.json"), "{\"schema\": \"other/v1\"}\n")
	for label, round := range map[string]string{"a missing round": missing, "a folder without round.json": empty,
		"a corrupt round.json": corrupt, "another record": other} {
		refuse(label, "cannot be read as a round", round, "agy", "--user-quote", rwQuote)
	}
}

// AC-5: a round where no required reviewer completed stays degraded with
// the user's decisions recorded, and the reason says that none completed.
func TestSparWaiverAC5_RoundWithoutACompletedReviewerStaysDegraded(t *testing.T) {
	const id = "no-completed-reviewer-not-degraded"
	w := newRWWorld(t, false)
	w.use(rwNative("host"), w.missingAgy())
	all := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.waive(all, "host", "native", "pending")
	w.waive(all, "agy", "agy", "unavailable")
	some := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.waive(some, "agy", "agy", "unavailable")
	notRun := w.run(filepath.Join(w.newSeries(), "01-candidate"), "--reviewers", "host")
	w.waive(notRun, "host", "native", "pending")
	w.waive(notRun, "agy", "agy", "not_run")
	for label, out := range map[string]string{"every required reviewer waived": all, "one waived, one pending": some,
		"waived and not run": notRun} {
		g := w.gate(w.head, []string{out})
		want := out + ": no required reviewer completed this round"
		if g.code != ExitDegraded || g.rec.State != StateDegraded || !rwHas(g.rec.Reasons, want) {
			rwViolation(t, id, "%s: the gate is %s (exit %d), reasons %v, want %q", label, g.rec.State, g.code, g.rec.Reasons, want)
		}
	}
}

// AC-6 (kept): a decision covers its round only. Another round where the
// same reviewer did not finish, a rerun of the same packet or a later
// fixes round, is a degraded reason as it is without any decision.
func TestSparWaiverAC6_WaiverCoversItsRoundOnly(t *testing.T) {
	w := newRWWorld(t, true)
	w.use(rwNative("host"), w.missingAgy())
	series := w.newSeries()
	first := w.run(filepath.Join(series, "01-candidate"))
	w.record(first, "host")
	rerun := w.run(filepath.Join(series, "02-rerun"), "--reviewers", "agy")
	w.waive(first, "agy", "agy", "unavailable")
	g := w.gate(w.head, []string{first, rerun})
	if listed, ok := g.waived(1); g.rec.State != StateDegraded || !rwHas(g.rec.Reasons, rwUnavailable(rerun, "agy")) || !ok || len(listed) != 0 {
		t.Errorf("a rerun without its own waiver: %s, reasons %v, waived %v", g.rec.State, g.rec.Reasons, listed)
	}

	series = w.newSeries()
	candidate := w.run(filepath.Join(series, "01-candidate"))
	w.record(candidate, "host")
	fixes := w.run(filepath.Join(series, "02-fixes"), "--stage", "fixes", "--base", w.head, "--head", w.fix)
	w.record(fixes, "host")
	w.waive(candidate, "agy", "agy", "unavailable")
	g = w.gate(w.fix, []string{candidate, fixes})
	if listed, ok := g.waived(1); g.rec.State != StateDegraded || !rwHas(g.rec.Reasons, rwUnavailable(fixes, "agy")) || !ok || len(listed) != 0 {
		t.Errorf("a fixes round without its own waiver: %s, reasons %v, waived %v", g.rec.State, g.rec.Reasons, listed)
	}
}

// The fields a round record holds before this change (contracts/review-
// round.md, section 5): a round written now holds no other, so the gate
// reads it as it reads an earlier round.
var (
	rwRoundRecordKeys = []string{"base", "base_tree", "config", "counted", "criteria", "criteria_digest", "diff_sha256",
		"dry_run", "finished_at", "head", "head_tree", "label", "links", "packet_sha256", "prior", "questions",
		"repo", "requested_reviewers", "required_reviewers", "rerun_of", "reviewers", "rules", "schema", "sequence",
		"series", "spar_version", "stage", "started_at", "subjects"}
	rwConfigKeys = []string{"angles", "caps", "check_paths", "configured_required", "digest", "files", "override",
		"reviewers", "rules", "source"}
	rwConfigReviewerKeys = []string{"argv", "binary", "effort", "fallback_model", "isolation", "model", "name", "preset",
		"required", "timeout_seconds"}
	rwReviewerStateKeys = []string{"agent", "angles_checked", "angles_unchecked", "attempts", "effort",
		"elapsed_seconds", "fallback_model", "fallback_used", "findings", "isolation", "limits", "name",
		"observed_model", "preset", "reason", "recorded_from", "requested_model", "required", "resets", "status",
		"timeout_seconds", "verdict", "warnings", "workspace_digest"}
)

// rwOutside lists the keys of m that are not among known.
func rwOutside(m map[string]any, known []string) []string {
	var out []string
	for _, k := range rwKeys(m) {
		if !rwHas(known, k) {
			out = append(out, k)
		}
	}
	return out
}

// rwPreChangeForm reports the fields of a round record that a round
// recorded before this change cannot hold.
func rwPreChangeForm(t *testing.T, round string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(round, "round.json"))
	var r map[string]any
	if err != nil || json.Unmarshal(data, &r) != nil {
		t.Fatalf("%s/round.json cannot be read: %v", round, err)
	}
	extra := rwOutside(r, rwRoundRecordKeys)
	config, _ := r["config"].(map[string]any)
	for _, k := range rwOutside(config, rwConfigKeys) {
		extra = append(extra, "config."+k)
	}
	reviewers, _ := config["reviewers"].([]any)
	for _, item := range reviewers {
		entry, _ := item.(map[string]any)
		for _, k := range rwOutside(entry, rwConfigReviewerKeys) {
			extra = append(extra, "config.reviewers[]."+k)
		}
	}
	states, _ := r["reviewers"].(map[string]any)
	for name, item := range states {
		entry, _ := item.(map[string]any)
		for _, k := range rwOutside(entry, rwReviewerStateKeys) {
			extra = append(extra, "reviewers."+name+"."+k)
		}
	}
	return extra
}

// rwRename turns a recorded round's reviewer into one of another name, as
// a round recorded before this change could hold it: the round record's
// lists, the reviewer's state and configuration entry, and its folder.
func rwRename(t *testing.T, round, from, to string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(round, "round.json"))
	var r map[string]any
	if err != nil || json.Unmarshal(data, &r) != nil {
		t.Fatalf("%s/round.json cannot be read: %v", round, err)
	}
	rename := func(list any) {
		items, _ := list.([]any)
		for i, item := range items {
			if item == from {
				items[i] = to
			}
			if entry, ok := item.(map[string]any); ok && entry["name"] == from {
				entry["name"] = to
			}
		}
	}
	rename(r["requested_reviewers"])
	rename(r["required_reviewers"])
	config, _ := r["config"].(map[string]any)
	rename(config["reviewers"])
	rename(config["configured_required"])
	states, _ := r["reviewers"].(map[string]any)
	state, _ := states[from].(map[string]any)
	if state == nil {
		t.Fatalf("%s has no reviewer %s", round, from)
	}
	state["name"] = to
	delete(states, from)
	states[to] = state
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(round, "round.json"), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(round, from), filepath.Join(round, to)); err != nil {
		t.Fatal(err)
	}
}

// The fields of a gate file and of each round in it, as they are without
// any waiver (contracts/review-gate.md, section 3).
var (
	rwGateKeys = []string{"auto_deferred", "expect_head", "growth", "last_reviewed_head", "open", "open_findings",
		"previous_gate_digest", "reasons", "rounds", "schema", "spar_version", "state", "written_at"}
	rwRoundKeys = []string{"base", "completed_required", "config_digest", "counted", "criteria_digest", "dry_run", "head",
		"packet_digest", "path", "requested_reviewers", "required_reviewers", "sequence", "stage"}
)

func rwKeys(m map[string]any) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AC-7 (kept): rounds without a waiver, as every round recorded before
// this change is, give the gate state, reasons and fields they give without
// waivers, each time the gate is made again.
func TestSparWaiverAC7_RoundsWithoutWaiversKeepTheirGate(t *testing.T) {
	w := newRWWorld(t, true)
	type scenario struct {
		label  string
		rounds []string
		state  string
		code   int
		want   []string
	}
	var scenarios []scenario

	w.use(rwNative("host"), w.missingAgy())
	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	scenarios = append(scenarios, scenario{"unavailable agy", []string{out}, StateDegraded, ExitDegraded,
		[]string{rwUnavailable(out, "agy")}})
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"), "--reviewers", "host")
	w.record(out, "host")
	scenarios = append(scenarios, scenario{"agy that did not run", []string{out}, StateDegraded, ExitDegraded,
		[]string{out + ": required reviewer agy (agy) did not run in this round"}})
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	scenarios = append(scenarios, scenario{"no reviewer completed", []string{out}, StateDegraded, ExitDegraded,
		[]string{out + ": required reviewer host is pending", rwUnavailable(out, "agy")}})

	w.use(rwNative("host"))
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	scenarios = append(scenarios, scenario{"every reviewer completed", []string{out}, StateComplete, ExitOK, nil})

	w.use(w.missingCommand("extra"))
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	scenarios = append(scenarios, scenario{"no required reviewer", []string{out}, StateDegraded, ExitDegraded,
		[]string{out + ": the round has no required reviewer"}})

	w.use(rwNative("host"), rwNative("second"))
	series := w.newSeries()
	first := w.run(filepath.Join(series, "01-candidate"))
	w.record(first, "host")
	rerun := w.run(filepath.Join(series, "02-rerun"), "--reviewers", "second")
	w.record(rerun, "second")
	scenarios = append(scenarios, scenario{"completed in a rerun", []string{first, rerun}, StateComplete, ExitOK, nil})

	// Before this change a reviewer could be named waivers.jsonl; its
	// folder in an earlier round is the reviewer's, not a waiver file.
	w.use(rwNative("host"), rwNative("legacy"))
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	w.record(out, "legacy")
	rwRename(t, out, "legacy", "waivers.jsonl")
	scenarios = append(scenarios, scenario{"a reviewer named waivers.jsonl", []string{out}, StateComplete, ExitOK, nil})

	for _, s := range scenarios {
		records := map[string][]byte{}
		for _, round := range s.rounds {
			if info, err := os.Stat(filepath.Join(round, "waivers.jsonl")); err == nil && !info.IsDir() {
				t.Fatalf("%s: spar wrote waivers.jsonl without a waiver, so the round is not in the form of an earlier one", s.label)
			}
			if extra := rwPreChangeForm(t, round); len(extra) > 0 {
				t.Errorf("%s: the round record holds fields an earlier round cannot: %v", s.label, extra)
			}
			for _, name := range []string{"round.json", "findings.json", "ledger.jsonl"} {
				data, err := os.ReadFile(filepath.Join(round, name))
				if err != nil {
					t.Fatal(err)
				}
				records[round+"/"+name] = data
			}
		}
		var previous []byte
		for i := 0; i < 2; i++ {
			g := w.gate(w.head, s.rounds)
			if g.rec.State != s.state || g.code != s.code || !rwSame(g.rec.Reasons, s.want) {
				t.Errorf("%s, gate %d: %s (exit %d), reasons %v, want %s %v", s.label, i+1, g.rec.State, g.code, g.rec.Reasons, s.state, s.want)
			}
			if keys := rwKeys(g.raw); !rwSame(keys, rwGateKeys) {
				t.Errorf("%s, gate %d: fields %v, want %v", s.label, i+1, keys, rwGateKeys)
			}
			rounds, _ := g.raw["rounds"].([]any)
			for j, item := range rounds {
				round, _ := item.(map[string]any)
				if keys := rwKeys(round); !rwSame(keys, rwRoundKeys) {
					t.Errorf("%s, gate %d, round %d: fields %v, want %v", s.label, i+1, j+1, keys, rwRoundKeys)
				}
			}
			if i == 1 && (g.rec.PreviousGateDigest == nil || *g.rec.PreviousGateDigest != fmt.Sprintf("%x", sha256.Sum256(previous))) {
				t.Errorf("%s: the second gate does not name the first one's digest", s.label)
			}
			previous = g.data
		}
		for path, data := range records {
			if now, err := os.ReadFile(path); err != nil || !bytes.Equal(now, data) {
				t.Errorf("%s: making the gate changed %s", s.label, path)
			}
		}
	}
}

// AC-8 (kept): the blocked reasons (open findings, a broken chain, a cap
// exceeded, an unreviewed head, a round left out of the list) are the same
// with and without the user's decisions.
func TestSparWaiverAC8_BlockedReasonsIgnoreWaivers(t *testing.T) {
	w := newRWWorld(t, true)
	w.use(rwNative("host"), w.missingAgy())
	compare := func(label, expect string, rounds []string, blocked []string) {
		t.Helper()
		before := w.gate(expect, rounds, blocked...)
		for _, round := range rounds {
			w.waive(round, "agy", "agy", "unavailable")
		}
		after := w.gate(expect, rounds, blocked...)
		keep := func(g rwGate) []string {
			var out []string
			for _, reason := range g.rec.Reasons {
				if !rwReviewerReason(reason, rounds) {
					out = append(out, reason)
				}
			}
			return out
		}
		if !rwSame(keep(before), blocked) {
			t.Fatalf("%s: the scenario does not give the blocked reasons it sets up: %v, want %v", label, before.rec.Reasons, blocked)
		}
		if after.rec.State != StateBlocked || after.code != ExitBlocked || !rwSame(keep(after), blocked) ||
			!rwSame(after.rec.OpenFindings, before.rec.OpenFindings) || after.rec.Open["defect"] != before.rec.Open["defect"] ||
			after.rec.Open["requirement-question"] != before.rec.Open["requirement-question"] {
			t.Errorf("%s: with waivers the gate is %s (exit %d), reasons %v, open %v %v; without them %v, open %v %v", label,
				after.rec.State, after.code, after.rec.Reasons, after.rec.Open, after.rec.OpenFindings,
				before.rec.Reasons, before.rec.Open, before.rec.OpenFindings)
		}
	}
	finding := func(id, class string) map[string]any {
		return map[string]any{"id": id, "proposed_class": class, "angle": "design", "location": "head/app/main.py:1",
			"claim": "synthetic claim " + id, "failure_scenario": "a synthetic input gives a wrong result",
			"evidence": "synthetic evidence", "link_kind": "criterion", "link": "AC-1", "introduced_by_change": "no",
			"regression": "no", "check_impact": "none", "confidence": "high"}
	}

	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host", finding("F1", "defect"), finding("F2", "requirement-question"))
	compare("open findings", w.head, []string{out}, []string{"2 open defect or requirement-question finding(s)"})

	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	compare("an unreviewed head", w.fix, []string{out},
		[]string{fmt.Sprintf("the expected head %s was not reviewed; the last reviewed head is %s", shortID(w.fix), shortID(w.head))})

	series := w.newSeries()
	listed := w.run(filepath.Join(series, "01-candidate"))
	w.record(listed, "host")
	w.record(w.run(filepath.Join(series, "02-candidate"), "--reviewers", "host"), "host")
	compare("a round left out", w.head, []string{listed},
		[]string{series + ": round 02-candidate of the series is not in the gate list"})

	codeRounds := func() (string, string) {
		series := w.newSeries()
		candidate := w.run(filepath.Join(series, "01-candidate"))
		w.record(candidate, "host")
		fixes := w.run(filepath.Join(series, "02-fixes"), "--stage", "fixes", "--base", w.head, "--head", w.fix)
		w.record(fixes, "host")
		return candidate, fixes
	}
	candidate, fixes := codeRounds()
	compare("a fixes round before its candidate", w.fix, []string{fixes, candidate},
		[]string{fixes + ": a fixes round needs an earlier code round in the gate list",
			fmt.Sprintf("the expected head %s was not reviewed; the last reviewed head is %s", shortID(w.fix), shortID(w.head))})
	candidate, fixes = codeRounds()
	other := w.run(filepath.Join(w.newSeries(), "01-candidate"), "--head", w.fix)
	w.record(other, "host")
	compare("a broken fixes chain", w.fix, []string{candidate, other, fixes},
		[]string{fmt.Sprintf("%s: the fixes chain is broken: base %s is not the previous head %s", fixes, shortID(w.head), shortID(w.fix))})

	// A cap lowered after two code rounds: the rerun that applies it is
	// admitted, and the gate counts the series against it.
	series = w.newSeries()
	first := w.run(filepath.Join(series, "01-candidate"))
	w.record(first, "host")
	w.record(w.run(filepath.Join(series, "02-candidate"), "--reviewers", "host"), "host")
	w.write(filepath.Join(w.repo, ".jaekit", "spar.toml"), "[caps]\ncode = 1\n")
	rerun := w.run(filepath.Join(series, "03-rerun"), "--reviewers", "agy")
	second := filepath.Join(series, "02-candidate")
	compare("a cap exceeded", w.head, []string{first, second, rerun},
		[]string{fmt.Sprintf("series %s ran 2 code rounds; the cap with recorded extensions is 1", series)})
}

// rwRead reads a repository file; a file that cannot be read is a broken
// setup, not an observation.
func rwRead(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("%s cannot be read: %v", rel, err)
	}
	return string(data)
}

// rwStates reports each sentence a document does not hold.
func rwStates(t *testing.T, id, rel string, sentences ...string) {
	t.Helper()
	text := rwRead(t, rel)
	for _, s := range sentences {
		if !strings.Contains(text, s) {
			rwViolation(t, id, "%s does not say %q", rel, s)
		}
	}
}

// rwDropped reports each statement a document still holds although the
// waiver makes it false.
func rwDropped(t *testing.T, id, rel string, statements ...string) {
	t.Helper()
	text := rwRead(t, rel)
	for _, s := range statements {
		if strings.Contains(text, s) {
			rwViolation(t, id, "%s still says %q", rel, s)
		}
	}
}

// AC-9: the round contract states how a waiver is recorded, that it needs
// the user's words and what it does to the gate.
func TestSparWaiverAC9_RoundContractStatesTheWaiver(t *testing.T) {
	rwStates(t, "round-contract-waiver-missing", "contracts/review-round.md",
		"## 10. 필수 리뷰어를 빼는 결정",
		"`spar waive <라운드> <리뷰어> --user-quote \"…\"`는 그 라운드 폴더의 `waivers.jsonl`에 `{\"round\", \"reviewer\", \"preset\", \"status\", \"reason\", \"user_quote\", \"recorded_at\"}` 한 줄을 더한다.",
		"사용자의 말이 없으면 기록하지 않는다.",
		"결정이 기록된 리뷰어는 그 라운드의 `degraded` 사유가 되지 않고 gate 파일에 따로 보인다. 완료한 필수 리뷰어가 하나도 없는 라운드는 결정이 있어도 `degraded`다",
		"결정은 그 라운드에만 적용된다.",
		"| `waivers.jsonl` |")
}

// AC-9: the gate contract states what a waiver does to the gate.
func TestSparWaiverAC9_GateContractStatesTheEffect(t *testing.T) {
	rwStates(t, "gate-contract-waiver-missing", "contracts/review-gate.md",
		"그 라운드의 `waivers.jsonl`에 사용자의 결정(`spar waive`, [검토 라운드](review-round.md) 10절)이 있는 리뷰어는 이 사유가 되지 않고 그 라운드의 `waived_reviewers`에 보인다.",
		"결정이 적용된 리뷰어가 있는데 완료한 필수 리뷰어가 하나도 없는 라운드: `<라운드>: no required reviewer completed this round`.",
		"결정은 사용자의 말을 인용해서만 기록되고, 그 라운드 폴더의 `waivers.jsonl`에서만 읽어 그 라운드에만 적용한다.",
		"| `rounds[].waived_reviewers` |")
}

// AC-9: the Spar skill states the same rules, and its gate states and
// limits no longer say otherwise.
func TestSparWaiverAC9_SkillStatesTheWaiver(t *testing.T) {
	const id = "skill-waiver-missing"
	const skill = "plugins/spar/skills/spar/SKILL.md"
	rwStates(t, id, skill,
		"\n- When a required reviewer did not complete a round and the user says to go on without it, record their words for that round: `spar waive <round> <reviewer> --user-quote \"<their words>\"`. Never record a waiver without the user's words. A waiver covers its round only; record one for each round the reviewer did not complete.\n",
		"\n- `complete` only when every required reviewer completed every round or holds the user's waiver for that round, at least one required reviewer completed each round's packet, the code rounds form a chain up to the expected head, and no `defect` or `requirement-question` is open.\n",
		"\n- `degraded` when a required reviewer did not complete a round and holds no waiver for it, a round had no required reviewer, or no required reviewer completed a round's packet. Tell the user which reviewer and why; do not present the review as complete.\n",
		"\n- A waived reviewer is not a `degraded` reason; the gate file lists it under its round's `waived_reviewers` with its status and the user's words. A round where no required reviewer completed stays `degraded` even with waivers. Report the waivers with the gate.\n",
		"A `complete` gate means the required reviewers the user did not waive found nothing open;",
		"\n- Do not edit round records by hand. Use `spar triage`, `spar record`, `spar extend` and `spar waive`.\n")
	rwDropped(t, id, skill,
		"\n- `complete` only when every required reviewer completed every round, the code rounds",
		"\n- `degraded` when a required reviewer did not complete, or a round had no required reviewer.",
		"A `complete` gate means the configured reviewers found nothing open",
		"Use `spar triage`, `spar record` and `spar extend`.")
}

// AC-9: both guides state the same rules, and their gate states no longer
// say otherwise.
func TestSparWaiverAC9_GuidesStateTheWaiver(t *testing.T) {
	const id = "guide-waiver-missing"
	rwStates(t, id, "guides/SPAR.md",
		"`complete`(종료 0), 필수 리뷰어가 끝내지 못했고 그 라운드에 사용자의 결정이 없으면 `degraded`(3)",
		"필수 리뷰어가 사용량 한도 같은 이유로 라운드를 끝내지 못했고 사용자가 그 리뷰어 없이 진행하라고 하면, 에이전트가 `spar waive <라운드> <리뷰어> --user-quote \"…\"`로 그 말을 그 라운드에 기록합니다. 사용자의 말 없이는 기록하지 않습니다. 결정이 기록된 리뷰어는 그 라운드의 `degraded` 사유가 되지 않고, gate 파일의 그 라운드 `waived_reviewers`에 상태와 사용자의 말이 함께 보입니다. 결정은 그 라운드에만 적용되고, 완료한 필수 리뷰어가 하나도 없는 라운드는 결정이 있어도 `degraded`입니다.")
	rwDropped(t, id, "guides/SPAR.md", "필수 리뷰어가 끝내지 못했으면 `degraded`(3)")
	rwStates(t, id, "guides/SPAR.en.md",
		"`degraded` (3) when a required reviewer did not finish and you made no decision for that round,",
		"When a required reviewer did not finish a round, for example because of a usage limit, and you say to go on without it, the agent records your words for that round with `spar waive <round> <reviewer> --user-quote \"…\"`. It never records one without your words. A waived reviewer is not a `degraded` reason for that round, and the gate file shows it under that round's `waived_reviewers` with its status and your words. A waiver covers only its round, and a round where no required reviewer completed stays `degraded` even with waivers.")
	rwDropped(t, id, "guides/SPAR.en.md", "`degraded` (3) when a required reviewer did not finish, or")
}
