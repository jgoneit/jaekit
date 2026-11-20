package plugins

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

var (
	ecPlaceholder = regexp.MustCompile(`<[^<>]*>`)
	ecNumbered    = regexp.MustCompile(`^#{1,6} (\d+(?:\.\d+)*)\.? `)
	ecTomlComment = regexp.MustCompile(`\s+#.*$`)
)

// ecShape is what a contract keeps from its Korean original: numbered headings in order, every occurrence
// of a code span that carries Korean outside <placeholders> (the format tokens), and a digest of its
// examples (code blocks, with comments in TOML examples left out).
func ecShape(text string) (heads string, tokens []string, examples string) {
	prose, spans, blocks := ecParts(text)
	var nums []string
	for _, line := range prose {
		if m := ecNumbered.FindStringSubmatch(line); m != nil {
			nums = append(nums, m[1])
		}
	}
	for _, s := range spans {
		if ecHangul(ecPlaceholder.ReplaceAllString(s, "")) {
			tokens = append(tokens, s)
		}
	}
	sort.Strings(tokens)
	toml := false
	var norm []string
	for _, line := range blocks {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			toml = !toml && strings.HasSuffix(strings.TrimSpace(line), "toml")
		} else if toml {
			line = ecTomlComment.ReplaceAllString(line, "")
		}
		norm = append(norm, line)
	}
	return strings.Join(nums, " "), tokens, fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(norm, "\n"))))[:16]
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
		if strings.Contains(text, "This is a translation of") || strings.Contains(text, "original decides") {
			t.Errorf("jaekit:violation=two-language-contract: %s keeps the translation header", rel)
		}
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

const (
	ecContractRule = "Every document directly under `contracts/` is one English file, `X.md`."
	ecGuideRule    = "Every document directly under `guides/` has a Korean and an English version, `X.md` and `X.en.md`; change both in the same change."
)

func TestEnglishContractsAC2_AgentsRule(t *testing.T) {
	text := ecRead(t, "AGENTS.md")
	for _, old := range []string{"original decides", "regressions.ko.md", "directly under `guides/` and `contracts/`"} {
		if strings.Contains(text, old) {
			t.Errorf("jaekit:violation=two-language-contract-rule: AGENTS.md still says %q", old)
		}
	}
	for _, want := range []string{ecContractRule, ecGuideRule} {
		if n := strings.Count(text, want); n != 1 {
			t.Errorf("jaekit:violation=two-language-contract-rule: AGENTS.md says %q %d times, want once", want, n)
		}
	}
}

