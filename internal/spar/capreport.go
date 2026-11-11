package spar

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// CapReportSchema names the JSON form of spar cap-report.
const CapReportSchema = "spar-cap-report/v1"

// The limits of the three finding signals, which the report states with
// them.
const (
	churnLimit     = "Counted by file: a finding in a file the fixes changed counts even when it is about another line."
	recurringLimit = "Counted by link: the same problem reported under different links is not grouped."
	unrelatedLimit = "Uses each reviewer's introduced_by_change as reported."
)

type CapReportOptions struct {
	Series string
	Format string
	Repo   string
	Out    io.Writer
}

type capReport struct {
	Schema      string         `json:"schema"`
	Series      string         `json:"series"`
	Cap         capUse         `json:"cap"`
	Rounds      []capRound     `json:"rounds"`
	Skipped     []capSkipped   `json:"skipped"`
	Open        []capOpen      `json:"open"`
	Churn       []capChurn     `json:"churn"`
	Recurring   []capRecurring `json:"recurring"`
	Unrelated   []capFinding   `json:"unrelated"`
	CheckGrowth capGrowth      `json:"check_growth"`
	Limits      capLimits      `json:"limits"`
}

// capUse is the code cap as spar run applies it. Code and Allowed are null
// when the series has no round, so no configuration names its cap.
type capUse struct {
	Code       *int64 `json:"code"`
	Extensions int64  `json:"extensions"`
	Allowed    *int64 `json:"allowed"`
	Counted    int64  `json:"counted"`
}

type capRound struct {
	Round    string         `json:"round"`
	Stage    string         `json:"stage"`
	Sequence int            `json:"sequence"`
	Base     string         `json:"base"`
	Head     string         `json:"head"`
	Counted  bool           `json:"counted"`
	Classes  map[string]int `json:"classes"`
}

type capSkipped struct {
	Round  string `json:"round"`
	Reason string `json:"reason"`
}

type capOpen struct {
	Round   string `json:"round"`
	Finding string `json:"finding"`
	Class   string `json:"class"`
}

type capFinding struct {
	Round   string `json:"round"`
	Finding string `json:"finding"`
}

type capChurn struct {
	Round    string   `json:"round"`
	Count    int      `json:"count"`
	Findings []string `json:"findings"`
}

type capRecurring struct {
	LinkKind string   `json:"link_kind"`
	Link     string   `json:"link"`
	Rounds   []string `json:"rounds"`
}

type capGrowth struct {
	Check         int  `json:"check"`
	Other         int  `json:"other"`
	OverHardening bool `json:"over_hardening"`
}

type capLimits struct {
	Churn     string `json:"churn"`
	Recurring string `json:"recurring"`
	Unrelated string `json:"unrelated"`
}

