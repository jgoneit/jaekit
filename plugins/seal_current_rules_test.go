package plugins

// Property: the Seal skill states the current rules only
// The skill's body, its ha and bundle references and its templates state no
// condition on the Seal version that started a goal; the ha reference keeps
// one rules section, linking the contract for earlier rules, and no release
// history; the earlier-goals reference is short and lists no format; the
// check-input rule applies to every check written now; and the three main
// documents are shorter than at this goal's start. Sentences are matched with
// white space runs read as one space. Every failure message starts with its
// violation marker, except a document that cannot be read or is not UTF-8,
// or templates that cannot be found, which fail unmarked as a broken setup.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	// scrK1 and scrN1 state the scope of the check-input rule.
	scrK1 = "They apply to every check written or changed under this skill, whichever Seal started the goal."
	scrN1 = "never require rewriting or converting an existing goal's checks, records, `PROGRESS.md` or `REVIEW.md`"
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

// scrRulesLinks are the exact URLs of the run record contract's sections for
// the earlier rules that ha.md's `## run-rules/8` links.
var scrRulesLinks = []string{
	"https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4",
	"https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#48-run-rules5",
	"https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#49-run-rules6",
	"https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#410-run-rules7",
}

var (
	scrVersion     = regexp.MustCompile(`(?i)\bv?0\.1\.[0-9]+`)
	scrEarlierSeal = regexp.MustCompile(`(?i)earlier\s+seal`)
	// scrTagLink is a repository link pinned to a release tag, with the tag's
	// version; that version is the only one a URL may carry.
	scrTagLink   = regexp.MustCompile("https://github\\.com/jgoneit/jaekit/blob/v([0-9]+\\.[0-9]+\\.[0-9]+)/[^\\s)<>\\]`]*")
	scrRulesHead = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+run-rules/`)
	// scrVersionKey is a front-matter `version:` key up to its value.
	scrVersionKey = regexp.MustCompile(`^\s*version:\s*"?`)
	// scrTicks is a backtick run; scrBlank is a blank line, which no code span
	// crosses.
	scrTicks = regexp.MustCompile("`+")
	scrBlank = regexp.MustCompile(`\n[ \t]*\n`)
)

// scrUnspanned returns the submatch indexes of the matches of re in text, the
// unfenced lines of a file or section joined with "\n", that no code span
// encloses. A code span is a backtick run, its content and the next run of
// the same length before a blank line; it may cross line breaks.
func scrUnspanned(text string, re *regexp.Regexp) [][]int {
	runs := scrTicks.FindAllStringIndex(text, -1)
	var spans [][]int
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs) && !scrBlank.MatchString(text[runs[i][1]:runs[j][0]]); j++ {
			if runs[j][1]-runs[j][0] == runs[i][1]-runs[i][0] {
				spans, i = append(spans, []int{runs[i][0], runs[j][1]}), j
				break
			}
		}
	}
	var out [][]int
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		if !slices.ContainsFunc(spans, func(s []int) bool { return s[0] <= m[0] && m[1] <= s[1] }) {
			out = append(out, m)
		}
	}
	return out
}

// scrSentence matches sentence with white space runs read as any white space.
func scrSentence(sentence string) *regexp.Regexp {
	words := strings.Fields(sentence)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(strings.Join(words, `\s+`))
}

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

