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

// rsmSection returns the section that opens with the line equal to heading,
// up to the next heading of the same or a higher level outside fenced code.
func rsmSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	level := rsmLevel(heading)
	start := -1
	fence := false
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") {
			fence = !fence
			continue
		}
		if fence {
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

// rsmTopHeadings lists the level-two headings of a document outside fenced
// code, in order.
func rsmTopHeadings(text string) []string {
	var out []string
	fence := false
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if !fence && rsmLevel(l) == 2 {
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
			"`## 검수 요약` right after `## 현재`",
			"as of each completion record",
			"never before completion or when work stops")
	}
}

// AC-2: the review summary has first lines and exactly the seven headings,
// in order, with `없음` under one that has nothing to list.
func TestReviewSummaryAC2_Structure(t *testing.T) {
	const v = "review-summary-structure-missing"
	if sec, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		var got []string
		for _, l := range strings.Split(sec, "\n") {
			if rsmLevel(l) == 3 {
				got = append(got, strings.TrimRight(l, " \t\r"))
			}
		}
		if strings.Join(got, "\n") != strings.Join(rsmHeadings, "\n") {
			t.Errorf("jaekit:violation=%s %s: the headings of %s are %q, not %q", v, rsmTemplate, rsmSummary, got, rsmHeadings)
		}
	}
	plain := make([]string, len(rsmHeadings))
	for i, h := range rsmHeadings {
		plain[i] = "`" + h + "`"
	}
	for _, d := range rsmDocs(t, v) {
		if strings.HasPrefix(d.where, rsmTemplate) {
			continue
		}
		rsmOrder(t, v, d.where, d.sec, plain...)
	}
	if sec, ok := rsmNeedSection(t, v, rsmContractKo, rsmKoSection); ok {
		rsmOrder(t, v, rsmContractKo+" "+rsmKoSection, sec, plain...)
	}
	rsmNeedAll(t, v, []string{
		"the completion record's seq and commit, the assurance tier, how many required conditions are satisfied, and the review gate result",
		"one line on what the change does",
		"what to look at first",
		"write `없음` under a heading with nothing to list",
	}, []string{
		"완료 기록의 seq와 commit, 보증 등급, 필수 조건 충족 수, 독립 검토 gate의 결과",
		"이 변경이 하는 일 한 줄",
		"먼저 볼 항목",
		"해당하는 것이 없는 항목에는 `없음`을 쓴다",
	})
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
	rsmNeedAll(t, v, []string{
		"Every file changed from the goal's base to the completion commit, goal documents excepted, appears in a row",
		"this goal's check files have rows too",
	}, []string{
		"`바뀐 동작 | 조건 | 파일`",
		"목표의 base부터 완료 commit까지 바뀐 파일은 목표 문서를 빼고 모두 어느 행엔가 나온다",
		"이 목표의 검사 파일도 행으로 둔다",
	})
	for _, d := range rsmDocs(t, v) {
		if !strings.HasPrefix(d.where, rsmTemplate) {
			rsmNeed(t, v, d.where, d.sec, "`바뀐 동작 | 조건 | 파일`")
		}
	}
}

// AC-4: changes outside the scope, to existing tests and to an earlier
// goal's checks are listed with the user's words or the reason.
func TestReviewSummaryAC4_ScopeChanges(t *testing.T) {
	const v = "scope-changes-missing"
	if summary, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		rsmSub(t, v, summary, rsmHeadings[1])
	}
	rsmNeedAll(t, v, []string{
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
		"what the check that satisfied it observed and the commit where it passed, never by seq alone",
		"Every finding of the round ends in exactly one of",
	}, rsmEndings...)
	ko := append([]string{
		"그 조건을 충족한 검사가 무엇을 확인했는지와 어느 commit에서 통과했는지",
		"seq만으로 가리키지 않는다",
		"`라운드 | 지적 | 끝`",
		"라운드의 지적은 하나도 빠짐없이 끝 하나로 분류된다",
	}, rsmEndings...)
	rsmNeedAll(t, v, en, ko)
	for _, d := range rsmDocs(t, v) {
		if !strings.HasPrefix(d.where, rsmTemplate) {
			rsmNeed(t, v, d.where, d.sec, "`라운드 | 지적 | 끝`")
		}
	}
}

