//
// CSS hooks:
//   .gc-stepper             — wrapper (override via opts.class)
//   button:first-child      — decrement button
//   button:last-child       — increment button
//   input                   — number input
//   [data-goop-disabled]    — disabled state
//   :disabled               — button at min/max
//

(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.stepper = function(opts) {
    opts = opts || {};
    var min = opts.min != null ? opts.min : -Infinity;
    var max = opts.max != null ? opts.max : Infinity;
    var step = opts.step || 1;
    var value = opts.value != null ? opts.value : 0;
    var isDisabled = !!opts.disabled;

    var wrap = document.createElement("div");
    for (var _k in opts) { if (_k.indexOf("data-") === 0) wrap.setAttribute(_k, opts[_k]); }
    wrap.className = opts.class || "gc-stepper";
    wrap.setAttribute("data-goop-component", "stepper");
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var btnDec = document.createElement("button"); btnDec.type = "button"; btnDec.textContent = "\u2212";
    var input = document.createElement("input"); input.className = opts.inputClass || ""; input.type = "number";
    if (min !== -Infinity) input.min = min;
    if (max !== Infinity) input.max = max;
    input.step = step;
    if (isDisabled) input.disabled = true;
    var btnInc = document.createElement("button"); btnInc.type = "button"; btnInc.textContent = "+";

    wrap.appendChild(btnDec); wrap.appendChild(input); wrap.appendChild(btnInc);

    function clamp(v) { return Math.max(min, Math.min(max, v)); }
    function setValue(v) { value = clamp(v); input.value = value; btnDec.disabled = isDisabled || value <= min; btnInc.disabled = isDisabled || value >= max; }
    function emitChange() { _f(wrap, "change", { value: value }); _f(wrap, "input", { value: value }); if (opts.onChange) opts.onChange(value); }

    btnDec.addEventListener("click", function() { if (!isDisabled) { setValue(value - step); emitChange(); } });
    btnInc.addEventListener("click", function() { if (!isDisabled) { setValue(value + step); emitChange(); } });
    input.addEventListener("change", function() { var v = parseFloat(input.value); if (!isNaN(v)) { setValue(v); emitChange(); } else input.value = value; });
    setValue(value);

    return {
      getValue: function() { return value; },
      setValue: function(v) { setValue(v); },
      setDisabled: function(v) { isDisabled = !!v; input.disabled = isDisabled; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); setValue(value); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
