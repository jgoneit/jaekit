//go:build pagessite

package pagessite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ------------------------------------------------------------- shared readers

func h1(source string) string {
	inFence := false
	for _, line := range strings.Split(source, "\n") {
		if regexp.MustCompile("^\\s{0,3}(```|~~~)").MatchString(line) {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(line, "# ") {
			text := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			return strings.NewReplacer("`", "", "*", "").Replace(text)
		}
	}
	return ""
}

var titleRE = regexp.MustCompile(`<title>([^<]*)</title>`)

func title(pageHTML string) string {
	m := titleRE.FindStringSubmatch(pageHTML)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

func langAttr(pageHTML string) string {
	m := regexp.MustCompile(`<html lang="([a-z]+)"`).FindStringSubmatch(pageHTML)
	if m == nil {
		return ""
	}
	return m[1]
}

// relURL is the relative link from one directory page to another.
func relURL(from, to string) string {
	if from == to {
		return "./"
	}
	a := strings.Split(strings.TrimSuffix(from, "/"), "/")
	b := strings.Split(strings.TrimSuffix(to, "/"), "/")
	if from == "" {
		a = nil
	}
	common := 0
	for common < len(a) && common < len(b) && a[common] == b[common] {
		common++
	}
	out := strings.Repeat("../", len(a)-common) + strings.Join(b[common:], "/")
	if !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out
}

// resolve turns a link on a built page into a site path, or "" when it
// leaves the site.
func resolve(pageURL, href string) (string, string) {
	u, err := url.Parse(href)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "", ""
	}
	target := pageURL
	if u.Path != "" {
		target = path.Clean(path.Join(pageURL, u.Path))
		if strings.HasSuffix(u.Path, "/") || u.Path == "." || u.Path == "./" {
			target += "/"
		}
		if target == "./" || target == "/" {
			target = ""
		}
	}
	return target, u.Fragment
}

func fileFor(dir, sitePath string) string {
	if sitePath == "" || strings.HasSuffix(sitePath, "/") {
		return filepath.Join(dir, filepath.FromSlash(sitePath), "index.html")
	}
	return filepath.Join(dir, filepath.FromSlash(sitePath))
}

var idRE = regexp.MustCompile(`\sid="([^"]+)"`)

func ids(fragment string) map[string]bool {
	out := map[string]bool{}
	for _, m := range idRE.FindAllStringSubmatch(fragment, -1) {
		out[html.UnescapeString(m[1])] = true
	}
	return out
}

var hrefRE = regexp.MustCompile(`<a ([^>]*?)href="([^"]*)"`)

type link struct {
	href   string
	anchor bool
}

func links(fragment string) []link {
	var out []link
	for _, m := range hrefRE.FindAllStringSubmatch(fragment, -1) {
		out = append(out, link{href: html.UnescapeString(m[2]), anchor: strings.Contains(m[1], `class="anchor"`)})
	}
	return out
}

func installLine(t *testing.T, name string) string {
	t.Helper()
	inFence := false
	for _, line := range strings.Split(readFile(t, filepath.Join(repoRoot(t), name)), "\n") {
		if regexp.MustCompile("^\\s*(```|~~~)").MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence && regexp.MustCompile(`^brew install \S+$`).MatchString(strings.TrimSpace(line)) {
			return strings.TrimSpace(line)
		}
	}
	t.Skipf("%s has no brew install line", name)
	return ""
}

func releaseVersion(t *testing.T) string {
	t.Helper()
	var data struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(repoRoot(t), ".claude-plugin", "marketplace.json"))), &data); err != nil {
		t.Skipf("marketplace.json: %v", err)
	}
	return data.Metadata.Version
}

func walk(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Skipf("cannot read %s: %v", dir, err)
	}
	return files
}

// representative pages for the browser observations
var widePages = []string{"ko/", "en/", "ko/docs/", "en/docs/", "en/guides/usage/", "ko/guides/install/",
	"ko/contracts/run-record/", "ko/contracts/", "en/guides/spar/"}

// ------------------------------------------------------------- AC-1

func TestPagesSiteAC1_Published(t *testing.T) {
	const v = "site-not-published"
	dir := site(t, v)
	head := strings.TrimSpace(git(t, repoRoot(t), "rev-parse", "HEAD"))
	for _, d := range documents(t) {
		text := page(t, dir, d.URL+"index.html", v)
		if got := meta(text, "jaekit-commit"); got != head {
			fail(t, v, "%s names commit %q, not the built commit %s", d.URL, got, head)
		}
		source, ok := pageSource(text)
		if !ok || source != readFile(t, filepath.Join(repoRoot(t), filepath.FromSlash(d.Path))) {
			fail(t, v, "%s does not carry the bytes of %s", d.URL, d.Path)
		}
	}
	// A change to a guide alone reaches its page through the same builder.
	const marker = "Pages-site check sentence 7f3c."
	syn := synthetic(t, func(root string) {
		p := filepath.Join(root, "guides", "USAGE.md")
		data, _ := os.ReadFile(p)
		write(t, p, string(data)+"\n"+marker+"\n")
	})
	res := runBuild(t, syn, filepath.Join(t.TempDir(), "out"))
	if res.err != nil || res.code != 0 {
		fail(t, v, "a guide-only change does not build: %v %s", res.err, res.stderr)
	}
	if !strings.Contains(article(t, page(t, res.out, "ko/guides/usage/index.html", v), "ko/guides/usage/", v), marker) {
		fail(t, v, "a guide-only change does not appear in its page's text")
	}
	// The published site is the build of main's latest commit.
	status, live := get(t, liveSite+"ko/guides/usage/")
	if status != 200 {
		fail(t, v, "%sko/guides/usage/ answers HTTP %d", liveSite, status)
	}
	main := mainHead(t)
	if got := meta(live, "jaekit-commit"); got != main {
		fail(t, v, "the published page names commit %q, but main is %s", got, main)
	}
	source, ok := pageSource(live)
	rawStatus, raw := get(t, "https://raw.githubusercontent.com/"+ownerRepo+"/"+main+"/guides/USAGE.md")
	if rawStatus != 200 {
		t.Skipf("raw guides/USAGE.md at %s: HTTP %d", main, rawStatus)
	}
	if !ok || source != raw {
		fail(t, v, "the published usage page does not carry guides/USAGE.md of %s", main)
	}
	// What visitors read is that file rendered: the published text equals the
	// checked builder's rendering of the same file.
	shown := synthetic(t, func(root string) { write(t, filepath.Join(root, "guides", "USAGE.md"), raw) })
	local := runBuild(t, shown, filepath.Join(t.TempDir(), "out"))
	if local.err != nil || local.code != 0 {
		t.Skipf("guides/USAGE.md of %s does not build with the checked builder: %v %s", main, local.err, local.stderr)
	}
	want := visibleText(article(t, page(t, local.out, "ko/guides/usage/index.html", v), "ko/guides/usage/", v))
	if got := visibleText(article(t, live, "the published ko/guides/usage/", v)); got != want {
		fail(t, v, "the published usage page does not show the text of guides/USAGE.md of %s", main)
	}
}

// visibleText is a fragment's text as a reader sees it, with spacing collapsed.
func visibleText(fragment string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(fragment, " "))), " ")
}

// ------------------------------------------------------------- AC-2

