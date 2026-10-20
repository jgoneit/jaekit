package bundle

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentSection(t *testing.T) {
	plan := "# PLAN\n\n## 조건표\n- 셸: `sh ignored.sh`\n\n## 실행 환경\n\n" +
		"- 셸: `sh tests/env/plain.sh`\n" +
		"* 인자: `sh tests/env/ok.sh 'a b' \"c d\"`\n" +
		"```\n## 실행 환경\n- 펜스 안: `not a probe`\n```\n" +
		"\n## 예산\n- 검증 실행: 50\n"
	probes, found, problems := Environment(plan, "PLAN.md")
	if !found || len(problems) != 0 {
		t.Fatalf("found %v, problems %v", found, problems)
	}
	want := []Probe{
		{Name: "셸", Command: "sh tests/env/plain.sh", Argv: []string{"sh", "tests/env/plain.sh"}, Line: 8},
		{Name: "인자", Command: `sh tests/env/ok.sh 'a b' "c d"`, Argv: []string{"sh", "tests/env/ok.sh", "a b", "c d"}, Line: 9},
	}
	if !reflect.DeepEqual(probes, want) {
		t.Fatalf("probes %+v want %+v", probes, want)
	}
}

func TestEnvironmentAbsentOrEmpty(t *testing.T) {
	for _, plan := range []string{"# PLAN\n\n## 예산\n", "# PLAN\n```\n## 실행 환경\n- 셸: `sh x.sh`\n```\n", "# PLAN\n\n### 실행 환경\n- 셸: `sh x.sh`\n"} {
		if probes, found, problems := Environment(plan, "PLAN.md"); found || probes != nil || problems != nil {
			t.Fatalf("%q: probes %v, found %v, problems %v", plan, probes, found, problems)
		}
	}
	probes, found, problems := Environment("# PLAN\n\n## 실행 환경\n\n## 예산\n", "PLAN.md")
	if !found || len(probes) != 0 || len(problems) != 0 {
		t.Fatalf("empty section: probes %v, found %v, problems %v", probes, found, problems)
	}
}

func TestEnvironmentProblems(t *testing.T) {
	cases := []struct{ item, code string }{
		{"셸 `sh x.sh`", "environment_item_invalid"},
		{"- 콜론 없음 `sh x.sh`", "environment_item_invalid"},
		{"- : `sh x.sh`", "environment_item_invalid"},
		{"- 셸: sh x.sh", "environment_item_invalid"},
		{"- 셸: `sh x.sh` `sh y.sh`", "environment_item_invalid"},
		{"- 셸: `sh x.sh` 설명", "environment_item_invalid"},
		{"- `셸`: `sh x.sh`", "environment_item_invalid"},
		{"- 파이프: `sh x.sh | cat`", "shell_syntax"},
		{"- 변수: `echo $HOME`", "shell_syntax"},
		{"- 빈 명령: `  `", "command_empty"},
		{"- 따옴표: `sh 'x.sh`", "command_parse"},
	}
	for _, tc := range cases {
		plan := "# PLAN\n\n## 실행 환경\n- 좋은 도구: `sh ok.sh`\n" + tc.item + "\n"
		probes, found, problems := Environment(plan, "docs/specs/g/PLAN.md")
		if !found || probes != nil || len(problems) != 1 || problems[0].Code != tc.code || problems[0].Line != 5 || problems[0].File != "docs/specs/g/PLAN.md" {
			t.Errorf("%q: probes %v, problems %+v; want one %s on line 5", tc.item, probes, problems, tc.code)
		}
	}
	plan := "# PLAN\n\n## 실행 환경\n- 셸: `sh a.sh`\n- 셸: `sh b.sh`\n\n## 실행 환경\n- 다른: `sh c.sh`\n"
	_, _, problems := Environment(plan, "PLAN.md")
	var codes []string
	for _, p := range problems {
		codes = append(codes, p.Code)
	}
	if got := strings.Join(codes, ","); got != "environment_name_duplicate,environment_section_duplicate" {
		t.Fatalf("problems %v", problems)
	}
}
