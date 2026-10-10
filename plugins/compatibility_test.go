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
	if marketplace.Metadata.Version != "0.1.2" {
		t.Errorf("[requirement] current released marketplace identity changed: %s", marketplace.Metadata.Version)
	}
	for name, want := range map[string]string{"spec": "0.1.11", "seal": "0.1.9"} {
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
	compatibilityContains(t, "Core development version", read(t, "../cmd/ha/main.go"), `var version = "0.1.3-dev"`)
	// Codex's marketplace format has no component version field.
	var codex map[string]any
	readJSON(t, "../.agents/plugins/marketplace.json", &codex)
	if _, added := codex["version"]; added {
		t.Error("[requirement] added an unnecessary Codex marketplace version")
	}
}

func TestCompatibilityAC11_ReleaseAndDevelopmentGuidance(t *testing.T) {
	for _, path := range []string{"../README.md", "../README.en.md", "../guides/INSTALL.md", "../guides/INSTALL.en.md", "../guides/USAGE.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.md"} {
		t.Run(path, func(t *testing.T) {
			content := read(t, path)
			compatibilityContains(t, path, content, "v0.1.2", "0.1.10", "0.1.8", "0.1.3-dev", "0.1.11", "0.1.9")
			if strings.HasSuffix(path, ".en.md") {
				compatibilityContains(t, path, strings.ToLower(content), "unreleased")
			} else if !strings.Contains(content, "미배포") && !strings.Contains(content, "아직 배포하지 않았") {
				t.Errorf("[requirement] %s does not distinguish the unreleased combination", path)
			}
			for _, futureInstall := range []string{"jaekit@v0.1.3", "jaekit#v0.1.3", "/releases/download/v0.1.3", "/releases/tag/v0.1.3"} {
				if strings.Contains(content, futureInstall) {
					t.Errorf("[requirement] %s directs installation of an unreleased artifact: %s", path, futureInstall)
				}
			}
		})
	}
	for _, path := range []string{"../guides/INSTALL.md", "../guides/INSTALL.en.md"} {
		compatibilityContains(t, path, read(t, path), "jaekit@v0.1.2", "jaekit#v0.1.2", "ha_0.1.2_${os}_${arch}", "contracts/core-capabilities.md")
	}
	compatibilityContains(t, "Korean budget guidance", read(t, "../guides/USAGE.md"), "시간 상한 변경은 지원하지 않습니다", "횟수 총상한", "명시적 재개", "원래 한도")
	compatibilityContains(t, "English budget guidance", read(t, "../guides/USAGE.en.md"), "Changing the elapsed-time limit is unsupported", "total run limit", "explicit reopen", "original limits")
}

func TestCompatibilityAC12_SpecAndVerificationBoundaries(t *testing.T) {
	specPath := "spec/skills/spec/SKILL.md"
	spec := read(t, specPath)
	keys, _ := frontMatter(t, specPath)
	if keys["disable-model-invocation"] != "true" {
		t.Error("[requirement] Spec lost its explicit-invocation boundary")
	}
	compatibilityContains(t, specPath, spec,
		"without installing Core, querying its capabilities, or running verification",
		"The implementing agent chooses the checks, runner and verification method",
		"implementation starts with a new request", "This skill never edits code or runs checks",
		"preserve its format selection or its absence")
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
	sealPath := "seal/skills/seal/SKILL.md"
	compatibilityContains(t, sealPath, read(t, sealPath),
		"Choose the checks, runner and verification method", "does not require an official adapter",
		"Python 3 is not a prerequisite for Seal", "Do not require `/3` declarations for `/1` or `/2`",
		"ordinary maintain, manual and task checks retain their roles")
}
