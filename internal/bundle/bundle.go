// Package bundle reads the execution bundle (contracts/bundle.md) of a
// goal directory and lints it against the goal documents. Only the parts that
// ha must read have a fixed format; everything else is free Markdown.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/diag"
	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/markdown"
)

// Criterion kinds.
const (
	KindChange   = "change"
	KindMaintain = "maintain"
	KindManual   = "manual"
)

// Task states in PROGRESS.md.
var TaskStates = []string{"todo", "doing", "done", "dropped", "blocked"}

// Row is one row of `## 조건표`.
type Row struct {
	ID         string
	Kind       string
	Command    string // plan command string; empty for manual rows
	Argv       []string
	Reason     string // manual rows: why no command
	CheckPaths []string
	Tasks      []string
	Line       int
}

// CommandDigest is the sha256 of the plan command string.
func (r Row) CommandDigest() string { return Digest(r.Command) }

// Executor reports whether the row is an executor-added EX-n condition. Such
// conditions are always optional.
func (r Row) Executor() bool { return strings.HasPrefix(r.ID, "EX-") }

// Task is one row of `## Task`, plus the commands from its task document.
type Task struct {
	ID       string
	Goal     string
	Criteria []string
	Proposal string
	Deps     []string
	Required bool
	Doc      string // goal-relative path or ""
	Commands []string
	Line     int
}

// Scope is `## 범위`.
type Scope struct {
	Allowed []string
	Tests   []string
	// OtherBundlesInput is set by the `다른 목표 실행 묶음: `검사 입력`` item:
	// this goal's checks read other goals' bundle files, so under run-rules/2
	// those files stay in the code state.
	OtherBundlesInput bool
}

// The `## 범위` item that declares checks reading other goals' bundle files,
// and its only value. Both are contract tokens.
const (
	OtherBundlesKey   = "다른 목표 실행 묶음"
	OtherBundlesInput = "검사 입력"
)

// Budget is `## 예산`.
type Budget struct {
	Runs           int
	ElapsedSeconds int64
	Cost           string
}

// Bundle is the parsed execution bundle.
type Bundle struct {
	Dir      string
	HasPlan  bool
	Rows     []Row
	Tasks    []Task
	Scope    Scope
	Budget   Budget
	Progress map[string]string // task ID -> state
	Problems []diag.Problem
}

// Row returns the plan row for a criterion ID, or nil.
func (b *Bundle) Row(id string) *Row {
	for i := range b.Rows {
		if b.Rows[i].ID == id {
			return &b.Rows[i]
		}
	}
	return nil
}

// Task returns the task with the given ID, or nil.
func (b *Bundle) Task(id string) *Task {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i]
		}
	}
	return nil
}

// Digest is the hex sha256 of s.
func Digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// DocPaths returns the bundle documents of a goal: the goal document set plus
// PLAN.md, REVIEW.md, PROGRESS.md, tasks/, and runs.jsonl. They are excluded
// from the code state.
func DocPaths(g *goaldocs.Goal) []string {
	out := append([]string(nil), g.Docs...)
	if len(out) == 0 {
		out = append(out, g.Dir+"/SPEC.md")
	}
	for _, n := range goaldocs.BundleNames {
		out = append(out, g.Dir+"/"+n)
	}
	sort.Strings(out)
	return out
}

// OtherGoalBundles returns the bundle files of the other goals next to g:
// every other directory under g's parent that holds a SPEC.md in the working
// tree, with its PLAN.md, REVIEW.md, PROGRESS.md, runs.jsonl, and tasks/.
// Other files there, the goal documents and checks/ among them, are code.
func OtherGoalBundles(root string, g *goaldocs.Goal) ([]string, error) {
	parent := path.Dir(g.Dir)
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(parent)))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		dir := path.Join(parent, e.Name())
		if !e.IsDir() || dir == g.Dir {
			continue
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), "SPEC.md"))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		for _, n := range goaldocs.BundleNames {
			out = append(out, dir+"/"+n)
		}
	}
	sort.Strings(out)
	return out, nil
}

