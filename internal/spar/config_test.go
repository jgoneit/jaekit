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