func TestPagesSiteAC2_DocumentSet(t *testing.T) {
	const v = "doc-pages-incomplete"
	dir := site(t, v)
	docs := documents(t)
	byKey := map[string]map[string]doc{}
	for _, d := range docs {
		text := page(t, dir, d.URL+"index.html", v)
		if got := langAttr(text); got != d.Lang {
			fail(t, v, "%s has lang %q, want %q", d.URL, got, d.Lang)
		}
		want := h1(readFile(t, filepath.Join(repoRoot(t), filepath.FromSlash(d.Path))))
		if want == "" || !strings.HasPrefix(title(text), want) {
			fail(t, v, "%s is titled %q, not by its first heading %q", d.URL, title(text), want)
		}
		key := strings.TrimSuffix(strings.TrimSuffix(d.Path, ".md"), ".en")
		if byKey[key] == nil {
			byKey[key] = map[string]doc{}
		}
		byKey[key][d.Lang] = d
	}
	for _, rel := range []string{"ko/guides/usage/index.html", "en/guides/usage/index.html", "ko/contracts/run-record/index.html"} {
		page(t, dir, rel, v)
	}
	// Each language's start page and navigation list every document; one
	// that exists only in the other language is marked and opens there.
	for _, lang := range []string{"ko", "en"} {
		only := map[string]string{"ko": "영어만", "en": "Korean only"}[lang]
		for _, rel := range []string{lang + "/docs/", lang + "/guides/usage/"} {
			text := page(t, dir, rel+"index.html", v)
			for key, langs := range byKey {
				d, same := langs[lang]
				if !same {
					for _, other := range langs {
						d = other
					}
				}
				re := regexp.MustCompile(`<a href="` + regexp.QuoteMeta(relURL(rel, d.URL)) + `"[^>]*>(.*?)</a>`)
				m := re.FindStringSubmatch(text)
				if m == nil {
					fail(t, v, "%s does not list %s (%s)", rel, key, d.URL)
				}
				if !same && !strings.Contains(m[1], only) {
					fail(t, v, "%s lists %s without the %q mark", rel, key, only)
				}
			}
		}
	}
	// A new file appears without site changes; untracked, ignored and docs/ files never do.
	const secret = "PAGES-SITE-PRIVATE-7d1e"
	syn := synthetic(t, func(root string) {
		write(t, filepath.Join(root, "guides", "NEWGUIDE.md"), "# New guide\n\nA new guide.\n")
		write(t, filepath.Join(root, ".gitignore"), "/docs/\n/guides/IGNORED.md\n")
	})
	write(t, filepath.Join(syn, "guides", "DRAFT.md"), "# Draft "+secret+"\n")
	write(t, filepath.Join(syn, "guides", "IGNORED.md"), "# Ignored "+secret+"\n")
	write(t, filepath.Join(syn, "docs", "specs", "private", "SPEC.md"), "# Goal "+secret+"\n")
	res := runBuild(t, syn, filepath.Join(t.TempDir(), "out"))
	if res.err != nil || res.code != 0 {
		fail(t, v, "a tree with a new guide does not build: %v %s", res.err, res.stderr)
	}
	page(t, res.out, "ko/guides/newguide/index.html", v)
	if !strings.Contains(page(t, res.out, "en/docs/index.html", v), `href="../../ko/guides/newguide/"`) {
		fail(t, v, "the new guide is missing from the English docs start page")
	}
	for name, data := range walk(t, res.out) {
		if bytes.Contains(data, []byte(secret)) {
			fail(t, v, "%s carries an untracked, ignored or docs/ file", name)
		}
	}
	// The not-found page links both homes under the published base.
	notFound := page(t, dir, "404.html", v)
	for _, want := range []string{`href="/jaekit/ko/"`, `href="/jaekit/en/"`, `href="/jaekit/assets/site.css"`} {
		if !strings.Contains(notFound, want) {
			fail(t, v, "404.html lacks %s", want)
		}
	}
	noDocs := synthetic(t, nil)
	res = runBuild(t, noDocs, filepath.Join(t.TempDir(), "out"))
	if res.err != nil || res.code != 0 {
		fail(t, v, "a tree without docs/ does not build: %v %s", res.err, res.stderr)
	}
}

// ------------------------------------------------------------- AC-3

var tableDelimRE = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$|^\s*\|\s*:?-{3,}:?\s*\|\s*$`)

type fenced struct {
	lang, body string
}

// fences splits a Markdown source into its fenced code blocks, with the
// opening fence's indentation removed from each line as CommonMark does, and
// the remaining lines.
func fences(source string) ([]fenced, []string) {
	var blocks []fenced
	var prose []string
	open, lang, indent := "", "", 0
	var body []string
	fenceRE := regexp.MustCompile("^( *)(`{3,}|~{3,})\\s*([^`\\s]*)")
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if open == "" {
			if m := fenceRE.FindStringSubmatch(line); m != nil {
				open, lang, indent, body = m[2], m[3], len(m[1]), nil
				continue
			}
			prose = append(prose, line)
			continue
		}
		if strings.HasPrefix(trimmed, open[:1]) && len(trimmed) >= len(open) &&
			strings.Trim(trimmed, open[:1]+" \t") == "" && len(strings.TrimRight(trimmed, " \t")) >= len(open) {
			blocks = append(blocks, fenced{lang, strings.Join(body, "\n")})
			open = ""
			continue
		}
		cut := 0
		for cut < indent && cut < len(line) && line[cut] == ' ' {
			cut++
		}
		body = append(body, line[cut:])
	}
	return blocks, prose
}

func TestPagesSiteAC3_Rendering(t *testing.T) {
	const v = "rendering-differs"
	dir := site(t, v)
	docs := documents(t)
	facts := publicDocs(t, docs)
	for _, d := range docs {
		rel := d.URL + "index.html"
		body := article(t, page(t, dir, rel, v), rel, v)
		source := readFile(t, filepath.Join(repoRoot(t), filepath.FromSlash(d.Path)))
		got := ids(body)
		want := map[string]bool{}
		for _, a := range facts[d.Path].Anchors {
			want[a] = true
		}
		if strings.Join(keys(got), "\n") != strings.Join(keys(want), "\n") {
			fail(t, v, "%s heading ids %v differ from check_public_docs anchors %v", d.URL, keys(got), keys(want))
		}
		blocks, prose := fences(source)
		shown := map[string]bool{}
		for _, m := range preRE.FindAllStringSubmatch(body, -1) {
			text := html.UnescapeString(tagRE.ReplaceAllString(m[2], ""))
			shown[strings.TrimSuffix(text, "\n")] = true
			if strings.Contains(m[1], `class="mermaid"`) {
				shown["mermaid:"+strings.TrimSuffix(text, "\n")] = true
			}
		}
		for _, b := range blocks {
			key := strings.TrimSuffix(b.body, "\n")
			if b.lang == "mermaid" {
				key = "mermaid:" + key
			}
			if !shown[key] {
				fail(t, v, "%s drops or changes a %q code block starting %q", d.URL, b.lang, firstLine(b.body))
			}
		}
		tables, details := 0, 0
		for _, line := range prose {
			if tableDelimRE.MatchString(line) && strings.Contains(line, "|") {
				tables++
			}
			details += strings.Count(line, "<details>")
		}
		if got := strings.Count(body, "<table>"); got != tables {
			fail(t, v, "%s shows %d tables, its source has %d", d.URL, got, tables)
		}
		if got := strings.Count(body, "<details>"); got != details {
			fail(t, v, "%s shows %d <details>, its source has %d", d.URL, got, details)
		}
	}
	for _, rel := range []string{"ko/guides/usage/index.html", "en/guides/usage/index.html"} {
		text := page(t, dir, rel, v)
		if !strings.Contains(text, `assets/mermaid.min.js"`) || !strings.Contains(text, `<pre class="mermaid"`) {
			fail(t, v, "%s does not load the mermaid renderer", rel)
		}
	}
	page(t, dir, "assets/mermaid.min.js", v)
	if !strings.Contains(page(t, dir, "en/guides/usage/index.html", v), "continue docs/specs/&lt;goal&gt;") {
		fail(t, v, "the <goal> placeholder in a code block is not shown as text")
	}
	// Collapsed <details>, and the diagram drawn readably in both themes: the
	// page opens in the dark theme and is switched to the light one.
	for _, rel := range []string{"/en/guides/usage/", "/ko/guides/usage/"} {
		got := browse(t, dir, `
function rgba(s) { var m = String(s).match(/rgba?\(([^)]+)\)/); if (!m) return null; var p = m[1].split(/[\s,\/]+/).filter(Boolean).map(Number); return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1]; }
function lum(c) { var a = c.slice(0, 3).map(function (v) { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }); return 0.2126 * a[0] + 0.7152 * a[1] + 0.0722 * a[2]; }
function contrast(a, b) { var x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); }
function diagram(w) {
  function backdrop(el) { for (var e = el; e && e.nodeType === 1; e = e.parentElement || e.parentNode) { var c = rgba(w.getComputedStyle(e).backgroundColor); if (c && c[3] > 0.5) return c; } return [255, 255, 255, 1]; }
  var svg = w.document.querySelector("pre.mermaid svg");
  if (!svg) return {svg: false};
  var bg = backdrop(svg.parentElement), out = {svg: true, theme: w.jaekit.theme(), height: svg.getBoundingClientRect().height, text: 99, lines: 99, count: 0};
  Array.prototype.forEach.call(svg.querySelectorAll(".flowchart-link"), function (e) { out.lines = Math.min(out.lines, contrast(rgba(w.getComputedStyle(e).stroke), bg)); });
  Array.prototype.forEach.call(svg.querySelectorAll(".node"), function (n) {
    var l = n.querySelector(".nodeLabel"), s = n.querySelector("rect, polygon, path");
    if (l && s) { out.count++; out.text = Math.min(out.text, contrast(rgba(w.getComputedStyle(l).color), rgba(w.getComputedStyle(s).fill))); }
  });
  Array.prototype.forEach.call(svg.querySelectorAll(".edgeLabel span, .edgeLabel p"), function (e) {
    if (e.textContent.trim()) { out.count++; out.text = Math.min(out.text, contrast(rgba(w.getComputedStyle(e).color), backdrop(e))); }
  });
  return out;
}
localStorage.setItem("jaekit-theme", "dark");
var w = await load(`+fmt.Sprintf("%q", rel)+`, 1280, 900);
await wait(4000);
var d = w.document.querySelectorAll(".markdown details");
var closed = Array.prototype.every.call(d, function (x) { return !x.open; });
var s = d.length ? d[0].querySelector("summary") : null;
if (s) s.click();
var opened = d.length ? d[0].open : false;
var dark = diagram(w);
w.document.getElementById("theme-btn").click();
await wait(2500);
var light = diagram(w);
localStorage.clear();
report({details: d.length, closed: closed, opened: opened, dark: dark, light: light});`)
		if got["details"].(float64) == 0 || got["closed"] != true || got["opened"] != true {
			fail(t, v, "%s: <details> blocks are not collapsed and expandable: %v", rel, got)
		}
		for _, theme := range []string{"dark", "light"} {
			d, _ := got[theme].(map[string]any)
			if d["svg"] != true || d["theme"] != theme || d["height"].(float64) < 50 || d["count"].(float64) < 2 {
				fail(t, v, "%s: the flowchart is not drawn as a diagram in the %s theme: %v", rel, theme, d)
			}
			// WCAG AA: 4.5:1 for the labels, 3:1 for the lines.
			if d["text"].(float64) < 4.5 || d["lines"].(float64) < 3 {
				fail(t, v, "%s: the flowchart is not readable in the %s theme (text %.2f:1, lines %.2f:1)", rel, theme, d["text"], d["lines"])
			}
		}
	}
}

