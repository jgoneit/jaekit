package plugins

// Checks of the goal check-cost for Seal, AGENTS.md and the documents. They
// observe fixed tokens in fixed sections of files of the tree under test
// (AGENTS.md "New checks"), run this tree's ha and Seal's helper on
// synthetic repositories, and run the two repository checks of AC-9 on
// synthetic copies of AGENTS.md. They never read a commit, a ref, history or
// another goal's bundle.
//
// Each test reads every file it needs first. A file that is missing or
// cannot be read or decoded, a tool that cannot start (go, git, python3, the
// built ha) and a synthetic repository that cannot be made are a broken
// setup (setup:), never the observed property. The opening line of an
// assertion's failure message names the property it observes
// (jaekit:violation=<id>).
//
// Tokens. A section runs from its heading to the next heading of the same or
// a higher level outside fenced code. "One line" means one line of that
// section (a paragraph or a list item; these files do not wrap lines).
// Tokens match ignoring case and reading runs of white space as one space;
// backquotes and other characters match as written.
//
// AC-1  SKILL.md: "`ha start <goal> --rules run-rules/7 --request".
//       core-requirements.json: rules exactly ["run-rules/7"], keeping the
//       formats and features Seal 0.1.26 declared.
//       Seal's start form with this tree's ha records run-rules/7; Seal's
//       helper accepts this ha under run-rules/7 and refuses a Core whose
//       capabilities lack it; an installed ha 0.1.8, when one is on PATH,
//       refuses Seal's start form and writes no record.
// AC-6  references/bundle.md `## REVIEW.md`, one line: "`run-rules/7`",
//       "requires a baseline", "bug fixes", "regressions", "X does not
//       happen", "partly met at base", "`baseline` column", "`요구`",
//       "plan round".
//       references/bundle.md `## Check inputs`, one line: "marked change
//       condition", "recorded once its check files are settled", "not after
//       every check fix", "working habit, not a record".
//       references/ha.md `## Structured change results`, one line: "entry
//       point that also exists at the base commit", "applies only to marked
//       conditions", "unmarked condition".
//       references/bundle.md `## Review summary`, one line: "`### 보지 않은
//       것`", "change conditions met without a baseline".
// AC-7  references/bundle.md `## Questions for the user`, one line:
//       "`run-rules/7`", "before any baseline", "before the goal has any
//       `ha check` record", "current check or baseline", "`ha doctor`",
//       "plan round". SKILL.md keeps "run `ha doctor <goal>` before any
//       baseline", "Before any baseline, ask in one message", "comes before
//       any baseline and also reviews them" and its link
//       "references/bundle.md#questions-for-the-user".
// AC-8  references/bundle.md `## Check inputs`, one line each:
//       "input failure", "missing target file", "corrupt input", "tool that
//       cannot start", "signal", "never a pass";
//       "require no baseline", "maintain", "`EX-n`", "task commands",
//       "unmarked change conditions", "not a check defect", "until the input
//       changes";
//       "dedicated producer of a marked change condition", "as a violation",
//       "result contract allows", "`REVIEW.md`", "before the plan round",
//       "before any review round";
//       "runner adapters classify input failures themselves".
//       references/bundle.md `## Assurance policy and counterexamples` keeps
//       "Uninterpretable input is not successful evidence or an intended
//       baseline violation." and "Preserve unobserved, environment error,
//       skip, and observed violation separately."
// AC-9  AGENTS.md `## Checks and review`, the item starting "- **Input
//       failures.**": "producer of a change condition that requires a
//       baseline", "before the plan round", "missing target file", "corrupt
//       input", "a tool that cannot start", "termination by a signal",
//       "every other check", "from counting as a pass", "does not depend on
//       the rule version", "earlier rule", "every change condition".
//       AGENTS.md `### For reviewers`, the item holding "counts an input
//       failure": "as a violation is a defect only when", "producer of a
//       condition that requires a baseline", "`missing: violation`", "as a
//       pass is a defect".
//       No line of AGENTS.md `## Checks and review` holds the earlier
//       unconditional wordings "For each producer, state before the plan
//       round", "is never counted as one" or "as a violation is a defect,
//       unless".
//       tools/workflow-checks/test_review_rules.py and
//       test_review_rules_consistency.py pass on this tree; on a copy of
//       AGENTS.md whose Input failures item is the earlier one,
//       test_review_rules.py fails with a "[violation:" message; on a copy
//       whose For reviewers item is the earlier one, both fail so.
// AC-11 references/bundle.md `## Check inputs`, one line each:
//       "freeze", "earliest `plan` round", "`strict`", "earliest
//       `candidate` round", "`standard`";
//       "frozen", "declarations", "producers", "tests", "helpers",
//       "settings";
//       "after the freeze", "commit", "`ha note <goal> check-change
//       checks:<path> --reason", "finding ID";
//       "next code round", "changed check files", "plan-delta";
//       "before the freeze", "no reason", "may still be fixed";
//       "`ha note <goal> review --gate", "right after the freeze round",
//       "condition table".
// AC-17 The formula token is "표시한 필수 change × 2 + 표시하지 않은 필수
//       change + 필수 maintain" in every language. The mark tokens are
//       "`baseline` 열" (ko) or "`baseline` column" (en) with "`요구`"; the
//       freeze line is one line with "동결", "첫 `plan` 라운드", "첫
//       `candidate` 라운드" (ko) or "freeze", "earliest `plan` round",
//       "earliest `candidate` round" (en).
//       contracts/run-record.md, .en.md: a section whose heading holds
//       "run-rules/7" with "run-rules/6", the formula, the mark tokens,
//       "`baseline_required`", "`check_freeze`", "`check-change`" and the
//       freeze line; a table row of §4.6 with "run-rules/7".
//       contracts/bundle.md, .en.md: §2.1 with "| ID | 종류 | 검증 명령 |
//       검사 경로 | Task | baseline |", "`요구`" and "run-rules/7".
//       contracts/core-capabilities.md, .en.md: a section whose heading
//       holds "run-rules/7" with "supported_rules" and "--rules
//       run-rules/7".
//       guides/USAGE.md, .en.md, guides/OPERATIONS.md, .en.md: a section
//       whose heading holds "run-rules/7" with the formula, the mark tokens,
//       the freeze line and three lines of the input-failure policy:
//       ko "입력 실패", "대상 파일 없음", "손상된 입력", "시작하지 못하는
//       도구", "신호 종료", "통과가 아니다"; "baseline을 요구하지 않는 검사",
//       "위반, 오류, 판정 불가", "검사의 결함이 아니다"; "baseline을 요구하는
//       change 조건의 전용 생산자", "위반으로 보고하지 않는다", "허용하는
//       경우";
//       en "input failure", "missing target file", "corrupt input", "tool
//       that cannot start", "signal", "never a pass"; "require no
//       baseline", "violation, error or inconclusive", "not a check
//       defect"; "dedicated producer of a marked change condition", "reports
//       none of them as a violation", "result contract allows".
//       The guides' earlier sentences that ask every change condition for a
//       baseline need not stay. Each sentence of a guide that still holds
//       one of them says its rule scope in that same sentence, backquotes
//       aside: "run-rules/6까지" (ko) or "up to run-rules/6" (en); or
//       "run-rules/7" together with "표시한 change 조건" (ko) or "marked
//       change condition" (en, not preceded by a letter, so "unmarked" does
//       not count). The rule name alone does not count. A sentence ends at
//       ".", "!" or "?" followed by a space or the line end; "e.g." and
//       "i.e." end none. A guide that drops them is checked only through the
//       run-rules/7 section's tokens above. The earlier sentences: USAGE.md
//       "필수 change가 34개인 목표는 최소량 68회"; USAGE.en.md "a goal with 34
//       required change conditions needs at least 68 runs"; OPERATIONS.md
//       "**의도한 위반**을 보여야", "수정 전·후 각 한 번씩 최소 68회";
//       OPERATIONS.en.md "must show the **intended violation** before the
//       change", "need at least 68 runs: one before and one after the change
//       for each".
//       SKILL.md: "--rules run-rules/7". references/ha.md `## run-rules/7`:
//       "run-rules/6", the formula, the en mark tokens, the en freeze line,
//       "`ha note <goal> check-change". references/bundle.md "### `## 조건표`":
//       the header row above, "`요구`", "`run-rules/7`".
//       assets/templates/PLAN.md `## 조건표`, one line: "`run-rules/7`",
//       "`baseline` column", "`요구`" (prose: the template's table header
//       stays the one examples/empty-input/PLAN.md has).
//       assets/templates/REVIEW.md `## 검증 방법과 환경`, one line:
//       "`run-rules/7`", "requires a baseline", "`요구`".
// AC-18 "0.1.29" in plugins/seal/.claude-plugin/plugin.json,
//       plugins/seal/.codex-plugin/plugin.json, the seal entry of
//       .claude-plugin/marketplace.json and the skill front matter
//       (`  version: "0.1.29"`); in references/ha.md the line holding
//       "Seal 0.1.29" holds after it "`run-rules/7`", "needs a Core that
//       declares `run-rules/7`", "baseline" and "freeze".
//
// SKILL.md, references/bundle.md and references/ha.md never hold the
// ordering words plugins_test.go TestSealSkillPrescribesNoOrder refuses
// ("first", "then", …), so their tokens say "earliest" and "any".

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
	ckSealVersion  = "0.1.29"
	ckNewRules     = "run-rules/7"
	ckOldRules     = "run-rules/6"
	ckSkillDir     = "seal/skills/seal"
	ckSkill        = ckSkillDir + "/SKILL.md"
	ckRequirements = ckSkillDir + "/core-requirements.json"
	ckBundleRef    = ckSkillDir + "/references/bundle.md"
	ckHaRef        = ckSkillDir + "/references/ha.md"
	ckHelper       = ckSkillDir + "/scripts/check_core.py"
	ckPlanTemplate = ckSkillDir + "/assets/templates/PLAN.md"
	ckRevTemplate  = ckSkillDir + "/assets/templates/REVIEW.md"
	ckAgents       = "../AGENTS.md"
	ckRules        = "../tools/workflow-checks/test_review_rules.py"
	ckConsistency  = "../tools/workflow-checks/test_review_rules_consistency.py"
	ckPRTemplate   = "../.github/pull_request_template.md"
	ckGoal         = "docs/specs/demo"

	ckFormula   = "표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain"
	ckHeaderRow = "| ID | 종류 | 검증 명령 | 검사 경로 | Task | baseline |"
)

