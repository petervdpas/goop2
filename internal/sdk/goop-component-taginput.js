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
    var tagClass = opts.tagClass || "";
    var suggestionsClass = opts.suggestionsClass || "";
    var disabledAttr = opts.disabledAttr || "";

    var input = opts.input ? el.querySelector(opts.input) : el.querySelector("input");
    var sugBox = null;

    function renderTags() {
      el.querySelectorAll("[data-tag]").forEach(function(t) { t.remove(); });
      tags.forEach(function(tag, idx) {
        var span = document.createElement("span");
        if (tagClass) span.className = tagClass;
        span.setAttribute("data-tag", idx);
        span.textContent = tag;
        var rm = document.createElement("button");
        rm.type = "button";
        rm.textContent = "\u00D7";
        rm.addEventListener("click", function(e) {
          e.stopPropagation();
          tags.splice(idx, 1);
          renderTags(); emitChange();
        });
        span.appendChild(rm);
        if (input) el.insertBefore(span, input);
        else el.appendChild(span);
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
      _f(el, "change", { value: tags.slice() }); _f(el, "input", { value: tags.slice() });
      if (opts.onChange) opts.onChange(tags.slice());
    }

    function showSuggestions(filter) {
      hideSuggestions();
      if (!suggestions.length || !filter) return;
      var lf = filter.toLowerCase();
      var matches = suggestions.filter(function(s) { return s.toLowerCase().indexOf(lf) >= 0 && tags.indexOf(s) < 0; });
      if (!matches.length) return;
      sugBox = document.createElement("div");
      if (suggestionsClass) sugBox.className = suggestionsClass;
      matches.slice(0, 8).forEach(function(m) {
        var div = document.createElement("div");
        div.textContent = m;
        div.addEventListener("mousedown", function(e) { e.preventDefault(); addTag(m); if (input) input.value = ""; hideSuggestions(); });
        sugBox.appendChild(div);
      });
      el.appendChild(sugBox);
    }

    function hideSuggestions() { if (sugBox) { sugBox.remove(); sugBox = null; } }

    if (input) {
      input.addEventListener("keydown", function(e) {
        if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addTag(input.value); input.value = ""; hideSuggestions(); }
        if (e.key === "Backspace" && input.value === "" && tags.length > 0) { tags.pop(); renderTags(); emitChange(); }
        if (e.key === "Escape") hideSuggestions();
      });
      input.addEventListener("input", function() { if (suggestions.length) showSuggestions(input.value); });
      input.addEventListener("blur", function() { setTimeout(function() { hideSuggestions(); if (input.value.trim()) { addTag(input.value); input.value = ""; } }, 150); });
    }

    el.addEventListener("click", function() { if (input) input.focus(); });
    renderTags();

    return {
      getValue: function() { return tags.slice(); },
      setValue: function(v) { tags = (v || []).slice(); renderTags(); },
      addTag: addTag,
      setSuggestions: function(s) { suggestions = s || []; },
      setDisabled: function(v) {
        isDisabled = !!v;
        if (input) input.disabled = isDisabled;
        if (disabledAttr) { if (isDisabled) el.setAttribute(disabledAttr, ""); else el.removeAttribute(disabledAttr); }
      },
      destroy: function() {},
      el: el,
    };
  };
})();
