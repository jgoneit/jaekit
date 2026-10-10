// Package goaldocs reads goal documents (contracts/goal-docs.md) as a
// reader: SPEC.md, the auxiliary documents it links inside the goal directory,
// the criterion IDs, and the optional work breakdown proposal. It never
// writes goal documents.
package goaldocs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jgoneit/jaekit/internal/diag"
	"github.com/jgoneit/jaekit/internal/markdown"
)

// OptionalMarker marks a criterion as optional. It is a contract token and
// is never translated.
const OptionalMarker = "(선택)"

// Criterion is one completion condition from `## Acceptance Criteria`.
type Criterion struct {
	ID       string
	Optional bool
	Text     string
	Line     int
}

// TextSHA256 is the digest a `confirm AC-n` note binds to.
func (c Criterion) TextSHA256() string { return sha(c.Text) }

// Proposal is one row of `## 작업 분해 제안`.
type Proposal struct {
	ID       string
	Criteria []string
	Deps     []string
	Line     int
}

// Goal is the goal document set of one goal directory.
type Goal struct {
	Dir       string // repository-relative, slash-separated
	Status    string
	Criteria  []Criterion
	Proposals []Proposal
	Docs      []string // repository-relative paths of the goal document set, sorted
	Digest    string   // goal digest (hex sha256), see Digest
	Problems  []diag.Problem
}

// Criterion returns the criterion with the given ID, or nil.
func (g *Goal) Criterion(id string) *Criterion {
	for i := range g.Criteria {
		if g.Criteria[i].ID == id {
			return &g.Criteria[i]
		}
	}
	return nil
}

// BundleNames are the execution bundle files in a goal directory. They never
// belong to the goal document set even if SPEC.md links them.
var BundleNames = []string{"PLAN.md", "REVIEW.md", "PROGRESS.md", "runs.jsonl", "tasks"}

func isBundleFile(goalDir, p string) bool {
	rel := strings.TrimPrefix(p, goalDir+"/")
	for _, n := range BundleNames {
		if rel == n || strings.HasPrefix(rel, n+"/") {
			return true
		}
	}
	return false
}

var (
	criterionItem = regexp.MustCompile(`^(?:\*\*(AC-\d+)\*\*|(AC-\d+))(?:\s+|$)(.*)$`)
	criterionRef  = regexp.MustCompile(`AC-\d+`)
	proposalRef   = regexp.MustCompile(`W\d+`)
	proposalID    = regexp.MustCompile(`^W\d+$`)
	statusLine    = regexp.MustCompile(`^Status:\s*(\S+)`)
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Load reads the goal document set of dir (repository-relative) under root.
// Format problems are returned in Goal.Problems; err is only for I/O failures
// other than a missing SPEC.md.
func Load(root, dir string) (*Goal, error) {
	dir = path.Clean(filepath.ToSlash(dir))
	g := &Goal{Dir: dir}
	specPath := dir + "/SPEC.md"
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(specPath)))
	if errors.Is(err, fs.ErrNotExist) {
		g.Problems = append(g.Problems, diag.Problem{Code: "spec_missing", File: specPath, Detail: "the goal directory has no SPEC.md"})
		g.Digest = sha("")
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	doc := markdown.Parse(string(src))
	for _, l := range doc.Lines() {
		if m := statusLine.FindStringSubmatch(strings.TrimSpace(l.Text)); m != nil {
			g.Status = m[1]
			break
		}
	}
	g.readCriteria(doc, specPath)
	g.readProposals(doc, specPath)
	if err := g.collectDocs(root, specPath, string(src)); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Goal) problem(code, file string, line int, format string, a ...any) {
	g.Problems = append(g.Problems, diag.Problem{Code: code, File: file, Line: line, Detail: fmt.Sprintf(format, a...)})
}

func (g *Goal) readCriteria(doc *markdown.Doc, file string) {
	lines, ok := doc.Section(2, "Acceptance Criteria")
	if !ok {
		g.problem("criteria_missing", file, 0, "SPEC.md has no `## Acceptance Criteria` section")
		return
	}
	items, valid := g.criteriaItems(doc, lines, file)
	if !valid {
		return
	}
	seen := map[string]int{}
	for _, it := range items {
		m := criterionItem.FindStringSubmatch(it.Text)
		if m == nil {
			g.problem("criterion_without_id", file, it.Line, "criterion has no AC-n ID: %q", clip(it.Text))
			continue
		}
		id := m[1] + m[2]
		rest := strings.TrimSpace(m[3])
		optional := false
		if strings.HasPrefix(rest, OptionalMarker) {
			optional = true
			rest = strings.TrimSpace(strings.TrimPrefix(rest, OptionalMarker))
		}
		text := strings.Join(append([]string{rest}, it.Body...), "\n")
		if prev, dup := seen[id]; dup {
			g.problem("criterion_duplicate_id", file, it.Line, "%s is already used on line %d", id, prev)
			continue
		}
		seen[id] = it.Line
		g.Criteria = append(g.Criteria, Criterion{ID: id, Optional: optional, Text: strings.TrimSpace(text), Line: it.Line})
	}
	if len(g.Criteria) == 0 && len(seen) == 0 {
		g.problem("criteria_missing", file, 0, "`## Acceptance Criteria` has no AC-n criteria")
	}
}

