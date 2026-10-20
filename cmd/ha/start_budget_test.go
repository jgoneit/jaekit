package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

const startBudgetGoal = "docs/specs/budget-example"

// startBudgetFixture is a synthetic goal with `changes` required change rows
// and the given `검증 실행` value, committed in a fresh repository that is
// the working directory. The user folders are fresh temporary directories.
func startBudgetFixture(t *testing.T, changes int, runs string) (root, xdg, home string) {
	t.Helper()
	root, xdg, home = t.TempDir(), t.TempDir(), t.TempDir()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "budget fixture")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "budget@example.invalid")
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	var spec, plan strings.Builder
	spec.WriteString("# Synthetic budget goal\n\nStatus: Ready\n\n## Acceptance Criteria\n")
	plan.WriteString("# PLAN\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n")
	var ids []string
	for i := 1; i <= changes; i++ {
		fmt.Fprintf(&spec, "- **AC-%d** A synthetic observable result.\n", i)
		fmt.Fprintf(&plan, "| AC-%d | change | `go version` | tests/observe.txt | T001 |\n", i)
		ids = append(ids, fmt.Sprintf("AC-%d", i))
	}
	spec.WriteString("\n## Open Decisions\n없음\n")
	fmt.Fprintf(&plan, "\n## 범위\n- 바꿀 수 있는 경로: `tests/**`\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n| T001 | synthetic behavior | %s | — | — | 예 | — |\n\n## 예산\n- 검증 실행: %s\n- 경과 시간: 4시간\n", strings.Join(ids, ", "), runs)
	estimateTestWrite(t, root, startBudgetGoal+"/SPEC.md", spec.String())
	estimateTestWrite(t, root, startBudgetGoal+"/PLAN.md", plan.String())
	estimateTestWrite(t, root, startBudgetGoal+"/REVIEW.md", "# REVIEW\n\n## 결론\nready\n")
	estimateTestWrite(t, root, startBudgetGoal+"/PROGRESS.md", "# PROGRESS\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | |\n")
	estimateTestWrite(t, root, "tests/observe.txt", "synthetic\n")
	estimateTestGit(t, root, "init", "-q", "-b", "main")
	estimateTestGit(t, root, "add", "-A")
	estimateTestGit(t, root, "commit", "-q", "-m", "synthetic base")
	t.Chdir(root)
	return root, xdg, home
}

