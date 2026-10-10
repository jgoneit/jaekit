package plugins

// Public documentation checks run without repository history or private records.
import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

type visualLang struct {
	readme, install, usage, flow, example, start, more, source, diagram string
	intro, roles, stages, result                                        []string
}

var visualLanguages = []visualLang{
	{readme: "../README.md", install: "../guides/INSTALL.md", usage: "../guides/USAGE.md",
		flow: "## 사용 흐름", example: "## 가상 사용 예시", start: "## 시작하기", more: "## 자세히 알아보기",
		source: "assets/readme/SOURCES.md", diagram: "assets/readme/flow.ko.svg",
		intro: []string{"Claude Code", "Codex", "목표", "구현", "검사", "이어"}, roles: []string{"목표", "구현"},
		stages: []string{"요청", "Spec", "멈", "확인", "시작", "Seal", "구현", "검사", "수정", "완료", "결과"},
		result: []string{"요청", "목표", "변경", "확인"}},
	{readme: "../README.en.md", install: "../guides/INSTALL.en.md", usage: "../guides/USAGE.en.md",
		flow: "## How it works", example: "## Fictional example", start: "## Get started", more: "## More",
		source: "assets/readme/SOURCES.en.md", diagram: "assets/readme/flow.en.svg",
		intro: []string{"Claude Code", "Codex", "goal", "implement", "check", "continue"}, roles: []string{"goal", "implement"},
		stages: []string{"request", "Spec", "stop", "review", "start", "Seal", "implement", "check", "fix", "report", "result"},
		result: []string{"request", "goal", "before", "after", "check"}},
}

func visualRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func visualSection(t *testing.T, path, heading string) string {
	t.Helper()
	text := visualRead(t, path)
	start := strings.Index(text, heading+"\n")
	if start < 0 {
		t.Fatalf("%s lacks %s", path, heading)
	}
	level := strings.IndexByte(heading, ' ')
	for _, m := range regexp.MustCompile(`(?m)^#{1,6} `).FindAllStringIndex(text[start+len(heading)+1:], -1) {
		if m[1]-m[0]-1 <= level {
			return text[start : start+len(heading)+1+m[0]]
		}
	}
	return text[start:]
}

func visualRequire(t *testing.T, name, text string, terms ...string) {
	t.Helper()
	for _, term := range terms {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(term)) {
			t.Errorf("%s lacks %q", name, term)
		}
	}
}

func visualPlain(text string) string {
	return html.UnescapeString(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(text, " "))
}

func visualBeforeInstall(t *testing.T, l visualLang) string {
	t.Helper()
	text := visualRead(t, l.readme)
	i := strings.Index(text, "brew install jgoneit/tap/jaekit")
	if i < 0 {
		t.Fatalf("%s lacks the basic installation command", l.readme)
	}
	return text[:i]
}

// Only follow actual links to the maintained user guides, never assume that a
// guide is reachable merely because its filename exists in the checkout.
func visualGuidance(t *testing.T, l visualLang) string {
	t.Helper()
	text := visualRead(t, l.readme)
	for _, path := range []string{l.install, l.usage} {
		if strings.Contains(text, strings.TrimPrefix(path, "../")) {
			text += "\n" + visualRead(t, path)
		}
	}
	return text
}

func TestReadmeVisualGuideAC1(t *testing.T) {
	for _, l := range visualLanguages {
		intro, _, _ := strings.Cut(visualRead(t, l.readme), "\n## ")
		visualRequire(t, l.readme+" intro", intro, l.intro...)
		for i, name := range []string{"Spec", "Seal"} {
			for _, line := range strings.Split(intro, "\n") {
				if strings.Contains(line, name) {
					visualRequire(t, l.readme+" first "+name, line, l.roles[i])
					break
				}
			}
		}
		for _, internal := range []string{"SPEC.md", "PLAN.md", "AC-1", "seq "} {
			if strings.Contains(intro, internal) {
				t.Errorf("%s introduction needs internal term %q", l.readme, internal)
			}
		}
	}
}

func TestReadmeVisualGuideAC2(t *testing.T) {
	for _, l := range visualLanguages {
		text := visualRead(t, l.readme)
		at := 0
		for _, heading := range []string{l.flow, l.example, l.start, l.more} {
			i := strings.Index(text[at:], heading+"\n")
			if i < 0 {
				t.Fatalf("%s lacks %s in introduction/flow/example/start/details order", l.readme, heading)
			}
			at += i + len(heading)
		}
		intro, _, _ := strings.Cut(text, "\n## ")
		anchor := "#설치"
		if strings.Contains(l.readme, ".en.md") {
			anchor = "#install"
		}
		visualRequire(t, l.readme+" shortcut", intro, anchor)
		if len(regexp.MustCompile(`(?m)^\| .*\|`).FindAllString(text, -1)) > 5 {
			t.Errorf("%s repeats the illustrated flow in a long table", l.readme)
		}
	}
}

