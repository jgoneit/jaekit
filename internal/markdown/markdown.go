// Package markdown reads the small Markdown subset that the Jaekit contracts
// rely on: headings, pipe tables, list items, inline code spans, and relative
// links. Content inside fenced code blocks is ignored.
package markdown

import (
	"regexp"
	"strings"
)

// Line is one source line with its 1-based number.
type Line struct {
	No   int
	Text string
}

// Doc is a parsed Markdown document.
type Doc struct {
	lines []Line // lines outside fenced code blocks; fenced lines are dropped
}

// Parse splits src into lines and drops fenced code blocks.
func Parse(src string) *Doc {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	raw := strings.Split(src, "\n")
	d := &Doc{}
	fence := ""
	for i, text := range raw {
		trimmed := strings.TrimSpace(text)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		d.lines = append(d.lines, Line{No: i + 1, Text: text})
	}
	return d
}

// Lines returns all lines outside fenced code blocks.
func (d *Doc) Lines() []Line { return d.lines }

func heading(text string) (level int, title string, ok bool) {
	t := strings.TrimRight(text, " \t")
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(t) || t[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(t[n:]), true
}

// Section returns the lines under the first heading of the given level whose
// text equals title exactly, up to the next heading of the same or a higher
// level. The heading line itself is not included.
func (d *Doc) Section(level int, title string) ([]Line, bool) {
	for i, l := range d.lines {
		lv, t, ok := heading(l.Text)
		if !ok || lv != level || t != title {
			continue
		}
		var out []Line
		for _, m := range d.lines[i+1:] {
			if lv2, _, ok := heading(m.Text); ok && lv2 <= level {
				break
			}
			out = append(out, m)
		}
		return out, true
	}
	return nil, false
}

// Table is a pipe table.
type Table struct {
	Line   int
	Header []string
	Rows   []Row
}

// Row is one table body row.
type Row struct {
	Line  int
	Cells []string
}

var separatorCell = regexp.MustCompile(`^:?-{3,}:?$`)

func isSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !separatorCell.MatchString(strings.TrimSpace(c)) {
			return false
		}
	}
	return true
}

// SplitRow splits a table row into trimmed cells. A backslash-escaped pipe
// (`\|`) stays inside its cell and is unescaped.
func SplitRow(text string) []string {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "|")
	if strings.HasSuffix(t, "|") && !strings.HasSuffix(t, `\|`) {
		t = t[:len(t)-1]
	}
	var cells []string
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c == '\\' && i+1 < len(t) && t[i+1] == '|' {
			b.WriteByte('|')
			i++
			continue
		}
		if c == '|' {
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	cells = append(cells, strings.TrimSpace(b.String()))
	return cells
}

// Tables returns the pipe tables found in lines.
func Tables(lines []Line) []Table {
	var out []Table
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i].Text), "|") {
			continue
		}
		if !isSeparator(SplitRow(lines[i+1].Text)) {
			continue
		}
		tb := Table{Line: lines[i].No, Header: SplitRow(lines[i].Text)}
		j := i + 2
		for ; j < len(lines); j++ {
			if !strings.HasPrefix(strings.TrimSpace(lines[j].Text), "|") {
				break
			}
			tb.Rows = append(tb.Rows, Row{Line: lines[j].No, Cells: SplitRow(lines[j].Text)})
		}
		out = append(out, tb)
		i = j - 1
	}
	return out
}

// HeaderIs reports whether the table header equals want exactly.
func (t Table) HeaderIs(want ...string) bool {
	if len(t.Header) != len(want) {
		return false
	}
	for i := range want {
		if t.Header[i] != want[i] {
			return false
		}
	}
	return true
}

// Cell returns cell i of the row, or "" when the row is short.
func (r Row) Cell(i int) string {
	if i < len(r.Cells) {
		return r.Cells[i]
	}
	return ""
}

// Item is a top-level list item. Text is the first line after the marker;
// Body holds continuation lines (indented lines and nested items).
type Item struct {
	Line int
	Text string
	Body []string
}

var itemMarker = regexp.MustCompile(`^ {0,3}[-*+] +`)

// Items returns the top-level list items in lines.
func Items(lines []Line) []Item {
	var out []Item
	cur := -1
	for _, l := range lines {
		if loc := itemMarker.FindStringIndex(l.Text); loc != nil {
			out = append(out, Item{Line: l.No, Text: strings.TrimSpace(l.Text[loc[1]:])})
			cur = len(out) - 1
			continue
		}
		if strings.TrimSpace(l.Text) == "" {
			cur = -1
			continue
		}
		if cur >= 0 && (strings.HasPrefix(l.Text, " ") || strings.HasPrefix(l.Text, "\t")) {
			out[cur].Body = append(out[cur].Body, strings.TrimSpace(l.Text))
			continue
		}
		cur = -1
	}
	return out
}

// CodeSpans returns the contents of inline code spans in s, in order.
func CodeSpans(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(s) && s[i+n] == '`' {
			n++
		}
		delim := strings.Repeat("`", n)
		rest := s[i+n:]
		end := strings.Index(rest, delim)
		if end < 0 {
			break
		}
		content := rest[:end]
		if n > 1 {
			content = strings.TrimSpace(content)
		}
		out = append(out, content)
		i += n + end + n
	}
	return out
}

var linkPattern = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
var schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// RelativeLinks returns relative link targets in d with any #fragment removed.
// Absolute URLs, absolute paths, and fragment-only links are skipped.
func (d *Doc) RelativeLinks() []string {
	var out []string
	for _, l := range d.lines {
		for _, m := range linkPattern.FindAllStringSubmatch(l.Text, -1) {
			t := m[1]
			if schemePattern.MatchString(t) || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "/") {
				continue
			}
			if k := strings.IndexByte(t, '#'); k >= 0 {
				t = t[:k]
			}
			if t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// IsNone reports whether a cell means "nothing": empty, "-", or "—".
func IsNone(cell string) bool {
	c := strings.TrimSpace(cell)
	return c == "" || c == "-" || c == "—"
}
