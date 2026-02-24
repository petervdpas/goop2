/**
 * mq/topics.js — MQ topic registry.
 *
 * Defines all known topic strings as constants and exposes typed subscribe/send
 * helpers on Goop.mq. Mirrors internal/mq/topics.go — keep both in sync.
 *
 * Requires mq/base.js to be loaded first.
 *
 * ── Topic map ────────────────────────────────────────────────────────────────
 *
 *   peer:announce             Go → browser   peer metadata update (PublishLocal)
 *   peer:gone                 Go → browser   peer pruned (PublishLocal)
 *
 *   call:{channelID}          P2P + local    call signaling (see CALL_TYPES below)
 *   call:loopback:{channelID} Go → browser   LocalPC ICE candidates (Phase 4, PublishLocal)
 *
 *   group:{groupID}:{type}    P2P            join, welcome, members, msg, leave, close, ping, pong
 *   group.invite              P2P            group invitation (not scoped to groupID)
 *
 *   listen:{groupID}:state    Go → browser   listen state (PublishLocal)
 *   chat                      P2P            direct peer message
 *   chat.broadcast            P2P broadcast  message to all peers
 *   log:mq                    Go → browser   MQ event log entry (PublishLocal)
 *
 * ── Call signaling protocol ───────────────────────────────────────────────────
 *
 * All call signals travel on topic "call:{channelID}" with a "type" field:
 *
 *   caller                               callee
 *   ────────────────────────────────────────────────────────────────────────
 *   sendCallRequest ───────────────────► (incoming call modal shown)
 *                   ◄─────────────────── sendCallAck   (on accept)
 *   sendCallOffer   ───────────────────►
 *                   ◄─────────────────── sendCallAnswer
 *   sendCallICE ◄──────────────────────► sendCallICE  (trickle, both ways)
 *   sendCallHangup  ───────────────────► (or either side ends the call)
 *
 * Native-mode (Go/Pion): after call-request/ack, Go handles SDP offer/answer
 * and peer ICE internally. Browser receives media via WebSocket + WebM/MSE
 * (/api/call/media/{channel}). Loopback ICE uses "call:loopback:{channelID}".
 */