func writeLimit(t *testing.T, base, content string) string {
	t.Helper()
	path := filepath.Join(base, "jaekit", "ha.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func startBudgetEstimate(t *testing.T) (int, map[string]any, string) {
	t.Helper()
	code, text := estimateTestCLI("estimate", startBudgetGoal, "--format", "json")
	var report struct {
		Auto map[string]any `json:"auto"`
	}
	json.Unmarshal([]byte(text), &report)
	return code, report.Auto, text
}

func startBudgetRecords(t *testing.T, root string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(startBudgetGoal), "runs.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func writeSkill(t *testing.T, declaration string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "skill")
	estimateTestWrite(t, dir, "SKILL.md", "---\nname: synthetic\nmetadata:\n  version: 1.0.0\n---\n\n# synthetic\n")
	if declaration != "" {
		estimateTestWrite(t, dir, "core-requirements.json", declaration)
	}
	return filepath.Join(dir, "SKILL.md")
}

// TestNativeStartBudgetAndSkill runs on every platform the Windows native
// checks and the Unix native CI cover: the configuration folder rule, the
// automatic limit and its refusal, and a skill's Core requirements.
func TestNativeStartBudgetAndSkill(t *testing.T) {
	t.Run("ancestors", checkPreapprovalAncestors)
	t.Run("folder_identity", checkFolderIdentity)
	t.Run("home_before_userprofile", func(t *testing.T) {
		_, _, home := startBudgetFixture(t, 34, "자동")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("USERPROFILE", t.TempDir()) // used only when HOME is empty
		want := writeLimit(t, filepath.Join(home, ".config"), `{"preapproved_runs": 120}`)
		code, auto, text := startBudgetEstimate(t)
		if code != 0 || auto["source_path"] != want {
			t.Fatalf("HOME did not take precedence: exit %d, %v: %s", code, auto, text)
		}
	})
	t.Run("xdg_folder", func(t *testing.T) {
		_, xdg, home := startBudgetFixture(t, 34, "자동")
		writeLimit(t, filepath.Join(home, ".config"), `{"preapproved_runs": 500}`)
		want := writeLimit(t, xdg, `{"preapproved_runs": 120}`)
		code, auto, text := startBudgetEstimate(t)
		if code != 0 || auto["preapproved_runs"] != float64(120) || auto["source_path"] != want {
			t.Fatalf("XDG_CONFIG_HOME/jaekit/ha.json not used: exit %d, %v: %s", code, auto, text)
		}
	})
	t.Run("home_folder", func(t *testing.T) {
		_, _, home := startBudgetFixture(t, 34, "자동")
		t.Setenv("XDG_CONFIG_HOME", "")
		if runtime.GOOS == "windows" {
			t.Setenv("HOME", "") // USERPROFILE is the home, as for os.UserHomeDir.
		}
		want := writeLimit(t, filepath.Join(home, ".config"), `{"preapproved_runs": 120}`)
		code, auto, text := startBudgetEstimate(t)
		if code != 0 || auto["preapproved_runs"] != float64(120) || auto["source_path"] != want {
			t.Fatalf("~/.config/jaekit/ha.json not used: exit %d, %v: %s", code, auto, text)
		}
	})
	t.Run("missing_default", func(t *testing.T) {
		startBudgetFixture(t, 34, "자동")
		code, auto, text := startBudgetEstimate(t)
		if code != 1 || auto["preapproved_runs"] != float64(50) || auto["source"] != "default" || auto["approval_needed"] != true {
			t.Fatalf("missing file: exit %d, %v: %s", code, auto, text)
		}
	})
	t.Run("corrupt_refused", func(t *testing.T) {
		root, xdg, _ := startBudgetFixture(t, 1, "자동")
		for _, content := range []string{"{", `{"preapproved_runs": "60"}`, `{"preapproved_runs": 0}`, `{"preapproved_runs": 60, "x": 1}`} {
			path := writeLimit(t, xdg, content)
			code, text := estimateTestCLI("estimate", startBudgetGoal)
			if code != 65 || !strings.Contains(text, "pre-approved run limit: "+path) {
				t.Errorf("%s: estimate exit %d: %s", content, code, text)
			}
			code, text = estimateTestCLI("start", startBudgetGoal, "--request", "synthetic")
			if code != 65 || startBudgetRecords(t, root) != nil {
				t.Errorf("%s: start exit %d: %s", content, code, text)
			}
		}
		if err := os.RemoveAll(filepath.Join(xdg, "jaekit")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(xdg, "jaekit"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if code, text := estimateTestCLI("estimate", startBudgetGoal); code != 65 || !strings.Contains(text, "is not a folder") {
			t.Errorf("a file where the folder belongs: exit %d: %s", code, text)
		}
		t.Setenv("XDG_CONFIG_HOME", "relative")
		if code, text := estimateTestCLI("estimate", startBudgetGoal); code != 65 || !strings.Contains(text, "XDG_CONFIG_HOME is not an absolute path") {
			t.Errorf("relative XDG_CONFIG_HOME: exit %d: %s", code, text)
		}
		inside := filepath.Join(root, "cfg")
		writeLimit(t, inside, `{"preapproved_runs": 60}`)
		t.Setenv("XDG_CONFIG_HOME", inside)
		if code, text := estimateTestCLI("estimate", startBudgetGoal); code != 65 || !strings.Contains(text, "inside the repository") {
			t.Errorf("a file inside the repository: exit %d: %s", code, text)
		}
	})
	t.Run("auto_start", func(t *testing.T) {
		root, xdg, _ := startBudgetFixture(t, 34, "자동")
		path := writeLimit(t, xdg, "\xef\xbb\xbf{\"preapproved_runs\": 120}\n")
		code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--rules", "run-rules/4")
		if code != 0 || !strings.Contains(text, "auto limit: 102") || !strings.Contains(text, "pre-approved limit: 120 from "+path) {
			t.Fatalf("start exit %d: %s", code, text)
		}
		records := startBudgetRecords(t, root)
		budget, _ := records[0]["budget"].(map[string]any)
		auto, _ := budget["auto"].(map[string]any)
		if budget["runs"] != float64(102) || auto["minimum_runs"] != float64(68) || auto["source"] != "user_config" {
			t.Fatalf("start record budget %v", budget)
		}
	})
	t.Run("auto_refused", func(t *testing.T) {
		root, xdg, _ := startBudgetFixture(t, 34, "자동")
		writeLimit(t, xdg, `{"preapproved_runs": 101}`)
		code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic")
		if code != 65 || !strings.Contains(text, "auto budget 102 runs") || startBudgetRecords(t, root) != nil {
			t.Fatalf("start over the limit: exit %d: %s", code, text)
		}
	})
	t.Run("skill_missing_support", func(t *testing.T) {
		root, _, _ := startBudgetFixture(t, 1, "60")
		skill := writeSkill(t, `{"schema": "core-requirements/v1", "features": ["no-such-feature/v1"]}`)
		code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", skill)
		if code != 65 || !strings.Contains(text, "missing Core support: features no-such-feature/v1 (ha "+version+")") || startBudgetRecords(t, root) != nil {
			t.Fatalf("unsupported feature: exit %d: %s", code, text)
		}
		malformed := writeSkill(t, `{"schema": "core-requirements/v1", "features": null}`)
		if code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", malformed); code != 65 || startBudgetRecords(t, root) != nil {
			t.Fatalf("malformed declaration: exit %d: %s", code, text)
		}
		plain := writeSkill(t, "")
		if code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", plain, "--check-skill"); code != 65 || startBudgetRecords(t, root) != nil {
			t.Fatalf("--check-skill without a declaration: exit %d: %s", code, text)
		}
		// A declaration that cannot be read is reported as the declaration,
		// not by the skill folder's digest.
		dangling := writeSkill(t, "")
		declaration := filepath.Join(filepath.Dir(dangling), coreRequirementsFile)
		if err := os.Symlink(filepath.Join(filepath.Dir(dangling), "moved.json"), declaration); err == nil {
			code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", dangling)
			if code != 65 || !strings.Contains(text, "skill requirements "+declaration+": ") || !strings.Contains(text, "; nothing recorded") || startBudgetRecords(t, root) != nil {
				t.Fatalf("dangling declaration: exit %d: %s", code, text)
			}
		} else if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		// A declaration that is a FIFO is refused, never opened by the folder digest.
		fifo := writeSkill(t, "")
		if err := makeEvidenceFIFO(filepath.Join(filepath.Dir(fifo), coreRequirementsFile)); err == nil {
			code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", fifo)
			if code != 65 || !strings.Contains(text, "skill requirements ") || startBudgetRecords(t, root) != nil {
				t.Fatalf("FIFO declaration: exit %d: %s", code, text)
			}
		} else if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		// Without a declaration, a skill path that cannot be read keeps its own error.
		file := filepath.Join(t.TempDir(), "notes")
		estimateTestWrite(t, filepath.Dir(file), "notes", "")
		code, text = estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", filepath.Join(file, "SKILL.md"))
		if code != 65 || strings.Contains(text, coreRequirementsFile) || startBudgetRecords(t, root) != nil {
			t.Fatalf("skill below a file: exit %d: %s", code, text)
		}
	})
	t.Run("skill_supported", func(t *testing.T) {
		root, _, _ := startBudgetFixture(t, 1, "60")
		have := supportedCapabilities()
		declaration, _ := json.Marshal(map[string]any{"schema": coreRequirementsSchema, "rules": have.SupportedRules,
			"formats": have.Formats, "features": have.Features})
		skill := writeSkill(t, string(declaration))
		code, text := estimateTestCLI("start", startBudgetGoal, "--request", "synthetic", "--skill", skill, "--check-skill")
		if code != 0 || len(startBudgetRecords(t, root)) != 1 {
			t.Fatalf("supported declaration: exit %d: %s", code, text)
		}
	})
}

func TestAutoRunsIsMaxOf50AndMinimumTimesOneAndAHalf(t *testing.T) {
	for minimum, want := range map[int]int{0: 50, 1: 50, 33: 50, 34: 51, 35: 53, 68: 102, 100: 150} {
		if got := autoRuns(minimum); got != want {
			t.Errorf("autoRuns(%d) = %d, want %d", minimum, got, want)
		}
	}
}

func TestReadPreapprovalForms(t *testing.T) {
	repo := t.TempDir()
	for content, want := range map[string]int{
		`{"preapproved_runs": 1}`: 1, `{"preapproved_runs": 2147483647}`: 2147483647, " {\"preapproved_runs\":120}\n": 120,
		"\xef\xbb\xbf{\"preapproved_runs\": 7}": 7,
	} {
		xdg := t.TempDir()
		writeLimit(t, xdg, content)
		got, err := readPreapproval(func(k string) string { return map[string]string{"XDG_CONFIG_HOME": xdg}[k] }, func() ([]string, error) { return []string{repo}, nil })
		if err != nil || got.Runs != want || got.Source != "user_config" {
			t.Errorf("%q: %+v, %v", content, got, err)
		}
	}
	for _, content := range []string{"", "{}", "null", "[120]", `{"preapproved_runs": null}`, `{"preapproved_runs": "120"}`,
		`{"preapproved_runs": 0}`, `{"preapproved_runs": -1}`, `{"preapproved_runs": 1.5}`, `{"preapproved_runs": 1e2}`,
		`{"preapproved_runs": 01}`, `{"preapproved_runs": 2147483648}`, `{"preapproved_runs": 120} {}`,
		`{"preapproved_runs": 120, "preapproved_runs": 130}`, `{"preapproved_runs": 120, "x": 1}`,
		`{"Preapproved_Runs": 120}`, `{"preapproved_runs": 60, "PREAPPROVED_RUNS": 500}`, "{\"preapproved_runs\": 120, \"\xff\": 1}"} {
		xdg := t.TempDir()
		path := writeLimit(t, xdg, content)
		_, err := readPreapproval(func(k string) string { return map[string]string{"XDG_CONFIG_HOME": xdg}[k] }, func() ([]string, error) { return []string{repo}, nil })
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%q: want a refusal naming %s, got %v", content, path, err)
		}
	}
}

