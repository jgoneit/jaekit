package spar

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const usageText = `usage:
  spar --version
  spar config [--repo DIR] [--reviewers NAME,...] [--model NAME=MODEL]... [--effort NAME=EFFORT]...
  spar run --stage plan|plan-delta|candidate|fixes --base REV [--head REV] --out DIR
           --criteria FILE... [--rules FILE]... [--subject FILE]... [--question TEXT]...
           [--decision TEXT --decision-quote TEXT]...
           [--check-path GLOB]... [--reviewers NAME,...] [--model NAME=MODEL]... [--effort NAME=EFFORT]...
           [--repo DIR] [--label TEXT] [--dry-run]
  spar record ROUND native (--result FILE|- | --transcript FILE) [--model M] [--agent TEXT]
  spar triage ROUND FINDING fix|reject|defer|duplicate|answer [--commit REV] [--failed-before TEXT]
           [--passed-after TEXT] [--evidence TEXT] [--of FINDING] [--to TEXT] [--user-quote TEXT] [--note TEXT]
  spar extend SERIES --stage plan|plan-delta|code --by N [--cause CAUSE] [--reason TEXT] --user-quote TEXT
  spar cap-report SERIES [--format md|json] [--repo DIR]
  spar waive ROUND REVIEWER --user-quote TEXT
  spar gate ROUND... --expect-head REV --out FILE [--repo DIR]
  spar summary ROUND...

exit codes: 0 success, 1 no reviewer completed or is pending, 3 gate degraded, 4 gate blocked,
            64 usage, 65 refused, 70 internal error
`

type parsed struct {
	positional []string
	values     map[string][]string
	flags      map[string]bool
	// order holds the valued options in the order given, for options that
	// are read in pairs.
	order []option
	// missing names the option given last without its value.
	missing string
}

type option struct{ name, value string }

