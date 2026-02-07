// internal/ui/assets/js/goop-call.js
//
// WebRTC video/audio call API built on top of Goop.realtime channels.
// Signaling flows over the realtime channel (offer/answer/ICE candidates).
//
// Usage:
//   <script src="/assets/js/goop-realtime.js"></script>
//   <script src="/assets/js/goop-call.js"></script>
//
//   // Start a call
//   const call = await Goop.call.start(peerId, { video: true, audio: true });
//   call.onRemoteStream(function(stream) { videoEl.srcObject = stream; });
//   call.hangup();
//
//   // Listen for incoming calls
//   Goop.call.onIncoming(function(info) {
//     // info.peerId, info.channelId
//     info.accept({ video: true, audio: true });
//     // or info.reject();
//   });
//
(() => {
  window.Goop = window.Goop || {};

  var ICE_SERVERS = [
    { urls: "stun:stun.l.google.com:19302" },
    { urls: "stun:stun1.l.google.com:19302" }
  ];

  // Signaling message types
  var SIG_OFFER     = "call-offer";
  var SIG_ANSWER    = "call-answer";
  var SIG_ICE       = "ice-candidate";
  var SIG_HANGUP    = "call-hangup";
  var SIG_CALL_REQ  = "call-request";
  var SIG_CALL_ACK  = "call-ack";

  var incomingHandlers = [];
  var activeCalls = {};   // channelId -> CallSession
  var listening = false;

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
  }

  CallSession.prototype.onRemoteStream = function(cb) {
    this._onRemoteStream.push(cb);
    // If we already have a remote stream, fire immediately
    if (this.remoteStream) {
      try { cb(this.remoteStream); } catch(e) { console.error(e); }
    }
  };

  CallSession.prototype.onHangup = function(cb) {
    this._onHangup.push(cb);
  };

  CallSession.prototype.onStateChange = function(cb) {
    this._onStateChange.push(cb);
  };

  CallSession.prototype._emitRemoteStream = function(stream) {
    this.remoteStream = stream;
    for (var i = 0; i < this._onRemoteStream.length; i++) {
      try { this._onRemoteStream[i](stream); } catch(e) { console.error(e); }
    }
  };

  CallSession.prototype._emitHangup = function() {
    for (var i = 0; i < this._onHangup.length; i++) {
      try { this._onHangup[i](); } catch(e) { console.error(e); }
    }
  };

  CallSession.prototype._emitStateChange = function(state) {
    for (var i = 0; i < this._onStateChange.length; i++) {
      try { this._onStateChange[i](state); } catch(e) { console.error(e); }
    }
  };

  CallSession.prototype.hangup = function() {
    if (this._ended) return;
    this._ended = true;

    // Send hangup signal
    try {
      this.channel.send({ type: SIG_HANGUP });
    } catch(e) { /* ignore */ }

    this._cleanup();
    this._emitHangup();
  };

  CallSession.prototype._cleanup = function() {
    if (this.pc) {
      this.pc.close();
      this.pc = null;
    }
    if (this.localStream) {
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
    return tracks[0].enabled;
  };

  CallSession.prototype.toggleVideo = function() {
    if (!this.localStream) return false;
    var tracks = this.localStream.getVideoTracks();
    if (tracks.length === 0) return false;
    tracks[0].enabled = !tracks[0].enabled;
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
      if (camId && c.video) {
        c.video = typeof c.video === 'object' ? Object.assign({}, c.video) : {};
        c.video.deviceId = { ideal: camId };
      }
      if (micId && c.audio) {
        c.audio = typeof c.audio === 'object' ? Object.assign({}, c.audio) : {};
        c.audio.deviceId = { ideal: micId };
      }
    } catch(e) { /* config unavailable, use defaults */ }
    return c;
  }

  async function setupPeerConnection(session, constraints) {
    var stream = await navigator.mediaDevices.getUserMedia(await applyPreferredDevices(constraints));
    session.localStream = stream;

    var pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });
    session.pc = pc;

    // Add local tracks
    stream.getTracks().forEach(function(track) {
      pc.addTrack(track, stream);
    });

    // Handle remote tracks
    pc.ontrack = function(event) {
      if (event.streams && event.streams[0]) {
        session._emitRemoteStream(event.streams[0]);
      }
    };

    // Handle ICE candidates
    pc.onicecandidate = function(event) {
      if (event.candidate) {
        session.channel.send({
          type: SIG_ICE,
          candidate: event.candidate.toJSON()
        });
      }
    };

    // Connection state changes
    pc.onconnectionstatechange = function() {
      session._emitStateChange(pc.connectionState);
      if (pc.connectionState === "failed" || pc.connectionState === "disconnected") {
        session.hangup();
      }
    };

    return pc;
  }

  // ── Signaling handler ───────────────────────────────────────────────────────

  function handleSignaling(payload, env) {
    if (!payload || !payload.type) return;

    var channelId = env.channel;
    var session = activeCalls[channelId];

    switch (payload.type) {
      case SIG_CALL_REQ:
        // Incoming call request on existing channel
        notifyIncoming(channelId, env.from, payload);
        break;

      case SIG_CALL_ACK:
        // Remote accepted — initiator creates offer
        if (session && session.isInitiator) {
          createAndSendOffer(session);
        }
        break;

      case SIG_OFFER:
        if (session && session.pc) {
          handleOffer(session, payload);
        }
        break;

      case SIG_ANSWER:
        if (session && session.pc) {
          handleAnswer(session, payload);
        }
        break;

      case SIG_ICE:
        if (session && session.pc) {
          handleICE(session, payload);
        }
        break;

      case SIG_HANGUP:
        if (session) {
          session._ended = true;
          session._cleanup();
          session._emitHangup();
        }
        break;
    }
  }

  async function createAndSendOffer(session) {
    try {
      var offer = await session.pc.createOffer();
      await session.pc.setLocalDescription(offer);
      session.channel.send({
        type: SIG_OFFER,
        sdp: session.pc.localDescription.sdp
      });
    } catch(e) {
      console.error("call: failed to create offer:", e);
      session.hangup();
    }
  }

  async function handleOffer(session, payload) {
    try {
      await session.pc.setRemoteDescription(
        new RTCSessionDescription({ type: "offer", sdp: payload.sdp })
      );
      var answer = await session.pc.createAnswer();
      await session.pc.setLocalDescription(answer);
      session.channel.send({
        type: SIG_ANSWER,
        sdp: session.pc.localDescription.sdp
      });
    } catch(e) {
      console.error("call: failed to handle offer:", e);
      session.hangup();
    }
  }

  async function handleAnswer(session, payload) {
    try {
      await session.pc.setRemoteDescription(
        new RTCSessionDescription({ type: "answer", sdp: payload.sdp })
      );
    } catch(e) {
      console.error("call: failed to handle answer:", e);
    }
  }

  async function handleICE(session, payload) {
    try {
      if (payload.candidate) {
        await session.pc.addIceCandidate(new RTCIceCandidate(payload.candidate));
      }
    } catch(e) {
      console.error("call: failed to add ICE candidate:", e);
    }
  }

  // ── Incoming call notification ──────────────────────────────────────────────

  function notifyIncoming(channelId, fromPeer, payload) {
    var info = {
      channelId: channelId,
      peerId: fromPeer,
      constraints: payload.constraints || { video: true, audio: true },

      accept: async function(constraints) {
        var c = constraints || info.constraints;
        var channel = { id: channelId, remotePeer: fromPeer, send: function(p) {
          return Goop.realtime.channels().then(function() {
            // Use the existing channel's send via realtime API
            return fetch("/api/realtime/send", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ channel_id: channelId, payload: p })
            });
          });
        }};

        // Try to find the actual channel wrapper
        var existingChannels = await Goop.realtime.channels();
        for (var i = 0; i < existingChannels.length; i++) {
          if (existingChannels[i].id === channelId) {
            channel.remotePeer = existingChannels[i].remote_peer;
            break;
          }
        }

        var session = new CallSession(channel, false);
        activeCalls[channelId] = session;

        await setupPeerConnection(session, c);

        // Send ack so initiator creates offer
        await fetch("/api/realtime/send", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ channel_id: channelId, payload: { type: SIG_CALL_ACK } })
        });

        return session;
      },

      reject: function() {
        fetch("/api/realtime/send", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ channel_id: channelId, payload: { type: SIG_HANGUP } })
        });
      }
    };

    for (var i = 0; i < incomingHandlers.length; i++) {
      try { incomingHandlers[i](info); } catch(e) { console.error(e); }
    }
  }

  // ── Start listening for signaling ───────────────────────────────────────────

  function ensureListening() {
    if (listening) return;
    listening = true;
    Goop.realtime.onMessage(handleSignaling);
  }

  // ── Public API ──────────────────────────────────────────────────────────────

  Goop.call = {
    // Start a call to a peer
    start: async function(peerId, constraints) {
      ensureListening();

      var c = constraints || { video: true, audio: true };

      // Create realtime channel
      var channel = await Goop.realtime.connect(peerId);

      var session = new CallSession(channel, true);
      activeCalls[channel.id] = session;

      // Set up WebRTC
      await setupPeerConnection(session, c);

      // Send call request
      channel.send({ type: SIG_CALL_REQ, constraints: c });

      return session;
    },

    // Listen for incoming calls
    onIncoming: function(callback) {
      ensureListening();
      incomingHandlers.push(callback);
    },

    // Get active call for a channel
    getCall: function(channelId) {
      return activeCalls[channelId] || null;
    },

    // Get all active calls
    activeCalls: function() {
      var out = [];
      for (var id in activeCalls) {
        out.push(activeCalls[id]);
      }
      return out;
    }
  };
})();
