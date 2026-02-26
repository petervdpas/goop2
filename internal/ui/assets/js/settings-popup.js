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
    var verificationToken = btn.getAttribute('data-verification-token') || '';
    var theme = document.documentElement.getAttribute('data-theme') || 'dark';

    backdrop = el('div', 'ed-dlg-backdrop');
    var dlg = el('div', 'ed-dlg settings-dlg');

    // Head
    var head = el('div', 'ed-dlg-head');
    head.innerHTML = '<span class="ed-dlg-title">Settings</span>';
    var closeBtn = el('button', 'ed-dlg-btn btn-sm');
    closeBtn.textContent = 'Close';
    closeBtn.onclick = closePopup;
    head.appendChild(closeBtn);
    dlg.appendChild(head);

    // Body
    var body = el('div', 'ed-dlg-body');

    // ── Profile section ──
    body.appendChild(sectionLabel('Profile'));

    var labelInput = inputField('Name', label, 'settings-label');
    body.appendChild(labelInput);

    var emailInput = inputField('Email', email, 'settings-email');
    body.appendChild(emailInput);

    var tokenField = passwordField('Verification token (from email)', verificationToken, 'settings-token');
    body.appendChild(tokenField);

    // ── Theme section ──
    body.appendChild(sectionLabel('Appearance'));
    body.appendChild(selectField('Theme', {
      id: 'settings-theme', value: theme,
      options: [{ value: 'dark', label: 'Dark' }, { value: 'light', label: 'Light' }]
    }));

    // ── Devices section (hidden if video_disabled) ──
    var devicesLabel = sectionLabel('Devices');
    devicesLabel.id = 'settings-devices-label';
    var osType = document.body.getAttribute('data-os') || '';
    if (osType === 'linux') {
      devicesLabel.innerHTML += ' <span class="devices-caveat">(may not work on Linux)</span>';
    }
    body.appendChild(devicesLabel);

    var defaultDev = [{ value: '', label: 'System default' }];
    var camField = selectField('Camera', {
      id: 'settings-camera', value: '', placeholder: 'System default', options: defaultDev
    });
    camField.id = 'settings-camera-field';
    body.appendChild(camField);

    var micField = selectField('Microphone', {
      id: 'settings-mic', value: '', placeholder: 'System default', options: defaultDev
    });
    micField.id = 'settings-mic-field';
    body.appendChild(micField);


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
      verificationTokenInput: tokenField.querySelector('input'),
      themeEl: themeEl,
      camEl: camEl,
      micEl: micEl
    };
  }

  function sectionLabel(text) {
    var lbl = el('div', 'popup-section-label');
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

  function passwordField(placeholder, value, id) {
    var wrap = el('div', 'password-wrap');
    var inp = el('input', 'ed-dlg-input');
    inp.type = 'password';
    inp.id = id;
    inp.placeholder = placeholder;
    inp.value = value;
    wrap.appendChild(inp);

    var toggle = el('button', 'password-toggle');
    toggle.type = 'button';
    toggle.title = 'Show/hide token';
    toggle.innerHTML = '&#x1F441;';
    toggle.onclick = function() {
      if (inp.type === 'password') {
        inp.type = 'text';
        toggle.classList.add('visible');
      } else {
        inp.type = 'password';
        toggle.classList.remove('visible');
      }
    };
    wrap.appendChild(toggle);
    return wrap;
  }

  function selectField(labelText, selectOpts) {
    var wrap = el('div', '', '');
    var lbl = el('label', 'small muted field-label');
    lbl.textContent = labelText;
    wrap.appendChild(lbl);
    if (gsel()) wrap.innerHTML += Goop.select.html(selectOpts);
    return wrap;
  }

  // ── device enumeration ────────────────────────────────────────────────────

  // Request brief media permission so enumerateDevices returns real IDs/labels.
  function ensureDevicePermission() {
    return navigator.mediaDevices.enumerateDevices().then(function(devices) {
      var hasLabels = devices.some(function(d) { return !!d.label; });
      if (hasLabels) return; // permission already granted
      // Briefly grab media to trigger the permission prompt
      return navigator.mediaDevices.getUserMedia({ audio: true, video: true })
        .then(function(stream) { stream.getTracks().forEach(function(t) { t.stop(); }); })
        .catch(function() { /* user denied — we'll just show System default */ });
    });
  }

  function enumerateDevices(camEl, micEl) {
    if (!gsel()) return;

    // First fetch settings to check if video is disabled
    Goop.api.settings.get().catch(function() { return {}; })
      .then(function(cfg) {
        // Disable devices section if video is disabled
        if (cfg.video_disabled) {
          var camField = document.getElementById('settings-camera-field');
          var micField = document.getElementById('settings-mic-field');
          if (camField) { camField.classList.add('dimmed'); camField.inert = true; }
          if (micField) { micField.classList.add('dimmed'); micField.inert = true; }
          return;
        }

        if (!navigator.mediaDevices || !navigator.mediaDevices.enumerateDevices) return;

        // Ensure permission first, then enumerate with full labels
        return ensureDevicePermission().then(function() {
          return navigator.mediaDevices.enumerateDevices();
        }).then(function(devices) {
          var camPref = cfg.preferred_cam || '';
          var micPref = cfg.preferred_mic || '';

          var camOpts = [{ value: '', label: 'System default' }];
          var micOpts = [{ value: '', label: 'System default' }];

          devices.forEach(function(dev) {
            if (!dev.deviceId) return; // skip phantom entries
            var lbl = dev.label || (dev.kind + ' ' + dev.deviceId.substring(0, 8));
            if (dev.kind === 'videoinput') {
              camOpts.push({ value: dev.deviceId, label: lbl });
            } else if (dev.kind === 'audioinput') {
              micOpts.push({ value: dev.deviceId, label: lbl });
            }
          });

          if (camEl) Goop.select.setOpts(camEl, { options: camOpts }, camPref);
          if (micEl) Goop.select.setOpts(micEl, { options: micOpts }, micPref);
        });
      }).catch(function(err) {
        Goop.log.warn('settings', 'cannot enumerate devices: ' + err);
      });
  }

  // ── save ────────────────────────────────────────────────────────────────────

  function save() {
    if (!backdrop || !backdrop._refs) return;
    var refs = backdrop._refs;
    var gs = gsel();

    var labelVal = (refs.labelInput.value || '').trim();
    var emailVal = (refs.emailInput.value || '').trim();
    var tokenVal = (refs.verificationTokenInput.value || '').trim();

    // Determine selected theme
    var themeVal = (gs && refs.themeEl) ? Goop.select.val(refs.themeEl) : (document.documentElement.getAttribute('data-theme') || 'dark');

    // Get device selections
    var camVal = (gs && refs.camEl) ? Goop.select.val(refs.camEl) : '';
    var micVal = (gs && refs.micEl) ? Goop.select.val(refs.micEl) : '';

    var payload = {
      label: labelVal,
      email: emailVal,
      verification_token: tokenVal,
      theme: themeVal,
      preferred_cam: camVal,
      preferred_mic: micVal
    };

    // Save all to peer config — close popup only after success
    Goop.api.settings.save(payload).then(function() {

      // Update navbar name
      var meLabel = document.querySelector('.me-label');
      if (meLabel) meLabel.textContent = labelVal || 'Me';

      // Update the cog data attrs for next open
      btn.setAttribute('data-label', labelVal);
      btn.setAttribute('data-email', emailVal);
      btn.setAttribute('data-verification-token', tokenVal);

      closePopup();
    }).catch(function(err) {
      Goop.log.error('settings', 'save failed: ' + err);
      alert('Failed to save settings: ' + err.message);
    });
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
