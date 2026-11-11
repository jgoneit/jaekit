package plugins

// Checks of the review-summary goal: once a goal completes, Seal writes
// `## 검수 요약` in PROGRESS.md right after `## 현재`, so the owner can review
// a finished goal from one place. Each test observes fixed text in a fixed
// section of the current tree and opens every failure message with its
// violation marker, which ha's go-test-json adapter reads. A document or
// section the property requires that is missing is that violation. A file
// that exists but cannot be read is skipped, never a violation.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	rsmTemplate   = "seal/skills/seal/assets/templates/PROGRESS.md"
	rsmSkill      = "seal/skills/seal/SKILL.md"
	rsmReference  = "seal/skills/seal/references/bundle.md"
	rsmHa         = "seal/skills/seal/references/ha.md"
	rsmContractKo = "../contracts/bundle.md"
	rsmContractEn = "../contracts/bundle.en.md"
	rsmRoleKo     = "../contracts/role-card.md"
	rsmRoleEn     = "../contracts/role-card.en.md"

	rsmSummary      = "## 검수 요약"
	rsmRefSection   = "## Review summary"
	rsmKoSection    = "### 5.4 검수 요약"
	rsmEnSection    = "### 5.4 Review summary"
	rsmKoProgress   = "## 5. `PROGRESS.md`"
	rsmEnProgress   = "## 5. `PROGRESS.md`"
	rsmRefProgress  = "## PROGRESS.md"
	rsmKoCompletion = "## 6. 완료 보고"
	rsmEnCompletion = "## 6. Completion report"
	rsmRefReport    = "## Completion report"
)

// The seven headings of the review summary: all of them, in this order.
var rsmHeadings = []string{
	"### 바뀐 동작과 파일",
	"### 범위 밖이나 기존 것을 바꾼 곳",
	"### 근거",
	"### 누가 정했나",
	"### 직접 볼 것",
	"### 보지 않은 것",
	"### 자세히",
}

// The opening lines of the review summary, before its first heading.
var (
	rsmOpeningEn = []string{
		"the completion record's seq and commit, the assurance tier, how many required conditions are satisfied, and the review gate result",
		"one line on what the change does",
		"where to start looking",
	}
	rsmOpeningKo = []string{
		"완료 기록의 seq와 commit, 보증 등급, 필수 조건 충족 수, 독립 검토 gate의 결과",
		"이 변경이 하는 일 한 줄",
		"먼저 볼 항목",
	}
)

// The qualifier the existing completion-report reading instructions carry.
const (
	rsmQualKo = "0.1.24 이상으로 시작해 완료한 목표에서는 판단 문장이 `## 검수 요약`에 있습니다"
	rsmQualEn = "In a finished goal started with 0.1.24 or later, the judgment is in `## 검수 요약`"
)

// The endings a review finding can have, all of them.
var rsmEndings = []string{"`고침`", "`반박`", "`중복`", "`사용자의 답`", "`이슈로 보류`", "`자동 보류`", "`처리 없음`"}

// rsmRead returns a file of the tree. A missing file is the violation; a
// file that exists but cannot be read skips the test.
func rsmRead(t *testing.T, violation, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("jaekit:violation=%s %s does not exist", violation, path)
		return "", false
	}
	if err != nil {
		t.Skipf("cannot read %s: %v", path, err)
	}
	return string(data), true
}

