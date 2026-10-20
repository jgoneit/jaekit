package spar

import (
	"strings"
	"testing"
)

// The reference rule implements the reading table of contracts/review-round.md
// section 6: every accepted form of a location, a rule or contract link and a
// criterion ID, with the prefixes each field removes.
func TestReferenceRule(t *testing.T) {
	for _, tc := range []struct {
		field, text, path, whole string
		line                     int
		hasLine                  bool
	}{
		// location: <prefix><path>:<line>, one of head/, base/, ./ removed once
		{"location", "head/app/main.py:12", "app/main.py", "", 12, true},
		{"location", "head/app/main.py:12-14", "app/main.py", "", 12, true},
		{"location", "head/app/main.py:12–14", "app/main.py", "", 12, true},
		{"location", "head/checks/my test.py:2", "checks/my test.py", "", 2, true},
		{"location", "head/checks/test.py:2 see head/app.py:10", "checks/test.py", "", 2, true},
		{"location", "head/app/main.go:10 and head/app/main_test.go:20", "app/main.go", "", 10, true},
		{"location", "head/checks/my test.py:2-4 and head/app.py:9", "checks/my test.py", "", 2, true},
		{"location", "base/app/old.py:3", "app/old.py", "", 3, true},
		{"location", "./app/main.py:4", "app/main.py", "", 4, true},
		{"location", "head/base/checks/test.py:2", "base/checks/test.py", "", 2, true},
		{"location", "head/head/x.py:1", "head/x.py", "", 1, true},
		{"location", "base/base/x.py:1", "base/x.py", "", 1, true},
		{"location", "head/base/head/x.py:5", "base/head/x.py", "", 5, true},
		{"location", "head/app/main.go", "app/main.go", "app/main.go", 0, false},
		{"location", "head/app/main.go and more", "app/main.go", "app/main.go and more", 0, false},
		{"location", "  head/app/main.go:7  ", "app/main.go", "", 7, true},
		// rule and contract links: the same form, only head/ or ./ removed once
		{"link", "AGENTS.md:12", "AGENTS.md", "", 12, true},
		{"link", "AGENTS.md:12-14", "AGENTS.md", "", 12, true},
		{"link", "head/AGENTS.md:2", "AGENTS.md", "", 2, true},
		{"link", "./AGENTS.md:1", "AGENTS.md", "", 1, true},
		{"link", "AGENTS.md:0", "AGENTS.md", "", 0, true},
		{"link", "rules/API rules.md:2", "rules/API rules.md", "", 2, true},
		{"link", "rules/API rules.md:2 (the second rule)", "rules/API rules.md", "", 2, true},
		{"link", "AGENTS.md:3 and contracts/x.md:4", "AGENTS.md", "", 3, true},
		{"link", "base/api.md:3", "base/api.md", "", 3, true},
		{"link", "head/base/api.md:3", "base/api.md", "", 3, true},
		{"link", "head/head/x.md:2", "head/x.md", "", 2, true},
		{"link", "AGENTS.md", "AGENTS.md", "AGENTS.md", 0, false},
		{"link", "rules/API rules.md", "rules/API", "rules/API rules.md", 0, false},
		{"link", "base/rules/review.md", "base/rules/review.md", "base/rules/review.md", 0, false},
		{"link", "AGENTS.md L54", "AGENTS.md", "AGENTS.md L54", 0, false},
	} {
		prefixes := locationPrefixes
		if tc.field == "link" {
			prefixes = linkPrefixes
		}
		r := parseReference(tc.text, prefixes)
		if r.path != tc.path || r.whole != tc.whole || r.line != tc.line || r.hasLine != tc.hasLine {
			t.Errorf("%s %q: got %+v", tc.field, tc.text, r)
		}
		if tc.field == "location" {
			if got := locationPath(tc.text); got != tc.path {
				t.Errorf("locationPath(%q) = %q, want %q", tc.text, got, tc.path)
			}
		}
	}
	// criterion links and criteria documents: AC-<n> not right after a letter or digit
	for _, tc := range []struct{ text, ids string }{
		{"AC-1", "AC-1"},
		{"AC-1,AC-2: both commands refuse empty names", "AC-1,AC-2"},
		{"AC-2, AC-3", "AC-2,AC-3"},
		{"(AC-4) and AC-10.", "AC-4,AC-10"},
		{"- **AC-5** one", "AC-5"},
		{"기준AC-6", "AC-6"},
		{"XAC-7 and 9AC-8 are not IDs", ""},
		{"AC-", ""},
	} {
		if got := strings.Join(criterionIDs(tc.text), ","); got != tc.ids {
			t.Errorf("criterionIDs(%q) = %q, want %q", tc.text, got, tc.ids)
		}
	}
}

// Every consumer of the rule reads the same forms: the check-path judgment of
// a location and the verification of rule links.
func TestReferenceConsumers(t *testing.T) {
	rules := []Document{newDocument("rules/API rules.md", "/r/rules/API rules.md", []byte("a\nb\nc\n")),
		newDocument("AGENTS.md", "/r/AGENTS.md", []byte("a\nb\n")),
		newDocument("base/rules/review.md", "/r/base/rules/review.md", []byte("a\nb\nc\n"))}
	c := classifier{checkPaths: []string{"base/checks/**"}, targets: LinkTargets{Rules: ruleTargets("/r", rules)}}
	if f := c.classify("codex", 1, rawFinding("F1", map[string]any{"location": "head/base/checks/test.py:2",
		"check_impact": "false_pass"})); f.Class != "defect" || !f.InCheckPaths || f.Path != "base/checks/test.py" {
		t.Errorf("a repository folder named base: %s %v %s", f.Class, f.InCheckPaths, f.Path)
	}
	for link, want := range map[string]bool{
		"rules/API rules.md:3": true, "rules/API rules.md:4": false, "rules/API rules.md": true,
		"head/AGENTS.md:2": true, "AGENTS.md:0": false, "AGENTS.md L2": true, "missing.md": false,
		"base/rules/review.md:3": true, "head/base/rules/review.md:3": true,
	} {
		if got := c.verifyLink("rule", link); got != want {
			t.Errorf("rule link %q: %v, want %v", link, got, want)
		}
	}
}

// agy 1.4.3 reads files with read_file; tools that write stay disallowed.
func TestAgyReadTools(t *testing.T) {
	dir := t.TempDir()
	spec := agySpec(Reviewer{Name: "agy", TimeoutSeconds: 60}, "m", "agy",
		runPaths{workspace: dir, dir: dir, schema: dir + "/schema.json"}, []byte("packet"))
	tool := func(name string) map[string]any {
		return map[string]any{"step_update": map[string]any{"step_type": "tool", "tool_name": name,
			"tool_info": map[string]any{"parameters": map[string]any{"Path": "head/app.py"}}}}
	}
	for name, allowed := range map[string]bool{"read_file": true, "view_file": true, "list_dir": true,
		"write_to_file": false, "replace_file_content": false} {
		if got := spec.checkEvent(tool(name)) == ""; got != allowed {
			t.Errorf("agy tool %s: allowed %v, want %v", name, got, allowed)
		}
	}
}
