(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-stepper-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-stepper { display: inline-flex; align-items: center; font: var(--goop-font, inherit); }
      .gc-stepper button {
        width: 2rem; height: 2rem; display: flex; align-items: center; justify-content: center;
        border: 1px solid var(--goop-border, #2a3142); background: var(--goop-field, rgba(0,0,0,.25));
        color: var(--goop-text, #e6e9ef); cursor: pointer; font: inherit; font-size: 1rem; transition: border-color .15s;
      }
      .gc-stepper button:hover:not([disabled]) { border-color: var(--goop-accent, #7aa2ff); }
      .gc-stepper button:first-child { border-radius: var(--goop-radius, 6px) 0 0 var(--goop-radius, 6px); }
      .gc-stepper button:last-child { border-radius: 0 var(--goop-radius, 6px) var(--goop-radius, 6px) 0; }
      .gc-stepper button:disabled { opacity: .4; cursor: not-allowed; }
      .gc-stepper input {
        width: 3.5rem; height: 2rem; box-sizing: border-box; text-align: center;
        border: 1px solid var(--goop-border, #2a3142); border-left: none; border-right: none;
        background: var(--goop-field, rgba(0,0,0,.25)); color: var(--goop-text, #e6e9ef); font: inherit; font-size: .9rem;
        outline: none; -moz-appearance: textfield;
      }
      .gc-stepper input::-webkit-inner-spin-button, .gc-stepper input::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.stepper = function(el, opts) {
    opts = opts || {};
    var min = opts.min != null ? opts.min : -Infinity;
    var max = opts.max != null ? opts.max : Infinity;
    var step = opts.step || 1;
    var value = opts.value != null ? opts.value : 0;
    var isDisabled = !!opts.disabled;

    var wrap = document.createElement("div");
    wrap.className = "gc-stepper";
    wrap.setAttribute("data-goop-component", "stepper");
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var btnDec = document.createElement("button"); btnDec.type = "button"; btnDec.textContent = "\u2212";
    var input = document.createElement("input"); input.type = "number";
    if (min !== -Infinity) input.min = min;
    if (max !== Infinity) input.max = max;
    input.step = step;
    if (isDisabled) input.disabled = true;
    var btnInc = document.createElement("button"); btnInc.type = "button"; btnInc.textContent = "+";

    wrap.appendChild(btnDec); wrap.appendChild(input); wrap.appendChild(btnInc);
    el.appendChild(wrap);

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
