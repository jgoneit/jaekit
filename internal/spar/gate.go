package spar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Gate states and their exit codes.
const (
	StateComplete = "complete"
	StateDegraded = "degraded"
	StateBlocked  = "blocked"
)

// GateRound is one round as the gate file lists it.
type GateRound struct {
	Path               string   `json:"path"`
	Stage              string   `json:"stage"`
	Sequence           int      `json:"sequence"`
	Base               string   `json:"base"`
	Head               string   `json:"head"`
	CriteriaDigest     string   `json:"criteria_digest"`
	PacketDigest       string   `json:"packet_digest"`
	ConfigDigest       string   `json:"config_digest"`
	RequestedReviewers []string `json:"requested_reviewers"`
	RequiredReviewers  int      `json:"required_reviewers"`
	CompletedRequired  int      `json:"completed_required"`
	DryRun             bool     `json:"dry_run"`
	Counted            bool     `json:"counted"`
	// WaivedReviewers are the required reviewers the user's decision in
	// this round covers; a round without one has no such field.
	WaivedReviewers []GateWaiver `json:"waived_reviewers,omitempty"`
}

// GateWaiver is a required reviewer a round went on without, by the user's
// recorded decision.
type GateWaiver struct {
	Name       string `json:"name"`
	Preset     string `json:"preset"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	UserQuote  string `json:"user_quote"`
	RecordedAt string `json:"recorded_at"`
}

// Growth is what review fix commits added, split by check paths.
type Growth struct {
	AddedLines    map[string]int `json:"added_lines"`
	OverHardening bool           `json:"over_hardening"`
	Rounds        []GrowthRound  `json:"rounds"`
}

// GrowthRound is one fixes round's added lines.
type GrowthRound struct {
	Path  string `json:"path"`
	Base  string `json:"base"`
	Head  string `json:"head"`
	Check int    `json:"check"`
	Other int    `json:"other"`
}

// GateRecord is the review-gate/v1 file.
type GateRecord struct {
	Schema             string         `json:"schema"`
	SparVersion        string         `json:"spar_version"`
	State              string         `json:"state"`
	Reasons            []string       `json:"reasons"`
	ExpectHead         string         `json:"expect_head"`
	LastReviewedHead   string         `json:"last_reviewed_head"`
	PreviousGateDigest *string        `json:"previous_gate_digest"`
	Rounds             []GateRound    `json:"rounds"`
	Open               map[string]int `json:"open"`
	OpenFindings       []string       `json:"open_findings"`
	AutoDeferred       int            `json:"auto_deferred"`
	Growth             Growth         `json:"growth"`
	WrittenAt          string         `json:"written_at"`
}

// GateOptions are the inputs of `spar gate`.
type GateOptions struct {
	Rounds     []string
	ExpectHead string
	Out        string
	Repo       string
	Version    string
}

type loadedRound struct {
	path     string
	round    *RoundRecord
	findings []Finding
	ledger   []LedgerEntry
	waivers  []Waiver
}

// Gate decides from round records whether the review of a head is complete,
// and writes the review-gate/v1 file in every state.
func Gate(o GateOptions) (int, error) {
	if len(o.Rounds) == 0 || o.ExpectHead == "" || o.Out == "" {
		return ExitUsage, usageError("spar gate needs round folders, --expect-head and --out")
	}
	start := o.Repo
	if start == "" {
		start = "."
	}
	repo, err := repoRoot(start)
	if err != nil {
		return ExitRefused, refused("%s is not in a Git repository: %v", start, err)
	}
	repo = realish(repo)
	out := realish(o.Out)
	if err := localRecordPath(repo, out); err != nil {
		return ExitRefused, refused("--out %v", err)
	}
	// The gate file never replaces an input of the gate (G28).
	for _, p := range o.Rounds {
		dir := realish(p)
		if insideRepo(dir, out) != "" {
			return ExitRefused, refused("--out %s is inside the given round %s", o.Out, p)
		}
		if out == filepath.Join(filepath.Dir(dir), "extensions.jsonl") {
			return ExitRefused, refused("--out %s is the extensions.jsonl of the series of %s", o.Out, p)
		}
	}
	g := &GateRecord{Schema: GateSchema, SparVersion: o.Version, Reasons: []string{}, Rounds: []GateRound{},
		Open: map[string]int{"defect": 0, "requirement-question": 0}, OpenFindings: []string{},
		Growth: Growth{AddedLines: map[string]int{"check": 0, "other": 0}, Rounds: []GrowthRound{}}}
	if previous, err := os.ReadFile(out); err == nil {
		// An existing file is replaced only when it is an earlier gate.
		var probe struct {
			Schema string `json:"schema"`
		}
		if json.Unmarshal(previous, &probe) != nil || probe.Schema != GateSchema {
			return ExitRefused, refused("--out %s exists and is not a %s file", o.Out, GateSchema)
		}
		sum := sha256.Sum256(previous)
		digest := hex.EncodeToString(sum[:])
		g.PreviousGateDigest = &digest
	} else if !errors.Is(err, os.ErrNotExist) {
		return ExitRefused, refused("--out %s cannot be read: %v", o.Out, err)
	}
	var blocked, degraded []string
	expect, err := resolveCommit(repo, o.ExpectHead)
	if err != nil {
		blocked = append(blocked, "--expect-head: "+err.Error())
		expect = o.ExpectHead
	}
	g.ExpectHead = expect
	var rounds []loadedRound
	for _, p := range o.Rounds {
		dir := realish(p)
		r, err := loadRound(dir)
		if err != nil {
			blocked = append(blocked, err.Error())
			continue
		}
		if missing := missingRecords(dir, r); missing != "" {
			blocked = append(blocked, fmt.Sprintf("%s: %s is missing", p, missing))
			continue
		}
		findings, err := loadFindings(dir)
		if err != nil {
			blocked = append(blocked, err.Error())
			continue
		}
		ledger, err := loadLedger(dir)
		if err != nil {
			blocked = append(blocked, err.Error())
			continue
		}
		var waivers []Waiver
		if !waiversFileTaken(r) {
			if waivers, err = loadWaivers(dir); err != nil {
				blocked = append(blocked, err.Error())
				continue
			}
		}
		rounds = append(rounds, loadedRound{p, r, findings, ledger, waivers})
	}
	// Required reviewers: the round's own and those its configuration makes
	// required, even when --reviewers left them out. Each must complete the
	// packet in some round of the same stage, base, head and packet (a rerun
	// completes the original), as a required reviewer of the same preset, or
	// hold the user's waiver in this round itself.
	completedIn := map[string]bool{}
	identity := func(r *RoundRecord) string {
		return r.Stage + "\x00" + r.Base + "\x00" + r.Head + "\x00" + r.PacketSHA256
	}
	credit := func(r *RoundRecord, name, preset string) string {
		return identity(r) + "\x00" + name + "\x00" + preset
	}
	for _, lr := range rounds {
		for name, st := range lr.round.Reviewers {
			if st.Status == StatusCompleted && st.Required {
				completedIn[credit(lr.round, name, st.Preset)] = true
			}
		}
	}
	var lastCode *RoundRecord
	var lastAny *RoundRecord
	for _, lr := range rounds {
		r := lr.round
		gr := GateRound{Path: lr.path, Stage: r.Stage, Sequence: r.Sequence, Base: r.Base, Head: r.Head,
			CriteriaDigest: r.CriteriaDigest, PacketDigest: r.PacketSHA256, RequestedReviewers: nonNilStrings(r.RequestedReviewers),
			DryRun: r.DryRun, Counted: r.Counted}
		if r.Config != nil {
			gr.ConfigDigest = r.Config.Digest
		}
		required := requiredOf(r)
		gr.RequiredReviewers = len(required)
		for _, ref := range required {
			st := r.Reviewers[ref.Name]
			switch {
			case st != nil && st.Status == StatusCompleted && st.Required && st.Preset == ref.Preset:
				gr.CompletedRequired++
			case completedIn[credit(r, ref.Name, ref.Preset)]:
				gr.CompletedRequired++ // completed by a rerun of the same packet
			case waiverFor(lr.waivers, ref) != nil:
				waiver := waiverFor(lr.waivers, ref)
				status, reason := waivedStatus(r, ref)
				gr.WaivedReviewers = append(gr.WaivedReviewers, GateWaiver{Name: ref.Name, Preset: ref.Preset,
					Status: status, Reason: reason, UserQuote: waiver.UserQuote, RecordedAt: waiver.RecordedAt})
			case st == nil || st.Preset != ref.Preset:
				degraded = append(degraded, fmt.Sprintf("%s: required reviewer %s (%s) did not run in this round", lr.path, ref.Name, ref.Preset))
			default:
				degraded = append(degraded, fmt.Sprintf("%s: required reviewer %s is %s", lr.path, ref.Name, st.Status))
			}
		}
		if len(required) == 0 {
			degraded = append(degraded, fmt.Sprintf("%s: the round has no required reviewer", lr.path))
		}
		// A waiver never makes a round with no independent review complete.
		if len(gr.WaivedReviewers) > 0 && gr.CompletedRequired == 0 {
			degraded = append(degraded, fmt.Sprintf("%s: no required reviewer completed this round", lr.path))
		}
		if r.DryRun {
			blocked = append(blocked, fmt.Sprintf("%s: a dry run reviewed nothing", lr.path))
		}
		for _, c := range []string{r.Base, r.Head} {
			if _, err := resolveCommit(repo, c); err != nil {
				blocked = append(blocked, fmt.Sprintf("%s: commit %s is not in the repository", lr.path, c))
			}
		}
		g.Rounds = append(g.Rounds, gr)
		lastAny = r
		if stageGroup(r.Stage) == "code" {
			if r.Stage == "fixes" {
				switch {
				case lastCode == nil:
					blocked = append(blocked, fmt.Sprintf("%s: a fixes round needs an earlier code round in the gate list", lr.path))
				case !isRerun(lastCode, r) && lastCode.Head != r.Base:
					blocked = append(blocked, fmt.Sprintf("%s: the fixes chain is broken: base %s is not the previous head %s",
						lr.path, shortID(r.Base), shortID(lastCode.Head)))
				}
			}
			lastCode = r
		}
	}
	switch {
	case lastCode != nil:
		g.LastReviewedHead = lastCode.Head
	case lastAny != nil:
		g.LastReviewedHead = lastAny.Head
	}
	if g.LastReviewedHead != expect {
		blocked = append(blocked, fmt.Sprintf("the expected head %s was not reviewed; the last reviewed head is %s",
			shortID(expect), shortID(g.LastReviewedHead)))
	}
	blocked = append(blocked, capProblems(rounds)...)
	blocked = append(blocked, unlistedRounds(o.Rounds)...)
	for _, lr := range rounds {
		final := finalDispositions(lr.ledger)
		for _, f := range lr.findings {
			e, done := final[f.ID]
			if done && e.Auto && e.Disposition == "defer" {
				g.AutoDeferred++
			}
			if done && e.Disposition == "duplicate" {
				e, done = resolveDuplicate(final, f.ID)
			}
			// A fix must be in the expected head whatever the finding's class.
			if done && e.Disposition == "fix" && expect != "" && e.Commit != expect && !isAncestor(repo, e.Commit, expect) {
				blocked = append(blocked, fmt.Sprintf("%s %s: the fix commit %s is not in the expected head", lr.path, f.ID, shortID(e.Commit)))
			}
			if f.Class != "defect" && f.Class != "requirement-question" {
				continue
			}
			if !done {
				g.Open[f.Class]++
				g.OpenFindings = append(g.OpenFindings, lr.path+" "+f.ID)
			}
		}
	}
	if n := g.Open["defect"] + g.Open["requirement-question"]; n > 0 {
		blocked = append(blocked, fmt.Sprintf("%d open defect or requirement-question finding(s)", n))
	}
	measured := map[string]bool{}
	for _, lr := range rounds {
		r := lr.round
		// A rerun of the same fixes reviews the same commits: count them once.
		if r.Stage != "fixes" || r.DryRun || measured[r.Base+".."+r.Head] {
			continue
		}
		measured[r.Base+".."+r.Head] = true
		added, err := numstat(repo, r.Base, r.Head)
		if err != nil {
			blocked = append(blocked, fmt.Sprintf("%s: added lines cannot be read: %v", lr.path, err))
			continue
		}
		var checkPaths []string
		if r.Config != nil {
			checkPaths = r.Config.CheckPaths
		}
		gr := GrowthRound{Path: lr.path, Base: r.Base, Head: r.Head}
		for _, path := range sortedKeys(added) {
			if matchAny(checkPaths, path) {
				gr.Check += added[path]
			} else {
				gr.Other += added[path]
			}
		}
		g.Growth.AddedLines["check"] += gr.Check
		g.Growth.AddedLines["other"] += gr.Other
		g.Growth.Rounds = append(g.Growth.Rounds, gr)
	}
	g.Growth.OverHardening = g.Growth.AddedLines["check"] > g.Growth.AddedLines["other"]
	code := ExitOK
	switch {
	case len(blocked) > 0:
		g.State, code = StateBlocked, ExitBlocked
	case len(degraded) > 0:
		g.State, code = StateDegraded, ExitDegraded
	default:
		g.State = StateComplete
	}
	g.Reasons = append(append(g.Reasons, blocked...), degraded...)
	g.WrittenAt = now()
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return ExitInternal, err
	}
	if err := writeJSON(out, g); err != nil {
		return ExitInternal, err
	}
	return code, nil
}

// requiredOf lists a round's required reviewers: those it ran as required
// and those its configuration requires.
func requiredOf(r *RoundRecord) []RequiredRef {
	var out []RequiredRef
	seen := map[RequiredRef]bool{}
	add := func(ref RequiredRef) {
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	for _, name := range r.RequiredReviewers {
		if st := r.Reviewers[name]; st != nil {
			add(RequiredRef{name, st.Preset})
		}
	}
	if r.Config != nil {
		for _, ref := range r.Config.ConfiguredRequired {
			add(ref)
		}
	}
	return out
}

// missingRecords names a record file spar always writes that a round lacks.
func missingRecords(dir string, r *RoundRecord) string {
	if r.DryRun {
		return ""
	}
	for _, name := range []string{"findings.json", "ledger.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return name
		}
	}
	return ""
}

// resolveDuplicate follows duplicate dispositions to the finding that
// carries the decision. A chain that ends open or loops stays open.
func resolveDuplicate(final map[string]LedgerEntry, id string) (LedgerEntry, bool) {
	seen := map[string]bool{}
	for {
		e, done := final[id]
		if !done || seen[id] {
			return LedgerEntry{}, false
		}
		if e.Disposition != "duplicate" {
			return e, true
		}
		seen[id] = true
		id = e.Of
	}
}

func nonNilStrings(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// unlistedRounds names the rounds of a given round's series that the gate
// list leaves out (G29). Every review-round/v1 round of the series that is
// not a dry run must be given, so a round with an open finding cannot be
// left out of the decision. A round that stopped before recording any
// finding (no finished_at, no findings.json) is left out like a dry run: it
// holds nothing to decide, and its rerun is another round.
func unlistedRounds(paths []string) []string {
	given := map[string]bool{}
	var series []string
	seen := map[string]bool{}
	for _, p := range paths {
		dir := realish(p)
		given[dir] = true
		if s := filepath.Dir(dir); !seen[s] {
			seen[s] = true
			series = append(series, s)
		}
	}
	var problems []string
	for _, s := range series {
		siblings, err := seriesRounds(s)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: the rounds of the series cannot be listed: %v", s, err))
			continue
		}
		for _, sib := range siblings {
			if sib.round.DryRun || stoppedBeforeFindings(sib) || given[realish(sib.dir)] {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: round %s of the series is not in the gate list", s, filepath.Base(sib.dir)))
		}
	}
	return problems
}

func stoppedBeforeFindings(sib sibling) bool {
	if sib.round.FinishedAt != "" {
		return false
	}
	_, err := os.Stat(filepath.Join(sib.dir, "findings.json"))
	return errors.Is(err, os.ErrNotExist)
}

// capProblems counts the rounds of each series against its caps and
// extensions: the rounds on disk and those given, without double counting.
func capProblems(rounds []loadedRound) []string {
	type key struct{ series, group string }
	counted := map[key]map[string]bool{}
	caps := map[string]Caps{}
	series := map[string]bool{}
	add := func(dir string, r *RoundRecord) {
		if !r.Counted || r.DryRun {
			return
		}
		k := key{r.Series, stageGroup(r.Stage)}
		if counted[k] == nil {
			counted[k] = map[string]bool{}
		}
		counted[k][realish(dir)] = true
	}
	for _, lr := range rounds {
		r := lr.round
		series[r.Series] = true
		if r.Config != nil {
			caps[r.Series] = r.Config.Caps
		}
		add(lr.path, r)
	}
	var problems []string
	for _, s := range sortedKeys(series) {
		siblings, err := seriesRounds(s)
		if err != nil {
			problems = append(problems, fmt.Sprintf("series %s: %v", s, err))
			continue
		}
		// Siblings come in sequence order, so the cap is the one the
		// series' last round applied, whichever rounds were given.
		for _, sib := range siblings {
			add(sib.dir, sib.round)
			if sib.round.Config != nil {
				caps[s] = sib.round.Config.Caps
			}
		}
		exts, err := loadExtensions(s)
		if err != nil {
			problems = append(problems, fmt.Sprintf("series %s: %v", s, err))
			continue
		}
		for _, group := range []string{"plan", "plan_delta", "code"} {
			n := int64(len(counted[key{s, group}]))
			allowed := capFor(caps[s], group) + extensionsFor(exts, group)
			if n > allowed {
				problems = append(problems, fmt.Sprintf("series %s ran %d %s rounds; the cap with recorded extensions is %d",
					s, n, extendStage(group), allowed))
			}
		}
	}
	sort.Strings(problems)
	return problems
}
