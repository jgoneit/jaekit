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
	readme, install, usage, flow, start, firstTask, before, more string
	source, hero, diagram, vector                                string
	intro                                                        []string
}

var visualLanguages = []visualLang{
	{readme: "../README.md", install: "../guides/INSTALL.md", usage: "../guides/USAGE.md",
		flow: "## 사용 흐름", start: "## 시작하기", firstTask: "### 첫 작업 맡기기", before: "## 쓰기 전에 알아두면 좋은 점", more: "## 더 알아보기",
		source: "assets/readme/SOURCES.md", hero: "assets/readme/hero.ko.png", diagram: "assets/readme/usage-flow.ko.png", vector: "assets/readme/usage-flow.ko.svg",
		intro: []string{"Claude Code", "Codex", "작업", "목표", "Spec", "Seal"}},
	{readme: "../README.en.md", install: "../guides/INSTALL.en.md", usage: "../guides/USAGE.en.md",
		flow: "## How it works", start: "## Get started", firstTask: "### Your first task", before: "## Before you start", more: "## More",
		source: "assets/readme/SOURCES.en.md", hero: "assets/readme/hero.en.png", diagram: "assets/readme/usage-flow.en.png", vector: "assets/readme/usage-flow.en.svg",
		intro: []string{"Claude Code", "Codex", "work", "goal", "Spec", "Seal"}},
}

const visualBrand = "assets/readme/brand/logo.svg"

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

// Prose checks ignore presentation markup and image descriptions: a diagram's
// alt text must not be the only explanation of the user's next action.
func visualProse(text string) string {
	text = regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`).ReplaceAllString(text, "")
	return strings.NewReplacer("**", "", "`", "").Replace(visualPlain(text))
}

func visualRequirePattern(t *testing.T, name, text, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(text) {
		t.Errorf("%s lacks meaning matching %q", name, pattern)
	}
}

func visualImage(t *testing.T, name, text, asset string) {
	t.Helper()
	markdown := regexp.MustCompile(`!\[[^\]]+\]\(` + regexp.QuoteMeta(asset) + `\)`).MatchString(text)
	htmlAlt := strings.TrimSpace(visualAttribute(visualHTMLImage(text, asset), "alt"))
	if !markdown && htmlAlt == "" {
		t.Errorf("%s does not display %s with alt text", name, asset)
	}
	if _, err := os.Stat("../" + asset); err != nil {
		t.Errorf("%s image is unavailable: %v", asset, err)
	}
}

func visualAttribute(tag, name string) string {
	match := regexp.MustCompile(`(?i)(?:^|\s)` + regexp.QuoteMeta(name) + `\s*=\s*(?:"([^"]*)"|'([^']*)')`).FindStringSubmatch(tag)
	if match == nil {
		return ""
	}
	return html.UnescapeString(match[1] + match[2])
}

func visualHTMLImage(text, asset string) string {
	for _, tag := range regexp.MustCompile(`(?is)<img\b[^>]*>`).FindAllString(text, -1) {
		if visualAttribute(tag, "src") == asset {
			return tag
		}
	}
	return ""
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
		visualRequire(t, l.readme+" intro", visualProse(intro), l.intro...)
		header := regexp.MustCompile(`(?is)^\s*<h1\b[^>]*>(.*?)</h1>`).FindStringSubmatch(intro)
		if header == nil {
			t.Errorf("%s lacks a brand heading before the introduction and flow", l.readme)
		} else {
			visualImage(t, l.readme+" brand heading", header[1], visualBrand)
			width, err := strconv.Atoi(visualAttribute(visualHTMLImage(header[1], visualBrand), "width"))
			if err != nil || width <= 0 || width > 320 {
				t.Errorf("%s brand heading needs a display width between 1 and 320 pixels", l.readme)
			}
			if strings.Contains(header[1], l.hero) {
				t.Errorf("%s puts the large hero in the brand heading", l.readme)
			}
		}
		collapsedHero := false
		for _, detail := range regexp.MustCompile(`(?is)<details\b([^>]*)>(.*?)</details>`).FindAllStringSubmatch(intro, -1) {
			if !strings.Contains(detail[2], l.hero) {
				continue
			}
			visualImage(t, l.readme+" optional hero", detail[2], l.hero)
			if regexp.MustCompile(`(?i)(?:^|\s)open(?:\s|=|$)`).MatchString(detail[1]) {
				t.Errorf("%s hero details must be closed by default", l.readme)
			} else {
				collapsedHero = true
			}
		}
		if !collapsedHero {
			t.Errorf("%s must keep the hero in closed details before the usage flow", l.readme)
		}
		visibleIntro := regexp.MustCompile(`(?is)<details\b[^>]*>.*?</details>`).ReplaceAllString(intro, "")
		if strings.Contains(visibleIntro, l.hero) {
			t.Errorf("%s displays the large hero outside its optional details", l.readme)
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
		for _, heading := range []string{l.flow, l.start, l.before, l.more} {
			i := strings.Index(text[at:], heading+"\n")
			if i < 0 {
				t.Fatalf("%s lacks %s in flow/start/guidance/details order", l.readme, heading)
			}
			at += i + len(heading)
		}
		intro, _, _ := strings.Cut(text, "\n## ")
		anchor := "#설치"
		if strings.Contains(l.readme, ".en.md") {
			anchor = "#install"
		}
		visualRequire(t, l.readme+" shortcut", intro, anchor)
	}
}

