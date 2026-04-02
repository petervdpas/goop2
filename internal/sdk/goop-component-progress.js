(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.progress = function(el, opts) {
    opts = opts || {};
    var value = opts.value || 0;
    var max = opts.max || 100;
    var progressVar = opts.progressVar || "--gc-progress";

    var bar = opts.bar ? el.querySelector(opts.bar) : el.firstElementChild;
    var label = opts.label ? el.querySelector(opts.label) : null;

    function update() {
      var pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
      if (bar) bar.style.setProperty(progressVar, pct + "%");
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
      destroy: function() {},
      el: el,
    };
  };
})();
