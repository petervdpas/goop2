(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.toggle = function(el, opts) {
    opts = opts || {};
    var checked = !!opts.checked;
    var isDisabled = !!opts.disabled;

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-toggle";
    wrap.tabIndex = isDisabled ? -1 : 0;
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    wrap.innerHTML = '<div class="gc-toggle-track"><div class="gc-toggle-thumb"></div></div>' +
      (opts.label ? '<span class="gc-toggle-label">' + _e(opts.label) + "</span>" : "");
    el.appendChild(wrap);

    function update() {
      if (checked) wrap.setAttribute("data-goop-checked", "");
      else wrap.removeAttribute("data-goop-checked");
    }
    function toggle() {
      if (isDisabled) return;
      checked = !checked; update();
      _f(wrap, "change", { value: checked }); _f(wrap, "input", { value: checked });
      if (opts.onChange) opts.onChange(checked);
    }

    wrap.addEventListener("click", toggle);
    wrap.addEventListener("keydown", function(e) { if (e.key === " " || e.key === "Enter") { e.preventDefault(); toggle(); } });
    update();

    return {
      getValue: function() { return checked; },
      setValue: function(v) { checked = !!v; update(); },
      setDisabled: function(v) { isDisabled = !!v; wrap.tabIndex = isDisabled ? -1 : 0; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
