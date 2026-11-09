package plugins

// Checks of the goal core-completion-integrity for Seal and the documents.
// AC-9: Seal starts new goals under run-rules/6, declares that rule in
// core-requirements.json, refuses a Core without it through its helper,
// resumes a goal under its saved rule, and raises its version with a line in
// the version list. AC-12: the contracts, guides and Seal's own documents
// describe run-rules/6 in Korean and English.
//
// Each test reads every file it needs first. A file that is missing or
// cannot be read or decoded, a tool that cannot start (go, git, python3, the
// built ha) and a synthetic repository that cannot be made are a broken
// setup, never the observed property. The opening line of an assertion's
// failure message names the property it observes (jaekit:violation=<id>).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	cpSealVersion  = "0.1.26"
	cpNewRules     = "run-rules/6"
	cpOldRules     = "run-rules/5"
	cpSkillDir     = "seal/skills/seal"
	cpSkill        = cpSkillDir + "/SKILL.md"
	cpRequirements = cpSkillDir + "/core-requirements.json"
	cpHaRef        = cpSkillDir + "/references/ha.md"
	cpHelper       = cpSkillDir + "/scripts/check_core.py"
	cpGoal         = "docs/specs/demo"
)

// The formats and features Seal declared under run-rules/5, which the
// run-rules/6 declaration keeps.
var (
	cpOldFormats = map[string][]string{
		"criteria":          {"legacy", "nested/1"},
		"check_declaration": {"check-declaration/v1"},
		"check_result":      {"check-result/v1"},
		"check_evidence":    {"check-evidence/v1"},
		"assurance_tier":    {"light", "standard", "strict"},
		"core_requirements": {"core-requirements/v1"},
		"review_gate":       {"review-gate/v1"},
	}
	cpOldFeatures = []string{"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1",
		"budget-change/v1", "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1",
		"command-time-limits/v1", "report/v1", "doctor/v1", "auto-budget/v1", "core-requirements/v1", "review-gate/v1"}
)

// cpEntry finds the Seal version's entry in the version list.
var cpEntry = regexp.MustCompile(`Seal ` + regexp.QuoteMeta(cpSealVersion) + `\b`)

// cpRead reads the files a test needs; any that cannot be read is a broken
// setup.
func cpRead(t *testing.T, paths ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		out[p] = string(data)
	}
	return out
}

func cpDecode(t *testing.T, path, text string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), value); err != nil {
		t.Fatalf("setup: %s is not JSON: %v", path, err)
	}
}

// cpLevel is the heading level of a Markdown line, or 0.
func cpLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// cpFence returns the open fence marker after line ("" outside fenced code)
// and whether line is fenced code or a fence line.
func cpFence(open, line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, m) {
			if open == "" {
				return m, true
			}
			if open == m {
				return "", true
			}
		}
	}
	return open, open != ""
}

