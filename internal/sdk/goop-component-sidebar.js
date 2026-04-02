(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.sidebar = function(opts) {
    opts = opts || {};
    var side = opts.side || "right";
    var showOverlay = opts.overlay !== false;
    var closeOnEscape = opts.closeOnEscape !== false;

    var backdrop = document.createElement("div");
    backdrop.className = "gc-sidebar-backdrop";
    if (!showOverlay) backdrop.style.background = "transparent";

    var panel = document.createElement("div");
    panel.className = "gc-sidebar";
    panel.setAttribute("data-goop-component", "sidebar");
    panel.setAttribute("data-goop-side", side);
    if (opts.width) panel.style.width = typeof opts.width === "number" ? opts.width + "px" : opts.width;

    var header = document.createElement("div");
    header.className = "gc-sidebar-header";
    header.innerHTML = '<span class="gc-sidebar-title">' + _e(opts.title || "") + '</span><button type="button" class="gc-sidebar-close">\u00D7</button>';

    var body = document.createElement("div");
    body.className = "gc-sidebar-body";
    if (opts.content) body.innerHTML = opts.content;

    panel.appendChild(header); panel.appendChild(body);
    document.body.appendChild(backdrop); document.body.appendChild(panel);

    function open() { backdrop.setAttribute("data-goop-open", ""); panel.setAttribute("data-goop-open", ""); _f(panel, "change", { open: true }); if (opts.onOpen) opts.onOpen(); }
    function close() { backdrop.removeAttribute("data-goop-open"); panel.removeAttribute("data-goop-open"); _f(panel, "change", { open: false }); if (opts.onClose) opts.onClose(); }

    backdrop.addEventListener("click", close);
    header.querySelector(".gc-sidebar-close").addEventListener("click", close);
    function onKey(e) { if (closeOnEscape && e.key === "Escape" && panel.hasAttribute("data-goop-open")) close(); }
    document.addEventListener("keydown", onKey);

    return {
      open: open,
      close: close,
      body: body,
      el: panel,
      destroy: function() { document.removeEventListener("keydown", onKey); backdrop.remove(); panel.remove(); },
    };
  };
})();
