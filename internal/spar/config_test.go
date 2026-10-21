package spar

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func names(c *Config) string {
	var out []string
	for _, r := range c.Reviewers {
		out = append(out, r.Name)
	}
	return strings.Join(out, ",")
}

func TestReviewerSourceOrder(t *testing.T) {
	dir := t.TempDir()
	home, xdg, repo := filepath.Join(dir, "home"), filepath.Join(dir, "xdg"), filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	explicit := writeFile(t, filepath.Join(dir, "explicit.toml"), "[[reviewers]]\npreset = \"codex\"\n")
	writeFile(t, filepath.Join(xdg, "jaekit", "spar.toml"), "[[reviewers]]\npreset = \"agy\"\n")
	writeFile(t, filepath.Join(home, ".config", "jaekit", "spar.toml"), "[[reviewers]]\npreset = \"claude\"\n")
	vars := map[string]string{"HOME": home, "XDG_CONFIG_HOME": xdg, "SPAR_CONFIG": explicit}
	for _, want := range []struct{ drop, names, kind string }{
		{"", "codex", "SPAR_CONFIG"},
		{"SPAR_CONFIG", "agy", "XDG_CONFIG_HOME"},
		{"XDG_CONFIG_HOME", "claude", "HOME"},
	} {
		delete(vars, want.drop)
		c, err := LoadConfig(repo, env(vars), nil)
		if err != nil {
			t.Fatal(err)
		}
		if names(c) != want.names || c.Source.Kind != want.kind {
			t.Fatalf("after dropping %q: %s from %s", want.drop, names(c), c.Source.Kind)
		}
	}
	// A SPAR_CONFIG that names no file falls through to the next location.
	vars["SPAR_CONFIG"] = filepath.Join(dir, "missing.toml")
	if c, err := LoadConfig(repo, env(vars), nil); err != nil || names(c) != "claude" || len(c.Source.Skipped) == 0 {
		t.Fatalf("missing SPAR_CONFIG: %v %+v", err, c)
	}
}

func TestCleanReviewFallbackDefaultsAndOverride(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	vars := map[string]string{"HOME": filepath.Join(dir, "home"), "XDG_CONFIG_HOME": filepath.Join(dir, "xdg")}
	c, err := LoadConfig(repo, env(vars), nil)
	if err != nil || names(c) != "claude,codex,agy" || c.Source.Kind != "default" {
		t.Fatalf("default: %v %s %s", err, names(c), c.Source.Kind)
	}
	writeFile(t, filepath.Join(dir, "xdg", "clean-review", "config.json"), `{"reviewers": ["codex", "agy"]}`)
	c, err = LoadConfig(repo, env(vars), nil)
	if err != nil || names(c) != "codex,agy" || c.Source.Kind != "clean-review" {
		t.Fatalf("clean-review: %v %s", err, names(c))
	}
	// Like clean-review, only one config.json is chosen: with XDG_CONFIG_HOME set, a file under
	// ~/.config is not read, and a CLEAN_REVIEW_CONFIG that names no file does not fall through.
	writeFile(t, filepath.Join(dir, "home", ".config", "clean-review", "config.json"), `{"reviewers": ["claude"]}`)
	for _, extra := range []map[string]string{{}, {"CLEAN_REVIEW_CONFIG": filepath.Join(dir, "missing.json")}} {
		only := map[string]string{"HOME": vars["HOME"], "XDG_CONFIG_HOME": filepath.Join(dir, "other")}
		for k, v := range extra {
			only[k] = v
		}
		if c, err := LoadConfig(repo, env(only), nil); err != nil || c.Source.Kind != "default" || len(c.Source.Skipped) == 0 {
			t.Fatalf("clean-review path %v: %v %+v", extra, err, c)
		}
	}
	writeFile(t, filepath.Join(dir, "xdg", "clean-review", "config.json"), "null")
	if _, err := LoadConfig(repo, env(vars), nil); err == nil {
		t.Fatal("a clean-review config.json holding null started the default reviewers")
	}
	writeFile(t, filepath.Join(dir, "xdg", "clean-review", "config.json"), `{"reviewers": []}`)
	if c, _ = LoadConfig(repo, env(vars), nil); names(c) != "native" || c.Reviewers[0].Isolation != IsolationHost {
		t.Fatalf("empty clean-review list: %s", names(c))
	}
	first, _ := LoadConfig(repo, env(vars), nil)
	again, _ := LoadConfig(repo, env(vars), nil)
	over, err := LoadConfig(repo, env(vars), []string{"agy"})
	if err != nil || first.Digest != again.Digest || over.Digest == first.Digest || names(over) != "agy" || over.Override == nil {
		t.Fatalf("digest or override: %v %s %s %s", err, first.Digest, again.Digest, over.Digest)
	}
	if _, err := LoadConfig(repo, env(vars), []string{"gemini"}); err == nil {
		t.Fatal("an unknown reviewer in --reviewers was accepted")
	}
	// The same --reviewers run under configurations that require different
	// reviewers is not the same applied configuration.
	writeFile(t, filepath.Join(dir, "xdg", "clean-review", "config.json"), `{"reviewers": ["native"]}`)
	one, _ := LoadConfig(repo, env(vars), []string{"native"})
	writeFile(t, filepath.Join(dir, "xdg", "clean-review", "config.json"), `{"reviewers": ["native", "codex"]}`)
	two, _ := LoadConfig(repo, env(vars), []string{"native"})
	if one == nil || two == nil || one.Digest == two.Digest {
		t.Fatal("the digest ignores the configured required reviewers")
	}
}

