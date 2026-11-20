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
	stale := []string{"time spent waiting does not use the budget", "time spent waiting is not included", "the waited time is not included"}
	for _, sec := range []struct {
		path, heading string
		wants         []string
	}{
		{rr2RunRecord, "### 5.2 Budget", []string{"the time spent waiting is included too", "querying"}},
		{rr2Bundle, "### 2.4 Budget (`## 예산`)", []string{"the waited time counts too", "query"}},
	} {
		text := mdSection(t, sec.path, sec.heading)
		for _, s := range stale {
			if strings.Contains(text, s) {
				t.Errorf("%s %s still says %q", sec.path, sec.heading, s)
			}
		}
		requireIn(t, sec.path+" "+sec.heading, text, sec.wants...)
	}
}

// the contracts state what differs between the rule
// versions, which files of other goals leave the code state and in which
// judgments, and the PLAN declaration with its form, effect, and timing.
func TestRunRules2Contracts(t *testing.T) {
	rules := mdSection(t, rr2RunRecord, "### 4.6 Rules versions")
	requireIn(t, "run-record.md §4.6", rules,
		"`run-rules/1`", "`run-rules/2`", "`budget_exceeded`", "`complete`",
		"the same parent", "`SPEC.md`", "`PLAN.md`", "`REVIEW.md`", "`PROGRESS.md`", "`runs.jsonl`", "`tasks/`",
		"`다른 목표 실행 묶음`", "`검사 입력`",
		"Freshness", "completion record", "`out_of_scope`", "`test_definition_changed`", "`tree_clean`", "66")
	requireIn(t, "run-record.md §3", mdSection(t, rr2RunRecord, "## 3. Freshness"), "§4.6")
	requireIn(t, "run-record.md §4.3", mdSection(t, rr2RunRecord, "### 4.3 Status"), "`run-rules/2`", "§4.6")
	requireIn(t, "run-record.md §5.1", mdSection(t, rr2RunRecord, "### 5.1 Completion record"), "§4.6")

	scope := mdSection(t, rr2Bundle, "### 2.2 Scope (`## 범위`)")
	requireIn(t, "bundle.md §2.2", scope,
		"`다른 목표 실행 묶음`", "`검사 입력`", "`other_bundles_invalid`", "when computing the status and when recording", "`run-rules/1`")
	requireIn(t, "bundle.md §8", mdSection(t, rr2Bundle, "## 8. `ha lint` error codes"), "`other_bundles_invalid`")
}

// the Seal guidance keeps the tokens of what run-rules/2 changes, of a goal
// complete over the budget and its reopen, and of the plan declaration for
// checks that read other goals' bundles; the PLAN template carries that
// declaration's field.
func TestRunRules2SealGuide(t *testing.T) {
	requireSkillTokens(t, sealSkillDir,
		"run-rules/1", "run-rules/2", "budget_exceeded", "reopen", "다른 목표 실행 묶음: `검사 입력`")
	requireIn(t, "templates/PLAN.md 범위", mdSection(t, "seal/skills/seal/assets/templates/PLAN.md", "## 범위"),
		"다른 목표 실행 묶음")
}
