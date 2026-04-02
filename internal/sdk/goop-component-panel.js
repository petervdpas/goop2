//
// CSS hooks for Goop.ui.panel:
//   .gc-panel               — wrapper
//   .gc-panel-header        — header row
//   .gc-panel-header-actions— header buttons area
//   .gc-panel-collapse-btn  — collapse toggle
//   .gc-panel-body          — content area
//   .gc-panel-footer        — footer area
//   [data-goop-collapsed]   — panel is collapsed
//   [data-goop-collapsible] — panel can collapse
//   [data-goop-scroll]      — body has overflow scroll
//   [data-goop-variant="flat|raised"]
//
// CSS hooks for Goop.ui.scrollbox:
//   .gc-scrollbox           — scrollable container
//
// CSS hooks for Goop.ui.splitpane:
//   .gc-splitpane           — flex container
//   .gc-splitpane-panel     — each pane
//   .gc-splitpane-divider   — draggable divider
//   [data-goop-dir="vertical"] — vertical split
//

(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };

  Goop.ui.panel = function(opts) {
    opts = opts || {};

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-panel";
    wrap.setAttribute("data-goop-component", "panel");
    if (opts.variant) wrap.setAttribute("data-goop-variant", opts.variant);
    var collapsible = !!opts.collapsible;
    var collapsed = !!opts.collapsed;
    if (collapsible) wrap.setAttribute("data-goop-collapsible", "");
    if (collapsed) wrap.setAttribute("data-goop-collapsed", "");

    var header = null;
    if (opts.title || opts.actions || collapsible) {
      header = document.createElement("div");
      header.className = "gc-panel-header";
      var titleSpan = document.createElement("span");
      titleSpan.textContent = opts.title || "";
      header.appendChild(titleSpan);

      var actionsDiv = document.createElement("div");
      actionsDiv.className = "gc-panel-header-actions";
      if (collapsible) {
        var colBtn = document.createElement("button");
        colBtn.type = "button";
        colBtn.className = "gc-panel-collapse-btn";
        colBtn.textContent = "\u25BC";
        colBtn.addEventListener("click", function() {
          collapsed = !collapsed;
          if (collapsed) wrap.setAttribute("data-goop-collapsed", "");
          else wrap.removeAttribute("data-goop-collapsed");
        });
        actionsDiv.appendChild(colBtn);
      }
      header.appendChild(actionsDiv);
      wrap.appendChild(header);
    }

    var body = document.createElement("div");
    body.className = "gc-panel-body";
    if (opts.maxHeight) { body.setAttribute("data-goop-scroll", ""); body.style.maxHeight = typeof opts.maxHeight === "number" ? opts.maxHeight + "px" : opts.maxHeight; }
    if (opts.padding === false) body.style.padding = "0";
    if (opts.content) body.innerHTML = opts.content;
    wrap.appendChild(body);

    var footer = null;
    if (opts.footer) {
      footer = document.createElement("div");
      footer.className = "gc-panel-footer";
      footer.innerHTML = opts.footer;
      wrap.appendChild(footer);
    }


    return {
      body: body,
      header: header,
      footer: footer,
      setTitle: function(t) { if (header) header.querySelector("span").textContent = t; },
      collapse: function() { collapsed = true; wrap.setAttribute("data-goop-collapsed", ""); },
      expand: function() { collapsed = false; wrap.removeAttribute("data-goop-collapsed"); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };

  Goop.ui.scrollbox = function(opts) {
    opts = opts || {};
    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-scrollbox";
    wrap.setAttribute("data-goop-component", "scrollbox");
    if (opts.maxHeight) wrap.style.maxHeight = typeof opts.maxHeight === "number" ? opts.maxHeight + "px" : opts.maxHeight;
    if (opts.height) wrap.style.height = typeof opts.height === "number" ? opts.height + "px" : opts.height;
    if (opts.content) wrap.innerHTML = opts.content;

    return {
      scrollTo: function(y) { wrap.scrollTop = y; },
      scrollToBottom: function() { wrap.scrollTop = wrap.scrollHeight; },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };

  Goop.ui.splitpane = function(opts) {
    opts = opts || {};
    var direction = opts.direction || "horizontal";
    var sizes = opts.sizes || [50, 50];
    var minSize = opts.minSize || 50;

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-splitpane";
    wrap.setAttribute("data-goop-component", "splitpane");
    if (direction === "vertical") wrap.setAttribute("data-goop-dir", "vertical");

    var panelA = document.createElement("div");
    panelA.className = "gc-splitpane-panel";
    var divider = document.createElement("div");
    divider.className = "gc-splitpane-divider";
    var panelB = document.createElement("div");
    panelB.className = "gc-splitpane-panel";

    var isHoriz = direction !== "vertical";
    var prop = isHoriz ? "width" : "height";

    panelA.style.flex = "0 0 " + sizes[0] + "%";
    panelB.style.flex = "1 1 0";

    wrap.appendChild(panelA);
    wrap.appendChild(divider);
    wrap.appendChild(panelB);

    var dragging = false;
    divider.addEventListener("mousedown", function(e) {
      e.preventDefault();
      dragging = true;
      var rect = wrap.getBoundingClientRect();
      var totalSize = isHoriz ? rect.width : rect.height;

      function onMove(e) {
        if (!dragging) return;
        var pos = isHoriz ? (e.clientX - rect.left) : (e.clientY - rect.top);
        var pct = Math.max(minSize / totalSize * 100, Math.min(100 - minSize / totalSize * 100, (pos / totalSize) * 100));
        panelA.style.flex = "0 0 " + pct + "%";
      }
      function onUp() { dragging = false; document.removeEventListener("mousemove", onMove); document.removeEventListener("mouseup", onUp); }
      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
    });

    return {
      panelA: panelA,
      panelB: panelB,
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
