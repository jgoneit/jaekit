package main

// Scenario tests run ha against small git repositories built from
// testdata/scenarios. Each scenario directory holds a `steps` file (one
// command per line) and optionally `files/`, which is laid over the shared
// `_base` repository before the base commit. Shared step sequences live in
// `_steps/` and are pulled in with `include <name>`. The steps are listed in
// testdata/scenarios/README.md.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/status"
)

var scenarioRoot = "../../testdata/scenarios"

const scenarioGoal = "docs/specs/greet"

type scenario struct {
	t        *testing.T
	name     string
	root     string // the repository's main checkout
	dir      string // the working tree steps run in
	path     string // PATH with the test runner
	bare     string // PATH without it
	lastOut  string
	lastCode int
	allOut   strings.Builder
}

func TestScenarios(t *testing.T) {
	abs, err := filepath.Abs(scenarioRoot)
	if err != nil {
		t.Fatal(err)
	}
	scenarioRoot = abs
	entries, err := os.ReadDir(scenarioRoot)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		n++
		name := e.Name()
		t.Run(name, func(t *testing.T) { runScenario(t, name) })
	}
	if n == 0 {
		t.Fatal("no scenarios found")
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runScenario(t *testing.T, name string) {
	s := &scenario{t: t, name: name, dir: t.TempDir()}
	s.root = s.dir
	copyTree(t, filepath.Join(scenarioRoot, "_base"), s.dir)
	if _, err := os.Stat(filepath.Join(scenarioRoot, name, "files")); err == nil {
		copyTree(t, filepath.Join(scenarioRoot, name, "files"), s.dir)
	}
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(cfg, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "scenario")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "scenario@example.invalid")
	}
	bin := t.TempDir()
	runner := filepath.Join(bin, "jaekit-test-runner")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\nexec sh \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.bare = os.Getenv("PATH")
	s.path = bin + string(os.PathListSeparator) + s.bare
	t.Setenv("PATH", s.path)
	s.git("init", "-q", "-b", "main")
	s.git("add", "-A")
	s.git("commit", "-q", "-m", "base")
	t.Chdir(s.dir)
	steps := s.load(filepath.Join(scenarioRoot, name, "steps"), 0)
	for _, st := range steps {
		s.do(st)
	}
}

type step struct {
	where string
	args  []string
}

func (s *scenario) load(file string, depth int) []step {
	if depth > 3 {
		s.t.Fatalf("include nesting too deep at %s", file)
	}
	f, err := os.Open(file)
	if err != nil {
		s.t.Fatal(err)
	}
	defer f.Close()
	var out []step
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		args, err := tokenize(line)
		if err != nil {
			s.t.Fatalf("%s:%d: %v", file, n, err)
		}
		for i := range args {
			args[i] = strings.ReplaceAll(args[i], "$GOAL", scenarioGoal)
		}
		if args[0] == "include" {
			out = append(out, s.load(filepath.Join(scenarioRoot, "_steps", args[1]), depth+1)...)
			continue
		}
		out = append(out, step{where: fmt.Sprintf("%s:%d", filepath.Base(filepath.Dir(file))+"/"+filepath.Base(file), n), args: args})
	}
	return out
}

// tokenize splits a step line into words. Double quotes group words and
// understand \n, \t, \", and \\.
func tokenize(line string) ([]string, error) {
	var out []string
	var b strings.Builder
	in, quoted := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted && c == '\\' && i+1 < len(line):
			i++
			switch line[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(line[i])
			}
		case quoted && c == '"':
			quoted = false
		case quoted:
			b.WriteByte(c)
		case c == '"':
			quoted, in = true, true
		case c == ' ' || c == '\t':
			if in {
				out = append(out, b.String())
				b.Reset()
				in = false
			}
		default:
			b.WriteByte(c)
			in = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("unclosed quote")
	}
	if in {
		out = append(out, b.String())
	}
	return out, nil
}

func (s *scenario) fatalf(st step, format string, a ...any) {
	s.t.Helper()
	s.t.Fatalf("%s: %s: "+format, append([]any{st.where, strings.Join(st.args, " ")}, a...)...)
}

