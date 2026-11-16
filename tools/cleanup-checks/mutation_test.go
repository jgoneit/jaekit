//go:build goalcheckcleanup

package cleanupchecks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

// edit replaces one fixed string in a file of a copied tree. The string must
// occur exactly once in the file, or in the named section when one is given.
type edit struct {
	file    string
	section string
	old     string
	new     string
}

// outcome is what an inner go test run ended with.
type outcome int

const (
	passed outcome = iota
	testsFailed
)

// innerRun is the judged result of go test -json in a copied tree.
type innerRun struct {
	result outcome
	failed []string // failed tests, top-level and subtests
	passed map[string]bool
}

// syntheticTree copies the tracked files of the checked tree into a new
// repository with one commit, applies the edits and commits them. It never
// changes the checked tree.
func syntheticTree(t *testing.T, edits []edit) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()
	for _, file := range trackedFiles(t, root) {
		src := filepath.Join(root, filepath.FromSlash(file.path))
		dst := filepath.Join(dir, filepath.FromSlash(file.path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		switch file.mode {
		case "120000":
			target, err := os.Readlink(src)
			if err == nil {
				err = os.Symlink(target, dst)
			}
			if err != nil {
				t.Fatalf("setup: copy link %s: %v", file.path, err)
			}
		case "100644", "100755":
			perm := os.FileMode(0o644)
			if file.mode == "100755" {
				perm = 0o755
			}
			if err := os.WriteFile(dst, readFile(t, root, file.path), perm); err != nil {
				t.Fatalf("setup: copy %s: %v", file.path, err)
			}
		default:
			t.Fatalf("setup: %s has unsupported mode %s", file.path, file.mode)
		}
	}
	syntheticGit(t, dir, "init", "-q")
	syntheticCommit(t, dir, "copy of the checked tree")
	if len(edits) == 0 {
		return dir
	}
	for _, change := range edits {
		applyEdit(t, dir, change)
	}
	syntheticCommit(t, dir, "edit")
	return dir
}

func syntheticGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "user.name=cleanup-check", "-c", "user.email=cleanup-check@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main", "-c", "core.hooksPath=" + os.DevNull}
	gitOutput(t, dir, append(base, args...)...)
}

func syntheticCommit(t *testing.T, dir, message string) {
	t.Helper()
	syntheticGit(t, dir, "add", "-A")
	syntheticGit(t, dir, "commit", "-q", "--allow-empty", "-m", message)
}

func applyEdit(t *testing.T, dir string, change edit) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(change.file))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: read %s in the copy: %v", change.file, err)
	}
	text := string(data)
	start, end := 0, len(text)
	if change.section != "" {
		body, ok := section(text, change.section)
		if !ok {
			t.Fatalf("setup: %s has no section %q", change.file, change.section)
		}
		start = strings.Index(text, body)
		end = start + len(body)
	}
	if n := strings.Count(text[start:end], change.old); n != 1 {
		t.Fatalf("setup: %s has %d occurrences of %q where the edit needs one", change.file, n, change.old)
	}
	i := start + strings.Index(text[start:end], change.old)
	text = text[:i] + change.new + text[i+len(change.old):]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), info.Mode().Perm()); err != nil {
		t.Fatalf("setup: write %s in the copy: %v", change.file, err)
	}
}