// CapReport shows, for the code rounds of a series, the cap use, each
// round, the open findings and the signals of why the rounds ran out. It
// reads only the series' records and Git, writes nothing and never decides a
// cause. An input it cannot read is refused with no report.
func CapReport(o CapReportOptions) (int, error) {
	info, err := os.Stat(o.Series)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return ExitRefused, refused("series %s does not exist", o.Series)
	case err != nil:
		return ExitRefused, refused("series %s cannot be read: %v", o.Series, err)
	case !info.IsDir():
		return ExitRefused, refused("series %s is not a folder", o.Series)
	}
	rounds, err := capSeriesRounds(o.Series)
	if err != nil {
		return ExitRefused, refused("%v", err)
	}
	exts, err := loadExtensions(o.Series)
	if err != nil {
		return ExitRefused, refused("%v", err)
	}
	if err := checkExtensions(o.Series, exts); err != nil {
		return ExitRefused, refused("%v", err)
	}
	start := o.Repo
	if start == "" {
		start = "."
	}
	repo, err := repoRoot(start)
	if err != nil {
		return ExitRefused, refused("%s is not in a Git repository: %v", start, err)
	}
	if err := checkLazyFetch(repo); err != nil {
		return ExitRefused, refused("%v", err)
	}
	rep := &capReport{Schema: CapReportSchema, Series: o.Series, Rounds: []capRound{}, Skipped: []capSkipped{},
		Open: []capOpen{}, Churn: []capChurn{}, Recurring: []capRecurring{}, Unrelated: []capFinding{},
		Limits: capLimits{churnLimit, recurringLimit, unrelatedLimit}}

	// The cap use is spar run's: every counted code round, a stopped one
	// too, against the cap of the series' last round and the extensions.
	for _, sib := range rounds {
		if sib.round.Config != nil {
			c := sib.round.Config.Caps.Code
			rep.Cap.Code = &c
		}
		if sib.round.Counted && !sib.round.DryRun && stageGroup(sib.round.Stage) == "code" {
			rep.Cap.Counted++
		}
	}
	rep.Cap.Extensions = extensionsFor(exts, "code")
	if rep.Cap.Code != nil {
		allowed := *rep.Cap.Code + rep.Cap.Extensions
		rep.Cap.Allowed = &allowed
	}

	type listed struct {
		name     string
		round    *RoundRecord
		findings []Finding
		final    map[string]LedgerEntry
	}
	var list []listed
	for _, sib := range rounds {
		r := sib.round
		name := filepath.Base(sib.dir)
		if r.Stage != "candidate" && r.Stage != "fixes" {
			continue
		}
		if r.DryRun {
			rep.Skipped = append(rep.Skipped, capSkipped{name, "dry run"})
			continue
		}
		// A round stopped before recording findings has neither finished_at
		// nor findings.json, as the gate reads it; a finished round that
		// lost findings.json is broken, not stopped.
		if _, err := os.Stat(filepath.Join(sib.dir, "findings.json")); errors.Is(err, os.ErrNotExist) {
			if r.FinishedAt != "" {
				return ExitRefused, refused("%s: the round finished but findings.json is missing", sib.dir)
			}
			rep.Skipped = append(rep.Skipped, capSkipped{name, "stopped before recording findings"})
			continue
		}
		findings, err := loadFindings(sib.dir)
		if err != nil {
			return ExitRefused, refused("%v", err)
		}
		if err := checkFindingFields(filepath.Join(sib.dir, "findings.json")); err != nil {
			return ExitRefused, refused("%v", err)
		}
		if _, err := os.Stat(filepath.Join(sib.dir, "ledger.jsonl")); err != nil {
			return ExitRefused, refused("%s: ledger.jsonl cannot be read: %v", sib.dir, err)
		}
		ledger, err := loadLedger(sib.dir)
		if err != nil {
			return ExitRefused, refused("%v", err)
		}
		if err := checkRecords(sib.dir, findings, ledger); err != nil {
			return ExitRefused, refused("%v", err)
		}
		for _, c := range []string{r.Base, r.Head} {
			if _, err := gitWith(repo, noLazyFetch, "rev-parse", "--verify", "--quiet", "--end-of-options", c+"^{commit}"); err != nil {
				return ExitRefused, refused("%s: commit %s is not in %s", sib.dir, c, repo)
			}
		}
		list = append(list, listed{name, r, findings, finalDispositions(ledger)})
	}

	// excluded is a finding whose last disposition rejects it or makes it a
	// duplicate: the three finding signals leave it out.
	excluded := func(final map[string]LedgerEntry, f Finding) bool {
		e, ok := final[f.ID]
		return ok && (e.Disposition == "reject" || e.Disposition == "duplicate")
	}
	asked := func(f Finding) bool { return f.Class == "defect" || f.Class == "requirement-question" }

	anchor := -1
	for i, l := range list {
		classes := map[string]int{"defect": 0, "requirement-question": 0, "hardening": 0, "note": 0}
		for _, f := range l.findings {
			classes[f.Class]++
			// Open as the gate judges it: no disposition, or a duplicate
			// chain that ends open.
			e, done := l.final[f.ID]
			if done && e.Disposition == "duplicate" {
				_, done = resolveDuplicate(l.final, f.ID)
			}
			if asked(f) && !done {
				rep.Open = append(rep.Open, capOpen{l.name, f.ID, f.Class})
			}
			if f.Class == "defect" && !excluded(l.final, f) &&
				(f.IntroducedByChange == "no" || f.IntroducedByChange == "unknown") {
				rep.Unrelated = append(rep.Unrelated, capFinding{l.name, f.ID})
			}
		}
		rep.Rounds = append(rep.Rounds, capRound{l.name, l.round.Stage, l.round.Sequence, l.round.Base, l.round.Head,
			l.round.Counted, classes})
		if anchor < 0 && l.round.Stage == "candidate" && l.round.Counted {
			anchor = i
		}
	}

	// Churn: after the first counted candidate round, the asked findings in
	// files that changed from its head to the round's head.
	if anchor >= 0 {
		from := list[anchor].round.Head
		changed := map[string]map[string]bool{}
		for _, l := range list[anchor+1:] {
			files, ok := changed[l.round.Head]
			if !ok {
				if files, err = changedFiles(repo, noLazyFetch, from, l.round.Head); err != nil {
					return ExitRefused, refused("%s: changed files cannot be read: %v", l.name, err)
				}
				changed[l.round.Head] = files
			}
			c := capChurn{Round: l.name, Findings: []string{}}
			for _, f := range l.findings {
				if asked(f) && !excluded(l.final, f) && !f.LocationUnreadable && f.Path != "" && files[f.Path] {
					c.Findings = append(c.Findings, f.ID)
				}
			}
			c.Count = len(c.Findings)
			rep.Churn = append(rep.Churn, c)
		}
	}

	// Recurring: a link of asked findings in two or more packets; reruns of
	// one packet are one.
	type link struct{ kind, name string }
	packets := map[link]map[packetIdentity]bool{}
	names := map[link][]string{}
	for _, l := range list {
		packet := packetKey(l.round)
		for _, f := range l.findings {
			if !asked(f) || excluded(l.final, f) || f.LinkKind == "none" || f.Link == "" {
				continue
			}
			k := link{f.LinkKind, f.Link}
			if packets[k] == nil {
				packets[k] = map[packetIdentity]bool{}
			}
			// A rerun of a listed packet adds no round: each packet shows as
			// its first round.
			if !packets[k][packet] {
				names[k] = append(names[k], l.name)
			}
			packets[k][packet] = true
		}
	}
	var links []link
	for k, p := range packets {
		if len(p) >= 2 {
			links = append(links, k)
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].kind != links[j].kind {
			return links[i].kind < links[j].kind
		}
		return links[i].name < links[j].name
	})
	for _, k := range links {
		rep.Recurring = append(rep.Recurring, capRecurring{k.kind, k.name, names[k]})
	}

	// Check growth: the gate's growth over the listed fixes rounds.
	measured := map[string]bool{}
	for _, l := range list {
		r := l.round
		if r.Stage != "fixes" || measured[r.Base+".."+r.Head] {
			continue
		}
		measured[r.Base+".."+r.Head] = true
		added, err := numstatWith(repo, noLazyFetch, r.Base, r.Head)
		if err != nil {
			return ExitRefused, refused("%s: added lines cannot be read: %v", l.name, err)
		}
		var checkPaths []string
		if r.Config != nil {
			checkPaths = r.Config.CheckPaths
		}
		for _, path := range sortedKeys(added) {
			if matchAny(checkPaths, path) {
				rep.CheckGrowth.Check += added[path]
			} else {
				rep.CheckGrowth.Other += added[path]
			}
		}
	}
	rep.CheckGrowth.OverHardening = rep.CheckGrowth.Check > rep.CheckGrowth.Other

	if o.Format == "json" {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return ExitInternal, err
		}
		_, err = o.Out.Write(append(data, '\n'))
		return ExitOK, err
	}
	_, err = io.WriteString(o.Out, renderCapReport(rep))
	return ExitOK, err
}

