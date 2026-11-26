// Package unusedcheckers holds the goal checks of the unused-checkers goal.
// Each test observes one acceptance criterion in the checked tree; Seal Core
// links them through the go-test-json adapter declarations beside this file.
// They are goal checks only and are deleted with this folder once the goal's
// completion is recorded.
package unusedcheckers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var base = flag.String("base", "", "the goal's start commit, for AC-8")

const root = "../../.."

const self = "tools/goal-checks/unused-checkers/"

func at(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

func read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(at(rel))
	if err != nil {
		t.Fatalf("cannot read %s: %v", rel, err)
	}
	return string(data)
}

// tracked lists the files Git tracks in the checked tree.
func tracked(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("cannot list tracked files: %v", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

// requireAbsent reports each path that is still tracked, as a file or as a
// directory with tracked files. Ignored leftovers on disk do not count.
func requireAbsent(t *testing.T, violation string, rels ...string) {
	t.Helper()
	files := tracked(t)
	for _, rel := range rels {
		for _, f := range files {
			if f == rel || strings.HasPrefix(f, rel+"/") {
				t.Errorf("jaekit:violation=%s %s is still tracked", violation, rel)
				break
			}
		}
	}
}

// run runs one command from the repository root and reports a nonzero exit
// as the violation. A command that cannot start is an error.
func run(t *testing.T, violation string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		t.Errorf("jaekit:violation=%s %s %s exited %d\n%s", violation, name, strings.Join(args, " "), exit.ExitCode(), out)
	default:
		t.Fatalf("cannot run %s: %v", name, err)
	}
}

func TestUnusedCheckersAC1(t *testing.T) {
	requireAbsent(t, "release-checker-present",
		"tools/release-checks", "tools/review-checks/test_observation.py", "tools/gap-checks/test_observation.py")
	verify := read(t, "tools/verify.py")
	for _, label := range []string{"Release evidence boundaries", "Release acceptance checker regressions"} {
		if strings.Contains(verify, label) {
			t.Errorf("jaekit:violation=release-checker-present tools/verify.py still has the step %q", label)
		}
	}
}

func TestUnusedCheckersAC2(t *testing.T) {
	requireAbsent(t, "finished-goal-checks-present",
		"tools/goal-checks/release-0-1-3-followups", "tools/goal-checks/release-0-1-3-review-fixes",
		"tools/goal-checks/windows-entry-name-refusal", "tools/goal-checks/test_history_audit.py",
		"tools/goal-checks/test_entry_name_ci_runs.py")
}

func TestUnusedCheckersAC3(t *testing.T) {
	requireAbsent(t, "authoring-producer-present",
		"tools/contract-checks/authoring_check.py", "tools/goal-checks/test_authoring_check.py")
	// The plugins Go tests that run the remaining contract producers' suites,
	// and the English-contracts tests that loaded the deleted producer: each
	// must run and pass.
	required := []string{
		"TestEnglishContractsAC1_OneEnglishFile", "TestEnglishContractsAC4_LinksOpen",
		"TestEnglishContractsAC5_PythonReaders", "TestEnglishContractsAC7_SiteEnglishOnly",
		"TestSpecBoundaryContractExamples", "TestVerificationDesignContracts",
		"TestVerificationFindingDocumentContracts", "TestVerificationReportingContracts",
	}
	cmd := exec.Command("go", "test", "-count=1", "-json", "-run", "^("+strings.Join(required, "|")+")$", "./plugins/")
	cmd.Dir = root
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("cannot run go test: %v", err)
	}
	passed := map[string]bool{}
	for _, line := range bytes.Split(out, []byte("\n")) {
		var event struct{ Action, Test string }
		if json.Unmarshal(line, &event) == nil && event.Action == "pass" && event.Test != "" {
			passed[event.Test] = true
		}
	}
	for _, name := range required {
		if !passed[name] {
			t.Errorf("jaekit:violation=authoring-producer-present plugins test %s did not run and pass", name)
		}
	}
	if exit != nil && !t.Failed() {
		t.Fatalf("go test exited %d although every required test passed\n%s", exit.ExitCode(), out)
	}
}

var removedRoutes = []string{
	"native-request-and-uncertainty", "shell-reachability-gap", "observation-identity-gap",
	"release-checker-import-boundary", "entry-name-ci-observation",
	"authoring-check-current-tree", "authoring-check-result-meaning", "public-scanner-path-gap",
}

