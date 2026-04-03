// Reusable collapsible panel component.
// Exposes Goop.panel = { init, initAll }
//
// Programmatic:
//   Goop.panel.init(triggerEl, contentEl, opts)
//   Returns { toggle(), open(), close(), isOpen() }
//
// Declarative:
//   Add class "section-toggle" to any element. Its next sibling becomes
//   the collapsible content. Config via data attributes:
//     data-panel-open="0"       — start collapsed (default: expanded)
//     data-panel-remember="key" — persist state in sessionStorage
//     data-panel-icon="0"       — hide arrow indicator
//
//   Goop.panel.initAll(root) scans a container for .section-toggle
//   elements and initializes them. Called automatically on DOMContentLoaded
//   for static HTML. Call manually after dynamic rendering.
(() => {
  window.Goop = window.Goop || {};

  var on = Goop.core.on;
  var qsa = Goop.core.qsa;
  var onReady = Goop.core.onReady;

  function init(triggerEl, contentEl, opts) {
    if (!triggerEl || !contentEl) return null;
    opts = opts || {};
    var isOpen = !!opts.open;
    if (opts.remember) {
      var stored = sessionStorage.getItem('panel:' + opts.remember);
      if (stored !== null) isOpen = stored === '1';
    }
    var arrow = null;
    if (opts.icon !== false) {
      arrow = document.createElement('span');
      arrow.className = 'panel-arrow';
      arrow.textContent = '\u25B8';
      var titleEl = triggerEl.querySelector('.section-title');
      if (titleEl) {
        titleEl.after(arrow);
      } else {
        triggerEl.appendChild(arrow);
      }
    }
    function apply() {
      contentEl.classList.toggle('hidden', !isOpen);
      if (arrow) arrow.textContent = isOpen ? '\u25BE' : '\u25B8';
      triggerEl.classList.toggle('panel-open', isOpen);
      if (opts.remember) sessionStorage.setItem('panel:' + opts.remember, isOpen ? '1' : '0');
      if (opts.onToggle) opts.onToggle(isOpen);
    }
    apply();
    on(triggerEl, 'click', function() { isOpen = !isOpen; apply(); });
    return {
      toggle: function() { isOpen = !isOpen; apply(); },
      open: function() { isOpen = true; apply(); },
      close: function() { isOpen = false; apply(); },
      isOpen: function() { return isOpen; },
    };
  }

  function initAll(root) {
    qsa('.section-toggle', root || document).forEach(function(trigger) {
      if (trigger._panelInit) return;
      trigger._panelInit = true;
      var content = trigger.nextElementSibling;
      if (!content) return;
      init(trigger, content, {
        open: trigger.getAttribute('data-panel-open') !== '0',
        icon: trigger.getAttribute('data-panel-icon') !== '0',
        remember: trigger.getAttribute('data-panel-remember') || undefined,
      });
    });
  }

  onReady(function() { initAll(); });

  window.Goop.panel = { init: init, initAll: initAll };
})();
