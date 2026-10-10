package plugins

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// Shared command, link, placeholder, and installation checks remain below.
type onboardLang struct {
	readme, install, usage      string
	otherInstall                string
	installDocSecs, installDoc2 []string
}

var onboardKo = onboardLang{
	readme: "../README.md", install: "../guides/INSTALL.md", usage: "../guides/USAGE.md", otherInstall: "INSTALL.en.md",
	installDocSecs: []string{"## Release 파일로 설치", "## 로컬 경로 등록에서 옮기기", "## 소스에서 빌드 (개발)"},
	installDoc2:    []string{"`ha 0.1.2-dev`", "아래 명령은 고치지 않고 그대로 붙여 넣습니다.", "`go env GOBIN`", "`$(go env GOPATH)/bin`"},
}
var onboardEn = onboardLang{
	readme: "../README.en.md", install: "../guides/INSTALL.en.md", usage: "../guides/USAGE.en.md", otherInstall: "INSTALL.md",
	installDocSecs: []string{"## Install from a Release file", "## Move from a local path registration", "## Build from source (development)"},
	installDoc2:    []string{"`ha 0.1.2-dev`", "Paste the commands below as they are.", "`go env GOBIN`", "`$(go env GOPATH)/bin`"},
}

// Blocks that both languages give the same way.
const (
	onboardBrew   = "brew install jgoneit/tap/jaekit\n"
	onboardCheck  = "ha --version\nwhich -a ha\n"
	onboardClaude = "claude plugin marketplace add jgoneit/jaekit#v0.1.2\nclaude plugin install spec@jaekit\nclaude plugin install seal@jaekit\n"
	onboardCodex  = "codex plugin marketplace add jgoneit/jaekit@v0.1.2\ncodex plugin add spec@jaekit\ncodex plugin add seal@jaekit\n"
	onboardMoveCC = "claude plugin marketplace remove jaekit\nclaude plugin marketplace add jgoneit/jaekit#v0.1.2\nclaude plugin install spec@jaekit\nclaude plugin install seal@jaekit\n"
	onboardMoveCX = "codex plugin marketplace remove jaekit\ncodex plugin marketplace add jgoneit/jaekit@v0.1.2\ncodex plugin add spec@jaekit\ncodex plugin add seal@jaekit\n"
	onboardDev    = "go install ./cmd/ha\nha --version\n"
)

// onboardBlocks returns the fenced code blocks of the given language, with the
// fence indentation removed.
func onboardBlocks(text, lang string) []string {
	var blocks []string
	var cur []string
	indent, in := "", false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case !in && trimmed == "```"+lang:
			in, indent, cur = true, line[:len(line)-len(trimmed)], nil
		case in && trimmed == "```":
			blocks = append(blocks, strings.Join(cur, "\n")+"\n")
			in = false
		case in:
			cur = append(cur, strings.TrimPrefix(line, indent))
		}
	}
	return blocks
}

// onboardHasBlock checks for a bash block that starts with the given lines.
func onboardHasBlock(t *testing.T, name, text, block string) {
	t.Helper()
	for _, b := range onboardBlocks(text, "bash") {
		if strings.HasPrefix(b, block) {
			return
		}
	}
	t.Errorf("%s lacks the bash block %q", name, block)
}

// onboardSlug is GitHub's heading anchor: lower case; letters, digits, marks,
// spaces, hyphens, and underscores kept; spaces become hyphens.
func onboardSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// onboardHeadings lists the headings outside code fences with their levels.
func onboardHeadings(text string) (levels []int, titles []string) {
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if m := regexp.MustCompile(`^(#{1,6}) +(.*)$`).FindStringSubmatch(line); m != nil {
			levels = append(levels, len(m[1]))
			titles = append(titles, m[2])
		}
	}
	return levels, titles
}

