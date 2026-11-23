// Package plugins holds static checks for the Spec and Seal plugin packages.
// The checks cover the build specs' acceptance criteria that can be decided
// from the files alone; loading in a host and real runs are verified
// separately.
package plugins

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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

// nothing in the Spec plugin refers to Seal's
// commands, bundle files, or run states.
func TestSpecPluginKnowsNothingOfSeal(t *testing.T) {
	forbidden := []*regexp.Regexp{
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

// Spec: agents may invoke it on both hosts.
func TestSpecInvocationBoundary(t *testing.T) {
	keys, _ := frontMatter(t, "spec/skills/spec/SKILL.md")
	if _, ok := keys["disable-model-invocation"]; ok {
		t.Error("Spec SKILL.md must not set disable-model-invocation")
	}
	if !strings.Contains(read(t, "spec/skills/spec/agents/openai.yaml"), "allow_implicit_invocation: true") {
		t.Error("Spec openai.yaml must allow implicit invocation")
	}
}

// the status follows the open decisions: the skill keeps the Ready status
// that documents with none open take.
func TestSpecStatusFollowsOpenDecisions(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Ready")
}

// templates carry the goal document format tokens.
func TestSpecTemplates(t *testing.T) {
	dir := "spec/skills/spec/assets/templates"
	for _, name := range []string{"small-change.md", "backend-feature.md", "cross-module-feature.md", "product.md"} {
		text := read(t, filepath.Join(dir, name))
		for _, token := range []string{"Status: Draft", "## Acceptance Criteria", "**AC-1**"} {
			if !strings.Contains(text, token) {
				t.Errorf("%s lacks %q", name, token)
			}
		}
	}
}

// The fixed forms that prescribe a work order in a Seal document. Ordering
// words in prose ("first", "then", "먼저") are not such a form.
const (
	orderNumberedItem = "numbered list item" // a line that begins `1.` or `2)`
	orderStepLabel    = "Step N label"       // `Step 1` or `Step 1:` opening a line, heading or list item
)

var (
	sealNumberedItem = regexp.MustCompile(`^\s*(?:>\s*)*\d{1,9}[.)](?:\s|$)`)
	sealStepLabel    = regexp.MustCompile(`^\s*(?:>\s*)*(?:#{1,6}\s+|[-*+]\s+|\d{1,9}[.)]\s+)?(?:\*\*|__|\*|_)?(?i:step)\s+\d+\b`)
)

// sealOrderForm returns the work-order form line has, or "" when it has none.
func sealOrderForm(line string) string {
	switch {
	case sealStepLabel.MatchString(line):
		return orderStepLabel
	case sealNumberedItem.MatchString(line):
		return orderNumberedItem
	}
	return ""
}

// sealLayoutSections are the sections, by document and innermost heading,
// whose numbered lists order the parts of a document the executor writes,
// not its work: the headings of `## 검수 요약` and the items of the
// completion report. Only their unfenced numbered list items are accepted;
// a Step label is reported there too.
var sealLayoutSections = []struct{ path, heading string }{
	{sealSkillDir + "/references/bundle.md", "## Review summary"},
	{sealSkillDir + "/references/bundle.md", "## Completion report"},
}

func sealLayoutSection(path, heading string) bool {
	for _, s := range sealLayoutSections {
		if s.path == path && s.heading == heading {
			return true
		}
	}
	return false
}

// sealOrderLines returns each line of a Seal document, given by its path
// relative to plugins/, that has a work-order form outside the layout
// sections, as "path:line: form: text".
func sealOrderLines(path, text string) []string {
	var out []string
	heading, fence := "", ""
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			fence = trimmed[:3]
		case strings.HasPrefix(line, "#"):
			heading = trimmed
		}
		form := sealOrderForm(line)
		if form == "" || form == orderNumberedItem && fence == "" && sealLayoutSection(path, heading) {
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d: %s: %s", path, i+1, form, line))
	}
	return out
}

// no document of the Seal skill states a work order: no numbered list of
// work and no Step label. Each layout section it accepts still exists.
func TestSealSkillPrescribesNoOrder(t *testing.T) {
	for _, doc := range skillDocs(t, sealSkillDir) {
		for _, found := range sealOrderLines(doc.path, doc.text) {
			t.Errorf("%s", found)
		}
	}
	for _, s := range sealLayoutSections {
		if !strings.Contains("\n"+read(t, s.path)+"\n", "\n"+s.heading+"\n") {
			t.Errorf("%s has no layout section %q: drop it from sealLayoutSections", s.path, s.heading)
		}
	}
}

// The line rule rejects the fixed forms of a work order and accepts prose
// that only uses an ordering word.
func TestSealOrderForms(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"1. Run `ha start`.", orderNumberedItem},
		{"2) Record the baseline.", orderNumberedItem},
		{"   3. Run the checks.", orderNumberedItem},
		{"> 4. Record `ha done`.", orderNumberedItem},
		{"10.", orderNumberedItem},
		{"# Step 1", orderStepLabel},
		{"### Step 1: Plan", orderStepLabel},
		{"Step 2 records the baseline.", orderStepLabel},
		{"- **Step 3.** Run the checks.", orderStepLabel},
		{"1. Step 1: Plan", orderStepLabel},
		{"Record the baseline, then run the checks.", ""},
		{"First read the goal documents; afterwards, report.", ""},
		{"먼저 목표 문서를 읽고 그다음 보고한다.", ""},
		{"Widen the boundary record in the same step as the fix.", ""},
		{"Redesign step 5 stays a later goal.", ""},
		{"1.5 times the budget", ""},
		{"- `ha start` records the start.", ""},
		{"| 1. | table cell |", ""},
	} {
		if got := sealOrderForm(c.line); got != c.want {
			t.Errorf("sealOrderForm(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

// A numbered list is accepted only in a layout section of its own document,
// and never under a subheading of one or inside a fence.
func TestSealOrderLinesLayoutSections(t *testing.T) {
	doc := strings.Join([]string{
		"## Completion report",
		"1. The `ha report` output.",
		"2. The executor's judgment.",
		"```",
		"## Not a heading",
		"1. Fenced item.",
		"```",
		"### Steps",
		"1. Run the checks.",
		"## Work",
		"1. Run `ha start`.",
		"## Completion report",
		"Step 1: Plan",
	}, "\n")
	path := sealSkillDir + "/references/bundle.md"
	got := sealOrderLines(path, doc)
	want := []string{
		path + ":6: numbered list item: 1. Fenced item.",
		path + ":9: numbered list item: 1. Run the checks.",
		path + ":11: numbered list item: 1. Run `ha start`.",
		path + ":13: Step N label: Step 1: Plan",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("sealOrderLines(bundle.md) =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if other := sealOrderLines(sealSkillDir+"/SKILL.md", doc); len(other) != 6 {
		t.Errorf("sealOrderLines(SKILL.md) reported %d lines, want all 6:\n%s", len(other), strings.Join(other, "\n"))
	}
}

// the skill keeps the format tokens of the completion rules and
// prohibitions from the role card, and passes itself to ha start.
func TestSealSkillCarriesCompletionRules(t *testing.T) {
	requireSkillTokens(t, sealSkillDir,
		"ha done", "ha note <goal> confirm", "reopen", "input", "--skill", "runs.jsonl", "ha log",
		"local", "Status: Ready")
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
	// Declarations bind committed inputs. Commit the copied public example in
	// an isolated fixture so this check also works before a source change is
	// committed, without weakening the production HEAD binding.
	root := t.TempDir()
	paths, err := filepath.Glob("../examples/empty-input/*")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../tools/check-result-reference.py")
	for _, source := range paths {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, strings.TrimPrefix(source, "../"))
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, argv := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-qm", "synthetic example"}} {
		command := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, argv...)...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("fixture git: %v %s", err, output)
		}
	}
	g, err := goaldocs.Load(root, "examples/empty-input")
	if err != nil {
		t.Fatal(err)
	}
	b, err := bundle.Load(root, g)
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
