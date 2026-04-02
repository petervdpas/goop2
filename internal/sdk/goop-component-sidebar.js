(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-sidebar-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-sidebar-backdrop {
        display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0;
        background: rgba(0,0,0,.5); z-index: 9980; backdrop-filter: blur(4px);
      }
      .gc-sidebar-backdrop[data-goop-open] { display: block; }
      .gc-sidebar {
        position: fixed; top: 0; bottom: 0; z-index: 9981;
        width: 320px; max-width: 85vw; overflow-y: auto;
        background: var(--goop-panel, #151924); border: 1px solid var(--goop-border, #2a3142);
        box-shadow: 0 8px 32px rgba(0,0,0,.3); font: var(--goop-font, inherit);
        transform: translateX(-100%); transition: transform .25s ease;
      }
      .gc-sidebar[data-goop-side="right"] { right: 0; left: auto; transform: translateX(100%); }
      .gc-sidebar[data-goop-side="left"], .gc-sidebar:not([data-goop-side]) { left: 0; }
      .gc-sidebar[data-goop-open] { transform: translateX(0); }
      .gc-sidebar-header {
        display: flex; align-items: center; justify-content: space-between;
        padding: .75rem 1rem; border-bottom: 1px solid var(--goop-border, #2a3142);
      }
      .gc-sidebar-title { font-weight: 600; color: var(--goop-text, #e6e9ef); font-size: 1rem; }
      .gc-sidebar-close { background: none; border: none; color: var(--goop-muted, #9aa3b2); cursor: pointer; font-size: 1.2rem; padding: 0; line-height: 1; }
      .gc-sidebar-close:hover { color: var(--goop-text, #e6e9ef); }
      .gc-sidebar-body { padding: .75rem 1rem; color: var(--goop-text, #e6e9ef); }
    `;
    document.head.appendChild(s);
  }

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
