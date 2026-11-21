package plugins

// Goal checks for regression-registry-trim: the registry keeps only routes and
// their checks, the static selection prover is gone, discover steps fail when
// they collect no tests, and unrun check files are deleted. Repository files
// are read relative to the package directory (plugins/), so "../" is the root.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	rrtRegistry = "contracts/regressions.json"
	rrtExpected = "tools/goal-checks/regression-registry-trim/expected.json"
	rrtSelf     = "plugins/regression_registry_trim_test.go"
	rrtGoalDir  = "tools/goal-checks/regression-registry-trim/"
)

// rrtSelection is the registry-selection-integrity route's checks after the change.
var rrtSelection = [][3]string{
	{"tools/workflow-checks/test_regressions.py", "RegistryShape", "docs"},
	{"tools/workflow-checks/test_verify.py", "ZeroCollectionCases", "docs"},
}

// rrtRead reads a repository file; an unreadable or non-UTF-8 file stops the test.
func rrtRead(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if !utf8.Valid(b) {
		t.Fatalf("%s is not UTF-8", rel)
	}
	return string(b)
}

// rrtJSON reads a repository JSON file; text that is not JSON stops the test.
func rrtJSON(t *testing.T, rel string) []byte {
	t.Helper()
	b := []byte(rrtRead(t, rel))
	if !json.Valid(b) {
		t.Fatalf("%s is not valid JSON", rel)
	}
	return b
}

// rrtTracked lists the tracked files from the repository root.
func rrtTracked(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel != "" {
			files = append(files, rel)
		}
	}
	if len(files) == 0 {
		t.Fatalf("git ls-files listed no files")
	}
	return files
}

// rrtObject is a JSON object with its keys in order, duplicates included.
type rrtObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func rrtObj(raw json.RawMessage) (rrtObject, error) {
	o := rrtObject{vals: map[string]json.RawMessage{}}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return o, errors.New("not a JSON object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return o, err
		}
		key, _ := tok.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return o, err
		}
		o.keys = append(o.keys, key)
		o.vals[key] = v
	}
	return o, nil
}

func rrtArray(raw json.RawMessage) ([]json.RawMessage, error) {
	var a []json.RawMessage
	if err := json.Unmarshal(raw, &a); err != nil || a == nil {
		return nil, errors.New("not a JSON array")
	}
	return a, nil
}

func rrtString(raw json.RawMessage) (string, error) {
	var s string
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '"' {
		return "", errors.New("not a JSON string")
	}
	err := json.Unmarshal(raw, &s)
	return s, err
}

func rrtSameKeys(got []string, want ...string) bool {
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	return reflect.DeepEqual(g, w)
}