// unescape reads a link the way a browser follows it: the renderer
// percent-encodes characters such as Hangul in anchors.
func unescape(href string) string {
	if out, err := url.PathUnescape(href); err == nil {
		return out
	}
	return href
}

var (
	preRE = regexp.MustCompile(`(?s)<pre([^>]*)>(.*?)</pre>`)
	tagRE = regexp.MustCompile(`<[^>]+>`)
)

// sameColor compares a hex color with a computed value, which registered
// custom properties report as rgb().
func sameColor(got any, hex string) bool {
	text, _ := got.(string)
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return strings.EqualFold(strings.TrimSpace(text), hex) || strings.ReplaceAll(text, " ", "") == fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// ------------------------------------------------------------- AC-4

func TestPagesSiteAC4_Links(t *testing.T) {
	const v = "links-broken"
	dir := site(t, v)
	docs := documents(t)
	facts := publicDocs(t, docs)
	pages := map[string]doc{}
	for _, d := range docs {
		pages[d.Path] = d
	}
	tracked := map[string]bool{}
	dirs := map[string]bool{}
	for _, name := range strings.Split(git(t, repoRoot(t), "ls-files", "-z"), "\x00") {
		if name == "" {
			continue
		}
		tracked[name] = true
		for p := path.Dir(name); p != "." && p != "/"; p = path.Dir(p) {
			dirs[p] = true
		}
	}
	for _, d := range docs {
		rel := d.URL + "index.html"
		body := article(t, page(t, dir, rel, v), rel, v)
		hrefs := map[string]bool{}
		for _, l := range links(body) {
			hrefs[unescape(l.href)] = true
		}
		// Every link of the source appears with its site target.
		for _, href := range facts[d.Path].Links {
			u, err := url.Parse(href)
			if err != nil {
				fail(t, v, "%s: unreadable link %q", d.Path, href)
			}
			want := href
			if u.Scheme == "" && u.Host == "" && u.Path != "" {
				target := path.Clean(path.Join(path.Dir(d.Path), u.Path))
				suffix := ""
				if _, frag, ok := strings.Cut(href, "#"); ok && frag != "" {
					suffix = "#" + frag
				}
				switch {
				case pages[target].URL != "":
					want = relURL(d.URL, pages[target].URL) + suffix
				case target == ".":
					want = repoURL + "/tree/main" + suffix
				case tracked[target]:
					want = repoURL + "/blob/main/" + (&url.URL{Path: target}).EscapedPath() + suffix
				case dirs[target]:
					want = repoURL + "/tree/main/" + (&url.URL{Path: target}).EscapedPath() + suffix
				default:
					fail(t, v, "%s links %q, which is not in the repository", d.Path, href)
				}
			}
			if !hrefs[unescape(want)] && !strings.Contains(body, `src="`+html.EscapeString(want)) {
				fail(t, v, "%s: link %q is not shown as %q", d.URL, href, want)
			}
		}
		// Every relative link on the page opens a built page and heading.
		for _, l := range links(body) {
			if strings.HasPrefix(l.href, "https://") || strings.HasPrefix(l.href, "http://") || strings.HasPrefix(l.href, "mailto:") {
				if strings.HasPrefix(l.href, repoURL+"/blob/main/") || strings.HasPrefix(l.href, repoURL+"/tree/main/") {
					p := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(l.href, repoURL+"/blob/main/"), repoURL+"/tree/main/"), "#", 2)[0]
					p, _ = url.PathUnescape(p)
					if !tracked[p] && !dirs[p] {
						fail(t, v, "%s links GitHub path %q, which is not tracked", d.URL, p)
					}
				}
				continue
			}
			target, fragment := resolve(d.URL, l.href)
			text, err := os.ReadFile(fileFor(dir, target))
			if err != nil {
				fail(t, v, "%s links %q, which opens no built page", d.URL, l.href)
			}
			if fragment != "" {
				frag, _ := url.PathUnescape(fragment)
				if !ids(string(text))[frag] {
					fail(t, v, "%s links %q, whose heading is not on %s", d.URL, l.href, target)
				}
			}
		}
	}
	for rel, want := range map[string]string{
		"en/guides/spar/index.html":    `href="../usage/#review-gate-run-rules5"`,
		"en/guides/install/index.html": `href="#install-from-a-release-file"`,
	} {
		if !strings.Contains(page(t, dir, rel, v), want) {
			fail(t, v, "%s lacks %s", rel, want)
		}
	}
	// A link to the repository root, which check_public_docs accepts, opens
	// the repository on GitHub.
	rootLink := synthetic(t, func(root string) {
		p := filepath.Join(root, "guides", "USAGE.md")
		data, _ := os.ReadFile(p)
		write(t, p, string(data)+"\n[the repository](../) and [again](..)\n")
	})
	res := runBuild(t, rootLink, filepath.Join(t.TempDir(), "out"))
	if res.err != nil || res.code != 0 {
		fail(t, v, "a link to the repository root stops the build: %v %s", res.err, res.stderr)
	}
	body := article(t, page(t, res.out, "ko/guides/usage/index.html", v), "ko/guides/usage/", v)
	if strings.Count(body, `href="`+repoURL+`/tree/main"`) != 2 {
		fail(t, v, "a link to the repository root does not open %s/tree/main", repoURL)
	}
	// A link the repository cannot resolve stops the build, naming file and link.
	for _, bad := range []string{"[gone](MISSING-PAGE.md)", "[nowhere](INSTALL.md#no-such-heading-42)"} {
		syn := synthetic(t, func(root string) {
			p := filepath.Join(root, "guides", "USAGE.md")
			data, _ := os.ReadFile(p)
			write(t, p, string(data)+"\n"+bad+"\n")
		})
		out := filepath.Join(t.TempDir(), "out")
		res := runBuild(t, syn, out)
		target := strings.TrimSuffix(strings.SplitN(bad, "(", 2)[1], ")")
		if res.err != nil || res.code != 1 || !strings.Contains(res.stderr, "guides/USAGE.md") || !strings.Contains(res.stderr, target) {
			fail(t, v, "a broken link %s did not stop the build naming the file and link (exit %d): %s", bad, res.code, res.stderr)
		}
		if _, err := os.Stat(out); err == nil {
			fail(t, v, "a failed build still wrote %s", out)
		}
	}
}

// ------------------------------------------------------------- AC-5

