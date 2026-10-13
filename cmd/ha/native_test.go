package main

// These portable fixtures use real Git and Go child processes without a shell.
// They are developer tests; installing or using ha does not require Go.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeCoreProducer = `package main
import("encoding/json";"fmt";"os";"strings")
func main(){
 fmt.Println("synthetic private output")
 if len(os.Args)>1 { return }
 data,err:=os.ReadFile("src/greeting.txt"); if err!=nil {panic(err)}
 passed:=strings.TrimSpace(string(data))=="hello"
 observation:=map[string]any{"target":"greeting","attempt":1,"status":"pass"}
 if !passed {observation["status"]="violation";observation["violation"]="missing-greeting"}
 report:=map[string]any{"schema":"check-result/v1","producer":"synthetic","producer_version":"1","invocation":os.Getenv("HA_EVIDENCE_INVOCATION"),"declaration_digest":os.Getenv("HA_EVIDENCE_DECLARATION_DIGEST"),"attempts_complete":true,"observations":[]any{observation}}
 raw,err:=json.Marshal(report);if err!=nil{panic(err)}
 if err=os.WriteFile(os.Getenv("HA_EVIDENCE_PATH"),raw,0600);err!=nil{panic(err)}
 if !passed {os.Exit(1)}
}
`

func TestNativeCoreGitLifecycle(t *testing.T) {
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprintf("linked=%v", linked), func(t *testing.T) {
			f := &preflightFixture{t: t, dir: filepath.Join(t.TempDir(), "프로젝트 space")}
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
			if linked {
				dir := filepath.Join(t.TempDir(), "연결 worktree")
				f.git("worktree", "add", "--detach", "-q", dir, "HEAD")
				f.dir = dir
			}
			t.Chdir(f.dir)
			must := func(want int, args ...string) string {
				t.Helper()
				code, out := f.ha(args...)
				if code != want {
					t.Fatalf("ha %v: %d want %d: %s", args, code, want, out)
				}
				return out
			}
			must(0, "start", scenarioGoal, "--request", "synthetic native lifecycle")
			must(0, "check", scenarioGoal, "AC-1", "--baseline")
			f.put("src/greeting.txt", "hello\n")
			must(65, "check", scenarioGoal, "AC-1")
			f.git("add", "src/greeting.txt")
			f.git("commit", "-qm", "synthetic change")
			must(0, "check", scenarioGoal, "AC-1", "AC-2")
			f.put(scenarioGoal+"/PROGRESS.md", strings.Replace(string(f.read(scenarioGoal+"/PROGRESS.md")), "| todo |", "| done |", 1))
			must(0, "done", scenarioGoal)
			out := must(0, "status", scenarioGoal, "--format", "json")
			if strings.Contains(out, "synthetic private output") {
				t.Fatal("status exposed raw output")
			}
			gitDir := f.git("rev-parse", "--absolute-git-dir")
			found := 0
			for _, line := range strings.Split(strings.TrimSpace(string(f.read(scenarioGoal+"/runs.jsonl"))), "\n") {
				var row map[string]any
				if err := json.Unmarshal([]byte(line), &row); err != nil {
					t.Fatal(err)
				}
				logPath, _ := row["output_path"].(string)
				if logPath == "" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(strings.TrimPrefix(logPath, ".git/"))))
				if err != nil {
					t.Fatal(err)
				}
				h := sha256.Sum256(data)
				if row["output_digest"] != hex.EncodeToString(h[:]) {
					t.Fatal("recorded log digest mismatch")
				}
				found++
			}
			if found != 3 {
				t.Fatalf("expected three recorded logs, got %d", found)
			}
			f.put("src/keep.txt", "dirty\n")
			if code, _ := f.ha("done", scenarioGoal); code == 0 {
				t.Fatal("dirty completion accepted")
			}
		})
	}
}
