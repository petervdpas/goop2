(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.tooltip = function(el, opts) {
    opts = opts || {};
    var openAttr = opts.openAttr || "";
    var openClass = opts.openClass || "";
    var tip = opts.tip ? el.querySelector(opts.tip) : null;

    return {
      setText: function(t) { if (tip) tip.textContent = t; },
      show: function() {
        if (tip) {
          if (openClass) tip.classList.add(openClass);
          if (openAttr) tip.setAttribute(openAttr, "");
        }
      },
      hide: function() {
        if (tip) {
          if (openClass) tip.classList.remove(openClass);
          if (openAttr) tip.removeAttribute(openAttr);
        }
      },
      destroy: function() {},
      el: el,
    };
  };
})();
