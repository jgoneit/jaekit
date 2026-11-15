package plugins

// Property: Spar skill rules
// Checks fixed tokens in the Spar skill body: the round example, project setup,
// stages and caps, standpoints, reviewer rules, decisions and dispositions.

import (
	"fmt"
	"strings"
	"testing"
)

const (
	spsSkill          = "spar/skills/spar/SKILL.md"
	spsFirstRound     = "## Before the first round"
	spsRunSection     = "## When to run each stage"
	spsReviewers      = "## Reviewers"
	spsExampleLead    = "For example, a candidate round"
	spsDecisionsTitle = "## Decisions the user made"
)

var (
	// The project setup step, in order, in the section before the first round.
	spsSetupTokens = []string{"never run Spar", "exit 65", "add `docs/reviews/` to the project's `.gitignore`",
		"ask the user", "\"Set up a project\"", "`.git/info/exclude`"}
	// Standpoint guidance in the stage section.
	spsStandpointGuidance = []string{
		"In plan and candidate rounds, including the cumulative round, pick the change's main risks.",
		"Give each reviewer a different standpoint, one each; when there are fewer risks than reviewers, leave the rest without one.",
		"In fixes and plan-delta rounds, you may give the earlier standpoints again or give none.",
		"A standpoint names the risk to attack, where to look first and the position from which to see it.",
		"Do not claim that the fix is right or explain the solution.",
		"a changed contract boundary",
		"a stored format and its earlier records",
		"the meaning of a check's result",
		"another place that keeps the same invariant",
		"A standpoint does not replace angles.",
		"When rerunning only reviewers that did not finish, give the same standpoints so the rerun is not counted.",
		"When reporting the gate, name standpoints left without a completed reviewer.",
	}
	// Standpoint argument, delivery, result, record, identity and unchanged
	// rules, each read in its own section.
	spsStandpointFormats = []struct {
		heading string
		tokens  []string
	}{
		{spsRunSection, []string{
			"`--standpoint NAME=TEXT`",
			"The first `=` separates the reviewer name from the text; the option may be repeated.",
			"Refused (65): a name outside this run's configured reviewers and `--reviewers`, more than one standpoint for a reviewer, text empty after trimming whitespace, or invalid UTF-8.",
			"Names are compared exactly and case-sensitively, as in G20.",
			"A missing `=` or an empty name is a usage error (64).",
			"Both refusals happen before any reviewer starts or any round folder is created; the message names the offending name or value.",
			"Other text is kept unchanged.",
		}},
		{spsReviewers, []string{
			"The shared packet's `## Standpoints for this round` section contains all standpoints and their reviewer assignments.",
			"Each reviewer also receives an instruction identifying its own standpoint and asking it to attack that risk while still checking every angle; an unassigned reviewer is told to review the whole change.",
			"`claude` and `codex` receive the instruction and packet on standard input.",
			"`agy` receives the command note, instruction and packet in `-p`; for a large packet, `PACKET.md` holds the packet and `-p` holds the command note, instruction and file reference.",
			"`command` receives the reviewer's `input.md` through `{prompt}`; `native` receives the instruction after its fixed instructions in `prompt.md`.",
			"For a round with standpoints, both a real run and `--dry-run` leave in `<round>/<reviewer>/input.md` exactly the input that reviewer receives.",
			"For `agy` with a large packet, `input.md` holds its `-p` text and the packet is in `PACKET.md`.",
		}},
		{spsReviewers, []string{
			"`standpoint_report`",
			"`checked` is a boolean and `detail` is nonblank text: what was checked when true, or why it could not be checked when false.",
			"For an assigned reviewer, a missing, null or malformed report is `invalid_output`, with `standpoint_report` in the reason; an unassigned reviewer need not report one.",
			"Every preset uses the same check in `spar run` and `spar record --result` or `--transcript`.",
		}},
		{spsReviewers, []string{
			"`round.json` keeps `standpoints` as reviewer/text pairs, `standpoints_uncovered` as reviewer names without a completed reviewer, and each reviewer's `standpoint` and `standpoint_report` as text/report or null.",
			"`summary.md` shows each reviewer's standpoint and report, and standpoints without a completed reviewer, including waived reviewers.",
			"The schema remains `review-round/v1`; records without standpoint fields mean no standpoints.",
		}},
		{spsRunSection, []string{
			"Standpoint text and assignments are in the packet, so `packet_sha256` identifies them as part of the same input; changing either makes a new input.",
			"A rerun of only unfinished reviewers is not counted when stage, base, head and `packet_sha256` are unchanged, including the same standpoints.",
		}},
		{spsRunSection, []string{
			"Standpoints do not change angles, classification, dispositions, caps or the gate's state and reasons.",
		}},
	}
)

