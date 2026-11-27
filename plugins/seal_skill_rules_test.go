package plugins

// Property: Seal skill rules
// The PLAN template and core-requirements.json keep the forms ha parses,
// the body stays within its word cap, this tree's ha carries a synthetic
// goal from the start form the skill shows and from the PROGRESS template,
// and spar runs the review round form the skill shows.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jgoneit/jaekit/internal/spar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	skSkill        = sealSkillDir + "/SKILL.md"
	skDecl         = sealSkillDir + "/core-requirements.json"
	skPlanTmpl     = sealSkillDir + "/assets/templates/PLAN.md"
	skProgressTmpl = sealSkillDir + "/assets/templates/PROGRESS.md"
)

// skFlat joins white space runs so that a phrase matches across wrapped
// lines.
func skFlat(s string) string { return strings.Join(strings.Fields(s), " ") }

// skRequire reports each phrase that text, with white space runs read as one
// space, does not hold.
func skRequire(t *testing.T, where, text string, phrases ...string) {
	t.Helper()
	flat := skFlat(text)
	for _, p := range phrases {
		if !strings.Contains(flat, skFlat(p)) {
			t.Errorf("%s lacks %q", where, p)
		}
	}
}

// skSection returns the lines from heading to the next heading of the same or
// a higher level, outside fenced code; "" when the heading is absent.
func skSection(text, heading string) string {
	level := strings.Index(heading, " ")
	lines := strings.Split(text, "\n")
	start, fenced := -1, false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if fenced {
			continue
		}
		if start < 0 {
			if line == heading {
				start = i
			}
			continue
		}
		if n := strings.Index(line, " "); n > 0 && n <= level && strings.Trim(line[:n], "#") == "" {
			return strings.Join(lines[start:i], "\n")
		}
	}
	if start < 0 {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

// skTemplate reports each phrase that the section under heading of a
// template does not hold, white space runs read as one space; heading ""
// means the whole file.
func skTemplate(t *testing.T, path, heading string, phrases ...string) {
	t.Helper()
	text, where := read(t, path), path
	if heading != "" {
		if text = skSection(text, heading); text == "" {
			t.Errorf("%s lacks the section %q", path, heading)
			return
		}
		where += " " + heading
	}
	skRequire(t, where, text, phrases...)
}

// skStartSpan finds the ha start command the skill tells the agent to run:
// the code span that starts with "ha start <goal>" and passes the skill.
var skStartSpan = regexp.MustCompile("`(ha start <goal> [^`]*--skill [^`]*)`")

// skStartForm is that code span in the first document of the Seal skill
// that holds one, or "".
func skStartForm(t *testing.T) string {
	t.Helper()
	for _, doc := range skillDocs(t, sealSkillDir) {
		if m := skStartSpan.FindStringSubmatch(doc.text); m != nil {
			return m[1]
		}
	}
	return ""
}

// skStartArgv fills the start form's placeholders for a synthetic goal.
func skStartArgv(form, goal, skillPath string) ([]string, error) {
	var out []string
	for _, word := range regexp.MustCompile(`"[^"]*"|<[^>]*>|\S+`).FindAllString(form, -1) {
		switch {
		case word == "ha" && len(out) == 0:
			continue
		case word == "<goal>":
			word = goal
		case word == `"<the user's words>"`:
			word = "synthetic request"
		case word == "<path to this SKILL.md>":
			word = skillPath
		case strings.Contains(word, "<"):
			return nil, fmt.Errorf("unknown placeholder %s", word)
		}
		out = append(out, word)
	}
	return out, nil
}

// skRun runs a tool and returns its combined output and exit code. A tool
// that cannot start or does not run to an exit code fails the test.
func skRun(t *testing.T, dir string, env []string, name string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &exit) && exit.Exited():
		return out.String(), exit.ExitCode()
	}
	t.Fatalf("setup: %s %v did not run to an exit code: %v %s", name, args, err, out.String())
	return "", 0
}

// skRepo is a synthetic Git repository with one goal, outside this tree.
type skRepo struct {
	dir, goal string
	env       []string
}

