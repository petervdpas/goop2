(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-toggle-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-toggle { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font: var(--goop-font, inherit); }
      .gc-toggle-track { position: relative; width: 2.5rem; height: 1.4rem; border-radius: 999px; background: var(--goop-border, #2a3142); transition: background .2s; }
      .gc-toggle[data-goop-checked] .gc-toggle-track { background: var(--goop-accent, #7aa2ff); }
      .gc-toggle-thumb { position: absolute; top: 2px; left: 2px; width: calc(1.4rem - 4px); height: calc(1.4rem - 4px); border-radius: 50%; background: var(--goop-text, #e6e9ef); transition: left .2s; }
      .gc-toggle[data-goop-checked] .gc-toggle-thumb { left: calc(2.5rem - 1.4rem + 2px); }
      .gc-toggle-label { color: var(--goop-text, #e6e9ef); font-size: .9rem; user-select: none; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.toggle = function(el, opts) {
    opts = opts || {};
    var checked = !!opts.checked;
    var isDisabled = !!opts.disabled;

    var wrap = document.createElement("div");
    wrap.className = "gc-toggle";
    wrap.setAttribute("data-goop-component", "toggle");
    wrap.tabIndex = isDisabled ? -1 : 0;
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    wrap.innerHTML = '<div class="gc-toggle-track"><div class="gc-toggle-thumb"></div></div>' +
      (opts.label ? '<span class="gc-toggle-label">' + _e(opts.label) + "</span>" : "");
    el.appendChild(wrap);

    function update() { if (checked) wrap.setAttribute("data-goop-checked", ""); else wrap.removeAttribute("data-goop-checked"); }
    function toggle() {
      if (isDisabled) return;
      checked = !checked; update();
      _f(wrap, "change", { value: checked }); _f(wrap, "input", { value: checked });
      if (opts.onChange) opts.onChange(checked);
    }

    wrap.addEventListener("click", toggle);
    wrap.addEventListener("keydown", function(e) { if (e.key === " " || e.key === "Enter") { e.preventDefault(); toggle(); } });
    update();

    return {
      getValue: function() { return checked; },
      setValue: function(v) { checked = !!v; update(); },
      setDisabled: function(v) { isDisabled = !!v; wrap.tabIndex = isDisabled ? -1 : 0; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
