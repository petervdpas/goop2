// Templates page: apply template with confirm dialog, credit handling, star animation.
(function() {
  var page = document.querySelector('.page-templates');
  if (!page) return;

  var csrf = page.dataset.csrf || '';

  document.querySelectorAll('.tpl-card-apply:not(.tpl-local-apply)').forEach(function(btn) {
    btn.addEventListener('click', function() {
      var dir    = btn.getAttribute('data-dir');
      var name   = btn.getAttribute('data-name');
      var source = btn.getAttribute('data-source');

      var msg = 'Apply "' + name + '"?\n\nThis will DELETE all your current site files and database tables and replace them with the template. This cannot be undone.';

      Goop.dialog.confirm(msg, 'Apply Template').then(function(ok) {
        if (ok) applyTemplate(dir, name, source);
      });
    });
  });

  function applyTemplate(dir, name, source) {
    var url = source === 'store' ? '/api/templates/apply-store' : '/api/templates/apply';

    fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ template: dir, csrf: csrf })
    })
    .then(function(res) {
      if (res.status === 402) {
        return res.text().then(function(t) {
          Goop.toast({ title: 'Warning', message: t || 'Template could not be applied, insufficient funding', duration: 6000 });
          return null;
        });
      }
      if (!res.ok) return res.text().then(function(t) { throw new Error(t); });
      return res.json();
    })
    .then(function(data) {
      if (!data) return;
      // Move active star to the newly applied card; demote old active to owned
      var oldStar = document.querySelector('.tpl-active-star');
      if (oldStar) {
        oldStar.className = 'tpl-owned-star';
        oldStar.title = 'Previously applied';
      }
      document.querySelectorAll('.tpl-card-active').forEach(function(c) { c.classList.remove('tpl-card-active'); });
      // Remove owned star from the newly active card (gold replaces gray)
      var existingOwned = document.querySelector('.tpl-card[data-dir="' + dir + '"] .tpl-owned-star');
      if (existingOwned) existingOwned.remove();
      var card = document.querySelector('.tpl-card[data-dir="' + dir + '"]');
      if (card) {
        card.classList.add('tpl-card-active');
        var star = document.createElement('span');
        star.className = 'tpl-active-star';
        star.title = 'Currently active';
        star.textContent = '\u2605';
        card.insertBefore(star, card.firstChild);
      }

      var msg = '"' + name + '" is now active.';
      if (data.balance !== undefined) {
        msg = 'Used credits. New balance: ' + data.balance + ' credits.';
        var el = document.getElementById('meCredits');
        if (el) el.textContent = '\uD83E\uDE99 ' + data.balance + ' credits';
      }
      Goop.toast({ title: 'Template Applied', message: msg, duration: 5000 });
    })
    .catch(function(err) {
      var errMsg = err.message || 'Unknown error';
      Goop.toast({ title: 'Error', message: errMsg, duration: 6000 });
    });
  }
  // ── Local Template ──
  var tplLocal = document.getElementById('tpl-local');
  if (tplLocal && window.Goop.pathpicker) {
    var preview   = document.getElementById('local-tpl-preview');

    var tplPicker = window.Goop.pathpicker.init(
      tplLocal.querySelector('.pathpicker'),
      {
        title: 'Choose template folder',
        onChange: function(path) { if (path) validateLocal(path); },
      }
    );

    function validateLocal(path) {
      preview.style.display = 'none';
      fetch('/api/templates/validate-local', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: path })
      })
      .then(function(res) {
        if (!res.ok) return res.text().then(function(t) { throw new Error(t); });
        return res.json();
      })
      .then(function(meta) {
        document.getElementById('local-tpl-icon').textContent = meta.icon || '';
        document.getElementById('local-tpl-name').textContent = meta.name || 'Unnamed';
        document.getElementById('local-tpl-cat').textContent  = meta.category || '';
        document.getElementById('local-tpl-desc').textContent = meta.description || '';
        preview.style.display = '';
      })
      .catch(function(err) {
        Goop.toast({ title: 'Invalid Template', message: err.message || 'Could not read template', duration: 5000 });
      });
    }

    var applyBtn = document.getElementById('local-tpl-apply');
    if (applyBtn) {
      applyBtn.addEventListener('click', function() {
        if (!localPath) return;
        var name = document.getElementById('local-tpl-name').textContent || 'local template';
        var msg = 'Apply "' + name + '"?\n\nThis will DELETE all your current site files and database tables and replace them with the template. This cannot be undone.';

        Goop.dialog.confirm(msg, 'Apply Template').then(function(ok) {
          if (!ok) return;
          fetch('/api/templates/apply-local', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: localPath, csrf: csrf })
          })
          .then(function(res) {
            if (!res.ok) return res.text().then(function(t) { throw new Error(t); });
            return res.json();
          })
          .then(function(data) {
            if (!data) return;
            // Clear any existing active star
            var oldStar = document.querySelector('.tpl-active-star');
            if (oldStar) {
              oldStar.className = 'tpl-owned-star';
              oldStar.title = 'Previously applied';
            }
            document.querySelectorAll('.tpl-card-active').forEach(function(c) { c.classList.remove('tpl-card-active'); });
            Goop.toast({ title: 'Template Applied', message: '"' + (data.template || name) + '" is now active.', duration: 5000 });
          })
          .catch(function(err) {
            Goop.toast({ title: 'Error', message: err.message || 'Unknown error', duration: 6000 });
          });
        });
      });
    }
  }
})();
