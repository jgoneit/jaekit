//go:build skillslim

// Package skillslimchecks holds the acceptance checks of the skill-slim goal.
// They read only the checked tree, the expected content in testdata/, and
// temporary copies of the checked tree that they build and edit themselves.
package skillslimchecks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// skill is one skill's directory; its SKILL.md and references/*.md are the
// skill's documents.
type skill struct {
	name string
	dir  string
}

var skills = []skill{
	{"spec", "plugins/spec/skills/spec"},
	{"spar", "plugins/spar/skills/spar"},
	{"seal", "plugins/seal/skills/seal"},
}

func skillNamed(name string) skill {
	for _, s := range skills {
		if s.name == name {
			return s
		}
	}
	panic("unknown skill " + name)
}

func (s skill) body() string { return s.dir + "/SKILL.md" }

// repoRoot is the checked tree: the nearest directory above this package that
// holds go.mod.
func repoRoot(t tb) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("setup: no go.mod above the check package")
		}
		dir = parent
	}
}

func readFile(t tb, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("setup: read %s: %v", rel, err)
	}
	return string(data)
}

func writeFile(t tb, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("setup: write %s: %v", rel, err)
	}
}

// expected reads one tracked expected-content file of this package into v.
func expected(t tb, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("setup: read testdata/%s: %v", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("setup: testdata/%s is not the expected JSON: %v", name, err)
	}
}

func gitOutput(t tb, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("setup: git %s: %v: %s", strings.Join(args, " "), err, errb.String())
	}
	return out.String()
}

type trackedFile struct{ path, mode string }

// trackedFiles lists the checked tree's tracked files from its index.
func trackedFiles(t tb, root string) []trackedFile {
	t.Helper()
	var files []trackedFile
	for _, entry := range strings.Split(gitOutput(t, root, "ls-files", "-s", "-z"), "\x00") {
		if entry == "" {
			continue
		}
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			t.Fatalf("setup: unreadable index entry %q", entry)
		}
		files = append(files, trackedFile{path: path, mode: fields[0]})
	}
	if len(files) == 0 {
		t.Fatalf("setup: the checked tree tracks no file")
	}
	return files
}

// docs returns a skill's documents in the tree at root, SKILL.md first, then
// references/*.md in name order, as slash-separated relative paths.
func docs(t tb, root string, s skill) []string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(s.body()))); err != nil {
		t.Fatalf("setup: %s: %v", s.body(), err)
	}
	out := []string{s.body()}
	refs, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(s.dir), "references", "*.md"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		out = append(out, s.dir+"/references/"+filepath.Base(ref))
	}
	return out
}

// words counts whitespace-separated words of the whole file, front matter
// included, as the Seal cap test counted at the goal's base.
func words(text string) int { return len(strings.Fields(text)) }

// line kinds of a Markdown document.
const (
	kindFront = "front"
	kindFence = "fence"
	kindHead  = "heading"
	kindRule  = "table-rule"
	kindBlank = "blank"
	kindProse = "prose"
)

var tableRule = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$`)

// classify sorts the lines of a document: YAML front matter at the start,
// fenced code blocks (``` or ~~~), headings, table rule rows, blank lines and
// everything else, which is prose. Table rows with text are prose.
func classify(text string) []string {
	lines := strings.Split(text, "\n")
	kinds := make([]string, len(lines))
	front := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case front:
			kinds[i] = kindFront
			if i > 0 && trimmed == "---" {
				front = false
			}
		case fence != "":
			kinds[i] = kindFence
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			kinds[i] = kindFence
			fence = trimmed[:3]
		case strings.HasPrefix(line, "#"):
			kinds[i] = kindHead
		case trimmed == "":
			kinds[i] = kindBlank
		case tableRule.MatchString(line):
			kinds[i] = kindRule
		default:
			kinds[i] = kindProse
		}
	}
	return kinds
}

func headingText(line string) string { return strings.TrimSpace(strings.TrimLeft(line, "#")) }

func headingLevel(line string) int { return len(line) - len(strings.TrimLeft(line, "#")) }

// codeRanges returns the byte ranges of the inline code spans of a line,
// delimiters included.
func codeRanges(line string) [][2]int {
	var out [][2]int
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		closing := strings.Index(line[i+n:], strings.Repeat("`", n))
		if closing < 0 {
			i += n
			continue
		}
		end := i + n + closing + n
		out = append(out, [2]int{i, end})
		i = end
	}
	return out
}