func TestReadmeVisualGuideAC3(t *testing.T) {
	for _, l := range visualLanguages {
		flow := visualSection(t, l.readme, l.flow)
		visualRequire(t, l.readme+" flow image", flow, l.diagram)
		diagram := visualPlain(visualRead(t, "../"+l.diagram))
		visualRequire(t, l.diagram, diagram, l.stages...)
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, "English flow fallback", flow, "Spec", "stop", "separate", "Seal", "decision", "continue")
		} else {
			visualRequire(t, "Korean flow fallback", flow, "Spec", "멈", "별도", "Seal", "결정", "이어")
		}
	}
}

func TestReadmeVisualGuideAC4(t *testing.T) {
	for _, l := range visualLanguages {
		lang := "ko"
		if strings.Contains(l.readme, ".en.md") {
			lang = "en"
		}
		asset := "assets/readme/example." + lang + ".svg"
		example := visualSection(t, l.readme, l.example)
		visualRequire(t, l.readme+" illustrated example", example, asset, l.source, "Spec", "Seal")
		if !regexp.MustCompile(`!\[[^\]]+\]\(` + regexp.QuoteMeta(asset) + `\)`).MatchString(example) {
			t.Errorf("%s does not display the illustration with alt text", l.readme)
		}
		illustration := visualRead(t, "../"+asset)
		visualRequire(t, asset, illustration, "<svg", "<title", "<desc")
		if lang == "ko" {
			visualRequire(t, l.readme+" example", example, "요청", "목표", "비밀번호", "검사", "확인", "가상", "실행 결과나 검증을 마친 사례가 아닙니다")
		} else {
			visualRequire(t, l.readme+" example", example, "request", "goal", "password", "check", "fictional", "not an executed or verified case")
		}
	}
}

func TestReadmeVisualGuideAC5(t *testing.T) {
	for _, l := range visualLanguages {
		source := visualRead(t, "../"+l.source)
		visualRequire(t, l.source, source, "generate-flow.py", "generate-example.py", "Python 3")
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.source, source, "conceptual", "fictional", "not an executed result")
		} else {
			visualRequire(t, l.source, source, "개념도", "가상", "실행 결과나 완료·검사 증명이 아닙니다")
		}
		for _, path := range []string{l.readme, "../" + l.source} {
			for _, private := range []string{"/Users/", "/home/", "ghp_", "github_pat_", "sk-proj-"} {
				if strings.Contains(visualRead(t, path), private) {
					t.Errorf("%s contains a private path or credential prefix", path)
				}
			}
		}
	}
}

func TestReadmeVisualGuideAC7(t *testing.T) {
	for _, l := range visualLanguages {
		text := visualRead(t, l.readme)
		if regexp.MustCompile(`(?m)^## (한계|Limits)`).MatchString(text) {
			t.Errorf("%s retains a long limitations section", l.readme)
		}
		before := visualBeforeInstall(t, l)
		visualRequire(t, l.readme+" prerequisites", before, "Windows", "macOS", "Linux", "Claude Code", "Codex")
		// Check the actual completion paragraph and its linked detailed section.
		// Words elsewhere in the document cannot stand in for this explanation.
		flow := visualSection(t, l.readme, l.flow)
		paragraph := ""
		for _, candidate := range strings.Split(flow, "\n\n") {
			if strings.HasPrefix(candidate, "완료는 ") || strings.HasPrefix(candidate, "Completion means ") {
				paragraph = candidate
				break
			}
		}
		usageHeading, usageAnchor := "## 완료 보고 읽기", "완료-보고-읽기"
		if strings.Contains(l.readme, ".en.md") {
			usageHeading, usageAnchor = "## Reading the completion report", "reading-the-completion-report"
		}
		visualRequire(t, l.readme+" completion link", paragraph, "("+strings.TrimPrefix(l.usage, "../")+"#"+usageAnchor+")")
		usage := visualSection(t, l.usage, usageHeading)
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.readme+" development status", text, "early version")
			visualRequire(t, l.readme+" completion paragraph", paragraph, "required conditions", "recorded checks", "necessary user confirmations", "agent writes the automated checks", "not a guarantee", "flawless", "independently verified")
			visualRequire(t, l.usage+" completion section", usage, "Required completion conditions need recorded checks or necessary user confirmations", "Optional conditions do not block completion", "agent writes the automated checks", "not independent third-party verification")
		} else {
			visualRequire(t, l.readme+" development status", text, "초기 개발")
			visualRequire(t, l.readme+" completion paragraph", paragraph, "필수 완료 조건", "검사 기록", "필요한 사용자 확인", "에이전트가 작성", "무결함", "제3자의 보증을 뜻하지")
			visualRequire(t, l.usage+" completion section", usage, "필수 완료 조건은 검사 기록 또는 필요한 사용자 확인으로 충족", "선택 조건은 완료를 막지 않", "자동 검사는 에이전트가 작성", "제3자의 검증을 뜻하지")
		}
		// Preserve the former onboarding regression guard: optional conditions
		// never become mandatory through a blanket "all criteria must pass".
		for _, path := range []string{l.readme, l.usage} {
			doc := visualRead(t, path)
			for _, forbidden := range []string{"모든 조건", "모든 완료 조건", "every criterion passed", "all of them pass", "all criteria pass", "all criteria must pass", "every criterion must pass", "all conditions pass", "all conditions must pass", "every condition must pass"} {
				if strings.Contains(strings.ToLower(doc), forbidden) {
					t.Errorf("%s requires optional conditions too: %q", path, forbidden)
				}
			}
			for _, line := range strings.Split(doc, "\n") {
				if strings.Contains(line, "완료를 선언") && !regexp.MustCompile(`필수(?: 완료)? 조건`).MatchString(line) {
					t.Errorf("%s declares completion without restricting it to required conditions: %s", path, line)
				}
				if strings.Contains(strings.ToLower(line), "declares completion") && !strings.Contains(strings.ToLower(line), "required") {
					t.Errorf("%s declares completion without required conditions: %s", path, line)
				}
			}
		}
	}
}