func (s *scenario) git(args ...string) string {
	s.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = s.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (s *scenario) ha(args ...string) {
	var out, errb bytes.Buffer
	s.lastCode = runMain(args, &out, &errb)
	s.lastOut = out.String() + errb.String()
	s.allOut.WriteString(out.String())
}

func (s *scenario) report(st step) *status.Report {
	s.t.Helper()
	var out, errb bytes.Buffer
	code := runMain([]string{"status", scenarioGoal, "--format", "json"}, &out, &errb)
	var rep status.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		s.fatalf(st, "status json: %v (exit %d, stderr %s)", err, code, errb.String())
	}
	if code != rep.ExitCode {
		s.fatalf(st, "exit %d but report says %d", code, rep.ExitCode)
	}
	s.allOut.WriteString(out.String())
	return &rep
}

func activeReasons(rep *status.Report) []string {
	var out []string
	for _, r := range rep.Reasons {
		if r.Criterion != "" {
			out = append(out, r.Criterion+":"+r.Code)
		} else {
			out = append(out, r.Code)
		}
	}
	sort.Strings(out)
	return out
}

func (s *scenario) file(rel string) string { return filepath.Join(s.dir, filepath.FromSlash(rel)) }

func (s *scenario) records(st step) [][]byte {
	data, err := os.ReadFile(s.file(scenarioGoal + "/runs.jsonl"))
	if err != nil {
		s.fatalf(st, "%v", err)
	}
	return bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
}

var prevField = regexp.MustCompile(`"prev":(null|"[0-9a-f]*")`)

// rechain rewrites prev values so an edited record stays a valid chain, as a
// careful forger (or this test) would.
func (s *scenario) writeRecords(st step, lines [][]byte, rechain bool) {
	if rechain {
		for i := 1; i < len(lines); i++ {
			h := sha256.Sum256(lines[i-1])
			lines[i] = prevField.ReplaceAll(lines[i], []byte(`"prev":"`+hex.EncodeToString(h[:])+`"`))
		}
	}
	data := append(bytes.Join(lines, []byte("\n")), '\n')
	if err := os.WriteFile(s.file(scenarioGoal+"/runs.jsonl"), data, 0o644); err != nil {
		s.fatalf(st, "%v", err)
	}
}

func fieldPattern(field string) *regexp.Regexp {
	return regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `":("(?:[^"\\]|\\.)*"|null|true|false|-?[0-9]+|\{[^{}]*\})`)
}

func (s *scenario) progress(st step) {
	var b strings.Builder
	b.WriteString("# PROGRESS — greet\n\n## 현재\n시나리오가 쓴 진행 기록\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n")
	for i := 1; i+1 < len(st.args); i += 2 {
		fmt.Fprintf(&b, "| %s | %s | |\n", st.args[i], st.args[i+1])
	}
	if err := os.WriteFile(s.file(scenarioGoal+"/PROGRESS.md"), []byte(b.String()), 0o644); err != nil {
		s.fatalf(st, "%v", err)
	}
}

