// Shared custom select dropdown component.
// Exposes Goop.select = { html, init, val, setVal, setOpts }
(() => {
  window.Goop = window.Goop || {};

  var esc = Goop.core.escapeHtml;

  // Build HTML string for a custom select.
  //
  //   opts.id          - element id
  //   opts.name        - form field name (adds hidden input for <form> POST)
  //   opts.value       - initial selected value
  //   opts.placeholder - text shown when nothing selected
  //   opts.className   - extra CSS class(es) on wrapper
  //   opts.style       - inline style on wrapper
  //   opts.options     - flat list: [{value, label, disabled}]
  //   opts.groups      - grouped:  [{label, items: [{value, label, disabled}]}]
  //
  function html(opts) {
    opts = opts || {};
    var id = opts.id ? ' id="' + esc(opts.id) + '"' : "";
    var cls = "gsel" + (opts.className ? " " + opts.className : "");
    var sty = opts.style ? ' style="' + esc(opts.style) + '"' : "";
    var val = opts.value || "";
    var placeholder = opts.placeholder || "";

    // Collect all options to find selected label
    var all = [];
    if (opts.groups) {
      opts.groups.forEach(function(g) {
        (g.items || []).forEach(function(o) { all.push(o); });
      });
    } else if (opts.options) {
      all = opts.options;
    }
    var selLabel = placeholder;
    all.forEach(function(o) { if (o.value === val) selLabel = o.label; });

    var h = '<div' + id + ' class="' + cls + '" data-value="' + esc(val) + '"' +
            ' data-placeholder="' + esc(placeholder) + '"' + sty + '>';

    if (opts.name) {
      h += '<input type="hidden" name="' + esc(opts.name) + '" value="' + esc(val) + '">';
    }

    h += '<button type="button" class="gsel-trigger">';
    h += '<span class="gsel-text">' + esc(selLabel) + '</span>';
    h += '<span class="gsel-arrow">&#9662;</span>';
    h += '</button><div class="gsel-dropdown">';

    h += buildOpts(opts, val);

    h += '</div></div>';
    return h;
  }

  function buildOpts(opts, val) {
    var h = "";
    if (opts.groups) {
      opts.groups.forEach(function(g) {
        h += '<div class="gsel-group-label">' + esc(g.label) + '</div>';
        (g.items || []).forEach(function(o) {
          h += optHtml(o, val);
        });
      });
    } else if (opts.options) {
      opts.options.forEach(function(o) {
        h += optHtml(o, val);
      });
    }
    return h;
  }

  function optHtml(o, val) {
    var sel = o.value === val ? " selected" : "";
    var dis = o.disabled ? " disabled" : "";
    return '<button type="button" class="gsel-option' + sel + '"' + dis +
           ' data-value="' + esc(o.value) + '">' + esc(o.label) + '</button>';
  }

  // Cap dropdown height so it doesn't overflow the viewport.
  function fitDropdown(el) {
    var dd = el.querySelector(".gsel-dropdown");
    if (!dd) return;
    dd.style.maxHeight = "";  // reset to CSS default
    var rect = dd.getBoundingClientRect();
    var space = window.innerHeight - rect.top - 8; // 8px margin
    if (space < rect.height && space > 60) {
      dd.style.maxHeight = space + "px";
    }
  }

  // Initialize a custom select element (bind events).
  // onChange(newValue) is called when the user picks an option.
  function init(el, onChange) {
    if (!el || el._gselInit) return;
    el._gselInit = true;

    var trigger = el.querySelector(".gsel-trigger");

    trigger.addEventListener("click", function(e) {
      e.stopPropagation();
      // Close other open selects first
      document.querySelectorAll(".gsel.open").forEach(function(other) {
        if (other !== el) other.classList.remove("open");
      });
      el.classList.toggle("open");
      if (el.classList.contains("open")) fitDropdown(el);
    });

    el.addEventListener("click", function(e) {
      var opt = e.target.closest ? e.target.closest(".gsel-option") : null;
      if (!opt || opt.disabled) return;
      e.stopPropagation();

      var newVal = opt.getAttribute("data-value");
      applyVal(el, newVal, opt.textContent);

      el.querySelectorAll(".gsel-option").forEach(function(o) {
        o.classList.toggle("selected", o === opt);
      });

      el.classList.remove("open");
      if (onChange) onChange(newVal);
    });

    document.addEventListener("click", function() {
      el.classList.remove("open");
    });
  }

  function applyVal(el, value, label) {
    el.setAttribute("data-value", value);
    var textEl = el.querySelector(".gsel-text");
    if (textEl && label != null) textEl.textContent = label;

    var hidden = el.querySelector('input[type="hidden"]');
    if (hidden) hidden.value = value;
  }

  // Get value
  function val(el) {
    return el ? el.getAttribute("data-value") || "" : "";
  }

  // Set value (picks label from existing options)
  function setVal(el, value) {
    if (!el) return;
    var opt = el.querySelector('.gsel-option[data-value="' + value + '"]');
    var label = opt ? opt.textContent : (el.getAttribute("data-placeholder") || "");

    applyVal(el, value, label);

    el.querySelectorAll(".gsel-option").forEach(function(o) {
      o.classList.toggle("selected", o.getAttribute("data-value") === value);
    });
  }

  // Replace options dynamically.
  //   opts.options  - flat:    [{value, label, disabled}]
  //   opts.groups   - grouped: [{label, items: [{value, label, disabled}]}]
  //   value         - which value to select (optional)
  function setOpts(el, opts, value) {
    if (!el) return;
    opts = opts || {};
    var dropdown = el.querySelector(".gsel-dropdown");
    if (!dropdown) return;

    value = value || "";
    dropdown.innerHTML = buildOpts(opts, value);

    // Determine display text
    var all = [];
    if (opts.groups) {
      opts.groups.forEach(function(g) {
        (g.items || []).forEach(function(o) { all.push(o); });
      });
    } else if (opts.options) {
      all = opts.options;
    }
    var placeholder = el.getAttribute("data-placeholder") || "";
    var label = placeholder;
    all.forEach(function(o) { if (o.value === value) label = o.label; });

    applyVal(el, value, label);
  }

  window.Goop.select = {
    html:    html,
    init:    init,
    val:     val,
    setVal:  setVal,
    setOpts: setOpts,
  };

  // Auto-init all static .gsel elements once the DOM is ready.
  // Dynamically created selects must call init() manually.
  function autoInit() {
    document.querySelectorAll(".gsel").forEach(function(el) { init(el); });
  }
  Goop.core.onReady(autoInit);
})();
