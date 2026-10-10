package plugins

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// without cloning Jaekit or preparing Go, and can update or remove it later.

const (
	rel011Tag     = "v0.1.2"
	rel011Version = "0.1.2"
)

type rel011Lang struct {
	readme, install            string // paths from this package
	installLink                string // the install guide, as the README links it
	quick, installSec          string // README headings
	release, update, uninstall string // install guide headings
	updateProse, removeProse   []string
}

var rel011Ko = rel011Lang{
	readme: "../README.md", install: "../guides/INSTALL.md", installLink: "guides/INSTALL.md",
	quick: "## 시작하기", installSec: "### 설치",
	release: "## Release 파일로 설치", update: "## 업데이트", uninstall: "## 제거",
	updateProse: []string{"(#release-파일로-설치)", "`ha 0.1.2`"},
	removeProse: []string{"`docs/specs/<goal>/`", "`runs.jsonl`", "`.git/ha/`", "`git rev-parse --absolute-git-dir`", "`git rm -r docs/specs/<goal>`",
		"linked worktree", "그 작업 트리에 있는 모든 목표의 검사 출력 원문이 지워집니다.", "Jaekit의 검사가 돌고 있지 않을 때"},
}

var rel011En = rel011Lang{
	readme: "../README.en.md", install: "../guides/INSTALL.en.md", installLink: "guides/INSTALL.en.md",
	quick: "## Get started", installSec: "### Install",
	release: "## Install from a Release file", update: "## Update", uninstall: "## Uninstall",
	updateProse: []string{"(#install-from-a-release-file)", "`ha 0.1.2`"},
	removeProse: []string{"`docs/specs/<goal>/`", "`runs.jsonl`", "`.git/ha/`", "`git rev-parse --absolute-git-dir`", "`git rm -r docs/specs/<goal>`",
		"linked worktree", "It removes the raw check output of every goal in that working tree.", "while no Jaekit check is running"},
}

// rel011RemoveOutput deletes the raw check output under the working tree's own
// Git directory, which is not .git in a linked worktree.
const rel011RemoveOutput = `gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha"` + "\n"

// rel011BeforeSteps includes prerequisites up to the first install command.
func rel011BeforeSteps(t *testing.T, l rel011Lang) string {
	t.Helper()
	text := read(t, l.readme)
	i := strings.Index(text, "\n"+l.quick+"\n")
	if i < 0 {
		t.Fatalf("%s lacks %s", l.readme, l.quick)
	}
	j := strings.Index(text[i:], "```bash")
	if j < 0 {
		t.Fatalf("%s has no installation command", l.readme)
	}
	return text[:i+j]
}

// when ha is missing, Seal points to brew and the Release
// file, says Windows is not supported, tells how to check PATH, and keeps the
// checkout build for Jaekit developers only. The fix ships in a raised seal
// version.
func TestRelease011SealMissingHa(t *testing.T) {
	skill := read(t, "seal/skills/seal/SKILL.md")
	var line string
	for _, l := range strings.Split(skill, "\n") {
		if strings.Contains(l, "If `ha` is missing") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("seal SKILL.md has no instruction for a missing ha")
	}
	requireIn(t, "seal SKILL.md missing-ha instruction", line,
		"record nothing", "stop",
		"`brew install jgoneit/tap/jaekit`", "https://github.com/jgoneit/jaekit/blob/main/guides/INSTALL.md",
		"Windows is not supported", "PATH", "`which -a ha`", "only for developing Jaekit")
	if strings.Contains(skill, "from the jaekit repository") {
		t.Error("seal SKILL.md still sends users to the jaekit repository to install ha")
	}
	// The checkout build may be named, but only in the developer-only clause:
	// after "checkout" and before "only for developing Jaekit".
	if i := strings.Index(line, "go install ./cmd/ha"); i >= 0 {
		if j := strings.Index(line, "only for developing Jaekit"); j < i || !strings.Contains(line[:i], "checkout") {
			t.Error("go install ./cmd/ha appears outside the developer-only clause")
		}
	}
	_, version := frontMatter(t, "seal/skills/seal/SKILL.md")
	if rel011Newer(version, "0.1.4") <= 0 {
		t.Errorf("seal version %s is not raised past 0.1.4", version)
	}
}

func rel011Newer(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x - y
		}
	}
	return len(pa) - len(pb)
}

// before the quick start
// steps, the README says what they need and links the way without Homebrew.
func TestRelease011Prerequisites(t *testing.T) {
	for _, l := range []rel011Lang{rel011Ko, rel011En} {
		visualRequire(t, l.readme+" before installation", rel011BeforeSteps(t, l), "[Homebrew](https://brew.sh)", "Git", "Claude Code", "Codex", l.installLink)
	}
}

// on the first screen or before the quick
// start, the README states Windows as checked: no Seal, Spec unchecked. It
// does not claim Seal, Spec, or WSL work on Windows.
func TestRelease011Windows(t *testing.T) {
	for _, l := range visualLanguages {
		before := visualBeforeInstall(t, l)
		if strings.Contains(l.readme, ".en.md") {
			visualRequire(t, l.readme, before, "Seal Core", "does not support Windows", "Seal cannot run there", "Spec does not need `ha`", "has not been checked yet")
		} else {
			visualRequire(t, l.readme, before, "Windows", "Seal을 쓸 수 없습니다", "Spec은 `ha`가 필요 없지만", "아직 확인하지 않았습니다")
		}
		forbidAll(t, l.readme, "WSL")
	}
}

