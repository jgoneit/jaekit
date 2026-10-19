package spar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Exit codes shared by the commands.
const (
	ExitOK        = 0
	ExitNoResult  = 1
	ExitDegraded  = 3
	ExitBlocked   = 4
	ExitUsage     = 64
	ExitRefused   = 65
	ExitInternal  = 70
	nativePreset  = "native"
	commandPreset = "command"
)

// Stages a round can be.
var Stages = []string{"plan", "plan-delta", "candidate", "fixes"}

// ExitError carries the exit code a failure ends the command with.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

func refused(format string, args ...any) error {
	return &ExitError{ExitRefused, fmt.Sprintf(format, args...)}
}

func usageError(format string, args ...any) error {
	return &ExitError{ExitUsage, fmt.Sprintf(format, args...)}
}

// RunOptions are the inputs of `spar run`.
type RunOptions struct {
	Repo       string
	Stage      string
	Base, Head string
	Out        string
	Criteria   []string
	Rules      []string
	Subjects   []string
	Questions  []string
	CheckPaths []string
	Reviewers  []string // nil: the configured list
	Label      string
	DryRun     bool
	Version    string
	Getenv     func(string) string
	Log        io.Writer
}

func readDocument(repo, path, what string) (Document, error) {
	real := realish(path)
	data, err := os.ReadFile(real)
	if err != nil {
		return Document{}, refused("%s %s cannot be read: %v", what, path, err)
	}
	name := filepath.Base(real)
	if rel := insideRepo(repo, real); rel != "" {
		name = filepath.ToSlash(rel)
	}
	return newDocument(name, real, data), nil
}

func readDocuments(repo string, paths []string, what string) ([]Document, error) {
	var docs []Document
	seen := map[string]bool{}
	for _, p := range paths {
		d, err := readDocument(repo, p, what)
		if err != nil {
			return nil, err
		}
		if !seen[d.Source] {
			seen[d.Source] = true
			docs = append(docs, d)
		}
	}
	return docs, nil
}

// priorLines lists the findings of earlier rounds of the same stage group,
// leaving out rounds of the same stage, base and head so that a rerun gets
// the same packet.
func priorLines(siblings []sibling, current *RoundRecord) ([]string, []string, error) {
	var groups []string
	switch current.Stage {
	case "plan-delta":
		groups = []string{"plan", "plan_delta"}
	case "fixes":
		groups = []string{"code"}
	default:
		return []string{}, []string{}, nil
	}
	lines, dirs := []string{}, []string{}
	for _, s := range siblings {
		if s.round.DryRun || sameIdentity(s.round, current) || !contains(groups, stageGroup(s.round.Stage)) {
			continue
		}
		findings, err := loadFindings(s.dir)
		if err != nil {
			return nil, nil, err
		}
		dirs = append(dirs, filepath.Base(s.dir))
		for _, f := range findings {
			lines = append(lines, fmt.Sprintf("%s %s [%s] at %s: %s", filepath.Base(s.dir), f.ID, f.Class,
				f.Location, short(f.Claim, 300)))
		}
	}
	return lines, dirs, nil
}

