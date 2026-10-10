package main

// These acceptance checks use the existing CLI boundary and a real child
// process. They also compile on the previous implementation when overlaid for
// a baseline; an absent feature fails an assertion, not test collection.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const resultProducer = `import json, os, sys
config=json.load(open(sys.argv[2]))
mode=config.get("mode", "pass")
if mode == "plain": sys.exit(1)
if mode == "signal":
 import signal
 os.kill(os.getpid(), signal.SIGTERM)
if mode == "missing": sys.exit(1)
report={"schema":"check-result/v1", "invocation":os.environ.get("HA_EVIDENCE_INVOCATION", "legacy"),
 "declaration_digest":os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", "legacy"),
 "producer":"jaekit-reference", "producer_version":"1", "attempts_complete":True,
 "observations":[{"target":"greeting","attempt":1,"status":"pass"}, {"target":"secondary","attempt":1,"status":"pass"}]}
code=0
if mode in ("violation", "wrong", "mixed", "mixed-assertion", "conflict"):
 report["observations"][0]={"target":"greeting","attempt":1,"status":"violation","violation":"missing-greeting" if mode != "wrong" else "different"}
 code=1
if mode in ("environment", "mixed"):
 report["observations"][1]={"target":"secondary","attempt":1,"status":"error","reason":"browser_start_error"}
 code=1
if mode == "mixed-assertion":
 report["observations"][1]={"target":"secondary","attempt":1,"status":"violation","violation":"different"}
if mode == "skip":
 for item in report["observations"]: item.update(status="skip",reason="skipped")
if mode == "empty": report["observations"]=[]
if mode == "wrong-target": report["observations"][0]["target"]="someone-else"
if mode == "nonce": report["invocation"]="previous-invocation"
if mode == "retry": report["observations"][0]["attempt"]=2
if mode == "hidden-retry": report["attempts_complete"]=False
if mode == "conflict": code=0
if mode == "secret": report["debug"]="SYNTHETIC_SECRET_SHOULD_STAY_PRIVATE"
path=os.environ.get("HA_EVIDENCE_PATH")
if path:
 with open(path,"w") as out:
  if mode == "malformed": out.write("{")
  else: json.dump(report,out)
 if mode == "lost-output":
  import pathlib
  for output in pathlib.Path(path).parent.parent.glob(".pending-*.log"): output.unlink()
sys.exit(code)
`

func resultFixture(t *testing.T, mode string) *preflightFixture {
	t.Helper()
	producer, err := os.ReadFile("../../tools/check-result-reference.py")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "reference" {
		producer = []byte(resultProducer)
	}
	f := newPreflightFixture(t)
	f.put("checks/reference.py", string(producer))
	f.put("checks/declaration.json", `{"schema":"check-declaration/v1","producer":"jaekit-reference","producer_version":"1","producer_paths":["checks/reference.py","checks/reference.json"],"targets":[{"id":"greeting","violation":"missing-greeting"},{"id":"secondary","violation":"missing-secondary"}]}`)
	config := map[string]any{"mode": mode, "checks": []any{
		map[string]any{"target": "greeting", "path": "src/greeting.txt", "equals": "hello\n", "violation": "missing-greeting", "missing": "violation"},
		map[string]any{"target": "secondary", "path": "src/greeting.txt", "equals": "hello\n", "violation": "missing-secondary", "missing": "violation"},
	}}
	data, _ := json.Marshal(config)
	f.put("checks/reference.json", string(data))
	plan := string(f.read(scenarioGoal + "/PLAN.md"))
	plan = strings.Replace(plan, "`sh tests/greeting.sh` | tests/greeting.sh", "`python3 checks/reference.py checks/declaration.json checks/reference.json` | checks/reference.py, checks/reference.json, checks/declaration.json", 1)
	plan = strings.Replace(plan, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`", 1)
	plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
	f.put(scenarioGoal+"/PLAN.md", plan)
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic result integration")
	return f
}

func resultCheck(t *testing.T, f *preflightFixture, baseline bool) map[string]any {
	t.Helper()
	args := []string{"check", scenarioGoal, "AC-1"}
	if baseline {
		args = append(args, "--baseline")
	}
	code, output := f.ha(args...)
	if code != 0 && code != 1 {
		t.Fatalf("check failed before a classification: exit %d: %s", code, output)
	}
	ls := fixtureRecords(t, f)
	last := ls[len(ls)-1]
	if last["kind"] != "check" && last["kind"] != "baseline" {
		t.Fatalf("no check was recorded: %v", last)
	}
	return last
}

