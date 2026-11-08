package spar

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Status values of a reviewer in a round.
const (
	StatusCompleted         = "completed"
	StatusPending           = "pending"
	StatusUnavailable       = "unavailable"
	StatusTimedOut          = "timed_out"
	StatusInvalidOutput     = "invalid_output"
	StatusFailed            = "failed"
	StatusToolViolation     = "tool_violation"
	StatusWorkspaceModified = "workspace_modified"
	StatusNotRun            = "not_run"
)

var codexDisabled = []string{"apps", "browser_use", "browser_use_external", "computer_use", "in_app_browser",
	"image_generation", "memories", "multi_agent", "plugins", "hooks"}

// Event item types a read-only codex review may produce.
var codexAllowed = map[string]bool{"agent_message": true, "reasoning": true, "command_execution": true,
	"todo_list": true, "error": true}

var claudeTools = []string{"Read", "Grep", "Glob"}
var claudeAllowed = map[string]bool{"Read": true, "Grep": true, "Glob": true, "StructuredOutput": true}

// agy 1.4.3 reads files with read_file, as clean-review 0.2.1 allows.
var agyAllowed = map[string]bool{"view_file": true, "read_file": true, "list_dir": true, "grep_search": true, "find_by_name": true,
	"finish": true, "wait": true, "wait_5_seconds": true, "command_status": true}

// Programs agy's shell may run: they read files and start nothing else;
// true does nothing.
var readOnlyPrograms = map[string]bool{"find": true, "ls": true, "grep": true, "egrep": true, "fgrep": true,
	"rg": true, "cat": true, "head": true, "tail": true, "wc": true, "nl": true, "sort": true, "uniq": true,
	"cut": true, "stat": true, "file": true, "diff": true, "cmp": true, "tree": true, "pwd": true,
	"basename": true, "dirname": true, "realpath": true, "tr": true, "du": true, "true": true}

// commandSeparators are the operators that may stand between two commands
// each of which is read-only on its own.
var commandSeparators = map[string]bool{"|": true, "&&": true, "||": true, ";": true}

// agyCommandNote opens agy's prompt, before the packet: which shell commands
// spar accepts. The packet itself stays the same for every reviewer.
var agyCommandNote = func() string {
	names := make([]string, 0, len(readOnlyPrograms))
	for name := range readOnlyPrograms {
		names = append(names, name)
	}
	sort.Strings(names)
	return "Spar checks every shell command you run during this review. Read, list and search files with your file tools " +
		"(view_file, read_file, list_dir, grep_search, find_by_name) first. A shell command may run only these read-only programs: " +
		strings.Join(names, ", ") + ", and sed -n with a print script such as sed -n '1,40p'; join commands only with |, &&, || or ;. " +
		"Running code, tests, builds, git or any other program, or other shell syntax such as cd, subshells, loops, background jobs " +
		"or redirection (2>/dev/null and 2>&1 aside), stops this review at once and discards your result as tool_violation.\n\n"
}()

// Arguments that start another program or delete files. The list is the
// one clean-review 0.2.0 blocks; agy's --sandbox keeps files from being
// written, and a change to the reviewed folders is workspace_modified.
var unsafeArgPrefixes = []string{"-exec", "-execdir", "-ok", "-okdir", "-delete", "--pre", "--compress-program"}

// packetFile is where a reviewer finds the packet when it is too large for
// one command-line argument.
const packetFile = "PACKET.md"

// argumentLimit keeps one argument under the operating system's limit:
// Linux refuses a single argument over 128 KiB, macOS limits all arguments
// and the environment together to 1 MiB.
func argumentLimit() int {
	if runtime.GOOS == "linux" {
		return 120 << 10
	}
	return 768 << 10
}

var sedPrint = regexp.MustCompile(`^(?:\d+|\$|/(?:[^/\\\n]|\\.)*/)(?:,(?:\d+|\$|/(?:[^/\\\n]|\\.)*/))?p$`)

// Error output that means the reviewer could not review at all: matched only
// against a CLI's error events and standard error, never its review text.
var (
	quotaCause = regexp.MustCompile(`(?i)RESOURCE_EXHAUSTED|(?:code|status)\W{0,4}429\b|\b429 too many requests|too many requests|` +
		`rate[ _-]?limit|quota (?:reached|exceeded|exhausted)|insufficient_quota|hit your (?:usage )?limit|usage limit reached`)
	authCause = regexp.MustCompile(`(?i)UNAUTHENTICATED|(?:code|status)\W{0,4}401\b|\b401 unauthorized|not logged in|` +
		`authentication (?:required|failed)|log ?in again|sign in again|codex login|agy login|claude login|run /login|` +
		`oauth token (?:has )?expired|(?:invalid|incorrect) api key`)
	resetText = regexp.MustCompile(`(?i)\b(?:resets?(?: in| at)?|try again (?:in|at))\s+\S*\d[^.\n]*`)
)

