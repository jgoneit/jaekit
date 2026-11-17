//go:build protectedcheckfollowups

package protectedchecks

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// run runs a command in the checked tree and returns its output and whether it exited 0.
func run(t *testing.T, name string, args ...string) (string, bool) {
	t.Helper()
	cmd := exec.Command(tool(t, name), args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = childEnv()
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); err != nil && (!ok || exit.ExitCode() < 0) {
		t.Fatalf("%s %v did not finish: %v\n%s", name, args, err, out)
	}
	return string(out), err == nil
}

// The criteria of each contract producer that pass on main where this goal
// started; the producer must still list each of them.
var passing = map[string][]string{
	"reporting":       {"AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-7"},
	"design":          {"AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-7", "AC-9"},
	"finding-docs":    {"AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6"},
	"spec-boundaries": {"AC-1", "AC-2", "AC-3", "AC-4", "AC-7"},
	"authoring":       {"AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-9", "AC-10"},
}

// The two authoring criteria already violated there (SPEC: 그대로 두는 것),
// with the problems they report; this goal leaves them as they are.
var violated = map[string][]string{
	"AC-6": {
		"user-resolved reasons changed: ['review_open_question']",
		"plugins/seal/skills/seal/references/ha.md: guidance may add a user step: Seal 0.1.23 asks the decisions it can foresee in one message before any baseline and lists the test and check files a plan adds in its scope from the start, wit",
	},
	"AC-8": {"guidance does not name Seal 0.1.31 as its starting version"},
}

// The contract producers give the checked tree the verdicts of main where
// this goal started: the passing criteria pass, and the two violated ones
// report the same problems.
func TestProtectedAC7_ContractVerdicts(t *testing.T) {
	root := repoRoot(t)
	for suite, criteria := range passing {
		for _, criterion := range criteria {
			if obs := observe(t, "contract", root, suite, criterion); obs.Status != "pass" {
				t.Errorf("%s %s is %s on the checked tree: %q", suite, criterion, obs.Status, obs.Problems)
			}
		}
	}
	for criterion, want := range violated {
		obs := observe(t, "contract", root, "authoring", criterion)
		got := slices.Sorted(slices.Values(obs.Problems))
		if obs.Status != "violation" || !slices.Equal(got, slices.Sorted(slices.Values(want))) {
			t.Errorf("authoring %s is %s %q on the checked tree, not the violation %q", criterion, obs.Status, obs.Problems, want)
		}
	}
}

// The protected routes' tests pass on the checked tree, the input-failure
// cases of CurrentTree and ResultMeaning included.
func TestProtectedAC7_ProtectedTests(t *testing.T) {
	for _, args := range [][]string{
		{"-m", "unittest", "discover", "-s", "tools/goal-checks", "-p", "test_authoring_check.py"},
		{"-m", "unittest", "discover", "-s", "tools/release-checks", "-p", "integration_evidence_boundaries.py"},
	} {
		if out, ok := run(t, "python3", args...); !ok {
			t.Errorf("python3 %v does not pass on the checked tree:\n%s", args, tail(out, 30))
		}
	}
}

// A clause on SKILL.md does not read references/earlier-goals.md; a clause
// on that document does.
func TestProtectedAC7_AnchorExclusionKept(t *testing.T) {
	const marker = "jaekit-protected-check-marker"
	root := docTree(t)
	appendText(t, filepath.Join(root, earlierDoc), marker)
	skill := observe(t, "clause", root, skillDoc).Text
	earlier := observe(t, "clause", root, earlierDoc).Text
	if skill == nil || earlier == nil {
		t.Fatalf("a clause lookup found no document")
	}
	if strings.Contains(*skill, marker) {
		t.Errorf("a clause on %s reads %s", skillDoc, earlierDoc)
	}
	if !strings.Contains(*earlier, marker) {
		t.Errorf("a clause on %s does not read that document", earlierDoc)
	}
}