// cpSection returns the first section whose heading text (without the
// leading #s) matches, up to the next heading of the same or a higher level
// outside fenced code.
func cpSection(text string, match func(heading string) bool) (string, bool) {
	lines := strings.Split(text, "\n")
	start, level, fence := -1, 0, ""
	for i, l := range lines {
		var fenced bool
		if fence, fenced = cpFence(fence, l); fenced {
			continue
		}
		lv := cpLevel(l)
		if lv == 0 {
			continue
		}
		if start >= 0 && lv <= level {
			return strings.Join(lines[start:i], "\n"), true
		}
		if start < 0 && match(strings.TrimSpace(strings.TrimRight(l[lv:], " \t\r"))) {
			start, level = i, lv
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

func cpHeading(want string) func(string) bool {
	return func(h string) bool { return h == want }
}

func cpNumbered(number string) func(string) bool {
	return func(h string) bool { return strings.HasPrefix(h, number+" ") }
}

// cpMissing lists the tokens text does not contain, each named with where.
func cpMissing(where, text string, tokens ...string) []string {
	var out []string
	for _, tok := range tokens {
		if !strings.Contains(text, tok) {
			out = append(out, fmt.Sprintf("%s: %q", where, tok))
		}
	}
	return out
}

// cpRun runs a command. A command that cannot start or that a signal ended
// is a broken setup; otherwise it returns the combined output and exit code.
func cpRun(t *testing.T, dir string, env []string, name string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return string(out), exit.ExitCode()
	}
	t.Fatalf("setup: %s %v: %v: %s", name, args, err, out)
	return "", 0
}

// cpMust runs a setup command that must succeed.
func cpMust(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	out, code := cpRun(t, dir, env, name, args...)
	if code != 0 {
		t.Fatalf("setup: %s %v exit %d: %s", name, args, code, out)
	}
	return out
}

func cpEnv(t *testing.T) []string {
	return append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
}

func cpWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// cpRepo makes a committed repository outside the tree with one synthetic
// goal whose only condition runs git.
func cpRepo(t *testing.T, env []string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"src/value.txt": "ok\n",
		cpGoal + "/SPEC.md": "# Demo\n\nStatus: Ready\n\n## 목표\n\n`src/value.txt` says ok.\n\n## 범위와 비목표\n\n- 범위: `src/**`\n\n" +
			"## Acceptance Criteria\n- **AC-1** `git --version` exits 0.\n\n## Open Decisions\n\n없음\n",
		cpGoal + "/PLAN.md": "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
			"| AC-1 | maintain | `git --version` | — | T001 |\n\n## 범위\n- 바꿀 수 있는 경로: `src/**` (SPEC: 범위와 비목표)\n" +
			"- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n" +
			"| --- | --- | --- | --- | --- | --- | --- |\n| T001 | git runs | AC-1 | — | — | 예 | — |\n\n" +
			"## 예산\n- 검증 실행: 10\n- 경과 시간: 1시간\n- 비용: 미관측\n",
		cpGoal + "/REVIEW.md":   "# REVIEW — demo\n\n## 결론\nready\n",
		cpGoal + "/PROGRESS.md": "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | |\n",
	} {
		cpWrite(t, filepath.Join(dir, filepath.FromSlash(name)), body, 0o644)
	}
	cpMust(t, dir, env, "git", "init", "-q")
	cpMust(t, dir, env, "git", "add", "-A")
	cpMust(t, dir, env, "git", "-c", "commit.gpgsign=false", "commit", "-qm", "base")
	return dir
}

// cpStartRules is the rule of a goal's start line.
func cpStartRules(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(cpGoal), "runs.jsonl"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var start struct {
		Kind  string `json:"kind"`
		Rules string `json:"rules"`
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if err := json.Unmarshal([]byte(first), &start); err != nil || start.Kind != "start" {
		t.Fatalf("setup: the record's first line is not a start line: %v: %s", err, first)
	}
	return start.Rules
}

// cpReport is what check_core.py prints.
type cpReport struct {
	Compatible bool     `json:"compatible"`
	Reason     string   `json:"reason"`
	Rules      *string  `json:"rules"`
	Missing    []string `json:"missing"`
}

// cpHelperRun runs Seal's compatibility helper. An exit other than 0
// (compatible) and 2 (not), output that is not its report, and a Core it
// could not query are a broken setup.
func cpHelperRun(t *testing.T, dir string, env []string, args ...string) (cpReport, int) {
	t.Helper()
	helper, err := filepath.Abs(cpHelper)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	cmd := exec.Command("python3", append([]string{"-I", helper}, args...)...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.Output()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("setup: python3 %s %v: %v", cpHelper, args, err)
	}
	var r cpReport
	if code != 0 && code != 2 {
		t.Fatalf("setup: check_core.py %v exit %d: %s", args, code, out)
	}
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("setup: check_core.py %v printed no report: %v: %s", args, err, out)
	}
	switch r.Reason {
	case "core_missing", "query_timeout", "query_failed", "query_unsupported", "binary_changed", "goal_unreadable":
		t.Fatalf("setup: check_core.py %v could not read its input: %s", args, r.Reason)
	}
	return r, code
}

func cpRulesOf(r cpReport) string {
	if r.Rules == nil {
		return "(none)"
	}
	return *r.Rules
}

// AC-9: Seal starts new goals under run-rules/6.
func TestCoreIntegrityAC9_SealStartsUnderRulesV6(t *testing.T) {
	const v = "seal-not-on-rules-v6"
	const (
		claude      = "seal/.claude-plugin/plugin.json"
		codex       = "seal/.codex-plugin/plugin.json"
		marketplace = "../.claude-plugin/marketplace.json"
	)
	files := cpRead(t, cpRequirements, cpSkill, cpHaRef, cpHelper, claude, codex, marketplace)
	var decl struct {
		Rules    []string            `json:"rules"`
		Formats  map[string][]string `json:"formats"`
		Features []string            `json:"features"`
	}
	cpDecode(t, cpRequirements, files[cpRequirements], &decl)
	versions := map[string]string{}
	for _, p := range []string{claude, codex} {
		var manifest struct {
			Version string `json:"version"`
		}
		cpDecode(t, p, files[p], &manifest)
		versions[p] = manifest.Version
	}
	var market struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	cpDecode(t, marketplace, files[marketplace], &market)
	versions[marketplace] = "(no seal entry)"
	for _, p := range market.Plugins {
		if p.Name == "seal" {
			versions[marketplace] = p.Version
		}
	}

	// Inputs the start and the helper read: this tree's ha, a copy of the
	// skill folder, a goal to start, a run-rules/5 goal, and a fake Core
	// that answers this ha's capabilities without run-rules/6.
	env := cpEnv(t)
	bin := t.TempDir()
	ha := filepath.Join(bin, "ha")
	cpMust(t, "..", os.Environ(), "go", "build", "-o", ha, "./cmd/ha")
	skillCopy := filepath.Join(t.TempDir(), "seal")
	if err := os.CopyFS(skillCopy, os.DirFS(cpSkillDir)); err != nil {
		t.Fatalf("setup: copying the skill folder: %v", err)
	}
	fresh, earlier := cpRepo(t, env), cpRepo(t, env)
	cpMust(t, earlier, env, ha, "start", cpGoal, "--rules", cpOldRules, "--request", "implement the synthetic goal")
	var caps map[string]any
	capsText := cpMust(t, bin, env, ha, "capabilities", "--format", "json")
	cpDecode(t, "ha capabilities --format json", capsText, &caps)
	supported, ok := caps["supported_rules"].([]any)
	if !ok {
		t.Fatalf("setup: ha capabilities has no supported_rules: %s", capsText)
	}
	var without []any
	for _, r := range supported {
		if r != cpNewRules {
			without = append(without, r)
		}
	}
	caps["supported_rules"] = without
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	cpWrite(t, filepath.Join(bin, "capabilities.json"), string(data)+"\n", 0o644)
	fake := filepath.Join(bin, "ha-without-rules-6")
	cpWrite(t, fake, "#!/bin/sh\nexec cat \"$(dirname \"$0\")/capabilities.json\"\n", 0o755)
	if out := cpMust(t, bin, env, fake, "capabilities", "--format", "json"); strings.TrimSpace(out) != string(data) {
		t.Fatalf("setup: the fake Core printed %q", out)
	}

	// core-requirements.json declares the new rule and keeps every format
	// and feature it declared under run-rules/5.
	if !slices.Equal(decl.Rules, []string{cpNewRules}) {
		t.Errorf("jaekit:violation=%s %s declares rules %v, want [%s]", v, cpRequirements, decl.Rules, cpNewRules)
	}
	var dropped []string
	for _, key := range []string{"criteria", "check_declaration", "check_result", "check_evidence", "assurance_tier", "core_requirements", "review_gate"} {
		for _, value := range cpOldFormats[key] {
			if !slices.Contains(decl.Formats[key], value) {
				dropped = append(dropped, "formats."+key+" "+value)
			}
		}
	}
	for _, feature := range cpOldFeatures {
		if !slices.Contains(decl.Features, feature) {
			dropped = append(dropped, "features "+feature)
		}
	}
	if len(dropped) > 0 {
		t.Errorf("jaekit:violation=%s %s no longer declares %s", v, cpRequirements, strings.Join(dropped, ", "))
	}

	// The skill's start line, version and version list.
	skill := files[cpSkill]
	if !strings.Contains(skill, "`ha start <goal> --rules "+cpNewRules+" --request") {
		t.Errorf("jaekit:violation=%s %s does not start with `ha start <goal> --rules %s --request`", v, cpSkill, cpNewRules)
	}
	front := ""
	if rest, ok := strings.CutPrefix(skill, "---\n"); ok {
		front, _, _ = strings.Cut(rest, "\n---")
	}
	if !slices.Contains(strings.Split(front, "\n"), `  version: "`+cpSealVersion+`"`) {
		t.Errorf("jaekit:violation=%s %s frontmatter does not say version %q", v, cpSkill, cpSealVersion)
	}
	for _, p := range []string{claude, codex, marketplace} {
		if versions[p] != cpSealVersion {
			t.Errorf("jaekit:violation=%s %s says Seal %s, want %s", v, p, versions[p], cpSealVersion)
		}
	}
	ref := files[cpHaRef]
	// The entry runs from "Seal 0.1.26" to the next Seal version or the end
	// of its paragraph.
	entry := ""
	if loc := cpEntry.FindStringIndex(ref); loc != nil {
		entry, _, _ = strings.Cut(ref[loc[1]:], "\n\n")
		entry, _, _ = strings.Cut(entry, "Seal 0.1.")
	}
	if !strings.Contains(entry, cpNewRules) {
		t.Errorf("jaekit:violation=%s %s has no Seal %s entry that names %s", v, cpHaRef, cpSealVersion, cpNewRules)
	}
	if _, ok := cpSection(ref, cpHeading(cpNewRules)); !ok {
		t.Errorf("jaekit:violation=%s %s has no `## %s` section", v, cpHaRef, cpNewRules)
	}

	// The skill's start, with its requirements checked, records run-rules/6.
	out, code := cpRun(t, fresh, env, ha, "start", cpGoal, "--rules", cpNewRules, "--request", "implement the synthetic goal",
		"--skill", filepath.Join(skillCopy, "SKILL.md"), "--check-skill")
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	switch {
	case code == 64 && strings.Contains(out, "--rules"):
		t.Errorf("jaekit:violation=%s ha start --rules %s --check-skill with the skill is refused: exit %d: %s", v, cpNewRules, code, first)
	case code == 65 && strings.Contains(out, "skill requirements"):
		t.Errorf("jaekit:violation=%s this tree's ha refuses the skill's requirements: exit %d: %s", v, code, first)
	case code != 0:
		t.Fatalf("setup: ha start --check-skill exit %d: %s", code, out)
	default:
		if got := cpStartRules(t, fresh); got != cpNewRules {
			t.Errorf("jaekit:violation=%s the skill's start recorded rules %q, want %q", v, got, cpNewRules)
		}
	}

	// Seal's helper starts with this tree's ha under run-rules/6, refuses a
	// Core without it, and resumes a run-rules/5 goal under its saved rule.
	if r, code := cpHelperRun(t, bin, env, "--ha", ha, "--operation", "start"); code != 0 || !r.Compatible || cpRulesOf(r) != cpNewRules {
		t.Errorf("jaekit:violation=%s check_core.py --operation start with this tree's ha: exit %d, compatible %v, rules %s, reason %s; want 0 with %s",
			v, code, r.Compatible, cpRulesOf(r), r.Reason, cpNewRules)
	}
	r, code := cpHelperRun(t, bin, env, "--ha", fake, "--operation", "start")
	named := slices.ContainsFunc(r.Missing, func(m string) bool { return strings.Contains(m, cpNewRules) })
	if code != 2 || r.Compatible || !named {
		t.Errorf("jaekit:violation=%s check_core.py --operation start with a Core without %s: exit %d, compatible %v, missing %v; want 2 naming %s",
			v, cpNewRules, code, r.Compatible, r.Missing, cpNewRules)
	}
	if r, code := cpHelperRun(t, earlier, env, "--ha", ha, "--operation", "resume", "--goal", cpGoal); code != 0 || !r.Compatible || cpRulesOf(r) != cpOldRules {
		t.Errorf("jaekit:violation=%s check_core.py --operation resume of a %s goal: exit %d, compatible %v, rules %s, reason %s, missing %v; want 0 under %s",
			v, cpOldRules, code, r.Compatible, cpRulesOf(r), r.Reason, r.Missing, cpOldRules)
	}
}

// AC-12: the documents describe run-rules/6, its four judgments, how it
// differs from run-rules/5 and Seal's start rule, in Korean and English.
// Each document missing a token gives one failure that lists them all.
func TestCoreIntegrityAC12_Documents(t *testing.T) {
	const v = "rules-v6-undocumented"
	report := func(path string, missing []string) {
		t.Helper()
		if len(missing) > 0 {
			t.Errorf("jaekit:violation=%s %s lacks %s", v, path, strings.Join(missing, "; "))
		}
	}
	paths := []string{
		"../contracts/run-record.md", "../contracts/run-record.en.md",
		"../contracts/core-capabilities.md", "../contracts/core-capabilities.en.md",
		"../guides/USAGE.md", "../guides/USAGE.en.md",
		"../guides/OPERATIONS.md", "../guides/OPERATIONS.en.md",
		cpSkill, cpHaRef,
	}
	files := cpRead(t, paths...)

	// run-record: the 4.6 table, the run-rules/6 section, 5.1 and 5.2.
	for _, p := range paths[:2] {
		text := files[p]
		var missing []string
		if sec, ok := cpSection(text, cpNumbered("4.6")); !ok {
			missing = append(missing, "the 4.6 section")
		} else {
			row := false
			for _, l := range strings.Split(sec, "\n") {
				row = row || (strings.HasPrefix(strings.TrimSpace(l), "|") && strings.Contains(l, cpNewRules))
			}
			if !row {
				missing = append(missing, "4.6: a table row with "+cpNewRules)
			}
		}
		if sec, ok := cpSection(text, func(h string) bool { return strings.Contains(h, "4.9") && strings.Contains(h, cpNewRules) }); !ok {
			missing = append(missing, "a 4.9 "+cpNewRules+" section")
		} else {
			missing = append(missing, cpMissing("4.9", sec, "review_base_mismatch", "post_completion_seconds", "base_commit", "record_head", "light", cpOldRules)...)
		}
		for _, n := range []struct {
			number string
			tokens []string
		}{{"5.1", []string{cpNewRules}}, {"5.2", []string{cpNewRules, "post_completion_seconds"}}} {
			if sec, ok := cpSection(text, cpNumbered(n.number)); !ok {
				missing = append(missing, "the "+n.number+" section")
			} else {
				missing = append(missing, cpMissing(n.number, sec, n.tokens...)...)
			}
		}
		report(p, missing)
	}

	// core-capabilities: the supported rule and the new start's requirement.
	for _, p := range paths[2:4] {
		report(p, cpMissing("the document", files[p], cpNewRules, "--rules "+cpNewRules))
	}

	// USAGE: a run-rules/6 paragraph in the review gate section.
	for i, heading := range []string{"검토 gate (run-rules/5)", "Review gate (run-rules/5)"} {
		p := paths[4+i]
		if sec, ok := cpSection(files[p], cpHeading(heading)); !ok {
			report(p, []string{"the section `## " + heading + "`"})
		} else {
			report(p, cpMissing("## "+heading, sec, cpNewRules, "review_base_mismatch", "post_completion_seconds"))
		}
	}

	// OPERATIONS: the rule description.
	for _, p := range paths[6:8] {
		report(p, cpMissing("the document", files[p], cpNewRules))
	}

	// Seal: the skill's start line and the reference's run-rules/6 section.
	report(cpSkill, cpMissing("the skill", files[cpSkill], "--rules "+cpNewRules))
	if sec, ok := cpSection(files[cpHaRef], cpHeading(cpNewRules)); !ok {
		report(cpHaRef, []string{"the section `## " + cpNewRules + "`"})
	} else {
		report(cpHaRef, cpMissing("## "+cpNewRules, sec, "review_base_mismatch", "post_completion_seconds"))
	}
}
