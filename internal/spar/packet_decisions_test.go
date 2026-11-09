package spar

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Checks of the goal spar-packet-decisions, AC-1 to AC-6: spar run takes the
// user's decisions as --decision and --decision-quote pairs, puts them in one
// section of packet.md with fixed guidance, gives every preset that packet,
// keeps them in round.json and refuses a decision out of form before any
// reviewer starts; a packet without decisions stays as it was.
//
// A change check names the property it observes on the first line of each
// failure (jaekit:violation=<id>). spar refusing the decision options is that
// observation. A setup that breaks (no git, a file, repository or fake CLI
// that cannot be made or run, a round that cannot be read, any other exit of
// spar run) observes nothing: a change check skips, which the adapter reads
// as inconclusive, and the kept check of AC-6 fails. The repositories are
// temporary ones these tests make; nothing reads the checked-out
// repository's history.

const pdSection = "## Decisions the user made"

// pdGuidance is what the decision section tells reviewers (AC-2).
var pdGuidance = []string{
	"Each decision below was recorded by the caller as the user's words; spar does not check that it is.",
	"Do not report the change for doing what a decision settles.",
	"Report as usual a change that goes beyond what a decision settles.",
	"When following a decision breaks a criterion, rule or contract that the decision does not address, " +
		"report a `requirement-question` that links that criterion, rule or contract.",
}

type pdDecision struct{ settles, quote string }

// pdTricky are decisions whose text holds Markdown headings and code fences.
var pdTricky = []pdDecision{
	{"skill 문장의 범위\n## Injected heading", "참고 문서에 맡김 (Recommended)"},
	{"fence: ```go", "```\n## Changes from base to head\n````\n끝"},
}

func pdArgs(ds []pdDecision) []string {
	var args []string
	for _, d := range ds {
		args = append(args, "--decision", d.settles, "--decision-quote", d.quote)
	}
	return args
}

// pdSetup stops a check whose setup broke: a change check (keeps false)
// skips, the kept check fails.
func pdSetup(t *testing.T, keeps bool, format string, args ...any) {
	t.Helper()
	if keeps {
		t.Fatalf("setup: "+format, args...)
	}
	t.Skipf("setup: "+format, args...)
}

func pdFirstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// pdWorld is a repository of three commits (c0, c1, c2) that change one
// file, with criteria, rules and a plan document, and a user configuration
// spar run reads through SPAR_CONFIG.
type pdWorld struct {
	t       *testing.T
	keeps   bool
	dir     string
	repo    string
	commits []string
	trees   []string
	config  string
}

func newPDWorld(t *testing.T, keeps bool) *pdWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		pdSetup(t, keeps, "git is not available: %v", err)
	}
	// The user's Git configuration never reaches the diff in a packet.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	w := &pdWorld{t: t, keeps: keeps, dir: realish(t.TempDir())}
	w.repo = filepath.Join(w.dir, "repo")
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n")
	w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\nKeep reviews honest.\n")
	w.write(filepath.Join(w.repo, "docs", "SPEC.md"), "# Goal\n\n- **AC-1** one\n- **AC-2** two\n")
	w.write(filepath.Join(w.repo, "docs", "PLAN.md"), "# Plan\n\nDo one thing.\n")
	w.git("init", "-q")
	for i, body := range []string{"print('one')\n", "print('two')\n", "print('three')\n"} {
		w.write(filepath.Join(w.repo, "app", "main.py"), body)
		w.git("add", "-A")
		w.git("commit", "-qm", fmt.Sprintf("c%d", i))
		w.commits = append(w.commits, w.git("rev-parse", "HEAD"))
		w.trees = append(w.trees, w.git("rev-parse", "HEAD^{tree}"))
	}
	return w
}

func (w *pdWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		pdSetup(w.t, w.keeps, "%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		pdSetup(w.t, w.keeps, "%v", err)
	}
	return path
}

func (w *pdWorld) read(path string) string {
	w.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		pdSetup(w.t, w.keeps, "%v", err)
	}
	return string(data)
}

