package plugins

// Checks of the goal review-summary-followups (AC-1 to AC-6). Each check
// reports a broken requirement on its first failure line as
// `jaekit:violation=<token>`, the token its declaration in
// tools/goal-checks/review-summary-followups/ names; anything that keeps the
// check from observing the requirement fails with `setup:` instead.
//
// The document checks (AC-1 to AC-3, AC-5) look for the fixed tokens below
// in the named sections. Implementers write these tokens verbatim: the
// Korean ones in contracts/bundle.md and guides/DEVELOPMENT.md, the English
// ones in the other documents. A token may wrap across lines.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
)

// rsfDoc is a document section a check reads, in one language.
type rsfDoc struct {
	path, heading, lang string
}

// The four places that state the review summary rules.
var rsfSummaryDocs = []rsfDoc{
	{"../contracts/bundle.md", "### 5.4 검수 요약", "ko"},
	{"../contracts/bundle.en.md", "### 5.4 Review summary", "en"},
	{"seal/skills/seal/references/bundle.md", "## Review summary", "en"},
	{"seal/skills/seal/assets/templates/PROGRESS.md", "## 검수 요약", "en"},
}

// AC-1: the review summary of a reopened goal.
var rsfReopen = map[string][]string{
	"ko": {
		"완료 기록이 있는 목표를 `reopen`으로 다시 열면",
		"다음 완료 기록 전까지 이 절의 이전 내용은 고치지 않는다",
		"맨 위에 한 줄을 둔다",
		"어느 완료 기록(seq와 commit) 기준인지",
		"그 완료 기록이 지금은 유효하지 않다",
		"다음 완료 기록 뒤 다시 쓴다",
		"그 사이에 멈춰도",
		"이전 내용과 그 줄은 그대로다",
		"그 줄은 남지 않는다",
	},
	"en": {
		"When a goal with a completion record is reopened with `reopen`",
		"the earlier content of this section stays unchanged until the next completion record",
		"one line at its top",
		"which completion record (seq and commit) it is as of",
		"that completion record is no longer valid",
		"rewritten after the next completion record",
		"A stop in between",
		"leaves the earlier content and that line as they are",
		"that line does not remain",
	},
}

// AC-2: the judgment items moved from item 2 of the completion report keep
// its qualifications.
var rsfMoved = map[string][]string{
	"ko": {
		"완료 보고 2에서 옮긴 판단 항목도 완료 보고 2의 한정을 지킨다",
		"실행자의 설명은 주장으로 표시한다",
		"기록으로 확인한 사실과 실행자의 해석을 나눈다",
		"근거의 버전과 그 근거가 덮는 범위를 밝힌다",
	},
	"en": {
		"The judgment items moved from item 2 of the completion report keep its qualifications",
		"the executor's statements are marked as claims",
		"facts confirmed by records are kept apart from the executor's interpretation",
		"the evidence's version and the scope that evidence covers",
	},
}

// AC-3: the two lists are minimum lists.
var rsfMinimum = map[string][]string{
	"ko": {
		"`### 근거`의 두 부분과 첫 줄들의 항목은 최소 목록이다",
		"판단 항목 대응 문단이 정한 항목은 그 목록과 함께 그 자리에 들어간다",
	},
	"en": {
		"The two parts of `### 근거` and the items of the opening lines are minimum lists",
		"the items the judgment-item paragraph assigns join them in the same place",
	},
}

// AC-5: the Seal skill size policy of the development guide.
var rsfPolicyDocs = []rsfDoc{
	{"../guides/DEVELOPMENT.md", "## Seal skill 분량", "ko"},
	{"../guides/DEVELOPMENT.en.md", "## Seal skill size", "en"},
}

