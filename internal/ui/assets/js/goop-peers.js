//
// Peer discovery client for site pages.
// Usage:
//
//   <script src="/assets/js/goop-peers.js"></script>
//
//   // list online peers
//   const peers = await Goop.peers.list();
//   // => [{ID, Content, LastSeen}, ...]
//
//   // subscribe to live updates
//   Goop.peers.subscribe({
//     onSnapshot(peers)   { /* full list */ },
//     onUpdate(peer)      { /* peer came online or updated */ },
//     onRemove(peer)      { /* peer went offline */ },
//   });
//
//   // unsubscribe
//   Goop.peers.unsubscribe();
//
(() => {
  window.Goop = window.Goop || {};

  let sse = null;

  window.Goop.peers = {
    /** Fetch current peer list. Returns [{ID, Content, LastSeen}] */
    list() {
      return fetch("/api/peers").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /**
     * Subscribe to live peer updates via SSE.
     * callbacks: { onSnapshot(peers), onUpdate(peer), onRemove(peer) }
     */
    subscribe(callbacks) {
      if (sse) sse.close();

      sse = new EventSource("/api/peers/events");
      const cb = callbacks || {};

      sse.addEventListener("snapshot", (e) => {
        try {
          const data = JSON.parse(e.data);
          if (cb.onSnapshot) cb.onSnapshot(data.peers || []);
        } catch (_) {}
      });

      sse.addEventListener("update", (e) => {
        try {
          if (cb.onUpdate) cb.onUpdate(JSON.parse(e.data));
        } catch (_) {}
      });

      sse.addEventListener("remove", (e) => {
        try {
          if (cb.onRemove) cb.onRemove(JSON.parse(e.data));
        } catch (_) {}
      });

      sse.onerror = () => {
        if (cb.onError) cb.onError();
      };

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
