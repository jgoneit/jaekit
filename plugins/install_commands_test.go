package plugins

// Property: install, update and uninstall commands
// The install guides and READMEs give both hosts' install, update and move
// commands for spec, seal and spar, the same commands in both languages, and
// a direct-install block that puts ha and spar in place or changes nothing.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"
)

const (
	icReadme    = "../README.md"
	icReadmeKo  = "../README.ko.md"
	icInstall   = "../guides/INSTALL.md"
	icInstallEn = "../guides/INSTALL.en.md"
)

// The commands every install, update and move route of the guides and the
// READMEs gives.
const (
	icBrew         = "brew install jgoneit/tap/jaekit\n"
	icCodexInstall = "codex plugin marketplace add jgoneit/jaekit@v0.1.9\n" +
		"codex plugin add spec@jaekit\ncodex plugin add seal@jaekit\ncodex plugin add spar@jaekit\n"
	icClaudeAdd = "claude plugin marketplace add jgoneit/jaekit#v0.1.9\n" +
		"claude plugin install spec@jaekit\nclaude plugin install seal@jaekit\nclaude plugin install spar@jaekit\n"
	icClaudeInstall = icClaudeAdd + "claude plugin list\n"
	icCodexUpdate   = "codex plugin marketplace remove jaekit\n" + icCodexInstall
	icClaudeUpdate  = "claude plugin marketplace remove jaekit\n" + icClaudeAdd
	// The version check of ha and spar: each version and the file PATH
	// selects.
	icVersionCheck = "ha --version\nwhich -a ha\nspar --version\nwhich -a spar\n"
)

// icSection returns the lines from heading to the next heading of the same
// or a higher level, skipping fenced code, and whether the heading exists.
func icSection(text, heading string) ([]string, bool) {
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
		if n := strings.Index(line, " "); n > 0 && n <= level && strings.Trim(line[:n], "#") == "" {
			return lines[start:i], true
		}
	}
	if start < 0 {
		return nil, false
	}
	return lines[start:], true
}

// icBash returns the contents of the column-0 "```bash" blocks among lines,
// each with a newline after every line.
func icBash(lines []string) []string {
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

// icBlocks returns the fenced code blocks of the given language, with the
// fence indentation removed.
func icBlocks(text, lang string) []string {
	var blocks []string
	var cur []string
	indent, in := "", false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case !in && trimmed == "```"+lang:
			in, indent, cur = true, line[:len(line)-len(trimmed)], nil
		case in && trimmed == "```":
			blocks = append(blocks, strings.Join(cur, "\n")+"\n")
			in = false
		case in:
			cur = append(cur, strings.TrimPrefix(line, indent))
		}
	}
	return blocks
}

// icHasBlock reports a section that lacks a bash block equal to want.
func icHasBlock(t *testing.T, path, heading, want string) {
	t.Helper()
	lines, ok := icSection(read(t, path), heading)
	if !ok {
		t.Errorf("%s lacks the section %q", path, heading)
		return
	}
	if !slices.Contains(icBash(lines), want) {
		t.Errorf("%s section %q has no bash block equal to %q", path, heading, want)
	}
}

// icHasBlockPrefix reports a document with no bash block that starts with
// the given lines.
func icHasBlockPrefix(t *testing.T, path, text, prefix string) {
	t.Helper()
	for _, b := range icBlocks(text, "bash") {
		if strings.HasPrefix(b, prefix) {
			return
		}
	}
	t.Errorf("%s lacks a bash block starting %q", path, prefix)
}

// The install guides give each host's install command, and their update and
// move sections both hosts' re-registration, for spec, seal and spar.
func TestInstallCommandsGuideHostBlocks(t *testing.T) {
	for _, g := range []struct{ path, codex, claude, update, move string }{
		{icInstall, "### Codex", "### Claude Code", "## 업데이트", "## 로컬 경로 등록에서 옮기기"},
		{icInstallEn, "### Codex", "### Claude Code", "## Update", "## Move from a local path registration"},
	} {
		icHasBlock(t, g.path, g.codex, icCodexInstall)
		icHasBlock(t, g.path, g.claude, icClaudeInstall)
		icHasBlock(t, g.path, g.update, icCodexUpdate)
		icHasBlock(t, g.path, g.update, icClaudeUpdate)
		icHasBlock(t, g.path, g.move, icCodexUpdate)
		icHasBlock(t, g.path, g.move, icClaudeUpdate)
	}
}