// codeText is a code span's content with white space runs read as one space.
func codeText(line string, r [2]int) string {
	n := 0
	for line[r[0]+n] == '`' {
		n++
	}
	return strings.Join(strings.Fields(line[r[0]+n:r[1]-n]), " ")
}

var (
	linkRe   = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	angleRe  = regexp.MustCompile(`<[^<>\s][^<>]*>`)
	bareURL  = regexp.MustCompile(`https?://[^\s)>]+`)
	anyLinks = regexp.MustCompile(`\]\([^)]*\)`)
)

// masked replaces the code spans of a line with spaces.
func masked(line string) string {
	b := []byte(line)
	for _, r := range codeRanges(line) {
		for i := r[0]; i < r[1]; i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

// linkTargets returns the link targets of a line outside code spans.
func linkTargets(line string) []string {
	var out []string
	for _, m := range linkRe.FindAllStringSubmatch(masked(line), -1) {
		out = append(out, m[1])
	}
	return out
}

// resolve turns a link target written in docPath into a slash-separated
// repository path with its anchor. External links stay as written; an anchor
// inside the file becomes docPath#anchor.
func resolve(docPath, target string) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return target
	}
	if strings.HasPrefix(target, "#") {
		return docPath + target
	}
	path, anchor, _ := strings.Cut(target, "#")
	joined := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(docPath), path)))
	if anchor != "" {
		return joined + "#" + anchor
	}
	return joined
}

// relink rewrites the relative link targets of a line written in from so
// that they point at the same places when the line is written in to.
func relink(line, from, to string) string {
	return linkRe.ReplaceAllStringFunc(line, func(m string) string {
		target := linkRe.FindStringSubmatch(m)[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
			return m
		}
		full := resolve(from, target)
		path, anchor, _ := strings.Cut(full, "#")
		rel, err := filepath.Rel(filepath.Dir(to), path)
		if err != nil {
			return m
		}
		rel = filepath.ToSlash(rel)
		if path == to {
			rel = ""
		}
		if anchor != "" {
			rel += "#" + anchor
		}
		return strings.Replace(m, target, rel, 1)
	})
}