func fixtureRecords(t *testing.T, f *preflightFixture) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, b := range bytes.Split(bytes.TrimSpace(f.read(scenarioGoal+"/runs.jsonl")), []byte("\n")) {
		var line map[string]any
		if err := json.Unmarshal(b, &line); err != nil {
			t.Fatal(err)
		}
		records = append(records, line)
	}
	return records
}

// Rewrite only synthetic fixture records, preserving a valid hash chain.
func putFixtureRecords(t *testing.T, f *preflightFixture, records []map[string]any) {
	t.Helper()
	var all []byte
	var prev any
	for i, line := range records {
		line["seq"], line["prev"] = i+1, prev
		b, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		prev = hex.EncodeToString(h[:])
		all = append(append(all, b...), '\n')
	}
	f.put(scenarioGoal+"/runs.jsonl", string(all))
}

func requireResult(t *testing.T, line map[string]any, wanted string) map[string]any {
	t.Helper()
	if line["result"] != wanted {
		t.Fatalf("result = %v, want %s; record %v", line["result"], wanted, line)
	}
	proof, ok := line["evidence"].(map[string]any)
	if !ok || proof["reason"] == "" || proof["result"] != wanted {
		t.Fatalf("missing bound classification evidence: %v", line)
	}
	return proof
}

func criterionReport(t *testing.T, f *preflightFixture, id string) map[string]any {
	t.Helper()
	for _, value := range f.report()["criteria"].([]any) {
		row := value.(map[string]any)
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("missing condition %s", id)
	return nil
}

func TestBaselineBudgetAC1_ReferenceFlow(t *testing.T) {
	f := resultFixture(t, "reference")
	before := requireResult(t, resultCheck(t, f, true), "fail_as_expected")
	if before["declaration_digest"] == "" || len(before["targets"].([]any)) != 2 {
		t.Fatalf("missing target/declaration identity: %v", before)
	}
	f.put("src/greeting.txt", "hello\n")
	f.git("add", "src/greeting.txt")
	f.git("commit", "-qm", "implement greeting")
	requireResult(t, resultCheck(t, f, false), "pass")
	if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
		t.Fatal(out)
	}
	f.put(scenarioGoal+"/PROGRESS.md", "## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | synthetic |\n")
	if code, out := f.ha("done", scenarioGoal, "--format", "json"); code != 0 || f.report()["completion_record"] == nil {
		t.Fatalf("could not complete the real flow: %d %s", code, out)
	}
	gitdir := f.git("rev-parse", "--absolute-git-dir")
	if err := os.RemoveAll(filepath.Join(gitdir, "ha", "runs")); err != nil {
		t.Fatal(err)
	}
	if f.report()["status"] != "complete" {
		t.Fatal("saved proof depended on private report/log availability")
	}
}

func TestBaselineBudgetAC2_Environment(t *testing.T) {
	for _, mode := range []string{"environment", "mixed", "signal"} {
		t.Run(mode, func(t *testing.T) {
			f := resultFixture(t, mode)
			requireResult(t, resultCheck(t, f, true), "error")
			if criterionReport(t, f, "AC-1")["satisfied"] == true {
				t.Fatal("environment failure satisfied change")
			}
		})
	}
	t.Run("missing feature is executable observation", TestBaselineBudgetAC1_ReferenceFlow)
}

func TestBaselineBudgetAC3_UnknownAndMixed(t *testing.T) {
	for _, mode := range []string{"plain", "missing", "skip", "empty", "wrong-target", "nonce", "retry", "hidden-retry", "conflict", "malformed", "secret"} {
		t.Run(mode, func(t *testing.T) {
			f := resultFixture(t, mode)
			requireResult(t, resultCheck(t, f, true), "unknown")
		})
	}
	for _, mode := range []string{"wrong", "mixed-assertion"} {
		t.Run(mode, func(t *testing.T) {
			f := resultFixture(t, mode)
			requireResult(t, resultCheck(t, f, true), "fail")
		})
	}
}