// The formats and features Seal 0.1.26 declared, which the run-rules/7
// declaration keeps.
var (
	ckFormats = map[string][]string{
		"criteria":          {"legacy", "nested/1"},
		"check_declaration": {"check-declaration/v1"},
		"check_result":      {"check-result/v1"},
		"check_evidence":    {"check-evidence/v1"},
		"assurance_tier":    {"light", "standard", "strict"},
		"core_requirements": {"core-requirements/v1"},
		"review_gate":       {"review-gate/v1"},
	}
	ckFeatures = []string{"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1",
		"budget-change/v1", "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1",
		"command-time-limits/v1", "report/v1", "doctor/v1", "auto-budget/v1", "core-requirements/v1", "review-gate/v1"}
)

// The AGENTS.md items AC-9 replaces, as they read before this goal. A
// synthetic copy of AGENTS.md with one of them back in place must fail the
// repository checks.
const (
	ckEarlierInputFailures = "- **Input failures.** For each producer, state before the plan round which result (violation, error or inconclusive) follows from a missing target file, corrupt input, a tool that cannot start, and termination by a signal. A case that the check result contract does not declare as a violation is never counted as one."
	ckEarlierReviewers     = "- A check that counts an input failure (a missing file, corrupt input, a tool that cannot start, a signal) as a violation is a defect, unless the check declares that case as a violation where the check result contract allows it, such as `missing: violation`."
)