func TestConfigErrorsNameFileLineAndKey(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	cases := []struct {
		body  string
		line  int
		token string
	}{
		{"[[reviewers]]\nname = \"codex\"\npreset = \"codex\n", 3, "syntax"},
		{"[[reviewers]]\nname = \"codex\"\npreset = \"codex\"\nmodle = \"x\"\n", 4, "modle"},
		{"[[reviewers]]\nname = \"a\"\npreset = \"codex\"\n\n[[reviewers]]\nname = \"a\"\npreset = \"claude\"\n", 6, "\"a\""},
		{"[[reviewers]]\nname = \"g\"\npreset = \"gemini\"\n", 3, "gemini"},
		{"[[reviewers]]\nname = \"c\"\npreset = \"command\"\n", 1, "argv"},
		{"# comment\ntimeout = 3\n", 2, "timeout"},
		{"[[reviewers]]\npreset = \"native\"\nmodel = \"x\"\n", 3, "model"},
		{"timeout_seconds = 0\n", 1, "timeout_seconds"},
		{"[[reviewers]]\nname = \"Packet.md\"\npreset = \"codex\"\n", 2, "Packet.md"},
	}
	for _, tc := range cases {
		path := writeFile(t, filepath.Join(dir, "spar.toml"), tc.body)
		_, err := LoadConfig(repo, env(map[string]string{"SPAR_CONFIG": path, "HOME": dir}), nil)
		var cfg *ConfigError
		if !errors.As(err, &cfg) {
			t.Fatalf("%q: %v", tc.body, err)
		}
		if cfg.Path != path || cfg.Line != tc.line || !strings.Contains(err.Error(), tc.token) {
			t.Errorf("%q: %v (line %d, want %d and %s)", tc.body, err, cfg.Line, tc.line, tc.token)
		}
	}
}

func TestFullTOMLSyntax(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	body := "# c\r\ntimeout_seconds = 1_800\r\n[[reviewers]]\r\nname = 'codex'\r\n\"preset\" = \"\"\"codex\"\"\"\r\n" +
		"timeout_seconds = 0x384\r\n[[reviewers]]\r\nname = \"cmd\"\r\npreset = \"command\"\r\nargv = [\r\n \"/bin/x\", # c\r\n '{prompt}',\r\n]\r\n"
	path := writeFile(t, filepath.Join(dir, "spar.toml"), body)
	c, err := LoadConfig(repo, env(map[string]string{"SPAR_CONFIG": path}), nil)
	if err != nil || names(c) != "codex,cmd" || c.Reviewers[0].TimeoutSeconds != 900 || c.Reviewers[1].TimeoutSeconds != 1800 {
		t.Fatalf("%v %+v", err, c)
	}
	inline := writeFile(t, filepath.Join(dir, "inline.toml"),
		"reviewers = [ { name = \"agy\", preset = \"agy\", fallback_model = \"\" }, { preset = \"codex\" } ]\n")
	c, err = LoadConfig(repo, env(map[string]string{"SPAR_CONFIG": inline}), nil)
	if err != nil || names(c) != "agy,codex" || c.Reviewers[0].FallbackModel != "" {
		t.Fatalf("inline tables: %v %+v", err, c)
	}
}