type reviewSpec struct {
	argv       []string
	display    []string
	stdin      []byte
	transport  string // how the packet reaches the reviewer
	model      string
	effort     string
	checkEvent func(map[string]any) string
	parse      func(stdout []byte) (result any, observed string, problem string)
	errors     func(stdout []byte) []string
}

type runPaths struct {
	workspace string // the reviewer's copy: head/ and base/
	dir       string // the reviewer's record folder in the round
	packet    string // the round's packet.md
	schema    string // the round's schema.json
}

func jsonLines(data []byte) []map[string]any {
	var out []map[string]any
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var event map[string]any
		if json.Unmarshal(line, &event) == nil {
			out = append(out, event)
		}
	}
	return out
}

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// errorMessage reads {"error": {"message": …}} and its common variants.
func errorMessage(v any) string {
	m := object(v)
	if m == nil {
		return ""
	}
	for _, key := range []string{"short_error", "message", "error"} {
		value := m[key]
		if inner := object(value); inner != nil {
			if s := errorMessage(inner); s != "" {
				return s
			}
			continue
		}
		if s := strings.TrimSpace(str(value)); s != "" {
			return s
		}
	}
	return ""
}

func specFor(r Reviewer, model, binary string, p runPaths, packet []byte) reviewSpec {
	switch r.Preset {
	case "claude":
		return claudeSpec(r, model, binary, p, packet)
	case "codex":
		return codexSpec(r, model, binary, p, packet)
	case "agy":
		return agySpec(r, model, binary, p, packet)
	default:
		return commandSpec(r, binary, p)
	}
}

func claudeSpec(r Reviewer, model, binary string, p runPaths, packet []byte) reviewSpec {
	schema := string(resultSchema())
	argv := []string{binary, "-p", "--model", model}
	if r.Effort != "" {
		argv = append(argv, "--effort", r.Effort)
	}
	argv = append(argv, "--safe-mode", "--restricted", "--strict-mcp-config", "--tools", strings.Join(claudeTools, ","),
		"--permission-mode", "dontAsk", "--no-session-persistence", "--disable-slash-commands",
		"--output-format", "stream-json", "--verbose", "--json-schema", schema)
	display := replaceArg(argv, schema, "<schema.json>")
	return reviewSpec{argv: argv, display: display, stdin: packet, transport: "stdin", model: model, effort: r.Effort,
		checkEvent: func(e map[string]any) string {
			if e["type"] == "system" && e["subtype"] == "init" {
				var extra []string
				for _, t := range anyList(e["tools"]) {
					if !claudeAllowed[str(t)] {
						extra = append(extra, str(t))
					}
				}
				if len(extra) > 0 {
					return "started with tools beyond read-only file access: " + strings.Join(extra, ", ")
				}
				if len(anyList(e["mcp_servers"])) > 0 {
					return "started with MCP servers"
				}
			}
			if e["type"] == "assistant" {
				for _, block := range anyList(object(e["message"])["content"]) {
					b := object(block)
					if b["type"] == "tool_use" && !claudeAllowed[str(b["name"])] {
						return "used a disallowed tool: " + str(b["name"])
					}
				}
			}
			return ""
		},
		parse: func(stdout []byte) (any, string, string) {
			var observed string
			var result map[string]any
			for _, e := range jsonLines(stdout) {
				if e["type"] == "system" && e["subtype"] == "init" {
					observed = str(e["model"])
				}
				if e["type"] == "result" {
					result = e
				}
			}
			if result == nil {
				return nil, observed, "no result event"
			}
			if out := object(result["structured_output"]); out != nil && result["is_error"] != true {
				return out, observed, ""
			}
			return nil, observed, fmt.Sprintf("no structured output (%v)", result["subtype"])
		},
		errors: func(stdout []byte) []string {
			var msgs []string
			for _, e := range jsonLines(stdout) {
				if e["type"] == "result" && e["is_error"] == true {
					msgs = append(msgs, fmt.Sprint(firstOf(e["result"], e["api_error_status"], e["subtype"])))
				} else if e["type"] == "error" {
					msgs = append(msgs, errorMessage(e))
				}
			}
			return msgs
		},
	}
}

