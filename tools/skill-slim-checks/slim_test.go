//go:build skillslim

package skillslimchecks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Body targets (AC-1 to AC-3) and caps (AC-4), in whitespace-separated words.
var (
	targets = map[string]int{"spec": 2500, "spar": 2300, "seal": 2000}
	caps    = map[string]int{"spec": 2800, "spar": 2600, "seal": 2300}
	guides  = []string{"guides/DEVELOPMENT.md", "guides/DEVELOPMENT.en.md"}
)

func requireBody(t *testing.T, name string) {
	s := skillNamed(name)
	if n := words(readFile(t, repoRoot(t), s.body())); n > targets[name] {
		t.Errorf("jaekit:violation=body-over-target: %s has %d words, above %d", s.body(), n, targets[name])
	}
}

// AC-1.
func TestSkillSlimAC1_SpecBody(t *testing.T) { requireBody(t, "spec") }

// AC-2.
func TestSkillSlimAC2_SparBody(t *testing.T) { requireBody(t, "spar") }

// AC-3.
func TestSkillSlimAC3_SealBody(t *testing.T) { requireBody(t, "seal") }

// number formats n as the plugins tests and guides may write it.
func number(n int) []string {
	plain := strconv.Itoa(n)
	if n < 1000 {
		return []string{plain}
	}
	return []string{plain, fmt.Sprintf("%d,%03d", n/1000, n%1000)}
}

// mentionsNumber reports whether one failed test names the path and both
// numbers, each in either format.
func mentionsNumber(run innerRun, path string, a, b int) bool {
	for _, x := range number(a) {
		for _, y := range number(b) {
			if run.mentions(path, x, y) {
				return true
			}
		}
	}
	return false
}

// pad appends one paragraph of filler words to a skill's SKILL.md so that it
// has n words.
func pad(t tb, dir string, s skill, n int) {
	text := readFile(t, dir, s.body())
	have := words(text)
	if have > n {
		t.Fatalf("setup: %s has %d words, more than %d", s.body(), have, n)
	}
	if have == n {
		return
	}
	writeFile(t, dir, s.body(), strings.TrimRight(text, "\n")+"\n\n"+strings.TrimSpace(strings.Repeat("pad ", n-have))+"\n")
}

// AC-4: a copy with a skill one word above its cap fails a plugins test that
// names the skill's path, the count and the cap; a copy at the cap passes.
func TestSkillSlimAC4_SkillCaps(t *testing.T) {
	root := repoRoot(t)
	requireControl(t)
	var over, at []*edited
	var names []string
	for _, s := range skills {
		s, limit := s, caps[s.name]
		if n := words(readFile(t, root, s.body())); n > limit {
			t.Errorf("jaekit:violation=cap-not-enforced: %s has %d words, above its cap %d, and the plugins tests pass", s.body(), n, limit)
			continue
		}
		names = append(names, s.name)
		over = append(over, &edited{what: s.name + " one word above its cap", change: func(t tb, dir string) { pad(t, dir, s, limit+1) }})
		at = append(at, &edited{what: s.name + " at its cap", change: func(t tb, dir string) { pad(t, dir, s, limit) }})
	}
	runEdited(t, append(append([]*edited{}, over...), at...))
	for i, name := range names {
		s, limit := skillNamed(name), caps[name]
		path := strings.TrimPrefix(s.body(), "plugins/")
		if over[i].run.passed {
			t.Errorf("jaekit:violation=cap-not-enforced: the plugins tests pass with %s at %d words, above its cap %d", s.body(), limit+1, limit)
		} else if !mentionsNumber(over[i].run, path, limit+1, limit) {
			t.Errorf("jaekit:violation=cap-not-enforced: with %s at %d words no failing plugins test names %s, %d and %d: %s",
				s.body(), limit+1, path, limit+1, limit, strings.Join(over[i].run.failed, ", "))
		}
		if !at[i].run.passed {
			t.Errorf("jaekit:violation=cap-not-enforced: the plugins tests fail with %s at exactly its cap %d: %s",
				s.body(), limit, strings.Join(at[i].run.failed, ", "))
		}
	}
}

// capLine returns the index of the first line of a guide that links the
// skill's SKILL.md and states its cap, the cap as written there, or -1.
func capLine(t tb, root, guide string, s skill) (int, string) {
	text := readFile(t, root, guide)
	lines := strings.Split(text, "\n")
	for i, kind := range classify(text) {
		if kind != kindProse {
			continue
		}
		linked := false
		for _, target := range linkTargets(lines[i]) {
			path, _, _ := strings.Cut(resolve(guide, target), "#")
			linked = linked || path == s.body()
		}
		if !linked {
			continue
		}
		for _, form := range number(caps[s.name]) {
			if regexp.MustCompile(`(^|[^\d,])` + regexp.QuoteMeta(form) + `($|[^\d,])`).MatchString(lines[i]) {
				return i, form
			}
		}
	}
	return -1, ""
}

