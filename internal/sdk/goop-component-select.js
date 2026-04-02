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

    var trigger = opts.trigger ? el.querySelector(opts.trigger) : el.firstElementChild;
    var dropdown = opts.dropdown ? el.querySelector(opts.dropdown) : el.lastElementChild;
    var openAttr = opts.openAttr || "";
    var openClass = opts.openClass || "";
    var selectedAttr = opts.selectedAttr || "";
    var selectedClass = opts.selectedClass || "";
    var disabledAttr = opts.disabledAttr || "";
    var placeholderClass = opts.placeholderClass || "";
    var tagClass = opts.tagClass || "";
    var clearClass = opts.clearClass || "";
    var arrowClass = opts.arrowClass || "";
    var searchClass = opts.searchClass || "";
    var optionClass = opts.optionClass || "";
    var emptyClass = opts.emptyClass || "";

    function renderTrigger() {
      if (!trigger) return;
      trigger.innerHTML = "";

      if (selected.length === 0) {
        var ph = document.createElement("span");
        if (placeholderClass) ph.className = placeholderClass;
        ph.textContent = opts.placeholder || "Select...";
        trigger.appendChild(ph);
      } else if (multi) {
        selected.forEach(function(v) {
          var lbl = v, opt = options.find(function(o) { return o.value === v; });
          if (opt) lbl = opt.label;
          var span = document.createElement("span");
          if (tagClass) span.className = tagClass;
          span.textContent = lbl;
          var rm = document.createElement("button");
          rm.type = "button";
          rm.textContent = "\u00D7";
          rm.addEventListener("click", function(e) {
            e.stopPropagation();
            selected = selected.filter(function(s) { return s !== v; });
            renderTrigger(); renderDropdown(); emitChange();
          });
          span.appendChild(rm);
          trigger.appendChild(span);
        });
      } else {
        var opt = options.find(function(o) { return o.value === selected[0]; });
        var span = document.createElement("span");
        span.textContent = opt ? opt.label : selected[0];
        trigger.appendChild(span);
      }

      if (clearable && selected.length > 0 && !multi) {
        var cb = document.createElement("button");
        cb.type = "button";
        if (clearClass) cb.className = clearClass;
        cb.textContent = "\u00D7";
        cb.addEventListener("click", function(e) { e.stopPropagation(); selected = []; renderTrigger(); emitChange(); });
        trigger.appendChild(cb);
      }

      if (arrowClass) {
        var arrow = document.createElement("span");
        arrow.className = arrowClass;
        arrow.textContent = "\u25BC";
        trigger.appendChild(arrow);
      }
    }

    function renderDropdown(filter) {
      if (!dropdown) return;
      dropdown.innerHTML = "";

      if (searchable) {
        var si = document.createElement("input");
        si.type = "text";
        if (searchClass) si.className = searchClass;
        si.placeholder = "Search...";
        si.value = filter || "";
        si.addEventListener("input", function() { renderDropdown(si.value); });
        si.addEventListener("click", function(e) { e.stopPropagation(); });
        dropdown.appendChild(si);
        if (filter != null) setTimeout(function() { si.focus(); }, 0);
      }

      var filtered = options;
      if (filter) { var lf = filter.toLowerCase(); filtered = options.filter(function(o) { return o.label.toLowerCase().indexOf(lf) >= 0; }); }

      if (filtered.length === 0) {
        var empty = document.createElement("div");
        if (emptyClass) empty.className = emptyClass;
        empty.textContent = "No results";
        dropdown.appendChild(empty);
      } else {
        filtered.forEach(function(o) {
          var opt = document.createElement("div");
          if (optionClass) opt.className = optionClass;
          if (selected.indexOf(o.value) >= 0) {
            if (selectedClass) opt.classList.add(selectedClass);
            if (selectedAttr) opt.setAttribute(selectedAttr, "");
          }
          opt.textContent = o.label;
          opt.addEventListener("click", function(e) {
            e.stopPropagation();
            if (multi) {
              var idx = selected.indexOf(o.value);
              if (idx >= 0) selected.splice(idx, 1); else selected.push(o.value);
              renderTrigger(); renderDropdown(searchable ? (dropdown.querySelector("input") || {}).value : "");
            } else {
              selected = [o.value]; renderTrigger(); closeDropdown();
            }
            emitChange();
          });
          dropdown.appendChild(opt);
        });
      }
    }

    function emitChange() {
      var val = multi ? selected.slice() : (selected[0] || "");
      _f(el, "change", { value: val }); _f(el, "input", { value: val });
      if (opts.onChange) opts.onChange(val);
    }

    function openDropdown() {
      if (isDisabled) return;
      renderDropdown();
      if (openClass) el.classList.add(openClass);
      if (openAttr) el.setAttribute(openAttr, "");
    }

    function closeDropdown() {
      if (openClass) el.classList.remove(openClass);
      if (openAttr) el.removeAttribute(openAttr);
    }

    if (trigger) {
      trigger.addEventListener("click", function(e) {
        e.stopPropagation();
        var isOpen = (openClass && el.classList.contains(openClass)) || (openAttr && el.hasAttribute(openAttr));
        if (isOpen) closeDropdown(); else openDropdown();
      });
    }

    function onDocClick(e) { if (!el.contains(e.target)) closeDropdown(); }
    document.addEventListener("click", onDocClick);
    renderTrigger();

    return {
      getValue: function() { return multi ? selected.slice() : (selected[0] || ""); },
      setValue: function(v) { selected = Array.isArray(v) ? v.slice() : (v != null ? [v] : []); renderTrigger(); },
      setOptions: function(no) { options = (no || []).map(function(o) { return typeof o === "string" ? { value: o, label: o } : o; }); renderTrigger(); },
      setDisabled: function(v) {
        isDisabled = !!v;
        if (disabledAttr) { if (isDisabled) el.setAttribute(disabledAttr, ""); else el.removeAttribute(disabledAttr); }
        if (trigger) trigger.tabIndex = isDisabled ? -1 : 0;
      },
      destroy: function() { document.removeEventListener("click", onDocClick); },
      el: el,
    };
  };
})();
