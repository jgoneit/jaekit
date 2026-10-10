package plugins

import (
	"strings"
	"testing"
)

// mdSection returns the part of a Markdown file from the heading line up to
// the next heading of the same or a higher level, ignoring fenced code.
func mdSection(t *testing.T, path, heading string) string {
	t.Helper()
	text := read(t, path)
	level := strings.Index(heading, " ")
	lines := strings.Split(text, "\n")
	start := -1
	fenced := false
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
			return strings.Join(lines[start:i], "\n")
		}
	}
	if start < 0 {
		t.Fatalf("%s lacks heading %q", path, heading)
	}
	return strings.Join(lines[start:], "\n")
}

func requireIn(t *testing.T, name, text string, musts ...string) {
	t.Helper()
	for _, s := range musts {
		if !strings.Contains(text, s) {
			t.Errorf("%s lacks %q", name, s)
		}
	}
}

// requireOrder checks that each string first appears after the previous one.
func requireOrder(t *testing.T, name, text string, ordered ...string) {
	t.Helper()
	at := 0
	for _, s := range ordered {
		i := strings.Index(text[at:], s)
		if i < 0 {
			t.Errorf("%s lacks %q after position %d", name, s, at)
			return
		}
		at += i + len(s)
	}
}

// without brew, the install guide that the
// README links names the Release files and installs one by platform, and the
// file names and the path inside the archive match what the release job
// builds. Running the commands per platform is checked separately. The guide
// moved from the README to guides/INSTALL.md .
func TestReleaseGuideReleaseFiles(t *testing.T) {
	files := mdSection(t, "../guides/INSTALL.md", "## Release 파일로 설치")
	requireIn(t, "INSTALL Release 파일로 설치", files,
		"https://github.com/jgoneit/jaekit/releases/tag/v0.1.3",
		"`ha_0.1.3_<os>_<arch>.tar.gz`",
		"아래 명령은 고치지 않고 그대로 붙여 넣습니다.",
		`name="ha_0.1.3_${os}_${arch}"`,
		`url="https://github.com/jgoneit/jaekit/releases/download/v0.1.3"`,
		`curl -fsSLO "$url/$name.tar.gz"`,
		`tar -xzf "$name.tar.gz"`,
		`mv -f "$binary_tmp" "$bin/ha"`,
		`export PATH="$HOME/.local/bin:$PATH"`,
	)
	forbidAll(t, "../README.md", "os=darwin arch=arm64")
	requireIn(t, "release workflow", read(t, "../.github/workflows/release.yml"),
		`name="ha_${version}_${os}_${arch}"`,
		`-o "${build}/${name}/ha" ./cmd/ha`,
		`tar -C "${build}" -czf "${dist}/${name}.tar.gz" "${name}"`,
		"for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do",
	)
	requireIn(t, "README 설치", mdSection(t, "../README.md", "### 설치"),
		"(guides/INSTALL.md)")
}

// the linked install guide shows the ha version and how to find
// another ha earlier on PATH.
func TestReleaseGuideVersionCheck(t *testing.T) {
	requireIn(t, "INSTALL 버전 확인", mdSection(t, "../guides/INSTALL.md", "### 버전 확인"),
		"ha --version\nwhich -a ha\n",
		"`ha --version`은 `ha 0.1.3`를 출력해야 합니다.", "`which -a ha`의 첫 줄", "실제로 실행되는 파일",
		"PATH", "지우거나 PATH 순서를 바꿉니다", "`go env GOBIN`", "`$(go env GOPATH)/bin`",
	)
}

// a user who registered a checkout path as the jaekit
// marketplace is told to remove it and register the release-pinned GitHub
// repository instead, then reinstall both plugins, for both hosts. The guide
// is in guides/INSTALL.md, linked from the README .
func TestReleaseGuideLocalMigration(t *testing.T) {
	requireIn(t, "README 설치", mdSection(t, "../README.md", "### 설치"),
		"(guides/INSTALL.md#로컬-경로-등록에서-옮기기)")
	move := mdSection(t, "../guides/INSTALL.md", "## 로컬 경로 등록에서 옮기기")
	requireIn(t, "INSTALL 로컬 경로 등록에서 옮기기", move,
		"claude plugin marketplace remove jaekit\n"+
			"claude plugin marketplace add jgoneit/jaekit#v0.1.3\n"+
			"claude plugin install spec@jaekit\n"+
			"claude plugin install seal@jaekit\n",
		"codex plugin marketplace remove jaekit\n"+
			"codex plugin marketplace add jgoneit/jaekit@v0.1.3\n"+
			"codex plugin add spec@jaekit\n"+
			"codex plugin add seal@jaekit\n",
		"지우면 그 marketplace에서 설치한 spec·seal도 함께 지워지므로 다시 설치합니다.",
	)
}

// the design set's "한 번 맡긴다" matches the two-step flow:

// the operations guide's first setup uses the README install
// path: brew or the Release file, then the release-pinned GitHub marketplace.
// The Release file and the move from a checkout registration are in
// guides/INSTALL.md .
func TestReleaseGuideOperationsSetup(t *testing.T) {
	requireAll(t, "../guides/OPERATIONS.md", "(INSTALL.md)", "(USAGE.md)")
	TestReadmeVisualGuideAC8(t)
}

// README-linked guides keep host invocations, the Seal start
// command and resume phrase, the two-step flow, and plugin conflict guidance.
func TestReleaseGuideKeepsExisting(t *testing.T) {
	TestSpecInvocationGuide(t)
	TestSealStartCommandGuide(t)
	TestGuidesDescribeDirectSave(t)
	requireAll(t, "../guides/USAGE.md", "`docs/specs/<goal> 이어서 해줘`", "`@Spec`", "보장이 없습니다")
	requireAll(t, "../guides/INSTALL.md", "다른 플러그인과 혼동되면", "(USAGE.md#문제-해결)")
}

// the linked install guide goes brew, version
// check, pinned marketplace, then both plugins.
func TestReleaseGuideInstallOrder(t *testing.T) {
	requireOrder(t, "INSTALL 기본 설치", mdSection(t, "../guides/INSTALL.md", "## 기본 설치"),
		"brew install jgoneit/tap/jaekit",
		"ha --version",
		"codex plugin marketplace add jgoneit/jaekit@v0.1.3",
		"codex plugin add spec@jaekit",
		"codex plugin add seal@jaekit",
	)
}

// both marketplace files keep repository-relative plugin
// sources, so a checkout registered by path uses its own plugin files.
func TestReleaseGuideMarketplaceSourcesLocal(t *testing.T) {
	var claude struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &claude)
	var codex struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Source string `json:"source"`
				Path   string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../.agents/plugins/marketplace.json", &codex)
	if len(claude.Plugins) != 2 || len(codex.Plugins) != 2 {
		t.Fatalf("want 2 plugins in each marketplace, got %d and %d", len(claude.Plugins), len(codex.Plugins))
	}
	for _, p := range claude.Plugins {
		if p.Source != "./plugins/"+p.Name {
			t.Errorf("Claude marketplace %s source = %q", p.Name, p.Source)
		}
	}
	for _, p := range codex.Plugins {
		if p.Source.Source != "local" || p.Source.Path != "./plugins/"+p.Name {
			t.Errorf("Codex marketplace %s source = %+v", p.Name, p.Source)
		}
	}
}
