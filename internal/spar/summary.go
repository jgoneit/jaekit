package spar

import (
	"fmt"
	"path/filepath"
	"strings"
)

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// reviewerLine is a reviewer's line in the summary. A reviewer that did not
// complete shows its status and cause, never a finding count.
func reviewerLine(st *ReviewerState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s: %s", st.Name, st.Status)
	if st.Status == StatusCompleted && st.Findings != nil {
		fmt.Fprintf(&b, ", %s", plural(*st.Findings, "finding"))
	}
	var facts []string
	if m := deref(st.RequestedModel); m != "" {
		facts = append(facts, "model "+m)
	}
	if m := deref(st.ObservedModel); m != "" && m != deref(st.RequestedModel) {
		facts = append(facts, "observed "+m)
	}
	facts = append(facts, st.Isolation+" isolation")
	if st.Required {
		facts = append(facts, "required")
	} else {
		facts = append(facts, "not required")
	}
	if st.ElapsedSeconds > 0 {
		facts = append(facts, fmt.Sprintf("%.1fs", st.ElapsedSeconds))
	}
	fmt.Fprintf(&b, " (%s)", strings.Join(facts, ", "))
	if st.Reason != "" && st.Status != StatusCompleted {
		fmt.Fprintf(&b, ": %s", short(st.Reason, 400))
	}
	b.WriteString("\n")
	if len(st.Attempts) > 1 || st.FallbackUsed {
		for _, a := range st.Attempts {
			fmt.Fprintf(&b, "  - model %s: %s", a.Model, a.Status)
			if a.Reason != "" {
				fmt.Fprintf(&b, ": %s", short(a.Reason, 300))
			}
			if a.Resets != "" && !strings.Contains(a.Reason, a.Resets) {
				fmt.Fprintf(&b, " (%s)", a.Resets)
			}
			b.WriteString("\n")
		}
		if st.FallbackUsed {
			fmt.Fprintf(&b, "  - the designated model %s was refused, so the fallback model %s ran once on the same packet\n",
				deref(st.RequestedModel), st.FallbackModel)
		}
	}
	if st.Status == StatusCompleted {
		fmt.Fprintf(&b, "  - angles checked: %s\n", strings.Join(orNone(st.AnglesChecked), ", "))
		if len(st.AnglesUnchecked) > 0 {
			var parts []string
			for _, a := range st.AnglesUnchecked {
				parts = append(parts, fmt.Sprintf("%s (%s)", a.Angle, short(a.Reason, 120)))
			}
			fmt.Fprintf(&b, "  - angles not checked: %s\n", strings.Join(parts, ", "))
		}
	}
	for _, w := range st.Warnings {
		fmt.Fprintf(&b, "  - warning: %s\n", w)
	}
	return b.String()
}

func orNone(list []string) []string {
	if len(list) == 0 {
		return []string{"none"}
	}
	return list
}

func renderSummary(dir string, r *RoundRecord, findings []Finding, ledger []LedgerEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Spar round %s\n\n", filepath.Base(dir))
	fmt.Fprintf(&b, "- Stage: %s, base %s, head %s\n", r.Stage, shortID(r.Base), shortID(r.Head))
	fmt.Fprintf(&b, "- Packet: sha256 %s\n", r.PacketSHA256)
	if r.Config != nil {
		source := r.Config.Source.Kind
		if r.Config.Source.Path != "" {
			source += " " + r.Config.Source.Path
		}
		if r.Config.Override != nil {
			source += fmt.Sprintf(", overridden by --reviewers %s", strings.Join(r.Config.Override.Names, ","))
		}
		fmt.Fprintf(&b, "- Configuration: %s (digest %s)\n", source, shortID(r.Config.Digest))
		fmt.Fprintf(&b, "- Angles asked: %s\n", strings.Join(r.Config.Angles, ", "))
	}
	switch {
	case r.DryRun:
		b.WriteString("- Dry run: no reviewer ran; a gate does not accept this round\n")
	case len(r.RerunOf) > 0:
		fmt.Fprintf(&b, "- Rerun of %s for reviewers that did not complete the same packet; not counted against the cap\n",
			strings.Join(r.RerunOf, ", "))
	}
	b.WriteString("\n## Reviewers\n\n")
	for _, name := range r.RequestedReviewers {
		if st := r.Reviewers[name]; st != nil {
			b.WriteString(reviewerLine(st))
		}
	}
	final := finalDispositions(ledger)
	b.WriteString("\n## Findings\n\n")
	if len(findings) == 0 {
		b.WriteString("No finding was recorded from the reviewers that completed.\n")
		return b.String()
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Class]++
	}
	var parts []string
	for _, c := range Classes {
		parts = append(parts, fmt.Sprintf("%s %d", c, counts[c]))
	}
	fmt.Fprintf(&b, "%s.\n\n", strings.Join(parts, ", "))
	b.WriteString("| ID | Class | Link | Disposition | Location | Claim |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, f := range findings {
		link := "none"
		if f.LinkKind != "none" {
			link = f.LinkKind + " " + f.Link
		}
		disposition := "open"
		if e, ok := final[f.ID]; ok {
			disposition = e.Disposition
			if e.Auto {
				disposition += " (auto)"
			}
		} else if f.Class == "note" || f.Class == "hardening" {
			// Neither waits for a disposition: a hardening finding without
			// an entry has an unreadable location and was not deferred.
			disposition = "-"
		}
		location := cell(f.Location)
		if f.LocationUnreadable {
			location += " (unreadable location)"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", f.ID, f.Class, cell(link), disposition, location,
			cell(short(f.Claim, 200)))
	}
	return b.String()
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