// admit applies the stage caps, the rerun rule and the fixes chain. It
// returns whether the run counts against the cap and which rounds it reruns.
func admit(siblings []sibling, exts []Extension, r *RoundRecord) (bool, []string, error) {
	group := stageGroup(r.Stage)
	// Rounds of a series run one at a time under its lock, so every earlier
	// round here has ended; one without finished_at was stopped, and its
	// reviewers did not complete the packet.
	var same []sibling
	for _, s := range siblings {
		if !s.round.DryRun && sameIdentity(s.round, r) && s.round.PacketSHA256 == r.PacketSHA256 {
			same = append(same, s)
		}
	}
	rerun := false
	var rerunOf []string
	if len(same) > 0 {
		// A reviewer is its name and preset, as in the gate: the same name
		// under another preset is another reviewer.
		reviewer := func(name, preset string) string { return name + "\x00" + preset }
		completed, unfinished := map[string]bool{}, map[string]bool{}
		for _, s := range same {
			rerunOf = append(rerunOf, filepath.Base(s.dir))
			for name, st := range s.round.Reviewers {
				if st.Status == StatusCompleted {
					completed[reviewer(name, st.Preset)] = true
				} else {
					unfinished[reviewer(name, st.Preset)] = true
				}
			}
		}
		presetOf := map[string]string{}
		if r.Config != nil {
			for _, rv := range r.Config.Reviewers {
				presetOf[rv.Name] = rv.Preset
			}
		}
		rerun = len(r.RequestedReviewers) > 0
		for _, name := range r.RequestedReviewers {
			key := reviewer(name, presetOf[name])
			if completed[key] || !unfinished[key] {
				rerun = false
			}
		}
	}
	if r.Stage == "fixes" {
		var previous *RoundRecord
		for _, s := range siblings {
			if s.round.DryRun || stageGroup(s.round.Stage) != "code" || sameIdentity(s.round, r) {
				continue
			}
			previous = s.round
		}
		switch {
		case previous == nil:
			return false, nil, refused("a fixes round needs an earlier candidate or fixes round in %s", r.Series)
		case previous.Head != r.Base:
			return false, nil, refused("the fixes chain is broken: the base %s is not the head %s of the previous code round in %s",
				r.Base, previous.Head, r.Series)
		}
	}
	if r.DryRun {
		return false, nil, nil
	}
	if rerun {
		return false, rerunOf, nil
	}
	counted := int64(0)
	for _, s := range siblings {
		if s.round.Counted && !s.round.DryRun && stageGroup(s.round.Stage) == group {
			counted++
		}
	}
	allowed := capFor(r.Config.Caps, group) + extensionsFor(exts, group)
	if counted >= allowed {
		return false, nil, refused("the %s cap is reached in %s: %d of %d rounds ran; only reviewers that did not complete the same packet may run again, "+
			"and more rounds need the user's words recorded with `spar extend %s --stage %s --by N --user-quote ...`",
			strings.ReplaceAll(group, "_", "-"), r.Series, counted, allowed, r.Series, extendStage(group))
	}
	return true, nil, nil
}

