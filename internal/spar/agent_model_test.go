//go:build !windows

package spar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// These tests observe how a reviewer's model and effort are chosen: the
// value the user's configuration writes, else the value the agent passes
// for one run (--model NAME=MODEL, --effort NAME=EFFORT), else the preset's.
// They drive the spar command line (Main) on a synthetic repository. The
// claude, codex and agy reviewers run /bin/sh scripts that log the
// arguments they receive and print a result in that CLI's output form; a
// model whose name starts with "quota-" is refused for quota and the model
// "unknown-model" fails. No reviewer CLI or model runs. The opening line of
// an assertion's failure message names the property it observes
// (jaekit:violation=<id>); a failure that starts with "setup:" is a broken
// setup, not an observation.

type amWorld struct {
	t               *testing.T
	dir, repo       string
	base, head      string
	criteria, rules string
	bin             string            // the fake CLIs, also first on PATH
	env             map[string]string // what spar's getenv returns
	n               int
}

// newAMWorld makes a repository whose head changes a greeting. Without git
// a check of a change condition is skipped and a check of a kept behavior
// fails, so neither passes.
func newAMWorld(t *testing.T, keeps bool) *amWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if keeps {
			t.Fatalf("setup: git is not available: %v", err)
		}
		t.Skipf("git is not available: %v", err)
	}
	w := &amWorld{t: t, dir: realish(t.TempDir())}
	w.repo = filepath.Join(w.dir, "repo")
	w.bin = filepath.Join(w.dir, "bin")
	w.env = map[string]string{"HOME": w.dir, "XDG_CONFIG_HOME": filepath.Join(w.dir, "xdg")}
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n")
	w.rules = w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\n- A greeting never ends with a comma.\n")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello ' + name\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "greet.py"), "def greet(name):\n    return 'hello, ' + name\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "# Goal\n\n- **AC-1** greet('') returns 'hello'.\n")
	if err := os.MkdirAll(w.bin, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("PATH", w.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return w
}

func (w *amWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return path
}

func (w *amWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("setup: git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// use makes body the user's configuration file of later runs and returns
// its path.
func (w *amWorld) use(body string) string {
	w.t.Helper()
	w.n++
	path := w.write(filepath.Join(w.dir, fmt.Sprintf("spar-%d.toml", w.n)), body)
	w.env["SPAR_CONFIG"] = path
	return path
}

// spar runs the spar command line and returns its exit code, standard
// output and standard error. An internal error (70) observes nothing and
// is a broken setup.
func (w *amWorld) spar(args ...string) (int, string, string) {
	w.t.Helper()
	var out, errs bytes.Buffer
	code := Main("test", args, strings.NewReader(""), &out, &errs, func(key string) string { return w.env[key] })
	if code == ExitInternal {
		w.t.Fatalf("setup: spar %v exits %d: %s", args, code, errs.String())
	}
	return code, out.String(), errs.String()
}

// round is a fresh round folder path in a fresh series.
func (w *amWorld) round(label string) string {
	w.n++
	return filepath.Join(w.repo, "reviews", fmt.Sprintf("%s-%d", label, w.n), "01-candidate")
}

// run starts a candidate round with extra arguments and returns its exit
// code and standard error.
func (w *amWorld) run(out string, extra ...string) (int, string) {
	w.t.Helper()
	args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--criteria", w.criteria,
		"--rules", w.rules, "--out", out, "--repo", w.repo}, extra...)
	code, _, errs := w.spar(args...)
	return code, errs
}

// answer is a result for the world's head with every angle checked and no
// finding.
func (w *amWorld) answer() string {
	w.t.Helper()
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": w.head, "verdict": "no_blocking_findings",
		"angles_checked": checked, "angles_unchecked": []any{}, "findings": []any{}, "limits": "synthetic reviewer"})
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	w.n++
	return w.write(filepath.Join(w.dir, "answers", fmt.Sprintf("%d.json", w.n)), string(data))
}

func amQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// fake writes the CLI of a preset as an executable called file in the fake
// folder. Each call writes its arguments, NUL separated, to the next
// numbered file of calls/<file>. The model the arguments name (after
// --model or -m) decides the outcome: "quota-..." is a quota refusal,
// "unknown-model" a failure, anything else the answer.
func (w *amWorld) fake(file, preset string) string {
	w.t.Helper()
	calls := filepath.Join(w.dir, "calls", file)
	if err := os.MkdirAll(calls, 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	answer := amQuote(w.answer())
	outputs := map[string]string{
		"claude": "cat >/dev/null\n" +
			"printf '%s\\n' '{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"synthetic\",\"tools\":[\"Read\",\"Grep\",\"Glob\"],\"mcp_servers\":[]}'\n" +
			"printf '%s' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"structured_output\":'\n" +
			"cat " + answer + "\nprintf '%s\\n' '}'\n",
		"codex": "cat >/dev/null\nout=\nfor a in \"$@\"; do if [ \"$prev\" = -o ]; then out=$a; fi; prev=$a; done\n" +
			"cat " + answer + " > \"$out\"\n" +
			"printf '%s\\n' '{\"type\":\"thread.started\"}' '{\"type\":\"turn.completed\",\"usage\":{}}'\n",
		"agy": "printf '%s\\n' '{\"init\":{\"model\":\"synthetic\"}}'\n" +
			"printf '%s' '{\"result\":{\"status\":\"success\",\"structured_output\":'\n" +
			"cat " + answer + "\nprintf '%s\\n' '}}'\n",
	}
	script := "#!/bin/sh\n" +
		fmt.Sprintf("if [ \"$1\" = --version ]; then echo \"%s 0.0.0 (synthetic)\"; exit 0; fi\n", preset) +
		"n=$(ls " + amQuote(calls) + " | wc -l | tr -d ' ')\n" +
		"printf '%s\\0' \"$@\" > " + amQuote(calls) + "/$n\n" +
		"model=\nprev=\nfor a in \"$@\"; do case \"$prev\" in --model|-m) model=$a;; esac; prev=$a; done\n" +
		"prev=\n" +
		"case \"$model\" in\n" +
		"  quota-*) echo \"RESOURCE_EXHAUSTED (code 429): quota exceeded for $model\" >&2; exit 1;;\n" +
		"  unknown-model) echo \"error: unknown model $model\" >&2; exit 2;;\n" +
		"esac\n" + outputs[preset]
	path := w.write(filepath.Join(w.bin, file), script)
	if err := os.Chmod(path, 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	// A fake that cannot start (no /bin/sh, a noexec folder) would make
	// spar record a failed reviewer: that observes nothing.
	if out, err := exec.Command(path, "--version").CombinedOutput(); err != nil {
		w.t.Fatalf("setup: the fake %s cannot run: %v %s", path, err, out)
	}
	return path
}

// entry is a configuration entry for a reviewer whose binary is a fake CLI
// of its preset, with extra TOML lines.
func (w *amWorld) entry(name, preset string, extra ...string) string {
	w.t.Helper()
	bin := w.fake(name, preset)
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = %q\nbinary = %q\n%s\n", name, preset, bin, strings.Join(extra, "\n"))
}

func amNative(name string) string {
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = \"native\"\n\n", name)
}

func amCommand(w *amWorld, name string) string {
	return fmt.Sprintf("[[reviewers]]\nname = %q\npreset = \"command\"\nargv = [\"/bin/cat\", %q]\n\n", name, w.answer())
}

// calls reads the argument lists of every call of a fake CLI, in order.
func (w *amWorld) calls(file string) [][]string {
	w.t.Helper()
	dir := filepath.Join(w.dir, "calls", file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	var numbers []int
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			w.t.Fatalf("setup: unexpected call file %s", e.Name())
		}
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	var out [][]string
	for _, n := range numbers {
		data, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(n)))
		if err != nil {
			w.t.Fatalf("setup: %v", err)
		}
		args := strings.Split(string(data), "\x00")
		out = append(out, args[:len(args)-1])
	}
	return out
}

