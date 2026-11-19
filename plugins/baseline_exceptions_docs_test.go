package plugins

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	beBundle       = "seal/skills/seal/references/bundle.md"
	beHA           = "seal/skills/seal/references/ha.md"
	beReviewTmpl   = "seal/skills/seal/assets/templates/REVIEW.md"
	beReviewPhrase = "after the last review round and right before the completion record"
)

type beDocument struct {
	path string
	text string
}

// beRead reads a document through the package's shared read helper and
// rejects invalid UTF-8 as an input failure.
func beRead(t *testing.T, path string) string {
	t.Helper()
	text := read(t, path)
	if !utf8.ValidString(text) {
		t.Fatalf("%s is not UTF-8", path)
	}
	return text
}

// beSealDocuments walks the Seal Markdown files from the repository root.
// Paths passed to read remain relative to the plugins package directory.
func beSealDocuments(t *testing.T) []beDocument {
	t.Helper()
	root := "../plugins/seal"
	var docs []beDocument
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		relative, err := filepath.Rel("../plugins", path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		docs = append(docs, beDocument{path: relative, text: beRead(t, relative)})
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk Seal documents: %v", err)
	}
	if len(docs) == 0 {
		t.Fatalf("Seal document file list is empty")
	}
	return docs
}

// bePublicGuides loads the root readmes, every Markdown file directly under
// guides/, and the public site home page.
func bePublicGuides(t *testing.T) []beDocument {
	t.Helper()
	paths := []string{"../README.md", "../README.ko.md", "../tools/site/home.html"}
	guides, err := filepath.Glob("../guides/*.md")
	if err != nil {
		t.Fatalf("cannot list public guides: %v", err)
	}
	paths = append(paths, guides...)
	if len(paths) == 0 {
		t.Fatalf("public guide file list is empty")
	}
	docs := make([]beDocument, 0, len(paths))
	for _, path := range paths {
		docs = append(docs, beDocument{path: filepath.ToSlash(path), text: beRead(t, path)})
	}
	return docs
}

func beSealText(t *testing.T, docs []beDocument, path string) string {
	t.Helper()
	for _, doc := range docs {
		if doc.path == path {
			return doc.text
		}
	}
	t.Fatalf("%s is missing from the Seal document file list", path)
	return ""
}

func bePublicText(t *testing.T, docs []beDocument, path string) string {
	t.Helper()
	for _, doc := range docs {
		if doc.path == path {
			return doc.text
		}
	}
	t.Fatalf("%s is missing from the public guide file list", path)
	return ""
}

func beRequire(issues *[]string, path, text string, tokens ...string) {
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			*issues = append(*issues, path+" is missing "+quoteToken(token))
		}
	}
}

func beForbid(issues *[]string, path, text string, tokens ...string) {
	for _, token := range tokens {
		if strings.Contains(text, token) {
			*issues = append(*issues, path+" contains leftover "+quoteToken(token))
		}
	}
}

func quoteToken(token string) string {
	return "\"" + token + "\""
}

type beHeading struct {
	line  int
	level int
	title string
}

func beATXHeading(line string) (int, string, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return 0, "", false
	}
	content := line[indent:]
	level := 0
	for level < len(content) && content[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || (len(content) > level && content[level] != ' ' && content[level] != '\t') {
		return 0, "", false
	}
	title := strings.TrimSpace(content[level:])
	end := len(title)
	for end > 0 && title[end-1] == '#' {
		end--
	}
	if end < len(title) && end > 0 && (title[end-1] == ' ' || title[end-1] == '\t') {
		title = strings.TrimSpace(title[:end])
	}
	return level, title, true
}

func beFenceMarker(line string) (byte, int, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return 0, 0, false
	}
	content := line[indent:]
	if len(content) == 0 || (content[0] != '`' && content[0] != '~') {
		return 0, 0, false
	}
	marker := content[0]
	length := 0
	for length < len(content) && content[length] == marker {
		length++
	}
	return marker, length, length >= 3
}