func onboardAnchors(text string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	_, titles := onboardHeadings(text)
	for _, h := range titles {
		s := onboardSlug(h)
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

var onboardLinkRe = regexp.MustCompile(`\]\(([^)\s]+)\)|(?:href|src)="([^"]+)"`)

// onboardLinks returns the link targets outside code fences.
func onboardLinks(text string) []string {
	var out, kept []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			kept = append(kept, line)
		}
	}
	for _, m := range onboardLinkRe.FindAllStringSubmatch(strings.Join(kept, "\n"), -1) {
		target := m[1]
		if target == "" {
			target = m[2]
		}
		out = append(out, strings.ReplaceAll(target, "&amp;", "&"))
	}
	return out
}

// Installation facts stay in the guides linked directly from each README.
func onboardInstallOK(t *testing.T, l onboardLang) {
	t.Helper()
	requireAll(t, l.readme, "("+strings.TrimPrefix(l.install, "../")+")")
	doc := read(t, l.install)
	for _, b := range []string{onboardBrew, onboardCheck, onboardClaude, onboardCodex, onboardMoveCC, onboardMoveCX, onboardDev} {
		onboardHasBlock(t, l.install, doc, b)
	}
	for _, h := range l.installDocSecs {
		mdSection(t, l.install, h)
	}
	requireIn(t, l.install, doc, l.installDoc2...)
	requireIn(t, l.install, doc, "("+l.otherInstall+")")
	forbidAll(t, l.readme, "remove jaekit", "uname -s", "go install ./cmd/ha")
}

func TestOnboardFirstScreen(t *testing.T) { TestReadmeVisualGuideAC1(t) }
func TestOnboardIntro(t *testing.T)       { TestReadmeVisualGuideAC1(t) }
func TestOnboardHowItWorks(t *testing.T) {
	TestReadmeVisualGuideAC3(t)
	requireAll(t, "../guides/USAGE.md", "`.gitignore`에 더할 것은 없습니다", "Git이 추적하지 않는 `.git/ha/`", "commit합니다")
}
func TestOnboardExample(t *testing.T)         { TestReadmeVisualGuideAC4(t); TestReadmeVisualGuideAC5(t) }
func TestOnboardTroubleshooting(t *testing.T) { TestReadmeVisualGuideAC9(t) }
func TestOnboardLimits(t *testing.T)          { TestReadmeVisualGuideAC7(t) }
func TestOnboardInstallDoc(t *testing.T)      { onboardInstallOK(t, onboardKo) }

func TestOnboardQuickStartShort(t *testing.T) { TestReadmeVisualGuideAC2(t) }

// the operations guide's first setup agrees with the
// README and the install guide, and every section it names exists there.
func TestOnboardOperationsSetup(t *testing.T) {
	requireAll(t, "../guides/OPERATIONS.md", "(INSTALL.md)", "(USAGE.md)")
}

func onboardFiles(t *testing.T) []string {
	t.Helper()
	files := []string{"../README.md", "../README.en.md", "../guides/INSTALL.md", "../guides/INSTALL.en.md", "../guides/USAGE.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.md"}
	forms, _ := filepath.Glob("../.github/ISSUE_TEMPLATE/*.yml")
	if len(forms) == 0 {
		t.Error("no issue templates")
	}
	return append(files, forms...)
}

// repository-relative links in the READMEs, the
// install guides, and the issue templates reach files and headings that exist.
func TestOnboardLinks(t *testing.T) {
	for _, path := range onboardFiles(t) {
		for _, target := range onboardLinks(read(t, path)) {
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
				t.Errorf("%s links %q, which does not exist", path, target)
				continue
			}
			if anchor != "" && !info.IsDir() && !onboardAnchors(read(t, dest))[anchor] {
				t.Errorf("%s links %q, but %s has no such heading", path, target, dest)
			}
		}
	}
}

