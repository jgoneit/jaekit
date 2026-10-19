// Command ha is Seal Core: it runs verification commands without a shell,
// appends the run record, computes goal state deterministically, and records
// completion. The behavior is defined in contracts/run-record.md.
package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/evidence"
	"github.com/jgoneit/jaekit/internal/gitx"
	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/run"
	"github.com/jgoneit/jaekit/internal/status"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.1.6-dev"

const checkTimeout = 900 * time.Second

const (
	exitOK       = 0
	exitProblems = 1
	exitUsage    = 64
	exitRefused  = 65
	exitRules    = 66
	exitInternal = 70
)

const usage = `ha — Seal Core: run records and completion for one goal

Usage:
  ha capabilities [--format json]
  ha lint   <goal>
  ha estimate <goal> [AC-n | EX-n | T001 ...] [--baseline] [--format md|json]
  ha start  <goal> --request <quote> [--skill <path to SKILL.md>]
                   [--rules run-rules/4] [--test-authority <agent interpretation>]
                   [--host-name <name>] [--host-version <version>] [--model <model>]
  ha check  <goal> [AC-n | EX-n | T001 ...] [--baseline]
  ha budget <goal> --runs <total> --from <old-total> --window <seq> --revision <seq>
                    [--quote <user words> --context <request context>]
                    [--reason <reason>] [--interpretation <agent interpretation>]
  ha budget <goal> --preapprove <total> --window <seq> --revision <seq>
                    --quote <user words> --context <request context>      (run-rules/4)
  ha note   <goal> block --cause <auth|permission|quota|environment|spec|other> [--quote <text>]
  ha note   <goal> unblock [--quote <text>]
  ha note   <goal> confirm <AC-n | scope:<path> | tests:<path> | spec> --quote <user words>
  ha note   <goal> reopen --quote <user words>
  ha note   <goal> input --quote <user words>
  ha note   <goal> test-change tests:<path> --criteria <AC-n,...> --how <how it is checked>
                   [--moved-to <path> ...] [--weakened]                   (run-rules/4)
  ha note   <goal> wait --reason <decision asked>                         (run-rules/4)
  ha note   <goal> resume                                                 (run-rules/4)
  ha note   <goal> recover <AC-n> --input check|baseline --change <changed condition>
                                                                          (run-rules/4)
  ha note   <goal> allow-recovery <AC-n> --input check|baseline --count <n> --quote <user words>
                                                                          (run-rules/4)
  ha finding <goal> [--input <JSON file>] [--format json|md]
  ha history <goal> [add|current|list|show] ...   (see ha history --help)
  ha status <goal> [--format md|json]
  ha done   <goal> [--format md|json]
  ha log    <goal> <seq> [--tail N]
  ha timing <goal> [--format md|json]
  ha --version

<goal> is a goal directory such as docs/specs/empty-input.

Exit codes
  status, done: 0 complete, 1 incomplete, 2 needs_user, 3 blocked, 4 budget_exhausted
  lint, check:  0 ok, 1 lint problems or a check that did not give the expected result
  estimate:     0 sufficient PLAN limit, 1 shortfall or lint problems
  all:          64 usage, 65 refused, 66 unsupported rules version, 70 internal error

Contract: contracts/run-record.md
`

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

type cli struct {
	out, err io.Writer
	execute  checkExecutor
}

func (c *cli) fail(code int, format string, a ...any) int {
	fmt.Fprintf(c.err, "ha: "+format+"\n", a...)
	return code
}

func runMain(argv []string, stdout, stderr io.Writer) int {
	c := &cli{out: stdout, err: stderr}
	if len(argv) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, rest := argv[0], argv[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "--version", "version":
		fmt.Fprintf(stdout, "ha %s\n", version)
		return exitOK
	case "capabilities":
		return c.capabilities(rest)
	case "lint":
		return c.lint(rest)
	case "estimate":
		return c.estimate(rest)
	case "start":
		return c.start(rest)
	case "check":
		return c.check(rest)
	case "budget":
		return c.budget(rest)
	case "finding":
		return c.finding(rest)
	case "history":
		return c.history(rest)
	case "note":
		return c.note(rest)
	case "status":
		return c.status(rest, false)
	case "done":
		return c.status(rest, true)
	case "log":
		return c.log(rest)
	case "timing":
		return c.timing(rest)
	}
	return c.fail(exitUsage, "unknown command %q; run ha --help", cmd)
}

// workspace is one goal in one repository.
type workspace struct {
	arg     string
	repo    *gitx.Repo
	goal    *goaldocs.Goal
	bundle  *bundle.Bundle
	records string
	lock    string
	logDir  string
	// logRel is logDir as records name it: repository-relative, with .git
	// standing for this working tree's git directory, so a linked worktree
	// records the same path as a normal checkout (run-record.md §2.2).
	logRel string
}

