package plugins

// Checks of the release-0-1-7 goal: every place that names the current
// release names v0.1.7, the install and update commands of both hosts install
// the spec, seal and spar plugins from that tag, both install routes say they
// put ha and spar in place together, and the guides, contract examples, Seal
// skill and release notes describe the v0.1.7 combination.
//
// Each test observes fixed tokens, lines, links and code blocks of the
// current tree, never an open category of sentences, and opens every failure
// message about its property with the same violation marker, which ha's
// go-test-json adapter reads. A file the test cannot read, or a manifest that
// is not JSON, fails with a "setup:" message and no marker: that is neither
// the declared violation nor a pass.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// r17Read returns a file of the tree under test. Any read error is a setup
// failure, never the violation.
func r17Read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return string(b)
}

// r17ReadNotes returns the v0.1.7 release notes. The goal adds this file, so
// its absence is the declared violation; any other read error is a setup
// failure.
func r17ReadNotes(t *testing.T, v string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(r17Notes)
	if os.IsNotExist(err) {
		t.Errorf("jaekit:violation=%s %s does not exist", v, r17Notes)
		return "", false
	}
	if err != nil {
		t.Fatalf("setup: read %s: %v", r17Notes, err)
	}
	return string(b), true
}

// r17JSON decodes a manifest of the tree under test; a decode error is a
// setup failure.
func r17JSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(r17Read(t, path)), value); err != nil {
		t.Fatalf("setup: decode %s: %v", path, err)
	}
}

// r17Section returns the lines from heading to the next heading of the same
// or a higher level, skipping fenced code, and whether the heading exists.
func r17Section(text, heading string) ([]string, bool) {
	lines := strings.Split(text, "\n")
	level := strings.Index(heading, " ")
	start, fenced := -1, false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if fenced {
			continue
		}
		if start < 0 {
			if line == heading {
				start = i
			}
			continue
		}
		if i == start {
			continue
		}
		if n := strings.Index(line, " "); n > 0 && n <= level && strings.Trim(line[:n], "#") == "" {
			return lines[start:i], true
		}
	}
	if start < 0 {
		return nil, false
	}
	return lines[start:], true
}

// r17Bash returns the contents of the "```bash" blocks among lines, each
// with a newline after every line.
func r17Bash(lines []string) []string {
	var blocks []string
	var cur strings.Builder
	in := false
	for _, line := range lines {
		switch {
		case !in && line == "```bash":
			in = true
			cur.Reset()
		case in && line == "```":
			in = false
			blocks = append(blocks, cur.String())
		case in:
			cur.WriteString(line + "\n")
		}
	}
	return blocks
}

const (
	r17Readme    = "../README.md"
	r17ReadmeKo  = "../README.ko.md"
	r17Install   = "../guides/INSTALL.md"
	r17InstallEn = "../guides/INSTALL.en.md"
	r17Spar      = "../guides/SPAR.md"
	r17SparEn    = "../guides/SPAR.en.md"
	r17Usage     = "../guides/USAGE.md"
	r17UsageEn   = "../guides/USAGE.en.md"
	r17Ops       = "../guides/OPERATIONS.md"
	r17OpsEn     = "../guides/OPERATIONS.en.md"
	r17Windows   = "../guides/WINDOWS.md"
	r17WindowsEn = "../guides/WINDOWS.en.md"
	r17SealSkill = "seal/skills/seal/SKILL.md"
	r17SealHa    = "seal/skills/seal/references/ha.md"
	r17Notes     = "../tools/release/notes/v0.1.7.md"
)

// The plugin commands every install and update route of the guides and the
// READMEs gives, pinned to the v0.1.7 tag.
const (
	r17CodexInstall = "codex plugin marketplace add jgoneit/jaekit@v0.1.7\n" +
		"codex plugin add spec@jaekit\ncodex plugin add seal@jaekit\ncodex plugin add spar@jaekit\n"
	r17ClaudeAdd = "claude plugin marketplace add jgoneit/jaekit#v0.1.7\n" +
		"claude plugin install spec@jaekit\nclaude plugin install seal@jaekit\nclaude plugin install spar@jaekit\n"
	r17ClaudeInstall = r17ClaudeAdd + "claude plugin list\n"
	r17CodexUpdate   = "codex plugin marketplace remove jaekit\n" + r17CodexInstall
	r17ClaudeUpdate  = "claude plugin marketplace remove jaekit\n" + r17ClaudeAdd
)