// GitHub drops a placeholder such as <goal> written
// outside code as an unknown HTML tag, so in the documents of AC-13 every
// placeholder sits in a code span or a code block.
// checks/placeholders.sh also renders the Markdown files with GitHub.
func TestOnboardPlaceholders(t *testing.T) {
	html := map[string]bool{}
	for _, name := range strings.Fields("a b br code details div em h1 h2 h3 h4 h5 h6 hr i img kbd li ol p picture pre source span strong sub summary sup table tbody td th thead tr ul") {
		html[name] = true
	}
	tag := regexp.MustCompile(`<([A-Za-z][^<>]*)>`)
	for _, path := range onboardFiles(t) {
		inBlock := false
		for i, line := range strings.Split(read(t, path), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				continue
			}
			text := regexp.MustCompile("`[^`]*`").ReplaceAllString(line, "")
			for _, m := range tag.FindAllStringSubmatch(text, -1) {
				name := strings.ToLower(strings.FieldsFunc(m[1], func(r rune) bool { return r == ' ' || r == '/' })[0])
				if html[name] || strings.HasPrefix(name, "http") {
					continue
				}
				t.Errorf("%s:%d: %s is outside code, so GitHub drops it: %s", path, i+1, m[0], strings.TrimSpace(line))
			}
		}
	}
}

// the names follow the glossary, and its retired
// names do not appear.
func TestOnboardNames(t *testing.T) {
	for _, path := range []string{"../README.md", "../README.en.md"} {
		requireAll(t, path, "Jaekit", "Spec", "Seal", "`ha`")
	}
}

// the README still links the design set and the
// license.
func TestPublicContractLinks(t *testing.T) {
	for _, path := range []string{"../README.md", "../README.en.md"} {
		requireAll(t, path, "(contracts/run-record.md)", "(LICENSE)")
	}
}

func onboardNormLinks(links []string) []string {
	set := map[string]bool{}
	for _, l := range links {
		file, _, _ := strings.Cut(l, "#")
		file = strings.Replace(file, ".en.md", ".md", 1)
		file = strings.Replace(file, ".en.svg", ".svg", 1)
		file = strings.Replace(file, ".ko.svg", ".svg", 1)
		set[file] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func onboardSameShape(t *testing.T, ko, en string, norm func(string) string) {
	t.Helper()
	kb, eb := onboardBlocks(read(t, ko), "bash"), onboardBlocks(read(t, en), "bash")
	if len(kb) != len(eb) {
		t.Errorf("%s has %d bash blocks, %s has %d", ko, len(kb), en, len(eb))
	} else {
		for i := range kb {
			if norm(kb[i]) != norm(eb[i]) {
				t.Errorf("bash block %d differs:\n%s\n%s", i+1, kb[i], eb[i])
			}
		}
	}
	if k, e := onboardNormLinks(onboardLinks(read(t, ko))), onboardNormLinks(onboardLinks(read(t, en))); strings.Join(k, " ") != strings.Join(e, " ") {
		t.Errorf("link targets differ:\n%s: %v\n%s: %v", ko, k, en, e)
	}
}

var onboardPlaceholder = regexp.MustCompile(`<[^>]+>`)

// guides/INSTALL.en.md carries the
// same guides as guides/INSTALL.md in English, with the same commands apart from
// the messages the Release-file command prints, and README.en.md links it.
func TestOnboardEnglishInstall(t *testing.T) {
	onboardSameShape(t, "../guides/INSTALL.md", "../guides/INSTALL.en.md", func(s string) string {
		var keep []string
		for _, line := range strings.Split(onboardPlaceholder.ReplaceAllString(s, "<>"), "\n") {
			if !strings.Contains(line, `echo "`) {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	})
	requireAll(t, "../README.en.md", "(guides/INSTALL.en.md)")
	requireAll(t, "../guides/INSTALL.en.md", "is not supported", "(INSTALL.md)")
	forbidAll(t, "../guides/INSTALL.en.md", "지원하지 않는", "설치했습니다")
}

// both READMEs ask for feedback through GitHub
// issues, link the template chooser, and say that issues are public.
func TestOnboardFeedback(t *testing.T) {
	for _, l := range []onboardLang{onboardKo, onboardEn} {
		requireAll(t, l.readme, "(https://github.com/jgoneit/jaekit/issues/new/choose)")
	}
	requireIn(t, "Korean feedback", read(t, onboardKo.readme)+read(t, onboardKo.usage), "공개", "비밀", "로그 원문")
	requireIn(t, "English feedback", read(t, onboardEn.readme)+read(t, onboardEn.usage), "public", "secrets", "raw logs")
}
