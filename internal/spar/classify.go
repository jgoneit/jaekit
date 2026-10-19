package spar

import (
	"bytes"
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

var criterionID = regexp.MustCompile(`(?:^|[^A-Za-z0-9])(AC-[0-9]+)(?:[^0-9]|$)`)
var linkLine = regexp.MustCompile(`^(.+?):([0-9]+)(?:[-–][0-9]+)?$`)

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
		for _, m := range criterionID.FindAllStringSubmatch(d.text, -1) {
			seen[m[1]] = true
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
	loc := strings.TrimSpace(location)
	if fields := strings.Fields(loc); len(fields) > 0 {
		loc = fields[0]
	}
	if m := linkLine.FindStringSubmatch(loc); m != nil {
		loc = m[1]
	}
	for _, side := range []string{"head/", "base/", "./"} {
		loc = strings.TrimPrefix(loc, side)
	}
	return loc
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
		ids := regexp.MustCompile(`AC-[0-9]+`).FindAllString(link, -1)
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
		ref := link
		if fields := strings.Fields(ref); len(fields) > 0 {
			ref = fields[0]
		}
		line := 0
		if m := linkLine.FindStringSubmatch(ref); m != nil {
			ref = m[1]
			if line, _ = strconv.Atoi(m[2]); line < 1 {
				return false // a line that is given must exist
			}
		}
		ref = strings.TrimPrefix(strings.TrimPrefix(ref, "head/"), "./")
		for _, d := range c.targets.Rules {
			if contains(d.Names, ref) {
				return line == 0 || line <= d.Lines
			}
		}
		if kind == "contract" && ref != "" && c.repo != "" {
			if data, ok := fileAt(c.repo, c.head, ref); ok {
				return line == 0 || line <= lineCount(data)
			}
		}
		return false
	}
	return false
}

// classify applies the class rules: a check-path finding is a defect only
// when it claims a false pass, an inverted meaning or a linked contract
// violation, and a hardening item otherwise, whatever was proposed; a
// regression the change made is a defect; an unlinked defect proposal is a
// note.
func (c classifier) classify(reviewer string, raw map[string]any) Finding {
	f := Finding{
		Reviewer: reviewer, ReviewerID: str(raw["id"]), ProposedClass: str(raw["proposed_class"]),
		Angle: str(raw["angle"]), Location: str(raw["location"]), Claim: str(raw["claim"]),
		FailureScenario: str(raw["failure_scenario"]), Evidence: str(raw["evidence"]),
		ClaimedLinkKind: str(raw["link_kind"]), ClaimedLink: str(raw["link"]),
		IntroducedByChange: str(raw["introduced_by_change"]), Regression: str(raw["regression"]),
		CheckImpact: str(raw["check_impact"]), Confidence: str(raw["confidence"]),
	}
	f.ID = reviewer + "-" + f.ReviewerID
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