// ckEarlierPhrases are the unconditional wordings of those items that no
// line of AGENTS.md `## Checks and review` holds any longer.
var ckEarlierPhrases = []string{"For each producer, state before the plan round", "is never counted as one", "as a violation is a defect, unless"}

// ckRead reads the files a test needs; any that cannot be read is a broken
// setup.
func ckRead(t *testing.T, paths ...string) map[string]string {
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

func ckDecode(t *testing.T, path, text string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), value); err != nil {
		t.Fatalf("setup: %s is not JSON: %v", path, err)
	}
}

var ckSpace = regexp.MustCompile(`\s+`)

// ckFlat reads runs of white space as one space and ignores case.
func ckFlat(s string) string {
	return strings.ToLower(strings.TrimSpace(ckSpace.ReplaceAllString(s, " ")))
}

// ckPlain is ckFlat without backquotes.
func ckPlain(s string) string { return ckFlat(strings.ReplaceAll(s, "`", "")) }

// ckSentenceEnd is the end of a sentence: ".", "!" or "?" followed by white
// space or the end of the text.
var ckSentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)

// ckSentences splits one line into its sentences; "e.g." and "i.e." end
// none.
func ckSentences(line string) []string {
	var out []string
	start := 0
	for _, m := range ckSentenceEnd.FindAllStringIndex(line, -1) {
		end := m[0] + 1
		if lower := strings.ToLower(line[start:end]); strings.HasSuffix(lower, "e.g.") || strings.HasSuffix(lower, "i.e.") {
			continue
		}
		out = append(out, strings.TrimSpace(line[start:end]))
		start = end
	}
	if rest := strings.TrimSpace(line[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

// ckWord reports whether text holds token where no ASCII letter comes right
// before it ("marked", not "unmarked").
func ckWord(text, token string) bool {
	for from := 0; ; {
		i := strings.Index(text[from:], token)
		if i < 0 {
			return false
		}
		at := from + i
		if at == 0 || !(text[at-1] >= 'a' && text[at-1] <= 'z' || text[at-1] >= 'A' && text[at-1] <= 'Z') {
			return true
		}
		from = at + 1
	}
}

// ckLevel is the heading level of a Markdown line, or 0.
func ckLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// ckSection returns the lines of the first section whose heading text
// (without the leading #s) matches, up to the next heading of the same or a
// higher level outside fenced code, and whether it exists.
func ckSection(text string, match func(heading string) bool) ([]string, bool) {
	lines := strings.Split(text, "\n")
	start, level, fence := -1, 0, ""
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			m := trimmed[:3]
			switch fence {
			case "":
				fence = m
			case m:
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		lv := ckLevel(l)
		if lv == 0 {
			continue
		}
		if start >= 0 && lv <= level {
			return lines[start:i], true
		}
		if start < 0 && match(strings.TrimSpace(strings.TrimRight(l[lv:], " \t\r"))) {
			start, level = i, lv
		}
	}
	if start < 0 {
		return nil, false
	}
	return lines[start:], true
}

func ckHeading(want string) func(string) bool { return func(h string) bool { return h == want } }

func ckHolding(token string) func(string) bool {
	return func(h string) bool { return strings.Contains(h, token) }
}

func ckNumbered(number string) func(string) bool {
	return func(h string) bool { return strings.HasPrefix(h, number+" ") }
}

// ckCheck collects what the documents lack, one entry per missing section,
// token or line, each named with its file and section.
type ckCheck struct{ missing []string }

func (c *ckCheck) lack(format string, args ...any) {
	c.missing = append(c.missing, fmt.Sprintf(format, args...))
}

// section returns the section, recording its absence.
func (c *ckCheck) section(path, text, name string, match func(string) bool) ([]string, bool) {
	lines, ok := ckSection(text, match)
	if !ok {
		c.lack("%s: no section %s", path, name)
	}
	return lines, ok
}

// tokens records each token the section does not hold.
func (c *ckCheck) tokens(path, name string, lines []string, tokens ...string) {
	body := ckFlat(strings.Join(lines, "\n"))
	for _, tok := range tokens {
		if !strings.Contains(body, ckFlat(tok)) {
			c.lack("%s %s: %q", path, name, tok)
		}
	}
}

// line records a missing line of the section that holds every token.
func (c *ckCheck) line(path, name string, lines []string, tokens ...string) {
	for _, l := range lines {
		flat, all := ckFlat(l), true
		for _, tok := range tokens {
			all = all && strings.Contains(flat, ckFlat(tok))
		}
		if all {
			return
		}
	}
	c.lack("%s %s: no line holding all of %q", path, name, tokens)
}

// in checks tokens and lines of one named section of a file.
func (c *ckCheck) in(path, text, name string, match func(string) bool, tokens []string, lines ...[]string) {
	sec, ok := c.section(path, text, name, match)
	if !ok {
		return
	}
	c.tokens(path, name, sec, tokens...)
	for _, l := range lines {
		c.line(path, name, sec, l...)
	}
}

func (c *ckCheck) report(t *testing.T, violation string) {
	t.Helper()
	if len(c.missing) > 0 {
		t.Errorf("jaekit:violation=%s missing: %s", violation, strings.Join(c.missing, "; "))
	}
}

// ckRun runs a command. A command that cannot start or that a signal ended
// is a broken setup; otherwise it returns the combined output and exit code.
func ckRun(t *testing.T, dir string, env []string, name string, args ...string) (string, int) {
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

func ckMust(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	out, code := ckRun(t, dir, env, name, args...)
	if code != 0 {
		t.Fatalf("setup: %s %v exit %d: %s", name, args, code, out)
	}
	return out
}

func ckEnv(t *testing.T) []string {
	return append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
}

func ckWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// ckRepo makes a committed repository outside the tree with one synthetic
// goal whose only condition runs git.
func ckRepo(t *testing.T, env []string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"src/value.txt": "ok\n",
		ckGoal + "/SPEC.md": "# Demo\n\nStatus: Ready\n\n## 목표\n\n`src/value.txt` says ok.\n\n## 범위와 비목표\n\n- 범위: `src/**`\n\n" +
			"## Acceptance Criteria\n- **AC-1** `git --version` exits 0.\n\n## Open Decisions\n\n없음\n",
		ckGoal + "/PLAN.md": "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
			"| AC-1 | maintain | `git --version` | — | T001 |\n\n## 범위\n- 바꿀 수 있는 경로: `src/**` (SPEC: 범위와 비목표)\n" +
			"- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n" +
			"| --- | --- | --- | --- | --- | --- | --- |\n| T001 | git runs | AC-1 | — | — | 예 | — |\n\n" +
			"## 예산\n- 검증 실행: 10\n- 경과 시간: 1시간\n- 비용: 미관측\n",
		ckGoal + "/REVIEW.md":   "# REVIEW — demo\n\n## 결론\nready\n",
		ckGoal + "/PROGRESS.md": "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | |\n",
	} {
		ckWrite(t, filepath.Join(dir, filepath.FromSlash(name)), body, 0o644)
	}
	ckMust(t, dir, env, "git", "init", "-q")
	ckMust(t, dir, env, "git", "add", "-A")
	ckMust(t, dir, env, "git", "-c", "commit.gpgsign=false", "commit", "-qm", "base")
	return dir
}

func ckRecordPath(dir string) string {
	return filepath.Join(dir, filepath.FromSlash(ckGoal), "runs.jsonl")
}

// ckStartRules is the rule of a goal's start line.
func ckStartRules(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(ckRecordPath(dir))
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

// ckReport is what check_core.py prints.
type ckReport struct {
	Compatible bool     `json:"compatible"`
	Reason     string   `json:"reason"`
	Rules      *string  `json:"rules"`
	Missing    []string `json:"missing"`
}

// ckHelperRun runs Seal's compatibility helper. An exit other than 0
// (compatible) and 2 (not), output that is not its report, and a Core it
// could not query are a broken setup.
func ckHelperRun(t *testing.T, dir string, env []string, args ...string) (ckReport, int) {
	t.Helper()
	helper, err := filepath.Abs(ckHelper)
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
		t.Fatalf("setup: python3 %s %v: %v", ckHelper, args, err)
	}
	var r ckReport
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

func ckRulesOf(r ckReport) string {
	if r.Rules == nil {
		return "(none)"
	}
	return *r.Rules
}

// ckHa018 is the first `ha` on PATH whose --version prints "ha 0.1.8", or "".
func ckHa018() string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "ha")
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			continue
		}
		out, err := exec.Command(p, "--version").Output()
		if err == nil && strings.TrimSpace(string(out)) == "ha 0.1.8" {
			return p
		}
	}
	return ""
}

