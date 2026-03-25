// Reusable toggle switch component.
// Exposes Goop.toggle_switch = { html, init, val, setVal, onChange }
//
// Supports multiple toggle types via opts or data attributes:
//
//   type: "default"   — standard flip switch (label.switch > input + span.slider)
//   type: "theme"     — theme toggle (same markup, different semantic)
//   type: "filter"    — filter toggle (e.g. show favorites)
//
// Server-rendered switches use the {{toggle}} Go template helper.
// JS-generated switches use toggle_switch.html().
// Both produce the same <label class="switch"> markup.
//
// Usage:
//   var t = Goop.toggle_switch;
//
//   // Generate from JS:
//   t.html({ id: "my-toggle", label: "Enable X", checked: true })
//   t.html({ id: "theme", type: "theme", label: "Theme" })
//   t.html({ id: "favs", type: "filter", label: "Show favorites" })
//
//   // Read/write:
//   t.val("my-toggle")              // true/false
//   t.setVal("my-toggle", false)
//
//   // Listen:
//   t.onChange("my-toggle", function(on) { ... })
//
//   // Init a container (returns checkbox elements):
//   var inputs = t.init(container)
//
(() => {
  window.Goop = window.Goop || {};

  var esc = Goop.core.escapeHtml;
  var qs = Goop.core.qs;

  function html(opts) {
    opts = opts || {};
    var id = opts.id || "";
    var name = opts.name || "";
    var type = opts.type || "default";
    var title = opts.title ? ' title="' + esc(opts.title) + '"' : '';
    var checked = opts.checked ? ' checked' : '';
    var ariaLabel = opts.ariaLabel ? ' aria-label="' + esc(opts.ariaLabel) + '"' : '';
    var style = opts.style ? ' style="' + esc(opts.style) + '"' : '';
    var extraClass = opts.className ? ' ' + opts.className : '';

    var h = '<label class="switch' + extraClass + '"' +
      title + style +
      ' data-type="' + esc(type) + '">' +
      '<input type="checkbox"' +
        (id ? ' id="' + esc(id) + '"' : '') +
        (name ? ' name="' + esc(name) + '"' : '') +
        ariaLabel + checked + '>' +
      '<span class="slider"></span>' +
    '</label>';

    if (opts.label) {
      h += '<span class="control-label">' + esc(opts.label) + '</span>';
    }
    return h;
  }

  function resolve(idOrEl) {
    var el = typeof idOrEl === 'string' ? qs('#' + idOrEl) : idOrEl;
    if (!el) return null;
    if (el.type === 'checkbox') return el;
    return el.querySelector('input[type="checkbox"]');
  }

  function val(idOrEl) {
    var cb = resolve(idOrEl);
    return cb ? cb.checked : false;
  }

  function setVal(idOrEl, checked) {
    var cb = resolve(idOrEl);
    if (cb) cb.checked = checked;
  }

  function onChange(idOrEl, fn) {
    var cb = resolve(idOrEl);
    if (cb) cb.addEventListener('change', function() { fn(cb.checked); });
  }

  function init(container) {
    container = container || document;
    var inputs = [];
    container.querySelectorAll('label.switch input[type="checkbox"]').forEach(function(cb) {
      if (!cb._switchInit) {
        cb._switchInit = true;
        inputs.push(cb);
      }
    });
    return inputs;
  }

  window.Goop.toggle_switch = {
    html: html,
    init: init,
    val: val,
    setVal: setVal,
    onChange: onChange,
  };
})();
