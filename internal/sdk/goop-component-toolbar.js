(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.toolbar = function(el, opts) {
    opts = opts || {};
    var buttons = opts.buttons || [];
    var activeId = opts.active || (buttons[0] && buttons[0].id) || "";
    var multi = !!opts.multi;
    var activeIds = {};
    if (multi && opts.active) (Array.isArray(opts.active) ? opts.active : [opts.active]).forEach(function(id) { activeIds[id] = true; });

    var btnClass = opts.buttonClass || "";
    var activeClass = opts.activeClass || null;

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-toolbar";
    el.appendChild(wrap);

    function setActive(btn, on) {
      if (on) btn.setAttribute("data-goop-active", "");
      else btn.removeAttribute("data-goop-active");
      if (activeClass) { if (on) btn.classList.add(activeClass); else btn.classList.remove(activeClass); }
    }

    function render() {
      wrap.innerHTML = "";
      buttons.forEach(function(b) {
        var btn = document.createElement("button"); btn.type = "button";
        btn.textContent = b.label || b.id;
        if (btnClass) btn.className = btnClass;
        if (b.title) btn.title = b.title;
        if (b.disabled) btn.disabled = true;
        btn.setAttribute("data-goop-id", b.id);
        if (multi) { if (activeIds[b.id]) setActive(btn, true); }
        else { if (b.id === activeId) setActive(btn, true); }
        btn.addEventListener("click", function() {
          if (b.disabled) return;
          if (multi) {
            if (activeIds[b.id]) { delete activeIds[b.id]; setActive(btn, false); }
            else { activeIds[b.id] = true; setActive(btn, true); }
            _f(wrap, "change", { value: Object.keys(activeIds) });
            if (opts.onChange) opts.onChange(Object.keys(activeIds));
          } else {
            activeId = b.id;
            wrap.querySelectorAll("button").forEach(function(x) { setActive(x, false); });
            setActive(btn, true);
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
