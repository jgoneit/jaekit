package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/run"
	"github.com/jgoneit/jaekit/internal/status"
)

const processInjectionNotice = "synthetic unconfirmed result after actual native executor completed"

// This helper is compiled only into the Go test executable. A child process
// invokes the real command handlers and returns their actual CLI exit code.
// Neither mode nor shortened timeout is available from a product ha binary.
func TestProcessScenarioDriver(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	args := os.Args[separator+1:]
	if len(args) < 2 {
		os.Exit(64)
	}
	mode, args := args[0], args[1:]
	if mode == "native" {
		os.Exit(runMain(args, os.Stdout, os.Stderr))
	}
	if args[0] != "check" || (mode != "timeout" && mode != "unconfirmed") {
		os.Exit(64)
	}
	c := &cli{out: os.Stdout, err: os.Stderr}
	c.execute = func(dir string, argv []string, timeout time.Duration, log string, env []string) (run.Result, error) {
		if timeout != checkTimeout {
			return run.Result{}, errors.New("product default timeout changed")
		}
		if mode == "timeout" {
			// Only the timer interval changes. The native starter, containment,
			// termination, output finalization and CLI recording are unmodified.
			return run.ExecWithEnv(dir, argv, 3*time.Second, log, env)
		}
		res, err := run.ExecWithEnv(dir, argv, timeout, log, env)
		if err != nil || !res.Attempted || res.Outcome != run.Pass || res.Exit == nil || *res.Exit != 0 || res.Digest == "" {
			return res, fmt.Errorf("native fixture did not complete before injection: %w", err)
		}
		// This is a command-boundary fault, not a Windows API fault. Direct
		// internal/run tests independently cover unconfirmed process shutdown.
		fmt.Fprintln(os.Stderr, processInjectionNotice)
		res.Outcome, res.Reason, res.Exit, res.Digest = run.Error, "process_wait", nil, ""
		return res, errors.New("synthetic output finalization is unconfirmed")
	}
	os.Exit(c.check(args[1:]))
}

type processScenarioCase struct {
	ID       string `json:"id"`
	Mode     string `json:"mode"`
	Rules    string `json:"rules"`
	Baseline bool   `json:"baseline"`
	Code     int    `json:"code"`
	Result   string `json:"result"`
	Exit     *int   `json:"exit"`
}

type processScenario struct {
	*preflightFixture
	driver string
	gitDir string
}

func TestNativeProcessTimeoutScenarios(t *testing.T) {
	runProcessScenarios(t, "timeout")
}

func TestNativeProcessUnconfirmedShutdownScenarios(t *testing.T) {
	runProcessScenarios(t, "unconfirmed")
}

