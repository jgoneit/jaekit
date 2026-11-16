//go:build skillslim

package skillslimchecks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
					if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
						t.Errorf("jaekit:violation=moved-unlinked: %s:%d links %s, which does not exist", doc, i+1, target)
						continue
					} else if err != nil {
						t.Fatalf("setup: %s:%d links %s, which cannot be read: %v", doc, i+1, target, err)
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

// refSection is one level-2 or level-3 section of a reference document: its
// heading, the anchors that reach it (its own and its enclosing level-2
// heading's) and a digest of its text with white space runs as one space.
type refSection struct {
	doc, heading string
	level        int
	anchors      []string
	// preamble marks the prose before a document's first level-2 heading;
	// only a link to the document itself, or to its title, reaches it.
	preamble bool
	sha256   string
}

// fieldsDigest is the SHA-256 of lines with white space runs as one space.
func fieldsDigest(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(strings.Join(lines, " ")), " ")))
	return hex.EncodeToString(sum[:])
}

// refSections reads the sections of a skill's reference documents in the
// tree at root: every level-2 and level-3 section; every level-1 heading
// after the first level-2 heading with the text directly under it; and the
// preamble, the non-heading text before the first level-2 heading. A
// level-1 part or preamble with no text holds no rule and is left out.
func refSections(t tb, root string, s skill) []refSection {
	var out []refSection
	for _, doc := range docs(t, root, s)[1:] {
		text := readFile(t, root, doc)
		lines := strings.Split(text, "\n")
		kinds := classify(text)
		anchor := map[int]string{}
		seen := map[string]int{}
		firstL2 := len(lines)
		for i, kind := range kinds {
			if kind != kindHead {
				continue
			}
			a := slug(headingText(lines[i]))
			if n := seen[a]; n > 0 {
				seen[a]++
				a = fmt.Sprintf("%s-%d", a, n)
			} else {
				seen[a]++
			}
			anchor[i] = a
			if headingLevel(lines[i]) == 2 && firstL2 == len(lines) {
				firstL2 = i
			}
		}
		end := func(i, lv int) int {
			for j := i + 1; j < len(lines); j++ {
				if kinds[j] == kindHead && headingLevel(lines[j]) <= lv {
					return j
				}
			}
			return len(lines)
		}
		parent := ""
		for i, kind := range kinds {
			if kind != kindHead {
				continue
			}
			lv := headingLevel(lines[i])
			if lv == 2 {
				parent = anchor[i]
			}
			switch {
			case lv == 2 || lv == 3:
				anchors := []string{anchor[i]}
				if lv == 3 && parent != "" {
					anchors = append(anchors, parent)
				}
				out = append(out, refSection{doc: doc, heading: headingText(lines[i]), level: lv, anchors: anchors, sha256: fieldsDigest(lines[i+1 : end(i, lv)])})
			case lv == 1 && i > firstL2:
				parent = ""
				body := lines[i+1 : end(i, 2)]
				if strings.TrimSpace(strings.Join(body, "")) != "" {
					out = append(out, refSection{doc: doc, heading: headingText(lines[i]), level: 1, anchors: []string{anchor[i]}, sha256: fieldsDigest(body)})
				}
			}
		}
		var pre []string
		var title []string
		for i := 0; i < firstL2; i++ {
			if kinds[i] == kindHead {
				if headingLevel(lines[i]) == 1 && len(title) == 0 {
					title = []string{anchor[i]}
				}
				continue
			}
			pre = append(pre, lines[i])
		}
		if strings.TrimSpace(strings.Join(pre, "")) != "" {
			out = append(out, refSection{doc: doc, level: 1, anchors: title, preamble: true, sha256: fieldsDigest(pre)})
		}
	}
	return out
}

