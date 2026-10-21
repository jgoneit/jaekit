package spar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Classes a finding ends up in.
var Classes = []string{"defect", "requirement-question", "hardening", "note"}

var (
	linkKinds   = []string{"criterion", "rule", "contract", "none"}
	checkImpact = []string{"false_pass", "meaning_inverted", "contract_violation", "none"}
	yesNo       = []string{"yes", "no", "unknown"}
	confidences = []string{"high", "medium", "low"}
	verdicts    = []string{"no_blocking_findings", "blocking_findings", "could_not_review"}
)

var angleText = map[string]string{
	"requirement-counterexample": "Find an input, state or sequence for which the head violates a criterion or rule while its checks still pass, or fails where the behavior is correct.",
	"side-effects":               "Look outside the diff: callers, other commands, installers, documents and records that depend on what changed and now behave differently.",
	"design":                     "Find a design flaw that makes a criterion impossible or fragile to meet: wrong ownership, duplicated rules that can drift, missing failure handling.",
	"premises":                   "Name the assumptions the change rests on and test each one against the files. A premise that is false is a finding even when the code follows it.",
	"check-validity":             "For each check the change adds or changes: does it observe the required property at the checked tree, or a proxy? Can it pass on wrong code, or report a tool failure as a violation?",
}

var stageText = map[string]string{
	"plan":       "The subject documents are a plan for meeting the criteria. Review the plan and the checks it adds, before implementation.",
	"plan-delta": "The plan was reviewed before and changed since. Review the change of the plan against the earlier findings listed below.",
	"candidate":  "The head is a candidate implementation of the criteria. Review the whole change from base to head.",
	"fixes":      "The head fixes findings of earlier rounds. Review the fixes from base to head, and whether they fix those findings without breaking anything else.",
}

// resultSchema is the JSON schema a reviewer's result follows.
func resultSchema() []byte {
	str := map[string]any{"type": "string"}
	enum := func(values []string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	finding := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"id", "proposed_class", "angle", "location", "claim", "failure_scenario", "evidence",
			"link_kind", "link", "introduced_by_change", "regression", "check_impact", "confidence"},
		"properties": map[string]any{
			"id": str, "proposed_class": enum(Classes), "angle": enum(Angles), "location": str, "claim": str,
			"failure_scenario": str, "evidence": str, "link_kind": enum(linkKinds), "link": str,
			"introduced_by_change": enum(yesNo), "regression": enum(yesNo), "check_impact": enum(checkImpact),
			"confidence": enum(confidences),
		},
	}
	angleList := func(second string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"angle", second},
			"properties": map[string]any{"angle": enum(Angles), second: str}}}
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"reviewed_head", "verdict", "angles_checked", "angles_unchecked", "findings", "limits"},
		"properties": map[string]any{
			"reviewed_head":    str,
			"verdict":          enum(verdicts),
			"angles_checked":   angleList("result"),
			"angles_unchecked": angleList("reason"),
			"findings":         map[string]any{"type": "array", "items": finding},
			"limits":           str,
		},
	}
	data, _ := json.MarshalIndent(schema, "", "  ")
	return append(data, '\n')
}

// Document is an input document as the packet and the record name it.
type Document struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
	text   string
}

func newDocument(name, source string, data []byte) Document {
	sum := sha256.Sum256(data)
	return Document{Name: name, Source: source, SHA256: hex.EncodeToString(sum[:]), text: string(data)}
}