func codexSpec(r Reviewer, model, binary string, p runPaths, packet []byte) reviewSpec {
	last := filepath.Join(p.dir, "last_message.json")
	argv := []string{binary, "exec", "--ignore-user-config", "--ignore-rules", "-m", model}
	if r.Effort != "" {
		argv = append(argv, "-c", fmt.Sprintf("model_reasoning_effort=%q", r.Effort))
	}
	argv = append(argv, "-c", `web_search="disabled"`)
	for _, f := range codexDisabled {
		argv = append(argv, "--disable", f)
	}
	argv = append(argv, "-s", "read-only", "--ephemeral", "--skip-git-repo-check", "-C", p.workspace, "--json",
		"--output-schema", p.schema, "-o", last, "-")
	return reviewSpec{argv: argv, display: argv, stdin: packet, transport: "stdin", model: model, effort: r.Effort,
		checkEvent: func(e map[string]any) string {
			if item := object(e["item"]); item != nil && !codexAllowed[str(item["type"])] {
				return "used a disallowed tool: " + str(item["type"])
			}
			return ""
		},
		parse: func(stdout []byte) (any, string, string) {
			data, err := os.ReadFile(last)
			if err != nil {
				return nil, "", "no final message"
			}
			var out any
			if err := json.Unmarshal(data, &out); err != nil {
				return nil, "", "final message is not JSON: " + err.Error()
			}
			return out, "", ""
		},
		errors: func(stdout []byte) []string {
			var msgs []string
			for _, e := range jsonLines(stdout) {
				item := object(e["item"])
				if e["type"] == "error" || e["type"] == "turn.failed" {
					msgs = append(msgs, errorMessage(e))
				} else if item != nil && item["type"] == "error" {
					msgs = append(msgs, errorMessage(item))
				}
			}
			return msgs
		},
	}
}

func agySpec(r Reviewer, model, binary string, p runPaths, packet []byte) reviewSpec {
	// agy takes its prompt as an argument: the command note, then the packet.
	// When the two are over the argument limit, the packet goes into the
	// workspace unchanged and the prompt is the note and a pointer to it.
	prompt, transport := agyCommandNote+string(packet), "argument"
	shown := agyCommandNote + "<packet.md>"
	if len(prompt) > argumentLimit() {
		if err := os.WriteFile(filepath.Join(p.workspace, packetFile), packet, 0o644); err == nil {
			prompt, transport = agyCommandNote+"The review packet is the file "+packetFile+" in the current folder, next to head/ and base/. "+
				"Read all of it and follow it exactly; it is the whole task.", "file"
			shown = prompt
		}
	}
	argv := []string{binary, "-p", prompt, "--model", model, "--sandbox", "--disable-slash-commands",
		"--output-format", "stream-json", "--json-schema", p.schema,
		"--print-timeout", fmt.Sprintf("%ds", r.TimeoutSeconds), "--log-file", filepath.Join(p.dir, "agy-runtime.log")}
	return reviewSpec{argv: argv, display: replaceArg(argv, prompt, shown), transport: transport, model: model,
		checkEvent: func(e map[string]any) string {
			step := object(e["step_update"])
			if step == nil || step["step_type"] != "tool" {
				return ""
			}
			name := str(step["tool_name"])
			params := object(object(step["tool_info"])["parameters"])
			switch {
			case name == "run_command":
				line := str(params["CommandLine"])
				if why := unsafeCommand(line); why != "" {
					return fmt.Sprintf("ran a command that is not read-only (%s): %s", why, short(line, 200))
				}
			case name == "manage_task":
				if strings.ToLower(str(params["Action"])) != "status" {
					return fmt.Sprintf("used manage_task for %q, not only to read a task's status", str(params["Action"]))
				}
			case !agyAllowed[name]:
				return "used a disallowed tool: " + name
			}
			return ""
		},
		parse: func(stdout []byte) (any, string, string) {
			var observed string
			var result map[string]any
			for _, e := range jsonLines(stdout) {
				if init := object(e["init"]); init != nil {
					observed = str(init["model"])
				}
				if res := object(e["result"]); res != nil {
					result = res
				}
			}
			if result == nil {
				return nil, observed, "no result event"
			}
			if out := object(result["structured_output"]); out != nil {
				return out, observed, ""
			}
			return nil, observed, fmt.Sprintf("no structured output (status %v)", result["status"])
		},
		errors: func(stdout []byte) []string {
			var msgs []string
			for _, e := range jsonLines(stdout) {
				if res := object(e["result"]); res != nil {
					msgs = append(msgs, errorMessage(map[string]any{"error": res["error"]}))
				}
			}
			return msgs
		},
	}
}

