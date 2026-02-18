// Layout-level JS: openExternal, chat notifications, credits balance.
// Runs on every page (no guard element needed).
(() => {
  var core = window.Goop && window.Goop.core || {};
  var escapeHtml = core.escapeHtml;
  // ── Expose bridge URL globally so any page script can call bridge endpoints ──
  window.Goop = window.Goop || {};
  window.Goop.bridgeURL = document.body.dataset.bridgeUrl
    || new URLSearchParams(location.search).get('bridge')
    || '';

  // ── openExternal (global) ──
  window.openExternal = function(url) {
    var b = window.Goop.bridgeURL;
    if (b) { fetch(b + '/open?url=' + encodeURIComponent(url), { method: 'POST' }); return false; }
    location.href = '/open?url=' + encodeURIComponent(url);
    return false;
  };

  // ── Read open_sites_external synchronously from server-rendered data attribute ──
  window._openSitesExternal = document.body.dataset.openSitesExternal === 'true';

  // ── openSite: embedded tab or external browser depending on setting ──
  window.openSite = function(peerID, peerName) {
    if (window._openSitesExternal) {
      var fullUrl = window.location.origin + '/p/' + peerID + '/';
      return openExternal(fullUrl);
    }
    window.location.href = '/view?open=' + encodeURIComponent(peerID) + '&name=' + encodeURIComponent(peerName || '');
    return false;
  };

  // Show "View" nav only when embedded mode is on (open_sites_external=false)
  var navView = document.getElementById('nav-view');
  if (navView) {
    navView.style.display = window._openSitesExternal ? 'none' : '';
  }

  // ── Chat notifications (only when logged in) ──
  var selfID = document.body.dataset.selfId;
  if (selfID) {
    if (!window.Goop) window.Goop = {};

    var currentPeerPage = null;
    var pathMatch = window.location.pathname.match(/\/peer\/([^/]+)/);
    if (pathMatch) {
      currentPeerPage = pathMatch[1];
    }

    function initChatNotifications() {
      if (!window.Goop || !window.Goop.toast) {
        setTimeout(initChatNotifications, 100);
        return;
      }

      var eventSource = new EventSource('/api/chat/events');

      eventSource.addEventListener('message', function(e) {
        try {
          var msg = JSON.parse(e.data);
          var msgType = msg.type || 'direct';

          if (msgType === 'direct' && msg.from && msg.from !== selfID && msg.from !== currentPeerPage) {
            var avatarUrl = '/api/avatar/peer/' + encodeURIComponent(msg.from);
            var avatarImg = '<img src="' + avatarUrl + '" style="width:28px;height:28px;border-radius:50%;object-fit:cover;">';

            fetch('/api/peers').then(function(r){ return r.json(); }).then(function(peers) {
              var label = msg.from.substring(0, 8) + '...';
              if (Array.isArray(peers)) {
                var found = peers.find(function(p){ return p.ID === msg.from; });
                if (found && found.Content) label = found.Content;
              }

              var preview = msg.content;
              if (preview.length > 60) preview = preview.substring(0, 60) + '...';

              window.Goop.toast({
                icon: avatarImg,
                title: label,
                message: '<div>' + preview.replace(/</g,'&lt;') + '</div>',
                onClick: function() {
                  window.location.href = '/peer/' + msg.from;
                },
                duration: 8000
              });
            }).catch(function() {
              var shortID = msg.from.substring(0, 8) + '...';
              var preview = msg.content;
              if (preview.length > 60) preview = preview.substring(0, 60) + '...';

              window.Goop.toast({
                icon: avatarImg,
                title: shortID,
                message: '<div>' + preview.replace(/</g,'&lt;') + '</div>',
                onClick: function() {
                  window.location.href = '/peer/' + msg.from;
                },
                duration: 8000
              });
            });
          }
        } catch (err) {
          console.error('Failed to parse chat message:', err);
        }
      });

      eventSource.onerror = function(err) {
        console.error('Chat SSE error:', err);
      };
    }

    initChatNotifications();
  }

  // ── Credits balance loader ──
  var el = document.getElementById('meCredits');
  if (el) {
    fetch('/api/my-balance').then(function(r){ return r.json(); }).then(function(d){
      if (!d.credits_active) return;
      el.textContent = '\uD83E\uDE99 ' + d.balance + ' credits';
      el.style.display = '';
    }).catch(function(){});
  }
})();
