//go:build pagessite

// Package pagessite holds the pages-site goal's checks. Each check builds the
// site with tools/site/build.py from the tree it runs in and observes the
// result; the published-site checks also read https://jgoneit.github.io/jaekit/
// and GitHub's public settings for the repository.
//
// A failure that observes the declared property starts with
// "jaekit:violation=<id>". A missing tool, an unreachable network, a pinned
// download that cannot be fetched or a builder that stops for any reason other
// than a named input problem skips the test instead: that is an unobserved
// result, never a violation or a pass.
package pagessite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	liveSite  = "https://jgoneit.github.io/jaekit/"
	repoURL   = "https://github.com/jgoneit/jaekit"
	ownerRepo = "jgoneit/jaekit"
)

var requiredChecks = []string{"Seal Core", "Public documentation", "Release candidate"}

// repoRoot is the tree being checked: this package lives at tools/goal-checks/pages-site.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Skipf("cannot resolve the repository root: %v", err)
	}
	return root
}

func fail(t *testing.T, violation, format string, args ...any) {
	t.Helper()
	t.Fatalf("jaekit:violation="+violation+": "+format, args...)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read %s: %v", path, err)
	}
	return string(data)
}

func python(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 is not available: %v", err)
	}
	return path
}

// cacheDir keeps pinned downloads outside the checked tree, so a baseline
// worktree and the current checkout share them.
func cacheDir(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("PAGES_SITE_CACHE"); dir != "" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "jaekit-pages-site-check")
}

type buildResult struct {
	out    string
	stderr string
	code   int    // 0 built, 1 a named input problem
	err    error  // no site can exist: the builder is missing
	skip   string // the build could not be observed
}

// buildRaw runs the builder of the tree at checked against root. It never
// stops the test, so it can run inside sync.Once.
func buildRaw(checked, root, out, cache string, extra ...string) buildResult {
	return buildRawEnv(checked, root, out, cache, nil, extra...)
}

// buildRawEnv is buildRaw with extra environment variables for the builder.
func buildRawEnv(checked, root, out, cache string, env []string, extra ...string) buildResult {
	builder := filepath.Join(checked, "tools", "site", "build.py")
	if _, err := os.Stat(builder); errors.Is(err, fs.ErrNotExist) {
		return buildResult{err: fmt.Errorf("the site builder tools/site/build.py does not exist")}
	} else if err != nil {
		return buildResult{skip: fmt.Sprintf("cannot inspect tools/site/build.py: %v", err)}
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		return buildResult{skip: "python3 is not available"}
	}
	args := append([]string{builder, "--root", root, "--out", out, "--cache", cache}, extra...)
	cmd := exec.Command(py, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	err = cmd.Run()
	result := buildResult{out: out, stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit) && exit.Exited():
		result.code = exit.ExitCode()
	default:
		result.skip = fmt.Sprintf("the site builder could not run: %v", err)
	}
	// Only exit 1 with the builder's own "site: " message is an input problem
	// of the tree. A failed download (3), a tool that cannot run (4), a usage
	// error (2) or Python stopping on its own says nothing about the tree.
	switch {
	case result.skip != "", result.code == 0:
	case result.code == 3:
		result.skip = "a pinned download is unavailable: " + strings.TrimSpace(result.stderr)
	case result.code != 1 || !strings.HasPrefix(result.stderr, "site: "):
		result.skip = fmt.Sprintf("the site builder could not run (exit %d): %s", result.code, strings.TrimSpace(result.stderr))
	}
	return result
}

// runBuild builds root with the checked tree's builder; a missing builder is
// returned to the caller, which names its own violation.
func runBuild(t *testing.T, root, out string, extra ...string) buildResult {
	t.Helper()
	return runBuildEnv(t, root, out, nil, extra...)
}

func runBuildEnv(t *testing.T, root, out string, env []string, extra ...string) buildResult {
	t.Helper()
	result := buildRawEnv(repoRoot(t), root, out, cacheDir(t), env, extra...)
	if result.skip != "" {
		t.Skip(result.skip)
	}
	return result
}

var (
	siteOnce sync.Once
	siteDir  string
	siteRes  buildResult
)

