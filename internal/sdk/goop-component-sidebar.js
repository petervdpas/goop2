(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.sidebar = function(el, opts) {
    opts = opts || {};
    var closeOnEscape = opts.closeOnEscape !== false;
    var openClass = opts.openClass || "";
    var openAttr = opts.openAttr || "";

    var backdrop = opts.backdrop ? document.querySelector(opts.backdrop) : null;
    var closeBtn = opts.close ? el.querySelector(opts.close) : null;
    var body = opts.body ? el.querySelector(opts.body) : null;

    function open() {
      if (openClass) { el.classList.add(openClass); if (backdrop) backdrop.classList.add(openClass); }
      if (openAttr) { el.setAttribute(openAttr, ""); if (backdrop) backdrop.setAttribute(openAttr, ""); }
      _f(el, "change", { open: true });
      if (opts.onOpen) opts.onOpen();
    }

    function close() {
      if (openClass) { el.classList.remove(openClass); if (backdrop) backdrop.classList.remove(openClass); }
      if (openAttr) { el.removeAttribute(openAttr); if (backdrop) backdrop.removeAttribute(openAttr); }
      _f(el, "change", { open: false });
      if (opts.onClose) opts.onClose();
    }

    if (backdrop) backdrop.addEventListener("click", close);
    if (closeBtn) closeBtn.addEventListener("click", close);

    function onKey(e) {
      if (!closeOnEscape) return;
      var isOpen = (openClass && el.classList.contains(openClass)) || (openAttr && el.hasAttribute(openAttr));
      if (e.key === "Escape" && isOpen) close();
    }
    document.addEventListener("keydown", onKey);

    return {
      open: open,
      close: close,
      body: body,
      destroy: function() { document.removeEventListener("keydown", onKey); },
      el: el,
    };
  };
})();
