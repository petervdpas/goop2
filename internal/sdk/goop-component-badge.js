(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.badge = function(el, opts) {
    opts = opts || {};
    var variantAttr = opts.variantAttr || "";
    var dotSelector = opts.dot || "";

    return {
      setText: function(t) { el.textContent = t; },
      setVariant: function(v) {
        if (variantAttr) {
          if (v) el.setAttribute(variantAttr, v);
          else el.removeAttribute(variantAttr);
        }
      },
      showDot: function(show) {
        if (!dotSelector) return;
        var dot = el.querySelector(dotSelector);
        if (dot) dot.hidden = !show;
      },
      destroy: function() {},
      el: el,
    };
  };
})();
