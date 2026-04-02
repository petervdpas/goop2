(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-pagination-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-pagination { display: flex; align-items: center; gap: .25rem; font: var(--goop-font, inherit); }
      .gc-pagination button {
        min-width: 2rem; height: 2rem; display: flex; align-items: center; justify-content: center;
        border: 1px solid var(--goop-border, #2a3142); border-radius: var(--goop-radius, 6px);
        background: var(--goop-field, rgba(0,0,0,.25)); color: var(--goop-text, #e6e9ef); cursor: pointer; font: inherit; font-size: .85rem;
      }
      .gc-pagination button:hover:not([disabled]) { border-color: var(--goop-accent, #7aa2ff); }
      .gc-pagination button[data-goop-active] { background: var(--goop-accent, #7aa2ff); color: var(--goop-bg, #0f1115); border-color: var(--goop-accent, #7aa2ff); }
      .gc-pagination button:disabled { opacity: .4; cursor: not-allowed; }
      .gc-pagination-ellipsis { color: var(--goop-muted, #9aa3b2); padding: 0 .3rem; }
      .gc-pagination-info { color: var(--goop-muted, #9aa3b2); font-size: .8rem; margin-left: .5rem; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.pagination = function(el, opts) {
    opts = opts || {};
    var perPage = opts.perPage || 0;
    var totalItems = opts.totalItems || 0;
    var total = opts.total || (perPage > 0 && totalItems > 0 ? Math.ceil(totalItems / perPage) : 1);
    var page = opts.page || 1;
    var maxButtons = opts.maxButtons || 7;

    var wrap = document.createElement("div");
    wrap.className = "gc-pagination";
    wrap.setAttribute("data-goop-component", "pagination");
    el.appendChild(wrap);

    function render() {
      wrap.innerHTML = "";
      var prev = document.createElement("button"); prev.type = "button"; prev.textContent = "\u2039"; prev.disabled = page <= 1;
      prev.addEventListener("click", function() { if (page > 1) goTo(page - 1); });
      wrap.appendChild(prev);

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
        if (p === "...") { var span = document.createElement("span"); span.className = "gc-pagination-ellipsis"; span.textContent = "\u2026"; wrap.appendChild(span); }
        else { var btn = document.createElement("button"); btn.type = "button"; btn.textContent = p; if (p === page) btn.setAttribute("data-goop-active", ""); btn.addEventListener("click", function() { goTo(p); }); wrap.appendChild(btn); }
      });

      var next = document.createElement("button"); next.type = "button"; next.textContent = "\u203A"; next.disabled = page >= total;
      next.addEventListener("click", function() { if (page < total) goTo(page + 1); });
      wrap.appendChild(next);

      if (opts.showInfo) {
        var info = document.createElement("span"); info.className = "gc-pagination-info";
        info.textContent = "Page " + page + " of " + total;
        wrap.appendChild(info);
      }
    }

    function goTo(p) {
      page = Math.max(1, Math.min(total, p)); render();
      _f(wrap, "change", { page: page });
      if (opts.onChange) opts.onChange(page);
    }

    render();

    return {
      getValue: function() { return page; },
      setValue: function(p) { goTo(p); },
      setTotal: function(t) { total = t; if (page > total) page = total || 1; render(); },
      setTotalItems: function(n) { totalItems = n; if (perPage > 0) total = Math.ceil(totalItems / perPage); if (page > total) page = total || 1; render(); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
