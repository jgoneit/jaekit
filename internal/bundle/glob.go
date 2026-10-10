package bundle

import (
	"path"
	"strings"
)

// DefaultTestPaths is used when PLAN.md leaves `테스트 경로` empty.
var DefaultTestPaths = []string{"tests/**", "**/*_test.*", "**/test_*.*", "**/*.test.*", "**/*.spec.*"}

// Match reports whether a repository-relative slash path matches pattern.
// Patterns are relative to the repository root. `**` as a whole segment
// matches zero or more segments; other segments use path.Match.
func Match(pattern, name string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegs(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			for i := 0; i <= len(name); i++ {
				if matchSegs(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// MatchAny reports whether name matches any of patterns.
func MatchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if Match(p, name) {
			return true
		}
	}
	return false
}
