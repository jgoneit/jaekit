package plugins

// Property: Spar skill rules
// Checks the format tokens of the Spar skill's documents: the round example,
// project setup, standpoints, reviewer model choice, decisions, the fix
// disposition and the code cap.

import "testing"

// The round example gives SPEC.md and a linked goal document as --criteria.
func TestSparSkillExampleGivesEveryGoalDocument(t *testing.T) {
	requireSkillTokens(t, sparSkillDir,
		"--criteria docs/specs/<goal>/SPEC.md",
		"--criteria docs/specs/<goal>/<linked goal document>.md")
}

// The round example shows --check-path with a check file.
func TestSparSkillExampleShowsCheckPath(t *testing.T) {
	requireSkillTokens(t, sparSkillDir, "--check-path <check file>")
}

// The skill names the repository configuration file and the setup step's
// ignore files.
func TestSparSkillBeforeFirstRound(t *testing.T) {
	requireSkillTokens(t, sparSkillDir, ".jaekit/spar.toml", "docs/reviews/", ".gitignore", ".git/info/exclude")
}

// The skill states the --standpoint argument, its delivery to each reviewer,
// the standpoint_report form, the record fields and the input identity.
func TestSparSkillStandpointFormats(t *testing.T) {
	requireSkillTokens(t, sparSkillDir,
		// The argument and its refusals.
		"--standpoint NAME=TEXT", "--reviewers",
		// Delivery.
		"## Standpoints for this round", "claude", "codex", "agy", "-p", "PACKET.md",
		"command", "{prompt}", "native", "prompt.md", "--dry-run", "<round>/<reviewer>/input.md",
		// The report.
		"standpoint_report", "checked", "detail", "invalid_output", "spar run", "spar record --result", "--transcript",
		// The record.
		"round.json", "standpoints_uncovered", "summary.md", "review-round/v1",
		// The input identity.
		"packet_sha256")
}

// The skill states the run arguments for a reviewer's model and effort, the
// allowed lists, the fallback and where the record shows the source.
func TestSparSkillReviewerModelChoice(t *testing.T) {
	requireSkillTokens(t, sparSkillDir,
		"--model NAME=MODEL", "--effort NAME=EFFORT", "spar config", "allowed_models", "fallback_model", "agy",
		"reviewers.<name>.model_source", "effort_source", "model <model> [<source>]", "effort <effort> [<source>]")
}

// The skill names the operators agy's shell may join read-only programs with.
func TestSparSkillAgyCommandNote(t *testing.T) {
	requireSkillTokens(t, sparSkillDir, "&&", "||")
}

// The skill states how decisions are passed, the packet section that tells
// reviewers about them and where the round record keeps them.
func TestSparSkillDecisionBullet(t *testing.T) {
	requireSkillTokens(t, sparSkillDir,
		"--decision", "--decision-quote", "## Decisions the user made", "decisions", "round.json", "requirement-question")
}

// The fix disposition names the finding's smallest_fix direction and --note.
func TestSparSkillFixWithinSmallestFix(t *testing.T) {
	requireSkillTokens(t, sparSkillDir, "smallest_fix", "--note")
}

// At the code cap the skill runs the cap report, names its causes, and
// records a code extension with the cause, by 1 unless a reason is given.
func TestSparSkillCapProcedure(t *testing.T) {
	requireSkillTokens(t, sparSkillDir,
		"spar cap-report <series>", "churn", "smallest_fix", "recurring", "unrelated", "hardening", "--cause other", "--reason",
		`spar extend <series> --stage code --by 1 --cause <cause> --user-quote "<their words>"`, "--by 2")
}

// The skill explains the cap report, its signals, and where ha cost shows an
// extension's cause.
func TestSparSkillCapReport(t *testing.T) {
	requireSkillTokens(t, sparSkillDir,
		"spar cap-report", "churn", "introduced_by_change", "ha cost", "--by 2", "--cause other", "--reason")
}