// spsNorm folds every run of white space to one space.
func spsNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// spsExpect requires each phrase verbatim in text.
func spsExpect(t *testing.T, where, text string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		if !strings.Contains(text, p) {
			t.Errorf("%s does not say %q", where, p)
		}
	}
}

// spsLine is the first line of text that starts with prefix, or "".
func spsLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

// spsLevel is the heading level of a Markdown line, or 0.
func spsLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// spsFence tracks ``` and ~~~ fences: it returns the open marker after line
// and whether line is fenced or a fence line.
func spsFence(open, line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, m) {
			if open == "" {
				return m, true
			}
			if open == m {
				return "", true
			}
		}
	}
	return open, open != ""
}

// spsFencedSection returns the section that opens with the line equal to
// heading, up to the next heading of the same or a higher level outside
// fenced code.
func spsFencedSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	level := spsLevel(heading)
	start, fence := -1, ""
	for i, l := range lines {
		var fenced bool
		if fence, fenced = spsFence(fence, l); fenced {
			continue
		}
		if start < 0 {
			if strings.TrimRight(l, " \t\r") == heading {
				start = i
			}
			continue
		}
		if lv := spsLevel(l); lv > 0 && lv <= level {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// spsPrefixSection is the text from the first line that starts with start up
// to the next heading of the same or a higher level; "" when there is none.
func spsPrefixSection(text, start string) string {
	level := strings.Index(start, " ")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, start) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if n := strings.Index(lines[j], " "); n > 0 && n <= level && strings.Trim(lines[j][:n], "#") == "" {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

// spsNeedSection lists the tokens missing, with white space folded, from the
// fenced-aware section heading of text.
func spsNeedSection(text, heading string, tokens []string) []string {
	section, ok := spsFencedSection(text, heading)
	var missing []string
	if !ok {
		missing = append(missing, fmt.Sprintf("%s: missing section %q", spsSkill, heading))
	}
	for _, token := range tokens {
		if !strings.Contains(spsNorm(section), spsNorm(token)) {
			missing = append(missing, fmt.Sprintf("%s [%s]: missing %q", spsSkill, heading, token))
		}
	}
	return missing
}

func spsReport(t *testing.T, missing []string) {
	t.Helper()
	for _, m := range missing {
		t.Error(m)
	}
}

// spsExample returns the skill's round example block and the first non-empty
// line after it.
func spsExample(t *testing.T) ([]string, string, bool) {
	t.Helper()
	lines := strings.Split(read(t, spsSkill), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, spsExampleLead) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, "", false
	}
	var block []string
	in := false
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "```") {
			if in {
				for _, after := range lines[i+1:] {
					if strings.TrimSpace(after) != "" {
						return block, after, true
					}
				}
				return block, "", true
			}
			in = true
			continue
		}
		if in {
			block = append(block, line)
		} else if strings.TrimSpace(line) != "" {
			break
		}
	}
	return nil, "", false
}

