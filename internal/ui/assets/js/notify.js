// Global notifier: group invites and relay status toasts on any page.
(function() {
  // Only run when a peer is active (body carries data-self-id).
  if (!document.body || !document.body.dataset.selfId) return;

  function initNotify() {
    if (!window.Goop || !window.Goop.mq) { setTimeout(initNotify, 100); return; }

    // ── Group invite toast ────────────────────────────────────────────────────
    Goop.mq.onGroupInvite(function(from, topic, payload, ack) {
      var p = (payload && payload.payload) || {};
      var name = p.group_name || p.group_id || 'a group';
      if (window.Goop && window.Goop.toast) {
        window.Goop.toast({
          icon: '👥',
          title: 'Group Invite',
          message: 'You were invited to: ' + name + '. Click to view.',
          duration: 8000,
          onClick: function() { window.location.href = '/self/groups'; }
        });
      }
      ack();
    });

    // ── Relay status toast ────────────────────────────────────────────────────
    // Only show relay notifications when the relay is unhealthy (lost/timeout)
    // or when it recovers after a failure.  "waiting" and "connected" at startup
    // are silent — normal startup noise that the user doesn't need to see.
    Goop.mq.onRelayStatus(function(from, topic, payload, ack) {
      ack();
      if (!payload || !window.Goop || !window.Goop.toast) return;
      var status = payload.status || '';
      var msg    = payload.msg    || '';
      if (status === 'lost' || status === 'timeout') {
        window.Goop.toast({
          icon: '⚠️',
          title: 'Relay unavailable',
          message: msg,
          duration: 8000,
        });
      } else if (status === 'recovered') {
        window.Goop.toast({
          icon: '✅',
          title: 'Relay restored',
          message: msg,
          duration: 4000,
        });
      }
    });
  }

  initNotify();
})();
