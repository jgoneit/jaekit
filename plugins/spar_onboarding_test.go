package plugins

// Checks of the spar-onboarding goal: the README and the install, Spar, usage
// and operations guides say when Spar is needed instead of calling it
// optional, the Spar guide says which reviewers are required and how to list
// only the CLIs a user has, the usage and operations guides say that Seal
// stops when ha lacks what it requires, and the Seal and Spar skills pass
// every goal document as criteria and the goal's check files as check paths.
//
// Each test observes fixed tokens, links and lines of the current tree, never
// an open category of sentences, and opens every failure message about its
// property with the same violation marker, which ha's go-test-json adapter
// reads. A file the test cannot read fails with a "setup:" message and no
// marker: that is neither the declared violation nor a pass.

import (
	"os"
	"strings"
	"testing"
)

// soRead returns a file of the tree under test. Any read error is a setup
// failure, never the violation.
func soRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return string(b)
}

// soLines returns the lines of text outside fenced code.
func soLines(text string) []string {
	var out []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			out = append(out, line)
		}
	}
	return out
}

// soSection returns the lines from heading to the next heading of the same
// or a higher level, outside fenced code, and whether the heading exists.
// The heading "" stands for the text before the first "## " heading.
func soSection(text, heading string) ([]string, bool) {
	lines := strings.Split(text, "\n")
	level := strings.Index(heading, " ")
	start, fenced := -1, false
	if heading == "" {
		start, level = 0, 2
	}
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
		if i == start {
			continue
		}
		if n := strings.Index(line, " "); n > 0 && n <= level && strings.Trim(line[:n], "#") == "" {
			return lines[start:i], true
		}
	}
	if start < 0 {
		return nil, false
	}
	return lines[start:], true
}

// soLineWith returns the index of the first line holding every token, or -1.
func soLineWith(lines []string, tokens ...string) int {
	for i, line := range lines {
		all := true
		for _, tok := range tokens {
			if !strings.Contains(line, tok) {
				all = false
				break
			}
		}
		if all {
			return i
		}
	}
	return -1
}

// soBlocks returns the fenced code blocks of lines whose opening fence is
// "```"+lang, each as its lines without the fences.
func soBlocks(lines []string, lang string) [][]string {
	var blocks [][]string
	var cur []string
	in, want := false, false
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if in {
				if want {
					blocks = append(blocks, cur)
				}
				in, cur = false, nil
			} else {
				in, want = true, strings.TrimSpace(strings.TrimPrefix(line, "```")) == lang
			}
			continue
		}
		if in {
			cur = append(cur, line)
		}
	}
	return blocks
}