// clearCalls forgets the calls of every fake CLI.
func (w *amWorld) clearCalls() {
	w.t.Helper()
	dir := filepath.Join(w.dir, "calls")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		files, _ := os.ReadDir(filepath.Join(dir, e.Name()))
		for _, f := range files {
			if err := os.Remove(filepath.Join(dir, e.Name(), f.Name())); err != nil {
				w.t.Fatalf("setup: %v", err)
			}
		}
	}
}

// after is the argument that follows flag, or "" without one.
func amAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// amModelArg is the model a call names: --model for claude and agy, -m for
// codex.
func amModelArg(preset string, args []string) string {
	if preset == "codex" {
		return amAfter(args, "-m")
	}
	return amAfter(args, "--model")
}

// amEffortArg is the effort a call names: --effort for claude, the
// model_reasoning_effort setting for codex; "" means none.
func amEffortArg(preset string, args []string) string {
	if preset == "codex" {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "-c" && strings.HasPrefix(args[i+1], "model_reasoning_effort=") {
				v, err := strconv.Unquote(strings.TrimPrefix(args[i+1], "model_reasoning_effort="))
				if err != nil {
					return "<unreadable " + args[i+1] + ">"
				}
				return v
			}
		}
		return ""
	}
	return amAfter(args, "--effort")
}

// record reads a round's round.json; a round that spar reported written
// but cannot be read is a broken setup.
func (w *amWorld) record(round string) map[string]any {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(round, "round.json"))
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	var r map[string]any
	if err := json.Unmarshal(data, &r); err != nil {
		w.t.Fatalf("setup: round.json: %v", err)
	}
	return r
}

// amState is a reviewer's state in a round record.
func amState(rec map[string]any, name string) map[string]any {
	states, _ := rec["reviewers"].(map[string]any)
	st, _ := states[name].(map[string]any)
	return st
}

// amApplied is a reviewer's entry in a configuration record (round.json's
// config or spar config's output).
func amApplied(config map[string]any, name string) map[string]any {
	list, _ := config["reviewers"].([]any)
	for _, item := range list {
		if m, _ := item.(map[string]any); m != nil && m["name"] == name {
			return m
		}
	}
	return nil
}

func amConfig(rec map[string]any) map[string]any {
	c, _ := rec["config"].(map[string]any)
	return c
}

func amStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func amExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// summary reads a round's summary.md.
func (w *amWorld) summary(round string) string {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(round, "summary.md"))
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// AC-1: a reviewer whose model the user's configuration writes runs on that
// model, and the record names the user's configuration as its source; a
// written model equal to the preset's is written all the same.
func TestSparAgentModelAC1_WrittenModelRuns(t *testing.T) {
	const v = "written-model-not-applied"
	w := newAMWorld(t, false)
	written := []struct{ name, preset, model string }{
		{"claude", "claude", "user-claude"}, {"codex", "codex", "user-codex"}, {"agy", "agy", "user-agy"},
		{"claude-preset", "claude", "opus"},
	}
	var config string
	for _, x := range written {
		config += w.entry(x.name, x.preset, fmt.Sprintf("model = %q", x.model))
	}
	w.use(config)
	out := w.round("ac1")
	if code, errs := w.run(out); code != ExitOK {
		t.Fatalf("jaekit:violation=%s the round with written models exits %d: %s", v, code, errs)
	}
	rec := w.record(out)
	for _, x := range written {
		calls := w.calls(x.name)
		if len(calls) != 1 || amModelArg(x.preset, calls[0]) != x.model {
			t.Errorf("jaekit:violation=%s %s was not started once with its written model %q: %v", v, x.name, x.model, calls)
		}
		st := amState(rec, x.name)
		if amStr(st, "requested_model") != x.model || amStr(st, "model_source") != "user" {
			t.Errorf("jaekit:violation=%s %s records requested_model %q, model_source %q; want %q from user", v, x.name,
				amStr(st, "requested_model"), amStr(st, "model_source"), x.model)
		}
		applied := amApplied(amConfig(rec), x.name)
		if amStr(applied, "model") != x.model || amStr(applied, "model_source") != "user" {
			t.Errorf("jaekit:violation=%s config.reviewers %s has model %q, model_source %q; want %q from user", v, x.name,
				amStr(applied, "model"), amStr(applied, "model_source"), x.model)
		}
	}
}

// AC-2: the agent's model for a reviewer whose model is not written reaches
// that reviewer's CLI as its model argument, for every form of "not
// written": an entry without the key, the clean-review list, the default
// list and a --reviewers name outside the configuration.
func TestSparAgentModelAC2_AgentChoiceReachesCLI(t *testing.T) {
	const v = "agent-model-not-applied"
	w := newAMWorld(t, false)
	for _, preset := range []string{"claude", "codex", "agy"} {
		w.fake(preset, preset) // the presets found on PATH
	}
	choose := func(names ...string) []string {
		var args []string
		for _, n := range names {
			args = append(args, "--model", n+"=agent-"+n)
		}
		return args
	}
	type form struct {
		label  string
		setup  func()
		extra  []string
		chosen []string
	}
	forms := []form{
		{"an entry without model", func() {
			w.use(w.entry("e-claude", "claude") + w.entry("e-codex", "codex") + w.entry("e-agy", "agy"))
		}, nil, []string{"e-claude", "e-codex", "e-agy"}},
		{"the clean-review list", func() {
			delete(w.env, "SPAR_CONFIG")
			w.env["CLEAN_REVIEW_CONFIG"] = w.write(filepath.Join(w.dir, "clean-review.json"), `{"reviewers": ["claude", "codex", "agy"]}`)
		}, nil, []string{"claude", "codex", "agy"}},
		{"the default list", func() {
			delete(w.env, "SPAR_CONFIG")
			delete(w.env, "CLEAN_REVIEW_CONFIG")
		}, nil, []string{"claude", "codex", "agy"}},
		{"a --reviewers name outside the configuration", func() {
			w.use(amNative("host"))
		}, []string{"--reviewers", "codex"}, []string{"codex"}},
	}
	for _, f := range forms {
		f.setup()
		w.clearCalls()
		out := w.round("ac2")
		code, errs := w.run(out, append(f.extra, choose(f.chosen...)...)...)
		if code != ExitOK {
			t.Errorf("jaekit:violation=%s %s: the round with the agent's models exits %d: %s", v, f.label, code, errs)
			continue
		}
		rec := w.record(out)
		for _, name := range f.chosen {
			st := amState(rec, name)
			preset := amStr(st, "preset")
			calls := w.calls(name)
			if len(calls) != 1 || amModelArg(preset, calls[0]) != "agent-"+name {
				t.Errorf("jaekit:violation=%s %s: %s was not started once with the agent's model %q: %v", v, f.label, name, "agent-"+name, calls)
			}
			if amStr(st, "requested_model") != "agent-"+name || amStr(st, "model_source") != "agent" {
				t.Errorf("jaekit:violation=%s %s: %s records requested_model %q, model_source %q; want %q from agent", v, f.label,
					name, amStr(st, "requested_model"), amStr(st, "model_source"), "agent-"+name)
			}
		}
	}
}

// amRefusal is a value the agent may not pass, with the reviewer and the
// value its refusal names.
type amRefusal struct {
	label, name, value string
	args               []string
}