// capSeriesRounds reads a series' rounds in sequence order. Unlike the
// series reading of spar run, a round.json that cannot be opened is
// refused here: a report without that round would be wrong. A folder
// without round.json, or with JSON of another tool, is not a round.
func capSeriesRounds(series string) ([]sibling, error) {
	entries, err := os.ReadDir(series)
	if err != nil {
		return nil, fmt.Errorf("series %s cannot be read: %v", series, err)
	}
	var out []sibling
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(series, e.Name())
		path := filepath.Join(dir, "round.json")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s cannot be read: %v", path, err)
		}
		var probe struct {
			Schema string `json:"schema"`
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("%s is not JSON", path)
		}
		if json.Unmarshal(data, &probe) != nil || probe.Schema != RoundSchema {
			continue // another tool's record, as spar run reads the series
		}
		if err := checkRoundFields(path, data); err != nil {
			return nil, err
		}
		r, err := loadRound(dir)
		if err != nil {
			return nil, err
		}
		switch r.Stage {
		case "plan", "plan-delta", "candidate", "fixes":
		default:
			return nil, fmt.Errorf("%s: stage %q is not a stage of the contract", path, r.Stage)
		}
		out = append(out, sibling{dir, r})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].round.Sequence != out[j].round.Sequence {
			return out[i].round.Sequence < out[j].round.Sequence
		}
		return out[i].dir < out[j].dir
	})
	return out, nil
}