// soNonEmpty returns the trimmed non-empty lines.
func soNonEmpty(lines []string) []string {
	var out []string
	for _, line := range lines {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

const (
	soReadme     = "../README.md"
	soReadmeKo   = "../README.ko.md"
	soInstall    = "../guides/INSTALL.md"
	soInstallEn  = "../guides/INSTALL.en.md"
	soSpar       = "../guides/SPAR.md"
	soSparEn     = "../guides/SPAR.en.md"
	soUsage      = "../guides/USAGE.md"
	soUsageEn    = "../guides/USAGE.en.md"
	soOps        = "../guides/OPERATIONS.md"
	soOpsEn      = "../guides/OPERATIONS.en.md"
	soSealSkill  = "seal/skills/seal/SKILL.md"
	soSparSkill  = "spar/skills/spar/SKILL.md"
	soSparConfig = "../contracts/spar-config.md"
)

// The guides AC-1 names.
var soGuides = []string{soReadme, soReadmeKo, soInstall, soInstallEn, soSpar, soSparEn, soUsage, soUsageEn, soOps, soOpsEn}

// The words and headings that called Spar optional without a condition.
var soOptional = []string{
	"Spar는 선택 기능이며", "첫 작업에는 필요 없습니다", "Spar (선택)", "Spar로 하는 선택적 독립 검토",
	"Spar is optional", "you do not need it for your first task", "Spar (optional)", "Optional independent review rounds with Spar",
}

// The statement of when Spar is needed: the rule for standard and strict
// goals under run-rules/5, the goals that do not need Spar, and how to finish
// without review.
var (
	soNeedKo = []string{"v0.1.7부터", "`run-rules/5`", "`standard`·`strict` 목표", "기록된 Spar gate가 있어야 완료됩니다",
		"`light` 목표", "`Assurance:` 줄이 없는 목표", "`run-rules/4` 이하로 시작한 목표", "Spar가 필요 없습니다",
		"검토 없이 끝내려면", "`light`로 낮추는 목표 문서 수정"}
	soNeedEn = []string{"From v0.1.7", "`run-rules/5`", "`standard` or `strict` goal", "completes only with a recorded Spar gate",
		"`light` goals", "goals without an `Assurance:` line", "goals started under `run-rules/4` or earlier", "do not need Spar",
		"To finish without review", "goal-document change that lowers the tier to `light`"}
)

type soPlace struct {
	path, heading string
	ko            bool
	link          string // the link to the usage guide's review gate section; "" inside that section
}

// Where each guide says when Spar is needed: where it called Spar optional,
// and the usage guide's review gate section.
var soNeedPlaces = []soPlace{
	{soReadme, "### Install", false, "](guides/USAGE.en.md#review-gate-run-rules5)"},
	{soReadmeKo, "### 설치", true, "](guides/USAGE.md#검토-gate-run-rules5)"},
	{soInstall, "### Spar", true, "](USAGE.md#검토-gate-run-rules5)"},
	{soInstallEn, "### Spar", false, "](USAGE.en.md#review-gate-run-rules5)"},
	{soSpar, "", true, "](USAGE.md#검토-gate-run-rules5)"},
	{soSparEn, "", false, "](USAGE.en.md#review-gate-run-rules5)"},
	{soUsage, "## 검토 gate (run-rules/5)", true, ""},
	{soUsageEn, "## Review gate (run-rules/5)", false, ""},
}

// AC-1: no guide calls Spar optional or unneeded for a first task without a
// condition.
func TestSparOnboardingAC1_NoGuideCallsSparOptional(t *testing.T) {
	const v = "spar-called-optional"
	for _, path := range soGuides {
		text := soRead(t, path)
		for _, phrase := range soOptional {
			if strings.Contains(text, phrase) {
				t.Errorf("jaekit:violation=%s %s still says %q", v, path, phrase)
			}
		}
	}
}

// AC-1: in its place each guide says, in one paragraph, when Spar is needed
// and how to finish without review, and links the review gate section.
func TestSparOnboardingAC1_GuidesStateWhenSparIsNeeded(t *testing.T) {
	const v = "spar-need-unstated"
	for _, p := range soNeedPlaces {
		lines, ok := soSection(soRead(t, p.path), p.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, p.path, p.heading)
			continue
		}
		tokens := soNeedEn
		if p.ko {
			tokens = soNeedKo
		}
		if p.link != "" {
			tokens = append(append([]string{}, tokens...), p.link)
		}
		if soLineWith(soLines(strings.Join(lines, "\n")), tokens...) < 0 {
			t.Errorf("jaekit:violation=%s %s section %q has no paragraph with all of %q", v, p.path, p.heading, tokens)
		}
	}
}

// AC-2: the README's install part says that standard and strict goals need
// the Spar executable and plugin and links the install guide's Spar section.
func TestSparOnboardingAC2_ReadmeInstallLeadsToSpar(t *testing.T) {
	const v = "readme-spar-install-missing"
	for _, c := range []struct {
		path, heading string
		tokens        []string
	}{
		{soReadme, "### Install", []string{"From v0.1.7", "`standard` and `strict` goals need the Spar executable and the Spar plugin", "](guides/INSTALL.en.md#spar)"}},
		{soReadmeKo, "### 설치", []string{"v0.1.7부터", "`standard`·`strict` 목표에는 Spar 실행 파일과 Spar 플러그인이 필요", "](guides/INSTALL.md#spar)"}},
	} {
		lines, ok := soSection(soRead(t, c.path), c.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, c.path, c.heading)
			continue
		}
		if soLineWith(soLines(strings.Join(lines, "\n")), c.tokens...) < 0 {
			t.Errorf("jaekit:violation=%s %s section %q has no paragraph with all of %q", v, c.path, c.heading, c.tokens)
		}
	}
}