func beFenceCloses(line string, marker byte, minimum int) bool {
	foundMarker, length, ok := beFenceMarker(line)
	if !ok || foundMarker != marker || length < minimum {
		return false
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	return strings.TrimSpace(line[indent+length:]) == ""
}

func beSetextHeading(line string) (int, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return 0, false
	}
	content := strings.TrimSpace(line)
	if content == "" || (content[0] != '=' && content[0] != '-') {
		return 0, false
	}
	for i := 1; i < len(content); i++ {
		if content[i] != content[0] {
			return 0, false
		}
	}
	if content[0] == '=' {
		return 1, true
	}
	return 2, true
}

func beHeadings(text string) ([]string, []beHeading) {
	lines := strings.Split(text, "\n")
	var headings []beHeading
	var fence byte
	fenceLength := 0
	for i, line := range lines {
		if fence != 0 {
			if beFenceCloses(line, fence, fenceLength) {
				fence = 0
				fenceLength = 0
			}
			continue
		}
		if marker, length, ok := beFenceMarker(line); ok {
			fence = marker
			fenceLength = length
			continue
		}
		if level, title, ok := beATXHeading(line); ok {
			headings = append(headings, beHeading{line: i, level: level, title: title})
			continue
		}
		if i+1 < len(lines) {
			if level, ok := beSetextHeading(lines[i+1]); ok && strings.TrimSpace(line) != "" {
				headings = append(headings, beHeading{line: i, level: level, title: strings.TrimSpace(line)})
			}
		}
	}
	return lines, headings
}

func beSectionEnd(lines []string, headings []beHeading, headingIndex int) int {
	heading := headings[headingIndex]
	for _, next := range headings[headingIndex+1:] {
		if next.level <= heading.level {
			return next.line
		}
	}
	return len(lines)
}

func beSection(text, headingLine string) (string, bool) {
	level, title, ok := beATXHeading(headingLine)
	if !ok {
		return "", false
	}
	lines, headings := beHeadings(text)
	for i, heading := range headings {
		if heading.level == level && heading.title == title {
			end := beSectionEnd(lines, headings, i)
			return strings.Join(lines[heading.line:end], "\n"), true
		}
	}
	return "", false
}

func beSectionContainingLine(text, token string) (string, bool, bool) {
	lines, headings := beHeadings(text)
	targetLine := -1
	for i, line := range lines {
		if strings.Contains(line, token) {
			targetLine = i
			break
		}
	}
	if targetLine == -1 {
		return "", false, false
	}
	headingIndex := -1
	for i, heading := range headings {
		if heading.line > targetLine {
			break
		}
		headingIndex = i
	}
	if headingIndex == -1 {
		return "", true, false
	}
	end := beSectionEnd(lines, headings, headingIndex)
	return strings.Join(lines[headings[headingIndex].line:end], "\n"), true, targetLine < end
}

func beRequireSection(issues *[]string, path, text, heading string, tokens ...string) {
	section, found := beSection(text, heading)
	if !found {
		*issues = append(*issues, path+" is missing section "+quoteToken(heading))
	}
	beRequire(issues, path, section, tokens...)
}

func beRequireExampleSection(issues *[]string, path, text, exampleLine string, tokens ...string) {
	section, foundLine, foundSection := beSectionContainingLine(text, exampleLine)
	if !foundLine {
		*issues = append(*issues, path+" is missing example line "+quoteToken(exampleLine))
	} else if !foundSection {
		*issues = append(*issues, path+" has no section containing example line "+quoteToken(exampleLine))
	}
	beRequire(issues, path, section, tokens...)
}

// beStripHTMLComments removes every <!-- … --> comment, so that only text a
// reader can see is checked. An unterminated comment removes the rest of the
// text, which then lacks the required tokens rather than passing.
func beStripHTMLComments(text string) string {
	var b strings.Builder
	for {
		start := strings.Index(text, "<!--")
		if start == -1 {
			b.WriteString(text)
			return b.String()
		}
		b.WriteString(text[:start])
		end := strings.Index(text[start+4:], "-->")
		if end == -1 {
			return b.String()
		}
		text = text[start+4+end+3:]
	}
}