// optionalChecks may stay in current-file-independence once rewritten to run
// without the deleted audits (SPEC AC-4); they are left out of that route
// only before it is compared. Like every registry check, a kept one must
// still name a test.
const optionalRoute = "current-file-independence"

var optionalChecks = map[string]bool{
	"tools/review-checks/test_producers.py::AC9": true,
	"tools/review-checks/test_producers.py::AC10.test_both_audits_require_an_explicit_range_and_do_not_fetch_missing_history": true,
	"tools/gap-checks/test_history_results.py::AC4": true,
	"tools/gap-checks/test_history_results.py::AC5": true,
}

// want holds the SHA-256 of each remaining route's JSON, re-encoded with
// sorted keys: the 16 routes this goal leaves as they were, and the four it
// narrows as they were without their deleted sources and checks.
var want = map[string]string{
	"current-file-independence":            "3ef6bd1022938f1e8fdb4195426dcfefab5300ff55e9872225eadd29015560f8",
	"stored-proof-independence":            "a17d264e92433ecc3471f870ece2394f6c58aa990b8c55658e97732b16144746",
	"structured-classification":            "0afbb490a9081522ab4869f906fabbe3ad6d06e6ab5f22e779c4b726637395e2",
	"stored-result-judgment":               "46c5a474aee798ad5474cfe6962162f6cfa1f66b0bbec7e5ce742be13741ce0b",
	"reference-producer-to-cli":            "089ddbeb506a7861a74f0fb76aed31a24179047aa2c425597f951a9ccfff073f",
	"goal-producer-meaning":                "832affd4cd8eeee891442b5b363e58a43156a413421780177fb91b967ab792de",
	"review-producer-meaning":              "d0435706b318182b96c46488e20272a52a67a74330807ae4ffad826e194f1f38",
	"gap-producer-meaning":                 "7e1ce5e038d3d34ff285da8cb028a26568adcde125c50d9a390ef30c647c8d51",
	"shared-tool-preflight":                "020a00b545fced51c72fa1f611d30528605cda75d51a3575bd7a0a1a3648c6f8",
	"shared-command-failures":              "93acbf82611fd12f581c948c77e5741e0461b03e273c0832e39d08261ea30c7c",
	"public-scanner-existing-scope":        "be41f72e73e7e4fe00d9f43bb79b06fc132a0aa49ecf3313bb7c4588008090f0",
	"registry-selection-integrity":         "04c25292d467beeb4ca300456646e40176a9eeddf1ef661e222f67f329041550",
	"archive-consumer-input-boundary":      "7f6db34a0a69382cc87dea4bd22b70082e1b8174af5420b2afc35ce4a3f8c754",
	"windows-archive-consumer-contract":    "922d940a4e3a2b5c1af6ec30fcbb699527c748c73228c666b9b2da43841a0a6b",
	"explicit-retention-inputs":            "08e248c398f784853d6e11edcf12f2258bdf3f24cf2af69d889ed676641ba974",
	"post-completion-recorded-facts":       "48b60639085d57874495120fbb07c219df1480b86942019a6b5ec63712b8bfcf",
	"spec-observation-boundary-contract":   "0c9597ca8215428be348a9b9f7d97c789272ba058c16cc3086c330a1c361e477",
	"seal-evidence-and-assurance-contract": "632dba8382a6e7bc7cd54983837c463664281b3558376777f8ba0957ef7d6735",
	"parent-result-adapter-meaning":        "48e4918ac6e7892b4426375865a37fa50a389bfb37a5ef4f9369270ae9504033",
	"runner-adapter-classification":        "d96405dac32f1456cec0fae40d99d5705cb6e43af63bd0b8dfa6f76acb9dc1fa",
}

// collects is a Python program that prints each FILE::SELECTOR argument from
// which unittest loads no test, as the registry's checks are run. Its files
// share the directory argv[1], so their module names cannot collide. The
// loader turns a missing name or a failed import into a placeholder test and
// an entry in its errors, so both count as loading nothing.
const collects = `import os, sys, unittest
sys.path.insert(0, sys.argv[1])
for pair in sys.argv[2:]:
    path, selector = pair.split("::", 1)
    name = os.path.splitext(os.path.basename(path))[0] + "." + selector
    loader = unittest.TestLoader()
    try:
        count = loader.loadTestsFromName(name).countTestCases()
    except Exception:
        count = 0
    if count == 0 or loader.errors:
        print(pair)
`

