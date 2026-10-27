package plugins

// AC-6 of the readme-first-run goal: what the goal does not change stays as
// it was. The README sections it edits are compared with their content at
// main 81a7d47 with only the edited parts masked; the hero, the command
// blocks and the links are compared with the same values, the links without
// the ones the goal adds; and the images the goal does not redraw keep their
// bytes. The expected values are written here.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// rfrKept are the sha256 values at main 81a7d47 of each README's flow,
// get-started and records sections with the goal's parts masked, and of its
// hero, command blocks and link list without the added links.
var rfrKept = map[string]map[string]string{
	"README.md": {
		"flow":        "f3d640a39d99aac5e948b357935fe9d791a12f5111a0c5151ec092c120855fec",
		"get-started": "d513e0f5306006ef33f58712f6d0e6b53a26c19c89a631a0c3064523d486eef7",
		"records":     "c9cb21d06688a18ed91f6af673393cfe81ef80212b851529ed796a40085b9abb",
		"hero":        "e2786373776467d77948970760fa85cc010d488ea902310cc820856304fc51bc",
		"commands":    "59f12488dd2183b2309aebdec44be212f4cf2383e234b5c3fbcead13a15dd150",
		"links":       "230cdd06ae13d86269ac5c8aa9c4f6140058ffdf1d4abd42b54227d9b69f2cc9",
	},
	"README.ko.md": {
		"flow":        "8a6501de29184c80a8af663fc7787de356e492ae5e09ccc502bf26cc8ace4fd1",
		"get-started": "cd21b3ebda85af36c614dc10ea2d53c35558de92288e057394d0226387230fa0",
		"records":     "6e4a611a894b01cfc1f0d54513c7dd9d6475004be759f2363fe5ee4d63acc2ff",
		"hero":        "b3c24b6ab3fc3fd3b9270086d61fe18135de39990a12d0159d4131f1d6e1ecbf",
		"commands":    "59f12488dd2183b2309aebdec44be212f4cf2383e234b5c3fbcead13a15dd150",
		"links":       "030ca6ade819d8427a62e35242be9c1b98204af988248a928b5d6618ed4d44ca",
	},
}

// rfrAdded are, per language, the link destinations the goal adds, the first
// lines of the paragraphs it adds to the records section, and the setup
// section heading it adds to get started.
var rfrAdded = map[string]struct {
	links, paragraphs []string
}{
	"ko": {
		links:      []string{"guides/SPAR.md#프로젝트-준비", "#첫-standard-목표-전에", "guides/USAGE.md#목표-하나의-비용-보기"},
		paragraphs: []string{"Spar는 프로젝트의 Git이 `docs/reviews/`를", "목표 하나가 검사와 검토 라운드에 쓴 비용은"},
	},
	"en": {
		links:      []string{"guides/SPAR.en.md#set-up-a-project", "#before-your-first-standard-goal", "guides/USAGE.en.md#reading-one-goals-cost"},
		paragraphs: []string{"Spar writes to `docs/reviews/` only once", "To see what one goal cost in checks and review rounds"},
	},
}

// rfrKeptImages are the sha256 values at main 81a7d47 of the images the goal
// does not redraw.
var rfrKeptImages = map[string]string{
	"assets/readme/demo.en.gif":        "8c898e100b55404f248021cd0d40b5ff248ac6adb35b26de31382c6e7539a6c7",
	"assets/readme/report-card.ko.png": "99a33a7541c3340e1349f8b9ae90c8228cbb5837f3e25aed5c81cc6a488fed8e",
	"assets/readme/report-card.en.png": "d7d84474ed79bd8bc4eaa997261fca596d980346535e9f979b49d24f5b8358ed",
	"assets/readme/report-card.ko.svg": "fcab4417c5a66fddb4e65745fadcbe25a6da8afe8ad5afcf0292092c831e8ac8",
	"assets/readme/report-card.en.svg": "987fd7e1ddb8dad73deabc2279c60b1ea131da1eb0e6d51ca925feb0beebb84d",
	"assets/readme/hero.ko.png":        "62bd0449ecc0d890fae579c9ca6fa9db79bdb2a38d825b554607c592fdc95584",
	"assets/readme/hero.en.png":        "d6209bb53f94ac7de240fa51bbd4559a6263702907d43e551eb113c02ded88b9",
}

