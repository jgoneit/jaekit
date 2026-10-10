// Package plugins holds static checks for the Spec and Seal plugin packages.
// The checks cover the build specs' acceptance criteria that can be decided
// from the files alone; loading in a host and real runs are verified
// separately.
package plugins

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/goaldocs"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// frontMatter returns the flat keys and the metadata.version of a SKILL.md.
func frontMatter(t *testing.T, path string) (map[string]string, string) {
	t.Helper()
	lines := strings.Split(read(t, path), "\n")
	if lines[0] != "---" {
		t.Fatalf("%s has no front matter", path)
	}
	keys := map[string]string{}
	version, inMeta := "", false
	for _, l := range lines[1:] {
		if l == "---" {
			return keys, version
		}
		k, v, _ := strings.Cut(strings.TrimSpace(l), ":")
		v = strings.Trim(strings.TrimSpace(v), `"`)
		if strings.HasPrefix(l, " ") {
			if inMeta && k == "version" {
				version = v
			}
			continue
		}
		keys[k] = v
		inMeta = k == "metadata"
	}
	t.Fatalf("%s front matter is not closed", path)
	return nil, ""
}

type manifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Skills  string `json:"skills"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(read(t, path)), v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// each skill names itself and a version, and every
// manifest and marketplace entry agrees on it.
func TestManifestsAndVersionsAgree(t *testing.T) {
	var claudeMarket struct {
		Plugins []struct {
			Name, Source, Version string
		} `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &claudeMarket)
	var codexMarket struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../.agents/plugins/marketplace.json", &codexMarket)
	for _, name := range []string{"spec", "seal"} {
		keys, version := frontMatter(t, filepath.Join(name, "skills", name, "SKILL.md"))
		if keys["name"] != name || keys["description"] == "" || version == "" {
			t.Errorf("%s SKILL.md front matter: name=%q description set=%v version=%q", name, keys["name"], keys["description"] != "", version)
		}
		var claude, codex manifest
		readJSON(t, filepath.Join(name, ".claude-plugin", "plugin.json"), &claude)
		readJSON(t, filepath.Join(name, ".codex-plugin", "plugin.json"), &codex)
		if claude.Name != name || codex.Name != name || claude.Version != version || codex.Version != version {
			t.Errorf("%s manifests disagree: claude %s %s, codex %s %s, skill %s", name, claude.Name, claude.Version, codex.Name, codex.Version, version)
		}
		if codex.Skills != "./skills/" {
			t.Errorf("%s codex manifest skills = %q", name, codex.Skills)
		}
		found := false
		for _, p := range claudeMarket.Plugins {
			if p.Name == name {
				found = p.Source == "./plugins/"+name && p.Version == version
			}
		}
		if !found {
			t.Errorf("%s is missing or wrong in .claude-plugin/marketplace.json", name)
		}
		found = false
		for _, p := range codexMarket.Plugins {
			if p.Name == name {
				found = p.Source.Path == "./plugins/"+name
			}
		}
		if !found {
			t.Errorf("%s is missing or wrong in .agents/plugins/marketplace.json", name)
		}
	}
}

