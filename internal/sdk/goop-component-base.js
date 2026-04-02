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