// rsmLevel is the heading level of a Markdown line, or 0 when it is not a
// heading. Lines inside fenced code blocks are left to the caller.
func rsmLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// rsmFence tracks fenced code: given the open fence marker ("" outside
// fenced code) and a line, it returns the marker after the line and whether
// the line is fenced or a fence line. A fence opens with ``` or ~~~ and
// closes with the same marker.
func rsmFence(open, line string) (string, bool) {
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

// rsmSection returns the section that opens with the line equal to heading,
// up to the next heading of the same or a higher level outside fenced code.
func rsmSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	level := rsmLevel(heading)
	start := -1
	fence := ""
	for i, l := range lines {
		var fenced bool
		if fence, fenced = rsmFence(fence, l); fenced {
			continue
		}
		if start < 0 {
			if strings.TrimRight(l, " \t\r") == heading {
				start = i
			}
			continue
		}
		if lv := rsmLevel(l); lv > 0 && lv <= level {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// rsmNeedSection returns a section of a file, reporting a missing file or
// section as the violation.
func rsmNeedSection(t *testing.T, violation, path, heading string) (string, bool) {
	t.Helper()
	text, ok := rsmRead(t, violation, path)
	if !ok {
		return "", false
	}
	sec, ok := rsmSection(text, heading)
	if !ok {
		t.Errorf("jaekit:violation=%s %s has no section %q", violation, path, heading)
	}
	return sec, ok
}

// rsmNorm collapses every run of white space to one space, so line wrapping
// does not change what a test sees.
func rsmNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// rsmNeed reports every token the text lacks.
func rsmNeed(t *testing.T, violation, where, text string, tokens ...string) {
	t.Helper()
	norm := rsmNorm(text)
	for _, tok := range tokens {
		if !strings.Contains(norm, rsmNorm(tok)) {
			t.Errorf("jaekit:violation=%s %s lacks %q", violation, where, tok)
		}
	}
}

// rsmOrder reports tokens the text lacks or holds out of the given order.
func rsmOrder(t *testing.T, violation, where, text string, tokens ...string) {
	t.Helper()
	norm := rsmNorm(text)
	at := 0
	for _, tok := range tokens {
		i := strings.Index(norm[at:], rsmNorm(tok))
		if i < 0 {
			t.Errorf("jaekit:violation=%s %s lacks %q after the tokens before it", violation, where, tok)
			return
		}
		at += i + len(rsmNorm(tok))
	}
}

// rsmSubHeadings lists the level-three headings of a section outside fenced
// code, in order.
func rsmSubHeadings(sec string) []string {
	var out []string
	fence := ""
	for _, l := range strings.Split(sec, "\n") {
		var fenced bool
		if fence, fenced = rsmFence(fence, l); fenced {
			continue
		}
		if rsmLevel(l) == 3 {
			out = append(out, strings.TrimRight(l, " \t\r"))
		}
	}
	return out
}

// rsmBlocks splits a Markdown text into paragraphs and list items: a blank
// line or a line opening a list item ("- ", "* " or "1. ") starts a new
// block, and other lines continue the block before them, so line wrapping
// does not split a sentence.
func rsmBlocks(text string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, l := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			flush()
			continue
		}
		item := strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")
		if i := strings.Index(trimmed, ". "); i > 0 {
			digits := true
			for _, r := range trimmed[:i] {
				if r < '0' || r > '9' {
					digits = false
				}
			}
			item = item || digits
		}
		if item {
			flush()
		}
		cur = append(cur, l)
	}
	flush()
	return out
}

// rsmTopHeadings lists the level-two headings of a document outside fenced
// code, in order.
func rsmTopHeadings(text string) []string {
	var out []string
	fence := ""
	for _, l := range strings.Split(text, "\n") {
		var fenced bool
		if fence, fenced = rsmFence(fence, l); fenced {
			continue
		}
		if rsmLevel(l) == 2 {
			out = append(out, strings.TrimRight(l, " \t\r"))
		}
	}
	return out
}

// rsmRows lists the table rows of a section, in order.
func rsmRows(sec string) []string {
	var out []string
	for _, l := range strings.Split(sec, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "|") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// rsmRowAfter reports unless the row starting with second directly follows
// the row starting with first in a section's tables.
func rsmRowAfter(t *testing.T, violation, where, sec, first, second string) {
	t.Helper()
	rows := rsmRows(sec)
	for i, r := range rows {
		if strings.HasPrefix(r, first) {
			if i+1 < len(rows) && strings.HasPrefix(rows[i+1], second) {
				return
			}
			t.Errorf("jaekit:violation=%s %s: the row after %q does not start with %q", violation, where, first, second)
			return
		}
	}
	t.Errorf("jaekit:violation=%s %s has no row starting with %q", violation, where, first)
}

// rsmSub returns a level-three subsection of the template's review summary.
func rsmSub(t *testing.T, violation, summary, heading string) (string, bool) {
	t.Helper()
	sec, ok := rsmSection(summary, heading)
	if !ok {
		t.Errorf("jaekit:violation=%s %s: %s has no %q", violation, rsmTemplate, rsmSummary, heading)
	}
	return sec, ok
}

// rsmItemStart is the text of a block after its list marker ("- ", "* " or
// "1. "), or the block itself when it opens no list item.
func rsmItemStart(block string) string {
	tr := strings.TrimSpace(block)
	if strings.HasPrefix(tr, "- ") || strings.HasPrefix(tr, "* ") {
		return tr[2:]
	}
	if i := strings.Index(tr, ". "); i > 0 {
		for _, r := range tr[:i] {
			if r < '0' || r > '9' {
				return tr
			}
		}
		return tr[i+2:]
	}
	return tr
}

// rsmItemHeadings lists the headings that open the list items of a
// description section, as code spans such as "`### 근거`", in order.
func rsmItemHeadings(sec string) []string {
	var out []string
	for _, b := range rsmBlocks(sec) {
		start := rsmItemStart(b)
		if strings.HasPrefix(start, "`### ") {
			if end := strings.Index(start[1:], "`"); end > 0 {
				out = append(out, start[1:end+1])
			}
		}
	}
	return out
}

// rsmItem returns the list item of a description section that opens with
// the heading as a code span.
func rsmItem(sec, heading string) (string, bool) {
	for _, b := range rsmBlocks(sec) {
		if strings.HasPrefix(rsmItemStart(b), "`"+heading+"`") {
			return b, true
		}
	}
	return "", false
}

// rsmNeedItem asks the English tokens of the template's subsection under
// heading and of the list item that opens with heading in the reference
// and the English contract, and the Korean tokens of that item in the
// Korean contract, so a rule stated under another heading does not count.
func rsmNeedItem(t *testing.T, violation, heading string, en, ko []string) {
	t.Helper()
	if summary, ok := rsmNeedSection(t, violation, rsmTemplate, rsmSummary); ok {
		if sub, ok := rsmSub(t, violation, summary, heading); ok {
			rsmNeed(t, violation, rsmTemplate+" "+heading, sub, en...)
		}
	}
	for _, d := range []struct {
		path, section string
		tokens        []string
	}{{rsmReference, rsmRefSection, en}, {rsmContractEn, rsmEnSection, en}, {rsmContractKo, rsmKoSection, ko}} {
		sec, ok := rsmNeedSection(t, violation, d.path, d.section)
		if !ok {
			continue
		}
		item, ok := rsmItem(sec, heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s %s has no item opening with `%s`", violation, d.path, d.section, heading)
			continue
		}
		rsmNeed(t, violation, d.path+" "+d.section+" item "+heading, item, d.tokens...)
	}
}

// rsmDoc is one description of the review summary and where it is.
type rsmDoc struct{ where, sec string }

// rsmDocs are the English descriptions of the review summary that must say
// the same thing, in a fixed order: the template's section, the reference
// section and the English contract section.
func rsmDocs(t *testing.T, violation string) []rsmDoc {
	t.Helper()
	var out []rsmDoc
	for _, d := range []struct{ path, heading string }{
		{rsmTemplate, rsmSummary}, {rsmReference, rsmRefSection}, {rsmContractEn, rsmEnSection},
	} {
		if sec, ok := rsmNeedSection(t, violation, d.path, d.heading); ok {
			out = append(out, rsmDoc{d.path + " " + d.heading, sec})
		}
	}
	return out
}

// rsmNeedAll asks the English token list of every English description and
// the Korean token list of the Korean contract section.
func rsmNeedAll(t *testing.T, violation string, en, ko []string) {
	t.Helper()
	for _, d := range rsmDocs(t, violation) {
		rsmNeed(t, violation, d.where, d.sec, en...)
	}
	if sec, ok := rsmNeedSection(t, violation, rsmContractKo, rsmKoSection); ok {
		rsmNeed(t, violation, rsmContractKo+" "+rsmKoSection, sec, ko...)
	}
}

// AC-1: `## 검수 요약` is the section right after `## 현재`, written after a
// completion record as of it, rewritten after a new one, and not written
// before completion or at a stop.
func TestReviewSummaryAC1_PlacementAndTiming(t *testing.T) {
	const v = "review-summary-placement-missing"
	if tmpl, ok := rsmRead(t, v, rsmTemplate); ok {
		heads := rsmTopHeadings(tmpl)
		found := false
		for i, h := range heads {
			if h == "## 현재" {
				found = true
				if i+1 >= len(heads) || heads[i+1] != rsmSummary {
					t.Errorf("jaekit:violation=%s %s: the section after `## 현재` is not %q", v, rsmTemplate, rsmSummary)
				}
			}
		}
		if !found {
			t.Errorf("jaekit:violation=%s %s has no `## 현재`", v, rsmTemplate)
		}
	}
	for _, d := range []struct{ path, heading string }{
		{rsmReference, rsmRefProgress}, {rsmContractEn, rsmEnProgress}, {rsmContractKo, rsmKoProgress},
	} {
		if sec, ok := rsmNeedSection(t, v, d.path, d.heading); ok {
			rsmRowAfter(t, v, d.path+" "+d.heading, sec, "| `## 현재` |", "| `## 검수 요약` |")
		}
	}
	rsmNeedAll(t, v, []string{
		"Written once `ha done` records completion, as of that completion record",
		"rewritten as of each new completion record after a reopen",
		"not written before completion or when work stops at `needs_user`, `blocked` or `budget_exhausted`",
	}, []string{
		"`ha done`이 완료 기록을 남긴 뒤 그 완료 기록 기준으로 쓴다",
		"목표를 다시 열어 완료를 다시 기록하면 새 완료 기록 기준으로 다시 쓴다",
		"완료 기록 전과 멈춤(`needs_user`, `blocked`, `budget_exhausted`) 때는 쓰지 않는다",
	})
	if skill, ok := rsmRead(t, v, rsmSkill); ok {
		rsmNeed(t, v, rsmSkill, skill,
			"At each completion record, judgment goes only to `## 검수 요약` after `## 현재`, never at stops")
	}
}

// AC-2: the review summary has first lines and exactly the seven headings,
// in order, with `없음` under one that has nothing to list.
func TestReviewSummaryAC2_Structure(t *testing.T) {
	const v = "review-summary-structure-missing"
	if sec, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		got := rsmSubHeadings(sec)
		if strings.Join(got, "\n") != strings.Join(rsmHeadings, "\n") {
			t.Errorf("jaekit:violation=%s %s: the headings of %s are %q, not %q", v, rsmTemplate, rsmSummary, got, rsmHeadings)
		}
		opening := sec
		if i := strings.Index(sec, "\n"+rsmHeadings[0]); i >= 0 {
			opening = sec[:i]
		}
		rsmNeed(t, v, rsmTemplate+" "+rsmSummary+" before "+rsmHeadings[0], opening, rsmOpeningEn...)
	}
	for _, d := range []struct {
		path, section string
		opening       []string
	}{{rsmReference, rsmRefSection, rsmOpeningEn}, {rsmContractEn, rsmEnSection, rsmOpeningEn}, {rsmContractKo, rsmKoSection, rsmOpeningKo}} {
		sec, ok := rsmNeedSection(t, v, d.path, d.section)
		if !ok {
			continue
		}
		if got := rsmItemHeadings(sec); strings.Join(got, "\n") != strings.Join(rsmHeadings, "\n") {
			t.Errorf("jaekit:violation=%s %s %s lists the headings %q, not %q", v, d.path, d.section, got, rsmHeadings)
		}
		rsmOrder(t, v, d.path+" "+d.section, sec, append(append([]string{}, d.opening...), "`"+rsmHeadings[0]+"`")...)
	}
	rsmNeedAll(t, v, []string{"write `없음` under a heading with nothing to list"}, []string{"해당하는 것이 없는 항목에는 `없음`을 쓴다"})
	rsmNeedItem(t, v, rsmHeadings[6], []string{"the documents and sections that hold the detail"}, []string{"세부 기록이 있는 문서와 절"})
}

// AC-3: the changed behavior and files table lists every changed file but
// the goal documents, check files included.
func TestReviewSummaryAC3_ChangedFiles(t *testing.T) {
	const v = "changed-files-table-missing"
	if summary, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		if sub, ok := rsmSub(t, v, summary, rsmHeadings[0]); ok {
			rsmNeed(t, v, rsmTemplate+" "+rsmHeadings[0], sub, "| 바뀐 동작 | 조건 | 파일 |")
		}
	}
	en := []string{
		"Every file changed from the goal's base to the completion commit, goal documents excepted, appears in a row",
		"this goal's check files have rows too",
	}
	rsmNeedItem(t, v, rsmHeadings[0], en, []string{
		"`바뀐 동작 | 조건 | 파일`",
		"목표의 base부터 완료 commit까지 바뀐 파일은 목표 문서를 빼고 모두 어느 행엔가 나온다",
		"이 목표의 검사 파일도 행으로 둔다",
	})
	for _, d := range []struct{ path, section string }{{rsmReference, rsmRefSection}, {rsmContractEn, rsmEnSection}} {
		if sec, ok := rsmNeedSection(t, v, d.path, d.section); ok {
			if item, ok := rsmItem(sec, rsmHeadings[0]); ok {
				rsmNeed(t, v, d.path+" "+d.section+" item "+rsmHeadings[0], item, "`바뀐 동작 | 조건 | 파일`")
			}
		}
	}
}