// checkPreapprovalAncestors runs inside the native test, on every OS.
func checkPreapprovalAncestors(t *testing.T) {
	repo := t.TempDir()
	read := func(xdg string) (preapproval, error) {
		return readPreapproval(func(k string) string { return map[string]string{"XDG_CONFIG_HOME": xdg}[k] }, func() ([]string, error) { return []string{repo}, nil })
	}
	// Nothing at the path and folders above it: the default.
	if got, err := read(filepath.Join(t.TempDir(), "not", "made")); err != nil || got.Source != "default" || got.Runs != autoFloor {
		t.Errorf("missing folders: %+v, %v", got, err)
	}
	// A file above the path, at any depth, is refused on every platform.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := read(filepath.Join(file, "deeper")); err == nil || !strings.Contains(err.Error(), file+" is not a folder") {
		t.Errorf("file above the path: %v", err)
	}
	// A configuration folder that is a symbolic link to nothing is refused, not absent.
	xdg := t.TempDir()
	folder := filepath.Join(xdg, configFolder)
	if err := os.Symlink(filepath.Join(xdg, "moved"), folder); err == nil {
		if _, err := read(xdg); err == nil || !strings.Contains(err.Error(), folder+" is a symbolic link whose target does not exist") {
			t.Errorf("dangling folder link: %v", err)
		}
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	// A folder that cannot be searched is refused.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		xdg := t.TempDir()
		writeLimit(t, xdg, `{"preapproved_runs": 120}`)
		folder := filepath.Join(xdg, configFolder)
		if err := os.Chmod(folder, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(folder, 0o755)
		if _, err := read(xdg); err == nil || !strings.Contains(err.Error(), filepath.Join(folder, configFile)) {
			t.Errorf("unsearchable folder: %v", err)
		}
	}
}

