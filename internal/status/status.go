// Package status computes the state of a goal from the run record, the
// execution bundle, and git (contracts/run-record.md §3–§5). It never
// calls a model and never reads the current time, so the same inputs always
// give the same report.
package status

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/diag"
	"github.com/jgoneit/jaekit/internal/evidence"
	"github.com/jgoneit/jaekit/internal/gitx"
	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/record"
)

// Status rule versions this build computes (run-record.md §4.6). A goal is
// computed under the version its start record names; a new start records
// Rules.
const (
	RulesV1 = "run-rules/1"
	RulesV2 = "run-rules/2"
	RulesV3 = "run-rules/3"
	Rules   = RulesV3
)

// Supported reports whether this build computes goals started under rules.
func Supported(rules string) bool { return rules == RulesV1 || rules == RulesV2 || rules == RulesV3 }

// Excludes returns the paths left out of the code state of goal g under
// rules: the goal's own documents and bundle, and under run-rules/2 also the
// bundle files of the other goals next to it, unless the plan declares that
// this goal's checks read them.
func Excludes(root string, g *goaldocs.Goal, b *bundle.Bundle, rules string) ([]string, error) {
	out := bundle.DocPaths(g)
	if (rules != RulesV2 && rules != RulesV3) || b.Scope.OtherBundlesInput {
		return out, nil
	}
	others, err := bundle.OtherGoalBundles(root, g)
	if err != nil {
		return nil, err
	}
	out = append(out, others...)
	sort.Strings(out)
	return out, nil
}

// ErrorReruns is E: error results allowed for one criterion on the same code.
const ErrorReruns = 1

// Goal states.
const (
	Complete        = "complete"
	Incomplete      = "incomplete"
	NeedsUser       = "needs_user"
	Blocked         = "blocked"
	BudgetExhausted = "budget_exhausted"
)

// ExitCode maps a state to the process exit code.
func ExitCode(state string) int {
	switch state {
	case Complete:
		return 0
	case Incomplete:
		return 1
	case NeedsUser:
		return 2
	case Blocked:
		return 3
	case BudgetExhausted:
		return 4
	}
	return 70
}

// Reason classes.
const (
	ClassExecutor = "executor"
	ClassUser     = "user"
	ClassBlocked  = "blocked"
	ClassBudget   = "budget"
)

var classOf = map[string]string{
	"criterion_missing":        ClassExecutor,
	"criterion_flaky":          ClassExecutor,
	"criterion_failed":         ClassExecutor,
	"criterion_error":          ClassExecutor,
	"baseline_missing":         ClassExecutor,
	"baseline_unexpected_pass": ClassExecutor,
	"baseline_error":           ClassExecutor,
	"baseline_failed":          ClassExecutor,
	"evidence_invalid":         ClassExecutor,
	"record_invalid":           ClassUser,
	"lint_error":               ClassExecutor,
	"not_started":              ClassExecutor,
	"worktree_dirty":           ClassExecutor,
	"task_open":                ClassExecutor,
	"error_limit":              ClassUser,
	"manual_unconfirmed":       ClassUser,
	"record_integrity":         ClassUser,
	"out_of_scope":             ClassUser,
	"test_definition_changed":  ClassUser,
	"spec_changed":             ClassUser,
	"blocked":                  ClassBlocked,
	"budget_exceeded":          ClassBudget,
}

// UnsupportedRulesError means the goal was started under rules this build
// does not compute.
type UnsupportedRulesError struct{ Rules string }

func (e *UnsupportedRulesError) Error() string {
	return fmt.Sprintf("goal was started with rules %q; this ha computes only %q, %q and %q", e.Rules, RulesV1, RulesV2, RulesV3)
}

// Input is everything the computation reads.
type Input struct {
	Repo      *gitx.Repo
	Goal      *goaldocs.Goal
	Bundle    *bundle.Bundle
	Lines     []record.Line
	Integrity *record.IntegrityError
	HaVersion string
}

