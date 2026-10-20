package plugins

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// the Spar plugin is listed in both marketplaces, its manifests and skill
// agree on name and version, and its skill body is English.
func TestSparPluginManifestsAgree(t *testing.T) {
	keys, version := frontMatter(t, filepath.Join("spar", "skills", "spar", "SKILL.md"))
	if keys["name"] != "spar" || keys["description"] == "" || version == "" {
		t.Fatalf("spar SKILL.md front matter: %v %q", keys, version)
	}
	var claude, codex manifest
	readJSON(t, filepath.Join("spar", ".claude-plugin", "plugin.json"), &claude)
	readJSON(t, filepath.Join("spar", ".codex-plugin", "plugin.json"), &codex)
	if claude.Name != "spar" || codex.Name != "spar" || claude.Version != version || codex.Version != version || codex.Skills != "./skills/" {
		t.Fatalf("spar manifests disagree: %+v %+v skill %s", claude, codex, version)
	}
	var descriptions struct {
		Description string `json:"description"`
	}
	var codexDescription struct {
		Description string `json:"description"`
	}
	readJSON(t, filepath.Join("spar", ".claude-plugin", "plugin.json"), &descriptions)
	readJSON(t, filepath.Join("spar", ".codex-plugin", "plugin.json"), &codexDescription)
	var market struct {
		Plugins []struct {
			Name, Source, Version, Description string
		} `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	found := false
	for _, p := range market.Plugins {
		if p.Name == "spar" {
			found = p.Source == "./plugins/spar" && p.Version == version && p.Description == descriptions.Description
		}
	}
	if !found || descriptions.Description != codexDescription.Description {
		t.Fatalf("spar marketplace entry or descriptions disagree")
	}
	body := strings.SplitN(read(t, filepath.Join("spar", "skills", "spar", "SKILL.md")), "\n---\n", 2)[1]
	if regexp.MustCompile(`[가-힣]`).MatchString(body) {
		t.Fatalf("spar skill body is not English")
	}
}
