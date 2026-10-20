package spar

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The representative forms of the input table (contracts/spar-input.md),
// which the goal checks read too.
type inputForms struct {
	Locations struct {
		CheckPaths []string `json:"check_paths"`
		Head       []string `json:"head"`
		Base       []string `json:"base"`
		Subject    string   `json:"subject"`
		Cases      []struct {
			Location     string `json:"location"`
			Path         string `json:"path"`
			InCheckPaths bool   `json:"in_check_paths"`
			Unreadable   string `json:"unreadable"`
		} `json:"cases"`
	} `json:"locations"`
	UnreadableClasses struct {
		Location         string `json:"location"`
		ReadableLocation string `json:"readable_location"`
		RuleDocument     []any  `json:"rule_document"`
		Cases            []struct {
			Values     map[string]any `json:"values"`
			Readable   string         `json:"readable"`
			Unreadable string         `json:"unreadable"`
		} `json:"cases"`
	} `json:"unreadable_classes"`
	Links struct {
		Groups []linkGroup `json:"groups"`
	} `json:"links"`
	Criteria struct {
		Text  string   `json:"text"`
		IDs   []string `json:"ids"`
		Cases []struct {
			Link      string `json:"link"`
			Confirmed bool   `json:"confirmed"`
		} `json:"cases"`
	} `json:"criteria"`
	Globs struct {
		Patterns      []string        `json:"patterns"`
		Paths         map[string]bool `json:"paths"`
		ProjectErrors []string        `json:"project_errors"`
		FlagErrors    []string        `json:"flag_errors"`
	} `json:"globs"`
}

type linkGroup struct {
	Rules          [][]any  `json:"rules"`
	OutsideRules   [][]any  `json:"outside_rules"`
	Head           [][]any  `json:"head"`
	HeadSymlinks   [][]any  `json:"head_symlinks"`
	HeadSubmodules []string `json:"head_submodules"`
	BaseOnly       [][]any  `json:"base_only"`
	Cases          []struct {
		Kind      string `json:"kind"`
		Link      string `json:"link"`
		Confirmed bool   `json:"confirmed"`
	} `json:"cases"`
}

func loadInputForms(t *testing.T) inputForms {
	t.Helper()
	data, err := os.ReadFile("testdata/input-forms.json")
	if err != nil {
		t.Fatal(err)
	}
	var f inputForms
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func numberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line\n")
	}
	return b.String()
}

// memoryCommit is a commit held in memory: regular files with their
// contents, plus entries of other modes and kinds.
func memoryCommit(files map[string]string, others ...treeEntry) *commitFiles {
	var entries []treeEntry
	for p := range files {
		entries = append(entries, treeEntry{mode: "100644", kind: "blob", object: p, path: p})
	}
	entries = append(entries, others...)
	return newCommitFiles(entries, func(object string) ([]byte, error) {
		return []byte(files[object]), nil
	})
}

func pairs(list [][]any) map[string]int {
	out := map[string]int{}
	for _, p := range list {
		out[p[0].(string)] = int(p[1].(float64))
	}
	return out
}

func TestInputLocations(t *testing.T) {
	table := loadInputForms(t).Locations
	head, base := map[string]string{}, map[string]string{}
	for _, p := range table.Head {
		head[p] = "x\n"
	}
	for _, p := range table.Base {
		base[p] = "x\n"
	}
	subject := newDocument(table.Subject, "/r/"+table.Subject, []byte("# Plan\n"))
	c := classifier{repo: "/r", head: memoryCommit(head), base: memoryCommit(base), checkPaths: table.CheckPaths,
		docs: roundDocuments("/r", []Document{subject})}
	for i, tc := range table.Cases {
		f := c.classify("codex", i+1, rawFinding("F1", map[string]any{"location": tc.Location, "check_impact": "false_pass"}))
		if tc.Unreadable != "" {
			if !f.LocationUnreadable || f.Path != "" || f.InCheckPaths || !strings.HasPrefix(f.ClassReason, "unreadable location") {
				t.Errorf("%q (%s): path %q, in check paths %v, unreadable %v, reason %q", tc.Location, tc.Unreadable,
					f.Path, f.InCheckPaths, f.LocationUnreadable, f.ClassReason)
			}
			continue
		}
		if f.LocationUnreadable || f.Path != tc.Path || f.InCheckPaths != tc.InCheckPaths {
			t.Errorf("%q: path %q, in check paths %v, unreadable %v; want %q, %v", tc.Location, f.Path, f.InCheckPaths,
				f.LocationUnreadable, tc.Path, tc.InCheckPaths)
		}
	}
}

