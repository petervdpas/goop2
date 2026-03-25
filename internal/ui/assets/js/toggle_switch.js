// Reusable toggle switch component.
// Exposes Goop.toggle_switch = { html, val, setVal, onChange }
(() => {
  window.Goop = window.Goop || {};

  var esc = Goop.core.escapeHtml;
  var qs = Goop.core.qs;

  // Build HTML string for a toggle switch.
  //
  //   opts.id    - checkbox element id (required)
  //   opts.label - text label shown next to the switch
  //   opts.checked - initial state (boolean)
  //   opts.title - tooltip text
  //
  function html(opts) {
    opts = opts || {};
    var id = opts.id || "";
    var title = opts.title ? ' title="' + esc(opts.title) + '"' : '';
    var checked = opts.checked ? ' checked' : '';

    var h = '<label class="switch"' + title + '>' +
      '<input type="checkbox"' + (id ? ' id="' + esc(id) + '"' : '') + checked + '>' +
      '<span class="slider"></span>' +
    '</label>';

    if (opts.label) {
      h += '<span class="control-label">' + esc(opts.label) + '</span>';
    }
    return h;
  }

  function val(idOrEl) {
    var el = typeof idOrEl === 'string' ? qs('#' + idOrEl) : idOrEl;
    if (!el) return false;
    if (el.type === 'checkbox') return el.checked;
    var input = el.querySelector('input[type="checkbox"]');
    return input ? input.checked : false;
  }

  function setVal(idOrEl, checked) {
    var el = typeof idOrEl === 'string' ? qs('#' + idOrEl) : idOrEl;
    if (!el) return;
    if (el.type === 'checkbox') { el.checked = checked; return; }
    var input = el.querySelector('input[type="checkbox"]');
    if (input) input.checked = checked;
  }

  function onChange(idOrEl, fn) {
    var el = typeof idOrEl === 'string' ? qs('#' + idOrEl) : idOrEl;
    if (!el) return;
    if (el.type !== 'checkbox') {
      el = el.querySelector('input[type="checkbox"]');
    }
    if (el) el.addEventListener('change', function() { fn(el.checked); });
  }

  window.Goop.toggle_switch = {
    html: html,
    val: val,
    setVal: setVal,
    onChange: onChange,
  };
})();