// The round example gives SPEC.md and a linked goal document as --criteria,
// and the line after it says to give every goal document.
func TestSparSkillExampleGivesEveryGoalDocument(t *testing.T) {
	block, after, ok := spsExample(t)
	if !ok {
		t.Fatalf("%s has no round example block after %q", spsSkill, spsExampleLead)
	}
	cmd := strings.Join(block, " ")
	if !strings.Contains(cmd, "--criteria docs/specs/<goal>/SPEC.md") {
		t.Errorf("%s round example does not give SPEC.md as --criteria: %q", spsSkill, cmd)
	}
	if n := strings.Count(cmd, "--criteria docs/specs/<goal>/"); n < 2 {
		t.Errorf("%s round example gives %d goal document(s) as --criteria, not SPEC.md and a linked document", spsSkill, n)
	}
	for _, tok := range []string{"Give every goal document as `--criteria`", "each goal document it links"} {
		if !strings.Contains(after, tok) {
			t.Errorf("%s line after the round example lacks %q: %q", spsSkill, tok, after)
		}
	}
}

// The round example shows --check-path, and the line after it says which
// files it takes.
func TestSparSkillExampleShowsCheckPath(t *testing.T) {
	block, after, ok := spsExample(t)
	if !ok {
		t.Fatalf("%s has no round example block after %q", spsSkill, spsExampleLead)
	}
	if cmd := strings.Join(block, " "); !strings.Contains(cmd, "--check-path ") {
		t.Errorf("%s round example has no --check-path: %q", spsSkill, cmd)
	}
	for _, tok := range []string{"each check file the change adds or changes", "`--check-path`"} {
		if !strings.Contains(after, tok) {
			t.Errorf("%s line after the round example lacks %q: %q", spsSkill, tok, after)
		}
	}
}

// Before the first round, the skill limits the project configuration to caps,
// angles, rules and check paths, and names the one setup step, in order.
func TestSparSkillBeforeFirstRound(t *testing.T) {
	section := mdSection(t, spsSkill, spsFirstRound)
	requireIn(t, spsSkill+" "+spsFirstRound, section,
		"A repository's `.jaekit/spar.toml` sets only caps, angles, rules and check paths.")
	requireOrder(t, spsSkill+" "+spsFirstRound, section, spsSetupTokens...)
}

// The skill documents the cumulative round and that extending the code cap
// is the user's decision.
func TestSparSkillCumulativeRound(t *testing.T) {
	flat := spsNorm(read(t, spsSkill))
	for _, p := range []string{
		"once more as the cumulative round over the goal's base commit to the last head",
		"counts toward the code-round cap like the first candidate round",
		"extending the cap follows the rule below", "Only the user can allow more rounds.",
	} {
		if !strings.Contains(flat, spsNorm(p)) {
			t.Errorf("%s lacks %q", spsSkill, p)
		}
	}
}

// The stage section tells the agent how to choose reviewer standpoints.
func TestSparSkillStandpointGuidance(t *testing.T) {
	spsReport(t, spsNeedSection(read(t, spsSkill), spsRunSection, spsStandpointGuidance))
}

// The stage and reviewer sections state the --standpoint argument, its
// delivery, the standpoint_report form, the record fields, the input identity
// and what standpoints leave unchanged.
func TestSparSkillStandpointFormats(t *testing.T) {
	text := read(t, spsSkill)
	var missing []string
	for _, g := range spsStandpointFormats {
		missing = append(missing, spsNeedSection(text, g.heading, g.tokens)...)
	}
	spsReport(t, missing)
}

// The skill states who decides a reviewer's model and effort, the run
// arguments, the allowed lists, the fallback, what is refused and where the
// record shows the source.
func TestSparSkillReviewerModelChoice(t *testing.T) {
	spsExpect(t, spsSkill, read(t, spsSkill),
		"`--model NAME=MODEL`", "`--effort NAME=EFFORT`", "`allowed_models`", "`model_source`", "Check `spar config` before you choose",
		"falls back only to the `fallback_model` the user wrote", "spar refuses",
		"an effort for `agy`", "a name that is not a reviewer of the run", "a second model or effort for the same reviewer", "is not valid UTF-8",
		"`reviewers.<name>.model_source` and `effort_source`", "`model <model> [<source>]`", "`effort <effort> [<source>]`",
		"say which model and effort you chose for each reviewer")
}

