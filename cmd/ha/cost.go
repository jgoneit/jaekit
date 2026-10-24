package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/reviewseries"
	"github.com/jgoneit/jaekit/internal/status"
)

// ha cost summarizes what one goal cost in verification and review
// (run-record.md §1.7). It reads the goal's run record and documents, Git,
// and the Spar review series folders the person names with --series, in the
// public review-round/v1 and review-gate/v1 forms. It writes nothing, takes
// no lock, never reads the current time and calls no model, so the same
// records, Git and arguments give the same output. It never compares the
// goal with another goal or judges the numbers.

const costSchema = "cost/v1"

// Why a value or section of the summary is absent.
const (
	costNotStarted    = "not_started"    // the goal has no start record
	costNotGiven      = "not_given"      // no review series was named
	costNotRecorded   = "not_recorded"   // no record holds the value
	costCannotCompare = "cannot_compare" // Git cannot compare the base with HEAD
	costNotComputed   = "not_computed"   // the record's budget cannot be computed
	costNotFinished   = "not_finished"   // the round has no finished_at
)

type costDoc struct {
	Schema       string            `json:"schema"`
	Goal         string            `json:"goal"`
	Kind         costKind          `json:"kind"`
	Verification *costVerification `json:"verification"`
	Questions    costQuestions     `json:"questions"`
	Review       *costReview       `json:"review"`
	Findings     *costFindings     `json:"findings"`
	Tokens       costTokens        `json:"tokens"`
	// Unavailable names each null value above with why it is absent.
	Unavailable map[string]string `json:"unavailable"`
}

type costKind struct {
	Assurance        *string `json:"assurance"`
	Rules            *string `json:"rules"`
	RequiredCriteria int     `json:"required_criteria"`
	OptionalCriteria int     `json:"optional_criteria"`
	Base             *string `json:"base"`
	ChangedFiles     *int    `json:"changed_files"`
	ChangedCommits   *int    `json:"changed_commits"`
	CompletionRecord *int    `json:"completion_record"`
}

type costVerification struct {
	Runs       map[string]map[string]int `json:"runs"`
	DurationMS costDuration              `json:"duration_ms"`
	Targets    []costTarget              `json:"targets"`
	Rerecords  []costRerecord            `json:"rerecords"`
	Notes      costNotes                 `json:"notes"`
	Span       costSpan                  `json:"span"`
	Budget     *costBudget               `json:"budget"`
}

type costDuration struct {
	Check    int64 `json:"check"`
	Baseline int64 `json:"baseline"`
	Total    int64 `json:"total"`
}

type costTarget struct {
	Target    string `json:"target"`
	Checks    int    `json:"checks"`
	Baselines int    `json:"baselines"`
}

// costRerecord is a run of a target that an earlier run of the same kind
// already recorded: the inputs that changed since that run, or none.
type costRerecord struct {
	Target         string   `json:"target"`
	Kind           string   `json:"kind"`
	Seq            int      `json:"seq"`
	PreviousSeq    int      `json:"previous_seq"`
	Changed        []string `json:"changed"`
	PreviousResult string   `json:"previous_result"`
}

// costNotes counts note kinds. The three run-rules/4 kinds are nil for a
// goal started under earlier rules, which cannot record them.
type costNotes struct {
	TestChange    *int `json:"test_change"`
	Recover       *int `json:"recover"`
	AllowRecovery *int `json:"allow_recovery"`
	Reopen        int  `json:"reopen"`
}

type costSpan struct {
	FirstSeq int    `json:"first_seq"`
	FirstAt  string `json:"first_at"`
	LastSeq  int    `json:"last_seq"`
	LastAt   string `json:"last_at"`
	Seconds  *int64 `json:"seconds"`
}

// costBudget is the budget window ha status shows.
type costBudget struct {
	WindowStart         int    `json:"window_start"`
	SpanSeconds         *int64 `json:"span_seconds"`
	ReportedWaitSeconds *int64 `json:"reported_wait_seconds"`
	ElapsedSeconds      int64  `json:"elapsed_seconds"`
	ElapsedLimitSeconds int64  `json:"elapsed_limit_seconds"`
	Runs                int    `json:"runs"`
	RunsLimit           int    `json:"runs_limit"`
}

