(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.tooltip = function(el, opts) {
    opts = opts || {};
    var text = opts.text || "";
    var pos = opts.position || "top";

    var wrap = document.createElement("span");
    wrap.className = opts.class || "gc-tooltip-wrap";
    while (el.firstChild) wrap.appendChild(el.firstChild);
    el.appendChild(wrap);

    var tip = document.createElement("span");
    tip.className = "gc-tooltip";
    tip.setAttribute("data-goop-pos", pos);
    tip.textContent = text;
    wrap.appendChild(tip);

    return {
      setText: function(t) { tip.textContent = t; },
      show: function() { tip.setAttribute("data-goop-open", ""); },
      hide: function() { tip.removeAttribute("data-goop-open"); },
      destroy: function() {
        while (wrap.firstChild) { if (wrap.firstChild !== tip) el.appendChild(wrap.firstChild); else wrap.removeChild(wrap.firstChild); }
        wrap.remove();
      },
      el: wrap,
    };
  };
})();
