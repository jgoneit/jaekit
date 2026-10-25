package plugins

// Checks of the release-0-1-8 goal: every place that names the current
// release names v0.1.8, the update check shows ha and spar alike (#162), the
// install and update commands of both hosts install spec, seal and spar from
// the v0.1.8 tag, the direct-install block stops without changing anything
// when the archive lacks spar (#164), and the guides, contract examples,
// skills, manifests and release notes describe the v0.1.8 combination.
//
// Each test observes fixed tokens, lines, links and code blocks of the
// current tree, or runs the guides' direct-install block on synthetic
// archives with a stand-in curl and uname, and opens every failure message
// about its property with the same violation marker, which ha's go-test-json
// adapter reads. A file the test cannot read, a manifest that is not JSON, a
// guide block it cannot find, or a fixture or shell it cannot prepare or
// start fails with a "setup:" message and no marker: that is neither the
// declared violation nor a pass.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// r18Read returns a file of the tree under test. Any read error is a setup
// failure, never the violation.
func r18Read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return string(b)
}

// r18ReadNotes returns the v0.1.8 release notes. The goal adds this file, so
// its absence is the declared violation; any other read error is a setup
// failure.
func r18ReadNotes(t *testing.T, v string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(r18Notes)
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("jaekit:violation=%s %s does not exist", v, r18Notes)
		return "", false
	}
	if err != nil {
		t.Fatalf("setup: read %s: %v", r18Notes, err)
	}
	return string(b), true
}

// r18JSON decodes a manifest of the tree under test; a decode error is a
// setup failure.
func r18JSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(r18Read(t, path)), value); err != nil {
		t.Fatalf("setup: decode %s: %v", path, err)
	}
}

