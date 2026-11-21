package plugins

// Checks of the review-summary-four goal: fixed tokens in named documents
// state the four-heading review summary, where each piece of information goes,
// the rewrite rules and the guides' description. A document that cannot be
// read, is not valid UTF-8 or lacks the section a test reads fails as a broken
// setup with no marker, so an input failure is never a pass.

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	rsfContract  = "contracts/bundle.md"
	rsfReference = "plugins/seal/skills/seal/references/bundle.md"
	rsfTemplate  = "plugins/seal/skills/seal/assets/templates/PROGRESS.md"
	rsfSummary   = "## 검수 요약"
)

// rsfOld are the seven-heading format's own headings, without backticks so
// that a code span and a template heading line both hold them.
var (
	rsfHeadings = []string{"### 바뀐 것", "### 근거", "### 직접 볼 것", "### 보지 않은 것"}
	rsfOld      = []string{"### 바뀐 동작과 파일", "### 범위 밖이나 기존 것을 바꾼 곳", "### 누가 정했나", "### 자세히"}
)

// rsfPlaces are the information tokens each heading's item holds.
var rsfPlaces = map[string][]string{
	"### 바뀐 것": {"바뀐 동작 | 조건 | 파일", "Every file changed from the goal's base to the completion commit",
		"changes outside the scope or to existing things"},
	"### 근거": {"what the check that satisfied it observed", "the commit where it passed", "never by seq alone",
		"라운드 | 지적 | 끝", "decisions in three groups", "executor assumptions nobody was asked about",
		"Mark an assumption that reads a SPEC sentence", "the documents and sections that hold the detail"},
	"### 직접 볼 것": {"every executor assumption", "executor assumptions that read a SPEC sentence",
		"changes outside the scope or to existing things"},
	"### 보지 않은 것": {"the unobserved scope", "change conditions met without a baseline"},
}

// rsfJudgment are the places the judgment-items line gives.
var rsfJudgment = []string{"the changed files and the condition each contributed to under `### 바뀐 것`",
	"assumed open decisions under `### 근거` and `### 직접 볼 것`", "`manual` conditions under `### 직접 볼 것`",
	"the one followed) under `### 근거`", "the unobserved scope and substitute environments under `### 보지 않은 것`",
	"where the independent review results are under `### 근거`", "in the opening lines"}

var (
	rsfItemLine = regexp.MustCompile("^(\\s*)(?:[-*]|\\d+\\.)\\s+`(### [^`]+)`")
	rsfListLine = regexp.MustCompile(`^\s*(?:[-*]|\d+\.)\s`)
)

func rsfRead(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if !utf8.Valid(b) {
		t.Fatalf("%s is not valid UTF-8", rel)
	}
	return string(b)
}

// rsfDoc is one document's review summary section: its lines and text, the
// heading of every list item that starts with a backticked `### ` token, in
// order, and the text of the first item of each heading, where a `### ` line
// is an item up to the next `### ` line.
type rsfDoc struct {
	rel, text string
	lines     []string
	names     []string
	items     map[string]string
}

