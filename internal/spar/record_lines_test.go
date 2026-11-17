//go:build !windows

package spar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/reviewseries"
)

// Checks of record lines that spar extend and spar triage never write but a
// hand edit or another tool can (issues #213 and #214). They drive the spar
// command line on the synthetic repository of cap_report_test.go; Core's
// series reader (the one ha cost uses) is called directly.

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

// rlEntry is a ledger line a hand edit or another tool could write.
type rlEntry struct {
	what   string
	round  string
	fields map[string]any
	// names is a word the refusal must contain: the missing key or the rule.
	names string
}

// rlClone copies a series to a new one beside it and appends entries to the
// ledger of one of its rounds.
func (w *crWorld) rlClone(series, name, round string, entries ...map[string]any) string {
	w.t.Helper()
	dst := filepath.Join(filepath.Dir(series), name)
	if err := os.CopyFS(dst, os.DirFS(series)); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	ledger := filepath.Join(dst, round, "ledger.jsonl")
	body := string(w.read(ledger))
	for _, e := range entries {
		e["recorded_at"] = "2026-01-01T00:00:00Z"
		data, err := json.Marshal(e)
		if err != nil {
			w.t.Fatalf("setup: %v", err)
		}
		body += string(data) + "\n"
	}
	w.write(ledger, body)
	return dst
}

// rlTriageArgs turns an entry a person could make into spar triage's
// arguments: each ledger key is the option of the same name.
func rlTriageArgs(e map[string]any) []string {
	args := []string{e["finding"].(string), e["disposition"].(string)}
	for _, key := range []string{"commit", "failed_before", "passed_after", "evidence", "of", "to", "user_quote"} {
		if val, ok := e[key]; ok {
			args = append(args, "--"+strings.ReplaceAll(key, "_", "-"), val.(string))
		}
	}
	return args
}