// documentsDigest identifies a list of documents by name and content.
func documentsDigest(docs []Document) string {
	h := sha256.New()
	for _, d := range docs {
		fmt.Fprintf(h, "%s\x00%s\x00", d.Name, d.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type packetInput struct {
	Stage      string
	Base, Head string
	BaseTree   string
	HeadTree   string
	Angles     []string
	CheckPaths []string
	Criteria   []Document
	Rules      []Document
	Subjects   []Document
	Questions  []string
	Prior      []string
	Diff       []byte
}

func fence(text string) string {
	ticks := "```"
	for strings.Contains(text, ticks) {
		ticks += "`"
	}
	return ticks
}

func embedDoc(b *strings.Builder, d Document) {
	f := fence(d.text)
	fmt.Fprintf(b, "### %s\n\n%s\n%s", d.Name, f, d.text)
	if !strings.HasSuffix(d.text, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(f + "\n\n")
}

// buildPacket writes the one text every reviewer of a round receives. It
// holds no commit message, no ignored file and nothing about the reviewers.
func buildPacket(in packetInput) string {
	var b strings.Builder
	b.WriteString("# Review packet\n\n")
	b.WriteString("You are an independent reviewer. Attack the change: look for the ways it fails its criteria and rules. " +
		"You cannot change the criteria; when meeting them needs a different requirement, report a `requirement-question`.\n\n")
	b.WriteString("Read only the files in the review workspace: `head/` holds the tracked files of the head commit and `base/` those of the base commit. " +
		"There is no Git history. Do not change, create or delete any file, and do not use the network.\n\n")
	fmt.Fprintf(&b, "## Round\n\n- Stage: `%s`. %s\n- Base commit: `%s` (tree `%s`)\n- Head commit: `%s` (tree `%s`)\n\n",
		in.Stage, stageText[in.Stage], in.Base, in.BaseTree, in.Head, in.HeadTree)
	b.WriteString("## Attack angles\n\nCheck every angle below. Report each one in `angles_checked` with what you checked, or in `angles_unchecked` with the reason; a result that leaves an angle out of both is not accepted.\n\n")
	for _, a := range in.Angles {
		fmt.Fprintf(&b, "- `%s`: %s\n", a, angleText[a])
	}
	b.WriteString("\n## How to report\n\n")
	b.WriteString("- Reply with one JSON object that follows the result schema: `reviewed_head` (the head commit ID above), `verdict`, `angles_checked`, `angles_unchecked`, `findings` and `limits`.\n")
	b.WriteString("- A finding needs a concrete `failure_scenario`: a specific input or state and the wrong outcome at the head. Without one, write the concern in `limits`.\n")
	b.WriteString("- `location` is `head/<path>:<line>`, `head/<path>:<line>-<line>` or `base/<path>:<line>`, or `head/<path>` for a whole file or folder. The path is a file or folder of that copy, and the line number after it is followed by a space, a comma, a semicolon or the end. A location that names no file or folder is an unreadable location: the finding is kept, but it is never counted as in the check paths and never deferred automatically.\n")
	b.WriteString("- Give findings the IDs `F1`, `F2`, … in report order.\n")
	b.WriteString("- `link_kind` is `criterion` with an `AC-n` from the criteria below, `rule` or `contract` with a document and a line (`AGENTS.md:12`) or a range inside the document (`AGENTS.md:12-14`), or `none`. A link that does not exist is recorded as `none`.\n")
	b.WriteString("- `proposed_class`: `defect` (violates a linked criterion, rule or contract, or a regression this change made), `requirement-question` (only a change of requirement resolves it), `hardening` (an edge case nothing requires) or `note`.\n")
	b.WriteString("- `introduced_by_change` is `yes` only when `base/` does not have the same problem; `regression` is `yes` when behavior that worked at base breaks at head.\n")
	b.WriteString("- `check_impact` says what a finding in a check means: `false_pass` (the check passes on wrong code), `meaning_inverted` (it reports a pass as a failure or the reverse), `contract_violation` (it breaks a named contract or rule) or `none`.\n")
	if len(in.CheckPaths) > 0 {
		b.WriteString("- Check paths:")
		for _, p := range in.CheckPaths {
			fmt.Fprintf(&b, " `%s`", p)
		}
		b.WriteString(". At these paths a finding that claims `false_pass`, `meaning_inverted` or a linked `contract_violation` is recorded as a `defect`, whatever its `proposed_class`. " +
			"Any other finding at these paths is recorded as `hardening` and deferred, unless it proposes `requirement-question` and links the criterion, rule or contract it asks about, " +
			"and that link exists: such a question stays open for the user. A `contract` link to a file in these paths does not keep a question open.\n")
	}
	b.WriteString("\n## Criteria\n\n")
	for _, d := range in.Criteria {
		embedDoc(&b, d)
	}
	if len(in.Rules) > 0 {
		b.WriteString("## Rules\n\n")
		for _, d := range in.Rules {
			embedDoc(&b, d)
		}
	}
	if len(in.Subjects) > 0 {
		b.WriteString("## Documents under review\n\n")
		for _, d := range in.Subjects {
			embedDoc(&b, d)
		}
	}
	if len(in.Questions) > 0 {
		b.WriteString("## Questions for this round\n\n")
		for _, q := range in.Questions {
			fmt.Fprintf(&b, "- %s\n", q)
		}
		b.WriteString("\n")
	}
	if len(in.Prior) > 0 {
		b.WriteString("## Findings of earlier rounds\n\n")
		for _, p := range in.Prior {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	diffText := string(in.Diff)
	f := fence(diffText)
	fmt.Fprintf(&b, "## Changes from base to head\n\n%sdiff\n%s", f, diffText)
	if diffText != "" && !strings.HasSuffix(diffText, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(f + "\n")
	return b.String()
}
