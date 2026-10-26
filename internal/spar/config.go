// Package spar runs independent review rounds on pinned commits and keeps
// the records a gate decides from. It is the `spar` executable's library;
// Seal Core (`ha` and its packages) never imports it.
package spar

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	toml "github.com/pelletier/go-toml/v2"
)

// Isolation grades a reviewer's protections are recorded with.
const (
	IsolationVerified   = "verified"   // spar starts the CLI with read-only, offline protections it checks
	IsolationHost       = "host"       // the host agent's clean-context subagent; the host enforces its tools
	IsolationUnverified = "unverified" // an arbitrary command; nothing is enforced
)

// DefaultTimeout is a reviewer's time limit in seconds when none is configured.
const DefaultTimeout = 1800

// Angles are the attack angles every packet asks about, in order.
var Angles = []string{"requirement-counterexample", "side-effects", "design", "premises", "check-validity"}

// Reviewer is one reviewer as a round applies it.
type Reviewer struct {
	Name      string   `json:"name"`
	Preset    string   `json:"preset"`
	Isolation string   `json:"isolation"`
	Required  bool     `json:"required"`
	Argv      []string `json:"argv,omitempty"`
	Binary    string   `json:"binary,omitempty"`
	Model     string   `json:"model,omitempty"`
	// ModelSource says where Model came from: the user's file, the agent's
	// --model for this run or the preset (claude, codex and agy only).
	ModelSource string `json:"model_source,omitempty"`
	// DefaultModel is the model used when the agent chooses none; it is
	// set only when the agent may choose.
	DefaultModel   string   `json:"default_model,omitempty"`
	AllowedModels  []string `json:"allowed_models,omitempty"`
	FallbackModel  string   `json:"fallback_model,omitempty"`
	Effort         string   `json:"effort,omitempty"`
	EffortSource   string   `json:"effort_source,omitempty"`
	DefaultEffort  string   `json:"default_effort,omitempty"`
	AllowedEfforts []string `json:"allowed_efforts,omitempty"`
	TimeoutSeconds int64    `json:"timeout_seconds"`
	// The keys the user's file writes for this reviewer.
	modelWritten, effortWritten, fallbackWritten bool
}

// Where a reviewer's model or effort came from.
const (
	SourceUser   = "user"
	SourceAgent  = "agent"
	SourcePreset = "preset"
)

