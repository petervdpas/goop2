// Global group-invite notifier: shows a toast on any page when this peer
// receives an invite, with a click-through to the Groups page.
(function() {
  // Only run when a peer is active (body carries data-self-id).
  if (!document.body || !document.body.dataset.selfId) return;

  function initInviteNotify() {
    if (!window.Goop || !window.Goop.mq) { setTimeout(initInviteNotify, 100); return; }

    Goop.mq.onGroupInvite( function(from, topic, payload, ack) {
      var p = (payload && payload.payload) || {};
      var name = p.group_name || p.group_id || 'a group';

      if (window.Goop && window.Goop.toast) {
        window.Goop.toast({
          icon: '👥',
          title: 'Group Invite',
          message: 'You were invited to: ' + name + '. Click to view.',
          duration: 8000,
          onClick: function() {
            window.location.href = '/self/groups';
          }
        });
      }
      ack();
    });
  }

  initInviteNotify();
})();
