// Lua scripts page: new script/function, delete, prefab install.
(function() {
  var page = document.querySelector('.page-lua');
  if (!page) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  var csrf = page.dataset.csrf || '';

  // New script/function buttons
  document.querySelectorAll('.lua-new-btn').forEach(function(btn) {
    btn.addEventListener('click', async function() {
      var isFunc = btn.getAttribute('data-func') === '1';
      var label = isFunc ? 'Function' : 'Script';
      var name = await Goop.dialog.prompt({
        title: 'New ' + label,
        message: 'Letters, numbers, hyphens, underscores only.',
        placeholder: label + ' name',
      });
      if (!name) return;
      name = name.trim().replace(/\.lua$/i, '');
      if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
        Goop.dialog.alert('Invalid Name', 'Use only letters, numbers, hyphens, and underscores.');
        return;
      }

      var form = document.createElement('form');
      form.method = 'POST';
      form.action = '/lua/new';
      var html = '<input type="hidden" name="csrf" value="' + csrf + '">' +
                 '<input type="hidden" name="name" value="' + name + '">';
      if (isFunc) html += '<input type="hidden" name="is_func" value="1">';
      form.innerHTML = html;
      document.body.appendChild(form);
      form.submit();
    });
  });

  // Delete button
  var delBtn = document.getElementById('lua-delete-btn');
  if (delBtn) {
    delBtn.addEventListener('click', function() {
      var name = delBtn.getAttribute('data-name');
      var isFunc = delBtn.getAttribute('data-func') === '1';
      var label = isFunc ? 'function' : 'script';
      var msg = 'Delete ' + label + ' "' + name + '.lua"? This cannot be undone.';

      Goop.dialog.confirm(msg, 'Delete').then(function(ok) {
        if (ok) doDelete(name, isFunc);
      });
    });
  }

  function doDelete(name, isFunc) {
    var form = document.createElement('form');
    form.method = 'POST';
    form.action = '/lua/delete';
    var html = '<input type="hidden" name="csrf" value="' + csrf + '">' +
               '<input type="hidden" name="name" value="' + name + '">';
    if (isFunc) html += '<input type="hidden" name="is_func" value="1">';
    form.innerHTML = html;
    document.body.appendChild(form);
    form.submit();
  }

  // Script click — load content via API instead of full page reload
  document.querySelectorAll('.lua-script-row').forEach(function(link) {
    link.addEventListener('click', function(e) {
      var cm = window.Goop && window.Goop.cm;
      var luaApi = window.Goop && window.Goop.api && window.Goop.api.lua;
      if (!cm || !luaApi) return; // fall through to href

      var href = link.getAttribute('href') || '';
      var params = new URLSearchParams(href.split('?')[1] || '');
      var name = params.get('edit');
      var isFunc = params.get('func') === '1';
      if (!name) return;

      e.preventDefault();

      luaApi.content(name, isFunc).then(function(data) {
        cm.setContent(data.content, name + '.lua');

        var nameInput = document.querySelector('input[name="name"]');
        if (nameInput) nameInput.value = name;
        var funcInput = document.querySelector('input[name="is_func"]');
        if (funcInput) funcInput.value = isFunc ? '1' : '';

        var titleEl = document.querySelector('.ed-right-title');
        if (titleEl) titleEl.textContent = name + '.lua';

        if (delBtn) {
          delBtn.setAttribute('data-name', name);
          delBtn.setAttribute('data-func', isFunc ? '1' : '0');
          delBtn.style.display = '';
        }

        document.querySelectorAll('.lua-script-row').forEach(function(r) { r.classList.remove('selected'); });
        link.classList.add('selected');

        history.replaceState(null, '', '/lua?edit=' + encodeURIComponent(name) + (isFunc ? '&func=1' : ''));
      }).catch(function() {
        window.location.href = href;
      });
    });
  });

  // Prefab "Install all" buttons
  document.querySelectorAll('.lua-prefab-apply').forEach(function(btn) {
    btn.addEventListener('click', function() {
      var dir = btn.getAttribute('data-dir');
      var name = btn.getAttribute('data-name');
      applyPrefab(dir, null, name);
    });
  });

  // Per-script "Add" buttons
  document.querySelectorAll('.lua-prefab-script-add').forEach(function(btn) {
    btn.addEventListener('click', function() {
      var dir = btn.getAttribute('data-dir');
      var script = btn.getAttribute('data-script');
      applyPrefab(dir, script, script + '.lua');
    });
  });

  function applyPrefab(dir, script, label) {
    api('/api/lua/prefabs/apply', { prefab: dir, script: script || undefined, csrf: csrf })
    .then(function() {
      Goop.toast({ title: 'Installed', message: '"' + label + '" added.', duration: 2000, level: 'success' });
      setTimeout(function() { window.location.href = '/lua'; }, 500);
    })
    .catch(function(err) {
      var errMsg = err.message || 'Unknown error';
      Goop.toast({ title: 'Error', message: errMsg, duration: 6000, level: 'error' });
    });
  }
})();