func TestPagesSiteAC5_DocsView(t *testing.T) {
	const v = "docs-view-missing"
	dir := site(t, v)
	for _, d := range documents(t) {
		rel := d.URL + "index.html"
		text := page(t, dir, rel, v)
		body := article(t, text, rel, v)
		for _, part := range []string{`<nav class="side"`, `<article class="article"`, `<aside class="toc"`, `id="copy-page"`,
			`href="` + repoURL + "/blob/main/" + d.Path + `"`} {
			if !strings.Contains(text, part) {
				fail(t, v, "%s lacks %s", d.URL, part)
			}
		}
		toc := regexp.MustCompile(`(?s)<ul id="toc-list">(.*?)</ul>`).FindStringSubmatch(text)
		if toc == nil {
			fail(t, v, "%s has no table of contents", d.URL)
		}
		for _, m := range regexp.MustCompile(`<h([23]) id="([^"]+)"`).FindAllStringSubmatch(body, -1) {
			if !strings.Contains(toc[1], `href="#`+m[2]+`"`) {
				fail(t, v, "%s lists no %s in its table of contents", d.URL, m[2])
			}
		}
	}
	for _, lang := range []string{"ko", "en"} {
		var entries [][]string
		if err := json.Unmarshal([]byte(page(t, dir, lang+"/search.json", v)), &entries); err != nil {
			fail(t, v, "%s/search.json is not readable: %v", lang, err)
		}
		urls := map[string]bool{}
		for _, e := range entries {
			urls[e[2]] = true
		}
		nav := page(t, dir, lang+"/docs/index.html", v)
		listed := regexp.MustCompile(`<a href="([^"#]+)" hreflang`).FindAllStringSubmatch(nav, -1)
		if len(listed) == 0 {
			fail(t, v, "%s/docs/ lists no documents", lang)
		}
		for _, m := range listed {
			target, _ := resolve(lang+"/docs/", m[1])
			if !urls[target] {
				fail(t, v, "%s search does not find %s", lang, target)
			}
			body := article(t, page(t, dir, target+"index.html", v), target, v)
			for _, h := range regexp.MustCompile(`<h[234] id="([^"]+)"`).FindAllStringSubmatch(body, -1) {
				if !urls[target+"#"+h[1]] {
					fail(t, v, "%s search does not find heading %s of %s", lang, h[1], target)
				}
			}
		}
	}
	var script strings.Builder
	script.WriteString("var narrow = {};\n")
	for _, p := range widePages {
		fmt.Fprintf(&script, "var w = await load(%q, 375, 812); await wait(600); narrow[%q] = [w.document.documentElement.scrollWidth, w.document.documentElement.clientWidth];\n", "/"+p, p)
	}
	script.WriteString(`
var kb = {};
// The navigation at 375px, by keyboard alone: Tab reaches the menu button,
// Enter opens the navigation with focus in it, Escape closes it and returns
// focus, Tab moves along its links and Enter follows one.
var w = await load("/en/guides/usage/", 375, 812);
await wait(300);
w.focus();
var menu = w.document.getElementById("menu-btn"), side = w.document.getElementById("side");
kb.menuShown = !!menu && w.getComputedStyle(menu).display !== "none";
for (var i = 0; i < 40 && w.document.activeElement !== menu; i++) await press("Tab");
kb.menuByTab = w.document.activeElement === menu;
await press("Enter"); await wait(400);
kb.navOpen = w.document.documentElement.classList.contains("nav-open") && side.getBoundingClientRect().left >= -1 && w.getComputedStyle(side).visibility === "visible";
kb.focusInNav = side.contains(w.document.activeElement);
await press("Escape"); await wait(400);
kb.navClosed = !w.document.documentElement.classList.contains("nav-open") && w.document.activeElement === menu;
await press("Enter"); await wait(400);
await press("Tab");
var next = w.document.activeElement, nextPath = next.pathname;
kb.nextInNav = side.contains(next) && next.tagName === "A" && nextPath !== "/en/guides/usage/";
var nav = navigated(); await press("Enter");
var w2 = await Promise.race([nav, wait(5000).then(function () { return null; })]);
kb.navFollowed = !!w2 && w2.location.pathname === nextPath;
// At full width: Tab reaches the navigation, Shift+Tab the "On this page" list,
// and Enter on its link goes to that heading, which the list then marks.
var w = await load("/en/guides/usage/", 1280, 900);
await wait(300);
w.focus();
function inside(sel) { var a = w.document.activeElement; return !!(a && a.closest && a.closest(sel)); }
for (var i = 0; i < 40 && !inside("#side"); i++) await press("Tab");
kb.sideByTab = inside("#side");
var w = await load("/en/guides/usage/", 1280, 900);
await wait(300);
w.focus();
for (var i = 0; i < 40 && !inside(".toc"); i++) await press("Tab", 8);
kb.tocByTab = inside(".toc");
if (kb.tocByTab) {
  var hash = w.document.activeElement.getAttribute("href");
  await press("Enter"); await wait(400);
  var h = w.document.getElementById(decodeURIComponent(hash.slice(1)));
  var on = w.document.querySelector("#toc-list a.on");
  kb.tocFollowed = w.location.hash === hash && !!h && h.getBoundingClientRect().top >= 0 && h.getBoundingClientRect().top < 300;
  kb.tocMarked = !!on && on.getAttribute("href") === hash;
}
// The "On this page" list marks the section being read, scrolling down and up.
var w = await load("/en/guides/usage/", 1280, 900);
await wait(300);
var heads = Array.prototype.map.call(w.document.querySelectorAll("#toc-list a"), function (a) { return w.document.getElementById(decodeURIComponent(a.getAttribute("href").slice(1))); });
function top(h) { return h.getBoundingClientRect().top + w.scrollY; }
function marked() { var on = w.document.querySelectorAll("#toc-list a.on"); return on.length === 1 ? on[0].getAttribute("href") : String(on.length); }
var spy = {};
for (var i = 0; i + 2 < heads.length; i++) {
  if (top(heads[i + 1]) - top(heads[i]) > 600 && top(heads[i + 2]) - top(heads[i + 1]) > 600) {
    w.scrollTo(0, top(heads[i + 1]) - 5); await wait(300);
    spy.down = [marked(), "#" + heads[i + 1].id];
    w.scrollTo(0, top(heads[i]) - 5); await wait(300);
    spy.up = [marked(), "#" + heads[i].id];
    break;
  }
}
// Search by keyboard: /, ⌘K and Ctrl+K open it, Escape closes it, typed text
// filters, the arrow keys move the choice and Enter goes to it.
var w = await load("/en/guides/usage/", 1280, 900);
await wait(300);
w.focus();
var veil = w.document.getElementById("palette"), input = w.document.getElementById("palette-input");
var opened = {};
await press("/"); opened.slash = !veil.hidden && w.document.activeElement === input;
await press("Escape"); opened.escape = veil.hidden;
await press("k", 4); opened.meta = !veil.hidden && w.document.activeElement === input;
await press("Escape");
await press("k", 2); opened.ctrl = !veil.hidden && w.document.activeElement === input;
await typeText("review");
await wait(1500);
var options = w.document.querySelectorAll("#palette-list button");
await press("ArrowDown");
opened.arrowDown = options.length > 1 && options[1].getAttribute("aria-selected") === "true";
await press("ArrowUp");
opened.arrowUp = options.length > 1 && options[0].getAttribute("aria-selected") === "true";
await press("Escape");
await press("/");
await typeText("set up a project");
await wait(300);
var options = w.document.querySelectorAll("#palette-list button");
var firstHref = options.length ? options[0].getAttribute("data-href") : null;
var nav = navigated(); await press("Enter");
var w2 = await Promise.race([nav, wait(5000).then(function () { return null; })]);
var landed = w2 ? w2.location.pathname + w2.location.hash : null;
// A word only the text of a section holds: "waive" in the Spar guide.
var w = await load("/en/guides/usage/", 1280, 900);
await wait(300);
w.focus();
await press("k", 4);
await typeText("waive");
await wait(1500);
var options = Array.prototype.slice.call(w.document.querySelectorAll("#palette-list button"));
var pick = options.findIndex(function (b) { return b.getAttribute("data-href").indexOf("en/guides/spar/#") === 0; });
for (var i = 0; i < pick; i++) await press("ArrowDown");
var waive = {href: pick >= 0 ? options[pick].getAttribute("data-href") : null};
var nav = navigated(); await press("Enter");
var w2 = await Promise.race([nav, wait(5000).then(function () { return null; })]);
waive.landed = w2 ? w2.location.pathname + w2.location.hash : null;
if (w2 && w2.location.hash) {
  var h = w2.document.getElementById(decodeURIComponent(w2.location.hash.slice(1))), text = "";
  for (var n = h ? h.nextElementSibling : null; n && !/^H[234]$/.test(n.tagName); n = n.nextElementSibling) text += " " + n.textContent;
  waive.inSection = /waive/i.test(text);
}
var w = await load("/en/guides/usage/", 1280, 900);
await wait(300);
var copied = null;
Object.defineProperty(w.navigator, "clipboard", {configurable: true, value: {writeText: function (t) { copied = t; return Promise.resolve(); }}});
w.document.getElementById("copy-page").click();
await wait(200);
report({narrow: narrow, kb: kb, spy: spy, opened: opened, firstHref: firstHref, landed: landed, waive: waive, copied: copied});`)
	got := browse(t, dir, script.String())
	for p, pair := range got["narrow"].(map[string]any) {
		dims := pair.([]any)
		if dims[0].(float64) > dims[1].(float64) {
			fail(t, v, "%s scrolls sideways at 375px (%v > %v)", p, dims[0], dims[1])
		}
	}
	kb, _ := got["kb"].(map[string]any)
	for _, step := range []string{"menuShown", "menuByTab", "navOpen", "focusInNav", "navClosed", "nextInNav", "navFollowed",
		"sideByTab", "tocByTab", "tocFollowed", "tocMarked"} {
		if kb[step] != true {
			fail(t, v, "the navigation and \"On this page\" list are not usable by keyboard alone: %s fails (%v)", step, kb)
		}
	}
	spy, _ := got["spy"].(map[string]any)
	for _, way := range []string{"down", "up"} {
		pair, _ := spy[way].([]any)
		if len(pair) != 2 || pair[0] != pair[1] {
			fail(t, v, "scrolling %s, \"On this page\" does not mark the section being read: %v", way, spy)
		}
	}
	for name, ok := range got["opened"].(map[string]any) {
		if ok != true {
			fail(t, v, "the search palette does not respond to %s: %v", name, got["opened"])
		}
	}
	if got["firstHref"] != "en/guides/spar/#set-up-a-project" || got["landed"] != "/en/guides/spar/#set-up-a-project" {
		fail(t, v, "searching 'set up a project' and pressing Enter went to %v (first result %v)", got["landed"], got["firstHref"])
	}
	waive, _ := got["waive"].(map[string]any)
	href, _ := waive["href"].(string)
	if !strings.HasPrefix(href, "en/guides/spar/#") || waive["landed"] != "/"+href || waive["inSection"] != true {
		fail(t, v, "searching 'waive' does not reach the Spar guide's section that holds it: %v", waive)
	}
	if got["copied"] != readFile(t, filepath.Join(repoRoot(t), "guides", "USAGE.en.md")) {
		fail(t, v, "Copy page does not copy guides/USAGE.en.md as it is")
	}
}

