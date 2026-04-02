//
// CSS hooks:
//   .gc-accordion           — wrapper (override via opts.class)
//   .gc-accordion-section   — each section (override via opts.sectionClass)
//   .gc-accordion-header    — clickable header (override via opts.headerClass)
//   .gc-accordion-chevron   — expand/collapse icon
//   .gc-accordion-body      — collapsible content (override via opts.bodyClass)
//   .open                   — expanded section (override via opts.openClass)
//

(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.accordion = function(opts) {
    opts = opts || {};
    var sections = opts.sections || [];
    var multi = opts.multi !== false;
    var openClass = opts.openClass || "open";
    var openIds = {};
    if (opts.open) (Array.isArray(opts.open) ? opts.open : [opts.open]).forEach(function(id) { openIds[id] = true; });

    var wrap = document.createElement("div");
    for (var _k in opts) { if (_k.indexOf("data-") === 0) wrap.setAttribute(_k, opts[_k]); }
    wrap.className = opts.class || "gc-accordion";

    function render() {
      wrap.innerHTML = "";
      sections.forEach(function(sec) {
        var section = document.createElement("div");
        section.className = opts.sectionClass || "gc-accordion-section";
        section.id = "gc-acc-" + sec.id;
        if (openIds[sec.id]) section.classList.add(openClass);

        var header = document.createElement("button"); header.type = "button";
        header.className = opts.headerClass || "gc-accordion-header";
        header.innerHTML = '<span>' + _e(sec.label) + '</span><span class="gc-accordion-chevron">\u25B6</span>';

        var body = document.createElement("div");
        body.className = opts.bodyClass || "gc-accordion-body";
        if (sec.content) body.innerHTML = sec.content;

        header.addEventListener("click", function() {
          if (openIds[sec.id]) {
            delete openIds[sec.id]; section.classList.remove(openClass);
          } else {
            if (!multi) { openIds = {}; wrap.querySelectorAll("." + (opts.sectionClass || "gc-accordion-section")).forEach(function(s) { s.classList.remove(openClass); }); }
            openIds[sec.id] = true; section.classList.add(openClass);
          }
          _f(wrap, "change", { value: Object.keys(openIds) });
          if (opts.onChange) opts.onChange(Object.keys(openIds));
        });

        section.appendChild(header); section.appendChild(body);
        wrap.appendChild(section);
      });
    }

    render();

    return {
      getValue: function() { return Object.keys(openIds); },
      getSection: function(id) { return wrap.querySelector("#gc-acc-" + id + " ." + (opts.bodyClass || "gc-accordion-body")); },
      open: function(id) { openIds[id] = true; var sec = wrap.querySelector("#gc-acc-" + id); if (sec) sec.classList.add(openClass); },
      close: function(id) { delete openIds[id]; var sec = wrap.querySelector("#gc-acc-" + id); if (sec) sec.classList.remove(openClass); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
