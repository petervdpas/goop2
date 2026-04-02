//
// CSS hooks:
//   .gc-tooltip-wrap        — wrapper around target element
//   .gc-tooltip             — the tooltip popup
//   [data-goop-pos="top|bottom|left|right"] — position
//   [data-goop-open]        — force-show (normally shows on hover)
//

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
    tip.className = opts.tipClass || "gc-tooltip";
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