func openGoal(arg string) (*workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	abs := arg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, arg)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("goal directory %s does not exist", arg)
	}
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return nil, err
	}
	repo, err := gitx.Open(abs)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(repo.Root, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("goal directory %s must be a subdirectory inside the repository", arg)
	}
	slashRel := filepath.ToSlash(rel)
	g, err := goaldocs.Load(repo.Root, slashRel)
	if err != nil {
		return nil, err
	}
	records := filepath.Join(repo.Root, rel, "runs.jsonl")
	lines, _, err := record.Read(records)
	if err != nil {
		return nil, err
	}
	b, err := bundle.LoadForRules(repo.Root, g, rules(lines))
	if err != nil {
		return nil, err
	}
	haDir := filepath.Join(repo.GitDir, "ha")
	return &workspace{
		arg:     arg,
		repo:    repo,
		goal:    g,
		bundle:  b,
		records: records,
		lock:    filepath.Join(haDir, "lock"),
		logDir:  filepath.Join(haDir, "runs", url.PathEscape(slashRel)),
		logRel:  ".git/ha/runs/" + url.PathEscape(slashRel),
	}, nil
}

func (w *workspace) read() ([]record.Line, *record.IntegrityError, error) {
	return record.Read(w.records)
}

func startLine(lines []record.Line) *record.Line {
	for i := range lines {
		if lines[i].Kind == "start" {
			return &lines[i]
		}
	}
	return nil
}

// rules returns the status rule version of a goal with these record lines:
// the one its start names, or the one a new start records.
func rules(lines []record.Line) string {
	if s := startLine(lines); s != nil {
		return s.Rules
	}
	return status.Rules
}

// excludes returns the paths outside the goal's code state under its rules,
// the same list status computation uses (run-record.md §4.6).
func (w *workspace) excludes(rules string) ([]string, error) {
	return status.Excludes(w.repo.Root, w.goal, w.bundle, rules)
}

func (c *cli) openOrFail(a *args, n int) (*workspace, int) {
	if len(a.pos) < n {
		return nil, c.fail(exitUsage, "missing <goal>; run ha --help")
	}
	w, err := openGoal(a.pos[0])
	if err != nil {
		return nil, c.fail(exitRefused, "%v", err)
	}
	return w, -1
}

func (c *cli) printLint(w *workspace) int {
	ps := bundle.Lint(w.goal, w.bundle)
	for _, p := range ps {
		fmt.Fprintln(c.out, p.String())
	}
	return len(ps)
}