// AC-4: each development guide states each skill's cap on a line that links
// the skill, and a copy whose guide states another number fails a plugins
// test that names that guide.
func TestSkillSlimAC4_GuideCaps(t *testing.T) {
	root := repoRoot(t)
	var trees []*edited
	var guideOf []string
	for _, guide := range guides {
		for _, s := range skills {
			guide, s := guide, s
			i, form := capLine(t, root, guide, s)
			if i < 0 {
				t.Errorf("jaekit:violation=cap-not-stated: %s has no line that links %s and states its cap %d", guide, s.body(), caps[s.name])
				continue
			}
			other := number(caps[s.name] + 1)[len(number(caps[s.name]))-1]
			if !strings.Contains(form, ",") {
				other = strconv.Itoa(caps[s.name] + 1)
			}
			trees = append(trees, &edited{what: guide + " states another cap for " + s.name, change: func(t tb, dir string) {
				lines := strings.Split(readFile(t, dir, guide), "\n")
				lines[i] = strings.Replace(lines[i], form, other, 1)
				writeFile(t, dir, guide, strings.Join(lines, "\n"))
			}})
			guideOf = append(guideOf, guide)
		}
	}
	if len(trees) == 0 {
		return
	}
	runEdited(t, trees)
	for i, tree := range trees {
		if tree.run.passed {
			t.Errorf("jaekit:violation=cap-not-stated: the plugins tests pass on a copy where %s", tree.what)
		} else if !tree.run.mentions(filepath.Base(guideOf[i])) {
			t.Errorf("jaekit:violation=cap-not-stated: on a copy where %s no failing plugins test names %s: %s",
				tree.what, filepath.Base(guideOf[i]), strings.Join(tree.run.failed, ", "))
		}
	}
}

// policy is the policy sentence for changes that reach a cap, one per guide.
var policy = map[string]string{
	"guides/DEVELOPMENT.md":    "변경이 스킬을 상한 위로 밀면, 같은 변경에서 그 스킬의 드문 상황에만 쓰는 규칙을 그 스킬의 참조 문서로 옮긴다. 상한은 사용자의 말로만 올린다.",
	"guides/DEVELOPMENT.en.md": "When a change would push a skill above its limit, the same change moves rules used only in rare situations to that skill's reference documents. A limit is raised only by the user's words.",
}

// AC-5.
func TestSkillSlimAC5_Policy(t *testing.T) {
	root := repoRoot(t)
	for _, guide := range guides {
		flat := strings.Join(strings.Fields(readFile(t, root, guide)), " ")
		if !strings.Contains(flat, policy[guide]) {
			t.Errorf("jaekit:violation=policy-missing: %s does not state %q", guide, policy[guide])
		}
	}
}

// AC-6: every relative link of the three skills' documents names an existing
// path and, into a Markdown file, an existing heading.
func TestSkillSlimAC6_LinksResolve(t *testing.T) {
	root := repoRoot(t)
	for _, s := range skills {
		for _, doc := range docs(t, root, s) {
			text := readFile(t, root, doc)
			lines := strings.Split(text, "\n")
			for i, kind := range classify(text) {
				if kind != kindProse && kind != kindHead {
					continue
				}
				for _, target := range linkTargets(lines[i]) {
					full := resolve(doc, target)
					if strings.Contains(full, "://") || strings.HasPrefix(full, "mailto:") {
						continue
					}
					path, anchor, _ := strings.Cut(full, "#")
					if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
						t.Errorf("jaekit:violation=moved-unlinked: %s:%d links %s, which does not exist", doc, i+1, target)
						continue
					}
					if anchor != "" && (!strings.HasSuffix(path, ".md") || !slugs(readFile(t, root, path))[anchor]) {
						t.Errorf("jaekit:violation=moved-unlinked: %s:%d links %s, whose heading does not exist", doc, i+1, target)
					}
				}
			}
		}
	}
}

// moved is a reference section that holds rules moved out of a body.
type moved struct{ skill, doc, heading string }

// representatives are the moves AC-6 names, and the section the Seal check
// input rules move into; every other new level-2 reference section is found
// against the base list.
var representatives = []moved{
	{"seal", "references/bundle.md", "Check inputs"},
	{"spec", "references/agent-invocations.md", "New goals"},
	{"spec", "references/agent-invocations.md", "Amendments"},
	{"spec", "references/agent-invocations.md", "Acceptance"},
	{"spar", "references/rounds.md", "At the code cap"},
	{"spar", "references/rounds.md", "Reviewer standpoints"},
}

var whenToRead = regexp.MustCompile(`(?i)\bread\b.*\b(when|whenever|before|after|if|while|once)\b`)

// AC-6: each moved section has a line in its SKILL.md that links it and says
// when to read it.
func TestSkillSlimAC6_MovedLinked(t *testing.T) {
	root := repoRoot(t)
	var base map[string][][2]string
	expected(t, "base-reference-headings.json", &base)
	want := map[moved]bool{}
	for _, m := range representatives {
		want[m] = true
	}
	for _, s := range skills {
		known := map[[2]string]bool{}
		for _, row := range base[s.name] {
			known[row] = true
		}
		for _, doc := range docs(t, root, s)[1:] {
			text := readFile(t, root, doc)
			lines := strings.Split(text, "\n")
			for i, kind := range classify(text) {
				if kind == kindHead && headingLevel(lines[i]) == 2 && !known[[2]string{doc, headingText(lines[i])}] {
					want[moved{s.name, strings.TrimPrefix(doc, s.dir+"/"), headingText(lines[i])}] = true
				}
			}
		}
	}
	var list []moved
	for m := range want {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool { return fmt.Sprint(list[i]) < fmt.Sprint(list[j]) })
	for _, m := range list {
		s := skillNamed(m.skill)
		doc := s.dir + "/" + m.doc
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(doc))); err != nil {
			t.Errorf("jaekit:violation=moved-unlinked: %s does not exist, so it holds no section %q", doc, m.heading)
			continue
		}
		anchor := slug(m.heading)
		if !slugs(readFile(t, root, doc))[anchor] {
			t.Errorf("jaekit:violation=moved-unlinked: %s has no section %q", doc, m.heading)
			continue
		}
		body := readFile(t, root, s.body())
		lines := strings.Split(body, "\n")
		found := false
		for i, kind := range classify(body) {
			if kind != kindProse {
				continue
			}
			links := false
			for _, target := range linkTargets(lines[i]) {
				links = links || resolve(s.body(), target) == doc+"#"+anchor
			}
			found = found || links && whenToRead.MatchString(anyLinks.ReplaceAllString(masked(lines[i]), "]"))
		}
		if !found {
			t.Errorf("jaekit:violation=moved-unlinked: %s has no line that links %s#%s and says when to read it", s.body(), m.doc, anchor)
		}
	}
}

