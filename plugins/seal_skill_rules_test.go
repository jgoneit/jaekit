package plugins

// Property: Seal skill rules
// The Seal skill's documents keep the format tokens of their rules (code
// spans, headings and link targets), wherever in the skill a rule sits. The
// templates and core-requirements.json keep their forms, the body stays
// within its word cap, this tree's ha carries a synthetic goal from the
// start form the skill shows and from the PROGRESS template, and spar runs
// the review round form the skill shows.

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
	"slices"
	"strings"
	"testing"
)

const (
	skSkill        = sealSkillDir + "/SKILL.md"
	skDecl         = sealSkillDir + "/core-requirements.json"
	skPlanTmpl     = sealSkillDir + "/assets/templates/PLAN.md"
	skReviewTmpl   = sealSkillDir + "/assets/templates/REVIEW.md"
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

// skTemplateLine reports unless one line of the section under heading of a
// template holds every phrase, white space runs read as one space and case
// ignored.
func skTemplateLine(t *testing.T, path, heading string, phrases ...string) {
	t.Helper()
	sec := skSection(read(t, path), heading)
	if sec == "" {
		t.Errorf("%s lacks the section %q", path, heading)
		return
	}
	for _, line := range strings.Split(sec, "\n") {
		flat, all := strings.ToLower(skFlat(line)), true
		for _, p := range phrases {
			all = all && strings.Contains(flat, strings.ToLower(skFlat(p)))
		}
		if all {
			return
		}
	}
	t.Errorf("%s %s has no line holding all of %q", path, heading, phrases)
}

// skUnquote strips the backquotes of tokens written as Markdown code spans,
// such as "`관측`", to give the code span contents a skill token check reads.
func skUnquote(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, tok := range tokens {
		out[i] = strings.Trim(tok, "`")
	}
	return out
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
// run-rules/7.
func TestSealSkillStartForm(t *testing.T) {
	form := skStartForm(t)
	if form == "" {
		t.Fatalf("%s names no `ha start <goal> … --skill …` command", sealSkillDir)
	}
	for _, flag := range []string{"--check-skill", "--rules run-rules/7", "--skill <path to this SKILL.md>"} {
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

// The skill names the install route and the PATH check for a missing ha,
// and never sends users to the jaekit repository to install it.
func TestSealSkillMissingHa(t *testing.T) {
	forbidSkillText(t, sealSkillDir, "from the jaekit repository")
	requireSkillTokens(t, sealSkillDir, "brew install jgoneit/tap/jaekit", "which -a ha")
}

// The check input rules keep their tokens: the condition kinds they cover,
// the runner adapter feature, the path that must not change and the reason
// Core reports for it, the documents they never convert and the link to the
// details; the skill keeps the confirm note and the legacy rule forms those
// rules sit beside.
func TestSealSkillCheckInputs(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## Check inputs", "## Assurance policy",
		"change", "maintain", "EX-n", "runner-adapters/v1", "바꿀 수 있는 경로", "out_of_scope", "PROGRESS.md", "REVIEW.md",
		"link:references/bundle.md#check-inputs",
		"ha note <goal> confirm <AC-n | scope:<path> | tests:<path> | spec>", "needs_user", "/1", "/2")
}

// The skill keeps its sections.
func TestSealSkillKeptAreas(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## Authority", "## The execution bundle", "## Seal Core", "## Check inputs",
		"## Assurance policy", "## Independent review", "## Completion", "## Post-completion discoveries", "## Resuming",
		"## Forbidden", "## Language")
}

// The skill has its `## References` heading and links every file in
// references/.
func TestSealSkillReferences(t *testing.T) {
	refs, err := filepath.Glob(filepath.Join(sealSkillDir, "references", "*.md"))
	if err != nil || len(refs) == 0 {
		t.Fatalf("%s/references holds no .md file: %v", sealSkillDir, err)
	}
	tokens := []string{"## References"}
	for _, ref := range refs {
		tokens = append(tokens, "link:references/"+filepath.Base(ref))
	}
	requireSkillTokens(t, sealSkillDir, tokens...)
}

// The forms of goals started with an earlier Seal live in a reference the
// skill links, which keeps its sections and the earlier completion report
// form.
func TestSealSkillEarlierGoalForms(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "link:references/earlier-goals.md",
		"# Goals started with an earlier Seal", "## Starting and resuming", "## PROGRESS", "## Review and evidence",
		"## Goals started under run-rules/4 or earlier", "ha done", "ha status --format md")
}

// The review stages, the order of ha done and rework, the finding classes
// and the earlier review form keep their tokens. The round after review fixes
// is a candidate round from the goal's base commit, as the cumulative round
// command shows.
func TestSealSkillReviewOrder(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "plan", "candidate", "--stage candidate --base <goal base commit>", "strict",
		"REVIEW.md", "ha done", "reopen", "defect", "requirement-question", "hardening", "run-rules/4")
}

// SPEC's `Assurance:` line, the tiers, the stages they take, the REVIEW.md
// record and the project instructions' floor keep their tokens.
func TestSealSkillAssuranceTiers(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "Assurance:", "light", "standard", "strict", "candidate", "plan", "REVIEW.md", "AGENTS.md")
}

// The automatic budget line, the tool list section and the ha doctor run
// keep their tokens.
func TestSealSkillStartChecks(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "- 검증 실행: 자동", "## 실행 환경", "ha doctor <goal>")
}

// `## 현재` and the completion report come from ha report in the user's
// language and never from ha status or ha done: the skill's documents keep
// those tokens and the link to the report format, and the PROGRESS template
// says the same.
func TestSealSkillProgressReport(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## 현재", "ha report", "ha report <goal>", "--lang ko", "--lang en", "ha status",
		"ha done", "## Completion report", "manual", "link:references/bundle.md#completion-report")
	skTemplate(t, skProgressTmpl, "## 현재",
		"opens with the stamp `ha report <goal>` prints for `## 현재`, pasted unchanged", "never type a check time yourself")
	skTemplate(t, skProgressTmpl, "## 완료 보고",
		"the output of `ha report <goal> --lang ko`", "never paste `ha status` or `ha done` output")
}