// SKILL.md, ha.md, bundle.md and every template name a 0.1.x version only
// as the tag's version inside a tag-pinned repository link, on SKILL.md's
// front-matter version line or on ha.md's release sentence, and say "earlier
// Seal" only in SKILL.md's `## References`. Both patterns read the whole
// file, so a match may span a line break; a match is placed by the line it
// starts on.
func TestSealCurrentRulesAC1_NoVersionConditions(t *testing.T) {
	var files []string
	err := filepath.WalkDir(sealSkillDir+"/assets/templates", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, filepath.ToSlash(path))
		}
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("setup: no templates under %s/assets/templates: %v", sealSkillDir, err)
	}
	var found []string
	for _, path := range append([]string{scrSkill, scrHa, scrBundle}, files...) {
		text := scrRead(t, path)
		lines := strings.Split(text, "\n")
		line := func(at int) int { return strings.Count(text[:at], "\n") }
		front := 0 // lines before it are SKILL.md's front matter
		if path == scrSkill && lines[0] == "---" {
			for front = 1; front < len(lines) && lines[front] != "---"; front++ {
			}
		}
		refs := []int{0, 0} // SKILL.md's `## References`, as a byte range
		if sec := skSection(text, "## References"); path == scrSkill && sec != "" {
			refs = []int{strings.Index(text, sec), strings.Index(text, sec) + len(sec)}
		}
		tags := scrTagLink.FindAllStringSubmatchIndex(text, -1)
		var bad [][]int
		for _, m := range scrVersion.FindAllStringIndex(text, -1) {
			n := line(m[0])
			tag := slices.ContainsFunc(tags, func(u []int) bool {
				return u[0] <= m[0] && m[1] <= u[1] && strings.TrimPrefix(strings.ToLower(text[m[0]:m[1]]), "v") == text[u[2]:u[3]]
			})
			key := scrVersionKey.FindString(lines[n]) // the value right after it is SKILL.md's version
			value := n < front && key != "" && m[0]-strings.LastIndex(text[:m[0]], "\n")-1 == len(key)
			if !tag && !value && !(path == scrHa && lines[n] == scrH0) {
				bad = append(bad, m)
			}
		}
		for _, m := range scrEarlierSeal.FindAllStringIndex(text, -1) {
			if !(refs[0] <= m[0] && m[1] <= refs[1]) {
				bad = append(bad, m)
			}
		}
		slices.SortFunc(bad, func(a, b []int) int { return a[0] - b[0] })
		last := -1
		for _, m := range bad {
			if n := line(m[0]); n != last {
				found, last = append(found, fmt.Sprintf("%s:%d: %q", path, n+1, text[m[0]:m[1]])), n
			}
		}
	}
	if len(found) > 0 {
		shown := found[:min(len(found), 20)]
		t.Errorf("jaekit:violation=skill-version-conditions: %d lines name a version or an earlier Seal (first %d):\n%s",
			len(found), len(shown), strings.Join(shown, "\n"))
	}
}

// The Markdown forms these checks read: a fence line, with its marks and
// what follows them; a heading line, up to three spaces, one to six `#` and a
// space or tab, with its marks and text; and level-one and level-two
// heading lines.
var (
	scrFence   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	scrHeading = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*)$`)
	scrTitle   = regexp.MustCompile(`^ {0,3}#[ \t]+`)
	scrH2      = regexp.MustCompile(`^ {0,3}##[ \t]+`)
)

// scrFenced reports for each line whether it is a fence line or inside
// fenced code, as CommonMark reads ``` and ~~~ fences.
func scrFenced(lines []string) []bool {
	fenced, fence := make([]bool, len(lines)), ""
	for i, line := range lines {
		m := scrFence.FindStringSubmatch(line)
		switch {
		case fence != "":
			fenced[i] = true
			if m != nil && m[1][0] == fence[0] && len(m[1]) >= len(fence) && strings.TrimSpace(m[2]) == "" {
				fence = ""
			}
		case m != nil:
			fenced[i], fence = true, m[1]
		}
	}
	return fenced
}

// scrIsHeading reports whether line is a heading with the marks and text of
// heading, such as "## run-rules/8".
func scrIsHeading(line, heading string) bool {
	marks, text, _ := strings.Cut(heading, " ")
	m := scrHeading.FindStringSubmatch(line)
	return m != nil && m[1] == marks && m[2] == text
}