func TestReadmeVisualGuideAC3(t *testing.T) {
	for _, l := range visualLanguages {
		flow := visualSection(t, l.readme, l.flow)
		visualImage(t, l.readme+" flow", flow, l.diagram)
		vector := visualRead(t, "../"+l.vector)
		visualRequire(t, l.vector+" accessible original", vector, "<svg", "<title", "<desc")
		flow = visualProse(flow)
		readme := visualProse(visualRead(t, l.readme))
		intro, _, _ := strings.Cut(readme, "\n## ")
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, "English flow fallback", flow, "Spec", "goal", "stop", "separate", "Seal", "agent", "check", "result")
			visualRequire(t, "English implementation role", intro+flow, "implement")
			visualRequirePattern(t, "English resume project", readme, `(?i)same project`)
			visualRequirePattern(t, "English resume goal", readme, `(?i)(same|previous|earlier) goal|goal you were working on`)
			visualRequirePattern(t, "English resume action", readme, `(?i)(resume|continue).*(goal|work)|(?:goal|work).*(resume|continue)`)
		} else {
			visualRequire(t, "Korean flow fallback", flow, "Spec", "목표", "멈", "별도", "Seal", "에이전트", "검사", "결과")
			visualRequire(t, "Korean implementation role", intro+flow, "구현")
			visualRequirePattern(t, "Korean resume project", readme, `같은 프로젝트`)
			visualRequirePattern(t, "Korean resume goal", readme, `(같은|이전|앞서).{0,30}목표`)
			visualRequirePattern(t, "Korean resume action", readme, `(이어|재개)`)
		}
	}
}

func TestReadmeVisualGuideAC4(t *testing.T) {
	for _, l := range visualLanguages {
		example := visualSection(t, l.readme, l.firstTask)
		visualRequire(t, l.readme+" first task", example, "$spec:spec ", "$seal:seal docs/specs/<goal>", "docs/specs/password-link")
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.readme+" fictional request", example, "password", "fictional", "goal", "separate", "conversation")
			visualRequirePattern(t, "English example disclaimer", example, `(?i)not (an? )?(executed|verified|real) (or verified )?(case|result)`)
			visualRequirePattern(t, "English placeholder", example, `(?i)replace.*<goal>|<goal>.*replace`)
		} else {
			visualRequire(t, l.readme+" fictional request", example, "비밀번호", "가상", "목표", "별도", "대화")
			visualRequirePattern(t, "Korean example disclaimer", example, `(실행|검증).*(사례|결과).*(아닙|아닌)`)
			visualRequirePattern(t, "Korean placeholder", example, `<goal>.*(바꿉|바꾸|바꿀)|(?:바꿉|바꾸|바꿀).*<goal>`)
		}
	}
}