var rsfPolicy = map[string][]string{
	"ko": {
		"앞머리를 포함해 공백으로 나눈 단어 2,300개",
		"`TestSealSlimAC14_WordLimit`",
		"`TestSparOnboardingAC8_SkillSizeAndPins`",
		"Seal에 더하는 규칙은 `references/`의 절에 쓰고 skill에는 그 절로 가는 링크를 둔다",
		"skill이 이미 그 절을 링크하면 skill을 바꾸지 않는다",
		"skill 단어를 늘리는 변경은 같은 변경에서 그만큼 줄이거나 상한을 올리는 사용자의 말을 받는다",
		"다른 목표의 시험이 skill에 고정한 문장은 그 시험을 바꾸는 사용자의 말 없이 옮기지 않는다",
	},
	"en": {
		"2,300 whitespace-separated words, front matter included",
		"`TestSealSlimAC14_WordLimit`",
		"`TestSparOnboardingAC8_SkillSizeAndPins`",
		"New Seal rules go into a section under `references/`, and the skill links that section",
		"When the skill already links that section, the skill does not change",
		"A change that adds skill words removes as many in the same change or gets the user's words raising the limit",
		"Sentences another goal's tests pin in the skill move only with the user's words changing those tests",
	},
}

// AC-6: the Seal version this goal raises to (the next patch of the goal's
// base, the user's answer "0.1.28로 올림 (Recommended)"), and the phrase of
// its change-list paragraph.
const (
	rsfSealVersion = "0.1.28"
	rsfNoCore      = "no Core change"
)

// rsfRead returns a file of the checked tree. A missing or unreadable file
// keeps the check from observing its requirement.
func rsfRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: cannot read %s: %v", path, err)
	}
	return string(data)
}

// rsfNorm collapses whitespace so that a token may wrap across lines.
func rsfNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// rsfLevel is the heading level of a Markdown line, or 0.
func rsfLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n == len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// rsfSection returns the lines under the heading equal to heading, up to the
// next heading of the same or a higher level, outside fenced code blocks.
func rsfSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	fence, start, level := "", -1, 0
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			if start >= 0 {
				out = append(out, line)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			if start >= 0 {
				out = append(out, line)
			}
			continue
		}
		if l := rsfLevel(line); l > 0 {
			if start >= 0 && l <= level {
				break
			}
			if start < 0 && strings.TrimRight(line, " \t") == heading {
				start, level = 1, l
				continue
			}
		}
		if start >= 0 {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), start >= 0
}

// rsfNeedTokens reports every token absent from the section of each
// document as the violation.
func rsfNeedTokens(t *testing.T, violation string, docs []rsfDoc, tokens map[string][]string) {
	t.Helper()
	for _, d := range docs {
		text := rsfRead(t, d.path)
		sec, ok := rsfSection(text, d.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s has no section %q", violation, d.path, d.heading)
			continue
		}
		norm := rsfNorm(sec)
		for _, token := range tokens[d.lang] {
			if !strings.Contains(norm, rsfNorm(token)) {
				t.Errorf("jaekit:violation=%s %s section %q lacks %q", violation, d.path, d.heading, token)
			}
		}
	}
}

func TestReviewSummaryFollowupsAC1_Reopen(t *testing.T) {
	rsfNeedTokens(t, "reopen-summary-unmarked", rsfSummaryDocs, rsfReopen)
}

func TestReviewSummaryFollowupsAC2_MovedJudgment(t *testing.T) {
	rsfNeedTokens(t, "moved-judgment-unqualified", rsfSummaryDocs, rsfMoved)
}

func TestReviewSummaryFollowupsAC3_MinimumLists(t *testing.T) {
	rsfNeedTokens(t, "minimum-lists-unstated", rsfSummaryDocs, rsfMinimum)
}

func TestReviewSummaryFollowupsAC5_SkillSizePolicy(t *testing.T) {
	rsfNeedTokens(t, "skill-size-policy-missing", rsfPolicyDocs, rsfPolicy)
}

// rsfManifestVersion is the "version" of a JSON manifest; a missing or
// unreadable manifest is a broken setup.
func rsfManifestVersion(t *testing.T, path string) string {
	t.Helper()
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(rsfRead(t, path)), &m); err != nil {
		t.Fatalf("setup: %s is not a JSON manifest: %v", path, err)
	}
	return m.Version
}

