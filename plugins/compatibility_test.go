package plugins

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func compatibilityContains(t *testing.T, path, content string, want ...string) {
	t.Helper()
	for _, text := range want {
		if !strings.Contains(content, text) {
			t.Errorf("[requirement] %s is missing %q", path, text)
		}
	}
}

func TestCompatibilityAC10_SourceVersions(t *testing.T) {
	var marketplace struct {
		Metadata struct{ Version string } `json:"metadata"`
		Plugins  []manifest               `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &marketplace)
	if marketplace.Metadata.Version != "0.1.6" {
		t.Errorf("[requirement] current released marketplace identity changed: %s", marketplace.Metadata.Version)
	}
	// Seal 0.1.18 is the source version that adds runner adapter guidance;
	// the released marketplace identity above stays v0.1.6.
	for name, want := range map[string]string{"spec": "0.1.12", "seal": "0.1.18"} {
		t.Run(name, func(t *testing.T) {
			_, version := frontMatter(t, filepath.Join(name, "skills", name, "SKILL.md"))
			if version != want {
				t.Errorf("[requirement] %s source skill is %s, want %s", name, version, want)
			}
			for _, host := range []string{".claude-plugin", ".codex-plugin"} {
				var plugin manifest
				readJSON(t, filepath.Join(name, host, "plugin.json"), &plugin)
				if plugin.Name != name || plugin.Version != want {
					t.Errorf("[requirement] %s/%s disagrees with source identity: %+v", name, host, plugin)
				}
			}
			found := false
			for _, plugin := range marketplace.Plugins {
				if plugin.Name == name {
					found = plugin.Version == want
				}
			}
			if !found {
				t.Errorf("[requirement] marketplace does not identify %s %s", name, want)
			}
		})
	}
	compatibilityContains(t, "Core development version", read(t, "../cmd/ha/main.go"), `var version = "0.1.6-dev"`)
	// These package a checkout build, which prints the development version;
	// install.ps1 refuses a manifest version the executable does not print.
	for _, path := range []string{"../tools/windows/docs_check.py", "../tools/windows/README.md"} {
		content := read(t, path)
		compatibilityContains(t, path, content, "0.1.6-dev")
		if strings.Contains(content, "0.1.5-dev") {
			t.Errorf("[requirement] %s packages a checkout build as 0.1.5-dev", path)
		}
	}
	// Codex's marketplace format has no component version field.
	var codex map[string]any
	readJSON(t, "../.agents/plugins/marketplace.json", &codex)
	if _, added := codex["version"]; added {
		t.Error("[requirement] added an unnecessary Codex marketplace version")
	}
}

func TestCompatibilityAC11_ReleaseAndDevelopmentGuidance(t *testing.T) {
	for _, path := range []string{"../README.md", "../README.ko.md", "../guides/INSTALL.md", "../guides/INSTALL.en.md", "../guides/USAGE.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.md"} {
		t.Run(path, func(t *testing.T) {
			content := read(t, path)
			if path == "../README.md" || path == "../README.ko.md" {
				// The short README links the version matrix instead of repeating
				// component identities and rule details in its introduction.
				guide, heading, anchor := "../guides/INSTALL.en.md", "## Release and development combinations", "release-and-development-combinations"
				if path == "../README.ko.md" {
					guide, heading, anchor = "../guides/INSTALL.md", "## 배포판과 개발 조합", "배포판과-개발-조합"
				}
				compatibilityContains(t, path, content, "v0.1.6", "ha 0.1.6", "("+strings.TrimPrefix(guide, "../")+"#"+anchor+")")
				content += "\n" + visualSection(t, guide, heading)
			}
			// Spec 0.1.12 and Seal 0.1.17 make up the current release.
			compatibilityContains(t, path, content, "v0.1.6", "ha 0.1.6", "0.1.12", "0.1.17", "`/3`")
			for _, stale := range []string{"unreleased", "미배포", "아직 배포하지 않았", "jaekit@v0.1.2", "jaekit#v0.1.2", "/releases/download/v0.1.2"} {
				if strings.Contains(strings.ToLower(content), stale) {
					t.Errorf("[requirement] %s retains obsolete release guidance: %s", path, stale)
				}
			}
		})
	}
	for _, path := range []string{"../guides/INSTALL.md", "../guides/INSTALL.en.md"} {
		compatibilityContains(t, path, read(t, path), "jaekit@v0.1.6", "jaekit#v0.1.6", "ha_0.1.6_${os}_${arch}", "contracts/core-capabilities.md", "ha 0.1.6-dev", "v0.1.2")
	}
	compatibilityContains(t, "Korean budget guidance", read(t, "../guides/USAGE.md"), "시간 상한 변경은 지원하지 않습니다", "횟수 총상한", "명시적 재개", "원래 한도")
	compatibilityContains(t, "English budget guidance", read(t, "../guides/USAGE.en.md"), "Changing the elapsed-time limit is unsupported", "total run limit", "explicit reopen", "original limits")
	// A release bump must not move a claim about what earlier releases lack
	// onto the release that now ships it.
	// Nor may it leave the previous release's Seal named as the current one,
	// or new goals described as starting under /3.
	for _, dir := range []string{"../guides", "../contracts", "../examples"} {
		paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			content := read(t, path)
			for _, stale := range []string{"v0.1.5 이하", "v0.1.5 and earlier", "v0.1.5 파일에 포함됐다고 가정하지", "included in the v0.1.5 files",
				"v0.1.6 이하", "v0.1.6 and earlier", "v0.1.6 파일에 포함됐다고 가정하지", "included in the v0.1.6 files",
				"현재 배포판의 Seal 0.1.12", "current release includes this format in Seal 0.1.12", "새 목표가 쓰는 `/3`",
				"새 목표는 `run-rules/3`으로 시작합니다", "v0.1.6의 Seal 0.1.16", "v0.1.6 includes this format in Seal 0.1.16",
				"seal 0.1.16을 받습니다", "spec 0.1.12 and seal 0.1.16."} {
				if strings.Contains(content, stale) {
					t.Errorf("[requirement] %s keeps stale current-release wording: %q", path, stale)
				}
			}
		}
	}
	// The operations guide names the /4 start and points to the contract for
	// what /4 adds to the /3 budget rules, instead of restating those rules.
	operations := read(t, "../guides/OPERATIONS.md")
	compatibilityContains(t, "operations guide", operations, "새 목표를 `run-rules/4`로 시작합니다", "USAGE.md#자율-처리-run-rules4", "run-record.md#47-run-rules4")
	for _, restated := range []string{"`/4`의 새 창은", "`/3`·`/4`의 새 창은", "`/4`에서는"} {
		if strings.Contains(operations, restated) {
			t.Errorf("[requirement] the operations guide restates a /4 budget rule: %q", restated)
		}
	}
	compatibilityContains(t, "Korean WSL guide", read(t, "../guides/WSL.md"), "v0.1.6 파일의 공개 자료", "v0.1.4 이하 배포 파일에는 없습니다")
	compatibilityContains(t, "English WSL guide", read(t, "../guides/WSL.en.md"), "public data of the v0.1.6 files", "Release files of v0.1.4 and earlier do not have it")
}

// The Seal reference points to run-record.md §4.7 for the run limit a /4
// reopen carries, instead of restating it: the restatement lost the
// contract's "later decreases are reflected" qualifier.
func TestCompatibilitySealReferenceLinksRunRules4Reopen(t *testing.T) {
	const path = "seal/skills/seal/references/ha.md"
	compatibilityContains(t, path, mdSection(t, path, "## run-rules/4"), "contracts/run-record.md#47-run-rules4")
	reference := read(t, path)
	for _, restated := range []string{"new window starts from", "last limit the user set", "`/4` reopens with"} {
		if strings.Contains(reference, restated) {
			t.Errorf("[requirement] %s restates the /4 reopen limit: %q", path, restated)
		}
	}
}

// Each section that gives the /3 reopen limit also points to run-record.md
// §4.7 for /4, so "/4 keeps every /3 rule" does not lead to the /3 limit.
func TestCompatibilitySealRun3ReopenLinksRunRules4(t *testing.T) {
	for _, s := range []struct{ path, heading string }{
		{"seal/skills/seal/SKILL.md", "## Budget"},
		{"seal/skills/seal/references/bundle.md", "### `## 예산`"},
		{"seal/skills/seal/references/ha.md", "## Budget changes"},
	} {
		compatibilityContains(t, s.path, mdSection(t, s.path, s.heading), "contracts/run-record.md#47-run-rules4")
	}
}

