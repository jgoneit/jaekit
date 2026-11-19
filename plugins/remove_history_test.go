package plugins

// Checks that the decision-history feature is gone from the documents,
// checks and module (goal remove-decision-history, AC-5, AC-7, AC-8). A
// violation starts with jaekit:violation=<id>; a file that must exist and
// cannot be read fails without one.
import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The one sentence each usage guide keeps about records stored by an
// earlier release.
const (
	storedNoteKo = "이전 배포판이 공통 Git 폴더의 `ha/history/`에 남긴 판단 기록은 지워지지 않습니다. 읽으려면 그 기능이 있는 마지막 배포판 `v0.1.8`을 씁니다."
	storedNoteEn = "Decision records an earlier release left under `ha/history/` in the common Git directory are not deleted. To read them, use `v0.1.8`, the last release with that feature."
)

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("setup: read %s: %v", rel, err)
	}
	return string(data)
}

func absent(rel string) bool {
	_, err := os.Lstat(filepath.Join("..", filepath.FromSlash(rel)))
	return os.IsNotExist(err)
}

// TestRemoveHistoryAC5_NoHistoryDocs: the contract is gone, and no contract,
// guide, Seal skill file or the site builder names the feature. The only
// other "판단 이력" is the findings sense in guides/OPERATIONS.md. Links to
// the removed files are caught by tools/check_public_docs.py.
func TestRemoveHistoryAC5_NoHistoryDocs(t *testing.T) {
	var problems []string
	for _, rel := range []string{"contracts/decision-history.md", "contracts/decision-history.en.md"} {
		if !absent(rel) {
			problems = append(problems, rel+" exists")
		}
	}
	var files []string
	for _, root := range []string{"contracts", "guides", "plugins/seal"} {
		err := filepath.WalkDir(filepath.Join("..", root), func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() && (root == "plugins/seal" || strings.HasSuffix(path, ".md")) {
				rel, _ := filepath.Rel("..", path)
				files = append(files, filepath.ToSlash(rel))
			}
			return err
		})
		if err != nil {
			t.Fatalf("setup: walk %s: %v", root, err)
		}
	}
	files = append(files, "tools/site/build.py")
	for _, rel := range files {
		text := readRepo(t, rel)
		if rel == "guides/OPERATIONS.md" {
			text = strings.ReplaceAll(text, "재현 범위·판단 이력·재작업 요청", "")
		}
		for n, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			for _, token := range []string{"ha history", "decision-history", "decision_history", "decision history", "판단 이력"} {
				if strings.Contains(lower, token) {
					problems = append(problems, rel+":"+strconv.Itoa(n+1)+": "+token)
				}
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=history-docs-present: %s", strings.Join(problems, "\n"))
	}
}

// TestRemoveHistoryAC5_StoredHistoryNote: each usage guide keeps its one
// sentence about the stored records.
func TestRemoveHistoryAC5_StoredHistoryNote(t *testing.T) {
	var problems []string
	if !strings.Contains(readRepo(t, "guides/USAGE.md"), storedNoteKo) {
		problems = append(problems, "guides/USAGE.md lacks the stored-records sentence")
	}
	if !strings.Contains(readRepo(t, "guides/USAGE.en.md"), storedNoteEn) {
		problems = append(problems, "guides/USAGE.en.md lacks the stored-records sentence")
	}
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=stored-history-note-missing: %s", strings.Join(problems, "\n"))
	}
}

// TestRemoveHistoryAC7_NoHistoryChecks: the history checks, scenarios,
// native-check requirements and skill duties are gone, and AGENTS.md no
// longer cites the removed observer. The required CI jobs are judged by the
// pull request's required checks.
func TestRemoveHistoryAC7_NoHistoryChecks(t *testing.T) {
	var problems []string
	for _, rel := range []string{"tools/history-checks", "tools/workflow-checks/test_history_ci_observer.py",
		"testdata/scenarios/decision-history-flow", "testdata/scenarios/history-rules-unsupported"} {
		if !absent(rel) {
			problems = append(problems, rel+" exists")
		}
	}
	for rel, tokens := range map[string][]string{
		"tools/windows/native_checks.py":      {"internal/history", "TestNativeHistory"},
		"tools/windows/test_native_checks.py": {"TestNativeHistory"},
		"testdata/scenarios/README.md":        {"decision-history-flow", "history-rules-unsupported"},
		"plugins/seal_skill_rules_test.go":    {`"history-`},
		"AGENTS.md":                           {"tools/history-checks"},
	} {
		text := readRepo(t, rel)
		for _, token := range tokens {
			if strings.Contains(text, token) {
				problems = append(problems, rel+": "+token)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=history-checks-present: %s", strings.Join(problems, "\n"))
	}
}

// TestRemoveHistoryAC8_NoSysDependency: the module no longer requires
// golang.org/x/sys. The four-platform and Windows candidate builds run in
// tools/verify.py.
func TestRemoveHistoryAC8_NoSysDependency(t *testing.T) {
	var problems []string
	for _, rel := range []string{"go.mod", "go.sum"} {
		if strings.Contains(readRepo(t, rel), "golang.org/x/sys") {
			problems = append(problems, rel+" names golang.org/x/sys")
		}
	}
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=sys-dependency-present: %s", strings.Join(problems, "\n"))
	}
}