func (s *scenario) do(st step) {
	s.t.Helper()
	a := st.args
	need := func(n int) {
		if len(a) < n {
			s.fatalf(st, "needs %d arguments", n-1)
		}
	}
	switch a[0] {
	case "ha":
		s.ha(a[1:]...)
	case "ha-without-runner":
		os.Setenv("PATH", s.bare)
		s.ha(a[1:]...)
		os.Setenv("PATH", s.path)
	case "write", "append":
		need(3)
		p := s.file(a[1])
		os.MkdirAll(filepath.Dir(p), 0o755)
		flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if a[0] == "append" {
			flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, err := os.OpenFile(p, flag, 0o644)
		if err != nil {
			s.fatalf(st, "%v", err)
		}
		f.WriteString(a[2])
		f.Close()
	case "replace":
		need(4)
		data, err := os.ReadFile(s.file(a[1]))
		if err != nil || !strings.Contains(string(data), a[2]) {
			s.fatalf(st, "text to replace not found")
		}
		os.WriteFile(s.file(a[1]), []byte(strings.Replace(string(data), a[2], a[3], 1)), 0o644)
	case "rm":
		need(2)
		if err := os.Remove(s.file(a[1])); err != nil {
			s.fatalf(st, "%v", err)
		}
	case "commit":
		need(2)
		s.git("add", "-A")
		s.git("commit", "-q", "--allow-empty", "-m", a[1])
	case "progress":
		s.progress(st)
	case "worktree":
		// Later steps run in a new linked worktree on a new branch from HEAD.
		// Its git directory lies outside the working tree.
		need(2)
		wt := filepath.Join(s.t.TempDir(), a[1])
		s.git("worktree", "add", "-q", "-b", a[1], wt)
		s.dir = wt
		s.t.Chdir(wt)
	case "drop-record":
		need(2)
		n, _ := strconv.Atoi(a[1])
		lines := s.records(st)
		lines = append(lines[:n-1], lines[n:]...)
		s.writeRecords(st, lines, false)
	case "rewrite-record":
		need(4)
		n, _ := strconv.Atoi(a[1])
		lines := s.records(st)
		re := fieldPattern(a[2])
		if !re.Match(lines[n-1]) {
			s.fatalf(st, "field not found in seq %d", n)
		}
		lines[n-1] = re.ReplaceAll(lines[n-1], []byte(`"`+a[2]+`":`+a[3]))
		s.writeRecords(st, lines, true)
	case "shift-time":
		need(3)
		n, _ := strconv.Atoi(a[1])
		d, err := time.ParseDuration(a[2])
		if err != nil {
			s.fatalf(st, "%v", err)
		}
		lines := s.records(st)
		re := fieldPattern("at")
		for i := n - 1; i < len(lines); i++ {
			m := re.FindSubmatch(lines[i])
			at, _ := time.Parse(time.RFC3339, strings.Trim(string(m[1]), `"`))
			lines[i] = re.ReplaceAll(lines[i], []byte(`"at":"`+at.Add(d).UTC().Format(time.RFC3339)+`"`))
		}
		s.writeRecords(st, lines, true)
	case "expect-exit":
		need(2)
		if want, _ := strconv.Atoi(a[1]); s.lastCode != want {
			s.fatalf(st, "exit %d, want %d\n%s", s.lastCode, want, s.lastOut)
		}
	case "expect-output":
		need(2)
		if !strings.Contains(s.lastOut, a[1]) {
			s.fatalf(st, "output does not contain %q:\n%s", a[1], s.lastOut)
		}
	case "expect-no-output":
		need(2)
		if strings.Contains(s.lastOut, a[1]) {
			s.fatalf(st, "output contains %q:\n%s", a[1], s.lastOut)
		}
	case "expect-status":
		need(2)
		rep := s.report(st)
		if rep.Status != a[1] || rep.ExitCode != status.ExitCode(a[1]) {
			s.fatalf(st, "status %s (exit %d), reasons %v", rep.Status, rep.ExitCode, activeReasons(rep))
		}
	case "expect-reasons":
		rep := s.report(st)
		want := append([]string(nil), a[1:]...)
		if len(want) == 1 && want[0] == "none" {
			want = nil
		}
		sort.Strings(want)
		got := activeReasons(rep)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			s.fatalf(st, "reasons %v, want %v", got, want)
		}
	case "expect-reason", "expect-no-reason":
		need(2)
		key := a[1]
		if len(a) > 2 {
			key = a[2] + ":" + a[1]
		}
		found := false
		for _, r := range activeReasons(s.report(st)) {
			found = found || r == key
		}
		if found != (a[0] == "expect-reason") {
			s.fatalf(st, "reason %s present = %v", key, found)
		}
	case "expect-satisfied", "expect-unsatisfied":
		need(2)
		for _, c := range s.report(st).Criteria {
			if c.ID == a[1] {
				if c.Satisfied != (a[0] == "expect-satisfied") {
					s.fatalf(st, "satisfied = %v, reason %s", c.Satisfied, c.Reason)
				}
				return
			}
		}
		s.fatalf(st, "criterion not in report")
	case "expect-attempt":
		need(4)
		for _, c := range s.report(st).Criteria {
			if c.ID != a[1] {
				continue
			}
			b, _ := json.Marshal(c.Attempts)
			var m map[string]any
			json.Unmarshal(b, &m)
			if fmt.Sprint(m[a[2]]) != a[3] {
				s.fatalf(st, "attempts.%s = %v", a[2], m[a[2]])
			}
			return
		}
		s.fatalf(st, "criterion not in report")
	case "expect-completion-record":
		need(2)
		rep := s.report(st)
		if (rep.CompletionRecord != nil) != (a[1] == "present") {
			s.fatalf(st, "completion record = %v", rep.CompletionRecord)
		}
	case "expect-budget-window":
		need(2)
		rep := s.report(st)
		if want, _ := strconv.Atoi(a[1]); rep.Budget == nil || rep.Budget.WindowStart != want {
			s.fatalf(st, "budget = %+v", rep.Budget)
		}
	case "expect-change":
		need(3)
		ch := s.report(st).Changes
		var have []string
		switch a[1] {
		case "command_changed":
			for _, c := range ch.CommandChanged {
				have = append(have, c.Criterion)
			}
		case "kind_changed":
			for _, c := range ch.KindChanged {
				have = append(have, c.Criterion)
			}
		case "optional":
			have = ch.Optional
		case "invalid_confirmations":
			for _, c := range ch.InvalidConfirmations {
				have = append(have, c.Subject)
			}
		case "ha_version_changed":
			for _, c := range ch.HaVersionChanged {
				have = append(have, strconv.Itoa(c.Seq))
			}
		default:
			s.fatalf(st, "unknown change list")
		}
		for _, h := range have {
			if h == a[2] {
				return
			}
		}
		s.fatalf(st, "%s = %v", a[1], have)
	case "expect-record":
		need(4)
		n, _ := strconv.Atoi(a[1])
		lines := s.records(st)
		if n > len(lines) {
			s.fatalf(st, "only %d records", len(lines))
		}
		var m map[string]json.RawMessage
		json.Unmarshal(lines[n-1], &m)
		got := string(m[a[2]])
		if got != a[3] && got != strconv.Quote(a[3]) {
			s.fatalf(st, "seq %d %s = %s", n, a[2], got)
		}
	case "expect-records":
		need(2)
		got := 0
		if _, err := os.Stat(s.file(scenarioGoal + "/runs.jsonl")); err == nil {
			got = len(s.records(st))
		}
		if want, _ := strconv.Atoi(a[1]); got != want {
			s.fatalf(st, "%d records, want %d", got, want)
		}
	case "expect-absent":
		need(2)
		rec, _ := os.ReadFile(s.file(scenarioGoal + "/runs.jsonl"))
		var md bytes.Buffer
		runMain([]string{"status", scenarioGoal}, &md, &bytes.Buffer{})
		for where, text := range map[string]string{"runs.jsonl": string(rec), "ha output": s.allOut.String(), "status md": md.String()} {
			if strings.Contains(text, a[1]) {
				s.fatalf(st, "%q found in %s", a[1], where)
			}
		}
	case "expect-no-local-paths":
		// No machine-specific directory of this repository appears in the
		// records, in any ha output so far, or in status as Markdown or JSON.
		rec, _ := os.ReadFile(s.file(scenarioGoal + "/runs.jsonl"))
		var md, js bytes.Buffer
		runMain([]string{"status", scenarioGoal}, &md, &bytes.Buffer{})
		runMain([]string{"status", scenarioGoal, "--format", "json"}, &js, &bytes.Buffer{})
		var dirs []string
		for _, d := range []string{s.root, s.dir} {
			dirs = append(dirs, d)
			if r, err := filepath.EvalSymlinks(d); err == nil && r != d {
				dirs = append(dirs, r)
			}
		}
		for where, text := range map[string]string{"runs.jsonl": string(rec), "ha output": s.allOut.String(), "status md": md.String(), "status json": js.String()} {
			for _, d := range dirs {
				if strings.Contains(text, d) {
					s.fatalf(st, "local path %s found in %s", d, where)
				}
			}
		}
	case "expect-deterministic":
		var first, second bytes.Buffer
		runMain([]string{"status", scenarioGoal, "--format", "json"}, &first, &bytes.Buffer{})
		time.Sleep(1100 * time.Millisecond)
		runMain([]string{"status", scenarioGoal, "--format", "json"}, &second, &bytes.Buffer{})
		if !bytes.Equal(first.Bytes(), second.Bytes()) {
			s.fatalf(st, "status output changed between runs")
		}
	case "expect-untouched":
		out := s.git("status", "--porcelain", "--untracked-files=all")
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l != "" && !strings.HasSuffix(l, scenarioGoal+"/runs.jsonl") {
				s.fatalf(st, "unexpected change in the working tree: %s", l)
			}
		}
	default:
		s.fatalf(st, "unknown step")
	}
}

// Every scenario directory is listed in testdata/scenarios/README.md and
// every listed scenario exists, so the table stays a faithful index.
func TestScenarioIndex(t *testing.T) {
	root, _ := filepath.Abs("../../testdata/scenarios")
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z0-9-]+)` \\|").FindAllStringSubmatch(string(readme), -1) {
		listed[m[1]] = true
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), "_") && !listed[e.Name()] {
			t.Errorf("scenario %s is not listed in README.md", e.Name())
		}
		delete(listed, e.Name())
	}
	for name := range listed {
		t.Errorf("README.md lists %s, which has no directory", name)
	}
}
