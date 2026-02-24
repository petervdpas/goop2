/**
 * mq/peers.js — Peer metadata cache.
 *
 * Maintains a local mirror of peer state by subscribing to peer:announce
 * and peer:gone events. Exposes two lookups on Goop.mq:
 *
 *   Goop.mq.getPeer(peerID)     → payload object | null
 *   Goop.mq.getPeerName(peerID) → display name string | null
 *
 * Requires mq/base.js and mq/topics.js to be loaded first.
 */
(function () {
  "use strict";

  var mq = window.Goop.mq;
  var _peerMeta = {}; // peerID → PeerAnnouncePayload

  // ── Public lookups ───────────────────────────────────────────────────────────

  /** getPeer(peerID) → last known peer:announce payload, or null if unknown */
  mq.getPeer = function (peerID) { return _peerMeta[peerID] || null; };

  /** getPeerName(peerID) → display name (content field), or null if unknown */
  mq.getPeerName = function (peerID) {
    var p = _peerMeta[peerID];
    return (p && p.content) || null;
  };

  // ── Built-in subscriptions ────────────────────────────────────────────────────

  mq.onPeerAnnounce(function (from, topic, payload, ack) {
    if (payload && payload.peerID) _peerMeta[payload.peerID] = payload;
    ack();
  });

  mq.onPeerGone(function (from, topic, payload, ack) {
    if (payload && payload.peerID) delete _peerMeta[payload.peerID];
    ack();
  });

})();