// amRefusalWorld is a world whose configuration holds every reviewer the
// refusals of AC-3 need, and those refusals: the complete list of what the
// agent may not pass (REVIEW, representative forms 1 and 2).
func amRefusalWorld(t *testing.T) (*amWorld, []amRefusal) {
	w := newAMWorld(t, false)
	w.use(w.entry("claude", "claude", `model = "user-claude"`, `effort = "low"`) + w.entry("claude2", "claude") +
		w.entry("claude3", "claude", `model = "opus"`, `effort = "high"`) +
		w.entry("codex", "codex", `allowed_models = ["codex-a", "codex-b"]`, `allowed_efforts = ["low"]`) +
		w.entry("codex2", "codex", `effort = ""`) + w.entry("agy", "agy") + amNative("host") + amCommand(w, "cmd"))
	refusals := []amRefusal{
		{"a written model", "claude", "other", []string{"--model", "claude=other"}},
		{"a written effort", "claude", "high", []string{"--effort", "claude=high"}},
		{"a written model equal to the preset's", "claude3", "opus", []string{"--model", "claude3=opus"}},
		{"another model for a written preset model", "claude3", "sonnet", []string{"--model", "claude3=sonnet"}},
		{"a written effort equal to the preset's", "claude3", "high", []string{"--effort", "claude3=high"}},
		{"a written empty effort", "codex2", "high", []string{"--effort", "codex2=high"}},
		{"a native model", "host", "x", []string{"--model", "host=x"}},
		{"a native effort", "host", "x", []string{"--effort", "host=x"}},
		{"a command model", "cmd", "x", []string{"--model", "cmd=x"}},
		{"a command effort", "cmd", "x", []string{"--effort", "cmd=x"}},
		{"an agy effort", "agy", "high", []string{"--effort", "agy=high"}},
		{"a name that is no reviewer", "nobody", "x", []string{"--model", "nobody=x"}},
		{"a reviewer left out of the run", "agy", "x", []string{"--reviewers", "codex", "--model", "agy=x"}},
		{"a model passed twice", "codex", "codex-b", []string{"--model", "codex=codex-a", "--model", "codex=codex-b"}},
		{"an effort passed twice", "codex", "low", []string{"--effort", "codex=low", "--effort", "codex=low"}},
		{"a model outside the allowed list", "codex", "codex-c", []string{"--model", "codex=codex-c"}},
		{"a model differing from the list in case", "codex", "CODEX-A", []string{"--model", "codex=CODEX-A"}},
		{"an effort outside the allowed list", "codex", "high", []string{"--effort", "codex=high"}},
	}
	for _, f := range []struct{ label, value string }{
		{"empty", ""}, {"starting with -", "-x"}, {"with a space", "a b"}, {"with a tab", "a\tb"},
		{"with a newline", "a\nb"}, {"with an ideographic space", "a\u3000b"}, {"with a control character", "a\x01b"},
		{"with DEL", "a\x7fb"}, {"with a byte that is not UTF-8", "a\xffb"}, {"with an encoded surrogate", "a\xed\xa0\x80b"},
	} {
		refusals = append(refusals,
			amRefusal{"a model " + f.label, "agy", f.value, []string{"--model", "agy=" + f.value}},
			amRefusal{"an effort " + f.label, "claude2", f.value, []string{"--effort", "claude2=" + f.value}})
	}
	return w, refusals
}

// amNoCalls reports the fake CLIs that were started.
func (w *amWorld) amNoCalls(label, v string) {
	w.t.Helper()
	for _, name := range []string{"claude", "claude2", "claude3", "codex", "codex2", "agy"} {
		if calls := w.calls(name); len(calls) > 0 {
			w.t.Errorf("jaekit:violation=%s %s: reviewer %s started although the value was refused", v, label, name)
		}
	}
}

// AC-3: a value the agent may not pass is refused before any reviewer
// starts and before the round folder exists, naming the reviewer and the
// value; any other value reaches the CLI unchanged as one argument.
func TestSparAgentModelAC3_RefusedChoices(t *testing.T) {
	const v = "refused-choice-not-reported"
	w, refusals := amRefusalWorld(t)
	for _, r := range refusals {
		w.clearCalls()
		out := w.round("ac3")
		code, errs := w.run(out, r.args...)
		if code == ExitOK || code == ExitNoResult || amExists(out) {
			t.Errorf("jaekit:violation=%s %s: spar exits %d and the round folder exists: %v", v, r.label, code, amExists(out))
		}
		if !strings.Contains(errs, strconv.Quote(r.name)) || !strings.Contains(errs, strconv.Quote(r.value)) {
			t.Errorf("jaekit:violation=%s %s: the message does not name reviewer %q and value %q: %s", v, r.label, r.name, r.value, errs)
		}
		w.amNoCalls(r.label, v)
	}
	// Other values reach the CLI as they are, one argument each.
	w.use(w.entry("claude", "claude") + w.entry("codex", "codex", `allowed_efforts = ["low"]`) + w.entry("agy", "agy"))
	for _, value := range []string{"vendor/model:tag@v1", "모델-1", "a=b", `"quoted"`} {
		w.clearCalls()
		out := w.round("ac3-ok")
		code, errs := w.run(out, "--model", "claude="+value, "--model", "agy="+value, "--model", "codex="+value, "--effort", "codex=low")
		if code != ExitOK {
			t.Errorf("jaekit:violation=%s the value %q is refused (exit %d): %s", v, value, code, errs)
			continue
		}
		for _, name := range []string{"claude", "codex", "agy"} {
			calls := w.calls(name)
			if len(calls) != 1 || amModelArg(name, calls[0]) != value {
				t.Errorf("jaekit:violation=%s %s did not receive %q as its model argument: %v", v, name, value, calls)
			}
		}
		if calls := w.calls("codex"); len(calls) == 1 && amEffortArg("codex", calls[0]) != "low" {
			t.Errorf("jaekit:violation=%s codex did not receive the effort low: %v", v, calls[0])
		}
	}
}

// AC-4: a model the CLI does not take leaves the reviewer not completed with
// the requested model recorded, and running it again on the same packet
// with another model is a rerun that the cap does not count.
func TestSparAgentModelAC4_UnknownModelFails(t *testing.T) {
	const v = "unknown-model-not-recorded"
	w := newAMWorld(t, false)
	w.use(w.entry("codex", "codex"))
	first := w.round("ac4")
	code, errs := w.run(first, "--model", "codex=unknown-model")
	if code != ExitNoResult {
		t.Fatalf("jaekit:violation=%s the round with an unknown model exits %d, want 1 (no reviewer completed): %s", v, code, errs)
	}
	rec := w.record(first)
	st := amState(rec, "codex")
	if amStr(st, "status") == StatusCompleted || amStr(st, "requested_model") != "unknown-model" {
		t.Fatalf("jaekit:violation=%s the round with an unknown model records codex %q with requested_model %q", v,
			amStr(st, "status"), amStr(st, "requested_model"))
	}
	gate := filepath.Join(w.dir, "gate-ac4.json")
	if code, _, errs := w.spar("gate", first, "--expect-head", w.head, "--out", gate, "--repo", w.repo); code != ExitDegraded {
		t.Errorf("jaekit:violation=%s the gate of the round codex did not complete exits %d, want degraded: %s", v, code, errs)
	}
	again := filepath.Join(filepath.Dir(first), "02-candidate")
	code, errs = w.run(again, "--model", "codex=codex-good")
	if code != ExitOK {
		t.Fatalf("jaekit:violation=%s the rerun with another model exits %d: %s", v, code, errs)
	}
	rec = w.record(again)
	st = amState(rec, "codex")
	if amStr(st, "status") != StatusCompleted || amStr(st, "requested_model") != "codex-good" {
		t.Fatalf("jaekit:violation=%s the rerun with another model records codex %q with requested_model %q", v,
			amStr(st, "status"), amStr(st, "requested_model"))
	}
	if counted, _ := rec["counted"].(bool); counted || rec["rerun_of"] == nil {
		t.Errorf("jaekit:violation=%s the rerun with another model is counted (%v) or names no earlier run (%v)", v, rec["counted"], rec["rerun_of"])
	}
	if code, _, errs := w.spar("gate", first, again, "--expect-head", w.head, "--out", gate, "--repo", w.repo); code != ExitOK {
		t.Errorf("jaekit:violation=%s the gate of both runs exits %d, want complete: %s", v, code, errs)
	}
}

