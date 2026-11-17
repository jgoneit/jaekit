//go:build !windows

package spar

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/reviewseries"
)

// Checks of record lines that spar extend never writes but a hand edit or
// another tool can (issue #214). They drive the spar command line on the
// synthetic repository of cap_report_test.go; Core's series reader (the one
// ha cost uses) is called directly.

// rlGate runs spar gate over the four code rounds of a main series with a
// fourth fixes round and returns its exit and its file without the fields
// that differ between two runs by construction.
func (w *crWorld) rlGate(series, out string) (int, map[string]any) {
	w.t.Helper()
	code, _, errs := w.spar("gate", filepath.Join(series, "01-candidate"), filepath.Join(series, "02-fixes"),
		filepath.Join(series, "03-candidate"), filepath.Join(series, "04-fixes"), "--expect-head", w.commit["head3"],
		"--out", out, "--repo", w.repo)
	var doc map[string]any
	if err := json.Unmarshal(w.read(out), &doc); err != nil {
		w.t.Fatalf("setup: spar gate exits %d with no gate file: %v: %s", code, err, errs)
	}
	delete(doc, "written_at")
	delete(doc, "previous_gate_digest")
	return code, doc
}

// rlFifthRound starts a fifth counted code round, which the code cap of 3
// with one extension refuses; it returns the exit and the reason.
func (w *crWorld) rlFifthRound(series string) (int, string) {
	w.t.Helper()
	out := filepath.Join(series, "05-candidate")
	code, _, errs := w.spar("run", "--stage", "candidate", "--base", w.commit["base"], "--head", w.commit["head3"],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", out, "--repo", w.repo)
	if code == ExitOK {
		w.t.Fatalf("setup: a fifth code round ran: %s", out)
	}
	return code, errs
}

// The extension lines: one of each form spar extend writes, and lines whose
// cause or reason is not text, which it never writes.
const (
	rlPlain = `{"stage":"code","by":1,"user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`
	rlCause = `{"stage":"code","by":1,"cause":"churn","reason":"synthetic reason","user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`
)

var rlNotText = []struct{ what, line string }{
	{"a numeric cause", `{"stage":"code","by":1,"cause":1,"user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`},
	{"a cause that is an object", `{"stage":"code","by":1,"cause":{"kind":"churn"},"user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`},
	{"a reason that is true", `{"stage":"code","by":1,"cause":"churn","reason":true,"user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`},
	{"a numeric reason without a cause", `{"stage":"code","by":1,"reason":2,"user_quote":"synthetic words","recorded_at":"2026-01-01T00:00:00Z"}`},
}

// Issue #214: spar run and spar gate read no cause or reason of an
// extension line, so a line whose cause or reason is not text admits,
// counts, reruns and gates exactly as the same line without them; its stage
// and by still count.
func TestExtensionCauseOrReasonNotTextDecidesAsWithoutThemInRunAndGate(t *testing.T) {
	const v = "extension-cause-type-changes-decision"
	w := newCRWorld(t)
	series := w.mainSeries("rl-ext")
	ext := filepath.Join(series, "extensions.jsonl")

	// spar run admits a fourth code round under the line with a numeric
	// cause: the cap of 3 and the line's by of 1.
	w.write(ext, rlNotText[0].line+"\n")
	w.use("host", "aide")
	r4 := filepath.Join(series, "04-fixes")
	code, _, errs := w.spar("run", "--stage", "fixes", "--base", w.commit["head2"], "--head", w.commit["head3"],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", r4, "--repo", w.repo)
	if code != ExitOK {
		t.Fatalf("jaekit:violation=%s spar run under an extension line with %s exits %d: %s", v, rlNotText[0].what, code, errs)
	}
	var counted struct {
		Counted bool `json:"counted"`
	}
	if json.Unmarshal(w.read(filepath.Join(r4, "round.json")), &counted) != nil || !counted.Counted {
		t.Errorf("jaekit:violation=%s the fourth code round under an extension line with %s is not counted", v, rlNotText[0].what)
	}
	w.record(r4, "host", "head3")

	// The gate file and the cap refusal of a fifth code round are the same
	// for every line.
	w.write(ext, rlPlain+"\n")
	wantCode, wantGate := w.rlGate(series, filepath.Join(w.dir, "gate-plain.json"))
	if wantCode != ExitBlocked {
		t.Fatalf("setup: spar gate exits %d, want blocked by the open findings", wantCode)
	}
	refusalCode, refusal := w.rlFifthRound(series)
	if refusalCode != ExitRefused || !strings.Contains(refusal, "4 of 4 rounds ran") {
		t.Fatalf("setup: a fifth code round exits %d (%q), want the cap refusal at 4 of 4", refusalCode, refusal)
	}
	lines := append([]struct{ what, line string }{{"a cause and a reason", rlCause}}, rlNotText...)
	for i, c := range lines {
		w.write(ext, c.line+"\n")
		code, gate := w.rlGate(series, filepath.Join(w.dir, fmt.Sprintf("gate-%d.json", i)))
		if code != wantCode || !reflect.DeepEqual(gate, wantGate) {
			g, _ := json.Marshal(gate)
			p, _ := json.Marshal(wantGate)
			t.Errorf("jaekit:violation=%s spar gate with an extension line with %s exits %d with\n%s\nwant %d with the gate of the line without cause and reason:\n%s",
				v, c.what, code, g, wantCode, p)
		}
		if code, msg := w.rlFifthRound(series); code != refusalCode || msg != refusal {
			t.Errorf("jaekit:violation=%s with an extension line with %s a fifth code round exits %d (%q), want %d (%q)",
				v, c.what, code, msg, refusalCode, refusal)
		}
	}

	// A reviewer that did not complete 04-fixes reruns its packet uncounted
	// under the line with a reason that is true.
	w.write(ext, rlNotText[2].line+"\n")
	rerun := filepath.Join(series, "04b-fixes")
	code, _, errs = w.spar("run", "--stage", "fixes", "--base", w.commit["head2"], "--head", w.commit["head3"],
		"--criteria", w.criteria, "--rules", w.rules, "--check-path", "checks/**", "--out", rerun, "--repo", w.repo,
		"--reviewers", "aide")
	var rec struct {
		Counted bool     `json:"counted"`
		RerunOf []string `json:"rerun_of"`
	}
	switch {
	case code != ExitOK:
		t.Errorf("jaekit:violation=%s under an extension line with %s a rerun of the same packet exits %d: %s", v, rlNotText[2].what, code, errs)
	case json.Unmarshal(w.read(filepath.Join(rerun, "round.json")), &rec) != nil || rec.Counted ||
		!reflect.DeepEqual(rec.RerunOf, []string{"04-fixes"}):
		t.Errorf("jaekit:violation=%s under an extension line with %s a rerun of the same packet is %+v, want uncounted of 04-fixes",
			v, rlNotText[2].what, rec)
	}
}

// Issue #214: the displays that show an extension's cause and reason, spar
// cap-report and the series reader of ha cost, refuse a series with a line
// whose cause or reason is not text, with the error they give any other
// extension line they cannot read; lines in the forms spar extend writes
// keep showing their cause and reason.
func TestExtensionCauseOrReasonNotTextRefusedByDisplays(t *testing.T) {
	const v = "extension-display-reads-broken-cause"
	w := newCRWorld(t)
	series := w.mainSeries("rl-display")
	ext := filepath.Join(series, "extensions.jsonl")
	for _, c := range []struct {
		line          string
		cause, reason string
	}{{rlPlain, "", ""}, {rlCause, "churn", "synthetic reason"}} {
		w.write(ext, c.line+"\n")
		doc := w.reportJSON(v, series)
		crExpect(t, v, "the cap with the extension line "+c.line, doc["cap"],
			map[string]any{"code": 3, "extensions": 1, "allowed": 4, "counted": 3})
		s, err := reviewseries.Read(series)
		if err != nil {
			t.Errorf("jaekit:violation=%s ha cost's series reader refuses the extension line %s: %v", v, c.line, err)
			continue
		}
		want := []reviewseries.Extension{{Stage: "code", By: 1, Cause: c.cause, Reason: c.reason, UserQuote: "synthetic words"}}
		if !reflect.DeepEqual(s.Extensions, want) {
			t.Errorf("jaekit:violation=%s ha cost's series reader reads the extension line %s as %+v, want %+v", v, c.line, s.Extensions, want)
		}
	}
	// The error each display gives a line that is not JSON names the file;
	// a line whose cause or reason is not text gets the same kind of error.
	for _, c := range append([]struct{ what, line string }{{"a line that is not JSON", `{"stage": "code", "by":`}}, rlNotText...) {
		w.write(ext, c.line+"\n")
		code, out, errs := w.report(series, "--format", "json")
		if code != ExitRefused || out != "" || !strings.Contains(errs, ext+" line 1") {
			t.Errorf("jaekit:violation=%s spar cap-report with an extension line with %s: exit %d, report %q, reason %q; want 65, no report and a reason naming %s line 1",
				v, c.what, code, out, errs, ext)
		}
		if _, err := reviewseries.Read(series); err == nil || !strings.Contains(err.Error(), ext+" line 1 cannot be read") {
			t.Errorf("jaekit:violation=%s ha cost's series reader with an extension line with %s gives %v, want the error naming %s line 1",
				v, c.what, err, ext)
		}
	}
}
