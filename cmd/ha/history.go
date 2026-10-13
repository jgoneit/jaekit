package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/history"
	"github.com/jgoneit/jaekit/internal/status"
)

const historyUsage = `ha history — decision history of one started goal (contracts/decision-history.md)

Usage:
  ha history <goal> add --kind <kind> --summary <text> [--status <claim>] [--reason <text>]
                        [--target <AC-n|EX-n|T001>]... [--ref <ref>]... [--checked-ref <ref>]...
                        [--builds-on <Dn>]... [--supersedes <Dn>[@<Dm>]]...
                        [--event-at <RFC 3339 time>] [--request-id <id>]
  ha history <goal> add --input <JSON file | -> [--request-id <id>]
  ha history <goal> [current|list] [--target <id>]... [--keyword <word>]...
                        [--since <cursor>] [--page <cursor>] [--limit <n>] [--format md|json]
  ha history <goal> show <Dn> | --request-id <id> [--page <cursor>] [--limit <n>] [--format md|json]

Kinds: 관측, 원인 (가설|확정|기각), 결정, 조치 (예정|반영), 검증 (대기|통과|실패), 막힘, 재개
Refs:  run:<seq>[:<sha256>]  file:<path>[#<section>]  commit:<sha>[@<repository>]  external:<reference>
       --checked-ref marks a reference whose content the executor says it read.

Exit codes: 0 ok, 64 invalid input or limit, 65 refused reference, relation, conflict, cursor or
damaged history, 66 unsupported history schema or rules, 70 unreadable storage, internal error, or a
write that may have applied.
`

var historyExit = map[string]int{
	"invalid_input": exitUsage, "limit_exceeded": exitUsage,
	"invalid_reference": exitRefused, "invalid_relation": exitRefused, "correction_conflict": exitRefused,
	"request_conflict": exitRefused, "not_started": exitRefused, "run_record_broken": exitRefused,
	"history_corrupt": exitRefused, "unknown_event": exitRefused, "cursor_invalid": exitRefused,
	"cursor_stale": exitRefused, "cursor_mismatch": exitRefused,
	"history_unsupported": exitRules,
	"history_unreadable":  exitInternal, "write_uncertain": exitInternal,
}

func (c *cli) historyFail(err error) int {
	code := history.Code(err)
	exit, ok := historyExit[code]
	if !ok {
		return c.fail(exitInternal, "%v", err)
	}
	return c.fail(exit, "%v", err)
}

func parseRef(s string, checked bool) (history.Evidence, error) {
	kind, rest, ok := strings.Cut(s, ":")
	bad := fmt.Errorf("invalid_input: reference %q must be run:<seq>[:<sha256>], file:<path>[#<section>], commit:<sha>[@<repository>] or external:<reference>", s)
	if !ok || rest == "" {
		return history.Evidence{}, bad
	}
	e := history.Evidence{Type: kind, Checked: checked}
	switch kind {
	case "run":
		seq, hash, _ := strings.Cut(rest, ":")
		n, err := strconv.Atoi(seq)
		if err != nil || strconv.Itoa(n) != seq {
			return e, bad
		}
		e.Seq, e.SHA256 = n, hash
	case "file":
		e.Path, e.Section, _ = strings.Cut(rest, "#")
	case "commit":
		e.Commit, e.Repository, _ = strings.Cut(rest, "@")
	case "external":
		e.Ref = rest
	default:
		return e, bad
	}
	return e, nil
}

func newRequestID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "auto-" + hex.EncodeToString(b), nil
}

// fileVersion hashes a repository file at registration or query. It reads
// regular files inside the repository only and never follows a symlink.
func fileVersion(root string) func(string) (string, string) {
	return func(p string) (string, string) {
		full := filepath.Join(root, filepath.FromSlash(p))
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", "unreadable"
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(full))
		if err != nil {
			return "", "missing"
		}
		if rel, err := filepath.Rel(realRoot, parent); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", "outside"
		}
		info, err := os.Lstat(full)
		if errors.Is(err, fs.ErrNotExist) {
			return "", "missing"
		}
		if err != nil {
			return "", "unreadable"
		}
		if !info.Mode().IsRegular() {
			return "", "not_regular"
		}
		if info.Size() > 64<<20 {
			return "", "too_large"
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return "", "unreadable"
		}
		h := sha256.Sum256(data)
		return "sha256:" + hex.EncodeToString(h[:]), "read"
	}
}