// runPlugins runs go test -json on ./plugins in dir. A build failure, a run
// that does not start, a signal, or output that cannot be read is a setup
// failure; only passing or failing tests are an outcome.
func runPlugins(t *testing.T, dir string, extra ...string) innerRun {
	t.Helper()
	args := append([]string{"test", "-json", "-count=1", "-timeout=15m"}, extra...)
	args = append(args, "./plugins")
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() < 0 {
			t.Fatalf("setup: go %s did not finish: %v: %s", strings.Join(args, " "), err, tail(stderr.String()))
		}
		code = exit.ExitCode()
	}
	run := innerRun{passed: map[string]bool{}}
	packageAction := ""
	buildFailed := false
	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		var event struct {
			Action, Package, Test, Output string
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("setup: unreadable go test -json line %q: %v", tail(scanner.Text()), err)
		}
		switch {
		case event.Action == "build-fail":
			buildFailed = true
		case event.Test == "" && (event.Action == "pass" || event.Action == "fail"):
			packageAction = event.Action
		case event.Test != "" && event.Action == "fail":
			run.failed = append(run.failed, event.Test)
		case event.Test != "" && event.Action == "pass":
			run.passed[event.Test] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("setup: read go test output: %v", err)
	}
	t.Logf("go %s in the copy took %s", strings.Join(args, " "), time.Since(started).Round(time.Second))
	switch {
	case !buildFailed && code == 0 && packageAction == "pass" && len(run.failed) == 0:
		run.result = passed
	case !buildFailed && code == 1 && packageAction == "fail" && len(run.failed) > 0:
		run.result = testsFailed
		sort.Strings(run.failed)
	default:
		t.Fatalf("setup: go %s ended with exit %d, package %q, build failed %t, failed tests %v: %s",
			strings.Join(args, " "), code, packageAction, buildFailed, run.failed, tail(stderr.String()+stdout.String()))
	}
	return run
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		s = "…" + s[len(s)-2000:]
	}
	return s
}

// The unchanged copy must pass before an edited copy says anything. The
// first test that needs it runs it; a run that stops on a setup failure
// leaves the state failed, so every later test reports a setup failure too.
var (
	controlMu    sync.Mutex
	controlState string // "" not run, "passed", or why it did not pass
)

func requireControl(t *testing.T) {
	t.Helper()
	controlMu.Lock()
	defer controlMu.Unlock()
	switch controlState {
	case "passed":
		return
	case "":
	default:
		t.Fatalf("setup: the plugins tests do not pass on an unchanged copy of the checked tree: %s", controlState)
	}
	controlState = "the run on the unchanged copy stopped"
	if run := runPlugins(t, syntheticTree(t, nil)); run.result != passed {
		controlState = "the unchanged copy fails " + strings.Join(run.failed, ", ")
		t.Fatalf("setup: the plugins tests do not pass on an unchanged copy of the checked tree: %s", controlState)
	}
	controlState = "passed"
}

// requireFailure checks that an edit which breaks a kept property makes the
// plugins tests fail.
func requireFailure(t *testing.T, what string, edits []edit) {
	t.Helper()
	requireControl(t)
	if run := runPlugins(t, syntheticTree(t, edits)); run.result == passed {
		t.Errorf("the plugins tests pass on a tree that %s", what)
	} else {
		t.Logf("failed tests: %s", strings.Join(run.failed, ", "))
	}
}

// requirePass checks that a prose-only edit leaves the plugins tests passing.
func requirePass(t *testing.T, what string, edits []edit) {
	t.Helper()
	requireControl(t)
	if run := runPlugins(t, syntheticTree(t, edits)); run.result != passed {
		t.Errorf("jaekit:violation=prose-pinned: the plugins tests fail on a tree that only %s: %s",
			what, strings.Join(run.failed, ", "))
	}
}

// AC-6: a tree that changes the current release in one place fails.
func TestCleanupAC6_ReleaseVersion(t *testing.T) {
	requireFailure(t, "names another current release in README.md", []edit{{
		file: "README.md",
		old:  "The current release is **v0.1.8**,",
		new:  "The current release is **v0.1.6**,",
	}})
}

// AC-6: a tree whose Claude Code install command drops spar fails.
func TestCleanupAC6_InstallWithoutSpar(t *testing.T) {
	requireFailure(t, "drops spar from the Claude Code install command", []edit{{
		file:    "guides/INSTALL.md",
		section: "### Claude Code",
		old:     "claude plugin install spar@jaekit\n",
		new:     "",
	}})
}

// AC-6: a tree that changes one plugin manifest's version fails.
func TestCleanupAC6_ManifestVersion(t *testing.T) {
	requireFailure(t, "changes only the Spar Claude Code manifest version", []edit{{
		file: "plugins/spar/.claude-plugin/plugin.json",
		old:  `"version": "0.1.4",`,
		new:  `"version": "0.1.5",`,
	}})
}