// The Reviewers section states agy's command rule and its command note.
func TestSparSkillAgyCommandNote(t *testing.T) {
	section := spsPrefixSection(read(t, spsSkill), spsReviewers)
	if section == "" {
		t.Fatalf("%s lacks the section %q", spsSkill, spsReviewers)
	}
	spsExpect(t, spsSkill+" "+spsReviewers, section, "`&&`", "`||`", "command note")
}

// The --decision bullet says how decisions are passed and what the packet
// tells reviewers about them.
func TestSparSkillDecisionBullet(t *testing.T) {
	bullet := ""
	for _, line := range strings.Split(read(t, spsSkill), "\n") {
		if strings.HasPrefix(line, "- ") && strings.Contains(line, "`--decision`") {
			bullet = line
		}
	}
	if strings.TrimSpace(bullet) == "" {
		t.Fatalf("%s has no bullet with `--decision`", spsSkill)
	}
	spsExpect(t, spsSkill+" --decision bullet", bullet,
		"`--decision-quote`", "with or without Seal", "`"+spsDecisionsTitle+"`", "`decisions` in `round.json`",
		"spar does not check that the words are the user's", "not to report what a decision settles",
		"to report a change that goes beyond it",
		"a criterion, rule or contract the decision does not address, to report a `requirement-question` that links it")
}

// The fix disposition starts a fix within the finding's smallest_fix
// direction and records why with --note when it goes further.
func TestSparSkillFixWithinSmallestFix(t *testing.T) {
	spsExpect(t, spsSkill+" fix line", spsLine(read(t, spsSkill), "- `fix`:"),
		"Start the fix within the finding's `smallest_fix` direction",
		"record why with `--note`")
}

// Each time the code cap is reached, the skill runs the cap report, picks one
// cause, shows its recommendation, and records a code extension only with the
// user's words and the cause, by 1 unless a reason is given.
func TestSparSkillCapProcedure(t *testing.T) {
	skill := read(t, spsSkill)
	spsExpect(t, spsSkill, skill,
		"Each time the code cap is reached, run `spar cap-report <series>` before you ask.",
		"Pick one cause from its signals and show the user the signals with that cause's recommendation",
		"Only the user can allow more rounds.")
	for _, c := range []struct {
		cause   string
		phrases []string
	}{
		{"churn", []string{"extend by 1", "`smallest_fix`"}},
		{"recurring", []string{"not to extend", "ask which reading"}},
		{"unrelated", []string{"not to extend", "defer them with the user's words", "issues"}},
		{"hardening", []string{"not to extend", "trim the checks"}},
		{"other", []string{"Say why in `--reason`."}},
	} {
		prefix := "  - `" + c.cause + "`:"
		line := spsLine(skill, prefix)
		if line == "" {
			t.Errorf("%s has no line starting %q", spsSkill, prefix)
			continue
		}
		spsExpect(t, spsSkill+" line "+prefix, line, c.phrases...)
	}
	const recordPrefix = "- Record a code extension only with the user's words"
	record := spsLine(skill, recordPrefix)
	if record == "" {
		t.Fatalf("%s has no line starting %q", spsSkill, recordPrefix)
	}
	spsExpect(t, spsSkill+" extension line", record,
		"`spar extend <series> --stage code --by 1 --cause <cause> --user-quote \"<their words>\"`",
		"`--by 2` or more, and `--cause other`, need `--reason`.")
}

// The skill explains the cap report, its signals and limits, and where ha cost
// shows an extension's cause.
func TestSparSkillCapReport(t *testing.T) {
	spsExpect(t, spsSkill, read(t, spsSkill), "`spar cap-report` shows", "churn", "recurring links",
		"not introduced by the change", "check growth", "Churn is counted by file", "Recurring links are counted by link",
		"`introduced_by_change` as reported", "`ha cost` shows each extension's cause and reason", "no cause was recorded",
		"`--by 2` or more, and `--cause other`, need `--reason`.")
}
