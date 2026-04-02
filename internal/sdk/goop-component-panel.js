(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };

  var SID = "gc-panel-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-panel {
        background: var(--goop-panel, #151924); border: 1px solid var(--goop-border, #2a3142);
        border-radius: var(--goop-radius, 6px); font: var(--goop-font, inherit); color: var(--goop-text, #e6e9ef);
        overflow: hidden;
      }
      .gc-panel[data-goop-variant="flat"] { border: none; box-shadow: none; }
      .gc-panel[data-goop-variant="raised"] { box-shadow: 0 4px 16px rgba(0,0,0,.2); }
      .gc-panel-header {
        display: flex; align-items: center; justify-content: space-between;
        padding: .6rem .85rem; border-bottom: 1px solid var(--goop-border, #2a3142);
        font-weight: 600; font-size: .95rem;
      }
      .gc-panel-header-actions { display: flex; gap: .35rem; }
      .gc-panel-body { padding: .75rem .85rem; }
      .gc-panel-body[data-goop-scroll] { overflow-y: auto; }
      .gc-panel-footer { padding: .6rem .85rem; border-top: 1px solid var(--goop-border, #2a3142); }
      .gc-panel[data-goop-collapsible] .gc-panel-collapse-btn {
        background: none; border: none; color: var(--goop-muted, #9aa3b2); cursor: pointer;
        font-size: .7rem; transition: transform .2s; padding: .1rem .3rem;
      }
      .gc-panel[data-goop-collapsed] .gc-panel-body,
      .gc-panel[data-goop-collapsed] .gc-panel-footer { display: none; }
      .gc-panel[data-goop-collapsed] .gc-panel-collapse-btn { transform: rotate(-90deg); }

      .gc-scrollbox {
        overflow-y: auto; overflow-x: hidden;
        scrollbar-width: thin;
        scrollbar-color: var(--goop-border, #2a3142) transparent;
      }
      .gc-scrollbox::-webkit-scrollbar { width: 6px; }
      .gc-scrollbox::-webkit-scrollbar-track { background: transparent; }
      .gc-scrollbox::-webkit-scrollbar-thumb { background: var(--goop-border, #2a3142); border-radius: 3px; }

      .gc-splitpane { display: flex; height: 100%; }
      .gc-splitpane[data-goop-dir="vertical"] { flex-direction: column; }
      .gc-splitpane-panel { overflow: auto; min-width: 0; min-height: 0; }
      .gc-splitpane-divider {
        flex-shrink: 0; background: var(--goop-border, #2a3142); cursor: col-resize;
        transition: background .15s;
      }
      .gc-splitpane[data-goop-dir="vertical"] .gc-splitpane-divider { cursor: row-resize; height: 4px; width: 100%; }
      .gc-splitpane:not([data-goop-dir="vertical"]) .gc-splitpane-divider { width: 4px; height: 100%; }
      .gc-splitpane-divider:hover { background: var(--goop-accent, #7aa2ff); }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.panel = function(el, opts) {
    opts = opts || {};

    var wrap = document.createElement("div");
    wrap.className = "gc-panel";
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

    el.appendChild(wrap);

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

  Goop.ui.scrollbox = function(el, opts) {
    opts = opts || {};
    var wrap = document.createElement("div");
    wrap.className = "gc-scrollbox";
    wrap.setAttribute("data-goop-component", "scrollbox");
    if (opts.maxHeight) wrap.style.maxHeight = typeof opts.maxHeight === "number" ? opts.maxHeight + "px" : opts.maxHeight;
    if (opts.height) wrap.style.height = typeof opts.height === "number" ? opts.height + "px" : opts.height;
    if (opts.content) wrap.innerHTML = opts.content;
    el.appendChild(wrap);

    return {
      scrollTo: function(y) { wrap.scrollTop = y; },
      scrollToBottom: function() { wrap.scrollTop = wrap.scrollHeight; },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };

  Goop.ui.splitpane = function(el, opts) {
    opts = opts || {};
    var direction = opts.direction || "horizontal";
    var sizes = opts.sizes || [50, 50];
    var minSize = opts.minSize || 50;

    var wrap = document.createElement("div");
    wrap.className = "gc-splitpane";
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
    el.appendChild(wrap);

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