// commandSpec runs a configured argv with its placeholders replaced. Its
// result is the JSON object it prints; nothing it does is checked.
func commandSpec(r Reviewer, binary string, p runPaths) reviewSpec {
	replace := strings.NewReplacer("{prompt}", p.packet, "{workspace}", p.workspace, "{schema}", p.schema)
	argv := make([]string, len(r.Argv))
	for i, a := range r.Argv {
		argv[i] = replace.Replace(a)
	}
	argv[0] = binary
	return reviewSpec{argv: argv, display: argv, transport: "file",
		checkEvent: func(map[string]any) string { return "" },
		parse: func(stdout []byte) (any, string, string) {
			if out, ok := lastJSONObject(stdout); ok {
				return out, "", ""
			}
			return nil, "", "standard output holds no JSON object"
		},
		errors: func([]byte) []string { return nil },
	}
}

// lastJSONObject reads a whole output, or its last line, or a fenced block,
// as one JSON object.
func lastJSONObject(data []byte) (map[string]any, bool) {
	text := strings.TrimSpace(string(data))
	candidates := []string{text, unfence(text)}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			candidates = append(candidates, l)
			break
		}
	}
	for _, c := range candidates {
		var out map[string]any
		if c != "" && json.Unmarshal([]byte(c), &out) == nil && out != nil {
			return out, true
		}
	}
	return nil, false
}

var fenced = regexp.MustCompile("(?s)```(?:json)?\\s*\n(.*?)\n```")

func unfence(text string) string {
	if m := fenced.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	return text
}

func anyList(v any) []any {
	l, _ := v.([]any)
	return l
}

func firstOf(values ...any) any {
	for _, v := range values {
		if v != nil && v != "" {
			return v
		}
	}
	return ""
}

func replaceArg(argv []string, old, display string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a == old {
			out[i] = display
		} else {
			out[i] = a
		}
	}
	return out
}

func short(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) <= limit {
		return text
	}
	return string([]rune(text)[:limit-1]) + "…"
}

// shellToken is one token of a command line: a word, or a run of the
// operator characters ();<>|& outside quotes and backslashes. expansion is
// the first character in a word that the shell would expand into other
// text: $ outside single quotes, or { outside quotes other than the word {}.
type shellToken struct {
	text      string
	op        bool
	expansion string
}

// shellWords splits a command line the way a POSIX shell would for these
// purposes. Operator characters that are quoted or escaped belong to a word.
func shellWords(line string) ([]shellToken, error) {
	var tokens []shellToken
	var cur strings.Builder
	inWord := false
	expansion := ""
	expand := func(c string) {
		if expansion == "" {
			expansion = c
		}
	}
	flush := func() {
		if inWord {
			tokens = append(tokens, shellToken{text: cur.String(), expansion: expansion})
			cur.Reset()
			inWord, expansion = false, ""
		}
	}
	boundary := func(i int) bool {
		return i >= len(line) || strings.ContainsRune(" \t();<>|&", rune(line[i]))
	}
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\'':
			j := i + 1
			for j < len(runes) && runes[j] != '\'' {
				j++
			}
			if j >= len(runes) {
				return nil, errors.New("unclosed quote")
			}
			cur.WriteString(string(runes[i+1 : j]))
			inWord, i = true, j
		case c == '"':
			j := i + 1
			for ; j < len(runes) && runes[j] != '"'; j++ {
				if runes[j] == '\\' && j+1 < len(runes) {
					j++
				} else if runes[j] == '$' {
					expand("$")
				}
				cur.WriteRune(runes[j])
			}
			if j >= len(runes) {
				return nil, errors.New("unclosed quote")
			}
			inWord, i = true, j
		case c == '\\' && i+1 < len(runes):
			cur.WriteRune(runes[i+1])
			inWord = true
			i++
		case c == ' ' || c == '\t':
			flush()
		case strings.ContainsRune("();<>|&", c):
			flush()
			j := i
			for j < len(runes) && strings.ContainsRune("();<>|&", runes[j]) {
				j++
			}
			tokens = append(tokens, shellToken{text: string(runes[i:j]), op: true})
			i = j - 1
		case c == '{' && !inWord && i+1 < len(runes) && runes[i+1] == '}' && boundary(len(string(runes[:i+2]))):
			cur.WriteString("{}")
			inWord = true
			i++
		default:
			if c == '$' || c == '{' {
				expand(string(c))
			}
			cur.WriteRune(c)
			inWord = true
		}
	}
	flush()
	return tokens, nil
}

