package main

// This file exercises the existing CLI and fixture APIs only. It can be
// overlaid alone on the previous code commit for a behavioral baseline.
import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const safetyProducer = `#!/usr/bin/env python3
import json, os, pathlib
with open(os.environ['HA_PREFLIGHT_MARKER'], 'a') as marker: marker.write('ran\n')
if os.environ.get('HA_SAFETY_EXPECT_COPY'):
 assert pathlib.Path('overlay/data.txt').read_text() == 'head overlay\n'
 assert pathlib.Path('new/deep/file.txt').read_text() == 'new file\n'
 assert pathlib.Path('checks/producer.py').stat().st_mode & 0o111
passed = pathlib.Path('src/greeting.txt').read_text().strip() == 'hello'
if os.environ.get('HA_EVIDENCE_PATH'):
 observation = {'target':'greeting', 'attempt':1, 'status':'pass' if passed else 'violation'}
 if not passed: observation['violation'] = 'missing-greeting'
 report = {'schema':'check-result/v1', 'producer':'synthetic', 'producer_version':'1',
  'invocation':os.environ['HA_EVIDENCE_INVOCATION'],
  'declaration_digest':os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
  'attempts_complete':True, 'observations':[observation]}
 pathlib.Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps(report))
raise SystemExit(0 if passed else 1)
`