// ckStartForm is Seal's start of the synthetic goal.
func ckStartForm(skill string) []string {
	return []string{"start", ckGoal, "--rules", ckNewRules, "--request", "implement the synthetic goal", "--skill", skill, "--check-skill"}
}

// AC-1: Seal starts new goals under run-rules/7 and declares it, its helper
// knows the rule, and a Core without it refuses Seal's start with no record.
func TestCheckCostAC1_SealStartsUnderRulesV7(t *testing.T) {
	const v = "seal-not-on-rules-v7"
	files := ckRead(t, ckRequirements, ckSkill, ckHelper)
	var decl struct {
		Rules    []string            `json:"rules"`
		Formats  map[string][]string `json:"formats"`
		Features []string            `json:"features"`
	}
	ckDecode(t, ckRequirements, files[ckRequirements], &decl)

	// Inputs: this tree's ha, a copy of the skill folder, a goal to start,
	// and a fake Core that answers this ha's capabilities without the rule.
	env := ckEnv(t)
	bin := t.TempDir()
	ha := filepath.Join(bin, "ha")
	ckMust(t, "..", os.Environ(), "go", "build", "-o", ha, "./cmd/ha")
	skillCopy := filepath.Join(t.TempDir(), "seal")
	if err := os.CopyFS(skillCopy, os.DirFS(ckSkillDir)); err != nil {
		t.Fatalf("setup: copying the skill folder: %v", err)
	}
	skill := filepath.Join(skillCopy, "SKILL.md")
	fresh := ckRepo(t, env)
	var caps map[string]any
	capsText := ckMust(t, bin, env, ha, "capabilities", "--format", "json")
	ckDecode(t, "ha capabilities --format json", capsText, &caps)
	supported, ok := caps["supported_rules"].([]any)
	if !ok {
		t.Fatalf("setup: ha capabilities has no supported_rules: %s", capsText)
	}
	without := []any{}
	for _, r := range supported {
		if r != ckNewRules {
			without = append(without, r)
		}
	}
	caps["supported_rules"] = without
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	ckWrite(t, filepath.Join(bin, "capabilities.json"), string(data)+"\n", 0o644)
	fake := filepath.Join(bin, "ha-without-rules-7")
	ckWrite(t, fake, "#!/bin/sh\nexec cat \"$(dirname \"$0\")/capabilities.json\"\n", 0o755)
	if out := ckMust(t, bin, env, fake, "capabilities", "--format", "json"); strings.TrimSpace(out) != string(data) {
		t.Fatalf("setup: the fake Core printed %q", out)
	}

	// The declaration names the new rule and keeps the earlier formats and
	// features.
	if !slices.Equal(decl.Rules, []string{ckNewRules}) {
		t.Errorf("jaekit:violation=%s %s declares rules %v, want [%s]", v, ckRequirements, decl.Rules, ckNewRules)
	}
	var dropped []string
	for _, key := range []string{"criteria", "check_declaration", "check_result", "check_evidence", "assurance_tier", "core_requirements", "review_gate"} {
		for _, value := range ckFormats[key] {
			if !slices.Contains(decl.Formats[key], value) {
				dropped = append(dropped, "formats."+key+" "+value)
			}
		}
	}
	for _, feature := range ckFeatures {
		if !slices.Contains(decl.Features, feature) {
			dropped = append(dropped, "features "+feature)
		}
	}
	if len(dropped) > 0 {
		t.Errorf("jaekit:violation=%s %s no longer declares %s", v, ckRequirements, strings.Join(dropped, ", "))
	}

	// The skill's start line.
	if !strings.Contains(files[ckSkill], "`ha start <goal> --rules "+ckNewRules+" --request") {
		t.Errorf("jaekit:violation=%s %s does not start with `ha start <goal> --rules %s --request`", v, ckSkill, ckNewRules)
	}

	// Seal's start form, with its requirements checked, records run-rules/7.
	out, code := ckRun(t, fresh, env, ha, ckStartForm(skill)...)
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	started := false
	switch {
	case code == 64 || code == 65:
		t.Errorf("jaekit:violation=%s this tree's ha refuses Seal's start form under %s: exit %d: %s", v, ckNewRules, code, first)
	case code != 0:
		t.Fatalf("setup: ha start --check-skill exit %d: %s", code, out)
	default:
		if got := ckStartRules(t, fresh); got != ckNewRules {
			t.Errorf("jaekit:violation=%s Seal's start recorded rules %q, want %q", v, got, ckNewRules)
		} else {
			started = true
		}
	}

	// Seal's helper accepts this tree's ha under run-rules/7 and refuses a
	// Core without it.
	if r, code := ckHelperRun(t, bin, env, "--ha", ha, "--operation", "start"); code != 0 || !r.Compatible || ckRulesOf(r) != ckNewRules {
		t.Errorf("jaekit:violation=%s check_core.py --operation start with this tree's ha: exit %d, compatible %v, rules %s, reason %s; want 0 with %s",
			v, code, r.Compatible, ckRulesOf(r), r.Reason, ckNewRules)
	}
	r, code := ckHelperRun(t, bin, env, "--ha", fake, "--operation", "start")
	named := slices.ContainsFunc(r.Missing, func(m string) bool { return strings.Contains(m, ckNewRules) })
	if code != 2 || r.Compatible || !named {
		t.Errorf("jaekit:violation=%s check_core.py --operation start with a Core without %s: exit %d, compatible %v, missing %v; want 2 naming %s",
			v, ckNewRules, code, r.Compatible, r.Missing, ckNewRules)
	}
	if started {
		for _, op := range []string{"resume", "check", "budget"} {
			if r, code := ckHelperRun(t, fresh, env, "--ha", ha, "--operation", op, "--goal", ckGoal); code != 0 || !r.Compatible || ckRulesOf(r) != ckNewRules {
				t.Errorf("jaekit:violation=%s check_core.py --operation %s of the %s goal Seal started: exit %d, compatible %v, rules %s, reason %s, missing %v",
					v, op, ckNewRules, code, r.Compatible, ckRulesOf(r), r.Reason, r.Missing)
			}
		}
	}

	// An installed Core that does not know the rule refuses Seal's start
	// form and writes no record. Without one on PATH the property is not
	// observed here, and the test is skipped when nothing else failed.
	old := ckHa018()
	if old == "" {
		if !t.Failed() {
			t.Skip("no ha 0.1.8 on PATH: the refusal of a Core without " + ckNewRules + " was not observed")
		}
		return
	}
	other := ckRepo(t, env)
	out, code = ckRun(t, other, env, old, ckStartForm(skill)...)
	if _, err := os.Stat(ckRecordPath(other)); code == 0 || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("jaekit:violation=%s ha 0.1.8 ran Seal's start form: exit %d, record present %v: %s", v, code, err == nil, strings.TrimSpace(out))
	}
}

