(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.progress = function(el, opts) {
    opts = opts || {};
    var value = opts.value || 0;
    var max = opts.max || 100;

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-progress";
    wrap.setAttribute("data-goop-component", "progress");
    if (opts.variant) wrap.setAttribute("data-goop-variant", opts.variant);
    if (opts.animated) wrap.setAttribute("data-goop-animated", "");
    if (opts.height) wrap.style.height = typeof opts.height === "number" ? opts.height + "px" : opts.height;

    var bar = document.createElement("div");
    bar.className = "gc-progress-bar";

    var label = null;
    if (opts.label !== false) {
      label = document.createElement("div");
      label.className = "gc-progress-label";
    }

    wrap.appendChild(bar);
    el.appendChild(wrap);
    if (label) el.appendChild(label);

    function update() {
      var pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
      bar.style.width = pct + "%";
      if (label) {
        label.textContent = typeof opts.format === "function"
          ? opts.format(value, max, pct)
          : Math.round(pct) + "%";
      }
    }

    update();

    return {
      getValue: function() { return value; },
      setValue: function(v, m) { value = v; if (m != null) max = m; update(); },
      destroy: function() { wrap.remove(); if (label) label.remove(); },
      el: wrap,
    };
  };
})();
