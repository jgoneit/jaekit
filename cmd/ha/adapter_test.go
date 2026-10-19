package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adapterFixture is a synthetic goal whose AC-1 runs command through an adapter
// declaration. The files in base are committed before the start; the linked
// test and the declaration's final violation come after it, as a user adds them.
func adapterFixture(t *testing.T, command, checks string, base map[string]string) *preflightFixture {
	t.Helper()
	root, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	f := &preflightFixture{t: t, dir: t.TempDir()}
	copyTree(t, root, f.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	for name, text := range base {
		f.put(name, text)
	}
	plan := string(f.read(scenarioGoal + "/PLAN.md"))
	plan = strings.Replace(plan, "`sh tests/greeting.sh` | tests/greeting.sh", "`"+command+"` | "+checks+", checks/AC-1.json", 1)
	plan = strings.Replace(plan, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`, `greet/**`", 1)
	plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/AC-1.json |\n"
	f.put(scenarioGoal+"/PLAN.md", plan)
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "synthetic base")
	t.Chdir(f.dir)
	if code, out := f.ha("start", scenarioGoal, "--request", "synthetic verification", "--rules", "run-rules/4"); code != 0 {
		t.Fatalf("start: exit %d: %s", code, out)
	}
	return f
}

const greetTest = `package greet

import "testing"

func TestGreets(t *testing.T) {
	t.Log("jaekit:violation=greeting-missing is only logged")
	if got := Hello(); got != "hello" {
		t.Fatalf("jaekit:violation=greeting-missing the greeting is %q", got)
	}
}

func TestUnlinked(t *testing.T) {}
`

// TestAdapterGoTestJSONFlow runs a real nested go test -json through ha: the
// baseline observes the declared violation on the base commit, the check
// passes after the change, and the record keeps the plan's command.
func TestAdapterGoTestJSONFlow(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	declaration := func(violation string) string {
		return `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "greeting", "violation": "` + violation + `", "test": {"package": "example.com/demo/greet", "name": "TestGreets"}}]}`
	}
	f := adapterFixture(t, "go test -json -count=1 ./greet/", "greet/greet_test.go", map[string]string{
		"go.mod":           "module example.com/demo\n\ngo 1.27\n",
		"greet/greet.go":   "package greet\n\nfunc Hello() string { return \"hi\" }\n",
		"checks/AC-1.json": declaration("greeting-draft"),
	})
	f.put("greet/greet_test.go", greetTest)
	f.put("checks/AC-1.json", declaration("greeting-missing"))
	f.git("add", "-A")
	f.git("commit", "-qm", "link the greeting test")
	base := requireResult(t, resultCheck(t, f, true), "fail_as_expected")
	if base["reason"] != "declared_violation" || base["report_digest"] == "" {
		t.Fatalf("baseline evidence: %v", base)
	}
	f.put("greet/greet.go", "package greet\n\nfunc Hello() string { return \"hello\" }\n")
	f.git("add", "-A")
	f.git("commit", "-qm", "say hello")
	last := resultCheck(t, f, false)
	if proof := requireResult(t, last, "pass"); proof["reason"] != "targets_passed" {
		t.Fatalf("check evidence: %v", proof)
	}
	argv, _ := last["argv"].([]any)
	if len(argv) != 5 || argv[0] != "go" || argv[2] != "-json" {
		t.Fatalf("recorded argv is not the plan's command: %v", last["argv"])
	}
	if criterionReport(t, f, "AC-1")["satisfied"] != true {
		t.Fatal("AC-1 is not satisfied after a passing adapter check")
	}
}

// TestAdapterJUnitFlow runs a separate runner that writes JUnit XML where ha
// names it, through the placeholder or the variable alone.
func TestAdapterJUnitFlow(t *testing.T) {
	runner := `# Write to the argument, or to HA_JUNIT_XML when there is none.
out="${1:-$HA_JUNIT_XML}"
if grep -qx hello src/greeting.txt; then
	printf '<testsuites><testcase classname="tests.greeting" name="says hello"/></testsuites>\n' > "$out"
	exit 0
fi
printf '<testsuites><testcase classname="tests.greeting" name="says hello"><failure message="jaekit:violation=greeting-missing hi">x</failure></testcase></testsuites>\n' > "$out"
exit 1
`
	declaration := `{"schema": "check-adapter/v1", "adapter": "junit-xml", "targets": [{"id": "greeting", "violation": "greeting-missing", "test": {"classname": "tests.greeting", "name": "says hello"}}]}`
	for name, command := range map[string]string{"placeholder": "sh tests/junit.sh {HA_JUNIT_XML}", "variable": "sh tests/junit.sh"} {
		t.Run(name, func(t *testing.T) {
			f := adapterFixture(t, command, "tests/junit.sh", map[string]string{"tests/junit.sh": runner, "checks/AC-1.json": declaration})
			requireResult(t, resultCheck(t, f, true), "fail_as_expected")
			f.put("src/greeting.txt", "hello\n")
			f.git("add", "-A")
			f.git("commit", "-qm", "say hello")
			requireResult(t, resultCheck(t, f, false), "pass")
			if entries, _ := os.ReadDir(f.dir); len(entries) == 0 {
				t.Fatal("fixture vanished")
			}
			if untracked := f.git("ls-files", "--others", "--exclude-standard"); untracked != "" {
				t.Fatalf("the runner wrote into the working tree: %s", untracked)
			}
		})
	}
}