func TestRegistryTrimAC1_Shape(t *testing.T) {
	top, err := rrtObj(rrtJSON(t, rrtRegistry))
	if err != nil {
		t.Errorf("jaekit:violation=registry-not-trimmed: %s: %v", rrtRegistry, err)
		return
	}
	if want := []string{"schema", "invariants", "routes"}; !rrtSameKeys(top.keys, want...) {
		t.Errorf("jaekit:violation=registry-not-trimmed: %s has top-level keys %q, want exactly %q", rrtRegistry, top.keys, want)
	}
	routes, err := rrtArray(top.vals["routes"])
	if err != nil {
		t.Errorf("jaekit:violation=registry-not-trimmed: %s routes: %v", rrtRegistry, err)
		return
	}
	for i, raw := range routes {
		where := fmt.Sprintf("route %d", i+1)
		route, err := rrtObj(raw)
		if err != nil {
			t.Errorf("jaekit:violation=registry-not-trimmed: %s: %v", where, err)
			continue
		}
		if id, err := rrtString(route.vals["id"]); err == nil {
			where = "route " + id
		}
		if want := []string{"id", "invariant", "sources", "checks"}; !rrtSameKeys(route.keys, want...) {
			t.Errorf("jaekit:violation=registry-not-trimmed: %s has keys %q, want exactly %q", where, route.keys, want)
		}
		for _, key := range []string{"id", "invariant"} {
			if _, err := rrtString(route.vals[key]); err != nil {
				t.Errorf("jaekit:violation=registry-not-trimmed: %s %s: %v", where, key, err)
			}
		}
		sources, err := rrtArray(route.vals["sources"])
		if err != nil {
			t.Errorf("jaekit:violation=registry-not-trimmed: %s sources: %v", where, err)
		}
		for j, source := range sources {
			if _, err := rrtString(source); err != nil {
				t.Errorf("jaekit:violation=registry-not-trimmed: %s source %d: %v", where, j+1, err)
			}
		}
		checks, err := rrtArray(route.vals["checks"])
		if err != nil {
			t.Errorf("jaekit:violation=registry-not-trimmed: %s checks: %v", where, err)
			continue
		}
		for j, raw := range checks {
			at := fmt.Sprintf("%s check %d", where, j+1)
			check, err := rrtObj(raw)
			if err != nil {
				t.Errorf("jaekit:violation=registry-not-trimmed: %s: %v", at, err)
				continue
			}
			if want := []string{"file", "selector", "group"}; !rrtSameKeys(check.keys, want...) {
				t.Errorf("jaekit:violation=registry-not-trimmed: %s has keys %q, want exactly %q", at, check.keys, want)
			}
			for _, key := range []string{"file", "selector"} {
				if s, err := rrtString(check.vals[key]); err != nil || s == "" {
					t.Errorf("jaekit:violation=registry-not-trimmed: %s %s is not a nonempty string", at, key)
				}
			}
			if group, err := rrtString(check.vals["group"]); err != nil || (group != "go" && group != "docs") {
				shown := string(check.vals["group"])
				if shown == "" {
					shown = "missing"
				}
				t.Errorf("jaekit:violation=registry-not-trimmed: %s group is %s, want \"go\" or \"docs\"", at, shown)
			}
		}
	}
}

func rrtText(v any) string {
	s, _ := v.(string)
	return s
}

func rrtList(v any) []any {
	l, _ := v.([]any)
	return l
}

