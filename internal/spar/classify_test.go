package spar

import (
	"strings"
	"testing"
)

func rawFinding(id string, values map[string]any) map[string]any {
	f := map[string]any{"id": id, "proposed_class": "defect", "angle": "design", "location": "head/app/main.py:2",
		"claim": "c", "failure_scenario": "s", "evidence": "e", "link_kind": "none", "link": "",
		"introduced_by_change": "no", "regression": "no", "check_impact": "none", "confidence": "high"}
	for k, v := range values {
		f[k] = v
	}
	return f
}

func TestClassification(t *testing.T) {
	criteria := []Document{newDocument("SPEC.md", "/r/SPEC.md", []byte("- **AC-1** one\n- **AC-2** two\n- **AC-10** ten\n"))}
	rules := []Document{newDocument("AGENTS.md", "/r/AGENTS.md", []byte("a\nb\nc\n"))}
	c := classifier{checkPaths: []string{"checks/**"}, targets: LinkTargets{CriteriaIDs: criteriaIDs(criteria),
		Rules: ruleTargets("/r", rules)}}
	check := "head/checks/test_app.py:2"
	cases := []struct {
		values map[string]any
		class  string
		linked bool
	}{
		{map[string]any{"location": check, "link_kind": "criterion", "link": "AC-1"}, "hardening", true},
		{map[string]any{"location": check, "check_impact": "false_pass"}, "defect", false},
		{map[string]any{"location": check, "check_impact": "meaning_inverted"}, "defect", false},
		{map[string]any{"location": check, "check_impact": "contract_violation", "link_kind": "rule", "link": "AGENTS.md:3"}, "defect", true},
		{map[string]any{"location": check, "check_impact": "contract_violation", "link_kind": "rule", "link": "AGENTS.md:4"}, "hardening", false},
		{map[string]any{"proposed_class": "note", "introduced_by_change": "yes", "regression": "yes"}, "defect", false},
		{map[string]any{"link_kind": "criterion", "link": "AC-99"}, "note", false},
		{map[string]any{"link_kind": "criterion", "link": "AC-10"}, "defect", true},
		{map[string]any{"link_kind": "criterion", "link": "AC-2, AC-3"}, "note", false},
		{map[string]any{"proposed_class": "requirement-question", "location": check}, "requirement-question", false},
		{map[string]any{"proposed_class": "note"}, "note", false},
		{map[string]any{"link_kind": "rule", "link": "AGENTS.md"}, "defect", true},
	}
	for i, tc := range cases {
		f := c.classify("codex", rawFinding("F1", tc.values))
		if f.Class != tc.class || (f.LinkKind != "none") != tc.linked || f.ID != "codex-F1" {
			t.Errorf("case %d %v: class %s link %s/%s", i, tc.values, f.Class, f.LinkKind, f.Link)
		}
	}
}

func TestGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		want          bool
	}{
		{"checks/**", "checks/test_app.py", true},
		{"checks/**", "checks/a/b.py", true},
		{"checks/**", "app/checks/x.py", false},
		{"**/test_*.py", "a/b/test_x.py", true},
		{"**/test_*.py", "test_x.py", true},
		{"tools/*.py", "tools/a/b.py", false},
		{"cmd/spar", "cmd/spar", true},
	} {
		if got := matchGlob(tc.pattern, tc.name); got != tc.want {
			t.Errorf("%s ~ %s: %v", tc.pattern, tc.name, got)
		}
	}
}

func TestUnsafeCommand(t *testing.T) {
	for line, safe := range map[string]bool{
		"ls -la head":                          true,
		"grep -rn foo head | head -20":         true,
		"sed -n '1,40p' head/a.go":             true,
		"find head -name '*.go'":               true,
		"rm -rf head":                          false,
		"find head -delete":                    false,
		"cat a > b":                            false,
		"find head | xargs grep x":             false,
		"echo $(id)":                           false,
		"sed -i s/a/b/ head/x":                 false,
		"grep x head/a 2>/dev/null; touch pwn": false,
	} {
		if got := unsafeCommand(line) == ""; got != safe {
			t.Errorf("%q: safe=%v (%s)", line, got, unsafeCommand(line))
		}
	}
}

func TestUnavailableCause(t *testing.T) {
	reason, resets, quota := unavailableCause([]string{"RESOURCE_EXHAUSTED (code 429): Individual quota reached. Resets in 2h13m."})
	if !quota || resets != "Resets in 2h13m" || !strings.HasPrefix(reason, "quota") {
		t.Fatalf("%q %q %v", reason, resets, quota)
	}
	reason, _, quota = unavailableCause([]string{"401 Unauthorized: run agy login"})
	if quota || !strings.HasPrefix(reason, "authentication") {
		t.Fatalf("auth: %q %v", reason, quota)
	}
	if reason, _, _ := unavailableCause([]string{"synthetic crash"}); reason != "" {
		t.Fatalf("a crash is not a refusal: %q", reason)
	}
}

func TestValidateResult(t *testing.T) {
	head := "0123456789abcdef0123456789abcdef01234567"
	angles := []string{"design", "premises"}
	good := map[string]any{"reviewed_head": head[:12], "verdict": "no_blocking_findings",
		"angles_checked":   []any{map[string]any{"angle": "design", "result": "ok"}},
		"angles_unchecked": []any{map[string]any{"angle": "premises", "reason": "time"}},
		"findings":         []any{rawFinding("F1", nil)}, "limits": ""}
	if p := validateResult(good, head, angles); len(p) != 0 {
		t.Fatalf("good result refused: %v", p)
	}
	for name, change := range map[string]func(map[string]any){
		"no angles":   func(m map[string]any) { delete(m, "angles_checked"); delete(m, "angles_unchecked") },
		"missing one": func(m map[string]any) { m["angles_unchecked"] = []any{} },
		"other head":  func(m map[string]any) { m["reviewed_head"] = "fedcba98" },
		"bad class": func(m map[string]any) {
			m["findings"] = []any{rawFinding("F1", map[string]any{"proposed_class": "bug"})}
		},
		"duplicate id": func(m map[string]any) { m["findings"] = []any{rawFinding("F1", nil), rawFinding("F1", nil)} },
	} {
		m := map[string]any{}
		for k, v := range good {
			m[k] = v
		}
		change(m)
		if p := validateResult(m, head, angles); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}