// beHTMLSectionTag reports whether a <section opening tag carries attr as an
// attribute of its own: attr must follow whitespace, so text inside another
// attribute's value (data-x='id="try"') does not count.
func beHTMLSectionTag(tag, attr string) bool {
	for _, sep := range []string{" ", "\t", "\n"} {
		if strings.Contains(tag, sep+attr) {
			return true
		}
	}
	return false
}

// beHTMLSection returns the lines from the line whose <section opening tag
// carries attr through the first following line containing </section>.
// foundOpen reports the opening line, foundClose the closing line, and count
// how many <section opening tags carry attr.
func beHTMLSection(text, attr string) (string, bool, bool, int) {
	lines := strings.Split(text, "\n")
	section, foundOpen, foundClose, count := "", false, false, 0
	for i, line := range lines {
		start := strings.Index(line, "<section")
		if start == -1 {
			continue
		}
		tag := line[start:]
		if end := strings.Index(tag, ">"); end != -1 {
			tag = tag[:end]
		}
		if !beHTMLSectionTag(tag, attr) {
			continue
		}
		count++
		if count > 1 {
			continue
		}
		foundOpen = true
		for j := i + 1; j < len(lines); j++ {
			if strings.Contains(lines[j], "</section>") {
				section, foundClose = strings.Join(lines[i:j+1], "\n"), true
				break
			}
		}
	}
	return section, foundOpen, foundClose, count
}

func beRequireHTMLSection(issues *[]string, path, text, attr string, tokens ...string) {
	section, foundOpen, foundClose, count := beHTMLSection(text, attr)
	if !foundOpen {
		*issues = append(*issues, path+" is missing a <section with "+attr)
	} else if !foundClose {
		*issues = append(*issues, path+" has no </section> after the <section with "+attr)
	} else if count > 1 {
		*issues = append(*issues, path+" has more than one <section with "+attr)
	}
	beRequire(issues, path, section, tokens...)
}

func beReport(t *testing.T, id string, issues []string) {
	t.Helper()
	if len(issues) != 0 {
		t.Errorf("jaekit:violation=%s: %s", id, strings.Join(issues, "; "))
	}
}

// AC-1: the bundle and REVIEW template limit exceptions to two cases and
// distinguish prior content from a prohibition; ha repeats the limit.
func TestBaselineExceptionsAC1_TwoKinds(t *testing.T) {
	const id = "third-baseline-kind-remains"
	docs := beSealDocuments(t)
	var issues []string
	for _, path := range []string{beBundle, beReviewTmpl} {
		beRequire(&issues, path, beSealText(t, docs, path),
			"only in two cases",
			"already present at the base commit",
			"a prohibition such as",
			"not marked by default")
	}
	beRequire(&issues, beHA, beSealText(t, docs, beHA), "only in two cases")
	for _, doc := range docs {
		beForbid(&issues, doc.path, doc.text, "partly met at base")
	}
	beReport(t, id, issues)
}

// AC-2: reasons apply only to marked conditions; the old every-condition
// reason rule is absent throughout Seal's documents.
func TestBaselineExceptionsAC2_ReasonsForMarkedOnly(t *testing.T) {
	const id = "reasons-for-every-condition"
	docs := beSealDocuments(t)
	var issues []string
	for _, path := range []string{beBundle, beReviewTmpl} {
		beRequire(&issues, path, beSealText(t, docs, path), "for each marked condition only", "needs no reason")
	}
	for _, doc := range docs {
		beForbid(&issues, doc.path, doc.text,
			"whether it requires a baseline and why",
			"REVIEW explains each judgment",
			"per-condition judgment")
	}
	beReport(t, id, issues)
}

