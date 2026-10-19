package spar

import (
	"fmt"
	"os"
	"path/filepath"
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
}

// dispositionProblem says why a disposition does not fit a finding's class.
func dispositionProblem(f Finding, findings []Finding, o TriageOptions, repo, head string) (string, string) {
	quote := strings.TrimSpace(o.UserQuote) != ""
	var commit string
	switch o.Disposition {
	case "fix":
		if o.Commit == "" || strings.TrimSpace(o.FailedBefore) == "" || strings.TrimSpace(o.PassedAfter) == "" {
			return "a fix needs --commit, --failed-before and --passed-after: the fix commit, the observation that failed before it and the one that passes after it", ""
		}
		c, err := resolveCommit(repo, o.Commit)
		if err != nil {
			return "--commit: " + err.Error(), ""
		}
		if c == head || !isAncestor(repo, head, c) {
			return fmt.Sprintf("the fix commit %s does not come after the reviewed head %s", shortID(c), shortID(head)), ""
		}
		commit = c
	case "reject":
		if f.Class != "requirement-question" && strings.TrimSpace(o.Evidence) == "" {
			return "a reject needs --evidence that disproves the finding", ""
		}
	case "defer":
		if strings.TrimSpace(o.To) == "" {
			return "a defer needs --to: where the finding goes", ""
		}
		if f.Class == "defect" && !quote {
			return "deferring a defect needs --user-quote with the user's words", ""
		}
	case "duplicate":
		if o.Of == "" || o.Of == f.ID {
			return "a duplicate needs --of another finding of the same round", ""
		}
		var target *Finding
		for i := range findings {
			if findings[i].ID == o.Of {
				target = &findings[i]
			}
		}
		if target == nil {
			return fmt.Sprintf("--of %s is not a finding of this round", o.Of), ""
		}
		if f.Class == "requirement-question" && target.Class != "requirement-question" {
			return "a requirement question can only duplicate another requirement question", ""
		}
	case "answer":
		if f.Class != "requirement-question" {
			return "only a requirement question is answered; this finding is " + f.Class, ""
		}
	default:
		return fmt.Sprintf("unknown disposition %q (%s)", o.Disposition, strings.Join(Dispositions, ", ")), ""
	}
	if f.Class == "requirement-question" && o.Disposition != "duplicate" && !quote {
		return "closing a requirement question needs --user-quote with the user's words", ""
	}
	if f.Class == "hardening" && o.Disposition == "fix" && !quote {
		return "fixing a deferred hardening item in this change needs --user-quote with the user's words", ""
	}
	return "", commit
}

// Triage appends one disposition to a round's ledger.
func Triage(o TriageOptions) error {
	dir := realish(o.Round)
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

// Extend records the user's words allowing more rounds in a series.
func Extend(series, stage string, by int, quote string, repoHint string) error {
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
	if strings.TrimSpace(quote) == "" {
		return refused("an extension needs --user-quote with the user's words")
	}
	path := realish(series)
	start := repoHint
	if start == "" {
		start = "."
	}
	if repo, err := repoRoot(start); err == nil {
		if err := localRecordPath(realish(repo), path); err != nil {
			return refused("%v", err)
		}
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return appendJSONLine(filepath.Join(path, "extensions.jsonl"), Extension{Stage: group, By: by, UserQuote: quote, RecordedAt: now()})
}
