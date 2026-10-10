package markdown

import (
	"fmt"
	"regexp"
	"strings"
)

// ListProblem locates an unsupported indentation in the nested list contract.
type ListProblem struct {
	Line   int
	Detail string
}

var nestedMarker = regexp.MustCompile(`^( *)([-*+])([ \t]+|$)`)

// NestedItems reads the explicitly selected nested/1 list contract. The first
// list marker fixes the section's root indentation. A root item's content
// column is the minimum indentation of its body, including nested list items.
// Content lines are trimmed; connected blank lines become one empty body line.
// Items remains the legacy reader used by other document contracts.
func NestedItems(lines []Line) ([]Item, []ListProblem) {
	var out []Item
	var problems []ListProblem
	rootIndent, contentColumn, cur := -1, 0, -1
	blank := false
	problem := func(line int, detail string) {
		problems = append(problems, ListProblem{Line: line, Detail: detail})
		cur, blank = -1, false
	}
	for _, line := range lines {
		text := line.Text
		if strings.TrimSpace(text) == "" {
			blank = cur >= 0
			continue
		}
		prefix := text[:len(text)-len(strings.TrimLeft(text, " \t"))]
		if strings.Contains(prefix, "\t") {
			problem(line.No, "nested/1 indentation must use spaces, not tabs")
			continue
		}
		indent := len(prefix)
		marker := nestedMarker.FindStringSubmatchIndex(text)
		if marker != nil && strings.Contains(text[marker[6]:marker[7]], "\t") {
			problem(line.No, "nested/1 list markers must be followed by spaces, not tabs")
			continue
		}
		if marker != nil && rootIndent < 0 {
			if indent > 3 {
				problem(line.No, "the first top-level list marker must have zero to three leading spaces")
				continue
			}
			rootIndent = indent
		}
		if marker != nil && indent == rootIndent {
			out = append(out, Item{Line: line.No, Text: strings.TrimSpace(text[marker[1]:])})
			cur, blank = len(out)-1, false
			contentColumn = marker[1]
			if contentColumn < indent+2 {
				contentColumn = indent + 2
			}
			continue
		}
		if cur >= 0 && indent >= contentColumn {
			if blank {
				out[cur].Body = append(out[cur].Body, "")
			}
			out[cur].Body = append(out[cur].Body, strings.TrimSpace(text))
			blank = false
			continue
		}
		if marker != nil || indent > 0 {
			if cur < 0 {
				problem(line.No, "indented content has no parent list item")
			} else {
				problem(line.No, fmt.Sprintf("use column %d for a top-level marker or column %d or later for its body (columns start at zero)", rootIndent, contentColumn))
			}
			continue
		}
		// An unindented paragraph or subheading is outside the current item.
		cur, blank = -1, false
	}
	return out, problems
}
