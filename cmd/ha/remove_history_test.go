package main

// Checks that the decision-history feature is gone from ha (goal
// remove-decision-history, AC-1, AC-3, AC-4). A violation starts with
// jaekit:violation=<id>; a setup that cannot run fails without one.
import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRemoveHistoryAC1_HistoryCommandUnknown: every history form is an
// unknown command (exit 64, empty stdout, the unknown-command line), the
// repository, including a stored history file, is unchanged, and the help
// does not list the command.
func TestRemoveHistoryAC1_HistoryCommandUnknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("setup: git init: %v: %s", err, out)
	}
	stored := filepath.Join(dir, ".git", "ha", "history", strings.Repeat("a", 64), "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(stored), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stored, []byte("{\"kind\":\"decision\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := treeDigest(t, dir)
	var problems []string
	for _, args := range [][]string{{"history"}, {"history", "--help"}, {"history", "docs/specs/g"},
		{"history", "docs/specs/g", "add", "--kind", "결정", "--summary", "x"}, {"history", "docs/specs/g", "show", "D1"}} {
		code, out, stderr := haProcess(t, dir, os.Environ(), args...)
		if code < 0 {
			t.Fatalf("setup: ha %v ended by a signal", args)
		}
		first, _, _ := strings.Cut(stderr, "\n")
		if code != 64 || out != "" || first != `ha: unknown command "history"; run ha --help` {
			problems = append(problems, fmt.Sprintf("ha %s: exit %d, stdout %q, stderr %q", strings.Join(args, " "), code, out, first))
		}
	}
	if treeDigest(t, dir) != before {
		problems = append(problems, "the repository changed")
	}
	code, out, stderr := haProcess(t, dir, os.Environ(), "--help")
	if code != 0 {
		t.Fatalf("setup: ha --help: exit %d: %s", code, stderr)
	}
	if strings.Contains(out+stderr, "ha history") {
		problems = append(problems, "ha --help lists ha history")
	}
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=history-command-present: %s", strings.Join(problems, "\n"))
	}
}

// treeDigest hashes every path, mode and content under dir.
func treeDigest(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %v\n", path, info.Mode())
		if d.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: read %s: %v", dir, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TestRemoveHistoryAC2_StoredHistoryKept (maintain): beside a goal carried to
// completion, stored history files change neither the output of ha status,
// report and doctor nor themselves, and ha done still records completion.
// (Each ha done appends a completion record, so its output is not compared.)
func TestRemoveHistoryAC2_StoredHistoryKept(t *testing.T) {
	f := reportFixture(t)
	f.must(0, "start", scenarioGoal, "--request", "synthetic stored history")
	f.must(0, "check", scenarioGoal, "AC-1", "--baseline")
	f.put("src/greeting.txt", "hello\n")
	f.git("add", "src/greeting.txt")
	f.git("commit", "-qm", "synthetic change")
	f.must(0, "check", scenarioGoal, "AC-1", "AC-2")
	f.put(scenarioGoal+"/PROGRESS.md", strings.Replace(string(f.read(scenarioGoal+"/PROGRESS.md")), "| todo |", "| done |", 1))
	f.must(0, "done", scenarioGoal)
	outputs := func() string {
		var b strings.Builder
		for _, args := range [][]string{{"status", scenarioGoal}, {"report", scenarioGoal, "--lang", "en"}, {"doctor", scenarioGoal}} {
			code, out := f.ha(args...)
			fmt.Fprintf(&b, "%v exit %d\n%s\n", args, code, out)
		}
		return b.String()
	}
	without := outputs()
	stored := ".git/ha/history/" + strings.Repeat("b", 64)
	f.put(stored+"/events.jsonl", "{\"kind\":\"decision\"}\n")
	f.put(stored+"/last", "1\n")
	history := filepath.Join(f.dir, ".git", "ha", "history")
	before := treeDigest(t, history)
	if with := outputs(); with != without {
		t.Errorf("outputs differ with stored history:\nwithout:\n%s\nwith:\n%s", without, with)
	}
	f.must(0, "done", scenarioGoal)
	if treeDigest(t, history) != before {
		t.Error("the stored history changed")
	}
}

// TestRemoveHistoryAC3_NoHistoryCapability: ha capabilities announces neither
// the decision-history format nor its feature, and a skill that requires
// either is refused at start with exit 65 and no record. Starting with
// Seal's own requirements stays with the existing start tests.
func TestRemoveHistoryAC3_NoHistoryCapability(t *testing.T) {
	code, out, stderr := haProcess(t, t.TempDir(), os.Environ(), "capabilities", "--format", "json")
	if code != 0 {
		t.Fatalf("setup: ha capabilities: exit %d: %s", code, stderr)
	}
	var caps struct {
		Formats  map[string]any `json:"formats"`
		Features []string       `json:"features"`
	}
	if err := json.Unmarshal([]byte(out), &caps); err != nil {
		t.Fatalf("setup: capabilities JSON: %v", err)
	}
	var problems []string
	if _, ok := caps.Formats["decision_history"]; ok {
		problems = append(problems, "formats has decision_history")
	}
	if slices.Contains(caps.Features, "decision-history/v1") {
		problems = append(problems, "features has decision-history/v1")
	}
	root, _, _ := startBudgetFixture(t, 1, "60")
	for _, declaration := range []string{`{"schema": "core-requirements/v1", "features": ["decision-history/v1"]}`,
		`{"schema": "core-requirements/v1", "formats": {"decision_history": ["decision-history/v1"]}}`} {
		code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", writeSkill(t, declaration), "--check-skill")
		if code != 65 || !strings.Contains(text, "decision") || startBudgetRecords(t, root) != nil {
			problems = append(problems, fmt.Sprintf("start with %s: exit %d: %s", declaration, code, text))
		}
	}
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=history-capability-declared: %s", strings.Join(problems, "\n"))
	}
}

// TestRemoveHistoryAC4_CheckCoreRefusesHistory: Seal's check_core.py no
// longer offers the history operation; it answers with its usage error
// (exit 64, "invalid choice: 'history'"). The other operations keep their
// results (TestCompatibilityAC8_ConsumerAndActualBinary).
func TestRemoveHistoryAC4_CheckCoreRefusesHistory(t *testing.T) {
	t.Parallel()
	helper, err := filepath.Abs("../../plugins/seal/skills/seal/scripts/check_core.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Fatalf("setup: %v", err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("setup: the check needs python3")
	}
	cmd := exec.Command(python, "-I", helper, "--ha", sharedBinary(t, "./cmd/ha"), "--operation", "history", "--goal", "docs/specs/g")
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) || !cmd.ProcessState.Exited() {
		t.Fatalf("setup: python3 did not run to an exit: %v", err)
	}
	if code := cmd.ProcessState.ExitCode(); code != 64 || !strings.Contains(string(out), "invalid choice: 'history'") {
		t.Errorf("jaekit:violation=history-operation-supported: check_core.py --operation history: exit %d: %s", code, out)
	}
}