// checkRoundFields refuses a round record whose fields the report reads
// are missing, null or not of the JSON type spar writes: decoding would
// turn them into zero values that look like real ones.
func checkRoundFields(path string, data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("%s is not a round record: %v", path, err)
	}
	field := func(fields map[string]json.RawMessage, name string, into any, nullable bool) error {
		return jsonField(path, fields, name, into, nullable)
	}
	var text string
	var number int64
	var flag bool
	var config, caps map[string]json.RawMessage
	var list []string
	for _, name := range []string{"stage", "base", "head", "packet_sha256"} {
		if err := field(top, name, &text, false); err != nil {
			return err
		}
	}
	if err := field(top, "sequence", &number, false); err != nil {
		return err
	}
	for _, name := range []string{"dry_run", "counted"} {
		if err := field(top, name, &flag, false); err != nil {
			return err
		}
	}
	if _, ok := top["finished_at"]; ok {
		if err := field(top, "finished_at", &text, false); err != nil {
			return err
		}
	}
	if err := field(top, "config", &config, false); err != nil {
		return err
	}
	if err := field(config, "check_paths", &list, true); err != nil {
		return fmt.Errorf("%s (config)", err)
	}
	if err := field(config, "caps", &caps, false); err != nil {
		return fmt.Errorf("%s (config)", err)
	}
	if err := field(caps, "code", &number, false); err != nil {
		return fmt.Errorf("%s (config.caps)", err)
	}
	return nil
}

// jsonField decodes one field of a record into its Go type, refusing it
// when it is missing, null (unless nullable) or of another JSON type.
func jsonField(path string, fields map[string]json.RawMessage, name string, into any, nullable bool) error {
	raw, ok := fields[name]
	switch {
	case !ok:
		return fmt.Errorf("%s: %s is missing", path, name)
	case string(bytes.TrimSpace(raw)) == "null":
		if nullable {
			return nil
		}
		return fmt.Errorf("%s: %s is null", path, name)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s: %s is not of its type: %v", path, name, err)
	}
	return nil
}

