package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/status"
)

func (c *cli) finding(in []string) int {
	a, err := parseArgs(in, []string{"input", "format"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if len(a.pos) != 1 {
		return c.fail(exitUsage, "usage: ha finding <goal> [--input <JSON file>] [--format json|md]")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	if !status.Supported(rules(lines)) {
		return c.fail(exitRules, "%v", &status.UnsupportedRulesError{Rules: rules(lines)})
	}
	if bad != nil {
		return c.fail(exitRefused, "%v", bad)
	}
	input, hasInput := a.vals["input"]
	if !hasInput {
		format := a.vals["format"]
		if format != "" && format != "json" && format != "md" {
			return c.fail(exitUsage, "--format must be json or md")
		}
		report, err := status.FoldFindings(lines, w.goal.Dir)
		if err != nil {
			return c.fail(exitRefused, "%v", err)
		}
		if format == "md" {
			_, err = fmt.Fprint(c.out, report.Markdown())
		} else {
			err = json.NewEncoder(c.out).Encode(report)
		}
		if err != nil {
			return c.fail(exitInternal, "write findings: %v", err)
		}
		return exitOK
	}
	if input == "" || a.vals["format"] != "" {
		return c.fail(exitUsage, "--input needs a file and writes a JSON receipt; --format is for queries")
	}
	f, err := os.Open(input)
	if err != nil {
		return c.fail(exitUsage, "finding input: %v", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, record.MaxFindingInput+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return c.fail(exitInternal, "read finding input: %v %v", err, closeErr)
	}
	change, err := record.DecodeFinding(data)
	if err != nil {
		return c.fail(exitUsage, "finding input: %v", err)
	}
	entry := &record.Finding{}
	var rejected error
	line, err := record.AppendChecked(w.records, w.lock, entry, time.Now(), func(current []record.Line) error {
		if !status.Supported(rules(current)) {
			rejected = &status.UnsupportedRulesError{Rules: rules(current)}
			return rejected
		}
		event, e := status.NewFinding(current, w.goal.Dir, change)
		if e != nil {
			rejected = e
			return e
		}
		*entry = *event
		entry.HaVersion = version
		return nil
	}, nil)
	replayed := false
	if err != nil {
		var unsupported *status.UnsupportedRulesError
		if errors.As(err, &unsupported) {
			return c.fail(exitRules, "%v", err)
		}
		var replay *status.FindingReplay
		if errors.As(err, &replay) {
			line.Seq = replay.Seq
			replayed = true
			return c.findingReceipt(change.ID, change.RequestID, replay.Seq, replay.SHA256, replayed)
		}
		if rejected != nil || errors.Is(err, record.ErrBroken) || errors.Is(err, record.ErrFindingTooLarge) {
			return c.fail(exitRefused, "%v; inspect stored findings before a new change", err)
		}
		return c.fail(exitInternal, "%v", err)
	}
	return c.findingReceipt(change.ID, change.RequestID, line.Seq, line.Hash(), replayed)
}

func (c *cli) findingReceipt(id, request string, seq int, hash string, replayed bool) int {
	value := struct {
		Schema    string `json:"schema"`
		ID        string `json:"id"`
		RequestID string `json:"request_id"`
		Seq       int    `json:"seq"`
		SHA256    string `json:"sha256"`
		Replayed  bool   `json:"replayed"`
	}{"finding-receipt/v1", id, request, seq, hash, replayed}
	if err := json.NewEncoder(c.out).Encode(value); err != nil {
		return c.fail(exitInternal, "finding applied; receipt output failed: %v", err)
	}
	return exitOK
}