// ------------------------------------------------------------- AC-6

func TestPagesSiteAC6_LanguageAndTheme(t *testing.T) {
	const v = "lang-theme-missing"
	dir := site(t, v)
	docs := documents(t)
	have := map[string]bool{}
	for _, d := range docs {
		have[d.URL] = true
	}
	switchRE := regexp.MustCompile(`<a class="ghost-btn lang" href="([^"]+)" hreflang="([a-z]+)" data-set-lang="([a-z]+)"`)
	check := func(rel, want string) {
		text := page(t, dir, rel+"index.html", v)
		m := switchRE.FindStringSubmatch(text)
		if m == nil {
			fail(t, v, "%s has no language switch", rel)
		}
		target, _ := resolve(rel, m[1])
		if target != want {
			fail(t, v, "%s switches language to %q, want %q", rel, target, want)
		}
	}
	for _, d := range docs {
		other := map[string]string{"ko": "en", "en": "ko"}[d.Lang]
		counterpart := other + strings.TrimPrefix(d.URL, d.Lang)
		if have[counterpart] {
			check(d.URL, counterpart)
		} else {
			check(d.URL, other+"/docs/")
		}
	}
	check("ko/", "en/")
	check("en/", "ko/")
	check("ko/docs/", "en/docs/")
	check("en/docs/", "ko/docs/")
	for _, rel := range []string{"ko/", "en/", "ko/docs/", "en/docs/"} {
		if got := langAttr(page(t, dir, rel+"index.html", v)); got != rel[:2] {
			fail(t, v, "%s has lang %q", rel, got)
		}
	}
	css := page(t, dir, "assets/site.css", v)
	if !strings.Contains(css, "@media (prefers-color-scheme: light)") || !strings.Contains(css, `:root[data-theme="light"]`) {
		fail(t, v, "the theme does not follow the system setting")
	}
	got := browse(t, dir, `
var out = {};
localStorage.clear();
var w = await load("/", 800, 600); await wait(800); out.none = frame.contentWindow.location.pathname;
localStorage.setItem("jaekit-lang", "ko");
await load("/", 800, 600); await wait(800); out.ko = frame.contentWindow.location.pathname;
localStorage.setItem("jaekit-lang", "en");
await load("/", 800, 600); await wait(800); out.en = frame.contentWindow.location.pathname;
localStorage.clear();
var w = await load("/en/guides/usage/", 1280, 800); await wait(300);
var nav = navigated();
w.document.querySelector("a.lang").click();
var w2 = await nav;
out.switched = w2.location.pathname;
out.stored = localStorage.getItem("jaekit-lang");
localStorage.setItem("jaekit-theme", "light");
var w = await load("/ko/", 1280, 800); await wait(300);
out.theme = w.document.documentElement.getAttribute("data-theme");
out.bg = w.getComputedStyle(w.document.documentElement).getPropertyValue("--bg").trim();
out.term = w.getComputedStyle(w.document.getElementById("term")).getPropertyValue("--t-bg").trim();
w.document.getElementById("theme-btn").click(); await wait(100);
out.toggled = w.document.documentElement.getAttribute("data-theme");
out.storedTheme = localStorage.getItem("jaekit-theme");
var w = await load("/ko/", 1280, 800); await wait(300);
out.remembered = w.document.documentElement.getAttribute("data-theme");
report(out);`)
	want := map[string]string{"none": "/en/", "ko": "/ko/", "en": "/en/", "switched": "/ko/guides/usage/", "stored": "ko",
		"theme": "light", "bg": "#FAFAFA", "toggled": "dark", "storedTheme": "dark", "remembered": "dark"}
	for k, w := range want {
		if got[k] != w {
			fail(t, v, "%s: got %v, want %q (%v)", k, got[k], w, got)
		}
	}
	if !sameColor(got["term"], "#131110") {
		fail(t, v, "the terminal is not dark in the light theme: --t-bg %v", got["term"])
	}
	korean := browse(t, dir, `localStorage.clear(); await load("/", 800, 600); await wait(800); report({path: frame.contentWindow.location.pathname});`, "--accept-lang=ko-KR")
	if korean["path"] != "/ko/" {
		fail(t, v, "a Korean browser without a stored choice opens %v", korean["path"])
	}
	root := page(t, dir, "index.html", v)
	for _, want := range []string{`href="ko/"`, `href="en/"`} {
		if !strings.Contains(root, want) {
			fail(t, v, "the root page lacks the %s fallback link", want)
		}
	}
	// With storage blocked (reading it throws, as when a browser blocks site
	// data) the root page stays and shows both language links.
	blocked := browseWith(t, dir, `Object.defineProperty(window, "localStorage", {configurable: true, get: function () { throw new DOMException("blocked", "SecurityError"); }});`, `
await load("/", 800, 600); await wait(1200);
var d = frame.contentWindow.document, shown = 0;
Array.prototype.forEach.call(d.querySelectorAll('a[href="ko/"], a[href="en/"]'), function (a) {
  var r = a.getBoundingClientRect();
  if (r.width > 0 && r.height > 0 && frame.contentWindow.getComputedStyle(a).visibility === "visible") shown++;
});
var blockedHere = false;
try { frame.contentWindow.localStorage; } catch (e) { blockedHere = true; }
report({path: frame.contentWindow.location.pathname, shown: shown, blocked: blockedHere});`)
	if blocked["blocked"] != true {
		t.Skip("the browser did not block storage for the observation")
	}
	if blocked["path"] != "/" || blocked["shown"].(float64) < 2 {
		fail(t, v, "with storage unavailable the root page does not show both language links: %v", blocked)
	}
}

// ------------------------------------------------------------- AC-7

