package bundle

import (
	"os/exec"
	"strings"
	"testing"
)

const declaration = `{"schema":"check-declaration/v1","producer":"test","producer_version":"1","producer_paths":["tests/greeting.sh"],"targets":[{"id":"greeting","violation":"missing-greeting"}]}`

func evidenceGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git: %v %s", err, out)
	}
}

func evidenceFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "g/SPEC.md", spec)
	write(t, root, "g/REVIEW.md", "# REVIEW\n")
	write(t, root, "g/PROGRESS.md", progress)
	write(t, root, "g/tasks/T001.md", task)
	write(t, root, "tests/greeting.sh", "exit 1\n")
	write(t, root, "tests/declaration.json", declaration)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-qm", "fixture"}} {
		evidenceGit(t, root, args...)
	}
	p := strings.Replace(plan, "| tests/greeting.sh |", "| tests/greeting.sh, tests/declaration.json |", 1)
	p += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | tests/declaration.json |\n"
	write(t, root, "g/PLAN.md", p)
	return root, p
}

func TestEvidenceDeclarationBindsOnlyNewRules(t *testing.T) {
	root, _ := evidenceFixture(t)
	g, b := load(t, root)
	if ps := Lint(g, b); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
	r := b.Row("AC-1")
	if r.Evidence == nil || r.EvidencePath != "tests/declaration.json" || r.EvidenceDigest == "" {
		t.Fatalf("%+v", r)
	}
	old, fresh := r.CommandDigest(), r.CommandDigestFor("run-rules/3")
	for _, rules := range []string{"run-rules/1", "run-rules/2"} {
		if got := r.CommandDigestFor(rules); got != old {
			t.Fatalf("legacy digest changed: %s", rules)
		}
	}
	write(t, root, "tests/greeting.sh", "exit 2\n")
	_, dirty := load(t, root)
	if len(dirty.Problems) == 0 {
		t.Fatal("dirty evidence input accepted")
	}
	evidenceGit(t, root, "add", ".")
	evidenceGit(t, root, "commit", "-qm", "producer update")
	_, b = load(t, root)
	if b.Row("AC-1").CommandDigestFor("run-rules/3") == fresh {
		t.Fatal("producer update reused /3 evidence")
	}
	if b.Row("AC-1").CommandDigestFor("run-rules/2") != old {
		t.Fatal("producer update changed legacy digest")
	}
	write(t, root, "tests/declaration.json", strings.Replace(declaration, "missing-greeting", "changed-meaning", 1))
	evidenceGit(t, root, "add", ".")
	evidenceGit(t, root, "commit", "-qm", "declaration update")
	_, changed := load(t, root)
	if changed.Row("AC-1").CommandDigestFor("run-rules/3") == b.Row("AC-1").CommandDigestFor("run-rules/3") {
		t.Fatal("declaration update reused evidence")
	}
}

func TestEvidenceDeclarationDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, old, new, code string }{
		{"missing-overlay", ", tests/declaration.json", "", "evidence_overlay_missing"},
		{"unknown-condition", "| AC-1 | tests/declaration.json |", "| AC-99 | tests/declaration.json |", "evidence_invalid"},
		{"maintain-contract", "| AC-1 | tests/declaration.json |", "| AC-2 | tests/declaration.json |", "evidence_invalid"},
		{"duplicate-contract", "| AC-1 | tests/declaration.json |", "| AC-1 | tests/declaration.json |\n| AC-1 | tests/declaration.json |", "evidence_invalid"},
		{"outside-path", "| AC-1 | tests/declaration.json |", "| AC-1 | ../declaration.json |", "evidence_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, p := evidenceFixture(t)
			write(t, root, "g/PLAN.md", strings.Replace(p, tc.old, tc.new, 1))
			_, b := load(t, root)
			for _, problem := range b.Problems {
				if problem.Code == tc.code {
					return
				}
			}
			t.Fatalf("missing %s: %v", tc.code, b.Problems)
		})
	}
}

func TestLegacyRulesIgnoreResultDeclarationSection(t *testing.T) {
	for _, variant := range []string{"free-text", "malformed-table", "dirty-producer"} {
		t.Run(variant, func(t *testing.T) {
			root, p := evidenceFixture(t)
			switch variant {
			case "free-text":
				p = plan + "\n## 결과 계약\nAn existing goal may use this heading for free text.\n"
			case "malformed-table":
				p = strings.Replace(p, "| ID | 선언 경로 |", "| Condition | Notes |", 1)
			case "dirty-producer":
				write(t, root, "tests/greeting.sh", "exit 2\n")
			}
			write(t, root, "g/PLAN.md", p)
			g, current := load(t, root)
			if len(current.Problems) == 0 {
				t.Fatal("new rules silently accepted an invalid result contract")
			}
			for _, rules := range []string{"run-rules/1", "run-rules/2"} {
				legacy, err := LoadForRules(root, g, rules)
				if err != nil {
					t.Fatal(err)
				}
				if problems := Lint(g, legacy); len(problems) != 0 {
					t.Fatalf("%s: %v", rules, problems)
				}
				row := legacy.Row("AC-1")
				if row.Evidence != nil || row.EvidencePath != "" || row.EvidenceDigest != "" || row.CommandDigestFor(rules) != row.CommandDigest() {
					t.Fatalf("%s interpreted the new declaration: %+v", rules, row)
				}
			}
		})
	}
}
