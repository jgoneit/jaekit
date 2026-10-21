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
  spar config [--repo DIR] [--reviewers NAME,...]
  spar run --stage plan|plan-delta|candidate|fixes --base REV [--head REV] --out DIR
           --criteria FILE... [--rules FILE]... [--subject FILE]... [--question TEXT]...
           [--check-path GLOB]... [--reviewers NAME,...] [--repo DIR] [--label TEXT] [--dry-run]
  spar record ROUND native (--result FILE|- | --transcript FILE) [--model M] [--agent TEXT]
  spar triage ROUND FINDING fix|reject|defer|duplicate|answer [--commit REV] [--failed-before TEXT]
           [--passed-after TEXT] [--evidence TEXT] [--of FINDING] [--to TEXT] [--user-quote TEXT] [--note TEXT]
  spar extend SERIES --stage plan|plan-delta|code --by N --user-quote TEXT
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
}

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
					return p, usageError("--%s needs a value", name)
				}
				i++
				value = args[i]
			}
			p.values[name] = append(p.values[name], value)
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
		p, err := parseArgs(rest, []string{"repo", "reviewers"}, nil)
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) > 0 {
			return ExitUsage, usageError("spar config takes no positional arguments")
		}
		return ExitOK, ShowConfig(p.one("repo"), getenv, reviewerOverride(p), stdout)
	case "run":
		p, err := parseArgs(rest, []string{"stage", "base", "head", "out", "criteria", "rules", "subject", "question",
			"check-path", "reviewers", "repo", "label"}, []string{"dry-run"})
		if err != nil {
			return ExitUsage, err
		}
		if len(p.positional) > 0 {
			return ExitUsage, usageError("spar run takes no positional arguments: %s", strings.Join(p.positional, " "))
		}
		code, err := Run(RunOptions{Repo: p.one("repo"), Stage: p.one("stage"), Base: p.one("base"), Head: p.one("head"),
			Out: p.one("out"), Criteria: p.values["criteria"], Rules: p.values["rules"], Subjects: p.values["subject"],
			Questions: p.values["question"], CheckPaths: p.values["check-path"], Reviewers: reviewerOverride(p),
			Label: p.one("label"), DryRun: p.flags["dry-run"], Version: version, Getenv: getenv, Log: stderr})
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
		p, err := parseArgs(rest, []string{"stage", "by", "user-quote", "repo"}, nil)
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
		return ExitOK, Extend(p.positional[0], p.one("stage"), by, p.one("user-quote"), p.one("repo"), stderr)
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
