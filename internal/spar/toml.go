package spar

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// keyLines maps each key path in a TOML document to the line it is written
// on: "timeout_seconds", "caps.plan", "reviewers[0]" (the header or array
// element) and "reviewers[0].preset". It is only called on documents that
// already parsed, so it ignores parser errors.
func keyLines(data []byte) map[string]int {
	lines := map[string]int{}
	set := func(path string, line int) {
		if _, ok := lines[path]; !ok && line > 0 {
			lines[path] = line
		}
	}
	var p unstable.Parser
	p.Reset(data)
	keyOf := func(n *unstable.Node) (string, int) {
		var parts []string
		line := 0
		it := n.Key()
		for it.Next() {
			k := it.Node()
			if line == 0 {
				line = p.Shape(k.Raw).Start.Line
			}
			parts = append(parts, string(k.Data))
		}
		return strings.Join(parts, "."), line
	}
	var walk func(prefix string, value *unstable.Node, line int)
	walk = func(prefix string, value *unstable.Node, line int) {
		switch value.Kind {
		case unstable.InlineTable:
			children := value.Children()
			for children.Next() {
				kv := children.Node()
				key, l := keyOf(kv)
				set(prefix+"."+key, l)
				walk(prefix+"."+key, kv.Value(), l)
			}
		case unstable.Array:
			children := value.Children()
			for i := 0; children.Next(); i++ {
				item := children.Node()
				l := line
				if item.Kind == unstable.InlineTable {
					l = p.Shape(item.Raw).Start.Line
				}
				path := fmt.Sprintf("%s[%d]", prefix, i)
				set(path, l)
				walk(path, item, l)
			}
		}
	}
	prefix := ""
	counts := map[string]int{}
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table:
			key, l := keyOf(e)
			prefix = key
			set(prefix, l)
		case unstable.ArrayTable:
			key, l := keyOf(e)
			prefix = fmt.Sprintf("%s[%d]", key, counts[key])
			counts[key]++
			set(key, l)
			set(prefix, l)
		case unstable.KeyValue:
			key, l := keyOf(e)
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			set(path, l)
			walk(path, e.Value(), l)
		}
	}
	return lines
}

// tableHeaderLine finds the header that redefines a table: its second
// header, or its only one when dotted keys defined the table first.
func tableHeaderLine(data []byte, name string) int {
	var p unstable.Parser
	p.Reset(data)
	seen, found := 0, 0
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind != unstable.Table && e.Kind != unstable.ArrayTable {
			continue
		}
		var parts []string
		line := 0
		it := e.Key()
		for it.Next() {
			k := it.Node()
			if line == 0 {
				line = p.Shape(k.Raw).Start.Line
			}
			parts = append(parts, string(k.Data))
		}
		if strings.Join(parts, ".") == name {
			seen++
			found = line
			if seen == 2 {
				return line
			}
		}
	}
	return found
}