// scrSection returns the lines outside fenced code from the first unfenced
// heading line with heading's marks and text up to the next unfenced heading
// of the same or a higher level; "" when there is no such line.
func scrSection(text, heading string) string {
	lines := strings.Split(text, "\n")
	fenced := scrFenced(lines)
	level := strings.Index(heading, " ")
	var out []string
	for i, line := range lines {
		switch {
		case fenced[i]:
		case out == nil:
			if scrIsHeading(line, heading) {
				out = []string{line}
			}
		default:
			if m := scrHeading.FindStringSubmatch(line); m != nil && len(m[1]) <= level {
				return strings.Join(out, "\n")
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// Between ha.md's title and its first `## ` heading only the release
// sentence remains. Headings may be indented by up to three spaces and take a
// space or tab after their marks, and lines in fenced code are never
// headings.
func TestSealCurrentRulesAC2_NoReleaseHistory(t *testing.T) {
	var lines []string
	title := false
	all := strings.Split(scrRead(t, scrHa), "\n")
	fenced := scrFenced(all)
	for i, line := range all {
		if !fenced[i] && scrH2.MatchString(line) {
			break
		}
		if !fenced[i] && !title && scrTitle.MatchString(line) {
			title = true
			continue
		}
		if title && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if !title || len(lines) != 1 || lines[0] != scrH0 {
		t.Errorf("jaekit:violation=ha-release-history: %s has %d non-blank lines between its title and its first `## ` heading (title found: %t); want only %q",
			scrHa, len(lines), title, scrH0)
	}
}

// ha.md's only rules heading outside fenced code, at any level, is
// `## run-rules/8`, and that section has an inline link, outside code spans,
// to each exact URL of the run record contract's sections for the earlier
// rules.
func TestSealCurrentRulesAC3_OneRulesSection(t *testing.T) {
	text := scrRead(t, scrHa)
	lines := strings.Split(text, "\n")
	fenced := scrFenced(lines)
	var heads []string
	for i, line := range lines {
		if !fenced[i] && scrRulesHead.MatchString(line) {
			heads = append(heads, line)
		}
	}
	if len(heads) != 1 || !scrIsHeading(heads[0], "## run-rules/8") {
		t.Errorf("jaekit:violation=ha-rules-sections: %s has the rules headings %q; want only %q", scrHa, heads, "## run-rules/8")
	}
	sec := scrSection(text, "## run-rules/8")
	links := scrUnspanned(sec, skillLink)
	for _, want := range scrRulesLinks {
		if !slices.ContainsFunc(links, func(m []int) bool { return sec[m[2]:m[3]] == want }) {
			t.Errorf("jaekit:violation=ha-rules-sections: %s ## run-rules/8 has no link to %s", scrHa, want)
		}
	}
}

// The earlier-goals reference holds its four sentences outside fenced code
// and code spans and at most 300 words, with no version and no rules name,
// which a per-rules format list would need.
func TestSealCurrentRulesAC4_EarlierGoalsShort(t *testing.T) {
	text := scrRead(t, scrEarlier)
	lines := strings.Split(text, "\n")
	fenced := scrFenced(lines)
	var prose []string
	for i, line := range lines {
		if !fenced[i] {
			prose = append(prose, line)
		}
	}
	for _, rule := range scrEarlierRules {
		if len(scrUnspanned(strings.Join(prose, "\n"), scrSentence(rule))) == 0 {
			t.Errorf("jaekit:violation=earlier-goals-long: %s lacks %q", scrEarlier, rule)
		}
	}
	if n := len(regexp.MustCompile(`0\.1\.[0-9]`).FindAllString(text, -1)); n > 0 {
		t.Errorf("jaekit:violation=earlier-goals-long: %s names a 0.1.x version %d times", scrEarlier, n)
	}
	if n := strings.Count(text, "run-rules/"); n > 0 {
		t.Errorf("jaekit:violation=earlier-goals-long: %s names run-rules/ %d times", scrEarlier, n)
	}
	if n := len(strings.Fields(text)); n > 300 {
		t.Errorf("jaekit:violation=earlier-goals-long: %s has %d words, more than 300", scrEarlier, n)
	}
}

// The check-input rule applies to every check written now and rewrites no
// existing goal: SKILL.md's and bundle.md's `## Check inputs` both say so
// outside fenced code and code spans.
func TestSealCurrentRulesAC5_CheckInputsApplyNow(t *testing.T) {
	for _, path := range []string{scrSkill, scrBundle} {
		sec := scrSection(scrRead(t, path), "## Check inputs")
		for _, want := range []string{scrK1, scrN1} {
			if len(scrUnspanned(sec, scrSentence(want))) == 0 {
				t.Errorf("jaekit:violation=check-inputs-versioned: %s ## Check inputs lacks %q", path, want)
			}
		}
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
