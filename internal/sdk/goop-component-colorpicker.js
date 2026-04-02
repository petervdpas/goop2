(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };
  var _d = Goop.dom;

  var DEFAULTS = [
    "#f87171","#fb923c","#fbbf24","#a3e635","#34d399","#22d3ee","#60a5fa","#a78bfa",
    "#f472b6","#e879f9","#c084fc","#818cf8","#38bdf8","#2dd4bf","#4ade80","#facc15",
    "#ef4444","#f97316","#eab308","#84cc16","#10b981","#06b6d4","#3b82f6","#8b5cf6",
  ];

  Goop.ui.colorpicker = function(el, opts) {
    opts = opts || {};
    var colors = opts.colors || DEFAULTS;
    var current = opts.value || colors[0];
    var isDisabled = !!opts.disabled;
    var showHex = opts.showHex !== false;

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-color";
    wrap.setAttribute("data-goop-component", "colorpicker");
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var swatch = document.createElement("div");
    swatch.className = "gc-color-swatch";

    var hex = document.createElement("input");
    hex.className = "gc-color-hex";
    hex.type = "text"; hex.maxLength = 7;
    if (!showHex) hex.setAttribute("data-goop-hidden", "");

    var popup = document.createElement("div");
    popup.className = "gc-color-popup";
    wrap.appendChild(swatch); wrap.appendChild(hex); wrap.appendChild(popup);
    el.appendChild(wrap);

    function setColor(c) {
      current = c;
      swatch.style.setProperty("--gc-color", c);
      hex.value = c;
    }

    function renderGrid() {
      popup.innerHTML = "";
      var grid = document.createElement("div");
      grid.className = "gc-color-grid";
      colors.forEach(function(c) {
        var btn = document.createElement("button");
        btn.type = "button";
        btn.setAttribute("data-goop-color", c);
        btn.style.setProperty("--gc-color", c);
        if (c.toLowerCase() === current.toLowerCase()) btn.setAttribute("data-goop-selected", "");
        btn.addEventListener("click", function(e) {
          e.stopPropagation(); setColor(c);
          _f(wrap, "change", { value: current }); _f(wrap, "input", { value: current });
          if (opts.onChange) opts.onChange(current); closePopup();
        });
        grid.appendChild(btn);
      });
      popup.appendChild(grid);
    }

    function openPopup() { if (isDisabled) return; renderGrid(); popup.setAttribute("data-goop-open", ""); }
    function closePopup() { popup.removeAttribute("data-goop-open"); }
    swatch.addEventListener("click", function(e) { e.stopPropagation(); if (popup.hasAttribute("data-goop-open")) closePopup(); else openPopup(); });
    hex.addEventListener("change", function() { var v = hex.value.trim(); if (/^#[0-9a-fA-F]{3,6}$/.test(v)) { setColor(v); _f(wrap, "change", { value: current }); if (opts.onChange) opts.onChange(current); } });
    function onDocClick(e) { if (!wrap.contains(e.target)) closePopup(); }
    document.addEventListener("click", onDocClick);
    setColor(current);

    return {
      getValue: function() { return current; },
      setValue: function(v) { setColor(v); },
      setDisabled: function(v) { isDisabled = !!v; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); },
      destroy: function() { document.removeEventListener("click", onDocClick); wrap.remove(); },
      el: wrap,
    };
  };
})();
