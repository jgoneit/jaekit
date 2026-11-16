package plugins

// Property: current release version and tag
// Every place that names the current release names the same version and tag:
// the marketplace metadata, both executables, the issue templates, the
// READMEs, the guides' pins and downloads, and the Seal skill's links.

import (
	"slices"
	"strings"
	"testing"
)

// The current release. The combination rows and sentences below also name
// each component's version, so they spell the whole combination out.
const (
	rvTag     = "v0.1.8"
	rvVersion = "0.1.8"
)

// The marketplace names the current release, both executables carry its
// development version, and the issue templates suggest it.
func TestReleaseVersionIdentity(t *testing.T) {
	var market struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	if market.Metadata.Version != rvVersion {
		t.Errorf("../.claude-plugin/marketplace.json metadata.version is %q, not %s", market.Metadata.Version, rvVersion)
	}
	for _, path := range []string{"../cmd/ha/main.go", "../cmd/spar/main.go"} {
		requireAll(t, path, `var version = "`+rvVersion+`-dev"`)
	}
	for _, path := range []string{"../.github/ISSUE_TEMPLATE/install-bug.yml", "../.github/ISSUE_TEMPLATE/usage-report.yml"} {
		requireAll(t, path, "placeholder: ha "+rvVersion+"\n")
	}
}

// The READMEs name the current release, the READMEs and install guides pin
// both hosts' marketplace to its tag, and the install and WSL guides
// download its files.
func TestReleaseVersionPinsAndDownloads(t *testing.T) {
	for _, path := range []string{"../README.md", "../README.ko.md"} {
		requireAll(t, path, "**"+rvTag+"**", "`ha "+rvVersion+"`")
	}
	for _, path := range []string{"../README.md", "../README.ko.md", "../guides/INSTALL.md", "../guides/INSTALL.en.md"} {
		requireAll(t, path,
			"claude plugin marketplace add jgoneit/jaekit#"+rvTag,
			"codex plugin marketplace add jgoneit/jaekit@"+rvTag)
	}
	for _, c := range []struct{ path, heading string }{
		{"../guides/INSTALL.md", "## Release 파일로 설치"},
		{"../guides/INSTALL.en.md", "## Install from a Release file"},
	} {
		requireAll(t, c.path,
			"https://github.com/jgoneit/jaekit/releases/tag/"+rvTag,
			`url="https://github.com/jgoneit/jaekit/releases/download/`+rvTag+`"`,
			`name="ha_`+rvVersion+`_${os}_${arch}"`,
			`share="$HOME/.local/share/jaekit/`+rvVersion+`"`,
			`package_root="$HOME/.local/share/jaekit/`+rvVersion+`"`,
			"current="+rvVersion+"\n")
		requireIn(t, c.path+" section "+c.heading, mdSection(t, c.path, c.heading),
			"releases/download/"+rvTag, "ha_"+rvVersion+"_")
	}
	for _, path := range []string{"../guides/WSL.md", "../guides/WSL.en.md"} {
		requireAll(t, path, "`"+rvTag+"`", "ha_"+rvVersion+"_linux_amd64.tar.gz", "~/.local/share/jaekit/"+rvVersion,
			"https://github.com/jgoneit/jaekit/releases/tag/"+rvTag)
	}
}

// The Windows guides name the current release where they speak of Windows
// files.
func TestReleaseVersionWindowsGuides(t *testing.T) {
	for _, path := range []string{"../guides/WINDOWS.md", "../guides/WINDOWS.en.md"} {
		rvLine(t, path, rvTag, "Windows")
	}
}

// rvLine requires one line of the document to hold every token, so a
// sentence around the tokens can be reworded.
func rvLine(t *testing.T, path string, tokens ...string) {
	t.Helper()
	for _, line := range strings.Split(read(t, path), "\n") {
		all := true
		for _, token := range tokens {
			all = all && strings.Contains(line, token)
		}
		if all {
			return
		}
	}
	t.Errorf("%s has no line with all of %q", path, tokens)
}

// The Seal skill and its ha reference send a missing ha to the current
// release's install guide, and the core capabilities contract links the
// current release's check_core.py.
func TestReleaseVersionSkillLinks(t *testing.T) {
	const skill = "seal/skills/seal/SKILL.md"
	install := "https://github.com/jgoneit/jaekit/blob/" + rvTag + "/guides/INSTALL.md"
	for _, path := range []string{skill, "seal/skills/seal/references/ha.md"} {
		requireAll(t, path, install)
	}
	requireAll(t, "../contracts/core-capabilities.md",
		"Seal에 포함된 [check_core.py](https://github.com/jgoneit/jaekit/blob/"+rvTag+"/plugins/seal/skills/seal/scripts/check_core.py)")
	// The instruction for a missing ha carries the link itself; the last
	// line naming that case is the instruction.
	var line string
	for _, l := range strings.Split(read(t, skill), "\n") {
		if strings.Contains(l, "If `ha` is missing") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("%s has no line with %q", skill, "If `ha` is missing")
	}
	requireIn(t, skill+" missing-ha instruction", line, install)
}

// The install guides' combination table has the current release's row and
// the source build's row, and the usage and operations guides and the Seal
// ha reference state the current combination.
func TestReleaseVersionCombination(t *testing.T) {
	for _, c := range []struct {
		path string
		rows []string
	}{
		{"../guides/INSTALL.md", []string{
			"| 현재 배포판 `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |",
			"| 소스 빌드 | `ha 0.1.8-dev`, `spar 0.1.8-dev` |"}},
		{"../guides/INSTALL.en.md", []string{
			"| Current release `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |",
			"| Source build | `ha 0.1.8-dev`, `spar 0.1.8-dev` |"}},
	} {
		lines := strings.Split(read(t, c.path), "\n")
		for _, prefix := range c.rows {
			if !slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, prefix) }) {
				t.Errorf("%s has no table row starting %q", c.path, prefix)
			}
		}
	}
	for _, path := range []string{"../guides/USAGE.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.md", "../guides/OPERATIONS.en.md"} {
		rvLine(t, path, "v0.1.8", "ha 0.1.8", "spar 0.1.8", "spec 0.1.13", "seal 0.1.23", "0.1.4")
	}
	requireAll(t, "seal/skills/seal/references/ha.md", "The commands below describe release v0.1.8: ha 0.1.8, spar 0.1.8 and Seal 0.1.23.")
}