// rsfSection reads rel from the line heading up to the next line that starts
// with one of stops; with no such line the section may be truncated, and the
// test fails as a broken setup.
func rsfSection(t *testing.T, rel, heading string, stops ...string) rsfDoc {
	t.Helper()
	lines := strings.Split(rsfRead(t, rel), "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimRight(l, " \t\r") == heading })
	if start < 0 {
		t.Fatalf("%s has no line %q", rel, heading)
	}
	n := slices.IndexFunc(lines[start+1:], func(l string) bool {
		return slices.ContainsFunc(stops, func(s string) bool { return strings.HasPrefix(l, s) })
	})
	if n < 0 {
		t.Fatalf("%s: no line starting with %q follows %q", rel, stops, heading)
	}
	end := start + 1 + n
	d := rsfDoc{rel: rel, text: strings.Join(lines[start:end], "\n"), lines: lines[start:end], items: map[string]string{}}
	for i := start + 1; i < end; i++ {
		name, j := strings.TrimRight(lines[i], " \t\r"), i+1
		if m := rsfItemLine.FindStringSubmatch(lines[i]); m != nil {
			name = m[2]
			for j < end && rsfInItem(lines, j, len(m[1])) {
				j++
			}
			d.names = append(d.names, name)
		} else if strings.HasPrefix(name, "### ") {
			for j < end && !strings.HasPrefix(lines[j], "### ") {
				j++
			}
		} else {
			continue
		}
		if d.items[name] == "" {
			d.items[name] = strings.Join(lines[i:j], "\n")
		}
	}
	return d
}

// rsfInItem reports whether line j continues a list item indented by n: a
// blank line, a deeper line, or a line that is no list item right after a
// non-blank line.
func rsfInItem(lines []string, j, n int) bool {
	l := lines[j]
	return strings.TrimSpace(l) == "" || len(l)-len(strings.TrimLeft(l, " \t")) > n ||
		!rsfListLine.MatchString(l) && strings.TrimSpace(lines[j-1]) != ""
}

// rsfFormat is the contract's and the skill reference's review summary.
func rsfFormat(t *testing.T) []rsfDoc {
	t.Helper()
	return []rsfDoc{rsfSection(t, rsfContract, "### 5.4 Review summary", "### ", "## "),
		rsfSection(t, rsfReference, "## Review summary", "## ")}
}

// rsfCheck reports each token that text, white space runs read as one space,
// lacks (want) or holds (!want), with the marker of id first; id "" adds none.
func rsfCheck(t *testing.T, id, where, text string, want bool, tokens ...string) {
	t.Helper()
	flat, mark, verb := strings.Join(strings.Fields(text), " "), "", "lacks"
	if id != "" {
		mark = "jaekit:violation=" + id + ": "
	}
	if !want {
		verb = "still holds"
	}
	for _, tok := range tokens {
		if strings.Contains(flat, strings.Join(strings.Fields(tok), " ")) != want {
			t.Errorf("%s%s %s %q", mark, where, verb, tok)
		}
	}
}

func TestReviewSummaryFourAC1_SameFourHeadings(t *testing.T) {
	for _, d := range rsfFormat(t) {
		if !slices.Equal(d.names, rsfHeadings) {
			t.Errorf("jaekit:violation=seven-heading-format: %s lists the headings %q, not %q", d.rel, d.names, rsfHeadings)
		}
		rsfCheck(t, "seven-heading-format", d.rel, d.text, true, "write `없음`")
	}
}

func TestReviewSummaryFourAC1_NoOldHeadings(t *testing.T) {
	for _, rel := range []string{rsfContract, rsfReference, "plugins/seal/skills/seal/SKILL.md", rsfTemplate} {
		rsfCheck(t, "seven-heading-format", rel, rsfRead(t, rel), false, append(rsfOld, "The seven headings below")...)
	}
}

func TestReviewSummaryFourAC2_ExampleFourHeadings(t *testing.T) {
	const rel = "examples/empty-input/PROGRESS.md"
	rsfCheck(t, "seven-heading-template", rel, rsfRead(t, rel), false, rsfOld...)
	rsfCheck(t, "seven-heading-template", rel+" "+rsfSummary, rsfSection(t, rel, rsfSummary, "## ").text, true,
		"`### 바뀐 것`, `### 근거`, `### 직접 볼 것`, `### 보지 않은 것`")
}

// Each heading's item in the contract, the reference and the template holds
// its information, and the judgment-items line of the contract and the
// reference names each place.
func TestReviewSummaryFourAC3_InformationPlaces(t *testing.T) {
	const id = "summary-item-without-place"
	format := rsfFormat(t)
	for _, d := range append(format, rsfSection(t, rsfTemplate, rsfSummary, "## ")) {
		for _, h := range rsfHeadings {
			if text, ok := d.items[h]; ok {
				rsfCheck(t, id, d.rel+" `"+h+"`", text, true, rsfPlaces[h]...)
			} else {
				t.Errorf("jaekit:violation=%s: %s has no `%s` item", id, d.rel, h)
			}
		}
	}
	for _, d := range format {
		if k := slices.IndexFunc(d.lines, func(l string) bool {
			return strings.HasPrefix(strings.TrimPrefix(l, "- "), "The judgment items of the")
		}); k < 0 {
			t.Errorf("jaekit:violation=%s: %s has no line that begins \"The judgment items of the\"", id, d.rel)
		} else {
			rsfCheck(t, id, d.rel+" judgment items", d.lines[k], true, rsfJudgment...)
		}
	}
}

func TestReviewSummaryFourAC4_RewriteRules(t *testing.T) {
	for _, d := range rsfFormat(t) {
		rsfCheck(t, "summary-rewrite-rule-missing", d.rel, d.text, true,
			"A review summary already written with the seven earlier headings stays as it is.",
			"A review summary rewritten after a reopen uses the four headings.",
			"Written once `ha done` records completion", "rewritten as of each new completion record after a reopen",
			"not written before completion", "stays unchanged until the next completion record",
			"Add one line at its top saying which completion record", "A stop in between",
			"leaves the earlier content and that line as they are", "that line does not remain",
			"applies instead of the one line kept before completion")
	}
}

func TestReviewSummaryFourAC5_PRDescription(t *testing.T) {
	for _, d := range rsfFormat(t) {
		rsfCheck(t, "", d.rel, d.text, true, "build it from that goal's `## 검수 요약`", "Never drop a `### 직접 볼 것` item")
	}
}

func TestReviewSummaryFourAC6_GuidesFourHeadings(t *testing.T) {
	ko := "그 아래에 바뀐 것, 근거, 직접 볼 것, 보지 않은 것이 이 순서로 옵니다."
	en := "below them come what changed, evidence, what to inspect and what was not observed, in that order."
	guides := map[string]string{"README.ko.md": ko, "guides/OVERVIEW.md": ko, "guides/USAGE.md": ko,
		"guides/OPERATIONS.md": ko, "README.md": en, "guides/OVERVIEW.en.md": en, "guides/USAGE.en.md": en,
		"guides/OPERATIONS.en.md": en}
	for _, rel := range slices.Sorted(maps.Keys(guides)) {
		text := rsfRead(t, rel)
		rsfCheck(t, "seven-part-summary-description", rel, text, true, guides[rel])
		rsfCheck(t, "seven-part-summary-description", rel, text, false,
			"바뀐 동작과 파일, 범위 밖이나 기존 것을 바꾼 곳", "changed behavior and files, scope changes")
	}
}
