// Global group-invite notifier: shows a toast on any page when this peer
// receives an invite, with a click-through to the Groups page.
(function() {
  // Only run when a peer is active (body carries data-self-id).
  if (!document.body || !document.body.dataset.selfId) return;

  var sse = new EventSource('/api/groups/events');

  sse.addEventListener('invite', function(e) {
    try {
      var evt = JSON.parse(e.data);
      var p = evt.payload || {};

      // Suppress invite toast for call channels (rt-*) —
      // the call accept modal handles user interaction directly.
      if ((p.group_id || '').startsWith('rt-')) return;

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
    } catch (_) {}
  });

  sse.onerror = function() {};
})();