func TestBaselineBudgetAC4_Freshness(t *testing.T) {
	for _, change := range []string{"declaration", "producer", "configuration"} {
		t.Run(change, func(t *testing.T) {
			f := resultFixture(t, "reference")
			requireResult(t, resultCheck(t, f, true), "fail_as_expected")
			f.put("src/greeting.txt", "hello\n")
			f.git("add", "src/greeting.txt")
			f.git("commit", "-qm", "implement")
			requireResult(t, resultCheck(t, f, false), "pass")
			if criterionReport(t, f, "AC-1")["satisfied"] != true {
				t.Fatal("valid initial evidence missing")
			}
			p := "checks/reference.json"
			if change == "producer" {
				p = "checks/reference.py"
			}
			if change == "declaration" {
				p = "checks/declaration.json"
			}
			f.put(p, string(f.read(p))+"\n")
			f.git("add", p)
			f.git("commit", "-qm", "change evidence input bytes")
			if criterionReport(t, f, "AC-1")["satisfied"] == true {
				t.Fatal("changed evidence input reused prior results")
			}
		})
	}
}

func TestBaselineBudgetAC5_InvalidResultsAndRetries(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/3"} {
		t.Run(rules, func(t *testing.T) {
			f := newPreflightFixture(t)
			f.rules(rules)
			if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
				t.Fatal(out)
			}
			ls := fixtureRecords(t, f)
			ls[len(ls)-1]["result"] = "invented-success"
			putFixtureRecords(t, f, ls)
			if criterionReport(t, f, "AC-2")["satisfied"] == true {
				t.Fatal("unknown result was treated as a current pass")
			}
		})
	}
	t.Run("shared unknown error allowance", func(t *testing.T) {
		f := resultFixture(t, "missing")
		resultCheck(t, f, true)
		resultCheck(t, f, true)
		if !strings.Contains(string(mustJSON(f.report())), "error_limit") {
			t.Fatal("unknown attempts bypassed error cap")
		}
	})
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestBaselineBudgetAC6_Usage(t *testing.T) {
	f := resultFixture(t, "environment")
	resultCheck(t, f, true)
	if f.report()["budget"].(map[string]any)["runs"] != float64(1) {
		t.Fatal("environment attempt lost usage")
	}
	f.put("src/greeting.txt", "dirty")
	before := f.read(scenarioGoal + "/runs.jsonl")
	if code, _ := f.ha("check", scenarioGoal, "--baseline", "AC-1"); code != 65 {
		t.Fatalf("dirty refusal %d", code)
	}
	if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
		t.Fatal("preflight charged an unexecuted check")
	}
}

func TestBaselineBudgetAC7_CountsAndPrivacy(t *testing.T) {
	f := resultFixture(t, "secret")
	requireResult(t, resultCheck(t, f, true), "unknown")
	data := mustJSON(f.report())
	if bytes.Contains(data, []byte("SYNTHETIC_SECRET")) || bytes.Contains(f.read(scenarioGoal+"/runs.jsonl"), []byte("SYNTHETIC_SECRET")) {
		t.Fatal("raw report escaped into persisted/public summaries")
	}
	if !bytes.Contains(data, []byte("unknown")) || !bytes.Contains(data, []byte("stale")) {
		t.Fatalf("classification/freshness dimensions missing: %s", data)
	}
	if f.report()["assurance"] != "local" {
		t.Fatal("classification overstated assurance")
	}
}

func TestBaselineBudgetAC6_PartialBatch(t *testing.T) {
	f := resultFixture(t, "environment")
	f.put("checks/reference.py", "open('became-dirty.txt', 'w').write('dirty')\n"+string(f.read("checks/reference.py")))
	f.git("add", "checks/reference.py")
	f.git("commit", "-qm", "synthetic mid-batch change")
	code, out := f.ha("check", scenarioGoal, "AC-1", "AC-2")
	if code != 65 || !strings.Contains(out, "1/2 targets executed, 1 not run") {
		t.Fatalf("partial /3 batch: %d %s", code, out)
	}
	ls := fixtureRecords(t, f)
	if len(ls) != 2 || ls[1]["result"] != "error" {
		t.Fatalf("attempt was lost or unexecuted target charged: %v", ls)
	}
	if f.report()["budget"].(map[string]any)["runs"] != float64(1) {
		t.Fatal("partial /3 batch lost usage")
	}
}

