package plugins

import "testing"

// version to ha. These checks hold the documents that describe it.

const (
	rr2RunRecord = "../contracts/run-record.md"
	rr2Bundle    = "../contracts/bundle.md"
)

// the contracts state what differs between the rule
// versions, which files of other goals leave the code state and in which
// judgments, and the PLAN declaration with its form, effect, and timing.
func TestRunRules2Contracts(t *testing.T) {
	rules := mdSection(t, rr2RunRecord, "### 4.6 Rules versions")
	requireIn(t, "run-record.md §4.6", rules,
		"`run-rules/1`", "`run-rules/2`", "`budget_exceeded`", "`complete`",
		"`SPEC.md`", "`PLAN.md`", "`REVIEW.md`", "`PROGRESS.md`", "`runs.jsonl`", "`tasks/`",
		"`다른 목표 실행 묶음`", "`검사 입력`",
		"`out_of_scope`", "`test_definition_changed`", "`tree_clean`", "66")
	requireIn(t, "run-record.md §4.3", mdSection(t, rr2RunRecord, "### 4.3 Status"), "`run-rules/2`")

	scope := mdSection(t, rr2Bundle, "### 2.2 Scope (`## 범위`)")
	requireIn(t, "bundle.md §2.2", scope,
		"`다른 목표 실행 묶음`", "`검사 입력`", "`other_bundles_invalid`", "`run-rules/1`")
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
