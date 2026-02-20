// Create Groups page (/create/groups): create form + hosted groups list.
(function() {
  if (!document.querySelector('#cg-create-form')) return;

  var g = window.Goop && window.Goop.groups;
  var core = window.Goop && window.Goop.core;
  var gsel = window.Goop && window.Goop.select;
  if (!g || !core || !gsel) return;

  var api = core.api;
  var toast = core.toast;

  var hostedListEl = document.getElementById('cg-hosted-list');
  var nameInput = document.getElementById('cg-name');
  var appTypeSelect = document.getElementById('cg-apptype');
  var maxMembersInput = document.getElementById('cg-maxmembers');
  var createBtn = document.getElementById('cg-create-btn');

  var hostedOpts = { showMgmt: false };

  gsel.init(appTypeSelect);
  g.renderHostedGroups(hostedListEl, hostedOpts);

  createBtn.addEventListener('click', function() {
    var name = (nameInput.value || '').trim();
    var appType = (gsel.val(appTypeSelect) || 'general').trim();
    var maxMembers = parseInt(maxMembersInput.value, 10) || 0;

    if (!name) { toast('Group name is required', true); return; }

    var p;
    if (appType === 'listen' && window.Goop && window.Goop.listen) {
      p = window.Goop.listen.create(name);
    } else {
      p = api('/api/groups', { name: name, app_type: appType, max_members: maxMembers });
    }

    p.then(function() {
      toast('Group created: ' + name);
      nameInput.value = '';
      gsel.setVal(appTypeSelect, 'general');
      maxMembersInput.value = '0';
      g.renderHostedGroups(hostedListEl, hostedOpts);
    }).catch(function(err) {
      toast('Failed to create group: ' + err.message, true);
    });
  });
})();