func TestBaselineBudgetAC6_PreparationAndOutputStorage(t *testing.T) {
	t.Run("maintain output lost after a real successful process", func(t *testing.T) {
		f := newPreflightFixture(t)
		f.put("tests/keep.sh", "python3 tests/lose-output.py\n")
		f.put("tests/lose-output.py", "from pathlib import Path\nfor p in Path('.git/ha/runs').rglob('.pending-*.log'): p.unlink()\n")
		f.git("add", "tests")
		f.git("commit", "-qm", "synthetic output failure")
		code, out := f.ha("check", scenarioGoal, "AC-2")
		if code != 1 || !strings.Contains(out, "error") {
			t.Fatalf("output failure: %d %s", code, out)
		}
		ls := fixtureRecords(t, f)
		r := f.report()
		if len(ls) != 2 || ls[1]["result"] != "error" || ls[1]["exit"] != float64(0) || r["budget"].(map[string]any)["runs"] != float64(1) || strings.Contains(string(mustJSON(r)), "record_invalid") {
			t.Fatalf("maintain attempt lost or malformed: records %v; report %v", ls, r)
		}
	})
	t.Run("missing overlay was never attempted", func(t *testing.T) {
		f := resultFixture(t, "reference")
		plan := string(f.read(scenarioGoal + "/PLAN.md"))
		plan = strings.Replace(plan, "checks/reference.json, checks/declaration.json |", "checks/reference.json, checks/declaration.json, checks/missing.txt |", 1)
		f.put(scenarioGoal+"/PLAN.md", plan)
		before := f.read(scenarioGoal + "/runs.jsonl")
		code, out := f.ha("check", scenarioGoal, "--baseline", "AC-1")
		if code != 65 || !strings.Contains(out, "checks/missing.txt") {
			t.Fatalf("preparation refusal: %d %s", code, out)
		}
		if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
			t.Fatal("baseline preparation charged an unexecuted attempt")
		}
	})
	for _, baseline := range []bool{false, true} {
		t.Run("lost output baseline="+strconv.FormatBool(baseline), func(t *testing.T) {
			f := resultFixture(t, "lost-output")
			line := resultCheck(t, f, baseline)
			proof := requireResult(t, line, "error")
			if proof["reason"] != "process_output" || line["output_path"] != "" {
				t.Fatalf("lost output was not distinguished: %v", line)
			}
			r := f.report()
			if r["budget"].(map[string]any)["runs"] != float64(1) || strings.Contains(string(mustJSON(r)), "record_invalid") {
				t.Fatalf("real attempt lost or malformed: %v", r)
			}
		})
	}
}

func budgetRequest(t *testing.T, f *preflightFixture, from, to, window, revision int, extra ...string) (int, string) {
	t.Helper()
	args := []string{"budget", scenarioGoal, "--runs", strconv.Itoa(to), "--from", strconv.Itoa(from), "--window", strconv.Itoa(window), "--revision", strconv.Itoa(revision)}
	return f.ha(append(args, extra...)...)
}

func seedUsage(t *testing.T, f *preflightFixture, n int) {
	t.Helper()
	if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
		t.Fatal(out)
	}
	ls := fixtureRecords(t, f)
	pattern := mustJSON(ls[len(ls)-1])
	ls = ls[:1]
	for i := 0; i < n; i++ {
		var clone map[string]any
		json.Unmarshal(pattern, &clone)
		ls = append(ls, clone)
	}
	putFixtureRecords(t, f, ls)
}

func TestBaselineBudgetAC8_TotalLimit(t *testing.T) {
	for _, limit := range []int{100, 40, 48} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			f := newPreflightFixture(t)
			seedUsage(t, f, 48)
			if code, out := budgetRequest(t, f, 50, limit, 1, 1, "--quote", "100회까지 진행해줘", "--context", "이 창의 총상한 요청", "--reason", "reduce scope"); code != 0 {
				t.Fatalf("budget: %d %s", code, out)
			}
			b := f.report()["budget"].(map[string]any)
			if b["runs"] != float64(48) || b["runs_limit"] != float64(limit) {
				t.Fatalf("lost usage or wrong total: %v", b)
			}
			remaining, excess := limit-48, 48-limit
			if remaining < 0 {
				remaining = 0
			}
			if excess < 0 {
				excess = 0
			}
			if b["remaining_runs"] != float64(remaining) || b["excess_runs"] != float64(excess) {
				t.Fatalf("wrong remaining/overage: %v", b)
			}
		})
	}
}