var (
	rowID       = regexp.MustCompile(`^(AC|EX)-\d+$`)
	criterionID = regexp.MustCompile(`(?:AC|EX)-\d+`)
	taskID      = regexp.MustCompile(`^T\d{3,}$`)
	taskRef     = regexp.MustCompile(`T\d{3,}`)
	proposalRef = regexp.MustCompile(`^W\d+$`)
	firstInt    = regexp.MustCompile(`\d+`)
	soleSpan    = regexp.MustCompile("^`[^`]+`$")
	hoursKo     = regexp.MustCompile(`(\d+)\s*시간`)
	minutesKo   = regexp.MustCompile(`(\d+)\s*분`)
)

type loader struct {
	root string
	g    *goaldocs.Goal
	b    *Bundle
}

func (l *loader) problem(code, file string, line int, format string, a ...any) {
	l.b.Problems = append(l.b.Problems, diag.Problem{Code: code, File: file, Line: line, Detail: fmt.Sprintf(format, a...)})
}

func (l *loader) read(rel string) (string, bool, error) {
	b, err := os.ReadFile(filepath.Join(l.root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// Load reads the bundle of goal g under root. Format problems are collected
// in Bundle.Problems; err is only for I/O failures.
func Load(root string, g *goaldocs.Goal) (*Bundle, error) {
	l := &loader{root: root, g: g, b: &Bundle{Dir: g.Dir, Progress: map[string]string{}}}
	planPath := g.Dir + "/PLAN.md"
	src, ok, err := l.read(planPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		l.problem("plan_missing", planPath, 0, "the goal directory has no PLAN.md")
	} else {
		l.b.HasPlan = true
		doc := markdown.Parse(src)
		l.readRows(doc, planPath)
		l.readScope(doc, planPath)
		if err := l.readTasks(doc, planPath); err != nil {
			return nil, err
		}
		l.readBudget(doc, planPath)
	}
	for _, name := range []string{"REVIEW.md", "PROGRESS.md"} {
		_, ok, err := l.read(g.Dir + "/" + name)
		if err != nil {
			return nil, err
		}
		if !ok {
			l.problem("bundle_file_missing", g.Dir+"/"+name, 0, "the bundle has no %s", name)
		}
	}
	if err := l.readProgress(); err != nil {
		return nil, err
	}
	if l.b.HasPlan {
		l.crossCheck(planPath)
	}
	return l.b, nil
}

func (l *loader) section(doc *markdown.Doc, file, title string) ([]markdown.Line, bool) {
	lines, ok := doc.Section(2, title)
	if !ok {
		l.problem("plan_section_missing", file, 0, "PLAN.md has no `## %s` section", title)
	}
	return lines, ok
}

func (l *loader) table(lines []markdown.Line, file, title string, cols ...string) (markdown.Table, bool) {
	tables := markdown.Tables(lines)
	if len(tables) == 0 {
		l.problem("plan_section_missing", file, 0, "`## %s` has no table", title)
		return markdown.Table{}, false
	}
	if !tables[0].HeaderIs(cols...) {
		l.problem("table_columns", file, tables[0].Line, "`## %s` columns must be %s", title, strings.Join(cols, " | "))
		return markdown.Table{}, false
	}
	return tables[0], true
}

func splitList(cell string) []string {
	var out []string
	for _, part := range strings.Split(cell, ",") {
		p := strings.TrimSpace(strings.Trim(strings.TrimSpace(part), "`"))
		if !markdown.IsNone(p) {
			out = append(out, p)
		}
	}
	return out
}

func (l *loader) readRows(doc *markdown.Doc, file string) {
	lines, ok := l.section(doc, file, "조건표")
	if !ok {
		return
	}
	tb, ok := l.table(lines, file, "조건표", "ID", "종류", "검증 명령", "검사 경로", "Task")
	if !ok {
		return
	}
	for _, r := range tb.Rows {
		row := Row{ID: strings.TrimSpace(r.Cell(0)), Kind: strings.TrimSpace(r.Cell(1)), Line: r.Line}
		if !rowID.MatchString(row.ID) {
			l.problem("criterion_invalid_id", file, r.Line, "condition ID must be AC-n or EX-n: %q", row.ID)
			continue
		}
		switch row.Kind {
		case KindChange, KindMaintain, KindManual:
		default:
			l.problem("invalid_kind", file, r.Line, "%s kind must be change, maintain, or manual: %q", row.ID, row.Kind)
		}
		cmdCell := r.Cell(2)
		spans := markdown.CodeSpans(cmdCell)
		if row.Kind == KindManual {
			row.Reason = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(cmdCell), "-—"))
		} else if len(spans) > 0 {
			row.Command = strings.TrimSpace(spans[0])
			if len(spans) > 1 {
				l.problem("command_multiple", file, r.Line, "%s has more than one command; put them in a script", row.ID)
			}
			argv, err := SplitCommand(row.Command)
			switch {
			case errors.Is(err, ErrShellSyntax):
				l.problem("shell_syntax", file, r.Line, "%s command uses shell syntax (pipes, redirects, &&, ;, $); ha runs commands without a shell: %s", row.ID, row.Command)
			case err != nil:
				l.problem("command_parse", file, r.Line, "%s command cannot be split into words: %v", row.ID, err)
			default:
				row.Argv = argv
			}
		}
		for _, p := range splitList(r.Cell(3)) {
			if strings.ContainsAny(p, "*?[") {
				l.problem("check_path_glob", file, r.Line, "%s check path must be a file path, not a glob: %s", row.ID, p)
				continue
			}
			row.CheckPaths = append(row.CheckPaths, path.Clean(p))
		}
		row.Tasks = taskRef.FindAllString(r.Cell(4), -1)
		l.b.Rows = append(l.b.Rows, row)
	}
}

func (l *loader) readScope(doc *markdown.Doc, file string) {
	lines, ok := l.section(doc, file, "범위")
	if !ok {
		return
	}
	hasAllowed := false
	for _, it := range markdown.Items(lines) {
		text := it.Text
		switch {
		case strings.HasPrefix(text, "바꿀 수 있는 경로"):
			hasAllowed = true
			l.b.Scope.Allowed = append(l.b.Scope.Allowed, markdown.CodeSpans(text)...)
		case strings.HasPrefix(text, "테스트 경로"):
			l.b.Scope.Tests = append(l.b.Scope.Tests, markdown.CodeSpans(text)...)
		case strings.HasPrefix(text, OtherBundlesKey):
			spans := markdown.CodeSpans(text)
			if strings.HasPrefix(text, OtherBundlesKey+":") && len(spans) == 1 && strings.TrimSpace(spans[0]) == OtherBundlesInput {
				l.b.Scope.OtherBundlesInput = true
				continue
			}
			l.problem("other_bundles_invalid", file, it.Line, "`%s:` takes one code span, `%s`, when this goal's checks read other goals' bundle files; otherwise leave the item out", OtherBundlesKey, OtherBundlesInput)
		}
	}
	if !hasAllowed || len(l.b.Scope.Allowed) == 0 {
		l.problem("scope_missing", file, 0, "`## 범위` must list `바꿀 수 있는 경로` as code spans")
	}
	if len(l.b.Scope.Tests) == 0 {
		l.b.Scope.Tests = append([]string(nil), DefaultTestPaths...)
	}
}

func (l *loader) readTasks(doc *markdown.Doc, file string) error {
	lines, ok := l.section(doc, file, "Task")
	if !ok {
		return nil
	}
	tb, ok := l.table(lines, file, "Task", "ID", "목표", "근거", "제안", "선행", "필수", "문서")
	if !ok {
		return nil
	}
	for _, r := range tb.Rows {
		t := Task{ID: strings.TrimSpace(r.Cell(0)), Goal: r.Cell(1), Line: r.Line}
		if !taskID.MatchString(t.ID) {
			l.problem("task_invalid_id", file, r.Line, "task ID must look like T001: %q", t.ID)
			continue
		}
		t.Criteria = criterionID.FindAllString(r.Cell(2), -1)
		if p := strings.TrimSpace(r.Cell(3)); !markdown.IsNone(p) {
			t.Proposal = p
		}
		if !markdown.IsNone(r.Cell(4)) {
			t.Deps = taskRef.FindAllString(r.Cell(4), -1)
		}
		switch strings.TrimSpace(r.Cell(5)) {
		case "예":
			t.Required = true
		case "아니오":
		default:
			l.problem("task_required_invalid", file, r.Line, "%s 필수 must be 예 or 아니오", t.ID)
		}
		if d := strings.Trim(strings.TrimSpace(r.Cell(6)), "`"); !markdown.IsNone(d) {
			t.Doc = path.Clean(d)
			if err := l.readTaskDoc(&t); err != nil {
				return err
			}
		}
		l.b.Tasks = append(l.b.Tasks, t)
	}
	return nil
}

func (l *loader) readTaskDoc(t *Task) error {
	rel := l.g.Dir + "/" + t.Doc
	src, ok, err := l.read(rel)
	if err != nil {
		return err
	}
	if !ok {
		l.problem("task_doc_missing", rel, 0, "%s refers to %s, which does not exist", t.ID, t.Doc)
		return nil
	}
	lines, ok := markdown.Parse(src).Section(2, "완료 조건")
	if !ok {
		return nil
	}
	for _, it := range markdown.Items(lines) {
		if !soleSpan.MatchString(it.Text) {
			continue
		}
		cmd := strings.Trim(it.Text, "`")
		if _, err := SplitCommand(cmd); err != nil {
			code := "command_parse"
			if errors.Is(err, ErrShellSyntax) {
				code = "shell_syntax"
			}
			l.problem(code, rel, it.Line, "%s task command cannot run without a shell: %s", t.ID, cmd)
			continue
		}
		t.Commands = append(t.Commands, cmd)
	}
	return nil
}

func (l *loader) readBudget(doc *markdown.Doc, file string) {
	lines, ok := l.section(doc, file, "예산")
	if !ok {
		return
	}
	haveRuns, haveTime := false, false
	for _, it := range markdown.Items(lines) {
		key, value, found := strings.Cut(it.Text, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "검증 실행":
			if m := firstInt.FindString(value); m != "" {
				n, _ := strconv.Atoi(m)
				l.b.Budget.Runs = n
				haveRuns = n > 0
			}
		case "경과 시간":
			if d, ok := ParseDuration(value); ok && d > 0 {
				l.b.Budget.ElapsedSeconds = int64(d / time.Second)
				haveTime = true
			}
		case "비용":
			l.b.Budget.Cost = value
		}
	}
	if !haveRuns {
		l.problem("budget_missing", file, 0, "`## 예산` needs `검증 실행: <positive number>`")
	}
	if !haveTime {
		l.problem("budget_missing", file, 0, "`## 예산` needs `경과 시간: <duration>` such as 4시간, 90분, or 2h30m")
	}
}

// ParseDuration reads `N시간`, `N분`, a combination of both, or a Go duration
// such as 2h30m.
func ParseDuration(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(strings.ReplaceAll(s, " ", "")); err == nil {
		return d, true
	}
	var d time.Duration
	found := false
	if m := hoursKo.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		d += time.Duration(n) * time.Hour
		found = true
	}
	if m := minutesKo.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		d += time.Duration(n) * time.Minute
		found = true
	}
	return d, found
}