// r18Section returns the lines from heading to the next heading of the same
// or a higher level, skipping fenced code, and whether the heading exists.
func r18Section(text, heading string) ([]string, bool) {
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

// r18Bash returns the contents of the "```bash" blocks among lines, each
// with a newline after every line.
func r18Bash(lines []string) []string {
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
	r18Readme    = "../README.md"
	r18ReadmeKo  = "../README.ko.md"
	r18Install   = "../guides/INSTALL.md"
	r18InstallEn = "../guides/INSTALL.en.md"
	r18Usage     = "../guides/USAGE.md"
	r18UsageEn   = "../guides/USAGE.en.md"
	r18Ops       = "../guides/OPERATIONS.md"
	r18OpsEn     = "../guides/OPERATIONS.en.md"
	r18Windows   = "../guides/WINDOWS.md"
	r18WindowsEn = "../guides/WINDOWS.en.md"
	r18Wsl       = "../guides/WSL.md"
	r18WslEn     = "../guides/WSL.en.md"
	r18SealSkill = "seal/skills/seal/SKILL.md"
	r18SealHa    = "seal/skills/seal/references/ha.md"
	r18SparSkill = "spar/skills/spar/SKILL.md"
	r18Native    = "../tools/ci/native_unix.py"
	r18Notes     = "../tools/release/notes/v0.1.8.md"
)

// The plugin commands every install and update route of the guides and the
// READMEs gives, pinned to the v0.1.8 tag.
const (
	r18CodexInstall = "codex plugin marketplace add jgoneit/jaekit@v0.1.8\n" +
		"codex plugin add spec@jaekit\ncodex plugin add seal@jaekit\ncodex plugin add spar@jaekit\n"
	r18ClaudeAdd = "claude plugin marketplace add jgoneit/jaekit#v0.1.8\n" +
		"claude plugin install spec@jaekit\nclaude plugin install seal@jaekit\nclaude plugin install spar@jaekit\n"
	r18ClaudeInstall = r18ClaudeAdd + "claude plugin list\n"
	r18CodexUpdate   = "codex plugin marketplace remove jaekit\n" + r18CodexInstall
	r18ClaudeUpdate  = "claude plugin marketplace remove jaekit\n" + r18ClaudeAdd
)

// r18UpdateCheck is the version check of the update steps in the READMEs
// and both installation guides (#162).
const r18UpdateCheck = "ha --version\nwhich -a ha\nspar --version\nwhich -a spar\n"

// r18Contains reports each token missing from text.
func r18Contains(t *testing.T, v, path, text string, tokens ...string) {
	t.Helper()
	for _, tok := range tokens {
		if !strings.Contains(text, tok) {
			t.Errorf("jaekit:violation=%s %s lacks %q", v, path, tok)
		}
	}
}

// r18Lacks reports each token that text still holds.
func r18Lacks(t *testing.T, v, path, text string, tokens ...string) {
	t.Helper()
	for _, tok := range tokens {
		if strings.Contains(text, tok) {
			t.Errorf("jaekit:violation=%s %s still holds %q", v, path, tok)
		}
	}
}

// r18HasBlock reports a section that lacks a bash block equal to want.
func r18HasBlock(t *testing.T, v, path, heading, want string) {
	t.Helper()
	lines, ok := r18Section(r18Read(t, path), heading)
	if !ok {
		t.Errorf("jaekit:violation=%s %s lacks the section %q", v, path, heading)
		return
	}
	for _, block := range r18Bash(lines) {
		if block == want {
			return
		}
	}
	t.Errorf("jaekit:violation=%s %s section %q has no bash block equal to %q", v, path, heading, want)
}

// AC-2: the release identity is v0.1.8: the marketplace's product metadata,
// the development identity of both executables and the issue templates'
// version placeholder.
func TestRelease018AC2_ReleaseIdentity(t *testing.T) {
	const v = "release-identity-stale"
	var market struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	r18JSON(t, "../.claude-plugin/marketplace.json", &market)
	if market.Metadata.Version != "0.1.8" {
		t.Errorf("jaekit:violation=%s marketplace metadata version is %q, not 0.1.8", v, market.Metadata.Version)
	}
	for _, path := range []string{"../cmd/ha/main.go", "../cmd/spar/main.go"} {
		r18Contains(t, v, path, r18Read(t, path), `var version = "0.1.8-dev"`)
	}
	for _, path := range []string{"../.github/ISSUE_TEMPLATE/install-bug.yml", "../.github/ISSUE_TEMPLATE/usage-report.yml"} {
		r18Contains(t, v, path, r18Read(t, path), "placeholder: ha 0.1.8\n")
	}
}

// AC-2: current-release mentions, host pins, download links and the Seal
// skill's install route name v0.1.8, and no v0.1.7 pin is left there.
func TestRelease018AC2_PinsAndDownloads(t *testing.T) {
	const v = "release-pin-stale"
	r18Contains(t, v, r18Readme, r18Read(t, r18Readme), "The current release is **v0.1.8**")
	r18Contains(t, v, r18ReadmeKo, r18Read(t, r18ReadmeKo), "현재 배포판은 **v0.1.8**입니다")
	for _, path := range []string{r18Readme, r18ReadmeKo, r18Install, r18InstallEn} {
		text := r18Read(t, path)
		r18Contains(t, v, path, text, "jgoneit/jaekit#v0.1.8", "jgoneit/jaekit@v0.1.8")
		r18Lacks(t, v, path, text, "jaekit#v0.1.7", "jaekit@v0.1.7")
	}
	for _, path := range []string{r18Install, r18InstallEn} {
		text := r18Read(t, path)
		r18Contains(t, v, path, text,
			"https://github.com/jgoneit/jaekit/releases/tag/v0.1.8",
			`url="https://github.com/jgoneit/jaekit/releases/download/v0.1.8"`,
			`name="ha_0.1.8_${os}_${arch}"`,
			`share="$HOME/.local/share/jaekit/0.1.8"`,
			`package_root="$HOME/.local/share/jaekit/0.1.8"`,
			"current=0.1.8\n")
		r18Lacks(t, v, path, text, "releases/download/v0.1.7", "ha_0.1.7_", "jaekit/0.1.7\"", "current=0.1.7")
	}
	for _, c := range []struct{ path, current string }{
		{r18Wsl, "현재 공개 배포판은 `v0.1.8`입니다"},
		{r18WslEn, "The current public release is `v0.1.8`"},
	} {
		text := r18Read(t, c.path)
		r18Contains(t, v, c.path, text, c.current, "ha_0.1.8_linux_amd64.tar.gz", "~/.local/share/jaekit/0.1.8",
			"https://github.com/jgoneit/jaekit/releases/tag/v0.1.8")
		r18Lacks(t, v, c.path, text, "ha_0.1.7_linux_amd64", "jaekit/0.1.7", "releases/tag/v0.1.7")
	}
	// Links to the current release's files; the historical check_core.py
	// links of core-capabilities.md stay at their own releases.
	for _, path := range []string{r18SealSkill, r18SealHa, "seal/skills/seal/references/bundle.md", "../contracts/goal-docs.md", "../contracts/role-card.md"} {
		r18Lacks(t, v, path, r18Read(t, path), "jaekit/blob/v0.1.7/")
	}
	for _, path := range []string{r18SealSkill, r18SealHa} {
		r18Contains(t, v, path, r18Read(t, path), "https://github.com/jgoneit/jaekit/blob/v0.1.8/guides/INSTALL.md")
	}
	r18Contains(t, v, "contracts/core-capabilities.md", r18Read(t, "../contracts/core-capabilities.md"),
		"Seal에 포함된 [check_core.py](https://github.com/jgoneit/jaekit/blob/v0.1.8/plugins/seal/skills/seal/scripts/check_core.py)")
}

// AC-2: the Windows guides say v0.1.8 has no Windows files, and the first
// release with Windows files and its block versions move past 0.1.8.
func TestRelease018AC2_WindowsPlanPast018(t *testing.T) {
	const v = "windows-plan-stale"
	for _, c := range []struct {
		path   string
		tokens []string
	}{
		{r18Windows, []string{"현재 공개 배포판 `v0.1.8`에 Windows 파일이 있다는 뜻이 아닙니다",
			"현재 배포판 `v0.1.8`에도 Windows 파일이 없습니다", "예정 `0.1.9`", "공개 배포판 v0.1.8에는 Windows 파일이 없고"}},
		{r18WindowsEn, []string{"It does not mean the current public release, `v0.1.8`, includes Windows files",
			"the current release, `v0.1.8`, has no Windows files", "planned `0.1.9`", "Public release v0.1.8 has no Windows files"}},
	} {
		text := r18Read(t, c.path)
		r18Contains(t, v, c.path, text, c.tokens...)
		if n := strings.Count(text, "$version = '0.1.9'"); n != 2 {
			t.Errorf("jaekit:violation=%s %s sets $version = '0.1.9' in %d blocks, not 2", v, c.path, n)
		}
		r18Lacks(t, v, c.path, text, "$version = '0.1.8'", "예정 `0.1.8`", "planned `0.1.8`")
	}
}

// AC-2: the tests that pin the current release expect v0.1.8.
func TestRelease018AC2_PinTestsExpect018(t *testing.T) {
	const v = "pin-test-stale"
	for path, tokens := range map[string][]string{
		"compatibility_test.go":                       {`marketplace.Metadata.Version != "0.1.8"`},
		"release_0_1_7_test.go":                       {`market.Metadata.Version != "0.1.8"`, "\"$version = '0.1.9'\""},
		"release_0_1_1_test.go":                       {`= "v0.1.8"`, `rel011Version = "0.1.8"`},
		"readme_onboarding_test.go":                   {"claude plugin marketplace add jgoneit/jaekit#v0.1.8"},
		"release_guide_test.go":                       {"https://github.com/jgoneit/jaekit/releases/tag/v0.1.8"},
		"spar_onboarding_test.go":                     {`"https://github.com/jgoneit/jaekit/blob/v0.1.8/guides/INSTALL.md"`},
		"../tools/release/test_archives.py":           {`VERSION = "0.1.8"`},
		"../tools/release/test_spar_archives.py":      {`VERSION = "0.1.8"`},
		"../tools/release/test_install.py":            {`NAME = "ha_0.1.8_linux_amd64"`},
		"../tools/release/test_install_cleanup.py":    {`CURRENT = "0.1.8"`},
		"../tools/review-checks/test_installation.py": {`CURRENT = "0.1.8"`},
		"../tools/contract-checks/authoring_check.py": {`metadata.get("version") != "0.1.8"`},
		"../tools/windows/docs_check.py":              {`"0.1.8-dev"`},
	} {
		r18Contains(t, v, path, r18Read(t, path), tokens...)
	}
}

// AC-3: the update steps of the READMEs and of both installation guides
// check ha and spar with the same four commands: each version and the file
// PATH selects (#162). Every version-check block in those update sections is
// that block.
func TestRelease018AC3_UpdateCheckShowsSpar(t *testing.T) {
	const v = "update-check-without-spar"
	for _, c := range []struct{ path, heading string }{
		{r18Readme, "### Update"},
		{r18ReadmeKo, "### 업데이트"},
		{r18Install, "## 업데이트"},
		{r18InstallEn, "## Update"},
	} {
		lines, ok := r18Section(r18Read(t, c.path), c.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, c.path, c.heading)
			continue
		}
		found := false
		for _, block := range r18Bash(lines) {
			if !strings.Contains(block, "--version") {
				continue
			}
			if block == r18UpdateCheck {
				found = true
				continue
			}
			t.Errorf("jaekit:violation=%s %s section %q has a version check %q other than %q", v, c.path, c.heading, block, r18UpdateCheck)
		}
		if !found {
			t.Errorf("jaekit:violation=%s %s section %q has no bash block equal to %q", v, c.path, c.heading, r18UpdateCheck)
		}
	}
}

