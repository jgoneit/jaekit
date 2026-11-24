package plugins

import (
	"strings"
	"testing"
)

// the README-linked usage guide, operations guide, and Codex plugin
// description name the explicit skill call on each host and distinguish it
// from a Codex @Spec plugin mention.
func TestSpecInvocationGuide(t *testing.T) {
	requireAll(t, "../README.ko.md", "(guides/USAGE.md)")
	for _, c := range []struct {
		path  string
		musts []string
	}{
		{"../guides/USAGE.md", []string{"`/spec:spec <요청>`", "`$spec:spec <요청>`", "`@Spec`"}},
		{"spec/.codex-plugin/plugin.json", []string{"spec:spec", "@Spec"}},
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
	requireAll(t, "../README.ko.md", "(guides/USAGE.md)")
	for _, path := range []string{"../guides/USAGE.md"} {
		text := read(t, path)
		for _, s := range []string{"`/seal:seal docs/specs/<goal>`", "`$seal:seal docs/specs/<goal>`"} {
			if !strings.Contains(text, s) {
				t.Errorf("%s lacks %q", path, s)
			}
		}
	}
}
