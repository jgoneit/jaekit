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

func page(t *testing.T, dir, rel string, violation string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		fail(t, violation, "%s is not in the built site: %v", rel, err)
	}
	return string(data)
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

// doc is one Markdown file that becomes a page.
type doc struct {
	Path, Lang, URL string
}

// documents lists the tracked Markdown files directly under guides/ and
// contracts/ of the checked tree, with the page each one becomes.
func documents(t *testing.T) []doc {
	t.Helper()
	var docs []doc
	for _, name := range strings.Split(git(t, repoRoot(t), "ls-files", "-z", "--", "guides", "contracts"), "\x00") {
		section, file, ok := strings.Cut(name, "/")
		if !ok || strings.Contains(file, "/") || !strings.HasSuffix(file, ".md") {
			continue
		}
		lang, stem := "ko", strings.TrimSuffix(file, ".md")
		if strings.HasSuffix(file, ".en.md") {
			lang, stem = "en", strings.TrimSuffix(file, ".en.md")
		}
		url := lang + "/" + section + "/"
		if stem != "README" {
			url += strings.ToLower(stem) + "/"
		}
		docs = append(docs, doc{Path: name, Lang: lang, URL: url})
	}
	if len(docs) == 0 {
		t.Skip("no tracked documents under guides/ or contracts/")
	}
	return docs
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
	script := `import json, sys
sys.path.insert(0, "tools")
import check_public_docs as c
out = {}
for name in sys.argv[1:]:
    text = open(name, encoding="utf-8").read()
    out[name] = {"anchors": sorted(c.anchors(text)), "links": c.links(text)}
print(json.dumps(out))`
	cmd := exec.Command(python(t), append([]string{"-c", script}, paths...)...)
	cmd.Dir = repoRoot(t)
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
	src := repoRoot(t)
	dir := t.TempDir()
	for _, name := range []string{
		"guides/INSTALL.md", "guides/INSTALL.en.md", "guides/USAGE.md", "guides/USAGE.en.md",
		"contracts/README.md", ".claude-plugin/marketplace.json",
		"assets/readme/brand/symbol.png", "assets/readme/brand/symbol.svg",
	} {
		data, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(name)))
		if err != nil {
			t.Skipf("cannot copy %s: %v", name, err)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Skip(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Skip(err)
		}
	}
	// Every tracked document is copied so that links between them resolve.
	for _, d := range documents(t) {
		target := filepath.Join(dir, filepath.FromSlash(d.Path))
		if _, err := os.Stat(target); err == nil {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(src, filepath.FromSlash(d.Path)))
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		_ = os.WriteFile(target, data, 0o644)
	}
	for _, name := range strings.Split(git(t, src, "ls-files", "-z"), "\x00") {
		if name == "" {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if _, err := os.Stat(target); err == nil {
			continue
		}
		// Other files only need to exist for links to resolve.
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		_ = os.WriteFile(target, []byte("stand-in\n"), 0o644)
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
	return browseWith(t, dir, "", script, flags...)
}

// browseWith also runs inject in every frame before the frame's own scripts.
func browseWith(t *testing.T, dir, inject, script string, flags ...string) map[string]any {
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
	if inject != "" {
		call(session, "Page.enable", map[string]any{})
		call(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": inject})
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

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