// AC-6: a tree without the Spec rule that it stops after writing the goal
// documents fails.
func TestCleanupAC6_SpecStopRule(t *testing.T) {
	requireFailure(t, "removes the Spec rule that it stops after writing", []edit{{
		file: "plugins/spec/skills/spec/SKILL.md",
		old:  "- When a decision about the content is still open, do not ask it: report that implementation starts with a new request, then stop. This holds even when the request that invoked this skill, or a reply that answers your questions or asks for changes, also asks for implementation.\n",
		new:  "",
	}})
}

// AC-6: a tree with a README link to a missing anchor fails.
func TestCleanupAC6_BrokenLink(t *testing.T) {
	requireFailure(t, "links README.md to a missing anchor of the install guide", []edit{{
		file: "README.md",
		old:  "](guides/INSTALL.en.md#check-the-version)",
		new:  "](guides/INSTALL.en.md#check-the-version-twice)",
	}})
}

// AC-7: figure alt text in the READMEs that have figures.
func TestCleanupAC7_FigureCaption(t *testing.T) {
	requirePass(t, "rewords a figure description in README.md and README.ko.md", []edit{
		{file: "README.md", old: "![Fictional completion report for password-link in the shape of ha report output:",
			new: "![Fictional completion report for password-link, shaped like ha report output:"},
		{file: "README.ko.md", old: "![password-link의 가상 완료 보고, ha report 출력 모양:",
			new: "![password-link의 가상 완료 보고(ha report 출력과 같은 모양):"},
	})
}

// AC-7: the scene description of the demo GIF.
func TestCleanupAC7_DemoScenes(t *testing.T) {
	requirePass(t, "rewords a scene of the demo GIF description", []edit{
		{file: "README.md", old: "you review the goal it wrote;", new: "you read through the goal it wrote;"},
		{file: "README.ko.md", old: "Spec이 쓴 목표를 확인하고,", new: "Spec이 쓴 목표를 읽어 보고,"},
	})
}

// AC-7: a sentence of the v0.1.7 release notes.
func TestCleanupAC7_ReleaseNotes(t *testing.T) {
	requirePass(t, "rewords a sentence of the v0.1.7 release notes", []edit{{
		file: "tools/release/notes/v0.1.7.md",
		old:  "이번 릴리스에는 Windows 파일이 없습니다.",
		new:  "이 릴리스에는 Windows용 파일이 들어 있지 않습니다.",
	}})
}

// AC-7: the order of sentences in one guide paragraph.
func TestCleanupAC7_ParagraphOrder(t *testing.T) {
	first := "Approving every step keeps this in check, but then there is little point in handing the work over."
	second := "Trusting the report is easy, but an agent can edit its own checks or write easier criteria."
	requirePass(t, "swaps two sentences of one guide paragraph", []edit{{
		file: "guides/OVERVIEW.en.md",
		old:  first + " " + second,
		new:  second + " " + first,
	}})
}