// AC-4: the install, update and migration commands of both hosts install
// spec, seal and spar from the v0.1.8 marketplace.
func TestRelease018AC4_GuideHostBlocks(t *testing.T) {
	const v = "host-block-stale"
	for _, g := range []struct{ path, codex, claude, update, move string }{
		{r18Install, "### Codex", "### Claude Code", "## 업데이트", "## 로컬 경로 등록에서 옮기기"},
		{r18InstallEn, "### Codex", "### Claude Code", "## Update", "## Move from a local path registration"},
	} {
		r18HasBlock(t, v, g.path, g.codex, r18CodexInstall)
		r18HasBlock(t, v, g.path, g.claude, r18ClaudeInstall)
		r18HasBlock(t, v, g.path, g.update, r18CodexUpdate)
		r18HasBlock(t, v, g.path, g.update, r18ClaudeUpdate)
		r18HasBlock(t, v, g.path, g.move, r18CodexUpdate)
		r18HasBlock(t, v, g.path, g.move, r18ClaudeUpdate)
	}
}

// AC-4: the READMEs give the same install and update commands as the guide.
func TestRelease018AC4_ReadmeHostBlocks(t *testing.T) {
	const v = "readme-block-stale"
	for _, r := range []struct{ path, install, update string }{
		{r18Readme, "### Install", "### Update"},
		{r18ReadmeKo, "### 설치", "### 업데이트"},
	} {
		r18HasBlock(t, v, r.path, r.install, r18CodexInstall)
		r18HasBlock(t, v, r.path, r.install, r18ClaudeInstall)
		r18HasBlock(t, v, r.path, r.update, r18CodexUpdate)
		r18HasBlock(t, v, r.path, r.update, r18ClaudeUpdate)
	}
}