// unresolved returns the registry checks, as FILE::SELECTOR, that name no
// test: a Python selector unittest loads nothing from, a Go test its package
// does not list, or a file of any other kind. A tool that cannot run is an
// error.
func unresolved(t *testing.T, checks []string) map[string]bool {
	t.Helper()
	bad := map[string]bool{}
	python, golang := map[string][]string{}, map[string][]string{}
	for _, c := range checks {
		file, _, _ := strings.Cut(c, "::")
		switch dir := path.Dir(file); {
		case strings.HasSuffix(file, ".py"):
			python[dir] = append(python[dir], c)
		case strings.HasSuffix(file, "_test.go"):
			golang[dir] = append(golang[dir], c)
		default:
			bad[c] = true
		}
	}
	for dir, cs := range python {
		cmd := exec.Command("python3", append([]string{"-I", "-c", collects, at(dir)}, cs...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("cannot load the Python selectors in %s: %v", dir, err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				bad[line] = true
			}
		}
	}
	for dir, cs := range golang {
		cmd := exec.Command("go", "test", "-list", ".", "./"+dir+"/")
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("cannot list the Go tests in %s: %v", dir, err)
		}
		listed := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			listed[strings.TrimSpace(line)] = true
		}
		for _, c := range cs {
			if _, selector, _ := strings.Cut(c, "::"); !listed[selector] {
				bad[c] = true
			}
		}
	}
	return bad
}

func TestUnusedCheckersAC4(t *testing.T) {
	var reg struct {
		Routes []map[string]any `json:"routes"`
	}
	if err := json.Unmarshal([]byte(read(t, "contracts/regressions.json")), &reg); err != nil {
		t.Fatalf("cannot decode contracts/regressions.json: %v", err)
	}
	var all []string
	for _, route := range reg.Routes {
		checks, _ := route["checks"].([]any)
		for _, c := range checks {
			check, _ := c.(map[string]any)
			file, _ := check["file"].(string)
			selector, _ := check["selector"].(string)
			all = append(all, file+"::"+selector)
		}
	}
	bad := unresolved(t, all)
	var ids []string
	invariants := map[string]int{}
	for _, route := range reg.Routes {
		id, _ := route["id"].(string)
		inv, _ := route["invariant"].(string)
		ids = append(ids, id)
		invariants[inv]++
		checks, ok := route["checks"].([]any)
		if !ok {
			t.Fatalf("route %s has no checks list", id)
		}
		var kept []any
		for _, c := range checks {
			check, _ := c.(map[string]any)
			file, _ := check["file"].(string)
			selector, _ := check["selector"].(string)
			key := file + "::" + selector
			if bad[key] {
				t.Errorf("jaekit:violation=registry-route-stale route %s keeps %s, which names no test", id, key)
			}
			if id != optionalRoute || !optionalChecks[key] {
				kept = append(kept, c)
			}
		}
		route["checks"] = kept
		encoded, err := json.Marshal(route)
		if err != nil {
			t.Fatalf("cannot re-encode route %s: %v", id, err)
		}
		sum := sha256.Sum256(encoded)
		got := hex.EncodeToString(sum[:])
		switch expected, ok := want[id]; {
		case !ok:
			t.Errorf("jaekit:violation=registry-route-stale route %s is not one this goal keeps", id)
		case got != expected:
			t.Errorf("jaekit:violation=registry-route-stale route %s differs from what this goal keeps (sha256 %s)", id, got)
		}
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("jaekit:violation=registry-route-stale route %s appears twice", id)
		}
		seen[id] = true
	}
	var missing []string
	for id := range want {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		t.Errorf("jaekit:violation=registry-route-stale route %s is missing", id)
	}
	for _, id := range removedRoutes {
		if seen[id] {
			t.Errorf("jaekit:violation=registry-route-stale route %s still exists", id)
		}
	}
	for _, inv := range []string{"I1", "I2", "I3", "I4", "I5"} {
		if invariants[inv] == 0 {
			t.Errorf("jaekit:violation=registry-route-stale invariant %s has no route", inv)
		}
	}
}