// The READMEs give the same install and update commands as the guides.
func TestReadmeHostInstallBlocks(t *testing.T) {
	for _, r := range []struct{ path, install, update string }{
		{icReadme, "### Install", "### Update"},
		{icReadmeKo, "### 설치", "### 업데이트"},
	} {
		icHasBlock(t, r.path, r.install, icCodexInstall)
		icHasBlock(t, r.path, r.install, icClaudeInstall)
		icHasBlock(t, r.path, r.update, icCodexUpdate)
		icHasBlock(t, r.path, r.update, icClaudeUpdate)
	}
}

// The update steps of the READMEs and both install guides check ha and spar
// with the same four commands, and every version-check block there is that
// block.
func TestInstallCommandsUpdateCheckShowsSpar(t *testing.T) {
	for _, c := range []struct{ path, heading string }{
		{icReadme, "### Update"},
		{icReadmeKo, "### 업데이트"},
		{icInstall, "## 업데이트"},
		{icInstallEn, "## Update"},
	} {
		lines, ok := icSection(read(t, c.path), c.heading)
		if !ok {
			t.Errorf("%s lacks the section %q", c.path, c.heading)
			continue
		}
		found := false
		for _, block := range icBash(lines) {
			if !strings.Contains(block, "--version") {
				continue
			}
			if block == icVersionCheck {
				found = true
				continue
			}
			t.Errorf("%s section %q has a version check %q other than %q", c.path, c.heading, block, icVersionCheck)
		}
		if !found {
			t.Errorf("%s section %q has no bash block equal to %q", c.path, c.heading, icVersionCheck)
		}
	}
}

// The Korean install guide gives the brew, version-check, host install and
// move blocks.
func TestInstallCommandsInstallGuideBlocks(t *testing.T) {
	doc := read(t, icInstall)
	for _, b := range []string{icBrew, icVersionCheck, icClaudeAdd, icCodexInstall, icClaudeUpdate, icCodexUpdate} {
		icHasBlockPrefix(t, icInstall, doc, b)
	}
}

// The install guides' Spar section gives the spar version check and both
// hosts' spar plugin command.
func TestInstallCommandsSparSection(t *testing.T) {
	for _, path := range []string{icInstall, icInstallEn} {
		lines, ok := icSection(read(t, path), "### Spar")
		if !ok {
			t.Errorf("%s lacks the section %q", path, "### Spar")
			continue
		}
		requireIn(t, path+" section ### Spar", strings.Join(lines, "\n"),
			"spar --version", "codex plugin add spar@jaekit", "claude plugin install spar@jaekit")
	}
}

// icFoldRequire reports each term that text lacks, ignoring case.
func icFoldRequire(t *testing.T, name, text string, terms ...string) {
	t.Helper()
	for _, term := range terms {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(term)) {
			t.Errorf("%s lacks %q", name, term)
		}
	}
}

// icHeadingSection returns the text from the first occurrence of heading
// followed by a newline up to the next heading line of the same or a higher
// level, counting heading lines inside code blocks too.
func icHeadingSection(t *testing.T, path, heading string) string {
	t.Helper()
	text := read(t, path)
	start := strings.Index(text, heading+"\n")
	if start < 0 {
		t.Fatalf("%s lacks %s", path, heading)
	}
	level := strings.IndexByte(heading, ' ')
	for _, m := range regexp.MustCompile(`(?m)^#{1,6} `).FindAllStringIndex(text[start+len(heading)+1:], -1) {
		if m[1]-m[0]-1 <= level {
			return text[start : start+len(heading)+1+m[0]]
		}
	}
	return text[start:]
}