// AC-2: the install guides' Spar section has a heading without an
// unconditional "optional", says when Spar is needed, and the install
// commands follow that statement.
func TestSparOnboardingAC2_InstallGuideSparSection(t *testing.T) {
	const v = "install-spar-section-optional"
	for _, c := range []struct {
		path   string
		tokens []string
	}{
		{soInstall, []string{"v0.1.7부터", "`standard`·`strict` 목표에는 Spar 실행 파일과 Spar 플러그인이 필요"}},
		{soInstallEn, []string{"From v0.1.7", "`standard` and `strict` goals need the Spar executable and the Spar plugin"}},
	} {
		text := soRead(t, c.path)
		for _, line := range soLines(text) {
			low := strings.ToLower(line)
			if strings.HasPrefix(line, "#") && strings.Contains(low, "spar") && (strings.Contains(line, "선택") || strings.Contains(low, "optional")) {
				t.Errorf("jaekit:violation=%s %s heading %q calls Spar optional", v, c.path, line)
			}
		}
		lines, ok := soSection(text, "### Spar")
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the heading %q", v, c.path, "### Spar")
			continue
		}
		at := soLineWith(lines, c.tokens...)
		if at < 0 {
			t.Errorf("jaekit:violation=%s %s section %q has no paragraph with all of %q", v, c.path, "### Spar", c.tokens)
			continue
		}
		// The commands sit in fenced blocks after the statement.
		rest := strings.Join(lines[at+1:], "\n")
		for _, cmd := range []string{"spar --version", "codex plugin add spar@jaekit", "claude plugin install spar@jaekit"} {
			if !strings.Contains(rest, cmd) {
				t.Errorf("jaekit:violation=%s %s section %q has no %q after the statement", v, c.path, "### Spar", cmd)
			}
		}
	}
}

// AC-3: the Spar guides' reviewer section says that the default reviewers
// are all required, what a missing CLI does to the gate and the goal, shows
// configurations for the CLIs a user has, and says to check them with spar
// config before a round.
func TestSparOnboardingAC3_ReviewerSectionStatesRequiredReviewers(t *testing.T) {
	const v = "reviewer-setup-unstated"
	for _, c := range []struct {
		path, heading      string
		required, beforeRd string
		degraded           []string
	}{
		{soSpar, "## 리뷰어 설정", "`claude`, `codex`, `agy` 셋이 모두 필수 리뷰어입니다", "라운드를 돌리기 전에 `spar config`로",
			[]string{"CLI가 없거나", "라운드를 끝내지 못하면", "`spar waive`", "`degraded`", "`standard`·`strict` 목표가 완료되지 않습니다"}},
		{soSparEn, "## Configure reviewers", "`claude`, `codex` and `agy` are all required reviewers", "Before running a round, check with `spar config`",
			[]string{"CLI is missing", "does not finish a round", "`spar waive`", "`degraded`", "`standard` or `strict` goal does not complete"}},
	} {
		lines, ok := soSection(soRead(t, c.path), c.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, c.path, c.heading)
			continue
		}
		prose := soLines(strings.Join(lines, "\n"))
		for _, tok := range []string{c.required, c.beforeRd} {
			if soLineWith(prose, tok) < 0 {
				t.Errorf("jaekit:violation=%s %s section %q lacks %q", v, c.path, c.heading, tok)
			}
		}
		if soLineWith(prose, c.degraded...) < 0 {
			t.Errorf("jaekit:violation=%s %s section %q has no paragraph with all of %q", v, c.path, c.heading, c.degraded)
		}
		one, native := false, false
		for _, block := range soBlocks(lines, "toml") {
			got := soNonEmpty(block)
			if len(got) == 2 && got[0] == "[[reviewers]]" &&
				(got[1] == `preset = "claude"` || got[1] == `preset = "codex"` || got[1] == `preset = "agy"`) {
				one = true
			}
			if len(got) == 1 && got[0] == "reviewers = []" {
				native = true
			}
		}
		if !one {
			t.Errorf("jaekit:violation=%s %s section %q has no example listing one reviewer CLI", v, c.path, c.heading)
		}
		if !native {
			t.Errorf("jaekit:violation=%s %s section %q has no example with the empty list for the native reviewer", v, c.path, c.heading)
		}
	}
}

