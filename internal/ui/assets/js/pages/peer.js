// Peer page: chat messages, send handler, SSE, call buttons.
(function() {
  var pageEl = document.querySelector('.peer-page');
  if (!pageEl) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  var peerID = pageEl.dataset.peerId;
  if (!peerID) return;

  var messagesDiv = document.getElementById('chat-messages');
  var form = document.getElementById('chat-form');
  var input = document.getElementById('chat-message');

  // Load peer content (display name) in background
  var peerNameEl = document.getElementById('peer-name');
  fetch('/api/peer/content?id=' + encodeURIComponent(peerID))
    .then(function(res) { return res.json(); })
    .then(function(data) {
      if (data.content) {
        peerNameEl.textContent = data.content;
      } else {
        peerNameEl.textContent = peerID;
      }
    })
    .catch(function() {
      peerNameEl.textContent = peerID;
    });

  // Load existing messages
  function loadMessages() {
    api('/api/chat/messages?peer=' + encodeURIComponent(peerID))
      .then(function(messages) { renderMessages(messages); })
      .catch(function(err) {
        console.error('Failed to load messages:', err);
        messagesDiv.innerHTML = '<p class="error">Failed to load messages</p>';
      });
  }

  // Render messages
  function renderMessages(messages) {
    if (messages.length === 0) {
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

    // Scroll to bottom
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
  }

  // Send message
  form.addEventListener('submit', function(e) {
    e.preventDefault();
    var content = input.value.trim();
    if (!content) return;

    api('/api/chat/send', { to: peerID, content: content })
    .then(function() {
      input.value = '';
      loadMessages();
    })
    .catch(function(err) {
      console.error('Failed to send:', err);
      alert('Failed to send message');
    });
  });

  // Listen for new messages via SSE
  var eventSource = new EventSource('/api/chat/events');
  eventSource.addEventListener('message', function(e) {
    var msg = JSON.parse(e.data);
    if (msg.from === peerID || msg.to === peerID) {
      loadMessages();
    }
  });

  eventSource.onerror = function() {
    console.error('SSE connection lost, reconnecting...');
  };

  var escapeHtml = Goop.core.escapeHtml;

  function emojify(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.emojify(text) : text;
  }

  function isEmojiOnly(text) {
    return window.Goop && window.Goop.emoji ? window.Goop.emoji.isEmojiOnly(text) : false;
  }

  // ── Call buttons ──
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
      console.error('Call failed:', err);
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

  // Initial load
  loadMessages();
})();
