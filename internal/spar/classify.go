package spar

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Finding is one reviewer finding as the round records it.
type Finding struct {
	ID                 string `json:"id"`
	Reviewer           string `json:"reviewer"`
	ReviewerID         string `json:"reviewer_id"`
	Class              string `json:"class"`
	ClassReason        string `json:"class_reason"`
	ProposedClass      string `json:"proposed_class"`
	Angle              string `json:"angle"`
	Location           string `json:"location"`
	Path               string `json:"path"`
	InCheckPaths       bool   `json:"in_check_paths"`
	Claim              string `json:"claim"`
	FailureScenario    string `json:"failure_scenario"`
	Evidence           string `json:"evidence"`
	LinkKind           string `json:"link_kind"`
	Link               string `json:"link"`
	ClaimedLinkKind    string `json:"claimed_link_kind"`
	ClaimedLink        string `json:"claimed_link"`
	IntroducedByChange string `json:"introduced_by_change"`
	Regression         string `json:"regression"`
	CheckImpact        string `json:"check_impact"`
	Confidence         string `json:"confidence"`
}

// LinkDocument is a rule document a finding may name, with its line count.
type LinkDocument struct {
	Names []string `json:"names"`
	Lines int      `json:"lines"`
}

// LinkTargets are what a link must name to be kept: criteria IDs present in
// the criteria documents and the rule documents of the round.
type LinkTargets struct {
	CriteriaIDs []string       `json:"criteria_ids"`
	Rules       []LinkDocument `json:"rules"`
}

var criterionID = regexp.MustCompile(`AC-[0-9]+`)

// One rule reads every place a finding names: its location and its rule or
// contract link go through parseReference, and criterion IDs, in criteria
// documents and in links, through criterionIDs.

// referenceLine is the start of a reference: a path, which may hold spaces,
// then :<line> or :<line>-<line>, then the end or a space.
var referenceLine = regexp.MustCompile(`^(.+?):([0-9]+)(?:[-–][0-9]+)?(?:\s|$)`)

// reference is what a location or link names, without its workspace prefix.
type reference struct {
	path    string // the path before the line; without a line, the first word
	whole   string // without a line, the whole text, for a path with spaces
	line    int
	hasLine bool
}

// parseReference reads a location or link. With a line, the path ends at the
// first :<line> that the end or a space follows, so text after it, even
// another location, is left out. Without a line, the path is the first word,
// and whole keeps the whole text.
func parseReference(text string) reference {
	text = strings.TrimSpace(text)
	if m := referenceLine.FindStringSubmatch(text); m != nil {
		line, _ := strconv.Atoi(m[2])
		return reference{path: workspacePath(m[1]), line: line, hasLine: true}
	}
	r := reference{whole: workspacePath(text)}
	if fields := strings.Fields(text); len(fields) > 0 {
		r.path = workspacePath(fields[0])
	}
	return r
}

// workspacePath removes one workspace prefix, once, so a repository folder
// named head or base keeps its name.
func workspacePath(path string) string {
	for _, prefix := range []string{"head/", "base/", "./"} {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix)
		}
	}
	return path
}

// criterionIDs finds the AC-n IDs in a text. An ID must not follow a letter
// or digit, and the check does not consume the character before it, so IDs
// written next to each other, as in "AC-1,AC-2", are all found.
func criterionIDs(text string) []string {
	var ids []string
	for _, at := range criterionID.FindAllStringIndex(text, -1) {
		if at[0] > 0 {
			if b := text[at[0]-1]; b < 0x80 && (b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z') {
				continue
			}
		}
		ids = append(ids, text[at[0]:at[1]])
	}
	return ids
}

func lineCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

