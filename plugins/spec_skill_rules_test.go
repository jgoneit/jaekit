package plugins

// Property: Spec skill rules
// Checks that the decision markers and the goal location stay format tokens of
// the Spec skill and that its documents name no local retention
// infrastructure, and checks the decision lines and header lines of its SPEC
// templates.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const (
	spkDecisionHead = "## Decisions and reasons"
	spkMarkerSlot   = "<[사용자], [제안] or [기존]>"
	spkSourceLabel  = "Source:"
	spkStatusKey    = "Status:"
	spkFormatKey    = "Criteria-Format:"
	spkAssuranceKey = "Assurance:"
)

var (
	spkMarkers      = []string{"[사용자]", "[제안]", "[기존]"}
	spkDecisionItem = regexp.MustCompile(`^\s*(?:[-+*]|[0-9]+[.)])\s+`)
	spkPlaceholder  = regexp.MustCompile(`<[^<>\n]+>`)
	spkTemplates    = []string{
		"spec/skills/spec/assets/templates/backend-feature.md",
		"spec/skills/spec/assets/templates/cross-module-feature.md",
		"spec/skills/spec/assets/templates/product.md",
	}
)

func spkLevel(line string) int {
	n := len(line) - len(strings.TrimLeft(line, "#"))
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// spkFence tracks ``` and ~~~ fences; fenced examples never supply a
// document's structural heading.
func spkFence(open, line string) (string, bool) {
	trim := strings.TrimSpace(line)
	if len(trim) >= 3 && (trim[0] == '`' || trim[0] == '~') {
		n := len(trim) - len(strings.TrimLeft(trim, trim[:1]))
		if n >= 3 {
			if open == "" {
				return trim[:n], true
			}
			if trim[0] == open[0] && n >= len(open) && strings.TrimSpace(trim[n:]) == "" {
				return "", true
			}
		}
	}
	return open, open != ""
}

// spkSection returns the section that opens with the unfenced line equal to
// heading, up to the next unfenced heading of the same or a higher level.
func spkSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	start, open := -1, ""
	for i, line := range lines {
		var fenced bool
		open, fenced = spkFence(open, line)
		if fenced {
			continue
		}
		if start >= 0 && spkLevel(line) > 0 && spkLevel(line) <= spkLevel(heading) {
			return strings.Join(lines[start:i], "\n"), true
		}
		if start < 0 && strings.TrimRight(line, " \t\r") == heading {
			start = i
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

func spkReport(t *testing.T, missing []string) {
	t.Helper()
	for _, m := range missing {
		t.Error(m)
	}
}

// spkDecisionLine returns the first decision placeholder line only if every
// unfenced list item in the decisions section carries the marker placeholder
// and a trailing source.
func spkDecisionLine(text string) (string, bool) {
	sec, ok := spkSection(text, spkDecisionHead)
	if !ok {
		return "", false
	}
	found, open := "", ""
	for _, line := range strings.Split(sec, "\n") {
		var fenced bool
		open, fenced = spkFence(open, line)
		if fenced || !spkDecisionItem.MatchString(line) {
			continue
		}
		if !spkMarkedDecisionLine(line) {
			return line, false
		}
		if found == "" {
			found = line
		}
	}
	return found, found != ""
}

func spkMarkedDecisionLine(line string) bool {
	prefix := "- " + spkMarkerSlot + " "
	if !strings.HasPrefix(line, prefix) || strings.Count(line, spkMarkerSlot) != 1 {
		return false
	}
	tail := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	slots := spkPlaceholder.FindAllStringIndex(tail, -1)
	if len(slots) != 2 || slots[0][0] != 0 {
		return false
	}
	return strings.TrimSpace(tail[slots[0][1]:slots[1][0]]) == spkSourceLabel &&
		strings.TrimSpace(tail[slots[1][1]:]) == ""
}

// spkHeader returns the value of a header line before the first level-two
// heading, or "".
func spkHeader(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		if spkLevel(line) == 2 {
			break
		}
		if strings.HasPrefix(line, key) {
			return strings.TrimSpace(strings.TrimPrefix(line, key))
		}
	}
	return ""
}

func spkTemplateHeaders(missing *[]string, where, text string) bool {
	ok := true
	for _, key := range []string{spkStatusKey, spkFormatKey, spkAssuranceKey} {
		if spkHeader(text, key) == "" {
			*missing = append(*missing, where+" lacks its "+key+" header")
			ok = false
		}
	}
	return ok
}

// The decision markers stay format tokens of the Spec skill, and each SPEC
// template's decision lines start with the marker placeholder and end with a
// source.
func TestSpecSkillDecisionMarkers(t *testing.T) {
	requireSkillTokens(t, specSkillDir, spkMarkers...)
	var missing []string
	for _, file := range spkTemplates {
		if _, ok := spkDecisionLine(read(t, file)); !ok {
			missing = append(missing, fmt.Sprintf("%s %s needs at least one unfenced decision line, with every unfenced list item starting %q, then a decision placeholder, then %q and its trailing placeholder", file, spkDecisionHead, "- "+spkMarkerSlot+" ", spkSourceLabel))
		}
	}
	spkReport(t, missing)
}

// Each marked SPEC template carries the Status:, Criteria-Format: and
// Assurance: header lines.
func TestSpecSkillTemplateHeaders(t *testing.T) {
	var missing []string
	for _, file := range spkTemplates {
		spkTemplateHeaders(&missing, file, read(t, file))
	}
	spkReport(t, missing)
}

// The goal location stays a format token of the Spec skill, and no document
// of the skill turns local retention infrastructure into a product
// requirement.
func TestSpecSkillProjectRecords(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "docs/specs/<goal>")
	forbidSkillText(t, specSkillDir, "private backup repository", "primary checkout", "launchd")
}

func TestSpecSkillHelperDecisionLine(t *testing.T) {
	prefix := "- " + spkMarkerSlot + " "
	valid := prefix + "<decision and reason> Source: <source>"
	bare := "- <decision and reason>"
	for _, c := range []struct {
		name, line string
		want       bool
	}{
		{"marker decision source", valid, true},
		{"multiple marked decisions", valid + "\n" + prefix + "<another decision> Source: <another source>", true},
		{"unmarked decision after marked decision", valid + "\n" + bare + " Source: <source>", false},
		{"unmarked decision before marked decision", bare + " Source: <source>\n" + valid, false},
		{"unmarked star list item", valid + "\n* <decision> Source: <source>", false},
		{"unmarked plus list item", valid + "\n+ <decision> Source: <source>", false},
		{"unmarked numbered list item", valid + "\n1. <decision> Source: <source>", false},
		{"source-less decision after marked decision", valid + "\n" + bare, false},
		{"source-less decision before marked decision", bare + "\n" + valid, false},
		{"marked decision without source after marked decision", valid + "\n" + prefix + "<another decision>", false},
		{"source in prose", "Source: describes the final field.\n" + valid, true},
		{"source before decision", prefix + "Source: <source> <decision and reason>", false},
		{"marker after decision", "- <decision and reason> " + spkMarkerSlot + " Source: <source>", false},
		{"indented marker", " " + prefix + "<decision and reason> Source: <source>", false},
		{"missing decision placeholder", prefix + "decision Source: <source>", false},
		{"missing source placeholder", prefix + "<decision and reason> Source: source", false},
		{"content after source", prefix + "<decision and reason> Source: <source> trailing decision", false},
		{"duplicate marker", prefix + spkMarkerSlot + " <decision and reason> Source: <source>", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			line, got := spkDecisionLine(spkDecisionHead + "\n\n" + c.line)
			if got != c.want {
				t.Errorf("decision line = %t, want %t for %q", got, c.want, line)
			}
		})
	}
	for _, fence := range []string{"```", "~~~"} {
		example := fence + "md\n" + prefix + "<example decision> Source: <example source>\n" + fence
		for _, c := range []struct {
			name, body string
			want       bool
		}{
			{"fenced decision only", example, false},
			{"fenced decision and bare real decision", example + "\n" + bare, false},
			{"fenced decision and unmarked real source", example + "\n" + bare + " Source: <source>", false},
			{"fenced example before real decision", example + "\n" + valid, true},
			{"fenced unmarked example before real decision", fence + "md\n" + bare + " Source: <source>\n" + fence + "\n" + valid, true},
		} {
			t.Run(fence+"/"+c.name, func(t *testing.T) {
				line, got := spkDecisionLine(spkDecisionHead + "\n\n" + c.body)
				if got != c.want || (got && line != valid) {
					t.Errorf("decision line = %q, %t; want unfenced line %q, %t", line, got, valid, c.want)
				}
			})
		}
	}
}

func TestSpecSkillHelperTemplateHeaders(t *testing.T) {
	line := "- " + spkMarkerSlot + " <decision and reason> Source: <source>"
	for _, c := range []struct {
		name, headers string
		want          bool
	}{
		{"standard", "Status: Draft\nCriteria-Format: nested/1\nAssurance: standard", true},
		{"strict", "Status: Draft\nCriteria-Format: nested/1\nAssurance: strict", true},
		{"missing status", "Criteria-Format: nested/1\nAssurance: strict", false},
		{"missing format", "Status: Draft\nAssurance: strict", false},
		{"missing assurance", "Status: Draft\nCriteria-Format: nested/1", false},
		{"empty assurance", "Status: Draft\nCriteria-Format: nested/1\nAssurance:", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := "# synthetic\n\n" + c.headers + "\n\n" + spkDecisionHead + "\n\n" + line
			var missing []string
			if got := spkTemplateHeaders(&missing, c.name, text); got != c.want || (len(missing) == 0) != c.want {
				t.Errorf("template headers = %t, want %t; missing = %q", got, c.want, missing)
			}
		})
	}
}
