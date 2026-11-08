package spar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Checks of the goal spar-agy-readonly: agy's shell commands (contracts/
// spar-input.md G11, contracts/review-round.md sections 2 and 4). The
// command lines come from the G11 representative forms in
// testdata/input-forms.json (agy_commands). A failure's opening line names
// the property it observes (jaekit:violation=<id>); a failure that starts
// with "setup:" is a broken setup, not an observation.

type agyCommandForms struct {
	Accepted   []string `json:"accepted"`
	Kept       []string `json:"kept"`
	Violations []struct {
		Line    string `json:"line"`
		Problem string `json:"problem"`
	} `json:"violations"`
}

// arLoadForms reads the G11 forms; a missing or empty list is a broken
// setup, never a pass.
func arLoadForms(t *testing.T) agyCommandForms {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "input-forms.json"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var doc struct {
		AgyCommands *agyCommandForms `json:"agy_commands"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("setup: input-forms.json: %v", err)
	}
	f := doc.AgyCommands
	if f == nil || len(f.Accepted) == 0 || len(f.Kept) == 0 || len(f.Violations) == 0 {
		t.Fatal("setup: input-forms.json has no agy_commands with accepted, kept and violations")
	}
	for _, v := range f.Violations {
		if v.Line == "" || v.Problem == "" {
			t.Fatalf("setup: an agy_commands violation lacks its line or problem: %+v", v)
		}
	}
	return *f
}

// arRunCommand is an agy tool step in the form agy prints it: a run_command
// step_update reported while it runs.
func arRunCommand(line string) map[string]any {
	return map[string]any{"step_update": map[string]any{"step_index": 2, "state": "ACTIVE", "step_type": "tool",
		"tool_name": "run_command", "tool_info": map[string]any{"name": "run_command",
			"parameters": map[string]any{"CommandLine": line}}}}
}

func arAgySpec(t *testing.T, packet []byte) (reviewSpec, string) {
	t.Helper()
	ws := t.TempDir()
	return agySpec(Reviewer{Name: "agy", Preset: "agy", TimeoutSeconds: 60}, "m", "agy",
		runPaths{workspace: ws, dir: t.TempDir(), schema: filepath.Join(ws, "schema.json")}, packet), ws
}

// AC-1: a line that joins read-only commands with |, &&, || or ; is not a
// violation, in the command check and in agy's event check.
func TestSparAgyReadonlyAC1_JoinedReadOnlyCommands(t *testing.T) {
	const id = "joined-read-only-command-refused"
	forms := arLoadForms(t)
	spec, _ := arAgySpec(t, []byte("packet"))
	for _, line := range forms.Accepted {
		if why := unsafeCommand(line); why != "" {
			t.Errorf("jaekit:violation=%s the command check refuses %q: %s", id, line, why)
		}
		if why := spec.checkEvent(arRunCommand(line)); why != "" {
			t.Errorf("jaekit:violation=%s agy's event check stops %q: %s", id, line, why)
		}
	}
}

// AC-2 (kept): every other line stays a violation, and the reason names the
// line and the first problem in it.
func TestSparAgyReadonlyAC2_OtherCommandsStayViolations(t *testing.T) {
	forms := arLoadForms(t)
	spec, _ := arAgySpec(t, []byte("packet"))
	for _, v := range forms.Violations {
		if unsafeCommand(v.Line) == "" {
			t.Errorf("%q is not a violation", v.Line)
			continue
		}
		reason := spec.checkEvent(arRunCommand(v.Line))
		if reason == "" {
			t.Errorf("agy's event check lets %q run", v.Line)
			continue
		}
		if !strings.Contains(reason, v.Line) || !strings.Contains(reason, strconv.Quote(v.Problem)) {
			t.Errorf("the reason for %q does not name the line and %q: %s", v.Line, v.Problem, reason)
		}
	}
}

// AC-3 (kept): the lines accepted before stay accepted: one program, a |
// pipeline, sed -n printing, 2>/dev/null or 2>&1 after a command.
func TestSparAgyReadonlyAC3_KeptCommands(t *testing.T) {
	forms := arLoadForms(t)
	spec, _ := arAgySpec(t, []byte("packet"))
	for _, line := range forms.Kept {
		if why := unsafeCommand(line); why != "" {
			t.Errorf("the command check refuses %q: %s", line, why)
		}
		if why := spec.checkEvent(arRunCommand(line)); why != "" {
			t.Errorf("agy's event check stops %q: %s", line, why)
		}
	}
}

// arNoteTokens are what agy's command note says: the file tools first, the
// programs and operators a shell command may use, and what stops a review.
func arNoteTokens() []string {
	tokens := []string{"view_file", "read_file", "list_dir", "grep_search", "find_by_name",
		"sed -n", "|", "&&", "||", ";", "true", "tests", "builds", "git", "stops", "discards", "tool_violation"}
	for name := range readOnlyPrograms {
		tokens = append(tokens, name)
	}
	slices.Sort(tokens)
	return tokens
}

// AC-4: agy's prompt opens with a command note apart from the packet, by
// argument and by PACKET.md; the packet and what the other reviewers get
// stay the same, and the round record shows the note.
func TestSparAgyReadonlyAC4_PromptNote(t *testing.T) {
	const id = "agy-command-note-missing"
	packet := []byte("# Review packet\n\nsmall synthetic packet\n")
	small, _ := arAgySpec(t, packet)
	if len(small.argv) < 3 || small.argv[1] != "-p" {
		t.Fatalf("setup: agy's argv has no -p prompt: %q", small.argv)
	}
	prompt := small.argv[2]
	note, ok := strings.CutSuffix(prompt, string(packet))
	switch {
	case small.transport != "argument":
		t.Errorf("jaekit:violation=%s a small packet goes by %s, not by argument", id, small.transport)
	case !ok:
		t.Errorf("jaekit:violation=%s agy's prompt does not end with the packet unchanged", id)
	case strings.TrimSpace(note) == "":
		t.Errorf("jaekit:violation=%s agy's prompt is the packet with no command note before it", id)
	default:
		for _, token := range arNoteTokens() {
			if !strings.Contains(note, token) {
				t.Errorf("jaekit:violation=%s agy's command note does not say %q", id, token)
			}
		}
		if len(small.display) < 3 || !strings.HasPrefix(small.display[2], note) ||
			!strings.HasSuffix(small.display[2], "<packet.md>") || strings.Contains(small.display[2], "small synthetic packet") {
			t.Errorf("jaekit:violation=%s the recorded argv does not show the note before <packet.md>: %q", id, small.display)
		}
	}

	// A packet under the limit alone that the note pushes over it goes
	// through PACKET.md, unchanged, and the short prompt opens with the note.
	edge := []byte(strings.Repeat("x", argumentLimit()-1))
	spec, ws := arAgySpec(t, edge)
	data, err := os.ReadFile(filepath.Join(ws, packetFile))
	switch {
	case spec.transport != "file":
		t.Errorf("jaekit:violation=%s a packet the note pushes over the argument limit goes by %s", id, spec.transport)
	case err != nil || string(data) != string(edge):
		t.Errorf("jaekit:violation=%s PACKET.md is not the packet unchanged: %v", id, err)
	case note == "" || !strings.HasPrefix(spec.argv[2], note) || !strings.Contains(spec.argv[2], packetFile) || len(spec.argv[2]) > 8192:
		t.Errorf("jaekit:violation=%s the PACKET.md prompt does not open with the command note: %q", id, short(spec.argv[2], 300))
	}

	// The other presets read the packet unchanged on standard input.
	p := runPaths{workspace: t.TempDir(), dir: t.TempDir(), schema: "schema.json"}
	for _, preset := range []string{"claude", "codex"} {
		other := specFor(Reviewer{Name: preset, Preset: preset, TimeoutSeconds: 60}, "m", preset, p, packet)
		if string(other.stdin) != string(packet) {
			t.Errorf("jaekit:violation=%s %s does not read the packet unchanged on standard input", id, preset)
		}
	}

	// A real round: the agy CLI receives the note and the round's packet.md,
	// and packet.md is the file packet_sha256 names.
	w := newAMWorld(t, false)
	w.use(w.entry("agy", "agy", `fallback_model = ""`))
	out := w.round("note")
	if code, errs := w.run(out); code != ExitOK {
		t.Fatalf("setup: spar run exits %d: %s", code, errs)
	}
	written, err := os.ReadFile(filepath.Join(out, "packet.md"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	rec := w.record(out)
	sum := sha256.Sum256(written)
	if amStr(rec, "packet_sha256") != hex.EncodeToString(sum[:]) {
		t.Errorf("jaekit:violation=%s packet_sha256 is not the sha256 of packet.md", id)
	}
	calls := w.calls("agy")
	if len(calls) != 1 || len(calls[0]) < 2 || calls[0][1] != note+string(written) {
		t.Errorf("jaekit:violation=%s the agy CLI was not given the note followed by packet.md (%d calls)", id, len(calls))
	}
	state := amState(rec, "agy")
	attempts, _ := state["attempts"].([]any)
	var argv []any
	if len(attempts) == 1 {
		argv, _ = attempts[0].(map[string]any)["argv"].([]any)
	}
	if len(argv) < 3 || argv[2] != note+"<packet.md>" {
		t.Errorf("jaekit:violation=%s round.json does not show the note agy received: %v", id, argv)
	}
}

// arAgyFake writes an agy CLI that prints, after its init event, a
// run_command step for each line and then a result for head with one
// finding.
func arAgyFake(t *testing.T, w *rwWorld, name string, lines ...string) string {
	t.Helper()
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	result := map[string]any{"result": map[string]any{"status": "success", "structured_output": map[string]any{
		"reviewed_head": w.head, "verdict": "blocking_findings", "angles_checked": checked, "angles_unchecked": []any{},
		"findings": []any{rawFinding("F1", map[string]any{"location": "head/app/main.py:1"})}, "limits": "synthetic reviewer"}}}
	var events []string
	for _, e := range append([]any{map[string]any{"event": "init", "init": map[string]any{"model": "synthetic"}}},
		append(arEvents(lines), result)...) {
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		events = append(events, string(data))
	}
	file := w.write(filepath.Join(w.dir, "agy-events", name+".jsonl"), strings.Join(events, "\n")+"\n")
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'agy 0.0.0 (synthetic)'; exit 0; fi\ncat " + amQuote(file) + "\n"
	path := w.write(filepath.Join(w.dir, "agy-bin", name), script)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return fmt.Sprintf("[[reviewers]]\nname = \"agy\"\npreset = \"agy\"\nbinary = %q\nfallback_model = \"\"\n\n", path)
}

func arEvents(lines []string) []any {
	var out []any
	for _, l := range lines {
		out = append(out, arRunCommand(l))
	}
	return out
}

// AC-5: the monitor of a real spar run judges agy's run_command steps by the
// new rule: joined read-only commands let agy finish with its findings; any
// other command stops it as tool_violation.
func TestSparAgyReadonlyAC5_RealRunMonitor(t *testing.T) {
	const id = "joined-command-stops-review"
	w := newRWWorld(t, false)
	w.use(arAgyFake(t, w, "joined", "ls -la && ls -la head base",
		"diff -u base/app/main.py head/app/main.py; diff -u base/AGENTS.md head/AGENTS.md",
		"ls -la head/app 2>&1 || true"))
	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	r, err := loadRound(out)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	st := r.Reviewers["agy"]
	switch {
	case st == nil:
		t.Fatalf("setup: the round has no agy reviewer")
	case st.Status != StatusCompleted:
		t.Errorf("jaekit:violation=%s agy with joined read-only commands ends %s: %s", id, st.Status, st.Reason)
	case st.Findings == nil || *st.Findings != 1:
		t.Errorf("jaekit:violation=%s the round does not record agy's finding: %v", id, st.Findings)
	}
	if st != nil && st.Status == StatusCompleted {
		data, err := os.ReadFile(filepath.Join(out, "findings.json"))
		if err != nil || !strings.Contains(string(data), "agy-F1") {
			t.Errorf("jaekit:violation=%s findings.json does not hold agy-F1: %v", id, err)
		}
	}

	const test = "cd head && go test ./..."
	w.use(arAgyFake(t, w, "test", "ls -la && ls -la head base", test))
	out = w.run(filepath.Join(w.newSeries(), "01-candidate"))
	if r, err = loadRound(out); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if st := r.Reviewers["agy"]; st == nil || st.Status != StatusToolViolation || !strings.Contains(st.Reason, test) {
		t.Errorf("jaekit:violation=%s agy that runs tests is not stopped as tool_violation naming the line: %+v", id, st)
	}
}

// AC-7 (kept): a round recorded before this change keeps its agy status and
// its gate. The gate reads the recorded status; it does not judge agy's
// recorded commands again, even one the new rule accepts.
func TestSparAgyReadonlyAC7_RecordedRoundsKeepTheirGate(t *testing.T) {
	w := newRWWorld(t, true)
	w.use(rwNative("host"), w.missingAgy())
	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	r, err := loadRound(out)
	if err != nil {
		t.Fatal(err)
	}
	const old = "ls -la && ls -la head base"
	reason := `ran a command that is not read-only (shell operator "&&"): ` + old
	st := r.Reviewers["agy"]
	if st == nil {
		t.Fatal("the round has no agy reviewer")
	}
	st.Status, st.Reason = StatusToolViolation, reason
	for i := range st.Attempts {
		st.Attempts[i].Status, st.Attempts[i].Reason = StatusToolViolation, reason
	}
	if err := writeJSON(filepath.Join(out, "round.json"), r); err != nil {
		t.Fatal(err)
	}
	events, err := json.Marshal(arRunCommand(old))
	if err != nil {
		t.Fatal(err)
	}
	w.write(filepath.Join(out, "agy", "stdout.jsonl"), string(events)+"\n")

	g := w.gate(w.head, []string{out})
	w.expectReasons(g, out+": required reviewer agy is tool_violation")
	if again, err := loadRound(out); err != nil || again.Reviewers["agy"].Status != StatusToolViolation {
		t.Errorf("the recorded agy status changed: %v", err)
	}
	w.waive(out, "agy", "agy", StatusToolViolation)
	if g := w.gate(w.head, []string{out}); g.rec.State != StateComplete {
		t.Errorf("with the user's waiver the gate is %s: %v", g.rec.State, g.rec.Reasons)
	}
}

// AC-8 (kept): without configuration the reviewers are claude, codex and
// agy, and the sources keep their order: SPAR_CONFIG, XDG_CONFIG_HOME,
// HOME, then the clean-review list, then the default list.
func TestSparAgyReadonlyAC8_DefaultReviewers(t *testing.T) {
	if !slices.Equal(DefaultReviewers, []string{"claude", "codex", "agy"}) {
		t.Errorf("the default list is %v", DefaultReviewers)
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	env := map[string]string{"HOME": filepath.Join(dir, "home")}
	names := func(label string) ([]string, string) {
		t.Helper()
		c, err := LoadConfig(repo, func(k string) string { return env[k] }, nil)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		var list []string
		for _, r := range c.Reviewers {
			list = append(list, r.Name)
		}
		return list, c.Source.Kind
	}
	steps := []struct {
		label, kind string
		want        []string
		set         func()
	}{
		{"no configuration", "default", []string{"claude", "codex", "agy"}, func() {}},
		{"a clean-review list", "clean-review", []string{"codex"}, func() {
			write(filepath.Join(dir, "home", ".config", "clean-review", "config.json"), `{"reviewers": ["codex"]}`)
		}},
		{"HOME's spar.toml", "HOME", []string{"h"}, func() {
			write(filepath.Join(dir, "home", ".config", "jaekit", "spar.toml"), "[[reviewers]]\nname = \"h\"\npreset = \"native\"\n")
		}},
		{"XDG_CONFIG_HOME's spar.toml", "XDG_CONFIG_HOME", []string{"x"}, func() {
			env["XDG_CONFIG_HOME"] = filepath.Join(dir, "xdg")
			write(filepath.Join(dir, "xdg", "jaekit", "spar.toml"), "[[reviewers]]\nname = \"x\"\npreset = \"native\"\n")
		}},
		{"SPAR_CONFIG", "SPAR_CONFIG", []string{"s"}, func() {
			env["SPAR_CONFIG"] = write(filepath.Join(dir, "s.toml"), "[[reviewers]]\nname = \"s\"\npreset = \"native\"\n")
		}},
	}
	for _, s := range steps {
		s.set()
		got, kind := names(s.label)
		if !slices.Equal(got, s.want) || kind != s.kind {
			t.Errorf("%s: reviewers %v from %s, want %v from %s", s.label, got, kind, s.want, s.kind)
		}
	}
}
