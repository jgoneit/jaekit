package spar

// Checks of a boundary document that is also a criteria document of the same
// run. Its text is in the packet once, under `## Criteria`; the boundary
// section names it and says so, the size check sees that packet, and
// round.json still keeps it among the boundary documents. A packet whose
// boundary documents are not criteria documents is the packet spar made
// before this rule: testdata/packet-boundaries holds those packets as the
// earlier packet.go wrote them.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// bpPointer stands under the name of a boundary document that is also a
// criteria document, in place of its text.
const bpPointer = "This is the criteria document of the same name under `## Criteria` above; its text is not repeated here."

// bpReview writes a REVIEW.md with a heading, a table and a code fence of its
// own, as a goal's REVIEW.md that is passed both as criteria and as boundary.
func (w *rptWorld) bpReview() string {
	return w.write(filepath.Join(w.repo, "docs", "REVIEW.md"),
		"## 독립 검토\n\n| 검사 | 경계 |\n| --- | --- |\n| AC-1 | 인사말 한 줄 |\n\n```text\n경계 밖: 빈 파일\n```\n")
}

// bpPacket is the packet of a dry candidate round from commit 0 to 1 in a
// series of its own, with extra arguments.
func (w *rptWorld) bpPacket(series string, extra ...string) string {
	w.t.Helper()
	w.useConfig(w.printer(w.answer(series, w.commits[1])))
	out := filepath.Join(w.repo, "reviews", series, "01-candidate")
	w.run(append(append(w.round("candidate", w.commits[0], w.commits[1], out), "--dry-run"), extra...)...)
	return w.read(filepath.Join(out, "packet.md"))
}