func safetyFixture(t *testing.T) *preflightFixture {
	t.Helper()
	f := &preflightFixture{t: t, dir: t.TempDir(), marker: filepath.Join(t.TempDir(), "executed")}
	copyTree(t, filepath.Join(scenarioRoot, "_base"), f.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	t.Setenv("HA_PREFLIGHT_MARKER", f.marker)
	f.put(".gitignore", "ignored/\n")
	f.put("checks/producer.py", safetyProducer)
	if err := os.Chmod(filepath.Join(f.dir, "checks/producer.py"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.put("checks/declaration.json", `{"schema":"check-declaration/v1","producer":"synthetic","producer_version":"1","producer_paths":["checks/producer.py"],"targets":[{"id":"greeting","violation":"missing-greeting"}]}`)
	f.put("overlay/data.txt", "base overlay\n")
	p := string(f.read(scenarioGoal + "/PLAN.md"))
	p = strings.Replace(p, "`sh tests/greeting.sh` | tests/greeting.sh", "`./checks/producer.py` | checks/producer.py, checks/declaration.json, overlay/data.txt", 1)
	p = strings.Replace(p, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`, `overlay/**`, `new/**`", 1)
	p += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
	f.put(scenarioGoal+"/PLAN.md", p)
	return f
}

func safetyStart(t *testing.T, f *preflightFixture, rules string) {
	t.Helper()
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic base")
	t.Chdir(f.dir)
	if code, out := f.ha("start", scenarioGoal, "--request", "synthetic baseline safety"); code != 0 {
		t.Fatalf("fixture start: %d: %s", code, out)
	}
	f.rules(rules)
}

func safetyCommit(f *preflightFixture) {
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic current input")
}

func safetyLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func safetyRemove(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
}

func safetyRefused(t *testing.T, f *preflightFixture, problem string, args ...string) string {
	t.Helper()
	before := f.read(scenarioGoal + "/runs.jsonl")
	code, out := f.ha(append([]string{"check", scenarioGoal, "--baseline"}, args...)...)
	if code != 65 || !strings.Contains(out, problem) {
		t.Fatalf("[requirement] unsafe baseline was not refused with path %q: exit %d: %s", problem, code, out)
	}
	if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
		t.Fatal("[requirement] a preparation refusal added a check record")
	}
	if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
		t.Fatalf("[requirement] refused command executed: %v", err)
	}
	if len(f.logs()) != 0 {
		t.Fatal("[requirement] preparation refusal left an output log")
	}
	r := f.report()
	if r["usage"].(map[string]any)["total"] != float64(0) || r["budget"].(map[string]any)["runs"] != float64(0) {
		t.Fatalf("[requirement] preparation refusal charged usage: %v", r["usage"])
	}
	return out
}

func TestBaselineSafetyAC1_BaseSymlinks(t *testing.T) {
	for _, parent := range []bool{false, true} {
		for _, destination := range []string{"internal", "external", "dangling"} {
			name := destination + "-leaf"
			if parent {
				name = destination + "-parent"
			}
			t.Run(name, func(t *testing.T) {
				f := safetyFixture(t)
				outside := filepath.Join(t.TempDir(), "sentinel")
				if err := os.Mkdir(outside, 0o755); err != nil {
					t.Fatal(err)
				}
				sentinel := filepath.Join(outside, "data.txt")
				if err := os.WriteFile(sentinel, []byte("preserve\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				f.put("untouched/data.txt", "preserve\n")
				safetyRemove(t, filepath.Join(f.dir, "overlay/data.txt"))
				link, target := filepath.Join(f.dir, "overlay/data.txt"), sentinel
				if parent {
					safetyRemove(t, filepath.Join(f.dir, "overlay"))
					link, target = filepath.Join(f.dir, "overlay"), outside
				}
				switch destination {
				case "internal":
					target = "../untouched/data.txt"
					if parent {
						target = "untouched"
					}
				case "dangling":
					target = filepath.Join(outside, "missing")
				}
				safetyLink(t, target, link)
				safetyStart(t, f, "run-rules/3")
				safetyRemove(t, link)
				f.put("overlay/data.txt", "head overlay\n")
				safetyCommit(f)
				safetyRefused(t, f, "overlay/data.txt", "AC-1")
				data, err := os.ReadFile(sentinel)
				info, statErr := os.Stat(sentinel)
				if err != nil || statErr != nil || string(data) != "preserve\n" || info.Mode().Perm() != 0o700 {
					t.Fatal("[requirement] external file content or mode changed")
				}
				if _, err := os.Lstat(filepath.Join(outside, "missing")); !os.IsNotExist(err) {
					t.Fatal("[requirement] dangling destination was created")
				}
				if string(f.read("overlay/data.txt")) != "head overlay\n" || string(f.read("untouched/data.txt")) != "preserve\n" {
					t.Fatal("[requirement] original working tree changed")
				}
			})
		}
	}
}

func TestBaselineSafetyAC1_HeadSources(t *testing.T) {
	for _, kind := range []string{"extra-symlink", "extra-directory", "producer-symlink", "declaration-symlink"} {
		t.Run(kind, func(t *testing.T) {
			f := safetyFixture(t)
			safetyStart(t, f, "run-rules/3")
			p := "overlay/data.txt"
			if kind == "producer-symlink" {
				p = "checks/producer.py"
			} else if kind == "declaration-symlink" {
				p = "checks/declaration.json"
			}
			old := f.read(p)
			safetyRemove(t, filepath.Join(f.dir, p))
			if kind == "extra-directory" {
				f.put(p+"/child.txt", "regular child\n")
			} else {
				outside := filepath.Join(t.TempDir(), "source")
				if err := os.WriteFile(outside, old, 0o755); err != nil {
					t.Fatal(err)
				}
				safetyLink(t, outside, filepath.Join(f.dir, p))
			}
			safetyCommit(f)
			safetyRefused(t, f, p, "AC-1")
		})
	}
}

// Observe the disposable worktree before cleanup. Optional faults change only
// this fixture's temporary tree, after git checkout and before preparation.
func safetyGitObserver(t *testing.T, f *preflightFixture, fault string) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	observation := filepath.Join(t.TempDir(), "snapshot.json")
	t.Setenv("HA_SAFETY_REAL_GIT", git)
	t.Setenv("HA_SAFETY_OBSERVATION", observation)
	t.Setenv("HA_SAFETY_FAULT", fault)
	script := `import json, os, pathlib, subprocess, sys
a=sys.argv[1:]
if a[:2] == ['worktree','remove']:
 tree=pathlib.Path(a[-1]); p=tree/'overlay/data.txt'
 pathlib.Path(os.environ['HA_SAFETY_OBSERVATION']).write_text(json.dumps({'data':p.read_text(),'mode':p.stat().st_mode & 0o777}))
 # Restore our synthetic permissions so normal cleanup can remove the tree.
 if (tree/'blocked').is_dir(): (tree/'blocked').chmod(0o755)
code=subprocess.call([os.environ['HA_SAFETY_REAL_GIT']]+a)
if code == 0 and a[:2] == ['worktree','add']:
 tree=pathlib.Path(a[-2]); fault=os.environ.get('HA_SAFETY_FAULT')
 if fault == 'readonly': (tree/'blocked').chmod(0o555)
 if fault == 'changed-parent':
  (tree/'overlay/data.txt').unlink(); (tree/'overlay').rmdir()
  (tree/'overlay').symlink_to(tree/'untouched', target_is_directory=True)
sys.exit(code)
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!"+python+"\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return observation
}

func safetyObserved(t *testing.T, p string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBaselineSafetyAC2_WholeListBeforeWrites(t *testing.T) {
	for _, problem := range []string{"missing-source", "unsafe-last-destination"} {
		t.Run(problem, func(t *testing.T) {
			f := safetyFixture(t)
			p := string(f.read(scenarioGoal + "/PLAN.md"))
			p = strings.Replace(p, "overlay/data.txt |", "overlay/data.txt, last/data.txt |", 1)
			f.put(scenarioGoal+"/PLAN.md", p)
			if problem == "unsafe-last-destination" {
				if err := os.Mkdir(filepath.Join(f.dir, "last"), 0o755); err != nil {
					t.Fatal(err)
				}
				safetyLink(t, "../overlay/data.txt", filepath.Join(f.dir, "last/data.txt"))
			}
			safetyStart(t, f, "run-rules/3")
			f.put("overlay/data.txt", "head overlay\n")
			if problem == "unsafe-last-destination" {
				safetyRemove(t, filepath.Join(f.dir, "last/data.txt"))
				f.put("last/data.txt", "safe HEAD\n")
			}
			safetyCommit(f)
			observation := safetyGitObserver(t, f, "")
			safetyRefused(t, f, "last/data.txt", "AC-1")
			if got := safetyObserved(t, observation)["data"]; got != "base overlay\n" {
				t.Fatalf("[requirement] applied an earlier overlay before rejecting the full list: %q", got)
			}
		})
	}
}

func TestBaselineSafetyAC2_PreparationFailure(t *testing.T) {
	for _, fault := range []string{"readonly", "changed-parent"} {
		t.Run(fault, func(t *testing.T) {
			if fault == "readonly" && os.Geteuid() == 0 {
				t.Skip("permission fault needs an unprivileged process")
			}
			f := safetyFixture(t)
			f.put("blocked/data.txt", "base blocked\n")
			f.put("untouched/data.txt", "preserve\n")
			p := string(f.read(scenarioGoal + "/PLAN.md"))
			f.put(scenarioGoal+"/PLAN.md", strings.Replace(p, "overlay/data.txt |", "overlay/data.txt, blocked/data.txt |", 1))
			safetyStart(t, f, "run-rules/3")
			f.put("overlay/data.txt", "head overlay\n")
			safetyCommit(f)
			observation := safetyGitObserver(t, f, fault)
			problem := "overlay/data.txt"
			if fault == "readonly" {
				problem = "blocked/data.txt"
			}
			safetyRefused(t, f, problem, "AC-1")
			if fault == "readonly" && safetyObserved(t, observation)["data"] != "head overlay\n" {
				t.Fatal("fixture did not exercise an I/O failure after an earlier safe copy")
			}
			if string(f.read("overlay/data.txt")) != "head overlay\n" || string(f.read("untouched/data.txt")) != "preserve\n" {
				t.Fatal("[requirement] preparation failure changed the original tree")
			}
		})
	}
}

func TestBaselineSafetyAC3_PartialBatch(t *testing.T) {
	f := safetyFixture(t)
	p := string(f.read(scenarioGoal + "/PLAN.md"))
	p = strings.Replace(p, "| AC-2 | maintain", "| EX-1 | change | `./checks/producer.py` | checks/producer.py, checks/declaration.json, missing.txt | T001 |\n| EX-2 | change | `./checks/producer.py` | checks/producer.py, checks/declaration.json | T001 |\n| AC-2 | maintain", 1)
	p = strings.Replace(p, "AC-1, AC-2 |", "AC-1, AC-2, EX-1, EX-2 |", 1)
	p += "| EX-1 | checks/declaration.json |\n| EX-2 | checks/declaration.json |\n"
	f.put(scenarioGoal+"/PLAN.md", p)
	safetyStart(t, f, "run-rules/3")
	before := f.read(scenarioGoal + "/runs.jsonl")
	code, out := f.ha("check", scenarioGoal, "--baseline", "AC-1", "EX-1", "EX-2")
	if code != 65 || !strings.Contains(out, "missing.txt") || !strings.Contains(out, "1/3 targets executed, 2 not run") {
		t.Fatalf("[requirement] partial refusal lost target progress: exit %d: %s", code, out)
	}
	after := f.read(scenarioGoal + "/runs.jsonl")
	ls := fixtureRecords(t, f)
	if !bytes.HasPrefix(after, before) || len(ls) != 2 || ls[1]["result"] != "fail_as_expected" || ls[1]["target"].(map[string]any)["criterion"] != "AC-1" {
		t.Fatalf("[requirement] partial batch did not preserve exactly its first attempt: %v", ls)
	}
	marker, err := os.ReadFile(f.marker)
	if err != nil || string(marker) != "ran\n" {
		t.Fatalf("[requirement] wrong number of processes started: %q, %v", marker, err)
	}
	r := f.report()
	if r["usage"].(map[string]any)["total"] != float64(1) || r["budget"].(map[string]any)["runs"] != float64(1) || len(f.logs()) != 1 {
		t.Fatal("[requirement] rejected or unexecuted targets added usage or logs")
	}
}

func TestBaselineSafetyAC4_RegularOverlay(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/3"} {
		t.Run(rules, func(t *testing.T) {
			f := safetyFixture(t)
			safetyLink(t, "missing-unrelated", filepath.Join(f.dir, "unrelated-link"))
			safetyStart(t, f, rules)
			f.put("overlay/data.txt", "head overlay\n")
			f.put("new/deep/file.txt", "new file\n")
			p := string(f.read(scenarioGoal + "/PLAN.md"))
			f.put(scenarioGoal+"/PLAN.md", strings.Replace(p, "overlay/data.txt |", "overlay/data.txt, new/deep/file.txt |", 1))
			safetyCommit(f)
			t.Setenv("HA_SAFETY_EXPECT_COPY", "1")
			code, out := f.ha("check", scenarioGoal, "--baseline", "AC-1")
			if code != 0 || !strings.Contains(out, "fail_as_expected") {
				t.Fatalf("[requirement] safe regular overlay failed: %d: %s", code, out)
			}
			ls := fixtureRecords(t, f)
			if len(ls) != 2 || len(ls[1]["overlay"].([]any)) != 4 || f.report()["budget"].(map[string]any)["runs"] != float64(1) {
				t.Fatal("[requirement] safe baseline lost overlay evidence or usage")
			}
			if f.git("status", "--porcelain", "--", ".", ":(exclude,literal)"+scenarioGoal) != "" {
				t.Fatal("[requirement] baseline changed the original working tree")
			}
		})
	}
}

func TestBaselineSafetyAC4_RealStartFailureIsCharged(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/3"} {
		t.Run(rules, func(t *testing.T) {
			f := safetyFixture(t)
			safetyStart(t, f, rules)
			p := string(f.read(scenarioGoal + "/PLAN.md"))
			f.put(scenarioGoal+"/PLAN.md", strings.Replace(p, "`./checks/producer.py`", "`jaekit-synthetic-command-that-does-not-exist`", 1))
			code, out := f.ha("check", scenarioGoal, "--baseline", "AC-1")
			ls := fixtureRecords(t, f)
			if code != 1 || len(ls) != 2 || ls[1]["result"] != "error" || f.report()["budget"].(map[string]any)["runs"] != float64(1) {
				t.Fatalf("[requirement] real process start failure lost its record or usage: %d %s", code, out)
			}
		})
	}
}

func TestBaselineSafetyAC1_AllRulesRefuseNewUnsafeCopies(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/3"} {
		t.Run(rules, func(t *testing.T) {
			f := safetyFixture(t)
			safetyRemove(t, filepath.Join(f.dir, "overlay/data.txt"))
			safetyLink(t, "../src/keep.txt", filepath.Join(f.dir, "overlay/data.txt"))
			safetyStart(t, f, rules)
			before := f.read(scenarioGoal + "/runs.jsonl")
			safetyRemove(t, filepath.Join(f.dir, "overlay/data.txt"))
			f.put("overlay/data.txt", "head overlay\n")
			safetyCommit(f)
			safetyRefused(t, f, "overlay/data.txt", "AC-1")
			if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) || fixtureRecords(t, f)[0]["rules"] != rules {
				t.Fatal("[requirement] safety refusal rewrote or migrated a historical start")
			}
		})
	}
}

func TestBaselineSafetyAC1_CommittedSymlinkWithCheckoutDisabled(t *testing.T) {
	f := safetyFixture(t)
	safetyRemove(t, filepath.Join(f.dir, "overlay/data.txt"))
	safetyLink(t, "../src/keep.txt", filepath.Join(f.dir, "overlay/data.txt"))
	safetyStart(t, f, "run-rules/3")
	safetyRemove(t, filepath.Join(f.dir, "overlay/data.txt"))
	f.put("overlay/data.txt", "head overlay\n")
	safetyCommit(f)
	f.git("config", "core.symlinks", "false")
	safetyRefused(t, f, "overlay/data.txt", "AC-1")
}

func TestBaselineSafetyAC5_StoredCompletionAndReopen(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/3"} {
		t.Run(rules, func(t *testing.T) {
			f := safetyFixture(t)
			safetyStart(t, f, rules)
			f.put("src/greeting.txt", "hello\n")
			safetyCommit(f)
			for _, args := range [][]string{{"check", scenarioGoal, "AC-1", "--baseline"}, {"check", scenarioGoal, "AC-1", "AC-2"}} {
				if code, out := f.ha(args...); code != 0 {
					t.Fatalf("[requirement] normal %s evidence changed: %d %s", rules, code, out)
				}
			}
			f.put(scenarioGoal+"/PROGRESS.md", "# PROGRESS\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | verified |\n")
			if code, out := f.ha("done", scenarioGoal); code != 0 {
				t.Fatalf("[requirement] normal %s completion changed: %d %s", rules, code, out)
			}
			stored := f.read(scenarioGoal + "/runs.jsonl")
			r := f.report()
			if r["status"] != "complete" || r["budget"].(map[string]any)["runs"] != float64(3) || !bytes.Equal(stored, f.read(scenarioGoal+"/runs.jsonl")) {
				t.Fatal("[requirement] reading saved completion changed its interpretation or bytes")
			}
			if code, out := f.ha("note", scenarioGoal, "reopen", "--quote", "synthetic continuation"); code != 0 {
				t.Fatalf("[requirement] normal reopen changed: %d %s", code, out)
			}
			r = f.report()
			if !bytes.HasPrefix(f.read(scenarioGoal+"/runs.jsonl"), stored) || fixtureRecords(t, f)[0]["rules"] != rules || r["budget"].(map[string]any)["runs"] != float64(0) || r["usage"].(map[string]any)["total"] != float64(3) {
				t.Fatal("[requirement] reopen migrated prior records or lost historical usage")
			}
		})
	}
}