// Each README, together with the install and usage guides it links, gives
// the install, version-check, host plugin, upgrade, uninstall and
// marketplace-removal commands; case is ignored. When a README links the
// install guide's Codex section directly, that guide's ha install and
// version-check sections hold their commands.
func TestReadmeAndGuidesGiveInstallCommands(t *testing.T) {
	for _, l := range []struct{ readme, install, usage, haHeading, versionHeading string }{
		{icReadmeKo, icInstall, "../guides/USAGE.md", "### ha 설치", "### 버전 확인"},
		{icReadme, icInstallEn, "../guides/USAGE.en.md", "### Install ha", "### Check the version"},
	} {
		readme := read(t, l.readme)
		text := readme
		for _, path := range []string{l.install, l.usage} {
			if strings.Contains(readme, strings.TrimPrefix(path, "../")) {
				text += "\n" + read(t, path)
			}
		}
		icFoldRequire(t, l.readme+" and its linked guides", text,
			"brew install jgoneit/tap/jaekit", "ha --version", "ha 0.1.9", "which -a ha",
			"claude plugin marketplace add jgoneit/jaekit#v0.1.9", "claude plugin install spec@jaekit", "claude plugin install seal@jaekit", "claude plugin install spar@jaekit",
			"codex plugin marketplace add jgoneit/jaekit@v0.1.9", "codex plugin add spec@jaekit", "codex plugin add seal@jaekit", "codex plugin add spar@jaekit",
			"brew upgrade jgoneit/tap/jaekit", "brew uninstall jgoneit/tap/jaekit", "marketplace remove jaekit")
		if strings.Contains(readme, strings.TrimPrefix(l.install, "../")+"#codex") {
			icFoldRequire(t, l.install+" section "+l.haHeading, icHeadingSection(t, l.install, l.haHeading), "brew install jgoneit/tap/jaekit")
			icFoldRequire(t, l.install+" section "+l.versionHeading, icHeadingSection(t, l.install, l.versionHeading), "ha --version", "which -a ha", "ha 0.1.9")
		}
	}
}

// icLabeledBlocks returns the column-0 bash blocks of text in order, each
// with the nearest non-blank line before its fence.
func icLabeledBlocks(text string) [][2]string {
	var out [][2]string
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		if lines[i] != "```bash" {
			continue
		}
		label := ""
		for j := i - 1; j >= 0; j-- {
			if strings.TrimSpace(lines[j]) != "" {
				label = lines[j]
				break
			}
		}
		var body []string
		for i++; i < len(lines) && lines[i] != "```"; i++ {
			body = append(body, lines[i])
		}
		out = append(out, [2]string{label, strings.Join(body, "\n") + "\n"})
	}
	return out
}

// icGuideBlock returns the guide's bash block in heading that starts with
// prefix.
func icGuideBlock(t *testing.T, path, heading, prefix string) string {
	t.Helper()
	for _, block := range icBlocks(mdSection(t, path, heading), "bash") {
		if strings.HasPrefix(block, prefix) {
			return block
		}
	}
	t.Fatalf("%s %s has no block starting %q", path, heading, prefix)
	return ""
}

// Every command the README shows is the install guide's own command for the
// same step and host. The README install and update sections list, in order,
// exactly the guide's blocks for each step, and each host's block follows
// that host's label. Every bash block anywhere in the README is one of the
// guide's blocks. Within the README, a code fence is only a column-0 ```bash,
// ```text, ```markdown or ```mermaid opening and its ``` closing, so no
// command escapes the comparison in a bare, tilde, indented or other-language
// fence. Inline code and prose are outside this check.
func TestReadmeInstallCommandsMatchGuide(t *testing.T) {
	for _, c := range []struct{ readme, install, update, guide, guideHa, guideCheck, guideUpdate string }{
		{icReadmeKo, "### 설치", "### 업데이트", icInstall, "### ha 설치", "### 버전 확인", "## 업데이트"},
		{icReadme, "### Install", "### Update", icInstallEn, "### Install ha", "### Check the version", "## Update"},
	} {
		check := icGuideBlock(t, c.guide, c.guideCheck, "ha --version")
		want := map[string][][2]string{
			c.install: {
				{"", icGuideBlock(t, c.guide, c.guideHa, "brew install")},
				{"", check},
				{"**Claude Code**", icGuideBlock(t, c.guide, "### Claude Code", "claude plugin marketplace add")},
				{"**Codex**", icGuideBlock(t, c.guide, "### Codex", "codex plugin marketplace add")},
			},
			c.update: {
				{"", icGuideBlock(t, c.guide, c.guideUpdate, "brew update")},
				{"", icGuideBlock(t, c.guide, c.guideUpdate, "brew upgrade")},
				{"", check},
				{"**Claude Code**", icGuideBlock(t, c.guide, c.guideUpdate, "claude plugin marketplace remove")},
				{"**Codex**", icGuideBlock(t, c.guide, c.guideUpdate, "codex plugin marketplace remove")},
			},
		}
		for _, heading := range []string{c.install, c.update} {
			got := icLabeledBlocks(mdSection(t, c.readme, heading))
			if len(got) != len(want[heading]) {
				t.Errorf("%s %s has %d command blocks, want %d", c.readme, heading, len(got), len(want[heading]))
				continue
			}
			for i, w := range want[heading] {
				if got[i][1] != w[1] {
					t.Errorf("%s %s block %d differs from %s: got %q, want %q", c.readme, heading, i+1, c.guide, got[i][1], w[1])
				}
				if w[0] != "" && got[i][0] != w[0] {
					t.Errorf("%s %s block %d follows %q, want the label %q", c.readme, heading, i+1, got[i][0], w[0])
				}
			}
		}
		guideAll := icBlocks(read(t, c.guide), "bash")
		for _, block := range icBlocks(read(t, c.readme), "bash") {
			if !slices.Contains(guideAll, block) {
				t.Errorf("%s block is not one of %s's commands: %q", c.readme, c.guide, block)
			}
		}
		open := false
		for n, line := range strings.Split(read(t, c.readme), "\n") {
			trimmed := strings.TrimLeft(line, " \t")
			if !strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "~~~") {
				continue
			}
			switch {
			case trimmed != line || strings.HasPrefix(trimmed, "~~~"):
				t.Errorf("%s:%d uses the fence %q; fences start at column 0 with backticks", c.readme, n+1, line)
			case !open && (line == "```bash" || line == "```text" || line == "```markdown" || line == "```mermaid"):
				open = true
			case open && line == "```":
				open = false
			default:
				t.Errorf("%s:%d uses the fence %q; only ```bash, ```text, ```markdown and ```mermaid open a block", c.readme, n+1, line)
			}
		}
		if open {
			t.Errorf("%s leaves a code fence open", c.readme)
		}
	}
}