func TestEnglishContractsAC3_SameShapeAsOriginal(t *testing.T) {
	for _, c := range ecContracts {
		want, ok := ecOriginal[c]
		if !ok {
			t.Fatalf("no original shape for %s", c)
		}
		rel := "contracts/" + c + ".md"
		heads, tokens, examples := ecShape(ecRead(t, rel))
		if heads != want.heads {
			t.Errorf("%s numbered headings %q, want the original's %q", rel, heads, want.heads)
		}
		count := map[string]int{}
		for _, tok := range tokens {
			count[tok]++
		}
		for _, tok := range want.tokens {
			count[tok]--
		}
		for _, tok := range want.tokens {
			if count[tok] < 0 {
				t.Errorf("%s has %d fewer occurrences of the original's format token `%s`", rel, -count[tok], tok)
				count[tok] = 0
			}
		}
		if examples != want.examples {
			t.Errorf("%s examples digest %s, want the original's %s (code blocks differ)", rel, examples, want.examples)
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
// goal's. AC-9 is judged without the Go suites it also starts; they run in this condition's command.
var ecAuthoring = []string{"AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-8", "AC-9", "AC-10"}

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
// judges on the checkout: the authoring check's contract problems and the regression map's guidance test.
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
	cmd = exec.Command(py, "-I", "-m", "unittest", "discover", "-s", "tools/workflow-checks", "-p", "test_regressions.py")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("tools/workflow-checks/test_regressions.py: %v\n%s", err, out)
	}
}

// ecTagLinks are the release-pinned contract links at the start commit, by file.
var ecTagLinks = []string{
	"plugins/seal/skills/seal/SKILL.md https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4",
	"plugins/seal/skills/seal/references/bundle.md https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4",
	"plugins/seal/skills/seal/references/ha.md https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/check-result.md",
	"plugins/seal/skills/seal/references/ha.md https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/findings.md",
	"plugins/seal/skills/seal/references/ha.md https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4",
	"plugins/seal/skills/seal/references/ha.md https://github.com/jgoneit/jaekit/blob/v0.1.8/contracts/run-record.md#47-run-rules4",
	"tools/release/notes/v0.1.6.md https://github.com/jgoneit/jaekit/blob/v0.1.6/contracts/run-record.md#47-run-rules4",
}

func TestEnglishContractsAC6_ReleaseLinksKept(t *testing.T) {
	pinned := regexp.MustCompile(`https://github\.com/jgoneit/jaekit/` + ecRelease + `contracts/[A-Za-z0-9._-]+(?:#[^)\s"'<>]+)?`)
	count := map[string]int{}
	for rel, text := range ecTracked(t) {
		for _, m := range pinned.FindAllString(text, -1) {
			count[rel+" "+m]++
		}
	}
	for _, link := range ecTagLinks {
		if count[link]--; count[link] < 0 {
			t.Errorf("release-pinned contract link changed or removed: %s", link)
		}
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

// ecOriginal is the shape of each contract's Korean original at the start commit (regressions: regressions.ko.md),
// computed with ecShape.
var ecOriginal = map[string]struct {
	heads, examples string
	tokens          []string
}{
	"README":               {heads: "", examples: "e3b0c44298fc1c14", tokens: []string{}},
	"bundle":               {heads: "1 2 2.1 2.1.1 2.1.2 2.1.3 2.2 2.3 2.4 2.5 2.6 3 4 5 5.1 5.2 5.4 6 7 8", examples: "4e12e5cae65342b3", tokens: []string{"## Task 상태", "## 개선 메모", "## 검수 요약", "## 검수 요약", "## 검수 요약", "## 검수 요약", "## 검수 요약", "## 검수 요약", "## 결과 계약", "## 계획 변경", "## 독립 검토", "## 막힘", "## 범위", "## 범위", "## 범위", "## 시간 제한", "## 시간 제한", "## 시간 제한", "## 실행 환경", "## 실행 환경", "## 실행 환경", "## 예산", "## 예산", "## 완료 뒤 발견", "## 완료 보고", "## 완료 조건", "## 완료 조건", "## 읽을 자료", "## 조건표", "## 조건표", "## 중요한 미결정", "## 타임라인", "## 타임라인", "## 타임라인", "## 타임라인", "## 현재", "## 현재", "## 현재", "## 현재", "## 현재", "## 현재", "## 현재", "## 현재", "### 근거", "### 근거", "### 근거", "### 누가 정했나", "### 누가 정했나", "### 바뀐 동작과 파일", "### 바뀐 동작과 파일", "### 범위 밖이나 기존 것을 바꾼 곳", "### 범위 밖이나 기존 것을 바꾼 곳", "### 보지 않은 것", "### 보지 않은 것", "### 보지 않은 것", "### 사용자의 답", "### 사용자의 답", "### 실행자 가정", "### 실행자 가정", "### 실행자 가정", "### 자세히", "### 자세히", "### 직접 볼 것", "### 직접 볼 것", "### 직접 볼 것", "### 직접 볼 것", "(선택)", "- AC-3: 30분", "- EX-4: 1시간 30분", "- T002 명령 1: 20분", "- 다른 목표 실행 묶음: `검사 입력` (이전 목표의 PROGRESS.md를 검사한다)", "- 브라우저: `node scripts/probe-browser.mjs`", "1시간 30분", "4시간", "90분", "ID | 선언 경로", "T002 명령 1", "max(50, ⌈필수 최소량 × 1.5⌉)", "가설", "검사 경로", "검사 입력", "검사 입력", "검증", "검증 명령", "검증 실행", "검증 실행", "검증 실행", "검증 실행", "검증 실행: 자동", "검증 실행: 자동", "결정", "경과 시간", "경과 시간", "고침", "관측", "근거", "기각", "내용", "다른 목표 실행 묶음", "다른 목표 실행 묶음", "다른 목표 실행 묶음:", "다른 목표 실행 묶음:", "다시 검토한 것", "대기", "라운드 | 지적 | 끝", "막힘", "메모", "목표", "문서", "문서", "미관측", "바꿀 수 있는 경로", "바꿀 수 있는 경로", "바꿀 수 있는 경로", "바꿀 수 있는 경로", "바뀐 동작 | 조건 | 파일", "반박", "반영", "변경", "분", "비용", "사용 가능", "사용 불가", "사용자의 답", "상태", "선행", "시각", "시각", "시각", "시각", "시간", "실패", "아니오", "아니오", "없음", "없음", "없음", "영향", "예", "예", "예정", "요구", "요구", "요구", "요구", "요구", "원인", "원인 [확정]: …", "원인 종류", "유지할 동작", "이슈로 보류", "이유", "자동", "자동 보류", "재개", "재개", "제안", "제안", "조치", "종류", "중복", "직접 볼 것", "처리 없음", "초", "테스트 경로", "통과", "필수", "필수", "해소 조건", "확정"}},
	"check-result":         {heads: "1 2 3 4 5", examples: "852a8d28f4c584b2", tokens: []string{"## 결과 계약", "expect(값, \"jaekit:violation=<선언 id>\")", "검사 경로"}},
	"core-capabilities":    {heads: "1 2 3 4 6 7 8 9 10 11", examples: "8f73eaefe7a4c50b", tokens: []string{"## 시간 제한", "## 실행 환경", "검증 실행: 자동", "검증 실행: 자동"}},
	"findings":             {heads: "", examples: "39b81150df9eac1e", tokens: []string{}},
	"goal-docs":            {heads: "1 2 2.1 2.2 2.3 2.4 2.5 2.6 3 4 5 6 6.1 6.2 6.3 6.4 6.5 6.6 6.7", examples: "7ba2231106eee1e2", tokens: []string{"## 작업 분해 제안", "(선택)", "[기존]", "[사용자]", "[제안]", "결과", "경로", "유지", "유지", "의존", "조건"}},
	"regressions":          {heads: "", examples: "e3b0c44298fc1c14", tokens: []string{}},
	"repository-producers": {heads: "", examples: "e3b0c44298fc1c14", tokens: []string{"self.fail(\"[violation:식별자] 설명\")"}},
	"review-gate":          {heads: "1 2 3", examples: "62c3e1cdb99c560d", tokens: []string{}},
	"review-round":         {heads: "1 2 3 4 5 6 7 8 9 10 11", examples: "ccdf2a8eefccb7d3", tokens: []string{"spar triage <라운드> <지적> <처리> [옵션]", "이름=강도", "이름=모델"}},
	"role-card":            {heads: "", examples: "e3b0c44298fc1c14", tokens: []string{"## 개선 메모", "## 검수 요약", "## 시간 제한"}},
	"run-record":           {heads: "1 1.1 1.2 1.3 1.4 1.5 1.6 1.7 2 2.1 2.2 2.3 2.4 2.5 2.6 3 4 4.1 4.2 4.3 4.4 4.5 4.6 4.7 4.8 4.9 4.10 5 5.1 5.2 6", examples: "4883f9023259f2d1", tokens: []string{"# 비용 요약: <goal>", "## 범위", "## 범위", "## 범위", "## 시간 제한", "## 시간 제한", "## 시간 제한", "## 실행 환경", "## 실행 환경", "## 예산", "## 현재", "## 현재", "## 현재", "### Stamp for `## 현재`", "### `## 현재` 도장", "### 미충족 조건의 원문 위치", "### 완료 범위", "### 조건별 근거", "(review note seq N 때의 조건표)", "(선택)", "- 검증 실행: 자동", "- 원인: <원인>; 이유: <이유>", "- 원인: 기록 없음", "0이 아닌 종료", "0이 아닌 종료, 종료 코드 <코드>", "<이름>: 사용 가능", "<이름>: 사용 불가 (<이유>)", "change 조건 N개 중 M개가 baseline을 요구하고 그중 K개에 신선한 baseline", "ha note <goal> <block \\| unblock \\| confirm \\| reopen \\| input> [대상] --quote <인용> [--cause <원인>]", "max(50, ⌈최소량 × 1.5⌉)", "검사 입력", "검사 작성자: 실행자", "검증 실행", "검증 실행", "검증 실행", "검토 gate의 지적 목록이 기록되지 않음(review note seq <n>)", "경과 시간", "기록된 검토 지적 목록을 읽을 수 없음(review note seq <n>)", "남은 한계", "남은 한계", "남은 한계", "다른 목표 실행 묶음", "다른 목표 실행 묶음", "다른 목표 실행 묶음:", "다음 행동", "다음 행동", "동결 뒤 까닭 없이 바뀐 검사 파일: <path>", "동결 뒤 바뀐 검사 파일(까닭 있음): <path> (seq n: <까닭>)", "동결 때의 조건표가 기록되지 않음", "바꿀 수 있는 경로", "바꿀 수 있는 경로", "바뀐 것", "바뀐 것", "보류한 검토 지적: <series>/<round> <id> [<class>] → <to>", "보증 수준: local", "사용 가능", "사용 가능", "사용 가능", "사용 가능", "사용 불가", "사용 불가", "사용 불가", "사용 불가", "사용 불가", "사용량 > 상한", "시간 초과", "시간 초과, 60초", "시작 뒤 baseline 표시를 뺀 조건: …", "신호로 종료", "신호로 종료, 신호 <번호>", "실행 권한 없음", "실행 파일 없음", "없음", "없음", "요구", "자동", "자동", "자동 보류한 hardening 지적: <n>개", "직접 볼 것", "직접 볼 것", "직접 볼 것", "직접 볼 것", "직접 볼 것", "처리 없는 검토 지적: <series>/<round> <id> [<class>]", "코드<탭>파일:줄<탭>설명", "테스트 경로", "표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain", "표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain", "필수 change × 2 + 필수 maintain", "확인한 것", "확인한 것", "확인한 것"}},
	"spar-config":          {heads: "1 2 3 4", examples: "b1b5f13b9c639754", tokens: []string{"--effort 이름=강도", "--model 이름=모델", "spar config [--repo DIR] [--reviewers a,b] [--model 이름=모델]... [--effort 이름=강도]..."}},
	"spar-input":           {heads: "1 2 3 4 5 6", examples: "323e6190be042645", tokens: []string{"--effort 이름=강도", "--model 이름=모델", ":줄", ":줄대시줄"}},
}
