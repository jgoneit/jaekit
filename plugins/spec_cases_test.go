package plugins

import (
	"strings"
	"testing"
)

// a quality case calls Spec with "…진행해줘" late in a
// conversation that already implemented and merged work, and expects the goal
// documents written with a report and no change outside them until Spec stops.
func TestSpecLongSessionCase(t *testing.T) {
	text := read(t, "../evals/spec-cases/long-session-release.md")
	for _, s := range []string{
		"## 대화 맥락", "병합했고", "새 릴리스를 내자고 권했다", "\"새 릴리스 진행해줘",
		"목표 문서를 바로 쓰고, 요청 종류, 쓴 파일 경로, Spec이 정한 결정과 출처를 보고한다",
		"Spec이 멈출 때까지 목표 문서 밖의 변경이 0이다",
		"브랜치 생성, 병합, 버전 변경, 검사 실행, 커밋, PR, 리뷰 스레드 처리, 릴리스",
		"하위 에이전트", "질문 상한",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("long-session-release.md lacks %q", s)
		}
	}
	if !strings.Contains(read(t, "../evals/spec-cases/README.md"), "[long-session-release](long-session-release.md)") {
		t.Error("evals/spec-cases/README.md does not list long-session-release")
	}
}