// authority is a rule only the user decides, or something the agent must not
// do, that stays in its skill's body: the code span holding token sits in
// the named body section.
type authority struct{ skill, section, token string }

var authorities = []authority{
	{"spec", "## Agent invocations", "Status: Ready"},
	{"spec", "## Agent invocations", "amendments/<name>.md"},
	{"spec", "## Writing and reporting", "implement-now"},
	{"spec", "## Boundaries", "docs/specs/<goal>/"},
	{"seal", "## Authority", "ha start --request"},
	{"seal", "## Authority", "Status: Ready"},
	{"seal", "## Authority", "SPEC.md"},
	{"seal", "## Independent review", "hardening"},
	{"seal", "## Independent review", "spar extend"},
	{"seal", "## Budget", "ha budget"},
	{"seal", "## Forbidden", "SPEC.md"},
	{"seal", "## Forbidden", "runs.jsonl"},
	{"seal", "## Forbidden", "block"},
	{"seal", "## Forbidden", "ha log"},
	{"seal", "## Forbidden", "## 개선 메모"},
	{"spar", "## When to run each stage", "spar extend"},
	{"spar", "## Reviewers", "spar waive"},
	{"spar", "## Classes and dispositions", "answer"},
}

// authorityLine returns the index of the first line of the rule's body
// section with a code span that holds its token, or -1.
func authorityLine(t tb, root string, a authority) int {
	text := readFile(t, root, skillNamed(a.skill).body())
	start, end, ok := section(text, a.section)
	if !ok {
		return -1
	}
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	for i := start + 1; i < end; i++ {
		if kinds[i] != kindProse {
			continue
		}
		for _, r := range codeRanges(lines[i]) {
			if strings.Contains(codeText(lines[i], r), a.token) {
				return i
			}
		}
	}
	return -1
}

// AC-7: each representative rule is in its body section.
func TestSkillSlimAC7_InBody(t *testing.T) {
	root := repoRoot(t)
	for _, a := range authorities {
		if authorityLine(t, root, a) < 0 {
			t.Errorf("jaekit:violation=authority-moved: %s %s holds no code span with %q", skillNamed(a.skill).body(), a.section, a.token)
		}
	}
}

// AC-7: a copy whose rule moved from its body section to the skill's first
// reference document fails a plugins test that names the rule's token.
func TestSkillSlimAC7_MoveFails(t *testing.T) {
	root := repoRoot(t)
	var trees []*edited
	var rules []authority
	for _, a := range authorities {
		a := a
		i := authorityLine(t, root, a)
		if i < 0 {
			t.Errorf("jaekit:violation=authority-moved: %s %s holds no code span with %q", skillNamed(a.skill).body(), a.section, a.token)
			continue
		}
		s := skillNamed(a.skill)
		refs := docs(t, root, s)[1:]
		if len(refs) == 0 {
			t.Fatalf("setup: %s has no reference document to move %q into", s.dir, a.token)
		}
		trees = append(trees, &edited{what: fmt.Sprintf("moves the %s rule with %q to %s", s.name, a.token, refs[0]),
			change: func(t tb, dir string) {
				start, end := block(readFile(t, dir, s.body()), i)
				moveBlock(t, dir, s.body(), start, end, refs[0])
			}})
		rules = append(rules, a)
	}
	if len(trees) == 0 {
		return
	}
	runEdited(t, trees)
	for i, tree := range trees {
		if tree.run.passed {
			t.Errorf("jaekit:violation=authority-moved: the plugins tests pass on a copy that %s", tree.what)
		} else if !tree.run.mentions(rules[i].token) {
			t.Errorf("jaekit:violation=authority-moved: on a copy that %s no failing plugins test names %q: %s",
				tree.what, rules[i].token, strings.Join(tree.run.failed, ", "))
		}
	}
}

// exempt reports whether a top-level plugins test is one AC-8 excepts: a test
// defined in a plugins file that a regression route with a skill source
// checks, or a TestCompatibilityAC10_ or TestCompatibilityAC11_ test.
func exempt(t tb, root string) func(string) bool {
	t.Helper()
	names := map[string]bool{}
	define := regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	for _, file := range protectedPluginFiles(t, root) {
		found := define.FindAllStringSubmatch(readFile(t, root, file), -1)
		if len(found) == 0 {
			t.Fatalf("setup: %s defines no test", file)
		}
		for _, m := range found {
			names[m[1]] = true
		}
	}
	return func(name string) bool {
		return names[name] || strings.HasPrefix(name, "TestCompatibilityAC10_") || strings.HasPrefix(name, "TestCompatibilityAC11_")
	}
}

type route struct {
	ID       string   `json:"id"`
	Sources  []string `json:"sources"`
	Invar    string   `json:"invariant"`
	Status   string   `json:"status"`
	Property string   `json:"property"`
	Boundary string   `json:"boundary"`
	Checks   []struct {
		File, Selector, Group string
	} `json:"checks"`
}