func TestReadmeVisualGuideAC5(t *testing.T) {
	for _, l := range visualLanguages {
		source := visualRead(t, "../"+l.source)
		visualRequire(t, l.source+" maintained originals", source, filepath.Base(l.hero), filepath.Base(l.diagram), filepath.Base(l.vector), "source/")
		visualRequire(t, l.source+" brand sources", source, "brand/logo.png", "brand/wordmark.png", "brand/symbol.png", "brand/logo.svg", "source/build_brand.py")
		visualCheckLinks(t, "../"+l.source)
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.source, source, "conceptual", "fictional")
			visualRequirePattern(t, "English image provenance", source, `(?i)not.{0,80}(executed|execution|result|proof)`)
		} else {
			visualRequire(t, l.source, source, "개념도", "가상")
			visualRequirePattern(t, "Korean image provenance", source, `(실행 결과|검사 증명).{0,30}아닙`)
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
		// Completion guidance may live in prose or a FAQ, but the README itself
		// must still state its limits rather than relying only on a guide link.
		prose := visualProse(text)
		usageHeading, usageAnchor := "## 완료 보고 읽기", "완료-보고-읽기"
		if strings.Contains(l.readme, ".en.md") {
			usageHeading, usageAnchor = "## Reading the completion report", "reading-the-completion-report"
		}
		visualRequire(t, l.readme+" completion link", text, "("+strings.TrimPrefix(l.usage, "../")+"#"+usageAnchor+")")
		usage := visualSection(t, l.usage, usageHeading)
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.readme+" development status", text, "early version")
			visualRequirePattern(t, "English required completion", prose, `(?i)required (completion )?conditions.{0,120}(recorded checks|check records)`)
			visualRequirePattern(t, "English manual confirmation", prose, `(?i)(necessary|required|needed) user confirmations?`)
			visualRequirePattern(t, "English check author", prose, `(?i)agent writes (the )?(automated )?checks|checks (are )?written by the agent`)
			visualRequirePattern(t, "English independent assurance", prose, `(?i)not.{0,80}(independent|third.party)|(?:independent|third.party).{0,80}(not|doesn't)`)
			visualRequirePattern(t, "English result guarantee", prose, `(?i)(not|no).{0,100}guarantee.{0,100}(flawless|defect.free)`)
			visualRequire(t, l.usage+" completion section", usage, "Required completion conditions need recorded checks or necessary user confirmations", "Optional conditions do not block completion", "agent writes the automated checks", "not independent third-party verification")
		} else {
			visualRequire(t, l.readme+" development status", text, "초기 개발")
			visualRequirePattern(t, "Korean required completion", prose, `필수( 완료)? 조건.{0,100}검사 기록`)
			visualRequirePattern(t, "Korean manual confirmation", prose, `필요한 사용자 확인`)
			visualRequirePattern(t, "Korean check author", prose, `검사.{0,40}에이전트가 작성|에이전트가.{0,40}검사.{0,40}작성`)
			visualRequirePattern(t, "Korean independent assurance", prose, `(독립|제3자).{0,100}(아닙|뜻하지|보장하지|보증하지)`)
			visualRequirePattern(t, "Korean result guarantee", prose, `무결함.{0,80}(아닙|뜻하지|보장하지|보증하지)`)
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
			"brew install jgoneit/tap/jaekit", "ha --version", "ha 0.1.3", "which -a ha",
			"claude plugin marketplace add jgoneit/jaekit#v0.1.3", "claude plugin install spec@jaekit", "claude plugin install seal@jaekit",
			"codex plugin marketplace add jgoneit/jaekit@v0.1.3", "codex plugin add spec@jaekit", "codex plugin add seal@jaekit",
			"/spec:spec ", "$spec:spec ", "@Spec", "/seal:seal docs/specs/<goal>", "$seal:seal docs/specs/<goal>",
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
			visualRequire(t, l.install+" version prerequisite destination", visualSection(t, l.install, versionHeading), "ha --version", "which -a ha", "ha 0.1.3")
		}

		before := visualBeforeInstall(t, l)
		visualRequire(t, l.readme+" before install", before, "Homebrew", "git", "Windows", "Seal", "Spec")
		if strings.Contains(l.readme, ".en.md") {
			visualRequirePattern(t, l.readme+" Windows support", before, `(?i)does not support (it|Windows)|Windows.{0,80}(not supported|unsupported)`)
			visualRequirePattern(t, l.readme+" Windows verification", before, `(?i)(has|have) not been checked`)
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
		visualRequire(t, l.readme+" troubleshooting", guide, "ha: command not found", "which -a ha", "/spec:spec", "/seal:seal", "claude plugin uninstall <plugin>", "codex plugin remove <plugin>", "@Spec", "$spec", "Cannot add marketplace")
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
	koLinks := onboardNormLinks(onboardLinks(visualRead(t, ko.readme)))
	enLinks := onboardNormLinks(onboardLinks(visualRead(t, en.readme)))
	if strings.Join(koLinks, "\n") != strings.Join(enLinks, "\n") {
		t.Errorf("README link destinations differ between languages:\n%v\n%v", koLinks, enLinks)
	}
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
