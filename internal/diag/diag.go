// Package diag holds the lint problem type shared by the goal document and
// execution bundle parsers.
package diag

import (
	"fmt"
	"sort"
)

// Problem is one lint finding. Code is a stable identifier from the bundle
// contract; File is repository-relative; Line is 1-based or 0 when unknown.
type Problem struct {
	Code   string `json:"code"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Detail string `json:"detail"`
}

func (p Problem) String() string {
	loc := p.File
	if p.Line > 0 {
		loc = fmt.Sprintf("%s:%d", p.File, p.Line)
	}
	return fmt.Sprintf("%s\t%s\t%s", p.Code, loc, p.Detail)
}

// Sort orders problems by file, line, code, and detail so output is stable.
func Sort(ps []Problem) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Detail < b.Detail
	})
}