func TestPagesSiteAC7_Home(t *testing.T) {
	const v = "home-missing"
	dir := site(t, v)
	css := page(t, dir, "assets/site.css", v)
	for _, token := range []string{"--bg: #0A0A0B", "--fg: #EDEDEF", "--accent: #8094FF", "--bg: #FAFAFA", "--fg: #0B0B0D", "--accent: #3A52E0"} {
		if !strings.Contains(css, token) {
			fail(t, v, "site.css lacks the prototype token %q", token)
		}
	}
	symbol, err := os.ReadFile(filepath.Join(repoRoot(t), "assets", "readme", "brand", "symbol.png"))
	if err != nil {
		t.Skip(err)
	}
	if !bytes.Equal([]byte(page(t, dir, "assets/symbol.png", v)), symbol) {
		fail(t, v, "assets/symbol.png is not assets/readme/brand/symbol.png")
	}
	shape := map[string][]int{}
	for _, lang := range []string{"ko", "en"} {
		rel := lang + "/"
		text := page(t, dir, rel+"index.html", v)
		install := installLine(t, map[string]string{"ko": "guides/INSTALL.md", "en": "guides/INSTALL.en.md"}[lang])
		for _, want := range []string{
			`<span class="p">$ </span>` + install + `</code>`, `data-copy="` + install + `"`,
			`id="get-started" href="../` + lang + `/guides/install/"`,
			`<canvas id="matrix" data-symbol="../assets/symbol.png"`,
			`id="how"`, `id="try"`, `id="records"`, `id="principles"`, `id="start"`,
			`<a href="` + repoURL + `">github</a>`, `<a href="` + repoURL + `/releases">releases</a>`,
		} {
			if !strings.Contains(text, want) {
				fail(t, v, "%s lacks %s", rel, want)
			}
		}
		shape[lang] = []int{strings.Count(text, `class="step"`), strings.Count(text, "<h3>"), strings.Count(text, "<li><code>"), strings.Count(text, "<section")}
	}
	if fmt.Sprint(shape["ko"]) != fmt.Sprint(shape["en"]) {
		fail(t, v, "the Korean and English homes differ in structure: %v", shape)
	}
	js := page(t, dir, "assets/home.js", v)
	for _, want := range []string{"prefers-reduced-motion", "pointermove", `"click"`} {
		if !strings.Contains(js, want) {
			fail(t, v, "home.js lacks %s", want)
		}
	}
	frames := `
async function frames(w) {
  var c = w.document.getElementById("matrix"), x = c.getContext("2d");
  function snap() { var d = x.getImageData(0, 0, c.width, c.height).data, ink = 0, h = 0; for (var i = 3; i < d.length; i += 4) { if (d[i]) ink++; h = (h * 31 + d[i] + d[i - 1]) | 0; } return [ink, h]; }
  function corner() { var d = x.getImageData(0, 0, Math.round(c.width / 4), Math.round(c.height / 4)).data, h = 0; for (var i = 0; i < d.length; i++) h = (h * 31 + d[i]) | 0; return h; }
  var a = snap(); await wait(700); var b = snap();
  var r = c.getBoundingClientRect();
  c.dispatchEvent(new w.PointerEvent("pointermove", {clientX: r.left + r.width / 2, clientY: r.top + r.height / 2, pointerType: "mouse", bubbles: true}));
  await wait(200); var p = snap();
  // The corner holds no part of the symbol, so only a ripple changes it.
  var c1 = corner(); await wait(300); var c2 = corner();
  c.dispatchEvent(new w.MouseEvent("click", {clientX: r.left + 12, clientY: r.top + 12, bubbles: true}));
  await wait(150); var c3 = corner();
  return {ink: a[0], still: a[1] === b[1] && b[1] === p[1], cornerStill: c1 === c2, ripple: c2 !== c3};
}
var w = await load("/ko/", 1280, 900); await wait(1500);
report(await frames(w));`
	moving := browse(t, dir, frames)
	if moving["ink"].(float64) < 1000 || moving["still"] == true {
		fail(t, v, "the symbol matrix is not drawn and moving: %v", moving)
	}
	if moving["cornerStill"] != true || moving["ripple"] != true {
		fail(t, v, "a click on the symbol matrix does not spread a ripple: %v", moving)
	}
	still := browse(t, dir, frames, "--force-prefers-reduced-motion")
	if still["ink"].(float64) < 1000 || still["still"] != true || still["ripple"] == true {
		fail(t, v, "the symbol matrix is not a still frame with reduced motion: %v", still)
	}
}

// ------------------------------------------------------------- AC-8

func TestPagesSiteAC8_Terminal(t *testing.T) {
	const v = "terminal-missing"
	dir := site(t, v)
	css := page(t, dir, "assets/site.css", v)
	for _, token := range []string{`.term[data-agent="claude"] { --t-accent: #C98D6E`, `.term[data-agent="codex"] { --t-accent: #A09BC4`} {
		if !strings.Contains(css, token) {
			fail(t, v, "site.css lacks %q", token)
		}
	}
	for lang, caption := range map[string]string{"ko": "가상 예시", "en": "Fictional"} {
		text := page(t, dir, lang+"/index.html", v)
		if !regexp.MustCompile(`<p class="term-caption"[^>]*>` + caption).MatchString(text) {
			fail(t, v, "the %s terminal is not labelled %q", lang, caption)
		}
	}
	session := `
var w = await load("/%s/", 1280, 900); await wait(500);
var d = w.document, out = d.getElementById("term-out");
function lines(kind) { return d.querySelectorAll("#term-out .ln" + (kind ? "." + kind : "")).length; }
var r = {boot1: d.querySelector("#term-out .boot b").textContent, agent1: d.getElementById("term").getAttribute("data-agent"),
         accent1: w.getComputedStyle(d.getElementById("term")).getPropertyValue("--t-accent").trim()};
var next = d.querySelector("#term-next button");
r.next1 = next ? next.textContent : null;
next.click();
r.immediate = lines("");
await wait(4000);
r.after = lines("");
r.you = d.querySelector("#term-out .ln.you span:last-child").textContent;
d.querySelector('[data-set-agent="codex"]').click();
await wait(800);
r.agent2 = d.getElementById("term").getAttribute("data-agent");
r.boot2 = d.querySelector("#term-out .boot b").textContent;
r.youAfterSwitch = lines("you");
r.next2 = d.querySelector("#term-next button").textContent;
await wait(800);
r.accent2 = w.getComputedStyle(d.getElementById("term")).getPropertyValue("--t-accent").trim();
r.caption = d.querySelector(".term-caption").textContent;
report(r);`
	for _, lang := range []string{"ko", "en"} {
		got := browse(t, dir, fmt.Sprintf(session, lang))
		if !strings.HasPrefix(got["boot1"].(string), "claude code") || got["agent1"] != "claude" || !strings.HasPrefix(got["next1"].(string), "/spec:spec") || !strings.HasPrefix(got["you"].(string), "/spec:spec") {
			fail(t, v, "%s: the Claude Code session does not start with /spec:spec: %v", lang, got)
		}
		if got["agent2"] != "codex" || !strings.HasPrefix(got["boot2"].(string), "codex") || got["youAfterSwitch"].(float64) != 0 || !strings.HasPrefix(got["next2"].(string), "$spec:spec") {
			fail(t, v, "%s: choosing Codex does not restart the session with $spec:spec: %v", lang, got)
		}
		if !sameColor(got["accent1"], "#C98D6E") || !sameColor(got["accent2"], "#A09BC4") {
			fail(t, v, "%s: the terminal colors do not follow the agent: %v", lang, got)
		}
		if got["immediate"].(float64) >= got["after"].(float64) {
			fail(t, v, "%s: the session shows a step at once without reduced motion: %v", lang, got)
		}
	}
	reduced := browse(t, dir, fmt.Sprintf(session, "ko"), "--force-prefers-reduced-motion")
	if reduced["immediate"].(float64) != reduced["after"].(float64) {
		fail(t, v, "with reduced motion a step still types out line by line: %v", reduced)
	}
}

// ------------------------------------------------------------- AC-9