func criteriaIDs(docs []Document) []string {
	seen := map[string]bool{}
	for _, d := range docs {
		for _, id := range criterionIDs(d.text) {
			seen[id] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func ruleTargets(repo string, docs []Document) []LinkDocument {
	out := make([]LinkDocument, 0, len(docs))
	for _, d := range docs {
		names := []string{d.Name, filepath.Base(d.Source), filepath.ToSlash(d.Source)}
		if rel, err := filepath.Rel(repo, d.Source); err == nil && filepath.IsLocal(rel) {
			names = append(names, filepath.ToSlash(rel))
		}
		out = append(out, LinkDocument{Names: uniq(names), Lines: lineCount([]byte(d.text))})
	}
	return out
}

func uniq(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// locationPath turns "head/app/main.py:12" into "app/main.py".
func locationPath(location string) string {
	return parseReference(location).path
}

type classifier struct {
	repo       string
	head       string
	checkPaths []string
	targets    LinkTargets
}

// verifyLink keeps a link only when what it names exists.
func (c classifier) verifyLink(kind, link string) bool {
	link = strings.TrimSpace(link)
	switch kind {
	case "criterion":
		ids := criterionIDs(link)
		if len(ids) == 0 {
			return false
		}
		for _, id := range ids {
			if !contains(c.targets.CriteriaIDs, id) {
				return false
			}
		}
		return true
	case "rule", "contract":
		// Without a line, the whole link names the document, or else its
		// first word does.
		r := parseReference(link)
		if r.hasLine && r.line < 1 {
			return false // a line that is given must exist
		}
		refs := []string{r.path}
		if !r.hasLine && r.whole != r.path {
			refs = []string{r.whole, r.path}
		}
		for _, ref := range refs {
			if ok, found := c.linkTarget(kind, ref, r.line); found {
				return ok
			}
		}
		return false
	}
	return false
}

// linkTarget looks up the document a rule or contract link names, and
// whether a given line exists in it.
func (c classifier) linkTarget(kind, ref string, line int) (ok, found bool) {
	for _, d := range c.targets.Rules {
		if contains(d.Names, ref) {
			return line == 0 || line <= d.Lines, true
		}
	}
	if kind == "contract" && ref != "" && c.repo != "" {
		if data, exists := fileAt(c.repo, c.head, ref); exists {
			return line == 0 || line <= lineCount(data), true
		}
	}
	return false, false
}

// classify applies the class rules: a check-path finding is a defect only
// when it claims a false pass, an inverted meaning or a linked contract
// violation, and a hardening item otherwise, whatever was proposed; a
// regression the change made is a defect; an unlinked defect proposal is a
// note.
func (c classifier) classify(reviewer string, n int, raw map[string]any) Finding {
	f := Finding{
		Reviewer: reviewer, ReviewerID: str(raw["id"]), ProposedClass: str(raw["proposed_class"]),
		Angle: str(raw["angle"]), Location: str(raw["location"]), Claim: str(raw["claim"]),
		FailureScenario: str(raw["failure_scenario"]), Evidence: str(raw["evidence"]),
		ClaimedLinkKind: str(raw["link_kind"]), ClaimedLink: str(raw["link"]),
		IntroducedByChange: str(raw["introduced_by_change"]), Regression: str(raw["regression"]),
		CheckImpact: str(raw["check_impact"]), Confidence: str(raw["confidence"]),
	}
	// Spar numbers a reviewer's findings in report order, so the round ID
	// never depends on the reviewer's own ID, which stays in reviewer_id.
	f.ID = fmt.Sprintf("%s-F%d", reviewer, n)
	f.Path = locationPath(f.Location)
	f.InCheckPaths = f.Path != "" && matchAny(c.checkPaths, f.Path)
	linked := f.ClaimedLinkKind != "none" && c.verifyLink(f.ClaimedLinkKind, f.ClaimedLink)
	if linked {
		f.LinkKind, f.Link = f.ClaimedLinkKind, f.ClaimedLink
	} else {
		f.LinkKind, f.Link = "none", ""
	}
	switch {
	case f.InCheckPaths && (f.CheckImpact == "false_pass" || f.CheckImpact == "meaning_inverted"):
		f.Class, f.ClassReason = "defect", "check path finding with "+f.CheckImpact
	case f.InCheckPaths && f.CheckImpact == "contract_violation" && linked:
		f.Class, f.ClassReason = "defect", "check path finding with a linked contract violation"
	case f.InCheckPaths:
		f.Class, f.ClassReason = "hardening", "check path finding without a false pass, inverted meaning or linked contract violation"
	case f.ProposedClass == "requirement-question":
		f.Class, f.ClassReason = "requirement-question", "proposed requirement question"
	case f.IntroducedByChange == "yes" && f.Regression == "yes":
		f.Class, f.ClassReason = "defect", "regression introduced by the change"
	case f.ProposedClass == "defect" && !linked:
		f.Class, f.ClassReason = "note", "defect proposal without a link that exists"
	default:
		f.Class, f.ClassReason = f.ProposedClass, "proposed class"
	}
	return f
}