// AC-3: check baselines remain fixed before completion review, and no settled
// check-file phrasing replaces that timing in Seal or public guides.
func TestBaselineExceptionsAC3_RecordBeforeCompletion(t *testing.T) {
	const id = "baseline-timing-unchanged"
	sealDocs := beSealDocuments(t)
	publicDocs := bePublicGuides(t)
	var issues []string
	for _, path := range []string{beBundle, beHA} {
		text := beSealText(t, sealDocs, path)
		beRequire(&issues, path, text, beReviewPhrase)
	}
	beRequire(&issues, beBundle, beSealText(t, sealDocs, beBundle),
		"reviewed like fixed code",
		"passes or errs instead of violating",
		"fix the check",
		"record the baseline again")
	for _, path := range []string{"../guides/USAGE.en.md", "../guides/OPERATIONS.en.md"} {
		beRequire(&issues, path, bePublicText(t, publicDocs, path), beReviewPhrase)
	}
	for _, path := range []string{"../guides/USAGE.md", "../guides/OPERATIONS.md"} {
		beRequire(&issues, path, bePublicText(t, publicDocs, path), "마지막 검토 라운드 뒤, 완료 기록 직전에")
	}
	for _, doc := range sealDocs {
		beForbid(&issues, doc.path, doc.text, "once its check files are settled", "once their checks are settled")
	}
	var englishGuides, koreanGuides []beDocument
	for _, doc := range publicDocs {
		if !strings.HasPrefix(doc.path, "../guides/") {
			continue
		}
		if strings.HasSuffix(doc.path, ".en.md") {
			englishGuides = append(englishGuides, doc)
		} else {
			koreanGuides = append(koreanGuides, doc)
		}
	}
	if len(englishGuides) == 0 {
		t.Fatalf("English guide file list is empty")
	}
	if len(koreanGuides) == 0 {
		t.Fatalf("Korean guide file list is empty")
	}
	for _, doc := range englishGuides {
		beForbid(&issues, doc.path, doc.text, "once its check files are settled", "once their checks are settled")
	}
	for _, doc := range koreanGuides {
		beForbid(&issues, doc.path, doc.text, "검사 파일이 정해진 뒤")
	}
	beReport(t, id, issues)
}

// AC-4: a goal started with its marks retains those marks.
func TestBaselineExceptionsAC4_StartedGoalsKeepMarks(t *testing.T) {
	const id = "started-marks-rule-missing"
	bundle := beRead(t, beBundle)
	ha := beRead(t, beHA)
	if strings.Contains(bundle, "keeps the marks it started with") || strings.Contains(ha, "keeps the marks it started with") {
		return
	}
	beReport(t, id, []string{beBundle + " or " + beHA + " is missing token keeps the marks it started with"})
}

// AC-5 maintains the list of change conditions met without a baseline.
func TestBaselineExceptionsAC5_Maintain(t *testing.T) {
	bundle := beRead(t, beBundle)
	ha := beRead(t, beHA)
	var issues []string
	beRequire(&issues, beBundle, bundle, "list change conditions met without a baseline")
	beRequire(&issues, beHA, ha, "lists change conditions met without a baseline under `### 보지 않은 것`")
	if len(issues) != 0 {
		t.Errorf("%s", strings.Join(issues, "; "))
	}
}

