package spar

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Dispositions a finding can be given.
var Dispositions = []string{"fix", "reject", "defer", "duplicate", "answer"}

// TriageOptions are the inputs of `spar triage`.
type TriageOptions struct {
	Round        string
	Finding      string
	Disposition  string
	Commit       string
	FailedBefore string
	PassedAfter  string
	Evidence     string
	Of           string
	To           string
	UserQuote    string
	Note         string
	Log          io.Writer
}

// entryNames names the fields of a disposition where they are given: as
// spar triage's options, or as the keys of a ledger line, quoted as JSON
// keys.
type entryNames struct{ commit, failedBefore, passedAfter, evidence, of, to, userQuote string }

var (
	triageOptions = entryNames{"--commit", "--failed-before", "--passed-after", "--evidence", "--of", "--to", "--user-quote"}
	ledgerKeys    = entryNames{`"commit"`, `"failed_before"`, `"passed_after"`, `"evidence"`, `"of"`, `"to"`, `"user_quote"`}
)

// fieldsProblem says why the fields of a disposition do not fit it or the
// finding's class (contracts/review-round.md §7). spar triage applies it
// before it writes an entry and spar cap-report before an entry it reads
// closes or excludes a finding, so both hold the same rules. It reads no
// Git: whether a fix commit comes after the reviewed head is triage's own
// check.
func fieldsProblem(f Finding, findings []Finding, e LedgerEntry, n entryNames) string {
	switch e.Disposition {
	case "fix":
		if e.Commit == "" || strings.TrimSpace(e.FailedBefore) == "" || strings.TrimSpace(e.PassedAfter) == "" {
			return fmt.Sprintf("a fix needs %s, %s and %s: the fix commit, the observation that failed before it and the one that passes after it",
				n.commit, n.failedBefore, n.passedAfter)
		}
	case "reject":
		if f.Class != "requirement-question" && strings.TrimSpace(e.Evidence) == "" {
			return fmt.Sprintf("a reject needs %s that disproves the finding", n.evidence)
		}
	case "defer":
		if strings.TrimSpace(e.To) == "" {
			return fmt.Sprintf("a defer needs %s: where the finding goes", n.to)
		}
		if f.Class == "defect" && strings.TrimSpace(e.UserQuote) == "" {
			return fmt.Sprintf("deferring a defect needs %s with the user's words", n.userQuote)
		}
	case "duplicate":
		if e.Of == "" || e.Of == f.ID {
			return fmt.Sprintf("a duplicate needs %s another finding of the same round", n.of)
		}
		var target *Finding
		for i := range findings {
			if findings[i].ID == e.Of {
				target = &findings[i]
			}
		}
		if target == nil {
			return fmt.Sprintf("%s %s is not a finding of this round", n.of, e.Of)
		}
		// A defect or requirement question closes only through a finding
		// of its own class, and only once that finding is closed (gate).
		if (f.Class == "defect" || f.Class == "requirement-question") && target.Class != f.Class {
			return fmt.Sprintf("a %s can only duplicate another %s; %s is %s", f.Class, f.Class, target.ID, target.Class)
		}
	case "answer":
		if f.Class != "requirement-question" {
			return "only a requirement question is answered; this finding is " + f.Class
		}
	default:
		return fmt.Sprintf("unknown disposition %q (%s)", e.Disposition, strings.Join(Dispositions, ", "))
	}
	return ""
}

// quoteProblem says why closing the finding with this disposition needs
// the user's words that the entry does not have.
func quoteProblem(f Finding, e LedgerEntry, n entryNames) string {
	quote := strings.TrimSpace(e.UserQuote) != ""
	if f.Class == "requirement-question" && e.Disposition != "duplicate" && !quote {
		return fmt.Sprintf("closing a requirement question needs %s with the user's words", n.userQuote)
	}
	if f.Class == "hardening" && e.Disposition == "fix" && !quote {
		return fmt.Sprintf("fixing a deferred hardening item in this change needs %s with the user's words", n.userQuote)
	}
	return ""
}

// entryProblem says why a ledger line may not close or exclude its finding.
// An automatic entry is valid only in the form spar run writes: the
// deferral of a hardening finding with a reason. Every other entry needs
// the fields spar triage requires before it writes one.
func entryProblem(f Finding, findings []Finding, e LedgerEntry) string {
	if e.Auto {
		if e.Disposition == "defer" && f.Class == "hardening" && strings.TrimSpace(e.Reason) != "" {
			return ""
		}
		return "an automatic entry is spar's deferral of a hardening finding, with a reason"
	}
	if problem := fieldsProblem(f, findings, e, ledgerKeys); problem != "" {
		return problem
	}
	return quoteProblem(f, e, ledgerKeys)
}

