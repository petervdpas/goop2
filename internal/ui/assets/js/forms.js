// forms.js — Auto-enhances forms with data-saveable attribute.
// Save/Undo button group, always visible, dimmed when inactive.
// One-level undo: save stores the pre-save state, undo reverts to it.
(function() {
  function enhanceForm(form) {
    if (form._saveableInit) return;
    form._saveableInit = true;

    var keyExtra = '';
    var pathInput = form.querySelector('input[name="path"]') || form.querySelector('input[name="name"]');
    if (pathInput) keyExtra = ':' + pathInput.value;
    var guidKey = 'saveable-guid:' + (form.action || location.pathname) + keyExtra;
    var guid = sessionStorage.getItem(guidKey);
    if (!guid) {
      guid = Math.random().toString(36).slice(2) + Date.now().toString(36);
      sessionStorage.setItem(guidKey, guid);
    }
    var storageKey = 'saveable:' + guid;

    var inputs = form.querySelectorAll('input[name], select[name], textarea[name]');

    var cmInstance = null;
    var cmField = null;
    inputs.forEach(function(el) {
      if (el.tagName === 'TEXTAREA' && el.nextElementSibling && el.nextElementSibling.classList.contains('CodeMirror')) {
        cmInstance = el.nextElementSibling.CodeMirror;
        cmField = el;
      }
    });

    function snap() {
      var s = {};
      inputs.forEach(function(el) {
        if (el.type === 'checkbox') {
          s[el.name] = el.checked ? '1' : '0';
        } else if (el.type === 'radio') {
          if (el.checked) s[el.name] = el.value;
        } else if (cmInstance && el === cmField) {
          s[el.name] = cmInstance.getValue();
        } else {
          s[el.name] = el.value;
        }
      });
      return s;
    }

    function apply(s) {
      inputs.forEach(function(el) {
        if (el.type === 'hidden') return;
        if (!(el.name in s)) return;
        if (el.type === 'checkbox') {
          el.checked = s[el.name] === '1';
          el.dispatchEvent(new Event('change', { bubbles: true }));
        } else if (el.type === 'radio') {
          el.checked = el.value === s[el.name];
          var opt = el.closest('.splash-picker-option');
          if (opt) opt.classList.toggle('selected', el.checked);
          if (el.checked) el.dispatchEvent(new Event('change', { bubbles: true }));
        } else if (cmInstance && el === cmField) {
          cmInstance.setValue(s[el.name] || '');
        } else {
          el.value = s[el.name] || '';
        }
      });
    }

    function differs(a, b) {
      for (var k in a) { if (a[k] !== b[k]) return true; }
      for (var k2 in b) { if (a[k2] !== b[k2]) return true; }
      return false;
    }

    var saved = snap();
    var previous = null;
    var didUndo = false;

    // Load previous state from storage (from last save)
    try {
      var raw = sessionStorage.getItem(storageKey);
      if (raw) {
        var parsed = JSON.parse(raw);
        if (differs(parsed, saved)) {
          previous = parsed;
        }
        sessionStorage.removeItem(storageKey);
      }
    } catch(e) {}

    function hasUnsavedChanges() {
      return differs(snap(), saved);
    }

    function canUndo() {
      return hasUnsavedChanges() || previous !== null;
    }

    function revert() {
      if (hasUnsavedChanges()) {
        // Revert unsaved edits back to saved state
        apply(saved);
        updateButtons();
      } else if (previous) {
        // Undo the last save: apply previous state and auto-save it
        apply(previous);
        previous = null;
        didUndo = true;
        sessionStorage.removeItem(storageKey);
        // Submit the form to persist the reverted state
        if (form.requestSubmit) {
          form.requestSubmit();
        } else {
          form.submit();
        }
      }
    }

    // On save: store pre-save state (unless this save is confirming an undo)
    form.addEventListener('submit', function() {
      if (hasUnsavedChanges()) {
        if (!didUndo) {
          try { sessionStorage.setItem(storageKey, JSON.stringify(saved)); } catch(e) {}
        } else {
          sessionStorage.removeItem(storageKey);
        }
      }
    });

    // Build button group
    var saveBtn = form.querySelector('button[type="submit"]');
    if (!saveBtn) return;

    var group = document.createElement('span');
    group.className = 'btn-group';
    saveBtn.parentNode.insertBefore(group, saveBtn);
    group.appendChild(saveBtn);

    var undoBtn = document.createElement('button');
    undoBtn.type = 'button';
    undoBtn.className = 'btn secondary';
    undoBtn.textContent = 'Undo';
    undoBtn.title = 'Revert changes';
    group.appendChild(undoBtn);

    undoBtn.addEventListener('click', revert);

    function updateButtons() {
      var dirty = hasUnsavedChanges();
      var undo = canUndo();
      saveBtn.disabled = !dirty;
      saveBtn.style.opacity = dirty ? '' : '0.4';
      undoBtn.disabled = !undo;
      undoBtn.style.opacity = undo ? '' : '0.4';
    }

    updateButtons();
    form.addEventListener('input', function() { didUndo = false; updateButtons(); });
    form.addEventListener('change', function() { didUndo = false; updateButtons(); });
    if (cmInstance) cmInstance.on('change', function() { didUndo = false; updateButtons(); });
  }

  function init() {
    document.querySelectorAll('form[data-saveable]').forEach(enhanceForm);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function() { setTimeout(init, 0); });
  } else {
    setTimeout(init, 0);
  }
})();
