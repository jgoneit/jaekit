package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// ecContracts are the contracts that stay, one English file each.
var ecContracts = []string{"README", "bundle", "check-result", "core-capabilities", "findings", "goal-docs", "regressions",
	"repository-producers", "review-gate", "review-round", "role-card", "run-record", "spar-config", "spar-input"}

func ecRead(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func ecHangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// ecParts splits Markdown into the lines outside code blocks with their code spans removed, the code spans,
// and the code block lines (fences included). A code span opens and closes with backtick runs of the same length.
func ecParts(text string) (prose, spans, blocks []string) {
	inBlock := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			inBlock = !inBlock
			blocks = append(blocks, line)
			continue
		}
		if inBlock {
			blocks = append(blocks, line)
			continue
		}
		var rest strings.Builder
		for i := 0; i < len(line); {
			n := ecRun(line, i)
			if n == 0 {
				rest.WriteByte(line[i])
				i++
				continue
			}
			end := -1
			for j := i + n; j < len(line); {
				if m := ecRun(line, j); m == n {
					end = j
					break
				} else if m > 0 {
					j += m
				} else {
					j++
				}
			}
			if end < 0 {
				rest.WriteString(line[i : i+n])
				i += n
				continue
			}
			spans = append(spans, strings.TrimSpace(line[i+n:end]))
			i = end + n
		}
		prose = append(prose, rest.String())
	}
	return prose, spans, blocks
}

func ecRun(s string, i int) int {
	n := 0
	for i+n < len(s) && s[i+n] == '`' {
		n++
	}
	return n
}

func TestEnglishContractsAC1_OneEnglishFile(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "contracts"))
	if err != nil {
		t.Fatalf("read contracts: %v", err)
	}
	have := map[string]bool{}
	var docs []string
	for _, e := range entries {
		name := e.Name()
		have[name] = true
		switch {
		case strings.HasSuffix(name, ".en.md") || strings.HasSuffix(name, ".ko.md"):
			t.Errorf("jaekit:violation=two-language-contract: contracts/%s exists; each contract is one English file", name)
		case strings.HasSuffix(name, ".md"):
			docs = append(docs, name)
		}
	}
	for _, need := range append(ecContracts, "regressions.json") {
		if !strings.HasSuffix(need, ".json") {
			need += ".md"
		}
		if !have[need] {
			t.Errorf("jaekit:violation=two-language-contract: contracts/%s is missing", need)
		}
	}
	for _, name := range docs {
		rel := "contracts/" + name
		text := ecRead(t, rel)
		prose, _, _ := ecParts(text)
		var korean []string
		for _, line := range prose {
			if ecHangul(line) {
				korean = append(korean, line)
			}
		}
		if len(korean) > 0 {
			t.Errorf("jaekit:violation=two-language-contract: %s has %d lines with Korean outside code spans and code blocks; first: %q", rel, len(korean), korean[0])
		}
	}
}

// ecTracked returns the tracked files outside docs/ and this file, with their text; binary files are skipped.
func ecTracked(t *testing.T) map[string]string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	files := map[string]string{}
	for _, rel := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if strings.HasPrefix(rel, "docs/") || rel == "plugins/english_contracts_test.go" {
			continue
		}
		b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.ContainsRune(string(b), 0) {
			files[rel] = string(b)
		}
	}
	return files
}

const ecRelease = `blob/v\d+\.\d+\.\d+/`

func TestEnglishContractsAC4_LinksOpen(t *testing.T) {
	names := strings.Join(ecContracts, "|")
	removed := regexp.MustCompile(`(` + ecRelease + `)?contracts/(?:` + names + `)\.en\.md|(` + ecRelease + `contracts/)?regressions\.ko\.md`)
	sibling := regexp.MustCompile(`\]\((?:` + names + `)\.en\.md`)
	onMain := regexp.MustCompile(`https://github\.com/jgoneit/jaekit/blob/main/(contracts/[A-Za-z0-9._-]+)(?:#([^)\s"'<>]+))?`)
	files := ecTracked(t)
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		text := files[rel]
		for _, m := range removed.FindAllStringSubmatch(text, -1) {
			if m[1] == "" && m[2] == "" {
				t.Errorf("jaekit:violation=contract-link-invalid: %s names %s, a removed second-language contract", rel, m[0])
			}
		}
		if strings.HasPrefix(rel, "contracts/") {
			for _, m := range sibling.FindAllString(text, -1) {
				t.Errorf("jaekit:violation=contract-link-invalid: %s links %s, a removed second-language contract", rel, m[2:])
			}
		}
		for _, m := range onMain.FindAllStringSubmatch(text, -1) {
			target, ok := files[m[1]]
			switch {
			case !ok:
				t.Errorf("jaekit:violation=contract-link-invalid: %s links %s, which is not a tracked file", rel, m[0])
			case m[2] != "" && !dlAnchorsOutsideFences(target)[m[2]]:
				t.Errorf("jaekit:violation=contract-link-invalid: %s links %s, but %s has no heading with that anchor", rel, m[0], m[1])
			}
		}
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	cmd := exec.Command(py, "-I", "tools/check_public_docs.py")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		t.Errorf("jaekit:violation=contract-link-invalid: tools/check_public_docs.py finds broken relative links or anchors:\n%s", out)
	case err != nil:
		t.Fatalf("tools/check_public_docs.py did not finish: %v\n%s", err, out)
	}
}