// amSourceFacts are the model and effort facts of a reviewer's summary line.
func amSourceFacts(summary, name string) string {
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, "- "+name+": ") {
			return line
		}
	}
	return ""
}

// AC-5: round.json and summary.md show each reviewer's requested model and
// effort with where each came from, and round.json keeps the agent's values
// as given, or that there were none.
func TestSparAgentModelAC5_RecordShowsSources(t *testing.T) {
	const v = "model-source-not-recorded"
	w := newAMWorld(t, false)
	w.use(w.entry("claude", "claude", `model = "user-claude"`) + w.entry("codex", "codex", `effort = "low"`) + w.entry("agy", "agy") +
		w.entry("codex2", "codex", `effort = ""`))
	out := w.round("ac5")
	code, errs := w.run(out, "--model", "codex=agent-codex", "--effort", "claude=medium")
	if code != ExitOK {
		t.Fatalf("jaekit:violation=%s the round with the agent's values exits %d: %s", v, code, errs)
	}
	rec := w.record(out)
	want := map[string][4]string{ // model, model source, effort, effort source
		"claude": {"user-claude", "user", "medium", "agent"},
		"codex":  {"agent-codex", "agent", "low", "user"},
		"agy":    {"gemini-4-argon-high", "preset", "", ""},
		"codex2": {"gpt-6-astra", "preset", "", "user"},
	}
	summary := w.summary(out)
	for name, x := range want {
		for _, place := range []struct {
			where string
			m     map[string]any
			model string
		}{{"reviewers." + name, amState(rec, name), "requested_model"}, {"config.reviewers " + name, amApplied(amConfig(rec), name), "model"}} {
			got := [4]string{amStr(place.m, place.model), amStr(place.m, "model_source"), amStr(place.m, "effort"), amStr(place.m, "effort_source")}
			if got != x {
				t.Errorf("jaekit:violation=%s %s has model, source, effort, source %v, want %v", v, place.where, got, x)
			}
		}
		line := amSourceFacts(summary, name)
		if !strings.Contains(line, fmt.Sprintf("model %s [%s]", x[0], x[1])) {
			t.Errorf("jaekit:violation=%s the summary line of %s does not show model %s [%s]: %q", v, name, x[0], x[1], line)
		}
		// An effort the user wrote empty is no effort, from the user.
		effort := map[bool]string{true: x[2], false: "none"}[x[2] != ""]
		if x[3] != "" && !strings.Contains(line, fmt.Sprintf("effort %s [%s]", effort, x[3])) {
			t.Errorf("jaekit:violation=%s the summary line of %s does not show effort %s [%s]: %q", v, name, effort, x[3], line)
		}
	}
	var choices []map[string]string
	raw, _ := json.Marshal(amConfig(rec)["choices"])
	_ = json.Unmarshal(raw, &choices)
	sort.Slice(choices, func(i, j int) bool { return choices[i]["reviewer"] < choices[j]["reviewer"] })
	wantChoices := []map[string]string{{"reviewer": "claude", "key": "effort", "value": "medium"},
		{"reviewer": "codex", "key": "model", "value": "agent-codex"}}
	if !reflect.DeepEqual(choices, wantChoices) {
		t.Errorf("jaekit:violation=%s config.choices is %s, want the agent's two values", v, raw)
	}
	none := w.round("ac5-none")
	if code, errs := w.run(none); code != ExitOK {
		t.Fatalf("jaekit:violation=%s the round without the agent's values exits %d: %s", v, code, errs)
	}
	if c, ok := amConfig(w.record(none))["choices"].([]any); !ok || len(c) != 0 {
		t.Errorf("jaekit:violation=%s a round without the agent's values does not record an empty choices list: %v", v, amConfig(w.record(none))["choices"])
	}
}

// AC-6: spar config shows for each reviewer whether its model and effort are
// written, may be chosen, the allowed lists and the values used without a
// choice; with the same choices it prints the configuration the run
// records, and it refuses what spar run refuses.
func TestSparAgentModelAC6_ConfigShowsChoice(t *testing.T) {
	const v = "config-choice-not-shown"
	w := newAMWorld(t, false)
	w.use(w.entry("claude", "claude", `model = "user-claude"`) +
		w.entry("codex", "codex", `allowed_models = ["codex-a", "codex-b"]`, `allowed_efforts = ["low", "medium"]`) +
		w.entry("agy", "agy") + amNative("host"))
	show := func(args ...string) map[string]any {
		code, out, errs := w.spar(append([]string{"config", "--repo", w.repo}, args...)...)
		var c map[string]any
		if code != ExitOK || json.Unmarshal([]byte(out), &c) != nil {
			t.Errorf("jaekit:violation=%s spar config %v exits %d without a configuration: %s", v, args, code, errs)
			return nil
		}
		return c
	}
	plain := show()
	for _, x := range []struct {
		name   string
		fields map[string]any
	}{
		{"claude", map[string]any{"model": "user-claude", "model_source": "user", "default_model": nil,
			"effort": "high", "effort_source": "preset", "default_effort": "high"}},
		{"codex", map[string]any{"model": "gpt-6-astra", "model_source": "preset", "default_model": "gpt-6-astra",
			"allowed_models": []any{"codex-a", "codex-b"}, "effort_source": "preset", "default_effort": "high",
			"allowed_efforts": []any{"low", "medium"}}},
		{"agy", map[string]any{"model": "gemini-4-argon-high", "model_source": "preset", "default_model": "gemini-4-argon-high",
			"effort_source": nil, "allowed_models": nil}},
		{"host", map[string]any{"model_source": nil, "effort_source": nil, "default_model": nil}},
	} {
		applied := amApplied(plain, x.name)
		for key, want := range x.fields {
			if got := applied[key]; !reflect.DeepEqual(got, want) {
				t.Errorf("jaekit:violation=%s spar config shows %s.%s = %v, want %v", v, x.name, key, got, want)
			}
		}
	}
	if c, ok := plain["choices"].([]any); !ok || len(c) != 0 {
		t.Errorf("jaekit:violation=%s spar config without choices does not show an empty choices list: %v", v, plain["choices"])
	}
	chosen := show("--model", "codex=codex-a", "--effort", "codex=low", "--model", "agy=agent-agy")
	if codex := amApplied(chosen, "codex"); amStr(codex, "model") != "codex-a" || amStr(codex, "model_source") != "agent" ||
		amStr(codex, "default_model") != "gpt-6-astra" || amStr(codex, "effort") != "low" || amStr(codex, "effort_source") != "agent" {
		t.Errorf("jaekit:violation=%s spar config with the agent's values shows codex %v", v, codex)
	}
	out := w.round("ac6")
	if code, errs := w.run(out, "--dry-run", "--model", "codex=codex-a", "--effort", "codex=low", "--model", "agy=agent-agy"); code != ExitOK {
		t.Errorf("jaekit:violation=%s the dry run with the same values exits %d: %s", v, code, errs)
	} else if rec := amConfig(w.record(out)); chosen != nil && !reflect.DeepEqual(rec, chosen) {
		a, _ := json.Marshal(chosen)
		b, _ := json.Marshal(rec)
		t.Errorf("jaekit:violation=%s spar config and the run record differ:\n%s\n%s", v, a, b)
	}
	// spar config refuses every value spar run refuses, with the same
	// message.
	rw, refusals := amRefusalWorld(t)
	for _, r := range refusals {
		out := rw.round("ac6-refused")
		_, runErrs := rw.run(out, r.args...)
		code, _, errs := rw.spar(append([]string{"config", "--repo", rw.repo}, r.args...)...)
		first := func(s string) string { line, _, _ := strings.Cut(s, "\n"); return line }
		if code != ExitRefused || !strings.Contains(errs, strconv.Quote(r.name)) || !strings.Contains(errs, strconv.Quote(r.value)) {
			t.Errorf("jaekit:violation=%s %s: spar config exits %d without naming %q and %q: %s", v, r.label, code, r.name, r.value, errs)
		} else if first(errs) != first(runErrs) {
			t.Errorf("jaekit:violation=%s %s: spar config and spar run refuse for different reasons:\n%s\n%s", v, r.label, first(errs), first(runErrs))
		}
		rw.amNoCalls(r.label, v)
	}
}

