(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  var SID = "gc-badge-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-badge {
        display: inline-flex; align-items: center; gap: .25rem;
        padding: .15rem .5rem; border-radius: 999px; font-size: .75rem; font-weight: 500;
        background: color-mix(in srgb, var(--goop-accent, #7aa2ff) 18%, transparent);
        color: var(--goop-accent, #7aa2ff);
        border: 1px solid color-mix(in srgb, var(--goop-accent, #7aa2ff) 30%, transparent);
      }
      .gc-badge[data-goop-variant="success"] { --goop-accent: #22c55e; }
      .gc-badge[data-goop-variant="warning"] { --goop-accent: #eab308; }
      .gc-badge[data-goop-variant="danger"] { --goop-accent: var(--goop-danger, #f87171); }
      .gc-badge[data-goop-variant="muted"] { --goop-accent: var(--goop-muted, #9aa3b2); }
      .gc-badge-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; flex-shrink: 0; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.badge = function(text, opts) {
    opts = opts || {};
    var el = document.createElement("span");
    el.className = "gc-badge";
    el.textContent = text;
    if (opts.variant) el.setAttribute("data-goop-variant", opts.variant);
    if (opts.dot) {
      var d = document.createElement("span");
      d.className = "gc-badge-dot";
      el.insertBefore(d, el.firstChild);
    }
    return el;
  };
})();
