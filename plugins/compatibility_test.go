package plugins

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func compatibilityContains(t *testing.T, path, content string, want ...string) {
	t.Helper()
	for _, text := range want {
		if !strings.Contains(content, text) {
			t.Errorf("[requirement] %s is missing %q", path, text)
		}
	}
}

func TestCompatibilityAC10_SourceVersions(t *testing.T) {
	var marketplace struct {
		Metadata struct{ Version string } `json:"metadata"`
		Plugins  []manifest               `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &marketplace)
	if marketplace.Metadata.Version != "0.1.9" {
		t.Errorf("[requirement] current released marketplace identity changed: %s", marketplace.Metadata.Version)
	}
	// Release v0.1.6 shipped Spec 0.1.12 and Seal 0.1.17. Spec 0.1.13 adds
	// agent invocation, the Assurance line and the implement-now question;
	// Seal 0.1.18 takes the user's yes to that question as a request,
	// Seal 0.1.19 adds runner adapter guidance, and Seal 0.1.20 leaves the
	// capability check, budget, environment and report to ha and adds the
	// review tiers; Seal 0.1.21 starts under run-rules/5 and records the review
	// gate, and Seal 0.1.22 points its install route and release links at
	// v0.1.7. Release v0.1.7 ships Spec 0.1.13 and Seal 0.1.22. Seal 0.1.23
	// asks foreseeable questions before any baseline and points its install
	// route and release links at v0.1.8; release v0.1.8 ships Spec 0.1.13 and
	// Seal 0.1.23. Seal 0.1.24 adds the review summary in PROGRESS.md after
	// that release, and Seal 0.1.25 passes decisions and invariant places to
	// review rounds. Spec 0.1.15 and Seal 0.1.30 move rules used only in rare
	// situations to their reference documents. Seal 0.1.31 writes check
	// boundaries before the first review round and passes them to every round.
	// Spec 0.1.16 marks its user's-words rule with a token of its own.
	// Seal 0.1.32 marks baselines for two kinds only and records them right
	// before completion. Seal 0.1.33 runs review rounds only as plan and
	// candidate: one candidate round from the base commit after review fixes.
	// Seal 0.1.34 drops its guidance for the removed `ha history` command.
	// Seal 0.1.35 reduces the review summary to four headings. Release v0.1.9
	// ships Spec 0.1.16 and Seal 0.1.36. Seal 0.1.37 names unmet required
	// conditions as a reason an earlier completion no longer holds under every
	// structured rule, not only run-rules/3.
	for name, want := range map[string]string{"spec": "0.1.16", "seal": "0.1.37"} {
		t.Run(name, func(t *testing.T) {
			_, version := frontMatter(t, filepath.Join(name, "skills", name, "SKILL.md"))
			if version != want {
				t.Errorf("[requirement] %s source skill is %s, want %s", name, version, want)
			}
			for _, host := range []string{".claude-plugin", ".codex-plugin"} {
				var plugin manifest
				readJSON(t, filepath.Join(name, host, "plugin.json"), &plugin)
				if plugin.Name != name || plugin.Version != want {
					t.Errorf("[requirement] %s/%s disagrees with source identity: %+v", name, host, plugin)
				}
			}
			found := false
			for _, plugin := range marketplace.Plugins {
				if plugin.Name == name {
					found = plugin.Version == want
				}
			}
			if !found {
				t.Errorf("[requirement] marketplace does not identify %s %s", name, want)
			}
		})
	}
	compatibilityContains(t, "Core development version", read(t, "../cmd/ha/main.go"), `var version = "0.1.9-dev"`)
	// These package a checkout build, which prints the development version;
	// install.ps1 refuses a manifest version the executable does not print.
	for _, path := range []string{"../tools/windows/README.md"} {
		content := read(t, path)
		compatibilityContains(t, path, content, "0.1.9-dev")
		if strings.Contains(content, "0.1.8-dev") {
			t.Errorf("[requirement] %s packages a checkout build as 0.1.8-dev", path)
		}
	}
	// Codex's marketplace format has no component version field.
	var codex map[string]any
	readJSON(t, "../.agents/plugins/marketplace.json", &codex)
	if _, added := codex["version"]; added {
		t.Error("[requirement] added an unnecessary Codex marketplace version")
	}
}

func TestCompatibilityAC12_SpecAndVerificationBoundaries(t *testing.T) {
	keys, _ := frontMatter(t, "spec/skills/spec/SKILL.md")
	// Agents may invoke Spec.
	if _, blocked := keys["disable-model-invocation"]; blocked {
		t.Error("[requirement] Spec lost its invocation boundary")
	}
	err := filepath.WalkDir("spec/skills/spec/assets/templates", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		content := read(t, path)
		compatibilityContains(t, path, content, "Criteria-Format: nested/1")
		for _, command := range []string{"ha capabilities", "ha start", "ha check", "ha done", "## 결과 계약"} {
			if strings.Contains(content, command) {
				t.Errorf("[requirement] Spec template %s acquired execution requirement %q", path, command)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
