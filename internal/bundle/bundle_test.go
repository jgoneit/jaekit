package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/goaldocs"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		err  error
	}{
		{"go test ./...", []string{"go", "test", "./..."}, nil},
		{`pytest -k "empty and not slow" -q`, []string{"pytest", "-k", "empty and not slow", "-q"}, nil},
		{`grep 'a|b' file`, []string{"grep", "a|b", "file"}, nil},
		{`echo a\ b`, []string{"echo", "a b"}, nil},
		{"go test | tee out", nil, ErrShellSyntax},
		{"make && make test", nil, ErrShellSyntax},
		{"run > out.txt", nil, ErrShellSyntax},
		{"echo $HOME", nil, ErrShellSyntax},
		{"a; b", nil, ErrShellSyntax},
		{`echo "open`, nil, ErrQuote},
		{"   ", nil, ErrEmpty},
	}
	for _, c := range cases {
		got, err := SplitCommand(c.in)
		if !errors.Is(err, c.err) || !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitCommand(%q) = %#v, %v; want %#v, %v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"src/**", "src/a/b.go", true},
		{"src/**", "src", true},
		{"src/*.go", "src/a/b.go", false},
		{"**/*_test.*", "pkg/x_test.go", true},
		{"**/*_test.*", "x_test.go", true},
		{"tests/**", "tests/unit/a.py", true},
		{"tests/**", "lib/tests/a.py", false},
		{"docs/x.md", "docs/x.md", true},
	}
	for _, c := range cases {
		if got := Match(c.pat, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pat, c.name, got, c.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"4시간":      4 * time.Hour,
		"90분":      90 * time.Minute,
		"1시간 30분":  90 * time.Minute,
		"2h30m":    150 * time.Minute,
		"2 hours?": 0,
	}
	for in, want := range cases {
		got, _ := ParseDuration(in)
		if got != want {
			t.Errorf("ParseDuration(%q) = %v, want %v", in, got, want)
		}
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const spec = `# G
Status: Ready
## Acceptance Criteria
- **AC-1** greeting says hello
- **AC-2** keep stays
- **AC-3** (선택) looks nice
## 작업 분해 제안
| ID | 결과 | 조건 | 의존 |
| --- | --- | --- | --- |
| W1 | greeting | AC-1, AC-2 | — |
`

const plan = "# PLAN\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n| AC-1 | change | `sh tests/greeting.sh` | tests/greeting.sh | T001 |\n| AC-2 | maintain | `sh tests/keep.sh` | — | T001 |\n| AC-3 | manual | — 사람이 눈으로 본다 | — | T002 |\n| EX-1 | maintain | `sh tests/all.sh` | — | T001 |\n\n## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/greeting.sh` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: keep\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n| T001 | greeting says hello | AC-1, AC-2, EX-1 | W1 | — | 예 | tasks/T001.md |\n| T002 | review look | AC-3 | — | T001 | 아니오 | — |\n\n## 예산\n- 검증 실행: 50\n- 경과 시간: 4시간\n- 비용: 미관측\n"

const task = "# T001\n\n## 완료 조건\n- AC-1 passes\n- `sh tests/greeting.sh --quick`\n"

const progress = "# PROGRESS\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | |\n| T002 | todo | |\n"

func load(t *testing.T, root string) (*goaldocs.Goal, *Bundle) {
	t.Helper()
	g, err := goaldocs.Load(root, "g")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(root, g)
	if err != nil {
		t.Fatal(err)
	}
	return g, b
}

func TestLoadValidBundle(t *testing.T) {
	root := t.TempDir()
	write(t, root, "g/SPEC.md", spec)
	write(t, root, "g/PLAN.md", plan)
	write(t, root, "g/REVIEW.md", "# REVIEW\n")
	write(t, root, "g/PROGRESS.md", progress)
	write(t, root, "g/tasks/T001.md", task)
	g, b := load(t, root)
	if ps := Lint(g, b); len(ps) != 0 {
		t.Fatalf("unexpected problems: %v", ps)
	}
	r := b.Row("AC-1")
	if r == nil || !reflect.DeepEqual(r.Argv, []string{"sh", "tests/greeting.sh"}) || !reflect.DeepEqual(r.CheckPaths, []string{"tests/greeting.sh"}) {
		t.Fatalf("AC-1 row = %#v", r)
	}
	if m := b.Row("AC-3"); m.Reason != "사람이 눈으로 본다" || m.Command != "" {
		t.Fatalf("AC-3 row = %#v", m)
	}
	if !reflect.DeepEqual(b.Scope.Allowed, []string{"src/**", "tests/greeting.sh"}) || !reflect.DeepEqual(b.Scope.Tests, DefaultTestPaths) {
		t.Fatalf("scope = %#v", b.Scope)
	}
	if b.Budget.Runs != 50 || b.Budget.ElapsedSeconds != 4*3600 || b.Budget.Cost != "미관측" {
		t.Fatalf("budget = %#v", b.Budget)
	}
	if got := b.Task("T001").Commands; !reflect.DeepEqual(got, []string{"sh tests/greeting.sh --quick"}) {
		t.Fatalf("task commands = %#v", got)
	}
	if b.Progress["T001"] != "done" || b.Progress["T002"] != "todo" {
		t.Fatalf("progress = %#v", b.Progress)
	}
}

func TestLintReportsDistinctCodes(t *testing.T) {
	root := t.TempDir()
	write(t, root, "g/SPEC.md", spec)
	write(t, root, "g/PLAN.md", "# PLAN\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n| AC-1 | change | `go test \\| tee x` | — | T001 |\n| AC-9 | maintain | — | — | T009 |\n| AC-3 | manual | — | — | |\n\n## 범위\n- 테스트 경로: (기본값)\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n| T001 | a | AC-1 | W7 | T003 | 예 | tasks/T001.md |\n| T002 | b | — | — | — | 예 | — |\n| T003 | c | AC-1 | — | T001 | 예 | — |\n\n## 예산\n- 비용: 미관측\n")
	g, b := load(t, root)
	got := map[string]bool{}
	for _, p := range Lint(g, b) {
		got[p.Code] = true
	}
	want := []string{
		"shell_syntax", "check_paths_missing", "criterion_unknown", "command_missing",
		"manual_reason_missing", "criterion_not_in_plan", "task_unknown", "criterion_without_task",
		"task_orphan", "task_cycle", "proposal_unknown", "task_doc_missing", "scope_missing",
		"budget_missing", "bundle_file_missing",
	}
	var missing []string
	for _, c := range want {
		if !got[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("missing lint codes %v; got %v", missing, got)
	}
}

func TestDocPathsCoverBundleFiles(t *testing.T) {
	g := &goaldocs.Goal{Dir: "g", Docs: []string{"g/SPEC.md", "g/aux.md"}}
	want := []string{"g/PLAN.md", "g/PROGRESS.md", "g/REVIEW.md", "g/SPEC.md", "g/aux.md", "g/runs.jsonl", "g/tasks"}
	if got := DocPaths(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("DocPaths = %#v", got)
	}
}

// Every lint code used by the parsers is documented in bundle.md §8, and the
// table lists no code that the parsers never emit.
func TestLintCodesMatchContract(t *testing.T) {
	used := map[string]bool{}
	call := regexp.MustCompile(`(?:problem\(|Code: )"([a-z_]+)"`)
	for _, f := range []string{"bundle.go", "evidence.go", "../goaldocs/goaldocs.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllStringSubmatch(string(src), -1) {
			used[m[1]] = true
		}
	}
	src, _ := os.ReadFile("bundle.go")
	for _, m := range regexp.MustCompile(`code := "([a-z_]+)"|code = "([a-z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		used[m[1]+m[2]] = true
	}
	doc, err := os.ReadFile("../../contracts/bundle.md")
	if err != nil {
		t.Fatal(err)
	}
	section := string(doc)[strings.Index(string(doc), "## 8. `ha lint` 오류 코드"):]
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|").FindAllStringSubmatch(section, -1) {
		documented[m[1]] = true
	}
	for c := range used {
		if !documented[c] {
			t.Errorf("lint code %s is not documented in bundle.md §8", c)
		}
	}
	for c := range documented {
		if !used[c] {
			t.Errorf("bundle.md §8 documents %s, which no parser emits", c)
		}
	}
}