// Maintenance checks allow either the former README details or their linked
// guide destinations; the supported commands and outcomes do not change.
func TestReadmeVisualGuideAC8(t *testing.T) {
	for _, l := range visualLanguages {
		text := visualGuidance(t, l)
		visualRequire(t, l.readme+" install and calls", text,
			"brew install jgoneit/tap/jaekit", "ha --version", "ha 0.1.2", "which -a ha",
			"claude plugin marketplace add jgoneit/jaekit#v0.1.2", "claude plugin install spec@jaekit", "claude plugin install seal@jaekit",
			"codex plugin marketplace add jgoneit/jaekit@v0.1.2", "codex plugin add spec@jaekit", "codex plugin add seal@jaekit",
			"/spec ", "/spec:spec ", "$spec ", "@Spec", "/seal docs/specs/<goal>", "/seal:seal", "$seal docs/specs/<goal>",
			"brew upgrade jgoneit/tap/jaekit", "brew uninstall jgoneit/tap/jaekit", "marketplace remove jaekit")
		// A direct README -> INSTALL#codex entry bypasses preceding sections.
		// Its own preamble must link both prerequisites before plugin commands.
		if strings.Contains(visualRead(t, l.readme), strings.TrimPrefix(l.install, "../")+"#codex") {
			codex := visualSection(t, l.install, "### Codex")
			preamble, _, found := strings.Cut(codex, "codex plugin marketplace add")
			if !found {
				t.Fatalf("%s Codex entry has no installation command", l.install)
			}
			installHeading, versionHeading := "### ha 설치", "### 버전 확인"
			installAnchor, versionAnchor := "ha-설치", "버전-확인"
			if strings.Contains(l.readme, ".en.md") {
				installHeading, versionHeading = "### Install ha", "### Check the version"
				installAnchor, versionAnchor = "install-ha", "check-the-version"
			}
			visualRequire(t, l.install+" Codex prerequisites", preamble, "(#"+installAnchor+")", "(#"+versionAnchor+")")
			visualRequire(t, l.install+" ha prerequisite destination", visualSection(t, l.install, installHeading), "brew install jgoneit/tap/jaekit")
			visualRequire(t, l.install+" version prerequisite destination", visualSection(t, l.install, versionHeading), "ha --version", "which -a ha", "ha 0.1.2")
		}

		before := visualBeforeInstall(t, l)
		visualRequire(t, l.readme+" before install", before, "Homebrew", "git", "Windows", "Seal", "Spec")
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.readme+" Windows scope", before, "does not support Windows", "has not been checked")
		} else {
			visualRequire(t, l.readme+" Windows scope", before, "Windows", "Seal을 쓸 수 없습니다", "아직 확인하지")
		}
	}
}