func TestBaselineBudgetAC9_TimeUnchanged(t *testing.T) {
	f := newPreflightFixture(t)
	before := f.report()["budget"].(map[string]any)
	if code, out := budgetRequest(t, f, 50, 100, 1, 1, "--quote", "좋아", "--context", "이 목표 현재 창 총상한 100회 요청에 대한 답"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	after := f.report()["budget"].(map[string]any)
	if after["elapsed_limit_seconds"] != before["elapsed_limit_seconds"] || after["window_start"] != before["window_start"] || after["runs"] != before["runs"] {
		t.Fatalf("time/window/usage reset: %v -> %v", before, after)
	}
	previous := f.read(scenarioGoal + "/runs.jsonl")
	if code, _ := f.ha("budget", scenarioGoal, "--elapsed", "5h"); code == 0 {
		t.Fatal("unexpected time mutation")
	}
	if !bytes.Equal(previous, f.read(scenarioGoal+"/runs.jsonl")) {
		t.Fatal("unsupported time mutation wrote history")
	}
}

func TestBaselineBudgetAC10_ConsentAndValidation(t *testing.T) {
	f := newPreflightFixture(t)
	for _, extra := range [][]string{nil, {"--quote", "좋아"}, {"--context", "total 100"}} {
		before := f.read(scenarioGoal + "/runs.jsonl")
		if code, _ := budgetRequest(t, f, 50, 100, 1, 1, extra...); code == 0 {
			t.Fatal("missing bound user context accepted")
		}
		if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
			t.Fatal("rejected event changed record")
		}
	}
	if code, out := budgetRequest(t, f, 50, 40, 1, 1, "--reason", "smaller task"); code != 0 {
		t.Fatalf("reduction unnecessarily required user quote: %d %s", code, out)
	}
	for _, to := range []int{0, -1, 40} {
		before := f.read(scenarioGoal + "/runs.jsonl")
		if code, _ := budgetRequest(t, f, 40, to, 1, 2, "--reason", "invalid"); code == 0 {
			t.Fatal("empty/invalid/no-op accepted")
		}
		if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
			t.Fatal("rejection appended")
		}
	}
}

func TestBaselineBudgetAC11_ConcurrentRevision(t *testing.T) {
	f := newPreflightFixture(t)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, limit := range []int{80, 90} {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			code, _ := budgetRequest(t, f, 50, n, 1, 1, "--quote", "increase this window", "--context", "explicit total")
			codes <- code
		}(limit)
	}
	wg.Wait()
	close(codes)
	ok, refused := 0, 0
	for c := range codes {
		if c == 0 {
			ok++
		}
		if c == 65 {
			refused++
		}
	}
	if ok != 1 || refused != 1 || len(fixtureRecords(t, f)) != 2 {
		t.Fatalf("concurrent changes overwritten: successes=%d refused=%d", ok, refused)
	}
	if code, out := f.ha("note", scenarioGoal, "reopen", "--quote", "resume"); code != 0 {
		t.Fatal(out)
	}
	if code, _ := budgetRequest(t, f, 50, 100, 1, 1, "--quote", "old request", "--context", "old window"); code != 65 {
		t.Fatalf("stale window not refused: %d", code)
	}
}

