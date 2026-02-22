/**
 * call-native.js — Native Go/Pion WebRTC call stack for Linux (Phase 1/2 skeleton).
 *
 * Loaded after video-call.js. On load it queries /api/call/mode.
 * If the server returns {"mode":"native"}, this module replaces Goop.call
 * with a native-mode implementation backed by the Go/Pion stack.
 *
 * The NativeSession API mirrors the browser CallSession exactly, so call-ui.js
 * needs zero changes — it calls session.onRemoteStream(), session.hangup(), etc.
 * and they just work.
 *
 * Loopback flow (Phase 4):
 *   Browser creates RTCPeerConnection({iceServers:[]}) → offer → POST /loopback/offer
 *   → Go creates LocalPC, answers → browser setRemoteDescription(answer)
 *   → ICE trickle via SSE /loopback/ice (Go→browser) and POST /loopback/ice (browser→Go)
 *   → browser gets real MediaStream from LocalPC tracks → <video>.srcObject
 */
(function () {
  "use strict";

  // ── NativeSession ──────────────────────────────────────────────────────────
  // Mirrors the CallSession API used by call-ui.js.

  class NativeSession {
    constructor(channelId) {
      this.channelId = channelId;
      this.isNative = true;

      this._remoteStreamCbs = [];
      this._hangupCbs = [];
      this._stateCbs = [];

      // Loopback RTCPeerConnection (Phase 4 — initialised in _connectLoopback).
      this._loopbackPc = null;
    }

    /** Register a callback for when the remote MediaStream becomes available. */
    onRemoteStream(cb) {
      this._remoteStreamCbs.push(cb);
    }

    /** Register a callback for when the call ends (local or remote hangup). */
    onHangup(cb) {
      this._hangupCbs.push(cb);
    }

    /** Register a callback for connection-state changes (string label). */
    onStateChange(cb) {
      this._stateCbs.push(cb);
    }

    /** Mute/unmute local audio. Returns promise resolving to {muted:bool}. */
    async toggleAudio() {
      const res = await fetch("/api/call/toggle-audio", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ channel_id: this.channelId }),
      });
      return res.ok ? res.json() : { muted: false };
    }

    /** Enable/disable local video. Returns promise resolving to {disabled:bool}. */
    async toggleVideo() {
      const res = await fetch("/api/call/toggle-video", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ channel_id: this.channelId }),
      });
      return res.ok ? res.json() : { disabled: false };
    }

    /** Hang up the call. */
    async hangup() {
      await fetch("/api/call/hangup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ channel_id: this.channelId }),
      }).catch(() => {});
      this._emitHangup();
    }

    // ── Internal helpers ──

    _emitRemoteStream(stream) {
      this._remoteStreamCbs.forEach((cb) => cb(stream));
    }

    _emitHangup() {
      this._hangupCbs.forEach((cb) => cb());
    }

    _emitState(state) {
      this._stateCbs.forEach((cb) => cb(state));
    }

    /**
     * Phase 4: establish loopback RTCPeerConnection so the browser gets a
     * real MediaStream from Go's LocalPC without needing getUserMedia.
     *
     * Until Phase 4 is implemented on the Go side, the /loopback/offer endpoint
     * returns a stub SDP and no tracks will appear. The wiring is already here
     * so Phase 4 only requires filling in the Go session.go side.
     */
    async _connectLoopback() {
      try {
        this._emitState("connecting");

        const pc = new RTCPeerConnection({ iceServers: [] });
        this._loopbackPc = pc;

        // When Go's LocalPC sends tracks, pipe them to the UI.
        pc.ontrack = (e) => {
          if (e.streams && e.streams[0]) {
            this._emitRemoteStream(e.streams[0]);
            this._emitState("connected");
          }
        };

        // Trickle our ICE candidates to Go's LocalPC.
        pc.onicecandidate = (e) => {
          if (!e.candidate) return;
          fetch(`/api/call/loopback/${this.channelId}/ice`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              candidate: e.candidate.candidate,
              sdpMid: e.candidate.sdpMid,
              sdpMLineIndex: e.candidate.sdpMLineIndex,
            }),
          }).catch(() => {});
        };

        // Subscribe to Go's loopback ICE candidates via SSE.
        const iceUrl = `/api/call/loopback/${this.channelId}/ice`;
        const evtSrc = new EventSource(iceUrl);
        evtSrc.addEventListener("candidate", (e) => {
          try {
            const c = JSON.parse(e.data);
            pc.addIceCandidate(new RTCIceCandidate(c)).catch(() => {});
          } catch (_) {}
        });

        // Request video + audio tracks from Go's LocalPC.
        const offer = await pc.createOffer({
          offerToReceiveAudio: true,
          offerToReceiveVideo: true,
        });
        await pc.setLocalDescription(offer);

        const resp = await fetch(`/api/call/loopback/${this.channelId}/offer`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sdp: offer.sdp }),
        });
        if (!resp.ok) {
          console.warn("[call-native] loopback offer failed:", resp.status);
          evtSrc.close();
          return;
        }
        const { sdp } = await resp.json();
        if (sdp) {
          await pc.setRemoteDescription({ type: "answer", sdp });
        } else {
          // Phase 1/2: stub answer — no media yet, expected.
          console.info("[call-native] loopback stub (Phase 4 pending) — no remote video yet");
          this._emitState("stub");
        }
      } catch (err) {
        console.warn("[call-native] loopback setup error:", err);
        this._emitState("error");
      }
    }
  }

  // ── NativeCallManager ──────────────────────────────────────────────────────
  // Replaces Goop.call when mode === "native".

  class NativeCallManager {
    constructor() {
      this._incomingCbs = [];
      this._evtSource = null;
    }

    /**
     * Initiate an outbound call.
     * The caller must have already created (or will create) a realtime channel
     * for signaling — channelId is that channel's ID.
     */
    async start(channelId, remotePeerId /*, constraints — ignored in native mode */) {
      const res = await fetch("/api/call/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ channel_id: channelId, remote_peer: remotePeerId }),
      });
      if (!res.ok) {
        throw new Error(`[call-native] start failed: ${res.status}`);
      }
      const sess = new NativeSession(channelId);
      // Connect loopback so browser gets a MediaStream (Phase 4).
      sess._connectLoopback();
      return sess;
    }

    /**
     * Register a handler for incoming calls.
     * The handler receives an object with channelId, remotePeerId, accept(), reject().
     */
    onIncoming(cb) {
      this._incomingCbs.push(cb);
      this._ensureEventSource();
    }

    // ── SSE subscription for incoming calls ──

    _ensureEventSource() {
      if (this._evtSource) return;
      const es = new EventSource("/api/call/events");
      this._evtSource = es;

      es.addEventListener("call", (e) => {
        try {
          const data = JSON.parse(e.data);
          if (data.type !== "incoming-call") return;
          this._handleIncoming(data);
        } catch (_) {}
      });

      es.onerror = () => {
        // Reconnect after a short delay.
        this._evtSource = null;
        setTimeout(() => this._ensureEventSource(), 3000);
      };
    }

    async _handleIncoming(data) {
      const { channel_id: channelId, remote_peer: remotePeerId } = data;

      const incoming = {
        channelId,
        remotePeerId,

        accept: async () => {
          const res = await fetch("/api/call/accept", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ channel_id: channelId, remote_peer: remotePeerId }),
          });
          if (!res.ok) throw new Error(`[call-native] accept failed: ${res.status}`);
          const sess = new NativeSession(channelId);
          sess._connectLoopback();
          return sess;
        },

        reject: async () => {
          await fetch("/api/call/hangup", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ channel_id: channelId }),
          }).catch(() => {});
        },
      };

      this._incomingCbs.forEach((cb) => cb(incoming));
    }
  }

  // ── Bootstrap: query mode and optionally replace Goop.call ──────────────────

  async function init() {
    let mode = "browser";
    try {
      const res = await fetch("/api/call/mode");
      if (res.ok) {
        const data = await res.json();
        mode = data.mode || "browser";
      }
    } catch (_) {
      // Endpoint not available — stay in browser mode.
    }

    if (mode !== "native") {
      // Browser mode: existing video-call.js handles everything, nothing to do.
      console.info("[call-native] mode=browser — using browser WebRTC");
      return;
    }

    console.info("[call-native] mode=native — replacing Goop.call with Go/Pion stack");

    // Replace Goop.call so call-ui.js and peers.js use the native manager.
    if (typeof Goop === "undefined") {
      window.Goop = {};
    }
    Goop.call = new NativeCallManager();
  }

  // Run after DOM + earlier scripts are ready.
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