// soExample returns the Spar skill's round example block and the first
// non-empty line after it.
func soExample(t *testing.T) ([]string, string, bool) {
	t.Helper()
	lines := strings.Split(soRead(t, soSparSkill), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "For example, a candidate round") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, "", false
	}
	var block []string
	in := false
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "```") {
			if in {
				for _, after := range lines[i+1:] {
					if strings.TrimSpace(after) != "" {
						return block, after, true
					}
				}
				return block, "", true
			}
			in = true
			continue
		}
		if in {
			block = append(block, line)
		} else if strings.TrimSpace(line) != "" {
			break
		}
	}
	return nil, "", false
}

// AC-4: the Spar skill's round example gives every goal document as
// criteria and says so next to the example.
func TestSparOnboardingAC4_SparSkillExampleGivesEveryGoalDocument(t *testing.T) {
	const v = "spar-example-criteria-partial"
	block, after, ok := soExample(t)
	if !ok {
		t.Fatalf("jaekit:violation=%s %s has no round example block after %q", v, soSparSkill, "For example, a candidate round")
	}
	cmd := strings.Join(block, " ")
	if !strings.Contains(cmd, "--criteria docs/specs/<goal>/SPEC.md") {
		t.Errorf("jaekit:violation=%s the example does not give SPEC.md as --criteria: %q", v, cmd)
	}
	if n := strings.Count(cmd, "--criteria docs/specs/<goal>/"); n < 2 {
		t.Errorf("jaekit:violation=%s the example gives %d goal document(s) as --criteria, not SPEC.md and a linked document", v, n)
	}
	for _, tok := range []string{"Give every goal document as `--criteria`", "each goal document it links"} {
		if !strings.Contains(after, tok) {
			t.Errorf("jaekit:violation=%s the line after the example lacks %q: %q", v, tok, after)
		}
	}
}

// AC-5: the Seal skill passes the goal's check files as check paths on
// every round.
func TestSparOnboardingAC5_SealSkillPassesCheckPaths(t *testing.T) {
	const v = "seal-check-path-unstated"
	tokens := []string{"Rounds take the goal's added or changed check files", "declarations", "producers", "tests", "`--check-path`"}
	for _, line := range soLines(soRead(t, soSealSkill)) {
		for _, sentence := range strings.Split(line, ". ") {
			if soLineWith([]string{sentence}, tokens...) == 0 {
				return
			}
		}
	}
	t.Errorf("jaekit:violation=%s %s has no sentence with all of %q", v, soSealSkill, tokens)
}

// AC-5: the Spar skill's round example shows --check-path and says which
// files it takes.
func TestSparOnboardingAC5_SparSkillExampleShowsCheckPath(t *testing.T) {
	const v = "spar-example-check-path-missing"
	block, after, ok := soExample(t)
	if !ok {
		t.Fatalf("jaekit:violation=%s %s has no round example block after %q", v, soSparSkill, "For example, a candidate round")
	}
	if cmd := strings.Join(block, " "); !strings.Contains(cmd, "--check-path ") {
		t.Errorf("jaekit:violation=%s the example has no --check-path: %q", v, cmd)
	}
	for _, tok := range []string{"each check file the change adds or changes", "`--check-path`"} {
		if !strings.Contains(after, tok) {
			t.Errorf("jaekit:violation=%s the line after the example lacks %q: %q", v, tok, after)
		}
	}
}

// AC-6: the usage and operations guides say that Seal stops without a record
// and without falling back when the installed ha lacks run-rules/5 or other
// support, and that the remedy is an ha with that support; the conditional
// start sentence is gone.
func TestSparOnboardingAC6_GuidesSayHaWithoutSupportStopsSeal(t *testing.T) {
	const v = "ha-requirement-fallback"
	ko := []string{"`run-rules/5`", "Seal은 기록 없이 멈추고", "이전 규칙으로 물러나지 않습니다", "지원을 갖춘 `ha`를 설치"}
	en := []string{"`run-rules/5`", "Seal stops without a record", "does not fall back to older rules", "install an `ha` that has that support"}
	for _, c := range []struct {
		path, heading string
		tokens        []string
	}{
		{soUsage, "## 검토 gate (run-rules/5)", ko},
		{soUsageEn, "## Review gate (run-rules/5)", en},
		{soOps, "## 실행 전에 확인할 것", ko},
		{soOpsEn, "## What to check before running", en},
	} {
		text := soRead(t, c.path)
		lines, ok := soSection(text, c.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, c.path, c.heading)
			continue
		}
		if soLineWith(soLines(strings.Join(lines, "\n")), c.tokens...) < 0 {
			t.Errorf("jaekit:violation=%s %s section %q has no paragraph with all of %q", v, c.path, c.heading, c.tokens)
		}
		for _, old := range []string{"Core가 `run-rules/5`와 검토 gate 지원을 갖추면", "when the Core supports `run-rules/5` and the review gate"} {
			if strings.Contains(text, old) {
				t.Errorf("jaekit:violation=%s %s still says %q", v, c.path, old)
			}
		}
	}
}