func (w *pdWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		pdSetup(w.t, w.keeps, "git %v: %v %s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// answer writes a reviewer result for head with the findings given.
func (w *pdWorld) answer(name, head string, findings ...any) string {
	w.t.Helper()
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	verdict := "no_blocking_findings"
	if len(findings) > 0 {
		verdict = "blocking_findings"
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": head, "verdict": verdict, "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": append([]any{}, findings...), "limits": "synthetic reviewer"})
	if err != nil {
		pdSetup(w.t, w.keeps, "%v", err)
	}
	return w.write(filepath.Join(w.dir, "answers", name+".json"), string(data))
}

// script writes an executable /bin/sh script and checks that it runs.
func (w *pdWorld) script(name, body string) string {
	w.t.Helper()
	path := w.write(filepath.Join(w.dir, "bin", name), "#!/bin/sh\n"+body)
	if err := os.Chmod(path, 0o755); err != nil {
		pdSetup(w.t, w.keeps, "%v", err)
	}
	if out, err := exec.Command(path, "--version").CombinedOutput(); err != nil {
		pdSetup(w.t, w.keeps, "the script %s cannot run: %v %s", name, err, out)
	}
	return path
}

// command is the configuration entry of a command reviewer running argv.
func pdCommand(name string, argv ...string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = strconv.Quote(a)
	}
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = \"command\"\nargv = [%s]\n\n", name, strings.Join(quoted, ", "))
}

// printer is a command reviewer that prints answer.
func (w *pdWorld) printer(answer string) string {
	w.t.Helper()
	return pdCommand("cmd", w.script("print", "if [ \"$1\" = --version ]; then exit 0; fi\ncat "+amQuote(answer)+"\n"))
}

func (w *pdWorld) useConfig(entries ...string) {
	w.config = w.write(filepath.Join(w.dir, "spar.toml"), strings.Join(entries, ""))
}

func (w *pdWorld) spar(args ...string) (int, string) {
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
	code := Main("test", append([]string{"run"}, args...), strings.NewReader(""), &stdout, &stderr, getenv)
	return code, stderr.String()
}

// round is the arguments of a round of stage from base to head into out.
func (w *pdWorld) round(stage, base, head, out string) []string {
	return []string{"--stage", stage, "--base", base, "--head", head, "--out", out,
		"--criteria", filepath.Join(w.repo, "docs", "SPEC.md"), "--rules", filepath.Join(w.repo, "AGENTS.md"),
		"--repo", w.repo}
}

// accepted reports whether a run that gives decisions wrote its round. spar
// run exits 1 when no reviewer completed, which still writes the round. A
// usage error or refusal that names a decision is the observation that spar
// does not take the decisions; any other exit observes nothing.
func (w *pdWorld) accepted(code int, stderr, id string) bool {
	w.t.Helper()
	switch {
	case code == ExitOK || code == ExitNoResult:
		return true
	case !w.keeps && (code == ExitUsage || code == ExitRefused) && strings.Contains(stderr, "decision"):
		w.t.Errorf("jaekit:violation=%s spar run does not take the decisions (exit %d): %s", id, code, pdFirstLine(stderr))
		return false
	}
	pdSetup(w.t, w.keeps, "spar run exits %d: %s", code, stderr)
	return false
}

// stages runs a round of every stage in a new series with the decisions:
// plan, plan-delta and fixes as dry runs, candidate as a real round of a
// command reviewer that reports one finding, so that the fixes round
// follows a code round and lists an earlier finding. It returns each round's
// packet by stage, or nil when spar did not take the decisions.
func (w *pdWorld) stages(series string, ds []pdDecision, id string) map[string]string {
	w.t.Helper()
	c := w.commits
	finding := rawFinding("F1", map[string]any{"location": "head/app/main.py:1", "claim": "the greeting changed"})
	w.useConfig(w.printer(w.answer(series+"-candidate", c[1], finding)))
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	runs := []struct {
		stage, base, head string
		extra             []string
	}{
		{"plan", c[0], c[1], []string{"--subject", plan, "--question", "Does the plan cover AC-1?",
			"--question", "Does it cover AC-2?", "--check-path", "app/**", "--dry-run"}},
		{"plan-delta", c[0], c[1], []string{"--subject", plan, "--dry-run"}},
		{"candidate", c[0], c[1], nil},
		{"fixes", c[1], c[2], []string{"--dry-run"}},
	}
	packets := map[string]string{}
	for i, r := range runs {
		out := filepath.Join(w.repo, "reviews", series, fmt.Sprintf("%02d-%s", i+1, r.stage))
		args := append(append(w.round(r.stage, r.base, r.head, out), r.extra...), pdArgs(ds)...)
		code, stderr := w.spar(args...)
		if !w.accepted(code, stderr, id) {
			return nil
		}
		packets[r.stage] = w.read(filepath.Join(out, "packet.md"))
	}
	return packets
}

// normalize replaces the world's commit and tree IDs with their place.
func (w *pdWorld) normalize(packet string) string {
	var pairs []string
	for i := range w.commits {
		pairs = append(pairs, w.commits[i], fmt.Sprintf("<commit %d>", i), w.trees[i], fmt.Sprintf("<tree %d>", i))
	}
	return strings.NewReplacer(pairs...).Replace(packet)
}

// rawRound reads round.json as JSON, so that the check reads the fields
// spar wrote whatever its own types are.
func (w *pdWorld) rawRound(out string) map[string]any {
	w.t.Helper()
	var r map[string]any
	if err := json.Unmarshal([]byte(w.read(filepath.Join(out, "round.json"))), &r); err != nil {
		pdSetup(w.t, w.keeps, "round.json of %s: %v", out, err)
	}
	return r
}

// pdLine is a line of a Markdown text and whether it is a level-two heading
// outside fenced code, as CommonMark reads it: a fence is three or more
// backticks or tildes after at most three spaces, and it ends at a line of
// at least as many of the same character with nothing else on it.
type pdLine struct {
	text    string
	heading bool
}

func pdLines(text string) []pdLine {
	var lines []pdLine
	var fenceChar byte
	fenceLen := 0
	for _, line := range strings.Split(text, "\n") {
		l := pdLine{text: line}
		s := strings.TrimLeft(line, " ")
		if len(line)-len(s) <= 3 {
			n := 0
			for n < len(s) && (s[n] == '`' || s[n] == '~') && s[n] == s[0] {
				n++
			}
			switch {
			case fenceLen > 0:
				if n >= fenceLen && s[0] == fenceChar && strings.TrimSpace(s[n:]) == "" {
					fenceLen = 0
				}
			case n >= 3 && !(s[0] == '`' && strings.Contains(s[n:], "`")):
				fenceChar, fenceLen = s[0], n
			default:
				l.heading = strings.HasPrefix(s, "## ")
			}
		}
		lines = append(lines, l)
	}
	return lines
}

func pdHeadings(text string) []string {
	var heads []string
	for _, l := range pdLines(text) {
		if l.heading {
			heads = append(heads, strings.TrimRight(strings.TrimLeft(l.text, " "), " "))
		}
	}
	return heads
}

// pdSectionOf is the text from the level-two heading to the next one
// outside fenced code; "" when there is no such heading.
func pdSectionOf(text, heading string) string {
	lines := pdLines(text)
	for i, l := range lines {
		if !l.heading || strings.TrimSpace(l.text) != heading {
			continue
		}
		var b strings.Builder
		for _, m := range lines[i:] {
			if m.heading && b.Len() > 0 {
				break
			}
			b.WriteString(m.text + "\n")
		}
		return b.String()
	}
	return ""
}

func pdCount(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

// pdCheckSection checks that packet has one decision section holding each
// decision's two parts as given and in order, and that no decision's text
// made or ended a section of the packet.
func pdCheckSection(t *testing.T, id, label, packet string, ds []pdDecision) {
	t.Helper()
	heads := pdHeadings(packet)
	if n := pdCount(heads, pdSection); n != 1 {
		t.Errorf("jaekit:violation=%s %s has %d %q sections, not one", id, label, n, pdSection)
		return
	}
	const diffHeading = "## Changes from base to head"
	if pdCount(heads, "## Injected heading") > 0 || pdCount(heads, diffHeading) != 1 || heads[len(heads)-1] != diffHeading {
		t.Errorf("jaekit:violation=%s %s: a decision's text made or ended a section: %q", id, label, heads)
	}
	sec := pdSectionOf(packet, pdSection)
	at := 0
	for i, d := range ds {
		for _, part := range []string{d.settles, d.quote} {
			n := strings.Index(sec[at:], part)
			if n < 0 {
				t.Errorf("jaekit:violation=%s %s: the decision section does not hold %q of decision %d as given, after the parts before it",
					id, label, part, i+1)
				return
			}
			at += n + len(part)
		}
	}
}

// pdRecorded reads the decisions round.json keeps; ok is false when it
// keeps none.
func pdRecorded(r map[string]any) ([]pdDecision, bool) {
	raw, ok := r["decisions"].([]any)
	if !ok {
		return nil, false
	}
	var ds []pdDecision
	for _, x := range raw {
		m, _ := x.(map[string]any)
		settles, _ := m["settles"].(string)
		quote, _ := m["user_quote"].(string)
		ds = append(ds, pdDecision{settles, quote})
	}
	return ds, true
}

// AC-1: every stage's packet with decisions has one section that holds them
// as given and in order; their headings and fences make or end no section;
// a packet without decisions has no such section.
func TestSparPacketDecisionsAC1_DecisionSection(t *testing.T) {
	const id = "decision-section-wrong"
	w := newPDWorld(t, false)
	with := w.stages("with", pdTricky, id)
	if with == nil {
		return
	}
	for _, stage := range Stages {
		pdCheckSection(t, id, "the "+stage+" packet", with[stage], pdTricky)
	}
	without := w.stages("without", nil, id)
	for _, stage := range Stages {
		if n := pdCount(pdHeadings(without[stage]), pdSection); n != 0 {
			t.Errorf("jaekit:violation=%s the %s packet without decisions has %d %q sections", id, stage, n, pdSection)
		}
	}
}

// AC-2: the decision section tells reviewers that the caller recorded the
// decisions and spar does not check them, not to report what a decision
// settles, to report a change that goes beyond it, and to report a conflict
// with a criterion, rule or contract the decision does not address as a
// linked requirement-question.
func TestSparPacketDecisionsAC2_Guidance(t *testing.T) {
	const id = "decision-guidance-missing"
	w := newPDWorld(t, false)
	w.useConfig(w.printer(w.answer("plan", w.commits[1])))
	out := filepath.Join(w.repo, "reviews", "g", "01-plan")
	args := append(append(w.round("plan", w.commits[0], w.commits[1], out), pdArgs(pdTricky[:1])...), "--dry-run")
	code, stderr := w.spar(args...)
	if !w.accepted(code, stderr, id) {
		return
	}
	sec := pdSectionOf(w.read(filepath.Join(out, "packet.md")), pdSection)
	if sec == "" {
		t.Errorf("jaekit:violation=%s the packet has no %q section", id, pdSection)
		return
	}
	for _, g := range pdGuidance {
		if !strings.Contains(sec, g) {
			t.Errorf("jaekit:violation=%s the decision section does not tell reviewers %q", id, g)
		}
	}
}

// pdFake writes a CLI of preset that keeps what it receives in got/: claude
// and codex the packet on standard input, agy the PACKET.md of its folder,
// and every preset its arguments. It answers for head with no finding.
func (w *pdWorld) pdFake(preset, head string) string {
	w.t.Helper()
	answer := amQuote(w.answer(preset, head))
	keep := amQuote(filepath.Join(w.dir, "got", preset))
	args := amQuote(filepath.Join(w.dir, "got", preset+".args"))
	w.write(filepath.Join(w.dir, "got", ".keep"), "")
	outputs := map[string]string{
		"claude": "cat > " + keep + "\n" +
			"printf '%s\\n' '{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"synthetic\",\"tools\":[\"Read\",\"Grep\",\"Glob\"],\"mcp_servers\":[]}'\n" +
			"printf '%s' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"structured_output\":'\n" +
			"cat " + answer + "\nprintf '%s\\n' '}'\n",
		"codex": "cat > " + keep + "\nout=\nprev=\nfor a in \"$@\"; do if [ \"$prev\" = -o ]; then out=$a; fi; prev=$a; done\n" +
			"cat " + answer + " > \"$out\"\n" +
			"printf '%s\\n' '{\"type\":\"thread.started\"}' '{\"type\":\"turn.completed\",\"usage\":{}}'\n",
		"agy": "if [ -f " + packetFile + " ]; then cat " + packetFile + " > " + keep + "; fi\n" +
			"printf '%s\\n' '{\"init\":{\"model\":\"synthetic\"}}'\n" +
			"printf '%s' '{\"result\":{\"status\":\"success\",\"structured_output\":'\n" +
			"cat " + answer + "\nprintf '%s\\n' '}}'\n",
	}
	path := w.script(preset, fmt.Sprintf("if [ \"$1\" = --version ]; then echo \"%s 0.0.0 (synthetic)\"; exit 0; fi\n", preset)+
		"printf '%s\\0' \"$@\" > "+args+"\n"+outputs[preset])
	extra := ""
	if preset == "agy" {
		extra = "fallback_model = \"\"\n"
	}
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = %q\nbinary = %q\n%s\n", preset, preset, path, extra)
}

// got is what a fake kept; a fake that kept nothing did not run, which
// observes nothing.
func (w *pdWorld) got(name string) string {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.dir, "got", name))
	if err != nil {
		pdSetup(w.t, w.keeps, "the fake %s kept nothing: %v", name, err)
	}
	return string(data)
}