// AC-4: changes outside the scope, to existing tests and to an earlier
// goal's checks are listed with the user's words or the reason.
func TestReviewSummaryAC4_ScopeChanges(t *testing.T) {
	const v = "scope-changes-missing"
	rsmNeedItem(t, v, rsmHeadings[1], []string{
		"files outside the SPEC scope",
		"changes to existing tests",
		"changes to checks an earlier goal left",
		"where the user's words allowing it are, or the reason",
	}, []string{
		"SPEC 범위 밖의 파일 변경",
		"기존 시험의 변경",
		"이전 목표가 남긴 검사의 변경",
		"허락한 사용자의 말이 있는 곳이나 이유",
	})
}

// AC-5: the evidence names what each condition's check observed and where
// it passed, and a round table gives every finding one ending.
func TestReviewSummaryAC5_Evidence(t *testing.T) {
	const v = "evidence-rules-missing"
	if summary, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		if sub, ok := rsmSub(t, v, summary, rsmHeadings[2]); ok {
			rsmNeed(t, v, rsmTemplate+" "+rsmHeadings[2], sub, "| 라운드 | 지적 | 끝 |")
		}
	}
	en := append([]string{
		"for every condition, what the check that satisfied it observed and the commit where it passed, never by seq alone",
		"one row for every review round, a round with no findings included",
		"Every finding of the round ends in exactly one of",
	}, rsmEndings...)
	ko := append([]string{
		"조건마다 그 조건을 충족한 검사가 무엇을 확인했는지와 어느 commit에서 통과했는지",
		"seq만으로 가리키지 않는다",
		"독립 검토 라운드마다 한 행인 `라운드 | 지적 | 끝` 표",
		"지적이 없는 라운드도 행을 둔다",
		"라운드의 지적은 하나도 빠짐없이 끝 하나로 분류된다",
	}, rsmEndings...)
	rsmNeedItem(t, v, rsmHeadings[2], en, ko)
	for _, d := range []struct{ path, section string }{{rsmReference, rsmRefSection}, {rsmContractEn, rsmEnSection}} {
		if sec, ok := rsmNeedSection(t, v, d.path, d.section); ok {
			if item, ok := rsmItem(sec, rsmHeadings[2]); ok {
				rsmNeed(t, v, d.path+" "+d.section+" item "+rsmHeadings[2], item, "`라운드 | 지적 | 끝`")
			}
		}
	}
}

