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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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

// requireAbsent reports each path that still exists. A path that cannot be
// inspected is an error of the check, not a violation.
func requireAbsent(t *testing.T, violation string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		_, err := os.Lstat(at(rel))
		switch {
		case err == nil:
			t.Errorf("jaekit:violation=%s %s still exists", violation, rel)
		case !errors.Is(err, fs.ErrNotExist):
			t.Fatalf("cannot inspect %s: %v", rel, err)
		}
	}
}

// runPython runs one Python script from the repository root and reports a
// nonzero exit as the violation. A script that cannot start is an error.
func runPython(t *testing.T, violation string, args ...string) {
	t.Helper()
	cmd := exec.Command("python3", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		t.Errorf("jaekit:violation=%s %s exited %d\n%s", violation, args[0], exit.ExitCode(), out)
	default:
		t.Fatalf("cannot run %s: %v", args[0], err)
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
	if strings.Contains(read(t, "plugins/english_contracts_test.go"), "authoring_check") {
		t.Errorf("jaekit:violation=authoring-producer-present plugins/english_contracts_test.go still loads authoring_check")
	}
	// The same suites the plugins Go tests run for the remaining producers.
	runPython(t, "authoring-producer-present", "tools/contract-checks/test_check.py", "reporting")
	runPython(t, "authoring-producer-present", "tools/contract-checks/test_design_check.py")
	runPython(t, "authoring-producer-present", "tools/contract-checks/test_finding_docs_check.py")
	runPython(t, "authoring-producer-present", "tools/contract-checks/test_spec_check.py")
}

// gone lists the deleted paths; a registry route may name none of them.
var gone = []string{
	"tools/release-checks/",
	"tools/goal-checks/release-0-1-3-followups/",
	"tools/goal-checks/release-0-1-3-review-fixes/",
	"tools/goal-checks/windows-entry-name-refusal/",
	"tools/contract-checks/authoring_check.py",
	"tools/goal-checks/test_authoring_check.py",
	"tools/goal-checks/test_entry_name_ci_runs.py",
	"tools/goal-checks/test_history_audit.py",
	"tools/review-checks/test_observation.py",
	"tools/gap-checks/test_observation.py",
}

// auditChecks are the checks that ran the deleted audits.
var auditChecks = []string{
	"tools/review-checks/test_producers.py::AC9",
	"tools/review-checks/test_producers.py::AC10.test_both_audits_require_an_explicit_range_and_do_not_fetch_missing_history",
	"tools/gap-checks/test_history_results.py::AC4",
	"tools/gap-checks/test_history_results.py::AC5",
}

var removedRoutes = []string{
	"native-request-and-uncertainty", "shell-reachability-gap", "observation-identity-gap",
	"release-checker-import-boundary", "entry-name-ci-observation",
	"authoring-check-current-tree", "authoring-check-result-meaning",
}

// kept lists the sources and checks each narrowed route keeps.
var kept = map[string][2][]string{
	"public-scanner-path-gap": {
		{"tools/check_public_tree.py", "tools/gap-checks/preservation.py"},
		{"tools/public-checks/test_paths.py::CurrentPathChecks", "tools/public-checks/test_paths.py::HistoryPathChecks",
			"tools/public-checks/test_paths.py::NameChecks", "tools/public-checks/test_paths.py::PreservedBoundaries"},
	},
	"archive-consumer-input-boundary": {
		{"tools/release/archive_reader.py", "tools/release/check_archives.py"},
		{"tools/release/test_archive_bounds.py::ValidatorBounds", "tools/release/test_archive_bounds.py::Preservation"},
	},
	"shared-tool-preflight": {
		{"tools/verify.py", "tools/review-checks/test_installation.py", ".github/workflows/ci.yml"},
		{"tools/gap-checks/test_docs_environment.py::AC8"},
	},
	"current-file-independence": {
		{"tools/goal-checks/produce.py", "tools/review-checks/produce.py", "tools/gap-checks/produce.py", "tools/gap-checks/preservation.py"},
		{"tools/workflow-checks/test_history_less_checkouts.py::HistoryLessCheckouts"},
	},
}

// unchanged holds the SHA-256 of each other route's JSON, re-encoded with
// sorted keys, as the registry had it before this goal.
var unchanged = map[string]string{
	"stored-proof-independence":            "a17d264e92433ecc3471f870ece2394f6c58aa990b8c55658e97732b16144746",
	"structured-classification":            "0afbb490a9081522ab4869f906fabbe3ad6d06e6ab5f22e779c4b726637395e2",
	"stored-result-judgment":               "46c5a474aee798ad5474cfe6962162f6cfa1f66b0bbec7e5ce742be13741ce0b",
	"reference-producer-to-cli":            "089ddbeb506a7861a74f0fb76aed31a24179047aa2c425597f951a9ccfff073f",
	"goal-producer-meaning":                "8d377b6b12e170aa864af6b4bb2cd73bae447735b6bc2043b3ee4292880ccf4e",
	"review-producer-meaning":              "d0435706b318182b96c46488e20272a52a67a74330807ae4ffad826e194f1f38",
	"gap-producer-meaning":                 "7e1ce5e038d3d34ff285da8cb028a26568adcde125c50d9a390ef30c647c8d51",
	"shared-command-failures":              "93acbf82611fd12f581c948c77e5741e0461b03e273c0832e39d08261ea30c7c",
	"public-scanner-existing-scope":        "be41f72e73e7e4fe00d9f43bb79b06fc132a0aa49ecf3313bb7c4588008090f0",
	"registry-selection-integrity":         "04c25292d467beeb4ca300456646e40176a9eeddf1ef661e222f67f329041550",
	"windows-archive-consumer-contract":    "922d940a4e3a2b5c1af6ec30fcbb699527c748c73228c666b9b2da43841a0a6b",
	"explicit-retention-inputs":            "08e248c398f784853d6e11edcf12f2258bdf3f24cf2af69d889ed676641ba974",
	"post-completion-recorded-facts":       "48b60639085d57874495120fbb07c219df1480b86942019a6b5ec63712b8bfcf",
	"spec-observation-boundary-contract":   "0c9597ca8215428be348a9b9f7d97c789272ba058c16cc3086c330a1c361e477",
	"seal-evidence-and-assurance-contract": "632dba8382a6e7bc7cd54983837c463664281b3558376777f8ba0957ef7d6735",
	"parent-result-adapter-meaning":        "48e4918ac6e7892b4426375865a37fa50a389bfb37a5ef4f9369270ae9504033",
	"runner-adapter-classification":        "d96405dac32f1456cec0fae40d99d5705cb6e43af63bd0b8dfa6f76acb9dc1fa",
}

func isGone(path string) bool {
	for _, g := range gone {
		if path == g || strings.HasSuffix(g, "/") && strings.HasPrefix(path, g) {
			return true
		}
	}
	return false
}

func TestUnusedCheckersAC4(t *testing.T) {
	var reg struct {
		Routes []map[string]any `json:"routes"`
	}
	if err := json.Unmarshal([]byte(read(t, "contracts/regressions.json")), &reg); err != nil {
		t.Fatalf("cannot decode contracts/regressions.json: %v", err)
	}
	seen := map[string]bool{}
	invariants := map[string]int{}
	for _, route := range reg.Routes {
		id, _ := route["id"].(string)
		inv, _ := route["invariant"].(string)
		seen[id] = true
		invariants[inv]++
		var sources, checks []string
		for _, s := range route["sources"].([]any) {
			sources = append(sources, s.(string))
		}
		for _, c := range route["checks"].([]any) {
			check := c.(map[string]any)
			file, _ := check["file"].(string)
			selector, _ := check["selector"].(string)
			checks = append(checks, file+"::"+selector)
			if isGone(file) {
				t.Errorf("jaekit:violation=registry-route-stale route %s checks with deleted %s", id, file)
			}
			for _, audit := range auditChecks {
				if file+"::"+selector == audit {
					t.Errorf("jaekit:violation=registry-route-stale route %s still runs the deleted audits through %s", id, audit)
				}
			}
		}
		for _, s := range sources {
			if isGone(s) {
				t.Errorf("jaekit:violation=registry-route-stale route %s has deleted source %s", id, s)
			}
		}
		if want, ok := kept[id]; ok {
			for _, s := range want[0] {
				if !contains(sources, s) {
					t.Errorf("jaekit:violation=registry-route-stale route %s lost source %s", id, s)
				}
			}
			for _, c := range want[1] {
				if !contains(checks, c) {
					t.Errorf("jaekit:violation=registry-route-stale route %s lost check %s", id, c)
				}
			}
		}
		if want, ok := unchanged[id]; ok {
			encoded, err := json.Marshal(route)
			if err != nil {
				t.Fatalf("cannot re-encode route %s: %v", id, err)
			}
			sum := sha256.Sum256(encoded)
			if got := hex.EncodeToString(sum[:]); got != want {
				t.Errorf("jaekit:violation=registry-route-stale route %s changed (sha256 %s)", id, got)
			}
		}
	}
	for _, id := range removedRoutes {
		if seen[id] {
			t.Errorf("jaekit:violation=registry-route-stale route %s still exists", id)
		}
	}
	for id := range kept {
		if !seen[id] {
			t.Errorf("jaekit:violation=registry-route-stale route %s is missing", id)
		}
	}
	for id := range unchanged {
		if !seen[id] {
			t.Errorf("jaekit:violation=registry-route-stale route %s is missing", id)
		}
	}
	for _, inv := range []string{"I1", "I2", "I3", "I4", "I5"} {
		if invariants[inv] == 0 {
			t.Errorf("jaekit:violation=registry-route-stale invariant %s has no route", inv)
		}
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func TestUnusedCheckersAC5(t *testing.T) {
	const doc = "tools/release/README.md"
	if !strings.Contains(read(t, doc), "## Archive resource limits") {
		t.Errorf("jaekit:violation=archive-limits-unpublished %s has no Archive resource limits section", doc)
	}
	for _, rel := range []string{"tools/release/archive_reader.py", "tools/release/check_archives.py", "tools/release/test_archive_bounds.py"} {
		if !strings.Contains(read(t, rel), doc) {
			t.Errorf("jaekit:violation=archive-limits-unpublished %s does not point to %s", rel, doc)
		}
	}
	runPython(t, "archive-limits-unpublished", "tools/release/test_archive_bounds.py")
}

// deletedNames are the names of the deleted paths that no tracked file may
// still use, whether as a full path, a directory group or a file name.
var deletedNames = []string{
	"release-checks", "release-0-1-3-followups", "release-0-1-3-review-fixes", "windows-entry-name-refusal",
	"authoring_check", "test_entry_name_ci_runs", "test_history_audit", "test_observation", "test_private_independence",
}

func TestUnusedCheckersAC7(t *testing.T) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("cannot list tracked files: %v", err)
	}
	for _, rel := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		// This goal's own checks name the deleted paths to observe them, and
		// published release notes keep their history.
		if rel == "" || strings.HasPrefix(rel, self) || strings.HasPrefix(rel, "tools/release/notes/") {
			continue
		}
		data, err := os.ReadFile(at(rel))
		if err != nil {
			t.Fatalf("cannot read tracked %s: %v", rel, err)
		}
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue
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