// Issue #213: spar cap-report refuses a ledger entry that lacks a value its
// disposition requires, as spar triage refuses to write it, instead of
// letting it close or exclude the finding; an automatic deferral in the form
// spar run writes stays valid.
func TestCapReportRefusesLedgerEntryMissingItsDispositionFields(t *testing.T) {
	const v = "cap-report-malformed-entry-closes-finding"
	w := newCRWorld(t)
	series := w.mainSeries("rl-ledger")
	if code, _, errs := w.report(series, "--format", "json"); code != ExitOK {
		t.Fatalf("setup: spar cap-report of the series exits %d: %s", code, errs)
	}
	// 01-candidate: host-F3 is a hardening spar deferred automatically.
	// 03-candidate: host-F1, host-F2 and host-F4 are open defects (host-F4
	// a duplicate of host-F1) and host-F3 an open requirement question.
	fix := func(finding, commit string) map[string]any {
		return map[string]any{"finding": finding, "disposition": "fix", "commit": w.commit[commit],
			"failed_before": "synthetic failure", "passed_after": "synthetic pass"}
	}
	without := func(e map[string]any, keys ...string) map[string]any {
		for _, k := range keys {
			delete(e, k)
		}
		return e
	}
	with := func(e map[string]any, key string, val any) map[string]any {
		e[key] = val
		return e
	}
	entry := func(finding, disposition string, kv ...string) map[string]any {
		e := map[string]any{"finding": finding, "disposition": disposition}
		for i := 0; i+1 < len(kv); i += 2 {
			e[kv[i]] = kv[i+1]
		}
		return e
	}
	for i, c := range []rlEntry{
		{"a fix without commit", "03-candidate", without(fix("host-F2", "head3"), "commit"), `needs "commit"`},
		{"a fix without failed_before", "03-candidate", without(fix("host-F2", "head3"), "failed_before"), `"failed_before"`},
		{"a fix with a blank passed_after", "03-candidate", with(fix("host-F2", "head3"), "passed_after", "  "), `"passed_after"`},
		{"a reject of a defect without evidence", "03-candidate", entry("host-F2", "reject"), `needs "evidence"`},
		{"a defer without to", "03-candidate", entry("host-F2", "defer", "user_quote", "synthetic words"), `needs "to"`},
		{"a defer of a defect without user_quote", "03-candidate", entry("host-F2", "defer", "to", "issue 1"), `needs "user_quote"`},
		{"a duplicate without of", "03-candidate", entry("host-F2", "duplicate"), `needs "of"`},
		{"a duplicate of itself", "03-candidate", entry("host-F2", "duplicate", "of", "host-F2"), `needs "of"`},
		{"a duplicate of no finding of the round", "03-candidate", entry("host-F2", "duplicate", "of", "host-F9"), `"of" host-F9 is not a finding`},
		{"a defect duplicating a requirement question", "03-candidate", entry("host-F2", "duplicate", "of", "host-F3"), "can only duplicate"},
		{"an answer to a defect", "03-candidate", entry("host-F2", "answer", "user_quote", "synthetic words"), "answered"},
		{"a reject of a requirement question without user_quote", "03-candidate", entry("host-F3", "reject", "evidence", "synthetic evidence"), `needs "user_quote"`},
		{"an answer without user_quote", "03-candidate", entry("host-F3", "answer"), `needs "user_quote"`},
		{"a fix of a hardening without user_quote", "01-candidate", fix("host-F3", "head2"), `needs "user_quote"`},
		{"an automatic deferral of a defect", "03-candidate", with(entry("host-F2", "defer", "reason", "synthetic", "to", "deferred hardening"), "auto", true), "automatic"},
		{"an automatic deferral without a reason", "01-candidate", with(entry("host-F3", "defer", "to", "deferred hardening"), "auto", true), "automatic"},
		{"an automatic fix", "03-candidate", with(fix("host-F2", "head3"), "auto", true), "automatic"},
	} {
		ledger := filepath.Join(w.rlClone(series, fmt.Sprintf("rl-refused-%d", i), c.round, c.fields), c.round, "ledger.jsonl")
		code, out, errs := w.report(filepath.Dir(filepath.Dir(ledger)), "--format", "json")
		if code != ExitRefused || out != "" || !strings.Contains(errs, ledger) || !strings.Contains(errs, c.fields["finding"].(string)) ||
			!strings.Contains(errs, c.names) {
			t.Errorf("jaekit:violation=%s spar cap-report with %s: exit %d, report %q, reason %q; want 65, no report and a reason naming %s, %s and %q",
				v, c.what, code, out, errs, ledger, c.fields["finding"], c.names)
		}
		// spar triage refuses to write the same entry, and writes nothing.
		if auto, _ := c.fields["auto"].(bool); auto {
			continue
		}
		round := filepath.Join(series, c.round)
		before := string(w.read(filepath.Join(round, "ledger.jsonl")))
		if code, _, errs := w.spar(append([]string{"triage", round}, rlTriageArgs(c.fields)...)...); code != ExitRefused {
			t.Errorf("jaekit:violation=%s spar triage writes %s (exit %d: %s), which spar cap-report refuses", v, c.what, code, errs)
		}
		if after := string(w.read(filepath.Join(round, "ledger.jsonl"))); after != before {
			t.Fatalf("jaekit:violation=%s spar triage wrote %s into %s/ledger.jsonl; later cases would read it", v, c.what, round)
		}
	}

	// Entries in the forms spar writes report as before: an automatic
	// deferral of a hardening with a reason, with or without to, and
	// complete entries that close the finding they name.
	open := func(doc map[string]any, round, finding string) bool {
		list, _ := doc["open"].([]any)
		for _, item := range list {
			if m, _ := item.(map[string]any); m["round"] == round && m["finding"] == finding {
				return true
			}
		}
		return false
	}
	unrelated := func(doc map[string]any, round, finding string) bool {
		list, _ := doc["unrelated"].([]any)
		for _, item := range list {
			if m, _ := item.(map[string]any); m["round"] == round && m["finding"] == finding {
				return true
			}
		}
		return false
	}
	base := w.reportJSON(v, series)
	if !open(base, "03-candidate", "host-F2") || !open(base, "03-candidate", "host-F3") || !unrelated(base, "03-candidate", "host-F2") {
		t.Fatalf("setup: the report of the series does not list 03-candidate host-F2 and host-F3 as open: %v", base["open"])
	}
	for i, c := range []struct {
		what    string
		round   string
		entries []map[string]any
		closes  string
	}{
		{"an automatic deferral of a hardening with a reason", "01-candidate",
			[]map[string]any{with(entry("host-F3", "defer", "reason", "hardening: synthetic", "to", "deferred hardening"), "auto", true)}, ""},
		{"an automatic deferral of a hardening with a reason and no to", "01-candidate",
			[]map[string]any{with(entry("host-F3", "defer", "reason", "hardening: synthetic"), "auto", true)}, ""},
		{"a complete fix", "03-candidate", []map[string]any{fix("host-F2", "head3")}, "host-F2"},
		{"a complete defer of a defect", "03-candidate", []map[string]any{entry("host-F2", "defer", "to", "issue 1", "user_quote", "synthetic words")}, "host-F2"},
		{"a complete reject", "03-candidate", []map[string]any{entry("host-F2", "reject", "evidence", "synthetic evidence")}, "host-F2"},
		{"a complete answer", "03-candidate", []map[string]any{entry("host-F3", "answer", "user_quote", "synthetic words")}, "host-F3"},
		{"a duplicate of a defect a complete fix closed", "03-candidate",
			[]map[string]any{fix("host-F1", "head3"), entry("host-F2", "duplicate", "of", "host-F1")}, "host-F2"},
	} {
		clone := w.rlClone(series, fmt.Sprintf("rl-accepted-%d", i), c.round, c.entries...)
		doc := w.reportJSON(v, clone)
		if c.closes == "" {
			crExpect(t, v, "the open findings with "+c.what, doc["open"], base["open"])
			continue
		}
		if open(doc, c.round, c.closes) {
			t.Errorf("jaekit:violation=%s with %s, %s %s is still open: %v", v, c.what, c.round, c.closes, doc["open"])
		}
		if c.what == "a complete reject" && unrelated(doc, c.round, c.closes) {
			t.Errorf("jaekit:violation=%s with %s, %s %s still counts as an unrelated defect", v, c.what, c.round, c.closes)
		}
	}
}
