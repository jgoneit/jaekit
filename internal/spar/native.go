package spar

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RecordOptions are the inputs of `spar record`.
type RecordOptions struct {
	Round      string
	Reviewer   string
	Result     string // a reply file, or "-" for standard input
	Transcript string // a host subagent transcript (JSON lines)
	Model      string
	Agent      string
	Stdin      io.Reader
}

var nativeTools = map[string]bool{"Read": true, "Grep": true, "Glob": true}

// Context a host adds to every subagent; anything else is a warning.
var expectedContext = map[string]bool{"environment": true, "session_context.userEmail": true}

type transcriptInfo struct {
	reply      string
	model      string
	agentType  string
	used       map[string]int
	offered    []string
	contexts   []string
	readPrompt bool
}

// readTranscript reads a host subagent transcript: its last reply, the
// model, the tools it used and offered, and what the host added to its
// context (names only, never the text).
func readTranscript(path, nativeDir string) (transcriptInfo, error) {
	info := transcriptInfo{used: map[string]int{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return info, err
	}
	prompt := filepath.Join(nativeDir, "prompt.md")
	for _, e := range jsonLines(data) {
		switch e["type"] {
		case "attachment":
			att := object(e["attachment"])
			kind := str(att["type"])
			if kind == "session_context" {
				for _, key := range sortedKeys(object(att["context"])) {
					info.contexts = append(info.contexts, "session_context."+key)
				}
			} else if kind != "" {
				info.contexts = append(info.contexts, kind)
			}
		case "assistant":
			msg := object(e["message"])
			if m := str(msg["model"]); m != "" {
				info.model = m
			}
			var text []string
			for _, block := range anyList(msg["content"]) {
				b := object(block)
				switch b["type"] {
				case "tool_use":
					name := str(b["name"])
					info.used[name]++
					target := str(object(b["input"])["file_path"])
					if target != "" && realish(target) == realish(prompt) {
						info.readPrompt = true
					}
				case "text":
					text = append(text, str(b["text"]))
				}
			}
			if len(text) > 0 {
				info.reply = strings.Join(text, "\n")
			}
		}
	}
	if m := regexp.MustCompile(`^agent-(.+)\.jsonl$`).FindStringSubmatch(filepath.Base(path)); m != nil {
		dir := filepath.Dir(path)
		if meta, err := os.ReadFile(filepath.Join(dir, "agent-"+m[1]+".meta.json")); err == nil {
			var v map[string]any
			if json.Unmarshal(meta, &v) == nil {
				info.agentType = str(v["agentType"])
			}
		}
		if prefix, err := os.ReadFile(filepath.Join(dir, "agent-"+m[1]+".prefix.json")); err == nil {
			var v map[string]any
			if json.Unmarshal(prefix, &v) == nil {
				for _, t := range anyList(object(v["state"])["toolNames"]) {
					info.offered = append(info.offered, str(t))
				}
			}
		}
	}
	return info, nil
}

func transcriptWarnings(info transcriptInfo, name string) []string {
	var warnings []string
	var extra []string
	for _, name := range sortedKeys(info.used) {
		if !nativeTools[name] {
			extra = append(extra, fmt.Sprintf("%s (%d)", name, info.used[name]))
		}
	}
	if len(extra) > 0 {
		warnings = append(warnings, "the subagent used tools beyond Read, Grep and Glob: "+strings.Join(extra, ", "))
	}
	var offered []string
	for _, t := range info.offered {
		if !nativeTools[t] {
			offered = append(offered, t)
		}
	}
	if len(offered) > 0 {
		sort.Strings(offered)
		warnings = append(warnings, "the host offered tools beyond Read, Grep and Glob: "+strings.Join(uniq(offered), ", "))
	}
	for _, c := range uniq(info.contexts) {
		if expectedContext[c] {
			continue
		}
		switch c {
		case "session_context.gitStatus":
			warnings = append(warnings, "the host added the repository's git status to the subagent's context")
		case "instructions", "nested_memory":
			warnings = append(warnings, "the host added instruction or memory files to the subagent's context")
		case "hook_additional_context":
			warnings = append(warnings, "a host hook added context to the subagent")
		default:
			warnings = append(warnings, "the host added context to the subagent: "+c)
		}
	}
	if !info.readPrompt {
		warnings = append(warnings, "the transcript does not show the subagent reading "+name+"/prompt.md")
	}
	return warnings
}

// Record stores the host subagent's reply for a round's native reviewer.
func Record(o RecordOptions) (int, error) {
	if (o.Result == "") == (o.Transcript == "") {
		return ExitUsage, usageError("give exactly one of --result and --transcript")
	}
	dir := realish(o.Round)
	r, err := loadRound(dir)
	if err != nil {
		return ExitRefused, refused("%v", err)
	}
	st := r.Reviewers[o.Reviewer]
	switch {
	case st == nil:
		return ExitRefused, refused("%s has no reviewer named %q", o.Round, o.Reviewer)
	case st.Preset != nativePreset:
		return ExitRefused, refused("%s is a %s reviewer; only native reviewers are recorded from the host", o.Reviewer, st.Preset)
	case r.DryRun:
		return ExitRefused, refused("%s is a dry run; no reviewer is recorded in it", o.Round)
	case st.Status == StatusCompleted:
		return ExitRefused, refused("%s of %s is already recorded", o.Reviewer, o.Round)
	}
	nativeDir := filepath.Join(dir, o.Reviewer)
	var reply string
	var warnings []string
	model := o.Model
	if o.Transcript != "" {
		info, err := readTranscript(o.Transcript, nativeDir)
		if err != nil {
			return ExitRefused, refused("transcript %s: %v", o.Transcript, err)
		}
		reply, warnings = info.reply, transcriptWarnings(info, o.Reviewer)
		if model == "" {
			model = info.model
		}
		if o.Agent == "" && info.agentType != "" {
			o.Agent = "host subagent " + info.agentType
		}
		st.RecordedFrom = "transcript"
	} else {
		var data []byte
		if o.Result == "-" {
			data, err = io.ReadAll(o.Stdin)
		} else {
			data, err = os.ReadFile(o.Result)
		}
		if err != nil {
			return ExitRefused, refused("--result: %v", err)
		}
		reply = string(data)
		st.RecordedFrom = "result"
	}
	st.Agent = o.Agent
	st.ObservedModel = strPtr(model)
	st.Warnings = append([]string{}, warnings...)
	st.Reason = ""
	attempt := Attempt{Model: model, StartedAt: r.StartedAt, FinishedAt: now()}
	ws := filepath.Join(nativeDir, "workspace")
	entries, err := manifest(ws)
	var added []Finding
	result, ok := lastJSONObject([]byte(reply))
	switch {
	case err != nil || st.WorkspaceDigest == "" || manifestDigest(entries) != st.WorkspaceDigest:
		st.Status, st.Reason = StatusWorkspaceModified, "the native workspace changed after it was prepared"
	case !ok:
		st.Status, st.Reason = StatusInvalidOutput, "the reply holds no JSON object"
	default:
		if problems := validateResult(result, r.Head, r.Config.Angles); len(problems) > 0 {
			st.Status, st.Reason = StatusInvalidOutput, strings.Join(first(problems, 5), "; ")
		} else if str(result["verdict"]) == "could_not_review" {
			st.Status, st.Reason = StatusFailed, "the reviewer reported it could not review: "+short(str(result["limits"]), 300)
		} else {
			st.Status = StatusCompleted
			c := classifier{repo: r.Repo, head: r.Head, checkPaths: r.Config.CheckPaths, targets: r.Links}
			added = applyResult(c, st, o.Reviewer, result)
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		os.WriteFile(filepath.Join(nativeDir, "result.json"), append(data, '\n'), 0o644)
	}
	attempt.Status, attempt.Reason = st.Status, st.Reason
	st.Attempts = append(st.Attempts, attempt)
	if err := saveRound(dir, r, added, false); err != nil {
		return ExitInternal, err
	}
	if st.Status != StatusCompleted {
		return ExitNoResult, nil
	}
	os.RemoveAll(ws)
	return ExitOK, nil
}
