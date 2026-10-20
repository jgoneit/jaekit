package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jgoneit/jaekit/internal/record"
)

// autoFloor is both the smallest automatic run limit and the pre-approved
// limit when the user has no configuration file (run-record.md §1.1).
const autoFloor = 50

const (
	configFolder = "jaekit"
	configFile   = "ha.json"
)

// autoRuns is max(50, ⌈minimum × 1.5⌉) in integers.
func autoRuns(minimum int) int {
	return max(autoFloor, (3*minimum+1)/2)
}

// preapproval is the user's pre-approved run limit for automatic budgets.
type preapproval struct {
	Runs   int
	Source string // "default" without a configuration file, "user_config" with one
	File   string // the configuration file's path, whether or not it exists
}

// shown names the limit's source in human output.
func (p preapproval) shown() string {
	if p.Source == "default" {
		return "default"
	}
	return p.File
}

// preapprovalError is a configuration that cannot give a limit. It never
// falls back to the default.
type preapprovalError struct{ path, reason string }

func (e *preapprovalError) Error() string {
	return fmt.Sprintf("pre-approved run limit: %s: %s", e.path, e.reason)
}

// userConfigFile is <$XDG_CONFIG_HOME or ~/.config>/jaekit/ha.json. The home
// is $HOME when set, otherwise os.UserHomeDir (USERPROFILE on Windows), as
// for Spar's configuration; unlike Spar there is one folder and no fallback.
func userConfigFile(getenv func(string) string) (string, error) {
	if base := getenv("XDG_CONFIG_HOME"); base != "" {
		path := filepath.Join(base, configFolder, configFile)
		if !filepath.IsAbs(base) {
			return path, &preapprovalError{path, "XDG_CONFIG_HOME is not an absolute path"}
		}
		return path, nil
	}
	home := getenv("HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", &preapprovalError{"~/.config/" + configFolder + "/" + configFile, "cannot determine the home folder: " + err.Error()}
		}
	}
	path := filepath.Join(home, ".config", configFolder, configFile)
	if !filepath.IsAbs(home) {
		return path, &preapprovalError{path, "the home folder is not an absolute path"}
	}
	return path, nil
}

var positiveInteger = regexp.MustCompile(`^[1-9][0-9]*$`)

// readPreapproval reads the pre-approved run limit from the user
// configuration file. Only `ha start` and `ha estimate` call it, and only for
// an automatic PLAN budget. A file inside any working tree of the goal's
// repository cannot set the limit; trees lists them and is called only when
// the file exists.
func readPreapproval(getenv func(string) string, trees func() ([]string, error)) (preapproval, error) {
	path, err := userConfigFile(getenv)
	if err != nil {
		return preapproval{}, err
	}
	data, err := readUserFile(path)
	if errors.Is(err, errUserFileAbsent) {
		return preapproval{Runs: autoFloor, Source: "default", File: path}, nil
	}
	if err != nil {
		return preapproval{}, &preapprovalError{path, err.Error()}
	}
	roots, err := trees()
	if err != nil {
		return preapproval{}, &preapprovalError{path, "cannot list the repository's working trees: " + err.Error()}
	}
	if real, err := filepath.EvalSymlinks(path); err != nil {
		return preapproval{}, &preapprovalError{path, err.Error()}
	} else if tree, ok := within(roots, real); ok {
		return preapproval{}, &preapprovalError{path, "the file is inside the repository's working tree " + tree +
			"; the limit comes only from the user configuration folder"}
	}
	var doc struct {
		PreapprovedRuns json.RawMessage `json:"preapproved_runs"`
	}
	if err := strictJSON(data, &doc); err != nil {
		return preapproval{}, &preapprovalError{path, `want {"preapproved_runs": <whole number>}: ` + err.Error()}
	}
	raw := strings.TrimSpace(string(doc.PreapprovedRuns))
	n, err := strconv.ParseInt(raw, 10, 64)
	if !positiveInteger.MatchString(raw) || err != nil || n > math.MaxInt32 {
		return preapproval{}, &preapprovalError{path, fmt.Sprintf("preapproved_runs must be a whole number from 1 to %d", math.MaxInt32)}
	}
	return preapproval{Runs: int(n), Source: "user_config", File: path}, nil
}

// within returns the tree that path is in or below. Folders are compared by
// identity (os.SameFile), not by spelling, so letter case on a file system
// that ignores it and symbolic links in a tree's path do not matter.
func within(trees []string, path string) (string, bool) {
	var roots []os.FileInfo
	var names []string
	for _, tree := range trees {
		if info, err := os.Stat(tree); err == nil {
			roots, names = append(roots, info), append(names, tree)
		}
	}
	for dir := path; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(dir); err == nil {
			for i, root := range roots {
				if os.SameFile(info, root) {
					return names[i], true
				}
			}
		}
		if filepath.Dir(dir) == dir {
			return "", false
		}
	}
}

// autoBudget is the start limit an automatic PLAN budget computes, with its
// basis, or the reason the user must approve it first.
type autoBudget struct {
	Minimum, Computed int
	Cap               preapproval
}

func (a autoBudget) approvalNeeded() bool { return a.Computed > a.Cap.Runs }

func (a autoBudget) record() *record.AutoBudget {
	return &record.AutoBudget{MinimumRuns: a.Minimum, ComputedRuns: a.Computed, PreapprovedRuns: a.Cap.Runs, Source: a.Cap.Source}
}

// computeAutoBudget is the one computation `ha start` and `ha estimate` share.
func computeAutoBudget(w *workspace) (autoBudget, error) {
	cap, err := readPreapproval(os.Getenv, w.repo.Worktrees)
	if err != nil {
		return autoBudget{}, err
	}
	minimum := minimumRequiredRuns(w)
	return autoBudget{Minimum: minimum, Computed: autoRuns(minimum), Cap: cap}, nil
}

func (a autoBudget) basis() string {
	return fmt.Sprintf("minimum %d × 1.5, at least %d", a.Minimum, autoFloor)
}

func (a autoBudget) refusal() string {
	return fmt.Sprintf("auto budget %d runs (%s) exceeds the pre-approved limit %d from %s; the user's approval is needed: "+
		"write the approved number in PLAN `검증 실행`, or have the user raise preapproved_runs in %s",
		a.Computed, a.basis(), a.Cap.Runs, a.Cap.shown(), a.Cap.File)
}