// The bundle and ha references keep the automatic budget, the `## 실행 환경`
// section, the start check and the report and doctor commands; the PLAN and
// REVIEW templates carry the automatic budget, `## 실행 환경` and the review
// plan.
func TestSealSkillReferencesAndTemplates(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "검증 실행: 자동", "ha start", "### `## 실행 환경`", "--check-skill",
		"ha report <goal> [--lang ko\\|en]", "ha doctor <goal>")
	skTemplate(t, skPlanTmpl, "", "- 검증 실행: 자동", "## 실행 환경")
	skTemplate(t, skReviewTmpl, "",
		"## 독립 검토", "the planned review stages", "product invariants and decision rules", "supported input forms")
}

// The rules for questions to the user keep their sections and tokens: the
// questions reference and the skill's link to it, the scope section that
// lists new check files from the start, the ha doctor report, the confirm
// note an early answer becomes and the wait note.
func TestSealSkillQuestionsUpFront(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## Starting", "## Authority", "## Seal Core", "## Questions for the user",
		"### `## 범위`", "## run-rules/4", "link:references/bundle.md#questions-for-the-user", "바꿀 수 있는 경로",
		"ha doctor", "ha note <goal> confirm", `ha note <goal> wait --reason "<the decision asked>"`)
}

// The skill names the goal directory, and no document of the skill turns
// local retention infrastructure into a product requirement.
func TestSealSkillProjectRecords(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "docs/specs/<goal>")
	forbidSkillText(t, sealSkillDir, "private backup repository", "primary checkout", "launchd")
}

// Review rounds take the goal's check files as `--check-path`.
func TestSealSkillRoundCheckPaths(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "--check-path")
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
	has("rules", decl.Rules, "run-rules/7")
	has("features", decl.Features, "review-gate/v1", "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1",
		"structured-change-results/v1", "report/v1", "doctor/v1", "auto-budget/v1")
	has("formats.check_declaration", decl.Formats["check_declaration"], "check-declaration/v1")
	has("formats.check_result", decl.Formats["check_result"], "check-result/v1")
	has("formats.check_evidence", decl.Formats["check_evidence"], "check-evidence/v1")
	has("formats.assurance_tier", decl.Formats["assurance_tier"], "light", "standard", "strict")
	requireSkillTokens(t, sealSkillDir, "core-requirements.json")
}

// skDuty is one executor duty of the Seal skill and the format tokens that
// state it in the skill's documents. A duty whose statement holds no format
// token, or only the bare name ha, is not listed.
type skDuty struct {
	id     string
	tokens []string
}

var skDuties = []skDuty{
	// Authority
	{"ready-not-request", []string{"Status: Ready"}},
	{"quote-request", []string{"ha start --request"}},
	// The execution bundle
	{"bundle-handover", []string{"ha status"}},
	{"templates", []string{"assets/templates/"}},
	{"jaekit-docs-policy", []string{"docs/"}},
	{"bundle-files", []string{"PLAN.md condition table, result declarations, scope, tasks, budget, tools"}},
	{"plan-changes", []string{"PROGRESS.md"}},
	{"plan-change-impact", []string{"시각 \\| 변경 \\| 이유 \\| 영향 \\| 다시 검토한 것"}},
	{"current-stamp", []string{"## 현재", "ha report"}},
	{"timeline-append", []string{"## 타임라인"}},
	{"extra-checks-optional", []string{"EX-n"}},
	{"needs-user-review", []string{"REVIEW.md", "needs_user"}},
	// Seal Core
	{"ha-install", []string{"brew install jgoneit/tap/jaekit"}},
	{"capability-compare", []string{"core-requirements.json"}},
	{"start-rules", []string{"--rules run-rules/7"}},
	{"host-flags", []string{"--host-name", "--host-version", "--model"}},
	{"baseline", []string{"ha check <goal> --baseline AC-n", "change"}},
	{"legacy-semantics", []string{"/1", "/2"}},
	{"clean-committed", []string{"ha check", "fail"}},
	{"unknown-shares-limit", []string{"unknown"}},
	// Check inputs
	{"inputs-scope-path", []string{"바꿀 수 있는 경로", "out_of_scope"}},
	// Final candidate evidence
	{"final-once", []string{"ha check"}},
	// Autonomy under run-rules/4
	{"test-authority", []string{`--test-authority "<your reading of the request>"`}},
	{"test-change-note", []string{"ha note <goal> test-change tests:<path> --criteria AC-n --how"}},
	{"test-change-moved", []string{"--moved-to <path>"}},
	{"test-weakened", []string{"--weakened"}},
	{"wait-note", []string{"ha note <goal> wait --reason"}},
	{"wait-resume", []string{"ha note <goal> resume"}},
	{"preapprove-raise", []string{"--runs", "--reason"}},
	{"preapprove-record", []string{`ha budget <goal> --preapprove <N> --window <seq> --revision <seq> --quote "…" --context "…"`}},
	{"recover-note", []string{"ha note <goal> recover <AC-n> --input check|baseline --change"}},
	{"recover-failures-stay", []string{"fail"}},
	{"time-limit", []string{"## 시간 제한"}},
	// Completion
	{"done-only", []string{"ha done <goal>"}},
	{"done-output", []string{"ha report"}},
	{"remote-review-stale", []string{"ha done"}},
	{"needs-user", []string{"needs_user"}},
	{"blocked-stop", []string{"blocked", "budget_exhausted", "PROGRESS.md"}},
	{"confirm-note", []string{"ha note <goal> confirm <AC-n | scope:<path> | tests:<path> | spec> --quote"}},
	{"terminal-block", []string{"block"}},
	{"report-local", []string{"local"}},
	// Post-completion discoveries
	{"discovery-rework", []string{"reopen"}},
	// Resuming
	{"resume-status", []string{"ha status <goal>"}},
	{"resume-trust", []string{"PROGRESS.md"}},
	{"resume-entry", []string{"재개"}},
	{"resume-no-convert", []string{"/3", "/1", "/2"}},
	// Budget
	{"budget-default", []string{"- 검증 실행: 자동"}},
	{"budget-interpretation", []string{"--interpretation"}},
	{"budget-rules4-window", []string{"/4"}},
	{"budget-legacy", []string{"/1", "/2"}},
	// Forbidden
	{"forbid-records", []string{"runs.jsonl"}},
	{"forbid-output", []string{"ha log"}},
	{"forbid-output-pointer", []string{"ha log <goal> <seq>"}},
	{"forbid-self-change", []string{"## 개선 메모"}},
}

