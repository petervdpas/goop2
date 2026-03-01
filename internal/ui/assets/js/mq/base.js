/**
 * mq/base.js — MQ transport engine.
 *
 * Provides the core Goop.mq object:
 *   Goop.mq.send(peerID, topic, payload)  → Promise
 *   Goop.mq.subscribe(topic, fn)          → unsubFn   (topic supports '*' suffix)
 *   Goop.mq.broadcast(topic, payload)     → Promise
 *   Goop.mq.onFailed(fn)                  register failed-message callback
 *
 * Topic constants, typed helpers, and the peer cache live in mq/topics.js
 * and mq/peers.js — load those after this file.
 *
 * Load order: mq/base.js → mq/topics.js → mq/peers.js
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
  var _db = null;
  var _dbReady = false;
  var _dbQueue = [];

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
        var outbox = db.createObjectStore("outbox", { keyPath: "id" });
        outbox.createIndex("status", "status", { unique: false });
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

  function dbPut(storeName, record) {
    withDB(function (db) {
      if (!db) return;
      try {
        db.transaction(storeName, "readwrite").objectStore(storeName).put(record);
      } catch (e) { log("warn", "dbPut failed: " + e); }
    });
  }

  function dbDelete(storeName, key) {
    withDB(function (db) {
      if (!db) return;
      try {
        db.transaction(storeName, "readwrite").objectStore(storeName).delete(key);
      } catch (e) { log("warn", "dbDelete failed: " + e); }
    });
  }

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
  var _subs = [];

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
      ackFn(); // auto-ack unhandled messages
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
      markDelivered(evt.msg_id);
      return;
    }

    if (evt.type === "message" && evt.msg) {
      var msg     = evt.msg;
      var from    = evt.from || "";
      var msgId   = msg.id;
      var seq     = msg.seq;
      var topic   = msg.topic;
      var payload = msg.payload;

      withDB(function (db) {
        if (!db) {
          dispatch(from, topic, payload, makeAckFn(msgId, from));
          return;
        }

        var tx    = db.transaction("inbox", "readwrite");
        var store = tx.objectStore("inbox");
        var getReq = store.get([from, seq]);
        getReq.onsuccess = function () {
          if (getReq.result) {
            sendAck(msgId, from); // already processed — re-ack silently
            return;
          }
          store.put({ from: from, seq: seq, id: msgId, topic: topic, payload: payload, processed: 0, received: Date.now() });
          var ackFn = makeAckFn(msgId, from);
          dispatch(from, topic, payload, function () {
            ackFn();
            withDB(function (db2) {
              if (!db2) return;
              try {
                db2.transaction("inbox", "readwrite").objectStore("inbox")
                   .put({ from: from, seq: seq, id: msgId, topic: topic, payload: payload, processed: 1, received: Date.now() });
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
    log("debug", "delivered: " + msgId.substring(0, 8));
  }

  var _onFailed = null;

  function markFailed(entry) {
    entry.status = "failed";
    dbPut("outbox", entry);
    if (typeof _onFailed === "function") {
      try { _onFailed(entry); } catch (_) {}
    }
  }

  // ── Core send ────────────────────────────────────────────────────────────────
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
      entry.status = "pending";
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

  function mqBroadcast(topic, payload) {
    return fetch("/api/peers").then(function (r) { return r.json(); }).then(function (peers) {
      if (!Array.isArray(peers)) return;
      return Promise.all(peers.map(function (p) {
        return mqSend(p.ID, topic, payload).catch(function (e) {
          log("warn", "broadcast to " + p.ID.substring(0, 8) + " failed: " + e);
        });
      }));
    });
  }

  // ── Retry timer (every 30 s) ──────────────────────────────────────────────────
  var MAX_ATTEMPTS = 10;

  setInterval(function () {
    dbGetAll("outbox", function (entries) {
      var now = Date.now();
      entries.forEach(function (entry) {
        if (entry.status === "failed") return;
        if (entry.status === "in-flight" && (now - entry.lastAttempt) < 30000) return;
        if (entry.attempts >= MAX_ATTEMPTS) { markFailed(entry); return; }
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
     * Writes to IndexedDB outbox, POSTs to /api/mq/send. Retried on failure.
     */
    send: mqSend,

    /**
     * subscribe(topic, fn(from, topic, payload, ackFn)) → unsubscribeFn
     * topic supports exact match or '*'-suffix wildcard (e.g. 'call:*').
     * Unhandled topics are auto-acked. Starts SSE connection on first call.
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
     * Fetches /api/peers and sends individually to each online peer.
     */
    broadcast: mqBroadcast,

    /**
     * onFailed(fn(entry)) — callback for messages that exceeded MAX_ATTEMPTS.
     */
    onFailed: function (fn) { _onFailed = fn; },
  };

  // Start IndexedDB and SSE immediately.
  openDB();
  ensureSSE();

})();