var icPlaceholder = regexp.MustCompile(`<[^>]+>`)

// icCommands returns the brew, host plugin, ha version and which lines of
// the bash blocks of text, with placeholders blanked.
func icCommands(text string) string {
	var out []string
	inBash := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "```bash" {
			inBash = true
			continue
		}
		if line == "```" {
			inBash = false
			continue
		}
		if !inBash {
			continue
		}
		line = icPlaceholder.ReplaceAllString(line, "<>")
		if strings.HasPrefix(line, "brew ") || strings.HasPrefix(line, "claude plugin ") || strings.HasPrefix(line, "codex plugin ") || line == "ha --version" || line == "which -a ha" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// The Korean and English install guides have the same bash blocks, apart
// from placeholders and the messages the Release-file block prints, and the
// READMEs and install guides give the same install commands in both
// languages.
func TestInstallCommandsSameInBothLanguages(t *testing.T) {
	norm := func(s string) string {
		var keep []string
		for _, line := range strings.Split(icPlaceholder.ReplaceAllString(s, "<>"), "\n") {
			if !strings.Contains(line, `echo "`) {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	}
	kb, eb := icBlocks(read(t, icInstall), "bash"), icBlocks(read(t, icInstallEn), "bash")
	if len(kb) != len(eb) {
		t.Errorf("%s has %d bash blocks, %s has %d", icInstall, len(kb), icInstallEn, len(eb))
	} else {
		for i := range kb {
			if norm(kb[i]) != norm(eb[i]) {
				t.Errorf("%s and %s bash block %d differs:\n%s\n%s", icInstall, icInstallEn, i+1, kb[i], eb[i])
			}
		}
	}
	for _, pair := range [][2]string{{icReadmeKo, icReadme}, {icInstall, icInstallEn}} {
		if icCommands(read(t, pair[0])) != icCommands(read(t, pair[1])) {
			t.Errorf("commands differ between %s and %s", pair[0], pair[1])
		}
	}
}

// icDirectGuides are the two install guides and their direct-install
// section headings.
var icDirectGuides = []struct{ path, heading string }{
	{icInstall, "## Release 파일로 설치"},
	{icInstallEn, "## Install from a Release file"},
}

// icFixture is one direct-install run: an isolated HOME, a stand-in bin
// directory that serves only the generated archive and checksums, and the
// guide's block as written.
type icFixture struct {
	home, dist, fake, tmp, block, name string
	bash                               string
}

var icNamePattern = regexp.MustCompile(`name="ha_([0-9]+\.[0-9]+\.[0-9]+)_\$\{os\}_\$\{arch\}"`)

// icNewFixture reads the guide's single direct-install block and prepares
// the isolated directories and the stand-in curl and uname. The archive name
// follows the block's own version. Every failure here fails the test before
// the block runs.
func icNewFixture(t *testing.T, path, heading string) *icFixture {
	t.Helper()
	lines, ok := icSection(read(t, path), heading)
	if !ok {
		t.Fatalf("setup: %s lacks the section %q", path, heading)
	}
	blocks := icBash(lines)
	if len(blocks) != 1 {
		t.Fatalf("setup: %s section %q has %d bash blocks, not 1", path, heading, len(blocks))
	}
	m := icNamePattern.FindStringSubmatch(blocks[0])
	if m == nil {
		t.Fatalf("setup: %s direct-install block has no recognized archive name", path)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("setup: bash: %v", err)
	}
	// The block runs with PATH set to the stand-ins, /usr/bin and /bin. A
	// tool it needs that is not there could not start, which fails the setup
	// rather than counting as the block's behavior.
	for _, tools := range [][]string{{"tar"}, {"grep"}, {"mktemp"}, {"cp"}, {"mv"}, {"chmod"}, {"mkdir"},
		{"find"}, {"diff"}, {"rm"}, {"sha256sum", "shasum"}} {
		if !icSystemTool(tools...) {
			t.Fatalf("setup: none of %v is an executable in /usr/bin or /bin", tools)
		}
	}
	root := t.TempDir()
	f := &icFixture{home: filepath.Join(root, "home"), dist: filepath.Join(root, "dist"),
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
			"  \"$IC_ARCHIVE\" | checksums.txt) cp \"$IC_DIST/$file\" \"$file\" ;;\n  *) exit 22 ;;\nesac\n",
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(f.fake, name), []byte(body), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return f
}

// icSystemTool reports whether one of names is an executable file in
// /usr/bin or /bin.
func icSystemTool(names ...string) bool {
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
func (f *icFixture) archive(t *testing.T, withSpar bool) map[string][]byte {
	t.Helper()
	files := map[string][]byte{"ha": []byte("synthetic ha\n"), "LICENSE": []byte("synthetic license\n"),
		"README.md": []byte("synthetic readme\n"), "README.ko.md": []byte("synthetic Korean readme\n"),
		"README.en.md":                    []byte("synthetic English readme\n"),
		"tools/check-result-reference.py": []byte("# synthetic\n"), "examples/check-result/input.txt": []byte("synthetic\n"),
		"guides/INSTALL.md": []byte("synthetic\n"), "contracts/run-record.md": []byte("synthetic\n"),
		"assets/readme/x.txt": []byte("synthetic\n")}
	if withSpar {
		files["spar"] = []byte("synthetic spar\n")
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
func (f *icFixture) prior(t *testing.T) {
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

// run executes the block with bash in the isolated HOME and returns the exit
// code and the standard error. A shell that cannot start or a run that does
// not end within a minute fails the setup.
func (f *icFixture) run(t *testing.T) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bash, "-c", f.block)
	cmd.Dir = f.tmp
	cmd.Env = []string{"HOME=" + f.home, "PATH=" + f.fake + string(os.PathListSeparator) + "/usr/bin:/bin",
		"TMPDIR=" + f.tmp, "LC_ALL=C", "IC_DIST=" + f.dist, "IC_ARCHIVE=" + f.name + ".tar.gz"}
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
func (f *icFixture) snapshot(t *testing.T) map[string]string {
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

// icDiff names the entries that differ between two snapshots.
func icDiff(before, after map[string]string) []string {
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

// When the extracted archive lacks spar, each guide's direct-install block
// replaces neither executable, changes nothing under HOME, names spar in its
// error message and exits nonzero, both over an earlier install and on a
// fresh HOME. A control run of the same block on an archive with spar comes
// first: when it does not install, this environment cannot run the block,
// which fails the setup rather than counting as the block's behavior.
func TestInstallCommandsDirectInstallStopsWithoutSpar(t *testing.T) {
	for _, g := range icDirectGuides {
		control := icNewFixture(t, g.path, g.heading)
		control.archive(t, true)
		if code, out := control.run(t); code != 0 {
			t.Fatalf("setup: %s block exited %d even with spar in the archive: %q", g.path, code, out)
		}
		for _, earlier := range []bool{true, false} {
			f := icNewFixture(t, g.path, g.heading)
			f.archive(t, false)
			if earlier {
				f.prior(t)
			}
			before := f.snapshot(t)
			code, out := f.run(t)
			if code == 0 {
				t.Errorf("%s block (earlier install %v) exited 0 for an archive without spar", g.path, earlier)
			}
			if !strings.Contains(out, "spar") {
				t.Errorf("%s block (earlier install %v) did not name spar on standard error %q", g.path, earlier, out)
			}
			if changed := icDiff(before, f.snapshot(t)); len(changed) > 0 {
				t.Errorf("%s block (earlier install %v) changed %v for an archive without spar", g.path, earlier, changed)
			}
		}
	}
}

// With spar in the archive, each guide's direct-install block puts the
// archive's ha and spar in place as executables and leaves no staging file.
func TestInstallCommandsDirectInstallPutsBoth(t *testing.T) {
	for _, g := range icDirectGuides {
		f := icNewFixture(t, g.path, g.heading)
		files := f.archive(t, true)
		f.prior(t)
		if code, out := f.run(t); code != 0 {
			t.Errorf("%s block exited %d with spar in the archive: %q", g.path, code, out)
			continue
		}
		bin := filepath.Join(f.home, ".local", "bin")
		for _, name := range []string{"ha", "spar"} {
			path := filepath.Join(bin, name)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 {
				t.Errorf("%s block left %s as %v (%v), not an executable file", g.path, name, info, err)
				continue
			}
			if b, _ := os.ReadFile(path); !bytes.Equal(b, files[name]) {
				t.Errorf("%s block did not install the archive's %s", g.path, name)
			}
		}
		entries, err := os.ReadDir(bin)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				t.Errorf("%s block left the staging file %s", g.path, e.Name())
			}
		}
	}
}

// The three update, uninstall and reference-check tests are moved here unchanged.

// onboardBlocks returns the fenced code blocks of the given language, with the
// fence indentation removed.
func onboardBlocks(text, lang string) []string {
	var blocks []string
	var cur []string
	indent, in := "", false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case !in && trimmed == "```"+lang:
			in, indent, cur = true, line[:len(line)-len(trimmed)], nil
		case in && trimmed == "```":
			blocks = append(blocks, strings.Join(cur, "\n")+"\n")
			in = false
		case in:
			cur = append(cur, strings.TrimPrefix(line, indent))
		}
	}
	return blocks
}

// onboardSlug is GitHub's heading anchor: lower case; letters, digits, marks,
// spaces, hyphens, and underscores kept; spaces become hyphens.
func onboardSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

const rel011Tag = "v0.1.9"

type rel011Lang struct {
	readme, install            string // paths from this package
	installLink                string // the install guide, as the README links it
	quick, installSec          string // README headings
	release, update, uninstall string // install guide headings
	updateProse, removeProse   []string
}

var rel011Ko = rel011Lang{
	readme: "../README.ko.md", install: "../guides/INSTALL.md", installLink: "guides/INSTALL.md",
	quick: "## 시작하기", installSec: "### 설치",
	release: "## Release 파일로 설치", update: "## 업데이트", uninstall: "## 제거",
	updateProse: []string{"(#release-파일로-설치)", "`ha 0.1.9`"},
	removeProse: []string{"`runs.jsonl`", "`.git/ha/`", "`git rev-parse --absolute-git-dir`", "`git rm -r docs/specs/<goal>`"},
}

var rel011En = rel011Lang{
	readme: "../README.md", install: "../guides/INSTALL.en.md", installLink: "guides/INSTALL.en.md",
	quick: "## Get started", installSec: "### Install",
	release: "## Install from a Release file", update: "## Update", uninstall: "## Uninstall",
	updateProse: []string{"(#install-from-a-release-file)", "`ha 0.1.9`"},
	removeProse: []string{"`runs.jsonl`", "`.git/ha/`", "`git rev-parse --absolute-git-dir`", "`git rm -r docs/specs/<goal>`"},
}

// rel011RemoveOutput deletes the raw check output under the working tree's own
// Git directory, which is not .git in a linked worktree. It removes only
// ha/runs, so anything else under the Git directory stays.
const rel011RemoveOutput = `gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha/runs"` + "\n"

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
			"brew update\n", "brew upgrade jgoneit/tap/jaekit\n", "ha --version\nwhich -a ha\nspar --version\nwhich -a spar\n",
			"claude plugin marketplace remove jaekit\nclaude plugin marketplace add jgoneit/jaekit#"+rel011Tag+"\nclaude plugin install spec@jaekit\nclaude plugin install seal@jaekit\nclaude plugin install spar@jaekit\n",
			"codex plugin marketplace remove jaekit\ncodex plugin marketplace add jgoneit/jaekit@"+rel011Tag+"\ncodex plugin add spec@jaekit\ncodex plugin add seal@jaekit\ncodex plugin add spar@jaekit\n")
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
// hosts' plugins and marketplace, and names what stays in the user's
// repositories and how to delete it. The raw check output is removed from the
// working tree's own Git directory, so it works in a linked worktree too.
func TestRelease011UninstallGuide(t *testing.T) {
	var ko []string
	for _, l := range []rel011Lang{rel011Ko, rel011En} {
		sec, blocks := rel011Blocks(t, l, l.uninstall)
		rel011HasBlocks(t, l.install+" "+l.uninstall, blocks,
			"brew uninstall jgoneit/tap/jaekit\n", "rm ~/.local/bin/ha\n", "rm ~/.local/bin/spar\n",
			"claude plugin uninstall spec@jaekit\nclaude plugin uninstall seal@jaekit\nclaude plugin uninstall spar@jaekit\nclaude plugin marketplace remove jaekit\n",
			"codex plugin remove spec@jaekit\ncodex plugin remove seal@jaekit\ncodex plugin remove spar@jaekit\ncodex plugin marketplace remove jaekit\n",
			rel011RemoveOutput)
		requireIn(t, l.install+" "+l.uninstall, sec, l.removeProse...)
		forbidAll(t, l.install, "rm -rf .git/ha")
		// The README may link the install guide's index instead of every
		// maintenance section directly; uninstall must remain reachable there.
		requireIn(t, l.readme+" installation guide", read(t, l.readme), "("+l.installLink+")")
		requireIn(t, l.install+" uninstall link", read(t, l.install),
			"(#"+onboardSlug(strings.TrimPrefix(l.uninstall, "## "))+")")
		if l.readme == rel011Ko.readme {
			ko = blocks
		} else {
			rel011SameBlocks(t, l.uninstall, ko, blocks)
		}
	}
}

func TestRelease013ReferenceGuidance(t *testing.T) {
	requireAll(t, "../examples/check-result/README.md", "## 조건표", "## 결과 계약", "ha check <goal> --baseline AC-1", "ha check <goal> AC-1", "ha done")
	var ko []string
	for _, guide := range []struct{ path, heading string }{
		{"../guides/INSTALL.md", "## 참조 검사 가져오기"},
		{"../guides/INSTALL.en.md", "## Get the reference check"},
	} {
		section := mdSection(t, guide.path, guide.heading)
		requireIn(t, guide.path, section,
			`package_root="$(brew --prefix jaekit)/share/jaekit"`,
			`package_root="$HOME/.local/share/jaekit/0.1.9"`,
			`cp "$package_root/tools/check-result-reference.py" tools/`,
			`cp "$package_root/examples/check-result/declaration.json" checks/`,
			`cp "$package_root/examples/check-result/reference.json" checks/`,
			`cp "$package_root/examples/check-result/input.txt" sample/`,
			"python3 tools/check-result-reference.py checks/declaration.json checks/reference.json",
			"../examples/check-result/README.md")
		blocks := onboardBlocks(section, "bash")
		if ko == nil {
			ko = blocks
		} else {
			rel011SameBlocks(t, "reference acquisition", ko, blocks)
		}
	}
	for _, guide := range []rel011Lang{rel011Ko, rel011En} {
		section := mdSection(t, guide.install, guide.release)
		// Verified public data and same-version conflicts are handled before
		// replacing the executable. This also prevents a failed data copy from
		// being reported as a successful complete installation.
		requireOrder(t, guide.install, section,
			`shasum -a 256 -c "$name.sha256"`, `tar -xzf "$name.tar.gz"`,
			`cp -R "$name/tools" "$name/examples"`,
			`if ! diff -qr "$tmp/public" "$share"; then`,
			`mv -f "$binary_tmp" "$bin/ha"`)
	}
}
