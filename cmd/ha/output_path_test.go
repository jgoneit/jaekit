package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The run-record contract states what output_path holds: a repository-relative
// path without absolute paths, the same form in a linked worktree with .git
// standing for that working tree's git directory, and older absolute paths
// read as written.
func TestRunRecordStatesOutputPath(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "contracts", "run-record.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"로컬 출력 위치(저장소 기준 경로, 예: `.git/ha/runs/<goal-key>/7.log`)",
		"`output_path`에는 절대 경로를 쓰지 않는다.",
		"linked worktree에서도 형식이 같고, 이때 `.git`은 그 작업 트리의 git 디렉토리를 뜻한다.",
		"이전 `ha`가 남긴 절대 경로는 고쳐 쓰지 않고 그대로 읽는다.",
	} {
		if !strings.Contains(string(doc), s) {
			t.Errorf("run-record.md lacks %q", s)
		}
	}
}