// No duty of the Seal skill silently loses the format tokens that state it.
func TestSealSkillDutiesAccounted(t *testing.T) {
	for _, d := range skDuties {
		t.Run(d.id, func(t *testing.T) { requireSkillTokens(t, sealSkillDir, d.tokens...) })
	}
}

// The baseline guidance keeps its tokens: the rule and the PLAN column mark,
// the sections that state it, the unobserved scope it adds and the skill's
// links to those sections.
func TestSealSkillBaselineGuidance(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## REVIEW.md", "## Check inputs", "## Structured change results", "## Review summary",
		"run-rules/7", "baseline", "요구", "### 보지 않은 것",
		"link:references/bundle.md", "link:references/bundle.md#check-inputs", "link:references/bundle.md#review-summary",
		"link:references/ha.md#structured-change-results")
}

// Under run-rules/7 "before any baseline" means before the goal's first ha
// check record: the questions section that says so, ha doctor and the
// skill's link to that section keep their tokens.
func TestSealSkillFirstCheckPoint(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## Questions for the user", "run-rules/7", "ha check", "ha doctor", "ha doctor <goal>",
		"link:references/bundle.md#questions-for-the-user")
}

// The input failure guidance keeps its tokens: the check inputs and
// assurance sections, the condition kinds that require no baseline, the
// declared missing case, REVIEW.md where the producer's outcomes go, and the
// links to those sections.
func TestSealSkillInputFailureGuidance(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## Check inputs", "## Assurance policy and counterexamples", "maintain", "EX-n",
		"REVIEW.md", "missing: violation", "link:references/bundle.md#check-inputs",
		"link:references/bundle.md#assurance-policy-and-counterexamples")
}

// The check freeze keeps its tokens: the stages whose earliest round is the
// freeze head, the notes that record the gate and a changed check file, the
// freeze display and the link to the check inputs.
func TestSealSkillCheckFreeze(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## Check inputs", "plan", "strict", "candidate", "standard",
		"ha note <goal> check-change checks:<path> --reason", "ha note <goal> review --gate", "check_freeze",
		"link:references/bundle.md#check-inputs")
}

// The skill starts under run-rules/7, and its documents keep the rule's
// section, the minimum formula, the baseline mark, the check-change note,
// the freeze stages and the condition table section; the PLAN and REVIEW
// templates name the rule and the mark.
func TestSealSkillRunRules7(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "--rules run-rules/7", "## run-rules/7", "run-rules/6",
		"표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain",
		"ha note <goal> check-change", "baseline", "요구", "plan", "candidate", "### `## 조건표`")
	skTemplateLine(t, skPlanTmpl, "## 조건표", "`run-rules/7`", "`baseline` column", "`요구`")
	skTemplateLine(t, skReviewTmpl, "## 검증 방법과 환경", "`run-rules/7`", "requires a baseline", "`요구`")
}

// The ha reference keeps a run-rules/6 section, and the skill's documents
// keep its review base and post-completion judgment tokens.
func TestSealSkillRunRules6(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "## run-rules/6", "review_base_mismatch", "post_completion_seconds")
}

// The review summary, the REVIEW and PROGRESS forms, and the decisions passed
// to Spar rounds: the PROGRESS and REVIEW templates keep their wording and
// structure, and the skill's documents keep the format tokens.

const sbSummary = "## 검수 요약"

// sbNorm collapses every run of white space to one space, so line wrapping
// does not change what a check sees.
func sbNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// sbMissing lists the tokens the text lacks, ignoring line wrapping.
func sbMissing(where, text string, tokens ...string) []string {
	var out []string
	norm := sbNorm(text)
	for _, tok := range tokens {
		if !strings.Contains(norm, sbNorm(tok)) {
			out = append(out, fmt.Sprintf("%s lacks %q", where, tok))
		}
	}
	return out
}

// sbNeed reports every token the text lacks, ignoring line wrapping.
func sbNeed(t *testing.T, where, text string, tokens ...string) {
	t.Helper()
	for _, m := range sbMissing(where, text, tokens...) {
		t.Error(m)
	}
}

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