// AC-6: Seal's guidance on which change conditions require a baseline, when
// the baseline is recorded, where the base-commit entry point applies and
// what the review summary leaves unobserved.
func TestCheckCostAC6_BaselineGuidance(t *testing.T) {
	const v = "seal-baseline-guidance-missing"
	files := ckRead(t, ckBundleRef, ckHaRef, ckSkill)
	var c ckCheck
	b, h := files[ckBundleRef], files[ckHaRef]
	c.in(ckBundleRef, b, "## REVIEW.md", ckHeading("REVIEW.md"), nil,
		[]string{"`run-rules/7`", "requires a baseline", "bug fixes", "regressions", "X does not happen", "partly met at base",
			"`baseline` column", "`요구`", "plan round"})
	c.in(ckBundleRef, b, "## Check inputs", ckHeading("Check inputs"), nil,
		[]string{"marked change condition", "recorded once its check files are settled", "not after every check fix", "working habit, not a record"})
	c.in(ckHaRef, h, "## Structured change results", ckHeading("Structured change results"), nil,
		[]string{"entry point that also exists at the base commit", "applies only to marked conditions", "unmarked condition"})
	c.in(ckBundleRef, b, "## Review summary", ckHeading("Review summary"), nil,
		[]string{"`### 보지 않은 것`", "change conditions met without a baseline"})
	c.tokens(ckSkill, "(the links to those sections)", []string{files[ckSkill]}, "](references/bundle.md)", "](references/bundle.md#check-inputs)",
		"](references/bundle.md#review-summary)", "](references/ha.md#structured-change-results)")
	c.report(t, v)
}

// AC-7: under run-rules/7 "before any baseline" means before the goal's first
// ha check record, in the section the skill's sentence links; the skill's
// sentences stay.
func TestCheckCostAC7_FirstCheckPoint(t *testing.T) {
	const v = "seal-first-check-point-missing"
	files := ckRead(t, ckBundleRef, ckSkill)
	var c ckCheck
	c.in(ckBundleRef, files[ckBundleRef], "## Questions for the user", ckHeading("Questions for the user"), nil,
		[]string{"`run-rules/7`", "before any baseline", "before the goal has any `ha check` record", "current check or baseline", "`ha doctor`", "plan round"})
	c.tokens(ckSkill, "(the earlier sentences)", []string{files[ckSkill]}, "run `ha doctor <goal>` before any baseline",
		"Before any baseline, ask in one message", "comes before any baseline and also reviews them",
		"](references/bundle.md#questions-for-the-user)")
	c.report(t, v)
}

