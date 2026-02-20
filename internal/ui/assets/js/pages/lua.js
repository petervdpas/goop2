// Lua scripts page: new script/function, delete, prefab install.
(function() {
  var page = document.querySelector('.page-lua');
  if (!page) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  var csrf = page.dataset.csrf || '';

  // New script/function buttons
  document.querySelectorAll('.lua-new-btn').forEach(function(btn) {
    btn.addEventListener('click', function() {
      var isFunc = btn.getAttribute('data-func') === '1';
      var label = isFunc ? 'Function' : 'Script';
      var name = prompt(label + ' name (letters, numbers, hyphens, underscores):');
      if (!name) return;
      name = name.trim().replace(/\.lua$/i, '');
      if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
        alert('Invalid name. Use only letters, numbers, hyphens, and underscores.');
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

      Goop.dialogs.confirm(msg, 'Delete').then(function(ok) {
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
      Goop.toast({ title: 'Installed', message: '"' + label + '" added.', duration: 2000 });
      setTimeout(function() { window.location.href = '/lua'; }, 500);
    })
    .catch(function(err) {
      var errMsg = err.message || 'Unknown error';
      Goop.toast({ title: 'Error', message: errMsg, duration: 6000 });
    });
  }
})();