// unsafeCommand says why a shell line is not a read-only listing or search.
// A line may join read-only commands with |, &&, || or ; and nothing else.
// The reason names the first problem: a shell construct anywhere in the
// line, else the first part, read from the left, that breaks the rule.
func unsafeCommand(line string) string {
	text := strings.NewReplacer("2>/dev/null", " ", "2>&1", " ").Replace(line)
	for _, bad := range []string{"`", "$(", "<(", ">(", "\n"} {
		if strings.Contains(text, bad) {
			return fmt.Sprintf("shell construct %q", bad)
		}
	}
	tokens, err := shellWords(text)
	if err != nil {
		return "unparsable command (" + err.Error() + ")"
	}
	var cur []shellToken
	for _, t := range tokens {
		switch {
		case !t.op:
			cur = append(cur, t)
			continue
		case !commandSeparators[t.text]:
			return fmt.Sprintf("shell operator %q", t.text)
		case len(cur) == 0:
			return fmt.Sprintf("shell operator %q with no command before it", t.text)
		}
		if why := unsafeSimpleCommand(cur); why != "" {
			return why
		}
		cur = nil
	}
	if len(cur) == 0 {
		if len(tokens) > 0 {
			return fmt.Sprintf("shell operator %q with no command after it", tokens[len(tokens)-1].text)
		}
		return "empty command"
	}
	return unsafeSimpleCommand(cur)
}

// unsafeSimpleCommand says why one command between operators is not a
// read-only program or a sed that only prints, reading its words from the
// left.
func unsafeSimpleCommand(words []shellToken) string {
	if words[0].expansion != "" {
		return fmt.Sprintf("shell expansion %q", words[0].expansion)
	}
	program := filepath.Base(words[0].text)
	if program != "sed" && !readOnlyPrograms[program] {
		return fmt.Sprintf("program %q", program)
	}
	for _, w := range words[1:] {
		if w.expansion != "" {
			return fmt.Sprintf("shell expansion %q", w.expansion)
		}
		for _, p := range unsafeArgPrefixes {
			if strings.HasPrefix(w.text, p) {
				return fmt.Sprintf("argument %q", w.text)
			}
		}
	}
	if program == "sed" {
		return unsafeSed(words[1:])
	}
	return ""
}

// unsafeSed accepts sed only when it prints: -n (or --quiet, --silent),
// optionally -E or -r, and scripts that are all print-only, given with -e,
// --expression= or as the first operand. Any other option is a violation.
func unsafeSed(args []shellToken) string {
	quiet := false
	var scripts, operands []string
	for i := 0; i < len(args); i++ {
		t := args[i].text
		switch {
		case t == "-n" || t == "--quiet" || t == "--silent":
			quiet = true
		case t == "-E" || t == "-r" || t == "--regexp-extended":
		case t == "-e":
			if i+1 >= len(args) {
				return fmt.Sprintf("argument %q without a script", t)
			}
			i++
			scripts = append(scripts, args[i].text)
		case strings.HasPrefix(t, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(t, "--expression="))
		case strings.HasPrefix(t, "-") && t != "-":
			return fmt.Sprintf("argument %q", t)
		default:
			operands = append(operands, t)
		}
	}
	if len(scripts) == 0 && len(operands) > 0 {
		scripts = operands[:1]
	}
	if !quiet {
		return `program "sed" without -n`
	}
	if len(scripts) == 0 {
		return `program "sed" without a script`
	}
	for _, script := range scripts {
		if !sedPrint.MatchString(script) {
			return fmt.Sprintf("sed script %q that does more than print lines (sed -n 'N,Mp' or '/regex/,/regex/p')", script)
		}
	}
	return ""
}

// unavailableCause recognizes a quota or authentication refusal.
func unavailableCause(messages []string) (reason, resets string, quota bool) {
	for _, m := range messages {
		if r := resetText.FindString(strings.Join(strings.Fields(m), " ")); r != "" {
			resets = strings.TrimSpace(r)
			break
		}
	}
	for _, m := range messages {
		m = strings.Join(strings.Fields(m), " ")
		switch {
		case quotaCause.MatchString(m):
			return "quota or rate limit: " + short(m, 300), resets, true
		case authCause.MatchString(m):
			return "authentication: " + short(m, 300), resets, false
		}
	}
	return "", "", false
}

func stderrMessages(data []byte) (found, lines []string) {
	all := strings.Split(string(data), "\n")
	if len(all) > 200 {
		all = all[len(all)-200:]
	}
	for _, line := range all {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if i := strings.Index(line, "{"); i >= 0 {
			var obj map[string]any
			if json.Unmarshal([]byte(line[i:]), &obj) == nil {
				if msg := errorMessage(obj); msg != "" {
					found = append(found, msg)
				}
			}
		}
	}
	return found, lines
}

