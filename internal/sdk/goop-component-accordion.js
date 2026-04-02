(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-accordion-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-accordion { font: var(--goop-font, inherit); }
      .gc-accordion-section { border-bottom: 1px solid var(--goop-border, #2a3142); }
      .gc-accordion-header {
        display: flex; align-items: center; justify-content: space-between;
        padding: .6rem .25rem; cursor: pointer; color: var(--goop-text, #e6e9ef); font-size: .9rem; font-weight: 500;
        background: none; border: none; width: 100%; text-align: left; font: inherit;
      }
      .gc-accordion-header:hover { color: var(--goop-accent, #7aa2ff); }
      .gc-accordion-chevron { font-size: .7rem; color: var(--goop-muted, #9aa3b2); transition: transform .2s; }
      .gc-accordion-section[data-goop-open] .gc-accordion-chevron { transform: rotate(90deg); }
      .gc-accordion-body { display: none; padding: 0 .25rem .6rem; color: var(--goop-text, #e6e9ef); font-size: .9rem; }
      .gc-accordion-section[data-goop-open] .gc-accordion-body { display: block; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.accordion = function(el, opts) {
    opts = opts || {};
    var sections = opts.sections || [];
    var multi = opts.multi !== false;
    var openIds = {};
    if (opts.open) (Array.isArray(opts.open) ? opts.open : [opts.open]).forEach(function(id) { openIds[id] = true; });

    var wrap = document.createElement("div");
    wrap.className = "gc-accordion";
    wrap.setAttribute("data-goop-component", "accordion");
    el.appendChild(wrap);

    function render() {
      wrap.innerHTML = "";
      sections.forEach(function(sec) {
        var section = document.createElement("div");
        section.className = "gc-accordion-section";
        section.id = "gc-acc-" + sec.id;
        if (openIds[sec.id]) section.setAttribute("data-goop-open", "");

        var header = document.createElement("button"); header.type = "button";
        header.className = "gc-accordion-header";
        header.innerHTML = '<span>' + _e(sec.label) + '</span><span class="gc-accordion-chevron">\u25B6</span>';

        var body = document.createElement("div");
        body.className = "gc-accordion-body";
        if (sec.content) body.innerHTML = sec.content;

        header.addEventListener("click", function() {
          if (openIds[sec.id]) { delete openIds[sec.id]; section.removeAttribute("data-goop-open"); }
          else {
            if (!multi) { openIds = {}; wrap.querySelectorAll(".gc-accordion-section").forEach(function(s) { s.removeAttribute("data-goop-open"); }); }
            openIds[sec.id] = true; section.setAttribute("data-goop-open", "");
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
      getSection: function(id) { return wrap.querySelector("#gc-acc-" + id + " .gc-accordion-body"); },
      open: function(id) { openIds[id] = true; var sec = wrap.querySelector("#gc-acc-" + id); if (sec) sec.setAttribute("data-goop-open", ""); },
      close: function(id) { delete openIds[id]; var sec = wrap.querySelector("#gc-acc-" + id); if (sec) sec.removeAttribute("data-goop-open"); },
      destroy: function() { wrap.remove(); },
      el: wrap,
    };
  };
})();
