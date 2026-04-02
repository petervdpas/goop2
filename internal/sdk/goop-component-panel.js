(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.panel = function(el, opts) {
    opts = opts || {};
    var collapsed = !!opts.collapsed;
    var collapsedClass = opts.collapsedClass || "";
    var collapsedAttr = opts.collapsedAttr || "";

    var header = opts.header ? el.querySelector(opts.header) : null;
    var body = opts.body ? el.querySelector(opts.body) : null;
    var footer = opts.footer ? el.querySelector(opts.footer) : null;
    var collapseBtn = opts.collapseBtn ? el.querySelector(opts.collapseBtn) : null;

    function updateCollapsed() {
      if (collapsedClass) { if (collapsed) el.classList.add(collapsedClass); else el.classList.remove(collapsedClass); }
      if (collapsedAttr) { if (collapsed) el.setAttribute(collapsedAttr, ""); else el.removeAttribute(collapsedAttr); }
    }

    if (collapseBtn) {
      collapseBtn.addEventListener("click", function() {
        collapsed = !collapsed;
        updateCollapsed();
      });
    }

    updateCollapsed();

    return {
      body: body,
      header: header,
      footer: footer,
      setTitle: function(t) { if (header) { var s = header.querySelector("span"); if (s) s.textContent = t; } },
      collapse: function() { collapsed = true; updateCollapsed(); },
      expand: function() { collapsed = false; updateCollapsed(); },
      destroy: function() {},
      el: el,
    };
  };

  Goop.ui.scrollbox = function(el, opts) {
    opts = opts || {};
    return {
      scrollTo: function(y) { el.scrollTop = y; },
      scrollToBottom: function() { el.scrollTop = el.scrollHeight; },
      destroy: function() {},
      el: el,
    };
  };

  Goop.ui.splitpane = function(el, opts) {
    opts = opts || {};
    var direction = opts.direction || "horizontal";
    var minSize = opts.minSize || 50;
    var flexVar = opts.flexVar || "--gc-split";

    var panelA = opts.panelA ? el.querySelector(opts.panelA) : null;
    var panelB = opts.panelB ? el.querySelector(opts.panelB) : null;
    var divider = opts.divider ? el.querySelector(opts.divider) : null;
    var isHoriz = direction !== "vertical";

    if (opts.initial && panelA) panelA.style.setProperty(flexVar, opts.initial + "%");

    if (divider) {
      var dragging = false;
      divider.addEventListener("mousedown", function(e) {
        e.preventDefault();
        dragging = true;
        var rect = el.getBoundingClientRect();
        var totalSize = isHoriz ? rect.width : rect.height;

        function onMove(e) {
          if (!dragging) return;
          var pos = isHoriz ? (e.clientX - rect.left) : (e.clientY - rect.top);
          var pct = Math.max(minSize / totalSize * 100, Math.min(100 - minSize / totalSize * 100, (pos / totalSize) * 100));
          if (panelA) panelA.style.setProperty(flexVar, pct + "%");
        }
        function onUp() { dragging = false; document.removeEventListener("mousemove", onMove); document.removeEventListener("mouseup", onUp); }
        document.addEventListener("mousemove", onMove);
        document.addEventListener("mouseup", onUp);
      });
    }

    return {
      panelA: panelA,
      panelB: panelB,
      destroy: function() {},
      el: el,
    };
  };
})();
