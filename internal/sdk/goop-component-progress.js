(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  var SID = "gc-progress-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-progress {
        width: 100%; height: .5rem; border-radius: 999px;
        background: var(--goop-field, rgba(0,0,0,.25)); overflow: hidden; font: var(--goop-font, inherit);
      }
      .gc-progress-bar {
        height: 100%; border-radius: 999px; background: var(--goop-accent, #7aa2ff);
        transition: width .3s ease;
      }
      .gc-progress[data-goop-variant="success"] .gc-progress-bar { background: #22c55e; }
      .gc-progress[data-goop-variant="warning"] .gc-progress-bar { background: #eab308; }
      .gc-progress[data-goop-variant="danger"] .gc-progress-bar { background: var(--goop-danger, #f87171); }
      .gc-progress[data-goop-animated] .gc-progress-bar {
        background-image: linear-gradient(45deg, rgba(255,255,255,.15) 25%, transparent 25%, transparent 50%, rgba(255,255,255,.15) 50%, rgba(255,255,255,.15) 75%, transparent 75%);
        background-size: 1rem 1rem;
        animation: gc-progress-stripe .6s linear infinite;
      }
      @keyframes gc-progress-stripe { 0% { background-position: 1rem 0; } 100% { background-position: 0 0; } }
      .gc-progress-label { font-size: .8rem; color: var(--goop-muted, #9aa3b2); margin-top: .2rem; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.progress = function(el, opts) {
    opts = opts || {};
    var value = opts.value || 0;
    var max = opts.max || 100;

    var wrap = document.createElement("div");
    wrap.className = "gc-progress";
    wrap.setAttribute("data-goop-component", "progress");
    if (opts.variant) wrap.setAttribute("data-goop-variant", opts.variant);
    if (opts.animated) wrap.setAttribute("data-goop-animated", "");
    if (opts.height) wrap.style.height = typeof opts.height === "number" ? opts.height + "px" : opts.height;

    var bar = document.createElement("div");
    bar.className = "gc-progress-bar";

    var label = null;
    if (opts.label !== false) {
      label = document.createElement("div");
      label.className = "gc-progress-label";
    }

    wrap.appendChild(bar);
    el.appendChild(wrap);
    if (label) el.appendChild(label);

    function update() {
      var pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
      bar.style.width = pct + "%";
      if (label) {
        label.textContent = typeof opts.format === "function"
          ? opts.format(value, max, pct)
          : Math.round(pct) + "%";
      }
    }

    update();

    return {
      getValue: function() { return value; },
      setValue: function(v, m) { value = v; if (m != null) max = m; update(); },
      destroy: function() { wrap.remove(); if (label) label.remove(); },
      el: wrap,
    };
  };
})();
