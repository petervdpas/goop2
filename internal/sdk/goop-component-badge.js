(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.badge = function(text, opts) {
    opts = opts || {};
    var el = document.createElement("span");
    el.className = "gc-badge";
    el.textContent = text;
    if (opts.variant) el.setAttribute("data-goop-variant", opts.variant);
    if (opts.dot) {
      var d = document.createElement("span");
      d.className = "gc-badge-dot";
      el.insertBefore(d, el.firstChild);
    }
    return el;
  };
})();