var rsfFrontVersion = regexp.MustCompile(`^\s*version:\s*"?([^"\s]+)"?\s*$`)

func TestReviewSummaryFollowupsAC6_Version(t *testing.T) {
	const v = "seal-version-not-raised"
	versions := map[string]string{}
	for _, p := range []string{"seal/.claude-plugin/plugin.json", "seal/.codex-plugin/plugin.json"} {
		versions[p] = rsfManifestVersion(t, p)
	}
	var market struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	const marketPath = "../.claude-plugin/marketplace.json"
	if err := json.Unmarshal([]byte(rsfRead(t, marketPath)), &market); err != nil {
		t.Fatalf("setup: %s is not a JSON marketplace: %v", marketPath, err)
	}
	seen := false
	for _, p := range market.Plugins {
		if p.Name == "seal" {
			versions[marketPath+" (seal)"], seen = p.Version, true
		}
	}
	if !seen {
		t.Fatalf("setup: %s has no seal entry", marketPath)
	}
	const skill = "seal/skills/seal/SKILL.md"
	text := rsfRead(t, skill)
	parts := strings.SplitN(text, "---\n", 3)
	if len(parts) < 3 || parts[0] != "" {
		t.Fatalf("setup: %s has no front matter", skill)
	}
	for _, line := range strings.Split(parts[1], "\n") {
		if m := rsfFrontVersion.FindStringSubmatch(line); m != nil {
			versions[skill+" (front matter)"] = m[1]
		}
	}
	if _, ok := versions[skill+" (front matter)"]; !ok {
		t.Fatalf("setup: %s front matter has no version", skill)
	}
	for where, got := range versions {
		if got != rsfSealVersion {
			t.Errorf("jaekit:violation=%s %s says Seal %q; want %s", v, where, got, rsfSealVersion)
		}
	}
	const ha = "seal/skills/seal/references/ha.md"
	found := false
	for _, block := range strings.Split(rsfRead(t, ha), "\n\n") {
		if strings.HasPrefix(strings.TrimSpace(block), "Seal "+rsfSealVersion+" ") {
			found = true
			if !strings.Contains(rsfNorm(block), rsfNoCore) {
				t.Errorf("jaekit:violation=%s %s: the Seal %s paragraph does not say %q", v, ha, rsfSealVersion, rsfNoCore)
			}
		}
	}
	if !found {
		t.Errorf("jaekit:violation=%s %s has no paragraph starting with \"Seal %s \"", v, ha, rsfSealVersion)
	}
}

// AC-4 runs the guide check of plugins/review_summary_test.go
// (TestReviewSummaryAC15_Guides) on a copy of the checked tree, once for each
// representative form below, and compares its outcome with the one the
// rendered page calls for. Only the copy is changed.

// The guide phrases the forms move into or around comments. The guide check
// requires each of them; the guides are outside this goal's scope.
const (
	rsfGuideKo   = "guides/USAGE.md"
	rsfPhraseKo  = "목표를 다시 열어 완료를 다시 기록하면 새 완료 기록 기준으로 다시 씁니다"
	rsfQualKo    = "0.1.24 이상으로 시작해 완료한 목표에서는 판단 문장이 `## 검수 요약`에 있습니다"
	rsfGuideEn   = "README.md"
	rsfPhraseEn  = "after a reopen the section is rewritten as of the new completion record"
	rsfGuideTest = "TestReviewSummaryAC15_Guides"
	rsfGuideViol = "jaekit:violation=guides-review-summary-missing"
)

// The outcomes of one run of the guide check.
const (
	rsfViolation = "its violation"
	rsfPass      = "a pass"
	rsfSkip      = "a skip"
)