(function () {
  "use strict";

  var mq = window.Goop.mq;

  // ── Topic constants ───────────────────────────────────────────────────────────
  mq.TOPICS = Object.freeze({
    PEER_ANNOUNCE:         "peer:announce",
    PEER_GONE:             "peer:gone",
    CALL_PREFIX:           "call:",          // + channelID
    CALL_LOOPBACK_PREFIX:  "call:loopback:", // + channelID (Go → browser, Phase 4)
    GROUP_PREFIX:          "group:",         // + groupID + ":" + type
    GROUP_INVITE:          "group.invite",
    LISTEN_PREFIX:         "listen:",        // + groupID + ":state"
    CHAT:                  "chat",
    CHAT_BROADCAST:        "chat.broadcast",
    LOG_MQ:                "log:mq",
  });

  // ── Call signal type constants ────────────────────────────────────────────────
  // Value of the "type" field inside every call:* payload. Mirrors Go CallType* consts.
  mq.CALL_TYPES = Object.freeze({
    REQUEST:      "call-request",  // caller → callee: initiate a call
    ACK:          "call-ack",      // callee → caller: accepted, SDP exchange starts
    OFFER:        "call-offer",    // caller → callee: SDP offer
    ANSWER:       "call-answer",   // callee → caller: SDP answer
    ICE:          "ice-candidate", // either → other: trickle ICE candidate
    HANGUP:       "call-hangup",   // either side: end the call
    LOOPBACK_ICE: "loopback-ice",  // Go → browser: LocalPC ICE candidate (Phase 4)
  });

  // ── Typed subscribe helpers ────────────────────────────────────────────────────
  // Each returns an unsubscribe function (same as Goop.mq.subscribe).

  /** peer:announce — peer metadata update from Go PeerTable bridge */
  mq.onPeerAnnounce = function (fn) { return mq.subscribe(mq.TOPICS.PEER_ANNOUNCE, fn); };

  /** peer:gone — peer pruned from in-memory PeerTable */
  mq.onPeerGone = function (fn) { return mq.subscribe(mq.TOPICS.PEER_GONE, fn); };

  /**
   * onCall(fn) — all call:* signaling from any channel.
   * fn(from, topic, payload, ack) — payload.type is one of mq.CALL_TYPES.
   * channelId = topic.slice(5) strips the "call:" prefix.
   */
  mq.onCall = function (fn) { return mq.subscribe(mq.TOPICS.CALL_PREFIX + "*", fn); };

  /**
   * onLoopbackICE(channelId, fn) — Go's LocalPC ICE candidates for Phase 4 loopback.
   * fn(candidate) — RTCIceCandidateInit: { candidate, sdpMid, sdpMLineIndex }
   * Returns an unsubscribe function.
   */
  mq.onLoopbackICE = function (channelId, fn) {
    return mq.subscribe(mq.TOPICS.CALL_LOOPBACK_PREFIX + channelId, function (from, topic, payload, ack) {
      ack();
      if (payload && payload.candidate) {
        try { fn(payload.candidate); } catch (_) {}
      }
    });
  };

  /** group:{groupID}:{type} — all group protocol messages */
  mq.onGroup = function (fn) { return mq.subscribe(mq.TOPICS.GROUP_PREFIX + "*", fn); };

  /** group.invite — incoming group invitation */
  mq.onGroupInvite = function (fn) { return mq.subscribe(mq.TOPICS.GROUP_INVITE, fn); };

  /** listen:{groupID}:state — listen state updates from Go */
  mq.onListen = function (fn) { return mq.subscribe(mq.TOPICS.LISTEN_PREFIX + "*", fn); };

  /** chat — direct P2P message */
  mq.onChat = function (fn) { return mq.subscribe(mq.TOPICS.CHAT, fn); };

  /** chat.broadcast — broadcast message to all peers */
  mq.onChatBroadcast = function (fn) { return mq.subscribe(mq.TOPICS.CHAT_BROADCAST, fn); };

  /** log:mq — MQ event log entry from Go */
  mq.onLogMQ = function (fn) { return mq.subscribe(mq.TOPICS.LOG_MQ, fn); };

  // ── Typed send helpers — call protocol ───────────────────────────────────────

  /**
   * sendCall(peerId, channelId, payload) → Promise
   * Low-level: sends any call:* payload. Use the typed helpers below where possible.
   */
  mq.sendCall = function (peerId, channelId, payload) {
    return mq.send(peerId, mq.TOPICS.CALL_PREFIX + channelId, payload);
  };

  /**
   * sendCallRequest(peerId, channelId, constraints) → Promise
   * caller → callee: invite to a call.
   * constraints: optional browser-mode getUserMedia constraints { video, audio }.
   */
  mq.sendCallRequest = function (peerId, channelId, constraints) {
    var p = { type: mq.CALL_TYPES.REQUEST };
    if (constraints) p.constraints = constraints;
    return mq.sendCall(peerId, channelId, p);
  };

  /**
   * sendCallAck(peerId, channelId) → Promise
   * callee → caller: call accepted; triggers caller to create and send SDP offer.
   */
  mq.sendCallAck = function (peerId, channelId) {
    return mq.sendCall(peerId, channelId, { type: mq.CALL_TYPES.ACK });
  };

  /**
   * sendCallOffer(peerId, channelId, sdp) → Promise
   * caller → callee: WebRTC SDP offer (after receiving ack).
   */
  mq.sendCallOffer = function (peerId, channelId, sdp) {
    return mq.sendCall(peerId, channelId, { type: mq.CALL_TYPES.OFFER, sdp: sdp });
  };

  /**
   * sendCallAnswer(peerId, channelId, sdp) → Promise
   * callee → caller: WebRTC SDP answer.
   */
  mq.sendCallAnswer = function (peerId, channelId, sdp) {
    return mq.sendCall(peerId, channelId, { type: mq.CALL_TYPES.ANSWER, sdp: sdp });
  };

  /**
   * sendCallICE(peerId, channelId, candidate) → Promise
   * either → other: trickle ICE candidate.
   * candidate: RTCIceCandidateInit { candidate, sdpMid, sdpMLineIndex }
   */
  mq.sendCallICE = function (peerId, channelId, candidate) {
    return mq.sendCall(peerId, channelId, { type: mq.CALL_TYPES.ICE, candidate: candidate });
  };

  /**
   * sendCallHangup(peerId, channelId) → Promise
   * either side: end the call.
   */
  mq.sendCallHangup = function (peerId, channelId) {
    return mq.sendCall(peerId, channelId, { type: mq.CALL_TYPES.HANGUP });
  };

  // ── Typed send helpers — chat ────────────────────────────────────────────────

  /** sendChat(peerId, payload) → Promise — P2P direct message */
  mq.sendChat = function (peerId, payload) {
    return mq.send(peerId, mq.TOPICS.CHAT, payload);
  };

  /** broadcastChat(payload) → Promise — broadcast to all peers */
  mq.broadcastChat = function (payload) {
    return mq.broadcast(mq.TOPICS.CHAT_BROADCAST, payload);
  };

})();
