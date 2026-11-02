package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Checks of the goal spar-cap-report that read fixed phrases in the Spar
// skill, the round, input and run record contracts in both languages and
// the Spar guides (REVIEW.md "대표 형식"). The opening line of an
// assertion's failure message names the property it observes
// (jaekit:violation=<id>); a failure that starts with "setup:" is a broken
// setup, not an observation.

func crpRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// crpLine is the first line of text that starts with prefix, or "".
func crpLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func crpExpect(t *testing.T, violation, where, text string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		if !strings.Contains(text, p) {
			t.Errorf("jaekit:violation=%s %s does not say %q", violation, where, p)
		}
	}
}

// AC-11: each time the code cap is reached the Spar skill runs the report,
// picks one cause, shows the signals with that cause's recommendation, and
// records a code extension only with the user's words and the cause, by 1
// unless a reason is given.
func TestSparCapReportAC11_SkillCapProcedure(t *testing.T) {
	const v = "skill-cap-procedure-missing"
	const where = "spar/skills/spar/SKILL.md"
	skill := crpRead(t, where)
	crpExpect(t, v, where, skill,
		"Each time the code cap is reached, run `spar cap-report <series>` before you ask.",
		"Pick one cause from its signals and show the user the signals with that cause's recommendation",
		"Only the user can allow more rounds.")
	for _, c := range []struct {
		cause   string
		phrases []string
	}{
		{"churn", []string{"extend by 1", "`smallest_fix`"}},
		{"recurring", []string{"not to extend", "ask which reading"}},
		{"unrelated", []string{"not to extend", "defer them with the user's words", "issues"}},
		{"hardening", []string{"not to extend", "trim the checks"}},
		{"other", []string{"Say why in `--reason`."}},
	} {
		prefix := "  - `" + c.cause + "`:"
		line := crpLine(skill, prefix)
		if line == "" {
			t.Errorf("jaekit:violation=%s %s has no line starting %q", v, where, prefix)
			continue
		}
		crpExpect(t, v, where+" line "+prefix, line, c.phrases...)
	}
	record := crpLine(skill, "- Record a code extension only with the user's words")
	if record == "" {
		t.Fatalf("jaekit:violation=%s %s has no line starting %q", v, where, "- Record a code extension only with the user's words")
	}
	crpExpect(t, v, where+" extension line", record,
		"`spar extend <series> --stage code --by 1 --cause <cause> --user-quote \"<their words>\"`",
		"`--by 2` or more, and `--cause other`, need `--reason`.")
}

// AC-12: the contracts in both languages and both Spar guides describe the
// report, the cause and reason of a code extension and ha cost's cause.
func TestSparCapReportAC12_ContractsAndGuides(t *testing.T) {
	const v = "cap-report-docs-missing"
	tokens := []string{"`spar cap-report`", "`spar-cap-report/v1`", "`--cause`", "`--reason`", "`churn`", "`recurring`",
		"`unrelated`", "`hardening`", "`other`", "`check_growth`"}
	for _, c := range []struct{ path, heading string }{
		{"../contracts/review-round.md", "## 11. 상한 보고"},
		{"../contracts/review-round.en.md", "## 11. Cap report"},
	} {
		text := crpRead(t, c.path)
		crpExpect(t, v, c.path, text, append(tokens, "\n"+c.heading+"\n")...)
	}
	for _, path := range []string{"../contracts/spar-input.md", "../contracts/spar-input.en.md"} {
		text := crpRead(t, path)
		crpExpect(t, v, path+" G31", crpLine(text, "| G31 |"), "`extend --cause`", "`extend --reason`")
		crpExpect(t, v, path, text, "`cap-report --format`")
	}
	for _, c := range []struct{ path, fields, row, cause, null string }{
		{"../contracts/run-record.md", "series의 `extensions.jsonl`(`stage`, `by`, `cause`, `reason`, `user_quote`)", "| 질문 |", "원인과 이유",
			"상한 연장의 `cause`·`reason`은 예외로, 값이 없으면 `null`을 쓰지 않고 키를 빼"},
		{"../contracts/run-record.en.md", "the series' `extensions.jsonl` (`stage`, `by`, `cause`, `reason`, `user_quote`)", "| Questions |", "cause and reason",
			"A cap extension's `cause` and `reason` are the exception: without a value the key is left out instead of `null`"},
	} {
		text := crpRead(t, c.path)
		crpExpect(t, v, c.path, text, c.fields)
		crpExpect(t, v, c.path+" "+c.row, crpLine(text, c.row), c.cause)
		crpExpect(t, v, c.path+" null rule", text, c.null)
	}
	// The guides explain the report and its limits, the causes and when a
	// reason is needed, and where ha cost shows the cause.
	for _, c := range []struct {
		path    string
		phrases []string
	}{
		{"../guides/SPAR.md", []string{"`spar cap-report`", "`--cause`", "`churn`, `recurring`, `unrelated`, `hardening`, `other`",
			"`--by 2` 이상이나 `--cause other`에는 `--reason`", "파일 단위", "연결 단위", "`ha cost`"}},
		{"../guides/SPAR.en.md", []string{"`spar cap-report`", "`--cause`", "`churn`, `recurring`, `unrelated`, `hardening`, `other`",
			"`--by 2` or more, and `--cause other`, need `--reason`", "by file", "by link", "`ha cost`"}},
	} {
		crpExpect(t, v, c.path, crpRead(t, c.path), c.phrases...)
	}
	// The Spar skill explains the report, its signals and limits, and where
	// ha cost shows the cause; the cause and reason rules are AC-11's lines.
	crpExpect(t, v, "the Spar skill", crpRead(t, "spar/skills/spar/SKILL.md"), "`spar cap-report` shows", "churn", "recurring links",
		"not introduced by the change", "check growth", "Churn is counted by file", "Recurring links are counted by link",
		"`introduced_by_change` as reported", "`ha cost` shows each extension's cause and reason", "no cause was recorded",
		"`--by 2` or more, and `--cause other`, need `--reason`.")
	// The round contracts state the signal limits as the report gives them.
	for _, path := range []string{"../contracts/review-round.md", "../contracts/review-round.en.md"} {
		crpExpect(t, v, path, crpRead(t, path),
			"Counted by file: a finding in a file the fixes changed counts even when it is about another line.",
			"Counted by link: the same problem reported under different links is not grouped.",
			"Uses each reviewer's introduced_by_change as reported.")
	}
}
