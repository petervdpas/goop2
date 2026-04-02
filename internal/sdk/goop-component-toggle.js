(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.toggle = function(el, opts) {
    opts = opts || {};
    var checked = !!opts.checked;
    var isDisabled = !!opts.disabled;
    var checkedClass = opts.checkedClass || "";
    var checkedAttr = opts.checkedAttr || "";
    var disabledAttr = opts.disabledAttr || "";

    function update() {
      if (checkedClass) { if (checked) el.classList.add(checkedClass); else el.classList.remove(checkedClass); }
      if (checkedAttr) { if (checked) el.setAttribute(checkedAttr, ""); else el.removeAttribute(checkedAttr); }
    }

    function toggle() {
      if (isDisabled) return;
      checked = !checked;
      update();
      _f(el, "change", { value: checked }); _f(el, "input", { value: checked });
      if (opts.onChange) opts.onChange(checked);
    }

    el.addEventListener("click", toggle);
    el.addEventListener("keydown", function(e) { if (e.key === " " || e.key === "Enter") { e.preventDefault(); toggle(); } });
    if (!isDisabled) el.tabIndex = el.tabIndex >= 0 ? el.tabIndex : 0;
    update();

    return {
      getValue: function() { return checked; },
      setValue: function(v) { checked = !!v; update(); },
      setDisabled: function(v) {
        isDisabled = !!v;
        el.tabIndex = isDisabled ? -1 : 0;
        if (disabledAttr) { if (isDisabled) el.setAttribute(disabledAttr, ""); else el.removeAttribute(disabledAttr); }
      },
      destroy: function() {},
      el: el,
    };
  };
})();
