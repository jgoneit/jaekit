package plugins

import (
	"strings"
	"testing"
)

// version to ha. These checks hold the documents that describe it.

const (
	rr2RunRecord = "../contracts/run-record.md"
	rr2Bundle    = "../contracts/bundle.md"
)

// rr2Row returns the first line of path that starts with prefix.
func rr2Row(t *testing.T, path, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(read(t, path), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("%s has no line starting with %q", path, prefix)
	return ""
}

// elapsed time is described the way ha measures it:
// between record timestamps, so a wait counts once a record follows it, and
// looking at the status while waiting adds nothing.
func TestRunRules2WaitTime(t *testing.T) {
	stale := []string{"기다리는 시간은 예산을 쓰지 않는다", "기다리는 시간은 들어가지 않고", "기다린 시간은 들어가지 않는다"}
	for _, sec := range []struct{ path, heading string }{
		{rr2RunRecord, "### 5.2 예산"},
		{rr2Bundle, "### 2.4 예산 (`## 예산`)"},
	} {
		text := mdSection(t, sec.path, sec.heading)
		for _, s := range stale {
			if strings.Contains(text, s) {
				t.Errorf("%s %s still says %q", sec.path, sec.heading, s)
			}
		}
		requireIn(t, sec.path+" "+sec.heading, text, "기다린 시간도", "조회")
	}
}

// the contracts state what differs between the rule
// versions, which files of other goals leave the code state and in which
// judgments, and the PLAN declaration with its form, effect, and timing.
func TestRunRules2Contracts(t *testing.T) {
	rules := mdSection(t, rr2RunRecord, "### 4.6 규칙 버전")
	requireIn(t, "run-record.md §4.6", rules,
		"`run-rules/1`", "`run-rules/2`", "`budget_exceeded`", "`complete`",
		"같은 부모", "`SPEC.md`", "`PLAN.md`", "`REVIEW.md`", "`PROGRESS.md`", "`runs.jsonl`", "`tasks/`",
		"`다른 목표 실행 묶음`", "`검사 입력`",
		"신선도", "완료 기록", "`out_of_scope`", "`test_definition_changed`", "`tree_clean`", "66")
	requireIn(t, "run-record.md §3", mdSection(t, rr2RunRecord, "## 3. 신선도"), "§4.6")
	requireIn(t, "run-record.md §4.3", mdSection(t, rr2RunRecord, "### 4.3 상태"), "`run-rules/2`", "§4.6")
	requireIn(t, "run-record.md §5.1", mdSection(t, rr2RunRecord, "### 5.1 완료 기록"), "§4.6")

	scope := mdSection(t, rr2Bundle, "### 2.2 범위 (`## 범위`)")
	requireIn(t, "bundle.md §2.2", scope,
		"`다른 목표 실행 묶음`", "`검사 입력`", "`other_bundles_invalid`", "계산하고 기록할 때", "`run-rules/1`")
	requireIn(t, "bundle.md §8", mdSection(t, rr2Bundle, "## 8. `ha lint` 오류 코드"), "`other_bundles_invalid`")
}

// the Seal guidance tells the agent what run-rules/2
// changes, that completing over the budget is not leave to keep working, how
// to declare checks that read other goals' bundles, and that run-rules/1
// goals keep the earlier rules.
func TestRunRules2SealGuide(t *testing.T) {
	const ha = "seal/skills/seal/references/ha.md"
	requireIn(t, "ha.md States", mdSection(t, ha, "## States"),
		"`run-rules/2`", "`run-rules/1`", "`budget_exceeded`", "evidence already", "`reopen`")
	requireIn(t, "ha.md Freshness", mdSection(t, ha, "## Freshness"),
		"other goals", "`run-rules/2`", "다른 목표 실행 묶음: `검사 입력`")
	requireIn(t, "references/bundle.md 범위", mdSection(t, "seal/skills/seal/references/bundle.md", "### `## 범위`"),
		"`다른 목표 실행 묶음:`", "`검사 입력`", "`run-rules/2`")
	requireIn(t, "templates/PLAN.md 범위", mdSection(t, "seal/skills/seal/assets/templates/PLAN.md", "## 범위"),
		"다른 목표 실행 묶음")
}
