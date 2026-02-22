/**
 * call-native.js — Native Go/Pion WebRTC call stack (Phase 2: signaling bridge).
 *
 * Load order: video-call.js → call-ui.js → call-native.js (see app.js)
 *
 * On load this module queries GET /api/call/mode.
 * - mode === "browser": nothing happens; video-call.js runs the show as usual.
 * - mode === "native": window._callNativeMode is set (suppresses video-call.js's
 *   browser modal), Goop.call is replaced with NativeCallManager, and call-ui.js's
 *   incoming handler is re-registered on the new manager.
 *
 * Phase 2 capability:
 *   - Incoming call modal rings ✓
 *   - Accept / Reject works ✓
 *   - Hangup cleans up both sides ✓
 *   - Toggle audio/video updates Go state ✓
 *   - call-ui.js showActiveCall overlay shows "Connected" (no video until Phase 4) ✓
 *
 * Phase 4 will wire the loopback RTCPeerConnection so the browser gets a real
 * MediaStream from Go's LocalPC without needing getUserMedia.
 */
(function () {
  "use strict";

  // ── Logging — routes through Goop.log so messages appear in the Goop Logs page.
  function log(level, msg) {
    if (window.Goop && window.Goop.log && window.Goop.log[level]) {
      window.Goop.log[level]('call-native', msg);
    } else {
      (console[level] || console.log)('[call-native]', msg);
    }
  }

  // ── NativeSession ──────────────────────────────────────────────────────────
  //
  // Mirrors the CallSession API that call-ui.js expects:
  //   session.channelId          string
  //   session.localStream        MediaStream | null  (null until Phase 3)
  //   session.onRemoteStream(cb) register callback
  //   session.onHangup(cb)       register callback
  //   session.onStateChange(cb)  register callback
  //   session.toggleAudio()      → bool (enabled, sync) — call-ui.js reads this sync
  //   session.toggleVideo()      → bool (enabled, sync)
  //   session.hangup()           void

  class NativeSession {
    constructor(channelId) {
      this.channelId  = channelId;
      this.isNative   = true;
      this.localStream = null; // Go captures camera/mic — Phase 3+

      this._remoteStreamCbs = [];
      this._hangupCbs       = [];
      this._stateCbs        = [];

      // Local toggle state — toggleAudio/toggleVideo must be sync for call-ui.js.
      this._audioEnabled = true;
      this._videoEnabled = true;

      this._loopbackPc    = null;
      this._loopbackIceEs = null;
      this._sessionEs     = null;
    }

    onRemoteStream(cb) { this._remoteStreamCbs.push(cb); }
    onHangup(cb)       { this._hangupCbs.push(cb); }
    onStateChange(cb)  { this._stateCbs.push(cb); }

    // Sync-compatible toggle: update local state immediately, fire async in background.
    // call-ui.js does:  var enabled = session.toggleAudio();  muteBtn.toggle(!enabled);
    toggleAudio() {
      this._audioEnabled = !this._audioEnabled;
      fetch('/api/call/toggle-audio', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: this.channelId }),
      }).catch(() => {});
      return this._audioEnabled; // bool: true = audio on (not muted)
    }

    toggleVideo() {
      this._videoEnabled = !this._videoEnabled;
      fetch('/api/call/toggle-video', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: this.channelId }),
      }).catch(() => {});
      return this._videoEnabled; // bool: true = video on (not disabled)
    }

    hangup() {
      this._cleanup();
      // keepalive: true ensures the request survives the page reload that
      // call-ui.js triggers synchronously from the onHangup callback below.
      fetch('/api/call/hangup', {
        method:    'POST',
        headers:   { 'Content-Type': 'application/json' },
        body:      JSON.stringify({ channel_id: this.channelId }),
        keepalive: true,
      }).catch(() => {});
      this._emitHangup();
    }

    // ── Internal ──

    _cleanup() {
      if (this._sessionEs)     { this._sessionEs.close();     this._sessionEs = null; }
      if (this._loopbackIceEs) { this._loopbackIceEs.close(); this._loopbackIceEs = null; }
      if (this._loopbackPc)    { this._loopbackPc.close();    this._loopbackPc = null; }
    }

    _emitRemoteStream(stream) { this._remoteStreamCbs.forEach(cb => cb(stream)); }
    _emitHangup()             { this._hangupCbs.forEach(cb => cb()); }
    _emitState(state)         { this._stateCbs.forEach(cb => cb(state)); }

    /**
     * Subscribe to server-side session events (hangup).
     * Go closes session.HangupCh() when either peer hangs up → SSE fires →
     * browser receives "hangup" event → call overlay closes.
     * Must be called after the Go session exists (i.e. after start/accept returns).
     */
    _listenForHangup() {
      const es = new EventSource(`/api/call/session/${this.channelId}/events`);
      this._sessionEs = es;

      es.addEventListener('hangup', () => {
        es.close();
        this._sessionEs = null;
        this._emitHangup();
      });

      es.onerror = () => {
        // Session ended or server restarted — stop retrying.
        if (this._sessionEs === es) {
          es.close();
          this._sessionEs = null;
        }
      };
    }

    /**
     * Phase 4: establish loopback RTCPeerConnection (localhost only, no STUN).
     * Go's LocalPC sends the remote peer's tracks + local camera preview to the
     * browser via this connection so <video>.srcObject just works.
     *
     * In Phase 2 the /loopback/offer endpoint returns an empty SDP (stub).
     * The connection proceeds without tracks and the overlay shows "Connected".
     * Phase 4 fills in the Go side so real media flows.
     */
    async _connectLoopback() {
      this._emitState('connecting');
      try {
        const pc = new RTCPeerConnection({ iceServers: [] });
        this._loopbackPc = pc;

        pc.ontrack = e => {
          if (e.streams && e.streams[0]) {
            this._emitRemoteStream(e.streams[0]);
            this._emitState('connected');
          }
        };

        // Trickle our ICE candidates to Go's LocalPC.
        pc.onicecandidate = e => {
          if (!e.candidate) return;
          fetch(`/api/call/loopback/${this.channelId}/ice`, {
            method:  'POST',
            headers: { 'Content-Type': 'application/json' },
            body:    JSON.stringify({
              candidate:     e.candidate.candidate,
              sdpMid:        e.candidate.sdpMid,
              sdpMLineIndex: e.candidate.sdpMLineIndex,
            }),
          }).catch(() => {});
        };

        // Subscribe to Go's loopback ICE candidates via SSE.
        const iceEs = new EventSource(`/api/call/loopback/${this.channelId}/ice`);
        this._loopbackIceEs = iceEs;
        iceEs.addEventListener('candidate', e => {
          try {
            pc.addIceCandidate(new RTCIceCandidate(JSON.parse(e.data))).catch(() => {});
          } catch (_) {}
        });

        // Ask Go to receive our offer and return an SDP answer.
        const offer = await pc.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true });
        await pc.setLocalDescription(offer);

        const resp = await fetch(`/api/call/loopback/${this.channelId}/offer`, {
          method:  'POST',
          headers: { 'Content-Type': 'application/json' },
          body:    JSON.stringify({ sdp: offer.sdp }),
        });
        if (!resp.ok) {
          log('warn', 'loopback offer rejected: ' + resp.status);
          iceEs.close();
          this._loopbackIceEs = null;
          return;
        }

        const { sdp } = await resp.json();
        if (sdp) {
          // Phase 4: real SDP answer from Go's LocalPC.
          await pc.setRemoteDescription({ type: 'answer', sdp });
        } else {
          // Phase 2/3 stub: no LocalPC yet — pretend connected so the overlay
          // shows "Connected" rather than "Connecting..." indefinitely.
          iceEs.close();
          this._loopbackIceEs = null;
          this._emitState('connected');
          log('info', 'loopback stub active (Go LocalPC wired in Phase 4)');
        }
      } catch (err) {
        log('warn', 'loopback setup error: ' + err);
        this._emitState('error');
      }
    }
  }

  // ── NativeCallManager ──────────────────────────────────────────────────────
  //
  // Replaces Goop.call when mode === "native".
  // Mirrors the Goop.call API that call-ui.js and peers.js use:
  //   Goop.call.start(peerId, constraints) → Promise<session>
  //   Goop.call.onIncoming(cb)             → register handler

  class NativeCallManager {
    constructor() {
      this._incomingCbs = [];
      this._evtSource   = null;
    }

    /**
     * Initiate an outbound call to peerId.
     * 1. Create a realtime channel (invites the peer via the group protocol).
     * 2. Tell Go's call manager about the session.
     * 3. Return a NativeSession — call-ui.js shows the active-call overlay.
     *
     * The callee's Go realtime manager fires a synthetic "call-request" from the
     * invite event, which flows through signalerAdapter → callMgr → /api/call/events
     * SSE → callee's call-native.js → showIncomingCall modal.  No explicit
     * "call-request" send needed from the caller's browser side.
     */
    async start(peerId /*, constraints — ignored; Go handles media in Phase 3+ */) {
      log('info', 'starting call to ' + peerId);

      // Step 1: create realtime channel (same HTTP call as browser mode).
      const chRes = await fetch('/api/realtime/connect', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ peer_id: peerId }),
      });
      if (!chRes.ok) throw new Error('realtime connect failed: ' + chRes.status);
      const channel = await chRes.json();
      log('info', 'channel created: ' + channel.id);

      // Step 2: register session with Go's call manager.
      const startRes = await fetch('/api/call/start', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: channel.id, remote_peer: peerId }),
      });
      if (!startRes.ok) throw new Error('call start failed: ' + startRes.status);
      log('info', 'Go call session started, channel=' + channel.id);

      // Step 3: create session and wire hangup + loopback.
      const sess = new NativeSession(channel.id);
      sess._listenForHangup();
      sess._connectLoopback();
      return sess;
    }

    /**
     * Register a handler for incoming calls.
     * Subscribes to the /api/call/events SSE.  call-ui.js calls this at load time
     * on the original browser Goop.call; call-native.js re-registers on the
     * new NativeCallManager after replacing Goop.call.
     */
    onIncoming(cb) {
      this._incomingCbs.push(cb);
      this._ensureEventSource();
    }

    // Stub required by peer.js and peers.js autorefresh check.
    // Native mode tracks sessions server-side; return empty for now.
    activeCalls() { return []; }

    // ── SSE subscription ──

    _ensureEventSource() {
      if (this._evtSource) return;
      const es = new EventSource('/api/call/events');
      this._evtSource = es;

      es.addEventListener('call', e => {
        try {
          const data = JSON.parse(e.data);
          if (data.type === 'incoming-call') this._handleIncoming(data);
        } catch (_) {}
      });

      es.onerror = () => {
        this._evtSource = null;
        setTimeout(() => this._ensureEventSource(), 3000);
      };
    }

    _handleIncoming({ channel_id: channelId, remote_peer: remotePeerId }) {
      log('info', 'incoming call from ' + remotePeerId + ' on channel ' + channelId);
      // Build an "info" object that matches what call-ui.js's showIncomingCall expects.
      // call-ui.js uses info.peerId and info.channelId.
      const incoming = {
        channelId,
        peerId:      remotePeerId, // ← call-ui.js uses info.peerId in the modal
        remotePeerId,              //   (kept for API completeness)

        accept: async () => {
          log('info', 'accepting call on channel ' + channelId);
          const res = await fetch('/api/call/accept', {
            method:  'POST',
            headers: { 'Content-Type': 'application/json' },
            body:    JSON.stringify({ channel_id: channelId, remote_peer: remotePeerId }),
          });
          if (!res.ok) throw new Error('accept failed: ' + res.status);
          const sess = new NativeSession(channelId);
          sess._listenForHangup();
          sess._connectLoopback();
          return sess;
        },

        reject: () => {
          fetch('/api/call/hangup', {
            method:  'POST',
            headers: { 'Content-Type': 'application/json' },
            body:    JSON.stringify({ channel_id: channelId }),
          }).catch(() => {});
        },
      };

      this._incomingCbs.forEach(cb => {
        try { cb(incoming); } catch (e) { log('error', 'incoming cb error: ' + e); }
      });
    }
  }

  // ── Bootstrap ──────────────────────────────────────────────────────────────

  async function init() {
    let mode = 'browser';
    try {
      const res = await fetch('/api/call/mode');
      if (res.ok) mode = (await res.json()).mode || 'browser';
    } catch (_) { /* endpoint unavailable — stay in browser mode */ }

    if (mode !== 'native') {
      log('info', 'mode=browser — browser WebRTC unchanged');
      return;
    }

    // Set the suppression flag BEFORE registering the new manager so that any
    // call-request that arrives on the realtime SSE during the tiny init window
    // is already suppressed in video-call.js.notifyIncoming.
    window._callNativeMode = true;
    log('info', 'mode=native — Go/Pion call stack active');

    window.Goop = window.Goop || {};
    Goop.call = new NativeCallManager();

    // Re-register call-ui.js's incoming handler on the new native manager.
    // call-ui.js already ran Goop.call.onIncoming(showIncomingCall) on the old
    // browser manager — that registration is now stale.  Re-wire it here using
    // the Goop.callUI.showIncoming bridge we added to call-ui.js.
    Goop.call.onIncoming(function (info) {
      if (Goop.callUI && typeof Goop.callUI.showIncoming === 'function') {
        Goop.callUI.showIncoming(info);
      }
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