// The status JSON contract says which run limit a /4 window shows.
func TestCompatibilityRunRecordRunsLimitNamesRunRules4(t *testing.T) {
	const path = "../contracts/run-record.md"
	for _, line := range strings.Split(mdSection(t, path, "#### 예산 (`budget`)"), "\n") {
		if strings.HasPrefix(line, "| `runs_limit` |") {
			compatibilityContains(t, path+" runs_limit row", line, "`/4`", "§4.7")
			return
		}
	}
	t.Fatalf("%s has no runs_limit row in the budget table", path)
}

func TestCompatibilityAC12_SpecAndVerificationBoundaries(t *testing.T) {
	specPath := "spec/skills/spec/SKILL.md"
	spec := read(t, specPath)
	keys, _ := frontMatter(t, specPath)
	if keys["disable-model-invocation"] != "true" {
		t.Error("[requirement] Spec lost its explicit-invocation boundary")
	}
	compatibilityContains(t, specPath, spec,
		"without installing Core, querying its capabilities, or running verification",
		"The implementing agent chooses the checks, runner and verification method",
		"implementation starts with a new request", "This skill never edits code or runs checks",
		"preserve its format selection or its absence")
	err := filepath.WalkDir("spec/skills/spec/assets/templates", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		content := read(t, path)
		compatibilityContains(t, path, content, "Criteria-Format: nested/1")
		for _, command := range []string{"ha capabilities", "ha start", "ha check", "ha done", "## 결과 계약"} {
			if strings.Contains(content, command) {
				t.Errorf("[requirement] Spec template %s acquired execution requirement %q", path, command)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sealPath := "seal/skills/seal/SKILL.md"
	compatibilityContains(t, sealPath, read(t, sealPath),
		"Choose the checks, runner and verification method", "does not require an official adapter",
		"Python 3 is not a prerequisite for Seal", "Do not require `/3` declarations for `/1` or `/2`",
		"ordinary maintain, manual and task checks retain their roles")
}