// r17Contains reports each token missing from text.
func r17Contains(t *testing.T, v, path, text string, tokens ...string) {
	t.Helper()
	for _, tok := range tokens {
		if !strings.Contains(text, tok) {
			t.Errorf("jaekit:violation=%s %s lacks %q", v, path, tok)
		}
	}
}

// r17Lacks reports each token that text still holds.
func r17Lacks(t *testing.T, v, path, text string, tokens ...string) {
	t.Helper()
	for _, tok := range tokens {
		if strings.Contains(text, tok) {
			t.Errorf("jaekit:violation=%s %s still holds %q", v, path, tok)
		}
	}
}

// r17HasBlock reports a section that lacks a bash block equal to want.
func r17HasBlock(t *testing.T, v, path, heading, want string) {
	t.Helper()
	lines, ok := r17Section(r17Read(t, path), heading)
	if !ok {
		t.Errorf("jaekit:violation=%s %s lacks the section %q", v, path, heading)
		return
	}
	for _, block := range r17Bash(lines) {
		if block == want {
			return
		}
	}
	t.Errorf("jaekit:violation=%s %s section %q has no bash block equal to %q", v, path, heading, want)
}

// AC-2: the release identity is v0.1.7: the marketplace's product metadata,
// the development identity of both executables and the issue templates'
// version placeholder.
func TestRelease017AC2_ReleaseIdentity(t *testing.T) {
	const v = "release-identity-stale"
	var market struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	r17JSON(t, "../.claude-plugin/marketplace.json", &market)
	if market.Metadata.Version != "0.1.7" {
		t.Errorf("jaekit:violation=%s marketplace metadata version is %q, not 0.1.7", v, market.Metadata.Version)
	}
	for _, path := range []string{"../cmd/ha/main.go", "../cmd/spar/main.go"} {
		r17Contains(t, v, path, r17Read(t, path), `var version = "0.1.7-dev"`)
	}
	for _, path := range []string{"../.github/ISSUE_TEMPLATE/install-bug.yml", "../.github/ISSUE_TEMPLATE/usage-report.yml"} {
		r17Contains(t, v, path, r17Read(t, path), "placeholder: ha 0.1.7\n")
	}
}

// AC-2: current-release mentions, host pins, download links and the Seal
// skill's install route name v0.1.7, and no v0.1.6 pin is left there.
func TestRelease017AC2_PinsAndDownloads(t *testing.T) {
	const v = "release-pin-stale"
	r17Contains(t, v, r17Readme, r17Read(t, r17Readme), "The current release is **v0.1.7**")
	r17Contains(t, v, r17ReadmeKo, r17Read(t, r17ReadmeKo), "현재 배포판은 **v0.1.7**입니다")
	for _, path := range []string{r17Readme, r17ReadmeKo, r17Install, r17InstallEn} {
		text := r17Read(t, path)
		r17Contains(t, v, path, text, "jgoneit/jaekit#v0.1.7", "jgoneit/jaekit@v0.1.7")
		r17Lacks(t, v, path, text, "jaekit#v0.1.6", "jaekit@v0.1.6")
	}
	for _, path := range []string{r17Install, r17InstallEn} {
		text := r17Read(t, path)
		r17Contains(t, v, path, text,
			"https://github.com/jgoneit/jaekit/releases/tag/v0.1.7",
			`url="https://github.com/jgoneit/jaekit/releases/download/v0.1.7"`,
			`name="ha_0.1.7_${os}_${arch}"`,
			`share="$HOME/.local/share/jaekit/0.1.7"`,
			`package_root="$HOME/.local/share/jaekit/0.1.7"`,
			"current=0.1.7\n")
		r17Lacks(t, v, path, text, "releases/download/v0.1.6", "ha_0.1.6_", "jaekit/0.1.6\"", "current=0.1.6")
	}
	// Links to the current release's files; the historical check_core.py
	// link of core-capabilities.md stays at v0.1.6.
	for _, path := range []string{r17SealSkill, r17SealHa, "seal/skills/seal/references/bundle.md", "../contracts/goal-docs.md", "../contracts/role-card.md"} {
		r17Lacks(t, v, path, r17Read(t, path), "jaekit/blob/v0.1.6/")
	}
	for _, path := range []string{r17SealSkill, r17SealHa} {
		r17Contains(t, v, path, r17Read(t, path), "https://github.com/jgoneit/jaekit/blob/v0.1.7/guides/INSTALL.md")
	}
	r17Contains(t, v, "contracts/core-capabilities.md", r17Read(t, "../contracts/core-capabilities.md"),
		"Seal에 포함된 [check_core.py](https://github.com/jgoneit/jaekit/blob/v0.1.7/plugins/seal/skills/seal/scripts/check_core.py)")
}

