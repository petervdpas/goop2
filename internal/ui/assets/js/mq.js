/**
 * 00-mq.js — Message Queue layer.
 *
 * Provides Goop.mq: a reliable message transport backed by IndexedDB (outbox +
 * inbox), HTTP (/api/mq/*), and SSE (/api/mq/events).
 *
 * IndexedDB is reset on every app start (session-only semantics: identical to
 * the previous in-memory ring buffer, but with retry support).
 *
 * Load order: must come before any file that uses Goop.mq (i.e. before 05-layout.js).
 */
(function () {
  "use strict";

  // ── Logging ─────────────────────────────────────────────────────────────────
  function log(level, msg) {
    if (window.Goop && window.Goop.log && window.Goop.log[level]) {
      window.Goop.log[level]("mq", msg);
    } else {
      (console[level] || console.log)("[mq]", msg);
    }
  }

  // ── UUID v4 ──────────────────────────────────────────────────────────────────
  function uuid4() {
    if (crypto && crypto.randomUUID) return crypto.randomUUID();
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function (c) {
      var r = (Math.random() * 16) | 0;
      return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
    });
  }

  // ── IndexedDB ────────────────────────────────────────────────────────────────
  var DB_NAME = "goop-mq";
  var DB_VERSION = 1;
  var _db = null; // resolved after init
  var _dbReady = false;
  var _dbQueue = []; // callbacks waiting for DB

  function withDB(fn) {
    if (_db) { fn(_db); return; }
    _dbQueue.push(fn);
  }

  function openDB() {
    // Reset on every app start: delete then re-open.
    var del = indexedDB.deleteDatabase(DB_NAME);
    del.onsuccess = del.onerror = function () {
      var req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = function (e) {
        var db = e.target.result;

        // outbox: messages we are sending
        var outbox = db.createObjectStore("outbox", { keyPath: "id" });
        outbox.createIndex("status", "status", { unique: false });

        // inbox: messages we have received
        var inbox = db.createObjectStore("inbox", { keyPath: ["from", "seq"] });
        inbox.createIndex("processed", "processed", { unique: false });
      };
      req.onsuccess = function (e) {
        _db = e.target.result;
        _dbReady = true;
        var q = _dbQueue.splice(0);
        q.forEach(function (fn) { fn(_db); });
        log("info", "IndexedDB ready (session-only)");
      };
      req.onerror = function () {
        log("warn", "IndexedDB unavailable — MQ will operate without persistence");
        _dbReady = true;
        var q = _dbQueue.splice(0);
        q.forEach(function (fn) { fn(null); });
      };
    };
  }

  // Write a record to an object store. db may be null (no-op).
  function dbPut(storeName, record) {
    withDB(function (db) {
      if (!db) return;
      try {
        db.transaction(storeName, "readwrite").objectStore(storeName).put(record);
      } catch (e) { log("warn", "dbPut failed: " + e); }
    });
  }

  // Delete a record from an object store by key (single value or array key).
  function dbDelete(storeName, key) {
    withDB(function (db) {
      if (!db) return;
      try {
        db.transaction(storeName, "readwrite").objectStore(storeName).delete(key);
      } catch (e) { log("warn", "dbDelete failed: " + e); }
    });
  }

  // Read all records from an object store.
  function dbGetAll(storeName, cb) {
    withDB(function (db) {
      if (!db) { cb([]); return; }
      try {
        var req = db.transaction(storeName, "readonly").objectStore(storeName).getAll();
        req.onsuccess = function () { cb(req.result || []); };
        req.onerror   = function () { cb([]); };
      } catch (e) { cb([]); }
    });
  }

  // ── Topic subscriptions ──────────────────────────────────────────────────────
  var _subs = []; // [{topic, fn}]

  // ── Peer metadata cache ─────────────────────────────────────────────────────
  // Populated by peer:announce messages pushed from Go via PublishLocal.
  // Keyed by full peer ID; values match the peer:announce payload shape.
  var _peerMeta = {};

  function matchTopic(pattern, topic) {
    if (pattern.endsWith("*")) {
      return topic.startsWith(pattern.slice(0, -1));
    }
    return topic === pattern;
  }

  function dispatch(from, topic, payload, ackFn) {
    var handled = false;
    _subs.forEach(function (sub) {
      if (matchTopic(sub.topic, topic)) {
        handled = true;
        try { sub.fn(from, topic, payload, ackFn); } catch (e) {
          log("error", "subscriber error on topic " + topic + ": " + e);
        }
      }
    });
    if (!handled) {
      // Auto-ack unhandled messages so the sender doesn't time out.
      ackFn();
    }
  }

  // ── SSE (/api/mq/events) ────────────────────────────────────────────────────
  var _es = null;
  var _esReconnectTimer = null;

  function ensureSSE() {
    if (_es) return;
    _es = new EventSource("/api/mq/events");

    _es.addEventListener("message", function (e) {
      try {
        var evt = JSON.parse(e.data);
        handleSSEEvent(evt);
      } catch (err) {
        log("error", "SSE parse error: " + err);
      }
    });

    _es.onerror = function () {
      log("warn", "SSE disconnected, reconnecting in 3s");
      _es.close();
      _es = null;
      clearTimeout(_esReconnectTimer);
      _esReconnectTimer = setTimeout(ensureSSE, 3000);
    };
  }

  function handleSSEEvent(evt) {
    if (evt.type === "delivered") {
      // Sender-side: our outbox entry was processed by the remote browser.
      markDelivered(evt.msg_id);
      return;
    }

    if (evt.type === "message" && evt.msg) {
      var msg   = evt.msg;
      var from  = evt.from || "";
      var msgId = msg.id;
      var seq   = msg.seq;
      var topic = msg.topic;
      var payload = msg.payload;

      // Deduplicate via [from, seq] compound key.
      withDB(function (db) {
        if (!db) {
          // No IndexedDB — dispatch directly.
          var ackFn = makeAckFn(msgId, from);
          dispatch(from, topic, payload, ackFn);
          return;
        }

        var tx    = db.transaction("inbox", "readwrite");
        var store = tx.objectStore("inbox");
        var getReq = store.get([from, seq]);
        getReq.onsuccess = function () {
          if (getReq.result) {
            // Already processed — silently re-ack.
            sendAck(msgId, from);
            return;
          }
          // Store and dispatch.
          store.put({ from: from, seq: seq, id: msgId, topic: topic, payload: payload, processed: 0, received: Date.now() });
          var ackFn = makeAckFn(msgId, from);
          dispatch(from, topic, payload, function () {
            ackFn();
            // Mark processed in inbox.
            withDB(function (db2) {
              if (!db2) return;
              try {
                var tx2 = db2.transaction("inbox", "readwrite");
                var rec = { from: from, seq: seq, id: msgId, topic: topic, payload: payload, processed: 1, received: Date.now() };
                tx2.objectStore("inbox").put(rec);
              } catch (_) {}
            });
          });
        };
      });
    }
  }

  function makeAckFn(msgId, fromPeerID) {
    var called = false;
    return function () {
      if (called) return;
      called = true;
      sendAck(msgId, fromPeerID);
    };
  }

  function sendAck(msgId, fromPeerID) {
    fetch("/api/mq/ack", {
      method:  "POST",
      headers: { "Content-Type": "application/json" },
      body:    JSON.stringify({ msg_id: msgId, from_peer_id: fromPeerID }),
    }).catch(function (e) { log("warn", "ack send failed: " + e); });
  }

  // ── Outbox management ────────────────────────────────────────────────────────

  function markDelivered(msgId) {
    if (!msgId) return;
    dbDelete("outbox", msgId);
    log("info", "delivered: " + msgId.substring(0, 8));
  }

  function markFailed(entry) {
    entry.status = "failed";
    dbPut("outbox", entry);
    if (typeof _onFailed === "function") {
      try { _onFailed(entry); } catch (_) {}
    }
  }

  var _onFailed = null;

  // ── Core send ────────────────────────────────────────────────────────────────

  /**
   * send(peerID, topic, payload) → Promise
   * Writes to outbox, POSTs to /api/mq/send.
   * Retries are handled by the 30s interval timer.
   */
  function mqSend(peerID, topic, payload) {
    var msgId = uuid4();
    var entry = {
      id:          msgId,
      peerID:      peerID,
      topic:       topic,
      payload:     payload,
      status:      "pending",
      attempts:    0,
      created:     Date.now(),
      lastAttempt: 0,
    };
    dbPut("outbox", entry);

    return doSend(entry).then(function (result) {
      markDelivered(msgId);
      return result;
    }).catch(function (err) {
      entry.status = "pending"; // will be retried
      dbPut("outbox", entry);
      throw err;
    });
  }

  function doSend(entry) {
    entry.status      = "in-flight";
    entry.attempts    = (entry.attempts || 0) + 1;
    entry.lastAttempt = Date.now();
    dbPut("outbox", entry);

    return fetch("/api/mq/send", {
      method:  "POST",
      headers: { "Content-Type": "application/json" },
      body:    JSON.stringify({
        peer_id: entry.peerID,
        topic:   entry.topic,
        payload: entry.payload,
        msg_id:  entry.id,
      }),
    }).then(function (res) {
      if (!res.ok) {
        return res.text().then(function (t) { throw new Error("HTTP " + res.status + ": " + t); });
      }
      return res.json();
    });
  }

  /**
   * broadcast(topic, payload) — fetch /api/peers then send individually.
   */
  function mqBroadcast(topic, payload) {
    return fetch("/api/peers").then(function (r) { return r.json(); }).then(function (peers) {
      if (!Array.isArray(peers)) return;
      var promises = peers.map(function (p) {
        return mqSend(p.ID, topic, payload).catch(function (e) {
          log("warn", "broadcast to " + p.ID.substring(0, 8) + " failed: " + e);
        });
      });
      return Promise.all(promises);
    });
  }

  // ── Retry timer (every 30 s) ─────────────────────────────────────────────────
  var MAX_ATTEMPTS = 10;

  setInterval(function () {
    dbGetAll("outbox", function (entries) {
      var now = Date.now();
      entries.forEach(function (entry) {
        if (entry.status === "failed") return;
        if (entry.status === "in-flight" && (now - entry.lastAttempt) < 30000) return;
        if (entry.attempts >= MAX_ATTEMPTS) {
          markFailed(entry);
          return;
        }
        doSend(entry).then(function () { markDelivered(entry.id); }).catch(function () {
          entry.status = "pending";
          dbPut("outbox", entry);
        });
      });
    });
  }, 30000);

  // ── Public API ───────────────────────────────────────────────────────────────

  window.Goop = window.Goop || {};

  window.Goop.mq = {
    /**
     * send(peerID, topic, payload) → Promise<{msg_id, status}>
     */
    send: mqSend,

    /**
     * subscribe(topic, fn(from, topic, payload, ackFn))
     * topic supports exact match or prefix wildcard ending with '*'.
     * Returns unsubscribe function.
     */
    subscribe: function (topic, fn) {
      var sub = { topic: topic, fn: fn };
      _subs.push(sub);
      ensureSSE();
      return function () {
        var idx = _subs.indexOf(sub);
        if (idx >= 0) _subs.splice(idx, 1);
      };
    },

    /**
     * broadcast(topic, payload) → Promise
     * Sends to all known peers individually.
     */
    broadcast: mqBroadcast,

    /**
     * onFailed(fn) — register a callback for messages that exceeded MAX_ATTEMPTS.
     */
    onFailed: function (fn) { _onFailed = fn; },

    /**
     * getPeer(peerID) → peer metadata object or null.
     * Returns the last known peer:announce payload for peerID, or null if unknown.
     */
    getPeer: function (peerID) { return _peerMeta[peerID] || null; },

    /**
     * getPeerName(peerID) → display name string or null.
     * Shorthand for getPeer(peerID)?.content.
     */
    getPeerName: function (peerID) {
      var p = _peerMeta[peerID];
      return (p && p.content) || null;
    },
  };

  // Auto-subscribe to peer:announce / peer:gone to maintain the local peer cache.
  // These are internal topics published by Go via PublishLocal — no P2P send.
  window.Goop.mq.subscribe("peer:announce", function (from, topic, payload, ack) {
    if (payload && payload.peerID) _peerMeta[payload.peerID] = payload;
    ack();
  });
  window.Goop.mq.subscribe("peer:gone", function (from, topic, payload, ack) {
    if (payload && payload.peerID) delete _peerMeta[payload.peerID];
    ack();
  });

  // Start IndexedDB and SSE immediately.
  openDB();
  ensureSSE();

})();
