(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.pagination = function(el, opts) {
    opts = opts || {};
    var perPage = opts.perPage || 0;
    var totalItems = opts.totalItems || 0;
    var total = opts.total || (perPage > 0 && totalItems > 0 ? Math.ceil(totalItems / perPage) : 1);
    var page = opts.page || 1;
    var maxButtons = opts.maxButtons || 7;
    var activeClass = opts.activeClass || "";
    var activeAttr = opts.activeAttr || "";
    var ellipsisClass = opts.ellipsisClass || "";
    var infoClass = opts.infoClass || "";
    var buttonClass = opts.buttonClass || "";

    function render() {
      el.innerHTML = "";

      var prev = document.createElement("button"); prev.type = "button"; prev.textContent = "\u2039";
      if (buttonClass) prev.className = buttonClass;
      prev.disabled = page <= 1;
      prev.addEventListener("click", function() { if (page > 1) goTo(page - 1); });
      el.appendChild(prev);

      var pages = [];
      if (total <= maxButtons) { for (var i = 1; i <= total; i++) pages.push(i); }
      else {
        pages.push(1);
        var start = Math.max(2, page - 1), end = Math.min(total - 1, page + 1);
        if (start > 2) pages.push("...");
        for (var i = start; i <= end; i++) pages.push(i);
        if (end < total - 1) pages.push("...");
        pages.push(total);
      }

      pages.forEach(function(p) {
        if (p === "...") {
          var span = document.createElement("span");
          if (ellipsisClass) span.className = ellipsisClass;
          span.textContent = "\u2026";
          el.appendChild(span);
        } else {
          var btn = document.createElement("button"); btn.type = "button"; btn.textContent = p;
          if (buttonClass) btn.className = buttonClass;
          if (p === page) {
            if (activeClass) btn.classList.add(activeClass);
            if (activeAttr) btn.setAttribute(activeAttr, "");
          }
          btn.addEventListener("click", function() { goTo(p); });
          el.appendChild(btn);
        }
      });

      var next = document.createElement("button"); next.type = "button"; next.textContent = "\u203A";
      if (buttonClass) next.className = buttonClass;
      next.disabled = page >= total;
      next.addEventListener("click", function() { if (page < total) goTo(page + 1); });
      el.appendChild(next);

      if (opts.showInfo) {
        var info = document.createElement("span");
        if (infoClass) info.className = infoClass;
        info.textContent = "Page " + page + " of " + total;
        el.appendChild(info);
      }
    }

    function goTo(p) {
      page = Math.max(1, Math.min(total, p)); render();
      _f(el, "change", { page: page });
      if (opts.onChange) opts.onChange(page);
    }

    render();

    return {
      getValue: function() { return page; },
      setValue: function(p) { goTo(p); },
      setTotal: function(t) { total = t; if (page > total) page = total || 1; render(); },
      setTotalItems: function(n) { totalItems = n; if (perPage > 0) total = Math.ceil(totalItems / perPage); if (page > total) page = total || 1; render(); },
      destroy: function() {},
      el: el,
    };
  };
})();