// sbRunFence opens a fence at a run of three or more backticks or tildes and
// closes it at a run of the same character at least as long with nothing
// after it.
func sbRunFence(open, line string) (string, bool) {
	trim := strings.TrimSpace(line)
	if len(trim) >= 3 && (trim[0] == '`' || trim[0] == '~') {
		n := len(trim) - len(strings.TrimLeft(trim, trim[:1]))
		if n >= 3 {
			if open == "" {
				return trim[:n], true
			}
			if trim[0] == open[0] && n >= len(open) && strings.TrimSpace(trim[n:]) == "" {
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

// sbNeedSection returns a section of a file, reporting a missing section. A
// file that cannot be read fails the test.
func sbNeedSection(t *testing.T, path, heading string) (string, bool) {
	t.Helper()
	sec, ok := sbSection(read(t, path), heading)
	if !ok {
		t.Errorf("%s has no section %q", path, heading)
	}
	return sec, ok
}

// sbHeadings lists the headings of one level outside fenced code, in order.
func sbHeadings(text string, level int, fence sbFencer) []string {
	var out []string
	open := ""
	for _, l := range strings.Split(text, "\n") {
		var fenced bool
		if open, fenced = fence(open, l); fenced {
			continue
		}
		if sbLevel(l) == level {
			out = append(out, strings.TrimRight(l, " \t\r"))
		}
	}
	return out
}

// The four headings of the review summary: all of them, in this order.
var sbSummaryHeadings = []string{
	"### 바뀐 것",
	"### 근거",
	"### 직접 볼 것",
	"### 보지 않은 것",
}

// The opening lines of the review summary, before its first heading.
var sbSummaryOpening = []string{
	"the completion record's seq and commit, the assurance tier, how many required conditions are satisfied, and the review gate result",
	"one line on what the change does",
	"where to start looking",
}

// The endings a review finding can have, all of them.
var sbEndings = []string{"`고침`", "`반박`", "`중복`", "`사용자의 답`", "`이슈로 보류`", "`자동 보류`", "`처리 없음`"}

// sbNoPlace marks a review summary item that has no place under its heading.
const sbNoPlace = "summary-item-without-place"

// sbMark reports, with the marker of id first, every token the text lacks.
func sbMark(t *testing.T, id, where, text string, tokens ...string) {
	t.Helper()
	for _, m := range sbMissing(where, text, tokens...) {
		t.Errorf("jaekit:violation=%s: %s", id, m)
	}
}

// sbMarkSkillTokens reports, with the marker of id first, each token that no
// document of the Seal skill holds as a format token.
func sbMarkSkillTokens(t *testing.T, id string, tokens ...string) {
	t.Helper()
	set := skillTokens(t, sealSkillDir)
	for _, tok := range tokens {
		if !set.has(sealSkillDir, tok) {
			t.Errorf("jaekit:violation=%s: %s lacks the format token %q", id, sealSkillDir, tok)
		}
	}
}

// sbTmplSummary is the PROGRESS template's `## 검수 요약`; a template without
// it, or without a `## ` line after it, fails the test as a broken setup.
func sbTmplSummary(t *testing.T) string {
	t.Helper()
	text := read(t, skProgressTmpl)
	sec, ok := sbSection(text, sbSummary)
	if !ok || !strings.Contains(text, sec+"\n## ") {
		t.Fatalf("%s has no section %q followed by a `## ` line", skProgressTmpl, sbSummary)
	}
	return sec
}

// sbNeedSub asks, with the marker of id, the tokens of the subsection under
// heading of the PROGRESS template's `## 검수 요약`; a missing subsection is
// reported with the marker.
func sbNeedSub(t *testing.T, id, heading string, tokens ...string) {
	t.Helper()
	sub, ok := sbSection(sbTmplSummary(t), heading)
	if !ok {
		t.Errorf("jaekit:violation=%s: %s: %s has no %q", id, skProgressTmpl, sbSummary, heading)
		return
	}
	sbMark(t, id, skProgressTmpl+" "+heading, sub, tokens...)
}

// sbNeedSummary asks the tokens of the PROGRESS template's `## 검수 요약`.
func sbNeedSummary(t *testing.T, tokens ...string) {
	t.Helper()
	if sec, ok := sbNeedSection(t, skProgressTmpl, sbSummary); ok {
		sbNeed(t, skProgressTmpl+" "+sbSummary, sec, tokens...)
	}
}

// `## 검수 요약` is the section right after `## 현재` in the PROGRESS
// template, which says when it is written; the skill's documents keep the
// tokens of both sections, of ha done and of the stops it is not written at.
func TestSealBundleReviewSummaryPlacement(t *testing.T) {
	heads := sbHeadings(read(t, skProgressTmpl), 2, sbPrefixFence)
	found := false
	for i, h := range heads {
		if h == "## 현재" {
			found = true
			if i+1 >= len(heads) || heads[i+1] != sbSummary {
				t.Errorf("%s: the section after `## 현재` is not %q", skProgressTmpl, sbSummary)
			}
		}
	}
	if !found {
		t.Errorf("%s has no `## 현재`", skProgressTmpl)
	}
	sbNeedSummary(t,
		"Written once `ha done` records completion, as of that completion record",
		"rewritten as of each new completion record after a reopen",
		"not written before completion or when work stops at `needs_user`, `blocked` or `budget_exhausted`")
	requireSkillTokens(t, sealSkillDir, "## 현재", sbSummary, "ha done", "needs_user", "blocked", "budget_exhausted")
}

// The template's review summary has opening lines and exactly the four
// headings, in order, with `없음` under one that has nothing to list; the
// skill's documents name every heading and `없음`.
func TestSealBundleReviewSummaryStructure(t *testing.T) {
	const id = "seven-heading-template"
	sec := sbTmplSummary(t)
	if got := sbHeadings(sec, 3, sbPrefixFence); !slices.Equal(got, sbSummaryHeadings) {
		t.Errorf("jaekit:violation=%s: %s: the headings of %s are %q, not %q", id, skProgressTmpl, sbSummary, got, sbSummaryHeadings)
	}
	opening := sec
	if i := strings.Index(sec, "\n"+sbSummaryHeadings[0]); i >= 0 {
		opening = sec[:i]
	}
	sbMark(t, id, skProgressTmpl+" "+sbSummary+" before "+sbSummaryHeadings[0], opening, sbSummaryOpening...)
	sbMark(t, id, skProgressTmpl+" "+sbSummary+" before "+sbSummaryHeadings[0], opening, "The four headings below follow")
	sbMark(t, id, skProgressTmpl+" "+sbSummary, sec, "write `없음` under a heading with nothing to list")
	sbNeedSub(t, id, sbSummaryHeadings[1], "the documents and sections that hold the detail")
	sbMarkSkillTokens(t, id, append([]string{"없음"}, sbSummaryHeadings...)...)
}

// What changed: a table lists every changed file but the goal documents,
// check files included; the skill's documents keep its heading and columns.
func TestSealBundleReviewSummaryChangedFiles(t *testing.T) {
	sbNeedSub(t, sbNoPlace, sbSummaryHeadings[0], "| 바뀐 동작 | 조건 | 파일 |",
		"Every file changed from the goal's base to the completion commit, goal documents excepted, appears in a row",
		"this goal's check files have rows too")
	sbMarkSkillTokens(t, sbNoPlace, sbSummaryHeadings[0], "바뀐 동작 | 조건 | 파일")
}

// Under what changed, changes outside the scope, to existing tests and to an
// earlier goal's checks are listed with the user's words or the reason.
func TestSealBundleReviewSummaryScopeChanges(t *testing.T) {
	sbNeedSub(t, sbNoPlace, sbSummaryHeadings[0],
		"changes outside the scope or to existing things",
		"files outside the SPEC scope",
		"changes to existing tests",
		"changes to checks an earlier goal left",
		"where the user's words allowing it are, or the reason")
	sbMarkSkillTokens(t, sbNoPlace, sbSummaryHeadings[0])
}

// The evidence names what each condition's check observed and where it
// passed, and a round table gives every finding one ending; the skill's
// documents keep the heading, the columns and every ending.
func TestSealBundleReviewSummaryEvidence(t *testing.T) {
	sbNeedSub(t, sbNoPlace, sbSummaryHeadings[1], append([]string{
		"| 라운드 | 지적 | 끝 |",
		"for every condition, what the check that satisfied it observed and the commit where it passed, never by seq alone",
		"one row for every review round, a round with no findings included",
		"Every finding of the round ends in exactly one of",
	}, sbEndings...)...)
	sbMarkSkillTokens(t, sbNoPlace, append([]string{sbSummaryHeadings[1], "라운드 | 지적 | 끝"}, skUnquote(sbEndings)...)...)
}

// What to inspect by hand, highest risk first, holds every deferred and
// undisposed finding whatever ha report shows.
func TestSealBundleReviewSummaryToInspect(t *testing.T) {
	sbNeedSub(t, sbNoPlace, sbSummaryHeadings[2],
		"ordered by risk, the highest at the top, one or two lines each",
		"every item the `직접 볼 것` line of `ha report` lists",
		"whatever that line shows",
		"every deferred finding with its issue",
		"every finding left with no disposition",
		"executor assumptions that read a SPEC sentence",
		"conditions met by checks that observe fixed text only",
		"`manual` conditions",
		"changes outside the scope or to existing things")
	sbMarkSkillTokens(t, sbNoPlace, sbSummaryHeadings[2], "직접 볼 것", "ha report", "manual", sbSummaryHeadings[0])
}

// Decisions in three groups, interpretations marked, under the evidence; and
// what was not observed.
func TestSealBundleReviewSummaryDecidersAndUnobserved(t *testing.T) {
	sbNeedSub(t, sbNoPlace, sbSummaryHeadings[1],
		"decided by the user, with where their words are",
		"proposed by the agent and accepted by the user",
		"executor assumptions nobody was asked about",
		"Mark an assumption that reads a SPEC sentence")
	sbNeedSub(t, sbNoPlace, sbSummaryHeadings[3],
		"unobserved scope and substitutes",
		"assurance `local`",
		"the executor wrote the checks",
		"not evidence of deployment, installation or a real model call")
	sbMarkSkillTokens(t, sbNoPlace, sbSummaryHeadings[1], sbSummaryHeadings[3], "local")
}

// At completion the PROGRESS template's completion report holds the ha
// report output and one line pointing to the review summary, a stop keeps
// the earlier report, and every judgment item that moves out of the
// completion report has a place in the review summary; the skill's
// documents keep the tokens of both sections and of each place.
func TestSealBundleCompletionReportPointsToSummary(t *testing.T) {
	// `## 완료 보고` is the template's last section: no line follows it.
	report, ok := sbSection(read(t, skProgressTmpl), "## 완료 보고")
	if !ok {
		t.Fatalf("%s has no section %q", skProgressTmpl, "## 완료 보고")
	}
	sbMark(t, sbNoPlace, skProgressTmpl+" ## 완료 보고", report,
		"add only one line pointing to `## 검수 요약`, which holds the judgment",
		"When work stops, or for a goal started with an earlier Seal, below it, only judgment")
	sbMark(t, sbNoPlace, skProgressTmpl+" "+sbSummary, sbTmplSummary(t),
		"the condition each contributed to under `### 바뀐 것`",
		"assumed open decisions under `### 근거` and `### 직접 볼 것`",
		"`manual` conditions under `### 직접 볼 것`",
		"the expected-result grounds, the actual observation boundary, the test method and the review stages actually taken",
		"the review each required and the one followed) under `### 근거`",
		"the unobserved scope and substitute environments under `### 보지 않은 것`",
		"where the independent review results are under `### 근거`",
		"whether the head to be sent or merged is the completion record's commit, with the fact that the completion does not cover a difference unless `ha status` at that head still names that completion record, in the opening lines")
	sbMarkSkillTokens(t, sbNoPlace, append([]string{"## Completion report", "## 완료 보고", sbSummary, "manual", "ha status"},
		sbSummaryHeadings...)...)
}

// The PROGRESS template's reading rules keep the summary short.
func TestSealBundleReviewSummaryReadingRules(t *testing.T) {
	sbNeedSummary(t,
		"write in the user's language",
		"say what a record is, with its seq only as an aid",
		"keep each item short",
		"never copy check output or finding text; a finding is its ID, one line and its ending")
}

// The review summary applies from seal 0.1.24: the PROGRESS template says
// so, and the skill's documents keep the section and PROGRESS.md tokens of
// that boundary.
func TestSealBundleReviewSummaryEarlierGoals(t *testing.T) {
	sbNeedSummary(t, "Only for a goal started with seal 0.1.24 or later")
	requireSkillTokens(t, sealSkillDir, sbSummary, "PROGRESS.md")
}

// The PROGRESS template says the user starts reviewing a finished goal at
// the review summary; the skill links the review summary format.
func TestSealBundleReviewSummaryStartsReview(t *testing.T) {
	sbNeedSummary(t, "where the user starts reviewing a finished goal")
	requireSkillTokens(t, sealSkillDir, "link:references/bundle.md#review-summary")
}

// The PROGRESS template keeps the review summary of a reopened goal as it
// was, marked by one line; the skill's documents keep the reopen token.
func TestSealBundleReviewSummaryReopen(t *testing.T) {
	sbNeedSummary(t,
		"When a goal with a completion record is reopened with `reopen`",
		"the earlier content of this section stays unchanged until the next completion record",
		"one line at its top",
		"which completion record (seq and commit) it is as of",
		"that completion record is no longer valid",
		"rewritten after the next completion record",
		"A stop in between",
		"leaves the earlier content and that line as they are",
		"that line does not remain")
	requireSkillTokens(t, sealSkillDir, "reopen")
}

// The judgment items moved from item 2 of the completion report keep its
// qualifications in the PROGRESS template.
func TestSealBundleReviewSummaryMovedJudgment(t *testing.T) {
	sbNeedSummary(t,
		"The judgment items moved from item 2 of the completion report keep its qualifications",
		"the executor's statements are marked as claims",
		"facts confirmed by records are kept apart from the executor's interpretation",
		"the evidence's version and the scope that evidence covers")
}

// The two parts of `### 근거` and the opening lines are minimum lists in the
// PROGRESS template; the skill's documents keep the `### 근거` token.
func TestSealBundleReviewSummaryMinimumLists(t *testing.T) {
	sbNeedSummary(t,
		"The two parts of `### 근거` and the items of the opening lines are minimum lists",
		"the items the judgment-item paragraph assigns join them in the same place")
	requireSkillTokens(t, sealSkillDir, sbSummaryHeadings[1])
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

// The section helpers skip fenced code opened with ``` or ~~~, so a heading
// inside an example block is not a heading of the document.
func TestSealBundleHelpersSkipFences(t *testing.T) {
	text := "## A\n~~~markdown\n### fake\n## fake\n~~~\n### real\n```\n### also fake\n~~~\n```\n## B\n"
	if got := sbHeadings(text, 2, sbPrefixFence); strings.Join(got, "|") != "## A|## B" {
		t.Errorf("sbHeadings(2) = %q", got)
	}
	sec, ok := sbSection(text, "## A")
	if !ok {
		t.Fatalf("no section ## A")
	}
	if got := sbHeadings(sec, 3, sbPrefixFence); strings.Join(got, "|") != "### real" {
		t.Errorf("sbHeadings(3) = %q", got)
	}
}

// The REVIEW template's decision section and its two subheadings. Fences
// are tracked by sbRunFence.
const (
	sbDecisionsHeading = "## 중요한 미결정"
	sbUserHeading      = "### 사용자의 답"
	sbAssumeHeading    = "### 실행자 가정"
)

var (
	sbUserItems      = []string{"the decision, the user's quoted answer, and where that answer is recorded"}
	sbAssumeItems    = []string{"the assumption and its reason", "mark an assumption that reads a SPEC sentence"}
	sbEmptyDecisions = []string{"With no decision at all, write only one line `없음`", "Otherwise keep both subheadings and write `없음` under an empty one"}
	sbReviewHeadings = []string{"## 요구사항 대응", "## 불필요한 task", "## 문서 충돌", "## 중요한 미결정", "## 검증 방법과 환경", "## 독립 검토", "## 결론"}
)

// sbRunSection is the section under heading with fences tracked by
// sbRunFence.
func sbRunSection(text, heading string) (string, bool) {
	return sbSectionBy(text, heading, sbRunFence, " \t\r")
}

// sbNeedRunSection returns a section found by sbRunSection, reporting a
// missing section; a file that cannot be read fails the test.
func sbNeedRunSection(t *testing.T, path, heading string) string {
	t.Helper()
	sec, ok := sbRunSection(read(t, path), heading)
	if !ok {
		t.Errorf("%s lacks section %q", path, heading)
	}
	return sec
}

// sbHeadingOrder is true when each heading occurs exactly once as an
// unfenced whole line, the first before the second.
func sbHeadingOrder(text, first, second string) bool {
	firstLine, secondLine, open := -1, -1, ""
	for i, line := range strings.Split(text, "\n") {
		var fenced bool
		if open, fenced = sbRunFence(open, line); fenced {
			continue
		}
		switch strings.TrimRight(line, " \t\r") {
		case first:
			if firstLine >= 0 {
				return false
			}
			firstLine = i
		case second:
			if secondLine >= 0 {
				return false
			}
			secondLine = i
		}
	}
	return firstLine >= 0 && secondLine > firstLine
}

// The REVIEW template splits `## 중요한 미결정` into the user's answers and
// the executor's assumptions, in that order; the skill's documents name both
// subheadings and `없음`.
func TestSealBundleReviewDecisionsSplit(t *testing.T) {
	sec := sbNeedRunSection(t, skReviewTmpl, sbDecisionsHeading)
	user, _ := sbRunSection(sec, sbUserHeading)
	assume, _ := sbRunSection(sec, sbAssumeHeading)
	if !sbHeadingOrder(sec, sbUserHeading, sbAssumeHeading) {
		t.Errorf("%s must have each subheading once, with %q before %q inside %s", skReviewTmpl, sbUserHeading, sbAssumeHeading, sbDecisionsHeading)
	}
	sbNeed(t, skReviewTmpl+" "+sbUserHeading, user, sbUserItems...)
	sbNeed(t, skReviewTmpl+" "+sbAssumeHeading, assume, sbAssumeItems...)
	sbNeed(t, skReviewTmpl+" "+sbDecisionsHeading, sec, sbEmptyDecisions...)
	requireSkillTokens(t, sealSkillDir, sbUserHeading, sbAssumeHeading, "없음")
}

// The REVIEW template keeps its level-two headings and reads a kept scope or
// trust boundary as an assumption.
func TestSealBundleReviewHeadingsKept(t *testing.T) {
	text := read(t, skReviewTmpl)
	if got := sbHeadings(text, 2, sbRunFence); !slices.Equal(got, sbReviewHeadings) {
		t.Errorf("%s headings = %q, want %q", skReviewTmpl, got, sbReviewHeadings)
	}
	sec, ok := sbRunSection(text, sbDecisionsHeading)
	if !ok {
		t.Errorf("%s lacks %s", skReviewTmpl, sbDecisionsHeading)
	}
	sbNeed(t, skReviewTmpl+" "+sbDecisionsHeading, sec,
		"a reading that keeps a component's declared scope or trust boundary is such an assumption, not a question")
}

// A completed goal's PR description is built from the review summary, its
// items to inspect and its unobserved scope: the skill's documents keep
// their tokens.
func TestSealBundlePRDescription(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, sbSummary, sbSummaryHeadings[2], sbSummaryHeadings[3])
}

func TestSealBundleHelpersHeadingOrder(t *testing.T) {
	ordered := sbUserHeading + "\n\n" + sbAssumeHeading
	reversed := sbAssumeHeading + "\n\n" + sbUserHeading
	mention := "Use `" + sbUserHeading + "`, then `" + sbAssumeHeading + "`."
	for _, c := range []struct {
		name, body string
		want       bool
	}{
		{"ordered headings", ordered, true},
		{"prose mentions only", mention, false},
		{"prose before reversed headings", mention + "\n" + reversed, false},
		{"duplicate user heading", ordered + "\n" + sbUserHeading, false},
		{"duplicate assumption heading", ordered + "\n" + sbAssumeHeading, false},
		{"missing user heading", sbAssumeHeading, false},
		{"missing assumption heading", sbUserHeading, false},
		{"indented mentions", " " + sbUserHeading + "\n " + sbAssumeHeading, false},
		{"fenced headings only", "```md\n" + ordered + "\n```", false},
		{"fenced reverse before headings", "~~~md\n" + reversed + "\n~~~\n" + ordered, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := sbDecisionsHeading + "\n\n" + c.body
			if got := sbHeadingOrder(text, sbUserHeading, sbAssumeHeading); got != c.want {
				t.Errorf("heading order = %t, want %t", got, c.want)
			}
		})
	}
}

// The PROGRESS parts a rule can name: `## 현재`, `## 타임라인`, `## 계획 변경`
// and `## 막힘`. Rules are read in the template's section of the part,
// compared in lower case.
const (
	sbCurrent  = "## 현재"
	sbTimeline = "## 타임라인"
	sbPlanEdit = "## 계획 변경"
	sbBlocked  = "## 막힘"
)

var (
	sbKinds = []string{"`관측`", "`원인`", "`결정`", "`조치`", "`검증`", "`막힘`", "`재개`"}
	sbMarks = []string{"`가설`", "`확정`", "`기각`", "`예정`", "`반영`", "`대기`", "`통과`", "`실패`"}
)

// An sbRule is one token a PROGRESS part must hold.
type sbRule struct{ part, token string }

// sbPartTokens is a rule for each token of one part.
func sbPartTokens(part string, tokens ...string) []sbRule {
	var rules []sbRule
	for _, s := range tokens {
		rules = append(rules, sbRule{part, s})
	}
	return rules
}

// sbCheckParts reports each rule whose token the template's section of its
// part does not hold, compared in lower case; a missing section is reported
// once.
func sbCheckParts(t *testing.T, rules []sbRule) {
	t.Helper()
	sections := map[string]string{}
	for _, r := range rules {
		sec, seen := sections[r.part]
		if !seen {
			sec, _ = sbNeedSection(t, skProgressTmpl, r.part)
			sections[r.part] = sec
		}
		if want := strings.ToLower(r.token); !strings.Contains(strings.ToLower(sec), want) {
			t.Errorf("%s `%s` lacks %q", skProgressTmpl, r.part, want)
		}
	}
}

// `## 현재` is a short summary that opens with the `ha report` stamp, then
// the work in progress, the open problems, the next work and how to resume;
// nobody types a check time. The PROGRESS template says so, and the skill's
// documents keep the stamp's tokens and link the earlier forms.
func TestSealBundleProgressCurrentStamped(t *testing.T) {
	sbCheckParts(t, sbPartTokens(sbCurrent,
		"short, current summary",
		"`ha report <goal>`",
		"never type a check time yourself",
		"time zone",
		"record head",
		"seq and hash",
		"`none` before `ha start`",
		"completion record seq",
		"work in progress",
		"open problems",
		"next work",
		"how to resume",
		"restamp with a fresh `ha report` stamp whenever the record head changes",
		"compares the record head with `ha status`",
		"the same seq with another hash is another record",
		"the stamp's time is for reference only",
		"past events stay out; point to `## 타임라인`"))
	requireSkillTokens(t, sealSkillDir, sbCurrent, "ha report <goal>", "none", "ha start", "ha status", sbTimeline, "skill:",
		"link:references/earlier-goals.md")
}

// The optional `## 타임라인` holds append-only entries with a time (with time
// zone), the record head, a kind and a summary; bodies mark causes, actions
// and verification, and point to evidence instead of copying check output.
// The PROGRESS template says so, and the skill's documents keep the
// timeline's section, heading form, kinds and marks.
func TestSealBundleProgressTimelineDefined(t *testing.T) {
	rules := sbPartTokens(sbTimeline,
		"optional",
		"entries are claims",
		" · seq <record head> — <kind> · <one-line summary>",
		"time zone",
		"never invent a time you do not know",
		"`seq none`",
		"an entry about a check run uses that run's seq",
		"point to evidence by AC, commit, and seq",
		"never copy check output",
		"append only",
		"never edit an earlier entry",
		"what was rejected and on what grounds",
		"may be one entry")
	rules = append(rules, sbPartTokens(sbTimeline, sbKinds...)...)
	rules = append(rules, sbPartTokens(sbTimeline, sbMarks...)...)
	sbCheckParts(t, rules)
	tokens := []string{"### `## 타임라인`", "### <time> · seq <record head> — <kind> · <one-line summary>", "seq none"}
	tokens = append(tokens, skUnquote(sbKinds)...)
	requireSkillTokens(t, sealSkillDir, append(tokens, skUnquote(sbMarks)...)...)
}

// Only events that changed a judgment or the next action are recorded; the
// length figures are editing guidance, not limits. The PROGRESS template
// says so; the skill's documents keep the tokens those rules name.
func TestSealBundleProgressTimelineWhatToRecord(t *testing.T) {
	rules := sbPartTokens(sbTimeline,
		"change a judgment or the next action",
		"a new problem",
		"a cause confirmed or changed",
		"a change of approach",
		"an important verification result",
		"blocks and resumes",
		"every resume counts",
		"a new session taking the goal up",
		"a cleared block",
		"an answer to a `needs_user` question",
		"where `## 현재` differed from the actual state",
		"routine edits and reads",
		"repeated runs of the same failure",
		"progress reports with no change",
		"an entry runs about 3 to 6 lines",
		"`## 현재` about 5 to 8 lines",
		"editing guidance, not a limit",
		"a complex cause analysis may run longer")
	rules = append(rules, sbRule{sbCurrent, "5 to 8 lines"})
	sbCheckParts(t, rules)
	requireSkillTokens(t, sealSkillDir, "needs_user", sbCurrent)
}

// The plan-change and block tables keep their names and columns; times carry
// a time zone, one row is one event, and the detail of an event that also has
// a timeline entry lives only in that entry. The PROGRESS template says so,
// and the skill's documents keep both sections, their columns and `시각`.
func TestSealBundleProgressTablesPointToTimeline(t *testing.T) {
	var rules []sbRule
	for _, p := range []string{sbPlanEdit, sbBlocked} {
		rules = append(rules, sbPartTokens(p,
			"`시각` carries a time zone",
			"one row is one event",
			"when the event also has a timeline entry",
			"the detail lives only in that entry",
			"one-line summary",
			"points to it by the entry's time")...)
	}
	sbCheckParts(t, rules)
	requireSkillTokens(t, sealSkillDir, sbPlanEdit, sbBlocked, "시각",
		"시각 \\| 변경 \\| 이유 \\| 영향 \\| 다시 검토한 것", "시각 \\| 원인 종류 \\| 내용 \\| 해소 조건")
	text := read(t, skProgressTmpl)
	for _, table := range []string{"## 계획 변경\n| 시각 | 변경 | 이유 | 영향 | 다시 검토한 것 |", "## 막힘\n| 시각 | 원인 종류 | 내용 | 해소 조건 |"} {
		if !strings.Contains(text, table) {
			t.Errorf("%s lacks %q", skProgressTmpl, table)
		}
	}
	// A line right after a table is a row of it; the template's table rules
	// stand apart from the tables.
	for _, p := range []string{sbPlanEdit, sbBlocked} {
		if sec, _ := sbNeedSection(t, skProgressTmpl, p); !strings.Contains(sec, "| --- |\n\n<rows: ") {
			t.Errorf("%s `%s` has no table rules apart from the table", skProgressTmpl, p)
		}
	}
}

// At completion, `## 현재` says it is as of that completion record and where
// the later events are found; it never writes them as work in progress.
func TestSealBundleProgressCurrentAtCompletion(t *testing.T) {
	sbCheckParts(t, sbPartTokens(sbCurrent,
		"as of that completion record",
		"past the bundle's last commit",
		"merge",
		"review replies",
		"checks after the merge",
		"where to find",
		"never as work in progress"))
	requireSkillTokens(t, sealSkillDir, sbCurrent)
}

// The skill's documents keep the tokens of `## 현재` and its ha report
// stamp, of `## 타임라인` with every kind and mark, and of the `재개` entry,
// and the skill links the reference for earlier goals.
func TestSealBundleSkillKeepsTimeline(t *testing.T) {
	tokens := []string{sbCurrent, "ha report", sbTimeline, "재개", "link:references/earlier-goals.md"}
	tokens = append(tokens, skUnquote(sbKinds)...)
	requireSkillTokens(t, sealSkillDir, append(tokens, skUnquote(sbMarks)...)...)
}

// The invariant places the REVIEW template's `## 독립 검토` lists, read by
// sbPrefixSection and checked for exact text.
var sbInvariantPlaces = []string{
	"from the plan round to the cumulative round",
	"the places that read or keep it: each place's repository path, and the name used there when it differs from the change's name",
	"places that keep the same invariant under another name, not only those found by name",
	"`없음` when the change alters no invariant",
	"one `--question` per place, asking whether that place still keeps its invariant at head",
	"the list adds places for reviewers to look at and never narrows what they read",
}

type sbPlace struct {
	label, text string
	want        []string
}

// sbCheckPlaces reports an empty place and every phrase a place lacks.
func sbCheckPlaces(t *testing.T, places []sbPlace) {
	t.Helper()
	for _, p := range places {
		if strings.TrimSpace(p.text) == "" {
			t.Errorf("%s is missing", p.label)
			continue
		}
		for _, w := range p.want {
			if !strings.Contains(p.text, w) {
				t.Errorf("%s does not say %q", p.label, w)
			}
		}
	}
}

// sbReviewPlaces is the REVIEW template's `## 독립 검토`.
func sbReviewPlaces(t *testing.T) string {
	t.Helper()
	return sbPrefixSection(read(t, skReviewTmpl), "## 독립 검토")
}

// The decisions passed to review rounds keep their tokens: the decision and
// quote flags, the notes and REVIEW.md items they come from, what is never
// passed, and the skill's link to them.
func TestSealBundleDecisionsToPass(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "link:references/bundle.md#questions-for-the-user", "--decision", "--decision-quote",
		sbDecisionsHeading, "confirm", "input", "requirement-question", "reopen")
}

// The REVIEW template's `## 독립 검토` lists, for each changed invariant, the
// places that read or keep it and passes one question per place to every
// round; the skill's documents keep the question flag and `없음`.
func TestSealBundleInvariantPlaces(t *testing.T) {
	sbCheckPlaces(t, []sbPlace{{skReviewTmpl + " ## 독립 검토", sbReviewPlaces(t), sbInvariantPlaces}})
	requireSkillTokens(t, sealSkillDir, "--question", "없음")
}

// The REVIEW template applies the list of places from seal 0.1.25; the
// skill's documents keep the REVIEW.md token of that boundary.
func TestSealBundleReviewRoundsScope(t *testing.T) {
	sbCheckPlaces(t, []sbPlace{{skReviewTmpl + " ## 독립 검토", sbReviewPlaces(t), []string{"seal 0.1.25 or later"}}})
	requireSkillTokens(t, sealSkillDir, "REVIEW.md")
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