// AC-6: what to inspect by hand, highest risk first, holds every deferred
// and undisposed finding whatever ha report shows.
func TestReviewSummaryAC6_ToInspect(t *testing.T) {
	const v = "inspect-list-missing"
	rsmNeedItem(t, v, rsmHeadings[4], []string{
		"ordered by risk, the highest at the top, one or two lines each",
		"every item the `직접 볼 것` line of `ha report` lists",
		"whatever that line shows",
		"every deferred finding with its issue",
		"every finding left with no disposition",
		"executor assumptions that read a SPEC sentence",
		"conditions met by checks that observe fixed text only",
		"`manual` conditions",
		"the changes under `### 범위 밖이나 기존 것을 바꾼 곳`",
	}, []string{
		"위험이 큰 순서",
		"항목마다 한두 줄",
		"`ha report`의 `직접 볼 것` 줄이 보이는 항목 전부",
		"그 줄이 무엇을 보이든",
		"보류한 지적 전부와 그 이슈",
		"처리 없이 남은 지적 전부",
		"SPEC 문장을 해석해서 정한 실행자 가정",
		"고정 문구만 확인하는 검사로 충족한 조건",
		"`manual` 조건",
		"`### 범위 밖이나 기존 것을 바꾼 곳`의 변경",
	})
}