// AC-7: values the user's configuration writes apply as before, and the
// fallback rule of a written model is unchanged.
func TestSparAgentModelAC7_WrittenSettingsKept(t *testing.T) {
	w := newAMWorld(t, true)
	w.use(w.entry("claude", "claude", `model = "user-claude"`, `effort = "low"`, `timeout_seconds = 120`) +
		w.entry("codex", "codex", `model = "quota-codex"`, `effort = ""`, `fallback_model = "codex-fb"`) +
		w.entry("agy", "agy", `model = "quota-agy"`) +
		w.entry("agy2", "agy", `model = "quota-agy2"`, `fallback_model = ""`))
	out := w.round("ac7")
	if code, errs := w.run(out); code != ExitOK {
		t.Fatalf("AC-7: the round with written settings exits %d: %s", code, errs)
	}
	rec := w.record(out)
	for _, x := range []struct {
		name, preset string
		models       []string
		effort       string
		status       string
		fallback     bool
	}{
		{"claude", "claude", []string{"user-claude"}, "low", StatusCompleted, false},
		{"codex", "codex", []string{"quota-codex", "codex-fb"}, "", StatusCompleted, true},
		{"agy", "agy", []string{"quota-agy", "gemini-3.8-flash-high"}, "", StatusCompleted, true},
		{"agy2", "agy", []string{"quota-agy2"}, "", StatusUnavailable, false},
	} {
		calls := w.calls(x.name)
		var models []string
		for _, c := range calls {
			models = append(models, amModelArg(x.preset, c))
			if got := amEffortArg(x.preset, c); got != x.effort {
				t.Errorf("AC-7: %s was started with effort %q, want %q", x.name, got, x.effort)
			}
		}
		if !reflect.DeepEqual(models, x.models) {
			t.Errorf("AC-7: %s was started with the models %v, want %v", x.name, models, x.models)
		}
		st := amState(rec, x.name)
		if amStr(st, "status") != x.status || st["fallback_used"] != x.fallback {
			t.Errorf("AC-7: %s ends %q with fallback_used %v, want %q and %v", x.name, amStr(st, "status"), st["fallback_used"], x.status, x.fallback)
		}
	}
	if st := amState(rec, "claude"); st["timeout_seconds"] != float64(120) {
		t.Errorf("AC-7: claude's written timeout is not applied: %v", st["timeout_seconds"])
	}
}

// AC-8: the project file still sets no model, fallback model, effort or
// allowed list; gate, rerun, cap and waiver judgments do not depend on the
// model or its source; native and command reviewers run as before.
func TestSparAgentModelAC8_JudgmentUnchanged(t *testing.T) {
	w := newAMWorld(t, true)
	w.use(w.entry("codex", "codex") + amNative("host"))
	project := filepath.Join(w.repo, ".jaekit", "spar.toml")
	for _, body := range []struct{ key, text string }{
		{"model", "model = \"x\"\n"}, {"fallback_model", "fallback_model = \"x\"\n"}, {"effort", "effort = \"x\"\n"},
		{"allowed_models", "allowed_models = [\"x\"]\n"}, {"allowed_efforts", "allowed_efforts = [\"x\"]\n"},
		{"caps.allowed_models", "[caps]\nallowed_models = [\"x\"]\n"},
	} {
		w.write(project, body.text)
		code, _, errs := w.spar("config", "--repo", w.repo)
		if code != ExitRefused || !strings.Contains(errs, body.key) {
			t.Errorf("AC-8: a project file with %s gives exit %d: %s", body.key, code, errs)
		}
	}
	if err := os.Remove(project); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// The same series with and without the agent's model: same gate.
	answer, err := os.ReadFile(w.answer())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	reply := w.write(filepath.Join(w.dir, "host-reply.json"), string(answer))
	gateOf := func(rounds ...string) (int, map[string]any) {
		gate := filepath.Join(w.dir, fmt.Sprintf("gate-%d.json", len(rounds)))
		code, _, errs := w.spar(append(append([]string{"gate"}, rounds...), "--expect-head", w.head, "--out", gate, "--repo", w.repo)...)
		data, err := os.ReadFile(gate)
		var g map[string]any
		if err != nil || json.Unmarshal(data, &g) != nil {
			t.Fatalf("AC-8: no gate file: %s", errs)
		}
		return code, g
	}
	judgment := func(g map[string]any, round string) string {
		rounds, _ := g["rounds"].([]any)
		var parts []string
		for _, item := range rounds {
			r, _ := item.(map[string]any)
			parts = append(parts, fmt.Sprintf("%v/%v/%v", r["required_reviewers"], r["completed_required"], r["counted"]))
		}
		return strings.ReplaceAll(fmt.Sprintf("%v %v %v", g["state"], g["reasons"], parts), round, "<round>")
	}
	var results []string
	for _, choice := range [][]string{nil, {"--model", "codex=agent-codex"}} {
		out := w.round("ac8")
		if code, errs := w.run(out, choice...); code != ExitOK {
			t.Fatalf("AC-8: the round %v exits %d: %s", choice, code, errs)
		}
		if code, _, errs := w.spar("record", out, "host", "--result", reply); code != ExitOK {
			t.Fatalf("AC-8: recording host exits %d: %s", code, errs)
		}
		code, g := gateOf(out)
		results = append(results, fmt.Sprintf("%d %s", code, judgment(g, out)))
	}
	if results[0] != results[1] || !strings.HasPrefix(results[0], "0 complete") {
		t.Errorf("AC-8: the gate depends on the agent's model or is not complete: %q", results)
	}
	// A waiver covers a reviewer whose chosen model failed.
	failed := w.round("ac8-waive")
	if code, errs := w.run(failed, "--model", "codex=unknown-model"); code != ExitOK {
		t.Fatalf("AC-8: the round with a failing model exits %d: %s", code, errs)
	}
	if code, _, errs := w.spar("record", failed, "host", "--result", reply); code != ExitOK {
		t.Fatalf("AC-8: recording host exits %d: %s", code, errs)
	}
	if code, _ := gateOf(failed); code != ExitDegraded {
		t.Errorf("AC-8: the gate without a waiver exits %d, want degraded", code)
	}
	if code, _, errs := w.spar("waive", failed, "codex", "--user-quote", "go on without codex"); code != ExitOK {
		t.Fatalf("AC-8: spar waive exits %d: %s", code, errs)
	}
	if code, g := gateOf(failed); code != ExitOK {
		t.Errorf("AC-8: the gate with the waiver exits %d: %s", code, judgment(g, failed))
	}
	// The cap counts rounds with the agent's model as it counts others:
	// the fourth code round of a series is refused before any reviewer.
	series := filepath.Dir(w.round("ac8-cap"))
	base, head := w.base, w.head
	for i := 1; i <= 4; i++ {
		stage := "candidate"
		if i > 1 {
			stage, base = "fixes", head
			w.write(filepath.Join(w.repo, "app", "greet.py"), fmt.Sprintf("def greet(name):\n    return 'hello ' + name  # %d\n", i))
			w.git("commit", "-qam", fmt.Sprintf("fix %d", i))
			head = w.git("rev-parse", "HEAD")
		}
		out := filepath.Join(series, fmt.Sprintf("%02d-%s", i, stage))
		code, _, errs := w.spar("run", "--stage", stage, "--base", base, "--head", head, "--criteria", w.criteria,
			"--out", out, "--repo", w.repo, "--model", "codex=agent-codex")
		switch {
		case i < 4 && code != ExitOK && code != ExitNoResult:
			t.Errorf("AC-8: code round %d with the agent's model exits %d: %s", i, code, errs)
		case i == 4 && (code != ExitRefused || amExists(out)):
			t.Errorf("AC-8: the fourth code round with the agent's model exits %d (folder %v), want the cap's refusal: %s", code, amExists(out), errs)
		}
	}
	// Native and command reviewers record no model source.
	w.use(amNative("host") + amCommand(w, "cmd"))
	out := w.round("ac8-native")
	if code, errs := w.run(out); code != ExitOK && code != ExitNoResult {
		t.Fatalf("AC-8: the native and command round exits %d: %s", code, errs)
	}
	rec := w.record(out)
	for _, name := range []string{"host", "cmd"} {
		st := amState(rec, name)
		applied := amApplied(amConfig(rec), name)
		for _, key := range []string{"model_source", "effort_source", "default_model", "default_effort"} {
			if _, ok := st[key]; ok {
				t.Errorf("AC-8: %s's state has %s", name, key)
			}
			if _, ok := applied[key]; ok {
				t.Errorf("AC-8: %s's applied entry has %s", name, key)
			}
		}
		if st["requested_model"] != nil {
			t.Errorf("AC-8: %s requests a model: %v", name, st["requested_model"])
		}
	}
	if st := amState(rec, "cmd"); amStr(st, "status") != StatusCompleted {
		t.Errorf("AC-8: the command reviewer ends %q", amStr(st, "status"))
	}
}