// An unreadable location never hides a defect: rules 3 and 6 do not apply
// and rules 1 and 2 apply without the check-path condition.
func TestInputUnreadableClasses(t *testing.T) {
	table := loadInputForms(t).UnreadableClasses
	name := table.RuleDocument[0].(string)
	rule := newDocument(name, "/outside/"+name, []byte(numberedLines(int(table.RuleDocument[1].(float64)))))
	c := classifier{repo: "/r", head: memoryCommit(map[string]string{"checks/t.py": numberedLines(20)}),
		base: memoryCommit(nil), checkPaths: []string{"checks/**"},
		targets: LinkTargets{Rules: ruleTargets("/r", []Document{rule})}}
	for _, tc := range table.Cases {
		for _, side := range []struct{ location, want string }{{table.Location, tc.Unreadable}, {table.ReadableLocation, tc.Readable}} {
			values := map[string]any{"location": side.location}
			for k, v := range tc.Values {
				values[k] = v
			}
			if f := c.classify("codex", 1, rawFinding("F1", values)); f.Class != side.want {
				t.Errorf("%v at %q: %s (%s), want %s", tc.Values, side.location, f.Class, f.ClassReason, side.want)
			}
		}
	}
}

// Each link group gives the table's results whatever the order of the rule
// documents.
func TestInputLinks(t *testing.T) {
	for gi, g := range loadInputForms(t).Links.Groups {
		head := map[string]string{"app/main.go": "x\n", "checks/t.py": "x\n"}
		var others []treeEntry
		var docs []Document
		for p, n := range pairs(g.Rules) {
			head[p] = numberedLines(n)
		}
		for _, p := range g.Rules {
			rel := p[0].(string)
			docs = append(docs, newDocument(rel, "/r/"+rel, []byte(head[rel])))
		}
		for _, p := range g.OutsideRules {
			rel := p[0].(string)
			docs = append(docs, newDocument(path.Base(rel), "/outside/"+rel, []byte(numberedLines(int(p[1].(float64))))))
		}
		for p, n := range pairs(g.Head) {
			head[p] = numberedLines(n)
		}
		for _, p := range g.HeadSymlinks {
			others = append(others, treeEntry{mode: "120000", kind: "blob", object: p[0].(string), path: p[0].(string)})
		}
		for _, p := range g.HeadSubmodules {
			others = append(others, treeEntry{mode: "160000", kind: "commit", object: p, path: p})
		}
		base := map[string]string{"app/main.go": "x\n", "checks/t.py": "x\n"}
		for p, n := range pairs(g.BaseOnly) {
			base[p] = numberedLines(n)
		}
		reversed := slices.Clone(docs)
		slices.Reverse(reversed)
		for _, order := range [][]Document{docs, reversed} {
			c := classifier{repo: "/r", head: memoryCommit(head, others...), base: memoryCommit(base),
				targets: LinkTargets{Rules: ruleTargets("/r", order)}}
			for _, tc := range g.Cases {
				if got := c.verifyLink(tc.Kind, tc.Link); got != tc.Confirmed {
					t.Errorf("group %d %s link %q: confirmed %v, want %v", gi, tc.Kind, tc.Link, got, tc.Confirmed)
				}
			}
		}
	}
}