// AC-2: the sentences that said "the release after v0.1.6" name v0.1.7.
func TestRelease017AC2_NextReleaseNamed(t *testing.T) {
	const v = "next-release-unnamed"
	for _, path := range []string{r17Readme, r17ReadmeKo, r17Install, r17InstallEn, r17Spar, r17SparEn, r17Usage, r17UsageEn, r17Ops, r17OpsEn} {
		r17Lacks(t, v, path, r17Read(t, path),
			"v0.1.6 다음 릴리스", "release after v0.1.6", "releases after v0.1.6", "그 릴리스의 Seal", "That release's Seal")
	}
	for _, c := range []struct {
		path   string
		tokens []string
	}{
		{r17ReadmeKo, []string{"v0.1.7부터 `standard`·`strict` 목표에는 Spar 실행 파일과 Spar 플러그인이 필요"}},
		{r17Readme, []string{"From v0.1.7, `standard` and `strict` goals need the Spar executable and the Spar plugin"}},
		{r17Install, []string{"v0.1.7부터 `standard`·`strict` 목표에는 Spar 실행 파일과 Spar 플러그인이 필요",
			"v0.1.7부터는 Homebrew formula와 Release 파일이 `ha`와 `spar`를 함께 설치"}},
		{r17InstallEn, []string{"From v0.1.7, `standard` and `strict` goals need the Spar executable and the Spar plugin",
			"From v0.1.7, installing with Homebrew or from a Release file installs `ha` and `spar` together"}},
		{r17Spar, []string{"v0.1.7부터 Seal은 새 목표를 `run-rules/5`로 시작", "`spar`는 v0.1.7부터 릴리스 파일과 Homebrew formula에 `ha`와 함께 들어 있습니다"}},
		{r17SparEn, []string{"From v0.1.7, Seal starts new goals under `run-rules/5`", "in the release files and the Homebrew formula from v0.1.7"}},
		{r17Usage, []string{"v0.1.7부터 Seal(0.1.21 이상)은 새 목표를"}},
		{r17UsageEn, []string{"From v0.1.7, Seal (0.1.21 or later) starts new goals"}},
	} {
		r17Contains(t, v, c.path, r17Read(t, c.path), c.tokens...)
	}
}

// AC-2: the Windows guides say v0.1.7 has no Windows files, and the first
// release with Windows files and its block versions move past 0.1.7.
func TestRelease017AC2_WindowsPlanPast017(t *testing.T) {
	const v = "windows-plan-stale"
	for _, c := range []struct {
		path   string
		tokens []string
	}{
		{r17Windows, []string{"현재 공개 배포판 `v0.1.7`에 Windows 파일이 있다는 뜻이 아닙니다",
			"현재 배포판 `v0.1.7`에도 Windows 파일이 없습니다", "예정 `0.1.8`", "공개 배포판 v0.1.7에는 Windows 파일이 없고"}},
		{r17WindowsEn, []string{"It does not mean the current public release, `v0.1.7`, includes Windows files",
			"the current release, `v0.1.7`, has no Windows files", "planned `0.1.8`", "Public release v0.1.7 has no Windows files"}},
	} {
		text := r17Read(t, c.path)
		r17Contains(t, v, c.path, text, c.tokens...)
		if n := strings.Count(text, "$version = '0.1.8'"); n != 2 {
			t.Errorf("jaekit:violation=%s %s sets $version = '0.1.8' in %d blocks, not 2", v, c.path, n)
		}
		r17Lacks(t, v, c.path, text, "$version = '0.1.7'", "예정 `0.1.7`", "planned `0.1.7`")
	}
}