// ecAuthoring lists the authoring check's criteria that read contract files. On the start commit none of
// them reported a problem naming a contract file; problems from other files (AC-6, AC-8) are not this
// goal's. AC-9 is judged without the Go suites it also starts, which run in this condition's command, and
// without tools/check_public_docs.py, which AC-4's test runs on the same tree.
var ecAuthoring = []string{"AC-6", "AC-8", "AC-9", "AC-10"}

const ecAuthoringScript = `import json, pathlib, sys
sys.path.insert(0, "tools/contract-checks")
import authoring_check
authoring_check.regressions = lambda root: []
out = {}
for c in sys.argv[1:]:
    _, problems = authoring_check.observe(pathlib.Path(".").resolve(), "authoring", c)
    out[c] = [p for p in problems if "contracts/" in p]
print(json.dumps(out))`

// The Go suites that read contract text run in the same command; this adds the Python readers no Go test
// judges on the checkout: the authoring check's contract problems and the regression registry's shape test.
func TestEnglishContractsAC5_PythonReaders(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	cmd := exec.Command(py, append([]string{"-I", "-c", ecAuthoringScript}, ecAuthoring...)...)
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("authoring check: %v", err)
	}
	var got map[string][]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("authoring check output: %v\n%s", err, out)
	}
	for _, c := range ecAuthoring {
		if p, ok := got[c]; !ok || len(p) > 0 {
			t.Errorf("authoring check %s reports contract problems %q; the start commit had none", c, p)
		}
	}
	// Before Python 3.12, unittest discover exits 0 when it finds no test, so a missing reader cannot pass.
	if _, err := os.Stat("../tools/workflow-checks/test_regressions.py"); err != nil {
		t.Fatalf("regressions reader: %v", err)
	}
	cmd = exec.Command(py, "-I", "-m", "unittest", "discover", "-s", "tools/workflow-checks", "-p", "test_regressions.py")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("tools/workflow-checks/test_regressions.py: %v\n%s", err, out)
	}
}

const ecSiteScript = `import json, pathlib, sys
sys.path.insert(0, "tools/site")
import build
docs = build.discover(build.Tree(pathlib.Path(".").resolve()))
listed = {doc.dir for (lang, _), rows in build.groups_for(docs).items() if lang == "ko" for doc, _ in rows}
print(json.dumps({"docs": [[d.path, d.lang, d.dir] for d in docs if d.section == "contracts"], "ko_list": sorted(listed)}))`

// The site writes each document at doc.dir with <html lang> from doc.lang, and its Korean document list
// links the rows groups_for gives. The check reads those from the site builder without downloading or
// rendering anything.
func TestEnglishContractsAC7_SiteEnglishOnly(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	cmd := exec.Command(py, "-I", "-c", ecSiteScript)
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("site builder: %v", err)
	}
	var site struct {
		Docs   [][3]string `json:"docs"`
		KoList []string    `json:"ko_list"`
	}
	if err := json.Unmarshal(out, &site); err != nil {
		t.Fatalf("site builder output: %v\n%s", err, out)
	}
	listed := map[string]bool{}
	for _, dir := range site.KoList {
		listed[dir] = true
	}
	pages := map[string][][3]string{}
	for _, d := range site.Docs {
		pages[d[0]] = append(pages[d[0]], d)
		if strings.HasPrefix(d[2], "ko/") {
			t.Errorf("jaekit:violation=korean-contract-page: %s is built as the Korean page %s", d[0], d[2])
		}
	}
	for _, c := range ecContracts {
		rel, dir := "contracts/"+c+".md", "en/contracts/"+c+"/"
		if c == "README" {
			dir = "en/contracts/"
		}
		got := pages[rel]
		switch {
		case len(got) != 1:
			t.Errorf("jaekit:violation=korean-contract-page: %s has %d pages, want one", rel, len(got))
		case got[0][1] != "en" || got[0][2] != dir:
			t.Errorf("jaekit:violation=korean-contract-page: %s is the %s page %s, want the English page %s", rel, got[0][1], got[0][2], dir)
		case !listed[dir]:
			t.Errorf("jaekit:violation=korean-contract-page: the Korean document list does not link %s", dir)
		}
	}
}
