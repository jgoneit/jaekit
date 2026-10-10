package plugins

import (
	"strings"
	"testing"
)

func TestRelease013ReferenceGuidance(t *testing.T) {
	requireAll(t, "../examples/check-result/README.md", "## 조건표", "## 결과 계약", "ha check <goal> --baseline AC-1", "ha check <goal> AC-1", "ha done", "synthetic example")
	var ko []string
	for _, guide := range []struct{ path, heading string }{
		{"../guides/INSTALL.md", "## 참조 검사 가져오기"},
		{"../guides/INSTALL.en.md", "## Get the reference check"},
	} {
		section := mdSection(t, guide.path, guide.heading)
		requireIn(t, guide.path, section,
			`package_root="$(brew --prefix jaekit)/share/jaekit"`,
			`package_root="$HOME/.local/share/jaekit/0.1.3"`,
			`cp "$package_root/tools/check-result-reference.py" tools/`,
			`cp "$package_root/examples/check-result/declaration.json" checks/`,
			`cp "$package_root/examples/check-result/reference.json" checks/`,
			`cp "$package_root/examples/check-result/input.txt" sample/`,
			"python3 tools/check-result-reference.py checks/declaration.json checks/reference.json",
			"../examples/check-result/README.md", "Python 3", "pytest", "Vitest", "Playwright")
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

func TestRelease013ProjectRecords(t *testing.T) {
	requireAll(t, "../guides/INSTALL.md", "Git 추적 여부는 프로젝트 정책", "추적된 기록과 ignore한 비공개 기록", "추적 중인 목표는", "ignore한 문서를 강제로 Git에 추가하지 않습니다")
	requireAll(t, "../guides/INSTALL.en.md", "depends on project policy", "Both tracked records and ignored private records stay", "For a tracked goal", "Do not force ignored documents into Git")
	requireAll(t, "../guides/USAGE.md", "프로젝트 정책", "다른 프로젝트의 필수 설정이 아닙니다")
	requireAll(t, "../guides/USAGE.en.md", "project policy", "not a requirement for other projects")
	for _, path := range []string{"spec/skills/spec/SKILL.md", "seal/skills/seal/SKILL.md"} {
		content := read(t, path)
		requireIn(t, path, content, "docs/specs/<goal>", "Do not force ignored private documents into Git", "not a requirement for other projects")
		for _, localOnly := range []string{"private backup repository", "primary checkout", "launchd"} {
			if strings.Contains(content, localOnly) {
				t.Errorf("%s turned local retention infrastructure into a product requirement: %s", path, localOnly)
			}
		}
	}
}
