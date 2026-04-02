// Create Groups page (/create/groups): create form + hosted groups list.
(function() {
  if (!document.querySelector('#cg-create-form')) return;

  var g = window.Goop && window.Goop.groups;
  var core = window.Goop && window.Goop.core;
  var gsel = window.Goop && window.Goop.select;
  if (!g || !core || !gsel) return;

  var toast = core.toast;

  var hostedListEl = document.getElementById('cg-hosted-list');
  var nameInput = document.getElementById('cg-name');
  var groupTypeSelect = document.getElementById('cg-grouptype');
  var maxMembersInput = document.getElementById('cg-maxmembers');
  var createBtn = document.getElementById('cg-create-btn');

  var hostedOpts = { showMgmt: true };

  gsel.init(groupTypeSelect);
  g.renderHostedGroups(hostedListEl, hostedOpts);

  createBtn.addEventListener('click', function() {
    var name = (nameInput.value || '').trim();
    var groupType = (gsel.val(groupTypeSelect) || 'general').trim();
    var maxMembers = parseInt(maxMembersInput.value, 10) || 0;

    if (!name) { toast('Group name is required', "warning"); return; }

    var p;
    if (groupType === 'listen' && window.Goop && window.Goop.listen) {
      p = window.Goop.listen.create(name);
    } else {
      p = Goop.api.groups.create({ name: name, group_type: groupType, max_members: maxMembers });
    }

    p.then(function() {
      toast('Group created: ' + name);
      nameInput.value = '';
      gsel.setVal(groupTypeSelect, 'general');
      maxMembersInput.value = '0';
      g.renderHostedGroups(hostedListEl, hostedOpts);
    }).catch(function(err) {
      toast('Failed to create group: ' + err.message, true);
    });
  });
})();