// AC-7: one prose sentence in each of five guides, in both languages.
func TestCleanupAC7_GuideSentence(t *testing.T) {
	requirePass(t, "rewords one prose sentence in each of five guides", []edit{
		{file: "guides/USAGE.md", old: "확인한 내용이 그 뒤 다시 바뀌면 새 변경이라 다시 묻습니다.",
			new: "확인한 내용이 그 뒤에 또 바뀌면, 새 변경이므로 다시 묻습니다."},
		{file: "guides/USAGE.en.md", old: "Seal records an answer given up front as the confirmation, quoting it, once the change it allows is committed, and does not ask about that change again.",
			new: "Once the change it allows is committed, Seal records an answer given up front as the confirmation, quoting it, and does not ask about that change again."},
		{file: "guides/OVERVIEW.md", old: "Claude Code나 Codex에 일을 맡기면 이런 일이 생기곤 합니다.",
			new: "Claude Code나 Codex에 일을 맡기면 이런 일이 자주 생깁니다."},
		{file: "guides/OVERVIEW.en.md", old: "Jaekit holds on to three things and leaves the rest to the agent.",
			new: "Jaekit keeps hold of three things and leaves everything else to the agent."},
		{file: "guides/OPERATIONS.md", old: "이 지원 확인은 자동 설치·권한 확대·과거 기록 변경으로 우회하지 않습니다.",
			new: "자동 설치·권한 확대·과거 기록 변경으로 이 지원 확인을 우회하지 않습니다."},
		{file: "guides/OPERATIONS.en.md", old: "An input failure—a missing target file, corrupt input, a tool that cannot start, or termination by a signal—is never a pass.",
			new: "An input failure is never a pass: a missing target file, corrupt input, a tool that cannot start, or termination by a signal."},
		{file: "guides/SPAR.md", old: "세 CLI가 모두 있으면 이렇게 적을 수 있습니다.",
			new: "세 CLI가 모두 있다면 이렇게 적을 수 있습니다."},
		{file: "guides/SPAR.en.md", old: "With all three CLIs, you can list them like this:",
			new: "If all three CLIs are installed, you can list them like this:"},
		{file: "guides/CI.md", old: "교차 빌드 성공은 컴파일만 확인합니다.",
			new: "교차 빌드가 성공해도 확인되는 것은 컴파일뿐입니다."},
		{file: "guides/CI.en.md", old: "Cross-build success only establishes compilation.",
			new: "A successful cross-build establishes only compilation."},
	})
}

// AC-7: guide sentences that staying plugins tests pinned before the cleanup.
// Each edit breaks the exact fragment that was pinned.
func TestCleanupAC7_FormerPins(t *testing.T) {
	requirePass(t, "rewords guide sentences that plugins tests used to pin", []edit{
		{file: "guides/INSTALL.md", section: "## Release 파일로 설치",
			old: "아래 명령은 고치지 않고 그대로 붙여 넣습니다.", new: "아래 명령은 그대로, 고치지 않고 붙여 넣습니다."},
		{file: "guides/INSTALL.md", section: "### 버전 확인",
			old: "`which -a ha`의 첫 줄이 실제로 실행되는 파일입니다", new: "`which -a ha`가 먼저 보여 주는 줄이 실행되는 파일입니다"},
		{file: "guides/INSTALL.md", section: "### 버전 확인",
			old: "해당 파일을 지우거나 PATH 순서를 바꿉니다.", new: "그 파일을 지우거나, PATH에서 순서를 바꿉니다."},
		{file: "guides/INSTALL.md", section: "### Codex",
			old: "다른 플러그인과 혼동되면", new: "다른 플러그인과 헷갈리면"},
		{file: "guides/INSTALL.md", section: "## 로컬 경로 등록에서 옮기기",
			old: "spec·seal·spar도 함께 지워지므로 다시 설치합니다.", new: "spec·seal·spar까지 지워지므로 다시 설치합니다."},
		{file: "guides/USAGE.md", old: "같은 지침이 선택된다는 보장이 없습니다.", new: "같은 지침이 선택된다고 보장할 수 없습니다."},
		{file: "guides/USAGE.md", old: "그 자체로 구현을 시작하지 않습니다.", new: "이 표시만으로 구현이 시작되지는 않습니다."},
		{file: "guides/USAGE.md", old: "\"응\"이라고 답하면 구현이 시작됩니다.", new: "\"응\"이라고 답하면 바로 구현이 시작됩니다."},
		{file: "guides/USAGE.md", old: "구현 시작 명령도 계속 쓸 수 있습니다.", new: "구현 시작 명령도 여전히 쓸 수 있습니다."},
	})
}

// AC-7: every prose line of the root READMEs and guides reworded at once. Only
// the tests the user kept as exceptions may fail: those other kept records
// bind by name or assertion order.
func TestCleanupAC7_AllProse(t *testing.T) {
	requireControl(t)
	dir := syntheticTree(t, nil)
	if n := rewordProse(t, dir); n == 0 {
		t.Fatalf("setup: the copy has no prose line to reword")
	}
	syntheticCommit(t, dir, "reword prose")
	exempt := exceptionTests(t)
	var pinned []string
	seen := map[string]bool{}
	for _, name := range runPlugins(t, dir).failed {
		top, _, _ := strings.Cut(name, "/")
		if !seen[top] && !exempt(top) {
			pinned = append(pinned, top)
		}
		seen[top] = true
	}
	if len(pinned) > 0 {
		t.Errorf("jaekit:violation=prose-pinned: the plugins tests fail on a tree whose README and guide prose lines are all reworded: %s",
			strings.Join(pinned, ", "))
	}
}