func (p parsed) one(name string) string {
	if v := p.values[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func (p parsed) has(name string) bool {
	_, ok := p.values[name]
	return ok
}

// parseArgs reads --name value, --name=value and bare flags in any order.
func parseArgs(args []string, valued, bare []string) (parsed, error) {
	p := parsed{values: map[string][]string{}, flags: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			p.positional = append(p.positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "--") || a == "-" {
			p.positional = append(p.positional, a)
			continue
		}
		name, value, inline := strings.Cut(a[2:], "=")
		switch {
		case contains(bare, name):
			if inline {
				return p, usageError("--%s takes no value", name)
			}
			p.flags[name] = true
		case contains(valued, name):
			if !inline {
				if i+1 >= len(args) {
					p.missing = name
					return p, usageError("--%s needs a value", name)
				}
				i++
				value = args[i]
			}
			p.values[name] = append(p.values[name], value)
			p.order = append(p.order, option{name, value})
		default:
			return p, usageError("unknown option --%s", name)
		}
	}
	return p, nil
}

func reviewerOverride(p parsed) []string {
	if !p.has("reviewers") {
		return nil
	}
	names := []string{}
	for _, v := range p.values["reviewers"] {
		for _, n := range strings.Split(v, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	}
	return names
}

// decisions pairs each --decision with the --decision-quote that follows it
// before the next --decision (G35). A part without its pair is refused: a
// round never records a decision without what it settles or without the
// user's words.
func decisions(p parsed) ([]Decision, error) {
	var out []Decision
	open := false
	for _, o := range p.order {
		switch o.name {
		case "decision":
			if open {
				return nil, refused("decision %d: --decision %q has no --decision-quote before the next --decision", len(out), out[len(out)-1].Settles)
			}
			out = append(out, Decision{Settles: o.value})
			open = true
		case "decision-quote":
			if !open {
				return nil, refused("decision %d: --decision-quote %q has no --decision before it", len(out)+1, o.value)
			}
			out[len(out)-1].UserQuote = o.value
			open = false
		}
	}
	if open {
		return nil, refused("decision %d: --decision %q has no --decision-quote after it", len(out), out[len(out)-1].Settles)
	}
	return out, nil
}

// missingDecisionValue is the refusal for a decision option given last
// without its value: the number of the decision it would start or end.
func missingDecisionValue(p parsed) error {
	n := 0
	for _, o := range p.order {
		if o.name == "decision" {
			n++
		}
	}
	if p.missing == "decision" || n == 0 {
		n++
	}
	return refused("decision %d: --%s needs a value: a decision has what it settles and the user's words", n, p.missing)
}

// agentChoices reads --model NAME=MODEL and --effort NAME=EFFORT (G25):
// the first "=" splits, and a value without one or without a name is a
// usage error. Models come first, each key in the order given.
func agentChoices(p parsed) ([]Choice, error) {
	var out []Choice
	for _, key := range []string{"model", "effort"} {
		for _, v := range p.values[key] {
			name, value, ok := strings.Cut(v, "=")
			if !ok || name == "" {
				return nil, usageError("--%s needs NAME=%s, got %q", key, strings.ToUpper(key), v)
			}
			out = append(out, Choice{Reviewer: name, Key: key, Value: value})
		}
	}
	return out, nil
}

// Main runs the spar command line and returns its exit code.
func Main(version string, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return ExitUsage
	}
	code, err := dispatch(version, args, stdin, stdout, stderr, getenv)
	if err != nil {
		var exit *ExitError
		var cfg *ConfigError
		switch {
		case errors.As(err, &exit):
			code = exit.Code
		case errors.As(err, &cfg):
			code = ExitRefused
		case code == ExitOK:
			code = ExitInternal
		}
		fmt.Fprintf(stderr, "spar: %v\n", err)
		if code == ExitUsage {
			fmt.Fprint(stderr, usageText)
		}
	}
	return code
}

func dispatch(version string, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) (int, error) {
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "--version", "version":
		fmt.Fprintf(stdout, "spar %s\n", version)
		return ExitOK, nil
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usageText)
		return ExitOK, nil
	case "config":
		p, err := parseArgs(rest, []string{"repo", "reviewers", "model", "effort"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) > 0 {
			return ExitUsage, usageError("spar config takes no positional arguments")
		}
		choices, err := agentChoices(p)
		if err != nil {
			return ExitUsage, err
		}
		return ExitOK, ShowConfigWith(p.one("repo"), getenv, reviewerOverride(p), choices, stdout)
	case "run":
		p, err := parseArgs(rest, []string{"stage", "base", "head", "out", "criteria", "rules", "subject", "question",
			"decision", "decision-quote", "check-path", "reviewers", "model", "effort", "repo", "label"}, []string{"dry-run"})
		if err != nil {
			// A decision without what it settles or without the user's
			// words is out of form (G35), whichever part is missing.
			if p.missing == "decision" || p.missing == "decision-quote" {
				return ExitRefused, missingDecisionValue(p)
			}
			return ExitUsage, err
		}
		if len(p.positional) > 0 {
			return ExitUsage, usageError("spar run takes no positional arguments: %s", strings.Join(p.positional, " "))
		}
		choices, err := agentChoices(p)
		if err != nil {
			return ExitUsage, err
		}
		ds, err := decisions(p)
		if err != nil {
			return ExitRefused, err
		}
		code, err := Run(RunOptions{Repo: p.one("repo"), Stage: p.one("stage"), Base: p.one("base"), Head: p.one("head"),
			Out: p.one("out"), Criteria: p.values["criteria"], Rules: p.values["rules"], Subjects: p.values["subject"],
			Questions: p.values["question"], Decisions: ds, CheckPaths: p.values["check-path"], Reviewers: reviewerOverride(p),
			Choices: choices, Label: p.one("label"), DryRun: p.flags["dry-run"], Version: version, Getenv: getenv, Log: stderr})
		if err == nil {
			fmt.Fprintf(stdout, "spar: round written to %s\n", p.one("out"))
		}
		return code, err
	case "record":
		p, err := parseArgs(rest, []string{"result", "transcript", "model", "agent"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) != 2 {
			return ExitUsage, usageError("spar record needs ROUND and the reviewer name")
		}
		return Record(RecordOptions{Round: p.positional[0], Reviewer: p.positional[1], Result: p.one("result"),
			Transcript: p.one("transcript"), Model: p.one("model"), Agent: p.one("agent"), Stdin: stdin, Log: stderr})
	case "triage":
		p, err := parseArgs(rest, []string{"commit", "failed-before", "passed-after", "evidence", "of", "to",
			"user-quote", "note"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) != 3 {
			return ExitUsage, usageError("spar triage needs ROUND, FINDING and a disposition")
		}
		return ExitOK, Triage(TriageOptions{Round: p.positional[0], Finding: p.positional[1], Disposition: p.positional[2],
			Commit: p.one("commit"), FailedBefore: p.one("failed-before"), PassedAfter: p.one("passed-after"),
			Evidence: p.one("evidence"), Of: p.one("of"), To: p.one("to"), UserQuote: p.one("user-quote"), Note: p.one("note"),
			Log: stderr})
	case "extend":
		p, err := parseArgs(rest, []string{"stage", "by", "cause", "reason", "user-quote", "repo"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) != 1 {
			return ExitUsage, usageError("spar extend needs the series folder")
		}
		by, err := strconv.Atoi(p.one("by"))
		if err != nil {
			return ExitUsage, usageError("--by must be a number")
		}
		// An empty --cause or --reason is still given: a plan extension
		// refuses it rather than recording the line as if it were absent.
		if s := p.one("stage"); (s == "plan" || s == "plan-delta") && (p.has("cause") || p.has("reason")) {
			return ExitUsage, usageError("--cause and --reason are only for --stage code")
		}
		return ExitOK, Extend(p.positional[0], p.one("stage"), by, p.one("cause"), p.one("reason"), p.one("user-quote"),
			p.one("repo"), stderr)
	case "cap-report":
		p, err := parseArgs(rest, []string{"format", "repo"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) != 1 {
			return ExitUsage, usageError("spar cap-report needs the series folder")
		}
		format := p.one("format")
		if !p.has("format") {
			format = "md"
		}
		if format != "md" && format != "json" {
			return ExitUsage, usageError("--format must be md or json")
		}
		return CapReport(CapReportOptions{Series: p.positional[0], Format: format, Repo: p.one("repo"), Out: stdout})
	case "waive":
		p, err := parseArgs(rest, []string{"user-quote"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) != 2 {
			return ExitUsage, usageError("spar waive needs ROUND and the reviewer name")
		}
		return ExitOK, Waive(WaiveOptions{Round: p.positional[0], Reviewer: p.positional[1], UserQuote: p.one("user-quote"),
			Log: stderr})
	case "gate":
		p, err := parseArgs(rest, []string{"expect-head", "out", "repo"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		return Gate(GateOptions{Rounds: p.positional, ExpectHead: p.one("expect-head"), Out: p.one("out"),
			Repo: p.one("repo"), Version: version})
	case "summary":
		p, err := parseArgs(rest, nil, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) == 0 {
			return ExitUsage, usageError("spar summary needs round folders")
		}
		for _, dir := range p.positional {
			text, err := os.ReadFile(realish(dir) + "/summary.md")
			if err != nil {
				return ExitRefused, refused("%s has no summary: %v", dir, err)
			}
			stdout.Write(text)
		}
		return ExitOK, nil
	}
	return ExitUsage, usageError("unknown command %q", cmd)
}