// nothing in the Spec plugin refers to Seal, its
// commands, bundle files, or run states.
func TestSpecPluginKnowsNothingOfSeal(t *testing.T) {
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bseal\b`),
		regexp.MustCompile(`\bha (lint|start|check|note|status|done|log)\b`),
		regexp.MustCompile(`PLAN\.md|REVIEW\.md|PROGRESS\.md|runs\.jsonl`),
		regexp.MustCompile(`\b(needs_user|budget_exhausted|incomplete)\b`),
	}
	filepath.WalkDir("spec", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		text := read(t, p)
		for _, re := range forbidden {
			if m := re.FindString(text); m != "" {
				t.Errorf("%s mentions %q", p, m)
			}
		}
		return nil
	})
}

// Spec: explicit invocation only, on both hosts.
func TestSpecIsExplicitOnly(t *testing.T) {
	keys, _ := frontMatter(t, "spec/skills/spec/SKILL.md")
	if keys["disable-model-invocation"] != "true" {
		t.Error("Spec SKILL.md must set disable-model-invocation: true")
	}
	if !strings.Contains(read(t, "spec/skills/spec/agents/openai.yaml"), "allow_implicit_invocation: false") {
		t.Error("Spec openai.yaml must disallow implicit invocation")
	}
}

// the status follows the open decisions, so a template's Draft does
// not survive into documents with none open.
func TestSpecStatusFollowsOpenDecisions(t *testing.T) {
	if !strings.Contains(read(t, "spec/skills/spec/SKILL.md"), "`Ready` when none is") {
		t.Error("Spec SKILL.md must say the status is Ready when no outcome-changing decision is open")
	}
}

// templates for behavior changes carry user scenarios, and
// the cross-module template asks for a criterion across the boundary.
func TestSpecTemplates(t *testing.T) {
	dir := "spec/skills/spec/assets/templates"
	for _, name := range []string{"small-change.md", "backend-feature.md", "cross-module-feature.md", "product.md"} {
		text := read(t, filepath.Join(dir, name))
		for _, token := range []string{"Status: Draft", "## Acceptance Criteria", "## Open Decisions", "## 사용 시나리오", "**AC-1**"} {
			if !strings.Contains(text, token) {
				t.Errorf("%s lacks %q", name, token)
			}
		}
		if name != "small-change.md" && !strings.Contains(text, "| 종류 | 시나리오 | 조건 |") {
			t.Errorf("%s lacks the scenario table", name)
		}
	}
	if !strings.Contains(read(t, filepath.Join(dir, "cross-module-feature.md")), "across the real boundary") {
		t.Error("cross-module template must ask for a criterion across the real boundary")
	}
	// Spec AC-18, AC-19: decisions carry a source, and templates for changes to
	// existing code ask for today's behavior and what stays the same.
	for _, name := range []string{"backend-feature.md", "cross-module-feature.md", "product.md"} {
		if !strings.Contains(read(t, filepath.Join(dir, name)), "Source:") {
			t.Errorf("%s decisions lack a Source placeholder", name)
		}
	}
	for _, name := range []string{"backend-feature.md", "cross-module-feature.md"} {
		text := read(t, filepath.Join(dir, name))
		for _, token := range []string{"- Today:", "- Stays the same:"} {
			if !strings.Contains(text, token) {
				t.Errorf("%s lacks %q", name, token)
			}
		}
	}
}

// the maintenance-first rules are in the
// skill body.
func TestSpecMaintenanceRules(t *testing.T) {
	text := read(t, "spec/skills/spec/SKILL.md")
	for _, must := range []string{
		"decide what kind of request this is",
		"report in the conversation instead of pasting the full documents",
		"names its source",
		"Decisions already there without a source stay as they are: never invent one",
		"states today's behavior as fact",
		"saved settings and data",
		"Code shows what happens, not what was meant.",
		"Look in `docs/specs/` for goal documents that cover the same behavior",
		"Small is a depth, not a kind",
		"For a change to existing code, whatever its size:",
	} {
		if !strings.Contains(text, must) {
			t.Errorf("Spec SKILL.md lacks %q", must)
		}
	}
}

// the repository instructions keep testdata scenarios for rules ha
// decides and send skill-text rules to static checks and quality cases.
func TestAgentsScopesScenarioRule(t *testing.T) {
	text := read(t, "../AGENTS.md")
	for _, must := range []string{"behavioral scenarios in `testdata/scenarios/`", "checks independent of private documents and earlier repository history"} {
		if !strings.Contains(text, must) {
			t.Errorf("AGENTS.md lacks %q", must)
		}
	}
}

// the skill body states no work order.
func TestSealSkillPrescribesNoOrder(t *testing.T) {
	ordering := regexp.MustCompile(`(?i)\b(first|then|afterwards|next step|step \d|before you|after you)\b|먼저|다음에|그다음`)
	for _, p := range []string{"seal/skills/seal/SKILL.md", "seal/skills/seal/references/bundle.md", "seal/skills/seal/references/ha.md"} {
		for i, line := range strings.Split(read(t, p), "\n") {
			if m := ordering.FindString(line); m != "" {
				t.Errorf("%s:%d uses ordering word %q: %s", p, i+1, m, line)
			}
		}
	}
}

// the completion rules and prohibitions from the role card
// are present, and the skill passes itself to ha start.
func TestSealSkillCarriesCompletionRules(t *testing.T) {
	text := read(t, "seal/skills/seal/SKILL.md")
	for _, must := range []string{
		"ha done", "completion record", "ha note <goal> confirm", "reopen", "input",
		"--skill", "never edit them", "runs.jsonl", "terminal failures", "ha log",
		"## 개선 메모", "`local`", "Status: Ready",
	} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(must)) {
			t.Errorf("Seal SKILL.md lacks %q", must)
		}
	}
}

func headings(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "| ") && strings.Contains(l, " | ") && !strings.Contains(l, "<") && !strings.HasPrefix(l, "| AC-") && !strings.HasPrefix(l, "| T0") && !strings.HasPrefix(l, "| ---") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// the example built from the templates passes lint,
// and it keeps every heading and table header of the templates.
func TestExampleFromTemplatesPassesLint(t *testing.T) {
	g, err := goaldocs.Load("..", "examples/empty-input")
	if err != nil {
		t.Fatal(err)
	}
	b, err := bundle.Load("..", g)
	if err != nil {
		t.Fatal(err)
	}
	if ps := bundle.Lint(g, b); len(ps) != 0 {
		t.Fatalf("example has lint problems: %v", ps)
	}
	for _, name := range []string{"PLAN.md", "REVIEW.md", "PROGRESS.md"} {
		tmpl := read(t, filepath.Join("seal/skills/seal/assets/templates", name))
		example := read(t, filepath.Join("../examples/empty-input", name))
		for _, h := range headings(tmpl) {
			if !strings.Contains(example, h) {
				t.Errorf("example %s lacks template line %q", name, h)
			}
		}
	}
}