// resolveBinary finds a reviewer's executable. A missing one makes the
// reviewer unavailable; a file that cannot run makes it failed.
func resolveBinary(r Reviewer) (path, status, reason string) {
	name := r.Binary
	if name == "" {
		if r.Preset == "command" {
			name = r.Argv[0]
		} else {
			name = r.Preset
		}
	}
	if strings.ContainsRune(name, os.PathSeparator) || strings.Contains(name, "/") {
		info, err := os.Stat(name)
		switch {
		case err != nil:
			return "", StatusUnavailable, fmt.Sprintf("%s not found: %s", r.Preset, name)
		case !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0:
			return "", StatusFailed, fmt.Sprintf("%s is not an executable file", name)
		}
		abs, err := filepath.Abs(name)
		if err != nil {
			return "", StatusFailed, err.Error()
		}
		return abs, "", ""
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return "", StatusUnavailable, fmt.Sprintf("%s not found on PATH", name)
	}
	return found, "", ""
}

// toolVersion asks the reviewer's executable for its version within the
// attempt's time limit (G19): it reports false when the limit ran out first.
func toolVersion(binary string, deadline time.Time) (string, bool) {
	cmd := exec.Command(binary, "--version")
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	ownGroup(cmd)
	if cmd.Start() != nil {
		return "", true
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	limit := time.Until(deadline)
	if limit > 30*time.Second {
		limit = 30 * time.Second
	}
	select {
	case <-done:
	case <-time.After(limit):
		signalGroup(cmd, true)
		<-done
		if !time.Now().Before(deadline) {
			return "", false
		}
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out.String()), "\n")
	return short(first, 200), true
}

// Attempt is one run of a reviewer with one model.
type Attempt struct {
	Model           string   `json:"model"`
	PacketTransport string   `json:"packet_transport,omitempty"`
	Status          string   `json:"status"`
	Reason          string   `json:"reason,omitempty"`
	Resets          string   `json:"resets,omitempty"`
	ObservedModel   string   `json:"observed_model,omitempty"`
	ExitCode        *int     `json:"exit_code,omitempty"`
	ElapsedSeconds  float64  `json:"elapsed_seconds"`
	Argv            []string `json:"argv,omitempty"`
	CLIVersion      string   `json:"cli_version,omitempty"`
	StartedAt       string   `json:"started_at"`
	FinishedAt      string   `json:"finished_at"`
	quota           bool
	result          any
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// runAttempt starts one reviewer CLI in its workspace copy, stops it on a
// forbidden event or at its time limit, and judges what it left.
func runAttempt(r Reviewer, model string, p runPaths, packet []byte, angles []string, head string) Attempt {
	a := Attempt{Model: model, StartedAt: now()}
	// The time limit covers the whole attempt, the version check included.
	limit := time.Duration(r.TimeoutSeconds) * time.Second
	deadlineAt := time.Now().Add(limit)
	finish := func(status, reason string) Attempt {
		a.Status, a.Reason, a.FinishedAt = status, reason, now()
		return a
	}
	binary, status, reason := resolveBinary(r)
	if status != "" {
		return finish(status, reason)
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return finish(StatusFailed, err.Error())
	}
	spec := specFor(r, model, binary, p, packet)
	a.Argv, a.PacketTransport = spec.display, spec.transport
	if r.Preset != "command" {
		version, inTime := toolVersion(binary, deadlineAt)
		if !inTime {
			a.ElapsedSeconds = float64(limit.Milliseconds()) / 1000
			return finish(StatusTimedOut, fmt.Sprintf("the version check of %s did not finish within %d seconds", binary, r.TimeoutSeconds))
		}
		a.CLIVersion = version
	}
	before, err := manifest(p.workspace)
	if err != nil {
		return finish(StatusFailed, "workspace: "+err.Error())
	}
	stdoutPath := filepath.Join(p.dir, "stdout.jsonl")
	stderrPath := filepath.Join(p.dir, "stderr.log")
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		return finish(StatusFailed, err.Error())
	}
	stderr, err := os.Create(stderrPath)
	if err != nil {
		stdout.Close()
		return finish(StatusFailed, err.Error())
	}
	cmd := exec.Command(spec.argv[0], spec.argv[1:]...)
	cmd.Dir = p.workspace
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if spec.stdin != nil {
		cmd.Stdin = bytes.NewReader(spec.stdin)
	}
	ownGroup(cmd)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return finish(StatusFailed, "could not start "+binary+": "+err.Error())
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stop := func() {
		signalGroup(cmd, false)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			signalGroup(cmd, true)
			<-done
		}
	}
	var violation string
	timedOut := false
	offset := int64(0)
	deadline := time.After(time.Until(deadlineAt))
	ticker := time.NewTicker(200 * time.Millisecond)