func routes(t tb, root string) []route {
	t.Helper()
	var reg struct {
		Routes []route `json:"routes"`
	}
	if err := json.Unmarshal([]byte(readFile(t, root, "contracts/regressions.json")), &reg); err != nil || len(reg.Routes) == 0 {
		t.Fatalf("setup: contracts/regressions.json has no readable routes: %v", err)
	}
	return reg.Routes
}

func skillSource(path string) bool {
	return regexp.MustCompile(`^plugins/[^/]+/skills/[^/]+/(SKILL\.md|references/[^/]+\.md)$`).MatchString(path)
}

// protectedPluginFiles are the plugins test files that routes with a skill
// source check.
func protectedPluginFiles(t tb, root string) []string {
	files := map[string]bool{}
	for _, r := range routes(t, root) {
		skillRoute := false
		for _, src := range r.Sources {
			skillRoute = skillRoute || skillSource(src)
		}
		for _, c := range r.Checks {
			if skillRoute && strings.HasPrefix(c.File, "plugins/") {
				files[c.File] = true
			}
		}
	}
	var out []string
	for f := range files {
		out = append(out, f)
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatalf("setup: no plugins test file is checked by a route with a skill source")
	}
	return out
}

var pyString = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'`)

// keptPhrases are the phrases AC-8 excepts, found in the skills' prose: the
// string literals of the protected contract checks, of the protected plugins
// test files, and of the TestCompatibilityAC10_ and AC11_ tests.
func keptPhrases(t tb, root string) []string {
	t.Helper()
	var literals []string
	sources, err := filepath.Glob(filepath.Join(root, "tools", "contract-checks", "*.py"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("setup: no tools/contract-checks/*.py: %v", err)
	}
	for _, src := range sources {
		if strings.HasPrefix(filepath.Base(src), "test_") {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		for _, m := range pyString.FindAllStringSubmatch(string(data), -1) {
			lit := m[1] + m[2]
			lit = strings.NewReplacer(`\\`, `\`, `\'`, `'`, `\"`, `"`).Replace(lit)
			literals = append(literals, lit)
		}
	}
	goLiterals := func(file string, keep func(name string) bool) {
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(file)), nil, 0)
		if err != nil {
			t.Fatalf("setup: parse %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && !keep(fn.Name.Name) {
				continue
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						literals = append(literals, s)
					}
				}
				return true
			})
		}
	}
	for _, file := range protectedPluginFiles(t, root) {
		goLiterals(file, func(string) bool { return true })
	}
	goLiterals("plugins/compatibility_test.go", func(name string) bool {
		return strings.HasPrefix(name, "TestCompatibilityAC10_") || strings.HasPrefix(name, "TestCompatibilityAC11_")
	})
	var prose []string
	for _, s := range skills {
		for _, doc := range docs(t, root, s) {
			prose = append(prose, readFile(t, root, doc))
		}
	}
	all := strings.Join(prose, "\n")
	seen := map[string]bool{}
	var out []string
	for _, lit := range literals {
		if len(strings.TrimSpace(lit)) >= 3 && !seen[lit] && strings.Contains(all, lit) {
			seen[lit] = true
			out = append(out, lit)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// keptRanges marks what a reworded prose line keeps: code spans, link
// targets, angle-bracket spans, URLs and the excepted phrases.
func keptRanges(line string, kept []string) []bool {
	keep := make([]bool, len(line))
	mark := func(a, b int) {
		for i := a; i < b; i++ {
			keep[i] = true
		}
	}
	for _, r := range codeRanges(line) {
		mark(r[0], r[1])
	}
	for _, re := range []*regexp.Regexp{anyLinks, angleRe, bareURL} {
		for _, m := range re.FindAllStringIndex(line, -1) {
			mark(m[0]+boolInt(re == anyLinks), m[1])
		}
	}
	for _, p := range kept {
		for from := 0; ; {
			i := strings.Index(line[from:], p)
			if i < 0 {
				break
			}
			mark(from+i, from+i+len(p))
			from += i + len(p)
		}
	}
	return keep
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// shift turns every letter of a prose line outside what it keeps into
// another letter: ROT13 for ASCII letters, the next syllable for Hangul.
func shift(line string, kept []string) string {
	keep := keptRanges(line, kept)
	var b strings.Builder
	for i, r := range line {
		switch {
		case keep[i]:
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune('a' + (r-'a'+13)%26)
		case r >= 'A' && r <= 'Z':
			b.WriteRune('A' + (r-'A'+13)%26)
		case r >= 0xAC00 && r <= 0xD7A3:
			b.WriteRune(0xAC00 + (r-0xAC00+1)%11172)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rewordAll shifts the letters of every prose line of the three skills'
// documents in dir and returns how many lines changed.
func rewordAll(t tb, dir string, kept []string) int {
	changed := 0
	for _, s := range skills {
		for _, doc := range docs(t, dir, s) {
			text := readFile(t, dir, doc)
			lines := strings.Split(text, "\n")
			for i, kind := range classify(text) {
				if kind != kindProse {
					continue
				}
				if r := shift(lines[i], kept); r != lines[i] {
					lines[i] = r
					changed++
				}
			}
			writeFile(t, dir, doc, strings.Join(lines, "\n"))
		}
	}
	return changed
}

// AC-8: a copy with every prose line of the three skills' SKILL.md and
// reference documents reworded passes every plugins test AC-8 does not
// except.
func TestSkillSlimAC8_AllProse(t *testing.T) {
	root := repoRoot(t)
	kept := keptPhrases(t, root)
	tree := &edited{what: "rewords every prose line of the three skills", change: func(t tb, dir string) {
		if rewordAll(t, dir, kept) == 0 {
			t.Fatalf("setup: the copy has no prose line to reword")
		}
	}}
	runEdited(t, []*edited{tree})
	if failed := tree.run.failedOutside(exempt(t, root)); len(failed) > 0 {
		t.Errorf("jaekit:violation=prose-pinned: the plugins tests fail on a copy that %s: %s", tree.what, strings.Join(failed, ", "))
	}
}

// units splits the text of a prose line after its list or table marker into
// whitespace-separated words, keeping each kept range whole.
func units(line string, kept []string) (prefix string, out []string) {
	if m := itemStart.FindStringIndex(line); m != nil {
		prefix, line = line[:m[1]], line[m[1]:]
	}
	keep := keptRanges(line, kept)
	cur := ""
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' && !keep[i] {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += line[i : i+1]
	}
	if cur != "" {
		out = append(out, cur)
	}
	return prefix, out
}

// reword reverses the order of a prose line's words, keeping code spans,
// links and excepted phrases whole.
func reword(line string, kept []string) string {
	prefix, words := units(line, kept)
	for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
		words[i], words[j] = words[j], words[i]
	}
	return prefix + strings.Join(words, " ")
}

// sectionDoc finds the document of a skill that holds heading.
func sectionDoc(t tb, root string, s skill, heading string) string {
	for _, doc := range docs(t, root, s) {
		if _, _, ok := section(readFile(t, root, doc), heading); ok {
			return doc
		}
	}
	t.Fatalf("setup: no document of %s has the section %q", s.dir, heading)
	return ""
}

// rewordFirst rewords the first prose line of a section that is not a table
// row.
func rewordFirst(t tb, dir string, s skill, heading string, kept []string) {
	doc := sectionDoc(t, dir, s, heading)
	text := readFile(t, dir, doc)
	start, end, _ := section(text, heading)
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	for i := start + 1; i < end; i++ {
		if kinds[i] == kindProse && !strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			lines[i] = reword(lines[i], kept)
			writeFile(t, dir, doc, strings.Join(lines, "\n"))
			return
		}
	}
	t.Fatalf("setup: %s %s has no prose line", doc, heading)
}

// moveFirstRule moves the first block of the Spec body that lies in a
// level-2 section other than an AC-7 section and holds no AC-7 token to the
// skill's goal-docs reference.
func moveFirstRule(t tb, dir string) {
	s := skillNamed("spec")
	text := readFile(t, dir, s.body())
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	protectedSection := map[string]bool{}
	for _, a := range authorities {
		if a.skill == "spec" {
			protectedSection[a.section] = true
		}
	}
	current := ""
	for i, kind := range kinds {
		if kind == kindHead && headingLevel(lines[i]) == 2 {
			current = strings.TrimSpace(lines[i])
			continue
		}
		if kind != kindProse || current == "" || protectedSection[current] {
			continue
		}
		start, end := block(text, i)
		holds := false
		for _, l := range lines[start:end] {
			for _, a := range authorities {
				holds = holds || a.skill == "spec" && strings.Contains(l, a.token)
			}
		}
		if !holds {
			moveBlock(t, dir, s.body(), start, end, s.dir+"/references/goal-docs.md")
			return
		}
	}
	t.Fatalf("setup: the Spec body has no block to move")
}

// AC-8: the representative edits AC-8 names each leave every plugins test
// AC-8 does not except passing.
func TestSkillSlimAC8_Representatives(t *testing.T) {
	root := repoRoot(t)
	kept := keptPhrases(t, root)
	trees := []*edited{
		{what: "rewords a sentence of Spec Questions", change: func(t tb, dir string) {
			rewordFirst(t, dir, skillNamed("spec"), "## Questions", kept)
		}},
		{what: "rewords a sentence of Seal Resuming", change: func(t tb, dir string) {
			rewordFirst(t, dir, skillNamed("seal"), "## Resuming", kept)
		}},
		{what: "rewords a sentence of Spar Classes and dispositions", change: func(t tb, dir string) {
			rewordFirst(t, dir, skillNamed("spar"), "## Classes and dispositions", kept)
		}},
		{what: "moves a rule paragraph of the Spec body to its reference", change: moveFirstRule},
	}
	runEdited(t, trees)
	ex := exempt(t, root)
	for _, tree := range trees {
		if failed := tree.run.failedOutside(ex); len(failed) > 0 {
			t.Errorf("jaekit:violation=prose-pinned: the plugins tests fail on a copy that %s: %s", tree.what, strings.Join(failed, ", "))
		}
	}
}

// AC-9: every base route keeps its invariant, status, property and boundary.
func TestSkillSlimAC9_RouteProperties(t *testing.T) {
	root := repoRoot(t)
	var base map[string]struct{ Invariant, Status, Property, Boundary string }
	expected(t, "base-routes.json", &base)
	now := map[string]route{}
	for _, r := range routes(t, root) {
		now[r.ID] = r
	}
	var ids []string
	for id := range base {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b, r := base[id], now[id]
		if r.ID == "" {
			t.Errorf("jaekit:violation=route-changed: route %s is gone", id)
		} else if r.Invar != b.Invariant || r.Status != b.Status || r.Property != b.Property || r.Boundary != b.Boundary {
			t.Errorf("jaekit:violation=route-changed: route %s changed its invariant, status, property or boundary", id)
		}
	}
}

// phrase is one phrase a protected contract check required in a skill
// document at the goal's base.
type phrase struct {
	Module, Suite, Criterion, File, Section, Row, Phrase string
}

// variant is one copy state for a phrase and whether its criterion should
// report the phrase missing there.
type variant struct {
	p      phrase
	name   string
	want   bool // reported missing
	change func(t tb, dir string, pristine map[string]string)
}

// insertIn adds lines to doc: right after the heading line of section when
// section is set (the section is added at the end when the document lacks
// it), else at the end of the document.
func insertIn(text, heading string, add []string) string {
	if heading == "" {
		return strings.TrimRight(text, "\n") + "\n\n" + strings.Join(add, "\n") + "\n"
	}
	start, _, ok := section(text, heading)
	if !ok {
		return strings.TrimRight(text, "\n") + "\n\n" + heading + "\n\n" + strings.Join(add, "\n") + "\n"
	}
	lines := strings.Split(text, "\n")
	out := append(append(append([]string{}, lines[:start+1]...), add...), lines[start+1:]...)
	return strings.Join(out, "\n")
}

// removePhrase deletes every occurrence of p from text, except from the
// heading line that names the phrase's own section, which the check needs to
// find that section.
func removePhrase(text, p, keep string) string {
	lines := strings.Split(text, "\n")
	for i, kind := range classify(text) {
		if kind != kindHead || keep == "" || strings.TrimSpace(lines[i]) != keep {
			lines[i] = strings.ReplaceAll(lines[i], p, "")
		}
	}
	return strings.Join(lines, "\n")
}

// variants builds the three copy states of a phrase: removed from every
// document of its skill (reported missing), and moved only into the skill's
// SKILL.md or into a reference document other than its own file (not
// reported). A row phrase moves with its table row.
func variants(t tb, root string, p phrase) []variant {
	s := skill{}
	for _, c := range skills {
		if strings.HasPrefix(p.File, c.dir+"/") {
			s = c
		}
	}
	if s.name == "" {
		t.Fatalf("setup: %s is in no skill", p.File)
	}
	all := docs(t, root, s)
	targets := []string{s.body()}
	for _, ref := range all[1:] {
		if ref != p.File {
			targets = append(targets, ref)
			break
		}
	}
	strip := func(dir string, pristine map[string]string) (row string) {
		for _, doc := range all {
			text := pristine[doc]
			if p.Row != "" && doc == p.File {
				lines := strings.Split(text, "\n")
				for i, l := range lines {
					if row == "" && strings.HasPrefix(l, "| "+p.Row+" |") && strings.Contains(l, p.Phrase) {
						row = l
						lines = append(lines[:i], lines[i+1:]...)
						break
					}
				}
				text = strings.Join(lines, "\n")
			}
			writeFile(t, dir, doc, removePhrase(text, p.Phrase, p.Section))
		}
		return row
	}
	out := []variant{{p: p, name: "removed from every document of " + s.name, want: true,
		change: func(t tb, dir string, pristine map[string]string) {
			for _, doc := range all {
				writeFile(t, dir, doc, removePhrase(pristine[doc], p.Phrase, p.Section))
			}
		}}}
	for _, target := range targets {
		target := target
		out = append(out, variant{p: p, name: "only in " + target, want: false,
			change: func(t tb, dir string, pristine map[string]string) {
				row := strip(dir, pristine)
				add := []string{p.Phrase}
				if p.Row != "" {
					if row == "" {
						t.Fatalf("setup: %s has no row %q holding %q", p.File, p.Row, p.Phrase)
					}
					add = []string{row}
				}
				writeFile(t, dir, target, insertIn(readFile(t, dir, target), p.Section, add))
			}})
	}
	return out
}

// lookup runs the protected checks' observe on dir for one phrase and tells
// whether its criterion reports the phrase missing.
func lookup(t tb, root, dir string, p phrase) bool {
	req, err := json.Marshal([]map[string]string{{"suite": p.Suite, "criterion": p.Criterion, "phrase": p.Phrase}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	reqPath := filepath.Join(dir, ".skill-slim-lookup.json")
	if err := os.WriteFile(reqPath, req, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cmd := exec.Command("python3", filepath.Join(root, "tools", "skill-slim-checks", "protected_lookup.py"), dir, reqPath)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("setup: protected_lookup.py on %s %s: %v: %s", p.Suite, p.Criterion, err, tail(errb.String()))
	}
	var got []bool
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("setup: protected_lookup.py printed %q: %v", tail(out.String()), err)
	}
	return got[0]
}

// AC-9: every phrase the protected contract checks required in a skill
// document at the base is still required, and found when it sits only in the
// skill's SKILL.md or only in another of its reference documents.
func TestSkillSlimAC9_PhraseLookup(t *testing.T) {
	root := repoRoot(t)
	var base []phrase
	expected(t, "base-protected-phrases.json", &base)
	if len(base) == 0 {
		t.Fatalf("setup: testdata/base-protected-phrases.json lists no phrase")
	}
	var all []variant
	for _, p := range base {
		all = append(all, variants(t, root, p)...)
	}
	const workers = 4
	type result struct {
		v        variant
		reported bool
	}
	results := make([]result, len(all))
	dirs := make([]string, workers)
	for w := range dirs {
		dirs[w] = t.TempDir()
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					failures = append(failures, fmt.Sprint(r))
					mu.Unlock()
				}
			}()
			g := goT{t}
			dir := syntheticTree(g, root, dirs[w])
			pristine := map[string]string{}
			for _, s := range skills {
				for _, doc := range docs(g, dir, s) {
					pristine[doc] = readFile(g, dir, doc)
				}
			}
			for i := w; i < len(all); i += workers {
				for doc, text := range pristine {
					writeFile(g, dir, doc, text)
				}
				all[i].change(g, dir, pristine)
				results[i] = result{all[i], lookup(g, root, dir, all[i].p)}
			}
		}(w)
	}
	wg.Wait()
	if len(failures) > 0 {
		t.Fatalf("setup: %s", strings.Join(failures, "; "))
	}
	for _, r := range results {
		if r.reported != r.v.want {
			verb := "still reports it missing"
			if r.v.want {
				verb = "no longer reports it missing"
			}
			t.Errorf("jaekit:violation=protected-lookup: %s %s %s: with %q %s, the check %s",
				r.v.p.Module, r.v.p.Suite, r.v.p.Criterion, r.v.p.Phrase, r.v.name, verb)
		}
	}
}

// AC-9: the checks of every route whose sources include a skill document or
// a protected contract check pass on the checked tree. The full verification
// under AC-13 runs the checks of every other route.
func TestSkillSlimAC9_RouteChecks(t *testing.T) {
	root := repoRoot(t)
	ran := 0
	for _, r := range routes(t, root) {
		related := false
		for _, src := range r.Sources {
			related = related || skillSource(src) || strings.HasPrefix(src, "tools/contract-checks/")
		}
		if !related {
			continue
		}
		for _, c := range r.Checks {
			ran++
			var cmd *exec.Cmd
			switch c.Group {
			case "go":
				cmd = exec.Command("go", "test", "-count=1", "-run", "^(?:"+c.Selector+")$", "./"+filepath.ToSlash(filepath.Dir(c.File)))
				cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS= ")
			case "docs":
				cmd = exec.Command("python3", "-m", "unittest", "discover", "-s", filepath.Dir(c.File), "-p", filepath.Base(c.File), "-k", c.Selector)
			default:
				t.Fatalf("setup: route %s check %s has unknown group %q", r.ID, c.File, c.Group)
			}
			cmd.Dir = root
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err := cmd.Run()
			text := out.String()
			var exit *exec.ExitError
			switch {
			case err == nil && (c.Group == "go" && strings.Contains(text, "ok ") && !strings.Contains(text, "[no tests to run]") ||
				c.Group == "docs" && regexp.MustCompile(`Ran [1-9]\d* tests?`).MatchString(text)):
			case errors.As(err, &exit) && exit.ExitCode() == 1 &&
				(c.Group == "go" && strings.Contains(text, "--- FAIL:") || c.Group == "docs" && regexp.MustCompile(`FAILED \(failures=\d+\)`).MatchString(text)):
				t.Errorf("jaekit:violation=route-check-fails: route %s check %s %s fails: %s", r.ID, c.File, c.Selector, tail(text))
			default:
				t.Fatalf("setup: route %s check %s %s did not run to a result: %v: %s", r.ID, c.File, c.Selector, err, tail(text))
			}
		}
	}
	if ran == 0 {
		t.Fatalf("setup: no route has a skill document or protected contract check among its sources")
	}
}

// AC-10: every format token of the three base SKILL.md files is still in the
// same skill's SKILL.md or reference documents.
func TestSkillSlimAC10_TokensKept(t *testing.T) {
	root := repoRoot(t)
	var base map[string]struct{ Code, Headings, Links []string }
	expected(t, "base-tokens.json", &base)
	for _, s := range skills {
		now := tokensOf(t, root, s)
		b := base[s.name]
		if len(b.Code)+len(b.Headings) == 0 {
			t.Fatalf("setup: testdata/base-tokens.json lists no token for %s", s.name)
		}
		for _, c := range b.Code {
			if !now.code[c] {
				t.Errorf("%s lost the code span %q", s.dir, c)
			}
		}
		for _, h := range b.Headings {
			if !now.headings[h] {
				t.Errorf("%s lost the heading %q", s.dir, h)
			}
		}
		for _, l := range b.Links {
			if !now.links[l] {
				t.Errorf("%s lost the link target %s", s.dir, l)
			}
		}
	}
}

// versionsOf reads a plugin's version from its Claude Code and Codex
// manifests, its SKILL.md front matter and the Claude Code marketplace list.
func versionsOf(t *testing.T, root, name string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, file := range []string{"plugins/" + name + "/.claude-plugin/plugin.json", "plugins/" + name + "/.codex-plugin/plugin.json"} {
		var m struct{ Version string }
		if err := json.Unmarshal([]byte(readFile(t, root, file)), &m); err != nil {
			t.Fatalf("setup: %s is not a JSON object: %v", file, err)
		}
		out[file] = m.Version
	}
	front := regexp.MustCompile(`(?m)^\s+version: "([^"]*)"$`).FindStringSubmatch(readFile(t, root, skillNamed(name).body()))
	if front == nil {
		t.Fatalf("setup: %s has no front matter version", skillNamed(name).body())
	}
	out[skillNamed(name).body()] = front[1]
	var market struct {
		Plugins []struct{ Name, Version string }
	}
	if err := json.Unmarshal([]byte(readFile(t, root, ".claude-plugin/marketplace.json")), &market); err != nil {
		t.Fatalf("setup: .claude-plugin/marketplace.json: %v", err)
	}
	for _, p := range market.Plugins {
		if p.Name == name {
			out[".claude-plugin/marketplace.json"] = p.Version
		}
	}
	return out
}

func semver(v string) []int {
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	if len(out) != 3 {
		return nil
	}
	return out
}

// AC-11: every place gives each plugin the same version.
func TestSkillSlimAC11_VersionsAgree(t *testing.T) {
	root := repoRoot(t)
	for _, s := range skills {
		got := versionsOf(t, root, s.name)
		seen := map[string]bool{}
		for _, v := range got {
			seen[v] = true
		}
		if len(got) != 4 || len(seen) != 1 {
			t.Errorf("jaekit:violation=versions-disagree: %s versions differ or are missing: %v", s.name, got)
		}
	}
}

// AC-11: each plugin whose skill this goal changes is above its base version.
func TestSkillSlimAC11_VersionsRaised(t *testing.T) {
	root := repoRoot(t)
	var base map[string]string
	expected(t, "base-versions.json", &base)
	for _, s := range skills {
		was := semver(base[s.name])
		now := semver(versionsOf(t, root, s.name)[s.body()])
		if was == nil || now == nil {
			t.Fatalf("setup: unreadable %s version: base %q, now %q", s.name, base[s.name], versionsOf(t, root, s.name)[s.body()])
		}
		raised := now[0] > was[0] || now[0] == was[0] && (now[1] > was[1] || now[1] == was[1] && now[2] > was[2])
		if !raised {
			t.Errorf("jaekit:violation=version-not-raised: %s is %v, not above its base version %s", s.name, now, base[s.name])
		}
	}
}

// AC-12: this tree's ha accepts a start that points at this tree's Seal skill
// and passes --check-skill, on a synthetic goal.
func TestSkillSlimAC12_StartAccepted(t *testing.T) {
	root := repoRoot(t)
	ha := filepath.Join(t.TempDir(), "ha")
	build := exec.Command("go", "build", "-o", ha, "./cmd/ha")
	build.Dir, build.Env = root, append(os.Environ(), "GOWORK=off", "GOFLAGS= ")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("setup: build ha: %v: %s", err, out)
	}
	dir := t.TempDir()
	env := append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	for name, content := range startFixture {
		writeFile(t, dir, name, content)
	}
	run := func(name string, args ...string) (string, int) {
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		switch {
		case err == nil:
			return string(out), 0
		case errors.As(err, &exit) && exit.Exited():
			return string(out), exit.ExitCode()
		}
		t.Fatalf("setup: %s %v did not run to an exit code: %v %s", name, args, err, out)
		return "", 0
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "commit.gpgsign=false", "commit", "-qm", "base"}} {
		if out, code := run("git", args...); code != 0 {
			t.Fatalf("setup: git %v: %s", args, out)
		}
	}
	skillPath := filepath.Join(root, filepath.FromSlash(skillNamed("seal").body()))
	if out, code := run(ha, "start", "docs/specs/demo", "--rules", "run-rules/7", "--request", "synthetic request",
		"--skill", skillPath, "--check-skill"); code != 0 {
		t.Errorf("ha start with --check-skill on this tree's Seal skill exited %d: %s", code, out)
		return
	}
	out, code := run(ha, "status", "docs/specs/demo", "--format", "json")
	if code != 0 && code != 1 {
		t.Fatalf("setup: ha status exited %d: %s", code, out)
	}
	var status struct {
		Skill struct{ Name, Version string } `json:"skill"`
	}
	front := regexp.MustCompile(`(?m)^\s+version: "([^"]*)"$`).FindStringSubmatch(readFile(t, root, skillNamed("seal").body()))
	if err := json.Unmarshal([]byte(out), &status); err != nil || front == nil {
		t.Fatalf("setup: ha status output or skill version unreadable: %v: %s", err, tail(out))
	}
	if status.Skill.Name != "seal" || status.Skill.Version != front[1] {
		t.Errorf("the started goal records skill %s %s, not seal %s", status.Skill.Name, status.Skill.Version, front[1])
	}
}

// startFixture is a synthetic repository with one goal ha can start.
var startFixture = map[string]string{
	"go.mod": "module example.com/demo\n\ngo 1.21\n",
	"greet/greet_test.go": "package greet\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
		"func TestGreeting(t *testing.T) {\n\tb, err := os.ReadFile(\"greeting.txt\")\n" +
		"\tif err != nil || string(b) != \"hello\\n\" {\n\t\tt.Fatalf(\"jaekit:violation=greeting-missing greeting.txt holds %q\", b)\n\t}\n}\n",
	"checks/AC-1.json": `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "greeting", "violation": "greeting-missing", "test": {"package": "example.com/demo/greet", "name": "TestGreeting"}}]}` + "\n",
	"docs/specs/demo/SPEC.md": "# Greeting\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\n\n`greet/greeting.txt` holds `hello`.\n\n" +
		"## Acceptance Criteria\n- **AC-1** `greet/greeting.txt` holds `hello` and a newline.\n\n## Open Decisions\n\n없음\n",
	"docs/specs/demo/PLAN.md": "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
		"| AC-1 | change | `go test -json -count=1 ./greet/` | greet/greet_test.go, checks/AC-1.json | T001 |\n\n" +
		"## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/AC-1.json |\n\n" +
		"## 범위\n- 바꿀 수 있는 경로: `greet/**` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n" +
		"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| T001 | greeting.txt holds hello | AC-1 | — | — | 예 | — |\n\n" +
		"## 예산\n- 검증 실행: 자동\n- 경과 시간: 1시간\n- 비용: 미관측\n\n## 실행 환경\n- git: `git --version`\n",
	"docs/specs/demo/REVIEW.md":   "# REVIEW — demo\n\n## 결론\nready\n",
	"docs/specs/demo/PROGRESS.md": "# PROGRESS — demo\n\n## 현재\nsynthetic\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | |\n",
}
