/**
 * call.js — unified call stack for the goop2 viewer.
 *
 * Replaces both video-call.js and call-native.js with a single file.
 * Path is determined by the local peer's mode, fetched once from /api/call/mode:
 *
 *   mode === 'native'  (Linux / Go / Pion)
 *     Caller: POST /api/call/start, send call-request via MQ, connect MSE on call-ack.
 *     Callee: POST /api/call/accept (Go sends call-ack), connect MSE.
 *     Go's Pion handles all SDP exchange and WebM encoding.
 *     MSE receives remote video via WebSocket /api/call/media/{channelId}.
 *
 *   mode === 'browser'  (Windows / WebView2 / standard WebRTC)
 *     Caller: getUserMedia + RTCPeerConnection, send call-request, create offer on call-ack.
 *     Callee: getUserMedia + RTCPeerConnection, send call-ack, handle offer.
 *     Standard trickle ICE via MQ ice-candidate signals.
 *
 * MQ signal types (all on topic "call:{channelId}"):
 *   call-request   caller → callee    invite
 *   call-ack       callee → caller    accepted (Go sends in native mode, JS sends in browser mode)
 *   call-offer     caller → callee    SDP offer  (browser mode only)
 *   call-answer    callee → caller    SDP answer (browser mode only)
 *   ice-candidate  either direction   trickle ICE (browser mode only)
 *   call-hangup    either direction   end call
 *
 * Goop.call is set synchronously so call-ui.js can register onIncoming immediately.
 * Mode is loaded asynchronously; it is always known before any real call starts.
 */
