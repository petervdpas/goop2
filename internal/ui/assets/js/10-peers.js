// Peers page: peer list rendering, SSE, broadcast chat, call buttons, autorefresh.
(function() {
  var peersPage = document.querySelector('.peers-page');
  if (!peersPage) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  var selfID = peersPage.dataset.selfId || '';
  var selfName = peersPage.dataset.selfName || '';
  var selfVideoDisabled = peersPage.dataset.selfVideoDisabled === 'true';
  var hideUnverified = peersPage.dataset.hideUnverified === 'true';
  var baseURL = peersPage.dataset.baseUrl || '';

  var peersList = document.getElementById('peers-list');
  var peerCount = document.getElementById('peer-count');
  var peerSearch = document.getElementById('peer-search');
  var messagesDiv = document.getElementById('broadcast-messages');
  var form = document.getElementById('broadcast-form');
  var input = document.getElementById('broadcast-input');

  // Track unread direct messages per peer
  var unreadPeers = new Set();

  // Current peers data for search filtering
  var currentPeers = [];

  // Map of peer ID -> friendly label (populated by SSE snapshot)
  var peerLabels = {};
  peerLabels[selfID] = selfName || 'Me';

  // =====================
  // Autorefresh (merged from 10-peers-autorefresh.js)
  // =====================
  var url = new URL(window.location.href);
  if (url.searchParams.get("autorefresh") === "1") {
    setInterval(function() {
      if (!document.hasFocus()) return;
      if (window.Goop && Goop.call && Goop.call.activeCalls().length > 0) return;
      window.location.reload();
    }, 5000);
  }

  // =====================
  // Peers List
  // =====================

  function renderPeerRow(peer) {
    var shortId = peer.ID.substring(0, 8) + '...';
    var lastSeen = new Date(peer.LastSeen).toISOString();
    var hasUnread = unreadPeers.has(peer.ID);

    var avatarSrc = '/api/avatar/peer/' + encodeURIComponent(peer.ID);
    if (peer.AvatarHash) avatarSrc += '?v=' + encodeURIComponent(peer.AvatarHash);

    var peerName = peer.Content || shortId;

    return '<li class="peerrow" data-peer-id="' + escapeHtml(peer.ID) + '">' +
      '<img class="avatar avatar-md" src="' + avatarSrc + '" alt="">' +
      '<div class="peerleft">' +
        '<div class="peer-name-row">' +
          '<a class="peerid" href="/peer/' + escapeHtml(peer.ID) + '">' +
            '<span class="peer-badge" data-unread-badge="' + escapeHtml(peer.ID) + '" style="display:' + (hasUnread ? 'inline' : 'none') + ';">\u25CF</span>' +
            escapeHtml(peerName) +
          '</a>' +
          (peer.Verified ? '' : '<span class="badge-unverified">unverified</span>') +
          (peer.Email ? '<span class="peeremail muted small">' + escapeHtml(peer.Email) + '</span>' : '') +
        '</div>' +
        '<span class="peercontent muted small"><code>' + escapeHtml(shortId) + '</code> &middot; seen ' + escapeHtml(lastSeen) + '</span>' +
      '</div>' +
      '<div class="peerright">' +
        ((peer.VideoDisabled || selfVideoDisabled) ?
        '<span class="peer-video-disabled" title="Video/audio calls disabled">' +
          '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="color:#f39c12;">' +
            '<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>' +
          '</svg>' +
        '</span>'
        :
        '<button class="peer-call-btn" data-call-audio="' + escapeHtml(peer.ID) + '" title="Voice call">' +
          '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.12.8.3 1.58.52 2.34a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.76.22 1.54.4 2.34.52A2 2 0 0 1 22 16.92z"/>' +
          '</svg>' +
        '</button>' +
        '<button class="peer-call-btn" data-call-video="' + escapeHtml(peer.ID) + '" title="Video call">' +
          '<svg width="16" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<polygon points="23 7 16 12 23 17 23 7"/>' +
            '<rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>' +
          '</svg>' +
        '</button>'
        ) +
        '<a class="btn" href="' + escapeHtml(baseURL) + '/p/' + peer.ID + '/" onclick="return openExternal(this.href)" rel="noopener">' +
          'Open Site' +
        '</a>' +
      '</div>' +
      (peer.ActiveTemplate ? '<span class="peer-template-badge">template: ' + escapeHtml(peer.ActiveTemplate) + '</span>' : '') +
    '</li>';
  }

  function renderPeersList(peers) {
    if (peers) {
      currentPeers = peers;
      // Update peer labels map
      peers.forEach(function(peer) {
        peerLabels[peer.ID] = peer.Content || peer.ID.substring(0, 8) + '...';
      });
      peerCount.textContent = '(' + peers.length + ')';
    }

    // Apply search filter
    var query = (peerSearch.value || '').trim().toLowerCase();
    var filtered = currentPeers;
    if (hideUnverified) {
      filtered = filtered.filter(function(peer) { return peer.Verified; });
    }
    if (query) {
      filtered = filtered.filter(function(peer) {
        var name = (peer.Content || '').toLowerCase();
        var id = peer.ID.toLowerCase();
        return name.includes(query) || id.includes(query);
      });
    }

    if (!filtered || filtered.length === 0) {
      if (query && currentPeers.length > 0) {
        peersList.innerHTML = '<p class="muted"><i>No peers match "' + escapeHtml(query) + '"</i></p>';
      } else {
        peersList.innerHTML = '<p class="muted"><i>No peers yet</i></p>';
      }
      return;
    }

    peersList.innerHTML = '<ul class="peers">' + filtered.map(renderPeerRow).join('') + '</ul>';

    // Re-attach click handlers for clearing badges
    document.querySelectorAll('.peerrow').forEach(function(row) {
      var pid = row.getAttribute('data-peer-id');
      row.addEventListener('click', function() {
        unreadPeers.delete(pid);
        var badge = document.querySelector('[data-unread-badge="' + pid + '"]');
        if (badge) badge.style.display = 'none';
      });
    });

    // Attach call button handlers + update busy state
    attachCallButtons();
  }

  // Wire up search
  peerSearch.addEventListener('input', function() {
    renderPeersList(null);
  });

  // Connect to peers SSE
  var peersSSE = new EventSource('/api/peers/events');

  peersSSE.addEventListener('snapshot', function(e) {
    try {
      var data = JSON.parse(e.data);
      if (data.peers) renderPeersList(data.peers);
    } catch (err) {
      console.error('Failed to parse peers snapshot:', err);
    }
  });

  peersSSE.addEventListener('update', function() {
    fetch('/api/peers').then(function(r) { return r.json(); }).then(renderPeersList).catch(console.error);
  });

  peersSSE.addEventListener('remove', function() {
    fetch('/api/peers').then(function(r) { return r.json(); }).then(renderPeersList).catch(console.error);
  });

  peersSSE.onerror = function() {
    console.error('Peers SSE connection lost');
  };

  // =====================
  // Call Buttons
  // =====================

  var busyPeers = new Set();

  function attachCallButtons() {
    document.querySelectorAll('.peer-call-btn').forEach(function(btn) {
      var audioId = btn.getAttribute('data-call-audio');
      var videoId = btn.getAttribute('data-call-video');
      var peerId = audioId || videoId;
      if (peerId && busyPeers.has(peerId)) {
        btn.disabled = true;
        btn.classList.add('busy');
        btn.title = 'In a call';
      }
    });

    document.querySelectorAll('[data-call-audio]').forEach(function(btn) {
      btn.onclick = function(e) {
        e.stopPropagation();
        var pid = btn.getAttribute('data-call-audio');
        startPeerCall(pid, { audio: true, video: false });
      };
    });

    document.querySelectorAll('[data-call-video]').forEach(function(btn) {
      btn.onclick = function(e) {
        e.stopPropagation();
        var pid = btn.getAttribute('data-call-video');
        startPeerCall(pid, { audio: true, video: true });
      };
    });
  }

  function startPeerCall(peerId, constraints) {
    if (!window.Goop || !window.Goop.callUI) {
      alert('Call feature not available');
      return;
    }
    if (busyPeers.has(peerId)) return;

    busyPeers.add(peerId);
    updateBusyState();

    Goop.callUI.startCall(peerId, constraints).then(function(session) {
      session.onHangup(function() {
        busyPeers.delete(peerId);
        updateBusyState();
      });
    }).catch(function(err) {
      console.error('Call failed:', err);
      busyPeers.delete(peerId);
      updateBusyState();
    });
  }

  function updateBusyState() {
    document.querySelectorAll('.peer-call-btn').forEach(function(btn) {
      var audioId = btn.getAttribute('data-call-audio');
      var videoId = btn.getAttribute('data-call-video');
      var peerId = audioId || videoId;
      if (!peerId) return;

      if (busyPeers.has(peerId)) {
        btn.disabled = true;
        btn.classList.add('busy');
        btn.title = 'In a call';
      } else {
        btn.disabled = false;
        btn.classList.remove('busy');
        btn.title = audioId ? 'Voice call' : 'Video call';
      }
    });
  }

  // Attach handlers for server-rendered rows
  attachCallButtons();

  // =====================
  // Broadcast Chat
  // =====================

  function loadBroadcasts() {
    api('/api/chat/broadcasts')
      .then(function(messages) { renderBroadcasts(messages); })
      .catch(function(err) {
        console.error('Failed to load broadcasts:', err);
        messagesDiv.innerHTML = '<p class="error">Failed to load messages</p>';
      });
  }

  function renderBroadcasts(messages) {
    if (!messages || messages.length === 0) {
      messagesDiv.innerHTML = '<p class="muted"><i>No broadcast messages yet. Say hello!</i></p>';
      return;
    }

    messagesDiv.innerHTML = messages.map(function(msg) {
      var time = new Date(msg.timestamp).toLocaleString();
      var isOutgoing = msg.from === selfID;
      var className = isOutgoing ? 'msg-out' : 'msg-in';
      var senderName = peerLabels[msg.from] || msg.from.substring(0, 8) + '...';
      var avatarUrl = isOutgoing
        ? '/api/avatar'
        : '/api/avatar/peer/' + encodeURIComponent(msg.from);
      var converted = emojify(msg.content);
      var emojiOnly = isEmojiOnly(converted) ? ' msg-emoji-only' : '';

      return '<div class="chat-msg ' + className + '">' +
        '<img class="avatar avatar-xs chat-msg-avatar" src="' + avatarUrl + '" alt="">' +
        '<div class="chat-msg-body">' +
          '<div class="msg-sender">' + escapeHtml(senderName) + '</div>' +
          '<div class="msg-content' + emojiOnly + '">' + escapeHtml(converted) + '</div>' +
          '<div class="msg-time">' + time + '</div>' +
        '</div>' +
      '</div>';
    }).join('');

    messagesDiv.scrollTop = messagesDiv.scrollHeight;
  }

  // Send broadcast
  form.addEventListener('submit', function(e) {
    e.preventDefault();
    var content = input.value.trim();
    if (!content) return;

    // Detect Lua commands typed in broadcast by mistake
    if (content.startsWith('!')) {
      if (window.Goop && window.Goop.toast) {
        Goop.toast({
          icon: '\u26A0\uFE0F',
          title: 'Wrong chat',
          message: 'Lua commands like <b>' + escapeHtml(content.split(' ')[0]) + '</b> only work in direct chat with a peer.',
          duration: 5000
        });
      }
      return;
    }

    api('/api/chat/broadcast', { content: content })
    .then(function() {
      input.value = '';
      loadBroadcasts();
    })
    .catch(function(err) {
      console.error('Failed to send:', err);
      alert('Failed to send broadcast');
    });
  });

  // Listen for new messages via SSE (both direct and broadcast)
  var chatSSE = new EventSource('/api/chat/events');

  chatSSE.addEventListener('message', function(e) {
    try {
      var msg = JSON.parse(e.data);
      var msgType = msg.type || 'direct';

      if (msgType === 'broadcast') {
        loadBroadcasts();
      } else if (msgType === 'direct' && msg.from && msg.from !== selfID) {
        unreadPeers.add(msg.from);
        var badge = document.querySelector('[data-unread-badge="' + msg.from + '"]');
        if (badge) badge.style.display = 'inline';
      }
    } catch (err) {
      console.error('Failed to parse message:', err);
    }
  });

  chatSSE.onerror = function() {
    console.error('Chat SSE connection lost');
  };

  var escapeHtml = Goop.core.escapeHtml;

  function emojify(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.emojify(text) : text;
  }

  function isEmojiOnly(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.isEmojiOnly(text) : false;
  }

  // Initial load
  loadBroadcasts();
})();
