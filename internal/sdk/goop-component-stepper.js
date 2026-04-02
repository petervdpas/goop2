(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.stepper = function(el, opts) {
    opts = opts || {};
    var min = opts.min != null ? opts.min : -Infinity;
    var max = opts.max != null ? opts.max : Infinity;
    var step = opts.step || 1;
    var value = opts.value != null ? opts.value : 0;
    var isDisabled = !!opts.disabled;
    var disabledAttr = opts.disabledAttr || "";

    var input = opts.input ? el.querySelector(opts.input) : el.querySelector("input");
    var decBtn = opts.dec ? el.querySelector(opts.dec) : null;
    var incBtn = opts.inc ? el.querySelector(opts.inc) : null;

    if (!decBtn) { var btns = el.querySelectorAll("button"); if (btns.length >= 2) { decBtn = btns[0]; incBtn = btns[btns.length - 1]; } }

    function clamp(v) { return Math.max(min, Math.min(max, v)); }

    function setValue(v) {
      value = clamp(v);
      if (input) input.value = value;
      if (decBtn) decBtn.disabled = isDisabled || value <= min;
      if (incBtn) incBtn.disabled = isDisabled || value >= max;
    }

    function emitChange() { _f(el, "change", { value: value }); _f(el, "input", { value: value }); if (opts.onChange) opts.onChange(value); }

    if (decBtn) decBtn.addEventListener("click", function() { if (!isDisabled) { setValue(value - step); emitChange(); } });
    if (incBtn) incBtn.addEventListener("click", function() { if (!isDisabled) { setValue(value + step); emitChange(); } });
    if (input) input.addEventListener("change", function() { var v = parseFloat(input.value); if (!isNaN(v)) { setValue(v); emitChange(); } else if (input) input.value = value; });

    setValue(value);

    return {
      getValue: function() { return value; },
      setValue: function(v) { setValue(v); },
      setDisabled: function(v) {
        isDisabled = !!v;
        if (input) input.disabled = isDisabled;
        if (disabledAttr) { if (isDisabled) el.setAttribute(disabledAttr, ""); else el.removeAttribute(disabledAttr); }
        setValue(value);
      },
      destroy: function() {},
      el: el,
    };
  };
})();
