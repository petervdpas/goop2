(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.taginput = function(el, opts) {
    opts = opts || {};
    var tags = (opts.value || []).slice();
    var allowDuplicates = !!opts.allowDuplicates;
    var isDisabled = !!opts.disabled;
    var suggestions = opts.suggestions || [];

    var outer = document.createElement("div");
    outer.style.position = "relative";

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-taginput";
    wrap.setAttribute("data-goop-component", "taginput");
    if (opts.name) wrap.setAttribute("data-goop-name", opts.name);
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var input = document.createElement("input");
    input.type = "text";
    input.placeholder = opts.placeholder || "Add...";
    if (isDisabled) input.disabled = true;

    var sugBox = null;

    outer.appendChild(wrap);
    el.appendChild(outer);

    function renderTags() {
      wrap.querySelectorAll(".gc-taginput-tag").forEach(function(t) { t.remove(); });
      tags.forEach(function(tag, idx) {
        var span = document.createElement("span");
        span.className = "gc-taginput-tag";
        span.innerHTML = _e(tag) + '<button type="button" data-goop-idx="' + idx + '">\u00D7</button>';
        wrap.insertBefore(span, input);
      });
      wrap.querySelectorAll(".gc-taginput-tag button").forEach(function(btn) {
        btn.addEventListener("click", function(e) {
          e.stopPropagation();
          tags.splice(parseInt(btn.getAttribute("data-goop-idx"), 10), 1);
          renderTags(); emitChange();
        });
      });
    }

    function addTag(text) {
      text = text.trim();
      if (!text) return;
      if (!allowDuplicates && tags.indexOf(text) >= 0) return;
      if (opts.max && tags.length >= opts.max) return;
      tags.push(text); renderTags(); emitChange();
    }

    function emitChange() {
      _f(wrap, "change", { value: tags.slice() }); _f(wrap, "input", { value: tags.slice() });
      if (opts.onChange) opts.onChange(tags.slice());
    }

    function showSuggestions(filter) {
      hideSuggestions();
      if (!suggestions.length || !filter) return;
      var lf = filter.toLowerCase();
      var matches = suggestions.filter(function(s) { return s.toLowerCase().indexOf(lf) >= 0 && tags.indexOf(s) < 0; });
      if (!matches.length) return;
      sugBox = document.createElement("div");
      sugBox.className = "gc-taginput-suggestions";
      matches.slice(0, 8).forEach(function(m) {
        var div = document.createElement("div");
        div.textContent = m;
        div.addEventListener("mousedown", function(e) { e.preventDefault(); addTag(m); input.value = ""; hideSuggestions(); });
        sugBox.appendChild(div);
      });
      outer.appendChild(sugBox);
    }

    function hideSuggestions() { if (sugBox) { sugBox.remove(); sugBox = null; } }

    input.addEventListener("keydown", function(e) {
      if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addTag(input.value); input.value = ""; hideSuggestions(); }
      if (e.key === "Backspace" && input.value === "" && tags.length > 0) { tags.pop(); renderTags(); emitChange(); }
      if (e.key === "Escape") hideSuggestions();
    });

    input.addEventListener("input", function() { if (suggestions.length) showSuggestions(input.value); });
    input.addEventListener("blur", function() { setTimeout(function() { hideSuggestions(); if (input.value.trim()) { addTag(input.value); input.value = ""; } }, 150); });

    wrap.addEventListener("click", function() { input.focus(); });
    wrap.appendChild(input);
    renderTags();

    return {
      getValue: function() { return tags.slice(); },
      setValue: function(v) { tags = (v || []).slice(); renderTags(); },
      addTag: addTag,
      setSuggestions: function(s) { suggestions = s || []; },
      setDisabled: function(v) { isDisabled = !!v; input.disabled = isDisabled; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); },
      destroy: function() { outer.remove(); },
      el: wrap,
    };
  };
})();