func TestReadCoreRequirementsForms(t *testing.T) {
	good := `{"schema": "core-requirements/v1", "rules": ["run-rules/4"], "formats": {"criteria": ["nested/1"]}, "features": ["estimate/v1"]}`
	req, _, err := readCoreRequirements(writeSkill(t, good))
	if err != nil || req == nil || len(req.missing(supportedCapabilities())) != 0 {
		t.Fatalf("supported declaration: %+v, %v", req, err)
	}
	if req, _, err := readCoreRequirements(writeSkill(t, "")); req != nil || err != nil {
		t.Fatalf("absent declaration: %+v, %v", req, err)
	}
	for _, bad := range []string{"", "null", "[]", `{}`, `{"schema": "core-requirements/v2"}`, `{"schema": 1}`,
		`{"schema": "core-requirements/v1", "x": []}`, `{"schema": "core-requirements/v1", "rules": null}`,
		`{"schema": "core-requirements/v1", "rules": "run-rules/4"}`, `{"schema": "core-requirements/v1", "rules": [""]}`,
		`{"schema": "core-requirements/v1", "rules": ["a", "a"]}`, `{"schema": "core-requirements/v1", "rules": ["a", null]}`,
		`{"schema": "core-requirements/v1", "formats": []}`, `{"schema": "core-requirements/v1", "formats": {"": ["x"]}}`,
		`{"schema": "core-requirements/v1", "formats": {"criteria": null}}`, `{"schema": "core-requirements/v1"} []`,
		`{"schema": "core-requirements/v1", "schema": "core-requirements/v1"}`} {
		dir := t.TempDir()
		estimateTestWrite(t, dir, "SKILL.md", "---\nname: x\n---\n")
		estimateTestWrite(t, dir, "core-requirements.json", bad)
		if _, _, err := readCoreRequirements(filepath.Join(dir, "SKILL.md")); err == nil {
			t.Errorf("%q: want a refusal", bad)
		}
	}
	req = &coreRequirements{Rules: []string{"run-rules/9"}, Formats: map[string][]string{"nope": {"x/v1"}, "criteria": {"legacy", "nested/9"}, "empty": {}},
		Features: []string{"estimate/v1", "later/v2"}}
	got := strings.Join(req.missing(supportedCapabilities()), "; ")
	if got != "rules run-rules/9; formats.criteria nested/9; formats.empty; formats.nope x/v1; features later/v2" {
		t.Fatalf("missing = %s", got)
	}
	// Several bad format entries give the same reason on every run: the first key in sorted order.
	for i := 0; i < 20; i++ {
		dir := t.TempDir()
		estimateTestWrite(t, dir, "SKILL.md", "---\nname: x\n---\n")
		estimateTestWrite(t, dir, "core-requirements.json", `{"schema": "core-requirements/v1", "formats": {"b": "x", "a": null, "c": 1}}`)
		if _, _, err := readCoreRequirements(filepath.Join(dir, "SKILL.md")); err == nil || !strings.Contains(err.Error(), "formats.a ") {
			t.Fatalf("run %d: want the reason for formats.a, got %v", i, err)
		}
	}
	// Field names are matched exactly; a key that differs only in case is unknown.
	for _, bad := range []string{`{"schema": "core-requirements/v1", "features": ["later/v2"], "FEATURES": []}`,
		`{"Schema": "core-requirements/v1"}`} {
		dir := t.TempDir()
		estimateTestWrite(t, dir, "SKILL.md", "---\nname: x\n---\n")
		estimateTestWrite(t, dir, "core-requirements.json", bad)
		if _, _, err := readCoreRequirements(filepath.Join(dir, "SKILL.md")); err == nil || !strings.Contains(err.Error(), "matched exactly") {
			t.Errorf("%q: want an exact-key refusal, got %v", bad, err)
		}
	}
}