// routeCheck is one check of a route record.
type routeCheck struct {
	File     string `json:"file"`
	Selector string `json:"selector"`
}

// What each of the three protected routes records once this goal's checks
// changed: its sources, its check, and the fixed tokens of its new examples,
// property and boundary.
var routeRecords = map[string]struct {
	sources, examples, property, boundary []string
	check                                 routeCheck
}{
	"authoring-check-current-tree": {
		sources:  []string{"tools/contract-checks/authoring_check.py", "tools/contract-checks/check.py"},
		examples: []string{"clause phrase only in the earlier-goals reference", "clause phrase absent from every skill document"},
		property: []string{"in the earlier-goals reference only when the clause names it"},
		check:    routeCheck{"tools/goal-checks/test_authoring_check.py", "CurrentTree"},
	},
	"authoring-check-result-meaning": {
		sources: []string{"tools/contract-checks/authoring_check.py", "tools/contract-checks/check.py"},
		check:   routeCheck{"tools/goal-checks/test_authoring_check.py", "ResultMeaning"},
	},
	"observation-identity-gap": {
		sources:  []string{"tools/release-checks/source_identity.py"},
		boundary: []string{"Fixture repositories turn off Git's automatic maintenance"},
		check:    routeCheck{"tools/release-checks/integration_evidence_boundaries.py", "SourceIdentityEvidence"},
	},
}

// The three protected routes stay protected, the registry is valid, each
// route check names a class or test that exists in its file, and each record
// carries what routeRecords lists.
func TestProtectedAC7_RouteRecords(t *testing.T) {
	if out, ok := run(t, "python3", "tools/regression_map.py", "--format", "json"); !ok {
		t.Errorf("the regression registry is not valid:\n%s", tail(out, 20))
	}
	var registry struct {
		Routes []struct {
			ID       string       `json:"id"`
			Status   string       `json:"status"`
			Sources  []string     `json:"sources"`
			Examples []string     `json:"examples"`
			Property string       `json:"property"`
			Boundary string       `json:"boundary"`
			Checks   []routeCheck `json:"checks"`
		} `json:"routes"`
	}
	root := repoRoot(t)
	if err := json.Unmarshal([]byte(read(t, filepath.Join(root, "contracts", "regressions.json"))), &registry); err != nil {
		t.Fatalf("contracts/regressions.json: %v", err)
	}
	found := map[string]bool{}
	for _, route := range registry.Routes {
		want, ok := routeRecords[route.ID]
		if !ok {
			continue
		}
		found[route.ID] = true
		if route.Status != "protected" {
			t.Errorf("route %s is %q, not protected", route.ID, route.Status)
		}
		for _, source := range want.sources {
			if !slices.Contains(route.Sources, source) {
				t.Errorf("route %s does not list the source %s", route.ID, source)
			}
		}
		for _, example := range want.examples {
			if !slices.Contains(route.Examples, example) {
				t.Errorf("route %s does not list the example %q", route.ID, example)
			}
		}
		for _, token := range want.property {
			if !strings.Contains(route.Property, token) {
				t.Errorf("route %s property does not say %q", route.ID, token)
			}
		}
		for _, token := range want.boundary {
			if !strings.Contains(route.Boundary, token) {
				t.Errorf("route %s boundary does not say %q", route.ID, token)
			}
		}
		if !slices.Contains(route.Checks, want.check) {
			t.Errorf("route %s does not check %s %s", route.ID, want.check.File, want.check.Selector)
		}
		for _, check := range route.Checks {
			text := read(t, filepath.Join(root, check.File))
			if !strings.Contains(text, "class "+check.Selector+"(") && !strings.Contains(text, "func "+check.Selector+"(") {
				t.Errorf("route %s names %s in %s, which has no such class or test", route.ID, check.Selector, check.File)
			}
		}
	}
	for id := range routeRecords {
		if !found[id] {
			t.Errorf("route %s is missing from contracts/regressions.json", id)
		}
	}
}
