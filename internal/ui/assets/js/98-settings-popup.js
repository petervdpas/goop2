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
    var dlg = el('div', 'ed-dlg');
    dlg.style.width = 'min(440px, 94vw)';
    dlg.style.overflow = 'visible';

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

    var tokenField = passwordField('Verification Token', verificationToken, 'settings-token');
    body.appendChild(tokenField);

    var tokenHint = el('div', 'small muted');
    tokenHint.textContent = 'Paste the token from your verification email to prove account ownership';
    tokenHint.style.marginTop = '-8px';
    tokenHint.style.fontSize = '11px';
    body.appendChild(tokenHint);

    // ── Theme section ──
    body.appendChild(sectionLabel('Appearance'));
    body.appendChild(selectField('Theme', {
      id: 'settings-theme', value: theme,
      options: [{ value: 'dark', label: 'Dark' }, { value: 'light', label: 'Light' }]
    }));

    // ── Devices section (hidden if video_disabled) ──
    var devicesLabel = sectionLabel('Devices');
    devicesLabel.id = 'settings-devices-label';
    body.appendChild(devicesLabel);

    // Linux warning
    var osType = document.body.getAttribute('data-os') || '';
    if (osType === 'linux') {
      var linuxWarn = el('div', 'banner warn small');
      linuxWarn.id = 'settings-linux-warning';
      linuxWarn.innerHTML = 'Video/audio calls may not work on Linux due to WebKitGTK limitations.';
      linuxWarn.style.marginBottom = '8px';
      body.appendChild(linuxWarn);
    }

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

  function passwordField(placeholder, value, id) {
    var wrap = el('div', '', '');
    wrap.style.position = 'relative';
    var inp = el('input', 'ed-dlg-input');
    inp.type = 'password';
    inp.id = id;
    inp.placeholder = placeholder;
    inp.value = value;
    inp.style.paddingRight = '36px';
    wrap.appendChild(inp);

    var toggle = el('button', '');
    toggle.type = 'button';
    toggle.title = 'Show/hide token';
    toggle.style.cssText = 'position:absolute;right:6px;top:50%;transform:translateY(-50%);background:none;border:none;cursor:pointer;padding:4px;color:var(--muted);font-size:16px;line-height:1';
    toggle.innerHTML = '&#x1F441;';
    toggle.onclick = function() {
      if (inp.type === 'password') {
        inp.type = 'text';
        toggle.style.opacity = '1';
      } else {
        inp.type = 'password';
        toggle.style.opacity = '0.5';
      }
    };
    toggle.style.opacity = '0.5';
    wrap.appendChild(toggle);
    return wrap;
  }

  function selectField(labelText, selectOpts) {
    var wrap = el('div', '', '');
    var lbl = el('label', 'small muted');
    lbl.textContent = labelText;
    lbl.style.display = 'block';
    lbl.style.marginBottom = '4px';
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
    fetch('/api/settings/quick/get').then(function(r) { return r.json(); }).catch(function() { return {}; })
      .then(function(cfg) {
        // Disable devices section if video is disabled
        if (cfg.video_disabled) {
          var camField = document.getElementById('settings-camera-field');
          var micField = document.getElementById('settings-mic-field');
          if (camField) camField.style.opacity = '0.5';
          if (micField) micField.style.opacity = '0.5';
          // Disable the gsel dropdowns
          var camDis = document.getElementById('settings-camera');
          var micDis = document.getElementById('settings-mic');
          if (camDis) {
            camDis.disabled = true;
            var camWrap = camDis.closest('.gsel');
            if (camWrap) camWrap.style.pointerEvents = 'none';
          }
          if (micDis) {
            micDis.disabled = true;
            var micWrap = micDis.closest('.gsel');
            if (micWrap) micWrap.style.pointerEvents = 'none';
          }
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
    fetch('/api/settings/quick', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    }).then(function(res) {
      if (!res.ok) return res.text().then(function(t) { throw new Error(t); });

      // Update navbar name
      var meLabel = document.querySelector('.me-label');
      if (meLabel) meLabel.textContent = labelVal || 'Me';

      // Update the cog data attrs for next open
      btn.setAttribute('data-label', labelVal);
      btn.setAttribute('data-email', emailVal);
      btn.setAttribute('data-verification-token', tokenVal);

      closePopup();
    }).catch(function(err) {
      console.error('settings: save failed:', err);
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
