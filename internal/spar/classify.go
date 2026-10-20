package spar

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	LocationUnreadable bool   `json:"location_unreadable"`
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

// LinkDocument is a rule document a link may name: its repository-relative
// and absolute paths, its file name and its line count.
type LinkDocument struct {
	Names    []string `json:"names"`
	FileName string   `json:"file_name,omitempty"`
	Lines    int      `json:"lines"`
}

// LinkTargets are what a link must name to be kept: criteria IDs present in
// the criteria documents and the rule documents of the round.
type LinkTargets struct {
	CriteriaIDs []string       `json:"criteria_ids"`
	Rules       []LinkDocument `json:"rules"`
}

var criterionID = regexp.MustCompile(`AC-[0-9]+`)

// One grammar reads every place a finding names, as the input table of
// contracts/spar-input.md (rows G7 and G8) gives it: a location and a rule
// or contract link go through parseReference, and criterion IDs, in
// criteria documents and in links, through criterionIDs (row G9).

// reference is what a location or link names, without its prefix.
type reference struct {
	prefix  string // the prefix removed, or ""
	path    string // with a line: the path before it; without one: the first word
	whole   string // without a line: the whole text
	line    int
	end     int // the last line of a range, or line
	hasLine bool
}

// A location names a file of a workspace copy, head/ or base/. A rule or
// contract link names a repository path, which head/ may precede; a link's
// base/ is a repository folder.
var (
	locationPrefixes = []string{"head/", "base/", "./"}
	linkPrefixes     = []string{"head/", "./"}
)

// isEnd reports a character that may follow a line: white space, a comma
// or a semicolon.
func isEnd(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == ';' }

func endsAt(text string, i int) bool {
	if i == len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return isEnd(r)
}

func digitsAt(text string, i int) int {
	j := i
	for j < len(text) && text[j] >= '0' && text[j] <= '9' {
		j++
	}
	return j
}

func lineNumber(digits string) int {
	n, err := strconv.Atoi(digits)
	if err != nil {
		return math.MaxInt // a line too large to count stays a line
	}
	return n
}

// parseReference reads a location or link. With a line, the path ends at
// the first ":<line>" or ":<line>-<line>" that the end of the text, white
// space, a comma or a semicolon follows, so text after it, even another
// location, is left out. Without such a place, the reference has no line:
// whole keeps the whole text and path its first word. One prefix is
// removed from the front, once.
func parseReference(text string, prefixes []string) reference {
	text = strings.TrimSpace(text)
	for i := 0; i < len(text); i++ {
		if text[i] != ':' {
			continue
		}
		j := digitsAt(text, i+1)
		if j == i+1 {
			continue
		}
		line := lineNumber(text[i+1 : j])
		for _, dash := range []string{"-", "–"} {
			if !strings.HasPrefix(text[j:], dash) {
				continue
			}
			k := j + len(dash)
			if m := digitsAt(text, k); m > k && endsAt(text, m) {
				prefix, path := trimOnePrefix(text[:i], prefixes)
				return reference{prefix: prefix, path: path, line: line, end: lineNumber(text[k:m]), hasLine: true}
			}
		}
		if endsAt(text, j) {
			prefix, path := trimOnePrefix(text[:i], prefixes)
			return reference{prefix: prefix, path: path, line: line, end: line, hasLine: true}
		}
	}
	prefix, whole := trimOnePrefix(text, prefixes)
	r := reference{prefix: prefix, whole: whole}
	if fields := strings.Fields(text); len(fields) > 0 {
		_, r.path = trimOnePrefix(fields[0], prefixes)
	}
	return r
}

// trimOnePrefix removes the first prefix that path starts with, and only
// that one, so a repository folder named head or base keeps its name.
func trimOnePrefix(path string, prefixes []string) (string, string) {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return prefix, strings.TrimPrefix(path, prefix)
		}
	}
	return "", path
}

// candidates are the paths a reference may name: with a line its path,
// without one the whole text first and then its first word.
func (r reference) candidates() []string {
	if r.hasLine {
		return []string{r.path}
	}
	if r.whole != r.path {
		return []string{r.whole, r.path}
	}
	return []string{r.path}
}