func (l *loader) readProgress() error {
	rel := l.g.Dir + "/PROGRESS.md"
	src, ok, err := l.read(rel)
	if err != nil || !ok {
		return err
	}
	lines, ok := markdown.Parse(src).Section(2, "Task 상태")
	if !ok {
		return nil
	}
	tables := markdown.Tables(lines)
	if len(tables) == 0 {
		return nil
	}
	if !tables[0].HeaderIs("Task", "상태", "메모") {
		l.problem("table_columns", rel, tables[0].Line, "`## Task 상태` columns must be Task | 상태 | 메모")
		return nil
	}
	for _, r := range tables[0].Rows {
		id, state := strings.TrimSpace(r.Cell(0)), strings.TrimSpace(r.Cell(1))
		valid := false
		for _, s := range TaskStates {
			valid = valid || s == state
		}
		if !valid {
			l.problem("progress_state_invalid", rel, r.Line, "%s state must be one of %s: %q", id, strings.Join(TaskStates, ", "), state)
			continue
		}
		l.b.Progress[id] = state
	}
	return nil
}

func (l *loader) crossCheck(file string) {
	b, g := l.b, l.g
	rows := map[string]int{}
	for _, r := range b.Rows {
		if _, dup := rows[r.ID]; dup {
			l.problem("criterion_duplicate_row", file, r.Line, "%s appears more than once in `## 조건표`", r.ID)
			continue
		}
		rows[r.ID] = r.Line
		if strings.HasPrefix(r.ID, "AC-") && g.Criterion(r.ID) == nil {
			l.problem("criterion_unknown", file, r.Line, "%s is not a criterion in the goal documents", r.ID)
		}
		switch r.Kind {
		case KindChange:
			if r.Command == "" {
				l.problem("command_missing", file, r.Line, "%s (change) needs a verification command", r.ID)
			}
			if len(r.CheckPaths) == 0 {
				l.problem("check_paths_missing", file, r.Line, "%s (change) needs check paths for the baseline check", r.ID)
			}
		case KindMaintain:
			if r.Command == "" {
				l.problem("command_missing", file, r.Line, "%s (maintain) needs a verification command", r.ID)
			}
		case KindManual:
			if r.Reason == "" {
				l.problem("manual_reason_missing", file, r.Line, "%s (manual) needs a reason after the dash", r.ID)
			}
		}
	}
	for _, c := range g.Criteria {
		if _, ok := rows[c.ID]; !ok {
			l.problem("criterion_not_in_plan", file, 0, "%s from the goal documents is missing from `## 조건표`", c.ID)
		}
	}

	tasks := map[string]*Task{}
	for i := range b.Tasks {
		t := &b.Tasks[i]
		if _, dup := tasks[t.ID]; dup {
			l.problem("task_duplicate_id", file, t.Line, "%s is listed twice", t.ID)
			continue
		}
		tasks[t.ID] = t
	}
	covered := map[string]bool{}
	isPrereq := map[string]bool{}
	for _, t := range b.Tasks {
		for _, d := range t.Deps {
			isPrereq[d] = true
		}
	}
	proposals := map[string]bool{}
	for _, p := range g.Proposals {
		proposals[p.ID] = true
	}
	deps := map[string][]string{}
	for _, t := range b.Tasks {
		deps[t.ID] = t.Deps
		for _, c := range t.Criteria {
			if _, ok := rows[c]; !ok {
				l.problem("task_unknown_criterion", file, t.Line, "%s refers to %s, which is not in `## 조건표`", t.ID, c)
			}
			covered[c] = true
		}
		for _, d := range t.Deps {
			if tasks[d] == nil {
				l.problem("task_unknown_dependency", file, t.Line, "%s depends on %s, which is not a task", t.ID, d)
			}
		}
		if len(t.Criteria) == 0 && !isPrereq[t.ID] {
			l.problem("task_orphan", file, t.Line, "%s is not grounded in a condition and no task depends on it", t.ID)
		}
		if t.Proposal != "" {
			if !proposalRef.MatchString(t.Proposal) || !proposals[t.Proposal] {
				l.problem("proposal_unknown", file, t.Line, "%s refers to proposal %s, which is not in the goal documents", t.ID, t.Proposal)
			}
		}
	}
	for _, r := range b.Rows {
		for _, tid := range r.Tasks {
			if tasks[tid] == nil {
				l.problem("task_unknown", file, r.Line, "%s refers to %s, which is not in `## Task`", r.ID, tid)
				continue
			}
			covered[r.ID] = true
		}
	}
	for _, r := range b.Rows {
		if !covered[r.ID] {
			l.problem("criterion_without_task", file, r.Line, "%s has no task", r.ID)
		}
	}
	if cyc := goaldocs.FindCycle(deps); cyc != nil {
		l.problem("task_cycle", file, 0, "task dependencies form a cycle: %s", strings.Join(cyc, " -> "))
	}
}

// Lint returns all goal document and bundle problems in a stable order.
func Lint(g *goaldocs.Goal, b *Bundle) []diag.Problem {
	out := append(append([]diag.Problem(nil), g.Problems...), b.Problems...)
	diag.Sort(out)
	return out
}
