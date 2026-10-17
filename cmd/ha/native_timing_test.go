package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A check that uses measurable processor time in its own process.
const nativeTimingBurner = `package main
func main(){
 n := 0
 for i := 0; i < 400000000; i++ { n += i % 7 }
 if n < 0 { panic(n) }
}
`

// TestNativeRecordedTiming records one check through the CLI on the current
// system and reads its record line: the applied limit, the limit interval and
// the CPU time with this system's scope and source (run-record.md §2.2).
func TestNativeRecordedTiming(t *testing.T) {
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	f := &preflightFixture{t: t, dir: filepath.Join(t.TempDir(), "timing")}
	copyTree(t, base, f.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	f.put(".gitattributes", "* -text\n")
	f.put("checks/burn.go", nativeTimingBurner)
	p := string(f.read(scenarioGoal + "/PLAN.md"))
	p = strings.Replace(p, "`sh tests/keep.sh`", "`go run checks/burn.go`", 1)
	p = strings.Replace(p, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`", 1)
	f.put(scenarioGoal+"/PLAN.md", p)
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic base")
	t.Chdir(f.dir)
	if code, out := f.ha("start", scenarioGoal, "--request", "synthetic timing record"); code != 0 {
		t.Fatalf("start: %d %s", code, out)
	}
	if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
		t.Fatalf("check: %d %s", code, out)
	}
	var line struct {
		Kind       string `json:"kind"`
		DurationMS int64  `json:"duration_ms"`
		Timing     *struct {
			LimitMS        int64  `json:"limit_ms"`
			LimitElapsedMS *int64 `json:"limit_elapsed_ms"`
			LimitReached   bool   `json:"limit_reached"`
			CPU            struct {
				Status   string `json:"status"`
				UserMS   *int64 `json:"user_ms"`
				SystemMS *int64 `json:"system_ms"`
				Scope    string `json:"scope"`
				Source   string `json:"source"`
			} `json:"cpu"`
		} `json:"timing"`
	}
	lines := strings.Split(strings.TrimSpace(string(f.read(scenarioGoal+"/runs.jsonl"))), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &line); err != nil || line.Kind != "check" || line.Timing == nil {
		t.Fatalf("record line %q: %v", lines[len(lines)-1], err)
	}
	timing := line.Timing
	if timing.LimitMS != checkTimeout.Milliseconds() || timing.LimitReached || timing.LimitElapsedMS == nil ||
		*timing.LimitElapsedMS < 0 || *timing.LimitElapsedMS > line.DurationMS {
		t.Fatalf("limit values of a finished check: %+v, duration %d", timing, line.DurationMS)
	}
	scope, source := "process-and-waited-descendants", "wait4-rusage"
	if runtime.GOOS == "windows" {
		scope, source = "job-process-tree", "job-object-accounting"
	}
	cpu := timing.CPU
	if cpu.Status != "measured" || cpu.UserMS == nil || cpu.SystemMS == nil || *cpu.UserMS+*cpu.SystemMS <= 0 ||
		cpu.Scope != scope || cpu.Source != source {
		t.Fatalf("cpu of a check that used processor time on %s: %+v", runtime.GOOS, cpu)
	}
}