func TestProjectConfigLimits(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	vars := env(map[string]string{"HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "xdg")})
	for _, key := range []string{"reviewers", "command", "argv", "binary", "model", "fallback_model", "effort", "preset"} {
		path := writeFile(t, filepath.Join(repo, ".jaekit", "spar.toml"), "[caps]\ncode = 2\n\n[extra]\n"+key+" = \"x\"\n")
		_, err := LoadConfig(repo, vars, nil)
		var cfg *ConfigError
		if !errors.As(err, &cfg) || cfg.Path != path || !strings.Contains(err.Error(), key) || cfg.Line != 5 {
			t.Errorf("project %s: %v", key, err)
		}
	}
	writeFile(t, filepath.Join(repo, ".jaekit", "spar.toml"),
		"caps.code = 2\n\"check_paths\" = [ \"checks/**\", ]\nangles = [\"check-validity\", \"premises\"]\n")
	c, err := LoadConfig(repo, vars, nil)
	if err != nil || c.Caps.Code != 2 || c.Caps.Plan != 1 || strings.Join(c.CheckPaths, ",") != "checks/**" ||
		strings.Join(c.Angles, ",") != "premises,check-validity" {
		t.Fatalf("%v %+v", err, c)
	}
	writeFile(t, filepath.Join(repo, ".jaekit", "spar.toml"), "caps.code = 2\n[caps]\n")
	if _, err := LoadConfig(repo, vars, nil); err == nil || !strings.Contains(err.Error(), ":2") {
		t.Fatalf("a table defined twice: %v", err)
	}
	for _, body := range []string{"rules = [\"../outside.md\"]\n", "check_paths = [\"/abs/**\"]\n", "rules = [\"a/../../b\"]\n"} {
		writeFile(t, filepath.Join(repo, ".jaekit", "spar.toml"), body)
		if _, err := LoadConfig(repo, vars, nil); err == nil || !strings.Contains(err.Error(), "inside the repository") {
			t.Errorf("%q: %v", body, err)
		}
	}
	writeFile(t, filepath.Join(repo, ".jaekit", "spar.toml"), "angles = [\"design\"]\nunknown_key = 1\n")
	if _, err := LoadConfig(repo, vars, nil); err == nil || !strings.Contains(err.Error(), "unknown_key") ||
		!strings.Contains(err.Error(), ":2:") {
		t.Fatalf("unknown project key: %v", err)
	}
}

// The configuration rows of the input table (contracts/spar-input.md,
// G16 to G25): what each source, list, name, rule and glob accepts.
func TestConfigInputRows(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	empty := filepath.Join(dir, "empty")
	os.MkdirAll(empty, 0o755)
	vars := map[string]string{"HOME": empty, "XDG_CONFIG_HOME": empty}
	load := func(spar string) (*Config, error) {
		v := map[string]string{"HOME": empty, "XDG_CONFIG_HOME": empty, "SPAR_CONFIG": spar}
		return LoadConfig(repo, env(v), nil)
	}
	// G16: a folder is an error, a broken link or a link loop is skipped.
	if _, err := load(empty); err == nil || !strings.Contains(err.Error(), empty) {
		t.Errorf("a folder as SPAR_CONFIG: %v", err)
	}
	loop := filepath.Join(dir, "loop.toml")
	os.Symlink(loop, loop)
	if c, err := load(loop); err != nil || !contains(c.Source.Skipped, loop) {
		t.Errorf("a symbolic link loop: %v %+v", err, c)
	}
	// G18: the clean-review list holds names only.
	for _, body := range []string{`{"reviewers": null}`, `{"reviewers": [null]}`, `{"reviewers": "codex"}`, `{"reviewers": [1]}`} {
		clean := writeFile(t, filepath.Join(dir, "clean.json"), body)
		v := map[string]string{"HOME": empty, "XDG_CONFIG_HOME": empty, "CLEAN_REVIEW_CONFIG": clean}
		if _, err := LoadConfig(repo, env(v), nil); err == nil {
			t.Errorf("clean-review %s was accepted", body)
		}
	}
	clean := writeFile(t, filepath.Join(dir, "clean.json"), `{"reviewers": [" codex ", ""]}`)
	if c, err := LoadConfig(repo, env(map[string]string{"HOME": empty, "XDG_CONFIG_HOME": empty, "CLEAN_REVIEW_CONFIG": clean}), nil); err != nil || names(c) != "codex" {
		t.Errorf("clean-review names with spaces and an empty one: %v %+v", err, c)
	}
	// G19: the top-level time limit is the default of every reviewer the file decides.
	only := writeFile(t, filepath.Join(dir, "timeout.toml"), "timeout_seconds = 60\n")
	c, err := load(only)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range c.Reviewers {
		if r.TimeoutSeconds != 60 {
			t.Errorf("%s: timeout %d, want 60", r.Name, r.TimeoutSeconds)
		}
	}
	listed := writeFile(t, filepath.Join(dir, "listed.toml"), "timeout_seconds = 45\n[[reviewers]]\npreset = \"claude\"\n")
	if c, err := LoadConfig(repo, env(map[string]string{"HOME": empty, "XDG_CONFIG_HOME": empty, "SPAR_CONFIG": listed}), []string{"codex"}); err != nil ||
		len(c.Reviewers) != 1 || c.Reviewers[0].TimeoutSeconds != 45 {
		t.Errorf("--reviewers codex with a top-level timeout: %v %+v", err, c)
	}
	// G20: names are compared ignoring case, and round file names and their
	// .tmp names are reserved.
	for _, pair := range [][2]string{{"Host", "host"}, {"codex", "CODEX"}} {
		body := "[[reviewers]]\nname = \"" + pair[0] + "\"\npreset = \"native\"\n[[reviewers]]\nname = \"" + pair[1] + "\"\npreset = \"native\"\n"
		if _, err := load(writeFile(t, filepath.Join(dir, "dup.toml"), body)); err == nil {
			t.Errorf("names %v were both accepted", pair)
		}
	}
	for _, name := range []string{"round.json.tmp", "Summary.md", "WORKSPACE"} {
		body := "[[reviewers]]\nname = \"" + name + "\"\npreset = \"native\"\n"
		if _, err := load(writeFile(t, filepath.Join(dir, "reserved.toml"), body)); err == nil {
			t.Errorf("the reserved name %q was accepted", name)
		}
	}
	// G25: empty items are skipped, and a list of none is native.
	for flag, want := range map[string]string{"": "native", ",": "native", " codex , agy ": "codex,agy"} {
		if c, err := LoadConfig(repo, env(vars), reviewerOverride(parsed{values: map[string][]string{"reviewers": {flag}}})); err != nil || names(c) != want {
			t.Errorf("--reviewers %q: %v %s", flag, err, names(c))
		}
	}
	// G23 and G24: rules are readable files inside the repository, and
	// check paths are valid globs; both errors name the key's line.
	outside := writeFile(t, filepath.Join(dir, "outside.md"), "rule\n")
	os.Symlink(outside, filepath.Join(repo, "linked.md"))
	os.MkdirAll(filepath.Join(repo, "somedir"), 0o755)
	for _, body := range []string{"rules = [\"linked.md\"]\n", "rules = [\"missing.md\"]\n", "rules = [\"somedir\"]\n",
		"check_paths = [\"checks/[a\"]\n", "check_paths = [\"a/../b\"]\n"} {
		writeFile(t, filepath.Join(repo, ".jaekit", "spar.toml"), body)
		if _, err := LoadConfig(repo, env(vars), nil); err == nil || !strings.Contains(err.Error(), "spar.toml:1") {
			t.Errorf("%q: %v", body, err)
		}
	}
}
