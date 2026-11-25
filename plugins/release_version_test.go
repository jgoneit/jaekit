package plugins

// Property: current release version and tag
// Every place that names the current release names the same version and tag:
// the marketplace metadata, both executables, the issue templates, the
// READMEs, the guides' pins and downloads, and the Seal skill's links.

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The current release. The combination rows and sentences below also name
// each component's version, so they spell the whole combination out.
const (
	rvTag     = "v0.1.9"
	rvVersion = "0.1.9"
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

// rvBacktickTag is a release tag in a code span, the form the Windows guides
// use for the current release.
var rvBacktickTag = regexp.MustCompile("`(v\\d+\\.\\d+\\.\\d+)`")

// Every release tag the Windows guides write in a code span names the current
// release, and each guide has one, so changing one of them fails. How many
// lines name it, and where, is left to the prose.
func TestReleaseVersionWindowsGuides(t *testing.T) {
	for _, path := range []string{"../guides/WINDOWS.md", "../guides/WINDOWS.en.md"} {
		seen := 0
		for number, line := range strings.Split(read(t, path), "\n") {
			for _, m := range rvBacktickTag.FindAllStringSubmatch(line, -1) {
				seen++
				if m[1] != rvTag {
					t.Errorf("%s:%d names %s, want the current release %s", path, number+1, m[1], rvTag)
				}
			}
		}
		if seen == 0 {
			t.Errorf("%s names no release tag in a code span, want %s", path, rvTag)
		}
		// The first release with Windows files is planned after the current
		// one, and every block of the guide already carries that version.
		requireAll(t, path, "`"+rvWindowsPlanned+"`")
		blocks := rvWindowsVersion.FindAllStringSubmatch(read(t, path), -1)
		if len(blocks) < 2 {
			t.Errorf("%s has %d $version lines, want the install and remove blocks' two", path, len(blocks))
		}
		for _, m := range blocks {
			if m[1] != rvWindowsPlanned {
				t.Errorf("%s has %q, want $version = '%s'", path, m[0], rvWindowsPlanned)
			}
		}
	}
}

// rvWindowsVersion is a Windows block's release version assignment.
var rvWindowsVersion = regexp.MustCompile(`\$version = '([^']*)'`)

// rvWindowsPlanned is the planned first release with Windows files.
const rvWindowsPlanned = "0.1.10"

// rvFixedForms are the ways the READMEs and guides write a version that can
// only mean the current release: marketplace pins, release pages and
// downloads, release-pinned links, archive names, installed data paths, the
// READMEs' bold release and the Release file script's current version.
var rvFixedForms = []*regexp.Regexp{
	regexp.MustCompile(`jaekit[#@](v\d+\.\d+\.\d+)`),
	regexp.MustCompile(`releases/(?:download|tag)/(v\d+\.\d+\.\d+)`),
	regexp.MustCompile(`jaekit/blob/(v\d+\.\d+\.\d+)/`),
	regexp.MustCompile(`\bha_(\d+\.\d+\.\d+)_`),
	regexp.MustCompile(`\.local/share/jaekit/(\d+\.\d+\.\d+)`),
	regexp.MustCompile(`\*\*(v\d+\.\d+\.\d+)\*\*`),
	regexp.MustCompile(`current=(\d+\.\d+\.\d+)`),
	regexp.MustCompile("`[#@](v\\d+\\.\\d+\\.\\d+)`"),
}

// rvVersionSpan is a backticked ha or spar version. It names the current
// release unless its line names an earlier or later release tag, as the
// release history rows do.
var (
	rvVersionSpan = regexp.MustCompile("`(?:ha|spar) (\\d+\\.\\d+\\.\\d+)(?:-dev)?`")
	rvAnyTag      = regexp.MustCompile(`\bv(\d+\.\d+\.\d+)\b`)
)

// Every fixed version form in the READMEs and guides names the current
// release, and so does every backticked ha or spar version on a line that
// names no other release, so changing one place to another version fails.
func TestReleaseVersionFixedForms(t *testing.T) {
	guides, err := filepath.Glob("../guides/*.md")
	if err != nil || len(guides) == 0 {
		t.Fatalf("no guides: %v", err)
	}
	seen := 0
	for _, path := range append([]string{"../README.md", "../README.ko.md", "../README.en.md"}, guides...) {
		for number, line := range strings.Split(read(t, path), "\n") {
			other := false
			for _, m := range rvAnyTag.FindAllStringSubmatch(line, -1) {
				other = other || m[1] != rvVersion
			}
			if !other {
				for _, m := range rvVersionSpan.FindAllStringSubmatch(line, -1) {
					seen++
					if m[1] != rvVersion {
						t.Errorf("%s:%d names %s in %q, want the current release %s", path, number+1, m[1], m[0], rvVersion)
					}
				}
			}
			for _, form := range rvFixedForms {
				for _, m := range form.FindAllStringSubmatch(line, -1) {
					seen++
					if m[1] != rvTag && m[1] != rvVersion {
						t.Errorf("%s:%d names %s in %q, want the current release %s", path, number+1, m[1], m[0], rvTag)
					}
				}
			}
		}
	}
	if seen == 0 {
		t.Fatalf("the READMEs and guides have no fixed version form")
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

// rvPinnedURL is a release-pinned Jaekit URL in a skill document, whether a
// link target or bare text.
var rvPinnedURL = regexp.MustCompile("https://github\\.com/jgoneit/jaekit/blob/(v\\d+\\.\\d+\\.\\d+)/[^\\s)<>\\]`]*")

// Every release-pinned URL of the Seal skill, linked or bare, names the
// current release; the skill links the current release's reference check and
// names its install guide; and the core capabilities contract links the
// current release's check_core.py.
func TestReleaseVersionSkillLinks(t *testing.T) {
	requireSkillTokens(t, sealSkillDir,
		"link:https://github.com/jgoneit/jaekit/blob/"+rvTag+"/guides/INSTALL.en.md#get-the-reference-check")
	install := "https://github.com/jgoneit/jaekit/blob/" + rvTag + "/guides/INSTALL.md"
	named := false
	for _, doc := range skillDocs(t, sealSkillDir) {
		for _, m := range rvPinnedURL.FindAllStringSubmatch(doc.text, -1) {
			url := strings.TrimRight(m[0], ".,;:")
			if m[1] != rvTag {
				t.Errorf("%s names %s, want the current release %s", doc.path, url, rvTag)
			}
			named = named || url == install || strings.HasPrefix(url, install+"#")
		}
	}
	if !named {
		t.Errorf("%s does not name %s", sealSkillDir, install)
	}
	requireAll(t, "../contracts/core-capabilities.md",
		"[check_core.py](https://github.com/jgoneit/jaekit/blob/"+rvTag+"/plugins/seal/skills/seal/scripts/check_core.py)",
		`"ha_version": "`+rvVersion+`"`)
	// The core capabilities contract names the Core, Seal and release versions
	// it applies to on one line, in that order; only the numbers are read.
	applies := regexp.MustCompile(`(?:^|[^0-9.])` + regexp.QuoteMeta(rvVersion) + `[^0-9]+` + regexp.QuoteMeta(rvSeal) + `[^0-9]+` + regexp.QuoteMeta(rvVersion) + `(?:[^0-9]|$)`)
	if !slices.ContainsFunc(strings.Split(read(t, "../contracts/core-capabilities.md"), "\n"), applies.MatchString) {
		t.Errorf("../contracts/core-capabilities.md has no line naming Core %s, Seal %s and release %s in that order", rvVersion, rvSeal, rvTag)
	}
	// The contracts link the published skills and show the current release's
	// versions in their examples.
	requireAll(t, "../contracts/role-card.md",
		"https://github.com/jgoneit/jaekit/blob/"+rvTag+"/plugins/seal/skills/seal/SKILL.md")
	requireAll(t, "../contracts/goal-docs.md",
		"https://github.com/jgoneit/jaekit/blob/"+rvTag+"/plugins/spec/skills/spec/SKILL.md",
		"https://github.com/jgoneit/jaekit/blob/"+rvTag+"/plugins/spec/skills/spec/references/goal-docs.md")
	requireAll(t, "../contracts/review-gate.md", `"spar_version": "`+rvVersion+`"`)
}

// rvSeal is the Seal plugin version of the current release.
const rvSeal = "0.1.36"

// The install guides' combination table has the current and the previous
// release's version cells and the source build's versions, read without the
// rows' labels; the usage and
// operations guides state the current combination.
func TestReleaseVersionCombination(t *testing.T) {
	for _, c := range []struct {
		path string
		rows []string
	}{
		{"../guides/INSTALL.md", []string{
			" `v0.1.9` | `ha 0.1.9`, `spar 0.1.9` | 0.1.16 | 0.1.36 | 0.1.9 |",
			" `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |",
			" | `ha 0.1.9-dev`, `spar 0.1.9-dev` |"}},
		{"../guides/INSTALL.en.md", []string{
			" `v0.1.9` | `ha 0.1.9`, `spar 0.1.9` | 0.1.16 | 0.1.36 | 0.1.9 |",
			" `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |",
			" | `ha 0.1.9-dev`, `spar 0.1.9-dev` |"}},
	} {
		lines := strings.Split(read(t, c.path), "\n")
		for _, cells := range c.rows {
			if !slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, "| ") && strings.Contains(line, cells) }) {
				t.Errorf("%s has no table row with %q", c.path, cells)
			}
		}
	}
	// The Spar plugin version is read with its label, since a bare number
	// can also be part of the release tag or the executables' versions.
	for _, c := range []struct{ path, plugin string }{
		{"../guides/USAGE.md", "플러그인 0.1.9"},
		{"../guides/USAGE.en.md", "plugin 0.1.9"},
		{"../guides/OPERATIONS.md", "플러그인 0.1.9"},
		{"../guides/OPERATIONS.en.md", "plugin 0.1.9"},
	} {
		rvLine(t, c.path, "v0.1.9", "ha 0.1.9", "spar 0.1.9", "spec 0.1.16", "seal "+rvSeal, c.plugin)
	}
	// The Seal skill states the combination its commands describe on one
	// line: the release, ha, spar and Seal versions in that order, with no
	// other number between them. Only the numbers are read, which rewording
	// and moving the sentence keep.
	seal := rvSeal
	v := regexp.QuoteMeta(rvVersion)
	combo := regexp.MustCompile(`(?:^|[^0-9.])` + v + `[^0-9]+` + v + `[^0-9]+` + v + `[^0-9]+` + regexp.QuoteMeta(seal) + `(?:[^0-9]|$)`)
	stated := false
	for _, doc := range skillDocs(t, sealSkillDir) {
		for _, line := range strings.Split(doc.text, "\n") {
			stated = stated || combo.MatchString(line)
		}
	}
	if !stated {
		t.Errorf("%s has no line naming release %s, ha %s, spar %s and Seal %s in that order", sealSkillDir, rvTag, rvVersion, rvVersion, seal)
	}
}
