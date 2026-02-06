// internal/ui/assets/js/98-settings-popup.js
//
// Quick-settings popup launched from the navbar cog icon.
// Sections: Profile (label/email), Appearance (theme), Devices (camera/mic).
// Uses Goop.select (gsel) for all dropdowns.
//
(() => {
  window.Goop = window.Goop || {};

  var btn = document.getElementById('settingsBtn');
  if (!btn) return; // not on a page with the cog

  var isOpen = false;
  var backdrop = null;

  // ── helpers ──────────────────────────────────────────────────────────────────

  function el(tag, cls, html) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (html !== undefined) e.innerHTML = html;
    return e;
  }

  function gsel() { return window.Goop && window.Goop.select; }

  // ── build dialog ────────────────────────────────────────────────────────────

  function buildDialog() {
    var label = btn.getAttribute('data-label') || '';
    var email = btn.getAttribute('data-email') || '';
    var theme = document.documentElement.getAttribute('data-theme') || 'dark';

    backdrop = el('div', 'ed-dlg-backdrop');
    var dlg = el('div', 'ed-dlg');
    dlg.style.width = 'min(440px, 94vw)';

    // Head
    var head = el('div', 'ed-dlg-head');
    head.innerHTML = '<span class="ed-dlg-title">Settings</span>';
    var closeBtn = el('button', 'ed-dlg-btn');
    closeBtn.textContent = 'Close';
    closeBtn.style.padding = '4px 10px';
    closeBtn.style.fontSize = '12px';
    closeBtn.onclick = closePopup;
    head.appendChild(closeBtn);
    dlg.appendChild(head);

    // Body
    var body = el('div', 'ed-dlg-body');
    body.style.gap = '14px';

    // ── Profile section ──
    body.appendChild(sectionLabel('Profile'));

    var labelInput = inputField('Name', label, 'settings-label');
    body.appendChild(labelInput);

    var emailInput = inputField('Email', email, 'settings-email');
    body.appendChild(emailInput);

    // ── Theme section ──
    body.appendChild(sectionLabel('Appearance'));

    var themeWrap = el('div', '', '');
    var themeLbl = el('label', 'small muted');
    themeLbl.textContent = 'Theme';
    themeLbl.style.display = 'block';
    themeLbl.style.marginBottom = '4px';
    themeWrap.appendChild(themeLbl);

    if (gsel()) {
      themeWrap.innerHTML += Goop.select.html({
        id: 'settings-theme',
        value: theme,
        options: [
          { value: 'dark', label: 'Dark' },
          { value: 'light', label: 'Light' }
        ]
      });
    }
    body.appendChild(themeWrap);

    // ── Devices section ──
    body.appendChild(sectionLabel('Devices'));

    var camWrap = el('div', '', '');
    var camLbl = el('label', 'small muted');
    camLbl.textContent = 'Camera';
    camLbl.style.display = 'block';
    camLbl.style.marginBottom = '4px';
    camWrap.appendChild(camLbl);
    if (gsel()) {
      camWrap.innerHTML += Goop.select.html({
        id: 'settings-camera',
        value: '',
        placeholder: 'System default',
        options: [{ value: '', label: 'System default' }]
      });
    }
    body.appendChild(camWrap);

    var micWrap = el('div', '', '');
    var micLbl = el('label', 'small muted');
    micLbl.textContent = 'Microphone';
    micLbl.style.display = 'block';
    micLbl.style.marginBottom = '4px';
    micWrap.appendChild(micLbl);
    if (gsel()) {
      micWrap.innerHTML += Goop.select.html({
        id: 'settings-mic',
        value: '',
        placeholder: 'System default',
        options: [{ value: '', label: 'System default' }]
      });
    }
    body.appendChild(micWrap);

    dlg.appendChild(body);

    // Footer
    var foot = el('div', 'ed-dlg-foot');
    var saveBtn = el('button', 'ed-dlg-btn');
    saveBtn.textContent = 'Save';
    saveBtn.onclick = function() { save(); };
    foot.appendChild(saveBtn);
    dlg.appendChild(foot);

    backdrop.appendChild(dlg);
    backdrop.addEventListener('click', function(e) {
      if (e.target === backdrop) closePopup();
    });

    document.body.appendChild(backdrop);

    // Initialize gsel dropdowns
    var themeEl = document.getElementById('settings-theme');
    var camEl = document.getElementById('settings-camera');
    var micEl = document.getElementById('settings-mic');

    if (gsel()) {
      Goop.select.init(themeEl, function(val) {
        if (window.Goop && window.Goop.theme) Goop.theme.set(val);
      });
      Goop.select.init(camEl);
      Goop.select.init(micEl);
    }

    // Populate device dropdowns
    enumerateDevices(camEl, micEl);

    // Store references for save
    backdrop._refs = {
      labelInput: labelInput.querySelector('input'),
      emailInput: emailInput.querySelector('input'),
      themeEl: themeEl,
      camEl: camEl,
      micEl: micEl
    };
  }

  function sectionLabel(text) {
    var lbl = el('div', '', '');
    lbl.style.fontWeight = '650';
    lbl.style.fontSize = '13px';
    lbl.style.color = 'var(--muted)';
    lbl.style.textTransform = 'uppercase';
    lbl.style.letterSpacing = '0.6px';
    lbl.textContent = text;
    return lbl;
  }

  function inputField(placeholder, value, id) {
    var wrap = el('div', '', '');
    var inp = el('input', 'ed-dlg-input');
    inp.type = 'text';
    inp.id = id;
    inp.placeholder = placeholder;
    inp.value = value;
    wrap.appendChild(inp);
    return wrap;
  }

  // ── device enumeration ────────────────────────────────────────────────────

  function enumerateDevices(camEl, micEl) {
    if (!navigator.mediaDevices || !navigator.mediaDevices.enumerateDevices) return;
    if (!gsel()) return;

    var camPref = '';
    var micPref = '';
    try { camPref = localStorage.getItem('goop-preferred-camera') || ''; } catch(e) {}
    try { micPref = localStorage.getItem('goop-preferred-mic') || ''; } catch(e) {}

    navigator.mediaDevices.enumerateDevices().then(function(devices) {
      var camOpts = [{ value: '', label: 'System default' }];
      var micOpts = [{ value: '', label: 'System default' }];

      devices.forEach(function(dev) {
        var lbl = dev.label || (dev.kind + ' ' + dev.deviceId.substring(0, 8));
        if (dev.kind === 'videoinput') {
          camOpts.push({ value: dev.deviceId, label: lbl });
        } else if (dev.kind === 'audioinput') {
          micOpts.push({ value: dev.deviceId, label: lbl });
        }
      });

      if (camEl) Goop.select.setOpts(camEl, { options: camOpts }, camPref);
      if (micEl) Goop.select.setOpts(micEl, { options: micOpts }, micPref);
    }).catch(function(err) {
      console.warn('settings: cannot enumerate devices:', err);
    });
  }

  // ── save ────────────────────────────────────────────────────────────────────

  function save() {
    if (!backdrop || !backdrop._refs) return;
    var refs = backdrop._refs;
    var gs = gsel();

    var labelVal = (refs.labelInput.value || '').trim();
    var emailVal = (refs.emailInput.value || '').trim();

    // Determine selected theme
    var themeVal = (gs && refs.themeEl) ? Goop.select.val(refs.themeEl) : (document.documentElement.getAttribute('data-theme') || 'dark');

    // Save profile + theme to server
    fetch('/api/settings/quick', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ label: labelVal, email: emailVal, theme: themeVal })
    }).then(function(res) {
      if (!res.ok) throw new Error('save failed');

      // Update navbar name
      var meLabel = document.querySelector('.me-label');
      if (meLabel) meLabel.textContent = labelVal || 'Me';

      // Update the cog data attrs for next open
      btn.setAttribute('data-label', labelVal);
      btn.setAttribute('data-email', emailVal);
    }).catch(function(err) {
      console.error('settings: save failed:', err);
      alert('Failed to save settings');
      return;
    });

    // Save device preferences to localStorage
    if (gs) {
      var camVal = refs.camEl ? Goop.select.val(refs.camEl) : '';
      var micVal = refs.micEl ? Goop.select.val(refs.micEl) : '';
      try { localStorage.setItem('goop-preferred-camera', camVal); } catch(e) {}
      try { localStorage.setItem('goop-preferred-mic', micVal); } catch(e) {}
    }

    closePopup();
  }

  // ── open/close ──────────────────────────────────────────────────────────────

  function openPopup() {
    if (isOpen) { closePopup(); return; }
    isOpen = true;
    buildDialog();
  }

  function closePopup() {
    isOpen = false;
    if (backdrop) {
      backdrop.remove();
      backdrop = null;
    }
  }

  btn.addEventListener('click', openPopup);

  // ESC to close
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape' && isOpen) closePopup();
  });

  // Public API
  Goop.settings = { open: openPopup, close: closePopup };
})();