// rel011Blocks returns a section of the install guide and its shell blocks.
func rel011Blocks(t *testing.T, l rel011Lang, heading string) (string, []string) {
	t.Helper()
	sec := mdSection(t, l.install, heading)
	return sec, onboardBlocks(sec, "bash")
}

func rel011HasBlocks(t *testing.T, name string, blocks []string, want ...string) {
	t.Helper()
	for _, w := range want {
		found := false
		for _, b := range blocks {
			if b == w {
				found = true
			}
		}
		if !found {
			t.Errorf("%s lacks the block %q", name, w)
		}
	}
}

func rel011SameBlocks(t *testing.T, heading string, ko, en []string) {
	t.Helper()
	if strings.Join(ko, "\x00") != strings.Join(en, "\x00") {
		t.Errorf("%s: the English blocks differ from the Korean ones", heading)
	}
}

// the install guide, linked from the README's
// install section, moves ha (brew or Release file) and both hosts' plugins to
// the new tag.
func TestRelease011UpdateGuide(t *testing.T) {
	var ko []string
	for _, l := range []rel011Lang{rel011Ko, rel011En} {
		sec, blocks := rel011Blocks(t, l, l.update)
		rel011HasBlocks(t, l.install+" "+l.update, blocks,
			"brew update\n", "brew upgrade jgoneit/tap/jaekit\n", "ha --version\n",
			"claude plugin marketplace remove jaekit\nclaude plugin marketplace add jgoneit/jaekit#"+rel011Tag+"\nclaude plugin install spec@jaekit\nclaude plugin install seal@jaekit\n",
			"codex plugin marketplace remove jaekit\ncodex plugin marketplace add jgoneit/jaekit@"+rel011Tag+"\ncodex plugin add spec@jaekit\ncodex plugin add seal@jaekit\n")
		requireIn(t, l.install+" "+l.update, sec, l.updateProse...)
		requireIn(t, l.readme+" "+l.installSec, read(t, l.readme),
			"("+l.installLink+"#"+onboardSlug(strings.TrimPrefix(l.update, "## "))+")")
		if l.readme == rel011Ko.readme {
			ko = blocks
		} else {
			rel011SameBlocks(t, l.update, ko, blocks)
		}
	}
}

// the install guide removes ha and both
// hosts' plugins and marketplace, and says what stays in the user's
// repositories and how to delete it. The raw check output is removed from the
// working tree's own Git directory, so it works in a linked worktree too, and
// the guide says that every goal's output in that working tree goes.
func TestRelease011UninstallGuide(t *testing.T) {
	var ko []string
	for _, l := range []rel011Lang{rel011Ko, rel011En} {
		sec, blocks := rel011Blocks(t, l, l.uninstall)
		rel011HasBlocks(t, l.install+" "+l.uninstall, blocks,
			"brew uninstall jgoneit/tap/jaekit\n", "rm ~/.local/bin/ha\n",
			"claude plugin uninstall spec@jaekit\nclaude plugin uninstall seal@jaekit\nclaude plugin marketplace remove jaekit\n",
			"codex plugin remove spec@jaekit\ncodex plugin remove seal@jaekit\ncodex plugin marketplace remove jaekit\n",
			rel011RemoveOutput)
		requireIn(t, l.install+" "+l.uninstall, sec, l.removeProse...)
		forbidAll(t, l.install, "rm -rf .git/ha")
		requireIn(t, l.readme+" "+l.installSec, read(t, l.readme),
			"("+l.installLink+"#"+onboardSlug(strings.TrimPrefix(l.uninstall, "## "))+")")
		if l.readme == rel011Ko.readme {
			ko = blocks
		} else {
			rel011SameBlocks(t, l.uninstall, ko, blocks)
		}
	}
}

// the README, the install guides, the
func TestRelease011Pins(t *testing.T) {
	for _, path := range []string{"../README.md", "../README.en.md", "../guides/INSTALL.md", "../guides/INSTALL.en.md",
		"../.github/ISSUE_TEMPLATE/usage-report.yml", "../.github/ISSUE_TEMPLATE/install-bug.yml"} {
		text := read(t, path)
		// 0.1.2-dev, the version a checkout build prints, is not a release.
		if m := regexp.MustCompile(`[^\n]*\b0\.1\.0([^-\d]|$)[^\n]*`).FindString(text); m != "" {
			t.Errorf("%s still guides 0.1.0: %s", path, strings.TrimSpace(m))
		}
		if strings.HasSuffix(path, ".yml") {
			requireIn(t, path, text, "ha "+rel011Version)
			continue
		}
		if strings.HasPrefix(path, "../README") {
			requireIn(t, path, text, "jgoneit/jaekit#"+rel011Tag)
			guide := "guides/INSTALL.md#codex"
			if strings.Contains(path, ".en.md") {
				guide = "guides/INSTALL.en.md#codex"
			}
			requireIn(t, path, text, "("+guide+")")
		} else {
			requireIn(t, path, text, "jgoneit/jaekit#"+rel011Tag, "jgoneit/jaekit@"+rel011Tag)
		}
	}
	for _, l := range []rel011Lang{rel011Ko, rel011En} {
		requireIn(t, l.readme, read(t, l.readme), "`ha "+rel011Version+"`")
		requireIn(t, l.install+" "+l.release, mdSection(t, l.install, l.release),
			"releases/download/"+rel011Tag, "ha_"+rel011Version+"_")
	}
}
