package plugins

// Property: Skill format tokens
// A skill's rules tests read its format tokens across the skill's SKILL.md
// and references/*.md: code span contents, fenced lines, headings and link
// targets. Rewording prose, or moving it between the skill's documents, keeps
// every token, so it never fails these checks. Rules only the user decides,
// and things the agent must not do, are checked in their SKILL.md body
// section. Each skill's SKILL.md stays within its word cap, and both
// development guides state that cap on the line that links the skill.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	specSkillDir = "spec/skills/spec"
	sparSkillDir = "spar/skills/spar"
	sealSkillDir = "seal/skills/seal"
)

// skillWordCaps are the whitespace-separated word caps of each SKILL.md,
// front matter included.
var skillWordCaps = map[string]int{specSkillDir: 2800, sparSkillDir: 2600, sealSkillDir: 2300}

// skillDoc is one document of a skill, its path relative to plugins/.
type skillDoc struct{ path, text string }

// skillDocs returns a skill's SKILL.md and then its references/*.md in name
// order.
func skillDocs(t *testing.T, dir string) []skillDoc {
	t.Helper()
	out := []skillDoc{{dir + "/SKILL.md", read(t, dir+"/SKILL.md")}}
	refs, err := filepath.Glob(filepath.Join(dir, "references", "*.md"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		p := filepath.ToSlash(ref)
		out = append(out, skillDoc{p, read(t, p)})
	}
	return out
}

var skillLink = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// skillSpans returns the contents of a line's inline code spans, white space
// runs read as one space, and the line with those spans removed.
func skillSpans(line string) (spans []string, outside string) {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		closing := strings.Index(line[i+n:], strings.Repeat("`", n))
		if closing < 0 {
			b.WriteString(line[i : i+n])
			i += n
			continue
		}
		spans = append(spans, skillFlat(line[i+n:i+n+closing]))
		i += n + closing + n
	}
	return spans, b.String()
}

// skillTokenSet holds a skill's format tokens.
type skillTokenSet struct {
	code     []string        // code span contents and fenced lines, white space runs as one space
	headings map[string]bool // heading texts without their markers
	links    map[string]bool // link targets resolved against plugins/
}

func skillFlat(s string) string { return strings.Join(strings.Fields(s), " ") }

// skillTokens reads every format token of a skill's documents.
func skillTokens(t *testing.T, dir string) skillTokenSet {
	t.Helper()
	set := skillTokenSet{headings: map[string]bool{}, links: map[string]bool{}}
	for _, doc := range skillDocs(t, dir) {
		fence := ""
		for i, line := range strings.Split(doc.text, "\n") {
			trimmed := strings.TrimSpace(line)
			switch {
			case i == 0 && trimmed == "---":
				fence = "---"
				continue
			case fence == "---":
				if trimmed == "---" {
					fence = ""
				}
				continue
			case fence != "":
				if strings.HasPrefix(trimmed, fence) {
					fence = ""
				} else {
					set.code = append(set.code, skillFlat(line))
				}
				continue
			case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
				fence = trimmed[:3]
				continue
			case strings.HasPrefix(line, "#"):
				set.headings[strings.TrimSpace(strings.TrimLeft(line, "#"))] = true
			}
			spans, outside := skillSpans(line)
			set.code = append(set.code, spans...)
			for _, m := range skillLink.FindAllStringSubmatch(outside, -1) {
				set.links[skillResolve(doc.path, m[1])] = true
			}
		}
	}
	return set
}

// skillResolve turns a link target written in doc into a path relative to
// plugins/ with its anchor; external links stay as written.
func skillResolve(doc, target string) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return target
	}
	if strings.HasPrefix(target, "#") {
		return doc + target
	}
	path, anchor, _ := strings.Cut(target, "#")
	joined := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(doc), path)))
	if anchor != "" {
		return joined + "#" + anchor
	}
	return joined
}

// has reports whether token is a format token of the set: a link target when
// it starts with "link:" (relative to the skill's directory), whole words
// inside one code span or fenced line, or, when it starts with "#", also a
// heading (its level does not matter).
func (s skillTokenSet) has(dir, token string) bool {
	switch {
	case strings.HasPrefix(token, "#") && s.headings[strings.TrimSpace(strings.TrimLeft(token, "#"))]:
		return true
	case strings.HasPrefix(token, "link:"):
		target := strings.TrimPrefix(token, "link:")
		return s.links[target] || s.links[skillResolve(dir+"/SKILL.md", target)]
	}
	want := skillFlat(token)
	for _, c := range s.code {
		if skillSpanHolds(c, want) {
			return true
		}
	}
	return false
}

