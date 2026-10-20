package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/run"
)

// ha doctor runs the check commands of the plan's `## 실행 환경` section before
// work starts (run-record.md §1.4). It writes nothing: no record line, run
// use, lock, bundle file or ha/runs output. A check's output goes to a new
// file in the system temporary directory, removed once its line is printed.

// doctorTimeout is the fixed limit of every check; the plan's
// `## 시간 제한` does not apply.
const doctorTimeout = 60 * time.Second

// The end of an unavailable tool's output that reaches the terminal.
const (
	doctorTailLines = 20
	doctorTailBytes = 2000
	doctorTailRead  = 8192
)

// Result words of the contract; they are tokens, never translated.
const (
	doctorAvailable   = "사용 가능"
	doctorUnavailable = "사용 불가"
)

func (c *cli) doctor(in []string) int {
	a, err := parseArgs(in, nil, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if len(a.pos) != 1 {
		return c.fail(exitUsage, "usage: ha doctor <goal>")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	plan := w.goal.Dir + "/PLAN.md"
	src, err := os.ReadFile(filepath.Join(w.repo.Root, filepath.FromSlash(plan)))
	if errors.Is(err, fs.ErrNotExist) {
		return c.fail(exitRefused, "%s does not exist", plan)
	}
	if err != nil {
		return c.fail(exitInternal, "read %s: %v", plan, err)
	}
	probes, found, problems := bundle.Environment(string(src), plan)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(c.out, p.String())
		}
		return c.fail(exitRefused, "fix the `## %s` problems above; no check was run", bundle.EnvironmentSection)
	}
	if !found {
		fmt.Fprintf(c.out, "%s has no `## %s` section; nothing to check\n", plan, bundle.EnvironmentSection)
		return exitOK
	}
	if len(probes) == 0 {
		fmt.Fprintf(c.out, "`## %s` in %s lists no tools; nothing to check\n", bundle.EnvironmentSection, plan)
		return exitOK
	}
	result := exitOK
	for _, p := range probes {
		reason, tail, err := c.probe(w.repo.Root, p)
		if err != nil {
			return c.fail(exitInternal, "%s: the check could not be observed: %v", p.Name, err)
		}
		if reason == "" {
			fmt.Fprintf(c.out, "%s: %s\n", p.Name, doctorAvailable)
			continue
		}
		result = exitProblems
		fmt.Fprintf(c.out, "%s: %s (%s)\n", p.Name, doctorUnavailable, reason)
		for _, line := range tail {
			fmt.Fprintf(c.out, "    %s\n", line)
		}
	}
	return result
}

// probe runs one check the way ha check runs a command: argv without a shell
// from the repository root, standard input closed, the same process control.
// reason is empty when the tool is available. An error means ha could not
// control or observe the process; it is not the tool's result.
func (c *cli) probe(root string, p bundle.Probe) (reason string, tail []string, err error) {
	f, err := os.CreateTemp("", "ha-doctor-*.log")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	res, err := c.executeCheckOutput(root, p.Argv, doctorTimeout, name, nil, nil)
	if err != nil {
		return "", nil, err
	}
	if res.Reason == "process_wait" {
		return "", nil, errors.New("process control or wait failed")
	}
	if reason = doctorReason(res); reason == "" {
		return "", nil, nil
	}
	tail, err = outputTail(name)
	return reason, tail, err
}

// doctorReason is a total function of a run's result: empty for exit 0, else
// one of the five reasons of the contract.
func doctorReason(res run.Result) string {
	switch {
	case res.Outcome == run.Pass:
		return ""
	case res.Reason == "process_start" && errors.Is(res.StartError, fs.ErrPermission):
		return "실행 권한 없음"
	case res.Reason == "process_start":
		return "실행 파일 없음"
	case res.Reason == "process_timeout":
		return fmt.Sprintf("시간 초과, %d초", int(doctorTimeout/time.Second))
	case res.Signal != "":
		return "신호로 종료, 신호 " + res.Signal
	case res.Exit != nil:
		return fmt.Sprintf("0이 아닌 종료, 종료 코드 %d", *res.Exit)
	}
	return "실행 파일 없음"
}

// outputTail is the end of a check's output: its last doctorTailLines lines,
// cut to the last doctorTailBytes bytes from a character boundary when
// longer, without line-ending carriage returns.
func outputTail(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := info.Size() - doctorTailRead
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > doctorTailLines {
		lines = lines[len(lines)-doctorTailLines:]
	}
	text = strings.Join(lines, "\n")
	if len(text) > doctorTailBytes {
		text = text[len(text)-doctorTailBytes:]
		for len(text) > 0 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
	}
	lines = strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines, nil
}