// AC-6: every reference section that holds moved rules has a line in its
// SKILL.md that links it, or its enclosing level-2 section, and says when to
// read it. Such sections are the representatives, every level-2 or level-3
// section that is new since the base, and every base section whose text
// changed; a level-1 heading after the first level-2 heading, with its text,
// counts as a section reached by its own anchor, and a document's preamble
// counts as one reached by a link to the document itself or its title.
func TestSkillSlimAC6_MovedLinked(t *testing.T) {
	root := repoRoot(t)
	var base map[string][]struct {
		Doc, Heading, Sha256 string
		Level                int
	}
	expected(t, "base-reference-sections.json", &base)
	for _, s := range skills {
		was := map[[2]string]string{}
		for _, row := range base[s.name] {
			was[[2]string{row.Doc, row.Heading}] = row.Sha256
		}
		now := refSections(t, root, s)
		var want []refSection
		for _, sec := range now {
			if sum, ok := was[[2]string{sec.doc, sec.heading}]; !ok || sum != sec.sha256 {
				want = append(want, sec)
			}
		}
		for _, m := range representatives {
			if m.skill != s.name {
				continue
			}
			doc := s.dir + "/" + m.doc
			found := false
			for _, sec := range now {
				if sec.doc == doc && sec.heading == m.heading {
					found = true
					want = append(want, sec)
				}
			}
			if !found {
				t.Errorf("jaekit:violation=moved-unlinked: %s has no section %q", doc, m.heading)
			}
		}
		body := readFile(t, root, s.body())
		lines := strings.Split(body, "\n")
		kinds := classify(body)
		reported := map[[2]string]bool{}
		for _, sec := range want {
			if reported[[2]string{sec.doc, sec.heading}] {
				continue
			}
			reported[[2]string{sec.doc, sec.heading}] = true
			found := false
			for i, kind := range kinds {
				if kind != kindProse || !whenToRead.MatchString(anyLinks.ReplaceAllString(masked(lines[i]), "]")) {
					continue
				}
				for _, target := range linkTargets(lines[i]) {
					full := resolve(s.body(), target)
					if sec.preamble {
						found = found || full == sec.doc
					}
					for _, a := range sec.anchors {
						found = found || full == sec.doc+"#"+a
					}
				}
			}
			if !found && sec.preamble {
				t.Errorf("jaekit:violation=moved-unlinked: %s has no line that links %s itself and says when to read it (its text before the first level-2 heading is new or changed)",
					s.body(), strings.TrimPrefix(sec.doc, s.dir+"/"))
			} else if !found {
				t.Errorf("jaekit:violation=moved-unlinked: %s has no line that links %s#%s and says when to read it",
					s.body(), strings.TrimPrefix(sec.doc, s.dir+"/"), sec.anchors[0])
			}
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
	{"seal", "## Forbidden", "T001"},
	{"spar", "## When to run each stage", "spar extend"},
	{"spar", "## Reviewers", "spar waive"},
	{"spar", "## Classes and dispositions", "answer"},
}

// authorityBlocks returns the first line of every block (list item or
// paragraph) of the rule's body section that has a code span holding its
// token, in order; ok is false when the section is absent.
func authorityBlocks(t tb, root string, a authority) (starts []int, ok bool) {
	text := readFile(t, root, skillNamed(a.skill).body())
	start, end, ok := section(text, a.section)
	if !ok {
		return nil, false
	}
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	seen := map[int]bool{}
	for i := start + 1; i < end; i++ {
		if kinds[i] != kindProse {
			continue
		}
		for _, r := range codeRanges(lines[i]) {
			if strings.Contains(codeText(lines[i], r), a.token) {
				b, _ := block(text, i)
				if !seen[b] {
					seen[b] = true
					starts = append(starts, b)
				}
			}
		}
	}
	return starts, true
}

// authorityBlock returns the one block that holds the rule's token, or
// reports a violation and -1: a rule whose token sits in no block, or in
// more than one, cannot be told apart from its section by a test.
func authorityBlock(t *testing.T, root string, a authority) int {
	t.Helper()
	starts, ok := authorityBlocks(t, root, a)
	switch {
	case !ok:
		t.Errorf("jaekit:violation=authority-moved: %s has no section %s", skillNamed(a.skill).body(), a.section)
	case len(starts) == 0:
		t.Errorf("jaekit:violation=authority-moved: %s %s holds no code span with %q", skillNamed(a.skill).body(), a.section, a.token)
	case len(starts) > 1:
		t.Errorf("jaekit:violation=authority-moved: %s %s holds %q in %d blocks, so moving the rule leaves it behind",
			skillNamed(a.skill).body(), a.section, a.token, len(starts))
	default:
		return starts[0]
	}
	return -1
}

// AC-7: each representative rule is in its body section, in one block.
func TestSkillSlimAC7_InBody(t *testing.T) {
	root := repoRoot(t)
	for _, a := range authorities {
		authorityBlock(t, root, a)
	}
}

// firstReference is the skill's first reference document, or a new
// references/moved.md when the skill has none.
func firstReference(t tb, dir string, s skill) string {
	if refs := docs(t, dir, s)[1:]; len(refs) > 0 {
		return refs[0]
	}
	moved := s.dir + "/references/moved.md"
	writeFile(t, dir, moved, "# Moved\n")
	return moved
}

// deleteBlock removes lines [start, end) of doc.
func deleteBlock(t tb, root, doc string, start, end int) {
	lines := strings.Split(readFile(t, root, doc), "\n")
	writeFile(t, root, doc, strings.Join(append(append([]string{}, lines[:start]...), lines[end:]...), "\n"))
}

// AC-7: a copy whose rule block moved from its body section to the skill's
// first reference document, and a copy whose rule block is deleted, each fail
// a plugins test that names the rule's token.
func TestSkillSlimAC7_MovedOrDeletedFails(t *testing.T) {
	root := repoRoot(t)
	var trees []*edited
	var rules []authority
	for _, a := range authorities {
		a := a
		i := authorityBlock(t, root, a)
		if i < 0 {
			continue
		}
		s := skillNamed(a.skill)
		trees = append(trees,
			&edited{what: fmt.Sprintf("moves the %s rule with %q to its first reference document", s.name, a.token),
				change: func(t tb, dir string) {
					start, end := block(readFile(t, dir, s.body()), i)
					moveBlock(t, dir, s.body(), start, end, firstReference(t, dir, s))
				}},
			&edited{what: fmt.Sprintf("deletes the %s rule with %q", s.name, a.token),
				change: func(t tb, dir string) {
					start, end := block(readFile(t, dir, s.body()), i)
					deleteBlock(t, dir, s.body(), start, end)
				}})
		rules = append(rules, a, a)
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

// baseExempt is what AC-8 excepts, extracted once from the goal's base: the
// plugins tests defined in the files that regression routes with a skill
// source check, the TestCompatibilityAC10_ and AC11_ tests, and the phrases
// the protected checks require in the skill documents (the protected contract
// checks' anchors, the strings their tests mutate, and the AC10 and AC11
// tests' strings).
type baseExempt struct {
	Tests   []string `json:"tests"`
	Phrases []string `json:"phrases"`
}

func readExempt(t tb) baseExempt {
	var e baseExempt
	expected(t, "base-exempt.json", &e)
	if len(e.Tests) == 0 || len(e.Phrases) == 0 {
		t.Fatalf("setup: testdata/base-exempt.json lists no test or no phrase")
	}
	return e
}

// exempt reports whether a top-level plugins test is one AC-8 excepts.
func exempt(t tb) func(string) bool {
	names := map[string]bool{}
	for _, n := range readExempt(t).Tests {
		names[n] = true
	}
	return func(name string) bool { return names[name] }
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

// keptPhrases are the phrases AC-8 excepts, longest first.
func keptPhrases(t tb) []string {
	out := append([]string{}, readExempt(t).Phrases...)
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
	kept := keptPhrases(t)
	tree := &edited{what: "rewords every prose line of the three skills", change: func(t tb, dir string) {
		if rewordAll(t, dir, kept) == 0 {
			t.Fatalf("setup: the copy has no prose line to reword")
		}
	}}
	runEdited(t, []*edited{tree})
	if failed := tree.run.failedOutside(exempt(t)); len(failed) > 0 {
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

// moveFirstBlock moves the first prose block of a SKILL.md section to the
// skill's first reference document. The block must hold no AC-7 token.
func moveFirstBlock(t tb, dir string, s skill, heading string) {
	text := readFile(t, dir, s.body())
	start, end, ok := section(text, heading)
	if !ok {
		t.Fatalf("setup: %s has no section %s", s.body(), heading)
	}
	lines := strings.Split(text, "\n")
	kinds := classify(text)
	for i := start + 1; i < end; i++ {
		if kinds[i] != kindProse {
			continue
		}
		b, e := block(text, i)
		for _, l := range lines[b:e] {
			for _, a := range authorities {
				if a.skill == s.name && strings.Contains(l, a.token) {
					t.Fatalf("setup: the first block of %s %s holds the AC-7 token %q", s.body(), heading, a.token)
				}
			}
		}
		moveBlock(t, dir, s.body(), b, e, firstReference(t, dir, s))
		return
	}
	t.Fatalf("setup: %s %s has no prose block", s.body(), heading)
}

// AC-8: the representative edits AC-8 names each leave every plugins test
// AC-8 does not except passing: a reworded sentence in Spec Questions, Seal
// Resuming and Spar Classes and dispositions, and one rule block of each
// body moved to its first reference document (Spec Outcome boundaries, a
// rule for rare goals; Seal Resuming; Spar Limits).
func TestSkillSlimAC8_Representatives(t *testing.T) {
	kept := keptPhrases(t)
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
		{what: "moves the first block of Spec Outcome boundaries to its reference", change: func(t tb, dir string) {
			moveFirstBlock(t, dir, skillNamed("spec"), "## Outcome boundaries")
		}},
		{what: "moves the first block of Seal Resuming to its reference", change: func(t tb, dir string) {
			moveFirstBlock(t, dir, skillNamed("seal"), "## Resuming")
		}},
		{what: "moves the first block of Spar Limits to its reference", change: func(t tb, dir string) {
			moveFirstBlock(t, dir, skillNamed("spar"), "## Limits")
		}},
	}
	runEdited(t, trees)
	ex := exempt(t)
	for _, tree := range trees {
		if failed := tree.run.failedOutside(ex); len(failed) > 0 {
			t.Errorf("jaekit:violation=prose-pinned: the plugins tests fail on a copy that %s: %s", tree.what, strings.Join(failed, ", "))
		}
	}
}

// AC-9: every base route keeps its invariant, status, property and boundary,
// and every source and check it listed; a route may add sources and checks.
func TestSkillSlimAC9_RouteProperties(t *testing.T) {
	root := repoRoot(t)
	var base map[string]struct {
		Invariant, Status, Property, Boundary string
		Sources                               []string
		Checks                                []struct{ File, Selector, Group string }
	}
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
		} else {
			sources := map[string]bool{}
			for _, s := range r.Sources {
				sources[s] = true
			}
			for _, s := range b.Sources {
				if !sources[s] {
					t.Errorf("jaekit:violation=route-changed: route %s no longer lists the source %s", id, s)
				}
			}
			checks := map[[3]string]bool{}
			for _, c := range r.Checks {
				checks[[3]string{c.File, c.Selector, c.Group}] = true
			}
			for _, c := range b.Checks {
				if !checks[[3]string{c.File, c.Selector, c.Group}] {
					t.Errorf("jaekit:violation=route-changed: route %s no longer lists the check %s %s", id, c.File, c.Selector)
				}
			}
		}
	}
}

// phrase is one phrase a protected contract check required in a skill
// document at the goal's base.
type phrase struct {
	Module, Suite, Criterion, File, Section, Row, Phrase string
	// Message is set for an inline clause, which its check tests inside
	// observe() and reports with this fixed message naming no phrase.
	Message string
	// Removed and OtherSection are how the base check treated the phrase's
	// removal states: "reported" or "error" (it raised).
	Removed      string `json:"removed"`
	OtherSection string `json:"other_section"`
	// EarlierGoals is how the base check treated the phrase placed only in
	// the Seal reference written for earlier goals.
	EarlierGoals string `json:"earlier_goals"`
}

// earlierGoalsOnly is the Seal reference written only for goals an earlier
// Seal started: a protected phrase of another document is not found there.
const earlierGoalsOnly = "plugins/seal/skills/seal/references/earlier-goals.md"

// variant is one copy state for a phrase and whether its criterion should
// report the phrase missing there.
type variant struct {
	p      phrase
	name   string
	want   bool   // reported missing
	base   string // for a removal state, how the base check treated it
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

// otherSection returns the first heading line of text, at the level of
// heading, whose text differs from it.
func otherSection(t tb, text, heading string) string {
	level := headingLevel(heading)
	lines := strings.Split(text, "\n")
	for i, kind := range classify(text) {
		if kind == kindHead && headingLevel(lines[i]) == level && headingText(lines[i]) != headingText(heading) {
			return strings.TrimSpace(lines[i])
		}
	}
	t.Fatalf("setup: no other level-%d section than %q to place a phrase in", level, heading)
	return ""
}

// variants builds the copy states of a phrase. A phrase of a skill document
// has three: removed from every document of its skill (reported missing), and
// moved only into the skill's SKILL.md or into a reference document other
// than its own file (not reported); a row phrase moves with its table row. A
// phrase scoped to a section has a fourth: moved only into another section of
// its file (reported missing). A Seal phrase of another document than the
// reference written for earlier goals has one more: moved only into that
// reference (reported missing), and the "other reference" state never uses it. A phrase of any other file is removed from
// that file, and a section phrase there is also moved only into another
// section of it; both are reported missing.
func variants(t tb, root string, p phrase) []variant {
	if !skillSource(p.File) {
		out := []variant{{p: p, name: "removed from " + p.File, want: true, base: p.Removed,
			change: func(t tb, dir string, pristine map[string]string) {
				writeFile(t, dir, p.File, removePhrase(pristine[p.File], p.Phrase, p.Section))
			}}}
		if p.Section != "" {
			other := otherSection(t, readFile(t, root, p.File), p.Section)
			out = append(out, variant{p: p, name: "only in " + p.File + " " + other, want: true, base: p.OtherSection,
				change: func(t tb, dir string, pristine map[string]string) {
					text, add := pristine[p.File], []string{p.Phrase}
					if p.Row != "" {
						lines := strings.Split(text, "\n")
						for i, l := range lines {
							if strings.HasPrefix(l, "| "+p.Row+" |") && strings.Contains(l, p.Phrase) {
								add = []string{l}
								lines = append(lines[:i], lines[i+1:]...)
								break
							}
						}
						if add[0] == p.Phrase {
							t.Fatalf("setup: %s has no row %q holding %q", p.File, p.Row, p.Phrase)
						}
						text = strings.Join(lines, "\n")
					}
					writeFile(t, dir, p.File, insertIn(removePhrase(text, p.Phrase, p.Section), other, add))
				}})
		}
		return out
	}
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
		if ref != p.File && ref != earlierGoalsOnly {
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
	out := []variant{{p: p, name: "removed from every document of " + s.name, want: true, base: p.Removed,
		change: func(t tb, dir string, pristine map[string]string) {
			for _, doc := range all {
				writeFile(t, dir, doc, removePhrase(pristine[doc], p.Phrase, p.Section))
			}
		}}}
	if p.Section != "" {
		other := otherSection(t, readFile(t, root, p.File), p.Section)
		out = append(out, variant{p: p, name: "only in " + p.File + " " + other, want: true, base: p.OtherSection,
			change: func(t tb, dir string, pristine map[string]string) {
				row := strip(dir, pristine)
				add := []string{p.Phrase}
				if p.Row != "" {
					if row == "" {
						t.Fatalf("setup: %s has no row %q holding %q", p.File, p.Row, p.Phrase)
					}
					add = []string{row}
				}
				writeFile(t, dir, p.File, insertIn(readFile(t, dir, p.File), other, add))
			}})
	}
	for _, ref := range all[1:] {
		if ref == earlierGoalsOnly && p.File != earlierGoalsOnly {
			out = append(out, variant{p: p, name: "only in " + ref, want: true, base: p.EarlierGoals,
				change: func(t tb, dir string, pristine map[string]string) {
					row := strip(dir, pristine)
					add := []string{p.Phrase}
					if p.Row != "" {
						if row == "" {
							t.Fatalf("setup: %s has no row %q holding %q", p.File, p.Row, p.Phrase)
						}
						add = []string{row}
					}
					writeFile(t, dir, earlierGoalsOnly, insertIn(readFile(t, dir, earlierGoalsOnly), p.Section, add))
				}})
		}
	}
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

// lookupResult is how one clause treats one phrase on a tree: reported
// missing, or its criterion's check raised and reached no result.
type lookupResult struct{ reported, errored bool }

// lookup runs the protected checks' observe on dir for each phrase and tells
// how its clause treats it.
func lookup(t tb, root, dir string, ps []phrase) []lookupResult {
	var reqs []map[string]string
	for _, p := range ps {
		reqs = append(reqs, map[string]string{"suite": p.Suite, "criterion": p.Criterion, "file": p.File,
			"section": p.Section, "row": p.Row, "phrase": p.Phrase, "message": p.Message})
	}
	req, err := json.Marshal(reqs)
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
		t.Fatalf("setup: protected_lookup.py: %v: %s", err, tail(errb.String()))
	}
	var got []any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got) != len(ps) {
		t.Fatalf("setup: protected_lookup.py printed %q: %v", tail(out.String()), err)
	}
	res := make([]lookupResult, len(got))
	for i, g := range got {
		switch g {
		case true:
			res[i].reported = true
		case false:
		case "error":
			res[i].errored = true
		default:
			t.Fatalf("setup: protected_lookup.py printed %v for %q", g, ps[i].Phrase)
		}
	}
	return res
}

// AC-9: every phrase the protected contract checks required at the base is
// still required, and a phrase of a skill document is found when it sits only
// in the skill's SKILL.md or only in another of its reference documents.
func TestSkillSlimAC9_PhraseLookup(t *testing.T) {
	root := repoRoot(t)
	var base []phrase
	expected(t, "base-protected-phrases.json", &base)
	if len(base) == 0 {
		t.Fatalf("setup: testdata/base-protected-phrases.json lists no phrase")
	}
	// Control: on the unchanged copy every criterion's check reaches a
	// result, so a check that raises on a copy state raised because of it.
	control := syntheticTree(t, root, t.TempDir())
	for i, r := range lookup(t, root, control, base) {
		if r.errored {
			t.Fatalf("setup: %s %s raises on the unchanged copy", base[i].Suite, base[i].Criterion)
		}
	}
	var all []variant
	for _, p := range base {
		all = append(all, variants(t, root, p)...)
	}
	const workers = 4
	type result struct {
		v                 variant
		reported, errored bool
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
			for _, p := range base {
				if _, ok := pristine[p.File]; !ok {
					pristine[p.File] = readFile(g, dir, p.File)
				}
			}
			for i := w; i < len(all); i += workers {
				for doc, text := range pristine {
					writeFile(g, dir, doc, text)
				}
				all[i].change(g, dir, pristine)
				r := lookup(g, root, dir, []phrase{all[i].p})[0]
				results[i] = result{all[i], r.reported, r.errored}
			}
		}(w)
	}
	wg.Wait()
	if len(failures) > 0 {
		t.Fatalf("setup: %s", strings.Join(failures, "; "))
	}
	for _, v := range all {
		if v.want && v.base != "reported" && v.base != "error" {
			t.Fatalf("setup: testdata/base-protected-phrases.json has no base outcome for %q %s", v.p.Phrase, v.name)
		}
	}
	counts := map[string]int{}
	for _, r := range results {
		counts[fmt.Sprintf("want reported %v, reported %v, raised %v", r.v.want, r.reported, r.errored)]++
	}
	t.Logf("%d phrases, %d copy states: %v", len(base), len(results), counts)
	// A removal state must be reported as the base check reported it; a
	// raise is accepted only where the base check raised too (it could not
	// read the document without that anchor), so a lookup that starts
	// raising instead of reporting fails here. A moved state must be
	// accepted, without a report or a raise.
	for _, r := range results {
		var verb string
		switch {
		case r.v.want && r.errored && r.v.base != "error":
			verb = "raises where the base check reported it missing"
		case r.v.want && !r.reported && !r.errored:
			verb = "no longer reports it missing"
		case !r.v.want && r.errored:
			verb = "raises"
		case !r.v.want && r.reported:
			verb = "still reports it missing"
		default:
			continue
		}
		t.Errorf("jaekit:violation=protected-lookup: %s %s %s: with %q %s, the check %s",
			r.v.p.Module, r.v.p.Suite, r.v.p.Criterion, r.v.p.Phrase, r.v.name, verb)
	}
}

// probeTools fails as setup when a tool the route checks start cannot start,
// so that a missing tool never reaches a route test as its failure.
func probeTools(t *testing.T, root string) {
	t.Helper()
	for _, argv := range [][]string{{"go", "version"}, {"git", "--version"}, {"python3", "--version"}} {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup: %s cannot start: %v: %s", strings.Join(argv, " "), err, tail(string(out)))
		}
	}
}

// goRouteResult reads go test -json output: the tests that passed, failed
// and were skipped.
func goRouteResult(out []byte) (passed, failed, skipped []string, ok bool) {
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev struct{ Action, Test string }
		if json.Unmarshal(line, &ev) != nil {
			return nil, nil, nil, false
		}
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass":
			passed = append(passed, ev.Test)
		case "fail":
			failed = append(failed, ev.Test)
		case "skip":
			skipped = append(skipped, ev.Test)
		}
	}
	return passed, failed, skipped, true
}

var (
	unittestRan      = regexp.MustCompile(`Ran [1-9]\d* tests?`)
	unittestFailures = regexp.MustCompile(`FAILED \([^)]*\bfailures=[1-9]\d*`)
)

// AC-9: the checks of every route whose sources include a skill document or
// a protected contract check pass on the checked tree. A failing test is the
// violation; a check that does not start, does not build, runs no test or
// skips a test reaches no result. The full verification under AC-13 runs the
// checks of every other route.
func TestSkillSlimAC9_RouteChecks(t *testing.T) {
	root := repoRoot(t)
	probeTools(t, root)
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
				cmd = exec.Command("go", "test", "-json", "-count=1", "-run", "^(?:"+c.Selector+")$", "./"+filepath.ToSlash(filepath.Dir(c.File)))
				cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS= ")
			case "docs":
				cmd = exec.Command("python3", "-m", "unittest", "discover", "-s", filepath.Dir(c.File), "-p", filepath.Base(c.File), "-k", c.Selector)
			default:
				t.Fatalf("setup: route %s check %s has unknown group %q", r.ID, c.File, c.Group)
			}
			cmd.Dir = root
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			var exit *exec.ExitError
			exited1 := errors.As(err, &exit) && exit.ExitCode() == 1
			name := fmt.Sprintf("route %s check %s %s", r.ID, c.File, c.Selector)
			if c.Group == "go" {
				passed, failed, skipped, ok := goRouteResult(stdout.Bytes())
				switch {
				case !ok:
					t.Fatalf("setup: %s printed no test events: %v: %s", name, err, tail(stdout.String()+stderr.String()))
				case exited1 && len(failed) > 0:
					t.Errorf("jaekit:violation=route-check-fails: %s fails: %s", name, strings.Join(failed, ", "))
				case err == nil && len(passed) > 0 && len(skipped) == 0:
				default:
					t.Fatalf("setup: %s reached no result (passed %d, skipped %v): %v: %s", name, len(passed), skipped, err, tail(stderr.String()))
				}
				continue
			}
			text := stdout.String() + stderr.String()
			switch {
			case exited1 && unittestFailures.MatchString(text):
				t.Errorf("jaekit:violation=route-check-fails: %s fails: %s", name, tail(text))
			case err == nil && unittestRan.MatchString(text) && !strings.Contains(text, "skipped="):
			default:
				t.Fatalf("setup: %s reached no result: %v: %s", name, err, tail(text))
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
