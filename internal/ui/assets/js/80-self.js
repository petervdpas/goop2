// Settings page: service tabs, health checks, sidebar nav, avatar upload,
// export/import, templates browse, generateToken/copyToken utilities.
(function() {
  var page = document.querySelector('.page-self');
  if (!page) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  var csrf = page.dataset.csrf || '';
  var bridgeURL = page.dataset.bridgeUrl || '';

  // ── Global utilities (used by inline onclick handlers) ──
  window.generateToken = function(inputId) {
    var arr = new Uint8Array(32);
    crypto.getRandomValues(arr);
    var token = 'goop2__' + btoa(String.fromCharCode.apply(null, arr)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    document.getElementById(inputId).value = token;
  };

  window.copyToken = function(inputId) {
    var el = document.getElementById(inputId);
    if (!el || !el.value) return;
    navigator.clipboard.writeText(el.value).then(function() {
      var btn = el.parentNode.querySelector('[onclick*="copyToken"]');
      if (btn) { var orig = btn.textContent; btn.textContent = 'Copied!'; setTimeout(function(){ btn.textContent = orig; }, 1500); }
    });
  };

  // ── Service sub-tabs ──
  document.querySelectorAll('.svc-tab').forEach(function(tab) {
    tab.addEventListener('click', function() {
      var target = tab.getAttribute('data-svc');
      document.querySelectorAll('.svc-tab').forEach(function(t) { t.classList.toggle('active', t.getAttribute('data-svc') === target); });
      document.querySelectorAll('.svc-tab-panel').forEach(function(p) { p.classList.toggle('active', p.getAttribute('data-svc') === target); });
    });
  });

  // ── Per-service health checks ──
  var svcMap = {
    registration: { url: 'svc-reg-url', status: 'svc-reg-status', key: 'registration', token: 'svc-reg-token' },
    credits:      { url: 'svc-credits-url', status: 'svc-credits-status', key: 'credits', token: 'svc-credits-token' },
    email:        { url: 'svc-email-url', status: 'svc-email-status', key: 'email' },
    templates:    { url: 'svc-templates-url', status: 'svc-templates-status', key: 'templates', token: 'svc-templates-token' }
  };

  function checkService(name) {
    var cfg = svcMap[name];
    if (!cfg) return;
    var statusEl = document.getElementById(cfg.status);
    var urlEl = document.getElementById(cfg.url);
    var btn = document.querySelector('[data-svc-check="' + name + '"]');
    var svcURL = urlEl ? urlEl.value.trim() : '';
    if (!svcURL) return;

    if (statusEl) { statusEl.textContent = '...'; statusEl.className = 'svc-status'; }
    if (btn) { btn.disabled = true; btn.textContent = '...'; }

    api('/api/services/check?type=' + encodeURIComponent(cfg.key) + '&url=' + encodeURIComponent(svcURL))
      .then(function(data) {
        if (!statusEl) return;
        if (data.ok) {
          var tokenEl = cfg.token ? document.getElementById(cfg.token) : null;
          var missingToken = tokenEl && !tokenEl.value.trim();
          if (missingToken) {
            statusEl.textContent = 'running \u2014 admin token missing';
            statusEl.className = 'svc-status svc-warn';
          } else {
            var label = 'running';
            if (data.dummy_mode) { label += ' (dummy)'; } else { label += ' (active)'; }
            statusEl.textContent = label;
            statusEl.className = 'svc-status ' + (data.dummy_mode ? 'svc-warn' : 'svc-ok');
          }
        } else {
          statusEl.textContent = data.error || 'not reachable';
          statusEl.className = 'svc-status svc-err';
        }
      })
      .catch(function(err) {
        console.error('Service check failed:', err);
        if (statusEl) { statusEl.textContent = 'check failed'; statusEl.className = 'svc-status svc-err'; }
      })
      .finally(function() {
        if (btn) { btn.disabled = false; btn.textContent = 'Check'; }
      });
  }

  // Wire up each Check button: show/hide based on URL, auto-check on load
  Object.keys(svcMap).forEach(function(name) {
    var cfg = svcMap[name];
    var urlEl = document.getElementById(cfg.url);
    var btn = document.querySelector('[data-svc-check="' + name + '"]');
    if (!urlEl || !btn) return;

    function updateVisibility() {
      btn.style.display = urlEl.value.trim() ? '' : 'none';
    }
    btn.addEventListener('click', function() { checkService(name); });
    urlEl.addEventListener('input', updateVisibility);
    updateVisibility();
    if (urlEl.value.trim()) checkService(name);
  });

  // ── Settings sidebar navigation ──
  var navItems = document.querySelectorAll('.settings-nav-item');
  var sections = document.querySelectorAll('.settings-section');
  var saveBar  = document.getElementById('settings-save-bar');

  navItems.forEach(function(item) {
    item.addEventListener('click', function() {
      var target = item.getAttribute('data-section');
      navItems.forEach(function(n) { n.classList.remove('active'); });
      item.classList.add('active');
      sections.forEach(function(s) {
        s.classList.toggle('active', s.getAttribute('data-section') === target);
      });
      if (saveBar) {
        saveBar.style.display = (target === 'data') ? 'none' : '';
      }
    });
  });

  // ── Avatar upload ──
  var fileInput = document.getElementById('avatar-file-input');
  var chooseBtn = document.getElementById('avatar-choose-btn');
  var removeBtn = document.getElementById('avatar-remove-btn');
  var preview = document.getElementById('avatar-preview');

  if (chooseBtn) {
    chooseBtn.addEventListener('click', function() {
      fileInput.click();
    });
  }

  if (fileInput) {
    fileInput.addEventListener('change', function() {
      var file = fileInput.files[0];
      if (!file) return;

      var img = new Image();
      img.onload = function() {
        var canvas = document.createElement('canvas');
        canvas.width = 256;
        canvas.height = 256;
        var ctx = canvas.getContext('2d');

        var size = Math.min(img.width, img.height);
        var sx = (img.width - size) / 2;
        var sy = (img.height - size) / 2;
        ctx.drawImage(img, sx, sy, size, size, 0, 0, 256, 256);

        canvas.toBlob(function(blob) {
          if (!blob) return;
          if (blob.size > 256 * 1024) {
            alert('Resized image still too large. Try a smaller image.');
            return;
          }

          var fd = new FormData();
          fd.append('avatar', blob, 'avatar.png');

          fetch('/api/avatar/upload', { method: 'POST', body: fd })
            .then(function(r) {
              if (!r.ok) throw new Error('Upload failed');
              return r.json();
            })
            .then(function(data) {
              preview.src = '/api/avatar?v=' + data.hash;
              if (!removeBtn) {
                window.location.reload();
              }
            })
            .catch(function(err) {
              alert('Upload failed: ' + err.message);
            });
        }, 'image/png');
      };
      img.src = URL.createObjectURL(file);
    });
  }

  if (removeBtn) {
    removeBtn.addEventListener('click', function() {
      fetch('/api/avatar/delete', { method: 'POST' })
        .then(function(r) {
          if (!r.ok) throw new Error('Delete failed');
          window.location.reload();
        })
        .catch(function(err) {
          alert('Delete failed: ' + err.message);
        });
    });
  }

  // ── Export site ──
  var exportBtn = document.getElementById('export-btn');
  if (exportBtn) {
    exportBtn.addEventListener('click', function() {
      exportBtn.disabled = true;
      exportBtn.textContent = 'Exporting...';

      fetch('/api/site/export')
        .then(function(res) {
          if (!res.ok) return res.text().then(function(t) { throw new Error(t); });
          var cd = res.headers.get('Content-Disposition') || '';
          var match = cd.match(/filename="?([^"]+)"?/);
          var filename = match ? match[1] : 'goop-export.zip';
          return res.blob().then(function(blob) { return { blob: blob, filename: filename }; });
        })
        .then(function(result) {
          if (bridgeURL) {
            var fd = new FormData();
            fd.append('file', result.blob, result.filename);
            fd.append('filename', result.filename);
            return fetch(bridgeURL + '/export-save', { method: 'POST', body: fd })
              .then(function(res) {
                if (!res.ok) throw new Error('Save dialog failed');
                return res.json();
              })
              .then(function(data) {
                if (data.cancelled) return;
                Goop.ui.toast({ title: 'Exported', message: 'Saved to ' + data.path, duration: 3000 });
              });
          }
          // Fallback: browser download
          var url = URL.createObjectURL(result.blob);
          var a = document.createElement('a');
          a.href = url;
          a.download = result.filename;
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
          URL.revokeObjectURL(url);
          Goop.ui.toast({ title: 'Exported', message: 'Site archive downloaded.', duration: 3000 });
        })
        .catch(function(err) {
          var errMsg = err.message || 'Unknown error';
          Goop.ui.toast({ title: 'Export Error', message: errMsg, duration: 6000 });
        })
        .finally(function() {
          exportBtn.disabled = false;
          exportBtn.textContent = 'Export site (.zip)';
        });
    });
  }

  // ── Import site ──
  var importBtn  = document.getElementById('import-btn');
  var importFile = document.getElementById('import-file');

  if (importBtn && importFile) {
    importBtn.addEventListener('click', function() {
      importFile.click();
    });

    importFile.addEventListener('change', function() {
      var file = importFile.files[0];
      if (!file) return;

      var msg = 'Import "' + file.name + '"?\n\nThis will DELETE all your current site files and database tables and replace them with the archive contents. This cannot be undone.';

      function doImport() {
        var fd = new FormData();
        fd.append('csrf', csrf);
        fd.append('file', file);

        fetch('/api/site/import', { method: 'POST', body: fd })
          .then(function(res) {
            if (!res.ok) return res.text().then(function(t) { throw new Error(t); });
            return res.json();
          })
          .then(function() {
            Goop.ui.toast({ title: 'Site Imported', message: 'Archive applied. Redirecting to editor...', duration: 3000 });
            setTimeout(function() { window.location.href = '/edit'; }, 1500);
          })
          .catch(function(err) {
            var errMsg = err.message || 'Unknown error';
            Goop.ui.toast({ title: 'Import Error', message: errMsg, duration: 6000 });
          })
          .finally(function() {
            importFile.value = '';
          });
      }

      if (window.Goop && window.Goop.ui && window.Goop.ui.confirm) {
        Goop.ui.confirm(msg, 'Import Site').then(function(ok) {
          if (ok) doImport();
          else importFile.value = '';
        });
      } else if (confirm(msg)) {
        doImport();
      } else {
        importFile.value = '';
      }
    });
  }

  // ── Splash picker ──
  document.querySelectorAll('.splash-picker input[type="radio"]').forEach(function(radio) {
    radio.addEventListener('change', function() {
      document.querySelectorAll('.splash-picker-option').forEach(function(opt) {
        opt.classList.remove('selected');
      });
      radio.closest('.splash-picker-option').classList.add('selected');
    });
  });

  // ── Templates directory browse ──
  var tplBrowse = document.getElementById('templates-dir-browse');
  var tplInput  = document.getElementById('templates-dir-input');
  if (tplBrowse && tplInput) {
    if (bridgeURL) {
      tplBrowse.addEventListener('click', function() {
        fetch(bridgeURL + '/select-dir?title=' + encodeURIComponent('Choose templates directory'), { method: 'POST' })
          .then(function(r) { return r.json(); })
          .then(function(data) {
            if (!data.cancelled && data.path) {
              tplInput.value = data.path;
            }
          })
          .catch(function(err) { console.error('Browse failed:', err); });
      });
    } else {
      tplBrowse.style.display = 'none';
    }
  }
})();
