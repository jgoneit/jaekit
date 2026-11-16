package plugins

import (
	"strings"
	"testing"
)

// Spec stops after writing the goal documents even when the request that
// invoked it, or a reply to its report, asks for implementation. The Spec
// skill holds none of the old wording that let implementation continue after
// saving, and its Codex manifest, the goal document contract and a quality
// case say that implementation starts with a new request.
func TestSpecStopsAfterSaving(t *testing.T) {
	forbidSkillText(t, specSkillDir,
		"implementation continues after saving",
		"or the reply that agrees to save",
		"If this same request does not also explicitly ask",
	)
	for _, c := range []struct {
		path        string
		musts, nots []string
	}{
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

// the Seal skill holds none of the old wording that took a message asking
// for goal documents, or a reply agreeing to save them, as an implementation
// request.
func TestSealStartsOnlyAfterSaving(t *testing.T) {
	forbidSkillText(t, sealSkillDir,
		"It is one only when that same message also asks for implementation.",
		"명세부터 구현까지 해줘",
		"a reply that agrees to save its draft",
	)
}