// slugs returns GitHub's heading anchors of a Markdown document, with the
// numbered suffix GitHub gives repeated headings.
func slugs(text string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	lines := strings.Split(text, "\n")
	for i, kind := range classify(text) {
		if kind != kindHead {
			continue
		}
		s := slug(headingText(lines[i]))
		if n := seen[s]; n > 0 {
			out[fmt.Sprintf("%s-%d", s, n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

// slug is GitHub's heading anchor: link and code markup removed, lower case,
// spaces to hyphens, and every character other than a letter, digit, hyphen
// or underscore dropped.
func slug(heading string) string {
	heading = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(heading, "$1")
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127 && isLetterOrDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isLetterOrDigit(r rune) bool {
	return r >= 0xAC00 && r <= 0xD7A3 || r >= 0x3131 && r <= 0x318E || r >= 0x00C0 && r <= 0x024F
}

// tokenSet holds a skill's format tokens: code span contents, heading texts
// and resolved link targets.
type tokenSet struct {
	code, headings, links map[string]bool
}

func tokensOf(t tb, root string, s skill) tokenSet {
	t.Helper()
	set := tokenSet{code: map[string]bool{}, headings: map[string]bool{}, links: map[string]bool{}}
	for _, doc := range docs(t, root, s) {
		text := readFile(t, root, doc)
		lines := strings.Split(text, "\n")
		for i, kind := range classify(text) {
			if kind != kindProse && kind != kindHead {
				continue
			}
			if kind == kindHead {
				set.headings[headingText(lines[i])] = true
			}
			for _, r := range codeRanges(lines[i]) {
				set.code[codeText(lines[i], r)] = true
			}
			for _, target := range linkTargets(lines[i]) {
				set.links[resolve(doc, target)] = true
			}
		}
	}
	return set
}

// section returns the line indexes of a heading's section in text: from the
// heading line to the next heading of the same or a higher level. ok is false
// when no heading line equals heading.
func section(text, heading string) (start, end int, ok bool) {
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	level := headingLevel(heading)
	start = -1
	for i, kind := range kinds {
		if kind != kindHead {
			continue
		}
		if start < 0 {
			if strings.TrimSpace(lines[i]) == heading {
				start = i
			}
			continue
		}
		if headingLevel(lines[i]) <= level {
			return start, i, true
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	return start, len(lines), true
}

var itemStart = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])\s`)

// block returns the line range of the paragraph or list item that holds line
// i: a list item with its more indented continuation lines, or a run of prose
// lines up to a blank line, heading or fence.
func block(text string, i int) (start, end int) {
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	if m := itemStart.FindStringSubmatch(lines[i]); m != nil {
		indent := len(m[1])
		end = i + 1
		for end < len(lines) && kinds[end] == kindProse && itemStart.FindStringSubmatch(lines[end]) == nil {
			end++
		}
		for end < len(lines) && kinds[end] == kindProse {
			if n := itemStart.FindStringSubmatch(lines[end]); n != nil && len(n[1]) <= indent {
				break
			}
			end++
		}
		return i, end
	}
	start, end = i, i+1
	for start > 0 && kinds[start-1] == kindProse && itemStart.FindStringSubmatch(lines[start]) == nil {
		start--
	}
	for end < len(lines) && kinds[end] == kindProse && itemStart.FindStringSubmatch(lines[end]) == nil {
		end++
	}
	return start, end
}

// moveBlock removes lines [start, end) of from and appends them, with their
// relative links rewritten, as a new paragraph at the end of to.
func moveBlock(t tb, root, from string, start, end int, to string) {
	t.Helper()
	lines := strings.Split(readFile(t, root, from), "\n")
	moved := make([]string, 0, end-start)
	for _, l := range lines[start:end] {
		moved = append(moved, relink(l, from, to))
	}
	rest := append(append([]string{}, lines[:start]...), lines[end:]...)
	writeFile(t, root, from, strings.Join(rest, "\n"))
	target := strings.TrimRight(readFile(t, root, to), "\n")
	writeFile(t, root, to, target+"\n\n"+strings.Join(moved, "\n")+"\n")
}

// syntheticTree copies the tracked files of the checked tree at root into
// the empty directory dir and makes it a repository with one commit. It
// never changes the checked tree.
func syntheticTree(t tb, root, dir string) string {
	t.Helper()
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
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("setup: copy %s: %v", file.path, err)
			}
			perm := os.FileMode(0o644)
			if file.mode == "100755" {
				perm = 0o755
			}
			if err := os.WriteFile(dst, data, perm); err != nil {
				t.Fatalf("setup: copy %s: %v", file.path, err)
			}
		default:
			t.Fatalf("setup: %s has unsupported mode %s", file.path, file.mode)
		}
	}
	syntheticGit(t, dir, "init", "-q")
	syntheticCommit(t, dir, "copy of the checked tree")
	return dir
}

func syntheticGit(t tb, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "user.name=skill-slim-check", "-c", "user.email=skill-slim-check@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main", "-c", "core.hooksPath=" + os.DevNull}
	gitOutput(t, dir, append(base, args...)...)
}

func syntheticCommit(t tb, dir, message string) {
	t.Helper()
	syntheticGit(t, dir, "add", "-A")
	syntheticGit(t, dir, "commit", "-q", "--allow-empty", "-m", message)
}

// innerRun is the judged result of go test -json on ./plugins in a copy.
type innerRun struct {
	passed bool
	failed []string          // failed top-level tests, sorted
	output map[string]string // output of each top-level test and its subtests
}

// failedOutside returns the failed top-level tests that exempt does not name.
func (r innerRun) failedOutside(exempt func(string) bool) []string {
	var out []string
	for _, name := range r.failed {
		if exempt == nil || !exempt(name) {
			out = append(out, name)
		}
	}
	return out
}

// mentions reports whether some failed test's output holds every needle.
func (r innerRun) mentions(needles ...string) bool {
	for _, name := range r.failed {
		out, all := r.output[name], true
		for _, n := range needles {
			all = all && strings.Contains(out, n)
		}
		if all {
			return true
		}
	}
	return false
}

// slots bounds how many inner go test runs this process starts at once.
var slots = make(chan struct{}, 4)

// runPlugins runs go test -json on ./plugins in dir. A build failure, a run
// that does not start, a signal, or output that cannot be read is a setup
// failure; only passing or failing tests are an outcome.
func runPlugins(t tb, dir string) innerRun {
	t.Helper()
	slots <- struct{}{}
	defer func() { <-slots }()
	args := []string{"test", "-json", "-count=1", "-timeout=10m", "./plugins"}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS= ")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() < 0 {
			t.Fatalf("setup: go test ./plugins in a copy did not finish: %v: %s", err, tail(stderr.String()))
		}
		code = exit.ExitCode()
	}
	run := innerRun{output: map[string]string{}}
	pkgAction, buildFailed := "", false
	seen := map[string]bool{}
	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 1<<20), 1<<26)
	for scanner.Scan() {
		var ev struct{ Action, Test, Output string }
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("setup: unreadable go test -json line %q: %v", tail(scanner.Text()), err)
		}
		top, _, _ := strings.Cut(ev.Test, "/")
		switch {
		case ev.Action == "build-fail":
			buildFailed = true
		case ev.Test == "" && (ev.Action == "pass" || ev.Action == "fail"):
			pkgAction = ev.Action
		case ev.Test != "" && ev.Action == "output":
			run.output[top] += ev.Output
		case ev.Test != "" && ev.Action == "fail" && !seen[top]:
			seen[top] = true
			run.failed = append(run.failed, top)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("setup: read go test output: %v", err)
	}
	t.Logf("go test ./plugins in %s took %s", filepath.Base(dir), time.Since(started).Round(time.Second))
	sort.Strings(run.failed)
	switch {
	case !buildFailed && code == 0 && pkgAction == "pass" && len(run.failed) == 0:
		run.passed = true
	case !buildFailed && code == 1 && pkgAction == "fail" && len(run.failed) > 0:
	default:
		t.Fatalf("setup: go test ./plugins in a copy ended with exit %d, package %q, build failed %t, failed %v: %s",
			code, pkgAction, buildFailed, run.failed, tail(stderr.String()+stdout.String()))
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

// tb is what the helpers need from a test. *testing.T has it; inside the
// goroutines of runEdited, goT turns Fatalf into a panic that runEdited
// recovers and reports from the test goroutine.
type tb interface {
	Helper()
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

type setupFailure string

type goT struct{ t *testing.T }

func (g goT) Helper()                           {}
func (g goT) Logf(format string, args ...any)   { g.t.Logf(format, args...) }
func (g goT) Fatalf(format string, args ...any) { panic(setupFailure(fmt.Sprintf(format, args...))) }

// The unchanged copy must pass before an edited copy says anything.
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
	if run := runPlugins(t, syntheticTree(t, repoRoot(t), t.TempDir())); !run.passed {
		controlState = "the unchanged copy fails " + strings.Join(run.failed, ", ")
		t.Fatalf("setup: the plugins tests do not pass on an unchanged copy of the checked tree: %s", controlState)
	}
	controlState = "passed"
}

// edited is one copy of the checked tree with change applied, and the result
// of the plugins tests in it.
type edited struct {
	what   string
	change func(t tb, dir string)
	run    innerRun
}

// runEdited builds every copy, applies its change, commits and runs the
// plugins tests there, several at once.
func runEdited(t *testing.T, trees []*edited) {
	t.Helper()
	requireControl(t)
	root := repoRoot(t)
	dirs := make([]string, len(trees))
	for i := range trees {
		dirs[i] = t.TempDir()
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	for i, tree := range trees {
		wg.Add(1)
		go func(tree *edited, dir string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					msg, ok := r.(setupFailure)
					if !ok {
						msg = setupFailure(fmt.Sprint(r))
					}
					mu.Lock()
					failures = append(failures, tree.what+": "+string(msg))
					mu.Unlock()
				}
			}()
			g := goT{t}
			syntheticTree(g, root, dir)
			tree.change(g, dir)
			syntheticCommit(g, dir, tree.what)
			tree.run = runPlugins(g, dir)
		}(tree, dirs[i])
	}
	wg.Wait()
	if len(failures) > 0 {
		sort.Strings(failures)
		t.Fatalf("setup: %s", strings.Join(failures, "; "))
	}
}