// rsfCopy copies the parts of the tree the plugins tests build from, and the
// guides, READMEs and contracts the guide check reads, into dir.
func rsfCopy(t *testing.T, dir string) {
	t.Helper()
	for _, f := range []string{"go.mod", "go.sum", "README.md", "README.ko.md"} {
		data, err := os.ReadFile(filepath.Join("..", f))
		if err != nil {
			t.Fatalf("setup: cannot copy %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	for _, d := range []string{"cmd", "internal", "plugins", "guides", "contracts"} {
		src := filepath.Join("..", d)
		err := filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel("..", p)
			if err != nil {
				return err
			}
			to := filepath.Join(dir, rel)
			if e.IsDir() {
				return os.MkdirAll(to, 0o755)
			}
			if !e.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(to, data, 0o644)
		})
		if err != nil {
			t.Fatalf("setup: cannot copy %s: %v", d, err)
		}
	}
}

// rsfSpan finds phrase in text with any whitespace between its words.
func rsfSpan(text, phrase string) (int, int, bool) {
	words := strings.Fields(phrase)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	loc := regexp.MustCompile(strings.Join(words, `\s+`)).FindStringIndex(text)
	if loc == nil {
		return 0, 0, false
	}
	return loc[0], loc[1], true
}

// rsfBlock finds the blank-line separated block that holds phrase.
func rsfBlock(text, phrase string) (int, int, bool) {
	s, e, ok := rsfSpan(text, phrase)
	if !ok {
		return 0, 0, false
	}
	start := strings.LastIndex(text[:s], "\n\n")
	if start < 0 {
		start = 0
	} else {
		start += 2
	}
	end := strings.Index(text[e:], "\n\n")
	if end < 0 {
		end = len(text)
	} else {
		end += e
	}
	return start, end, true
}

// rsfGuideRun runs the guide check in dir and classifies its outcome. An
// outcome that is none of the three (a build failure, a signal) is a broken
// setup.
func rsfGuideRun(t *testing.T, dir, form string) string {
	t.Helper()
	cmd := exec.Command("go", "test", "-v", "-count=1", "-run", "^"+rsfGuideTest+"$", "./plugins")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := out.String()
	var exit *exec.ExitError
	switch {
	case err != nil && !errors.As(err, &exit):
		t.Fatalf("setup: %s: go test did not start: %v", form, err)
	case exit != nil:
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			t.Fatalf("setup: %s: go test ended by signal %v", form, ws.Signal())
		}
		if strings.Contains(text, "--- FAIL: "+rsfGuideTest) && strings.Contains(text, rsfGuideViol) {
			return rsfViolation
		}
		t.Fatalf("setup: %s: go test failed without the guide check's violation:\n%s", form, text)
	case strings.Contains(text, "--- SKIP: "+rsfGuideTest):
		return rsfSkip
	case strings.Contains(text, "--- PASS: "+rsfGuideTest):
		return rsfPass
	}
	t.Fatalf("setup: %s: go test did not run %s:\n%s", form, rsfGuideTest, text)
	return ""
}