func skNewRepo(t *testing.T, plan string) *skRepo {
	t.Helper()
	r := &skRepo{dir: t.TempDir(), goal: "docs/specs/demo"}
	r.env = append(os.Environ(),
		"XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	files := map[string]string{
		"go.mod": "module example.com/demo\n\ngo 1.21\n",
		"greet/greet_test.go": "package greet\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
			"func TestGreeting(t *testing.T) {\n\tb, err := os.ReadFile(\"greeting.txt\")\n" +
			"\tif err != nil || string(b) != \"hello\\n\" {\n\t\tt.Fatalf(\"jaekit:violation=greeting-missing greeting.txt holds %q\", b)\n\t}\n}\n",
		"checks/AC-1.json": `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "greeting", "violation": "greeting-missing", "test": {"package": "example.com/demo/greet", "name": "TestGreeting"}}]}` + "\n",
		r.goal + "/SPEC.md": "# Greeting\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\n\n`greet/greeting.txt` holds `hello`.\n\n" +
			"## Acceptance Criteria\n- **AC-1** `greet/greeting.txt` holds `hello` and a newline.\n\n## Open Decisions\n\n없음\n",
		r.goal + "/PLAN.md":     plan,
		r.goal + "/REVIEW.md":   "# REVIEW — demo\n\n## 결론\nready\n",
		r.goal + "/PROGRESS.md": "# PROGRESS — demo\n\n## 현재\nsynthetic\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | |\n",
	}
	for name, content := range files {
		path := filepath.Join(r.dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("setup: synthetic repository: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: synthetic repository: %v", err)
		}
	}
	r.git(t, "init", "-q")
	r.commit(t, "base")
	return r
}

func (r *skRepo) git(t *testing.T, args ...string) {
	t.Helper()
	if out, code := skRun(t, r.dir, r.env, "git", args...); code != 0 {
		t.Fatalf("setup: git %v exited %d: %s", args, code, out)
	}
}

func (r *skRepo) commit(t *testing.T, message string) {
	t.Helper()
	r.git(t, "add", "-A")
	r.git(t, "-c", "commit.gpgsign=false", "commit", "-qm", message)
}

// skPlan is the synthetic goal's PLAN; %s is its run budget.
const skPlan = "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
	"| AC-1 | change | `go test -json -count=1 ./greet/` | greet/greet_test.go, checks/AC-1.json | T001 |\n\n" +
	"## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/AC-1.json |\n\n" +
	"## 범위\n- 바꿀 수 있는 경로: `greet/**` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n" +
	"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
	"| T001 | greeting.txt holds hello | AC-1 | — | — | 예 | — |\n\n" +
	"## 예산\n- 검증 실행: %s\n- 경과 시간: 1시간\n- 비용: 미관측\n\n## 실행 환경\n- git: `git --version`\n"

func skAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return abs
}

// The start form the skill names checks the skill's declaration under
// run-rules/8.
func TestSealSkillStartForm(t *testing.T) {
	form := skStartForm(t)
	if form == "" {
		t.Fatalf("%s names no `ha start <goal> … --skill …` command", sealSkillDir)
	}
	for _, flag := range []string{"--check-skill", "--rules run-rules/8", "--skill <path to this SKILL.md>"} {
		if !strings.Contains(form, flag) {
			t.Errorf("%s: the start form %q lacks %s", sealSkillDir, form, flag)
		}
	}
}

