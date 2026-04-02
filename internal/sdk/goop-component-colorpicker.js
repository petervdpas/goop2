(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-colorpicker-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-color { position: relative; display: inline-flex; align-items: center; gap: .5rem; font: var(--goop-font, inherit); }
      .gc-color-swatch { width: 2rem; height: 2rem; border-radius: var(--goop-radius, 6px); cursor: pointer; border: 1px solid var(--goop-border, #2a3142); flex-shrink: 0; }
      .gc-color-hex {
        box-sizing: border-box; width: 6rem; padding: .4rem .5rem;
        border: 1px solid var(--goop-border, #2a3142); border-radius: var(--goop-radius, 6px);
        background: var(--goop-field, rgba(0,0,0,.25)); color: var(--goop-text, #e6e9ef); font: inherit; font-size: .85rem;
      }
      .gc-color-popup {
        display: none; position: absolute; top: 100%; left: 0; z-index: 9990;
        margin-top: 4px; padding: .5rem;
        background: var(--goop-panel, #151924); border: 1px solid var(--goop-border, #2a3142);
        border-radius: var(--goop-radius, 6px); box-shadow: 0 8px 24px rgba(0,0,0,.3);
      }
      .gc-color-popup[data-goop-open] { display: block; }
      .gc-color-grid { display: grid; grid-template-columns: repeat(8, 1fr); gap: 4px; }
      .gc-color-grid button { width: 1.5rem; height: 1.5rem; border-radius: 4px; border: 2px solid transparent; cursor: pointer; padding: 0; }
      .gc-color-grid button[data-goop-selected] { border-color: var(--goop-text, #e6e9ef); }
      .gc-color-grid button:hover { transform: scale(1.15); }
    `;
    document.head.appendChild(s);
  }

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
    wrap.className = "gc-color";
    wrap.setAttribute("data-goop-component", "colorpicker");
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var swatch = document.createElement("div");
    swatch.className = "gc-color-swatch";

    var hex = document.createElement("input");
    hex.className = "gc-color-hex";
    hex.type = "text"; hex.maxLength = 7;
    if (!showHex) hex.style.display = "none";

    var popup = document.createElement("div");
    popup.className = "gc-color-popup";
    wrap.appendChild(swatch); wrap.appendChild(hex); wrap.appendChild(popup);
    el.appendChild(wrap);

    function setColor(c) { current = c; swatch.style.backgroundColor = c; hex.value = c; }

    function renderGrid() {
      var html = '<div class="gc-color-grid">';
      colors.forEach(function(c) {
        var sel = c.toLowerCase() === current.toLowerCase() ? " data-goop-selected" : "";
        html += '<button type="button" style="background:' + c + '"' + sel + ' data-goop-color="' + _e(c) + '"></button>';
      });
      html += "</div>";
      popup.innerHTML = html;
      popup.querySelectorAll("[data-goop-color]").forEach(function(btn) {
        btn.addEventListener("click", function(e) {
          e.stopPropagation(); setColor(btn.getAttribute("data-goop-color"));
          _f(wrap, "change", { value: current }); _f(wrap, "input", { value: current });
          if (opts.onChange) opts.onChange(current); closePopup();
        });
      });
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