func (c *cli) lint(in []string) int {
	a, err := parseArgs(in, nil, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	if c.printLint(w) > 0 {
		return exitProblems
	}
	fmt.Fprintln(c.out, "ok: no lint problems")
	// The tier is reported only here, under its own name; status and done
	// output stay as they were (goal-docs.md §2.5).
	if w.goal.Assurance != "" {
		fmt.Fprintf(c.out, "assurance tier: %s\n", w.goal.Assurance)
	}
	return exitOK
}

func (c *cli) start(in []string) int {
	a, err := parseArgs(in, []string{"request", "skill", "skill-name", "skill-version", "host-name", "host-version", "model", "rules", "test-authority"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	quote := strings.TrimSpace(a.vals["request"])
	if quote == "" {
		return c.fail(exitUsage, "--request is required: quote the user's request that authorizes this goal")
	}
	// run-rules/4 is chosen, never the default (run-record.md §4.6).
	goalRules := status.Rules
	if r, ok := a.vals["rules"]; ok && r != status.Rules {
		if r != status.RulesV4 {
			return c.fail(exitUsage, "--rules accepts %s or %s", status.RulesV3, status.RulesV4)
		}
		goalRules = r
		if w.bundle, err = bundle.LoadForRules(w.repo.Root, w.goal, goalRules); err != nil {
			return c.fail(exitInternal, "%v", err)
		}
	}
	interpretation, hasAuthority := a.vals["test-authority"]
	interpretation = strings.TrimSpace(interpretation)
	if hasAuthority && (goalRules != status.RulesV4 || interpretation == "") {
		return c.fail(exitUsage, "--test-authority needs --rules %s and the executor's reading of the request", status.RulesV4)
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	if bad != nil {
		return c.fail(exitRefused, "%v", bad)
	}
	if s := startLine(lines); s != nil {
		return c.fail(exitRefused, "already started at seq %d; a new start needs a new goal bundle", s.Seq)
	}
	if c.printLint(w) > 0 {
		return c.fail(exitRefused, "fix the lint problems above before ha start")
	}
	minimum := minimumRequiredRuns(w)
	if w.bundle.Budget.Runs < minimum {
		return c.fail(exitRefused, "minimum %d runs exceeds plan limit %d (shortfall %d); decide a sufficient budget before starting", minimum, w.bundle.Budget.Runs, minimum-w.bundle.Budget.Runs)
	}
	head, err := w.repo.Head()
	if err != nil {
		return c.fail(exitRefused, "%v", err)
	}
	excl, err := w.excludes(goalRules)
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	clean, err := w.repo.Clean(excl)
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	skill := record.Skill{Name: a.vals["skill-name"], Version: a.vals["skill-version"]}
	if p := a.vals["skill"]; p != "" {
		if skill, err = readSkill(p); err != nil {
			return c.fail(exitRefused, "%v", err)
		}
	}
	bud := w.bundle.Budget
	e := &record.Start{
		Header:     record.Header{Kind: "start", HaVersion: version},
		Goal:       w.goal.Dir,
		BaseCommit: head,
		TreeClean:  clean,
		SpecDigest: w.goal.Digest,
		Request:    record.Request{Source: "conversation", Quote: quote},
		Host:       record.Host{Name: a.vals["host-name"], Version: a.vals["host-version"], Model: a.vals["model"]},
		Skill:      skill,
		Rules:      goalRules,
		Budget:     record.Budget{Runs: bud.Runs, ElapsedSeconds: bud.ElapsedSeconds, Cost: bud.Cost},
	}
	if goalRules == status.RulesV4 {
		// The authority range and the test classification stay as started.
		e.Scope = &record.ScopeCopy{Allowed: append([]string{}, w.bundle.Scope.Allowed...),
			Tests: append([]string{}, w.bundle.Scope.Tests...)}
		if hasAuthority {
			e.TestAuthority = &record.TestAuthority{Interpretation: interpretation}
		}
	}
	l, err := record.Append(w.records, w.lock, e, time.Now(), nil)
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	fmt.Fprintf(c.out, "started: seq %d, base %s, rules %s, budget %d runs / %ds\n", l.Seq, head[:12], goalRules, bud.Runs, bud.ElapsedSeconds)
	fmt.Fprintf(c.out, "minimum required: %d runs; plan limit: %d (retries and optional runs are additional)\n", minimum, bud.Runs)
	if !clean {
		fmt.Fprintln(c.err, "ha: warning: the working tree has uncommitted changes outside the bundle documents; results count only on a clean, committed tree")
	}
	return exitOK
}

type target struct {
	criterion string
	task      string
	index     int
	kind      string
	command   string
	argv      []string
	checks    []string      // baseline overlay paths
	limit     time.Duration // the applied time limit; zero means checkTimeout
	// execArgv is the argv ha runs when it differs from the recorded argv (an
	// adapter's output location); stdout receives a copy of standard output.
	execArgv []string
	stdout   io.Writer
}

// runArgv is the argv ha executes; records keep argv.
func (t target) runArgv() []string {
	if t.execArgv != nil {
		return t.execArgv
	}
	return t.argv
}

func (t target) label() string {
	if t.criterion != "" {
		return t.criterion
	}
	return fmt.Sprintf("%s#%d", t.task, t.index)
}

func (c *cli) targets(w *workspace, names []string, baseline bool) ([]target, error) {
	g, b := w.goal, w.bundle
	if len(names) == 0 {
		for _, r := range b.Rows {
			if baseline && r.Kind == bundle.KindChange {
				names = append(names, r.ID)
				continue
			}
			crit := g.Criterion(r.ID)
			if !baseline && crit != nil && !crit.Optional && (r.Kind == bundle.KindChange || r.Kind == bundle.KindMaintain) {
				names = append(names, r.ID)
			}
		}
		if len(names) == 0 {
			return nil, errors.New("no targets: the plan has no matching conditions")
		}
	}
	var out []target
	for _, n := range names {
		if strings.HasPrefix(n, "T") {
			if baseline {
				return nil, fmt.Errorf("--baseline applies to change conditions, not to task %s", n)
			}
			t := b.Task(n)
			if t == nil {
				return nil, fmt.Errorf("%s is not a task in PLAN.md", n)
			}
			if len(t.Commands) == 0 {
				return nil, fmt.Errorf("%s has no commands in its task document's `## 완료 조건`", n)
			}
			for i, cmd := range t.Commands {
				argv, err := bundle.SplitCommand(cmd)
				if err != nil {
					return nil, fmt.Errorf("%s command %d: %v", n, i+1, err)
				}
				out = append(out, target{task: n, index: i + 1, command: cmd, argv: argv})
			}
			continue
		}
		r := b.Row(n)
		if r == nil {
			return nil, fmt.Errorf("%s is not in `## 조건표`", n)
		}
		switch {
		case r.Kind == bundle.KindManual:
			return nil, fmt.Errorf("%s is a manual condition; it is satisfied by `ha note confirm`, not by a check", n)
		case baseline && r.Kind != bundle.KindChange:
			return nil, fmt.Errorf("--baseline applies only to change conditions; %s is %s", n, r.Kind)
		}
		out = append(out, target{criterion: n, kind: r.Kind, command: r.Command, argv: r.Argv, checks: r.CheckPaths})
	}
	return out, nil
}

func (c *cli) check(in []string) int {
	a, err := parseArgs(in, nil, []string{"baseline"})
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	start := startLine(lines)
	if start != nil && !status.Supported(start.Rules) {
		return c.fail(exitRules, "%v", &status.UnsupportedRulesError{Rules: start.Rules})
	}
	if bad != nil {
		return c.fail(exitRefused, "%v", bad)
	}
	if start == nil {
		return c.fail(exitRefused, "the goal has not started; run ha start first")
	}
	if c.printLint(w) > 0 {
		return c.fail(exitRefused, "fix the lint problems above before ha check")
	}
	baseline := a.bools["baseline"]
	ts, err := c.targets(w, a.pos[1:], baseline)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if err := os.MkdirAll(w.logDir, 0o755); err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	excl, err := w.excludes(start.Rules)
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	allGood := true
	// Each target that did not meet its expectation, in run order, for the
	// output pointers printed after the result lines.
	type unmet struct {
		label  string
		seq    int
		output string
	}
	var missed []unmet
	// The pointers are printed however the batch ends, so a target that did
	// not meet its expectation keeps its pointer when a later target is
	// refused or cannot be recorded.
	defer func() {
		for _, m := range missed {
			if m.output != "" {
				fmt.Fprintf(c.out, "output for %s: ha log %s %d\n", m.label, status.DisplayGoal(w.arg, w.goal.Dir), m.seq)
			} else {
				fmt.Fprintf(c.out, "output for %s: no stored output (seq %d)\n", m.label, m.seq)
			}
		}
	}()
	for i, t := range ts {
		head, err := w.repo.Head()
		if err != nil {
			return c.fail(exitRefused, "%v", err)
		}
		if code := c.checkPreflight(w, excl, t.label(), i, len(ts)); code != exitOK {
			return code
		}
		clean := true
		var invocation *evidence.Invocation
		var declaration *evidence.Declaration
		var binding string
		var extraEnv []string
		limit := checkTimeout
		if start.Rules == status.RulesV4 {
			limit = w.bundle.Limit(t.label())
			if notice := timeLimitNotice(lines, t.label()); notice != "" {
				fmt.Fprintln(c.out, notice)
			}
		}
		t.limit = limit
		digest := bundle.Digest(t.command)
		if start.Rules == status.RulesV4 {
			digest = bundle.LimitDigest(digest, limit)
		}
		if row := w.bundle.Row(t.criterion); row != nil {
			digest = row.CommandDigestFor(start.Rules)
			if status.Structured(start.Rules) && t.kind == bundle.KindChange {
				declaration, binding = row.Evidence, row.EvidenceDigest
				if declaration != nil && declaration.Adapter != "" {
					invocation, err = evidence.PrepareAdapter(w.logDir, declaration)
					if err != nil {
						return c.fail(exitInternal, "before execution: prepare runner output location: %v", err)
					}
					defer invocation.Close()
					extraEnv = invocation.AdapterEnv()
					t.execArgv, t.stdout = invocation.CommandArgv(t.argv), invocation.Stdout()
				} else {
					invocation, err = evidence.Prepare(w.logDir)
					if err != nil {
						return c.fail(exitInternal, "before execution: prepare result report: %v", err)
					}
					defer invocation.Close()
					extraEnv = invocation.Env(binding)
				}
			}
		}
		tmp, err := os.CreateTemp(w.logDir, ".pending-*.log")
		if err != nil {
			return c.fail(exitInternal, "%v", err)
		}
		tmp.Close()
		var res run.Result
		var overlay []record.Overlay
		var startedAt string
		if start.Rules == status.RulesV4 {
			startedAt = time.Now().UTC().Format(time.RFC3339)
		}
		if baseline {
			res, overlay, err = c.runBaseline(w, start.BaseCommit, head, t, tmp.Name(), extraEnv...)
		} else {
			res, err = c.executeCheckOutput(w.repo.Root, t.runArgv(), limit, tmp.Name(), extraEnv, t.stdout)
		}
		if baseline && !res.Attempted {
			os.Remove(tmp.Name())
			if err == nil {
				err = errors.New("baseline preparation did not finish")
			}
			return c.fail(exitRefused, "before execution: baseline preparation for %s: %v; %d/%d targets executed, %d not run", t.label(), err, i, len(ts), len(ts)-i)
		}
		if err != nil {
			if !res.Attempted || !status.Structured(start.Rules) {
				os.Remove(tmp.Name())
				return c.fail(exitInternal, "verification could not be recorded: %v", err)
			}
			// The process was attempted. Keep its usage even if its output
			// could not be finalized; do not promote the partial output.
			res.Outcome, res.Reason = run.Error, "process_output"
			fmt.Fprintln(c.err, "ha: attempted verification had an output storage error; recording an error, not usable evidence")
		}
		var outputPath string
		var outputLost bool
		prepare := func(seq int) error {
			name := strconv.Itoa(seq) + ".log"
			if err := os.Rename(tmp.Name(), filepath.Join(w.logDir, name)); err != nil {
				if !status.Structured(start.Rules) || !res.Attempted {
					return err
				}
				// Output storage is distinct from record storage. Preserve an
				// attempted run even when its raw output cannot be retained.
				outputLost = true
				res.Outcome, res.Reason, res.Digest = run.Error, "process_output", ""
				fmt.Fprintln(c.err, "ha: output unavailable; recording the attempted verification as an error without a log reference")
				return nil
			}
			outputPath = w.logRel + "/" + name
			return nil
		}
		result := res.Outcome
		if baseline {
			result = map[string]string{run.Pass: "unexpected_pass", run.Fail: "fail_as_expected", run.Error: "error"}[res.Outcome]
		}
		var proof *evidence.Summary
		if invocation != nil {
			summary := invocation.Classify(declaration, binding, res, baseline)
			proof, result = &summary, summary.Result
			_ = invocation.Close()
		}
		finalizeOutput := func() {
			if !outputLost {
				return
			}
			result = run.Error
			if proof != nil {
				*proof = invocation.Classify(declaration, binding, res, baseline)
				result = proof.Result
			}
		}
		tgt := record.Target{Criterion: t.criterion, Task: t.task, Index: t.index}
		var entry record.Entry
		if baseline {
			bl := &record.Baseline{
				Header: record.Header{Kind: "baseline", HaVersion: version}, Target: tgt, CriterionKind: t.kind,
				Argv: t.argv, CommandDigest: digest, Commit: head, TreeClean: clean, SpecDigest: w.goal.Digest,
				BaseCommit: start.BaseCommit, Overlay: overlay, Exit: res.Exit, Result: result,
				DurationMS: res.Duration.Milliseconds(), OutputDigest: res.Digest,
				Evidence: proof, Timing: timing(res, limit), StartedAt: startedAt,
			}
			entry = bl
			prepareBase := prepare
			prepare = func(seq int) error {
				if err := prepareBase(seq); err != nil {
					return err
				}
				finalizeOutput()
				bl.Result, bl.OutputDigest = result, res.Digest
				bl.OutputPath = outputPath
				return nil
			}
		} else {
			ck := &record.Check{
				Header: record.Header{Kind: "check", HaVersion: version}, Target: tgt, CriterionKind: t.kind,
				Argv: t.argv, CommandDigest: digest, Commit: head, TreeClean: clean, SpecDigest: w.goal.Digest,
				Exit: res.Exit, Result: result, DurationMS: res.Duration.Milliseconds(), OutputDigest: res.Digest,
				Evidence: proof, Timing: timing(res, limit), StartedAt: startedAt,
			}
			entry = ck
			prepareBase := prepare
			prepare = func(seq int) error {
				if err := prepareBase(seq); err != nil {
					return err
				}
				finalizeOutput()
				ck.Result, ck.OutputDigest = result, res.Digest
				ck.OutputPath = outputPath
				return nil
			}
		}
		l, err := record.Append(w.records, w.lock, entry, time.Now(), prepare)
		if err != nil {
			os.Remove(tmp.Name())
			if errors.Is(err, record.ErrBroken) {
				return c.fail(exitRefused, "%v", err)
			}
			return c.fail(exitInternal, "%v", err)
		}
		kind := "check"
		if baseline {
			kind = "baseline"
		}
		lines = append(lines, l) // later targets' notices see this run
		fmt.Fprintf(c.out, "%s %s %s (seq %d)\n", t.label(), kind, result, l.Seq)
		if proof != nil {
			fmt.Fprintf(c.out, "classification: %s\n", proof.Reason)
			for _, notice := range invocation.Notices() {
				fmt.Fprintf(c.out, "notice: %s %s\n", t.label(), notice)
			}
		}
		if result != run.Pass && result != "fail_as_expected" {
			allGood = false
			missed = append(missed, unmet{t.label(), l.Seq, outputPath})
		}
	}
	if allGood {
		return exitOK
	}
	return exitProblems
}

// Notice before a run-rules/4 run (run-record.md §4.7): earlier runs of the same
// target that reached their limit or used at least noticePermille of it.
const (
	noticePrefix   = "time-limit notice:"
	noticePermille = 800
)

// timeLimitNotice reads the record only. It names the latest earlier run of
// the target that reached or neared its limit, says its time risk is unknown
// when no earlier run kept a limit, and is empty otherwise. A run without its
// limit interval is judged from duration_ms and marked approximate.
func timeLimitNotice(lines []record.Line, label string) string {
	known := false
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if (l.Kind != "check" && l.Kind != "baseline") || l.Target == nil ||
			bundle.LimitLabel(l.Target.Criterion, l.Target.Task, l.Target.Index) != label {
			continue
		}
		r := timingOf(l)
		if r.LimitMS == nil || *r.LimitMS <= 0 {
			continue
		}
		known = true
		switch {
		case r.LimitReached != nil && *r.LimitReached:
			return fmt.Sprintf("%s %s seq %d limit reached", noticePrefix, label, l.Seq)
		case r.LimitUsePermille != nil:
			if *r.LimitUsePermille >= noticePermille {
				return fmt.Sprintf("%s %s seq %d %d‰ of the limit", noticePrefix, label, l.Seq, *r.LimitUsePermille)
			}
		case r.LimitElapsedMS == nil && l.DurationMS >= 0 && l.DurationMS <= math.MaxInt64/1000:
			if use := l.DurationMS * 1000 / *r.LimitMS; use >= noticePermille {
				return fmt.Sprintf("%s %s seq %d %d‰ of the limit (approximate)", noticePrefix, label, l.Seq, use)
			}
		}
	}
	if !known {
		return fmt.Sprintf("%s %s risk unknown", noticePrefix, label)
	}
	return ""
}

// timing is the record form of a run's process measurement (run-record.md
// §2.2). limit is the time limit ha applied to the run.
func timing(res run.Result, limit time.Duration) *record.Timing {
	t := &record.Timing{LimitMS: limit.Milliseconds(), LimitReached: res.LimitReached,
		CPU: record.CPU{Status: res.CPU.Status, Scope: res.CPU.Scope, Source: res.CPU.Source}}
	if res.LimitElapsed != nil {
		ms := res.LimitElapsed.Milliseconds()
		t.LimitElapsedMS = &ms
	}
	switch res.CPU.Status {
	case run.CPUMeasured:
		user, system := res.CPU.User.Milliseconds(), res.CPU.System.Milliseconds()
		t.CPU.UserMS, t.CPU.SystemMS = &user, &system
	case run.CPUFailed:
	default:
		t.CPU.Status = run.CPUUnavailable
	}
	return t
}

// runBaseline runs the command in a temporary worktree of the base commit with
// the condition's check files copied from HEAD. The user's working tree and
// index are not touched.
func (c *cli) runBaseline(w *workspace, base, head string, t target, logPath string, extraEnv ...string) (run.Result, []record.Overlay, error) {
	parent := filepath.Join(w.repo.GitDir, "ha", "worktrees")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return run.Result{}, nil, err
	}
	holder, err := os.MkdirTemp(parent, "base-")
	if err != nil {
		return run.Result{}, nil, err
	}
	defer os.RemoveAll(holder)
	tree := filepath.Join(holder, "tree")
	if err := w.repo.AddWorktree(tree, base); err != nil {
		return run.Result{}, nil, err
	}
	defer w.repo.RemoveWorktree(tree)
	files, err := baselineFiles(w.repo, base, head, t.checks)
	if err != nil {
		return run.Result{}, nil, err
	}
	root, err := os.OpenRoot(tree)
	if err != nil {
		return run.Result{}, nil, err
	}
	defer root.Close()
	overlay, err := applyBaseline(root, files)
	if err != nil {
		return run.Result{}, nil, err
	}
	limit := t.limit
	if limit == 0 {
		limit = checkTimeout
	}
	res, err := c.executeCheckOutput(tree, t.runArgv(), limit, logPath, extraEnv, t.stdout)
	return res, overlay, err
}

var blockCauses = []string{"auth", "permission", "quota", "environment", "spec", "other"}

// rules4Notes are the note kinds only run-rules/4 goals record (run-record.md
// §4.7); earlier rules refuse them before writing.
var rules4Notes = map[string]bool{"test-change": true, "wait": true, "resume": true, "recover": true, "allow-recovery": true}

// noteFlags are the run-rules/4 options each note kind takes.
var noteFlags = map[string]map[string]bool{
	"test-change":    {"criteria": true, "how": true, "moved-to": true, "weakened": true},
	"wait":           {"reason": true},
	"recover":        {"input": true, "change": true},
	"allow-recovery": {"input": true, "count": true},
}

func (c *cli) note(in []string) int {
	a, err := parseFlags(in, []string{"quote", "cause", "reason", "criteria", "how", "input", "change", "count"}, []string{"moved-to"}, []string{"weakened"})
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if len(a.pos) < 2 {
		return c.fail(exitUsage, "usage: ha note <goal> <block|unblock|confirm|reopen|input|test-change|wait|resume|recover|allow-recovery> ...")
	}
	// The run-rules/4 options belong to their note kinds; every other kind
	// refuses them as an unknown flag, as before they existed.
	for _, name := range []string{"reason", "criteria", "how", "input", "change", "count", "moved-to", "weakened"} {
		_, val := a.vals[name]
		_, list := a.lists[name]
		if (val || list || a.bools[name]) && !noteFlags[a.pos[1]][name] {
			return c.fail(exitUsage, "unknown flag --%s for %s notes", name, a.pos[1])
		}
	}
	w, code := c.openOrFail(a, 2)
	if w == nil {
		return code
	}
	lines, _, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	goalRules := rules(lines)
	if !status.Supported(goalRules) {
		return c.fail(exitRules, "%v", &status.UnsupportedRulesError{Rules: goalRules})
	}
	kind, quote := a.pos[1], strings.TrimSpace(a.vals["quote"])
	if rules4Notes[kind] && goalRules != status.RulesV4 {
		return c.fail(exitRefused, "%s notes need a goal started with %s; this goal keeps %s", kind, status.RulesV4, goalRules)
	}
	e := &record.Note{Header: record.Header{Kind: "note", HaVersion: version}, Note: kind, Quote: quote}
	var validate func([]record.Line) error
	if kind == "reopen" && status.Structured(goalRules) {
		before, err := status.FoldBudget(lines)
		if err != nil {
			return c.fail(exitRefused, "%v", err)
		}
		validate = func(current []record.Line) error {
			after, err := status.FoldBudget(current)
			if err != nil {
				return err
			}
			if after.WindowStart != before.WindowStart || after.Revision != before.Revision {
				return fmt.Errorf("budget conflict before reopen: current window %d, revision %d; read ha status and decide again", after.WindowStart, after.Revision)
			}
			return nil
		}
	}
	switch kind {
	case "block":
		cause := a.vals["cause"]
		valid := false
		for _, cs := range blockCauses {
			valid = valid || cs == cause
		}
		if !valid {
			return c.fail(exitUsage, "block needs --cause %s", strings.Join(blockCauses, "|"))
		}
		e.Cause = cause
	case "unblock":
	case "confirm":
		if len(a.pos) < 3 {
			return c.fail(exitUsage, "confirm needs a subject: AC-n, scope:<path>, tests:<path>, or spec")
		}
		if quote == "" {
			return c.fail(exitUsage, "confirm needs --quote with the user's words")
		}
		e.Subject = a.pos[2]
		bound, code, err := c.bound(w, e.Subject, goalRules)
		if err != nil {
			return c.fail(code, "%v", err)
		}
		e.Bound = bound
	case "reopen", "input":
		if quote == "" {
			return c.fail(exitUsage, "%s needs --quote with the user's words", kind)
		}
	case "test-change":
		if len(a.pos) < 3 || !strings.HasPrefix(a.pos[2], "tests:") || a.pos[2] == "tests:" {
			return c.fail(exitUsage, "test-change needs a subject tests:<path>")
		}
		e.Subject = a.pos[2]
		data, code, err := c.testChange(w, strings.TrimPrefix(e.Subject, "tests:"), a, goalRules)
		if err != nil {
			return c.fail(code, "%v", err)
		}
		e.TestChange, e.Bound = &data.TestChangeData, &record.Bound{Blob: data.boundBlob}
	case "wait":
		if e.Reason = strings.TrimSpace(a.vals["reason"]); e.Reason == "" {
			return c.fail(exitUsage, "wait needs --reason naming the decision the user was asked")
		}
	case "resume":
	case "recover", "allow-recovery":
		if len(a.pos) < 3 {
			return c.fail(exitUsage, "%s needs a condition ID", kind)
		}
		data := &record.RecoveryData{Criterion: a.pos[2], Input: a.vals["input"]}
		row := w.bundle.Row(data.Criterion)
		if row == nil || row.Kind == bundle.KindManual {
			return c.fail(exitUsage, "%s: %s is not a condition with a command", kind, data.Criterion)
		}
		if data.Input != "check" && (data.Input != "baseline" || row.Kind != bundle.KindChange) {
			return c.fail(exitUsage, "%s needs --input check, or --input baseline for a change condition", kind)
		}
		if kind == "recover" {
			if data.Change = strings.TrimSpace(a.vals["change"]); data.Change == "" {
				return c.fail(exitUsage, "recover needs --change naming what changed in how the check runs")
			}
		} else {
			n, err := strconv.Atoi(a.vals["count"])
			if err != nil || n <= 0 {
				return c.fail(exitUsage, "allow-recovery needs --count <positive number>")
			}
			if quote == "" {
				return c.fail(exitUsage, "allow-recovery needs --quote with the user's words")
			}
			data.Count = n
		}
		e.Recovery = data
		if kind == "recover" {
			validate = func(current []record.Line) error { return c.recoverable(w, current, data) }
		}
	default:
		return c.fail(exitUsage, "unknown note kind %q", kind)
	}
	l, err := record.AppendChecked(w.records, w.lock, e, time.Now(), validate, nil)
	if err != nil {
		if errors.Is(err, record.ErrBroken) {
			return c.fail(exitRefused, "%v", err)
		}
		var uncertain *record.UncertainWriteError
		if validate != nil && !errors.As(err, &uncertain) {
			return c.fail(exitRefused, "%v", err)
		}
		return c.fail(exitInternal, "%v", err)
	}
	fmt.Fprintf(c.out, "noted: %s (seq %d)\n", strings.TrimSpace(kind+" "+e.Subject), l.Seq)
	return exitOK
}

type testChangeNote struct {
	record.TestChangeData
	boundBlob string
}

// testChange reads a test-change note's reason and binds it to the file at
// HEAD, and each moved-to path to its file (run-record.md §4.7).
func (c *cli) testChange(w *workspace, path string, a *args, rules string) (*testChangeNote, int, error) {
	n := &testChangeNote{TestChangeData: record.TestChangeData{How: strings.TrimSpace(a.vals["how"]), Weakened: a.bools["weakened"]}}
	for _, id := range strings.Split(a.vals["criteria"], ",") {
		if id = strings.TrimSpace(id); id == "" {
			continue
		}
		if w.goal.Criterion(id) == nil && w.bundle.Row(id) == nil {
			return nil, exitUsage, fmt.Errorf("test-change: %s is not a condition of this goal", id)
		}
		n.Criteria = append(n.Criteria, id)
	}
	if len(n.Criteria) == 0 || n.How == "" {
		return nil, exitUsage, errors.New("test-change needs --criteria <AC-n,...> and --how saying how the file checks them")
	}
	excl, err := w.excludes(rules)
	if err != nil {
		return nil, exitInternal, err
	}
	clean, err := w.repo.Clean(excl)
	if err != nil {
		return nil, exitInternal, err
	}
	if !clean {
		return nil, exitRefused, errors.New("commit or revert changes before recording a test change; it binds to the files at HEAD")
	}
	head, err := w.repo.Head()
	if err != nil {
		return nil, exitRefused, err
	}
	if n.boundBlob, err = w.repo.Blob(head, path); err != nil {
		return nil, exitInternal, err
	}
	for _, moved := range a.lists["moved-to"] {
		blob, err := w.repo.Blob(head, moved)
		if err != nil {
			return nil, exitInternal, err
		}
		if blob == gitx.Deleted {
			return nil, exitRefused, fmt.Errorf("test-change: moved-to path %s does not exist at HEAD", moved)
		}
		n.MovedTo = append(n.MovedTo, record.MovedPath{Path: moved, Blob: blob})
	}
	return n, 0, nil
}

// recoverable refuses a recover note unless the condition's input is past its
// error limit and a recovery remains (run-record.md §4.7).
func (c *cli) recoverable(w *workspace, lines []record.Line, d *record.RecoveryData) error {
	rep, err := status.Compute(status.Input{Repo: w.repo, Goal: w.goal, Bundle: w.bundle, Lines: lines, HaVersion: version})
	if err != nil {
		return err
	}
	for _, r := range rep.Recoveries {
		if r.Criterion != d.Criterion || r.Input != d.Input {
			continue
		}
		if !r.LimitReached {
			return fmt.Errorf("%s %s input is not past its error limit; nothing to recover", d.Criterion, d.Input)
		}
		if r.Remaining <= 0 {
			return fmt.Errorf("%s %s input has no recovery left; the user can allow more with allow-recovery", d.Criterion, d.Input)
		}
		d.Clears = append([]int(nil), r.ErrorSeqs...)
		return nil
	}
	return fmt.Errorf("%s %s input is not past its error limit; nothing to recover", d.Criterion, d.Input)
}

// bound computes the value a confirmation is tied to (run-record.md §2.4).
func (c *cli) bound(w *workspace, subject, rules string) (*record.Bound, int, error) {
	switch {
	case subject == "spec":
		return &record.Bound{SpecDigest: w.goal.Digest}, 0, nil
	case strings.HasPrefix(subject, "scope:") || strings.HasPrefix(subject, "tests:"):
		p := subject[strings.IndexByte(subject, ':')+1:]
		if p == "" {
			return nil, exitUsage, fmt.Errorf("%s needs a path", subject)
		}
		excl, err := w.excludes(rules)
		if err != nil {
			return nil, exitInternal, err
		}
		clean, err := w.repo.Clean(excl)
		if err != nil {
			return nil, exitInternal, err
		}
		if !clean {
			return nil, exitRefused, errors.New("commit or revert changes before recording a path confirmation; it binds to the file at HEAD")
		}
		head, err := w.repo.Head()
		if err != nil {
			return nil, exitRefused, err
		}
		blob, err := w.repo.Blob(head, p)
		if err != nil {
			return nil, exitInternal, err
		}
		return &record.Bound{Blob: blob}, 0, nil
	default:
		crit := w.goal.Criterion(subject)
		if crit == nil {
			return nil, exitUsage, fmt.Errorf("%s is not a criterion in the goal documents", subject)
		}
		if r := w.bundle.Row(subject); r == nil || r.Kind != bundle.KindManual {
			return nil, exitRefused, fmt.Errorf("confirm %s is only for manual conditions; machine-checkable conditions need ha check", subject)
		}
		return &record.Bound{CriterionSHA256: crit.TextSHA256(), SpecDigest: w.goal.Digest}, 0, nil
	}
}

func (c *cli) compute(w *workspace) (*status.Report, *record.IntegrityError, int) {
	lines, bad, err := w.read()
	if err != nil {
		return nil, nil, c.fail(exitInternal, "%v", err)
	}
	rep, err := status.Compute(status.Input{Repo: w.repo, Goal: w.goal, Bundle: w.bundle, Lines: lines, Integrity: bad, HaVersion: version})
	var ur *status.UnsupportedRulesError
	if errors.As(err, &ur) {
		return nil, nil, c.fail(exitRules, "%v", err)
	}
	if err != nil {
		return nil, nil, c.fail(exitInternal, "%v", err)
	}
	return rep, bad, -1
}

func (c *cli) status(in []string, done bool) int {
	a, err := parseArgs(in, []string{"format"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	format := a.vals["format"]
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "json" {
		return c.fail(exitUsage, "--format must be md or json")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	rep, bad, code := c.compute(w)
	if rep == nil {
		return code
	}
	if done {
		if bad != nil {
			c.printDone(rep, w, format)
			fmt.Fprintf(c.err, "ha: not recorded: %v\n", bad)
			return rep.ExitCode
		}
		if _, err := record.Append(w.records, w.lock, rep.DoneEntry(version), time.Now(), nil); err != nil {
			return c.fail(exitInternal, "%v", err)
		}
		if rep, _, code = c.compute(w); rep == nil {
			return code
		}
	}
	show := c.print
	if done {
		show = c.printDone
	}
	if err := show(rep, w, format); err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	return rep.ExitCode
}

// printDone prints `ha done` output, which keeps its earlier form.
func (c *cli) printDone(rep *status.Report, w *workspace, format string) error {
	if format == "json" {
		return c.print(rep, w, format)
	}
	_, err := io.WriteString(c.out, rep.DoneMarkdown(w.arg))
	return err
}

func (c *cli) print(rep *status.Report, w *workspace, format string) error {
	if format == "json" {
		b, err := rep.JSON()
		if err != nil {
			return err
		}
		_, err = c.out.Write(b)
		return err
	}
	_, err := io.WriteString(c.out, rep.Markdown(w.arg))
	return err
}

func (c *cli) log(in []string) int {
	a, err := parseArgs(in, []string{"tail"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if len(a.pos) < 2 {
		return c.fail(exitUsage, "usage: ha log <goal> <seq> [--tail N]")
	}
	w, code := c.openOrFail(a, 2)
	if w == nil {
		return code
	}
	seq, err := strconv.Atoi(a.pos[1])
	if err != nil || seq < 1 {
		return c.fail(exitUsage, "seq must be a positive number")
	}
	data, err := os.ReadFile(filepath.Join(w.logDir, strconv.Itoa(seq)+".log"))
	if err != nil {
		return c.fail(exitRefused, "no local output for seq %d (output is kept only in this clone's .git/ha)", seq)
	}
	if t := a.vals["tail"]; t != "" {
		n, err := strconv.Atoi(t)
		if err != nil || n < 0 {
			return c.fail(exitUsage, "--tail must be a non-negative number")
		}
		lines := strings.SplitAfter(string(data), "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		if n < len(lines) {
			lines = lines[len(lines)-n:]
		}
		data = []byte(strings.Join(lines, ""))
	}
	_, _ = c.out.Write(data)
	return exitOK
}