// exceptionTests reports whether a top-level plugins test is one the user kept
// as an exception to AC-7: a test defined in a protected file, the
// compatibility tests that root declarations bind, or the uninstall test that
// a kept declaration runs by name.
func exceptionTests(t *testing.T) func(string) bool {
	t.Helper()
	root := repoRoot(t)
	protected := map[string]bool{}
	define := regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\) \{`)
	for path := range protectedFiles {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		names := define.FindAllStringSubmatch(string(readFile(t, root, path)), -1)
		if len(names) == 0 {
			t.Fatalf("setup: %s defines no test", path)
		}
		for _, name := range names {
			protected[name[1]] = true
		}
	}
	return func(name string) bool {
		return protected[name] || name == "TestRelease011UninstallGuide" ||
			strings.HasPrefix(name, "TestCompatibilityAC10_") || strings.HasPrefix(name, "TestCompatibilityAC11_")
	}
}

var (
	proseKeep  = regexp.MustCompile("`[^`]*`|!?\\[[^\\]]*\\]\\([^)]*\\)|https?://\\S+|<[^>]+>")
	boldLabel  = regexp.MustCompile(`^\*\*[^*]+\*\*:?\s*$`)
	readmeDocs = []string{"README.md", "README.ko.md", "README.en.md"}
)

// rewordProse puts " ~ " between every two words of every prose line of the
// root READMEs and guides/*.md in dir, and returns how many lines changed. A
// prose line is outside code fences and not a heading, table row, blank line
// or bold label; code spans, links, URLs and HTML tags inside it are kept.
func rewordProse(t *testing.T, dir string) int {
	t.Helper()
	guides, err := filepath.Glob(filepath.Join(dir, "guides", "*.md"))
	if err != nil || len(guides) == 0 {
		t.Fatalf("setup: the copy has no guides/*.md: %v", err)
	}
	docs := guides
	for _, name := range readmeDocs {
		docs = append(docs, filepath.Join(dir, name))
	}
	changed := 0
	for _, path := range docs {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("setup: read %s: %v", path, err)
		}
		lines := strings.Split(string(data), "\n")
		fence := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				fence = !fence
				continue
			}
			if fence || trimmed == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(trimmed, "|") || boldLabel.MatchString(trimmed) {
				continue
			}
			if reworded := rewordLine(line); reworded != line {
				lines[i] = reworded
				changed++
			}
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("setup: write %s: %v", path, err)
		}
	}
	return changed
}

func rewordLine(line string) string {
	var b strings.Builder
	last := 0
	for _, m := range proseKeep.FindAllStringIndex(line, -1) {
		b.WriteString(markWords(line[last:m[0]]))
		b.WriteString(line[m[0]:m[1]])
		last = m[1]
	}
	b.WriteString(markWords(line[last:]))
	return b.String()
}