// AC-4: Homebrew and the direct install both say they install ha and spar
// together. This already holds on the base, so no AC target names it; it
// runs with the shared Go tests.
func TestRelease018BinariesTogether(t *testing.T) {
	const v = "spar-binary-unstated"
	for _, c := range []struct{ path, heading, token string }{
		{r18Install, "### ha 설치", "brew는 `ha`와 `spar`를 함께 설치합니다"},
		{r18InstallEn, "### Install ha", "Homebrew installs `ha` and `spar` together"},
		{r18Install, "## Release 파일로 설치", "`ha`와 `spar`를 `~/.local/bin/ha`와 `~/.local/bin/spar`에 둡니다"},
		{r18InstallEn, "## Install from a Release file", "puts `ha` and `spar` at `~/.local/bin/ha` and `~/.local/bin/spar`"},
		{r18ReadmeKo, "### 설치", "brew는 `ha`와 `spar`를 함께 설치합니다"},
		{r18Readme, "### Install", "Homebrew installs `ha` and `spar` together"},
		{r18ReadmeKo, "### 설치", "`ha`와 `spar`를 `~/.local/bin/ha`와 `~/.local/bin/spar`에 둡니다"},
		{r18Readme, "### Install", "puts `ha` and `spar` at `~/.local/bin/ha` and `~/.local/bin/spar`"},
	} {
		lines, ok := r18Section(r18Read(t, c.path), c.heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, c.path, c.heading)
			continue
		}
		r18Contains(t, v, c.path+" "+c.heading, strings.Join(lines, "\n"), c.token)
	}
}

