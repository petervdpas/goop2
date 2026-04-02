(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-toolbar-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-toolbar { display: flex; flex-wrap: wrap; gap: .25rem; font: var(--goop-font, inherit); }
      .gc-toolbar button {
        padding: .35rem .7rem; border: 1px solid var(--goop-border, #2a3142); border-radius: var(--goop-radius, 6px);
        background: var(--goop-field, rgba(0,0,0,.25)); color: var(--goop-muted, #9aa3b2); cursor: pointer; font: inherit; font-size: .85rem;
        transition: background .15s, color .15s, border-color .15s;
      }
      .gc-toolbar button:hover:not([disabled]) { color: var(--goop-text, #e6e9ef); border-color: var(--goop-accent, #7aa2ff); }
      .gc-toolbar button[data-goop-active] {
        background: color-mix(in srgb, var(--goop-accent, #7aa2ff) 18%, transparent);
        color: var(--goop-accent, #7aa2ff); border-color: var(--goop-accent, #7aa2ff);
      }
      .gc-toolbar button:disabled { opacity: .4; cursor: not-allowed; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.toolbar = function(el, opts) {
    opts = opts || {};
    var buttons = opts.buttons || [];
    var activeId = opts.active || (buttons[0] && buttons[0].id) || "";
    var multi = !!opts.multi;
    var activeIds = {};
    if (multi && opts.active) (Array.isArray(opts.active) ? opts.active : [opts.active]).forEach(function(id) { activeIds[id] = true; });

    var wrap = document.createElement("div");
    wrap.className = "gc-toolbar";
    wrap.setAttribute("data-goop-component", "toolbar");
    el.appendChild(wrap);

    function render() {
      wrap.innerHTML = "";
      buttons.forEach(function(b) {
        var btn = document.createElement("button"); btn.type = "button";
        btn.textContent = b.label || b.id;
        if (b.title) btn.title = b.title;
        if (b.disabled) btn.disabled = true;
        btn.setAttribute("data-goop-id", b.id);
        if (multi) { if (activeIds[b.id]) btn.setAttribute("data-goop-active", ""); }
        else { if (b.id === activeId) btn.setAttribute("data-goop-active", ""); }
        btn.addEventListener("click", function() {
          if (b.disabled) return;
          if (multi) {
            if (activeIds[b.id]) { delete activeIds[b.id]; btn.removeAttribute("data-goop-active"); }
            else { activeIds[b.id] = true; btn.setAttribute("data-goop-active", ""); }
            _f(wrap, "change", { value: Object.keys(activeIds) });
            if (opts.onChange) opts.onChange(Object.keys(activeIds));
          } else {
            activeId = b.id;
            wrap.querySelectorAll("button").forEach(function(x) { x.removeAttribute("data-goop-active"); });
            btn.setAttribute("data-goop-active", "");
            _f(wrap, "change", { value: activeId });
            if (opts.onChange) opts.onChange(activeId);
          }
        });
        wrap.appendChild(btn);
      });
    }

    render();

    return {
      getValue: function() { return multi ? Object.keys(activeIds) : activeId; },
      setValue: function(v) { if (multi) { activeIds = {}; (Array.isArray(v) ? v : [v]).forEach(function(id) { activeIds[id] = true; }); } else activeId = v; render(); },
      setButtonDisabled: function(id, v) { var b = buttons.find(function(x) { return x.id === id; }); if (b) { b.disabled = !!v; render(); } },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
