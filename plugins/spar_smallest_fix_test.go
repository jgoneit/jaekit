package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Checks of the goal spar-smallest-fix that read fixed phrases in the Spar
// skill, the round and input contracts and the Spar guides. The opening line
// of an assertion's failure message names the property it observes
// (jaekit:violation=<id>); a failure that starts with "setup:" is a broken
// setup, not an observation.

func sfpRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// sfpSection is the text from the heading line that starts with start up to
// the next heading of the same level, or "" when there is no such heading.
func sfpSection(text, start string) string {
	level := start[:strings.Index(start, " ")+1]
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, start) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], level) {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

// sfpLine is the first line of text that starts with prefix, or "".
func sfpLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func sfpExpect(t *testing.T, violation, where, text string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		if !strings.Contains(text, p) {
			t.Errorf("jaekit:violation=%s %s does not say %q", violation, where, p)
		}
	}
}

// AC-5: the Spar skill's fix disposition says to start a fix within the
// finding's direction and to record why with --note when it goes further.
func TestSparSmallestFixAC5_TriageGuidance(t *testing.T) {
	const v = "smallest-fix-guidance-missing"
	skill := sfpRead(t, "spar/skills/spar/SKILL.md")
	sfpExpect(t, v, "the Spar skill's fix line", sfpLine(skill, "- `fix`:"),
		"Start the fix within the finding's `smallest_fix` direction",
		"record why with `--note`")
}

// AC-8: the round contract (sections 3 and 5), the input contract's G5 row
// and both Spar guides say what smallest_fix means, that it may be empty and
// that no judgment uses it.
func TestSparSmallestFixAC8_ContractsAndGuides(t *testing.T) {
	const v = "smallest-fix-docs-missing"
	round := sfpRead(t, "../contracts/review-round.md")
	sfpExpect(t, v, "contracts/review-round.md section 3", sfpSection(round, "## 3. "),
		"`smallest_fix`", "가장 작은 변경", "방향 없음", "분류·처리·gate에 쓰지 않는다")
	five := sfpSection(round, "## 5. ")
	sfpExpect(t, v, "the findings.json row of contracts/review-round.md section 5", sfpLine(five, "| `findings.json` |"), "`smallest_fix`")
	sfpExpect(t, v, "the summary.md row of contracts/review-round.md section 5", sfpLine(five, "| `summary.md` |"), "`smallest_fix`")
	input := sfpRead(t, "../contracts/spar-input.md")
	sfpExpect(t, v, "the G5 row of contracts/spar-input.md", sfpLine(input, "| G5 |"),
		"선택 키 `smallest_fix`", "키가 없어도 받고", "문자열 아님(`smallest_fix` 포함)")
	sfpExpect(t, v, "guides/SPAR.md 지적과 gate", sfpSection(sfpRead(t, "../guides/SPAR.md"), "## 지적과 gate"),
		"`smallest_fix`", "가장 작은 변경", "비워 둘 수 있습니다", "분류·처리·gate에 쓰지 않습니다")
	sfpExpect(t, v, "guides/SPAR.en.md Findings and the gate", sfpSection(sfpRead(t, "../guides/SPAR.en.md"), "## Findings and the gate"),
		"`smallest_fix`", "smallest change", "may leave it empty", "is not used for classes, dispositions or the gate")
}
