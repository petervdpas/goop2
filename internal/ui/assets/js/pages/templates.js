// Templates page: apply template with confirm dialog, credit handling, star animation.
(function() {
  var page = document.querySelector('.page-templates');
  if (!page) return;

  var csrf = page.dataset.csrf || '';

  document.querySelectorAll('.tpl-card-apply').forEach(function(btn) {
    btn.addEventListener('click', function() {
      var dir    = btn.getAttribute('data-dir');
      var name   = btn.getAttribute('data-name');
      var source = btn.getAttribute('data-source');

      var msg = 'Apply "' + name + '"?\n\nThis will DELETE all your current site files and database tables and replace them with the template. This cannot be undone.';

      if (window.Goop && window.Goop.ui && window.Goop.ui.confirm) {
        Goop.ui.confirm(msg, 'Apply Template').then(function(ok) {
          if (ok) applyTemplate(dir, name, source);
        });
      } else if (confirm(msg)) {
        applyTemplate(dir, name, source);
      }
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
          Goop.ui.toast({ title: 'Warning', message: t || 'Template could not be applied, insufficient funding', duration: 6000 });
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
      Goop.ui.toast({ title: 'Template Applied', message: msg, duration: 5000 });
    })
    .catch(function(err) {
      var errMsg = err.message || 'Unknown error';
      Goop.ui.toast({ title: 'Error', message: errMsg, duration: 6000 });
    });
  }
})();