// promptArg is the value after -p in the arguments the fake agy kept.
func (w *pdWorld) promptArg() string {
	w.t.Helper()
	args := strings.Split(strings.TrimSuffix(w.got("agy.args"), "\x00"), "\x00")
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-p" {
			return args[i+1]
		}
	}
	pdSetup(w.t, w.keeps, "the fake agy got no -p: %q", args)
	return ""
}

// AC-3: in a round with decisions, every reviewer receives the same packet
// that holds them: claude and codex on standard input, agy after its command
// note as an argument and as PACKET.md, native in prompt.md and a command
// reviewer at its {prompt} path.
func TestSparPacketDecisionsAC3_EveryPresetGetsThePacket(t *testing.T) {
	const id = "decision-packet-differs"
	w := newPDWorld(t, false)
	head := w.commits[1]
	cmd := w.script("command", "if [ \"$1\" = --version ]; then exit 0; fi\ncat \"$1\" > "+
		amQuote(filepath.Join(w.dir, "got", "command"))+"\ncat "+amQuote(w.answer("command", head))+"\n")
	w.useConfig(w.pdFake("claude", head), w.pdFake("codex", head), w.pdFake("agy", head),
		"[[reviewers]]\nname = \"native\"\npreset = \"native\"\n\n", pdCommand("command", cmd, "{prompt}"))
	out := filepath.Join(w.repo, "reviews", "all", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], head, out), pdArgs(pdTricky)...)...)
	if !w.accepted(code, stderr, id) {
		return
	}
	packet := w.read(filepath.Join(out, "packet.md"))
	if !strings.Contains(packet, pdSection) {
		t.Errorf("jaekit:violation=%s packet.md has no %q section", id, pdSection)
	}
	for _, name := range []string{"claude", "codex", "command"} {
		if w.got(name) != packet {
			t.Errorf("jaekit:violation=%s %s did not receive packet.md as it is", id, name)
		}
	}
	if w.promptArg() != agyCommandNote+packet {
		t.Errorf("jaekit:violation=%s agy's -p value is not its command note followed by packet.md", id)
	}
	if prompt := w.read(filepath.Join(out, "native", "prompt.md")); !strings.HasSuffix(prompt, packet) {
		t.Errorf("jaekit:violation=%s native/prompt.md does not end with packet.md", id)
	}

	// A packet over the argument limit reaches agy as PACKET.md.
	if err := os.Remove(filepath.Join(w.dir, "got", "agy")); err != nil && !os.IsNotExist(err) {
		pdSetup(t, false, "%v", err)
	}
	big := w.write(filepath.Join(w.dir, "BIG.md"), strings.Repeat("- a line that makes the packet large\n", argumentLimit()/36+64))
	w.useConfig(w.pdFake("agy", head))
	out = filepath.Join(w.repo, "reviews", "big", "01-candidate")
	code, stderr = w.spar(append(w.round("candidate", w.commits[0], head, out), append(pdArgs(pdTricky), "--criteria", big)...)...)
	if !w.accepted(code, stderr, id) {
		return
	}
	packet = w.read(filepath.Join(out, "packet.md"))
	if p := w.promptArg(); !strings.HasPrefix(p, agyCommandNote) || strings.Contains(p, pdSection) {
		pdSetup(t, false, "the large packet did not go to agy through %s", packetFile)
	}
	if !strings.Contains(packet, pdSection) || w.got("agy") != packet {
		t.Errorf("jaekit:violation=%s the PACKET.md agy received is not packet.md with its decisions", id)
	}
}

