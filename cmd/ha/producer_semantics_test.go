package main

// The actual repository producers run as child processes of the Core command
// boundary. Fixtures and recorder histories are temporary and synthetic.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func producerFactsFixture(t *testing.T, body, expected string, command bool) *preflightFixture {
	return producerFactsFixtureFor(t, body, expected, command, "goal")
}

func producerFactsFixtureFor(t *testing.T, body, expected string, command bool, producer string) *preflightFixture {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	producerPath := "tools/" + producer + "-checks/produce.py"
	for _, name := range []string{producerPath, "tools/result_observations.py"} {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[name] = string(data)
	}
	f := newPreflightFixture(t)
	for name, data := range files {
		f.put(name, data)
	}
	names := map[string]any{"additional_violations": []string{"suite.assertion-2"}, "execution": "suite.execution", "selection": "suite.selection", "attribution": "suite.attribution"}
	targets := []map[string]string{{"id": "suite", "violation": expected}}
	for _, id := range []string{"suite.assertion-2", "suite.execution", "suite.selection", "suite.attribution"} {
		targets = append(targets, map[string]string{"id": id, "violation": "no-additional-violation"})
	}
	inputs := []string{producerPath, "tools/result_observations.py", "checks/fixture.py", "checks/targets.json"}
	producerName := "jaekit-goal-tests"
	commandText := "python3 tools/goal-checks/produce.py checks/declaration.json checks/targets.json"
	if producer != "goal" {
		producerName = "jaekit-" + producer + "-regressions"
		configuration := "tools/" + producer + "-checks/suites.json"
		inputs = []string{producerPath, "tools/result_observations.py", "checks/fixture.py", configuration}
		entry := map[string]any{"suite": map[string]any{"file": "checks/fixture.py", "class": "Case", "result_targets": names}}
		data, _ := json.Marshal(entry)
		f.put(configuration, string(data))
		commandText = "python3 " + producerPath + " suite"
	}
	decl := map[string]any{"schema": "check-declaration/v1", "producer": producerName, "producer_version": "2", "producer_paths": inputs, "targets": targets}
	data, _ := json.Marshal(decl)
	f.put("checks/declaration.json", string(data))
	check := map[string]any{"runner": "unittest", "target": "suite", "file": "checks/fixture.py", "test": "Case", "result_targets": names}
	if command {
		check = map[string]any{"runner": "command", "target": "suite", "argv": []string{"python3", "checks/fixture.py"}, "result_targets": names,
			"exit_contract": map[string]any{"schema": "jaekit-command-exits/v1", "pass": []int{0}, "violations": map[string]string{"1": "actual"}, "unverified": []int{2}}}
		f.put("checks/fixture.py", body)
	} else {
		f.put("checks/fixture.py", "import unittest\nclass Case(unittest.TestCase):\n"+body)
	}
	data, _ = json.Marshal(map[string]any{"schema": "jaekit-goal-targets/v2", "checks": []any{check}})
	f.put("checks/targets.json", string(data))
	plan := string(f.read(scenarioGoal + "/PLAN.md"))
	plan = strings.Replace(plan, "`sh tests/greeting.sh` | tests/greeting.sh", "`"+commandText+"` | "+strings.Join(append(inputs, "checks/declaration.json"), ", "), 1)
	plan = strings.Replace(plan, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `tools/**`, `checks/**`", 1)
	plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
	f.put(scenarioGoal+"/PLAN.md", plan)
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic producer facts")
	return f
}

func TestProducerFacts_EachSuiteProducerReachesCore(t *testing.T) {
	for _, producer := range []string{"review", "gap"} {
		for _, mixed := range []bool{false, true} {
			t.Run(producer+map[bool]string{false: "-known", true: "-mixed"}[mixed], func(t *testing.T) {
				body := "    def test_one(self): self.fail('[violation:actual] observed')\n"
				if mixed {
					body += "    def test_two(self): self.skipTest('not observed')\n"
				}
				f := producerFactsFixtureFor(t, body, "actual", false, producer)
				for _, baseline := range []bool{true, false} {
					entry := resultCheck(t, f, baseline)
					want := "fail"
					if baseline {
						want = "fail_as_expected"
					}
					if mixed {
						want = "unknown"
					}
					if entry["result"] != want {
						t.Fatalf("[requirement] suite producer/Core semantics: want %s: %v", want, entry)
					}
				}
			})
		}
	}
}

func TestProducerFacts_CurrentAndBaselinePreserveObservedMeaning(t *testing.T) {
	cases := []struct {
		name, body, expected, current, baseline, violation string
		command                                            bool
	}{
		{"known", "    def test_one(self): self.fail('[violation:actual] observed')\n", "actual", "fail", "fail_as_expected", "actual", false},
		{"typed", "    def test_one(self):\n        from result_observations import RequirementViolation\n        raise RequirementViolation('actual', 'observed')\n", "actual", "fail", "fail_as_expected", "actual", false},
		{"compared-data-tag", "    def test_one(self): self.assertEqual('[violation:actual]', 'different')\n", "actual", "error", "error", "unclassified-assertion", false},
		{"repr-prefix-tag", "    def test_one(self):\n        class Data:\n            def __repr__(self): return '[violation:actual] data'\n        self.assertEqual(Data(), object())\n", "actual", "error", "error", "unclassified-assertion", false},
		{"assertion-suffix-tag", "    def test_one(self): self.assertEqual(1, 2, '[violation:actual] suffix')\n", "actual", "error", "error", "unclassified-assertion", false},
		{"unexpected", "    def test_one(self): self.fail('[violation:other] different assertion')\n", "actual", "fail", "fail", "other", false},
		{"unclassified", "    def test_one(self): self.fail('precondition failed')\n", "unclassified-assertion", "error", "error", "unclassified-assertion", false},
		{"mixed-skip", "    def test_one(self): self.fail('[violation:actual] observed')\n    def test_two(self): self.skipTest('not observed')\n", "actual", "unknown", "unknown", "actual", false},
		{"mixed-error", "    def test_one(self): self.fail('[violation:actual] observed')\n    def test_two(self): raise RuntimeError('broken environment')\n", "actual", "error", "error", "actual", false},
		{"unverified", "raise SystemExit(2)\n", "actual", "unknown", "unknown", "", true},
		{"command-violation", "raise SystemExit(1)\n", "actual", "fail", "fail_as_expected", "actual", true},
		{"command-unknown-exit", "raise SystemExit(17)\n", "actual", "error", "error", "", true},
		{"command-pass", "raise SystemExit(0)\n", "actual", "pass", "unexpected_pass", "", true},
		{"pass", "    def test_one(self): pass\n", "actual", "pass", "unexpected_pass", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := producerFactsFixture(t, tc.body, tc.expected, tc.command)
			for _, baseline := range []bool{true, false} {
				entry := resultCheck(t, f, baseline)
				want := tc.current
				if baseline {
					want = tc.baseline
				}
				if entry["result"] != want {
					t.Fatalf("[requirement] actual producer result lost meaning: want %s: %v", want, entry)
				}
				evidence, ok := entry["evidence"].(map[string]any)
				if !ok {
					t.Fatalf("[requirement] no structured producer observations: %v", entry)
				}
				observations, ok := evidence["targets"].([]any)
				if !ok || len(observations) != 5 {
					t.Fatalf("[requirement] independent fact targets were lost: %v", evidence)
				}
				seen := false
				for _, value := range observations {
					item := value.(map[string]any)
					if item["attempt"] != float64(1) {
						t.Fatalf("[requirement] mixed facts became retries: %v", item)
					}
					seen = seen || item["violation"] == tc.violation
				}
				if tc.violation != "" && !seen {
					t.Fatalf("[requirement] observed assertion missing from stored evidence: %v", evidence)
				}
			}
		})
	}
}

func TestProducerFacts_AdverseAssertionSurvivesLaterSuccess(t *testing.T) {
	body := "    def test_one(self):\n        from pathlib import Path\n        if not Path('ignored/now-pass').exists(): self.fail('[violation:actual] observed')\n"
	f := producerFactsFixture(t, body, "actual", false)
	first := resultCheck(t, f, false)
	if first["result"] != "fail" {
		t.Fatalf("[requirement] first assertion was not observed: %v", first)
	}
	f.put("ignored/now-pass", "synthetic change outside code\n")
	second := resultCheck(t, f, false)
	if second["result"] != "pass" {
		t.Fatalf("expected independent later passing observation: %v", second)
	}
	criterion := criterionReport(t, f, "AC-1")
	encoded, _ := json.Marshal(criterion)
	if !strings.Contains(string(encoded), "criterion_flaky") {
		t.Fatalf("[requirement] later pass hid earlier adverse assertion: %s", encoded)
	}
}