func extendStage(group string) string {
	switch group {
	case "plan_delta":
		return "plan-delta"
	default:
		return group
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case d.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
	})
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Run carries out `spar run`: one round on a pinned packet.
func Run(o RunOptions) (int, error) {
	if !contains(Stages, o.Stage) {
		return ExitUsage, usageError("--stage must be one of %s", strings.Join(Stages, ", "))
	}
	if o.Base == "" || o.Out == "" {
		return ExitUsage, usageError("spar run needs --base and --out")
	}
	if len(o.Criteria) == 0 {
		return ExitUsage, usageError("spar run needs at least one --criteria document")
	}
	if o.Head == "" {
		o.Head = "HEAD"
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
	cfg, err := LoadConfig(repo, o.Getenv, o.Reviewers)
	if err != nil {
		return ExitRefused, err
	}
	if len(o.CheckPaths) > 0 {
		cfg.CheckPaths = uniq(append(cfg.CheckPaths, o.CheckPaths...))
		cfg.Digest = cfg.digest()
	}
	base, err := resolveCommit(repo, o.Base)
	if err != nil {
		return ExitRefused, refused("--base: %v", err)
	}
	head, err := resolveCommit(repo, o.Head)
	if err != nil {
		return ExitRefused, refused("--head: %v", err)
	}
	out := realish(o.Out)
	if _, err := os.Lstat(out); err == nil {
		return ExitRefused, refused("%s already exists; each round writes a new folder", o.Out)
	}
	if err := localRecordPath(repo, out); err != nil {
		return ExitRefused, refused("--out %v", err)
	}
	criteria, err := readDocuments(repo, o.Criteria, "criteria document")
	if err != nil {
		return ExitRefused, err
	}
	rulePaths := append([]string{}, o.Rules...)
	for _, r := range cfg.Rules {
		// A project names rule files inside its repository only; a link
		// out of it is refused like a path that leaves it.
		path := filepath.Join(repo, filepath.FromSlash(r))
		if insideRepo(repo, realish(path)) == "" {
			return ExitRefused, refused("the project configuration's rules entry %q leaves the repository", r)
		}
		rulePaths = append(rulePaths, path)
	}
	rules, err := readDocuments(repo, rulePaths, "rule document")
	if err != nil {
		return ExitRefused, err
	}
	subjects, err := readDocuments(repo, o.Subjects, "subject document")
	if err != nil {
		return ExitRefused, err
	}
	baseTree, err := treeOf(repo, base)
	if err != nil {
		return ExitInternal, err
	}
	headTree, err := treeOf(repo, head)
	if err != nil {
		return ExitInternal, err
	}
	changes, err := diff(repo, base, head)
	if err != nil {
		return ExitInternal, err
	}
	series := filepath.Dir(out)
	// One round at a time per series, from admission until its record is
	// finished: two runs cannot both take the last round under a cap, and a
	// round found unfinished here was stopped before it finished.
	if err := os.MkdirAll(series, 0o755); err != nil {
		return ExitInternal, err
	}
	unlock, err := lockFile(series, func() {
		if o.Log != nil {
			fmt.Fprintf(o.Log, "spar: waiting for another round of %s to finish\n", series)
		}
	})
	if err != nil {
		return ExitInternal, err
	}
	defer unlock()
	if _, err := os.Lstat(out); err == nil {
		return ExitRefused, refused("%s already exists; each round writes a new folder", o.Out)
	}
	siblings, err := seriesRounds(series)
	if err != nil {
		return ExitRefused, refused("series %s: %v", series, err)
	}
	exts, err := loadExtensions(series)
	if err != nil {
		return ExitRefused, refused("series %s: %v", series, err)
	}
	r := &RoundRecord{
		Schema: RoundSchema, SparVersion: o.Version, Stage: o.Stage, Label: o.Label, Series: series,
		Sequence: len(siblings) + 1, Repo: repo, Base: base, Head: head, BaseTree: baseTree, HeadTree: headTree,
		DiffSHA256: newDocument("", "", changes).SHA256, Criteria: criteria, CriteriaDigest: documentsDigest(criteria),
		Rules: nonNil(rules), Subjects: nonNil(subjects), Questions: append([]string{}, o.Questions...),
		Config: cfg, DryRun: o.DryRun, RerunOf: []string{}, Reviewers: map[string]*ReviewerState{},
		Links: LinkTargets{CriteriaIDs: criteriaIDs(criteria), Rules: ruleTargets(repo, rules)},
	}
	for _, rv := range cfg.Reviewers {
		r.RequestedReviewers = append(r.RequestedReviewers, rv.Name)
		if rv.Required {
			r.RequiredReviewers = append(r.RequiredReviewers, rv.Name)
		}
	}
	if r.RequiredReviewers == nil {
		r.RequiredReviewers = []string{}
	}
	prior, priorDirs, err := priorLines(siblings, r)
	if err != nil {
		return ExitRefused, refused("series %s: %v", series, err)
	}
	r.Prior = priorDirs
	packet := buildPacket(packetInput{Stage: o.Stage, Base: base, Head: head, BaseTree: baseTree, HeadTree: headTree,
		Angles: cfg.Angles, CheckPaths: cfg.CheckPaths, Criteria: criteria, Rules: rules, Subjects: subjects,
		Questions: o.Questions, Prior: prior, Diff: changes})
	r.PacketSHA256 = newDocument("", "", []byte(packet)).SHA256
	counted, rerunOf, err := admit(siblings, exts, r)
	if err != nil {
		return ExitRefused, err
	}
	r.Counted = counted
	if rerunOf != nil {
		r.RerunOf = rerunOf
	}
	return runRound(o, r, out, []byte(packet))
}

func nonNil(docs []Document) []Document {
	if docs == nil {
		return []Document{}
	}
	return docs
}

func runRound(o RunOptions, r *RoundRecord, out string, packet []byte) (int, error) {
	if err := os.Mkdir(out, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ExitRefused, refused("%s already exists; each round writes a new folder", o.Out)
		}
		return ExitInternal, err
	}
	r.StartedAt = now()
	packetPath := filepath.Join(out, "packet.md")
	schemaPath := filepath.Join(out, "schema.json")
	if err := os.WriteFile(packetPath, packet, 0o644); err != nil {
		return ExitInternal, err
	}
	if err := os.WriteFile(schemaPath, resultSchema(), 0o644); err != nil {
		return ExitInternal, err
	}
	for _, rv := range r.Config.Reviewers {
		st := &ReviewerState{Name: rv.Name, Preset: rv.Preset, Isolation: rv.Isolation, Required: rv.Required,
			Status: "running", RequestedModel: strPtr(rv.Model), FallbackModel: rv.FallbackModel, Effort: rv.Effort,
			TimeoutSeconds: rv.TimeoutSeconds, Attempts: []Attempt{}, AnglesChecked: []string{},
			AnglesUnchecked: []AngleNote{}, Warnings: []string{}}
		if r.DryRun {
			st.Status, st.Reason = StatusNotRun, "dry run: the packet and workspace were built, no reviewer ran"
		}
		r.Reviewers[rv.Name] = st
	}
	if err := writeJSON(filepath.Join(out, "round.json"), r); err != nil {
		return ExitInternal, err
	}
	tmp, err := os.MkdirTemp("", "spar-workspace-")
	if err != nil {
		return ExitInternal, err
	}
	defer os.RemoveAll(tmp)
	template := filepath.Join(tmp, "template")
	if err := exportTree(r.Repo, r.Head, filepath.Join(template, "head")); err != nil {
		return ExitInternal, err
	}
	if err := exportTree(r.Repo, r.Base, filepath.Join(template, "base")); err != nil {
		return ExitInternal, err
	}
	if r.DryRun {
		// The dry run leaves the workspace a reviewer would get.
		if err := copyTree(template, filepath.Join(out, dryRunWorkspace)); err != nil {
			return ExitInternal, err
		}
		r.FinishedAt = now()
		if err := finishRound(out, r, nil); err != nil {
			return ExitInternal, err
		}
		return ExitOK, nil
	}
	var wg sync.WaitGroup
	results := map[string]any{}
	var mu sync.Mutex
	for i, rv := range r.Config.Reviewers {
		st := r.Reviewers[rv.Name]
		if rv.Preset == nativePreset {
			if err := prepareNative(out, template, packet, r, st); err != nil {
				return ExitInternal, err
			}
			continue
		}
		wg.Add(1)
		go func(i int, rv Reviewer, st *ReviewerState) {
			defer wg.Done()
			result := runReviewer(rv, st, tmp, i, out, packet, r)
			mu.Lock()
			results[rv.Name] = result
			mu.Unlock()
		}(i, rv, st)
	}
	wg.Wait()
	r.FinishedAt = now()
	if err := finishRound(out, r, results); err != nil {
		return ExitInternal, err
	}
	for _, st := range r.Reviewers {
		if st.Status == StatusCompleted || st.Status == StatusPending {
			return ExitOK, nil
		}
	}
	return ExitNoResult, nil
}