// r18Guides are the two installation guides and their direct-install
// section headings.
var r18Guides = []struct{ path, heading string }{
	{r18Install, "## Release 파일로 설치"},
	{r18InstallEn, "## Install from a Release file"},
}

// r18Fixture is one direct-install run: an isolated HOME, a stand-in bin
// directory that serves only the generated archive and checksums, and the
// guide's block as written.
type r18Fixture struct {
	home, dist, fake, tmp, block, name string
	bash                               string
}

var r18NamePattern = regexp.MustCompile(`name="ha_([0-9]+\.[0-9]+\.[0-9]+)_\$\{os\}_\$\{arch\}"`)

// r18NewFixture reads the guide's single direct-install block and prepares
// the isolated directories and the stand-in curl and uname. The archive
// name follows the block's own version, so the same fixture runs on any
// tree. Every failure here is a setup failure.
func r18NewFixture(t *testing.T, path, heading string) *r18Fixture {
	t.Helper()
	lines, ok := r18Section(r18Read(t, path), heading)
	if !ok {
		t.Fatalf("setup: %s lacks the section %q", path, heading)
	}
	blocks := r18Bash(lines)
	if len(blocks) != 1 {
		t.Fatalf("setup: %s section %q has %d bash blocks, not 1", path, heading, len(blocks))
	}
	m := r18NamePattern.FindStringSubmatch(blocks[0])
	if m == nil {
		t.Fatalf("setup: %s direct-install block has no recognized archive name", path)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("setup: bash: %v", err)
	}
	// The block runs with PATH set to the stand-ins, /usr/bin and /bin. A
	// tool it needs that is not there could not start, which is a setup
	// failure, not the block's behavior.
	for _, tools := range [][]string{{"tar"}, {"grep"}, {"mktemp"}, {"cp"}, {"mv"}, {"chmod"}, {"mkdir"},
		{"find"}, {"diff"}, {"rm"}, {"sha256sum", "shasum"}} {
		if !r18SystemTool(tools...) {
			t.Fatalf("setup: none of %v is an executable in /usr/bin or /bin", tools)
		}
	}
	root := t.TempDir()
	f := &r18Fixture{home: filepath.Join(root, "home"), dist: filepath.Join(root, "dist"),
		fake: filepath.Join(root, "fake"), tmp: filepath.Join(root, "tmp"), block: blocks[0],
		name: "ha_" + m[1] + "_linux_amd64", bash: bash}
	for _, dir := range []string{f.home, f.dist, f.fake, f.tmp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	scripts := map[string]string{
		"uname": "#!/bin/sh\nif [ \"$1\" = -s ]; then echo Linux; else echo x86_64; fi\n",
		"curl": "#!/bin/sh\nfor a; do url=$a; done\nfile=${url##*/}\ncase \"$file\" in\n" +
			"  \"$R18_ARCHIVE\" | checksums.txt) cp \"$R18_DIST/$file\" \"$file\" ;;\n  *) exit 22 ;;\nesac\n",
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(f.fake, name), []byte(body), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return f
}

// r18SystemTool reports whether one of names is an executable file in
// /usr/bin or /bin.
func r18SystemTool(names ...string) bool {
	for _, name := range names {
		for _, dir := range []string{"/usr/bin", "/bin"} {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
				return true
			}
		}
	}
	return false
}

// archive writes the synthetic release archive and its checksums: ha, spar
// when withSpar, the root documents and one file in each public folder the
// block copies.
func (f *r18Fixture) archive(t *testing.T, withSpar bool) map[string][]byte {
	t.Helper()
	files := map[string][]byte{"ha": []byte("synthetic ha 0.1.8\n"), "LICENSE": []byte("synthetic license\n"),
		"README.md": []byte("synthetic readme\n"), "README.ko.md": []byte("synthetic Korean readme\n"),
		"README.en.md":                    []byte("synthetic English readme\n"),
		"tools/check-result-reference.py": []byte("# synthetic\n"), "examples/check-result/input.txt": []byte("synthetic\n"),
		"guides/INSTALL.md": []byte("synthetic\n"), "contracts/run-record.md": []byte("synthetic\n"),
		"assets/readme/x.txt": []byte("synthetic\n")}
	if withSpar {
		files["spar"] = []byte("synthetic spar 0.1.8\n")
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		mode := int64(0o644)
		if name == "ha" || name == "spar" {
			mode = 0o755
		}
		hdr := &tar.Header{Name: f.name + "/" + name, Mode: mode, Size: int64(len(files[name])), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := tw.Write(files[name]); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	if err := os.WriteFile(filepath.Join(f.dist, f.name+".tar.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.dist, "checksums.txt"), []byte(hex.EncodeToString(sum[:])+"  "+f.name+".tar.gz\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return files
}

// prior installs earlier ha and spar files where the block puts them.
func (f *r18Fixture) prior(t *testing.T) {
	t.Helper()
	bin := filepath.Join(f.home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, body := range map[string]string{"ha": "earlier ha\n", "spar": "earlier spar\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
}

// run executes the block with bash in the isolated HOME and returns the
// exit code and the standard error. A shell that cannot start or a run that
// does not end within a minute is a setup failure.
func (f *r18Fixture) run(t *testing.T) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bash, "-c", f.block)
	cmd.Dir = f.tmp
	cmd.Env = []string{"HOME=" + f.home, "PATH=" + f.fake + string(os.PathListSeparator) + "/usr/bin:/bin",
		"TMPDIR=" + f.tmp, "LC_ALL=C", "R18_DIST=" + f.dist, "R18_ARCHIVE=" + f.name + ".tar.gz"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		t.Fatalf("setup: direct-install block did not end: %v", ctx.Err())
	case errors.As(err, &exit) && exit.Exited():
		return exit.ExitCode(), stderr.String()
	case err != nil:
		t.Fatalf("setup: run bash: %v", err)
	}
	return 0, stderr.String()
}

// snapshot lists every entry under HOME with its type, mode and content
// digest.
func (f *r18Fixture) snapshot(t *testing.T) map[string]string {
	t.Helper()
	entries := map[string]string{}
	err := filepath.WalkDir(f.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(f.home, path)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(path)
			entries[rel] = "link " + target
		case info.IsDir():
			entries[rel] = fmt.Sprintf("dir %o", info.Mode().Perm())
		default:
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			entries[rel] = fmt.Sprintf("file %o %x", info.Mode().Perm(), sum)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: snapshot %s: %v", f.home, err)
	}
	return entries
}

// r18Diff names the entries that differ between two snapshots.
func r18Diff(before, after map[string]string) []string {
	var changed []string
	for name, value := range after {
		if before[name] != value {
			changed = append(changed, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

// AC-5: when the extracted archive lacks spar, each guide's direct-install
// block replaces neither executable, changes nothing under HOME, names spar
// in its error message and exits nonzero, both over an earlier install and
// on a fresh HOME (#164). A control run of the same block on an archive with
// spar comes first: when it does not install, this environment cannot run
// the block, which is a setup failure rather than the violation.
func TestRelease018AC5_MissingSparStops(t *testing.T) {
	const v = "spar-optional-install"
	for _, g := range r18Guides {
		control := r18NewFixture(t, g.path, g.heading)
		control.archive(t, true)
		if code, out := control.run(t); code != 0 {
			t.Fatalf("setup: %s block exited %d even with spar in the archive: %q", g.path, code, out)
		}
		for _, earlier := range []bool{true, false} {
			f := r18NewFixture(t, g.path, g.heading)
			f.archive(t, false)
			if earlier {
				f.prior(t)
			}
			before := f.snapshot(t)
			code, out := f.run(t)
			if code == 0 {
				t.Errorf("jaekit:violation=%s %s block (earlier install %v) exited 0 for an archive without spar", v, g.path, earlier)
			}
			if !strings.Contains(out, "spar") {
				t.Errorf("jaekit:violation=%s %s block (earlier install %v) did not name spar on standard error %q", v, g.path, earlier, out)
			}
			if changed := r18Diff(before, f.snapshot(t)); len(changed) > 0 {
				t.Errorf("jaekit:violation=%s %s block (earlier install %v) changed %v for an archive without spar", v, g.path, earlier, changed)
			}
		}
	}
}

// AC-5: the native Unix CI observer builds spar into its synthetic archives
// and runs the guides' block on an archive without spar too, so its check
// passes the success and the failure path of the block.
func TestRelease018AC5_NativeArchivesCarrySpar(t *testing.T) {
	const v = "native-archive-without-spar"
	r18Contains(t, v, r18Native, r18Read(t, r18Native), `"./cmd/spar"`, `"/missing-spar/"`, `"missing_spar_rejection_preserves"`)
}

// AC-5: with spar in the archive, each guide's block puts the archive's ha
// and spar in place as executables and leaves no staging file. This already
// holds on the base, so no AC target names it.
func TestRelease018DirectInstallPutsBoth(t *testing.T) {
	const v = "direct-install-incomplete"
	for _, g := range r18Guides {
		f := r18NewFixture(t, g.path, g.heading)
		files := f.archive(t, true)
		f.prior(t)
		if code, out := f.run(t); code != 0 {
			t.Errorf("jaekit:violation=%s %s block exited %d with spar in the archive: %q", v, g.path, code, out)
			continue
		}
		bin := filepath.Join(f.home, ".local", "bin")
		for _, name := range []string{"ha", "spar"} {
			path := filepath.Join(bin, name)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 {
				t.Errorf("jaekit:violation=%s %s block left %s as %v (%v), not an executable file", v, g.path, name, info, err)
				continue
			}
			if b, _ := os.ReadFile(path); !bytes.Equal(b, files[name]) {
				t.Errorf("jaekit:violation=%s %s block did not install the archive's %s", v, g.path, name)
			}
		}
		entries, err := os.ReadDir(bin)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				t.Errorf("jaekit:violation=%s %s block left the staging file %s", v, g.path, e.Name())
			}
		}
	}
}

// AC-9: the guides, contract examples, skills, manifests and release notes
// state the v0.1.8 combination: ha 0.1.8, spar 0.1.8, Spec 0.1.13, Seal
// 0.1.23 and the Spar plugin 0.1.4, with v0.1.7 kept as the previous row.
func TestRelease018AC9_CombinationStated(t *testing.T) {
	const v = "combination-unstated"
	for _, c := range []struct {
		path, row, previous, source string
	}{
		{r18Install, "| 현재 배포판 `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |",
			"| 이전 배포판 `v0.1.7` | `ha 0.1.7`, `spar 0.1.7` | 0.1.13 | 0.1.22 | 0.1.3 |", "| 소스 빌드 | `ha 0.1.8-dev`, `spar 0.1.8-dev` |"},
		{r18InstallEn, "| Current release `v0.1.8` | `ha 0.1.8`, `spar 0.1.8` | 0.1.13 | 0.1.23 | 0.1.4 |",
			"| Previous release `v0.1.7` | `ha 0.1.7`, `spar 0.1.7` | 0.1.13 | 0.1.22 | 0.1.3 |", "| Source build | `ha 0.1.8-dev`, `spar 0.1.8-dev` |"},
	} {
		text := r18Read(t, c.path)
		found := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			for _, prefix := range []string{c.row, c.previous, c.source} {
				if strings.HasPrefix(line, prefix) {
					found[prefix] = true
				}
			}
		}
		for _, prefix := range []string{c.row, c.previous, c.source} {
			if !found[prefix] {
				t.Errorf("jaekit:violation=%s %s has no table row starting %q", v, c.path, prefix)
			}
		}
	}
	for path, tokens := range map[string][]string{
		r18Usage:                            {"현재 배포판 v0.1.8은 ha 0.1.8·spar 0.1.8·spec 0.1.13·seal 0.1.23·spar 플러그인 0.1.4 조합입니다"},
		r18UsageEn:                          {"The current release v0.1.8 combines ha 0.1.8, spar 0.1.8, spec 0.1.13, seal 0.1.23 and the spar plugin 0.1.4"},
		r18Ops:                              {"v0.1.8(ha 0.1.8·spar 0.1.8·spec 0.1.13·seal 0.1.23·spar 플러그인 0.1.4)"},
		r18OpsEn:                            {"v0.1.8 (ha 0.1.8, spar 0.1.8, spec 0.1.13, seal 0.1.23, spar plugin 0.1.4)"},
		"../contracts/core-capabilities.md": {`"ha_version": "0.1.8"`, "v0.1.8의 Core `0.1.8`·Seal `0.1.23`"},
		"../contracts/review-gate.md":       {`"spar_version": "0.1.8"`},
		"../examples/README.md":             {"현재 배포판 v0.1.8의 Seal 0.1.23에도 포함되며"},
		r18SealHa:                           {"The commands below describe release v0.1.8: ha 0.1.8, spar 0.1.8 and Seal 0.1.23."},
		r18SealSkill:                        {"version: \"0.1.23\"\n"},
		r18SparSkill:                        {"version: \"0.1.4\"\n"},
	} {
		r18Contains(t, v, path, r18Read(t, path), tokens...)
	}
	if notes, ok := r18ReadNotes(t, v); ok {
		r18Contains(t, v, r18Notes, notes, "`ha 0.1.8`", "`spar 0.1.8`", "Spec `0.1.13`", "Seal `0.1.23`", "Spar `0.1.4`",
			"macOS·Linux의 arm64·amd64", "이번 릴리스에는 Windows 파일이 없습니다", "`ha cost`",
			"이번 변경:", "확인한 범위:", "확인하지 않은 범위:")
	}
	for path, want := range map[string]string{
		"seal/.claude-plugin/plugin.json": "0.1.23", "seal/.codex-plugin/plugin.json": "0.1.23",
		"spar/.claude-plugin/plugin.json": "0.1.4", "spar/.codex-plugin/plugin.json": "0.1.4",
	} {
		var manifest struct {
			Version string `json:"version"`
		}
		r18JSON(t, path, &manifest)
		if manifest.Version != want {
			t.Errorf("jaekit:violation=%s %s version is %q, not %s", v, path, manifest.Version, want)
		}
	}
	var market struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	r18JSON(t, "../.claude-plugin/marketplace.json", &market)
	want := map[string]string{"spec": "0.1.13", "seal": "0.1.23", "spar": "0.1.4"}
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