func isIDChar(b byte) bool {
	return b < 0x80 && (b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z')
}

// criterionIDs finds the AC-n IDs in a text. Neither the character before
// an ID nor the one after it may be an ASCII letter or digit, and the check
// consumes neither, so IDs written next to each other, as in "AC-1,AC-2",
// are all found while "AC-1a" holds none.
func criterionIDs(text string) []string {
	var ids []string
	for _, at := range criterionID.FindAllStringIndex(text, -1) {
		if at[0] > 0 && isIDChar(text[at[0]-1]) || at[1] < len(text) && isIDChar(text[at[1]]) {
			continue
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

// documentNames are the names a document is found by: its repository path
// when it is inside the repository, and its real absolute path.
func documentNames(repo string, d Document) []string {
	names := []string{filepath.ToSlash(d.Source)}
	if rel, err := filepath.Rel(repo, d.Source); err == nil && filepath.IsLocal(rel) {
		names = append([]string{filepath.ToSlash(rel)}, names...)
	}
	return uniq(names)
}

func ruleTargets(repo string, docs []Document) []LinkDocument {
	out := make([]LinkDocument, 0, len(docs))
	for _, d := range docs {
		out = append(out, LinkDocument{Names: documentNames(repo, d), FileName: filepath.Base(d.Source),
			Lines: lineCount([]byte(d.text))})
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

// commitFiles is a commit as its workspace copy holds it: the paths of its
// files (blobs, symbolic links included) and the folders above them.
type commitFiles struct {
	files   map[string]treeEntry
	folders map[string]bool
	read    func(object string) ([]byte, error)
}

func newCommitFiles(entries []treeEntry, read func(string) ([]byte, error)) *commitFiles {
	t := &commitFiles{files: map[string]treeEntry{}, folders: map[string]bool{}, read: read}
	for _, e := range entries {
		if e.kind != "blob" {
			continue // a gitlink is not in the copy
		}
		t.files[e.path] = e
		for dir := e.path; ; {
			i := strings.LastIndex(dir, "/")
			if i < 0 {
				break
			}
			dir = dir[:i]
			t.folders[dir] = true
		}
	}
	return t
}

func (t *commitFiles) has(path string) bool {
	if t == nil {
		return false
	}
	_, file := t.files[path]
	return file || t.folders[path]
}

// regularLines returns the line count of a regular file (mode 100644 or
// 100755) of the commit.
func (t *commitFiles) regularLines(path string) (int, bool) {
	if t == nil {
		return 0, false
	}
	e, ok := t.files[path]
	if !ok || e.mode != "100644" && e.mode != "100755" || t.read == nil {
		return 0, false
	}
	data, err := t.read(e.object)
	if err != nil {
		return 0, false
	}
	return lineCount(data), true
}

// gitTree lists a commit of the repository as commitFiles.
func gitTree(repo, commit string) (*commitFiles, error) {
	entries, err := listTree(repo, commit)
	if err != nil {
		return nil, err
	}
	return newCommitFiles(entries, func(object string) ([]byte, error) {
		return git(repo, "cat-file", "blob", object)
	}), nil
}

// roundDocument is a document of the round a location may name.
type roundDocument struct {
	name     string   // its name in the record (G26)
	names    []string // repository path, real absolute path
	fileName string
	source   string
}

type classifier struct {
	repo       string
	head, base *commitFiles
	docs       []roundDocument
	checkPaths []string
	targets    LinkTargets
}

// newClassifier reads what a round's findings may name: the head and base
// copies, the round's documents and its link targets.
func newClassifier(r *RoundRecord) (classifier, error) {
	c := classifier{repo: r.Repo, checkPaths: r.Config.CheckPaths, targets: r.Links}
	var err error
	if c.head, err = gitTree(r.Repo, r.Head); err != nil {
		return c, fmt.Errorf("the head commit cannot be read: %v", err)
	}
	if c.base, err = gitTree(r.Repo, r.Base); err != nil {
		return c, fmt.Errorf("the base commit cannot be read: %v", err)
	}
	c.docs = roundDocuments(r.Repo, r.Criteria, r.Rules, r.Subjects)
	return c, nil
}

func roundDocuments(repo string, lists ...[]Document) []roundDocument {
	var out []roundDocument
	seen := map[string]bool{}
	for _, list := range lists {
		for _, d := range list {
			if seen[d.Source] {
				continue
			}
			seen[d.Source] = true
			out = append(out, roundDocument{name: d.Name, names: documentNames(repo, d), fileName: filepath.Base(d.Source),
				source: d.Source})
		}
	}
	return out
}

// document finds the round document a path names: by repository or real
// absolute path, or by file name when exactly one document has it.
func (c classifier) document(path string) (string, bool) {
	for _, d := range c.docs {
		if contains(d.names, path) {
			return d.name, true
		}
	}
	var match []roundDocument
	for _, d := range c.docs {
		if d.fileName == path {
			match = append(match, d)
		}
	}
	if len(match) == 1 {
		return match[0].name, true
	}
	return "", false
}

// readLocation returns the repository path a location names, or why it
// cannot be read. head/ and ./ name the head copy, base/ the base copy,
// and no prefix the head copy and then the base copy; except under base/, a
// location may also name a document of the round.
func (c classifier) readLocation(location string) (string, string) {
	if strings.TrimSpace(location) == "" {
		return "", "the location is empty"
	}
	r := parseReference(location, locationPrefixes)
	if r.hasLine && r.line < 1 {
		return "", "line 0 does not exist"
	}
	if r.hasLine && r.end < r.line {
		return "", "the range ends before it starts"
	}
	copies := []*commitFiles{c.head, c.base}
	switch r.prefix {
	case "head/", "./":
		copies = []*commitFiles{c.head}
	case "base/":
		copies = []*commitFiles{c.base}
	}
	for _, cand := range r.candidates() {
		path := strings.TrimRight(cand, "/")
		if path == "" {
			if r.prefix != "" {
				return "", "" // the prefix alone: the top of the repository
			}
			continue
		}
		for _, t := range copies {
			if t.has(path) {
				return path, ""
			}
		}
		if r.prefix != "base/" {
			if name, ok := c.document(path); ok {
				return name, ""
			}
		}
	}
	return "", fmt.Sprintf("no file or folder of the reviewed copies and no round document is %q", strings.TrimRight(r.candidates()[0], "/"))
}

// verifyLink keeps a link only when what it names exists.
func (c classifier) verifyLink(kind, link string) bool {
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
		r := parseReference(link, linkPrefixes)
		if r.hasLine && (r.line < 1 || r.end < r.line) {
			return false // a line that is given must exist, and a range must run forward
		}
		// Without a line, the whole link names the document, or else its
		// first word does.
		for _, ref := range r.candidates() {
			if lines, found, unique := c.linkTarget(kind, ref); found {
				return unique && (!r.hasLine || r.end <= lines)
			}
		}
		return false
	}
	return false
}

// linkTarget looks up the document a rule or contract link names and its
// line count. Rule documents come first, as before: a path names one, and a
// file name alone names one only when no other rule document has that file
// name. A contract link may then name a regular file of the head commit.
func (c classifier) linkTarget(kind, ref string) (lines int, found, unique bool) {
	if ref == "" {
		return 0, false, false
	}
	for _, d := range c.targets.Rules {
		if contains(d.Names, ref) {
			return d.Lines, true, true
		}
	}
	var match []LinkDocument
	for _, d := range c.targets.Rules {
		if d.FileName != "" && d.FileName == ref {
			match = append(match, d)
		}
	}
	if len(match) == 1 {
		return match[0].Lines, true, true
	}
	if kind == "contract" {
		if n, ok := c.head.regularLines(ref); ok {
			return n, true, true
		}
	}
	if len(match) > 1 {
		return 0, true, false // a file name two rule documents share names neither
	}
	return 0, false, false
}

// classify applies the class rules. A readable location in the check paths
// is a defect only when it claims a false pass, an inverted meaning or a
// linked contract violation, and a hardening item otherwise; a regression
// the change made is a defect; an unlinked defect proposal is a note. An
// unreadable location never makes a check-path judgment: rules 3 and 6 do
// not apply and rules 1 and 2 apply without the check-path condition, so a
// misread location cannot hide a defect.
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
	path, why := c.readLocation(f.Location)
	f.LocationUnreadable = why != ""
	f.Path = path
	f.InCheckPaths = !f.LocationUnreadable && f.Path != "" && matchAny(c.checkPaths, f.Path)
	linked := f.ClaimedLinkKind != "none" && c.verifyLink(f.ClaimedLinkKind, f.ClaimedLink)
	if linked {
		f.LinkKind, f.Link = f.ClaimedLinkKind, f.ClaimedLink
	} else {
		f.LinkKind, f.Link = "none", ""
	}
	if f.LocationUnreadable {
		f.Class, f.ClassReason = classifyUnreadable(f, linked)
		f.ClassReason = "unreadable location (" + why + "): " + f.ClassReason
		return f
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

// classifyUnreadable is the class of a finding whose location cannot be
// read: what would have been a defect wherever the location pointed stays
// one, and nothing is deferred automatically.
func classifyUnreadable(f Finding, linked bool) (string, string) {
	switch {
	case f.CheckImpact == "false_pass" || f.CheckImpact == "meaning_inverted":
		return "defect", "a finding with " + f.CheckImpact
	case f.CheckImpact == "contract_violation" && linked:
		return "defect", "a finding with a linked contract violation"
	case f.ProposedClass == "requirement-question":
		return "requirement-question", "proposed requirement question"
	case f.IntroducedByChange == "yes" && f.Regression == "yes":
		return "defect", "regression introduced by the change"
	default:
		return f.ProposedClass, "proposed class"
	}
}