// Choice is a value the agent passed for one run: --model NAME=MODEL or
// --effort NAME=EFFORT.
type Choice struct {
	Reviewer string `json:"reviewer"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

// Caps are the stage limits: plan rounds, plan-delta rounds and code rounds.
type Caps struct {
	Plan      int64 `json:"plan"`
	PlanDelta int64 `json:"plan_delta"`
	Code      int64 `json:"code"`
}

// Source says where the reviewer list came from.
type Source struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// Override records a one-run reviewer list given on the command line.
type Override struct {
	Kind  string   `json:"kind"`
	Names []string `json:"names"`
}

// FileDigest identifies a configuration file that was read.
type FileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// RequiredRef names a reviewer the configuration makes required.
type RequiredRef struct {
	Name   string `json:"name"`
	Preset string `json:"preset"`
}

// Config is the applied configuration: reviewers from the user's file and
// limits from the project's file.
type Config struct {
	Source   Source    `json:"source"`
	Override *Override `json:"override"`
	// Choices are the agent's values for this run, an empty list for none.
	// A record written before they existed has none and keeps none.
	Choices   *[]Choice    `json:"choices,omitempty"`
	Files     []FileDigest `json:"files"`
	Reviewers []Reviewer   `json:"reviewers"`
	// ConfiguredRequired are the required reviewers of the configuration
	// itself: a --reviewers list never removes them from a gate.
	ConfiguredRequired []RequiredRef `json:"configured_required"`
	Angles             []string      `json:"angles"`
	Caps               Caps          `json:"caps"`
	CheckPaths         []string      `json:"check_paths"`
	Rules              []string      `json:"rules"`
	Digest             string        `json:"digest"`
	// defaultTimeout is the time limit of reviewers the source file does
	// not set one for: its top-level timeout_seconds (G19).
	defaultTimeout int64
}

// presets are the reviewer kinds and their defaults.
var presets = map[string]Reviewer{
	"claude":  {Preset: "claude", Isolation: IsolationVerified, Required: true, Model: "opus", Effort: "high"},
	"codex":   {Preset: "codex", Isolation: IsolationVerified, Required: true, Model: "gpt-6-astra", Effort: "high"},
	"agy":     {Preset: "agy", Isolation: IsolationVerified, Required: true, Model: "gemini-4-argon-high", FallbackModel: "gemini-3.8-flash-high"},
	"native":  {Preset: "native", Isolation: IsolationHost, Required: true},
	"command": {Preset: "command", Isolation: IsolationUnverified, Required: false},
}

// DefaultReviewers is the list without any configuration.
var DefaultReviewers = []string{"claude", "codex", "agy"}

// Keys a project file may never set: they choose what runs on the user's
// computer and what it costs.
var forbiddenProjectKeys = map[string]bool{
	"reviewers": true, "preset": true, "command": true, "argv": true, "binary": true,
	"model": true, "fallback_model": true, "effort": true, "allowed_models": true, "allowed_efforts": true,
}

// modelPresets take a model, effortPresets an effort.
var (
	modelPresets  = map[string]bool{"claude": true, "codex": true, "agy": true}
	effortPresets = map[string]bool{"claude": true, "codex": true}
)

// roundFiles are the names spar writes in a round folder next to the
// reviewers' folders.
var roundFiles = []string{"round.json", "packet.md", "schema.json", "findings.json", "ledger.jsonl", "summary.md",
	waiversFile, dryRunWorkspace}

const dryRunWorkspace = "workspace"

// reservedName reports a reviewer name that would be a round file's
// folder, or the temporary file spar writes it through, ignoring case for
// case-insensitive file systems (G20).
func reservedName(name string) bool {
	for _, f := range roundFiles {
		if strings.EqualFold(name, f) || strings.EqualFold(name, f+".tmp") {
			return true
		}
	}
	return false
}

var reviewerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ConfigError is a configuration problem with its file and location.
type ConfigError struct {
	Path   string
	Line   int
	Column int
	Msg    string
}

func (e *ConfigError) Error() string {
	switch {
	case e.Line > 0 && e.Column > 0:
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Column, e.Msg)
	case e.Line > 0:
		return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Msg)
	default:
		return fmt.Sprintf("%s: %s", e.Path, e.Msg)
	}
}

type reviewerEntry struct {
	Name          *string  `toml:"name"`
	Preset        *string  `toml:"preset"`
	Argv          []string `toml:"argv"`
	Binary        *string  `toml:"binary"`
	Model         *string  `toml:"model"`
	FallbackModel *string  `toml:"fallback_model"`
	Effort        *string  `toml:"effort"`
	// The allowed lists are read as any value so that a wrong item is
	// reported with its key's line (G21).
	AllowedModels  any    `toml:"allowed_models"`
	AllowedEfforts any    `toml:"allowed_efforts"`
	TimeoutSeconds *int64 `toml:"timeout_seconds"`
}

type userFile struct {
	Reviewers      []reviewerEntry `toml:"reviewers"`
	TimeoutSeconds *int64          `toml:"timeout_seconds"`
}

type capsEntry struct {
	Plan      *int64 `toml:"plan"`
	PlanDelta *int64 `toml:"plan_delta"`
	Code      *int64 `toml:"code"`
}

type projectFile struct {
	Caps       *capsEntry `toml:"caps"`
	Angles     []string   `toml:"angles"`
	Rules      []string   `toml:"rules"`
	CheckPaths []string   `toml:"check_paths"`
}

// LoadConfig resolves the configuration for a repository. getenv supplies
// the environment; override, when not nil, replaces the reviewer list for
// one run (an empty override means the native reviewer).
func LoadConfig(repo string, getenv func(string) string, override []string) (*Config, error) {
	return LoadConfigWith(repo, getenv, override, nil)
}

// LoadConfigWith is LoadConfig with the agent's values for one run (G25): a
// reviewer whose model or effort the user's file does not write takes the
// agent's value, else the preset's.
func LoadConfigWith(repo string, getenv func(string) string, override []string, choices []Choice) (*Config, error) {
	c := &Config{Angles: append([]string(nil), Angles...), Caps: Caps{Plan: 1, PlanDelta: 1, Code: 3},
		CheckPaths: []string{}, Rules: []string{}, Files: []FileDigest{}, defaultTimeout: DefaultTimeout}
	configured, err := c.loadReviewers(getenv)
	if err != nil {
		return nil, err
	}
	c.ConfiguredRequired = []RequiredRef{}
	for _, r := range configured {
		if r.Required {
			c.ConfiguredRequired = append(c.ConfiguredRequired, RequiredRef{r.Name, r.Preset})
		}
	}
	if override != nil {
		c.Override = &Override{Kind: "--reviewers", Names: append([]string{}, override...)}
		if len(override) == 0 {
			c.Reviewers = []Reviewer{c.preset("native", "native")}
		} else {
			byName := map[string]Reviewer{}
			for _, r := range configured {
				byName[r.Name] = r
			}
			seen := map[string]bool{}
			for _, name := range override {
				// Names differing only in case share a folder on a
				// case-insensitive file system (G20).
				if seen[strings.ToLower(name)] {
					return nil, &ConfigError{Path: "--reviewers", Msg: fmt.Sprintf("reviewer %q is listed twice (names are compared ignoring case)", name)}
				}
				seen[strings.ToLower(name)] = true
				if r, ok := byName[name]; ok {
					c.Reviewers = append(c.Reviewers, r)
				} else if _, ok := presets[name]; ok && name != "command" {
					c.Reviewers = append(c.Reviewers, c.preset(name, name))
				} else {
					return nil, &ConfigError{Path: "--reviewers", Msg: fmt.Sprintf("reviewer %q is neither configured nor a preset (claude, codex, agy, native)", name)}
				}
			}
		}
	} else {
		c.Reviewers = configured
	}
	if err := c.loadProject(repo); err != nil {
		return nil, err
	}
	c.setSources()
	if err := c.applyChoices(choices); err != nil {
		return nil, err
	}
	c.Digest = c.digest()
	return c, nil
}

// setSources names where each reviewer's model and effort came from before
// the agent's values: the user's file or the preset.
func (c *Config) setSources() {
	for i := range c.Reviewers {
		r := &c.Reviewers[i]
		if modelPresets[r.Preset] {
			if r.modelWritten {
				r.ModelSource = SourceUser
			} else {
				r.ModelSource, r.DefaultModel = SourcePreset, r.Model
			}
		}
		if effortPresets[r.Preset] {
			if r.effortWritten {
				r.EffortSource = SourceUser
			} else {
				r.EffortSource, r.DefaultEffort = SourcePreset, r.Effort
			}
		}
	}
}

// choiceProblem says why the agent may not pass a value (G25): an empty
// value, one that is not valid UTF-8 and so cannot be recorded as it is,
// one a CLI could read as an option, or one with a space or control
// character. "" means the value passes to the CLI as it is.
func choiceProblem(v string) string {
	if v == "" {
		return "the value is empty"
	}
	if !utf8.ValidString(v) {
		return "the value is not valid UTF-8, so the record could not keep it as it is"
	}
	if strings.HasPrefix(v, "-") {
		return "the value starts with '-', which a CLI could read as an option"
	}
	for _, r := range v {
		switch {
		case unicode.IsSpace(r):
			return "the value contains a space character"
		case unicode.IsControl(r):
			return "the value contains a control character"
		}
	}
	return ""
}

// applyChoices applies the agent's values for this run (G25). A value for
// a reviewer whose key the user's file writes, a reviewer without that key,
// a name outside this run, a second value for the same key, a value outside
// the reviewer's allowed list or a value of the wrong form is refused. A
// chosen model falls back only to the fallback_model the user wrote.
func (c *Config) applyChoices(choices []Choice) error {
	applied := []Choice{}
	seen := map[string]bool{}
	for _, ch := range choices {
		fail := func(format string, args ...any) error {
			return &ConfigError{Path: "--" + ch.Key, Msg: fmt.Sprintf("%q=%q: ", ch.Reviewer, ch.Value) + fmt.Sprintf(format, args...)}
		}
		i := -1
		for j := range c.Reviewers {
			if c.Reviewers[j].Name == ch.Reviewer {
				i = j
			}
		}
		if i < 0 {
			names := make([]string, 0, len(c.Reviewers))
			for _, r := range c.Reviewers {
				names = append(names, r.Name)
			}
			return fail("%q is not a reviewer of this run (%s)", ch.Reviewer, strings.Join(names, ", "))
		}
		r := &c.Reviewers[i]
		model := ch.Key == "model"
		switch {
		case model && !modelPresets[r.Preset]:
			return fail("reviewer %q (preset %s) has no model to choose", r.Name, r.Preset)
		case !model && !effortPresets[r.Preset]:
			return fail("reviewer %q (preset %s) takes no effort", r.Name, r.Preset)
		case seen[ch.Key+"\x00"+r.Name]:
			return fail("reviewer %q already has a %s from this run", r.Name, ch.Key)
		case model && r.modelWritten:
			return fail("reviewer %q has the model %q in the user's configuration (%s), so the agent cannot choose it", r.Name, r.Model, c.Source.Path)
		case !model && r.effortWritten:
			return fail("reviewer %q has the effort %q in the user's configuration (%s), so the agent cannot choose it", r.Name, r.Effort, c.Source.Path)
		}
		seen[ch.Key+"\x00"+r.Name] = true
		if why := choiceProblem(ch.Value); why != "" {
			return fail("%s", why)
		}
		allowed, key := r.AllowedEfforts, "allowed_efforts"
		if model {
			allowed, key = r.AllowedModels, "allowed_models"
		}
		if len(allowed) > 0 && !contains(allowed, ch.Value) {
			return fail("%q is not in %s of reviewer %q (%s)", ch.Value, key, r.Name, strings.Join(allowed, ", "))
		}
		if model {
			r.Model, r.ModelSource = ch.Value, SourceAgent
			if !r.fallbackWritten {
				// The preset's fallback pairs with the preset's model.
				r.FallbackModel = ""
			}
		} else {
			r.Effort, r.EffortSource = ch.Value, SourceAgent
		}
		applied = append(applied, ch)
	}
	c.Choices = &applied
	return nil
}

func preset(name, kind string) Reviewer {
	r := presets[kind]
	r.Name = name
	r.TimeoutSeconds = DefaultTimeout
	return r
}

// preset is a preset reviewer with the time limit of the source file.
func (c *Config) preset(name, kind string) Reviewer {
	r := preset(name, kind)
	r.TimeoutSeconds = c.defaultTimeout
	return r
}

func fileDigest(path string, data []byte) FileDigest {
	sum := sha256.Sum256(data)
	return FileDigest{Path: path, SHA256: hex.EncodeToString(sum[:])}
}

// sourceFile says whether a configuration source exists (G16). A path that
// does not exist, a broken symbolic link included, is absent; one that
// exists must be a regular file, after following symbolic links.
func sourceFile(path string) (bool, error) {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.ELOOP):
		// A symbolic link loop is a broken link too.
		return false, nil
	case err != nil:
		return false, &ConfigError{Path: path, Msg: "the configuration file cannot be read: " + err.Error()}
	case !info.Mode().IsRegular():
		return false, &ConfigError{Path: path, Msg: "the configuration path exists but is not a regular file"}
	}
	return true, nil
}

func (c *Config) loadReviewers(getenv func(string) string) ([]Reviewer, error) {
	home := getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	type candidate struct{ kind, path string }
	var candidates []candidate
	if v := getenv("SPAR_CONFIG"); v != "" {
		candidates = append(candidates, candidate{"SPAR_CONFIG", v})
	}
	if v := getenv("XDG_CONFIG_HOME"); v != "" {
		candidates = append(candidates, candidate{"XDG_CONFIG_HOME", filepath.Join(v, "jaekit", "spar.toml")})
	}
	if home != "" {
		candidates = append(candidates, candidate{"HOME", filepath.Join(home, ".config", "jaekit", "spar.toml")})
	}
	var skipped []string
	for _, cand := range candidates {
		exists, err := sourceFile(cand.path)
		if err != nil {
			return nil, err
		}
		if !exists {
			skipped = append(skipped, cand.path)
			continue
		}
		data, err := os.ReadFile(cand.path)
		if err != nil {
			return nil, &ConfigError{Path: cand.path, Msg: err.Error()}
		}
		c.Files = append(c.Files, fileDigest(cand.path, data))
		c.Source = Source{Kind: cand.kind, Path: cand.path, Skipped: skipped}
		reviewers, present, timeout, err := parseUserFile(cand.path, data)
		if err != nil {
			return nil, err
		}
		// The top-level timeout_seconds is the default of every reviewer
		// this file decides, the default list and native included (G19).
		c.defaultTimeout = timeout
		if !present {
			c.Source.Note = "no reviewers key: default list"
			return c.defaults(DefaultReviewers), nil
		}
		if len(reviewers) == 0 {
			c.Source.Note = "empty list: native"
			return []Reviewer{c.preset("native", "native")}, nil
		}
		return reviewers, nil
	}
	clean := getenv("CLEAN_REVIEW_CONFIG")
	if clean == "" {
		base := getenv("XDG_CONFIG_HOME")
		if base == "" && home != "" {
			base = filepath.Join(home, ".config")
		}
		if base != "" {
			clean = filepath.Join(base, "clean-review", "config.json")
		}
	}
	cleanExists := false
	if clean != "" {
		exists, err := sourceFile(clean)
		if err != nil {
			return nil, err
		}
		cleanExists = exists
	}
	if cleanExists {
		data, err := os.ReadFile(clean)
		if err != nil {
			return nil, &ConfigError{Path: clean, Msg: err.Error()}
		}
		c.Files = append(c.Files, fileDigest(clean, data))
		c.Source = Source{Kind: "clean-review", Path: clean, Skipped: skipped}
		names, present, err := parseCleanReview(clean, data)
		if err != nil {
			return nil, err
		}
		if !present {
			c.Source.Note = "no reviewers key: default list"
			return c.defaults(DefaultReviewers), nil
		}
		if len(names) == 0 {
			c.Source.Note = "empty list: native"
			return []Reviewer{c.preset("native", "native")}, nil
		}
		return c.defaults(names), nil
	}
	if clean != "" {
		skipped = append(skipped, clean)
	}
	c.Source = Source{Kind: "default", Skipped: skipped}
	return c.defaults(DefaultReviewers), nil
}

func (c *Config) defaults(names []string) []Reviewer {
	out := make([]Reviewer, 0, len(names))
	for _, n := range names {
		out = append(out, c.preset(n, n))
	}
	return out
}

func parseCleanReview(path string, data []byte) ([]string, bool, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		line := 0
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line = bytes.Count(data[:min(int(syntax.Offset), len(data))], []byte("\n")) + 1
		}
		return nil, false, &ConfigError{Path: path, Line: line, Msg: "not a JSON object: " + err.Error()}
	}
	if doc == nil {
		return nil, false, &ConfigError{Path: path, Line: 1, Msg: "not a JSON object: null"}
	}
	raw, ok := doc["reviewers"]
	if !ok {
		return nil, false, nil
	}
	// As clean-review reads it (G18): a list whose items are all strings;
	// null, another type or a non-string item is an error.
	var items []json.RawMessage
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &items) != nil {
		return nil, true, &ConfigError{Path: path, Msg: `"reviewers" must be a list of names`}
	}
	seen := map[string]bool{}
	var out []string
	for i, item := range items {
		var n string
		if trimmed := bytes.TrimSpace(item); len(trimmed) == 0 || trimmed[0] != '"' || json.Unmarshal(trimmed, &n) != nil {
			return nil, true, &ConfigError{Path: path, Msg: fmt.Sprintf(`"reviewers" item %d is not a name: %s`, i, short(string(item), 40))}
		}
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, known := presets[n]; !known || n == "command" {
			return nil, true, &ConfigError{Path: path, Msg: fmt.Sprintf("unknown reviewer %q (known: claude, codex, agy, native)", n)}
		}
		if seen[n] {
			return nil, true, &ConfigError{Path: path, Msg: fmt.Sprintf("reviewer %q is listed twice", n)}
		}
		seen[n] = true
		out = append(out, n)
	}
	return out, true, nil
}

// decodeStrict parses a TOML document: syntax errors and keys the target
// does not know are ConfigErrors with their location.
func decodeStrict(path string, data []byte, raw *map[string]any, target any) error {
	if err := toml.Unmarshal(data, raw); err != nil {
		return tomlError(path, data, err)
	}
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(target); err != nil {
		return tomlError(path, data, err)
	}
	return nil
}

var tableExists = regexp.MustCompile(`table (\S+) already exists`)

func tomlError(path string, data []byte, err error) error {
	var decode *toml.DecodeError
	var strict *toml.StrictMissingError
	switch {
	case errors.As(err, &strict) && len(strict.Errors) > 0:
		e := strict.Errors[0]
		row, col := e.Position()
		return &ConfigError{Path: path, Line: row, Column: col, Msg: fmt.Sprintf("unknown key %q", strings.Join(e.Key(), "."))}
	case errors.As(err, &decode):
		row, col := decode.Position()
		msg := strings.TrimPrefix(decode.Error(), "toml: ")
		if key := decode.Key(); len(key) > 0 {
			msg = fmt.Sprintf("%s (key %q)", msg, strings.Join(key, "."))
		}
		return &ConfigError{Path: path, Line: row, Column: col, Msg: "TOML syntax error: " + msg}
	}
	msg := strings.TrimPrefix(err.Error(), "toml: ")
	line := 0
	if m := tableExists.FindStringSubmatch(msg); m != nil {
		line = tableHeaderLine(data, m[1])
	}
	return &ConfigError{Path: path, Line: line, Msg: "TOML syntax error: " + msg}
}

func parseUserFile(path string, data []byte) ([]Reviewer, bool, int64, error) {
	var raw map[string]any
	var file userFile
	if err := decodeStrict(path, data, &raw, &file); err != nil {
		return nil, false, 0, err
	}
	lines := keyLines(data)
	timeout := int64(DefaultTimeout)
	if file.TimeoutSeconds != nil {
		if *file.TimeoutSeconds <= 0 {
			return nil, false, 0, &ConfigError{Path: path, Line: lines["timeout_seconds"], Msg: "timeout_seconds must be positive"}
		}
		timeout = *file.TimeoutSeconds
	}
	reviewers, present, err := parseReviewers(path, raw, file, lines, timeout)
	return reviewers, present, timeout, err
}

func parseReviewers(path string, raw map[string]any, file userFile, lines map[string]int, timeout int64) ([]Reviewer, bool, error) {
	at := func(key string, i int, field string) int {
		if field != "" {
			if l := lines[fmt.Sprintf("%s[%d].%s", key, i, field)]; l > 0 {
				return l
			}
		}
		return lines[fmt.Sprintf("%s[%d]", key, i)]
	}
	_, present := raw["reviewers"]
	seen := map[string]int{}
	var out []Reviewer
	for i, e := range file.Reviewers {
		fail := func(field, format string, args ...any) error {
			return &ConfigError{Path: path, Line: at("reviewers", i, field), Msg: fmt.Sprintf("reviewers[%d]: ", i) + fmt.Sprintf(format, args...)}
		}
		if e.Preset == nil {
			return nil, true, fail("", "needs preset (claude, codex, agy, native or command)")
		}
		kind := *e.Preset
		base, known := presets[kind]
		if !known {
			return nil, true, fail("preset", "unknown preset %q (claude, codex, agy, native or command)", kind)
		}
		r := base
		r.TimeoutSeconds = timeout
		r.Name = kind
		if e.Name != nil {
			r.Name = *e.Name
		}
		if !reviewerName.MatchString(r.Name) {
			return nil, true, fail("name", "name %q must be letters, digits, '.', '_' or '-'", r.Name)
		}
		if reservedName(r.Name) {
			return nil, true, fail("name", "name %q is a file of the round folder", r.Name)
		}
		// Names differing only in case share a folder on a case-insensitive
		// file system (G20).
		if j, dup := seen[strings.ToLower(r.Name)]; dup {
			return nil, true, fail("name", "duplicate reviewer name %q (also reviewers[%d]; names are compared ignoring case)", r.Name, j)
		}
		seen[strings.ToLower(r.Name)] = i
		if kind == "command" {
			if len(e.Argv) == 0 {
				return nil, true, fail("", "preset command needs argv")
			}
			for _, a := range e.Argv {
				if a == "" {
					return nil, true, fail("argv", "argv has an empty argument")
				}
			}
			r.Argv = append([]string(nil), e.Argv...)
		} else if len(e.Argv) > 0 {
			return nil, true, fail("argv", "argv is only for preset command")
		}
		usable := map[string]bool{"claude": true, "codex": true, "agy": true}[kind]
		for field, value := range map[string]*string{"binary": e.Binary, "model": e.Model, "fallback_model": e.FallbackModel, "effort": e.Effort} {
			if value != nil && !usable {
				return nil, true, fail(field, "%s is not used by preset %s", field, kind)
			}
		}
		if e.Effort != nil && kind == "agy" {
			return nil, true, fail("effort", "effort is not used by preset agy")
		}
		if e.Binary != nil {
			r.Binary = *e.Binary
		}
		if e.Model != nil {
			if *e.Model == "" {
				return nil, true, fail("model", "model is empty")
			}
			r.Model = *e.Model
		}
		if e.FallbackModel != nil {
			r.FallbackModel = *e.FallbackModel // "" turns the preset's default off
		}
		if e.Effort != nil {
			r.Effort = *e.Effort
		}
		r.modelWritten, r.effortWritten, r.fallbackWritten = e.Model != nil, e.Effort != nil, e.FallbackModel != nil
		// An allowed list limits the agent's choices of a key the file
		// leaves open (G21, G25).
		for _, l := range []struct {
			key, fixed string
			value      any
			usable     bool
			written    bool
			target     *[]string
		}{
			{"allowed_models", "model", e.AllowedModels, modelPresets[kind], e.Model != nil, &r.AllowedModels},
			{"allowed_efforts", "effort", e.AllowedEfforts, effortPresets[kind], e.Effort != nil, &r.AllowedEfforts},
		} {
			if l.value == nil {
				continue
			}
			items, isList := l.value.([]any)
			switch {
			case !l.usable:
				return nil, true, fail(l.key, "%s is not used by preset %s", l.key, kind)
			case l.written:
				return nil, true, fail(l.key, "%s and %s are both set: a written %s leaves the agent nothing to choose", l.fixed, l.key, l.fixed)
			case !isList:
				return nil, true, fail(l.key, "%s must be a list of strings", l.key)
			case len(items) == 0:
				return nil, true, fail(l.key, "%s is empty: write %s to fix the value instead", l.key, l.fixed)
			}
			for n, item := range items {
				s, ok := item.(string)
				if !ok {
					return nil, true, fail(l.key, "%s[%d] is not a string", l.key, n)
				}
				if why := choiceProblem(s); why != "" {
					return nil, true, fail(l.key, "%s[%d] %q: %s", l.key, n, s, why)
				}
				*l.target = append(*l.target, s)
			}
		}
		if e.TimeoutSeconds != nil {
			if *e.TimeoutSeconds <= 0 {
				return nil, true, fail("timeout_seconds", "timeout_seconds must be positive")
			}
			r.TimeoutSeconds = *e.TimeoutSeconds
		}
		out = append(out, r)
	}
	return out, present, nil
}

// forbiddenKey finds a key a project file may not set, at any depth.
func forbiddenKey(prefix string, value any) string {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if forbiddenProjectKeys[k] {
				return join(prefix, k)
			}
			if found := forbiddenKey(join(prefix, k), v[k]); found != "" {
				return found
			}
		}
	case []any:
		for i, item := range v {
			if found := forbiddenKey(fmt.Sprintf("%s[%d]", prefix, i), item); found != "" {
				return found
			}
		}
	}
	return ""
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func (c *Config) loadProject(repo string) error {
	path := filepath.Join(repo, ".jaekit", "spar.toml")
	exists, err := sourceFile(path)
	if err != nil || !exists {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return &ConfigError{Path: path, Msg: err.Error()}
	}
	c.Files = append(c.Files, fileDigest(path, data))
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return tomlError(path, data, err)
	}
	lines := keyLines(data)
	if key := forbiddenKey("", raw); key != "" {
		last := key[strings.LastIndex(key, ".")+1:]
		if i := strings.Index(last, "["); i >= 0 {
			last = last[:i]
		}
		return &ConfigError{Path: path, Line: lineOfKey(lines, key), Msg: fmt.Sprintf("a project configuration may not set %q (%s): reviewers, commands, executables and models come only from the user's configuration", last, key)}
	}
	var file projectFile
	var ignored map[string]any
	if err := decodeStrict(path, data, &ignored, &file); err != nil {
		return err
	}
	if file.Caps != nil {
		for key, pair := range map[string]struct {
			value  *int64
			target *int64
		}{"plan": {file.Caps.Plan, &c.Caps.Plan}, "plan_delta": {file.Caps.PlanDelta, &c.Caps.PlanDelta}, "code": {file.Caps.Code, &c.Caps.Code}} {
			if pair.value == nil {
				continue
			}
			if *pair.value < 0 {
				return &ConfigError{Path: path, Line: lines["caps."+key], Msg: fmt.Sprintf("caps.%s must not be negative", key)}
			}
			*pair.target = *pair.value
		}
	}
	if file.Angles != nil {
		if len(file.Angles) == 0 {
			return &ConfigError{Path: path, Line: lines["angles"], Msg: "angles must name at least one angle"}
		}
		seen := map[string]bool{}
		for _, a := range file.Angles {
			if !contains(Angles, a) {
				return &ConfigError{Path: path, Line: lines["angles"], Msg: fmt.Sprintf("unknown angle %q (%s)", a, strings.Join(Angles, ", "))}
			}
			if seen[a] {
				return &ConfigError{Path: path, Line: lines["angles"], Msg: fmt.Sprintf("angle %q is listed twice", a)}
			}
			seen[a] = true
		}
		var ordered []string
		for _, a := range Angles {
			if seen[a] {
				ordered = append(ordered, a)
			}
		}
		c.Angles = ordered
	}
	for _, v := range file.Rules {
		if msg := ruleFileProblem(repo, v); msg != "" {
			return &ConfigError{Path: path, Line: lines["rules"], Msg: fmt.Sprintf("rules entry %q: %s", v, msg)}
		}
	}
	for _, v := range file.CheckPaths {
		if err := validGlob(v); err != nil {
			return &ConfigError{Path: path, Line: lines["check_paths"], Msg: fmt.Sprintf("check_paths entry %q: %v", v, err)}
		}
	}
	if file.Rules != nil {
		c.Rules = append([]string{}, file.Rules...)
	}
	if file.CheckPaths != nil {
		c.CheckPaths = append([]string{}, file.CheckPaths...)
	}
	return nil
}

// ruleFileProblem says why a project rules entry is not a readable file
// inside the repository (G23), or "" when it is one. A symbolic link counts
// by the file it leads to.
func ruleFileProblem(repo, entry string) string {
	if strings.TrimSpace(entry) == "" || strings.HasPrefix(entry, "/") || filepath.IsAbs(entry) || !filepath.IsLocal(filepath.FromSlash(entry)) {
		return "not a path inside the repository"
	}
	root, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return "the repository cannot be resolved: " + err.Error()
	}
	target, err := filepath.EvalSymlinks(filepath.Join(repo, filepath.FromSlash(entry)))
	if err != nil {
		return "the file cannot be read: " + err.Error()
	}
	if rel, err := filepath.Rel(root, target); err != nil || !filepath.IsLocal(rel) {
		return "a symbolic link leads outside the repository"
	}
	info, err := os.Stat(target)
	if err != nil {
		return "the file cannot be read: " + err.Error()
	}
	if !info.Mode().IsRegular() {
		return "not a regular file"
	}
	f, err := os.Open(target)
	if err != nil {
		return "the file cannot be read: " + err.Error()
	}
	f.Close()
	return ""
}

func lineOfKey(lines map[string]int, key string) int {
	for k := key; k != ""; {
		if l := lines[k]; l > 0 {
			return l
		}
		cut := strings.LastIndexAny(k, ".[")
		if cut < 0 {
			break
		}
		k = k[:cut]
	}
	return 0
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// digest identifies the applied settings, not where they came from: the
// sources and defaults of models and efforts are left out.
func (c *Config) digest() string {
	reviewers := make([]Reviewer, len(c.Reviewers))
	for i, r := range c.Reviewers {
		r.ModelSource, r.DefaultModel, r.EffortSource, r.DefaultEffort = "", "", "", ""
		reviewers[i] = r
	}
	applied := struct {
		Reviewers          []Reviewer    `json:"reviewers"`
		ConfiguredRequired []RequiredRef `json:"configured_required"`
		Angles             []string      `json:"angles"`
		Caps               Caps          `json:"caps"`
		CheckPaths         []string      `json:"check_paths"`
		Rules              []string      `json:"rules"`
	}{reviewers, c.ConfiguredRequired, c.Angles, c.Caps, c.CheckPaths, c.Rules}
	data, _ := json.Marshal(applied)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Find returns the reviewer with a name.
func (c *Config) Find(name string) (Reviewer, bool) {
	for _, r := range c.Reviewers {
		if r.Name == name {
			return r, true
		}
	}
	return Reviewer{}, false
}