// Report is the computed state. Its JSON form is the `status/v1` document
// defined in contracts/run-record.md §4.5.
type Report struct {
	Schema                string            `json:"schema"`
	Goal                  string            `json:"goal"`
	Status                string            `json:"status"`
	ExitCode              int               `json:"exit_code"`
	Assurance             string            `json:"assurance"`
	CheckAuthor           string            `json:"check_author"`
	Rules                 string            `json:"rules"`
	HaVersion             string            `json:"ha_version"`
	Skill                 *record.Skill     `json:"skill"`
	RecordHead            *Head             `json:"record_head"`
	CompletionRecord      *int              `json:"completion_record"`
	CompleteWithoutRecord bool              `json:"complete_without_record"`
	Commit                string            `json:"commit"`
	TreeClean             bool              `json:"tree_clean"`
	SpecDigest            string            `json:"spec_digest"`
	Criteria              []Criterion       `json:"criteria"`
	Reasons               []Reason          `json:"reasons"`
	Lint                  []diag.Problem    `json:"lint"`
	Changes               Changes           `json:"changes"`
	Budget                *Budget           `json:"budget"`
	Logs                  []Log             `json:"logs"`
	Usage                 Usage             `json:"usage"`
	Findings              *FindingsReport   `json:"findings"`
	commandDigests        map[string]string // for the done line
}

// Head identifies the last record line.
type Head struct {
	Seq    int    `json:"seq"`
	SHA256 string `json:"sha256"`
}

// Criterion is the evaluation of one condition.
type Criterion struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Required  bool     `json:"required"`
	Satisfied bool     `json:"satisfied"`
	Reason    string   `json:"reason"`
	Records   []int    `json:"records"`
	Attempts  Attempts `json:"attempts"`
}

// Attempts counts every record for a criterion, fresh or not, so failures
// erased by a trivial commit stay visible.
type Attempts struct {
	Checks           int      `json:"checks"`
	Fails            int      `json:"fails"`
	Errors           int      `json:"errors"`
	Baselines        int      `json:"baselines"`
	UnexpectedPasses int      `json:"unexpected_passes"`
	BaselineErrors   int      `json:"baseline_errors"`
	Stale            int      `json:"stale"`
	FailCommits      []string `json:"fail_commits"`
	Unknowns         int      `json:"unknowns"`
	BaselineUnknowns int      `json:"baseline_unknowns"`
	BaselineFails    int      `json:"baseline_fails"`
}