// bpFixed is the packet of stage that the earlier packet.go made for
// rptStages with the two rptBoundaries documents, which are not criteria
// documents.
func bpFixed(t *testing.T, stage string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "packet-boundaries", stage+".md"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// bpGuidance is the boundary section of packet up to its first document.
func bpGuidance(packet string) string {
	section := pdSectionOf(packet, rptBoundaryHeading)
	if k := strings.Index(section, "\n### "); k >= 0 {
		return section[:k+1]
	}
	return section
}

// bpFirstDiff is the first line at which got and want differ, with both
// lines.
func bpFirstDiff(got, want string) (int, string, string) {
	g, h := strings.Split(got, "\n"), strings.Split(want, "\n")
	n := 0
	for n < len(g) && n < len(h) && g[n] == h[n] {
		n++
	}
	line := func(lines []string) string {
		if n < len(lines) {
			return lines[n]
		}
		return "(end)"
	}
	return n + 1, line(g), line(h)
}

func bpSum(text string) string {
	s := sha256.Sum256([]byte(text))
	return hex.EncodeToString(s[:])
}

// bpRecorded is the names and sha256 values of the documents round.json
// keeps under key.
func bpRecorded(r map[string]any, key string) ([]string, []string) {
	list, _ := r[key].([]any)
	var names, sums []string
	for _, item := range list {
		m, _ := item.(map[string]any)
		name, _ := m["name"].(string)
		sum, _ := m["sha256"].(string)
		names, sums = append(names, name), append(sums, sum)
	}
	return names, sums
}

// The same file passed as --criteria and as --boundary is in the packet
// once, under `## Criteria`, in every stage; the boundary section keeps its
// place in the order given and names it with the line that points to the
// criteria document.
func TestBoundaryCriteriaDocumentInPacketOnce(t *testing.T) {
	w := newRptWorld(t, true)
	one, _ := w.rptBoundaries()
	review := w.bpReview()
	text := w.read(review)
	packets := w.rptStages("once", "--criteria", review, "--boundary", one, "--boundary", review)
	for _, stage := range Stages {
		packet := packets[stage]
		if n := strings.Count(packet, text); n != 1 {
			t.Errorf("the %s packet holds the text of docs/REVIEW.md %d times, want 1", stage, n)
		}
		if !strings.Contains(pdSectionOf(packet, "## Criteria"), "### docs/REVIEW.md\n\n````\n"+text+"````\n") {
			t.Errorf("the %s packet's criteria section does not hold docs/REVIEW.md", stage)
		}
		section := pdSectionOf(packet, rptBoundaryHeading)
		i, j := strings.Index(section, "### docs/B1.md\n"), strings.Index(section, "### docs/REVIEW.md\n\n"+bpPointer+"\n\n")
		if i < 0 || j < 0 || i > j {
			t.Errorf("the %s packet's boundary section does not name docs/B1.md and then docs/REVIEW.md with the pointer to the criteria document:\n%s", stage, section)
		}
		if strings.Contains(section, text) {
			t.Errorf("the %s packet's boundary section repeats the text of docs/REVIEW.md", stage)
		}
	}
}

// A boundary document that is not a criteria document is embedded in full
// beside one that is, and has no pointer.
func TestBoundaryOnlyDocumentStillEmbedded(t *testing.T) {
	w := newRptWorld(t, true)
	one, two := w.rptBoundaries()
	review := w.bpReview()
	packets := w.rptStages("only", "--criteria", review, "--boundary", one, "--boundary", review, "--boundary", two)
	for _, stage := range Stages {
		section := pdSectionOf(packets[stage], rptBoundaryHeading)
		for _, path := range []string{one, two} {
			if !strings.Contains(section, w.read(path)) {
				t.Errorf("the %s packet's boundary section does not hold the text of %s", stage, filepath.Base(path))
			}
		}
		if n := strings.Count(section, bpPointer); n != 1 {
			t.Errorf("the %s packet's boundary section has %d pointers to a criteria document, want 1 (docs/REVIEW.md only)", stage, n)
		}
	}
}

// A packet whose boundary documents are not criteria documents is byte for
// byte the packet the earlier packet.go made from the same input.
func TestBoundaryPacketWithoutCriteriaCopyUnchanged(t *testing.T) {
	w := newRptWorld(t, true)
	one, two := w.rptBoundaries()
	packets := w.rptStages("fixed", "--boundary", one, "--boundary", two)
	for _, stage := range Stages {
		if got, want := w.normalize(packets[stage]), bpFixed(t, stage); got != want {
			n, g, h := bpFirstDiff(got, want)
			t.Errorf("the %s packet with boundary documents that are not criteria differs from the earlier packet at line %d: got %q, want %q", stage, n, g, h)
		}
	}
}

// The guidance that opens the boundary section is the earlier guidance,
// byte for byte, when a boundary document is also a criteria document.
func TestBoundaryGuidanceUnchangedWithCriteriaCopy(t *testing.T) {
	w := newRptWorld(t, true)
	review := w.bpReview()
	packets := w.rptStages("guidance", "--criteria", review, "--boundary", review)
	for _, stage := range Stages {
		got, want := bpGuidance(packets[stage]), bpGuidance(bpFixed(t, stage))
		if want == "" || got != want {
			n, g, h := bpFirstDiff(got, want)
			t.Errorf("the %s packet's boundary guidance differs from the earlier guidance at line %d: got %q, want %q", stage, n, g, h)
		}
	}
}

// round.json keeps a boundary document that is also a criteria document
// among the boundaries, by name and digest in the order given, as well as
// among the criteria.
func TestBoundaryCriteriaDocumentStillRecorded(t *testing.T) {
	w := newRptWorld(t, true)
	one, _ := w.rptBoundaries()
	review := w.bpReview()
	w.rptStages("rec", "--criteria", review, "--boundary", one, "--boundary", review)
	spec := filepath.Join(w.repo, "docs", "SPEC.md")
	for i, stage := range []string{"plan", "plan-delta", "candidate", "fixes"} {
		r := w.rawRound(filepath.Join(w.repo, "reviews", "rec", fmt.Sprintf("%02d-%s", i+1, stage)))
		names, sums := bpRecorded(r, "boundaries")
		if !reflect.DeepEqual(names, []string{"docs/B1.md", "docs/REVIEW.md"}) || !reflect.DeepEqual(sums, []string{bpSum(w.read(one)), bpSum(w.read(review))}) {
			t.Errorf("the %s round.json keeps boundaries %v %v", stage, names, sums)
		}
		names, sums = bpRecorded(r, "criteria")
		if !reflect.DeepEqual(names, []string{"docs/SPEC.md", "docs/REVIEW.md"}) || !reflect.DeepEqual(sums, []string{bpSum(w.read(spec)), bpSum(w.read(review))}) {
			t.Errorf("the %s round.json keeps criteria %v %v", stage, names, sums)
		}
	}
}

// The same file is found through the path normalization --criteria uses: a
// boundary given through a symbolic link to the criteria document is that
// document.
func TestBoundaryCriteriaDocumentThroughLinkInPacketOnce(t *testing.T) {
	w := newRptWorld(t, true)
	review := w.bpReview()
	link := filepath.Join(w.repo, "docs", "REVIEW-link.md")
	if err := os.Symlink("REVIEW.md", link); err != nil {
		t.Skipf("setup: a symbolic link cannot be made here: %v", err)
	}
	packet := w.bpPacket("link", "--criteria", review, "--boundary", link)
	if n := strings.Count(packet, w.read(review)); n != 1 {
		t.Errorf("the packet holds the text of docs/REVIEW.md %d times, want 1", n)
	}
	if !strings.Contains(pdSectionOf(packet, rptBoundaryHeading), "### docs/REVIEW.md\n\n"+bpPointer+"\n") {
		t.Errorf("the boundary section does not point to docs/REVIEW.md")
	}
}

// A file with the same text as a criteria document at another path is not
// that document, and neither is a criteria document whose name another
// criteria document shares: each is embedded in full.
func TestBoundaryOtherDocumentStillEmbedded(t *testing.T) {
	w := newRptWorld(t, true)
	review := w.bpReview()
	copyOf := w.write(filepath.Join(w.repo, "docs", "COPY.md"), w.read(review))
	section := pdSectionOf(w.bpPacket("copy", "--criteria", review, "--boundary", copyOf), rptBoundaryHeading)
	if !strings.Contains(section, "### docs/COPY.md\n\n````\n"+w.read(review)) || strings.Contains(section, bpPointer) {
		t.Errorf("a copy of a criteria document at another path is not embedded in full:\n%s", section)
	}
	// Outside the repository a document's name is its file name, so two
	// criteria documents can share it; the pointer would not say which.
	a := w.write(filepath.Join(w.dir, "outside", "a", "R.md"), "## 경계 가\n\n첫 R 문서.\n")
	b := w.write(filepath.Join(w.dir, "outside", "b", "R.md"), "## 경계 나\n\n둘째 R 문서.\n")
	section = pdSectionOf(w.bpPacket("shared", "--criteria", a, "--criteria", b, "--boundary", a), rptBoundaryHeading)
	if !strings.Contains(section, "### R.md\n\n```\n"+w.read(a)) || strings.Contains(section, bpPointer) {
		t.Errorf("a boundary document whose name two criteria documents share is not embedded in full:\n%s", section)
	}
}

// The size check measures the packet that holds the document once: at a
// limit of that size the round is admitted, and the same text embedded a
// second time from another path is over it.
func TestBoundaryCriteriaDocumentCountsOnceTowardLimit(t *testing.T) {
	w := newRptWorld(t, true)
	text := strings.Repeat("- 경계 한 줄이 패킷을 키운다\n", 400)
	review := w.write(filepath.Join(w.repo, "docs", "REVIEW.md"), text)
	copyOf := w.write(filepath.Join(w.repo, "docs", "COPY.md"), text)
	mark := w.script("mark", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n")
	w.useConfig(pdCommand("cmd", mark))
	alone := w.rptSize("measure-alone", "--criteria", review)
	once := w.rptSize("measure-once", "--criteria", review, "--boundary", review)
	twice := w.rptSize("measure-twice", "--criteria", review, "--boundary", copyOf)
	if once-alone >= len(text) {
		t.Errorf("passing docs/REVIEW.md also as a boundary adds %d bytes to the packet, not less than its %d-byte text", once-alone, len(text))
	}
	if twice <= once {
		t.Fatalf("setup: the packet with a copy at another path (%d bytes) is not larger than %d bytes", twice, once)
	}
	w.useConfig(pdCommand("cmd", mark) + fmt.Sprintf("max_packet_bytes = %d\n", once))
	for _, dry := range []bool{true, false} {
		out := filepath.Join(w.repo, "reviews", fmt.Sprintf("at-%v", dry), "01-candidate")
		args := append(w.round("candidate", w.commits[0], w.commits[1], out), "--criteria", review, "--boundary", review)
		if dry {
			args = append(args, "--dry-run")
		}
		if code, stderr := w.spar(args...); code != ExitOK && code != ExitNoResult {
			t.Errorf("a %d-byte packet that holds docs/REVIEW.md once was not admitted at its size (dry run %v): exit %d: %s", once, dry, code, pdFirstLine(stderr))
		}
	}
	out := filepath.Join(w.repo, "reviews", "over", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), "--criteria", review, "--boundary", copyOf, "--dry-run")...)
	if !w.rptRefused(code, stderr, out, "cmd", once, twice) {
		t.Errorf("a %d-byte packet with the text twice was not refused at the %d-byte limit: exit %d: %s", twice, once, code, pdFirstLine(stderr))
	}
}
