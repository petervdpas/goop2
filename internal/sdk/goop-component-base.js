//
// Shared foundation for goop component library.
// Provides CSS custom properties, theme detection, utility functions, and grid system.
// Load this before any individual goop-component-*.js files.
//
// Usage:
//   <script src="/sdk/goop-component-base.js"></script>
//   <script src="/sdk/goop-component-tabs.js"></script>
//
(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui._esc = function(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  };

  Goop.ui._fire = function(el, name, detail) {
    el.dispatchEvent(new CustomEvent(name, { bubbles: true, detail: detail }));
  };

  // ── Theme detection ──

  var LIGHT = {
    bg: "#ffffff", panel: "#ffffff", text: "#101325", muted: "#4a4f6b",
    line: "rgba(16,19,37,.12)", accent: "#5a3dff",
    field: "rgba(16,19,37,.04)", shadow: "0 22px 60px rgba(16,19,37,.14)"
  };
  var DARK = {
    bg: "#0f1115", panel: "#151924", text: "#e6e9ef", muted: "#9aa3b2",
    line: "rgba(255,255,255,.10)", accent: "#7aa2ff",
    field: "rgba(0,0,0,.25)", shadow: "0 18px 46px rgba(0,0,0,.34)"
  };

  function isDark() {
    var t = document.documentElement.getAttribute("data-theme") || "";
    if (t === "dark") return true;
    if (t === "light") return false;
    if (document.documentElement.classList.contains("theme-dark")) return true;
    if (document.documentElement.classList.contains("theme-light")) return false;
    var bg = getComputedStyle(document.documentElement).backgroundColor;
    if (bg) {
      var m = bg.match(/\d+/g);
      if (m && m.length >= 3) {
        var lum = (parseInt(m[0]) * 299 + parseInt(m[1]) * 587 + parseInt(m[2]) * 114) / 1000;
        return lum < 128;
      }
    }
    return false;
  }

  function cssVar(name) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || null;
  }

  Goop.ui._isDark = isDark;

  Goop.ui._resolveTheme = function() {
    var d = isDark() ? DARK : LIGHT;
    return {
      bg:     cssVar("--bg") || cssVar("--gui-bg") || d.bg,
      panel:  cssVar("--panel") || cssVar("--gui-panel") || d.panel,
      text:   cssVar("--text") || cssVar("--gui-text") || d.text,
      muted:  cssVar("--muted") || cssVar("--gui-muted") || d.muted,
      line:   cssVar("--line") || cssVar("--border") || cssVar("--gui-line") || d.line,
      accent: cssVar("--accent") || cssVar("--gui-accent") || d.accent,
      field:  cssVar("--field-bg") || cssVar("--gui-field") || d.field,
      shadow: d.shadow,
    };
  };

  Goop.ui.theme = function() {
    return document.documentElement.getAttribute("data-theme") || (isDark() ? "dark" : "light");
  };

  // ── Inline style helper (CSP-safe for WebKitGTK site pages) ──

  Goop.ui._setStyles = function(el, styles) {
    for (var k in styles) el.style[k] = styles[k];
  };

  // ── DOM builder ──
  //
  //   Goop.dom("div", { class: "card", onclick: fn },
  //     Goop.dom("h3", {}, title),
  //     Goop.dom("p", { class: "muted" }, description),
  //     condition && Goop.dom("button", { onclick: del }, "Delete")
  //   )
  //
  //   Strings → text nodes (auto-escaped). Nulls/false → skipped.
  //   Event handlers (onclick, onchange, ...) attached directly.
  //   "class" attr supported (maps to className).
  //   Arrays flattened. Numbers coerced to strings.
  //

  var d = Goop.dom = function(tag, attrs) {
    var el = document.createElement(tag);
    if (attrs) {
      for (var k in attrs) {
        if (attrs[k] == null || attrs[k] === false) continue;
        if (typeof attrs[k] === "function") {
          el.addEventListener(k.replace(/^on/, ""), attrs[k]);
        } else if (k === "class") {
          el.className = attrs[k];
        } else if (k === "data" && typeof attrs[k] === "object") {
          for (var dk in attrs[k]) el.setAttribute("data-" + dk, attrs[k][dk]);
        } else {
          el.setAttribute(k, attrs[k]);
        }
      }
    }
    for (var i = 2; i < arguments.length; i++) {
      appendChild(el, arguments[i]);
    }
    return el;
  };

  function appendChildren(el, children) {
    for (var i = 0; i < children.length; i++) appendChild(el, children[i]);
  }

  function appendChild(el, child) {
    if (child == null || child === false || child === true) return;
    if (Array.isArray(child)) { appendChildren(el, child); return; }
    if (child instanceof Node) { el.appendChild(child); return; }
    el.appendChild(document.createTextNode(String(child)));
  }

  // ── Goop.list — DOM-native list renderer ──
  //
  //   Goop.list(container, rows, function(row, idx) {
  //     return Goop.dom("div", { class: "card", data: { id: row._id } },
  //       Goop.dom("h3", {}, row.title)
  //     );
  //   }, { empty: "Nothing here yet." });
  //

  var _oldList = Goop.list;
  Goop.list = function(el, rows, renderFn, opts) {
    opts = opts || {};
    if (!rows || rows.length === 0) {
      el.innerHTML = "";
      if (opts.empty) {
        if (typeof opts.empty === "string") {
          el.appendChild(d("div", { class: "gc-empty" }, opts.empty));
        } else if (opts.empty instanceof Node) {
          el.appendChild(opts.empty);
        } else {
          el.innerHTML = '<div class="gc-empty">' + opts.empty + '</div>';
        }
      }
      return;
    }
    var first = renderFn(rows[0], 0);
    if (typeof first === "string") {
      if (_oldList) return _oldList(el, rows, renderFn, opts);
      el.innerHTML = rows.map(renderFn).join("");
      return;
    }
    el.innerHTML = "";
    if (first) el.appendChild(first);
    for (var i = 1; i < rows.length; i++) {
      var node = renderFn(rows[i], i);
      if (node) el.appendChild(node);
    }
  };

  // ── Goop.render — replace container contents ──

  Goop.render = function(el) {
    el.innerHTML = "";
    for (var i = 1; i < arguments.length; i++) {
      appendChild(el, arguments[i]);
    }
  };

  // ── Built-in micro-components ──

  Goop.ui.avatar = function(peerId, opts) {
    opts = opts || {};
    var size = opts.size || 24;
    var src = "/api/avatar/peer/" + encodeURIComponent(peerId);
    return d("img", {
      class: "gc-avatar " + (opts.class || ""),
      src: src,
      alt: opts.alt || "",
      width: size,
      height: size,
    });
  };

  Goop.ui.empty = function(message, opts) {
    opts = opts || {};
    return d("div", { class: "gc-empty" },
      opts.icon ? d("div", { class: "gc-empty-icon" }, opts.icon) : null,
      d("p", {}, message)
    );
  };

  Goop.ui.loading = function(message) {
    return d("div", { class: "gc-loading" },
      d("div", { class: "gc-spinner" }),
      message ? d("p", {}, message) : null
    );
  };

  Goop.ui.time = function(ts) {
    if (!ts) return "";
    var then = new Date(String(ts).replace(" ", "T") + "Z");
    var diff = Math.floor((Date.now() - then) / 1000);
    if (diff < 60) return "just now";
    if (diff < 3600) return Math.floor(diff / 60) + "m ago";
    if (diff < 86400) return Math.floor(diff / 3600) + "h ago";
    return Math.floor(diff / 86400) + "d ago";
  };

  // ── CSS custom properties + grid ──

  var STYLE_ID = "gc-base-style";
  if (!document.getElementById(STYLE_ID)) {
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      :root {
        --goop-bg: var(--bg, #0f1115);
        --goop-panel: var(--panel, #151924);
        --goop-text: var(--text, #e6e9ef);
        --goop-muted: var(--muted, #9aa3b2);
        --goop-border: var(--border, #2a3142);
        --goop-accent: var(--accent, #7aa2ff);
        --goop-danger: var(--danger, #f87171);
        --goop-field: var(--field-bg, rgba(0,0,0,.25));
        --goop-radius: 6px;
        --goop-font: inherit;
      }
      [data-theme="light"], .theme-light {
        --goop-bg: var(--bg, #ffffff);
        --goop-panel: var(--panel, #ffffff);
        --goop-text: var(--text, #101325);
        --goop-muted: var(--muted, #4a4f6b);
        --goop-border: var(--border, rgba(16,19,37,.12));
        --goop-accent: var(--accent, #5a3dff);
        --goop-danger: var(--danger, #dc2626);
        --goop-field: var(--field-bg, rgba(16,19,37,.04));
      }
      [data-goop-disabled] { opacity: 0.5; pointer-events: none; }

      .gc-avatar { border-radius: 50%; vertical-align: middle; object-fit: cover; }
      .gc-empty { text-align: center; padding: 2rem 1rem; color: var(--goop-muted, #9aa3b2); }
      .gc-empty-icon { font-size: 2rem; margin-bottom: .5rem; }
      .gc-loading { display: flex; flex-direction: column; align-items: center; gap: .5rem; padding: 2rem; color: var(--goop-muted, #9aa3b2); }
      .gc-spinner {
        width: 1.5rem; height: 1.5rem; border: 2px solid var(--goop-border, #2a3142);
        border-top-color: var(--goop-accent, #7aa2ff); border-radius: 50%;
        animation: gc-spin .6s linear infinite;
      }
      @keyframes gc-spin { to { transform: rotate(360deg); } }

      .gc-grid {
        display: grid; gap: 1rem;
        grid-template-columns: repeat(12, 1fr);
      }
      .gc-grid[data-goop-gap="none"] { gap: 0; }
      .gc-grid[data-goop-gap="sm"] { gap: .5rem; }
      .gc-grid[data-goop-gap="lg"] { gap: 1.5rem; }
      .gc-grid[data-goop-gap="xl"] { gap: 2rem; }
      .gc-col-1 { grid-column: span 1; } .gc-col-2 { grid-column: span 2; }
      .gc-col-3 { grid-column: span 3; } .gc-col-4 { grid-column: span 4; }
      .gc-col-5 { grid-column: span 5; } .gc-col-6 { grid-column: span 6; }
      .gc-col-7 { grid-column: span 7; } .gc-col-8 { grid-column: span 8; }
      .gc-col-9 { grid-column: span 9; } .gc-col-10 { grid-column: span 10; }
      .gc-col-11 { grid-column: span 11; } .gc-col-12 { grid-column: span 12; }
      @media (max-width: 768px) {
        .gc-sm-1 { grid-column: span 1; } .gc-sm-2 { grid-column: span 2; }
        .gc-sm-3 { grid-column: span 3; } .gc-sm-4 { grid-column: span 4; }
        .gc-sm-5 { grid-column: span 5; } .gc-sm-6 { grid-column: span 6; }
        .gc-sm-7 { grid-column: span 7; } .gc-sm-8 { grid-column: span 8; }
        .gc-sm-9 { grid-column: span 9; } .gc-sm-10 { grid-column: span 10; }
        .gc-sm-11 { grid-column: span 11; } .gc-sm-12 { grid-column: span 12; }
      }
      @media (min-width: 1024px) {
        .gc-lg-1 { grid-column: span 1; } .gc-lg-2 { grid-column: span 2; }
        .gc-lg-3 { grid-column: span 3; } .gc-lg-4 { grid-column: span 4; }
        .gc-lg-5 { grid-column: span 5; } .gc-lg-6 { grid-column: span 6; }
        .gc-lg-7 { grid-column: span 7; } .gc-lg-8 { grid-column: span 8; }
        .gc-lg-9 { grid-column: span 9; } .gc-lg-10 { grid-column: span 10; }
        .gc-lg-11 { grid-column: span 11; } .gc-lg-12 { grid-column: span 12; }
      }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.grid = function(el, opts) {
    opts = opts || {};
    var wrap = document.createElement("div");
    wrap.className = "gc-grid";
    wrap.setAttribute("data-goop-component", "grid");
    if (opts.gap) wrap.setAttribute("data-goop-gap", opts.gap);
    el.appendChild(wrap);

    function col(span, opts2) {
      opts2 = opts2 || {};
      var c = document.createElement("div");
      c.className = "gc-col-" + (span || 12);
      if (opts2.sm) c.classList.add("gc-sm-" + opts2.sm);
      if (opts2.lg) c.classList.add("gc-lg-" + opts2.lg);
      if (opts2.content) c.innerHTML = opts2.content;
      wrap.appendChild(c);
      return c;
    }

    return {
      col: col,
      clear: function() { wrap.innerHTML = ""; },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