// AC-7 (kept): the guidance for light, untiered and earlier-rule goals and
// the meaning of project check paths and --check-path stay as they were.
func TestSparOnboardingAC7_KeptGuidance(t *testing.T) {
	for _, c := range []struct {
		path, heading string
		keep          []string
	}{
		{soUsage, "## 검토 gate (run-rules/5)", []string{
			"Spar를 쓸 수 없어 검토 없이 끝내려면, 사용자가 SPEC을 `Assurance: light`로 낮추는 수정을 정합니다. 그 뒤 `confirm spec`과 신선한 검사로 gate 없이 완료됩니다.",
			"`light`이거나 등급 줄이 없는 목표, 그리고 `run-rules/1`~`run-rules/4`로 시작한 목표는 지금처럼 gate 없이 판정합니다."}},
		{soUsageEn, "## Review gate (run-rules/5)", []string{
			"To finish without review when Spar cannot run, you decide a SPEC change that lowers the tier to `Assurance: light`. After `confirm spec` and fresh checks, the goal completes without a gate.",
			"A `light` goal, a goal without the tier line, and goals started under `run-rules/1` to `run-rules/4` are judged without a gate, as before."}},
		{soSpar, "### 프로젝트 설정", []string{
			"저장소의 `.jaekit/spar.toml`은 라운드 상한, 공격 각도, 규칙 문서, 검사 경로만 정할 수 있습니다.",
			`check_paths = ["tools/checks/**"]`}},
		{soSparEn, "### Project settings", []string{
			"A repository's `.jaekit/spar.toml` can set only round caps, attack angles, rule documents and check paths:",
			`check_paths = ["tools/checks/**"]`}},
		{soSparConfig, "## 2. 프로젝트 설정", []string{
			"`check_paths`: 검사 코드로 보는 경로의 glob(G24). 잘못된 glob은 설정 오류다. 분류와 gate의 `growth`, 패킷이 쓴다."}},
		{soSparConfig, "## 4. 적용 기록", []string{
			"`spar run --check-path GLOB`은 그 실행의 `check_paths`에 더하며 digest에 반영된다."}},
		{soSparSkill, "## Before the first round", []string{
			"A repository's `.jaekit/spar.toml` sets only caps, angles, rules and check paths."}},
	} {
		lines, ok := soSection(soRead(t, c.path), c.heading)
		if !ok {
			t.Errorf("%s lacks the section %q", c.path, c.heading)
			continue
		}
		body := strings.Join(lines, "\n")
		for _, keep := range c.keep {
			if !strings.Contains(body, keep) {
				t.Errorf("%s section %q no longer says %q", c.path, c.heading, keep)
			}
		}
	}
}

// AC-8 (kept): the Seal skill stays within 2,200 words, and the install pins
// of the guides and the Seal skill name the current release, v0.1.7.
func TestSparOnboardingAC8_SkillSizeAndPins(t *testing.T) {
	if n := len(strings.Fields(soRead(t, soSealSkill))); n > 2200 {
		t.Errorf("%s has %d words, more than 2,200", soSealSkill, n)
	}
	for _, path := range []string{soReadme, soReadmeKo, soInstall, soInstallEn} {
		text := soRead(t, path)
		for _, pin := range []string{"claude plugin marketplace add jgoneit/jaekit#v0.1.7", "codex plugin marketplace add jgoneit/jaekit@v0.1.7"} {
			if !strings.Contains(text, pin) {
				t.Errorf("%s lost the pin %q", path, pin)
			}
		}
	}
	if pin := "https://github.com/jgoneit/jaekit/blob/v0.1.7/guides/INSTALL.md"; !strings.Contains(soRead(t, soSealSkill), pin) {
		t.Errorf("%s lost the install route %q", soSealSkill, pin)
	}
}