loop:
	for {
		select {
		case <-done:
			break loop
		case <-deadline:
			timedOut = true
			stop()
			break loop
		case <-ticker.C:
			violation, offset = scanEvents(stdoutPath, offset, spec.checkEvent)
			if violation != "" {
				stop()
				break loop
			}
		}
	}
	ticker.Stop()
	stdout.Close()
	stderr.Close()
	a.ElapsedSeconds = float64(time.Since(started).Milliseconds()) / 1000
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		a.ExitCode = &code
	}
	out, _ := os.ReadFile(stdoutPath)
	if violation == "" && !timedOut {
		violation, _ = scanEvents(stdoutPath, 0, spec.checkEvent)
	}
	result, observed, problem := spec.parse(out)
	a.ObservedModel = observed
	after, err := manifest(p.workspace)
	changed := changedPaths(before, after)
	switch {
	case err != nil:
		return finish(StatusWorkspaceModified, "workspace could not be read after the review: "+err.Error())
	case len(changed) > 0:
		return finish(StatusWorkspaceModified, "reviewed files changed during the review: "+strings.Join(first(changed, 5), ", "))
	case violation != "":
		return finish(StatusToolViolation, violation)
	case timedOut:
		return finish(StatusTimedOut, fmt.Sprintf("no result within %ds", r.TimeoutSeconds))
	case result == nil:
		errData, _ := os.ReadFile(stderrPath)
		found, lines := stderrMessages(errData)
		reported := append(found, spec.errors(out)...)
		var nonEmpty []string
		for _, m := range reported {
			if strings.TrimSpace(m) != "" {
				nonEmpty = append(nonEmpty, m)
			}
		}
		if why, resets, quota := unavailableCause(append(nonEmpty, lines...)); why != "" {
			a.Resets, a.quota = resets, quota
			if resets != "" && !strings.Contains(why, resets) {
				why += " (" + resets + ")"
			}
			return finish(StatusUnavailable, why)
		}
		detail := problem
		if len(nonEmpty) > 0 {
			detail += "; the CLI reported: " + short(nonEmpty[0], 200)
		} else if len(lines) > 0 {
			detail += "; standard error: " + short(lines[len(lines)-1], 200)
		}
		if a.ExitCode != nil && *a.ExitCode != 0 {
			return finish(StatusFailed, fmt.Sprintf("exit code %d: %s", *a.ExitCode, detail))
		}
		return finish(StatusInvalidOutput, detail)
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(filepath.Join(p.dir, "result.json"), append(data, '\n'), 0o644)
	if problems := validateResult(result, head, angles); len(problems) > 0 {
		return finish(StatusInvalidOutput, strings.Join(first(problems, 5), "; "))
	}
	a.result = result
	if str(object(result)["verdict"]) == "could_not_review" {
		return finish(StatusFailed, "the reviewer reported it could not review: "+short(str(object(result)["limits"]), 300))
	}
	return finish(StatusCompleted, "")
}

func first(list []string, n int) []string {
	if len(list) > n {
		return list[:n]
	}
	return list
}

// scanEvents checks the complete lines written after offset.
func scanEvents(path string, offset int64, check func(map[string]any) string) (string, int64) {
	f, err := os.Open(path)
	if err != nil {
		return "", offset
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return "", offset
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return "", offset // an incomplete line is read again later
		}
		offset += int64(len(line))
		var event map[string]any
		if json.Unmarshal(bytes.TrimSpace(line), &event) == nil && event != nil {
			if v := check(event); v != "" {
				return v, offset
			}
		}
	}
}

// manifest identifies every path under the reviewed folders of a workspace
// and the packet file when there is one.
func manifest(root string) (map[string]string, error) {
	entries := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(root, packetFile)); err == nil {
		entries[packetFile] = "F:" + newDocument("", "", data).SHA256
	}
	for _, side := range []string{"head", "base"} {
		top := filepath.Join(root, side)
		if _, err := os.Lstat(top); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(top, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			switch {
			case d.Type()&os.ModeSymlink != 0:
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				entries[rel] = "L:" + target
			case d.IsDir():
				entries[rel] = "D"
			default:
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				entries[rel] = fmt.Sprintf("F:%o:%s", info.Mode().Perm()&0o111, newDocument("", "", data).SHA256)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func manifestDigest(entries map[string]string) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "\x00" + entries[k] + "\x00")
	}
	return newDocument("", "", []byte(b.String())).SHA256
}