// AC-2: the tests that pin the current release expect v0.1.7.
func TestRelease017AC2_PinTestsExpect017(t *testing.T) {
	const v = "pin-test-stale"
	for path, tokens := range map[string][]string{
		"compatibility_test.go":                       {`marketplace.Metadata.Version != "0.1.7"`},
		"release_0_1_1_test.go":                       {`= "v0.1.7"`, `rel011Version = "0.1.7"`},
		"readme_onboarding_test.go":                   {"claude plugin marketplace add jgoneit/jaekit#v0.1.7"},
		"release_guide_test.go":                       {"https://github.com/jgoneit/jaekit/releases/tag/v0.1.7"},
		"../tools/release/test_archives.py":           {`VERSION = "0.1.7"`},
		"../tools/release/test_spar_archives.py":      {`VERSION = "0.1.7"`},
		"../tools/release/test_install.py":            {`NAME = "ha_0.1.7_linux_amd64"`},
		"../tools/release/test_install_cleanup.py":    {`CURRENT = "0.1.7"`},
		"../tools/review-checks/test_installation.py": {`CURRENT = "0.1.7"`},
		"../tools/contract-checks/authoring_check.py": {`metadata.get("version") != "0.1.7"`},
	} {
		r17Contains(t, v, path, r17Read(t, path), tokens...)
	}
}

// AC-3: the install, update and migration commands of both hosts install
// spec, seal and spar from the v0.1.7 marketplace.
func TestRelease017AC3_GuideHostBlocks(t *testing.T) {
	const v = "host-block-without-spar"
	for _, g := range []struct{ path, codex, claude, update, move string }{
		{r17Install, "### Codex", "### Claude Code", "## 업데이트", "## 로컬 경로 등록에서 옮기기"},
		{r17InstallEn, "### Codex", "### Claude Code", "## Update", "## Move from a local path registration"},
	} {
		r17HasBlock(t, v, g.path, g.codex, r17CodexInstall)
		r17HasBlock(t, v, g.path, g.claude, r17ClaudeInstall)
		r17HasBlock(t, v, g.path, g.update, r17CodexUpdate)
		r17HasBlock(t, v, g.path, g.update, r17ClaudeUpdate)
		r17HasBlock(t, v, g.path, g.move, r17CodexUpdate)
		r17HasBlock(t, v, g.path, g.move, r17ClaudeUpdate)
	}
}

// AC-3: the READMEs give the same install and update commands as the guide.
func TestRelease017AC3_ReadmeHostBlocks(t *testing.T) {
	const v = "readme-block-differs"
	for _, r := range []struct{ path, install, update string }{
		{r17Readme, "### Install", "### Update"},
		{r17ReadmeKo, "### 설치", "### 업데이트"},
	} {
		r17HasBlock(t, v, r.path, r.install, r17CodexInstall)
		r17HasBlock(t, v, r.path, r.install, r17ClaudeInstall)
		r17HasBlock(t, v, r.path, r.update, r17CodexUpdate)
		r17HasBlock(t, v, r.path, r.update, r17ClaudeUpdate)
	}
}