func TestReviewSummaryFollowupsAC4_VisibleGuideText(t *testing.T) {
	const v = "guide-comment-counted"
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("setup: go is not available: %v", err)
	}
	dir := t.TempDir()
	rsfCopy(t, dir)
	orig := map[string]string{}
	for _, f := range []string{rsfGuideKo, rsfGuideEn, "guides/OVERVIEW.md", "guides/OPERATIONS.en.md"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("setup: cannot read the copy of %s: %v", f, err)
		}
		orig[f] = string(data)
	}
	restore := func() {
		for f, text := range orig {
			p := filepath.Join(dir, f)
			if err := os.Chmod(p, 0o644); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("setup: %v", err)
			}
			if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}
	}
	// edit rewrites one guide of the copy: the text before the span, the span
	// and the text after it become before+left+span+right+after.
	edit := func(form, file, phrase string, block bool, left, right string) {
		t.Helper()
		text := orig[file]
		s, e, ok := rsfSpan(text, phrase)
		if block {
			s, e, ok = rsfBlock(text, phrase)
		}
		if !ok {
			t.Fatalf("setup: %s: %s has no %q to build the form from", form, file, phrase)
		}
		out := text[:s] + left + text[s:e] + right + text[e:]
		if err := os.WriteFile(filepath.Join(dir, file), []byte(out), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// The unchanged copy is the premise of every form: the guide check passes.
	restore()
	if got := rsfGuideRun(t, dir, "the unchanged copy"); got != rsfPass {
		t.Fatalf("setup: the guide check gives %s on the unchanged copy; every form needs a pass there", got)
	}
	_, nextEnd, ok := rsfBlock(orig[rsfGuideKo], rsfPhraseKo)
	if !ok {
		t.Fatalf("setup: %s has no %q", rsfGuideKo, rsfPhraseKo)
	}
	forms := []struct {
		name  string
		build func()
		want  string
	}{
		{"a comment around the whole paragraph of a Korean guide phrase", func() {
			edit("whole paragraph", rsfGuideKo, rsfPhraseKo, true, "<!-- ", " -->")
		}, rsfViolation},
		{"a comment around only the Korean guide phrase inside its paragraph", func() {
			edit("phrase only", rsfGuideKo, rsfPhraseKo, false, "<!-- ", " -->")
		}, rsfViolation},
		{"a comment over several lines and two paragraphs", func() {
			text := orig[rsfGuideKo]
			s, _, _ := rsfBlock(text, rsfPhraseKo)
			end := nextEnd
			if end+2 < len(text) {
				if i := strings.Index(text[end+2:], "\n\n"); i >= 0 {
					end = end + 2 + i
				} else {
					end = len(text)
				}
			}
			out := text[:s] + "<!--\n" + text[s:end] + "\n-->" + text[end:]
			if err := os.WriteFile(filepath.Join(dir, rsfGuideKo), []byte(out), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}, rsfViolation},
		{"a comment around only the qualifier inside its list item", func() {
			edit("qualifier only", rsfGuideKo, rsfQualKo, false, "<!-- ", " -->")
		}, rsfViolation},
		{"a comment around the whole paragraph of an English guide phrase", func() {
			edit("whole paragraph", rsfGuideEn, rsfPhraseEn, true, "<!-- ", " -->")
		}, rsfViolation},
		{"an unclosed `<!--` inside a code span before the phrase", func() {
			edit("code span", rsfGuideKo, rsfPhraseKo, false, "`<!--` ", "")
		}, rsfPass},
		{"an unclosed `<!--` inside a fenced code block before the paragraph", func() {
			edit("code block", rsfGuideKo, rsfPhraseKo, true, "```text\n<!--\n```\n\n", "")
		}, rsfPass},
		{"the paragraph in a comment and again outside it", func() {
			text := orig[rsfGuideKo]
			s, e, _ := rsfBlock(text, rsfPhraseKo)
			out := text[:s] + "<!-- " + text[s:e] + " -->\n\n" + text[s:e] + text[e:]
			if err := os.WriteFile(filepath.Join(dir, rsfGuideKo), []byte(out), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}, rsfPass},
		{"a missing guide", func() {
			if err := os.Remove(filepath.Join(dir, "guides/OVERVIEW.md")); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}, rsfViolation},
		{"a guide that cannot be read", func() {
			p := filepath.Join(dir, "guides/OPERATIONS.en.md")
			if err := os.Chmod(p, 0); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if _, err := os.ReadFile(p); err == nil {
				t.Fatalf("setup: a file without permissions can still be read here, so the form cannot be built")
			}
		}, rsfSkip},
	}
	for _, f := range forms {
		restore()
		f.build()
		if got := rsfGuideRun(t, dir, f.name); got != f.want {
			t.Errorf("jaekit:violation=%s %s: the guide check gives %s; the rendered page calls for %s", v, f.name, got, f.want)
		}
	}
	restore()
}