// AC-8: Seal's check guidance on input failures: never a pass, no defect for
// a check that requires no baseline, the dedicated producer of a marked
// condition, runner adapters, and the earlier distinctions kept.
func TestCheckCostAC8_InputFailureGuidance(t *testing.T) {
	const v = "seal-input-failure-guidance-missing"
	files := ckRead(t, ckBundleRef, ckSkill)
	var c ckCheck
	b := files[ckBundleRef]
	c.in(ckBundleRef, b, "## Check inputs", ckHeading("Check inputs"), nil,
		[]string{"input failure", "missing target file", "corrupt input", "tool that cannot start", "signal", "never a pass"},
		[]string{"require no baseline", "maintain", "`EX-n`", "task commands", "unmarked change conditions", "not a check defect", "until the input changes"},
		[]string{"dedicated producer of a marked change condition", "as a violation", "result contract allows", "`REVIEW.md`", "before the plan round", "before any review round"},
		[]string{"runner adapters classify input failures themselves"})
	c.in(ckBundleRef, b, "## Assurance policy and counterexamples", ckHeading("Assurance policy and counterexamples"),
		[]string{"Uninterpretable input is not successful evidence or an intended baseline violation.",
			"Preserve unobserved, environment error, skip, and observed violation separately."})
	c.tokens(ckSkill, "(the link to that section)", []string{files[ckSkill]}, "](references/bundle.md#check-inputs)")
	c.report(t, v)
}

// AC-11: Seal's guidance on the check freeze.
func TestCheckCostAC11_FreezeGuidance(t *testing.T) {
	const v = "seal-freeze-guidance-missing"
	files := ckRead(t, ckBundleRef, ckSkill)
	var c ckCheck
	c.in(ckBundleRef, files[ckBundleRef], "## Check inputs", ckHeading("Check inputs"), nil,
		[]string{"freeze", "earliest `plan` round", "`strict`", "earliest `candidate` round", "`standard`"},
		[]string{"frozen", "declarations", "producers", "tests", "helpers", "settings"},
		[]string{"after the freeze", "commit", "`ha note <goal> check-change checks:<path> --reason", "finding ID"},
		[]string{"next code round", "changed check files", "plan-delta"},
		[]string{"before the freeze", "no reason", "may still be fixed"},
		[]string{"`ha note <goal> review --gate", "right after the freeze round", "condition table"})
	c.tokens(ckSkill, "(the link to that section)", []string{files[ckSkill]}, "](references/bundle.md#check-inputs)")
	c.report(t, v)
}

// ckItem returns the index of the first line in lines[from:to] for which
// match holds, or -1.
func ckItem(lines []string, from, to int, match func(string) bool) int {
	for i := from; i < to && i < len(lines); i++ {
		if match(lines[i]) {
			return i
		}
	}
	return -1
}

// ckRulesRun runs one repository check file of the tree rooted at root. It
// returns whether the file passed and, when it failed, the opening line of
// a failed rule ("[violation:"). A failure without one, such as a check
// file that cannot run or a file it cannot read, is a broken setup.
func ckRulesRun(t *testing.T, root, file string) (bool, string) {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	out, code := ckRun(t, abs, os.Environ(), "python3", "-I", filepath.Join(abs, "tools", "workflow-checks", file))
	if code == 0 {
		return true, ""
	}
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "[violation:"); i >= 0 {
			return false, strings.TrimSpace(line[i:])
		}
	}
	t.Fatalf("setup: tools/workflow-checks/%s exit %d without a failed rule: %s", file, code, ckFlat(out))
	return false, ""
}