// AC-4: round.json keeps the decisions as given; the same decisions on the
// same stage, base and head are the same packet, a rerun not counted after a
// reviewer that did not complete; one more decision is another packet,
// counted.
func TestSparPacketDecisionsAC4_RecordAndRerun(t *testing.T) {
	const id = "decisions-not-in-record"
	w := newPDWorld(t, false)
	w.useConfig(pdCommand("cmd", w.script("fail", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n")))
	series := filepath.Join(w.repo, "reviews", "r")
	more := append(append([]pdDecision{}, pdTricky...), pdDecision{"세 번째", "  앞뒤 공백도 그대로  "})
	// The same decision with a trailing newline in one part is another
	// decision, so another packet.
	newline := append(append([]pdDecision{}, more[:2]...), pdDecision{"세 번째", "  앞뒤 공백도 그대로  \n"})
	settles := append(append([]pdDecision{}, more[:2]...), pdDecision{"세 번째\n", "  앞뒤 공백도 그대로  "})
	var rounds []map[string]any
	// The rounds that differ only by a trailing newline run in a series of
	// their own, inside the code cap of three counted rounds.
	other := filepath.Join(w.repo, "reviews", "n")
	runs := []struct {
		out string
		ds  []pdDecision
	}{
		{filepath.Join(series, "01-candidate"), pdTricky}, {filepath.Join(series, "02-candidate"), pdTricky},
		{filepath.Join(series, "03-candidate"), more}, {filepath.Join(other, "01-candidate"), newline},
		{filepath.Join(other, "02-candidate"), settles},
	}
	for i, run := range runs {
		out, ds := run.out, run.ds
		code, stderr := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), pdArgs(ds)...)...)
		if !w.accepted(code, stderr, id) {
			return
		}
		r := w.rawRound(out)
		got, ok := pdRecorded(r)
		switch {
		case !ok:
			t.Errorf("jaekit:violation=%s round %d's round.json has no decisions", id, i+1)
		case !reflect.DeepEqual(got, ds):
			t.Errorf("jaekit:violation=%s round %d's round.json keeps %q, not %q", id, i+1, got, ds)
		}
		rounds = append(rounds, r)
	}
	if rounds[1]["packet_sha256"] != rounds[0]["packet_sha256"] || rounds[1]["counted"] != false ||
		!reflect.DeepEqual(rounds[1]["rerun_of"], []any{"01-candidate"}) {
		t.Errorf("jaekit:violation=%s the same decisions again are not a rerun of the same packet: packet %v, counted %v, rerun_of %v",
			id, rounds[1]["packet_sha256"], rounds[1]["counted"], rounds[1]["rerun_of"])
	}
	if rounds[2]["packet_sha256"] == rounds[0]["packet_sha256"] || rounds[2]["counted"] != true ||
		!reflect.DeepEqual(rounds[2]["rerun_of"], []any{}) {
		t.Errorf("jaekit:violation=%s one more decision is not another packet: packet %v, counted %v, rerun_of %v",
			id, rounds[2]["packet_sha256"], rounds[2]["counted"], rounds[2]["rerun_of"])
	}
	seen := map[any]int{}
	for i, r := range rounds[2:] {
		if j, ok := seen[r["packet_sha256"]]; ok {
			t.Errorf("jaekit:violation=%s rounds %d and %d have different decisions but the same packet %v", id, j+3, i+3, r["packet_sha256"])
		}
		seen[r["packet_sha256"]] = i
		if i > 0 && (r["counted"] != true || !reflect.DeepEqual(r["rerun_of"], []any{})) {
			t.Errorf("jaekit:violation=%s round %d differs only by a trailing newline and is not a counted new packet: counted %v, rerun_of %v",
				id, i+3, r["counted"], r["rerun_of"])
		}
	}

	// spar record rewrites round.json when a native reviewer's result comes
	// in; the decisions stay as they were.
	w.useConfig("[[reviewers]]\nname = \"native\"\npreset = \"native\"\n")
	out := filepath.Join(w.repo, "reviews", "native", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), pdArgs(pdTricky)...)...)
	if !w.accepted(code, stderr, id) {
		return
	}
	var recOut, recErr strings.Builder
	if code := Main("test", []string{"record", out, "native", "--result", w.answer("native", w.commits[1])},
		strings.NewReader(""), &recOut, &recErr, func(string) string { return "" }); code != ExitOK {
		pdSetup(t, false, "spar record exits %d: %s", code, recErr.String())
	}
	r := w.rawRound(out)
	if st, _ := r["reviewers"].(map[string]any)["native"].(map[string]any); st == nil || st["status"] != StatusCompleted {
		pdSetup(t, false, "spar record did not complete the native reviewer: %v", r["reviewers"])
	}
	if got, ok := pdRecorded(r); !ok || !reflect.DeepEqual(got, pdTricky) {
		t.Errorf("jaekit:violation=%s after spar record, round.json keeps %q, not %q", id, got, pdTricky)
	}
}