// AC-7: decisions in three groups, interpretations marked, and what was not
// observed.
func TestReviewSummaryAC7_DeciderAndUnobserved(t *testing.T) {
	const v = "decider-unobserved-missing"
	rsmNeedItem(t, v, rsmHeadings[3], []string{
		"decided by the user, with where their words are",
		"proposed by the agent and accepted by the user",
		"executor assumptions nobody was asked about",
		"Mark an assumption that reads a SPEC sentence",
	}, []string{
		"사용자가 정한 것(사용자의 말이 있는 곳)",
		"에이전트가 제안하고 사용자가 받아들인 것",
		"묻지 않고 정한 실행자 가정",
		"해석한 가정에는 그렇다는 표시를 붙인다",
	})
	rsmNeedItem(t, v, rsmHeadings[5], []string{
		"Unobserved scope and substitutes",
		"assurance `local`",
		"the executor wrote the checks",
		"not evidence of deployment, installation or a real model call",
	}, []string{
		"관측하지 않은 범위와 대체 환경",
		"보증 수준 `local`",
		"검사를 실행자가 썼다는 사실",
		"배포·설치·실제 모델 실행의 증거가 아니라는 사실",
	})
}

// AC-8: at completion the completion report holds the ha report output and
// one line pointing to the review summary; a stop keeps today's report.
func TestReviewSummaryAC8_CompletionReport(t *testing.T) {
	const v = "completion-report-relation-missing"
	if sec, ok := rsmNeedSection(t, v, rsmTemplate, "## 완료 보고"); ok {
		rsmNeed(t, v, rsmTemplate+" ## 완료 보고", sec,
			"add only one line pointing to `## 검수 요약`, which holds the judgment",
			"When work stops, or for a goal started with an earlier Seal, below it, only judgment")
	}
	for _, d := range []struct{ path, heading string }{{rsmReference, rsmRefReport}, {rsmContractEn, rsmEnCompletion}} {
		if sec, ok := rsmNeedSection(t, v, d.path, d.heading); ok {
			rsmNeed(t, v, d.path+" "+d.heading, sec,
				"at completion `## 완료 보고` holds item 1 and one line pointing to `## 검수 요약`",
				"item 2 lives only in `## 검수 요약`",
				"When work stops, both parts stay as above")
		}
	}
	if sec, ok := rsmNeedSection(t, v, rsmContractKo, rsmKoCompletion); ok {
		rsmNeed(t, v, rsmContractKo+" "+rsmKoCompletion, sec,
			"완료 기록 뒤의 완료 보고는 1의 출력과 `## 검수 요약`을 가리키는 한 줄이다",
			"2의 판단 문장은 `## 검수 요약`에만 쓴다",
			"멈출 때는 위 두 부분 그대로다")
	}
	if skill, ok := rsmRead(t, v, rsmSkill); ok {
		rsmNeed(t, v, rsmSkill, skill, "`## 완료 보고` points there")
	}
	// Every judgment item that moves out of the completion report has a
	// place in the review summary.
	rsmNeedAll(t, v, []string{
		"the condition each contributed to under `### 바뀐 동작과 파일`",
		"assumed open decisions under `### 누가 정했나`",
		"`manual` conditions under `### 직접 볼 것`",
		"the expected-result grounds, the actual observation boundary, the test method and the review stages actually taken",
		"the review each required and the one followed) under `### 근거`",
		"the unobserved scope and substitute environments under `### 보지 않은 것`",
		"where the independent review results are under `### 자세히`",
		"whether the head to be sent or merged is the completion record's commit, with the fact that the completion does not cover a difference unless `ha status` at that head still names that completion record, in the opening lines",
	}, []string{
		"바뀐 파일과 각 파일이 기여한 조건은 `### 바뀐 동작과 파일`",
		"가정한 미결정은 `### 누가 정했나`",
		"`manual` 조건은 `### 직접 볼 것`",
		"기대 결과의 근거·실제 관측 경계·시험 방법과 실제로 받은 검토 단계",
		"각각이 요구한 검토와 따른 쪽)는 `### 근거`",
		"미관측 범위와 대체 환경은 `### 보지 않은 것`",
		"독립 검토 결과의 위치는 `### 자세히`",
		"보내거나 병합하려는 head가 완료 기록의 commit인지와, 다르면 그 차이를 완료가 덮지 않는다는 사실(그 head에서 `ha status`가 같은 완료 기록을 계속 보이면 덮는다)은 첫 줄들에 쓴다",
	})
	if card, ok := rsmRead(t, v, rsmRoleKo); ok {
		rsmNeed(t, v, rsmRoleKo, card,
			"seal 0.1.24 이상으로 시작한 목표는 완료 기록 뒤의 판단 문장을 `## 검수 요약`에 두고, 완료 보고에는 `ha report` 출력과 그 절을 가리키는 한 줄만 둔다")
	}
	if card, ok := rsmRead(t, v, rsmRoleEn); ok {
		rsmNeed(t, v, rsmRoleEn, card,
			"For a goal started with seal 0.1.24 or later, the judgment sentences after the completion record go in `## 검수 요약`, and the completion report holds only the `ha report` output and one line pointing to that section")
	}
}

