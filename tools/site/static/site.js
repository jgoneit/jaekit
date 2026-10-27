/* Shared page behavior: theme, remembered language, copy buttons, the docs
   navigation and table of contents, the search palette and mermaid diagrams.
   Browser storage keeps only the two choices below; nothing else is stored. */
(function () {
  "use strict";
  var root = document.documentElement;
  var THEME_KEY = "jaekit-theme", LANG_KEY = "jaekit-lang";
  var lang = root.lang === "ko" ? "ko" : "en";

  function store(key, value) { try { localStorage.setItem(key, value); } catch (e) {} }
  function $(id) { return document.getElementById(id); }

  /* ---------- Theme ---------- */
  var themeHooks = [];
  window.jaekit = { onTheme: function (fn) { themeHooks.push(fn); } };
  function currentTheme() {
    var set = root.getAttribute("data-theme");
    if (set === "light" || set === "dark") return set;
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  window.jaekit.theme = currentTheme;
  function themeChanged() { themeHooks.forEach(function (fn) { try { fn(currentTheme()); } catch (e) {} }); }
  var themeBtn = $("theme-btn");
  if (themeBtn) themeBtn.addEventListener("click", function () {
    var next = currentTheme() === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    store(THEME_KEY, next);
    themeChanged();
  });
  if (window.matchMedia) {
    var mq = window.matchMedia("(prefers-color-scheme: light)");
    if (mq.addEventListener) mq.addEventListener("change", themeChanged);
  }

  /* ---------- Language: a followed switch is the visitor's choice ---------- */
  document.addEventListener("click", function (e) {
    var link = e.target.closest && e.target.closest("[data-set-lang]");
    if (link) store(LANG_KEY, link.getAttribute("data-set-lang"));
  });

  /* ---------- Copy ---------- */
  function copyText(text, btn) {
    function done() {
      if (!btn) return;
      var old = btn.getAttribute("data-label") || btn.textContent;
      btn.setAttribute("data-label", old);
      btn.classList.add("done"); btn.textContent = lang === "ko" ? "복사됨" : "copied";
      setTimeout(function () { btn.classList.remove("done"); btn.textContent = old; }, 1400);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, function () { fallback(text); done(); });
    } else { fallback(text); done(); }
  }
  function fallback(text) {
    var area = document.createElement("textarea");
    area.value = text; area.setAttribute("readonly", ""); area.style.position = "fixed"; area.style.opacity = "0";
    document.body.appendChild(area); area.select();
    try { document.execCommand("copy"); } catch (e) {}
    area.remove();
  }
  document.addEventListener("click", function (e) {
    var btn = e.target.closest && e.target.closest(".copy[data-copy], [data-copy-code]");
    if (!btn) return;
    if (btn.hasAttribute("data-copy")) { copyText(btn.getAttribute("data-copy"), btn); return; }
    var pre = btn.closest(".codeblock").querySelector("pre");
    copyText(pre.textContent, btn);
  });
  var copyPage = $("copy-page"), source = $("page-source");
  if (copyPage && source) copyPage.addEventListener("click", function () {
    // The page's original Markdown file, exactly as it is in the repository.
    copyText(JSON.parse(source.textContent), copyPage);
  });

  /* ---------- Docs navigation ---------- */
  var menuBtn = $("menu-btn"), side = $("side");
  function setNav(open) {
    var was = root.classList.contains("nav-open");
    root.classList.toggle("nav-open", open);
    if (menuBtn) menuBtn.setAttribute("aria-expanded", String(open));
    // Keyboard users land in the opened navigation and return to its button.
    if (open && !was && side) {
      var first = side.querySelector('a[aria-current="page"]') || side.querySelector("a");
      if (first) first.focus();
    } else if (!open && was && menuBtn && side && side.contains(document.activeElement)) {
      menuBtn.focus();
    }
  }
  if (menuBtn) menuBtn.addEventListener("click", function () { setNav(!root.classList.contains("nav-open")); });
  if (side) side.addEventListener("click", function (e) { if (e.target.closest("a")) setNav(false); });
  document.addEventListener("click", function (e) {
    if (root.classList.contains("nav-open") && side && !side.contains(e.target) && !(menuBtn && menuBtn.contains(e.target))) setNav(false);
  });

  /* ---------- Table of contents: mark the section being read ----------
     The current section is the last listed heading at or above the reading
     line, in whichever direction the page scrolls; at the end of the page it
     is the last heading in view. */
  var tocLinks = Array.prototype.slice.call(document.querySelectorAll("#toc-list a"));
  var tocHeads = tocLinks.map(function (a) { return document.getElementById(decodeURIComponent(a.getAttribute("href").slice(1))); });
  var spyQueued = false;
  function spy() {
    spyQueued = false;
    var line = window.innerHeight * 0.25, end = window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 2;
    var current = -1;
    tocHeads.forEach(function (h, i) {
      if (!h) return;
      var top = h.getBoundingClientRect().top;
      if (top <= line || (end && top < window.innerHeight)) current = i;
    });
    tocLinks.forEach(function (a, i) { a.classList.toggle("on", i === current); });
  }
  if (tocLinks.length) {
    window.addEventListener("scroll", function () { if (!spyQueued) { spyQueued = true; requestAnimationFrame(spy); } }, { passive: true });
    window.addEventListener("resize", spy);
    spy();
  }

  /* ---------- Search palette ---------- */
  var veil = $("palette"), input = $("palette-input"), list = $("palette-list");
  var entries = null, shown = [], cur = 0, lastFocus = null, loading = false;
  function load() {
    if (entries || loading || !veil) return;
    loading = true;
    fetch(veil.getAttribute("data-index")).then(function (r) {
      if (!r.ok) throw new Error(String(r.status));
      return r.json();
    }).then(function (data) {
      entries = data.map(function (e) {
        return { title: e[0], label: e[1], url: e[2], t: e[0].toLowerCase(), hay: (e[0] + " " + e[1]).toLowerCase(), body: (e[3] || "").toLowerCase() };
      });
      loading = false; render();
    }, function () { entries = []; loading = false; render(); });
  }
  function matches(q) {
    if (!entries) return [];
    if (!q) return entries.filter(function (e) { return e.url.indexOf("#") < 0; });
    var lower = q.toLowerCase(), words = lower.split(/\s+/).filter(Boolean), out = [];
    entries.forEach(function (e) {
      var named = words.every(function (w) { return e.hay.indexOf(w) > -1; });
      if (named || words.every(function (w) { return e.hay.indexOf(w) > -1 || e.body.indexOf(w) > -1; })) {
        // An exact title first, then titles that start with or contain the
        // query, other title matches, and last the parts whose text matches.
        var rank = e.t === lower ? 0 : e.t.indexOf(lower) === 0 ? 1 : e.t.indexOf(lower) > -1 ? 2 : named ? 3 : 4;
        out.push([rank, out.length, e]);
      }
    });
    out.sort(function (a, b) { return a[0] - b[0] || a[1] - b[1]; });
    return out.slice(0, 60).map(function (x) { return x[2]; });
  }
  function render() {
    if (!list) return;
    shown = matches(input.value.trim());
    if (cur >= shown.length) cur = 0;
    list.textContent = "";
    if (!shown.length) {
      var li0 = document.createElement("li"); li0.className = "empty";
      li0.textContent = entries ? veil.getAttribute("data-empty") : "…";
      list.appendChild(li0);
      return;
    }
    shown.forEach(function (e, i) {
      var li = document.createElement("li"), b = document.createElement("button");
      b.type = "button"; b.setAttribute("role", "option"); b.className = i === cur ? "on" : "";
      b.setAttribute("aria-selected", String(i === cur));
      b.setAttribute("data-href", e.url);
      var t = document.createElement("span"); t.textContent = e.title;
      var s = document.createElement("small"); s.textContent = e.label;
      b.appendChild(t); b.appendChild(s);
      b.addEventListener("click", function () { go(e); });
      b.addEventListener("mousemove", function () { if (cur !== i) { cur = i; paint(); } });
      li.appendChild(b); list.appendChild(li);
    });
  }
  function paint() {
    Array.prototype.forEach.call(list.querySelectorAll("button"), function (b, i) {
      b.className = i === cur ? "on" : ""; b.setAttribute("aria-selected", String(i === cur));
      if (i === cur && b.scrollIntoView) b.scrollIntoView({ block: "nearest" });
    });
  }
  function go(e) {
    close();
    location.href = veil.getAttribute("data-root") + e.url;
  }
  function open() {
    if (!veil) return;
    lastFocus = document.activeElement; veil.hidden = false; input.value = ""; cur = 0;
    load(); render(); input.focus();
  }
  function close() {
    if (!veil || veil.hidden) return;
    veil.hidden = true;
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  }
  var searchBtn = $("search-btn");
  if (searchBtn) searchBtn.addEventListener("click", open);
  if (veil) {
    veil.addEventListener("click", function (e) { if (e.target === veil) close(); });
    input.addEventListener("input", function () { cur = 0; render(); });
    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown") { e.preventDefault(); cur = Math.min(shown.length - 1, cur + 1); paint(); }
      else if (e.key === "ArrowUp") { e.preventDefault(); cur = Math.max(0, cur - 1); paint(); }
      else if (e.key === "Enter") { e.preventDefault(); if (shown[cur]) go(shown[cur]); }
      else if (e.key === "Escape") { e.preventDefault(); close(); }
      else if (e.key === "Tab") { e.preventDefault(); }
    });
  }
  document.addEventListener("keydown", function (e) {
    var tag = (e.target && e.target.tagName) || "";
    var typing = /^(input|textarea|select)$/i.test(tag) || (e.target && e.target.isContentEditable);
    if ((e.key === "k" || e.key === "K") && (e.metaKey || e.ctrlKey)) { e.preventDefault(); if (veil && veil.hidden) open(); else close(); }
    else if (e.key === "/" && !typing && veil && veil.hidden) { e.preventDefault(); open(); }
    else if (e.key === "Escape") { close(); setNav(false); }
  });

  /* ---------- Code colors: spans around the same text, so copy is unchanged ---------- */
  // Each rule is [class, sticky pattern, test]; the first that matches at a
  // position wins. A test is given the whole text and the match's position.
  function wordStart(text, i) { return i === 0 || /[\s(|;&]/.test(text[i - 1]); }
  function lineStart(text, i) {
    var j = i - 1;
    while (j >= 0 && (text[j] === " " || text[j] === "\t")) j--;
    return j < 0 || text[j] === "\n";
  }
  var SHELL = [
    ["com", /#[^\n]*/y, wordStart],
    ["str", /"(?:\\.|[^"\\\n])*"|'[^'\n]*'/y],
    ["var", /\$\{[^}\n]*\}|\$env:[A-Za-z_]\w*|\$[A-Za-z_]\w*/y],
    ["ph", /<[^<>\n]+>/y, wordStart],
    ["flag", /--?[A-Za-z][\w-]*/y, wordStart],
    ["word", /[^\s"'$<>|;&()]+/y]
  ];
  var RULES = {
    bash: SHELL, sh: SHELL, shell: SHELL, powershell: SHELL,
    json: [
      ["key", /"(?:\\.|[^"\\\n])*"(?=\s*:)/y],
      ["str", /"(?:\\.|[^"\\\n])*"/y],
      ["num", /-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/y],
      ["kw", /\b(?:true|false|null)\b/y]
    ],
    toml: [
      ["com", /#[^\n]*/y],
      ["head", /\[\[?[^\]\n]+\]\]?/y, lineStart],
      ["key", /[A-Za-z0-9_.-]+(?=[ \t]*=)/y, lineStart],
      ["str", /"""[\s\S]*?"""|"(?:\\.|[^"\\\n])*"|'[^'\n]*'/y],
      ["kw", /\b(?:true|false)\b/y],
      ["num", /[+-]?\d[\d_]*(?:\.\d+)?/y]
    ],
    markdown: [
      ["head", /#{1,6} [^\n]*/y, lineStart],
      ["punc", /(?:[-*+]|\d+\.) (?=\S)/y, lineStart],
      ["str", /`[^`\n]+`/y],
      ["kw", /\*\*[^*\n]+\*\*/y]
    ]
  };
  function esc(s) { return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
  function paintCode(code, rules) {
    var text = code.textContent, out = "", plain = "", i = 0, command = true;
    function flush() { if (plain) { out += esc(plain); plain = ""; } }
    while (i < text.length) {
      var hit = null;
      for (var r = 0; r < rules.length && !hit; r++) {
        var re = rules[r][1]; re.lastIndex = i;
        var m = re.exec(text);
        if (m && m[0] && (!rules[r][2] || rules[r][2](text, i))) hit = [rules[r][0], m[0]];
      }
      if (!hit) {
        var ch = text[i];
        // A shell command starts a line, unless the line before ends in a
        // continuation, and follows a pipe, a separator or an opening paren.
        if (ch === "\n" ? text[i - 1] !== "\\" : "|;&(".indexOf(ch) >= 0) command = true;
        plain += ch; i++; continue;
      }
      var cls = hit[0];
      if (cls === "word") {
        // NAME=value before a command keeps the command position.
        if (command && /^[A-Za-z_]\w*=/.test(hit[1])) cls = "var";
        else { cls = command ? "cmd" : ""; command = false; }
      } else if (cls === "com" || cls === "str" || cls === "flag" || cls === "ph") {
        command = false;
      }
      if (cls) { flush(); out += '<span class="tok-' + cls + '">' + esc(hit[1]) + "</span>"; }
      else plain += hit[1];
      i += hit[1].length;
    }
    flush();
    code.innerHTML = out;
  }
  Array.prototype.forEach.call(document.querySelectorAll('.markdown pre > code[class^="language-"]'), function (code) {
    var rules = RULES[code.className.slice("language-".length)];
    if (rules) paintCode(code, rules);
  });

  /* ---------- Mermaid diagrams, drawn in the page's theme ---------- */
  var diagrams = Array.prototype.slice.call(document.querySelectorAll("pre.mermaid"));
  function draw() {
    if (!diagrams.length || !window.mermaid) return;
    diagrams.forEach(function (pre) { pre.removeAttribute("data-processed"); pre.textContent = pre.getAttribute("data-source"); });
    var dark = currentTheme() === "dark";
    // The dark theme's grey edge-label box is too light for its text; use a darker one.
    window.mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: dark ? "dark" : "default",
                                themeVariables: dark ? { edgeLabelBackground: "#2A2A2E" } : {}, fontFamily: "inherit" });
    window.mermaid.run({ nodes: diagrams }).catch(function () {});
  }
  if (diagrams.length) {
    window.addEventListener("load", draw);
    themeHooks.push(draw);
  }
})();
