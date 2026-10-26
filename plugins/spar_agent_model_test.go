package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A check of the goal spar-agent-model-choice that reads fixed phrases in
// the Spar configuration, round and input contracts, the Spar guides and
// the Spar skill. The opening line of an assertion's failure message names
// the property it observes (jaekit:violation=<id>); a failure that starts
// with "setup:" is a broken setup, not an observation.

func amdRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// amdSection is the text from the heading line that starts with start up
// to the next heading of the same level, or "" when there is no such
// heading.
func amdSection(text, start string) string {
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

// amdLine is the first line of text that starts with prefix, or "".
func amdLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func amdExpect(t *testing.T, violation, where, text string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		if !strings.Contains(text, p) {
			t.Errorf("jaekit:violation=%s %s does not say %q", violation, where, p)
		}
	}
}

// AC-11: the contracts, both Spar guides and the Spar skill state who
// decides a reviewer's model and effort, the agent's run arguments, the
// allowed lists, the fallback of a chosen model, what is refused and where
// the record shows the source.
func TestSparAgentModelAC11_ContractsGuidesSkill(t *testing.T) {
	const v = "agent-model-docs-missing"
	config := amdRead(t, "../contracts/spar-config.md")
	amdExpect(t, v, "contracts/spar-config.md", config,
		"`--model 이름=모델`", "`--effort 이름=강도`", "`allowed_models`", "`allowed_efforts`",
		"`model_source`", "`effort_source`", "`default_model`", "`choices`", "사용자가 적은 `fallback_model`")
	round := amdRead(t, "../contracts/review-round.md")
	amdExpect(t, v, "the --model row of contracts/review-round.md", amdLine(round, "| `--model` |"), "`--model`")
	amdExpect(t, v, "the --effort row of contracts/review-round.md", amdLine(round, "| `--effort` |"), "`--effort`")
	amdExpect(t, v, "the reviewers field line of contracts/review-round.md", amdLine(round, "`reviewers.<이름>`:"),
		"`model_source`", "`effort_source`")
	amdExpect(t, v, "the fallback paragraph of contracts/review-round.md", amdLine(round, "대체 모델:"),
		"사용자가 적은 `fallback_model`뿐")
	input := amdRead(t, "../contracts/spar-input.md")
	amdExpect(t, v, "the G25 row of contracts/spar-input.md", amdLine(input, "| G25 |"),
		"`--model`", "`--effort`", "`allowed_models`", "`-`로 시작")
	amdExpect(t, v, "guides/SPAR.md 리뷰어 설정", amdSection(amdRead(t, "../guides/SPAR.md"), "## 리뷰어 설정"),
		"`--model 이름=모델`", "`--effort 이름=강도`", "`allowed_models`", "에이전트가 고릅니다",
		"사용자가 적은 `fallback_model`만", "거부합니다", "(`user`, `agent`, `preset`)")
	amdExpect(t, v, "guides/SPAR.en.md Configure reviewers", amdSection(amdRead(t, "../guides/SPAR.en.md"), "## Configure reviewers"),
		"`--model NAME=MODEL`", "`--effort NAME=EFFORT`", "`allowed_models`", "the agent chooses",
		"falls back only to the `fallback_model` you wrote", "spar refuses", "(`user`, `agent`, `preset`)")
	amdExpect(t, v, "the Spar skill", amdRead(t, "spar/skills/spar/SKILL.md"),
		"`--model NAME=MODEL`", "`--effort NAME=EFFORT`", "`allowed_models`", "`model_source`", "Check `spar config` before you choose",
		"falls back only to the `fallback_model` the user wrote", "spar refuses",
		"say which model and effort you chose for each reviewer")
}