// amOldForm removes from a round folder's records what this goal adds, so
// the round reads as one spar wrote before it.
func amOldForm(t *testing.T, round string) {
	t.Helper()
	path := filepath.Join(round, "round.json")
	data, err := os.ReadFile(path)
	var rec map[string]any
	if err != nil || json.Unmarshal(data, &rec) != nil {
		t.Fatalf("setup: %s: %v", path, err)
	}
	added := []string{"model_source", "effort_source", "default_model", "default_effort", "allowed_models", "allowed_efforts"}
	config := amConfig(rec)
	delete(config, "choices")
	list, _ := config["reviewers"].([]any)
	for _, item := range list {
		for _, k := range added {
			delete(item.(map[string]any), k)
		}
	}
	states, _ := rec["reviewers"].(map[string]any)
	for _, item := range states {
		for _, k := range added {
			delete(item.(map[string]any), k)
		}
	}
	out, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// amStripSources removes the source facts from summary lines: what a
// summary of the same round looked like before this goal.
func amStripSources(summary string) string {
	var lines []string
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, "- ") && strings.Contains(line, " (") {
			for _, s := range []string{" [user]", " [agent]", " [preset]"} {
				line = strings.ReplaceAll(line, s, "")
			}
			for {
				i := strings.Index(line, ", effort ")
				if i < 0 {
					break
				}
				j := strings.IndexAny(line[i+2:], ",)")
				line = line[:i] + line[i+2+j:]
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// AC-10 (spar): a round in the form spar wrote before this goal reads as it
// did: summary, triage, waive and gate give the results they give on the
// same round written now.
func TestSparAgentModelAC10_OldRoundsRead(t *testing.T) {
	w := newAMWorld(t, true)
	w.use(w.entry("codex", "codex") + amNative("host"))
	finding := map[string]any{"id": "F1", "proposed_class": "defect", "angle": "requirement-counterexample",
		"location": "head/app/greet.py:2", "claim": "synthetic claim", "failure_scenario": "greet('') returns 'hello, '",
		"evidence": "head/app/greet.py:2", "link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes",
		"regression": "no", "check_impact": "none", "confidence": "high"}
	checked := []any{}
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	reply, _ := json.Marshal(map[string]any{"reviewed_head": w.head, "verdict": "blocking_findings", "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": []any{finding}, "limits": "synthetic reviewer"})
	replyFile := w.write(filepath.Join(w.dir, "reply.json"), string(reply))
	newSeries := filepath.Dir(w.round("ac10-new"))
	round := filepath.Join(newSeries, "01-candidate")
	if code, errs := w.run(round, "--model", "codex=unknown-model", "--effort", "codex=low"); code != ExitOK {
		t.Fatalf("AC-10: the round with the agent's values exits %d: %s", code, errs)
	}
	oldSeries := filepath.Join(filepath.Dir(newSeries), "ac10-old")
	if err := os.CopyFS(oldSeries, os.DirFS(newSeries)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	oldRound := filepath.Join(oldSeries, "01-candidate")
	amOldForm(t, oldRound)
	// Each step's exit code follows the round contract: a reject without
	// evidence and a waiver for a reviewer that completed are refused.
	steps := []struct {
		args []string
		want int
	}{
		{[]string{"record", "<round>", "host", "--result", replyFile}, ExitOK},
		{[]string{"triage", "<round>", "host-F1", "reject"}, ExitRefused},
		{[]string{"triage", "<round>", "host-F1", "reject", "--evidence", "synthetic evidence"}, ExitOK},
		{[]string{"waive", "<round>", "host", "--user-quote", "go on"}, ExitRefused},
		{[]string{"waive", "<round>", "codex", "--user-quote", "go on without codex"}, ExitOK},
		{[]string{"summary", "<round>"}, ExitOK},
	}
	for _, x := range steps {
		step := x.args
		var codes [2]int
		var outs [2]string
		for i, r := range []string{round, oldRound} {
			args := append([]string(nil), step...)
			args[1] = r
			code, out, _ := w.spar(args...)
			codes[i], outs[i] = code, strings.ReplaceAll(out, r, "<round>")
		}
		if codes[0] != x.want || codes[1] != x.want {
			t.Errorf("AC-10: spar %v exits %d on the round written now and %d on the earlier form, want %d", step, codes[0], codes[1], x.want)
		}
		if step[0] == "summary" && amStripSources(outs[0]) != outs[1] {
			t.Errorf("AC-10: the earlier form's summary is not the summary without sources:\n%s\n---\n%s", outs[1], outs[0])
		}
		if step[0] == "summary" && strings.Contains(outs[1], "[agent]") {
			t.Errorf("AC-10: the earlier form's summary shows a source: %s", outs[1])
		}
	}
	var gates [2]string
	for i, r := range []string{round, oldRound} {
		gate := filepath.Join(w.dir, fmt.Sprintf("gate-ac10-%d.json", i))
		code, _, _ := w.spar("gate", r, "--expect-head", w.head, "--out", gate, "--repo", w.repo)
		data, err := os.ReadFile(gate)
		var g map[string]any
		if err != nil || json.Unmarshal(data, &g) != nil {
			t.Fatalf("AC-10: no gate file for %s", r)
		}
		gates[i] = strings.ReplaceAll(fmt.Sprintf("%d %v %v %v %v", code, g["state"], g["reasons"], g["open"], g["waived_reviewers"]), r, "<round>")
	}
	if gates[0] != gates[1] || !strings.HasPrefix(gates[1], "0 complete [] map[defect:0 ") || strings.Contains(gates[1], "question:1") {
		t.Errorf("AC-10: the gate of the round written now and of the earlier form, want both complete with nothing open:\n%s\n%s", gates[0], gates[1])
	}
}

// amNormalized is a call's arguments with what differs between runs
// replaced: absolute paths by their file name, the workspace folder and
// the packet and schema texts by a placeholder.
func amNormalized(preset string, args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case i > 0 && args[i-1] == "-C":
			a = "<workspace>"
		case i > 0 && preset == "agy" && args[i-1] == "-p":
			a = "<packet>"
		case i > 0 && preset == "claude" && args[i-1] == "--json-schema":
			a = "<schema>"
		case filepath.IsAbs(a):
			a = "<" + filepath.Base(a) + ">"
		}
		out[i] = a
	}
	return out
}

// The arguments, after the program name, the presets started their CLIs
// with before this goal (internal/spar/reviewers.go at main 4404bf4), with
// amNormalized's placeholders.
var (
	amClaudeArgs = []string{"-p", "--model", "opus", "--effort", "high", "--safe-mode", "--restricted",
		"--strict-mcp-config", "--tools", "Read,Grep,Glob", "--permission-mode", "dontAsk", "--no-session-persistence",
		"--disable-slash-commands", "--output-format", "stream-json", "--verbose", "--json-schema", "<schema>"}
	amCodexArgs = []string{"exec", "--ignore-user-config", "--ignore-rules", "-m", "gpt-6-astra",
		"-c", `model_reasoning_effort="high"`, "-c", `web_search="disabled"`, "--disable", "apps", "--disable", "browser_use",
		"--disable", "browser_use_external", "--disable", "computer_use", "--disable", "in_app_browser",
		"--disable", "image_generation", "--disable", "memories", "--disable", "multi_agent", "--disable", "plugins",
		"--disable", "hooks", "-s", "read-only", "--ephemeral", "--skip-git-repo-check", "-C", "<workspace>", "--json",
		"--output-schema", "<schema.json>", "-o", "<last_message.json>", "-"}
	amAgyArgs = func(model string) []string {
		return []string{"-p", "<packet>", "--model", model, "--sandbox", "--disable-slash-commands",
			"--output-format", "stream-json", "--json-schema", "<schema.json>", "--print-timeout", "1800s",
			"--log-file", "<agy-runtime.log>"}
	}
)

// AC-15: without the agent's values, a reviewer uses the preset's model,
// fallback model and effort: each CLI gets the arguments it got before this
// goal, the preset fallback included, with or without allowed lists.
func TestSparAgentModelAC15_NoChoiceKeepsPresets(t *testing.T) {
	w := newAMWorld(t, true)
	plain := "[[reviewers]]\npreset = \"claude\"\nbinary = %q\n%s\n[[reviewers]]\npreset = \"codex\"\nbinary = %q\n%s\n" +
		"[[reviewers]]\npreset = \"agy\"\nbinary = %q\n%s\n"
	claude, codex, agy := w.fake("claude", "claude"), w.fake("codex", "codex"), w.fake("agy", "agy")
	// A second agy CLI refuses the preset model for quota, so the preset
	// fallback runs.
	quota := w.write(filepath.Join(w.bin, "agy-quota"), "#!/bin/sh\nmodel=\nprev=\nfor a in \"$@\"; do case \"$prev\" in --model) model=$a;; esac; prev=$a; done\n"+
		"if [ \"$model\" = gemini-4-argon-high ]; then exec "+amQuote(agy)+" --model quota-preset; fi\nexec "+amQuote(agy)+" \"$@\"\n")
	if err := os.Chmod(quota, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, variant := range []struct{ label, claudeExtra, codexExtra, agyExtra string }{
		{"without allowed lists", "", "", ""},
		{"with allowed lists", `allowed_models = ["claude-a"]` + "\n" + `allowed_efforts = ["low"]`, `allowed_models = ["codex-a"]`, `allowed_models = ["agy-a"]`},
	} {
		w.use(fmt.Sprintf(plain, claude, variant.claudeExtra, codex, variant.codexExtra, agy, variant.agyExtra))
		w.clearCalls()
		out := w.round("ac15")
		if code, errs := w.run(out); code != ExitOK {
			t.Errorf("AC-15 %s: the round exits %d: %s", variant.label, code, errs)
			continue
		}
		for _, x := range []struct {
			preset string
			want   []string
		}{{"claude", amClaudeArgs}, {"codex", amCodexArgs}, {"agy", amAgyArgs("gemini-4-argon-high")}} {
			calls := w.calls(x.preset)
			if len(calls) != 1 || !reflect.DeepEqual(amNormalized(x.preset, calls[0]), x.want) {
				var got [][]string
				for _, c := range calls {
					got = append(got, amNormalized(x.preset, c))
				}
				t.Errorf("AC-15 %s: %s was started with %q, want %q", variant.label, x.preset, got, x.want)
			}
		}
	}
	w.use(fmt.Sprintf("[[reviewers]]\npreset = \"agy\"\nbinary = %q\n", quota))
	w.clearCalls()
	out := w.round("ac15-fallback")
	if code, errs := w.run(out); code != ExitOK {
		t.Fatalf("AC-15: the round with the quota refusal exits %d: %s", code, errs)
	}
	var models []string
	for _, c := range w.calls("agy") {
		models = append(models, amModelArg("agy", c))
	}
	// The wrapper turns the preset model into a quota refusal, then runs
	// the fallback model as it is.
	if want := []string{"quota-preset", "gemini-3.8-flash-high"}; !reflect.DeepEqual(models, want) {
		t.Errorf("AC-15: the preset fallback did not run once after the refusal: %v", models)
	}
	if st := amState(w.record(out), "agy"); st["fallback_used"] != true || amStr(st, "requested_model") != "gemini-4-argon-high" {
		t.Errorf("AC-15: agy records fallback_used %v and requested_model %q", st["fallback_used"], amStr(st, "requested_model"))
	}
}

// AC-12: an allowed list in the user's configuration limits the agent's
// choices to its exact entries, leaves a run without a choice on the
// preset's values, and is a configuration error in every listed form.
func TestSparAgentModelAC12_AllowedLists(t *testing.T) {
	const v = "allowed-list-not-applied"
	w := newAMWorld(t, false)
	w.use(w.entry("claude", "claude", `allowed_models = ["claude-a"]`, `allowed_efforts = ["low", "medium"]`) +
		w.entry("codex", "codex", `allowed_models = ["codex-a", "codex-b"]`) +
		w.entry("agy", "agy", `allowed_models = ["agy-a"]`))
	out := w.round("ac12")
	code, errs := w.run(out, "--model", "codex=codex-b", "--model", "claude=claude-a", "--model", "agy=agy-a", "--effort", "claude=medium")
	if code != ExitOK {
		t.Fatalf("jaekit:violation=%s choices inside the allowed lists exit %d: %s", v, code, errs)
	}
	for name, want := range map[string][2]string{"claude": {"claude-a", "medium"}, "codex": {"codex-b", "high"}, "agy": {"agy-a", ""}} {
		calls := w.calls(name)
		if len(calls) != 1 || amModelArg(name, calls[0]) != want[0] || amEffortArg(name, calls[0]) != want[1] {
			t.Errorf("jaekit:violation=%s %s was not started with model %q and effort %q: %v", v, name, want[0], want[1], calls)
		}
	}
	for _, arg := range []string{"codex=codex-c", "codex=Codex-a", "claude=claude-b"} {
		code, _, errs := w.spar("config", "--repo", w.repo, "--model", arg)
		name, value, _ := strings.Cut(arg, "=")
		if code != ExitRefused || !strings.Contains(errs, strconv.Quote(name)) || !strings.Contains(errs, strconv.Quote(value)) {
			t.Errorf("jaekit:violation=%s --model %s outside the allowed list exits %d: %s", v, arg, code, errs)
		}
	}
	if code, _, errs := w.spar("config", "--repo", w.repo, "--effort", "claude=high"); code != ExitRefused ||
		!strings.Contains(errs, strconv.Quote("high")) {
		t.Errorf("jaekit:violation=%s --effort claude=high outside the allowed list exits %d: %s", v, code, errs)
	}
	w.clearCalls()
	none := w.round("ac12-none")
	if code, errs := w.run(none); code != ExitOK {
		t.Fatalf("jaekit:violation=%s the round without choices exits %d: %s", v, code, errs)
	}
	for name, want := range map[string][2]string{"claude": {"opus", "high"}, "codex": {"gpt-6-astra", "high"}, "agy": {"gemini-4-argon-high", ""}} {
		calls := w.calls(name)
		if len(calls) != 1 || amModelArg(name, calls[0]) != want[0] || amEffortArg(name, calls[0]) != want[1] {
			t.Errorf("jaekit:violation=%s without a choice %s was not started with the preset model %q and effort %q: %v", v, name, want[0], want[1], calls)
		}
	}
	bin := w.fake("errors", "claude")
	errorsCases := []struct {
		label, preset, lines, key string
	}{
		{"an empty model list", "claude", `allowed_models = []`, "allowed_models"},
		{"an empty effort list", "codex", `allowed_efforts = []`, "allowed_efforts"},
		{"a number in a model list", "claude", `allowed_models = [1]`, "allowed_models"},
		{"a table in an effort list", "codex", `allowed_efforts = [{ a = "b" }]`, "allowed_efforts"},
		{"an empty entry", "agy", `allowed_models = [""]`, "allowed_models"},
		{"an entry starting with -", "agy", `allowed_models = ["-x"]`, "allowed_models"},
		{"an entry with a space", "codex", `allowed_models = ["a b"]`, "allowed_models"},
		{"an entry with a control character", "claude", `allowed_efforts = ["a\u0001b"]`, "allowed_efforts"},
		{"an entry that is not UTF-8", "codex", "allowed_models = [\"a\xffb\"]", "allowed_models"},
		{"a model and a model list", "claude", "model = \"m\"\nallowed_models = [\"m\"]", "allowed_models"},
		{"an effort and an effort list", "codex", "effort = \"low\"\nallowed_efforts = [\"low\"]", "allowed_efforts"},
		{"a model list on native", "native", `allowed_models = ["m"]`, "allowed_models"},
		{"a model list on command", "command", "argv = [\"/bin/cat\"]\nallowed_models = [\"m\"]", "allowed_models"},
		{"an effort list on agy", "agy", `allowed_efforts = ["low"]`, "allowed_efforts"},
		{"an effort list on native", "native", `allowed_efforts = ["low"]`, "allowed_efforts"},
		{"an effort list on command", "command", "argv = [\"/bin/cat\"]\nallowed_efforts = [\"low\"]", "allowed_efforts"},
	}
	for _, c := range errorsCases {
		binary := ""
		if c.preset == "claude" || c.preset == "codex" || c.preset == "agy" {
			binary = fmt.Sprintf("binary = %q\n", bin)
		}
		body := fmt.Sprintf("timeout_seconds = 60\n\n[[reviewers]]\nname = \"r\"\npreset = %q\n%s%s\n", c.preset, binary, c.lines)
		path := w.use(body)
		line := 0
		for i, l := range strings.Split(body, "\n") {
			if strings.HasPrefix(l, c.key+" =") {
				line = i + 1
			}
		}
		code, _, errs := w.spar("config", "--repo", w.repo)
		if code != ExitRefused || !strings.Contains(errs, fmt.Sprintf("%s:%d", path, line)) {
			t.Errorf("jaekit:violation=%s %s: spar config exits %d without naming %s:%d: %s", v, c.label, code, path, line, errs)
		}
	}
}

// AC-13: a reviewer's effort is the written one, else the agent's, else the
// preset's; the agent may choose the model and the effort apart.
func TestSparAgentModelAC13_AgentEffort(t *testing.T) {
	const v = "agent-effort-not-applied"
	w := newAMWorld(t, false)
	w.use(w.entry("claude", "claude") + w.entry("codex", "codex"))
	for _, x := range []struct {
		label  string
		args   []string
		claude [3]string // model, effort, effort source
		codex  [3]string
	}{
		{"efforts only", []string{"--effort", "claude=medium", "--effort", "codex=low"},
			[3]string{"opus", "medium", "agent"}, [3]string{"gpt-6-astra", "low", "agent"}},
		{"a model only", []string{"--model", "claude=claude-x"},
			[3]string{"claude-x", "high", "preset"}, [3]string{"gpt-6-astra", "high", "preset"}},
		{"both", []string{"--model", "codex=codex-x", "--effort", "codex=medium"},
			[3]string{"opus", "high", "preset"}, [3]string{"codex-x", "medium", "agent"}},
	} {
		w.clearCalls()
		out := w.round("ac13")
		code, errs := w.run(out, x.args...)
		if code != ExitOK {
			t.Errorf("jaekit:violation=%s %s: the round exits %d: %s", v, x.label, code, errs)
			continue
		}
		rec := w.record(out)
		for name, want := range map[string][3]string{"claude": x.claude, "codex": x.codex} {
			calls := w.calls(name)
			if len(calls) != 1 || amModelArg(name, calls[0]) != want[0] || amEffortArg(name, calls[0]) != want[1] {
				t.Errorf("jaekit:violation=%s %s: %s was not started with model %q and effort %q: %v", v, x.label, name, want[0], want[1], calls)
			}
			st := amState(rec, name)
			if amStr(st, "effort") != want[1] || amStr(st, "effort_source") != want[2] {
				t.Errorf("jaekit:violation=%s %s: %s records effort %q from %q, want %q from %q", v, x.label, name,
					amStr(st, "effort"), amStr(st, "effort_source"), want[1], want[2])
			}
		}
	}
}

// AC-14: a reviewer whose model the agent chose falls back only to the
// fallback model the user wrote, once, with the same effort, and only when
// it differs from the chosen model.
func TestSparAgentModelAC14_ChosenModelFallback(t *testing.T) {
	const v = "chosen-model-fallback-wrong"
	w := newAMWorld(t, false)
	w.use(w.entry("agy", "agy") + w.entry("agy2", "agy", `fallback_model = "agy-fb"`) + w.entry("agy3", "agy", `fallback_model = ""`) +
		w.entry("codex", "codex", `fallback_model = "codex-fb"`, `effort = "low"`) + w.entry("claude", "claude", `fallback_model = "quota-same"`))
	out := w.round("ac14")
	code, errs := w.run(out, "--model", "agy=quota-agy", "--model", "agy2=quota-agy2", "--model", "agy3=quota-agy3",
		"--model", "codex=quota-codex", "--model", "claude=quota-same")
	if code != ExitOK {
		t.Fatalf("jaekit:violation=%s the round exits %d: %s", v, code, errs)
	}
	rec := w.record(out)
	for _, x := range []struct {
		name, preset string
		models       []string
		effort       string
		status       string
		fallback     bool
	}{
		{"agy", "agy", []string{"quota-agy"}, "", StatusUnavailable, false},
		{"agy2", "agy", []string{"quota-agy2", "agy-fb"}, "", StatusCompleted, true},
		{"agy3", "agy", []string{"quota-agy3"}, "", StatusUnavailable, false},
		{"codex", "codex", []string{"quota-codex", "codex-fb"}, "low", StatusCompleted, true},
		{"claude", "claude", []string{"quota-same"}, "high", StatusUnavailable, false},
	} {
		var models []string
		for _, c := range w.calls(x.name) {
			models = append(models, amModelArg(x.preset, c))
			if got := amEffortArg(x.preset, c); got != x.effort {
				t.Errorf("jaekit:violation=%s %s was started with effort %q, want %q", v, x.name, got, x.effort)
			}
		}
		if !reflect.DeepEqual(models, x.models) {
			t.Errorf("jaekit:violation=%s %s was started with the models %v, want %v", v, x.name, models, x.models)
		}
		st := amState(rec, x.name)
		if amStr(st, "status") != x.status || st["fallback_used"] != x.fallback {
			t.Errorf("jaekit:violation=%s %s ends %q with fallback_used %v, want %q and %v", v, x.name, amStr(st, "status"), st["fallback_used"], x.status, x.fallback)
		}
	}
}
