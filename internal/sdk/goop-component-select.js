(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.select = function(el, opts) {
    opts = opts || {};
    var multi = !!opts.multi;
    var searchable = opts.searchable !== false;
    var clearable = !!opts.clearable;
    var isDisabled = !!opts.disabled;
    var options = (opts.options || []).map(function(o) { return typeof o === "string" ? { value: o, label: o } : o; });
    var selected = [];
    if (opts.value != null) selected = Array.isArray(opts.value) ? opts.value.slice() : [opts.value];

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-select";
    wrap.setAttribute("data-goop-component", "select");
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var trigger = document.createElement("div");
    trigger.className = "gc-select-trigger";
    trigger.tabIndex = isDisabled ? -1 : 0;

    var dropdown = document.createElement("div");
    dropdown.className = "gc-select-dropdown";
    wrap.appendChild(trigger);
    wrap.appendChild(dropdown);
    el.appendChild(wrap);

    function renderTrigger() {
      var html = "";
      if (selected.length === 0) {
        html = '<span class="gc-select-placeholder">' + _e(opts.placeholder || "Select...") + "</span>";
      } else if (multi) {
        selected.forEach(function(v) {
          var lbl = v, opt = options.find(function(o) { return o.value === v; });
          if (opt) lbl = opt.label;
          html += '<span class="gc-select-tag">' + _e(lbl) + '<button type="button" data-goop-remove="' + _e(v) + '">\u00D7</button></span>';
        });
      } else {
        var opt = options.find(function(o) { return o.value === selected[0]; });
        html = '<span>' + _e(opt ? opt.label : selected[0]) + "</span>";
      }
      if (clearable && selected.length > 0 && !multi) html += '<button type="button" class="gc-select-clear">\u00D7</button>';
      html += '<span class="gc-select-arrow">\u25BC</span>';
      trigger.innerHTML = html;

      var cb = trigger.querySelector(".gc-select-clear");
      if (cb) cb.addEventListener("click", function(e) { e.stopPropagation(); selected = []; renderTrigger(); emitChange(); });

      trigger.querySelectorAll("[data-goop-remove]").forEach(function(btn) {
        btn.addEventListener("click", function(e) {
          e.stopPropagation();
          selected = selected.filter(function(s) { return s !== btn.getAttribute("data-goop-remove"); });
          renderTrigger(); renderDropdown(); emitChange();
        });
      });
    }

    function renderDropdown(filter) {
      var html = "";
      if (searchable) html += '<input class="gc-select-search" type="text" placeholder="Search..." value="' + _e(filter || "") + '">';
      var filtered = options;
      if (filter) { var lf = filter.toLowerCase(); filtered = options.filter(function(o) { return o.label.toLowerCase().indexOf(lf) >= 0; }); }
      if (filtered.length === 0) { html += '<div class="gc-select-empty">No results</div>'; }
      else { filtered.forEach(function(o) { html += '<div class="gc-select-option" data-goop-value="' + _e(o.value) + '"' + (selected.indexOf(o.value) >= 0 ? ' data-goop-selected' : '') + '>' + _e(o.label) + "</div>"; }); }
      dropdown.innerHTML = html;

      if (searchable) {
        var si = dropdown.querySelector(".gc-select-search");
        si.addEventListener("input", function() { renderDropdown(si.value); });
        si.addEventListener("click", function(e) { e.stopPropagation(); });
        if (filter != null) si.focus();
      }

      dropdown.querySelectorAll(".gc-select-option").forEach(function(opt) {
        opt.addEventListener("click", function(e) {
          e.stopPropagation();
          var v = opt.getAttribute("data-goop-value");
          if (multi) { var idx = selected.indexOf(v); if (idx >= 0) selected.splice(idx, 1); else selected.push(v); renderTrigger(); renderDropdown(searchable ? (dropdown.querySelector(".gc-select-search") || {}).value : ""); }
          else { selected = [v]; renderTrigger(); closeDropdown(); }
          emitChange();
        });
      });
    }

    function emitChange() { var val = multi ? selected.slice() : (selected[0] || ""); _f(wrap, "change", { value: val }); _f(wrap, "input", { value: val }); if (opts.onChange) opts.onChange(val); }
    function openDropdown() { if (isDisabled) return; renderDropdown(); dropdown.setAttribute("data-goop-open", ""); }
    function closeDropdown() { dropdown.removeAttribute("data-goop-open"); }
    trigger.addEventListener("click", function(e) { e.stopPropagation(); if (dropdown.hasAttribute("data-goop-open")) closeDropdown(); else openDropdown(); });
    function onDocClick(e) { if (!wrap.contains(e.target)) closeDropdown(); }
    document.addEventListener("click", onDocClick);
    renderTrigger();

    return {
      getValue: function() { return multi ? selected.slice() : (selected[0] || ""); },
      setValue: function(v) { selected = Array.isArray(v) ? v.slice() : (v != null ? [v] : []); renderTrigger(); },
      setOptions: function(no) { options = (no || []).map(function(o) { return typeof o === "string" ? { value: o, label: o } : o; }); renderTrigger(); },
      setDisabled: function(v) { isDisabled = !!v; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); trigger.tabIndex = isDisabled ? -1 : 0; },
      destroy: function() { document.removeEventListener("click", onDocClick); wrap.remove(); },
      el: wrap,
    };
  };
})();
