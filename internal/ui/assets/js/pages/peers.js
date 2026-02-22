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
  var peerSearchModeSelector = document.getElementById('peer-search-mode-selector');
  var peerSearchModeTrigger = document.getElementById('peer-search-mode-trigger');
  var messagesDiv = document.getElementById('broadcast-messages');
  var form = document.getElementById('broadcast-form');
  var input = document.getElementById('broadcast-input');
  var addressbookToggle = document.getElementById('addressbook-toggle');
  var peerCtxMenu = document.getElementById('peer-ctx-menu');

  // Track unread direct messages per peer
  var unreadPeers = new Set();

  // Current peers data for search filtering
  var currentPeers = [];

  // Address book mode
  var addrBookMode = false;

  // Search mode
  var searchMode = 'name';

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

    // peer.Offline = explicitly gone (OfflineSince set) → full inert + dimmed.
    // peer.Reachable === false = probe failed or not yet run → just dim; call may
    // still succeed via relay so we keep the row interactive.
    var rowOffline = !!peer.Offline;
    var rowDimmed  = rowOffline || peer.Reachable === false;
    var rowClass   = rowDimmed ? ' dimmed' : '';

    return '<li class="peerrow' + rowClass + '" data-peer-id="' + escapeHtml(peer.ID) + '" data-favorite="' + (peer.Favorite ? 'true' : 'false') + '"' + (rowOffline ? ' inert' : '') + '>' +
      '<img class="avatar avatar-md" src="' + avatarSrc + '" alt="">' +
      '<div class="peerleft">' +
        '<div class="peer-name-row">' +
          '<a class="peerid" href="/peer/' + escapeHtml(peer.ID) + '">' +
            '<span class="peer-badge' + (hasUnread ? '' : ' hidden') + '" data-unread-badge="' + escapeHtml(peer.ID) + '">\u25CF</span>' +
            escapeHtml(peerName) +
            (peer.Favorite ? '<span class="peer-fav-star">★</span>' : '') +
          '</a>' +
          (peer.Verified ? '' : '<span class="badge-unverified">unverified</span>') +
          (peer.Email ? '<span class="peeremail muted small">' + escapeHtml(peer.Email) + '</span>' : '') +
        '</div>' +
        '<span class="peercontent muted small"><code>' + escapeHtml(shortId) + '</code> &middot; seen ' + escapeHtml(lastSeen) + '</span>' +
      '</div>' +
      '<div class="peerright">' +
        core.callButtonsHTML(peer.ID, core.callDisabledReason(peer.VideoDisabled, selfVideoDisabled)) +
        '<a class="btn" href="' + escapeHtml(baseURL) + '/p/' + peer.ID + '/" onclick="return openSite(\'' + escapeAttr(peer.ID) + '\', \'' + escapeAttr(peerName) + '\')" rel="noopener">' +
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
    if (addrBookMode) {
      filtered = filtered.filter(function(peer) { return peer.Favorite === true; });
    }
    if (hideUnverified) {
      filtered = filtered.filter(function(peer) { return peer.Verified; });
    }
    if (query) {
      filtered = filtered.filter(function(peer) {
        if (searchMode === 'id') {
          var id = peer.ID.toLowerCase();
          return id.includes(query);
        } else {
          var name = (peer.Content || '').toLowerCase();
          return name.includes(query);
        }
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
        if (badge) badge.classList.add('hidden');
      });
    });

    // Attach call button handlers + update busy state
    attachCallButtons();
  }

  // Wire up search
  peerSearch.addEventListener('input', function() {
    renderPeersList(null);
  });

  // Wire up search mode dropdown (using Goop.select API)
  if (peerSearchModeSelector && window.Goop && window.Goop.select) {
    Goop.select.init(peerSearchModeSelector, function(newMode) {
      searchMode = newMode;
      renderPeersList(null);
    });
  }

  // Wire up address book toggle
  if (addressbookToggle) {
    addressbookToggle.addEventListener('change', function() {
      addrBookMode = this.checked;
      renderPeersList(null);
    });
  }

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

  peersSSE.addEventListener('update', function(e) {
    try {
      var data = JSON.parse(e.data);
      if (data.peer_id && data.peer) {
        var idx = currentPeers.findIndex(function(p) { return p.ID === data.peer_id; });
        if (idx >= 0) {
          currentPeers[idx] = data.peer;
        } else {
          currentPeers.push(data.peer);
        }
        renderPeersList(null);
      }
    } catch (err) { console.error('Failed to parse peer update:', err); }
  });

  peersSSE.addEventListener('remove', function(e) {
    try {
      var data = JSON.parse(e.data);
      if (data.peer_id) {
        currentPeers = currentPeers.filter(function(p) { return p.ID !== data.peer_id; });
        renderPeersList(null);
      }
    } catch (err) { console.error('Failed to parse peer remove:', err); }
  });

  peersSSE.onerror = function() {
    console.error('Peers SSE connection lost');
  };

  // =====================
  // Context Menu
  // =====================

  var ctxMenuTarget = null;

  if (peerCtxMenu) {
    // Show context menu on right-click of peer row
    document.addEventListener('contextmenu', function(e) {
      var peerRow = e.target.closest('.peerrow');
      if (!peerRow) return;

      e.preventDefault();
      ctxMenuTarget = peerRow.getAttribute('data-peer-id');
      if (!ctxMenuTarget) return;

      var peer = currentPeers.find(function(p) { return p.ID === ctxMenuTarget; });
      if (!peer) return;

      // Show/hide buttons based on favorite state
      var favBtn = peerCtxMenu.querySelector('[data-action="favorite"]');
      var unfavBtn = peerCtxMenu.querySelector('[data-action="unfavorite"]');
      if (peer.Favorite) {
        if (favBtn) favBtn.style.display = 'none';
        if (unfavBtn) unfavBtn.style.display = 'block';
      } else {
        if (favBtn) favBtn.style.display = 'block';
        if (unfavBtn) unfavBtn.style.display = 'none';
      }

      // Position menu
      peerCtxMenu.classList.remove('hidden');
      peerCtxMenu.style.position = 'fixed';
      peerCtxMenu.style.left = Math.min(e.clientX, window.innerWidth - 200) + 'px';
      peerCtxMenu.style.top = Math.min(e.clientY, window.innerHeight - 100) + 'px';
    });

    // Hide menu on click or escape
    document.addEventListener('click', function(e) {
      if (!e.target.closest('#peer-ctx-menu')) {
        peerCtxMenu.classList.add('hidden');
      }
    });

    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') {
        peerCtxMenu.classList.add('hidden');
      }
    });

    // Handle menu actions
    peerCtxMenu.querySelectorAll('button').forEach(function(btn) {
      btn.addEventListener('click', function(e) {
        e.preventDefault();
        var action = this.getAttribute('data-action');
        if (!ctxMenuTarget) return;

        var isFav = action === 'favorite';
        api('/api/peers/favorite', { peer_id: ctxMenuTarget, favorite: isFav })
          .then(function() {
            // Update local peer state immediately
            var idx = currentPeers.findIndex(function(p) { return p.ID === ctxMenuTarget; });
            if (idx >= 0) {
              currentPeers[idx].Favorite = isFav;
              renderPeersList(null);
            }
          })
          .catch(function(err) {
            console.error('Failed to toggle favorite:', err);
          });

        peerCtxMenu.classList.add('hidden');
      });
    });
  }

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
    if (busyPeers.has(peerId)) {
      console.warn('[peers] call button: peer already busy', peerId.substring(0, 8));
      return;
    }

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
        if (badge) badge.classList.remove('hidden');
      }
    } catch (err) {
      console.error('Failed to parse message:', err);
    }
  });

  chatSSE.onerror = function() {
    console.error('Chat SSE connection lost');
  };

  var escapeHtml = Goop.core.escapeHtml;

  function escapeAttr(s) {
    return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function emojify(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.emojify(text) : text;
  }

  function isEmojiOnly(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.isEmojiOnly(text) : false;
  }

  // Initial load
  loadBroadcasts();

  // Reachability: probe peers and render the result directly.
  var probing = false;
  function triggerProbe() {
    if (probing) return;
    probing = true;
    api('/api/peers/probe', {})
      .then(function(peers) { if (peers) renderPeersList(peers); })
      .catch(function() {})
      .then(function() { probing = false; });
  }

  triggerProbe();
  var probeInterval = setInterval(triggerProbe, 5000);

  // Clean up polling when navigating away
  window.addEventListener('beforeunload', function() {
    if (probeInterval) clearInterval(probeInterval);
    if (peersSSE) peersSSE.close();
    if (chatSSE) chatSSE.close();
  });
})();