(function () {
  'use strict';

  var ICE_SERVERS = [
    { urls: 'stun:stun.l.google.com:19302' },
    { urls: 'stun:stun1.l.google.com:19302' },
  ];

  // Runtime mode — resolved once from /api/call/mode.
  // start() and accept() both await _modePromise so path selection never uses the default.
  var _mode        = 'browser'; // 'browser' | 'native'
  var _platform    = 'unknown';
  var _modePromise = null;      // set by _init(), awaited before any call path decision

  var _sessions     = {};  // channelId → CallSession
  var _incomingCbs  = [];
  var _restoreCbs   = [];
  var _mqSubscribed = false;

  // ── Logging ──────────────────────────────────────────────────────────────────

  function log(level, msg) {
    if (window.Goop && Goop.log && Goop.log[level]) {
      Goop.log[level]('call', msg);
    } else {
      (console[level] || console.log)('[call]', msg);
    }
  }

  // ── CallSession ──────────────────────────────────────────────────────────────
  //
  // API consumed by call-ui.js:
  //   session.channelId           string
  //   session.remotePeer          string   (alias for remotePeerId)
  //   session.localStream         MediaStream | null
  //   session.onLocalStream(cb)   replay-on-subscribe
  //   session.onRemoteStream(cb)  replay-on-subscribe  (browser path)
  //   session.onRemoteVideoSrc(cb) replay-on-subscribe (native MSE path)
  //   session.onHangup(cb)
  //   session.onStateChange(cb)
  //   session.toggleAudio()       → bool
  //   session.toggleVideo()       → bool
  //   session.hangup()

  function CallSession(channelId, remotePeerId, isCaller, mediaType) {
    this.channelId    = channelId;
    this.remotePeerId = remotePeerId;
    this.remotePeer   = remotePeerId; // alias — peer.js checks remotePeer
    this.isCaller     = isCaller;
    this.mediaType    = mediaType;    // 'audio' | 'video'
    this.localStream  = null;
    this.remoteStream = null;

    this._localStreamCbs    = [];
    this._remoteStreamCbs   = [];
    this._remoteVideoSrcCbs = [];
    this._hangupCbs         = [];
    this._stateCbs          = [];

    // Native MSE path: replay-on-subscribe URL
    this._remoteVideoSrc = null;

    // Browser WebRTC path
    this.pc           = null;
    this._pendingICE  = [];    // ICE candidates buffered before remoteDescription is set
    this._localVideoSrc  = null;    // MediaSource URL for native self-view (replay-on-subscribe)
    this._localVideoSrcCbs = [];
    this._selfWs      = null;      // WebSocket for self-view stream
    this._pendingOffer = null; // SDP offer buffered before PC is ready (callee race)

    // Native path handles
    this._mediaWs          = null;
    this._loopbackPc       = null;
    this._loopbackIceUnsub = null;

    // Toggle state (native mode — Go owns the tracks)
    this._audioEnabled = true;
    this._videoEnabled = true;
  }

  // ── Callbacks (replay-on-subscribe) ──

  CallSession.prototype.onLocalStream = function (cb) {
    this._localStreamCbs.push(cb);
    if (this.localStream) { try { cb(this.localStream); } catch (_) {} }
  };

  CallSession.prototype.onLocalVideoSrc = function (cb) {
    this._localVideoSrcCbs.push(cb);
    if (this._localVideoSrc !== null) { try { cb(this._localVideoSrc); } catch (_) {} }
  };

  CallSession.prototype.onRemoteStream = function (cb) {
    this._remoteStreamCbs.push(cb);
    if (this.remoteStream) { try { cb(this.remoteStream); } catch (_) {} }
  };

  CallSession.prototype.onRemoteVideoSrc = function (cb) {
    this._remoteVideoSrcCbs.push(cb);
    if (this._remoteVideoSrc !== null) { try { cb(this._remoteVideoSrc); } catch (_) {} }
  };

  CallSession.prototype.onHangup      = function (cb) { this._hangupCbs.push(cb); };
  CallSession.prototype.onStateChange = function (cb) { this._stateCbs.push(cb); };

  // ── Emit helpers ──

  CallSession.prototype._emitLocalStream = function (s) {
    this.localStream = s;
    this._localStreamCbs.forEach(function (cb) { try { cb(s); } catch (_) {} });
  };

  CallSession.prototype._emitLocalVideoSrc = function (url) {
    this._localVideoSrc = url;
    this._localVideoSrcCbs.forEach(function (cb) { try { cb(url); } catch (_) {} });
  };

  CallSession.prototype._emitRemoteStream = function (s) {
    this.remoteStream = s;
    this._remoteStreamCbs.forEach(function (cb) { try { cb(s); } catch (_) {} });
  };

  CallSession.prototype._emitRemoteVideoSrc = function (url) {
    this._remoteVideoSrc = url;
    this._remoteVideoSrcCbs.forEach(function (cb) { try { cb(url); } catch (_) {} });
  };

  CallSession.prototype._emitHangup = function () {
    this._hangupCbs.forEach(function (cb) { try { cb(); } catch (_) {} });
  };

  CallSession.prototype._emitState = function (s) {
    this._stateCbs.forEach(function (cb) { try { cb(s); } catch (_) {} });
  };

  // ── Toggles (sync for call-ui.js button handlers) ──

  CallSession.prototype.toggleAudio = function () {
    if (_mode === 'native') {
      this._audioEnabled = !this._audioEnabled;
      fetch('/api/call/toggle-audio', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel_id: this.channelId }),
      }).catch(function () {});
      return this._audioEnabled;
    }
    if (!this.localStream) return false;
    var tracks = this.localStream.getAudioTracks();
    if (!tracks.length) return false;
    tracks[0].enabled = !tracks[0].enabled;
    return tracks[0].enabled;
  };

  CallSession.prototype.toggleVideo = function () {
    if (_mode === 'native') {
      this._videoEnabled = !this._videoEnabled;
      fetch('/api/call/toggle-video', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel_id: this.channelId }),
      }).catch(function () {});
      return this._videoEnabled;
    }
    if (!this.localStream) return false;
    var tracks = this.localStream.getVideoTracks();
    if (!tracks.length) return false;
    tracks[0].enabled = !tracks[0].enabled;
    return tracks[0].enabled;
  };

  // ── Hangup ──

  CallSession.prototype.hangup = function () {
    _clearCallFromSession();
    this._cleanup();
    if (_mode === 'native') {
      fetch('/api/call/hangup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel_id: this.channelId }),
        keepalive: true,
      }).catch(function () {});
    } else {
      _sendMQ(this.remotePeerId, this.channelId, { type: 'call-hangup' });
    }
    this._emitHangup();
    delete _sessions[this.channelId];
  };

  CallSession.prototype._handleRemoteHangup = function () {
    _clearCallFromSession();
    this._cleanup();
    this._emitHangup();
    delete _sessions[this.channelId];
  };

  CallSession.prototype._cleanup = function () {
    if (this._loopbackIceUnsub) { this._loopbackIceUnsub(); this._loopbackIceUnsub = null; }
    if (this._loopbackPc) { this._loopbackPc.close(); this._loopbackPc = null; }
    if (this._mediaWs)    { this._mediaWs.close();    this._mediaWs    = null; }
    if (this._selfWs)     { this._selfWs.close();     this._selfWs     = null; }
    if (this.pc)          { this.pc.close();           this.pc          = null; }
    if (this.localStream) {
      this.localStream.getTracks().forEach(function (t) { t.stop(); });
      this.localStream = null;
    }
  };

  // ── Browser call persistence across page navigation ──────────────────────────
  //
  // sessionStorage keeps {channelId, remotePeer, isCaller, mediaType} across
  // full-page navigations so the W2W overlay can be restored on any page.

  function _saveCallToSession(sess) {
    try {
      sessionStorage.setItem('goop_active_call', JSON.stringify({
        channelId:  sess.channelId,
        remotePeer: sess.remotePeerId,
        isCaller:   sess.isCaller,
        mediaType:  sess.mediaType,
      }));
    } catch (_) {}
  }

  function _clearCallFromSession() {
    try { sessionStorage.removeItem('goop_active_call'); } catch (_) {}
  }

  // ── ICE candidate buffering ──────────────────────────────────────────────────

  CallSession.prototype._addIceCandidate = function (c) {
    if (!c || !c.candidate) return;
    var rd = this.pc && this.pc.remoteDescription && this.pc.remoteDescription.type;
    var ld = this.pc && this.pc.localDescription  && this.pc.localDescription.type;
    if (rd && ld) {
      this.pc.addIceCandidate(new RTCIceCandidate(c)).catch(function () {});
    } else {
      this._pendingICE.push(c);
    }
  };

  CallSession.prototype._flushPendingICE = function () {
    var self = this;
    var pending = this._pendingICE.splice(0);
    pending.forEach(function (c) {
      self.pc.addIceCandidate(new RTCIceCandidate(c)).catch(function () {});
    });
  };

  // ── Browser WebRTC path ──────────────────────────────────────────────────────

  CallSession.prototype._setupBrowserPC = async function () {
    var constraints = await _buildConstraints(this.mediaType);

    var stream;
    try {
      stream = await navigator.mediaDevices.getUserMedia(constraints);
    } catch (e) {
      log('warn', 'getUserMedia failed: ' + e);
      if (this.mediaType === 'video') {
        // Retry audio-only
        try {
          stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
        } catch (e2) {
          log('warn', 'getUserMedia audio-only also failed: ' + e2);
          this._emitState('error');
          return false;
        }
      } else {
        this._emitState('error');
        return false;
      }
    }
    this._emitLocalStream(stream);

    var pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });
    this.pc = pc;
    var self = this;

    stream.getTracks().forEach(function (t) { pc.addTrack(t, stream); });

    pc.ontrack = function (e) {
      if (e.streams && e.streams[0]) {
        self._emitRemoteStream(e.streams[0]);
        self._emitState('connected');
      } else if (e.track) {
        self._emitRemoteStream(new MediaStream([e.track]));
        self._emitState('connected');
      }
    };

    pc.onicecandidate = function (e) {
      if (!e.candidate) return;
      _sendMQ(self.remotePeerId, self.channelId, {
        type:      'ice-candidate',
        candidate: e.candidate.toJSON(),
      });
    };

    pc.oniceconnectionstatechange = function () {
      log('info', 'ICE state: ' + pc.iceConnectionState);
    };

    pc.onconnectionstatechange = function () {
      var s = pc.connectionState;
      log('info', 'PC state: ' + s);
      self._emitState(s);
      if (s === 'failed') {
        self.hangup();
      }
      // 'disconnected' is NOT an immediate hangup — remote peer may be navigating
      // and will send call-reconnect within the ICE timeout window (~30s).
    };

    return true;
  };

  // ── Native path (Linux / Go / Pion) ─────────────────────────────────────────
  //
  // Called by both caller (on call-ack) and callee (after /api/call/accept).
  // Connects MSE for displaying the remote video stream encoded by Go.

  CallSession.prototype._connectNative = async function () {
    this._emitState('connecting');
    // Go owns the camera via V4L2 — never call getUserMedia here (blocks WebKitGTK).
    // Self-view comes from Go's /api/call/self/{channel} WebM stream.
    this._connectSelfMSE(); // fire and forget
    // Remote video from Go's /api/call/media/{channel} WebM stream via MSE.
    await this._connectMSE();
  };

  CallSession.prototype._connectLoopback = async function () {
    try {
      var self = this;
      var pc = new RTCPeerConnection({ iceServers: [] });
      this._loopbackPc = pc;

      pc.ontrack = function (e) {
        if (e.streams && e.streams[0]) {
          self._emitRemoteStream(e.streams[0]);
          self._emitState('connected');
        }
      };

      pc.onicecandidate = function (e) {
        if (!e.candidate) return;
        fetch('/api/call/loopback/' + self.channelId + '/ice', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            candidate:     e.candidate.candidate,
            sdpMid:        e.candidate.sdpMid,
            sdpMLineIndex: e.candidate.sdpMLineIndex,
          }),
        }).catch(function () {});
      };

      this._loopbackIceUnsub = window.Goop.mq.onLoopbackICE(this.channelId, function (candidate) {
        pc.addIceCandidate(new RTCIceCandidate(candidate)).catch(function () {});
      });

      var offer = await pc.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true });
      await pc.setLocalDescription(offer);

      var resp = await fetch('/api/call/loopback/' + this.channelId + '/offer', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sdp: offer.sdp }),
      });
      if (!resp.ok) { log('warn', 'loopback offer rejected: ' + resp.status); return; }

      var data = await resp.json();
      if (data.sdp) {
        await pc.setRemoteDescription({ type: 'answer', sdp: data.sdp });
      } else {
        this._emitState('connected');
        log('info', 'loopback stub active (Go LocalPC not yet wired — Phase 5)');
      }
    } catch (err) {
      log('warn', 'loopback setup error: ' + err);
      this._emitState('error');
    }
  };

  // _connectSelfMSE opens the Go self-view WebSocket (/api/call/self/{channel})
  // and feeds localVideo via MSE.  Used in native mode when getUserMedia is
  // unavailable (Go holds the V4L2 device).  Fire-and-forget — do not await.
  CallSession.prototype._connectSelfMSE = async function () {
    if (typeof MediaSource === 'undefined') return;
    var mimeType = 'video/webm; codecs="vp8"';
    if (!MediaSource.isTypeSupported(mimeType)) {
      log('warn', 'VP8 WebM not supported for self-view');
      return;
    }

    var ms  = new MediaSource();
    var url = URL.createObjectURL(ms);
    this._emitLocalVideoSrc(url);

    var self = this;
    var sourceOpenOk = await new Promise(function (resolve) {
      var timeout = setTimeout(function () {
        log('warn', 'self-view sourceopen timeout');
        resolve(false);
      }, 4000);
      ms.addEventListener('sourceopen', function () { clearTimeout(timeout); resolve(true); }, { once: true });
    });

    if (!sourceOpenOk || ms.readyState !== 'open') {
      log('warn', 'self-view MSE not ready');
      return;
    }

    var sb;
    try {
      sb = ms.addSourceBuffer(mimeType);
      sb.mode = 'sequence';
    } catch (e) {
      log('warn', 'self-view MSE addSourceBuffer failed: ' + e);
      return;
    }

    var queue     = [];
    var appending = false;

    function tryAppend() {
      if (appending || queue.length === 0 || sb.updating || ms.readyState !== 'open') return;
      appending = true;
      try {
        sb.appendBuffer(queue.shift());
      } catch (e) {
        log('warn', 'self-view MSE appendBuffer error: ' + e);
        appending = false;
      }
    }

    sb.addEventListener('updateend', function () {
      appending = false;
      if (!sb.updating && sb.buffered.length > 0 && ms.readyState === 'open') {
        var s0 = sb.buffered.start(0), e0 = sb.buffered.end(0);
        if (e0 - s0 > 60) {
          try { sb.remove(s0, e0 - 60); return; } catch (_) {}
        }
      }
      tryAppend();
    });

    var wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var wsUrl   = wsProto + '//' + window.location.host + '/api/call/self/' + this.channelId;
    log('info', 'Opening self-view WebSocket: ' + wsUrl);

    var ws = new WebSocket(wsUrl);
    this._selfWs   = ws;
    ws.binaryType  = 'arraybuffer';
    ws.onopen    = function () { log('info', 'Self-view WebSocket connected'); };
    ws.onmessage = function (e) { queue.push(new Uint8Array(e.data)); tryAppend(); };
    ws.onerror   = function () { log('warn', 'Self-view WebSocket error'); };
    ws.onclose   = function () {
      log('info', 'Self-view WebSocket closed');
      self._selfWs = null;
      if (ms.readyState === 'open') {
        try { ms.endOfStream(); } catch (_) {}
      }
    };
  };

  CallSession.prototype._connectMSE = async function () {
    if (typeof MediaSource === 'undefined') {
      log('warn', 'MSE not available — remote video will not display');
      this._emitState('connected');
      return;
    }

    var mimeType  = 'video/webm; codecs="vp8,opus"';
    var supported = MediaSource.isTypeSupported(mimeType);
    log('info', 'MSE support for ' + mimeType + ': ' + supported);
    if (!supported) {
      log('warn', 'VP8+Opus WebM not supported — remote video unavailable');
      this._emitState('connected');
      return;
    }

    var ms  = new MediaSource();
    var url = URL.createObjectURL(ms);

    // Emit early — call-ui.js sets video.src = url, triggering 'sourceopen'.
    this._emitRemoteVideoSrc(url);

    var self = this;
    var sourceOpenOk = await new Promise(function (resolve) {
      var timeout = setTimeout(function () {
        log('warn', 'sourceopen timeout — MSE may not be supported or video element not in DOM');
        resolve(false);
      }, 4000);
      ms.addEventListener('sourceopen', function () { clearTimeout(timeout); resolve(true); }, { once: true });
    });

    if (!sourceOpenOk || ms.readyState !== 'open') {
      log('warn', 'MSE not ready (readyState=' + ms.readyState + ')');
      this._emitState('connected');
      return;
    }

    var sb;
    try {
      sb = ms.addSourceBuffer(mimeType);
      sb.mode = 'sequence';
    } catch (e) {
      log('warn', 'MSE addSourceBuffer failed: ' + e);
      this._emitState('connected');
      return;
    }

    var queue      = [];
    var appending  = false;
    var emittedConnected = false;

    function tryAppend() {
      if (appending || queue.length === 0 || sb.updating || ms.readyState !== 'open') return;
      appending = true;
      try {
        sb.appendBuffer(queue.shift());
      } catch (e) {
        log('warn', 'MSE appendBuffer error: ' + e);
        appending = false;
      }
    }

    sb.addEventListener('updateend', function () {
      appending = false;
      if (!emittedConnected && ms.readyState === 'open') {
        emittedConnected = true;
        self._emitState('connected');
      }
      if (!sb.updating && sb.buffered.length > 0 && ms.readyState === 'open') {
        var s0 = sb.buffered.start(0), e0 = sb.buffered.end(0);
        if (e0 - s0 > 120) {
          try { sb.remove(s0, e0 - 120); return; } catch (_) {}
        }
      }
      tryAppend();
    });

    var wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var wsUrl   = wsProto + '//' + window.location.host + '/api/call/media/' + this.channelId;
    log('info', 'Opening media WebSocket: ' + wsUrl);

    var ws = new WebSocket(wsUrl);
    this._mediaWs  = ws;
    ws.binaryType  = 'arraybuffer';
    ws.onopen    = function () { log('info', 'Media WebSocket connected'); };
    ws.onmessage = function (e) { queue.push(new Uint8Array(e.data)); tryAppend(); };
    ws.onerror   = function () { log('warn', 'Media WebSocket error'); };
    ws.onclose   = function () {
      log('info', 'Media WebSocket closed');
      self._mediaWs = null;
      if (ms.readyState === 'open') {
        try { ms.endOfStream(); } catch (_) {}
      }
    };
  };

  // ── Signal handlers (called from _dispatch) ──────────────────────────────────

  // call-ack received by caller — callee accepted.
  // payload carries callee's mode/platform (JS browser callee) or just platform (Go native callee).
  CallSession.prototype._handleCallAck = function (payload) {
    var calleeMode     = payload.mode     || 'native'; // Go omits mode → native
    var calleePlatform = payload.platform || 'unknown';
    log('info', 'call-ack on ' + this.channelId +
        ' [callee: ' + calleeMode + '/' + calleePlatform +
        ', self: ' + _mode + '/' + _platform + ']');

    if (_mode === 'native') {
      // Go Pion handles SDP; JS connects MSE for remote video display.
      this._connectNative();
      return;
    }
    // Browser mode: create and send SDP offer.
    var self = this;
    if (!self.pc) { log('warn', 'call-ack but no PC yet'); return; }
    self._emitState('connecting');
    self.pc.createOffer()
      .then(function (offer) {
        return self.pc.setLocalDescription(offer).then(function () {
          return _sendMQ(self.remotePeerId, self.channelId, {
            type: 'call-offer',
            sdp:  offer.sdp,
          });
        });
      })
      .catch(function (e) {
        log('warn', 'createOffer failed: ' + e);
        self._emitState('error');
      });
  };

  // call-offer received by callee (browser mode).
  CallSession.prototype._handleOffer = async function (sdp) {
    if (!this.pc) {
      this._pendingOffer = sdp; // PC not ready yet — will be handled after _setupBrowserPC
      return;
    }
    try {
      await this.pc.setRemoteDescription({ type: 'offer', sdp: sdp });
      var answer = await this.pc.createAnswer();
      await this.pc.setLocalDescription(answer);
      this._flushPendingICE();
      await _sendMQ(this.remotePeerId, this.channelId, {
        type: 'call-answer',
        sdp:  answer.sdp,
      });
    } catch (e) {
      log('warn', 'handleOffer error: ' + e);
      this._emitState('error');
    }
  };

  // call-answer received by caller (browser mode).
  CallSession.prototype._handleAnswer = async function (sdp) {
    if (!this.pc) return;
    try {
      await this.pc.setRemoteDescription({ type: 'answer', sdp: sdp });
      this._flushPendingICE();
    } catch (e) {
      log('warn', 'handleAnswer error: ' + e);
    }
  };

  // ── MQ helpers ───────────────────────────────────────────────────────────────

  function _sendMQ(peerId, channelId, payload) {
    var mq = window.Goop && window.Goop.mq;
    if (!mq) { log('warn', 'MQ not ready'); return Promise.resolve(); }
    return mq.sendCall(peerId, channelId, payload).catch(function () {});
  }

  // ── Media constraints builder ────────────────────────────────────────────────

  async function _buildConstraints(mediaType) {
    var c = { audio: true, video: mediaType === 'video' };
    try {
      var res = await fetch('/api/settings/quick/get');
      var cfg = await res.json();
      if (cfg.preferred_cam && c.video) {
        c.video = { deviceId: { ideal: cfg.preferred_cam } };
      }
      if (cfg.preferred_mic) {
        c.audio = { deviceId: { ideal: cfg.preferred_mic } };
      }
    } catch (_) {}
    return c;
  }

  // ── MQ subscription ──────────────────────────────────────────────────────────

  function _ensureMQSubscription() {
    if (_mqSubscribed) return;
    _mqSubscribed = true;
    function init() {
      if (!window.Goop || !window.Goop.mq) { setTimeout(init, 100); return; }
      window.Goop.mq.onCall(function (from, topic, payload, ack) {
        ack();
        if (!payload) return;
        var channelId = topic.slice(5); // strip 'call:' prefix
        if (channelId.startsWith('loopback:')) return; // handled by onLoopbackICE
        _dispatch(from, channelId, payload);
      });
    }
    init();
  }

  function _dispatch(from, channelId, payload) {
    var type = payload.type;

    if (type === 'call-request') {
      _handleIncoming(channelId, from, payload);
      return;
    }

    // call-reconnect: remote peer navigated and is re-establishing the call.
    // Handled outside the sess lookup because it creates a new session.
    if (type === 'call-reconnect') {
      _handleReconnect(from, channelId, payload);
      return;
    }

    var sess = _sessions[channelId];
    if (!sess) return;

    if      (type === 'call-ack')           { sess._handleCallAck(payload); }
    else if (type === 'call-reconnect-ack') { sess._handleCallAck(payload); } // same flow as ack
    else if (type === 'call-offer')         { sess._handleOffer(payload.sdp); }
    else if (type === 'call-answer')        { sess._handleAnswer(payload.sdp); }
    else if (type === 'ice-candidate')      { sess._addIceCandidate(payload.candidate); }
    else if (type === 'call-hangup')        { sess._handleRemoteHangup(); }
  }

  // ── Reconnect handling (browser mode, page navigation) ───────────────────────
  //
  // _handleReconnect: remote peer navigated mid-call and is re-establishing.
  // We silently set up a fresh RTCPeerConnection and send call-reconnect-ack.
  // The reconnecting peer receives the ack, calls _handleCallAck → createOffer
  // → normal offer/answer/ICE flow.
  //
  // _restoreBrowserCall: we navigated mid-call. sessionStorage has the state.
  // Set up a fresh PC, send call-reconnect to the remote peer, show the overlay.

  function _handleReconnect(from, channelId, payload) {
    log('info', 'call-reconnect from ' + from + ' on ' + channelId);
    // Clean up any existing session for this channel — the old RTCPeerConnection
    // is dead (remote peer's JS context was destroyed on navigate). Don't emit
    // hangup; we want the overlay to transition, not close.
    var existing = _sessions[channelId];
    if (existing) {
      existing._cleanup();
      delete _sessions[channelId];
    }

    var mediaType = payload.mediaType || 'video';
    var sess = new CallSession(channelId, from, false, mediaType);
    _sessions[channelId] = sess;
    sess.onHangup(function () { _clearCallFromSession(); delete _sessions[channelId]; });

    _ensureMQSubscription();

    sess._setupBrowserPC().then(function (ok) {
      if (!ok) { delete _sessions[channelId]; return; }
      _saveCallToSession(sess);
      _sendMQ(from, channelId, {
        type:     'call-reconnect-ack',
        mode:     'browser',
        platform: _platform,
      });
      // Re-wire the overlay (prepareOverlay removes the old one automatically).
      _restoreCbs.forEach(function (cb) {
        try { cb(sess); } catch (e) { log('error', 'reconnect cb error: ' + e); }
      });
    });
  }

  // _restoreBrowserCall: called on page load when sessionStorage has a pending
  // call. Sets up a fresh PC, signals the remote peer, and fires restoreCbs so
  // call-ui.js can recreate the overlay on the new page.
  function _restoreBrowserCall() {
    var stored;
    try {
      var raw = sessionStorage.getItem('goop_active_call');
      if (!raw) return;
      stored = JSON.parse(raw);
    } catch (_) { return; }
    if (!stored || !stored.channelId || !stored.remotePeer) return;

    log('info', 'Restoring browser call: ' + stored.channelId + ' → ' + stored.remotePeer);
    _ensureMQSubscription();

    var sess = new CallSession(stored.channelId, stored.remotePeer, stored.isCaller, stored.mediaType || 'video');
    _sessions[stored.channelId] = sess;
    sess.onHangup(function () {
      _clearCallFromSession();
      delete _sessions[stored.channelId];
    });

    sess._setupBrowserPC().then(function (ok) {
      if (!ok) {
        _clearCallFromSession();
        delete _sessions[stored.channelId];
        return;
      }
      // Signal the remote peer that we've navigated and are ready for a fresh
      // offer/answer.  They respond with call-reconnect-ack → _handleCallAck
      // → createOffer → normal ICE/SDP flow.
      _sendMQ(stored.remotePeer, stored.channelId, {
        type:      'call-reconnect',
        channelId: stored.channelId,
        mediaType: stored.mediaType,
      });
      // Show the overlay on the new page.
      _restoreCbs.forEach(function (cb) {
        try { cb(sess); } catch (e) { log('error', 'restore cb error: ' + e); }
      });
    });
  }

  // ── Incoming call handling ────────────────────────────────────────────────────

  function _handleIncoming(channelId, fromPeer, payload) {
    var callerMode     = payload.mode     || 'browser';
    var callerPlatform = payload.platform || 'unknown';
    var mediaType      = payload.mediaType || 'video';
    log('info', 'Incoming ' + mediaType + ' call from ' + fromPeer +
        ' [caller: ' + callerMode + '/' + callerPlatform + '] on ' + channelId);

    var info = {
      channelId:    channelId,
      peerId:       fromPeer,
      remotePeerId: fromPeer,
      mediaType:    mediaType,

      accept: async function () {
        // Wait for our own mode to be resolved before choosing the local path.
        await _modePromise;

        log('info', 'accepting ' + channelId +
            ' [caller: ' + callerMode + '/' + callerPlatform +
            ', self: ' + _mode + '/' + _platform + ']');

        var sess = new CallSession(channelId, fromPeer, false, mediaType);
        _sessions[channelId] = sess;
        sess.onHangup(function () { delete _sessions[channelId]; });

        if (_mode === 'native') {
          // Register Go session → Go sends call-ack → Go handles SDP.
          var res = await fetch('/api/call/accept', {
            method:  'POST',
            headers: { 'Content-Type': 'application/json' },
            body:    JSON.stringify({ channel_id: channelId, remote_peer: fromPeer }),
          });
          if (!res.ok) throw new Error('accept failed: ' + res.status);
          // Start native media setup in the background — wireSession() must be
          // called first to register onRemoteVideoSrc before _connectMSE() emits
          // the MediaSource URL and waits for sourceopen. Awaiting here would
          // deadlock: sourceopen needs video.src set by wireSession, but wireSession
          // runs after info.accept() returns.
          sess._connectNative();
        } else {
          // Browser mode: set up getUserMedia + RTCPeerConnection.
          var ok = await sess._setupBrowserPC();
          if (!ok) throw new Error('getUserMedia failed');
          // Send call-ack so caller knows we're ready.
          _sendMQ(fromPeer, channelId, {
            type:     'call-ack',
            mode:     'browser',
            platform: _platform,
          });
          // Persist so the overlay survives page navigation (browser mode).
          _saveCallToSession(sess);
          // Flush any offer that arrived while _setupBrowserPC was running.
          if (sess._pendingOffer) {
            var sdp = sess._pendingOffer;
            sess._pendingOffer = null;
            await sess._handleOffer(sdp);
          }
        }
        return sess;
      },

      reject: function () {
        _sendMQ(fromPeer, channelId, { type: 'call-hangup' });
      },
    };

    _incomingCbs.forEach(function (cb) {
      try { cb(info); } catch (e) { log('error', 'incoming cb error: ' + e); }
    });
  }

  // ── Public Goop.call API ──────────────────────────────────────────────────────

  window.Goop = window.Goop || {};

  Goop.call = {

    /**
     * Start an outbound call to peerId.
     * mediaTypeOrConstraints: 'audio' | 'video' | {video:bool, audio:bool} (legacy)
     */
    start: async function (peerId, mediaTypeOrConstraints) {
      var mediaType;
      if (typeof mediaTypeOrConstraints === 'string') {
        mediaType = mediaTypeOrConstraints;
      } else if (mediaTypeOrConstraints && mediaTypeOrConstraints.video) {
        mediaType = 'video';
      } else {
        mediaType = 'audio';
      }

      // Wait for mode to be resolved before making any path decision.
      await _modePromise;

      log('info', 'start call → ' + peerId + ' [' + mediaType + ', mode=' + _mode + ']');
      _ensureMQSubscription();

      var channelId = (_mode === 'native' ? 'nc-' : 'mc-') +
                      Math.random().toString(36).slice(2, 10);

      var sess = new CallSession(channelId, peerId, true, mediaType);
      _sessions[channelId] = sess;
      sess.onHangup(function () { delete _sessions[channelId]; });

      if (_mode === 'native') {
        // Register Go session first, then invite callee.
        var startRes = await fetch('/api/call/start', {
          method:  'POST',
          headers: { 'Content-Type': 'application/json' },
          body:    JSON.stringify({ channel_id: channelId, remote_peer: peerId }),
        });
        if (!startRes.ok) {
          delete _sessions[channelId];
          throw new Error('call start failed: ' + startRes.status);
        }
        _sendMQ(peerId, channelId, {
          type:      'call-request',
          mode:      _mode,
          platform:  _platform,
          mediaType: mediaType,
        });
        // _handleCallAck() will call _connectNative() when callee accepts.

      } else {
        // Browser mode: set up PC before inviting so we're ready the moment
        // call-ack arrives (avoids a race where ack → createOffer before PC exists).
        var ok = await sess._setupBrowserPC();
        if (!ok) {
          delete _sessions[channelId];
          throw new Error('getUserMedia failed');
        }
        _sendMQ(peerId, channelId, {
          type:      'call-request',
          mode:      _mode,
          platform:  _platform,
          mediaType: mediaType,
        });
        // Persist so the overlay survives page navigation (browser mode).
        _saveCallToSession(sess);
        // _handleCallAck() will call createOffer() once callee sends call-ack.
      }

      return sess;
    },

    /**
     * Register a handler for incoming calls.
     * cb is called with { channelId, peerId, mediaType, accept(), reject() }.
     */
    onIncoming: function (cb) {
      _incomingCbs.push(cb);
      _ensureMQSubscription();
    },

    /**
     * Register a handler for restored calls (active call detected on page load).
     * cb is called with a live CallSession — the same object passed to showActiveCall.
     * Native mode: fires when Go reports an active Pion session (/api/call/active).
     * Browser mode: fires when sessionStorage has a call and the fresh PC is ready.
     */
    onRestore: function (cb) {
      _restoreCbs.push(cb);
    },

    /**
     * Snapshot of active sessions (used by peer.js to detect existing calls on load).
     */
    activeCalls: function () {
      return Object.keys(_sessions).map(function (k) { return _sessions[k]; });
    },
  };

  // ── Initialise — fetch mode, store as an awaitable promise ──────────────────
  //
  // Goop.call is set synchronously so call-ui.js can register onIncoming right away.
  // _modePromise is awaited inside start() and accept() — the two places where
  // the local mode drives a real code-path decision.  This eliminates the race
  // between the async fetch and a user clicking a call button immediately after load.

  function _init() {
    _modePromise = fetch('/api/call/mode')
      .then(function (res) { return res.ok ? res.json() : {}; })
      .then(function (j) {
        _mode     = j.mode     || 'browser';
        _platform = j.platform || 'unknown';
        log('info', 'mode=' + _mode + ' platform=' + _platform);
        // Restore any active call sessions after page navigation.
        // Native: Go keeps Pion alive — query /api/call/active and re-attach UI.
        // Browser: RTCPeerConnections are destroyed on navigate — use sessionStorage
        //          to remember the call and MQ to signal a fresh offer/answer.
        if (_mode === 'native') {
          _restoreActiveCalls();
        } else {
          _restoreBrowserCall();
        }
      })
      .catch(function () { /* endpoint unavailable — stay in browser mode */ });
  }

  // _restoreActiveCalls queries Go for active sessions and re-attaches the UI.
  // Called once after mode resolves.  Fires _restoreCbs (e.g. showActiveCall in
  // call-ui.js) for each session so the call overlay reappears after navigation.
  function _restoreActiveCalls() {
    fetch('/api/call/active')
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (sessions) {
        if (!sessions || !sessions.length) return;
        _ensureMQSubscription();
        sessions.forEach(function (s) {
          if (_sessions[s.channel_id]) return; // already tracked
          log('info', 'restoring call session: ' + s.channel_id + ' remote=' + s.remote_peer);
          var sess = new CallSession(s.channel_id, s.remote_peer, s.is_caller, 'video');
          _sessions[s.channel_id] = sess;
          sess.onHangup(function () { delete _sessions[s.channel_id]; });
          // Reconnect native media (fire and forget — restoreCbs register callbacks first).
          sess._connectNative();
          _restoreCbs.forEach(function (cb) {
            try { cb(sess); } catch (e) { log('error', 'restore cb error: ' + e); }
          });
        });
      })
      .catch(function (e) { log('warn', 'active call check failed: ' + e); });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _init);
  } else {
    _init();
  }
})();