func changedPaths(before, after map[string]string) []string {
	var changed []string
	for k, v := range before {
		if after[k] != v {
			changed = append(changed, k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}

// validateResult checks a reviewer's result against the schema and the
// round: its head, every requested angle, and each finding's fields.
// findingKeys are the keys of a finding in a reviewer's result.
var findingKeys = []string{"id", "proposed_class", "angle", "location", "claim", "failure_scenario", "evidence",
	"link_kind", "link", "introduced_by_change", "regression", "check_impact", "confidence"}

// optionalFindingKeys may be left out of a finding: a result that follows
// an earlier packet, or a reviewer no schema binds, has no smallest_fix.
var optionalFindingKeys = []string{"smallest_fix"}

// unknownKeys lists, sorted, the keys of m outside known.
func unknownKeys(m map[string]any, known []string) []string {
	var out []string
	for k := range m {
		if !contains(known, k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func validateResult(result any, head string, angles []string) []string {
	m := object(result)
	if m == nil {
		return []string{"result is not a JSON object"}
	}
	var problems []string
	resultKeys := []string{"reviewed_head", "verdict", "angles_checked", "angles_unchecked", "findings", "limits"}
	for _, key := range resultKeys {
		if _, ok := m[key]; !ok {
			problems = append(problems, "missing "+key)
		}
	}
	// The keys are exactly the schema's (G1).
	for _, key := range unknownKeys(m, resultKeys) {
		problems = append(problems, fmt.Sprintf("unknown key %q", key))
	}
	// limits is a string, possibly empty (G6).
	if v, present := m["limits"]; present {
		if _, ok := v.(string); !ok {
			problems = append(problems, "limits is not a string")
		}
	}
	reviewed := str(m["reviewed_head"])
	if len(reviewed) < 7 || !strings.HasPrefix(head, reviewed) {
		problems = append(problems, fmt.Sprintf("reviewed_head %q is not the packet's head", reviewed))
	}
	if !contains(verdicts, str(m["verdict"])) {
		problems = append(problems, fmt.Sprintf("bad verdict %q", str(m["verdict"])))
	}
	seen := map[string]bool{}
	for _, key := range []string{"angles_checked", "angles_unchecked"} {
		list, ok := m[key].([]any)
		if !ok {
			if _, present := m[key]; present {
				problems = append(problems, key+" is not a list")
			}
			continue
		}
		// Each entry is {"angle", "result"} when checked and {"angle",
		// "reason"} when not, as the schema says.
		second := "result"
		if key == "angles_unchecked" {
			second = "reason"
		}
		for i, item := range list {
			entry := object(item)
			if entry == nil {
				problems = append(problems, fmt.Sprintf("%s[%d] is not an object", key, i))
				continue
			}
			a := str(entry["angle"])
			if !contains(Angles, a) {
				problems = append(problems, fmt.Sprintf("%s[%d] has angle %q", key, i, a))
				continue
			}
			if _, ok := entry[second].(string); !ok {
				problems = append(problems, fmt.Sprintf("%s[%d] lacks %s", key, i, second))
			}
			keys := make([]string, 0, len(entry))
			for k := range entry {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if k != "angle" && k != second {
					problems = append(problems, fmt.Sprintf("%s[%d] has unknown key %q", key, i, k))
				}
			}
			seen[a] = true
		}
	}
	for _, a := range angles {
		if !seen[a] {
			problems = append(problems, "angle "+a+" is neither checked nor unchecked")
		}
	}
	findings, ok := m["findings"].([]any)
	if !ok {
		return append(problems, "findings is not a list")
	}
	for i, item := range findings {
		f := object(item)
		if f == nil {
			problems = append(problems, fmt.Sprintf("finding %d is not an object", i))
			continue
		}
		for _, key := range findingKeys {
			if _, ok := f[key].(string); !ok {
				problems = append(problems, fmt.Sprintf("finding %d lacks %s", i, key))
			}
		}
		// The keys are exactly the schema's, smallest_fix being optional (G5).
		for _, key := range unknownKeys(f, append(append([]string{}, findingKeys...), optionalFindingKeys...)) {
			problems = append(problems, fmt.Sprintf("finding %d has unknown key %q", i, key))
		}
		for _, key := range optionalFindingKeys {
			if v, present := f[key]; present {
				if _, ok := v.(string); !ok {
					problems = append(problems, fmt.Sprintf("finding %d has a %s that is not a string", i, key))
				}
			}
		}
		// The reviewer's own id is any string: spar numbers the findings
		// itself and keeps that id only as reviewer_id.
		for key, allowed := range map[string][]string{"proposed_class": Classes, "angle": Angles, "link_kind": linkKinds,
			"introduced_by_change": yesNo, "regression": yesNo, "check_impact": checkImpact, "confidence": confidences} {
			if v, ok := f[key].(string); ok && !contains(allowed, v) {
				problems = append(problems, fmt.Sprintf("finding %d has %s %q", i, key, v))
			}
		}
		if strings.TrimSpace(str(f["failure_scenario"])) == "" {
			problems = append(problems, fmt.Sprintf("finding %d has no failure scenario", i))
		}
	}
	sort.Strings(problems)
	return problems
}
