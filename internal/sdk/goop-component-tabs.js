(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.tabs = function(el, opts) {
    opts = opts || {};
    var tabs = opts.tabs || [];
    var activeId = opts.active || (tabs[0] && tabs[0].id) || "";
    var activeClass = opts.activeClass || "active";
    var tabClass = opts.tabClass || "";

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-tabs";
    var bar = document.createElement("div"); bar.className = opts.barClass || "gc-tabs-bar";
    var panels = document.createElement("div");
    wrap.appendChild(bar); wrap.appendChild(panels);
    el.appendChild(wrap);

    function render() {
      bar.innerHTML = ""; panels.innerHTML = "";
      tabs.forEach(function(tab) {
        var btn = document.createElement("button"); btn.type = "button";
        btn.textContent = tab.label || tab.id;
        if (tabClass) btn.className = tabClass;
        if (tab.id === activeId) btn.classList.add(activeClass);
        if (tab.disabled) btn.disabled = true;
        btn.addEventListener("click", function() { if (!tab.disabled) activate(tab.id); });
        bar.appendChild(btn);

        var panel = document.createElement("div");
        panel.className = opts.panelClass || "gc-tabs-panel";
        panel.id = "gc-tab-" + tab.id;
        if (tab.id === activeId) panel.classList.add(activeClass);
        if (tab.content) panel.innerHTML = tab.content;
        panels.appendChild(panel);
      });
    }

    function activate(id) {
      activeId = id;
      bar.querySelectorAll("button").forEach(function(btn, i) {
        if (tabs[i].id === id) btn.classList.add(activeClass); else btn.classList.remove(activeClass);
      });
      panels.querySelectorAll("[id^='gc-tab-']").forEach(function(p, i) {
        if (tabs[i].id === id) p.classList.add(activeClass); else p.classList.remove(activeClass);
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