// skillWordByte reports whether b continues a word in a code span: an ASCII
// letter or digit, '_', '-', or a byte of a non-ASCII character.
func skillWordByte(b byte) bool {
	return b == '_' || b == '-' || b >= 0x80 ||
		b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// skillSpanHolds reports whether a code span holds token as whole words: at
// an occurrence where the span does not continue a word past either edge of
// the token, so "block" is not found in "review_blocked".
func skillSpanHolds(span, token string) bool {
	if token == "" {
		return false
	}
	for from := 0; from <= len(span); {
		i := strings.Index(span[from:], token)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(token)
		left := i == 0 || !skillWordByte(token[0]) || !skillWordByte(span[i-1])
		right := end == len(span) || !skillWordByte(token[len(token)-1]) || !skillWordByte(span[end])
		if left && right {
			return true
		}
		from = i + 1
	}
	return false
}

// A code span token is found only as whole words of a span.
func TestSkillSpanHoldsWholeWords(t *testing.T) {
	for _, c := range []struct {
		span, token string
		want        bool
	}{
		{"block", "block", true},
		{"record a block", "block", true},
		{"review_blocked", "block", false},
		{"check-change", "check", false},
		{"run-rules/7", "rules", false},
		{"--weakened-invalid", "--weakened", false},
		{"run-rules/70", "run-rules/7", false},
		{"ha start --rules run-rules/7 --check-skill", "--rules run-rules/7", true},
		{"budget.post_completion_seconds", "post_completion_seconds", true},
		{"docs/specs/<goal>/SPEC.md", "docs/specs/<goal>", true},
		{"ha note <goal> review --gate <file>", "ha note <goal> review --gate", true},
		{"## 개선 메모", "개선 메모", true},
		{"개선메모", "개선", false},
		{"blockblock block", "block", true},
		{"", "block", false},
	} {
		if got := skillSpanHolds(c.span, c.token); got != c.want {
			t.Errorf("skillSpanHolds(%q, %q) = %v, want %v", c.span, c.token, got, c.want)
		}
	}
}

// requireSkillTokens reports each token that no document of the skill holds
// as a format token.
func requireSkillTokens(t *testing.T, dir string, tokens ...string) {
	t.Helper()
	set := skillTokens(t, dir)
	for _, tok := range tokens {
		if !set.has(dir, tok) {
			t.Errorf("%s lacks the format token %q", dir, tok)
		}
	}
}

// skillBodySection returns the lines of one section of a skill's SKILL.md,
// from heading to the next heading of the same or a higher level, outside
// fenced code; ok is false when the heading is absent.
func skillBodySection(t *testing.T, dir, heading string) ([]string, bool) {
	t.Helper()
	level := len(heading) - len(strings.TrimLeft(heading, "#"))
	var out []string
	in, fenced := false, false
	for _, line := range strings.Split(read(t, dir+"/SKILL.md"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "#") {
			depth := len(line) - len(strings.TrimLeft(line, "#"))
			if in && depth <= level {
				break
			}
			if trimmed == heading {
				in = true
				continue
			}
		}
		if in {
			out = append(out, line)
		}
	}
	return out, in
}

// requireBodyTokens reports each token that no code span of the named
// SKILL.md body section holds. It checks the rules that must stay in the
// body: rules only the user decides and things the agent must not do.
func requireBodyTokens(t *testing.T, dir, heading string, tokens ...string) {
	t.Helper()
	lines, ok := skillBodySection(t, dir, heading)
	if !ok {
		t.Errorf("%s/SKILL.md has no body section %q", dir, heading)
		return
	}
	var spans []string
	for _, line := range lines {
		s, _ := skillSpans(line)
		spans = append(spans, s...)
	}
	for _, tok := range tokens {
		found := false
		for _, s := range spans {
			found = found || skillSpanHolds(s, skillFlat(tok))
		}
		if !found {
			t.Errorf("%s/SKILL.md %s lacks the rule token %q: this rule stays in the body", dir, heading, tok)
		}
	}
}

// skillWords counts a SKILL.md as the caps count it: whitespace-separated
// words of the whole file, front matter included.
func skillWords(t *testing.T, dir string) int {
	t.Helper()
	return len(strings.Fields(read(t, dir+"/SKILL.md")))
}

// TestSkillWordCaps keeps each SKILL.md within its cap.
func TestSkillWordCaps(t *testing.T) {
	for _, dir := range []string{specSkillDir, sparSkillDir, sealSkillDir} {
		if n, limit := skillWords(t, dir), skillWordCaps[dir]; n > limit {
			t.Errorf("%s/SKILL.md has %d words, above its cap %s", dir, n, skillCapText(limit))
		}
	}
}

func skillCapText(n int) string { return fmt.Sprintf("%d,%03d", n/1000, n%1000) }

// TestSkillWordCapsStated checks that both development guides state each
// skill's cap on the line that links its SKILL.md.
func TestSkillWordCapsStated(t *testing.T) {
	for _, guide := range []string{"../guides/DEVELOPMENT.md", "../guides/DEVELOPMENT.en.md"} {
		lines := strings.Split(read(t, guide), "\n")
		for _, dir := range []string{specSkillDir, sparSkillDir, sealSkillDir} {
			link, stated := "(../plugins/"+dir+"/SKILL.md)", skillCapText(skillWordCaps[dir])
			found := false
			for _, line := range lines {
				found = found || strings.Contains(line, link) && strings.Contains(line, stated)
			}
			if !found {
				t.Errorf("%s has no line that links %s/SKILL.md and states its cap %s", guide, dir, stated)
			}
		}
	}
}

// TestSkillLinksResolve checks every relative link of the three skills'
// documents: the path exists and an anchor names a heading of its file.
func TestSkillLinksResolve(t *testing.T) {
	for _, dir := range []string{specSkillDir, sparSkillDir, sealSkillDir} {
		for _, doc := range skillDocs(t, dir) {
			fenced := false
			for i, line := range strings.Split(doc.text, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
					fenced = !fenced
				}
				if fenced {
					continue
				}
				_, outside := skillSpans(line)
				for _, m := range skillLink.FindAllStringSubmatch(outside, -1) {
					target := skillResolve(doc.path, m[1])
					if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
						continue
					}
					path, anchor, _ := strings.Cut(target, "#")
					if _, err := os.Stat(path); err != nil {
						t.Errorf("%s:%d links %s, which does not exist", doc.path, i+1, m[1])
						continue
					}
					if anchor != "" && !skillAnchors(read(t, path))[anchor] {
						t.Errorf("%s:%d links %s, whose heading does not exist", doc.path, i+1, m[1])
					}
				}
			}
		}
	}
}