// AC-7: public English and Korean guides describe baseline exceptions in the
// same terms and remove the old new-behavior failure promise.
func TestBaselineExceptionsAC7_PublicGuidesMatch(t *testing.T) {
	const id = "public-baseline-promise-remains"
	docs := bePublicGuides(t)
	var issues []string
	for _, path := range []string{"../README.md", "../guides/OVERVIEW.en.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.en.md"} {
		beRequire(&issues, path, bePublicText(t, docs, path),
			"already present at the base commit",
			"a prohibition such as")
	}
	for _, path := range []string{"../README.ko.md", "../guides/OVERVIEW.md", "../guides/USAGE.md", "../guides/OPERATIONS.md"} {
		beRequire(&issues, path, bePublicText(t, docs, path),
			"base commit에 이미 있는",
			"같은 금지 조건")
	}
	beRequire(&issues, "../README.md", bePublicText(t, docs, "../README.md"),
		"change conditions marked `요구`")
	beRequire(&issues, "../guides/OVERVIEW.en.md", bePublicText(t, docs, "../guides/OVERVIEW.en.md"),
		"change conditions marked `요구`")
	beRequire(&issues, "../README.ko.md", bePublicText(t, docs, "../README.ko.md"),
		"`요구`를 표시한 change 조건")
	beRequire(&issues, "../guides/OVERVIEW.md", bePublicText(t, docs, "../guides/OVERVIEW.md"),
		"`요구`를 표시한 change 조건")
	beRequire(&issues, "../guides/OPERATIONS.en.md", bePublicText(t, docs, "../guides/OPERATIONS.en.md"),
		"Up to `run-rules/6`, a check for every change condition must show")
	beRequire(&issues, "../guides/OPERATIONS.md", bePublicText(t, docs, "../guides/OPERATIONS.md"),
		"`run-rules/6`까지는 모든 change 조건의 검사가")
	beRequire(&issues, "../tools/site/home.html", beStripHTMLComments(bePublicText(t, docs, "../tools/site/home.html")),
		"already present at the base commit",
		"a prohibition such as",
		"base commit에 이미 있는",
		"같은 금지 조건")
	beRequireSection(&issues, "../guides/OVERVIEW.en.md", bePublicText(t, docs, "../guides/OVERVIEW.en.md"),
		"## What completion means, and what it does not", "can be met by a passing record")
	beRequireSection(&issues, "../guides/OVERVIEW.md", bePublicText(t, docs, "../guides/OVERVIEW.md"),
		"## 완료가 뜻하는 것과 뜻하지 않는 것", "통과 기록으로 충족될 수 있습니다")
	beRequireSection(&issues, "../guides/OVERVIEW.en.md", bePublicText(t, docs, "../guides/OVERVIEW.en.md"),
		"## How one goal goes through", "to show the record's shape", "is usually not marked")
	beRequireSection(&issues, "../guides/OVERVIEW.md", bePublicText(t, docs, "../guides/OVERVIEW.md"),
		"## 목표 하나가 지나가는 길", "기록의 모양을 보이려는", "보통 표시하지 않")
	beRequireExampleSection(&issues, "../README.md", bePublicText(t, docs, "../README.md"),
		"`ha log docs/specs/password-link 3`", "to show the record's shape", "is usually not marked")
	beRequireExampleSection(&issues, "../README.ko.md", bePublicText(t, docs, "../README.ko.md"),
		"`ha log docs/specs/password-link 3`", "기록의 모양을 보이려는", "보통 표시하지 않")
	beRequireHTMLSection(&issues, "../tools/site/home.html", beStripHTMLComments(bePublicText(t, docs, "../tools/site/home.html")),
		`id="try"`, "to show the record's shape", "is usually not marked", "기록의 모양을 보이려는", "보통 표시하지 않")
	englishForbidden := []string{
		"Mark conditions at risk of a false pass, such as",
		"For new behavior, the check must fail on the code before the change",
		"A check for new behavior counts only with a record of it failing",
		"each required condition on new behavior has records of its check failing",
		"where it must fail the way the declaration says",
		"or conditions partly met at base",
		"records the requirement violation before the change and satisfaction afterward",
	}
	koreanForbidden := []string{
		"거짓 통과 위험이 있는 조건을 고르고",
		"새로 생기는 동작이라면 그 검사가 바꾸기 전 코드에서는",
		"새 동작의 검사는 바꾸기 전 코드에서 그 동작이 없어 실패한 기록이 있어야 근거가 됩니다",
		"필수 조건 가운데 새 동작의 검사는 바꾸기 전에",
		"선언대로 실패하는지 보고",
		"base에서 일부가 이미 충족된 조건",
		"수정 전 요구 위반과 수정 뒤 충족을 기록합니다",
	}
	for _, doc := range docs {
		beForbid(&issues, doc.path, doc.text, englishForbidden...)
		beForbid(&issues, doc.path, doc.text, koreanForbidden...)
	}
	beReport(t, id, issues)
}
