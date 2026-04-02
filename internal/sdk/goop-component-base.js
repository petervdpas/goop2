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

  Goop.ui._isDark = isDark;

  Goop.ui.theme = function() {
    return document.documentElement.getAttribute("data-theme") || (isDark() ? "dark" : "light");
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
      class: opts.class || "",
      src: src,
      alt: opts.alt || "",
      width: size,
      height: size,
    });
  };

  Goop.ui.empty = function(message, opts) {
    opts = opts || {};
    return d("div", { class: opts.class || "" },
      opts.icon ? d("div", { class: opts.iconClass || "" }, opts.icon) : null,
      d("p", {}, message)
    );
  };

  Goop.ui.loading = function(message, opts) {
    opts = opts || {};
    return d("div", { class: opts.class || "" },
      d("div", { class: opts.spinnerClass || "" }),
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

  // ── Grid helper ──

  Goop.ui.grid = function(el, opts) {
    opts = opts || {};

    function col(colOpts) {
      colOpts = colOpts || {};
      var c = document.createElement("div");
      if (colOpts.class) c.className = colOpts.class;
      el.appendChild(c);
      return c;
    }

    return {
      col: col,
      clear: function() { el.innerHTML = ""; },
      el: el,
    };
  };
})();