// dispositionProblem says why a disposition does not fit a finding's class.
func dispositionProblem(f Finding, findings []Finding, o TriageOptions, repo, head string) (string, string) {
	e := LedgerEntry{Disposition: o.Disposition, Commit: o.Commit, FailedBefore: o.FailedBefore, PassedAfter: o.PassedAfter,
		Evidence: o.Evidence, Of: o.Of, To: o.To, UserQuote: o.UserQuote}
	if problem := fieldsProblem(f, findings, e, triageOptions); problem != "" {
		return problem, ""
	}
	var commit string
	if o.Disposition == "fix" {
		c, err := resolveCommit(repo, o.Commit)
		if err != nil {
			return "--commit: " + err.Error(), ""
		}
		if c == head || !isAncestor(repo, head, c) {
			return fmt.Sprintf("the fix commit %s does not come after the reviewed head %s", shortID(c), shortID(head)), ""
		}
		commit = c
	}
	if problem := quoteProblem(f, e, triageOptions); problem != "" {
		return problem, ""
	}
	return "", commit
}

// Triage appends one disposition to a round's ledger.
func Triage(o TriageOptions) error {
	dir := realish(o.Round)
	unlock, err := lockSeries(filepath.Dir(dir), o.Log)
	if err != nil {
		return refused("%s cannot be locked: %v", o.Round, err)
	}
	defer unlock()
	r, err := loadRound(dir)
	if err != nil {
		return refused("%v", err)
	}
	findings, err := loadFindings(dir)
	if err != nil {
		return refused("%v", err)
	}
	var f *Finding
	for i := range findings {
		if findings[i].ID == o.Finding {
			f = &findings[i]
		}
	}
	if f == nil {
		return refused("%s is not a finding of %s", o.Finding, o.Round)
	}
	// A round whose ledger is gone or unreadable is refused, not given a new
	// one (G32): its earlier dispositions would be lost.
	if _, err := os.Stat(filepath.Join(dir, "ledger.jsonl")); err != nil {
		return refused("%s/ledger.jsonl cannot be read: %v", o.Round, err)
	}
	if _, err := loadLedger(dir); err != nil {
		return refused("%v", err)
	}
	problem, commit := dispositionProblem(*f, findings, o, r.Repo, r.Head)
	if problem != "" {
		return refused("%s %s (%s): %s", o.Finding, o.Disposition, f.Class, problem)
	}
	entry := LedgerEntry{Finding: f.ID, Disposition: o.Disposition, Commit: commit, FailedBefore: o.FailedBefore,
		PassedAfter: o.PassedAfter, Evidence: o.Evidence, Of: o.Of, To: o.To, UserQuote: o.UserQuote, Note: o.Note,
		RecordedAt: now()}
	if err := appendJSONLine(filepath.Join(dir, "ledger.jsonl"), entry); err != nil {
		return err
	}
	ledger, err := loadLedger(dir)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(renderSummary(dir, r, findings, ledger)), 0o644)
}

// Extend records the user's words allowing more rounds in a series. A code
// extension also records why the cap ran out: one of Causes, and a reason,
// which --by 2 or more and the cause other need.
func Extend(series, stage string, by int, cause, reason, quote string, repoHint string, log io.Writer) error {
	group := ""
	switch stage {
	case "plan", "plan-delta", "code":
		group = stage
	default:
		return usageError("--stage must be plan, plan-delta or code")
	}
	if by < 1 {
		return usageError("--by must be a positive number of rounds")
	}
	reason = strings.TrimSpace(reason)
	if group != "code" {
		if cause != "" || reason != "" {
			return usageError("--cause and --reason are only for --stage code")
		}
	} else if !slices.Contains(Causes, cause) {
		return usageError("--stage code needs --cause %s", strings.Join(Causes, ", "))
	}
	if strings.TrimSpace(quote) == "" {
		return refused("an extension needs --user-quote with the user's words")
	}
	switch {
	case group == "code" && by > 1 && reason == "":
		return refused("a code extension by %d rounds needs --reason", by)
	case cause == "other" && reason == "":
		return refused("--cause other needs --reason")
	}
	path := realish(series)
	start := repoHint
	if start == "" {
		start = "."
	}
	// Without a reviewed repository the path is still checked against the
	// repository it lies in, if any.
	repo := ""
	if root, err := repoRoot(start); err == nil {
		repo = realish(root)
	} else if repoHint != "" {
		return refused("--repo %s is not a Git repository: %v", repoHint, err)
	}
	if err := localRecordPath(repo, path); err != nil {
		return refused("%v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	unlock, err := lockSeries(path, log)
	if err != nil {
		return refused("%s cannot be locked: %v", series, err)
	}
	defer unlock()
	return appendJSONLine(filepath.Join(path, "extensions.jsonl"), Extension{Stage: group, By: by, Cause: cause, Reason: reason,
		UserQuote: quote, RecordedAt: now()})
}