func TestPagesSiteAC9_Version(t *testing.T) {
	const v = "version-not-from-release"
	dir := site(t, v)
	version := releaseVersion(t)
	if version == "" {
		t.Skip("marketplace.json names no version")
	}
	release := repoURL + "/releases/tag/v" + version
	anchorRE := regexp.MustCompile(`(?s)<a\b[^>]*\bhref="([^"]*)"[^>]*>(.*?)</a>`)
	shownRE := regexp.MustCompile(`(^|[^\w.])v` + regexp.QuoteMeta(version) + `($|[^\w.])`)
	// linked counts the links to the release that show the version, and says
	// whether the fragment shows the version anywhere else.
	linked := func(fragment string) (int, bool) {
		n := 0
		rest := anchorRE.ReplaceAllStringFunc(fragment, func(a string) string {
			m := anchorRE.FindStringSubmatch(a)
			if html.UnescapeString(m[1]) == release && shownRE.MatchString(tagRE.ReplaceAllString(m[2], " ")) {
				n++
				return ""
			}
			return a
		})
		return n, !shownRE.MatchString(tagRE.ReplaceAllString(rest, " "))
	}
	footerRE := regexp.MustCompile(`(?s)<footer class="foot">.*?</footer>`)
	codeRE := regexp.MustCompile(`(?s)<script\b.*?</script>|<meta\b[^>]*>`)
	for name, data := range walk(t, dir) {
		if !strings.HasSuffix(name, ".html") || name == "404.html" {
			continue
		}
		text := string(data)
		if got := meta(text, "jaekit-version"); got != version {
			fail(t, v, "%s names version %q, not %q from marketplace.json", name, got, version)
		}
		if name == "index.html" { // the language chooser shows no version
			continue
		}
		footer := footerRE.FindString(text)
		if n, only := linked(footer); n != 1 || !only {
			fail(t, v, "%s's footer does not show v%s linked to %s", name, version, release)
		}
	}
	for _, lang := range []string{"ko", "en"} {
		text := codeRE.ReplaceAllString(page(t, dir, lang+"/index.html", v), "")
		if !strings.Contains(text, `id="version-pill" href="`+release+`"><b>v`+version+`</b>`) {
			fail(t, v, "%s/ does not show v%s linked to its release", lang, version)
		}
		if n, only := linked(text); n != 2 || !only {
			fail(t, v, "%s/ shows the version without its release link (%d linked)", lang, n)
		}
	}
	cases := map[string]func(root string){
		"missing": func(root string) { os.Remove(filepath.Join(root, ".claude-plugin", "marketplace.json")) },
		"invalid": func(root string) { write(t, filepath.Join(root, ".claude-plugin", "marketplace.json"), "{not json") },
		"empty": func(root string) {
			write(t, filepath.Join(root, ".claude-plugin", "marketplace.json"), `{"metadata": {"version": ""}}`)
		},
		"absent": func(root string) {
			write(t, filepath.Join(root, ".claude-plugin", "marketplace.json"), `{"metadata": {}}`)
		},
	}
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		syn := synthetic(t, cases[name])
		out := filepath.Join(t.TempDir(), "out")
		res := runBuild(t, syn, out)
		if res.err != nil || res.code != 1 || !strings.Contains(res.stderr, "marketplace.json") {
			fail(t, v, "a %s release version did not stop the build naming marketplace.json (exit %d): %s", name, res.code, res.stderr)
		}
		if _, err := os.Stat(out); err == nil {
			fail(t, v, "a %s release version still wrote a site", name)
		}
	}
}

// ------------------------------------------------------------- AC-10

func TestPagesSiteAC10_NoExternalRequests(t *testing.T) {
	const v = "external-requests"
	dir := site(t, v)
	loadTag := regexp.MustCompile(`(?i)<(script|link|img|iframe|source|video|audio|embed|object)\b[^>]*\b(src|href|data)\s*=\s*["']?\s*(https?:)?//`)
	cssURL := regexp.MustCompile(`(?i)(url\(\s*["']?\s*(https?:)?//|@import)`)
	for name, data := range walk(t, dir) {
		text := string(data)
		switch {
		case strings.HasSuffix(name, ".html"):
			if m := loadTag.FindString(text); m != "" {
				fail(t, v, "%s loads %q from outside the site", name, m)
			}
		case strings.HasSuffix(name, ".css"):
			if m := cssURL.FindString(text); m != "" {
				fail(t, v, "%s loads %q from outside the site", name, m)
			}
		}
	}
	licenses := page(t, dir, "assets/licenses.txt", v)
	for _, want := range []string{"Geist 5.3.0", "Geist Mono 5.3.0", "Noto Sans KR 5.3.1", "mermaid 11.17.2", "SIL Open Font License", "MIT License"} {
		if !strings.Contains(licenses, want) {
			fail(t, v, "assets/licenses.txt lacks %q", want)
		}
	}
	if !strings.Contains(page(t, dir, "ko/index.html", v), `href="../assets/licenses.txt"`) {
		fail(t, v, "the home page does not link the license notice")
	}
	var script strings.Builder
	script.WriteString("var foreign = [], keys = {};\n")
	for _, p := range append([]string{""}, widePages...) {
		fmt.Fprintf(&script, `var w = await load(%q, 1280, 900); await wait(2500);
w.performance.getEntriesByType("resource").forEach(function (e) { if (new URL(e.name).origin !== location.origin) foreign.push(e.name); });
`, "/"+p)
	}
	script.WriteString(`
var w = await load("/en/guides/usage/", 1280, 900); await wait(300);
w.document.getElementById("theme-btn").click();
w.document.getElementById("search-btn").click(); await wait(800);
var nav = navigated(); w.document.querySelector("a.lang").click(); await nav;
for (var i = 0; i < localStorage.length; i++) keys[localStorage.key(i)] = true;
report({foreign: foreign, cookie: document.cookie + frame.contentWindow.document.cookie, keys: Object.keys(keys).sort()});`)
	got := browse(t, dir, script.String())
	if f := got["foreign"].([]any); len(f) != 0 {
		fail(t, v, "pages requested files outside the site: %v", f)
	}
	if got["cookie"] != "" {
		fail(t, v, "the site set cookies: %v", got["cookie"])
	}
	for _, k := range got["keys"].([]any) {
		if k != "jaekit-lang" && k != "jaekit-theme" {
			fail(t, v, "the site stored %v in the browser", k)
		}
	}
}

// ------------------------------------------------------------- AC-11

func workflowJobs(text string) map[string]string {
	jobs := map[string]string{}
	_, after, ok := strings.Cut(text, "\njobs:\n")
	if !ok {
		return jobs
	}
	name, body := "", []string{}
	for _, line := range strings.Split(after, "\n") {
		if m := regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*$`).FindStringSubmatch(line); m != nil {
			if name != "" {
				jobs[name] = strings.Join(body, "\n")
			}
			name, body = m[1], nil
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		body = append(body, line)
	}
	if name != "" {
		jobs[name] = strings.Join(body, "\n")
	}
	return jobs
}

// jobPermissions reads a job's permissions block: its "scope: level" lines, or
// the value written on the permissions line itself.
func jobPermissions(body string) ([]string, string) {
	var lines []string
	in := false
	for _, line := range strings.Split(body, "\n") {
		if m := regexp.MustCompile(`^    permissions:\s*(.*)$`).FindStringSubmatch(line); m != nil {
			if strings.TrimSpace(m[1]) != "" {
				return nil, strings.TrimSpace(m[1])
			}
			in = true
			continue
		}
		if in {
			if m := regexp.MustCompile(`^      ([\w-]+:\s*\S+)\s*$`).FindStringSubmatch(line); m != nil {
				lines = append(lines, strings.Join(strings.Fields(m[1]), " "))
				continue
			}
			in = false
		}
	}
	return lines, ""
}

func TestPagesSiteAC11_Workflow(t *testing.T) {
	const v = "workflow-missing"
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "pages.yml"))
	if err != nil {
		fail(t, v, "there is no .github/workflows/pages.yml: %v", err)
	}
	text := string(data)
	_, on, _ := strings.Cut(text, "\non:\n")
	on, _, _ = strings.Cut(on, "\n\n")
	for _, want := range []string{"\n  pull_request:", "\n  push:\n    branches: [main]", "\n  workflow_dispatch:"} {
		if !strings.Contains("\n"+on, want) {
			fail(t, v, "pages.yml does not run on %q", strings.TrimSpace(want))
		}
	}
	if !regexp.MustCompile(`(?m)^permissions:\n  contents: read\n\n`).MatchString(text) {
		fail(t, v, "pages.yml's default token permission is not contents: read only")
	}
	uses := regexp.MustCompile(`(?m)^\s+(?:- )?uses: (\S+)(.*)$`).FindAllStringSubmatch(text, -1)
	if len(uses) == 0 {
		fail(t, v, "pages.yml uses no actions")
	}
	for _, u := range uses {
		if !regexp.MustCompile(`^[\w.-]+/[\w.-]+@[0-9a-f]{40}$`).MatchString(u[1]) || !regexp.MustCompile(`^ # v\d`).MatchString(u[2]) {
			fail(t, v, "pages.yml uses %s%s, not pinned to a commit with a version comment", u[1], u[2])
		}
	}
	jobs := workflowJobs(text)
	build, deploy := jobs["build"], jobs["deploy"]
	if build == "" || deploy == "" {
		fail(t, v, "pages.yml lacks the build or deploy job: %v", keys(map[string]bool{"build": build != "", "deploy": deploy != ""}))
	}
	for name, body := range jobs {
		m := regexp.MustCompile(`(?m)^    name: (.+)$`).FindStringSubmatch(body)
		if m == nil {
			fail(t, v, "job %s has no name", name)
		}
		for _, required := range requiredChecks {
			if strings.TrimSpace(m[1]) == required {
				fail(t, v, "job %s is named like the required check %q", name, required)
			}
		}
	}
	// Token rights: the workflow default is contents: read, a job may only
	// narrow it, and only the deploy job holds the Pages rights.
	for name, body := range jobs {
		lines, inline := jobPermissions(body)
		got := strings.Join(lines, ", ")
		switch {
		case inline != "":
			fail(t, v, "job %s sets permissions: %s", name, inline)
		case name == "deploy":
			sort.Strings(lines)
			if got := strings.Join(lines, ", "); got != "id-token: write, pages: write" {
				fail(t, v, "the deploy job has permissions %q, want pages: write and id-token: write", got)
			}
		case got != "" && got != "contents: read":
			fail(t, v, "job %s has permissions %q, more than contents: read", name, got)
		}
	}
	if !strings.Contains(build, "tools/site/build.py") {
		fail(t, v, "the build job does not build the site")
	}
	main := "github.event_name != 'pull_request' && github.ref == 'refs/heads/main'"
	if !strings.Contains(build, "uses: actions/upload-pages-artifact@") || !strings.Contains(build, "if: "+main) {
		fail(t, v, "the build job uploads a Pages artifact outside main pushes and manual runs on main")
	}
	for _, want := range []string{"needs: build", "if: " + main, "pages: write", "id-token: write", "uses: actions/deploy-pages@", "name: github-pages"} {
		if !strings.Contains(deploy, want) {
			fail(t, v, "the deploy job lacks %q", want)
		}
	}
	if !strings.Contains(text, "group: pages-${{ github.ref }}") || !strings.Contains(text, "cancel-in-progress: ${{ github.event_name == 'pull_request' }}") {
		fail(t, v, "pages.yml does not run main deployments one at a time")
	}
	// Queued runs need not start in push order: the deploy step runs only while
	// the run's commit is still main's head, read just before publishing.
	steps := strings.Split(deploy, "\n      - ")
	guard, publish := -1, -1
	for i, step := range steps {
		if strings.Contains(step, "id: latest") && strings.Contains(step, "git ls-remote") && strings.Contains(step, "refs/heads/main") &&
			strings.Contains(step, "${{ github.sha }}") && strings.Contains(step, `latest=true`) {
			guard = i
		}
		if strings.Contains(step, "uses: actions/deploy-pages@") {
			publish = i
			if !strings.Contains(step, "if: steps.latest.outputs.latest == 'true'") {
				fail(t, v, "the deploy step publishes without checking that its commit is main's head")
			}
		}
	}
	if guard < 0 || publish < 0 || guard > publish {
		fail(t, v, "the deploy job does not read main's head before publishing")
	}
	readme := readFile(t, filepath.Join(root, "tools", "site", "README.md"))
	for _, want := range []string{"python3 tools/site/build.py", "macOS", "Linux", "Python 3.10"} {
		if !strings.Contains(readme, want) {
			fail(t, v, "tools/site/README.md does not say %q", want)
		}
	}
	first := site(t, v)
	second := runBuild(t, root, filepath.Join(t.TempDir(), "again"), "--base", "/jaekit/")
	if second.err != nil || second.code != 0 {
		fail(t, v, "a second build failed: %v %s", second.err, second.stderr)
	}
	a, b := walk(t, first), walk(t, second.out)
	if len(a) != len(b) {
		fail(t, v, "two builds of one commit differ in file count: %d and %d", len(a), len(b))
	}
	for name, data := range a {
		if !bytes.Equal(data, b[name]) {
			fail(t, v, "two builds of one commit differ in %s", name)
		}
	}
}

