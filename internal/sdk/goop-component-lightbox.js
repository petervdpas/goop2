(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.lightbox = function(el, opts) {
    opts = opts || {};
    var items = opts.items || [];
    var current = 0;
    var loop = opts.loop !== false;
    var openAttr = opts.openAttr || "";
    var openClass = opts.openClass || "";

    var img = opts.img ? el.querySelector(opts.img) : el.querySelector("img");
    var prevBtn = opts.prev ? el.querySelector(opts.prev) : null;
    var nextBtn = opts.next ? el.querySelector(opts.next) : null;
    var closeBtn = opts.close ? el.querySelector(opts.close) : null;
    var captionEl = opts.caption ? el.querySelector(opts.caption) : null;
    var counterEl = opts.counter ? el.querySelector(opts.counter) : null;

    function show(idx) {
      if (!items.length) return;
      if (loop) current = ((idx % items.length) + items.length) % items.length;
      else current = Math.max(0, Math.min(items.length - 1, idx));
      var item = items[current];
      if (img) img.src = typeof item === "string" ? item : item.src;
      if (captionEl) captionEl.textContent = (typeof item === "string" ? "" : item.caption) || "";
      if (counterEl) counterEl.textContent = (current + 1) + " / " + items.length;
      if (!loop) {
        if (prevBtn) prevBtn.disabled = current <= 0;
        if (nextBtn) nextBtn.disabled = current >= items.length - 1;
      }
      if (openClass) el.classList.add(openClass);
      if (openAttr) el.setAttribute(openAttr, "");
      if (opts.onChange) opts.onChange(current);
    }

    function hide() {
      if (openClass) el.classList.remove(openClass);
      if (openAttr) el.removeAttribute(openAttr);
    }

    if (closeBtn) closeBtn.addEventListener("click", hide);
    if (prevBtn) prevBtn.addEventListener("click", function() { show(current - 1); });
    if (nextBtn) nextBtn.addEventListener("click", function() { show(current + 1); });
    el.addEventListener("click", function(e) { if (e.target === el) hide(); });

    function onKey(e) {
      var isOpen = (openClass && el.classList.contains(openClass)) || (openAttr && el.hasAttribute(openAttr));
      if (!isOpen) return;
      if (e.key === "Escape") hide();
      if (e.key === "ArrowLeft") show(current - 1);
      if (e.key === "ArrowRight") show(current + 1);
    }
    document.addEventListener("keydown", onKey);

    return {
      open: show,
      close: hide,
      setItems: function(i) { items = i; },
      destroy: function() { document.removeEventListener("keydown", onKey); },
      el: el,
    };
  };
})();