// checkFindingFields refuses a findings.json that is not a list, or whose
// findings lack a field the signals read or hold it as null or another
// JSON type: decoding would turn it into an empty value that drops the
// finding from a signal. location_unreadable is absent in records written
// before it existed.
func checkFindingFields(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s cannot be read: %v", path, err)
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(data, &items) != nil || items == nil {
		return fmt.Errorf("%s is not a list of findings", path)
	}
	var text string
	var flag bool
	for i, item := range items {
		at := fmt.Sprintf("%s finding %d", path, i+1)
		for _, name := range []string{"id", "class", "introduced_by_change", "link_kind", "link", "path"} {
			if err := jsonField(at, item, name, &text, false); err != nil {
				return err
			}
		}
		if _, ok := item["location_unreadable"]; ok {
			if err := jsonField(at, item, "location_unreadable", &flag, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkLazyFetch refuses a partial clone when the running Git is older
// than 2.44: such a Git ignores GIT_NO_LAZY_FETCH and could fetch the
// objects the report reads into the clone.
func checkLazyFetch(repo string) error {
	out, err := git(repo, "version")
	if err != nil {
		return err
	}
	var major, minor int
	if _, err := fmt.Sscanf(string(out), "git version %d.%d", &major, &minor); err != nil {
		return fmt.Errorf("the Git version %q cannot be read", strings.TrimSpace(string(out)))
	}
	if major > 2 || major == 2 && minor >= 44 {
		return nil
	}
	// git config exits 1 when no key matches, with nothing on standard output.
	cfg, _ := git(repo, "config", "--get-regexp", `^(extensions\.partialclone|remote\..*\.promisor)$`)
	for _, l := range strings.Split(strings.TrimSpace(string(cfg)), "\n") {
		key, value, _ := strings.Cut(l, " ")
		key = strings.ToLower(key)
		value = strings.ToLower(strings.TrimSpace(value))
		if key == "extensions.partialclone" && value != "" ||
			strings.HasSuffix(key, ".promisor") && slices.Contains([]string{"true", "yes", "on", "1"}, value) {
			return fmt.Errorf("%s is a partial clone, and Git %d.%d could fetch into it; the report needs Git 2.44 or later there",
				repo, major, minor)
		}
	}
	return nil
}

// checkExtensions refuses an extension line outside the form spar extend
// writes: a known stage, --by of 1 or more, the user's words, a cause only
// on a code line and only from Causes, and a reason where the cause needs
// one. A line without a cause is the form written before causes existed.
func checkExtensions(series string, exts []Extension) error {
	path := filepath.Join(series, "extensions.jsonl")
	for i, e := range exts {
		switch {
		case e.Stage != "plan" && e.Stage != "plan-delta" && e.Stage != "code":
			return fmt.Errorf("%s: extension %d has stage %q", path, i+1, e.Stage)
		case e.By < 1:
			return fmt.Errorf("%s: extension %d extends by %d rounds", path, i+1, e.By)
		case strings.TrimSpace(e.UserQuote) == "":
			return fmt.Errorf("%s: extension %d has no user_quote", path, i+1)
		case e.Cause != "" && (e.Stage != "code" || !slices.Contains(Causes, e.Cause)):
			return fmt.Errorf("%s: extension %d has cause %q", path, i+1, e.Cause)
		case e.Cause != "" && (e.By > 1 || e.Cause == "other") && strings.TrimSpace(e.Reason) == "":
			// A line with a cause is spar extend's: it records a reason
			// whenever --by 2 or more or the cause other needs one.
			return fmt.Errorf("%s: extension %d by %d with cause %s has no reason", path, i+1, e.By, e.Cause)
		}
	}
	return nil
}

// checkRecords refuses findings and ledger entries outside the form spar
// writes: every finding has an ID, one of the four classes and the values
// spar writes for the fields the signals read, and every ledger entry names
// a finding and one of the five dispositions.
func checkRecords(dir string, findings []Finding, ledger []LedgerEntry) error {
	for i, f := range findings {
		switch f.Class {
		case "defect", "requirement-question", "hardening", "note":
		default:
			return fmt.Errorf("%s: finding %d has class %q", filepath.Join(dir, "findings.json"), i+1, f.Class)
		}
		if f.ID == "" {
			return fmt.Errorf("%s: finding %d has no id", filepath.Join(dir, "findings.json"), i+1)
		}
		// The fields the signals read.
		switch {
		case !slices.Contains([]string{"yes", "no", "unknown"}, f.IntroducedByChange):
			return fmt.Errorf("%s: finding %d has introduced_by_change %q", filepath.Join(dir, "findings.json"), i+1, f.IntroducedByChange)
		case !slices.Contains([]string{"criterion", "rule", "contract", "none"}, f.LinkKind):
			return fmt.Errorf("%s: finding %d has link_kind %q", filepath.Join(dir, "findings.json"), i+1, f.LinkKind)
		}
	}
	for i, e := range ledger {
		switch e.Disposition {
		case "fix", "reject", "defer", "duplicate", "answer":
		default:
			return fmt.Errorf("%s: entry %d has disposition %q", filepath.Join(dir, "ledger.jsonl"), i+1, e.Disposition)
		}
		if e.Finding == "" {
			return fmt.Errorf("%s: entry %d names no finding", filepath.Join(dir, "ledger.jsonl"), i+1)
		}
	}
	return nil
}

func renderCapReport(rep *capReport) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	none := func(n int) {
		if n == 0 {
			line("- none")
		}
	}
	line("# Cap report: %s", rep.Series)
	line("")
	if rep.Cap.Code != nil {
		line("- code rounds counted: %d of %d (cap %d, extensions %d)", rep.Cap.Counted, *rep.Cap.Allowed, *rep.Cap.Code, rep.Cap.Extensions)
	} else {
		line("- code rounds counted: %d of unknown (cap unknown, extensions %d)", rep.Cap.Counted, rep.Cap.Extensions)
	}
	open := map[string]int{}
	for _, f := range rep.Open {
		open[f.Class]++
	}
	line("- open: defect %d, requirement-question %d", open["defect"], open["requirement-question"])
	line("")
	line("## Rounds")
	line("")
	line("| Round | Stage | Sequence | Counted | Base | Head | defect | requirement-question | hardening | note |")
	line("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, r := range rep.Rounds {
		counted := "no"
		if r.Counted {
			counted = "yes"
		}
		line("| %s | %s | %d | %s | %s | %s | %d | %d | %d | %d |", r.Round, r.Stage, r.Sequence, counted, r.Base, r.Head,
			r.Classes["defect"], r.Classes["requirement-question"], r.Classes["hardening"], r.Classes["note"])
	}
	if len(rep.Skipped) > 0 {
		line("")
		for _, s := range rep.Skipped {
			line("- skipped %s: %s", s.Round, s.Reason)
		}
	}
	line("")
	line("## Open findings")
	line("")
	for _, f := range rep.Open {
		line("- %s %s (%s)", f.Round, f.Finding, f.Class)
	}
	none(len(rep.Open))
	line("")
	line("## Signals")
	line("")
	line("### Churn")
	line("")
	for _, c := range rep.Churn {
		if c.Count == 0 {
			line("- %s: 0", c.Round)
			continue
		}
		line("- %s: %d (%s)", c.Round, c.Count, strings.Join(c.Findings, ", "))
	}
	none(len(rep.Churn))
	line("")
	line("### Recurring links")
	line("")
	for _, r := range rep.Recurring {
		line("- %s %s: %s", r.LinkKind, r.Link, strings.Join(r.Rounds, ", "))
	}
	none(len(rep.Recurring))
	line("")
	line("### Unrelated defects")
	line("")
	line("- count: %d", len(rep.Unrelated))
	for _, f := range rep.Unrelated {
		line("- %s %s", f.Round, f.Finding)
	}
	line("")
	line("### Check growth")
	line("")
	over := "no"
	if rep.CheckGrowth.OverHardening {
		over = "yes"
	}
	line("- added lines: check %d, other %d; over hardening: %s", rep.CheckGrowth.Check, rep.CheckGrowth.Other, over)
	line("")
	line("## Limits")
	line("")
	line("- churn: %s", rep.Limits.Churn)
	line("- recurring: %s", rep.Limits.Recurring)
	line("- unrelated: %s", rep.Limits.Unrelated)
	return b.String()
}
