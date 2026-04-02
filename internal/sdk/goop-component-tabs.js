(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-tabs-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-tabs { font: var(--goop-font, inherit); }
      .gc-tabs-bar { display: flex; gap: 0; border-bottom: 1px solid var(--goop-border, #2a3142); }
      .gc-tabs-bar button {
        padding: .5rem 1rem; border: none; border-bottom: 2px solid transparent;
        background: none; color: var(--goop-muted, #9aa3b2); cursor: pointer; font: inherit; font-size: .9rem;
        transition: color .15s, border-color .15s;
      }
      .gc-tabs-bar button:hover:not([disabled]) { color: var(--goop-text, #e6e9ef); }
      .gc-tabs-bar button[data-goop-active] { color: var(--goop-accent, #7aa2ff); border-bottom-color: var(--goop-accent, #7aa2ff); }
      .gc-tabs-bar button:disabled { opacity: .4; cursor: not-allowed; }
      .gc-tabs-panel { display: none; padding: .75rem 0; }
      .gc-tabs-panel[data-goop-active] { display: block; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.tabs = function(el, opts) {
    opts = opts || {};
    var tabs = opts.tabs || [];
    var activeId = opts.active || (tabs[0] && tabs[0].id) || "";

    var wrap = document.createElement("div");
    wrap.className = "gc-tabs";
    wrap.setAttribute("data-goop-component", "tabs");
    var bar = document.createElement("div"); bar.className = "gc-tabs-bar";
    var panels = document.createElement("div");
    wrap.appendChild(bar); wrap.appendChild(panels);
    el.appendChild(wrap);

    function render() {
      bar.innerHTML = ""; panels.innerHTML = "";
      tabs.forEach(function(tab) {
        var btn = document.createElement("button"); btn.type = "button";
        btn.textContent = tab.label || tab.id;
        if (tab.id === activeId) btn.setAttribute("data-goop-active", "");
        if (tab.disabled) btn.disabled = true;
        btn.addEventListener("click", function() { if (!tab.disabled) activate(tab.id); });
        bar.appendChild(btn);

        var panel = document.createElement("div");
        panel.className = "gc-tabs-panel";
        panel.id = "gc-tab-" + tab.id;
        if (tab.id === activeId) panel.setAttribute("data-goop-active", "");
        if (tab.content) panel.innerHTML = tab.content;
        panels.appendChild(panel);
      });
    }

    function activate(id) {
      activeId = id;
      bar.querySelectorAll("button").forEach(function(btn, i) {
        if (tabs[i].id === id) btn.setAttribute("data-goop-active", ""); else btn.removeAttribute("data-goop-active");
      });
      panels.querySelectorAll(".gc-tabs-panel").forEach(function(p, i) {
        if (tabs[i].id === id) p.setAttribute("data-goop-active", ""); else p.removeAttribute("data-goop-active");
      });
      _f(wrap, "change", { value: id });
      if (opts.onChange) opts.onChange(id);
    }

    render();

    return {
      getValue: function() { return activeId; },
      setValue: function(id) { activate(id); },
      getPanel: function(id) { return panels.querySelector("#gc-tab-" + id); },
      setTabDisabled: function(id, v) { var t = tabs.find(function(x) { return x.id === id; }); if (t) { t.disabled = !!v; render(); } },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