// AC-10: the reading rules keep the summary short. The template, the
// reference and both contracts state them; the skill links the reference
// section and keeps its rule against copying findings (the user's answers of
// seq 9 and seq 30).
func TestReviewSummaryAC10_ReadingRules(t *testing.T) {
	const v = "reading-rules-missing"
	rsmNeedAll(t, v, []string{
		"write in the user's language",
		"say what a record is, with its seq only as an aid",
		"keep each item short",
		"never copy check output or finding text; a finding is its ID, one line and its ending",
	}, []string{
		"사용자의 언어로 쓴다",
		"기록은 무엇인지를 먼저 쓰고 seq는 보조로만 붙인다",
		"항목마다 짧게 쓴다",
		"검증 출력과 지적 원문을 옮겨 적지 않는다",
		"지적은 ID, 한 줄 요약, 끝만 적는다",
	})
}

// AC-11: the review summary applies from seal 0.1.24; earlier goals keep the
// form they started with and no existing PROGRESS is converted.
func TestReviewSummaryAC11_EarlierGoals(t *testing.T) {
	const v = "earlier-goal-format-missing"
	en := []string{
		"`## 검수 요약` applies to goals whose start record names seal 0.1.24 or later",
		"A goal started with an earlier Seal keeps the PROGRESS and report form it started with",
		"existing goals' `PROGRESS.md` is never converted to the new form",
	}
	for _, d := range []struct{ path, heading string }{{rsmReference, rsmRefSection}, {rsmContractEn, rsmEnSection}} {
		if sec, ok := rsmNeedSection(t, v, d.path, d.heading); ok {
			rsmNeed(t, v, d.path+" "+d.heading, sec, en...)
		}
	}
	if sec, ok := rsmNeedSection(t, v, rsmContractKo, rsmKoSection); ok {
		rsmNeed(t, v, rsmContractKo+" "+rsmKoSection, sec,
			"`## 검수 요약`은 seal 0.1.24 이상으로 시작한 목표에 쓴다",
			"그 전 Seal로 시작한 목표는 시작할 때의 PROGRESS와 보고 형식을 유지한다",
			"이미 있는 목표의 `PROGRESS.md`는 새 형식으로 바꾸지 않는다")
	}
	if sec, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		rsmNeed(t, v, rsmTemplate+" "+rsmSummary, sec, "Only for a goal started with seal 0.1.24 or later")
	}
	if skill, ok := rsmRead(t, v, rsmSkill); ok {
		rsmNeed(t, v, rsmSkill, skill, "A goal started with an earlier Seal keeps its saved rules, formats and budget")
	}
}

