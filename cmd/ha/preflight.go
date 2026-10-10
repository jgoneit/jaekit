package main

import "fmt"

// checkPreflight refuses a target before creating its log or starting a process.
// It does not append a synthetic check for work that never ran.
func (c *cli) checkPreflight(w *workspace, excludes []string, targetLabel string, completed, total int) int {
	changes, err := w.repo.WorktreeChanges(excludes)
	if err != nil {
		return c.fail(exitInternal, "cannot inspect working tree before %s: %q; %d/%d targets executed, %d not run", targetLabel, err.Error(), completed, total, total-completed)
	}
	if len(changes) == 0 {
		return exitOK
	}
	fmt.Fprintln(c.err, "ha: uncommitted changes outside the bundle documents:")
	for _, change := range changes {
		fmt.Fprintf(c.err, "  %s\n", change)
		if !change.Tracked {
			fmt.Fprintf(c.err, "    if this is local-only, review ignore rules for %q; no rules were changed\n", change.Path)
		}
	}
	return c.fail(exitRefused, "refused before %s: commit or revert relevant changes; %d/%d targets executed, %d not run", targetLabel, completed, total, total-completed)
}
