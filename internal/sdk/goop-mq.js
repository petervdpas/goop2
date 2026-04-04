(() => {
  window.Goop = window.Goop || {};

  let sse = null;
  const subs = [];

  function matchTopic(pattern, topic) {
    if (pattern.endsWith("*")) {
      return topic.startsWith(pattern.slice(0, -1));
    }
    return topic === pattern;
  }

  function dispatch(from, topic, payload) {
    let handled = false;
    subs.forEach(function(sub) {
      if (matchTopic(sub.topic, topic)) {
        handled = true;
        try {
          sub.fn(from, topic, payload, function() {
            sendAck(payload && payload.id, from);
          });
        } catch (_) {}
      }
    });
    if (!handled && payload && payload.id) {
      sendAck(payload.id, from);
    }
  }

  function sendAck(msgId, fromPeerID) {
    if (!msgId) return;
    fetch("/api/mq/ack", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ msg_id: msgId, from_peer_id: fromPeerID || "" }),
    }).catch(function() {});
  }

  function ensureSSE() {
    if (sse) return;
    sse = new EventSource("/api/mq/events");

    sse.addEventListener("message", function(e) {
      try {
        var evt = JSON.parse(e.data);
        if (evt.type === "message" && evt.msg) {
          var msg = evt.msg;
          dispatch(evt.from || "", msg.topic, msg.payload);
        }
      } catch (_) {}
    });

    sse.onerror = function() {
      sse.close();
      sse = null;
      setTimeout(ensureSSE, 3000);
    };
  }

  window.Goop.mq = {
    subscribe(topic, fn) {
      var sub = { topic: topic, fn: fn };
      subs.push(sub);
      ensureSSE();
      return function() {
        var idx = subs.indexOf(sub);
        if (idx >= 0) subs.splice(idx, 1);
      };
    },

    send(peerId, topic, payload) {
      return fetch("/api/mq/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ peer_id: peerId, topic: topic, payload: payload }),
      }).then(function(r) {
        if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
        return r.json();
      });
    },

    unsubscribe() {
      if (sse) {
        sse.close();
        sse = null;
      }
      subs.length = 0;
    },
  };
})();