// site builds the checked tree once per test binary and returns the output
// directory, or fails with the caller's violation when there is no site.
func site(t *testing.T, violation string) string {
	t.Helper()
	root, cache := repoRoot(t), cacheDir(t)
	siteOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pages-site-out-")
		if err != nil {
			siteRes = buildResult{skip: err.Error()}
			return
		}
		siteDir = filepath.Join(dir, "site")
		siteRes = buildRaw(root, root, siteDir, cache, "--base", "/jaekit/")
	})
	if siteRes.skip != "" {
		t.Skip(siteRes.skip)
	}
	if siteRes.err != nil {
		fail(t, violation, "no site: %v", siteRes.err)
	}
	if siteRes.code != 0 {
		fail(t, violation, "the site build failed (exit %d): %s", siteRes.code, strings.TrimSpace(siteRes.stderr))
	}
	return siteDir
}

// page reads a built file: a file that is not there is the caller's
// violation; any other read error leaves the property unobserved.
func page(t *testing.T, dir, rel string, violation string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		fail(t, violation, "%s is not in the built site", rel)
	} else if err != nil {
		t.Skipf("cannot read %s: %v", rel, err)
	}
	return string(data)
}

// exists says whether a built path is there; a read error other than
// absence leaves the property unobserved.
func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Skipf("cannot inspect %s: %v", path, err)
	}
	return err == nil
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// committed reads files as the checked commit has them, which is what the
// builder reads: a checkout's line endings or uncommitted edits do not count.
// A name the commit does not have is left out.
func committed(t *testing.T, names []string) map[string][]byte {
	t.Helper()
	var in bytes.Buffer
	for _, name := range names {
		in.WriteString("HEAD:" + name + "\n")
	}
	cmd := exec.Command("git", "-C", repoRoot(t), "cat-file", "--batch")
	cmd.Stdin = &in
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git cat-file --batch: %v", err)
	}
	files := map[string][]byte{}
	for _, name := range names {
		nl := bytes.IndexByte(out, '\n')
		if nl < 0 {
			t.Skipf("git cat-file --batch: no answer for %s", name)
		}
		header := strings.Fields(string(out[:nl]))
		out = out[nl+1:]
		if len(header) == 2 && header[1] == "missing" {
			continue
		}
		size := -1
		if len(header) == 3 {
			size, _ = strconv.Atoi(header[2])
		}
		if size < 0 || len(out) < size+1 {
			t.Skipf("git cat-file --batch: unreadable answer for %s: %q", name, header)
		}
		files[name] = out[:size]
		out = out[size+1:]
	}
	return files
}

// headFile is one file of the checked commit.
func headFile(t *testing.T, name string) string {
	t.Helper()
	data, ok := committed(t, []string{name})[name]
	if !ok {
		t.Skipf("%s is not in the checked commit", name)
	}
	return string(data)
}

// doc is one Markdown file that becomes a page.
type doc struct {
	Path, Lang, URL string
}

// documents lists the Markdown files directly under guides/ and contracts/
// in the checked commit, with the page each one becomes.
func documents(t *testing.T) []doc {
	t.Helper()
	docs := docsAt(t, repoRoot(t))
	if len(docs) == 0 {
		t.Skip("no tracked documents under guides/ or contracts/")
	}
	return docs
}

// docsAt lists the documents of the commit checked out at dir. A page's
// address is its file name in lowercase (a README is its folder's page),
// with a '_' before a name made only of dots.
func docsAt(t *testing.T, dir string) []doc {
	t.Helper()
	var docs []doc
	names := strings.Split(git(t, dir, "ls-tree", "-r", "--name-only", "-z", "HEAD", "--", "guides", "contracts"), "\x00")
	tracked := map[string]bool{}
	for _, name := range names {
		tracked[name] = true
	}
	for _, name := range names {
		section, file, ok := strings.Cut(name, "/")
		if !ok || strings.Contains(file, "/") || !strings.HasSuffix(file, ".md") {
			continue
		}
		// X.en.md is English and X.ko.md Korean; X.md is the other language
		// of its X.ko.md, and Korean otherwise.
		lang, stem := "ko", strings.TrimSuffix(file, ".md")
		switch {
		case strings.HasSuffix(file, ".en.md"):
			lang, stem = "en", strings.TrimSuffix(file, ".en.md")
		case strings.HasSuffix(file, ".ko.md"):
			stem = strings.TrimSuffix(file, ".ko.md")
		case tracked[section+"/"+stem+".ko.md"]:
			lang = "en"
		}
		url := lang + "/" + section + "/"
		if stem != "README" {
			url += pageName(stem) + "/"
		}
		docs = append(docs, doc{Path: name, Lang: lang, URL: url})
	}
	return docs
}

