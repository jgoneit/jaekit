/* Home page: the ASCII matrix of the Jaekit symbol and the fictional agent
   session. Nothing here is stored; the session restarts on every visit. */
(function () {
  "use strict";
  var root = document.documentElement;
  var lang = root.lang === "ko" ? "ko" : "en";
  var reduce = !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);

  function css(name) { return getComputedStyle(root).getPropertyValue(name).trim(); }
  function rgb(hex) { hex = hex.replace("#", ""); var n = parseInt(hex, 16); return [(n >> 16) & 255, (n >> 8) & 255, n & 255]; }
  function onTheme(fn) { if (window.jaekit && window.jaekit.onTheme) window.jaekit.onTheme(fn); }

  /* ---------- ASCII matrix of the symbol ---------- */
  var RAMP = " .:-=+*#%@";
  function Matrix(canvas, img) {
    this.c = canvas; this.ctx = canvas.getContext("2d"); this.img = img;
    this.ripples = []; this.ptr = null; this.visible = true; this.running = false; this.t0 = performance.now();
    this.bbox = this.findBox();
    this.colors();
  }
  Matrix.prototype.findBox = function () {
    var W = this.img.naturalWidth, H = this.img.naturalHeight, c = document.createElement("canvas");
    c.width = W; c.height = H; var x = c.getContext("2d"); x.drawImage(this.img, 0, 0);
    var d = x.getImageData(0, 0, W, H).data, minX = W, minY = H, maxX = 0, maxY = 0;
    for (var y = 0; y < H; y++) for (var i = 0; i < W; i++) {
      var k = (y * W + i) * 4, ink = d[k + 3] / 255 * (1 - (d[k] + d[k + 1] + d[k + 2]) / 765);
      if (ink > 0.3) { if (i < minX) minX = i; if (i > maxX) maxX = i; if (y < minY) minY = y; if (y > maxY) maxY = y; }
    }
    if (maxX < minX) return { x: 0, y: 0, w: W, h: H };
    return { x: minX, y: minY, w: maxX - minX + 1, h: maxY - minY + 1 };
  };
  Matrix.prototype.colors = function () { this.fg = rgb(css("--fg")); this.dim = rgb(css("--fg-3")); this.acc = rgb(css("--accent")); };
  Matrix.prototype.layout = function () {
    var r = this.c.getBoundingClientRect(), dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.w = r.width; this.h = r.height; this.dpr = dpr;
    this.c.width = Math.max(1, Math.round(r.width * dpr)); this.c.height = Math.max(1, Math.round(r.height * dpr));
    this.s = r.width < 560 ? 7 : 9;
    this.cols = Math.max(1, Math.floor(this.w / this.s)); this.rows = Math.max(1, Math.floor(this.h / this.s));
    this.ox = (this.w - this.cols * this.s) / 2 + this.s / 2; this.oy = (this.h - this.rows * this.s) / 2 + this.s / 2;
    var SS = 3, off = document.createElement("canvas"); off.width = this.cols * SS; off.height = this.rows * SS;
    var o = off.getContext("2d"), bb = this.bbox, target = this.rows * SS * 0.8, k = target / Math.max(bb.w, bb.h);
    var dw = bb.w * k, dh = bb.h * k;
    o.drawImage(this.img, bb.x, bb.y, bb.w, bb.h, (off.width - dw) / 2, (off.height - dh) / 2, dw, dh);
    var d = o.getImageData(0, 0, off.width, off.height).data, n = this.cols * this.rows;
    this.cov = new Float32Array(n); this.ang = new Float32Array(n); this.rnd = new Float32Array(n);
    var cx = this.cols / 2, cy = this.rows / 2;
    for (var y = 0; y < this.rows; y++) for (var x = 0; x < this.cols; x++) {
      var sum = 0;
      for (var sy = 0; sy < SS; sy++) for (var sx = 0; sx < SS; sx++) {
        var q = ((y * SS + sy) * off.width + (x * SS + sx)) * 4;
        sum += d[q + 3] / 255 * (1 - (d[q] + d[q + 1] + d[q + 2]) / 765);
      }
      var i = y * this.cols + x;
      this.cov[i] = sum / (SS * SS);
      this.ang[i] = Math.atan2(y - cy, (x - cx) * 0.9);
      this.rnd[i] = Math.random();
    }
  };
  Matrix.prototype.draw = function (now) {
    var ctx = this.ctx, s = this.s, t = (now - this.t0) / 1000, cols = this.cols, rows = this.rows;
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, this.w, this.h);
    ctx.font = "500 " + Math.round(s * 1.25) + "px 'Geist Mono Variable', ui-monospace, monospace";
    ctx.textAlign = "center"; ctx.textBaseline = "middle";
    var ptr = this.ptr, R = Math.max(70, this.w * 0.09), sweep = t * 0.55, fg = this.fg, dim = this.dim, acc = this.acc;
    var rips = this.ripples.filter(function (r) { return now - r.t < 1700; });
    this.ripples = rips;
    var style = "", plain = "rgba(" + dim[0] + "," + dim[1] + "," + dim[2] + ",0.16)";
    function paint(v) { if (v !== style) { style = v; ctx.fillStyle = v; } }
    for (var y = 0; y < rows; y++) {
      var py = this.oy + y * s;
      for (var x = 0; x < cols; x++) {
        var i = y * cols + x, px = this.ox + x * s, c = this.cov[i], lens = 0, rip = 0;
        if (ptr) {
          var dx = px - ptr.x, dy = py - ptr.y, dd = dx * dx + dy * dy;
          if (dd < R * R) lens = 1 - Math.sqrt(dd) / R;
        }
        for (var j = 0; j < rips.length; j++) {
          var rr = rips[j], age = (now - rr.t) / 1000, rad = age * 460, life = 1 - age / 1.7;
          var dist = Math.hypot(px - rr.x, py - rr.y), band = Math.abs(dist - rad);
          if (band < 28) rip = Math.max(rip, (1 - band / 28) * life);
        }
        var hot = Math.max(lens, rip);
        if (c > 0.32) {
          var sw = reduce ? 0 : Math.pow(0.5 + 0.5 * Math.cos(this.ang[i] - sweep), 10);
          var fl = (!reduce && this.rnd[i] > 0.86) ? 0.18 * Math.sin(t * 2.4 + this.rnd[i] * 60) : 0;
          var b = Math.min(1, 0.28 + 0.42 * c + 0.38 * sw + 0.55 * hot + fl);
          var ch = RAMP[Math.max(1, Math.min(9, Math.round(b * 9)))];
          var m = Math.round(Math.min(1, hot * 1.2 + sw * 0.35) * 10) / 10;
          var a = Math.round((0.3 + 0.7 * b) * 20) / 20;
          paint("rgba(" + ((fg[0] + (acc[0] - fg[0]) * m) | 0) + "," + ((fg[1] + (acc[1] - fg[1]) * m) | 0) + "," + ((fg[2] + (acc[2] - fg[2]) * m) | 0) + "," + a + ")");
          ctx.fillText(ch, px, py + 0.5);
        } else if (hot > 0.02 || c > 0.08) {
          var a2 = Math.round((0.16 + 0.6 * hot + (c > 0.08 ? 0.12 : 0)) * 20) / 20, z = 1.1 + 1.8 * hot, hq = Math.round(hot * 10) / 10;
          paint("rgba(" + ((dim[0] + (acc[0] - dim[0]) * hq) | 0) + "," + ((dim[1] + (acc[1] - dim[1]) * hq) | 0) + "," + ((dim[2] + (acc[2] - dim[2]) * hq) | 0) + "," + a2 + ")");
          ctx.fillRect(px - z / 2, py - z / 2, z, z);
        } else {
          paint(plain);
          ctx.fillRect(px - 0.55, py - 0.55, 1.1, 1.1);
        }
      }
    }
  };
  // With reduced motion the matrix is a still frame: pointer and clicks redraw
  // nothing, and no animation loop runs.
  Matrix.prototype.kick = function () {
    if (reduce) { this.draw(this.t0); return; }
    if (this.running) return;
    this.running = true;
    var self = this;
    (function frame(now) {
      if (!self.visible || document.hidden) { self.running = false; return; }
      self.draw(now);
      requestAnimationFrame(frame);
    })(performance.now());
  };

  var canvas = document.getElementById("matrix");
  if (canvas && canvas.getContext) {
    var img = new Image();
    img.onload = function () {
      var matrix = new Matrix(canvas, img);
      matrix.layout();
      matrix.draw(performance.now());
      if (!reduce) matrix.ripples.push({ x: matrix.w / 2, y: matrix.h / 2, t: performance.now() + 250 });
      matrix.kick();
      onTheme(function () { matrix.colors(); matrix.draw(performance.now()); });
      var rt = null;
      window.addEventListener("resize", function () { clearTimeout(rt); rt = setTimeout(function () { matrix.layout(); matrix.draw(performance.now()); }, 120); });
      function local(e) { var r = canvas.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top }; }
      if (!reduce) {
        canvas.addEventListener("pointermove", function (e) { if (e.pointerType !== "touch") { matrix.ptr = local(e); matrix.kick(); } });
        canvas.addEventListener("pointerleave", function () { matrix.ptr = null; });
        canvas.addEventListener("click", function (e) { var p = local(e); matrix.ripples.push({ x: p.x, y: p.y, t: performance.now() }); matrix.kick(); });
      }
      if ("IntersectionObserver" in window) {
        new IntersectionObserver(function (es) { matrix.visible = es[0].isIntersecting; if (matrix.visible) matrix.kick(); }).observe(canvas);
      }
      document.addEventListener("visibilitychange", function () { if (!document.hidden) matrix.kick(); });
      if (document.fonts && document.fonts.ready) document.fonts.ready.then(function () { matrix.draw(performance.now()); });
    };
    img.src = canvas.getAttribute("data-symbol");
  }

  /* ---------- Fictional agent session ---------- */
  var termEl = document.getElementById("term");
  if (!termEl) return;
  var agentName = "claude";
  function agent() { return agentName; }
  var G = "docs/specs/password-link";
  var SESSION = [
    {
      input: {
        ko: function () { return (agent() === "codex" ? "$" : "/") + 'spec:spec 로그인 화면에 "비밀번호 찾기" 링크를 추가해줘'; },
        en: function () { return (agent() === "codex" ? "$" : "/") + 'spec:spec Add a "Forgot password" link to the login screen'; }
      },
      out: {
        ko: [["agent", "spec", G + "/SPEC.md를 썼습니다"], ["dim", "", "  AC-1  로그인 화면에 /reset-password로 가는 링크가 보인다"], ["dim", "", "  AC-2  기존 로그인 폼이 그대로 동작한다"], ["dim", "", "  Status: Ready · Assurance: standard"], ["agent", "spec", "열린 결정이 없습니다. 지금 구현할까요?"]],
        en: [["agent", "spec", "Wrote " + G + "/SPEC.md"], ["dim", "", "  AC-1  The login screen shows a link to /reset-password"], ["dim", "", "  AC-2  The existing login form keeps working"], ["dim", "", "  Status: Ready · Assurance: standard"], ["agent", "spec", "No decision is left open. Implement it now?"]]
      }
    },
    {
      input: { ko: function () { return "응"; }, en: function () { return "Yes"; } },
      out: {
        ko: [["agent", "seal", G + " 시작 · 물을 결정이 없어 바로 진행합니다"], ["cmd", "ha", "ha check " + G + " --baseline AC-1"], ["bad", "", "✕ AC-1 baseline fail_as_expected (seq 2)   # 바꾸기 전"], ["dim", "", "… 구현하고 commit …"], ["cmd", "ha", "ha check " + G + " AC-1 AC-2"], ["ok", "", "✓ AC-1 check pass (seq 3)   # 바꾼 뒤"], ["ok", "", "✓ AC-2 check pass (seq 4)   # 계속 통과"], ["cmd", "spar", "spar run --stage candidate --base f46020c …"], ["cmd", "ha", "ha note " + G + " review --gate …"], ["dim", "", "noted: review (seq 5)   # gate 기록"], ["cmd", "ha", "ha done " + G], ["ok", "", "- status: complete (exit 0)"], ["dim", "", "- completion record: seq 6"], ["agent", "seal", "완료 보고를 PROGRESS.md에 남겼습니다"]],
        en: [["agent", "seal", "starting " + G + " · nothing to ask, going ahead"], ["cmd", "ha", "ha check " + G + " --baseline AC-1"], ["bad", "", "✕ AC-1 baseline fail_as_expected (seq 2)   # before the change"], ["dim", "", "… implement and commit …"], ["cmd", "ha", "ha check " + G + " AC-1 AC-2"], ["ok", "", "✓ AC-1 check pass (seq 3)   # after the change"], ["ok", "", "✓ AC-2 check pass (seq 4)   # still passes"], ["cmd", "spar", "spar run --stage candidate --base f46020c …"], ["cmd", "ha", "ha note " + G + " review --gate …"], ["dim", "", "noted: review (seq 5)   # gate recorded"], ["cmd", "ha", "ha done " + G], ["ok", "", "- status: complete (exit 0)"], ["dim", "", "- completion record: seq 6"], ["agent", "seal", "The completion report is in PROGRESS.md"]]
      }
    },
    {
      input: { ko: function () { return "완료 보고 보여줘"; }, en: function () { return "Show me the completion report"; } },
      out: {
        ko: [["agent", "seal", "PROGRESS.md › ## 완료 보고"], ["rep", "", "바뀐 것: base f46020ce6f77 이후 파일 1개, commit 1개"], ["rep", "", "확인한 것: 필수 조건 2개 중 2개 충족 (검사 기록 2, 사용자 확인 0)"], ["rep", "", "직접 볼 것: 없음"], ["rep", "", "남은 한계: 보증 수준 local; 검사 작성자: 실행자"], ["rep", "", "다음 행동: 없음; 완료 기록 seq 6이 유효함"]],
        en: [["agent", "seal", "PROGRESS.md › ## 완료 보고"], ["rep", "", "Changed: 1 files and 1 commits since base f46020ce6f77"], ["rep", "", "Checked: 2 of 2 required conditions satisfied (2 by check records, 0 by user confirmation)"], ["rep", "", "To review: none"], ["rep", "", "Remaining limits: assurance local; check author: executor"], ["rep", "", "Next step: none; completion record seq 6 holds"]]
      }
    },
    {
      input: { ko: function () { return G + " 이어서 해줘"; }, en: function () { return "continue " + G; } },
      out: {
        ko: [["sys", "", "(다음 날, 같은 프로젝트의 새 대화)"], ["agent", "seal", "저장된 목표와 진행 기록을 읽었습니다"], ["agent", "seal", "완료 기록 seq 6이 유효하고 남은 할 일이 없습니다"]],
        en: [["sys", "", "(the next day, a new conversation in the same project)"], ["agent", "seal", "Read the saved goal and progress records"], ["agent", "seal", "Completion record seq 6 holds and nothing is left to do"]]
      }
    }
  ];
  var MSG = {
    ko: { you: "you", intro: "가상 세션입니다. 아래 버튼을 누르거나 → 키로 다음 입력을 채우고 Enter를 누르세요.", wrong: "이 데모는 정해진 가상 세션만 재생합니다. → 키로 다음 입력을 채워 보세요.", next: "다음 입력", again: "↻ 처음부터", done: "끝 ·" },
    en: { you: "you", intro: "A fictional session. Press the button below, or → to fill the next input, then Enter.", wrong: "This demo replays one fixed session. Press → to fill the next input.", next: "next", again: "↻ start over", done: "end ·" }
  };
  var out = document.getElementById("term-out"), form = document.getElementById("term-form"),
      input = document.getElementById("term-input"), nextBox = document.getElementById("term-next");
  var stepIndex = 0, busy = false, timers = [], fadeTimer = null;
  function line(kind, who, text) {
    var el = document.createElement("div"); el.className = "ln " + kind;
    var a = document.createElement("span"); a.className = "who"; a.textContent = who;
    var b = document.createElement("span"), cut = text.indexOf("   # ");
    if (cut > -1) { b.textContent = text.slice(0, cut); var n = document.createElement("span"); n.className = "note"; n.textContent = text.slice(cut); b.appendChild(n); }
    else b.textContent = text;
    el.appendChild(a); el.appendChild(b); out.appendChild(el); out.scrollTop = out.scrollHeight;
  }
  function gap() { var g = document.createElement("div"); g.className = "gap"; out.appendChild(g); }
  function suggestion() { var s = SESSION[stepIndex]; return s ? s.input[lang]() : ""; }
  function suggest() {
    nextBox.textContent = "";
    var l = MSG[lang];
    if (stepIndex < SESSION.length) {
      input.placeholder = suggestion();
      var lab = document.createElement("span"); lab.textContent = l.next + " " + (stepIndex + 1) + "/" + SESSION.length; nextBox.appendChild(lab);
      var b = document.createElement("button"); b.type = "button"; b.className = "next-input"; b.textContent = suggestion() + "  ⏎";
      b.addEventListener("click", function () { input.value = suggestion(); run(); });
      nextBox.appendChild(b);
    } else {
      input.placeholder = "";
      var d = document.createElement("span"); d.textContent = l.done; nextBox.appendChild(d);
      var r = document.createElement("button"); r.type = "button"; r.textContent = l.again; r.addEventListener("click", function () { reset(false); }); nextBox.appendChild(r);
    }
  }
  // Reduced motion shows each step's lines at once instead of one by one.
  function play(lines, i, after) {
    if (i >= lines.length) { after(); return; }
    line(lines[i][0], lines[i][1], lines[i][2]);
    if (reduce) { play(lines, i + 1, after); return; }
    timers.push(setTimeout(function () { play(lines, i + 1, after); }, lines[i][0] === "cmd" ? 380 : 180));
  }
  function run() {
    if (busy || stepIndex >= SESSION.length) return;
    var typed = input.value.trim(), want = suggestion();
    if (typed && typed !== want) { line("you", MSG[lang].you, typed); line("sys", "", MSG[lang].wrong); gap(); input.value = ""; return; }
    busy = true; input.value = ""; input.disabled = true; nextBox.textContent = "";
    line("you", MSG[lang].you, want);
    play(SESSION[stepIndex].out[lang], 0, function () { gap(); stepIndex++; busy = false; input.disabled = false; suggest(); });
  }
  function boot() {
    var box = document.createElement("div"); box.className = "boot";
    var title = document.createElement("b"); title.textContent = agent() === "codex" ? "codex " : "claude code ";
    var where = document.createElement("span"); where.textContent = "· ~/my-app"; title.appendChild(where);
    var plugins = document.createElement("span"); plugins.textContent = "spec@jaekit · seal@jaekit · spar@jaekit";
    box.appendChild(title); box.appendChild(plugins); out.appendChild(box);
  }
  function reset(animated) {
    timers.forEach(clearTimeout); timers = [];
    clearTimeout(fadeTimer);
    stepIndex = 0; busy = false; input.disabled = false; input.value = "";
    function fill() {
      out.textContent = "";
      boot(); line("sys", "", MSG[lang].intro); gap(); suggest();
      out.classList.remove("fade");
    }
    if (animated === true && !reduce) { out.classList.add("fade"); nextBox.textContent = ""; fadeTimer = setTimeout(fill, 190); }
    else fill();
  }
  // A session belongs to one agent: choosing the other one starts it over.
  function setAgent(a, animated) {
    var changed = a !== agentName;
    agentName = a;
    root.setAttribute("data-agent", a);
    termEl.setAttribute("data-agent", a);
    Array.prototype.forEach.call(document.querySelectorAll("[data-set-agent]"), function (b) {
      b.setAttribute("aria-pressed", String(b.getAttribute("data-set-agent") === a));
    });
    if (changed || animated === null) reset(animated);
  }
  Array.prototype.forEach.call(document.querySelectorAll("[data-set-agent]"), function (b) {
    b.addEventListener("click", function () { setAgent(b.getAttribute("data-set-agent"), true); });
  });
  form.addEventListener("submit", function (e) { e.preventDefault(); run(); });
  input.addEventListener("keydown", function (e) {
    if (e.key === "ArrowRight" && input.selectionStart === input.value.length && stepIndex < SESSION.length) {
      var want = suggestion();
      if (want.indexOf(input.value) === 0 && input.value !== want) { e.preventDefault(); input.value = want; }
    }
  });
  setAgent("claude", null);
})();