// ------------------------------------------------------------- AC-12

func TestPagesSiteAC12_Cutover(t *testing.T) {
	const v = "cutover-incomplete"
	gh, err := exec.LookPath("gh")
	if err != nil {
		t.Skip("gh is not available to read the Pages settings")
	}
	out, err := exec.Command(gh, "api", "repos/"+ownerRepo+"/pages").Output()
	if err != nil {
		t.Skipf("gh api pages: %v", err)
	}
	var pages struct {
		BuildType string `json:"build_type"`
		HTTPS     bool   `json:"https_enforced"`
		URL       string `json:"html_url"`
	}
	if err := json.Unmarshal(out, &pages); err != nil {
		t.Skipf("gh api pages: %v", err)
	}
	if pages.BuildType != "workflow" {
		fail(t, v, "Pages builds from %q, not GitHub Actions", pages.BuildType)
	}
	if !pages.HTTPS || pages.URL != liveSite {
		fail(t, v, "Pages is %s with HTTPS enforced %v", pages.URL, pages.HTTPS)
	}
	var policies struct {
		Policies []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"branch_policies"`
	}
	if status := getJSON(t, "https://api.github.com/repos/"+ownerRepo+"/environments/github-pages/deployment-branch-policies", &policies); status != 200 {
		t.Skipf("environment policies: HTTP %d", status)
	}
	if len(policies.Policies) != 1 || policies.Policies[0].Name != "main" || policies.Policies[0].Type != "branch" {
		fail(t, v, "github-pages deploys from %v, not main alone", policies.Policies)
	}
	status, live := get(t, liveSite+"ko/")
	if status != 200 || meta(live, "jaekit-commit") != mainHead(t) {
		fail(t, v, "%sko/ (HTTP %d) is not the build of main's latest commit", liveSite, status)
	}
	refs, err := exec.Command("git", "ls-remote", repoURL, "refs/heads/gh-pages").Output()
	if err != nil {
		t.Skipf("git ls-remote: %v", err)
	}
	if strings.TrimSpace(string(refs)) != "" {
		fail(t, v, "the gh-pages branch still exists")
	}
}

// ------------------------------------------------------------- AC-13

// TestPagesSiteAC13_Kept is a maintain check: any problem, including an
// unreachable service, fails it.
func TestPagesSiteAC13_Kept(t *testing.T) {
	root := repoRoot(t)
	var branch struct {
		Protection struct {
			Checks struct {
				Contexts []string `json:"contexts"`
			} `json:"required_status_checks"`
		} `json:"protection"`
	}
	mustJSON(t, "https://api.github.com/repos/"+ownerRepo+"/branches/main", &branch)
	got := append([]string(nil), branch.Protection.Checks.Contexts...)
	sort.Strings(got)
	want := append([]string(nil), requiredChecks...)
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("main requires %v, want exactly %v", got, want)
	}
	var rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			Checks []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
		} `json:"parameters"`
	}
	mustJSON(t, "https://api.github.com/repos/"+ownerRepo+"/rules/branches/main", &rules)
	for _, rule := range rules {
		for _, c := range rule.Parameters.Checks {
			if !strings.Contains("|"+strings.Join(requiredChecks, "|")+"|", "|"+c.Context+"|") {
				t.Fatalf("a ruleset requires %q on main", c.Context)
			}
		}
	}
	builder := exec.Command("python3", "tools/site/build.py")
	builder.Dir = root
	if out, err := builder.CombinedOutput(); err != nil {
		t.Fatalf("the default local build failed: %v\n%s", err, out)
	}
	status := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	status.Dir = root
	out, err := status.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if len(bytes.TrimSpace(out)) != 0 {
		t.Fatalf("a local build left files Git does not ignore:\n%s", out)
	}
	verify := exec.Command("python3", "tools/verify.py")
	verify.Dir = root
	if out, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("python3 tools/verify.py failed: %v\n%s", err, tail(out))
	}
}

func mustJSON(t *testing.T, url string, into any) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("%s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%s: HTTP %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("%s: %v", url, err)
	}
}

func tail(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return strings.Join(lines, "\n")
}

// ------------------------------------------------------------- AC-14

func section(text, heading string) string {
	_, after, ok := strings.Cut(text, "\n"+heading+"\n")
	if !ok {
		return ""
	}
	body, _, _ := strings.Cut(after, "\n## ")
	return body
}

func TestPagesSiteAC14_CIGuide(t *testing.T) {
	const v = "ci-guide-missing"
	root := repoRoot(t)
	guides := map[string]struct {
		heading string
		tokens  []string
	}{
		"guides/CI.md": {"## 사이트 workflow", []string{"](../.github/workflows/pages.yml)", "pull request", "`main` push", "수동 실행",
			"`main`에서만 게시", "필수 체크가 아니", "병합을 막지 않", "이전에 게시된 사이트가 그대로"}},
		"guides/CI.en.md": {"## Site workflow", []string{"](../.github/workflows/pages.yml)", "pull request", "push to `main`", "manual run",
			"publishes only from `main`", "not required checks", "does not block a merge", "previously published site stays"}},
	}
	for name, want := range guides {
		body := section(readFile(t, filepath.Join(root, filepath.FromSlash(name))), want.heading)
		if body == "" {
			fail(t, v, "%s has no %q section", name, want.heading)
		}
		for _, token := range want.tokens {
			if !strings.Contains(body, token) {
				fail(t, v, "%s %q does not say %q", name, want.heading, token)
			}
		}
	}
}