// The skill's start form, filled for a synthetic goal with an automatic
// budget and `## 실행 환경`, carries this tree's ha from ha doctor through
// ha done to the report of that completion.
func TestSealSkillStartFormCompletesGoal(t *testing.T) {
	form := skStartForm(t)
	if !strings.Contains(form, "--check-skill") {
		t.Fatalf("%s names no start form that checks its declaration: %q", sealSkillDir, form)
	}
	ha := filepath.Join(t.TempDir(), "ha")
	if out, code := skRun(t, "..", os.Environ(), "go", "build", "-o", ha, "./cmd/ha"); code != 0 {
		t.Fatalf("setup: building ha exited %d: %s", code, out)
	}
	r := skNewRepo(t, fmt.Sprintf(skPlan, "자동"))
	step := func(args ...string) string {
		t.Helper()
		out, code := skRun(t, r.dir, r.env, ha, args...)
		if code != 0 {
			t.Fatalf("ha %v exited %d: %s", args, code, out)
		}
		return out
	}
	if out := step("doctor", r.goal); !strings.Contains(out, "git: 사용 가능") {
		t.Fatalf("ha doctor did not report the listed tool: %s", out)
	}
	argv, err := skStartArgv(form, r.goal, skAbs(t, skSkill))
	if err != nil {
		t.Fatalf("the start form %q cannot be filled: %v", form, err)
	}
	out, code := skRun(t, r.dir, r.env, ha, argv...)
	refused := code == 64 && strings.Contains(out, "unknown flag --") ||
		code == 65 && (strings.Contains(out, "declaration; nothing recorded") || strings.Contains(out, "needs Core support this ha does not provide"))
	switch {
	case refused:
		t.Fatalf("ha refused the start form %v with exit %d: %s", argv, code, out)
	case code != 0:
		t.Fatalf("ha %v exited %d: %s", argv, code, out)
	case !strings.Contains(out, "auto limit: 50"):
		t.Fatalf("the start did not use the computed limit: %s", out)
	}
	if out := step("check", r.goal, "--baseline", "AC-1"); !strings.Contains(out, "AC-1 baseline fail_as_expected (seq ") {
		t.Fatalf("the synthetic baseline was not the expected failure: %s", out)
	}
	if err := os.WriteFile(filepath.Join(r.dir, "greet/greeting.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	r.commit(t, "change")
	if out := step("check", r.goal, "AC-1"); !strings.Contains(out, "AC-1 check pass (seq ") {
		t.Fatalf("the synthetic check did not pass: %s", out)
	}
	step("done", r.goal)
	var status struct {
		Completion *int `json:"completion_record"`
	}
	if err := json.Unmarshal([]byte(step("status", r.goal, "--format", "json")), &status); err != nil {
		t.Fatalf("ha status printed no JSON: %v", err)
	}
	if status.Completion == nil {
		t.Fatalf("ha status names no completion record after ha done")
	}
	report := step("report", r.goal, "--lang", "en")
	lines := strings.Split(report, "\n")
	for i, label := range []string{"Changed: ", "Checked: ", "To review: ", "Remaining limits: ", "Next step: "} {
		if i >= len(lines) || !strings.HasPrefix(lines[i], label) {
			t.Errorf("report line %d does not start with %q", i+1, label)
		}
	}
	seq := *status.Completion
	if len(lines) < 5 || !strings.Contains(lines[4], fmt.Sprintf("completion record seq %d", seq)) {
		t.Errorf("the summary does not name completion record seq %d", seq)
	}
	if !strings.Contains(skSection(report, "### Stamp for `## 현재`"), fmt.Sprintf("completion record: seq %d", seq)) {
		t.Errorf("the stamp does not name completion record seq %d", seq)
	}
}

// skWordCap is the Seal skill's limit in whitespace-separated words, front
// matter included.
const skWordCap = 2300

// The skill is at most 2,300 whitespace-separated words, front matter
// included.
func TestSealSkillWordCap(t *testing.T) {
	if n := len(strings.Fields(read(t, skSkill))); n > skWordCap {
		t.Errorf("%s has %d words, more than 2,300", skSkill, n)
	}
}

// Both development guides state the cap this file enforces on the line that
// links the Seal skill, and the skill is within it.
func TestSealSkillWordCapStated(t *testing.T) {
	const link, stated = "(../plugins/seal/skills/seal/SKILL.md)", "2,300"
	for _, path := range []string{"../guides/DEVELOPMENT.md", "../guides/DEVELOPMENT.en.md"} {
		found := false
		for _, line := range strings.Split(read(t, path), "\n") {
			found = found || strings.Contains(line, link) && strings.Contains(line, stated)
		}
		if !found {
			t.Errorf("%s has no line that links the Seal skill and states the cap %s", path, stated)
		}
	}
	if n := len(strings.Fields(read(t, skSkill))); n > skWordCap {
		t.Errorf("%s has %d words, more than the stated 2,300", skSkill, n)
	}
}

// The PLAN template carries the automatic budget and `## 실행 환경`.
func TestSealSkillReferencesAndTemplates(t *testing.T) {
	skTemplate(t, skPlanTmpl, "", "- 검증 실행: 자동", "## 실행 환경")
}

// The skill declares the Core support it needs in core-requirements.json
// beside it.
func TestSealSkillCoreRequirements(t *testing.T) {
	raw := read(t, skDecl)
	var decl struct {
		Schema   string              `json:"schema"`
		Rules    []string            `json:"rules"`
		Formats  map[string][]string `json:"formats"`
		Features []string            `json:"features"`
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decl); err != nil || dec.More() {
		t.Fatalf("%s is not one core-requirements object: %v", skDecl, err)
	}
	if decl.Schema != "core-requirements/v1" {
		t.Errorf("%s: schema %q, want core-requirements/v1", skDecl, decl.Schema)
	}
	has := func(field string, list []string, want ...string) {
		for _, w := range want {
			found := false
			for _, v := range list {
				found = found || v == w
			}
			if !found {
				t.Errorf("%s %s does not declare %s", skDecl, field, w)
			}
		}
	}
	has("rules", decl.Rules, "run-rules/8")
	has("features", decl.Features, "review-gate/v1", "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1",
		"structured-change-results/v1", "report/v1", "doctor/v1", "auto-budget/v1")
	has("formats.check_declaration", decl.Formats["check_declaration"], "check-declaration/v1")
	has("formats.check_result", decl.Formats["check_result"], "check-result/v1")
	has("formats.check_evidence", decl.Formats["check_evidence"], "check-evidence/v1")
	has("formats.assurance_tier", decl.Formats["assurance_tier"], "light", "standard", "strict")
	requireSkillTokens(t, sealSkillDir, "core-requirements.json")
}

// The PROGRESS template's review summary and the review round form.

const sbSummary = "## 검수 요약"

// sbLevel is the heading level of a Markdown line, or 0 when it is not a
// heading. Lines inside fenced code are left to the caller.
func sbLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// An sbFencer tracks fenced code: given the open fence marker ("" outside
// fenced code) and a line, it returns the marker after the line and whether
// the line is fenced or a fence line.
type sbFencer func(open, line string) (string, bool)

// sbPrefixFence opens a fence at a line starting with ``` or ~~~ and closes
// it at the next line starting with the same three characters.
func sbPrefixFence(open, line string) (string, bool) {
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

// sbSectionBy returns the section that opens with the line equal to heading
// once the characters in cut are trimmed from its end, up to the next heading
// of the same or a higher level outside fenced code. The section includes its
// heading line.
func sbSectionBy(text, heading string, fence sbFencer, cut string) (string, bool) {
	lines := strings.Split(text, "\n")
	level := sbLevel(heading)
	start := -1
	open := ""
	for i, l := range lines {
		var fenced bool
		if open, fenced = fence(open, l); fenced {
			continue
		}
		if start < 0 {
			if strings.TrimRight(l, cut) == heading {
				start = i
			}
			continue
		}
		if lv := sbLevel(l); lv > 0 && lv <= level {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// sbSection is the section under heading, with fences tracked by
// sbPrefixFence and trailing spaces, tabs and carriage returns ignored on the
// heading line.
func sbSection(text, heading string) (string, bool) {
	return sbSectionBy(text, heading, sbPrefixFence, " \t\r")
}

// sbPrefixSection is the text from the first line that starts with start up
// to the next line whose first word is no more '#' marks than start's; fenced
// code is not skipped. It is "" when no line starts with start.
func sbPrefixSection(text, start string) string {
	level := strings.Index(start, " ")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, start) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if n := strings.Index(lines[j], " "); n > 0 && n <= level && strings.Trim(lines[j][:n], "#") == "" {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

// sbRun runs a command in dir with env and returns its combined output. A
// command that cannot start or exits nonzero returns an error.
func sbRun(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// A goal whose PROGRESS.md is this tree's template, with `## 검수 요약` right
// after `## 현재`, works with this tree's ha. The test builds ha and makes a
// synthetic repository outside the tree whose PROGRESS.md is the template
// with only the goal name and the task state filled in. ha lint finds no
// problem before the start; after ha done, a commit that fills the review
// summary in PROGRESS.md alone leaves ha lint clean and ha status complete
// with its completion record. Every input failure (go, git or ha that cannot
// start or exits nonzero, an unreadable template) fails the test.
func TestSealBundleTemplateGoalCompletes(t *testing.T) {
	tmplData, err := os.ReadFile(skProgressTmpl)
	if err != nil {
		t.Fatalf("cannot read %s: %v", skProgressTmpl, err)
	}
	skill, err := filepath.Abs(skSkill)
	if err != nil {
		t.Fatalf("cannot resolve %s: %v", skSkill, err)
	}
	ha := filepath.Join(t.TempDir(), "ha")
	if out, err := sbRun("..", os.Environ(), "go", "build", "-o", ha, "./cmd/ha"); err != nil {
		t.Fatalf("building ha: %v\n%s", err, out)
	}
	dir := t.TempDir()
	env := append(os.Environ(),
		"XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	const goal = "docs/specs/demo"
	progressPath := filepath.Join(dir, goal, "PROGRESS.md")
	progress := strings.ReplaceAll(string(tmplData), "<goal>", "demo")
	files := map[string]string{
		"greet/greeting.txt": "hello\n",
		goal + "/SPEC.md": "# Demo\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\n\nGit runs here.\n\n" +
			"## Acceptance Criteria\n- **AC-1** `git --version` exits 0.\n\n## Open Decisions\n\n없음\n",
		goal + "/PLAN.md": "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
			"| AC-1 | maintain | `git --version` | — | T001 |\n\n" +
			"## 범위\n- 바꿀 수 있는 경로: `greet/**` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n" +
			"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
			"| T001 | git runs | AC-1 | — | — | 예 | — |\n\n" +
			"## 예산\n- 검증 실행: 10\n- 경과 시간: 1시간\n- 비용: 미관측\n\n## 실행 환경\n- git: `git --version`\n",
		goal + "/REVIEW.md":   "# REVIEW — demo\n\n## 결론\nready\n",
		goal + "/PROGRESS.md": progress,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("synthetic repository: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("synthetic repository: %v", err)
		}
	}
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := sbRun(dir, env, name, args...)
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return out
	}
	commit := func(message string) {
		t.Helper()
		run("git", "add", "-A")
		run("git", "-c", "commit.gpgsign=false", "commit", "-qm", message)
	}
	writeProgress := func(old, new string) {
		t.Helper()
		if !strings.Contains(progress, old) {
			t.Fatalf("the template-made PROGRESS.md has no %q", old)
		}
		progress = strings.Replace(progress, old, new, 1)
		if err := os.WriteFile(progressPath, []byte(progress), 0o644); err != nil {
			t.Fatalf("synthetic repository: %v", err)
		}
	}
	run("git", "init", "-q")
	commit("base")
	run(ha, "lint", goal)
	run(ha, "start", goal, "--rules", "run-rules/7", "--request", "demo 구현해줘", "--skill", skill, "--check-skill")
	writeProgress("| T001 | todo | |", "| T001 | done | |")
	commit("task done")
	run(ha, "check", goal)
	run(ha, "done", goal)
	summary, ok := sbSection(progress, sbSummary)
	if !ok {
		t.Fatalf("the template-made PROGRESS.md has no %s", sbSummary)
	}
	filled := []string{sbSummary, "완료 기록 기준. 필수 조건 1개 중 1개 충족. git이 돈다. 먼저 볼 것: 없음", ""}
	for _, l := range strings.Split(summary, "\n")[1:] {
		switch {
		case strings.HasPrefix(l, "<"):
			filled = append(filled, "없음")
		default:
			filled = append(filled, l)
		}
	}
	writeProgress(summary, strings.Join(filled, "\n"))
	commit("fill the review summary")
	run(ha, "lint", goal)
	var status struct {
		Status     string `json:"status"`
		Completion *int   `json:"completion_record"`
	}
	if err := json.Unmarshal([]byte(run(ha, "status", goal, "--format", "json")), &status); err != nil {
		t.Fatalf("ha status printed no JSON: %v", err)
	}
	if status.Status != "complete" || status.Completion == nil {
		t.Errorf("after a commit that fills the review summary alone, ha status is %q with completion record %v, not complete with one", status.Status, status.Completion)
	}
}

// sbWorld is a repository of two commits with criteria and rules, and a user
// configuration whose command reviewer answers for the head with no finding.
// Anything that keeps the world from being made fails the test.
type sbWorld struct {
	t          *testing.T
	dir, repo  string
	base, head string
	config     string
}

func sbNewWorld(t *testing.T) *sbWorld {
	t.Helper()
	w := &sbWorld{t: t, dir: t.TempDir()}
	if real, err := filepath.EvalSymlinks(w.dir); err == nil {
		w.dir = real
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is not available: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	w.repo = filepath.Join(w.dir, "repo")
	w.write(filepath.Join(w.repo, ".gitignore"), "/docs/reviews/\n")
	w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\nKeep reviews honest.\n")
	w.write(filepath.Join(w.repo, "docs", "specs", "example", "SPEC.md"), "# Goal\n\n- **AC-1** one\n")
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('one')\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('two')\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	checked := []any{}
	for _, a := range spar.Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	answer, err := json.Marshal(map[string]any{"reviewed_head": w.head, "verdict": "no_blocking_findings",
		"angles_checked": checked, "angles_unchecked": []any{}, "findings": []any{}, "limits": "synthetic reviewer"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	answerPath := w.write(filepath.Join(w.dir, "answer.json"), string(answer))
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("cat is not available: %v", err)
	}
	w.config = w.write(filepath.Join(w.dir, "spar.toml"),
		fmt.Sprintf("[[reviewers]]\nname = \"cmd\"\npreset = \"command\"\nargv = [%q, %q]\n", cat, answerPath))
	return w
}

func (w *sbWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatalf("%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatalf("%v", err)
	}
	return path
}

func (w *sbWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		w.t.Fatalf("git %v: %v %s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// spar runs the spar command line in process with the world's configuration.
func (w *sbWorld) spar(args ...string) (int, string) {
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
	code := spar.Main("test", args, strings.NewReader(""), &stdout, &stderr, getenv)
	return code, stderr.String()
}

// sbShellQuote quotes a value as the skill's documents say the round form's
// values are quoted: in single quotes, each single quote in it closed,
// escaped with a backslash and reopened. sbQuote2 holds every character a
// shell would change outside single quotes.
const sbQuote2 = "치명적이면 `$HOME`과 \\ 그리고 'x'도 그대로 (Recommended)"

func sbShellQuote(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }

// sbWords splits a shell command line into words: a backslash at the end of
// a line joins it to the next, a backslash outside quotes keeps the next
// character, and single and double quotes group a word. A quote that does
// not close is an error.
func sbWords(line string) ([]string, error) {
	line = strings.ReplaceAll(line, "\\\n", " ")
	var words []string
	var cur strings.Builder
	in, quote, started := false, byte(0), false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case in && c == quote:
			in = false
		case in:
			cur.WriteByte(c)
		case c == '"' || c == '\'':
			in, quote, started = true, c, true
		case c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
			started = true
		case c == ' ' || c == '\t' || c == '\n':
			if started {
				words = append(words, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if in {
		return nil, fmt.Errorf("the quote %c does not close", quote)
	}
	if started {
		words = append(words, cur.String())
	}
	return words, nil
}

// sbForm is the first bash block of the Seal skill's documents that runs
// spar with --decision.
func sbForm(t *testing.T) string {
	t.Helper()
	for _, doc := range skillDocs(t, sealSkillDir) {
		for _, p := range strings.Split(doc.text, "```bash\n")[1:] {
			block, _, _ := strings.Cut(p, "\n```")
			if strings.HasPrefix(block, "spar run ") && strings.Contains(block, "--decision ") {
				return block
			}
		}
	}
	return ""
}

// The command form the skill's documents show, with two decisions and one
// place question filled in, runs spar without a refusal, and the round's
// packet holds both decisions in its decision section and the question. The
// documents keep the single-quote escape the filled values use.
func TestSealBundleReferenceSparForm(t *testing.T) {
	const decisions = "## Decisions the user made"
	requireSkillTokens(t, sealSkillDir, `'\''`)
	form := sbForm(t)
	if form == "" {
		t.Errorf("%s shows no `spar run` form with --decision", sealSkillDir)
		return
	}

	w := sbNewWorld(t)
	values := []string{
		"<goal base commit>", w.base, "<head>", w.head, "<goal>", "example", "<NN>", "01",
		"<what decision 1 settles>", "skill 문장의 범위",
		"<the recorded words of decision 1>", "참고 문서에 맡김 (Recommended)",
		"<what decision 2 settles>", "코드 라운드 상한",
		"<the recorded words of decision 2>", sbShellQuote(sbQuote2),
		"<path>", "internal/spar/packet.go", "<name used there>", "buildPacket", "<invariant>", "the decision section",
	}
	filled := strings.NewReplacer(values...).Replace(form)
	if i := strings.Index(filled, "<"); i >= 0 {
		end := strings.Index(filled[i:], ">")
		if end < 0 {
			end = len(filled) - i - 1
		}
		t.Errorf("%s: the form has a placeholder this check does not fill: %s", sealSkillDir, filled[i:i+end+1])
		return
	}
	if _, err := sbWords(`spar run --decision "x`); err == nil {
		t.Fatalf("the word splitter takes a quote that does not close")
	}
	words, err := sbWords(filled)
	if err != nil {
		t.Errorf("%s: the form is not a command line: %v", sealSkillDir, err)
		return
	}
	if len(words) < 2 || words[0] != "spar" || words[1] != "run" {
		t.Errorf("%s: the form does not start with `spar run`: %q", sealSkillDir, words)
		return
	}
	// Every document the form names is in the world, so that a refusal
	// observes the form and not a missing file.
	for i := 0; i+1 < len(words); i++ {
		if (words[i] == "--criteria" || words[i] == "--rules" || words[i] == "--subject" || words[i] == "--boundary") && !filepath.IsAbs(words[i+1]) {
			path := filepath.Join(w.repo, filepath.FromSlash(words[i+1]))
			if _, err := os.Stat(path); err != nil {
				w.write(path, "# Document\n\n- **AC-1** one\n")
			}
		}
	}
	t.Chdir(w.repo)
	code, stderr := w.spar(append(words[1:], "--repo", w.repo)...)
	switch {
	case code == spar.ExitOK:
	case code == spar.ExitUsage || (code == spar.ExitRefused && strings.Contains(stderr, "decision")):
		// A usage error comes only from the form's arguments; a refusal
		// that names a decision judges the form's decisions.
		t.Errorf("spar refuses the form of %s (exit %d): %s", sealSkillDir, code, strings.TrimSpace(stderr))
		return
	default:
		t.Fatalf("spar run exits %d: %s", code, stderr)
	}
	out := ""
	for i, word := range words {
		if word == "--out" && i+1 < len(words) {
			out = words[i+1]
		}
	}
	data, err := os.ReadFile(filepath.Join(w.repo, filepath.FromSlash(out), "packet.md"))
	if err != nil {
		t.Fatalf("%v", err)
	}
	packet := string(data)
	at := strings.Index(packet, "\n"+decisions+"\n")
	if at < 0 {
		t.Errorf("the packet has no %q section", decisions)
		return
	}
	rest := packet[at:]
	for _, v := range []string{"skill 문장의 범위", "참고 문서에 맡김 (Recommended)", "코드 라운드 상한", sbQuote2} {
		n := strings.Index(rest, v)
		if n < 0 {
			t.Errorf("the packet's decision section does not hold %q after the parts before it", v)
			return
		}
		rest = rest[n+len(v):]
	}
	question := "- internal/spar/packet.go (buildPacket): does it still keep the decision section at head?"
	if !strings.Contains(sbPrefixSection(packet, "## Questions for this round"), question) {
		t.Errorf("the packet's questions do not hold %q", question)
	}
}
