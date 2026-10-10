package plugins

import (
	"strings"
	"testing"
)

// the README-linked usage guide, operations guide, and Codex plugin
// description name the explicit skill call on each host and distinguish it
// from a Codex @Spec plugin mention.
func TestSpecInvocationGuide(t *testing.T) {
	requireAll(t, "../README.md", "(guides/USAGE.md)")
	for _, c := range []struct {
		path  string
		musts []string
	}{
		{"../guides/USAGE.md", []string{"`/spec:spec <요청>`", "`$spec:spec <요청>`", "`@Spec`"}},
		{"spec/.codex-plugin/plugin.json", []string{"spec:spec", "$ menu", "@Spec"}},
	} {
		text := read(t, c.path)
		for _, s := range c.musts {
			if !strings.Contains(text, s) {
				t.Errorf("%s lacks %q", c.path, s)
			}
		}
	}
}

// The linked usage guide gives the namespaced Seal skill invocation on
// each host as the step after Spec.
func TestSealStartCommandGuide(t *testing.T) {
	requireAll(t, "../README.md", "(guides/USAGE.md)")
	for _, path := range []string{"../guides/USAGE.md"} {
		text := read(t, path)
		for _, s := range []string{"`/seal:seal docs/specs/<goal>`", "`$seal:seal docs/specs/<goal>`"} {
			if !strings.Contains(text, s) {
				t.Errorf("%s lacks %q", path, s)
			}
		}
	}
}

// every document that describes J2 tells the same two steps:
// save the goal documents, then send the start command.
func TestPublicWorkflowUsesSeparateStart(t *testing.T) {
	TestReadmeVisualGuideAC3(t)
	requireAll(t, "../contracts/goal-docs.md", "`Status: Ready`는 실행 권한이 아니다", "목표 문서가 저장된 뒤 사용자가 따로 보내는 구현 요청")
}