// AC-3: Homebrew and the direct install both say they install ha and spar
// together.
func TestRelease017AC3_BinariesTogether(t *testing.T) {
	const v = "spar-binary-unstated"
	for _, c := range []struct{ path, heading, token string }{
		{r17Install, "### ha 설치", "brew는 `ha`와 `spar`를 함께 설치합니다"},
		{r17InstallEn, "### Install ha", "Homebrew installs `ha` and `spar` together"},
		{r17Install, "## Release 파일로 설치", "`ha`와 `spar`를 `~/.local/bin/ha`와 `~/.local/bin/spar`에 둡니다"},
		{r17InstallEn, "## Install from a Release file", "puts `ha` and `spar` at `~/.local/bin/ha` and `~/.local/bin/spar`"},
		{r17ReadmeKo, "### 설치", "brew는 `ha`와 `spar`를 함께 설치합니다"},
		{r17Readme, "### Install", "Homebrew installs `ha` and `spar` together"},
		{r17ReadmeKo, "### 설치", "`ha`와 `spar`를 `~/.local/bin/ha`와 `~/.local/bin/spar`에 둡니다"},
		{r17Readme, "### Install", "puts `ha` and `spar` at `~/.local/bin/ha` and `~/.local/bin/spar`"},
	} {
		lines, ok := r17Section(r17Read(t, c.path), c.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, c.path, c.heading)
			continue
		}
		r17Contains(t, v, c.path+" "+c.heading, strings.Join(lines, "\n"), c.token)
	}
}

// AC-9: the guides, contract example, Seal skill, manifests and release
// notes state the v0.1.7 combination: ha 0.1.7, spar 0.1.7, Spec 0.1.13,
// Seal 0.1.22 and the Spar plugin 0.1.3.
func TestRelease017AC9_CombinationStated(t *testing.T) {
	const v = "combination-unstated"
	for _, c := range []struct {
		path, row, previous string
	}{
		{r17Install, "| 현재 배포판 `v0.1.7` | `ha 0.1.7`, `spar 0.1.7` | 0.1.13 | 0.1.22 | 0.1.3 |", "| 이전 배포판 `v0.1.6` | `ha 0.1.6` | 0.1.12 | 0.1.17 | — |"},
		{r17InstallEn, "| Current release `v0.1.7` | `ha 0.1.7`, `spar 0.1.7` | 0.1.13 | 0.1.22 | 0.1.3 |", "| Previous release `v0.1.6` | `ha 0.1.6` | 0.1.12 | 0.1.17 | — |"},
	} {
		text := r17Read(t, c.path)
		found := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			for _, prefix := range []string{c.row, c.previous} {
				if strings.HasPrefix(line, prefix) {
					found[prefix] = true
				}
			}
		}
		for _, prefix := range []string{c.row, c.previous} {
			if !found[prefix] {
				t.Errorf("jaekit:violation=%s %s has no table row starting %q", v, c.path, prefix)
			}
		}
	}
	for path, tokens := range map[string][]string{
		r17Usage:                            {"현재 배포판 v0.1.7은 ha 0.1.7·spar 0.1.7·spec 0.1.13·seal 0.1.22·spar 플러그인 0.1.3 조합입니다"},
		r17UsageEn:                          {"The current release v0.1.7 combines ha 0.1.7, spar 0.1.7, spec 0.1.13, seal 0.1.22 and the spar plugin 0.1.3"},
		r17Ops:                              {"v0.1.7(ha 0.1.7·spar 0.1.7·spec 0.1.13·seal 0.1.22·spar 플러그인 0.1.3)"},
		r17OpsEn:                            {"v0.1.7 (ha 0.1.7, spar 0.1.7, spec 0.1.13, seal 0.1.22, spar plugin 0.1.3)"},
		"../contracts/core-capabilities.md": {`"ha_version": "0.1.7"`, "v0.1.7의 Core `0.1.7`·Seal `0.1.22`"},
		"../contracts/review-gate.md":       {`"spar_version": "0.1.7"`},
		r17SealHa:                           {"The commands below describe release v0.1.7: ha 0.1.7, spar 0.1.7 and Seal 0.1.22."},
		r17SealSkill:                        {"version: \"0.1.22\"\n"},
	} {
		r17Contains(t, v, path, r17Read(t, path), tokens...)
	}
	if notes, ok := r17ReadNotes(t, v); ok {
		r17Contains(t, v, r17Notes, notes, "`ha 0.1.7`", "`spar 0.1.7`", "Spec `0.1.13`", "Seal `0.1.22`", "Spar `0.1.3`",
			"macOS·Linux의 arm64·amd64", "이번 릴리스에는 Windows 파일이 없습니다", "이번 변경:", "확인한 범위:", "확인하지 않은 범위:")
	}
	for _, path := range []string{"seal/.claude-plugin/plugin.json", "seal/.codex-plugin/plugin.json"} {
		var manifest struct {
			Version string `json:"version"`
		}
		r17JSON(t, path, &manifest)
		if manifest.Version != "0.1.22" {
			t.Errorf("jaekit:violation=%s %s version is %q, not 0.1.22", v, path, manifest.Version)
		}
	}
	var market struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	r17JSON(t, "../.claude-plugin/marketplace.json", &market)
	want := map[string]string{"spec": "0.1.13", "seal": "0.1.22", "spar": "0.1.3"}
	for _, p := range market.Plugins {
		if w, ok := want[p.Name]; ok {
			if p.Version != w {
				t.Errorf("jaekit:violation=%s marketplace lists %s %q, not %s", v, p.Name, p.Version, w)
			}
			delete(want, p.Name)
		}
	}
	for name := range want {
		t.Errorf("jaekit:violation=%s marketplace lacks the %s plugin", v, name)
	}
}