func TestReadmeVisualGuideAC9(t *testing.T) {
	for _, l := range visualLanguages {
		readme := visualRead(t, l.readme)
		anchor := "설치"
		if strings.Contains(l.readme, ".en.md") {
			anchor = "install"
		}
		if !visualAnchors(readme)[anchor] {
			t.Errorf("%s loses #%s", l.readme, anchor)
		}
		visualRequire(t, l.readme+" destinations", readme, "guides/INSTALL", "guides/OPERATIONS.md", "contracts/run-record.md", "LICENSE", "https://github.com/jgoneit/jaekit/issues/new/choose")
		for _, path := range []string{l.readme, l.install, "../guides/OPERATIONS.md"} {
			visualCheckLinks(t, path)
		}
		for _, path := range []string{l.usage, "../" + l.source} {
			if strings.Contains(readme, strings.TrimPrefix(path, "../")) {
				visualCheckLinks(t, path)
			}
		}
		guide := visualGuidance(t, l)
		visualRequire(t, l.readme+" troubleshooting", guide, "ha: command not found", "which -a ha", "/spec:spec", "/seal:seal", "claude plugin disable <plugin>", "enabled = false", "@Spec", "$spec", "Cannot add marketplace")
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, "English recovery", guide, "permission", "budget", "continue docs/specs/<goal>", "decide")
		} else {
			visualRequire(t, "Korean recovery", guide, "권한", "예산", "docs/specs/<goal> 이어서 해줘", "정할 것")
		}
	}
}

func visualCheckLinks(t *testing.T, path string) {
	t.Helper()
	text := visualRead(t, path)
	var kept []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			kept = append(kept, line)
		}
	}
	for _, match := range regexp.MustCompile(`\]\(([^)\s]+)\)|(?:href|src)="([^"]+)"`).FindAllStringSubmatch(strings.Join(kept, "\n"), -1) {
		target := match[1]
		if target == "" {
			target = match[2]
		}
		if regexp.MustCompile(`^[a-z]+:`).MatchString(target) {
			continue
		}
		file, anchor, _ := strings.Cut(target, "#")
		dest := path
		if file != "" {
			dest = filepath.Join(filepath.Dir(path), file)
		}
		info, err := os.Stat(dest)
		if err != nil {
			t.Errorf("%s links missing %s", path, target)
			continue
		}
		if anchor != "" && !info.IsDir() && !visualAnchors(visualRead(t, dest))[anchor] {
			t.Errorf("%s links missing anchor %s", path, target)
		}
	}
}

func visualAnchors(text string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	for _, m := range regexp.MustCompile(`(?m)^#{1,6} (.+)$`).FindAllStringSubmatch(text, -1) {
		var b strings.Builder
		for _, r := range strings.ToLower(m[1]) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '-' || r == '_' {
				b.WriteRune(r)
			} else if r == ' ' {
				b.WriteByte('-')
			}
		}
		slug := b.String()
		if seen[slug] > 0 {
			out[slug+"-"+strconv.Itoa(seen[slug])] = true
		} else {
			out[slug] = true
		}
		seen[slug]++
	}
	return out
}

func TestReadmeVisualGuideAC10(t *testing.T) {
	for _, l := range visualLanguages {
		intro, _, _ := strings.Cut(visualRead(t, l.readme), "\n## ")
		other := "README.en.md"
		if strings.Contains(l.readme, ".en.md") {
			other = "README.md"
		}
		visualRequire(t, l.readme+" language switch", intro, other)
		visualRequire(t, l.readme+" translated flow", visualSection(t, l.readme, l.flow), l.diagram)
		visualRequire(t, l.readme+" localized guidance", visualRead(t, l.readme), strings.TrimPrefix(l.usage, "../"), strings.TrimPrefix(l.install, "../"))
		visualRead(t, l.usage)
	}
	ko, en := visualLanguages[0], visualLanguages[1]
	TestReadmeVisualGuideAC4(t)
	// Install commands stay identical; translation changes prose, not argv.
	commands := func(text string) string {
		var out []string
		inBash := false
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "```bash" {
				inBash = true
				continue
			}
			if line == "```" {
				inBash = false
				continue
			}
			if !inBash {
				continue
			}
			line = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(line, "<>")
			if strings.HasPrefix(line, "brew ") || strings.HasPrefix(line, "claude plugin ") || strings.HasPrefix(line, "codex plugin ") || line == "ha --version" || line == "which -a ha" {
				out = append(out, line)
			}
		}
		return strings.Join(out, "\n")
	}
	for _, pair := range [][2]string{{ko.readme, en.readme}, {ko.install, en.install}} {
		if commands(visualRead(t, pair[0])) != commands(visualRead(t, pair[1])) {
			t.Errorf("commands differ between %s and %s", filepath.Base(pair[0]), filepath.Base(pair[1]))
		}
	}
}