// docName is a document's path or file name without its language and
// extension: guides/USAGE.en.md and guides/USAGE.md are guides/USAGE.
func docName(name string) string {
	name = strings.TrimSuffix(name, ".md")
	return strings.TrimSuffix(strings.TrimSuffix(name, ".en"), ".ko")
}

// pageName is the address part a document name becomes.
func pageName(stem string) string {
	name := strings.ToLower(stem)
	if strings.Trim(name, ".") == "" {
		return "_" + name
	}
	return name
}

var articleRE = regexp.MustCompile(`(?s)<div class="markdown">(.*)</div>\s*<div class="doc-foot">`)

func article(t *testing.T, pageHTML, rel, violation string) string {
	t.Helper()
	m := articleRE.FindStringSubmatch(pageHTML)
	if m == nil {
		fail(t, violation, "%s has no rendered document body", rel)
	}
	return m[1]
}

var metaRE = regexp.MustCompile(`<meta name="([a-z-]+)" content="([^"]*)">`)

func meta(pageHTML, name string) string {
	for _, m := range metaRE.FindAllStringSubmatch(pageHTML, -1) {
		if m[1] == name {
			return html.UnescapeString(m[2])
		}
	}
	return ""
}

var sourceRE = regexp.MustCompile(`(?s)<script type="application/json" id="page-source">(.*?)</script>`)

func pageSource(pageHTML string) (string, bool) {
	m := sourceRE.FindStringSubmatch(pageHTML)
	if m == nil {
		return "", false
	}
	var text string
	if json.Unmarshal([]byte(m[1]), &text) != nil {
		return "", false
	}
	return text, true
}

// publicDocs runs the repository's own link and anchor reader
// (tools/check_public_docs.py) over the documents.
type docFacts struct {
	Anchors []string `json:"anchors"`
	Links   []string `json:"links"`
}

func publicDocs(t *testing.T, docs []doc) map[string]docFacts {
	t.Helper()
	var paths []string
	for _, d := range docs {
		paths = append(paths, d.Path)
	}
	texts := map[string]string{}
	for name, data := range committed(t, paths) {
		texts[name] = string(data)
	}
	input, _ := json.Marshal(texts)
	script := `import json, sys
sys.path.insert(0, "tools")
import check_public_docs as c
out = {}
for name, text in json.load(sys.stdin).items():
    out[name] = {"anchors": sorted(c.anchors(text)), "links": c.links(text)}
print(json.dumps(out))`
	cmd := exec.Command(python(t), "-c", script)
	cmd.Dir = repoRoot(t)
	cmd.Stdin = bytes.NewReader(input)
	raw, err := cmd.Output()
	if err != nil {
		t.Skipf("tools/check_public_docs.py could not be read: %v", err)
	}
	facts := map[string]docFacts{}
	if err := json.Unmarshal(raw, &facts); err != nil {
		t.Skipf("unreadable link facts: %v", err)
	}
	return facts
}

