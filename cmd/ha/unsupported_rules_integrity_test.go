package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jgoneit/jaekit/internal/status"
)

func TestUnsupportedRulesPrecedeRecordIntegrity(t *testing.T) {
	for _, rules := range []string{"run-rules/99", status.RulesV1, status.RulesV2} {
		t.Run(rules, func(t *testing.T) {
			for _, damage := range []string{"prev", "seq", "unterminated"} {
				t.Run(damage, func(t *testing.T) {
					s := unsupportedRulesFixture(t)
					s.ha("start", scenarioGoal, "--request", "implement", "--skill-name", "seal", "--skill-version", "0.1.0")
					if s.lastCode != 0 {
						t.Fatalf("start: exit %d: %s", s.lastCode, s.lastOut)
					}
					s.ha("note", scenarioGoal, "input", "--quote", "continue")
					if s.lastCode != 0 {
						t.Fatalf("note: exit %d: %s", s.lastCode, s.lastOut)
					}
					st := step{where: t.Name()}
					lines := s.records(st)
					lines[0] = fieldPattern("rules").ReplaceAll(lines[0], []byte(`"rules":"`+rules+`"`))
					s.writeRecords(st, lines, true)
					lines = s.records(st)
					switch damage {
					case "prev":
						lines[1] = fieldPattern("prev").ReplaceAll(lines[1], []byte(`"prev":null`))
					case "seq":
						lines[1] = fieldPattern("seq").ReplaceAll(lines[1], []byte(`"seq":99`))
					}
					before := bytes.Join(lines, []byte("\n"))
					if damage != "unterminated" {
						before = append(before, '\n')
					}
					path := s.file(scenarioGoal + "/runs.jsonl")
					if err := os.WriteFile(path, before, 0o644); err != nil {
						t.Fatal(err)
					}
					commands := [][]string{
						{"check", scenarioGoal, "AC-2"},
						{"check", scenarioGoal, "--baseline", "AC-1"},
					}
					want := exitRefused
					if rules == "run-rules/99" {
						want = exitRules
						commands = append(commands,
							[]string{"status", scenarioGoal},
							[]string{"done", scenarioGoal},
							[]string{"note", scenarioGoal, "input", "--quote", "continue"},
						)
					}
					for _, args := range commands {
						s.ha(args...)
						if s.lastCode != want {
							t.Errorf("%v: exit %d, want %d: %s", args, s.lastCode, want, s.lastOut)
						}
						after, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(after, before) {
							t.Fatalf("%v changed the run record", args)
						}
					}
				})
			}
		})
	}
}

func TestCheckWithoutStartStillRefused(t *testing.T) {
	s := unsupportedRulesFixture(t)
	s.ha("check", scenarioGoal, "AC-2")
	if s.lastCode != exitRefused {
		t.Fatalf("check: exit %d, want %d: %s", s.lastCode, exitRefused, s.lastOut)
	}
	if _, err := os.Stat(s.file(scenarioGoal + "/runs.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("check without start created a record: %v", err)
	}
}

func unsupportedRulesFixture(t *testing.T) *scenario {
	t.Helper()
	s := &scenario{t: t, name: t.Name(), dir: t.TempDir()}
	s.root = s.dir
	copyTree(t, filepath.Join(scenarioRoot, "_base"), s.dir)
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "scenario")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "scenario@example.invalid")
	}
	s.git("init", "-q", "-b", "main")
	s.git("add", "-A")
	s.git("commit", "-q", "-m", "base")
	t.Chdir(s.dir)
	return s
}
