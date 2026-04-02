(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.colorpicker = function(el, opts) {
    opts = opts || {};
    var colors = opts.colors || [
      "#f87171","#fb923c","#fbbf24","#a3e635","#34d399","#22d3ee","#60a5fa","#a78bfa",
      "#f472b6","#e879f9","#c084fc","#818cf8","#38bdf8","#2dd4bf","#4ade80","#facc15",
      "#ef4444","#f97316","#eab308","#84cc16","#10b981","#06b6d4","#3b82f6","#8b5cf6",
    ];
    var current = opts.value || colors[0];
    var isDisabled = !!opts.disabled;

    var swatch = opts.swatch ? el.querySelector(opts.swatch) : null;
    var hex = opts.hex ? el.querySelector(opts.hex) : null;
    var popup = opts.popup ? el.querySelector(opts.popup) : null;
    var grid = opts.grid ? el.querySelector(opts.grid) : null;
    var openAttr = opts.openAttr || "";
    var openClass = opts.openClass || "";
    var selectedAttr = opts.selectedAttr || "";
    var colorAttr = opts.colorAttr || "data-color";
    var colorVar = opts.colorVar || "--gc-color";
    var disabledAttr = opts.disabledAttr || "";
    var hiddenAttr = opts.hiddenAttr || "";
    var buttonClass = opts.buttonClass || "";

    function setColor(c) {
      current = c;
      if (swatch) swatch.style.setProperty(colorVar, c);
      if (hex) hex.value = c;
    }

    function renderGrid() {
      var target = grid || popup;
      if (!target) return;
      target.innerHTML = "";
      colors.forEach(function(c) {
        var btn = document.createElement("button");
        btn.type = "button";
        if (buttonClass) btn.className = buttonClass;
        btn.setAttribute(colorAttr, c);
        btn.style.setProperty(colorVar, c);
        if (c.toLowerCase() === current.toLowerCase() && selectedAttr) btn.setAttribute(selectedAttr, "");
        btn.addEventListener("click", function(e) {
          e.stopPropagation(); setColor(c);
          _f(el, "change", { value: current }); _f(el, "input", { value: current });
          if (opts.onChange) opts.onChange(current); closePopup();
        });
        target.appendChild(btn);
      });
    }

    function openPopup() {
      if (isDisabled) return;
      renderGrid();
      if (popup) {
        if (openClass) popup.classList.add(openClass);
        if (openAttr) popup.setAttribute(openAttr, "");
      }
    }

    function closePopup() {
      if (popup) {
        if (openClass) popup.classList.remove(openClass);
        if (openAttr) popup.removeAttribute(openAttr);
      }
    }

    if (swatch) {
      swatch.addEventListener("click", function(e) {
        e.stopPropagation();
        var isOpen = popup && ((openClass && popup.classList.contains(openClass)) || (openAttr && popup.hasAttribute(openAttr)));
        if (isOpen) closePopup(); else openPopup();
      });
    }

    if (hex) {
      hex.addEventListener("change", function() {
        var v = hex.value.trim();
        if (/^#[0-9a-fA-F]{3,6}$/.test(v)) {
          setColor(v);
          _f(el, "change", { value: current });
          if (opts.onChange) opts.onChange(current);
        }
      });
      if (hiddenAttr && opts.showHex === false) hex.setAttribute(hiddenAttr, "");
    }

    function onDocClick(e) { if (!el.contains(e.target)) closePopup(); }
    document.addEventListener("click", onDocClick);
    setColor(current);

    return {
      getValue: function() { return current; },
      setValue: function(v) { setColor(v); },
      setDisabled: function(v) {
        isDisabled = !!v;
        if (disabledAttr) { if (isDisabled) el.setAttribute(disabledAttr, ""); else el.removeAttribute(disabledAttr); }
      },
      destroy: function() { document.removeEventListener("click", onDocClick); },
      el: el,
    };
  };
})();
