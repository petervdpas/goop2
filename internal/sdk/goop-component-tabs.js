(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.tabs = function(el, opts) {
    opts = opts || {};
    var tabAttr = opts.tabAttr || "data-tab";
    var panelAttr = opts.panelAttr || tabAttr;
    var activeClass = opts.activeClass || "";
    var activeAttr = opts.activeAttr || "";

    var bar = opts.bar ? el.querySelector(opts.bar) : el;
    var panelContainer = opts.panels ? el.querySelector(opts.panels) : el;
    var activeId = opts.active || "";

    function getButtons() {
      return bar ? Array.from(bar.querySelectorAll("[" + tabAttr + "]")) : [];
    }

    function getPanels() {
      return panelContainer ? Array.from(panelContainer.querySelectorAll("[" + panelAttr + "]")) : [];
    }

    function activate(id) {
      activeId = id;
      getButtons().forEach(function(btn) {
        var match = btn.getAttribute(tabAttr) === id;
        if (activeClass) { if (match) btn.classList.add(activeClass); else btn.classList.remove(activeClass); }
        if (activeAttr) { if (match) btn.setAttribute(activeAttr, ""); else btn.removeAttribute(activeAttr); }
      });
      getPanels().forEach(function(panel) {
        var match = panel.getAttribute(panelAttr) === id;
        if (activeClass) { if (match) panel.classList.add(activeClass); else panel.classList.remove(activeClass); }
        if (activeAttr) { if (match) panel.setAttribute(activeAttr, ""); else panel.removeAttribute(activeAttr); }
      });
      _f(el, "change", { value: id });
      if (opts.onChange) opts.onChange(id);
    }

    getButtons().forEach(function(btn) {
      btn.addEventListener("click", function() {
        if (btn.disabled) return;
        activate(btn.getAttribute(tabAttr));
      });
    });

    if (!activeId) {
      var btns = getButtons();
      if (btns.length) activeId = btns[0].getAttribute(tabAttr);
    }
    if (activeId) activate(activeId);

    return {
      getValue: function() { return activeId; },
      setValue: function(id) { activate(id); },
      getPanel: function(id) {
        return panelContainer ? panelContainer.querySelector("[" + panelAttr + '="' + id + '"]') : null;
      },
      destroy: function() {},
      el: el,
    };
  };
})();