type costQuestions struct {
	BySource            costSources       `json:"by_source"`
	Waits               []status.WaitSpan `json:"waits"`
	ReportedWaitSeconds *int64            `json:"reported_wait_seconds"`
	InputNotes          *int              `json:"input_notes"`
	ConfirmNotes        *int              `json:"confirm_notes"`
	RequirementQuestion []costQuestion    `json:"requirement_questions"`
	Extensions          []costExtension   `json:"extensions"`
	Waivers             []costWaiver      `json:"waivers"`
	sourcesGiven        bool              // review series were named
	notStarted          bool              // the goal has no start record
}

type costSources struct {
	Wait                *int `json:"wait"`
	RequirementQuestion *int `json:"requirement_question"`
	Extension           *int `json:"extension"`
	Waiver              *int `json:"waiver"`
}

type costQuestion struct {
	Series      string  `json:"series"`
	Round       string  `json:"round"`
	Finding     string  `json:"finding"`
	Disposition *string `json:"disposition"`
	UserQuote   *string `json:"user_quote"`
}

type costExtension struct {
	Series    string `json:"series"`
	Stage     string `json:"stage"`
	By        int    `json:"by"`
	UserQuote string `json:"user_quote"`
}

type costWaiver struct {
	Series    string `json:"series"`
	Round     string `json:"round"`
	Reviewer  string `json:"reviewer"`
	UserQuote string `json:"user_quote"`
}

type costReview struct {
	Series          []costSeries        `json:"series"`
	Stages          []costStage         `json:"stages"`
	Rounds          []costRound         `json:"rounds"`
	ReviewerSeconds []costReviewerTotal `json:"reviewer_seconds"`
}

type costSeries struct {
	Path    string                 `json:"path"`
	Rounds  int                    `json:"rounds"`
	DryRuns int                    `json:"dry_runs"`
	Gates   []costGate             `json:"gates"`
	Skipped []reviewseries.Skipped `json:"skipped"`
}

type costGate struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	State  string `json:"state"`
	// RecordedSeq is the last review note whose gate digest is this file's.
	RecordedSeq *int `json:"recorded_seq"`
}

type costStage struct {
	Stage          string `json:"stage"`
	Counted        int    `json:"counted"`
	Reruns         int    `json:"reruns"`
	CountedUnknown int    `json:"counted_not_recorded"`
}

type costRound struct {
	Series     string              `json:"series"`
	Round      string              `json:"round"`
	Stage      string              `json:"stage"`
	Sequence   int                 `json:"sequence"`
	Counted    *bool               `json:"counted"`
	RerunOf    []string            `json:"rerun_of"`
	StartedAt  *string             `json:"started_at"`
	FinishedAt *string             `json:"finished_at"`
	Seconds    *int64              `json:"seconds"`
	Reviewers  []costRoundReviewer `json:"reviewers"`
}

type costRoundReviewer struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	ElapsedSeconds *float64 `json:"elapsed_seconds"`
	Attempts       *int     `json:"attempts"`
	FallbackUsed   *bool    `json:"fallback_used"`
}

type costReviewerTotal struct {
	Reviewer string  `json:"reviewer"`
	Seconds  float64 `json:"seconds"`
	Rounds   int     `json:"rounds"`
	// ms sums the rounds' recorded seconds as whole milliseconds, the
	// precision Spar records, so the sum does not depend on float rounding.
	ms int64
	// NotRecorded counts the rounds whose record has no elapsed_seconds for
	// this reviewer; they are not in Seconds.
	NotRecorded int `json:"not_recorded"`
}

type costFindings struct {
	Classes     []costClass  `json:"classes"`
	Growth      []costGrowth `json:"growth"`
	GrowthTotal *costLines   `json:"growth_total"`
}

type costClass struct {
	Class               string           `json:"class"`
	Total               int              `json:"total"`
	CheckPaths          int              `json:"check_paths"`
	Other               int              `json:"other"`
	Unreadable          int              `json:"unreadable"`
	LocationNotRecorded int              `json:"location_not_recorded"`
	Dispositions        costDispositions `json:"dispositions"`
}

type costDispositions struct {
	Fix       int `json:"fix"`
	Reject    int `json:"reject"`
	Defer     int `json:"defer"`
	AutoDefer int `json:"auto_defer"`
	Duplicate int `json:"duplicate"`
	Answer    int `json:"answer"`
	None      int `json:"none"`
}