// AC-6: what to inspect by hand, highest risk first, holds every deferred
// and undisposed finding whatever ha report shows.
func TestReviewSummaryAC6_ToInspect(t *testing.T) {
	const v = "inspect-list-missing"
	if summary, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		rsmSub(t, v, summary, rsmHeadings[4])
	}
	rsmNeedAll(t, v, []string{
		"Highest risk first, one or two lines each",
		"whatever the `직접 볼 것` line of `ha report` shows",
		"every deferred finding with its issue",
		"every finding left with no disposition",
		"executor assumptions that read a SPEC sentence",
		"conditions met by checks that observe fixed text only",
		"`manual` conditions",
		"the changes under `### 범위 밖이나 기존 것을 바꾼 곳`",
	}, []string{
		"위험이 큰 순서",
		"`ha report`의 `직접 볼 것` 줄이 무엇을 보이든",
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
	if summary, ok := rsmNeedSection(t, v, rsmTemplate, rsmSummary); ok {
		rsmSub(t, v, summary, rsmHeadings[3])
		rsmSub(t, v, summary, rsmHeadings[5])
	}
	rsmNeedAll(t, v, []string{
		"decided by the user, with where their words are",
		"proposed by the agent and accepted by the user",
		"executor assumptions nobody was asked about",
		"Mark an assumption that reads a SPEC sentence",
		"Unobserved scope and substitutes",
		"assurance `local`",
		"the executor wrote the checks",
		"not evidence of deployment, installation or a real model call",
	}, []string{
		"사용자가 정한 것",
		"에이전트가 제안하고 사용자가 받아들인 것",
		"묻지 않고 정한 실행자 가정",
		"해석한 가정에는 그렇다는 표시를 붙인다",
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
}

// AC-10: the reading rules keep the summary short, and the skill lists a
// finding by its ID, one line and its ending.
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
	if skill, ok := rsmRead(t, v, rsmSkill); ok {
		rsmNeed(t, v, rsmSkill, skill, "`## 검수 요약` lists each finding by its ID, one line and its ending")
	}
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
		rsmNeed(t, v, rsmSkill, skill, "For a goal started with Seal 0.1.24 or later")
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
		"바뀐 동작과 파일, 범위 밖이나 기존 것을 바꾼 곳, 근거, 누가 정했나, 직접 볼 것, 보지 않은 것",
		"항목마다 짧게 쓰고 seq는 보조로만 붙입니다",
		"Seal의 판단은 이 절에만 있고, `## 완료 보고`에는 `ha report` 출력과 이 절을 가리키는 한 줄이 있습니다",
		"완료한 목표는 `## 검수 요약`부터 읽습니다",
	}
	en := []string{
		"goals started with 0.1.24 or later",
		"after completion, `## 검수 요약` right after `## 현재`",
		"changed behavior and files, scope changes, evidence, who decided, what to inspect, and what was not observed",
		"each item is short, with seq numbers only as an aid",
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
}

// AC-16: the Seal version goes up to 0.1.24 in every manifest, the skill and
// the marketplace, and the version list names this change.
func TestReviewSummaryAC16_Version(t *testing.T) {
	const v = "seal-version-not-raised"
	const want = "0.1.24"
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
		rsmNeed(t, v, rsmSkill, skill, "version: \"0.1.24\"")
	}
	if ha, ok := rsmRead(t, v, rsmHa); ok {
		rsmNeed(t, v, rsmHa, ha, "Seal 0.1.24 adds the review summary (`## 검수 요약`) in `PROGRESS.md` without a Core change.")
	}
}