// AC-9: AGENTS.md narrows the input-failure rules to producers of conditions
// that require a baseline, and its two repository checks fix the new
// sentences instead of the earlier ones.
func TestCheckCostAC9_AgentsInputFailureRule(t *testing.T) {
	const v = "agents-input-failure-rule-unchanged"
	files := ckRead(t, ckAgents, ckRules, ckConsistency, ckPRTemplate)
	agents := files[ckAgents]
	lines := strings.Split(agents, "\n")

	// Locate `## Checks and review`, its Input failures item and the For
	// reviewers items about input failures.
	rulesAt := ckItem(lines, 0, len(lines), func(l string) bool { return l == "## Checks and review" })
	if rulesAt < 0 {
		t.Fatalf("jaekit:violation=%s %s has no `## Checks and review`", v, ckAgents)
	}
	end := ckItem(lines, rulesAt+1, len(lines), func(l string) bool { return ckLevel(l) == 2 || ckLevel(l) == 1 })
	if end < 0 {
		end = len(lines)
	}
	inputAt := ckItem(lines, rulesAt+1, end, func(l string) bool { return strings.HasPrefix(l, "- **Input failures.**") })
	reviewersAt := ckItem(lines, rulesAt+1, end, func(l string) bool { return l == "### For reviewers" })
	var reviewerItems []int
	if reviewersAt >= 0 {
		stop := ckItem(lines, reviewersAt+1, end, func(l string) bool { return ckLevel(l) > 0 && ckLevel(l) <= 3 })
		if stop < 0 {
			stop = end
		}
		for i := reviewersAt + 1; i < stop; i++ {
			if strings.HasPrefix(lines[i], "- ") && strings.Contains(lines[i], "counts an input failure") {
				reviewerItems = append(reviewerItems, i)
			}
		}
	}
	var c ckCheck
	if inputAt < 0 {
		c.lack("%s `## Checks and review`: no item starting \"- **Input failures.**\"", ckAgents)
	} else {
		c.line(ckAgents, "Input failures", lines[inputAt:inputAt+1], "producer of a change condition that requires a baseline",
			"before the plan round", "missing target file", "corrupt input", "a tool that cannot start", "termination by a signal",
			"every other check", "from counting as a pass", "does not depend on the rule version", "earlier rule", "every change condition")
	}
	if len(reviewerItems) == 0 {
		c.lack("%s `### For reviewers`: no item holding \"counts an input failure\"", ckAgents)
	} else {
		var items []string
		for _, i := range reviewerItems {
			items = append(items, lines[i])
		}
		c.tokens(ckAgents, "For reviewers", items, "as a violation is a defect only when", "producer of a condition that requires a baseline",
			"`missing: violation`", "as a pass is a defect")
	}
	// The earlier unconditional wordings are gone from the whole section.
	for i := rulesAt + 1; i < end; i++ {
		flat := ckFlat(lines[i])
		for _, phrase := range ckEarlierPhrases {
			if strings.Contains(flat, ckFlat(phrase)) {
				c.lack("%s `## Checks and review` line %d still holds the earlier unconditional %q", ckAgents, i+1, phrase)
			}
		}
	}
	c.report(t, v)

	// The repository checks pass on this tree.
	for _, file := range []string{"test_review_rules.py", "test_review_rules_consistency.py"} {
		if passed, rule := ckRulesRun(t, "..", file); !passed {
			t.Errorf("jaekit:violation=%s tools/workflow-checks/%s fails on this tree's AGENTS.md: %s", v, file, rule)
		}
	}
	if inputAt < 0 || len(reviewerItems) == 0 {
		return
	}

	// On a synthetic copy of AGENTS.md with an earlier item back in place,
	// the checks that read that item fail with their violation tag.
	variant := func(name string, replace func(lines []string) []string, failing ...string) {
		t.Helper()
		root := t.TempDir()
		ckWrite(t, filepath.Join(root, "AGENTS.md"), strings.Join(replace(slices.Clone(lines)), "\n"), 0o644)
		ckWrite(t, filepath.Join(root, ".github", "pull_request_template.md"), files[ckPRTemplate], 0o644)
		ckWrite(t, filepath.Join(root, "tools", "workflow-checks", "test_review_rules.py"), files[ckRules], 0o644)
		ckWrite(t, filepath.Join(root, "tools", "workflow-checks", "test_review_rules_consistency.py"), files[ckConsistency], 0o644)
		for _, file := range failing {
			if passed, _ := ckRulesRun(t, root, file); passed {
				t.Errorf("jaekit:violation=%s tools/workflow-checks/%s does not observe the narrowed rule: it passes with %s", v, file, name)
			}
		}
	}
	variant("the earlier Input failures item", func(l []string) []string {
		l[inputAt] = ckEarlierInputFailures
		return l
	}, "test_review_rules.py")
	variant("the earlier For reviewers item", func(l []string) []string {
		var out []string
		for i, line := range l {
			switch {
			case i == reviewerItems[0]:
				out = append(out, ckEarlierReviewers)
			case slices.Contains(reviewerItems, i):
			default:
				out = append(out, line)
			}
		}
		return out
	}, "test_review_rules.py", "test_review_rules_consistency.py")
}

