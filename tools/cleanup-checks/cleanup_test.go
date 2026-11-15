//go:build goalcheckcleanup

package cleanupchecks

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// AC-1: none of the removed folders has a tracked file.
func TestCleanupAC1_Folders(t *testing.T) {
	root := repoRoot(t)
	files := trackedFiles(t, root)
	if !hasPrefix(files, goalCheckRoot+"/") {
		t.Fatalf("setup: the checked tree has no tracked file under %s", goalCheckRoot)
	}
	var present []string
	for _, name := range removedFolders {
		if hasPrefix(files, goalCheckRoot+"/"+name+"/") {
			present = append(present, goalCheckRoot+"/"+name)
		}
	}
	if len(present) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=finished-goal-folder-present: these folders still have tracked files:", present))
	}
}

// AC-1: the subfolders of tools/goal-checks are exactly the five kept ones,
// and their files keep content, path and executable bit.
func TestCleanupAC1_KeptFolders(t *testing.T) {
	root := repoRoot(t)
	files := trackedFiles(t, root)
	var problems []string
	got := subfolders(files, goalCheckRoot+"/")
	if want := strings.Join(keptFolders, " "); strings.Join(got, " ") != want {
		problems = append(problems, "subfolders are "+strings.Join(got, " ")+", want "+want)
	}
	kept := map[string]bool{}
	for _, name := range keptFolders {
		kept[name] = true
	}
	problems = append(problems, compareFiles(t, root, files, func(path string) bool {
		rest, ok := strings.CutPrefix(path, goalCheckRoot+"/")
		name, _, nested := strings.Cut(rest, "/")
		return ok && nested && kept[name]
	}, keptFolderFiles)...)
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=kept-folder-changed: the kept folders differ from the expected files:", problems))
	}
}

// folderReference matches goal-checks/<name> with either separator, with or
// without the tools/ prefix. The caller checks the name boundary.
var folderReference = regexp.MustCompile(`goal-checks[/\\]+([A-Za-z0-9_.-]+)`)

// AC-2: no tracked text file names a removed folder's path.
func TestCleanupAC2_PathReferences(t *testing.T) {
	root := repoRoot(t)
	removed := map[string]bool{}
	for _, name := range removedFolders {
		removed[name] = true
	}
	var found []string
	for _, file := range trackedFiles(t, root) {
		var data []byte
		if file.mode == "120000" {
			target, err := os.Readlink(filepath.Join(root, filepath.FromSlash(file.path)))
			if err != nil {
				t.Fatalf("setup: read link %s: %v", file.path, err)
			}
			data = []byte(target)
		} else {
			data = readFile(t, root, file.path)
		}
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue // binary, outside the boundary
		}
		for _, line := range referencingLines(string(data), removed) {
			found = append(found, file.path+": "+line)
		}
	}
	if len(found) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=deleted-folder-referenced: these lines name removed folders:", found))
	}
}

// referencingLines returns each line that names a removed folder as a path.
// The name ends at a character that cannot continue a folder name; a period
// ends it only when no letter or digit follows.
func referencingLines(text string, removed map[string]bool) []string {
	var lines []string
	for number, line := range strings.Split(text, "\n") {
		for _, match := range folderReference.FindAllStringSubmatch(line, -1) {
			name := match[1]
			for strings.HasSuffix(name, ".") {
				name = strings.TrimSuffix(name, ".")
			}
			if base, _, ok := strings.Cut(name, "."); ok {
				name = base // a file name such as goal-checks/AC-1.json
			}
			if removed[name] {
				lines = append(lines, strings.TrimSpace(strings.Join([]string{itoa(number + 1), line}, ": ")))
				break
			}
		}
	}
	return lines
}

// AC-2: the two files that kept tracked files read by path keep their names.
func TestCleanupAC2_KeptNames(t *testing.T) {
	root := repoRoot(t)
	tracked := map[string]bool{}
	for _, file := range trackedFiles(t, root) {
		tracked[file.path] = true
	}
	var missing []string
	for _, path := range []string{"plugins/compatibility_test.go", "plugins/current_release_test.go"} {
		if !tracked[path] {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=kept-test-file-renamed: these files are not tracked under their names:", missing))
	}
}

// AC-3: every tracked file directly under tools/goal-checks is unchanged.
func TestCleanupAC3_RootFiles(t *testing.T) {
	root := repoRoot(t)
	problems := compareFiles(t, root, trackedFiles(t, root), func(path string) bool {
		rest, ok := strings.CutPrefix(path, goalCheckRoot+"/")
		return ok && !strings.Contains(rest, "/")
	}, rootFiles)
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("the files directly under "+goalCheckRoot+" differ from the expected files:", problems))
	}
}

