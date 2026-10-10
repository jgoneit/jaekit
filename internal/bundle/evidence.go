package bundle

import (
	"strings"

	"github.com/jgoneit/jaekit/internal/evidence"
	"github.com/jgoneit/jaekit/internal/markdown"
)

// A declaration is optional so an unsupported script can still produce a
// recorded unknown result. Malformed declarations are configuration errors.
func (l *loader) readEvidence(doc *markdown.Doc, file string) {
	lines, ok := doc.Section(2, "결과 계약")
	if !ok {
		return
	}
	table, ok := l.table(lines, file, "결과 계약", "ID", "선언 경로")
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, entry := range table.Rows {
		id := strings.TrimSpace(entry.Cell(0))
		row := l.b.Row(id)
		if row == nil || row.Kind != KindChange || seen[id] {
			l.problem("evidence_invalid", file, entry.Line, "result declaration needs a unique change condition ID: %q", id)
			continue
		}
		seen[id] = true
		p := strings.Trim(strings.TrimSpace(entry.Cell(1)), "`")
		row.EvidencePath = p
		d, binding, err := evidence.Load(l.root, p)
		if err != nil {
			l.problem("evidence_invalid", file, entry.Line, "%s: %v", id, err)
			continue
		}
		paths := map[string]bool{}
		for _, p := range row.CheckPaths {
			paths[p] = true
		}
		valid := true
		for _, input := range append([]string{p}, d.ProducerPaths...) {
			if !paths[input] {
				l.problem("evidence_overlay_missing", file, entry.Line, "%s evidence input %q must be included in its check paths", id, input)
				valid = false
			}
		}
		if valid {
			row.Evidence, row.EvidenceDigest = d, binding
		}
	}
}