func TestRegistryTrimAC1_Selectors(t *testing.T) {
	var expected struct {
		Invariants []any `json:"invariants"`
		Routes     []struct {
			ID        string `json:"id"`
			Invariant string `json:"invariant"`
		} `json:"routes"`
		Checks []struct {
			Route    string `json:"route"`
			File     string `json:"file"`
			Selector string `json:"selector"`
			Group    string `json:"group"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rrtJSON(t, rrtExpected), &expected); err != nil {
		t.Fatalf("%s: %v", rrtExpected, err)
	}
	if len(expected.Invariants) == 0 || len(expected.Routes) == 0 || len(expected.Checks) == 0 {
		t.Fatalf("%s has an empty invariants, routes or checks list", rrtExpected)
	}
	var registry map[string]any
	if err := json.Unmarshal(rrtJSON(t, rrtRegistry), &registry); err != nil {
		t.Errorf("jaekit:violation=registry-not-trimmed: %s is not a JSON object: %v", rrtRegistry, err)
		return
	}
	tracked := map[string]bool{}
	for _, rel := range rrtTracked(t) {
		tracked[rel] = true
	}
	requireTracked := func(where string, v any) {
		if rel := rrtText(v); !tracked[rel] {
			t.Errorf("jaekit:violation=registry-not-trimmed: %s lists %v, which is not a tracked file", where, v)
		}
	}

	invariants := rrtList(registry["invariants"])
	if !reflect.DeepEqual(invariants, expected.Invariants) {
		t.Errorf("jaekit:violation=registry-not-trimmed: %s invariants differ from %s\n got: %v\nwant: %v", rrtRegistry, rrtExpected, invariants, expected.Invariants)
	}
	for _, inv := range invariants {
		item, _ := inv.(map[string]any)
		for _, basis := range rrtList(item["basis"]) {
			requireTracked(fmt.Sprintf("invariant %v basis", item["id"]), basis)
		}
	}

	var gotRoutes, wantRoutes []string
	for _, r := range expected.Routes {
		wantRoutes = append(wantRoutes, r.ID+" "+r.Invariant)
	}
	checks := map[string]map[[3]string]int{}
	for _, raw := range rrtList(registry["routes"]) {
		route, _ := raw.(map[string]any)
		id := rrtText(route["id"])
		gotRoutes = append(gotRoutes, id+" "+rrtText(route["invariant"]))
		for _, source := range rrtList(route["sources"]) {
			requireTracked("route "+id+" sources", source)
		}
		if checks[id] == nil {
			checks[id] = map[[3]string]int{}
		}
		for _, c := range rrtList(route["checks"]) {
			check, _ := c.(map[string]any)
			checks[id][[3]string{rrtText(check["file"]), rrtText(check["selector"]), rrtText(check["group"])}]++
			requireTracked("route "+id+" checks", check["file"])
		}
	}
	if !reflect.DeepEqual(gotRoutes, wantRoutes) {
		t.Errorf("jaekit:violation=registry-not-trimmed: route ids and invariants differ from %s\n got: %q\nwant: %q", rrtExpected, gotRoutes, wantRoutes)
	}
	for _, c := range expected.Checks {
		if checks[c.Route][[3]string{c.File, c.Selector, c.Group}] == 0 {
			t.Errorf("jaekit:violation=registry-not-trimmed: route %s lacks check %s %s (%s)", c.Route, c.File, c.Selector, c.Group)
		}
	}
	want := map[[3]string]int{}
	for _, c := range rrtSelection {
		want[c] = 1
	}
	if got := checks["registry-selection-integrity"]; !reflect.DeepEqual(got, want) {
		t.Errorf("jaekit:violation=registry-not-trimmed: route registry-selection-integrity has checks %v, want exactly %v", got, rrtSelection)
	}
}

func TestRegistryTrimAC1_Contract(t *testing.T) {
	text := rrtRead(t, "contracts/regressions.md")
	for _, want := range []string{
		"Each route has `id`, `invariant`, `sources` and `checks`.",
		"To add a regression, write the test and list it under its route's `checks`.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("jaekit:violation=registry-not-trimmed: contracts/regressions.md lacks %q", want)
		}
	}
}

// rrtReadTracked reads a tracked file's bytes; a tracked file that cannot be
// read, including one deleted from the checkout, stops the test.
func rrtReadTracked(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read tracked file %s: %v", rel, err)
	}
	return b
}

// rrtPresent reports whether a repository path exists on the filesystem.
func rrtPresent(t *testing.T, rel string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %s: %v", rel, err)
	}
	return err == nil
}

func TestRegistryTrimAC2_ProverGone(t *testing.T) {
	tracked := rrtTracked(t)
	isTracked := map[string]bool{}
	for _, rel := range tracked {
		isTracked[rel] = true
	}
	for _, rel := range []string{
		"tools/regression_map.py",
		"tools/go_selection.py",
		"tools/regression-checks/integration_selection_go.py",
		"tools/regression-checks/integration_selection_go_identifiers.py",
		"tools/regression-checks/integration_selection_go_layout.py",
		"tools/regression-checks/integration_selection_go_runtime.py",
		"tools/regression-checks/integration_selection_python.py",
		"tools/regression-checks/test_map.py",
	} {
		if rrtPresent(t, rel) || isTracked[rel] {
			t.Errorf("jaekit:violation=selection-prover-present: %s exists", rel)
		}
	}
	verify := rrtRead(t, "tools/verify.py")
	for _, step := range []string{"Static selection regressions", "Go selection execution"} {
		if strings.Contains(verify, step) {
			t.Errorf("jaekit:violation=selection-prover-present: tools/verify.py still has the step %q", step)
		}
	}
	for _, rel := range tracked {
		if rel == rrtSelf || strings.HasPrefix(rel, rrtGoalDir) || strings.HasPrefix(rel, "docs/") {
			continue
		}
		b := rrtReadTracked(t, rel)
		for _, token := range []string{"regression_map", "go_selection"} {
			if bytes.Contains(b, []byte(token)) {
				t.Errorf("jaekit:violation=selection-prover-present: %s contains %s", rel, token)
			}
		}
	}
}

var (
	rrtRanLine = regexp.MustCompile(`(?m)^Ran (\d+) tests? in `)
	rrtOKLine  = regexp.MustCompile(`^OK(?: \((.*)\))?$`)
	rrtSkipped = regexp.MustCompile(`skipped=(\d+)`)
	rrtCase    = regexp.MustCompile(`(?m)^(?:FAIL|ERROR): test\S* \(.*\)$`)
)

// rrtRan is the count in the last "Ran N tests" line, or -1 without one.
func rrtRan(stderr string) int {
	m := rrtRanLine.FindAllStringSubmatch(stderr, -1)
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(m[len(m)-1][1])
	if err != nil {
		return -1
	}
	return n
}

// rrtTail is the failed unittest cases followed by the last 40 lines of text.
func rrtTail(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return strings.Join(append(rrtCase.FindAllString(text, -1), lines...), "\n")
}

// rrtPython runs python3 from the repository root. A python3 that cannot
// start or ends by a signal stops the test; it is not an observation.
func rrtPython(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command("python3", args...)
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit) && exit.Exited():
		code = exit.ExitCode()
	case errors.As(err, &exit):
		t.Fatalf("python3 %s did not exit normally: %v\n%s", strings.Join(args, " "), err, rrtTail(errOut.String()))
	default:
		t.Fatalf("python3 cannot start: %v", err)
	}
	return code, out.String(), errOut.String()
}

func TestRegistryTrimAC3_ZeroCollection(t *testing.T) {
	code, _, stderr := rrtPython(t, "-m", "unittest", "discover", "-s", "tools/workflow-checks",
		"-p", "test_verify.py", "-k", "ZeroCollectionCases", "-v")
	ran := rrtRan(stderr)
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	ok := rrtOKLine.FindStringSubmatch(strings.TrimSpace(lines[len(lines)-1]))
	skipped := 0
	if ok != nil {
		if m := rrtSkipped.FindStringSubmatch(ok[1]); m != nil {
			skipped, _ = strconv.Atoi(m[1])
		}
	}
	if code != 0 || ran < 1 || ok == nil || skipped >= ran {
		t.Errorf("jaekit:violation=zero-collection-passes: ZeroCollectionCases did not pass (exit %d, ran %d, skipped %d)\n%s", code, ran, skipped, rrtTail(stderr))
	}
}

// rrtSelectorProgram runs unittest selectors of one file the way discover
// imports it: the file's directory first on sys.path, the module loaded by
// path under its own name. It writes one JSON line per selector to argv[1].
const rrtSelectorProgram = `
import importlib.util, json, os, sys, unittest
results, path, selectors = sys.argv[1], sys.argv[2], sys.argv[3:]
sys.path.insert(0, os.path.dirname(os.path.abspath(path)))
name = os.path.splitext(os.path.basename(path))[0]
spec = importlib.util.spec_from_file_location(name, path)
module = importlib.util.module_from_spec(spec)
sys.modules[name] = module
spec.loader.exec_module(module)
failed = False
with open(results, "w") as out:
    for selector in selectors:
        try:
            suite = unittest.defaultTestLoader.loadTestsFromName(selector, module)
            result = unittest.TextTestRunner(stream=sys.stderr, verbosity=2).run(suite)
            counts = {"ran": result.testsRun, "failures": len(result.failures), "errors": len(result.errors),
                      "skipped": len(result.skipped), "expected_failures": len(result.expectedFailures),
                      "unexpected_successes": len(result.unexpectedSuccesses)}
        except Exception as error:
            counts = {"ran": 0, "load_error": repr(error)}
        ok = counts["ran"] > 0 and not any(value for key, value in counts.items() if key != "ran")
        failed = failed or not ok
        out.write(json.dumps(dict(counts, selector=selector, ok=ok)) + "\n")
        out.flush()
sys.exit(1 if failed else 0)
`

// rrtGoRun runs `go test -json` in one package directory with a -run pattern
// and reports each named test that has no pass event.
func rrtGoRun(t *testing.T, dir, pattern string, names []string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-json", "-count=1", "-run", pattern, "./"+dir)
	cmd.Dir = ".."
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil, errors.As(err, &exit) && exit.Exited():
	case errors.As(err, &exit):
		t.Fatalf("go test ./%s did not exit normally: %v", dir, err)
	default:
		t.Fatalf("go cannot start: %v", err)
	}
	passed := map[string]bool{}
	output := map[string]*strings.Builder{}
	for _, line := range strings.Split(out.String(), "\n") {
		var e struct{ Action, Test, Output string }
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		if e.Action == "pass" && e.Test != "" {
			passed[e.Test] = true
		}
		if e.Action == "output" {
			if output[e.Test] == nil {
				output[e.Test] = &strings.Builder{}
			}
			output[e.Test].WriteString(e.Output)
		}
	}
	for _, name := range names {
		if passed[name] {
			continue
		}
		detail := errOut.String()
		for _, test := range []string{name, ""} {
			if output[test] != nil {
				detail = output[test].String() + detail
				break
			}
		}
		t.Errorf("go test ./%s -run '%s': %s did not pass\n%s", dir, pattern, name, rrtTail(detail))
	}
}

// TestRegistryTrimAC4_SelectorsRun runs every check of expected.json by its
// exact selector; it carries no violation marker and runs only when
// RRT_RUN_SELECTORS=1.
func TestRegistryTrimAC4_SelectorsRun(t *testing.T) {
	if os.Getenv("RRT_RUN_SELECTORS") != "1" {
		t.Skip("set RRT_RUN_SELECTORS=1 to run every expected check by its selector")
	}
	var expected struct {
		Checks []struct{ File, Selector string } `json:"checks"`
	}
	if err := json.Unmarshal(rrtJSON(t, rrtExpected), &expected); err != nil {
		t.Fatalf("%s: %v", rrtExpected, err)
	}
	if len(expected.Checks) == 0 {
		t.Fatalf("%s lists no checks", rrtExpected)
	}
	goNames := map[string][]string{}
	pyNames := map[string][]string{}
	seen := map[[2]string]bool{}
	for _, c := range expected.Checks {
		if key := [2]string{c.File, c.Selector}; !seen[key] {
			seen[key] = true
			switch {
			case strings.HasSuffix(c.File, "_test.go"):
				goNames[path.Dir(c.File)] = append(goNames[path.Dir(c.File)], c.Selector)
			case strings.HasSuffix(c.File, ".py"):
				pyNames[c.File] = append(pyNames[c.File], c.Selector)
			default:
				t.Errorf("%s %s: the file is neither a Go test file nor a Python file", c.File, c.Selector)
			}
		}
	}
	for _, dir := range rrtSortedKeys(goNames) {
		var plain []string
		for _, name := range goNames[dir] {
			if !strings.Contains(name, "/") {
				plain = append(plain, regexp.QuoteMeta(name))
				continue
			}
			// -run splits its pattern at "/" into one pattern per subtest level.
			var levels []string
			for _, level := range strings.Split(name, "/") {
				levels = append(levels, "^"+regexp.QuoteMeta(level)+"$")
			}
			rrtGoRun(t, dir, strings.Join(levels, "/"), []string{name})
		}
		if len(plain) > 0 {
			var names []string
			for _, name := range goNames[dir] {
				if !strings.Contains(name, "/") {
					names = append(names, name)
				}
			}
			rrtGoRun(t, dir, "^("+strings.Join(plain, "|")+")$", names)
		}
	}
	for _, file := range rrtSortedKeys(pyNames) {
		results := filepath.Join(t.TempDir(), "results.jsonl")
		code, stdout, stderr := rrtPython(t, append([]string{"-c", rrtSelectorProgram, results, file}, pyNames[file]...)...)
		got := map[string]map[string]any{}
		if b, err := os.ReadFile(results); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				var r map[string]any
				if json.Unmarshal([]byte(line), &r) == nil {
					got[rrtText(r["selector"])] = r
				}
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("read %s: %v", results, err)
		}
		failed := false
		for _, selector := range pyNames[file] {
			if r := got[selector]; r == nil || r["ok"] != true {
				failed = true
				t.Errorf("python3 %s %s did not pass: %v (exit %d)\n%s", file, selector, r, code, rrtTail(stdout+stderr))
			}
		}
		if code != 0 && !failed {
			t.Errorf("python3 selectors of %s exited %d although each selector passed\n%s", file, code, rrtTail(stdout+stderr))
		}
	}
	t.Logf("ran %d distinct selectors covering %d checks", len(seen), len(expected.Checks))
}

func rrtSortedKeys(m map[string][]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestRegistryTrimAC5_Rules(t *testing.T) {
	agents := rrtRead(t, "AGENTS.md")
	for _, c := range []struct{ rel, want string }{
		{"AGENTS.md", "A check bound to a protected path in `contracts/regressions.json` changes only in a separate goal that changes that path."},
		{"AGENTS.md", "Deleting or weakening a protected test needs the user's words in the pull request."},
		{"guides/DEVELOPMENT.md", "보호 시험을 지우거나 약하게 하려면 PR에 사용자의 말이 있어야 합니다."},
		{"guides/DEVELOPMENT.en.md", "Deleting or weakening a protected test needs the user's words in the pull request."},
	} {
		text := agents
		if c.rel != "AGENTS.md" {
			text = rrtRead(t, c.rel)
		}
		if !strings.Contains(text, c.want) {
			t.Errorf("jaekit:violation=registry-rules-stale: %s lacks %q", c.rel, c.want)
		}
	}
	for _, rel := range []string{"AGENTS.md", "guides/DEVELOPMENT.md", "guides/DEVELOPMENT.en.md", "guides/CI.md",
		"guides/CI.en.md", "contracts/regressions.md", "contracts/README.md"} {
		original := rrtRead(t, rel)
		text := strings.ToLower(original)
		for _, token := range []string{"regression_map", "go_selection", "selection-only", "static selection"} {
			if strings.Contains(text, token) {
				t.Errorf("jaekit:violation=registry-rules-stale: %s still mentions %q", rel, token)
			}
		}
		// Invariants keep `property`; these route fields are gone.
		for _, field := range []string{"examples", "families", "boundary", "follow_up", "status"} {
			if token := "`" + field + "`"; strings.Contains(original, token) {
				t.Errorf("jaekit:violation=registry-rules-stale: %s still names the removed route field %s", rel, token)
			}
		}
	}
}

// rrtRemoved lists paths that must not exist. A pattern ending in "/**" is a
// whole directory; the others are path.Match patterns, whose "*" stays within
// one path segment.
var rrtRemoved = []string{
	"tools/baseline-budget/**",
	"tools/history-checks/**",
	"tools/gap-checks/delivery.py",
	"tools/gap-checks/documentation.py",
	"tools/gap-checks/AC-*.json",
	"tools/review-checks/test_platform_installation.py",
	"tools/review-checks/test_preservation.py",
	"tools/review-checks/preserved-public.json",
	"tools/review-checks/AC-*.json",
	"tools/regression-checks/check.py",
	"tools/regression-checks/test_checker.py",
	"tools/regression-checks/run_unittest.py",
	"tools/verification-checks/pr_merge_blockers.py",
	"tools/verification-checks/producer_results.py",
	"tools/verification-checks/release_evidence.py",
	"tools/verification-checks/review_archive_boundaries.py",
	"tools/verification-checks/review_followups.py",
	"tools/verification-checks/review_producer_results.py",
	"tools/verification-checks/review_release_evidence.py",
	"tools/verification-checks/selection_integrity.py",
	"tools/verification-checks/windows_lifecycle_review.py",
	"tools/verification-checks/windows_native.py",
	"tools/verification-checks/windows_review_p1.py",
	"tools/verification-checks/windows_review_triage.py",
	"tools/verification-checks/windows_stream_review.py",
	"tools/verification-checks/wsl.py",
	"tools/verification-checks/wsl_preserved.py",
	"tools/verification-checks/integration-AC-*.json",
	// Declaration folders of the deleted verification-checks producers.
	"tools/verification-checks/selection-integrity/**",
	"tools/verification-checks/pr-merge-blockers/**",
	"tools/verification-checks/producer-results/**",
	"tools/verification-checks/release-evidence/**",
	"tools/verification-checks/review-archive-boundaries/**",
	"tools/verification-checks/review-followups/**",
	"tools/verification-checks/review-producer-results/**",
	"tools/verification-checks/review-release-evidence/**",
	"tools/verification-checks/windows-native/**",
	"tools/verification-checks/windows-lifecycle-review/**",
	"tools/verification-checks/windows-review-p1/**",
	"tools/verification-checks/windows-review-triage/**",
	"tools/verification-checks/windows-stream-review/**",
	"tools/verification-checks/wsl/**",
	// AC-1.json stays: tests copy it.
	"tools/verification-checks/review-regression-gates/AC-2.json",
	"tools/verification-checks/review-regression-gates/AC-3.json",
	"tools/verification-checks/review-regression-gates/AC-4.json",
	"tools/verification-checks/review-regression-gates/AC-5.json",
	"tools/verification-checks/review-regression-gates/AC-6.json",
	"tools/goal-checks/AC-*.json",
	"tools/goal-checks/retention_evidence.py",
	"tools/goal-checks/review-findings/**",
	"tools/goal-checks/english-contracts/**",
	"tools/goal-checks/remove-decision-history/**",
	"tools/goal-checks/spar-earlier-dispositions/**",
	"tools/goal-checks/spar-stage-merge/**",
	"tools/goal-checks/pages-site/AC-*.json",
	"tools/contract-checks/*.json",
	"tools/public-checks/path_evidence.py",
	"tools/public-checks/AC-*.json",
	"tools/release-checks/archive_evidence.py",
	"tools/release-checks/AC-*.json",
	"tools/findings-checks/declaration.json",
	"tools/regression-checks/declaration.json",
}

// rrtMatches lists the tracked and on-disk paths a removal pattern matches.
// Bytecode caches are not check files and are not listed.
func rrtMatches(t *testing.T, pattern string, tracked []string) []string {
	t.Helper()
	found := map[string]bool{}
	if dir, ok := strings.CutSuffix(pattern, "/**"); ok {
		for _, rel := range tracked {
			if strings.HasPrefix(rel, dir+"/") {
				found[rel] = true
			}
		}
		root := filepath.Join("..", filepath.FromSlash(dir))
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				rel, err := filepath.Rel("..", p)
				if err != nil {
					return err
				}
				found[filepath.ToSlash(rel)] = true
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("walk %s: %v", dir, err)
		}
	} else {
		for _, rel := range tracked {
			if ok, _ := path.Match(pattern, rel); ok {
				found[rel] = true
			}
		}
		matches, err := filepath.Glob(filepath.Join("..", filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		for _, p := range matches {
			rel, err := filepath.Rel("..", p)
			if err != nil {
				t.Fatalf("relative path of %s: %v", p, err)
			}
			found[filepath.ToSlash(rel)] = true
		}
	}
	var paths []string
	for rel := range found {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

func TestRegistryTrimAC6_Removed(t *testing.T) {
	tracked := rrtTracked(t)
	for _, pattern := range rrtRemoved {
		for _, rel := range rrtMatches(t, pattern, tracked) {
			t.Errorf("jaekit:violation=unrun-check-files: %s exists (%s)", rel, pattern)
		}
	}
}

// rrtWordByte is a byte that continues a file or directory name.
func rrtWordByte(b byte) bool {
	return b == '_' || b == '-' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// rrtHasWord reports whether text holds word with no name byte on either side.
func rrtHasWord(text, word string) bool {
	for i := 0; i < len(text); {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(word)
		if (start == 0 || !rrtWordByte(text[start-1])) && (end == len(text) || !rrtWordByte(text[end])) {
			return true
		}
		i = start + 1
	}
	return false
}

// rrtNamedBy reports whether file x, whose text is given, names file f: it
// holds f's base name and f's directory name as whole words, or it sits in
// f's directory and holds f's base name.
func rrtNamedBy(f, x, text string) bool {
	if !rrtHasWord(text, path.Base(f)) {
		return false
	}
	return path.Dir(x) == path.Dir(f) || rrtHasWord(text, path.Base(path.Dir(f)))
}

// rrtDiscovered are the directories full verification discovers with -p test_*.py.
var rrtDiscovered = map[string]bool{
	"tools/public-checks": true, "tools/workflow-checks": true, "tools/ci": true, "tools/release": true,
	"tools/windows": true, "tools/goal-checks": true, "tools/release-checks": true,
}

// rrtReachedByDiscovery reports whether full verification discovers a file
// with -p test_*.py (rule (a)).
func rrtReachedByDiscovery(rel string) bool {
	return strings.HasPrefix(path.Base(rel), "test_") && strings.HasSuffix(rel, ".py") && rrtDiscovered[path.Dir(rel)]
}

func TestRegistryTrimAC6_TestFilesRun(t *testing.T) {
	// R0 is verify.py, the workflows, the Go tests and the test files that
	// discovery reaches (rule (a)); R1 is the tools scripts R0 names. A
	// candidate runs when discovery reaches it or a file in R0 or R1 names it.
	var r0, scripts, candidates []string
	for _, rel := range rrtTracked(t) {
		workflow, _ := path.Match(".github/workflows/*.yml", rel)
		if rel == "tools/verify.py" || workflow || strings.HasSuffix(rel, "_test.go") && rel != rrtSelf ||
			rrtReachedByDiscovery(rel) {
			r0 = append(r0, rel)
		}
		if !strings.HasPrefix(rel, "tools/") {
			continue
		}
		if strings.HasSuffix(rel, ".py") || strings.HasSuffix(rel, ".sh") {
			scripts = append(scripts, rel)
		}
		if base := path.Base(rel); strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasPrefix(base, "integration_")) {
			candidates = append(candidates, rel)
		}
	}
	if len(candidates) == 0 {
		t.Fatalf("no tracked tools/**/test_*.py or tools/**/integration_*.py files")
	}
	texts := map[string]string{}
	text := func(rel string) string {
		if s, ok := texts[rel]; ok {
			return s
		}
		texts[rel] = string(rrtReadTracked(t, rel))
		return texts[rel]
	}
	runners := map[string]bool{}
	for _, x := range r0 {
		runners[x] = true
	}
	for _, s := range scripts {
		for _, x := range r0 {
			if rrtNamedBy(s, x, text(x)) {
				runners[s] = true
				break
			}
		}
	}
	var order []string
	for x := range runners {
		order = append(order, x)
	}
	sort.Strings(order)
	for _, c := range candidates {
		if rrtReachedByDiscovery(c) {
			continue
		}
		run := false
		for _, x := range order {
			if x != c && rrtNamedBy(c, x, text(x)) {
				run = true
				break
			}
		}
		if !run {
			t.Errorf("jaekit:violation=unrun-check-files: %s is not run: discovery does not reach it and no runner names it", c)
		}
	}
}

// TestRegistryTrimAC7_Preserved keeps the public checks and the verification
// and repository regressions passing; it carries no violation marker.
func TestRegistryTrimAC7_Preserved(t *testing.T) {
	for _, args := range [][]string{
		{"tools/check_public_tree.py"},
		{"tools/check_public_docs.py"},
		{"-m", "unittest", "discover", "-s", "tools/workflow-checks", "-p", "test_verify*.py"},
		{"-m", "unittest", "discover", "-s", "tools/workflow-checks", "-p", "test_repository.py"},
	} {
		code, stdout, stderr := rrtPython(t, args...)
		command := "python3 " + strings.Join(args, " ")
		if code != 0 {
			t.Errorf("%s exited %d\n%s", command, code, rrtTail(stdout+stderr))
			continue
		}
		if args[0] == "-m" {
			if ran := rrtRan(stderr); ran < 1 {
				t.Errorf("%s ran %d tests, want at least 1\n%s", command, ran, rrtTail(stderr))
			}
		}
	}
}