// oldTestFiles are the 28 plugins test files named after goals or releases.
var oldTestFiles = []string{
	"bundle_readability", "check_cost", "core_integrity", "progress_timeline", "readme_demo_user_steps",
	"readme_first_run_kept", "readme_first_run", "readme_flow_visuals", "readme_onboarding",
	"readme_visual_guide", "release_0_1_1", "release_0_1_3", "release_0_1_7", "release_0_1_8",
	"review_findings_report", "review_summary_followups", "review_summary", "reviewer_standpoints",
	"seal_questions_regression", "seal_questions_up_front", "seal_slim_obligations",
	"seal_slim_review_tiers", "spar_agent_model", "spar_agy_readonly", "spar_cap_report",
	"spar_onboarding", "spar_packet_decisions", "spar_smallest_fix",
}

// stayingTestFiles are the plugins test files the cleanup does not rename.
var stayingTestFiles = []string{
	"compatibility", "criteria_rules", "current_release", "examples_index", "invocation", "main",
	"nested_criteria", "plugins", "release_guide", "request_boundary", "run_rules_2",
	"seal_completion", "spar_plugin", "spec_boundaries_contract", "spec_cases", "spec_direct_save",
	"spec_evaluation", "spec_questions", "verification_design", "verification_finding_docs",
	"verification_reporting",
}

// propertyFiles are the new file names and the category each one names on
// its first comment line.
var propertyFiles = map[string]string{
	"release_version":   "current release version and tag",
	"install_commands":  "install, update and uninstall commands",
	"manifest_versions": "manifest and marketplace versions",
	"seal_skill_rules":  "Seal skill rules",
	"spec_skill_rules":  "Spec skill rules",
	"spar_skill_rules":  "Spar skill rules",
	"doc_links":         "document links and language pairs",
	"doc_helpers":       "shared document helpers",
}

// AC-4: no goal-named test file remains in plugins/.
func TestCleanupAC4_OldNames(t *testing.T) {
	root := repoRoot(t)
	names := pluginTestNames(trackedFiles(t, root))
	if len(names) == 0 {
		t.Fatalf("setup: the checked tree has no plugins test files")
	}
	var present []string
	for _, name := range oldTestFiles {
		if names[name] {
			present = append(present, "plugins/"+name+"_test.go")
		}
	}
	if len(present) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=goal-named-test-file: these goal-named test files remain:", present))
	}
}

// AC-4: every plugins test file is a staying file or a property file whose
// first comment line names its category.
func TestCleanupAC4_PropertyFiles(t *testing.T) {
	root := repoRoot(t)
	names := pluginTestNames(trackedFiles(t, root))
	staying := map[string]bool{}
	for _, name := range stayingTestFiles {
		staying[name] = true
	}
	old := map[string]bool{}
	for _, name := range oldTestFiles {
		old[name] = true
	}
	var problems []string
	for _, name := range sortedKeys(names) {
		path := "plugins/" + name + "_test.go"
		category, property := propertyFiles[name]
		switch {
		case staying[name] || old[name]:
		case !property:
			problems = append(problems, path+" is neither a staying file nor a property file")
		default:
			if got := firstComment(string(readFile(t, root, path))); got != "// Property: "+category {
				problems = append(problems, path+" starts with "+quote(got)+", want "+quote("// Property: "+category))
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=test-file-without-property: these test files are not named for a property:", problems))
	}
}

// AC-5: the plugins tests bound in contracts/regressions.json and the map
// itself are unchanged.
func TestCleanupAC5_ProtectedFiles(t *testing.T) {
	root := repoRoot(t)
	problems := compareFiles(t, root, trackedFiles(t, root), func(path string) bool {
		_, ok := protectedFiles[path]
		return ok
	}, protectedFiles)
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("the protected files differ from the expected files:", problems))
	}
}