func TestUnusedCheckersAC5(t *testing.T) {
	const doc = "tools/release/README.md"
	read(t, doc)
	for _, rel := range []string{"tools/release/archive_reader.py", "tools/release/check_archives.py"} {
		if !strings.Contains(read(t, rel), doc) {
			t.Errorf("jaekit:violation=archive-limits-unpublished %s does not point to %s", rel, doc)
		}
	}
	// The publication check reads the document itself.
	if !strings.Contains(read(t, "tools/release/test_archive_bounds.py"), `(ROOT / "`+doc+`").read_text(`) {
		t.Errorf("jaekit:violation=archive-limits-unpublished tools/release/test_archive_bounds.py does not read %s", doc)
	}
	run(t, "archive-limits-unpublished", "python3", "tools/release/test_archive_bounds.py")
}

// deletedNames are the names of the deleted paths that no tracked file may
// still use: the deleted directories, the verification group, and every
// deleted file name that no remaining file shares.
var deletedNames = []string{
	"release-checks", "release-0-1-3-followups", "release-0-1-3-review-fixes", "windows-entry-name-refusal",
	"AC-11.targets.json", "AC-6.targets.json", "AC-9.targets.json", "authoring_check.py", "ci_runs.py",
	"execution_provenance.py", "execution_trace.py", "install_trace.py", "integration_child_imports.py",
	"integration_core_observation.py", "integration_direct_install.py", "integration_evidence_boundaries.py",
	"integration_install_destination.py", "integration_install_imports.py", "integration_observe_imports.py",
	"integration_product_source.py", "observe.py", "preserved.json", "preserved.py", "shell_reader.py",
	"snapshot_worker.py", "source_identity.py", "test_archive_limits.py", "test_authoring_check.py",
	"test_direct_import_boundary.py", "test_entry_name_ci_runs.py", "test_history_audit.py",
	"test_import_provenance.py", "test_loader_invocations.py", "test_observation.py",
	"test_observed_invocations.py", "test_private_independence.py", "test_windows_assets.py",
	"authoring_check", "test_private_independence",
	"current-files.declaration.json", "current-files.targets.json",
}

// trackedContent returns what Git tracks for a path: a symbolic link's
// target text, or a file's bytes.
func trackedContent(rel string) ([]byte, error) {
	info, err := os.Lstat(at(rel))
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(at(rel))
		return []byte(target), err
	}
	return os.ReadFile(at(rel))
}

func TestUnusedCheckersAC7(t *testing.T) {
	for _, rel := range tracked(t) {
		// This goal's own checks name the deleted paths to observe them. SPEC
		// AC-7's exceptions (issue and commit links, what published release
		// notes already said) are not applied: no tracked file needed them, so
		// the scan can only be stricter than the criterion.
		if strings.HasPrefix(rel, self) {
			continue
		}
		for _, name := range deletedNames {
			if strings.Contains(rel, name) {
				t.Errorf("jaekit:violation=deleted-path-named tracked path %s has the name %s", rel, name)
			}
		}
		data, err := trackedContent(rel)
		if err != nil {
			t.Fatalf("cannot read tracked %s: %v", rel, err)
		}
		for _, name := range deletedNames {
			if bytes.Contains(data, []byte(name)) {
				t.Errorf("jaekit:violation=deleted-path-named %s names %s", rel, name)
			}
		}
	}
}

func TestUnusedCheckersAC8(t *testing.T) {
	if *base == "" {
		t.Skip("AC-8 reads the goal's change and needs -base")
	}
	out, err := exec.Command("git", "-C", root, "diff", "--numstat", *base, "HEAD").Output()
	if err != nil {
		t.Fatalf("cannot read the change from %s to HEAD: %v", *base, err)
	}
	added, deleted := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 || fields[0] == "-" {
			continue
		}
		a, errA := strconv.Atoi(fields[0])
		d, errD := strconv.Atoi(fields[1])
		if errA != nil || errD != nil {
			t.Fatalf("cannot read numstat line %q", line)
		}
		added += a
		deleted += d
	}
	if net := added - deleted; net > -9000 {
		t.Errorf("jaekit:violation=net-reduction-short the change is %+d lines (+%d -%d); it must be -9000 or fewer", net, added, deleted)
	}
}