func TestBaselineBudgetAC12_Persistence(t *testing.T) {
	f := newPreflightFixture(t)
	if code, out := budgetRequest(t, f, 50, 90, 1, 1, "--quote", "90회까지", "--context", "same window total", "--interpretation", "agent interpretation"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	ls := fixtureRecords(t, f)
	change, ok := ls[len(ls)-1]["budget_change"].(map[string]any)
	if !ok || change["quote"] != "90회까지" || change["context"] != "same window total" || change["interpretation"] != "agent interpretation" {
		t.Fatalf("provenance not preserved: %v", ls)
	}
	first := mustJSON(f.report())
	if !bytes.Equal(first, mustJSON(f.report())) {
		t.Fatal("record-only status is nondeterministic")
	}
	change["previous_runs"] = float64(49)
	putFixtureRecords(t, f, ls)
	if f.report()["status"] == "complete" || !bytes.Contains(mustJSON(f.report()), []byte("record_invalid")) {
		t.Fatal("invalid persisted transition accepted")
	}
}

func TestBaselineBudgetAC13_ReopenAndCompletion(t *testing.T) {
	f := newPreflightFixture(t)
	if code, out := budgetRequest(t, f, 50, 90, 1, 1, "--quote", "90회까지", "--context", "current total"); code != 0 {
		t.Fatal(out)
	}
	if code, out := f.ha("note", scenarioGoal, "block", "--cause", "environment"); code != 0 {
		t.Fatal(out)
	}
	if f.report()["status"] != "blocked" {
		t.Fatal("budget changed unrelated blocking")
	}
	if code, out := f.ha("note", scenarioGoal, "reopen", "--quote", "resume"); code != 0 {
		t.Fatal(out)
	}
	b := f.report()["budget"].(map[string]any)
	if b["runs_limit"] != float64(90) || b["runs"] != float64(0) || b["elapsed_limit_seconds"] != float64(14400) {
		t.Fatalf("reopen limits/usage: %v", b)
	}
	ls := fixtureRecords(t, f)
	// An explicit completed window needs reopen even if a later code edit
	// would make the old completion's input proof stale.
	ls = append(ls, map[string]any{"schema": "run/v1", "kind": "done", "at": ls[len(ls)-1]["at"], "status": "complete", "ha_version": "synthetic"})
	putFixtureRecords(t, f, ls)
	if code, out := budgetRequest(t, f, 90, 100, 4, 4, "--quote", "more", "--context", "total"); code != 65 || !strings.Contains(out, "reopen") {
		t.Fatalf("completed window changed: %d %s", code, out)
	}
}

func TestBaselineBudgetAC14_Legacy(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2"} {
		t.Run(rules, func(t *testing.T) {
			f := newPreflightFixture(t)
			f.rules(rules)
			// Preserve legacy's plain exit interpretation, including an expected
			// failure without the new contract.
			plan := strings.Replace(string(f.read(scenarioGoal+"/PLAN.md")), "sh tests/greeting.sh", "sh tests/legacy-fail.sh", 1)
			plan = strings.Replace(plan, "| tests/greeting.sh |", "| tests/legacy-fail.sh |", 1)
			plan += "\n## 결과 계약\nLegacy free-form notes were not a structured declaration.\n"
			f.put(scenarioGoal+"/PLAN.md", plan)
			f.put("tests/legacy-fail.sh", "exit 1\n")
			f.git("add", "-A")
			f.git("commit", "-qm", "legacy check")
			if code, out := f.ha("check", scenarioGoal, "--baseline", "AC-1"); code != 0 || !strings.Contains(out, "fail_as_expected") {
				t.Fatalf("legacy changed: %d %s", code, out)
			}
			if code, _ := budgetRequest(t, f, 50, 90, 1, 1, "--quote", "more", "--context", "total"); code != 66 {
				t.Fatalf("legacy event accepted: %d", code)
			}
			if code, out := f.ha("note", scenarioGoal, "reopen", "--quote", "again"); code != 0 {
				t.Fatal(out)
			}
			if f.report()["budget"].(map[string]any)["runs_limit"] != float64(50) {
				t.Fatal("legacy reopen changed")
			}
		})
	}
}

func TestBaselineBudgetAC15_BudgetOnlyComplete(t *testing.T) {
	f := resultFixture(t, "reference")
	requireResult(t, resultCheck(t, f, true), "fail_as_expected")
	f.put("src/greeting.txt", "hello\n")
	f.git("add", "src/greeting.txt")
	f.git("commit", "-qm", "implement")
	requireResult(t, resultCheck(t, f, false), "pass")
	if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
		t.Fatal(out)
	}
	f.put(scenarioGoal+"/PROGRESS.md", "## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | synthetic |\n")
	if code, out := budgetRequest(t, f, 50, 1, 1, 1, "--reason", "reduce cap below actual use"); code != 0 {
		t.Fatal(out)
	}
	f.put("docs/specs/other/PROGRESS.md", "other goal progress changed\n")
	if code, out := f.ha("done", scenarioGoal); code != 0 {
		t.Fatalf("/3 lost /2 exclusion or budget-only completion: %d %s", code, out)
	}
	if f.report()["budget"].(map[string]any)["runs"] != float64(3) {
		t.Fatal("budget-only completion hid usage")
	}
}

func TestBaselineBudgetAC16_ExistingEntryPaths(t *testing.T) {
	// Existing focused tests cover actual dirty, symlink, task, exclusion,
	// nested parser and minimum-budget paths under the new default.
	t.Run("dirty", TestCheckPreflightRefusesDirty)
	t.Run("diagnostics", TestDirtyDiagnostics)
	t.Run("batch", TestCheckPreflightStopsDirtyBatch)
	t.Run("exclusions", TestCheckPreflightPreservesExclusions)
	for _, target := range []string{"AC-2", "T001"} {
		t.Run(target, func(t *testing.T) {
			f := newPreflightFixture(t)
			if code, out := f.ha("check", scenarioGoal, target); code != 0 {
				t.Fatalf("unrelated path required result contract: %d %s", code, out)
			}
		})
	}
}