// checkFolderIdentity runs inside the native test, on every OS.
func checkFolderIdentity(t *testing.T) {
	// The tree's own name has letters, so swapping their case gives another
	// spelling (t.TempDir names its folders with digits only).
	root, other := filepath.Join(t.TempDir(), "Repo"), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		filepath.Join(root, "a", "ha.json"): true, root: true, filepath.Dir(root): false,
		filepath.Join(other, "ha.json"):                                            false,
		filepath.Join(filepath.Dir(root), filepath.Base(root)+"-other", "ha.json"): false,
	} {
		if _, got := within([]string{other + "-missing", root}, path); got != want {
			t.Errorf("within(%s) = %v, want %v", path, got, want)
		}
	}
	if tree, ok := within([]string{other, root}, filepath.Join(other, "x")); !ok || tree != other {
		t.Errorf("second tree: %s, %v", tree, ok)
	}
	// The same folder spelled in other letter case is the same folder where
	// the file system ignores case (macOS, Windows) and another folder where
	// it does not (Linux).
	swapped := filepath.Join(filepath.Dir(root), swapCase(filepath.Base(root)), "a", "ha.json")
	if strings.HasPrefix(swapped, root+string(filepath.Separator)) {
		t.Fatalf("the swapped spelling %s is the same string as the tree", swapped)
	}
	a, errA := os.Stat(root)
	b, errB := os.Stat(filepath.Dir(filepath.Dir(swapped)))
	same := errA == nil && errB == nil && os.SameFile(a, b)
	if _, got := within([]string{root}, swapped); got != same {
		t.Errorf("within(%s) = %v; the file system says same folder: %v", swapped, got, same)
	}
	// A tree named through a symbolic link is the folder it points to.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err == nil {
		if _, got := within([]string{link}, filepath.Join(root, "a", "ha.json")); !got {
			t.Errorf("tree named by a link: not within")
		}
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}
}

func swapCase(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsUpper(r) {
			return unicode.ToLower(r)
		}
		return unicode.ToUpper(r)
	}, s)
}

func TestReadPreapprovalListsTreesOnlyForAFile(t *testing.T) {
	xdg := t.TempDir()
	env := func(k string) string { return map[string]string{"XDG_CONFIG_HOME": xdg}[k] }
	failing := func() ([]string, error) { return nil, fmt.Errorf("git worktree list --porcelain -z: refused") }
	if got, err := readPreapproval(env, failing); err != nil || got.Source != "default" {
		t.Fatalf("absent file: %+v, %v", got, err)
	}
	path := writeLimit(t, xdg, `{"preapproved_runs": 120}`)
	if _, err := readPreapproval(env, failing); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "cannot list the repository's working trees") {
		t.Fatalf("file with an unreadable tree list: %v", err)
	}
}