// markWords replaces each single space between two letters with " ~ ".
func markWords(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r == ' ' && i > 0 && i+1 < len(runes) && unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1]) {
			b.WriteString(" ~ ")
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// namedTests are the plugins tests that kept declarations and the release
// check select by name.
var namedTests = []string{"TestRelease011UninstallGuide", "TestRelease011UpdateGuide", "TestRelease013ReferenceGuidance"}

// AC-13: each name is defined once in package plugins with its expected body.
func TestCleanupAC13_Bodies(t *testing.T) {
	root := repoRoot(t)
	found := map[string][]string{}
	for _, file := range trackedFiles(t, root) {
		rest, ok := strings.CutPrefix(file.path, "plugins/")
		if !ok || strings.Contains(rest, "/") || !strings.HasSuffix(rest, "_test.go") {
			continue
		}
		src := readFile(t, root, file.path)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file.path, src, 0)
		if err != nil {
			t.Fatalf("setup: parse %s: %v", file.path, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			if _, wanted := namedTestBodies[fn.Name.Name]; wanted {
				body := src[fset.Position(fn.Body.Lbrace).Offset : fset.Position(fn.Body.Rbrace).Offset+1]
				found[fn.Name.Name] = append(found[fn.Name.Name], file.path+" "+digest(body))
			}
		}
	}
	var problems []string
	for _, name := range namedTests {
		switch defs := found[name]; {
		case len(defs) != 1:
			problems = append(problems, name+" is defined "+itoa(len(defs))+" times: "+strings.Join(defs, ", "))
		case !strings.HasSuffix(defs[0], " "+namedTestBodies[name]):
			problems = append(problems, name+" in "+strings.Fields(defs[0])[0]+" has a changed body")
		}
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("the tests run by name are not kept as they were:", problems))
	}
}

// AC-13: the kept declarations and the release check select the three tests,
// and selecting them in ./plugins runs and passes each one.
func TestCleanupAC13_Selection(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	declared := map[string]bool{}
	for _, path := range []string{
		goalCheckRoot + "/release-0-1-3-followups/AC-3.targets.json",
		goalCheckRoot + "/release-0-1-3-followups/AC-6.targets.json",
		goalCheckRoot + "/release-0-1-3-review-fixes/AC-9.targets.json",
	} {
		var targets struct {
			Checks []struct {
				Runner, Package, Test string
			}
		}
		if err := json.Unmarshal(readFile(t, root, path), &targets); err != nil {
			t.Fatalf("setup: read %s: %v", path, err)
		}
		for _, check := range targets.Checks {
			if check.Runner != "go" {
				continue
			}
			if check.Package != "./plugins" {
				problems = append(problems, path+" runs "+check.Test+" in "+check.Package)
			}
			declared[check.Test] = true
		}
	}
	for _, name := range namedTests {
		if !declared[name] {
			problems = append(problems, "the kept declarations no longer run "+name)
		}
	}
	release := string(readFile(t, root, "tools/release-checks/check.py"))
	const selection = `"TestNestedCriteria|TestRelease011UninstallGuide|TestRelease011UpdateGuide)"`
	if !strings.Contains(release, selection) {
		problems = append(problems, "tools/release-checks/check.py lacks the selection "+selection)
	}
	// Every test a kept declaration names runs by name, the three and any
	// other one (a declaration also names a test of a staying file).
	names := sortedKeys(declared)
	byName := runPlugins(t, root, "-run", "^("+strings.Join(names, "|")+")$")
	releaseRun := runPlugins(t, root, "-run",
		"^(TestCompatibilityAC9_|TestBaselineBudget|TestBudget|TestBaselineSafetyAC5_|TestNestedCriteria|TestRelease011UninstallGuide|TestRelease011UpdateGuide)")
	for _, name := range names {
		if !byName.passed[name] {
			problems = append(problems, "selecting "+name+" by name does not run it to a pass")
		}
	}
	for _, name := range namedTests[:2] {
		if !releaseRun.passed[name] {
			problems = append(problems, "the release check's selection does not run "+name+" to a pass")
		}
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("the tests run by name are not selected and run:", problems))
	}
}

// AC-13: a tree whose Claude Code update command drops spar fails
// TestRelease011UpdateGuide.
func TestCleanupAC13_UpdateWithoutSpar(t *testing.T) {
	dir := syntheticTree(t, []edit{{
		file:    "guides/INSTALL.md",
		section: "## 업데이트",
		old:     "claude plugin install spar@jaekit\n",
		new:     "",
	}})
	run := runPlugins(t, dir, "-run", "^TestRelease011UpdateGuide$")
	failed := false
	for _, name := range run.failed {
		failed = failed || name == "TestRelease011UpdateGuide"
	}
	if !failed {
		t.Errorf("TestRelease011UpdateGuide passes on a tree whose Claude Code update command drops spar")
	}
}
