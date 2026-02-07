// internal/ui/assets/js/goop-call-ui.js
//
// Call UI overlay — incoming call modal, active call controls.
// Requires goop-call.js to be loaded first.
//
// Usage:
//   <script src="/assets/js/goop-realtime.js"></script>
//   <script src="/assets/js/goop-call.js"></script>
//   <script src="/assets/js/goop-call-ui.js"></script>
//
//   // The UI auto-registers for incoming calls and renders overlays.
//   // To manually trigger from a button:
//   Goop.callUI.startCall(peerId);
//
(() => {
  window.Goop = window.Goop || {};

  // Logging helper
  function log(level, msg) {
    if (Goop.log && Goop.log[level]) {
      Goop.log[level]('call-ui', msg);
    } else {
      var fn = console[level] || console.log;
      fn('[call-ui]', msg);
    }
  }

  var currentSession = null;
  var overlayEl = null;
  var incomingEl = null;
  var styleInjected = false;

  // ── CSS injection ───────────────────────────────────────────────────────────

  function injectStyles() {
    if (styleInjected) return;
    styleInjected = true;

    var css = [
      ".goop-call-overlay {",
      "  position: fixed; bottom: 16px; right: 16px; z-index: 10000;",
      "  background: #1a1a2e; border: 1px solid #333; border-radius: 12px;",
      "  padding: 12px; display: flex; flex-direction: column; gap: 8px;",
      "  box-shadow: 0 8px 32px rgba(0,0,0,0.4); max-width: 320px;",
      "  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;",
      "  color: #e0e0e0; font-size: 13px;",
      "}",
      ".goop-call-videos { position: relative; }",
      ".goop-call-videos video {",
      "  border-radius: 8px; background: #000; object-fit: cover;",
      "}",
      ".goop-call-remote { width: 240px; height: 180px; }",
      ".goop-call-local {",
      "  width: 80px; height: 60px; position: absolute; bottom: 8px; right: 6px;",
      "  border: 1px solid rgba(255,255,255,0.2); border-radius: 6px; z-index: 1;",
      "}",
      ".goop-call-controls {",
      "  display: flex; gap: 8px; justify-content: center; padding-top: 4px;",
      "}",
      ".goop-call-btn {",
      "  border: none; border-radius: 50%; width: 40px; height: 40px;",
      "  cursor: pointer; font-size: 18px; display: flex; align-items: center;",
      "  justify-content: center; transition: opacity 0.2s;",
      "}",
      ".goop-call-btn:hover { opacity: 0.8; }",
      ".goop-call-btn-hangup { background: #e74c3c; color: #fff; }",
      ".goop-call-btn-mute { background: #333; color: #fff; }",
      ".goop-call-btn-mute.active { background: #e74c3c; }",
      ".goop-call-btn-video { background: #333; color: #fff; }",
      ".goop-call-btn-video.active { background: #e74c3c; }",
      ".goop-call-status {",
      "  text-align: center; font-size: 11px; color: #7a8194;",
      "}",
      "",
      ".goop-call-incoming {",
      "  position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%);",
      "  z-index: 10001; background: #1a1a2e; border: 1px solid #444;",
      "  border-radius: 16px; padding: 24px 32px; text-align: center;",
      "  box-shadow: 0 16px 48px rgba(0,0,0,0.6);",
      "  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;",
      "  color: #e0e0e0;",
      "}",
      ".goop-call-incoming h3 { margin: 0 0 8px; font-size: 16px; }",
      ".goop-call-incoming p { margin: 0 0 16px; color: #7a8194; font-size: 13px; }",
      ".goop-call-incoming-actions { display: flex; gap: 16px; justify-content: center; }",
      ".goop-call-incoming-btn {",
      "  border: none; border-radius: 24px; padding: 10px 24px;",
      "  cursor: pointer; font-size: 14px; font-weight: 500;",
      "}",
      ".goop-call-accept { background: #16c784; color: #fff; }",
      ".goop-call-reject { background: #e74c3c; color: #fff; }",
      ".goop-call-backdrop {",
      "  position: fixed; inset: 0; z-index: 9999;",
      "  background: rgba(0,0,0,0.5);",
      "}"
    ].join("\n");

    var style = document.createElement("style");
    style.textContent = css;
    document.head.appendChild(style);
  }

  // ── Incoming call modal ─────────────────────────────────────────────────────

  function showIncomingCall(info) {
    log('info', 'Incoming call UI: peerId=' + info.peerId + ', channelId=' + info.channelId);
    injectStyles();
    removeIncoming();

    var backdrop = document.createElement("div");
    backdrop.className = "goop-call-backdrop";

    var modal = document.createElement("div");
    modal.className = "goop-call-incoming";
    modal.innerHTML =
      '<h3>Incoming Call</h3>' +
      '<p>From: ' + escapeHtml(info.peerId || "Unknown peer") + '</p>' +
      '<div class="goop-call-incoming-actions">' +
        '<button class="goop-call-incoming-btn goop-call-accept">Accept</button>' +
        '<button class="goop-call-incoming-btn goop-call-reject">Reject</button>' +
      '</div>';

    modal.querySelector(".goop-call-accept").onclick = async function() {
      removeIncoming();
      try {
        var session = await info.accept();
        showActiveCall(session);
      } catch(e) {
        console.error("Failed to accept call:", e);
      }
    };

    modal.querySelector(".goop-call-reject").onclick = function() {
      removeIncoming();
      info.reject();
    };

    incomingEl = { backdrop: backdrop, modal: modal };
    document.body.appendChild(backdrop);
    document.body.appendChild(modal);
  }

  function removeIncoming() {
    if (!incomingEl) return;
    if (incomingEl.backdrop.parentNode) incomingEl.backdrop.parentNode.removeChild(incomingEl.backdrop);
    if (incomingEl.modal.parentNode) incomingEl.modal.parentNode.removeChild(incomingEl.modal);
    incomingEl = null;
  }

  // ── Soft navigation (keeps call alive across page changes) ─────────────────

  var softNavInstalled = false;

  function softNavHandler(e) {
    var a = e.target.closest ? e.target.closest('a') : null;
    if (!a) return;

    // Skip if no active call
    if (!currentSession) return;

    // Skip modifier-key clicks (new tab, etc.)
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;

    var href = a.getAttribute('href');
    if (!href) return;

    // Skip non-navigational links
    if (href.startsWith('#') || href.startsWith('javascript:')) return;

    // Skip external links
    try {
      var target = new URL(href, window.location.origin);
      if (target.origin !== window.location.origin) return;
    } catch(_) {
      return;
    }

    // Skip links with explicit target
    if (a.target && a.target !== '_self') return;

    e.preventDefault();
    e.stopPropagation();
    softNavigateTo(target.href);
  }

  function softNavigateTo(url) {
    fetch(url, { credentials: 'same-origin' })
      .then(function(resp) {
        if (!resp.ok) {
          log('warn', 'Soft nav fetch failed: ' + resp.status);
          return;
        }
        return resp.text();
      })
      .then(function(html) {
        if (!html) return;

        var doc = new DOMParser().parseFromString(html, 'text/html');

        // Swap content
        var newContent = doc.querySelector('.content');
        var curContent = document.querySelector('.content');
        if (newContent && curContent) {
          curContent.innerHTML = newContent.innerHTML;

          // Re-execute inline scripts from new content
          var scripts = curContent.querySelectorAll('script');
          for (var i = 0; i < scripts.length; i++) {
            var oldScript = scripts[i];
            var newScript = document.createElement('script');
            if (oldScript.src) {
              newScript.src = oldScript.src;
            } else {
              newScript.textContent = oldScript.textContent;
            }
            oldScript.parentNode.replaceChild(newScript, oldScript);
          }
        }

        // Update URL
        history.pushState(null, '', url);

        // Update title
        var newTitle = doc.querySelector('title');
        if (newTitle) {
          document.title = newTitle.textContent;
        }

        // Update nav active state
        var navItems = document.querySelectorAll('.topnav .navitem');
        for (var j = 0; j < navItems.length; j++) {
          var item = navItems[j];
          var itemHref = item.getAttribute('href');
          if (itemHref && new URL(url).pathname.startsWith(itemHref)) {
            item.classList.add('active');
          } else {
            item.classList.remove('active');
          }
        }
      })
      .catch(function(err) {
        log('error', 'Soft nav error: ' + err.message);
      });
  }

  function softNavPopState() {
    if (!currentSession) return;
    softNavigateTo(window.location.href);
  }

  function installSoftNav() {
    if (softNavInstalled) return;
    softNavInstalled = true;
    document.addEventListener('click', softNavHandler, true);
    window.addEventListener('popstate', softNavPopState);
    log('info', 'Soft navigation installed (call active)');
  }

  function removeSoftNav() {
    if (!softNavInstalled) return;
    softNavInstalled = false;
    document.removeEventListener('click', softNavHandler, true);
    window.removeEventListener('popstate', softNavPopState);
    log('info', 'Soft navigation removed');
  }

  // ── Active call overlay ─────────────────────────────────────────────────────

  function showActiveCall(session) {
    log('info', 'Showing active call UI for channel: ' + session.channelId);
    injectStyles();
    removeOverlay();

    currentSession = session;

    var el = document.createElement("div");
    el.className = "goop-call-overlay";
    el.innerHTML =
      '<div class="goop-call-videos" style="position:relative;">' +
        '<video class="goop-call-remote" autoplay playsinline></video>' +
        '<video class="goop-call-local" autoplay playsinline muted></video>' +
      '</div>' +
      '<div class="goop-call-status">Connecting...</div>' +
      '<div class="goop-call-controls">' +
        '<button class="goop-call-btn goop-call-btn-mute" title="Toggle Mute">' +
          '<svg class="icon-on" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>' +
            '<path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" y1="19" x2="12" y2="23"/>' +
            '<line x1="8" y1="23" x2="16" y2="23"/>' +
          '</svg>' +
          '<svg class="icon-off" style="display:none;" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<line x1="1" y1="1" x2="23" y2="23"/>' +
            '<path d="M9 9v3a3 3 0 0 0 5.12 2.12M15 9.34V4a3 3 0 0 0-5.94-.6"/>' +
            '<path d="M17 16.95A7 7 0 0 1 5 12v-2m14 0v2c0 .76-.13 1.49-.35 2.17"/>' +
            '<line x1="12" y1="19" x2="12" y2="23"/><line x1="8" y1="23" x2="16" y2="23"/>' +
          '</svg>' +
        '</button>' +
        '<button class="goop-call-btn goop-call-btn-hangup" title="Hang Up">' +
          '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<path d="M10.68 13.31a16 16 0 0 0 3.41 2.6l1.27-1.27a2 2 0 0 1 2.11-.45c.76.22 1.54.4 2.34.52A2 2 0 0 1 22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6A19.79 19.79 0 0 1 2.12 4.18 2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.12.8.3 1.58.52 2.34a2 2 0 0 1-.45 2.11L8.09 9.91"/>' +
            '<line x1="23" y1="1" x2="1" y2="23"/>' +
          '</svg>' +
        '</button>' +
        '<button class="goop-call-btn goop-call-btn-video" title="Toggle Video">' +
          '<svg class="icon-on" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<polygon points="23 7 16 12 23 17 23 7"/>' +
            '<rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>' +
          '</svg>' +
          '<svg class="icon-off" style="display:none;" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<path d="M16 16v1a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h2m5.66 0H14a2 2 0 0 1 2 2v3.34l1 1L23 7v10"/>' +
            '<line x1="1" y1="1" x2="23" y2="23"/>' +
          '</svg>' +
        '</button>' +
      '</div>';

    var remoteVideo = el.querySelector(".goop-call-remote");
    var localVideo = el.querySelector(".goop-call-local");
    var statusEl = el.querySelector(".goop-call-status");
    var muteBtn = el.querySelector(".goop-call-btn-mute");
    var hangupBtn = el.querySelector(".goop-call-btn-hangup");
    var videoBtn = el.querySelector(".goop-call-btn-video");

    // Show local video
    if (session.localStream) {
      localVideo.srcObject = session.localStream;
    }

    // Show remote video when available
    session.onRemoteStream(function(stream) {
      var trackInfo = stream.getTracks().map(function(t) { return t.kind + ":" + t.readyState + ":enabled=" + t.enabled; }).join(", ");
      log('info', 'Remote stream received in UI! tracks=[' + trackInfo + ']');

      log('debug', 'Setting remote video srcObject...');
      remoteVideo.srcObject = stream;
      log('debug', 'srcObject set, video element: readyState=' + remoteVideo.readyState + ', networkState=' + remoteVideo.networkState);

      // Explicitly try to play in case autoplay is blocked
      remoteVideo.play().then(function() {
        log('info', 'Remote video playing successfully');
      }).catch(function(e) {
        log('warn', 'Remote video autoplay blocked: ' + e.message);
      });

      // Monitor video element
      remoteVideo.onloadedmetadata = function() {
        log('info', 'Remote video metadata loaded: ' + remoteVideo.videoWidth + 'x' + remoteVideo.videoHeight);
      };
      remoteVideo.onplaying = function() {
        log('info', 'Remote video is now playing');
      };
      remoteVideo.onerror = function(e) {
        log('error', 'Remote video error: ' + (e.message || 'unknown'));
      };

      statusEl.textContent = "Connected";
    });

    session.onStateChange(function(state) {
      statusEl.textContent = state === "connected" ? "Connected" :
                             state === "connecting" ? "Connecting..." :
                             state;
    });

    session.onHangup(function() {
      removeOverlay();
    });

    muteBtn.onclick = function() {
      var enabled = session.toggleAudio();
      muteBtn.classList.toggle("active", !enabled);
      muteBtn.querySelector(".icon-on").style.display = enabled ? "" : "none";
      muteBtn.querySelector(".icon-off").style.display = enabled ? "none" : "";
    };

    videoBtn.onclick = function() {
      var enabled = session.toggleVideo();
      videoBtn.classList.toggle("active", !enabled);
      videoBtn.querySelector(".icon-on").style.display = enabled ? "" : "none";
      videoBtn.querySelector(".icon-off").style.display = enabled ? "none" : "";
    };

    hangupBtn.onclick = function() {
      session.hangup();
    };

    overlayEl = el;
    document.body.appendChild(el);
    installSoftNav();
  }

  function removeOverlay() {
    var wasActive = !!overlayEl;
    if (overlayEl && overlayEl.parentNode) {
      overlayEl.parentNode.removeChild(overlayEl);
    }
    overlayEl = null;
    currentSession = null;
    if (wasActive && softNavInstalled) {
      removeSoftNav();
      window.location.reload();
    }
  }

  function escapeHtml(s) {
    if (!s) return "";
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }

  // ── Auto-register for incoming calls ────────────────────────────────────────

  if (Goop.call) {
    Goop.call.onIncoming(function(info) {
      showIncomingCall(info);
    });
  }

  // ── Public API ──────────────────────────────────────────────────────────────

  Goop.callUI = {
    // Convenience: start a call and show the UI
    startCall: async function(peerId, constraints) {
      log('info', 'CallUI.startCall: peerId=' + peerId);
      try {
        var session = await Goop.call.start(peerId, constraints);
        showActiveCall(session);
        return session;
      } catch(e) {
        log('error', 'Failed to start call: ' + e.message);
        throw e;
      }
    },

    // Get the current active session (if any)
    activeSession: function() {
      return currentSession;
    },

    // Manually show call UI for an existing session
    showCall: function(session) {
      showActiveCall(session);
    }
  };
})();