// runReviewer runs a reviewer with its model and, after a quota or rate
// refusal, once more with its fallback model on the same packet.
func runReviewer(rv Reviewer, st *ReviewerState, tmp string, i int, out string, packet []byte, r *RoundRecord) any {
	models := []string{rv.Model}
	if rv.FallbackModel != "" && rv.FallbackModel != rv.Model && rv.Preset != commandPreset {
		models = append(models, rv.FallbackModel)
	}
	started := time.Now()
	var result any
	for n, model := range models {
		ws := filepath.Join(tmp, fmt.Sprintf("%d-%s-%d", i, rv.Name, n))
		dir := filepath.Join(out, rv.Name)
		if n > 0 {
			dir = filepath.Join(out, rv.Name, "fallback")
		}
		var a Attempt
		if err := copyTree(filepath.Join(tmp, "template"), ws); err != nil {
			a = Attempt{Model: model, Status: StatusFailed, Reason: "workspace: " + err.Error(), StartedAt: now(), FinishedAt: now()}
		} else {
			a = runAttempt(rv, model, runPaths{workspace: ws, dir: dir, packet: filepath.Join(out, "packet.md"),
				schema: filepath.Join(out, "schema.json")}, packet, r.Config.Angles, r.Head)
		}
		os.RemoveAll(ws)
		st.Attempts = append(st.Attempts, a)
		st.Status, st.Reason, st.Resets = a.Status, a.Reason, a.Resets
		st.ObservedModel = strPtr(a.ObservedModel)
		st.FallbackUsed = n > 0
		result = a.result
		if !(a.Status == StatusUnavailable && a.quota) {
			break
		}
	}
	if rv.Preset == commandPreset {
		st.RequestedModel = nil
	}
	st.ElapsedSeconds = float64(time.Since(started).Milliseconds()) / 1000
	if st.Status != StatusCompleted {
		return nil
	}
	return result
}

// finishRound classifies the completed reviewers' findings and writes the
// findings, the automatic deferrals, the record and the summary.
func finishRound(out string, r *RoundRecord, results map[string]any) error {
	c := classifier{repo: r.Repo, head: r.Head, checkPaths: r.Config.CheckPaths, targets: r.Links}
	var findings []Finding
	for _, rv := range r.Config.Reviewers {
		result := object(results[rv.Name])
		if result == nil {
			continue
		}
		findings = append(findings, applyResult(c, r.Reviewers[rv.Name], rv.Name, result)...)
	}
	return saveRound(out, r, findings, true)
}