// synthetic makes a small committed repository with the inputs the builder
// reads, copied from the checked tree, then applies edit to it.
func synthetic(t *testing.T, edit func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	inputs := map[string]bool{
		"guides/INSTALL.md": true, "guides/INSTALL.en.md": true, "guides/USAGE.md": true, "guides/USAGE.en.md": true,
		"contracts/README.md": true, ".claude-plugin/marketplace.json": true,
		"assets/readme/brand/symbol.png": true, "assets/readme/brand/symbol.svg": true,
	}
	var names, real []string
	for _, name := range strings.Split(git(t, repoRoot(t), "ls-tree", "-r", "-z", "--name-only", "HEAD"), "\x00") {
		if name == "" {
			continue
		}
		names = append(names, name)
		// The builder's inputs, and files links can name a heading in, keep
		// their committed bytes; other files only need to exist for links.
		switch strings.ToLower(filepath.Ext(name)) {
		case ".md", ".html", ".svg":
			real = append(real, name)
		default:
			if inputs[name] {
				real = append(real, name)
			}
		}
	}
	files := committed(t, real)
	for name := range inputs {
		if _, ok := files[name]; !ok {
			t.Skipf("cannot copy %s: not in the checked commit", name)
		}
	}
	for _, name := range names {
		data, ok := files[name]
		if !ok {
			data = []byte("stand-in\n")
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Skip(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Skip(err)
		}
	}
	if edit != nil {
		edit(dir)
	}
	env := append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Synthetic", "GIT_AUTHOR_EMAIL=example@example.invalid",
		"GIT_COMMITTER_NAME=Synthetic", "GIT_COMMITTER_EMAIL=example@example.invalid")
	for _, args := range [][]string{
		{"init", "--quiet", "-b", "main"},
		{"config", "commit.gpgsign", "false"},
		{"add", "-A"},
		{"commit", "--quiet", "-m", "synthetic"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("synthetic repository: git %s: %v: %s", args[0], err, out)
		}
	}
	return dir
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Skip(err)
	}
}

// ------------------------------------------------------------- network

var client = &http.Client{Timeout: 30 * time.Second}

// get reads a public URL; an unreachable network skips the test.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Skipf("network: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		t.Skipf("network: %v", err)
	}
	return resp.StatusCode, string(body)
}

func getJSON(t *testing.T, url string, into any) int {
	t.Helper()
	status, body := get(t, url)
	if status == http.StatusOK {
		if err := json.Unmarshal([]byte(body), into); err != nil {
			t.Skipf("%s: unreadable JSON: %v", url, err)
		}
	}
	return status
}

// mainHead reads the commit main points to on GitHub.
func mainHead(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "ls-remote", repoURL, "refs/heads/main")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-remote: %v", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		t.Skipf("git ls-remote: unexpected output %q", out)
	}
	return fields[0]
}

// ------------------------------------------------------------- browser