// rfrMaskAlt replaces the alt text of the image that shows asset.
func rfrMaskAlt(text, asset string) string {
	alt, ok := rfvAlt(text, asset)
	if !ok {
		return text
	}
	return strings.Replace(text, "!["+alt+"]("+asset+")", "![<alt>]("+asset+")", 1)
}

// rfrDropParagraph removes the line that starts with prefix and the empty
// line after it.
func rfrDropParagraph(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			end := i + 1
			if end < len(lines) && lines[end] == "" {
				end++
			}
			return strings.Join(append(lines[:i:i], lines[end:]...), "\n")
		}
	}
	return text
}

// rfrMaskStep3 replaces the first task's step 3 from its heading to the
// first "**Claude Code**" line after it: the commands and the tip stay.
func rfrMaskStep3(text string) string {
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if start < 0 && strings.HasPrefix(line, "#### 3. ") {
			start = i
		}
		if start >= 0 && line == "**Claude Code**" {
			return strings.Join(append(append(lines[:start:start], "#### 3. <masked>", ""), lines[i:]...), "\n")
		}
	}
	return text
}

// rfrMasked returns the digests of one README's flow, get-started and
// records sections with the goal's parts masked or removed.
func rfrMasked(t *testing.T, l rfrLang, text string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, heading := range map[string]string{"flow": l.flow, "get-started": l.started, "records": l.records} {
		section, ok := rfvSection(text, heading)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, heading)
		}
		out[name] = section
	}
	flow := rfrMaskLine(out["flow"], "    decide{{")
	flow = rfrMaskLine(flow, "3. ")
	out["flow"] = rfrMaskAlt(flow, "assets/readme/usage-flow."+l.code+".png")
	started := out["get-started"]
	if setup, ok := rfvSection(started, l.setup); ok {
		started = strings.Replace(started, setup+"\n", "", 1)
	}
	out["get-started"] = rfrMaskStep3(started)
	records := rfrMaskAlt(out["records"], "assets/readme/goal-folder."+l.code+".png")
	for _, prefix := range rfrAdded[l.code].paragraphs {
		records = rfrDropParagraph(records, prefix)
	}
	out["records"] = rfrMaskLine(records, "docs/reviews/password-link/")
	for name, section := range out {
		out[name] = rfvSHA([]byte(section))
	}
	return out
}

// rfrWhole returns the hero, command block and link digests of a README, the
// links without one occurrence of each link the goal adds.
func rfrWhole(text string, added []string) map[string]string {
	out := map[string]string{}
	hero, _, _ := strings.Cut(text, "\n</p>\n")
	out["hero"] = rfvSHA([]byte(hero))
	var commands []string
	inBash := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case line == "```bash":
			inBash = true
		case line == "```":
			inBash = false
		case inBash:
			commands = append(commands, line)
		}
	}
	out["commands"] = rfvSHA([]byte(strings.Join(commands, "\n")))
	var links []string
	for _, m := range regexp.MustCompile(`\]\(([^)\s]+)\)|(?:href|src)="([^"]+)"`).FindAllStringSubmatch(text, -1) {
		links = append(links, m[1]+m[2])
	}
	for _, link := range added {
		for i, got := range links {
			if got == link {
				links = append(links[:i:i], links[i+1:]...)
				break
			}
		}
	}
	out["links"] = rfvSHA([]byte(strings.Join(links, "\n")))
	return out
}

// AC-6: outside the goal's parts, both READMEs, the hero, the commands, the
// links and the images it does not redraw are as they were.
func TestReadmeFirstRunAC6_KeptParts(t *testing.T) {
	for _, l := range rfrLangs {
		path := l.readme[3:]
		text := rfvRead(t, path)
		got := rfrMasked(t, l, text)
		for name, digest := range rfrWhole(text, rfrAdded[l.code].links) {
			got[name] = digest
		}
		for part, want := range rfrKept[path] {
			if got[part] != want {
				t.Errorf("%s %s changed outside this goal's parts: sha256 %s, want %s", path, part, got[part], want)
			}
		}
	}
	for path, want := range rfrKeptImages {
		b, err := os.ReadFile("../" + path)
		if err != nil {
			t.Fatalf("setup: read %s: %v", path, err)
		}
		if got := rfvSHA(b); got != want {
			t.Errorf("%s changed: sha256 %s, want %s", path, got, want)
		}
	}
}
