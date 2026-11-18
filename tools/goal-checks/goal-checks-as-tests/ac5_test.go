//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"strings"
	"testing"
)

// The texts below are copied from AGENTS.md before the change.
const (
	ac5InputFailureSentence = "Every other check only has to keep an input failure from counting as a pass."

	ac5BoundariesBullet = `- **Boundaries cannot narrow requirements.** Inputs that a SPEC criterion, a contract, a protected regression path or a selected assurance requires cannot be placed outside a check's boundary. A counterexample on such an input is fixed, or it blocks completion. Narrowing a boundary below the SPEC criterion is a scope change that the user decides.`

	ac5ProtectedPathsBullet = "- **Protected paths.** A check bound to a protected path in `contracts/regressions.json` changes only in a separate goal that changes that path."

	// ac5ReviewBlock runs from the "### Review findings" line up to, not
	// including, the "## Validation" line.
	ac5ReviewBlock = `### Review findings

- Findings from remote pull request reviews follow the same triage as local review rounds. Each one ends as fix, reject, defer or duplicate, and its record says whether it is linked to a criterion, rule or contract and whether this change introduced it.
- Defer only edge cases that are outside the recorded boundary and linked to nothing. List deferred findings in the pull request description and in the completion report.
- Classify a finding against the boundary records and protected paths as of the reviewed commit. A path widened later in the same pull request does not reclassify findings already received.
- When more than three remote review rounds have led to criterion-linked fixes, stop. State the recurring findings and the criterion they trace to, ask the user which of two readings of that criterion is intended, and wait for the answer.

### For reviewers

- State whether each finding links to a criterion, rule or contract, and whether this change introduced it.
- A counterexample on an input that a SPEC criterion, a contract or a protected regression path requires is a defect. Mark an edge case that lies outside the stated check boundary and links to nothing as low severity, separately.
- A check that counts an input failure (a missing file, corrupt input, a tool that cannot start, a signal) as a violation is a defect only when it is the producer of a condition that requires a baseline, unless the check declares that case as a violation where the check result contract allows it, such as ` + "`missing: violation`" + `. Any check that counts an input failure as a pass is a defect.
- Read the check boundaries stated in the pull request description as grounds for your judgment. A boundary added after a finding does not cover that finding.

`
)

// ac5FirstDiff describes where got first differs from want.
func ac5FirstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	window := func(s string) string {
		end := min(i+60, len(s))
		return s[i:end]
	}
	return fmt.Sprintf("first difference at byte %d (got %d bytes, want %d): got %q, want %q",
		i, len(got), len(want), window(got), window(want))
}

// TestGoalChecksAC5_PrinciplesKept keeps the input-failure pass rule, the
// Boundaries cannot narrow requirements and Protected paths bullets, and the
// review-findings text unchanged. A bullet's text ends with its line's
// newline, which is not part of the compared text.
func TestGoalChecksAC5_PrinciplesKept(t *testing.T) {
	text := readAgents(t)
	section := checksSection(t, text)
	review := reviewBlock(t, text)
	bullets := sectionBullets(section)

	if !strings.Contains(section, ac5InputFailureSentence) {
		t.Errorf("%s: the section lacks the sentence %q", agentsFile, ac5InputFailureSentence)
	}
	for _, want := range []struct{ lead, text string }{
		{"Boundaries cannot narrow requirements", ac5BoundariesBullet},
		{"Protected paths", ac5ProtectedPathsBullet},
	} {
		got, ok := findBullet(t, bullets, want.lead)
		if !ok {
			t.Errorf("%s: no bullet with the lead %q", agentsFile, want.lead)
			continue
		}
		if got = strings.TrimRight(got, "\n"); got != want.text {
			t.Errorf("%s: the %q bullet changed: %s", agentsFile, want.lead, ac5FirstDiff(got, want.text))
		}
	}
	if review != ac5ReviewBlock {
		t.Errorf("%s: the text from %q to %q changed: %s", agentsFile, reviewHeading, validationHeading, ac5FirstDiff(review, ac5ReviewBlock))
	}
}