// AC-17: the contracts, guides and Seal's documents describe run-rules/7 in
// Korean and English with the representative forms: the rule name, the
// minimum formula, the mark names and the freeze stages.
func TestCheckCostAC17_Documents(t *testing.T) {
	const v = "rules-v7-undocumented"
	type lang struct {
		mark, freeze []string
		// inputs are the lines of the narrowed input-failure policy.
		inputs [][]string
		// earlierRules restricts an earlier sentence to the earlier rules;
		// marked, with the rule name, restricts it to the marked conditions
		// under run-rules/7.
		earlierRules, marked string
	}
	ko := lang{[]string{"`baseline` 열", "`요구`"}, []string{"동결", "첫 `plan` 라운드", "첫 `candidate` 라운드"},
		[][]string{{"입력 실패", "대상 파일 없음", "손상된 입력", "시작하지 못하는 도구", "신호 종료", "통과가 아니다"},
			{"baseline을 요구하지 않는 검사", "위반, 오류, 판정 불가", "검사의 결함이 아니다"},
			{"baseline을 요구하는 change 조건의 전용 생산자", "위반으로 보고하지 않는다", "허용하는 경우"}},
		"run-rules/6까지", "표시한 change 조건"}
	en := lang{[]string{"`baseline` column", "`요구`"}, []string{"freeze", "earliest `plan` round", "earliest `candidate` round"},
		[][]string{{"input failure", "missing target file", "corrupt input", "tool that cannot start", "signal", "never a pass"},
			{"require no baseline", "violation, error or inconclusive", "not a check defect"},
			{"dedicated producer of a marked change condition", "reports none of them as a violation", "result contract allows"}},
		"up to run-rules/6", "marked change condition"}
	// scoped reports whether a flattened sentence without backquotes says
	// the rule scope of the earlier sentence it holds.
	scoped := func(l lang, sentence string) bool {
		return strings.Contains(sentence, ckFlat(l.earlierRules)) ||
			(strings.Contains(sentence, ckNewRules) && ckWord(sentence, ckFlat(l.marked)))
	}
	// The guides' sentences that ask every change condition for a baseline,
	// as they read before this goal: each sentence still holding one says
	// its rule scope.
	earlier := map[string][]string{
		"../guides/USAGE.md":         {"필수 change가 34개인 목표는 최소량 68회"},
		"../guides/USAGE.en.md":      {"a goal with 34 required change conditions needs at least 68 runs"},
		"../guides/OPERATIONS.md":    {"**의도한 위반**을 보여야", "수정 전·후 각 한 번씩 최소 68회"},
		"../guides/OPERATIONS.en.md": {"must show the **intended violation** before the change", "need at least 68 runs: one before and one after the change for each"},
	}
	docs := map[string]lang{
		"../contracts/run-record.md": ko, "../contracts/run-record.en.md": en,
		"../guides/USAGE.md": ko, "../guides/USAGE.en.md": en,
		"../guides/OPERATIONS.md": ko, "../guides/OPERATIONS.en.md": en,
	}
	paths := []string{"../contracts/run-record.md", "../contracts/run-record.en.md", "../contracts/bundle.md", "../contracts/bundle.en.md",
		"../contracts/core-capabilities.md", "../contracts/core-capabilities.en.md", "../guides/USAGE.md", "../guides/USAGE.en.md",
		"../guides/OPERATIONS.md", "../guides/OPERATIONS.en.md", ckSkill, ckHaRef, ckBundleRef, ckPlanTemplate, ckRevTemplate}
	files := ckRead(t, paths...)
	var c ckCheck
	rule := "a section whose heading holds " + ckNewRules

	// run-record: the run-rules/7 section and the 4.6 table.
	for _, p := range paths[:2] {
		l := docs[p]
		c.in(p, files[p], rule, ckHolding(ckNewRules),
			append([]string{ckOldRules, ckFormula, "`baseline_required`", "`check_freeze`", "`check-change`"}, l.mark...), l.freeze)
		if sec, ok := c.section(p, files[p], "4.6", ckNumbered("4.6")); ok {
			row := false
			for _, line := range sec {
				row = row || (strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, ckNewRules))
			}
			if !row {
				c.lack("%s 4.6: no table row with %s", p, ckNewRules)
			}
		}
	}
	// bundle: the condition table section.
	for _, p := range paths[2:4] {
		c.in(p, files[p], "2.1", ckNumbered("2.1"), []string{ckHeaderRow, "`요구`", ckNewRules})
	}
	// core-capabilities: the supported rule.
	for _, p := range paths[4:6] {
		c.in(p, files[p], rule, ckHolding(ckNewRules), []string{"supported_rules", "--rules " + ckNewRules})
	}
	// guides: the run-rules/7 section with the replacement guidance (the
	// formula, the mark, the freeze and the narrowed input-failure policy),
	// and the rule scope of each earlier sentence a guide keeps. A guide may
	// drop the earlier sentences.
	for _, p := range paths[6:10] {
		l := docs[p]
		c.in(p, files[p], rule, ckHolding(ckNewRules), append([]string{ckFormula}, l.mark...), append([][]string{l.freeze}, l.inputs...)...)
		for _, phrase := range earlier[p] {
			want := ckPlain(phrase)
			for i, line := range strings.Split(files[p], "\n") {
				for _, sentence := range ckSentences(ckPlain(line)) {
					if strings.Contains(sentence, want) && !scoped(l, sentence) {
						c.lack("%s line %d keeps the earlier %q in the sentence %q without its rule scope (%q, or %q with %q)",
							p, i+1, phrase, sentence, l.earlierRules, ckNewRules, l.marked)
					}
				}
			}
		}
	}
	// Seal: the skill, its references and templates.
	c.tokens(ckSkill, "(the skill)", []string{files[ckSkill]}, "--rules "+ckNewRules)
	c.in(ckHaRef, files[ckHaRef], "## "+ckNewRules, ckHeading(ckNewRules),
		append([]string{ckOldRules, ckFormula, "`ha note <goal> check-change"}, en.mark...), en.freeze)
	c.in(ckBundleRef, files[ckBundleRef], "### `## 조건표`", ckHeading("`## 조건표`"), []string{ckHeaderRow, "`요구`", "`" + ckNewRules + "`"})
	c.in(ckPlanTemplate, files[ckPlanTemplate], "## 조건표", ckHeading("조건표"), nil, []string{"`" + ckNewRules + "`", "`baseline` column", "`요구`"})
	c.in(ckRevTemplate, files[ckRevTemplate], "## 검증 방법과 환경", ckHeading("검증 방법과 환경"), nil,
		[]string{"`" + ckNewRules + "`", "requires a baseline", "`요구`"})
	c.report(t, v)
}

// AC-18: Seal's version is 0.1.29 everywhere it is written, and the version
// list says what 0.1.29 changes and that it needs a Core with run-rules/7.
func TestCheckCostAC18_SealVersion(t *testing.T) {
	const v = "seal-version-not-raised"
	const (
		claude      = "seal/.claude-plugin/plugin.json"
		codex       = "seal/.codex-plugin/plugin.json"
		marketplace = "../.claude-plugin/marketplace.json"
	)
	files := ckRead(t, claude, codex, marketplace, ckSkill, ckHaRef)
	versions := map[string]string{}
	for _, p := range []string{claude, codex} {
		var manifest struct {
			Version string `json:"version"`
		}
		ckDecode(t, p, files[p], &manifest)
		versions[p] = manifest.Version
	}
	var market struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	ckDecode(t, marketplace, files[marketplace], &market)
	versions[marketplace] = "(no seal entry)"
	for _, p := range market.Plugins {
		if p.Name == "seal" {
			versions[marketplace] = p.Version
		}
	}
	for _, p := range []string{claude, codex, marketplace} {
		if versions[p] != ckSealVersion {
			t.Errorf("jaekit:violation=%s %s says Seal %s, want %s", v, p, versions[p], ckSealVersion)
		}
	}
	front := ""
	if rest, ok := strings.CutPrefix(files[ckSkill], "---\n"); ok {
		front, _, _ = strings.Cut(rest, "\n---")
	}
	if !slices.Contains(strings.Split(front, "\n"), `  version: "`+ckSealVersion+`"`) {
		t.Errorf("jaekit:violation=%s %s front matter does not say version %q", v, ckSkill, ckSealVersion)
	}
	// The entry runs from "Seal 0.1.29" to the end of its line.
	entry := ""
	if _, rest, ok := strings.Cut(files[ckHaRef], "Seal "+ckSealVersion); ok {
		entry, _, _ = strings.Cut(rest, "\n")
	}
	var c ckCheck
	if entry == "" {
		c.lack("%s: no line holding %q", ckHaRef, "Seal "+ckSealVersion)
	} else {
		c.tokens(ckHaRef, "(the Seal "+ckSealVersion+" entry)", []string{entry}, "`"+ckNewRules+"`",
			"needs a Core that declares `"+ckNewRules+"`", "baseline", "freeze")
	}
	c.report(t, v)
}
