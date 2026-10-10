package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This reads product templates/examples, rather than restating a passing
// declaration in the test. The copied goal invokes the real reference process.
func TestTemplateFlowAC6_ExampleAndTemplateReachRecordedCompletion(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	readProduct := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			if os.IsNotExist(err) {
				t.Fatalf("[requirement] template flow input %s is missing", name)
			}
			t.Fatalf("read template flow input %s: %v", name, err)
		}
		return string(data)
	}
	example := readProduct("examples/empty-input/PLAN.md")
	template := readProduct("plugins/seal/skills/seal/assets/templates/PLAN.md")
	for _, text := range []string{example, template} {
		if !strings.Contains(text, "## 결과 계약") {
			t.Fatal("[requirement] a published plan lacks the structured result connection")
		}
	}
	files := map[string]string{}
	for _, name := range []string{"SPEC.md", "PLAN.md", "REVIEW.md", "PROGRESS.md", "declaration.json", "reference.json"} {
		files["examples/empty-input/"+name] = readProduct("examples/empty-input/" + name)
	}
	files["tools/check-result-reference.py"] = readProduct("tools/check-result-reference.py")
	for _, form := range []string{"published-example", "filled-template"} {
		t.Run(form, func(t *testing.T) {
			f := &preflightFixture{t: t, dir: t.TempDir()}
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
				t.Setenv(key, "synthetic")
			}
			for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
				t.Setenv(key, "synthetic@example.invalid")
			}
			for name, contents := range files {
				f.put(name, contents)
			}
			plan := example
			if form == "filled-template" {
				// Fill placeholders only. A malformed table header, missing row,
				// or incorrect link in the real template must reach the parser.
				plan = strings.NewReplacer(
					"<goal>", "empty-input",
					"<command that reports the declared AC-1 observations>", "python3 tools/check-result-reference.py examples/empty-input/declaration.json examples/empty-input/reference.json",
					"<declaration JSON>, <producer and configuration files>, <other check files>", "examples/empty-input/declaration.json, examples/empty-input/reference.json, tools/check-result-reference.py",
					"<command that checks AC-2>", "python3 -c \"assert __import__('pathlib').Path('src/textstats/sample-output.txt').read_text().strip() == '1 2 3'\"",
					"<repository-relative declaration JSON>", "examples/empty-input/declaration.json",
					"`<glob>`, `<glob>`", "`src/textstats/empty-output.txt`",
					"<section>", "목표", "<behavior this task completes>", "빈 입력 요약 파일에 0 0 0을 저장한다",
				).Replace(template)
				// This optional annotation is explicitly omitted when unused.
				var lines []string
				for _, line := range strings.Split(plan, "\n") {
					if !strings.HasPrefix(line, "- <only when a check reads other goals'") {
						lines = append(lines, line)
					}
				}
				plan = strings.Join(lines, "\n")
			}
			f.put(scenarioGoal+"/SPEC.md", files["examples/empty-input/SPEC.md"])
			f.put(scenarioGoal+"/PLAN.md", plan)
			f.put(scenarioGoal+"/REVIEW.md", files["examples/empty-input/REVIEW.md"])
			f.put(scenarioGoal+"/PROGRESS.md", files["examples/empty-input/PROGRESS.md"])
			f.put("src/textstats/empty-output.txt", "incorrect empty summary\n")
			f.put("src/textstats/sample-output.txt", "1 2 3\n")
			f.git("init", "-q", "-b", "main")
			f.git("add", "-A")
			f.git("commit", "-qm", "synthetic template baseline")
			t.Chdir(f.dir)
			for _, args := range [][]string{{"lint", scenarioGoal}, {"start", scenarioGoal, "--request", "synthetic template flow"}, {"check", scenarioGoal, "--baseline", "AC-1"}} {
				if code, out := f.ha(args...); code != 0 {
					t.Fatalf("template flow preparation/execution %v failed: %d %s", args, code, out)
				}
			}
			before := fixtureRecords(t, f)
			if last := before[len(before)-1]; last["result"] != "fail_as_expected" || last["evidence"] == nil {
				t.Fatalf("[requirement] missing structured baseline proof: %v", last)
			}
			f.put("src/textstats/empty-output.txt", "0 0 0\n")
			f.git("add", "src")
			f.git("commit", "-qm", "synthetic empty summary fix")
			if code, out := f.ha("check", scenarioGoal); code != 0 {
				t.Fatalf("template check execution: %d %s", code, out)
			}
			f.put(scenarioGoal+"/PROGRESS.md", "## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | synthetic |\n")
			code, out := f.ha("done", scenarioGoal, "--format", "json")
			var report map[string]any
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatalf("completion report transport: %v: %s", err, out)
			}
			if code != 0 || report["completion_record"] == nil || report["assurance"] != "local" {
				t.Fatalf("template flow completion failed: %d %s", code, out)
			}
			lines := fixtureRecords(t, f)
			if lines[len(lines)-1]["kind"] != "done" || lines[len(lines)-1]["status"] != "complete" {
				t.Fatalf("[requirement] completion not durable: %v", lines[len(lines)-1])
			}
		})
	}
}