// AC-8: AGENTS.md states the goal check lifetime rule in one item of
// "## Checks and review", and the executable-checks rule stays.
func TestCleanupAC8_LifetimeRule(t *testing.T) {
	root := repoRoot(t)
	text := string(readFile(t, root, "AGENTS.md"))
	var problems []string
	const kept = "- Put executable checks in tracked paths outside root `docs/` so baseline checks can run from the code commit."
	if !containsLine(text, kept) {
		problems = append(problems, "the rule "+quote(kept)+" is missing")
	}
	tokens := []string{
		"A check that a goal adds is not deleted before that goal's completion record.",
		"Once the goal's pull request is merged, each check it added either becomes a regression,",
		"bound in `contracts/regressions.json` or placed where the full verification or a required CI job runs it,",
		"or is deleted in the next change.",
		"This applies wherever the check lives, inside or outside `tools/goal-checks/`.",
	}
	rules, ok := section(text, "## Checks and review")
	item := ""
	if ok {
		for _, candidate := range listItems(rules) {
			if strings.Contains(candidate, tokens[0]) {
				item = candidate
				break
			}
		}
	}
	switch {
	case !ok:
		problems = append(problems, "AGENTS.md has no \"## Checks and review\" section")
	case item == "":
		problems = append(problems, "no item of \"## Checks and review\" contains "+quote(tokens[0]))
	default:
		for _, token := range tokens[1:] {
			if !strings.Contains(item, token) {
				problems = append(problems, "the lifetime item lacks "+quote(token))
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=lifetime-rule-missing: AGENTS.md lacks the goal check lifetime rule:", problems))
	}
}

// testLink is a link from a guide to a plugins test, as the development
// guides write it.
var testLink = regexp.MustCompile("\\[`(Test[A-Za-z0-9_]+)`\\]\\(\\.\\./plugins/([a-z0-9_]+_test\\.go)\\)")

// AC-9: the development guides differ from the expected content only in the
// test names and link targets of their test links, both guides link the same
// tests, and each linked test is defined in the file the link names.
func TestCleanupAC9_DevelopmentGuides(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	var linked []string
	for _, path := range sortedKeys(developmentGuides) {
		text := string(readFile(t, root, path))
		masked := testLink.ReplaceAllString(text, "[`TEST`](../plugins/FILE)")
		if digest([]byte(masked)) != developmentGuides[path] {
			problems = append(problems, path+" changed outside its test links")
		}
		var links []string
		for _, match := range testLink.FindAllStringSubmatch(text, -1) {
			links = append(links, match[1]+" "+match[2])
			source := string(readFile(t, root, "plugins/"+match[2]))
			if !regexp.MustCompile(`(?m)^func ` + match[1] + `\(t \*testing\.T\) \{`).MatchString(source) {
				problems = append(problems, path+" links "+match[1]+" to plugins/"+match[2]+", which does not define it")
			}
		}
		if linked == nil {
			linked = links
		} else if strings.Join(linked, ",") != strings.Join(links, ",") {
			problems = append(problems, "the two guides link different tests: "+strings.Join(linked, ", ")+" and "+strings.Join(links, ", "))
		}
	}
	if len(linked) == 0 {
		problems = append(problems, "the guides link no plugins test")
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("the development guides differ from the expected content:", problems))
	}
}

// AC-11: under the local docs/ of this working tree, every file is inside
// this goal's folder, and docs/ has no tracked file.
func TestCleanupAC11_LocalDocs(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	for _, file := range trackedFiles(t, root) {
		if strings.HasPrefix(file.path, "docs/") {
			problems = append(problems, "tracked "+file.path)
		}
	}
	own := filepath.Join(root, "docs", "specs", "goal-check-cleanup")
	if _, err := os.Stat(filepath.Join(own, "SPEC.md")); err != nil {
		t.Fatalf("setup: this goal's SPEC.md is not under %s: %v", own, err)
	}
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == own {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			problems = append(problems, "outside this goal's folder: "+filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: walk docs/: %v", err)
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("local docs/ has files outside this goal's folder:", problems))
	}
}

// completionRow is one row of the goal's completion-commits.md.
var completionRow = regexp.MustCompile("(?m)^\\| `([a-z0-9.-]+)` \\| `([0-9a-f]{7,40})` \\|$")

// AC-12: each removed folder has, in HEAD's ancestry, the commit that removed
// it; that commit's message names the folder and its completion commit, and
// the completion commit is an ancestor of HEAD whose tree has the folder.
func TestCleanupAC12_DeletionTrail(t *testing.T) {
	root := repoRoot(t)
	if strings.TrimSpace(gitOutput(t, root, "rev-parse", "--is-shallow-repository")) != "false" {
		t.Fatalf("setup: the repository is shallow, so its history cannot show the removal commits")
	}
	list := string(readFile(t, root, "docs/specs/goal-check-cleanup/completion-commits.md"))
	completion := map[string]string{}
	for _, row := range completionRow.FindAllStringSubmatch(list, -1) {
		completion[row[1]] = row[2]
	}
	for _, name := range removedFolders {
		if completion[name] == "" {
			t.Fatalf("setup: completion-commits.md has no row for %s", name)
		}
	}
	files := trackedFiles(t, root)
	var problems []string
	for _, name := range removedFolders {
		folder := goalCheckRoot + "/" + name
		commit := completion[name]
		if hasPrefix(files, folder+"/") {
			problems = append(problems, folder+" is still in the tree")
			continue
		}
		removal := strings.TrimSpace(gitOutput(t, root, "log", "-1", "--format=%H", "--no-renames",
			"--diff-filter=D", "HEAD", "--", folder))
		if removal == "" {
			problems = append(problems, "no commit in HEAD's ancestry removed "+folder)
			continue
		}
		message := gitOutput(t, root, "log", "-1", "--format=%B", removal)
		if !messageNames(message, name, commit) {
			problems = append(problems, removal+" removed "+folder+" but its message has no line naming "+name+" and "+commit)
		}
		if gitStatus(t, root, "cat-file", "-e", commit+"^{commit}") != 0 {
			problems = append(problems, "the completion commit "+commit+" of "+name+" is not in the repository")
			continue
		}
		if gitStatus(t, root, "merge-base", "--is-ancestor", commit, "HEAD") != 0 {
			problems = append(problems, "the completion commit "+commit+" of "+name+" is not an ancestor of HEAD")
		}
		if strings.TrimSpace(gitOutput(t, root, "ls-tree", "-r", "--name-only", commit, "--", folder+"/")) == "" {
			problems = append(problems, "the tree of "+commit+" has no files under "+folder)
		}
	}
	if len(problems) > 0 {
		t.Errorf("%s", joinLines("jaekit:violation=deletion-trail-missing: the history does not lead back to every removed folder's checks:", problems))
	}
}

// messageNames reports whether a line of the message has the folder name and
// the commit as whole words.
func messageNames(message, folder, commit string) bool {
	for _, line := range strings.Split(message, "\n") {
		fields := strings.Fields(line)
		hasFolder, hasCommit := false, false
		for _, field := range fields {
			hasFolder = hasFolder || field == folder
			hasCommit = hasCommit || field == commit
		}
		if hasFolder && hasCommit {
			return true
		}
	}
	return false
}

func hasPrefix(files []trackedFile, prefix string) bool {
	for _, file := range files {
		if strings.HasPrefix(file.path, prefix) {
			return true
		}
	}
	return false
}

// subfolders lists the folder names directly under prefix that hold files.
func subfolders(files []trackedFile, prefix string) []string {
	seen := map[string]bool{}
	for _, file := range files {
		rest, ok := strings.CutPrefix(file.path, prefix)
		if name, _, nested := strings.Cut(rest, "/"); ok && nested {
			seen[name] = true
		}
	}
	return sortedKeys(seen)
}

// pluginTestNames lists the test files directly under plugins/ without the
// _test.go suffix.
func pluginTestNames(files []trackedFile) map[string]bool {
	names := map[string]bool{}
	for _, file := range files {
		rest, ok := strings.CutPrefix(file.path, "plugins/")
		if name, test := strings.CutSuffix(rest, "_test.go"); ok && test && !strings.Contains(name, "/") {
			names[name] = true
		}
	}
	return names
}

// firstComment is the first comment line that is not a build constraint.
func firstComment(source string) string {
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "//go:build") && !strings.HasPrefix(line, "// +build") {
			return line
		}
	}
	return ""
}

// listItems splits Markdown into top-level list items with their
// continuation lines.
func listItems(text string) []string {
	var items []string
	current := ""
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "- "):
			if current != "" {
				items = append(items, current)
			}
			current = line
		case current != "" && strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#"):
			current += "\n" + line
		default:
			if current != "" {
				items = append(items, current)
			}
			current = ""
		}
	}
	if current != "" {
		items = append(items, current)
	}
	return items
}

func containsLine(text, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimRight(line, "\r") == want {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func quote(s string) string { return "\"" + s + "\"" }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}
