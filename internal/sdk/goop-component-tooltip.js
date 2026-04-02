(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  var SID = "gc-tooltip-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-tooltip-wrap { position: relative; display: inline-block; }
      .gc-tooltip {
        display: none; position: absolute; z-index: 9970;
        padding: .3rem .6rem; border-radius: var(--goop-radius, 6px);
        background: var(--goop-text, #e6e9ef); color: var(--goop-bg, #0f1115);
        font-size: .75rem; white-space: nowrap; pointer-events: none;
      }
      .gc-tooltip[data-goop-pos="top"] { bottom: calc(100% + 6px); left: 50%; transform: translateX(-50%); }
      .gc-tooltip[data-goop-pos="bottom"] { top: calc(100% + 6px); left: 50%; transform: translateX(-50%); }
      .gc-tooltip[data-goop-pos="left"] { right: calc(100% + 6px); top: 50%; transform: translateY(-50%); }
      .gc-tooltip[data-goop-pos="right"] { left: calc(100% + 6px); top: 50%; transform: translateY(-50%); }
      .gc-tooltip-wrap:hover .gc-tooltip, .gc-tooltip[data-goop-open] { display: block; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.tooltip = function(el, opts) {
    opts = opts || {};
    var text = opts.text || "";
    var pos = opts.position || "top";

    var wrap = document.createElement("span");
    wrap.className = "gc-tooltip-wrap";
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
