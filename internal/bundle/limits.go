package bundle

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	var seconds int64
	for i, unit := range []int64{3600, 60, 1} {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil || n > int64(MaxLimit/time.Second)/unit {
			// Too large to be a limit: report it as over the maximum, never wrapped.
			return MaxLimit + time.Second, true
		}
		seconds += n * unit
	}
	return time.Duration(seconds) * time.Second, seconds > 0
}

// LimitDigest binds a run-rules/4 command digest to its applied limit, so a
// changed limit makes earlier results stale.
func LimitDigest(digest string, limit time.Duration) string {
	return Digest(digest + "\x00limit_ms=" + strconv.FormatInt(limit.Milliseconds(), 10))
}
