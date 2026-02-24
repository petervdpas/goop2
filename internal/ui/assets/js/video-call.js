//
// video-call.js — WebRTC video/audio call engine for the goop2 viewer.
// This is a viewer-only file. It has nothing to do with the SDK.
//
// Signaling flows over realtime channels (offer/answer/ICE candidates).
// Talks directly to /api/realtime/* — no SDK dependency.
//
(() => {
  window.Goop = window.Goop || {};

  function log(level, msg) {
    if (Goop.log && Goop.log[level]) {
      Goop.log[level]('webrtc', msg);
    } else {
      var fn = console[level] || console.log;
      fn('[webrtc]', msg);
    }
  }

  var ICE_SERVERS = [
    { urls: "stun:stun.l.google.com:19302" },
    { urls: "stun:stun1.l.google.com:19302" }
  ];


  var SIG_OFFER     = "call-offer";
  var SIG_ANSWER    = "call-answer";
  var SIG_ICE       = "ice-candidate";
  var SIG_HANGUP    = "call-hangup";
  var SIG_CALL_REQ  = "call-request";
  var SIG_CALL_ACK  = "call-ack";

  var incomingHandlers = [];
  var activeCalls = {};
  var listening = false;

  function closeChannel(channelId) {
    // MQ channels are virtual — no explicit close needed.
    log('debug', 'closeChannel: ' + channelId + ' (no-op with MQ transport)');
  }

  // ── CallSession ─────────────────────────────────────────────────────────────

  function CallSession(channel, isInitiator) {
    this.channel = channel;
    this.channelId = channel.id;
    this.remotePeer = channel.remotePeer;
    this.isInitiator = isInitiator;
    this.pc = null;
    this.localStream = null;
    this.remoteStream = null;
    this._onRemoteStream = [];
    this._onHangup = [];
    this._onStateChange = [];
    this._ended = false;
    this._pendingCallReq = false;
    this._constraints = null;
    this._callReqTimeout = null;

    log('info', 'CallSession created: channel=' + channel.id + ', initiator=' + isInitiator + ', remotePeer=' + (channel.remotePeer || 'unknown'));
  }

  CallSession.prototype.onRemoteStream = function(cb) {
    this._onRemoteStream.push(cb);
    if (this.remoteStream) {
      log('debug', 'onRemoteStream: already have stream, firing immediately');
      try { cb(this.remoteStream); } catch(e) { log('error', 'onRemoteStream callback error: ' + e.message); }
    }
  };

  CallSession.prototype.onHangup = function(cb) {
    this._onHangup.push(cb);
  };

  CallSession.prototype.onStateChange = function(cb) {
    this._onStateChange.push(cb);
  };

  CallSession.prototype._emitRemoteStream = function(stream) {
    var trackInfo = stream.getTracks().map(function(t) {
      return t.kind + ':' + t.readyState + ':enabled=' + t.enabled;
    }).join(', ');
    log('info', 'REMOTE STREAM RECEIVED! tracks=[' + trackInfo + ']');
    this.remoteStream = stream;
    stream.getTracks().forEach(function(track) {
      track.onended = function() { log('warn', 'Remote track ended: ' + track.kind); };
      track.onmute  = function() { log('debug', 'Remote track muted: ' + track.kind); };
      track.onunmute = function() { log('debug', 'Remote track unmuted: ' + track.kind); };
    });
    for (var i = 0; i < this._onRemoteStream.length; i++) {
      try {
        log('debug', 'Firing onRemoteStream callback ' + (i+1) + '/' + this._onRemoteStream.length);
        this._onRemoteStream[i](stream);
      } catch(e) { log('error', 'onRemoteStream callback error: ' + e.message); }
    }
  };

  CallSession.prototype._emitHangup = function() {
    log('info', 'Call ended, firing hangup callbacks');
    for (var i = 0; i < this._onHangup.length; i++) {
      try { this._onHangup[i](); } catch(e) { log('error', 'onHangup callback error: ' + e.message); }
    }
  };

  CallSession.prototype._emitStateChange = function(state) {
    log('debug', 'Connection state changed: ' + state);
    for (var i = 0; i < this._onStateChange.length; i++) {
      try { this._onStateChange[i](state); } catch(e) { log('error', 'onStateChange callback error: ' + e.message); }
    }
  };

  CallSession.prototype.hangup = function() {
    if (this._ended) return;
    this._ended = true;
    log('info', 'Hanging up call on channel: ' + this.channelId);
    try { this.channel.send({ type: SIG_HANGUP }).catch(function(){}); } catch(e) { /* ignore */ }
    this._cleanup();
    this._emitHangup();
    closeChannel(this.channelId);
  };

  CallSession.prototype._cleanup = function() {
    log('debug', 'Cleaning up call session');
    if (this._callReqTimeout) {
      clearTimeout(this._callReqTimeout);
      this._callReqTimeout = null;
    }
    if (this.pc) {
      log('debug', 'Closing RTCPeerConnection');
      this.pc.close();
      this.pc = null;
    }
    if (this.localStream) {
      log('debug', 'Stopping local stream tracks');
      this.localStream.getTracks().forEach(function(t) { t.stop(); });
      this.localStream = null;
    }
    delete activeCalls[this.channelId];
  };

  CallSession.prototype.toggleAudio = function() {
    if (!this.localStream) return false;
    var tracks = this.localStream.getAudioTracks();
    if (tracks.length === 0) return false;
    tracks[0].enabled = !tracks[0].enabled;
    log('debug', 'Audio toggled: ' + (tracks[0].enabled ? 'unmuted' : 'muted'));
    return tracks[0].enabled;
  };

  CallSession.prototype.toggleVideo = function() {
    if (!this.localStream) return false;
    var tracks = this.localStream.getVideoTracks();
    if (tracks.length === 0) return false;
    tracks[0].enabled = !tracks[0].enabled;
    log('debug', 'Video toggled: ' + (tracks[0].enabled ? 'on' : 'off'));
    return tracks[0].enabled;
  };

  // ── WebRTC setup ────────────────────────────────────────────────────────────

  async function applyPreferredDevices(constraints) {
    var c = Object.assign({}, constraints);
    try {
      var res = await fetch('/api/settings/quick/get');
      var cfg = await res.json();
      var camId = cfg.preferred_cam || '';
      var micId = cfg.preferred_mic || '';
      log('debug', 'Device preferences: cam=' + (camId || 'default') + ', mic=' + (micId || 'default'));
      if (camId && c.video) {
        c.video = typeof c.video === 'object' ? Object.assign({}, c.video) : {};
        c.video.deviceId = { ideal: camId };
      }
      if (micId && c.audio) {
        c.audio = typeof c.audio === 'object' ? Object.assign({}, c.audio) : {};
        c.audio.deviceId = { ideal: micId };
      }
    } catch(e) {
      log('warn', 'Could not load device preferences: ' + e.message);
    }
    return c;
  }

  async function setupPeerConnection(session, constraints) {
    log('info', 'Setting up peer connection, constraints: ' + JSON.stringify(constraints));
    log('debug', 'Requesting getUserMedia...');
    var appliedConstraints = await applyPreferredDevices(constraints);
    log('debug', 'Applied constraints: ' + JSON.stringify(appliedConstraints));

    var stream;
    try {
      stream = await navigator.mediaDevices.getUserMedia(appliedConstraints);
    } catch(e) {
      log('error', 'getUserMedia FAILED: ' + e.name + ': ' + e.message);
      throw e;
    }

    session.localStream = stream;
    var trackInfo = stream.getTracks().map(function(t) {
      return t.kind + ':' + t.label + ':' + t.readyState;
    }).join(', ');
    log('info', 'Local stream acquired: [' + trackInfo + ']');

    log('debug', 'Creating RTCPeerConnection with ICE servers: ' + JSON.stringify(ICE_SERVERS));
    var pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });
    session.pc = pc;

    log('debug', 'Initial PC state: signaling=' + pc.signalingState + ', ice=' + pc.iceConnectionState + ', connection=' + pc.connectionState);

    stream.getTracks().forEach(function(track) {
      log('info', 'Adding local track to PC: ' + track.kind + ' (id=' + track.id.substring(0,8) + ')');
      pc.addTrack(track, stream);
    });

    var transceivers = pc.getTransceivers();
    log('debug', 'Transceivers after adding tracks: ' + transceivers.length);
    transceivers.forEach(function(t, i) {
      log('debug', '  Transceiver ' + i + ': direction=' + t.direction + ', currentDirection=' + t.currentDirection + ', mid=' + t.mid);
    });

    pc.ontrack = function(event) {
      log('info', '*** ONTRACK EVENT ***');
      log('info', 'Track received: kind=' + event.track.kind + ', id=' + event.track.id.substring(0,8) + ', readyState=' + event.track.readyState);
      log('debug', 'Event streams count: ' + (event.streams ? event.streams.length : 0));
      if (event.streams && event.streams.length > 0) {
        var remoteTrackInfo = event.streams[0].getTracks().map(function(t) {
          return t.kind + ':' + t.readyState + ':enabled=' + t.enabled;
        }).join(', ');
        log('info', 'Remote stream tracks: [' + remoteTrackInfo + ']');
        session._emitRemoteStream(event.streams[0]);
      } else if (event.track) {
        log('warn', 'No streams in ontrack event, creating MediaStream from track');
        session._emitRemoteStream(new MediaStream([event.track]));
      } else {
        log('error', 'ontrack event has no track or streams!');
      }
    };

    pc.onicecandidate = function(event) {
      if (event.candidate) {
        var c = event.candidate;
        log('debug', 'ICE candidate: type=' + (c.type || 'unknown') + ', protocol=' + (c.protocol || 'unknown') + ', address=' + (c.address || 'hidden'));
        session.channel.send({ type: SIG_ICE, candidate: event.candidate.toJSON() }).catch(function(){});
      } else {
        log('info', 'ICE gathering complete (null candidate)');
      }
    };

    pc.oniceconnectionstatechange = function() {
      var state = pc.iceConnectionState;
      log('info', 'ICE CONNECTION STATE: ' + state);
      if (state === 'connected') {
        log('info', 'ICE connected! Media should be flowing.');
      } else if (state === 'completed') {
        log('info', 'ICE completed - best path found');
      } else if (state === 'failed') {
        log('error', 'ICE FAILED! Possible causes: NAT/firewall blocking, need TURN server, network unreachable');
        pc.getStats().then(function(stats) {
          stats.forEach(function(report) {
            if (report.type === 'candidate-pair' && report.state === 'failed') {
              log('error', 'Failed candidate pair: local=' + report.localCandidateId + ', remote=' + report.remoteCandidateId);
            }
          });
        });
      } else if (state === 'disconnected') {
        log('warn', 'ICE disconnected - temporary connectivity loss');
      }
    };

    pc.onicegatheringstatechange = function() {
      log('debug', 'ICE gathering state: ' + pc.iceGatheringState);
    };

    pc.onsignalingstatechange = function() {
      log('debug', 'Signaling state: ' + pc.signalingState);
    };

    pc.onconnectionstatechange = function() {
      var state = pc.connectionState;
      log('info', 'CONNECTION STATE: ' + state);
      session._emitStateChange(state);
      if (state === 'connected') {
        log('info', 'WebRTC fully connected!');
        logConnectionStats(pc);
      } else if (state === 'failed') {
        log('error', 'Connection FAILED! Check ICE state above for details.');
        session.hangup();
      } else if (state === 'disconnected') {
        log('warn', 'Connection disconnected');
        session.hangup();
      }
    };

    pc.onnegotiationneeded = function() {
      log('debug', 'Negotiation needed event fired');
    };

    return pc;
  }

  function logConnectionStats(pc) {
    pc.getStats().then(function(stats) {
      stats.forEach(function(report) {
        if (report.type === 'candidate-pair' && report.state === 'succeeded') {
          log('info', 'Active candidate pair: local=' + report.localCandidateId + ', remote=' + report.remoteCandidateId);
        }
        if (report.type === 'local-candidate') {
          log('debug', 'Local candidate: ' + report.candidateType + ' ' + (report.protocol || '') + ' ' + (report.address || 'hidden') + ':' + (report.port || ''));
        }
        if (report.type === 'remote-candidate') {
          log('debug', 'Remote candidate: ' + report.candidateType + ' ' + (report.protocol || '') + ' ' + (report.address || 'hidden') + ':' + (report.port || ''));
        }
      });
    }).catch(function(e) {
      log('warn', 'Could not get connection stats: ' + e.message);
    });
  }

  // ── Signaling handler ───────────────────────────────────────────────────────

  function handleSignaling(payload, env) {
    if (!payload) return;

    // In native mode Go handles all call signaling; browser has no session.
    // Only process call-request here (already suppressed separately) — skip
    // offer/answer/ICE entirely to avoid "no session/pc" noise in the logs.
    if (window._callNativeMode) return;

    var channelId = env.channel;
    var session = activeCalls[channelId];

    if (!payload.type) return;

    log('debug', 'Signaling received: type=' + payload.type + ', channel=' + channelId + ', hasSession=' + !!session);

    switch (payload.type) {
      case SIG_CALL_REQ:
        log('info', 'Incoming call request from: ' + env.from);
        notifyIncoming(channelId, env.from, payload);
        break;

      case SIG_CALL_ACK:
        log('info', 'Call acknowledged by remote peer');
        if (session && session.isInitiator) {
          createAndSendOffer(session);
        } else {
          log('warn', 'Received ACK but no session or not initiator');
        }
        break;

      case SIG_OFFER:
        log('info', 'Received SDP offer');
        if (session && session.pc) {
          handleOffer(session, payload);
        } else {
          log('error', 'Received offer but no session/pc! session=' + !!session + ', pc=' + !!(session && session.pc));
        }
        break;

      case SIG_ANSWER:
        log('info', 'Received SDP answer');
        if (session && session.pc) {
          handleAnswer(session, payload);
        } else {
          log('error', 'Received answer but no session/pc!');
        }
        break;

      case SIG_ICE:
        if (session && session.pc) {
          handleICE(session, payload);
        } else {
          log('warn', 'Received ICE candidate but no session/pc');
        }
        break;

      case SIG_HANGUP:
        log('info', 'Remote peer hung up');
        if (session) {
          session._ended = true;
          session._cleanup();
          session._emitHangup();
        }
        closeChannel(channelId);
        break;
    }
  }

  async function createAndSendOffer(session) {
    try {
      log('info', 'Creating SDP offer...');
      var offer = await session.pc.createOffer();
      var videoLines = (offer.sdp.match(/m=video/g) || []).length;
      var audioLines = (offer.sdp.match(/m=audio/g) || []).length;
      log('debug', 'Offer contains: ' + videoLines + ' video, ' + audioLines + ' audio media lines');
      await session.pc.setLocalDescription(offer);
      log('info', 'Sending offer to remote peer...');
      await session.channel.send({ type: SIG_OFFER, sdp: session.pc.localDescription.sdp });
      log('debug', 'Offer sent successfully');
    } catch(e) {
      log('error', 'Failed to create/send offer: ' + e.message);
      session.hangup();
    }
  }

  async function handleOffer(session, payload) {
    try {
      log('debug', 'Setting remote description (offer), SDP length: ' + payload.sdp.length);
      await session.pc.setRemoteDescription(new RTCSessionDescription({ type: "offer", sdp: payload.sdp }));
      var transceivers = session.pc.getTransceivers();
      log('debug', 'Transceivers after remote description: ' + transceivers.length);
      transceivers.forEach(function(t, i) {
        log('debug', '  Transceiver ' + i + ': direction=' + t.direction + ', currentDirection=' + t.currentDirection);
      });
      log('info', 'Creating SDP answer...');
      var answer = await session.pc.createAnswer();
      await session.pc.setLocalDescription(answer);
      log('info', 'Sending answer to remote peer...');
      await session.channel.send({ type: SIG_ANSWER, sdp: session.pc.localDescription.sdp });
      log('debug', 'Answer sent successfully');
    } catch(e) {
      log('error', 'Failed to handle offer: ' + e.message);
      session.hangup();
    }
  }

  async function handleAnswer(session, payload) {
    try {
      log('debug', 'Setting remote description (answer), SDP length: ' + payload.sdp.length);
      await session.pc.setRemoteDescription(new RTCSessionDescription({ type: "answer", sdp: payload.sdp }));
      log('info', 'Remote description (answer) set - waiting for ICE to connect...');
      var transceivers = session.pc.getTransceivers();
      transceivers.forEach(function(t, i) {
        log('debug', '  Transceiver ' + i + ': direction=' + t.direction + ', currentDirection=' + t.currentDirection);
      });
    } catch(e) {
      log('error', 'Failed to handle answer: ' + e.message);
    }
  }

  async function handleICE(session, payload) {
    try {
      if (payload.candidate) {
        var c = payload.candidate;
        log('debug', 'Adding remote ICE candidate: type=' + (c.type || 'unknown') + ', protocol=' + (c.protocol || 'unknown'));
        await session.pc.addIceCandidate(new RTCIceCandidate(payload.candidate));
        log('debug', 'Remote ICE candidate added');
      }
    } catch(e) {
      log('error', 'Failed to add ICE candidate: ' + e.message);
    }
  }

  // ── Incoming call notification ──────────────────────────────────────────────

  function notifyIncoming(channelId, fromPeer, payload) {
    // In native mode call-native.js handles incoming calls via Goop.mq.subscribe('call:*').
    // Suppress the browser modal here to avoid showing two incoming-call dialogs.
    if (window._callNativeMode) {
      log('debug', 'notifyIncoming suppressed — native call mode active');
      return;
    }
    log('info', 'Processing incoming call: channel=' + channelId + ', from=' + fromPeer);

    var info = {
      channelId: channelId,
      peerId: fromPeer,
      constraints: payload.constraints || { video: true, audio: true },

      accept: async function(constraints) {
        log('info', 'Accepting incoming call...');
        var c = constraints || info.constraints;
        var channel = { id: channelId, remotePeer: fromPeer, send: makeCallSend(fromPeer, channelId) };

        var session = new CallSession(channel, false);
        activeCalls[channelId] = session;

        await setupPeerConnection(session, c);

        log('info', 'Sending call ACK to initiator...');
        await channel.send({ type: SIG_CALL_ACK }).catch(function(){});
        log('debug', 'ACK sent, waiting for offer...');

        return session;
      },

      reject: function() {
        log('info', 'Rejecting incoming call');
        if (window.Goop && window.Goop.mq) {
          window.Goop.mq.sendCall(fromPeer, channelId, { type: SIG_HANGUP }).catch(function() {});
        }
      }
    };

    for (var i = 0; i < incomingHandlers.length; i++) {
      try { incomingHandlers[i](info); } catch(e) { log('error', 'Incoming handler error: ' + e.message); }
    }
  }

  // ── Call send factory ───────────────────────────────────────────────────────
  // Returns a send(payload) function that routes each call signal type to the
  // appropriate typed Goop.mq helper. Falls back to sendCall for unknown types.

  function makeCallSend(peerId, channelId) {
    return function (p) {
      var mq = window.Goop && window.Goop.mq;
      if (!mq) return Promise.reject(new Error('MQ not available'));
      switch (p.type) {
        case SIG_CALL_REQ: return mq.sendCallRequest(peerId, channelId, p.constraints);
        case SIG_CALL_ACK: return mq.sendCallAck(peerId, channelId);
        case SIG_OFFER:    return mq.sendCallOffer(peerId, channelId, p.sdp);
        case SIG_ANSWER:   return mq.sendCallAnswer(peerId, channelId, p.sdp);
        case SIG_ICE:      return mq.sendCallICE(peerId, channelId, p.candidate);
        case SIG_HANGUP:   return mq.sendCallHangup(peerId, channelId);
        default:           return mq.sendCall(peerId, channelId, p);
      }
    };
  }

  // ── Start listening for signaling ───────────────────────────────────────────

  function ensureListening() {
    if (listening) return;
    listening = true;
    log('debug', 'Starting to listen for call signaling messages via MQ');
    // Subscribe to all call:* topics; handleSignaling receives (payload, {channel,from})
    function initMQSig() {
      if (!window.Goop || !window.Goop.mq) { setTimeout(initMQSig, 100); return; }
      window.Goop.mq.onCall( function(from, topic, payload, ack) {
        var channelId = topic.replace(/^call:/, '');
        handleSignaling(payload, { channel: channelId, from: from });
        ack();
      });
    }
    initMQSig();
  }

  // ── Public API ──────────────────────────────────────────────────────────────

  Goop.call = {
    start: async function(peerId, constraints) {
      log('info', '========== STARTING CALL ==========');
      log('info', 'Target peer: ' + peerId);
      ensureListening();

      var c = constraints || { video: true, audio: true };
      log('debug', 'Call constraints: ' + JSON.stringify(c));

      // Create a virtual MQ channel (no server-side channel registration needed).
      var channelId = 'mc-' + Math.random().toString(36).slice(2, 10);
      var channel = {
        id: channelId,
        remotePeer: peerId,
        send: makeCallSend(peerId, channelId),
      };
      log('info', 'MQ channel created: ' + channel.id);

      var session = new CallSession(channel, true);
      activeCalls[channel.id] = session;

      // Set up media and RTCPeerConnection first so session.pc is ready before
      // the callee can accept and send SIG_CALL_ACK (which triggers createAndSendOffer).
      await setupPeerConnection(session, c);

      // Now invite the callee. No server-side channel registration needed —
      // the call-request goes directly over MQ.
      if (!session._ended) {
        log('info', 'Sending call-request to ' + peerId);
        session.channel.send({ type: SIG_CALL_REQ, constraints: c }).catch(function() {});
      }

      return session;
    },

    onIncoming: function(callback) {
      ensureListening();
      incomingHandlers.push(callback);
    },


    activeCalls: function() {
      var out = [];
      for (var id in activeCalls) { out.push(activeCalls[id]); }
      return out;
    }
  };
})();