// AC-15: the public guides and the READMEs tell the user to read the review
// summary first and say where it is, what it holds and how it relates to the
// completion report.
func TestReviewSummaryAC15_Guides(t *testing.T) {
	const v = "guides-review-summary-missing"
	ko := []string{
		"0.1.24 이상으로 시작한 목표",
		"완료 기록 뒤 `## 현재` 바로 다음의 `## 검수 요약`",
		"완료 기록, 보증 등급, 충족한 필수 조건 수, 검토 gate 결과, 이 변경이 하는 일, 먼저 볼 것",
		"바뀐 동작과 파일, 범위 밖이나 기존 것을 바꾼 곳, 근거, 누가 정했나, 직접 볼 것, 보지 않은 것, 자세히",
		"사용자의 언어로 쓰고",
		"기록은 무엇인지를 먼저 쓰며",
		"해당하는 것이 없는 항목에는 `없음`을 쓰고",
		"목표를 다시 열어 완료를 다시 기록하면 새 완료 기록 기준으로 다시 씁니다",
		"항목마다 짧게 쓰고 seq는 보조로만 붙입니다",
		"검증 출력과 지적 원문은 옮기지 않고 지적은 ID, 한 줄 요약, 끝만 적습니다",
		"Seal의 판단은 이 절에만 있고, `## 완료 보고`에는 `ha report` 출력과 이 절을 가리키는 한 줄이 있습니다",
		"완료한 목표는 `## 검수 요약`부터 읽습니다",
	}
	en := []string{
		"goals started with 0.1.24 or later",
		"after completion, `## 검수 요약` right after `## 현재`",
		"the completion record, the assurance tier, how many required conditions are satisfied, the review gate result, what the change does and where to start looking",
		"changed behavior and files, scope changes, evidence, who decided, what to inspect, what was not observed, and details",
		"written in the user's language",
		"says what each record is",
		"A heading with nothing to list says `없음`",
		"after a reopen the section is rewritten as of the new completion record",
		"each item is short, with seq numbers only as an aid",
		"no check output or finding text is copied; a finding is its ID, one line and its ending",
		"Seal's judgment is only there, and `## 완료 보고` holds the `ha report` output and one line pointing to it",
		"For a finished goal, read `## 검수 요약` first",
	}
	for _, p := range []string{"../guides/USAGE.md", "../guides/OPERATIONS.md", "../guides/OVERVIEW.md", "../README.ko.md"} {
		if text, ok := rsmRead(t, v, p); ok {
			rsmNeed(t, v, p, text, ko...)
		}
	}
	for _, p := range []string{"../guides/USAGE.en.md", "../guides/OPERATIONS.en.md", "../guides/OVERVIEW.en.md", "../README.md"} {
		if text, ok := rsmRead(t, v, p); ok {
			rsmNeed(t, v, p, text, en...)
		}
	}
	if text, ok := rsmRead(t, v, "../README.ko.md"); ok {
		rsmNeed(t, v, "../README.ko.md", text, "현재 상태, 검수 요약, 타임라인, 완료 보고")
	}
	if text, ok := rsmRead(t, v, "../README.md"); ok {
		rsmNeed(t, v, "../README.md", text, "current state, review summary, timeline, completion report")
	}
	// The reading instructions the guides and READMEs already had now say
	// where the judgment of a finished 0.1.24 goal is, in the same paragraph
	// or list item.
	for _, q := range []struct{ path, anchor, token string }{
		{"../guides/USAGE.md", "을 먼저 읽고, 그 아래에서 완료 기록과 Seal의 판단 문장을 봅니다", rsmQualKo},
		{"../guides/USAGE.en.md", "then the completion record and Seal's judgment below them", rsmQualEn},
		{"../guides/OPERATIONS.md", "맨 위 다섯 줄을 먼저 읽고 아래에서 완료 기록과 Seal의 판단 문장을 봅니다", rsmQualKo},
		{"../guides/OPERATIONS.en.md", "read the five lines at the top, then the completion record and Seal's judgment below them", rsmQualEn},
		{"../guides/OVERVIEW.md", "8. **보고를 읽습니다.**", rsmQualKo},
		{"../guides/OVERVIEW.en.md", "8. **You read the report.**", rsmQualEn},
		{"../README.ko.md", "그 아래에 자기 판단만 덧붙입니다", rsmQualKo},
		{"../README.md", "then adds only its own judgment below it", rsmQualEn},
	} {
		text, ok := rsmRead(t, v, q.path)
		if !ok {
			continue
		}
		found := false
		for _, block := range rsmBlocks(text) {
			if strings.Contains(rsmNorm(block), rsmNorm(q.anchor)) {
				found = true
				rsmNeed(t, v, q.path+" (the paragraph or item with "+q.anchor+")", block, q.token)
			}
		}
		if !found {
			t.Errorf("jaekit:violation=%s %s has no paragraph or item with %q", v, q.path, q.anchor)
		}
	}
	// The contracts, the template and the reference say the user starts
	// there; the skill states where and when in one sentence and links the
	// reference section, which holds the rest (the user's answer of seq 9).
	rsmNeedAll(t, v, []string{"where the user starts reviewing a finished goal"}, []string{"사용자가 완료한 목표의 검수를 시작하는 절이다"})
	if skill, ok := rsmRead(t, v, rsmSkill); ok {
		rsmNeed(t, v, rsmSkill, skill, "([format](references/bundle.md#review-summary))")
	}
}

