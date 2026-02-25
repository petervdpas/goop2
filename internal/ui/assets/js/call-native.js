/**
 * call-native.js — Native Go/Pion WebRTC call stack, platform-aware.
 *
 * Load order: video-call.js → call-ui.js → call-native.js (see app.js)
 *
 * On load this module queries GET /api/call/mode.
 * - mode === "browser": nothing happens; video-call.js runs the show as usual.
 * - mode === "native": window._callNativeMode is set (suppresses video-call.js's
 *   browser modal), Goop.call is replaced with NativeCallManager, and call-ui.js's
 *   incoming handler is re-registered on the new manager.
 *
 * Platform-aware call constellations:
 *   W2W (both non-Linux): browser getUserMedia + RTCPeerConnection, bidirectional.
 *     Signaling uses custom types "browser-offer" / "browser-answer" / "browser-ice"
 *     over MQ so Go's Pion SDP exchange (which runs in parallel but harmlessly) doesn't
 *     interfere.  Both browsers have full WebRTC (WebView2/Chromium on Windows).
 *
 *   L2L, L2W, W2L: native Pion path.  Linux Go captures camera/mic, streams via
 *     ExternalPC.  Browser receives remote video via WebM/MSE over WebSocket
 *     (/api/call/media/{channel}).  Local self-view deferred to Phase 5.
 *
 * Constellation detection:
 *   1. Local platform comes from /api/call/mode response ("platform" field = runtime.GOOS).
 *   2. Remote platform is exchanged in MQ payloads:
 *      - call-request carries caller's platform (caller → callee)
 *      - call-ack carries callee's platform (callee → caller, sent by Go AcceptCall)
 *   3. Both sides therefore know the constellation before connecting media.
 *
 * MQ topic: "call:{channelID}" — all signals share one topic.
 * Signal types handled by JS: call-request, call-ack, call-hangup,
 *                              browser-offer, browser-answer, browser-ice.
 * Signal types handled by Go:  call-ack (→ Pion SDP), call-offer, call-answer,
 *                              ice-candidate (Pion ICE), call-hangup.
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
  //   session.localStream        MediaStream | null
  //   session.onLocalStream(cb)  fires when getUserMedia resolves (W2W)
  //   session.onRemoteStream(cb) fires when remote track arrives (W2W)
  //   session.onRemoteVideoSrc(cb) fires with MSE blob URL (native Pion path)
  //   session.onHangup(cb)       register callback
  //   session.onStateChange(cb)  register callback
  //   session.toggleAudio()      → bool (enabled, sync)
  //   session.toggleVideo()      → bool (enabled, sync)
  //   session.hangup()           void

  class NativeSession {
    constructor(channelId, remotePeerId) {
      this.channelId    = channelId;
      this.remotePeerId = remotePeerId;
      this.isNative     = true;
      this.localStream  = null; // set by _emitLocalStream when getUserMedia resolves

      this._remoteStreamCbs   = [];
      this._hangupCbs         = [];
      this._stateCbs          = [];
      this._localStreamCbs    = [];

      // Phase 4 / native path: remote video src (MSE blob URL).
      // Replay-on-subscribe so late registrations still receive the URL.
      this._remoteVideoSrcCbs = [];
      this._remoteVideoSrc    = null;

      // Local toggle state — toggleAudio/toggleVideo must be sync for call-ui.js.
      this._audioEnabled = true;
      this._videoEnabled = true;

      // Native Pion path (L2L, L2W, W2L)
      this._loopbackPc       = null;
      this._loopbackIceUnsub = null;
      this._mediaWs          = null;

      // W2W browser path
      this._browserPc          = null;
      this._pendingBrowserSignal = null; // resolve fn for incoming browser-offer / browser-answer
    }

    // ── Callbacks ──

    onRemoteStream(cb)   { this._remoteStreamCbs.push(cb); }
    onHangup(cb)         { this._hangupCbs.push(cb); }
    onStateChange(cb)    { this._stateCbs.push(cb); }

    onLocalStream(cb) {
      this._localStreamCbs.push(cb);
      if (this.localStream) {
        try { cb(this.localStream); } catch(_) {}
      }
    }

    onRemoteVideoSrc(cb) {
      this._remoteVideoSrcCbs.push(cb);
      if (this._remoteVideoSrc !== null) {
        try { cb(this._remoteVideoSrc); } catch(_) {}
      }
    }

    // ── Sync-compatible toggles ──

    toggleAudio() {
      this._audioEnabled = !this._audioEnabled;
      fetch('/api/call/toggle-audio', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: this.channelId }),
      }).catch(() => {});
      return this._audioEnabled;
    }

    toggleVideo() {
      this._videoEnabled = !this._videoEnabled;
      fetch('/api/call/toggle-video', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: this.channelId }),
      }).catch(() => {});
      return this._videoEnabled;
    }

    // ── Hangup ──

    hangup() {
      this._cleanup();
      fetch('/api/call/hangup', {
        method:    'POST',
        headers:   { 'Content-Type': 'application/json' },
        body:      JSON.stringify({ channel_id: this.channelId }),
        keepalive: true,
      }).catch(() => {});
      this._emitHangup();
    }

    // ── Internal emit helpers ──

    _emitLocalStream(stream) {
      this.localStream = stream;
      this._localStreamCbs.forEach(cb => { try { cb(stream); } catch(_) {} });
    }

    _emitRemoteStream(stream) {
      this._remoteStreamCbs.forEach(cb => cb(stream));
    }

    _emitHangup() {
      this._hangupCbs.forEach(cb => cb());
    }

    _emitState(state) {
      this._stateCbs.forEach(cb => cb(state));
    }

    _emitRemoteVideoSrc(src) {
      this._remoteVideoSrc = src;
      this._remoteVideoSrcCbs.forEach(cb => { try { cb(src); } catch(_) {} });
    }

    // ── Cleanup ──

    _cleanup() {
      if (this._loopbackIceUnsub) { this._loopbackIceUnsub(); this._loopbackIceUnsub = null; }
      if (this._loopbackPc)       { this._loopbackPc.close(); this._loopbackPc = null; }
      if (this._mediaWs)          { this._mediaWs.close();    this._mediaWs = null; }
      if (this._browserPc)        { this._browserPc.close();  this._browserPc = null; }
      if (this.localStream) {
        this.localStream.getTracks().forEach(t => t.stop());
        this.localStream = null;
      }
    }

    // Called by NativeCallManager when a call-hangup arrives on this channel via MQ.
    _handleRemoteHangup() {
      this._cleanup();
      this._emitHangup();
    }

    // ── W2W browser-to-browser WebRTC ──────────────────────────────────────────
    //
    // Both peers have full browser WebRTC (WebView2/Chromium on Windows).
    // Go's ExternalPC still runs (receive-only, no media) but is harmless.
    // Browser signals use "browser-offer" / "browser-answer" / "browser-ice" so
    // they don't collide with Go's "call-offer" / "call-answer" / "ice-candidate".
    //
    // role: 'caller' → creates offer  |  'callee' → waits for offer

    async _connectBrowserWebRTC(role) {
      this._emitState('connecting');

      if (typeof RTCPeerConnection === 'undefined' || !navigator.mediaDevices) {
        log('warn', 'browser WebRTC not available — W2W cannot connect');
        this._emitState('error');
        return;
      }

      // Acquire local media.
      let localStream;
      try {
        localStream = await navigator.mediaDevices.getUserMedia({ video: true, audio: true });
      } catch (e) {
        log('warn', 'getUserMedia (video+audio) failed: ' + e + ' — trying audio-only');
        try {
          localStream = await navigator.mediaDevices.getUserMedia({ video: false, audio: true });
        } catch (e2) {
          log('warn', 'getUserMedia (audio-only) failed: ' + e2);
          this._emitState('error');
          return;
        }
      }
      this._emitLocalStream(localStream);

      const pc = new RTCPeerConnection({ iceServers: [] });
      this._browserPc = pc;

      localStream.getTracks().forEach(t => pc.addTrack(t, localStream));

      // Remote track → emit to call-ui.js.
      pc.ontrack = e => {
        if (e.streams && e.streams[0]) {
          this._emitRemoteStream(e.streams[0]);
          this._emitState('connected');
        }
      };

      // Trickle ICE to the remote browser via MQ "browser-ice".
      pc.onicecandidate = e => {
        if (!e.candidate || !window.Goop || !window.Goop.mq) return;
        window.Goop.mq.sendCall(this.remotePeerId, this.channelId, {
          type: 'browser-ice',
          candidate: {
            candidate:     e.candidate.candidate,
            sdpMid:        e.candidate.sdpMid,
            sdpMLineIndex: e.candidate.sdpMLineIndex,
          },
        }).catch(() => {});
      };

      pc.onconnectionstatechange = () => {
        log('info', 'browser PC state: ' + pc.connectionState);
      };

      try {
        if (role === 'caller') {
          // Create offer → send "browser-offer" → wait for "browser-answer".
          const offer = await pc.createOffer();
          await pc.setLocalDescription(offer);
          window.Goop.mq.sendCall(this.remotePeerId, this.channelId, {
            type: 'browser-offer',
            sdp:  offer.sdp,
          }).catch(() => {});
          log('info', 'browser-offer sent, waiting for browser-answer');

          const answerPayload = await new Promise(resolve => {
            this._pendingBrowserSignal = resolve;
          });
          await pc.setRemoteDescription({ type: 'answer', sdp: answerPayload.sdp });
          log('info', 'browser WebRTC answer set — ICE connecting');

        } else {
          // Callee: wait for "browser-offer" → create answer → send "browser-answer".
          log('info', 'waiting for browser-offer from caller');
          const offerPayload = await new Promise(resolve => {
            this._pendingBrowserSignal = resolve;
          });
          await pc.setRemoteDescription({ type: 'offer', sdp: offerPayload.sdp });
          const answer = await pc.createAnswer();
          await pc.setLocalDescription(answer);
          window.Goop.mq.sendCall(this.remotePeerId, this.channelId, {
            type: 'browser-answer',
            sdp:  answer.sdp,
          }).catch(() => {});
          log('info', 'browser-answer sent');
        }
      } catch (err) {
        log('warn', 'browser WebRTC setup error: ' + err);
        this._emitState('error');
      }
    }

    // Called by NativeCallManager._ensureEventSource when a browser-* signal arrives.
    _handleBrowserSignal(type, payload) {
      if ((type === 'browser-offer' || type === 'browser-answer') && this._pendingBrowserSignal) {
        const resolve = this._pendingBrowserSignal;
        this._pendingBrowserSignal = null;
        resolve(payload);
      } else if (type === 'browser-ice' && this._browserPc) {
        const c = payload && payload.candidate;
        if (c && c.candidate) {
          this._browserPc.addIceCandidate(new RTCIceCandidate(c)).catch(() => {});
        }
      }
    }

    // ── Native Pion path (L2L, L2W, W2L) ──────────────────────────────────────
    //
    // Two paths depending on browser capability:
    //   A) RTCPeerConnection available (macOS/Windows): loopback PeerConnection — Go LocalPC.
    //      (Currently a stub at /api/call/loopback — Phase 4 LocalPC not yet wired.)
    //   B) RTCPeerConnection unavailable (WebKitGTK/Wails on Linux): WebM/MSE streaming.

    async _connectLoopback() {
      this._emitState('connecting');

      if (typeof RTCPeerConnection === 'undefined') {
        log('info', 'RTCPeerConnection unavailable — using WebM/MSE stream');
        await this._connectMSE();
        return;
      }

      // Path A — RTCPeerConnection loopback to Go's LocalPC (stub until Phase 5).
      try {
        const pc = new RTCPeerConnection({ iceServers: [] });
        this._loopbackPc = pc;

        pc.ontrack = e => {
          if (e.streams && e.streams[0]) {
            this._emitRemoteStream(e.streams[0]);
            this._emitState('connected');
          }
        };

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

        this._loopbackIceUnsub = window.Goop.mq.onLoopbackICE(this.channelId, candidate => {
          pc.addIceCandidate(new RTCIceCandidate(candidate)).catch(() => {});
        });

        const offer = await pc.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true });
        await pc.setLocalDescription(offer);

        const resp = await fetch(`/api/call/loopback/${this.channelId}/offer`, {
          method:  'POST',
          headers: { 'Content-Type': 'application/json' },
          body:    JSON.stringify({ sdp: offer.sdp }),
        });
        if (!resp.ok) {
          log('warn', 'loopback offer rejected: ' + resp.status);
          return;
        }

        const { sdp } = await resp.json();
        if (sdp) {
          await pc.setRemoteDescription({ type: 'answer', sdp });
        } else {
          // Stub: no LocalPC yet — mark connected so overlay shows correctly.
          this._emitState('connected');
          log('info', 'loopback stub active (Go LocalPC not yet wired)');
        }
      } catch (err) {
        log('warn', 'loopback setup error: ' + err);
        this._emitState('error');
      }
    }

    // Phase 4 MSE path — WebKitGTK/Wails Linux: receive remote video via WebM/WebSocket.
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
      this._emitRemoteVideoSrc(url);

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
        sb.mode = 'sequence';
      } catch (e) {
        log('warn', 'MSE addSourceBuffer failed: ' + e);
        this._emitState('connected');
        return;
      }

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
        if (!connectedEmitted && ms.readyState === 'open') {
          connectedEmitted = true;
          this._emitState('connected');
        }
        if (!sb.updating && sb.buffered.length > 0 && ms.readyState === 'open') {
          const s0 = sb.buffered.start(0), e0 = sb.buffered.end(0);
          if (e0 - s0 > 120) {
            try { sb.remove(s0, e0 - 120); return; } catch (_) {}
          }
        }
        tryAppend();
      });

      const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl   = wsProto + '//' + window.location.host + '/api/call/media/' + this.channelId;
      log('info', 'Opening media WebSocket: ' + wsUrl);

      const ws = new WebSocket(wsUrl);
      this._mediaWs  = ws;
      ws.binaryType  = 'arraybuffer';

      ws.onopen    = () => log('info', 'Media WebSocket connected');
      ws.onmessage = e => { queue.push(new Uint8Array(e.data)); tryAppend(); };
      ws.onerror   = () => log('warn', 'Media WebSocket error');
      ws.onclose   = () => {
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
      this._platform    = 'linux'; // set from /api/call/mode "platform" field
      this._incomingCbs = [];
      this._evtSource   = null;
      this._sessions    = {}; // channelId → NativeSession
    }

    /**
     * Initiate an outbound call to peerId.
     * _connectLoopback / _connectBrowserWebRTC is deferred to _handleCallAck
     * so we know the remote platform before choosing the media path.
     */
    async start(peerId /*, constraints — ignored; Go handles media in Pion path */) {
      log('info', 'starting call to ' + peerId);

      const channelId = 'nc-' + Math.random().toString(36).slice(2, 10);
      log('info', 'MQ channel id: ' + channelId);

      const startRes = await fetch('/api/call/start', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ channel_id: channelId, remote_peer: peerId }),
      });
      if (!startRes.ok) throw new Error('call start failed: ' + startRes.status);
      log('info', 'Go call session started, channel=' + channelId);

      const sess = new NativeSession(channelId, peerId);
      this._sessions[channelId] = sess;
      sess.onHangup(() => { delete this._sessions[channelId]; });

      // Media connection deferred: _handleCallAck chooses the path once we
      // know both platforms from the call-ack payload.
      this._notifyCallee(channelId, peerId);

      return sess;
    }

    /**
     * Send call-request with local platform so the callee can determine constellation.
     */
    _notifyCallee(channelId, peerId) {
      const send = () => {
        if (!window.Goop || !window.Goop.mq) { setTimeout(send, 200); return; }
        window.Goop.mq.sendCall(peerId, channelId, {
          type:     window.Goop.mq.CALL_TYPES.REQUEST,
          platform: this._platform,
        }).catch(() => {});
      };
      send();
    }

    /**
     * Register a handler for incoming calls.
     */
    onIncoming(cb) {
      this._incomingCbs.push(cb);
      this._ensureEventSource();
    }

    // Stub required by peer.js / peers.js autorefresh check.
    activeCalls() { return []; }

    // ── MQ subscription ──────────────────────────────────────────────────────

    _ensureEventSource() {
      if (this._evtSource) return;
      this._evtSource = true;
      const init = () => {
        if (!window.Goop || !window.Goop.mq) { setTimeout(init, 100); return; }
        window.Goop.mq.onCall((from, topic, payload, ack) => {
          ack();
          if (!payload) return;
          const channelId = topic.slice(5); // strip "call:" prefix
          if (channelId.startsWith('loopback:')) return; // handled by onLoopbackICE

          const type = payload.type;
          if (type === 'call-request') {
            this._handleIncoming({
              channel_id:      channelId,
              remote_peer:     from,
              caller_platform: payload.platform || 'linux',
            });
          } else if (type === 'call-ack') {
            this._handleCallAck(channelId, from, payload);
          } else if (type === 'call-hangup') {
            const sess = this._sessions[channelId];
            if (sess) sess._handleRemoteHangup();
          } else if (type === 'browser-offer' || type === 'browser-answer' || type === 'browser-ice') {
            const sess = this._sessions[channelId];
            if (sess) sess._handleBrowserSignal(type, payload);
          }
          // call-offer, call-answer, ice-candidate handled by Go's Pion path — ignored in JS.
        });
      };
      init();
    }

    /**
     * call-ack received: callee accepted.
     * payload.platform tells us the callee's OS.
     * Determine constellation and connect media accordingly.
     */
    _handleCallAck(channelId, from, payload) {
      const sess = this._sessions[channelId];
      if (!sess) {
        log('warn', 'call-ack for unknown session ' + channelId);
        return;
      }
      const calleePlatform = payload.platform || 'linux';
      const isW2W = this._platform !== 'linux' && calleePlatform !== 'linux';
      log('info', 'call-ack: local=' + this._platform + ' remote=' + calleePlatform + ' W2W=' + isW2W);

      if (isW2W) {
        log('info', 'W2W constellation → browser WebRTC (getUserMedia + RTCPeerConnection)');
        sess._connectBrowserWebRTC('caller');
      } else {
        log('info', 'native Pion constellation → Go ExternalPC + WebM/MSE');
        sess._connectLoopback();
      }
    }

    /**
     * Incoming call-request received.
     * caller_platform tells us the caller's OS.
     * The accept() closure uses constellation info to pick the right media path.
     */
    _handleIncoming({ channel_id: channelId, remote_peer: remotePeerId, caller_platform: callerPlatform }) {
      log('info', 'incoming call from ' + remotePeerId + ' (' + callerPlatform + ') on ' + channelId);
      const isW2W = this._platform !== 'linux' && callerPlatform !== 'linux';

      const incoming = {
        channelId,
        peerId:       remotePeerId,
        remotePeerId,

        accept: async () => {
          log('info', 'accepting call on channel ' + channelId + ' (W2W=' + isW2W + ')');
          const res = await fetch('/api/call/accept', {
            method:  'POST',
            headers: { 'Content-Type': 'application/json' },
            body:    JSON.stringify({ channel_id: channelId, remote_peer: remotePeerId }),
          });
          if (!res.ok) throw new Error('accept failed: ' + res.status);

          const sess = new NativeSession(channelId, remotePeerId);
          this._sessions[channelId] = sess;
          sess.onHangup(() => { delete this._sessions[channelId]; });

          if (isW2W) {
            log('info', 'W2W constellation → browser WebRTC (getUserMedia + RTCPeerConnection)');
            sess._connectBrowserWebRTC('callee');
          } else {
            log('info', 'native Pion constellation → Go ExternalPC + WebM/MSE');
            sess._connectLoopback();
          }
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
    let mode = 'browser', first = false, platform = 'linux';
    try {
      const res = await fetch('/api/call/mode');
      if (res.ok) {
        const j = await res.json();
        mode     = j.mode     || 'browser';
        first    = !!j.first;
        platform = j.platform || 'linux';
      }
    } catch (_) { /* endpoint unavailable — stay in browser mode */ }

    if (mode !== 'native') {
      return;
    }

    if (first) {
      log('info', 'mode=native platform=' + platform + ' — Go/Pion call stack active');
    }

    // Set suppression flag BEFORE registering the new manager so that any
    // call-request that arrives during the tiny init window is already suppressed
    // in video-call.js.notifyIncoming.
    window._callNativeMode = true;

    window.Goop = window.Goop || {};
    Goop.call = new NativeCallManager();
    Goop.call._platform = platform;

    // Re-register call-ui.js's incoming handler on the new native manager.
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
