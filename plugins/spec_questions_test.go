package plugins

import (
	"strconv"
	"strings"
	"testing"
)

// a decision the user leaves open is recorded with Status: Draft, a format
// token of the Spec skill.
func TestSpecKeepsQuestionPrinciples(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Draft")
}

// Every public quality case is listed.
func TestSyntheticSpecCasesHaveContext(t *testing.T) {
	for _, name := range []string{
		"ambiguous-current-behavior", "ambiguous-export", "bug-report-after-implementation",
		"evaluation-items", "existing-discount-expiry", "login-message-scope",
		"long-session-release", "profile-nickname-save", "readme-typo", "same-request-handoff",
		"meeting-room-booking", "admin-order-search", "draft-then-ready", "target-then-draft",
	} {
		t.Run(name, func(t *testing.T) {
			requireAll(t, "../evals/spec-cases/README.md", "["+name+"]("+name+".md)")
		})
	}
}

// the Spec plugin version is above 0.1.7, and the skill,
// both plugin manifests, and the marketplace entry agree on it.
func TestSpecPluginVersionRaised(t *testing.T) {
	_, version := frontMatter(t, "spec/skills/spec/SKILL.md")
	var claude, codex manifest
	readJSON(t, "spec/.claude-plugin/plugin.json", &claude)
	readJSON(t, "spec/.codex-plugin/plugin.json", &codex)
	var market struct {
		Plugins []struct{ Name, Version string } `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	marketVersion := ""
	for _, p := range market.Plugins {
		if p.Name == "spec" {
			marketVersion = p.Version
		}
	}
	if claude.Version != version || codex.Version != version || marketVersion != version {
		t.Errorf("Spec versions disagree: skill %q, claude %q, codex %q, marketplace %q", version, claude.Version, codex.Version, marketVersion)
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		t.Fatalf("Spec version %q is not major.minor.patch", version)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("Spec version %q: %v", version, err)
		}
		n[i] = v
	}
	if n[0] == 0 && (n[1] < 1 || n[1] == 1 && n[2] <= 7) {
		t.Errorf("Spec version %s is not above 0.1.7", version)
	}
}