// rsmRun runs a command in dir with env and returns its combined output.
// A command that cannot start or exits nonzero returns an error.
func rsmRun(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// AC-12 (maintain): a goal whose PROGRESS.md is this tree's template, with
// `## 검수 요약` right after `## 현재`, works with this tree's ha as before.
// The test builds ha and makes a synthetic repository outside the tree whose
// PROGRESS.md is the template with only the goal name and the task state
// filled in. ha lint finds no problem before the start; after ha done, a
// commit that fills the review summary in PROGRESS.md alone leaves ha lint
// clean and ha status complete with its completion record. Every input
// failure (go, git or ha that cannot start or exits nonzero, an unreadable
// template) fails the test: this maintain condition is judged by the exit
// code, so a skip would read as a pass.
func TestReviewSummaryAC12_TemplateBundle(t *testing.T) {
	tmplData, err := os.ReadFile(rsmTemplate)
	if err != nil {
		t.Fatalf("cannot read %s: %v", rsmTemplate, err)
	}
	skill, err := filepath.Abs(rsmSkill)
	if err != nil {
		t.Fatalf("cannot resolve %s: %v", rsmSkill, err)
	}
	ha := filepath.Join(t.TempDir(), "ha")
	if out, err := rsmRun("..", os.Environ(), "go", "build", "-o", ha, "./cmd/ha"); err != nil {
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
		out, err := rsmRun(dir, env, name, args...)
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
	summary, ok := rsmSection(progress, rsmSummary)
	if !ok {
		t.Fatalf("the template-made PROGRESS.md has no %s", rsmSummary)
	}
	filled := []string{rsmSummary, "완료 기록 기준. 필수 조건 1개 중 1개 충족. git이 돈다. 먼저 볼 것: 없음", ""}
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

// AC-16: the Seal version goes up to 0.1.24 in every manifest, the skill and
// the marketplace, and the version list names this change. The source Seal is
// now 0.1.25, which passes decisions and invariant places to review rounds.
func TestReviewSummaryAC16_Version(t *testing.T) {
	const v = "seal-version-not-raised"
	const want = "0.1.26"
	for _, p := range []string{"seal/.claude-plugin/plugin.json", "seal/.codex-plugin/plugin.json"} {
		text, ok := rsmRead(t, v, p)
		if !ok {
			continue
		}
		var m struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Skipf("cannot parse %s: %v", p, err)
		}
		if m.Version != want {
			t.Errorf("jaekit:violation=%s %s version is %q, not %s", v, p, m.Version, want)
		}
	}
	if text, ok := rsmRead(t, v, "../.claude-plugin/marketplace.json"); ok {
		var m struct {
			Plugins []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"plugins"`
		}
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Skipf("cannot parse the marketplace: %v", err)
		}
		found := false
		for _, p := range m.Plugins {
			if p.Name == "seal" {
				found = true
				if p.Version != want {
					t.Errorf("jaekit:violation=%s the marketplace lists seal %q, not %s", v, p.Version, want)
				}
			}
		}
		if !found {
			t.Errorf("jaekit:violation=%s the marketplace lists no seal plugin", v)
		}
	}
	if skill, ok := rsmRead(t, v, rsmSkill); ok {
		rsmNeed(t, v, rsmSkill, skill, "version: \"0.1.26\"")
	}
	if ha, ok := rsmRead(t, v, rsmHa); ok {
		rsmNeed(t, v, rsmHa, ha, "Seal 0.1.24 adds the review summary (`## 검수 요약`) in `PROGRESS.md` without a Core change.")
	}
}

// The section helpers skip fenced code opened with ``` or ~~~, so a heading
// inside an example block is not a heading of the document.
func TestReviewSummaryHelpersSkipFences(t *testing.T) {
	text := "## A\n~~~markdown\n### fake\n## fake\n~~~\n### real\n```\n### also fake\n~~~\n```\n## B\n"
	if got := rsmTopHeadings(text); strings.Join(got, "|") != "## A|## B" {
		t.Errorf("rsmTopHeadings = %q", got)
	}
	sec, ok := rsmSection(text, "## A")
	if !ok {
		t.Fatalf("no section ## A")
	}
	if got := rsmSubHeadings(sec); strings.Join(got, "|") != "### real" {
		t.Errorf("rsmSubHeadings = %q", got)
	}
}

// rsmBlocks keeps a wrapped sentence in one block and splits list items.
func TestReviewSummaryHelpersBlocks(t *testing.T) {
	text := "intro line\ncontinued here\n\n- item one\n  wrapped\n- item two\n8. step eight\n   wrapped\n"
	got := rsmBlocks(text)
	want := []string{"intro line\ncontinued here", "- item one\n  wrapped", "- item two", "8. step eight\n   wrapped"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("rsmBlocks = %q, want %q", got, want)
	}
}

// rsmItemHeadings reads the headings that open list items, numbered or not,
// and ignores a heading named inside an item.
func TestReviewSummaryHelpersItems(t *testing.T) {
	sec := "## X\n\n- `### 가` (a): see `### 나` below\n  1. `### 나`: b\n- plain item\n- `### 다` (c)\n"
	if got := rsmItemHeadings(sec); strings.Join(got, "|") != "### 가|### 나|### 다" {
		t.Errorf("rsmItemHeadings = %q", got)
	}
	if item, ok := rsmItem(sec, "### 나"); !ok || !strings.Contains(item, ": b") {
		t.Errorf("rsmItem(### 나) = %q, %v", item, ok)
	}
}
