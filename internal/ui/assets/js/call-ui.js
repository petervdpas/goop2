//
// call-ui.js — incoming call modal and active call overlay for the goop2 viewer.
// This is a viewer-only file. Requires video-call.js to be loaded first.
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
      // Entry animation — forces WebKitGTK to composite the overlay immediately.
      // Without this, fixed-position elements appended outside a user-interaction
      // task (e.g. after an await) are not painted until the next reflow/repaint.
      "@keyframes goop-call-appear {",
      "  from { opacity: 0; transform: translateY(6px) scale(0.97); }",
      "  to   { opacity: 1; transform: translateY(0)   scale(1);    }",
      "}",
      ".goop-call-overlay {",
      "  position: fixed; bottom: 16px; right: 16px; z-index: 10000;",
      "  background: #1a1a2e; border: 1px solid #333; border-radius: 12px;",
      "  padding: 12px; display: flex; flex-direction: column; gap: 8px;",
      "  box-shadow: 0 8px 32px rgba(0,0,0,0.4); max-width: 320px;",
      "  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;",
      "  color: #e0e0e0; font-size: 13px;",
      "  animation: goop-call-appear 0.18s ease-out forwards;",
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

      // ── KEY FIX: create and attach the overlay to the DOM synchronously,
      // *inside the click-event task*, so WebKitGTK's compositor sees it and
      // renders it before we yield to the network for info.accept().
      // Without this, the overlay is only added after the await resolves (a
      // microtask), and WebKitGTK doesn't repaint until the next user interaction.
      var parts = prepareOverlay();

      // Allow cancellation while accept() is in-flight.
      var cancelled = false;
      parts.hangupBtn.onclick = function() {
        cancelled = true;
        info.reject();
        removeOverlay();
      };

      try {
        var session = await info.accept();
        if (cancelled) return; // user hung up while connecting
        wireSession(parts, session);
      } catch(e) {
        log('error', 'Failed to accept call: ' + e);
        if (!cancelled) {
          removeOverlay();
          // Show a brief error in the status area of the overlay if it still exists,
          // or surface via notify if available.
          if (window.Goop && Goop.notify) {
            Goop.notify('Call failed — caller may have already hung up', 'error');
          }
        }
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


  // ── Active call overlay ─────────────────────────────────────────────────────

  // prepareOverlay() creates and appends the overlay shell to document.body
  // SYNCHRONOUSLY — no session needed yet.  The overlay shows "Connecting…"
  // and has placeholder button handlers.  Call wireSession() once the session
  // is available to activate the real callbacks.
  //
  // Returns a {el, remoteVideo, localVideo, statusEl, muteBtn, hangupBtn, videoBtn} object.
  //
  // Must be called from within a user-interaction task (click handler) so that
  // WebKitGTK composites and paints the element before any await suspends execution.
  function prepareOverlay() {
    injectStyles();
    removeOverlay();

    var el = document.createElement("div");
    el.className = "goop-call-overlay";
    el.innerHTML =
      '<div class="goop-call-videos" style="position:relative;">' +
        '<video class="goop-call-remote" autoplay playsinline></video>' +
        '<video class="goop-call-local" autoplay playsinline muted></video>' +
      '</div>' +
      '<div class="goop-call-status">Connecting\u2026</div>' +
      '<div class="goop-call-controls">' +
        '<button class="goop-call-btn goop-call-btn-mute" title="Toggle Mute">' +
          '<svg class="icon-on" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>' +
            '<path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" y1="19" x2="12" y2="23"/>' +
            '<line x1="8" y1="23" x2="16" y2="23"/>' +
          '</svg>' +
          '<svg class="icon-off hidden" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
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
          '<svg class="icon-off hidden" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
            '<path d="M16 16v1a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h2m5.66 0H14a2 2 0 0 1 2 2v3.34l1 1L23 7v10"/>' +
            '<line x1="1" y1="1" x2="23" y2="23"/>' +
          '</svg>' +
        '</button>' +
      '</div>';

    overlayEl = el;
    document.body.appendChild(el);
    return {
      el:          el,
      remoteVideo: el.querySelector(".goop-call-remote"),
      localVideo:  el.querySelector(".goop-call-local"),
      statusEl:    el.querySelector(".goop-call-status"),
      muteBtn:     el.querySelector(".goop-call-btn-mute"),
      hangupBtn:   el.querySelector(".goop-call-btn-hangup"),
      videoBtn:    el.querySelector(".goop-call-btn-video"),
    };
  }

  // wireSession() attaches a live session to an overlay created by prepareOverlay().
  // Safe to call as a separate step after an await (the overlay is already visible).
  function wireSession(parts, session) {
    log('info', 'Wiring session to call overlay, channel: ' + session.channelId);
    currentSession = session;

    var remoteVideo = parts.remoteVideo;
    var localVideo  = parts.localVideo;
    var statusEl    = parts.statusEl;
    var muteBtn     = parts.muteBtn;
    var hangupBtn   = parts.hangupBtn;
    var videoBtn    = parts.videoBtn;

    // Show local video (self-view inset).
    // onLocalStream uses replay-on-subscribe so it fires immediately if already
    // available (browser path) or when getUserMedia resolves later (W2W async).
    if (typeof session.onLocalStream === 'function') {
      session.onLocalStream(function(stream) { localVideo.srcObject = stream; });
    } else if (session.localStream) {
      localVideo.srcObject = session.localStream;
    }

    // Show remote video when available (browser RTCPeerConnection path)
    session.onRemoteStream(function(stream) {
      var trackInfo = stream.getTracks().map(function(t) { return t.kind + ":" + t.readyState + ":enabled=" + t.enabled; }).join(", ");
      log('info', 'Remote stream received in UI! tracks=[' + trackInfo + ']');
      remoteVideo.srcObject = stream;
      remoteVideo.play().then(function() {
        log('info', 'Remote video playing successfully');
      }).catch(function(e) {
        log('warn', 'Remote video autoplay blocked: ' + e.message);
      });
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

    // Phase 4 (native MSE path): session emits a blob URL instead of a MediaStream.
    // The overlay element is already in the DOM (prepareOverlay appended it), so
    // setting video.src here will correctly trigger MediaSource's sourceopen event.
    if (typeof session.onRemoteVideoSrc === 'function') {
      session.onRemoteVideoSrc(function(src) {
        log('info', 'Remote video src received (MSE WebM stream)');
        remoteVideo.srcObject = null;
        remoteVideo.src = src;
        remoteVideo.play().catch(function(e) {
          log('warn', 'MSE video autoplay: ' + e.message);
        });
        remoteVideo.onloadedmetadata = function() {
          log('info', 'MSE video metadata: ' + remoteVideo.videoWidth + 'x' + remoteVideo.videoHeight);
        };
        remoteVideo.onerror = function() {
          log('error', 'MSE video error: ' + (remoteVideo.error ? remoteVideo.error.message : 'unknown'));
        };
        // WebKitGTK can enter a stalled/waiting state (shows black) when the MSE
        // buffer is trimmed or after long playback. Recover by calling play().
        remoteVideo.addEventListener('waiting', function() {
          log('warn', 'MSE video waiting — attempting play() recovery');
          remoteVideo.play().catch(function() {});
        });
        remoteVideo.addEventListener('stalled', function() {
          log('warn', 'MSE video stalled — attempting play() recovery');
          remoteVideo.play().catch(function() {});
        });
      });
    }

    session.onStateChange(function(state) {
      statusEl.textContent = state === "connected" ? "Connected" :
                             state === "connecting" ? "Connecting\u2026" :
                             state;
    });

    session.onHangup(function() {
      removeOverlay();
    });

    muteBtn.onclick = function() {
      var enabled = session.toggleAudio();
      muteBtn.classList.toggle("active", !enabled);
      muteBtn.querySelector(".icon-on").classList.toggle('hidden', !enabled);
      muteBtn.querySelector(".icon-off").classList.toggle('hidden', enabled);
    };

    videoBtn.onclick = function() {
      var enabled = session.toggleVideo();
      videoBtn.classList.toggle("active", !enabled);
      videoBtn.querySelector(".icon-on").classList.toggle('hidden', !enabled);
      videoBtn.querySelector(".icon-off").classList.toggle('hidden', enabled);
    };

    hangupBtn.onclick = function() {
      session.hangup();
    };

  }

  // showActiveCall() — convenience wrapper for outbound calls and callUI.showCall().
  // Creates the overlay and wires the session in one step.
  function showActiveCall(session) {
    log('info', 'Showing active call UI for channel: ' + session.channelId);
    wireSession(prepareOverlay(), session);
  }

  function removeOverlay() {
    if (overlayEl && overlayEl.parentNode) {
      overlayEl.parentNode.removeChild(overlayEl);
    }
    overlayEl = null;
    currentSession = null;
    // No window.location.reload() — removing the element from the DOM is
    // sufficient.  A forced reload is disruptive in Wails webviews and can
    // appear to the user as if the call didn't close.
  }

  var escapeHtml = (window.Goop && window.Goop.core && window.Goop.core.escapeHtml) || function(s) { if (!s) return ''; var d = document.createElement('div'); d.appendChild(document.createTextNode(s)); return d.innerHTML; };

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
    },

    // Show the incoming call modal for a given info object.
    // Called by call-native.js after it replaces Goop.call in native mode,
    // since call-ui.js's auto-registration above already ran on the old manager.
    showIncoming: function(info) {
      showIncomingCall(info);
    }
  };
})();
