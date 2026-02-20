//
// Chat client for site pages.
// Usage:
//
//   <script src="/sdk/goop-chat.js"></script>
//
//   // send a broadcast message
//   await Goop.chat.broadcast("Hello everyone!");
//
//   // send a direct message
//   await Goop.chat.send(peerId, "Hey there");
//
//   // fetch broadcast history
//   const msgs = await Goop.chat.broadcasts();
//
//   // fetch direct message history with a peer
//   const dms = await Goop.chat.messages(peerId);
//
//   // subscribe to all incoming messages (broadcast + direct)
//   Goop.chat.subscribe(function(msg) {
//     // msg: {from, content, type, timestamp}
//   });
//
//   Goop.chat.unsubscribe();
//
(() => {
  window.Goop = window.Goop || {};

  let sse = null;

  function post(url, body) {
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then((r) => {
      if (!r.ok) return r.text().then((t) => { throw new Error(t); });
      return r.json();
    });
  }

  window.Goop.chat = {
    /** Send a broadcast message to all peers */
    broadcast(content) {
      return post("/api/chat/broadcast", { content });
    },

    /** Send a direct message to a specific peer */
    send(to, content) {
      return post("/api/chat/send", { to, content });
    },

    /** Fetch broadcast message history */
    broadcasts() {
      return fetch("/api/chat/broadcasts").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /** Fetch direct message history with a peer */
    messages(peerId) {
      const url = peerId
        ? "/api/chat/messages?peer=" + encodeURIComponent(peerId)
        : "/api/chat/messages";
      return fetch(url).then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /**
     * Subscribe to incoming messages via SSE.
     * callback(msg) where msg has {from, content, type, timestamp}
     */
    subscribe(callback) {
      if (sse) sse.close();

      sse = new EventSource("/api/chat/events");

      sse.addEventListener("message", (e) => {
        try {
          if (callback) callback(JSON.parse(e.data));
        } catch (_) {}
      });

      sse.onerror = () => {};

      return sse;
    },

    /** Close the SSE connection */
    unsubscribe() {
      if (sse) {
        sse.close();
        sse = null;
      }
    },
  };
})();
