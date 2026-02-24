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

      // Phase 4: remote video src (MSE blob URL) callbacks.
      // Uses replay-on-subscribe so late registrations still receive the URL.
      this._remoteVideoSrcCbs = [];
      this._remoteVideoSrc    = null;

      // Local toggle state — toggleAudio/toggleVideo must be sync for call-ui.js.
      this._audioEnabled = true;
      this._videoEnabled = true;

      this._loopbackPc      = null;
      this._loopbackIceUnsub = null; // Phase 4: unsubscribe for MQ loopback ICE
      this._mediaWs          = null; // Phase 4: WebM/MSE WebSocket
    }

    onRemoteStream(cb) { this._remoteStreamCbs.push(cb); }
    onHangup(cb)       { this._hangupCbs.push(cb); }
    onStateChange(cb)  { this._stateCbs.push(cb); }

    // Phase 4: subscribe to remote video src URL (MSE WebM stream).
    // Fires immediately if the URL is already known (replay-on-subscribe).
    onRemoteVideoSrc(cb) {
      this._remoteVideoSrcCbs.push(cb);
      if (this._remoteVideoSrc !== null) {
        try { cb(this._remoteVideoSrc); } catch (_) {}
      }
    }

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
      if (this._loopbackIceUnsub) { this._loopbackIceUnsub(); this._loopbackIceUnsub = null; }
      if (this._loopbackPc)       { this._loopbackPc.close(); this._loopbackPc = null; }
      if (this._mediaWs)          { this._mediaWs.close();    this._mediaWs = null; }
    }

    // Called by NativeCallManager when a call-hangup arrives on this channel via MQ.
    _handleRemoteHangup() {
      this._cleanup();
      this._emitHangup();
    }

    _emitRemoteStream(stream) { this._remoteStreamCbs.forEach(cb => cb(stream)); }
    _emitHangup()             { this._hangupCbs.forEach(cb => cb()); }
    _emitState(state)         { this._stateCbs.forEach(cb => cb(state)); }
    _emitRemoteVideoSrc(src)  {
      this._remoteVideoSrc = src;
      this._remoteVideoSrcCbs.forEach(cb => { try { cb(src); } catch (_) {} });
    }

    /**
     * Phase 4: deliver remote media to the browser.
     *
     * Two paths depending on browser capability:
     *   A) RTCPeerConnection available (macOS/Windows browser, or WebKitGTK with
     *      WebRTC enabled): loopback PeerConnection — Go's LocalPC relays tracks.
     *   B) RTCPeerConnection unavailable (WebKitGTK/Wails on Linux): WebM/MSE
     *      streaming — Go encodes VP8+Opus into a live WebM and streams it over
     *      a WebSocket to the browser's MediaSource API.
     */
    async _connectLoopback() {
      this._emitState('connecting');

      // Path B — WebKitGTK/Wails on Linux: no RTCPeerConnection.
      if (typeof RTCPeerConnection === 'undefined') {
        log('info', 'RTCPeerConnection unavailable — using WebM/MSE stream');
        await this._connectMSE();
        return;
      }

      // Path A — standard browser RTCPeerConnection loopback.
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

        // Subscribe to Go's LocalPC ICE candidates via MQ (Phase 4).
        // Go publishes on "call:loopback:{channelId}" via mqMgr.PublishLoopbackICE().
        this._loopbackIceUnsub = window.Goop.mq.onLoopbackICE(this.channelId, candidate => {
          pc.addIceCandidate(new RTCIceCandidate(candidate)).catch(() => {});
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

    /**
     * Phase 4 (MSE path): receive remote video as a live WebM stream over WebSocket.
     *
     * Go encodes the remote VP8 video and Opus audio into WebM clusters and
     * sends them as binary WebSocket messages.  The browser feeds these into a
     * MediaSource so <video>.src just works — no RTCPeerConnection needed.
     *
     * Message layout:
     *   - First message: EBML header + Segment(unknown-size) + Info + Tracks  (init segment)
     *   - Subsequent messages: Cluster elements  (one per video keyframe interval)
     */
    async _connectMSE() {
      if (typeof MediaSource === 'undefined') {
        log('warn', 'MSE not available — remote video will not display');
        this._emitState('connected');
        return;
      }

      const mimeType = 'video/webm; codecs="vp8,opus"';
      const supported = MediaSource.isTypeSupported(mimeType);
      log('info', 'MSE support for ' + mimeType + ': ' + supported);
      if (!supported) {
        log('warn', 'VP8+Opus WebM not supported by MSE — remote video unavailable');
        this._emitState('connected');
        return;
      }

      const ms  = new MediaSource();
      const url = URL.createObjectURL(ms);

      // Emit early — call-ui.js sets video.src = url, which triggers 'sourceopen'.
      // Uses replay-on-subscribe so this is safe even if the callback isn't registered yet.
      this._emitRemoteVideoSrc(url);

      // Wait for the video element to connect to the MediaSource.
      // Add a 4-second timeout: if WebKitGTK never fires sourceopen (e.g. the
      // video element isn't rendering), bail so the WebSocket still opens and
      // we at least get audio/connection state — better than hanging forever.
      const sourceOpenOk = await new Promise(resolve => {
        const timeout = setTimeout(() => {
          log('warn', 'sourceopen timeout — MSE may not be supported or video is not rendered');
          resolve(false);
        }, 4000);
        ms.addEventListener('sourceopen', () => { clearTimeout(timeout); resolve(true); }, { once: true });
      });

      if (!sourceOpenOk || ms.readyState !== 'open') {
        log('warn', 'MSE not ready (readyState=' + ms.readyState + ') — remote video unavailable');
        this._emitState('connected');
        return;
      }

      let sb;
      try {
        sb = ms.addSourceBuffer(mimeType);
        // Sequence mode: MSE plays clusters in arrival order, ignoring absolute
        // timecodes.  Without this, VP8 RTP-derived timecodes (large random
        // initial values) would place data millions of ms into the future and
        // the video element would show black (nothing buffered at currentTime=0).
        sb.mode = 'sequence';
      } catch (e) {
        log('warn', 'MSE addSourceBuffer failed: ' + e);
        this._emitState('connected');
        return;
      }

      // Sequential append queue — MSE requires one appendBuffer at a time.
      const queue = [];
      let   appending = false;
      let   connectedEmitted = false;

      const tryAppend = () => {
        if (appending || queue.length === 0 || sb.updating || ms.readyState !== 'open') return;
        appending = true;
        try {
          sb.appendBuffer(queue.shift());
        } catch (e) {
          log('warn', 'MSE appendBuffer error: ' + e);
          appending = false;
        }
      };

      sb.addEventListener('updateend', () => {
        appending = false;
        // Emit "connected" once the first chunk has been buffered.
        if (!connectedEmitted && ms.readyState === 'open') {
          connectedEmitted = true;
          this._emitState('connected');
        }
        // Trim old buffered data to prevent QuotaExceededError on long calls.
        // Keep only the last 30 s; remove() is async and fires updateend again.
        if (!sb.updating && sb.buffered.length > 0 && ms.readyState === 'open') {
          const s0 = sb.buffered.start(0), e0 = sb.buffered.end(0);
          if (e0 - s0 > 30) {
            try { sb.remove(s0, e0 - 30); return; } catch (_) {}
          }
        }
        tryAppend();
      });

      // Connect WebSocket — same host as the page (viewer HTTP server).
      const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl   = wsProto + '//' + window.location.host + '/api/call/media/' + this.channelId;
      log('info', 'Opening media WebSocket: ' + wsUrl);

      const ws = new WebSocket(wsUrl);
      this._mediaWs  = ws;
      ws.binaryType  = 'arraybuffer';

      ws.onopen = () => log('info', 'Media WebSocket connected');

      ws.onmessage = e => {
        queue.push(new Uint8Array(e.data));
        tryAppend();
      };

      ws.onerror = () => log('warn', 'Media WebSocket error');

      ws.onclose = () => {
        log('info', 'Media WebSocket closed');
        this._mediaWs = null;
        if (ms.readyState === 'open') {
          try { ms.endOfStream(); } catch (_) {}
        }
      };
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
      this._sessions    = {}; // channelId → NativeSession
    }

    /**
     * Initiate an outbound call to peerId.
     * 1. Create a realtime channel (invites the peer via the group protocol).
     * 2. Tell Go's call manager about the session.
     * 3. Return a NativeSession — call-ui.js shows the active-call overlay.
     * 4. In background: watch for callee to join, then send call-request signal.
     */
    async start(peerId /*, constraints — ignored; Go handles media in Phase 3+ */) {
      log('info', 'starting call to ' + peerId);

      // Create a virtual MQ channel ID (no server-side registration needed).
      const channelId = 'nc-' + Math.random().toString(36).slice(2, 10);
      log('info', 'MQ channel id: ' + channelId);

      // Register session with Go's call manager.
      const startRes = await fetch('/api/call/start', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: channelId, remote_peer: peerId }),
      });
      if (!startRes.ok) throw new Error('call start failed: ' + startRes.status);
      log('info', 'Go call session started, channel=' + channelId);

      // Create session, register it, then wire loopback.
      const sess = new NativeSession(channelId);
      this._sessions[channelId] = sess;
      sess.onHangup(() => { delete this._sessions[channelId]; });
      sess._connectLoopback();

      // Send call-request to callee via MQ.
      this._notifyCallee(channelId, peerId);

      return sess;
    }

    /**
     * Send a call-request message to the callee via MQ.
     */
    _notifyCallee(channelId, peerId) {
      log('info', 'sending call-request to ' + peerId + ' on ' + channelId);
      const send = () => {
        if (!window.Goop || !window.Goop.mq) {
          setTimeout(send, 200);
          return;
        }
        window.Goop.mq.sendCallRequest(peerId, channelId).catch(() => {
          // MQ will retry automatically from outbox.
        });
      };
      send();
    }

    /**
     * Register a handler for incoming calls.
     * Subscribes to 'call:*' via Goop.mq.  call-ui.js calls this at load time
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

    // ── MQ subscription ──

    _ensureEventSource() {
      if (this._evtSource) return;
      this._evtSource = true; // prevent double-init
      const init = () => {
        if (!window.Goop || !window.Goop.mq) { setTimeout(init, 100); return; }
        window.Goop.mq.onCall( (from, topic, payload, ack) => {
          ack();
          if (!payload) return;
          const channelId = topic.slice(5); // strip 'call:' prefix
          if (payload.type === 'call-request') {
            this._handleIncoming({ channel_id: channelId, remote_peer: from });
          } else if (payload.type === 'call-hangup') {
            const sess = this._sessions[channelId];
            if (sess) sess._handleRemoteHangup();
          }
        });
      };
      init();
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
          this._sessions[channelId] = sess;
          sess.onHangup(() => { delete this._sessions[channelId]; });
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
    let mode = 'browser', first = false;
    try {
      const res = await fetch('/api/call/mode');
      if (res.ok) { const j = await res.json(); mode = j.mode || 'browser'; first = !!j.first; }
    } catch (_) { /* endpoint unavailable — stay in browser mode */ }

    if (mode !== 'native') {
      return;
    }

    // Set the suppression flag BEFORE registering the new manager so that any
    // call-request that arrives on the realtime SSE during the tiny init window
    // is already suppressed in video-call.js.notifyIncoming.
    window._callNativeMode = true;

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
