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
      ".goop-call-videos { display: flex; gap: 8px; }",
      ".goop-call-videos video {",
      "  border-radius: 8px; background: #000; object-fit: cover;",
      "}",
      ".goop-call-remote { width: 240px; height: 180px; }",
      ".goop-call-local {",
      "  width: 80px; height: 60px; position: absolute; bottom: 60px; right: 20px;",
      "  border: 2px solid #333; z-index: 1;",
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

  // ── Active call overlay ─────────────────────────────────────────────────────

  function showActiveCall(session) {
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
        '<button class="goop-call-btn goop-call-btn-mute" title="Toggle Mute">M</button>' +
        '<button class="goop-call-btn goop-call-btn-hangup" title="Hang Up">X</button>' +
        '<button class="goop-call-btn goop-call-btn-video" title="Toggle Video">V</button>' +
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
      remoteVideo.srcObject = stream;
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
      muteBtn.textContent = enabled ? "M" : "m";
    };

    videoBtn.onclick = function() {
      var enabled = session.toggleVideo();
      videoBtn.classList.toggle("active", !enabled);
      videoBtn.textContent = enabled ? "V" : "v";
    };

    hangupBtn.onclick = function() {
      session.hangup();
    };

    overlayEl = el;
    document.body.appendChild(el);
  }

  function removeOverlay() {
    if (overlayEl && overlayEl.parentNode) {
      overlayEl.parentNode.removeChild(overlayEl);
    }
    overlayEl = null;
    currentSession = null;
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
      try {
        var session = await Goop.call.start(peerId, constraints);
        showActiveCall(session);
        return session;
      } catch(e) {
        console.error("Failed to start call:", e);
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