// applyResult records a completed result in a reviewer's state and returns
// its classified findings.
func applyResult(c classifier, st *ReviewerState, name string, result map[string]any) []Finding {
	st.Verdict = str(result["verdict"])
	st.Limits = str(result["limits"])
	st.AnglesChecked = []string{}
	st.AnglesUnchecked = []AngleNote{}
	for _, item := range anyList(result["angles_checked"]) {
		st.AnglesChecked = append(st.AnglesChecked, str(object(item)["angle"]))
	}
	for _, item := range anyList(result["angles_unchecked"]) {
		st.AnglesUnchecked = append(st.AnglesUnchecked, AngleNote{Angle: str(object(item)["angle"]), Reason: str(object(item)["reason"])})
	}
	var out []Finding
	for _, item := range anyList(result["findings"]) {
		out = append(out, c.classify(name, object(item)))
	}
	n := len(out)
	st.Findings = &n
	return out
}

// saveRound adds findings and their automatic deferrals to a round and
// rewrites its record and summary.
func saveRound(out string, r *RoundRecord, added []Finding, fresh bool) error {
	if !fresh {
		// Adding to a round needs its records: rewriting a lost findings
		// file would drop the findings it held.
		for _, name := range []string{"findings.json", "ledger.jsonl"} {
			if _, err := os.Stat(filepath.Join(out, name)); err != nil {
				return refused("%s/%s is missing; the round record is incomplete", out, name)
			}
		}
	}
	findings, err := loadFindings(out)
	if err != nil {
		return err
	}
	if fresh {
		findings = nil
	}
	findings = append(findings, added...)
	if findings == nil {
		findings = []Finding{}
	}
	if err := writeJSON(filepath.Join(out, "findings.json"), findings); err != nil {
		return err
	}
	ledgerPath := filepath.Join(out, "ledger.jsonl")
	if fresh {
		if err := os.WriteFile(ledgerPath, nil, 0o644); err != nil {
			return err
		}
	}
	for _, f := range added {
		if f.Class == "hardening" {
			entry := LedgerEntry{Finding: f.ID, Disposition: "defer", Auto: true,
				Reason: "hardening: " + f.ClassReason, To: "deferred hardening", RecordedAt: now()}
			if err := appendJSONLine(ledgerPath, entry); err != nil {
				return err
			}
		}
	}
	if err := writeJSON(filepath.Join(out, "round.json"), r); err != nil {
		return err
	}
	ledger, err := loadLedger(out)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "summary.md"), []byte(renderSummary(out, r, findings, ledger)), 0o644)
}

const nativeInstructions = `# Native review

You are a clean-context reviewer started by the host agent. Review the packet below on its own: you have no other context, and you must not use anything you know about the work beyond these files.

- The review workspace is %s. Its head/ and base/ folders hold the tracked files of the two commits. Read only files in that workspace. Do not change, create or delete any file, run no commands, and do not use the network.
- Reply with one JSON object and nothing else. It follows the result schema in %s.
- The host records your reply with ` + "`spar record %s %s --result <file>`" + ` or from your transcript.

`

// prepareNative writes a native reviewer's prompt and workspace, in the
// folder named after it, for the host's clean-context subagent.
func prepareNative(out, template string, packet []byte, r *RoundRecord, st *ReviewerState) error {
	dir := filepath.Join(out, st.Name)
	ws := filepath.Join(dir, "workspace")
	if err := copyTree(template, ws); err != nil {
		return err
	}
	entries, err := manifest(ws)
	if err != nil {
		return err
	}
	st.WorkspaceDigest = manifestDigest(entries)
	prompt := fmt.Sprintf(nativeInstructions, ws, filepath.Join(out, "schema.json"), out, st.Name) + string(packet)
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(prompt), 0o644); err != nil {
		return err
	}
	st.Status = StatusPending
	st.Reason = "waiting for the host's clean-context subagent; record its reply with spar record"
	return nil
}

// Config prints the configuration a run would apply.
func ShowConfig(repo string, getenv func(string) string, override []string, w io.Writer) error {
	if repo == "" {
		repo = "."
	}
	root, err := repoRoot(repo)
	if err != nil {
		root = repo
	}
	cfg, err := LoadConfig(realish(root), getenv, override)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var errNotFound = errors.New("not found")
