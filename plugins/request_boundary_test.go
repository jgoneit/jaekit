package plugins

import (
	"strings"
	"testing"
)

// Spec stops after writing the goal documents even when the
// request that invoked it, or a reply to its report, asks for implementation,
// and nothing outside the goal documents changes, remote state and subagent
// work included.
func TestSpecStopsAfterSaving(t *testing.T) {
	for _, c := range []struct {
		path        string
		musts, nots []string
	}{
		{"spec/skills/spec/SKILL.md", []string{
			"An implementation request made earlier in the conversation does not extend to it.",
			"that implementation starts with a new request, then stop",
			"or a reply that answers your questions or asks for changes, also asks for implementation",
			"pull requests, review replies or resolutions, releases, or deployments",
			"Subagents you start follow these rules too",
		}, []string{
			"implementation continues after saving",
			"or the reply that agrees to save",
			"If this same request does not also explicitly ask",
		}},
		{"spec/.codex-plugin/plugin.json",
			[]string{"implementation starts with a new request"},
			[]string{"unless the same request also asked for implementation"}},
		{"../contracts/goal-docs.md",
			[]string{"질문에 답하거나 수정을 요청하는 답이 구현까지 요청해도 Spec은 목표 문서를 쓴 뒤 멈춘다"},
			[]string{"같은 요청이 구현까지 명시했거나", "저장 동의 답이 구현까지 요청해도"}},
		{"../evals/spec-cases/same-request-handoff.md",
			[]string{"구현은 새 요청으로 시작한다고 알린 뒤 멈춘다"},
			[]string{"같은 요청으로 구현이 이어진다"}},
	} {
		text := read(t, c.path)
		for _, s := range c.musts {
			if !strings.Contains(text, s) {
				t.Errorf("%s lacks %q", c.path, s)
			}
		}
		for _, s := range c.nots {
			if strings.Contains(text, s) {
				t.Errorf("%s still says %q", c.path, s)
			}
		}
	}
}

// a message that asks for goal documents to be written or changed
// never starts the work, even when it also asks for implementation.
func TestSealStartsOnlyAfterSaving(t *testing.T) {
	text := read(t, "seal/skills/seal/SKILL.md")
	for _, s := range []string{
		"A message that asks for goal documents to be written or changed",
		"an answer to its questions",
		"even when it also asks for implementation",
		"Work starts only from a later message, sent once the goal documents are written.",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("Seal SKILL.md lacks %q", s)
		}
	}
	for _, s := range []string{
		"It is one only when that same message also asks for implementation.",
		"명세부터 구현까지 해줘",
		"a reply that agrees to save its draft",
	} {
		if strings.Contains(text, s) {
			t.Errorf("Seal SKILL.md still says %q", s)
		}
	}
}
