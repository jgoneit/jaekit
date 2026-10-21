package bundle

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jgoneit/jaekit/internal/diag"
	"github.com/jgoneit/jaekit/internal/markdown"
)

// EnvironmentSection is the optional PLAN section that lists the tools a
// goal's checks need (contracts/bundle.md §2.6). Only `ha doctor` reads it:
// Load, lint, start, check and status never do.
const EnvironmentSection = "실행 환경"

// Probe is one `## 실행 환경` item: a tool name and the command that checks it.
type Probe struct {
	Name    string
	Command string
	Argv    []string
	Line    int
}

// Environment reads `## 실행 환경` from PLAN source src; file names it in
// problems. found is false when the plan has no such section. The section
// runs from its heading to the next level 1 or 2 heading outside fenced code
// blocks, like every PLAN section. Every other line in it is blank or one item
// `- <이름>: `<점검 명령>“, so a fenced code block inside it is a problem.
// When problems is not empty, no probe may run.
func Environment(src, file string) (probes []Probe, found bool, problems []diag.Problem) {
	doc := markdown.Parse(src)
	headings := doc.Headings(2, EnvironmentSection)
	if len(headings) == 0 {
		return nil, false, nil
	}
	add := func(code string, line int, format string, a ...any) {
		problems = append(problems, diag.Problem{Code: code, File: file, Line: line, Detail: fmt.Sprintf(format, a...)})
	}
	lines, _ := doc.Section(2, EnvironmentSection)
	for _, no := range fencedLines(doc, headings[0], lines) {
		add("environment_item_invalid", no, "`## %s` holds only items `- <이름>: `<점검 명령>``; a fenced code block is not an item", EnvironmentSection)
	}
	seen := map[string]int{}
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		end := markdown.ItemMarker(l.Text)
		if end < 0 {
			add("environment_item_invalid", l.No, "`## %s` holds only items `- <이름>: `<점검 명령>``, one tool per line: %q", EnvironmentSection, l.Text)
			continue
		}
		text := strings.TrimSpace(l.Text[end:])
		name, rest, ok := strings.Cut(text, ":")
		name, rest = strings.TrimSpace(name), strings.TrimSpace(rest)
		if !ok || name == "" || strings.Contains(name, "`") || len(rest) < 2 || rest[0] != '`' || rest[len(rest)-1] != '`' || strings.Count(rest, "`") != 2 {
			add("environment_item_invalid", l.No, "an item is a name, a colon and one code span with the check command: %q", text)
			continue
		}
		command := strings.TrimSpace(rest[1 : len(rest)-1])
		if first, dup := seen[name]; dup {
			add("environment_name_duplicate", l.No, "%s is listed twice; the first is on line %d", name, first)
			continue
		}
		seen[name] = l.No
		argv, err := SplitCommand(command)
		switch {
		case errors.Is(err, ErrShellSyntax):
			add("shell_syntax", l.No, "%s check uses shell syntax (pipes, redirects, &&, ;, $); ha runs commands without a shell: %s", name, command)
		case errors.Is(err, ErrEmpty):
			add("command_empty", l.No, "%s has an empty check command", name)
		case err != nil:
			add("command_parse", l.No, "%s check cannot be split into words: %v", name, err)
		default:
			probes = append(probes, Probe{Name: name, Command: command, Argv: argv, Line: l.No})
		}
	}
	for _, line := range headings[1:] {
		add("environment_section_duplicate", line, "PLAN.md has more than one `## %s` section", EnvironmentSection)
	}
	diag.Sort(problems)
	if len(problems) > 0 {
		probes = nil
	}
	return probes, true, problems
}

// fencedLines returns the opening line of each fenced code block between the
// section heading and the section's end. The section ends at the next line
// Parse kept after its last line (the next level 1 or 2 heading), or at the
// end of the document.
func fencedLines(doc *markdown.Doc, heading int, lines []markdown.Line) []int {
	last := heading
	if len(lines) > 0 {
		last = lines[len(lines)-1].No
	}
	next := 0
	for _, l := range doc.Lines() {
		if l.No > last {
			next = l.No
			break
		}
	}
	var opening []int
	for _, no := range doc.Fences() {
		if no > heading && (next == 0 || no < next) {
			opening = append(opening, no)
		}
	}
	return opening
}