type costLines struct {
	Check int64 `json:"check"`
	Other int64 `json:"other"`
}

// costGrowth is the lines one series' fixes rounds added: 0 without a fixes
// round, else the growth of the series' review-gate/v1 file.
type costGrowth struct {
	Series string     `json:"series"`
	Lines  *costLines `json:"lines"`
	Source string     `json:"source"`
	Gate   *string    `json:"gate"`
}

// Growth sources.
const (
	growthNoFixes   = "no_fixes_round"
	growthGate      = "gate"
	growthNoGate    = "no_gate_file"
	growthNotInGate = "gate_without_growth"
	growthMisses    = "gate_misses_rounds"
)

type costTokens struct {
	Input       *int64 `json:"input"`
	CachedInput *int64 `json:"cached_input"`
	Output      *int64 `json:"output"`
}

func (c *cli) cost(in []string) int {
	a, err := parseFlags(in, []string{"lang", "format"}, []string{"series"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	lang, given := a.vals["lang"]
	if !given {
		lang = "en"
	}
	words, ok := costLanguages[lang]
	if !ok {
		return c.fail(exitUsage, "--lang must be ko or en")
	}
	format, given := a.vals["format"]
	if !given {
		format = "md"
	}
	if format != "md" && format != "json" {
		return c.fail(exitUsage, "--format must be md or json")
	}
	if len(a.pos) != 1 {
		return c.fail(exitUsage, "usage: ha cost <goal> [--series <folder>]... [--format md|json] [--lang ko|en]")
	}
	seen := map[string]string{}
	for _, s := range a.lists["series"] {
		if s == "" {
			return c.fail(exitUsage, "--series needs a folder")
		}
		key, err := filepath.Abs(s)
		if err != nil {
			return c.fail(exitUsage, "--series %s: %v", s, err)
		}
		if real, err := filepath.EvalSymlinks(key); err == nil {
			key = real
		}
		if first, dup := seen[key]; dup {
			return c.fail(exitUsage, "--series %s names the same folder as --series %s", s, first)
		}
		seen[key] = s
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitRefused, "%s cannot be read: %v", w.records, err)
	}
	if bad != nil {
		return c.fail(exitRefused, "%s: %v; nothing is summarized", w.records, bad)
	}
	var series []*reviewseries.Series
	for _, path := range a.lists["series"] {
		s, err := reviewseries.Read(path)
		if err != nil {
			return c.fail(exitRefused, "%v; nothing is summarized", err)
		}
		series = append(series, s)
	}
	doc, err := c.costSummary(w, lines, series)
	var ur *status.UnsupportedRulesError
	if errors.As(err, &ur) {
		return c.fail(exitRules, "%v", err)
	}
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	var out []byte
	if format == "json" {
		if out, err = json.MarshalIndent(doc, "", "  "); err != nil {
			return c.fail(exitInternal, "%v", err)
		}
		out = append(out, '\n')
	} else {
		out = []byte(renderCost(doc, words))
	}
	if _, err := c.out.Write(out); err != nil {
		return c.fail(exitInternal, "write summary: %v", err)
	}
	return exitOK
}

func (c *cli) costSummary(w *workspace, lines []record.Line, series []*reviewseries.Series) (*costDoc, error) {
	doc := &costDoc{Schema: costSchema, Goal: w.goal.Dir, Unavailable: map[string]string{}}
	g := w.goal
	if g.Assurance != "" {
		tier := g.Assurance
		doc.Kind.Assurance = &tier
	} else {
		doc.Unavailable["kind.assurance"] = "none"
	}
	for _, cr := range g.Criteria {
		if cr.Optional {
			doc.Kind.OptionalCriteria++
		} else {
			doc.Kind.RequiredCriteria++
		}
	}
	start := startLine(lines)
	if start == nil {
		for _, key := range []string{"kind.rules", "kind.base", "kind.changed_files", "kind.changed_commits", "kind.completion_record",
			"verification", "questions.waits", "questions.reported_wait_seconds", "questions.input_notes", "questions.confirm_notes",
			"questions.by_source.wait"} {
			doc.Unavailable[key] = costNotStarted
		}
		doc.Questions.notStarted = true
	} else {
		if err := costKindFromRecord(w, lines, start, doc); err != nil {
			return nil, err
		}
		doc.Verification = costVerify(lines, doc)
		costWaits(lines, doc)
	}
	if len(series) == 0 {
		for _, key := range []string{"review", "findings", "questions.by_source.requirement_question", "questions.by_source.extension",
			"questions.by_source.waiver", "questions.requirement_questions", "questions.extensions", "questions.waivers"} {
			doc.Unavailable[key] = costNotGiven
		}
	} else {
		costSeriesPart(lines, series, doc)
	}
	for _, key := range []string{"tokens.input", "tokens.cached_input", "tokens.output"} {
		doc.Unavailable[key] = costNotRecorded
	}
	return doc, nil
}

// costKindFromRecord fills the rules, the changed files and commits since
// the base, as ha report's Changed counts them, and the valid completion
// record. When Git cannot compare the base with HEAD, those two say so and
// the values read from the record stay.
func costKindFromRecord(w *workspace, lines []record.Line, start *record.Line, doc *costDoc) error {
	rules, base := start.Rules, start.BaseCommit
	doc.Kind.Rules, doc.Kind.Base = &rules, &base
	rep, err := status.Compute(status.Input{Repo: w.repo, Goal: w.goal, Bundle: w.bundle, Lines: lines, HaVersion: version})
	var rangeErr *status.RangeError
	var unsupported *status.UnsupportedRulesError
	switch {
	case errors.As(err, &unsupported):
		return err
	case errors.As(err, &rangeErr):
		for _, key := range []string{"kind.changed_files", "kind.changed_commits", "kind.completion_record"} {
			doc.Unavailable[key] = costCannotCompare
		}
		return nil
	case err != nil:
		// The state needs Git to compare recorded commits with HEAD, as in
		// a clone without them; the record's own values still count.
		doc.Unavailable["kind.completion_record"] = costNotComputed
	case rep.CompletionRecord != nil:
		seq := *rep.CompletionRecord
		doc.Kind.CompletionRecord = &seq
	default:
		doc.Unavailable["kind.completion_record"] = "none"
	}
	head, err := w.repo.Head()
	if err == nil {
		var excl []string
		if excl, err = w.excludes(start.Rules); err == nil {
			var files []string
			changes, cerr := w.repo.Changes(base, head, excl)
			err = cerr
			if err == nil {
				for _, ch := range changes {
					files = append(files, ch.Path)
				}
				var commits int
				if commits, err = w.repo.CommitsChanging(base, head, excl); err == nil {
					n := len(files)
					doc.Kind.ChangedFiles, doc.Kind.ChangedCommits = &n, &commits
				}
			}
		}
	}
	if err != nil {
		doc.Unavailable["kind.changed_files"] = costCannotCompare
		doc.Unavailable["kind.changed_commits"] = costCannotCompare
	}
	return nil
}

// costTargetName is a run's target as ha timing and ha check name it.
func costTargetName(t *record.Target) string {
	switch {
	case t == nil:
		return "—"
	case t.Criterion != "":
		return t.Criterion
	default:
		return fmt.Sprintf("%s#%d", t.Task, t.Index)
	}
}

func costVerify(lines []record.Line, doc *costDoc) *costVerification {
	v := &costVerification{Runs: map[string]map[string]int{"check": {}, "baseline": {}}, Targets: []costTarget{}, Rerecords: []costRerecord{}}
	var testChange, recover, allowRecovery int
	if start := startLine(lines); status.Autonomous(start.Rules) {
		v.Notes.TestChange, v.Notes.Recover, v.Notes.AllowRecovery = &testChange, &recover, &allowRecovery
	} else {
		for _, key := range []string{"test_change", "recover", "allow_recovery"} {
			doc.Unavailable["verification.notes."+key] = costNotRecorded
		}
	}
	exec := record.ExecutionLines(lines)
	targets := map[string]*costTarget{}
	var order []string
	last := map[string]*record.Line{}
	for i := range exec {
		l := &exec[i]
		switch l.Kind {
		case "note":
			switch l.Note {
			case "test-change":
				testChange++
			case "recover":
				recover++
			case "allow-recovery":
				allowRecovery++
			case "reopen":
				v.Notes.Reopen++
			}
			continue
		case "check", "baseline":
		default:
			continue
		}
		v.Runs[l.Kind][l.Result]++
		if l.Kind == "check" {
			v.DurationMS.Check += l.DurationMS
		} else {
			v.DurationMS.Baseline += l.DurationMS
		}
		name := costTargetName(l.Target)
		t := targets[name]
		if t == nil {
			t = &costTarget{Target: name}
			targets[name] = t
			order = append(order, name)
		}
		if l.Kind == "check" {
			t.Checks++
		} else {
			t.Baselines++
		}
		key := l.Kind + "\x00" + name
		if prev := last[key]; prev != nil {
			v.Rerecords = append(v.Rerecords, costRerecord{Target: name, Kind: l.Kind, Seq: l.Seq, PreviousSeq: prev.Seq,
				Changed: costChangedInputs(prev, l), PreviousResult: prev.Result})
		}
		last[key] = l
	}
	v.DurationMS.Total = v.DurationMS.Check + v.DurationMS.Baseline
	for _, name := range order {
		v.Targets = append(v.Targets, *targets[name])
	}
	first, final := exec[0], exec[len(exec)-1]
	v.Span = costSpan{FirstSeq: first.Seq, FirstAt: first.At, LastSeq: final.Seq, LastAt: final.At}
	from, err1 := time.Parse(time.RFC3339, first.At)
	to, err2 := time.Parse(time.RFC3339, final.At)
	if err1 == nil && err2 == nil {
		s := int64(to.Sub(from) / time.Second)
		v.Span.Seconds = &s
	} else {
		doc.Unavailable["verification.span.seconds"] = costNotRecorded
	}
	bud, err := status.RecordBudget(lines)
	if err != nil {
		doc.Unavailable["verification.budget"] = costNotComputed
		return v
	}
	v.Budget = &costBudget{WindowStart: bud.WindowStart, SpanSeconds: bud.SpanSeconds, ReportedWaitSeconds: bud.ReportedWaitSeconds,
		ElapsedSeconds: bud.ElapsedSeconds, ElapsedLimitSeconds: bud.ElapsedLimitSeconds, Runs: bud.Runs, RunsLimit: bud.RunsLimit}
	if bud.SpanSeconds == nil {
		// Before run-rules/4 no wait is reported: the window span is the
		// elapsed time.
		doc.Unavailable["verification.budget.span_seconds"] = costNotRecorded
		doc.Unavailable["verification.budget.reported_wait_seconds"] = costNotRecorded
	}
	return v
}

// costChangedInputs names the inputs of run l that differ from the earlier
// run prev of the same target and kind: the code (the commit a check ran on,
// the base a baseline ran on), the goal documents, the verification command
// and, for a baseline, the check files it overlaid.
func costChangedInputs(prev, l *record.Line) []string {
	changed := []string{}
	code := prev.Commit != l.Commit
	if l.Kind == "baseline" {
		code = prev.BaseCommit != l.BaseCommit
	}
	if code {
		changed = append(changed, "code")
	}
	if prev.SpecDigest != l.SpecDigest {
		changed = append(changed, "goal_documents")
	}
	if prev.CommandDigest != l.CommandDigest {
		changed = append(changed, "command")
	}
	if l.Kind == "baseline" && costOverlay(prev) != costOverlay(l) {
		changed = append(changed, "overlay")
	}
	return changed
}

func costOverlay(l *record.Line) string {
	items := make([]string, len(l.Overlay))
	for i, o := range l.Overlay {
		items[i] = o.Path + "\x00" + o.Digest
	}
	sort.Strings(items)
	data, _ := json.Marshal(items)
	return string(data)
}

// costWaits fills the reported waits and the input and confirm notes. A goal
// started before run-rules/4 cannot report waits, so they are not recorded.
func costWaits(lines []record.Line, doc *costDoc) {
	q := &doc.Questions
	if status.Autonomous(startLine(lines).Rules) {
		q.Waits = status.WaitSpans(lines)
		total := status.ReportedWaitSeconds(lines)
		n := len(q.Waits)
		q.ReportedWaitSeconds, q.BySource.Wait = &total, &n
	} else {
		for _, key := range []string{"questions.waits", "questions.reported_wait_seconds", "questions.by_source.wait"} {
			doc.Unavailable[key] = costNotRecorded
		}
	}
	var input, confirm int
	for _, l := range record.ExecutionLines(lines) {
		if l.Kind != "note" {
			continue
		}
		switch l.Note {
		case "input":
			input++
		case "confirm":
			confirm++
		}
	}
	q.InputNotes, q.ConfirmNotes = &input, &confirm
}

// costSeriesPart fills the review rounds, the findings and their
// dispositions, the growth, and the questions the series hold.
func costSeriesPart(lines []record.Line, series []*reviewseries.Series, doc *costDoc) {
	rev := &costReview{Series: []costSeries{}, Rounds: []costRound{}, ReviewerSeconds: []costReviewerTotal{}}
	fnd := &costFindings{Growth: []costGrowth{}}
	q := &doc.Questions
	q.sourcesGiven = true
	q.RequirementQuestion, q.Extensions, q.Waivers = []costQuestion{}, []costExtension{}, []costWaiver{}
	stages := map[string]*costStage{}
	for _, st := range reviewseries.Stages {
		stages[st] = &costStage{Stage: st}
	}
	classes := map[string]*costClass{}
	for _, cl := range reviewseries.Classes {
		classes[cl] = &costClass{Class: cl}
	}
	reviewerTotals := map[string]*costReviewerTotal{}
	recorded := map[string]int{}
	for _, l := range lines {
		if l.Kind == "note" && l.Note == "review" && l.Review != nil {
			recorded[l.Review.GateDigest] = l.Seq
		}
	}
	growthComplete := true
	total := costLines{}
	for _, s := range series {
		cs := costSeries{Path: s.Path, Gates: []costGate{}, Skipped: s.Skipped}
		for _, gt := range s.Gates {
			cg := costGate{Name: gt.Name, Digest: gt.Digest, State: gt.State}
			if seq, ok := recorded[gt.Digest]; ok {
				cg.RecordedSeq = &seq
			}
			cs.Gates = append(cs.Gates, cg)
		}
		var fixes []string
		for _, r := range s.Rounds {
			if r.DryRun {
				cs.DryRuns++
				continue
			}
			cs.Rounds++
			if r.Stage == "fixes" && r.HasFindings {
				fixes = append(fixes, r.Base+".."+r.Head)
			}
			st := stages[r.Stage]
			switch {
			case r.Counted == nil:
				st.CountedUnknown++
			case *r.Counted:
				st.Counted++
			default:
				st.Reruns++
			}
			rev.Rounds = append(rev.Rounds, costRoundOf(s.Path, r, doc))
			for _, rv := range r.Reviewers {
				t := reviewerTotals[rv.Name]
				if t == nil {
					t = &costReviewerTotal{Reviewer: rv.Name}
					reviewerTotals[rv.Name] = t
				}
				t.Rounds++
				if rv.ElapsedSeconds == nil {
					t.NotRecorded++
				} else {
					t.ms += int64(math.Round(*rv.ElapsedSeconds * 1000))
				}
			}
			final := map[string]reviewseries.LedgerEntry{}
			for _, e := range r.Ledger {
				final[e.Finding] = e
			}
			for _, f := range r.Findings {
				cl := classes[f.Class]
				cl.Total++
				switch {
				case f.LocationUnreadable:
					cl.Unreadable++
				case f.InCheckPaths == nil:
					cl.LocationNotRecorded++
				case *f.InCheckPaths:
					cl.CheckPaths++
				default:
					cl.Other++
				}
				e, done := final[f.ID]
				d := &cl.Dispositions
				switch {
				case !done:
					d.None++
				case e.Disposition == "fix":
					d.Fix++
				case e.Disposition == "reject":
					d.Reject++
				case e.Disposition == "defer" && e.Auto:
					d.AutoDefer++
				case e.Disposition == "defer":
					d.Defer++
				case e.Disposition == "duplicate":
					d.Duplicate++
				case e.Disposition == "answer":
					d.Answer++
				}
				if f.Class == "requirement-question" {
					cq := costQuestion{Series: s.Path, Round: r.Name, Finding: f.ID}
					if done {
						disp := e.Disposition
						cq.Disposition = &disp
						if e.UserQuote != "" {
							quote := e.UserQuote
							cq.UserQuote = &quote
						}
					}
					q.RequirementQuestion = append(q.RequirementQuestion, cq)
				}
			}
			for _, wv := range r.Waivers {
				q.Waivers = append(q.Waivers, costWaiver{Series: s.Path, Round: r.Name, Reviewer: wv.Reviewer, UserQuote: wv.UserQuote})
			}
		}
		for _, e := range s.Extensions {
			q.Extensions = append(q.Extensions, costExtension{Series: s.Path, Stage: e.Stage, By: e.By, UserQuote: e.UserQuote})
		}
		gr := costSeriesGrowth(s, fixes, recorded)
		if gr.Lines == nil {
			growthComplete = false
		} else {
			total.Check += gr.Lines.Check
			total.Other += gr.Lines.Other
		}
		fnd.Growth = append(fnd.Growth, gr)
		rev.Series = append(rev.Series, cs)
	}
	for _, st := range reviewseries.Stages {
		rev.Stages = append(rev.Stages, *stages[st])
	}
	names := make([]string, 0, len(reviewerTotals))
	for n := range reviewerTotals {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t := reviewerTotals[n]
		t.Seconds = float64(t.ms) / 1000
		rev.ReviewerSeconds = append(rev.ReviewerSeconds, *t)
	}
	for _, cl := range reviewseries.Classes {
		fnd.Classes = append(fnd.Classes, *classes[cl])
	}
	if growthComplete {
		fnd.GrowthTotal = &total
	} else {
		doc.Unavailable["findings.growth_total"] = costNotRecorded
	}
	rq, ext, wv := len(q.RequirementQuestion), len(q.Extensions), len(q.Waivers)
	q.BySource.RequirementQuestion, q.BySource.Extension, q.BySource.Waiver = &rq, &ext, &wv
	doc.Review, doc.Findings = rev, fnd
}

func costRoundOf(series string, r reviewseries.Round, doc *costDoc) costRound {
	cr := costRound{Series: series, Round: r.Name, Stage: r.Stage, Sequence: r.Sequence, Counted: r.Counted, RerunOf: r.RerunOf,
		Reviewers: []costRoundReviewer{}}
	if r.StartedAt != "" {
		at := r.StartedAt
		cr.StartedAt = &at
	}
	if r.FinishedAt != "" {
		at := r.FinishedAt
		cr.FinishedAt = &at
	}
	from, err1 := time.Parse(time.RFC3339, r.StartedAt)
	to, err2 := time.Parse(time.RFC3339, r.FinishedAt)
	switch {
	case r.FinishedAt == "":
		cr.Seconds = nil
	case err1 == nil && err2 == nil && !to.Before(from):
		s := int64(to.Sub(from) / time.Second)
		cr.Seconds = &s
	}
	for _, rv := range r.Reviewers {
		cr.Reviewers = append(cr.Reviewers, costRoundReviewer{Name: rv.Name, Status: rv.Status, ElapsedSeconds: rv.ElapsedSeconds,
			Attempts: rv.Attempts, FallbackUsed: rv.FallbackUsed})
	}
	return cr
}

// costSeriesGrowth is 0 for a series without a fixes round. Otherwise it is
// the growth of a review-gate/v1 file of the series whose measured commit
// ranges (growth.rounds) are exactly the ranges of the series' fixes rounds,
// since a gate adds up only the ranges it measured and counts a range once
// (contracts/review-gate.md §3); among several such files, the one a review
// note recorded last, else the first by name. fixes holds "<base>..<head>".
func costSeriesGrowth(s *reviewseries.Series, fixes []string, recorded map[string]int) costGrowth {
	gr := costGrowth{Series: s.Path}
	if len(fixes) == 0 {
		gr.Lines, gr.Source = &costLines{}, growthNoFixes
		return gr
	}
	if len(s.Gates) == 0 {
		gr.Source = growthNoGate
		return gr
	}
	want := append([]string(nil), fixes...)
	sort.Strings(want)
	want = slices.Compact(want)
	var chosen *reviewseries.Gate
	best := 0
	for i := range s.Gates {
		g := &s.Gates[i]
		if g.Measured == nil || !slices.Equal(g.Measured, want) {
			continue
		}
		if seq := recorded[g.Digest]; chosen == nil || seq > best {
			chosen, best = g, seq
		}
	}
	if chosen == nil {
		gr.Source = growthMisses
		if len(s.Gates) == 1 {
			name := s.Gates[0].Name
			gr.Gate = &name
		}
		return gr
	}
	name := chosen.Name
	gr.Gate = &name
	if chosen.Growth == nil {
		gr.Source = growthNotInGate
		return gr
	}
	gr.Lines, gr.Source = &costLines{Check: chosen.Growth.Check, Other: chosen.Growth.Other}, growthGate
	return gr
}