// AC-9: no guide calls what v0.1.7 ships missing from the current release.
func TestRelease017AC9_ShippedNotDeferred(t *testing.T) {
	const v = "shipped-feature-deferred"
	for path, stale := range map[string][]string{
		r17Install:   {"아직 Spar가 없습니다"},
		r17InstallEn: {"do not contain Spar yet"},
		r17Usage:     {"배포판 v0.1.6의 `ha`는 이 시작을 받지 않으므로"},
		r17UsageEn:   {"The `ha` of release v0.1.6 does not accept this start"},
	} {
		r17Lacks(t, v, path, r17Read(t, path), stale...)
	}
	r17Contains(t, v, r17Usage, r17Read(t, r17Usage), "v0.1.7의 `ha`는 이 지원을 모두 갖췄습니다")
	r17Contains(t, v, r17UsageEn, r17Read(t, r17UsageEn), "The `ha` of release v0.1.7 has all of this support")
}

// The README's completion report example follows the Seal this release
// ships: since Seal 0.1.20 the report is `ha report` output pasted once
// `ha done` records completion, never `ha status` or `ha done` output. It
// reads fixed labels between the section heading and the next one. No AC
// target names it, so it runs with the shared Go tests.
func TestRelease017ReadmeCompletionReport(t *testing.T) {
	const v = "readme-report-stale"
	for _, c := range []struct {
		path, heading, next string
		labels              []string
	}{
		{"../README.md", "### The completion report\n", "\n### The other files\n",
			[]string{"Changed: ", "Checked: ", "To review: ", "Remaining limits: ", "Next step: ", "### Evidence per condition", "### Output of unmet conditions"}},
		{"../README.ko.md", "### 완료 보고\n", "\n### 다른 파일\n",
			[]string{"바뀐 것: ", "확인한 것: ", "직접 볼 것: ", "남은 한계: ", "다음 행동: ", "### 조건별 근거", "### 미충족 조건의 원문 위치"}},
	} {
		text := r17Read(t, c.path)
		start := strings.Index(text, c.heading)
		end := -1
		if start >= 0 {
			end = strings.Index(text[start:], c.next)
		}
		if start < 0 || end < 0 {
			t.Errorf("jaekit:violation=%s %s lacks the section %q followed by %q", v, c.path, strings.TrimSpace(c.heading), strings.TrimSpace(c.next))
			continue
		}
		section := text[start : start+end]
		r17Contains(t, v, c.path, section, append([]string{"`ha report`"}, c.labels...)...)
		r17Lacks(t, v, c.path, section, "ha status: docs/specs/", "### Unmet", "skill: seal 0.1.17", "ha: 0.1.6")
	}
}
