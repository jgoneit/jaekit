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
		b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
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
		text := strings.ToLower(rrtRead(t, rel))
		for _, token := range []string{"regression_map", "go_selection", "selection-only", "static selection"} {
			if strings.Contains(text, token) {
				t.Errorf("jaekit:violation=registry-rules-stale: %s still mentions %q", rel, token)
			}
		}
	}
	contract := rrtRead(t, "contracts/regressions.md")
	for _, field := range []string{"property", "examples", "families", "boundary", "follow_up", "status"} {
		if token := "`" + field + "`"; strings.Contains(contract, token) {
			t.Errorf("jaekit:violation=registry-rules-stale: contracts/regressions.md still names the removed field %s", token)
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
	"tools/review-checks/preserved-public.json",
	"tools/review-checks/AC-*.json",
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

// rrtTestName matches test and integration file names in reference text.
var rrtTestName = regexp.MustCompile(`(?:test|integration)_[A-Za-z0-9_-]*\.py`)

func rrtWordByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func TestRegistryTrimAC6_TestFilesRun(t *testing.T) {
	tracked := rrtTracked(t)
	allowed := map[string]bool{}
	for _, dir := range []string{"tools/public-checks", "tools/workflow-checks", "tools/ci", "tools/release",
		"tools/windows", "tools/goal-checks", "tools/release-checks"} {
		allowed[dir] = true
	}
	// referenced maps a file name to the files whose text names it.
	referenced := map[string]map[string]bool{}
	var candidates []string
	for _, rel := range tracked {
		base := path.Base(rel)
		inTools := strings.HasPrefix(rel, "tools/")
		if inTools && path.Ext(base) == ".py" && (strings.HasPrefix(base, "test_") || strings.HasPrefix(base, "integration_")) {
			candidates = append(candidates, rel)
		}
		workflow, _ := path.Match(".github/workflows/*.yml", rel)
		reference := workflow || strings.HasSuffix(rel, "_test.go") && rel != rrtSelf ||
			inTools && (strings.HasSuffix(rel, ".py") || strings.HasSuffix(rel, ".sh"))
		if !reference {
			continue
		}
		b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(b)
		for _, m := range rrtTestName.FindAllStringIndex(text, -1) {
			if m[0] > 0 && rrtWordByte(text[m[0]-1]) || m[1] < len(text) && rrtWordByte(text[m[1]]) {
				continue
			}
			name := text[m[0]:m[1]]
			if referenced[name] == nil {
				referenced[name] = map[string]bool{}
			}
			referenced[name][rel] = true
		}
	}
	if len(candidates) == 0 {
		t.Fatalf("no tracked tools/**/test_*.py or tools/**/integration_*.py files")
	}
	for _, rel := range candidates {
		if allowed[path.Dir(rel)] {
			continue
		}
		users := referenced[path.Base(rel)]
		if len(users) == 0 || len(users) == 1 && users[rel] {
			t.Errorf("jaekit:violation=unrun-check-files: %s sits outside the discovered directories and no runner names it", rel)
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