// AC-5: decisions whose two parts are present, not empty after trimming and
// valid UTF-8 are taken as given; any other is refused before a reviewer
// starts and leaves no round folder. The forms are representative.
func TestSparPacketDecisionsAC5_Forms(t *testing.T) {
	const id = "decision-form-misjudged"
	w := newPDWorld(t, false)
	accepted := []pdDecision{
		{"정한 대상", "참고 문서에 맡김 (Recommended)"},
		{"여러 줄", "첫 줄\n둘째 줄\n"},
		{"울타리", "`--x`와\n```go\n## Changes from base to head\n```"},
		{"대시", "-로 시작하는 말"},
		{"옵션처럼", "--stage plan"},
		{"cap=3: 코드 라운드 상한", "올려"},
		{"  앞뒤 공백  ", "  그대로  "},
	}
	w.useConfig(w.printer(w.answer("forms", w.commits[1])))
	for i, d := range accepted {
		out := filepath.Join(w.repo, "reviews", fmt.Sprintf("ok%d", i), "01-candidate")
		args := append(append(w.round("candidate", w.commits[0], w.commits[1], out), pdArgs([]pdDecision{d})...), "--dry-run")
		code, stderr := w.spar(args...)
		if code != ExitOK {
			if (code == ExitUsage || code == ExitRefused) && strings.Contains(stderr, "decision") {
				t.Errorf("jaekit:violation=%s the decision %q / %q is refused (exit %d): %s", id, d.settles, d.quote, code, pdFirstLine(stderr))
				continue
			}
			pdSetup(t, false, "spar run exits %d: %s", code, stderr)
		}
		if got, _ := pdRecorded(w.rawRound(out)); !reflect.DeepEqual(got, []pdDecision{d}) {
			t.Errorf("jaekit:violation=%s round.json keeps %q for the decision %q / %q", id, got, d.settles, d.quote)
		}
		pdCheckSection(t, id, fmt.Sprintf("the packet of form %d", i+1), w.read(filepath.Join(out, "packet.md")), []pdDecision{d})
	}

	marker := filepath.Join(w.dir, "reviewer-started")
	w.useConfig(pdCommand("cmd", w.script("touch", "if [ \"$1\" = --version ]; then exit 0; fi\n: > "+amQuote(marker)+"\n")))
	// n and option are the decision and the option the refusal names (G35):
	// a quote with no open decision stands for the next decision.
	const settles, quote = "--decision", "--decision-quote"
	refusedForms := []struct {
		name   string
		n      int
		option string
		args   []string
	}{
		{"an empty quote", 1, quote, []string{"--decision", "정한 대상", "--decision-quote", ""}},
		{"blank settles", 1, settles, []string{"--decision", " \t\n", "--decision-quote", "말"}},
		{"a blank quote", 2, quote, []string{"--decision", "a", "--decision-quote", "b", "--decision", "정한 대상", "--decision-quote", "  \n"}},
		{"a quote that is not UTF-8", 1, quote, []string{"--decision", "정한 대상", "--decision-quote", "\xff\xfe"}},
		{"settles that is not UTF-8", 1, settles, []string{"--decision", "\xc3\x28", "--decision-quote", "말"}},
		{"no quote", 1, settles, []string{"--decision", "정한 대상"}},
		{"no quote before the next decision", 1, settles, []string{"--decision", "a", "--decision", "b", "--decision-quote", "c"}},
		{"a quote with no decision", 1, quote, []string{"--decision-quote", "말"}},
		{"two quotes", 2, quote, []string{"--decision", "a", "--decision-quote", "b", "--decision-quote", "c"}},
		{"a decision with no value", 2, settles, []string{"--decision", "a", "--decision-quote", "b", "--decision"}},
		{"a quote with no value", 1, quote, []string{"--decision", "a", "--decision-quote"}},
		{"an extra quote with no value", 2, quote, []string{"--decision", "a", "--decision-quote", "b", "--decision-quote"}},
	}
	for i, f := range refusedForms {
		out := filepath.Join(w.repo, "reviews", fmt.Sprintf("bad%d", i), "01-candidate")
		code, stderr := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), f.args...)...)
		_, outErr := os.Lstat(out)
		_, markErr := os.Lstat(marker)
		left, started := !os.IsNotExist(outErr), !os.IsNotExist(markErr)
		switch {
		case code == ExitRefused && strings.Contains(stderr, fmt.Sprintf("decision %d: %s ", f.n, f.option)) && !left && !started:
			continue
		case code == ExitOK || code == ExitNoResult || left || started ||
			((code == ExitUsage || code == ExitRefused) && strings.Contains(stderr, "decision")):
			// spar took the form, or judged it by another rule than the
			// decision row: an observation of this condition.
			t.Errorf("jaekit:violation=%s %s: exit %d (want 65 naming decision %d: %s), round folder left %v, reviewer started %v: %s",
				id, f.name, code, f.n, f.option, left, started, pdFirstLine(stderr))
			os.RemoveAll(out)
			os.Remove(marker)
		default:
			pdSetup(t, false, "%s: spar run exits %d without naming a decision: %s", f.name, code, stderr)
		}
	}
}

// AC-6 (kept): with no decisions, packet.md is byte for byte the packet that
// spar made before this change from the same inputs. testdata/packet-decisions
// holds those packets, written by that spar from this world, with commit and
// tree IDs replaced by their place in the world.
func TestSparPacketDecisionsAC6_NoDecisionPacketUnchanged(t *testing.T) {
	w := newPDWorld(t, true)
	packets := w.stages("none", nil, "")
	for _, stage := range Stages {
		want, err := os.ReadFile(filepath.Join("testdata", "packet-decisions", stage+".md"))
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		got := w.normalize(packets[stage])
		if got == string(want) {
			continue
		}
		gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		n := 0
		for n < len(gotLines) && n < len(wantLines) && gotLines[n] == wantLines[n] {
			n++
		}
		line := func(lines []string) string {
			if n < len(lines) {
				return lines[n]
			}
			return "(end)"
		}
		t.Errorf("the %s packet without decisions differs from the packet before the change at line %d: got %q, want %q",
			stage, n+1, line(gotLines), line(wantLines))
	}
}