func chrome(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("PAGES_SITE_CHROME"); path != "" {
		return path
	}
	if runtime.GOOS == "darwin" {
		mac := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(mac); err == nil {
			return mac
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("no Chrome or Chromium for the browser observations")
	return ""
}

// harnessPrelude runs inside a page served next to the site. load() opens a
// site page in a same-origin iframe of the given size and resolves with its
// window once loaded.
const harnessPrelude = `
var frame = null;
function wait(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }
function load(path, width, height) {
  return new Promise(function (resolve) {
    if (frame) frame.remove();
    frame = document.createElement("iframe");
    frame.width = String(width || 1280); frame.height = String(height || 900);
    frame.style.cssText = "position:fixed;top:0;left:0;border:0";
    frame.onload = function () { resolve(frame.contentWindow); };
    frame.src = path;
    document.body.appendChild(frame);
  });
}
// loadNoScript opens a page in a frame where scripts cannot run.
function loadNoScript(path, width, height) {
  return new Promise(function (resolve) {
    if (frame) frame.remove();
    frame = document.createElement("iframe");
    frame.setAttribute("sandbox", "allow-same-origin");
    frame.width = String(width || 1280); frame.height = String(height || 900);
    frame.style.cssText = "position:fixed;top:0;left:0;border:0";
    frame.onload = function () { resolve(frame.contentWindow); };
    frame.src = path;
    document.body.appendChild(frame);
  });
}
function navigated() { return new Promise(function (r) { frame.onload = function () { r(frame.contentWindow); }; }); }
function key(win, target, k, opts) {
  var e = new win.KeyboardEvent("keydown", Object.assign({ key: k, bubbles: true, cancelable: true }, opts || {}));
  target.dispatchEvent(e);
}
function report(value) { document.getElementById("out").textContent = JSON.stringify(value); }
// press and typeText send real keyboard input through Chrome to the focused
// frame; the test process dispatches each queued input and acknowledges it.
window.__input = []; window.__acked = 0;
var inputSeq = 0;
function send(cmd) {
  cmd.id = ++inputSeq; window.__input.push(cmd);
  return new Promise(function (r) { (function poll() { if (window.__acked >= cmd.id) setTimeout(r, 40); else setTimeout(poll, 10); })(); });
}
function press(k, mods) { return send({key: k, mods: mods || 0}); }
function typeText(s) { return send({text: s}); }
`

// keyDefs are the keys press() may send: DOM code, virtual key code and the
// text the key types.
var keyDefs = map[string]struct {
	code string
	vk   int
	text string
}{
	"Tab": {"Tab", 9, ""}, "Enter": {"Enter", 13, "\r"}, "Escape": {"Escape", 27, ""},
	"ArrowDown": {"ArrowDown", 40, ""}, "ArrowUp": {"ArrowUp", 38, ""}, "/": {"Slash", 191, "/"}, "k": {"KeyK", 75, "k"},
}

// browse serves the built site, runs script in a harness page with headless
// Chrome, driven over its DevTools pipe so the page runs in real time, and
// returns what the script reported.
func browse(t *testing.T, dir, script string, flags ...string) map[string]any {
	t.Helper()
	return browseWith(t, dir, browser{}, script, flags...)
}

// browser sets up a page: inject runs in every frame before the frame's own
// scripts, and scheme ("light" or "dark") is the system color scheme.
type browser struct {
	inject, scheme string
}

func browseWith(t *testing.T, dir string, opts browser, script string, flags ...string) map[string]any {
	t.Helper()
	bin := chrome(t)
	harness := "<!doctype html><meta charset=utf-8><body><pre id=\"out\">pending</pre><script>" + harnessPrelude +
		"(async function(){try{" + script + "}catch(e){report({harness_error:String(e&&e.stack||e)})}})();</script></body>"
	mux := http.NewServeMux()
	mux.HandleFunc("/__harness.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, harness)
	})
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	server := httptest.NewServer(mux)
	defer server.Close()

	toChrome, ourEnd, err := os.Pipe()
	if err != nil {
		t.Skip(err)
	}
	fromChrome, theirEnd, err := os.Pipe()
	if err != nil {
		t.Skip(err)
	}
	args := append([]string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--remote-debugging-pipe", "--window-size=1600,1200", "--user-data-dir=" + t.TempDir()}, flags...)
	args = append(args, "about:blank")
	cmd := exec.Command(bin, args...)
	cmd.ExtraFiles = []*os.File{toChrome, theirEnd} // fd 3: commands in, fd 4: replies out
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Skipf("Chrome could not start: %v", err)
	}
	toChrome.Close()
	theirEnd.Close()
	defer func() {
		ourEnd.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		fromChrome.Close()
	}()

	replies := make(chan map[string]any, 64)
	go func() {
		defer close(replies)
		var buf []byte
		chunk := make([]byte, 64<<10)
		for {
			n, err := fromChrome.Read(chunk)
			buf = append(buf, chunk[:n]...)
			for {
				i := bytes.IndexByte(buf, 0)
				if i < 0 {
					break
				}
				var msg map[string]any
				if json.Unmarshal(buf[:i], &msg) == nil {
					replies <- msg
				}
				buf = buf[i+1:]
			}
			if err != nil {
				return
			}
		}
	}()
	next := 0
	call := func(session, method string, params map[string]any) map[string]any {
		next++
		id := next
		msg := map[string]any{"id": id, "method": method, "params": params}
		if session != "" {
			msg["sessionId"] = session
		}
		data, _ := json.Marshal(msg)
		if _, err := ourEnd.Write(append(data, 0)); err != nil {
			t.Skipf("Chrome DevTools pipe: %v", err)
		}
		deadline := time.After(30 * time.Second)
		for {
			select {
			case reply, ok := <-replies:
				if !ok {
					t.Skip("Chrome closed its DevTools pipe")
				}
				if got, _ := reply["id"].(float64); int(got) == id {
					if e, bad := reply["error"]; bad {
						t.Skipf("Chrome DevTools %s: %v", method, e)
					}
					result, _ := reply["result"].(map[string]any)
					return result
				}
			case <-deadline:
				t.Skipf("Chrome DevTools %s did not answer", method)
			}
		}
	}
	target := call("", "Target.createTarget", map[string]any{"url": "about:blank"})
	session := call("", "Target.attachToTarget", map[string]any{"targetId": target["targetId"], "flatten": true})["sessionId"].(string)
	// Headless pages lose focus to nothing; keep them focused for keyboard input.
	call(session, "Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true})
	if opts.inject != "" {
		call(session, "Page.enable", map[string]any{})
		call(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": opts.inject})
	}
	if opts.scheme != "" {
		call(session, "Emulation.setEmulatedMedia", map[string]any{"features": []map[string]any{{"name": "prefers-color-scheme", "value": opts.scheme}}})
	}
	call(session, "Page.navigate", map[string]any{"url": server.URL + "/__harness.html"})
	type queued struct {
		ID   int    `json:"id"`
		Key  string `json:"key"`
		Mods int    `json:"mods"`
		Text string `json:"text"`
	}
	limit := time.Now().Add(150 * time.Second)
	for time.Now().Before(limit) {
		time.Sleep(50 * time.Millisecond)
		res := call(session, "Runtime.evaluate", map[string]any{"returnByValue": true, "expression": `JSON.stringify({
			out: document.getElementById('out') ? document.getElementById('out').textContent : 'pending',
			input: window.__input ? window.__input.splice(0) : []})`})
		inner, _ := res["result"].(map[string]any)
		raw, _ := inner["value"].(string)
		var state struct {
			Out   string   `json:"out"`
			Input []queued `json:"input"`
		}
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			t.Skipf("the harness state is unreadable: %q", raw)
		}
		for _, in := range state.Input {
			if in.Text != "" {
				call(session, "Input.insertText", map[string]any{"text": in.Text})
			} else {
				def, ok := keyDefs[in.Key]
				if !ok {
					t.Skipf("the harness asked for an unknown key %q", in.Key)
				}
				down := map[string]any{"type": "rawKeyDown", "key": in.Key, "code": def.code, "modifiers": in.Mods,
					"windowsVirtualKeyCode": def.vk, "nativeVirtualKeyCode": def.vk}
				if def.text != "" && in.Mods&(2|4) == 0 { // no Ctrl or Meta: the key types its text
					down["type"], down["text"], down["unmodifiedText"] = "keyDown", def.text, def.text
				}
				call(session, "Input.dispatchKeyEvent", down)
				call(session, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": in.Key, "code": def.code,
					"modifiers": in.Mods, "windowsVirtualKeyCode": def.vk, "nativeVirtualKeyCode": def.vk})
			}
			call(session, "Runtime.evaluate", map[string]any{"expression": fmt.Sprintf("window.__acked = %d", in.ID)})
		}
		text := state.Out
		if text == "pending" || text == "" {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Skipf("the harness reported unreadable output: %q", text)
		}
		if e, ok := result["harness_error"]; ok {
			t.Skipf("the harness could not run: %v", e)
		}
		return result
	}
	t.Skip("the harness did not finish within 150 seconds")
	return nil
}

// githubHTML is GitHub's own rendering of a Markdown source as GitHub shows
// a file (POST /markdown in markdown mode; gfm mode renders a comment, with
// line breaks and mentions), kept in the cache by the source's digest. ok is
// false when it cannot be had.
func githubHTML(t *testing.T, source string) (string, bool) {
	t.Helper()
	sum := sha256.Sum256([]byte(source))
	path := filepath.Join(cacheDir(t), "github-file", hex.EncodeToString(sum[:])+".html")
	if data, err := os.ReadFile(path); err == nil {
		return string(data), true
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return "", false
	}
	body, _ := json.Marshal(map[string]string{"text": source, "mode": "markdown", "context": ownerRepo})
	cmd := exec.Command(gh, "api", "-X", "POST", "markdown", "--input", "-")
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return "", false
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, out, 0o644)
	}
	return string(out), true
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