// Reason is one reason the goal is not complete.
type Reason struct {
	Code      string   `json:"code"`
	Class     string   `json:"class"`
	Criterion string   `json:"criterion,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

// Changes lists what a person should look at.
type Changes struct {
	CommandChanged       []CommandChange       `json:"command_changed"`
	KindChanged          []KindChange          `json:"kind_changed"`
	Optional             []string              `json:"optional"`
	InvalidConfirmations []InvalidConfirmation `json:"invalid_confirmations"`
	HaVersionChanged     []VersionChange       `json:"ha_version_changed"`
}

// CommandChange is a criterion whose verification command differs from an
// earlier record.
type CommandChange struct {
	Criterion string   `json:"criterion"`
	Seq       int      `json:"seq"`
	Before    []string `json:"before"`
	After     string   `json:"after"`
}

// KindChange is a criterion whose kind differs from an earlier record.
type KindChange struct {
	Criterion string `json:"criterion"`
	Seq       int    `json:"seq"`
	Before    string `json:"before"`
	After     string `json:"after"`
}

// InvalidConfirmation is a confirm note whose bound value no longer holds.
type InvalidConfirmation struct {
	Seq     int    `json:"seq"`
	Subject string `json:"subject"`
	Reason  string `json:"reason"`
}

// VersionChange is a record line written by a different ha version than the
// start line.
type VersionChange struct {
	Seq       int    `json:"seq"`
	HaVersion string `json:"ha_version"`
}

// Budget is the use of the current budget window.
type Budget struct {
	WindowStart         int           `json:"window_start"`
	Runs                int           `json:"runs"`
	RunsLimit           int           `json:"runs_limit"`
	ElapsedSeconds      int64         `json:"elapsed_seconds"`
	ElapsedLimitSeconds int64         `json:"elapsed_limit_seconds"`
	Exceeded            bool          `json:"exceeded"`
	Revision            int           `json:"revision"`
	RemainingRuns       int           `json:"remaining_runs"`
	ExcessRuns          int           `json:"excess_runs"`
	Reached             bool          `json:"reached"`
	History             []BudgetEvent `json:"history"`
}

// Log points to the local output of a failed or errored run.
type Log struct {
	Seq    int    `json:"seq"`
	Target string `json:"target"`
	Result string `json:"result"`
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
}

type evaluator struct {
	in       Input
	rules    string
	head     string
	clean    bool
	dirty    []gitx.WorktreeChange
	excl     []string
	start    *record.Line
	same     map[string]bool
	overlay  map[string]string
	confirms map[string]bool
	invalid  []string
}

// Compute evaluates the goal.
func Compute(in Input) (*Report, error) {
	g, b := in.Goal, in.Bundle
	e := &evaluator{in: in, rules: Rules, same: map[string]bool{}, overlay: map[string]string{}, confirms: map[string]bool{}}
	for i := range in.Lines {
		if in.Lines[i].Kind == "start" {
			e.start = &in.Lines[i]
			e.rules = e.start.Rules
			break
		}
	}
	if !Supported(e.rules) {
		return nil, &UnsupportedRulesError{Rules: e.rules}
	}
	var err error
	if e.excl, err = Excludes(in.Repo.Root, g, b, e.rules); err != nil {
		return nil, err
	}
	if e.head, err = in.Repo.Head(); err != nil {
		return nil, err
	}
	if e.dirty, err = in.Repo.WorktreeChanges(e.excl); err != nil {
		return nil, err
	}
	e.clean = len(e.dirty) == 0
	rep := &Report{
		Schema: "status/v1", Goal: g.Dir, Assurance: "local", CheckAuthor: "executor",
		Rules: e.rules, HaVersion: in.HaVersion, Commit: e.head, TreeClean: e.clean, SpecDigest: g.Digest,
		Criteria: []Criterion{}, Reasons: []Reason{}, Lint: bundle.Lint(g, b), Logs: []Log{},
		Changes: Changes{CommandChanged: []CommandChange{}, KindChanged: []KindChange{}, Optional: []string{},
			InvalidConfirmations: []InvalidConfirmation{}, HaVersionChanged: []VersionChange{}},
		commandDigests: map[string]string{},
	}
	if e.start != nil && e.start.Skill != nil {
		rep.Skill = e.start.Skill
	}
	if n := len(in.Lines); n > 0 {
		rep.RecordHead = &Head{Seq: in.Lines[n-1].Seq, SHA256: in.Lines[n-1].Hash()}
	}
	if findings, err := FoldFindings(in.Lines, g.Dir); err != nil {
		e.invalid = append(e.invalid, err.Error())
	} else {
		rep.Findings = findings
	}
	if err := e.collectUsage(rep); err != nil {
		return nil, err
	}
	if err := e.readConfirms(rep); err != nil {
		return nil, err
	}
	if err := e.criteria(rep); err != nil {
		return nil, err
	}
	goalReasons, err := e.goalReasons(rep)
	if err != nil {
		return nil, err
	}
	e.changes(rep)
	e.logs(rep)

	completion, err := e.completionRecord(rep)
	if err != nil {
		return nil, err
	}
	var reasons []Reason
	for _, c := range rep.Criteria {
		if c.Required && !c.Satisfied {
			reasons = append(reasons, Reason{Code: c.Reason, Class: classOf[c.Reason], Criterion: c.ID})
		}
	}
	reasons = append(reasons, goalReasons...)
	if completion != nil {
		rep.CompletionRecord = completion
		var kept []Reason
		for _, r := range reasons {
			if r.Code != "budget_exceeded" {
				kept = append(kept, r)
			}
		}
		reasons = kept
		rep.Status = Complete
	} else {
		if (e.rules == RulesV2 || e.rules == RulesV3) && onlyBudget(reasons) {
			// run-rules/2: the evidence is complete and only the budget is
			// over; the goal may record completion. rep.Budget still shows
			// the use and that it was exceeded.
			reasons = nil
		}
		rep.Status = decide(reasons)
		rep.CompleteWithoutRecord = rep.Status == Complete
	}
	if reasons != nil {
		rep.Reasons = reasons
	}
	rep.ExitCode = ExitCode(rep.Status)
	return rep, nil
}

// onlyBudget reports whether budget_exceeded is the one remaining reason.
func onlyBudget(reasons []Reason) bool {
	for _, r := range reasons {
		if r.Code != "budget_exceeded" {
			return false
		}
	}
	return len(reasons) > 0
}

func decide(reasons []Reason) string {
	has := func(class string) bool {
		for _, r := range reasons {
			if r.Class == class {
				return true
			}
		}
		return false
	}
	switch {
	case len(reasons) == 0:
		return Complete
	case has(ClassBlocked):
		return Blocked
	case has(ClassBudget):
		return BudgetExhausted
	case has(ClassExecutor):
		return Incomplete
	default:
		return NeedsUser
	}
}

func (e *evaluator) sameAsHead(commit string) (bool, error) {
	if v, ok := e.same[commit]; ok {
		return v, nil
	}
	v, err := e.in.Repo.Same(commit, e.head, e.excl)
	if err != nil {
		return false, err
	}
	e.same[commit] = v
	return v, nil
}

func (e *evaluator) currentDigest(id string) string {
	if r := e.in.Bundle.Row(id); r != nil {
		return r.CommandDigestFor(e.rules)
	}
	return ""
}

func (e *evaluator) freshCheck(l *record.Line) (bool, error) {
	if !e.clean || !l.TreeClean || l.SpecDigest != e.in.Goal.Digest {
		return false, nil
	}
	if l.Target == nil || l.CommandDigest != e.currentDigest(l.Target.Criterion) {
		return false, nil
	}
	return e.sameAsHead(l.Commit)
}

func (e *evaluator) headDigest(p string) (string, error) {
	if v, ok := e.overlay[p]; ok {
		return v, nil
	}
	data, ok, err := e.in.Repo.Cat(e.head, p)
	if err != nil {
		return "", err
	}
	v := gitx.Deleted
	if ok {
		h := sha256.Sum256(data)
		v = hex.EncodeToString(h[:])
	}
	e.overlay[p] = v
	return v, nil
}

// freshBaseline applies run-record.md §3 to a baseline line. Unlike a check,
// a baseline does not need its commit to match HEAD: it ran on the base
// commit, and the overlay digests tie it to the check files at HEAD.
func (e *evaluator) freshBaseline(l *record.Line) (bool, error) {
	if e.start == nil || l.BaseCommit != e.start.BaseCommit {
		return false, nil
	}
	if !e.clean || !l.TreeClean || l.SpecDigest != e.in.Goal.Digest {
		return false, nil
	}
	if l.Target == nil || l.CommandDigest != e.currentDigest(l.Target.Criterion) {
		return false, nil
	}
	row := e.in.Bundle.Row(l.Target.Criterion)
	if row == nil || !samePaths(row.CheckPaths, l.Overlay) {
		return false, nil
	}
	for _, o := range l.Overlay {
		d, err := e.headDigest(o.Path)
		if err != nil || d != o.Digest {
			return false, err
		}
	}
	return true, nil
}

func (e *evaluator) criteria(rep *Report) error {
	g, b := e.in.Goal, e.in.Bundle
	type cond struct {
		id       string
		required bool
	}
	var conds []cond
	for _, c := range g.Criteria {
		conds = append(conds, cond{c.ID, !c.Optional})
		if c.Optional {
			rep.Changes.Optional = append(rep.Changes.Optional, c.ID)
		}
	}
	for _, r := range b.Rows {
		if r.Executor() {
			conds = append(conds, cond{r.ID, false})
			rep.Changes.Optional = append(rep.Changes.Optional, r.ID)
		}
	}
	for _, c := range conds {
		kind := ""
		if r := b.Row(c.id); r != nil {
			kind = r.Kind
		}
		rep.commandDigests[c.id] = e.currentDigest(c.id)
		ev := Criterion{ID: c.id, Kind: kind, Required: c.required, Records: []int{}, Attempts: Attempts{FailCommits: []string{}}}
		var C, B []*record.Line
		for i := range e.in.Lines {
			l := &e.in.Lines[i]
			if l.Target == nil || l.Target.Criterion != c.id {
				continue
			}
			switch l.Kind {
			case "check":
				ev.Attempts.Checks++
				if l.Result == "fail" || l.Result == "error" {
					if l.Result == "fail" {
						ev.Attempts.Fails++
					} else {
						ev.Attempts.Errors++
					}
					ev.Attempts.FailCommits = appendUnique(ev.Attempts.FailCommits, short(l.Commit))
				}
				if l.Result == "unknown" {
					ev.Attempts.Unknowns++
				}
				fresh, err := e.freshCheck(l)
				if err != nil {
					return err
				}
				if fresh {
					C = append(C, l)
				} else {
					ev.Attempts.Stale++
				}
			case "baseline":
				ev.Attempts.Baselines++
				switch l.Result {
				case "unexpected_pass":
					ev.Attempts.UnexpectedPasses++
				case "error":
					ev.Attempts.BaselineErrors++
				case "unknown":
					ev.Attempts.BaselineUnknowns++
				case "fail":
					ev.Attempts.BaselineFails++
				}
				fresh, err := e.freshBaseline(l)
				if err != nil {
					return err
				}
				if fresh {
					B = append(B, l)
				} else {
					ev.Attempts.Stale++
				}
			}
		}
		ev.Reason = e.judge(c.id, kind, C, B)
		ev.Satisfied = ev.Reason == ""
		if ev.Satisfied {
			for _, l := range append(C, B...) {
				ev.Records = append(ev.Records, l.Seq)
			}
			sort.Ints(ev.Records)
		}
		rep.Criteria = append(rep.Criteria, ev)
	}
	return nil
}

func samePaths(paths []string, overlay []record.Overlay) bool {
	if len(paths) == 0 || len(paths) != len(overlay) {
		return false
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	for _, o := range overlay {
		if !want[o.Path] {
			return false
		}
	}
	return true
}

func count(ls []*record.Line, result string) int {
	n := 0
	for _, l := range ls {
		if l.Result == result {
			n++
		}
	}
	return n
}

// judge returns the first matching reason from run-record.md §4.1, or "" when
// the criterion is satisfied.
func (e *evaluator) judge(id, kind string, C, B []*record.Line) string {
	switch kind {
	case bundle.KindManual:
		if e.confirms[id] {
			return ""
		}
		return "manual_unconfirmed"
	case bundle.KindChange, bundle.KindMaintain:
	default:
		return "criterion_missing"
	}
	for _, l := range append(append([]*record.Line{}, C...), B...) {
		if !validResult(l, e.rules) {
			return "record_invalid"
		}
		if e.rules == RulesV3 && kind == bundle.KindChange {
			r := e.in.Bundle.Row(id)
			if r == nil || evidence.ValidateStored(l.Evidence, r.Evidence, r.EvidenceDigest, l.Kind == "baseline", l.Result, l.Exit) != nil {
				return "evidence_invalid"
			}
		}
	}
	last := func(ls []*record.Line) string {
		if len(ls) == 0 {
			return ""
		}
		return ls[len(ls)-1].Result
	}
	cPass, cFail, cErr := count(C, "pass"), count(C, "fail"), count(C, "error")
	bFail, bErr := count(B, "fail"), count(B, "error")
	if e.rules == RulesV3 && kind == bundle.KindChange {
		// An environment error or skipped observation can determine the
		// overall result without erasing an assertion in the same report.
		// All summaries above have already passed ValidateStored.
		r := e.in.Bundle.Row(id)
		for _, l := range C {
			if l.Result != "fail" && evidence.HasAdverse(l.Evidence, r.Evidence, false) {
				cFail++
			}
		}
		for _, l := range B {
			if l.Result != "fail" && evidence.HasAdverse(l.Evidence, r.Evidence, true) {
				bFail++
			}
		}
	}
	if e.rules == RulesV3 {
		cErr += count(C, "unknown")
		bErr += count(B, "unknown")
		if cErr > ErrorReruns || bErr > ErrorReruns {
			return "error_limit"
		}
	}
	switch {
	case len(C) == 0:
		return "criterion_missing"
	case cPass > 0 && cFail > 0:
		return "criterion_flaky"
	case cFail > 0:
		return "criterion_failed"
	case cErr <= ErrorReruns && (last(C) == "error" || last(C) == "unknown"):
		return "criterion_error"
	}
	if kind == bundle.KindChange {
		switch {
		case len(B) == 0:
			return "baseline_missing"
		case count(B, "unexpected_pass") > 0:
			return "baseline_unexpected_pass"
		case bFail > 0:
			return "baseline_failed"
		case bErr <= ErrorReruns && (last(B) == "error" || last(B) == "unknown"):
			return "baseline_error"
		}
	}
	if cErr > ErrorReruns || bErr > ErrorReruns {
		return "error_limit"
	}
	if cPass == 0 {
		return "criterion_missing"
	}
	if kind == bundle.KindChange && count(B, "fail_as_expected") == 0 {
		return "baseline_missing"
	}
	return ""
}

func (e *evaluator) readConfirms(rep *Report) error {
	type pending struct {
		seq     int
		subject string
		reason  string
	}
	var invalid []pending
	for _, l := range e.in.Lines {
		if l.Kind != "note" || l.Note != "confirm" {
			continue
		}
		valid, why, err := e.confirmValid(l)
		if err != nil {
			return err
		}
		if valid {
			e.confirms[l.Subject] = true
		} else {
			invalid = append(invalid, pending{l.Seq, l.Subject, why})
		}
	}
	for _, p := range invalid {
		if !e.confirms[p.subject] {
			rep.Changes.InvalidConfirmations = append(rep.Changes.InvalidConfirmations, InvalidConfirmation{Seq: p.seq, Subject: p.subject, Reason: p.reason})
		}
	}
	return nil
}

func (e *evaluator) confirmValid(l record.Line) (bool, string, error) {
	g := e.in.Goal
	if l.Bound == nil {
		return false, "no bound value", nil
	}
	s := l.Subject
	switch {
	case s == "spec":
		if l.Bound.SpecDigest != g.Digest {
			return false, "goal documents changed after the confirmation", nil
		}
		return true, "", nil
	case strings.HasPrefix(s, "scope:") || strings.HasPrefix(s, "tests:"):
		p := s[strings.IndexByte(s, ':')+1:]
		blob, err := e.in.Repo.Blob(e.head, p)
		if err != nil {
			return false, "", err
		}
		if blob != l.Bound.Blob {
			return false, "the file changed after the confirmation", nil
		}
		return true, "", nil
	default:
		c := g.Criterion(s)
		if c == nil {
			return false, "the criterion no longer exists", nil
		}
		if l.Bound.CriterionSHA256 != c.TextSHA256() {
			return false, "the criterion text changed after the confirmation", nil
		}
		if l.Bound.SpecDigest != g.Digest {
			return false, "goal documents changed after the confirmation", nil
		}
		return true, "", nil
	}
}

func (e *evaluator) goalReasons(rep *Report) ([]Reason, error) {
	g, b := e.in.Goal, e.in.Bundle
	var rs []Reason
	add := func(code, detail string, paths []string) {
		rs = append(rs, Reason{Code: code, Class: classOf[code], Detail: detail, Paths: paths})
	}
	if n := len(rep.Lint); n > 0 {
		add("lint_error", fmt.Sprintf("%d lint problem(s); run ha lint", n), nil)
	}
	if e.start == nil {
		add("not_started", "no start record", nil)
	}
	if e.in.Integrity != nil {
		add("record_integrity", e.in.Integrity.Error(), nil)
	}
	for _, detail := range e.invalid {
		add("record_invalid", detail, nil)
	}
	if !e.clean {
		var paths, details []string
		for _, change := range e.dirty {
			paths = append(paths, change.Path)
			details = append(details, change.String())
		}
		add("worktree_dirty", "uncommitted changes outside the bundle documents; commit or revert them: "+strings.Join(details, "; "), paths)
	}
	if e.start != nil {
		changes, err := e.in.Repo.Changes(e.start.BaseCommit, e.head, e.excl)
		if err != nil {
			return nil, err
		}
		var outside, tests []string
		for _, ch := range changes {
			if !bundle.MatchAny(b.Scope.Allowed, ch.Path) && !e.confirms["scope:"+ch.Path] {
				outside = append(outside, ch.Path)
			}
			if ch.Status != "A" && bundle.MatchAny(b.Scope.Tests, ch.Path) && !e.confirms["tests:"+ch.Path] {
				tests = append(tests, ch.Path)
			}
		}
		if len(outside) > 0 {
			add("out_of_scope", "changed since base outside 바꿀 수 있는 경로", outside)
		}
		if len(tests) > 0 {
			add("test_definition_changed", "test files that existed at base were modified or deleted", tests)
		}
		if g.Digest != e.start.SpecDigest && !e.confirms["spec"] {
			add("spec_changed", "goal documents differ from the start record", nil)
		}
	}
	var open []string
	for _, t := range b.Tasks {
		if !t.Required {
			continue
		}
		if s := b.Progress[t.ID]; s != "done" && s != "dropped" {
			open = append(open, t.ID)
		}
	}
	if len(open) > 0 {
		add("task_open", "required tasks not done or dropped in PROGRESS.md: "+strings.Join(open, ", "), nil)
	}
	var lastBlock *record.Line
	for i := range e.in.Lines {
		l := &e.in.Lines[i]
		if l.Kind == "note" && (l.Note == "block" || l.Note == "unblock") {
			lastBlock = l
		}
	}
	if lastBlock != nil && lastBlock.Note == "block" {
		add("blocked", fmt.Sprintf("blocked since seq %d (cause: %s)", lastBlock.Seq, lastBlock.Cause), nil)
	}
	if e.start != nil {
		budgetReasons, err := e.budgetReasons(rep)
		if err != nil {
			return nil, err
		}
		rs = append(rs, budgetReasons...)
	}
	return rs, nil
}

func (e *evaluator) budget() (*Budget, error) {
	if e.rules == RulesV3 {
		state, err := FoldBudget(e.in.Lines)
		if err != nil {
			return nil, err
		}
		bud := &Budget{WindowStart: state.WindowStart, Revision: state.Revision, Runs: state.Runs,
			RunsLimit: state.RunsLimit, ElapsedSeconds: state.ElapsedSeconds,
			ElapsedLimitSeconds: state.ElapsedLimitSeconds, History: state.History}
		finishBudget(bud)
		return bud, nil
	}
	lines := record.ExecutionLines(e.in.Lines)
	ws := 0
	for i, l := range lines {
		if l.Kind == "start" || (l.Kind == "note" && l.Note == "reopen") {
			ws = i
		}
	}
	bud := &Budget{WindowStart: lines[ws].Seq, Revision: lines[ws].Seq, History: []BudgetEvent{}}
	if e.start.Budget != nil {
		bud.RunsLimit, bud.ElapsedLimitSeconds = e.start.Budget.Runs, e.start.Budget.ElapsedSeconds
	}
	for _, l := range lines[ws:] {
		if l.Kind == "check" || l.Kind == "baseline" {
			bud.Runs++
		}
	}
	from, err := time.Parse(time.RFC3339, lines[ws].At)
	if err != nil {
		return nil, fmt.Errorf("record seq %d: bad time %q", lines[ws].Seq, lines[ws].At)
	}
	to, err := time.Parse(time.RFC3339, lines[len(lines)-1].At)
	if err != nil {
		return nil, fmt.Errorf("record seq %d: bad time %q", lines[len(lines)-1].Seq, lines[len(lines)-1].At)
	}
	bud.ElapsedSeconds = int64(to.Sub(from) / time.Second)
	finishBudget(bud)
	return bud, nil
}

// completionRecord returns the seq of a valid completion record, or nil.
func (e *evaluator) completionRecord(rep *Report) (*int, error) {
	if e.in.Integrity != nil || !e.clean || len(e.invalid) > 0 {
		return nil, nil
	}
	if e.rules == RulesV3 {
		for _, c := range rep.Criteria {
			if c.Required && !c.Satisfied {
				return nil, nil
			}
		}
	}
	lines := e.in.Lines
	for i := len(lines) - 1; i >= 0; i-- {
		d := lines[i]
		if d.Kind != "done" || d.Status != Complete {
			continue
		}
		for _, l := range lines[i+1:] {
			if l.Kind == "note" && l.Note == "reopen" {
				return nil, nil
			}
		}
		if d.SpecDigest != e.in.Goal.Digest {
			return nil, nil
		}
		for _, c := range rep.Criteria {
			if c.Required && d.CommandDigests[c.ID] != rep.commandDigests[c.ID] {
				return nil, nil
			}
		}
		same, err := e.sameAsHead(d.Commit)
		if err != nil || !same {
			return nil, err
		}
		seq := d.Seq
		return &seq, nil
	}
	return nil, nil
}

func (e *evaluator) changes(rep *Report) {
	b := e.in.Bundle
	type key struct{ id, v string }
	seenCmd, seenKind := map[key]bool{}, map[key]bool{}
	ids := make([]string, 0, len(rep.Criteria))
	for _, c := range rep.Criteria {
		ids = append(ids, c.ID)
	}
	for _, id := range ids {
		row := b.Row(id)
		if row == nil {
			continue
		}
		for _, l := range e.in.Lines {
			if (l.Kind != "check" && l.Kind != "baseline") || l.Target == nil || l.Target.Criterion != id {
				continue
			}
			if l.CommandDigest != row.CommandDigestFor(e.rules) && !seenCmd[key{id, l.CommandDigest}] {
				seenCmd[key{id, l.CommandDigest}] = true
				rep.Changes.CommandChanged = append(rep.Changes.CommandChanged, CommandChange{Criterion: id, Seq: l.Seq, Before: l.Argv, After: row.Command})
			}
			if l.CriterionKind != "" && l.CriterionKind != row.Kind && !seenKind[key{id, l.CriterionKind}] {
				seenKind[key{id, l.CriterionKind}] = true
				rep.Changes.KindChanged = append(rep.Changes.KindChanged, KindChange{Criterion: id, Seq: l.Seq, Before: l.CriterionKind, After: row.Kind})
			}
		}
	}
	if len(e.in.Lines) > 0 {
		first := e.in.Lines[0].HaVersion
		if e.start != nil {
			first = e.start.HaVersion
		}
		for _, l := range e.in.Lines {
			if l.HaVersion != first {
				rep.Changes.HaVersionChanged = append(rep.Changes.HaVersionChanged, VersionChange{Seq: l.Seq, HaVersion: l.HaVersion})
			}
		}
	}
}

func (e *evaluator) logs(rep *Report) {
	for _, l := range e.in.Lines {
		if (l.Kind != "check" && l.Kind != "baseline") || l.Result == "pass" || l.Result == "fail_as_expected" {
			continue
		}
		log := Log{Seq: l.Seq, Target: targetString(l.Target), Result: l.Result, Path: l.OutputPath}
		if !validResult(&l, e.rules) {
			log.Result, log.Reason = "invalid", "record_invalid"
		} else if e.rules == RulesV3 && l.CriterionKind == bundle.KindChange && l.Target != nil {
			row := e.in.Bundle.Row(l.Target.Criterion)
			if row != nil && evidence.ValidateStored(l.Evidence, row.Evidence, row.EvidenceDigest, l.Kind == "baseline", l.Result, l.Exit) == nil {
				log.Reason = l.Evidence.Reason
			} else {
				log.Reason = "evidence_invalid"
			}
		}
		rep.Logs = append(rep.Logs, log)
	}
}

func targetString(t *record.Target) string {
	switch {
	case t == nil:
		return ""
	case t.Criterion != "":
		return t.Criterion
	default:
		return fmt.Sprintf("%s#%d", t.Task, t.Index)
	}
}

// DoneEntry builds the `done` line for this report. recordHead is the hash of
// the last line before it ("" when there is none).
func (rep *Report) DoneEntry(haVersion string) *record.Done {
	d := &record.Done{
		Header:         record.Header{Kind: "done", HaVersion: haVersion},
		Status:         rep.Status,
		Reasons:        []string{},
		Commit:         rep.Commit,
		TreeClean:      rep.TreeClean,
		SpecDigest:     rep.SpecDigest,
		Criteria:       []record.CriterionResult{},
		CommandDigests: map[string]string{},
	}
	if rep.RecordHead != nil {
		d.RecordHead = rep.RecordHead.SHA256
	}
	for _, r := range rep.Reasons {
		s := r.Code
		if r.Criterion != "" {
			s = r.Criterion + ":" + r.Code
		}
		d.Reasons = append(d.Reasons, s)
	}
	for _, c := range rep.Criteria {
		d.Criteria = append(d.Criteria, record.CriterionResult{ID: c.ID, Kind: c.Kind, Satisfied: c.Satisfied, Records: c.Records})
	}
	for id, v := range rep.commandDigests {
		d.CommandDigests[id] = v
	}
	return d
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