func TestInputCriteria(t *testing.T) {
	table := loadInputForms(t).Criteria
	doc := newDocument("SPEC.md", "/r/SPEC.md", []byte(table.Text))
	ids := criteriaIDs([]Document{doc})
	if !slices.Equal(ids, table.IDs) {
		t.Errorf("criteria IDs %v, want %v", ids, table.IDs)
	}
	c := classifier{targets: LinkTargets{CriteriaIDs: ids}}
	for _, tc := range table.Cases {
		if got := c.verifyLink("criterion", tc.Link); got != tc.Confirmed {
			t.Errorf("criterion link %q: %v, want %v", tc.Link, got, tc.Confirmed)
		}
	}
}

func TestInputGlobs(t *testing.T) {
	table := loadInputForms(t).Globs
	for _, p := range table.Patterns {
		if err := validGlob(p); err != nil {
			t.Errorf("validGlob(%q): %v", p, err)
		}
	}
	for name, want := range table.Paths {
		if got := matchAny(table.Patterns, name); got != want {
			t.Errorf("%q matches %v, want %v", name, got, want)
		}
	}
	for _, p := range append(table.ProjectErrors, table.FlagErrors...) {
		if validGlob(p) == nil {
			t.Errorf("validGlob(%q) accepted a pattern outside the table", p)
		}
	}
}

// Line numbers of any size compare as numbers, so a reversed range is never
// read as a location or a link however many digits its lines have.
func TestInputHugeLines(t *testing.T) {
	c := classifier{repo: "/r", head: memoryCommit(map[string]string{"checks/t.py": "a\n"}), base: memoryCommit(nil),
		checkPaths: []string{"checks/**"},
		targets:    LinkTargets{Rules: ruleTargets("/r", []Document{newDocument("AGENTS.md", "/r/AGENTS.md", []byte("a\nb\n"))})}}
	for location, unreadable := range map[string]bool{
		"head/checks/t.py:100000000000000000000-99999999999999999999": true,
		"head/checks/t.py:1-100000000000000000000":                    false,
		"head/checks/t.py:0000":                                       true,
	} {
		f := c.classify("codex", 1, rawFinding("F1", map[string]any{"location": location}))
		if f.LocationUnreadable != unreadable {
			t.Errorf("%q: unreadable %v, want %v (%s)", location, f.LocationUnreadable, unreadable, f.ClassReason)
		}
	}
	for _, link := range []string{"AGENTS.md:99999999999999999999-1", "AGENTS.md:1-99999999999999999999"} {
		if c.verifyLink("rule", link) {
			t.Errorf("rule link %q was confirmed", link)
		}
	}
}

// A triage of a round whose ledger is gone is refused and writes no new
// ledger (G32).
func TestTriageRefusesAMissingLedger(t *testing.T) {
	f := newFixture(t)
	dir := f.round("01-candidate", &RoundRecord{Stage: "candidate", Base: f.revs[0], Head: f.revs[1], Sequence: 1},
		[]Finding{{ID: "x-F1", Class: "defect"}}, nil)
	os.Remove(filepath.Join(dir, "ledger.jsonl"))
	if err := Triage(TriageOptions{Round: dir, Finding: "x-F1", Disposition: "reject", Evidence: "e"}); err == nil {
		t.Fatal("a triage without the round's ledger was recorded")
	}
	if _, err := os.Stat(filepath.Join(dir, "ledger.jsonl")); err == nil {
		t.Fatal("a refused triage wrote a new ledger")
	}
}

// spar extend from a series outside every repository, without --repo, is
// checked only against the repositories that hold the path: none (G28).
func TestExtendOutsideEveryRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	series := t.TempDir()
	if underGitFolder(series) {
		t.Skip("the temporary folder is inside a Git repository")
	}
	t.Chdir(series)
	if err := Extend(".", "code", 1, "yes", "", nil); err != nil {
		t.Fatalf("an extension outside every repository: %v", err)
	}
	if _, err := os.Stat(filepath.Join(series, "extensions.jsonl")); err != nil {
		t.Fatalf("no extension was recorded: %v", err)
	}
}
