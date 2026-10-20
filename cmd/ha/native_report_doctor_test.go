package main

// Shell-independent fixtures for ha report and ha doctor (run-record.md §1.3
// and §1.4). They use real Git, Go and test-binary child processes, so
// Windows native CI runs the same expectations as Unix.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/run"
)

// reportFixture is a goal carried to a completion record with the native
// producer of TestNativeCoreGitLifecycle.
func reportFixture(t *testing.T) *preflightFixture {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	f := &preflightFixture{t: t, dir: filepath.Join(t.TempDir(), "보고 space")}
	copyTree(t, base, f.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	for _, key := range []string{"GIT_AUTHOR_DATE", "GIT_COMMITTER_DATE"} {
		t.Setenv(key, "2026-01-02T03:04:05Z")
	}
	f.put(".gitattributes", "* -text\n")
	f.put("checks/observer.go", nativeCoreProducer)
	f.put("checks/declaration.json", `{"schema":"check-declaration/v1","producer":"synthetic","producer_version":"1","producer_paths":["checks/observer.go"],"targets":[{"id":"greeting","violation":"missing-greeting"}]}`)
	p := string(f.read(scenarioGoal + "/PLAN.md"))
	p = strings.Replace(p, "`sh tests/greeting.sh` | tests/greeting.sh", "`go run checks/observer.go` | checks/observer.go, checks/declaration.json", 1)
	p = strings.Replace(p, "`sh tests/keep.sh`", "`go run checks/observer.go keep`", 1)
	p = strings.Replace(p, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`", 1)
	p += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
	f.put(scenarioGoal+"/PLAN.md", p)
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic base")
	t.Chdir(f.dir)
	return f
}

func (f *preflightFixture) must(want int, args ...string) string {
	f.t.Helper()
	code, out := f.ha(args...)
	if code != want {
		f.t.Fatalf("ha %v: %d want %d: %s", args, code, want, out)
	}
	return out
}

// head is the record head and its stored time, as ha status reports them.
func (f *preflightFixture) head() (seq int, hash, at string) {
	f.t.Helper()
	rep := f.report()
	h, _ := rep["record_head"].(map[string]any)
	n, _ := h["seq"].(float64)
	hash, _ = h["sha256"].(string)
	data := bytes.TrimSpace(f.read(scenarioGoal + "/runs.jsonl"))
	var last map[string]any
	if err := json.Unmarshal(data[bytes.LastIndexByte(data, '\n')+1:], &last); err != nil {
		f.t.Fatal(err)
	}
	at, _ = last["at"].(string)
	if n == 0 || len(hash) < 12 || at == "" {
		f.t.Fatalf("no record head in %v", rep)
	}
	return int(n), hash[:12], at
}

func TestNativeReportOutput(t *testing.T) {
	f := reportFixture(t)
	base := f.git("rev-parse", "HEAD")[:12]
	f.must(0, "start", scenarioGoal, "--request", "synthetic native report")
	f.must(0, "check", scenarioGoal, "AC-1", "--baseline")
	f.put("src/greeting.txt", "hello\n")
	f.git("add", "src/greeting.txt")
	f.git("commit", "-qm", "synthetic change")
	commit := f.git("rev-parse", "HEAD")[:12]
	f.must(0, "check", scenarioGoal, "AC-1", "AC-2")
	f.put(scenarioGoal+"/PROGRESS.md", strings.Replace(string(f.read(scenarioGoal+"/PROGRESS.md")), "| todo |", "| done |", 1))
	f.must(0, "done", scenarioGoal)
	seq, hash, at := f.head()
	records := f.read(scenarioGoal + "/runs.jsonl")

	want := map[string]string{
		"en": `Changed: 1 files and 1 commits outside the bundle documents since base {base}
Checked: 2 of 2 required conditions satisfied (2 by check records, 0 by user confirmation); 1 of 1 change conditions have a fresh baseline
To review: none
Remaining limits: assurance local; check author: executor
Next step: none; completion record seq {seq} holds and nothing is left to do

### Stamp for ` + "`## 현재`" + `

- record head: seq {seq} ({hash}), recorded at {at}; status: complete; completion record: seq {seq}

### Evidence per condition

Source: completion record seq {seq}
| ID | kind | required | satisfied | records |
| --- | --- | --- | --- | --- |
| AC-1 | change | yes | yes | 2, 3 |
| AC-2 | maintain | yes | yes | 4 |

### Completion coverage

- completion record seq {seq}, commit {commit}; HEAD {commit} is covered

### Output of unmet conditions

- none
`,
		"ko": `바뀐 것: base {base} 이후 실행 묶음 문서 밖에서 파일 1개, commit 1개
확인한 것: 필수 조건 2개 중 2개 충족(검사 기록 2개, 사용자 확인 0개); change 조건 1개 중 1개에 신선한 baseline
직접 볼 것: 없음
남은 한계: 보증 수준 local; 검사 작성자: 실행자
다음 행동: 없음; 완료 기록 seq {seq}이 유효하고 남은 할 일이 없음

### ` + "`## 현재`" + ` 도장

- 기록 head: seq {seq} ({hash}), 기록 시각 {at}; 상태: complete; 완료 기록: seq {seq}

### 조건별 근거

근거 출처: 완료 기록 seq {seq}
| ID | 종류 | 필수 | 충족 | 근거 기록 |
| --- | --- | --- | --- | --- |
| AC-1 | change | 예 | 예 | 2, 3 |
| AC-2 | maintain | 예 | 예 | 4 |

### 완료 범위

- 완료 기록 seq {seq}, commit {commit}; 지금 HEAD {commit}를 덮음

### 미충족 조건의 원문 위치

- 없음
`,
	}
	fill := strings.NewReplacer("{base}", base, "{commit}", commit, "{seq}", fmt.Sprint(seq), "{hash}", hash, "{at}", at)
	for _, lang := range []string{"en", "ko"} {
		t.Run(lang, func(t *testing.T) {
			args := []string{"report", scenarioGoal}
			if lang == "ko" {
				args = append(args, "--lang", "ko")
			}
			var out, stderr bytes.Buffer
			if code := runMain(args, &out, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("ha %v: exit %d: %s", args, code, stderr.String())
			}
			if got, expected := out.String(), fill.Replace(want[lang]); got != expected {
				t.Fatalf("ha %v printed\n%s\nwant\n%s", args, got, expected)
			}
			if !bytes.Equal(f.read(scenarioGoal+"/runs.jsonl"), records) {
				t.Fatal("report changed the record")
			}
		})
	}
}

// TestNativeDoctorHelper is a probe compiled only into the Go test
// executable. It acts only when called with arguments after "--".
func TestNativeDoctorHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) || os.Args[separator+1] != "doctor-helper" {
		return
	}
	args := os.Args[separator+2:]
	switch {
	case len(args) == 1 && args[0] == "ok":
		fmt.Println("synthetic ok output")
		os.Exit(0)
	case len(args) == 2 && args[0] == "exit":
		fmt.Println("synthetic failure")
		code := 0
		fmt.Sscan(args[1], &code)
		os.Exit(code)
	case len(args) == 1 && args[0] == "tail":
		for i := 1; i <= 30; i++ {
			fmt.Printf("line %02d\r\n", i)
		}
		os.Exit(3)
	case len(args) == 2 && args[0] == "parent":
		// A descendant keeps writing until the process control ends it.
		child := exec.Command(os.Args[0], "-test.run=^TestNativeDoctorHelper$", "--", "doctor-helper", "beat", args[1])
		if err := child.Start(); err != nil {
			os.Exit(65)
		}
		time.Sleep(10 * time.Minute)
	case len(args) == 2 && args[0] == "beat":
		for {
			f, err := os.OpenFile(args[1], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err == nil {
				f.WriteString("beat\n")
				f.Close()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	os.Exit(64)
}

func doctorFixture(t *testing.T, probes string) *preflightFixture {
	t.Helper()
	f := reportFixture(t)
	f.put(scenarioGoal+"/PLAN.md", string(f.read(scenarioGoal+"/PLAN.md"))+"\n## 실행 환경\n"+probes)
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic probes")
	return f
}

// helperCommand is the check command that runs this test executable's helper.
func helperCommand(t *testing.T, args string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(exe, "'") {
		t.Skip("test executable path contains a single quote")
	}
	return "'" + exe + "' '-test.run=^TestNativeDoctorHelper$' -- doctor-helper " + args
}

func TestNativeDoctorProbes(t *testing.T) {
	type probe struct{ name, args, want string }
	cases := []struct {
		name   string
		probes []probe
		code   int
		tail   []string
	}{
		{"available", []probe{{"도우미", "ok", "도우미: 사용 가능"}}, 0, nil},
		{"missing", []probe{{"없는 도구", "", "없는 도구: 사용 불가 (실행 파일 없음)"}}, 1, nil},
		{"exit", []probe{{"도우미", "ok", "도우미: 사용 가능"}, {"실패", "exit 7", "실패: 사용 불가 (0이 아닌 종료, 종료 코드 7)"}}, 1, []string{"    synthetic failure"}},
		{"timeout", []probe{{"멈춤", "parent", "멈춤: 사용 불가 (시간 초과, 60초)"}}, 1, nil},
		{"tail", []probe{{"긴 출력", "tail", "긴 출력: 사용 불가 (0이 아닌 종료, 종료 코드 3)"}}, 1, nil},
	}
	for i := 11; i <= 30; i++ {
		cases[4].tail = append(cases[4].tail, fmt.Sprintf("    line %02d", i))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			heartbeat := filepath.Join(t.TempDir(), "heartbeat")
			var section, want []string
			for _, p := range tc.probes {
				command := "no-such-tool-ha-doctor-7c1e"
				if p.args == "parent" {
					command = helperCommand(t, "parent '"+heartbeat+"'")
				} else if p.args != "" {
					command = helperCommand(t, p.args)
				}
				section = append(section, fmt.Sprintf("- %s: `%s`", p.name, command))
				want = append(want, p.want)
			}
			f := doctorFixture(t, strings.Join(section, "\n")+"\n")
			records, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(scenarioGoal+"/runs.jsonl")))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			c := &cli{out: &out, err: &stderr}
			var limits []time.Duration
			c.execute = func(dir string, argv []string, timeout time.Duration, log string, env []string) (run.Result, error) {
				limits = append(limits, timeout)
				if tc.name == "timeout" {
					// Only the timer interval changes; the process control is ha check's.
					timeout = 3 * time.Second
				}
				return run.ExecWithEnv(dir, argv, timeout, log, env)
			}
			code := c.doctor([]string{scenarioGoal})
			var results, tail []string
			for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
				if strings.HasPrefix(line, "    ") {
					tail = append(tail, line)
				} else {
					results = append(results, line)
				}
			}
			if code != tc.code || stderr.Len() != 0 || strings.Join(results, "\n") != strings.Join(want, "\n") {
				t.Fatalf("ha doctor: exit %d want %d: %s%s", code, tc.code, out.String(), stderr.String())
			}
			if tc.tail != nil && strings.Join(tail, "\n") != strings.Join(tc.tail, "\n") {
				t.Fatalf("ha doctor tail:\n%s\nwant\n%s", strings.Join(tail, "\n"), strings.Join(tc.tail, "\n"))
			}
			for _, limit := range limits {
				if limit != doctorTimeout || doctorTimeout != 60*time.Second {
					t.Fatalf("ha doctor asked for limit %v", limit)
				}
			}
			if len(limits) != len(tc.probes) {
				t.Fatalf("ha doctor ran %d checks, want %d", len(limits), len(tc.probes))
			}
			after, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(scenarioGoal+"/runs.jsonl")))
			if (err == nil) != (records != nil) || !bytes.Equal(after, records) {
				t.Fatal("doctor changed the record")
			}
			if tc.name == "timeout" {
				first, err := os.Stat(heartbeat)
				if err != nil {
					t.Fatalf("the descendant never wrote its heartbeat: %v", err)
				}
				time.Sleep(time.Second)
				if later, err := os.Stat(heartbeat); err != nil || later.Size() != first.Size() {
					t.Fatal("a descendant still writes after the timeout")
				}
			}
			if status := f.git("status", "--porcelain", "--ignored"); status != "" {
				t.Fatalf("doctor left files: %s", status)
			}
		})
	}
}
