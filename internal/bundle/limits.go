package bundle

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/markdown"
)

// Command time limits (contracts/bundle.md §2.5). A run-rules/4 plan may
// declare a limit per condition or task command in the optional
// `## 시간 제한` section; every other command runs under DefaultLimit.
const (
	DefaultLimit = 900 * time.Second
	MaxLimit     = 2 * time.Hour
	// LimitsSection is the optional PLAN section title, a contract token.
	LimitsSection = "시간 제한"
)

var (
	limitTask     = regexp.MustCompile(`^(T\d{3,}) 명령 ([1-9]\d*)$`)
	limitDuration = regexp.MustCompile(`^(?:([1-9]\d*|0)\s*시간)?\s*(?:([1-9]\d*|0)\s*분)?\s*(?:([1-9]\d*|0)\s*초)?$`)
)

// LimitLabel names a command the way `ha check` prints it: a condition ID, or
// a task and its command number such as T002#1.
func LimitLabel(criterion, task string, index int) string {
	if criterion != "" {
		return criterion
	}
	return fmt.Sprintf("%s#%d", task, index)
}

// Limit is the time limit for a command under run-rules/4: its declaration,
// or DefaultLimit.
func (b *Bundle) Limit(label string) time.Duration {
	if d, ok := b.Limits[label]; ok {
		return d
	}
	return DefaultLimit
}

// parseLimit reads `N시간`, `N분`, `N초` or a combination such as 1시간 30분.
func parseLimit(s string) (time.Duration, bool) {
	m := limitDuration.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || (m[1] == "" && m[2] == "" && m[3] == "") {
		return 0, false
	}
	var d time.Duration
	for i, unit := range []time.Duration{time.Hour, time.Minute, time.Second} {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil || n > 1_000_000_000 {
			return 0, false
		}
		d += time.Duration(n) * unit
	}
	return d, d > 0
}

// readLimits reads `## 시간 제한` after the condition table and the tasks.
// The section is optional; a declaration it cannot apply is a lint problem,
// so a run never starts under a limit the plan did not mean.
func (l *loader) readLimits(doc *markdown.Doc, file string) {
	lines, ok := doc.Section(2, LimitsSection)
	if !ok {
		return
	}
	l.b.Limits = map[string]time.Duration{}
	for _, it := range markdown.Items(lines) {
		key, value, found := strings.Cut(it.Text, ":")
		if !found {
			l.problem("time_limit_invalid", file, it.Line, "`## %s` items look like `- AC-3: 30분` or `- T002 명령 1: 20분`: %q", LimitsSection, it.Text)
			continue
		}
		key = strings.TrimSpace(key)
		d, valid := parseLimit(value)
		if !valid {
			l.problem("time_limit_invalid", file, it.Line, "%s: the limit must be a positive number of 시간, 분 or 초: %q", key, strings.TrimSpace(value))
			continue
		}
		if d > MaxLimit {
			l.problem("time_limit_invalid", file, it.Line, "%s: %s exceeds the maximum of %d seconds", key, strings.TrimSpace(value), int64(MaxLimit/time.Second))
			continue
		}
		label := key
		if m := limitTask.FindStringSubmatch(key); m != nil {
			n, _ := strconv.Atoi(m[2])
			t := l.b.Task(m[1])
			if t == nil || n > len(t.Commands) {
				l.problem("time_limit_invalid", file, it.Line, "%s: no such task command", key)
				continue
			}
			label = LimitLabel("", m[1], n)
		} else if r := l.b.Row(key); r == nil || r.Kind == KindManual {
			l.problem("time_limit_invalid", file, it.Line, "%s: no condition with a command", key)
			continue
		}
		if _, dup := l.b.Limits[label]; dup {
			l.problem("time_limit_invalid", file, it.Line, "%s: declared twice", key)
			continue
		}
		l.b.Limits[label] = d
	}
}

// LimitDigest binds a run-rules/4 command digest to its applied limit, so a
// changed limit makes earlier results stale.
func LimitDigest(digest string, limit time.Duration) string {
	return Digest(digest + "\x00limit_ms=" + strconv.FormatInt(limit.Milliseconds(), 10))
}