func (c *cli) history(in []string) int {
	if len(in) > 0 && (in[0] == "--help" || in[0] == "-h" || in[0] == "help") {
		fmt.Fprint(c.out, historyUsage)
		return exitOK
	}
	a, err := parseFlags(in,
		[]string{"kind", "status", "summary", "reason", "event-at", "request-id", "input", "since", "page", "limit", "format"},
		[]string{"target", "ref", "checked-ref", "builds-on", "supersedes", "keyword"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if len(a.pos) == 0 {
		fmt.Fprint(c.err, historyUsage)
		return exitUsage
	}
	sub := "current"
	if len(a.pos) > 1 {
		sub = a.pos[1]
	}
	allowed := map[string][]string{
		"add":     {"kind", "status", "summary", "reason", "event-at", "request-id", "input", "target", "ref", "checked-ref", "builds-on", "supersedes"},
		"current": {"target", "keyword", "since", "page", "limit", "format"},
		"list":    {"target", "keyword", "since", "page", "limit", "format"},
		"show":    {"request-id", "page", "limit", "format"},
	}
	names, ok := allowed[sub]
	if !ok {
		return c.fail(exitUsage, "unknown history subcommand %q; run ha history --help", sub)
	}
	permitted := map[string]bool{}
	for _, n := range names {
		permitted[n] = true
	}
	for name := range a.vals {
		if !permitted[name] {
			return c.fail(exitUsage, "--%s does not apply to ha history %s", name, sub)
		}
	}
	for name := range a.lists {
		if !permitted[name] {
			return c.fail(exitUsage, "--%s does not apply to ha history %s", name, sub)
		}
	}
	wantPos := 2
	if sub == "show" && a.vals["request-id"] == "" {
		wantPos = 3
	}
	if len(a.pos) > wantPos || (sub == "show" && len(a.pos) != wantPos) {
		return c.fail(exitUsage, "usage: see ha history --help")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "history_unreadable: run record: %v", err)
	}
	start := startLine(lines)
	if start == nil {
		return c.fail(exitRefused, "not_started: the goal has no start record; decision history needs an existing ha start and never starts a run")
	}
	if !status.Supported(rules(lines)) {
		return c.fail(exitRules, "%v", &status.UnsupportedRulesError{Rules: rules(lines)})
	}
	common, err := w.repo.CommonDir()
	if err != nil {
		return c.fail(exitInternal, "history_unreadable: %v", err)
	}
	store, err := history.Open(common, start.Hash())
	if err != nil {
		return c.historyFail(err)
	}
	head, err := w.repo.Head()
	if err != nil {
		return c.fail(exitRefused, "%v", err)
	}
	excl, err := w.excludes(rules(lines))
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	clean, err := w.repo.Clean(excl)
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	hashes := make([]string, len(lines))
	for i := range lines {
		hashes[i] = lines[i].Hash()
	}
	startRef := history.Ref{Seq: start.Seq, SHA256: start.Hash()}
	if sub == "add" {
		if bad != nil {
			return c.fail(exitRefused, "run_record_broken: %v; inspect the run record before recording decisions", bad)
		}
		return c.historyAdd(a, w, store, history.Facts{Start: startRef, RunHashes: hashes, Goal: w.goal.Dir,
			Commit: head, TreeClean: clean, SpecDigest: w.goal.Digest, Targets: w.targetIDs(),
			FileVersion: fileVersion(w.repo.Root), ResolveCommit: w.repo.ResolveCommit})
	}
	format := a.vals["format"]
	if format != "" && format != "md" && format != "json" {
		return c.fail(exitUsage, "--format must be md or json")
	}
	limit := 0
	if s := a.vals["limit"]; s != "" {
		if limit, err = strconv.Atoi(s); err != nil || limit < 1 {
			return c.fail(exitUsage, "invalid_input: --limit must be a positive integer")
		}
	}
	state, events, err := store.Load()
	if err != nil {
		return c.historyFail(err)
	}
	q := history.Query{Mode: sub, RequestID: a.vals["request-id"], Targets: a.lists["target"], Keywords: a.lists["keyword"],
		Since: a.vals["since"], Page: a.vals["page"], Limit: limit}
	if sub == "show" && len(a.pos) == 3 {
		q.ID = a.pos[2]
	}
	v, err := history.Run(history.Input{State: state, Events: events, Start: startRef,
		Chain:       history.Chain{Hashes: hashes, Valid: bad == nil},
		Now:         history.Now{Goal: w.goal.Dir, Commit: head, TreeClean: clean, SpecDigest: w.goal.Digest},
		FileVersion: fileVersion(w.repo.Root)}, q)
	if err != nil {
		return c.historyFail(err)
	}
	if format == "json" {
		enc := json.NewEncoder(c.out)
		enc.SetEscapeHTML(false)
		err = enc.Encode(v)
	} else {
		_, err = fmt.Fprint(c.out, v.Markdown())
	}
	if err != nil {
		return c.fail(exitInternal, "write history: %v", err)
	}
	return exitOK
}

// targetIDs are the criteria, extra checks, and tasks an event may name.
func (w *workspace) targetIDs() map[string]bool {
	out := map[string]bool{}
	for _, c := range w.goal.Criteria {
		out[c.ID] = true
	}
	for _, r := range w.bundle.Rows {
		out[r.ID] = true
	}
	for _, t := range w.bundle.Tasks {
		out[t.ID] = true
	}
	return out
}

func (c *cli) historyAdd(a *args, w *workspace, store *history.Store, facts history.Facts) int {
	request := a.vals["request-id"]
	var d history.Decision
	if path, ok := a.vals["input"]; ok {
		for name := range a.vals {
			if name != "input" && name != "request-id" {
				return c.fail(exitUsage, "--input carries the whole decision; do not combine it with --%s", name)
			}
		}
		if len(a.lists) > 0 {
			return c.fail(exitUsage, "--input carries the whole decision; do not combine it with reference or relation flags")
		}
		var r io.Reader = os.Stdin
		if path != "-" {
			f, err := os.Open(path)
			if err != nil {
				return c.fail(exitUsage, "invalid_input: %v", err)
			}
			defer f.Close()
			r = f
		}
		data, err := io.ReadAll(io.LimitReader(r, history.MaxInput+1))
		if err != nil {
			return c.fail(exitUsage, "invalid_input: read input: %v", err)
		}
		var stored string
		if d, stored, err = history.DecodeInput(data); err != nil {
			return c.historyFail(err)
		}
		if stored != "" && request != "" && stored != request {
			return c.fail(exitUsage, "invalid_input: --request-id differs from the input's request_id")
		}
		if stored != "" {
			request = stored
		}
	} else {
		d = history.Decision{Kind: a.vals["kind"], Status: a.vals["status"], Summary: a.vals["summary"],
			Reason: a.vals["reason"], Targets: a.lists["target"], BuildsOn: a.lists["builds-on"], EventAt: a.vals["event-at"]}
		for _, list := range []struct {
			name    string
			checked bool
		}{{"ref", false}, {"checked-ref", true}} {
			for _, s := range a.lists[list.name] {
				e, err := parseRef(s, list.checked)
				if err != nil {
					return c.fail(exitUsage, "%v", err)
				}
				d.Evidence = append(d.Evidence, e)
			}
		}
		for _, s := range a.lists["supersedes"] {
			id, after, _ := strings.Cut(s, "@")
			d.Supersedes = append(d.Supersedes, history.Supersede{ID: id, After: after})
		}
	}
	if request == "" {
		var err error
		if request, err = newRequestID(); err != nil {
			return c.fail(exitInternal, "request id: %v", err)
		}
	}
	e, err := store.Append(time.Now(), version, func(events []history.Event) (*history.Event, error) {
		return history.Prepare(d, request, events, facts)
	})
	replayed := false
	if err != nil {
		var replay *history.Replay
		if !errors.As(err, &replay) {
			if history.Code(err) == "write_uncertain" {
				return c.fail(exitInternal, "%v; the event may have been stored under request id %s. Before writing again, repeat this request with --request-id %s or run: ha history %s show --request-id %s",
					err, request, request, w.arg, request)
			}
			return c.historyFail(err)
		}
		e, replayed = &replay.Event, true
	}
	receipt := struct {
		Schema    string `json:"schema"`
		ID        string `json:"id"`
		RequestID string `json:"request_id"`
		Seq       int    `json:"seq"`
		SHA256    string `json:"sha256"`
		Replayed  bool   `json:"replayed"`
	}{history.ReceiptSchema, e.ID, e.RequestID, e.Seq, e.Hash(), replayed}
	enc := json.NewEncoder(c.out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(receipt); err != nil {
		return c.fail(exitInternal, "event %s stored; receipt output failed: %v (request id %s)", e.ID, err, e.RequestID)
	}
	return exitOK
}