// Every reference document of a skill is the target of a link in its
// SKILL.md, so a rule outside the body is always reachable from it.
func TestSkillReferencesLinkedFromBody(t *testing.T) {
	for _, dir := range []string{specSkillDir, sparSkillDir, sealSkillDir} {
		docs := skillDocs(t, dir)
		linked := map[string]bool{}
		fenced := false
		for _, line := range strings.Split(docs[0].text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				fenced = !fenced
			}
			if fenced {
				continue
			}
			_, outside := skillSpans(line)
			for _, m := range skillLink.FindAllStringSubmatch(outside, -1) {
				path, _, _ := strings.Cut(skillResolve(docs[0].path, m[1]), "#")
				linked[path] = true
			}
		}
		for _, doc := range docs[1:] {
			if !linked[doc.path] {
				t.Errorf("%s does not link %s", docs[0].path, doc.path)
			}
		}
	}
}

// skillAnchors returns GitHub's heading anchors of a Markdown document.
func skillAnchors(text string) map[string]bool {
	out, seen := map[string]bool{}, map[string]int{}
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		var b strings.Builder
		for _, r := range strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#"))) {
			switch {
			case r == ' ':
				b.WriteRune('-')
			case r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 0xAC00 && r <= 0xD7A3:
				b.WriteRune(r)
			}
		}
		s := b.String()
		if n := seen[s]; n > 0 {
			out[fmt.Sprintf("%s-%d", s, n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

// TestSkillAuthorityRulesInBody checks the representative rules that only
// the user decides, and things the agent must not do: each stays in its
// SKILL.md body section, marked by a code span token, so moving it to a
// reference document or deleting it fails here.
func TestSkillAuthorityRulesInBody(t *testing.T) {
	for _, c := range []struct {
		dir     string
		section string
		tokens  []string
	}{
		{specSkillDir, "## Agent invocations", []string{"Status: Ready", "amendments/<name>.md"}},
		{sealSkillDir, "## Authority", []string{"ha start --request", "Status: Ready", "SPEC.md"}},
		{sealSkillDir, "## Independent review", []string{"hardening", "spar extend"}},
		{sealSkillDir, "## Budget", []string{"ha budget"}},
		{sealSkillDir, "## Forbidden", []string{"SPEC.md", "runs.jsonl", "block", "ha log", "T001"}},
		{sparSkillDir, "## When to run each stage", []string{"spar extend"}},
		{sparSkillDir, "## Reviewers", []string{"spar waive"}},
		{sparSkillDir, "## Classes and dispositions", []string{"answer"}},
	} {
		t.Run(c.dir+" "+c.section, func(t *testing.T) {
			requireBodyTokens(t, c.dir, c.section, c.tokens...)
		})
	}
}
