package plugins

// Property: the Seal skill states the current rules only
// The skill's body, its ha and bundle references and its templates state no
// condition on the Seal version that started a goal; the ha reference keeps
// one rules section and no release history; the earlier-goals reference is
// short; the check-input rule applies to every check written now; and the
// three main documents are shorter than at this goal's start. Every failure
// message starts with its violation marker, except a document that cannot be
// read or is not UTF-8, which fails unmarked as a broken setup.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	scrSkill   = sealSkillDir + "/SKILL.md"
	scrHa      = sealSkillDir + "/references/ha.md"
	scrBundle  = sealSkillDir + "/references/bundle.md"
	scrEarlier = sealSkillDir + "/references/earlier-goals.md"

	// scrH0 is the release sentence that alone stays above ha.md's first
	// `## ` heading.
	scrH0 = "The commands below describe release v0.1.8: ha 0.1.8, spar 0.1.8 and Seal 0.1.23."
	// scrK1 is the scope of the check-input rule.
	scrK1 = "They apply to every check written or changed under this skill, whichever Seal started the goal."
	// scrWordCap is 80% of 17968, rounded down: the strings.Fields word count
	// of SKILL.md (2092), references/ha.md (7775) and references/bundle.md
	// (8101) at the commit that declared this goal's checks.
	scrWordCap = 14374
)

// scrEarlierRules are the sentences the earlier-goals reference holds.
var scrEarlierRules = []string{
	"Goal documents and bundle files keep the format they were started with; do not convert them.",
	"`ha status` computes the state under the goal's own saved rules.",
	"Checks, review rounds and reports you write now follow the current skill.",
	"For the exact earlier formats, read the contracts.",
}

var (
	scrVersion     = regexp.MustCompile(`(?i)\b(seal|ha|spar|core)\s+v?0\.1\.[0-9]+`)
	scrEarlierSeal = regexp.MustCompile(`(?i)earlier seal`)
	scrURL         = regexp.MustCompile(`https?://\S+`)
)

func scrRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !utf8.Valid(b) {
		t.Fatalf("setup: %s is not UTF-8", path)
	}
	return string(b)
}

// A Seal version is named only on SKILL.md's front-matter version line and
// on ha.md's release sentence, outside URLs; "earlier Seal" only on a line
// that names earlier-goals.md.
func TestSealCurrentRulesAC1_NoVersionConditions(t *testing.T) {
	templates, err := filepath.Glob(sealSkillDir + "/assets/templates/*.md")
	if err != nil || len(templates) == 0 {
		t.Fatalf("setup: no templates in %s/assets/templates: %v", sealSkillDir, err)
	}
	var found []string
	for _, path := range append([]string{scrSkill, scrHa, scrBundle}, templates...) {
		lines := strings.Split(scrRead(t, path), "\n")
		front := 0 // lines before it are SKILL.md's front matter
		if path == scrSkill && lines[0] == "---" {
			for front = 1; front < len(lines) && lines[front] != "---"; front++ {
			}
		}
		for i, line := range lines {
			bare := scrURL.ReplaceAllString(line, "<url>")
			allowed := i < front && strings.HasPrefix(strings.TrimSpace(line), "version:") || path == scrHa && line == scrH0
			if m := scrVersion.FindString(bare); m != "" && !allowed {
				found = append(found, fmt.Sprintf("%s:%d: %q", path, i+1, m))
			} else if m := scrEarlierSeal.FindString(bare); m != "" && !strings.Contains(line, "earlier-goals.md") {
				found = append(found, fmt.Sprintf("%s:%d: %q", path, i+1, m))
			}
		}
	}
	if len(found) > 0 {
		shown := found[:min(len(found), 20)]
		t.Errorf("jaekit:violation=skill-version-conditions: %d lines state a Seal version condition (first %d):\n%s",
			len(found), len(shown), strings.Join(shown, "\n"))
	}
}

// Between ha.md's title and its first `## ` heading only the release
// sentence remains.
func TestSealCurrentRulesAC2_NoReleaseHistory(t *testing.T) {
	var lines []string
	title := false
	for _, line := range strings.Split(scrRead(t, scrHa), "\n") {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if title && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
		title = title || strings.HasPrefix(line, "# ")
	}
	if !title || len(lines) != 1 || lines[0] != scrH0 {
		t.Errorf("jaekit:violation=ha-release-history: %s has %d non-blank lines between its title and its first `## ` heading (title found: %t); want only %q",
			scrHa, len(lines), title, scrH0)
	}
}

// ha.md has one rules section, `## run-rules/8`, which links the run record
// contract's rules for earlier goals.
func TestSealCurrentRulesAC3_OneRulesSection(t *testing.T) {
	text := scrRead(t, scrHa)
	var heads []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## run-rules/") {
			heads = append(heads, line)
		}
	}
	if len(heads) != 1 || heads[0] != "## run-rules/8" {
		t.Errorf("jaekit:violation=ha-rules-sections: %s has the rules sections %q; want only %q", scrHa, heads, "## run-rules/8")
	}
	for _, m := range skillLink.FindAllStringSubmatch(skSection(text, "## run-rules/8"), -1) {
		if strings.Contains(m[1], "contracts/run-record.md#4") {
			return
		}
	}
	t.Errorf("jaekit:violation=ha-rules-sections: %s ## run-rules/8 has no link to contracts/run-record.md#4…", scrHa)
}

// The earlier-goals reference holds its four sentences, no version and at
// most 300 words.
func TestSealCurrentRulesAC4_EarlierGoalsShort(t *testing.T) {
	text := scrRead(t, scrEarlier)
	for _, rule := range scrEarlierRules {
		if !strings.Contains(text, rule) {
			t.Errorf("jaekit:violation=earlier-goals-long: %s lacks %q", scrEarlier, rule)
		}
	}
	if n := len(regexp.MustCompile(`0\.1\.[0-9]`).FindAllString(text, -1)); n > 0 {
		t.Errorf("jaekit:violation=earlier-goals-long: %s names a 0.1.x version %d times", scrEarlier, n)
	}
	if n := len(strings.Fields(text)); n > 300 {
		t.Errorf("jaekit:violation=earlier-goals-long: %s has %d words, more than 300", scrEarlier, n)
	}
}

// The check-input rule applies to every check written now: SKILL.md's
// `## Check inputs` and bundle.md say so.
func TestSealCurrentRulesAC5_CheckInputsApplyNow(t *testing.T) {
	if !strings.Contains(skSection(scrRead(t, scrSkill), "## Check inputs"), scrK1) {
		t.Errorf("jaekit:violation=check-inputs-versioned: %s ## Check inputs lacks %q", scrSkill, scrK1)
	}
	if !strings.Contains(scrRead(t, scrBundle), scrK1) {
		t.Errorf("jaekit:violation=check-inputs-versioned: %s lacks %q", scrBundle, scrK1)
	}
}

// SKILL.md, ha.md and bundle.md together have at most scrWordCap words.
func TestSealCurrentRulesAC8_Shorter(t *testing.T) {
	total := 0
	for _, path := range []string{scrSkill, scrHa, scrBundle} {
		total += len(strings.Fields(scrRead(t, path)))
	}
	if total > scrWordCap {
		t.Errorf("jaekit:violation=skill-not-shorter: %s, references/ha.md and references/bundle.md have %d words, more than %d",
			scrSkill, total, scrWordCap)
	}
}