// criteriaItems keeps format selection in the goal document bytes, so changing
// the interpretation also changes the goal digest used by existing evidence.
// Unmarked documents and all execution-bundle lists keep the legacy reader.
func (g *Goal) criteriaItems(doc *markdown.Doc, lines []markdown.Line, file string) ([]markdown.Item, bool) {
	const key = "Criteria-Format:"
	selected, valid, inSection := false, true, false
	for _, line := range doc.Lines() {
		if strings.HasPrefix(line.Text, "## ") {
			inSection = true
		}
		if !strings.HasPrefix(line.Text, key) {
			continue
		}
		if selected {
			g.problem("criteria_format_duplicate", file, line.No, "SPEC.md must contain at most one %s line", key)
			valid = false
		}
		selected = true
		if inSection || strings.TrimSpace(strings.TrimPrefix(line.Text, key)) != "nested/1" {
			g.problem("criteria_format", file, line.No, "use `Criteria-Format: nested/1` before the first level-two section, or omit the line to keep the legacy format")
			valid = false
		}
	}
	if !valid {
		return nil, false
	}
	if !selected {
		return markdown.Items(lines), true
	}
	items, problems := markdown.NestedItems(lines)
	for _, p := range problems {
		g.problem("criterion_indentation", file, p.Line, "%s", p.Detail)
	}
	return items, true
}

func (g *Goal) readProposals(doc *markdown.Doc, file string) {
	lines, ok := doc.Section(2, "작업 분해 제안")
	if !ok {
		return
	}
	tables := markdown.Tables(lines)
	if len(tables) == 0 {
		return
	}
	tb := tables[0]
	if !tb.HeaderIs("ID", "결과", "조건", "의존") {
		g.problem("table_columns", file, tb.Line, "`## 작업 분해 제안` columns must be ID | 결과 | 조건 | 의존")
		return
	}
	seen := map[string]bool{}
	for _, r := range tb.Rows {
		id := strings.TrimSpace(r.Cell(0))
		if !proposalID.MatchString(id) {
			g.problem("proposal_invalid_id", file, r.Line, "proposal ID must look like W1: %q", id)
			continue
		}
		if seen[id] {
			g.problem("proposal_duplicate_id", file, r.Line, "%s is listed twice", id)
			continue
		}
		seen[id] = true
		p := Proposal{ID: id, Line: r.Line, Criteria: criterionRef.FindAllString(r.Cell(2), -1)}
		if !markdown.IsNone(r.Cell(3)) {
			p.Deps = proposalRef.FindAllString(r.Cell(3), -1)
		}
		g.Proposals = append(g.Proposals, p)
	}
	for _, p := range g.Proposals {
		for _, c := range p.Criteria {
			if g.Criterion(c) == nil {
				g.problem("proposal_unknown_criterion", file, p.Line, "%s refers to %s, which is not a criterion", p.ID, c)
			}
		}
		for _, d := range p.Deps {
			if !seen[d] {
				g.problem("proposal_unknown_dependency", file, p.Line, "%s depends on %s, which is not a proposal", p.ID, d)
			}
		}
	}
	deps := map[string][]string{}
	for _, p := range g.Proposals {
		deps[p.ID] = p.Deps
	}
	if cyc := FindCycle(deps); cyc != nil {
		g.problem("proposal_cycle", file, 0, "proposal dependencies form a cycle: %s", strings.Join(cyc, " -> "))
	}
}

// collectDocs walks relative links from SPEC.md and keeps the Markdown files
// inside the goal directory. The result is sorted and digested.
func (g *Goal) collectDocs(root, specPath, specSrc string) error {
	docs := map[string]string{specPath: specSrc}
	queue := []string{specPath}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, target := range markdown.Parse(docs[cur]).RelativeLinks() {
			p := path.Clean(path.Join(path.Dir(cur), target))
			if !strings.HasSuffix(p, ".md") || !strings.HasPrefix(p, g.Dir+"/") || isBundleFile(g.Dir, p) {
				continue
			}
			if _, done := docs[p]; done {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
			if errors.Is(err, fs.ErrNotExist) {
				g.problem("goal_doc_link_missing", cur, 0, "linked goal document %s does not exist", p)
				docs[p] = ""
				continue
			}
			if err != nil {
				return err
			}
			docs[p] = string(b)
			queue = append(queue, p)
		}
	}
	for p := range docs {
		g.Docs = append(g.Docs, p)
	}
	sort.Strings(g.Docs)
	g.Digest = Digest(g.Docs, docs)
	return nil
}

// Digest is the goal digest: the sha256 of, for each document in path order,
// the path, a NUL byte, the hex sha256 of its content, and a newline.
func Digest(paths []string, content map[string]string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(p)
		b.WriteByte(0)
		b.WriteString(sha(content[p]))
		b.WriteByte('\n')
	}
	return sha(b.String())
}

// FindCycle returns one dependency cycle in deps, or nil. Iteration order is
// sorted so the reported cycle is deterministic.
func FindCycle(deps map[string][]string) []string {
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	state := map[string]int{} // 0 new, 1 visiting, 2 done
	var stack []string
	var found []string
	var visit func(string) bool
	visit = func(n string) bool {
		state[n] = 1
		stack = append(stack, n)
		next := append([]string(nil), deps[n]...)
		sort.Strings(next)
		for _, m := range next {
			if _, known := deps[m]; !known {
				continue
			}
			if state[m] == 1 {
				for i, s := range stack {
					if s == m {
						found = append(append([]string(nil), stack[i:]...), m)
						return true
					}
				}
			}
			if state[m] == 0 && visit(m) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = 2
		return false
	}
	for _, k := range keys {
		if state[k] == 0 && visit(k) {
			return found
		}
	}
	return nil
}

func clip(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