func runProcessScenarios(t *testing.T, group string) {
	t.Helper()
	if checkTimeout != 900*time.Second {
		t.Fatal("product default timeout changed")
	}
	fixture, err := filepath.Abs(filepath.Join(scenarioRoot, "_native", "windows-process-control"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(fixture, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string][]processScenarioCase
	if err := json.Unmarshal(raw, &groups); err != nil || len(groups[group]) == 0 {
		t.Fatalf("missing process scenario group %q: %v", group, err)
	}
	producer := filepath.Join(t.TempDir(), "process producer")
	if runtime.GOOS == "windows" {
		producer += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", producer, filepath.Join(fixture, "producer.go")).CombinedOutput(); err != nil {
		t.Fatalf("build actual scenario checker: %v: %s", err, out)
	}
	for _, item := range groups[group] {
		t.Run(item.ID, func(t *testing.T) {
			s := newProcessScenario(t, fixture, producer, item)
			before := s.read(scenarioGoal + "/runs.jsonl")
			beforeStatus := s.report(t)
			args := []string{"check", scenarioGoal, "AC-1"}
			if item.Baseline {
				args = append(args, "--baseline")
			}
			mode := group
			if group == "parent" {
				mode = "native"
			}
			started := time.Now()
			code, out := s.command(t, mode, args...)
			if code != item.Code {
				t.Fatalf("ha %v: exit %d, want %d: %s", args, code, item.Code, out)
			}
			if group == "unconfirmed" && !strings.Contains(out, processInjectionNotice) {
				t.Fatal("fault was not injected after an actual completed native process")
			}
			if data, err := os.ReadFile(s.marker + ".producer"); err != nil || string(data) != "producer executed\n" {
				t.Fatalf("checker did not actually execute: %q: %v", data, err)
			}
			if item.Rules != status.RulesV3 {
				if !bytes.Equal(before, s.read(scenarioGoal+"/runs.jsonl")) || len(s.logs()) != 0 {
					t.Fatal("legacy internal error added a record or retained a finalized log")
				}
				afterStatus := s.report(t)
				if beforeStatus.RecordHead == nil || afterStatus.RecordHead == nil || *beforeStatus.RecordHead != *afterStatus.RecordHead || afterStatus.Criteria[0].Attempts.Checks != 0 || afterStatus.Criteria[0].Attempts.Baselines != 0 {
					t.Fatal("legacy internal error changed record head or attempted-check counters")
				}
				return
			}
			row := s.assertRecord(t, item, before, beforeStatus)
			if group == "unconfirmed" {
				if row.OutputDigest != "" || row.Evidence.Reason != "process_output" {
					t.Fatalf("unconfirmed output was finalized or misclassified: %+v", row)
				}
			} else {
				if group == "timeout" && row.Evidence.Reason != "process_timeout" {
					t.Fatalf("timeout classification: %+v", row.Evidence)
				}
				// The child fallback is 15 seconds. A result that waited for that
				// fallback cannot count as observed native containment/termination.
				if time.Since(started) >= 10*time.Second {
					t.Fatal("command exceeded the cleanup bound before descendant fallback")
				}
				s.assertStableOutput(t, row)
			}
			if item.ID == "parent-pass-check" {
				s.assertUsableParentEvidence(t)
			}
			if item.Result == "error" {
				previous := s.read(scenarioGoal + "/runs.jsonl")
				if code, _ := s.command(t, "native", "done", scenarioGoal); code == 0 {
					t.Fatal("process error became completion evidence")
				}
				after := s.read(scenarioGoal + "/runs.jsonl")
				rows, bad, err := record.Parse(after)
				if err != nil || bad != nil || len(rows) != 3 || !bytes.HasPrefix(after, previous) || rows[2].Kind != "done" || rows[2].Status == "complete" {
					t.Fatal("completion assessment changed prior records or recorded a false completion")
				}
				if rep := s.report(t); rep.Budget.Runs != 1 || rep.CompletionRecord != nil {
					t.Fatal("unsuccessful completion assessment changed usage or supplied completion evidence")
				}
			}
		})
	}
}

func newProcessScenario(t *testing.T, fixture, producer string, item processScenarioCase) *processScenario {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	driver, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := &processScenario{preflightFixture: &preflightFixture{t: t, dir: filepath.Join(t.TempDir(), "프로젝트 space"), marker: filepath.Join(t.TempDir(), "heartbeat")}, driver: driver}
	copyTree(t, base, s.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	s.put(".gitattributes", "* -text\n")
	data, err := os.ReadFile(filepath.Join(fixture, "producer.go"))
	if err != nil {
		t.Fatal(err)
	}
	s.put("checks/producer.go", string(data))
	s.put("checks/declaration.json", `{"schema":"check-declaration/v1","producer":"synthetic-process","producer_version":"1","producer_paths":["checks/producer.go"],"targets":[{"id":"greeting","violation":"missing-greeting"}]}`)
	command := fmt.Sprintf("'%s' %s '%s'", filepath.ToSlash(producer), item.Mode, filepath.ToSlash(s.marker))
	plan := string(s.read(scenarioGoal + "/PLAN.md"))
	plan = strings.Replace(plan, "`sh tests/greeting.sh` | tests/greeting.sh", "`"+command+"` | checks/producer.go, checks/declaration.json", 1)
	plan = strings.Replace(plan, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`", 1)
	plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
	s.put(scenarioGoal+"/PLAN.md", plan)
	s.git("init", "-q", "-b", "main")
	s.git("add", "-A")
	s.git("commit", "-qm", "synthetic process-control base")
	s.gitDir = s.git("rev-parse", "--absolute-git-dir")
	if code, out := s.command(t, "native", "start", scenarioGoal, "--request", "synthetic process-control scenario"); code != 0 {
		t.Fatalf("start: exit %d: %s", code, out)
	}
	if item.Rules != status.RulesV3 {
		// A standalone synthetic start represents a persisted supported legacy
		// goal. No existing user record, linked record chain, or source is used.
		s.rules(item.Rules)
	}
	if item.ID == "parent-pass-check" {
		s.put("src/greeting.txt", "hello\n")
		s.git("add", "src/greeting.txt")
		s.git("commit", "-qm", "synthetic greeting implementation")
	}
	return s
}

func (s *processScenario) command(t *testing.T, mode string, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	argv := append([]string{"-test.run=^TestProcessScenarioDriver$", "--", mode}, args...)
	cmd := exec.CommandContext(ctx, s.driver, argv...)
	cmd.Dir = s.dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("command driver deadline: %v: %s", args, out)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("command driver did not start: %v", err)
	}
	return 0, string(out)
}

func (s *processScenario) report(t *testing.T) status.Report {
	t.Helper()
	code, out := s.command(t, "native", "status", scenarioGoal, "--format", "json")
	var rep status.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil || rep.ExitCode != code || len(rep.Criteria) != 2 {
		t.Fatalf("status was not a valid CLI report: exit %d, %v: %s", code, err, out)
	}
	for _, reason := range rep.Reasons {
		if reason.Code == "record_invalid" || reason.Code == "record_integrity" || reason.Code == "evidence_invalid" {
			t.Fatalf("recorded process result is invalid: %+v", reason)
		}
	}
	return rep
}

func (s *processScenario) assertRecord(t *testing.T, item processScenarioCase, before []byte, beforeStatus status.Report) record.Line {
	t.Helper()
	data := s.read(scenarioGoal + "/runs.jsonl")
	rows, bad, err := record.Parse(data)
	if err != nil || bad != nil || len(rows) != 2 || !bytes.HasPrefix(data, before) {
		t.Fatalf("attempt lost or record chain changed: rows=%d integrity=%v error=%v", len(rows), bad, err)
	}
	row := rows[1]
	kind := "check"
	if item.Baseline {
		kind = "baseline"
	}
	if row.Kind != kind || row.Result != item.Result || row.Target == nil || row.Target.Criterion != "AC-1" || row.Evidence == nil || row.Evidence.Result != item.Result {
		t.Fatalf("CLI process classification was not recorded: %+v", row)
	}
	if reason := map[string]string{"pass": "targets_passed", "fail": "assertion_failed", "fail_as_expected": "declared_violation", "unexpected_pass": "baseline_passed"}[item.Result]; reason != "" && row.Evidence.Reason != reason {
		t.Fatalf("original producer result meaning changed: %+v", row.Evidence)
	}
	if (item.Exit == nil) != (row.Exit == nil) || (item.Exit != nil && *item.Exit != *row.Exit) {
		t.Fatalf("checker exit differs: got %v want %v", row.Exit, item.Exit)
	}
	if !strings.HasPrefix(row.OutputPath, ".git/ha/") {
		t.Fatalf("raw output storage was not connected to the record: %+v", row)
	}
	log := s.readLog(t, row)
	if !bytes.Contains(log, []byte("synthetic process-control producer")) {
		t.Fatal("recorded output lacks the actual producer output")
	}
	rep := s.report(t)
	if beforeStatus.Budget == nil || rep.Budget == nil || rep.Budget.Runs != beforeStatus.Budget.Runs+1 || rep.RecordHead == nil || rep.RecordHead.Seq != row.Seq || rep.RecordHead.SHA256 != row.Hash() {
		t.Fatal("attempt usage or record head was not preserved in status")
	}
	attempts := rep.Criteria[0].Attempts
	checks, baselines := 1, 0
	if item.Baseline {
		checks, baselines = 0, 1
	}
	if attempts.Checks != checks || attempts.Baselines != baselines {
		t.Fatalf("status lost an actual attempt: %+v", attempts)
	}
	if item.Result == "error" && (rep.Criteria[0].Satisfied || attempts.Errors+attempts.BaselineErrors != 1 || rep.Status == "complete") {
		t.Fatalf("process error was promoted or not counted: %+v", rep.Criteria[0])
	}
	return row
}

func (s *processScenario) assertUsableParentEvidence(t *testing.T) {
	t.Helper()
	// The same committed checker now sees the original source in the baseline
	// worktree. A pass/error-shaped record alone cannot substitute for usable
	// current-pass plus declared-baseline-violation evidence in status.
	if err := os.Remove(s.marker); err != nil {
		t.Fatal(err)
	}
	if code, out := s.command(t, "native", "check", scenarioGoal, "AC-1", "--baseline"); code != 0 {
		t.Fatalf("parent baseline violation: exit %d: %s", code, out)
	}
	rows, bad, err := record.Parse(s.read(scenarioGoal + "/runs.jsonl"))
	if err != nil || bad != nil || len(rows) != 3 || rows[2].Result != "fail_as_expected" || rows[2].Evidence == nil || rows[2].Evidence.Reason != "declared_violation" || rows[2].Exit == nil || *rows[2].Exit != 3 {
		t.Fatalf("parent baseline was not usable expected-violation evidence: %v, %v", bad, err)
	}
	s.assertStableOutput(t, rows[2])
	rep := s.report(t)
	criterion := rep.Criteria[0]
	if !criterion.Satisfied || criterion.Reason != "" || len(criterion.Records) != 2 || criterion.Records[0] != 2 || criterion.Records[1] != 3 || criterion.Attempts.Checks != 1 || criterion.Attempts.Baselines != 1 || rep.Budget == nil || rep.Budget.Runs != 2 {
		t.Fatalf("cleaned-up parent results were not usable in status: %+v", criterion)
	}
}

func (s *processScenario) readLog(t *testing.T, row record.Line) []byte {
	t.Helper()
	path := filepath.Join(s.gitDir, filepath.FromSlash(strings.TrimPrefix(row.OutputPath, ".git/")))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (s *processScenario) assertStableOutput(t *testing.T, row record.Line) {
	t.Helper()
	beforeLog := s.readLog(t, row)
	beforeMarker, err := os.ReadFile(s.marker)
	if err != nil || len(beforeMarker) == 0 || !bytes.Contains(beforeLog, []byte("descendant stdout")) || !bytes.Contains(beforeLog, []byte("descendant stderr")) {
		t.Fatalf("ordinary descendant never wrote its marker and inherited log: %v", err)
	}
	digest := sha256.Sum256(beforeLog)
	if row.OutputDigest != hex.EncodeToString(digest[:]) {
		t.Fatal("final log bytes do not match the recorded output digest")
	}
	time.Sleep(200 * time.Millisecond)
	afterMarker, err := os.ReadFile(s.marker)
	if err != nil || !bytes.Equal(beforeMarker, afterMarker) || !bytes.Equal(beforeLog, s.readLog(t, row)) {
		t.Fatal("descendant continued to change output after the command returned")
	}
}
