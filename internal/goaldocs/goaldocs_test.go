package goaldocs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func codes(g *Goal) []string {
	var out []string
	for _, p := range g.Problems {
		out = append(out, p.Code)
	}
	return out
}

const spec = `# Greeting

Status: Ready

See [the contract](contract.md) and [the plan](PLAN.md).

## Acceptance Criteria
- **AC-1** greeting says hello
- **AC-2** (선택) keep stays
  even after the change

## 작업 분해 제안
| ID | 결과 | 조건 | 의존 |
| --- | --- | --- | --- |
| W1 | greeting | AC-1 | — |
| W2 | keep | AC-2 | W1 |
`

func TestLoadReadsCriteriaProposalsAndDocSet(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/specs/g/SPEC.md", spec)
	write(t, root, "docs/specs/g/contract.md", "see [more](sub/more.md)\n")
	write(t, root, "docs/specs/g/sub/more.md", "leaf\n")
	write(t, root, "docs/specs/g/PLAN.md", "plan\n")
	g, err := Load(root, "docs/specs/g")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Problems) != 0 {
		t.Fatalf("problems: %v", g.Problems)
	}
	if g.Status != "Ready" {
		t.Fatalf("status = %q", g.Status)
	}
	if len(g.Criteria) != 2 || g.Criteria[0].Optional || !g.Criteria[1].Optional {
		t.Fatalf("criteria = %#v", g.Criteria)
	}
	if g.Criteria[1].Text != "keep stays\neven after the change" {
		t.Fatalf("criterion text = %q", g.Criteria[1].Text)
	}
	want := []string{"docs/specs/g/SPEC.md", "docs/specs/g/contract.md", "docs/specs/g/sub/more.md"}
	if !reflect.DeepEqual(g.Docs, want) {
		t.Fatalf("docs = %#v", g.Docs)
	}
	before := g.Digest
	write(t, root, "docs/specs/g/sub/more.md", "changed\n")
	g2, _ := Load(root, "docs/specs/g")
	if g2.Digest == before {
		t.Fatal("changing a linked auxiliary document must change the goal digest")
	}
	write(t, root, "docs/specs/g/PLAN.md", "plan changed\n")
	g3, _ := Load(root, "docs/specs/g")
	if g3.Digest != g2.Digest {
		t.Fatal("bundle files must not be part of the goal digest")
	}
}

func TestLoadReportsFormatProblems(t *testing.T) {
	root := t.TempDir()
	write(t, root, "g/SPEC.md", `# X
Status: Draft
[gone](missing.md)
## Acceptance Criteria
- **AC-1** one
- two without id
- **AC-1** duplicate
## 작업 분해 제안
| ID | 결과 | 조건 | 의존 |
| --- | --- | --- | --- |
| W1 | a | AC-9 | W2 |
| W2 | b | AC-1 | W1 |
| W3 | c | AC-1 | W7 |
`)
	g, err := Load(root, "g")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range codes(g) {
		got[c] = true
	}
	for _, c := range []string{"criterion_without_id", "criterion_duplicate_id", "proposal_unknown_criterion", "proposal_unknown_dependency", "proposal_cycle", "goal_doc_link_missing"} {
		if !got[c] {
			t.Errorf("missing problem %s; got %v", c, codes(g))
		}
	}
}

func TestMissingSpec(t *testing.T) {
	g, err := Load(t.TempDir(), "g")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(codes(g), []string{"spec_missing"}) {
		t.Fatalf("problems = %v", codes(g))
	}
}
