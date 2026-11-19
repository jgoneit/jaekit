package spar

// A later candidate run of a recorded candidate round's stage, base and head
// receives exactly the earlier rounds that round's prior names, so that it
// reruns the original packet: none when the original was recorded before
// candidate rounds received earlier findings, whatever code rounds the
// series recorded before or after it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// rrWorld adds a fourth commit, c3, to a repeat-findings world, so that a
// fixes round can follow a candidate round from c0 to c2, and a project
// configuration that allows the three counted code rounds its series hold
// before an extension.
type rrWorld struct {
	*rptWorld
	late string // a reviewer that never completes
}

func newRRWorld(t *testing.T) *rrWorld {
	t.Helper()
	w := &rrWorld{rptWorld: newRptWorld(t, true)}
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('four')\n")
	w.git("add", "-A")
	w.git("commit", "-qm", "c3")
	w.commits = append(w.commits, w.git("rev-parse", "HEAD"))
	w.trees = append(w.trees, w.git("rev-parse", "HEAD^{tree}"))
	w.late = pdCommand("late", w.script("late", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n"))
	w.write(filepath.Join(w.repo, ".jaekit", "spar.toml"), "[caps]\ncode = 3\n")
	return w
}

// code runs a code round from base to head into series/name whose command
// reviewer reports one finding with claim, beside the reviewer late. A
// fixes round is written as a series recorded before fixes rounds stopped
// starting holds it: spar run writes a candidate round of its range, whose
// round.json then names the stage fixes.
func (w *rrWorld) code(series, name, stage, base, head, claim string) string {
	w.t.Helper()
	w.useConfig(w.printer(w.answer(filepath.Base(series)+"-"+name, head, rptFinding("F1", "defect", "AC-1", claim))), w.late)
	out := filepath.Join(series, name)
	run := stage
	if stage == "fixes" {
		run = "candidate"
	}
	w.run(w.round(run, base, head, out)...)
	if run != stage {
		w.recordAs(out, stage)
	}
	if n := len(w.findings(out)); n != 1 {
		w.t.Fatalf("setup: %s recorded %d findings, want 1", name, n)
	}
	return out
}

func (w *rrWorld) findings(dir string) []Finding {
	w.t.Helper()
	var list []Finding
	if err := json.Unmarshal([]byte(w.read(filepath.Join(dir, "findings.json"))), &list); err != nil {
		w.t.Fatalf("setup: findings.json of %s: %v", filepath.Base(dir), err)
	}
	return list
}

// extend records one more code round for series in the user's words.
func (w *rrWorld) extend(series string) {
	w.t.Helper()
	if code, _, stderr := w.main("extend", series, "--stage", "code", "--by", "1", "--cause", "churn",
		"--user-quote", "synthetic words", "--repo", w.repo); code != ExitOK {
		w.t.Fatalf("setup: spar extend exits %d: %s", code, stderr)
	}
}

// rerun runs the candidate identity from base to head again into
// series/name with only the reviewer late, which did not complete it. A
// refusal fails the test: a rerun of the same packet is never refused at
// the cap.
func (w *rrWorld) rerun(series, name, base, head string) string {
	w.t.Helper()
	w.useConfig(w.printer(w.answer(filepath.Base(series)+"-"+name, head)), w.late)
	out := filepath.Join(series, name)
	code, stderr := w.spar(append(w.round("candidate", base, head, out), "--reviewers", "late")...)
	if code != ExitOK && code != ExitNoResult {
		w.t.Fatalf("the rerun %s of the candidate identity exits %d: %s", name, code, stderr)
	}
	return out
}

// checkRerun reports what rerun did not do: get original's packet byte for
// byte under the same digest, name the same earlier rounds in prior, and be
// recorded as an uncounted rerun of original.
func (w *rrWorld) checkRerun(rerun, original string, wantPrior []any) {
	w.t.Helper()
	if got, want := w.read(filepath.Join(rerun, "packet.md")), w.read(filepath.Join(original, "packet.md")); got != want {
		w.t.Errorf("%s gets another packet than %s; earlier findings %q, want %q", filepath.Base(rerun), filepath.Base(original),
			edgeLines(got), edgeLines(want))
	}
	r, o := w.rawRound(rerun), w.rawRound(original)
	if r["packet_sha256"] != o["packet_sha256"] {
		w.t.Errorf("%s has packet_sha256 %v, want %v as in %s", filepath.Base(rerun), r["packet_sha256"], o["packet_sha256"],
			filepath.Base(original))
	}
	if !reflect.DeepEqual(r["prior"], wantPrior) {
		w.t.Errorf("%s has prior %v, want %v", filepath.Base(rerun), r["prior"], wantPrior)
	}
	if r["counted"] != false || !reflect.DeepEqual(r["rerun_of"], []any{filepath.Base(original)}) {
		w.t.Errorf("%s is recorded with counted %v and rerun_of %v, want false and [%s]", filepath.Base(rerun),
			r["counted"], r["rerun_of"], filepath.Base(original))
	}
}

// edgeLines are the lines of a packet's earlier findings.
func edgeLines(packet string) []string {
	var lines []string
	for _, line := range strings.Split(pdSectionOf(packet, rptEarlierHeading), "\n") {
		if strings.HasPrefix(line, "- ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// A candidate round recorded by a Spar that gave candidate rounds no earlier
// findings has a packet without them and a prior that is null, missing or
// empty. A rerun of its identity after code rounds recorded before and after
// it gets that packet again and is not counted.
func TestCandidateRerunOfRoundWithoutPriorGetsNoEarlierFindings(t *testing.T) {
	for _, form := range []struct {
		name  string
		prior func(map[string]any)
	}{
		{"null", func(r map[string]any) { r["prior"] = nil }},
		{"missing", func(r map[string]any) { delete(r, "prior") }},
		{"empty", func(r map[string]any) { r["prior"] = []any{} }},
	} {
		t.Run(form.name, func(t *testing.T) {
			w := newRRWorld(t)
			c := w.commits
			series := filepath.Join(w.repo, "reviews", "old")
			w.code(series, "01-candidate", "candidate", c[0], c[1], "first candidate claim")
			w.code(series, "02-fixes", "fixes", c[1], c[2], "fixes claim")
			// What the earlier Spar wrote for a candidate round from c0 to
			// c2: the packet such a round gets in a series without earlier
			// rounds.
			fresh := filepath.Join(w.repo, "reviews", "fresh", "01-candidate")
			w.run(append(w.round("candidate", c[0], c[2], fresh), "--dry-run")...)
			packet := w.read(filepath.Join(fresh, "packet.md"))
			if lines := edgeLines(packet); len(lines) != 0 {
				t.Fatalf("setup: a candidate round in a series of its own lists earlier findings %q", lines)
			}
			original := w.code(series, "03-candidate", "candidate", c[0], c[2], "original claim")
			if lines := edgeLines(w.read(filepath.Join(original, "packet.md"))); len(lines) != 2 {
				t.Fatalf("setup: 03-candidate lists earlier findings %q, want the two of 01-candidate and 02-fixes", lines)
			}
			r := w.rawRound(original)
			form.prior(r)
			r["packet_sha256"] = w.rawRound(fresh)["packet_sha256"]
			data, err := json.MarshalIndent(r, "", "  ")
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			w.write(filepath.Join(original, "round.json"), string(data)+"\n")
			w.write(filepath.Join(original, "packet.md"), packet)
			w.extend(series)
			w.code(series, "04-fixes", "fixes", c[2], c[3], "later fixes claim")

			w.checkRerun(w.rerun(series, "05-candidate", c[0], c[2]), original, []any{})
		})
	}
}

// A candidate round recorded by this Spar after earlier code rounds names
// them in prior. A rerun of its identity after a later code round gets the
// same rounds, with the disposition recorded before the original, and not
// the later round.
func TestCandidateRerunGetsTheRoundsTheOriginalReceived(t *testing.T) {
	w := newRRWorld(t)
	c := w.commits
	series := filepath.Join(w.repo, "reviews", "new")
	first := w.code(series, "01-candidate", "candidate", c[0], c[1], "first candidate claim")
	w.triage(first, "cmd-F1", "reject", "--evidence", "AC-1 asks for the first greeting")
	w.code(series, "02-fixes", "fixes", c[1], c[2], "fixes claim")
	original := w.code(series, "03-candidate", "candidate", c[0], c[2], "original claim")
	want := []any{"01-candidate", "02-fixes"}
	if got := w.rawRound(original)["prior"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("setup: 03-candidate has prior %v, want %v", got, want)
	}
	w.extend(series)
	w.code(series, "04-fixes", "fixes", c[2], c[3], "later fixes claim")

	rerun := w.rerun(series, "05-candidate", c[0], c[2])
	w.checkRerun(rerun, original, want)
	lines := edgeLines(w.read(filepath.Join(rerun, "packet.md")))
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "- 01-candidate cmd-F1 ") ||
		!strings.HasSuffix(lines[0], " — reject: AC-1 asks for the first greeting") ||
		!strings.HasPrefix(lines[1], "- 02-fixes cmd-F1 ") {
		t.Errorf("05-candidate lists earlier findings %q, want 01-candidate's with its disposition and 02-fixes'", lines)
	}
}

// A rerun cannot get what the original received when a round the original's
// prior names is gone, so it is refused before its round is written.
func TestCandidateRerunRefusedWhenAPriorRoundIsGone(t *testing.T) {
	w := newRRWorld(t)
	c := w.commits
	series := filepath.Join(w.repo, "reviews", "gone")
	first := w.code(series, "01-candidate", "candidate", c[0], c[1], "first candidate claim")
	w.code(series, "02-fixes", "fixes", c[1], c[2], "fixes claim")
	w.code(series, "03-candidate", "candidate", c[0], c[2], "original claim")
	if err := os.RemoveAll(first); err != nil {
		t.Fatalf("setup: %v", err)
	}
	w.useConfig(w.printer(w.answer("gone-rerun", c[2])), w.late)
	out := filepath.Join(series, "04-candidate")
	code, stderr := w.spar(append(w.round("candidate", c[0], c[2], out), "--reviewers", "late")...)
	if code != ExitRefused || !strings.Contains(stderr, "01-candidate") {
		t.Errorf("a rerun of 03-candidate without 01-candidate exits %d: %s; want 65 naming 01-candidate", code, stderr)
	}
	if _, err := os.Lstat(out); err == nil {
		t.Errorf("the refused rerun wrote %s", filepath.Base(out))
	}
}
