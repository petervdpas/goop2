//
// Chat client for site pages — bridged over Goop MQ.
// API surface identical to the previous SSE-based version.
//
// Usage:
//   <script src="/sdk/goop-chat.js"></script>
//
//   // send a broadcast message
//   await Goop.chat.broadcast("Hello everyone!");
//
//   // send a direct message
//   await Goop.chat.send(peerId, "Hey there");
//
//   // fetch broadcast history (returns empty — history not stored over MQ)
//   const msgs = await Goop.chat.broadcasts();
//
//   // fetch direct message history with a peer (returns empty — not stored over MQ)
//   const dms = await Goop.chat.messages(peerId);
//
//   // subscribe to all incoming messages (broadcast + direct)
//   Goop.chat.subscribe(function(msg) {
//     // msg: {from, content, type, timestamp}
//     // type: "broadcast" | "direct"
//   });
//
//   Goop.chat.unsubscribe();
//
// MQ topics:
//   chat:broadcast   — broadcast message to all peers
//   chat:direct      — direct message from one peer to another
//
(() => {
  window.Goop = window.Goop || {};

  var _unsub = null;

  function waitMQ(fn) {
    if (window.Goop && window.Goop.mq) { fn(); return; }
    var t = setInterval(function() {
      if (window.Goop && window.Goop.mq) { clearInterval(t); fn(); }
    }, 50);
  }

  window.Goop.chat = {
    /** Send a broadcast message to all peers */
    broadcast: function(content) {
      return new Promise(function(resolve, reject) {
        waitMQ(function() {
          Goop.mq.broadcast('chat:broadcast', { content: content, timestamp: Date.now() })
            .then(resolve).catch(reject);
        });
      });
    },

    /** Send a direct message to a specific peer */
    send: function(to, content) {
      return new Promise(function(resolve, reject) {
        waitMQ(function() {
          Goop.mq.send(to, 'chat:direct', { content: content, timestamp: Date.now() })
            .then(resolve).catch(reject);
        });
      });
    },

    /** Fetch broadcast message history — not persisted over MQ, returns empty array */
    broadcasts: function() {
      return Promise.resolve([]);
    },

    /** Fetch direct message history — not persisted over MQ, returns empty array */
    messages: function(peerId) {
      return Promise.resolve([]);
    },

    /**
     * Subscribe to incoming messages via MQ.
     * callback(msg) where msg has {from, content, type, timestamp}
     * Returns an object with a close() method.
     */
    subscribe: function(callback) {
      if (_unsub) { _unsub(); _unsub = null; }

      waitMQ(function() {
        _unsub = Goop.mq.subscribe('chat:*', function(from, topic, payload, ack) {
          var type = topic === 'chat:broadcast' ? 'broadcast' : 'direct';
          if (callback) {
            try {
              callback({
                from:      from,
                content:   payload && payload.content,
                type:      type,
                timestamp: (payload && payload.timestamp) || Date.now(),
              });
            } catch (_) {}
          }
          ack();
        });
      });

      return {
        close: function() {
          if (_unsub) { _unsub(); _unsub = null; }
        },
      };
    },

    /** Stop receiving messages */
    unsubscribe: function() {
      if (_unsub) { _unsub(); _unsub = null; }
    },
  };
})();
