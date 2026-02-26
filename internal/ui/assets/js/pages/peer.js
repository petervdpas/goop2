// Peer page: chat messages, send handler, MQ subscription, call buttons.
(function() {
  var pageEl = document.querySelector('.peer-page');
  if (!pageEl) return;

  var core = window.Goop && window.Goop.core || {};
  var escapeHtml = core.escapeHtml || function(s) { return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); };

  var peerID = pageEl.dataset.peerId;
  if (!peerID) return;

  var selfID = document.body.dataset.selfId || '';

  var messagesDiv = document.getElementById('chat-messages');
  var form = document.getElementById('chat-form');
  var input = document.getElementById('chat-message');

  // ── In-memory message list (session only, like the old ring buffer) ──
  var _messages = [];

  function addMessage(msg) {
    _messages.push(msg);
    renderMessages(_messages);
  }

  // Load peer name
  var peerNameEl = document.getElementById('peer-name');
  fetch('/api/peer/content?id=' + encodeURIComponent(peerID))
    .then(function(res) { return res.json(); })
    .then(function(data) {
      peerNameEl.textContent = data.content || peerID;
    })
    .catch(function() { peerNameEl.textContent = peerID; });

  // ── Render messages ──
  function renderMessages(messages) {
    if (!messages || messages.length === 0) {
      messagesDiv.innerHTML = '<p class="muted"><i>No messages yet. Start a conversation!</i></p>';
      return;
    }

    messagesDiv.innerHTML = messages.map(function(msg) {
      var time = new Date(msg.timestamp).toLocaleString();
      var isOutgoing = msg.from !== peerID;
      var className = isOutgoing ? 'msg-out' : 'msg-in';
      var avatarUrl = isOutgoing
        ? '/api/avatar'
        : '/api/avatar/peer/' + encodeURIComponent(peerID);
      var converted = emojify(msg.content);
      var emojiOnly = isEmojiOnly(converted) ? ' msg-emoji-only' : '';
      return '<div class="chat-msg ' + className + '">' +
        '<img class="avatar avatar-xs chat-msg-avatar" src="' + avatarUrl + '" alt="">' +
        '<div class="chat-msg-body">' +
          '<div class="msg-content' + emojiOnly + '">' + escapeHtml(converted) + '</div>' +
          '<button class="msg-copy" data-text="' + escapeHtml(msg.content).replace(/"/g, '&quot;') + '" title="Copy">Copy</button>' +
          '<div class="msg-time">' + time + '</div>' +
        '</div>' +
      '</div>';
    }).join('');

    // Wire copy buttons
    messagesDiv.querySelectorAll('.msg-copy').forEach(function(btn) {
      btn.addEventListener('click', function() {
        var text = btn.getAttribute('data-text');
        navigator.clipboard.writeText(text).then(function() {
          btn.textContent = 'Copied!';
          setTimeout(function() { btn.textContent = 'Copy'; }, 1500);
        }).catch(function() {
          btn.textContent = 'Failed';
          setTimeout(function() { btn.textContent = 'Copy'; }, 1500);
        });
      });
    });

    messagesDiv.scrollTop = messagesDiv.scrollHeight;
  }

  // ── Send message ──
  form.addEventListener('submit', function(e) {
    e.preventDefault();
    var content = input.value.trim();
    if (!content) return;
    if (!window.Goop || !window.Goop.mq) { alert('MQ not available'); return; }

    var outMsg = { from: selfID, to: peerID, content: content, timestamp: Date.now() };
    addMessage(outMsg);
    input.value = '';

    window.Goop.mq.sendChat(peerID, { content: content, to: peerID })
      .catch(function(err) {
        Goop.log.error('peer', 'chat send failed: ' + err);
        alert('Failed to send message');
      });
  });

  // ── Subscribe to incoming chat for this peer ──
  function initMQChat() {
    if (!window.Goop || !window.Goop.mq) {
      setTimeout(initMQChat, 100);
      return;
    }
    window.Goop.mq.onChat( function(from, _topic, payload, ack) {
      // Show messages from this peer only
      if (from !== peerID) { ack(); return; }
      var msg = { from: from, to: selfID, content: payload.content || '', timestamp: Date.now() };
      addMessage(msg);
      ack();
    });
  }
  initMQChat();

  // ── Call buttons ──
  var callActionsEl = document.querySelector('.chat-call-actions');
  if (callActionsEl) {
    var peerVid = callActionsEl.dataset.peerVideoDisabled === 'true';
    var selfVid = callActionsEl.dataset.selfVideoDisabled === 'true';
    var reason  = core.callDisabledReason(peerVid, selfVid);
    callActionsEl.innerHTML = core.callButtonsHTML(peerID, reason, { cls: 'call-btn', audioId: 'btn-audio-call', videoId: 'btn-video-call', large: true });
  }
  var audioBtn = document.getElementById('btn-audio-call');
  var videoBtn = document.getElementById('btn-video-call');
  var inCall = false;

  function setCallBusy(busy) {
    inCall = busy;
    if (audioBtn) { audioBtn.disabled = busy; audioBtn.classList.toggle('busy', busy); }
    if (videoBtn) { videoBtn.disabled = busy; videoBtn.classList.toggle('busy', busy); }
  }

  // Check if already in a call with this peer on load
  if (window.Goop && window.Goop.call) {
    var existing = Goop.call.activeCalls();
    for (var i = 0; i < existing.length; i++) {
      if (existing[i].remotePeer === peerID) {
        setCallBusy(true);
        existing[i].onHangup(function() { setCallBusy(false); });
        break;
      }
    }
  }

  function startCall(constraints) {
    if (!window.Goop || !window.Goop.callUI) {
      alert('Call feature not available');
      return;
    }
    if (inCall) return;
    setCallBusy(true);

    Goop.callUI.startCall(peerID, constraints).then(function(session) {
      session.onHangup(function() { setCallBusy(false); });
    }).catch(function(err) {
      Goop.log.error('peer', 'call failed: ' + err);
      setCallBusy(false);
    });
  }

  if (audioBtn) {
    audioBtn.addEventListener('click', function() {
      startCall({ audio: true, video: false });
    });
  }

  if (videoBtn) {
    videoBtn.addEventListener('click', function() {
      startCall({ audio: true, video: true });
    });
  }

  function emojify(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.emojify(text) : text;
  }

  function isEmojiOnly(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.isEmojiOnly(text) : false;
  }

  // Load any messages received while we were on a different page.
  try {
    var key = 'goop:chat:' + peerID;
    var cached = JSON.parse(sessionStorage.getItem(key) || '[]');
    cached.forEach(function(msg) { _messages.push(msg); });
    sessionStorage.removeItem(key);
  } catch (_) {}
  renderMessages(_messages);
})();
