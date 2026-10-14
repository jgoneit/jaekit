package history

import (
	"fmt"
	"strings"
)

func head12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func orNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func claim(kind, status string) string {
	if status == "" {
		return kind
	}
	return kind + " [" + status + "]"
}

// marks renders the interpretation markers an item carries.
func marks(it Item) []string {
	var m []string
	if it.Lineage != "" && it.Lineage != LineageSame {
		m = append(m, "lineage "+it.Lineage)
	}
	if len(it.SupersededBy) > 0 {
		m = append(m, "superseded by "+strings.Join(it.SupersededBy, ", "))
	}
	if len(it.OtherCorrections) > 0 {
		m = append(m, "corrected in another lineage by "+strings.Join(it.OtherCorrections, ", "))
	}
	for _, gr := range it.GroundsCorrected {
		m = append(m, "linked grounds "+gr.ID+" corrected by "+strings.Join(gr.By, ", "))
	}
	if c := it.Context; c != nil {
		if c.Code != "same" {
			m = append(m, "code "+c.Code)
		}
		if c.Spec != "same" {
			m = append(m, "goal documents "+c.Spec)
		}
		if c.Goal != "same" {
			m = append(m, "goal "+c.Goal)
		}
	}
	return m
}

func itemLine(it Item) string {
	if it.Type == "replacement" {
		return fmt.Sprintf("- replacement: %s → %s · %s → %s", it.ID, it.By, it.Summary, it.BySummary)
	}
	line := fmt.Sprintf("- %s · %s · %s · %s", it.ID, claim(it.Kind, it.Status), orNone(it.Targets), it.Summary)
	if m := marks(it); len(m) > 0 {
		line += " (" + strings.Join(m, "; ") + ")"
	}
	return line
}

// Markdown renders the same content as the JSON view.
func (v *View) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## ha history: %s — %s\n\n", v.Goal, v.Mode)
	if v.History == Absent {
		b.WriteString("- history: absent — no local decision history is stored for this start record in this repository; absence does not show that no event was ever recorded\n")
	} else {
		fmt.Fprintf(&b, "- history: %s · events in range: %d\n", v.History, v.Events)
	}
	head := "none"
	if v.RunHead != nil {
		head = fmt.Sprintf("seq %d (%s)", v.RunHead.Seq, head12(v.RunHead.SHA256))
	}
	fmt.Fprintf(&b, "- run head: %s · run chain: %s · start: seq %d (%s)\n", head, v.RunChain, v.Start.Seq, head12(v.Start.SHA256))
	clean := "clean"
	if !v.Now.TreeClean {
		clean = "uncommitted changes"
	}
	fmt.Fprintf(&b, "- now: code %s, %s · goal digest %s\n", head12(v.Now.Commit), clean, head12(v.Now.SpecDigest))
	since := "none"
	if v.Filters.Since != nil {
		since = *v.Filters.Since
	}
	fmt.Fprintf(&b, "- filters: targets %s · keywords %s · since %s\n", orNone(v.Filters.Targets), orNone(v.Filters.Keywords), since)
	if v.Event != nil {
		e := v.Event
		b.WriteString("\n### " + e.ID + " · " + claim(e.Kind, e.Status) + "\n\n")
		fmt.Fprintf(&b, "- summary: %s\n", e.Summary)
		fmt.Fprintf(&b, "- targets: %s\n", orNone(e.Targets))
		eventAt := "unknown"
		if e.EventAt != "" {
			eventAt = e.EventAt + " (claimed)"
		}
		fmt.Fprintf(&b, "- recorded at: %s · event time: %s · request: %s\n", e.RecordedAt, eventAt, e.RequestID)
		current := "not current"
		if e.Current {
			current = "current in the selected lineage"
		}
		fmt.Fprintf(&b, "- lineage: %s · %s\n", e.Lineage, current)
		if m := marks(e.Item); len(m) > 0 {
			fmt.Fprintf(&b, "- markers: %s\n", strings.Join(m, "; "))
		}
		r := e.Recorded
		fmt.Fprintf(&b, "- recorded context: run head seq %d (%s) · code %s, tree clean %v · goal %s · goal digest %s\n",
			r.RunHead.Seq, head12(r.RunHead.SHA256), head12(r.Commit), r.TreeClean, r.Goal, head12(r.SpecDigest))
		if e.Reason != "" {
			b.WriteString("- reason:\n")
			for _, line := range strings.Split(e.Reason, "\n") {
				b.WriteString("  > " + line + "\n")
			}
		}
		if len(e.Evidence) == 0 {
			b.WriteString("- evidence: none\n")
		}
		for _, ev := range e.Evidence {
			fmt.Fprintf(&b, "- evidence %s\n", evidenceLine(ev))
		}
		b.WriteString("\n#### Direct predecessors\n\n")
		if len(v.Predecessors) == 0 {
			b.WriteString("- none\n")
		}
		for _, p := range v.Predecessors {
			b.WriteString(relatedLine(p))
		}
		fmt.Fprintf(&b, "\n#### Direct successors (%d of %d)\n\n", v.Shown, v.Total)
		if len(v.Successors) == 0 {
			b.WriteString("- none recorded\n")
		}
		for _, s := range v.Successors {
			b.WriteString(relatedLine(s))
		}
	} else {
		fmt.Fprintf(&b, "\n### Items (%d of %d)\n\n", v.Shown, v.Total)
		if len(v.Items) == 0 {
			b.WriteString("- none\n")
		}
		for _, it := range v.Items {
			b.WriteString(itemLine(it) + "\n")
		}
		if v.OtherLineage != nil && v.OtherLineage.Count > 0 {
			var ids []string
			for _, o := range v.OtherLineage.IDs {
				ids = append(ids, o.ID+" ("+o.Lineage+")")
			}
			fmt.Fprintf(&b, "\n- outside the selected lineage, not merged into current judgments: %d — %s\n", v.OtherLineage.Count, strings.Join(ids, ", "))
		}
	}
	b.WriteString("\n")
	if v.Next != nil {
		fmt.Fprintf(&b, "- truncated: more results remain; continue with --page %s\n", *v.Next)
	}
	fmt.Fprintf(&b, "- later changes: --since %s\n", v.Cursor)
	return b.String()
}

func relatedLine(r Related) string {
	current := ""
	if r.Current {
		current = ", current"
	}
	if r.After != "" {
		current += ", after " + r.After
	}
	return fmt.Sprintf("- %s %s · %s · %s (lineage %s%s)\n", r.Relation, r.ID, claim(r.Kind, r.Status), r.Summary, r.Lineage, current)
}

func evidenceLine(ev EvidenceView) string {
	checked := "content not checked"
	if ev.Checked {
		checked = "content checked (claim)"
	}
	version := "version unidentified"
	if v := ev.Recorded.Version; v != "" {
		// Hashes Core computed are shortened; an executor's external version is
		// arbitrary text whose prefix may not tell versions apart.
		if ev.Type != "external" {
			v = head12(strings.TrimPrefix(v, "sha256:"))
		}
		version = "version " + v
	}
	var what string
	switch ev.Type {
	case "run":
		what = fmt.Sprintf("run seq %d", ev.Seq)
	case "file":
		what = "file " + ev.Path
		if ev.Section != "" {
			what += " § " + ev.Section
		}
	case "commit":
		what = "commit " + ev.Commit
		if ev.Repository != "" {
			what += " in " + ev.Repository
		}
	case "external":
		what = "external " + ev.Ref
	}
	return fmt.Sprintf("%s · %s · %s · recorded %s · now %s", what, checked, version, ev.Recorded.State, ev.Now)
}
